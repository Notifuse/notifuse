package migrations

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/database/schema"
	"github.com/Notifuse/notifuse/internal/domain"
)

func TestV44Migration_Metadata(t *testing.T) {
	m := &V44Migration{}
	assert.Equal(t, 44.0, m.GetMajorVersion())
	assert.False(t, m.HasSystemUpdate())
	assert.True(t, m.HasWorkspaceUpdate())
	assert.False(t, m.ShouldRestartServer())
	assert.NoError(t, m.UpdateSystem(context.Background(), &config.Config{}, nil))
}

func TestV44Migration_IsRegistered(t *testing.T) {
	migration, ok := GetRegisteredMigration(44.0)
	require.True(t, ok)
	assert.IsType(t, &V44Migration{}, migration)
}

func TestV44Migration_IsWhatLetsTheStampReachFortyFour(t *testing.T) {
	var selected []float64
	for _, m := range GetRegisteredMigrations() {
		if v := m.GetMajorVersion(); v > 43.0 && v <= 44.0 {
			selected = append(selected, v)
		}
	}
	assert.Equal(t, []float64{44.0}, selected)
}

// The exact statements of the workspace update, in order. The first one replaces the body of
// the timeline function (no trigger is dropped or created). The two SET LOCAL statements let
// the index builds outlive a configured statement_timeout, and bound the wait for the table
// lock behind a transaction already writing to it. The exited recount runs last, after the
// builds, so it never holds automations rows while waiting for the message_history lock. No column is touched, so no automation trigger function needs
// regenerating.
var v44ExpectedWorkspaceStatements = []string{
	schema.TrackMessageHistoryChangesFunction(),
	`SET LOCAL statement_timeout = 0`,
	`SET LOCAL lock_timeout = '5s'`,
	`CREATE INDEX IF NOT EXISTS idx_message_history_failed_recent ON message_history(created_at DESC, id DESC) WHERE failed_at IS NOT NULL`,
	`CREATE INDEX IF NOT EXISTS idx_message_history_bounced_recent ON message_history(created_at DESC, id DESC) WHERE bounced_at IS NOT NULL`,
	`CREATE INDEX IF NOT EXISTS idx_message_history_complained_recent ON message_history(created_at DESC, id DESC) WHERE complained_at IS NOT NULL`,
	`UPDATE automations a
SET stats = jsonb_set(COALESCE(a.stats, '{}'::jsonb), '{exited}', to_jsonb(c.n)),
    updated_at = NOW()
FROM (SELECT automation_id, COUNT(*) AS n FROM contact_automations
      WHERE status = 'exited' GROUP BY automation_id) c
WHERE c.automation_id = a.id
  AND a.deleted_at IS NULL
  AND (a.stats IS NULL OR jsonb_typeof(a.stats) = 'object')
  AND c.n > CASE WHEN a.stats->>'exited' ~ '^[0-9]{1,18}$'
                 THEN (a.stats->>'exited')::bigint ELSE 0 END`,
}

func TestV44Migration_UpdateWorkspaceRunsExactlyItsStatementsInOrder(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	for _, stmt := range v44ExpectedWorkspaceStatements {
		mock.ExpectExec("^" + regexp.QuoteMeta(stmt) + "$").WillReturnResult(sqlmock.NewResult(0, 0))
	}

	require.NoError(t, (&V44Migration{}).UpdateWorkspace(context.Background(), &config.Config{}, &domain.Workspace{ID: "ws1"}, db))
	require.NoError(t, mock.ExpectationsWereMet())
}

// The timeline step keeps its own message: the run-window test of the manager matches it.
func TestV44Migration_UpdateWorkspaceReturnsTheTimelineError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec(regexp.QuoteMeta(schema.TrackMessageHistoryChangesFunction())).
		WillReturnError(errors.New("boom"))

	err = (&V44Migration{}).UpdateWorkspace(context.Background(), &config.Config{}, &domain.Workspace{ID: "ws1"}, db)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "link click timeline step")
	assert.Contains(t, err.Error(), "ws1")
	assert.Contains(t, err.Error(), "boom")
	require.NoError(t, mock.ExpectationsWereMet())
}

// An error on any later statement is returned with the workspace ID, and the statements
// after it are not run (sqlmock fails on an unexpected Exec).
func TestV44Migration_UpdateWorkspaceStopsOnTheFirstError(t *testing.T) {
	for failing := 1; failing < len(v44ExpectedWorkspaceStatements); failing++ {
		stmt := v44ExpectedWorkspaceStatements[failing]
		t.Run(stmt, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()

			for _, before := range v44ExpectedWorkspaceStatements[:failing] {
				mock.ExpectExec("^" + regexp.QuoteMeta(before) + "$").WillReturnResult(sqlmock.NewResult(0, 0))
			}
			mock.ExpectExec("^" + regexp.QuoteMeta(stmt) + "$").WillReturnError(errors.New("boom"))

			err = (&V44Migration{}).UpdateWorkspace(context.Background(), &config.Config{}, &domain.Workspace{ID: "ws1"}, db)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "ws1")
			assert.Contains(t, err.Error(), "boom")
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
