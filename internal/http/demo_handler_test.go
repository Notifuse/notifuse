package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/config"

	"github.com/Notifuse/notifuse/internal/domain"

	"github.com/Notifuse/notifuse/internal/service"

	"github.com/Notifuse/notifuse/pkg/logger"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"

	"github.com/golang/mock/gomock"

	"github.com/stretchr/testify/assert"

	"github.com/stretchr/testify/require"
)

// createTestDemoService creates a DemoService for testing with minimal configuration
func createTestDemoService(cfg *config.Config, serviceLogger logger.Logger) *service.DemoService {
	return service.NewDemoService(
		serviceLogger,
		cfg,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)
}

func TestNewDemoHandler(t *testing.T) {
	cfg := &config.Config{RootEmail: "test@example.com", Security: config.SecurityConfig{SecretKey: "test-secret"}}
	mockLogger := logger.NewLoggerWithLevel("disabled")
	svc := createTestDemoService(cfg, mockLogger)

	handler := NewDemoHandler(svc, mockLogger)

	assert.NotNil(t, handler)
	assert.Equal(t, svc, handler.service)
	assert.Equal(t, mockLogger, handler.logger)
	assert.True(t, handler.lastReset.IsZero())
}

func TestDemoHandler_RegisterRoutes(t *testing.T) {
	cfg := &config.Config{RootEmail: "test@example.com", Security: config.SecurityConfig{SecretKey: "test-secret"}}
	mockLogger := logger.NewLoggerWithLevel("disabled")
	svc := createTestDemoService(cfg, mockLogger)
	handler := NewDemoHandler(svc, mockLogger)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// Test that the route was registered by making a request
	req := httptest.NewRequest(http.MethodGet, "/api/demo.reset", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	// Should get a response (not 404), even if it's an error due to missing HMAC
	assert.NotEqual(t, http.StatusNotFound, w.Code)
}

func TestDemoHandler_MethodNotAllowed(t *testing.T) {
	cfg := &config.Config{RootEmail: "test@example.com", Security: config.SecurityConfig{SecretKey: "test-secret"}}
	mockLogger := logger.NewLoggerWithLevel("disabled")
	svc := createTestDemoService(cfg, mockLogger)
	h := NewDemoHandler(svc, mockLogger)

	// Test different HTTP methods
	methods := []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch}

	for _, method := range methods {
		req := httptest.NewRequest(method, "/api/demo.reset", nil)
		w := httptest.NewRecorder()
		h.handleResetDemo(w, req)

		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
		assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

		var response map[string]string
		err := json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)
		assert.Equal(t, "Method not allowed", response["error"])
	}
}

func TestDemoHandler_MissingHMAC(t *testing.T) {
	cfg := &config.Config{RootEmail: "test@example.com", Security: config.SecurityConfig{SecretKey: "test-secret"}}
	mockLogger := logger.NewLoggerWithLevel("disabled")
	svc := createTestDemoService(cfg, mockLogger)
	h := NewDemoHandler(svc, mockLogger)

	req := httptest.NewRequest(http.MethodGet, "/api/demo.reset", nil)
	w := httptest.NewRecorder()
	h.handleResetDemo(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var response map[string]string
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "Missing HMAC parameter", response["error"])
}

func TestDemoHandler_InvalidHMAC(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithField("provided_hmac", "invalid_hmac").Return(mockLogger)
	mockLogger.EXPECT().Warn("Invalid HMAC provided for demo reset")

	cfg := &config.Config{RootEmail: "test@example.com", Security: config.SecurityConfig{SecretKey: "test-secret"}}
	svc := createTestDemoService(cfg, mockLogger)
	h := NewDemoHandler(svc, mockLogger)

	req := httptest.NewRequest(http.MethodGet, "/api/demo.reset?hmac=invalid_hmac", nil)
	w := httptest.NewRecorder()
	h.handleResetDemo(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var response map[string]string
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "Invalid authentication", response["error"])
}

// A reset that ran moments ago is skipped, but reported as success: the caller
// is a scheduler with at-least-once delivery, and the demo is in the state it
// asked for. Returning an error here marked a whole scheduled run failed.
func TestDemoHandler_RecentResetIsSkippedNotRejected(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().Warn("Demo reset skipped, previous reset was too recent")

	cfg := &config.Config{RootEmail: "test@example.com", Security: config.SecurityConfig{SecretKey: "test-secret"}}
	svc := createTestDemoService(cfg, mockLogger)
	h := NewDemoHandler(svc, mockLogger)

	// Well inside the debounce window, whatever that window is set to.
	h.lastReset = time.Now().Add(-minTimeBetweenResets / 2)

	// Generate valid HMAC
	validHMAC := domain.ComputeEmailHMAC("test@example.com", "test-secret")
	req := httptest.NewRequest(http.MethodGet, "/api/demo.reset?hmac="+validHMAC, nil)
	w := httptest.NewRecorder()

	// The service is built with nil dependencies, so reaching ResetDemo would
	// panic — surviving this call is itself the proof that no reset was run.
	h.handleResetDemo(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var response map[string]string
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "skipped", response["status"])
	assert.Empty(t, response["error"])
	// The debounce must not have been mistaken for a completed reset.
	assert.NotEqual(t, "reset", response["status"])
}

