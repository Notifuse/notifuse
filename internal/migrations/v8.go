package migrations

import (
	"context"
	"fmt"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
)

// V8Migration implements the migration from version 7.x to 8.0
// Adds kind and db_created_at columns to contact_timeline table
// Removes default from created_at to support historical data imports
type V8Migration struct{}

// GetMajorVersion returns the major version this migration handles
func (m *V8Migration) GetMajorVersion() float64 {
	return 8.0
}

// HasSystemUpdate indicates if this migration has system-level changes
func (m *V8Migration) HasSystemUpdate() bool {
	return true
}

// HasWorkspaceUpdate indicates if this migration has workspace-level changes
func (m *V8Migration) HasWorkspaceUpdate() bool {
	return true
}

// UpdateSystem executes system-level migration changes
func (m *V8Migration) UpdateSystem(ctx context.Context, config *config.Config, db DBExecutor) error {
	// Get all workspaces to create queue processing tasks
	rows, err := db.QueryContext(ctx, `SELECT id FROM workspaces`)
	if err != nil {
		return fmt.Errorf("failed to query workspaces: %w", err)
	}
	defer rows.Close()

	// Collect all workspace IDs first to avoid query conflicts
	var workspaceIDs []string
	for rows.Next() {
		var workspaceID string
		if err := rows.Scan(&workspaceID); err != nil {
			return fmt.Errorf("failed to scan workspace ID: %w", err)
		}
		workspaceIDs = append(workspaceIDs, workspaceID)
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("error iterating workspaces: %w", err)
	}

	// Now process each workspace
	for _, workspaceID := range workspaceIDs {
		// Create permanent task for processing contact_segment_queue
		// This task will run on every cron tick to process queued contacts
		// Check if task already exists first
		var exists bool
		err = db.QueryRowContext(ctx, `
			SELECT EXISTS(SELECT 1 FROM tasks WHERE workspace_id = $1 AND type = 'process_contact_segment_queue')
		`, workspaceID).Scan(&exists)
		if err != nil {
			return fmt.Errorf("failed to check existing task for workspace %s: %w", workspaceID, err)
		}

		// Only create if it doesn't exist
		if !exists {
			_, err = db.ExecContext(ctx, `
				INSERT INTO tasks (id, workspace_id, type, status, next_run_after, max_runtime, max_retries, retry_interval, progress, state, created_at, updated_at)
				VALUES (
					gen_random_uuid(),
					$1,
					'process_contact_segment_queue',
					'pending',
					CURRENT_TIMESTAMP,
					50,
					3,
					60,
					0,
					'{"message": "Contact segment queue processing task"}'::jsonb,
					CURRENT_TIMESTAMP,
					CURRENT_TIMESTAMP
				)
			`, workspaceID)
			if err != nil {
				return fmt.Errorf("failed to create queue processing task for workspace %s: %w", workspaceID, err)
			}
		}
	}

	return nil
}

