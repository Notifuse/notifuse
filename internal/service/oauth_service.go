package service

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// oauthAuthenticator is what the authorization server needs from the auth
// service: an API key pasted on the page, the console session of an owner,
// and the account behind an access token.
type oauthAuthenticator interface {
	AuthenticateAPIKeyToken(ctx context.Context, token string) (*domain.User, error)
	AuthenticateUserFromContext(ctx context.Context) (*domain.User, error)
	AuthenticateUserForWorkspace(ctx context.Context, workspaceID string) (context.Context, *domain.User, *domain.UserWorkspace, error)
}

// oauthKeyCreator creates an API key under the rules of workspaces.createAPIKey,
// letting the caller write it in its own transaction.
type oauthKeyCreator interface {
	CreateAPIKeyWith(ctx context.Context, workspaceID string, emailPrefix string, permissions domain.UserPermissions, store func(ctx context.Context, key *domain.User, membership *domain.UserWorkspace) error) (*domain.User, error)
}

// oauthWorkspaceReader reads workspaces and memberships.
type oauthWorkspaceReader interface {
	GetByID(ctx context.Context, id string) (*domain.Workspace, error)
	GetUserWorkspaces(ctx context.Context, userID string) ([]*domain.UserWorkspace, error)
	List(ctx context.Context) ([]*domain.Workspace, error)
}

// OAuthServiceConfig builds the OAuth service.
type OAuthServiceConfig struct {
	Repo       domain.OAuthRepository
	Auth       oauthAuthenticator
	Keys       oauthKeyCreator
	Workspaces oauthWorkspaceReader
	// Fetcher fetches client metadata documents; NewHTTPOAuthClientFetcher in
	// production.
	Fetcher  OAuthClientFetcher
	Recorder domain.AuditLogRecorder
	// APIEndpoint returns API_ENDPOINT, read at each call.
	APIEndpoint func() string
	IsRootEmail func(string) bool
	Logger      logger.Logger
	// Now is the clock; time.Now when nil.
	Now func() time.Time
}

// OAuthService is the OAuth authorization server of the MCP endpoint, without
// HTTP: authorization requests, the authorization page's calls, the token and
// revocation endpoints, token resolution for /mcp, the MCP tab's connections,
// and the periodic job.
type OAuthService struct {
	repo        domain.OAuthRepository
	auth        oauthAuthenticator
	keys        oauthKeyCreator
	workspaces  oauthWorkspaceReader
	fetcher     OAuthClientFetcher
	recorder    domain.AuditLogRecorder
	apiEndpoint func() string
	isRootEmail func(string) bool
	logger      logger.Logger
	now         func() time.Time
}

var _ domain.OAuthService = (*OAuthService)(nil)

// NewOAuthService builds the service.
func NewOAuthService(cfg OAuthServiceConfig) *OAuthService {
	s := &OAuthService{
		repo:        cfg.Repo,
		auth:        cfg.Auth,
		keys:        cfg.Keys,
		workspaces:  cfg.Workspaces,
		fetcher:     cfg.Fetcher,
		recorder:    cfg.Recorder,
		apiEndpoint: cfg.APIEndpoint,
		isRootEmail: cfg.IsRootEmail,
		logger:      cfg.Logger,
		now:         cfg.Now,
	}
	if s.now == nil {
		s.now = time.Now
	}
	if s.recorder == nil {
		s.recorder = domain.NoopAuditLogRecorder{}
	}
	if s.apiEndpoint == nil {
		s.apiEndpoint = func() string { return "" }
	}
	return s
}

// SetFetcher replaces the metadata document fetcher, for the integration
// suite.
func (s *OAuthService) SetFetcher(fetcher OAuthClientFetcher) {
	s.fetcher = fetcher
}

// randomSecret returns n random bytes, base64url without padding.
func randomSecret(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		// crypto/rand does not fail on supported platforms; a token must
		// never be built from anything weaker.
		panic(fmt.Sprintf("crypto/rand failed: %v", err))
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}

// endpoint is the canonical API endpoint when the authorization server can
// run on it.
func (s *OAuthService) endpoint() (domain.CanonicalEndpoint, error) {
	endpoint, err := domain.CanonicalAPIEndpoint(s.apiEndpoint())
	if err != nil || endpoint.OAuthUnavailableReason() != "" {
		return domain.CanonicalEndpoint{}, domain.ErrOAuthUnavailable
	}
	return endpoint, nil
}

// Unavailable says why the authorization server is off, or "" when it runs.
func (s *OAuthService) Unavailable() string {
	endpoint, err := domain.CanonicalAPIEndpoint(s.apiEndpoint())
	if err != nil {
		return "API_ENDPOINT is not an http(s) URL"
	}
	return endpoint.OAuthUnavailableReason()
}

// Issuer is the issuer, and the iss of every authorization response: the
// canonical API endpoint.
func (s *OAuthService) Issuer() (string, error) {
	endpoint, err := s.endpoint()
	if err != nil {
		return "", err
	}
	return endpoint.URL, nil
}

