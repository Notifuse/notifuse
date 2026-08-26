package schema

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The five function names the workspace initializer attaches triggers to. A
// generator that renamed its function would install a second function under the
// new name and leave every existing trigger pointing at the stale old one, which
// no test that only reads the generated string would otherwise notice.
var webhookTriggerFunctionNames = []string{
	"webhook_contacts_trigger",
	"webhook_contact_lists_trigger",
	"webhook_contact_segments_trigger",
	"webhook_message_history_trigger",
	"webhook_custom_events_trigger",
}

func webhookTriggerBodies(t *testing.T) map[string]string {
	t.Helper()

	bodies := make(map[string]string)
	for _, sql := range WebhookTriggerFunctions() {
		for _, name := range webhookTriggerFunctionNames {
			if strings.Contains(sql, "CREATE OR REPLACE FUNCTION "+name+"()") {
				bodies[name] = sql
			}
		}
	}
	require.Len(t, bodies, len(webhookTriggerFunctionNames),
		"WebhookTriggerFunctions must return one definition per trigger function")
	return bodies
}

// lineContaining returns the index of the first line holding needle. Filter
// order matters — the guard has to precede the only statement that can suppress
// a delivery — and line indices are how that ordering gets asserted.
func lineContaining(t *testing.T, sql, needle string) int {
	t.Helper()
	for i, line := range strings.Split(sql, "\n") {
		if strings.Contains(line, needle) {
			return i
		}
	}
	require.FailNowf(t, "line not found", "no line containing %q in:\n%s", needle, sql)
	return -1
}

func TestWebhookTriggerFunctions_ReturnsEveryTriggerExactlyOnce(t *testing.T) {
	defs := WebhookTriggerFunctions()
	require.Len(t, defs, len(webhookTriggerFunctionNames))

	bodies := webhookTriggerBodies(t)
	for name, sql := range bodies {
		// CREATE OR REPLACE, not CREATE: a migration reinstalls these over
		// functions that already exist, and a plain CREATE would fail the whole
		// migration transaction on the second workspace onwards.
		assert.Truef(t, strings.HasPrefix(sql, "CREATE OR REPLACE FUNCTION "+name+"()"),
			"%s must be a CREATE OR REPLACE so a reinstall is idempotent", name)
		assert.Truef(t, strings.HasSuffix(sql, "$$ LANGUAGE plpgsql"),
			"%s must end at the closing dollar quote, with no trailing statement", name)
		assert.Equalf(t, 2, strings.Count(sql, "$$"),
			"%s must open and close exactly one dollar-quoted body", name)
		assert.Equalf(t, 1, strings.Count(sql, "CREATE OR REPLACE FUNCTION"),
			"%s must define exactly one function", name)
	}
}

// A migration installs these on every existing workspace inside one transaction,
// so every lock they take is held until commit. CREATE OR REPLACE FUNCTION
// updates a pg_proc row; DROP/CREATE TRIGGER takes a ShareRowExclusiveLock on
// contacts, contact_lists, contact_segments, message_history or custom_events —
// and because deliveries are enqueued by AFTER-row triggers inside customer
// write transactions, locking those tables is a write outage, not a webhook
// outage. An already-attached trigger picks up a replaced body on its next call.
func TestWebhookTriggerFunctions_CarryNoTriggerAttachment(t *testing.T) {
	for name, sql := range webhookTriggerBodies(t) {
		assert.NotContainsf(t, sql, "CREATE TRIGGER",
			"%s must not reattach its trigger; replacing the function body is enough", name)
		assert.NotContainsf(t, sql, "DROP TRIGGER",
			"%s must not drop its trigger; the window with no trigger loses events", name)
	}
}

