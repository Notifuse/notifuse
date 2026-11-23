package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/http/middleware"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// maxFileStorageRequestBytes bounds the JSON body of the files.* endpoints. The largest is
// files.delete, with at most domain.MaxDeletedStorageKeys keys of 1 KiB each at worst.
const maxFileStorageRequestBytes = 2 << 20

// FileStorageHandler serves the workspace's bucket to the console: listings, folder
// markers, deletes and presigned upload URLs, so the bucket's secret stays on the server.
type FileStorageHandler struct {
	service      domain.FileStorageService
	logger       logger.Logger
	getJWTSecret func() ([]byte, error)
	isDemo       bool
}

// NewFileStorageHandler creates the files.* handler.
func NewFileStorageHandler(service domain.FileStorageService, getJWTSecret func() ([]byte, error), logger logger.Logger, isDemo bool) *FileStorageHandler {
	return &FileStorageHandler{
		service:      service,
		logger:       logger,
		getJWTSecret: getJWTSecret,
		isDemo:       isDemo,
	}
}

// RegisterRoutes registers the files.* routes and workspaces.testFileManager.
func (h *FileStorageHandler) RegisterRoutes(mux *http.ServeMux) {
	requireAuth := middleware.NewAuthMiddleware(h.getJWTSecret).RequireAuth()
	restrictedInDemo := middleware.RestrictedInDemo(h.isDemo)

	mux.Handle("/api/files.list", requireAuth(http.HandlerFunc(h.handleList)))
	mux.Handle("/api/files.presignUpload", requireAuth(http.HandlerFunc(h.handlePresignUpload)))
	mux.Handle("/api/files.createFolder", requireAuth(http.HandlerFunc(h.handleCreateFolder)))
	mux.Handle("/api/files.delete", requireAuth(http.HandlerFunc(h.handleDelete)))
	mux.Handle("/api/files.deleteFolder", requireAuth(http.HandlerFunc(h.handleDeleteFolder)))
	// Like workspaces.update, whose form it serves: an owner-only call to an endpoint the
	// caller chooses.
	mux.Handle("/api/workspaces.testFileManager", restrictedInDemo(requireAuth(http.HandlerFunc(h.handleTestSettings))))
}

// decode reads a JSON body of at most maxFileStorageRequestBytes into v. It answers the
// request and returns false when the body is too large (413) or not valid JSON (400).
func (h *FileStorageHandler) decode(w http.ResponseWriter, r *http.Request, v interface{}) bool {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return false
	}
	err := json.NewDecoder(middleware.MaxBytesReader(w, r, maxFileStorageRequestBytes)).Decode(v)
	if err == nil {
		return true
	}
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		WriteJSONError(w, "Request body too large", http.StatusRequestEntityTooLarge)
		return false
	}
	WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
	return false
}

// writeError answers a service error: the shared mapping first, then a provider failure
// as 502 with the provider's message.
func (h *FileStorageHandler) writeError(w http.ResponseWriter, err error, fallback string) {
	if writeServiceError(w, err, fallback) {
		return
	}
	var storageErr *domain.FileStorageError
	if errors.As(err, &storageErr) {
		WriteJSONError(w, storageErr.Error(), http.StatusBadGateway)
		return
	}
	h.logger.WithField("error", err.Error()).Error(fallback)
	WriteJSONError(w, fallback, http.StatusInternalServerError)
}

func requireWorkspaceID(w http.ResponseWriter, workspaceID string) bool {
	if workspaceID == "" {
		WriteJSONError(w, "workspace_id is required", http.StatusBadRequest)
		return false
	}
	return true
}

func (h *FileStorageHandler) handleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	query := r.URL.Query()
	workspaceID := query.Get("workspace_id")
	if !requireWorkspaceID(w, workspaceID) {
		return
	}
	result, err := h.service.List(r.Context(), workspaceID, query.Get("prefix"))
	if err != nil {
		h.writeError(w, err, "Failed to list files")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *FileStorageHandler) handlePresignUpload(w http.ResponseWriter, r *http.Request) {
	var req domain.PresignUploadRequest
	if !h.decode(w, r, &req) || !requireWorkspaceID(w, req.WorkspaceID) {
		return
	}
	result, err := h.service.PresignUpload(r.Context(), req)
	if err != nil {
		h.writeError(w, err, "Failed to prepare the upload")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *FileStorageHandler) handleCreateFolder(w http.ResponseWriter, r *http.Request) {
	var req struct {
		WorkspaceID string `json:"workspace_id"`
		Key         string `json:"key"`
	}
	if !h.decode(w, r, &req) || !requireWorkspaceID(w, req.WorkspaceID) {
		return
	}
	result, err := h.service.CreateFolder(r.Context(), req.WorkspaceID, req.Key)
	if err != nil {
		h.writeError(w, err, "Failed to create the folder")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *FileStorageHandler) handleDelete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		WorkspaceID string   `json:"workspace_id"`
		Keys        []string `json:"keys"`
	}
	if !h.decode(w, r, &req) || !requireWorkspaceID(w, req.WorkspaceID) {
		return
	}
	result, err := h.service.Delete(r.Context(), req.WorkspaceID, req.Keys)
	if err != nil {
		h.writeError(w, err, "Failed to delete files")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *FileStorageHandler) handleDeleteFolder(w http.ResponseWriter, r *http.Request) {
	var req struct {
		WorkspaceID string `json:"workspace_id"`
		Prefix      string `json:"prefix"`
	}
	if !h.decode(w, r, &req) || !requireWorkspaceID(w, req.WorkspaceID) {
		return
	}
	result, err := h.service.DeleteFolder(r.Context(), req.WorkspaceID, req.Prefix)
	if err != nil {
		h.writeError(w, err, "Failed to delete the folder")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *FileStorageHandler) handleTestSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		WorkspaceID string                     `json:"workspace_id"`
		Settings    domain.FileManagerSettings `json:"settings"`
	}
	if !h.decode(w, r, &req) || !requireWorkspaceID(w, req.WorkspaceID) {
		return
	}
	if err := h.service.TestSettings(r.Context(), req.WorkspaceID, req.Settings); err != nil {
		h.writeError(w, err, "Failed to test the file storage settings")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}
