//go:build integration
// +build integration

package integration

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/migrations"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// v44 raises stats.exited of each automation to the number of its exited journeys, so the
// journeys that left by replying before 44, never counted there, are counted.
func TestV44ExitedRecount(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()
	factory := suite.DataFactory
	ctx := context.Background()

	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	db, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	// Each automation: its stored stats (NULL for SQL NULL), whether it is deleted, and its
	// journeys by status.
	type automation struct {
		stats    sql.NullString
		deleted  bool
		journeys map[string]int
	}
	cases := map[string]automation{
		"short":       {stats: sql.NullString{String: `{"entered": 9, "exited": 1, "completed": 4}`, Valid: true}, journeys: map[string]int{"exited": 3, "completed": 4, "active": 2}},
		"no-key":      {stats: sql.NullString{String: `{"entered": 2}`, Valid: true}, journeys: map[string]int{"exited": 2}},
		"null-stats":  {journeys: map[string]int{"exited": 1}},
		"not-a-count": {stats: sql.NullString{String: `{"exited": "abc"}`, Valid: true}, journeys: map[string]int{"exited": 2}},
		"ahead":       {stats: sql.NullString{String: `{"exited": 7}`, Valid: true}, journeys: map[string]int{"exited": 2}},
		"equal":       {stats: sql.NullString{String: `{"exited": 2}`, Valid: true}, journeys: map[string]int{"exited": 2}},
		"no-exit":     {stats: sql.NullString{String: `{"exited": 0}`, Valid: true}, journeys: map[string]int{"completed": 3}},
		"deleted":     {stats: sql.NullString{String: `{"exited": 0}`, Valid: true}, deleted: true, journeys: map[string]int{"exited": 4}},
		"array-stats": {stats: sql.NullString{String: `[1, 2]`, Valid: true}, journeys: map[string]int{"exited": 4}},
		"json-null":   {stats: sql.NullString{String: `null`, Valid: true}, journeys: map[string]int{"exited": 4}},
		"decimal":     {stats: sql.NullString{String: `{"exited": 7.0}`, Valid: true}, journeys: map[string]int{"exited": 2}},
		"boolean":     {stats: sql.NullString{String: `{"exited": true}`, Valid: true}, journeys: map[string]int{"exited": 2}},
		"beyond-int":  {stats: sql.NullString{String: `{"exited": 99999999999}`, Valid: true}, journeys: map[string]int{"exited": 2}},
	}
	for id, a := range cases {
		var stats interface{}
		if a.stats.Valid {
			stats = a.stats.String
		}
		_, err := db.ExecContext(ctx, `
			INSERT INTO automations (id, workspace_id, name, status, trigger_config, stats, updated_at, deleted_at)
			VALUES ($1, $2, $1, 'live', '{}'::jsonb, $3::jsonb, '2026-01-01T00:00:00Z',
			        CASE WHEN $4 THEN NOW() END)`, id, workspace.ID, stats, a.deleted)
		require.NoError(t, err, id)
		n := 0
		for status, count := range a.journeys {
			for i := 0; i < count; i++ {
				n++
				_, err := db.ExecContext(ctx, `
					INSERT INTO contact_automations (id, automation_id, contact_email, status)
					VALUES ($1, $2, $3, $4)`,
					fmt.Sprintf("%s-%d", id, n), id, fmt.Sprintf("%s-%d@example.test", id, n), status)
				require.NoError(t, err, id)
			}
		}
	}

	stored := func(t *testing.T, id string) (sql.NullString, bool) {
		t.Helper()
		var stats sql.NullString
		var touched bool
		require.NoError(t, db.QueryRowContext(ctx,
			`SELECT stats::text, updated_at > '2026-01-01T00:00:00Z' FROM automations WHERE id = $1`, id).Scan(&stats, &touched))
		return stats, touched
	}

	runV44 := func(t *testing.T) {
		t.Helper()
		tx, err := db.BeginTx(ctx, nil)
		require.NoError(t, err)
		require.NoError(t, (&migrations.V44Migration{}).UpdateWorkspace(ctx, &config.Config{}, workspace, tx))
		require.NoError(t, tx.Commit())
	}

	runV44(t)

	t.Run("raised to the exited journeys, the other counters kept", func(t *testing.T) {
		want := map[string]string{
			"short":       `{"exited": 3, "entered": 9, "completed": 4}`,
			"no-key":      `{"exited": 2, "entered": 2}`,
			"null-stats":  `{"exited": 1}`,
			"not-a-count": `{"exited": 2}`,
			// Not a whole number: read as 0, so set to the journeys, without an error.
			"decimal": `{"exited": 2}`,
			"boolean": `{"exited": 2}`,
		}
		for id, stats := range want {
			got, touched := stored(t, id)
			require.True(t, got.Valid, id)
			assert.JSONEq(t, stats, got.String, id)
			assert.True(t, touched, "%s: updated_at moves with the stats", id)
		}
	})

	t.Run("a whole number never lowered, and left alone when nothing is missing", func(t *testing.T) {
		for _, id := range []string{"ahead", "equal", "no-exit", "deleted", "array-stats", "json-null", "beyond-int"} {
			got, touched := stored(t, id)
			assert.Equal(t, cases[id].stats, got, id)
			assert.False(t, touched, id)
		}
	})

	t.Run("a rerun writes nothing", func(t *testing.T) {
		_, err := db.ExecContext(ctx, `UPDATE automations SET updated_at = '2026-01-01T00:00:00Z'`)
		require.NoError(t, err)
		runV44(t)
		for id := range cases {
			_, touched := stored(t, id)
			assert.False(t, touched, id)
		}
	})
}