// CreateAuthorizationRequest validates GET /oauth/authorize and stores the
// request. Syntax is checked before any fetch, so made-up requests cost no
// outbound call. Every refusal is an *domain.OAuthAuthorizeError, rendered as
// an error page, never a redirect.
func (s *OAuthService) CreateAuthorizationRequest(ctx context.Context, p domain.OAuthAuthorizeParams) (*domain.OAuthAuthorizationStarted, error) {
	endpoint, err := s.endpoint()
	if err != nil {
		return nil, err
	}
	refuse := func(reason, message string) error {
		return &domain.OAuthAuthorizeError{Reason: reason, Message: message}
	}
	if p.ResponseType != "code" {
		return nil, refuse(domain.OAuthAuthorizeUnsupportedResponseType, "response_type must be code")
	}
	if err := domain.ValidateOAuthClientID(p.ClientID); err != nil {
		return nil, err
	}
	if err := domain.ValidateRedirectURISyntax(p.RedirectURI); err != nil {
		return nil, refuse(domain.OAuthAuthorizeInvalidRedirect, err.Error())
	}
	if err := domain.ValidateCodeChallenge(p.CodeChallenge, p.CodeChallengeMethod); err != nil {
		return nil, refuse(domain.OAuthAuthorizeInvalidPKCE, err.Error())
	}
	resource := endpoint.MCPURL()
	if p.Resource != "" {
		if canonical, err := domain.CanonicalResource(p.Resource); err != nil || canonical != resource {
			return nil, refuse(domain.OAuthAuthorizeInvalidResource, "resource must be "+resource)
		}
	}

	now := s.now()
	client, err := s.resolveClient(ctx, p.ClientID, now)
	if err != nil {
		return nil, err
	}
	if !domain.MatchRedirectURI(p.RedirectURI, client.RedirectURIs) {
		return nil, refuse(domain.OAuthAuthorizeInvalidRedirect, "redirect_uri is not one of the application's redirect URIs")
	}

	binding := randomSecret(32)
	request := &domain.OAuthAuthorizationRequest{
		ID:       randomSecret(16),
		ClientID: client.ClientID,
		ClientSnapshot: domain.OAuthClientSnapshot{
			ClientID:     client.ClientID,
			ClientName:   client.ClientName,
			RedirectURIs: client.RedirectURIs,
		},
		RedirectURI:        p.RedirectURI,
		State:              p.State,
		CodeChallenge:      p.CodeChallenge,
		Resource:           resource,
		BrowserBindingHash: domain.HashOAuthSecret(binding),
		CreatedAt:          now,
		ExpiresAt:          now.Add(domain.OAuthRequestLifetime),
	}
	if err := s.repo.CreateRequest(ctx, request); err != nil {
		return nil, err
	}
	return &domain.OAuthAuthorizationStarted{RequestID: request.ID, BrowserBinding: binding, ExpiresAt: request.ExpiresAt}, nil
}

// resolveClient returns the client's metadata from the cache, or fetches it.
// A failed fetch is remembered for OAuthClientFailureCache; a new client_id is
// refused while the records without a grant are at the cap, the known clients
// excepted. A known client's record is outside the cap only once its host
// served its document: a failed fetch always counts.
func (s *OAuthService) resolveClient(ctx context.Context, clientID string, now time.Time) (*domain.OAuthClient, error) {
	unusable := func(why string) error {
		return &domain.OAuthAuthorizeError{Reason: domain.OAuthAuthorizeClientMetadata, Message: "the application's metadata document cannot be used: " + why}
	}
	// atCap: a new client of a known host went past the full cap. Only a
	// document its host serves is outside the cap, so its failure is not
	// recorded: any path on a known host would otherwise add a row.
	atCap := false
	cached, err := s.repo.GetClient(ctx, clientID)
	switch {
	case err == nil:
		if cached.FetchFailedUntil != nil && now.Before(*cached.FetchFailedUntil) {
			return nil, unusable("its last fetch failed, try again in a few minutes")
		}
		if cached.FetchedAt != nil && cached.CacheExpiresAt != nil && now.Before(*cached.CacheExpiresAt) {
			return cached, nil
		}
	case errors.Is(err, domain.ErrOAuthNotFound):
		count, err := s.repo.CountGrantlessClients(ctx, domain.OAuthKnownClientHosts)
		if err != nil {
			return nil, err
		}
		atCap = count >= domain.OAuthGrantlessClientCap
		if atCap && !domain.IsKnownOAuthClient(clientID) {
			return nil, &domain.OAuthAuthorizeError{Reason: domain.OAuthAuthorizeClientCap, Message: "this server is refusing new applications for now, try again later"}
		}
	default:
		return nil, err
	}

	if s.fetcher == nil {
		return nil, unusable("no fetcher is configured")
	}
	fetchCtx, cancel := context.WithTimeout(ctx, domain.OAuthClientFetchTimeout)
	defer cancel()
	document, fetchErr := s.fetcher.Fetch(fetchCtx, clientID)
	var metadata *domain.OAuthClientMetadata
	if fetchErr == nil {
		metadata, fetchErr = domain.ParseOAuthClientMetadata(clientID, document.Body)
	}
	if fetchErr != nil {
		if atCap {
			return nil, unusable(fetchErr.Error())
		}
		if err := s.repo.MarkClientFetchFailed(ctx, clientID, now.Add(domain.OAuthClientFailureCache), now); err != nil {
			return nil, err
		}
		return nil, unusable(fetchErr.Error())
	}

	expires := now.Add(oauthClientCacheTTL(document.Header, now))
	client := &domain.OAuthClient{
		ClientID:       clientID,
		ClientName:     metadata.ClientName,
		RedirectURIs:   metadata.RedirectURIs,
		Metadata:       document.Body,
		CreatedAt:      now,
		FetchedAt:      &now,
		CacheExpiresAt: &expires,
	}
	if err := s.repo.SaveClient(ctx, client); err != nil {
		return nil, err
	}
	return client, nil
}

