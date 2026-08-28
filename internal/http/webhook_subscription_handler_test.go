package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestWebhookSubscriptionHandler_HandleRegenerateSecret_NonOwnerIsForbidden(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	authSvc := mocks.NewMockAuthService(ctrl)
	authSvc.EXPECT().
		AuthenticateUserForWorkspace(gomock.Any(), "ws123").
		DoAndReturn(func(ctx context.Context, _ string) (context.Context, *domain.User, *domain.UserWorkspace, error) {
			return ctx, &domain.User{ID: "user123"}, &domain.UserWorkspace{
				UserID:      "user123",
				WorkspaceID: "ws123",
				Role:        "member",
			}, nil
		})

	handler := &WebhookSubscriptionHandler{
		// The repository is never reached: the owner check rejects first.
		service:      service.NewWebhookSubscriptionService(nil, nil, authSvc, &mockLogger{}),
		worker:       nil,
		logger:       &mockLogger{},
		getJWTSecret: func() ([]byte, error) { return []byte("test"), nil },
	}

	req := httptest.NewRequest(http.MethodPost, "/api/webhookSubscriptions.regenerateSecret",
		bytes.NewBufferString(`{"workspace_id":"ws123","id":"sub123"}`))
	rr := httptest.NewRecorder()

	handler.handleRegenerateSecret(rr, req)

	// The owner-only denial is the caller's answer, not an opaque server failure.
	assert.Equal(t, http.StatusForbidden, rr.Code)

	var response map[string]string
	_ = json.NewDecoder(rr.Body).Decode(&response)
	assert.Equal(t, "only a workspace owner may regenerate a webhook secret", response["error"])
}

// /api/webhookSubscriptions.test fires a real outbound request, so a read-only
// caller must be refused. It used to authorize through GetByID — a read method —
// which would have made the whole delivery reachable with webhook_subscriptions:read.
func TestWebhookSubscriptionHandler_HandleTest_ReadOnlyIsForbidden(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	authSvc := mocks.NewMockAuthService(ctrl)
	authSvc.EXPECT().
		AuthenticateUserForWorkspace(gomock.Any(), "ws123").
		DoAndReturn(func(ctx context.Context, _ string) (context.Context, *domain.User, *domain.UserWorkspace, error) {
			return ctx, &domain.User{ID: "user123"}, &domain.UserWorkspace{
				UserID:      "user123",
				WorkspaceID: "ws123",
				Role:        "member",
				Permissions: domain.UserPermissions{
					domain.PermissionResourceWebhookSubscriptions: {Read: true},
				},
			}, nil
		})

	handler := &WebhookSubscriptionHandler{
		// Neither the repository nor the delivery worker is reached: the write gate
		// rejects first, which is the point of the test.
		service:      service.NewWebhookSubscriptionService(nil, nil, authSvc, &mockLogger{}),
		worker:       nil,
		logger:       &mockLogger{},
		getJWTSecret: func() ([]byte, error) { return []byte("test"), nil },
	}

	req := httptest.NewRequest(http.MethodPost, "/api/webhookSubscriptions.test",
		bytes.NewBufferString(`{"workspace_id":"ws123","id":"sub123","event_type":"contact.created"}`))
	rr := httptest.NewRecorder()

	handler.handleTest(rr, req)

	assert.Equal(t, http.StatusForbidden, rr.Code)

	var response map[string]interface{}
	_ = json.NewDecoder(rr.Body).Decode(&response)
	assert.Equal(t, string(domain.PermissionResourceWebhookSubscriptions), response["resource"])
	assert.Equal(t, string(domain.PermissionTypeWrite), response["permission"])
}

// The read routes answer 403 on a denial too — without the mapping they fall
// through to their own generic branch, which reports a 500 for a caller-side
// problem.
func TestWebhookSubscriptionHandler_HandleList_UngrantedIsForbidden(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	authSvc := mocks.NewMockAuthService(ctrl)
	authSvc.EXPECT().
		AuthenticateUserForWorkspace(gomock.Any(), "ws123").
		DoAndReturn(func(ctx context.Context, _ string) (context.Context, *domain.User, *domain.UserWorkspace, error) {
			return ctx, &domain.User{ID: "user123"}, &domain.UserWorkspace{
				UserID:      "user123",
				WorkspaceID: "ws123",
				Role:        "member",
				Permissions: domain.UserPermissions{
					domain.PermissionResourceWebhookSubscriptions: {Write: true},
				},
			}, nil
		})

	handler := &WebhookSubscriptionHandler{
		service:      service.NewWebhookSubscriptionService(nil, nil, authSvc, &mockLogger{}),
		worker:       nil,
		logger:       &mockLogger{},
		getJWTSecret: func() ([]byte, error) { return []byte("test"), nil },
	}

	req := httptest.NewRequest(http.MethodGet, "/api/webhookSubscriptions.list?workspace_id=ws123", nil)
	rr := httptest.NewRecorder()

	handler.handleList(rr, req)

	assert.Equal(t, http.StatusForbidden, rr.Code)

	var response map[string]interface{}
	_ = json.NewDecoder(rr.Body).Decode(&response)
	assert.Equal(t, string(domain.PermissionResourceWebhookSubscriptions), response["resource"])
	assert.Equal(t, string(domain.PermissionTypeRead), response["permission"])
}

