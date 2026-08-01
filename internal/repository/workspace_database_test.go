package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/repository/testutil"
)

// testWorkspaceRepository is a test implementation that wraps the real repository
// and allows simulating specific errors
type testWorkspaceRepository struct {
	domain.WorkspaceRepository
	createDatabaseError error
	createDatabaseFunc  func(ctx context.Context, workspaceID string) error
}

// Create overrides the Create method to handle the database creation error
func (r *testWorkspaceRepository) Create(ctx context.Context, workspace *domain.Workspace) error {
	// Call the underlying repository's Create method
	err := r.WorkspaceRepository.Create(ctx, workspace)
	if err != nil {
		return err
	}

	// If there was no error but we want to simulate a database creation error
	if r.createDatabaseError != nil {
		return r.createDatabaseError
	}

	return nil
}

// CreateDatabase overrides the CreateDatabase method to use our custom function
func (r *testWorkspaceRepository) CreateDatabase(ctx context.Context, workspaceID string) error {
	if r.createDatabaseFunc != nil {
		return r.createDatabaseFunc(ctx, workspaceID)
	}
	if r.createDatabaseError != nil {
		return r.createDatabaseError
	}
	return nil
}

// Update overrides the Update method to handle errors
func (r *testWorkspaceRepository) Update(ctx context.Context, workspace *domain.Workspace) error {
	if workspace.Name == "" {
		return fmt.Errorf("workspace not found")
	}
	err := r.WorkspaceRepository.Update(ctx, workspace)
	if err != nil {
		return err
	}
	return nil
}

// Delete overrides the Delete method to handle errors
func (r *testWorkspaceRepository) Delete(ctx context.Context, workspaceID string) error {
	if workspaceID == "" {
		return fmt.Errorf("workspace not found")
	}
	err := r.WorkspaceRepository.Delete(ctx, workspaceID)
	if err != nil {
		return err
	}
	return nil
}

func TestWorkspaceRepository_CreateDatabase(t *testing.T) {
	_, _, cleanup := testutil.SetupMockDB(t)
	defer cleanup()

	// Test using a custom mock repository to test error handling
	t.Run("database creation error", func(t *testing.T) {
		// Create a mock repo that returns an error
		mockRepo := &testWorkspaceRepository{
			createDatabaseError: errors.New("database already exists"),
		}

		err := mockRepo.CreateDatabase(context.Background(), "testworkspace")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "database already exists")
	})

	t.Run("successful database creation", func(t *testing.T) {
		// Create a mock repo that succeeds
		mockRepo := &testWorkspaceRepository{}

		err := mockRepo.CreateDatabase(context.Background(), "testworkspace")
		require.NoError(t, err)
	})

	t.Run("workspace with hyphens", func(t *testing.T) {
		// Create a mock repo that succeeds
		mockRepo := &testWorkspaceRepository{}

		workspaceIDWithHyphens := "test-workspace-123"
		err := mockRepo.CreateDatabase(context.Background(), workspaceIDWithHyphens)
		require.NoError(t, err)
	})
}

