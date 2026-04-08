package http

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Notifuse/notifuse/internal/service"
	"github.com/stretchr/testify/assert"
)

func TestWebhookSubscriptionHandler_HandleCreate_ValidationErrors(t *testing.T) {
	testCases := []struct {
		name           string
		method         string
		reqBody        interface{}
		expectedStatus int
		expectedError  string
	}{
		{
			name:           "Method Not Allowed",
			method:         http.MethodGet,
			reqBody:        map[string]interface{}{"workspace_id": "ws123"},
			expectedStatus: http.StatusMethodNotAllowed,
			expectedError:  "Method not allowed",
		},
		{
			name:           "Invalid JSON",
			method:         http.MethodPost,
			reqBody:        "invalid json",
			expectedStatus: http.StatusBadRequest,
			expectedError:  "Invalid request body",
		},
		{
			name:           "Missing Workspace ID",
			method:         http.MethodPost,
			reqBody:        map[string]interface{}{"name": "Test"},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "workspace_id is required",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			handler := &WebhookSubscriptionHandler{
				service:      nil,
				worker:       nil,
				logger:       &mockLogger{},
				getJWTSecret: func() ([]byte, error) { return []byte("test"), nil },
			}

			var reqBody bytes.Buffer
			if str, ok := tc.reqBody.(string); ok {
				reqBody = *bytes.NewBufferString(str)
			} else {
				_ = json.NewEncoder(&reqBody).Encode(tc.reqBody)
			}

			req := httptest.NewRequest(tc.method, "/api/webhookSubscriptions.create", &reqBody)
			rr := httptest.NewRecorder()

			handler.handleCreate(rr, req)

			assert.Equal(t, tc.expectedStatus, rr.Code)

			var response map[string]string
			_ = json.NewDecoder(rr.Body).Decode(&response)
			assert.Equal(t, tc.expectedError, response["error"])
		})
	}
}

func TestWebhookSubscriptionHandler_HandleGet_ValidationErrors(t *testing.T) {
	testCases := []struct {
		name           string
		method         string
		queryParams    string
		expectedStatus int
		expectedError  string
	}{
		{
			name:           "Method Not Allowed",
			method:         http.MethodPost,
			queryParams:    "workspace_id=ws123&id=sub123",
			expectedStatus: http.StatusMethodNotAllowed,
			expectedError:  "Method not allowed",
		},
		{
			name:           "Missing Workspace ID",
			method:         http.MethodGet,
			queryParams:    "id=sub123",
			expectedStatus: http.StatusBadRequest,
			expectedError:  "workspace_id is required",
		},
		{
			name:           "Missing ID",
			method:         http.MethodGet,
			queryParams:    "workspace_id=ws123",
			expectedStatus: http.StatusBadRequest,
			expectedError:  "id is required",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			handler := &WebhookSubscriptionHandler{
				service:      nil,
				worker:       nil,
				logger:       &mockLogger{},
				getJWTSecret: func() ([]byte, error) { return []byte("test"), nil },
			}

			req := httptest.NewRequest(tc.method, "/api/webhookSubscriptions.get?"+tc.queryParams, nil)
			rr := httptest.NewRecorder()

			handler.handleGet(rr, req)

			assert.Equal(t, tc.expectedStatus, rr.Code)

			var response map[string]string
			_ = json.NewDecoder(rr.Body).Decode(&response)
			assert.Equal(t, tc.expectedError, response["error"])
		})
	}
}

func TestWebhookSubscriptionHandler_HandleList_ValidationErrors(t *testing.T) {
	testCases := []struct {
		name           string
		method         string
		queryParams    string
		expectedStatus int
		expectedError  string
	}{
		{
			name:           "Method Not Allowed",
			method:         http.MethodPost,
			queryParams:    "workspace_id=ws123",
			expectedStatus: http.StatusMethodNotAllowed,
			expectedError:  "Method not allowed",
		},
		{
			name:           "Missing Workspace ID",
			method:         http.MethodGet,
			queryParams:    "",
			expectedStatus: http.StatusBadRequest,
			expectedError:  "workspace_id is required",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			handler := &WebhookSubscriptionHandler{
				service:      nil,
				worker:       nil,
				logger:       &mockLogger{},
				getJWTSecret: func() ([]byte, error) { return []byte("test"), nil },
			}

			req := httptest.NewRequest(tc.method, "/api/webhookSubscriptions.list?"+tc.queryParams, nil)
			rr := httptest.NewRecorder()

			handler.handleList(rr, req)

			assert.Equal(t, tc.expectedStatus, rr.Code)

			var response map[string]string
			_ = json.NewDecoder(rr.Body).Decode(&response)
			assert.Equal(t, tc.expectedError, response["error"])
		})
	}
}

