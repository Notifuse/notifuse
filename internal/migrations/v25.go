package migrations

import (
	"context"
	"fmt"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
)

// V25Migration fixes automation_enroll_contact to not check list subscription.
//
// Bug (GitHub Issue #191): The function incorrectly checked if contact is subscribed
// to automation.list_id before enrolling. But list_id is only for unsubscribe URLs
// in email templates, not enrollment filtering.
//
// Impact: Any automation with a list_id set (required for email nodes) would fail
// to enroll contacts for non-list triggers like contact.created, because a newly
// created contact can never be subscribed to a list yet.
//
// Fix:
// 1. Remove the p_list_id parameter from automation_enroll_contact() entirely
// 2. Regenerate all existing automation trigger functions to use 4 params instead of 5
type V25Migration struct{}

func (m *V25Migration) GetMajorVersion() float64 {
	return 25.0
}

func (m *V25Migration) HasSystemUpdate() bool {
	return false
}

func (m *V25Migration) HasWorkspaceUpdate() bool {
	return true
}

func (m *V25Migration) ShouldRestartServer() bool {
	return false
}

func (m *V25Migration) UpdateSystem(ctx context.Context, cfg *config.Config, db DBExecutor) error {
	return nil
}

func (m *V25Migration) UpdateWorkspace(ctx context.Context, cfg *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	// Step 1: Replace the automation_enroll_contact function with 4 parameters (no p_list_id)
	_, err := db.ExecContext(ctx, `
		CREATE OR REPLACE FUNCTION automation_enroll_contact(
			p_automation_id VARCHAR(36),
			p_contact_email VARCHAR(255),
			p_root_node_id VARCHAR(36),
			p_frequency VARCHAR(20)
		) RETURNS VOID AS $$
		DECLARE
			v_already_triggered BOOLEAN;
			v_new_id VARCHAR(36);
		BEGIN
			-- 1. For "once" frequency, check if already triggered
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

			-- 2. Generate new ID for contact_automation
			v_new_id := gen_random_uuid()::text;

			-- 3. Enroll contact in automation
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

			-- 4. Increment enrolled stat
			UPDATE automations
			SET stats = jsonb_set(
				COALESCE(stats, '{}'::jsonb),
				'{enrolled}',
				to_jsonb(COALESCE((stats->>'enrolled')::int, 0) + 1)
			),
			updated_at = NOW()
			WHERE id = p_automation_id;

			-- 5. Log node execution entry
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

			-- 6. Create automation.start timeline event
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

	// Step 2: Regenerate all automation trigger functions to use 4 parameters
	// Each automation has its own function: automation_trigger_{id}()
	// We need to update them to call automation_enroll_contact with 4 params instead of 5
	// Only regenerate for LIVE automations (draft/paused don't have active trigger functions)
	_, err = db.ExecContext(ctx, `
		DO $$
		DECLARE
			auto RECORD;
			safe_id TEXT;
			function_name TEXT;
			frequency TEXT;
		BEGIN
			-- Loop through all LIVE automations that have triggers
			-- Draft/paused automations don't have trigger functions to regenerate
			FOR auto IN
				SELECT id, root_node_id, trigger_config
				FROM automations
				WHERE trigger_config IS NOT NULL
				AND status = 'live'
				AND root_node_id IS NOT NULL
			LOOP
				-- Generate safe ID (remove hyphens for valid PostgreSQL identifier)
				safe_id := REPLACE(auto.id, '-', '');
				function_name := 'automation_trigger_' || safe_id;

				-- Get frequency from trigger_config, default to 'every_time'
				frequency := COALESCE(auto.trigger_config->>'frequency', 'every_time');

				-- Recreate the function with 4 parameters (no list_id)
				EXECUTE format(
					'CREATE OR REPLACE FUNCTION %I()
					RETURNS TRIGGER AS $func$
					BEGIN
						PERFORM automation_enroll_contact(
							%L,
							NEW.email,
							%L,
							%L
						);
						RETURN NEW;
					END;
					$func$ LANGUAGE plpgsql',
					function_name,
					auto.id,
					auto.root_node_id,
					frequency
				);
			END LOOP;
		END;
		$$
	`)
	if err != nil {
		return fmt.Errorf("failed to regenerate automation trigger functions: %w", err)
	}

	return nil
}

func init() {
	Register(&V25Migration{})
}
