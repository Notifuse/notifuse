package http

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
)

// The handlers that ran their own error chains now answer an authentication or
// authorization refusal with its status: a dead session 401, a missing second factor or
// grant 403, and only what is left 500 (or the handler's own fallback).

var authMatrix = []struct {
	name   string
	err    error
	status int
	code   string
}{
	{"session gone", fmt.Errorf("%w: expired", domain.ErrSessionAuthFailed), http.StatusUnauthorized, ""},
	{"second factor required", fmt.Errorf("failed to authenticate user: %w", domain.ErrTwoFactorRequired), http.StatusForbidden, "two_factor_required"},
	{"missing grant", domain.NewPermissionError(domain.PermissionResourceTemplates, domain.PermissionTypeRead, "no"), http.StatusForbidden, ""},
}

func quietLogger(ctrl *gomock.Controller) *pkgmocks.MockLogger {
	log := pkgmocks.NewMockLogger(ctrl)
	log.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(log).AnyTimes()
	log.EXPECT().WithFields(gomock.Any()).Return(log).AnyTimes()
	log.EXPECT().Error(gomock.Any()).AnyTimes()
	log.EXPECT().Warn(gomock.Any()).AnyTimes()
	log.EXPECT().Info(gomock.Any()).AnyTimes()
	log.EXPECT().Debug(gomock.Any()).AnyTimes()
	return log
}

func assertAuthAnswer(t *testing.T, w *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	assert.Equal(t, status, w.Code, w.Body.String())
	if code != "" {
		var body map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Equal(t, code, body["error"])
	}
}

func TestAnnexA_Templates(t *testing.T) {
	for _, tc := range authMatrix {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			svc := mocks.NewMockTemplateService(ctrl)
			h := NewTemplateHandler(svc, func() ([]byte, error) { return []byte("s"), nil }, quietLogger(ctrl))

			svc.EXPECT().GetTemplates(gomock.Any(), "ws1", "", "").Return(nil, tc.err)
			w := httptest.NewRecorder()
			h.handleList(w, httptest.NewRequest(http.MethodGet, "/api/templates.list?workspace_id=ws1", nil))
			assertAuthAnswer(t, w, tc.status, tc.code)

			svc.EXPECT().GetTemplateByID(gomock.Any(), "ws1", "t1", gomock.Any()).Return(nil, tc.err)
			w = httptest.NewRecorder()
			h.handleGet(w, httptest.NewRequest(http.MethodGet, "/api/templates.get?workspace_id=ws1&id=t1", nil))
			assertAuthAnswer(t, w, tc.status, tc.code)
		})
	}
}

func TestAnnexA_TemplateBlocks(t *testing.T) {
	for _, tc := range authMatrix {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			svc := mocks.NewMockTemplateBlockService(ctrl)
			h := NewTemplateBlockHandler(svc, func() ([]byte, error) { return []byte("s"), nil }, quietLogger(ctrl))

			svc.EXPECT().ListTemplateBlocks(gomock.Any(), "ws1").Return(nil, tc.err)
			w := httptest.NewRecorder()
			h.handleList(w, httptest.NewRequest(http.MethodGet, "/api/templateBlocks.list?workspace_id=ws1", nil))
			assertAuthAnswer(t, w, tc.status, tc.code)

			svc.EXPECT().GetTemplateBlock(gomock.Any(), "ws1", "b1").Return(nil, tc.err)
			w = httptest.NewRecorder()
			h.handleGet(w, httptest.NewRequest(http.MethodGet, "/api/templateBlocks.get?workspace_id=ws1&id=b1", nil))
			assertAuthAnswer(t, w, tc.status, tc.code)
		})
	}
}

func TestAnnexA_SES(t *testing.T) {
	for _, tc := range authMatrix {
		t.Run(tc.name, func(t *testing.T) {
			h, _ := setupSESHandler(t)
			w := httptest.NewRecorder()
			h.writeError(w, tc.err, "Failed to list tenants")
			assertAuthAnswer(t, w, tc.status, tc.code)
		})
	}
}

func TestAnnexA_BroadcastUpdate(t *testing.T) {
	for _, tc := range authMatrix {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			svc := mocks.NewMockBroadcastService(ctrl)
			h := NewBroadcastHandler(svc, func() ([]byte, error) { return []byte("s"), nil }, quietLogger(ctrl), false)
			svc.EXPECT().GetBroadcast(gomock.Any(), "ws1", "b1", gomock.Any()).Return(nil, tc.err)

			w := httptest.NewRecorder()
			h.HandleUpdate(w, httptest.NewRequest(http.MethodPost, "/api/broadcasts.update", bytes.NewBufferString(`{"workspace_id":"ws1","id":"b1"}`)))
			assertAuthAnswer(t, w, tc.status, tc.code)
		})
	}
}

func TestAnnexA_WorkspaceRoutes(t *testing.T) {
	for _, tc := range authMatrix {
		t.Run(tc.name, func(t *testing.T) {
			h, svc, _, _, _ := setupTest(t)

			svc.EXPECT().CreateAPIKey(gomock.Any(), "ws1", "zap", gomock.Any()).Return("", "", tc.err)
			w := httptest.NewRecorder()
			h.handleCreateAPIKey(w, httptest.NewRequest(http.MethodPost, "/api/workspaces.createAPIKey", bytes.NewBufferString(`{"workspace_id":"ws1","email_prefix":"zap"}`)))
			assertAuthAnswer(t, w, tc.status, tc.code)

			svc.EXPECT().DeleteInvitation(gomock.Any(), "inv1").Return(tc.err)
			w = httptest.NewRecorder()
			h.handleDeleteInvitation(w, httptest.NewRequest(http.MethodPost, "/api/workspaces.deleteInvitation", bytes.NewBufferString(`{"invitation_id":"inv1"}`)))
			assertAuthAnswer(t, w, tc.status, tc.code)
		})
	}
}
