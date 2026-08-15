package schema

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The installed per-automation triggers call this function by name with four arguments.
// Changing the signature would leave every trigger already installed calling a function
// that no longer exists, aborting the contact_timeline insert that fired it.
func TestAutomationEnrollContactFunction_KeepsItsSignature(t *testing.T) {
	sql := AutomationEnrollContactFunction()

	assert.Contains(t, sql, "CREATE OR REPLACE FUNCTION automation_enroll_contact(")
	for _, param := range []string{"p_automation_id", "p_contact_email", "p_root_node_id", "p_frequency"} {
		assert.Containsf(t, sql, param, "parameter %s must survive", param)
	}
}

// A trigger can outlive the automation being live: Pause drops it after writing the status,
// Delete ignores a failed drop, and two concurrent transitions can leave it installed against
// a paused row. Enrolling from any of those states accrues journeys, timeline events and stats
// that nothing detects and that all thaw at once on re-activation.
func TestAutomationEnrollContactFunction_EnrolsOnlyForLiveAutomations(t *testing.T) {
	sql := AutomationEnrollContactFunction()

	assert.Contains(t, sql, "status = 'live'",
		"enrolment must be gated on the automation still being live")
	assert.Contains(t, sql, "deleted_at IS NULL",
		"a soft-deleted automation whose trigger drop failed must not enrol either")
}

// The guard has to run before anything is written, not merely before the enrolment insert:
// the 'once' dedup row is permanent and nothing ever deletes it, so a ghost fire that records
// one would bar that contact from the automation for good once it is live again.
func TestAutomationEnrollContactFunction_GuardPrecedesEveryWrite(t *testing.T) {
	sql := AutomationEnrollContactFunction()

	guard := strings.Index(sql, "status = 'live'")
	require.NotEqual(t, -1, guard, "guard not found")

	for _, write := range []string{
		"INSERT INTO automation_trigger_log",
		"INSERT INTO contact_automations",
		"UPDATE automations",
		"INSERT INTO automation_node_executions",
		"INSERT INTO contact_timeline",
	} {
		idx := strings.Index(sql, write)
		require.NotEqualf(t, -1, idx, "%q not found in the function body", write)
		assert.Lessf(t, guard, idx, "the live guard must precede %q", write)
	}
}
