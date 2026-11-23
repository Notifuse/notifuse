package http

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"go.opencensus.io/trace"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/http/middleware"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/Notifuse/notifuse/pkg/tracing"
)

// WorkspaceServiceInterface is already defined in workspace_handler.go
// So no need to define it again here

// UserServiceInterface defines the methods required from a user service
type UserServiceInterface interface {
	SignIn(ctx context.Context, input domain.SignInInput) (string, error)
	VerifyCode(ctx context.Context, input domain.VerifyCodeInput) (*domain.AuthResponse, error)
	RootSignin(ctx context.Context, input domain.RootSigninInput) (*domain.AuthResponse, error)
	VerifyUserSession(ctx context.Context, userID string, sessionID string) (*domain.User, error)
	GetUserByID(ctx context.Context, userID string) (*domain.User, error)
	UpdateUserLanguage(ctx context.Context, userID string, language string) error
	Logout(ctx context.Context, userID string) error
}

// LicenseStateReader is the slice of the licence service /api/user.me needs: what this
// deployment is licensed for.
//
// One method and no more. settings_handler.go declares its own LicenseServiceInterface with a
// second — SetKey — because installing a key is a settings concern; user.me only ever reads,
// and an interface that carried SetKey here would put a licence installer one typo away from
// the endpoint every signed-in session calls.
//
// service.LicenseService satisfies both.
type LicenseStateReader interface {
	Entitlements() domain.Entitlements
}

type UserHandler struct {
	userService      UserServiceInterface
	workspaceService domain.WorkspaceServiceInterface
	licenseService   LicenseStateReader
	authService      domain.AuthService
	twoFactor        TwoFactorServiceInterface
	config           *config.Config
	getJWTSecret     func() ([]byte, error)
	logger           logger.Logger
	tracer           tracing.Tracer
}

// extractEmailDomain extracts domain part from an email address
// This is used to add context to traces without exposing PII
func extractEmailDomain(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) == 2 {
		return parts[1]
	}
	return ""
}

// NewUserHandler builds the handler serving sign-in, sign-out and /api/user.me.
//
// licenseService is a REQUIRED positional parameter, deliberately not variadic and not
// nil-tolerant. /api/user.me is the only endpoint by which a non-root console session can
// learn what its deployment is licensed for — /api/licence.get is root-only — so a wiring
// that forgot to pass one would leave every non-root user discovering each gate from the
// first refusal, with no explanation attached. Omitting it has to be a compile error, because
// a silently unwired licence source is exactly the failure this design cannot detect at
// runtime.
//
// authService and twoFactor are required too: user.me, user.logout and
// user.updateLanguage check the session through authService without the instance's
// two-factor requirement, and user.me reports the two-factor state.
func NewUserHandler(userService UserServiceInterface, workspaceService domain.WorkspaceServiceInterface, licenseService LicenseStateReader, authService domain.AuthService, twoFactor TwoFactorServiceInterface, cfg *config.Config, getJWTSecret func() ([]byte, error), logger logger.Logger) *UserHandler {
	return &UserHandler{
		userService:      userService,
		workspaceService: workspaceService,
		licenseService:   licenseService,
		authService:      authService,
		twoFactor:        twoFactor,
		config:           cfg,
		getJWTSecret:     getJWTSecret,
		logger:           logger,
		tracer:           tracing.GetTracer(),
	}
}

func (h *UserHandler) SignIn(w http.ResponseWriter, r *http.Request) {
	// POST only. This handler SENDS AN EMAIL, so a verb that reaches it by accident is a
	// message somebody did not ask for.
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx, span := h.tracer.StartSpan(r.Context(), "UserHandler.SignIn")
	defer span.End()

	var input domain.SignInInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		WriteJSONError(w, "Invalid SignIn request body", http.StatusBadRequest)
		span.SetStatus(trace.Status{
			Code:    trace.StatusCodeInvalidArgument,
			Message: "Invalid request body",
		})
		return
	}

	// Add email domain to span for context (masking email for privacy)
	span.AddAttributes(trace.StringAttribute("user.email.domain", extractEmailDomain(input.Email)))

	h.tracer.AddAttribute(ctx, "operation", "SignIn")
	code, err := h.userService.SignIn(ctx, input)
	if err != nil {
		// Check if it's a user not found error and return 400
		if _, ok := err.(*domain.ErrUserNotFound); ok {
			WriteJSONError(w, err.Error(), http.StatusBadRequest)
			h.tracer.MarkSpanError(ctx, err)
			return
		}

		// For all other errors, return 500
		WriteJSONError(w, err.Error(), http.StatusInternalServerError)
		h.tracer.MarkSpanError(ctx, err)
		return
	}

	// In development mode, the code will be returned
	// In production, the code will be empty
	response := map[string]string{
		"message": "Magic code sent to your email",
	}

	if code != "" {
		response["code"] = code
		span.AddAttributes(trace.BoolAttribute("dev.code_returned", true))
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(response)
}

