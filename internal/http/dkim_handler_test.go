package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
)

func setupDKIMHandlerTest(t *testing.T, isDemo bool) (*mocks.MockDKIMService, *DKIMHandler) {
	t.Helper()
	ctrl := gomock.NewController(t)
	service := mocks.NewMockDKIMService(ctrl)
	log := pkgmocks.NewMockLogger(ctrl)
	log.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(log).AnyTimes()
	log.EXPECT().Error(gomock.Any()).AnyTimes()
	secret := []byte("test-jwt-secret-key-for-testing-32bytes")
	return service, NewDKIMHandler(service, func() ([]byte, error) { return secret, nil }, log, isDemo)
}

func dkimView() *domain.DKIMDomainView {
	v := domain.NewDKIMDomainView(domain.DKIMKey{
		Domain: "acme.io", Selector: "s1", PublicKey: "PUB", EncryptedPrivateKey: "SENTINEL-PRIVATE", Status: domain.DKIMKeyStatusPending,
	}, nil)
	return &v
}

func TestDKIMHandler_Routes(t *testing.T) {
	_, handler := setupDKIMHandlerTest(t, false)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	for _, path := range []string{
		"/api/workspaces.listDKIMDomains", "/api/workspaces.createDKIMDomain", "/api/workspaces.regenerateDKIMKey",
		"/api/workspaces.verifyDKIMDomain", "/api/workspaces.deleteDKIMDomain",
	} {
		_, pattern := mux.Handler(httptest.NewRequest(http.MethodPost, path, nil))
		assert.Equal(t, path, pattern)
	}

	t.Run("they need a session", func(t *testing.T) {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/workspaces.listDKIMDomains?workspace_id=ws-1", nil))
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})
}

