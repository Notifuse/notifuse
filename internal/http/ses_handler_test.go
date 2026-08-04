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

func setupSESHandler(t *testing.T) (*SESHandler, *mocks.MockSESDiscoveryServiceInterface) {
	ctrl := gomock.NewController(t)
	service := mocks.NewMockSESDiscoveryServiceInterface(ctrl)
	logger := pkgmocks.NewMockLogger(ctrl)
	logger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(logger).AnyTimes()
	logger.EXPECT().Error(gomock.Any()).AnyTimes()
	logger.EXPECT().Warn(gomock.Any()).AnyTimes()
	logger.EXPECT().Info(gomock.Any()).AnyTimes()
	logger.EXPECT().Debug(gomock.Any()).AnyTimes()

	getJWTSecret := func() ([]byte, error) { return []byte("test-secret"), nil }
	return NewSESHandler(service, getJWTSecret, logger), service
}

func post(t *testing.T, handler http.HandlerFunc, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/ses.listTenants", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler(rec, req)
	return rec
}

func TestSESHandler_ListTenants(t *testing.T) {
	t.Run("returns the tenant list", func(t *testing.T) {
		handler, service := setupSESHandler(t)

		service.EXPECT().ListTenants(gomock.Any(), gomock.Any()).
			Return(&domain.ListSESTenantsResponse{
				Tenants: []domain.SESTenant{{Name: "notifuse-int-1"}},
			}, nil)

		rec := post(t, handler.handleListTenants, `{"workspace_id":"ws","integration_id":"int-1"}`)

		require.Equal(t, http.StatusOK, rec.Code)
		var got domain.ListSESTenantsResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
		require.Len(t, got.Tenants, 1)
		assert.Equal(t, "notifuse-int-1", got.Tenants[0].Name)
	})

	t.Run("GET is refused", func(t *testing.T) {
		handler, _ := setupSESHandler(t)

		req := httptest.NewRequest(http.MethodGet, "/api/ses.listTenants", nil)
		rec := httptest.NewRecorder()
		handler.handleListTenants(rec, req)

		assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	})

	t.Run("malformed body", func(t *testing.T) {
		handler, _ := setupSESHandler(t)
		rec := post(t, handler.handleListTenants, `{not json`)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	// The degradation contract: the console must be able to tell "you may not list these"
	// apart from "your credentials are wrong", because the first is not an error to show.
	t.Run("IAM denial is a distinguishable 403", func(t *testing.T) {
		handler, service := setupSESHandler(t)

		service.EXPECT().ListTenants(gomock.Any(), gomock.Any()).
			Return(nil, domain.ErrSESAccessDenied)

		rec := post(t, handler.handleListTenants, `{"workspace_id":"ws","integration_id":"int-1"}`)

		require.Equal(t, http.StatusForbidden, rec.Code)
		var body map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		assert.Equal(t, "ses_access_denied", body["code"])
	})

	t.Run("non-owner is refused without the degradation code", func(t *testing.T) {
		handler, service := setupSESHandler(t)

		service.EXPECT().ListTenants(gomock.Any(), gomock.Any()).
			Return(nil, &domain.ErrUnauthorized{Message: "user is not an owner of the workspace"})

		rec := post(t, handler.handleListTenants, `{"workspace_id":"ws","integration_id":"int-1"}`)

		require.Equal(t, http.StatusForbidden, rec.Code)
		assert.NotContains(t, rec.Body.String(), "ses_access_denied")
	})

	t.Run("other AWS failures are not silently swallowed", func(t *testing.T) {
		handler, service := setupSESHandler(t)

		service.EXPECT().ListTenants(gomock.Any(), gomock.Any()).
			Return(nil, errors.New("SES error: InternalServiceErrorException"))

		rec := post(t, handler.handleListTenants, `{"workspace_id":"ws","integration_id":"int-1"}`)

		assert.Equal(t, http.StatusBadGateway, rec.Code)
	})
}

func TestSESHandler_VerifyTenant(t *testing.T) {
	t.Run("reports a missing association with a fix", func(t *testing.T) {
		handler, service := setupSESHandler(t)

		service.EXPECT().VerifyTenant(gomock.Any(), gomock.Any()).
			Return(&domain.SESTenantVerification{
				TenantName:                 "team-acme",
				Exists:                     true,
				ConfigurationSetAssociated: false,
				SuppressionScope:           "ACCOUNT",
				FixCommand:                 "aws sesv2 create-tenant-resource-association ...",
			}, nil)

		rec := post(t, handler.handleVerifyTenant,
			`{"workspace_id":"ws","integration_id":"int-1","tenant_name":"team-acme"}`)

		require.Equal(t, http.StatusOK, rec.Code)
		var got domain.SESTenantVerification
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
		assert.True(t, got.Exists)
		assert.False(t, got.ConfigurationSetAssociated)
		assert.Equal(t, "ACCOUNT", got.SuppressionScope)
		assert.NotEmpty(t, got.FixCommand)
	})

	t.Run("tenant_name is required", func(t *testing.T) {
		handler, _ := setupSESHandler(t)
		rec := post(t, handler.handleVerifyTenant, `{"workspace_id":"ws","integration_id":"int-1"}`)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
}

func TestSESHandler_EnableTenantIsolation(t *testing.T) {
	t.Run("returns the provisioning result", func(t *testing.T) {
		handler, service := setupSESHandler(t)

		service.EXPECT().EnableTenantIsolation(gomock.Any(), gomock.Any()).
			Return(&domain.SESTenantProvisionResult{
				TenantName:        "notifuse-int-1",
				Created:           true,
				SuppressionScoped: true,
				Associated:        []string{"arn:aws:ses:eu-west-3:1:configuration-set/notifuse-int-1"},
			}, nil)

		rec := post(t, handler.handleEnableTenantIsolation, `{"workspace_id":"ws","integration_id":"int-1"}`)

		require.Equal(t, http.StatusOK, rec.Code)
		var got domain.SESTenantProvisionResult
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
		assert.True(t, got.Created)
		assert.True(t, got.SuppressionScoped)
	})

	// Provisioning creates a billable AWS resource and writes derived state back onto a stored
	// integration, so it must never run against credentials typed into an unsaved form.
	t.Run("inline credentials are refused", func(t *testing.T) {
		handler, _ := setupSESHandler(t)

		rec := post(t, handler.handleEnableTenantIsolation,
			`{"workspace_id":"ws","region":"eu-west-3","access_key":"AKIA","secret_key":"s"}`)

		require.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "integration_id")
	})

	t.Run("partial provisioning is reported, not hidden", func(t *testing.T) {
		handler, service := setupSESHandler(t)

		service.EXPECT().EnableTenantIsolation(gomock.Any(), gomock.Any()).
			Return(&domain.SESTenantProvisionResult{
				TenantName:            "notifuse-int-1",
				Created:               true,
				ProvisionedButUnsaved: true,
				MissingPermissions:    []string{"ses:CreateTenantResourceAssociation"},
				FixCommands:           []string{"aws sesv2 create-tenant-resource-association ..."},
			}, nil)

		rec := post(t, handler.handleEnableTenantIsolation, `{"workspace_id":"ws","integration_id":"int-1"}`)

		require.Equal(t, http.StatusOK, rec.Code, "a partial result is a result, not an error")
		var got domain.SESTenantProvisionResult
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
		assert.True(t, got.ProvisionedButUnsaved)
		assert.Contains(t, got.MissingPermissions, "ses:CreateTenantResourceAssociation")
	})
}
