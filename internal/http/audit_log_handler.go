package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/http/middleware"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// AuditLogHandler serves the read side of the audit log: list, get, the
// action catalogue, and a streaming export. None of these consult the
// licence; the service refuses on membership and permission only.
type AuditLogHandler struct {
	service      domain.AuditLogService
	logger       logger.Logger
	getJWTSecret func() ([]byte, error)
}

// NewAuditLogHandler builds the handler.
func NewAuditLogHandler(service domain.AuditLogService, getJWTSecret func() ([]byte, error), logger logger.Logger) *AuditLogHandler {
	return &AuditLogHandler{
		service:      service,
		logger:       logger,
		getJWTSecret: getJWTSecret,
	}
}

// RegisterRoutes registers the audit log endpoints. Reads stay open in demo
// mode — the log is content to browse — and the export is a read too.
func (h *AuditLogHandler) RegisterRoutes(mux *http.ServeMux) {
	authMiddleware := middleware.NewAuthMiddleware(h.getJWTSecret)
	requireAuth := authMiddleware.RequireAuth()

	mux.Handle("/api/auditLogs.list", requireAuth(http.HandlerFunc(h.handleList)))
	mux.Handle("/api/auditLogs.get", requireAuth(http.HandlerFunc(h.handleGet)))
	mux.Handle("/api/auditLogs.actions", requireAuth(http.HandlerFunc(h.handleActions)))
	mux.Handle("/api/auditLogs.export", requireAuth(http.HandlerFunc(h.handleExport)))
}

// GET /api/auditLogs.list
func (h *AuditLogHandler) handleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var filter domain.AuditLogFilter
	// A malformed from/to or cursor is refused rather than dropped: answering
	// the unfiltered question would be worse than refusing the request.
	if err := filter.FromURLParams(r.URL.Query()); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	page, err := h.service.List(r.Context(), filter)
	if err != nil {
		h.writeError(w, err, "Failed to list audit logs")
		return
	}
	writeJSON(w, http.StatusOK, page)
}

// GET /api/auditLogs.get?id=...&workspace_id=...  (or scope=deployment)
func (h *AuditLogHandler) handleGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	query := r.URL.Query()
	id := query.Get("id")
	if id == "" {
		WriteJSONError(w, "id is required", http.StatusBadRequest)
		return
	}

	log, err := h.service.Get(r.Context(), query.Get("scope"), query.Get("workspace_id"), id)
	if err != nil {
		h.writeError(w, err, "Failed to get audit log")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"log": log})
}

// GET /api/auditLogs.actions — the catalogue, for the console's filter.
func (h *AuditLogHandler) handleActions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"actions":    h.service.Actions(),
		"categories": domain.AuditCategories,
	})
}

// POST /api/auditLogs.export — the first streaming download in the codebase.
//
// Authorisation runs inside the service before any byte is written, so a
// refusal is still a JSON error with a proper status. Once the header is out
// the only thing that can be done about a mid-stream failure is to log it
// and close the connection; the client sees a short file, not a 500.
func (h *AuditLogHandler) handleExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.ExportAuditLogsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if err := req.Validate(); err != nil {
		WriteJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	flusher, _ := w.(http.Flusher)
	flush := func() {
		if flusher != nil {
			flusher.Flush()
		}
	}

	// Headers are written by the writer below on the first byte, which the
	// service produces only after authorising. Set them now; they are discarded
	// with the buffer if the service refuses before writing.
	stream := &exportWriter{ResponseWriter: w, req: req}
	rows, truncated, err := h.service.Export(r.Context(), req, stream, func() {
		stream.start()
		flush()
	})
	if err != nil {
		if !stream.started {
			h.writeError(w, err, "Failed to export audit logs")
			return
		}
		h.logger.WithField("error", err.Error()).WithField("rows", rows).Error("Audit export interrupted mid-stream")
		// The client has a short file and a 200: the row must not say success.
		audit := domain.AuditFromContext(r.Context())
		audit.AddMetadata("rows", rows)
		audit.Fail("stream_interrupted")
		return
	}
	stream.start()
	flush()
	_ = truncated
}

// exportWriter delays the response headers until the first byte, so a
// refusal from the service can still answer with a JSON error, and names the
// file and the format once it does write.
type exportWriter struct {
	http.ResponseWriter
	req     domain.ExportAuditLogsRequest
	started bool
}

func (e *exportWriter) start() {
	if e.started {
		return
	}
	e.started = true
	scope := e.req.WorkspaceID
	if e.req.Scope == domain.AuditScopeDeployment && scope == "" {
		scope = domain.AuditScopeDeployment
	}
	header := e.Header()
	if e.req.Format == domain.AuditExportFormatNDJSON {
		header.Set("Content-Type", "application/x-ndjson")
	} else {
		header.Set("Content-Type", "text/csv; charset=utf-8")
	}
	header.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="audit-logs-%s-%s.%s"`, scope, time.Now().UTC().Format("20060102"), e.req.Format))
	header.Set("Cache-Control", "no-store")
	header.Set("X-Content-Type-Options", "nosniff")
	e.ResponseWriter.WriteHeader(http.StatusOK)
}

func (e *exportWriter) Write(b []byte) (int, error) {
	e.start()
	return e.ResponseWriter.Write(b)
}

func (h *AuditLogHandler) writeError(w http.ResponseWriter, err error, fallback string) {
	if writeServiceError(w, err, fallback) {
		return
	}
	if errors.Is(err, domain.ErrAuditLogNotFound) {
		WriteJSONError(w, "Audit log not found", http.StatusNotFound)
		return
	}
	h.logger.WithField("error", err.Error()).Error(fallback)
	WriteJSONError(w, fallback, http.StatusInternalServerError)
}
