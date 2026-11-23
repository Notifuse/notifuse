package domain

//go:generate mockgen -destination mocks/mock_oauth_repository.go -package mocks github.com/Notifuse/notifuse/internal/domain OAuthRepository
//go:generate mockgen -destination mocks/mock_oauth_service.go -package mocks github.com/Notifuse/notifuse/internal/domain OAuthService

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// The OAuth authorization server of the MCP endpoint. A client (claude.ai,
// ChatGPT, Claude Code) is identified by its Client ID Metadata Document, the
// authorization page takes a Notifuse API key, and the tokens it issues lead,
// through a grant, to that key. Notifuse keeps the key's account ID, never the
// key itself.

// Lifetimes and limits.
const (
	// OAuthAccessTokenLifetime is how long an access token is valid.
	OAuthAccessTokenLifetime = time.Hour
	// OAuthRefreshTokenLifetime is how long a refresh token is valid without
	// use: each rotation issues a new one with a fresh lifetime.
	OAuthRefreshTokenLifetime = 90 * 24 * time.Hour
	// OAuthCodeLifetime is how long an authorization code is valid.
	OAuthCodeLifetime = 60 * time.Second
	// OAuthRequestLifetime is how long an authorization request, and the
	// cookie binding it to the browser, are valid.
	OAuthRequestLifetime = 10 * time.Minute
	// OAuthRefreshGracePeriod is how long after its rotation a replaced
	// refresh token may be presented again, once, when its successor is
	// untouched (a client that lost the response).
	OAuthRefreshGracePeriod = 60 * time.Second
	// OAuthLastUsedInterval bounds how often last_used_at is written.
	OAuthLastUsedInterval = 5 * time.Minute

	// OAuthClientIDMaxLength bounds a client_id URL.
	OAuthClientIDMaxLength = 2048
	// OAuthClientNameMaxLength bounds client_name, in characters.
	OAuthClientNameMaxLength = 200
	// OAuthMaxRedirectURIs bounds the redirect_uris of a client.
	OAuthMaxRedirectURIs = 20
	// OAuthRedirectURIMaxLength bounds each redirect URI.
	OAuthRedirectURIMaxLength = 2048

	// OAuthClientFetchTimeout bounds a Client ID Metadata Document fetch.
	OAuthClientFetchTimeout = 5 * time.Second
	// OAuthClientMaxBodyBytes caps the body of a metadata document (64 KB).
	OAuthClientMaxBodyBytes = 64 * 1024
	// OAuthClientMinCache and OAuthClientMaxCache bound how long a fetched
	// document is cached, whatever its cache headers say.
	OAuthClientMinCache = 5 * time.Minute
	OAuthClientMaxCache = 24 * time.Hour
	// OAuthClientFailureCache is how long a failed fetch is remembered.
	OAuthClientFailureCache = 5 * time.Minute
	// OAuthGrantlessClientCap caps the client records that never led to a
	// grant. The known clients are outside the cap.
	OAuthGrantlessClientCap = 10000
	// OAuthGrantlessClientRetention is how long a fetched record that never
	// led to a grant is kept.
	OAuthGrantlessClientRetention = 24 * time.Hour
	// OAuthRevokedClientRetention is how long a record whose grants are all
	// revoked is kept after its last use.
	OAuthRevokedClientRetention = 365 * 24 * time.Hour

	// OAuthAccessTokenPrefix and OAuthRefreshTokenPrefix start every token, so
	// that a token found in a log or a paste says what it is.
	OAuthAccessTokenPrefix  = "nfmcp_at_"
	OAuthRefreshTokenPrefix = "nfmcp_rt_"
	// OAuthScope is the one scope granted, whatever the client asks for.
	OAuthScope = "mcp"
)

// OAuthKnownClientHosts are the client_id hosts the authorization page does
// not warn about, and whose fetched records are outside the cap. Lowercase.
var OAuthKnownClientHosts = []string{"claude.ai", "chatgpt.com"}

