package schema

// UpdateContactListsOnStatusChangeFunction returns the trigger function that turns a
// complaint or a hard bounce on a message into the contact's status on the list the
// message was sent for. message_history_status_trigger runs it after every update of
// message_history.
//
// Two paths install it: the new-workspace path (internal/database/init.go) and the v42
// migration on every existing workspace. internal/migrations/v10.go and v17.go carry
// the bodies this one replaced and must stay frozen: a historical migration keeps
// reproducing what it shipped.
//
// What it does, for a new complaint (complained_at set) or a new hard bounce (bounced_at
// set; the application sets it only for hard bounces):
//   - The list is the one stored on the message, as broadcasts store it. An automation
//     message stored without one was sent for the automation's list, the fallback
//     MessageHistoryRepository.WasSentForList also reads.
//   - The contact's row on that list takes the status, a removed row included. It stays
//     removed: a removal is not membership, and the automation email gate reads the
//     status of a removed row.
//   - A contact with no row gets one with that status, as an opt-out from a non-member
//     does. Automations email contacts who aren't on their list, and without a row the
//     next email would go out. Nothing is inserted for an address that is no longer a
//     contact (an erased contact's messages are kept with the address redacted) or on a
//     deleted list.
//   - A complaint replaces any status; a bounce never replaces a complaint.
func UpdateContactListsOnStatusChangeFunction() string {
	return `CREATE OR REPLACE FUNCTION update_contact_lists_on_status_change()
		RETURNS TRIGGER AS $$
		DECLARE
			is_complaint BOOLEAN;
			is_bounce BOOLEAN;
			target_list_id TEXT;
		BEGIN
			is_complaint := NEW.complained_at IS NOT NULL AND OLD.complained_at IS NULL;
			is_bounce := NEW.bounced_at IS NOT NULL AND OLD.bounced_at IS NULL;
			IF NOT is_complaint AND NOT is_bounce THEN
				RETURN NEW;
			END IF;

			-- The list the message was sent for
			target_list_id := NEW.list_id;
			IF target_list_id IS NULL AND NEW.automation_id IS NOT NULL THEN
				SELECT a.list_id INTO target_list_id FROM automations a WHERE a.id = NEW.automation_id;
			END IF;
			IF target_list_id IS NULL OR target_list_id = '' THEN
				RETURN NEW;
			END IF;

			-- Complaint: the worst status, it replaces any other
			IF is_complaint THEN
				UPDATE contact_lists
				SET status = 'complained',
					updated_at = NEW.complained_at
				WHERE email = NEW.contact_email
				AND list_id = target_list_id
				AND status != 'complained';
				IF NOT FOUND THEN
					INSERT INTO contact_lists (email, list_id, status, created_at, updated_at)
					SELECT NEW.contact_email, target_list_id, 'complained', NEW.complained_at, NEW.complained_at
					WHERE EXISTS (SELECT 1 FROM contacts c WHERE c.email = NEW.contact_email)
					AND EXISTS (SELECT 1 FROM lists l WHERE l.id = target_list_id AND l.deleted_at IS NULL)
					ON CONFLICT (email, list_id) DO NOTHING;
				END IF;
			END IF;

			-- Hard bounce: never replaces a complaint
			IF is_bounce THEN
				UPDATE contact_lists
				SET status = 'bounced',
					updated_at = NEW.bounced_at
				WHERE email = NEW.contact_email
				AND list_id = target_list_id
				AND status NOT IN ('complained', 'bounced');
				IF NOT FOUND THEN
					INSERT INTO contact_lists (email, list_id, status, created_at, updated_at)
					SELECT NEW.contact_email, target_list_id, 'bounced', NEW.bounced_at, NEW.bounced_at
					WHERE EXISTS (SELECT 1 FROM contacts c WHERE c.email = NEW.contact_email)
					AND EXISTS (SELECT 1 FROM lists l WHERE l.id = target_list_id AND l.deleted_at IS NULL)
					ON CONFLICT (email, list_id) DO NOTHING;
				END IF;
			END IF;

			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;`
}
