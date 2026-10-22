package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/repository"
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

// opensWith reports whether key opens the setting called name, or, for a workspace row,
// every value the server decrypts when it loads that workspace.
func opensWith(rows encryptedRows, name, key string) error {
	if strings.HasPrefix(name, "setting ") {
		_, _, err := crypto.DecryptWithKeys(rows[name], []string{key})
		return err
	}
	workspace := strings.TrimSuffix(strings.TrimSuffix(name, " settings"), " integrations")
	return domain.CheckStoredWorkspace([]byte(rows[workspace+" settings"]), []byte(rows[workspace+" integrations"]), key)
}

// reencryptStored puts every stored value that one of from opens under key, as an
// installation running with key would have written it, without going through the server.
func reencryptStored(t *testing.T, suite *testutil.IntegrationTestSuite, key string, from []string) {
	t.Helper()
	db := suite.DBManager.GetDB()
	settingsPaths, integrationsPaths := domain.EncryptedFieldPaths()

	type document struct {
		id                     string
		settings, integrations []byte
	}
	var workspaces []document
	rows, err := db.Query(`SELECT id, settings, integrations FROM workspaces`)
	require.NoError(t, err)
	for rows.Next() {
		var w document
		require.NoError(t, rows.Scan(&w.id, &w.settings, &w.integrations))
		workspaces = append(workspaces, w)
	}
	require.NoError(t, rows.Err())
	_ = rows.Close()
	for _, w := range workspaces {
		settings, _, err := crypto.ReencryptFields(w.settings, settingsPaths, key, from)
		require.NoError(t, err)
		integrations, _, err := crypto.ReencryptFields(w.integrations, integrationsPaths, key, from)
		require.NoError(t, err)
		var integrationsArg any
		if integrations != nil {
			integrationsArg = integrations
		}
		_, err = db.Exec(`UPDATE workspaces SET settings = $1, integrations = $2 WHERE id = $3`, settings, integrationsArg, w.id)
		require.NoError(t, err)
	}

	for name, value := range readEncryptedRows(t, suite) {
		if !strings.HasPrefix(name, "setting ") {
			continue
		}
		reencrypted, _, err := crypto.ReencryptValue(value, key, from)
		require.NoError(t, err)
		_, err = db.Exec(`UPDATE settings SET value = $1 WHERE key = $2`, reencrypted, strings.TrimPrefix(name, "setting "))
		require.NoError(t, err)
	}
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

	// SES settings the SMTP integration does not use, holding a value no key opens, as an
	// API client could store before 42.0: nothing reads them, so they stop nothing.
	_, err = suite.DBManager.GetDB().Exec(`UPDATE workspaces
		SET integrations = jsonb_set(integrations, '{0,email_provider,ses}',
			'{"region": "eu-west-1", "encrypted_secret_key": "not-a-ciphertext"}')
		WHERE id = $1`, workspaceID)
	require.NoError(t, err)

	// A workspace written before integrations existed holds NULL in that column.
	legacyID := createTestWorkspaceWithToken(t, client, rootToken, "Rotation legacy WS")
	_, err = suite.DBManager.GetDB().Exec(`UPDATE workspaces SET integrations = NULL WHERE id = $1`, legacyID)
	require.NoError(t, err)

	underA := readEncryptedRows(t, suite)
	for name := range underA {
		require.NoError(t, opensWith(underA, name, keyA), "%s is under A before the rotation", name)
	}

	t.Run("listing B,A re-encrypts every stored value under B", func(t *testing.T) {
		require.NoError(t, startWithSecretKey(t, suite, rotationKeyB+","+keyA))

		rotated := readEncryptedRows(t, suite)
		require.Equal(t, len(underA), len(rotated))
		for name := range rotated {
			assert.NoError(t, opensWith(rotated, name, rotationKeyB), "%s opens with B", name)
		}
		for _, name := range []string{
			"workspace " + workspaceID + " settings",
			"setting encrypted_smtp_password",
		} {
			assert.True(t, errors.Is(opensWith(rotated, name, keyA), crypto.ErrUndecryptable),
				"%s no longer opens with A", name)
		}
		assert.Contains(t, rotated["workspace "+workspaceID+" integrations"], `"encrypted_secret_key": "not-a-ciphertext"`,
			"a value nothing reads is left as it is")
		var legacyIntegrations sql.NullString
		require.NoError(t, suite.DBManager.GetDB().QueryRow(
			`SELECT integrations::text FROM workspaces WHERE id = $1`, legacyID).Scan(&legacyIntegrations))
		assert.False(t, legacyIntegrations.Valid, "integrations that were NULL stay NULL")

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
		// A single key is used as written: in quotes, as docker --env-file keeps them,
		// B is another key, which opens no workspace.
		err := startWithSecretKey(t, suite, `"`+rotationKeyB+`"`)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "remove them")

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
		for name := range before {
			require.NoError(t, opensWith(before, name, rotationLegacyKey), "%s is under the legacy key", name)
		}

		err := startWithSecretKey(t, suite, rotationLegacyKey)
		require.Error(t, err, "written once, the key reads as two keys that open nothing")
		assert.Contains(t, err.Error(), "twice")
		assert.Equal(t, before, readEncryptedRows(t, suite))

		require.NoError(t, startWithSecretKey(t, suite, escaped))
		assert.Equal(t, before, readEncryptedRows(t, suite))
	})

	// docker --env-file kept the quotes of SECRET_KEY="C" before 42.0, so the data is under
	// the key in quotes, while compose passes C without them.
	t.Run("data under the key in quotes is brought under the key as written by listing it twice", func(t *testing.T) {
		quoted := `"` + rotationKeyC + `"`
		reencryptStored(t, suite, quoted, []string{rotationLegacyKey})
		before := readEncryptedRows(t, suite)
		for name := range before {
			require.NoError(t, opensWith(before, name, quoted), "%s is under C in quotes", name)
		}

		err := startWithSecretKey(t, suite, rotationKeyC)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "in quotes")
		assert.Equal(t, before, readEncryptedRows(t, suite))

		require.NoError(t, startWithSecretKey(t, suite, rotationKeyC+","+rotationKeyC))
		rotated := readEncryptedRows(t, suite)
		for name := range rotated {
			assert.NoError(t, opensWith(rotated, name, rotationKeyC), "%s opens with C as written", name)
		}
		require.NoError(t, startWithSecretKey(t, suite, rotationKeyC))
	})
}

