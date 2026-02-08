package migrations

import (
	"context"
	"fmt"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
)

// V27Migration adds data_feed column to broadcasts and recurring task columns to tasks.
//
// This migration adds:
// - data_feed: JSONB column in broadcasts for DataFeedSettings
// - recurring_interval: INTEGER column in tasks for recurring task scheduling (seconds between runs)
// - integration_id: VARCHAR(36) column in tasks to link to integrations for sync tasks
type V27Migration struct{}

func (m *V27Migration) GetMajorVersion() float64 {
	return 27.0
}

func (m *V27Migration) HasSystemUpdate() bool {
	return true
}

func (m *V27Migration) HasWorkspaceUpdate() bool {
	return true
}

func (m *V27Migration) ShouldRestartServer() bool {
	return false
}

func (m *V27Migration) UpdateSystem(ctx context.Context, cfg *config.Config, db DBExecutor) error {
	// Add recurring_interval column for recurring task scheduling (seconds between runs)
	_, err := db.ExecContext(ctx, `
		ALTER TABLE tasks ADD COLUMN IF NOT EXISTS recurring_interval INTEGER DEFAULT NULL
	`)
	if err != nil {
		return fmt.Errorf("failed to add recurring_interval column: %w", err)
	}

	// Add integration_id column to link tasks to integrations for sync tasks
	_, err = db.ExecContext(ctx, `
		ALTER TABLE tasks ADD COLUMN IF NOT EXISTS integration_id VARCHAR(36) DEFAULT NULL
	`)
	if err != nil {
		return fmt.Errorf("failed to add integration_id column: %w", err)
	}

	// Create index for faster integration_id lookups
	_, err = db.ExecContext(ctx, `
		CREATE INDEX IF NOT EXISTS idx_tasks_integration_id ON tasks(integration_id) WHERE integration_id IS NOT NULL
	`)
	if err != nil {
		return fmt.Errorf("failed to create integration_id index: %w", err)
	}

	// Create unique index to ensure only one active task per workspace/integration
	_, err = db.ExecContext(ctx, `
		CREATE UNIQUE INDEX IF NOT EXISTS idx_tasks_workspace_integration_active
		ON tasks(workspace_id, integration_id)
		WHERE integration_id IS NOT NULL AND status NOT IN ('completed', 'failed')
	`)
	if err != nil {
		return fmt.Errorf("failed to create workspace_integration unique index: %w", err)
	}

	return nil
}

func (m *V27Migration) UpdateWorkspace(ctx context.Context, cfg *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	// Add data_feed column (JSONB for DataFeedSettings - contains global_feed, global_feed_data, global_feed_fetched_at, recipient_feed)
	_, err := db.ExecContext(ctx, `
		ALTER TABLE broadcasts
		ADD COLUMN IF NOT EXISTS data_feed JSONB DEFAULT NULL
	`)
	if err != nil {
		return fmt.Errorf("failed to add data_feed column: %w", err)
	}

	// Drop redundant index on contacts.email — the PRIMARY KEY already creates a unique B-tree index
	_, err = db.ExecContext(ctx, `DROP INDEX IF EXISTS idx_contacts_email`)
	if err != nil {
		return fmt.Errorf("failed to drop redundant idx_contacts_email: %w", err)
	}

	// Add index on contact_lists.list_id — the composite PK (email, list_id) only supports lookups by email
	_, err = db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_contact_lists_list_id ON contact_lists(list_id)`)
	if err != nil {
		return fmt.Errorf("failed to create idx_contact_lists_list_id: %w", err)
	}

	return nil
}

func init() {
	Register(&V27Migration{})
}
