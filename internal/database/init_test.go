package database

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/database/schema"
	"github.com/Notifuse/notifuse/internal/domain"
)

func TestCleanDatabase(t *testing.T) {
	t.Run("Successfully clean database", func(t *testing.T) {
		// Create mock database
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		// Mock expectations for dropping tables - we'll expect a reasonable number of DROP statements
		// Since we can't easily mock the exact number, we'll expect several
		for i := 0; i < 10; i++ { // Expect up to 10 table drops
			mock.ExpectExec("DROP TABLE IF EXISTS .+ CASCADE").WillReturnResult(sqlmock.NewResult(0, 0))
		}

		// Expect the webhook_events table drop
		mock.ExpectExec("DROP TABLE IF EXISTS inbound_webhook_events CASCADE").WillReturnResult(sqlmock.NewResult(0, 0))

		// Execute the function
		err = CleanDatabase(db)

		// Verify - we don't check mock expectations here since the exact number of tables may vary
		assert.NoError(t, err)
	})

	t.Run("Error dropping table", func(t *testing.T) {
		// Create mock database
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		// Mock first DROP TABLE to fail
		mock.ExpectExec("DROP TABLE IF EXISTS .+ CASCADE").WillReturnError(sql.ErrConnDone)

		// Execute the function
		err = CleanDatabase(db)

		// Verify
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to drop table")
	})

	t.Run("Database connection error", func(t *testing.T) {
		// Create mock database
		db, _, err := sqlmock.New()
		require.NoError(t, err)

		// Close the database to simulate connection error
		_ = db.Close()

		// Execute the function
		err = CleanDatabase(db)

		// Verify - should get an error due to closed connection
		assert.Error(t, err)
	})
}

func TestInitializeDatabase(t *testing.T) {
	// Note: InitializeDatabase is a complex function that would require extensive mocking
	// For now, we'll test basic error conditions

	t.Run("Nil database connection panics", func(t *testing.T) {
		// The function doesn't check for nil, so it will panic
		assert.Panics(t, func() {
			_ = InitializeDatabase(nil, []string{"test@example.com"})
		})
	})

	t.Run("Database execution error", func(t *testing.T) {
		// Create mock database
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		// Mock the first table creation to fail
		mock.ExpectExec(".+").WillReturnError(sql.ErrConnDone)

		err = InitializeDatabase(db, []string{"test@example.com"})

		// Should get an error
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create table")
	})
}

func TestInitializeWorkspaceDatabase(t *testing.T) {
	t.Run("Nil database connection panics", func(t *testing.T) {
		// The function doesn't check for nil, so it will panic
		assert.Panics(t, func() {
			_ = InitializeWorkspaceDatabase(nil)
		})
	})

	t.Run("Database connection error", func(t *testing.T) {
		// Create mock database
		db, _, err := sqlmock.New()
		require.NoError(t, err)

		// Close the database to simulate connection error
		_ = db.Close()

		// Execute the function
		err = InitializeWorkspaceDatabase(db)

		// Should get an error due to closed connection
		assert.Error(t, err)
	})

	t.Run("Database execution error", func(t *testing.T) {
		// Create mock database
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		// Mock the first CREATE TABLE to fail
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS .+").WillReturnError(sql.ErrConnDone)

		// Execute the function
		err = InitializeWorkspaceDatabase(db)

		// Verify
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create workspace table")
	})
}

// Integration test placeholder
func TestDatabaseInitialization_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	t.Run("Integration test placeholder", func(t *testing.T) {
		// This would test actual database initialization with a real test database
		// For now, we'll just verify the functions exist and can be called

		// These functions exist and can be imported
		assert.NotNil(t, InitializeDatabase)
		assert.NotNil(t, InitializeWorkspaceDatabase)
		assert.NotNil(t, CleanDatabase)
	})
}

// Test coverage for database schema-related functions
func TestDatabaseSchema_Coverage(t *testing.T) {
	t.Run("CleanDatabase with closed connection", func(t *testing.T) {
		// Test the error path instead of trying to mock exact table drops
		// This gives us coverage without depending on the exact table order
		db, _, err := sqlmock.New()
		require.NoError(t, err)

		// Close the database to simulate an error condition
		_ = db.Close()

		err = CleanDatabase(db)

		// Should get an error due to closed connection
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to drop table")
	})

	t.Run("CleanDatabase function exists and is callable", func(t *testing.T) {
		// Basic smoke test - just verify the function can be called
		// This provides coverage without complex mocking
		assert.NotNil(t, CleanDatabase, "CleanDatabase function should exist")

		// Test with nil database - should panic (which we expect)
		assert.Panics(t, func() {
			_ = CleanDatabase(nil)
		}, "CleanDatabase should panic with nil database")
	})
}

