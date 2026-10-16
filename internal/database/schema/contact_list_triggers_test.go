package schema

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// insertKindCase is the status-to-kind mapping a first subscription records.
const insertKindCase = `kind_value := CASE NEW.status
					WHEN 'active' THEN 'list.subscribed'
					WHEN 'pending' THEN 'list.pending'
					WHEN 'unsubscribed' THEN 'list.unsubscribed'
					WHEN 'bounced' THEN 'list.bounced'
					WHEN 'complained' THEN 'list.complained'
					ELSE 'list.subscribed'
				END;`

// insertChanges is the changes payload a first subscription records.
const insertChanges = `changes_json := jsonb_build_object(
					'list_id', jsonb_build_object('new', NEW.list_id),
					'status', jsonb_build_object('new', NEW.status)
				);`

// normalizeIndent strips leading whitespace from every line, so a block can be
// compared wherever it is nested.
func normalizeIndent(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSpace(line)
	}
	return strings.Join(lines, "\n")
}

func TestTrackContactListChangesFunction_KeepsTheKindsItShipped(t *testing.T) {
	sql := TrackContactListChangesFunction()

	assert.True(t, strings.HasPrefix(sql, "CREATE OR REPLACE FUNCTION track_contact_list_changes()"),
		"the function name is what contact_list_changes_trigger executes")

	// INSERT: initial status → kind, with the insert's changes.
	assert.Contains(t, sql, "IF TG_OP = 'INSERT' THEN\n\t\t\t\top := 'insert';")
	assert.Contains(t, sql, insertKindCase)
	assert.Contains(t, sql, insertChanges)

	// Removal.
	assert.Contains(t, sql, "IF OLD.deleted_at IS DISTINCT FROM NEW.deleted_at AND NEW.deleted_at IS NOT NULL THEN")
	assert.Contains(t, sql, "kind_value := 'list.removed';")

	// Status transitions keep their own kinds.
	for _, line := range []string{
		"WHEN OLD.status = 'pending' AND NEW.status = 'active' THEN 'list.confirmed'",
		"WHEN OLD.status IN ('unsubscribed', 'bounced', 'complained') AND NEW.status = 'active' THEN 'list.resubscribed'",
		"WHEN NEW.status = 'unsubscribed' THEN 'list.unsubscribed'",
		"WHEN NEW.status = 'bounced' THEN 'list.bounced'",
		"WHEN NEW.status = 'complained' THEN 'list.complained'",
		"WHEN NEW.status = 'pending' THEN 'list.pending'",
		"WHEN NEW.status = 'active' THEN 'list.subscribed'",
		"ELSE 'list.status_changed'",
	} {
		assert.Contains(t, sql, line)
	}

	assert.Contains(t, sql, "INSERT INTO contact_timeline (email, operation, entity_type, kind, entity_id, changes, created_at)")
	assert.Contains(t, sql, "VALUES (NEW.email, op, 'contact_list', kind_value, NEW.list_id, changes_json, CURRENT_TIMESTAMP);")
}

// A removed contact added back with the same status must record what a first
// subscription records: the timeline row re-queues them for segment
// recomputation, and timeline conditions on changes match it like an insert.
func TestTrackContactListChangesFunction_ReAddRecordsAnInsert(t *testing.T) {
	sql := TrackContactListChangesFunction()

	const restoreGuard = "ELSIF OLD.deleted_at IS NOT NULL AND NEW.deleted_at IS NULL THEN"
	removal := strings.Index(sql, "kind_value := 'list.removed';")
	transition := strings.Index(sql, "ELSIF OLD.status IS DISTINCT FROM NEW.status THEN")
	restore := strings.Index(sql, restoreGuard)
	elseBranch := strings.LastIndex(sql, "ELSE\n\t\t\t\t\tRETURN NEW;")
	require.NotEqual(t, -1, removal)
	require.NotEqual(t, -1, transition)
	require.NotEqual(t, -1, restore, "the re-add branch must exist")
	require.NotEqual(t, -1, elseBranch)
	assert.Equal(t, 1, strings.Count(sql, restoreGuard))

	// A re-add that also changes status is a transition, so the re-add branch
	// comes after the transition branch, and before the catch-all.
	assert.Less(t, removal, transition)
	assert.Less(t, transition, restore)
	assert.Less(t, restore, elseBranch)

	branch := normalizeIndent(sql[restore:elseBranch])
	assert.Contains(t, branch, "op := 'insert';", "a re-add is recorded with the insert operation")
	assert.Contains(t, branch, normalizeIndent(insertKindCase), "a re-add maps every status as the insert branch does")
	assert.Contains(t, branch, normalizeIndent(insertChanges), "a re-add writes the insert's changes")

	// The insert mapping appears in the INSERT branch and in the re-add branch, nowhere else.
	assert.Equal(t, 2, strings.Count(normalizeIndent(sql), normalizeIndent(insertKindCase)))
}
