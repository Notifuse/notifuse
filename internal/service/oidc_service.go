package service

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/cache"
	"github.com/Notifuse/notifuse/pkg/crypto"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/Notifuse/notifuse/pkg/ratelimiter"
	"github.com/Notifuse/notifuse/pkg/tracing"
)

const (
	// oidcExchangeTTL bounds the life of the one-time exchange code. Kept short
	// deliberately: together with single-use GetAndDelete and 256-bit entropy it, not
	// the per-IP rate limit, is what makes the code unguessable in practice.
	oidcExchangeTTL       = 60 * time.Second
	oidcExchangeKeyPrefix = "oidc:exchange:"

	// oidcInitRetryWindow bounds how often a failed provider init is re-attempted so
	// a transient issuer outage self-heals without a process restart.
	oidcInitRetryWindow = 30 * time.Second
)

// OIDCServiceConfig configures the OIDC service.
type OIDCServiceConfig struct {
	UserRepo              domain.UserRepository
	FederatedIdentityRepo domain.FederatedIdentityRepository
	AuthService           domain.AuthService
	OIDCConfig            config.OIDCConfig
	SessionExpiry         time.Duration
	RateLimiter           *ratelimiter.RateLimiter
	ExchangeCache         cache.Cache   // app wires an InMemoryCache
	HTTPTimeout           time.Duration // bounds discovery/JWKS/token-exchange HTTP calls (default 10s)
	SecretKey             string        // AEAD passphrase for sealing the flow-state cookie
	// IsRootEmail guards both provisioning paths against a ROOT_EMAIL account. It is
	// injected and nil-tolerant, so leaving it unset silently removes the guard — no
	// compile error, no failing unit test. The app wires the case-INSENSITIVE matcher
	// on purpose: an IdP that returns a differently-cased address must not slip past.
	IsRootEmail  func(string) bool
	IsProduction bool
	Logger       logger.Logger
	Tracer       tracing.Tracer
}

// OIDCService mints Notifuse sessions from a verified external OIDC identity. It is
// a second session-minter alongside magic-code login: after verifying the IdP ID
// token it creates the SAME session row + HS256 JWT the magic-code path produces.
type OIDCService struct {
	userRepo      domain.UserRepository
	fedRepo       domain.FederatedIdentityRepository
	authService   domain.AuthService
	cfg           config.OIDCConfig
	sessionExpiry time.Duration
	rateLimiter   *ratelimiter.RateLimiter
	exchangeCache cache.Cache
	httpTimeout   time.Duration
	secretKey     string
	isRootEmail   func(string) bool
	isProduction  bool
	logger        logger.Logger
	tracer        tracing.Tracer

	initMu      sync.Mutex // guards lazy provider init + self-healing retry
	lastAttempt time.Time  // timestamp of last init attempt (for the retry window)
	provider    *gooidc.Provider
	verifier    *gooidc.IDTokenVerifier
	oauthCfg    *oauth2.Config
	initErr     error
}

// NewOIDCService constructs an OIDCService, defaulting the tracer when nil.
func NewOIDCService(cfg OIDCServiceConfig) *OIDCService {
	tracer := cfg.Tracer
	if tracer == nil {
		tracer = tracing.GetTracer()
	}
	httpTimeout := cfg.HTTPTimeout
	if httpTimeout <= 0 {
		httpTimeout = 10 * time.Second
	}
	return &OIDCService{
		userRepo:      cfg.UserRepo,
		fedRepo:       cfg.FederatedIdentityRepo,
		authService:   cfg.AuthService,
		cfg:           cfg.OIDCConfig,
		sessionExpiry: cfg.SessionExpiry,
		rateLimiter:   cfg.RateLimiter,
		exchangeCache: cfg.ExchangeCache,
		httpTimeout:   httpTimeout,
		secretKey:     cfg.SecretKey,
		isRootEmail:   cfg.IsRootEmail,
		isProduction:  cfg.IsProduction,
		logger:        cfg.Logger,
		tracer:        tracer,
	}
}

var _ domain.OIDCServiceInterface = (*OIDCService)(nil)

// IsEnabled reports config.Enabled without touching the provider.
func (s *OIDCService) IsEnabled() bool { return s.cfg.Enabled }

