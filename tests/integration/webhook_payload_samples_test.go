//go:build integration
// +build integration

package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// zapierSamplesPath is the checked-in artifact this test generates. The Zapier
// app reads it as the source of its static `sample` objects, which Zapier
// machine-checks against live deliveries at app review time.
var zapierSamplesPath = filepath.Join("testdata", "webhook_payload_samples.json")

// zapierV1EventTypes are the event types the first published Zapier triggers
// subscribe to. Kept in the order a subscriber meets them — a contact appears,
// changes, joins and leaves a list, joins and leaves a segment — because the
// generated file is read by humans deciding what to map.
var zapierV1EventTypes = []string{
	"contact.created",
	"contact.updated",
	"list.subscribed",
	"list.confirmed",
	"list.resubscribed",
	"list.unsubscribed",
	"segment.joined",
	"segment.left",
}

// Values substituted for the parts of an envelope a capture cannot pin down: the
// delivery row's id is a fresh uuid, the workspace id is minted per test run, and
// the timestamp is stamped by the delivery worker at send time and never stored.
// They are obvious placeholders on purpose — a reader of the samples file must
// not mistake them for values a real delivery carries.
const (
	zapierSampleDeliveryID = "00000000-0000-0000-0000-000000000000"
	zapierSampleWorkspace  = "sampleworkspace"
	zapierSampleTimestamp  = "2024-01-15T09:30:00Z"
)

// The seed data behind every sample. Fixed rather than generated so the file
// contains representative values a Zap author can recognise, and so two runs
// produce the same bytes. Nothing here is a real address or a real person.
const (
	zapierSampleEmail       = "bob.sample@example.com"
	zapierSampleListID      = "zapsamplelist"
	zapierSampleListName    = "Product Updates"
	zapierSampleSegmentID   = "zapsampleseg"
	zapierSampleSegmentName = "Recent Buyers"
	// Every timestamp column is written explicitly with this instant, so the only
	// thing left to normalise is the rendering (see normalizeZapierSampleValue).
	zapierSampleInstant = "2024-01-15T09:30:00Z"
)

