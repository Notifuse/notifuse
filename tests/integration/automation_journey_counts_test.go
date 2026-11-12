//go:build integration
// +build integration

package integration

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"

	shortuuid "github.com/lithammer/shortuuid/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// automations.list?with_journey_counts=true counts the journeys of each listed automation by
// status, and gives zeros to an automation without any.
func TestAutomationJourneyCounts(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface { return app.NewApp(cfg) })
	defer suite.Cleanup()
	factory := suite.DataFactory
	client := suite.APIClient

	owner, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(owner.ID, workspace.ID, "owner"))
	require.NoError(t, client.Login(owner.Email, "password"))
	client.SetWorkspaceID(workspace.ID)
	db, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	create := func(t *testing.T) string {
		t.Helper()
		id, triggerID := shortuuid.New(), shortuuid.New()
		resp, err := client.CreateAutomation(map[string]interface{}{
			"workspace_id": workspace.ID,
			"automation": map[string]interface{}{
				"id": id, "workspace_id": workspace.ID, "name": "Journeys " + id, "status": "draft",
				"trigger":      map[string]interface{}{"event_kind": "custom_event", "custom_event_name": "journeys", "frequency": "once"},
				"root_node_id": triggerID,
				"nodes": []map[string]interface{}{{
					"id": triggerID, "automation_id": id, "type": "trigger", "config": map[string]interface{}{},
					"position": map[string]interface{}{"x": 0, "y": 0},
				}},
			},
		})
		require.NoError(t, err)
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
		return id
	}
	withJourneys, without := create(t), create(t)

	n := 0
	for status, count := range map[string]int{"active": 3, "completed": 2, "exited": 4, "failed": 1} {
		for i := 0; i < count; i++ {
			n++
			_, err := db.Exec(`INSERT INTO contact_automations (id, automation_id, contact_email, status) VALUES ($1, $2, $3, $4)`,
				shortuuid.New(), withJourneys, fmt.Sprintf("journey-%d@example.com", n), status)
			require.NoError(t, err)
		}
	}

	list := func(t *testing.T, params map[string]string) (int, map[string]json.RawMessage) {
		t.Helper()
		query := map[string]string{"workspace_id": workspace.ID}
		for k, v := range params {
			query[k] = v
		}
		resp, err := client.Get("/api/automations.list", query)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		var body map[string]json.RawMessage
		if resp.StatusCode == http.StatusOK {
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		}
		return resp.StatusCode, body
	}

	t.Run("exact counts by status, zeros without journey", func(t *testing.T) {
		status, body := list(t, map[string]string{"with_journey_counts": "true"})
		require.Equal(t, http.StatusOK, status)
		var counts map[string]domain.JourneyCounts
		require.NoError(t, json.Unmarshal(body["journey_counts"], &counts))
		assert.Equal(t, map[string]domain.JourneyCounts{
			withJourneys: {Active: 3, Completed: 2, Exited: 4, Failed: 1, Total: 10},
			without:      {},
		}, counts)
	})

	t.Run("not asked: no journey_counts", func(t *testing.T) {
		status, body := list(t, nil)
		require.Equal(t, http.StatusOK, status)
		assert.NotContains(t, body, "journey_counts")
	})

	t.Run("an invalid value is a 400", func(t *testing.T) {
		status, _ := list(t, map[string]string{"with_journey_counts": "maybe"})
		assert.Equal(t, http.StatusBadRequest, status)
	})
}