// The subscription lookup is the fan-out predicate, and it is identical in all
// five bodies on purpose. This is the test that catches the tempting wrong fix
// for id filtering: pushing `settings->'list_ids' ? NEW.list_id` into this
// WHERE clause silently drops every subscription that has no list_ids key, which
// is every subscription that exists today.
func TestWebhookTriggerFunctions_ShareOneSubscriptionMatchPredicate(t *testing.T) {
	const predicate = "WHERE enabled = true AND event_kind = ANY(ARRAY(SELECT jsonb_array_elements_text(settings->'event_types')))"

	// The custom-events fan-out matches on a separate variable, because a
	// soft-delete and a restore report a different kind from the type subscribed
	// to, but it is otherwise the same predicate.
	const customEventsPredicate = "WHERE enabled = true AND subscribed_event_type = ANY(ARRAY(SELECT jsonb_array_elements_text(settings->'event_types')))"

	for name, sql := range webhookTriggerBodies(t) {
		want := predicate
		if name == "webhook_custom_events_trigger" {
			want = customEventsPredicate
		}

		// Equality against the whole line, not Contains. Contains would accept a
		// filter appended to this clause, and appending is the tempting wrong
		// fix for id filtering: `AND settings->'list_ids' ? NEW.list_id` reads
		// correctly and silently drops every subscription that has no list_ids
		// key — which is every subscription that exists today.
		var found []string
		for _, line := range strings.Split(sql, "\n") {
			if strings.Contains(line, "WHERE enabled = true") {
				found = append(found, strings.TrimSpace(line))
			}
		}
		require.Lenf(t, found, 1, "%s must have exactly one subscription lookup", name)
		assert.Equalf(t, want, found[0],
			"%s must select subscriptions on enabled + event type and nothing else", name)
	}
}

func TestWebhookTriggerFunctions_EnqueueAPendingDeliveryPerMatch(t *testing.T) {
	const insert = "INSERT INTO webhook_deliveries (id, subscription_id, event_type, payload, status, attempts, max_attempts, next_attempt_at)"

	for name, sql := range webhookTriggerBodies(t) {
		assert.Containsf(t, sql, insert, "%s must enqueue through the delivery table", name)
		assert.Containsf(t, sql, "'pending', 0, 10, NOW()",
			"%s must enqueue rows the delivery worker's pending predicate selects", name)
		assert.Containsf(t, sql, "gen_random_uuid()::text",
			"%s must generate the delivery id in the database", name)
	}
}

// ---------------------------------------------------------------------------
// list_ids / segment_ids filtering
// ---------------------------------------------------------------------------

type filteredTrigger struct {
	function  string
	variable  string
	key       string
	matchedBy string
}

var filteredTriggers = []filteredTrigger{
	{
		function:  "webhook_contact_lists_trigger",
		variable:  "list_filter",
		key:       "list_ids",
		matchedBy: "NEW.list_id",
	},
	{
		function: "webhook_contact_segments_trigger",
		variable: "segment_filter",
		key:      "segment_ids",
		// contact_segments fires AFTER INSERT OR DELETE, and NEW is unassigned
		// on the DELETE that produces segment.left. Reading NEW.segment_id
		// directly there would compare NULL against the filter, and NULL = ANY
		// is never true — so every filtered subscription would silently stop
		// receiving segment.left while still receiving segment.joined.
		matchedBy: "COALESCE(NEW.segment_id, OLD.segment_id)",
	},
}

// The case that decides whether this feature is safe to ship. No subscription
// that exists today carries a list_ids or segment_ids key, so a filter that
// treats "no key" as "match nothing" turns every customer's webhooks off at the
// moment of upgrade, silently and with no error anywhere.
//
// The proof is structural rather than a string search for the absent case: the
// only statement that can suppress a delivery is inside the guard, so a
// subscription whose settings do not satisfy the guard cannot reach it.
func TestFilteredWebhookTriggers_AbsentOrEmptyFilterMatchesEverything(t *testing.T) {
	bodies := webhookTriggerBodies(t)

	for _, tc := range filteredTriggers {
		t.Run(tc.function, func(t *testing.T) {
			sql := bodies[tc.function]

			// jsonb_typeof, not a `? 'key'` existence test: `?` is true for a
			// JSON null, and jsonb_array_length then raises "cannot get array
			// length of a scalar" — inside the customer's own write
			// transaction, which fails their INSERT rather than the webhook.
			// jsonb_typeof of a missing key is SQL NULL and of a JSON null is
			// 'null', so both fall out of the guard as "no filter".
			guard := fmt.Sprintf("IF jsonb_typeof(%s) = 'array' AND jsonb_array_length(%s) > 0 THEN", tc.variable, tc.variable)
			require.Contains(t, sql, guard,
				"the filter must be guarded on the value actually being a non-empty array")

			assert.Equal(t, 1, strings.Count(sql, "should_deliver := true;"),
				"should_deliver must start true exactly once per subscription")
			require.Equal(t, 1, strings.Count(sql, "should_deliver := false;"),
				"exactly one statement may suppress a delivery, and it must be the guarded one")

			// Ordering is the whole proof: the suppressing statement sits after
			// the guard that opens the only block containing it, so an absent,
			// null or empty filter never reaches it.
			assert.Less(t,
				lineContaining(t, sql, guard),
				lineContaining(t, sql, "should_deliver := false;"),
				"the guard must precede the only statement that suppresses a delivery")

			assert.Contains(t, sql, "IF should_deliver THEN",
				"the enqueue must be gated on the flag rather than on the filter directly")
		})
	}
}

