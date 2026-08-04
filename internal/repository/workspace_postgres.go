package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/database"
	"github.com/Notifuse/notifuse/internal/domain"
	pkgDatabase "github.com/Notifuse/notifuse/pkg/database"
)

const (
	// pgSyntaxErrorCode is PostgreSQL's syntax_error. DROP DATABASE ... WITH (FORCE)
	// was introduced in PostgreSQL 13; older servers reject it while parsing, so this
	// code tells us the statement never ran and a fallback is safe.
	pgSyntaxErrorCode = "42601"

	// dropDatabaseTimeout bounds the workspace database drop. It is deliberately
	// generous: DROP DATABASE marks the database invalid within milliseconds and
	// only clears that mark once it completes, so a deadline that fires mid-drop
	// produces exactly the unusable database this timeout is meant to avoid. The
	// drop also forces an immediate checkpoint, whose cost scales with the whole
	// server's dirty buffers rather than with the size of the workspace, so a busy
	// server can legitimately take far longer than the drop itself suggests. This
	// exists only to stop a wedged server from pinning a connection forever.
	dropDatabaseTimeout = 5 * time.Minute

	// deleteWorkspaceTimeout bounds a whole workspace deletion. Deletion is detached
	// from the caller, and no statement_timeout is configured server-side, so without
	// this a system-database row delete blocked on a lock would hold a connection from
	// the small system pool indefinitely. It sits above dropDatabaseTimeout so that a
	// slow drop still fails on its own, more specific deadline.
	deleteWorkspaceTimeout = 10 * time.Minute
)

type workspaceRepository struct {
	systemDB          *sql.DB
	dbConfig          *config.DatabaseConfig
	secretKey         string
	connectionManager pkgDatabase.ConnectionManager
}

