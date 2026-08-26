package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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
	return setupWorkspaceTest(t, false)
}

// setupDemoTest builds the handler the way app.go builds it on a demo instance:
// the flag goes through the constructor, so the routes are wired from it.
func setupDemoTest(t *testing.T) (*WorkspaceHandler, *mocks.MockWorkspaceServiceInterface, *http.ServeMux, []byte, *mocks.MockAuthService) {
	return setupWorkspaceTest(t, true)
}

func setupWorkspaceTest(t *testing.T, isDemo bool) (*WorkspaceHandler, *mocks.MockWorkspaceServiceInterface, *http.ServeMux, []byte, *mocks.MockAuthService) {
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

	handler := NewWorkspaceHandler(workspaceSvc, authSvc, func() ([]byte, error) { return jwtSecret, nil }, mockLogger, passphrase, isDemo)

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
			// The missing grant, named rather than left to prose parsing.
			assert.Equal(t, string(domain.PermissionResourceAutomations), response["resource"])
			assert.Equal(t, string(domain.PermissionTypeWrite), response["permission"])
		})
	}
}

func TestWriteServiceError(t *testing.T) {
	denial := domain.NewPermissionError(
		domain.PermissionResourceWorkspace,
		domain.PermissionTypeRead,
		"Insufficient permissions: read access to workspace required",
	)

	testCases := []struct {
		name            string
		err             error
		expectedHandled bool
		expectedStatus  int
		expectedMessage string
	}{
		{
			name:            "permission error",
			err:             denial,
			expectedHandled: true,
			expectedStatus:  http.StatusForbidden,
			expectedMessage: denial.Message,
		},
		{
			name:            "wrapped permission error",
			err:             fmt.Errorf("failed to get workspace: %w", denial),
			expectedHandled: true,
			expectedStatus:  http.StatusForbidden,
			expectedMessage: denial.Message,
		},
		{
			name:            "workspace not found",
			err:             &domain.ErrWorkspaceNotFound{WorkspaceID: "workspace-123"},
			expectedHandled: true,
			expectedStatus:  http.StatusNotFound,
			expectedMessage: "Workspace not found",
		},
		{
			name:            "unauthorized",
			err:             &domain.ErrUnauthorized{Message: "only owners may do that"},
			expectedHandled: true,
			expectedStatus:  http.StatusForbidden,
			expectedMessage: "only owners may do that",
		},
		{
			// The authenticate step that precedes every check wraps on the way up.
			name:            "wrapped unauthorized",
			err:             fmt.Errorf("failed to authenticate user: %w", &domain.ErrUnauthorized{Message: "only owners may do that"}),
			expectedHandled: true,
			expectedStatus:  http.StatusForbidden,
			expectedMessage: "only owners may do that",
		},
		{
			name:            "unauthorized without a message of its own",
			err:             &domain.ErrUnauthorized{},
			expectedHandled: true,
			expectedStatus:  http.StatusForbidden,
			expectedMessage: "You are not allowed to do that",
		},
		{
			name:            "user not in workspace",
			err:             fmt.Errorf("failed to authenticate user: %w", domain.ErrUserNotInWorkspace),
			expectedHandled: true,
			expectedStatus:  http.StatusForbidden,
			expectedMessage: "You do not have access to this workspace",
		},
		{
			// Revoking an API key deletes its user row and nothing else: the
			// token stays signed and valid for ten years, so a deleted user is
			// the only shape a revoked key ever takes. Untyped, it collapsed to
			// 500 at every handler — which tells an API client its server is
			// broken rather than that its credential is dead, and specifically
			// stops Zapier raising the reconnect prompt only a 401 triggers.
			name:            "revoked api key",
			err:             domain.ErrAPIKeyRevoked,
			expectedHandled: true,
			expectedStatus:  http.StatusUnauthorized,
			expectedMessage: "API key has been revoked",
		},
		{
			name:            "wrapped revoked api key",
			err:             fmt.Errorf("failed to authenticate user: %w", domain.ErrAPIKeyRevoked),
			expectedHandled: true,
			expectedStatus:  http.StatusUnauthorized,
			expectedMessage: "API key has been revoked",
		},
		{
			// Deleting a row that is already gone has reached the state the
			// caller asked for. The Zapier app deletes its subscription on every
			// Zap turn-off and the console lets a user delete it by hand first,
			// so a 500 here made turning a Zap off fail permanently.
			name:            "webhook subscription already deleted",
			err:             fmt.Errorf("webhook subscription sub-1: %w", domain.ErrWebhookSubscriptionNotFound),
			expectedHandled: true,
			expectedStatus:  http.StatusNotFound,
			expectedMessage: "Webhook subscription not found",
		},
		{
			// A Zap outliving the list it points at is the ordinary case, not an
			// exotic one: the dropdown value is stored in the Zap and never
			// re-resolved.
			name:            "list not found names the list",
			err:             fmt.Errorf("subscribing: %w", &domain.ErrListNotFound{Message: "list list123 not found"}),
			expectedHandled: true,
			expectedStatus:  http.StatusNotFound,
			expectedMessage: "list list123 not found",
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

			handled := writeServiceError(w, tc.err, "You are not allowed to do that")

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
			assert.Equal(t, tc.expectedMessage, response["error"])
		})
	}
}

