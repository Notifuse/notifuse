package http

import (
	"context"
	"errors"
	"fmt"
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

// demoOperations is what the handler needs from service.DemoService.
type demoOperations interface {
	VerifyRootEmailHMAC(providedHMAC string) bool
	ResetDemo(ctx context.Context) error
	SeedFixtures(ctx context.Context) error
}

// DemoHandler handles HTTP requests for demo operations
type DemoHandler struct {
	service demoOperations
	logger  logger.Logger
	// lastReset and lastSeed are read and written only by the goroutine holding
	// resetMutex. One mutex for both operations: a seed and a reset both rebuild the
	// demo workspace and must never run together.
	lastReset  demoRun
	lastSeed   demoRun
	resetMutex sync.Mutex
}

// demoRun is the last attempt of one operation. The debounce counts attempts, not
// successes: a deterministic failure (an unlicensed template, a broken list) would
// otherwise rerun the whole operation, and a reset deletes every workspace, on every
// retry of the scheduler. A failed attempt keeps its answer, which retries inside the
// window receive again without anything running.
type demoRun struct {
	at      time.Time
	failure *demoFailure
}

type demoFailure struct {
	status  int
	message string
}

// NewDemoHandler creates a new demo handler
func NewDemoHandler(service demoOperations, logger logger.Logger) *DemoHandler {
	return &DemoHandler{
		service: service,
		logger:  logger,
	}
}

// RegisterRoutes registers the demo HTTP endpoints.
//
// demo.seed is registered on every environment: it replaces only the workspace it
// created itself (see service.DemoService.SeedFixtures), so a preview running as
// production can carry the fixtures. demo.reset deletes every workspace on the
// instance and is registered only when resetEnabled, which app.go ties to a
// non-production environment.
func (h *DemoHandler) RegisterRoutes(mux *http.ServeMux, resetEnabled bool) {
	mux.HandleFunc("/api/demo.seed", h.handleSeedFixtures)
	if resetEnabled {
		mux.HandleFunc("/api/demo.reset", h.handleResetDemo)
	}
}

// authorize answers the request itself and returns false unless it is a GET signed
// with the root email HMAC. It runs before the lock is touched, so an
// unauthenticated request arriving mid-reset is turned away immediately rather
// than parked for the minutes the reset holds it.
func (h *DemoHandler) authorize(w http.ResponseWriter, r *http.Request, operation string) bool {
	if r.Method != http.MethodGet {
		WriteJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return false
	}

	providedHMAC := r.URL.Query().Get("hmac")
	if providedHMAC == "" {
		WriteJSONError(w, "Missing HMAC parameter", http.StatusBadRequest)
		return false
	}

	if !h.service.VerifyRootEmailHMAC(providedHMAC) {
		h.logger.WithField("provided_hmac", providedHMAC).Warn("Invalid HMAC provided for demo " + operation)
		WriteJSONError(w, "Invalid authentication", http.StatusUnauthorized)
		return false
	}
	return true
}

// demoOperation describes one debounced, mutually exclusive demo operation.
type demoOperation struct {
	name          string // "reset" or "seed", in log lines
	last          *demoRun
	run           func(ctx context.Context) error
	failureStatus func(err error) int
	failurePrefix string
	skipped       string
	done          string
	doneStatus    string
}

// handleSeedFixtures handles the GET request that replaces the fixtures workspace.
// It answers like handleResetDemo, for the same scheduler; a workspace with the
// fixtures ID that the seed did not create is a 409, with nothing deleted.
func (h *DemoHandler) handleSeedFixtures(w http.ResponseWriter, r *http.Request) {
	if !h.authorize(w, r, "seed") {
		return
	}
	h.runExclusive(w, r, demoOperation{
		name: "seed",
		last: &h.lastSeed,
		run:  h.service.SeedFixtures,
		failureStatus: func(err error) int {
			if errors.Is(err, service.ErrDemoWorkspaceNotFixtures) {
				return http.StatusConflict
			}
			return http.StatusInternalServerError
		},
		failurePrefix: "Failed to seed demo fixtures",
		skipped:       "Demo fixtures were seeded too recently",
		done:          "Demo fixtures seeded successfully",
		doneStatus:    "seeded",
	})
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
// failure among false ones. A reset that actually fails still answers 500, and
// so does every retry inside the debounce window, without resetting again.
func (h *DemoHandler) handleResetDemo(w http.ResponseWriter, r *http.Request) {
	if !h.authorize(w, r, "reset") {
		return
	}
	h.runExclusive(w, r, demoOperation{
		name:          "reset",
		last:          &h.lastReset,
		run:           h.service.ResetDemo,
		failureStatus: func(error) int { return http.StatusInternalServerError },
		failurePrefix: "Failed to reset demo data",
		skipped:       "Demo was reset too recently",
		done:          "Demo data reset successfully",
		doneStatus:    "reset",
	})
}

// runExclusive runs op unless a reset or seed is in flight or op was attempted
// within minTimeBetweenResets.
func (h *DemoHandler) runExclusive(w http.ResponseWriter, r *http.Request, op demoOperation) {
	// TryLock, never Lock. A blocking acquire is what turned a duplicate
	// dispatch into a failure: the second request sat on the mutex for the whole
	// reset, then read a lastReset stamped microseconds before it got in and
	// reported a reset that had just succeeded as too frequent.
	if !h.resetMutex.TryLock() {
		h.logger.Warn("Demo reset or seed already in progress, skipping duplicate " + op.name + " request")
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"message": "Demo reset or seed already in progress",
			"status":  "in_progress",
		})
		return
	}
	defer h.resetMutex.Unlock()

	if since := time.Since(op.last.at); since < minTimeBetweenResets {
		if failure := op.last.failure; failure != nil {
			h.logger.Warn("Demo " + op.name + " not retried, previous attempt failed too recently")
			WriteJSONError(w, fmt.Sprintf("%s (previous attempt %s ago; not retried before %s)",
				failure.message, since.Round(time.Second), op.last.at.Add(minTimeBetweenResets).Format(time.RFC3339)),
				failure.status)
			return
		}
		h.logger.Warn("Demo " + op.name + " skipped, previous " + op.name + " was too recent")
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"message": op.skipped,
			"status":  "skipped",
		})
		return
	}

	*op.last = demoRun{at: time.Now().UTC()}
	if err := op.run(r.Context()); err != nil {
		h.logger.WithField("error", err.Error()).Error(op.failurePrefix)
		failure := &demoFailure{status: op.failureStatus(err), message: op.failurePrefix + ": " + err.Error()}
		op.last.failure = failure
		WriteJSONError(w, failure.message, failure.status)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": op.done,
		"status":  op.doneStatus,
	})
}
