package repository

import (
	"context"
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

func TestWorkspaceRepository_DropWorkspaceDatabase(t *testing.T) {
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

		require.NoError(t, repo.dropWorkspaceDatabase(context.Background(), workspaceID))
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("returns the drop error", func(t *testing.T) {
		repo, mock, cleanup := newRepo(t, "notifuse")
		defer cleanup()

		mock.ExpectExec(regexp.QuoteMeta(forceDropQuery)).
			WillReturnError(errors.New("permission denied"))

		err := repo.dropWorkspaceDatabase(context.Background(), workspaceID)
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

		require.NoError(t, repo.dropWorkspaceDatabase(context.Background(), workspaceID))
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

		require.NoError(t, repo.dropWorkspaceDatabase(context.Background(), workspaceID))
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

		err := repo.dropWorkspaceDatabase(context.Background(), workspaceID)
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

		err := repo.dropWorkspaceDatabase(context.Background(), workspaceID)
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

		require.NoError(t, repo.dropWorkspaceDatabase(context.Background(), workspaceID))
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("does not fall back on unrelated errors", func(t *testing.T) {
		repo, mock, cleanup := newRepo(t, "notifuse")
		defer cleanup()

		mock.ExpectExec(regexp.QuoteMeta(forceDropQuery)).
			WillReturnError(&pq.Error{Code: pq.ErrorCode("55006"), Message: "database is being accessed by other users"})

		err := repo.dropWorkspaceDatabase(context.Background(), workspaceID)
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

		require.NoError(t, repo.dropWorkspaceDatabase(ctx, workspaceID))
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

		require.NoError(t, repo.dropWorkspaceDatabase(ctx, workspaceID))
		require.NoError(t, mock.ExpectationsWereMet())
	})

	// The database is created under the name as written, so the drop targets that same
	// name: lowering it with Unicode rules would miss a prefix with a non-ASCII capital.
	t.Run("drops the database under the name as written", func(t *testing.T) {
		repo, mock, cleanup := newRepo(t, "notifuseÜ")
		defer cleanup()

		asWritten := fmt.Sprintf(`DROP DATABASE IF EXISTS %s WITH (FORCE)`, pq.QuoteIdentifier("notifuseÜ_ws_testworkspace"))
		mock.ExpectExec(regexp.QuoteMeta(asWritten)).
			WillReturnResult(sqlmock.NewResult(0, 0))

		require.NoError(t, repo.dropWorkspaceDatabase(context.Background(), workspaceID))
		require.NoError(t, mock.ExpectationsWereMet())
	})

	// No database can exist under a name the create refuses, so nothing is dropped.
	t.Run("refuses a name no database can have", func(t *testing.T) {
		for _, tc := range []struct {
			prefix      string
			workspaceID string
			expected    string
		}{
			{"Notifuse", workspaceID, "workspace database name must be lowercase: Notifuse_ws_testworkspace"},
			{"notifuse", "x template template0", `invalid workspace ID for a database name: "x template template0"`},
		} {
			repo, mock, cleanup := newRepo(t, tc.prefix)

			err := repo.dropWorkspaceDatabase(context.Background(), tc.workspaceID)
			assert.EqualError(t, err, tc.expected)
			require.NoError(t, mock.ExpectationsWereMet())
			cleanup()
		}
	})

	t.Run("replaces hyphens in the workspace ID", func(t *testing.T) {
		repo, mock, cleanup := newRepo(t, "notifuse")
		defer cleanup()

		hyphenated := fmt.Sprintf(`DROP DATABASE IF EXISTS %s WITH (FORCE)`, pq.QuoteIdentifier("notifuse_ws_test_ws_1"))
		mock.ExpectExec(regexp.QuoteMeta(hyphenated)).
			WillReturnResult(sqlmock.NewResult(0, 0))

		require.NoError(t, repo.dropWorkspaceDatabase(context.Background(), "test-ws-1"))
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
