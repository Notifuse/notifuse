package schema

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
		$$ LANGUAGE plpgsql`
}
