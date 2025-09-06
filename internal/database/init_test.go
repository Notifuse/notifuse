package database

import (
	"testing"

	"github.com/Notifuse/notifuse/internal/database/schema"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitializeDatabase(t *testing.T) {

	t.Run("creates tables successfully", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		// Setup expectations for table creation
		for range schema.TableDefinitions {
			mock.ExpectExec("").WillReturnResult(sqlmock.NewResult(0, 0))
		}

		// Setup expectations for migration statements
		for range schema.GetMigrationStatements() {
			mock.ExpectExec("").WillReturnResult(sqlmock.NewResult(0, 0))
		}

		err = InitializeDatabase(db, "")
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("creates root user if not exists", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		// Setup expectations for table creation
		for range schema.TableDefinitions {
			mock.ExpectExec("").WillReturnResult(sqlmock.NewResult(0, 0))
		}

		// Setup expectations for migration statements
		for range schema.GetMigrationStatements() {
			mock.ExpectExec("").WillReturnResult(sqlmock.NewResult(0, 0))
		}

		// Root user doesn't exist
		mock.ExpectQuery("SELECT EXISTS").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

		// Expect user creation
		mock.ExpectExec("INSERT INTO users").
			WillReturnResult(sqlmock.NewResult(1, 1))

		err = InitializeDatabase(db, "admin@example.com")
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("skips root user creation if exists", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		// Setup expectations for table creation
		for range schema.TableDefinitions {
			mock.ExpectExec("").WillReturnResult(sqlmock.NewResult(0, 0))
		}

		// Setup expectations for migration statements
		for range schema.GetMigrationStatements() {
			mock.ExpectExec("").WillReturnResult(sqlmock.NewResult(0, 0))
		}

		// Root user already exists
		mock.ExpectQuery("SELECT EXISTS").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

		err = InitializeDatabase(db, "admin@example.com")
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("handles table creation error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		// First table creation fails
		mock.ExpectExec("").WillReturnError(assert.AnError)

		err = InitializeDatabase(db, "admin@example.com")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create table")
	})

	t.Run("handles migration error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		// Setup expectations for table creation
		for range schema.TableDefinitions {
			mock.ExpectExec("").WillReturnResult(sqlmock.NewResult(0, 0))
		}

		// First migration statement fails
		mock.ExpectExec("").WillReturnError(assert.AnError)

		err = InitializeDatabase(db, "admin@example.com")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to run migration")
	})

	t.Run("handles root user check error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		// Setup expectations for table creation
		for range schema.TableDefinitions {
			mock.ExpectExec("").WillReturnResult(sqlmock.NewResult(0, 0))
		}

		// Setup expectations for migration statements
		for range schema.GetMigrationStatements() {
			mock.ExpectExec("").WillReturnResult(sqlmock.NewResult(0, 0))
		}

		// Root user check fails
		mock.ExpectQuery("SELECT EXISTS").
			WillReturnError(assert.AnError)

		err = InitializeDatabase(db, "admin@example.com")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to check root user existence")
	})

	t.Run("handles root user creation error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		// Setup expectations for table creation
		for range schema.TableDefinitions {
			mock.ExpectExec("").WillReturnResult(sqlmock.NewResult(0, 0))
		}

		// Setup expectations for migration statements
		for range schema.GetMigrationStatements() {
			mock.ExpectExec("").WillReturnResult(sqlmock.NewResult(0, 0))
		}

		// Root user doesn't exist
		mock.ExpectQuery("SELECT EXISTS").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

		// User creation fails
		mock.ExpectExec("INSERT INTO users").
			WillReturnError(assert.AnError)

		err = InitializeDatabase(db, "admin@example.com")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create root user")
	})
}

func TestInitializeWorkspaceDatabase(t *testing.T) {
	t.Run("creates workspace tables successfully", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		// There are many queries in InitializeWorkspaceDatabase
		// We'll just expect that many exec calls succeed
		for i := 0; i < 20; i++ { // Approximate number of queries
			mock.ExpectExec("").WillReturnResult(sqlmock.NewResult(0, 0))
		}

		err = InitializeWorkspaceDatabase(db)
		assert.NoError(t, err)
	})

	t.Run("handles table creation error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		// First table creation fails
		mock.ExpectExec("").WillReturnError(assert.AnError)

		err = InitializeWorkspaceDatabase(db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create workspace table")
	})
}
