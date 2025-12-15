package migrations

import (
	"context"
	"fmt"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
)

// V20Migration adds automation system tables
type V20Migration struct{}

func (m *V20Migration) GetMajorVersion() float64 {
	return 20.0
}

func (m *V20Migration) HasSystemUpdate() bool {
	return true
}

func (m *V20Migration) HasWorkspaceUpdate() bool {
	return true
}

func (m *V20Migration) ShouldRestartServer() bool {
	return false
}

func (m *V20Migration) UpdateSystem(ctx context.Context, config *config.Config, db DBExecutor) error {
	// Add automations permissions to all existing user workspaces
	_, err := db.ExecContext(ctx, `
		UPDATE user_workspaces
		SET permissions = permissions || '{"automations": {"read": true, "write": true}}'::jsonb
		WHERE permissions IS NOT NULL
		AND NOT permissions ? 'automations'
	`)
	if err != nil {
		return fmt.Errorf("failed to add automations permissions to user workspaces: %w", err)
	}

	// Add automations permissions to all existing workspace invitations
	_, err = db.ExecContext(ctx, `
		UPDATE workspace_invitations
		SET permissions = permissions || '{"automations": {"read": true, "write": true}}'::jsonb
		WHERE permissions IS NOT NULL
		AND NOT permissions ? 'automations'
	`)
	if err != nil {
		return fmt.Errorf("failed to add automations permissions to workspace invitations: %w", err)
	}

	return nil
}

