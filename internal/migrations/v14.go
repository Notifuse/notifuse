package migrations

import (
	"context"
	"fmt"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
)

// V14Migration implements the migration from version 13.x to 14.0
// Adds channel_options column to message_history table to store email delivery options
// like CC, BCC, FromName overrides, and ReplyTo addresses
type V14Migration struct{}

// GetMajorVersion returns the major version this migration handles
func (m *V14Migration) GetMajorVersion() float64 {
	return 14.0
}

// HasSystemUpdate indicates if this migration has system-level changes
func (m *V14Migration) HasSystemUpdate() bool {
	return false // No system-level changes needed
}

// HasWorkspaceUpdate indicates if this migration has workspace-level changes
func (m *V14Migration) HasWorkspaceUpdate() bool {
	return true // Adds channel_options column to message_history table
}

// UpdateSystem executes system-level migration changes (none for v14)
func (m *V14Migration) UpdateSystem(ctx context.Context, config *config.Config, db DBExecutor) error {
	return nil
}

// UpdateWorkspace executes workspace-level migration changes
func (m *V14Migration) UpdateWorkspace(ctx context.Context, config *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	// Add channel_options column to message_history table
	// This column stores channel-specific delivery options for email:
	// - CC, BCC: Additional recipients
	// - FromName: Override sender display name
	// - ReplyTo: Reply-to address override
	_, err := db.ExecContext(ctx, `
		ALTER TABLE message_history
		ADD COLUMN IF NOT EXISTS channel_options JSONB DEFAULT NULL
	`)
	if err != nil {
		return fmt.Errorf("failed to add channel_options column to message_history for workspace %s: %w", workspace.ID, err)
	}

	// Create GIN index for channel_options JSONB column
	// Enables efficient querying like:
	// - Find all messages with CC recipients
	// - Find messages sent with specific from_name override
	_, err = db.ExecContext(ctx, `
		CREATE INDEX IF NOT EXISTS idx_message_history_channel_options 
		ON message_history USING gin(channel_options)
	`)
	if err != nil {
		return fmt.Errorf("failed to create index on channel_options for workspace %s: %w", workspace.ID, err)
	}

	return nil
}

// init registers this migration with the default registry
func init() {
	Register(&V14Migration{})
}
