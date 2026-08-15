package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"

	"github.com/golang/mock/gomock"

	"github.com/stretchr/testify/assert"

	"github.com/stretchr/testify/require"
)

// Test setup helper
func setupTest(t *testing.T) (*WorkspaceHandler, *mocks.MockWorkspaceServiceInterface, *http.ServeMux, []byte, *mocks.MockAuthService) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	workspaceSvc := mocks.NewMockWorkspaceServiceInterface(ctrl)
	authSvc := mocks.NewMockAuthService(ctrl)
	// Create key pair for testing
	jwtSecret := []byte("test-jwt-secret-key-for-testing-32bytes")
	passphrase := "test-passphrase"

	// Create and configure mock logger
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Set up expectations for logger methods that might be called during tests
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()

	handler := NewWorkspaceHandler(workspaceSvc, authSvc, func() ([]byte, error) { return jwtSecret, nil }, mockLogger, passphrase)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	return handler, workspaceSvc, mux, jwtSecret, authSvc
}

func TestWriteJSONError(t *testing.T) {
	testCases := []struct {
		name       string
		message    string
		statusCode int
	}{
		{
			name:       "bad_request",
			message:    "Bad request",
			statusCode: http.StatusBadRequest,
		},
		{
			name:       "unauthorized",
			message:    "Unauthorized access",
			statusCode: http.StatusUnauthorized,
		},
		{
			name:       "internal_server_error",
			message:    "Internal server error",
			statusCode: http.StatusInternalServerError,
		},
		{
			name:       "not_found",
			message:    "Resource not found",
			statusCode: http.StatusNotFound,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a response recorder
			w := httptest.NewRecorder()

			// Call the function
			WriteJSONError(w, tc.message, tc.statusCode)

			// Check status code
			assert.Equal(t, tc.statusCode, w.Code)

			// Check content type
			assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

			// Parse the response body
			var response map[string]string
			err := json.NewDecoder(w.Body).Decode(&response)
			require.NoError(t, err)

			// Check error message
			assert.Equal(t, tc.message, response["error"])
		})
	}
}

func TestWriteJSONError_EmptyMessage(t *testing.T) {
	// Create a response recorder
	w := httptest.NewRecorder()

	// Call with empty message
	WriteJSONError(w, "", http.StatusBadRequest)

	// Check status code
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Parse the response body
	var response map[string]string
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	// Check empty error message
	assert.Equal(t, "", response["error"])
}

func TestWriteJSONError_EncoderFailure(t *testing.T) {
	// Create a test response writer that fails after headers are written
	w := &failingResponseWriter{
		ResponseWriter: httptest.NewRecorder(),
		failOnWrite:    true,
	}

	// This should not panic even if encoding fails
	WriteJSONError(w, "Test message", http.StatusBadRequest)

	// Verify the status code was set before failure
	assert.Equal(t, http.StatusBadRequest, w.status)
	assert.Equal(t, "application/json", w.headers.Get("Content-Type"))
}

// A mock response writer that can be made to fail during Write
type failingResponseWriter struct {
	ResponseWriter http.ResponseWriter
	failOnWrite    bool
	status         int
	headers        http.Header
}

func (f *failingResponseWriter) Header() http.Header {
	if f.headers == nil {
		f.headers = make(http.Header)
	}
	return f.headers
}

func (f *failingResponseWriter) Write(b []byte) (int, error) {
	if f.failOnWrite {
		return 0, assert.AnError
	}
	return f.ResponseWriter.Write(b)
}

func (f *failingResponseWriter) WriteHeader(statusCode int) {
	f.status = statusCode
	f.ResponseWriter.WriteHeader(statusCode)
}

func TestWritePermissionError(t *testing.T) {
	denial := domain.NewPermissionError(
		domain.PermissionResourceAutomations,
		domain.PermissionTypeWrite,
		"Insufficient permissions: write access to automations required",
	)

	testCases := []struct {
		name            string
		err             error
		expectedHandled bool
		expectedStatus  int
		expectedMessage string
	}{
		{
			name:            "bare permission error",
			err:             denial,
			expectedHandled: true,
			expectedStatus:  http.StatusForbidden,
			expectedMessage: denial.Message,
		},
		{
			// Services wrap on the way up — the authenticate step that precedes every
			// permission check already does. A type assertion would miss this and the
			// caller would answer an opaque 500 to what is really a 403.
			name:            "wrapped permission error",
			err:             fmt.Errorf("failed to authenticate: %w", denial),
			expectedHandled: true,
			expectedStatus:  http.StatusForbidden,
			expectedMessage: denial.Message,
		},
		{
			name:            "twice-wrapped permission error",
			err:             fmt.Errorf("create failed: %w", fmt.Errorf("invalid template for channel email: %w", denial)),
			expectedHandled: true,
			expectedStatus:  http.StatusForbidden,
			expectedMessage: denial.Message,
		},
		{
			name:            "unrelated error is left to the caller",
			err:             errors.New("database connection lost"),
			expectedHandled: false,
		},
		{
			name:            "nil error is left to the caller",
			err:             nil,
			expectedHandled: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()

			handled := writePermissionError(w, tc.err)

			assert.Equal(t, tc.expectedHandled, handled)
			if !tc.expectedHandled {
				// Nothing written, so the caller's own mapping still applies.
				assert.Equal(t, http.StatusOK, w.Code)
				assert.Empty(t, w.Body.String())
				return
			}

			assert.Equal(t, tc.expectedStatus, w.Code)

			var response map[string]string
			require.NoError(t, json.NewDecoder(w.Body).Decode(&response))
			// The permission message itself, not the wrapping prose around it.
			assert.Equal(t, tc.expectedMessage, response["error"])
		})
	}
}
