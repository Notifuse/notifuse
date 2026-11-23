package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/service"
)

func seedInstalled(repo *mockSettingRepository) {
	ctx := context.Background()
	_ = repo.Set(ctx, "is_installed", "true")
	_ = repo.Set(ctx, "root_email", testRootEmail)
}

func postSettings(t *testing.T, handler *SettingsHandler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := reqWithUserContext(httptest.NewRequest(http.MethodPost, "/api/settings.update", bytes.NewBufferString(body)), "root-user-id")
	w := httptest.NewRecorder()
	handler.handleUpdate(w, req)
	return w
}

// The root routes refuse a root session that the root check refuses, with the code the
// console switches on, never 401.
func TestSettingsHandler_RefusesANonCompliantRootSession(t *testing.T) {
	handler, repo, userSvc, _ := setupSettingsHandler(t)
	seedInstalled(repo)
	handler.authService = &sessionViaUserService{users: userSvc, rootErr: domain.ErrTwoFactorRequired}

	req := reqWithUserContext(httptest.NewRequest(http.MethodGet, "/api/settings.get", nil), "root-user-id")
	w := httptest.NewRecorder()
	handler.handleGet(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "two_factor_required")
}

func TestSettingsHandler_Get_TwoFactor(t *testing.T) {
	handler, repo, _, _ := setupSettingsHandler(t)
	seedInstalled(repo)
	handler.twoFactor = &stubTwoFactor{required: true, usersWithout: []string{"ann@example.com"}}
	handler.requireTwoFactorEnv = "true"

	req := reqWithUserContext(httptest.NewRequest(http.MethodGet, "/api/settings.get", nil), "root-user-id")
	w := httptest.NewRecorder()
	handler.handleGet(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp SystemSettingsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.True(t, resp.Settings.RequireTwoFactor)
	assert.True(t, resp.EnvOverrides["require_two_factor"])
	assert.Equal(t, TwoFactorSettings{Required: true, FromEnv: true, UsersWithout: []string{"ann@example.com"}}, resp.TwoFactor)
}

func TestSettingsHandler_Update_RequireTwoFactor(t *testing.T) {
	enrolled := &stubTwoFactor{status: &service.TwoFactorStatus{Enabled: true}}

	t.Run("a root user with two-factor authentication, from a session that passed it, turns it on", func(t *testing.T) {
		handler, repo, userSvc, _ := setupSettingsHandler(t)
		seedInstalled(repo)
		handler.twoFactor = enrolled
		handler.authService = &sessionViaUserService{users: userSvc, session: &domain.SessionAuth{AuthMethod: domain.AuthMethodMagicCode, SecondFactor: domain.SecondFactorTOTP}}

		w := postSettings(t, handler, `{"require_two_factor": true}`)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.Equal(t, "true", repo.settings["require_two_factor"])
	})

	// Otherwise the next sign-in would lock the root user out.
	t.Run("a root user without it is refused", func(t *testing.T) {
		handler, repo, userSvc, _ := setupSettingsHandler(t)
		seedInstalled(repo)
		handler.authService = &sessionViaUserService{users: userSvc}

		w := postSettings(t, handler, `{"require_two_factor": true}`)
		assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
		assert.Contains(t, w.Body.String(), "two_factor_required_for_root")
		_, written := repo.settings["require_two_factor"]
		assert.False(t, written)
	})

	t.Run("set by REQUIRE_TWO_FACTOR, it cannot be changed here", func(t *testing.T) {
		handler, repo, _, _ := setupSettingsHandler(t)
		seedInstalled(repo)
		handler.requireTwoFactorEnv = "false"

		w := postSettings(t, handler, `{"require_two_factor": true}`)
		assert.Equal(t, http.StatusConflict, w.Code)
	})

	// The console's form submits only the fields it registered: a save that does not carry
	// the key keeps the stored value.
	t.Run("a save without the key keeps the stored value", func(t *testing.T) {
		handler, repo, _, _ := setupSettingsHandler(t)
		seedInstalled(repo)
		_ = repo.Set(context.Background(), "require_two_factor", "true")
		handler.requireTwoFactorEnv = "true"

		w := postSettings(t, handler, `{"api_endpoint": "https://api.example.com"}`)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.Equal(t, "true", repo.settings["require_two_factor"])
	})

	// A save that does not carry the key, with REQUIRE_TWO_FACTOR differing from what is
	// stored: nothing to refuse, the switch was not touched.
	t.Run("set by REQUIRE_TWO_FACTOR, a save without the key goes through", func(t *testing.T) {
		handler, repo, _, _ := setupSettingsHandler(t)
		seedInstalled(repo)
		handler.requireTwoFactorEnv = "true"

		w := postSettings(t, handler, `{"api_endpoint": "https://api.example.com"}`)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	})

	// GET shows the value in force: saving the form sends it back unchanged.
	t.Run("set by REQUIRE_TWO_FACTOR, sending its value back saves the rest", func(t *testing.T) {
		handler, repo, _, _ := setupSettingsHandler(t)
		seedInstalled(repo)
		handler.requireTwoFactorEnv = "true"

		w := postSettings(t, handler, `{"require_two_factor": true, "api_endpoint": "https://api.example.com"}`)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		_, written := repo.settings["require_two_factor"]
		assert.False(t, written, "the environment decides; the stored value is left alone")
	})

	t.Run("turning it off needs no check", func(t *testing.T) {
		handler, repo, _, _ := setupSettingsHandler(t)
		seedInstalled(repo)
		_ = repo.Set(context.Background(), "require_two_factor", "true")

		w := postSettings(t, handler, `{"require_two_factor": false}`)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.Equal(t, "false", repo.settings["require_two_factor"])
	})
}
