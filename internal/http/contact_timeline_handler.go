package http

import (
	"net/http"

	"aidanwoods.dev/go-paseto"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/http/middleware"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/Notifuse/notifuse/pkg/tracing"
)

// ContactTimelineHandler handles HTTP requests for contact timeline
type ContactTimelineHandler struct {
	service     domain.ContactTimelineService
	authService domain.AuthService
	logger      logger.Logger
	publicKey   paseto.V4AsymmetricPublicKey
	tracer      tracing.Tracer
}

// NewContactTimelineHandler creates a new contact timeline handler
func NewContactTimelineHandler(
	service domain.ContactTimelineService,
	authService domain.AuthService,
	publicKey paseto.V4AsymmetricPublicKey,
	logger logger.Logger,
) *ContactTimelineHandler {
	return &ContactTimelineHandler{
		service:     service,
		authService: authService,
		logger:      logger,
		publicKey:   publicKey,
		tracer:      tracing.GetTracer(),
	}
}

// NewContactTimelineHandlerWithTracer creates a new contact timeline handler with a custom tracer
func NewContactTimelineHandlerWithTracer(
	service domain.ContactTimelineService,
	authService domain.AuthService,
	publicKey paseto.V4AsymmetricPublicKey,
	logger logger.Logger,
	tracer tracing.Tracer,
) *ContactTimelineHandler {
	return &ContactTimelineHandler{
		service:     service,
		authService: authService,
		logger:      logger,
		publicKey:   publicKey,
		tracer:      tracer,
	}
}

// RegisterRoutes registers the contact timeline HTTP endpoints
func (h *ContactTimelineHandler) RegisterRoutes(mux *http.ServeMux) {
	// Create auth middleware
	authMiddleware := middleware.NewAuthMiddleware(h.publicKey)
	requireAuth := authMiddleware.RequireAuth()

	// Register RPC-style endpoints with dot notation
	mux.Handle("/api/timeline.list", requireAuth(http.HandlerFunc(h.handleList)))
}

// handleList handles requests to list contact timeline with pagination
func (h *ContactTimelineHandler) handleList(w http.ResponseWriter, r *http.Request) {
	// codecov:ignore:start
	ctx, span := h.tracer.StartSpan(r.Context(), "ContactTimelineHandler.handleList")
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

	// Parse and validate request parameters
	var req domain.TimelineListRequest
	if err := req.FromQuery(r.URL.Query()); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Authenticate user for the workspace
	var err error
	ctx, _, _, err = h.authService.AuthenticateUserForWorkspace(ctx, req.WorkspaceID)
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

	// Call service to get the timeline entries
	entries, nextCursor, err := h.service.List(ctx, req.WorkspaceID, req.Email, req.Limit, req.Cursor)
	if err != nil {
		// codecov:ignore:start
		h.logger.Error(err.Error())
		if span != nil {
			h.tracer.MarkSpanError(ctx, err)
		}
		// codecov:ignore:end
		WriteJSONError(w, "Failed to list timeline entries", http.StatusInternalServerError)
		return
	}

	// Return JSON response
	response := domain.TimelineListResponse{
		Timeline:   entries,
		NextCursor: nextCursor,
	}
	writeJSON(w, http.StatusOK, response)
}