// liveRequest returns an unconsumed, unexpired request.
func (s *OAuthService) liveRequest(ctx context.Context, requestID string, now time.Time) (*domain.OAuthAuthorizationRequest, error) {
	if requestID == "" {
		return nil, domain.ErrOAuthRequestUnavailable
	}
	request, err := s.repo.GetRequest(ctx, requestID)
	if errors.Is(err, domain.ErrOAuthNotFound) {
		return nil, domain.ErrOAuthRequestUnavailable
	}
	if err != nil {
		return nil, err
	}
	if request.ConsumedAt != nil || !now.Before(request.ExpiresAt) {
		return nil, domain.ErrOAuthRequestUnavailable
	}
	return request, nil
}

// boundRequest returns a live request started in this browser: the request
// cookie must hash to the stored binding.
func (s *OAuthService) boundRequest(ctx context.Context, requestID, binding string, now time.Time) (*domain.OAuthAuthorizationRequest, error) {
	request, err := s.liveRequest(ctx, requestID, now)
	if err != nil {
		return nil, err
	}
	if binding == "" || subtle.ConstantTimeCompare([]byte(domain.HashOAuthSecret(binding)), []byte(request.BrowserBindingHash)) != 1 {
		return nil, domain.ErrOAuthBrowserMismatch
	}
	return request, nil
}

// GetAuthorizationRequest is what the authorization page shows. With the
// console session of an owner, it lists the workspaces the caller owns, for
// the owner shortcut; a workspace whose MCP switch is off is marked.
func (s *OAuthService) GetAuthorizationRequest(ctx context.Context, requestID string) (*domain.OAuthAuthorizationRequestView, error) {
	request, err := s.liveRequest(ctx, requestID, s.now())
	if err != nil {
		return nil, err
	}
	view := &domain.OAuthAuthorizationRequestView{
		RequestID:        request.ID,
		ClientID:         request.ClientID,
		ClientName:       request.ClientSnapshot.ClientName,
		KnownClient:      domain.IsKnownOAuthClient(request.ClientID),
		LoopbackRedirect: domain.IsLoopbackRedirectURI(request.RedirectURI),
		ExpiresAt:        request.ExpiresAt,
	}
	if parsed, err := url.Parse(request.ClientID); err == nil {
		view.ClientHost = parsed.Host
	}
	if parsed, err := url.Parse(request.RedirectURI); err == nil {
		view.RedirectHost = parsed.Host
	}
	owned, err := s.ownedWorkspaces(ctx)
	if err != nil {
		return nil, err
	}
	view.OwnedWorkspaces = owned
	return view, nil
}

// ownedWorkspaces lists what a console session owns; nothing for any other
// caller or an expired session.
func (s *OAuthService) ownedWorkspaces(ctx context.Context) ([]domain.OAuthOwnedWorkspace, error) {
	if userType, _ := ctx.Value(domain.UserTypeKey).(string); userType != string(domain.UserTypeUser) {
		return nil, nil
	}
	user, err := s.auth.AuthenticateUserFromContext(ctx)
	if err != nil || user == nil || user.Type != domain.UserTypeUser {
		return nil, nil
	}
	var workspaces []*domain.Workspace
	if s.isRootEmail != nil && s.isRootEmail(user.Email) {
		if workspaces, err = s.workspaces.List(ctx); err != nil {
			return nil, err
		}
	} else {
		memberships, err := s.workspaces.GetUserWorkspaces(ctx, user.ID)
		if err != nil {
			return nil, err
		}
		for _, membership := range memberships {
			if membership.Role != "owner" {
				continue
			}
			workspace, err := s.workspaces.GetByID(ctx, membership.WorkspaceID)
			if err != nil {
				return nil, err
			}
			workspaces = append(workspaces, workspace)
		}
	}
	owned := make([]domain.OAuthOwnedWorkspace, 0, len(workspaces))
	for _, workspace := range workspaces {
		owned = append(owned, domain.OAuthOwnedWorkspace{ID: workspace.ID, Name: workspace.Name, MCPDisabled: !workspace.MCPEnabled()})
	}
	return owned, nil
}

// newGrantAndCode builds a grant for a request and its single-use code. The
// code is returned in clear once, for the redirect; only its hash is stored.
func newGrantAndCode(request *domain.OAuthAuthorizationRequest, keyID, workspaceID, via, createdBy string, now time.Time) (*domain.OAuthGrant, *domain.OAuthAuthorizationCode, string) {
	grant := &domain.OAuthGrant{
		ID:              uuid.New().String(),
		APIKeyUserID:    keyID,
		WorkspaceID:     workspaceID,
		ClientID:        request.ClientID,
		ClientName:      request.ClientSnapshot.ClientName,
		Resource:        request.Resource,
		CreatedVia:      via,
		CreatedByUserID: createdBy,
		CreatedAt:       now,
	}
	code := randomSecret(32)
	record := &domain.OAuthAuthorizationCode{
		CodeHash:      domain.HashOAuthSecret(code),
		GrantID:       grant.ID,
		ClientID:      request.ClientID,
		RedirectURI:   request.RedirectURI,
		CodeChallenge: request.CodeChallenge,
		Resource:      request.Resource,
		ExpiresAt:     now.Add(domain.OAuthCodeLifetime),
	}
	return grant, record, code
}

// authorizationRedirect appends the response parameters to the redirect URI,
// keeping its own query as it is. iss is the issuer byte for byte.
func authorizationRedirect(request *domain.OAuthAuthorizationRequest, params url.Values, issuer string) string {
	if request.State != "" {
		params.Set("state", request.State)
	}
	params.Set("iss", issuer)
	parsed, err := url.Parse(request.RedirectURI)
	if err != nil {
		return request.RedirectURI
	}
	if parsed.RawQuery == "" {
		parsed.RawQuery = params.Encode()
	} else {
		parsed.RawQuery += "&" + params.Encode()
	}
	return parsed.String()
}