func TestWebhookSubscriptionHandler_HandleUpdate_ValidationErrors(t *testing.T) {
	testCases := []struct {
		name           string
		method         string
		reqBody        interface{}
		expectedStatus int
		expectedError  string
	}{
		{
			name:           "Method Not Allowed",
			method:         http.MethodGet,
			reqBody:        map[string]interface{}{"workspace_id": "ws123", "id": "sub123"},
			expectedStatus: http.StatusMethodNotAllowed,
			expectedError:  "Method not allowed",
		},
		{
			name:           "Invalid JSON",
			method:         http.MethodPost,
			reqBody:        "invalid json",
			expectedStatus: http.StatusBadRequest,
			expectedError:  "Invalid request body",
		},
		{
			name:           "Missing Workspace ID",
			method:         http.MethodPost,
			reqBody:        map[string]interface{}{"id": "sub123", "name": "Test"},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "workspace_id is required",
		},
		{
			name:           "Missing ID",
			method:         http.MethodPost,
			reqBody:        map[string]interface{}{"workspace_id": "ws123", "name": "Test"},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "id is required",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			handler := &WebhookSubscriptionHandler{
				service:      nil,
				worker:       nil,
				logger:       &mockLogger{},
				getJWTSecret: func() ([]byte, error) { return []byte("test"), nil },
			}

			var reqBody bytes.Buffer
			if str, ok := tc.reqBody.(string); ok {
				reqBody = *bytes.NewBufferString(str)
			} else {
				_ = json.NewEncoder(&reqBody).Encode(tc.reqBody)
			}

			req := httptest.NewRequest(tc.method, "/api/webhookSubscriptions.update", &reqBody)
			rr := httptest.NewRecorder()

			handler.handleUpdate(rr, req)

			assert.Equal(t, tc.expectedStatus, rr.Code)

			var response map[string]string
			_ = json.NewDecoder(rr.Body).Decode(&response)
			assert.Equal(t, tc.expectedError, response["error"])
		})
	}
}

func TestWebhookSubscriptionHandler_HandleDelete_ValidationErrors(t *testing.T) {
	testCases := []struct {
		name           string
		method         string
		reqBody        interface{}
		expectedStatus int
		expectedError  string
	}{
		{
			name:           "Method Not Allowed",
			method:         http.MethodGet,
			reqBody:        map[string]interface{}{"workspace_id": "ws123", "id": "sub123"},
			expectedStatus: http.StatusMethodNotAllowed,
			expectedError:  "Method not allowed",
		},
		{
			name:           "Invalid JSON",
			method:         http.MethodPost,
			reqBody:        "invalid json",
			expectedStatus: http.StatusBadRequest,
			expectedError:  "Invalid request body",
		},
		{
			name:           "Missing Workspace ID",
			method:         http.MethodPost,
			reqBody:        map[string]interface{}{"id": "sub123"},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "workspace_id is required",
		},
		{
			name:           "Missing ID",
			method:         http.MethodPost,
			reqBody:        map[string]interface{}{"workspace_id": "ws123"},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "id is required",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			handler := &WebhookSubscriptionHandler{
				service:      nil,
				worker:       nil,
				logger:       &mockLogger{},
				getJWTSecret: func() ([]byte, error) { return []byte("test"), nil },
			}

			var reqBody bytes.Buffer
			if str, ok := tc.reqBody.(string); ok {
				reqBody = *bytes.NewBufferString(str)
			} else {
				_ = json.NewEncoder(&reqBody).Encode(tc.reqBody)
			}

			req := httptest.NewRequest(tc.method, "/api/webhookSubscriptions.delete", &reqBody)
			rr := httptest.NewRecorder()

			handler.handleDelete(rr, req)

			assert.Equal(t, tc.expectedStatus, rr.Code)

			var response map[string]string
			_ = json.NewDecoder(rr.Body).Decode(&response)
			assert.Equal(t, tc.expectedError, response["error"])
		})
	}
}

