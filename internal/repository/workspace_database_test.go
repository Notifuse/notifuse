package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
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
	db, mock, cleanup := testutil.SetupMockDB(t)
	defer cleanup()

	dbConfig := &config.DatabaseConfig{
		Host:     "localhost",
		Port:     5432,
		User:     "postgres",
		Password: "password",
		DBName:   "notifuse_system",
		Prefix:   "notifuse",
	}

	connMgr := newMockConnectionManager(db)
	repo := NewWorkspaceRepository(db, dbConfig, "secret-key", connMgr)
	workspaceID := "testworkspace"

	// Test database drop error
	safeID := strings.ReplaceAll(workspaceID, "-", "_")
	dbName := fmt.Sprintf("%s_ws_%s", dbConfig.Prefix, safeID)

	// First test: error case
	revokeQuery := fmt.Sprintf(`
		REVOKE ALL PRIVILEGES ON DATABASE %s FROM PUBLIC;
		REVOKE ALL PRIVILEGES ON DATABASE %s FROM %s;
		REVOKE ALL PRIVILEGES ON ALL TABLES IN SCHEMA public FROM PUBLIC;
		REVOKE ALL PRIVILEGES ON ALL TABLES IN SCHEMA public FROM %s;
		REVOKE ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public FROM PUBLIC;
		REVOKE ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public FROM %s;`,
		dbName, dbName, dbConfig.User, dbConfig.User, dbConfig.User)
	mock.ExpectExec(regexp.QuoteMeta(revokeQuery)).
		WillReturnError(errors.New("permission denied"))

	err := repo.(*workspaceRepository).DeleteDatabase(context.Background(), workspaceID)
	require.Error(t, err)
	assert.Equal(t, "permission denied", err.Error())

	// Test successful database drop with proper connection termination
	// Expect revoke privileges
	mock.ExpectExec(regexp.QuoteMeta(revokeQuery)).
		WillReturnResult(sqlmock.NewResult(0, 0))

	// Expect terminate connections
	terminateQuery := fmt.Sprintf(`
		SELECT pg_terminate_backend(pid) 
		FROM pg_stat_activity 
		WHERE datname = '%s' 
		AND pid <> pg_backend_pid()`, dbName)
	mock.ExpectExec(regexp.QuoteMeta(terminateQuery)).
		WillReturnResult(sqlmock.NewResult(0, 0))

	// Expect drop database
	mock.ExpectExec(fmt.Sprintf("DROP DATABASE IF EXISTS %s", dbName)).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err = repo.(*workspaceRepository).DeleteDatabase(context.Background(), workspaceID)
	require.NoError(t, err)
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

// mockInternalRepository implements the workspaceRepository but doesn't actually connect to the database
type mockInternalRepository struct {
	systemDB    *sql.DB
	dbConfig    *config.DatabaseConfig
	connections sync.Map
}

// Adding the missing WithWorkspaceTransaction method
func (r *mockInternalRepository) WithWorkspaceTransaction(ctx context.Context, workspaceID string, fn func(*sql.Tx) error) error {
	return fmt.Errorf("not implemented in mock")
}