// ensureProvider lazily builds (and self-heals) the provider/verifier/oauth2 config.
// Success is cached forever; a failed init is retried at most once per
// oidcInitRetryWindow so a transient issuer outage recovers without a restart.
func (s *OIDCService) ensureProvider(ctx context.Context) error {
	if !s.cfg.Enabled {
		return domain.ErrOIDCNotConfigured
	}
	s.initMu.Lock()
	defer s.initMu.Unlock()

	// Already initialized successfully.
	if s.provider != nil && s.verifier != nil && s.oauthCfg != nil {
		return nil
	}
	// Within the back-off window after a previous failure: short-circuit to 503.
	if !s.lastAttempt.IsZero() && time.Since(s.lastAttempt) < oidcInitRetryWindow {
		return domain.ErrOIDCNotConfigured
	}
	s.lastAttempt = time.Now()

	// Bound every OIDC HTTP call (discovery now, JWKS refreshes later) with a client
	// Timeout, so a slow/black-holed issuer can't hang forever under initMu. We use a
	// bounded client over a non-cancelable background context (NOT context.WithTimeout)
	// because go-oidc stores this client on the Provider and reuses it for the
	// self-refreshing RemoteKeySet — cancelling the context would break key rotation,
	// whereas the client Timeout safely bounds each individual request.
	httpClient := &http.Client{Timeout: s.httpTimeout}
	bg := gooidc.ClientContext(context.Background(), httpClient) // outlives any request; JWKS self-refreshes
	provider, err := gooidc.NewProvider(bg, s.cfg.IssuerURL)
	if err != nil {
		s.initErr = fmt.Errorf("oidc provider init (%s): %w", s.cfg.IssuerURL, err)
		if s.logger != nil {
			// Discovery is an unauthenticated GET, so unlike the token endpoint its
			// response cannot carry a secret and the text is worth keeping — a wrong
			// OIDC_ISSUER_URL is the most common misconfiguration and the status is how
			// you spot it. go-oidc does append the whole response body on a non-200,
			// though, so bound it: an issuer behind a proxy can answer with a full HTML
			// page, and that belongs nowhere near a log line.
			s.logger.WithField("issuer", s.cfg.IssuerURL).WithField("error", truncateForLog(err.Error(), 256)).
				Error("OIDC provider unreachable; routes will 503 and retry in ~30s (magic-code login unaffected)")
		}
		return domain.ErrOIDCNotConfigured
	}
	s.provider = provider
	s.oauthCfg = &oauth2.Config{
		ClientID:     s.cfg.ClientID,
		ClientSecret: s.cfg.ClientSecret,
		RedirectURL:  s.cfg.RedirectURI,
		Endpoint:     provider.Endpoint(),
		Scopes:       s.cfg.Scopes, // includes "openid"
	}
	// Pin asymmetric algs; leave all Skip*/Insecure* false. A symmetric alg would be
	// forgeable by anyone holding the client secret, and each Skip* switch drops a
	// check the rest of HandleCallback assumes has already passed. The key set is a
	// self-refreshing RemoteKeySet rather than a snapshot, so issuer key rotation
	// takes effect without a restart.
	s.verifier = provider.Verifier(&gooidc.Config{
		ClientID:             s.cfg.ClientID,
		SupportedSigningAlgs: []string{gooidc.RS256, gooidc.ES256},
	})
	s.initErr = nil
	return nil
}

// BuildAuthURL generates state+nonce+PKCE verifier and the IdP authorization URL.
func (s *OIDCService) BuildAuthURL(ctx context.Context) (*domain.OIDCAuthRequest, error) {
	ctx, span := s.tracer.StartServiceSpan(ctx, "OIDCService", "BuildAuthURL")
	defer span.End()

	if err := s.ensureProvider(ctx); err != nil {
		return nil, err
	}
	state, err := randToken(32)
	if err != nil {
		s.tracer.MarkSpanError(ctx, err)
		return nil, fmt.Errorf("oidc state gen: %w", err)
	}
	nonce, err := randToken(32)
	if err != nil {
		s.tracer.MarkSpanError(ctx, err)
		return nil, fmt.Errorf("oidc nonce gen: %w", err)
	}
	verifier := oauth2.GenerateVerifier()

	authURL := s.oauthCfg.AuthCodeURL(state,
		gooidc.Nonce(nonce),                  // adds &nonce=
		oauth2.S256ChallengeOption(verifier), // adds code_challenge + method=S256
		oauth2.AccessTypeOnline,              // no refresh token; login-only
	)
	return &domain.OIDCAuthRequest{
		AuthURL:   authURL,
		FlowState: domain.OIDCFlowState{State: state, Nonce: nonce, Verifier: verifier},
	}, nil
}

