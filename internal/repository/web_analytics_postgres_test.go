package repository

import (
	"context"
	"database/sql/driver"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/golang/mock/gomock"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/database/schema"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/pkg/logger"
)

const waTestWorkspace = "ws-web"

func newWebAnalyticsRepoForTest(t *testing.T) (domain.WebAnalyticsRepository, sqlmock.Sqlmock, func()) {
	t.Helper()
	ctrl := gomock.NewController(t)
	db, mock, err := sqlmock.New()
	require.NoError(t, err)

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockWorkspaceRepo.EXPECT().
		GetConnection(gomock.Any(), waTestWorkspace).
		Return(db, nil).
		AnyTimes()

	repo := NewWebAnalyticsRepository(mockWorkspaceRepo, logger.NewLogger())
	cleanup := func() {
		_ = db.Close()
		ctrl.Finish()
	}
	return repo, mock, cleanup
}

func waTestSession(id string, date time.Time) *domain.WebSession {
	return &domain.WebSession{
		SessionDate: date,
		ID:          id,
		BeatSeq:     2,
		CreatedAt:   date.Add(10 * time.Hour),
		UpdatedAt:   date.Add(10*time.Hour + time.Minute),
		Channel:     "google-ads",
	}
}

func TestUpsertSuffix(t *testing.T) {
	t.Run("sessions: guard, conflict target, created_at never updated, sticky contact_email", func(t *testing.T) {
		suffix := webSessionUpsertSuffix
		assert.True(t, strings.HasPrefix(suffix, "ON CONFLICT (session_date, id) DO UPDATE SET "))
		assert.Contains(t, suffix, "WHERE EXCLUDED.beat_seq > web_sessions.beat_seq")
		assert.NotContains(t, suffix, "created_at = EXCLUDED.created_at")
		assert.NotContains(t, suffix, "id = EXCLUDED.id")
		assert.NotContains(t, suffix, "session_date = EXCLUDED.session_date")
		assert.Contains(t, suffix, "updated_at = EXCLUDED.updated_at")
		assert.Contains(t, suffix, "duration_ms = EXCLUDED.duration_ms")
		assert.Contains(t, suffix, "contact_email = COALESCE(EXCLUDED.contact_email, web_sessions.contact_email)")
		assert.NotContains(t, suffix, "contact_email = EXCLUDED.contact_email")
	})

	t.Run("sessions: created_at is set once and never rewritten by later beats", func(t *testing.T) {
		// The session start time is the anchor for reporting and for the
		// uuid-derived partition; letting a later beat rewrite it would make
		// a session's start silently drift forward for its whole lifetime.
		suffix := webSessionUpsertSuffix
		assert.NotContains(t, suffix, "created_at",
			"created_at must not appear anywhere in the DO UPDATE SET assignments")
		// updated_at, by contrast, must advance on every beat.
		assert.Contains(t, suffix, "updated_at = EXCLUDED.updated_at")
	})

	t.Run("pages: PK excluded, everything else refreshed under the guard", func(t *testing.T) {
		suffix := webPageUpsertSuffix
		assert.True(t, strings.HasPrefix(suffix, "ON CONFLICT (session_date, session_id, page_number) DO UPDATE SET "))
		assert.Contains(t, suffix, "WHERE EXCLUDED.beat_seq > web_pages.beat_seq")
		assert.Contains(t, suffix, "is_exit = EXCLUDED.is_exit")
		assert.NotContains(t, suffix, "page_number = EXCLUDED.page_number")
	})

	t.Run("goals: four-column dedup key excluded", func(t *testing.T) {
		suffix := webGoalUpsertSuffix
		assert.True(t, strings.HasPrefix(suffix, "ON CONFLICT (session_date, session_id, goal_name, client_ts_ms) DO UPDATE SET "))
		assert.Contains(t, suffix, "WHERE EXCLUDED.beat_seq > web_goals.beat_seq")
		assert.NotContains(t, suffix, "client_ts_ms = EXCLUDED.client_ts_ms")
		assert.Contains(t, suffix, "properties = EXCLUDED.properties")
	})
}

// anyArgsExcept builds a WithArgs slice of the given size where every position
// is AnyArg except the pinned ones.
func anyArgsExcept(size int, pinned map[int]driver.Value) []driver.Value {
	args := make([]driver.Value, size)
	for i := range args {
		if v, ok := pinned[i]; ok {
			args[i] = v
		} else {
			args[i] = sqlmock.AnyArg()
		}
	}
	return args
}

