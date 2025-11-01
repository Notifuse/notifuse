package http

import (
	"encoding/json"
	"net/http"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/http/middleware"
	"github.com/Notifuse/notifuse/pkg/logger")

// EmailHandler handles HTTP requests for email operations
type EmailHandler struct {
	emailService domain.EmailServiceInterface
	getJWTSecret func() ([]byte, error)
	logger       logger.Logger
	secretKey    string
}

// NewEmailHandler creates a new email handler
func NewEmailHandler(
	emailService domain.EmailServiceInterface,
	getJWTSecret func() ([]byte, error),
	logger logger.Logger,
	secretKey string,
) *EmailHandler {
	return &EmailHandler{
		emailService: emailService,
		getJWTSecret: getJWTSecret,
		logger:       logger,
		secretKey:    secretKey,
	}
}

// RegisterRoutes registers all workspace RPC-style routes with authentication middleware
func (h *EmailHandler) RegisterRoutes(mux *http.ServeMux) {
	// Create auth middleware
	authMiddleware := middleware.NewAuthMiddleware(h.getJWTSecret)
	requireAuth := authMiddleware.RequireAuth()

	// Register RPC-style endpoints with dot notation
	mux.Handle("/visit", http.HandlerFunc(h.handleClickRedirection))
	mux.Handle("/opens", http.HandlerFunc(h.handleOpens))
	mux.Handle("/api/email.testProvider", requireAuth(http.HandlerFunc(h.handleTestEmailProvider)))
}

// Add the handler for testEmailProvider
func (h *EmailHandler) handleTestEmailProvider(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.TestEmailProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.To == "" {
		WriteJSONError(w, "Missing recipient email (to)", http.StatusBadRequest)
		return
	}

	if req.WorkspaceID == "" {
		WriteJSONError(w, "Missing workspace ID", http.StatusBadRequest)
		return
	}

	err := h.emailService.TestEmailProvider(r.Context(), req.WorkspaceID, req.Provider, req.To)
	resp := domain.TestEmailProviderResponse{Success: err == nil}
	if err != nil {
		resp.Error = err.Error()
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *EmailHandler) handleClickRedirection(w http.ResponseWriter, r *http.Request) {
	// Get the message id (mid) and workspace id (wid) from the query parameters
	messageID := r.URL.Query().Get("mid")
	workspaceID := r.URL.Query().Get("wid")
	redirectTo := r.URL.Query().Get("url")

	// Check if URL is provided, show error if missing
	if redirectTo == "" {
		http.Error(w, "Missing redirect URL", http.StatusBadRequest)
		return
	}

	// redirect to the url if mid and wid are present
	if messageID == "" || workspaceID == "" {
		http.Redirect(w, r, redirectTo, http.StatusSeeOther)
		return
	}

	// increment the click count and ignore errors
	h.emailService.VisitLink(r.Context(), messageID, workspaceID)

	// redirect to the url
	http.Redirect(w, r, redirectTo, http.StatusSeeOther)
}

func (h *EmailHandler) handleOpens(w http.ResponseWriter, r *http.Request) {
	// Get the message id (mid) and workspace id (wid) from the query parameters
	messageID := r.URL.Query().Get("mid")
	workspaceID := r.URL.Query().Get("wid")

	// Check if URL is provided, show error if missing
	if messageID == "" || workspaceID == "" {
		http.Error(w, "Missing message ID or workspace ID", http.StatusBadRequest)
		return
	}

	// ignore errors
	h.emailService.OpenEmail(r.Context(), messageID, workspaceID)

	// return a transparent 1x1 pixel
	w.Header().Set("Content-Type", "image/png")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x08, 0x06, 0x00, 0x00, 0x00, 0x1F, 0x15, 0xC4, 0x89, 0x00, 0x00, 0x00, 0x0B, 0x49, 0x44, 0x41, 0x54, 0x08, 0xD7, 0x63, 0x60, 0x00, 0x00, 0x00, 0x02, 0x00, 0x01, 0xE2, 0x21, 0xBC, 0x33, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4E, 0x44, 0xAE, 0x42, 0x60, 0x82})

}