// HandleCallback runs all ID-token checks then the identity state machine, mints a
// session, stores the AuthResponse under a one-time code, and returns that code.
func (s *OIDCService) HandleCallback(ctx context.Context, in domain.OIDCCallbackInput) (string, error) {
	ctx, span := s.tracer.StartServiceSpan(ctx, "OIDCService", "HandleCallback")
	defer span.End()

	if err := s.ensureProvider(ctx); err != nil {
		return "", err
	}
	// (0) CSRF: cookie state must equal ?state=. The two emptiness tests are not
	// redundant with the compare below — ConstantTimeCompare("", "") returns 1, so
	// dropping them would let a callback carrying no state match a flow with none.
	if in.State == "" || in.FlowState.State == "" ||
		subtle.ConstantTimeCompare([]byte(in.State), []byte(in.FlowState.State)) != 1 {
		s.tracer.AddAttribute(ctx, "error", "state_mismatch")
		return "", fmt.Errorf("oidc state mismatch")
	}
	// RFC 9207 ?iss= defense-in-depth pre-exchange short-circuit (NOT the primary
	// control — the authoritative check is the post-Verify idToken.Issuer assertion).
	if in.Iss != "" && in.Iss != s.cfg.IssuerURL {
		if s.logger != nil {
			s.logger.WithField("got_iss", in.Iss).WithField("want_iss", s.cfg.IssuerURL).
				Warn("OIDC iss parameter mismatch (RFC 9207 pre-exchange short-circuit)")
		}
		s.tracer.AddAttribute(ctx, "error", "iss_mismatch")
		return "", fmt.Errorf("oidc issuer mismatch")
	}
	// Bound the network leg (token exchange + any JWKS fetch during Verify) so a hung
	// IdP token/JWKS endpoint can't stall the request indefinitely. DB work below uses
	// the original request ctx.
	netCtx, cancelNet := context.WithTimeout(ctx, s.httpTimeout)
	defer cancelNet()

	// Exchange code (PKCE verifier from flow-state). The error is reduced to metadata
	// before it goes anywhere — see sanitizeTokenExchangeError.
	oauth2Token, err := s.oauthCfg.Exchange(netCtx, in.Code, oauth2.VerifierOption(in.FlowState.Verifier))
	if err != nil {
		safeErr := sanitizeTokenExchangeError(err)
		s.tracer.MarkSpanError(ctx, safeErr)
		return "", safeErr
	}
	rawIDToken, ok := oauth2Token.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		s.tracer.AddAttribute(ctx, "error", "no_id_token")
		return "", fmt.Errorf("oidc response missing id_token")
	}
	// RFC 9068 confusion guard: reject typ at+jwt presented as an ID token.
	if err := rejectAccessTokenTyp(rawIDToken); err != nil {
		s.tracer.AddAttribute(ctx, "error", "id_token_typ_at_jwt")
		return "", err
	}
	// Cryptographic verification (RS256/ES256 pinned; iss/aud/exp/iat; Skip* all false).
	idToken, err := s.verifier.Verify(netCtx, rawIDToken)
	if err != nil {
		s.tracer.MarkSpanError(ctx, err)
		return "", fmt.Errorf("oidc id_token verify: %w", err)
	}
	// PRIMARY issuer control: UNCONDITIONALLY assert the signed token's issuer equals
	// our configured issuer (go-oidc pins it internally; we re-assert explicitly).
	if idToken.Issuer != s.cfg.IssuerURL {
		if s.logger != nil {
			s.logger.WithField("got_iss", idToken.Issuer).WithField("want_iss", s.cfg.IssuerURL).
				Error("OIDC id_token issuer mismatch (post-Verify assertion)")
		}
		s.tracer.AddAttribute(ctx, "error", "idtoken_iss_mismatch")
		return "", fmt.Errorf("oidc id_token issuer mismatch")
	}
	// Manual nonce equality — go-oidc Verify does NOT check nonce. As with the state
	// compare above, the empty test carries its own weight: a token with no nonce at
	// all would otherwise compare equal to an empty flow nonce.
	if idToken.Nonce == "" ||
		subtle.ConstantTimeCompare([]byte(idToken.Nonce), []byte(in.FlowState.Nonce)) != 1 {
		s.tracer.AddAttribute(ctx, "error", "nonce_mismatch")
		return "", fmt.Errorf("oidc nonce mismatch")
	}
	var claims struct {
		Sub           string        `json:"sub"`
		Email         string        `json:"email"`
		EmailVerified *flexibleBool `json:"email_verified"` // nil = claim absent; tolerates string "true"/"false"
		Name          string        `json:"name"`
		Azp           string        `json:"azp"`
		Aud           audience      `json:"aud"` // tolerates string OR []string
	}
	if err := idToken.Claims(&claims); err != nil {
		s.tracer.MarkSpanError(ctx, err)
		return "", fmt.Errorf("oidc claims decode: %w", err)
	}
	// Multi-audience: azp MUST equal our ClientID only when aud contains a genuine
	// SECOND DISTINCT audience. A single aud — or duplicates of our own ClientID —
	// never requires azp.
	if hasDistinctSecondAudience(claims.Aud, s.cfg.ClientID) && claims.Azp != s.cfg.ClientID {
		s.tracer.AddAttribute(ctx, "error", "azp_mismatch")
		return "", fmt.Errorf("oidc azp mismatch for multi-audience token")
	}
	issuer := idToken.Issuer // verified-trusted issuer from the signed token
	sub := claims.Sub
	email := strings.ToLower(strings.TrimSpace(claims.Email))
	s.tracer.AddAttribute(ctx, "oidc.sub", sub)
	s.tracer.AddAttribute(ctx, "oidc.issuer", issuer)

	user, err := s.resolveOrProvisionUser(ctx, issuer, sub, email, (*bool)(claims.EmailVerified), claims.Name)
	if err != nil {
		return "", err // typed errors flow to the HTTP layer for redirect mapping
	}
	authResp, err := s.mintSession(ctx, user)
	if err != nil {
		return "", err
	}
	oneTimeCode, err := randToken(32)
	if err != nil {
		return "", fmt.Errorf("oidc one-time code gen: %w", err)
	}
	s.exchangeCache.Set(oidcExchangeKeyPrefix+oneTimeCode, authResp, oidcExchangeTTL)
	if s.logger != nil {
		s.logger.WithField("user_id", user.ID).WithField("oidc_sub", sub).
			WithField("issuer", issuer).Info("OIDC login succeeded")
	}
	return oneTimeCode, nil
}

