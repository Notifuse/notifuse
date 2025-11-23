package migrations

import (
	"context"
	"fmt"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
)

// V9Migration implements the migration from version 8.x to 9.0
// Adds message_attachments table and attachments column to message_history
type V9Migration struct{}

// GetMajorVersion returns the major version this migration handles
func (m *V9Migration) GetMajorVersion() float64 {
	return 9.0
}

// HasSystemUpdate indicates if this migration has system-level changes
func (m *V9Migration) HasSystemUpdate() bool {
	return false // No system-level changes needed
}

// HasWorkspaceUpdate indicates if this migration has workspace-level changes
func (m *V9Migration) HasWorkspaceUpdate() bool {
	return true // Adds tables to workspace databases
}

// ShouldRestartServer indicates if the server should restart after this migration
func (m *V9Migration) ShouldRestartServer() bool {
	return false
}

// UpdateSystem executes system-level migration changes (none for v9)
func (m *V9Migration) UpdateSystem(ctx context.Context, config *config.Config, db DBExecutor) error {
	return nil
}

// UpdateWorkspace executes workspace-level migration changes
func (m *V9Migration) UpdateWorkspace(ctx context.Context, config *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	// Create message_attachments table
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS message_attachments (
			checksum VARCHAR(64) PRIMARY KEY,
			content BYTEA NOT NULL,
			content_type VARCHAR(255),
			size_bytes BIGINT NOT NULL,
			created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create message_attachments table for workspace %s: %w", workspace.ID, err)
	}

	// Add attachments column to message_history
	_, err = db.ExecContext(ctx, `
		ALTER TABLE message_history
		ADD COLUMN IF NOT EXISTS attachments JSONB
	`)
	if err != nil {
		return fmt.Errorf("failed to add attachments column to message_history for workspace %s: %w", workspace.ID, err)
	}

	return nil
}

// init registers this migration with the default registry
func init() {
	Register(&V9Migration{})
}
