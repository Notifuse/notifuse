package schema

import (
	"fmt"
	"strings"
)

// TrackedContactColumns are the contacts columns whose change writes a
// contact.updated row to contact_timeline, with the old and new value.
//
// tracking_consent_updated_at is deliberately absent: it moves only with a
// consent, which is already recorded, and alone it is no change to report.
var TrackedContactColumns = []string{
	"external_id", "timezone", "language", "first_name", "last_name", "full_name",
	"phone", "address_line_1", "address_line_2", "country", "postcode", "state", "job_title",
	"custom_string_1", "custom_string_2", "custom_string_3", "custom_string_4", "custom_string_5",
	"custom_number_1", "custom_number_2", "custom_number_3", "custom_number_4", "custom_number_5",
	"custom_datetime_1", "custom_datetime_2", "custom_datetime_3", "custom_datetime_4", "custom_datetime_5",
	"custom_json_1", "custom_json_2", "custom_json_3", "custom_json_4", "custom_json_5",
	"tracking_opens_consent", "tracking_clicks_consent", "tracking_consent_source",
}

// TrackContactChangesFunction returns the trigger function that writes a
// contact_timeline row when a contact is created or one of
// TrackedContactColumns changes.
//
// It lives here because two paths install it: the new-workspace path
// (internal/database/init.go) and the v43 migration on every existing
// workspace. internal/migrations/v23.go carries the body this one replaced and
// must stay frozen.
func TrackContactChangesFunction() string {
	var changes strings.Builder
	for _, column := range TrackedContactColumns {
		fmt.Fprintf(&changes,
			"\t\t\t\tIF OLD.%[1]s IS DISTINCT FROM NEW.%[1]s THEN changes_json := changes_json || jsonb_build_object('%[1]s', jsonb_build_object('old', OLD.%[1]s, 'new', NEW.%[1]s)); END IF;\n",
			column)
	}

	return `CREATE OR REPLACE FUNCTION track_contact_changes()
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
` + changes.String() + `				IF changes_json = '{}'::jsonb THEN RETURN NEW; END IF;
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
		$$ LANGUAGE plpgsql;`
}
