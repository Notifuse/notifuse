package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/http/middleware"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// sesAccessDeniedCode lets the console tell "these credentials may not list this" apart from
// "these credentials are wrong". The first degrades to a plain text input with a hint; the
// second is a real error the operator must fix.
const sesAccessDeniedCode = "ses_access_denied"

// SESHandler exposes SES account discovery and managed tenant provisioning.
type SESHandler struct {
	service      domain.SESDiscoveryServiceInterface
	logger       logger.Logger
	getJWTSecret func() ([]byte, error)
}

func NewSESHandler(
	service domain.SESDiscoveryServiceInterface,
	getJWTSecret func() ([]byte, error),
	logger logger.Logger,
) *SESHandler {
	return &SESHandler{
		service:      service,
		logger:       logger,
		getJWTSecret: getJWTSecret,
	}
}

// RegisterRoutes registers the SES endpoints. All are POST because AWS credentials may travel
// in the body when an integration has not been saved yet.
func (h *SESHandler) RegisterRoutes(mux *http.ServeMux) {
	authMiddleware := middleware.NewAuthMiddleware(h.getJWTSecret)
	requireAuth := authMiddleware.RequireAuth()

	mux.Handle("/api/ses.listTenants", requireAuth(http.HandlerFunc(h.handleListTenants)))
	mux.Handle("/api/ses.listConfigurationSets", requireAuth(http.HandlerFunc(h.handleListConfigurationSets)))
	mux.Handle("/api/ses.verifyTenant", requireAuth(http.HandlerFunc(h.handleVerifyTenant)))
	mux.Handle("/api/ses.enableTenantIsolation", requireAuth(http.HandlerFunc(h.handleEnableTenantIsolation)))
}

func (h *SESHandler) handleListTenants(w http.ResponseWriter, r *http.Request) {
	var req domain.SESCredentialsRef
	if !h.decode(w, r, &req) {
		return
	}

	result, err := h.service.ListTenants(r.Context(), req)
	if err != nil {
		h.writeError(w, err, "Failed to list SES tenants")
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (h *SESHandler) handleListConfigurationSets(w http.ResponseWriter, r *http.Request) {
	var req domain.SESCredentialsRef
	if !h.decode(w, r, &req) {
		return
	}

	result, err := h.service.ListConfigurationSets(r.Context(), req)
	if err != nil {
		h.writeError(w, err, "Failed to list SES configuration sets")
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (h *SESHandler) handleVerifyTenant(w http.ResponseWriter, r *http.Request) {
	var req domain.VerifySESTenantRequest
	if !h.decode(w, r, &req) {
		return
	}
	if err := req.Validate(); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	result, err := h.service.VerifyTenant(r.Context(), req)
	if err != nil {
		h.writeError(w, err, "Failed to verify SES tenant")
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (h *SESHandler) handleEnableTenantIsolation(w http.ResponseWriter, r *http.Request) {
	var req domain.EnableSESTenantIsolationRequest
	if !h.decode(w, r, &req) {
		return
	}
	if err := req.Validate(); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	result, err := h.service.EnableTenantIsolation(r.Context(), req)
	if err != nil {
		h.writeError(w, err, "Failed to enable SES tenant isolation")
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// decode enforces POST and parses the body. It does not validate: each handler validates its own
// request type, and SESCredentialsRef is validated inside the service where it is used.
func (h *SESHandler) decode(w http.ResponseWriter, r *http.Request, target interface{}) bool {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return false
	}

	if err := json.NewDecoder(r.Body).Decode(target); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to decode SES request body")
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return false
	}

	return true
}

func (h *SESHandler) writeError(w http.ResponseWriter, err error, context string) {
	var unauthorized *domain.ErrUnauthorized
	switch {
	case domain.IsSESAccessDenied(err):
		// Not a failure to report loudly: the feature is designed to degrade without these
		// optional IAM permissions.
		writeJSON(w, http.StatusForbidden, map[string]interface{}{
			"error": err.Error(),
			"code":  sesAccessDeniedCode,
		})
	case errors.As(err, &unauthorized):
		WriteJSONError(w, err.Error(), http.StatusForbidden)
	default:
		h.logger.WithField("error", err.Error()).Error(context)
		WriteJSONError(w, context+": "+err.Error(), http.StatusBadGateway)
	}
}
