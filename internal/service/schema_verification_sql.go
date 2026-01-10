package service

// SQL definitions for trigger functions - copied from internal/database/init.go
// These are used for schema verification and repair

const trackContactChangesSQL = `CREATE OR REPLACE FUNCTION track_contact_changes()
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
	VALUES (NEW.email, op, 'contact', 'contact.created', changes_json, NEW.created_at);
ELSE
	INSERT INTO contact_timeline (email, operation, entity_type, kind, changes, created_at)
	VALUES (NEW.email, op, 'contact', 'contact.updated', changes_json, NEW.updated_at);
END IF;
	RETURN NEW;
END;
$$ LANGUAGE plpgsql`

const trackContactListChangesSQL = `CREATE OR REPLACE FUNCTION track_contact_list_changes()
RETURNS TRIGGER AS $$
DECLARE
	changes_json JSONB := '{}'::jsonb;
	op VARCHAR(20);
	kind_value VARCHAR(50);
BEGIN
	IF TG_OP = 'INSERT' THEN
		op := 'insert';
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
		IF OLD.deleted_at IS DISTINCT FROM NEW.deleted_at AND NEW.deleted_at IS NOT NULL THEN
			kind_value := 'list.removed';
			changes_json := jsonb_build_object(
				'deleted_at', jsonb_build_object('old', OLD.deleted_at, 'new', NEW.deleted_at)
			);
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
$$ LANGUAGE plpgsql`

const trackMessageHistoryChangesSQL = `CREATE OR REPLACE FUNCTION track_message_history_changes()
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
$$ LANGUAGE plpgsql`

const trackInboundWebhookEventChangesSQL = `CREATE OR REPLACE FUNCTION track_inbound_webhook_event_changes()
RETURNS TRIGGER AS $$
DECLARE
	changes_json JSONB := '{}'::jsonb;
	entity_id_value VARCHAR(255);
BEGIN
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
$$ LANGUAGE plpgsql`

const trackContactSegmentChangesSQL = `CREATE OR REPLACE FUNCTION track_contact_segment_changes()
RETURNS TRIGGER AS $$
DECLARE
	changes_json JSONB := '{}'::jsonb;
	op VARCHAR(20);
	kind_value VARCHAR(50);
BEGIN
	IF TG_OP = 'INSERT' THEN
		op := 'insert';
		kind_value := 'segment.joined';
		changes_json := jsonb_build_object('segment_id', jsonb_build_object('new', NEW.segment_id), 'version', jsonb_build_object('new', NEW.version), 'matched_at', jsonb_build_object('new', NEW.matched_at));
	ELSIF TG_OP = 'DELETE' THEN
		op := 'delete';
		kind_value := 'segment.left';
		changes_json := jsonb_build_object('segment_id', jsonb_build_object('old', OLD.segment_id), 'version', jsonb_build_object('old', OLD.version));
		INSERT INTO contact_timeline (email, operation, entity_type, kind, entity_id, changes, created_at)
		VALUES (OLD.email, op, 'contact_segment', kind_value, OLD.segment_id, changes_json, CURRENT_TIMESTAMP);
		RETURN OLD;
	END IF;
	INSERT INTO contact_timeline (email, operation, entity_type, kind, entity_id, changes, created_at)
	VALUES (NEW.email, op, 'contact_segment', kind_value, NEW.segment_id, changes_json, CURRENT_TIMESTAMP);
	RETURN NEW;
END;
$$ LANGUAGE plpgsql`

const queueContactForSegmentRecomputationSQL = `CREATE OR REPLACE FUNCTION queue_contact_for_segment_recomputation()
RETURNS TRIGGER AS $$
BEGIN
	INSERT INTO contact_segment_queue (email, queued_at)
	VALUES (NEW.email, CURRENT_TIMESTAMP)
	ON CONFLICT (email) DO UPDATE SET queued_at = EXCLUDED.queued_at;
	RETURN NEW;
END;
$$ LANGUAGE plpgsql`