// flexibleBool decodes a JSON boolean claim that some IdPs (or claim-mapping
// bridges) emit as the STRING "true"/"false" instead of a bool. Without this,
// the whole claims decode fails and every login errors before the
// email_verified gate is reached. A JSON null sets the enclosing *flexibleBool
// field to nil without calling this method — same as an absent claim.
type flexibleBool bool

func (b *flexibleBool) UnmarshalJSON(data []byte) error {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	switch t := v.(type) {
	case bool:
		*b = flexibleBool(t)
		return nil
	case string:
		parsed, err := strconv.ParseBool(t)
		if err != nil {
			return fmt.Errorf("boolean claim: %q is not a boolean", t)
		}
		*b = flexibleBool(parsed)
		return nil
	default:
		return fmt.Errorf("boolean claim: unsupported JSON type %T", v)
	}
}

// resolveOrProvisionUser implements the identity/linking state machine.
func (s *OIDCService) resolveOrProvisionUser(
	ctx context.Context, issuer, sub, email string, emailVerified *bool, name string,
) (*domain.User, error) {

	// (1) Stable identity lookup by (issuer, sub). Email NOT used for auth here.
	fi, err := s.fedRepo.GetByIssuerSubject(ctx, issuer, sub)
	if err == nil {
		u, uerr := s.userRepo.GetUserByID(ctx, fi.UserID)
		if uerr != nil {
			return nil, fmt.Errorf("oidc linked user load: %w", uerr)
		}
		// The bridge below refuses to link an api_key identity in the first place, so
		// a link to one can only predate that guard. Re-check here rather than let a
		// stale row keep minting sessions.
		if u.Type == domain.UserTypeAPIKey {
			if s.logger != nil {
				s.logger.WithField("user_id", u.ID).WithField("oidc_sub", sub).WithField("issuer", issuer).
					Error("OIDC refused: linked identity belongs to an API key")
			}
			return nil, domain.ErrOIDCAccountNotProvisioned
		}
		return u, nil
	}
	var fiNotFound *domain.ErrFederatedIdentityNotFound
	if !errors.As(err, &fiNotFound) {
		return nil, fmt.Errorf("oidc federated lookup: %w", err)
	}

	// NOT FOUND -> first login. email_verified gate applies to every link/provision.
	// Explicit false ALWAYS rejects; an absent claim (nil) rejects unless
	// OIDC_ALLOW_UNVERIFIED_EMAIL=true, because some IdPs (Microsoft Entra ID,
	// Cloudflare Access) never emit the claim at all.
	switch {
	case emailVerified != nil && !*emailVerified:
		if s.logger != nil {
			s.logger.WithField("oidc_sub", sub).WithField("issuer", issuer).
				Warn("OIDC first-login rejected: email_verified=false")
		}
		return nil, domain.ErrOIDCEmailNotVerified
	case emailVerified == nil && !s.cfg.AllowUnverifiedEmail:
		if s.logger != nil {
			s.logger.WithField("oidc_sub", sub).WithField("issuer", issuer).
				Warn("OIDC login rejected: email_verified claim absent from id_token. " +
					"If your IdP (e.g. Entra ID, Cloudflare Access) never emits this claim, " +
					"set OIDC_ALLOW_UNVERIFIED_EMAIL=true")
		}
		return nil, domain.ErrOIDCEmailNotVerified
	}
	if email == "" {
		return nil, domain.ErrOIDCEmailNotVerified
	}

	// (2) Existing Notifuse user by email (invited-user bridge) — CASE-INSENSITIVE.
	existing, uerr := s.userRepo.GetUserByEmailInsensitive(ctx, email)
	if uerr == nil {
		// API-KEY guard: an api_key is a users row with a real email, so an IdP
		// asserting that address would otherwise bridge a machine credential onto a
		// human session. An API key authenticates with its own long-lived token and
		// has no business holding a session, so the bridge refuses before it links.
		// The generic not-provisioned error keeps the response from singling the
		// address out.
		if existing.Type == domain.UserTypeAPIKey {
			if s.logger != nil {
				s.logger.WithField("user_id", existing.ID).WithField("oidc_sub", sub).WithField("issuer", issuer).
					Error("OIDC refused: email belongs to an API key identity")
			}
			return nil, domain.ErrOIDCAccountNotProvisioned
		}
		linked, lerr := s.fedRepo.GetByUserAndIssuer(ctx, existing.ID, issuer)
		if lerr == nil {
			// Deterministic by UNIQUE(user_id, idp_issuer): at most one row.
			if linked.IDPSub != sub {
				if s.logger != nil {
					s.logger.WithField("user_id", existing.ID).WithField("issuer", issuer).
						WithField("existing_sub", linked.IDPSub).WithField("incoming_sub", sub).
						Error("OIDC identity conflict: email maps to a user with a different sub for this issuer")
				}
				return nil, domain.ErrOIDCIdentityConflict
			}
			return existing, nil // same sub, row appeared via race
		}
		var linkNotFound *domain.ErrFederatedIdentityNotFound
		if !errors.As(lerr, &linkNotFound) {
			return nil, fmt.Errorf("oidc user-issuer lookup: %w", lerr)
		}
		// ROOT_EMAIL guard (bridge path): NEVER auto-link a ROOT_EMAIL account to an
		// external identity on first login — a configured ROOT_EMAIL is synthesized
		// owner of EVERY workspace (auth_service.go), so binding an attacker-controlled
		// IdP subject to it is full platform-admin takeover. This must hold even with
		// JIT disabled (the most locked-down config). An already-linked root identity
		// is handled by the (issuer,sub) branch above and still re-logs-in, so an
		// operator who intentionally provisioned root SSO is unaffected. The match is
		// on the stored account email via a case-insensitive matcher (app.go wires
		// config.IsRootEmailInsensitive).
		if s.isRootEmail != nil && s.isRootEmail(existing.Email) {
			if s.logger != nil {
				s.logger.WithField("email", existing.Email).WithField("oidc_sub", sub).WithField("issuer", issuer).
					Error("OIDC refused: first-time link to a ROOT_EMAIL account is forbidden (privilege escalation)")
			}
			return nil, domain.ErrOIDCAccountNotProvisioned
		}
		// LINK: bridge invited user to this identity. This is the one place email
		// decides anything, and it carries an accepted residual risk: an account that
		// was invited but never signed in is matched on email alone, so an address
		// deprovisioned and later reassigned at the same IdP would bridge the new
		// holder onto the stale invitation. Three things bound it — email_verified is
		// mandatory before any link, the bridge runs at most once per account (every
		// later login keys off (issuer, sub) above), and the conflict branch refuses a
		// second sub outright. Closing it fully means dropping the email bridge and
		// requiring an explicit link action from an already-authenticated session,
		// which costs every invited user a second sign-in method to get started.
		if cerr := s.linkIdentity(ctx, existing.ID, issuer, sub); cerr != nil {
			return nil, cerr
		}
		if s.logger != nil {
			s.logger.WithField("user_id", existing.ID).WithField("oidc_sub", sub).
				Info("OIDC linked existing (invited) user to federated identity")
		}
		return existing, nil
	}
	var userNotFound *domain.ErrUserNotFound
	if !errors.As(uerr, &userNotFound) {
		return nil, fmt.Errorf("oidc user-by-email lookup: %w", uerr)
	}

	// Provisioning policy.
	if !s.cfg.AutoCreateUsers {
		if s.logger != nil {
			s.logger.WithField("email", email).WithField("oidc_sub", sub).
				Warn("OIDC login rejected: no Notifuse account (invited-only)")
		}
		return nil, domain.ErrOIDCAccountNotProvisioned
	}
	// ROOT_EMAIL guard: NEVER JIT-create a user whose email matches a configured
	// ROOT_EMAIL — it is synthesized owner of ALL workspaces, so auto-minting it
	// would be privilege escalation. Force the invite path.
	if s.isRootEmail != nil && s.isRootEmail(email) {
		if s.logger != nil {
			s.logger.WithField("email", email).WithField("oidc_sub", sub).
				Error("OIDC JIT refused: email matches ROOT_EMAIL (would be privilege escalation); invite path required")
		}
		return nil, domain.ErrOIDCAccountNotProvisioned
	}
	if len(s.cfg.AllowedDomains) == 0 { // defense in depth (boot fails otherwise)
		return nil, domain.ErrOIDCAccountNotProvisioned
	}
	if !domainAllowed(email, s.cfg.AllowedDomains) {
		if s.logger != nil {
			s.logger.WithField("email", email).Warn("OIDC JIT rejected: domain not in allowlist")
		}
		return nil, domain.ErrOIDCDomainNotAllowed
	}

	// JIT create: users row ONLY (no workspace membership — login != access).
	newUser := &domain.User{
		ID:       generateID(),
		Email:    email,
		Name:     name,
		Type:     domain.UserTypeUser,
		Language: domain.DefaultLanguageCode,
	}
	if cerr := s.userRepo.CreateUser(ctx, newUser); cerr != nil {
		var exists *domain.ErrUserExists
		// Recovering onto the winner's row rather than failing is only safe because
		// the email was lower-cased upstream and users.email is a case-SENSITIVE
		// UNIQUE: the row this re-fetch finds is necessarily the one that just lost
		// the race, not some pre-existing mixed-case account.
		if errors.As(cerr, &exists) { // race: another login created it
			if u, gerr := s.userRepo.GetUserByEmailInsensitive(ctx, email); gerr == nil {
				newUser = u
			} else {
				return nil, fmt.Errorf("oidc jit re-fetch: %w", gerr)
			}
		} else {
			return nil, fmt.Errorf("oidc jit create: %w", cerr)
		}
	}
	if lerr := s.linkIdentity(ctx, newUser.ID, issuer, sub); lerr != nil {
		return nil, lerr
	}
	if s.logger != nil {
		s.logger.WithField("user_id", newUser.ID).WithField("oidc_sub", sub).
			Info("OIDC JIT-provisioned new user")
	}
	return newUser, nil
}

