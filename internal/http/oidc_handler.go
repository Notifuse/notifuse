package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/Notifuse/notifuse/pkg/ratelimiter"
	"github.com/Notifuse/notifuse/pkg/tracing"
)

// OIDCHandler owns the three public OIDC routes, the AEAD-encrypted flow-state
// cookie, and the one-time-code fragment handoff. It is standalone (not bolted onto
// UserHandler) because it depends on the OIDC service, config, the rate limiter, and
// owns cookie semantics the JSON-only UserHandler does not.
type OIDCHandler struct {
	service     domain.OIDCServiceInterface
	config      *config.Config
	rateLimiter *ratelimiter.RateLimiter
	logger      logger.Logger
	tracer      tracing.Tracer
	isSecure    bool // HTTPS? controls Secure attr / __Host- prefix viability in dev
}

// NewOIDCHandler constructs the OIDC HTTP handler.
func NewOIDCHandler(
	service domain.OIDCServiceInterface,
	cfg *config.Config,
	rateLimiter *ratelimiter.RateLimiter,
	log logger.Logger,
) *OIDCHandler {
	return &OIDCHandler{
		service:     service,
		config:      cfg,
		rateLimiter: rateLimiter,
		logger:      log,
		tracer:      tracing.GetTracer(),
		// Decided from the CONFIGURED endpoint, not the request: behind a TLS-terminating
		// proxy r.TLS is nil on every request, so asking the request would silently drop
		// Secure (and the __Host- prefix with it) on exactly the deployments that need it.
		isSecure: strings.HasPrefix(cfg.APIEndpoint, "https://"),
	}
}

// RegisterRoutes registers the three public OIDC routes (no requireAuth).
func (h *OIDCHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/user.oidc.start", h.Start)       // GET  -> 302 to IdP
	mux.HandleFunc("/api/user.oidc.callback", h.Callback) // GET  <- IdP redirect
	mux.HandleFunc("/api/user.oidc.exchange", h.Exchange) // POST <- SPA, code in body
}

// flowCookieName returns the __Host--prefixed name only over HTTPS; dev HTTP drops
// the prefix (which mandates Secure) so the cookie still works on localhost. The
// prefix is what binds the flow cookie to this exact origin — a browser refuses to
// accept a __Host- cookie carrying a Domain, so no sibling subdomain can plant one.
func (h *OIDCHandler) flowCookieName() string {
	if h.isSecure {
		return "__Host-oidc_flow"
	}
	return "oidc_flow"
}

// signinBase is the fixed internal console sign-in URL the callback redirects to.
func (h *OIDCHandler) signinBase() string {
	return strings.TrimRight(h.config.APIEndpoint, "/") + "/console/signin"
}

// redirectSigninError 302s to the sign-in page with a fixed enumerated error reason.
// The redirect target is NEVER derived from IdP/user input (open-redirect defense).
func (h *OIDCHandler) redirectSigninError(w http.ResponseWriter, r *http.Request, reason string) {
	http.Redirect(w, r, h.signinBase()+"?oidc_error="+url.QueryEscape(reason), http.StatusFound)
}

func (h *OIDCHandler) allow(namespace, ip string) bool {
	return h.rateLimiter == nil || h.rateLimiter.Allow(namespace, ip)
}

// Start begins the SSO flow: build the IdP auth URL, seal the flow-state into the
// __Host- cookie, and 302 the browser to the IdP.
func (h *OIDCHandler) Start(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.redirectSigninError(w, r, "auth_failed")
		return
	}
	if !h.allow("oidc:start", getClientIP(r)) {
		h.redirectSigninError(w, r, "rate_limited")
		return
	}
	if !h.service.IsEnabled() {
		WriteJSONError(w, "OIDC is not available", http.StatusServiceUnavailable)
		return
	}
	req, err := h.service.BuildAuthURL(r.Context())
	if err != nil {
		h.warn("OIDC start: BuildAuthURL failed", err)
		// Enabled-but-provider-not-ready (unreachable/misconfigured issuer, lazy init
		// not yet succeeded) → tell the user SSO is temporarily unavailable and to use
		// a magic code, rather than a generic "failed". Other errors stay auth_failed.
		reason := "auth_failed"
		if errors.Is(err, domain.ErrOIDCNotConfigured) {
			reason = "provider_unavailable"
		}
		h.redirectSigninError(w, r, reason)
		return
	}
	sealed, err := h.service.SealFlowState(req.FlowState)
	if err != nil {
		h.warn("OIDC start: seal flow-state failed", err)
		h.redirectSigninError(w, r, "auth_failed")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     h.flowCookieName(),
		Value:    sealed, // AES-GCM ciphertext, opaque to the browser
		Path:     "/",    // __Host- requires Path=/ and no Domain
		MaxAge:   300,    // ~5 min: must outlive the IdP round-trip
		Secure:   h.isSecure,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode, // Lax: survives the top-level IdP -> callback redirect
	})
	http.Redirect(w, r, req.AuthURL, http.StatusFound)
}

