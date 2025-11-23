package http

import (
	"net/http"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/http/middleware"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/Notifuse/notifuse/pkg/tracing"
)

// MessageHistoryHandler handles HTTP requests for message history
type MessageHistoryHandler struct {
	service      domain.MessageHistoryService
	authService  domain.AuthService
	logger       logger.Logger
	getJWTSecret func() ([]byte, error)
	tracer       tracing.Tracer
}

// NewMessageHistoryHandler creates a new message history handler
func NewMessageHistoryHandler(
	service domain.MessageHistoryService,
	authService domain.AuthService,
	getJWTSecret func() ([]byte, error),
	logger logger.Logger,
) *MessageHistoryHandler {
	return &MessageHistoryHandler{
		service:      service,
		authService:  authService,
		logger:       logger,
		getJWTSecret: getJWTSecret,
		tracer:       tracing.GetTracer(),
	}
}

// NewMessageHistoryHandlerWithTracer creates a new message history handler with a custom tracer
func NewMessageHistoryHandlerWithTracer(
	service domain.MessageHistoryService,
	authService domain.AuthService,
	getJWTSecret func() ([]byte, error),
	logger logger.Logger,
	tracer tracing.Tracer,
) *MessageHistoryHandler {
	return &MessageHistoryHandler{
		service:      service,
		authService:  authService,
		logger:       logger,
		getJWTSecret: getJWTSecret,
		tracer:       tracer,
	}
}

// RegisterRoutes registers the message history HTTP endpoints
func (h *MessageHistoryHandler) RegisterRoutes(mux *http.ServeMux) {
	// Create auth middleware
	authMiddleware := middleware.NewAuthMiddleware(h.getJWTSecret)
	requireAuth := authMiddleware.RequireAuth()

	// Register RPC-style endpoints with dot notation
	mux.Handle("/api/messages.list", requireAuth(http.HandlerFunc(h.handleList)))
	mux.Handle("/api/messages.broadcastStats", requireAuth(http.HandlerFunc(h.handleBroadcastStats)))
}

// handleList handles requests to list message history with pagination and filtering
func (h *MessageHistoryHandler) handleList(w http.ResponseWriter, r *http.Request) {
	// codecov:ignore:start
	ctx, span := h.tracer.StartSpan(r.Context(), "MessageHistoryHandler.handleList")
	defer func() {
		if span != nil {
			h.tracer.EndSpan(span, nil)
		}
	}()
	// codecov:ignore:end

	// Only accept GET requests
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get workspace ID from query parameters
	workspaceID := r.URL.Query().Get("workspace_id")
	if workspaceID == "" {
		WriteJSONError(w, "Missing workspace ID", http.StatusBadRequest)
		return
	}

	// Authenticate user for the workspace
	var err error
	ctx, _, _, err = h.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		// codecov:ignore:start
		h.logger.Error(err.Error())
		if span != nil {
			h.tracer.MarkSpanError(ctx, err)
		}
		// codecov:ignore:end
		WriteJSONError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Create and parse query parameters
	var params domain.MessageListParams
	if err := params.FromQuery(r.URL.Query()); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Call service to get the messages with pagination and filtering
	result, err := h.service.ListMessages(ctx, workspaceID, params)
	if err != nil {
		// codecov:ignore:start
		h.logger.Error(err.Error())
		if span != nil {
			h.tracer.MarkSpanError(ctx, err)
		}
		// codecov:ignore:end
		WriteJSONError(w, "Failed to list messages", http.StatusInternalServerError)
		return
	}

	// Return JSON response
	writeJSON(w, http.StatusOK, result)
}

func (h *MessageHistoryHandler) handleBroadcastStats(w http.ResponseWriter, r *http.Request) {
	// codecov:ignore:start
	ctx, span := h.tracer.StartSpan(r.Context(), "MessageHistoryHandler.handleBroadcastStats")
	defer func() {
		if span != nil {
			h.tracer.EndSpan(span, nil)
		}
	}()
	// codecov:ignore:end

	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	broadcastID := r.URL.Query().Get("broadcast_id")
	if broadcastID == "" {
		WriteJSONError(w, "broadcast_id is required", http.StatusBadRequest)
		return
	}

	workspaceID := r.URL.Query().Get("workspace_id")
	if workspaceID == "" {
		WriteJSONError(w, "workspace_id is required", http.StatusBadRequest)
		return
	}

	stats, err := h.service.GetBroadcastStats(ctx, workspaceID, broadcastID)
	if err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to get stats")
		WriteJSONError(w, "Failed to get stats", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"broadcast_id": broadcastID,
		"stats":        stats,
	})
}
