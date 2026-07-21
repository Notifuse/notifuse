package integration

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// TestSegmentQueueClaimBlocksBroadcastInsert deterministically reproduces the
// core mechanism of the segment-queue idle-in-transaction deadlock.
//
// ProcessQueue (internal/service/contact_segment_queue_processor.go:45) claims
// queue rows inside a transaction with:
//
//	SELECT email FROM contact_segment_queue
//	WHERE queued_at < NOW() - INTERVAL '15 seconds'
//	ORDER BY queued_at ASC LIMIT $1 FOR UPDATE SKIP LOCKED
//
// and then does every per-contact write on a DIFFERENT pooled connection —
// leaving that transaction idle-in-transaction while it still holds the row
// locks (contact_segment_queue_processor.go:107-108 pass workspaceDB, not tx).
//
// This test opens exactly that claim transaction, holds it open, and shows that
// a concurrent broadcast send — INSERT INTO message_history on a pooled
// connection — blocks indefinitely. It blocks because the message_history
// AFTER INSERT trigger cascade
//
//	track_message_history_changes()  (writes contact_timeline 'email.sent')
//	  -> contact_timeline_queue_trigger
//	    -> queue_contact_for_segment_recomputation()
//	      -> INSERT INTO contact_segment_queue ... ON CONFLICT (email) DO UPDATE
//
// tries to UPSERT the very contact_segment_queue row the claim transaction has
// locked. That is the "pid 37" symptom from the report: broadcasts stall at the
// DB level behind the idle claimer, and because the claimer is idle (not itself
// waiting on a lock) the wait is invisible to Postgres's deadlock detector.
//
// A control send for an UNLOCKED contact succeeds while the claim is open, and
// the identical locked send succeeds immediately once the claim commits —
// proving the block is the claim lock and nothing else.
func TestSegmentQueueClaimBlocksBroadcastInsert(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	factory := suite.DataFactory
	ctx := context.Background()

	ws, err := factory.CreateWorkspace()
	require.NoError(t, err)

	workspaceDB, err := suite.DBManager.GetWorkspaceDB(ws.ID)
	require.NoError(t, err)

	victim := fmt.Sprintf("victim-%s@example.com", uuid.New().String()[:8])
	bystander := fmt.Sprintf("bystander-%s@example.com", uuid.New().String()[:8])

	// A real template so the message_history inserts reference a valid template_id.
	tmpl, err := factory.CreateTemplate(ws.ID, testutil.WithTemplateName("segqueue-repro"))
	require.NoError(t, err)

	_, err = factory.CreateContact(ws.ID, testutil.WithContactEmail(victim))
	require.NoError(t, err)
	_, err = factory.CreateContact(ws.ID, testutil.WithContactEmail(bystander))
	require.NoError(t, err)

	// Seed a queue row for the victim, aged past the worker's 15s debounce so the
	// claim SELECT is guaranteed to lock it (getPendingEmailsInTx filters on
	// queued_at < NOW() - INTERVAL '15 seconds').
	_, err = workspaceDB.ExecContext(ctx, `
		INSERT INTO contact_segment_queue (email, queued_at)
		VALUES ($1, NOW() - INTERVAL '1 minute')
		ON CONFLICT (email) DO UPDATE SET queued_at = EXCLUDED.queued_at`, victim)
	require.NoError(t, err)

	// --- Open the claim transaction, byte-for-byte as ProcessQueue does. ------
	tx, err := workspaceDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()

	claimed := claimSegmentQueue(t, ctx, tx, 100)
	require.Contains(t, claimed, victim,
		"precondition: the claim SELECT must lock the victim's queue row")

	// tx now holds FOR UPDATE on the victim's row. Exactly like ProcessQueue
	// during its per-contact loop, it issues no further statement on tx: from
	// Postgres's side it is now idle-in-transaction, still holding the lock.

	// --- Control: a send for an UNLOCKED contact must NOT be blocked. ----------
	// Establishes that the inserts themselves work and that the block below is
	// specific to the locked row, not a global stall.
	elapsed, err := insertBroadcastSend(workspaceDB, bystander, tmpl.ID, 4*time.Second)
	require.NoError(t, err,
		"control: a broadcast send for an unlocked contact must succeed while the claim is open (took %s)", elapsed)
	t.Logf("control: unlocked-contact send succeeded in %s", elapsed)

	// --- The bug: a send for the LOCKED contact blocks on the idle claimer. ----
	elapsed, err = insertBroadcastSend(workspaceDB, victim, tmpl.ID, 4*time.Second)
	require.Error(t, err,
		"message_history insert for the locked contact must block on the idle "+
			"claim transaction via the trigger cascade (took %s)", elapsed)
	require.GreaterOrEqual(t, elapsed, 2*time.Second,
		"the insert must be blocked for the whole window, not fail fast (err=%v, elapsed=%s)", err, elapsed)
	t.Logf("reproduced: locked-contact send blocked for %s then failed with: %v", elapsed, err)

	// --- Releasing the claim unblocks the identical send immediately. ----------
	require.NoError(t, tx.Commit())

	elapsed, err = insertBroadcastSend(workspaceDB, victim, tmpl.ID, 4*time.Second)
	require.NoError(t, err,
		"after the claim commits, the identical send must succeed (took %s)", elapsed)
	require.Less(t, elapsed, 2*time.Second,
		"post-commit send should complete promptly (took %s)", elapsed)
	t.Logf("recovered: after commit the identical send succeeded in %s", elapsed)
}

