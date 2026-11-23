package schema

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContactOptOutStatements(t *testing.T) {
	statements := ContactOptOutStatements()
	all := strings.Join(statements, "\n")

	// The key, generated in SQL without an extension, once per workspace.
	assert.Contains(t, all, "CREATE TABLE IF NOT EXISTS contact_opt_out_hash_key")
	assert.Contains(t, all, "uuid_send(gen_random_uuid()) || uuid_send(gen_random_uuid())")
	assert.Contains(t, all, "ON CONFLICT (id) DO NOTHING")
	assert.NotContains(t, all, "pgcrypto")

	// The hashes hold no address, only the three opt-out statuses.
	assert.Contains(t, all, "CREATE TABLE IF NOT EXISTS contact_opt_out_hashes")
	assert.Contains(t, all, "CHECK (status IN ('unsubscribed', 'bounced', 'complained'))")
	assert.Contains(t, all, "PRIMARY KEY (email_hash, list_id)")
	assert.Contains(t, all, "idx_contact_opt_out_hashes_recorded_at")

	// Every statement can run again.
	for _, statement := range statements {
		assert.Regexp(t, `IF NOT EXISTS|ON CONFLICT|CREATE OR REPLACE|DROP TRIGGER IF EXISTS|CREATE TRIGGER`, statement)
	}
	// The trigger is created after the functions it calls, and replaced rather than added twice.
	last := statements[len(statements)-1]
	require.Contains(t, last, "CREATE TRIGGER contact_opt_out_restore_trigger AFTER INSERT ON contacts")
	assert.Contains(t, statements[len(statements)-2], "DROP TRIGGER IF EXISTS contact_opt_out_restore_trigger ON contacts")
}

func TestContactOptOutHashFunction(t *testing.T) {
	fn := ContactOptOutHashFunction()
	// The same normalisation on every write and lookup, independent of the collation.
	assert.Contains(t, fn, `sha256(k || convert_to(lower(btrim(p_email) COLLATE "C"), 'UTF8'))`)
	// A missing key raises instead of returning NULL, which would match nothing.
	assert.Contains(t, fn, "RAISE EXCEPTION")
}

func TestRestoreContactOptOutsFunction(t *testing.T) {
	fn := RestoreContactOptOutsFunction()
	// Only lists that still exist, and a row already there wins.
	assert.Contains(t, fn, "JOIN lists l ON l.id = o.list_id AND l.deleted_at IS NULL")
	assert.Contains(t, fn, "ON CONFLICT (email, list_id) DO NOTHING")
	// A contact removed from the list comes back removed.
	assert.Contains(t, fn, "CASE WHEN o.removed THEN NOW() END")
	// The setting is on around the insert only, and the hashes go once restored.
	on := strings.Index(fn, "set_config('"+OptOutRestoreGUC+"', 'on', true)")
	insert := strings.Index(fn, "INSERT INTO contact_lists")
	off := strings.Index(fn, "set_config('"+OptOutRestoreGUC+"', 'off', true)")
	del := strings.Index(fn, "DELETE FROM contact_opt_out_hashes WHERE email_hash = h")
	require.True(t, on >= 0 && insert >= 0 && off >= 0 && del >= 0)
	assert.True(t, on < insert && insert < off && off < del)
}

// The two contact_lists functions honour the restore setting: the timeline records the
// restored row under its own kind, and no webhook is sent.
func TestContactListFunctionsHonourTheRestoreSetting(t *testing.T) {
	guard := "IF current_setting('" + OptOutRestoreGUC + "', true) = 'on' THEN"

	timeline := TrackContactListChangesFunction()
	require.Contains(t, timeline, guard)
	branch := timeline[strings.Index(timeline, guard):]
	branch = branch[:strings.Index(branch, "END IF;")]
	assert.Contains(t, branch, "'"+OptOutRestoredKind+"'")
	assert.Contains(t, branch, "RETURN NEW;")
	// It comes before the ordinary branches, which would record list.unsubscribed.
	assert.Less(t, strings.Index(timeline, guard), strings.Index(timeline, "IF TG_OP = 'INSERT' THEN"))

	webhook := WebhookContactListsTriggerFunction()
	require.Contains(t, webhook, guard)
	assert.Less(t, strings.Index(webhook, guard), strings.Index(webhook, "SELECT name INTO list_name"))
}

func TestTrackMessageHistoryChangesSkipsErasedContacts(t *testing.T) {
	fn := TrackMessageHistoryChangesFunction()
	require.Equal(t, 1, strings.Count(fn, ErasedContactStep))
	// The early return opens the UPDATE branch, before the status and timeline steps.
	update := strings.Index(fn, "ELSIF TG_OP = 'UPDATE' THEN")
	step := strings.Index(fn, ErasedContactStep)
	assert.Equal(t, update+len("ELSIF TG_OP = 'UPDATE' THEN"), step)
	assert.Contains(t, ErasedContactStep, "IF NEW.contact_email = 'DELETED_EMAIL' THEN")
}
