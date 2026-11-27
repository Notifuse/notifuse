package integration

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/branding"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// licensedWithout runs the server under a valid key that carries every feature but the ones
// named, so a test can meet one gate on an otherwise licensed deployment.
func licensedWithout(t *testing.T, missing ...domain.Feature) func(*config.Config) testutil.AppInterface {
	t.Helper()
	var features []domain.Feature
	for _, f := range []domain.Feature{
		domain.FeatureRBAC, domain.FeatureSESTenant, domain.FeatureSSO,
		domain.FeatureAuditLogs, domain.FeatureTemplateI18n, domain.FeatureDataRetention, domain.FeatureBranding,
	} {
		keep := true
		for _, m := range missing {
			if f == m {
				keep = false
			}
		}
		if keep {
			features = append(features, f)
		}
	}
	key, err := testutil.MintDevLicense(features...)
	require.NoError(t, err)
	return func(cfg *config.Config) testutil.AppInterface {
		cfg.LicenseKey = key
		return app.NewApp(cfg)
	}
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return string(data)
}

func servedBranding(t *testing.T, client *testutil.APIClient) branding.Resolved {
	t.Helper()
	resp, err := client.Get("/config.js")
	require.NoError(t, err)
	body := readBody(t, resp)
	const prefix = "window.INSTANCE_BRANDING = "
	start := strings.Index(body, prefix)
	require.NotEqual(t, -1, start, "config.js carries no INSTANCE_BRANDING")
	rest := body[start+len(prefix):]
	var got branding.Resolved
	require.NoError(t, json.Unmarshal([]byte(rest[:strings.Index(rest, ";")]), &got))
	return got
}

func setupStatus(t *testing.T, client *testutil.APIClient) (bootID, revision string) {
	t.Helper()
	var status struct {
		BootID           string `json:"boot_id"`
		BrandingRevision string `json:"branding_revision"`
	}
	require.NoError(t, client.GetJSON("/api/setup.status", &status))
	return status.BootID, status.BrandingRevision
}

// restartAfterSave waits for the process that answered settings.update to finish shutting down,
// stops its server, and starts a new App on the same database, the way a deployment restarts
// after a save. It returns a client for the new server, carrying token.
func restartAfterSave(t *testing.T, suite *testutil.IntegrationTestSuite, factory func(*config.Config) testutil.AppInterface, token string) *testutil.APIClient {
	t.Helper()
	old, ok := suite.ServerManager.GetApp().(interface{ ShutdownDone() <-chan struct{} })
	require.True(t, ok, "the app exposes ShutdownDone")
	select {
	case <-old.ShutdownDone():
	case <-time.After(90 * time.Second):
		t.Fatal("the saving process did not finish shutting down")
	}
	require.NoError(t, suite.ServerManager.Stop())

	next := testutil.NewServerManager(factory, suite.DBManager)
	require.NoError(t, next.Start())
	suite.ServerManager = next // Cleanup stops the server that is running
	client := testutil.NewAPIClient(next.GetURL())
	client.SetToken(token)
	return client
}