const updateContactListsOnStatusChangeSQL = `CREATE OR REPLACE FUNCTION update_contact_lists_on_status_change()
RETURNS TRIGGER AS $$
BEGIN
	IF NEW.complained_at IS NOT NULL AND OLD.complained_at IS NULL THEN
		IF NEW.list_id IS NOT NULL THEN
			UPDATE contact_lists
			SET status = 'complained',
				updated_at = NEW.complained_at
			WHERE email = NEW.contact_email
			AND list_id = NEW.list_id
			AND status != 'complained';
		END IF;
	END IF;
	IF NEW.bounced_at IS NOT NULL AND OLD.bounced_at IS NULL THEN
		IF NEW.list_id IS NOT NULL THEN
			UPDATE contact_lists
			SET status = 'bounced',
				updated_at = NEW.bounced_at
			WHERE email = NEW.contact_email
			AND list_id = NEW.list_id
			AND status NOT IN ('complained', 'bounced');
		END IF;
	END IF;
	RETURN NEW;
END;
$$ LANGUAGE plpgsql`

const trackCustomEventTimelineSQL = `CREATE OR REPLACE FUNCTION track_custom_event_timeline()
RETURNS TRIGGER AS $$
DECLARE
	timeline_operation TEXT;
	changes_json JSONB;
	property_key TEXT;
	property_diff JSONB;
	kind_value TEXT;
BEGIN
	IF TG_OP = 'INSERT' THEN
		timeline_operation := 'insert';
		kind_value := 'custom_event.' || NEW.event_name;
		changes_json := jsonb_build_object(
			'event_name', jsonb_build_object('new', NEW.event_name),
			'external_id', jsonb_build_object('new', NEW.external_id)
		);
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
		timeline_operation := 'update';
		kind_value := 'custom_event.' || NEW.event_name;
		property_diff := '{}'::jsonb;
		FOR property_key IN
			SELECT DISTINCT key
			FROM (
				SELECT key FROM jsonb_object_keys(OLD.properties) AS key
				UNION
				SELECT key FROM jsonb_object_keys(NEW.properties) AS key
			) AS all_keys
		LOOP
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
	INSERT INTO contact_timeline (
		email, operation, entity_type, kind, entity_id, changes, created_at
	) VALUES (
		NEW.email, timeline_operation, 'custom_event', kind_value,
		NEW.external_id, changes_json, NEW.occurred_at
	);
	RETURN NEW;
END;
$$ LANGUAGE plpgsql`

const webhookContactsTriggerSQL = `CREATE OR REPLACE FUNCTION webhook_contacts_trigger()
RETURNS TRIGGER AS $$
DECLARE
	sub RECORD;
	event_kind VARCHAR(50);
	payload JSONB;
	contact_record RECORD;
BEGIN
	IF TG_OP = 'INSERT' THEN
		event_kind := 'contact.created';
		contact_record := NEW;
	ELSIF TG_OP = 'UPDATE' THEN
		event_kind := 'contact.updated';
		contact_record := NEW;
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
	payload := jsonb_build_object(
		'contact', to_jsonb(contact_record)
	);
	FOR sub IN
		SELECT id FROM webhook_subscriptions
		WHERE enabled = true AND event_kind = ANY(ARRAY(SELECT jsonb_array_elements_text(settings->'event_types')))
	LOOP
		INSERT INTO webhook_deliveries (id, subscription_id, event_type, payload, status, attempts, max_attempts, next_attempt_at)
		VALUES (gen_random_uuid()::text, sub.id, event_kind, payload, 'pending', 0, 10, NOW());
	END LOOP;
	RETURN COALESCE(NEW, OLD);
END;
$$ LANGUAGE plpgsql`

const webhookContactListsTriggerSQL = `CREATE OR REPLACE FUNCTION webhook_contact_lists_trigger()
RETURNS TRIGGER AS $$
DECLARE
	sub RECORD;
	event_kind VARCHAR(50);
	payload JSONB;
	list_name VARCHAR(255);
BEGIN
	SELECT name INTO list_name FROM lists WHERE id = NEW.list_id;
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
	payload := jsonb_build_object(
		'email', NEW.email,
		'list_id', NEW.list_id,
		'list_name', list_name,
		'status', NEW.status,
		'previous_status', CASE WHEN TG_OP = 'UPDATE' THEN OLD.status ELSE NULL END
	);
	FOR sub IN
		SELECT id FROM webhook_subscriptions
		WHERE enabled = true AND event_kind = ANY(ARRAY(SELECT jsonb_array_elements_text(settings->'event_types')))
	LOOP
		INSERT INTO webhook_deliveries (id, subscription_id, event_type, payload, status, attempts, max_attempts, next_attempt_at)
		VALUES (gen_random_uuid()::text, sub.id, event_kind, payload, 'pending', 0, 10, NOW());
	END LOOP;
	RETURN NEW;
END;
$$ LANGUAGE plpgsql`

