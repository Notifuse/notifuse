package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/crypto"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Keys of at least 32 bytes, the minimum the first key of SECRET_KEY must give.
const (
	rotationKeyB = "rotation-key-b-for-integration-tests-only"
	rotationKeyC = "rotation-key-c-for-integration-tests-only"
	rotationKeyD = "rotation-key-d-for-integration-tests-only"
	// A key written before 42.0 that contains a comma. Its first piece alone would
	// pass as a key, so only the stored data shows that it was read wrong.
	rotationLegacyKey = "legacy-key-whose-first-piece-is-long,and-its-second-piece"
)

// encryptedRows is every stored value that holds data encrypted with SECRET_KEY: each
// workspace's settings and integrations, and the encrypted_ settings rows.
type encryptedRows map[string]string

func readEncryptedRows(t *testing.T, suite *testutil.IntegrationTestSuite) encryptedRows {
	t.Helper()
	db := suite.DBManager.GetDB()
	rows := encryptedRows{}

	workspaces, err := db.Query(`SELECT id, settings::text, COALESCE(integrations::text, '') FROM workspaces`)
	require.NoError(t, err)
	defer func() { _ = workspaces.Close() }()
	for workspaces.Next() {
		var id, settings, integrations string
		require.NoError(t, workspaces.Scan(&id, &settings, &integrations))
		rows["workspace "+id+" settings"] = settings
		rows["workspace "+id+" integrations"] = integrations
	}
	require.NoError(t, workspaces.Err())

	settings, err := db.Query(`SELECT key, value FROM settings WHERE key LIKE 'encrypted\_%'`)
	require.NoError(t, err)
	defer func() { _ = settings.Close() }()
	for settings.Next() {
		var key, value string
		require.NoError(t, settings.Scan(&key, &value))
		rows["setting "+key] = value
	}
	require.NoError(t, settings.Err())
	return rows
}

// opensWith reports whether key opens every encrypted value of a stored row.
func opensWith(name, value, key string) error {
	if strings.HasPrefix(name, "setting ") {
		_, _, err := crypto.DecryptWithKeys(value, []string{key})
		return err
	}
	settingsPaths, integrationsPaths := domain.EncryptedFieldPaths()
	paths := settingsPaths
	if strings.HasSuffix(name, " integrations") {
		paths = integrationsPaths
	}
	_, _, err := crypto.ReencryptFields([]byte(value), paths, key, nil)
	return err
}

// startWithSecretKey restarts the server as an installation with SECRET_KEY=raw does.
func startWithSecretKey(t *testing.T, suite *testutil.IntegrationTestSuite, raw string) error {
	t.Helper()
	require.NoError(t, suite.ServerManager.SetSecretKey(raw))
	return suite.ServerManager.Restart()
}

