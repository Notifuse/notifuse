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

// The part builds the index init.go builds on a new workspace, after turning empty keys into
// NULL and the deduplication, and only when the index is missing, so a rerun neither locks nor
// rewrites anything.
func TestV45ExternalIDUniqueSQL(t *testing.T) {
	sql := v45ExternalIDUniqueSQL

	assert.Contains(t, sql, "IF to_regclass('"+schema.MessageHistoryExternalIDIndexName+"') IS NULL THEN")
	assert.Contains(t, sql, schema.MessageHistoryExternalIDIndex+";")

	lock := strings.Index(sql, "LOCK TABLE message_history IN SHARE MODE")
	empty := strings.Index(sql, "UPDATE message_history SET external_id = NULL WHERE external_id = '';")
	dedup := strings.Index(sql, "UPDATE message_history AS m")
	build := strings.Index(sql, schema.MessageHistoryExternalIDIndex)
	guard := strings.Index(sql, "IF to_regclass(")
	require.True(t, guard >= 0 && lock >= 0 && empty >= 0 && dedup >= 0 && build >= 0)
	assert.True(t, guard < lock && lock < empty && empty < dedup && dedup < build,
		"the guard, then the lock, then '' to NULL, then the deduplication, then the build")

	// The oldest keeps the key, the others give it up; no row is deleted.
	assert.Contains(t, sql, "ROW_NUMBER() OVER (PARTITION BY external_id ORDER BY created_at, id)")
	assert.Contains(t, sql, "ranked.position > 1")
	assert.NotContains(t, strings.ToUpper(sql), "DELETE")

	// Runs in the migration's transaction: no CONCURRENTLY.
	assert.NotContains(t, strings.ToUpper(sql), "CONCURRENTLY")
}

// The timeouts are saved, lifted and bounded around the build, then restored to the values
// the transaction had, so the parts that run later keep theirs.
func TestExternalIDWorkspaceV45Statements(t *testing.T) {
	assert.Equal(t, []string{
		`SELECT set_config('notifuse.v45_external_id_statement_timeout', current_setting('statement_timeout'), true)`,
		`SELECT set_config('notifuse.v45_external_id_lock_timeout', current_setting('lock_timeout'), true)`,
		`SET LOCAL statement_timeout = 0`,
		`SET LOCAL lock_timeout = '5s'`,
		v45ExternalIDUniqueSQL,
		`SELECT set_config('lock_timeout', current_setting('notifuse.v45_external_id_lock_timeout'), true)`,
		`SELECT set_config('statement_timeout', current_setting('notifuse.v45_external_id_statement_timeout'), true)`,
	}, externalIDWorkspaceV45Statements())
}

func TestApplyExternalIDWorkspaceV45(t *testing.T) {
	t.Run("runs every statement in order", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		for _, statement := range externalIDWorkspaceV45Statements() {
			mock.ExpectExec(regexp.QuoteMeta(statement)).WillReturnResult(sqlmock.NewResult(0, 0))
		}
		require.NoError(t, applyExternalIDWorkspaceV45(context.Background(), db))
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("stops at the first error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		statements := externalIDWorkspaceV45Statements()
		mock.ExpectExec(regexp.QuoteMeta(statements[0])).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(regexp.QuoteMeta(statements[1])).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(regexp.QuoteMeta(statements[2])).WillReturnError(errors.New("could not create unique index"))
		err = applyExternalIDWorkspaceV45(context.Background(), db)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unique external_id (v45)")
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

// A failing external_id part fails the workspace update with its own name.
func TestV45Migration_ExternalIDPartFailure(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	for _, statement := range dataRetentionWorkspaceV45Statements() {
		mock.ExpectExec(regexp.QuoteMeta(statement)).WillReturnResult(sqlmock.NewResult(0, 0))
	}
	mock.ExpectExec(regexp.QuoteMeta(externalIDWorkspaceV45Statements()[0])).WillReturnError(errors.New("boom"))

	err = (&V45Migration{}).UpdateWorkspace(context.Background(), &config.Config{}, &domain.Workspace{ID: "ws1"}, db)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unique external_id (v45)")
	require.NoError(t, mock.ExpectationsWereMet())
}