func TestFilteredWebhookTriggers_PopulatedFilterMatchesOnlyThoseIDs(t *testing.T) {
	bodies := webhookTriggerBodies(t)

	for _, tc := range filteredTriggers {
		t.Run(tc.function, func(t *testing.T) {
			sql := bodies[tc.function]

			// The loop reads sub.settings, so the cursor query has to return it.
			// PL/pgSQL resolves record fields at run time, so a cursor that
			// selects only id raises "record sub has no field settings" on the
			// first matching subscription — inside the customer's own write
			// transaction, aborting their INSERT rather than just the webhook.
			assert.Contains(t, sql, "SELECT id, settings FROM webhook_subscriptions",
				"a filtered fan-out must select the settings column it reads")

			assert.Contains(t, sql, fmt.Sprintf("%s := sub.settings->'%s';", tc.variable, tc.key),
				"the filter must be read per subscription, not once for the whole fan-out")

			// jsonb_array_elements_text, not jsonb_array_elements: the ids are
			// text columns, and comparing a text column against a jsonb scalar
			// would need a cast that does not exist and would fail at runtime.
			assert.Contains(t, sql, fmt.Sprintf("IF NOT (%s = ANY(", tc.matchedBy))
			assert.Contains(t, sql, fmt.Sprintf("SELECT jsonb_array_elements_text(%s)", tc.variable))
		})
	}
}

// Only the list and segment fan-outs take an id filter. A filter leaking into
// the contact, email or custom-event triggers would narrow event types that have
// no list or segment to narrow by, and every subscription carrying the key would
// stop receiving them.
func TestWebhookTriggerFunctions_OnlyListAndSegmentTriggersFilterOnIDs(t *testing.T) {
	filtered := map[string]bool{}
	for _, tc := range filteredTriggers {
		filtered[tc.function] = true
	}

	for name, sql := range webhookTriggerBodies(t) {
		if filtered[name] {
			continue
		}
		assert.NotContainsf(t, sql, "list_ids", "%s must not filter on list_ids", name)
		assert.NotContainsf(t, sql, "segment_ids", "%s must not filter on segment_ids", name)
	}
}

// contacts and message_history carry no per-subscription filter of any kind:
// every subscription matching the event type gets the delivery. custom_events is
// excluded because custom_event_filters is its own, older feature.
func TestWebhookTriggerFunctions_UnfilteredTriggersEnqueueForEveryMatch(t *testing.T) {
	bodies := webhookTriggerBodies(t)

	for _, name := range []string{"webhook_contacts_trigger", "webhook_message_history_trigger"} {
		sql := bodies[name]
		assert.NotContainsf(t, sql, "should_deliver",
			"%s must enqueue for every matching subscription", name)
		// Without a filter the fan-out has no reason to read settings, and not
		// reading it is what keeps the row narrow on a hot write path.
		assert.Containsf(t, sql, "SELECT id FROM webhook_subscriptions",
			"%s must not select settings it has no filter to apply", name)
	}
}

// ---------------------------------------------------------------------------
// The three unfiltered bodies, pinned so the extraction cannot have changed them
// ---------------------------------------------------------------------------

