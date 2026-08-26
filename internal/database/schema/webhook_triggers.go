package schema

// The five outbound webhook trigger functions live here, rather than inline in
// the workspace initializer, because two independent paths install them: the
// new-workspace path (internal/database/init.go) and the migration that
// reinstalls them on every existing workspace. A body that drifts between those
// two makes a fresh install emit a different payload from an upgraded one for
// identical data, and the payload is a public contract — a webhook consumer
// maps its fields once and never looks again.
//
// Note internal/migrations/v19.go carries the ORIGINAL bodies and must stay
// frozen: a historical migration has to keep reproducing the behaviour it
// shipped, and every workspace it ran against is brought forward by the later
// migration that installs these instead.

// WebhookTriggerFunctions returns every outbound webhook trigger function
// definition, for callers that install the whole set.
//
// Each entry is a CREATE OR REPLACE FUNCTION, deliberately without the
// accompanying CREATE TRIGGER: replacing a function body is a pg_proc row
// update, while DROP/CREATE TRIGGER takes a ShareRowExclusiveLock on the table
// the trigger is attached to. Those tables are contacts, contact_lists,
// contact_segments, message_history and custom_events, so reattaching triggers
// inside a migration would block every customer write until the migration
// transaction commits. An already-attached trigger picks up the new body on its
// next invocation without being touched.
func WebhookTriggerFunctions() []string {
	return []string{
		WebhookContactsTriggerFunction(),
		WebhookContactListsTriggerFunction(),
		WebhookContactSegmentsTriggerFunction(),
		WebhookMessageHistoryTriggerFunction(),
		WebhookCustomEventsTriggerFunction(),
	}
}

// WebhookContactsTriggerFunction returns the trigger function that fans contacts
// rows out to outbound webhook subscriptions as contact.created,
// contact.updated and contact.deleted.
//
// The update branch compares an explicit column list and returns early when
// none of them moved, so a touch-only UPDATE emits nothing. Any column added to
// contacts is invisible to webhook consumers until it is added to that list.
func WebhookContactsTriggerFunction() string {
	return `CREATE OR REPLACE FUNCTION webhook_contacts_trigger()
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
		$$ LANGUAGE plpgsql`
}