// claimSegmentQueue runs the exact getPendingEmailsInTx query from
// contact_segment_queue_processor.go:143 inside tx, returning the locked emails.
// The FOR UPDATE SKIP LOCKED holds row locks until tx commits or rolls back.
func claimSegmentQueue(t *testing.T, ctx context.Context, tx *sql.Tx, limit int) []string {
	t.Helper()
	rows, err := tx.QueryContext(ctx, `
		SELECT email
		FROM contact_segment_queue
		WHERE queued_at < NOW() - INTERVAL '15 seconds'
		ORDER BY queued_at ASC
		LIMIT $1
		FOR UPDATE SKIP LOCKED`, limit)
	require.NoError(t, err)
	defer rows.Close()

	var emails []string
	for rows.Next() {
		var e string
		require.NoError(t, rows.Scan(&e))
		emails = append(emails, e)
	}
	require.NoError(t, rows.Err())
	return emails
}

// insertBroadcastSend records one send in message_history on a pooled connection,
// bounded by timeout, and reports how long the insert took. The INSERT fires the
// AFTER INSERT cascade track_message_history_changes -> contact_timeline
// ('email.sent') -> queue_contact_for_segment_recomputation, whose
// contact_segment_queue ON CONFLICT (email) DO UPDATE is what contends with a
// held claim lock. On a lock wait, the bounded context cancels the statement and
// a non-nil error is returned.
func insertBroadcastSend(db *sql.DB, email, templateID string, timeout time.Duration) (time.Duration, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	start := time.Now()
	_, err := db.ExecContext(ctx, `
		INSERT INTO message_history
			(id, contact_email, template_id, template_version, channel, message_data, sent_at)
		VALUES ($1, $2, $3, 1, 'email', '{}'::jsonb, NOW())`,
		uuid.New().String(), email, templateID)
	return time.Since(start), err
}