// linkIdentity inserts (user_id, issuer, sub). A duplicate-key on EITHER unique
// constraint is REFUSED as an identity conflict unless a re-read proves it is the
// exact same link landing via a race (idempotent success).
//
// The two constraints fail for different reasons and both must refuse: a clash on
// (idp_issuer, idp_sub) means a DIFFERENT user already holds that subject, and one
// on (user_id, idp_issuer) means this user already has a different subject at this
// issuer. Swallowing either as success is what would turn a recycled email into an
// account takeover, so the refusal is the control, not an edge case.
func (s *OIDCService) linkIdentity(ctx context.Context, userID, issuer, sub string) error {
	err := s.fedRepo.Create(ctx, &domain.FederatedIdentity{
		UserID: userID, IDPIssuer: issuer, IDPSub: sub,
	})
	if err == nil {
		return nil
	}
	var exists *domain.ErrFederatedIdentityExists
	if errors.As(err, &exists) {
		if cur, gerr := s.fedRepo.GetByUserAndIssuer(ctx, userID, issuer); gerr == nil &&
			cur.IDPSub == sub && cur.UserID == userID {
			return nil // exact same link landed first via race — idempotent success
		}
		if s.logger != nil {
			s.logger.WithField("user_id", userID).WithField("issuer", issuer).
				WithField("incoming_sub", sub).
				Error("OIDC link create hit a unique-constraint conflict; refusing")
		}
		return domain.ErrOIDCIdentityConflict
	}
	return fmt.Errorf("oidc link create: %w", err)
}