func (h *UserHandler) VerifyCode(w http.ResponseWriter, r *http.Request) {
	// POST only. This handler mints a session.
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx, span := h.tracer.StartSpan(r.Context(), "UserHandler.VerifyCode")
	defer span.End()

	var input domain.VerifyCodeInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		WriteJSONError(w, "Invalid VerifyCode request body", http.StatusBadRequest)
		span.SetStatus(trace.Status{
			Code:    trace.StatusCodeInvalidArgument,
			Message: "Invalid request body",
		})
		return
	}

	// Add email domain to span for context (masking email for privacy)
	span.AddAttributes(trace.StringAttribute("user.email.domain", extractEmailDomain(input.Email)))

	h.tracer.AddAttribute(ctx, "operation", "VerifyCode")
	response, err := h.userService.VerifyCode(ctx, input)
	if err != nil {
		WriteJSONError(w, err.Error(), http.StatusUnauthorized)
		h.tracer.MarkSpanError(ctx, err)
		return
	}

	// Set user ID in span once we have it
	if response != nil && response.User.ID != "" {
		span.AddAttributes(trace.StringAttribute("user.id", response.User.ID))
	}

	// The code was right and the account has two-factor authentication: no token yet.
	if response.Challenge != nil {
		writeChallenge(w, response.Challenge)
		return
	}

	noStore(w)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(response)
}

// RootSignIn handles programmatic signin for the root user using HMAC signature
func (h *UserHandler) RootSignIn(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tracer.StartSpan(r.Context(), "UserHandler.RootSignIn")
	defer span.End()

	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		span.SetStatus(trace.Status{
			Code:    trace.StatusCodeInvalidArgument,
			Message: "Method not allowed",
		})
		return
	}

	var input domain.RootSigninInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		span.SetStatus(trace.Status{
			Code:    trace.StatusCodeInvalidArgument,
			Message: "Invalid request body",
		})
		return
	}

	// Validate required fields
	if input.Email == "" || input.Timestamp == 0 || input.Signature == "" {
		WriteJSONError(w, "Missing required fields: email, timestamp, signature", http.StatusBadRequest)
		span.SetStatus(trace.Status{
			Code:    trace.StatusCodeInvalidArgument,
			Message: "Missing required fields",
		})
		return
	}

	// Add email domain to span for context (masking email for privacy)
	span.AddAttributes(trace.StringAttribute("user.email.domain", extractEmailDomain(input.Email)))

	h.tracer.AddAttribute(ctx, "operation", "RootSignin")
	response, err := h.userService.RootSignin(ctx, input)
	if err != nil {
		// Use generic error message to prevent enumeration
		WriteJSONError(w, "Invalid credentials", http.StatusUnauthorized)
		h.tracer.MarkSpanError(ctx, err)
		return
	}

	// Set user ID in span once we have it
	if response != nil && response.User.ID != "" {
		span.AddAttributes(trace.StringAttribute("user.id", response.User.ID))
	}

	noStore(w)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(response)
}