// The installation starts with SECRET_KEY=A (the harness key), then rotates to B by
// listing B,A. Everything Notifuse encrypted with A must open with B afterwards, and
// nothing signed with A may still authenticate.
func TestSecretKeyRotation(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	keyA := testutil.TestSecretKey
	cfg := suite.ServerManager.GetApp().GetConfig()
	client := suite.APIClient

	// Data under A: a workspace with an SMTP integration (username and password are
	// both encrypted), and an encrypted system setting.
	rootToken := performCompleteSignInFlow(t, client, cfg.RootEmail)
	workspaceID := createTestWorkspaceWithToken(t, client, rootToken, "Rotation WS")
	owner := testutil.NewAPIClient(suite.ServerManager.GetURL())
	owner.SetToken(rootToken)
	var created struct {
		IntegrationID string `json:"integration_id"`
	}
	require.NoError(t, owner.PostJSON("/api/workspaces.createIntegration", domain.CreateIntegrationRequest{
		WorkspaceID: workspaceID,
		Name:        "SMTP",
		Type:        domain.IntegrationTypeEmail,
		Provider: domain.EmailProvider{
			Kind:               domain.EmailProviderKindSMTP,
			RateLimitPerMinute: 25,
			Senders:            []domain.EmailSender{domain.NewEmailSender("sender@example.com", "Sender")},
			SMTP:               &domain.SMTPSettings{Host: "localhost", Port: 1025, Username: "smtp-user", Password: "smtp-pass"},
		},
	}, &created))
	require.NotEmpty(t, created.IntegrationID)

	// What the console holds for the integration: the encrypted forms, which it echoes
	// back on save.
	var loaded struct {
		Workspace domain.Workspace `json:"workspace"`
	}
	require.NoError(t, owner.GetJSON("/api/workspaces.get", &loaded, map[string]string{"id": workspaceID}))
	var echoed *domain.Integration
	for i := range loaded.Workspace.Integrations {
		if loaded.Workspace.Integrations[i].ID == created.IntegrationID {
			echoed = &loaded.Workspace.Integrations[i]
		}
	}
	require.NotNil(t, echoed)
	require.NotNil(t, echoed.EmailProvider.SMTP)
	require.NotEmpty(t, echoed.EmailProvider.SMTP.EncryptedPassword)

	smtpPassword, err := crypto.EncryptString("system-smtp-pass", keyA)
	require.NoError(t, err)
	_, err = suite.DBManager.GetDB().Exec(`INSERT INTO settings (key, value, created_at, updated_at)
		VALUES ('encrypted_smtp_password', $1, NOW(), NOW())
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`, smtpPassword)
	require.NoError(t, err)

	underA := readEncryptedRows(t, suite)
	for name, value := range underA {
		require.NoError(t, opensWith(name, value, keyA), "%s is under A before the rotation", name)
	}

	t.Run("listing B,A re-encrypts every stored value under B", func(t *testing.T) {
		require.NoError(t, startWithSecretKey(t, suite, rotationKeyB+","+keyA))

		rotated := readEncryptedRows(t, suite)
		require.Equal(t, len(underA), len(rotated))
		for name, value := range rotated {
			assert.NoError(t, opensWith(name, value, rotationKeyB), "%s opens with B", name)
		}
		for _, name := range []string{
			"workspace " + workspaceID + " settings",
			"workspace " + workspaceID + " integrations",
			"setting encrypted_smtp_password",
		} {
			assert.True(t, errors.Is(opensWith(name, rotated[name], keyA), crypto.ErrUndecryptable),
				"%s no longer opens with A", name)
		}

		workspace, err := suite.ServerManager.GetApp().GetWorkspaceRepository().GetByID(context.Background(), workspaceID)
		require.NoError(t, err)
		integration := workspace.GetIntegrationByID(created.IntegrationID)
		require.NotNil(t, integration)
		assert.Equal(t, "smtp-user", integration.EmailProvider.SMTP.Username)
		assert.Equal(t, "smtp-pass", integration.EmailProvider.SMTP.Password)
	})

	t.Run("starting again with B,A changes nothing", func(t *testing.T) {
		before := readEncryptedRows(t, suite)

		require.NoError(t, startWithSecretKey(t, suite, rotationKeyB+","+keyA))

		assert.Equal(t, before, readEncryptedRows(t, suite))
	})

	t.Run("keys that open nothing stop the start and change nothing", func(t *testing.T) {
		before := readEncryptedRows(t, suite)

		err := startWithSecretKey(t, suite, rotationKeyC+","+rotationKeyD)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "no key listed in SECRET_KEY opens")
		assert.Equal(t, before, readEncryptedRows(t, suite))
	})

	t.Run("once rotated, B alone runs the installation and A signs nothing", func(t *testing.T) {
		require.NoError(t, startWithSecretKey(t, suite, rotationKeyB))
		client := testutil.NewAPIClient(suite.ServerManager.GetURL())

		rootSignin := func(t *testing.T, key string) int {
			t.Helper()
			timestamp := time.Now().Unix()
			resp, err := client.Post("/api/user.rootSignin", domain.RootSigninInput{
				Email:     cfg.RootEmail,
				Timestamp: timestamp,
				Signature: crypto.ComputeHMAC256([]byte(fmt.Sprintf("%s:%d", cfg.RootEmail, timestamp)), key),
			})
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()
			return resp.StatusCode
		}
		assert.Equal(t, http.StatusUnauthorized, rootSignin(t, keyA))
		assert.Equal(t, http.StatusOK, rootSignin(t, rotationKeyB))

		signedWithA := testutil.NewAPIClient(suite.ServerManager.GetURL())
		signedWithA.SetToken(rootToken)
		resp, err := signedWithA.Get("/api/user.me")
		require.NoError(t, err)
		_ = resp.Body.Close()
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "a session signed with A is refused")

		newOwner := testutil.NewAPIClient(suite.ServerManager.GetURL())
		newOwner.SetToken(performCompleteSignInFlow(t, client, cfg.RootEmail))
		resp, err = newOwner.Get("/api/user.me")
		require.NoError(t, err)
		_ = resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode, "a session signed with B is accepted")

		// A page loaded before the rotation saves the integration with what it loaded.
		before := readEncryptedRows(t, suite)
		resp, err = newOwner.Post("/api/workspaces.updateIntegration", domain.UpdateIntegrationRequest{
			WorkspaceID:   workspaceID,
			IntegrationID: created.IntegrationID,
			Name:          echoed.Name,
			Provider:      echoed.EmailProvider,
		})
		require.NoError(t, err)
		var refused map[string]string
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&refused))
		_ = resp.Body.Close()
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.Contains(t, refused["error"], "Reload the page")
		assert.Equal(t, before, readEncryptedRows(t, suite), "the refused save wrote nothing")
	})

	t.Run("a key containing a comma is read whole only with its comma written twice", func(t *testing.T) {
		escaped := strings.ReplaceAll(rotationLegacyKey, ",", ",,")
		require.NoError(t, startWithSecretKey(t, suite, escaped+","+rotationKeyB))
		before := readEncryptedRows(t, suite)
		for name, value := range before {
			require.NoError(t, opensWith(name, value, rotationLegacyKey), "%s is under the legacy key", name)
		}

		err := startWithSecretKey(t, suite, rotationLegacyKey)
		require.Error(t, err, "written once, the key reads as two keys that open nothing")
		assert.Contains(t, err.Error(), "twice")
		assert.Equal(t, before, readEncryptedRows(t, suite))

		require.NoError(t, startWithSecretKey(t, suite, escaped))
		assert.Equal(t, before, readEncryptedRows(t, suite))
	})
}
