//go:build integration
// +build integration

package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/repository"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// TestCustomEventBatchInsertNewIsImmutable pins down the ON CONFLICT DO NOTHING
// clause of BatchInsertNew, the durable half of the web-analytics bridge's
// exactly-once guarantee.
//
// The bridge's in-memory cursor covers the common case, but it dies with the
// process: a restart, a buffer eviction or a second replica all replay goals the
// database has already stored. DO NOTHING makes that replay free
// unconditionally, whatever the second copy carries.
//
// Swapping it for the mutable DO UPDATE that BatchUpsert uses would not, on its
// own, duplicate an ordinary replay: that clause only fires when the incoming
// occurred_at is strictly newer or its deleted_at differs, and the bridge
// freezes occurred_at at the client timestamp already baked into the external
// id, so a second copy of the same goal compares equal. The real cost is that
// exactly-once would stop being a property of the write and become contingent on
// invariants owned elsewhere — and the deleted_at one is broken by design: the
// bridge has no idea an admin removed an event, so it keeps sending deleted_at
// NULL, which IS DISTINCT FROM the deletion and resurrects the row on the next
// beat. The occurred_at one is a line of service code away from breaking too,
// since the skew-corrected timestamp the bridge deliberately does not use is
// recomputed per beat and would look newer every time. Either way the timeline
// trigger, which fires on INSERT *or* UPDATE and does no diffing, appends
// another contact_timeline row and with it a segment recomputation and an
// automation re-enrolment.
//
// So each case here re-inserts a key that already exists with DIFFERENT payload
// — including an occurred_at deliberately newer than the first, the only shape
// that tells the two clauses apart — and asserts the stored row still holds the
// FIRST values, plus that the timeline row count did not move: the read-back
// catches the overwrite, the count catches what the overwrite costs downstream.
func TestCustomEventBatchInsertNewIsImmutable(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	workspace, err := suite.DataFactory.CreateWorkspace()
	require.NoError(t, err)

	contact, err := suite.DataFactory.CreateContact(workspace.ID, func(c *domain.Contact) {
		c.Email = "buyer@example.com"
	})
	require.NoError(t, err)

	wsDB, err := suite.DBManager.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	// The repository is built directly rather than driven through ingest: this is
	// about the write itself, so the bridge's own dedup cursor must be out of the
	// picture — it would hide a mutable clause by never issuing the second write.
	repo := repository.NewCustomEventRepository(suite.ServerManager.GetApp().GetWorkspaceRepository())
	ctx := context.Background()

	const eventName = "web.purchase"
	// Postgres stores timestamptz at microsecond resolution; truncating keeps the
	// round-trip comparison exact instead of tolerance-based.
	firstOccurred := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Microsecond)
	// Later than firstOccurred on purpose: DO UPDATE's guard is
	// "EXCLUDED.occurred_at > custom_events.occurred_at", so a replay carrying an
	// older timestamp would be left alone by both clauses and prove nothing.
	replayOccurred := firstOccurred.Add(time.Hour)

	event := func(externalID string, occurredAt time.Time, value float64, source string, props map[string]interface{}) *domain.CustomEvent {
		goalName, goalType := "purchase", domain.GoalTypePurchase
		return &domain.CustomEvent{
			EventName:  eventName,
			ExternalID: externalID,
			Email:      contact.Email,
			Properties: props,
			OccurredAt: occurredAt,
			Source:     source,
			GoalName:   &goalName,
			GoalType:   &goalType,
			GoalValue:  &value,
		}
	}

	type storedEvent struct {
		properties map[string]interface{}
		occurredAt time.Time
		goalValue  *float64
		source     string
		deletedAt  *time.Time
		updatedAt  time.Time
	}

	// Read straight from the table rather than through GetByID, which filters out
	// soft-deleted rows — the resurrection case needs to see them.
	read := func(t *testing.T, externalID string) storedEvent {
		t.Helper()
		var raw []byte
		var goalValue sql.NullFloat64
		var deletedAt sql.NullTime
		got := storedEvent{}
		require.NoError(t, wsDB.QueryRow(`
			SELECT properties, occurred_at, goal_value, source, deleted_at, updated_at
			FROM custom_events WHERE event_name = $1 AND external_id = $2`,
			eventName, externalID).Scan(&raw, &got.occurredAt, &goalValue, &got.source, &deletedAt, &got.updatedAt))
		require.NoError(t, json.Unmarshal(raw, &got.properties))
		if goalValue.Valid {
			got.goalValue = &goalValue.Float64
		}
		if deletedAt.Valid {
			got.deletedAt = &deletedAt.Time
		}
		return got
	}

	timelineRows := func(t *testing.T, externalID string) int {
		t.Helper()
		var count int
		require.NoError(t, wsDB.QueryRow(
			`SELECT count(*) FROM contact_timeline WHERE entity_id = $1 AND kind = $2`,
			externalID, "custom_event."+eventName).Scan(&count))
		return count
	}

	eventRows := func(t *testing.T, externalID string) int {
		t.Helper()
		var count int
		require.NoError(t, wsDB.QueryRow(
			`SELECT count(*) FROM custom_events WHERE event_name = $1 AND external_id = $2`,
			eventName, externalID).Scan(&count))
		return count
	}

	t.Run("re-inserting an existing key keeps the first row and fires no second timeline entry", func(t *testing.T) {
		require.NoError(t, repo.BatchInsertNew(ctx, workspace.ID, []*domain.CustomEvent{
			event("order_1", firstOccurred, 49.90, "web_analytics", map[string]interface{}{"plan": "pro"}),
		}))
		original := read(t, "order_1")
		require.Equal(t, 1, timelineRows(t, "order_1"), "precondition: the first write lands once")

		// Every field the mutable clause would overwrite differs here.
		require.NoError(t, repo.BatchInsertNew(ctx, workspace.ID, []*domain.CustomEvent{
			event("order_1", replayOccurred, 999.99, "api", map[string]interface{}{"plan": "enterprise"}),
		}))

		got := read(t, "order_1")
		assert.Equal(t, map[string]interface{}{"plan": "pro"}, got.properties,
			"properties must stay as first written")
		assert.True(t, firstOccurred.Equal(got.occurredAt),
			"occurred_at must stay at %s, got %s", firstOccurred, got.occurredAt)
		require.NotNil(t, got.goalValue)
		assert.InDelta(t, 49.90, *got.goalValue, 0.001, "goal_value must stay as first written")
		assert.Equal(t, "web_analytics", got.source)
		assert.True(t, original.updatedAt.Equal(got.updatedAt),
			"updated_at moving means the row was rewritten, not skipped")

		assert.Equal(t, 1, eventRows(t, "order_1"))
		assert.Equal(t, 1, timelineRows(t, "order_1"),
			"a replayed goal must not append a second timeline row, segment recompute and automation enrolment")
	})

	t.Run("a soft-deleted event is not resurrected by a replay", func(t *testing.T) {
		require.NoError(t, repo.BatchInsertNew(ctx, workspace.ID, []*domain.CustomEvent{
			event("order_2", firstOccurred, 10.00, "web_analytics", map[string]interface{}{"plan": "pro"}),
		}))

		_, err := wsDB.Exec(
			`UPDATE custom_events SET deleted_at = NOW() WHERE event_name = $1 AND external_id = $2`,
			eventName, "order_2")
		require.NoError(t, err)
		deleted := read(t, "order_2")
		require.NotNil(t, deleted.deletedAt, "precondition: the admin's deletion landed")
		// The manual UPDATE fired the timeline trigger itself, so the baseline is
		// taken after it rather than assumed to be 1.
		baseline := timelineRows(t, "order_2")

		// The bridge always sends deleted_at NULL: it has no idea an admin removed
		// the event, so DO UPDATE would silently bring it back.
		require.NoError(t, repo.BatchInsertNew(ctx, workspace.ID, []*domain.CustomEvent{
			event("order_2", replayOccurred, 10.00, "web_analytics", map[string]interface{}{"plan": "pro"}),
		}))

		got := read(t, "order_2")
		require.NotNil(t, got.deletedAt, "a deliberately removed event must stay removed")
		assert.True(t, deleted.deletedAt.Equal(*got.deletedAt))
		assert.Equal(t, baseline, timelineRows(t, "order_2"),
			"resurrecting the row would also replay it onto the timeline")
	})

	t.Run("a conflict does not abort the rest of the batch", func(t *testing.T) {
		// The realistic replay shape: a source re-sends its whole history, so most
		// of the batch is already stored and only the tail is new.
		require.NoError(t, repo.BatchInsertNew(ctx, workspace.ID, []*domain.CustomEvent{
			event("order_1", replayOccurred, 999.99, "api", map[string]interface{}{"plan": "enterprise"}),
			event("order_3", replayOccurred, 25.50, "web_analytics", map[string]interface{}{"plan": "starter"}),
		}))

		fresh := read(t, "order_3")
		assert.Equal(t, map[string]interface{}{"plan": "starter"}, fresh.properties)
		require.NotNil(t, fresh.goalValue)
		assert.InDelta(t, 25.50, *fresh.goalValue, 0.001)
		assert.Equal(t, 1, timelineRows(t, "order_3"), "the new event still reaches the timeline")

		existing := read(t, "order_1")
		assert.Equal(t, map[string]interface{}{"plan": "pro"}, existing.properties)
		assert.True(t, firstOccurred.Equal(existing.occurredAt))
		assert.Equal(t, 1, timelineRows(t, "order_1"), "the conflicting key stays untouched")
	})

	t.Run("an empty batch is a no-op", func(t *testing.T) {
		// The bridge flushes on a timer, so most beats have nothing new to write.
		assert.NoError(t, repo.BatchInsertNew(ctx, workspace.ID, nil))
		assert.NoError(t, repo.BatchInsertNew(ctx, workspace.ID, []*domain.CustomEvent{}))

		var events, timeline int
		require.NoError(t, wsDB.QueryRow(
			`SELECT count(*) FROM custom_events WHERE event_name = $1`, eventName).Scan(&events))
		require.NoError(t, wsDB.QueryRow(
			`SELECT count(*) FROM contact_timeline WHERE kind = $1`, "custom_event."+eventName).Scan(&timeline))
		assert.Equal(t, 3, events)
		assert.Equal(t, 4, timeline, "3 inserts plus the one the admin's soft delete appended")
	})
}
