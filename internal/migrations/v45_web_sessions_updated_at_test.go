package migrations

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/database/schema"
	"github.com/Notifuse/notifuse/internal/domain"
)

// The part creates the index on the parent alone, only when it is missing, so it reads no
// partition and a rerun takes no lock.
func TestV45WebSessionsUpdatedAtIndexSQL(t *testing.T) {
	sql := v45WebSessionsUpdatedAtIndexSQL

	guard := strings.Index(sql, "IF to_regclass('"+schema.WebSessionsUpdatedAtIndexName+"') IS NULL THEN")
	build := strings.Index(sql, schema.WebSessionsUpdatedAtParentIndex+";")
	require.True(t, guard >= 0 && build >= 0)
	assert.Less(t, guard, build, "the guard, then the parent index")
	assert.Contains(t, sql, "ON ONLY web_sessions USING BRIN (updated_at)")
	assert.NotContains(t, sql, schema.WebSessionsUpdatedAtIndex, "never the index built on every partition")

	// Runs in the migration's transaction: no CONCURRENTLY.
	assert.NotContains(t, strings.ToUpper(sql), "CONCURRENTLY")
}

// lock_timeout is saved, bounded around the statement, then restored to the value the
// transaction had.
func TestWebSessionsUpdatedAtWorkspaceV45Statements(t *testing.T) {
	assert.Equal(t, []string{
		`SELECT set_config('notifuse.v45_web_sessions_lock_timeout', current_setting('lock_timeout'), true)`,
		`SET LOCAL lock_timeout = '5s'`,
		v45WebSessionsUpdatedAtIndexSQL,
		`SELECT set_config('lock_timeout', current_setting('notifuse.v45_web_sessions_lock_timeout'), true)`,
	}, webSessionsUpdatedAtWorkspaceV45Statements())
}

func TestApplyWebSessionsUpdatedAtWorkspaceV45(t *testing.T) {
	t.Run("runs every statement in order", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		for _, statement := range webSessionsUpdatedAtWorkspaceV45Statements() {
			mock.ExpectExec(regexp.QuoteMeta(statement)).WillReturnResult(sqlmock.NewResult(0, 0))
		}
		require.NoError(t, applyWebSessionsUpdatedAtWorkspaceV45(context.Background(), db))
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("stops at the first error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		statements := webSessionsUpdatedAtWorkspaceV45Statements()
		for _, statement := range statements[:2] {
			mock.ExpectExec(regexp.QuoteMeta(statement)).WillReturnResult(sqlmock.NewResult(0, 0))
		}
		mock.ExpectExec(regexp.QuoteMeta(statements[2])).WillReturnError(errors.New("canceling statement due to lock timeout"))
		err = applyWebSessionsUpdatedAtWorkspaceV45(context.Background(), db)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "web_sessions updated_at index (v45)")
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

// A failing updated_at index part fails the workspace update with its own name, after the
// parts before it ran.
func TestV45Migration_WebSessionsUpdatedAtPartFailure(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	for _, statement := range dataRetentionWorkspaceV45Statements() {
		mock.ExpectExec(regexp.QuoteMeta(statement)).WillReturnResult(sqlmock.NewResult(0, 0))
	}
	for _, statement := range externalIDWorkspaceV45Statements() {
		mock.ExpectExec(regexp.QuoteMeta(statement)).WillReturnResult(sqlmock.NewResult(0, 0))
	}
	mock.ExpectExec(regexp.QuoteMeta(webSessionsUpdatedAtWorkspaceV45Statements()[0])).WillReturnError(errors.New("boom"))

	err = (&V45Migration{}).UpdateWorkspace(context.Background(), &config.Config{}, &domain.Workspace{ID: "ws1"}, db)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "web_sessions updated_at index (v45)")
	require.NoError(t, mock.ExpectationsWereMet())
}
