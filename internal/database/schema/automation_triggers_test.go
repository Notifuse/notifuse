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

// onceBlock returns the 'once' block of an enrolment body, from its IF to the END IF; that
// closes it.
func onceBlock(t *testing.T, sql string) string {
	t.Helper()
	start := strings.Index(sql, "IF p_frequency = 'once' THEN")
	require.NotEqual(t, -1, start, "once block not found")
	end := strings.Index(sql[start:], "\n\t\t\tEND IF;")
	require.NotEqual(t, -1, end, "end of the once block not found")
	return sql[start : start+end]
}

// Two transactions that enrol the same contact into a 'once' automation at the same time both
// pass the existence check; the second insert into automation_trigger_log then waits for the
// first and inserts nothing. Only the transaction whose insert added the row may enrol.
//
// The check has to sit inside the 'once' block: FOUND is false when the function starts, and
// the live check above does not set it, so the same check after the block would stop every
// every_time enrolment.
func TestAutomationEnrollContactWithTriggerFunction_OnceEnrolsOnlyWhenItsLogRowWasInserted(t *testing.T) {
	sql := AutomationEnrollContactWithTriggerFunction()
	block := onceBlock(t, sql)

	insertIdx := strings.Index(block, "INSERT INTO automation_trigger_log")
	notFoundIdx := strings.Index(block, "IF NOT FOUND THEN\n\t\t\t\t\tRETURN;\n\t\t\t\tEND IF;")
	require.NotEqual(t, -1, insertIdx)
	require.NotEqual(t, -1, notFoundIdx, "the once block must return when its log insert did nothing")
	assert.Less(t, insertIdx, notFoundIdx)
	assert.Equal(t, 1, strings.Count(sql, "IF NOT FOUND"), "FOUND is read only after the log insert")
}

// Two enrolments of one contact into one every_time automation in one transaction (two events of
// one name for one buyer in a batch) must be two journeys, not a violation of
// UNIQUE(automation_id, contact_email, entered_at) that aborts the batch: entered_at is the time
// of the call, not of the transaction start.
func TestAutomationEnrollContactWithTriggerFunction_DatesTheJourneyAtTheCall(t *testing.T) {
	sql := AutomationEnrollContactWithTriggerFunction()

	insert := strings.Index(sql, "INSERT INTO contact_automations (")
	require.NotEqual(t, -1, insert)
	stmt := sql[insert:]
	stmt = stmt[:strings.Index(stmt, ");")+2]
	assert.Contains(t, stmt, "'active',\n\t\t\t\tclock_timestamp(),\n\t\t\t\tNOW(),",
		"entered_at takes clock_timestamp(), scheduled_at stays NOW()")
	assert.Equal(t, 1, strings.Count(sql, "clock_timestamp()"), "only entered_at changes clock")
}

// v38 still installs its own body on a database below 38, before v44 replaces it: that body
// stays as v38 shipped it.
func TestAutomationEnrollContactFunction_StaysAsV38ShippedIt(t *testing.T) {
	sql := AutomationEnrollContactFunction()
	assert.NotContains(t, sql, "clock_timestamp()")
	assert.NotContains(t, sql, "IF NOT FOUND")
}