// Every column the update branch compares. A column missing from this list means
// an UPDATE that changes only that column emits no contact.updated at all, which
// looks to a subscriber exactly like a write that never happened.
func TestWebhookContactsTriggerFunction_ComparesTheColumnsItShipped(t *testing.T) {
	sql := WebhookContactsTriggerFunction()

	pairs := regexp.MustCompile(`NEW\.(\w+) IS NOT DISTINCT FROM OLD\.(\w+)`).FindAllStringSubmatch(sql, -1)

	var compared []string
	for _, pair := range pairs {
		assert.Equal(t, pair[1], pair[2], "a comparison must put the same column on both sides")
		compared = append(compared, pair[1])
	}

	expected := []string{
		"external_id", "timezone", "language", "first_name", "last_name", "full_name",
		"phone", "address_line_1", "address_line_2", "country", "postcode", "state", "job_title",
		"custom_string_1", "custom_string_2", "custom_string_3", "custom_string_4", "custom_string_5",
		"custom_number_1", "custom_number_2", "custom_number_3", "custom_number_4", "custom_number_5",
		"custom_datetime_1", "custom_datetime_2", "custom_datetime_3", "custom_datetime_4", "custom_datetime_5",
		"custom_json_1", "custom_json_2", "custom_json_3", "custom_json_4", "custom_json_5",
	}
	require.Len(t, expected, 33)
	assert.Equal(t, expected, compared)

	// A DELETE has no NEW row, so the payload is built from OLD and the function
	// must return a row that exists or the delete is silently discarded.
	assert.Contains(t, sql, "contact_record := OLD;")
	assert.Contains(t, sql, "RETURN COALESCE(NEW, OLD);")
	assert.Contains(t, sql, "'contact', to_jsonb(contact_record)")
}

func TestWebhookContactListsTriggerFunction_EmitsTheKindsItShipped(t *testing.T) {
	sql := WebhookContactListsTriggerFunction()

	// The INSERT branch keys off the status the membership row is created with.
	for status, kind := range map[string]string{
		"active":       "list.subscribed",
		"pending":      "list.pending",
		"unsubscribed": "list.unsubscribed",
		"bounced":      "list.bounced",
		"complained":   "list.complained",
	} {
		assert.Containsf(t, sql, fmt.Sprintf("WHEN '%s' THEN event_kind := '%s';", status, kind),
			"the INSERT branch must map status %s", status)
	}

	// A returning contact reports confirmed or resubscribed rather than
	// subscribed, so a consumer bound to list.subscribed alone misses every
	// re-subscriber. Both transitions have to keep their own kind.
	assert.Contains(t, sql, "IF OLD.status = 'pending' AND NEW.status = 'active' THEN")
	assert.Contains(t, sql, "event_kind := 'list.confirmed';")
	assert.Contains(t, sql, "ELSIF OLD.status IN ('unsubscribed', 'bounced', 'complained') AND NEW.status = 'active' THEN")
	assert.Contains(t, sql, "event_kind := 'list.resubscribed';")

	// list.removed is a subordinate branch: it is only reached when the status
	// did not change, so a soft delete that also changes status reports the
	// status change and never the removal.
	assert.Contains(t, sql, "ELSIF NEW.deleted_at IS NOT NULL AND OLD.deleted_at IS NULL THEN")
	assert.Contains(t, sql, "event_kind := 'list.removed';")

	// previous_status is in the payload and is reproducible by no read endpoint,
	// which is exactly why it must not be dropped.
	assert.Contains(t, sql, "'previous_status', CASE WHEN TG_OP = 'UPDATE' THEN OLD.status ELSE NULL END")
	for _, key := range []string{"'email', NEW.email", "'list_id', NEW.list_id", "'list_name', list_name", "'status', NEW.status"} {
		assert.Contains(t, sql, key)
	}
}

func TestWebhookContactSegmentsTriggerFunction_HandlesTheDeletePath(t *testing.T) {
	sql := WebhookContactSegmentsTriggerFunction()

	assert.Contains(t, sql, "event_kind := 'segment.joined';")
	assert.Contains(t, sql, "event_kind := 'segment.left';")

	// NEW is unassigned on DELETE, so every read of the row has to coalesce, and
	// the function has to return OLD — returning NEW from an AFTER trigger is
	// harmless but returning the wrong row makes the intent unreadable.
	assert.Contains(t, sql, "SELECT name INTO segment_name FROM segments WHERE id = COALESCE(NEW.segment_id, OLD.segment_id);")
	assert.Contains(t, sql, "contact_email := COALESCE(NEW.email, OLD.email);")
	assert.Contains(t, sql, "'segment_id', COALESCE(NEW.segment_id, OLD.segment_id)")
	assert.Regexp(t, `IF TG_OP = 'DELETE' THEN\s+RETURN OLD;`, sql,
		"the DELETE path must return OLD; returning an unassigned NEW makes the intent unreadable")
}

