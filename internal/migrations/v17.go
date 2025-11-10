package migrations

import (
	"context"
	"fmt"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
)

// V17Migration updates mailing list structure
// Broadcasts: pause_reason, audience.lists -> audience.list
// Message history: list_ids -> list_id
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

	// Add pause_reason column
	_, err := db.ExecContext(ctx, `
		ALTER TABLE broadcasts
		ADD COLUMN IF NOT EXISTS pause_reason TEXT
	`)
	if err != nil {
		return fmt.Errorf("failed to add pause_reason column to broadcasts: %w", err)
	}

	// Migrate audience structure: convert lists array to single list, remove skip_duplicate_emails
	_, err = db.ExecContext(ctx, `
		UPDATE broadcasts
		SET audience = (
			audience 
			- 'lists' 
			- 'skip_duplicate_emails'
			|| jsonb_build_object('list', COALESCE(audience->'lists'->0, 'null'::jsonb))
		)
		WHERE audience ? 'lists'
	`)
	if err != nil {
		return fmt.Errorf("failed to migrate audience structure: %w", err)
	}

	// ===== MESSAGE_HISTORY TABLE =====

	// Add list_id column
	_, err = db.ExecContext(ctx, `
		ALTER TABLE message_history
		ADD COLUMN IF NOT EXISTS list_id VARCHAR(32)
	`)
	if err != nil {
		return fmt.Errorf("failed to add list_id column to message_history: %w", err)
	}

	// Migrate list_ids array to list_id (keep first list)
	_, err = db.ExecContext(ctx, `
		UPDATE message_history
		SET list_id = list_ids[1]
		WHERE list_ids IS NOT NULL AND array_length(list_ids, 1) > 0
	`)
	if err != nil {
		return fmt.Errorf("failed to migrate list_ids to list_id: %w", err)
	}

	// Drop list_ids column
	_, err = db.ExecContext(ctx, `
		ALTER TABLE message_history
		DROP COLUMN IF EXISTS list_ids
	`)
	if err != nil {
		return fmt.Errorf("failed to drop list_ids column from message_history: %w", err)
	}

	// ===== BLOG_CATEGORIES TABLE =====

	// Create blog_categories table
	_, err = db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS blog_categories (
			id VARCHAR(32) PRIMARY KEY,
			slug VARCHAR(100) NOT NULL UNIQUE,
			settings JSONB NOT NULL DEFAULT '{}',
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
			deleted_at TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create blog_categories table: %w", err)
	}

	// Create unique index on slug
	_, err = db.ExecContext(ctx, `
		CREATE UNIQUE INDEX IF NOT EXISTS idx_blog_categories_slug 
		ON blog_categories(slug) 
		WHERE deleted_at IS NULL
	`)
	if err != nil {
		return fmt.Errorf("failed to create idx_blog_categories_workspace_slug index: %w", err)
	}

	// ===== BLOG_POSTS TABLE =====

	// Create blog_posts table
	_, err = db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS blog_posts (
			id VARCHAR(32) PRIMARY KEY,
			category_id VARCHAR(32) REFERENCES blog_categories(id) ON DELETE SET NULL,
			slug VARCHAR(100) NOT NULL UNIQUE,
			settings JSONB NOT NULL DEFAULT '{}',
			published_at TIMESTAMP,
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
			deleted_at TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create blog_posts table: %w", err)
	}

	// Create index on published_at for published posts
	_, err = db.ExecContext(ctx, `
		CREATE INDEX IF NOT EXISTS idx_blog_posts_published 
		ON blog_posts(published_at DESC) 
		WHERE deleted_at IS NULL AND published_at IS NOT NULL
	`)
	if err != nil {
		return fmt.Errorf("failed to create idx_blog_posts_published index: %w", err)
	}

	// Create index on category_id
	_, err = db.ExecContext(ctx, `
		CREATE INDEX IF NOT EXISTS idx_blog_posts_category 
		ON blog_posts(category_id) 
		WHERE deleted_at IS NULL
	`)
	if err != nil {
		return fmt.Errorf("failed to create idx_blog_posts_category index: %w", err)
	}

	// Create unique index on slug
	_, err = db.ExecContext(ctx, `
		CREATE UNIQUE INDEX IF NOT EXISTS idx_blog_posts_slug 
		ON blog_posts(slug) 
		WHERE deleted_at IS NULL
	`)
	if err != nil {
		return fmt.Errorf("failed to create idx_blog_posts_workspace_slug index: %w", err)
	}

	return nil
}

func init() {
	Register(&V17Migration{})
}
