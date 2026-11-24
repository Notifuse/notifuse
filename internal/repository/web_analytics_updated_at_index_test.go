package repository

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/database/schema"
)

func expectIndexValidity(mock sqlmock.Sqlmock, index string, valid bool) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT indisvalid FROM pg_index WHERE indexrelid = to_regclass($1)`)).WithArgs(index).
		WillReturnRows(sqlmock.NewRows([]string{"indisvalid"}).AddRow(valid))
}

func expectNoIndex(mock sqlmock.Sqlmock, index string) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT indisvalid FROM pg_index WHERE indexrelid = to_regclass($1)`)).WithArgs(index).
		WillReturnRows(sqlmock.NewRows([]string{"indisvalid"}))
}

func expectBuildLock(mock sqlmock.Sqlmock, locked bool) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT pg_try_advisory_lock(hashtext($1))`)).WithArgs(webSessionsUpdatedAtIndexLockKey).
		WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(locked))
}

func TestBuildWebSessionsUpdatedAtIndex(t *testing.T) {
	ctx := context.Background()
	parent := schema.WebSessionsUpdatedAtIndexName
	listQuery := regexp.QuoteMeta(`WHERE t.inhparent = 'web_sessions'::regclass`)

	t.Run("a valid index costs one catalog read", func(t *testing.T) {
		repo, mock, cleanup := newWebAnalyticsRepoForTest(t)
		defer cleanup()
		expectIndexValidity(mock, parent, true)

		valid, err := repo.BuildWebSessionsUpdatedAtIndex(ctx, waTestWorkspace)
		require.NoError(t, err)
		assert.True(t, valid)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("another node building it: nothing is built", func(t *testing.T) {
		repo, mock, cleanup := newWebAnalyticsRepoForTest(t)
		defer cleanup()
		expectIndexValidity(mock, parent, false)
		expectBuildLock(mock, false)

		valid, err := repo.BuildWebSessionsUpdatedAtIndex(ctx, waTestWorkspace)
		require.NoError(t, err)
		assert.False(t, valid)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("builds and attaches each partition's index concurrently, replacing an invalid one", func(t *testing.T) {
		repo, mock, cleanup := newWebAnalyticsRepoForTest(t)
		defer cleanup()
		expectIndexValidity(mock, parent, false)
		expectBuildLock(mock, true)
		mock.ExpectExec(regexp.QuoteMeta(`SET statement_timeout = 0`)).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(listQuery).WithArgs(parent).
			WillReturnRows(sqlmock.NewRows([]string{"relname"}).AddRow("web_sessions_y2026m10").AddRow("web_sessions_y2026m11"))

		expectNoIndex(mock, "web_sessions_y2026m10_updated_at_brin")
		mock.ExpectExec(regexp.QuoteMeta(`CREATE INDEX CONCURRENTLY IF NOT EXISTS "web_sessions_y2026m10_updated_at_brin" ON "web_sessions_y2026m10" USING BRIN (updated_at)`)).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(regexp.QuoteMeta(`ALTER INDEX ` + parent + ` ATTACH PARTITION "web_sessions_y2026m10_updated_at_brin"`)).
			WillReturnResult(sqlmock.NewResult(0, 0))

		// A build cut short left an invalid index: dropped and built again.
		expectIndexValidity(mock, "web_sessions_y2026m11_updated_at_brin", false)
		mock.ExpectExec(regexp.QuoteMeta(`DROP INDEX CONCURRENTLY IF EXISTS "web_sessions_y2026m11_updated_at_brin"`)).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(regexp.QuoteMeta(`CREATE INDEX CONCURRENTLY IF NOT EXISTS "web_sessions_y2026m11_updated_at_brin" ON "web_sessions_y2026m11" USING BRIN (updated_at)`)).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(regexp.QuoteMeta(`ALTER INDEX ` + parent + ` ATTACH PARTITION "web_sessions_y2026m11_updated_at_brin"`)).
			WillReturnResult(sqlmock.NewResult(0, 0))

		expectIndexValidity(mock, parent, true)
		mock.ExpectExec(regexp.QuoteMeta(`RESET statement_timeout`)).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_unlock(hashtext($1))`)).WithArgs(webSessionsUpdatedAtIndexLockKey).
			WillReturnResult(sqlmock.NewResult(0, 0))

		valid, err := repo.BuildWebSessionsUpdatedAtIndex(ctx, waTestWorkspace)
		require.NoError(t, err)
		assert.True(t, valid)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("a missing parent index is created alone first", func(t *testing.T) {
		repo, mock, cleanup := newWebAnalyticsRepoForTest(t)
		defer cleanup()
		expectNoIndex(mock, parent)
		expectBuildLock(mock, true)
		mock.ExpectExec(regexp.QuoteMeta(`SET statement_timeout = 0`)).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(regexp.QuoteMeta(schema.WebSessionsUpdatedAtParentIndex)).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(listQuery).WithArgs(parent).WillReturnRows(sqlmock.NewRows([]string{"relname"}))
		expectIndexValidity(mock, parent, true)
		mock.ExpectExec(regexp.QuoteMeta(`RESET statement_timeout`)).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_unlock`)).WillReturnResult(sqlmock.NewResult(0, 0))

		valid, err := repo.BuildWebSessionsUpdatedAtIndex(ctx, waTestWorkspace)
		require.NoError(t, err)
		assert.True(t, valid, "a parent without partitions is valid at once")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("a partition name outside the naming scheme is refused", func(t *testing.T) {
		repo, mock, cleanup := newWebAnalyticsRepoForTest(t)
		defer cleanup()
		expectIndexValidity(mock, parent, false)
		expectBuildLock(mock, true)
		mock.ExpectExec(regexp.QuoteMeta(`SET statement_timeout = 0`)).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(listQuery).WithArgs(parent).
			WillReturnRows(sqlmock.NewRows([]string{"relname"}).AddRow(`web_sessions_old"; DROP TABLE contacts; --`))
		mock.ExpectExec(regexp.QuoteMeta(`RESET statement_timeout`)).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_unlock`)).WillReturnResult(sqlmock.NewResult(0, 0))

		_, err := repo.BuildWebSessionsUpdatedAtIndex(ctx, waTestWorkspace)
		assert.ErrorContains(t, err, "invalid partition name")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("a failed build is returned", func(t *testing.T) {
		repo, mock, cleanup := newWebAnalyticsRepoForTest(t)
		defer cleanup()
		expectIndexValidity(mock, parent, false)
		expectBuildLock(mock, true)
		mock.ExpectExec(regexp.QuoteMeta(`SET statement_timeout = 0`)).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(listQuery).WithArgs(parent).WillReturnRows(sqlmock.NewRows([]string{"relname"}).AddRow("web_sessions_y2026m11"))
		expectNoIndex(mock, "web_sessions_y2026m11_updated_at_brin")
		mock.ExpectExec(`CREATE INDEX CONCURRENTLY`).WillReturnError(errors.New("canceling statement due to user request"))
		mock.ExpectExec(regexp.QuoteMeta(`RESET statement_timeout`)).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_unlock`)).WillReturnResult(sqlmock.NewResult(0, 0))

		_, err := repo.BuildWebSessionsUpdatedAtIndex(ctx, waTestWorkspace)
		assert.ErrorContains(t, err, "failed to build web_sessions_y2026m11_updated_at_brin")
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}