// webhookWriteAuth admits a member holding webhook_subscriptions:write, which is
// what create and update authorize against.
func webhookWriteAuth(ctrl *gomock.Controller, workspaceID string) *mocks.MockAuthService {
	authSvc := mocks.NewMockAuthService(ctrl)
	authSvc.EXPECT().
		AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
		DoAndReturn(func(ctx context.Context, _ string) (context.Context, *domain.User, *domain.UserWorkspace, error) {
			return ctx, &domain.User{ID: "user123"}, &domain.UserWorkspace{
				UserID:      "user123",
				WorkspaceID: workspaceID,
				Role:        "member",
				Permissions: domain.UserPermissions{
					domain.PermissionResourceWebhookSubscriptions: {Read: true, Write: true},
				},
			}, nil
		}).AnyTimes()
	return authSvc
}

// The bodies here are raw JSON rather than struct literals on purpose: the cases
// that matter are an absent "source" and an absent "list_ids", and a typed
// literal cannot express a key that was never sent.
func TestWebhookSubscriptionHandler_HandleCreate_SourceAndIDFilters(t *testing.T) {
	testCases := []struct {
		name               string
		body               string
		expectedSource     string
		expectedListIDs    []string
		expectedSegmentIDs []string
	}{
		{
			name:           "zapier source is accepted and persisted",
			body:           `{"workspace_id":"ws123","name":"Zap","url":"https://hooks.zapier.com/hooks/standard/1/abc/","event_types":["contact.created"],"source":"zapier"}`,
			expectedSource: "zapier",
		},
		{
			// A body from any pre-existing client, which never sends the field.
			name:           "an absent source stores the user-created value",
			body:           `{"workspace_id":"ws123","name":"Mine","url":"https://example.com/hook","event_types":["contact.created"]}`,
			expectedSource: "",
		},
		{
			name:               "list_ids and segment_ids reach the stored settings",
			body:               `{"workspace_id":"ws123","name":"Filtered","url":"https://example.com/hook","event_types":["list.subscribed","segment.joined"],"list_ids":["list-a"],"segment_ids":["seg-a","seg-b"]}`,
			expectedListIDs:    []string{"list-a"},
			expectedSegmentIDs: []string{"seg-a", "seg-b"},
		},
		{
			// Absent and empty must be indistinguishable downstream — both mean
			// "every list, every segment". Storing a present-but-empty array invites
			// a filter predicate that keys off presence to match nothing at all.
			name:               "an empty array is no filter, not an empty filter",
			body:               `{"workspace_id":"ws123","name":"Unfiltered","url":"https://example.com/hook","event_types":["list.subscribed"],"list_ids":[],"segment_ids":[]}`,
			expectedListIDs:    nil,
			expectedSegmentIDs: nil,
		},
		{
			name:               "absent id filters are stored as no filter",
			body:               `{"workspace_id":"ws123","name":"Unfiltered","url":"https://example.com/hook","event_types":["list.subscribed"]}`,
			expectedListIDs:    nil,
			expectedSegmentIDs: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			var stored *domain.WebhookSubscription
			repo := mocks.NewMockWebhookSubscriptionRepository(ctrl)
			repo.EXPECT().
				Create(gomock.Any(), "ws123", gomock.Any()).
				DoAndReturn(func(_ context.Context, _ string, sub *domain.WebhookSubscription) error {
					stored = sub
					return nil
				})

			handler := &WebhookSubscriptionHandler{
				service:      service.NewWebhookSubscriptionService(repo, nil, webhookWriteAuth(ctrl, "ws123"), &mockLogger{}),
				worker:       nil,
				logger:       &mockLogger{},
				getJWTSecret: func() ([]byte, error) { return []byte("test"), nil },
			}

			req := httptest.NewRequest(http.MethodPost, "/api/webhookSubscriptions.create", bytes.NewBufferString(tc.body))
			rr := httptest.NewRecorder()

			handler.handleCreate(rr, req)

			assert.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
			if assert.NotNil(t, stored) {
				assert.Equal(t, tc.expectedSource, stored.Source)
				assert.Equal(t, tc.expectedListIDs, stored.Settings.ListIDs)
				assert.Equal(t, tc.expectedSegmentIDs, stored.Settings.SegmentIDs)
			}
		})
	}
}