// consumeAndGrant consumes the request and writes the grant and its code, in
// the transaction it is given.
func consumeAndGrant(ctx context.Context, tx domain.OAuthStore, requestID string, grant *domain.OAuthGrant, code *domain.OAuthAuthorizationCode, now time.Time) error {
	if _, err := tx.ConsumeRequest(ctx, requestID, now); err != nil {
		if errors.Is(err, domain.ErrOAuthNotFound) {
			return domain.ErrOAuthRequestUnavailable
		}
		return err
	}
	if err := tx.CreateGrant(ctx, grant); err != nil {
		return err
	}
	return tx.CreateCode(ctx, code)
}

// ApproveWithKey is oauth.approveWithKey: the key pasted on the page becomes
// the identity of the connection. The key is never stored, logged or
// returned: the grant holds its account ID.
func (s *OAuthService) ApproveWithKey(ctx context.Context, req domain.OAuthApproveWithKeyRequest, meta domain.OAuthCallMeta) (*domain.OAuthRedirect, error) {
	endpoint, err := s.endpoint()
	if err != nil {
		return nil, err
	}
	now := s.now()
	request, err := s.boundRequest(ctx, req.RequestID, req.BrowserBinding, now)
	if err != nil {
		return nil, err
	}

	key, err := s.auth.AuthenticateAPIKeyToken(ctx, req.APIKey)
	switch {
	case errors.Is(err, domain.ErrNotAnAPIKey), errors.Is(err, domain.ErrAPIKeyRevoked), errors.Is(err, ErrUserNotFound):
		return nil, domain.ErrOAuthNotAnAPIKey
	case err != nil:
		return nil, fmt.Errorf("failed to check the API key: %w", err)
	case key == nil || key.Type != domain.UserTypeAPIKey:
		return nil, domain.ErrOAuthNotAnAPIKey
	}
	memberships, err := s.workspaces.GetUserWorkspaces(ctx, key.ID)
	if err != nil {
		return nil, err
	}
	if len(memberships) == 0 {
		return nil, domain.ErrOAuthNotAnAPIKey
	}
	membership := memberships[0]
	workspace, err := s.workspaces.GetByID(ctx, membership.WorkspaceID)
	if err != nil {
		return nil, err
	}
	if !workspace.MCPEnabled() {
		return nil, domain.ErrOAuthWorkspaceMCPDisabled
	}

	grant, code, clearCode := newGrantAndCode(request, key.ID, workspace.ID, domain.OAuthGrantViaKey, "", now)
	if err := s.repo.WithTx(ctx, func(tx domain.OAuthStore) error {
		return consumeAndGrant(ctx, tx, request.ID, grant, code, now)
	}); err != nil {
		return nil, err
	}

	record := domain.NewAuditRecord(meta.IP, meta.UserAgent, meta.RequestID)
	record.SetActor(key, membership, false)
	s.recordGrantEvent(ctx, record, domain.AuditActionOAuthApproveWithKey, grant, "")

	return &domain.OAuthRedirect{RedirectURL: authorizationRedirect(request, url.Values{"code": {clearCode}}, endpoint.URL)}, nil
}

// CreateKeyAndApprove is oauth.createKeyAndApprove, the owner shortcut: a
// console session of the workspace's owner (root counts as owner) creates the
// key on the page. Every check runs first; then one transaction consumes the
// request and writes the key, the grant and the code, and any failure leaves
// none of them. The key's token is never minted.
func (s *OAuthService) CreateKeyAndApprove(ctx context.Context, req domain.OAuthCreateKeyAndApproveRequest, meta domain.OAuthCallMeta) (*domain.OAuthRedirect, error) {
	endpoint, err := s.endpoint()
	if err != nil {
		return nil, err
	}
	now := s.now()

	ownerRecord := domain.NewAuditRecord(meta.IP, meta.UserAgent, meta.RequestID)
	ownerCtx, owner, err := s.requireOwner(domain.WithAuditRecord(ctx, ownerRecord), req.WorkspaceID)
	if err != nil {
		return nil, err
	}
	request, err := s.boundRequest(ctx, req.RequestID, req.BrowserBinding, now)
	if err != nil {
		return nil, err
	}
	workspace, err := s.workspaces.GetByID(ctx, req.WorkspaceID)
	if err != nil {
		return nil, err
	}
	if !workspace.MCPEnabled() {
		return nil, domain.ErrOAuthWorkspaceMCPDisabled
	}
	keyRequest := domain.CreateAPIKeyRequest{WorkspaceID: req.WorkspaceID, EmailPrefix: req.Name, Permissions: req.Permissions}
	if err := keyRequest.Validate(); err != nil {
		return nil, domain.NewValidationError(err.Error())
	}

	keyRecord := domain.NewAuditRecord(meta.IP, meta.UserAgent, meta.RequestID)
	var grant *domain.OAuthGrant
	var clearCode string
	_, err = s.keys.CreateAPIKeyWith(domain.WithAuditRecord(ownerCtx, keyRecord), req.WorkspaceID, req.Name, req.Permissions,
		func(ctx context.Context, key *domain.User, membership *domain.UserWorkspace) error {
			var code *domain.OAuthAuthorizationCode
			grant, code, clearCode = newGrantAndCode(request, key.ID, req.WorkspaceID, domain.OAuthGrantViaOwnerShortcut, owner.ID, now)
			return s.repo.WithTx(ctx, func(tx domain.OAuthStore) error {
				if _, err := tx.ConsumeRequest(ctx, request.ID, now); err != nil {
					if errors.Is(err, domain.ErrOAuthNotFound) {
						return domain.ErrOAuthRequestUnavailable
					}
					return err
				}
				if err := tx.CreateAPIKeyAccount(ctx, key, membership); err != nil {
					return err
				}
				if err := tx.CreateGrant(ctx, grant); err != nil {
					return err
				}
				return tx.CreateCode(ctx, code)
			})
		})
	var nameTaken *domain.ErrUserExists
	if errors.As(err, &nameTaken) {
		return nil, domain.ErrOAuthKeyNameTaken
	}
	if err != nil {
		return nil, err
	}

	// The key creation row, as workspaces.createAPIKey writes it, then the
	// connection's, both naming the owner.
	keyRecord.AddMetadata("oauth_client", grant.ClientName)
	if event, skipped := keyRecord.Finalize(http.StatusOK); !skipped {
		spec := domain.AuditedActions["workspaces.createAPIKey"]
		event.Action = "workspaces.createAPIKey"
		event.Category = spec.Category
		if event.TargetType == "" {
			event.TargetType = spec.TargetType
		}
		s.recorder.Record(ctx, &event)
	}
	ownerRecord.AddMetadata("api_key_user_id", grant.APIKeyUserID)
	s.recordGrantEvent(ctx, ownerRecord, domain.AuditActionOAuthCreateKeyAndApprove, grant, "")

	return &domain.OAuthRedirect{RedirectURL: authorizationRedirect(request, url.Values{"code": {clearCode}}, endpoint.URL)}, nil
}