// An installation with no workspace yet, upgraded with a key from before 42.0 that holds
// a single comma, or given another key: only its encrypted settings show that the key
// is wrong.
func TestSecretKeyRotationWithoutWorkspace(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	db := suite.DBManager.GetDB()
	_, err := db.Exec(`DELETE FROM workspaces`)
	require.NoError(t, err)
	smtpPassword, err := crypto.EncryptString("system-smtp-pass", rotationLegacyKey)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO settings (key, value, created_at, updated_at)
		VALUES ('encrypted_smtp_password', $1, NOW(), NOW())
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`, smtpPassword)
	require.NoError(t, err)
	before := readEncryptedRows(t, suite)

	err = startWithSecretKey(t, suite, rotationLegacyKey)
	require.Error(t, err, "written once, the key reads as two keys that open no stored setting")
	assert.Contains(t, err.Error(), "encrypted_smtp_password")
	assert.Contains(t, err.Error(), "twice")
	assert.Equal(t, before, readEncryptedRows(t, suite))

	// The same holds for another single key: the settings would be dropped.
	err = startWithSecretKey(t, suite, rotationKeyB)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "opens none of the system settings stored in the database (encrypted_smtp_password)")
	assert.Equal(t, before, readEncryptedRows(t, suite))

	require.NoError(t, startWithSecretKey(t, suite, strings.ReplaceAll(rotationLegacyKey, ",", ",,")))
	assert.Equal(t, before, readEncryptedRows(t, suite))
}

// A database whose default isolation is stricter than READ COMMITTED: an instance that
// waited on the rotation lock while another changed the rows must go on, not fail on
// them as a serialization error.
func TestSecretKeyRotationUnderStricterIsolation(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	dbConfig := suite.DBManager.GetConfig()
	strict, err := sql.Open("postgres", fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s default_transaction_isolation='repeatable read'",
		dbConfig.Host, dbConfig.Port, dbConfig.User, dbConfig.Password, dbConfig.DBName, dbConfig.SSLMode))
	require.NoError(t, err)
	defer func() { _ = strict.Close() }()
	var isolation string
	require.NoError(t, strict.QueryRow(`SHOW default_transaction_isolation`).Scan(&isolation))
	require.Equal(t, "repeatable read", isolation)

	// Another instance holds the lock RotateSecretKey takes, and changes the rows.
	ctx := context.Background()
	holder, err := suite.DBManager.GetDB().BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = holder.Rollback() }()
	_, err = holder.Exec(`SELECT pg_advisory_xact_lock(4242000001)`)
	require.NoError(t, err)

	done := make(chan error, 1)
	go func() {
		_, err := repository.RotateSecretKey(ctx, strict, testutil.TestSecretKey, nil)
		done <- err
	}()
	require.Eventually(t, func() bool {
		var waiting int
		_ = suite.DBManager.GetDB().QueryRow(
			`SELECT count(*) FROM pg_locks WHERE locktype = 'advisory' AND NOT granted`).Scan(&waiting)
		return waiting > 0
	}, 10*time.Second, 20*time.Millisecond, "the rotation waits on the lock")

	_, err = holder.Exec(`UPDATE workspaces SET settings = settings`)
	require.NoError(t, err)
	_, err = holder.Exec(`UPDATE settings SET value = value WHERE key LIKE 'encrypted\_%'`)
	require.NoError(t, err)
	require.NoError(t, holder.Commit())

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(30 * time.Second):
		t.Fatal("the rotation did not finish")
	}
}
