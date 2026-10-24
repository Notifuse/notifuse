package service

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// The seeded journeys stay inert through one load-bearing detail —
// scheduled_at is NULL — and stay believable through the timeline/list side
// effects matching what the nodes would have done. Pin both.

var demoWriteBase = time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)

func demoTestJourney(email string, status domain.ContactAutomationStatus, visits []demoNodeVisit) demoJourney {
	return demoJourney{
		ID: "j-" + email, AutomationID: demoAutomationPostPurchase, Email: email,
		CurrentNodeID: "pp-email-thanks", Status: status,
		EnteredAt: demoWriteBase, EndedAt: demoWriteBase.Add(time.Hour),
		Visits: visits, TimelineReason: "completed",
	}
}

func TestDemoContactAutomationArgs_Inertness(t *testing.T) {
	j := demoTestJourney("a@example.com", domain.ContactAutomationStatusCompleted, nil)
	args := demoContactAutomationArgs(j)
	require.Len(t, args, demoContactAutomationColumns)
	assert.Equal(t, "j-a@example.com", args[0])
	assert.Equal(t, string(domain.ContactAutomationStatusCompleted), args[4])
	// scheduled_at: NULL is the inertness guarantee. A timestamp <= now would
	// let a future Activate claim the row; any timestamp at all satisfies
	// nothing — only NULL can never pass `scheduled_at <= now`.
	assert.Nil(t, args[7], "seeded journeys must carry scheduled_at NULL")
	assert.Nil(t, demoScheduledAtArg(j))
	assert.Equal(t, []byte(`{}`), args[8])
}

func TestExecDemoBatches(t *testing.T) {
	ctx := context.Background()
	argsFor := func(i int) []interface{} { return []interface{}{i, i * 10} }

	t.Run("zero rows issue no statement", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		require.NoError(t, execDemoBatches(ctx, db, 0, 2, 2, "INSERT INTO t VALUES ", argsFor))
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("placeholders are numbered continuously across a batch", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		mock.ExpectExec(regexp.QuoteMeta("INSERT INTO t VALUES ($1, $2), ($3, $4)")).
			WithArgs(0, 0, 1, 10).
			WillReturnResult(sqlmock.NewResult(0, 2))
		require.NoError(t, execDemoBatches(ctx, db, 2, 2, 2, "INSERT INTO t VALUES ", argsFor))
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("a partial final batch and the suffix land in one statement each", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		mock.ExpectExec(regexp.QuoteMeta("($1, $2), ($3, $4) ON CONFLICT DO NOTHING")).
			WillReturnResult(sqlmock.NewResult(0, 2))
		mock.ExpectExec(regexp.QuoteMeta("($1, $2) ON CONFLICT DO NOTHING")).
			WillReturnResult(sqlmock.NewResult(0, 1))
		require.NoError(t, execDemoBatches(ctx, db, 3, 2, 2,
			"INSERT INTO t VALUES ", argsFor, " ON CONFLICT DO NOTHING"))
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("an insert failure wraps and stops later batches", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		sentinel := errors.New("too many parameters")
		mock.ExpectExec("INSERT INTO t").WillReturnError(sentinel)
		err = execDemoBatches(ctx, db, 5, 2, 2, "INSERT INTO t VALUES ", argsFor)
		require.ErrorContains(t, err, "failed to insert demo automation rows:")
		assert.ErrorIs(t, err, sentinel)
		require.NoError(t, mock.ExpectationsWereMet(), "no second batch may run after a failure")
	})
}

func TestWriteDemoJourneys_Transaction(t *testing.T) {
	ctx := context.Background()
	svc := &DemoService{logger: logger.NewLoggerWithLevel("disabled")}
	automation := &domain.Automation{ID: demoAutomationWelcome}
	journeys := []demoJourney{demoTestJourney("a@example.com", domain.ContactAutomationStatusCompleted, []demoNodeVisit{
		{NodeID: "ws-trigger", NodeType: domain.NodeTypeTrigger, Action: domain.NodeActionEntered, EnteredAt: demoWriteBase},
		{NodeID: "ws-trigger", NodeType: domain.NodeTypeTrigger, Action: domain.NodeActionCompleted,
			EnteredAt: demoWriteBase, CompletedAt: demoWriteBase.Add(time.Second), DurationMs: 1000},
	})}

	t.Run("enrollments, executions and stats land in one transaction", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta("INSERT INTO contact_automations")).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(regexp.QuoteMeta("INSERT INTO automation_node_executions")).
			WillReturnResult(sqlmock.NewResult(0, 2))
		// The welcome flow comes to rest paused; the status rides along with
		// the stats so the badge and the numbers commit together.
		mock.ExpectExec(regexp.QuoteMeta("UPDATE automations SET stats")).
			WithArgs(sqlmock.AnyArg(), string(domain.AutomationStatusPaused), demoAutomationWelcome).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()

		require.NoError(t, svc.writeDemoJourneys(ctx, db, automation, journeys))
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("an insert failure rolls the whole journey set back", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		sentinel := errors.New("unique violation")
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta("INSERT INTO contact_automations")).WillReturnError(sentinel)
		mock.ExpectRollback()

		err = svc.writeDemoJourneys(ctx, db, automation, journeys)
		assert.ErrorIs(t, err, sentinel)
		require.NoError(t, mock.ExpectationsWereMet(), "stats must never land without their rows")
	})
}

