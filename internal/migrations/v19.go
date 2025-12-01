package migrations

import (
	"context"
	"fmt"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
)

// V19Migration adds webhook subscription system for outgoing webhooks
// This migration includes:
// 1. webhook_subscriptions table for storing webhook endpoint configurations
// 2. webhook_deliveries table for queuing and tracking webhook deliveries
// 3. 5 trigger functions for capturing events from: contacts, contact_lists, contact_segments, message_history, custom_events
// 4. full_name contact field + fix timeline timestamps to use CURRENT_TIMESTAMP
type V19Migration struct{}

func (m *V19Migration) GetMajorVersion() float64 {
	return 19.0
}

func (m *V19Migration) HasSystemUpdate() bool {
	return false
}

func (m *V19Migration) HasWorkspaceUpdate() bool {
	return true
}

func (m *V19Migration) ShouldRestartServer() bool {
	return false
}

func (m *V19Migration) UpdateSystem(ctx context.Context, config *config.Config, db DBExecutor) error {
	// No system updates needed
	return nil
}

func (m *V19Migration) UpdateWorkspace(ctx context.Context, config *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	// ========================================================================
	// PART 0: Rename webhook_events to inbound_webhook_events
	// ========================================================================

	// Rename the table if it exists with old name
	_, err := db.ExecContext(ctx, `
		DO $$
		BEGIN
			-- Check if old table exists and new table does not
			IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'webhook_events')
			   AND NOT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'inbound_webhook_events') THEN
				-- Rename the table
				ALTER TABLE webhook_events RENAME TO inbound_webhook_events;

				-- Rename indexes
				ALTER INDEX IF EXISTS webhook_events_message_id_idx RENAME TO inbound_webhook_events_message_id_idx;
				ALTER INDEX IF EXISTS webhook_events_type_idx RENAME TO inbound_webhook_events_type_idx;
				ALTER INDEX IF EXISTS webhook_events_timestamp_idx RENAME TO inbound_webhook_events_timestamp_idx;
				ALTER INDEX IF EXISTS webhook_events_recipient_email_idx RENAME TO inbound_webhook_events_recipient_email_idx;
			END IF;
		END $$
	`)
	if err != nil {
		return fmt.Errorf("failed to rename webhook_events to inbound_webhook_events: %w", err)
	}

	// Update trigger function name and entity types
	_, err = db.ExecContext(ctx, `
		DO $$
		BEGIN
			-- Drop old trigger if exists
			DROP TRIGGER IF EXISTS webhook_event_changes_trigger ON inbound_webhook_events;

			-- Drop old function if exists
			DROP FUNCTION IF EXISTS track_webhook_event_changes();
		END $$
	`)
	if err != nil {
		return fmt.Errorf("failed to cleanup old webhook_event trigger: %w", err)
	}

	// Create the new inbound webhook event trigger function
	_, err = db.ExecContext(ctx, `
		CREATE OR REPLACE FUNCTION track_inbound_webhook_event_changes()
		RETURNS TRIGGER AS $$
		DECLARE
			changes_json JSONB := '{}'::jsonb;
			entity_id_value VARCHAR(255);
		BEGIN
			-- Use message_id if available, otherwise use inbound webhook event id
			entity_id_value := COALESCE(NEW.message_id, NEW.id::text);

			changes_json := jsonb_build_object('type', jsonb_build_object('new', NEW.type), 'source', jsonb_build_object('new', NEW.source));
			IF NEW.bounce_type IS NOT NULL AND NEW.bounce_type != '' THEN changes_json := changes_json || jsonb_build_object('bounce_type', jsonb_build_object('new', NEW.bounce_type)); END IF;
			IF NEW.bounce_category IS NOT NULL AND NEW.bounce_category != '' THEN changes_json := changes_json || jsonb_build_object('bounce_category', jsonb_build_object('new', NEW.bounce_category)); END IF;
			IF NEW.bounce_diagnostic IS NOT NULL AND NEW.bounce_diagnostic != '' THEN changes_json := changes_json || jsonb_build_object('bounce_diagnostic', jsonb_build_object('new', NEW.bounce_diagnostic)); END IF;
			IF NEW.complaint_feedback_type IS NOT NULL AND NEW.complaint_feedback_type != '' THEN changes_json := changes_json || jsonb_build_object('complaint_feedback_type', jsonb_build_object('new', NEW.complaint_feedback_type)); END IF;
			INSERT INTO contact_timeline (email, operation, entity_type, kind, entity_id, changes, created_at)
			VALUES (NEW.recipient_email, 'insert', 'inbound_webhook_event', 'insert_inbound_webhook_event', entity_id_value, changes_json, CURRENT_TIMESTAMP);
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql
	`)
	if err != nil {
		return fmt.Errorf("failed to create track_inbound_webhook_event_changes function: %w", err)
	}

	// Create the new trigger on inbound_webhook_events table
	_, err = db.ExecContext(ctx, `
		DROP TRIGGER IF EXISTS inbound_webhook_event_changes_trigger ON inbound_webhook_events;
		CREATE TRIGGER inbound_webhook_event_changes_trigger AFTER INSERT ON inbound_webhook_events
		FOR EACH ROW EXECUTE FUNCTION track_inbound_webhook_event_changes()
	`)
	if err != nil {
		return fmt.Errorf("failed to create inbound_webhook_event_changes_trigger: %w", err)
	}

	// Update existing contact_timeline entries to use new entity_type name
	_, err = db.ExecContext(ctx, `
		UPDATE contact_timeline
		SET entity_type = 'inbound_webhook_event'
		WHERE entity_type = 'webhook_event'
	`)
	if err != nil {
		return fmt.Errorf("failed to update contact_timeline entity_type: %w", err)
	}

	// Update any segment trees that reference the old entity type
	// Segments store their tree structure in JSONB, so we need to update the JSON
	_, err = db.ExecContext(ctx, `
		UPDATE segments
		SET tree = REPLACE(tree::text, '"entity_type":"webhook_event"', '"entity_type":"inbound_webhook_event"')::jsonb
		WHERE tree::text LIKE '%"entity_type":"webhook_event"%'
	`)
	if err != nil {
		return fmt.Errorf("failed to update segment trees with new entity_type: %w", err)
	}

	// ========================================================================
	// PART 1: Create webhook_subscriptions table
	// ========================================================================

	_, err = db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS webhook_subscriptions (
			id VARCHAR(32) PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			url TEXT NOT NULL,
			secret VARCHAR(64) NOT NULL,
			settings JSONB NOT NULL DEFAULT '{}'::jsonb,
			enabled BOOLEAN DEFAULT true,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW(),
			last_delivery_at TIMESTAMPTZ
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create webhook_subscriptions table: %w", err)
	}

	_, err = db.ExecContext(ctx, `
		CREATE INDEX IF NOT EXISTS idx_webhook_subscriptions_enabled
		ON webhook_subscriptions(enabled) WHERE enabled = true
	`)
	if err != nil {
		return fmt.Errorf("failed to create webhook_subscriptions index: %w", err)
	}

	// ========================================================================
	// PART 2: Create webhook_deliveries table
	// ========================================================================

	_, err = db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS webhook_deliveries (
			id VARCHAR(36) PRIMARY KEY,
			subscription_id VARCHAR(32) NOT NULL,
			event_type VARCHAR(100) NOT NULL,
			payload JSONB NOT NULL,
			status VARCHAR(20) DEFAULT 'pending',
			attempts INT DEFAULT 0,
			max_attempts INT DEFAULT 10,
			next_attempt_at TIMESTAMPTZ DEFAULT NOW(),
			last_attempt_at TIMESTAMPTZ,
			delivered_at TIMESTAMPTZ,
			last_response_status INT,
			last_response_body TEXT,
			last_error TEXT,
			created_at TIMESTAMPTZ DEFAULT NOW()
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create webhook_deliveries table: %w", err)
	}

	_, err = db.ExecContext(ctx, `
		CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_pending
		ON webhook_deliveries(next_attempt_at)
		WHERE status IN ('pending', 'failed') AND attempts < max_attempts
	`)
	if err != nil {
		return fmt.Errorf("failed to create webhook_deliveries pending index: %w", err)
	}

	_, err = db.ExecContext(ctx, `
		CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_subscription
		ON webhook_deliveries(subscription_id, created_at DESC)
	`)
	if err != nil {
		return fmt.Errorf("failed to create webhook_deliveries subscription index: %w", err)
	}

	// ========================================================================
	// PART 3: Create webhook trigger for contacts table
	// ========================================================================

	_, err = db.ExecContext(ctx, `
		CREATE OR REPLACE FUNCTION webhook_contacts_trigger()
		RETURNS TRIGGER AS $$
		DECLARE
			sub RECORD;
			event_kind VARCHAR(50);
			payload JSONB;
			contact_record RECORD;
		BEGIN
			-- Determine event kind and which record to use
			IF TG_OP = 'INSERT' THEN
				event_kind := 'contact.created';
				contact_record := NEW;
			ELSIF TG_OP = 'UPDATE' THEN
				event_kind := 'contact.updated';
				contact_record := NEW;
				-- Skip if nothing changed (compare all relevant fields)
				IF NEW.external_id IS NOT DISTINCT FROM OLD.external_id AND
				   NEW.timezone IS NOT DISTINCT FROM OLD.timezone AND
				   NEW.language IS NOT DISTINCT FROM OLD.language AND
				   NEW.first_name IS NOT DISTINCT FROM OLD.first_name AND
				   NEW.last_name IS NOT DISTINCT FROM OLD.last_name AND
				   NEW.phone IS NOT DISTINCT FROM OLD.phone AND
				   NEW.address_line_1 IS NOT DISTINCT FROM OLD.address_line_1 AND
				   NEW.address_line_2 IS NOT DISTINCT FROM OLD.address_line_2 AND
				   NEW.country IS NOT DISTINCT FROM OLD.country AND
				   NEW.postcode IS NOT DISTINCT FROM OLD.postcode AND
				   NEW.state IS NOT DISTINCT FROM OLD.state AND
				   NEW.job_title IS NOT DISTINCT FROM OLD.job_title AND
				   NEW.custom_string_1 IS NOT DISTINCT FROM OLD.custom_string_1 AND
				   NEW.custom_string_2 IS NOT DISTINCT FROM OLD.custom_string_2 AND
				   NEW.custom_string_3 IS NOT DISTINCT FROM OLD.custom_string_3 AND
				   NEW.custom_string_4 IS NOT DISTINCT FROM OLD.custom_string_4 AND
				   NEW.custom_string_5 IS NOT DISTINCT FROM OLD.custom_string_5 AND
				   NEW.custom_number_1 IS NOT DISTINCT FROM OLD.custom_number_1 AND
				   NEW.custom_number_2 IS NOT DISTINCT FROM OLD.custom_number_2 AND
				   NEW.custom_number_3 IS NOT DISTINCT FROM OLD.custom_number_3 AND
				   NEW.custom_number_4 IS NOT DISTINCT FROM OLD.custom_number_4 AND
				   NEW.custom_number_5 IS NOT DISTINCT FROM OLD.custom_number_5 AND
				   NEW.custom_datetime_1 IS NOT DISTINCT FROM OLD.custom_datetime_1 AND
				   NEW.custom_datetime_2 IS NOT DISTINCT FROM OLD.custom_datetime_2 AND
				   NEW.custom_datetime_3 IS NOT DISTINCT FROM OLD.custom_datetime_3 AND
				   NEW.custom_datetime_4 IS NOT DISTINCT FROM OLD.custom_datetime_4 AND
				   NEW.custom_datetime_5 IS NOT DISTINCT FROM OLD.custom_datetime_5 AND
				   NEW.custom_json_1 IS NOT DISTINCT FROM OLD.custom_json_1 AND
				   NEW.custom_json_2 IS NOT DISTINCT FROM OLD.custom_json_2 AND
				   NEW.custom_json_3 IS NOT DISTINCT FROM OLD.custom_json_3 AND
				   NEW.custom_json_4 IS NOT DISTINCT FROM OLD.custom_json_4 AND
				   NEW.custom_json_5 IS NOT DISTINCT FROM OLD.custom_json_5 THEN
					RETURN NEW;
				END IF;
			ELSIF TG_OP = 'DELETE' THEN
				event_kind := 'contact.deleted';
				contact_record := OLD;
			ELSE
				RETURN COALESCE(NEW, OLD);
			END IF;

			-- Build payload with full contact object
			payload := jsonb_build_object(
				'contact', to_jsonb(contact_record)
			);

			-- Insert webhook deliveries for matching subscriptions
			FOR sub IN
				SELECT id FROM webhook_subscriptions
				WHERE enabled = true AND event_kind = ANY(ARRAY(SELECT jsonb_array_elements_text(settings->'event_types')))
			LOOP
				INSERT INTO webhook_deliveries (id, subscription_id, event_type, payload, status, attempts, max_attempts, next_attempt_at)
				VALUES (gen_random_uuid()::text, sub.id, event_kind, payload, 'pending', 0, 10, NOW());
			END LOOP;
			RETURN COALESCE(NEW, OLD);
		END;
		$$ LANGUAGE plpgsql
	`)
	if err != nil {
		return fmt.Errorf("failed to create webhook_contacts_trigger function: %w", err)
	}

	_, err = db.ExecContext(ctx, `
		DROP TRIGGER IF EXISTS webhook_contacts ON contacts;
		CREATE TRIGGER webhook_contacts AFTER INSERT OR UPDATE OR DELETE ON contacts
		FOR EACH ROW EXECUTE FUNCTION webhook_contacts_trigger()
	`)
	if err != nil {
		return fmt.Errorf("failed to create webhook_contacts trigger: %w", err)
	}

	// ========================================================================
	// PART 4: Create webhook trigger for contact_lists table
	// ========================================================================

	_, err = db.ExecContext(ctx, `
		CREATE OR REPLACE FUNCTION webhook_contact_lists_trigger()
		RETURNS TRIGGER AS $$
		DECLARE
			sub RECORD;
			event_kind VARCHAR(50);
			payload JSONB;
			list_name VARCHAR(255);
		BEGIN
			-- Get list name for payload enrichment
			SELECT name INTO list_name FROM lists WHERE id = NEW.list_id;

			-- Determine event kind based on status transitions
			IF TG_OP = 'INSERT' THEN
				CASE NEW.status
					WHEN 'active' THEN event_kind := 'list.subscribed';
					WHEN 'pending' THEN event_kind := 'list.pending';
					WHEN 'unsubscribed' THEN event_kind := 'list.unsubscribed';
					WHEN 'bounced' THEN event_kind := 'list.bounced';
					WHEN 'complained' THEN event_kind := 'list.complained';
					ELSE RETURN NEW;
				END CASE;
			ELSIF TG_OP = 'UPDATE' THEN
				-- Detect status transitions
				IF NEW.status IS DISTINCT FROM OLD.status THEN
					IF OLD.status = 'pending' AND NEW.status = 'active' THEN
						event_kind := 'list.confirmed';
					ELSIF OLD.status IN ('unsubscribed', 'bounced', 'complained') AND NEW.status = 'active' THEN
						event_kind := 'list.resubscribed';
					ELSIF NEW.status = 'unsubscribed' THEN
						event_kind := 'list.unsubscribed';
					ELSIF NEW.status = 'bounced' THEN
						event_kind := 'list.bounced';
					ELSIF NEW.status = 'complained' THEN
						event_kind := 'list.complained';
					ELSE
						RETURN NEW;
					END IF;
				ELSIF NEW.deleted_at IS NOT NULL AND OLD.deleted_at IS NULL THEN
					event_kind := 'list.removed';
				ELSE
					RETURN NEW;
				END IF;
			ELSE
				RETURN NEW;
			END IF;

			-- Build payload
			payload := jsonb_build_object(
				'email', NEW.email,
				'list_id', NEW.list_id,
				'list_name', list_name,
				'status', NEW.status,
				'previous_status', CASE WHEN TG_OP = 'UPDATE' THEN OLD.status ELSE NULL END
			);

			-- Insert webhook deliveries for matching subscriptions
			FOR sub IN
				SELECT id FROM webhook_subscriptions
				WHERE enabled = true AND event_kind = ANY(ARRAY(SELECT jsonb_array_elements_text(settings->'event_types')))
			LOOP
				INSERT INTO webhook_deliveries (id, subscription_id, event_type, payload, status, attempts, max_attempts, next_attempt_at)
				VALUES (gen_random_uuid()::text, sub.id, event_kind, payload, 'pending', 0, 10, NOW());
			END LOOP;
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql
	`)
	if err != nil {
		return fmt.Errorf("failed to create webhook_contact_lists_trigger function: %w", err)
	}

	_, err = db.ExecContext(ctx, `
		DROP TRIGGER IF EXISTS webhook_contact_lists ON contact_lists;
		CREATE TRIGGER webhook_contact_lists AFTER INSERT OR UPDATE ON contact_lists
		FOR EACH ROW EXECUTE FUNCTION webhook_contact_lists_trigger()
	`)
	if err != nil {
		return fmt.Errorf("failed to create webhook_contact_lists trigger: %w", err)
	}

	// ========================================================================
	// PART 5: Create webhook trigger for contact_segments table
	// ========================================================================

	_, err = db.ExecContext(ctx, `
		CREATE OR REPLACE FUNCTION webhook_contact_segments_trigger()
		RETURNS TRIGGER AS $$
		DECLARE
			sub RECORD;
			event_kind VARCHAR(50);
			payload JSONB;
			segment_name VARCHAR(255);
			contact_email VARCHAR(255);
		BEGIN
			-- Get segment name for payload
			SELECT name INTO segment_name FROM segments WHERE id = COALESCE(NEW.segment_id, OLD.segment_id);
			-- contact_segments uses email directly as the key
			contact_email := COALESCE(NEW.email, OLD.email);

			-- Determine event kind
			IF TG_OP = 'INSERT' THEN
				event_kind := 'segment.joined';
			ELSIF TG_OP = 'DELETE' THEN
				event_kind := 'segment.left';
			ELSE
				RETURN NEW;
			END IF;

			-- Build payload
			payload := jsonb_build_object(
				'email', contact_email,
				'segment_id', COALESCE(NEW.segment_id, OLD.segment_id),
				'segment_name', segment_name
			);

			-- Insert webhook deliveries for matching subscriptions
			FOR sub IN
				SELECT id FROM webhook_subscriptions
				WHERE enabled = true AND event_kind = ANY(ARRAY(SELECT jsonb_array_elements_text(settings->'event_types')))
			LOOP
				INSERT INTO webhook_deliveries (id, subscription_id, event_type, payload, status, attempts, max_attempts, next_attempt_at)
				VALUES (gen_random_uuid()::text, sub.id, event_kind, payload, 'pending', 0, 10, NOW());
			END LOOP;

			IF TG_OP = 'DELETE' THEN
				RETURN OLD;
			END IF;
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql
	`)
	if err != nil {
		return fmt.Errorf("failed to create webhook_contact_segments_trigger function: %w", err)
	}

	_, err = db.ExecContext(ctx, `
		DROP TRIGGER IF EXISTS webhook_contact_segments ON contact_segments;
		CREATE TRIGGER webhook_contact_segments AFTER INSERT OR DELETE ON contact_segments
		FOR EACH ROW EXECUTE FUNCTION webhook_contact_segments_trigger()
	`)
	if err != nil {
		return fmt.Errorf("failed to create webhook_contact_segments trigger: %w", err)
	}

	// ========================================================================
	// PART 6: Create webhook trigger for message_history table
	// ========================================================================

	_, err = db.ExecContext(ctx, `
		CREATE OR REPLACE FUNCTION webhook_message_history_trigger()
		RETURNS TRIGGER AS $$
		DECLARE
			sub RECORD;
			event_kind VARCHAR(50);
			event_timestamp TIMESTAMPTZ;
			payload JSONB;
		BEGIN
			-- Detect which email event occurred
			IF TG_OP = 'INSERT' THEN
				event_kind := 'email.sent';
				event_timestamp := NEW.sent_at;
			ELSIF TG_OP = 'UPDATE' THEN
				IF NEW.delivered_at IS NOT NULL AND OLD.delivered_at IS NULL THEN
					event_kind := 'email.delivered';
					event_timestamp := NEW.delivered_at;
				ELSIF NEW.opened_at IS NOT NULL AND OLD.opened_at IS NULL THEN
					event_kind := 'email.opened';
					event_timestamp := NEW.opened_at;
				ELSIF NEW.clicked_at IS NOT NULL AND OLD.clicked_at IS NULL THEN
					event_kind := 'email.clicked';
					event_timestamp := NEW.clicked_at;
				ELSIF NEW.bounced_at IS NOT NULL AND OLD.bounced_at IS NULL THEN
					event_kind := 'email.bounced';
					event_timestamp := NEW.bounced_at;
				ELSIF NEW.complained_at IS NOT NULL AND OLD.complained_at IS NULL THEN
					event_kind := 'email.complained';
					event_timestamp := NEW.complained_at;
				ELSIF NEW.unsubscribed_at IS NOT NULL AND OLD.unsubscribed_at IS NULL THEN
					event_kind := 'email.unsubscribed';
					event_timestamp := NEW.unsubscribed_at;
				ELSE
					RETURN NEW;
				END IF;
			ELSE
				RETURN NEW;
			END IF;

			-- Build rich payload with full message context
			payload := jsonb_build_object(
				'email', NEW.contact_email,
				'message_id', NEW.id,
				'template_id', NEW.template_id,
				'broadcast_id', NEW.broadcast_id,
				'list_id', NEW.list_id,
				'channel', NEW.channel,
				'event_timestamp', event_timestamp
			);

			-- Insert webhook deliveries for matching subscriptions
			FOR sub IN
				SELECT id FROM webhook_subscriptions
				WHERE enabled = true AND event_kind = ANY(ARRAY(SELECT jsonb_array_elements_text(settings->'event_types')))
			LOOP
				INSERT INTO webhook_deliveries (id, subscription_id, event_type, payload, status, attempts, max_attempts, next_attempt_at)
				VALUES (gen_random_uuid()::text, sub.id, event_kind, payload, 'pending', 0, 10, NOW());
			END LOOP;
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql
	`)
	if err != nil {
		return fmt.Errorf("failed to create webhook_message_history_trigger function: %w", err)
	}

	_, err = db.ExecContext(ctx, `
		DROP TRIGGER IF EXISTS webhook_message_history ON message_history;
		CREATE TRIGGER webhook_message_history AFTER INSERT OR UPDATE ON message_history
		FOR EACH ROW EXECUTE FUNCTION webhook_message_history_trigger()
	`)
	if err != nil {
		return fmt.Errorf("failed to create webhook_message_history trigger: %w", err)
	}

	// ========================================================================
	// PART 7: Create webhook trigger for custom_events table
	// ========================================================================

	_, err = db.ExecContext(ctx, `
		CREATE OR REPLACE FUNCTION webhook_custom_events_trigger()
		RETURNS TRIGGER AS $$
		DECLARE
			sub RECORD;
			custom_filters JSONB;
			should_deliver BOOLEAN;
			payload JSONB;
			event_kind VARCHAR(50);
			subscribed_event_type VARCHAR(50);
		BEGIN
			-- Determine event kind based on operation and soft-delete status
			IF TG_OP = 'INSERT' THEN
				-- New record - check if it's being created as deleted
				IF NEW.deleted_at IS NOT NULL THEN
					event_kind := 'custom_event.deleted';
					subscribed_event_type := 'custom_event.deleted';
				ELSE
					event_kind := 'custom_event.created';
					subscribed_event_type := 'custom_event.created';
				END IF;
			ELSIF TG_OP = 'UPDATE' THEN
				-- Check for soft-delete: was not deleted, now is deleted
				IF (OLD.deleted_at IS NULL AND NEW.deleted_at IS NOT NULL) THEN
					event_kind := 'custom_event.deleted';
					subscribed_event_type := 'custom_event.deleted';
				-- Check for restore: was deleted, now is not deleted
				ELSIF (OLD.deleted_at IS NOT NULL AND NEW.deleted_at IS NULL) THEN
					event_kind := 'custom_event.created';
					subscribed_event_type := 'custom_event.created';
				-- Regular update (skip if record is deleted)
				ELSIF NEW.deleted_at IS NULL THEN
					event_kind := 'custom_event.updated';
					subscribed_event_type := 'custom_event.updated';
				ELSE
					-- Record is deleted and staying deleted, skip
					RETURN NEW;
				END IF;
			ELSE
				RETURN NEW;
			END IF;

			-- Build payload with full custom event object
			payload := jsonb_build_object(
				'custom_event', to_jsonb(NEW)
			);

			-- Find matching subscriptions with the correct event type
			FOR sub IN
				SELECT id, settings FROM webhook_subscriptions
				WHERE enabled = true AND subscribed_event_type = ANY(ARRAY(SELECT jsonb_array_elements_text(settings->'event_types')))
			LOOP
				should_deliver := true;
				custom_filters := sub.settings->'custom_event_filters';

				-- Apply goal_types filter if specified
				IF custom_filters IS NOT NULL AND custom_filters ? 'goal_types'
				   AND jsonb_array_length(custom_filters->'goal_types') > 0 THEN
					IF NEW.goal_type IS NULL OR NOT (NEW.goal_type = ANY(
						SELECT jsonb_array_elements_text(custom_filters->'goal_types')
					)) THEN
						should_deliver := false;
					END IF;
				END IF;

				-- Apply event_names filter if specified
				IF should_deliver AND custom_filters IS NOT NULL AND custom_filters ? 'event_names'
				   AND jsonb_array_length(custom_filters->'event_names') > 0 THEN
					IF NOT (NEW.event_name = ANY(
						SELECT jsonb_array_elements_text(custom_filters->'event_names')
					)) THEN
						should_deliver := false;
					END IF;
				END IF;

				IF should_deliver THEN
					INSERT INTO webhook_deliveries (id, subscription_id, event_type, payload, status, attempts, max_attempts, next_attempt_at)
					VALUES (gen_random_uuid()::text, sub.id, event_kind, payload, 'pending', 0, 10, NOW());
				END IF;
			END LOOP;
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql
	`)
	if err != nil {
		return fmt.Errorf("failed to create webhook_custom_events_trigger function: %w", err)
	}

	_, err = db.ExecContext(ctx, `
		DROP TRIGGER IF EXISTS webhook_custom_events ON custom_events;
		CREATE TRIGGER webhook_custom_events AFTER INSERT OR UPDATE ON custom_events
		FOR EACH ROW EXECUTE FUNCTION webhook_custom_events_trigger()
	`)
	if err != nil {
		return fmt.Errorf("failed to create webhook_custom_events trigger: %w", err)
	}

	// ========================================================================
	// PART 8: Add full_name column to contacts table
	// ========================================================================

	_, err = db.ExecContext(ctx, `
		ALTER TABLE contacts ADD COLUMN IF NOT EXISTS full_name VARCHAR(255)
	`)
	if err != nil {
		return fmt.Errorf("failed to add full_name column: %w", err)
	}

	// ========================================================================
	// PART 9: Fix track_contact_changes trigger to use CURRENT_TIMESTAMP for updates
	// and add full_name field tracking
	// ========================================================================

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
				IF OLD.full_name IS DISTINCT FROM NEW.full_name THEN changes_json := changes_json || jsonb_build_object('full_name', jsonb_build_object('old', OLD.full_name, 'new', NEW.full_name)); END IF;
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
				VALUES (NEW.email, op, 'contact', op || '_contact', changes_json, NEW.created_at);
			ELSE
				INSERT INTO contact_timeline (email, operation, entity_type, kind, changes, created_at)
				VALUES (NEW.email, op, 'contact', op || '_contact', changes_json, CURRENT_TIMESTAMP);
			END IF;
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql
	`)
	if err != nil {
		return fmt.Errorf("failed to update track_contact_changes function: %w", err)
	}

	// Also update the webhook_contacts_trigger to include full_name in the change detection
	_, err = db.ExecContext(ctx, `
		CREATE OR REPLACE FUNCTION webhook_contacts_trigger()
		RETURNS TRIGGER AS $$
		DECLARE
			sub RECORD;
			event_kind VARCHAR(50);
			payload JSONB;
			contact_record RECORD;
		BEGIN
			-- Determine event kind and which record to use
			IF TG_OP = 'INSERT' THEN
				event_kind := 'contact.created';
				contact_record := NEW;
			ELSIF TG_OP = 'UPDATE' THEN
				event_kind := 'contact.updated';
				contact_record := NEW;
				-- Skip if nothing changed (compare all relevant fields)
				IF NEW.external_id IS NOT DISTINCT FROM OLD.external_id AND
				   NEW.timezone IS NOT DISTINCT FROM OLD.timezone AND
				   NEW.language IS NOT DISTINCT FROM OLD.language AND
				   NEW.first_name IS NOT DISTINCT FROM OLD.first_name AND
				   NEW.last_name IS NOT DISTINCT FROM OLD.last_name AND
				   NEW.full_name IS NOT DISTINCT FROM OLD.full_name AND
				   NEW.phone IS NOT DISTINCT FROM OLD.phone AND
				   NEW.address_line_1 IS NOT DISTINCT FROM OLD.address_line_1 AND
				   NEW.address_line_2 IS NOT DISTINCT FROM OLD.address_line_2 AND
				   NEW.country IS NOT DISTINCT FROM OLD.country AND
				   NEW.postcode IS NOT DISTINCT FROM OLD.postcode AND
				   NEW.state IS NOT DISTINCT FROM OLD.state AND
				   NEW.job_title IS NOT DISTINCT FROM OLD.job_title AND
				   NEW.custom_string_1 IS NOT DISTINCT FROM OLD.custom_string_1 AND
				   NEW.custom_string_2 IS NOT DISTINCT FROM OLD.custom_string_2 AND
				   NEW.custom_string_3 IS NOT DISTINCT FROM OLD.custom_string_3 AND
				   NEW.custom_string_4 IS NOT DISTINCT FROM OLD.custom_string_4 AND
				   NEW.custom_string_5 IS NOT DISTINCT FROM OLD.custom_string_5 AND
				   NEW.custom_number_1 IS NOT DISTINCT FROM OLD.custom_number_1 AND
				   NEW.custom_number_2 IS NOT DISTINCT FROM OLD.custom_number_2 AND
				   NEW.custom_number_3 IS NOT DISTINCT FROM OLD.custom_number_3 AND
				   NEW.custom_number_4 IS NOT DISTINCT FROM OLD.custom_number_4 AND
				   NEW.custom_number_5 IS NOT DISTINCT FROM OLD.custom_number_5 AND
				   NEW.custom_datetime_1 IS NOT DISTINCT FROM OLD.custom_datetime_1 AND
				   NEW.custom_datetime_2 IS NOT DISTINCT FROM OLD.custom_datetime_2 AND
				   NEW.custom_datetime_3 IS NOT DISTINCT FROM OLD.custom_datetime_3 AND
				   NEW.custom_datetime_4 IS NOT DISTINCT FROM OLD.custom_datetime_4 AND
				   NEW.custom_datetime_5 IS NOT DISTINCT FROM OLD.custom_datetime_5 AND
				   NEW.custom_json_1 IS NOT DISTINCT FROM OLD.custom_json_1 AND
				   NEW.custom_json_2 IS NOT DISTINCT FROM OLD.custom_json_2 AND
				   NEW.custom_json_3 IS NOT DISTINCT FROM OLD.custom_json_3 AND
				   NEW.custom_json_4 IS NOT DISTINCT FROM OLD.custom_json_4 AND
				   NEW.custom_json_5 IS NOT DISTINCT FROM OLD.custom_json_5 THEN
					RETURN NEW;
				END IF;
			ELSIF TG_OP = 'DELETE' THEN
				event_kind := 'contact.deleted';
				contact_record := OLD;
			ELSE
				RETURN COALESCE(NEW, OLD);
			END IF;

			-- Build payload with full contact object
			payload := jsonb_build_object(
				'contact', to_jsonb(contact_record)
			);

			-- Insert webhook deliveries for matching subscriptions
			FOR sub IN
				SELECT id FROM webhook_subscriptions
				WHERE enabled = true AND event_kind = ANY(ARRAY(SELECT jsonb_array_elements_text(settings->'event_types')))
			LOOP
				INSERT INTO webhook_deliveries (id, subscription_id, event_type, payload, status, attempts, max_attempts, next_attempt_at)
				VALUES (gen_random_uuid()::text, sub.id, event_kind, payload, 'pending', 0, 10, NOW());
			END LOOP;
			RETURN COALESCE(NEW, OLD);
		END;
		$$ LANGUAGE plpgsql
	`)
	if err != nil {
		return fmt.Errorf("failed to update webhook_contacts_trigger function: %w", err)
	}

	return nil
}

func init() {
	Register(&V19Migration{})
}
