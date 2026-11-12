package repository

import (
	"context"
	"database/sql/driver"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
)

// sqlPattern turns a SQL fragment into a pattern where any run of whitespace matches a space.
func sqlPattern(fragment string) string {
	return strings.ReplaceAll(regexp.QuoteMeta(fragment), " ", `\s+`)
}

var contactVisitSessionColumns = []string{"id", "session_date", "created_at", "duration_ms", "pageview_count",
	"landing_path", "exit_path", "channel", "channel_group", "referrer_domain", "utm_source", "utm_campaign",
	"device", "os", "browser", "country", "goal_count", "goal_value", "first_goal_name"}

func contactVisitSessionRow(rows *sqlmock.Rows, id string, createdAt time.Time, firstGoal interface{}) *sqlmock.Rows {
	day := time.Date(createdAt.Year(), createdAt.Month(), createdAt.Day(), 0, 0, 0, 0, time.UTC)
	return rows.AddRow(id, day, createdAt, 61000, 3, "/pricing", "/signup", "email", "email", "", "newsletter", "nov",
		"desktop", "macOS", "Safari", "FR", 1, 49.5, firstGoal)
}

func setupContactVisitsRepo(t *testing.T) (domain.WebAnalyticsContactVisitsRepository, sqlmock.Sqlmock) {
	ctrl := gomock.NewController(t)
	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	workspaceRepo.EXPECT().GetConnection(gomock.Any(), "ws").Return(db, nil)
	return NewWebAnalyticsContactVisitsRepository(workspaceRepo), mock
}