// TestWebhookPayloadSamples generates the payload contract the Zapier app is
// built against, and fails when the backend moves away from it.
//
// The webhook payload is assembled inside PostgreSQL by the PL/pgSQL trigger
// functions in internal/database/init.go. No Go or TypeScript compiler can see
// that shape, so nothing warns when an edited trigger renames a key or nests it
// differently — and a Zapier app whose `sample` and `performList` output stop
// matching the live payload does not error, it silently blanks every field
// mapping its users built. Deriving those samples by reading the SQL string
// literals by eye is exactly how that drift starts.
//
// So the samples are captured from real deliveries: insert a record of each
// kind, let the real trigger fire, read what it wrote into
// webhook_deliveries.payload, and write the collected envelopes to
// testdata/webhook_payload_samples.json. A trigger change then shows up as a
// diff in that file, in the pull request that changed the trigger.
//
// Byte stability is a property of the generated file, not a nicety: a file that
// churns on every run teaches its readers to ignore its diffs, which defeats the
// whole mechanism. Two independent captures must therefore produce identical
// bytes, which this test checks by capturing twice into two separate workspaces.
func TestWebhookPayloadSamples(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	user, err := suite.DataFactory.CreateUser()
	require.NoError(t, err)
	require.NoError(t, suite.APIClient.Login(user.Email, "password"))

	firstWorkspace, firstEvents := captureZapierSampleEnvelopes(t, suite, user.ID)
	_, secondEvents := captureZapierSampleEnvelopes(t, suite, user.ID)

	first := renderZapierSamplesFile(t, firstEvents)
	second := renderZapierSamplesFile(t, secondEvents)

	t.Run("Regenerated Samples Are Byte Stable", func(t *testing.T) {
		// The two captures ran against different workspaces, different delivery
		// row ids and different wall-clock moments. Identical bytes mean the
		// normalisation left nothing volatile in the file.
		require.Equal(t, first, second,
			"two captures of the same events produced different bytes, so the generated file would churn on every run")
	})

	t.Run("Envelope Keys Match A Real Delivery", func(t *testing.T) {
		// The trigger builds only the `data` object; the envelope around it is
		// assembled in Go at send time. Reading webhook_deliveries alone can
		// therefore never reveal an envelope key that was added or dropped, so
		// the sample envelope is checked against one the delivery path actually
		// put on the wire.
		delivered := sendZapierProbeDelivery(t, suite, firstWorkspace)

		var sample map[string]interface{}
		require.NoError(t, json.Unmarshal([]byte(first), &sample))
		events := sample["events"].(map[string]interface{})
		captured := events["contact.created"].(map[string]interface{})

		require.Equal(t, sortedKeys(delivered), sortedKeys(captured),
			"the samples file describes an envelope shape no consumer receives")
	})

	t.Run("Checked-In Samples Match The Live Triggers", func(t *testing.T) {
		if os.Getenv("UPDATE_WEBHOOK_SAMPLES") == "1" {
			require.NoError(t, os.MkdirAll(filepath.Dir(zapierSamplesPath), 0o755))
			require.NoError(t, os.WriteFile(zapierSamplesPath, []byte(first), 0o644))
			t.Logf("wrote %s — commit it in the same change as the trigger edit that produced it", zapierSamplesPath)
			return
		}

		// A missing file is a failure rather than a reason to write one. Writing
		// it here would let a run with no baseline pass, and a baseline that is
		// regenerated by the same run it is compared against guards nothing.
		existing, err := os.ReadFile(zapierSamplesPath)
		require.NoError(t, err,
			"%s does not exist yet. It is the payload contract the Zapier app is built from: "+
				"generate it with UPDATE_WEBHOOK_SAMPLES=1 and commit it.", zapierSamplesPath)

		require.Equal(t, string(existing), first,
			"the payload the webhook triggers build no longer matches %s.\n"+
				"The Zapier app derives its static samples and its performList shape from that file, and Zapier "+
				"machine-checks both against live deliveries at review time: a key that exists in the sample and "+
				"not in the payload blanks every user mapping that reads it.\n"+
				"Re-run this test with UPDATE_WEBHOOK_SAMPLES=1 and commit the diff alongside the trigger change.",
			zapierSamplesPath)
	})
}

// captureZapierSampleEnvelopes seeds one workspace with the shortest sequence of
// writes that fires each v1 trigger exactly once, then returns that workspace and
// the normalised envelope per event type.
func captureZapierSampleEnvelopes(t *testing.T, suite *testutil.IntegrationTestSuite, userID string) (string, map[string]interface{}) {
	t.Helper()

	factory := suite.DataFactory

	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(userID, workspace.ID, "owner"))
	suite.APIClient.SetWorkspaceID(workspace.ID)

	db, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	// example.com is reserved for documentation and resolves nowhere, and the
	// delivery worker does not run under this harness anyway, so the rows stay
	// pending and readable for the whole test.
	subscriptionID := createZapierSubscription(t, suite, workspace.ID, "Payload sample capture",
		"https://hooks.example.com/notifuse", zapierV1EventTypes, nil, nil)

	seedZapierSampleEvents(t, db)

	deliveries := readZapierDeliveryPayloads(t, db, subscriptionID)
	require.Len(t, deliveries, len(zapierV1EventTypes),
		"the seed sequence must fire each subscribed trigger exactly once: %v", sortedKeys(deliveries))

	events := make(map[string]interface{}, len(deliveries))
	for _, eventType := range zapierV1EventTypes {
		payload, ok := deliveries[eventType]
		require.True(t, ok, "no delivery was queued for %s", eventType)

		// Mirrors the envelope processDelivery marshals around the stored
		// payload; the "Envelope Keys Match A Real Delivery" subtest is what
		// keeps this mirror honest.
		events[eventType] = map[string]interface{}{
			"id":           zapierSampleDeliveryID,
			"type":         eventType,
			"workspace_id": zapierSampleWorkspace,
			"timestamp":    zapierSampleTimestamp,
			"data":         normalizeZapierSampleValue(payload),
		}
	}

	return workspace.ID, events
}

