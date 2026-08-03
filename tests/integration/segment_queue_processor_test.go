package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/repository"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSegmentQueueProcessorIntegration drives the REAL ProcessQueue
// (delete-on-claim + requeue-on-failure) against a real Postgres workspace
// database with its full trigger set. Unlike segment_queue_claim_lock_test.go —
// which pins the LOCK semantics of the claim statement with hand-written SQL —
// these subtests execute the production code path end to end and pin its
// business behavior: membership recompute, at-least-once retry via requeue,
// the 15-second debounce, batch limiting, and claim ordering.
//
// The processor is built from the exported constructors against the suite's
// workspace repository, exactly as the app wires it at boot; each subtest runs
// in its own workspace for isolation. Factory-created workspaces do not seed
// the recurring queue task, so no background scheduler competes with the
// explicit ProcessQueue calls below — every observation is deterministic.
func TestSegmentQueueProcessorIntegration(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	factory := suite.DataFactory
	ctx := context.Background()

	wsRepo := suite.ServerManager.GetApp().GetWorkspaceRepository()
	processor := service.NewContactSegmentQueueProcessor(
		repository.NewContactSegmentQueueRepository(wsRepo),
		repository.NewSegmentRepository(wsRepo),
		repository.NewContactRepository(wsRepo),
		wsRepo,
		logger.NewLoggerWithLevel("error"),
	)

	t.Run("membership add and remove with real segment SQL", func(t *testing.T) {
		ws, err := factory.CreateWorkspace()
		require.NoError(t, err)
		db, err := suite.DBManager.GetWorkspaceDB(ws.ID)
		require.NoError(t, err)

		tag := uuid.New().String()[:8]
		matching := fmt.Sprintf("match-%s@example.com", tag)
		other := fmt.Sprintf("other-%s@example.com", tag)

		_, err = factory.CreateContact(ws.ID, testutil.WithContactEmail(matching))
		require.NoError(t, err)
		_, err = factory.CreateContact(ws.ID, testutil.WithContactEmail(other))
		require.NoError(t, err)

		// A segment that discriminates between the two contacts. Its stored SQL
		// carries one bound arg, so the evaluation also exercises the email-arg
		// append + placeholder rebinding of processContact.
		segID := createSegmentWithSQL(t, db, "segmatch"+tag[:4],
			"SELECT email FROM contacts WHERE email LIKE $1",
			domain.JSONArray{"match-%"},
			string(domain.SegmentStatusActive))

		// Stale membership for the non-matching contact: the recompute must
		// remove it (and only it).
		_, err = db.ExecContext(ctx, `
			INSERT INTO contact_segments (email, segment_id, version, matched_at, computed_at)
			VALUES ($1, $2, 1, NOW(), NOW())`, other, segID)
		require.NoError(t, err)

		enqueueAged(t, db, matching)
		enqueueAged(t, db, other)

		count, err := processor.ProcessQueue(ctx, ws.ID)
		require.NoError(t, err)
		assert.Equal(t, 2, count, "both claimed contacts must be processed")

		// Membership: matching contact added, stale membership removed.
		assert.Equal(t, 1, memberCount(t, db, matching, segID), "matching contact must join the segment")
		assert.Equal(t, 0, memberCount(t, db, other, segID), "non-matching contact must leave the segment")

		// The membership writes fire the segment.joined/segment.left timeline
		// triggers; both kinds are short-circuited by the queue trigger, so the
		// queue must be empty — the worker's own writes never re-enqueue.
		assert.Equal(t, 1, timelineKindCount(t, db, matching, "segment.joined", segID))
		assert.Equal(t, 1, timelineKindCount(t, db, other, "segment.left", segID))
		assert.Equal(t, 0, segQueueSize(t, db), "queue must be fully drained; membership writes must not re-enqueue")
	})

	t.Run("segment SQL that reuses a bound placeholder still evaluates", func(t *testing.T) {
		ws, err := factory.CreateWorkspace()
		require.NoError(t, err)
		db, err := suite.DBManager.GetWorkspaceDB(ws.ID)
		require.NoError(t, err)

		tag := uuid.New().String()[:8]
		clicker := fmt.Sprintf("clicker-%s@example.com", tag)
		quiet := fmt.Sprintf("quiet-%s@example.com", tag)
		_, err = factory.CreateContact(ws.ID, testutil.WithContactEmail(clicker))
		require.NoError(t, err)
		_, err = factory.CreateContact(ws.ID, testutil.WithContactEmail(quiet))
		require.NoError(t, err)

		// Insert a real custom event so the production trigger writes contact_timeline.changes in
		// its actual {field: {old, new}} shape — a hand-rolled changes payload would not match
		// what the filter reads and would make this test pass for the wrong reason.
		_, err = db.ExecContext(ctx, `
			INSERT INTO custom_events
				(event_name, external_id, email, properties, occurred_at, source, goal_type, goal_value)
			VALUES ('shopify.order', $1, $2, '{}', NOW(), 'test', 'purchase', 10)`,
			"dup_"+tag, clicker)
		require.NoError(t, err)

		// A multi-value `contains` on a timeline change key: the key is bound ONCE and referenced
		// in BOTH comparisons, so the compiled query holds a repeated placeholder. processContact
		// splices this query into a UNION and shifts its placeholders; shifting them by order of
		// appearance rather than by their own number invented an extra placeholder here, and the
		// whole UNION failed to bind — silently stopping membership updates for the contact
		// across every segment.
		qb := service.NewQueryBuilder()
		generatedSQL, args, err := qb.BuildSQL(&domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Source: "contact_timeline",
				ContactTimeline: &domain.ContactTimelineCondition{
					Kind:          "custom_event.shopify.order",
					CountOperator: "at_least",
					CountValue:    1,
					Filters: []*domain.DimensionFilter{
						{FieldName: "goal_type", FieldType: "string", Operator: "contains",
							StringValues: []string{"purch", "subscr"}},
					},
				},
			},
		})
		require.NoError(t, err)
		require.Equal(t, 2, strings.Count(generatedSQL, "ct.changes->$2->>'new'"),
			"this test is only meaningful while the compiled query reuses a placeholder")

		segID := createSegmentWithSQL(t, db, "segdup"+tag[:4], generatedSQL,
			domain.JSONArray(args), string(domain.SegmentStatusActive))

		// A second segment so the splice also exercises a non-zero placeholder offset.
		otherID := createSegmentWithSQL(t, db, "segall"+tag[:4],
			"SELECT email FROM contacts WHERE email LIKE $1",
			domain.JSONArray{"clicker-%"}, string(domain.SegmentStatusActive))

		enqueueAged(t, db, clicker)
		enqueueAged(t, db, quiet)

		count, err := processor.ProcessQueue(ctx, ws.ID)
		require.NoError(t, err)
		assert.Equal(t, 2, count, "both contacts must be processed, not skipped by a bind failure")

		assert.Equal(t, 1, memberCount(t, db, clicker, segID),
			"the contact whose event matches must join the repeated-placeholder segment")
		assert.Equal(t, 0, memberCount(t, db, quiet, segID))
		assert.Equal(t, 1, memberCount(t, db, clicker, otherID),
			"the second segment must still evaluate correctly after the placeholder shift")
		assert.Equal(t, 0, segQueueSize(t, db), "a bind failure would have requeued the contacts")
	})

	t.Run("failed evaluation requeues with debounce then recovers", func(t *testing.T) {
		ws, err := factory.CreateWorkspace()
		require.NoError(t, err)
		db, err := suite.DBManager.GetWorkspaceDB(ws.ID)
		require.NoError(t, err)

		tag := uuid.New().String()[:8]
		victim := fmt.Sprintf("victim-%s@example.com", tag)
		_, err = factory.CreateContact(ws.ID, testutil.WithContactEmail(victim))
		require.NoError(t, err)

		// A segment whose stored SQL fails at execution: the per-contact
		// evaluation errors and the contact must take the requeue path.
		segID := createSegmentWithSQL(t, db, "segbroken"+tag[:4],
			"SELECT email FROM contacts WHERE nonexistent_column = $1",
			domain.JSONArray{"x"},
			string(domain.SegmentStatusActive))

		enqueueAged(t, db, victim)

		// Pass 1: evaluation fails -> no error surfaced (batch keeps going),
		// nothing processed, contact re-enqueued with a FRESH queued_at.
		count, err := processor.ProcessQueue(ctx, ws.ID)
		require.NoError(t, err, "a per-contact failure must not fail the batch")
		assert.Equal(t, 0, count)
		assert.Equal(t, 1, queueRowsFor(t, db, victim), "failed contact must be back in the queue")
		var fresh bool
		require.NoError(t, db.QueryRowContext(ctx, `
			SELECT queued_at > NOW() - INTERVAL '5 seconds'
			FROM contact_segment_queue WHERE email = $1`, victim).Scan(&fresh))
		assert.True(t, fresh, "requeue must reset queued_at to now (debounced retry, not immediate)")

		// Pass 2, immediately: the fresh requeue is inside the 15s debounce
		// window, so the claim must see nothing.
		count, err = processor.ProcessQueue(ctx, ws.ID)
		require.NoError(t, err)
		assert.Equal(t, 0, count, "requeued contact must be debounced, not re-claimed immediately")
		assert.Equal(t, 1, queueRowsFor(t, db, victim), "row must survive the empty pass")

		// Repair the segment and age the row past the debounce: the retry must
		// now complete the recompute — nothing was lost along the way.
		_, err = db.ExecContext(ctx, `UPDATE segments SET generated_sql = $1, generated_args = $2 WHERE id = $3`,
			"SELECT email FROM contacts WHERE email = $1", mustJSON(t, domain.JSONArray{victim}), segID)
		require.NoError(t, err)
		enqueueAged(t, db, victim)

		count, err = processor.ProcessQueue(ctx, ws.ID)
		require.NoError(t, err)
		assert.Equal(t, 1, count)
		assert.Equal(t, 1, memberCount(t, db, victim, segID), "recovered retry must land the membership")
		assert.Equal(t, 0, segQueueSize(t, db))
	})

	t.Run("no active segments drains the claimed batch", func(t *testing.T) {
		ws, err := factory.CreateWorkspace()
		require.NoError(t, err)
		db, err := suite.DBManager.GetWorkspaceDB(ws.ID)
		require.NoError(t, err)

		tag := uuid.New().String()[:8]
		email := fmt.Sprintf("noseg-%s@example.com", tag)
		enqueueAged(t, db, email)

		count, err := processor.ProcessQueue(ctx, ws.ID)
		require.NoError(t, err)
		assert.Equal(t, 1, count, "with no segments the claimed contacts count as processed")
		assert.Equal(t, 0, segQueueSize(t, db), "queue must be cleared")
	})

	t.Run("claim honors batch limit and queued_at ordering", func(t *testing.T) {
		ws, err := factory.CreateWorkspace()
		require.NoError(t, err)
		db, err := suite.DBManager.GetWorkspaceDB(ws.ID)
		require.NoError(t, err)

		// 105 aged rows with strictly increasing queued_at: one pass must claim
		// exactly batchSize (100), oldest first, leaving the 5 newest.
		tag := uuid.New().String()[:8]
		_, err = db.ExecContext(ctx, fmt.Sprintf(`
			INSERT INTO contact_segment_queue (email, queued_at)
			SELECT 'bulk-%s-' || lpad(i::text, 3, '0') || '@example.com',
			       NOW() - INTERVAL '10 minutes' + (i * INTERVAL '1 second')
			FROM generate_series(0, 104) i`, tag))
		require.NoError(t, err)

		count, err := processor.ProcessQueue(ctx, ws.ID)
		require.NoError(t, err)
		assert.Equal(t, 100, count, "one pass must claim at most the batch size")
		assert.Equal(t, 5, segQueueSize(t, db))

		// The survivors must be the 5 NEWEST rows (ORDER BY queued_at ASC).
		rows, err := db.QueryContext(ctx, `SELECT email FROM contact_segment_queue ORDER BY email`)
		require.NoError(t, err)
		defer rows.Close()
		var remaining []string
		for rows.Next() {
			var e string
			require.NoError(t, rows.Scan(&e))
			remaining = append(remaining, e)
		}
		require.NoError(t, rows.Err())
		expected := make([]string, 0, 5)
		for i := 100; i <= 104; i++ {
			expected = append(expected, fmt.Sprintf("bulk-%s-%03d@example.com", tag, i))
		}
		assert.Equal(t, expected, remaining, "claim must take the oldest rows first")

		// Second pass drains the remainder.
		count, err = processor.ProcessQueue(ctx, ws.ID)
		require.NoError(t, err)
		assert.Equal(t, 5, count)
		assert.Equal(t, 0, segQueueSize(t, db))
	})

	t.Run("debounce excludes fresh rows", func(t *testing.T) {
		ws, err := factory.CreateWorkspace()
		require.NoError(t, err)
		db, err := suite.DBManager.GetWorkspaceDB(ws.ID)
		require.NoError(t, err)

		tag := uuid.New().String()[:8]
		email := fmt.Sprintf("debounce-%s@example.com", tag)

		// 10s old: inside the 15s debounce window -> must not be claimed.
		_, err = db.ExecContext(ctx, `
			INSERT INTO contact_segment_queue (email, queued_at)
			VALUES ($1, NOW() - INTERVAL '10 seconds')
			ON CONFLICT (email) DO UPDATE SET queued_at = EXCLUDED.queued_at`, email)
		require.NoError(t, err)

		count, err := processor.ProcessQueue(ctx, ws.ID)
		require.NoError(t, err)
		assert.Equal(t, 0, count, "a 10s-old row is inside the debounce window")
		assert.Equal(t, 1, queueRowsFor(t, db, email))

		// Aged past the window -> claimed.
		_, err = db.ExecContext(ctx, `
			UPDATE contact_segment_queue SET queued_at = NOW() - INTERVAL '16 seconds'
			WHERE email = $1`, email)
		require.NoError(t, err)

		count, err = processor.ProcessQueue(ctx, ws.ID)
		require.NoError(t, err)
		assert.Equal(t, 1, count, "a 16s-old row is past the debounce window")
		assert.Equal(t, 0, queueRowsFor(t, db, email))
	})
}

