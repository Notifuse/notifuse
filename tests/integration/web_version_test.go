package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lithammer/shortuuid/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/trackingtoken"
	"github.com/Notifuse/notifuse/tests/testutil"
)

const webVersionMJML = `<mjml><mj-body><mj-section><mj-column>` +
	`<mj-text><a href="{{ view_in_browser_url }}">View in browser</a></mj-text>` +
	`<mj-text>Hello {{ contact.first_name }}, %s</mj-text>` +
	`<mj-button href="https://shop.example.com/sale">Shop</mj-button>` +
	`</mj-column></mj-section></mj-body></mjml>`

// deliveredWebVersionLink returns the web version link of a delivered email: the click
// redirect wrapping it when clicks are tracked, and its destination, the /v/ URL with the
// UTM parameters the send appended.
func deliveredWebVersionLink(t *testing.T, html string) (clickLink, destination string) {
	t.Helper()
	for _, m := range deliveredClickLinkRe.FindAllStringSubmatch(html, -1) {
		tok, err := trackingtoken.Parse(m[2], trackingtoken.PurposeClick)
		require.NoError(t, err)
		if strings.Contains(tok.URL, "/v/") {
			return m[1], tok.URL
		}
	}
	t.Fatalf("no web version link in the delivered email: %s", html)
	return "", ""
}

func getWebVersion(t *testing.T, link string) (*http.Response, string) {
	t.Helper()
	resp, err := noRedirectClient().Get(link)
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	return resp, string(body)
}

func assertWebVersionSandboxed(t *testing.T, resp *http.Response) {
	t.Helper()
	csp := resp.Header.Get("Content-Security-Policy")
	require.True(t, strings.HasPrefix(csp, "sandbox "), csp)
	sandbox := strings.SplitN(csp, ";", 2)[0]
	assert.NotContains(t, sandbox, "allow-scripts")
	assert.NotContains(t, sandbox, "allow-same-origin")
	assert.Contains(t, csp, "default-src 'none'")
}

func webVersionEngagement(t *testing.T, db *sql.DB, messageID string) (opened, clicked sql.NullTime) {
	t.Helper()
	require.NoError(t, db.QueryRow(`SELECT opened_at, clicked_at FROM message_history WHERE id = $1`, messageID).Scan(&opened, &clicked))
	return opened, clicked
}

