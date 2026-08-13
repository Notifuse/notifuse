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

// TestBridgedGoalIsSegmentable is the end-to-end reason S2 exists.
//
// Before it, the bridge left custom_events.goal_type NULL, and the Custom Events
// Goal segment condition filters on goal_type in EVERY configuration — including
// its "All types" wildcard, which compiles to `ce.goal_type IS NOT NULL`
// (query_builder.go). So no web goal could match that condition, in any
// configuration, ever. A workspace could record thousands of web conversions and
// build no segment from them.
//
// The two assertions below are exactly the two predicates the query builder
// emits. They are checked against the real bridged row rather than against the
// segment API, so the test proves the DATA is matchable without coupling to how a
// segment tree happens to be built today.
func TestBridgedGoalIsSegmentable(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	workspace, err := suite.DataFactory.CreateWorkspace(func(w *domain.Workspace) {
		w.Settings.WebAnalytics = &domain.WebAnalyticsSettings{
			Enabled: true,
			Filters: domain.DefaultWebFilters(),
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

	beat := func(t *testing.T, sessionID string, actions []map[string]interface{}) {
		t.Helper()
		payload := map[string]interface{}{
			"workspace_id":       workspace.ID,
			"session_id":         sessionID,
			"tab_id":             1,
			"actions":            actions,
			"contact_email":      "buyer@example.com",
			"contact_email_hmac": domain.ComputeWebIdentifyHMAC("buyer@example.com", workspace.Settings.SecretKey),
			"attributes": map[string]interface{}{
				"landing_page": "https://shop.example.com/pricing",
				"user_agent":   "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 Chrome/126.0.0.0 Safari/537.36",
			},
			"created_at":  now.Add(-5 * time.Minute).UnixMilli(),
			"updated_at":  now.UnixMilli(),
			"sent_at":     now.UnixMilli(),
			"sdk_version": "1.0.0",
			"seq":         1,
		}
		body, err := json.Marshal(payload)
		require.NoError(t, err)
		decoded := waCloseAndDecode(t, waPostBeat(t, baseURL, body, nil))
		require.Equal(t, true, decoded["success"], "beat rejected: %v", decoded)
		buffer.FlushAll(ctx)
	}

	typedGoal := func(name, goalType string, tsMs int64) map[string]interface{} {
		return map[string]interface{}{
			"type": "goal", "name": name, "page_number": 1,
			"timestamp": tsMs, "value": 49.9, "path": "/checkout",
			"goal_type": goalType,
		}
	}

	t.Run("the declared type reaches web_goals and custom_events", func(t *testing.T) {
		sessionID := waUUIDv7At(now.Add(-4*time.Minute), 0xB1)
		beat(t, sessionID, []map[string]interface{}{
			waPageview("/checkout", 1, 1000, 10, now),
			typedGoal("purchase", "purchase", now.Add(-3*time.Minute).UnixMilli()),
		})

		var webGoalType string
		require.NoError(t, wsDB.QueryRow(
			`SELECT goal_type FROM web_goals WHERE goal_name = 'purchase'`).Scan(&webGoalType))
		assert.Equal(t, "purchase", webGoalType, "the analytics row carries what the site declared")

		var eventGoalType *string
		require.NoError(t, wsDB.QueryRow(
			`SELECT goal_type FROM custom_events WHERE email = 'buyer@example.com' AND event_name = 'purchase'`,
		).Scan(&eventGoalType))
		require.NotNil(t, eventGoalType, "a NULL here is invisible to every goal segment condition")
		assert.Equal(t, "purchase", *eventGoalType)
	})

	t.Run("the bridged goal matches the wildcard All types condition", func(t *testing.T) {
		// Verbatim the predicate the query builder emits for the wildcard.
		var matches int
		require.NoError(t, wsDB.QueryRow(`
			SELECT count(*) FROM custom_events ce
			WHERE ce.email = 'buyer@example.com' AND ce.goal_type IS NOT NULL`).Scan(&matches))
		assert.Positive(t, matches, "this is the assertion that was impossible before S2")
	})

	t.Run("the bridged goal matches its own declared type", func(t *testing.T) {
		var matches int
		require.NoError(t, wsDB.QueryRow(`
			SELECT count(*) FROM custom_events ce
			WHERE ce.email = 'buyer@example.com' AND ce.goal_type = $1`, "purchase").Scan(&matches))
		assert.Positive(t, matches)
	})

	t.Run("a goal declaring a type the server does not know is recorded as other", func(t *testing.T) {
		// The lenient-wire contract, end to end: a site pinned to a stale cached
		// bundle must not lose the conversion, it must lose only the precision.
		sessionID := waUUIDv7At(now.Add(-4*time.Minute), 0xB2)
		beat(t, sessionID, []map[string]interface{}{
			waPageview("/checkout", 1, 1000, 10, now),
			typedGoal("mystery", "not-a-real-type", now.Add(-2*time.Minute).UnixMilli()),
		})

		var goalType string
		require.NoError(t, wsDB.QueryRow(
			`SELECT goal_type FROM web_goals WHERE goal_name = 'mystery'`).Scan(&goalType))
		assert.Equal(t, domain.GoalTypeOther, goalType, "recorded, not rejected")
	})

	t.Run("a goal that declares no type at all is still segmentable", func(t *testing.T) {
		sessionID := waUUIDv7At(now.Add(-4*time.Minute), 0xB3)
		beat(t, sessionID, []map[string]interface{}{
			waPageview("/checkout", 1, 1000, 10, now),
			map[string]interface{}{
				"type": "goal", "name": "untyped", "page_number": 1,
				"timestamp": now.Add(-time.Minute).UnixMilli(), "path": "/checkout",
			},
		})

		var eventGoalType *string
		require.NoError(t, wsDB.QueryRow(
			`SELECT goal_type FROM custom_events WHERE email = 'buyer@example.com' AND event_name = 'untyped'`,
		).Scan(&eventGoalType))
		require.NotNil(t, eventGoalType, "NULL would make the conversion unsegmentable")
		assert.Equal(t, domain.GoalTypeOther, *eventGoalType)
	})
}