func TestSettingsHandler_InstanceBranding(t *testing.T) {
	// Before anything else: the subtests mint a dev licence, which a build without the licdev
	// tag refuses, so a plain `go test ./...` must skip here rather than fail.
	skipIfNotIntegrationTest(t)
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	brand := map[string]interface{}{
		"branding_name":           "Acme",
		"branding_logo_url":       "https://cdn.example.com/logo.png",
		"branding_hide_help_menu": true,
		"branding_background_url": "https://cdn.example.com/bg.jpg",
	}

	t.Run("only root changes it", func(t *testing.T) {
		suite := testutil.NewIntegrationTestSuite(t, appFactory)
		defer func() { suite.Cleanup() }()

		resp, err := suite.APIClient.Post("/api/settings.update", brand)
		require.NoError(t, err)
		_ = readBody(t, resp)
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "no session")

		tokens := testutil.NewTokenCache(suite.APIClient)
		suite.APIClient.SetToken(tokens.GetOrCreate(t, "testuser@example.com"))
		resp, err = suite.APIClient.Post("/api/settings.update", brand)
		require.NoError(t, err)
		_ = readBody(t, resp)
		assert.Equal(t, http.StatusForbidden, resp.StatusCode, "a workspace owner who is not root")

		rootToken := tokens.GetOrCreate(t, suite.Config.RootEmail)
		suite.APIClient.SetToken(rootToken)
		workspaceID := createTestWorkspaceWithToken(t, suite.APIClient, rootToken, "Branding key")
		var created map[string]interface{}
		require.NoError(t, suite.APIClient.PostJSON("/api/workspaces.createAPIKey", domain.CreateAPIKeyRequest{WorkspaceID: workspaceID, EmailPrefix: "brand"}, &created))
		suite.APIClient.SetToken(created["token"].(string))
		resp, err = suite.APIClient.Post("/api/settings.update", brand)
		require.NoError(t, err)
		_ = readBody(t, resp)
		assert.Equal(t, http.StatusForbidden, resp.StatusCode, "an API key, even one of root's workspace")
	})

	t.Run("a key without branding refuses it with 402 and saves nothing", func(t *testing.T) {
		suite := testutil.NewIntegrationTestSuite(t, licensedWithout(t, domain.FeatureBranding))
		defer func() { suite.Cleanup() }()
		suite.APIClient.SetToken(testutil.NewTokenCache(suite.APIClient).GetOrCreate(t, suite.Config.RootEmail))

		resp, err := suite.APIClient.Post("/api/settings.update", brand)
		require.NoError(t, err)
		body := readBody(t, resp)
		require.Equal(t, http.StatusPaymentRequired, resp.StatusCode, body)
		assert.Contains(t, body, `"feature":"branding"`)
		assert.Contains(t, body, `"required_tier":"Agency"`)

		var settings SettingsResponseEnvelope
		require.NoError(t, suite.APIClient.GetJSON("/api/settings.get", &settings))
		assert.Empty(t, settings.Settings.BrandingName, "nothing was saved")
		assert.False(t, servedBranding(t, suite.APIClient).Custom)
	})

	t.Run("a licensed save survives the restart, and a reset brings Notifuse back", func(t *testing.T) {
		suite := testutil.NewIntegrationTestSuite(t, appFactory)
		defer func() { suite.Cleanup() }()
		rootToken := testutil.NewTokenCache(suite.APIClient).GetOrCreate(t, suite.Config.RootEmail)
		suite.APIClient.SetToken(rootToken)

		var before SettingsResponseEnvelope
		require.NoError(t, suite.APIClient.GetJSON("/api/settings.get", &before))
		retention := before.Settings.AuditLogsRetentionDays
		firstBoot, _ := setupStatus(t, suite.APIClient)

		var saved struct {
			BootID           string `json:"boot_id"`
			BrandingRevision string `json:"branding_revision"`
		}
		require.NoError(t, suite.APIClient.PostJSON("/api/settings.update", brand, &saved))
		assert.Equal(t, firstBoot, saved.BootID, "the response names the process that saved")
		want := branding.Branding{Name: "Acme", LogoURL: "https://cdn.example.com/logo.png", HideHelpMenu: true, BackgroundURL: "https://cdn.example.com/bg.jpg"}
		assert.Equal(t, want.Revision(), saved.BrandingRevision)

		client := restartAfterSave(t, suite, appFactory, rootToken)
		boot, revision := setupStatus(t, client)
		assert.NotEqual(t, firstBoot, boot, "a new process")
		assert.Equal(t, saved.BrandingRevision, revision, "serving the saved brand")

		served := servedBranding(t, client)
		assert.True(t, served.Custom)
		assert.Equal(t, "Acme", served.Name)
		assert.Equal(t, "https://cdn.example.com/logo.png", served.LogoURL)
		assert.True(t, served.HideHelpMenu)
		assert.Equal(t, "https://cdn.example.com/bg.jpg", served.BackgroundURL)

		var after SettingsResponseEnvelope
		require.NoError(t, client.GetJSON("/api/settings.get", &after))
		assert.Equal(t, "Acme", after.Settings.BrandingName)
		assert.Equal(t, "https://cdn.example.com/bg.jpg", after.Settings.BrandingBackgroundURL)
		assert.Equal(t, retention, after.Settings.AuditLogsRetentionDays, "a branding save leaves the retention alone")

		reset := map[string]interface{}{"branding_name": "", "branding_logo_url": "", "branding_icon_url": "", "branding_hide_help_menu": false, "branding_background_url": ""}
		require.NoError(t, client.PostJSON("/api/settings.update", reset, nil))
		suite.APIClient = client
		client = restartAfterSave(t, suite, appFactory, rootToken)
		served = servedBranding(t, client)
		assert.False(t, served.Custom)
		assert.Equal(t, branding.DefaultName, served.Name)
		assert.False(t, served.HideHelpMenu)
		assert.Empty(t, served.BackgroundURL, "the Notifuse photo is back")
	})

	t.Run("a background alone needs no name, and only the licence shows it", func(t *testing.T) {
		suite := testutil.NewIntegrationTestSuite(t, appFactory)
		defer func() { suite.Cleanup() }()
		rootToken := testutil.NewTokenCache(suite.APIClient).GetOrCreate(t, suite.Config.RootEmail)
		suite.APIClient.SetToken(rootToken)

		require.NoError(t, suite.APIClient.PostJSON("/api/settings.update", map[string]interface{}{"branding_background_url": "https://cdn.example.com/bg.jpg"}, nil))
		client := restartAfterSave(t, suite, appFactory, rootToken)
		served := servedBranding(t, client)
		assert.False(t, served.Custom, "a background alone keeps the Notifuse identity")
		assert.Equal(t, branding.DefaultName, served.Name)
		assert.Equal(t, "https://cdn.example.com/bg.jpg", served.BackgroundURL)

		// The same stored row on a process whose key lacks branding: the Notifuse photo shows. The
		// unchanged save only restarts the process, which then starts with that key.
		require.NoError(t, client.PostJSON("/api/settings.update", map[string]interface{}{"branding_background_url": "https://cdn.example.com/bg.jpg"}, nil))
		suite.APIClient = client
		unlicensed := restartAfterSave(t, suite, licensedWithout(t, domain.FeatureBranding), rootToken)
		assert.Empty(t, servedBranding(t, unlicensed).BackgroundURL)
		var stored SettingsResponseEnvelope
		require.NoError(t, unlicensed.GetJSON("/api/settings.get", &stored))
		assert.Equal(t, "https://cdn.example.com/bg.jpg", stored.Settings.BrandingBackgroundURL, "the saved value is kept")
	})

	t.Run("a key without branding refuses a background with 402", func(t *testing.T) {
		suite := testutil.NewIntegrationTestSuite(t, licensedWithout(t, domain.FeatureBranding))
		defer func() { suite.Cleanup() }()
		suite.APIClient.SetToken(testutil.NewTokenCache(suite.APIClient).GetOrCreate(t, suite.Config.RootEmail))

		resp, err := suite.APIClient.Post("/api/settings.update", map[string]interface{}{"branding_background_url": "https://cdn.example.com/bg.jpg"})
		require.NoError(t, err)
		body := readBody(t, resp)
		require.Equal(t, http.StatusPaymentRequired, resp.StatusCode, body)
		assert.Empty(t, servedBranding(t, suite.APIClient).BackgroundURL)
	})
}

// SettingsResponseEnvelope is the body of /api/settings.get, read for the fields these tests
// check.
type SettingsResponseEnvelope struct {
	Settings struct {
		BrandingName           string `json:"branding_name"`
		BrandingBackgroundURL  string `json:"branding_background_url"`
		AuditLogsRetentionDays int    `json:"audit_logs_retention_days"`
	} `json:"settings"`
}
