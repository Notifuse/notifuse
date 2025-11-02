package migrations

import (
	"context"
	"fmt"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
)

// V16Migration adds integration_id column to templates table
// This allows templates to be marked as managed by integrations (e.g., Supabase)
// Integration-managed templates cannot be deleted by users
type V16Migration struct{}

func (m *V16Migration) GetMajorVersion() float64 {
	return 16.0
}

func (m *V16Migration) HasSystemUpdate() bool {
	return false
}

func (m *V16Migration) HasWorkspaceUpdate() bool {
	return true
}

func (m *V16Migration) ShouldRestartServer() bool {
	return false
}

func (m *V16Migration) UpdateSystem(ctx context.Context, config *config.Config, db DBExecutor) error {
	// No system-level changes needed
	return nil
}

func (m *V16Migration) UpdateWorkspace(ctx context.Context, config *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	// Add integration_id column to templates table
	_, err := db.ExecContext(ctx, `
		ALTER TABLE templates
		ADD COLUMN IF NOT EXISTS integration_id VARCHAR(255) DEFAULT NULL
	`)
	if err != nil {
		return fmt.Errorf("failed to add integration_id column to templates: %w", err)
	}

	// Add integration_id column to transactional_notifications table
	_, err = db.ExecContext(ctx, `
		ALTER TABLE transactional_notifications
		ADD COLUMN IF NOT EXISTS integration_id VARCHAR(255) DEFAULT NULL
	`)
	if err != nil {
		return fmt.Errorf("failed to add integration_id column to transactional_notifications: %w", err)
	}

	return nil
}

func init() {
	Register(&V16Migration{})
}
