package migrations

import (
	"context"
	"fmt"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
)

// V18Migration adds custom events system and semantic naming for all internal timeline events
// This migration includes:
// 1. Custom events table and timeline trigger (generic, no hardcoded logic)
// 2. Contact list semantic naming (list.subscribed, list.unsubscribed, etc.)
// 3. Segment semantic naming (segment.joined, segment.left)
// 4. Contact semantic naming (contact.created, contact.updated)
// 5. Historical data migration for all three entity types
// 6. Remove deprecated contact fields (lifetime_value, orders_count, last_order_at) - now handled by custom_events_goals
type V18Migration struct{}

func (m *V18Migration) GetMajorVersion() float64 {
	return 18.0
}

func (m *V18Migration) HasSystemUpdate() bool {
	return false
}

func (m *V18Migration) HasWorkspaceUpdate() bool {
	return true
}

func (m *V18Migration) ShouldRestartServer() bool {
	return false
}

func (m *V18Migration) UpdateSystem(ctx context.Context, config *config.Config, db DBExecutor) error {
	// No system updates needed
	return nil
}

func (m *V18Migration) UpdateWorkspace(ctx context.Context, config *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	// ========================================================================
	// PART 1: Custom Events System
	// ========================================================================

	// Create custom_events table with composite PRIMARY KEY (event_name, external_id)
	// Includes goal tracking fields and soft-delete support
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS custom_events (
			event_name VARCHAR(100) NOT NULL,
			external_id VARCHAR(255) NOT NULL,
			email VARCHAR(255) NOT NULL,
			properties JSONB NOT NULL DEFAULT '{}'::jsonb,
			occurred_at TIMESTAMPTZ NOT NULL,
			source VARCHAR(50) NOT NULL DEFAULT 'api',
			integration_id VARCHAR(32),
			-- Goal tracking fields
			goal_name VARCHAR(100) DEFAULT NULL,
			goal_type VARCHAR(20) DEFAULT NULL,
			goal_value DECIMAL(15,2) DEFAULT NULL,
			-- Soft delete
			deleted_at TIMESTAMPTZ DEFAULT NULL,
			-- Timestamps
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW(),
			PRIMARY KEY (event_name, external_id)
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create custom_events table: %w", err)
	}

	// Ensure all columns exist (for idempotent migrations where table might exist without these columns)
	_, err = db.ExecContext(ctx, `
		ALTER TABLE custom_events
		ADD COLUMN IF NOT EXISTS goal_name VARCHAR(100) DEFAULT NULL,
		ADD COLUMN IF NOT EXISTS goal_type VARCHAR(20) DEFAULT NULL,
		ADD COLUMN IF NOT EXISTS goal_value DECIMAL(15,2) DEFAULT NULL,
		ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ DEFAULT NULL,
		ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ DEFAULT NOW(),
		ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ DEFAULT NOW()
	`)
	if err != nil {
		return fmt.Errorf("failed to add missing columns to custom_events: %w", err)
	}

	// Create indexes for custom_events table (exclude deleted rows where applicable)
	_, err = db.ExecContext(ctx, `
		CREATE INDEX IF NOT EXISTS idx_custom_events_email
			ON custom_events(email, occurred_at DESC)
			WHERE deleted_at IS NULL;

		CREATE INDEX IF NOT EXISTS idx_custom_events_external_id
			ON custom_events(external_id);

		CREATE INDEX IF NOT EXISTS idx_custom_events_integration_id
			ON custom_events(integration_id)
			WHERE integration_id IS NOT NULL;

		CREATE INDEX IF NOT EXISTS idx_custom_events_properties
			ON custom_events USING GIN (properties jsonb_path_ops);

		-- Goal tracking indexes (exclude deleted rows)
		CREATE INDEX IF NOT EXISTS idx_custom_events_goal_type
			ON custom_events(email, goal_type, occurred_at DESC)
			WHERE goal_type IS NOT NULL AND deleted_at IS NULL;

		CREATE INDEX IF NOT EXISTS idx_custom_events_purchases
			ON custom_events(email, goal_value, occurred_at)
			WHERE goal_type = 'purchase' AND deleted_at IS NULL;

		-- Index for soft-deleted records (for cleanup queries)
		CREATE INDEX IF NOT EXISTS idx_custom_events_deleted
			ON custom_events(deleted_at)
			WHERE deleted_at IS NOT NULL;
	`)
	if err != nil {
		return fmt.Errorf("failed to create custom_events indexes: %w", err)
	}

	// Create trigger function for custom events (generic, no hardcoded logic)
	_, err = db.ExecContext(ctx, `
		CREATE OR REPLACE FUNCTION track_custom_event_timeline()
		RETURNS TRIGGER AS $$
		DECLARE
			timeline_operation TEXT;
			changes_json JSONB;
			property_key TEXT;
			property_diff JSONB;
		BEGIN
			IF TG_OP = 'INSERT' THEN
				-- On INSERT: Create timeline entry with operation='insert'
				timeline_operation := 'insert';

				changes_json := jsonb_build_object(
					'event_name', jsonb_build_object('new', NEW.event_name),
					'external_id', jsonb_build_object('new', NEW.external_id)
				);

				-- Add goal fields if present
				IF NEW.goal_type IS NOT NULL THEN
					changes_json := changes_json || jsonb_build_object('goal_type', jsonb_build_object('new', NEW.goal_type));
				END IF;
				IF NEW.goal_value IS NOT NULL THEN
					changes_json := changes_json || jsonb_build_object('goal_value', jsonb_build_object('new', NEW.goal_value));
				END IF;
				IF NEW.goal_name IS NOT NULL THEN
					changes_json := changes_json || jsonb_build_object('goal_name', jsonb_build_object('new', NEW.goal_name));
				END IF;

			ELSIF TG_OP = 'UPDATE' THEN
				-- On UPDATE: Create timeline entry with operation='update'
				timeline_operation := 'update';

				-- Compute JSON diff between OLD.properties and NEW.properties
				property_diff := '{}'::jsonb;

				-- Find changed, added, or removed keys
				FOR property_key IN
					SELECT DISTINCT key
					FROM (
						SELECT key FROM jsonb_object_keys(OLD.properties) AS key
						UNION
						SELECT key FROM jsonb_object_keys(NEW.properties) AS key
					) AS all_keys
				LOOP
					-- Compare old and new values for this key
					IF (OLD.properties->property_key) IS DISTINCT FROM (NEW.properties->property_key) THEN
						property_diff := property_diff || jsonb_build_object(
							property_key,
							jsonb_build_object(
								'old', OLD.properties->property_key,
								'new', NEW.properties->property_key
							)
						);
					END IF;
				END LOOP;

				changes_json := jsonb_build_object(
					'properties', property_diff,
					'occurred_at', jsonb_build_object(
						'old', OLD.occurred_at,
						'new', NEW.occurred_at
					)
				);

				-- Add goal fields if changed
				IF OLD.goal_type IS DISTINCT FROM NEW.goal_type THEN
					changes_json := changes_json || jsonb_build_object('goal_type', jsonb_build_object('old', OLD.goal_type, 'new', NEW.goal_type));
				END IF;
				IF OLD.goal_value IS DISTINCT FROM NEW.goal_value THEN
					changes_json := changes_json || jsonb_build_object('goal_value', jsonb_build_object('old', OLD.goal_value, 'new', NEW.goal_value));
				END IF;
				IF OLD.goal_name IS DISTINCT FROM NEW.goal_name THEN
					changes_json := changes_json || jsonb_build_object('goal_name', jsonb_build_object('old', OLD.goal_name, 'new', NEW.goal_name));
				END IF;
			END IF;

			-- Insert timeline entry with exact event_name as kind
			INSERT INTO contact_timeline (
				email, operation, entity_type, kind, entity_id, changes, created_at
			) VALUES (
				NEW.email, timeline_operation, 'custom_event', NEW.event_name,
				NEW.external_id, changes_json, NEW.occurred_at
			);

			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;
	`)
	if err != nil {
		return fmt.Errorf("failed to create track_custom_event_timeline function: %w", err)
	}

	// Create trigger for INSERT and UPDATE on custom_events
	_, err = db.ExecContext(ctx, `
		DROP TRIGGER IF EXISTS custom_event_timeline_trigger ON custom_events;

		CREATE TRIGGER custom_event_timeline_trigger
		AFTER INSERT OR UPDATE ON custom_events
		FOR EACH ROW EXECUTE FUNCTION track_custom_event_timeline();
	`)
	if err != nil {
		return fmt.Errorf("failed to create custom_event_timeline_trigger: %w", err)
	}

	// ========================================================================
	// PART 2: Contact List Semantic Naming
	// ========================================================================

	// Update contact_list trigger to use semantic event names (list.subscribed, list.unsubscribed, etc.)
	_, err = db.ExecContext(ctx, `
		CREATE OR REPLACE FUNCTION track_contact_list_changes()
		RETURNS TRIGGER AS $$
		DECLARE
			changes_json JSONB := '{}'::jsonb;
			op VARCHAR(20);
			kind_value VARCHAR(50);
		BEGIN
			IF TG_OP = 'INSERT' THEN
				op := 'insert';

				-- Map initial status to semantic event kind (dotted format)
				kind_value := CASE NEW.status
					WHEN 'active' THEN 'list.subscribed'
					WHEN 'pending' THEN 'list.pending'
					WHEN 'unsubscribed' THEN 'list.unsubscribed'
					WHEN 'bounced' THEN 'list.bounced'
					WHEN 'complained' THEN 'list.complained'
					ELSE 'list.subscribed'
				END;

				changes_json := jsonb_build_object(
					'list_id', jsonb_build_object('new', NEW.list_id),
					'status', jsonb_build_object('new', NEW.status)
				);

			ELSIF TG_OP = 'UPDATE' THEN
				op := 'update';

				-- Handle soft delete
				IF OLD.deleted_at IS DISTINCT FROM NEW.deleted_at AND NEW.deleted_at IS NOT NULL THEN
					kind_value := 'list.removed';
					changes_json := jsonb_build_object(
						'deleted_at', jsonb_build_object('old', OLD.deleted_at, 'new', NEW.deleted_at)
					);

				-- Handle status transitions
				ELSIF OLD.status IS DISTINCT FROM NEW.status THEN
					kind_value := CASE
						WHEN OLD.status = 'pending' AND NEW.status = 'active' THEN 'list.confirmed'
						WHEN OLD.status IN ('unsubscribed', 'bounced', 'complained') AND NEW.status = 'active' THEN 'list.resubscribed'
						WHEN NEW.status = 'unsubscribed' THEN 'list.unsubscribed'
						WHEN NEW.status = 'bounced' THEN 'list.bounced'
						WHEN NEW.status = 'complained' THEN 'list.complained'
						WHEN NEW.status = 'pending' THEN 'list.pending'
						WHEN NEW.status = 'active' THEN 'list.subscribed'
						ELSE 'list.status_changed'
					END;

					changes_json := jsonb_build_object(
						'status', jsonb_build_object('old', OLD.status, 'new', NEW.status)
					);
				ELSE
					RETURN NEW;
				END IF;
			END IF;

			INSERT INTO contact_timeline (email, operation, entity_type, kind, entity_id, changes, created_at)
			VALUES (NEW.email, op, 'contact_list', kind_value, NEW.list_id, changes_json, CURRENT_TIMESTAMP);

			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;
	`)
	if err != nil {
		return fmt.Errorf("failed to update track_contact_list_changes function: %w", err)
	}

	// Migrate existing contact_list timeline entries to use semantic event names
	_, err = db.ExecContext(ctx, `
		UPDATE contact_timeline
		SET kind = CASE
			-- INSERT events mapped by status
			WHEN kind = 'insert_contact_list' AND changes->'status'->'new' IS NOT NULL THEN
				CASE changes->'status'->>'new'
					WHEN 'active' THEN 'list.subscribed'
					WHEN 'pending' THEN 'list.pending'
					WHEN 'unsubscribed' THEN 'list.unsubscribed'
					WHEN 'bounced' THEN 'list.bounced'
					WHEN 'complained' THEN 'list.complained'
					ELSE 'list.subscribed'
				END

			-- UPDATE events mapped by status transition
			WHEN kind = 'update_contact_list' AND changes->'status' IS NOT NULL THEN
				CASE
					-- Confirmed double opt-in
					WHEN changes->'status'->>'old' = 'pending' AND changes->'status'->>'new' = 'active'
						THEN 'list.confirmed'

					-- Resubscription from unsubscribed/bounced/complained
					WHEN changes->'status'->>'old' IN ('unsubscribed', 'bounced', 'complained')
						AND changes->'status'->>'new' = 'active'
						THEN 'list.resubscribed'

					-- Unsubscribe action
					WHEN changes->'status'->>'new' = 'unsubscribed' THEN 'list.unsubscribed'

					-- Bounce event
					WHEN changes->'status'->>'new' = 'bounced' THEN 'list.bounced'

					-- Complaint event
					WHEN changes->'status'->>'new' = 'complained' THEN 'list.complained'

					-- Moved to pending
					WHEN changes->'status'->>'new' = 'pending' THEN 'list.pending'

					-- Default fallback for any other transition to active
					WHEN changes->'status'->>'new' = 'active' THEN 'list.subscribed'

					-- Catch-all
					ELSE 'list.status_changed'
				END

			-- Soft delete
			WHEN kind = 'update_contact_list' AND changes->'deleted_at'->'new' IS NOT NULL
				THEN 'list.removed'

			ELSE kind
		END
		WHERE entity_type = 'contact_list'
		  AND kind IN ('insert_contact_list', 'update_contact_list')
	`)
	if err != nil {
		return fmt.Errorf("failed to migrate contact_list timeline entries: %w", err)
	}

	// ========================================================================
	// PART 3: Segment Semantic Naming
	// ========================================================================

	// Update segment trigger with semantic naming (segment.joined, segment.left)
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
				kind_value := 'segment.joined';
				changes_json := jsonb_build_object(
					'segment_id', jsonb_build_object('new', NEW.segment_id),
					'version', jsonb_build_object('new', NEW.version),
					'matched_at', jsonb_build_object('new', NEW.matched_at)
				);
			ELSIF TG_OP = 'DELETE' THEN
				op := 'delete';
				kind_value := 'segment.left';
				changes_json := jsonb_build_object(
					'segment_id', jsonb_build_object('old', OLD.segment_id),
					'version', jsonb_build_object('old', OLD.version)
				);
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
		return fmt.Errorf("failed to update track_contact_segment_changes function: %w", err)
	}

	// Migrate historical segment timeline entries
	_, err = db.ExecContext(ctx, `
		UPDATE contact_timeline
		SET kind = CASE
			WHEN kind = 'join_segment' THEN 'segment.joined'
			WHEN kind = 'leave_segment' THEN 'segment.left'
			ELSE kind
		END
		WHERE entity_type = 'contact_segment'
		  AND kind IN ('join_segment', 'leave_segment')
	`)
	if err != nil {
		return fmt.Errorf("failed to migrate segment timeline entries: %w", err)
	}

	// ========================================================================
	// PART 4: Contact Semantic Naming
	// ========================================================================

	// Update contact trigger with semantic naming (contact.created, contact.updated)
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
				VALUES (NEW.email, op, 'contact', 'contact.created', changes_json, NEW.created_at);
			ELSE
				INSERT INTO contact_timeline (email, operation, entity_type, kind, changes, created_at)
				VALUES (NEW.email, op, 'contact', 'contact.updated', changes_json, NEW.updated_at);
			END IF;
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;
	`)
	if err != nil {
		return fmt.Errorf("failed to update track_contact_changes function: %w", err)
	}

	// Migrate historical contact timeline entries
	_, err = db.ExecContext(ctx, `
		UPDATE contact_timeline
		SET kind = CASE
			WHEN kind = 'insert_contact' THEN 'contact.created'
			WHEN kind = 'update_contact' THEN 'contact.updated'
			ELSE kind
		END
		WHERE entity_type = 'contact'
		  AND kind IN ('insert_contact', 'update_contact')
	`)
	if err != nil {
		return fmt.Errorf("failed to migrate contact timeline entries: %w", err)
	}

	// ========================================================================
	// PART 5: Remove deprecated contact fields
	// These fields are now replaced by custom_events_goals segmentation
	// ========================================================================

	// Delete contact_segments for segments that use deprecated contact fields
	// Must be done before deleting the segments due to foreign key constraints
	_, err = db.ExecContext(ctx, `
		DELETE FROM contact_segments
		WHERE segment_id IN (
			SELECT id FROM segments
			WHERE tree::text LIKE '%lifetime_value%'
			   OR tree::text LIKE '%orders_count%'
			   OR tree::text LIKE '%last_order_at%'
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to delete contact_segments for deprecated segments: %w", err)
	}

	// Delete segments that use deprecated contact fields (lifetime_value, orders_count, last_order_at)
	// These segments will no longer work after the columns are dropped
	_, err = db.ExecContext(ctx, `
		DELETE FROM segments
		WHERE tree::text LIKE '%lifetime_value%'
		   OR tree::text LIKE '%orders_count%'
		   OR tree::text LIKE '%last_order_at%'
	`)
	if err != nil {
		return fmt.Errorf("failed to delete segments using deprecated fields: %w", err)
	}

	// Drop deprecated columns from contacts table
	_, err = db.ExecContext(ctx, `
		ALTER TABLE contacts
		DROP COLUMN IF EXISTS lifetime_value,
		DROP COLUMN IF EXISTS orders_count,
		DROP COLUMN IF EXISTS last_order_at
	`)
	if err != nil {
		return fmt.Errorf("failed to drop deprecated contact columns: %w", err)
	}

	// ========================================================================
	// PART 6: Rename "table" to "source" in segment tree JSONB
	// The field was renamed for semantic clarity (custom_events_goals is not a real table)
	// ========================================================================

	_, err = db.ExecContext(ctx, `
		UPDATE segments
		SET tree = REPLACE(tree::text, '"table":', '"source":')::jsonb
		WHERE tree IS NOT NULL
	`)
	if err != nil {
		return fmt.Errorf("failed to rename table to source in segment trees: %w", err)
	}

	return nil
}

func init() {
	Register(&V18Migration{})
}
