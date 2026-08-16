//go:build integration
// +build integration

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/database/schema"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/repository"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// TestWebAnalyticsUsageMetering is the acceptance gate for the usage meter.
//
// The billable unit is one row in web_pages — one page a visitor opened, which
// is what the pricing page publishes ("counts once per page, not once per
// heartbeat"). Every claim that makes that true depends on how PostgreSQL
// actually behaves under the real primary key, real tuple routing into monthly
// partitions, and the real beat_seq guard: sqlmock returns whatever a test
// declares and can prove none of it. So the meter is only ever really tested
// here.
func TestWebAnalyticsUsageMetering(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	workspace, err := suite.DataFactory.CreateWorkspace(func(w *domain.Workspace) {
		w.Settings.WebAnalytics = &domain.WebAnalyticsSettings{Enabled: true}
	})
	require.NoError(t, err)

	wsDB, err := suite.DBManager.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	workspaceRepo := suite.ServerManager.GetApp().GetWorkspaceRepository()
	webRepo := repository.NewWebAnalyticsRepository(workspaceRepo, logger.NewLogger())
	ctx := context.Background()

	// A fixed month well away from "now", so the test never straddles a real
	// month boundary and never collides with rows another test left behind.
	month := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	sessionDate := time.Date(2026, 3, 14, 0, 0, 0, 0, time.UTC)
	require.NoError(t, webRepo.EnsureMonthlyPartitions(ctx, workspace.ID, []time.Time{month}))

	// UUIDv7-shaped ids: the column is UUID, and sessions are keyed by one.
	sessionID := func(n int) string {
		return fmt.Sprintf("019614a0-0000-7000-8000-%012d", n)
	}

	page := func(session string, tab int64, number int, beat int64, path string) *domain.WebPage {
		return &domain.WebPage{
			SessionDate: sessionDate,
			SessionID:   session,
			TabID:       tab,
			PageNumber:  number,
			BeatSeq:     beat,
			Path:        path,
			EnteredAt:   sessionDate.Add(10 * time.Hour),
			ExitedAt:    sessionDate.Add(10*time.Hour + time.Minute),
			DurationMs:  60000,
			EntryType:   "navigate",
		}
	}

	meteredPageviews := func(t *testing.T, m time.Time) int64 {
		t.Helper()
		usage, err := webRepo.GetUsage(ctx, workspace.ID, []time.Time{m})
		require.NoError(t, err)
		if len(usage) == 0 {
			return -1
		}
		return usage[0].Pageviews
	}

	rawPageCount := func(t *testing.T) int64 {
		t.Helper()
		var n int64
		require.NoError(t, wsDB.QueryRow(
			`SELECT COUNT(*) FROM web_pages WHERE session_date >= $1::date AND session_date < $2::date`,
			month, month.AddDate(0, 1, 0)).Scan(&n))
		return n
	}

	t.Run("one row per page opened, not per heartbeat", func(t *testing.T) {
		// Five pages of one visit, then forty more heartbeats replaying the whole
		// cumulative action list — which is exactly what the SDK ships on every
		// beat. Counting requests would overcount ~8x here, counting the actions
		// in each beat ~40x.
		var pages []*domain.WebPage
		for n := 1; n <= 5; n++ {
			pages = append(pages, page(sessionID(1), 1, n, 1, fmt.Sprintf("/p%d", n)))
		}
		require.NoError(t, webRepo.FlushBatch(ctx, workspace.ID, nil, pages, nil))

		for beat := int64(2); beat <= 41; beat++ {
			var replay []*domain.WebPage
			for n := 1; n <= 5; n++ {
				replay = append(replay, page(sessionID(1), 1, n, beat, fmt.Sprintf("/p%d", n)))
			}
			require.NoError(t, webRepo.FlushBatch(ctx, workspace.ID, nil, replay, nil))
		}

		require.NoError(t, webRepo.RecomputeUsage(ctx, workspace.ID, month, true))
		assert.Equal(t, int64(5), meteredPageviews(t, month), "40 heartbeats over 5 pages is 5 pageviews")
		assert.Equal(t, int64(5), rawPageCount(t))
	})

	t.Run("a stale beat is not a new pageview", func(t *testing.T) {
		before := meteredPageviews(t, month)

		// beat_seq below the stored one: the upsert guard drops the update, and
		// the row count cannot move either way.
		require.NoError(t, webRepo.FlushBatch(ctx, workspace.ID, nil,
			[]*domain.WebPage{page(sessionID(1), 1, 3, 1, "/stale")}, nil))

		require.NoError(t, webRepo.RecomputeUsage(ctx, workspace.ID, month, true))
		assert.Equal(t, before, meteredPageviews(t, month))

		var path string
		require.NoError(t, wsDB.QueryRow(
			`SELECT path FROM web_pages WHERE session_id = $1 AND tab_id = 1 AND page_number = 3`,
			sessionID(1)).Scan(&path))
		assert.Equal(t, "/p3", path, "the stale beat must not have overwritten the row either")
	})

	t.Run("a second tab of the same session counts separately", func(t *testing.T) {
		before := meteredPageviews(t, month)

		// Same session, same page numbers, different tab: two real pages open.
		// tab_id is in the primary key precisely so these do not collapse.
		require.NoError(t, webRepo.FlushBatch(ctx, workspace.ID, nil, []*domain.WebPage{
			page(sessionID(1), 2, 1, 1, "/p1"),
			page(sessionID(1), 2, 2, 1, "/p2"),
		}, nil))

		require.NoError(t, webRepo.RecomputeUsage(ctx, workspace.ID, month, true))
		assert.Equal(t, before+2, meteredPageviews(t, month))
	})

	t.Run("duplicate rows inside one batch collapse to one pageview", func(t *testing.T) {
		before := meteredPageviews(t, month)

		dup := page(sessionID(2), 1, 1, 1, "/dup")
		require.NoError(t, webRepo.FlushBatch(ctx, workspace.ID, nil,
			[]*domain.WebPage{dup, dup, dup}, nil))

		require.NoError(t, webRepo.RecomputeUsage(ctx, workspace.ID, month, true))
		assert.Equal(t, before+1, meteredPageviews(t, month))
	})

	t.Run("months are metered separately", func(t *testing.T) {
		nextMonth := month.AddDate(0, 1, 0)
		require.NoError(t, webRepo.EnsureMonthlyPartitions(ctx, workspace.ID, []time.Time{nextMonth}))

		aprilPage := page(sessionID(3), 1, 1, 1, "/april")
		aprilPage.SessionDate = time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC)
		require.NoError(t, webRepo.FlushBatch(ctx, workspace.ID, nil, []*domain.WebPage{aprilPage}, nil))

		marchBefore := meteredPageviews(t, month)
		require.NoError(t, webRepo.RecomputeUsage(ctx, workspace.ID, nextMonth, true))
		require.NoError(t, webRepo.RecomputeUsage(ctx, workspace.ID, month, true))

		assert.Equal(t, int64(1), meteredPageviews(t, nextMonth))
		assert.Equal(t, marchBefore, meteredPageviews(t, month), "April traffic must not land in March")

		usage, err := webRepo.GetUsage(ctx, workspace.ID, []time.Time{month, nextMonth})
		require.NoError(t, err)
		require.Len(t, usage, 2)
		assert.Equal(t, month, usage[0].PeriodMonth)
		assert.Equal(t, nextMonth, usage[1].PeriodMonth)
	})

	t.Run("a month never metered is absent rather than zero", func(t *testing.T) {
		// The distinction matters for a quota: "no data reported" is not "used
		// nothing", and the control plane must never act on the difference.
		usage, err := webRepo.GetUsage(ctx, workspace.ID, []time.Time{time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)})
		require.NoError(t, err)
		assert.Empty(t, usage)
	})

	t.Run("dropping history cannot rewrite a closed month", func(t *testing.T) {
		billed := meteredPageviews(t, month)
		require.Greater(t, billed, int64(0))

		// Retention is the operator's call and drops whole monthly partitions.
		// Recounting after that returns 0, and the already-reported value has to
		// survive it — this is the entire reason a closed month is written with
		// GREATEST instead of an overwrite.
		_, err := wsDB.Exec("DROP TABLE " + schema.WebAnalyticsPartitionName("web_pages", month))
		require.NoError(t, err)
		require.Equal(t, int64(0), rawPageCount(t))

		require.NoError(t, webRepo.RecomputeUsage(ctx, workspace.ID, month, false))
		assert.Equal(t, billed, meteredPageviews(t, month), "a closed month must never be lowered")

		// The open month is deliberately free to fall, so the same recount as
		// live does rewrite it. Asserted here so the asymmetry is deliberate and
		// visible rather than an accident of which branch was taken.
		require.NoError(t, webRepo.RecomputeUsage(ctx, workspace.ID, month, true))
		assert.Equal(t, int64(0), meteredPageviews(t, month))
	})
}

