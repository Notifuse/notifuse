package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// fakeDemoOperations stands in for service.DemoService so the seed handler can be
// driven through to the service call and its outcomes.
type fakeDemoOperations struct {
	validHMAC string
	seedErr   error
	seeds     int
	resets    int
	// seeding, when set, is closed once SeedFixtures has started and SeedFixtures
	// then waits on release.
	seeding chan struct{}
	release chan struct{}
}

func (f *fakeDemoOperations) VerifyRootEmailHMAC(providedHMAC string) bool {
	return providedHMAC == f.validHMAC
}

func (f *fakeDemoOperations) ResetDemo(context.Context) error {
	f.resets++
	return nil
}

func (f *fakeDemoOperations) SeedFixtures(context.Context) error {
	f.seeds++
	if f.seeding != nil {
		close(f.seeding)
		<-f.release
	}
	return f.seedErr
}

func callDemo(h *DemoHandler, handle func(http.ResponseWriter, *http.Request), method, query string) (int, map[string]string) {
	w := httptest.NewRecorder()
	handle(w, httptest.NewRequest(method, "/api/demo.seed"+query, nil))
	var body map[string]string
	_ = json.NewDecoder(w.Body).Decode(&body)
	return w.Code, body
}

func TestDemoHandler_Seed_Authorization(t *testing.T) {
	testCases := []struct {
		name     string
		method   string
		query    string
		wantCode int
		wantErr  string
	}{
		{"wrong method", http.MethodPost, "?hmac=good", http.StatusMethodNotAllowed, "Method not allowed"},
		{"missing hmac", http.MethodGet, "", http.StatusBadRequest, "Missing HMAC parameter"},
		{"invalid hmac", http.MethodGet, "?hmac=bad", http.StatusUnauthorized, "Invalid authentication"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ops := &fakeDemoOperations{validHMAC: "good"}
			h := NewDemoHandler(ops, logger.NewLoggerWithLevel("disabled"))

			code, body := callDemo(h, h.handleSeedFixtures, tc.method, tc.query)

			assert.Equal(t, tc.wantCode, code)
			assert.Equal(t, tc.wantErr, body["error"])
			assert.Zero(t, ops.seeds, "an unauthorized request must not seed")
		})
	}
}

func TestDemoHandler_Seed_Success(t *testing.T) {
	ops := &fakeDemoOperations{validHMAC: "good"}
	h := NewDemoHandler(ops, logger.NewLoggerWithLevel("disabled"))

	code, body := callDemo(h, h.handleSeedFixtures, http.MethodGet, "?hmac=good")

	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, "seeded", body["status"])
	assert.Equal(t, 1, ops.seeds)
	assert.False(t, h.lastSeed.at.IsZero(), "a completed seed starts the debounce")
	assert.Nil(t, h.lastSeed.failure)
	assert.True(t, h.lastReset.at.IsZero(), "a seed is not a reset")
}

// The seed debounce is its own: a reset moments ago does not skip a seed, and a
// recent seed is skipped with a 200, like the reset, for the same scheduler.
func TestDemoHandler_Seed_Debounce(t *testing.T) {
	t.Run("recent seed is skipped", func(t *testing.T) {
		ops := &fakeDemoOperations{validHMAC: "good"}
		h := NewDemoHandler(ops, logger.NewLoggerWithLevel("disabled"))
		h.lastSeed = demoRun{at: time.Now().Add(-minTimeBetweenResets / 2)}

		code, body := callDemo(h, h.handleSeedFixtures, http.MethodGet, "?hmac=good")

		assert.Equal(t, http.StatusOK, code)
		assert.Equal(t, "skipped", body["status"])
		assert.Zero(t, ops.seeds)
	})

	t.Run("recent reset does not skip a seed", func(t *testing.T) {
		ops := &fakeDemoOperations{validHMAC: "good"}
		h := NewDemoHandler(ops, logger.NewLoggerWithLevel("disabled"))
		h.lastReset = demoRun{at: time.Now()}

		code, body := callDemo(h, h.handleSeedFixtures, http.MethodGet, "?hmac=good")

		assert.Equal(t, http.StatusOK, code)
		assert.Equal(t, "seeded", body["status"])
		assert.Equal(t, 1, ops.seeds)
	})
}

func TestDemoHandler_Seed_Errors(t *testing.T) {
	t.Run("workspace not created by the seed is a conflict", func(t *testing.T) {
		ops := &fakeDemoOperations{validHMAC: "good", seedErr: service.ErrDemoWorkspaceNotFixtures}
		h := NewDemoHandler(ops, logger.NewLoggerWithLevel("disabled"))

		code, body := callDemo(h, h.handleSeedFixtures, http.MethodGet, "?hmac=good")

		assert.Equal(t, http.StatusConflict, code)
		assert.Contains(t, body["error"], "nothing was deleted")
	})

	t.Run("wrapped conflict is still a conflict", func(t *testing.T) {
		ops := &fakeDemoOperations{validHMAC: "good", seedErr: fmt.Errorf("seed: %w", service.ErrDemoWorkspaceNotFixtures)}
		h := NewDemoHandler(ops, logger.NewLoggerWithLevel("disabled"))

		code, _ := callDemo(h, h.handleSeedFixtures, http.MethodGet, "?hmac=good")

		assert.Equal(t, http.StatusConflict, code)
	})

	t.Run("any other failure is a 500 that names it", func(t *testing.T) {
		ops := &fakeDemoOperations{validHMAC: "good", seedErr: errors.New("template_i18n is not licensed")}
		h := NewDemoHandler(ops, logger.NewLoggerWithLevel("disabled"))

		code, body := callDemo(h, h.handleSeedFixtures, http.MethodGet, "?hmac=good")

		assert.Equal(t, http.StatusInternalServerError, code)
		assert.Contains(t, body["error"], "template_i18n is not licensed",
			"the scheduler's log must say why the seed failed")
	})
}