// mintSession creates a session row (no magic code) and an HS256 JWT — identical to
// the RootSignin minting path.
func (s *OIDCService) mintSession(ctx context.Context, user *domain.User) (*domain.AuthResponse, error) {
	expiresAt := time.Now().Add(s.sessionExpiry)
	session := &domain.Session{
		ID:        generateID(),
		UserID:    user.ID,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now(),
		// MagicCode / MagicCodeExpires left nil — identical to RootSignin.
	}
	if err := s.userRepo.CreateSession(ctx, session); err != nil {
		s.tracer.MarkSpanError(ctx, err)
		return nil, fmt.Errorf("oidc create session: %w", err)
	}
	token := s.authService.GenerateUserAuthToken(user, session.ID, expiresAt)
	if token == "" {
		return nil, fmt.Errorf("oidc token generation failed")
	}
	return &domain.AuthResponse{Token: token, User: *user, ExpiresAt: expiresAt}, nil
}

// ExchangeCode atomically consumes the one-time code and returns the AuthResponse.
func (s *OIDCService) ExchangeCode(ctx context.Context, oneTimeCode string) (*domain.AuthResponse, error) {
	ctx, span := s.tracer.StartServiceSpan(ctx, "OIDCService", "ExchangeCode")
	defer span.End()

	if !s.cfg.Enabled {
		return nil, domain.ErrOIDCNotConfigured
	}
	if oneTimeCode == "" {
		return nil, fmt.Errorf("oidc exchange: empty code")
	}
	v, ok := s.exchangeCache.GetAndDelete(oidcExchangeKeyPrefix + oneTimeCode)
	if !ok {
		s.tracer.AddAttribute(ctx, "error", "code_not_found_or_used")
		return nil, fmt.Errorf("oidc exchange: invalid or expired code")
	}
	authResp, ok := v.(*domain.AuthResponse)
	if !ok {
		return nil, fmt.Errorf("oidc exchange: corrupt cache entry")
	}
	return authResp, nil
}

