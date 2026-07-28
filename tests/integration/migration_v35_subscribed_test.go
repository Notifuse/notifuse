package integration

import (
	"context"
	"testing"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/migrations"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestV35MigrationSubscribedRepair runs the ACTUAL compiled V35 migration against
// a real workspace Postgres database seeded with the invalid 'subscribed' status
// in every place the bug produced it. Unlike the sqlmock unit test (which only
// checks the statements are issued), this proves the SQL is valid against the real
// schema, actually converts the data, and does not crash on a no-nodes automation
// whose nodes column is the JSON scalar 'null' (what json.Marshal(nil slice) writes).
func TestV35MigrationSubscribedRepair(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	factory := suite.DataFactory
	ctx := context.Background()

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))

	db, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	// --- Seed the corruption exactly as the bug produced it ---

	// (a) automation whose add_to_list node stored the invalid 'subscribed' status
	_, err = db.ExecContext(ctx, `
		INSERT INTO automations (id, workspace_id, name, status, trigger_config, nodes, stats)
		VALUES ('auto_sub', $1, 'Bug Repro', 'draft', '{}'::jsonb,
			'[{"id":"n1","type":"add_to_list","config":{"list_id":"l1","status":"subscribed"}}]'::jsonb,
			'{}'::jsonb)`, workspace.ID)
	require.NoError(t, err)

	// (b) a no-nodes automation stored as the JSON scalar 'null' — the crash case
	//     that json.Marshal(nil []*AutomationNode) writes. jsonb_array_elements would
	//     raise "cannot extract elements from a scalar" without the jsonb_typeof guard.
	_, err = db.ExecContext(ctx, `
		INSERT INTO automations (id, workspace_id, name, status, trigger_config, nodes, stats)
		VALUES ('auto_nonodes', $1, 'No Nodes', 'draft', '{}'::jsonb, 'null'::jsonb, '{}'::jsonb)`, workspace.ID)
	require.NoError(t, err)

	// (c) contact_lists row that got the invalid status
	_, err = db.ExecContext(ctx, `
		INSERT INTO contact_lists (email, list_id, status) VALUES ('c@x.com', 'l1', 'subscribed')`)
	require.NoError(t, err)

	// (d) contact_timeline entry recording a change to 'subscribed'
	_, err = db.ExecContext(ctx, `
		INSERT INTO contact_timeline (email, operation, entity_type, kind, changes, created_at)
		VALUES ('c@x.com', 'update', 'contact_list', 'update_contact_list',
			'{"status":{"old":"pending","new":"subscribed"}}'::jsonb, NOW())`)
	require.NoError(t, err)

	// --- Run the actual migration ---
	err = (&migrations.V35Migration{}).UpdateWorkspace(ctx, &config.Config{}, workspace, db)
	require.NoError(t, err, "V35 migration must not error (including on jsonb 'null' nodes)")

	// --- Assert every corrupted value was repaired ---

	var nodeStatus string
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT nodes->0->'config'->>'status' FROM automations WHERE id='auto_sub'`).Scan(&nodeStatus))
	assert.Equal(t, "active", nodeStatus, "add_to_list node status must be repaired to active")

	var noNodes string
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT nodes::text FROM automations WHERE id='auto_nonodes'`).Scan(&noNodes))
	assert.Equal(t, "null", noNodes, "no-nodes automation must be left untouched (and must not crash the migration)")

	var clStatus string
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT status FROM contact_lists WHERE email='c@x.com' AND list_id='l1'`).Scan(&clStatus))
	assert.Equal(t, "active", clStatus, "contact_lists status must be repaired to active")

	var tlNew string
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT changes->'status'->>'new' FROM contact_timeline WHERE email='c@x.com'`).Scan(&tlNew))
	assert.Equal(t, "active", tlNew, "contact_timeline changes must be repaired to active")

	// Idempotency: running again must be a clean no-op.
	err = (&migrations.V35Migration{}).UpdateWorkspace(ctx, &config.Config{}, workspace, db)
	require.NoError(t, err, "V35 migration must be idempotent")
}
