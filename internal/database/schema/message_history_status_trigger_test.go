package schema

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// statusWrite is what a new complaint or hard bounce writes on the list the message was
// sent for: the contact's existing row is updated, and a contact with no row gets one.
// An already-complained row is not updated either, so NOT FOUND holds for it too; the
// insert then hits the primary key and DO NOTHING keeps the row as it is.
func statusWrite(status, at, updateGuard string) string {
	return `UPDATE contact_lists
SET status = '` + status + `',
updated_at = NEW.` + at + `
WHERE email = NEW.contact_email
AND list_id = target_list_id
AND ` + updateGuard + `;
IF NOT FOUND THEN
INSERT INTO contact_lists (email, list_id, status, created_at, updated_at)
SELECT NEW.contact_email, target_list_id, '` + status + `', NEW.` + at + `, NEW.` + at + `
WHERE EXISTS (SELECT 1 FROM contacts c WHERE c.email = NEW.contact_email)
AND EXISTS (SELECT 1 FROM lists l WHERE l.id = target_list_id AND l.deleted_at IS NULL)
ON CONFLICT (email, list_id) DO NOTHING;
END IF;`
}

const (
	newComplaint = "is_complaint := NEW.complained_at IS NOT NULL AND OLD.complained_at IS NULL;"
	newBounce    = "is_bounce := NEW.bounced_at IS NOT NULL AND OLD.bounced_at IS NULL;"
	nothingToDo  = "IF NOT is_complaint AND NOT is_bounce THEN\nRETURN NEW;\nEND IF;"
	storedList   = "target_list_id := NEW.list_id;"
	// Automation messages are stored without a list; they are sent for the automation's.
	automationList = "IF target_list_id IS NULL AND NEW.automation_id IS NOT NULL THEN\n" +
		"SELECT a.list_id INTO target_list_id FROM automations a WHERE a.id = NEW.automation_id;\n" +
		"END IF;"
	noList = "IF target_list_id IS NULL OR target_list_id = '' THEN\nRETURN NEW;\nEND IF;"
)

func TestUpdateContactListsOnStatusChangeFunction_Header(t *testing.T) {
	sql := UpdateContactListsOnStatusChangeFunction()
	assert.True(t, strings.HasPrefix(sql, "CREATE OR REPLACE FUNCTION update_contact_lists_on_status_change()"),
		"the function name is what message_history_status_trigger executes")
	assert.True(t, strings.HasSuffix(sql, "$$ LANGUAGE plpgsql;"))
}

// The function runs after every message_history update, opens and clicks included, so
// it decides first whether anything changed, and only then looks up an automation.
func TestUpdateContactListsOnStatusChangeFunction_ActsOnlyOnANewComplaintOrBounce(t *testing.T) {
	body := normalizeIndent(UpdateContactListsOnStatusChangeFunction())

	for _, part := range []string{newComplaint, newBounce, nothingToDo} {
		assert.Contains(t, body, part)
	}
	guard := strings.Index(body, nothingToDo)
	lookup := strings.Index(body, automationList)
	require.NotEqual(t, -1, guard)
	require.NotEqual(t, -1, lookup)
	assert.Less(t, guard, lookup, "the automation lookup runs only for a new complaint or bounce")

	complaint := strings.Index(body, "IF is_complaint THEN")
	bounce := strings.Index(body, "IF is_bounce THEN")
	require.NotEqual(t, -1, complaint)
	require.NotEqual(t, -1, bounce)
	// A complaint and a bounce saved together end complained: the complaint is written
	// first, and the bounce never overwrites it.
	assert.Less(t, complaint, bounce)
}

// The list the message was sent for is the one stored on it, as a broadcast stores it.
// An automation message stored without one was sent for the automation's list, the
// same fallback MessageHistoryRepository.WasSentForList reads for an opt-out.
func TestUpdateContactListsOnStatusChangeFunction_ResolvesTheListTheMessageWasSentFor(t *testing.T) {
	body := normalizeIndent(UpdateContactListsOnStatusChangeFunction())

	stored := strings.Index(body, storedList)
	fallback := strings.Index(body, automationList)
	none := strings.Index(body, noList)
	firstWrite := strings.Index(body, "UPDATE contact_lists")
	require.NotEqual(t, -1, stored)
	require.NotEqual(t, -1, fallback)
	require.NotEqual(t, -1, none)
	require.NotEqual(t, -1, firstWrite)
	assert.Less(t, stored, fallback, "the stored list wins; the automation's is read only without one")
	assert.Less(t, fallback, none)
	assert.Less(t, none, firstWrite, "a message sent for no list writes nothing")

	assert.NotContains(t, body, "list_id = NEW.list_id",
		"every write targets the resolved list, not the stored column")
}

// A complaint or hard bounce must stop the next email whether or not the contact is on
// the list: automations also email contacts with no row on theirs. Like an opt-out from a
// non-member, it records a row on the list the message was sent for.
func TestUpdateContactListsOnStatusChangeFunction_RecordsARowForAContactWithNone(t *testing.T) {
	body := normalizeIndent(UpdateContactListsOnStatusChangeFunction())

	cases := []struct {
		name, status, at, updateGuard string
	}{
		// The worst status: it replaces any other
		{"complaint", "complained", "complained_at", "status != 'complained'"},
		// It never replaces a complaint
		{"hard bounce", "bounced", "bounced_at", "status NOT IN ('complained', 'bounced')"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, 1, strings.Count(body, statusWrite(tc.status, tc.at, tc.updateGuard)))
		})
	}

	// The inserts never store an address that is no longer a contact: an erased contact's
	// messages are kept with the address redacted, and a provider can still report on
	// them. Nor do they put anyone on a deleted list.
	assert.Equal(t, 2, strings.Count(body, "WHERE EXISTS (SELECT 1 FROM contacts c WHERE c.email = NEW.contact_email)"))
	assert.Equal(t, 2, strings.Count(body, "AND EXISTS (SELECT 1 FROM lists l WHERE l.id = target_list_id AND l.deleted_at IS NULL)"))
}

// A removed contact's row takes the complaint or bounce and stays removed: a removal is
// not membership, and the automation email gate reads the status of a removed row.
func TestUpdateContactListsOnStatusChangeFunction_NeverRestoresARemovedRow(t *testing.T) {
	sql := UpdateContactListsOnStatusChangeFunction()
	assert.NotContains(t, sql, "deleted_at = NULL")
	assert.NotContains(t, sql, "SET deleted_at")
	assert.NotContains(t, sql, "cl.deleted_at", "the update reaches a removed row too")
}
