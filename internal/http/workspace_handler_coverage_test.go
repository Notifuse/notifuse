package http

import (
	"encoding/json"
	"errors"
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

func coverageWorkspaceHandler(t *testing.T) (*WorkspaceHandler, *mocks.MockWorkspaceServiceInterface) {
	t.Helper()
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)
	workspaceSvc := mocks.NewMockWorkspaceServiceInterface(ctrl)
	authSvc := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
	h := NewWorkspaceHandler(workspaceSvc, authSvc, func() ([]byte, error) { return []byte("secret"), nil }, mockLogger, "passphrase", false)
	return h, workspaceSvc
}

func TestCoverage_WithWebAnalyticsCacheInvalidator(t *testing.T) {
	h, _ := coverageWorkspaceHandler(t)
	assert.Nil(t, h.webAnalyticsCacheInvalidator)

	var got []string
	fn := func(workspaceID string) { got = append(got, workspaceID) }
	returned := h.WithWebAnalyticsCacheInvalidator(fn)

	assert.Same(t, h, returned, "builder must return the same handler for chaining")
	require.NotNil(t, h.webAnalyticsCacheInvalidator)
	h.webAnalyticsCacheInvalidator("ws9")
	assert.Equal(t, []string{"ws9"}, got)
}

func TestCoverage_HandleSetWebAnalyticsSettings(t *testing.T) {
	postSettings := func(h *WorkspaceHandler, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/workspaces.setWebAnalyticsSettings", strings.NewReader(body))
		rr := httptest.NewRecorder()
		h.handleSetWebAnalyticsSettings(rr, req)
		return rr
	}
	decodeErr := func(t *testing.T, rr *httptest.ResponseRecorder) string {
		t.Helper()
		var body map[string]string
		require.NoError(t, json.NewDecoder(rr.Body).Decode(&body))
		return body["error"]
	}

	t.Run("GET is 405", func(t *testing.T) {
		h, _ := coverageWorkspaceHandler(t)
		req := httptest.NewRequest(http.MethodGet, "/api/workspaces.setWebAnalyticsSettings", nil)
		rr := httptest.NewRecorder()
		h.handleSetWebAnalyticsSettings(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
		assert.Equal(t, "Method not allowed", decodeErr(t, rr))
	})

	t.Run("invalid JSON is 400", func(t *testing.T) {
		h, _ := coverageWorkspaceHandler(t)
		rr := postSettings(h, `{bad`)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Equal(t, "Invalid request body", decodeErr(t, rr))
	})

	t.Run("missing workspace_id is 400 with exact validation message", func(t *testing.T) {
		h, _ := coverageWorkspaceHandler(t) // no service EXPECT: must not be reached
		rr := postSettings(h, `{"settings":{"enabled":false}}`)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Equal(t, "invalid set web analytics settings request: workspace_id is required", decodeErr(t, rr))
	})

	t.Run("non-alphanumeric workspace_id is 400", func(t *testing.T) {
		h, _ := coverageWorkspaceHandler(t)
		rr := postSettings(h, `{"workspace_id":"ws-1","settings":{"enabled":false}}`)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Equal(t, "invalid set web analytics settings request: workspace_id must be alphanumeric", decodeErr(t, rr))
	})

	t.Run("enabling without allowed domains is 400", func(t *testing.T) {
		h, _ := coverageWorkspaceHandler(t)
		rr := postSettings(h, `{"workspace_id":"ws123","settings":{"enabled":true}}`)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Equal(t, "allowed_domains must list at least one domain to enable web analytics", decodeErr(t, rr))
	})

	t.Run("negative bounce threshold is 400", func(t *testing.T) {
		h, _ := coverageWorkspaceHandler(t)
		rr := postSettings(h, `{"workspace_id":"ws123","settings":{"enabled":false,"bounce_threshold_seconds":-1}}`)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Equal(t, "bounce_threshold_seconds must be >= 0", decodeErr(t, rr))
	})

	t.Run("success forwards exact settings and invalidates cache once", func(t *testing.T) {
		h, svc := coverageWorkspaceHandler(t)
		svc.EXPECT().
			SetWebAnalyticsSettings(gomock.Any(), "ws123", &domain.WebAnalyticsSettings{Enabled: true, AllowedDomains: []string{"example.com"}}).
			Return(nil)
		var invalidated []string
		h.WithWebAnalyticsCacheInvalidator(func(id string) { invalidated = append(invalidated, id) })

		rr := postSettings(h, `{"workspace_id":"ws123","settings":{"enabled":true,"allowed_domains":["example.com"]}}`)

		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		assert.JSONEq(t, `{"status":"success","message":"Web analytics settings updated successfully"}`, rr.Body.String())
		assert.Equal(t, []string{"ws123"}, invalidated)
	})

	t.Run("nil settings clears config, still invalidates, service receives nil", func(t *testing.T) {
		h, svc := coverageWorkspaceHandler(t)
		svc.EXPECT().SetWebAnalyticsSettings(gomock.Any(), "ws123", gomock.Nil()).Return(nil)
		var invalidated []string
		h.WithWebAnalyticsCacheInvalidator(func(id string) { invalidated = append(invalidated, id) })

		rr := postSettings(h, `{"workspace_id":"ws123"}`)

		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		assert.Equal(t, []string{"ws123"}, invalidated)
	})

	t.Run("success without an invalidator does not panic", func(t *testing.T) {
		h, svc := coverageWorkspaceHandler(t)
		svc.EXPECT().SetWebAnalyticsSettings(gomock.Any(), "ws123", gomock.Any()).Return(nil)
		rr := postSettings(h, `{"workspace_id":"ws123","settings":{"enabled":false}}`)
		assert.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	})

	t.Run("permission error is 403 and cache is not invalidated", func(t *testing.T) {
		h, svc := coverageWorkspaceHandler(t)
		svc.EXPECT().SetWebAnalyticsSettings(gomock.Any(), "ws123", gomock.Any()).
			Return(domain.NewPermissionError(domain.PermissionResourceWebAnalytics, domain.PermissionTypeWrite, "no write grant"))
		invalidated := false
		h.WithWebAnalyticsCacheInvalidator(func(string) { invalidated = true })

		rr := postSettings(h, `{"workspace_id":"ws123","settings":{"enabled":false}}`)

		assert.Equal(t, http.StatusForbidden, rr.Code)
		var body map[string]interface{}
		require.NoError(t, json.NewDecoder(rr.Body).Decode(&body))
		assert.Equal(t, "no write grant", body["error"])
		assert.Equal(t, string(domain.PermissionResourceWebAnalytics), body["resource"])
		assert.Equal(t, string(domain.PermissionTypeWrite), body["permission"])
		assert.False(t, invalidated)
	})

	t.Run("unauthorized is 403 with the service message and no invalidation", func(t *testing.T) {
		h, svc := coverageWorkspaceHandler(t)
		svc.EXPECT().SetWebAnalyticsSettings(gomock.Any(), "ws123", gomock.Any()).
			Return(&domain.ErrUnauthorized{Message: "user is not a member of this workspace"})
		invalidated := false
		h.WithWebAnalyticsCacheInvalidator(func(string) { invalidated = true })

		rr := postSettings(h, `{"workspace_id":"ws123","settings":{"enabled":false}}`)

		assert.Equal(t, http.StatusForbidden, rr.Code)
		assert.Equal(t, "user is not a member of this workspace", decodeErr(t, rr))
		assert.False(t, invalidated)
	})

	t.Run("generic service error is 500 with generic message and no invalidation", func(t *testing.T) {
		h, svc := coverageWorkspaceHandler(t)
		svc.EXPECT().SetWebAnalyticsSettings(gomock.Any(), "ws123", gomock.Any()).
			Return(errors.New("db exploded"))
		invalidated := false
		h.WithWebAnalyticsCacheInvalidator(func(string) { invalidated = true })

		rr := postSettings(h, `{"workspace_id":"ws123","settings":{"enabled":false}}`)

		assert.Equal(t, http.StatusInternalServerError, rr.Code)
		// Internal detail must not leak to the caller.
		assert.Equal(t, "Failed to set web analytics settings", decodeErr(t, rr))
		assert.False(t, invalidated)
	})
}