func TestDemoJourneyTimelineEntries_Ordering(t *testing.T) {
	automation := &domain.Automation{ID: demoAutomationWelcome, RootNodeID: "ws-trigger"}
	later := demoTestJourney("b@example.com", domain.ContactAutomationStatusCompleted, nil)
	later.AutomationID = demoAutomationWelcome
	earlier := demoTestJourney("a@example.com", domain.ContactAutomationStatusExited, nil)
	earlier.AutomationID = demoAutomationWelcome
	earlier.TimelineReason = "unsubscribed"

	entries, err := demoJourneyTimelineEntries(automation, []demoJourney{later, earlier})
	require.NoError(t, err)
	require.Len(t, entries, 4, "one start and one end row per journey")

	// Sorted by email so segment-queue locks are taken in a consistent order.
	assert.Equal(t, "a@example.com", entries[0].Email)
	assert.Equal(t, "automation.start", entries[0].Kind)
	assert.Equal(t, "insert", entries[0].Operation)
	assert.Equal(t, earlier.EnteredAt, entries[0].CreatedAt,
		"timeline rows carry the journey's own time, not the reset time")
	assert.Equal(t, "automation.end", entries[1].Kind)
	assert.Equal(t, "update", entries[1].Operation)
	assert.Equal(t, earlier.EndedAt, entries[1].CreatedAt)
	assert.Equal(t, "b@example.com", entries[2].Email)

	var endChanges map[string]map[string]interface{}
	require.NoError(t, json.Unmarshal(entries[1].Changes, &endChanges))
	assert.Equal(t, "unsubscribed", endChanges["exit_reason"]["new"])
	assert.Equal(t, string(domain.ContactAutomationStatusExited), endChanges["status"]["new"])
	assert.Equal(t, demoAutomationWelcome, endChanges["automation_id"]["new"])
}

func TestDemoJourneyListChanges_Filtering(t *testing.T) {
	completedAt := func(nodeID string) demoNodeVisit {
		return demoNodeVisit{NodeID: nodeID, Action: domain.NodeActionCompleted}
	}
	journeys := []demoJourney{
		demoTestJourney("vip@example.com", domain.ContactAutomationStatusCompleted, []demoNodeVisit{completedAt("pp-add-vip")}),
		demoTestJourney("gone@example.com", domain.ContactAutomationStatusCompleted, []demoNodeVisit{completedAt("wb-remove")}),
		// Reached the removal node but failed there: nothing was removed.
		demoTestJourney("stayed@example.com", domain.ContactAutomationStatusFailed, []demoNodeVisit{
			{NodeID: "wb-remove", Action: domain.NodeActionFailed},
		}),
		demoTestJourney("aaa@example.com", domain.ContactAutomationStatusCompleted, []demoNodeVisit{completedAt("vc-add-vip")}),
	}

	adds, removals := demoJourneyListChanges(journeys)
	require.Len(t, adds, 2)
	assert.Equal(t, "aaa@example.com", adds[0].Email, "adds are email-ordered for lock ordering")
	assert.Equal(t, "vip@example.com", adds[1].Email)
	require.Len(t, removals, 1)
	assert.Equal(t, "gone@example.com", removals[0].Email)
}

func TestWriteDemoJourneyTimelineAndListChanges(t *testing.T) {
	ctx := context.Background()
	svc := &DemoService{logger: logger.NewLoggerWithLevel("disabled")}
	automation := &domain.Automation{ID: demoAutomationWelcome}
	journeys := []demoJourney{
		demoTestJourney("a@example.com", domain.ContactAutomationStatusCompleted, []demoNodeVisit{
			{NodeID: "pp-add-vip", Action: domain.NodeActionCompleted},
		}),
	}

	t.Run("timeline entries insert in one batched statement", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		// 2 entries x 7 columns in a single INSERT.
		mock.ExpectExec(regexp.QuoteMeta("INSERT INTO contact_timeline")).
			WithArgs(
				"a@example.com", "insert", "automation", "automation.start", demoAutomationWelcome, sqlmock.AnyArg(), sqlmock.AnyArg(),
				"a@example.com", "update", "automation", "automation.end", demoAutomationWelcome, sqlmock.AnyArg(), sqlmock.AnyArg(),
			).
			WillReturnResult(sqlmock.NewResult(0, 2))

		require.NoError(t, svc.writeDemoJourneyTimeline(ctx, db, automation, journeys))
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("list changes add VIP members and remove winback leavers", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		mock.ExpectExec(regexp.QuoteMeta("INSERT INTO contact_lists")).
			WithArgs("a@example.com", demoListVIPClub, "active", sqlmock.AnyArg(), sqlmock.AnyArg()).
			WillReturnResult(sqlmock.NewResult(0, 1))

		require.NoError(t, svc.writeDemoJourneyListChanges(ctx, db, journeys))
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("no journeys means no statements", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		require.NoError(t, svc.writeDemoJourneyTimeline(ctx, db, automation, nil))
		require.NoError(t, svc.writeDemoJourneyListChanges(ctx, db, nil))
		require.NoError(t, mock.ExpectationsWereMet())
	})
}