// TestWritePermissionError_RevokedAPIKey pins WHERE the revoked-key mapping
// lives, not merely that it exists.
//
// Most handlers call writePermissionError and never writeServiceError —
// transactional notifications, custom events, broadcasts, automations, the
// contact timeline, message history, blog themes and web analytics all do. While
// the mapping sat one level up in writeServiceError, every one of them still
// answered a dead credential with a 500, which is the case that matters: those
// are the endpoints an API key actually calls. Asserting through this helper is
// what keeps the mapping at the level all of them share.
func TestWritePermissionError_RevokedAPIKey(t *testing.T) {
	// The shape AuthenticateUserFromContext really produces: revoking a key deletes
	// its user row, so the lookup failure it wraps is a plain "user not found".
	authFailure := fmt.Errorf("%w: %w", domain.ErrAPIKeyRevoked, errors.New("user not found"))

	testCases := []struct {
		name string
		err  error
	}{
		{name: "bare sentinel", err: domain.ErrAPIKeyRevoked},
		{name: "as the auth service builds it", err: authFailure},
		{
			name: "wrapped again on the way up",
			err:  fmt.Errorf("failed to authenticate user: %w", authFailure),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()

			require.True(t, writePermissionError(w, tc.err))

			// 401 and nothing else: a client cannot tell a 403 from a permission it
			// might be granted, and Zapier raises its reconnect prompt on 401 alone.
			assert.Equal(t, http.StatusUnauthorized, w.Code)

			body := w.Body.String()
			var response map[string]string
			require.NoError(t, json.Unmarshal([]byte(body), &response))
			assert.Equal(t, "API key has been revoked", response["error"])

			// The wrapped internal wording stays inside the process — it names how
			// revocation is implemented, and it is what a substring-matching handler
			// misread as a client mistake.
			assert.NotContains(t, body, "user not found")
		})
	}
}

// TestTransactionalSendRevokedKeyIsAnAuthFailure exercises the worst-affected of
// the handlers that reach for writePermissionError and nothing else.
//
// handleSend classifies whatever that helper declines by substring, and a revoked
// key arrives as "api key has been revoked: user not found" — so the "not found"
// arm caught it and answered 400 with that internal string in the response body.
// 400 is the one answer a Zap can act on least: it raises no reconnect prompt,
// triggers no retry, and reports the account's own request as malformed.
//
// The logger mock carries no expectations on purpose: the revoked key must be
// answered before the error-level logging further down, so any call to it fails
// this test.
func TestTransactionalSendRevokedKeyIsAnAuthFailure(t *testing.T) {
	mockService, _, handler := setupTransactionalHandlerTest(t)

	mockService.EXPECT().
		SendNotification(gomock.Any(), "ws1", gomock.Any()).
		Return("", fmt.Errorf("%w: %w", domain.ErrAPIKeyRevoked, errors.New("user not found")))

	// Raw JSON rather than a typed literal: this is what an integration puts on
	// the wire, and the request has to survive Validate to reach the service at all.
	body := `{"workspace_id":"ws1","notification":{"id":"welcome","contact":{"email":"user@example.com"},"channels":["email"]}}`
	req := httptest.NewRequest(http.MethodPost, "/api/transactional.send", strings.NewReader(body))
	w := httptest.NewRecorder()

	handler.handleSend(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)

	responseBody := w.Body.String()
	var response map[string]string
	require.NoError(t, json.Unmarshal([]byte(responseBody), &response))
	assert.Equal(t, "API key has been revoked", response["error"])
	assert.NotContains(t, responseBody, "user not found", "an internal error string reached the caller")
}