const webhookContactSegmentsTriggerSQL = `CREATE OR REPLACE FUNCTION webhook_contact_segments_trigger()
RETURNS TRIGGER AS $$
DECLARE
	sub RECORD;
	event_kind VARCHAR(50);
	payload JSONB;
	segment_name VARCHAR(255);
	contact_email VARCHAR(255);
BEGIN
	SELECT name INTO segment_name FROM segments WHERE id = COALESCE(NEW.segment_id, OLD.segment_id);
	contact_email := COALESCE(NEW.email, OLD.email);
	IF TG_OP = 'INSERT' THEN
		event_kind := 'segment.joined';
		payload := jsonb_build_object(
			'email', contact_email,
			'segment_id', NEW.segment_id,
			'segment_name', segment_name,
			'matched_at', NEW.matched_at
		);
	ELSIF TG_OP = 'DELETE' THEN
		event_kind := 'segment.left';
		payload := jsonb_build_object(
			'email', contact_email,
			'segment_id', OLD.segment_id,
			'segment_name', segment_name,
			'left_at', NOW()
		);
	ELSE
		RETURN COALESCE(NEW, OLD);
	END IF;
	FOR sub IN
		SELECT id FROM webhook_subscriptions
		WHERE enabled = true AND event_kind = ANY(ARRAY(SELECT jsonb_array_elements_text(settings->'event_types')))
	LOOP
		INSERT INTO webhook_deliveries (id, subscription_id, event_type, payload, status, attempts, max_attempts, next_attempt_at)
		VALUES (gen_random_uuid()::text, sub.id, event_kind, payload, 'pending', 0, 10, NOW());
	END LOOP;
	RETURN COALESCE(NEW, OLD);
END;
$$ LANGUAGE plpgsql`

const webhookMessageHistoryTriggerSQL = `CREATE OR REPLACE FUNCTION webhook_message_history_trigger()
RETURNS TRIGGER AS $$
DECLARE
	sub RECORD;
	event_kind VARCHAR(50);
	payload JSONB;
BEGIN
	IF TG_OP = 'INSERT' THEN
		event_kind := 'email.sent';
	ELSIF TG_OP = 'UPDATE' THEN
		IF OLD.delivered_at IS NULL AND NEW.delivered_at IS NOT NULL THEN
			event_kind := 'email.delivered';
		ELSIF OLD.opened_at IS NULL AND NEW.opened_at IS NOT NULL THEN
			event_kind := 'email.opened';
		ELSIF OLD.clicked_at IS NULL AND NEW.clicked_at IS NOT NULL THEN
			event_kind := 'email.clicked';
		ELSIF OLD.bounced_at IS NULL AND NEW.bounced_at IS NOT NULL THEN
			event_kind := 'email.bounced';
		ELSIF OLD.complained_at IS NULL AND NEW.complained_at IS NOT NULL THEN
			event_kind := 'email.complained';
		ELSIF OLD.unsubscribed_at IS NULL AND NEW.unsubscribed_at IS NOT NULL THEN
			event_kind := 'email.unsubscribed';
		ELSIF OLD.failed_at IS NULL AND NEW.failed_at IS NOT NULL THEN
			event_kind := 'email.failed';
		ELSE
			RETURN NEW;
		END IF;
	ELSE
		RETURN NEW;
	END IF;
	payload := jsonb_build_object(
		'message_id', NEW.id,
		'contact_email', NEW.contact_email,
		'template_id', NEW.template_id,
		'template_version', NEW.template_version,
		'channel', NEW.channel,
		'broadcast_id', NEW.broadcast_id,
		'automation_id', NEW.automation_id,
		'list_id', NEW.list_id,
		'status_info', NEW.status_info,
		'sent_at', NEW.sent_at,
		'delivered_at', NEW.delivered_at,
		'opened_at', NEW.opened_at,
		'clicked_at', NEW.clicked_at,
		'bounced_at', NEW.bounced_at,
		'complained_at', NEW.complained_at,
		'unsubscribed_at', NEW.unsubscribed_at,
		'failed_at', NEW.failed_at
	);
	FOR sub IN
		SELECT id FROM webhook_subscriptions
		WHERE enabled = true AND event_kind = ANY(ARRAY(SELECT jsonb_array_elements_text(settings->'event_types')))
	LOOP
		INSERT INTO webhook_deliveries (id, subscription_id, event_type, payload, status, attempts, max_attempts, next_attempt_at)
		VALUES (gen_random_uuid()::text, sub.id, event_kind, payload, 'pending', 0, 10, NOW());
	END LOOP;
	RETURN NEW;
END;
$$ LANGUAGE plpgsql`