// GetCurrentUser returns the authenticated user, their workspaces, and — for a console
// session — what the deployment is licensed for.
func (h *UserHandler) GetCurrentUser(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tracer.StartSpan(r.Context(), "UserHandler.GetCurrentUser")
	defer span.End()

	// GET only. user.me is a read and answers 405 to everything else, in every licence
	// state — this is the call the console makes to find out what the deployment is
	// licensed for, and it must answer the same way whether or not it is licensed.
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		span.SetStatus(trace.Status{
			Code:    trace.StatusCodeInvalidArgument,
			Message: "Method not allowed",
		})
		return
	}

	// Get authenticated user from context
	userID, ok := ctx.Value(domain.UserIDKey).(string)
	if !ok || userID == "" {
		WriteJSONError(w, "Unauthorized", http.StatusUnauthorized)
		span.SetStatus(trace.Status{
			Code:    trace.StatusCodePermissionDenied,
			Message: "Unauthorized - missing userID",
		})
		return
	}

	// Add user ID to span for context
	span.AddAttributes(trace.StringAttribute("user.id", userID))

	// Verify session exists for user-type tokens
	var session *domain.SessionAuth
	userType, ok := ctx.Value(domain.UserTypeKey).(string)
	if ok && userType == string(domain.UserTypeUser) {
		sessionID, ok := ctx.Value(domain.SessionIDKey).(string)
		if !ok || sessionID == "" {
			WriteJSONError(w, "Unauthorized", http.StatusUnauthorized)
			span.SetStatus(trace.Status{
				Code:    trace.StatusCodeUnauthenticated,
				Message: "Missing session ID",
			})
			return
		}

		// Verify session exists (this will fail if user logged out). Without the
		// instance's two-factor requirement: user.me is how a session that has not met
		// it learns that it must.
		var err error
		_, session, err = h.authService.AuthenticateSession(ctx)
		if err != nil {
			if sessionCheckStatus(err) != http.StatusUnauthorized {
				WriteJSONError(w, "Failed to verify session", http.StatusInternalServerError)
				h.tracer.MarkSpanError(ctx, err)
				return
			}
			WriteJSONError(w, "Session expired or invalid", http.StatusUnauthorized)
			span.SetStatus(trace.Status{
				Code:    trace.StatusCodeUnauthenticated,
				Message: "Session verification failed",
			})
			return
		}
	}
	// A session that has not met the instance's requirement sees no workspace: a console
	// session's workspace carries the file manager's S3 secret (Workspace.Redact).
	twoFactorRequired := session != nil && h.twoFactor.RequireTwoFactor() && !domain.SessionSatisfiesTwoFactor(*session)

	// Get user details
	h.tracer.AddAttribute(ctx, "operation", "GetUserByID")
	user, err := h.userService.GetUserByID(ctx, userID)
	if err != nil {
		WriteJSONError(w, "User not found", http.StatusNotFound)
		span.SetStatus(trace.Status{
			Code:    trace.StatusCodeNotFound,
			Message: "User not found",
		})
		return
	}

	// Add user email domain to span for context (masking email for privacy)
	span.AddAttributes(trace.StringAttribute("user.email.domain", extractEmailDomain(user.Email)))

	// Get user's workspaces
	workspaces := []*domain.Workspace{}
	if !twoFactorRequired {
		h.tracer.AddAttribute(ctx, "operation", "ListWorkspaces")
		workspaces, err = h.workspaceService.ListWorkspaces(ctx)
		if err != nil {
			if !writeServiceError(w, err, "Failed to retrieve workspaces") {
				WriteJSONError(w, "Failed to retrieve workspaces", http.StatusInternalServerError)
			}
			span.SetStatus(trace.Status{
				Code:    trace.StatusCodeInternal,
				Message: "Failed to retrieve workspaces",
			})
			return
		}
	}

	// Add workspace count to span for context
	span.AddAttributes(trace.Int64Attribute("workspaces.count", int64(len(workspaces))))

	// Credentials are decrypted on load for the sending path; they must not leave
	// the process. See redactWorkspace.
	//
	// This is the widest of the workspace-serialising endpoints: one call, every
	// workspace the caller belongs to, for a session or an API key alike.
	for _, ws := range workspaces {
		redactWorkspace(ws)
	}

	// Combine user and workspaces in response
	response := map[string]interface{}{
		"user":       user,
		"workspaces": workspaces,
	}

	// The console's licence state is fed from here, not from a second round trip.
	// /api/licence.get is the other source and it is root-only, so this is the ONLY way a
	// non-root session can learn what the deployment bought before a gate refuses something —
	// which is the difference between an explained restriction and a button that mysteriously
	// stopped working.
	//
	// domain.Entitlements verbatim rather than a shape of this endpoint's own: it is the
	// same value every gate reads and the same value /api/licence.get answers with, so what
	// the console greys out and what the backend refuses cannot drift apart. It carries no
	// key and cannot be made to — a licence key is a bearer credential, and the claims hold
	// everything a console needs anyway.
	//
	// Console sessions only. An API key authenticates this same endpoint — it is how an
	// integration resolves its workspaces — and the entitlements name the licensee and
	// their billing address, which is why /api/licence.get is root-only in the first
	// place. The test fails closed: absent means "not told", and the console types the
	// field optional so absence reads as unknown rather than as unlicensed.
	if isConsoleSession(ctx) {
		entitlements := h.licenseService.Entitlements()
		// Org and Sub name the licensee and their billing contact. /api/licence.get is
		// root-only for exactly that reason, and the "Licensed to" card that shows them
		// lives in a root-only settings panel — so a non-root session has no use for
		// them and no right to them. Everything the banner needs (state, features,
		// quota, expiry, tier) is left intact. Entitlements() returns a value, so
		// this blanks a copy and nothing shared.
		if !h.config.IsRootEmail(user.Email) {
			entitlements.Org = ""
			entitlements.Sub = ""
		}
		response["entitlements"] = entitlements

		// Whether this console asks updates.notifuse.com for the releases it is missing,
		// and the version to ask with. Only the root user and workspace owners can act on
		// an upgrade, so only their consoles ask. The version is the compiled constant:
		// Config.Version follows the VERSION environment variable, and a value such as
		// "44.0-custom" would be refused by the update service and hide the answer.
		response["check_for_updates"] = h.consoleChecksForUpdates(ctx, user)
		response["version"] = config.VERSION

		// The two-factor state of the account and of this session. A failure to read it
		// leaves the field out rather than failing the call every console starts with;
		// two_factor_required below does not depend on it.
		if status, err := h.twoFactor.Status(ctx, user.ID); err != nil {
			h.logger.WithField("error", err.Error()).Error("Failed to read the two-factor state")
			h.tracer.MarkSpanError(ctx, err)
		} else {
			response["two_factor"] = map[string]interface{}{
				"enabled":                  status.Enabled,
				"enabled_at":               status.EnabledAt,
				"recovery_codes_remaining": status.RecoveryCodesRemaining,
				"required_by_instance":     h.twoFactor.RequireTwoFactor(),
			}
		}
		if session != nil {
			response["session_second_factor"] = session.SecondFactor
			// Whether this session counts as having passed the second factor (single sign-on
			// and the root signature do): a root user with two-factor authentication needs it
			// to reach workspaces.
			response["session_two_factor_passed"] = domain.SessionSatisfiesTwoFactor(*session)
		}
		response["two_factor_required"] = twoFactorRequired
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(response)
}

