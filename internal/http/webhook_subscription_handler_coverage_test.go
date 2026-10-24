package http

import (
	"context"
	"encoding/json"
	"errors"
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

func TestCoverage_NewWebhookSubscriptionHandler(t *testing.T) {
	secret := func() ([]byte, error) { return []byte("test"), nil }
	h := NewWebhookSubscriptionHandler(nil, nil, secret, &mockLogger{})

	require.NotNil(t, h)
	assert.Nil(t, h.service)
	assert.Nil(t, h.worker)
	assert.NotNil(t, h.getJWTSecret)
	assert.NotNil(t, h.logger)
	got, err := h.getJWTSecret()
	require.NoError(t, err)
	assert.Equal(t, []byte("test"), got)
}

func TestCoverage_WebhookSubscriptionRegisterRoutes_RequireAuth(t *testing.T) {
	h := NewWebhookSubscriptionHandler(nil, nil, func() ([]byte, error) { return []byte("test"), nil }, &mockLogger{})
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	routes := []string{
		"/api/webhookSubscriptions.create",
		"/api/webhookSubscriptions.list",
		"/api/webhookSubscriptions.get",
		"/api/webhookSubscriptions.update",
		"/api/webhookSubscriptions.delete",
		"/api/webhookSubscriptions.toggle",
		"/api/webhookSubscriptions.regenerateSecret",
		"/api/webhookSubscriptions.deliveries",
		"/api/webhookSubscriptions.test",
		"/api/webhookSubscriptions.eventTypes",
	}
	for _, route := range routes {
		t.Run(route, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, route, nil)
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)
			// 401 (not 404) proves the route is registered and auth-wrapped.
			assert.Equal(t, http.StatusUnauthorized, rr.Code)
			var body map[string]string
			require.NoError(t, json.NewDecoder(rr.Body).Decode(&body))
			assert.Equal(t, "Authorization header is required", body["error"])
		})
	}

	t.Run("unregistered route is 404", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/webhookSubscriptions.nope", nil)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})
}

// coverageDeliveriesHandler builds a handler backed by the real service with
// mocked repo/auth so argument forwarding into ListAll is observable.
func coverageDeliveriesHandler(t *testing.T, authSvc *mocks.MockAuthService, deliveryRepo *mocks.MockWebhookDeliveryRepository) *WebhookSubscriptionHandler {
	t.Helper()
	return &WebhookSubscriptionHandler{
		service:      service.NewWebhookSubscriptionService(nil, deliveryRepo, authSvc, &mockLogger{}, false),
		worker:       nil,
		logger:       &mockLogger{},
		getJWTSecret: func() ([]byte, error) { return []byte("test"), nil },
	}
}

func TestCoverage_HandleGetDeliveries_SuccessDefaults(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	deliveryRepo := mocks.NewMockWebhookDeliveryRepository(ctrl)
	deliveryRepo.EXPECT().
		ListAll(gomock.Any(), "ws123", gomock.Nil(), 20, 0).
		Return([]*domain.WebhookDelivery{
			{ID: "d1", SubscriptionID: "sub123", EventType: "contact.created", Status: "delivered", Attempts: 2},
			{ID: "d2", SubscriptionID: "sub999", EventType: "list.subscribed", Status: "failed", Attempts: 3},
		}, 7, nil)

	handler := coverageDeliveriesHandler(t, webhookWriteAuth(ctrl, "ws123"), deliveryRepo)
	req := httptest.NewRequest(http.MethodGet, "/api/webhookSubscriptions.deliveries?workspace_id=ws123", nil)
	rr := httptest.NewRecorder()
	handler.handleGetDeliveries(rr, req)

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	var body struct {
		Deliveries []*domain.WebhookDelivery `json:"deliveries"`
		Total      int                       `json:"total"`
		Limit      int                       `json:"limit"`
		Offset     int                       `json:"offset"`
	}
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&body))
	assert.Equal(t, 7, body.Total)
	assert.Equal(t, 20, body.Limit, "default limit must be 20")
	assert.Equal(t, 0, body.Offset, "default offset must be 0")
	require.Len(t, body.Deliveries, 2)
	assert.Equal(t, "d1", body.Deliveries[0].ID)
	assert.Equal(t, "contact.created", body.Deliveries[0].EventType)
	assert.Equal(t, "delivered", body.Deliveries[0].Status)
	assert.Equal(t, 2, body.Deliveries[0].Attempts)
	assert.Equal(t, "d2", body.Deliveries[1].ID)
}