// IsKnownOAuthClient reports whether a client_id is on a known host: it
// starts with "https://", a known host and "/", ASCII letters compared without
// case. Only ASCII is folded, so no other character can pass for a letter of
// a known host. CountGrantlessClients applies the same rule in SQL, so the
// service and the count agree on which records are outside the cap.
func IsKnownOAuthClient(clientID string) bool {
	for _, host := range OAuthKnownClientHosts {
		prefix := "https://" + host + "/"
		if len(clientID) >= len(prefix) && asciiLower(clientID[:len(prefix)]) == prefix {
			return true
		}
	}
	return false
}

// asciiLower lowercases the ASCII letters of s and nothing else.
func asciiLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if 'A' <= c && c <= 'Z' {
			b[i] = c + ('a' - 'A')
		}
	}
	return string(b)
}

// Token kinds, grant origins and revocation reasons.
const (
	OAuthTokenKindAccess  = "access"
	OAuthTokenKindRefresh = "refresh"

	// OAuthGrantViaKey: a key was pasted on the authorization page.
	OAuthGrantViaKey = "key"
	// OAuthGrantViaOwnerShortcut: a signed-in owner created the key there.
	OAuthGrantViaOwnerShortcut = "owner_shortcut"

	// OAuthRevokedByOwner: an owner revoked the connection.
	OAuthRevokedByOwner = "owner"
	// OAuthRevokedTokenReuse: a replaced or invalidated refresh token was
	// presented again.
	OAuthRevokedTokenReuse = "token_reuse"
	// OAuthRevokedCodeReuse: an authorization code was presented twice.
	OAuthRevokedCodeReuse = "code_reuse"
	// OAuthRevokedByClient: the client revoked its refresh token (RFC 7009).
	OAuthRevokedByClient = "client_revocation"
	// OAuthRevokedNoCredential: the periodic job found neither a valid
	// refresh token nor a live code.
	OAuthRevokedNoCredential = "no_valid_credential"
)

// OAuthClient is a cached Client ID Metadata Document. A record whose fetch
// failed has FetchFailedUntil set and may have no metadata at all.
type OAuthClient struct {
	ClientID         string
	ClientName       string
	RedirectURIs     []string
	Metadata         json.RawMessage
	CreatedAt        time.Time
	LastUsedAt       *time.Time
	FetchedAt        *time.Time
	CacheExpiresAt   *time.Time
	FetchFailedUntil *time.Time
}

// OAuthClientSnapshot is what an authorization request keeps of its client:
// the token endpoint never fetches the document again.
type OAuthClientSnapshot struct {
	ClientID     string   `json:"client_id"`
	ClientName   string   `json:"client_name"`
	RedirectURIs []string `json:"redirect_uris"`
}

// OAuthAuthorizationRequest is a validated GET /oauth/authorize, waiting for
// the authorization page. BrowserBindingHash is the SHA-256 of the cookie that
// binds it to the browser that started it.
type OAuthAuthorizationRequest struct {
	ID                 string
	ClientID           string
	ClientSnapshot     OAuthClientSnapshot
	RedirectURI        string
	State              string
	CodeChallenge      string
	Resource           string
	BrowserBindingHash string
	CreatedAt          time.Time
	ConsumedAt         *time.Time
	ExpiresAt          time.Time
}

// OAuthAuthorizationCode is a single-use code, stored as a SHA-256 hash.
type OAuthAuthorizationCode struct {
	CodeHash      string
	GrantID       string
	ClientID      string
	RedirectURI   string
	CodeChallenge string
	Resource      string
	ExpiresAt     time.Time
	UsedAt        *time.Time
}

