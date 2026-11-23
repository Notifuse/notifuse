package schema

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDataRetentionStatements_AreIdempotent(t *testing.T) {
	for _, statement := range DataRetentionStatements() {
		assert.Contains(t, statement, "IF NOT EXISTS", statement)
	}
}

// Every function that a retention purge must silence tests the same GUC.
func TestRetentionPurgeBranches(t *testing.T) {
	assert.Equal(t, `current_setting('notifuse.retention_purge', true) = 'on'`, RetentionPurgeOn)
	for name, body := range map[string]string{
		"track_contact_segment_changes":    TrackContactSegmentChangesFunction(),
		"webhook_contact_segments_trigger": WebhookContactSegmentsTriggerFunction(),
		"webhook_contacts_trigger":         WebhookContactsTriggerFunction(),
	} {
		assert.Contains(t, body, RetentionPurgeOn, name)
	}
	// The segment functions return before writing a segment.left row, on DELETE only.
	for _, body := range []string{TrackContactSegmentChangesFunction(), WebhookContactSegmentsTriggerFunction()} {
		deleteBranch := body[strings.Index(body, "ELSIF TG_OP = 'DELETE' THEN"):]
		assert.Less(t, strings.Index(deleteBranch, RetentionPurgeOn), strings.Index(deleteBranch, "segment.left"))
	}
	// A contact deleted by a purge is announced by its address only (D8).
	assert.Contains(t, WebhookContactsTriggerFunction(),
		`payload := jsonb_build_object('contact', jsonb_build_object('email', OLD.email));`)
}

func TestDataRetentionMirrorStatements_AreIdempotent(t *testing.T) {
	for _, statement := range DataRetentionMirrorStatements() {
		assert.Contains(t, statement, "ADD COLUMN IF NOT EXISTS", statement)
	}
}

// The condition and the function share one rule, the purge's: not ready, off, gone, exempt or
// with an exemption that cannot be trusted gives nothing; the clock reads engagement,
// collection and designated events; the waiting period has its own exemptions and events and
// never applies before its date; the 7-day floor.
func TestRetentionExpiryRule(t *testing.T) {
	condition := RetentionExpiresWithinCondition("contacts", "$1")
	function := ContactRetentionExpiresAtFunction()
	for _, part := range []string{
		"st.backfill_completed_at IS NOT NULL",
		"l.deleted_at IS NULL) = cardinality(st.exempt_list_ids)",
		"sg.status = 'active') = cardinality(st.exempt_segment_ids)",
		"cl.list_id = ANY(st.exempt_list_ids) AND cl.deleted_at IS NULL",
		"cl.list_id = ANY(st.pending_exempt_list_ids)",
		"cs.segment_id = ANY(st.pending_exempt_segment_ids)",
		"ce.event_name = ANY(st.engagement_event_names)",
		"ce.event_name = ANY(st.pending_engagement_event_names)",
		"st.contacts_pending_days), st.contacts_pending_at)",
		".db_created_at + INTERVAL '7 days'",
	} {
		assert.Contains(t, condition, part)
		assert.Contains(t, function, part)
	}
	assert.Contains(t, condition, "LEAST(contacts.created_at, contacts.db_created_at)")
	assert.Contains(t, condition, "<= NOW() + make_interval(days => $1)")
	assert.Contains(t, function, "CREATE OR REPLACE FUNCTION contact_retention_expires_at(p_email TEXT)")
}
