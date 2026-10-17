package schema

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// statusWrite is what a new complaint or hard bounce writes on the list stored on the
// message: the contact's existing row is updated, and a contact with no row gets one.
// NOT FOUND also holds for a row whose status the update keeps, and for a row the update
// didn't see because a subscribe or an import committed it after the update began. The
// insert then hits the primary key, and its DO UPDATE applies the status under the same
// guard as the update: the kept row stays as it is, and the new one takes the status.
func statusWrite(status, at, updateGuard, conflictGuard string) string {
	return `UPDATE contact_lists
SET status = '` + status + `',
updated_at = NEW.` + at + `
WHERE email = NEW.contact_email
AND list_id = NEW.list_id
AND ` + updateGuard + `;
IF NOT FOUND THEN
INSERT INTO contact_lists (email, list_id, status, created_at, updated_at)
SELECT NEW.contact_email, NEW.list_id, '` + status + `', NEW.` + at + `, NEW.` + at + `
WHERE EXISTS (SELECT 1 FROM contacts c WHERE c.email = NEW.contact_email)
AND EXISTS (SELECT 1 FROM lists l WHERE l.id = NEW.list_id AND l.deleted_at IS NULL)
ON CONFLICT (email, list_id) DO UPDATE
SET status = EXCLUDED.status, updated_at = EXCLUDED.updated_at
WHERE ` + conflictGuard + `;
END IF;`
}

const (
	newComplaint = "is_complaint := NEW.complained_at IS NOT NULL AND OLD.complained_at IS NULL;"
	newBounce    = "is_bounce := NEW.bounced_at IS NOT NULL AND OLD.bounced_at IS NULL;"
	nothingToDo  = "IF (NOT is_complaint AND NOT is_bounce) OR NEW.list_id IS NULL THEN\nRETURN NEW;\nEND IF;"
)

func TestUpdateContactListsOnStatusChangeFunction_Header(t *testing.T) {
	sql := UpdateContactListsOnStatusChangeFunction()
	assert.True(t, strings.HasPrefix(sql, "CREATE OR REPLACE FUNCTION update_contact_lists_on_status_change()"),
		"the function name is what message_history_status_trigger executes")
	assert.True(t, strings.HasSuffix(sql, "$$ LANGUAGE plpgsql;"))
}

// The function runs after every message_history update, opens and clicks included, so
// it writes nothing unless the update is a new complaint or hard bounce on a message
// that stores a list.
func TestUpdateContactListsOnStatusChangeFunction_ActsOnlyOnANewComplaintOrBounce(t *testing.T) {
	body := normalizeIndent(UpdateContactListsOnStatusChangeFunction())

	for _, part := range []string{newComplaint, newBounce, nothingToDo} {
		assert.Contains(t, body, part)
	}
	guard := strings.Index(body, nothingToDo)
	firstWrite := strings.Index(body, "UPDATE contact_lists")
	require.NotEqual(t, -1, guard)
	require.NotEqual(t, -1, firstWrite)
	assert.Less(t, guard, firstWrite)

	complaint := strings.Index(body, "IF is_complaint THEN")
	bounce := strings.Index(body, "IF is_bounce THEN")
	require.NotEqual(t, -1, complaint)
	require.NotEqual(t, -1, bounce)
	// A complaint and a bounce saved together end complained: the complaint is written
	// first, and the bounce never overwrites it.
	assert.Less(t, complaint, bounce)
}

// Only the list stored on the message counts, as a broadcast and (from 42) an automation
// store it. An automation email sent before automations stored their list has none, and
// the automation's current list is not used instead: it may have changed since the email
// went out, and the complaint would land on a list it wasn't sent for.
func TestUpdateContactListsOnStatusChangeFunction_UsesOnlyTheListStoredOnTheMessage(t *testing.T) {
	sql := UpdateContactListsOnStatusChangeFunction()
	assert.NotContains(t, sql, "automations", "no fallback to the automation's current list")
	assert.NotContains(t, sql, "automation_id")
	assert.Equal(t, 2, strings.Count(sql, "AND list_id = NEW.list_id"),
		"both updates target the stored list")
}

// A complaint or hard bounce must stop the next email whether or not the contact is on
// the list: automations also email contacts with no row on theirs. Like an opt-out from a
// non-member, it records a row on the list stored on the message.
func TestUpdateContactListsOnStatusChangeFunction_RecordsARowForAContactWithNone(t *testing.T) {
	body := normalizeIndent(UpdateContactListsOnStatusChangeFunction())

	cases := []struct {
		name, status, at, updateGuard, conflictGuard string
	}{
		// The worst status: it replaces any other
		{"complaint", "complained", "complained_at", "status != 'complained'", "contact_lists.status <> 'complained'"},
		// It never replaces a complaint
		{"hard bounce", "bounced", "bounced_at", "status NOT IN ('complained', 'bounced')",
			"contact_lists.status NOT IN ('complained', 'bounced')"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, 1, strings.Count(body, statusWrite(tc.status, tc.at, tc.updateGuard, tc.conflictGuard)))
		})
	}

	// The inserts never store an address that is no longer a contact: an erased contact's
	// messages are kept with the address redacted, and a provider can still report on
	// them. Nor do they put anyone on a deleted list.
	assert.Equal(t, 2, strings.Count(body, "WHERE EXISTS (SELECT 1 FROM contacts c WHERE c.email = NEW.contact_email)"))
	assert.Equal(t, 2, strings.Count(body, "AND EXISTS (SELECT 1 FROM lists l WHERE l.id = NEW.list_id AND l.deleted_at IS NULL)"))
}

// A row a subscribe or an import commits after the update began is invisible to the
// update, and DO NOTHING on the insert's conflict would drop the complaint or bounce, so
// the next automation email would go out. The segment queue happens to serialize those
// writes today, so this pins the SQL the inserts use rather than a behaviour a test can
// reach; the inserts must not rely on that side effect.
func TestUpdateContactListsOnStatusChangeFunction_InsertsApplyTheStatusOnConflict(t *testing.T) {
	sql := UpdateContactListsOnStatusChangeFunction()
	assert.NotContains(t, sql, "DO NOTHING")
	assert.Equal(t, 2, strings.Count(sql, "ON CONFLICT (email, list_id) DO UPDATE"),
		"both inserts apply the status to a row they conflict with")
}

// A removed contact's row takes the complaint or bounce and stays removed: a removal is
// not membership, and the automation email gate reads the status of a removed row.
func TestUpdateContactListsOnStatusChangeFunction_NeverRestoresARemovedRow(t *testing.T) {
	sql := UpdateContactListsOnStatusChangeFunction()
	assert.NotContains(t, sql, "deleted_at =", "neither the updates nor the inserts' conflict updates write deleted_at")
	assert.NotContains(t, sql, "SET deleted_at")
	assert.NotContains(t, sql, "EXCLUDED.deleted_at")
	assert.NotContains(t, sql, "AND deleted_at IS NULL", "the update reaches a removed row too")
}
