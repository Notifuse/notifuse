package http

import (
	"encoding/json"
	"fmt"
	"net/http"

	"aidanwoods.dev/go-paseto"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/http/middleware"
	"github.com/Notifuse/notifuse/pkg/analytics"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// AnalyticsHandler handles HTTP requests related to analytics
type AnalyticsHandler struct {
	service   domain.AnalyticsService
	logger    logger.Logger
	publicKey paseto.V4AsymmetricPublicKey
}

// NewAnalyticsHandler creates a new analytics handler
func NewAnalyticsHandler(
	service domain.AnalyticsService,
	publicKey paseto.V4AsymmetricPublicKey,
	logger logger.Logger,
) *AnalyticsHandler {
	return &AnalyticsHandler{
		service:   service,
		logger:    logger,
		publicKey: publicKey,
	}
}

// RegisterRoutes registers the analytics-related routes
func (h *AnalyticsHandler) RegisterRoutes(mux *http.ServeMux) {
	// Create auth middleware
	authMiddleware := middleware.NewAuthMiddleware(h.publicKey)
	requireAuth := authMiddleware.RequireAuth()

	// Register RPC-style endpoints with dot notation
	mux.Handle("/api/analytics.query", requireAuth(http.HandlerFunc(h.handleQuery)))
	mux.Handle("/api/analytics.schemas", requireAuth(http.HandlerFunc(h.handleGetSchemas)))
}

// AnalyticsQueryRequest represents the request payload for analytics queries
type AnalyticsQueryRequest struct {
	WorkspaceID string          `json:"workspace_id"`
	Query       analytics.Query `json:"query"`
}

// AnalyticsSchemasRequest represents the request payload for getting schemas
type AnalyticsSchemasRequest struct {
	WorkspaceID string `json:"workspace_id"`
}

// handleQuery handles analytics query requests
func (h *AnalyticsHandler) handleQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.writeErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req AnalyticsQueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to decode analytics query request")
		h.writeErrorResponse(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	// Validate required fields
	if req.WorkspaceID == "" {
		h.writeErrorResponse(w, http.StatusBadRequest, "workspace_id is required")
		return
	}

	// Execute the query
	response, err := h.service.Query(r.Context(), req.WorkspaceID, req.Query)
	if err != nil {
		h.logger.WithField("workspace_id", req.WorkspaceID).WithField("error", err.Error()).Error("Analytics query failed")
		h.writeErrorResponse(w, http.StatusInternalServerError, fmt.Sprintf("Query failed: %v", err))
		return
	}

	// Return the response
	h.writeJSONResponse(w, http.StatusOK, response)
}

// handleGetSchemas handles requests to get available analytics schemas
func (h *AnalyticsHandler) handleGetSchemas(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.writeErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req AnalyticsSchemasRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to decode analytics schemas request")
		h.writeErrorResponse(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	// Validate required fields
	if req.WorkspaceID == "" {
		h.writeErrorResponse(w, http.StatusBadRequest, "workspace_id is required")
		return
	}

	// Get schemas
	schemas, err := h.service.GetSchemas(r.Context(), req.WorkspaceID)
	if err != nil {
		h.logger.WithField("workspace_id", req.WorkspaceID).WithField("error", err.Error()).Error("Failed to get analytics schemas")
		h.writeErrorResponse(w, http.StatusInternalServerError, fmt.Sprintf("Failed to get schemas: %v", err))
		return
	}

	// Return the schemas
	h.writeJSONResponse(w, http.StatusOK, map[string]interface{}{
		"schemas": schemas,
	})
}

// writeJSONResponse writes a JSON response
func (h *AnalyticsHandler) writeJSONResponse(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to encode JSON response")
	}
}

// writeErrorResponse writes an error response
func (h *AnalyticsHandler) writeErrorResponse(w http.ResponseWriter, statusCode int, message string) {
	h.writeJSONResponse(w, statusCode, map[string]interface{}{
		"error":   true,
		"message": message,
	})
}
