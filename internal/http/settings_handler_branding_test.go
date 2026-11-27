package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/pkg/branding"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type brandingGrantStub struct{ features []domain.Feature }

func (b brandingGrantStub) Entitlements() domain.Entitlements {
	return domain.Entitlements{Tier: "agency", MaxWorkspaces: 15, Features: b.features, State: domain.LicenseStateActive, ExpiresAt: time.Now().Add(time.Hour)}
}

// brandingSettingsHandler is the settings handler of an installed instance, licensed for
// branding or not.
func brandingSettingsHandler(t *testing.T, licensed bool) (*SettingsHandler, *mockSettingRepository, *mockAppShutdowner) {
	t.Helper()
	handler, repo, _, shutdowner := setupSettingsHandler(t)
	ctx := context.Background()
	_ = repo.Set(ctx, "is_installed", "true")
	_ = repo.Set(ctx, "root_email", testRootEmail)
	grant := brandingGrantStub{features: []domain.Feature{domain.FeatureRBAC}}
	if licensed {
		grant.features = append(grant.features, domain.FeatureBranding)
	}
	handler.settingService = service.NewSettingService(repo, grant)
	handler.SetBootID("boot-before-restart")
	return handler, repo, shutdowner
}

func postBrandingSettings(t *testing.T, handler *SettingsHandler, body string) (*httptest.ResponseRecorder, *domain.AuditRecord) {
	t.Helper()
	audit := domain.NewAuditRecord("127.0.0.1", "test", "req-1")
	req := httptest.NewRequest(http.MethodPost, "/api/settings.update", bytes.NewBufferString(body))
	req = reqWithUserContext(req.WithContext(domain.WithAuditRecord(req.Context(), audit)), "root-user-id")
	w := httptest.NewRecorder()
	handler.handleUpdate(w, req)
	return w, audit
}

