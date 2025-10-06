package http

import (
	"net/http"
	"sync"
	"time"

	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// DemoHandler handles HTTP requests for demo operations
type DemoHandler struct {
	service    *service.DemoService
	logger     logger.Logger
	lastReset  time.Time
	resetMutex sync.Mutex
}

// NewDemoHandler creates a new demo handler
func NewDemoHandler(service *service.DemoService, logger logger.Logger) *DemoHandler {
	return &DemoHandler{
		service: service,
		logger:  logger,
	}
}

// RegisterRoutes registers the demo HTTP endpoints
func (h *DemoHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/demo.reset", h.handleResetDemo)
}

// handleResetDemo handles the GET request to reset demo data with rate limiting
func (h *DemoHandler) handleResetDemo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Rate limiting: enforce minimum time between resets (5 minutes)
	h.resetMutex.Lock()
	defer h.resetMutex.Unlock()

	if time.Since(h.lastReset) < 5*time.Minute {
		h.logger.Warn("Reset request rejected due to rate limiting")
		WriteJSONError(w, "Reset too frequent. Please wait 5 minutes between resets.", http.StatusTooManyRequests)
		return
	}

	// Get HMAC from query string
	providedHMAC := r.URL.Query().Get("hmac")
	if providedHMAC == "" {
		WriteJSONError(w, "Missing HMAC parameter", http.StatusBadRequest)
		return
	}

	// Verify HMAC using the service
	if !h.service.VerifyRootEmailHMAC(providedHMAC) {
		h.logger.WithField("provided_hmac", providedHMAC).Warn("Invalid HMAC provided for demo reset")
		WriteJSONError(w, "Invalid authentication", http.StatusUnauthorized)
		return
	}

	// Reset demo data
	if err := h.service.ResetDemo(r.Context()); err != nil {
		h.logger.WithField("error", err.Error()).Error("Failed to reset demo data")
		WriteJSONError(w, "Failed to reset demo data", http.StatusInternalServerError)
		return
	}

	// Update last reset time
	h.lastReset = time.Now().UTC()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Demo data reset successfully",
	})
}
