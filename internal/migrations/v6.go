package migrations

import (
	"context"
	"fmt"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
)

// V6Migration implements the migration from version 5.x to 6.0
type V6Migration struct{}

// GetMajorVersion returns the major version this migration handles
func (m *V6Migration) GetMajorVersion() float64 {
	return 6.0
}

// HasSystemUpdate indicates if this migration has system-level changes
func (m *V6Migration) HasSystemUpdate() bool {
	return true
}

// HasWorkspaceUpdate indicates if this migration has workspace-level changes
func (m *V6Migration) HasWorkspaceUpdate() bool {
	return false
}

// UpdateSystem executes system-level migration changes
func (m *V6Migration) UpdateSystem(ctx context.Context, config *config.Config, db DBExecutor) error {
	// Add permissions column to workspace_invitations table
	// Using IF NOT EXISTS to make the migration idempotent
	_, err := db.ExecContext(ctx, `
		ALTER TABLE workspace_invitations
		ADD COLUMN IF NOT EXISTS permissions JSONB DEFAULT '{}'::jsonb
	`)
	if err != nil {
		return fmt.Errorf("failed to add permissions column to workspace_invitations table: %w", err)
	}

	// Grant default read permissions to existing invitations to maintain backward compatibility
	// This ensures existing invitations work with the new permission system
	_, err = db.ExecContext(ctx, `
		UPDATE workspace_invitations
		SET permissions = '{
			"contacts": {"read": true, "write": false},
			"lists": {"read": true, "write": false},
			"templates": {"read": true, "write": false},
			"broadcasts": {"read": true, "write": false},
			"transactional": {"read": true, "write": false},
			"workspace": {"read": true, "write": false},
			"message_history": {"read": true, "write": false}
		}'::jsonb
		WHERE permissions IS NULL OR permissions = '{}'::jsonb
	`)
	if err != nil {
		return fmt.Errorf("failed to grant default permissions to existing invitations: %w", err)
	}

	// Grant full permissions to all existing workspace users
	// This maintains backward compatibility by preserving current access levels
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
		WHERE permissions IS NULL OR permissions = '{}'::jsonb
	`)
	if err != nil {
		return fmt.Errorf("failed to grant default permissions to existing workspace users: %w", err)
	}

	return nil
}

// UpdateWorkspace executes workspace-level migration changes
func (m *V6Migration) UpdateWorkspace(ctx context.Context, config *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	// No workspace-level changes for v6
	return nil
}

// init registers this migration with the default registry
func init() {
	Register(&V6Migration{})
}