// NewWorkspaceRepository creates a new PostgreSQL workspace repository
func NewWorkspaceRepository(
	systemDB *sql.DB,
	dbConfig *config.DatabaseConfig,
	secretKey string,
	connectionManager pkgDatabase.ConnectionManager,
) domain.WorkspaceRepository {
	return &workspaceRepository{
		systemDB:          systemDB,
		dbConfig:          dbConfig,
		secretKey:         secretKey,
		connectionManager: connectionManager,
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

	now := time.Now().UTC()
	workspace.CreatedAt = now
	workspace.UpdatedAt = now

	if err := workspace.BeforeSave(r.secretKey); err != nil {
		return err
	}

	// STEP 1: Create the workspace database FIRST
	// If this fails (e.g., due to permissions), no system record is created
	if err := r.CreateDatabase(ctx, workspace.ID); err != nil {
		return fmt.Errorf("failed to create workspace database: %w", err)
	}

	// STEP 2: Now insert the workspace record into system database
	// Marshal settings to JSON
	settings, err := json.Marshal(workspace.Settings)
	if err != nil {
		// Clean up: delete the database we just created
		_ = r.DeleteDatabase(ctx, workspace.ID)
		return err
	}

	// Marshal integrations to JSON
	integrations, err := json.Marshal(workspace.Integrations)
	if err != nil {
		// Clean up: delete the database we just created
		_ = r.DeleteDatabase(ctx, workspace.ID)
		return err
	}

	query := `
		INSERT INTO workspaces (id, name, settings, integrations, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`
	_, err = r.systemDB.ExecContext(ctx, query,
		workspace.ID,
		workspace.Name,
		settings,
		integrations,
		workspace.CreatedAt,
		workspace.UpdatedAt,
	)
	if err != nil {
		// Clean up: delete the database we just created
		_ = r.DeleteDatabase(ctx, workspace.ID)
		return err
	}

	if err := workspace.AfterLoad(r.secretKey); err != nil {
		return err
	}

	return nil
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

// GetWorkspaceByCustomDomain retrieves a workspace by custom domain hostname
func (r *workspaceRepository) GetWorkspaceByCustomDomain(ctx context.Context, hostname string) (*domain.Workspace, error) {
	// Query to find workspace where custom_endpoint_url contains the hostname
	// We need to extract the hostname from the URL and compare (case-insensitive)
	query := `
		SELECT id, name, settings, integrations, created_at, updated_at
		FROM workspaces
		WHERE settings->>'custom_endpoint_url' IS NOT NULL
		  AND settings->>'custom_endpoint_url' != ''
		  AND LOWER(
		      CASE 
		        WHEN settings->>'custom_endpoint_url' LIKE 'http://%' 
		          THEN SPLIT_PART(SPLIT_PART(settings->>'custom_endpoint_url', '://', 2), '/', 1)
		        WHEN settings->>'custom_endpoint_url' LIKE 'https://%'
		          THEN SPLIT_PART(SPLIT_PART(settings->>'custom_endpoint_url', '://', 2), '/', 1)
		        ELSE SPLIT_PART(settings->>'custom_endpoint_url', '/', 1)
		      END
		  ) = LOWER($1)
		LIMIT 1
	`

	workspace, err := domain.ScanWorkspace(r.systemDB.QueryRowContext(ctx, query, hostname))
	if err == sql.ErrNoRows {
		return nil, nil // Return nil without error when no workspace is found (not an error condition)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query workspace by custom domain: %w", err)
	}

	if err := workspace.AfterLoad(r.secretKey); err != nil {
		return nil, fmt.Errorf("failed to decrypt workspace secrets: %w", err)
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
	defer func() { _ = rows.Close() }()

	// Initialize as an empty (non-nil) slice so an installation with no
	// workspaces serializes to `[]` rather than `null`. The root user hits
	// this path directly (see WorkspaceService.ListWorkspaces), and a null
	// value crashes the console which accesses `workspaces.length`.
	workspaces := []*domain.Workspace{}
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

// PatchIntegrationSESSettings merges patch into the SES settings of one integration.
//
// This exists because Update() marshals and writes the entire integrations array: two
// concurrent writers (a console edit and a background write of provisioned state, say) each
// read, modify and write the whole thing, so the last one wins and the other's change vanishes.
// Merging server-owned fields in a single statement removes that race for them, and skips the
// encryption round-trip Update() performs on every stored secret.
func (r *workspaceRepository) PatchIntegrationSESSettings(ctx context.Context, workspaceID string, integrationID string, patch map[string]interface{}) error {
	if len(patch) == 0 {
		return nil
	}

	encoded, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("failed to encode SES settings patch: %w", err)
	}

	// The index of the integration is resolved in the same statement, so no read happens in Go.
	const query = `
		UPDATE workspaces AS w
		SET integrations = jsonb_set(
				w.integrations,
				ARRAY[(target.idx - 1)::text, 'email_provider', 'ses'],
				COALESCE(w.integrations -> (target.idx - 1)::int -> 'email_provider' -> 'ses', '{}'::jsonb) || $3::jsonb,
				true
			),
			updated_at = $4
		FROM (
			SELECT e.ordinality AS idx
			FROM workspaces inner_w,
				 jsonb_array_elements(inner_w.integrations) WITH ORDINALITY AS e(elem, ordinality)
			WHERE inner_w.id = $1 AND e.elem ->> 'id' = $2
			LIMIT 1
		) AS target
		WHERE w.id = $1
	`

	result, err := r.systemDB.ExecContext(ctx, query, workspaceID, integrationID, encoded, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("failed to patch SES settings: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to read patch result: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("integration %s not found in workspace %s", integrationID, workspaceID)
	}

	return nil
}

func (r *workspaceRepository) Update(ctx context.Context, workspace *domain.Workspace) error {
	workspace.UpdatedAt = time.Now().UTC()

	if err := workspace.BeforeSave(r.secretKey); err != nil {
		return err
	}

	// Marshal settings to JSON
	settings, err := json.Marshal(workspace.Settings)
	if err != nil {
		return err
	}

	// Marshal integrations to JSON
	integrations, err := json.Marshal(workspace.Integrations)
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
		integrations,
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
	// Dropping the database is irreversible and the system-database rows are cleaned
	// up afterwards, so the whole sequence has to survive the caller going away. If
	// only the drop were detached, a client disconnecting mid-request would leave the
	// workspace rows pointing at a database that no longer exists — and the next
	// request for that workspace would silently recreate it empty.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), deleteWorkspaceTimeout)
	defer cancel()

	// Delete the workspace database first. Dropping is much likelier to fail than the
	// row cleanup below — it needs an exclusive lock and is refused while a prepared
	// transaction, an active replication slot or a backend it cannot terminate is
	// attached — so it goes first, where a failure aborts the whole operation without
	// having changed anything and the caller can simply retry.
	if err := r.DeleteDatabase(ctx, id); err != nil {
		return err
	}

	// The row deletions have to land together. Removing the memberships without
	// removing the workspace row leaves a workspace its owner can no longer see, and
	// therefore can no longer delete, with no way back short of direct database access.
	tx, err := r.systemDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction deleting workspace %s: %w", id, err)
	}
	defer func() { _ = tx.Rollback() }()

	// Delete all user_workspaces entries for this workspace
	deleteUserWorkspacesQuery := `DELETE FROM user_workspaces WHERE workspace_id = $1`
	if _, err := tx.ExecContext(ctx, deleteUserWorkspacesQuery, id); err != nil {
		return fmt.Errorf("failed to delete memberships of workspace %s: %w", id, err)
	}

	// Delete all workspace invitations for this workspace
	deleteInvitationsQuery := `DELETE FROM workspace_invitations WHERE workspace_id = $1`
	if _, err := tx.ExecContext(ctx, deleteInvitationsQuery, id); err != nil {
		return fmt.Errorf("failed to delete invitations of workspace %s: %w", id, err)
	}

	// Then delete the workspace record
	query := `DELETE FROM workspaces WHERE id = $1`
	result, err := tx.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete workspace %s: %w", id, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to read rows affected deleting workspace %s: %w", id, err)
	}
	if rows == 0 {
		return &domain.ErrWorkspaceNotFound{WorkspaceID: id}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit deletion of workspace %s: %w", id, err)
	}

	return nil
}

// GetConnection returns a connection to the workspace database
func (r *workspaceRepository) GetConnection(ctx context.Context, workspaceID string) (*sql.DB, error) {
	return r.connectionManager.GetWorkspaceConnection(ctx, workspaceID)
}

// GetSystemConnection returns a connection to the system database
func (r *workspaceRepository) GetSystemConnection(ctx context.Context) (*sql.DB, error) {
	return r.systemDB, nil
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
	defer func() { _ = tx.Rollback() }()

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
	// The database was created through an unquoted CREATE DATABASE, so PostgreSQL
	// folded its name to lower case. Fold here too: a quoted mixed-case name would
	// not match the database that actually exists, and IF EXISTS would turn that
	// miss into a silent success, orphaning the database.
	dbName := strings.ToLower(fmt.Sprintf("%s_ws_%s", r.dbConfig.Prefix, safeID))
	quotedDBName := pq.QuoteIdentifier(dbName)

	// Close the workspace connection pool
	if err := r.connectionManager.CloseWorkspaceConnection(workspaceID); err != nil {
		// Log error but continue with database deletion
		fmt.Printf("Warning: failed to close workspace connection: %v\n", err)
	}

	// An aborted DROP DATABASE can leave the database marked invalid in place: it
	// then rejects every connection and is not recreated automatically, so only a
	// manual drop recovers it. Detach from the caller's context so a disconnecting
	// client cannot cancel the drop midway.
	dropCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), dropDatabaseTimeout)
	defer cancel()

	// WITH (FORCE) terminates the remaining backends and drops the database as one
	// operation, which closes the race where a connection is re-established between
	// terminating backends and dropping.
	dropQuery := fmt.Sprintf("DROP DATABASE IF EXISTS %s WITH (FORCE)", quotedDBName)
	_, err := r.systemDB.ExecContext(dropCtx, dropQuery)
	if err == nil {
		return nil
	}

	// PostgreSQL below 13 rejects WITH (FORCE) while parsing, so nothing ran and it
	// is safe to retry the pre-13 way.
	var pqErr *pq.Error
	if errors.As(err, &pqErr) && string(pqErr.Code) == pgSyntaxErrorCode {
		if fallbackErr := r.dropDatabaseWithoutForce(dropCtx, dbName, quotedDBName); fallbackErr != nil {
			return fmt.Errorf("failed to drop workspace database %s (server predates WITH (FORCE)): %w", dbName, fallbackErr)
		}
		return nil
	}

	return fmt.Errorf("failed to drop workspace database %s: %w", dbName, err)
}