// consoleChecksForUpdates is CHECK_FOR_UPDATES narrowed to the people who can upgrade: the
// root user, and the owners of at least one workspace. A failure to read the memberships
// answers false: a missed announcement is better than a failed sign-in.
func (h *UserHandler) consoleChecksForUpdates(ctx context.Context, user *domain.User) bool {
	if h.config == nil || !h.config.CheckForUpdates {
		return false
	}
	if h.config.IsRootEmail(user.Email) {
		return true
	}
	owns, err := h.workspaceService.UserOwnsAWorkspace(ctx, user.ID)
	if err != nil {
		h.logger.WithField("error", err.Error()).Warn("Could not tell whether the user owns a workspace; not checking for updates")
		return false
	}
	return owns
}

// Logout logs out the current user by deleting all their sessions
func (h *UserHandler) Logout(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tracer.StartSpan(r.Context(), "UserHandler.Logout")
	defer span.End()

	// Get authenticated user from context
	userID, ok := ctx.Value(domain.UserIDKey).(string)
	if !ok || userID == "" {
		WriteJSONError(w, "Unauthorized", http.StatusUnauthorized)
		span.SetStatus(trace.Status{
			Code:    trace.StatusCodeUnauthenticated,
			Message: "No user ID in context",
		})
		return
	}

	span.AddAttributes(trace.StringAttribute("user.id", userID))
	h.tracer.AddAttribute(ctx, "operation", "Logout")

	if !h.requirePersonSession(w, r) {
		return
	}

	// Logout user - delete all sessions
	err := h.userService.Logout(ctx, userID)
	if err != nil {
		WriteJSONError(w, "Failed to logout", http.StatusInternalServerError)
		h.tracer.MarkSpanError(ctx, err)
		return
	}

	// Return success response
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"message": "Logged out successfully",
	})
}

