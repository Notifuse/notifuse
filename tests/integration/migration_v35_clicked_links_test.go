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

// TestV35MigrationClickedLinks runs the ACTUAL compiled V35 migration against a
// real workspace Postgres database. Unlike the sqlmock unit test (which only
// checks the statements are issued), this proves the SQL is valid against the
// real schema: the clicked_links column is created, the Supabase notifications'
// tracking opt-out is applied only to real 'supabase_*' IDs (the LIKE '_' is
// escaped, not a wildcard), and a poisoned jsonb-null tracking_settings value
// neither crashes the migration nor gets corrupted by '||' array concatenation.
func TestV35MigrationClickedLinks(t *testing.T) {
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

	// A fresh workspace is created with the current schema (clicked_links
	// included), so drop the column to simulate a pre-v35 workspace and prove
	// the migration is what brings it back.
	_, err = db.ExecContext(ctx, `ALTER TABLE message_history DROP COLUMN IF EXISTS clicked_links`)
	require.NoError(t, err)

	// --- Seed the notification states the data fix must handle ---

	// (a) Supabase notification with an object tracking_settings (the normal
	//     case): must gain tracking_mode "disabled", keeping existing keys
	_, err = db.ExecContext(ctx, `
		INSERT INTO transactional_notifications (id, name, channels, tracking_settings, integration_id)
		VALUES ('supabase_signup_000001', 'Supabase Signup', '{}'::jsonb,
			'{"enable_tracking": false}'::jsonb, 'supabase-integration-1')`)
	require.NoError(t, err)

	// (b) Supabase notification with SQL NULL tracking_settings: the COALESCE
	//     branch must still apply the opt-out
	_, err = db.ExecContext(ctx, `
		INSERT INTO transactional_notifications (id, name, channels, tracking_settings, integration_id)
		VALUES ('supabase_recovery_000001', 'Supabase Recovery', '{}'::jsonb, NULL, 'supabase-integration-1')`)
	require.NoError(t, err)

	// (c) poisoned row whose tracking_settings is the JSON scalar 'null' (what
	//     json.Marshal of a nil struct pointer writes): '||' on it degrades to
	//     array concatenation, so the jsonb_typeof guard must skip it untouched
	_, err = db.ExecContext(ctx, `
		INSERT INTO transactional_notifications (id, name, channels, tracking_settings, integration_id)
		VALUES ('supabase_invite_000001', 'Supabase Invite', '{}'::jsonb, 'null'::jsonb, 'supabase-integration-1')`)
	require.NoError(t, err)

	// (d) non-Supabase notification whose id would match an unescaped LIKE
	//     'supabase_%' (the '_' wildcard): must NOT be touched
	_, err = db.ExecContext(ctx, `
		INSERT INTO transactional_notifications (id, name, channels, tracking_settings)
		VALUES ('supabaseXfoo', 'Not Supabase', '{}'::jsonb, '{"enable_tracking": true}'::jsonb)`)
	require.NoError(t, err)

	// (e) user-created notification whose id happens to start with 'supabase_'
	//     but has no integration_id: must NOT be opted out (the flag is
	//     server-managed and could never be unset over the API)
	_, err = db.ExecContext(ctx, `
		INSERT INTO transactional_notifications (id, name, channels, tracking_settings)
		VALUES ('supabase_welcome', 'User Created', '{}'::jsonb, '{"enable_tracking": true}'::jsonb)`)
	require.NoError(t, err)

	// (f) Supabase notification carrying the interim disable_tracking key (a dev
	//     database migrated with the pre-tri-state build): the key must be
	//     stripped and replaced by tracking_mode
	_, err = db.ExecContext(ctx, `
		INSERT INTO transactional_notifications (id, name, channels, tracking_settings, integration_id)
		VALUES ('supabase_email_change_000001', 'Supabase Email Change', '{}'::jsonb,
			'{"disable_tracking": true}'::jsonb, 'supabase-integration-1')`)
	require.NoError(t, err)

	// --- Run the actual migration ---
	err = (&migrations.V35Migration{}).UpdateWorkspace(ctx, &config.Config{}, workspace, db)
	require.NoError(t, err, "V35 migration must not error (including on jsonb 'null' tracking_settings)")

	// --- Assert the schema change ---

	var dataType string
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT data_type FROM information_schema.columns
		WHERE table_name = 'message_history' AND column_name = 'clicked_links'`).Scan(&dataType))
	assert.Equal(t, "jsonb", dataType, "clicked_links column must exist as JSONB after migration")

	// --- Assert the tracking_settings data fix ---

	var signupSettings string
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT tracking_settings::text FROM transactional_notifications
		WHERE id = 'supabase_signup_000001'`).Scan(&signupSettings))
	assert.JSONEq(t, `{"enable_tracking": false, "tracking_mode": "disabled"}`, signupSettings,
		"object tracking_settings must gain tracking_mode and keep existing keys")

	var recoverySettings string
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT tracking_settings::text FROM transactional_notifications
		WHERE id = 'supabase_recovery_000001'`).Scan(&recoverySettings))
	assert.JSONEq(t, `{"tracking_mode": "disabled"}`, recoverySettings,
		"NULL tracking_settings must become an object with tracking_mode disabled")

	var inviteSettings string
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT tracking_settings::text FROM transactional_notifications
		WHERE id = 'supabase_invite_000001'`).Scan(&inviteSettings))
	assert.Equal(t, "null", inviteSettings,
		"jsonb-null tracking_settings must be left untouched (and must not crash the migration)")

	var emailChangeSettings string
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT tracking_settings::text FROM transactional_notifications
		WHERE id = 'supabase_email_change_000001'`).Scan(&emailChangeSettings))
	assert.JSONEq(t, `{"tracking_mode": "disabled"}`, emailChangeSettings,
		"the interim disable_tracking key must be stripped and replaced by tracking_mode")

	var userCreatedSettings string
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT tracking_settings::text FROM transactional_notifications
		WHERE id = 'supabase_welcome'`).Scan(&userCreatedSettings))
	assert.JSONEq(t, `{"enable_tracking": true}`, userCreatedSettings,
		"a user-created notification without integration_id must NOT be opted out")

	var otherSettings string
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT tracking_settings::text FROM transactional_notifications
		WHERE id = 'supabaseXfoo'`).Scan(&otherSettings))
	assert.JSONEq(t, `{"enable_tracking": true}`, otherSettings,
		"an id matching only via the '_' wildcard must NOT be modified")

	// Idempotency: running again must be a clean no-op.
	err = (&migrations.V35Migration{}).UpdateWorkspace(ctx, &config.Config{}, workspace, db)
	require.NoError(t, err, "V35 migration must be idempotent")

	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT tracking_settings::text FROM transactional_notifications
		WHERE id = 'supabase_signup_000001'`).Scan(&signupSettings))
	assert.JSONEq(t, `{"enable_tracking": false, "tracking_mode": "disabled"}`, signupSettings,
		"re-running the migration must not change the repaired value")
}
