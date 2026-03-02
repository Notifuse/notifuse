package migrations

import (
	"context"
	"fmt"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
)

// V28Migration adds language support to workspaces and translations to templates.
//
// This migration adds:
// - System: backfills workspace settings with default_language and languages
// - Workspace: adds translations JSONB column to templates table
type V28Migration struct{}

func (m *V28Migration) GetMajorVersion() float64 {
	return 28.0
}

func (m *V28Migration) HasSystemUpdate() bool {
	return true
}

func (m *V28Migration) HasWorkspaceUpdate() bool {
	return true
}

func (m *V28Migration) ShouldRestartServer() bool {
	return false
}

func (m *V28Migration) UpdateSystem(ctx context.Context, cfg *config.Config, db DBExecutor) error {
	// Backfill workspace settings with default_language and languages for existing workspaces
	_, err := db.ExecContext(ctx, `
		UPDATE workspaces SET settings = settings || '{"default_language": "en", "languages": ["en"]}'::jsonb
		WHERE NOT (settings ? 'default_language')
	`)
	if err != nil {
		return fmt.Errorf("failed to backfill workspace language settings: %w", err)
	}

	return nil
}

func (m *V28Migration) UpdateWorkspace(ctx context.Context, cfg *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	// Add translations column to templates table with DEFAULT '{}'
	_, err := db.ExecContext(ctx, `
		ALTER TABLE templates ADD COLUMN IF NOT EXISTS translations JSONB DEFAULT '{}'::jsonb
	`)
	if err != nil {
		return fmt.Errorf("failed to add translations column to templates: %w", err)
	}

	// Normalize any existing NULL translations to empty JSON object
	_, err = db.ExecContext(ctx, `
		UPDATE templates SET translations = '{}'::jsonb WHERE translations IS NULL
	`)
	if err != nil {
		return fmt.Errorf("failed to normalize NULL translations: %w", err)
	}

	return nil
}

func init() {
	Register(&V28Migration{})
}
