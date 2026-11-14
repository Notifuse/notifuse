//go:build integration
// +build integration

package integration

import (
	"context"
	"database/sql"
	"testing"

	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/migrations"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// v44 removes the segment memberships whose contact no longer exists (mostly left behind by
// deletions before 38.0) without a segment.left timeline entry or webhook delivery for the erased address,
// keeps the memberships of existing contacts, and leaves both membership triggers working.
func TestV44OrphanSegmentMembershipsMigration(t *testing.T) {
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

	const kept = "v44-kept@example.test"
	const gone = "v44-gone@example.test"
	_, err = factory.CreateContact(workspace.ID, testutil.WithContactEmail(kept))
	require.NoError(t, err)
	segment, err := factory.CreateSegment(workspace.ID)
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, `
		INSERT INTO webhook_subscriptions (id, name, url, secret, settings, enabled)
		VALUES ('v44-sub', 'crm', 'https://crm.example.com/hook', 'whsec_x', $1::jsonb, true)`,
		`{"event_types": ["segment.left", "segment.joined"]}`)
	require.NoError(t, err)

	insertMembership := func(t *testing.T, email string) {
		t.Helper()
		_, err := db.ExecContext(ctx, `
			INSERT INTO contact_segments (email, segment_id, version, matched_at, computed_at)
			VALUES ($1, $2, 1, NOW(), NOW())`, email, segment.ID)
		require.NoError(t, err)
	}
	insertMembership(t, kept)
	insertMembership(t, gone) // no contact row, as a deletion before 38.0 left behind

	runV44 := func(t *testing.T) *sql.Tx {
		t.Helper()
		tx, err := db.BeginTx(ctx, nil)
		require.NoError(t, err)
		// Rolled back if the test fails before its commit, so no lock outlives it (a no-op after it).
		t.Cleanup(func() { _ = tx.Rollback() })
		require.NoError(t, (&migrations.V44Migration{}).UpdateWorkspace(ctx, &config.Config{}, workspace, tx))
		return tx
	}

	// The delete is the first statement that reads contacts: with contacts locked elsewhere, it
	// fails on the lock timeout after the triggers were disabled, and the rollback restores them.
	t.Run("a run failing after disabling the triggers leaves the orphan and both triggers enabled", func(t *testing.T) {
		locker, err := db.BeginTx(ctx, nil)
		require.NoError(t, err)
		defer func() { _ = locker.Rollback() }()
		_, err = locker.ExecContext(ctx, `LOCK TABLE contacts IN ACCESS EXCLUSIVE MODE`)
		require.NoError(t, err)

		tx, err := db.BeginTx(ctx, nil)
		require.NoError(t, err)
		defer func() { _ = tx.Rollback() }()
		err = (&migrations.V44Migration{}).UpdateWorkspace(ctx, &config.Config{}, workspace, tx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "remove the segment memberships of deleted contacts")
		require.NoError(t, tx.Rollback())
		require.NoError(t, locker.Rollback())

		assert.Equal(t, 1, v44MembershipCount(t, db, gone, segment.ID))
		assertV44MembershipTriggersEnabled(t, db)
	})

	t.Run("v44 removes the orphan without any exit event, and runs twice", func(t *testing.T) {
		leftTimelineBefore := v44SegmentLeftTimelineCount(t, db, gone)
		leftDeliveriesBefore := v44SegmentLeftDeliveryCount(t, db, gone)

		tx := runV44(t)
		// Until commit, a membership write waits for the migration and a read goes through, so
		// no write runs while the triggers are disabled.
		other, err := db.BeginTx(ctx, nil)
		require.NoError(t, err)
		defer func() { _ = other.Rollback() }()
		_, err = other.ExecContext(ctx, `SET LOCAL lock_timeout = '200ms'`)
		require.NoError(t, err)
		_, err = other.ExecContext(ctx, `
			INSERT INTO contact_segments (email, segment_id, version, matched_at, computed_at)
			VALUES ('v44-late@example.test', $1, 1, NOW(), NOW())`, segment.ID)
		var pqErr *pq.Error
		require.ErrorAs(t, err, &pqErr)
		assert.Equal(t, pq.ErrorCode("55P03"), pqErr.Code, "the write waits for the migration's lock")
		require.NoError(t, other.Rollback())
		reader, err := db.BeginTx(ctx, nil)
		require.NoError(t, err)
		defer func() { _ = reader.Rollback() }()
		_, err = reader.ExecContext(ctx, `SET LOCAL lock_timeout = '200ms'`)
		require.NoError(t, err)
		var members int
		require.NoError(t, reader.QueryRowContext(ctx,
			`SELECT count(*) FROM contact_segments WHERE segment_id = $1`, segment.ID).Scan(&members),
			"a read does not wait for the migration")
		assert.Equal(t, 2, members, "a read sees the rows as they were before the migration")
		require.NoError(t, reader.Rollback())
		require.NoError(t, tx.Commit())

		require.NoError(t, runV44(t).Commit())

		assert.Equal(t, 0, v44MembershipCount(t, db, gone, segment.ID), "the orphan membership is removed")
		assert.Equal(t, 1, v44MembershipCount(t, db, kept, segment.ID), "an existing contact keeps its membership")
		assert.Equal(t, leftTimelineBefore, v44SegmentLeftTimelineCount(t, db, gone), "no segment.left timeline entry for the erased address")
		assert.Equal(t, leftDeliveriesBefore, v44SegmentLeftDeliveryCount(t, db, gone), "no segment.left delivery for the erased address")
		assertV44MembershipTriggersEnabled(t, db)
	})

	t.Run("the membership triggers still fire after the run", func(t *testing.T) {
		leftTimelineBefore := v44SegmentLeftTimelineCount(t, db, kept)
		leftDeliveriesBefore := v44SegmentLeftDeliveryCount(t, db, kept)

		_, err := db.ExecContext(ctx, `DELETE FROM contact_segments WHERE email = $1 AND segment_id = $2`, kept, segment.ID)
		require.NoError(t, err)

		assert.Equal(t, leftTimelineBefore+1, v44SegmentLeftTimelineCount(t, db, kept))
		assert.Equal(t, leftDeliveriesBefore+1, v44SegmentLeftDeliveryCount(t, db, kept))
	})
}

