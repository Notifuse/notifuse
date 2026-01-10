package http

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/http/middleware"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// SchemaVerificationServiceInterface defines the interface for schema verification
type SchemaVerificationServiceInterface interface {
	VerifyAllSchemas(ctx context.Context) (*domain.SchemaVerificationResult, error)
	RepairSchemas(ctx context.Context, req *domain.SchemaRepairRequest) (*domain.SchemaRepairResult, error)
}

// AuthServiceForSchemaInterface defines the auth interface needed by this handler
type AuthServiceForSchemaInterface interface {
	AuthenticateUserFromContext(ctx context.Context) (*domain.User, error)
}

// SchemaVerificationHandler handles debug schema verification and repair endpoints
type SchemaVerificationHandler struct {
	service      SchemaVerificationServiceInterface
	authService  AuthServiceForSchemaInterface
	getJWTSecret func() ([]byte, error)
	rootEmail    string
	logger       logger.Logger
}

// NewSchemaVerificationHandler creates a new schema verification handler
func NewSchemaVerificationHandler(
	service SchemaVerificationServiceInterface,
	authService AuthServiceForSchemaInterface,
	getJWTSecret func() ([]byte, error),
	rootEmail string,
	log logger.Logger,
) *SchemaVerificationHandler {
	return &SchemaVerificationHandler{
		service:      service,
		authService:  authService,
		getJWTSecret: getJWTSecret,
		rootEmail:    rootEmail,
		logger:       log,
	}
}

// RegisterRoutes registers the schema verification routes
func (h *SchemaVerificationHandler) RegisterRoutes(mux *http.ServeMux) {
	authMiddleware := middleware.NewAuthMiddleware(h.getJWTSecret)
	requireAuth := authMiddleware.RequireAuth()

	mux.Handle("/api/debug.verifySchema", requireAuth(http.HandlerFunc(h.handleVerifySchema)))
	mux.Handle("/api/debug.repairSchema", requireAuth(http.HandlerFunc(h.handleRepairSchema)))
}

// handleVerifySchema handles GET /api/debug.verifySchema
func (h *SchemaVerificationHandler) handleVerifySchema(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Authenticate user
	user, err := h.authService.AuthenticateUserFromContext(r.Context())
	if err != nil {
		h.logger.WithField("error", err.Error()).Warn("Authentication failed")
		WriteJSONError(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	// Root user check
	if user.Email != h.rootEmail {
		WriteJSONError(w, "Access denied: root user only", http.StatusForbidden)
		return
	}

	// Perform verification
	result, err := h.service.VerifyAllSchemas(r.Context())
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Schema verification failed")
		WriteJSONError(w, "Schema verification failed", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// handleRepairSchema handles POST /api/debug.repairSchema
func (h *SchemaVerificationHandler) handleRepairSchema(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Authenticate user
	user, err := h.authService.AuthenticateUserFromContext(r.Context())
	if err != nil {
		h.logger.WithField("error", err.Error()).Warn("Authentication failed")
		WriteJSONError(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	// Root user check
	if user.Email != h.rootEmail {
		WriteJSONError(w, "Access denied: root user only", http.StatusForbidden)
		return
	}

	// Parse request body
	var req domain.SchemaRepairRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Perform repair
	result, err := h.service.RepairSchemas(r.Context(), &req)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Schema repair failed")
		WriteJSONError(w, "Schema repair failed", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, result)
}
