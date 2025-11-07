package migrations

import (
	"context"
	"fmt"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
)

// V17Migration adds web publication support with mailing list structure
// Broadcasts: channels, web_publication_settings JSONB, web_published_at column
// Lists: slug, web_publication_enabled, web_publication_settings
type V17Migration struct{}

func (m *V17Migration) GetMajorVersion() float64 {
	return 17.0
}

func (m *V17Migration) HasSystemUpdate() bool {
	return false
}

func (m *V17Migration) HasWorkspaceUpdate() bool {
	return true
}

func (m *V17Migration) ShouldRestartServer() bool {
	return false
}

func (m *V17Migration) UpdateSystem(ctx context.Context, config *config.Config, db DBExecutor) error {
	// No system-level changes needed
	return nil
}

func (m *V17Migration) UpdateWorkspace(ctx context.Context, config *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	// ===== BROADCASTS TABLE =====

	// Add channels column
	_, err := db.ExecContext(ctx, `
		ALTER TABLE broadcasts
		ADD COLUMN IF NOT EXISTS channels JSONB DEFAULT '{"email": true, "web": false}'::jsonb
	`)
	if err != nil {
		return fmt.Errorf("failed to add channels column to broadcasts: %w", err)
	}

	// Add web_publication_settings column (renamed from web_settings)
	_, err = db.ExecContext(ctx, `
		ALTER TABLE broadcasts
		ADD COLUMN IF NOT EXISTS web_publication_settings JSONB
	`)
	if err != nil {
		return fmt.Errorf("failed to add web_publication_settings column to broadcasts: %w", err)
	}

	// Add web_published_at column (separate from settings for performance)
	_, err = db.ExecContext(ctx, `
		ALTER TABLE broadcasts
		ADD COLUMN IF NOT EXISTS web_published_at TIMESTAMP
	`)
	if err != nil {
		return fmt.Errorf("failed to add web_published_at column to broadcasts: %w", err)
	}

	// Create index for querying published web broadcasts
	_, err = db.ExecContext(ctx, `
		CREATE INDEX IF NOT EXISTS idx_broadcasts_web_published
		ON broadcasts(workspace_id, web_published_at)
		WHERE web_published_at IS NOT NULL
	`)
	if err != nil {
		return fmt.Errorf("failed to create index on web_published_at: %w", err)
	}

	// Update existing broadcasts to have web disabled
	_, err = db.ExecContext(ctx, `
		UPDATE broadcasts
		SET channels = '{"email": true, "web": false}'::jsonb
		WHERE channels IS NULL
	`)
	if err != nil {
		return fmt.Errorf("failed to update existing broadcasts with default channels: %w", err)
	}

	// ===== LISTS TABLE =====

	// Add slug column
	_, err = db.ExecContext(ctx, `
		ALTER TABLE lists
		ADD COLUMN IF NOT EXISTS slug VARCHAR(100)
	`)
	if err != nil {
		return fmt.Errorf("failed to add slug column to lists: %w", err)
	}

	// Add web_publication_enabled column
	_, err = db.ExecContext(ctx, `
		ALTER TABLE lists
		ADD COLUMN IF NOT EXISTS web_publication_enabled BOOLEAN DEFAULT false
	`)
	if err != nil {
		return fmt.Errorf("failed to add web_publication_enabled column to lists: %w", err)
	}

	// Add web_publication_settings column
	_, err = db.ExecContext(ctx, `
		ALTER TABLE lists
		ADD COLUMN IF NOT EXISTS web_publication_settings JSONB
	`)
	if err != nil {
		return fmt.Errorf("failed to add web_publication_settings column to lists: %w", err)
	}

	// Create unique index for list slugs
	// Note: Lists are in workspace-specific databases, so no workspace_id column needed
	_, err = db.ExecContext(ctx, `
		CREATE UNIQUE INDEX IF NOT EXISTS idx_lists_slug
		ON lists(slug)
		WHERE slug IS NOT NULL
	`)
	if err != nil {
		return fmt.Errorf("failed to create unique index on list slugs: %w", err)
	}

	return nil
}

func init() {
	Register(&V17Migration{})
}