// An unrecognised source is refused before the service is reached — the handler
// here has no service at all, so a nil dereference is the failure mode if the
// check ever moves below the call.
func TestWebhookSubscriptionHandler_HandleCreate_RejectsUnknownSource(t *testing.T) {
	handler := &WebhookSubscriptionHandler{
		service:      nil,
		worker:       nil,
		logger:       &mockLogger{},
		getJWTSecret: func() ([]byte, error) { return []byte("test"), nil },
	}

	body := map[string]interface{}{
		"workspace_id": "ws123",
		"name":         "Evil",
		"url":          "https://example.com/hook",
		"event_types":  []string{"contact.created"},
		"source":       "evil",
	}
	var reqBody bytes.Buffer
	_ = json.NewEncoder(&reqBody).Encode(body)

	req := httptest.NewRequest(http.MethodPost, "/api/webhookSubscriptions.create", &reqBody)
	rr := httptest.NewRecorder()

	handler.handleCreate(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)

	var response map[string]string
	_ = json.NewDecoder(rr.Body).Decode(&response)
	assert.Contains(t, response["error"], "invalid webhook subscription source")
}

// Source is not one of the fields webhookSubscriptions.update accepts at all, so it
// has to survive a body that does not carry it — and must not be settable by one
// that does.
func TestWebhookSubscriptionHandler_HandleUpdate_LeavesSourceUnchanged(t *testing.T) {
	testCases := []struct {
		name           string
		storedSource   string
		body           string
		expectedSource string
	}{
		{
			name:           "a body claiming a different source cannot re-attribute the subscription",
			storedSource:   domain.WebhookSubscriptionSourceUser,
			body:           `{"workspace_id":"ws123","id":"sub123","name":"Renamed","url":"https://example.com/hook","event_types":["contact.created"],"enabled":true,"source":"zapier"}`,
			expectedSource: domain.WebhookSubscriptionSourceUser,
		},
		{
			// The console sends exactly this body when a user edits a Zapier-created
			// subscription; it must not silently strip the attribution.
			name:           "an absent source keeps the stored zapier attribution",
			storedSource:   domain.WebhookSubscriptionSourceZapier,
			body:           `{"workspace_id":"ws123","id":"sub123","name":"Renamed","url":"https://example.com/hook","event_types":["contact.created"],"enabled":true}`,
			expectedSource: domain.WebhookSubscriptionSourceZapier,
		},
		{
			name:           "an explicitly empty source does not clear the stored attribution",
			storedSource:   domain.WebhookSubscriptionSourceZapier,
			body:           `{"workspace_id":"ws123","id":"sub123","name":"Renamed","url":"https://example.com/hook","event_types":["contact.created"],"enabled":true,"source":""}`,
			expectedSource: domain.WebhookSubscriptionSourceZapier,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			repo := mocks.NewMockWebhookSubscriptionRepository(ctrl)
			repo.EXPECT().
				GetByID(gomock.Any(), "ws123", "sub123").
				Return(&domain.WebhookSubscription{
					ID:      "sub123",
					Name:    "Original",
					URL:     "https://example.com/hook",
					Secret:  "whsec_abc",
					Source:  tc.storedSource,
					Enabled: true,
					Settings: domain.WebhookSubscriptionSettings{
						EventTypes: []string{"contact.created"},
					},
				}, nil)

			var stored *domain.WebhookSubscription
			repo.EXPECT().
				Update(gomock.Any(), "ws123", gomock.Any()).
				DoAndReturn(func(_ context.Context, _ string, sub *domain.WebhookSubscription) error {
					stored = sub
					return nil
				})

			handler := &WebhookSubscriptionHandler{
				service:      service.NewWebhookSubscriptionService(repo, nil, webhookWriteAuth(ctrl, "ws123"), &mockLogger{}),
				worker:       nil,
				logger:       &mockLogger{},
				getJWTSecret: func() ([]byte, error) { return []byte("test"), nil },
			}

			req := httptest.NewRequest(http.MethodPost, "/api/webhookSubscriptions.update", bytes.NewBufferString(tc.body))
			rr := httptest.NewRecorder()

			handler.handleUpdate(rr, req)

			assert.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
			if assert.NotNil(t, stored) {
				assert.Equal(t, tc.expectedSource, stored.Source)
				// The rest of the replace still applies, so the test cannot pass by
				// the update having been dropped altogether.
				assert.Equal(t, "Renamed", stored.Name)
			}
		})
	}
}