// TestWebVersion sends emails whose template links to their web version, through each
// path in scope, and opens the link exactly as delivered.
func TestWebVersion(t *testing.T) {
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

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))
	require.NoError(t, factory.EnableEmailTracking(workspace.ID))
	integration, err := factory.SetupWorkspaceWithSMTPProvider(workspace.ID)
	require.NoError(t, err)
	// Links point at the test server, so they are opened exactly as delivered.
	require.NoError(t, factory.SetWorkspaceCustomEndpointURL(workspace.ID, serverURL))
	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(workspace.ID)
	workspaceDB, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)
	require.NoError(t, testutil.ClearMailpitMessages(t))

	newEmail := func(name string) string {
		return fmt.Sprintf("webversion-%s-%s@example.com", name, uuid.New().String()[:8])
	}
	newTemplate := func(t *testing.T, line string) *domain.Template {
		t.Helper()
		template, err := factory.CreateTemplate(workspace.ID,
			testutil.WithTemplateSubject("Web version "+uuid.New().String()[:8]),
			testutil.WithCodeModeTemplate(fmt.Sprintf(webVersionMJML, line)))
		require.NoError(t, err)
		return template
	}
	postOK := func(t *testing.T, path string, body map[string]interface{}) {
		t.Helper()
		resp, err := client.Post(path, body)
		require.NoError(t, err)
		raw, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode, "%s: %s", path, raw)
	}

	t.Run("broadcast", func(t *testing.T) {
		template := newTemplate(t, "the autumn coffees are here.")
		list, err := factory.CreateList(workspace.ID)
		require.NoError(t, err)
		camille := newEmail("camille")
		attacker := newEmail("attacker")
		for email, firstName := range map[string]string{camille: "Camille", attacker: `<img src=x onerror=alert(1)>`} {
			_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(email), testutil.WithContactName(firstName, "Doe"))
			require.NoError(t, err)
			_, err = factory.CreateContactList(workspace.ID, testutil.WithContactListEmail(email),
				testutil.WithContactListListID(list.ID), testutil.WithContactListStatus(domain.ContactListStatusActive))
			require.NoError(t, err)
		}
		broadcast, err := factory.CreateBroadcast(workspace.ID,
			testutil.WithBroadcastTemplateID(template.ID),
			testutil.WithBroadcastAudience(domain.AudienceSettings{List: list.ID, ExcludeUnsubscribed: true}))
		require.NoError(t, err)

		scheduleResp, err := client.ScheduleBroadcast(map[string]interface{}{"workspace_id": workspace.ID, "id": broadcast.ID, "send_now": true})
		require.NoError(t, err)
		_ = scheduleResp.Body.Close()
		_, err = testutil.WaitForBroadcastStatusWithExecution(t, client, broadcast.ID, []string{"processed", "completed", "sent"}, 60*time.Second)
		require.NoError(t, err)

		msg, err := waitForEmailAndGetMessage(t, camille, 30*time.Second)
		require.NoError(t, err)
		clickLink, destination := deliveredWebVersionLink(t, msg.HTML)
		require.True(t, strings.HasPrefix(destination, serverURL+"/v/"), destination)
		viewTok, err := trackingtoken.Parse(strings.SplitN(strings.TrimPrefix(destination, serverURL+"/v/"), "?", 2)[0], trackingtoken.PurposeView)
		require.NoError(t, err)

		// Viewing records nothing.
		openedBefore, clickedBefore := webVersionEngagement(t, workspaceDB, viewTok.MessageID)
		resp, body := getWebVersion(t, destination)
		require.Equal(t, http.StatusOK, resp.StatusCode, body)
		assertWebVersionSandboxed(t, resp)
		assert.Contains(t, body, "Hello Camille")
		assert.Contains(t, body, `href="https://shop.example.com/sale?`, "links go straight to their destination, with their UTM")
		assert.NotContains(t, body, "/r/", "no click redirect in the web version")
		assert.NotContains(t, body, "/t/", "no open pixel in the web version")
		openedAfter, clickedAfter := webVersionEngagement(t, workspaceDB, viewTok.MessageID)
		assert.Equal(t, openedBefore, openedAfter)
		assert.Equal(t, clickedBefore, clickedAfter)

		// Another workspace's custom domain does not show it.
		foreign, err := http.NewRequest(http.MethodGet, destination, nil)
		require.NoError(t, err)
		foreign.Host = "mail.another-brand.example"
		foreignResp, err := noRedirectClient().Do(foreign)
		require.NoError(t, err)
		foreignBody, _ := io.ReadAll(foreignResp.Body)
		_ = foreignResp.Body.Close()
		assert.Equal(t, http.StatusNotFound, foreignResp.StatusCode)
		assert.NotContains(t, string(foreignBody), "Camille")

		// The tracked link in the email redirects to the web version.
		redirect := clickDelivered(t, clickLink)
		assert.Equal(t, http.StatusSeeOther, redirect.StatusCode)
		assert.Equal(t, destination, redirect.Header.Get("Location"))

		// A first name carrying markup reaches the page as written, under the sandbox.
		attackerMsg, err := waitForEmailAndGetMessage(t, attacker, 30*time.Second)
		require.NoError(t, err)
		_, attackerDestination := deliveredWebVersionLink(t, attackerMsg.HTML)
		resp, body = getWebVersion(t, attackerDestination)
		require.Equal(t, http.StatusOK, resp.StatusCode, body)
		assert.Contains(t, body, `<img src=x onerror=alert(1)`)
		assertWebVersionSandboxed(t, resp)

		// The page shows the version sent, not the template as edited since.
		edited := *template
		edited.Email = &domain.EmailTemplate{}
		*edited.Email = *template.Email
		editedSource := fmt.Sprintf(webVersionMJML, "edited after the send.")
		edited.Email.MjmlSource = &editedSource
		edited.Email.CompiledPreview = editedSource
		require.NoError(t, factory.UpdateTemplate(workspace.ID, &edited))
		require.Greater(t, edited.Version, template.Version)
		_, body = getWebVersion(t, destination)
		assert.Contains(t, body, "the autumn coffees are here.")
		assert.NotContains(t, body, "edited after the send.")

		// Deleting the template takes its web versions down.
		delResp, err := client.DeleteTemplate(workspace.ID, template.ID)
		require.NoError(t, err)
		_ = delResp.Body.Close()
		require.Equal(t, http.StatusOK, delResp.StatusCode)
		resp, _ = getWebVersion(t, destination)
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		assertWebVersionSandboxed(t, resp)
	})

	t.Run("erasing the contact takes the web version down", func(t *testing.T) {
		template := newTemplate(t, "a test send.")
		email := newEmail("erased")
		_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(email), testutil.WithContactName("Erin", "Doe"))
		require.NoError(t, err)
		list, err := factory.CreateList(workspace.ID)
		require.NoError(t, err)
		broadcast, err := factory.CreateBroadcast(workspace.ID,
			testutil.WithBroadcastTemplateID(template.ID),
			testutil.WithBroadcastAudience(domain.AudienceSettings{List: list.ID}))
		require.NoError(t, err)
		postOK(t, "/api/broadcasts.sendToIndividual", map[string]interface{}{
			"workspace_id": workspace.ID, "broadcast_id": broadcast.ID, "recipient_email": email, "template_id": template.ID,
		})
		msg, err := waitForEmailAndGetMessage(t, email, 20*time.Second)
		require.NoError(t, err)
		_, destination := deliveredWebVersionLink(t, msg.HTML)

		resp, body := getWebVersion(t, destination)
		require.Equal(t, http.StatusOK, resp.StatusCode, body)
		assert.Contains(t, body, "Hello Erin")

		postOK(t, "/api/contacts.delete", map[string]interface{}{"workspace_id": workspace.ID, "email": email})
		resp, _ = getWebVersion(t, destination)
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("automation", func(t *testing.T) {
		template := newTemplate(t, "welcome aboard.")
		email := newEmail("automation")
		_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(email), testutil.WithContactName("Alex", "Doe"))
		require.NoError(t, err)
		list, err := factory.CreateList(workspace.ID)
		require.NoError(t, err)
		eventName := "web_version_" + uuid.New().String()[:8]
		createActiveAutomation(t, client, workspace.ID, shortuuid.New(), list.ID, eventName, template.ID)
		require.NoError(t, factory.CreateCustomEvent(workspace.ID, email, eventName, map[string]interface{}{}))

		msg, err := waitForEmailAndGetMessage(t, email, 30*time.Second)
		require.NoError(t, err)
		_, destination := deliveredWebVersionLink(t, msg.HTML)
		resp, body := getWebVersion(t, destination)
		require.Equal(t, http.StatusOK, resp.StatusCode, body)
		assert.Contains(t, body, "Hello Alex, welcome aboard.")
		assertWebVersionSandboxed(t, resp)
	})

	t.Run("a template that does not use the variable has no web version", func(t *testing.T) {
		// The send records a link for every broadcast email; only the variable in the
		// template makes it open.
		source := `<mjml><mj-body><mj-section><mj-column><mj-text>Hello {{ contact.first_name }}, no link here.</mj-text></mj-column></mj-section></mj-body></mjml>`
		template, err := factory.CreateTemplate(workspace.ID,
			testutil.WithTemplateSubject("No web version "+uuid.New().String()[:8]),
			testutil.WithCodeModeTemplate(source))
		require.NoError(t, err)
		email := newEmail("nolink")
		_, err = factory.CreateContact(workspace.ID, testutil.WithContactEmail(email), testutil.WithContactName("Noa", "Doe"))
		require.NoError(t, err)
		list, err := factory.CreateList(workspace.ID)
		require.NoError(t, err)
		broadcast, err := factory.CreateBroadcast(workspace.ID,
			testutil.WithBroadcastTemplateID(template.ID),
			testutil.WithBroadcastAudience(domain.AudienceSettings{List: list.ID}))
		require.NoError(t, err)
		postOK(t, "/api/broadcasts.sendToIndividual", map[string]interface{}{
			"workspace_id": workspace.ID, "broadcast_id": broadcast.ID, "recipient_email": email, "template_id": template.ID,
		})
		_, err = waitForEmailAndGetMessage(t, email, 20*time.Second)
		require.NoError(t, err)

		var link string
		require.Eventually(t, func() bool {
			resp, err := client.Get("/api/messages.list", map[string]string{"workspace_id": workspace.ID, "limit": "100"})
			if err != nil {
				return false
			}
			raw, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			var payload struct {
				Messages []struct {
					ContactEmail string `json:"contact_email"`
					MessageData  struct {
						Data map[string]interface{} `json:"data"`
					} `json:"message_data"`
				} `json:"messages"`
			}
			if json.Unmarshal(raw, &payload) != nil {
				return false
			}
			for _, m := range payload.Messages {
				if m.ContactEmail == email {
					link, _ = m.MessageData.Data["view_in_browser_url"].(string)
					return true
				}
			}
			return false
		}, 20*time.Second, 200*time.Millisecond)
		require.True(t, strings.HasPrefix(link, serverURL+"/v/"), link)

		resp, body := getWebVersion(t, link)
		assert.Equal(t, http.StatusNotFound, resp.StatusCode, body)
		assert.NotContains(t, body, "Noa")
	})

	t.Run("transactional emails get no web version link", func(t *testing.T) {
		template := newTemplate(t, "your receipt.")
		email := newEmail("transactional")
		postOK(t, "/api/transactional.testTemplate", map[string]interface{}{
			"workspace_id":    workspace.ID,
			"template_id":     template.ID,
			"integration_id":  integration.ID,
			"sender_id":       integration.EmailProvider.Senders[0].ID,
			"recipient_email": email,
		})
		msg, err := waitForEmailAndGetMessage(t, email, 20*time.Second)
		require.NoError(t, err)
		for _, m := range deliveredClickLinkRe.FindAllStringSubmatch(msg.HTML, -1) {
			tok, err := trackingtoken.Parse(m[2], trackingtoken.PurposeClick)
			require.NoError(t, err)
			assert.NotContains(t, tok.URL, "/v/")
		}
		assert.NotContains(t, msg.HTML, "/v/")
	})

	t.Run("a forged token is not found", func(t *testing.T) {
		resp, _ := getWebVersion(t, serverURL+"/v/forged-token")
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		assertWebVersionSandboxed(t, resp)
	})
}