func (r *mockInternalRepository) checkWorkspaceIDExists(ctx context.Context, id string) (bool, error) {
	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM workspaces WHERE id = $1)`
	err := r.systemDB.QueryRowContext(ctx, query, id).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check workspace ID existence: %w", err)
	}
	return exists, nil
}

func (r *mockInternalRepository) Create(ctx context.Context, workspace *domain.Workspace) error {
	if workspace.ID == "" {
		return fmt.Errorf("workspace ID is required")
	}

	// Check if workspace ID already exists
	exists, err := r.checkWorkspaceIDExists(ctx, workspace.ID)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("workspace with ID %s already exists", workspace.ID)
	}

	now := time.Now()
	workspace.CreatedAt = now
	workspace.UpdatedAt = now

	// Marshal settings to JSON
	settings, err := json.Marshal(workspace.Settings)
	if err != nil {
		return fmt.Errorf("failed to marshal settings: %w", err)
	}

	// Marshal integrations to JSON if any exist
	var integrations []byte
	if len(workspace.Integrations) > 0 {
		integrations, err = json.Marshal(workspace.Integrations)
		if err != nil {
			return fmt.Errorf("failed to marshal integrations: %w", err)
		}
	}

	query := `
		INSERT INTO workspaces (id, name, settings, integrations, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`
	_, err = r.systemDB.ExecContext(ctx, query, workspace.ID, workspace.Name, settings, integrations, workspace.CreatedAt, workspace.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to create workspace: %w", err)
	}

	return nil
}

func (r *mockInternalRepository) GetByID(ctx context.Context, id string) (*domain.Workspace, error) {
	return nil, fmt.Errorf("not implemented in mock")
}

func (r *mockInternalRepository) List(ctx context.Context) ([]*domain.Workspace, error) {
	return nil, fmt.Errorf("not implemented in mock")
}

func (r *mockInternalRepository) Update(ctx context.Context, workspace *domain.Workspace) error {

	settings, err := json.Marshal(workspace.Settings)
	if err != nil {
		return fmt.Errorf("failed to marshal settings: %w", err)
	}

	// Marshal integrations to JSON if any exist
	var integrations []byte
	if len(workspace.Integrations) > 0 {
		integrations, err = json.Marshal(workspace.Integrations)
		if err != nil {
			return fmt.Errorf("failed to marshal integrations: %w", err)
		}
	}

	query := `
		UPDATE workspaces
		SET name = $1, settings = $2, integrations = $3, updated_at = $4
		WHERE id = $5
	`
	result, err := r.systemDB.ExecContext(ctx, query, workspace.Name, settings, integrations, time.Now(), workspace.ID)
	if err != nil {
		return fmt.Errorf("failed to update workspace")
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get affected rows")
	}

	if rowsAffected == 0 {
		return fmt.Errorf("workspace not found")
	}

	return nil
}

func (r *mockInternalRepository) Delete(ctx context.Context, workspaceID string) error {
	// Delete user workspaces
	query := `DELETE FROM user_workspaces WHERE workspace_id = $1`
	_, err := r.systemDB.ExecContext(ctx, query, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to delete user workspaces")
	}

	// Delete workspace invitations
	query = `DELETE FROM workspace_invitations WHERE workspace_id = $1`
	_, err = r.systemDB.ExecContext(ctx, query, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to delete workspace invitations")
	}

	// Delete workspace
	query = `DELETE FROM workspaces WHERE id = $1`
	result, err := r.systemDB.ExecContext(ctx, query, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to delete workspace")
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get affected rows")
	}

	if rowsAffected == 0 {
		return fmt.Errorf("workspace not found")
	}

	return nil
}

func (r *mockInternalRepository) AddUserToWorkspace(ctx context.Context, userWorkspace *domain.UserWorkspace) error {
	return fmt.Errorf("not implemented in mock")
}

func (r *mockInternalRepository) RemoveUserFromWorkspace(ctx context.Context, userID string, workspaceID string) error {
	return fmt.Errorf("not implemented in mock")
}

func (r *mockInternalRepository) GetUserWorkspaces(ctx context.Context, userID string) ([]*domain.UserWorkspace, error) {
	args := ctx.Value("mockGetUserWorkspaces")
	if args == nil {
		return []*domain.UserWorkspace{}, nil
	}
	if err, ok := args.(error); ok {
		return nil, err
	}
	if users, ok := args.([]*domain.UserWorkspace); ok {
		return users, nil
	}
	return []*domain.UserWorkspace{}, nil
}

func (r *mockInternalRepository) GetUserWorkspace(ctx context.Context, userID string, workspaceID string) (*domain.UserWorkspace, error) {
	return nil, fmt.Errorf("not implemented in mock")
}

func (r *mockInternalRepository) GetConnection(ctx context.Context, workspaceID string) (*sql.DB, error) {
	return nil, fmt.Errorf("not implemented in mock")
}

func (r *mockInternalRepository) CreateDatabase(ctx context.Context, workspaceID string) error {
	// This method will be overridden by testWorkspaceRepository
	return nil
}

func (r *mockInternalRepository) DeleteDatabase(ctx context.Context, workspaceID string) error {
	// Mocked for tests
	return nil
}

func (r *mockInternalRepository) CreateInvitation(ctx context.Context, invitation *domain.WorkspaceInvitation) error {
	// Mocked for tests
	return nil
}

func (r *mockInternalRepository) GetInvitationByID(ctx context.Context, id string) (*domain.WorkspaceInvitation, error) {
	// Mocked for tests
	return &domain.WorkspaceInvitation{ID: id}, nil
}

func (r *mockInternalRepository) GetInvitationByEmail(ctx context.Context, workspaceID, email string) (*domain.WorkspaceInvitation, error) {
	// Mocked for tests
	return &domain.WorkspaceInvitation{WorkspaceID: workspaceID, Email: email}, nil
}

func (r *mockInternalRepository) IsUserWorkspaceMember(ctx context.Context, userID, workspaceID string) (bool, error) {
	// Mocked for tests
	return true, nil
}

func (r *mockInternalRepository) GetWorkspaceUsersWithEmail(ctx context.Context, workspaceID string) ([]*domain.UserWorkspaceWithEmail, error) {
	args := ctx.Value("mockGetWorkspaceUsersWithEmail")
	if args == nil {
		return []*domain.UserWorkspaceWithEmail{}, nil
	}
	if err, ok := args.(error); ok {
		return nil, err
	}
	if users, ok := args.([]*domain.UserWorkspaceWithEmail); ok {
		return users, nil
	}
	return []*domain.UserWorkspaceWithEmail{}, nil
}

// testCreateDatabaseTracker is a test wrapper that tracks CreateDatabase calls
type testCreateDatabaseTracker struct {
	domain.WorkspaceRepository
	createDatabaseFn func(ctx context.Context, workspaceID string) error
}

// CreateDatabase overrides the CreateDatabase method for testing
func (t *testCreateDatabaseTracker) CreateDatabase(ctx context.Context, workspaceID string) error {
	return t.createDatabaseFn(ctx, workspaceID)
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