// The console can only badge a Zapier subscription, or explain an automatic
// disable, if these fields survive the response encoding.
func TestWebhookSubscriptionHandler_HandleGet_SurfacesAttributionAndFailureState(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	reason := "endpoint returned 410 Gone"
	repo := mocks.NewMockWebhookSubscriptionRepository(ctrl)
	repo.EXPECT().
		GetByID(gomock.Any(), "ws123", "sub123").
		Return(&domain.WebhookSubscription{
			ID:                  "sub123",
			Name:                "Zap",
			URL:                 "https://hooks.zapier.com/hooks/standard/1/abc/",
			Source:              domain.WebhookSubscriptionSourceZapier,
			ConsecutiveFailures: 7,
			DisabledReason:      &reason,
			Settings: domain.WebhookSubscriptionSettings{
				EventTypes: []string{"list.subscribed"},
				ListIDs:    []string{"list-a"},
			},
		}, nil)

	handler := &WebhookSubscriptionHandler{
		service:      service.NewWebhookSubscriptionService(repo, nil, webhookWriteAuth(ctrl, "ws123"), &mockLogger{}),
		worker:       nil,
		logger:       &mockLogger{},
		getJWTSecret: func() ([]byte, error) { return []byte("test"), nil },
	}

	req := httptest.NewRequest(http.MethodGet, "/api/webhookSubscriptions.get?workspace_id=ws123&id=sub123", nil)
	rr := httptest.NewRecorder()

	handler.handleGet(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code, rr.Body.String())

	var response struct {
		Subscription map[string]interface{} `json:"subscription"`
	}
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&response))
	assert.Equal(t, "zapier", response.Subscription["source"])
	assert.Equal(t, float64(7), response.Subscription["consecutive_failures"])
	assert.Equal(t, reason, response.Subscription["disabled_reason"])
	assert.Equal(t, []interface{}{"list-a"}, response.Subscription["list_ids"])
}

// TestWebhookSubscriptionHandler_HandleDelete_AlreadyGoneIsNotFound pins the
// status code for deleting a subscription that no longer exists.
//
// The repository already reports it as a typed sentinel, but writeServiceError
// did not know that sentinel, so it fell through to 500. That made the one
// recovery the console's own delete dialog recommends — turn the Zap off and on
// again — the action that errors: performUnsubscribe posts the stored id, gets
// 500, and Zapier reports the unsubscribe as failed forever. Zapier's
// at-least-once retry of a delete whose response was lost lands in the same
// place. Deleting a row that is already gone has reached the state the caller
// asked for; it is not a server fault.
//
// Driven through the real service and a mocked repository rather than a mocked
// service, because the wrapping between the two is the thing that used to
// defeat the mapping.
func TestWebhookSubscriptionHandler_HandleDelete_AlreadyGoneIsNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	authSvc := mocks.NewMockAuthService(ctrl)
	authSvc.EXPECT().
		AuthenticateUserForWorkspace(gomock.Any(), "ws123").
		DoAndReturn(func(ctx context.Context, _ string) (context.Context, *domain.User, *domain.UserWorkspace, error) {
			return ctx, &domain.User{ID: "user123"}, &domain.UserWorkspace{
				UserID:      "user123",
				WorkspaceID: "ws123",
				Role:        "owner",
			}, nil
		})

	subRepo := mocks.NewMockWebhookSubscriptionRepository(ctrl)
	subRepo.EXPECT().Delete(gomock.Any(), "ws123", "sub123").
		Return(fmt.Errorf("webhook subscription sub123: %w", domain.ErrWebhookSubscriptionNotFound))

	deliveryRepo := mocks.NewMockWebhookDeliveryRepository(ctrl)
	deliveryRepo.EXPECT().DeleteBySubscriptionID(gomock.Any(), "ws123", "sub123").Return(nil)

	handler := &WebhookSubscriptionHandler{
		service:      service.NewWebhookSubscriptionService(subRepo, deliveryRepo, authSvc, &mockLogger{}),
		logger:       &mockLogger{},
		getJWTSecret: func() ([]byte, error) { return []byte("test"), nil },
	}

	req := httptest.NewRequest(http.MethodPost, "/api/webhookSubscriptions.delete",
		bytes.NewBufferString(`{"workspace_id":"ws123","id":"sub123"}`))
	rr := httptest.NewRecorder()

	handler.handleDelete(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)

	var response map[string]string
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&response))
	assert.Equal(t, "Webhook subscription not found", response["error"])
}