// The email event chain is an ELSIF over first-transition timestamps, so its
// order is behaviour: one UPDATE that stamps both delivered_at and opened_at
// emits only the earlier branch. Reordering it would change which event a
// provider's combined callback produces.
func TestWebhookMessageHistoryTriggerFunction_KeepsItsEventChainOrder(t *testing.T) {
	sql := WebhookMessageHistoryTriggerFunction()

	matches := regexp.MustCompile(`NEW\.(\w+) IS NOT NULL AND OLD\.\w+ IS NULL THEN\s+event_kind := '([\w.]+)';`).
		FindAllStringSubmatch(sql, -1)

	var chain [][2]string
	for _, m := range matches {
		chain = append(chain, [2]string{m[1], m[2]})
	}

	assert.Equal(t, [][2]string{
		{"delivered_at", "email.delivered"},
		{"opened_at", "email.opened"},
		{"clicked_at", "email.clicked"},
		{"bounced_at", "email.bounced"},
		{"complained_at", "email.complained"},
		{"unsubscribed_at", "email.unsubscribed"},
	}, chain)

	// INSERT is email.sent and is not part of the chain.
	assert.Contains(t, sql, "event_kind := 'email.sent';")

	for _, key := range []string{
		"'email', NEW.contact_email", "'message_id', NEW.id", "'template_id', NEW.template_id",
		"'broadcast_id', NEW.broadcast_id", "'list_id', NEW.list_id", "'channel', NEW.channel",
		"'event_timestamp', event_timestamp",
	} {
		assert.Contains(t, sql, key)
	}
}

// ---------------------------------------------------------------------------
// custom_event_filters — unchanged by the list/segment work
// ---------------------------------------------------------------------------

func TestWebhookCustomEventsTriggerFunction_FiltersAreUnchanged(t *testing.T) {
	sql := WebhookCustomEventsTriggerFunction()

	assert.Contains(t, sql, "custom_filters := sub.settings->'custom_event_filters';")

	for _, key := range []string{"goal_types", "event_names"} {
		// jsonb_typeof, not a key-exists test. Both guards read a free-form jsonb
		// column, and `settings ? 'goal_types'` is true for a key whose value is
		// JSON null — at which point jsonb_array_length raises "cannot get array
		// length of a scalar". This trigger runs inside the customer's write
		// transaction, so that raise fails their INSERT into custom_events: the
		// events.track call 500s, and no webhook is involved in the damage.
		assert.Regexpf(t,
			fmt.Sprintf(`jsonb_typeof\(custom_filters->'%s'\) = 'array'\s+AND jsonb_array_length\(custom_filters->'%s'\) > 0 THEN`, key, key),
			sql, "the %s filter must be guarded on the type, not on the key", key)
		assert.NotContainsf(t, sql, fmt.Sprintf(`custom_filters ? '%s'`, key),
			"a key-exists test lets a JSON null through to jsonb_array_length, which raises")
		assert.Contains(t, sql, fmt.Sprintf("SELECT jsonb_array_elements_text(custom_filters->'%s')", key))
	}

	// An unset goal_type must not silently pass a goal_types filter.
	assert.Contains(t, sql, "IF NEW.goal_type IS NULL OR NOT (NEW.goal_type = ANY(")

	// Web analytics goals are bridged into custom_events as a first-party
	// artifact. Fanning them out would ship pageview-scale conversions to
	// subscribers who asked for API-sourced commerce events.
	assert.Contains(t, sql, "IF NEW.source = 'web_analytics' THEN")
	assert.Less(t,
		lineContaining(t, sql, "IF NEW.source = 'web_analytics' THEN"),
		lineContaining(t, sql, "INSERT INTO webhook_deliveries"),
		"the web analytics skip must precede any enqueue")

	// Soft delete and restore report a kind that differs from the subscribed
	// type only in that both are tracked separately; losing either variable
	// collapses the two and changes which subscriptions match.
	assert.Contains(t, sql, "event_kind := 'custom_event.deleted';")
	assert.Contains(t, sql, "subscribed_event_type := 'custom_event.created';")
}