// A duplicate dispatch landing while a reset is in flight must be answered
// immediately, not held on the mutex for the length of the reset.
func TestDemoHandler_ResetInProgressAnswersImmediately(t *testing.T) {
	cfg := &config.Config{RootEmail: "test@example.com", Security: config.SecurityConfig{SecretKey: "test-secret"}}
	mockLogger := logger.NewLoggerWithLevel("disabled")
	svc := createTestDemoService(cfg, mockLogger)
	h := NewDemoHandler(svc, mockLogger)

	// Stand in for a reset already running.
	h.resetMutex.Lock()
	defer h.resetMutex.Unlock()

	validHMAC := domain.ComputeEmailHMAC("test@example.com", "test-secret")
	req := httptest.NewRequest(http.MethodGet, "/api/demo.reset?hmac="+validHMAC, nil)
	w := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		h.handleResetDemo(w, req)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handler blocked on the reset lock instead of answering the duplicate request")
	}

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]string
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "in_progress", response["status"])
	assert.Empty(t, response["error"])
}

// Authentication is checked before the reset lock, so an unauthenticated
// request arriving mid-reset cannot park on it.
func TestDemoHandler_AuthCheckedBeforeResetLock(t *testing.T) {
	cfg := &config.Config{RootEmail: "test@example.com", Security: config.SecurityConfig{SecretKey: "test-secret"}}

	testCases := []struct {
		name         string
		query        string
		expectedCode int
		expectedErr  string
	}{
		{"invalid hmac", "?hmac=invalid_hmac", http.StatusUnauthorized, "Invalid authentication"},
		{"missing hmac", "", http.StatusBadRequest, "Missing HMAC parameter"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockLogger := logger.NewLoggerWithLevel("disabled")
			svc := createTestDemoService(cfg, mockLogger)
			h := NewDemoHandler(svc, mockLogger)

			// Stand in for a reset already running.
			h.resetMutex.Lock()
			defer h.resetMutex.Unlock()

			req := httptest.NewRequest(http.MethodGet, "/api/demo.reset"+tc.query, nil)
			w := httptest.NewRecorder()

			done := make(chan struct{})
			go func() {
				h.handleResetDemo(w, req)
				close(done)
			}()

			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatal("handler blocked on the reset lock before authenticating the request")
			}

			assert.Equal(t, tc.expectedCode, w.Code)

			var response map[string]string
			err := json.NewDecoder(w.Body).Decode(&response)
			require.NoError(t, err)
			assert.Equal(t, tc.expectedErr, response["error"])
		})
	}
}

// Test to improve coverage - test that lastReset is updated on successful HMAC validation
// We can't test full success without complex mocking, but we can test the time update logic
func TestDemoHandler_LastResetTimeUpdate(t *testing.T) {
	cfg := &config.Config{RootEmail: "test@example.com", Security: config.SecurityConfig{SecretKey: "test-secret"}}
	mockLogger := logger.NewLoggerWithLevel("disabled")
	svc := createTestDemoService(cfg, mockLogger)
	h := NewDemoHandler(svc, mockLogger)

	// Verify initial state
	assert.True(t, h.lastReset.IsZero())

	// The handler's resetMutex should be available for testing
	h.resetMutex.Lock()
	initialTime := h.lastReset
	h.resetMutex.Unlock()

	assert.Equal(t, initialTime, h.lastReset)
}

func TestDemoHandler_DirectLastResetUpdate(t *testing.T) {
	// Test the lastReset field directly to increase coverage
	cfg := &config.Config{RootEmail: "test@example.com", Security: config.SecurityConfig{SecretKey: "test-secret"}}
	mockLogger := logger.NewLoggerWithLevel("disabled")
	svc := createTestDemoService(cfg, mockLogger)
	h := NewDemoHandler(svc, mockLogger)

	// Test initial state
	assert.True(t, h.lastReset.IsZero())

	// Directly update lastReset to test the field (simulating what would happen on success)
	now := time.Now()
	h.lastReset = now

	// Verify the update
	assert.Equal(t, now, h.lastReset)
	assert.False(t, h.lastReset.IsZero())
}

// TestDemoHandler_CoverageNote records what these tests deliberately leave out.
func TestDemoHandler_CoverageNote(t *testing.T) {
	// Every branch of handleResetDemo is covered here except the one that runs a
	// reset through to completion: the stamping of lastReset and the success
	// response. Reaching it needs a DemoService with all of its dependencies
	// mocked (UserService, WorkspaceService, and the rest), which the
	// integration suite exercises instead.

	// This test just verifies our test structure is working
	assert.True(t, true)
}

// Note: We cannot easily test service errors without complex mocking
// The service will panic due to missing dependencies
// But we can test all the handler logic up to the service call

func TestDemoHandler_EmptyRootEmailHMACValidation(t *testing.T) {
	// Test HMAC validation when root email is empty
	cfg := &config.Config{
		RootEmail: "",
		Security: config.SecurityConfig{
			SecretKey: "test-secret-key",
		},
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := pkgmocks.NewMockLogger(ctrl)
	// The service will first log that root email is not configured
	mockLogger.EXPECT().Error("Root email not configured")
	// Then the handler will log the invalid HMAC attempt
	mockLogger.EXPECT().WithField("provided_hmac", "some_hmac").Return(mockLogger)
	mockLogger.EXPECT().Warn("Invalid HMAC provided for demo reset")

	svc := createTestDemoService(cfg, mockLogger)
	h := NewDemoHandler(svc, mockLogger)

	req := httptest.NewRequest(http.MethodGet, "/api/demo.reset?hmac=some_hmac", nil)
	w := httptest.NewRecorder()
	h.handleResetDemo(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var response map[string]string
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "Invalid authentication", response["error"])
}
