package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCoverage_SESRegisterRoutes_RequireAuth(t *testing.T) {
	handler, _ := setupSESHandler(t)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	routes := []string{
		"/api/ses.listTenants",
		"/api/ses.listConfigurationSets",
		"/api/ses.verifyTenant",
		"/api/ses.enableTenantIsolation",
	}
	for _, route := range routes {
		t.Run(route, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, route, strings.NewReader(`{}`))
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			assert.Equal(t, http.StatusUnauthorized, rec.Code, "401 not 404 proves registration + auth wrap")
			var body map[string]string
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&body))
			assert.Equal(t, "Authorization header is required", body["error"])
		})
	}

	t.Run("unregistered SES route is 404", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/ses.nope", strings.NewReader(`{}`))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})
}

func TestCoverage_SESListConfigurationSets(t *testing.T) {
	t.Run("returns sets and forwards the exact credentials ref", func(t *testing.T) {
		handler, service := setupSESHandler(t)
		service.EXPECT().
			ListConfigurationSets(gomock.Any(), domain.SESCredentialsRef{WorkspaceID: "ws", IntegrationID: "int-1"}).
			Return(&domain.ListSESConfigurationSetsResponse{
				ConfigurationSets: []string{"notifuse-default", "tenant-acme"},
				HasMore:           true,
			}, nil)

		rec := post(t, handler.handleListConfigurationSets, `{"workspace_id":"ws","integration_id":"int-1"}`)

		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		var got domain.ListSESConfigurationSetsResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
		assert.Equal(t, []string{"notifuse-default", "tenant-acme"}, got.ConfigurationSets)
		assert.True(t, got.HasMore)
	})

	t.Run("forwards inline credentials verbatim", func(t *testing.T) {
		handler, service := setupSESHandler(t)
		service.EXPECT().
			ListConfigurationSets(gomock.Any(), domain.SESCredentialsRef{
				WorkspaceID: "ws", Region: "eu-west-3", AccessKey: "AKIAEXAMPLE", SecretKey: "s3cr3t",
			}).
			Return(&domain.ListSESConfigurationSetsResponse{ConfigurationSets: []string{}}, nil)

		rec := post(t, handler.handleListConfigurationSets, `{"workspace_id":"ws","region":"eu-west-3","access_key":"AKIAEXAMPLE","secret_key":"s3cr3t"}`)

		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		assert.JSONEq(t, `{"configuration_sets":[],"has_more":false}`, rec.Body.String())
	})

	t.Run("GET is refused with 405", func(t *testing.T) {
		handler, _ := setupSESHandler(t)
		req := httptest.NewRequest(http.MethodGet, "/api/ses.listConfigurationSets", nil)
		rec := httptest.NewRecorder()
		handler.handleListConfigurationSets(rec, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
		var body map[string]string
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&body))
		assert.Equal(t, "Method not allowed", body["error"])
	})

	t.Run("malformed body is 400", func(t *testing.T) {
		handler, _ := setupSESHandler(t)
		rec := post(t, handler.handleListConfigurationSets, `{not json`)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		var body map[string]string
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&body))
		assert.Equal(t, "Invalid request body", body["error"])
	})

	t.Run("IAM denial is 403 with the degradation code", func(t *testing.T) {
		handler, service := setupSESHandler(t)
		service.EXPECT().ListConfigurationSets(gomock.Any(), gomock.Any()).
			Return(nil, domain.ErrSESAccessDenied)

		rec := post(t, handler.handleListConfigurationSets, `{"workspace_id":"ws","integration_id":"int-1"}`)

		require.Equal(t, http.StatusForbidden, rec.Code)
		var body map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		assert.Equal(t, "ses_access_denied", body["code"])
		assert.Equal(t, domain.ErrSESAccessDenied.Error(), body["error"])
	})

	t.Run("non-owner is 403 without the degradation code", func(t *testing.T) {
		handler, service := setupSESHandler(t)
		service.EXPECT().ListConfigurationSets(gomock.Any(), gomock.Any()).
			Return(nil, &domain.ErrUnauthorized{Message: "user is not an owner of the workspace"})

		rec := post(t, handler.handleListConfigurationSets, `{"workspace_id":"ws","integration_id":"int-1"}`)

		require.Equal(t, http.StatusForbidden, rec.Code)
		var body map[string]string
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&body))
		assert.Equal(t, "user is not an owner of the workspace", body["error"])
		assert.NotContains(t, rec.Body.String(), "ses_access_denied")
	})

	t.Run("other AWS failures are 502 with prefixed context", func(t *testing.T) {
		handler, service := setupSESHandler(t)
		service.EXPECT().ListConfigurationSets(gomock.Any(), gomock.Any()).
			Return(nil, errors.New("SES error: InternalServiceErrorException"))

		rec := post(t, handler.handleListConfigurationSets, `{"workspace_id":"ws","integration_id":"int-1"}`)

		assert.Equal(t, http.StatusBadGateway, rec.Code)
		var body map[string]string
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&body))
		assert.Equal(t, "Failed to list SES configuration sets: SES error: InternalServiceErrorException", body["error"])
	})
}
