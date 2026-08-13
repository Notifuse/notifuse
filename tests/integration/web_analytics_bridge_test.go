//go:build integration
// +build integration

package integration

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// TestWebAnalyticsBridgeExactlyOnce covers W5: a verified web goal becomes a
// custom_events row, which the database triggers turn into a contact_timeline
// entry, a segment-queue entry and any matching automation enrolment.
//
// "Exactly once" is the whole contract. The timeline trigger fires on INSERT and
// on UPDATE with no diffing, and every timeline row queues a segment
// recomputation — so a bridge that re-emits on each flush would amplify one
// conversion into a stream of duplicate timeline entries and repeated automation
// enrolments, which is both wrong and the failure mode that once froze the
// segment queue in production.
func TestWebAnalyticsBridgeExactlyOnce(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	workspace, err := suite.DataFactory.CreateWorkspace(func(w *domain.Workspace) {
		w.Settings.WebAnalytics = &domain.WebAnalyticsSettings{
			Enabled:              true,
			Filters:              domain.DefaultWebFilters(),
		}
	})
	require.NoError(t, err)

	_, err = suite.DataFactory.CreateContact(workspace.ID, func(c *domain.Contact) {
		c.Email = "buyer@example.com"
	})
	require.NoError(t, err)

	baseURL := suite.ServerManager.GetURL()
	buffer := suite.ServerManager.GetApp().GetWebAnalyticsBuffer()
	wsDB, err := suite.DBManager.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)
	now := time.Now().UTC()
	ctx := context.Background()

	// A subscriber for custom_event.created, installed before the first beat: the
	// fan-out trigger runs at INSERT time, so a subscription created later would
	// simply never see the bridged goals and the exclusion asserted at the end of
	// this test would hold for the wrong reason. No custom_event_filters, so
	// nothing but the source guard can narrow the match.
	_, err = wsDB.Exec(`
		INSERT INTO webhook_subscriptions (id, name, url, secret, settings, enabled)
		VALUES ($1, $2, $3, $4, $5::jsonb, true)`,
		"wa_bridge_sub", "custom events subscriber", "https://hooks.example.com/custom-events",
		"wa_bridge_sub_secret", `{"event_types": ["custom_event.created"]}`)
	require.NoError(t, err)

	beat := func(t *testing.T, sessionID, email string, seq int64, actions []map[string]interface{}) {
		t.Helper()
		payload := map[string]interface{}{
			"workspace_id": workspace.ID,
			"session_id":   sessionID,
			"tab_id":       7,
			"actions":      actions,
			"attributes": map[string]interface{}{
				"landing_page": "https://shop.example.com/pricing",
				"user_agent":   "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 Chrome/126.0.0.0 Safari/537.36",
			},
			"created_at":  now.Add(-5 * time.Minute).UnixMilli(),
			"updated_at":  now.UnixMilli(),
			"sent_at":     now.UnixMilli(),
			"sdk_version": "1.0.0",
			"seq":         seq,
		}
		if email != "" {
			payload["contact_email"] = email
			payload["contact_email_hmac"] = domain.ComputeWebIdentifyHMAC(email, workspace.Settings.SecretKey)
		}
		body, err := json.Marshal(payload)
		require.NoError(t, err)
		decoded := waCloseAndDecode(t, waPostBeat(t, baseURL, body, nil))
		require.Equal(t, true, decoded["success"], "beat rejected: %v", decoded)
		buffer.FlushAll(ctx)
	}

	goalAction := func(name string, tsMs int64) map[string]interface{} {
		return map[string]interface{}{
			"type": "goal", "name": name, "page_number": 1,
			"timestamp": tsMs, "value": 49.9, "path": "/checkout",
			"properties": map[string]string{"plan": "pro"},
		}
	}

	counts := func(t *testing.T, email, eventName string) (events, timeline, queue int) {
		t.Helper()
		require.NoError(t, wsDB.QueryRow(
			`SELECT count(*) FROM custom_events WHERE email = $1 AND event_name = $2`,
			email, eventName).Scan(&events))
		require.NoError(t, wsDB.QueryRow(
			`SELECT count(*) FROM contact_timeline WHERE email = $1 AND kind = $2`,
			email, "custom_event."+eventName).Scan(&timeline))
		require.NoError(t, wsDB.QueryRow(
			`SELECT count(*) FROM contact_segment_queue WHERE email = $1`, email).Scan(&queue))
		return
	}

	t.Run("an anonymous goal is not bridged", func(t *testing.T) {
		sessionID := waUUIDv7At(now.Add(-4*time.Minute), 0xA1)
		beat(t, sessionID, "", 1, []map[string]interface{}{
			waPageview("/pricing", 1, 1000, 10, now),
			goalAction("anon_purchase", now.Add(-3*time.Minute).UnixMilli()),
		})
		var events int
		require.NoError(t, wsDB.QueryRow(
			`SELECT count(*) FROM custom_events WHERE event_name = 'anon_purchase'`).Scan(&events))
		assert.Equal(t, 0, events, "no identity means nothing to attach the goal to")
	})

	t.Run("a goal for a known contact lands once in every downstream surface", func(t *testing.T) {
		sessionID := waUUIDv7At(now.Add(-4*time.Minute), 0xA2)
		ts := now.Add(-3 * time.Minute).UnixMilli()
		beat(t, sessionID, "buyer@example.com", 1, []map[string]interface{}{
			waPageview("/pricing", 1, 1000, 10, now),
			goalAction("purchase", ts),
		})

		events, timeline, queue := counts(t, "buyer@example.com", "purchase")
		assert.Equal(t, 1, events)
		assert.Equal(t, 1, timeline)
		assert.GreaterOrEqual(t, queue, 1, "the contact is queued for segment recomputation")

		var value *float64
		var source string
		require.NoError(t, wsDB.QueryRow(
			`SELECT goal_value, source FROM custom_events WHERE email = $1 AND event_name = 'purchase'`,
			"buyer@example.com").Scan(&value, &source))
		require.NotNil(t, value)
		assert.InDelta(t, 49.9, *value, 0.01)
		assert.Equal(t, "web_analytics", source, "the source distinguishes bridged goals from API ones")
	})

	t.Run("re-beating the same session does not duplicate anything", func(t *testing.T) {
		// The SDK re-sends its entire cumulative action list on every beat, so
		// this is the normal case, not an edge one.
		sessionID := waUUIDv7At(now.Add(-4*time.Minute), 0xA2)
		ts := now.Add(-3 * time.Minute).UnixMilli()
		for seq := int64(2); seq <= 4; seq++ {
			beat(t, sessionID, "buyer@example.com", seq, []map[string]interface{}{
				waPageview("/pricing", 1, 1000, 10, now),
				waPageview("/checkout", 2, 2000, 20, now),
				goalAction("purchase", ts),
			})
		}

		events, timeline, _ := counts(t, "buyer@example.com", "purchase")
		assert.Equal(t, 1, events)
		assert.Equal(t, 1, timeline, "a repeated goal must not re-fire the timeline trigger")
	})

	t.Run("a goal fired after identification is still bridged", func(t *testing.T) {
		sessionID := waUUIDv7At(now.Add(-4*time.Minute), 0xA3)
		beat(t, sessionID, "buyer@example.com", 1, []map[string]interface{}{
			waPageview("/pricing", 1, 1000, 10, now),
		})
		beat(t, sessionID, "buyer@example.com", 2, []map[string]interface{}{
			waPageview("/pricing", 1, 1000, 10, now),
			goalAction("signup", now.Add(-2*time.Minute).UnixMilli()),
		})

		events, timeline, _ := counts(t, "buyer@example.com", "signup")
		assert.Equal(t, 1, events)
		assert.Equal(t, 1, timeline)
	})

	t.Run("a goal fired BEFORE identification is bridged once identity arrives", func(t *testing.T) {
		// The case retroactive identification exists to serve: someone converts,
		// then logs in. The SDK re-sends its whole cumulative action list, so the
		// goal is still on the wire — but the bridge saw it while anonymous and
		// had nothing to attach it to. Marking it emitted at selection time
		// rather than after a successful write silently loses it forever.
		sessionID := waUUIDv7At(now.Add(-4*time.Minute), 0xA4)
		ts := now.Add(-3 * time.Minute).UnixMilli()

		beat(t, sessionID, "", 1, []map[string]interface{}{
			waPageview("/pricing", 1, 1000, 10, now),
			goalAction("late_signup", ts),
		})
		events, _, _ := counts(t, "buyer@example.com", "late_signup")
		require.Equal(t, 0, events, "nothing to attach an anonymous goal to yet")

		beat(t, sessionID, "buyer@example.com", 2, []map[string]interface{}{
			waPageview("/pricing", 1, 1000, 10, now),
			goalAction("late_signup", ts),
		})

		events, timeline, _ := counts(t, "buyer@example.com", "late_signup")
		assert.Equal(t, 1, events, "the pre-identification goal must land once identity arrives")
		assert.Equal(t, 1, timeline)
	})

	t.Run("bridged goals do not fan out to webhook subscribers", func(t *testing.T) {
		// Scoped to bridged goals rather than counting every delivery: the four
		// other webhook triggers write to the same table, so an unfiltered zero
		// would turn any future fixture that legitimately delivers something into
		// a failure blaming the source guard.
		var bridgedDeliveries int
		require.NoError(t, wsDB.QueryRow(
			`SELECT count(*) FROM webhook_deliveries WHERE payload->'custom_event'->>'source' = 'web_analytics'`).
			Scan(&bridgedDeliveries))
		assert.Equal(t, 0, bridgedDeliveries,
			"exclusion half: web goals are a first-party analytics artifact; subscribers asked for API events")

		// Control. A count of zero is only evidence if the very same fixture can
		// produce a non-zero count — with a subscription that never matches (wrong
		// event_types, disabled, absent) the loop iterates zero times and the
		// assertion above passes even with the source guard deleted from the
		// trigger. So feed the subscription an API-sourced twin of the goal already
		// bridged above and require it to be delivered. It runs after the
		// zero-check precisely so it cannot contaminate it.
		const controlExternalID = "api-purchase-control"
		_, err := wsDB.Exec(`
			INSERT INTO custom_events (event_name, external_id, email, occurred_at, source, goal_value)
			VALUES ($1, $2, $3, $4, 'api', 49.9)`,
			"purchase", controlExternalID, "buyer@example.com", now)
		require.NoError(t, err)

		// Matched on the control's own key, so a failure names the delivery that is
		// actually missing instead of whatever else the table happens to hold.
		var apiDeliveries int
		require.NoError(t, wsDB.QueryRow(
			`SELECT count(*) FROM webhook_deliveries WHERE payload->'custom_event'->>'external_id' = $1`,
			controlExternalID).Scan(&apiDeliveries))
		require.Equal(t, 1, apiDeliveries,
			"control half: the API event must reach the subscription, otherwise the zero above proves nothing")

		var deliveredSource, deliveredSubscription string
		require.NoError(t, wsDB.QueryRow(`
			SELECT payload->'custom_event'->>'source', subscription_id
			FROM webhook_deliveries WHERE payload->'custom_event'->>'external_id' = $1`,
			controlExternalID).Scan(&deliveredSource, &deliveredSubscription))
		assert.Equal(t, "api", deliveredSource,
			"control half: the delivered payload must be the API twin, not a bridged goal")
		assert.Equal(t, "wa_bridge_sub", deliveredSubscription,
			"control half: it must reach the subscription installed before the first beat")
	})
}