func TestWebhookSubscriptionHandler_HandleToggle_ValidationErrors(t *testing.T) {
	testCases := []struct {
		name           string
		method         string
		reqBody        interface{}
		expectedStatus int
		expectedError  string
	}{
		{
			name:           "Method Not Allowed",
			method:         http.MethodGet,
			reqBody:        map[string]interface{}{"workspace_id": "ws123", "id": "sub123", "enabled": true},
			expectedStatus: http.StatusMethodNotAllowed,
			expectedError:  "Method not allowed",
		},
		{
			name:           "Invalid JSON",
			method:         http.MethodPost,
			reqBody:        "invalid json",
			expectedStatus: http.StatusBadRequest,
			expectedError:  "Invalid request body",
		},
		{
			name:           "Missing Workspace ID",
			method:         http.MethodPost,
			reqBody:        map[string]interface{}{"id": "sub123", "enabled": true},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "workspace_id is required",
		},
		{
			name:           "Missing ID",
			method:         http.MethodPost,
			reqBody:        map[string]interface{}{"workspace_id": "ws123", "enabled": true},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "id is required",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			handler := &WebhookSubscriptionHandler{
				service:      nil,
				worker:       nil,
				logger:       &mockLogger{},
				getJWTSecret: func() ([]byte, error) { return []byte("test"), nil },
			}

			var reqBody bytes.Buffer
			if str, ok := tc.reqBody.(string); ok {
				reqBody = *bytes.NewBufferString(str)
			} else {
				_ = json.NewEncoder(&reqBody).Encode(tc.reqBody)
			}

			req := httptest.NewRequest(tc.method, "/api/webhookSubscriptions.toggle", &reqBody)
			rr := httptest.NewRecorder()

			handler.handleToggle(rr, req)

			assert.Equal(t, tc.expectedStatus, rr.Code)

			var response map[string]string
			_ = json.NewDecoder(rr.Body).Decode(&response)
			assert.Equal(t, tc.expectedError, response["error"])
		})
	}
}

func TestWebhookSubscriptionHandler_HandleRegenerateSecret_ValidationErrors(t *testing.T) {
	testCases := []struct {
		name           string
		method         string
		reqBody        interface{}
		expectedStatus int
		expectedError  string
	}{
		{
			name:           "Method Not Allowed",
			method:         http.MethodGet,
			reqBody:        map[string]interface{}{"workspace_id": "ws123", "id": "sub123"},
			expectedStatus: http.StatusMethodNotAllowed,
			expectedError:  "Method not allowed",
		},
		{
			name:           "Invalid JSON",
			method:         http.MethodPost,
			reqBody:        "invalid json",
			expectedStatus: http.StatusBadRequest,
			expectedError:  "Invalid request body",
		},
		{
			name:           "Missing Workspace ID",
			method:         http.MethodPost,
			reqBody:        map[string]interface{}{"id": "sub123"},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "workspace_id is required",
		},
		{
			name:           "Missing ID",
			method:         http.MethodPost,
			reqBody:        map[string]interface{}{"workspace_id": "ws123"},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "id is required",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			handler := &WebhookSubscriptionHandler{
				service:      nil,
				worker:       nil,
				logger:       &mockLogger{},
				getJWTSecret: func() ([]byte, error) { return []byte("test"), nil },
			}

			var reqBody bytes.Buffer
			if str, ok := tc.reqBody.(string); ok {
				reqBody = *bytes.NewBufferString(str)
			} else {
				_ = json.NewEncoder(&reqBody).Encode(tc.reqBody)
			}

			req := httptest.NewRequest(tc.method, "/api/webhookSubscriptions.regenerateSecret", &reqBody)
			rr := httptest.NewRecorder()

			handler.handleRegenerateSecret(rr, req)

			assert.Equal(t, tc.expectedStatus, rr.Code)

			var response map[string]string
			_ = json.NewDecoder(rr.Body).Decode(&response)
			assert.Equal(t, tc.expectedError, response["error"])
		})
	}
}

