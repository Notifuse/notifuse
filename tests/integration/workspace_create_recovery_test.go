package integration

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	pgdb "github.com/Notifuse/notifuse/pkg/database"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// Workspace creation through the API around leftover databases, and deletion racing a
// request, against the real server and PostgreSQL.
func TestWorkspaceCreateRecovery(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer suite.Cleanup()

	client := suite.APIClient
	client.SetToken(testutil.NewTokenCache(client).GetOrCreate(t, "test@example.com"))
	cfg := suite.DBManager.GetConfig()
	admin := openAdmin(t, cfg)
	systemDB := suite.DBManager.GetDB()

	createBody := func(id string) map[string]any {
		return map[string]any{
			"id":   id,
			"name": "Recovery " + id,
			"settings": map[string]any{
				"timezone":         "UTC",
				"default_language": "en",
				"languages":        []string{"en"},
			},
		}
	}
	create := func(t *testing.T, id string) (int, map[string]any) {
		t.Helper()
		resp, err := client.Post("/api/workspaces.create", createBody(id))
		require.NoError(t, err)
		defer resp.Body.Close()
		var body map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&body)
		return resp.StatusCode, body
	}
	rowCount := func(t *testing.T, id string) int {
		t.Helper()
		var n int
		require.NoError(t, systemDB.QueryRow(`SELECT count(*) FROM workspaces WHERE id = $1`, id).Scan(&n))
		return n
	}

	t.Run("an empty leftover database is adopted and initialised", func(t *testing.T) {
		id := uniqueID("emptyleft")
		dbName := cfg.Prefix + "_ws_" + id
		createEmptyDatabase(t, admin, dbName)

		status, body := create(t, id)
		require.Equal(t, http.StatusCreated, status, "%v", body)

		holds, err := intdbHoldsTables(t, cfg, dbName)
		require.NoError(t, err)
		assert.True(t, holds)

		resp, err := client.Post("/api/contacts.upsert", map[string]any{
			"workspace_id": id,
			"contact":      map[string]any{"email": "adopted@example.com"},
		})
		require.NoError(t, err)
		resp.Body.Close()
		assert.Contains(t, []int{http.StatusOK, http.StatusCreated}, resp.StatusCode)
	})

	t.Run("a leftover database holding tables is refused with a message naming it", func(t *testing.T) {
		id := uniqueID("fullleft")
		dbName := cfg.Prefix + "_ws_" + id
		createEmptyDatabase(t, admin, dbName)
		_, err := openDatabase(t, cfg, dbName).Exec(`CREATE TABLE keep_me (x int)`)
		require.NoError(t, err)

		status, body := create(t, id)
		assert.Equal(t, http.StatusConflict, status)
		want := (&domain.ErrWorkspaceDatabaseNotEmpty{WorkspaceID: id, DatabaseName: dbName}).Error()
		assert.Equal(t, want, body["error"])
		assert.Equal(t, 0, rowCount(t, id))

		var kept bool
		require.NoError(t, openDatabase(t, cfg, dbName).QueryRow(`SELECT to_regclass('public.keep_me') IS NOT NULL`).Scan(&kept))
		assert.True(t, kept, "the leftover database is never changed")
	})

	t.Run("an ID a workspace already has is a conflict", func(t *testing.T) {
		id := uniqueID("dupe")
		dropDatabaseAtCleanup(t, admin, cfg.Prefix+"_ws_"+id)
		status, _ := create(t, id)
		require.Equal(t, http.StatusCreated, status)
		status, body := create(t, id)
		assert.Equal(t, http.StatusConflict, status)
		assert.Equal(t, "Workspace already exists", body["error"])
	})

	t.Run("delete then create again under the same ID", func(t *testing.T) {
		id := uniqueID("recreate")
		dropDatabaseAtCleanup(t, admin, cfg.Prefix+"_ws_"+id)
		status, body := create(t, id)
		require.Equal(t, http.StatusCreated, status, "%v", body)

		resp, err := client.Post("/api/workspaces.delete", map[string]any{"id": id})
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		assert.False(t, databaseExists(t, admin, cfg.Prefix+"_ws_"+id))

		status, body = create(t, id)
		assert.Equal(t, http.StatusCreated, status, "%v", body)
	})

	// The state a deletion is in between its drop and its row deletion, made
	// deterministic: a request needing the database waits on the name lock the test
	// holds, the rows go, then the request finds the workspace gone and creates nothing.
	t.Run("a request racing a deletion does not recreate the database", func(t *testing.T) {
		id := uniqueID("delrace")
		dbName := cfg.Prefix + "_ws_" + id
		dropDatabaseAtCleanup(t, admin, dbName)
		status, body := create(t, id)
		require.Equal(t, http.StatusCreated, status, "%v", body)

		cm, err := pgdb.GetConnectionManager()
		require.NoError(t, err)

		release := holdNameLock(t, cfg, dbName)
		require.NoError(t, cm.CloseWorkspaceConnection(id))
		_, err = admin.Exec(`DROP DATABASE ` + quoteIdent(dbName) + ` WITH (FORCE)`)
		require.NoError(t, err)

		requestErr := make(chan error, 1)
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			_, err := cm.GetWorkspaceConnection(ctx, id)
			requestErr <- err
		}()
		waitForNameLockWaiters(t, admin, dbName, 1)

		for _, q := range []string{
			`DELETE FROM user_workspaces WHERE workspace_id = $1`,
			`DELETE FROM workspace_invitations WHERE workspace_id = $1`,
			`DELETE FROM workspaces WHERE id = $1`,
		} {
			_, err := systemDB.Exec(q, id)
			require.NoError(t, err)
		}
		require.NoError(t, release())

		select {
		case err := <-requestErr:
			var notFound *domain.ErrWorkspaceNotFound
			require.True(t, errors.As(err, &notFound), "got %v", err)
		case <-time.After(barrierTimeout):
			require.FailNow(t, "the request never returned")
		}
		assert.False(t, databaseExists(t, admin, dbName), "the database must not come back")
	})
}