// The enabled flag is patched, not replaced. A body that omits it decoded as false
// and disabled the subscription, which the console reported as a successful save —
// and disabling one drains its queued deliveries into a state the worker will never
// claim again, so nothing about that is recoverable by switching it back on.
func TestWebhookSubscriptionHandler_HandleUpdate_EnabledIsOptional(t *testing.T) {
	testCases := []struct {
		name          string
		storedEnabled bool
		body          string
		expected      bool
	}{
		{
			name:          "a body without the key leaves an enabled subscription enabled",
			storedEnabled: true,
			body:          `{"workspace_id":"ws123","id":"sub123","name":"Renamed","url":"https://example.com/hook","event_types":["contact.created"]}`,
			expected:      true,
		},
		{
			name:          "a body without the key leaves a disabled subscription disabled",
			storedEnabled: false,
			body:          `{"workspace_id":"ws123","id":"sub123","name":"Renamed","url":"https://example.com/hook","event_types":["contact.created"]}`,
			expected:      false,
		},
		{
			name:          "an explicit false still disables",
			storedEnabled: true,
			body:          `{"workspace_id":"ws123","id":"sub123","name":"Renamed","url":"https://example.com/hook","event_types":["contact.created"],"enabled":false}`,
			expected:      false,
		},
		{
			name:          "an explicit true still enables",
			storedEnabled: false,
			body:          `{"workspace_id":"ws123","id":"sub123","name":"Renamed","url":"https://example.com/hook","event_types":["contact.created"],"enabled":true}`,
			expected:      true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			repo := mocks.NewMockWebhookSubscriptionRepository(ctrl)
			repo.EXPECT().
				GetByID(gomock.Any(), "ws123", "sub123").
				Return(&domain.WebhookSubscription{
					ID:      "sub123",
					Name:    "Original",
					URL:     "https://example.com/hook",
					Enabled: tc.storedEnabled,
					Settings: domain.WebhookSubscriptionSettings{
						EventTypes: []string{"contact.created"},
					},
				}, nil)

			var stored *domain.WebhookSubscription
			repo.EXPECT().
				Update(gomock.Any(), "ws123", gomock.Any()).
				DoAndReturn(func(_ context.Context, _ string, sub *domain.WebhookSubscription) error {
					stored = sub
					return nil
				})

			handler := &WebhookSubscriptionHandler{
				service:      service.NewWebhookSubscriptionService(repo, nil, webhookWriteAuth(ctrl, "ws123"), &mockLogger{}),
				worker:       nil,
				logger:       &mockLogger{},
				getJWTSecret: func() ([]byte, error) { return []byte("test"), nil },
			}

			req := httptest.NewRequest(http.MethodPost, "/api/webhookSubscriptions.update", bytes.NewBufferString(tc.body))
			rr := httptest.NewRecorder()

			handler.handleUpdate(rr, req)

			assert.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
			if assert.NotNil(t, stored) {
				assert.Equal(t, tc.expected, stored.Enabled)
				// The replace itself still has to land, so a dropped update cannot
				// pass for a preserved flag.
				assert.Equal(t, "Renamed", stored.Name)
			}
		})
	}
}

