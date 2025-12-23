package http

import (
	"net/http"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/http/middleware"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// EmailQueueHandler handles email queue API endpoints
type EmailQueueHandler struct {
	repo         domain.EmailQueueRepository
	authService  domain.AuthService
	logger       logger.Logger
	getJWTSecret func() ([]byte, error)
}

// NewEmailQueueHandler creates a new email queue handler
func NewEmailQueueHandler(repo domain.EmailQueueRepository, authService domain.AuthService, getJWTSecret func() ([]byte, error), logger logger.Logger) *EmailQueueHandler {
	return &EmailQueueHandler{
		repo:         repo,
		authService:  authService,
		logger:       logger,
		getJWTSecret: getJWTSecret,
	}
}

// RegisterRoutes registers the email queue routes
func (h *EmailQueueHandler) RegisterRoutes(mux *http.ServeMux) {
	authMiddleware := middleware.NewAuthMiddleware(h.getJWTSecret)
	requireAuth := authMiddleware.RequireAuth()

	mux.Handle("/api/email_queue.stats", requireAuth(http.HandlerFunc(h.handleStats)))
}

// handleStats returns queue statistics for a workspace
func (h *EmailQueueHandler) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.GetEmailQueueStatsRequest
	if err := req.FromURLParams(r.URL.Query()); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Authorize user for workspace
	_, _, _, err := h.authService.AuthenticateUserForWorkspace(r.Context(), req.WorkspaceID)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to authenticate user for workspace")
		WriteJSONError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	stats, err := h.repo.GetStats(r.Context(), req.WorkspaceID)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to get email queue stats")
		WriteJSONError(w, "Failed to get queue stats", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"stats": stats,
	})
}
