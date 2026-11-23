//go:build integration
// +build integration

package integration

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/migrations"
	"github.com/Notifuse/notifuse/pkg/trackingtoken"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// TestDataRetentionEngagementClock drives each way a contact can engage, through the paths a
// real contact takes (the public form, the links of a delivered email, a provider's reply
// webhook, a tracked visit), and each thing that must not count: an open, a click on the
// unsubscribe link, a pending double opt-in, the workspace's own API call, an auto-reply. It
// then checks that deleting a contact deletes their engagement date, and that the backfill
// of an upgraded workspace reads engagement back from history and marks the workspace ready.
func TestDataRetentionEngagementClock(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	workerCtx, cancelWorkers := context.WithCancel(context.Background())
	defer cancelWorkers()
	require.NoError(t, suite.ServerManager.StartBackgroundWorkers(workerCtx))

	client := suite.APIClient
	factory := suite.DataFactory
	serverURL := suite.ServerManager.GetURL()
	bg := context.Background()

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace(func(w *domain.Workspace) {
		w.Settings.WebAnalytics = &domain.WebAnalyticsSettings{Enabled: true, Filters: domain.DefaultWebFilters()}
	})
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))
	require.NoError(t, factory.EnableEmailTracking(workspace.ID))
	_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID)
	require.NoError(t, err)
	require.NoError(t, factory.SetWorkspaceCustomEndpointURL(workspace.ID, serverURL))
	// A Mailgun integration for inbound replies (no signing key: the signature check is skipped).
	mailgun, err := factory.CreateIntegration(workspace.ID, testutil.WithIntegrationEmailProvider(domain.EmailProvider{
		Kind:               domain.EmailProviderKindMailgun,
		RateLimitPerMinute: 100,
		Senders:            []domain.EmailSender{domain.NewEmailSender("hello@example.com", "Hello")},
		Mailgun:            &domain.MailgunSettings{Domain: "example.com", Region: "US"},
	}))
	require.NoError(t, err)
	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(workspace.ID)
	db, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)
	// The test reads the workspace's secret key for the visit's identity signature.
	workspace, err = factory.GetWorkspace(workspace.ID)
	require.NoError(t, err)

	const dest = "https://shop.example.com/engaged"
	template, err := factory.CreateTemplate(workspace.ID,
		testutil.WithTemplateSubject("Confirm "+uuid.New().String()[:8]),
		testutil.WithCodeModeTemplate(`<mjml><mj-body><mj-section><mj-column>`+
			`<mj-text><a href="`+dest+`">Shop</a> <a href="{{ confirm_subscription_url }}">Confirm</a>`+
			` <a href="{{ unsubscribe_url }}">Unsubscribe</a></mj-text>`+
			`</mj-column></mj-section></mj-body></mjml>`))
	require.NoError(t, err)
	singleOptin, err := factory.CreateList(workspace.ID, testutil.WithListPublic(true), testutil.WithListDoubleOptin(false))
	require.NoError(t, err)
	doubleOptin, err := factory.CreateList(workspace.ID, testutil.WithListPublic(true), testutil.WithListDoubleOptin(true),
		func(l *domain.List) {
			l.DoubleOptInTemplate = &domain.TemplateReference{ID: template.ID, Version: template.Version}
		})
	require.NoError(t, err)
	require.NoError(t, testutil.ClearMailpitMessages(t))

	newEmail := func(name string) string {
		return fmt.Sprintf("engagement-%s-%s@example.com", name, uuid.New().String()[:8])
	}
	engagedAt := func(t *testing.T, email string) *time.Time {
		t.Helper()
		var at time.Time
		err := db.QueryRowContext(bg, `SELECT last_engaged_at FROM contact_engagement WHERE email = $1`, email).Scan(&at)
		if err == sql.ErrNoRows {
			return nil
		}
		require.NoError(t, err)
		return &at
	}
	// Some writes land after the response (the click handler, the visit buffer): poll.
	requireEngaged := func(t *testing.T, email string) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for engagedAt(t, email) == nil {
			require.True(t, time.Now().Before(deadline), "no engagement recorded for %s", email)
			time.Sleep(100 * time.Millisecond)
		}
	}
	requireNotEngaged := func(t *testing.T, email string) {
		t.Helper()
		time.Sleep(500 * time.Millisecond) // long enough for a write that should not happen
		assert.Nil(t, engagedAt(t, email), "%s must have no engagement date", email)
	}

	// The public form is rate limited per address: each call comes from its own.
	var forwardedFor atomic.Int32
	publicSubscribe := func(t *testing.T, listID string, contact map[string]interface{}) {
		t.Helper()
		body, err := json.Marshal(map[string]interface{}{"workspace_id": workspace.ID, "contact": contact, "list_ids": []string{listID}})
		require.NoError(t, err)
		req, err := http.NewRequest(http.MethodPost, serverURL+"/subscribe", bytes.NewReader(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Forwarded-For", fmt.Sprintf("203.0.113.%d", forwardedFor.Add(1)))
		resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
		require.NoError(t, err)
		raw, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode, "/subscribe: %s", raw)
	}
	// deliveredLink finds, in the delivered email, the tracked link whose destination starts
	// with prefix, and returns it with that destination.
	deliveredLink := func(t *testing.T, html, prefix string) (string, string) {
		t.Helper()
		for _, m := range regexp.MustCompile(`href="([^"]*/r/([A-Za-z0-9_-]+))"`).FindAllStringSubmatch(html, -1) {
			tok, err := trackingtoken.Parse(m[2], trackingtoken.PurposeClick)
			require.NoError(t, err)
			if strings.HasPrefix(tok.URL, prefix) {
				return m[1], tok.URL
			}
		}
		t.Fatalf("no tracked link to %s in the delivered email", prefix)
		return "", ""
	}
	listAction := func(t *testing.T, html, action string) (string, url.Values) {
		t.Helper()
		link, target := deliveredLink(t, html, serverURL+"/notification-center")
		for _, m := range regexp.MustCompile(`href="([^"]*/r/[A-Za-z0-9_-]+)"`).FindAllStringSubmatch(html, -1) {
			tok, err := trackingtoken.Parse(m[1][strings.LastIndex(m[1], "/")+1:], trackingtoken.PurposeClick)
			require.NoError(t, err)
			if u, err := url.Parse(tok.URL); err == nil && u.Query().Get("action") == action {
				return m[1], u.Query()
			}
		}
		t.Fatalf("no %s link in the delivered email (first notification center link: %s -> %s)", action, link, target)
		return "", nil
	}

	t.Run("the contact's own subscription on a single opt-in list counts", func(t *testing.T) {
		email := newEmail("form")
		publicSubscribe(t, singleOptin.ID, map[string]interface{}{"email": email})
		requireEngaged(t, email)
	})

	t.Run("the workspace's own subscription call does not count", func(t *testing.T) {
		email := newEmail("api")
		resp, err := client.Post("/api/lists.subscribe", map[string]interface{}{
			"workspace_id": workspace.ID, "contact": map[string]interface{}{"email": email}, "list_ids": []string{singleOptin.ID},
		})
		require.NoError(t, err)
		raw, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode, "%s", raw)
		requireNotEngaged(t, email)
	})

	t.Run("double opt-in emails: pending, an open and an unsubscribe click do not count; a confirmation and a real click do", func(t *testing.T) {
		confirming, clicking, refusing := newEmail("doi"), newEmail("click"), newEmail("refused")
		publicSubscribe(t, doubleOptin.ID, map[string]interface{}{"email": confirming})
		publicSubscribe(t, doubleOptin.ID, map[string]interface{}{"email": clicking})
		publicSubscribe(t, doubleOptin.ID, map[string]interface{}{"email": refusing})
		confirmingMsg, err := testutil.WaitForMailpitMessageByRecipient(t, confirming, 20*time.Second)
		require.NoError(t, err)
		clickingMsg, err := testutil.WaitForMailpitMessageByRecipient(t, clicking, 20*time.Second)
		require.NoError(t, err)
		refusingMsg, err := testutil.WaitForMailpitMessageByRecipient(t, refusing, 20*time.Second)
		require.NoError(t, err)
		requireNotEngaged(t, confirming)
		requireNotEngaged(t, clicking)
		requireNotEngaged(t, refusing)

		// The click handler drops an open or a click within 7 s of the send as a bot's
		// prefetch: wait it out, or every negative below would pass for that reason.
		time.Sleep(8 * time.Second)
		messageState := func(t *testing.T, email string) (opened, clicked bool) {
			t.Helper()
			require.NoError(t, db.QueryRowContext(bg, `SELECT opened_at IS NOT NULL, clicked_at IS NOT NULL FROM message_history
				WHERE contact_email = $1 ORDER BY created_at DESC LIMIT 1`, email).Scan(&opened, &clicked))
			return opened, clicked
		}

		// The open pixel: recorded as an open, not as engagement.
		pixel := regexp.MustCompile(`<img src="([^"]*/t/[A-Za-z0-9_-]+)"`).FindStringSubmatch(confirmingMsg.HTML)
		require.NotNil(t, pixel, "the delivered email has an open pixel")
		resp := clickDelivered(t, pixel[1])
		require.Less(t, resp.StatusCode, 400)
		opened, _ := messageState(t, confirming)
		require.True(t, opened, "the open was recorded")
		requireNotEngaged(t, confirming)

		// A click on the unsubscribe link: recorded as a click, but it leads back to the
		// instance, so it is not engagement.
		unsubscribeLink, _ := listAction(t, confirmingMsg.HTML, "unsubscribe")
		resp = clickDelivered(t, unsubscribeLink)
		require.Less(t, resp.StatusCode, 400)
		_, clicked := messageState(t, confirming)
		require.True(t, clicked, "the click was recorded")
		requireNotEngaged(t, confirming)

		// The confirmation, as the notification center sends it from the confirm link.
		_, confirm := listAction(t, confirmingMsg.HTML, "confirm")
		publicSubscribe(t, doubleOptin.ID, map[string]interface{}{"email": confirm.Get("email"), "email_hmac": confirm.Get("email_hmac")})
		var status string
		require.NoError(t, db.QueryRowContext(bg, `SELECT status FROM contact_lists WHERE email = $1 AND list_id = $2`, confirming, doubleOptin.ID).Scan(&status))
		require.Equal(t, "active", status)
		requireEngaged(t, confirming)

		// A click on a real destination.
		link, _ := deliveredLink(t, clickingMsg.HTML, dest)
		resp = clickDelivered(t, link)
		require.Less(t, resp.StatusCode, 400)
		requireEngaged(t, clicking)

		// The same click from a contact who refused click tracking after the send: the click
		// is not recorded, and engagement is not derived from it either.
		_, err = db.ExecContext(bg, `UPDATE contacts SET tracking_clicks_consent = 'denied' WHERE email = $1`, refusing)
		require.NoError(t, err)
		link, _ = deliveredLink(t, refusingMsg.HTML, dest)
		resp = clickDelivered(t, link)
		require.Less(t, resp.StatusCode, 400)
		_, clicked = messageState(t, refusing)
		require.False(t, clicked, "a refused click is not recorded")
		requireNotEngaged(t, refusing)
	})

	postReply := func(t *testing.T, email, extraHeaders string) {
		t.Helper()
		smtpID := fmt.Sprintf("send-%d@example.com", time.Now().UnixNano())
		_, err := factory.CreateMessageHistory(workspace.ID,
			testutil.WithMessageHistoryContactEmail(email),
			testutil.WithMessageHistorySMTPMessageID(smtpID),
			testutil.WithMessageHistoryTemplateID("tpl-reply"))
		require.NoError(t, err)
		form := url.Values{}
		form.Set("sender", email)
		form.Set("from", "Replier <"+email+">")
		form.Set("recipient", "hello@example.com")
		form.Set("subject", "Re: hello")
		form.Set("timestamp", fmt.Sprintf("%d", time.Now().Unix()))
		form.Set("message-headers", fmt.Sprintf(`[%s["In-Reply-To","<%s>"],["Message-Id","<reply-%d@x>"]]`, extraHeaders, smtpID, time.Now().UnixNano()))
		resp, err := http.Post(fmt.Sprintf("%s/webhooks/email/inbound?workspace_id=%s&integration_id=%s", serverURL, workspace.ID, mailgun.ID),
			"application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
		require.NoError(t, err)
		_ = resp.Body.Close()
		require.Less(t, resp.StatusCode, 300)
	}

	t.Run("a reply counts, an auto-reply does not", func(t *testing.T) {
		genuine, auto := newEmail("reply"), newEmail("autoreply")
		for _, email := range []string{genuine, auto} {
			_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
			require.NoError(t, err)
		}
		postReply(t, genuine, "")
		postReply(t, auto, `["Auto-Submitted","auto-replied"],`)
		requireEngaged(t, genuine)
		requireNotEngaged(t, auto)
	})

	t.Run("an identified visit counts", func(t *testing.T) {
		email := newEmail("visit")
		_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
		require.NoError(t, err)
		now := time.Now().UTC()
		body, err := json.Marshal(map[string]interface{}{
			"workspace_id":       workspace.ID,
			"session_id":         waUUIDv7At(now.Add(-2*time.Minute), 0xE1),
			"tab_id":             1,
			"contact_email":      email,
			"contact_email_hmac": domain.ComputeWebIdentifyHMAC(email, workspace.Settings.SecretKey),
			"actions":            []map[string]interface{}{waPageview("/pricing", 1, 1000, 10, now)},
			"attributes":         map[string]interface{}{"landing_page": "https://shop.example.com/pricing"},
			"created_at":         now.Add(-2 * time.Minute).UnixMilli(),
			"updated_at":         now.UnixMilli(),
			"sent_at":            now.UnixMilli(),
			"sdk_version":        "1.0.0",
			"seq":                1,
		})
		require.NoError(t, err)
		require.Equal(t, true, waCloseAndDecode(t, waPostBeat(t, serverURL, body, nil))["success"])
		suite.ServerManager.GetApp().GetWebAnalyticsBuffer().FlushAll(bg)
		requireEngaged(t, email)
	})

	t.Run("an engagement for an address with no contact creates nothing", func(t *testing.T) {
		email := newEmail("ghost")
		postReply(t, email, "")
		requireNotEngaged(t, email)
		var contacts int
		require.NoError(t, db.QueryRowContext(bg, `SELECT COUNT(*) FROM contacts WHERE email = $1`, email).Scan(&contacts))
		assert.Zero(t, contacts)
	})

	t.Run("deleting a contact deletes their engagement date", func(t *testing.T) {
		email := newEmail("deleted")
		publicSubscribe(t, singleOptin.ID, map[string]interface{}{"email": email})
		requireEngaged(t, email)
		resp, err := client.Post("/api/contacts.delete", map[string]interface{}{"workspace_id": workspace.ID, "email": email})
		require.NoError(t, err)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Nil(t, engagedAt(t, email))
	})

	t.Run("the backfill of an upgraded workspace reads history and marks it ready", func(t *testing.T) {
		clicked, replied, visited, confirmed, newer, silent :=
			newEmail("h-click"), newEmail("h-reply"), newEmail("h-visit"), newEmail("h-confirm"), newEmail("h-newer"), newEmail("h-silent")
		perLink, unsubscribeOnly, refused := newEmail("h-perlink"), newEmail("h-unsub"), newEmail("h-refused")
		for _, email := range []string{clicked, replied, visited, confirmed, newer, silent, perLink, unsubscribeOnly, refused} {
			_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
			require.NoError(t, err)
		}
		day := func(n int) time.Time { return time.Now().UTC().AddDate(0, 0, -n).Truncate(time.Second) }
		_, err := factory.CreateMessageHistory(workspace.ID, testutil.WithMessageHistoryContactEmail(clicked),
			testutil.WithMessageHistoryTemplateID("tpl-bf"))
		require.NoError(t, err)
		_, err = db.ExecContext(bg, `UPDATE message_history SET clicked_at = $2 WHERE contact_email = $1`, clicked, day(200))
		require.NoError(t, err)
		// Per-link clicks: the latest real destination counts, a later click on the
		// unsubscribe link does not; an email whose only clicks went to the unsubscribe link
		// gives nothing; a contact who refused click tracking gets nothing from their clicks.
		nc := serverURL + "/notification-center?action=unsubscribe&lid=x"
		link := func(at time.Time) map[string]interface{} {
			return map[string]interface{}{"count": 1, "first_at": at, "last_at": at, "verified": true}
		}
		for _, seed := range []struct {
			email     string
			clickedAt time.Time
			links     map[string]interface{}
		}{
			{perLink, day(160), map[string]interface{}{"https://shop.example.com/a": link(day(150)), nc: link(day(5))}},
			{unsubscribeOnly, day(20), map[string]interface{}{nc: link(day(20))}},
			{refused, day(40), map[string]interface{}{"https://shop.example.com/a": link(day(40))}},
		} {
			_, err := factory.CreateMessageHistory(workspace.ID, testutil.WithMessageHistoryContactEmail(seed.email),
				testutil.WithMessageHistoryTemplateID("tpl-bf"))
			require.NoError(t, err)
			links, err := json.Marshal(seed.links)
			require.NoError(t, err)
			_, err = db.ExecContext(bg, `UPDATE message_history SET clicked_at = $2, clicked_links = $3::jsonb WHERE contact_email = $1`,
				seed.email, seed.clickedAt, string(links))
			require.NoError(t, err)
		}
		_, err = db.ExecContext(bg, `UPDATE contacts SET tracking_clicks_consent = 'denied' WHERE email = $1`, refused)
		require.NoError(t, err)
		// A message sent later and never clicked does not move it.
		_, err = factory.CreateMessageHistory(workspace.ID, testutil.WithMessageHistoryContactEmail(clicked),
			testutil.WithMessageHistoryTemplateID("tpl-bf"))
		require.NoError(t, err)
		for _, row := range []struct {
			email, entityType, kind string
			at                      time.Time
		}{
			{replied, "inbound_webhook_event", "email.replied", day(100)},
			{visited, "web_session", "web.visit", day(50)},
			{confirmed, "contact_list", "list.confirmed", day(30)},
			{newer, "inbound_webhook_event", "email.replied", day(300)},
			// What the contact did not do themselves is not read back.
			{silent, "contact_list", "list.subscribed", day(10)},
			{silent, "message_history", "email.opened", day(10)},
		} {
			_, err := db.ExecContext(bg, `INSERT INTO contact_timeline (id, email, operation, entity_type, kind, entity_id, changes, created_at)
				VALUES ($1, $2, 'insert', $3, $4, 'x', '{}'::jsonb, $5)`, uuid.New().String(), row.email, row.entityType, row.kind, row.at)
			require.NoError(t, err)
		}
		// Recorded live since the upgrade: the backfill must not move it back.
		_, err = db.ExecContext(bg, `INSERT INTO contact_engagement (email, last_engaged_at) VALUES ($1, $2)`, newer, day(1))
		require.NoError(t, err)
		// The silent contact may have been touched by nothing; make sure it starts empty.
		_, err = db.ExecContext(bg, `DELETE FROM contact_engagement WHERE email = ANY($1)`,
			fmt.Sprintf("{%s,%s,%s,%s,%s,%s,%s,%s}", clicked, replied, visited, confirmed, silent, perLink, unsubscribeOnly, refused))
		require.NoError(t, err)

		// The workspace as v45 leaves it: not ready, with a backfill task queued.
		_, err = db.ExecContext(bg, `UPDATE data_retention_state SET backfill_completed_at = NULL`)
		require.NoError(t, err)
		require.Equal(t, "not ready", retentionMarker(t, db))
		migration, ok := migrations.GetRegisteredMigration(45.0)
		require.True(t, ok)
		tx, err := suite.DBManager.GetDB().BeginTx(bg, nil)
		require.NoError(t, err)
		require.NoError(t, migration.UpdateSystem(bg, &config.Config{}, tx))
		require.NoError(t, tx.Commit())

		deadline := time.Now().Add(30 * time.Second)
		for retentionMarker(t, db) != "ready" {
			require.True(t, time.Now().Before(deadline), "the backfill task never marked the workspace ready")
			resp, err := client.ExecutePendingTasks(10)
			require.NoError(t, err)
			_ = resp.Body.Close()
			time.Sleep(200 * time.Millisecond)
		}

		expect := map[string]time.Time{clicked: day(200), replied: day(100), visited: day(50), confirmed: day(30), newer: day(1), perLink: day(150)}
		for email, want := range expect {
			got := engagedAt(t, email)
			if assert.NotNil(t, got, email) {
				assert.WithinDuration(t, want, *got, time.Second, email)
			}
		}
		assert.Nil(t, engagedAt(t, silent), "a subscription and an open are not engagement")
		assert.Nil(t, engagedAt(t, unsubscribeOnly), "a click on the unsubscribe link is not engagement")
		assert.Nil(t, engagedAt(t, refused), "a contact who refused click tracking gets nothing from their clicks")

		var status string
		require.NoError(t, suite.DBManager.GetDB().QueryRowContext(bg,
			`SELECT status FROM tasks WHERE workspace_id = $1 AND type = $2`, workspace.ID, domain.DataRetentionBackfillTaskType).Scan(&status))
		assert.Equal(t, "completed", status)
	})
}
