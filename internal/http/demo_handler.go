package http

import (
	"net/http"
	"sync"
	"time"

	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// minTimeBetweenResets debounces the destructive half of a reset. A reset drops
// every workspace before reseeding, so two in quick succession are never worth
// the churn.
const minTimeBetweenResets = 5 * time.Minute

// DemoHandler handles HTTP requests for demo operations
type DemoHandler struct {
	service *service.DemoService
	logger  logger.Logger
	// lastReset is read and written only by the goroutine holding resetMutex.
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

// handleResetDemo handles the GET request to reset demo data.
//
// Both skip paths answer 200 rather than an error, because the caller is a
// scheduler whose delivery is at-least-once: it will occasionally dispatch a
// second attempt for a run already in flight, and a reset takes minutes now
// that most of it is seeding web analytics. "A reset is already running" and
// "one finished moments ago" both mean the demo is fresh, which is the whole of
// what the caller asked for — answering an error there marks the scheduled run
// failed for a job that had just done exactly its work, and buries a real
// failure among false ones. A reset that actually fails still answers 500.
func (h *DemoHandler) handleResetDemo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Authenticated before the lock is touched, so an unauthenticated request
	// arriving mid-reset is turned away immediately rather than parked for the
	// minutes the reset holds it.
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

	// TryLock, never Lock. A blocking acquire is what turned a duplicate
	// dispatch into a failure: the second request sat on the mutex for the whole
	// reset, then read a lastReset stamped microseconds before it got in and
	// reported a reset that had just succeeded as too frequent.
	if !h.resetMutex.TryLock() {
		h.logger.Warn("Demo reset already in progress, skipping duplicate request")
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"message": "Demo reset already in progress",
			"status":  "in_progress",
		})
		return
	}
	defer h.resetMutex.Unlock()

	if time.Since(h.lastReset) < minTimeBetweenResets {
		h.logger.Warn("Demo reset skipped, previous reset was too recent")
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"message": "Demo was reset too recently",
			"status":  "skipped",
		})
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
		"status":  "reset",
	})
}
