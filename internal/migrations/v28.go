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

	// Add transactional_notification_id column to message_history
	_, err = db.ExecContext(ctx, `
		ALTER TABLE message_history ADD COLUMN IF NOT EXISTS transactional_notification_id VARCHAR(32)
	`)
	if err != nil {
		return fmt.Errorf("failed to add transactional_notification_id column: %w", err)
	}

	// Create partial index for transactional_notification_id
	_, err = db.ExecContext(ctx, `
		CREATE INDEX IF NOT EXISTS idx_message_history_transactional_notification_id
		ON message_history(transactional_notification_id)
		WHERE transactional_notification_id IS NOT NULL
	`)
	if err != nil {
		return fmt.Errorf("failed to create transactional_notification_id index: %w", err)
	}

	// Backfill transactional_notification_id via JOIN on template_id.
	// Note: if multiple notifications share the same template_id, the match is
	// non-deterministic. This is a best-effort backfill for historical data.
	_, err = db.ExecContext(ctx, `
		UPDATE message_history mh
		SET transactional_notification_id = tn.id
		FROM transactional_notifications tn
		WHERE mh.template_id = tn.channels->'email'->>'template_id'
		  AND mh.broadcast_id IS NULL
		  AND mh.transactional_notification_id IS NULL
		  AND tn.deleted_at IS NULL
	`)
	if err != nil {
		return fmt.Errorf("failed to backfill transactional_notification_id: %w", err)
	}

	return nil
}

func init() {
	Register(&V28Migration{})
}
