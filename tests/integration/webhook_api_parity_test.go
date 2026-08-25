//go:build integration
// +build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// canonicalContactFields are the contact fields a Zapier trigger exposes, and so
// the fields that must mean the same thing whether the record arrived through a
// webhook or through the read endpoint behind performList.
//
// The two sources do not spell a contact the same way. The webhook payload is
// to_jsonb() over the contacts row — every column, raw column names, unset ones
// present and null — while the API marshals the Contact struct, which drops the
// db_* bookkeeping columns entirely and omits an unset field instead of sending
// null. Reconciling the two is precisely the job of the canonical shape module in
// the Zapier app, and this list is that module's field set.
var canonicalContactFields = []string{
	"email",
	"external_id",
	"timezone",
	"language",
	"first_name",
	"last_name",
	"full_name",
	"phone",
	"address_line_1",
	"address_line_2",
	"country",
	"postcode",
	"state",
	"job_title",
	"custom_string_1",
	"custom_string_2",
	"custom_string_3",
	"custom_string_4",
	"custom_string_5",
	"custom_number_1",
	"custom_number_2",
	"custom_number_3",
	"custom_number_4",
	"custom_number_5",
	"custom_datetime_1",
	"custom_datetime_2",
	"custom_datetime_3",
	"custom_datetime_4",
	"custom_datetime_5",
	"custom_json_1",
	"custom_json_2",
	"custom_json_3",
	"custom_json_4",
	"custom_json_5",
	"created_at",
	"updated_at",
}