func TestCoverage_HandleGetDeliveries_ForwardsSubscriptionAndPagination(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	deliveryRepo := mocks.NewMockWebhookDeliveryRepository(ctrl)
	// Exact pointer value, limit and offset must reach the repository.
	deliveryRepo.EXPECT().
		ListAll(gomock.Any(), "ws123", gomock.Any(), 5, 10).
		DoAndReturn(func(_ context.Context, _ string, subID *string, _, _ int) ([]*domain.WebhookDelivery, int, error) {
			require.NotNil(t, subID)
			assert.Equal(t, "sub123", *subID)
			return []*domain.WebhookDelivery{}, 0, nil
		})

	handler := coverageDeliveriesHandler(t, webhookWriteAuth(ctrl, "ws123"), deliveryRepo)
	req := httptest.NewRequest(http.MethodGet, "/api/webhookSubscriptions.deliveries?workspace_id=ws123&subscription_id=sub123&limit=5&offset=10", nil)
	rr := httptest.NewRecorder()
	handler.handleGetDeliveries(rr, req)

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&body))
	assert.Equal(t, float64(5), body["limit"])
	assert.Equal(t, float64(10), body["offset"])
	assert.Equal(t, float64(0), body["total"])
}

func TestCoverage_HandleGetDeliveries_InvalidPaginationFallsBack(t *testing.T) {
	cases := []struct {
		name       string
		query      string
		wantLimit  int
		wantOffset int
	}{
		{"non-numeric limit and offset", "limit=abc&offset=xyz", 20, 0},
		{"zero limit", "limit=0", 20, 0},
		{"limit above cap", "limit=101", 20, 0},
		{"negative limit", "limit=-5", 20, 0},
		{"negative offset", "offset=-1", 20, 0},
		{"limit at cap is accepted", "limit=100&offset=0", 100, 0},
		{"limit one is accepted", "limit=1&offset=3", 1, 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			deliveryRepo := mocks.NewMockWebhookDeliveryRepository(ctrl)
			deliveryRepo.EXPECT().
				ListAll(gomock.Any(), "ws123", gomock.Nil(), tc.wantLimit, tc.wantOffset).
				Return([]*domain.WebhookDelivery{}, 0, nil)
			handler := coverageDeliveriesHandler(t, webhookWriteAuth(ctrl, "ws123"), deliveryRepo)
			req := httptest.NewRequest(http.MethodGet, "/api/webhookSubscriptions.deliveries?workspace_id=ws123&"+tc.query, nil)
			rr := httptest.NewRecorder()
			handler.handleGetDeliveries(rr, req)
			require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
			var body map[string]interface{}
			require.NoError(t, json.NewDecoder(rr.Body).Decode(&body))
			assert.Equal(t, float64(tc.wantLimit), body["limit"])
			assert.Equal(t, float64(tc.wantOffset), body["offset"])
		})
	}
}

func TestCoverage_HandleGetDeliveries_ServiceErrorIs500(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	deliveryRepo := mocks.NewMockWebhookDeliveryRepository(ctrl)
	deliveryRepo.EXPECT().
		ListAll(gomock.Any(), "ws123", gomock.Nil(), 20, 0).
		Return(nil, 0, errors.New("db exploded"))

	handler := coverageDeliveriesHandler(t, webhookWriteAuth(ctrl, "ws123"), deliveryRepo)
	req := httptest.NewRequest(http.MethodGet, "/api/webhookSubscriptions.deliveries?workspace_id=ws123", nil)
	rr := httptest.NewRecorder()
	handler.handleGetDeliveries(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	var body map[string]string
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&body))
	// Generic failure must not leak the wrapped internal error.
	assert.Equal(t, "Failed to get webhook deliveries", body["error"])
}

func TestCoverage_HandleGetDeliveries_ReadPermissionDeniedIs403(t *testing.T) {
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
					// Write without Read: deliveries is a read, so it must be denied.
					domain.PermissionResourceWebhookSubscriptions: {Write: true},
				},
			}, nil
		})
	// No ListAll expectation: the repository must not be reached.
	deliveryRepo := mocks.NewMockWebhookDeliveryRepository(ctrl)

	handler := coverageDeliveriesHandler(t, authSvc, deliveryRepo)
	req := httptest.NewRequest(http.MethodGet, "/api/webhookSubscriptions.deliveries?workspace_id=ws123", nil)
	rr := httptest.NewRecorder()
	handler.handleGetDeliveries(rr, req)

	assert.Equal(t, http.StatusForbidden, rr.Code)
	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&body))
	assert.Equal(t, string(domain.PermissionResourceWebhookSubscriptions), body["resource"])
	assert.Equal(t, string(domain.PermissionTypeRead), body["permission"])
	assert.Contains(t, body["error"], "Insufficient permissions")
}