func TestWebhookSubscriptionHandler_HandleGetDeliveries_ValidationErrors(t *testing.T) {
	testCases := []struct {
		name           string
		method         string
		queryParams    string
		expectedStatus int
		expectedError  string
	}{
		{
			name:           "Method Not Allowed",
			method:         http.MethodPost,
			queryParams:    "workspace_id=ws123&subscription_id=sub123",
			expectedStatus: http.StatusMethodNotAllowed,
			expectedError:  "Method not allowed",
		},
		{
			name:           "Missing Workspace ID",
			method:         http.MethodGet,
			queryParams:    "subscription_id=sub123",
			expectedStatus: http.StatusBadRequest,
			expectedError:  "workspace_id is required",
		},
		// Note: subscription_id is now optional, so "Missing Subscription ID" is no longer an error
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			handler := &WebhookSubscriptionHandler{
				service:      nil,
				worker:       nil,
				logger:       &mockLogger{},
				getJWTSecret: func() ([]byte, error) { return []byte("test"), nil },
			}

			req := httptest.NewRequest(tc.method, "/api/webhookSubscriptions.deliveries?"+tc.queryParams, nil)
			rr := httptest.NewRecorder()

			handler.handleGetDeliveries(rr, req)

			assert.Equal(t, tc.expectedStatus, rr.Code)

			var response map[string]string
			_ = json.NewDecoder(rr.Body).Decode(&response)
			assert.Equal(t, tc.expectedError, response["error"])
		})
	}
}

func TestWebhookSubscriptionHandler_HandleTest_ValidationErrors(t *testing.T) {
	testCases := []struct {
		name           string
		method         string
		reqBody        interface{}
		expectedStatus int
		expectedError  string
	}{
		{
			name:           "Method Not Allowed",
			method:         http.MethodGet,
			reqBody:        map[string]interface{}{"workspace_id": "ws123", "id": "sub123"},
			expectedStatus: http.StatusMethodNotAllowed,
			expectedError:  "Method not allowed",
		},
		{
			name:           "Invalid JSON",
			method:         http.MethodPost,
			reqBody:        "invalid json",
			expectedStatus: http.StatusBadRequest,
			expectedError:  "Invalid request body",
		},
		{
			name:           "Missing Workspace ID",
			method:         http.MethodPost,
			reqBody:        map[string]interface{}{"id": "sub123"},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "workspace_id is required",
		},
		{
			name:           "Missing ID",
			method:         http.MethodPost,
			reqBody:        map[string]interface{}{"workspace_id": "ws123"},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "id is required",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			handler := &WebhookSubscriptionHandler{
				service:      nil,
				worker:       nil,
				logger:       &mockLogger{},
				getJWTSecret: func() ([]byte, error) { return []byte("test"), nil },
			}

			var reqBody bytes.Buffer
			if str, ok := tc.reqBody.(string); ok {
				reqBody = *bytes.NewBufferString(str)
			} else {
				_ = json.NewEncoder(&reqBody).Encode(tc.reqBody)
			}

			req := httptest.NewRequest(tc.method, "/api/webhookSubscriptions.test", &reqBody)
			rr := httptest.NewRecorder()

			handler.handleTest(rr, req)

			assert.Equal(t, tc.expectedStatus, rr.Code)

			var response map[string]string
			_ = json.NewDecoder(rr.Body).Decode(&response)
			assert.Equal(t, tc.expectedError, response["error"])
		})
	}
}

func TestWebhookSubscriptionHandler_HandleGetEventTypes_Success(t *testing.T) {
	handler := &WebhookSubscriptionHandler{
		service:      &service.WebhookSubscriptionService{},
		worker:       nil,
		logger:       &mockLogger{},
		getJWTSecret: func() ([]byte, error) { return []byte("test"), nil },
	}

	req := httptest.NewRequest(http.MethodGet, "/api/webhookSubscriptions.eventTypes", nil)
	rr := httptest.NewRecorder()

	handler.handleGetEventTypes(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var response map[string]interface{}
	err := json.NewDecoder(rr.Body).Decode(&response)
	assert.NoError(t, err)
	assert.NotNil(t, response["event_types"])

	eventTypes := response["event_types"].([]interface{})
	assert.Greater(t, len(eventTypes), 0)
}

func TestWebhookSubscriptionHandler_HandleGetEventTypes_MethodNotAllowed(t *testing.T) {
	handler := &WebhookSubscriptionHandler{
		service:      nil,
		worker:       nil,
		logger:       &mockLogger{},
		getJWTSecret: func() ([]byte, error) { return []byte("test"), nil },
	}

	req := httptest.NewRequest(http.MethodPost, "/api/webhookSubscriptions.eventTypes", nil)
	rr := httptest.NewRecorder()

	handler.handleGetEventTypes(rr, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)

	var response map[string]string
	_ = json.NewDecoder(rr.Body).Decode(&response)
	assert.Equal(t, "Method not allowed", response["error"])
}