// -----------------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------------

// createSegmentWithSQL inserts an active segment whose compiled query is exactly
// generatedSQL/args — the fields ProcessQueue evaluates — mirroring the column
// set of testutil's CreateSegment but with a caller-chosen compiled query.
func createSegmentWithSQL(t *testing.T, db *sql.DB, id, generatedSQL string, args domain.JSONArray, status string) string {
	t.Helper()

	tree := &domain.TreeNode{
		Kind: "leaf",
		Leaf: &domain.TreeNodeLeaf{
			Source: "contacts",
			Contact: &domain.ContactCondition{
				Filters: []*domain.DimensionFilter{
					{
						FieldName:    "email",
						FieldType:    "string",
						Operator:     "is_set",
						StringValues: []string{},
					},
				},
			},
		},
	}
	treeMap, err := tree.ToMapOfAny()
	require.NoError(t, err)
	treeJSON, err := json.Marshal(treeMap)
	require.NoError(t, err)

	_, err = db.ExecContext(context.Background(), `
		INSERT INTO segments (
			id, name, color, tree, timezone, version, status,
			generated_sql, generated_args, recompute_after, db_created_at, db_updated_at
		) VALUES ($1, $2, '#FF5733', $3, 'UTC', 1, $4, $5, $6, NULL, NOW(), NOW())`,
		id, "Segment "+id, treeJSON, status, generatedSQL, mustJSON(t, args))
	require.NoError(t, err)
	return id
}