func (m *V20Migration) UpdateWorkspace(ctx context.Context, config *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	// PART 1: Create automations table (nodes embedded as JSONB)
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS automations (
			id VARCHAR(36) PRIMARY KEY,
			workspace_id VARCHAR(36) NOT NULL,
			name VARCHAR(255) NOT NULL,
			status VARCHAR(20) DEFAULT 'draft',
			list_id VARCHAR(36),
			trigger_config JSONB NOT NULL,
			trigger_sql TEXT,
			root_node_id VARCHAR(36),
			nodes JSONB DEFAULT '[]',
			stats JSONB DEFAULT '{}',
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW(),
			deleted_at TIMESTAMPTZ
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create automations table: %w", err)
	}

	_, err = db.ExecContext(ctx, `
		CREATE INDEX IF NOT EXISTS idx_automations_workspace_status
			ON automations(workspace_id, status)
			WHERE deleted_at IS NULL
	`)
	if err != nil {
		return fmt.Errorf("failed to create automations workspace_status index: %w", err)
	}

	_, err = db.ExecContext(ctx, `
		CREATE INDEX IF NOT EXISTS idx_automations_list
			ON automations(list_id, status)
	`)
	if err != nil {
		return fmt.Errorf("failed to create automations list index: %w", err)
	}

	// PART 2: Create contact_automations table (with exit_reason)
	_, err = db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS contact_automations (
			id VARCHAR(36) PRIMARY KEY,
			automation_id VARCHAR(36) NOT NULL REFERENCES automations(id),
			contact_email VARCHAR(255) NOT NULL,
			current_node_id VARCHAR(36),
			status VARCHAR(20) DEFAULT 'active',
			exit_reason VARCHAR(50),
			entered_at TIMESTAMPTZ DEFAULT NOW(),
			scheduled_at TIMESTAMPTZ,
			context JSONB DEFAULT '{}',
			retry_count INTEGER DEFAULT 0,
			last_error TEXT,
			last_retry_at TIMESTAMPTZ,
			max_retries INTEGER DEFAULT 3,
			UNIQUE(automation_id, contact_email, entered_at)
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create contact_automations table: %w", err)
	}

	_, err = db.ExecContext(ctx, `
		CREATE INDEX IF NOT EXISTS idx_contact_automations_scheduled
			ON contact_automations(scheduled_at)
			WHERE status = 'active' AND scheduled_at IS NOT NULL
	`)
	if err != nil {
		return fmt.Errorf("failed to create contact_automations scheduled index: %w", err)
	}

	_, err = db.ExecContext(ctx, `
		CREATE INDEX IF NOT EXISTS idx_contact_automations_automation
			ON contact_automations(automation_id, status)
	`)
	if err != nil {
		return fmt.Errorf("failed to create contact_automations automation index: %w", err)
	}

	_, err = db.ExecContext(ctx, `
		CREATE INDEX IF NOT EXISTS idx_contact_automations_email
			ON contact_automations(contact_email, status)
	`)
	if err != nil {
		return fmt.Errorf("failed to create contact_automations email index: %w", err)
	}

	// PART 3: Create automation_node_executions table
	_, err = db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS automation_node_executions (
			id VARCHAR(36) PRIMARY KEY,
			contact_automation_id VARCHAR(36) NOT NULL REFERENCES contact_automations(id) ON DELETE CASCADE,
			node_id VARCHAR(36) NOT NULL,
			node_type VARCHAR(50) NOT NULL,
			action VARCHAR(20) NOT NULL,
			entered_at TIMESTAMPTZ DEFAULT NOW(),
			completed_at TIMESTAMPTZ,
			duration_ms INTEGER,
			output JSONB DEFAULT '{}',
			error TEXT
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create automation_node_executions table: %w", err)
	}

	_, err = db.ExecContext(ctx, `
		CREATE INDEX IF NOT EXISTS idx_node_executions_contact_automation
			ON automation_node_executions(contact_automation_id, entered_at DESC)
	`)
	if err != nil {
		return fmt.Errorf("failed to create automation_node_executions index: %w", err)
	}

	// PART 4: Create automation_trigger_log table (for "once" frequency deduplication)
	_, err = db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS automation_trigger_log (
			id VARCHAR(36) PRIMARY KEY,
			automation_id VARCHAR(36) NOT NULL REFERENCES automations(id),
			contact_email VARCHAR(255) NOT NULL,
			triggered_at TIMESTAMPTZ DEFAULT NOW(),
			UNIQUE(automation_id, contact_email)
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create automation_trigger_log table: %w", err)
	}

	_, err = db.ExecContext(ctx, `
		CREATE INDEX IF NOT EXISTS idx_trigger_log_automation
			ON automation_trigger_log(automation_id, triggered_at DESC)
	`)
	if err != nil {
		return fmt.Errorf("failed to create automation_trigger_log automation index: %w", err)
	}

	_, err = db.ExecContext(ctx, `
		CREATE INDEX IF NOT EXISTS idx_trigger_log_contact
			ON automation_trigger_log(contact_email, automation_id)
	`)
	if err != nil {
		return fmt.Errorf("failed to create automation_trigger_log contact index: %w", err)
	}

	// PART 5: Create automation_enroll_contact function
	// This function is called by per-automation triggers to enroll contacts
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
				id, contact_automation_id, node_id, node_type, action, entered_at, output
			) VALUES (
				gen_random_uuid()::text,
				v_new_id,
				p_root_node_id,
				'trigger',
				'entered',
				NOW(),
				'{}'::jsonb
			);

		END;
		$$ LANGUAGE plpgsql
	`)
	if err != nil {
		return fmt.Errorf("failed to create automation_enroll_contact function: %w", err)
	}

	// PART 6: Add automation_id column to message_history
	// This allows tracking which automation sent each email
	_, err = db.ExecContext(ctx, `
		ALTER TABLE message_history
		ADD COLUMN IF NOT EXISTS automation_id VARCHAR(36)
	`)
	if err != nil {
		return fmt.Errorf("failed to add automation_id column to message_history: %w", err)
	}

	_, err = db.ExecContext(ctx, `
		CREATE INDEX IF NOT EXISTS idx_message_history_automation_id
			ON message_history(automation_id)
			WHERE automation_id IS NOT NULL
	`)
	if err != nil {
		return fmt.Errorf("failed to create message_history automation_id index: %w", err)
	}

	return nil
}

func init() {
	Register(&V20Migration{})
}
