package http

import (
	"encoding/json"
	"net/http"
	"time"

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
	mux.Handle("/api/email_queue.dead_letter.list", requireAuth(http.HandlerFunc(h.handleDeadLetterList)))
	mux.Handle("/api/email_queue.dead_letter.cleanup", requireAuth(http.HandlerFunc(h.handleDeadLetterCleanup)))
	mux.Handle("/api/email_queue.dead_letter.retry", requireAuth(http.HandlerFunc(h.handleDeadLetterRetry)))
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

// handleDeadLetterList returns paginated dead letter entries
func (h *EmailQueueHandler) handleDeadLetterList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.GetDeadLetterEntriesRequest
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

	entries, total, err := h.repo.GetDeadLetterEntries(r.Context(), req.WorkspaceID, req.Limit, req.Offset)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to get dead letter entries")
		WriteJSONError(w, "Failed to get dead letter entries", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"entries": entries,
		"total":   total,
		"limit":   req.Limit,
		"offset":  req.Offset,
	})
}

// handleDeadLetterCleanup deletes old dead letter entries
func (h *EmailQueueHandler) handleDeadLetterCleanup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.CleanupDeadLetterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to decode request body")
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := req.Validate(); err != nil {
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

	olderThan := time.Duration(req.RetentionHours) * time.Hour
	deleted, err := h.repo.CleanupDeadLetter(r.Context(), req.WorkspaceID, olderThan)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to cleanup dead letter entries")
		WriteJSONError(w, "Failed to cleanup dead letter entries", http.StatusInternalServerError)
		return
	}

	h.logger.WithFields(map[string]interface{}{
		"workspace_id":    req.WorkspaceID,
		"retention_hours": req.RetentionHours,
		"deleted":         deleted,
	}).Info("Dead letter cleanup completed")

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"deleted":         deleted,
		"retention_hours": req.RetentionHours,
	})
}

// handleDeadLetterRetry moves a dead letter entry back to the queue for retry
func (h *EmailQueueHandler) handleDeadLetterRetry(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.RetryDeadLetterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to decode request body")
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := req.Validate(); err != nil {
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

	err = h.repo.RetryDeadLetter(r.Context(), req.WorkspaceID, req.DeadLetterID)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to retry dead letter entry")
		WriteJSONError(w, "Failed to retry dead letter entry", http.StatusInternalServerError)
		return
	}

	h.logger.WithFields(map[string]interface{}{
		"workspace_id":   req.WorkspaceID,
		"dead_letter_id": req.DeadLetterID,
	}).Info("Dead letter entry moved back to queue for retry")

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
	})
}