// seedZapierSampleEvents performs the writes, in order, that make each v1 trigger
// fire once. It writes SQL directly rather than going through the API because the
// subject under test is the trigger: what a payload contains has to be a function
// of the row, not of whichever endpoint happened to write it.
func seedZapierSampleEvents(t *testing.T, db *sql.DB) {
	t.Helper()

	ctx := context.Background()
	exec := func(query string, args ...interface{}) {
		t.Helper()
		_, err := db.ExecContext(ctx, query, args...)
		require.NoError(t, err, "seed statement failed: %s", query)
	}

	// Every timestamp column is written explicitly, including the db_* pair that
	// would otherwise default to now(), so the captured payload holds no
	// wall-clock value at all.
	//
	// Every field a Zap author can map from a contact is populated. to_jsonb emits
	// every column of the row, so an unpopulated column reaches the Zapier app's
	// static sample as null — and a null in a sample is a key with no preview in
	// the field picker, which is where a Zap is built before any real delivery has
	// arrived. The custom slots are left NULL deliberately and separately: the app
	// strips them from its contact samples, because their labels are per-workspace.
	exec(`INSERT INTO contacts (
			email, external_id, timezone, language,
			first_name, last_name, full_name, phone, country, job_title,
			address_line_1, address_line_2, postcode, state,
			custom_string_1, custom_number_1, custom_datetime_1, custom_json_1,
			created_at, updated_at, db_created_at, db_updated_at
		) VALUES (
			$1, 'crm-4815', 'Europe/Paris', 'en',
			'Bob', 'Sample', 'Bob Sample', '+33123456789', 'FR', 'Head of Coffee',
			'12 Rue du Café', 'Apt 4', '75011', 'Île-de-France',
			'gold', 149.5, $2::timestamptz, '{"plan":"pro","seats":3}'::jsonb,
			$2::timestamptz, $2::timestamptz, $2::timestamptz, $2::timestamptz
		)`, zapierSampleEmail, zapierSampleInstant)

	// contact.updated is suppressed unless one of the columns the trigger compares
	// actually changed, so this has to be a real change to a compared column.
	exec(`UPDATE contacts SET job_title = 'VP of Coffee', updated_at = $2::timestamptz, db_updated_at = $2::timestamptz
		WHERE email = $1`, zapierSampleEmail, zapierSampleInstant)

	exec(`INSERT INTO lists (id, name, is_double_optin, is_public, created_at, updated_at)
		VALUES ($1, $2, true, false, $3::timestamptz, $3::timestamptz)`,
		zapierSampleListID, zapierSampleListName, zapierSampleInstant)

	setListStatus := func(status string) {
		t.Helper()
		exec(`UPDATE contact_lists SET status = $3, updated_at = $4::timestamptz
			WHERE email = $1 AND list_id = $2`,
			zapierSampleEmail, zapierSampleListID, status, zapierSampleInstant)
	}

	// list.subscribed fires on the INSERT of an active membership.
	exec(`INSERT INTO contact_lists (email, list_id, status, created_at, updated_at)
		VALUES ($1, $2, 'active', $3::timestamptz, $3::timestamptz)`,
		zapierSampleEmail, zapierSampleListID, zapierSampleInstant)

	// active -> pending matches none of the trigger's transitions and emits
	// nothing; it exists only to set up the pending -> active move below, which is
	// the single transition that produces list.confirmed.
	setListStatus("pending")
	setListStatus("active")

	setListStatus("unsubscribed")
	// unsubscribed -> active is list.resubscribed, not a second list.subscribed:
	// a returning contact never re-enters through the INSERT branch, which is why
	// a Zap bound to list.subscribed alone misses every re-subscriber.
	setListStatus("active")

	exec(`INSERT INTO segments (id, name, color, tree, timezone, version, status, db_created_at, db_updated_at)
		VALUES ($1, $2, '#4F46E5', '{}'::jsonb, 'UTC', 1, 'active', $3::timestamptz, $3::timestamptz)`,
		zapierSampleSegmentID, zapierSampleSegmentName, zapierSampleInstant)

	exec(`INSERT INTO contact_segments (email, segment_id, version, matched_at, computed_at)
		VALUES ($1, $2, 1, $3::timestamptz, $3::timestamptz)`,
		zapierSampleEmail, zapierSampleSegmentID, zapierSampleInstant)

	// Leaving a segment is a DELETE of the membership row, so the payload is built
	// from OLD and there is no row left behind for any read endpoint to return.
	exec(`DELETE FROM contact_segments WHERE email = $1 AND segment_id = $2`,
		zapierSampleEmail, zapierSampleSegmentID)
}