// TestWebAnalyticsUsageTimelineMeter pins the exclusion that makes the published
// "the same event is never metered twice" true: the rows the web analytics
// timeline projection writes are already counted as pageviews, so they must not
// also count as timeline entries.
func TestWebAnalyticsUsageTimelineMeter(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	workspace, err := suite.DataFactory.CreateWorkspace(func(w *domain.Workspace) {
		w.Settings.WebAnalytics = &domain.WebAnalyticsSettings{Enabled: true}
	})
	require.NoError(t, err)

	wsDB, err := suite.DBManager.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	workspaceRepo := suite.ServerManager.GetApp().GetWorkspaceRepository()
	webRepo := repository.NewWebAnalyticsRepository(workspaceRepo, logger.NewLogger())
	ctx := context.Background()

	month := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	at := time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)

	// Contact rows write their own timeline entries by trigger, so insert
	// directly to keep the fixture to exactly what is being counted.
	insertTimeline := func(entityType, kind string) {
		t.Helper()
		_, err := wsDB.Exec(`
			INSERT INTO contact_timeline (email, operation, entity_type, kind, created_at)
			VALUES ($1, 'insert', $2, $3, $4)`,
			"visitor@example.com", entityType, kind, at)
		require.NoError(t, err)
	}

	// Billable: things that are not web pageviews or web sessions.
	insertTimeline("contact", "contact.created")
	insertTimeline("message_history", "message.sent")
	insertTimeline("custom_event", "custom_event.signup")

	// Not billable: written by web_analytics_timeline_projection.go for rows the
	// pageview meter has already counted.
	insertTimeline("web_page", "web.pageview")
	insertTimeline("web_page", "web.pageview")
	insertTimeline("web_session", "web.session")

	// A row outside the month must not leak in.
	_, err = wsDB.Exec(`
		INSERT INTO contact_timeline (email, operation, entity_type, kind, created_at)
		VALUES ('visitor@example.com', 'insert', 'contact', 'contact.created', $1)`,
		month.AddDate(0, 1, 0))
	require.NoError(t, err)

	require.NoError(t, webRepo.EnsureMonthlyPartitions(ctx, workspace.ID, []time.Time{month}))
	require.NoError(t, webRepo.RecomputeUsage(ctx, workspace.ID, month, true))

	usage, err := webRepo.GetUsage(ctx, workspace.ID, []time.Time{month})
	require.NoError(t, err)
	require.Len(t, usage, 1)
	assert.Equal(t, int64(3), usage[0].TimelineEntries, "the three web-derived rows must not be metered")
	assert.Equal(t, int64(0), usage[0].Pageviews)
}
