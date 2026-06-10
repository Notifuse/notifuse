package migrations

import (
	"context"
	"fmt"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
)

// V33Migration adds inbound reply detection support.
//
// It redefines the workspace-level track_inbound_webhook_event_changes()
// trigger function so that inbound webhook events of type "reply" are recorded
// on the contact timeline with the dedicated kind "email.replied" (instead of
// the generic "insert_inbound_webhook_event"). This lets automations trigger on
// replies — enabling stop-on-reply sequences. All other inbound webhook events
// (delivered/bounce/complaint) keep their existing kind, so behaviour for
// existing data is unchanged.
//
// The function is replaced with CREATE OR REPLACE; the existing trigger keeps
// pointing at it, so no trigger recreation is needed.
type V33Migration struct{}

func (m *V33Migration) GetMajorVersion() float64 {
	return 33.0
}

func (m *V33Migration) HasSystemUpdate() bool {
	return false
}

func (m *V33Migration) HasWorkspaceUpdate() bool {
	return true
}

func (m *V33Migration) ShouldRestartServer() bool {
	return false
}

func (m *V33Migration) UpdateSystem(ctx context.Context, cfg *config.Config, db DBExecutor) error {
	return nil
}

func (m *V33Migration) UpdateWorkspace(ctx context.Context, cfg *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	_, err := db.ExecContext(ctx, `
		CREATE OR REPLACE FUNCTION track_inbound_webhook_event_changes()
		RETURNS TRIGGER AS $$
		DECLARE
			changes_json JSONB := '{}'::jsonb;
			entity_id_value VARCHAR(255);
			kind_value VARCHAR(50);
		BEGIN
			-- Use message_id if available, otherwise use inbound webhook event id
			entity_id_value := COALESCE(NEW.message_id, NEW.id::text);

			-- Reply events get a dedicated timeline kind ("email.replied") so
			-- automations can trigger on them; all other inbound webhook events
			-- keep the generic kind.
			IF NEW.type = 'reply' THEN
				kind_value := 'email.replied';
			ELSE
				kind_value := 'insert_inbound_webhook_event';
			END IF;

			changes_json := jsonb_build_object('type', jsonb_build_object('new', NEW.type), 'source', jsonb_build_object('new', NEW.source));
			IF NEW.bounce_type IS NOT NULL AND NEW.bounce_type != '' THEN changes_json := changes_json || jsonb_build_object('bounce_type', jsonb_build_object('new', NEW.bounce_type)); END IF;
			IF NEW.bounce_category IS NOT NULL AND NEW.bounce_category != '' THEN changes_json := changes_json || jsonb_build_object('bounce_category', jsonb_build_object('new', NEW.bounce_category)); END IF;
			IF NEW.bounce_diagnostic IS NOT NULL AND NEW.bounce_diagnostic != '' THEN changes_json := changes_json || jsonb_build_object('bounce_diagnostic', jsonb_build_object('new', NEW.bounce_diagnostic)); END IF;
			IF NEW.complaint_feedback_type IS NOT NULL AND NEW.complaint_feedback_type != '' THEN changes_json := changes_json || jsonb_build_object('complaint_feedback_type', jsonb_build_object('new', NEW.complaint_feedback_type)); END IF;
			INSERT INTO contact_timeline (email, operation, entity_type, kind, entity_id, changes, created_at)
			VALUES (NEW.recipient_email, 'insert', 'inbound_webhook_event', kind_value, entity_id_value, changes_json, CURRENT_TIMESTAMP);
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;
	`)
	if err != nil {
		return fmt.Errorf("failed to update track_inbound_webhook_event_changes trigger function: %w", err)
	}
	return nil
}

func init() {
	Register(&V33Migration{})
}