// oauthNotMemberMessage refuses a caller outside the workspace. RevokeGrant
// answers it as a grant that does not exist, so a grant ID tells a stranger
// nothing.
const oauthNotMemberMessage = "user is not a member of the workspace"

// requireOwner accepts a console session of an owner of the workspace, root
// counting as owner. An API key, whose role is always member, never passes.
// A session that failed verification keeps its domain.ErrSessionAuthFailed,
// which the HTTP layer answers with 401 as on every other workspace route.
func (s *OAuthService) requireOwner(ctx context.Context, workspaceID string) (context.Context, *domain.User, error) {
	if userType, _ := ctx.Value(domain.UserTypeKey).(string); userType != string(domain.UserTypeUser) {
		return ctx, nil, &domain.ErrUnauthorized{Message: "a console session is required"}
	}
	if workspaceID == "" {
		return ctx, nil, domain.NewValidationError("workspace_id is required")
	}
	ctx, user, membership, err := s.auth.AuthenticateUserForWorkspace(ctx, workspaceID)
	switch {
	case errors.Is(err, domain.ErrUserNotInWorkspace):
		return ctx, nil, &domain.ErrUnauthorized{Message: oauthNotMemberMessage}
	case err != nil:
		return ctx, nil, err
	case user == nil || user.Type != domain.UserTypeUser || membership == nil || membership.Role != "owner":
		return ctx, nil, &domain.ErrUnauthorized{Message: "user is not an owner of the workspace"}
	}
	return ctx, user, nil
}

// Deny is oauth.deny: it consumes the request and sends the browser back with
// access_denied. Nothing is created, so nothing is recorded.
func (s *OAuthService) Deny(ctx context.Context, req domain.OAuthDenyRequest) (*domain.OAuthRedirect, error) {
	endpoint, err := s.endpoint()
	if err != nil {
		return nil, err
	}
	now := s.now()
	request, err := s.boundRequest(ctx, req.RequestID, req.BrowserBinding, now)
	if err != nil {
		return nil, err
	}
	if _, err := s.repo.ConsumeRequest(ctx, request.ID, now); err != nil {
		if errors.Is(err, domain.ErrOAuthNotFound) {
			return nil, domain.ErrOAuthRequestUnavailable
		}
		return nil, err
	}
	return &domain.OAuthRedirect{RedirectURL: authorizationRedirect(request, url.Values{"error": {"access_denied"}}, endpoint.URL)}, nil
}

// issuePair writes a new access and refresh token for the grant and returns
// the response that carries them, with the refresh token's hash.
func issuePair(ctx context.Context, tx domain.OAuthStore, grant *domain.OAuthGrant, now time.Time) (*domain.OAuthTokenResponse, string, error) {
	access := domain.OAuthAccessTokenPrefix + randomSecret(32)
	refresh := domain.OAuthRefreshTokenPrefix + randomSecret(32)
	pairID := uuid.New().String()
	refreshHash := domain.HashOAuthSecret(refresh)
	err := tx.CreateTokens(ctx,
		&domain.OAuthToken{TokenHash: domain.HashOAuthSecret(access), GrantID: grant.ID, PairID: pairID, ClientID: grant.ClientID, Kind: domain.OAuthTokenKindAccess, ExpiresAt: now.Add(domain.OAuthAccessTokenLifetime)},
		&domain.OAuthToken{TokenHash: refreshHash, GrantID: grant.ID, PairID: pairID, ClientID: grant.ClientID, Kind: domain.OAuthTokenKindRefresh, ExpiresAt: now.Add(domain.OAuthRefreshTokenLifetime)},
	)
	if err != nil {
		return nil, "", err
	}
	return &domain.OAuthTokenResponse{
		AccessToken:  access,
		TokenType:    "Bearer",
		ExpiresIn:    int(domain.OAuthAccessTokenLifetime / time.Second),
		RefreshToken: refresh,
		Scope:        domain.OAuthScope,
	}, refreshHash, nil
}

// resourceMatches checks a resource sent to the token endpoint against the
// grant's, canonically; an omitted one is the grant's.
func resourceMatches(sent, bound string) bool {
	if sent == "" {
		return true
	}
	canonical, err := domain.CanonicalResource(sent)
	return err == nil && canonical == bound
}

