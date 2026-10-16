package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/google/uuid"
	shortuuid "github.com/lithammer/shortuuid/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/html"
)

// TestAutomationUnsubscribeAsDelivered unsubscribes from automation emails the way their
// recipients do. Every link comes from the email Mailpit received and is used unchanged:
// the one-click POST a mail client sends from the List-Unsubscribe header, and the request
// the notification center sends when the in-body link opens it.
//
// An automation with a list also emails contacts who have no row on that list, and those
// emails carry the list's unsubscribe links. Their unsubscribe used to answer 500 and
// record nothing, so the automation kept emailing them.
func TestAutomationUnsubscribeAsDelivered(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	// Automation emails are sent by the automation scheduler and the email queue worker,
	// which the harness only runs on request.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, suite.ServerManager.StartBackgroundWorkers(ctx))

	client := suite.APIClient
	factory := suite.DataFactory

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))
	_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID)
	require.NoError(t, err)
	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(workspace.ID)

	// The harness runs with an empty API endpoint, which puts relative URLs in the email.
	// Point the workspace endpoint at the live server so every delivered link is absolute.
	serverURL := suite.ServerManager.GetURL()
	require.NoError(t, factory.SetWorkspaceCustomEndpointURL(workspace.ID, serverURL))

	workspaceDB, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	countRows := func(t *testing.T, query string, args ...interface{}) int {
		t.Helper()
		var n int
		require.NoError(t, workspaceDB.QueryRowContext(context.Background(), query, args...).Scan(&n))
		return n
	}

	statusOf := func(t *testing.T, email, listID string) domain.ContactListStatus {
		t.Helper()
		var status string
		err := workspaceDB.QueryRowContext(context.Background(),
			`SELECT status FROM contact_lists WHERE email = $1 AND list_id = $2`, email, listID).Scan(&status)
		require.NoError(t, err, "expected a contact_lists row for %s", email)
		return domain.ContactListStatus(status)
	}

	type automationOnList struct {
		id, listID, eventName string
	}

	// newAutomation creates a list and an active automation on it: a custom event, then a
	// marketing email whose body links to the list's unsubscribe page.
	newAutomation := func(t *testing.T) automationOnList {
		t.Helper()
		list, err := factory.CreateList(workspace.ID)
		require.NoError(t, err)
		template, err := factory.CreateTemplate(workspace.ID,
			testutil.WithTemplateEmailContent(`Hello! <a href="{{ unsubscribe_url }}">Unsubscribe</a>`))
		require.NoError(t, err)
		require.Equal(t, "marketing", template.Category, "only marketing and blog emails check the list status")

		a := automationOnList{
			id:        shortuuid.New(),
			listID:    list.ID,
			eventName: "unsubscribe_delivered_" + uuid.New().String()[:8],
		}
		triggerNodeID := shortuuid.New()
		emailNodeID := shortuuid.New()

		resp, err := client.CreateAutomation(map[string]interface{}{
			"workspace_id": workspace.ID,
			"automation": map[string]interface{}{
				"id":           a.id,
				"workspace_id": workspace.ID,
				"name":         "Unsubscribe as delivered",
				"status":       "draft",
				"list_id":      list.ID,
				"trigger": map[string]interface{}{
					"event_kind":        "custom_event",
					"custom_event_name": a.eventName,
					"frequency":         "every_time",
				},
				"root_node_id": triggerNodeID,
				"nodes": []map[string]interface{}{
					{
						"id":            triggerNodeID,
						"automation_id": a.id,
						"type":          "trigger",
						"config":        map[string]interface{}{},
						"next_node_id":  emailNodeID,
						"position":      map[string]interface{}{"x": 0, "y": 0},
					},
					{
						"id":            emailNodeID,
						"automation_id": a.id,
						"type":          "email",
						"config":        map[string]interface{}{"template_id": template.ID},
						"position":      map[string]interface{}{"x": 0, "y": 100},
					},
				},
				"stats": map[string]interface{}{"enrolled": 0, "completed": 0, "exited": 0, "failed": 0},
			},
		})
		require.NoError(t, err)
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusCreated, resp.StatusCode, "create automation: %s", body)

		resp, err = client.ActivateAutomation(map[string]interface{}{
			"workspace_id":  workspace.ID,
			"automation_id": a.id,
		})
		require.NoError(t, err)
		body, _ = io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode, "activate automation: %s", body)

		return a
	}

	// deliver fires the automation's event for a new contact and returns the email Mailpit
	// received. A member has an active row on the automation's list; anyone else has none.
	deliver := func(t *testing.T, a automationOnList, member bool) (email string, msg *testutil.MailpitMessage) {
		t.Helper()
		email = fmt.Sprintf("unsub-delivered-%s@example.com", uuid.New().String()[:8])
		_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
		require.NoError(t, err)
		if member {
			_, err = factory.CreateContactList(workspace.ID,
				testutil.WithContactListEmail(email),
				testutil.WithContactListListID(a.listID),
				testutil.WithContactListStatus(domain.ContactListStatusActive))
			require.NoError(t, err)
		}

		require.NoError(t, factory.CreateCustomEvent(workspace.ID, email, a.eventName, map[string]interface{}{}))
		msg, err = waitForEmailAndGetMessage(t, email, 20*time.Second)
		require.NoError(t, err, "the automation email should reach Mailpit")

		// An SMTP send records its message_history row only once the server has accepted the
		// email, and an unsubscribe arriving before that has no message to mark.
		mid := listUnsubscribeURL(t, msg).Query().Get("mid")
		require.NotEmpty(t, mid, "the one-click URL identifies the message")
		testutil.WaitForCondition(t, func() bool {
			return countRows(t, `SELECT COUNT(*) FROM message_history WHERE id = $1`, mid) == 1
		}, 10*time.Second, "waiting for the message_history row of "+mid)
		return email, msg
	}

	// sendAgain fires the event once more and returns the new enrolment once it exits.
	sendAgain := func(t *testing.T, a automationOnList, email string) *domain.ContactAutomation {
		t.Helper()
		require.NoError(t, factory.CreateCustomEvent(workspace.ID, email, a.eventName, map[string]interface{}{}))
		// The first enrolment completed, so only the new one can report exited
		return waitForAutomationStatus(t, factory, workspace.ID, a.id, email, domain.ContactAutomationStatusExited, 15*time.Second)
	}

	oneClickURLEncoded := func(t *testing.T) (string, io.Reader) {
		return "application/x-www-form-urlencoded", strings.NewReader("List-Unsubscribe=One-Click")
	}

	// widgetUnsubscribe sends the request the notification center sends from its page's
	// own parameters, on load for action=unsubscribe and on the confirmation button for
	// action=confirm_unsubscribe (notification_center/src/App.tsx).
	widgetUnsubscribe := func(t *testing.T, page url.Values) {
		t.Helper()
		payload, err := json.Marshal(map[string]interface{}{
			"wid":        page.Get("wid"),
			"email":      page.Get("email"),
			"email_hmac": page.Get("email_hmac"),
			"lids":       []string{page.Get("lid")},
			"mid":        page.Get("mid"),
		})
		require.NoError(t, err)
		resp, err := (&http.Client{Timeout: 10 * time.Second}).Post(serverURL+"/unsubscribe", "application/json", bytes.NewReader(payload))
		require.NoError(t, err)
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode, "unsubscribe: %s", body)
	}

	// messageMarked reports whether the delivered message records an unsubscribe.
	messageMarked := func(t *testing.T, msg *testutil.MailpitMessage) bool {
		t.Helper()
		mid := listUnsubscribeURL(t, msg).Query().Get("mid")
		require.Equal(t, 1, countRows(t, `SELECT COUNT(*) FROM message_history WHERE id = $1`, mid),
			"the message history row must still exist for this check to mean anything")
		return countRows(t, `SELECT COUNT(*) FROM message_history WHERE id = $1 AND unsubscribed_at IS NOT NULL`, mid) == 1
	}

	automation := newAutomation(t)

	// RFC 8058 section 3.2: the receiver SHOULD send the one-click POST as
	// multipart/form-data and MAY send it url-encoded.
	oneClickBodies := []struct {
		name  string
		build func(t *testing.T) (contentType string, body io.Reader)
	}{
		{"url-encoded", oneClickURLEncoded},
		{"multipart/form-data", func(t *testing.T) (string, io.Reader) {
			var body bytes.Buffer
			form := multipart.NewWriter(&body)
			require.NoError(t, form.WriteField("List-Unsubscribe", "One-Click"))
			require.NoError(t, form.Close())
			return form.FormDataContentType(), &body
		}},
	}
	for _, oc := range oneClickBodies {
		t.Run("contact not on the list: one-click POST records the opt-out, "+oc.name, func(t *testing.T) {
			email, msg := deliver(t, automation, false)
			u := listUnsubscribeURL(t, msg)
			require.Equal(t, automation.listID, u.Query().Get("lids"), "the email carries the automation list's links")
			require.Zero(t, countRows(t, `SELECT COUNT(*) FROM contact_lists WHERE email = $1`, email))

			contentType, body := oc.build(t)
			resp := requestListUnsubscribeURL(t, http.MethodPost, u, contentType, body)

			require.Equal(t, http.StatusOK, resp.StatusCode)
			assert.Equal(t, domain.ContactListStatusUnsubscribed, statusOf(t, email, automation.listID))
			assert.True(t, messageMarked(t, msg), "the message records the unsubscribe")

			ca := sendAgain(t, automation, email)
			require.NotNil(t, ca.ExitReason)
			assert.Equal(t, "unsubscribed", *ca.ExitReason, "the next email is skipped, not sent")
			assert.Equal(t, 1, deliveredCountTo(t, email))
		})
	}

	t.Run("contact not on the list: the in-body link records the opt-out", func(t *testing.T) {
		email, msg := deliver(t, automation, false)
		link := deliveredLinkHref(t, msg.HTML, "Unsubscribe")

		// Open the link as a browser does, following any redirect
		pageResp, err := (&http.Client{Timeout: 10 * time.Second}).Get(link)
		require.NoError(t, err)
		_ = pageResp.Body.Close()
		page := pageResp.Request.URL
		require.Equal(t, "/notification-center", page.Path)
		params := page.Query()
		require.Equal(t, "unsubscribe", params.Get("action"))
		require.Equal(t, automation.listID, params.Get("lid"))

		widgetUnsubscribe(t, params)

		assert.Equal(t, domain.ContactListStatusUnsubscribed, statusOf(t, email, automation.listID))
	})

	// A mail client without one-click opens the header URL in a browser. The server
	// redirects it to the notification center, which unsubscribes once the visitor confirms.
	t.Run("contact not on the list: the header link opened in a browser, then confirmed", func(t *testing.T) {
		email, msg := deliver(t, automation, false)

		resp := requestListUnsubscribeURL(t, http.MethodGet, listUnsubscribeURL(t, msg), "", nil)

		require.Equal(t, http.StatusSeeOther, resp.StatusCode)
		page, err := resp.Location()
		require.NoError(t, err)
		require.Equal(t, "/notification-center", page.Path)
		params := page.Query()
		require.Equal(t, "confirm_unsubscribe", params.Get("action"))
		require.Zero(t, countRows(t, `SELECT COUNT(*) FROM contact_lists WHERE email = $1`, email),
			"opening the link must not unsubscribe anyone")

		widgetUnsubscribe(t, params)

		assert.Equal(t, domain.ContactListStatusUnsubscribed, statusOf(t, email, automation.listID))
	})

	// The signature covers the address, not the list: someone who edits their link to
	// name another list, such as a private one, must not end up on that list
	t.Run("contact not on the list: a link edited to name another list records nothing", func(t *testing.T) {
		email, msg := deliver(t, automation, false)
		other, err := factory.CreateList(workspace.ID)
		require.NoError(t, err)
		u := listUnsubscribeURL(t, msg)
		query := u.Query()
		query.Set("lids", other.ID)
		u.RawQuery = query.Encode()
		contentType, body := oneClickURLEncoded(t)

		resp := requestListUnsubscribeURL(t, http.MethodPost, u, contentType, body)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Zero(t, countRows(t, `SELECT COUNT(*) FROM contact_lists WHERE email = $1`, email))
		assert.False(t, messageMarked(t, msg))
	})

	t.Run("contact on the list: one-click POST unsubscribes", func(t *testing.T) {
		email, msg := deliver(t, automation, true)
		contentType, body := oneClickURLEncoded(t)

		resp := requestListUnsubscribeURL(t, http.MethodPost, listUnsubscribeURL(t, msg), contentType, body)

		require.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, domain.ContactListStatusUnsubscribed, statusOf(t, email, automation.listID))
		ca := sendAgain(t, automation, email)
		require.NotNil(t, ca.ExitReason)
		assert.Equal(t, "unsubscribed", *ca.ExitReason)
	})

	t.Run("contact erased after delivery: one-click POST succeeds and stores nothing", func(t *testing.T) {
		email, msg := deliver(t, automation, false)
		resp, err := client.Post("/api/contacts.delete", map[string]interface{}{
			"workspace_id": workspace.ID,
			"email":        email,
		})
		require.NoError(t, err)
		deleteBody, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode, "delete contact: %s", deleteBody)
		contentType, body := oneClickURLEncoded(t)

		oneClick := requestListUnsubscribeURL(t, http.MethodPost, listUnsubscribeURL(t, msg), contentType, body)

		assert.Equal(t, http.StatusOK, oneClick.StatusCode)
		assert.Zero(t, countRows(t, `SELECT COUNT(*) FROM contact_lists WHERE email = $1`, email),
			"the erased address must not be stored again")
		assert.Zero(t, countRows(t, `SELECT COUNT(*) FROM contacts WHERE email = $1`, email))
		// Erasure keeps the message with the address redacted. Marking it would still fire
		// its email.unsubscribed webhook, which names the message.
		assert.False(t, messageMarked(t, msg))
	})

	// Last, with its own list and automation: it deletes the list the automation sends for.
	t.Run("list deleted after delivery: one-click POST succeeds and stores nothing", func(t *testing.T) {
		deleted := newAutomation(t)
		email, msg := deliver(t, deleted, false)
		resp, err := client.Post("/api/lists.delete", map[string]interface{}{
			"workspace_id": workspace.ID,
			"id":           deleted.listID,
		})
		require.NoError(t, err)
		deleteBody, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode, "delete list: %s", deleteBody)
		contentType, body := oneClickURLEncoded(t)

		oneClick := requestListUnsubscribeURL(t, http.MethodPost, listUnsubscribeURL(t, msg), contentType, body)

		assert.Equal(t, http.StatusOK, oneClick.StatusCode)
		assert.Zero(t, countRows(t, `SELECT COUNT(*) FROM contact_lists WHERE email = $1`, email))
		assert.False(t, messageMarked(t, msg), "nothing was recorded, so the message isn't marked")
	})
}

// deliveredLinkHref returns the href of the one link in a delivered HTML body whose text
// is text, with entities decoded as a mail client decodes them.
func deliveredLinkHref(t *testing.T, body, text string) string {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(body))
	require.NoError(t, err)

	var linkText func(n *html.Node) string
	linkText = func(n *html.Node) string {
		if n.Type == html.TextNode {
			return n.Data
		}
		var s strings.Builder
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			s.WriteString(linkText(c))
		}
		return s.String()
	}

	var hrefs []string
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" && strings.TrimSpace(linkText(n)) == text {
			for _, attr := range n.Attr {
				if attr.Key == "href" {
					hrefs = append(hrefs, attr.Val)
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	require.Len(t, hrefs, 1, "expected exactly one %q link in the delivered email", text)
	return hrefs[0]
}

// deliveredCountTo counts the emails Mailpit holds for address.
func deliveredCountTo(t *testing.T, address string) int {
	t.Helper()
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Get("http://localhost:8025/api/v1/messages?limit=1000")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	var messages testutil.MailpitMessagesResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&messages))

	count := 0
	for _, msg := range messages.Messages {
		for _, to := range msg.To {
			if strings.EqualFold(to.Address, address) {
				count++
			}
		}
	}
	return count
}
