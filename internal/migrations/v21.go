package migrations

import (
	"context"
	"fmt"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
)

// V21Migration adds email queue tables for unified broadcast and automation email sending
type V21Migration struct{}

func (m *V21Migration) GetMajorVersion() float64 {
	return 21.0
}

func (m *V21Migration) HasSystemUpdate() bool {
	return false
}

func (m *V21Migration) HasWorkspaceUpdate() bool {
	return true
}

func (m *V21Migration) ShouldRestartServer() bool {
	return false
}

func (m *V21Migration) UpdateSystem(ctx context.Context, cfg *config.Config, db DBExecutor) error {
	return nil
}

func (m *V21Migration) UpdateWorkspace(ctx context.Context, cfg *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	// PART 1: Create email_queue table
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS email_queue (
			id VARCHAR(36) PRIMARY KEY,
			status VARCHAR(20) NOT NULL DEFAULT 'pending',
			priority INTEGER NOT NULL DEFAULT 5,
			source_type VARCHAR(20) NOT NULL,
			source_id VARCHAR(36) NOT NULL,
			integration_id VARCHAR(36) NOT NULL,
			provider_kind VARCHAR(20) NOT NULL,
			contact_email VARCHAR(255) NOT NULL,
			message_id VARCHAR(100) NOT NULL,
			template_id VARCHAR(36) NOT NULL,
			payload JSONB NOT NULL,
			attempts INTEGER NOT NULL DEFAULT 0,
			max_attempts INTEGER NOT NULL DEFAULT 3,
			last_error TEXT,
			next_retry_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			processed_at TIMESTAMPTZ
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create email_queue table: %w", err)
	}

	// Index for fetching pending emails by priority and creation time
	// Used by workers to fetch emails in priority order
	// Note: next_retry_at filtering is done at query time since NOW() is not IMMUTABLE
	_, err = db.ExecContext(ctx, `
		CREATE INDEX IF NOT EXISTS idx_email_queue_pending
		ON email_queue(priority ASC, created_at ASC)
		WHERE status = 'pending'
	`)
	if err != nil {
		return fmt.Errorf("failed to create email_queue pending index: %w", err)
	}

	// Index for next_retry_at to support retry filtering
	_, err = db.ExecContext(ctx, `
		CREATE INDEX IF NOT EXISTS idx_email_queue_next_retry
		ON email_queue(next_retry_at)
		WHERE status = 'pending' AND next_retry_at IS NOT NULL
	`)
	if err != nil {
		return fmt.Errorf("failed to create email_queue next_retry index: %w", err)
	}

	// Index for fetching failed emails ready for retry
	_, err = db.ExecContext(ctx, `
		CREATE INDEX IF NOT EXISTS idx_email_queue_retry
		ON email_queue(next_retry_at)
		WHERE status = 'failed' AND attempts < max_attempts
	`)
	if err != nil {
		return fmt.Errorf("failed to create email_queue retry index: %w", err)
	}

	// Index for tracking broadcast/automation progress
	_, err = db.ExecContext(ctx, `
		CREATE INDEX IF NOT EXISTS idx_email_queue_source
		ON email_queue(source_type, source_id, status)
	`)
	if err != nil {
		return fmt.Errorf("failed to create email_queue source index: %w", err)
	}

	// Index for integration-based queries (useful for rate limiting monitoring)
	_, err = db.ExecContext(ctx, `
		CREATE INDEX IF NOT EXISTS idx_email_queue_integration
		ON email_queue(integration_id, status)
	`)
	if err != nil {
		return fmt.Errorf("failed to create email_queue integration index: %w", err)
	}

	// PART 2: Create email_queue_dead_letter table
	_, err = db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS email_queue_dead_letter (
			id VARCHAR(36) PRIMARY KEY,
			original_entry_id VARCHAR(36) NOT NULL,
			source_type VARCHAR(20) NOT NULL,
			source_id VARCHAR(36) NOT NULL,
			contact_email VARCHAR(255) NOT NULL,
			message_id VARCHAR(100) NOT NULL,
			payload JSONB NOT NULL,
			final_error TEXT NOT NULL,
			attempts INTEGER NOT NULL,
			created_at TIMESTAMPTZ NOT NULL,
			failed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create email_queue_dead_letter table: %w", err)
	}

	// Index for investigating dead letter entries by source
	_, err = db.ExecContext(ctx, `
		CREATE INDEX IF NOT EXISTS idx_email_queue_dead_letter_source
		ON email_queue_dead_letter(source_type, source_id, failed_at DESC)
	`)
	if err != nil {
		return fmt.Errorf("failed to create email_queue_dead_letter source index: %w", err)
	}

	// Index for dead letter cleanup
	_, err = db.ExecContext(ctx, `
		CREATE INDEX IF NOT EXISTS idx_email_queue_dead_letter_cleanup
		ON email_queue_dead_letter(failed_at)
	`)
	if err != nil {
		return fmt.Errorf("failed to create email_queue_dead_letter cleanup index: %w", err)
	}

	// PART 3: Add broadcast count columns for email queue tracking
	_, err = db.ExecContext(ctx, `
		ALTER TABLE broadcasts
		ADD COLUMN IF NOT EXISTS enqueued_count INTEGER DEFAULT 0,
		ADD COLUMN IF NOT EXISTS sent_count INTEGER DEFAULT 0,
		ADD COLUMN IF NOT EXISTS failed_count INTEGER DEFAULT 0
	`)
	if err != nil {
		return fmt.Errorf("failed to add broadcast count columns: %w", err)
	}

	// PART 4: Migrate broadcast statuses from sending/sent to processing/processed
	_, err = db.ExecContext(ctx, `UPDATE broadcasts SET status = 'processing' WHERE status = 'sending'`)
	if err != nil {
		return fmt.Errorf("failed to migrate sending status: %w", err)
	}

	_, err = db.ExecContext(ctx, `UPDATE broadcasts SET status = 'processed' WHERE status = 'sent'`)
	if err != nil {
		return fmt.Errorf("failed to migrate sent status: %w", err)
	}

	return nil
}

func init() {
	Register(&V21Migration{})
}