// OAuthGrant is one connection: a client allowed to act as an API key.
type OAuthGrant struct {
	ID              string     `json:"id"`
	APIKeyUserID    string     `json:"api_key_user_id"`
	WorkspaceID     string     `json:"workspace_id"`
	ClientID        string     `json:"client_id"`
	ClientName      string     `json:"client_name"`
	Resource        string     `json:"resource"`
	CreatedVia      string     `json:"created_via"`
	CreatedByUserID string     `json:"created_by_user_id,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	LastUsedAt      *time.Time `json:"last_used_at,omitempty"`
	RevokedAt       *time.Time `json:"revoked_at,omitempty"`
	RevokedReason   string     `json:"revoked_reason,omitempty"`
}

// Revoked reports whether the grant was revoked.
func (g OAuthGrant) Revoked() bool {
	return g.RevokedAt != nil
}

// OAuthGrantListItem is a grant as the MCP tab lists it, with its key's name.
type OAuthGrantListItem struct {
	OAuthGrant
	APIKeyEmail string `json:"api_key_email"`
	Status      string `json:"status"`
}

// Grant statuses in OAuthGrantListItem.
const (
	OAuthGrantStatusActive  = "active"
	OAuthGrantStatusRevoked = "revoked"
)

// OAuthToken is an access or refresh token, stored as a SHA-256 hash. The
// access and refresh tokens issued together share a PairID.
type OAuthToken struct {
	TokenHash      string
	GrantID        string
	PairID         string
	ClientID       string
	Kind           string
	ExpiresAt      time.Time
	RotatedAt      *time.Time
	ReplacedByHash string
	InvalidatedAt  *time.Time
}

// OAuthPurgeResult counts what one run of the periodic job did.
type OAuthPurgeResult struct {
	GrantsRevoked  int64
	RequestsPurged int64
	CodesPurged    int64
	TokensPurged   int64
	ClientsPurged  int64
}

// ErrOAuthNotFound is a missing request, code, token, grant or client.
var ErrOAuthNotFound = errors.New("oauth record not found")

// OAuthStore is the system-database storage of the authorization server.
// Every time is passed in, so that the service decides what "now" is.
type OAuthStore interface {
	GetClient(ctx context.Context, clientID string) (*OAuthClient, error)
	// SaveClient stores a fetched document and clears a failure.
	SaveClient(ctx context.Context, client *OAuthClient) error
	// MarkClientFetchFailed remembers a failed fetch until the given time,
	// creating the record when there is none.
	MarkClientFetchFailed(ctx context.Context, clientID string, until, now time.Time) error
	// CountGrantlessClients counts the records no grant references, except
	// the fetched records of the given lowercase hosts, matched as
	// IsKnownOAuthClient matches them. A failed fetch always counts.
	CountGrantlessClients(ctx context.Context, exemptHosts []string) (int, error)
	// TouchClient writes last_used_at at most once per OAuthLastUsedInterval.
	TouchClient(ctx context.Context, clientID string, now time.Time) error

	CreateRequest(ctx context.Context, request *OAuthAuthorizationRequest) error
	GetRequest(ctx context.Context, id string) (*OAuthAuthorizationRequest, error)
	// ConsumeRequest marks an unconsumed, unexpired request consumed in one
	// statement and returns it; ErrOAuthNotFound when no row qualifies.
	ConsumeRequest(ctx context.Context, id string, now time.Time) (*OAuthAuthorizationRequest, error)

	CreateGrant(ctx context.Context, grant *OAuthGrant) error
	GetGrant(ctx context.Context, id string) (*OAuthGrant, error)
	// LockGrant reads the grant FOR UPDATE: inside a transaction it
	// serializes every refresh of one grant.
	LockGrant(ctx context.Context, id string) (*OAuthGrant, error)
	// RevokeGrant revokes a grant that is not revoked yet; it reports whether
	// it did.
	RevokeGrant(ctx context.Context, id, reason string, now time.Time) (bool, error)
	// TouchGrant writes last_used_at at most once per OAuthLastUsedInterval.
	TouchGrant(ctx context.Context, id string, now time.Time) error
	ListWorkspaceGrants(ctx context.Context, workspaceID string) ([]*OAuthGrantListItem, error)

	CreateCode(ctx context.Context, code *OAuthAuthorizationCode) error
	GetCode(ctx context.Context, codeHash string) (*OAuthAuthorizationCode, error)
	// UseCode marks an unused, unexpired code used in one statement and
	// returns it; ErrOAuthNotFound when no row qualifies.
	UseCode(ctx context.Context, codeHash string, now time.Time) (*OAuthAuthorizationCode, error)

	CreateTokens(ctx context.Context, tokens ...*OAuthToken) error
	GetToken(ctx context.Context, tokenHash string) (*OAuthToken, error)
	// GetAccessTokenGrant returns the grant of a live access token: kind
	// access, not invalidated, not expired, grant not revoked.
	GetAccessTokenGrant(ctx context.Context, tokenHash string, now time.Time) (*OAuthGrant, error)
	MarkTokenRotated(ctx context.Context, tokenHash, replacedByHash string, now time.Time) error
	InvalidateToken(ctx context.Context, tokenHash string, now time.Time) error
	// InvalidatePair invalidates the tokens of one pair that are not yet.
	InvalidatePair(ctx context.Context, grantID, pairID string, now time.Time) error
	// InvalidateLiveAccessTokens invalidates every unexpired access token of
	// the grant that is not yet.
	InvalidateLiveAccessTokens(ctx context.Context, grantID string, now time.Time) error

	// CreateAPIKeyAccount writes the users row and the membership of a new API
	// key, for a key created together with its grant.
	CreateAPIKeyAccount(ctx context.Context, key *User, membership *UserWorkspace) error

	// RevokeGrantsWithoutCredential revokes every grant with neither a valid
	// refresh token nor a live code.
	RevokeGrantsWithoutCredential(ctx context.Context, now time.Time) (int64, error)
	// PurgeExpired deletes expired requests, codes and tokens.
	PurgeExpired(ctx context.Context, now time.Time) (requests, codes, tokens int64, err error)
	// PurgeClients deletes the client records the cache rules let go.
	PurgeClients(ctx context.Context, now time.Time) (int64, error)
}

// OAuthRepository is the OAuthStore with transactions.
type OAuthRepository interface {
	OAuthStore
	// WithTx runs fn in one transaction; an error from fn rolls it back.
	WithTx(ctx context.Context, fn func(tx OAuthStore) error) error
}

// OAuthCallMeta is what the HTTP layer knows of a call, for the audit rows
// the service writes itself.
type OAuthCallMeta struct {
	IP        string
	UserAgent string
	RequestID string
}

// OAuthAuthorizeParams are the parameters of GET /oauth/authorize. An empty
// Resource means the client sent none.
type OAuthAuthorizeParams struct {
	ResponseType        string
	ClientID            string
	RedirectURI         string
	State               string
	CodeChallenge       string
	CodeChallengeMethod string
	Resource            string
}

// OAuthAuthorizationStarted is a stored request: the HTTP layer redirects to
// the authorization page with RequestID and sets BrowserBinding as the
// request cookie, which is never stored in clear.
type OAuthAuthorizationStarted struct {
	RequestID      string
	BrowserBinding string
	ExpiresAt      time.Time
}

// OAuthAuthorizationRequestView is what the authorization page shows.
type OAuthAuthorizationRequestView struct {
	RequestID        string                `json:"request_id"`
	ClientID         string                `json:"client_id"`
	ClientName       string                `json:"client_name"`
	ClientHost       string                `json:"client_host"`
	KnownClient      bool                  `json:"known_client"`
	RedirectHost     string                `json:"redirect_host"`
	LoopbackRedirect bool                  `json:"loopback_redirect"`
	ExpiresAt        time.Time             `json:"expires_at"`
	OwnedWorkspaces  []OAuthOwnedWorkspace `json:"owned_workspaces,omitempty"`
}

// OAuthOwnedWorkspace is a workspace the signed-in caller owns, for the
// owner shortcut. A workspace whose MCP switch is off is listed, not
// selectable.
type OAuthOwnedWorkspace struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	MCPDisabled bool   `json:"mcp_disabled"`
}

// OAuthApproveWithKeyRequest is oauth.approveWithKey. BrowserBinding is the
// request cookie, read by the HTTP layer.
type OAuthApproveWithKeyRequest struct {
	RequestID      string `json:"request_id"`
	APIKey         string `json:"api_key"`
	BrowserBinding string `json:"-"`
}

// OAuthCreateKeyAndApproveRequest is oauth.createKeyAndApprove. Name becomes
// the key's email prefix; nil Permissions means full access, as for
// workspaces.createAPIKey.
type OAuthCreateKeyAndApproveRequest struct {
	RequestID      string          `json:"request_id"`
	WorkspaceID    string          `json:"workspace_id"`
	Name           string          `json:"name"`
	Permissions    UserPermissions `json:"permissions,omitempty"`
	BrowserBinding string          `json:"-"`
}

// OAuthDenyRequest is oauth.deny.
type OAuthDenyRequest struct {
	RequestID      string `json:"request_id"`
	BrowserBinding string `json:"-"`
}

// OAuthRedirect is where the authorization page sends the browser.
type OAuthRedirect struct {
	RedirectURL string `json:"redirect_url"`
}

// OAuthCodeExchange is the authorization_code grant. An empty Resource means
// the client sent none.
type OAuthCodeExchange struct {
	Code         string
	ClientID     string
	RedirectURI  string
	CodeVerifier string
	Resource     string
}

// OAuthRefresh is the refresh_token grant.
type OAuthRefresh struct {
	RefreshToken string
	ClientID     string
	Resource     string
}

// OAuthTokenRevocation is POST /oauth/revoke.
type OAuthTokenRevocation struct {
	Token    string
	ClientID string
}

// OAuthTokenResponse is a successful token endpoint answer.
type OAuthTokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
}

// OAuthAccess is what an access token resolves to on /mcp.
type OAuthAccess struct {
	Grant *OAuthGrant
	Key   *User
}

// OAuthService is the authorization server without HTTP.
type OAuthService interface {
	// Unavailable says why the authorization server is off, or "".
	Unavailable() string
	Issuer() (string, error)
	CreateAuthorizationRequest(ctx context.Context, params OAuthAuthorizeParams) (*OAuthAuthorizationStarted, error)
	GetAuthorizationRequest(ctx context.Context, requestID string) (*OAuthAuthorizationRequestView, error)
	ApproveWithKey(ctx context.Context, req OAuthApproveWithKeyRequest, meta OAuthCallMeta) (*OAuthRedirect, error)
	CreateKeyAndApprove(ctx context.Context, req OAuthCreateKeyAndApproveRequest, meta OAuthCallMeta) (*OAuthRedirect, error)
	Deny(ctx context.Context, req OAuthDenyRequest) (*OAuthRedirect, error)
	ExchangeCode(ctx context.Context, req OAuthCodeExchange, meta OAuthCallMeta) (*OAuthTokenResponse, error)
	Refresh(ctx context.Context, req OAuthRefresh, meta OAuthCallMeta) (*OAuthTokenResponse, error)
	Revoke(ctx context.Context, req OAuthTokenRevocation, meta OAuthCallMeta) error
	ResolveAccessToken(ctx context.Context, token string) (*OAuthAccess, error)
	ListWorkspaceGrants(ctx context.Context, workspaceID string) ([]*OAuthGrantListItem, error)
	RevokeGrant(ctx context.Context, grantID string, meta OAuthCallMeta) error
	RunMaintenance(ctx context.Context) (*OAuthPurgeResult, error)
}

// Token endpoint error codes (RFC 6749 §5.2).
const (
	OAuthErrInvalidRequest       = "invalid_request"
	OAuthErrInvalidClient        = "invalid_client"
	OAuthErrInvalidGrant         = "invalid_grant"
	OAuthErrUnsupportedGrantType = "unsupported_grant_type"
)

// OAuthError is an error of the token endpoint, answered as RFC 6749 §5.2
// says.
type OAuthError struct {
	Code        string
	Description string
}

func (e *OAuthError) Error() string {
	if e.Description == "" {
		return e.Code
	}
	return e.Code + ": " + e.Description
}

// NewOAuthInvalidGrant is the answer to a code, refresh token or grant that
// cannot be used.
func NewOAuthInvalidGrant(description string) *OAuthError {
	return &OAuthError{Code: OAuthErrInvalidGrant, Description: description}
}

// OAuthAuthorizeError refuses GET /oauth/authorize. It is rendered as an
// error page and never redirects.
type OAuthAuthorizeError struct {
	Reason  string
	Message string
}

func (e *OAuthAuthorizeError) Error() string {
	return e.Message
}

// Reasons of OAuthAuthorizeError.
const (
	OAuthAuthorizeUnsupportedResponseType = "unsupported_response_type"
	// OAuthAuthorizeNotCIMD: the client_id is not an https URL, so the
	// application does not support Client ID Metadata Documents.
	OAuthAuthorizeNotCIMD         = "client_id_not_metadata_document"
	OAuthAuthorizeInvalidClientID = "invalid_client_id"
	OAuthAuthorizeClientMetadata  = "client_metadata_unavailable"
	OAuthAuthorizeClientCap       = "too_many_clients"
	OAuthAuthorizeInvalidRedirect = "invalid_redirect_uri"
	OAuthAuthorizeInvalidPKCE     = "invalid_code_challenge"
	OAuthAuthorizeInvalidResource = "invalid_resource"
)

// Errors of the console calls.
var (
	// ErrOAuthUnavailable: the authorization server is off (§5.1 of the spec).
	ErrOAuthUnavailable = errors.New("the OAuth authorization server is unavailable on this server")
	// ErrOAuthRequestUnavailable: expired, consumed or unknown request.
	ErrOAuthRequestUnavailable = errors.New("this authorization request has expired or was already used: start again from your application")
	// ErrOAuthBrowserMismatch: the request cookie is missing or different.
	ErrOAuthBrowserMismatch = errors.New("this authorization request was started in another browser: start again from your application")
	// ErrOAuthNotAnAPIKey: what was pasted is not a live API key.
	ErrOAuthNotAnAPIKey = errors.New("this is not a Notifuse API key: create one in Settings, Team & API keys, and paste it here")
	// ErrOAuthKeyNameTaken: the owner shortcut's key name is already the name
	// of an API key, here or in another workspace: key addresses are unique
	// across the deployment.
	ErrOAuthKeyNameTaken = errors.New("an API key already uses this name: pick another name")
	// ErrOAuthWorkspaceMCPDisabled: the key's workspace has MCP switched off.
	ErrOAuthWorkspaceMCPDisabled = errors.New("AI assistants (MCP) are switched off for this workspace")
	// ErrOAuthInvalidToken: an access token /mcp cannot accept: unknown, not an
	// access token, invalidated, expired, its grant revoked or bound to another
	// resource, or its key deleted.
	ErrOAuthInvalidToken = errors.New("invalid OAuth access token")
	// ErrOAuthGrantNotFound: no such connection, or not one the caller may see.
	ErrOAuthGrantNotFound = errors.New("connection not found")
)

// ErrNotAnAPIKey is returned when a bearer string is not an API key token of
// this installation: malformed, signed elsewhere, a console session or an
// invitation.
var ErrNotAnAPIKey = errors.New("not an API key")

// ValidateOAuthClientID checks a client_id as a Client ID Metadata Document
// URL: https, a path other than "/", no fragment, no user info, no "." or
// ".." segment, at most OAuthClientIDMaxLength characters. A client_id that is
// not an https URL at all gets OAuthAuthorizeNotCIMD, so the page can say the
// application must support metadata documents.
func ValidateOAuthClientID(clientID string) error {
	parsed, err := url.Parse(clientID)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return &OAuthAuthorizeError{Reason: OAuthAuthorizeNotCIMD, Message: "this application does not identify itself with a Client ID Metadata Document, which Notifuse requires: connect it with an API key instead"}
	}
	invalid := func(why string) error {
		return &OAuthAuthorizeError{Reason: OAuthAuthorizeInvalidClientID, Message: "invalid client_id: " + why}
	}
	switch {
	case len(clientID) > OAuthClientIDMaxLength:
		return invalid(fmt.Sprintf("longer than %d characters", OAuthClientIDMaxLength))
	case parsed.Fragment != "" || strings.Contains(clientID, "#"):
		return invalid("it has a fragment")
	case parsed.User != nil:
		return invalid("it has user info")
	case parsed.Path == "" || parsed.Path == "/":
		return invalid("it has no path")
	}
	for _, segment := range strings.Split(parsed.EscapedPath(), "/") {
		if segment == "." || segment == ".." || strings.EqualFold(segment, "%2e") || strings.EqualFold(segment, "%2e%2e") || strings.EqualFold(segment, ".%2e") || strings.EqualFold(segment, "%2e.") {
			return invalid("it has a dot segment")
		}
	}
	return nil
}

// OAuthClientMetadata is a Client ID Metadata Document, as far as this server
// reads it. The authentication method fields are raw so that "absent" and
// "present but empty" stay apart.
type OAuthClientMetadata struct {
	ClientID                          string          `json:"client_id"`
	ClientName                        string          `json:"client_name"`
	RedirectURIs                      []string        `json:"redirect_uris"`
	TokenEndpointAuthMethod           json.RawMessage `json:"token_endpoint_auth_method,omitempty"`
	TokenEndpointAuthMethodsSupported json.RawMessage `json:"token_endpoint_auth_methods_supported,omitempty"`
}

// ParseOAuthClientMetadata decodes and validates a fetched document for the
// client_id it was fetched from.
func ParseOAuthClientMetadata(clientID string, body []byte) (*OAuthClientMetadata, error) {
	var metadata OAuthClientMetadata
	if err := json.Unmarshal(body, &metadata); err != nil {
		return nil, fmt.Errorf("the client metadata document is not a JSON object: %w", err)
	}
	if err := metadata.Validate(clientID); err != nil {
		return nil, err
	}
	return &metadata, nil
}

// Validate applies the rules of a Client ID Metadata Document.
func (m *OAuthClientMetadata) Validate(clientID string) error {
	if m.ClientID != clientID {
		return fmt.Errorf("the document's client_id does not equal the URL it was fetched from")
	}
	if strings.TrimSpace(m.ClientName) == "" {
		return fmt.Errorf("the document has no client_name")
	}
	if len([]rune(m.ClientName)) > OAuthClientNameMaxLength {
		return fmt.Errorf("client_name is longer than %d characters", OAuthClientNameMaxLength)
	}
	if len(m.RedirectURIs) == 0 {
		return fmt.Errorf("the document has no redirect_uris")
	}
	if len(m.RedirectURIs) > OAuthMaxRedirectURIs {
		return fmt.Errorf("the document has more than %d redirect_uris", OAuthMaxRedirectURIs)
	}
	for _, uri := range m.RedirectURIs {
		if len(uri) > OAuthRedirectURIMaxLength {
			return fmt.Errorf("a redirect URI is longer than %d characters", OAuthRedirectURIMaxLength)
		}
		if strings.Contains(uri, "#") {
			return fmt.Errorf("a redirect URI has a fragment")
		}
	}
	return m.validateAuthMethod()
}

// validateAuthMethod accepts the document when this server's only method,
// none, is one the client can use. The plural field wins when present, so
// ChatGPT's document (plural none and private_key_jwt, singular
// private_key_jwt) is accepted; the singular alone must be none (Claude
// Code's document); neither means none.
func (m *OAuthClientMetadata) validateAuthMethod() error {
	if len(m.TokenEndpointAuthMethodsSupported) > 0 && string(m.TokenEndpointAuthMethodsSupported) != "null" {
		var methods []string
		if err := json.Unmarshal(m.TokenEndpointAuthMethodsSupported, &methods); err != nil {
			return fmt.Errorf("token_endpoint_auth_methods_supported is not a list of strings")
		}
		for _, method := range methods {
			if method == "none" {
				return nil
			}
		}
		return fmt.Errorf("the client cannot authenticate with none, the only method this server supports")
	}
	if len(m.TokenEndpointAuthMethod) > 0 && string(m.TokenEndpointAuthMethod) != "null" {
		var method string
		if err := json.Unmarshal(m.TokenEndpointAuthMethod, &method); err != nil || method != "none" {
			return fmt.Errorf("the client cannot authenticate with none, the only method this server supports")
		}
	}
	return nil
}

// ValidateRedirectURISyntax refuses a redirect URI this server never sends a
// browser to: one with a fragment, a scheme other than https, or http on a
// host that is not loopback.
func ValidateRedirectURISyntax(redirectURI string) error {
	if redirectURI == "" {
		return fmt.Errorf("redirect_uri is required")
	}
	if len(redirectURI) > OAuthRedirectURIMaxLength {
		return fmt.Errorf("redirect_uri is longer than %d characters", OAuthRedirectURIMaxLength)
	}
	if strings.Contains(redirectURI, "#") {
		return fmt.Errorf("redirect_uri has a fragment")
	}
	parsed, err := url.Parse(redirectURI)
	if err != nil || parsed.Host == "" {
		return fmt.Errorf("redirect_uri is not an absolute URL")
	}
	switch parsed.Scheme {
	case "https":
		return nil
	case "http":
		if IsLoopbackHostname(parsed.Hostname()) {
			return nil
		}
		return fmt.Errorf("redirect_uri uses http on a host that is not loopback")
	default:
		return fmt.Errorf("redirect_uri must use https, or http on a loopback host")
	}
}

// IsLoopbackRedirectURI reports whether a redirect URI is http on a loopback
// host, for the authorization page's warning.
func IsLoopbackRedirectURI(redirectURI string) bool {
	parsed, err := url.Parse(redirectURI)
	return err == nil && parsed.Scheme == "http" && IsLoopbackHostname(parsed.Hostname())
}

// MatchRedirectURI reports whether a requested redirect URI matches one of
// the client's. An https URI must equal a registered one byte for byte. An
// http URI on 127.0.0.1, [::1] or localhost matches a registered one with the
// same scheme, host, path and query on any port (RFC 8252 §7.3). Anything else
// is refused.
func MatchRedirectURI(requested string, registered []string) bool {
	if ValidateRedirectURISyntax(requested) != nil {
		return false
	}
	parsedRequested, _ := url.Parse(requested)
	loopback := parsedRequested.Scheme == "http"
	for _, candidate := range registered {
		if !loopback {
			if candidate == requested {
				return true
			}
			continue
		}
		parsedCandidate, err := url.Parse(candidate)
		if err != nil || parsedCandidate.Scheme != "http" || strings.Contains(candidate, "#") {
			continue
		}
		if withoutPort(parsedCandidate) == withoutPort(parsedRequested) {
			return true
		}
	}
	return false
}

// withoutPort writes a URL with its port removed, its host as written.
func withoutPort(u *url.URL) string {
	copied := *u
	host := u.Hostname()
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	copied.Host = host
	return copied.String()
}

// pkceUnreserved reports whether s is 43 to 128 characters of the RFC 7636
// unreserved set: ALPHA, DIGIT, "-", ".", "_", "~".
func pkceUnreserved(s string) bool {
	if len(s) < 43 || len(s) > 128 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '-', c == '.', c == '_', c == '~':
		default:
			return false
		}
	}
	return true
}

// ValidateCodeChallenge requires S256 and the code_challenge syntax of
// RFC 7636 §4.2. A missing method, or plain, is refused.
func ValidateCodeChallenge(challenge, method string) error {
	if method != "S256" {
		return fmt.Errorf("code_challenge_method must be S256")
	}
	if !pkceUnreserved(challenge) {
		return fmt.Errorf("code_challenge must be 43 to 128 characters of A-Z, a-z, 0-9, '-', '.', '_' and '~'")
	}
	return nil
}

// VerifyCodeVerifier checks a code_verifier against its S256 challenge, with
// the verifier syntax of RFC 7636 §4.1.
func VerifyCodeVerifier(verifier, challenge string) bool {
	if !pkceUnreserved(verifier) {
		return false
	}
	computed := PKCEChallengeS256(verifier)
	return subtle.ConstantTimeCompare([]byte(computed), []byte(challenge)) == 1
}

// PKCEChallengeS256 is the S256 challenge of a verifier: BASE64URL(SHA256).
func PKCEChallengeS256(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// HashOAuthSecret is the stored form of a code, a token or a browser binding:
// the hex SHA-256 of the string.
func HashOAuthSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return fmt.Sprintf("%x", sum[:])
}
