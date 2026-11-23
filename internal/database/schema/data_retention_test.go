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