const webhookCustomEventsTriggerSQL = `CREATE OR REPLACE FUNCTION webhook_custom_events_trigger()
RETURNS TRIGGER AS $$
DECLARE
	sub RECORD;
	custom_filters JSONB;
	should_deliver BOOLEAN;
	payload JSONB;
	event_kind VARCHAR(50);
	subscribed_event_type VARCHAR(50);
BEGIN
	IF TG_OP = 'INSERT' THEN
		IF NEW.deleted_at IS NOT NULL THEN
			event_kind := 'custom_event.deleted';
			subscribed_event_type := 'custom_event.deleted';
		ELSE
			event_kind := 'custom_event.created';
			subscribed_event_type := 'custom_event.created';
		END IF;
	ELSIF TG_OP = 'UPDATE' THEN
		IF (OLD.deleted_at IS NULL AND NEW.deleted_at IS NOT NULL) THEN
			event_kind := 'custom_event.deleted';
			subscribed_event_type := 'custom_event.deleted';
		ELSIF (OLD.deleted_at IS NOT NULL AND NEW.deleted_at IS NULL) THEN
			event_kind := 'custom_event.created';
			subscribed_event_type := 'custom_event.created';
		ELSIF NEW.deleted_at IS NULL THEN
			event_kind := 'custom_event.updated';
			subscribed_event_type := 'custom_event.updated';
		ELSE
			RETURN NEW;
		END IF;
	ELSE
		RETURN NEW;
	END IF;
	payload := jsonb_build_object('custom_event', to_jsonb(NEW));
	FOR sub IN
		SELECT id, settings FROM webhook_subscriptions
		WHERE enabled = true AND subscribed_event_type = ANY(ARRAY(SELECT jsonb_array_elements_text(settings->'event_types')))
	LOOP
		should_deliver := true;
		custom_filters := sub.settings->'custom_event_filters';
		IF custom_filters IS NOT NULL AND custom_filters ? 'goal_types'
		   AND jsonb_array_length(custom_filters->'goal_types') > 0 THEN
			IF NEW.goal_type IS NULL OR NOT (NEW.goal_type = ANY(
				SELECT jsonb_array_elements_text(custom_filters->'goal_types')
			)) THEN
				should_deliver := false;
			END IF;
		END IF;
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
$$ LANGUAGE plpgsql`

const automationEnrollContactSQL = `CREATE OR REPLACE FUNCTION automation_enroll_contact(
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
	IF p_list_id IS NOT NULL AND p_list_id != '' THEN
		SELECT EXISTS(
			SELECT 1 FROM contact_lists
			WHERE email = p_contact_email
			AND list_id = p_list_id
			AND status = 'active'
			AND deleted_at IS NULL
		) INTO v_is_subscribed;
		IF NOT v_is_subscribed THEN
			RETURN;
		END IF;
	END IF;
	IF p_frequency = 'once' THEN
		SELECT EXISTS(
			SELECT 1 FROM automation_trigger_log
			WHERE automation_id = p_automation_id
			AND contact_email = p_contact_email
		) INTO v_already_triggered;
		IF v_already_triggered THEN
			RETURN;
		END IF;
		INSERT INTO automation_trigger_log (id, automation_id, contact_email, triggered_at)
		VALUES (gen_random_uuid()::text, p_automation_id, p_contact_email, NOW())
		ON CONFLICT (automation_id, contact_email) DO NOTHING;
	END IF;
	v_new_id := gen_random_uuid()::text;
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
	UPDATE automations
	SET stats = jsonb_set(
		COALESCE(stats, '{}'::jsonb),
		'{enrolled}',
		to_jsonb(COALESCE((stats->>'enrolled')::int, 0) + 1)
	),
	updated_at = NOW()
	WHERE id = p_automation_id;
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
$$ LANGUAGE plpgsql`
