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
	"os"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/trackingtoken"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// TestNotificationCenterLinksBrowserE2E opens the links of real emails in a real browser,
// on the real server serving the built notification center, and reads the database to
// check that opening a link writes nothing: email security scanners open every link of
// an email, and run its JavaScript too. Only a click on the page's button writes.
//
// Every other test of these links replays by hand the request the page sends, so none of
// them can see what the page does on its own when it loads.
//
// Enable with BROWSER_E2E=true. It needs Chrome, and the notification center built
// (cd notification_center && npm run build).
func TestNotificationCenterLinksBrowserE2E(t *testing.T) {
	if os.Getenv("BROWSER_E2E") != "true" {
		t.Skip("set BROWSER_E2E=true to run the notification center browser test (needs Chrome and a built notification_center/dist)")
	}
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	// The live server: its API endpoint is its own address, which the page reads from
	// /config.js to call back
	suite := testutil.NewIntegrationTestSuiteWithLiveScheduler(t, appFactory)
	defer suite.Cleanup()
	client := suite.APIClient
	factory := suite.DataFactory
	serverURL := suite.ServerManager.GetURL()
	bg := context.Background()

	resp, err := http.Get(serverURL + "/notification-center")
	require.NoError(t, err)
	page, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode,
		"the server does not serve the notification center: build it with `cd notification_center && npm run build`")
	require.Contains(t, string(page), `<div id="root">`, "the notification center page is not the built widget")

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))
	require.NoError(t, factory.EnableEmailTracking(workspace.ID))
	_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID)
	require.NoError(t, err)
	require.NoError(t, factory.SetWorkspaceCustomEndpointURL(workspace.ID, serverURL))
	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(workspace.ID)
	db, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	listStatus := func(t *testing.T, email, listID string) string {
		t.Helper()
		var status string
		require.NoError(t, db.QueryRowContext(bg,
			`SELECT status FROM contact_lists WHERE email = $1 AND list_id = $2`, email, listID).Scan(&status))
		return status
	}
	type messageState struct{ opened, clicked, unsubscribed bool }
	lastMessage := func(t *testing.T, email string) messageState {
		t.Helper()
		var s messageState
		require.NoError(t, db.QueryRowContext(bg, `SELECT opened_at IS NOT NULL, clicked_at IS NOT NULL, unsubscribed_at IS NOT NULL
			FROM message_history WHERE contact_email = $1 ORDER BY created_at DESC LIMIT 1`, email).
			Scan(&s.opened, &s.clicked, &s.unsubscribed))
		return s
	}
	preferencesOf := func(t *testing.T, email string) (language, timezone sql.NullString) {
		t.Helper()
		require.NoError(t, db.QueryRowContext(bg, `SELECT language, timezone FROM contacts WHERE email = $1`, email).
			Scan(&language, &timezone))
		return language, timezone
	}
	trackingConsentOf := func(t *testing.T, email string) (opens, clicks, source sql.NullString) {
		t.Helper()
		require.NoError(t, db.QueryRowContext(bg, `SELECT tracking_opens_consent, tracking_clicks_consent, tracking_consent_source
			FROM contacts WHERE email = $1`, email).Scan(&opens, &clicks, &source))
		return opens, clicks, source
	}
	engaged := func(t *testing.T, email string) bool {
		t.Helper()
		var ok bool
		require.NoError(t, db.QueryRowContext(bg, `SELECT EXISTS (SELECT 1 FROM contact_engagement WHERE email = $1)`, email).Scan(&ok))
		return ok
	}
	eventually := func(t *testing.T, what string, condition func() bool) {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for !condition() {
			require.True(t, time.Now().Before(deadline), "timed out waiting for %s", what)
			time.Sleep(100 * time.Millisecond)
		}
	}
	requirePreferencesUntouched := func(t *testing.T, email string) {
		t.Helper()
		language, timezone := preferencesOf(t, email)
		assert.False(t, language.Valid, "opening the link saved a language")
		assert.False(t, timezone.Valid, "opening the link saved a timezone")
	}

	// A contact with no language nor timezone, so that the page has something to save the
	// moment a person interacts with it, and a scanner's visit would show
	newContact := func(t *testing.T, name string) string {
		t.Helper()
		email := fmt.Sprintf("nc-browser-%s-%s@example.com", name, uuid.New().String()[:8])
		_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(email), func(c *domain.Contact) {
			c.Language = nil
			c.Timezone = nil
		})
		require.NoError(t, err)
		return email
	}
	hrefPattern := regexp.MustCompile(`href="([^"]+)"`)
	// pageLink returns the delivered link to the instance page at path with that action
	// ("" for none). It must be the page's own address: a link to the instance that goes
	// through the click redirect fails the test.
	pageLink := func(t *testing.T, html, path, action string) string {
		t.Helper()
		for _, m := range hrefPattern.FindAllStringSubmatch(html, -1) {
			link := strings.ReplaceAll(m[1], "&amp;", "&")
			if token, ok := strings.CutPrefix(link, serverURL+"/r/"); ok {
				tok, err := trackingtoken.Parse(token, trackingtoken.PurposeClick)
				require.NoError(t, err)
				require.False(t, strings.HasPrefix(tok.URL, serverURL),
					"a link to the instance went through the click redirect: %s", tok.URL)
				continue
			}
			if !strings.HasPrefix(link, serverURL+path+"?") {
				continue
			}
			if u, err := url.Parse(link); err == nil && u.Query().Get("action") == action {
				return link
			}
		}
		t.Fatalf("no link to %s (action %q) in the delivered email", path, action)
		return ""
	}

	browser := launchCDPBrowser(t)
	// Deferred after suite.Cleanup, so it runs first: the browser stops before the server
	defer browser.Close()
	// settle leaves the page time to run whatever it would run after showing, then checks
	// it raised nothing
	settle := func(t *testing.T) {
		t.Helper()
		time.Sleep(2 * time.Second)
		require.Empty(t, browser.thrownErrors(), "the page raised an exception")
	}
	card := func(testID string) string {
		return `document.querySelector('[data-testid="` + testID + `"]')`
	}
	cardButton := func(testID string) string {
		return `document.querySelector('[data-testid="` + testID + `"] button')`
	}
	resultShown := `!!document.querySelector('[role="status"]')`

	t.Run("the links of a broadcast ask before anything is written", func(t *testing.T) {
		list, err := factory.CreateList(workspace.ID, testutil.WithListPublic(true), testutil.WithListDoubleOptin(false))
		require.NoError(t, err)
		unsubscriber, headerUser, visitor, refuser, scanned :=
			newContact(t, "unsubscribe"), newContact(t, "header"), newContact(t, "preferences"), newContact(t, "optout"), newContact(t, "scanner")
		recipients := []string{unsubscriber, headerUser, visitor, refuser, scanned}
		for _, email := range recipients {
			_, err := factory.CreateContactList(workspace.ID,
				testutil.WithContactListEmail(email),
				testutil.WithContactListListID(list.ID),
				testutil.WithContactListStatus(domain.ContactListStatusActive))
			require.NoError(t, err)
		}
		template, err := factory.CreateTemplate(workspace.ID,
			testutil.WithTemplateSubject("Browser links "+uuid.New().String()[:8]),
			testutil.WithCodeModeTemplate(`<mjml><mj-body><mj-section><mj-column><mj-text>`+
				`<a href="https://shop.example.com/sale">Sale</a> <a href="{{ unsubscribe_url }}">Unsubscribe</a>`+
				` <a href="{{ notification_center_url }}">Preferences</a> <a href="{{ tracking_optout_url }}">Stop tracking</a>`+
				` <a href="{{ view_in_browser_url }}">View in browser</a>`+
				`</mj-text></mj-column></mj-section></mj-body></mjml>`))
		require.NoError(t, err)
		broadcast, err := factory.CreateBroadcast(workspace.ID,
			testutil.WithBroadcastTemplateID(template.ID),
			testutil.WithBroadcastAudience(domain.AudienceSettings{List: list.ID, ExcludeUnsubscribed: true}))
		require.NoError(t, err)
		scheduleResp, err := client.ScheduleBroadcast(map[string]interface{}{
			"workspace_id": workspace.ID, "id": broadcast.ID, "send_now": true,
		})
		require.NoError(t, err)
		_ = scheduleResp.Body.Close()
		require.Equal(t, http.StatusOK, scheduleResp.StatusCode)

		delivered := map[string]*testutil.MailpitMessage{}
		for _, email := range recipients {
			msg, err := testutil.WaitForMailpitMessageByRecipient(t, email, 60*time.Second)
			require.NoError(t, err)
			delivered[email] = msg
		}
		// The content link goes through the click redirect; the web version link, like the
		// other links to the instance's pages, is its page's own address
		for _, email := range recipients {
			html := delivered[email].HTML
			assert.Contains(t, html, `href="`+serverURL+`/r/`, "the content link is tracked")
			var webVersion string
			for _, m := range hrefPattern.FindAllStringSubmatch(html, -1) {
				if strings.HasPrefix(m[1], serverURL+"/v/") {
					webVersion = m[1]
				}
				if token, ok := strings.CutPrefix(m[1], serverURL+"/r/"); ok {
					tok, err := trackingtoken.Parse(token, trackingtoken.PurposeClick)
					require.NoError(t, err)
					assert.NotContains(t, tok.URL, "/v/", "the web version link went through the click redirect")
				}
			}
			assert.NotEmpty(t, webVersion, "the email links directly to its web version")
		}

		// The click route ignores clicks within 7 s of the send as prefetches: wait it out,
		// or "no click recorded" below would hold for that reason alone
		time.Sleep(8 * time.Second)

		// The email's unsubscribe link: the page asks, only the click unsubscribes
		browser.navigate(t, pageLink(t, delivered[unsubscriber].HTML, "/notification-center", "unsubscribe"))
		browser.waitFor(t, "!!"+card("confirm-unsubscribe"), "the unsubscribe question")
		settle(t)
		assert.Equal(t, "active", listStatus(t, unsubscriber, list.ID), "opening the link unsubscribed the contact")
		assert.Equal(t, messageState{}, lastMessage(t, unsubscriber))
		requirePreferencesUntouched(t, unsubscriber)
		browser.click(t, cardButton("confirm-unsubscribe"))
		browser.waitFor(t, resultShown, "the unsubscribe result")
		eventually(t, "the unsubscribe", func() bool { return listStatus(t, unsubscriber, list.ID) == "unsubscribed" })
		state := lastMessage(t, unsubscriber)
		assert.True(t, state.unsubscribed, "the email records the unsubscribe")
		assert.False(t, state.clicked, "the unsubscribe is not a click")

		// The List-Unsubscribe header link opened in a browser: the server redirects it to
		// the same question
		browser.navigate(t, listUnsubscribeURL(t, delivered[headerUser]).String())
		browser.waitFor(t, "!!"+card("confirm-unsubscribe"), "the unsubscribe question of the header link")
		settle(t)
		assert.Equal(t, "active", listStatus(t, headerUser, list.ID), "opening the header link unsubscribed the contact")
		requirePreferencesUntouched(t, headerUser)
		browser.click(t, cardButton("confirm-unsubscribe"))
		eventually(t, "the unsubscribe from the header link", func() bool { return listStatus(t, headerUser, list.ID) == "unsubscribed" })

		// The preferences link: loading saves nothing, the visitor's first interaction saves
		// the browser's timezone and changes no list
		browser.navigate(t, pageLink(t, delivered[visitor].HTML, "/notification-center", ""))
		listName, err := json.Marshal(list.Name)
		require.NoError(t, err)
		browser.waitFor(t, `document.body.innerText.includes(`+string(listName)+`)`, "the preferences page")
		settle(t)
		requirePreferencesUntouched(t, visitor)
		assert.Equal(t, messageState{}, lastMessage(t, visitor))
		// A key press is a first interaction too, and it clicks nothing on the page
		browser.pressShift(t)
		eventually(t, "the timezone saved on the first interaction", func() bool {
			_, timezone := preferencesOf(t, visitor)
			return timezone.Valid
		})
		assert.Equal(t, "active", listStatus(t, visitor, list.ID))

		// The tracking opt-out link: the dialog asks, only its confirmation refuses tracking
		browser.navigate(t, pageLink(t, delivered[refuser].HTML, "/notification-center", "tracking_optout"))
		browser.waitFor(t, "!!"+card("tracking-dialog"), "the tracking opt-out question")
		settle(t)
		opens, clicks, _ := trackingConsentOf(t, refuser)
		assert.False(t, opens.Valid, "opening the opt-out link refused open tracking")
		assert.False(t, clicks.Valid, "opening the opt-out link refused click tracking")
		browser.click(t, `[...document.querySelectorAll('[data-testid="tracking-dialog"] button')].at(-1)`)
		eventually(t, "the tracking refusal", func() bool {
			opens, clicks, _ := trackingConsentOf(t, refuser)
			return opens.String == "denied" && clicks.String == "denied"
		})
		_, _, source := trackingConsentOf(t, refuser)
		assert.Equal(t, "email_link", source.String)

		// A scanner that fetches the links without running the page changes nothing
		html := delivered[scanned].HTML
		for _, link := range []string{
			pageLink(t, html, "/notification-center", "unsubscribe"),
			pageLink(t, html, "/notification-center", ""),
			pageLink(t, html, "/notification-center", "tracking_optout"),
			listUnsubscribeURL(t, delivered[scanned]).String(),
		} {
			for _, method := range []string{http.MethodGet, http.MethodHead} {
				req, err := http.NewRequest(method, link, nil)
				require.NoError(t, err)
				resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
				require.NoError(t, err)
				_ = resp.Body.Close()
			}
		}
		time.Sleep(time.Second)
		assert.Equal(t, "active", listStatus(t, scanned, list.ID))
		assert.Equal(t, messageState{}, lastMessage(t, scanned))
		requirePreferencesUntouched(t, scanned)
		opens, clicks, _ = trackingConsentOf(t, scanned)
		assert.False(t, opens.Valid || clicks.Valid, "fetching the links changed the tracking consent")
	})

	t.Run("the double opt-in link asks before it confirms", func(t *testing.T) {
		template, err := factory.CreateTemplate(workspace.ID,
			testutil.WithTemplateSubject("Confirm "+uuid.New().String()[:8]),
			testutil.WithCodeModeTemplate(`<mjml><mj-body><mj-section><mj-column>`+
				`<mj-text><a href="{{ confirm_subscription_url }}">Confirm</a></mj-text>`+
				`</mj-column></mj-section></mj-body></mjml>`))
		require.NoError(t, err)
		list, err := factory.CreateList(workspace.ID, testutil.WithListPublic(true), testutil.WithListDoubleOptin(true),
			func(l *domain.List) {
				l.DoubleOptInTemplate = &domain.TemplateReference{ID: template.ID, Version: template.Version}
			})
		require.NoError(t, err)

		// The public form is rate limited per address: each post comes from its own
		var forwardedFor atomic.Int32
		publicSubscribe := func(t *testing.T, email string) {
			t.Helper()
			body, err := json.Marshal(map[string]interface{}{
				"workspace_id": workspace.ID, "contact": map[string]interface{}{"email": email}, "list_ids": []string{list.ID},
			})
			require.NoError(t, err)
			req, err := http.NewRequest(http.MethodPost, serverURL+"/subscribe", bytes.NewReader(body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Forwarded-For", fmt.Sprintf("198.51.100.%d", forwardedFor.Add(1)))
			resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
			require.NoError(t, err)
			raw, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode, "/subscribe: %s", raw)
		}

		// A newcomer, pending after the form; and a contact who had left the list and fills
		// the form again, whom the form leaves unsubscribed and sends the confirmation
		newcomer := fmt.Sprintf("nc-browser-newcomer-%s@example.com", uuid.New().String()[:8])
		returning := newContact(t, "returning")
		_, err = factory.CreateContactList(workspace.ID,
			testutil.WithContactListEmail(returning),
			testutil.WithContactListListID(list.ID),
			testutil.WithContactListStatus(domain.ContactListStatusUnsubscribed))
		require.NoError(t, err)
		publicSubscribe(t, newcomer)
		publicSubscribe(t, returning)

		confirmations := map[string]*testutil.MailpitMessage{}
		for _, email := range []string{newcomer, returning} {
			msg, err := testutil.WaitForMailpitMessageByRecipient(t, email, 30*time.Second)
			require.NoError(t, err)
			confirmations[email] = msg
		}
		time.Sleep(8 * time.Second) // past the click route's prefetch window, as above

		for _, tc := range []struct{ email, before string }{
			{newcomer, "pending"},
			{returning, "unsubscribed"},
		} {
			require.Equal(t, tc.before, listStatus(t, tc.email, list.ID))
			browser.navigate(t, pageLink(t, confirmations[tc.email].HTML, "/notification-center", "confirm"))
			browser.waitFor(t, "!!"+card("confirm-subscription"), "the subscription question for "+tc.email)
			settle(t)
			assert.Equal(t, tc.before, listStatus(t, tc.email, list.ID), "opening the link changed the subscription of %s", tc.email)
			assert.False(t, engaged(t, tc.email), "opening the link counted as engagement for %s", tc.email)
			assert.False(t, lastMessage(t, tc.email).clicked)

			browser.click(t, cardButton("confirm-subscription"))
			browser.waitFor(t, resultShown, "the confirmation result for "+tc.email)
			eventually(t, "the confirmation of "+tc.email, func() bool { return listStatus(t, tc.email, list.ID) == "active" })
			assert.True(t, engaged(t, tc.email), "the confirmation counts as engagement")
			assert.False(t, lastMessage(t, tc.email).clicked, "the confirmation is not a click")
		}
	})
}
