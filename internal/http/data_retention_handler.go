package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/http/middleware"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// dataRetentionService is what the handler needs from service.DataRetentionService.
type dataRetentionService interface {
	Get(ctx context.Context, workspaceID string) (*service.DataRetentionView, error)
	Preview(ctx context.Context, workspaceID string, policy *domain.DataRetentionPolicy) (*service.DataRetentionPreview, error)
	SetPolicy(ctx context.Context, workspaceID string, policy *domain.DataRetentionPolicy) (*domain.DataRetentionSettings, error)
	CancelPending(ctx context.Context, workspaceID string) (*domain.DataRetentionSettings, error)
	ListRuns(ctx context.Context, workspaceID string, limit int) ([]*domain.RetentionRun, error)
	ExportRuns(ctx context.Context, workspaceID string, w io.Writer) error
	RetryBackfill(ctx context.Context, workspaceID string) error
}

// DataRetentionHandler serves the dataRetention.* routes.
type DataRetentionHandler struct {
	service      dataRetentionService
	getJWTSecret func() ([]byte, error)
	isDemo       bool
	logger       logger.Logger
}

// NewDataRetentionHandler creates the handler.
func NewDataRetentionHandler(svc dataRetentionService, getJWTSecret func() ([]byte, error), isDemo bool, log logger.Logger) *DataRetentionHandler {
	return &DataRetentionHandler{service: svc, getJWTSecret: getJWTSecret, isDemo: isDemo, logger: log}
}

// RegisterRoutes registers the routes. Changing the policy is closed on the demo instance.
func (h *DataRetentionHandler) RegisterRoutes(mux *http.ServeMux) {
	requireAuth := middleware.NewAuthMiddleware(h.getJWTSecret).RequireAuth()
	restrictedInDemo := middleware.RestrictedInDemo(h.isDemo)
	mux.Handle("/api/dataRetention.get", requireAuth(http.HandlerFunc(h.handleGet)))
	mux.Handle("/api/dataRetention.preview", requireAuth(http.HandlerFunc(h.handlePreview)))
	mux.Handle("/api/dataRetention.set", restrictedInDemo(requireAuth(http.HandlerFunc(h.handleSet))))
	mux.Handle("/api/dataRetention.cancelPending", restrictedInDemo(requireAuth(http.HandlerFunc(h.handleCancelPending))))
	mux.Handle("/api/dataRetention.retryBackfill", restrictedInDemo(requireAuth(http.HandlerFunc(h.handleRetryBackfill))))
	mux.Handle("/api/dataRetention.runs", requireAuth(http.HandlerFunc(h.handleRuns)))
	mux.Handle("/api/dataRetention.exportRuns", requireAuth(http.HandlerFunc(h.handleExportRuns)))
}

// dataRetentionPolicyRequest is the body of dataRetention.preview and dataRetention.set.
type dataRetentionPolicyRequest struct {
	WorkspaceID string                      `json:"workspace_id"`
	Policy      *domain.DataRetentionPolicy `json:"policy"`
}

func (h *DataRetentionHandler) writeError(w http.ResponseWriter, err error, fallback string) {
	var conflict *domain.ErrRetentionConflict
	if errors.As(err, &conflict) {
		WriteJSONError(w, conflict.Error(), http.StatusConflict)
		return
	}
	if writeServiceError(w, err, fallback) {
		return
	}
	h.logger.WithField("error", err.Error()).Error(fallback)
	WriteJSONError(w, fallback, http.StatusInternalServerError)
}

func (h *DataRetentionHandler) workspaceIDFromQuery(w http.ResponseWriter, r *http.Request) (string, bool) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return "", false
	}
	workspaceID := r.URL.Query().Get("workspace_id")
	if workspaceID == "" {
		WriteJSONError(w, "workspace_id is required", http.StatusBadRequest)
		return "", false
	}
	return workspaceID, true
}

