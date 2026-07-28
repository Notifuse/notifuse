package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/crypto"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// TestLinkClickMailpitE2E exercises the full per-link click pipeline with a
// real email: a broadcast is sent through the SMTP provider to Mailpit, the
// delivered message is fetched from the Mailpit API, the rewritten /r/
// tracking links are extracted from its HTML and clicked against the running
// server, and the resulting clicked_links counters, timeline rows, and
// broadcastLinkStats aggregation are verified. Unlike TestLinkClickStats
// (which crafts tokens directly), this proves the send path actually rewrites
// links and that tokens minted at compile time round-trip through the click
// handler into the database.
func TestLinkClickMailpitE2E(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	baseURL := suite.ServerManager.GetURL()
	client := suite.APIClient
	factory := suite.DataFactory
	ctx := context.Background()

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))
	require.NoError(t, factory.EnableEmailTracking(workspace.ID))

	_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID,
		testutil.WithIntegrationEmailProvider(domain.EmailProvider{
			Kind: domain.EmailProviderKindSMTP,
			Senders: []domain.EmailSender{
				domain.NewEmailSender("noreply@notifuse.test", "Link Click E2E"),
			},
			SMTP: &domain.SMTPSettings{
				Host:     "localhost",
				Port:     1025,
				Username: "",
				Password: "",
				UseTLS:   false,
			},
			RateLimitPerMinute: 2000,
		}))
	require.NoError(t, err)

	workerCtx, cancelWorker := context.WithCancel(context.Background())
	defer cancelWorker()
	require.NoError(t, suite.ServerManager.StartBackgroundWorkers(workerCtx))
	time.Sleep(100 * time.Millisecond)

	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(workspace.ID)

	require.NoError(t, testutil.ClearMailpitMessages(t))

	list, err := factory.CreateList(workspace.ID,
		testutil.WithListName("Link Click E2E List"))
	require.NoError(t, err)

	contactEmail := fmt.Sprintf("clicker-%s@example.com", uuid.New().String()[:8])
	contact, err := factory.CreateContact(workspace.ID,
		testutil.WithContactEmail(contactEmail),
		testutil.WithContactName("Link", "Clicker"))
	require.NoError(t, err)

	_, err = factory.CreateContactList(workspace.ID,
		testutil.WithContactListEmail(contact.Email),
		testutil.WithContactListListID(list.ID),
		testutil.WithContactListStatus(domain.ContactListStatusActive))
	require.NoError(t, err)

	// Two distinct destinations; the offer URL appears twice in the email
	// (text link + button) to mirror a realistic multi-CTA layout.
	offerURL := "https://shop.example.com/offer?utm_source=newsletter"
	docsURL := "https://docs.example.com/getting-started"
	mjmlSource := fmt.Sprintf(`<mjml><mj-body><mj-section><mj-column>
		<mj-text>Check <a href="%s">our offer</a> and read the <a href="%s">docs</a>.</mj-text>
		<mj-button href="%s">Shop now</mj-button>
	</mj-column></mj-section></mj-body></mjml>`, offerURL, docsURL, offerURL)

	template, err := factory.CreateTemplate(workspace.ID,
		testutil.WithTemplateName("Link Click E2E Template"),
		testutil.WithTemplateSubject(fmt.Sprintf("Link click e2e %s", uuid.New().String()[:8])),
		testutil.WithCodeModeTemplate(mjmlSource))
	require.NoError(t, err)

	broadcast, err := factory.CreateBroadcast(workspace.ID,
		testutil.WithBroadcastName("Link Click E2E Broadcast"),
		testutil.WithBroadcastTemplateID(template.ID),
		testutil.WithBroadcastAudience(domain.AudienceSettings{
			List:                list.ID,
			ExcludeUnsubscribed: true,
		}))
	require.NoError(t, err)

	t.Log("Scheduling broadcast...")
	scheduleResp, err := client.ScheduleBroadcast(map[string]interface{}{
		"workspace_id": workspace.ID,
		"id":           broadcast.ID,
		"send_now":     true,
	})
	require.NoError(t, err)
	scheduleResp.Body.Close()

	_, err = testutil.WaitForBroadcastStatusWithExecution(t, client, broadcast.ID,
		[]string{"processed", "completed", "sent"}, 60*time.Second)
	require.NoError(t, err)

	t.Log("Fetching email from Mailpit...")
	email, err := waitForEmailByRecipientAddr(t, contactEmail, 30*time.Second)
	require.NoError(t, err, "the broadcast email must arrive in Mailpit")

	// The authored destinations must not appear as raw hrefs: every link is
	// expected to be rewritten through the encrypted /r/ redirect.
	assert.NotContains(t, email.HTML, `href="https://shop.example.com`,
		"offer link must be rewritten through the tracking redirect")
	assert.NotContains(t, email.HTML, `href="https://docs.example.com`,
		"docs link must be rewritten through the tracking redirect")
	assert.Contains(t, email.HTML, "/t/", "open-tracking pixel must be injected")

	// Extract the /r/{token} links and decrypt each token (instead of following
	// it, which would consume a click) to map destination URL -> click URL.
	linkRe := regexp.MustCompile(`href="([^"]*/r/([A-Za-z0-9_-]+))"`)
	matches := linkRe.FindAllStringSubmatch(email.HTML, -1)
	require.NotEmpty(t, matches, "email HTML must contain rewritten /r/ links")

	var messageID string
	var sentTs int64
	clickURLByDest := map[string]string{}
	for _, m := range matches {
		decrypted, err := crypto.DecryptTrackingToken(m[2])
		require.NoError(t, err)
		parts := strings.SplitN(decrypted, "\n", 4)
		require.Len(t, parts, 4)
		require.Equal(t, workspace.ID, parts[1], "token must carry the workspace ID")
		if messageID == "" {
			messageID = parts[0]
			sentTs, err = strconv.ParseInt(parts[2], 10, 64)
			require.NoError(t, err)
		} else {
			require.Equal(t, messageID, parts[0], "all links of one email share the message ID")
		}
		// Click via the test server regardless of the endpoint host baked into
		// the email, so the test does not depend on endpoint configuration.
		clickURLByDest[parts[3]] = fmt.Sprintf("%s/r/%s", baseURL, m[2])
	}
	// The send path appends utm_content=<templateID> to links that carry no
	// UTM parameters, and leaves links with author-set UTM params untouched —
	// per-link stats group by the URL as stored, so assert both behaviors.
	docsDest := fmt.Sprintf("%s?utm_content=%s", docsURL, template.ID)
	require.Contains(t, clickURLByDest, offerURL,
		"author-tagged link must keep its URL untouched")
	require.Contains(t, clickURLByDest, docsDest,
		"untagged link must gain the default utm_content")
	require.Len(t, clickURLByDest, 2, "exactly two distinct destinations expected")

	// Clicks within 7 seconds of the compile-time token timestamp are treated
	// as bot prefetch and not recorded — wait out the window.
	if wait := time.Until(time.Unix(sentTs, 0).Add(8 * time.Second)); wait > 0 {
		t.Logf("Waiting %s for the too-fast-click bot gate to pass...", wait.Round(time.Millisecond))
		time.Sleep(wait)
	}

	redirectClient := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	click := func(t *testing.T, dest string) {
		t.Helper()
		req, err := http.NewRequest(http.MethodGet, clickURLByDest[dest], nil)
		require.NoError(t, err)
		req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) Safari/605.1.15")
		resp, err := redirectClient.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		assert.Equal(t, http.StatusSeeOther, resp.StatusCode)
		assert.Equal(t, dest, resp.Header.Get("Location"), "redirect must point at the authored destination")
	}

	click(t, offerURL)
	time.Sleep(50 * time.Millisecond) // ensure last_at advances past first_at
	click(t, offerURL)
	click(t, docsDest)

	workspaceDB, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	t.Run("clicks from the real email are recorded in clicked_links", func(t *testing.T) {
		var broadcastID *string
		var clickedAt, openedAt *time.Time
		var clickedLinksJSON []byte
		err := workspaceDB.QueryRowContext(ctx,
			`SELECT broadcast_id, clicked_at, opened_at, clicked_links FROM message_history WHERE id = $1`,
			messageID).Scan(&broadcastID, &clickedAt, &openedAt, &clickedLinksJSON)
		require.NoError(t, err)

		require.NotNil(t, broadcastID)
		assert.Equal(t, broadcast.ID, *broadcastID, "message must belong to the sent broadcast")
		require.NotNil(t, clickedAt, "clicked_at must be set")
		require.NotNil(t, openedAt, "opened_at must be backfilled by the click")

		var clickedLinks map[string]clickedLinkEntry
		require.NoError(t, json.Unmarshal(clickedLinksJSON, &clickedLinks))
		require.Len(t, clickedLinks, 2)

		offer, ok := clickedLinks[offerURL]
		require.True(t, ok, "offer URL must be recorded under its authored destination")
		assert.Equal(t, 2, offer.Count)
		assert.True(t, offer.LastAt.After(offer.FirstAt), "repeat click must refresh last_at")

		docs, ok := clickedLinks[docsDest]
		require.True(t, ok, "docs URL must be recorded under its stored (utm-augmented) destination")
		assert.Equal(t, 1, docs.Count)
	})

	t.Run("triggers fire once despite three clicks", func(t *testing.T) {
		var clickRows, openRows int
		require.NoError(t, workspaceDB.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM contact_timeline WHERE entity_id = $1 AND kind = 'email.clicked'`,
			messageID).Scan(&clickRows))
		require.NoError(t, workspaceDB.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM contact_timeline WHERE entity_id = $1 AND kind = 'email.opened'`,
			messageID).Scan(&openRows))
		assert.Equal(t, 1, clickRows, "exactly one email.clicked timeline row despite 3 clicks")
		assert.Equal(t, 1, openRows, "exactly one email.opened timeline row from the backfill")
	})

	t.Run("broadcastLinkStats aggregates the real clicks", func(t *testing.T) {
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

		assert.Equal(t, offerURL, result.LinkStats[0].URL)
		assert.Equal(t, int64(2), result.LinkStats[0].TotalClicks)
		assert.Equal(t, int64(1), result.LinkStats[0].UniqueClicks)
		assert.Equal(t, docsDest, result.LinkStats[1].URL)
		assert.Equal(t, int64(1), result.LinkStats[1].TotalClicks)
		assert.Equal(t, int64(1), result.LinkStats[1].UniqueClicks)
	})
}
