package schema

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// statusWrite is what a complaint or hard bounce writes on the list stored on the
// message: the contact's existing row is updated, and a contact with no row gets one.
// NOT FOUND also holds for a row whose status the update keeps, and for a row the update
// didn't see because a subscribe or an import committed it after the update began. The
// insert then hits the primary key, and its DO UPDATE applies the status under the same
// guard as the update: the kept row stays as it is, and the new one takes the status.
func statusWrite(status, at, updateGuard, conflictGuard string) string {
	return `UPDATE contact_lists
SET status = '` + status + `',
updated_at = ` + at + `
WHERE email = p_email
AND list_id = p_list_id
AND ` + updateGuard + `;
IF NOT FOUND THEN
INSERT INTO contact_lists (email, list_id, status, created_at, updated_at)
SELECT p_email, p_list_id, '` + status + `', ` + at + `, ` + at + `
WHERE EXISTS (SELECT 1 FROM contacts c WHERE c.email = p_email)
AND EXISTS (SELECT 1 FROM lists l WHERE l.id = p_list_id AND l.deleted_at IS NULL)
ON CONFLICT (email, list_id) DO UPDATE
SET status = EXCLUDED.status, updated_at = EXCLUDED.updated_at
WHERE ` + conflictGuard + `;
END IF;`
}

// statusStep is how the timeline function calls the shared one: only for a new complaint
// or a new hard bounce on a message that stores a list, passing the new timestamp and NULL
// for the other. Opens, clicks and deliveries call nothing.
const statusStep = `IF NEW.list_id IS NOT NULL AND ((NEW.complained_at IS NOT NULL AND OLD.complained_at IS NULL) OR (NEW.bounced_at IS NOT NULL AND OLD.bounced_at IS NULL)) THEN
PERFORM apply_message_status_to_contact_lists(NEW.contact_email, NEW.list_id,
CASE WHEN NEW.complained_at IS NOT NULL AND OLD.complained_at IS NULL THEN NEW.complained_at END,
CASE WHEN NEW.bounced_at IS NOT NULL AND OLD.bounced_at IS NULL THEN NEW.bounced_at END);
END IF;`

// The signature is what track_message_history_changes() calls: a changed one would create
// a second function next to the first, which it would keep calling.
func TestApplyMessageStatusToContactListsFunction_Header(t *testing.T) {
	body := normalizeIndent(ApplyMessageStatusToContactListsFunction())
	assert.True(t, strings.HasPrefix(body, `CREATE OR REPLACE FUNCTION apply_message_status_to_contact_lists(
p_email VARCHAR, p_list_id VARCHAR, p_complained_at TIMESTAMPTZ, p_bounced_at TIMESTAMPTZ)
RETURNS VOID AS $$`))
	assert.True(t, strings.HasSuffix(body, "$$ LANGUAGE plpgsql;"))
}

// A message that stores no list changes no list status, and a complaint and a bounce
// passed together end complained: the complaint is written first, and the bounce never
// overwrites it.
func TestApplyMessageStatusToContactListsFunction_WritesNothingWithoutAList(t *testing.T) {
	body := normalizeIndent(ApplyMessageStatusToContactListsFunction())

	guard := strings.Index(body, "IF p_list_id IS NULL THEN\nRETURN;\nEND IF;")
	firstWrite := strings.Index(body, "UPDATE contact_lists")
	require.NotEqual(t, -1, guard)
	require.NotEqual(t, -1, firstWrite)
	assert.Less(t, guard, firstWrite)

	complaint := strings.Index(body, "IF p_complained_at IS NOT NULL THEN")
	bounce := strings.Index(body, "IF p_bounced_at IS NOT NULL THEN")
	require.NotEqual(t, -1, complaint)
	require.NotEqual(t, -1, bounce)
	assert.Less(t, complaint, bounce)
}