func v44MembershipCount(t *testing.T, db *sql.DB, email, segmentID string) int {
	t.Helper()
	var n int
	require.NoError(t, db.QueryRow(
		`SELECT count(*) FROM contact_segments WHERE email = $1 AND segment_id = $2`, email, segmentID).Scan(&n))
	return n
}

func v44SegmentLeftTimelineCount(t *testing.T, db *sql.DB, email string) int {
	t.Helper()
	var n int
	require.NoError(t, db.QueryRow(
		`SELECT count(*) FROM contact_timeline WHERE email = $1 AND kind = 'segment.left'`, email).Scan(&n))
	return n
}

func v44SegmentLeftDeliveryCount(t *testing.T, db *sql.DB, email string) int {
	t.Helper()
	var n int
	require.NoError(t, db.QueryRow(
		`SELECT count(*) FROM webhook_deliveries WHERE event_type = 'segment.left' AND payload->>'email' = $1`, email).Scan(&n))
	return n
}

func assertV44MembershipTriggersEnabled(t *testing.T, db *sql.DB) {
	t.Helper()
	for _, name := range []string{"contact_segment_changes_trigger", "webhook_contact_segments"} {
		var enabled string
		require.NoError(t, db.QueryRow(`
			SELECT tgenabled FROM pg_trigger
			WHERE tgrelid = 'contact_segments'::regclass AND tgname = $1 AND NOT tgisinternal`, name).Scan(&enabled))
		assert.Equal(t, "O", enabled, name)
	}
}
