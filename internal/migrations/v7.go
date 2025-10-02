package migrations

import (
	"context"
	"fmt"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
)

// V7Migration implements the migration from version 6.x to 7.0
// Adds contact_timeline table and triggers for tracking contact and contact_list changes
type V7Migration struct{}

// GetMajorVersion returns the major version this migration handles
func (m *V7Migration) GetMajorVersion() float64 {
	return 7.0
}

// HasSystemUpdate indicates if this migration has system-level changes
func (m *V7Migration) HasSystemUpdate() bool {
	return false
}

// HasWorkspaceUpdate indicates if this migration has workspace-level changes
func (m *V7Migration) HasWorkspaceUpdate() bool {
	return true
}

// UpdateSystem executes system-level migration changes
func (m *V7Migration) UpdateSystem(ctx context.Context, config *config.Config, db DBExecutor) error {
	// No system-level changes for v7
	return nil
}

// UpdateWorkspace executes workspace-level migration changes
func (m *V7Migration) UpdateWorkspace(ctx context.Context, config *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	// Create contact_timeline table
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS contact_timeline (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			email VARCHAR(255) NOT NULL,
			operation VARCHAR(20) NOT NULL,
			entity_type VARCHAR(20) NOT NULL,
			changes JSONB,
			entity_id VARCHAR(255),
			created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create contact_timeline table: %w", err)
	}

	// Create indexes
	_, err = db.ExecContext(ctx, `
		CREATE INDEX IF NOT EXISTS idx_contact_timeline_email ON contact_timeline(email)
	`)
	if err != nil {
		return fmt.Errorf("failed to create email index: %w", err)
	}

	_, err = db.ExecContext(ctx, `
		CREATE INDEX IF NOT EXISTS idx_contact_timeline_created_at ON contact_timeline(created_at DESC)
	`)
	if err != nil {
		return fmt.Errorf("failed to create created_at index: %w", err)
	}

	_, err = db.ExecContext(ctx, `
		CREATE INDEX IF NOT EXISTS idx_contact_timeline_entity_id ON contact_timeline(entity_id) WHERE entity_id IS NOT NULL
	`)
	if err != nil {
		return fmt.Errorf("failed to create entity_id index: %w", err)
	}

	// Create trigger function for contacts table
	_, err = db.ExecContext(ctx, `
		CREATE OR REPLACE FUNCTION track_contact_changes()
		RETURNS TRIGGER AS $$
		DECLARE
			changes_json JSONB := '{}'::jsonb;
			op VARCHAR(20);
		BEGIN
			IF TG_OP = 'INSERT' THEN
				op := 'insert';
				changes_json := NULL;
				
			ELSIF TG_OP = 'UPDATE' THEN
				op := 'update';
				
				-- Track all field changes
				IF OLD.external_id IS DISTINCT FROM NEW.external_id THEN
					changes_json := changes_json || jsonb_build_object('external_id', jsonb_build_object('old', OLD.external_id, 'new', NEW.external_id));
				END IF;
				IF OLD.timezone IS DISTINCT FROM NEW.timezone THEN
					changes_json := changes_json || jsonb_build_object('timezone', jsonb_build_object('old', OLD.timezone, 'new', NEW.timezone));
				END IF;
				IF OLD.language IS DISTINCT FROM NEW.language THEN
					changes_json := changes_json || jsonb_build_object('language', jsonb_build_object('old', OLD.language, 'new', NEW.language));
				END IF;
				IF OLD.first_name IS DISTINCT FROM NEW.first_name THEN
					changes_json := changes_json || jsonb_build_object('first_name', jsonb_build_object('old', OLD.first_name, 'new', NEW.first_name));
				END IF;
				IF OLD.last_name IS DISTINCT FROM NEW.last_name THEN
					changes_json := changes_json || jsonb_build_object('last_name', jsonb_build_object('old', OLD.last_name, 'new', NEW.last_name));
				END IF;
				IF OLD.phone IS DISTINCT FROM NEW.phone THEN
					changes_json := changes_json || jsonb_build_object('phone', jsonb_build_object('old', OLD.phone, 'new', NEW.phone));
				END IF;
				IF OLD.address_line_1 IS DISTINCT FROM NEW.address_line_1 THEN
					changes_json := changes_json || jsonb_build_object('address_line_1', jsonb_build_object('old', OLD.address_line_1, 'new', NEW.address_line_1));
				END IF;
				IF OLD.address_line_2 IS DISTINCT FROM NEW.address_line_2 THEN
					changes_json := changes_json || jsonb_build_object('address_line_2', jsonb_build_object('old', OLD.address_line_2, 'new', NEW.address_line_2));
				END IF;
				IF OLD.country IS DISTINCT FROM NEW.country THEN
					changes_json := changes_json || jsonb_build_object('country', jsonb_build_object('old', OLD.country, 'new', NEW.country));
				END IF;
				IF OLD.postcode IS DISTINCT FROM NEW.postcode THEN
					changes_json := changes_json || jsonb_build_object('postcode', jsonb_build_object('old', OLD.postcode, 'new', NEW.postcode));
				END IF;
				IF OLD.state IS DISTINCT FROM NEW.state THEN
					changes_json := changes_json || jsonb_build_object('state', jsonb_build_object('old', OLD.state, 'new', NEW.state));
				END IF;
				IF OLD.job_title IS DISTINCT FROM NEW.job_title THEN
					changes_json := changes_json || jsonb_build_object('job_title', jsonb_build_object('old', OLD.job_title, 'new', NEW.job_title));
				END IF;
				IF OLD.lifetime_value IS DISTINCT FROM NEW.lifetime_value THEN
					changes_json := changes_json || jsonb_build_object('lifetime_value', jsonb_build_object('old', OLD.lifetime_value, 'new', NEW.lifetime_value));
				END IF;
				IF OLD.orders_count IS DISTINCT FROM NEW.orders_count THEN
					changes_json := changes_json || jsonb_build_object('orders_count', jsonb_build_object('old', OLD.orders_count, 'new', NEW.orders_count));
				END IF;
				IF OLD.last_order_at IS DISTINCT FROM NEW.last_order_at THEN
					changes_json := changes_json || jsonb_build_object('last_order_at', jsonb_build_object('old', OLD.last_order_at, 'new', NEW.last_order_at));
				END IF;
				
				-- Custom string fields
				IF OLD.custom_string_1 IS DISTINCT FROM NEW.custom_string_1 THEN
					changes_json := changes_json || jsonb_build_object('custom_string_1', jsonb_build_object('old', OLD.custom_string_1, 'new', NEW.custom_string_1));
				END IF;
				IF OLD.custom_string_2 IS DISTINCT FROM NEW.custom_string_2 THEN
					changes_json := changes_json || jsonb_build_object('custom_string_2', jsonb_build_object('old', OLD.custom_string_2, 'new', NEW.custom_string_2));
				END IF;
				IF OLD.custom_string_3 IS DISTINCT FROM NEW.custom_string_3 THEN
					changes_json := changes_json || jsonb_build_object('custom_string_3', jsonb_build_object('old', OLD.custom_string_3, 'new', NEW.custom_string_3));
				END IF;
				IF OLD.custom_string_4 IS DISTINCT FROM NEW.custom_string_4 THEN
					changes_json := changes_json || jsonb_build_object('custom_string_4', jsonb_build_object('old', OLD.custom_string_4, 'new', NEW.custom_string_4));
				END IF;
				IF OLD.custom_string_5 IS DISTINCT FROM NEW.custom_string_5 THEN
					changes_json := changes_json || jsonb_build_object('custom_string_5', jsonb_build_object('old', OLD.custom_string_5, 'new', NEW.custom_string_5));
				END IF;
				
				-- Custom number fields
				IF OLD.custom_number_1 IS DISTINCT FROM NEW.custom_number_1 THEN
					changes_json := changes_json || jsonb_build_object('custom_number_1', jsonb_build_object('old', OLD.custom_number_1, 'new', NEW.custom_number_1));
				END IF;
				IF OLD.custom_number_2 IS DISTINCT FROM NEW.custom_number_2 THEN
					changes_json := changes_json || jsonb_build_object('custom_number_2', jsonb_build_object('old', OLD.custom_number_2, 'new', NEW.custom_number_2));
				END IF;
				IF OLD.custom_number_3 IS DISTINCT FROM NEW.custom_number_3 THEN
					changes_json := changes_json || jsonb_build_object('custom_number_3', jsonb_build_object('old', OLD.custom_number_3, 'new', NEW.custom_number_3));
				END IF;
				IF OLD.custom_number_4 IS DISTINCT FROM NEW.custom_number_4 THEN
					changes_json := changes_json || jsonb_build_object('custom_number_4', jsonb_build_object('old', OLD.custom_number_4, 'new', NEW.custom_number_4));
				END IF;
				IF OLD.custom_number_5 IS DISTINCT FROM NEW.custom_number_5 THEN
					changes_json := changes_json || jsonb_build_object('custom_number_5', jsonb_build_object('old', OLD.custom_number_5, 'new', NEW.custom_number_5));
				END IF;
				
				-- Custom datetime fields
				IF OLD.custom_datetime_1 IS DISTINCT FROM NEW.custom_datetime_1 THEN
					changes_json := changes_json || jsonb_build_object('custom_datetime_1', jsonb_build_object('old', OLD.custom_datetime_1, 'new', NEW.custom_datetime_1));
				END IF;
				IF OLD.custom_datetime_2 IS DISTINCT FROM NEW.custom_datetime_2 THEN
					changes_json := changes_json || jsonb_build_object('custom_datetime_2', jsonb_build_object('old', OLD.custom_datetime_2, 'new', NEW.custom_datetime_2));
				END IF;
				IF OLD.custom_datetime_3 IS DISTINCT FROM NEW.custom_datetime_3 THEN
					changes_json := changes_json || jsonb_build_object('custom_datetime_3', jsonb_build_object('old', OLD.custom_datetime_3, 'new', NEW.custom_datetime_3));
				END IF;
				IF OLD.custom_datetime_4 IS DISTINCT FROM NEW.custom_datetime_4 THEN
					changes_json := changes_json || jsonb_build_object('custom_datetime_4', jsonb_build_object('old', OLD.custom_datetime_4, 'new', NEW.custom_datetime_4));
				END IF;
				IF OLD.custom_datetime_5 IS DISTINCT FROM NEW.custom_datetime_5 THEN
					changes_json := changes_json || jsonb_build_object('custom_datetime_5', jsonb_build_object('old', OLD.custom_datetime_5, 'new', NEW.custom_datetime_5));
				END IF;
				
				-- Custom JSON fields
				IF OLD.custom_json_1 IS DISTINCT FROM NEW.custom_json_1 THEN
					changes_json := changes_json || jsonb_build_object('custom_json_1', jsonb_build_object('old', OLD.custom_json_1, 'new', NEW.custom_json_1));
				END IF;
				IF OLD.custom_json_2 IS DISTINCT FROM NEW.custom_json_2 THEN
					changes_json := changes_json || jsonb_build_object('custom_json_2', jsonb_build_object('old', OLD.custom_json_2, 'new', NEW.custom_json_2));
				END IF;
				IF OLD.custom_json_3 IS DISTINCT FROM NEW.custom_json_3 THEN
					changes_json := changes_json || jsonb_build_object('custom_json_3', jsonb_build_object('old', OLD.custom_json_3, 'new', NEW.custom_json_3));
				END IF;
				IF OLD.custom_json_4 IS DISTINCT FROM NEW.custom_json_4 THEN
					changes_json := changes_json || jsonb_build_object('custom_json_4', jsonb_build_object('old', OLD.custom_json_4, 'new', NEW.custom_json_4));
				END IF;
				IF OLD.custom_json_5 IS DISTINCT FROM NEW.custom_json_5 THEN
					changes_json := changes_json || jsonb_build_object('custom_json_5', jsonb_build_object('old', OLD.custom_json_5, 'new', NEW.custom_json_5));
				END IF;
				
				-- Skip if no actual changes
				IF changes_json = '{}'::jsonb THEN
					RETURN NEW;
				END IF;
			END IF;

			INSERT INTO contact_timeline (email, operation, entity_type, changes)
			VALUES (NEW.email, op, 'contact', changes_json);

			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;
	`)
	if err != nil {
		return fmt.Errorf("failed to create track_contact_changes function: %w", err)
	}

	// Create trigger for contacts table
	_, err = db.ExecContext(ctx, `
		DROP TRIGGER IF EXISTS contact_changes_trigger ON contacts;
		CREATE TRIGGER contact_changes_trigger
		AFTER INSERT OR UPDATE ON contacts
		FOR EACH ROW EXECUTE FUNCTION track_contact_changes();
	`)
	if err != nil {
		return fmt.Errorf("failed to create contact_changes_trigger: %w", err)
	}

	// Create trigger function for contact_lists table
	_, err = db.ExecContext(ctx, `
		CREATE OR REPLACE FUNCTION track_contact_list_changes()
		RETURNS TRIGGER AS $$
		DECLARE
			changes_json JSONB := '{}'::jsonb;
			op VARCHAR(20);
		BEGIN
			IF TG_OP = 'INSERT' THEN
				op := 'insert';
				changes_json := jsonb_build_object(
					'list_id', jsonb_build_object('new', NEW.list_id),
					'status', jsonb_build_object('new', NEW.status)
				);
				
			ELSIF TG_OP = 'UPDATE' THEN
				op := 'update';
				
				IF OLD.status IS DISTINCT FROM NEW.status THEN
					changes_json := changes_json || jsonb_build_object('status', jsonb_build_object('old', OLD.status, 'new', NEW.status));
				END IF;
				
				IF OLD.deleted_at IS DISTINCT FROM NEW.deleted_at THEN
					changes_json := changes_json || jsonb_build_object('deleted_at', jsonb_build_object('old', OLD.deleted_at, 'new', NEW.deleted_at));
				END IF;
				
				IF changes_json = '{}'::jsonb THEN
					RETURN NEW;
				END IF;
			END IF;

			INSERT INTO contact_timeline (email, operation, entity_type, entity_id, changes)
			VALUES (NEW.email, op, 'contact_list', NEW.list_id, changes_json);

			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;
	`)
	if err != nil {
		return fmt.Errorf("failed to create track_contact_list_changes function: %w", err)
	}

	// Create trigger for contact_lists table
	_, err = db.ExecContext(ctx, `
		DROP TRIGGER IF EXISTS contact_list_changes_trigger ON contact_lists;
		CREATE TRIGGER contact_list_changes_trigger
		AFTER INSERT OR UPDATE ON contact_lists
		FOR EACH ROW EXECUTE FUNCTION track_contact_list_changes();
	`)
	if err != nil {
		return fmt.Errorf("failed to create contact_list_changes_trigger: %w", err)
	}

	return nil
}

// init registers this migration with the default registry
func init() {
	Register(&V7Migration{})
}