// SealFlowState AEAD-encrypts the per-login flow state into an opaque hex blob for
// the __Host- flow cookie. The PKCE verifier inside is a secret, so this uses
// authenticated encryption (not HMAC-signing alone). The AES key (SecretKey) stays
// in the service layer; the handler only ever sees ciphertext.
func (s *OIDCService) SealFlowState(fs domain.OIDCFlowState) (string, error) {
	b, err := json.Marshal(fs)
	if err != nil {
		return "", fmt.Errorf("oidc seal flow-state marshal: %w", err)
	}
	enc, err := crypto.EncryptString(string(b), s.secretKey)
	if err != nil {
		return "", fmt.Errorf("oidc seal flow-state encrypt: %w", err)
	}
	return enc, nil
}

// OpenFlowState decrypts and parses a sealed flow-state blob. A tampered or garbage
// blob returns an error (never a silent zero value).
func (s *OIDCService) OpenFlowState(enc string) (domain.OIDCFlowState, error) {
	var fs domain.OIDCFlowState
	if enc == "" {
		return fs, fmt.Errorf("oidc open flow-state: empty")
	}
	dec, err := crypto.DecryptFromHexString(enc, s.secretKey)
	if err != nil {
		return fs, fmt.Errorf("oidc open flow-state decrypt: %w", err)
	}
	if err := json.Unmarshal([]byte(dec), &fs); err != nil {
		return fs, fmt.Errorf("oidc open flow-state parse: %w", err)
	}
	return fs, nil
}

