package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"aidanwoods.dev/go-paseto"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test setup helper
func setupTest(t *testing.T) (*WorkspaceHandler, *mocks.MockWorkspaceServiceInterface, *http.ServeMux, paseto.V4AsymmetricSecretKey, string) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	workspaceSvc := mocks.NewMockWorkspaceServiceInterface(ctrl)
	authSvc := mocks.NewMockAuthService(ctrl)
	// Create key pair for testing
	secretKey := paseto.NewV4AsymmetricSecretKey()
	publicKey := secretKey.Public()
	passphrase := "test-passphrase"
	mockLogger := new(pkgmocks.MockLogger)
	handler := NewWorkspaceHandler(workspaceSvc, authSvc, publicKey, mockLogger, passphrase)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	return handler, workspaceSvc, mux, secretKey, passphrase
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
