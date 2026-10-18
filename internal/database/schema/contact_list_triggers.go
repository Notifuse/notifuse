package schema

// TrackContactListChangesFunction returns the trigger function that writes a
// contact_timeline row for every membership change on contact_lists.
//
// It lives here rather than inline in the workspace initializer because two
// paths install it: the new-workspace path (internal/database/init.go) and the
// v42 migration on every existing workspace. A body that drifts between the two
// would record different timeline events for the same write.
//
// internal/migrations/v23.go carries the body this one replaced and must stay
// frozen: a historical migration keeps reproducing what it shipped.
//
// The branches:
//   - INSERT: the kind matching the initial status (list.subscribed, list.pending, …);
//   - a write to a row that is removed before and after: nothing, as the webhook
//     function reports nothing. A removed contact is not on the list, so neither
//     the status a bounce, complaint or unsubscribe leaves on their row for the
//     email gate, nor removing them again, is a change of membership. Removing
//     them again used to record list.removed again, which re-enrolled them in
//     automations triggered by it;
//   - removal (deleted_at set on a row that had none): list.removed;
//   - status change: the transition kind (list.confirmed, list.resubscribed, …),
//     including on a re-add that also changes the status;
//   - re-add with the same status (deleted_at cleared): exactly what an INSERT of
//     that row records, operation 'insert' included. Without it a re-added
//     contact wrote no timeline row, so the segment queue never re-evaluated them
//     and they stayed out of every "in list" segment;
//   - anything else writes nothing.
func TrackContactListChangesFunction() string {
	return `CREATE OR REPLACE FUNCTION track_contact_list_changes()
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

				-- A contact removed from the list is not on it: a write to their row, such as
				-- the status a bounce, complaint or unsubscribe leaves for the email gate, or
				-- removing them again, records nothing
				IF OLD.deleted_at IS NOT NULL AND NEW.deleted_at IS NOT NULL THEN
					RETURN NEW;
				END IF;

				-- Handle a removal
				IF OLD.deleted_at IS NULL AND NEW.deleted_at IS NOT NULL THEN
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

				-- Handle a removed contact added back with the same status: record it
				-- as the insert it stands for
				ELSIF OLD.deleted_at IS NOT NULL AND NEW.deleted_at IS NULL THEN
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
				ELSE
					RETURN NEW;
				END IF;
			END IF;

			INSERT INTO contact_timeline (email, operation, entity_type, kind, entity_id, changes, created_at)
			VALUES (NEW.email, op, 'contact_list', kind_value, NEW.list_id, changes_json, CURRENT_TIMESTAMP);

			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;`
}