// touchUse writes the grant's and the client's last use, at most once per
// OAuthLastUsedInterval each.
func touchUse(ctx context.Context, tx domain.OAuthStore, grant *domain.OAuthGrant, now time.Time) error {
	if err := tx.TouchGrant(ctx, grant.ID, now); err != nil {
		return err
	}
	return tx.TouchClient(ctx, grant.ClientID, now)
}

// ExchangeCode is the authorization_code grant. The code is consumed first, in
// one statement, so a code is used once whatever follows; presenting a used
// code again revokes its grant. Refusals answer invalid_grant.
func (s *OAuthService) ExchangeCode(ctx context.Context, req domain.OAuthCodeExchange, meta domain.OAuthCallMeta) (*domain.OAuthTokenResponse, error) {
	endpoint, err := s.endpoint()
	if err != nil {
		return nil, err
	}
	if req.Code == "" || req.ClientID == "" || req.RedirectURI == "" || req.CodeVerifier == "" {
		return nil, &domain.OAuthError{Code: domain.OAuthErrInvalidRequest, Description: "code, client_id, redirect_uri and code_verifier are required"}
	}
	now := s.now()
	codeHash := domain.HashOAuthSecret(req.Code)

	var response *domain.OAuthTokenResponse
	var refusal *domain.OAuthError
	var reused *domain.OAuthGrant
	err = s.repo.WithTx(ctx, func(tx domain.OAuthStore) error {
		code, err := tx.UseCode(ctx, codeHash, now)
		if errors.Is(err, domain.ErrOAuthNotFound) {
			existing, getErr := tx.GetCode(ctx, codeHash)
			switch {
			case errors.Is(getErr, domain.ErrOAuthNotFound):
				refusal = domain.NewOAuthInvalidGrant("unknown authorization code")
				return nil
			case getErr != nil:
				return getErr
			case existing.UsedAt != nil:
				// RFC 6749 §4.1.2: a code presented twice revokes what it granted.
				grant, err := tx.GetGrant(ctx, existing.GrantID)
				if err == nil {
					if revoked, err := tx.RevokeGrant(ctx, grant.ID, domain.OAuthRevokedCodeReuse, now); err != nil {
						return err
					} else if revoked {
						reused = grant
					}
				} else if !errors.Is(err, domain.ErrOAuthNotFound) {
					return err
				}
				refusal = domain.NewOAuthInvalidGrant("the authorization code was already used")
				return nil
			default:
				refusal = domain.NewOAuthInvalidGrant("the authorization code has expired")
				return nil
			}
		}
		if err != nil {
			return err
		}
		switch {
		case code.ClientID != req.ClientID:
			refusal = domain.NewOAuthInvalidGrant("the code was issued to another client")
		case code.RedirectURI != req.RedirectURI:
			refusal = domain.NewOAuthInvalidGrant("redirect_uri differs from the authorization request's")
		case !domain.VerifyCodeVerifier(req.CodeVerifier, code.CodeChallenge):
			refusal = domain.NewOAuthInvalidGrant("code_verifier does not match the code challenge")
		case code.Resource != endpoint.MCPURL() || !resourceMatches(req.Resource, code.Resource):
			refusal = domain.NewOAuthInvalidGrant("resource does not match the authorization's")
		}
		if refusal != nil {
			return nil
		}
		grant, err := tx.LockGrant(ctx, code.GrantID)
		if errors.Is(err, domain.ErrOAuthNotFound) {
			refusal = domain.NewOAuthInvalidGrant("the authorization was revoked")
			return nil
		}
		if err != nil {
			return err
		}
		if grant.Revoked() {
			refusal = domain.NewOAuthInvalidGrant("the authorization was revoked")
			return nil
		}
		if response, _, err = issuePair(ctx, tx, grant, now); err != nil {
			return err
		}
		return touchUse(ctx, tx, grant, now)
	})
	if err != nil {
		return nil, err
	}
	if reused != nil {
		s.recordReuseRevocation(ctx, reused, domain.OAuthRevokedCodeReuse, meta)
	}
	if refusal != nil {
		return nil, refusal
	}
	return response, nil
}

