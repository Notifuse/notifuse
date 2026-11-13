package http

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
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

// automationEndpoint describes one automation route: how a valid request looks,
// and how to make the service answer it with an error.
type automationEndpoint struct {
	name   string
	method string
	path   string
	// query is the URL query of a valid GET request.
	query string
	// body is the JSON body of a valid POST request.
	body func(t *testing.T) []byte
	// invalidBody is a well-formed body that fails Validate, for POST routes
	// whose validation is not covered elsewhere.
	invalidBody string
	// fails makes the service answer the valid request with err.
	fails func(svc *mocks.MockAutomationService, err error)
}

func jsonBody(v interface{}) func(t *testing.T) []byte {
	return func(t *testing.T) []byte {
		b, err := json.Marshal(v)
		require.NoError(t, err)
		return b
	}
}

func automationEndpoints() []automationEndpoint {
	automation := createTestAutomation("auto-123", "workspace-123")
	idRequest := map[string]string{"workspace_id": "workspace-123", "automation_id": "auto-123"}
	return []automationEndpoint{
		{
			name: "create", method: http.MethodPost, path: "/api/automations.create",
			body: jsonBody(domain.CreateAutomationRequest{WorkspaceID: "workspace-123", Automation: automation}),
			fails: func(svc *mocks.MockAutomationService, err error) {
				svc.EXPECT().Create(gomock.Any(), "workspace-123", gomock.Any()).Return(err)
			},
		},
		{
			name: "get", method: http.MethodGet, path: "/api/automations.get",
			query: "workspace_id=workspace-123&automation_id=auto-123",
			fails: func(svc *mocks.MockAutomationService, err error) {
				svc.EXPECT().Get(gomock.Any(), "workspace-123", "auto-123").Return(nil, err)
			},
		},
		{
			name: "list", method: http.MethodGet, path: "/api/automations.list",
			query: "workspace_id=workspace-123",
			fails: func(svc *mocks.MockAutomationService, err error) {
				svc.EXPECT().List(gomock.Any(), "workspace-123", gomock.Any()).Return(nil, 0, err)
			},
		},
		{
			name: "update", method: http.MethodPost, path: "/api/automations.update",
			body: jsonBody(domain.UpdateAutomationRequest{WorkspaceID: "workspace-123", Automation: automation}),
			fails: func(svc *mocks.MockAutomationService, err error) {
				svc.EXPECT().Update(gomock.Any(), "workspace-123", gomock.Any()).Return(err)
			},
		},
		{
			name: "delete", method: http.MethodPost, path: "/api/automations.delete",
			body: jsonBody(idRequest), invalidBody: `{"workspace_id":"workspace-123"}`,
			fails: func(svc *mocks.MockAutomationService, err error) {
				svc.EXPECT().Delete(gomock.Any(), "workspace-123", "auto-123").Return(err)
			},
		},
		{
			name: "activate", method: http.MethodPost, path: "/api/automations.activate",
			body: jsonBody(idRequest), invalidBody: `{"automation_id":"auto-123"}`,
			fails: func(svc *mocks.MockAutomationService, err error) {
				svc.EXPECT().Activate(gomock.Any(), "workspace-123", "auto-123").Return(err)
			},
		},
		{
			name: "pause", method: http.MethodPost, path: "/api/automations.pause",
			body: jsonBody(idRequest), invalidBody: `{"workspace_id":"workspace-123"}`,
			fails: func(svc *mocks.MockAutomationService, err error) {
				svc.EXPECT().Pause(gomock.Any(), "workspace-123", "auto-123").Return(err)
			},
		},
		{
			name: "nodeExecutions", method: http.MethodGet, path: "/api/automations.nodeExecutions",
			query: "workspace_id=workspace-123&automation_id=auto-123&email=contact@example.com",
			fails: func(svc *mocks.MockAutomationService, err error) {
				svc.EXPECT().GetContactNodeExecutions(gomock.Any(), "workspace-123", "auto-123", "contact@example.com").
					Return(nil, nil, err)
			},
		},
	}
}

