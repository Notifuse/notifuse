package database

import (
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func expectSystemDatabaseLookup(mock sqlmock.Sqlmock, exists bool) {
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM pg_database WHERE datname = \$1\)`).
		WithArgs("notifuse_system").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(exists))
}

func TestEnsureSystemDatabase(t *testing.T) {
	t.Run("creates a missing system database when DB_EXPECT_EXISTING is off", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		expectSystemDatabaseLookup(mock, false)
		mock.ExpectExec(`CREATE DATABASE "notifuse_system"`).WillReturnResult(sqlmock.NewResult(0, 0))

		assert.NoError(t, ensureSystemDatabase(db, "notifuse_system", false))
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("creates a mixed-case name under that exact name", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM pg_database WHERE datname = \$1\)`).
			WithArgs("Notifuse").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
		mock.ExpectExec(`CREATE DATABASE "Notifuse"`).WillReturnResult(sqlmock.NewResult(0, 0))

		assert.NoError(t, ensureSystemDatabase(db, "Notifuse", false))
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("refuses a missing system database when DB_EXPECT_EXISTING is on, and creates nothing", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		expectSystemDatabaseLookup(mock, false)
		// No CREATE DATABASE expected: sqlmock fails an unexpected Exec.

		err = ensureSystemDatabase(db, "notifuse_system", true)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "the system database notifuse_system does not exist")
		assert.Contains(t, err.Error(), "DB_EXPECT_EXISTING")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("leaves an existing system database alone when DB_EXPECT_EXISTING is on", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		expectSystemDatabaseLookup(mock, true)

		assert.NoError(t, ensureSystemDatabase(db, "notifuse_system", true))
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestRequireExistingInstallation(t *testing.T) {
	regclass := `SELECT to_regclass\('settings'\) IS NOT NULL`
	version := `SELECT EXISTS\(SELECT 1 FROM settings WHERE key = 'db_version'\)`

	t.Run("accepts a database that records its version", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		mock.ExpectQuery(regclass).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
		mock.ExpectQuery(version).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

		assert.NoError(t, RequireExistingInstallation(db, "notifuse_system"))
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("refuses an empty database", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		mock.ExpectQuery(regclass).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

		err = RequireExistingInstallation(db, "notifuse_system")
		assert.ErrorContains(t, err, "the system database notifuse_system holds no Notifuse installation")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("refuses a settings table without a version", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		mock.ExpectQuery(regclass).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
		mock.ExpectQuery(version).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

		err = RequireExistingInstallation(db, "notifuse_system")
		assert.ErrorContains(t, err, "holds no Notifuse installation")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("returns a failed query", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		mock.ExpectQuery(regclass).WillReturnError(errors.New("connection reset"))

		err = RequireExistingInstallation(db, "notifuse_system")
		assert.ErrorContains(t, err, "failed to check the system database notifuse_system: connection reset")
	})
}