// Refresh is the refresh_token grant. Every refresh of one grant holds the
// grant's row lock, so a token and its successor are never rotated together.
// Rotation invalidates every live access token of the grant.
//
// The grace rule keeps a single chain: a replaced token presented again
// within OAuthRefreshGracePeriod of its rotation, while its successor has
// neither rotated nor been invalidated, invalidates the successor pair and
// gets a new pair; it works once per token, since rotated_at never moves and
// the successor is then invalidated. Any other replaced token, and any
// invalidated one, revokes the grant.
func (s *OAuthService) Refresh(ctx context.Context, req domain.OAuthRefresh, meta domain.OAuthCallMeta) (*domain.OAuthTokenResponse, error) {
	endpoint, err := s.endpoint()
	if err != nil {
		return nil, err
	}
	if req.RefreshToken == "" || req.ClientID == "" {
		return nil, &domain.OAuthError{Code: domain.OAuthErrInvalidRequest, Description: "refresh_token and client_id are required"}
	}
	now := s.now()
	hash := domain.HashOAuthSecret(req.RefreshToken)
	presented, err := s.repo.GetToken(ctx, hash)
	if errors.Is(err, domain.ErrOAuthNotFound) {
		return nil, domain.NewOAuthInvalidGrant("unknown refresh token")
	}
	if err != nil {
		return nil, err
	}

	var response *domain.OAuthTokenResponse
	var refusal *domain.OAuthError
	var reused *domain.OAuthGrant
	err = s.repo.WithTx(ctx, func(tx domain.OAuthStore) error {
		grant, err := tx.LockGrant(ctx, presented.GrantID)
		if errors.Is(err, domain.ErrOAuthNotFound) {
			refusal = domain.NewOAuthInvalidGrant("the authorization was revoked")
			return nil
		}
		if err != nil {
			return err
		}
		// Read again under the lock: a concurrent refresh may have rotated it.
		token, err := tx.GetToken(ctx, hash)
		if errors.Is(err, domain.ErrOAuthNotFound) {
			refusal = domain.NewOAuthInvalidGrant("unknown refresh token")
			return nil
		}
		if err != nil {
			return err
		}
		switch {
		case token.Kind != domain.OAuthTokenKindRefresh:
			refusal = domain.NewOAuthInvalidGrant("not a refresh token")
		case token.ClientID != req.ClientID:
			refusal = domain.NewOAuthInvalidGrant("the refresh token was issued to another client")
		case grant.Revoked():
			refusal = domain.NewOAuthInvalidGrant("the authorization was revoked")
		case grant.Resource != endpoint.MCPURL() || !resourceMatches(req.Resource, grant.Resource):
			// After an API_ENDPOINT change, so the client starts again rather
			// than loop.
			refusal = domain.NewOAuthInvalidGrant("the authorization is for another resource")
		}
		if refusal != nil {
			return nil
		}

		revokeForReuse := func(why string) error {
			revoked, err := tx.RevokeGrant(ctx, grant.ID, domain.OAuthRevokedTokenReuse, now)
			if err != nil {
				return err
			}
			if revoked {
				reused = grant
			}
			refusal = domain.NewOAuthInvalidGrant(why)
			return nil
		}
		if token.InvalidatedAt != nil {
			return revokeForReuse("the refresh token was invalidated")
		}
		if token.RotatedAt != nil {
			successor, err := tx.GetToken(ctx, token.ReplacedByHash)
			if err != nil && !errors.Is(err, domain.ErrOAuthNotFound) {
				return err
			}
			withinGrace := now.Before(token.RotatedAt.Add(domain.OAuthRefreshGracePeriod))
			if !withinGrace || successor == nil || successor.RotatedAt != nil || successor.InvalidatedAt != nil {
				return revokeForReuse("the refresh token was already used")
			}
			// Grace: the client lost the answer to its refresh. The pair it
			// never received goes, and a new one replaces it.
			if err := tx.InvalidatePair(ctx, grant.ID, successor.PairID, now); err != nil {
				return err
			}
			if err := tx.InvalidateLiveAccessTokens(ctx, grant.ID, now); err != nil {
				return err
			}
			if response, _, err = issuePair(ctx, tx, grant, now); err != nil {
				return err
			}
			return touchUse(ctx, tx, grant, now)
		}
		if !now.Before(token.ExpiresAt) {
			refusal = domain.NewOAuthInvalidGrant("the refresh token has expired")
			return nil
		}

		if err := tx.InvalidateLiveAccessTokens(ctx, grant.ID, now); err != nil {
			return err
		}
		var successorHash string
		if response, successorHash, err = issuePair(ctx, tx, grant, now); err != nil {
			return err
		}
		if err := tx.MarkTokenRotated(ctx, hash, successorHash, now); err != nil {
			return err
		}
		return touchUse(ctx, tx, grant, now)
	})
	if err != nil {
		return nil, err
	}
	if reused != nil {
		s.recordReuseRevocation(ctx, reused, domain.OAuthRevokedTokenReuse, meta)
	}
	if refusal != nil {
		return nil, refusal
	}
	return response, nil
}

