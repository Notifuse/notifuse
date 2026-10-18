package schema

// ApplyMessageStatusToContactListsFunction returns apply_message_status_to_contact_lists(),
// the function that turns a complaint or a hard bounce on a message into the contact's
// status on the list the message was sent for. track_message_history_changes() (see
// TrackMessageHistoryChangesFunction) calls it when an update of message_history records a
// new complaint or a new hard bounce, passing the new complained_at or bounced_at and NULL
// for the other.
//
// Two paths install it: the new-workspace path (internal/database/init.go) and the v42
// migration on every existing workspace. Before 42 the status was set by
// update_contact_lists_on_status_change(), which only updated an existing row;
// internal/migrations/v10.go and v17.go carry its bodies and must stay frozen: a historical
// migration keeps reproducing what it shipped. A workspace upgraded to 42 keeps that
// function and its trigger as they were; see TrackMessageHistoryChangesFunction.
//
// What it does, for a complaint or a hard bounce (the application sets bounced_at only for
// hard bounces):
//   - The list is message_history.list_id, which broadcasts and automations store. A
//     message without one (a transactional email, or an automation email sent before
//     automations stored their list) changes no list status. The automation's current
//     list is deliberately not used: it may have changed since the email went out.
//   - The contact's row on that list takes the status, a removed row included. It stays
//     removed: a removal is not membership, and the automation email gate reads the
//     status of a removed row.
//   - A contact with no row gets one with that status, as an opt-out from a non-member
//     does. Automations email contacts who aren't on their list, and without a row the
//     next email would go out. Nothing is inserted for an address that is no longer a
//     contact (an erased contact's messages are kept with the address redacted) or on a
//     deleted list.
//   - A row the update didn't see, because a subscribe or an import committed it after
//     the update began, takes the status through the insert's ON CONFLICT DO UPDATE,
//     under the same guard as the update; DO NOTHING would drop the complaint. A
//     subscribe or an import still uncommitted on that row makes the update or the
//     insert wait, and the status lands once it commits.
//   - A complaint replaces any status; a bounce never replaces a complaint. Applying the
//     same status twice changes nothing the second time.
func ApplyMessageStatusToContactListsFunction() string {
	return `CREATE OR REPLACE FUNCTION apply_message_status_to_contact_lists(
			p_email VARCHAR, p_list_id VARCHAR, p_complained_at TIMESTAMPTZ, p_bounced_at TIMESTAMPTZ)
		RETURNS VOID AS $$
		BEGIN
			IF p_list_id IS NULL THEN
				RETURN;
			END IF;

			-- Complaint: the worst status, it replaces any other
			IF p_complained_at IS NOT NULL THEN
				UPDATE contact_lists
				SET status = 'complained',
					updated_at = p_complained_at
				WHERE email = p_email
				AND list_id = p_list_id
				AND status != 'complained';
				IF NOT FOUND THEN
					INSERT INTO contact_lists (email, list_id, status, created_at, updated_at)
					SELECT p_email, p_list_id, 'complained', p_complained_at, p_complained_at
					WHERE EXISTS (SELECT 1 FROM contacts c WHERE c.email = p_email)
					AND EXISTS (SELECT 1 FROM lists l WHERE l.id = p_list_id AND l.deleted_at IS NULL)
					ON CONFLICT (email, list_id) DO UPDATE
					SET status = EXCLUDED.status, updated_at = EXCLUDED.updated_at
					WHERE contact_lists.status <> 'complained';
				END IF;
			END IF;

			-- Hard bounce: never replaces a complaint
			IF p_bounced_at IS NOT NULL THEN
				UPDATE contact_lists
				SET status = 'bounced',
					updated_at = p_bounced_at
				WHERE email = p_email
				AND list_id = p_list_id
				AND status NOT IN ('complained', 'bounced');
				IF NOT FOUND THEN
					INSERT INTO contact_lists (email, list_id, status, created_at, updated_at)
					SELECT p_email, p_list_id, 'bounced', p_bounced_at, p_bounced_at
					WHERE EXISTS (SELECT 1 FROM contacts c WHERE c.email = p_email)
					AND EXISTS (SELECT 1 FROM lists l WHERE l.id = p_list_id AND l.deleted_at IS NULL)
					ON CONFLICT (email, list_id) DO UPDATE
					SET status = EXCLUDED.status, updated_at = EXCLUDED.updated_at
					WHERE contact_lists.status NOT IN ('complained', 'bounced');
				END IF;
			END IF;
		END;
		$$ LANGUAGE plpgsql;`
}