// TestWebhookSubscriptionHandler_HandleUpdate_FiltersArePatched pins what the wire
// means for the three narrowing filters.
//
// Naming a filter replaces it, an explicitly empty one clears it, and saying nothing
// about it — an absent key or a JSON null — leaves the stored one alone. The stored
// value has to win the tie because the two readings are not equally wrong: reading
// silence as "clear it" widens the subscription, and a widened subscription delivers
// events its owner never asked for without anything reporting the change.
//
// Raw JSON bodies rather than typed literals, deliberately: a Go struct literal cannot
// express a key that was never sent, which is the entire distinction under test.
func TestWebhookSubscriptionHandler_HandleUpdate_FiltersArePatched(t *testing.T) {
	const head = `{"workspace_id":"ws123","id":"sub123","name":"Renamed","url":"https://example.com/hook","event_types":["list.subscribed"]`

	testCases := []struct {
		name               string
		body               string
		expectedListIDs    []string
		expectedSegmentIDs []string
		expectedCustom     *domain.CustomEventFilters
	}{
		{
			name:               "a body that names no filter keeps every stored one",
			body:               head + `}`,
			expectedListIDs:    []string{"list-a"},
			expectedSegmentIDs: []string{"seg-a"},
			expectedCustom:     &domain.CustomEventFilters{GoalTypes: []string{"purchase"}},
		},
		{
			name:               "a null filter is silence too, and never widens",
			body:               head + `,"list_ids":null,"segment_ids":null,"custom_event_filters":null}`,
			expectedListIDs:    []string{"list-a"},
			expectedSegmentIDs: []string{"seg-a"},
			expectedCustom:     &domain.CustomEventFilters{GoalTypes: []string{"purchase"}},
		},
		{
			// A caller that knows about the switch and not about the filters, which is
			// the shape of every client written before the filters existed. Naming
			// enabled must not drag the settings along with it.
			name:               "naming the switch says nothing about the filters",
			body:               head + `,"enabled":true}`,
			expectedListIDs:    []string{"list-a"},
			expectedSegmentIDs: []string{"seg-a"},
			expectedCustom:     &domain.CustomEventFilters{GoalTypes: []string{"purchase"}},
		},
		{
			name:               "an explicitly empty filter is how a caller clears one",
			body:               head + `,"list_ids":[],"segment_ids":[],"custom_event_filters":{}}`,
			expectedListIDs:    nil,
			expectedSegmentIDs: nil,
			expectedCustom:     &domain.CustomEventFilters{},
		},
		{
			name:               "a populated filter replaces the stored one",
			body:               head + `,"list_ids":["list-b"],"segment_ids":["seg-b"],"custom_event_filters":{"event_names":["orders/fulfilled"]}}`,
			expectedListIDs:    []string{"list-b"},
			expectedSegmentIDs: []string{"seg-b"},
			expectedCustom:     &domain.CustomEventFilters{EventNames: []string{"orders/fulfilled"}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			repo := mocks.NewMockWebhookSubscriptionRepository(ctrl)
			// Seeded with all three filters populated: against a blank subscription a
			// wipe and a correct merge look exactly the same.
			repo.EXPECT().
				GetByID(gomock.Any(), "ws123", "sub123").
				Return(&domain.WebhookSubscription{
					ID:      "sub123",
					Name:    "Zap: new contact to Slack",
					URL:     "https://example.com/hook",
					Enabled: true,
					Source:  domain.WebhookSubscriptionSourceZapier,
					Settings: domain.WebhookSubscriptionSettings{
						EventTypes:         []string{"list.subscribed"},
						ListIDs:            []string{"list-a"},
						SegmentIDs:         []string{"seg-a"},
						CustomEventFilters: &domain.CustomEventFilters{GoalTypes: []string{"purchase"}},
					},
				}, nil)

			var stored *domain.WebhookSubscription
			repo.EXPECT().
				Update(gomock.Any(), "ws123", gomock.Any()).
				DoAndReturn(func(_ context.Context, _ string, sub *domain.WebhookSubscription) error {
					stored = sub
					return nil
				})

			handler := &WebhookSubscriptionHandler{
				service:      service.NewWebhookSubscriptionService(repo, nil, webhookWriteAuth(ctrl, "ws123"), &mockLogger{}),
				worker:       nil,
				logger:       &mockLogger{},
				getJWTSecret: func() ([]byte, error) { return []byte("test"), nil },
			}

			req := httptest.NewRequest(http.MethodPost, "/api/webhookSubscriptions.update", bytes.NewBufferString(tc.body))
			rr := httptest.NewRecorder()

			handler.handleUpdate(rr, req)

			assert.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
			require.NotNil(t, stored)
			// Literals throughout: the repository hands the service the very object it
			// mutates, so an assertion written against the fixture would compare a field
			// with itself and pass no matter what the handler did.
			assert.Equal(t, tc.expectedListIDs, stored.Settings.ListIDs)
			assert.Equal(t, tc.expectedSegmentIDs, stored.Settings.SegmentIDs)
			assert.Equal(t, tc.expectedCustom, stored.Settings.CustomEventFilters)
			// The replace half of the endpoint still has to land, so a dropped update
			// cannot pass for a preserved filter.
			assert.Equal(t, "Renamed", stored.Name)
		})
	}
}
