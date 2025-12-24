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

	// PART 2: Add broadcast enqueued_count column for email queue tracking
	// Note: sent_count and failed_count are tracked via message_history table
	_, err = db.ExecContext(ctx, `
		ALTER TABLE broadcasts
		ADD COLUMN IF NOT EXISTS enqueued_count INTEGER DEFAULT 0
	`)
	if err != nil {
		return fmt.Errorf("failed to add broadcast enqueued_count column: %w", err)
	}

	// PART 3: Migrate broadcast statuses from sending/sent to processing/processed
	_, err = db.ExecContext(ctx, `UPDATE broadcasts SET status = 'processing' WHERE status = 'sending'`)
	if err != nil {
		return fmt.Errorf("failed to migrate sending status: %w", err)
	}

	_, err = db.ExecContext(ctx, `UPDATE broadcasts SET status = 'processed' WHERE status = 'sent'`)
	if err != nil {
		return fmt.Errorf("failed to migrate sent status: %w", err)
	}

	// PART 4: Add missing broadcast_id index (was in init.go but never migrated)
	// This fixes schema drift between new workspaces (have index) and existing ones (missing index)
	_, err = db.ExecContext(ctx, `
		CREATE INDEX IF NOT EXISTS idx_message_history_broadcast_id
			ON message_history(broadcast_id)
			WHERE broadcast_id IS NOT NULL
	`)
	if err != nil {
		return fmt.Errorf("failed to create message_history broadcast_id index: %w", err)
	}

	// PART 5: Update automation_enroll_contact function to create timeline events
	// This adds automation.start event when contacts enter automations
	_, err = db.ExecContext(ctx, `
		CREATE OR REPLACE FUNCTION automation_enroll_contact(
			p_automation_id VARCHAR(36),
			p_contact_email VARCHAR(255),
			p_root_node_id VARCHAR(36),
			p_list_id VARCHAR(36),
			p_frequency VARCHAR(20)
		) RETURNS VOID AS $$
		DECLARE
			v_is_subscribed BOOLEAN;
			v_already_triggered BOOLEAN;
			v_new_id VARCHAR(36);
		BEGIN
			-- 1. Check list subscription (only if list_id is provided)
			IF p_list_id IS NOT NULL AND p_list_id != '' THEN
				SELECT EXISTS(
					SELECT 1 FROM contact_lists
					WHERE email = p_contact_email
					AND list_id = p_list_id
					AND status = 'active'
					AND deleted_at IS NULL
				) INTO v_is_subscribed;

				IF NOT v_is_subscribed THEN
					RETURN;  -- Contact not subscribed to list, skip enrollment
				END IF;
			END IF;

			-- 2. For "once" frequency, check if already triggered
			IF p_frequency = 'once' THEN
				SELECT EXISTS(
					SELECT 1 FROM automation_trigger_log
					WHERE automation_id = p_automation_id
					AND contact_email = p_contact_email
				) INTO v_already_triggered;

				IF v_already_triggered THEN
					RETURN;  -- Already triggered for this contact, skip
				END IF;

				-- Record trigger for deduplication
				INSERT INTO automation_trigger_log (id, automation_id, contact_email, triggered_at)
				VALUES (gen_random_uuid()::text, p_automation_id, p_contact_email, NOW())
				ON CONFLICT (automation_id, contact_email) DO NOTHING;
			END IF;

			-- 3. Generate new ID for contact_automation
			v_new_id := gen_random_uuid()::text;

			-- 4. Enroll contact in automation
			INSERT INTO contact_automations (
				id, automation_id, contact_email, current_node_id,
				status, entered_at, scheduled_at
			) VALUES (
				v_new_id,
				p_automation_id,
				p_contact_email,
				p_root_node_id,
				'active',
				NOW(),
				NOW()
			);

			-- 5. Increment enrolled stat
			UPDATE automations
			SET stats = jsonb_set(
				COALESCE(stats, '{}'::jsonb),
				'{enrolled}',
				to_jsonb(COALESCE((stats->>'enrolled')::int, 0) + 1)
			),
			updated_at = NOW()
			WHERE id = p_automation_id;

			-- 6. Log node execution entry
			INSERT INTO automation_node_executions (
				id, contact_automation_id, automation_id, node_id, node_type, action, entered_at, output
			) VALUES (
				gen_random_uuid()::text,
				v_new_id,
				p_automation_id,
				p_root_node_id,
				'trigger',
				'entered',
				NOW(),
				'{}'::jsonb
			);

			-- 7. Create automation.start timeline event
			INSERT INTO contact_timeline (email, operation, entity_type, kind, entity_id, changes, created_at)
			VALUES (
				p_contact_email,
				'insert',
				'automation',
				'automation.start',
				p_automation_id,
				jsonb_build_object(
					'automation_id', jsonb_build_object('new', p_automation_id),
					'root_node_id', jsonb_build_object('new', p_root_node_id)
				),
				NOW()
			);

		END;
		$$ LANGUAGE plpgsql
	`)
	if err != nil {
		return fmt.Errorf("failed to update automation_enroll_contact function: %w", err)
	}

	// PART 6: Add automation_id column to automation_node_executions for analytics
	_, err = db.ExecContext(ctx, `
		ALTER TABLE automation_node_executions
		ADD COLUMN IF NOT EXISTS automation_id VARCHAR(36)
	`)
	if err != nil {
		return fmt.Errorf("failed to add automation_id column: %w", err)
	}

	// Backfill existing data
	_, err = db.ExecContext(ctx, `
		UPDATE automation_node_executions ne
		SET automation_id = ca.automation_id
		FROM contact_automations ca
		WHERE ne.contact_automation_id = ca.id
		AND ne.automation_id IS NULL
	`)
	if err != nil {
		return fmt.Errorf("failed to backfill automation_id: %w", err)
	}

	// Add index for analytics queries
	_, err = db.ExecContext(ctx, `
		CREATE INDEX IF NOT EXISTS idx_node_executions_automation
		ON automation_node_executions(automation_id, node_id, action)
	`)
	if err != nil {
		return fmt.Errorf("failed to create automation_id index: %w", err)
	}

	return nil
}

func init() {
	Register(&V21Migration{})
}
