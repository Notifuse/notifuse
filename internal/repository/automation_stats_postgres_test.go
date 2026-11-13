package repository

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAutomationRepository_CountJourneysByStatus(t *testing.T) {
	const query = `^SELECT automation_id, status, COUNT\(\*\) FROM contact_automations WHERE automation_id = ANY\(\$1\) GROUP BY automation_id, status$`

	t.Run("counts by automation and status, an unknown status in total only", func(t *testing.T) {
		db, mock, repo := setupAutomationMock(t)
		defer func() { _ = db.Close() }()

		mock.ExpectQuery(query).
			WithArgs(pq.Array([]string{"a1", "a2", "a3"})).
			WillReturnRows(sqlmock.NewRows([]string{"automation_id", "status", "count"}).
				AddRow("a1", "active", 4).
				AddRow("a1", "completed", 3).
				AddRow("a1", "exited", 2).
				AddRow("a1", "failed", 1).
				AddRow("a2", "archived", 5))

		got, err := repo.CountJourneysByStatus(context.Background(), "ws1", []string{"a1", "a2", "a3"})
		require.NoError(t, err)
		assert.Equal(t, map[string]domain.JourneyCounts{
			"a1": {Active: 4, Completed: 3, Exited: 2, Failed: 1, Total: 10},
			"a2": {Total: 5},
		}, got, "an automation without journey is absent")
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("no automation: no query, an empty map", func(t *testing.T) {
		db, mock, repo := setupAutomationMock(t)
		defer func() { _ = db.Close() }()

		got, err := repo.CountJourneysByStatus(context.Background(), "ws1", nil)
		require.NoError(t, err)
		assert.NotNil(t, got)
		assert.Empty(t, got)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("returns the query error", func(t *testing.T) {
		db, mock, repo := setupAutomationMock(t)
		defer func() { _ = db.Close() }()

		mock.ExpectQuery(query).WillReturnError(errors.New("db down"))

		_, err := repo.CountJourneysByStatus(context.Background(), "ws1", []string{"a1"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "db down")
	})
}

// The node stats statements, matched on their FROM and WHERE clauses, the part that decides
// which rows count; the integration test checks what they count.
const (
	visitsQuery         = `SELECT node_id,\s+COUNT\(\*\) FILTER \(WHERE action IN \('processing', 'completed', 'skipped'\)\),[\s\S]*FROM automation_node_executions\s+WHERE automation_id = \$1%s\s+GROUP BY node_id$`
	notPassedQuery      = `FROM contact_automations ca\s+CROSS JOIN LATERAL \([\s\S]*ORDER BY e.entered_at DESC\s+LIMIT 1\s+\) l\s+WHERE ca.automation_id = \$1 AND ca.status IN \('active', 'failed', 'exited'\)\s+AND ca.current_node_id IS NOT NULL\s+AND l.node_id = ca.current_node_id AND l.action IN \('entered', 'failed'\)%s\s+GROUP BY 1$`
	skipReasonsQuery    = `SELECT node_id, output->>'skip_reason', COUNT\(\*\)\s+FROM automation_node_executions\s+WHERE automation_id = \$1 AND action = 'completed' AND output->>'skipped' = 'true'\s+AND output->>'skip_reason' IS NOT NULL%s\s+GROUP BY 1, 2$`
	branchesQuery       = `AND node_type IN \('filter', 'list_status_branch', 'ab_test', 'branch'\)%s\s+\) b\s+WHERE branch IS NOT NULL`
	inPlaceQuery        = `WHERE ca.automation_id = \$1 AND ca.status = 'active'\s+\) s\s+WHERE node_id IS NOT NULL\s+GROUP BY 1, 2$`
	failedJourneysQuery = `FROM contact_automations\s+WHERE automation_id = \$1 AND status = 'failed' AND current_node_id IS NOT NULL%s\s+GROUP BY 1$`
	emailQuery          = `WHERE e.automation_id = \$1 AND e.node_type = 'email' AND e.action = 'completed'\s+AND e.output \? 'message_id'%s\s+GROUP BY e.node_id$`
)

func period(column string) string {
	return ` AND ` + column + ` >= \$2 AND ` + column + ` <= \$3`
}

func expectNodeStatsQueries(mock sqlmock.Sqlmock, bounded, withEmail bool, args ...driver.Value) {
	p := func(column string) string {
		if bounded {
			return period(column)
		}
		return ""
	}
	sprintf := func(format, column string) string { return fmt.Sprintf(format, p(column)) }

	mock.ExpectQuery(sprintf(visitsQuery, "entered_at")).WithArgs(args...).
		WillReturnRows(sqlmock.NewRows([]string{"node_id", "visits", "completed", "errors", "failed_attempts", "skipped"}).
			AddRow("email-1", 10, 8, 0, 3, 2).
			AddRow("list-1", 5, 4, 1, 0, 0))
	// The passes not passed yet: on email-1 (retrying) and on a trigger no execution reached in
	// the visits rows (enrolled, not processed yet).
	mock.ExpectQuery(sprintf(notPassedQuery, "l.entered_at")).WithArgs(args...).
		WillReturnRows(sqlmock.NewRows([]string{"node_id", "count"}).AddRow("email-1", 2).AddRow("trigger-1", 1))
	mock.ExpectQuery(sprintf(skipReasonsQuery, "entered_at")).WithArgs(args...).
		WillReturnRows(sqlmock.NewRows([]string{"node_id", "reason", "count"}).AddRow("email-1", "unsubscribed", 2))
	mock.ExpectQuery(sprintf(branchesQuery, "entered_at")).WithArgs(args...).
		WillReturnRows(sqlmock.NewRows([]string{"node_id", "branch", "count"}).
			AddRow("filter-1", "yes", 6).AddRow("filter-1", "no", 4))
	// No period on the journeys in place: only the automation ID.
	mock.ExpectQuery(inPlaceQuery).WithArgs(args[0]).
		WillReturnRows(sqlmock.NewRows([]string{"state", "node_id", "count"}).
			AddRow("waiting", "delay-1", 7).AddRow("retrying", "email-1", 1).AddRow("in_progress", "email-1", 2))
	mock.ExpectQuery(sprintf(failedJourneysQuery, "entered_at")).WithArgs(args...).
		WillReturnRows(sqlmock.NewRows([]string{"node_id", "count"}).AddRow("email-1", 3))
	if withEmail {
		mock.ExpectQuery(sprintf(emailQuery, "e.entered_at")).WithArgs(args...).
			WillReturnRows(sqlmock.NewRows([]string{"node_id", "executions", "queued", "not_sent", "sent", "delivered", "opened",
				"opens_tracked", "clicked", "clicks_tracked", "failed", "bounced", "complained", "unsubscribed"}).
				AddRow("email-1", 8, 1, 1, 6, 5, 4, 6, 2, 6, 0, 1, 0, 1))
	}
}

func TestAutomationRepository_GetNodeStats(t *testing.T) {
	since := time.Date(2026, 10, 11, 22, 0, 0, 0, time.UTC)
	until := time.Date(2026, 11, 10, 22, 59, 59, 0, time.UTC)

	t.Run("all time with email: every statement, one read-only snapshot", func(t *testing.T) {
		db, mock, repo := setupAutomationMock(t)
		defer func() { _ = db.Close() }()

		mock.ExpectBegin()
		expectNodeStatsQueries(mock, false, true, "auto1")
		mock.ExpectCommit()

		got, err := repo.GetNodeStats(context.Background(), "ws1", "auto1", nil, nil, true)
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())

		email := got.Nodes["email-1"]
		require.NotNil(t, email)
		assert.Equal(t, int64(12), email.Visits, "passed rows plus the passes not passed yet")
		assert.Equal(t, int64(8), email.Completed)
		assert.Equal(t, int64(3), email.FailedAttempts)
		assert.Equal(t, int64(2), email.Skipped)
		assert.Equal(t, map[string]int64{"unsubscribed": 2}, email.SkipReasons)
		assert.Equal(t, int64(1), email.Retrying)
		assert.Equal(t, int64(2), email.InProgress)
		assert.Equal(t, int64(3), email.FailedJourneys)
		require.NotNil(t, email.Email)
		assert.Equal(t, domain.NodeEmailStats{Executions: 8, Queued: 1, NotSent: 1, Sent: 6, Delivered: 5, Opened: 4,
			OpensTracked: 6, Clicked: 2, ClicksTracked: 6, Bounced: 1, Unsubscribed: 1}, *email.Email)

		assert.Equal(t, int64(1), got.Nodes["list-1"].Errors)
		assert.Nil(t, got.Nodes["list-1"].Email, "no email row, no email counters")
		assert.Equal(t, map[string]int64{"yes": 6, "no": 4}, got.Nodes["filter-1"].Branches)
		assert.Equal(t, int64(7), got.Nodes["delay-1"].Waiting)
		assert.NotNil(t, got.Nodes["delay-1"].Branches, "maps are never nil")
		trigger := got.Nodes["trigger-1"]
		require.NotNil(t, trigger, "a step only the not-passed statement returns")
		assert.Equal(t, int64(1), trigger.Visits)
		assert.NotNil(t, trigger.Branches)
		assert.NotNil(t, trigger.SkipReasons)
	})

	t.Run("a period bounds every statement but the journeys in place", func(t *testing.T) {
		db, mock, repo := setupAutomationMock(t)
		defer func() { _ = db.Close() }()

		mock.ExpectBegin()
		expectNodeStatsQueries(mock, true, true, "auto1", since, until)
		mock.ExpectCommit()

		_, err := repo.GetNodeStats(context.Background(), "ws1", "auto1", &since, &until, true)
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("without email: the email statement is not run", func(t *testing.T) {
		db, mock, repo := setupAutomationMock(t)
		defer func() { _ = db.Close() }()

		mock.ExpectBegin()
		expectNodeStatsQueries(mock, true, false, "auto1", since, until)
		mock.ExpectCommit()

		got, err := repo.GetNodeStats(context.Background(), "ws1", "auto1", &since, &until, false)
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
		assert.Nil(t, got.Nodes["email-1"].Email)
	})

	t.Run("a statement error is returned and the snapshot rolled back", func(t *testing.T) {
		db, mock, repo := setupAutomationMock(t)
		defer func() { _ = db.Close() }()

		mock.ExpectBegin()
		mock.ExpectQuery(fmt.Sprintf(visitsQuery, "")).WithArgs("auto1").WillReturnError(errors.New("db down"))
		mock.ExpectRollback()

		_, err := repo.GetNodeStats(context.Background(), "ws1", "auto1", nil, nil, true)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "visits")
		assert.Contains(t, err.Error(), "db down")
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("the not-passed statement error is returned and the snapshot rolled back", func(t *testing.T) {
		db, mock, repo := setupAutomationMock(t)
		defer func() { _ = db.Close() }()

		mock.ExpectBegin()
		mock.ExpectQuery(fmt.Sprintf(visitsQuery, "")).WithArgs("auto1").
			WillReturnRows(sqlmock.NewRows([]string{"node_id", "visits", "completed", "errors", "failed_attempts", "skipped"}))
		mock.ExpectQuery(fmt.Sprintf(notPassedQuery, "")).WithArgs("auto1").WillReturnError(errors.New("db down"))
		mock.ExpectRollback()

		_, err := repo.GetNodeStats(context.Background(), "ws1", "auto1", nil, nil, true)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not passed")
		assert.Contains(t, err.Error(), "db down")
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("the email statement error is returned", func(t *testing.T) {
		db, mock, repo := setupAutomationMock(t)
		defer func() { _ = db.Close() }()

		mock.ExpectBegin()
		expectNodeStatsQueries(mock, false, false, "auto1")
		mock.ExpectQuery(fmt.Sprintf(emailQuery, "")).WithArgs("auto1").WillReturnError(errors.New("db down"))
		mock.ExpectRollback()

		_, err := repo.GetNodeStats(context.Background(), "ws1", "auto1", nil, nil, true)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "emails")
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("a failure to begin the snapshot is returned", func(t *testing.T) {
		db, mock, repo := setupAutomationMock(t)
		defer func() { _ = db.Close() }()

		mock.ExpectBegin().WillReturnError(sql.ErrConnDone)
		_, err := repo.GetNodeStats(context.Background(), "ws1", "auto1", nil, nil, true)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "begin")
	})
}
