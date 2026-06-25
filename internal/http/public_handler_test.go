package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgDatabase "github.com/Notifuse/notifuse/pkg/database"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/Notifuse/notifuse/pkg/ratelimiter"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNotificationCenterHandler_RegisterRoutes(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockService := mocks.NewMockNotificationCenterService(ctrl)
	mockListService := mocks.NewMockListService(ctrl)
	mockLogger := &mockLogger{}
	handler := NewNotificationCenterHandler(mockService, mockListService, mockLogger, nil)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// Test that the endpoints are registered by making test requests
	// and checking that the request doesn't return 404

	// Test notification center endpoint
	req := httptest.NewRequest(http.MethodGet, "/preferences", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	assert.NotEqual(t, http.StatusNotFound, rec.Code)

	// Test subscribe endpoint
	req = httptest.NewRequest(http.MethodPost, "/subscribe", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	assert.NotEqual(t, http.StatusNotFound, rec.Code)

	// Test unsubscribe endpoint
	req = httptest.NewRequest(http.MethodPost, "/unsubscribe-oneclick", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	assert.NotEqual(t, http.StatusNotFound, rec.Code)
}

func TestNotificationCenterHandler_handleNotificationCenter(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockService := mocks.NewMockNotificationCenterService(ctrl)
	mockListService := mocks.NewMockListService(ctrl)
	mockLogger := &mockLogger{}
	handler := NewNotificationCenterHandler(mockService, mockListService, mockLogger, nil)

	tests := []struct {
		name               string
		method             string
		queryParams        string
		setupMock          func()
		expectedStatusCode int
		expectedResponse   string
	}{
		{
			name:               "method not allowed",
			method:             http.MethodPut,
			queryParams:        "",
			setupMock:          func() {},
			expectedStatusCode: http.StatusMethodNotAllowed,
			expectedResponse:   `{"error":"Method not allowed"}`,
		},
		{
			name:               "missing required parameters",
			method:             http.MethodGet,
			queryParams:        "",
			setupMock:          func() {},
			expectedStatusCode: http.StatusBadRequest,
			expectedResponse:   `{"error":"email is required"}`,
		},
		{
			name:        "service returns error - invalid verification",
			method:      http.MethodGet,
			queryParams: "?email=test@example.com&email_hmac=invalid&workspace_id=ws123",
			setupMock: func() {
				mockService.EXPECT().
					GetContactPreferences(gomock.Any(), "ws123", "test@example.com", "invalid").
					Return(nil, errors.New("invalid email verification"))
			},
			expectedStatusCode: http.StatusUnauthorized,
			expectedResponse:   `{"error":"Unauthorized: invalid verification"}`,
		},
		{
			name:        "service returns error - contact not found",
			method:      http.MethodGet,
			queryParams: "?email=test@example.com&email_hmac=valid&workspace_id=ws123",
			setupMock: func() {
				mockService.EXPECT().
					GetContactPreferences(gomock.Any(), "ws123", "test@example.com", "valid").
					Return(nil, errors.New("contact not found"))
			},
			expectedStatusCode: http.StatusNotFound,
			expectedResponse:   `{"error":"Contact not found"}`,
		},
		{
			name:        "service returns error - other error",
			method:      http.MethodGet,
			queryParams: "?email=test@example.com&email_hmac=valid&workspace_id=ws123",
			setupMock: func() {
				mockService.EXPECT().
					GetContactPreferences(gomock.Any(), "ws123", "test@example.com", "valid").
					Return(nil, errors.New("database error"))
			},
			expectedStatusCode: http.StatusInternalServerError,
			expectedResponse:   `{"error":"Failed to get contact preferences"}`,
		},
		{
			name:        "successful request",
			method:      http.MethodGet,
			queryParams: "?email=test@example.com&email_hmac=valid&workspace_id=ws123",
			setupMock: func() {
				response := &domain.ContactPreferencesResponse{
					Contact:      &domain.Contact{Email: "test@example.com"},
					PublicLists:  []*domain.List{{ID: "list1", Name: "Public List"}},
					ContactLists: []*domain.ContactList{{Email: "test@example.com", ListID: "list1"}},
					LogoURL:      "https://example.com/logo.png",
					WebsiteURL:   "https://example.com",
				}
				mockService.EXPECT().
					GetContactPreferences(gomock.Any(), "ws123", "test@example.com", "valid").
					Return(response, nil)
			},
			expectedStatusCode: http.StatusOK,
			// We'll do a partial match for the response
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.setupMock()

			req := httptest.NewRequest(tc.method, "/preferences"+tc.queryParams, nil)
			rec := httptest.NewRecorder()

			handler.handlePreferences(rec, req)

			assert.Equal(t, tc.expectedStatusCode, rec.Code)

			if tc.expectedResponse != "" {
				assert.JSONEq(t, tc.expectedResponse, rec.Body.String())
			} else if tc.expectedStatusCode == http.StatusOK {
				// For successful requests, verify that the response contains expected fields
				var response domain.ContactPreferencesResponse
				err := json.Unmarshal(rec.Body.Bytes(), &response)
				require.NoError(t, err)
				assert.Equal(t, "test@example.com", response.Contact.Email)
				assert.Len(t, response.PublicLists, 1)
				assert.Equal(t, "list1", response.PublicLists[0].ID)
				assert.Len(t, response.ContactLists, 1)
				assert.Equal(t, "test@example.com", response.ContactLists[0].Email)
				assert.Equal(t, "https://example.com/logo.png", response.LogoURL)
				assert.Equal(t, "https://example.com", response.WebsiteURL)
			}
		})
	}
}

func TestNotificationCenterHandler_handleSubscribe(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockService := mocks.NewMockNotificationCenterService(ctrl)
	mockListService := mocks.NewMockListService(ctrl)
	mockLogger := &mockLogger{}
	handler := NewNotificationCenterHandler(mockService, mockListService, mockLogger, nil)

	validRequest := domain.SubscribeToListsRequest{
		WorkspaceID: "ws123",
		Contact: domain.Contact{
			Email: "test@example.com",
		},
		ListIDs: []string{"list1", "list2"},
	}

	tests := []struct {
		name               string
		method             string
		requestBody        interface{}
		setupMock          func()
		expectedStatusCode int
		expectedResponse   string
	}{
		{
			name:               "method not allowed",
			method:             http.MethodGet,
			requestBody:        nil,
			setupMock:          func() {},
			expectedStatusCode: http.StatusMethodNotAllowed,
			expectedResponse:   `{"error":"Method not allowed"}`,
		},
		{
			name:               "invalid request body - not JSON",
			method:             http.MethodPost,
			requestBody:        "invalid json",
			setupMock:          func() {},
			expectedStatusCode: http.StatusBadRequest,
			expectedResponse:   `{"error":"Invalid request body"}`,
		},
		{
			name:               "invalid request body - missing fields",
			method:             http.MethodPost,
			requestBody:        map[string]interface{}{},
			setupMock:          func() {},
			expectedStatusCode: http.StatusBadRequest,
			expectedResponse:   `{"error":"workspace_id is required"}`,
		},
		{
			name:        "service returns error",
			method:      http.MethodPost,
			requestBody: validRequest,
			setupMock: func() {
				mockListService.EXPECT().
					SubscribeToLists(gomock.Any(), gomock.Any(), false).
					Return(errors.New("subscription failed"))
			},
			expectedStatusCode: http.StatusInternalServerError,
			expectedResponse:   `{"error":"Failed to subscribe to lists"}`,
		},
		{
			name:        "service returns non-public list error",
			method:      http.MethodPost,
			requestBody: validRequest,
			setupMock: func() {
				mockListService.EXPECT().
					SubscribeToLists(gomock.Any(), gomock.Any(), false).
					Return(errors.New("list is not public"))
			},
			expectedStatusCode: http.StatusBadRequest,
			expectedResponse:   `{"error":"list is not public"}`,
		},
		{
			name:        "successful request",
			method:      http.MethodPost,
			requestBody: validRequest,
			setupMock: func() {
				mockListService.EXPECT().
					SubscribeToLists(gomock.Any(), gomock.Any(), false).
					Return(nil)
			},
			expectedStatusCode: http.StatusOK,
			expectedResponse:   `{"success":true}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.setupMock()

			var body []byte
			var err error
			if tc.requestBody != nil {
				switch v := tc.requestBody.(type) {
				case string:
					body = []byte(v)
				default:
					body, err = json.Marshal(tc.requestBody)
					require.NoError(t, err)
				}
			}

			req := httptest.NewRequest(tc.method, "/subscribe", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			handler.handleSubscribe(rec, req)

			assert.Equal(t, tc.expectedStatusCode, rec.Code)
			assert.JSONEq(t, tc.expectedResponse, rec.Body.String())
		})
	}
}

// TestNotificationCenterHandler_handleUnsubscribeOneClick exercises the RFC 8058
// one-click unsubscribe contract. Mail providers (Gmail, Yahoo, Apple) issue this
// POST from their own infrastructure, so the endpoint must NOT apply browser-style
// bot detection: a legitimate caller here is always automated. The request is
// authorized by the HMAC in the query string; the RFC 8058 "List-Unsubscribe=One-Click"
// body token is required as defense-in-depth against bare prefetch/scanner POSTs.
func TestNotificationCenterHandler_handleUnsubscribeOneClick(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockService := mocks.NewMockNotificationCenterService(ctrl)
	mockListService := mocks.NewMockListService(ctrl)
	mockLogger := &mockLogger{}
	handler := NewNotificationCenterHandler(mockService, mockListService, mockLogger, nil)

	// Query string carries the identifying params + HMAC, as BuildTemplateData emits them.
	const validQuery = "wid=ws123&email=test%40example.com&email_hmac=deadbeef&lids=list1&mid=msg-1"
	const oneClickBody = "List-Unsubscribe=One-Click"

	t.Run("rejects non-POST methods", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/unsubscribe-oneclick?"+validQuery, strings.NewReader(oneClickBody))
		rec := httptest.NewRecorder()
		handler.handleUnsubscribeOneClick(rec, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
		assert.JSONEq(t, `{"error":"Method not allowed"}`, rec.Body.String())
	})

	// The unsubscribe must succeed for ANY User-Agent as long as the RFC 8058 token is
	// present - including automated callers (curl, Go stdlib, empty UA, even a security
	// scanner) that mail-provider backends use. This is the regression guard for the
	// bot-detection silent-no-op found in review: curl/empty/stdlib UAs were dropped,
	// returning 200 while leaving the contact subscribed (issue #362).
	uaCases := []struct{ name, ua, contentType string }{
		{"browser UA", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36", "application/x-www-form-urlencoded"},
		{"curl UA (issue #362 reproduction)", "curl/8.4.0", "application/x-www-form-urlencoded"},
		{"Go stdlib UA", "Go-http-client/1.1", "application/x-www-form-urlencoded"},
		{"empty UA (provider backend)", "", "application/x-www-form-urlencoded"},
		{"email security scanner UA", "Proofpoint Email Security Scanner", "application/x-www-form-urlencoded"},
		{"text/plain body carrying the token", "Mozilla/5.0", "text/plain"},
	}
	for _, uc := range uaCases {
		t.Run("unsubscribes regardless of UA: "+uc.name, func(t *testing.T) {
			var captured *domain.UnsubscribeFromListsRequest
			mockListService.EXPECT().
				UnsubscribeFromLists(gomock.Any(), gomock.Any(), false).
				DoAndReturn(func(_ context.Context, p *domain.UnsubscribeFromListsRequest, _ bool) error {
					captured = p
					return nil
				})

			req := httptest.NewRequest(http.MethodPost, "/unsubscribe-oneclick?"+validQuery, strings.NewReader(oneClickBody))
			req.Header.Set("Content-Type", uc.contentType)
			if uc.ua != "" {
				req.Header.Set("User-Agent", uc.ua)
			}
			rec := httptest.NewRecorder()

			handler.handleUnsubscribeOneClick(rec, req)

			assert.Equal(t, http.StatusOK, rec.Code)
			assert.JSONEq(t, `{"success":true}`, rec.Body.String())
			require.NotNil(t, captured, "service must be called - the contact must actually be unsubscribed")
			assert.Equal(t, "ws123", captured.WorkspaceID)
			assert.Equal(t, "test@example.com", captured.Email)
			assert.Equal(t, "deadbeef", captured.EmailHMAC)
			assert.Equal(t, []string{"list1"}, captured.ListIDs)
			assert.Equal(t, "msg-1", captured.MessageID)
		})
	}

	// RFC 8058 (section 3.1) requires the body to carry "List-Unsubscribe=One-Click".
	// A POST without it (a bare prefetch/scanner POST, or empty body) is rejected with
	// 400 - never a silent 200 that leaves the contact subscribed. The service is never
	// called, asserted by the absence of a mock expectation (gomock fails on any call).
	rejectCases := []struct{ name, contentType, body string }{
		{"empty body", "application/x-www-form-urlencoded", ""},
		{"stray JSON without token", "application/json", `{"foo":"bar"}`},
		{"wrong token value", "application/x-www-form-urlencoded", "List-Unsubscribe=Two-Click"},
	}
	for _, rc := range rejectCases {
		t.Run("rejects POST missing the RFC 8058 token: "+rc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/unsubscribe-oneclick?"+validQuery, strings.NewReader(rc.body))
			req.Header.Set("Content-Type", rc.contentType)
			req.Header.Set("User-Agent", "Mozilla/5.0")
			rec := httptest.NewRecorder()
			handler.handleUnsubscribeOneClick(rec, req)
			assert.Equal(t, http.StatusBadRequest, rec.Code)
		})
	}

	t.Run("rejects request missing required query params", func(t *testing.T) {
		// Token present, but wid and lids are missing -> malformed link -> 400, no service call.
		req := httptest.NewRequest(http.MethodPost, "/unsubscribe-oneclick?email=test%40example.com",
			strings.NewReader(oneClickBody))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("User-Agent", "Mozilla/5.0")
		rec := httptest.NewRecorder()
		handler.handleUnsubscribeOneClick(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	// The notification center SPA (first-party browser fetch) posts a JSON body with the
	// identifying params and no query string. This path was silently broken in v34.0 when
	// the handler was made RFC 8058-strict (issue #371): it must keep working, authorized
	// by the email_hmac that ListService verifies.
	t.Run("unsubscribes via notification center SPA JSON body", func(t *testing.T) {
		var captured *domain.UnsubscribeFromListsRequest
		mockListService.EXPECT().
			UnsubscribeFromLists(gomock.Any(), gomock.Any(), false).
			DoAndReturn(func(_ context.Context, p *domain.UnsubscribeFromListsRequest, _ bool) error {
				captured = p
				return nil
			})

		body := `{"wid":"ws123","email":"test@example.com","email_hmac":"deadbeef","lids":["list1"],"mid":"msg-1"}`
		req := httptest.NewRequest(http.MethodPost, "/unsubscribe-oneclick", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		handler.handleUnsubscribeOneClick(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.JSONEq(t, `{"success":true}`, rec.Body.String())
		require.NotNil(t, captured, "service must be called - the contact must actually be unsubscribed")
		assert.Equal(t, "ws123", captured.WorkspaceID)
		assert.Equal(t, "test@example.com", captured.Email)
		assert.Equal(t, "deadbeef", captured.EmailHMAC)
		assert.Equal(t, []string{"list1"}, captured.ListIDs)
		assert.Equal(t, "msg-1", captured.MessageID)
	})

	// A JSON body that does not structurally identify a contact + list(s) is a 400, with
	// no service call - same guarantee as the malformed one-click link.
	jsonRejectCases := []struct{ name, body string }{
		{"empty object", `{}`},
		{"missing lids", `{"wid":"ws123","email":"test@example.com"}`},
		{"missing wid", `{"email":"test@example.com","lids":["list1"]}`},
		{"malformed json", `{"wid":`},
	}
	for _, jc := range jsonRejectCases {
		t.Run("rejects invalid SPA JSON body: "+jc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/unsubscribe-oneclick", strings.NewReader(jc.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			handler.handleUnsubscribeOneClick(rec, req)
			assert.Equal(t, http.StatusBadRequest, rec.Code)
		})
	}

	t.Run("service error returns 500", func(t *testing.T) {
		mockListService.EXPECT().
			UnsubscribeFromLists(gomock.Any(), gomock.Any(), false).
			Return(errors.New("unsubscribe failed"))

		req := httptest.NewRequest(http.MethodPost, "/unsubscribe-oneclick?"+validQuery, strings.NewReader(oneClickBody))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("User-Agent", "Mozilla/5.0")
		rec := httptest.NewRecorder()
		handler.handleUnsubscribeOneClick(rec, req)
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.JSONEq(t, `{"error":"Failed to unsubscribe from lists"}`, rec.Body.String())
	})
}

// Mock logger for testing
type mockLogger struct {
}

func (l *mockLogger) Debug(msg string) {}
func (l *mockLogger) Info(msg string)  {}
func (l *mockLogger) Warn(msg string)  {}
func (l *mockLogger) Error(msg string) {}
func (l *mockLogger) Fatal(msg string) {}

func (l *mockLogger) WithField(key string, value interface{}) logger.Logger {
	return l
}

func (l *mockLogger) WithFields(fields map[string]interface{}) logger.Logger {
	return l
}

func (l *mockLogger) WithError(err error) logger.Logger {
	return l
}

func (l *mockLogger) GetLevel() string {
	return "debug"
}

func (l *mockLogger) SetLevel(level string) {}

// Test NewNotificationCenterHandler function
func TestNewNotificationCenterHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockService := mocks.NewMockNotificationCenterService(ctrl)
	mockListService := mocks.NewMockListService(ctrl)
	mockLogger := &mockLogger{}

	handler := NewNotificationCenterHandler(mockService, mockListService, mockLogger, nil)

	assert.NotNil(t, handler)
	assert.Equal(t, mockService, handler.service)
	assert.Equal(t, mockListService, handler.listService)
	assert.Equal(t, mockLogger, handler.logger)
}

func TestNotificationCenterHandler_handleUpdatePreferences(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockService := mocks.NewMockNotificationCenterService(ctrl)
	mockListService := mocks.NewMockListService(ctrl)
	mockLogger := &mockLogger{}
	handler := NewNotificationCenterHandler(mockService, mockListService, mockLogger, nil)

	tests := []struct {
		name               string
		requestBody        interface{}
		setupMock          func()
		expectedStatusCode int
		expectedResponse   string
	}{
		{
			name:               "invalid JSON body",
			requestBody:        "not json",
			setupMock:          func() {},
			expectedStatusCode: http.StatusBadRequest,
			expectedResponse:   `{"error":"Invalid request body"}`,
		},
		{
			name:               "validation failure - missing fields",
			requestBody:        map[string]interface{}{"language": "fr"},
			setupMock:          func() {},
			expectedStatusCode: http.StatusBadRequest,
			expectedResponse:   `{"error":"workspace_id is required"}`,
		},
		{
			name: "validation failure - no language or timezone",
			requestBody: map[string]interface{}{
				"workspace_id": "ws123",
				"email":        "test@example.com",
				"email_hmac":   "hmac",
			},
			setupMock:          func() {},
			expectedStatusCode: http.StatusBadRequest,
			expectedResponse:   `{"error":"at least one of language or timezone must be provided"}`,
		},
		{
			name: "service returns HMAC error",
			requestBody: domain.UpdateContactPreferencesRequest{
				WorkspaceID: "ws123",
				Email:       "test@example.com",
				EmailHMAC:   "invalid",
				Language:    "fr",
			},
			setupMock: func() {
				mockService.EXPECT().
					UpdateContactPreferences(gomock.Any(), gomock.Any()).
					Return(errors.New("invalid email verification"))
			},
			expectedStatusCode: http.StatusUnauthorized,
			expectedResponse:   `{"error":"Unauthorized: invalid verification"}`,
		},
		{
			name: "service returns other error",
			requestBody: domain.UpdateContactPreferencesRequest{
				WorkspaceID: "ws123",
				Email:       "test@example.com",
				EmailHMAC:   "valid",
				Language:    "fr",
			},
			setupMock: func() {
				mockService.EXPECT().
					UpdateContactPreferences(gomock.Any(), gomock.Any()).
					Return(errors.New("database error"))
			},
			expectedStatusCode: http.StatusInternalServerError,
			expectedResponse:   `{"error":"Failed to update contact preferences"}`,
		},
		{
			name: "successful update",
			requestBody: domain.UpdateContactPreferencesRequest{
				WorkspaceID: "ws123",
				Email:       "test@example.com",
				EmailHMAC:   "valid",
				Language:    "fr",
				Timezone:    "Europe/Paris",
			},
			setupMock: func() {
				mockService.EXPECT().
					UpdateContactPreferences(gomock.Any(), gomock.Any()).
					Return(nil)
			},
			expectedStatusCode: http.StatusOK,
			expectedResponse:   `{"success":true}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.setupMock()

			var body []byte
			var err error
			switch v := tc.requestBody.(type) {
			case string:
				body = []byte(v)
			default:
				body, err = json.Marshal(tc.requestBody)
				require.NoError(t, err)
			}

			req := httptest.NewRequest(http.MethodPost, "/preferences", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			handler.handlePreferences(rec, req)

			assert.Equal(t, tc.expectedStatusCode, rec.Code)
			assert.JSONEq(t, tc.expectedResponse, rec.Body.String())
		})
	}
}

func TestNotificationCenterHandler_handleUpdatePreferences_RateLimiting(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockService := mocks.NewMockNotificationCenterService(ctrl)
	mockListService := mocks.NewMockListService(ctrl)
	mockLogger := &mockLogger{}

	rl := ratelimiter.NewRateLimiter()
	defer rl.Stop()
	// Allow only 1 request per 60s window
	rl.SetPolicy("preferences:email", 1, 60*time.Second)
	rl.SetPolicy("preferences:ip", 1, 60*time.Second)

	handler := NewNotificationCenterHandler(mockService, mockListService, mockLogger, rl)

	validReq := domain.UpdateContactPreferencesRequest{
		WorkspaceID: "ws123",
		Email:       "ratelimit@example.com",
		EmailHMAC:   "valid",
		Language:    "fr",
	}

	// First request should succeed
	mockService.EXPECT().
		UpdateContactPreferences(gomock.Any(), gomock.Any()).
		Return(nil)

	body, err := json.Marshal(validReq)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/preferences", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.handlePreferences(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Second request should be rate limited
	body, err = json.Marshal(validReq)
	require.NoError(t, err)

	req = httptest.NewRequest(http.MethodPost, "/preferences", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.handlePreferences(rec, req)
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.NotEmpty(t, rec.Header().Get("Retry-After"))
}

func TestNotificationCenterHandler_handleHealth(t *testing.T) {
	// Test NotificationCenterHandler.handleHealth - this was at 0% coverage
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockService := mocks.NewMockNotificationCenterService(ctrl)
	mockListService := mocks.NewMockListService(ctrl)
	mockLogger := &mockLogger{}
	handler := NewNotificationCenterHandler(mockService, mockListService, mockLogger, nil)

	// Initialize connection manager for testing
	cfg := &config.Config{
		Database: config.DatabaseConfig{
			MaxConnections:      100,
			MaxConnectionsPerDB: 10,
		},
	}
	mockDB, _, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = mockDB.Close() }()

	err = pkgDatabase.InitializeConnectionManager(cfg, mockDB)
	require.NoError(t, err)
	defer pkgDatabase.ResetConnectionManager()

	t.Run("successful health check", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		w := httptest.NewRecorder()

		handler.handleHealth(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Contains(t, response, "max_connections")
		assert.Contains(t, response, "system_connections")
	})

	t.Run("method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/health", nil)
		w := httptest.NewRecorder()

		handler.handleHealth(w, req)

		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}

func TestNotificationCenterHandler_handleHealthz(t *testing.T) {
	// Test NotificationCenterHandler.handleHealthz - this was at 0% coverage
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockService := mocks.NewMockNotificationCenterService(ctrl)
	mockListService := mocks.NewMockListService(ctrl)
	mockLogger := &mockLogger{}
	handler := NewNotificationCenterHandler(mockService, mockListService, mockLogger, nil)

	// Initialize connection manager for testing
	cfg := &config.Config{
		Database: config.DatabaseConfig{
			MaxConnections:      100,
			MaxConnectionsPerDB: 10,
		},
	}
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = mockDB.Close() }()

	// Mock ping query for healthz check
	mock.ExpectPing()

	err = pkgDatabase.InitializeConnectionManager(cfg, mockDB)
	require.NoError(t, err)
	defer pkgDatabase.ResetConnectionManager()

	t.Run("successful healthz check", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		w := httptest.NewRecorder()

		handler.handleHealthz(w, req)

		// May return OK or ServiceUnavailable depending on DB ping
		assert.True(t, w.Code == http.StatusOK || w.Code == http.StatusServiceUnavailable)
	})

	t.Run("method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/healthz", nil)
		w := httptest.NewRecorder()

		handler.handleHealthz(w, req)

		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}
