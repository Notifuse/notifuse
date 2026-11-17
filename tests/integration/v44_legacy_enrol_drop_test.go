//go:build integration
// +build integration

package integration

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/migrations"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// v44 drops the enrolment v20 and v21 created with a VARCHAR p_list_id, which v25 left beside
// its four-argument replacement on every workspace created before v25. Without the drop, a
// call whose fifth argument has no type resolves to that overload, not to the JSONB one:
// PostgreSQL prefers the string type for an unknown argument.
func TestV44DropsTheLegacyEnrolment(t *testing.T) {
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

	// The signature v20 and v21 installed, with a body that fails, so that a call reaching it
	// fails the test.
	_, err = db.ExecContext(ctx, `
		CREATE OR REPLACE FUNCTION automation_enroll_contact(
			p_automation_id VARCHAR(36),
			p_contact_email VARCHAR(255),
			p_root_node_id VARCHAR(36),
			p_list_id VARCHAR(36),
			p_frequency VARCHAR(20)
		) RETURNS VOID AS $$ BEGIN RAISE EXCEPTION 'legacy enrolment called'; END; $$ LANGUAGE plpgsql`)
	require.NoError(t, err)

	// No automation has this ID, so the JSONB enrolment returns before writing anything.
	untypedCall := `SELECT automation_enroll_contact('none', 'a@example.test', 'root', 'once', NULL)`

	signatures := func(t *testing.T) []string {
		t.Helper()
		rows, err := db.QueryContext(ctx, `
			SELECT pg_get_function_identity_arguments(oid) FROM pg_proc
			WHERE proname = 'automation_enroll_contact' ORDER BY 1`)
		require.NoError(t, err)
		defer func() { _ = rows.Close() }()
		var got []string
		for rows.Next() {
			var s string
			require.NoError(t, rows.Scan(&s))
			got = append(got, s)
		}
		require.NoError(t, rows.Err())
		return got
	}
	const legacy = "p_automation_id character varying, p_contact_email character varying, p_root_node_id character varying, p_list_id character varying, p_frequency character varying"
	const withTrigger = "p_automation_id character varying, p_contact_email character varying, p_root_node_id character varying, p_frequency character varying, p_trigger jsonb"
	require.Contains(t, signatures(t), legacy)
	_, err = db.ExecContext(ctx, untypedCall)
	require.ErrorContains(t, err, "legacy enrolment called", "before 44 the untyped call reaches the legacy overload")

	runV44 := func(t *testing.T) {
		t.Helper()
		tx, err := db.BeginTx(ctx, nil)
		require.NoError(t, err)
		require.NoError(t, (&migrations.V44Migration{}).UpdateWorkspace(ctx, &config.Config{}, workspace, tx))
		require.NoError(t, tx.Commit())
	}
	runV44(t)

	assert.NotContains(t, signatures(t), legacy)
	assert.Contains(t, signatures(t), withTrigger)

	_, err = db.ExecContext(ctx, untypedCall)
	require.NoError(t, err, "after 44 the untyped call reaches the JSONB overload")

	t.Run("a rerun finds nothing to drop", func(t *testing.T) {
		runV44(t)
		assert.NotContains(t, signatures(t), legacy)
		assert.Contains(t, signatures(t), withTrigger)
	})
}