func getSettings(t *testing.T, handler *SettingsHandler) SystemSettingsResponse {
	t.Helper()
	req := reqWithUserContext(httptest.NewRequest(http.MethodGet, "/api/settings.get", nil), "root-user-id")
	w := httptest.NewRecorder()
	handler.handleGet(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp SystemSettingsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return resp
}

func TestSettingsHandler_Get_ReturnsBrandingAndRetention(t *testing.T) {
	handler, repo, _ := brandingSettingsHandler(t, true)
	ctx := context.Background()
	_ = repo.Set(ctx, service.InstanceBrandingSettingKey, `{"name":"Acme","logo_url":"https://cdn.example.com/l.png","icon_url":"","hide_help_menu":true,"background_url":"https://cdn.example.com/bg.jpg"}`)
	_ = repo.Set(ctx, domain.AuditLogsRetentionSettingKey, "90")

	got := getSettings(t, handler).Settings
	assert.Equal(t, "Acme", got.BrandingName)
	assert.Equal(t, "https://cdn.example.com/l.png", got.BrandingLogoURL)
	assert.Equal(t, "", got.BrandingIconURL)
	assert.True(t, got.BrandingHideHelpMenu)
	assert.Equal(t, "https://cdn.example.com/bg.jpg", got.BrandingBackgroundURL)
	assert.Equal(t, 90, got.AuditLogsRetentionDays, "the stored retention is returned, so a form that saves what it shows keeps it")
}

func TestSettingsHandler_Update_SavesBrandingAndAnswersTheRestartIdentity(t *testing.T) {
	handler, repo, shutdowner := brandingSettingsHandler(t, true)

	w, audit := postBrandingSettings(t, handler, `{"branding_name":" Acme ","branding_logo_url":"https://cdn.example.com/l.png","branding_hide_help_menu":true,"branding_background_url":"https://cdn.example.com/bg.jpg"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp SettingsUpdateResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	saved := branding.Branding{Name: "Acme", LogoURL: "https://cdn.example.com/l.png", HideHelpMenu: true, BackgroundURL: "https://cdn.example.com/bg.jpg"}
	assert.Equal(t, "boot-before-restart", resp.BootID)
	assert.Equal(t, saved.Revision(), resp.BrandingRevision)

	stored, err := branding.Decode([]byte(repo.settings[service.InstanceBrandingSettingKey]))
	require.NoError(t, err)
	assert.Equal(t, saved, stored)

	event, _ := audit.Finalize(http.StatusOK)
	for _, field := range []string{"BrandingName", "BrandingLogoURL", "BrandingHideHelpMenu", "BrandingBackgroundURL"} {
		assert.Contains(t, event.Changes, field, "one audit change per branding field")
	}
	assert.NotContains(t, event.Changes, "BrandingIconURL", "an unchanged field is not reported")

	assert.Eventually(t, shutdowner.shutdownCalled.Load, 3*time.Second, 50*time.Millisecond, "a successful save restarts the process")
}

func TestSettingsHandler_Update_AbsentBrandingKeysKeepTheStoredValues(t *testing.T) {
	handler, repo, _ := brandingSettingsHandler(t, false)
	stored := `{"name":"Acme","logo_url":"https://cdn.example.com/l.png","icon_url":"","hide_help_menu":true,"background_url":"https://cdn.example.com/bg.jpg"}`
	_ = repo.Set(context.Background(), service.InstanceBrandingSettingKey, stored)

	// An SMTP-only save on an unlicensed instance: the brand is carried over, unchanged.
	w, _ := postBrandingSettings(t, handler, `{"smtp_host":"smtp.example.com"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	got, err := branding.Decode([]byte(repo.settings[service.InstanceBrandingSettingKey]))
	require.NoError(t, err)
	assert.Equal(t, branding.Branding{Name: "Acme", LogoURL: "https://cdn.example.com/l.png", HideHelpMenu: true, BackgroundURL: "https://cdn.example.com/bg.jpg"}, got)
}

func TestSettingsHandler_Update_EmptyStringClearsABrandingField(t *testing.T) {
	handler, repo, _ := brandingSettingsHandler(t, false)
	_ = repo.Set(context.Background(), service.InstanceBrandingSettingKey, `{"name":"Acme","logo_url":"https://cdn.example.com/l.png","background_url":"https://cdn.example.com/bg.jpg"}`)

	w, _ := postBrandingSettings(t, handler, `{"branding_name":"","branding_logo_url":"","branding_background_url":""}`)
	require.Equal(t, http.StatusOK, w.Code, "clearing needs no licence: %s", w.Body.String())

	got, err := branding.Decode([]byte(repo.settings[service.InstanceBrandingSettingKey]))
	require.NoError(t, err)
	assert.Equal(t, branding.Branding{}, got)
}

func TestSettingsHandler_Update_RefusesBeforeSavingOrRestarting(t *testing.T) {
	cases := []struct {
		name     string
		licensed bool
		body     string
		status   int
	}{
		{"a null name", true, `{"branding_name":null}`, http.StatusBadRequest},
		{"a null help switch", true, `{"branding_hide_help_menu":null}`, http.StatusBadRequest},
		{"a null name in another case", true, `{"Branding_Name":null}`, http.StatusBadRequest},
		{"a number as name", true, `{"branding_name":12}`, http.StatusBadRequest},
		{"a string as help switch", true, `{"branding_hide_help_menu":"yes"}`, http.StatusBadRequest},
		{"a logo without a name", true, `{"branding_logo_url":"https://cdn.example.com/l.png"}`, http.StatusBadRequest},
		{"an http logo", true, `{"branding_name":"Acme","branding_logo_url":"http://cdn.example.com/l.png"}`, http.StatusBadRequest},
		{"a brand without the licence", false, `{"branding_name":"Acme"}`, http.StatusPaymentRequired},
		{"the help switch without the licence", false, `{"branding_hide_help_menu":true}`, http.StatusPaymentRequired},
		{"a null background", true, `{"branding_background_url":null}`, http.StatusBadRequest},
		{"an http background", true, `{"branding_background_url":"http://cdn.example.com/bg.jpg"}`, http.StatusBadRequest},
		{"a background without the licence", false, `{"branding_background_url":"https://cdn.example.com/bg.jpg"}`, http.StatusPaymentRequired},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler, repo, shutdowner := brandingSettingsHandler(t, tc.licensed)
			before := len(repo.settings)

			w, _ := postBrandingSettings(t, handler, tc.body)
			assert.Equal(t, tc.status, w.Code, w.Body.String())
			assert.Equal(t, before, len(repo.settings), "nothing is saved")
			_, written := repo.settings[service.InstanceBrandingSettingKey]
			assert.False(t, written)

			time.Sleep(700 * time.Millisecond) // past the restart delay of a successful save
			assert.False(t, shutdowner.shutdownCalled.Load(), "a refused save does not restart")
		})
	}
}

func TestSettingsHandler_Update_LicenceRefusalCarriesTheFeatureAndPlan(t *testing.T) {
	handler, _, _ := brandingSettingsHandler(t, false)
	w, _ := postBrandingSettings(t, handler, `{"branding_name":"Acme"}`)
	require.Equal(t, http.StatusPaymentRequired, w.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "branding", body["feature"])
	assert.Equal(t, "Agency", body["required_tier"])
}

func TestSetupHandler_Status_ReportsTheServedBrandingRevision(t *testing.T) {
	settingRepo := newMockSettingRepository()
	settingService := service.NewSettingService(settingRepo, nil)
	setupService := service.NewSetupService(settingService, &service.UserService{}, newMockUserRepository(), nil, testSecretKey, nil, &service.EnvironmentConfig{})
	handler := NewSetupHandler(setupService, settingService, nil, newMockAppShutdowner())

	status := func() StatusResponse {
		w := httptest.NewRecorder()
		handler.Status(w, httptest.NewRequest(http.MethodGet, "/api/setup.status", nil))
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var resp StatusResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		return resp
	}

	assert.Equal(t, branding.Branding{}.Revision(), status().BrandingRevision, "unwired, it reports the default identity")

	served := branding.Branding{Name: "Acme"}
	handler.SetBrandingRevision(served.Revision)
	resp := status()
	assert.Equal(t, served.Revision(), resp.BrandingRevision)
	assert.Equal(t, handler.BootID(), resp.BootID, "the accessor names the same process as the status")
}

func TestSettingsHandler_UnreadableBrandDoesNotLockTheSettings(t *testing.T) {
	setup := func(t *testing.T) (*SettingsHandler, *mockSettingRepository) {
		handler, repo, _ := brandingSettingsHandler(t, false)
		ctx := context.Background()
		_ = repo.Set(ctx, service.InstanceBrandingSettingKey, "{not json")
		_ = repo.Set(ctx, "smtp_host", "smtp.example.com")
		return handler, repo
	}

	t.Run("settings.get answers, with the brand reported unreadable", func(t *testing.T) {
		handler, _ := setup(t)
		resp := getSettings(t, handler)
		assert.Equal(t, brandingUnreadableMessage, resp.BrandingError)
		assert.Empty(t, resp.Settings.BrandingName)
		assert.Equal(t, "smtp.example.com", resp.Settings.SMTPHost)
	})

	t.Run("a save without branding keys keeps the row for the operator to repair", func(t *testing.T) {
		handler, repo := setup(t)
		w, _ := postBrandingSettings(t, handler, `{"smtp_host":"smtp2.example.com"}`)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.Equal(t, "{not json", repo.settings[service.InstanceBrandingSettingKey])
		assert.Equal(t, "smtp2.example.com", repo.settings["smtp_host"])
	})

	t.Run("a save that clears the brand replaces the row", func(t *testing.T) {
		handler, repo := setup(t)
		w, _ := postBrandingSettings(t, handler, `{"branding_name":"","branding_logo_url":"","branding_icon_url":"","branding_hide_help_menu":false}`)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		got, err := branding.Decode([]byte(repo.settings[service.InstanceBrandingSettingKey]))
		require.NoError(t, err)
		assert.Equal(t, branding.Branding{}, got)
	})

	t.Run("a readable brand gets no error", func(t *testing.T) {
		handler, _, _ := brandingSettingsHandler(t, false)
		assert.Empty(t, getSettings(t, handler).BrandingError)
	})
}