// newAutomationRoutes returns the automation routes over a strict service mock:
// a request the handler must refuse before the service fails the test if it
// reaches it.
func newAutomationRoutes(t *testing.T) (*mocks.MockAutomationService, *http.ServeMux, string) {
	ctrl := gomock.NewController(t)
	svc := mocks.NewMockAutomationService(ctrl)
	secret := []byte("test-jwt-secret-key-for-testing-32bytes")
	handler := NewAutomationHandler(svc, func() ([]byte, error) { return secret, nil }, newPermissiveLogger(ctrl))
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	return svc, mux, createTestToken(t, secret, "test-user")
}

// newPermissiveLogger is a mock logger that accepts every log call.
func newPermissiveLogger(ctrl *gomock.Controller) *pkgmocks.MockLogger {
	log := pkgmocks.NewMockLogger(ctrl)
	log.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(log).AnyTimes()
	log.EXPECT().WithFields(gomock.Any()).Return(log).AnyTimes()
	log.EXPECT().Error(gomock.Any()).AnyTimes()
	log.EXPECT().Info(gomock.Any()).AnyTimes()
	log.EXPECT().Debug(gomock.Any()).AnyTimes()
	log.EXPECT().Warn(gomock.Any()).AnyTimes()
	return log
}

func serveAutomation(mux *http.ServeMux, token, method, target string, body io.Reader) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, body)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

func (e automationEndpoint) validTarget() string {
	if e.query != "" {
		return e.path + "?" + e.query
	}
	return e.path
}

func (e automationEndpoint) validBody(t *testing.T) io.Reader {
	if e.body == nil {
		return nil
	}
	return bytes.NewReader(e.body(t))
}

func TestAutomationHandler_WrongMethodIsRefused(t *testing.T) {
	for _, e := range automationEndpoints() {
		t.Run(e.name, func(t *testing.T) {
			_, mux, token := newAutomationRoutes(t)
			wrong := http.MethodPost
			if e.method == http.MethodPost {
				wrong = http.MethodGet
			}

			w := serveAutomation(mux, token, wrong, e.validTarget(), e.validBody(t))

			assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
		})
	}
}

func TestAutomationHandler_MalformedJSONIsRefused(t *testing.T) {
	for _, e := range automationEndpoints() {
		if e.method != http.MethodPost {
			continue
		}
		t.Run(e.name, func(t *testing.T) {
			_, mux, token := newAutomationRoutes(t)

			w := serveAutomation(mux, token, http.MethodPost, e.path, bytes.NewReader([]byte(`{"workspace_id":`)))

			assert.Equal(t, http.StatusBadRequest, w.Code)
			assert.Equal(t, "Invalid request body", decodeErrorMessage(t, w))
		})
	}
}

func TestAutomationHandler_InvalidRequestIsRefused(t *testing.T) {
	for _, e := range automationEndpoints() {
		if e.invalidBody == "" {
			continue
		}
		t.Run(e.name, func(t *testing.T) {
			_, mux, token := newAutomationRoutes(t)

			w := serveAutomation(mux, token, http.MethodPost, e.path, bytes.NewReader([]byte(e.invalidBody)))

			assert.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

// Each route in the table answers a denied permission with 403 and a revoked API
// key with 401, wrapped or not, instead of a generic 500.
func TestAutomationHandler_AuthErrorsKeepTheirStatus(t *testing.T) {
	permissionErr := domain.NewPermissionError(domain.PermissionResourceAutomations, domain.PermissionTypeWrite, "user lacks automations access")
	cases := []struct {
		name   string
		err    error
		status int
	}{
		{name: "permission denied", err: permissionErr, status: http.StatusForbidden},
		{name: "wrapped permission denied", err: fmt.Errorf("failed to authorize: %w", permissionErr), status: http.StatusForbidden},
		{name: "revoked API key", err: fmt.Errorf("failed to authenticate: %w", domain.ErrAPIKeyRevoked), status: http.StatusUnauthorized},
	}
	for _, e := range automationEndpoints() {
		for _, tc := range cases {
			t.Run(e.name+"/"+tc.name, func(t *testing.T) {
				svc, mux, token := newAutomationRoutes(t)
				e.fails(svc, tc.err)

				w := serveAutomation(mux, token, e.method, e.validTarget(), e.validBody(t))

				assert.Equal(t, tc.status, w.Code)
			})
		}
	}
}