func TestWorkspaceRepository_DeleteDatabase(t *testing.T) {
	newRepo := func(t *testing.T, prefix string) (*workspaceRepository, sqlmock.Sqlmock, func()) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		dbConfig := &config.DatabaseConfig{
			Host:     "localhost",
			Port:     5432,
			User:     "postgres",
			Password: "password",
			DBName:   "notifuse_system",
			Prefix:   prefix,
		}
		connMgr := newMockConnectionManager(db)
		repo := NewWorkspaceRepository(db, dbConfig, "secret-key", connMgr).(*workspaceRepository)
		return repo, mock, cleanup
	}

	const workspaceID = "testworkspace"
	const dbName = "notifuse_ws_testworkspace"
	forceDropQuery := fmt.Sprintf(`DROP DATABASE IF EXISTS %s WITH (FORCE)`, pq.QuoteIdentifier(dbName))

	t.Run("drops the database with FORCE", func(t *testing.T) {
		repo, mock, cleanup := newRepo(t, "notifuse")
		defer cleanup()

		mock.ExpectExec(regexp.QuoteMeta(forceDropQuery)).
			WillReturnResult(sqlmock.NewResult(0, 0))

		require.NoError(t, repo.DeleteDatabase(context.Background(), workspaceID))
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("returns the drop error", func(t *testing.T) {
		repo, mock, cleanup := newRepo(t, "notifuse")
		defer cleanup()

		mock.ExpectExec(regexp.QuoteMeta(forceDropQuery)).
			WillReturnError(errors.New("permission denied"))

		err := repo.DeleteDatabase(context.Background(), workspaceID)
		require.Error(t, err)
		assert.ErrorContains(t, err, "permission denied")
		// The database name has to reach the operator: an unattributed privilege
		// error is what made the original bug so hard to place.
		assert.ErrorContains(t, err, dbName)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	// Guards the shape the plain expectations cannot catch: a REVOKE concatenated
	// into the same statement string as the drop. A REVOKE issued as its own Exec is
	// already rejected by sqlmock as an unexpected call.
	t.Run("never revokes privileges on the system database", func(t *testing.T) {
		// Record the statements sqlmock compares against a pending expectation. This
		// is not necessarily every statement sent, so it complements rather than
		// replaces the strict expectations above.
		var executed []string
		matcher := sqlmock.QueryMatcherFunc(func(expectedSQL, actualSQL string) error {
			executed = append(executed, actualSQL)
			return sqlmock.QueryMatcherRegexp.Match(expectedSQL, actualSQL)
		})
		db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(matcher))
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		dbConfig := &config.DatabaseConfig{
			User:   "postgres",
			DBName: "notifuse_system",
			Prefix: "notifuse",
		}
		repo := NewWorkspaceRepository(db, dbConfig, "secret-key", newMockConnectionManager(db)).(*workspaceRepository)

		mock.ExpectExec(regexp.QuoteMeta(forceDropQuery)).
			WillReturnResult(sqlmock.NewResult(0, 0))

		require.NoError(t, repo.DeleteDatabase(context.Background(), workspaceID))
		require.NoError(t, mock.ExpectationsWereMet())

		require.NotEmpty(t, executed, "expected the repository to issue at least one statement")
		for _, q := range executed {
			assert.NotContains(t, q, "REVOKE", "workspace deletion must not revoke privileges")
			assert.NotContains(t, q, "ALL TABLES", "revokes on ALL TABLES resolve against the system database")
			assert.NotContains(t, q, "ALL SEQUENCES", "revokes on ALL SEQUENCES resolve against the system database")
		}
	})

	t.Run("falls back to terminate and plain drop on PostgreSQL older than 13", func(t *testing.T) {
		repo, mock, cleanup := newRepo(t, "notifuse")
		defer cleanup()

		// PostgreSQL below 13 rejects WITH (FORCE) while parsing.
		mock.ExpectExec(regexp.QuoteMeta(forceDropQuery)).
			WillReturnError(&pq.Error{Code: pq.ErrorCode("42601"), Message: `syntax error at or near "WITH"`})

		mock.ExpectExec(`SELECT pg_terminate_backend\(pid\)`).
			WithArgs(dbName).
			WillReturnResult(sqlmock.NewResult(0, 0))

		mock.ExpectExec(regexp.QuoteMeta(fmt.Sprintf(`DROP DATABASE IF EXISTS %s`, pq.QuoteIdentifier(dbName)))).
			WillReturnResult(sqlmock.NewResult(0, 0))

		require.NoError(t, repo.DeleteDatabase(context.Background(), workspaceID))
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("returns the terminate error from the pre-13 fallback", func(t *testing.T) {
		repo, mock, cleanup := newRepo(t, "notifuse")
		defer cleanup()

		mock.ExpectExec(regexp.QuoteMeta(forceDropQuery)).
			WillReturnError(&pq.Error{Code: pq.ErrorCode("42601")})
		mock.ExpectExec(`SELECT pg_terminate_backend\(pid\)`).
			WithArgs(dbName).
			WillReturnError(errors.New("insufficient privilege"))

		err := repo.DeleteDatabase(context.Background(), workspaceID)
		require.Error(t, err)
		assert.ErrorContains(t, err, "insufficient privilege")
		assert.ErrorContains(t, err, dbName)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("returns the drop error from the pre-13 fallback", func(t *testing.T) {
		repo, mock, cleanup := newRepo(t, "notifuse")
		defer cleanup()

		mock.ExpectExec(regexp.QuoteMeta(forceDropQuery)).
			WillReturnError(&pq.Error{Code: pq.ErrorCode("42601")})
		mock.ExpectExec(`SELECT pg_terminate_backend\(pid\)`).
			WithArgs(dbName).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(regexp.QuoteMeta(fmt.Sprintf(`DROP DATABASE IF EXISTS %s`, pq.QuoteIdentifier(dbName)))).
			WillReturnError(errors.New("being accessed by other users"))

		err := repo.DeleteDatabase(context.Background(), workspaceID)
		require.Error(t, err)
		assert.ErrorContains(t, err, "being accessed by other users")
		require.NoError(t, mock.ExpectationsWereMet())
	})

	// Failing to close the pool must not abort the deletion: the drop terminates any
	// surviving backend anyway.
	t.Run("drops the database even if closing the pool fails", func(t *testing.T) {
		db, mock, cleanup := testutil.SetupMockDB(t)
		defer cleanup()

		connMgr := newMockConnectionManager(db)
		connMgr.closeErr = errors.New("pool already closed")
		repo := NewWorkspaceRepository(db,
			&config.DatabaseConfig{User: "postgres", DBName: "notifuse_system", Prefix: "notifuse"},
			"secret-key", connMgr).(*workspaceRepository)

		mock.ExpectExec(regexp.QuoteMeta(forceDropQuery)).
			WillReturnResult(sqlmock.NewResult(0, 0))

		require.NoError(t, repo.DeleteDatabase(context.Background(), workspaceID))
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("does not fall back on unrelated errors", func(t *testing.T) {
		repo, mock, cleanup := newRepo(t, "notifuse")
		defer cleanup()

		mock.ExpectExec(regexp.QuoteMeta(forceDropQuery)).
			WillReturnError(&pq.Error{Code: pq.ErrorCode("55006"), Message: "database is being accessed by other users"})

		err := repo.DeleteDatabase(context.Background(), workspaceID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "being accessed by other users")
		require.NoError(t, mock.ExpectationsWereMet())
	})

	// An aborted DROP DATABASE can leave the database marked invalid and unusable,
	// so a cancelled caller context must not cancel the drop.
	t.Run("completes the drop even when the caller context is already cancelled", func(t *testing.T) {
		repo, mock, cleanup := newRepo(t, "notifuse")
		defer cleanup()

		mock.ExpectExec(regexp.QuoteMeta(forceDropQuery)).
			WillReturnResult(sqlmock.NewResult(0, 0))

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		require.NoError(t, repo.DeleteDatabase(ctx, workspaceID))
		require.NoError(t, mock.ExpectationsWereMet())
	})

	// The production scenario is cancellation arriving while the drop is in flight,
	// which is when an aborted DROP DATABASE leaves the database marked invalid.
	t.Run("completes the drop when the caller context is cancelled mid-statement", func(t *testing.T) {
		repo, mock, cleanup := newRepo(t, "notifuse")
		defer cleanup()

		mock.ExpectExec(regexp.QuoteMeta(forceDropQuery)).
			WillDelayFor(200 * time.Millisecond).
			WillReturnResult(sqlmock.NewResult(0, 0))

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			time.Sleep(20 * time.Millisecond)
			cancel()
		}()

		require.NoError(t, repo.DeleteDatabase(ctx, workspaceID))
		require.NoError(t, mock.ExpectationsWereMet())
	})

	// EnsureWorkspaceDatabaseExists creates the database with an unquoted identifier,
	// so PostgreSQL stores it lower-cased. The drop has to target that same name.
	t.Run("lower-cases the database name to match unquoted creation", func(t *testing.T) {
		repo, mock, cleanup := newRepo(t, "Notifuse")
		defer cleanup()

		lowered := fmt.Sprintf(`DROP DATABASE IF EXISTS %s WITH (FORCE)`, pq.QuoteIdentifier("notifuse_ws_testworkspace"))
		mock.ExpectExec(regexp.QuoteMeta(lowered)).
			WillReturnResult(sqlmock.NewResult(0, 0))

		require.NoError(t, repo.DeleteDatabase(context.Background(), workspaceID))
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("replaces hyphens in the workspace ID", func(t *testing.T) {
		repo, mock, cleanup := newRepo(t, "notifuse")
		defer cleanup()

		hyphenated := fmt.Sprintf(`DROP DATABASE IF EXISTS %s WITH (FORCE)`, pq.QuoteIdentifier("notifuse_ws_test_ws_1"))
		mock.ExpectExec(regexp.QuoteMeta(hyphenated)).
			WillReturnResult(sqlmock.NewResult(0, 0))

		require.NoError(t, repo.DeleteDatabase(context.Background(), "test-ws-1"))
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestWorkspaceRepository_GetConnection(t *testing.T) {
	// Create a test database config
	dbConfig := &config.DatabaseConfig{
		Host:     "localhost",
		Port:     5432,
		User:     "postgres",
		Password: "postgres",
		DBName:   "test_db",
		Prefix:   "test",
	}

	// Create a mock database
	mockDB, _, cleanup := testutil.SetupMockDB(t)
	defer cleanup()

	// Create a repository instance
	connMgr := newMockConnectionManager(mockDB)
	repo := NewWorkspaceRepository(mockDB, dbConfig, "secret-key", connMgr).(*workspaceRepository)

	ctx := context.Background()
	workspaceID := "test-workspace"

	// Test with a successful mock workspace DB connection
	mockWorkspaceDB, _, mockWorkspaceCleanup := testutil.SetupMockDB(t)
	defer mockWorkspaceCleanup()

	// Store the mock connection in the connection manager
	connMgr.AddWorkspaceDB(workspaceID, mockWorkspaceDB)

	// Test case 1: Getting a connection that already exists
	db1, err := repo.GetConnection(ctx, workspaceID)
	assert.NoError(t, err)
	assert.Equal(t, mockWorkspaceDB, db1)

	// Test case 2: Non-existent workspace returns the system DB (mock fallback)
	db2, err := repo.GetConnection(ctx, "non-existent-workspace")
	assert.NoError(t, err)
	assert.NotNil(t, db2) // Mock returns system DB as fallback

	// Test case 3: Add a workspace connection to the manager and verify we can get it
	newWorkspaceDB, _, newWorkspaceCleanup := testutil.SetupMockDB(t)
	defer newWorkspaceCleanup()

	newWorkspaceID := "new-workspace"
	connMgr.AddWorkspaceDB(newWorkspaceID, newWorkspaceDB)

	// GetConnection should return the workspace DB
	db3, err := repo.GetConnection(context.Background(), newWorkspaceID)
	assert.NoError(t, err)
	assert.Equal(t, newWorkspaceDB, db3)
}

// Define a mocking variable for the EnsureWorkspaceDatabaseExists function
var mockEnsureWorkspaceDB func(cfg *config.DatabaseConfig, workspaceID string) error

// Test the actual CreateDatabase method implementation
func TestWorkspaceRepository_CreateDatabaseMethod(t *testing.T) {
	// Create a mock DB and config
	db, _, cleanup := testutil.SetupMockDB(t)
	defer cleanup()

	dbConfig := &config.DatabaseConfig{
		Host:     "localhost",
		Port:     5432,
		User:     "postgres",
		Password: "password",
		DBName:   "notifuse_system",
		Prefix:   "notifuse",
	}

	// Create a custom repo that uses our mock function instead of the real one
	repo := &mockEnsureDBRepository{
		db:       db,
		dbConfig: dbConfig,
	}

	// Test successful database creation
	t.Run("successful database creation", func(t *testing.T) {
		ensureCalled := false
		mockEnsureWorkspaceDB = func(cfg *config.DatabaseConfig, workspaceID string) error {
			ensureCalled = true
			require.Equal(t, dbConfig, cfg)
			require.Equal(t, "testworkspace", workspaceID)
			return nil
		}

		err := repo.CreateDatabase(context.Background(), "testworkspace")
		require.NoError(t, err)
		require.True(t, ensureCalled, "EnsureWorkspaceDatabaseExists should be called")
	})

	// Test database creation error
	t.Run("database creation error", func(t *testing.T) {
		ensureCalled := false
		mockEnsureWorkspaceDB = func(cfg *config.DatabaseConfig, workspaceID string) error {
			ensureCalled = true
			return fmt.Errorf("database creation failed")
		}

		err := repo.CreateDatabase(context.Background(), "testworkspace")
		require.Error(t, err)
		require.True(t, ensureCalled, "EnsureWorkspaceDatabaseExists should be called")
		require.Contains(t, err.Error(), "failed to create and initialize workspace database")
	})
}

// mockEnsureDBRepository is a special repository for testing the CreateDatabase method
type mockEnsureDBRepository struct {
	domain.WorkspaceRepository
	db       *sql.DB
	dbConfig *config.DatabaseConfig
}

// CreateDatabase implements the WorkspaceRepository interface
func (r *mockEnsureDBRepository) CreateDatabase(ctx context.Context, workspaceID string) error {
	// Use our mockEnsureWorkspaceDB instead of database.EnsureWorkspaceDatabaseExists
	if err := mockEnsureWorkspaceDB(r.dbConfig, workspaceID); err != nil {
		return fmt.Errorf("failed to create and initialize workspace database: %w", err)
	}
	return nil
}
