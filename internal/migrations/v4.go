package migrations

import (
	"context"
	"fmt"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
)

// V4Migration implements the migration from version 3.x to 4.0
type V4Migration struct{}

// GetMajorVersion returns the major version this migration handles
func (m *V4Migration) GetMajorVersion() float64 {
	return 4.0
}

// HasSystemUpdate indicates if this migration has system-level changes
func (m *V4Migration) HasSystemUpdate() bool {
	return true
}

// HasWorkspaceUpdate indicates if this migration has workspace-level changes
func (m *V4Migration) HasWorkspaceUpdate() bool {
	return false
}

// UpdateSystem executes system-level migration changes
func (m *V4Migration) UpdateSystem(ctx context.Context, config *config.Config, db DBExecutor) error {
	// Verify database connection by querying system table
	var count int
	err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to verify system database connection: %w", err)
	}

	// Add permissions column to user_workspaces table for granular permissions
	_, err = db.ExecContext(ctx, `
		ALTER TABLE user_workspaces
		ADD COLUMN IF NOT EXISTS permissions JSONB DEFAULT '{}'::jsonb
	`)
	if err != nil {
		return fmt.Errorf("failed to add permissions column to user_workspaces: %w", err)
	}

	// Grant all permissions to existing workspace owners to maintain backward compatibility
	_, err = db.ExecContext(ctx, `
		UPDATE user_workspaces
		SET permissions = '{
			"contacts": {"read": true, "write": true},
			"lists": {"read": true, "write": true},
			"templates": {"read": true, "write": true},
			"broadcasts": {"read": true, "write": true},
			"transactional": {"read": true, "write": true},
			"workspace": {"read": true, "write": true},
			"message_history": {"read": true, "write": true}
		}'::jsonb
		WHERE role = 'owner'
	`)
	if err != nil {
		return fmt.Errorf("failed to grant permissions to existing owners: %w", err)
	}

	// Grant read permissions to existing workspace members
	_, err = db.ExecContext(ctx, `
		UPDATE user_workspaces
		SET permissions = '{
			"contacts": {"read": true, "write": false},
			"lists": {"read": true, "write": false},
			"templates": {"read": true, "write": false},
			"broadcasts": {"read": true, "write": false},
			"transactional": {"read": true, "write": false},
			"workspace": {"read": true, "write": false},
			"message_history": {"read": true, "write": false}
		}'::jsonb
		WHERE role = 'member' AND (permissions IS NULL OR permissions = '{}'::jsonb)
	`)
	if err != nil {
		return fmt.Errorf("failed to grant read permissions to existing members: %w", err)
	}

	return nil
}

// UpdateWorkspace executes workspace-level migration changes
func (m *V4Migration) UpdateWorkspace(ctx context.Context, config *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	// Verify database connection by querying workspace table
	var count int
	err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM contacts").Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to verify workspace database connection: %w", err)
	}

	// Example: Modify workspace tables for version 4.0
	// In a real migration, you would execute workspace-specific changes

	// For now, this is a placeholder that demonstrates the structure
	// Real migrations would include ALTER TABLE for workspace tables

	return nil
}

// init registers this migration with the default registry
func init() {
	Register(&V4Migration{})
}
