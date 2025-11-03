package migrations

import (
	"context"
	"fmt"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
)

// V16Migration adds integration_id column to templates table
// This allows templates to be marked as managed by integrations (e.g., Supabase)
// Integration-managed templates cannot be deleted by users
type V16Migration struct{}

func (m *V16Migration) GetMajorVersion() float64 {
	return 16.0
}

func (m *V16Migration) HasSystemUpdate() bool {
	return false
}

func (m *V16Migration) HasWorkspaceUpdate() bool {
	return true
}

func (m *V16Migration) ShouldRestartServer() bool {
	return false
}

func (m *V16Migration) UpdateSystem(ctx context.Context, config *config.Config, db DBExecutor) error {
	// No system-level changes needed
	return nil
}

func (m *V16Migration) UpdateWorkspace(ctx context.Context, config *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	// Add integration_id column to templates table
	_, err := db.ExecContext(ctx, `
		ALTER TABLE templates
		ADD COLUMN IF NOT EXISTS integration_id VARCHAR(255) DEFAULT NULL
	`)
	if err != nil {
		return fmt.Errorf("failed to add integration_id column to templates: %w", err)
	}

	// Add integration_id column to transactional_notifications table
	_, err = db.ExecContext(ctx, `
		ALTER TABLE transactional_notifications
		ADD COLUMN IF NOT EXISTS integration_id VARCHAR(255) DEFAULT NULL
	`)
	if err != nil {
		return fmt.Errorf("failed to add integration_id column to transactional_notifications: %w", err)
	}

	// Rename email_provider_kind to source in webhook_events table
	_, err = db.ExecContext(ctx, `
		ALTER TABLE webhook_events
		RENAME COLUMN email_provider_kind TO source
	`)
	if err != nil {
		return fmt.Errorf("failed to rename email_provider_kind to source: %w", err)
	}

	// Make message_id nullable in webhook_events table
	_, err = db.ExecContext(ctx, `
		ALTER TABLE webhook_events
		ALTER COLUMN message_id DROP NOT NULL
	`)
	if err != nil {
		return fmt.Errorf("failed to make message_id nullable: %w", err)
	}

	// Update the webhook event trigger function to handle nullable message_id and renamed column
	_, err = db.ExecContext(ctx, `
		CREATE OR REPLACE FUNCTION track_webhook_event_changes()
		RETURNS TRIGGER AS $$
		DECLARE
			changes_json JSONB := '{}'::jsonb;
			entity_id_value VARCHAR(255);
		BEGIN
			-- Use message_id if available, otherwise use webhook event id
			entity_id_value := COALESCE(NEW.message_id, NEW.id::text);
			
			changes_json := jsonb_build_object('type', jsonb_build_object('new', NEW.type), 'source', jsonb_build_object('new', NEW.source));
			IF NEW.bounce_type IS NOT NULL AND NEW.bounce_type != '' THEN 
				changes_json := changes_json || jsonb_build_object('bounce_type', jsonb_build_object('new', NEW.bounce_type)); 
			END IF;
			IF NEW.bounce_category IS NOT NULL AND NEW.bounce_category != '' THEN 
				changes_json := changes_json || jsonb_build_object('bounce_category', jsonb_build_object('new', NEW.bounce_category)); 
			END IF;
			IF NEW.bounce_diagnostic IS NOT NULL AND NEW.bounce_diagnostic != '' THEN 
				changes_json := changes_json || jsonb_build_object('bounce_diagnostic', jsonb_build_object('new', NEW.bounce_diagnostic)); 
			END IF;
			IF NEW.complaint_feedback_type IS NOT NULL AND NEW.complaint_feedback_type != '' THEN 
				changes_json := changes_json || jsonb_build_object('complaint_feedback_type', jsonb_build_object('new', NEW.complaint_feedback_type)); 
			END IF;
			
			INSERT INTO contact_timeline (email, operation, entity_type, kind, entity_id, changes, created_at) 
			VALUES (NEW.recipient_email, 'insert', 'webhook_event', 'insert_webhook_event', entity_id_value, changes_json, CURRENT_TIMESTAMP);
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;
	`)
	if err != nil {
		return fmt.Errorf("failed to update webhook event trigger function: %w", err)
	}

	return nil
}

func init() {
	Register(&V16Migration{})
}