// enqueueAged upserts a contact_segment_queue row aged past the 15s claim
// debounce, so the next ProcessQueue pass is guaranteed to see it.
func enqueueAged(t *testing.T, db *sql.DB, email string) {
	t.Helper()
	_, err := db.ExecContext(context.Background(), `
		INSERT INTO contact_segment_queue (email, queued_at)
		VALUES ($1, NOW() - INTERVAL '1 minute')
		ON CONFLICT (email) DO UPDATE SET queued_at = EXCLUDED.queued_at`, email)
	require.NoError(t, err)
}

// segQueueSize counts all contact_segment_queue rows, debounced or not.
func segQueueSize(t *testing.T, db *sql.DB) int {
	t.Helper()
	var n int
	require.NoError(t, db.QueryRowContext(context.Background(),
		`SELECT count(*) FROM contact_segment_queue`).Scan(&n))
	return n
}

// memberCount counts contact_segments rows for one (email, segment) pair.
func memberCount(t *testing.T, db *sql.DB, email, segmentID string) int {
	t.Helper()
	var n int
	require.NoError(t, db.QueryRowContext(context.Background(),
		`SELECT count(*) FROM contact_segments WHERE email = $1 AND segment_id = $2`,
		email, segmentID).Scan(&n))
	return n
}

// timelineKindCount counts contact_timeline rows for (email, kind, entity_id).
func timelineKindCount(t *testing.T, db *sql.DB, email, kind, entityID string) int {
	t.Helper()
	var n int
	require.NoError(t, db.QueryRowContext(context.Background(),
		`SELECT count(*) FROM contact_timeline WHERE email = $1 AND kind = $2 AND entity_id = $3`,
		email, kind, entityID).Scan(&n))
	return n
}

// mustJSON marshals v for a JSONB column.
func mustJSON(t *testing.T, v interface{}) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}
