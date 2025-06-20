package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/database"
	"github.com/Notifuse/notifuse/internal/domain"
)

type workspaceRepository struct {
	systemDB  *sql.DB
	dbConfig  *config.DatabaseConfig
	secretKey string

	// Connection pool for workspace databases
	connectionPools sync.Map
}

// NewWorkspaceRepository creates a new PostgreSQL workspace repository
func NewWorkspaceRepository(systemDB *sql.DB, dbConfig *config.DatabaseConfig, secretKey string) domain.WorkspaceRepository {
	return &workspaceRepository{
		systemDB:  systemDB,
		dbConfig:  dbConfig,
		secretKey: secretKey,
	}
}

// checkWorkspaceIDExists checks if a workspace with the given ID already exists
func (r *workspaceRepository) checkWorkspaceIDExists(ctx context.Context, id string) (bool, error) {
	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM workspaces WHERE id = $1)`
	err := r.systemDB.QueryRowContext(ctx, query, id).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

// Create creates a new workspace
func (r *workspaceRepository) Create(ctx context.Context, workspace *domain.Workspace) error {

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

	if err := workspace.BeforeSave(r.secretKey); err != nil {
		return err
	}

	query := `
		INSERT INTO workspaces (id, name, settings, integrations, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`
	_, err = r.systemDB.ExecContext(ctx, query,
		workspace.ID,
		workspace.Name,
		workspace.Settings,
		workspace.Integrations,
		workspace.CreatedAt,
		workspace.UpdatedAt,
	)
	if err != nil {
		return err
	}

	if err := workspace.AfterLoad(r.secretKey); err != nil {
		return err
	}

	// Create the workspace database
	return r.CreateDatabase(ctx, workspace.ID)
}

func (r *workspaceRepository) GetByID(ctx context.Context, id string) (*domain.Workspace, error) {
	query := `
		SELECT id, name, settings, integrations, created_at, updated_at
		FROM workspaces
		WHERE id = $1
	`
	workspace, err := domain.ScanWorkspace(r.systemDB.QueryRowContext(ctx, query, id))
	if err == sql.ErrNoRows {
		return nil, &domain.ErrWorkspaceNotFound{WorkspaceID: id}
	}
	if err != nil {
		return nil, err
	}

	if err := workspace.AfterLoad(r.secretKey); err != nil {
		return nil, err
	}

	return workspace, nil
}

func (r *workspaceRepository) List(ctx context.Context) ([]*domain.Workspace, error) {
	query := `
		SELECT id, name, settings, integrations, created_at, updated_at
		FROM workspaces
		ORDER BY created_at DESC
	`
	rows, err := r.systemDB.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var workspaces []*domain.Workspace
	for rows.Next() {
		workspace, err := domain.ScanWorkspace(rows)
		if err != nil {
			return nil, err
		}
		if err := workspace.AfterLoad(r.secretKey); err != nil {
			return nil, err
		}
		workspaces = append(workspaces, workspace)
	}
	return workspaces, rows.Err()
}

func (r *workspaceRepository) Update(ctx context.Context, workspace *domain.Workspace) error {
	workspace.UpdatedAt = time.Now()

	if err := workspace.BeforeSave(r.secretKey); err != nil {
		return err
	}

	// Marshal settings to JSON
	settings, err := json.Marshal(workspace.Settings)
	if err != nil {
		return err
	}

	query := `
		UPDATE workspaces
		SET name = $1, settings = $2, integrations = $3, updated_at = $4
		WHERE id = $5
	`
	result, err := r.systemDB.ExecContext(ctx, query,
		workspace.Name,
		settings,
		workspace.Integrations,
		workspace.UpdatedAt,
		workspace.ID,
	)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return &domain.ErrWorkspaceNotFound{WorkspaceID: workspace.ID}
	}

	if err := workspace.AfterLoad(r.secretKey); err != nil {
		return err
	}

	return nil
}

func (r *workspaceRepository) Delete(ctx context.Context, id string) error {
	// Delete the workspace database first
	if err := r.DeleteDatabase(ctx, id); err != nil {
		return err
	}

	// Delete all user_workspaces entries for this workspace
	deleteUserWorkspacesQuery := `DELETE FROM user_workspaces WHERE workspace_id = $1`
	if _, err := r.systemDB.ExecContext(ctx, deleteUserWorkspacesQuery, id); err != nil {
		return err
	}

	// Delete all workspace invitations for this workspace
	deleteInvitationsQuery := `DELETE FROM workspace_invitations WHERE workspace_id = $1`
	if _, err := r.systemDB.ExecContext(ctx, deleteInvitationsQuery, id); err != nil {
		return err
	}

	// Then delete the workspace record
	query := `DELETE FROM workspaces WHERE id = $1`
	result, err := r.systemDB.ExecContext(ctx, query, id)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return &domain.ErrWorkspaceNotFound{WorkspaceID: id}
	}
	return nil
}

// GetConnection returns a connection to the workspace database
func (r *workspaceRepository) GetConnection(ctx context.Context, workspaceID string) (*sql.DB, error) {
	// Check if we already have a connection
	if conn, ok := r.connectionPools.Load(workspaceID); ok {
		db := conn.(*sql.DB)
		// Test the connection
		if err := db.PingContext(ctx); err == nil {
			return db, nil
		}
		// If ping fails, remove the connection and create a new one
		r.connectionPools.Delete(workspaceID)
	}

	// Create a new connection
	db, err := database.ConnectToWorkspace(r.dbConfig, workspaceID)
	if err != nil {
		return nil, err
	}

	// Store the connection
	r.connectionPools.Store(workspaceID, db)
	return db, nil
}

// WithWorkspaceTransaction executes a function within a database transaction
func (r *workspaceRepository) WithWorkspaceTransaction(ctx context.Context, workspaceID string, fn func(*sql.Tx) error) error {
	// Get the workspace database connection
	workspaceDB, err := r.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}

	// Begin a transaction
	tx, err := workspaceDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	// Defer rollback - this will be a no-op if we successfully commit
	defer tx.Rollback()

	// Execute the provided function with the transaction
	if err := fn(tx); err != nil {
		return err
	}

	// Commit the transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

func (r *workspaceRepository) CreateDatabase(ctx context.Context, workspaceID string) error {
	// Use the utility function to ensure the database exists and initialize it
	if err := database.EnsureWorkspaceDatabaseExists(r.dbConfig, workspaceID); err != nil {
		return err
	}
	return nil
}

func (r *workspaceRepository) DeleteDatabase(ctx context.Context, workspaceID string) error {
	// Replace hyphens with underscores for PostgreSQL compatibility
	safeID := strings.ReplaceAll(workspaceID, "-", "_")
	dbName := fmt.Sprintf("%s_ws_%s", r.dbConfig.Prefix, safeID)

	// Get our own connection to close it
	if conn, ok := r.connectionPools.Load(workspaceID); ok {
		db := conn.(*sql.DB)
		// Close our connection to the workspace database
		db.Close()
		// Remove from the pool
		r.connectionPools.Delete(workspaceID)
	}

	// First, revoke all privileges to prevent new connections
	revokeQuery := fmt.Sprintf(`
		REVOKE ALL PRIVILEGES ON DATABASE %s FROM PUBLIC;
		REVOKE ALL PRIVILEGES ON DATABASE %s FROM %s;
		REVOKE ALL PRIVILEGES ON ALL TABLES IN SCHEMA public FROM PUBLIC;
		REVOKE ALL PRIVILEGES ON ALL TABLES IN SCHEMA public FROM %s;
		REVOKE ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public FROM PUBLIC;
		REVOKE ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public FROM %s;`,
		dbName, dbName, r.dbConfig.User, r.dbConfig.User, r.dbConfig.User)
	if _, err := r.systemDB.ExecContext(ctx, revokeQuery); err != nil {
		return err
	}

	// Then terminate all connections to the database
	terminateQuery := fmt.Sprintf(`
		SELECT pg_terminate_backend(pid) 
		FROM pg_stat_activity 
		WHERE datname = '%s' 
		AND pid <> pg_backend_pid()`, dbName)
	if _, err := r.systemDB.ExecContext(ctx, terminateQuery); err != nil {
		return err
	}

	// Add a small delay to ensure connections are closed
	time.Sleep(100 * time.Millisecond)

	// Finally, drop the database
	dropQuery := fmt.Sprintf("DROP DATABASE IF EXISTS %s", dbName)
	if _, err := r.systemDB.ExecContext(ctx, dropQuery); err != nil {
		return err
	}

	return nil
}

func (r *workspaceRepository) AddUserToWorkspace(ctx context.Context, userWorkspace *domain.UserWorkspace) error {
	query := `
		INSERT INTO user_workspaces (user_id, workspace_id, role, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (user_id, workspace_id) DO UPDATE
		SET role = $3, updated_at = $5
	`
	_, err := r.systemDB.ExecContext(ctx, query,
		userWorkspace.UserID,
		userWorkspace.WorkspaceID,
		userWorkspace.Role,
		userWorkspace.CreatedAt,
		userWorkspace.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to add user to workspace: %w", err)
	}
	return nil
}

func (r *workspaceRepository) RemoveUserFromWorkspace(ctx context.Context, userID string, workspaceID string) error {
	query := `DELETE FROM user_workspaces WHERE user_id = $1 AND workspace_id = $2`
	result, err := r.systemDB.ExecContext(ctx, query, userID, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to remove user from workspace: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get affected rows: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("user is not a member of the workspace")
	}
	return nil
}

func (r *workspaceRepository) GetUserWorkspaces(ctx context.Context, userID string) ([]*domain.UserWorkspace, error) {
	query := `
		SELECT user_id, workspace_id, role, created_at, updated_at
		FROM user_workspaces
		WHERE user_id = $1
	`
	rows, err := r.systemDB.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user workspaces: %w", err)
	}
	defer rows.Close()

	var userWorkspaces []*domain.UserWorkspace
	for rows.Next() {
		var uw domain.UserWorkspace
		err := rows.Scan(&uw.UserID, &uw.WorkspaceID, &uw.Role, &uw.CreatedAt, &uw.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan user workspace: %w", err)
		}
		userWorkspaces = append(userWorkspaces, &uw)
	}
	return userWorkspaces, rows.Err()
}

func (r *workspaceRepository) GetUserWorkspace(ctx context.Context, userID string, workspaceID string) (*domain.UserWorkspace, error) {
	query := `
		SELECT user_id, workspace_id, role, created_at, updated_at
		FROM user_workspaces
		WHERE user_id = $1 AND workspace_id = $2
	`
	var uw domain.UserWorkspace
	err := r.systemDB.QueryRowContext(ctx, query, userID, workspaceID).Scan(
		&uw.UserID, &uw.WorkspaceID, &uw.Role, &uw.CreatedAt, &uw.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("user is not a member of the workspace")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user workspace: %w", err)
	}
	return &uw, nil
}

// CreateInvitation creates a new workspace invitation
func (r *workspaceRepository) CreateInvitation(ctx context.Context, invitation *domain.WorkspaceInvitation) error {
	query := `
		INSERT INTO workspace_invitations (id, workspace_id, inviter_id, email, expires_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`
	_, err := r.systemDB.ExecContext(
		ctx,
		query,
		invitation.ID,
		invitation.WorkspaceID,
		invitation.InviterID,
		invitation.Email,
		invitation.ExpiresAt,
		invitation.CreatedAt,
		invitation.UpdatedAt,
	)
	if err != nil {
		return err
	}
	return nil
}

// GetInvitationByID retrieves a workspace invitation by its ID
func (r *workspaceRepository) GetInvitationByID(ctx context.Context, id string) (*domain.WorkspaceInvitation, error) {
	query := `
		SELECT id, workspace_id, inviter_id, email, expires_at, created_at, updated_at
		FROM workspace_invitations
		WHERE id = $1
	`
	var invitation domain.WorkspaceInvitation
	err := r.systemDB.QueryRowContext(ctx, query, id).Scan(
		&invitation.ID,
		&invitation.WorkspaceID,
		&invitation.InviterID,
		&invitation.Email,
		&invitation.ExpiresAt,
		&invitation.CreatedAt,
		&invitation.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("invitation not found")
		}
		return nil, err
	}
	return &invitation, nil
}

// GetInvitationByEmail retrieves a workspace invitation by workspace ID and email
func (r *workspaceRepository) GetInvitationByEmail(ctx context.Context, workspaceID, email string) (*domain.WorkspaceInvitation, error) {
	query := `
		SELECT id, workspace_id, inviter_id, email, expires_at, created_at, updated_at
		FROM workspace_invitations
		WHERE workspace_id = $1 AND email = $2
		ORDER BY created_at DESC
		LIMIT 1
	`
	var invitation domain.WorkspaceInvitation
	err := r.systemDB.QueryRowContext(ctx, query, workspaceID, email).Scan(
		&invitation.ID,
		&invitation.WorkspaceID,
		&invitation.InviterID,
		&invitation.Email,
		&invitation.ExpiresAt,
		&invitation.CreatedAt,
		&invitation.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("invitation not found")
		}
		return nil, err
	}
	return &invitation, nil
}

// IsUserWorkspaceMember checks if a user is a member of a workspace
func (r *workspaceRepository) IsUserWorkspaceMember(ctx context.Context, userID, workspaceID string) (bool, error) {
	query := `
		SELECT COUNT(*)
		FROM user_workspaces
		WHERE user_id = $1 AND workspace_id = $2
	`
	var count int
	err := r.systemDB.QueryRowContext(ctx, query, userID, workspaceID).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// GetWorkspaceUsersWithEmail returns all users for a workspace including email information
func (r *workspaceRepository) GetWorkspaceUsersWithEmail(ctx context.Context, workspaceID string) ([]*domain.UserWorkspaceWithEmail, error) {
	query := `
		SELECT uw.user_id, uw.workspace_id, uw.role, uw.created_at, uw.updated_at, u.email, u.type
		FROM user_workspaces uw
		JOIN users u ON uw.user_id = u.id
		WHERE uw.workspace_id = $1
	`
	rows, err := r.systemDB.QueryContext(ctx, query, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace users with email: %w", err)
	}
	defer rows.Close()

	var userWorkspaces []*domain.UserWorkspaceWithEmail
	for rows.Next() {
		var uw domain.UserWorkspaceWithEmail
		err := rows.Scan(
			&uw.UserID,
			&uw.WorkspaceID,
			&uw.Role,
			&uw.CreatedAt,
			&uw.UpdatedAt,
			&uw.Email,
			&uw.Type,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan user workspace with email: %w", err)
		}
		userWorkspaces = append(userWorkspaces, &uw)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating workspace users rows: %w", err)
	}

	return userWorkspaces, nil
}