// TestWebhookAPIParity holds the webhook payload and the read endpoint that backs
// performList to the same shape, for every v1 Zapier trigger.
//
// Zapier requires a hook trigger's performList to return records with the same
// schema as the hook itself — same spelling, same casing, same nesting — and
// checks it at review time. The failure mode when they diverge is the expensive
// one: nothing errors. The Zap keeps running, and every field the user mapped
// from the sample silently resolves to blank, on every run, until someone
// notices. Those two shapes are produced by code that shares nothing (a PL/pgSQL
// trigger on one side, a Squirrel query and a struct tag on the other), in a
// repository where neither compiler can see the other, so the only thing that can
// keep them together is a test that reads both.
//
// Each subtest builds the canonical record from both sources and compares them,
// which makes this test the executable specification of the Zapier app's shape
// modules rather than a set of spot checks.
func TestWebhookAPIParity(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	factory := suite.DataFactory
	client := suite.APIClient

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))
	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(workspace.ID)

	db, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)
	ctx := context.Background()

	subscriptionID := createZapierSubscription(t, suite, workspace.ID, "API parity",
		"https://hooks.example.com/notifuse", zapierV1EventTypes, nil, nil)

	// The delivery worker is not running under this harness, so every row the
	// triggers queue stays pending and readable for the whole test.
	deliveryFor := func(eventType string) map[string]interface{} {
		t.Helper()
		payloads := readZapierDeliveryPayloads(t, db, subscriptionID)
		payload, ok := payloads[eventType]
		require.True(t, ok, "no %s delivery was queued; the trigger did not fire", eventType)
		return payload
	}

	exec := func(query string, args ...interface{}) {
		t.Helper()
		_, err := db.ExecContext(ctx, query, args...)
		require.NoError(t, err, "statement failed: %s", query)
	}

	const email = "parity.contact@example.com"
	const listID = "parityList"
	const listName = "Parity Digest"
	const segmentID = "paritySegment"
	const segmentName = "Parity Members"

	t.Run("Contact Triggers Match contacts.list", func(t *testing.T) {
		// A spread of populated and unset fields: the two sources disagree about
		// how to say "unset" — null in the webhook, key absent in the API — and a
		// contact with every field filled would hide that.
		exec(`INSERT INTO contacts (
				email, external_id, timezone, language,
				first_name, last_name, full_name, country, job_title,
				custom_string_1, custom_number_1, custom_datetime_1, custom_json_1
			) VALUES (
				$1, 'crm-9001', 'Europe/Paris', 'fr',
				'Ada', 'Parity', 'Ada Parity', 'FR', 'Engineer',
				'gold', 42.5, NOW(), '{"plan":"pro"}'::jsonb
			)`, email)

		created := deliveryFor("contact.created")
		require.Equal(t,
			canonicalContactFromAPI(t, fetchContactRecord(t, client, workspace.ID, email, "")),
			canonicalContactFromWebhook(t, created),
			"contacts.list and the contact.created payload describe the same contact differently")

		// contact.updated fires only when one of the columns the trigger compares
		// actually changed, so this has to be a real change.
		exec(`UPDATE contacts SET job_title = 'Staff Engineer', updated_at = NOW() WHERE email = $1`, email)

		updated := deliveryFor("contact.updated")
		require.Equal(t,
			canonicalContactFromAPI(t, fetchContactRecord(t, client, workspace.ID, email, "")),
			canonicalContactFromWebhook(t, updated),
			"contacts.list and the contact.updated payload describe the same contact differently")
	})

	t.Run("List Triggers Match contacts.list Filtered By List", func(t *testing.T) {
		exec(`INSERT INTO lists (id, name, is_double_optin, is_public) VALUES ($1, $2, true, false)`, listID, listName)

		setStatus := func(status string) {
			t.Helper()
			exec(`UPDATE contact_lists SET status = $3, updated_at = NOW() WHERE email = $1 AND list_id = $2`,
				email, listID, status)
		}

		// Each transition is compared against the API immediately after it fires:
		// the read endpoint only ever reports the membership's current status, so
		// a batch of transitions followed by one read would compare four events
		// against one state.
		assertMembershipParity := func(eventType, expectedStatus string, expectedPrevious interface{}) {
			t.Helper()

			payload := deliveryFor(eventType)
			record := fetchContactRecord(t, client, workspace.ID, email, listID)
			membership := contactListEntry(t, record, listID)

			require.Equal(t,
				canonicalListMembershipFromAPI(t, record, membership),
				canonicalListMembershipFromWebhook(t, payload),
				"%s and contacts.list?list_id= describe the same membership differently", eventType)

			require.Equal(t, expectedStatus, payload["status"],
				"%s should carry the status the transition landed on", eventType)
			require.Equal(t, expectedPrevious, payload["previous_status"],
				"%s should carry the status the membership came from", eventType)

			// previous_status is reproducible by no read endpoint, which is why
			// the canonical shape emits it as null on the performList path rather
			// than leaving the key out: a key present in one path and missing from
			// the other is what breaks a user's field mapping. If the API ever
			// grows the field, the shape module should start reading it.
			require.NotContains(t, membership, "previous_status",
				"contacts.list can now report a previous status, so the canonical shape should use it")
		}

		exec(`INSERT INTO contact_lists (email, list_id, status) VALUES ($1, $2, 'active')`, email, listID)
		assertMembershipParity("list.subscribed", "active", nil)

		// active -> pending matches none of the trigger's transitions and queues
		// nothing; it exists only to set up the pending -> active move, which is
		// the one transition that produces list.confirmed.
		setStatus("pending")
		setStatus("active")
		assertMembershipParity("list.confirmed", "active", "pending")

		setStatus("unsubscribed")
		assertMembershipParity("list.unsubscribed", "unsubscribed", "active")

		setStatus("active")
		assertMembershipParity("list.resubscribed", "active", "unsubscribed")
	})

	t.Run("Segment Triggers Match segments.contacts", func(t *testing.T) {
		exec(`INSERT INTO segments (id, name, color, tree, timezone, version, status)
			VALUES ($1, $2, '#4F46E5', '{}'::jsonb, 'UTC', 1, 'active')`, segmentID, segmentName)
		exec(`INSERT INTO contact_segments (email, segment_id, version, matched_at) VALUES ($1, $2, 1, NOW())`,
			email, segmentID)

		joined := deliveryFor("segment.joined")
		members := fetchSegmentMembers(t, client, workspace.ID, segmentID)
		require.Len(t, members, 1, "the expanded listing should return the contact that just joined")

		member := members[0]
		contact, ok := member["contact"].(map[string]interface{})
		require.True(t, ok, "segments.contacts?expand=contact must nest the contact under `contact`")

		require.Equal(t,
			canonicalSegmentMembershipFromAPI(t, contact, segmentID, segmentName),
			canonicalSegmentMembershipFromWebhook(t, joined),
			"segment.joined and segments.contacts describe the same membership differently")

		// performList must be able to return the *recent* joiners, so the record
		// has to carry the moment the contact entered the segment. The emails-only
		// response shape carries no timestamp at all, which is why the expanded
		// one exists.
		require.NotEmpty(t, member["matched_at"], "an expanded member must carry its join time")

		// The expanded contact travels through a different query from
		// contacts.list, so it gets the same parity check: a Zap that samples a
		// segment trigger and a Zap that samples a contact trigger must be
		// mapping the same field names.
		require.Equal(t,
			canonicalContactFromAPI(t, fetchContactRecord(t, client, workspace.ID, email, "")),
			canonicalContactFromAPI(t, contact),
			"the contact nested in segments.contacts is shaped differently from the one contacts.list returns")

		exec(`DELETE FROM contact_segments WHERE email = $1 AND segment_id = $2`, email, segmentID)

		left := deliveryFor("segment.left")
		require.Equal(t, email, left["email"])
		require.Equal(t, segmentID, left["segment_id"])
		require.Equal(t, segmentName, left["segment_name"])

		// Leaving is a deletion, so nothing is left for a read endpoint to list.
		// The Left trigger's performList can only ever show current members, which
		// is a documented limitation rather than a bug — and the assertion is here
		// so it stays a known one.
		require.Empty(t, fetchSegmentMembers(t, client, workspace.ID, segmentID),
			"a departed member must not still be listed as belonging to the segment")
	})
}

