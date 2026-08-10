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

// TestWebAnalyticsIngestInvariants locks down the ingest rules that decide
// whether stored data is correct rather than merely present. Each subtest
// corresponds to a business rule that silently corrupts reporting if it
// regresses, and each was chosen because it survives naive assertions:
// symmetric fixtures, or fixtures where the right and wrong answers coincide.
func TestWebAnalyticsIngestInvariants(t *testing.T) {
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

	baseURL := suite.ServerManager.GetURL()
	buffer := suite.ServerManager.GetApp().GetWebAnalyticsBuffer()
	wsDB, err := suite.DBManager.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	now := time.Now().UTC()

	// sendBeat posts a beat with full control over sent_at (which drives
	// clock-skew correction) and the client-side created_at.
	sendBeat := func(t *testing.T, sessionID string, seq int64, clientNow time.Time, sentAt time.Time, actions []map[string]interface{}) {
		t.Helper()
		payload := map[string]interface{}{
			"workspace_id": workspace.ID,
			"session_id":   sessionID,
			"actions":      actions,
			"attributes": map[string]interface{}{
				"landing_page": "https://shop.example.com/landing",
				"user_agent":   "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 Chrome/126.0.0.0 Safari/537.36",
			},
			"created_at":  clientNow.Add(-2 * time.Minute).UnixMilli(),
			"updated_at":  clientNow.UnixMilli(),
			"sent_at":     sentAt.UnixMilli(),
			"sdk_version": "1.0.0",
			"seq":         seq,
		}
		body, err := json.Marshal(payload)
		require.NoError(t, err)
		resp := waPostBeat(t, baseURL, body, nil)
		decoded := waCloseAndDecode(t, resp)
		require.Equal(t, true, decoded["success"], "beat rejected: %v", decoded)
		buffer.FlushAll(context.Background())
	}

	t.Run("clock skew shifts stored timestamps onto server time", func(t *testing.T) {
		// A visitor whose device clock runs 10 minutes slow. sent_at is stamped
		// with that wrong clock, so the server must shift every client
		// timestamp forward by the observed skew — otherwise this session
		// lands 10 minutes in the past and drops out of "last 30 minutes".
		clientNow := now.Add(-10 * time.Minute)
		// The id is minted by the same slow clock as the payload timestamps.
		sessionID := waUUIDv7At(clientNow.Add(-3*time.Minute), 0x41)

		sendBeat(t, sessionID, 1, clientNow, clientNow, []map[string]interface{}{
			waPageview("/skewed", 1, 1000, 10, clientNow),
		})

		var createdAt time.Time
		require.NoError(t, wsDB.QueryRow(`SELECT created_at FROM web_sessions WHERE id = $1`, sessionID).Scan(&createdAt))

		// Uncorrected the row would sit ~12 minutes back (10 min skew + the
		// 2 min the payload's created_at trails the client clock).
		driftFromNow := now.Sub(createdAt.UTC())
		assert.Less(t, driftFromNow, 5*time.Minute,
			"created_at must be corrected onto server time, got %s of drift", driftFromNow)
		assert.Greater(t, driftFromNow, 30*time.Second,
			"the payload's own 2-minute offset must be preserved, not zeroed")
	})

	t.Run("created_at is set once; later beats never move the session start", func(t *testing.T) {
		sessionID := waUUIDv7At(now.Add(-4*time.Minute), 0x42)

		sendBeat(t, sessionID, 1, now.Add(-4*time.Minute), now.Add(-4*time.Minute), []map[string]interface{}{
			waPageview("/first", 1, 1000, 10, now.Add(-4*time.Minute)),
		})
		var firstCreatedAt time.Time
		require.NoError(t, wsDB.QueryRow(`SELECT created_at FROM web_sessions WHERE id = $1`, sessionID).Scan(&firstCreatedAt))

		// A later beat legitimately carries a much newer created_at (the SDK
		// re-sends session state, and clocks move). The stored session start
		// must stay put or every session's start silently drifts forward.
		// Deliberately a very different skew: the session start is derived from
		// the id and skew-corrected, so a beat correcting by ~20 minutes more
		// would visibly move created_at if it were not excluded from the upsert.
		sendBeat(t, sessionID, 2, now, now.Add(-20*time.Minute), []map[string]interface{}{
			waPageview("/first", 1, 1000, 10, now),
			waPageview("/second", 2, 2000, 20, now),
		})

		var secondCreatedAt, updatedAt time.Time
		require.NoError(t, wsDB.QueryRow(
			`SELECT created_at, updated_at FROM web_sessions WHERE id = $1`, sessionID).
			Scan(&secondCreatedAt, &updatedAt))

		assert.WithinDuration(t, firstCreatedAt, secondCreatedAt, time.Millisecond,
			"created_at must be immutable after the first beat")
		assert.True(t, updatedAt.After(firstCreatedAt),
			"updated_at must still advance with each beat")
	})

	t.Run("goal dedup survives a retry sent under different clock skew", func(t *testing.T) {
		// The offline queue replays a beat later, so sent_at differs and the
		// skew correction differs with it. The goal's dedup key is the
		// ORIGINAL client timestamp precisely so the replay updates the same
		// row instead of inventing a second conversion.
		sessionID := waUUIDv7At(now.Add(-5*time.Minute), 0x43)
		clientNow := now.Add(-5 * time.Minute)
		goalTs := clientNow.UnixMilli()

		goalAction := map[string]interface{}{
			"type": "goal", "name": "purchase", "path": "/checkout", "page_number": 1,
			"timestamp": goalTs, "value": 99.0,
		}
		actions := []map[string]interface{}{
			waPageview("/checkout", 1, 1000, 10, clientNow),
			goalAction,
		}

		sendBeat(t, sessionID, 1, clientNow, clientNow, actions)
		// Same goal, replayed with a wildly different sent_at (large skew).
		sendBeat(t, sessionID, 2, clientNow, clientNow.Add(-45*time.Minute), actions)

		var goalCount int
		require.NoError(t, wsDB.QueryRow(
			`SELECT COUNT(*) FROM web_goals WHERE session_id = $1`, sessionID).Scan(&goalCount))
		assert.Equal(t, 1, goalCount,
			"a replayed goal must dedup onto one row regardless of clock skew")

		var sessionGoalCount int
		var goalValue float64
		require.NoError(t, wsDB.QueryRow(
			`SELECT goal_count, goal_value FROM web_sessions WHERE id = $1`, sessionID).
			Scan(&sessionGoalCount, &goalValue))
		assert.Equal(t, 1, sessionGoalCount)
		assert.InDelta(t, 99.0, goalValue, 0.01, "revenue must not be double counted")
	})

	t.Run("session_date comes from the session id, not the server clock", func(t *testing.T) {
		// A session that started ~44h ago and is still beating (offline replay).
		// Its rows must be routed to the partition of the session's own start
		// date; deriving it from the server clock would scatter one session
		// across partitions and break the pruning predicate that assumes
		// session_date tracks the id.
		sessionStart := now.Add(-44 * time.Hour)
		sessionID := waUUIDv7At(sessionStart, 0x44)

		sendBeat(t, sessionID, 1, now, now, []map[string]interface{}{
			waPageview("/late", 1, 1000, 10, now),
		})

		var sessionDate time.Time
		require.NoError(t, wsDB.QueryRow(
			`SELECT session_date FROM web_sessions WHERE id = $1`, sessionID).Scan(&sessionDate))

		expected := time.Date(sessionStart.Year(), sessionStart.Month(), sessionStart.Day(), 0, 0, 0, 0, time.UTC)
		assert.Equal(t, expected.Format("2006-01-02"), sessionDate.UTC().Format("2006-01-02"),
			"session_date must equal the UUIDv7 date, not today")
		assert.NotEqual(t, now.Format("2006-01-02"), sessionDate.UTC().Format("2006-01-02"),
			"fixture must actually straddle a day boundary to be meaningful")

		// The page rows must colocate with their session's partition.
		var pageDate time.Time
		require.NoError(t, wsDB.QueryRow(
			`SELECT session_date FROM web_pages WHERE session_id = $1`, sessionID).Scan(&pageDate))
		assert.Equal(t, sessionDate.UTC().Format("2006-01-02"), pageDate.UTC().Format("2006-01-02"))
	})

	t.Run("median page duration is a true median, not a mean", func(t *testing.T) {
		// Deliberately skewed durations: one long page would drag a mean far
		// above the median, so this fixture separates the two.
		sessionID := waUUIDv7At(now.Add(-6*time.Minute), 0x45)
		clientNow := now.Add(-6 * time.Minute)

		sendBeat(t, sessionID, 1, clientNow, clientNow, []map[string]interface{}{
			waPageview("/a", 1, 1000, 10, clientNow),
			waPageview("/b", 2, 3000, 20, clientNow),
			waPageview("/c", 3, 50000, 30, clientNow),
		})

		var median, total int64
		require.NoError(t, wsDB.QueryRow(
			`SELECT median_page_duration_ms, duration_ms FROM web_sessions WHERE id = $1`, sessionID).
			Scan(&median, &total))

		assert.Equal(t, int64(3000), median, "median of 1000/3000/50000 is 3000, the mean is 18000")
		assert.Equal(t, int64(54000), total, "duration is the SUM of page focus, not the max")
	})
}