// --- local helpers ----------------------------------------------------------

// audience tolerates the OIDC `aud` claim being a bare string OR a JSON array.
type audience []string

func (a *audience) UnmarshalJSON(b []byte) error {
	var single string
	if err := json.Unmarshal(b, &single); err == nil {
		*a = audience{single}
		return nil
	}
	var many []string
	if err := json.Unmarshal(b, &many); err != nil {
		return err
	}
	*a = many
	return nil
}

// randToken returns n random bytes base64url-encoded (no padding).
func randToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// domainAllowed reports whether email's domain is in the allowlist (case-insensitive).
func domainAllowed(email string, allowed []string) bool {
	at := strings.LastIndex(email, "@")
	if at < 0 {
		return false
	}
	dom := strings.ToLower(email[at+1:])
	for _, a := range allowed {
		if strings.ToLower(strings.TrimSpace(a)) == dom {
			return true
		}
	}
	return false
}

// hasDistinctSecondAudience reports whether aud contains any entry that is NOT our
// ClientID — a genuine second distinct audience. Duplicates of our own ClientID and
// a lone ClientID return false, so azp is required exactly when it is meaningful.
func hasDistinctSecondAudience(aud audience, clientID string) bool {
	for _, a := range aud {
		if a != clientID {
			return true
		}
	}
	return false
}

// truncateForLog caps a remote-supplied string at max runes, marking any cut so a
// reader can tell the difference between a short message and a trimmed one.
func truncateForLog(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…(truncated)"
}

// sanitizeTokenExchangeError reduces a token-endpoint failure to metadata that is
// safe to hand to a log sink.
//
// oauth2.RetrieveError.Error() prints the endpoint's raw response body verbatim
// whenever the response is not a well-formed OAuth error object. Wrapping it with %w
// would put an IdP payload nobody has inspected into every sink that renders the
// error chain, so the original is deliberately NOT wrapped: nothing downstream can
// walk back to the body. What survives is the HTTP status and the RFC 6749 error
// code, which is what actually tells an operator what went wrong. error_description
// is dropped on purpose — it is free text by design; read it at the IdP.
func sanitizeTokenExchangeError(err error) error {
	var re *oauth2.RetrieveError
	if !errors.As(err, &re) {
		// Transport-level failure (dial, TLS, timeout): the text is Go's own and
		// carries no endpoint payload, so it is kept as-is for diagnosis.
		return fmt.Errorf("oidc code exchange: %w", err)
	}
	status := "no response"
	if re.Response != nil {
		status = re.Response.Status
	}
	if code := safeOAuthErrorCode(re.ErrorCode); code != "" {
		return fmt.Errorf("oidc code exchange rejected by token endpoint (%s, error=%s)", status, code)
	}
	return fmt.Errorf("oidc code exchange rejected by token endpoint (%s)", status)
}

// safeOAuthErrorCode passes through an error code that looks like the bare token RFC
// 6749 defines and rejects anything else. The field is IdP-controlled, and the spec
// permits nearly every printable character in it, so a code that is not a plain token
// is treated as unusable rather than copied into a log. Extension codes real
// providers emit still pass; a JSON fragment or a prose sentence does not.
func safeOAuthErrorCode(code string) string {
	if code == "" {
		return ""
	}
	if len(code) > 64 {
		return "unrecognized"
	}
	for _, r := range code {
		isTokenChar := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.'
		if !isTokenChar {
			return "unrecognized"
		}
	}
	return code
}

// rejectAccessTokenTyp rejects a JWT whose header typ is an access-token type
// (RFC 9068 "at+jwt" / "application/at+jwt") presented as an ID token.
func rejectAccessTokenTyp(raw string) error {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return fmt.Errorf("oidc id_token malformed")
	}
	hb, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return fmt.Errorf("oidc id_token header decode: %w", err)
	}
	var hdr struct {
		Typ string `json:"typ"`
	}
	if err := json.Unmarshal(hb, &hdr); err != nil {
		return fmt.Errorf("oidc id_token header parse: %w", err)
	}
	if strings.EqualFold(hdr.Typ, "at+jwt") || strings.EqualFold(hdr.Typ, "application/at+jwt") {
		return fmt.Errorf("oidc rejected: id_token has access-token typ %q", hdr.Typ)
	}
	return nil
}