// UpdateWorkspace executes workspace-level migration changes
func (m *V8Migration) UpdateWorkspace(ctx context.Context, config *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	// Add kind column (operation_entityType)
	_, err := db.ExecContext(ctx, `
		ALTER TABLE contact_timeline
		ADD COLUMN IF NOT EXISTS kind VARCHAR(50) NOT NULL DEFAULT ''
	`)
	if err != nil {
		return fmt.Errorf("failed to add kind column to contact_timeline table for workspace %s: %w", workspace.ID, err)
	}

	// Add db_created_at column
	_, err = db.ExecContext(ctx, `
		ALTER TABLE contact_timeline
		ADD COLUMN IF NOT EXISTS db_created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
	`)
	if err != nil {
		return fmt.Errorf("failed to add db_created_at column to contact_timeline table for workspace %s: %w", workspace.ID, err)
	}

	// Update existing rows to populate kind based on operation and entity_type
	// Handle message_history engagement events specially
	// Note: ct.changes is JSONB, using ? operator to check if key exists
	_, err = db.ExecContext(ctx, `
		UPDATE contact_timeline ct
		SET kind = CASE
			-- Message history engagement events with channel (using JSONB ? operator to check key existence)
			WHEN ct.entity_type = 'message_history' AND ct.operation = 'update' AND ct.changes ? 'opened_at' 
				THEN 'open_' || COALESCE((SELECT channel FROM message_history WHERE id = ct.entity_id), 'email')
			WHEN ct.entity_type = 'message_history' AND ct.operation = 'update' AND ct.changes ? 'clicked_at' 
				THEN 'click_' || COALESCE((SELECT channel FROM message_history WHERE id = ct.entity_id), 'email')
			WHEN ct.entity_type = 'message_history' AND ct.operation = 'update' AND ct.changes ? 'bounced_at' 
				THEN 'bounce_' || COALESCE((SELECT channel FROM message_history WHERE id = ct.entity_id), 'email')
			WHEN ct.entity_type = 'message_history' AND ct.operation = 'update' AND ct.changes ? 'complained_at' 
				THEN 'complain_' || COALESCE((SELECT channel FROM message_history WHERE id = ct.entity_id), 'email')
			WHEN ct.entity_type = 'message_history' AND ct.operation = 'update' AND ct.changes ? 'unsubscribed_at' 
				THEN 'unsubscribe_' || COALESCE((SELECT channel FROM message_history WHERE id = ct.entity_id), 'email')
			-- Default: operation_entitytype
			ELSE ct.operation || '_' || ct.entity_type
		END
		WHERE ct.kind = '' OR ct.kind IS NULL
	`)
	if err != nil {
		return fmt.Errorf("failed to populate kind column in contact_timeline table for workspace %s: %w", workspace.ID, err)
	}

	// Update existing rows to set db_created_at from created_at for historical data
	_, err = db.ExecContext(ctx, `
		UPDATE contact_timeline
		SET db_created_at = created_at
		WHERE db_created_at IS NULL OR db_created_at = CURRENT_TIMESTAMP
	`)
	if err != nil {
		return fmt.Errorf("failed to populate db_created_at column in contact_timeline table for workspace %s: %w", workspace.ID, err)
	}

	// Remove default from created_at column (PostgreSQL requires a different approach)
	// We need to alter the column to remove the default
	_, err = db.ExecContext(ctx, `
		ALTER TABLE contact_timeline
		ALTER COLUMN created_at DROP DEFAULT
	`)
	if err != nil {
		// This is not critical - if it fails, it just means the default stays
		// Log it but don't fail the migration
		fmt.Printf("Warning: failed to remove default from created_at column in contact_timeline table for workspace %s: %v\n", workspace.ID, err)
	}

	// Add db_created_at and db_updated_at to contacts table with defaults
	_, err = db.ExecContext(ctx, `
		ALTER TABLE contacts
		ADD COLUMN IF NOT EXISTS db_created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
		ADD COLUMN IF NOT EXISTS db_updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
	`)
	if err != nil {
		return fmt.Errorf("failed to add db_created_at and db_updated_at columns to contacts table for workspace %s: %w", workspace.ID, err)
	}

	// Create index on kind column for filtering
	_, err = db.ExecContext(ctx, `
		CREATE INDEX IF NOT EXISTS idx_contact_timeline_kind ON contact_timeline(kind)
	`)
	if err != nil {
		return fmt.Errorf("failed to create kind index on contact_timeline table for workspace %s: %w", workspace.ID, err)
	}

	// Create contact_segment_queue table (before triggers that reference it)
	_, err = db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS contact_segment_queue (
			email VARCHAR(255) PRIMARY KEY,
			queued_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create contact_segment_queue table for workspace %s: %w", workspace.ID, err)
	}

	// Create index on queued_at for ordered processing
	_, err = db.ExecContext(ctx, `
		CREATE INDEX IF NOT EXISTS idx_contact_segment_queue_queued_at ON contact_segment_queue(queued_at ASC)
	`)
	if err != nil {
		return fmt.Errorf("failed to create queued_at index on contact_segment_queue table for workspace %s: %w", workspace.ID, err)
	}

	// Update trigger functions to include kind field and queue logic
	// Contact changes trigger function
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
				IF OLD.external_id IS DISTINCT FROM NEW.external_id THEN changes_json := changes_json || jsonb_build_object('external_id', jsonb_build_object('old', OLD.external_id, 'new', NEW.external_id)); END IF;
				IF OLD.timezone IS DISTINCT FROM NEW.timezone THEN changes_json := changes_json || jsonb_build_object('timezone', jsonb_build_object('old', OLD.timezone, 'new', NEW.timezone)); END IF;
				IF OLD.language IS DISTINCT FROM NEW.language THEN changes_json := changes_json || jsonb_build_object('language', jsonb_build_object('old', OLD.language, 'new', NEW.language)); END IF;
				IF OLD.first_name IS DISTINCT FROM NEW.first_name THEN changes_json := changes_json || jsonb_build_object('first_name', jsonb_build_object('old', OLD.first_name, 'new', NEW.first_name)); END IF;
				IF OLD.last_name IS DISTINCT FROM NEW.last_name THEN changes_json := changes_json || jsonb_build_object('last_name', jsonb_build_object('old', OLD.last_name, 'new', NEW.last_name)); END IF;
				IF OLD.phone IS DISTINCT FROM NEW.phone THEN changes_json := changes_json || jsonb_build_object('phone', jsonb_build_object('old', OLD.phone, 'new', NEW.phone)); END IF;
				IF OLD.address_line_1 IS DISTINCT FROM NEW.address_line_1 THEN changes_json := changes_json || jsonb_build_object('address_line_1', jsonb_build_object('old', OLD.address_line_1, 'new', NEW.address_line_1)); END IF;
				IF OLD.address_line_2 IS DISTINCT FROM NEW.address_line_2 THEN changes_json := changes_json || jsonb_build_object('address_line_2', jsonb_build_object('old', OLD.address_line_2, 'new', NEW.address_line_2)); END IF;
				IF OLD.country IS DISTINCT FROM NEW.country THEN changes_json := changes_json || jsonb_build_object('country', jsonb_build_object('old', OLD.country, 'new', NEW.country)); END IF;
				IF OLD.postcode IS DISTINCT FROM NEW.postcode THEN changes_json := changes_json || jsonb_build_object('postcode', jsonb_build_object('old', OLD.postcode, 'new', NEW.postcode)); END IF;
				IF OLD.state IS DISTINCT FROM NEW.state THEN changes_json := changes_json || jsonb_build_object('state', jsonb_build_object('old', OLD.state, 'new', NEW.state)); END IF;
				IF OLD.job_title IS DISTINCT FROM NEW.job_title THEN changes_json := changes_json || jsonb_build_object('job_title', jsonb_build_object('old', OLD.job_title, 'new', NEW.job_title)); END IF;
				IF OLD.lifetime_value IS DISTINCT FROM NEW.lifetime_value THEN changes_json := changes_json || jsonb_build_object('lifetime_value', jsonb_build_object('old', OLD.lifetime_value, 'new', NEW.lifetime_value)); END IF;
				IF OLD.orders_count IS DISTINCT FROM NEW.orders_count THEN changes_json := changes_json || jsonb_build_object('orders_count', jsonb_build_object('old', OLD.orders_count, 'new', NEW.orders_count)); END IF;
				IF OLD.last_order_at IS DISTINCT FROM NEW.last_order_at THEN changes_json := changes_json || jsonb_build_object('last_order_at', jsonb_build_object('old', OLD.last_order_at, 'new', NEW.last_order_at)); END IF;
				IF OLD.custom_string_1 IS DISTINCT FROM NEW.custom_string_1 THEN changes_json := changes_json || jsonb_build_object('custom_string_1', jsonb_build_object('old', OLD.custom_string_1, 'new', NEW.custom_string_1)); END IF;
				IF OLD.custom_string_2 IS DISTINCT FROM NEW.custom_string_2 THEN changes_json := changes_json || jsonb_build_object('custom_string_2', jsonb_build_object('old', OLD.custom_string_2, 'new', NEW.custom_string_2)); END IF;
				IF OLD.custom_string_3 IS DISTINCT FROM NEW.custom_string_3 THEN changes_json := changes_json || jsonb_build_object('custom_string_3', jsonb_build_object('old', OLD.custom_string_3, 'new', NEW.custom_string_3)); END IF;
				IF OLD.custom_string_4 IS DISTINCT FROM NEW.custom_string_4 THEN changes_json := changes_json || jsonb_build_object('custom_string_4', jsonb_build_object('old', OLD.custom_string_4, 'new', NEW.custom_string_4)); END IF;
				IF OLD.custom_string_5 IS DISTINCT FROM NEW.custom_string_5 THEN changes_json := changes_json || jsonb_build_object('custom_string_5', jsonb_build_object('old', OLD.custom_string_5, 'new', NEW.custom_string_5)); END IF;
				IF OLD.custom_number_1 IS DISTINCT FROM NEW.custom_number_1 THEN changes_json := changes_json || jsonb_build_object('custom_number_1', jsonb_build_object('old', OLD.custom_number_1, 'new', NEW.custom_number_1)); END IF;
				IF OLD.custom_number_2 IS DISTINCT FROM NEW.custom_number_2 THEN changes_json := changes_json || jsonb_build_object('custom_number_2', jsonb_build_object('old', OLD.custom_number_2, 'new', NEW.custom_number_2)); END IF;
				IF OLD.custom_number_3 IS DISTINCT FROM NEW.custom_number_3 THEN changes_json := changes_json || jsonb_build_object('custom_number_3', jsonb_build_object('old', OLD.custom_number_3, 'new', NEW.custom_number_3)); END IF;
				IF OLD.custom_number_4 IS DISTINCT FROM NEW.custom_number_4 THEN changes_json := changes_json || jsonb_build_object('custom_number_4', jsonb_build_object('old', OLD.custom_number_4, 'new', NEW.custom_number_4)); END IF;
				IF OLD.custom_number_5 IS DISTINCT FROM NEW.custom_number_5 THEN changes_json := changes_json || jsonb_build_object('custom_number_5', jsonb_build_object('old', OLD.custom_number_5, 'new', NEW.custom_number_5)); END IF;
				IF OLD.custom_datetime_1 IS DISTINCT FROM NEW.custom_datetime_1 THEN changes_json := changes_json || jsonb_build_object('custom_datetime_1', jsonb_build_object('old', OLD.custom_datetime_1, 'new', NEW.custom_datetime_1)); END IF;
				IF OLD.custom_datetime_2 IS DISTINCT FROM NEW.custom_datetime_2 THEN changes_json := changes_json || jsonb_build_object('custom_datetime_2', jsonb_build_object('old', OLD.custom_datetime_2, 'new', NEW.custom_datetime_2)); END IF;
				IF OLD.custom_datetime_3 IS DISTINCT FROM NEW.custom_datetime_3 THEN changes_json := changes_json || jsonb_build_object('custom_datetime_3', jsonb_build_object('old', OLD.custom_datetime_3, 'new', NEW.custom_datetime_3)); END IF;
				IF OLD.custom_datetime_4 IS DISTINCT FROM NEW.custom_datetime_4 THEN changes_json := changes_json || jsonb_build_object('custom_datetime_4', jsonb_build_object('old', OLD.custom_datetime_4, 'new', NEW.custom_datetime_4)); END IF;
				IF OLD.custom_datetime_5 IS DISTINCT FROM NEW.custom_datetime_5 THEN changes_json := changes_json || jsonb_build_object('custom_datetime_5', jsonb_build_object('old', OLD.custom_datetime_5, 'new', NEW.custom_datetime_5)); END IF;
				IF OLD.custom_json_1 IS DISTINCT FROM NEW.custom_json_1 THEN changes_json := changes_json || jsonb_build_object('custom_json_1', jsonb_build_object('old', OLD.custom_json_1, 'new', NEW.custom_json_1)); END IF;
				IF OLD.custom_json_2 IS DISTINCT FROM NEW.custom_json_2 THEN changes_json := changes_json || jsonb_build_object('custom_json_2', jsonb_build_object('old', OLD.custom_json_2, 'new', NEW.custom_json_2)); END IF;
				IF OLD.custom_json_3 IS DISTINCT FROM NEW.custom_json_3 THEN changes_json := changes_json || jsonb_build_object('custom_json_3', jsonb_build_object('old', OLD.custom_json_3, 'new', NEW.custom_json_3)); END IF;
				IF OLD.custom_json_4 IS DISTINCT FROM NEW.custom_json_4 THEN changes_json := changes_json || jsonb_build_object('custom_json_4', jsonb_build_object('old', OLD.custom_json_4, 'new', NEW.custom_json_4)); END IF;
				IF OLD.custom_json_5 IS DISTINCT FROM NEW.custom_json_5 THEN changes_json := changes_json || jsonb_build_object('custom_json_5', jsonb_build_object('old', OLD.custom_json_5, 'new', NEW.custom_json_5)); END IF;
				IF changes_json = '{}'::jsonb THEN RETURN NEW; END IF;
			END IF;
			IF TG_OP = 'INSERT' THEN
				INSERT INTO contact_timeline (email, operation, entity_type, kind, changes, created_at) 
				VALUES (NEW.email, op, 'contact', op || '_contact', changes_json, NEW.created_at);
				-- Queue the contact for segment recomputation
				INSERT INTO contact_segment_queue (email, queued_at)
				VALUES (NEW.email, NEW.created_at)
				ON CONFLICT (email) DO UPDATE SET queued_at = EXCLUDED.queued_at;
			ELSE
				INSERT INTO contact_timeline (email, operation, entity_type, kind, changes, created_at) 
				VALUES (NEW.email, op, 'contact', op || '_contact', changes_json, NEW.updated_at);
				-- Queue the contact for segment recomputation (only for actual changes)
				INSERT INTO contact_segment_queue (email, queued_at)
				VALUES (NEW.email, NEW.updated_at)
				ON CONFLICT (email) DO UPDATE SET queued_at = EXCLUDED.queued_at;
			END IF;
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;
	`)
	if err != nil {
		return fmt.Errorf("failed to update track_contact_changes function for workspace %s: %w", workspace.ID, err)
	}

	// Contact list changes trigger function
	_, err = db.ExecContext(ctx, `
		CREATE OR REPLACE FUNCTION track_contact_list_changes()
		RETURNS TRIGGER AS $$
		DECLARE
			changes_json JSONB := '{}'::jsonb;
			op VARCHAR(20);
		BEGIN
			IF TG_OP = 'INSERT' THEN
				op := 'insert';
				changes_json := jsonb_build_object('list_id', jsonb_build_object('new', NEW.list_id), 'status', jsonb_build_object('new', NEW.status));
			ELSIF TG_OP = 'UPDATE' THEN
				op := 'update';
				IF OLD.status IS DISTINCT FROM NEW.status THEN changes_json := changes_json || jsonb_build_object('status', jsonb_build_object('old', OLD.status, 'new', NEW.status)); END IF;
				IF OLD.deleted_at IS DISTINCT FROM NEW.deleted_at THEN changes_json := changes_json || jsonb_build_object('deleted_at', jsonb_build_object('old', OLD.deleted_at, 'new', NEW.deleted_at)); END IF;
				IF changes_json = '{}'::jsonb THEN RETURN NEW; END IF;
			END IF;
			INSERT INTO contact_timeline (email, operation, entity_type, kind, entity_id, changes, created_at) 
			VALUES (NEW.email, op, 'contact_list', op || '_contact_list', NEW.list_id, changes_json, CURRENT_TIMESTAMP);
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;
	`)
	if err != nil {
		return fmt.Errorf("failed to update track_contact_list_changes function for workspace %s: %w", workspace.ID, err)
	}

	// Message history changes trigger function
	_, err = db.ExecContext(ctx, `
		CREATE OR REPLACE FUNCTION track_message_history_changes()
		RETURNS TRIGGER AS $$
		DECLARE
			changes_json JSONB := '{}'::jsonb;
			op VARCHAR(20);
			kind_value VARCHAR(50);
		BEGIN
			IF TG_OP = 'INSERT' THEN
				op := 'insert';
				changes_json := jsonb_build_object('template_id', jsonb_build_object('new', NEW.template_id), 'template_version', jsonb_build_object('new', NEW.template_version), 'channel', jsonb_build_object('new', NEW.channel), 'broadcast_id', jsonb_build_object('new', NEW.broadcast_id), 'sent_at', jsonb_build_object('new', NEW.sent_at));
				INSERT INTO contact_timeline (email, operation, entity_type, kind, entity_id, changes, created_at) 
				VALUES (NEW.contact_email, op, 'message_history', 'insert_message_history', NEW.id, changes_json, NEW.updated_at);
			ELSIF TG_OP = 'UPDATE' THEN
				op := 'update';
				-- Handle engagement events separately with specific kinds
				IF OLD.opened_at IS DISTINCT FROM NEW.opened_at AND NEW.opened_at IS NOT NULL THEN
					changes_json := jsonb_build_object('opened_at', jsonb_build_object('old', OLD.opened_at, 'new', NEW.opened_at));
					INSERT INTO contact_timeline (email, operation, entity_type, kind, entity_id, changes, created_at) 
					VALUES (NEW.contact_email, op, 'message_history', 'open_' || NEW.channel, NEW.id, changes_json, NEW.updated_at);
				END IF;
				IF OLD.clicked_at IS DISTINCT FROM NEW.clicked_at AND NEW.clicked_at IS NOT NULL THEN
					changes_json := jsonb_build_object('clicked_at', jsonb_build_object('old', OLD.clicked_at, 'new', NEW.clicked_at));
					INSERT INTO contact_timeline (email, operation, entity_type, kind, entity_id, changes, created_at) 
					VALUES (NEW.contact_email, op, 'message_history', 'click_' || NEW.channel, NEW.id, changes_json, NEW.updated_at);
				END IF;
				IF OLD.bounced_at IS DISTINCT FROM NEW.bounced_at AND NEW.bounced_at IS NOT NULL THEN
					changes_json := jsonb_build_object('bounced_at', jsonb_build_object('old', OLD.bounced_at, 'new', NEW.bounced_at));
					INSERT INTO contact_timeline (email, operation, entity_type, kind, entity_id, changes, created_at) 
					VALUES (NEW.contact_email, op, 'message_history', 'bounce_' || NEW.channel, NEW.id, changes_json, NEW.updated_at);
				END IF;
				IF OLD.complained_at IS DISTINCT FROM NEW.complained_at AND NEW.complained_at IS NOT NULL THEN
					changes_json := jsonb_build_object('complained_at', jsonb_build_object('old', OLD.complained_at, 'new', NEW.complained_at));
					INSERT INTO contact_timeline (email, operation, entity_type, kind, entity_id, changes, created_at) 
					VALUES (NEW.contact_email, op, 'message_history', 'complain_' || NEW.channel, NEW.id, changes_json, NEW.updated_at);
				END IF;
				IF OLD.unsubscribed_at IS DISTINCT FROM NEW.unsubscribed_at AND NEW.unsubscribed_at IS NOT NULL THEN
					changes_json := jsonb_build_object('unsubscribed_at', jsonb_build_object('old', OLD.unsubscribed_at, 'new', NEW.unsubscribed_at));
					INSERT INTO contact_timeline (email, operation, entity_type, kind, entity_id, changes, created_at) 
					VALUES (NEW.contact_email, op, 'message_history', 'unsubscribe_' || NEW.channel, NEW.id, changes_json, NEW.updated_at);
				END IF;
				-- Handle other updates (delivered, failed, status_info) as generic updates
				changes_json := '{}'::jsonb;
				IF OLD.delivered_at IS DISTINCT FROM NEW.delivered_at THEN changes_json := changes_json || jsonb_build_object('delivered_at', jsonb_build_object('old', OLD.delivered_at, 'new', NEW.delivered_at)); END IF;
				IF OLD.failed_at IS DISTINCT FROM NEW.failed_at THEN changes_json := changes_json || jsonb_build_object('failed_at', jsonb_build_object('old', OLD.failed_at, 'new', NEW.failed_at)); END IF;
				IF OLD.status_info IS DISTINCT FROM NEW.status_info THEN changes_json := changes_json || jsonb_build_object('status_info', jsonb_build_object('old', OLD.status_info, 'new', NEW.status_info)); END IF;
				IF changes_json != '{}'::jsonb THEN
					INSERT INTO contact_timeline (email, operation, entity_type, kind, entity_id, changes, created_at) 
					VALUES (NEW.contact_email, op, 'message_history', 'update_message_history', NEW.id, changes_json, NEW.updated_at);
				END IF;
			END IF;
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;
	`)
	if err != nil {
		return fmt.Errorf("failed to update track_message_history_changes function for workspace %s: %w", workspace.ID, err)
	}

	// Webhook event changes trigger function
	_, err = db.ExecContext(ctx, `
		CREATE OR REPLACE FUNCTION track_webhook_event_changes()
		RETURNS TRIGGER AS $$
		DECLARE
			changes_json JSONB := '{}'::jsonb;
		BEGIN
			changes_json := jsonb_build_object('type', jsonb_build_object('new', NEW.type), 'email_provider_kind', jsonb_build_object('new', NEW.email_provider_kind));
			IF NEW.bounce_type IS NOT NULL AND NEW.bounce_type != '' THEN changes_json := changes_json || jsonb_build_object('bounce_type', jsonb_build_object('new', NEW.bounce_type)); END IF;
			IF NEW.bounce_category IS NOT NULL AND NEW.bounce_category != '' THEN changes_json := changes_json || jsonb_build_object('bounce_category', jsonb_build_object('new', NEW.bounce_category)); END IF;
			IF NEW.bounce_diagnostic IS NOT NULL AND NEW.bounce_diagnostic != '' THEN changes_json := changes_json || jsonb_build_object('bounce_diagnostic', jsonb_build_object('new', NEW.bounce_diagnostic)); END IF;
			IF NEW.complaint_feedback_type IS NOT NULL AND NEW.complaint_feedback_type != '' THEN changes_json := changes_json || jsonb_build_object('complaint_feedback_type', jsonb_build_object('new', NEW.complaint_feedback_type)); END IF;
			INSERT INTO contact_timeline (email, operation, entity_type, kind, entity_id, changes, created_at) 
			VALUES (NEW.recipient_email, 'insert', 'webhook_event', 'insert_webhook_event', NEW.message_id, changes_json, CURRENT_TIMESTAMP);
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;
	`)
	if err != nil {
		return fmt.Errorf("failed to update track_webhook_event_changes function for workspace %s: %w", workspace.ID, err)
	}

	// Create segments table
	_, err = db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS segments (
			id VARCHAR(32) PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			color VARCHAR(50) NOT NULL,
			tree JSONB NOT NULL,
			timezone VARCHAR(100) NOT NULL,
			version INTEGER NOT NULL,
			status VARCHAR(20) NOT NULL,
			generated_sql TEXT,
			generated_args JSONB,
			db_created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
			db_updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create segments table for workspace %s: %w", workspace.ID, err)
	}

	// Create indexes for segments table
	_, err = db.ExecContext(ctx, `
		CREATE INDEX IF NOT EXISTS idx_segments_status ON segments(status)
	`)
	if err != nil {
		return fmt.Errorf("failed to create status index on segments table for workspace %s: %w", workspace.ID, err)
	}

	// Create contact_segments table
	_, err = db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS contact_segments (
			email VARCHAR(255) NOT NULL,
			segment_id VARCHAR(32) NOT NULL,
			version INTEGER NOT NULL,
			matched_at TIMESTAMP WITH TIME ZONE NOT NULL,
			computed_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (email, segment_id)
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create contact_segments table for workspace %s: %w", workspace.ID, err)
	}

	// Create indexes for contact_segments table
	_, err = db.ExecContext(ctx, `
		CREATE INDEX IF NOT EXISTS idx_contact_segments_segment_id ON contact_segments(segment_id)
	`)
	if err != nil {
		return fmt.Errorf("failed to create segment_id index on contact_segments table for workspace %s: %w", workspace.ID, err)
	}

	_, err = db.ExecContext(ctx, `
		CREATE INDEX IF NOT EXISTS idx_contact_segments_version ON contact_segments(segment_id, version)
	`)
	if err != nil {
		return fmt.Errorf("failed to create version index on contact_segments table for workspace %s: %w", workspace.ID, err)
	}

	// Create trigger function for contact_segments timeline tracking
	_, err = db.ExecContext(ctx, `
		CREATE OR REPLACE FUNCTION track_contact_segment_changes()
		RETURNS TRIGGER AS $$
		DECLARE
			changes_json JSONB := '{}'::jsonb;
			op VARCHAR(20);
			kind_value VARCHAR(50);
		BEGIN
			IF TG_OP = 'INSERT' THEN
				op := 'insert';
				kind_value := 'join_segment';
				changes_json := jsonb_build_object('segment_id', jsonb_build_object('new', NEW.segment_id), 'version', jsonb_build_object('new', NEW.version), 'matched_at', jsonb_build_object('new', NEW.matched_at));
			ELSIF TG_OP = 'DELETE' THEN
				op := 'delete';
				kind_value := 'leave_segment';
				changes_json := jsonb_build_object('segment_id', jsonb_build_object('old', OLD.segment_id), 'version', jsonb_build_object('old', OLD.version));
				INSERT INTO contact_timeline (email, operation, entity_type, kind, entity_id, changes, created_at) 
				VALUES (OLD.email, op, 'contact_segment', kind_value, OLD.segment_id, changes_json, CURRENT_TIMESTAMP);
				RETURN OLD;
			END IF;
			INSERT INTO contact_timeline (email, operation, entity_type, kind, entity_id, changes, created_at) 
			VALUES (NEW.email, op, 'contact_segment', kind_value, NEW.segment_id, changes_json, CURRENT_TIMESTAMP);
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;
	`)
	if err != nil {
		return fmt.Errorf("failed to create track_contact_segment_changes function for workspace %s: %w", workspace.ID, err)
	}

	// Create trigger for contact_segments
	_, err = db.ExecContext(ctx, `
		DROP TRIGGER IF EXISTS contact_segment_changes_trigger ON contact_segments
	`)
	if err != nil {
		return fmt.Errorf("failed to drop contact_segment_changes_trigger for workspace %s: %w", workspace.ID, err)
	}

	_, err = db.ExecContext(ctx, `
		CREATE TRIGGER contact_segment_changes_trigger 
		AFTER INSERT OR DELETE ON contact_segments 
		FOR EACH ROW EXECUTE FUNCTION track_contact_segment_changes()
	`)
	if err != nil {
		return fmt.Errorf("failed to create contact_segment_changes_trigger for workspace %s: %w", workspace.ID, err)
	}

	// Note: Task creation has been moved to UpdateSystem since tasks table is in the system database

	return nil
}

// init registers this migration with the default registry
func init() {
	Register(&V8Migration{})
}