// canonicalContactFromWebhook builds the canonical contact from a contact.* hook
// payload, which nests the whole database row under `contact`.
func canonicalContactFromWebhook(t *testing.T, payload map[string]interface{}) map[string]interface{} {
	t.Helper()

	record, ok := payload["contact"].(map[string]interface{})
	require.True(t, ok, "a contact.* payload must nest the contact under `contact`")

	return projectCanonicalContact(t, record)
}

// canonicalContactFromAPI builds the canonical contact from an API contact
// record. It reads the same field names as the webhook path: any field it has to
// rename is a field the Zapier app would have to rename too, and renaming on one
// side only is the drift this test exists to catch.
func canonicalContactFromAPI(t *testing.T, record map[string]interface{}) map[string]interface{} {
	t.Helper()

	return projectCanonicalContact(t, record)
}

// projectCanonicalContact projects a contact record onto the canonical field set,
// reading an absent key as null.
//
// Absent and null have to collapse into one value here, because the two sources
// disagree about which to use for an unset field and neither is wrong: a Zapier
// output field that is null on one path and missing on the other is the same
// blank cell to the user.
func projectCanonicalContact(t *testing.T, record map[string]interface{}) map[string]interface{} {
	t.Helper()

	canonical := make(map[string]interface{}, len(canonicalContactFields))
	for _, field := range canonicalContactFields {
		canonical[field] = normalizeParityValue(record[field])
	}
	return canonical
}

// normalizeParityValue renders every timestamp in a record as a whole second in
// UTC, so that two spellings of one instant compare equal.
//
// They are spelled differently for two reasons, neither of which a Zap can see.
// PostgreSQL renders a timestamptz in the session time zone, so to_jsonb gives an
// offset where Go gives "Z"; and the API marshals a nullable datetime with
// second precision while the same column reaches the webhook with the
// microseconds PostgreSQL stored. The contract the canonical shape owes its users
// is the same instant in ISO 8601, not the same characters, so the comparison is
// made at the precision both paths can actually carry.
func normalizeParityValue(value interface{}) interface{} {
	return rewriteTimestamps(value, func(instant time.Time) string {
		return instant.UTC().Truncate(time.Second).Format(time.RFC3339)
	})
}