// TrackMessageHistoryChangesFunction returns track_message_history_changes(), the function
// message_history_changes_trigger runs after every insert and update of message_history. It
// writes the message's contact_timeline entries (email.sent, email.opened,
// email.complained, …). For an update that records a new complaint or a new hard bounce,
// it first sets the contact's status on the message's list through
// apply_message_status_to_contact_lists().
//
// The status comes first because of locks. Every contact_timeline entry locks the
// contact's contact_segment_queue row, and every write to contact_lists locks the
// contact_lists row first and the contact_segment_queue row second, through its own
// timeline entry. Setting the status before writing any timeline entry takes them in that
// same order, so a complaint or a bounce that meets an uncommitted subscribe or import of
// the same contact waits for it. Before 42 a separate trigger,
// message_history_status_trigger, set the status after this function had written the
// timeline entries: the complaint held the queue row while it waited for the contact_lists
// row, the import held the contact_lists row while it waited for the queue row, and
// PostgreSQL aborted one of the two. An update of several messages in one statement can
// still deadlock with an import of several of their contacts, as any two writes of several
// contact_lists rows in different orders can.
//
// Setting the status first also means that the list.complained or list.bounced entry is
// written before the email.complained or email.bounced one. The conditions of an automation
// triggered by the email.* entry see the new list status, which they did not before 42; in
// exchange, a timeline condition of an automation triggered by the list.* entry no longer
// counts the email.* entry of the same complaint or bounce, which it did. The order the
// timeline shows does not change: it sorts by created_at, which the two entries take from
// different clocks.
//
// A workspace created at 42 or later has no other trigger setting the status. One upgraded
// to 42 keeps message_history_status_trigger, and the body its function has had since v17:
// dropping the trigger would hold an ACCESS EXCLUSIVE lock on message_history until the
// migration commits, blocking every send and every webhook in the meantime. That trigger
// fires after this function's, as PostgreSQL fires the AFTER triggers of one event in name
// order, and its function only updates a row whose status differs: once this function has
// set the status, it changes nothing, unless a row for the contact was committed in between.
// It updates that row as before 42, after the contact_segment_queue row, so it can deadlock
// with another write of the row as before 42. It is deliberately not given the insert of
// apply_message_status_to_contact_lists(): firing after the timeline entries, the insert
// can wait for an uncommitted subscribe while holding the contact_segment_queue row the
// subscribe waits for, when the address became a contact while the complaint ran. An update
// that ran this function's body from before 42, around the v42 commit, sets the status as
// before 42.
//
// Two paths install it: the new-workspace path (internal/database/init.go) and the v42
// migration. internal/migrations/v7.go, v8.go and v36.go carry the bodies this one
// replaced and stay frozen. Apart from the status step, this body is v36's.
func TrackMessageHistoryChangesFunction() string {
	return `CREATE OR REPLACE FUNCTION track_message_history_changes()
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
				VALUES (NEW.contact_email, op, 'message_history', NEW.channel || '.sent', NEW.id, changes_json, NEW.updated_at);
			ELSIF TG_OP = 'UPDATE' THEN
				op := 'update';
				-- A new complaint or hard bounce sets the list status before any timeline entry
				-- is written: the contact_lists row is locked before the contact_segment_queue row,
				-- in the order every write to contact_lists takes them.
				IF NEW.list_id IS NOT NULL AND ((NEW.complained_at IS NOT NULL AND OLD.complained_at IS NULL) OR (NEW.bounced_at IS NOT NULL AND OLD.bounced_at IS NULL)) THEN
					PERFORM apply_message_status_to_contact_lists(NEW.contact_email, NEW.list_id,
						CASE WHEN NEW.complained_at IS NOT NULL AND OLD.complained_at IS NULL THEN NEW.complained_at END,
						CASE WHEN NEW.bounced_at IS NOT NULL AND OLD.bounced_at IS NULL THEN NEW.bounced_at END);
				END IF;
				-- Handle engagement events separately with specific kinds
				IF OLD.opened_at IS DISTINCT FROM NEW.opened_at AND NEW.opened_at IS NOT NULL THEN
					changes_json := jsonb_build_object('opened_at', jsonb_build_object('old', OLD.opened_at, 'new', NEW.opened_at));
					INSERT INTO contact_timeline (email, operation, entity_type, kind, entity_id, changes, created_at)
					VALUES (NEW.contact_email, op, 'message_history', NEW.channel || '.opened', NEW.id, changes_json, NEW.updated_at);
				END IF;
				IF OLD.clicked_at IS DISTINCT FROM NEW.clicked_at AND NEW.clicked_at IS NOT NULL THEN
					changes_json := jsonb_build_object('clicked_at', jsonb_build_object('old', OLD.clicked_at, 'new', NEW.clicked_at));
					INSERT INTO contact_timeline (email, operation, entity_type, kind, entity_id, changes, created_at)
					VALUES (NEW.contact_email, op, 'message_history', NEW.channel || '.clicked', NEW.id, changes_json, NEW.updated_at);
				END IF;
				IF OLD.bounced_at IS DISTINCT FROM NEW.bounced_at AND NEW.bounced_at IS NOT NULL THEN
					changes_json := jsonb_build_object('bounced_at', jsonb_build_object('old', OLD.bounced_at, 'new', NEW.bounced_at));
					INSERT INTO contact_timeline (email, operation, entity_type, kind, entity_id, changes, created_at)
					VALUES (NEW.contact_email, op, 'message_history', NEW.channel || '.bounced', NEW.id, changes_json, NEW.updated_at);
				END IF;
				IF OLD.complained_at IS DISTINCT FROM NEW.complained_at AND NEW.complained_at IS NOT NULL THEN
					changes_json := jsonb_build_object('complained_at', jsonb_build_object('old', OLD.complained_at, 'new', NEW.complained_at));
					INSERT INTO contact_timeline (email, operation, entity_type, kind, entity_id, changes, created_at)
					VALUES (NEW.contact_email, op, 'message_history', NEW.channel || '.complained', NEW.id, changes_json, NEW.updated_at);
				END IF;
				IF OLD.unsubscribed_at IS DISTINCT FROM NEW.unsubscribed_at AND NEW.unsubscribed_at IS NOT NULL THEN
					changes_json := jsonb_build_object('unsubscribed_at', jsonb_build_object('old', OLD.unsubscribed_at, 'new', NEW.unsubscribed_at));
					INSERT INTO contact_timeline (email, operation, entity_type, kind, entity_id, changes, created_at)
					VALUES (NEW.contact_email, op, 'message_history', NEW.channel || '.unsubscribed', NEW.id, changes_json, NEW.updated_at);
				END IF;
				-- Handle other updates (delivered, failed, status_info) as generic updates
				changes_json := '{}'::jsonb;
				IF OLD.delivered_at IS DISTINCT FROM NEW.delivered_at THEN changes_json := changes_json || jsonb_build_object('delivered_at', jsonb_build_object('old', OLD.delivered_at, 'new', NEW.delivered_at)); END IF;
				IF OLD.failed_at IS DISTINCT FROM NEW.failed_at THEN changes_json := changes_json || jsonb_build_object('failed_at', jsonb_build_object('old', OLD.failed_at, 'new', NEW.failed_at)); END IF;
				IF OLD.status_info IS DISTINCT FROM NEW.status_info THEN changes_json := changes_json || jsonb_build_object('status_info', jsonb_build_object('old', OLD.status_info, 'new', NEW.status_info)); END IF;
				IF changes_json != '{}'::jsonb THEN
					INSERT INTO contact_timeline (email, operation, entity_type, kind, entity_id, changes, created_at)
					VALUES (NEW.contact_email, op, 'message_history', NEW.channel || '.updated', NEW.id, changes_json, NEW.updated_at);
				END IF;
			END IF;
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;`
}
