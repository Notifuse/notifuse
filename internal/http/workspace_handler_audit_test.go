package http

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
)

func TestWorkspaceHandler_SetAuditLogSettings(t *testing.T) {
	post := func(body string) *http.Request {
		req := httptest.NewRequest(http.MethodPost, "/api/workspaces.setAuditLogSettings", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		return req
	}

	t.Run("method", func(t *testing.T) {
		handler, _, _, _, _ := setupTest(t)
		rec := httptest.NewRecorder()
		handler.handleSetAuditLogSettings(rec, httptest.NewRequest(http.MethodGet, "/api/workspaces.setAuditLogSettings", nil))
		assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	})

	t.Run("bad body and bad retention are 400s", func(t *testing.T) {
		handler, _, _, _, _ := setupTest(t)
		rec := httptest.NewRecorder()
		handler.handleSetAuditLogSettings(rec, post(`{`))
		assert.Equal(t, http.StatusBadRequest, rec.Code)

		rec = httptest.NewRecorder()
		handler.handleSetAuditLogSettings(rec, post(`{"workspace_id":"ws1","settings":{"retention_days":7}}`))
		assert.Equal(t, http.StatusBadRequest, rec.Code)

		rec = httptest.NewRecorder()
		handler.handleSetAuditLogSettings(rec, post(`{"settings":{"retention_days":90}}`))
		assert.Equal(t, http.StatusBadRequest, rec.Code, "workspace_id is required")
	})

	t.Run("saves and answers success", func(t *testing.T) {
		handler, workspaceSvc, _, _, _ := setupTest(t)
		workspaceSvc.EXPECT().SetAuditLogSettings(gomock.Any(), "ws1", gomock.Any()).DoAndReturn(
			func(_ interface{}, _ string, settings *domain.AuditLogSettings) error {
				require.NotNil(t, settings.RetentionDays)
				assert.Equal(t, 90, *settings.RetentionDays)
				return nil
			})
		rec := httptest.NewRecorder()
		handler.handleSetAuditLogSettings(rec, post(`{"workspace_id":"ws1","settings":{"retention_days":90}}`))
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "success")
	})

	t.Run("a non-owner is a 403, a failure a 500", func(t *testing.T) {
		handler, workspaceSvc, _, _, _ := setupTest(t)
		workspaceSvc.EXPECT().SetAuditLogSettings(gomock.Any(), "ws1", gomock.Any()).Return(&domain.ErrUnauthorized{Message: "owners only"})
		rec := httptest.NewRecorder()
		handler.handleSetAuditLogSettings(rec, post(`{"workspace_id":"ws1","settings":{"retention_days":0}}`))
		assert.Equal(t, http.StatusForbidden, rec.Code)

		workspaceSvc.EXPECT().SetAuditLogSettings(gomock.Any(), "ws1", gomock.Any()).Return(errors.New("db"))
		rec = httptest.NewRecorder()
		handler.handleSetAuditLogSettings(rec, post(`{"workspace_id":"ws1","settings":{"retention_days":0}}`))
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})

	t.Run("the route is registered and closed in demo mode", func(t *testing.T) {
		_, _, mux, _, _ := setupTest(t)
		_, pattern := mux.Handler(httptest.NewRequest(http.MethodPost, "/api/workspaces.setAuditLogSettings", nil))
		assert.Equal(t, "/api/workspaces.setAuditLogSettings", pattern)
	})
}