// canonicalListMembershipFromWebhook builds the canonical list membership from a
// list.* hook payload.
func canonicalListMembershipFromWebhook(t *testing.T, payload map[string]interface{}) map[string]interface{} {
	t.Helper()

	return map[string]interface{}{
		"email":     payload["email"],
		"list_id":   payload["list_id"],
		"list_name": payload["list_name"],
		"status":    payload["status"],
	}
}

// canonicalListMembershipFromAPI builds the canonical list membership from a
// contacts.list record and one of its contact_lists entries.
//
// The address comes from the contact, not from the membership: contacts.list
// returns the memberships nested under the contact that owns them and does not
// repeat the address inside each one, so the membership alone cannot say who it
// belongs to.
func canonicalListMembershipFromAPI(t *testing.T, record, membership map[string]interface{}) map[string]interface{} {
	t.Helper()

	return map[string]interface{}{
		"email":     record["email"],
		"list_id":   membership["list_id"],
		"list_name": membership["list_name"],
		"status":    membership["status"],
	}
}

// canonicalSegmentMembershipFromWebhook builds the canonical segment membership
// from a segment.* hook payload.
func canonicalSegmentMembershipFromWebhook(t *testing.T, payload map[string]interface{}) map[string]interface{} {
	t.Helper()

	return map[string]interface{}{
		"email":        payload["email"],
		"segment_id":   payload["segment_id"],
		"segment_name": payload["segment_name"],
	}
}

// canonicalSegmentMembershipFromAPI builds the canonical segment membership from
// an expanded segments.contacts record.
//
// The segment's id and name are not in the response — the endpoint answers "who
// is in this segment" with contacts — so they come from the trigger's own input
// field, which is where the Zap author picked the segment. Passing them in here
// rather than reading them back is the honest model of what the app can do.
func canonicalSegmentMembershipFromAPI(t *testing.T, contact map[string]interface{}, segmentID, segmentName string) map[string]interface{} {
	t.Helper()

	return map[string]interface{}{
		"email":        contact["email"],
		"segment_id":   segmentID,
		"segment_name": segmentName,
	}
}

// fetchContactRecord returns the single contact contacts.list reports for an
// address, as raw JSON. When listID is set the request is the one the list
// triggers' performList makes, which also asks for the memberships.
func fetchContactRecord(t *testing.T, client *testutil.APIClient, workspaceID, email, listID string) map[string]interface{} {
	t.Helper()

	params := map[string]string{
		"workspace_id": workspaceID,
		"email":        email,
		"limit":        "10",
	}
	if listID != "" {
		params["list_id"] = listID
		params["with_contact_lists"] = "true"
	}

	resp, err := client.Get("/api/contacts.list", params)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode, "contacts.list must answer the performList request")

	var body struct {
		Contacts []map[string]interface{} `json:"contacts"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.Len(t, body.Contacts, 1, "expected exactly one contact for %s", email)

	return body.Contacts[0]
}

// contactListEntry returns the membership a contacts.list record carries for one
// list.
func contactListEntry(t *testing.T, record map[string]interface{}, listID string) map[string]interface{} {
	t.Helper()

	memberships, ok := record["contact_lists"].([]interface{})
	require.True(t, ok, "contacts.list must return contact_lists when with_contact_lists is set")

	for _, entry := range memberships {
		membership, ok := entry.(map[string]interface{})
		require.True(t, ok)
		if membership["list_id"] == listID {
			return membership
		}
	}

	require.FailNowf(t, "membership not returned", "no contact_lists entry for %s", listID)
	return nil
}

// fetchSegmentMembers returns the expanded segments.contacts listing, which is
// the source behind the segment triggers' performList.
func fetchSegmentMembers(t *testing.T, client *testutil.APIClient, workspaceID, segmentID string) []map[string]interface{} {
	t.Helper()

	resp, err := client.Get("/api/segments.contacts", map[string]string{
		"workspace_id": workspaceID,
		"segment_id":   segmentID,
		"expand":       "contact",
		"limit":        "10",
	})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode, "segments.contacts must answer the expanded request")

	var body struct {
		Contacts []map[string]interface{} `json:"contacts"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))

	return body.Contacts
}
