package migrations

// v39WebhookContactsTriggerFunction is webhook_contacts_trigger() as v39 shipped it,
// byte for byte the body every release from 39.0 to 42.2 installed. It is frozen
// here because v39 runs on databases below 39, which lack columns the shared
// generator in internal/database/schema compares later on: 43.0 added the tracking
// consent columns to that comparison, and a PL/pgSQL body naming a missing column
// installs without complaint and then fails every UPDATE on contacts. v43 adds the
// columns and installs the generator's body. Never update this to follow the
// generator.
const v39WebhookContactsTriggerFunction = `CREATE OR REPLACE FUNCTION webhook_contacts_trigger()
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