// Revoke is POST /oauth/revoke (RFC 7009). Revoking an access token revokes
// that token; revoking a refresh token revokes the grant. An unknown token,
// or one issued to another client, changes nothing and is no error, so the
// endpoint tells nothing about tokens it does not own.
func (s *OAuthService) Revoke(ctx context.Context, req domain.OAuthTokenRevocation, meta domain.OAuthCallMeta) error {
	if req.ClientID == "" || req.Token == "" {
		return &domain.OAuthError{Code: domain.OAuthErrInvalidRequest, Description: "token and client_id are required"}
	}
	now := s.now()
	hash := domain.HashOAuthSecret(req.Token)
	token, err := s.repo.GetToken(ctx, hash)
	if errors.Is(err, domain.ErrOAuthNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if token.ClientID != req.ClientID {
		return nil
	}
	grant, err := s.repo.GetGrant(ctx, token.GrantID)
	if errors.Is(err, domain.ErrOAuthNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	changed := false
	switch token.Kind {
	case domain.OAuthTokenKindAccess:
		if token.InvalidatedAt == nil {
			if err := s.repo.InvalidateToken(ctx, hash, now); err != nil {
				return err
			}
			changed = true
		}
	case domain.OAuthTokenKindRefresh:
		if changed, err = s.repo.RevokeGrant(ctx, grant.ID, domain.OAuthRevokedByClient, now); err != nil {
			return err
		}
	}
	if changed {
		record := domain.NewAuditRecord(meta.IP, meta.UserAgent, meta.RequestID)
		record.SetAuthMethod(domain.AuditAuthOAuth)
		record.SetActorClaims(grant.APIKeyUserID, string(domain.UserTypeAPIKey))
		record.AddMetadata("token_kind", token.Kind)
		s.recordGrantEvent(ctx, record, domain.AuditActionOAuthRevoke, grant, "")
	}
	return nil
}

// ResolveAccessToken resolves the bearer of an /mcp request: a live access
// token (kind access, not invalidated, not expired), its grant (not revoked,
// bound to the current resource) and the grant's key, loaded as a header key
// is. Any refusal is domain.ErrOAuthInvalidToken; another error means the
// token could not be checked.
func (s *OAuthService) ResolveAccessToken(ctx context.Context, token string) (*domain.OAuthAccess, error) {
	endpoint, err := s.endpoint()
	if err != nil {
		return nil, domain.ErrOAuthInvalidToken
	}
	now := s.now()
	grant, err := s.repo.GetAccessTokenGrant(ctx, domain.HashOAuthSecret(token), now)
	if errors.Is(err, domain.ErrOAuthNotFound) {
		return nil, domain.ErrOAuthInvalidToken
	}
	if err != nil {
		return nil, err
	}
	if grant.Resource != endpoint.MCPURL() {
		return nil, domain.ErrOAuthInvalidToken
	}
	keyCtx := context.WithValue(ctx, domain.UserIDKey, grant.APIKeyUserID)
	keyCtx = context.WithValue(keyCtx, domain.UserTypeKey, string(domain.UserTypeAPIKey))
	key, err := s.auth.AuthenticateUserFromContext(keyCtx)
	switch {
	case errors.Is(err, domain.ErrAPIKeyRevoked), errors.Is(err, ErrUserNotFound):
		return nil, domain.ErrOAuthInvalidToken
	case err != nil:
		return nil, err
	case key == nil || key.Type != domain.UserTypeAPIKey:
		return nil, domain.ErrOAuthInvalidToken
	}
	if err := s.repo.TouchGrant(ctx, grant.ID, now); err != nil && s.logger != nil {
		s.logger.WithField("grant_id", grant.ID).WithField("error", err.Error()).Warn("Failed to record the last use of an OAuth grant")
	}
	return &domain.OAuthAccess{Grant: grant, Key: key}, nil
}

// ListWorkspaceGrants lists the connections of a workspace's keys, for its
// owners (root included).
func (s *OAuthService) ListWorkspaceGrants(ctx context.Context, workspaceID string) ([]*domain.OAuthGrantListItem, error) {
	if _, _, err := s.requireOwner(ctx, workspaceID); err != nil {
		return nil, err
	}
	return s.repo.ListWorkspaceGrants(ctx, workspaceID)
}

// RevokeGrant ends one connection without deleting its key, for an owner of
// the grant's workspace (root included).
func (s *OAuthService) RevokeGrant(ctx context.Context, grantID string, meta domain.OAuthCallMeta) error {
	if grantID == "" {
		return domain.NewValidationError("grant_id is required")
	}
	grant, err := s.repo.GetGrant(ctx, grantID)
	if errors.Is(err, domain.ErrOAuthNotFound) {
		return domain.ErrOAuthGrantNotFound
	}
	if err != nil {
		return err
	}
	record := domain.NewAuditRecord(meta.IP, meta.UserAgent, meta.RequestID)
	if _, _, err := s.requireOwner(domain.WithAuditRecord(ctx, record), grant.WorkspaceID); err != nil {
		var unauthorized *domain.ErrUnauthorized
		if errors.As(err, &unauthorized) && unauthorized.Message == oauthNotMemberMessage {
			return domain.ErrOAuthGrantNotFound
		}
		return err
	}
	revoked, err := s.repo.RevokeGrant(ctx, grant.ID, domain.OAuthRevokedByOwner, s.now())
	if err != nil {
		return err
	}
	if revoked {
		s.recordGrantEvent(ctx, record, domain.AuditActionOAuthRevokeGrant, grant, domain.OAuthRevokedByOwner)
	}
	return nil
}

// RunMaintenance is the periodic job: it revokes the grants with neither a
// valid refresh token nor a live code, purges expired requests, codes and
// tokens, then the client records the cache rules let go.
func (s *OAuthService) RunMaintenance(ctx context.Context) (*domain.OAuthPurgeResult, error) {
	now := s.now()
	result := &domain.OAuthPurgeResult{}
	var err error
	if result.GrantsRevoked, err = s.repo.RevokeGrantsWithoutCredential(ctx, now); err != nil {
		return result, err
	}
	if result.RequestsPurged, result.CodesPurged, result.TokensPurged, err = s.repo.PurgeExpired(ctx, now); err != nil {
		return result, err
	}
	if result.ClientsPurged, err = s.repo.PurgeClients(ctx, now); err != nil {
		return result, err
	}
	return result, nil
}

// recordReuseRevocation records a grant revoked because a code or a refresh
// token was presented again. The actor is the grant's key, through OAuth.
func (s *OAuthService) recordReuseRevocation(ctx context.Context, grant *domain.OAuthGrant, reason string, meta domain.OAuthCallMeta) {
	record := domain.NewAuditRecord(meta.IP, meta.UserAgent, meta.RequestID)
	record.SetAuthMethod(domain.AuditAuthOAuth)
	record.SetActorClaims(grant.APIKeyUserID, string(domain.UserTypeAPIKey))
	s.recordGrantEvent(ctx, record, domain.AuditActionOAuthRevokeGrant, grant, reason)
}

// recordGrantEvent writes one row of AuditSelfRecordedRoutes, in the
// workspace of the grant's key. Never a key, a code or a token.
func (s *OAuthService) recordGrantEvent(ctx context.Context, record *domain.AuditRecord, action string, grant *domain.OAuthGrant, reason string) {
	spec := domain.AuditSelfRecordedRoutes[action]
	record.SetWorkspace(grant.WorkspaceID)
	record.SetTarget(spec.TargetType, grant.ID, grant.ClientName)
	record.AddMetadata("oauth_client", grant.ClientName)
	record.AddMetadata("client_id", grant.ClientID)
	if reason != "" {
		record.AddMetadata("reason", reason)
	}
	event, skipped := record.Finalize(http.StatusOK)
	if skipped {
		return
	}
	event.Action = action
	event.Category = spec.Category
	s.recorder.Record(ctx, &event)
}
