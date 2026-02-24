package migrations

import (
	"context"
	"fmt"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
)

// V28Migration adds template i18n support.
type V28Migration struct{}

func (m *V28Migration) GetMajorVersion() float64 { return 28.0 }
func (m *V28Migration) HasSystemUpdate() bool     { return false }
func (m *V28Migration) HasWorkspaceUpdate() bool   { return true }
func (m *V28Migration) ShouldRestartServer() bool  { return false }

func (m *V28Migration) UpdateSystem(ctx context.Context, cfg *config.Config, db DBExecutor) error {
	return nil
}

func (m *V28Migration) UpdateWorkspace(ctx context.Context, cfg *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	// Add translations column to templates table
	_, err := db.ExecContext(ctx, `
		ALTER TABLE templates
		ADD COLUMN IF NOT EXISTS translations JSONB NOT NULL DEFAULT '{}'::jsonb
	`)
	if err != nil {
		return fmt.Errorf("failed to add translations column: %w", err)
	}

	// Add default_language column to templates table
	_, err = db.ExecContext(ctx, `
		ALTER TABLE templates
		ADD COLUMN IF NOT EXISTS default_language VARCHAR(10) DEFAULT NULL
	`)
	if err != nil {
		return fmt.Errorf("failed to add default_language column: %w", err)
	}

	// Create workspace_translations table
	_, err = db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS workspace_translations (
			locale VARCHAR(10) NOT NULL PRIMARY KEY,
			content JSONB NOT NULL DEFAULT '{}'::jsonb,
			created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create workspace_translations table: %w", err)
	}

	return nil
}

func init() {
	Register(&V28Migration{})
}
