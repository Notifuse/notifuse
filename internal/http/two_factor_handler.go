package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/http/middleware"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/Notifuse/notifuse/pkg/ratelimiter"
)

// TwoFactorVerifyPolicy is the rate-limiter namespace of user.twoFactor.verify, by client
// address. It only damps noise: the address comes from X-Forwarded-For, which a client
// chooses. The counters stored on the account are the protection.
const TwoFactorVerifyPolicy = "2fa:verify"

// TwoFactorServiceInterface is what the two-factor routes and user.me need.
type TwoFactorServiceInterface interface {
	Status(ctx context.Context, userID string) (*service.TwoFactorStatus, error)
	Setup(ctx context.Context, userID, sessionID string, proof service.TwoFactorProof, meta service.TwoFactorRequestMeta) (*service.TwoFactorSetup, error)
	Enable(ctx context.Context, userID, sessionID, code string, signOutOthers bool, meta service.TwoFactorRequestMeta) ([]string, error)
	Disable(ctx context.Context, userID string, proof service.TwoFactorProof, meta service.TwoFactorRequestMeta) error
	RegenerateRecoveryCodes(ctx context.Context, userID string, proof service.TwoFactorProof, meta service.TwoFactorRequestMeta) ([]string, error)
	VerifyChallenge(ctx context.Context, challengeToken string, proof service.TwoFactorProof, meta service.TwoFactorRequestMeta) (*domain.AuthResponse, error)
	ResetForUser(ctx context.Context, rootUserID, rootSessionID string, rootProof service.TwoFactorProof, targetUserID, targetEmail string, meta service.TwoFactorRequestMeta) error
	UsersWithoutTwoFactor(ctx context.Context) ([]string, error)
	RequireTwoFactor() bool
}

// TwoFactorHandler serves the two-factor authentication routes. None answers 401 to a
// wrong code or a dead challenge (writeTwoFactorError); a session that is really gone does.
type TwoFactorHandler struct {
	twoFactor    TwoFactorServiceInterface
	authService  domain.AuthService
	rateLimiter  *ratelimiter.RateLimiter
	getJWTSecret func() ([]byte, error)
	isDemo       bool
	logger       logger.Logger
}

// NewTwoFactorHandler builds the handler of the two-factor routes.
func NewTwoFactorHandler(twoFactor TwoFactorServiceInterface, authService domain.AuthService, rateLimiter *ratelimiter.RateLimiter, getJWTSecret func() ([]byte, error), isDemo bool, logger logger.Logger) *TwoFactorHandler {
	return &TwoFactorHandler{
		twoFactor:    twoFactor,
		authService:  authService,
		rateLimiter:  rateLimiter,
		getJWTSecret: getJWTSecret,
		isDemo:       isDemo,
		logger:       logger,
	}
}

// RegisterRoutes registers the routes. Every one is closed on a demo instance, whose
// accounts are shared.
func (h *TwoFactorHandler) RegisterRoutes(mux *http.ServeMux) {
	requireAuth := middleware.NewAuthMiddleware(h.getJWTSecret).RequireAuth()
	restricted := middleware.RestrictedInDemo(h.isDemo)

	mux.Handle("/api/user.twoFactor.verify", restricted(http.HandlerFunc(h.Verify)))
	mux.Handle("/api/user.twoFactor.setup", requireAuth(restricted(http.HandlerFunc(h.Setup))))
	mux.Handle("/api/user.twoFactor.enable", requireAuth(restricted(http.HandlerFunc(h.Enable))))
	mux.Handle("/api/user.twoFactor.disable", requireAuth(restricted(http.HandlerFunc(h.Disable))))
	mux.Handle("/api/user.twoFactor.regenerateRecoveryCodes", requireAuth(restricted(http.HandlerFunc(h.RegenerateRecoveryCodes))))
	mux.Handle("/api/users.resetTwoFactor", requireAuth(restricted(http.HandlerFunc(h.ResetForUser))))
}

// proofRequest is the second factor a request carries.
type proofRequest struct {
	Code         string `json:"code"`
	RecoveryCode string `json:"recovery_code"`
}

func (p proofRequest) proof() service.TwoFactorProof {
	return service.TwoFactorProof{Code: p.Code, RecoveryCode: p.RecoveryCode}
}

func requestMeta(r *http.Request) service.TwoFactorRequestMeta {
	return service.TwoFactorRequestMeta{IP: middleware.ClientIP(r), UserAgent: r.UserAgent()}
}

// noStore marks a response that carries a secret, a token or recovery codes.
func noStore(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
}

// decodePost refuses anything but a POST with a JSON body.
func decodePost(w http.ResponseWriter, r *http.Request, v interface{}) bool {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return false
	}
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return false
	}
	return true
}

// consoleSession returns the user and session of a console session. An API key is a valid
// credential without access to these routes: 403, never 401. compliant also applies the
// instance's two-factor requirement; the routes that let a session turn it on pass false.
func (h *TwoFactorHandler) consoleSession(w http.ResponseWriter, r *http.Request, compliant bool) (userID, sessionID string, ok bool) {
	ctx := r.Context()
	if !isConsoleSession(ctx) {
		WriteJSONError(w, "Two-factor authentication belongs to people, not API keys", http.StatusForbidden)
		return "", "", false
	}
	var user *domain.User
	var err error
	if compliant {
		user, err = h.authService.AuthenticateUserFromContext(ctx)
	} else {
		user, _, err = h.authService.AuthenticateSession(ctx)
	}
	if err != nil {
		h.writeError(w, err)
		return "", "", false
	}
	sessionID, _ = ctx.Value(domain.SessionIDKey).(string)
	return user.ID, sessionID, true
}