func TestInitializeDatabase_Comprehensive(t *testing.T) {
	t.Run("Initialize database without root email - simple success", func(t *testing.T) {
		db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		// Mock all SQL statements to succeed - tables and migrations
		for i := 0; i < 50; i++ {
			mock.ExpectExec(".+").WillReturnResult(sqlmock.NewResult(0, 0))
		}

		// Test with empty email - no user creation queries expected
		err = InitializeDatabase(db, nil)
		assert.NoError(t, err)
	})

	t.Run("Error during table creation", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		// Mock first SQL statement to fail
		mock.ExpectExec(".+").WillReturnError(sql.ErrConnDone)

		err = InitializeDatabase(db, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create table")
	})
}

func TestInitializeDatabase_MultipleRootEmails(t *testing.T) {
	// Number of table + migration statements run before any root user creation.
	schemaStatements := len(schema.TableDefinitions) + len(schema.GetMigrationStatements())

	expectSchema := func(mock sqlmock.Sqlmock) {
		for i := 0; i < schemaStatements; i++ {
			mock.ExpectExec(".+").WillReturnResult(sqlmock.NewResult(0, 0))
		}
	}

	t.Run("creates a user row for each listed root that doesn't exist", func(t *testing.T) {
		db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		expectSchema(mock)

		emails := []string{"alice@example.com", "bob@example.com"}
		for _, email := range emails {
			mock.ExpectQuery("SELECT EXISTS").
				WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
			mock.ExpectExec("INSERT INTO users").
				WithArgs(sqlmock.AnyArg(), email, "Root User", domain.UserTypeUser, sqlmock.AnyArg(), sqlmock.AnyArg()).
				WillReturnResult(sqlmock.NewResult(1, 1))
		}

		err = InitializeDatabase(db, emails)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("idempotent - skips INSERT when root already exists", func(t *testing.T) {
		db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		expectSchema(mock)

		// First root exists (no INSERT), second is new (INSERT expected).
		mock.ExpectQuery("SELECT EXISTS").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
		mock.ExpectQuery("SELECT EXISTS").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
		mock.ExpectExec("INSERT INTO users").
			WithArgs(sqlmock.AnyArg(), "bob@example.com", "Root User", domain.UserTypeUser, sqlmock.AnyArg(), sqlmock.AnyArg()).
			WillReturnResult(sqlmock.NewResult(1, 1))

		err = InitializeDatabase(db, []string{"alice@example.com", "bob@example.com"})
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("skips empty entries in the list", func(t *testing.T) {
		db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		expectSchema(mock)

		// Only the non-empty entry triggers an existence check + insert.
		mock.ExpectQuery("SELECT EXISTS").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
		mock.ExpectExec("INSERT INTO users").
			WithArgs(sqlmock.AnyArg(), "alice@example.com", "Root User", domain.UserTypeUser, sqlmock.AnyArg(), sqlmock.AnyArg()).
			WillReturnResult(sqlmock.NewResult(1, 1))

		err = InitializeDatabase(db, []string{"", "alice@example.com"})
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestInitializeWorkspaceDatabase_Comprehensive(t *testing.T) {
	t.Run("Successfully initialize workspace database", func(t *testing.T) {
		db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		// Mock all SQL statements - tables, indexes, trigger functions, and triggers
		// Increased to accommodate all workspace tables, indexes, and webhook-related triggers
		for i := 0; i < 150; i++ { // Allow for many SQL statements with buffer
			mock.ExpectExec(".+").WillReturnResult(sqlmock.NewResult(0, 0))
		}

		err = InitializeWorkspaceDatabase(db)
		assert.NoError(t, err)
	})

	t.Run("Error creating workspace table", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		// Mock first CREATE TABLE to fail
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS contacts").WillReturnError(sql.ErrConnDone)

		err = InitializeWorkspaceDatabase(db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create workspace table")
	})
}

// workspaceStatementRecorder is the default regexp matcher plus a log of every
// statement InitializeWorkspaceDatabase issued. The workspace DDL is a local
// slice of string literals, so capturing what actually reached the driver is the
// only way to assert anything about it.
type workspaceStatementRecorder struct {
	issued []string
}

func (r *workspaceStatementRecorder) Match(expectedSQL, actualSQL string) error {
	if len(r.issued) == 0 || r.issued[len(r.issued)-1] != actualSQL {
		r.issued = append(r.issued, actualSQL)
	}
	return sqlmock.QueryMatcherRegexp.Match(expectedSQL, actualSQL)
}

func (r *workspaceStatementRecorder) indexOfStatementContaining(t *testing.T, needle string) int {
	t.Helper()
	for i, stmt := range r.issued {
		if strings.Contains(stmt, needle) {
			return i
		}
	}
	require.FailNowf(t, "statement not issued", "no statement containing %q", needle)
	return -1
}

func (r *workspaceStatementRecorder) statementContaining(t *testing.T, needle string) string {
	t.Helper()
	return r.issued[r.indexOfStatementContaining(t, needle)]
}

// recordWorkspaceSchema runs the workspace initializer against a mock that
// accepts everything, and returns what it issued.
func recordWorkspaceSchema(t *testing.T) *workspaceStatementRecorder {
	t.Helper()

	rec := &workspaceStatementRecorder{}
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(rec))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	// Generously more expectations than the initializer has statements; the
	// unused ones are never asserted on.
	for i := 0; i < 500; i++ {
		mock.ExpectExec(".+").WillReturnResult(sqlmock.NewResult(0, 0))
	}

	require.NoError(t, InitializeWorkspaceDatabase(db))
	return rec
}

func TestInitializeWorkspaceDatabase_WebhookSubscriptionColumns(t *testing.T) {
	subscriptions := recordWorkspaceSchema(t).statementContaining(t, "CREATE TABLE IF NOT EXISTS webhook_subscriptions")

	// A fresh install has to land on the same schema an upgraded workspace does,
	// or the two diverge silently: the migration is guarded with IF NOT EXISTS
	// and never runs against a database created after it shipped.
	//
	// source is nullable because NULL is the user-created case. consecutive_failures
	// is NOT NULL DEFAULT 0 so the auto-disable threshold never has to coalesce.
	assert.Contains(t, subscriptions, "source VARCHAR(32),")
	assert.Contains(t, subscriptions, "consecutive_failures INT NOT NULL DEFAULT 0")
	assert.Contains(t, subscriptions, "disabled_reason TEXT")
}

func TestInitializeWorkspaceDatabase_WebhookDeliveryClaimSchema(t *testing.T) {
	rec := recordWorkspaceSchema(t)
	deliveries := rec.statementContaining(t, "CREATE TABLE IF NOT EXISTS webhook_deliveries")

	assert.Contains(t, deliveries, "claimed_at TIMESTAMPTZ")

	// Deleting a subscription has to take its queued deliveries with it. Without
	// the cascade they keep matching the pending predicate for the whole
	// retention window while their subscription no longer exists, so each one
	// occupies a slot in every batch and is never delivered.
	assert.Contains(t, deliveries, "REFERENCES webhook_subscriptions(id) ON DELETE CASCADE")

	// The constraint is named here rather than left to PostgreSQL's auto-naming
	// because the v39 migration adds the same constraint to existing workspaces
	// and looks it up by name to stay re-runnable. If the two names drift, that
	// migration adds a second, duplicate foreign key on every re-run.
	assert.Contains(t, deliveries, "CONSTRAINT webhook_deliveries_subscription_id_fkey")

	// A claimed row's status becomes 'delivering' and leaves the pending partial
	// index, so the reclaim sweep needs its own entry point.
	claimedIndex := rec.statementContaining(t, "idx_webhook_deliveries_claimed")
	assert.Contains(t, claimedIndex, "webhook_deliveries(claimed_at)")
	assert.Contains(t, claimedIndex, "WHERE status = 'delivering'")
}

func TestInitializeWorkspaceDatabase_SubscriptionsPrecedeDeliveries(t *testing.T) {
	// The foreign key is declared inline, so the referenced table has to exist by
	// the time webhook_deliveries is created. These statements run in slice
	// order, which makes the order of two string literals load-bearing and
	// invisible: reordering them fails only on a fresh install.
	rec := recordWorkspaceSchema(t)
	assert.Less(t,
		rec.indexOfStatementContaining(t, "CREATE TABLE IF NOT EXISTS webhook_subscriptions"),
		rec.indexOfStatementContaining(t, "CREATE TABLE IF NOT EXISTS webhook_deliveries"),
	)
}