// TestSegmentQueueDeleteOnClaimHoldsNoLock verifies the delete-on-claim fix. The
// claim path (ProcessQueue -> claimBatch) claims rows with a DELETE ... RETURNING
// (backed by FOR UPDATE SKIP LOCKED) inside a short transaction that commits
// BEFORE any per-contact processing — so no lock is held while the claimed
// contacts are processed. (The transaction is load-bearing in production: a
// client-side scan failure rolls the DELETE back so rows survive; do not
// "simplify" claimBatch down to an autocommit QueryContext.) A concurrent
// broadcast send for a just-claimed contact must therefore complete immediately,
// instead of blocking behind an idle claimer as it did before the fix (see
// TestSegmentQueueClaimBlocksBroadcastInsert).
//
// The claim SQL below is kept identical to claimBatch in
// internal/service/contact_segment_queue_processor.go.
func TestSegmentQueueDeleteOnClaimHoldsNoLock(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	factory := suite.DataFactory
	ctx := context.Background()

	ws, err := factory.CreateWorkspace()
	require.NoError(t, err)

	workspaceDB, err := suite.DBManager.GetWorkspaceDB(ws.ID)
	require.NoError(t, err)

	victim := fmt.Sprintf("claimed-%s@example.com", uuid.New().String()[:8])
	tmpl, err := factory.CreateTemplate(ws.ID, testutil.WithTemplateName("segqueue-fix"))
	require.NoError(t, err)
	_, err = factory.CreateContact(ws.ID, testutil.WithContactEmail(victim))
	require.NoError(t, err)

	_, err = workspaceDB.ExecContext(ctx, `
		INSERT INTO contact_segment_queue (email, queued_at)
		VALUES ($1, NOW() - INTERVAL '1 minute')
		ON CONFLICT (email) DO UPDATE SET queued_at = EXCLUDED.queued_at`, victim)
	require.NoError(t, err)

	// Claim with the same statement claimBatch runs. (Production wraps it in a
	// short tx that commits immediately after the scan; lock-wise the two are
	// equivalent once committed, which is what this test asserts.)
	claimed := deleteOnClaim(t, ctx, workspaceDB, 100)
	require.Contains(t, claimed, victim, "claim must return the victim's email")

	// The claim committed on its own: the row is gone and nothing stays locked.
	require.Equal(t, 0, queueRowsFor(t, workspaceDB, victim),
		"claimed row must be deleted (the claim commits immediately)")

	// A broadcast send for the just-claimed contact must NOT be blocked.
	elapsed, err := insertBroadcastSend(workspaceDB, victim, tmpl.ID, 4*time.Second)
	require.NoError(t, err,
		"after delete-on-claim no lock is held, so the send must succeed (took %s)", elapsed)
	require.Less(t, elapsed, 2*time.Second,
		"send should complete promptly, proving no claim lock is held (took %s)", elapsed)
	t.Logf("verified: send for a just-claimed contact succeeded in %s (no lock held)", elapsed)
}

// deleteOnClaim runs the delete-on-claim statement from claimBatch and returns
// the claimed emails. It runs the statement in autocommit for simplicity;
// production claimBatch wraps the same statement in a short transaction
// (committed before any processing) so a client-side scan failure rolls the
// delete back — that difference does not affect the lock behavior asserted here.
func deleteOnClaim(t *testing.T, ctx context.Context, db *sql.DB, limit int) []string {
	t.Helper()
	rows, err := db.QueryContext(ctx, `
		WITH claimed AS (
			SELECT email
			FROM contact_segment_queue
			WHERE queued_at < NOW() - INTERVAL '15 seconds'
			ORDER BY queued_at ASC
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		DELETE FROM contact_segment_queue q
		USING claimed
		WHERE q.email = claimed.email
		RETURNING q.email`, limit)
	require.NoError(t, err)
	defer rows.Close()

	var emails []string
	for rows.Next() {
		var e string
		require.NoError(t, rows.Scan(&e))
		emails = append(emails, e)
	}
	require.NoError(t, rows.Err())
	return emails
}

// queueRowsFor counts contact_segment_queue rows for a single email.
func queueRowsFor(t *testing.T, db *sql.DB, email string) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var n int
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT count(*) FROM contact_segment_queue WHERE email = $1`, email).Scan(&n))
	return n
}