// dropDatabaseWithoutForce drops a workspace database on PostgreSQL servers that
// predate DROP DATABASE ... WITH (FORCE), by terminating the remaining backends
// first. This leaves a window in which a new connection can block the drop; the
// caller is expected to surface the resulting error.
func (r *workspaceRepository) dropDatabaseWithoutForce(ctx context.Context, dbName, quotedDBName string) error {
	terminateQuery := `
		SELECT pg_terminate_backend(pid)
		FROM pg_stat_activity
		WHERE datname = $1
		AND pid <> pg_backend_pid()`
	if _, err := r.systemDB.ExecContext(ctx, terminateQuery, dbName); err != nil {
		return err
	}

	dropQuery := fmt.Sprintf("DROP DATABASE IF EXISTS %s", quotedDBName)
	if _, err := r.systemDB.ExecContext(ctx, dropQuery); err != nil {
		return err
	}

	return nil
}

func (r *workspaceRepository) AddUserToWorkspace(ctx context.Context, userWorkspace *domain.UserWorkspace) error {
	query := `
		INSERT INTO user_workspaces (user_id, workspace_id, role, permissions, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (user_id, workspace_id) DO UPDATE
		SET role = $3, permissions = $4, updated_at = $6
	`
	_, err := r.systemDB.ExecContext(ctx, query,
		userWorkspace.UserID,
		userWorkspace.WorkspaceID,
		userWorkspace.Role,
		userWorkspace.Permissions,
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
		SELECT user_id, workspace_id, role, permissions, created_at, updated_at
		FROM user_workspaces
		WHERE user_id = $1
	`
	rows, err := r.systemDB.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user workspaces: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var userWorkspaces []*domain.UserWorkspace
	for rows.Next() {
		var uw domain.UserWorkspace
		err := rows.Scan(&uw.UserID, &uw.WorkspaceID, &uw.Role, &uw.Permissions, &uw.CreatedAt, &uw.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan user workspace: %w", err)
		}
		userWorkspaces = append(userWorkspaces, &uw)
	}
	return userWorkspaces, rows.Err()
}

func (r *workspaceRepository) GetUserWorkspace(ctx context.Context, userID string, workspaceID string) (*domain.UserWorkspace, error) {
	query := `
		SELECT user_id, workspace_id, role, permissions, created_at, updated_at
		FROM user_workspaces
		WHERE user_id = $1 AND workspace_id = $2
	`
	var uw domain.UserWorkspace
	err := r.systemDB.QueryRowContext(ctx, query, userID, workspaceID).Scan(
		&uw.UserID, &uw.WorkspaceID, &uw.Role, &uw.Permissions, &uw.CreatedAt, &uw.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, domain.ErrUserNotInWorkspace
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user workspace: %w", err)
	}
	return &uw, nil
}

// UpdateUserWorkspacePermissions updates the permissions for a user in a workspace
func (r *workspaceRepository) UpdateUserWorkspacePermissions(ctx context.Context, userWorkspace *domain.UserWorkspace) error {
	query := `
		UPDATE user_workspaces 
		SET permissions = $1, updated_at = $2
		WHERE user_id = $3 AND workspace_id = $4
	`
	_, err := r.systemDB.ExecContext(
		ctx, query,
		userWorkspace.Permissions,
		time.Now(),
		userWorkspace.UserID,
		userWorkspace.WorkspaceID,
	)
	if err != nil {
		return fmt.Errorf("failed to update user workspace permissions: %w", err)
	}
	return nil
}

// CreateInvitation creates a new workspace invitation or updates an existing one
func (r *workspaceRepository) CreateInvitation(ctx context.Context, invitation *domain.WorkspaceInvitation) error {
	query := `
		INSERT INTO workspace_invitations (id, workspace_id, inviter_id, email, permissions, expires_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (workspace_id, email) DO UPDATE SET
			inviter_id = EXCLUDED.inviter_id,
			permissions = EXCLUDED.permissions,
			expires_at = EXCLUDED.expires_at,
			updated_at = EXCLUDED.updated_at
	`
	_, err := r.systemDB.ExecContext(
		ctx,
		query,
		invitation.ID,
		invitation.WorkspaceID,
		invitation.InviterID,
		invitation.Email,
		invitation.Permissions,
		invitation.ExpiresAt,
		invitation.CreatedAt,
		invitation.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create or update invitation: %w", err)
	}
	return nil
}

// GetInvitationByID retrieves a workspace invitation by its ID
func (r *workspaceRepository) GetInvitationByID(ctx context.Context, id string) (*domain.WorkspaceInvitation, error) {
	query := `
		SELECT id, workspace_id, inviter_id, email, permissions, expires_at, created_at, updated_at
		FROM workspace_invitations
		WHERE id = $1
	`
	var invitation domain.WorkspaceInvitation
	err := r.systemDB.QueryRowContext(ctx, query, id).Scan(
		&invitation.ID,
		&invitation.WorkspaceID,
		&invitation.InviterID,
		&invitation.Email,
		&invitation.Permissions,
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
		SELECT id, workspace_id, inviter_id, email, permissions, expires_at, created_at, updated_at
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
		&invitation.Permissions,
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

// GetWorkspaceInvitations retrieves all workspace invitations for a specific workspace
func (r *workspaceRepository) GetWorkspaceInvitations(ctx context.Context, workspaceID string) ([]*domain.WorkspaceInvitation, error) {
	query := `
		SELECT id, workspace_id, inviter_id, email, permissions, expires_at, created_at, updated_at
		FROM workspace_invitations
		WHERE workspace_id = $1
		ORDER BY created_at DESC
	`
	rows, err := r.systemDB.QueryContext(ctx, query, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace invitations: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var invitations []*domain.WorkspaceInvitation
	for rows.Next() {
		var invitation domain.WorkspaceInvitation
		err := rows.Scan(
			&invitation.ID,
			&invitation.WorkspaceID,
			&invitation.InviterID,
			&invitation.Email,
			&invitation.Permissions,
			&invitation.ExpiresAt,
			&invitation.CreatedAt,
			&invitation.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan workspace invitation: %w", err)
		}
		invitations = append(invitations, &invitation)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating workspace invitations rows: %w", err)
	}

	return invitations, nil
}

// DeleteInvitation deletes a workspace invitation by its ID
func (r *workspaceRepository) DeleteInvitation(ctx context.Context, id string) error {
	query := `DELETE FROM workspace_invitations WHERE id = $1`
	result, err := r.systemDB.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete invitation: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("invitation not found")
	}

	return nil
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

// CountWorkspaceMembersAndInvitations returns the count of current human members
// plus non-expired pending invitations. API key users are excluded from the count.
func (r *workspaceRepository) CountWorkspaceMembersAndInvitations(ctx context.Context, workspaceID string) (int, error) {
	query := `
		SELECT
			(SELECT COUNT(*) FROM user_workspaces uw JOIN users u ON uw.user_id = u.id WHERE uw.workspace_id = $1 AND u.type != 'api_key') +
			(SELECT COUNT(*) FROM workspace_invitations WHERE workspace_id = $1 AND expires_at > NOW())
	`
	var count int
	err := r.systemDB.QueryRowContext(ctx, query, workspaceID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count workspace members and invitations: %w", err)
	}
	return count, nil
}

// CountWorkspaces returns the total number of workspaces in the system.
func (r *workspaceRepository) CountWorkspaces(ctx context.Context) (int, error) {
	var count int
	err := r.systemDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM workspaces").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count workspaces: %w", err)
	}
	return count, nil
}

// GetWorkspaceUsersWithEmail returns all users for a workspace including email information
func (r *workspaceRepository) GetWorkspaceUsersWithEmail(ctx context.Context, workspaceID string) ([]*domain.UserWorkspaceWithEmail, error) {
	query := `
		SELECT uw.user_id, uw.workspace_id, uw.role, uw.permissions, uw.created_at, uw.updated_at, u.email, u.type, u.language
		FROM user_workspaces uw
		JOIN users u ON uw.user_id = u.id
		WHERE uw.workspace_id = $1
	`
	rows, err := r.systemDB.QueryContext(ctx, query, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace users with email: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var userWorkspaces []*domain.UserWorkspaceWithEmail
	for rows.Next() {
		var uw domain.UserWorkspaceWithEmail
		err := rows.Scan(
			&uw.UserID,
			&uw.WorkspaceID,
			&uw.Role,
			&uw.Permissions,
			&uw.CreatedAt,
			&uw.UpdatedAt,
			&uw.Email,
			&uw.Type,
			&uw.Language,
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