// writeError answers a service error: the two-factor table, then the authentication and
// permission errors, then 500.
func (h *TwoFactorHandler) writeError(w http.ResponseWriter, err error) {
	if writePermissionError(w, err) {
		return
	}
	var unauthorized *domain.ErrUnauthorized
	var notFound *domain.ErrUserNotFound
	switch {
	case errors.As(err, &unauthorized):
		WriteJSONError(w, unauthorized.Error(), http.StatusForbidden)
	case errors.As(err, &notFound):
		WriteJSONError(w, "User not found", http.StatusNotFound)
	case errors.Is(err, service.ErrCannotResetOwnTwoFactor):
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
	default:
		h.logger.WithField("error", err.Error()).Error("Two-factor request failed")
		WriteJSONError(w, "Two-factor request failed", http.StatusInternalServerError)
	}
}

// Verify completes a sign-in that waits for its second factor. Public: the challenge token
// is the credential.
func (h *TwoFactorHandler) Verify(w http.ResponseWriter, r *http.Request) {
	// A public route: the limit comes first, and a request that names no challenge stays out
	// of the audit log (the service skips an unknown token the same way).
	if h.rateLimiter != nil && !h.rateLimiter.Allow(TwoFactorVerifyPolicy, middleware.ClientIP(r)) {
		domain.AuditFromContext(r.Context()).Skip()
		WriteJSONError(w, "Too many attempts, try again in a minute", http.StatusTooManyRequests)
		return
	}
	var req struct {
		ChallengeToken string `json:"challenge_token"`
		proofRequest
	}
	if !decodePost(w, r, &req) {
		domain.AuditFromContext(r.Context()).Skip()
		return
	}
	resp, err := h.twoFactor.VerifyChallenge(r.Context(), req.ChallengeToken, req.proof(), requestMeta(r))
	if err != nil {
		h.writeError(w, err)
		return
	}
	noStore(w)
	body := map[string]interface{}{
		"token":      resp.Token,
		"user":       resp.User,
		"expires_at": resp.ExpiresAt,
	}
	if resp.InvitationStatus != "" {
		body["invitation_status"] = resp.InvitationStatus
	}
	writeJSON(w, http.StatusOK, body)
}

// Setup starts setting up an authenticator app. Open to a session that has not passed the
// instance's requirement: it is how that session meets it.
func (h *TwoFactorHandler) Setup(w http.ResponseWriter, r *http.Request) {
	var req proofRequest
	if !decodePost(w, r, &req) {
		return
	}
	userID, sessionID, ok := h.consoleSession(w, r, false)
	if !ok {
		return
	}
	setup, err := h.twoFactor.Setup(r.Context(), userID, sessionID, req.proof(), requestMeta(r))
	if err != nil {
		h.writeError(w, err)
		return
	}
	noStore(w)
	writeJSON(w, http.StatusOK, setup)
}

// Enable confirms the setup with a code from the app.
func (h *TwoFactorHandler) Enable(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code                 string `json:"code"`
		SignOutOtherSessions *bool  `json:"sign_out_other_sessions"`
	}
	if !decodePost(w, r, &req) {
		return
	}
	userID, sessionID, ok := h.consoleSession(w, r, false)
	if !ok {
		return
	}
	// Signing the other sessions out is the default (ASVS 7.4.3).
	signOut := req.SignOutOtherSessions == nil || *req.SignOutOtherSessions
	codes, err := h.twoFactor.Enable(r.Context(), userID, sessionID, req.Code, signOut, requestMeta(r))
	if err != nil {
		h.writeError(w, err)
		return
	}
	noStore(w)
	if codes == nil {
		codes = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"recovery_codes": codes})
}

// Disable turns two-factor authentication off.
func (h *TwoFactorHandler) Disable(w http.ResponseWriter, r *http.Request) {
	var req proofRequest
	if !decodePost(w, r, &req) {
		return
	}
	userID, _, ok := h.consoleSession(w, r, true)
	if !ok {
		return
	}
	if err := h.twoFactor.Disable(r.Context(), userID, req.proof(), requestMeta(r)); err != nil {
		h.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

// RegenerateRecoveryCodes replaces the recovery codes.
func (h *TwoFactorHandler) RegenerateRecoveryCodes(w http.ResponseWriter, r *http.Request) {
	var req proofRequest
	if !decodePost(w, r, &req) {
		return
	}
	userID, _, ok := h.consoleSession(w, r, true)
	if !ok {
		return
	}
	codes, err := h.twoFactor.RegenerateRecoveryCodes(r.Context(), userID, req.proof(), requestMeta(r))
	if err != nil {
		h.writeError(w, err)
		return
	}
	noStore(w)
	writeJSON(w, http.StatusOK, map[string]interface{}{"recovery_codes": codes})
}

// ResetForUser removes another person's two-factor authentication: root only.
func (h *TwoFactorHandler) ResetForUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID string `json:"user_id"`
		Email  string `json:"email"`
		proofRequest
	}
	if !decodePost(w, r, &req) {
		return
	}
	if (req.UserID == "") == (req.Email == "") {
		WriteJSONError(w, "Give user_id or email", http.StatusBadRequest)
		return
	}
	userID, sessionID, ok := h.consoleSession(w, r, true)
	if !ok {
		return
	}
	err := h.twoFactor.ResetForUser(r.Context(), userID, sessionID, req.proof(), req.UserID, req.Email, requestMeta(r))
	if err != nil {
		h.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

// writeChallenge answers a sign-in that waits for its second factor: no token, no user.
func writeChallenge(w http.ResponseWriter, challenge *domain.TwoFactorChallenge) {
	noStore(w)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"two_factor_required":  true,
		"challenge_token":      challenge.Token,
		"challenge_expires_at": challenge.ExpiresAt.UTC().Format(time.RFC3339),
	})
}
