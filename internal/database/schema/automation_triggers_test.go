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

// The generated triggers call the five-argument signature with the event that fired them,
// which must land in contact_automations.context.trigger for the email node to read.
func TestAutomationEnrollContactWithTriggerFunction_StoresTheTriggerInContext(t *testing.T) {
	sql := AutomationEnrollContactWithTriggerFunction()

	assert.Contains(t, sql, "p_trigger JSONB")
	assert.Contains(t, sql, "status, entered_at, scheduled_at, context")
	assert.Contains(t, sql, "jsonb_build_object('trigger', p_trigger)")
	assert.Contains(t, sql, "WHEN p_trigger IS NULL THEN '{}'::jsonb",
		"a call without an event must store the column's default, not {\"trigger\": null}")
}

// The five-argument overload carries the same guard as the four-argument body it replaces.
func TestAutomationEnrollContactWithTriggerFunction_EnrolsOnlyForLiveAutomations(t *testing.T) {
	sql := AutomationEnrollContactWithTriggerFunction()

	guard := strings.Index(sql, "status = 'live'")
	require.NotEqual(t, -1, guard)
	assert.Contains(t, sql, "deleted_at IS NULL")
	for _, write := range []string{"INSERT INTO automation_trigger_log", "INSERT INTO contact_automations", "UPDATE automations"} {
		idx := strings.Index(sql, write)
		require.NotEqualf(t, -1, idx, "%s missing", write)
		assert.Lessf(t, guard, idx, "the live guard must precede %s", write)
	}
}

// Triggers installed before 44.0 keep calling the four-argument signature until they are
// regenerated. The relay keeps it, and passes an explicitly typed NULL: an untyped NULL could
// also match the VARCHAR five-argument overload a pre-v25 workspace still has.
func TestAutomationEnrollContactRelayFunction_KeepsTheOldSignatureAndRelays(t *testing.T) {
	sql := AutomationEnrollContactRelayFunction()

	assert.Contains(t, sql, "CREATE OR REPLACE FUNCTION automation_enroll_contact(")
	assert.NotContains(t, sql, "p_trigger")
	assert.Contains(t, sql, "p_automation_id, p_contact_email, p_root_node_id, p_frequency, NULL::jsonb")
	assert.NotContains(t, sql, "INSERT INTO", "the relay must not carry a second copy of the enrolment")
}