// Only the list stored on the message counts, as a broadcast and (from 42) an automation
// store it. An automation email sent before automations stored their list has none, and
// the automation's current list is not used instead: it may have changed since the email
// went out, and the complaint would land on a list it wasn't sent for.
func TestApplyMessageStatusToContactListsFunction_UsesOnlyTheListStoredOnTheMessage(t *testing.T) {
	sql := ApplyMessageStatusToContactListsFunction()
	assert.NotContains(t, sql, "automations", "no fallback to the automation's current list")
	assert.NotContains(t, sql, "automation_id")
	assert.Equal(t, 2, strings.Count(sql, "AND list_id = p_list_id"),
		"both updates target the stored list")
}

// A complaint or hard bounce must stop the next email whether or not the contact is on
// the list: automations also email contacts with no row on theirs. Like an opt-out from a
// non-member, it records a row on the list stored on the message.
func TestApplyMessageStatusToContactListsFunction_RecordsARowForAContactWithNone(t *testing.T) {
	body := normalizeIndent(ApplyMessageStatusToContactListsFunction())

	cases := []struct {
		name, status, at, updateGuard, conflictGuard string
	}{
		// The worst status: it replaces any other
		{"complaint", "complained", "p_complained_at", "status != 'complained'", "contact_lists.status <> 'complained'"},
		// It never replaces a complaint
		{"hard bounce", "bounced", "p_bounced_at", "status NOT IN ('complained', 'bounced')",
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
	assert.Equal(t, 2, strings.Count(body, "WHERE EXISTS (SELECT 1 FROM contacts c WHERE c.email = p_email)"))
	assert.Equal(t, 2, strings.Count(body, "AND EXISTS (SELECT 1 FROM lists l WHERE l.id = p_list_id AND l.deleted_at IS NULL)"))
}

// A row a subscribe or an import commits after the update began is invisible to the
// update, and DO NOTHING on the insert's conflict would drop the complaint or bounce, so
// the next automation email would go out. TestMessageStatusRecordsTheListRow
// (tests/integration) races a complaint with an import that inserts the row; this pins
// the conflict clause of the bounce's insert too.
func TestApplyMessageStatusToContactListsFunction_InsertsApplyTheStatusOnConflict(t *testing.T) {
	sql := ApplyMessageStatusToContactListsFunction()
	assert.NotContains(t, sql, "DO NOTHING")
	assert.Equal(t, 2, strings.Count(sql, "ON CONFLICT (email, list_id) DO UPDATE"),
		"both inserts apply the status to a row they conflict with")
}

// A removed contact's row takes the complaint or bounce and stays removed: a removal is
// not membership, and the automation email gate reads the status of a removed row.
func TestApplyMessageStatusToContactListsFunction_NeverRestoresARemovedRow(t *testing.T) {
	sql := ApplyMessageStatusToContactListsFunction()
	assert.NotContains(t, sql, "deleted_at =", "neither the updates nor the inserts' conflict updates write deleted_at")
	assert.NotContains(t, sql, "SET deleted_at")
	assert.NotContains(t, sql, "EXCLUDED.deleted_at")
	assert.NotContains(t, sql, "AND deleted_at IS NULL", "the update reaches a removed row too")
}

// The timeline function sets the list status on an update, before the first timeline
// entry of that branch: the entry locks the contact's contact_segment_queue row, and a
// complaint that locked it before the contact_lists row deadlocked with an import of the
// same contact. An insert of message_history (a send) sets no status.
func TestTrackMessageHistoryChangesFunction_SetsTheListStatusBeforeTheTimeline(t *testing.T) {
	body := normalizeIndent(TrackMessageHistoryChangesFunction())
	require.True(t, strings.HasPrefix(body, "CREATE OR REPLACE FUNCTION track_message_history_changes()"),
		"the function name is what message_history_changes_trigger executes")
	assert.True(t, strings.HasSuffix(body, "$$ LANGUAGE plpgsql;"))

	require.Equal(t, 1, strings.Count(body, statusStep))
	step := strings.Index(body, statusStep)
	updateBranch := strings.Index(body, "ELSIF TG_OP = 'UPDATE' THEN")
	require.NotEqual(t, -1, updateBranch)
	firstEntryOfTheUpdate := updateBranch + strings.Index(body[updateBranch:], "INSERT INTO contact_timeline")
	assert.Less(t, updateBranch, step, "a send sets no status")
	assert.Less(t, step, firstEntryOfTheUpdate)
}
