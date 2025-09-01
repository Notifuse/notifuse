package database

import (
	"database/sql"
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
		for range schema.MigrationStatements {
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
		for range schema.MigrationStatements {
			mock.ExpectExec("").WillReturnResult(sqlmock.NewResult(0, 0))
		}

		// Root user doesn't exist
		mock.ExpectQuery("SELECT EXISTS").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

		// Expect insert
		mock.ExpectExec("INSERT INTO users").
			WillReturnResult(sqlmock.NewResult(1, 1))

		err = InitializeDatabase(db, "admin@example.com")
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("skips root user creation if already exists", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		// Setup expectations for table creation
		for range schema.TableDefinitions {
			mock.ExpectExec("").WillReturnResult(sqlmock.NewResult(0, 0))
		}

		// Setup expectations for migration statements
		for range schema.MigrationStatements {
			mock.ExpectExec("").WillReturnResult(sqlmock.NewResult(0, 0))
		}

		// Root user already exists
		mock.ExpectQuery("SELECT EXISTS").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

		// No insert should be made

		err = InitializeDatabase(db, "admin@example.com")
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("handles table creation error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		// First table creation fails
		mock.ExpectExec("").WillReturnError(sql.ErrConnDone)

		err = InitializeDatabase(db, "admin@example.com")
		assert.Error(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("handles root user query error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		// Setup expectations for table creation
		for range schema.TableDefinitions {
			mock.ExpectExec("").WillReturnResult(sqlmock.NewResult(0, 0))
		}

		// Setup expectations for migration statements
		for range schema.MigrationStatements {
			mock.ExpectExec("").WillReturnResult(sqlmock.NewResult(0, 0))
		}

		// Query fails
		mock.ExpectQuery("SELECT EXISTS").
			WillReturnError(sql.ErrConnDone)

		err = InitializeDatabase(db, "admin@example.com")
		assert.Error(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("handles root user insertion error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		// Setup expectations for table creation
		for range schema.TableDefinitions {
			mock.ExpectExec("").WillReturnResult(sqlmock.NewResult(0, 0))
		}

		// Setup expectations for migration statements
		for range schema.MigrationStatements {
			mock.ExpectExec("").WillReturnResult(sqlmock.NewResult(0, 0))
		}

		// Root user doesn't exist
		mock.ExpectQuery("SELECT EXISTS").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

		// Insert fails
		mock.ExpectExec("INSERT INTO users").
			WillReturnError(sql.ErrConnDone)

		err = InitializeDatabase(db, "admin@example.com")
		assert.Error(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestCleanDatabase(t *testing.T) {
	t.Run("drops tables successfully", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		// Setup expectations for table drops (in reverse order)
		for range schema.TableNames {
			mock.ExpectExec("DROP TABLE IF EXISTS").
				WillReturnResult(sqlmock.NewResult(0, 0))
		}

		// Add expectation for the separate webhook_events table drop
		mock.ExpectExec("DROP TABLE IF EXISTS webhook_events CASCADE").
			WillReturnResult(sqlmock.NewResult(0, 0))

		err = CleanDatabase(db)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("handles table drop error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		// First table drop fails
		mock.ExpectExec("DROP TABLE IF EXISTS").
			WillReturnError(sql.ErrConnDone)

		err = CleanDatabase(db)
		assert.Error(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestInitializeWorkspaceDatabase(t *testing.T) {
	t.Run("creates workspace tables successfully", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		// Expect generic table creation for all tables without checking the specific query
		// This is more maintainable as the implementation might change
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS contacts").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_contacts_email").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_contacts_external_id").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS lists").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS contact_lists").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS templates").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS broadcasts").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS message_history").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_message_history_contact_email").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_message_history_broadcast_id").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_message_history_template_id").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_message_history_created_at_id").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS transactional_notifications").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS webhook_events").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS webhook_events_message_id_idx").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS webhook_events_type_idx").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS webhook_events_timestamp_idx").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS webhook_events_recipient_email_idx").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_broadcasts_status_testing").
			WillReturnResult(sqlmock.NewResult(0, 0))

		err = InitializeWorkspaceDatabase(db)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("handles table creation error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		// Table creation fails
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS contacts").
			WillReturnError(sql.ErrConnDone)

		err = InitializeWorkspaceDatabase(db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create workspace table")
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}