// TestRedactWorkspaceForCaller covers the one credential Workspace.Redact
// deliberately keeps.
//
// The console builds an S3 client in the browser from Settings.FileManager
// .SecretKey and talks to the bucket directly, so blanking it for everyone breaks
// the file manager rather than hardening anything. But an API key authenticates
// the very same endpoints a session does, and no integration has any use for a
// live bucket credential — while the platforms they run on log whole response
// bodies. The caller decides, so the console keeps working and the key gets
// nothing.
//
// Asserted against marshalled bytes, not fields: what leaks is what goes on the
// wire.
func TestRedactWorkspaceForCaller(t *testing.T) {
	const fileManagerSecret = "SENTINEL-live-bucket-secret"
	const smtpPassword = "SENTINEL-smtp-password"

	newWorkspace := func() *domain.Workspace {
		ws := &domain.Workspace{
			ID:   "ws1",
			Name: "Acme",
			Integrations: domain.Integrations{{
				ID:   "int-1",
				Type: domain.IntegrationTypeEmail,
				EmailProvider: domain.EmailProvider{
					Kind: domain.EmailProviderKindSMTP,
					SMTP: &domain.SMTPSettings{Host: "smtp.example.com", Password: smtpPassword},
				},
			}},
		}
		ws.Settings.FileManager = domain.FileManagerSettings{
			Bucket:    "assets",
			AccessKey: "AKIAEXAMPLE",
			SecretKey: fileManagerSecret,
		}
		return ws
	}

	serialise := func(t *testing.T, ctx context.Context) string {
		t.Helper()
		ws := newWorkspace()
		redactWorkspaceForCaller(ctx, ws)
		encoded, err := json.Marshal(ws)
		require.NoError(t, err)
		return string(encoded)
	}

	consoleSession := context.WithValue(context.Background(), domain.UserTypeKey, string(domain.UserTypeUser))
	apiKey := context.WithValue(context.Background(), domain.UserTypeKey, string(domain.UserTypeAPIKey))

	t.Run("a console session still gets the S3 secret", func(t *testing.T) {
		body := serialise(t, consoleSession)
		assert.Contains(t, body, fileManagerSecret, "blanking this breaks the browser file manager")
		assert.Contains(t, body, "assets")
	})

	t.Run("an API key does not", func(t *testing.T) {
		body := serialise(t, apiKey)
		assert.NotContains(t, body, fileManagerSecret)
		// Everything an integration reads from a workspace survives.
		assert.Contains(t, body, "assets")
		assert.Contains(t, body, "AKIAEXAMPLE")
	})

	t.Run("a context that proves nothing fails closed", func(t *testing.T) {
		body := serialise(t, context.Background())
		assert.NotContains(t, body, fileManagerSecret)
	})

	t.Run("integration credentials go for both", func(t *testing.T) {
		assert.NotContains(t, serialise(t, consoleSession), smtpPassword)
		assert.NotContains(t, serialise(t, apiKey), smtpPassword)
	})

	t.Run("a nil workspace is not a panic", func(t *testing.T) {
		assert.NotPanics(t, func() { redactWorkspaceForCaller(apiKey, nil) })
	})
}