func TestDKIMHandler_ChangesAreClosedInDemo(t *testing.T) {
	_, handler := setupDKIMHandlerTest(t, true)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	for _, path := range []string{"/api/workspaces.createDKIMDomain", "/api/workspaces.regenerateDKIMKey", "/api/workspaces.verifyDKIMDomain", "/api/workspaces.deleteDKIMDomain"} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"workspace_id":"ws-1","domain":"acme.io"}`)))
		assert.Equal(t, http.StatusBadRequest, rec.Code, path)
		assert.Contains(t, rec.Body.String(), "demo mode")
	}
}

func TestDKIMHandler_List(t *testing.T) {
	service, handler := setupDKIMHandlerTest(t, false)
	service.EXPECT().ListDomains(gomock.Any(), "ws-1").Return([]domain.DKIMDomainView{*dkimView()}, nil)

	rec := httptest.NewRecorder()
	handler.handleList(rec, httptest.NewRequest(http.MethodGet, "/api/workspaces.listDKIMDomains?workspace_id=ws-1", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, `"domain":"acme.io"`)
	assert.Contains(t, body, `"dns_record"`)
	assert.Contains(t, body, `"used_by":[]`)
	assert.NotContains(t, body, "SENTINEL-PRIVATE")
	assert.NotContains(t, body, "private_key")

	rec = httptest.NewRecorder()
	handler.handleList(rec, httptest.NewRequest(http.MethodGet, "/api/workspaces.listDKIMDomains", nil))
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	rec = httptest.NewRecorder()
	handler.handleList(rec, httptest.NewRequest(http.MethodPost, "/api/workspaces.listDKIMDomains?workspace_id=ws-1", nil))
	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

func TestDKIMHandler_Mutations(t *testing.T) {
	body := `{"workspace_id":"ws-1","domain":"acme.io"}`
	post := func(path string, b string) *http.Request {
		return httptest.NewRequest(http.MethodPost, path, strings.NewReader(b))
	}

	t.Run("create answers 201 with the domain", func(t *testing.T) {
		service, handler := setupDKIMHandlerTest(t, false)
		service.EXPECT().CreateDomain(gomock.Any(), "ws-1", "acme.io").Return(dkimView(), nil)
		rec := httptest.NewRecorder()
		handler.handleCreate(rec, post("/api/workspaces.createDKIMDomain", body))
		assert.Equal(t, http.StatusCreated, rec.Code)
		assert.Contains(t, rec.Body.String(), `"selector":"s1"`)
		assert.NotContains(t, rec.Body.String(), "SENTINEL-PRIVATE")
	})

	t.Run("regenerate", func(t *testing.T) {
		service, handler := setupDKIMHandlerTest(t, false)
		service.EXPECT().RegenerateKey(gomock.Any(), "ws-1", "acme.io").Return(dkimView(), nil)
		rec := httptest.NewRecorder()
		handler.handleRegenerate(rec, post("/api/workspaces.regenerateDKIMKey", body))
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("verify returns the check and the domain", func(t *testing.T) {
		service, handler := setupDKIMHandlerTest(t, false)
		service.EXPECT().VerifyDomain(gomock.Any(), "ws-1", "acme.io").Return(&domain.DKIMVerifyResult{
			Domain: *dkimView(), Check: domain.DKIMCheck{Result: domain.DKIMCheckResultNotFound, Detail: "no TXT record found"},
		}, nil)
		rec := httptest.NewRecorder()
		handler.handleVerify(rec, post("/api/workspaces.verifyDKIMDomain", body))
		require.Equal(t, http.StatusOK, rec.Code)
		var got map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
		assert.Equal(t, "not_found", got["check"].(map[string]any)["result"])
		assert.Equal(t, "acme.io", got["domain"].(map[string]any)["domain"])
	})

	t.Run("delete", func(t *testing.T) {
		service, handler := setupDKIMHandlerTest(t, false)
		service.EXPECT().DeleteDomain(gomock.Any(), "ws-1", "acme.io").Return(nil)
		rec := httptest.NewRecorder()
		handler.handleDelete(rec, post("/api/workspaces.deleteDKIMDomain", body))
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("a body without a domain is refused before the service", func(t *testing.T) {
		_, handler := setupDKIMHandlerTest(t, false)
		rec := httptest.NewRecorder()
		handler.handleCreate(rec, post("/api/workspaces.createDKIMDomain", `{"workspace_id":"ws-1"}`))
		assert.Equal(t, http.StatusBadRequest, rec.Code)

		rec = httptest.NewRecorder()
		handler.handleCreate(rec, post("/api/workspaces.createDKIMDomain", `not json`))
		assert.Equal(t, http.StatusBadRequest, rec.Code)

		rec = httptest.NewRecorder()
		handler.handleCreate(rec, httptest.NewRequest(http.MethodGet, "/api/workspaces.createDKIMDomain", nil))
		assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	})
}

func TestDKIMHandler_ErrorCodes(t *testing.T) {
	cases := []struct {
		name string
		err  error
		code int
	}{
		{"not an owner", &domain.ErrUnauthorized{Message: "Only workspace owners can manage DKIM keys"}, http.StatusForbidden},
		{"invalid domain", &domain.ErrDKIMInvalidDomain{Message: "bad"}, http.StatusBadRequest},
		{"unknown domain", &domain.ErrDKIMDomainNotFound{Domain: "acme.io"}, http.StatusNotFound},
		{"unknown workspace", &domain.ErrWorkspaceNotFound{WorkspaceID: "ws-1"}, http.StatusNotFound},
		{"conflict", &domain.ErrDKIMConflict{Message: "in use"}, http.StatusConflict},
		{"anything else", errors.New("db down"), http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service, handler := setupDKIMHandlerTest(t, false)
			service.EXPECT().DeleteDomain(gomock.Any(), "ws-1", "acme.io").Return(tc.err)
			rec := httptest.NewRecorder()
			handler.handleDelete(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"workspace_id":"ws-1","domain":"acme.io"}`)))
			assert.Equal(t, tc.code, rec.Code)
			if tc.code == http.StatusInternalServerError {
				assert.NotContains(t, rec.Body.String(), "db down")
			}
		})
	}

	t.Run("a refused deletion names the integrations that use the domain", func(t *testing.T) {
		service, handler := setupDKIMHandlerTest(t, false)
		service.EXPECT().DeleteDomain(gomock.Any(), "ws-1", "acme.io").Return(&domain.ErrDKIMConflict{
			Message: "acme.io is used", UsedBy: []domain.DKIMDomainUser{{IntegrationID: "smtp1", IntegrationName: "Relay"}},
		})
		rec := httptest.NewRecorder()
		handler.handleDelete(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"workspace_id":"ws-1","domain":"acme.io"}`)))
		assert.Equal(t, http.StatusConflict, rec.Code)
		assert.JSONEq(t, `{"error":"acme.io is used","used_by":[{"integration_id":"smtp1","integration_name":"Relay"}]}`, rec.Body.String())
	})
}