// createZapierSubscription creates a webhook subscription through the public API
// and returns its id. listIDs and segmentIDs are sent only when non-empty, so a
// caller that wants no filter produces a request body with the keys absent —
// which is what an unfiltered subscription looks like on the wire.
func createZapierSubscription(t *testing.T, suite *testutil.IntegrationTestSuite, workspaceID, name, url string, eventTypes, listIDs, segmentIDs []string) string {
	t.Helper()

	body := map[string]interface{}{
		"workspace_id": workspaceID,
		"name":         name,
		"url":          url,
		"event_types":  eventTypes,
	}
	if len(listIDs) > 0 {
		body["list_ids"] = listIDs
	}
	if len(segmentIDs) > 0 {
		body["segment_ids"] = segmentIDs
	}

	resp, err := suite.APIClient.Post("/api/webhookSubscriptions.create", body)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusCreated, resp.StatusCode, "failed to create webhook subscription")

	var created struct {
		Subscription struct {
			ID string `json:"id"`
		} `json:"subscription"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))
	require.NotEmpty(t, created.Subscription.ID)

	return created.Subscription.ID
}

// readZapierDeliveryPayloads returns the `data` object each queued delivery
// carries, keyed by event type, and fails if one event type was queued twice.
//
// A duplicate matters as much as a missing one: the Zapier app treats each
// delivery as one trigger run, so a trigger that queues an event twice runs the
// user's Zap twice.
func readZapierDeliveryPayloads(t *testing.T, db *sql.DB, subscriptionID string) map[string]map[string]interface{} {
	t.Helper()

	rows, err := db.QueryContext(context.Background(),
		`SELECT id, event_type, payload FROM webhook_deliveries WHERE subscription_id = $1`, subscriptionID)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	payloads := make(map[string]map[string]interface{})
	for rows.Next() {
		var id, eventType string
		var raw []byte
		require.NoError(t, rows.Scan(&id, &eventType, &raw))
		require.NotEmpty(t, id, "every delivery row carries the id that becomes the envelope id")

		var payload map[string]interface{}
		require.NoError(t, json.Unmarshal(raw, &payload), "delivery payload for %s is not a JSON object", eventType)

		_, duplicate := payloads[eventType]
		require.False(t, duplicate, "%s was queued more than once for one subscription", eventType)
		payloads[eventType] = payload
	}
	require.NoError(t, rows.Err())

	return payloads
}

// sendZapierProbeDelivery pushes one delivery through the real send path to a
// local receiver and returns the envelope that arrived.
//
// Only the envelope keys are of interest here — the `data` this path builds is
// the console's Test-button stand-in, not a trigger-built payload, and must never
// be used as a sample source.
func sendZapierProbeDelivery(t *testing.T, suite *testutil.IntegrationTestSuite, workspaceID string) map[string]interface{} {
	t.Helper()

	received := make(chan []byte, 1)
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		received <- body
		w.WriteHeader(http.StatusOK)
	}))
	defer receiver.Close()

	suite.APIClient.SetWorkspaceID(workspaceID)
	subscriptionID := createZapierSubscription(t, suite, workspaceID, "Envelope shape probe",
		receiver.URL, []string{"contact.created"}, nil, nil)

	resp, err := suite.APIClient.Post("/api/webhookSubscriptions.test", map[string]interface{}{
		"workspace_id": workspaceID,
		"id":           subscriptionID,
		"event_type":   "contact.created",
	})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	select {
	case body := <-received:
		var envelope map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &envelope))
		return envelope
	case <-time.After(5 * time.Second):
		require.FailNow(t, "no webhook reached the local receiver")
		return nil
	}
}

// renderZapierSamplesFile turns the captured envelopes into the exact bytes of
// the checked-in file. Marshalling through map[string]interface{} sorts every
// object's keys, so the output depends on the payload's content and never on the
// order PostgreSQL happened to return columns in.
func renderZapierSamplesFile(t *testing.T, events map[string]interface{}) string {
	t.Helper()

	document := map[string]interface{}{
		"readme":    "Generated from real webhook deliveries by tests/integration/webhook_payload_samples_test.go — do not edit by hand. Each entry is the JSON body Notifuse POSTs to a subscribed endpoint for that event type. The envelope id, workspace_id and timestamp are placeholders: a real delivery carries a fresh uuid, the workspace the event happened in, and the moment it was sent. Regenerate with UPDATE_WEBHOOK_SAMPLES=1 make test-integration.",
		"generator": "tests/integration/webhook_payload_samples_test.go",
		"events":    events,
	}

	encoded, err := json.MarshalIndent(document, "", "  ")
	require.NoError(t, err)

	return string(encoded) + "\n"
}

// normalizeZapierSampleValue rewrites captured values that would otherwise differ
// between two runs of the same capture.
//
// Timestamps are the only such value left once the seed writes every column
// explicitly, and they differ for a reason worth spelling out: to_jsonb renders a
// timestamptz in the session's TimeZone, so the same instant reaches this test as
// "...T09:30:00+00:00" on a UTC connection and "...T10:30:00+01:00" on one that
// is not. Re-rendering in UTC keeps the checked-in file identical whatever the
// database's time zone happens to be, and RFC 3339 with an offset is the format
// Zapier expects a date field to arrive in. Sub-second precision is preserved:
// the file is a faithful capture of what a subscriber receives.
func normalizeZapierSampleValue(value interface{}) interface{} {
	return rewriteTimestamps(value, func(instant time.Time) string {
		return instant.UTC().Format(time.RFC3339Nano)
	})
}

// rewriteTimestamps walks a decoded JSON value and re-renders every string that
// parses as an RFC 3339 timestamp, leaving everything else untouched.
//
// The rule is deliberately "any string that parses" rather than a list of known
// timestamp keys: the payloads are built by trigger bodies no compiler reads, so
// a key list would silently stop covering a column the day one is added, and a
// stale entry in it would be invisible.
func rewriteTimestamps(value interface{}, render func(time.Time) string) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		rewritten := make(map[string]interface{}, len(typed))
		for key, inner := range typed {
			rewritten[key] = rewriteTimestamps(inner, render)
		}
		return rewritten
	case []interface{}:
		rewritten := make([]interface{}, len(typed))
		for i, inner := range typed {
			rewritten[i] = rewriteTimestamps(inner, render)
		}
		return rewritten
	case string:
		if instant, err := time.Parse(time.RFC3339, typed); err == nil {
			return render(instant)
		}
		return typed
	default:
		return value
	}
}

// sortedKeys returns the keys of any string-keyed map, sorted, so two maps can be
// compared on shape alone with a readable failure message.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
