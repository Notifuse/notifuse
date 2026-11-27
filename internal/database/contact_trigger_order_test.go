package database

import (
	"os"
	"testing"

	"github.com/Notifuse/notifuse/internal/database/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A new workspace creates contact_changes_trigger under the name schema.ContactChangesTrigger
// holds, so the opt-out restore trigger, named to sort before it, fires first there too.
// init.go writes the name as a literal for the architecture repository's references.
func TestInitCreatesContactChangesTriggerUnderTheSharedName(t *testing.T) {
	source, err := os.ReadFile("init.go")
	require.NoError(t, err)
	assert.Contains(t, string(source), "CREATE TRIGGER "+schema.ContactChangesTrigger+" AFTER INSERT OR UPDATE ON contacts")
	assert.Less(t, schema.ContactOptOutRestoreTrigger, schema.ContactChangesTrigger)
}