// A failed attempt starts the debounce too: retries inside the window get the same
// answer again and run nothing. Otherwise a deterministic failure reruns the whole
// operation on every retry of the scheduler, and for a reset that means deleting
// every workspace each time.
func TestDemoHandler_FailedAttemptIsReplayedNotRerun(t *testing.T) {
	testCases := []struct {
		name     string
		err      error
		wantCode int
	}{
		{"conflict", service.ErrDemoWorkspaceNotFixtures, http.StatusConflict},
		{"failure", errors.New("template_i18n is not licensed"), http.StatusInternalServerError},
	}
	for _, tc := range testCases {
		t.Run("seed "+tc.name, func(t *testing.T) {
			ops := &fakeDemoOperations{validHMAC: "good", seedErr: tc.err}
			h := NewDemoHandler(ops, logger.NewLoggerWithLevel("disabled"))

			first, firstBody := callDemo(h, h.handleSeedFixtures, http.MethodGet, "?hmac=good")
			retry, retryBody := callDemo(h, h.handleSeedFixtures, http.MethodGet, "?hmac=good")

			assert.Equal(t, tc.wantCode, first)
			assert.Equal(t, tc.wantCode, retry, "the retry keeps the failure, it is not reported as skipped")
			assert.Contains(t, retryBody["error"], firstBody["error"])
			assert.Contains(t, retryBody["error"], "not retried before")
			assert.Equal(t, 1, ops.seeds, "the retry ran nothing")
		})
	}

	t.Run("reset failure", func(t *testing.T) {
		ops := &failingResetOperations{fakeDemoOperations: fakeDemoOperations{validHMAC: "good"}, resetErr: errors.New("lists table unavailable")}
		h := NewDemoHandler(ops, logger.NewLoggerWithLevel("disabled"))

		first, firstBody := callDemo(h, h.handleResetDemo, http.MethodGet, "?hmac=good")
		retry, _ := callDemo(h, h.handleResetDemo, http.MethodGet, "?hmac=good")

		assert.Equal(t, http.StatusInternalServerError, first)
		assert.Contains(t, firstBody["error"], "lists table unavailable")
		assert.Equal(t, http.StatusInternalServerError, retry)
		assert.Equal(t, 1, ops.resets, "a failing reset is not rerun, so it deletes the workspaces once")
	})

	t.Run("after the window the operation runs again", func(t *testing.T) {
		ops := &fakeDemoOperations{validHMAC: "good", seedErr: errors.New("boom")}
		h := NewDemoHandler(ops, logger.NewLoggerWithLevel("disabled"))

		callDemo(h, h.handleSeedFixtures, http.MethodGet, "?hmac=good")
		h.lastSeed.at = time.Now().Add(-minTimeBetweenResets - time.Second)
		ops.seedErr = nil
		code, body := callDemo(h, h.handleSeedFixtures, http.MethodGet, "?hmac=good")

		assert.Equal(t, http.StatusOK, code)
		assert.Equal(t, "seeded", body["status"])
		assert.Equal(t, 2, ops.seeds)
		assert.Nil(t, h.lastSeed.failure, "a success clears the recorded failure")
	})
}

type failingResetOperations struct {
	fakeDemoOperations
	resetErr error
}

func (f *failingResetOperations) ResetDemo(context.Context) error {
	f.resets++
	return f.resetErr
}

// Seed and reset share one lock: either one running answers the other with
// in_progress at once, without running it.
func TestDemoHandler_SeedAndResetExcludeEachOther(t *testing.T) {
	waitFor := func(t *testing.T, done <-chan struct{}, what string) {
		t.Helper()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal(what)
		}
	}

	t.Run("reset during a seed", func(t *testing.T) {
		ops := &fakeDemoOperations{validHMAC: "good", seeding: make(chan struct{}), release: make(chan struct{})}
		h := NewDemoHandler(ops, logger.NewLoggerWithLevel("disabled"))

		seedDone := make(chan struct{})
		go func() {
			callDemo(h, h.handleSeedFixtures, http.MethodGet, "?hmac=good")
			close(seedDone)
		}()
		waitFor(t, ops.seeding, "the seed never started")

		resetDone := make(chan struct{})
		var code int
		var body map[string]string
		go func() {
			code, body = callDemo(h, h.handleResetDemo, http.MethodGet, "?hmac=good")
			close(resetDone)
		}()
		waitFor(t, resetDone, "the reset blocked on the lock held by the seed")

		assert.Equal(t, http.StatusOK, code)
		assert.Equal(t, "in_progress", body["status"])
		assert.Zero(t, ops.resets)

		close(ops.release)
		waitFor(t, seedDone, "the seed never finished")
	})

	t.Run("seed during a reset", func(t *testing.T) {
		ops := &fakeDemoOperations{validHMAC: "good"}
		h := NewDemoHandler(ops, logger.NewLoggerWithLevel("disabled"))
		h.resetMutex.Lock() // a reset in flight
		defer h.resetMutex.Unlock()

		done := make(chan struct{})
		var code int
		var body map[string]string
		go func() {
			code, body = callDemo(h, h.handleSeedFixtures, http.MethodGet, "?hmac=good")
			close(done)
		}()
		waitFor(t, done, "the seed blocked on the lock held by the reset")

		assert.Equal(t, http.StatusOK, code)
		assert.Equal(t, "in_progress", body["status"])
		assert.Zero(t, ops.seeds)
	})
}

// The real service satisfies the handler's interface; app.go relies on it.
var _ demoOperations = (*service.DemoService)(nil)
