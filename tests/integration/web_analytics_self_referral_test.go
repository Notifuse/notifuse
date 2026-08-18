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

// TestWebAnalyticsSelfReferral proves a visit is never credited to the site it
// happened on.
//
// The SDK mints a new session id whenever the inactivity window has lapsed —
// rotating in place on a tab the visitor left open, or on their next internal
// click — and stamps it with document.referrer, which in both cases is one of
// the site's own pages. Stored as-is that puts the customer's own domain in
// their referrers report, drops the session out of Direct into not-mapped, and
// loses the real acquisition source that brought the visitor in hours earlier.
//
// The last subtest is the half only the full stack shows: attribution columns
// are sticky per column on the session row (first non-empty writer wins) while
// is_direct is not, so a self-referral beat arriving after a direct one could
// overwrite an empty referrer and flip is_direct along with it.
func TestWebAnalyticsSelfReferral(t *testing.T) {
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

	// Every beat lands on shop.example.com, so the referrer argument is what
	// decides whether it is a self-referral.
	sendBeat := func(t *testing.T, sessionID string, tabID, seq int64, referrer string, path string) {
		t.Helper()
		attrs := map[string]interface{}{
			"landing_page": "https://shop.example.com/landing",
			"user_agent":   "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 Chrome/126.0.0.0 Safari/537.36",
		}
		if referrer != "" {
			attrs["referrer"] = referrer
		}
		body, err := json.Marshal(map[string]interface{}{
			"workspace_id": workspace.ID,
			"session_id":   sessionID,
			"tab_id":       tabID,
			"actions":      []map[string]interface{}{waPageview(path, 1, 1000, 10, now)},
			"attributes":   attrs,
			"created_at":   now.Add(-2 * time.Minute).UnixMilli(),
			"updated_at":   now.UnixMilli(),
			"sent_at":      now.UnixMilli(),
			"sdk_version":  "1.0.0",
			"seq":          seq,
		})
		require.NoError(t, err)
		decoded := waCloseAndDecode(t, waPostBeat(t, baseURL, body, nil))
		require.Equal(t, true, decoded["success"], "beat rejected: %v", decoded)
		buffer.FlushAll(context.Background())
	}

	scanAttribution := func(t *testing.T, sessionID string) (referrer, refDomain, refPath, channel string, isDirect bool) {
		t.Helper()
		require.NoError(t, wsDB.QueryRow(
			`SELECT referrer, referrer_domain, referrer_path, channel, is_direct FROM web_sessions WHERE id = $1`,
			sessionID).Scan(&referrer, &refDomain, &refPath, &channel, &isDirect))
		return
	}

	t.Run("a referrer on the landing host is stored as direct", func(t *testing.T) {
		sessionID := waUUIDv7At(now.Add(-5*time.Minute), 0x51)
		// Mixed case: hosts are compared after both sides are lowercased.
		sendBeat(t, sessionID, 501, 1, "https://SHOP.example.com/compare/", "/pricing")

		referrer, refDomain, refPath, channel, isDirect := scanAttribution(t, sessionID)
		assert.Empty(t, referrer)
		assert.Empty(t, refDomain)
		assert.Empty(t, refPath)
		assert.True(t, isDirect)
		assert.Equal(t, "direct", channel, "the rules run on the corrected is_direct")
	})

	t.Run("another host of the same site stays a referral", func(t *testing.T) {
		sessionID := waUUIDv7At(now.Add(-5*time.Minute), 0x52)
		sendBeat(t, sessionID, 502, 1, "https://docs.shop.example.com/guide", "/pricing")

		referrer, refDomain, refPath, _, isDirect := scanAttribution(t, sessionID)
		assert.Equal(t, "https://docs.shop.example.com/guide", referrer)
		assert.Equal(t, "docs.shop.example.com", refDomain)
		assert.Equal(t, "/guide", refPath)
		assert.False(t, isDirect)
	})

	t.Run("a self-referral beat cannot overwrite a direct session", func(t *testing.T) {
		sessionID := waUUIDv7At(now.Add(-5*time.Minute), 0x53)
		// The visitor arrives with no referrer, then opens an internal link in
		// a second tab, which beats under the same session id.
		sendBeat(t, sessionID, 531, 1, "", "/landing")
		sendBeat(t, sessionID, 532, 1, "https://shop.example.com/landing", "/pricing")

		referrer, refDomain, _, channel, isDirect := scanAttribution(t, sessionID)
		assert.Empty(t, referrer, "the empty referrer of a direct session is not a slot to be filled")
		assert.Empty(t, refDomain)
		assert.True(t, isDirect)
		assert.Equal(t, "direct", channel)
	})
}