func (h *DataRetentionHandler) decodePolicy(w http.ResponseWriter, r *http.Request) (*dataRetentionPolicyRequest, bool) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return nil, false
	}
	var req dataRetentionPolicyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return nil, false
	}
	if req.WorkspaceID == "" {
		WriteJSONError(w, "workspace_id is required", http.StatusBadRequest)
		return nil, false
	}
	if req.Policy == nil {
		WriteJSONError(w, "policy is required", http.StatusBadRequest)
		return nil, false
	}
	return &req, true
}

// GET /api/dataRetention.get?workspace_id=…
func (h *DataRetentionHandler) handleGet(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := h.workspaceIDFromQuery(w, r)
	if !ok {
		return
	}
	view, err := h.service.Get(r.Context(), workspaceID)
	if err != nil {
		h.writeError(w, err, "Failed to get the data retention settings")
		return
	}
	writeJSON(w, http.StatusOK, view)
}

// POST /api/dataRetention.preview {workspace_id, policy}
func (h *DataRetentionHandler) handlePreview(w http.ResponseWriter, r *http.Request) {
	req, ok := h.decodePolicy(w, r)
	if !ok {
		return
	}
	preview, err := h.service.Preview(r.Context(), req.WorkspaceID, req.Policy)
	if err != nil {
		h.writeError(w, err, "Failed to preview the data retention policy")
		return
	}
	writeJSON(w, http.StatusOK, preview)
}

// POST /api/dataRetention.set {workspace_id, policy}
func (h *DataRetentionHandler) handleSet(w http.ResponseWriter, r *http.Request) {
	req, ok := h.decodePolicy(w, r)
	if !ok {
		return
	}
	settings, err := h.service.SetPolicy(r.Context(), req.WorkspaceID, req.Policy)
	if err != nil {
		h.writeError(w, err, "Failed to set the data retention policy")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"data_retention": settings})
}

// POST /api/dataRetention.cancelPending {workspace_id}
func (h *DataRetentionHandler) handleCancelPending(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		WorkspaceID string `json:"workspace_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.WorkspaceID == "" {
		WriteJSONError(w, "workspace_id is required", http.StatusBadRequest)
		return
	}
	settings, err := h.service.CancelPending(r.Context(), req.WorkspaceID)
	if err != nil {
		h.writeError(w, err, "Failed to cancel the data retention change")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"data_retention": settings})
}

// POST /api/dataRetention.retryBackfill {workspace_id} — runs a failed engagement backfill
// again (owners).
func (h *DataRetentionHandler) handleRetryBackfill(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		WorkspaceID string `json:"workspace_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.WorkspaceID == "" {
		WriteJSONError(w, "workspace_id is required", http.StatusBadRequest)
		return
	}
	if err := h.service.RetryBackfill(r.Context(), req.WorkspaceID); err != nil {
		h.writeError(w, err, "Failed to run the engagement backfill again")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true})
}

// GET /api/dataRetention.runs?workspace_id=…&limit=…
func (h *DataRetentionHandler) handleRuns(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := h.workspaceIDFromQuery(w, r)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	runs, err := h.service.ListRuns(r.Context(), workspaceID, limit)
	if err != nil {
		h.writeError(w, err, "Failed to list the data retention runs")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"runs": runs})
}

// POST /api/dataRetention.exportRuns {workspace_id} — CSV, counts only. A POST, like
// auditLogs.export, so the console's download helper can fetch it.
func (h *DataRetentionHandler) handleExportRuns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		WorkspaceID string `json:"workspace_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.WorkspaceID == "" {
		WriteJSONError(w, "workspace_id is required", http.StatusBadRequest)
		return
	}
	workspaceID := req.WorkspaceID
	// Rendered first, so an error still answers JSON rather than a broken file.
	var buf bytes.Buffer
	if err := h.service.ExportRuns(r.Context(), workspaceID, &buf); err != nil {
		h.writeError(w, err, "Failed to export the data retention runs")
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="data-retention-runs.csv"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}
