package schema

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTrackContactChangesFunction_RecordsEveryTrackedColumn(t *testing.T) {
	sql := TrackContactChangesFunction()

	pairs := regexp.MustCompile(`IF OLD\.(\w+) IS DISTINCT FROM NEW\.(\w+) THEN changes_json := changes_json \|\| jsonb_build_object\('(\w+)', jsonb_build_object\('old', OLD\.(\w+), 'new', NEW\.(\w+)\)\); END IF;`).FindAllStringSubmatch(sql, -1)

	var recorded []string
	for _, p := range pairs {
		for _, name := range p[2:] {
			assert.Equal(t, p[1], name, "each change line must name a single column throughout")
		}
		recorded = append(recorded, p[1])
	}
	assert.Equal(t, TrackedContactColumns, recorded)
}

func TestTrackContactChangesFunction_ConsentChangesAreRecordedButNotTheirDate(t *testing.T) {
	assert.Contains(t, TrackedContactColumns, "tracking_opens_consent")
	assert.Contains(t, TrackedContactColumns, "tracking_clicks_consent")
	assert.Contains(t, TrackedContactColumns, "tracking_consent_source")
	assert.NotContains(t, TrackContactChangesFunction(), "tracking_consent_updated_at",
		"a date alone is no change to report")
}

func TestTrackContactChangesFunction_KeepsTheKindsItShipped(t *testing.T) {
	sql := TrackContactChangesFunction()
	require.Contains(t, sql, "CREATE OR REPLACE FUNCTION track_contact_changes()")
	assert.Contains(t, sql, "VALUES (NEW.email, op, 'contact', 'contact.created', changes_json, NEW.created_at);")
	assert.Contains(t, sql, "VALUES (NEW.email, op, 'contact', 'contact.updated', changes_json, NEW.updated_at);")
	assert.Contains(t, sql, "IF changes_json = '{}'::jsonb THEN RETURN NEW; END IF;",
		"an update that changes no tracked column writes nothing")
	assert.NotContains(t, sql, "CREATE TRIGGER", "only the function is installed; the trigger stays attached")
}
