package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/pkg/crypto"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// clickedLinkEntry mirrors the per-URL JSONB value stored in message_history.clicked_links
type clickedLinkEntry struct {
	Count   int       `json:"count"`
	FirstAt time.Time `json:"first_at"`
	LastAt  time.Time `json:"last_at"`
}

// TestLinkClickStats exercises per-link click tracking end to end: real /r/
// tokens are followed against the running server, the clicked_links JSONB is
// inspected, the broadcastLinkStats API is queried, and the row-level triggers
// are verified to fire only on the clicked_at transition (repeat clicks add
// no timeline rows or webhook deliveries).
func TestLinkClickStats(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	baseURL := suite.ServerManager.GetURL()
	client := suite.APIClient
	factory := suite.DataFactory
	ctx := context.Background()

	// Create test user and workspace
	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	err = factory.AddUserToWorkspace(user.ID, workspace.ID, "owner")
	require.NoError(t, err)

	err = client.Login(user.Email, "password")
	require.NoError(t, err)
	client.SetWorkspaceID(workspace.ID)

	// Subscribe to email.clicked webhooks so the trigger produces deliveries
	createSubResp, err := client.Post("/api/webhookSubscriptions.create", map[string]interface{}{
		"workspace_id": workspace.ID,
		"name":         "Click tracking subscription",
		"url":          "https://example.com/webhook",
		"event_types":  []string{"email.clicked"},
	})
	require.NoError(t, err)
	defer func() { _ = createSubResp.Body.Close() }()
	require.Equal(t, http.StatusCreated, createSubResp.StatusCode)

	// Create a broadcast-bound message that has not been opened or clicked yet
	contact, err := factory.CreateContact(workspace.ID)
	require.NoError(t, err)
	template, err := factory.CreateTemplate(workspace.ID)
	require.NoError(t, err)
	broadcast, err := factory.CreateBroadcast(workspace.ID)
	require.NoError(t, err)
	message, err := factory.CreateMessageHistory(workspace.ID,
		testutil.WithMessageContact(contact.Email),
		testutil.WithMessageTemplate(template.ID),
		testutil.WithMessageBroadcast(broadcast.ID))
	require.NoError(t, err)

	urlA := "https://shop.example.com/products/sneakers?utm_source=newsletter"
	urlB := "https://shop.example.com/pricing"

	// Old enough to pass the too-fast-click bot gate
	tokenTimestamp := strconv.FormatInt(time.Now().Add(-30*time.Second).Unix(), 10)

	redirectClient := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	clickLink := func(t *testing.T, destinationURL string) {
		t.Helper()

		token, err := crypto.EncryptTrackingToken(fmt.Sprintf("%s\n%s\n%s\n%s",
			message.ID, workspace.ID, tokenTimestamp, destinationURL))
		require.NoError(t, err)

		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/r/%s", baseURL, token), nil)
		require.NoError(t, err)
		req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) Safari/605.1.15")

		resp, err := redirectClient.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusSeeOther, resp.StatusCode)
		assert.Equal(t, destinationURL, resp.Header.Get("Location"))
	}

	// Two clicks on URL A (the second one is a repeat), one click on URL B
	clickLink(t, urlA)
	time.Sleep(50 * time.Millisecond) // ensure last_at advances past first_at
	clickLink(t, urlA)
	clickLink(t, urlB)

	workspaceDB, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	t.Run("clicked_links JSONB records per-URL counters", func(t *testing.T) {
		var clickedAt, openedAt *time.Time
		var clickedLinksJSON []byte
		err := workspaceDB.QueryRowContext(ctx,
			`SELECT clicked_at, opened_at, clicked_links FROM message_history WHERE id = $1`,
			message.ID).Scan(&clickedAt, &openedAt, &clickedLinksJSON)
		require.NoError(t, err)

		require.NotNil(t, clickedAt, "clicked_at must be set")
		require.NotNil(t, openedAt, "opened_at must be backfilled by the click")

		var clickedLinks map[string]clickedLinkEntry
		require.NoError(t, json.Unmarshal(clickedLinksJSON, &clickedLinks))
		require.Len(t, clickedLinks, 2)

		entryA, ok := clickedLinks[urlA]
		require.True(t, ok, "URL A must be recorded")
		assert.Equal(t, 2, entryA.Count)
		assert.False(t, entryA.FirstAt.After(entryA.LastAt), "first_at must be <= last_at")
		assert.True(t, entryA.LastAt.After(entryA.FirstAt), "repeat click must refresh last_at")

		entryB, ok := clickedLinks[urlB]
		require.True(t, ok, "URL B must be recorded")
		assert.Equal(t, 1, entryB.Count)
		assert.False(t, entryB.FirstAt.After(entryB.LastAt), "first_at must be <= last_at")
	})

	t.Run("broadcastLinkStats returns aggregated per-URL stats", func(t *testing.T) {
		resp, err := client.Get("/api/messages.broadcastLinkStats", map[string]string{
			"workspace_id": workspace.ID,
			"broadcast_id": broadcast.ID,
		})
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		require.Equal(t, http.StatusOK, resp.StatusCode)

		var result struct {
			LinkStats []struct {
				URL          string `json:"url"`
				TotalClicks  int64  `json:"total_clicks"`
				UniqueClicks int64  `json:"unique_clicks"`
			} `json:"link_stats"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))

		require.Len(t, result.LinkStats, 2)
		// Ordered by unique clicks descending, then total clicks (both URLs tie at 1
		// unique here, so urlA's higher total puts it first).
		assert.Equal(t, urlA, result.LinkStats[0].URL)
		assert.Equal(t, int64(2), result.LinkStats[0].TotalClicks)
		assert.Equal(t, int64(1), result.LinkStats[0].UniqueClicks)
		assert.Equal(t, urlB, result.LinkStats[1].URL)
		assert.Equal(t, int64(1), result.LinkStats[1].TotalClicks)
		assert.Equal(t, int64(1), result.LinkStats[1].UniqueClicks)
	})

	t.Run("repeat clicks add no timeline rows", func(t *testing.T) {
		events, err := factory.GetContactTimelineEvents(workspace.ID, contact.Email, "email.clicked")
		require.NoError(t, err)
		assert.Len(t, events, 1, "only the clicked_at transition inserts a timeline row")
	})

	t.Run("repeat clicks add no webhook deliveries", func(t *testing.T) {
		var deliveries int
		err := workspaceDB.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM webhook_deliveries WHERE event_type = 'email.clicked'`).Scan(&deliveries)
		require.NoError(t, err)
		assert.Equal(t, 1, deliveries, "only the clicked_at transition produces a webhook delivery")
	})

	// Runs last: its extra clicks create more email.clicked deliveries and timeline rows,
	// which would break the workspace-wide count assertions above.
	t.Run("broadcastLinkStats orders by unique clicks, not total clicks", func(t *testing.T) {
		// A fresh broadcast so these counts are isolated from the urlA/urlB fixture above.
		abBroadcast, err := factory.CreateBroadcast(workspace.ID)
		require.NoError(t, err)

		newMessage := func(t *testing.T) string {
			t.Helper()
			c, err := factory.CreateContact(workspace.ID)
			require.NoError(t, err)
			m, err := factory.CreateMessageHistory(workspace.ID,
				testutil.WithMessageContact(c.Email),
				testutil.WithMessageTemplate(template.ID),
				testutil.WithMessageBroadcast(abBroadcast.ID))
			require.NoError(t, err)
			return m.ID
		}

		clickAs := func(t *testing.T, msgID, destinationURL string) {
			t.Helper()
			token, err := crypto.EncryptTrackingToken(fmt.Sprintf("%s\n%s\n%s\n%s",
				msgID, workspace.ID, tokenTimestamp, destinationURL))
			require.NoError(t, err)
			req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/r/%s", baseURL, token), nil)
			require.NoError(t, err)
			req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) Safari/605.1.15")
			resp, err := redirectClient.Do(req)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()
			require.Equal(t, http.StatusSeeOther, resp.StatusCode)
		}

		// urlHigh: clicked once by each of two messages → unique 2, total 2.
		// urlLow:  clicked three times by a single message → unique 1, total 3.
		urlHigh := "https://shop.example.com/winner"
		urlLow := "https://shop.example.com/runner-up"

		msg1 := newMessage(t)
		msg2 := newMessage(t)
		clickAs(t, msg1, urlHigh)
		clickAs(t, msg2, urlHigh)
		clickAs(t, msg1, urlLow)
		clickAs(t, msg1, urlLow)
		clickAs(t, msg1, urlLow)

		resp, err := client.Get("/api/messages.broadcastLinkStats", map[string]string{
			"workspace_id": workspace.ID,
			"broadcast_id": abBroadcast.ID,
		})
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var result struct {
			LinkStats []struct {
				URL          string `json:"url"`
				TotalClicks  int64  `json:"total_clicks"`
				UniqueClicks int64  `json:"unique_clicks"`
			} `json:"link_stats"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
		require.Len(t, result.LinkStats, 2)

		// unique_clicks is the primary sort key: urlHigh (2 unique) ranks above urlLow
		// (1 unique) even though urlLow has MORE total clicks (3 > 2). The old
		// total-clicks-first ordering would have flipped these two rows.
		assert.Equal(t, urlHigh, result.LinkStats[0].URL)
		assert.Equal(t, int64(2), result.LinkStats[0].UniqueClicks)
		assert.Equal(t, int64(2), result.LinkStats[0].TotalClicks)
		assert.Equal(t, urlLow, result.LinkStats[1].URL)
		assert.Equal(t, int64(1), result.LinkStats[1].UniqueClicks)
		assert.Equal(t, int64(3), result.LinkStats[1].TotalClicks)
	})
}