func TestFlushBatch(t *testing.T) {
	date := time.Date(2024, 5, 10, 0, 0, 0, 0, time.UTC)

	t.Run("empty batch does nothing", func(t *testing.T) {
		repo, mock, cleanup := newWebAnalyticsRepoForTest(t)
		defer cleanup()
		require.NoError(t, repo.FlushBatch(context.Background(), waTestWorkspace, nil, nil, nil))
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("one transaction, rows sorted by primary key", func(t *testing.T) {
		repo, mock, cleanup := newWebAnalyticsRepoForTest(t)
		defer cleanup()

		// Deliberately out of order: "zz" before "aa".
		sessions := []*domain.WebSession{waTestSession("zz0e8400-e29b-41d4-a716-446655440000", date), waTestSession("aa0e8400-e29b-41d4-a716-446655440000", date)}
		pages := []*domain.WebPage{
			{SessionDate: date, SessionID: "aa0e8400-e29b-41d4-a716-446655440000", PageNumber: 2, BeatSeq: 2, EnteredAt: date, ExitedAt: date},
			{SessionDate: date, SessionID: "aa0e8400-e29b-41d4-a716-446655440000", PageNumber: 1, BeatSeq: 2, EnteredAt: date, ExitedAt: date},
		}
		goals := []*domain.WebGoal{
			{SessionDate: date, SessionID: "aa0e8400-e29b-41d4-a716-446655440000", GoalName: "signup", ClientTsMs: 200, BeatSeq: 2, GoalAt: date, Properties: map[string]string{"plan": "pro"}},
			{SessionDate: date, SessionID: "aa0e8400-e29b-41d4-a716-446655440000", GoalName: "signup", ClientTsMs: 100, BeatSeq: 2, GoalAt: date},
		}

		mock.ExpectBegin()
		mock.ExpectExec("INSERT INTO web_sessions").
			WithArgs(anyArgsExcept(2*len(webSessionColumns), map[int]driver.Value{
				1:                        "aa0e8400-e29b-41d4-a716-446655440000",
				len(webSessionColumns)+1: "zz0e8400-e29b-41d4-a716-446655440000",
			})...).
			WillReturnResult(sqlmock.NewResult(0, 2))
		mock.ExpectExec("INSERT INTO web_pages").
			WithArgs(anyArgsExcept(2*len(webPageColumns), map[int]driver.Value{
				2:                     int64(1), // first row is page_number 1 after sorting
				len(webPageColumns)+2: int64(2),
			})...).
			WillReturnResult(sqlmock.NewResult(0, 2))
		mock.ExpectExec("INSERT INTO web_goals").
			WithArgs(anyArgsExcept(2*len(webGoalColumns), map[int]driver.Value{
				3:                     int64(100), // client_ts_ms ascending after sorting
				len(webGoalColumns)+3: int64(200),
			})...).
			WillReturnResult(sqlmock.NewResult(0, 2))
		mock.ExpectCommit()

		require.NoError(t, repo.FlushBatch(context.Background(), waTestWorkspace, sessions, pages, goals))
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("duplicate keys within one batch are collapsed, not sent to Postgres", func(t *testing.T) {
		// A client can legitimately produce two goals with the same name in
		// the same millisecond (double-click, retry loop). Sending both in one
		// INSERT ... ON CONFLICT raises "command cannot affect row a second
		// time" and aborts the whole workspace transaction, destroying every
		// other visitor batched alongside them.
		repo, mock, cleanup := newWebAnalyticsRepoForTest(t)
		defer cleanup()

		session := "aa0e8400-e29b-41d4-a716-446655440000"
		pages := []*domain.WebPage{
			{SessionDate: date, SessionID: session, PageNumber: 1, BeatSeq: 1, Path: "/first", EnteredAt: date, ExitedAt: date},
			{SessionDate: date, SessionID: session, PageNumber: 1, BeatSeq: 2, Path: "/second", EnteredAt: date, ExitedAt: date},
		}
		goals := []*domain.WebGoal{
			{SessionDate: date, SessionID: session, GoalName: "signup", ClientTsMs: 500, BeatSeq: 1, GoalAt: date, GoalValue: 1},
			{SessionDate: date, SessionID: session, GoalName: "signup", ClientTsMs: 500, BeatSeq: 2, GoalAt: date, GoalValue: 2},
		}

		mock.ExpectBegin()
		// Exactly one row each: the later action wins.
		mock.ExpectExec("INSERT INTO web_pages").
			WithArgs(anyArgsExcept(len(webPageColumns), map[int]driver.Value{4: "/second"})...).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec("INSERT INTO web_goals").
			WithArgs(anyArgsExcept(len(webGoalColumns), map[int]driver.Value{3: int64(500)})...).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()

		require.NoError(t, repo.FlushBatch(context.Background(), waTestWorkspace, nil, pages, goals))
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("oversized durations are clamped instead of overflowing INTEGER", func(t *testing.T) {
		// duration_ms is INTEGER; an unclamped hostile value raises SQLSTATE
		// 22003 and takes the whole batch down with it.
		repo, mock, cleanup := newWebAnalyticsRepoForTest(t)
		defer cleanup()

		session := waTestSession("aa0e8400-e29b-41d4-a716-446655440000", date)
		session.DurationMs = 9_000_000_000
		session.MedianPageDurationMs = 9_000_000_000

		mock.ExpectBegin()
		mock.ExpectExec("INSERT INTO web_sessions").
			WithArgs(anyArgsExcept(len(webSessionColumns), map[int]driver.Value{
				5: int64(2147483647), // duration_ms
				7: int64(2147483647), // median_page_duration_ms
			})...).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()

		require.NoError(t, repo.FlushBatch(context.Background(), waTestWorkspace, []*domain.WebSession{session}, nil, nil))
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("missing partition triggers create and exactly one retry", func(t *testing.T) {
		repo, mock, cleanup := newWebAnalyticsRepoForTest(t)
		defer cleanup()

		sessions := []*domain.WebSession{waTestSession("aa0e8400-e29b-41d4-a716-446655440000", date)}

		mock.ExpectBegin()
		mock.ExpectExec("INSERT INTO web_sessions").
			WillReturnError(&pq.Error{Code: "23514", Message: `no partition of relation "web_sessions" found for row`})
		mock.ExpectRollback()

		// EnsureMonthlyPartitions for 2024-05 (past month → no autovacuum ALTER).
		for range schema.WebAnalyticsTableNames {
			mock.ExpectExec("CREATE TABLE IF NOT EXISTS web_(sessions|pages|goals)_y2024m05 PARTITION OF").
				WillReturnResult(sqlmock.NewResult(0, 0))
		}

		mock.ExpectBegin()
		mock.ExpectExec("INSERT INTO web_sessions").WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()

		require.NoError(t, repo.FlushBatch(context.Background(), waTestWorkspace, sessions, nil, nil))
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("second failure after partition creation surfaces the error", func(t *testing.T) {
		repo, mock, cleanup := newWebAnalyticsRepoForTest(t)
		defer cleanup()

		sessions := []*domain.WebSession{waTestSession("aa0e8400-e29b-41d4-a716-446655440000", date)}

		mock.ExpectBegin()
		mock.ExpectExec("INSERT INTO web_sessions").
			WillReturnError(&pq.Error{Code: "23514", Message: `no partition of relation "web_sessions" found for row`})
		mock.ExpectRollback()
		for range schema.WebAnalyticsTableNames {
			mock.ExpectExec("CREATE TABLE IF NOT EXISTS").WillReturnResult(sqlmock.NewResult(0, 0))
		}
		mock.ExpectBegin()
		mock.ExpectExec("INSERT INTO web_sessions").
			WillReturnError(&pq.Error{Code: "23514", Message: `no partition of relation "web_sessions" found for row`})
		mock.ExpectRollback()

		err := repo.FlushBatch(context.Background(), waTestWorkspace, sessions, nil, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "web_sessions")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("non-partition errors do not retry", func(t *testing.T) {
		repo, mock, cleanup := newWebAnalyticsRepoForTest(t)
		defer cleanup()

		sessions := []*domain.WebSession{waTestSession("aa0e8400-e29b-41d4-a716-446655440000", date)}

		mock.ExpectBegin()
		mock.ExpectExec("INSERT INTO web_sessions").WillReturnError(errors.New("connection refused"))
		mock.ExpectRollback()

		err := repo.FlushBatch(context.Background(), waTestWorkspace, sessions, nil, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "connection refused")
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestEnsureMonthlyPartitions(t *testing.T) {
	t.Run("past months are created without autovacuum tuning", func(t *testing.T) {
		repo, mock, cleanup := newWebAnalyticsRepoForTest(t)
		defer cleanup()

		month := time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC)
		for _, table := range schema.WebAnalyticsTableNames {
			mock.ExpectExec("CREATE TABLE IF NOT EXISTS " + schema.WebAnalyticsPartitionName(table, month)).
				WillReturnResult(sqlmock.NewResult(0, 0))
		}
		require.NoError(t, repo.EnsureMonthlyPartitions(context.Background(), waTestWorkspace, []time.Time{month}))
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("current month also gets the aggressive autovacuum profile", func(t *testing.T) {
		repo, mock, cleanup := newWebAnalyticsRepoForTest(t)
		defer cleanup()

		month := time.Now().UTC()
		for _, table := range schema.WebAnalyticsTableNames {
			mock.ExpectExec("CREATE TABLE IF NOT EXISTS " + schema.WebAnalyticsPartitionName(table, month)).
				WillReturnResult(sqlmock.NewResult(0, 0))
			mock.ExpectExec(`ALTER TABLE "` + schema.WebAnalyticsPartitionName(table, month) + `" SET`).
				WillReturnResult(sqlmock.NewResult(0, 0))
		}
		require.NoError(t, repo.EnsureMonthlyPartitions(context.Background(), waTestWorkspace, []time.Time{month}))
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestListPartitions(t *testing.T) {
	repo, mock, cleanup := newWebAnalyticsRepoForTest(t)
	defer cleanup()

	t.Run("unknown table rejected", func(t *testing.T) {
		_, err := repo.ListPartitions(context.Background(), waTestWorkspace, "contacts")
		assert.ErrorContains(t, err, "unknown web analytics table")
	})

	t.Run("lists partitions of a parent", func(t *testing.T) {
		mock.ExpectQuery("SELECT c.relname").WithArgs("web_sessions").
			WillReturnRows(sqlmock.NewRows([]string{"relname"}).AddRow("web_sessions_y2026m07").AddRow("web_sessions_y2026m08"))
		names, err := repo.ListPartitions(context.Background(), waTestWorkspace, "web_sessions")
		require.NoError(t, err)
		assert.Equal(t, []string{"web_sessions_y2026m07", "web_sessions_y2026m08"}, names)
	})
}

func TestAnalyzePartitions(t *testing.T) {
	repo, mock, cleanup := newWebAnalyticsRepoForTest(t)
	defer cleanup()

	t.Run("invalid names rejected before touching the database", func(t *testing.T) {
		err := repo.AnalyzePartitions(context.Background(), waTestWorkspace, []string{"web_sessions_y2026m08; DROP TABLE contacts"})
		assert.ErrorContains(t, err, "invalid partition name")
	})

	t.Run("analyzes valid partitions", func(t *testing.T) {
		mock.ExpectExec(`ANALYZE "web_sessions_y2026m08"`).WillReturnResult(sqlmock.NewResult(0, 0))
		require.NoError(t, repo.AnalyzePartitions(context.Background(), waTestWorkspace, []string{"web_sessions_y2026m08"}))
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestSetPartitionAutovacuum(t *testing.T) {
	repo, mock, cleanup := newWebAnalyticsRepoForTest(t)
	defer cleanup()

	t.Run("invalid name rejected", func(t *testing.T) {
		assert.ErrorContains(t,
			repo.SetPartitionAutovacuum(context.Background(), waTestWorkspace, "bogus", true),
			"invalid partition name")
	})

	t.Run("aggressive applies SET, reset applies RESET", func(t *testing.T) {
		mock.ExpectExec(`(?s)ALTER TABLE "web_pages_y2026m08" SET.*autovacuum_vacuum_scale_factor`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		require.NoError(t, repo.SetPartitionAutovacuum(context.Background(), waTestWorkspace, "web_pages_y2026m08", true))

		mock.ExpectExec(`(?s)ALTER TABLE "web_pages_y2026m08" RESET.*autovacuum_vacuum_scale_factor`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		require.NoError(t, repo.SetPartitionAutovacuum(context.Background(), waTestWorkspace, "web_pages_y2026m08", false))
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}
