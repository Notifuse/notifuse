//go:build integration
// +build integration

package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// TestDemoSeedOnAProductionInstance drives /api/demo.seed the way the preview's
// scheduler does: an instance running as production, its background workers on,
// with a workspace of its own that the seed must leave alone.
//
// What only a real stack shows: the route is served on production at all, the
// seed's workspace database is dropped and created again under the same name
// while workers run (the second seed), and a "demo" workspace the seed did not
// create is refused with its data intact.
func TestDemoSeedOnAProductionInstance(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		cfg.Environment = "production"
		return appFactory(cfg)
	})
	defer func() { suite.Cleanup() }()

	// The instance's own workspace, with a contact. Created before the
	// restart below, which leaves the data factory's repositories behind.
	own, err := suite.DataFactory.CreateWorkspace()
	require.NoError(t, err)
	_, err = suite.DataFactory.CreateContact(own.ID, func(c *domain.Contact) { c.Email = "keep-me@example.com" })
	require.NoError(t, err)
	ownDB, err := suite.DBManager.GetWorkspaceDB(own.ID)
	require.NoError(t, err)

	// The application's server has no write timeout; a seed runs for tens of seconds.
	suite.ServerManager.SetWriteTimeout(10 * time.Minute)
	require.NoError(t, suite.ServerManager.Restart())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, suite.ServerManager.StartBackgroundWorkers(ctx))

	systemDB := suite.DBManager.GetDB()
	hmac := domain.ComputeEmailHMAC("test@example.com", testutil.TestSecretKey)

	seed := func(t *testing.T, signature string) (int, map[string]string) {
		t.Helper()
		client := &http.Client{Timeout: 10 * time.Minute}
		resp, err := client.Get(suite.ServerManager.GetURL() + "/api/demo.seed?hmac=" + signature)
		require.NoError(t, err)
		defer resp.Body.Close()
		var body map[string]string
		_ = json.NewDecoder(resp.Body).Decode(&body)
		return resp.StatusCode, body
	}
	count := func(t *testing.T, db *sql.DB, query string, args ...interface{}) int {
		t.Helper()
		var n int
		require.NoError(t, db.QueryRowContext(ctx, query, args...).Scan(&n))
		return n
	}

	t.Run("an unsigned request is refused", func(t *testing.T) {
		code, _ := seed(t, "not-the-hmac")
		assert.Equal(t, http.StatusUnauthorized, code)
	})

	// Unknown paths fall through to the root handler, so the status code alone says
	// nothing: what matters is that a correctly signed reset deletes nothing.
	t.Run("demo.reset is not served on production", func(t *testing.T) {
		resp, err := http.Get(suite.ServerManager.GetURL() + "/api/demo.reset?hmac=" + hmac)
		require.NoError(t, err)
		var body map[string]string
		_ = json.NewDecoder(resp.Body).Decode(&body)
		resp.Body.Close()
		assert.NotEqual(t, "reset", body["status"])
		assert.Equal(t, 1, count(t, systemDB, `SELECT COUNT(*) FROM workspaces WHERE id = $1`, own.ID),
			"a signed reset on production must not delete the instance's workspaces")
	})

	assertSeeded := func(t *testing.T) {
		t.Helper()
		demoDB, err := suite.DBManager.GetWorkspaceDB("demo")
		require.NoError(t, err)
		assert.Positive(t, count(t, demoDB, `SELECT COUNT(*) FROM contacts`), "contacts")
		assert.Positive(t, count(t, demoDB, `SELECT COUNT(*) FROM templates`), "templates")
		assert.Positive(t, count(t, demoDB, `SELECT COUNT(*) FROM broadcasts`), "broadcasts")
		assert.Positive(t, count(t, demoDB, `SELECT COUNT(*) FROM automations`), "automations")
		assert.Zero(t, count(t, demoDB, `SELECT COUNT(*) FROM automations WHERE status = 'live'`),
			"off ENVIRONMENT=demo the seeded automations must stay inert: this instance's scheduler runs")
		var marker string
		require.NoError(t, systemDB.QueryRowContext(ctx,
			`SELECT value FROM settings WHERE key = 'demo_fixtures_workspace_created_at'`).Scan(&marker),
			"the seed marks the workspace it created")
		var createdAt time.Time
		require.NoError(t, systemDB.QueryRowContext(ctx,
			`SELECT created_at FROM workspaces WHERE id = 'demo'`).Scan(&createdAt))
		markedAt, err := time.Parse(time.RFC3339Nano, marker)
		require.NoError(t, err)
		assert.True(t, markedAt.Equal(createdAt), "the marker %s is the workspace's created_at %s", marker, createdAt)

		// The instance's own workspace is untouched.
		assert.Equal(t, 1, count(t, systemDB, `SELECT COUNT(*) FROM workspaces WHERE id = $1`, own.ID))
		assert.Equal(t, 1, count(t, ownDB, `SELECT COUNT(*) FROM contacts WHERE email = 'keep-me@example.com'`))
	}

	t.Run("a signed seed creates the fixtures workspace and nothing else changes", func(t *testing.T) {
		code, body := seed(t, hmac)
		require.Equal(t, http.StatusOK, code, "body: %v", body)
		assert.Equal(t, "seeded", body["status"])
		assertSeeded(t)
	})

	t.Run("a second seed replaces the fixtures workspace", func(t *testing.T) {
		demoDB, err := suite.DBManager.GetWorkspaceDB("demo")
		require.NoError(t, err)
		_, err = demoDB.ExecContext(ctx,
			`INSERT INTO contacts (email, created_at, updated_at) VALUES ('stale@example.com', NOW(), NOW())`)
		require.NoError(t, err)

		// A fresh handler: the five-minute debounce lives in the handler.
		require.NoError(t, suite.ServerManager.Restart())
		require.NoError(t, suite.ServerManager.StartBackgroundWorkers(ctx))

		code, body := seed(t, hmac)
		require.Equal(t, http.StatusOK, code, "body: %v", body)
		assertSeeded(t)

		demoDB, err = suite.DBManager.GetWorkspaceDB("demo")
		require.NoError(t, err)
		assert.Zero(t, count(t, demoDB, `SELECT COUNT(*) FROM contacts WHERE email = 'stale@example.com'`),
			"the previous fixtures were dropped, not added to")
	})

	t.Run("a demo workspace the seed did not create is refused", func(t *testing.T) {
		_, err := systemDB.ExecContext(ctx, `DELETE FROM settings WHERE key = 'demo_fixtures_workspace_created_at'`)
		require.NoError(t, err)
		require.NoError(t, suite.ServerManager.Restart())

		code, body := seed(t, hmac)
		assert.Equal(t, http.StatusConflict, code, "body: %v", body)
		assert.Contains(t, body["error"], "nothing was deleted")

		demoDB, err := suite.DBManager.GetWorkspaceDB("demo")
		require.NoError(t, err)
		assert.Positive(t, count(t, demoDB, `SELECT COUNT(*) FROM contacts`), "its data is intact")
	})
}