// Callback handles the IdP redirect: validate state, exchange + verify the ID token,
// run the identity state machine, and 302 to the sign-in page with the one-time code
// in the URL fragment. Never 500, never echoes a raw IdP error, never a secret in a URL.
func (h *OIDCHandler) Callback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.redirectSigninError(w, r, "auth_failed")
		return
	}
	if !h.allow("oidc:callback", getClientIP(r)) {
		h.redirectSigninError(w, r, "rate_limited")
		return
	}
	if !h.service.IsEnabled() {
		h.warn("OIDC callback while OIDC not initialized", nil)
		h.redirectSigninError(w, r, "auth_failed")
		return
	}

	// Read and IMMEDIATELY clear the flow cookie (single-use, regardless of outcome).
	cookie, cerr := r.Cookie(h.flowCookieName())
	h.clearFlowCookie(w)
	if cerr != nil || cookie.Value == "" {
		// No flow cookie in this browser → cannot be a flow we started (login-CSRF guard).
		h.redirectSigninError(w, r, "auth_failed")
		return
	}

	q := r.URL.Query()
	if idpErr := q.Get("error"); idpErr != "" {
		h.logger.WithField("idp_error", idpErr).
			WithField("idp_error_description", q.Get("error_description")).
			Warn("OIDC callback: IdP returned an error")
		h.redirectSigninError(w, r, "auth_failed")
		return
	}

	flowState, ferr := h.service.OpenFlowState(cookie.Value)
	if ferr != nil {
		h.warn("OIDC callback: flow-state decrypt failed", ferr)
		h.redirectSigninError(w, r, "auth_failed")
		return
	}

	oneTimeCode, err := h.service.HandleCallback(r.Context(), domain.OIDCCallbackInput{
		Code:      q.Get("code"),
		State:     q.Get("state"),
		Iss:       q.Get("iss"),
		FlowState: flowState,
	})
	if err != nil {
		h.redirectSigninError(w, r, mapCallbackError(err))
		if !isExpectedCallbackError(err) {
			h.warn("OIDC callback failed", err)
		}
		return
	}

	// Success: one-time code in the URL FRAGMENT (never the query string / logs / Referer).
	http.Redirect(w, r, h.signinBase()+"#oidc_code="+url.PathEscape(oneTimeCode), http.StatusFound)
}

// Exchange trades the one-time code (in the POST body) for the AuthResponse JSON,
// byte-identical to VerifyCode. No cookie is involved → works cross-origin.
//
// Handing the code back in an HttpOnly __Host- cookie instead looks stronger and was
// the earlier design, but it only holds when the console and the API share an origin.
// Split across origins it fails twice over: a SameSite=Lax cookie is not sent on a
// cross-site POST, and the wildcard CORS origin this API serves cannot be combined
// with credentialed requests. Hence the fragment: it never reaches a server log or a
// Referer header, and the code it carries is single-use and short-lived.
func (h *OIDCHandler) Exchange(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !h.allow("oidc:exchange", getClientIP(r)) {
		w.Header().Set("Retry-After", strconv.Itoa(60))
		WriteJSONError(w, "Too many requests", http.StatusTooManyRequests)
		return
	}
	var body struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Code == "" {
		WriteJSONError(w, "No pending sign-in", http.StatusUnauthorized)
		return
	}
	resp, err := h.service.ExchangeCode(r.Context(), body.Code)
	if err != nil {
		WriteJSONError(w, "Invalid or expired sign-in", http.StatusUnauthorized)
		return
	}
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *OIDCHandler) clearFlowCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     h.flowCookieName(),
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Secure:   h.isSecure,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func (h *OIDCHandler) warn(msg string, err error) {
	if h.logger == nil {
		return
	}
	l := h.logger
	if err != nil {
		l = l.WithField("error", err.Error())
	}
	l.Warn(msg)
}

// mapCallbackError maps a service error to a fixed enumerated SPA error reason.
func mapCallbackError(err error) string {
	switch {
	case errors.Is(err, domain.ErrOIDCAccountNotProvisioned):
		return "not_provisioned"
	case errors.Is(err, domain.ErrOIDCIdentityConflict):
		return "link_conflict"
	case errors.Is(err, domain.ErrOIDCEmailNotVerified):
		return "email_unverified"
	case errors.Is(err, domain.ErrOIDCNotConfigured):
		return "provider_unavailable"
	default:
		return "auth_failed"
	}
}

// isExpectedCallbackError reports whether the error is a normal policy outcome
// (logged at most at info) rather than an unexpected failure worth a warn log.
func isExpectedCallbackError(err error) bool {
	return errors.Is(err, domain.ErrOIDCAccountNotProvisioned) ||
		errors.Is(err, domain.ErrOIDCIdentityConflict) ||
		errors.Is(err, domain.ErrOIDCEmailNotVerified)
}