// WebhookContactListsTriggerFunction returns the trigger function that fans
// contact_lists rows out as list.* events, narrowed by the subscription's
// optional list_ids filter.
func WebhookContactListsTriggerFunction() string {
	return `CREATE OR REPLACE FUNCTION webhook_contact_lists_trigger()
		RETURNS TRIGGER AS $$
		DECLARE
			sub RECORD;
			event_kind VARCHAR(50);
			payload JSONB;
			list_name VARCHAR(255);
			list_filter JSONB;
			should_deliver BOOLEAN;
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
				SELECT id, settings FROM webhook_subscriptions
				WHERE enabled = true AND event_kind = ANY(ARRAY(SELECT jsonb_array_elements_text(settings->'event_types')))
			LOOP
				should_deliver := true;
				list_filter := sub.settings->'list_ids';

				-- Absent, JSON null and the empty array all mean "every list",
				-- which is what every subscription written before list_ids
				-- existed carries. This IF body is the only place should_deliver
				-- can become false, so an unrecognised shape can only ever widen
				-- the match — it can never silence a live subscription.
				--
				-- jsonb_typeof rather than a "? 'list_ids'" key test because
				-- jsonb_array_length raises on a scalar, and this trigger runs
				-- inside the customer's own write transaction: an error here
				-- fails their INSERT into contact_lists, not just the webhook.
				IF jsonb_typeof(list_filter) = 'array' AND jsonb_array_length(list_filter) > 0 THEN
					IF NOT (NEW.list_id = ANY(
						SELECT jsonb_array_elements_text(list_filter)
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
}

// WebhookContactSegmentsTriggerFunction returns the trigger function that fans
// contact_segments rows out as segment.joined and segment.left, narrowed by the
// subscription's optional segment_ids filter.
//
// It is attached AFTER INSERT OR DELETE, so NEW is unassigned on the segment.left
// path and every read of the segment id has to come through COALESCE.
func WebhookContactSegmentsTriggerFunction() string {
	return `CREATE OR REPLACE FUNCTION webhook_contact_segments_trigger()
		RETURNS TRIGGER AS $$
		DECLARE
			sub RECORD;
			event_kind VARCHAR(50);
			payload JSONB;
			segment_name VARCHAR(255);
			contact_email VARCHAR(255);
			segment_filter JSONB;
			should_deliver BOOLEAN;
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
				SELECT id, settings FROM webhook_subscriptions
				WHERE enabled = true AND event_kind = ANY(ARRAY(SELECT jsonb_array_elements_text(settings->'event_types')))
			LOOP
				should_deliver := true;
				segment_filter := sub.settings->'segment_ids';

				-- Absent, JSON null and the empty array all mean "every
				-- segment", which is what every subscription written before
				-- segment_ids existed carries. This IF body is the only place
				-- should_deliver can become false, so an unrecognised shape can
				-- only ever widen the match — it can never silence a live
				-- subscription.
				--
				-- jsonb_typeof rather than a "? 'segment_ids'" key test because
				-- jsonb_array_length raises on a scalar, and this trigger runs
				-- inside the transaction that recomputes segment membership: an
				-- error here fails that write, not just the webhook.
				IF jsonb_typeof(segment_filter) = 'array' AND jsonb_array_length(segment_filter) > 0 THEN
					IF NOT (COALESCE(NEW.segment_id, OLD.segment_id) = ANY(
						SELECT jsonb_array_elements_text(segment_filter)
					)) THEN
						should_deliver := false;
					END IF;
				END IF;

				IF should_deliver THEN
					INSERT INTO webhook_deliveries (id, subscription_id, event_type, payload, status, attempts, max_attempts, next_attempt_at)
					VALUES (gen_random_uuid()::text, sub.id, event_kind, payload, 'pending', 0, 10, NOW());
				END IF;
			END LOOP;

			IF TG_OP = 'DELETE' THEN
				RETURN OLD;
			END IF;
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql`
}

// WebhookMessageHistoryTriggerFunction returns the trigger function that fans
// message_history rows out as email.* events.
//
// The UPDATE branch is an ELSIF chain over first-transition timestamps, so one
// UPDATE that stamps two of them emits only the first match in chain order.
func WebhookMessageHistoryTriggerFunction() string {
	return `CREATE OR REPLACE FUNCTION webhook_message_history_trigger()
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
		$$ LANGUAGE plpgsql`
}

// WebhookCustomEventsTriggerFunction returns the trigger function that fans
// custom_events rows out to outbound webhook subscriptions.
//
// It lives here, rather than inline in the initializer, for the same reason the
// web analytics DDL does: the new-workspace path (internal/database/init.go) and
// the v38 migration both install it, and a body that drifts between them makes a
// fresh install behave differently from an upgraded one for identical data.
//
// Note internal/migrations/v19.go carries the ORIGINAL body and must stay frozen
// — a historical migration has to keep reproducing the behaviour it shipped.
func WebhookCustomEventsTriggerFunction() string {
	return `CREATE OR REPLACE FUNCTION webhook_custom_events_trigger()
		RETURNS TRIGGER AS $$
		DECLARE
			sub RECORD;
			custom_filters JSONB;
			should_deliver BOOLEAN;
			payload JSONB;
			event_kind VARCHAR(50);
			subscribed_event_type VARCHAR(50);
		BEGIN
			-- Web analytics goals are bridged into custom_events as a first-party
			-- analytics artifact. Fanning them out to third-party webhook
			-- subscribers would ship every pageview-scale conversion — including
			-- client-supplied properties — to endpoints that subscribed to
			-- API-sourced commerce events and never asked for web traffic. A
			-- subscriber who wants them can be given a dedicated event type
			-- later, which is additive; an unannounced firehose is not.
			IF NEW.source = 'web_analytics' THEN
				RETURN NEW;
			END IF;

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

			-- Build payload with full custom_event object
			payload := jsonb_build_object('custom_event', to_jsonb(NEW));

			-- Find matching subscriptions with the correct event type
			FOR sub IN
				SELECT id, settings FROM webhook_subscriptions
				WHERE enabled = true AND subscribed_event_type = ANY(ARRAY(SELECT jsonb_array_elements_text(settings->'event_types')))
			LOOP
				should_deliver := true;
				custom_filters := sub.settings->'custom_event_filters';

				-- jsonb_typeof rather than a key-exists test, for the same reason
				-- the id filters in the other triggers use it: jsonb_array_length
				-- raises on a scalar, and a key-exists test is true for a key
				-- whose value is JSON null. This runs inside the customer's own
				-- write transaction, so a raise here fails their INSERT into
				-- custom_events — their events.track call 500s, not just the
				-- webhook. Reproduced on PostgreSQL 17 with a goal_types of JSON
				-- null: the old shape answered "cannot get array length of a
				-- scalar" and rolled the INSERT back. settings is a free-form
				-- jsonb column, so nothing upstream guarantees the shape.
				IF jsonb_typeof(custom_filters->'goal_types') = 'array'
				   AND jsonb_array_length(custom_filters->'goal_types') > 0 THEN
					IF NEW.goal_type IS NULL OR NOT (NEW.goal_type = ANY(
						SELECT jsonb_array_elements_text(custom_filters->'goal_types')
					)) THEN
						should_deliver := false;
					END IF;
				END IF;

				-- Apply event_names filter if specified
				IF should_deliver AND jsonb_typeof(custom_filters->'event_names') = 'array'
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
}