func TestContactVisitsRepository_ListContactSessions(t *testing.T) {
	ctx := context.Background()
	at := time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC)

	t.Run("bare contact_email, newest first, limit + 1, first goal by goal order", func(t *testing.T) {
		repo, mock := setupContactVisitsRepo(t)
		mock.ExpectQuery(sqlPattern(`FROM web_sessions s WHERE s.contact_email = $1 ORDER BY s.created_at DESC, s.id DESC LIMIT $2`)).
			WithArgs("ann@example.com", 3).
			WillReturnRows(contactVisitSessionRow(sqlmock.NewRows(contactVisitSessionColumns), "0190d5a4-1c2b-7000-8000-000000000002", at, "signup"))

		sessions, next, err := repo.ListContactSessions(ctx, "ws", "ann@example.com", domain.ContactVisitsFilterAll, 2, "")
		require.NoError(t, err)
		require.Len(t, sessions, 1)
		assert.Equal(t, "", next, "no row beyond the page")
		assert.Equal(t, "2026-11-03", sessions[0].SessionDate)
		require.NotNil(t, sessions[0].FirstGoalName)
		assert.Equal(t, "signup", *sessions[0].FirstGoalName)
		assert.Equal(t, int64(61000), sessions[0].DurationMs)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("the first goal is read by the session's key, in goal order", func(t *testing.T) {
		repo, mock := setupContactVisitsRepo(t)
		mock.ExpectQuery(sqlPattern(`(SELECT g.goal_name FROM web_goals g WHERE g.session_date = s.session_date AND g.session_id = s.id ORDER BY g.goal_at, g.tab_id, g.client_ts_ms, g.goal_name LIMIT 1)`)).
			WillReturnRows(sqlmock.NewRows(contactVisitSessionColumns))
		_, _, err := repo.ListContactSessions(ctx, "ws", "ann@example.com", domain.ContactVisitsFilterAll, 2, "")
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("one more row than the limit gives the cursor of the last row kept", func(t *testing.T) {
		repo, mock := setupContactVisitsRepo(t)
		rows := sqlmock.NewRows(contactVisitSessionColumns)
		contactVisitSessionRow(rows, "0190d5a4-1c2b-7000-8000-000000000003", at, nil)
		contactVisitSessionRow(rows, "0190d5a4-1c2b-7000-8000-000000000002", at.Add(-time.Hour), nil)
		contactVisitSessionRow(rows, "0190d5a4-1c2b-7000-8000-000000000001", at.Add(-2*time.Hour), nil)
		mock.ExpectQuery(`FROM web_sessions s`).WithArgs("ann@example.com", 3).WillReturnRows(rows)

		sessions, next, err := repo.ListContactSessions(ctx, "ws", "ann@example.com", domain.ContactVisitsFilterAll, 2, "")
		require.NoError(t, err)
		require.Len(t, sessions, 2)
		assert.Nil(t, sessions[0].FirstGoalName)
		assert.Equal(t, domain.EncodeContactVisitsCursor(at.Add(-time.Hour), "0190d5a4-1c2b-7000-8000-000000000002"), next)
	})

	t.Run("the cursor condition and the goal filter", func(t *testing.T) {
		repo, mock := setupContactVisitsRepo(t)
		cursorAt := at.Add(-time.Hour)
		mock.ExpectQuery(sqlPattern(`WHERE s.contact_email = $1 AND (s.created_at < $2 OR (s.created_at = $2 AND s.id < $3)) AND s.goal_count > 0 ORDER BY s.created_at DESC, s.id DESC LIMIT $4`)).
			WithArgs("ann@example.com", cursorAt, "0190d5a4-1c2b-7000-8000-000000000002", 21).
			WillReturnRows(sqlmock.NewRows(contactVisitSessionColumns))

		_, _, err := repo.ListContactSessions(ctx, "ws", "ann@example.com", domain.ContactVisitsFilterGoal, 20,
			domain.EncodeContactVisitsCursor(cursorAt, "0190d5a4-1c2b-7000-8000-000000000002"))
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("the email filter", func(t *testing.T) {
		repo, mock := setupContactVisitsRepo(t)
		mock.ExpectQuery(sqlPattern(`WHERE s.contact_email = $1 AND s.channel_group = 'email' ORDER BY`)).
			WithArgs("ann@example.com", 21).
			WillReturnRows(sqlmock.NewRows(contactVisitSessionColumns))
		_, _, err := repo.ListContactSessions(ctx, "ws", "ann@example.com", domain.ContactVisitsFilterEmail, 20, "")
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("query error returned", func(t *testing.T) {
		repo, mock := setupContactVisitsRepo(t)
		mock.ExpectQuery(`FROM web_sessions s`).WillReturnError(errors.New("boom"))
		_, _, err := repo.ListContactSessions(ctx, "ws", "ann@example.com", domain.ContactVisitsFilterAll, 20, "")
		require.ErrorContains(t, err, "boom")
	})
}

func TestContactVisitsRepository_ContactVisitsSummary(t *testing.T) {
	ctx := context.Background()
	at := time.Date(2026, 10, 2, 8, 0, 0, 0, time.UTC)

	expectSummary := func(mock sqlmock.Sqlmock, counts []driver.Value, first *sqlmock.Rows, goals, pages *sqlmock.Rows) {
		mock.ExpectQuery(sqlPattern(`SELECT COUNT(*), COUNT(*) FILTER (WHERE goal_count > 0), COUNT(*) FILTER (WHERE channel_group = 'email'), COALESCE(ROUND(percentile_cont(0.5) WITHIN GROUP (ORDER BY duration_ms))::bigint, 0) FROM web_sessions WHERE contact_email = $1`)).
			WithArgs("ann@example.com").
			WillReturnRows(sqlmock.NewRows([]string{"sessions", "goal_sessions", "email_sessions", "median"}).AddRow(counts...))
		mock.ExpectQuery(sqlPattern(`FROM web_sessions s WHERE s.contact_email = $1 ORDER BY s.created_at ASC, s.id ASC LIMIT 1`)).
			WithArgs("ann@example.com").WillReturnRows(first)
		mock.ExpectQuery(sqlPattern(`SELECT goal_name, COUNT(*), COALESCE(SUM(goal_value), 0) FROM web_goals WHERE contact_email = $1 GROUP BY goal_name ORDER BY COUNT(*) DESC, goal_name LIMIT $2`)).
			WithArgs("ann@example.com", 5).WillReturnRows(goals)
		mock.ExpectQuery(sqlPattern(`SELECT path, COUNT(*) FROM web_pages WHERE contact_email = $1 GROUP BY path ORDER BY COUNT(*) DESC, path LIMIT $2`)).
			WithArgs("ann@example.com", 3).WillReturnRows(pages)
	}

	t.Run("counts, median, first session, goals and top pages", func(t *testing.T) {
		repo, mock := setupContactVisitsRepo(t)
		expectSummary(mock, []driver.Value{4, 1, 2, 30500},
			contactVisitSessionRow(sqlmock.NewRows(contactVisitSessionColumns), "0190d5a4-1c2b-7000-8000-000000000001", at, nil),
			sqlmock.NewRows([]string{"goal_name", "count", "value"}).AddRow("signup", 2, 10.5).AddRow("demo", 1, 0),
			sqlmock.NewRows([]string{"path", "views"}).AddRow("/pricing", 5).AddRow("/", 3))

		summary, err := repo.ContactVisitsSummary(ctx, "ws", "ann@example.com")
		require.NoError(t, err)
		assert.Equal(t, 4, summary.Sessions)
		assert.Equal(t, 1, summary.GoalSessions)
		assert.Equal(t, 2, summary.EmailSessions)
		assert.Equal(t, int64(30500), summary.MedianDurationMs)
		require.NotNil(t, summary.FirstSession)
		assert.Equal(t, "2026-10-02", summary.FirstSession.SessionDate)
		assert.Equal(t, []domain.ContactVisitGoal{{GoalName: "signup", Count: 2, Value: 10.5}, {GoalName: "demo", Count: 1}}, summary.Goals)
		assert.Equal(t, []domain.ContactVisitPage{{Path: "/pricing", Views: 5}, {Path: "/", Views: 3}}, summary.TopPages)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("a contact without sessions: zeros, no first session, empty lists", func(t *testing.T) {
		repo, mock := setupContactVisitsRepo(t)
		expectSummary(mock, []driver.Value{0, 0, 0, 0},
			sqlmock.NewRows(contactVisitSessionColumns),
			sqlmock.NewRows([]string{"goal_name", "count", "value"}),
			sqlmock.NewRows([]string{"path", "views"}))

		summary, err := repo.ContactVisitsSummary(ctx, "ws", "ann@example.com")
		require.NoError(t, err)
		assert.Equal(t, 0, summary.Sessions)
		assert.Nil(t, summary.FirstSession)
		assert.NotNil(t, summary.Goals)
		assert.Empty(t, summary.Goals)
		assert.NotNil(t, summary.TopPages)
		assert.Empty(t, summary.TopPages)
	})
}

func TestContactVisitsRepository_ContactSessionPages(t *testing.T) {
	ctx := context.Background()
	sessionID := "0190d5a4-1c2b-7000-8000-000000000001"
	enteredAt := time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC)
	ownership := sqlPattern(`SELECT EXISTS (SELECT 1 FROM web_sessions WHERE session_date = $1 AND id = $2 AND contact_email = $3)`)

	t.Run("the session's pages and goals, by key, in order", func(t *testing.T) {
		repo, mock := setupContactVisitsRepo(t)
		mock.ExpectQuery(ownership).WithArgs("2026-11-03", sessionID, "ann@example.com").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
		mock.ExpectQuery(sqlPattern(`SELECT entered_at, path, duration_ms, page_number, tab_id FROM web_pages WHERE session_date = $1 AND session_id = $2 ORDER BY entered_at, tab_id, page_number LIMIT $3`)).
			WithArgs("2026-11-03", sessionID, 200).
			WillReturnRows(sqlmock.NewRows([]string{"entered_at", "path", "duration_ms", "page_number", "tab_id"}).
				AddRow(enteredAt, "/", 1000, 1, 7).AddRow(enteredAt.Add(time.Second), "/pricing", 2000, 1, 9))
		mock.ExpectQuery(sqlPattern(`SELECT goal_at, goal_name, goal_value, path, tab_id FROM web_goals WHERE session_date = $1 AND session_id = $2 ORDER BY goal_at, tab_id, client_ts_ms, goal_name LIMIT $3`)).
			WithArgs("2026-11-03", sessionID, 200).
			WillReturnRows(sqlmock.NewRows([]string{"goal_at", "goal_name", "goal_value", "path", "tab_id"}).
				AddRow(enteredAt.Add(2*time.Second), "signup", 9.5, "/pricing", 9))

		got, err := repo.ContactSessionPages(ctx, "ws", "ann@example.com", "2026-11-03", sessionID)
		require.NoError(t, err)
		assert.Equal(t, []domain.ContactSessionPage{
			{EnteredAt: enteredAt, Path: "/", DurationMs: 1000, PageNumber: 1, TabID: 7},
			{EnteredAt: enteredAt.Add(time.Second), Path: "/pricing", DurationMs: 2000, PageNumber: 1, TabID: 9},
		}, got.Pages)
		assert.Equal(t, []domain.ContactSessionGoal{{GoalAt: enteredAt.Add(2 * time.Second), GoalName: "signup", GoalValue: 9.5, Path: "/pricing", TabID: 9}}, got.Goals)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("another contact's session: empty, pages and goals not read", func(t *testing.T) {
		repo, mock := setupContactVisitsRepo(t)
		mock.ExpectQuery(ownership).WithArgs("2026-11-03", sessionID, "bob@example.com").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

		got, err := repo.ContactSessionPages(ctx, "ws", "bob@example.com", "2026-11-03", sessionID)
		require.NoError(t, err)
		assert.Equal(t, &domain.ContactSessionPagesResponse{Pages: []domain.ContactSessionPage{}, Goals: []domain.ContactSessionGoal{}}, got)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}