// UpdateLanguage updates the authenticated user's preferred language for the
// console UI and system emails.
func (h *UserHandler) UpdateLanguage(w http.ResponseWriter, r *http.Request) {
	// POST only, like every other write in this package.
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx, span := h.tracer.StartSpan(r.Context(), "UserHandler.UpdateLanguage")
	defer span.End()

	// Get authenticated user from context
	userID, ok := ctx.Value(domain.UserIDKey).(string)
	if !ok || userID == "" {
		WriteJSONError(w, "Unauthorized", http.StatusUnauthorized)
		span.SetStatus(trace.Status{
			Code:    trace.StatusCodeUnauthenticated,
			Message: "No user ID in context",
		})
		return
	}

	if !h.requirePersonSession(w, r) {
		return
	}

	var input struct {
		Language string `json:"language"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		WriteJSONError(w, "Invalid UpdateLanguage request body", http.StatusBadRequest)
		span.SetStatus(trace.Status{
			Code:    trace.StatusCodeInvalidArgument,
			Message: "Invalid request body",
		})
		return
	}

	span.AddAttributes(
		trace.StringAttribute("user.id", userID),
		trace.StringAttribute("user.language", input.Language),
	)
	h.tracer.AddAttribute(ctx, "operation", "UpdateLanguage")

	// The service is the single validation authority for the language value.
	if err := h.userService.UpdateUserLanguage(ctx, userID, input.Language); err != nil {
		if _, ok := err.(*domain.ErrUnsupportedLanguage); ok {
			WriteJSONError(w, err.Error(), http.StatusBadRequest)
			h.tracer.MarkSpanError(ctx, err)
			return
		}
		if _, ok := err.(*domain.ErrUserNotFound); ok {
			WriteJSONError(w, err.Error(), http.StatusNotFound)
			h.tracer.MarkSpanError(ctx, err)
			return
		}
		WriteJSONError(w, "Failed to update language", http.StatusInternalServerError)
		h.tracer.MarkSpanError(ctx, err)
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"message": "Language updated successfully",
	})
}

// requirePersonSession lets through a live console session, whether or not it met the
// instance's two-factor requirement. An API key is a valid credential without access to
// these routes: 403, which a client does not read as a dead credential.
func (h *UserHandler) requirePersonSession(w http.ResponseWriter, r *http.Request) bool {
	if !isConsoleSession(r.Context()) {
		WriteJSONError(w, "This route is for console sessions", http.StatusForbidden)
		return false
	}
	if _, _, err := h.authService.AuthenticateSession(r.Context()); err != nil {
		if sessionCheckStatus(err) == http.StatusUnauthorized {
			WriteJSONError(w, "Session expired or invalid", http.StatusUnauthorized)
		} else {
			WriteJSONError(w, "Failed to verify session", http.StatusInternalServerError)
		}
		return false
	}
	return true
}

func (h *UserHandler) RegisterRoutes(mux *http.ServeMux) {
	// Public routes (no auth required)
	mux.HandleFunc("/api/user.signin", h.SignIn)
	mux.HandleFunc("/api/user.verify", h.VerifyCode)
	mux.HandleFunc("/api/user.rootSignin", h.RootSignIn)

	// Create auth middleware
	authMiddleware := middleware.NewAuthMiddleware(h.getJWTSecret)
	requireAuth := authMiddleware.RequireAuth()

	// Register protected routes
	mux.Handle("/api/user.me", requireAuth(http.HandlerFunc(h.GetCurrentUser)))
	mux.Handle("/api/user.logout", requireAuth(http.HandlerFunc(h.Logout)))
	mux.Handle("/api/user.updateLanguage", requireAuth(http.HandlerFunc(h.UpdateLanguage)))
}
