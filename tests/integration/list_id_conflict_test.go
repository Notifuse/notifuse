//go:build integration
// +build integration

package integration

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestListIDConflict creates a list under an id already taken, by a live list and then by a
// deleted one, through the API.
//
// lists.create used to answer both with a 500 "Failed to create list", so the console showed
// a generic failure. A deleted list keeps its id (deletion only stamps deleted_at, and the id
// is the primary key) while lists.get answers 404 for it, which is what the console's check
// while typing reads: the create is where the author learns the id cannot be reused.
func TestListIDConflict(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	client := suite.APIClient
	factory := suite.DataFactory

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	workspaceID := workspace.ID
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspaceID, "owner"))
	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(workspaceID)

	type apiResult struct {
		status int
		body   map[string]interface{}
		raw    string
	}
	read := func(t *testing.T, resp *http.Response, err error) apiResult {
		t.Helper()
		require.NoError(t, err)
		raw, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		require.NoError(t, readErr)
		result := apiResult{status: resp.StatusCode, raw: string(raw)}
		_ = json.Unmarshal(raw, &result.body)
		return result
	}
	create := func(t *testing.T, id string) apiResult {
		t.Helper()
		resp, err := client.CreateList(map[string]interface{}{"workspace_id": workspaceID, "id": id, "name": "News"})
		return read(t, resp, err)
	}

	listID := "news" + strings.ReplaceAll(uuid.New().String(), "-", "")[:20]

	first := create(t, listID)
	require.Equalf(t, http.StatusCreated, first.status, "lists.create: %s", first.raw)

	t.Run("an id taken by a live list", func(t *testing.T) {
		result := create(t, listID)
		require.Equalf(t, http.StatusBadRequest, result.status, "lists.create: %s", result.raw)
		assert.Equal(t, "list_id_exists", result.body["code"])
		assert.Equal(t, "List id already exists", result.body["error"])
	})

	resp, err := client.Post("/api/lists.delete", map[string]interface{}{"workspace_id": workspaceID, "id": listID})
	deleted := read(t, resp, err)
	require.Equalf(t, http.StatusOK, deleted.status, "lists.delete: %s", deleted.raw)

	t.Run("lists.get no longer finds a deleted list", func(t *testing.T) {
		resp, err := client.Get("/api/lists.get", map[string]string{"workspace_id": workspaceID, "id": listID})
		result := read(t, resp, err)
		assert.Equalf(t, http.StatusNotFound, result.status, "lists.get: %s", result.raw)
	})

	t.Run("an id taken by a deleted list", func(t *testing.T) {
		result := create(t, listID)
		require.Equalf(t, http.StatusBadRequest, result.status, "lists.create: %s", result.raw)
		assert.Equal(t, "list_id_deleted", result.body["code"])
		assert.Contains(t, result.body["error"], "deleted list")
	})
}
