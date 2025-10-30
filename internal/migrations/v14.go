package migrations

import (
	"context"
	"fmt"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
)

// V14Migration adds channel_options JSONB column to message_history table
// This allows storing channel-specific delivery options like CC, BCC, FromName, ReplyTo
type V14Migration struct{}

func (m *V14Migration) GetMajorVersion() float64 {
	return 14.0
}

func (m *V14Migration) HasSystemUpdate() bool {
	return false // No system database changes
}

func (m *V14Migration) HasWorkspaceUpdate() bool {
	return true // Workspace database changes
}

func (m *V14Migration) UpdateSystem(ctx context.Context, config *config.Config, db DBExecutor) error {
	// No system updates for v14
	return nil
}

func (m *V14Migration) UpdateWorkspace(ctx context.Context, config *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	// Add channel_options column to message_history table
	_, err := db.ExecContext(ctx, `
		ALTER TABLE message_history
		ADD COLUMN IF NOT EXISTS channel_options JSONB
	`)
	if err != nil {
		return fmt.Errorf("failed to add channel_options column: %w", err)
	}

	return nil
}

func init() {
	Register(&V14Migration{})
}
