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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestListUnsubscribeHeaders is the full-stack contract test for the RFC 8058
// List-Unsubscribe header required by Gmail and Yahoo since 2024. Every subtest takes the
// URL from the email actually delivered to Mailpit and uses it as a mail receiver would -
// never a path re-joined onto the test server, which would test a URL no email contains -
// in each of the ways receivers and people use it: the one-click POST in both encodings
// RFC 8058 allows, and the GET/HEAD a browser or a scanner sends.
func TestListUnsubscribeHeaders(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	// Scheduled broadcasts are sent by the email queue worker, which the harness only runs
	// on request.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, suite.ServerManager.StartBackgroundWorkers(ctx))

	client := suite.APIClient
	factory := suite.DataFactory

	// Create test user and workspace
	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)

	// Add user to workspace as owner
	err = factory.AddUserToWorkspace(user.ID, workspace.ID, "owner")
	require.NoError(t, err)

	// Set up SMTP email provider for testing (Mailpit)
	integration, err := factory.SetupWorkspaceWithSMTPProvider(workspace.ID)
	require.NoError(t, err)

	// Login to get auth token
	err = client.Login(user.Email, "password")
	require.NoError(t, err)
	client.SetWorkspaceID(workspace.ID)

	// The harness runs with an empty API endpoint, which puts a relative URL in the header -
	// one no mail client can use. Point the workspace's tracking endpoint at the live server
	// so the delivered header carries a real, absolute URL.
	serverURL := suite.ServerManager.GetURL()
	require.NoError(t, factory.SetWorkspaceCustomEndpointURL(workspace.ID, serverURL))

	// Clear any existing messages in Mailpit
	if err := testutil.ClearMailpitMessages(t); err != nil {
		t.Logf("Warning: Could not clear Mailpit messages: %v", err)
	}

	contactListRepo := suite.ServerManager.GetApp().GetContactListRepository()
	statusOf := func(t *testing.T, email, listID string) domain.ContactListStatus {
		t.Helper()
		got, err := contactListRepo.GetContactListByIDs(context.Background(), workspace.ID, email, listID)
		require.NoError(t, err)
		return got.Status
	}

	// subscribedContact creates a list and a contact actively subscribed to it, so every
	// subtest can tell whether its own request unsubscribed anyone.
	subscribedContact := func(t *testing.T) (email, listID string) {
		t.Helper()
		list, err := factory.CreateList(workspace.ID)
		require.NoError(t, err)
		contact, err := factory.CreateContact(workspace.ID)
		require.NoError(t, err)
		_, err = factory.CreateContactList(workspace.ID,
			testutil.WithContactListEmail(contact.Email),
			testutil.WithContactListListID(list.ID),
			testutil.WithContactListStatus(domain.ContactListStatusActive))
		require.NoError(t, err)
		return contact.Email, list.ID
	}

	// sendToIndividualWith sends a broadcast to a fresh subscribed contact through
	// broadcasts.sendToIndividual and returns the email Mailpit received. excludeUnsubscribed
	// is the broadcast's audience setting.
	sendToIndividualWith := func(t *testing.T, excludeUnsubscribed bool) (msg *testutil.MailpitMessage, email, listID string) {
		t.Helper()
		email, listID = subscribedContact(t)

		// Create templates for A/B testing (requires at least 2)
		template1, err := factory.CreateTemplate(workspace.ID)
		require.NoError(t, err)
		template2, err := factory.CreateTemplate(workspace.ID)
		require.NoError(t, err)

		broadcast, err := factory.CreateBroadcast(workspace.ID,
			testutil.WithBroadcastABTesting([]string{template1.ID, template2.ID}),
			testutil.WithBroadcastAudience(domain.AudienceSettings{
				List:                listID,
				ExcludeUnsubscribed: excludeUnsubscribed,
			}))
		require.NoError(t, err)

		resp, err := client.SendBroadcastToIndividual(map[string]interface{}{
			"workspace_id":    workspace.ID,
			"broadcast_id":    broadcast.ID,
			"recipient_email": email,
			"template_id":     template1.ID,
		})
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusOK {
			var errResult map[string]interface{}
			_ = json.NewDecoder(resp.Body).Decode(&errResult)
			t.Fatalf("Failed to send broadcast: status=%d, response=%+v", resp.StatusCode, errResult)
		}

		msg, err = waitForEmailAndGetMessage(t, email, 10*time.Second)
		require.NoError(t, err, "Should receive email in Mailpit")
		return msg, email, listID
	}

	// sendToIndividual sends the usual broadcast, which excludes unsubscribed contacts.
	sendToIndividual := func(t *testing.T) (msg *testutil.MailpitMessage, email, listID string) {
		t.Helper()
		return sendToIndividualWith(t, true)
	}

	t.Run("header carries an absolute one-click URL on the workspace endpoint", func(t *testing.T) {
		msg, email, listID := sendToIndividual(t)

		u := listUnsubscribeURL(t, msg)
		assert.Equal(t, serverURL, u.Scheme+"://"+u.Host, "the URL must point at the workspace endpoint")
		assert.Equal(t, "/unsubscribe-oneclick", u.Path)
		assert.Equal(t, email, u.Query().Get("email"))
		assert.Equal(t, listID, u.Query().Get("lids"))
		assert.NotEmpty(t, u.Query().Get("email_hmac"), "the endpoint authorizes the unsubscribe with this HMAC")

		listUnsubscribePost := msg.Headers["List-Unsubscribe-Post"]
		require.NotEmpty(t, listUnsubscribePost, "Email should have List-Unsubscribe-Post header")
		assert.Equal(t, "List-Unsubscribe=One-Click", listUnsubscribePost[0],
			"List-Unsubscribe-Post should be 'List-Unsubscribe=One-Click'")
	})

	// A broadcast that keeps the list's unsubscribed contacts is a notice every member must
	// receive (issue #484): its emails carry neither header, so mail clients show no
	// Unsubscribe button.
	t.Run("no header when the broadcast includes unsubscribed contacts", func(t *testing.T) {
		msg, _, _ := sendToIndividualWith(t, false)

		assert.Empty(t, msg.Headers["List-Unsubscribe"], "Email should have no List-Unsubscribe header")
		assert.Empty(t, msg.Headers["List-Unsubscribe-Post"], "Email should have no List-Unsubscribe-Post header")
	})

	// RFC 8058 section 3.2: the receiver SHOULD send the one-click POST as
	// multipart/form-data and MAY send it url-encoded. Both must reach the real
	// HMAC-verifying service and flip the subscription - a mocked service cannot show that.
	oneClickBodies := []struct {
		name  string
		build func(t *testing.T) (contentType string, body io.Reader)
	}{
		{"url-encoded", func(t *testing.T) (string, io.Reader) {
			return "application/x-www-form-urlencoded", strings.NewReader("List-Unsubscribe=One-Click")
		}},
		{"multipart/form-data", func(t *testing.T) (string, io.Reader) {
			var body bytes.Buffer
			form := multipart.NewWriter(&body)
			require.NoError(t, form.WriteField("List-Unsubscribe", "One-Click"))
			require.NoError(t, form.Close())
			return form.FormDataContentType(), &body
		}},
	}
	for _, oc := range oneClickBodies {
		t.Run("one-click POST unsubscribes: "+oc.name, func(t *testing.T) {
			msg, email, listID := sendToIndividual(t)
			contentType, body := oc.build(t)

			resp := requestListUnsubscribeURL(t, http.MethodPost, listUnsubscribeURL(t, msg), contentType, body)

			assert.Equal(t, http.StatusOK, resp.StatusCode, "RFC 8058 one-click POST must be accepted, not rejected")
			assert.Equal(t, domain.ContactListStatusUnsubscribed, statusOf(t, email, listID),
				"one-click POST must flip contact_lists.status to 'unsubscribed'")
		})
	}

	// A receiver that does not offer one-click - RFC 8058 section 4 tells it not to unless a
	// DKIM signature covers both headers, which Mailpit never checks - opens the URL in a
	// browser, as people do, and scanners fetch it on their own. That must land on the
	// confirmation page and unsubscribe nobody.
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		t.Run(method+" lands on the confirmation page and unsubscribes nobody", func(t *testing.T) {
			msg, email, listID := sendToIndividual(t)

			resp := requestListUnsubscribeURL(t, method, listUnsubscribeURL(t, msg), "", nil)

			require.Equal(t, http.StatusSeeOther, resp.StatusCode)
			location, err := url.Parse(resp.Header.Get("Location"))
			require.NoError(t, err)
			assert.Equal(t, "/notification-center", location.Path)
			assert.Equal(t, "confirm_unsubscribe", location.Query().Get("action"))
			assert.Equal(t, email, location.Query().Get("email"))
			assert.Equal(t, listID, location.Query().Get("lid"))
			assert.Equal(t, domain.ContactListStatusActive, statusOf(t, email, listID),
				"opening the link must not unsubscribe without the visitor's confirmation")
		})
	}

	// The Custom Endpoint URL is stored as typed. A trailing slash must not produce
	// "//unsubscribe-oneclick": the mux answers that with a redirect, and RFC 8058
	// section 3.1 forbids redirecting the one-click POST.
	t.Run("custom endpoint with a trailing slash emits a single slash", func(t *testing.T) {
		require.NoError(t, factory.SetWorkspaceCustomEndpointURL(workspace.ID, serverURL+"/"))
		defer func() {
			require.NoError(t, factory.SetWorkspaceCustomEndpointURL(workspace.ID, serverURL))
		}()

		msg, email, listID := sendToIndividual(t)
		u := listUnsubscribeURL(t, msg)
		assert.Equal(t, "/unsubscribe-oneclick", u.Path)

		resp := requestListUnsubscribeURL(t, http.MethodPost, u,
			"application/x-www-form-urlencoded", strings.NewReader("List-Unsubscribe=One-Click"))
		assert.Equal(t, http.StatusOK, resp.StatusCode, "the one-click POST must not be redirected")
		assert.Equal(t, domain.ContactListStatusUnsubscribed, statusOf(t, email, listID))
	})

	// A test email sent from the console for a mailing list (issue #423) carries that list's
	// real links and the header, and its one-click unsubscribe works for a recipient who was
	// not on the list - typed with capitals, as people do.
	t.Run("test email for a list", func(t *testing.T) {
		list, err := factory.CreateList(workspace.ID)
		require.NoError(t, err)
		template, err := factory.CreateTemplate(workspace.ID, testutil.WithCodeModeTemplate(
			`<mjml><mj-body><mj-section><mj-column><mj-text>List: {{ list.name }} Link: {{ unsubscribe_url }}</mj-text></mj-column></mj-section></mj-body></mjml>`))
		require.NoError(t, err)
		senderID := integration.EmailProvider.Senders[0].ID

		testSend := func(t *testing.T, recipient string, extra map[string]interface{}) *testutil.MailpitMessage {
			t.Helper()
			req := map[string]interface{}{
				"workspace_id":    workspace.ID,
				"template_id":     template.ID,
				"integration_id":  integration.ID,
				"sender_id":       senderID,
				"recipient_email": recipient,
			}
			for k, v := range extra {
				req[k] = v
			}
			resp, err := client.TestTransactionalTemplate(req)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()
			require.Equal(t, http.StatusOK, resp.StatusCode)
			var result domain.TestTemplateResponse
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
			require.True(t, result.Success, "test send failed: %s", result.Error)

			msg, err := waitForEmailAndGetMessage(t, recipient, 10*time.Second)
			require.NoError(t, err, "Should receive the test email in Mailpit")
			return msg
		}

		t.Run("carries the list's links and a working one-click header", func(t *testing.T) {
			recipient := fmt.Sprintf("Tester.%s@Example.com", uuid.New().String()[:8])
			msg := testSend(t, recipient, map[string]interface{}{"list_id": list.ID})

			assert.Contains(t, msg.HTML, list.Name, "the body should name the real list")
			assert.Contains(t, msg.HTML, "lid="+list.ID, "the body's unsubscribe link should point at the real list")

			u := listUnsubscribeURL(t, msg)
			assert.Equal(t, "/unsubscribe-oneclick", u.Path)
			assert.Equal(t, list.ID, u.Query().Get("lids"))
			listUnsubscribePost := msg.Headers["List-Unsubscribe-Post"]
			require.NotEmpty(t, listUnsubscribePost)
			assert.Equal(t, "List-Unsubscribe=One-Click", listUnsubscribePost[0])

			resp := requestListUnsubscribeURL(t, http.MethodPost, u,
				"application/x-www-form-urlencoded", strings.NewReader("List-Unsubscribe=One-Click"))
			assert.Equal(t, http.StatusOK, resp.StatusCode)
			assert.Equal(t, domain.ContactListStatusUnsubscribed, statusOf(t, domain.NormalizeEmail(recipient), list.ID),
				"the one-click POST from the test must unsubscribe the recipient from the list")
		})

		t.Run("no header when the test leaves it out", func(t *testing.T) {
			recipient := fmt.Sprintf("notice.%s@example.com", uuid.New().String()[:8])
			msg := testSend(t, recipient, map[string]interface{}{
				"list_id":                      list.ID,
				"omit_list_unsubscribe_header": true,
			})

			assert.Contains(t, msg.HTML, "lid="+list.ID, "the body keeps the list's links")
			assert.Empty(t, msg.Headers["List-Unsubscribe"])
			assert.Empty(t, msg.Headers["List-Unsubscribe-Post"])
		})

		t.Run("no header without a list", func(t *testing.T) {
			recipient := fmt.Sprintf("nolist.%s@example.com", uuid.New().String()[:8])
			msg := testSend(t, recipient, nil)

			assert.Empty(t, msg.Headers["List-Unsubscribe"])
			assert.Empty(t, msg.Headers["List-Unsubscribe-Post"])
		})

		// The test is recorded for the list as a real send is, so a bounce reported on it
		// changes the recipient's subscription, as the dialog says.
		t.Run("a bounce on the test marks the subscriber bounced on the list", func(t *testing.T) {
			email, listID := subscribedContact(t)
			testSend(t, email, map[string]interface{}{"list_id": listID})

			db, err := factory.GetWorkspaceDB(workspace.ID)
			require.NoError(t, err)
			res, err := db.ExecContext(context.Background(),
				`UPDATE message_history SET bounced_at = NOW() WHERE contact_email = $1 AND list_id = $2`,
				email, listID)
			require.NoError(t, err)
			rows, err := res.RowsAffected()
			require.NoError(t, err)
			require.Equal(t, int64(1), rows, "the test should be recorded for the list")

			assert.Equal(t, domain.ContactListStatusBounced, statusOf(t, email, listID))
		})

		t.Run("an unknown list is refused and creates no contact", func(t *testing.T) {
			recipient := fmt.Sprintf("unknown.%s@example.com", uuid.New().String()[:8])
			resp, err := client.TestTransactionalTemplate(map[string]interface{}{
				"workspace_id":    workspace.ID,
				"template_id":     template.ID,
				"integration_id":  integration.ID,
				"sender_id":       senderID,
				"recipient_email": recipient,
				"list_id":         "nosuchlist",
			})
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()
			assert.Equal(t, http.StatusNotFound, resp.StatusCode)

			db, err := factory.GetWorkspaceDB(workspace.ID)
			require.NoError(t, err)
			var contacts int
			require.NoError(t, db.QueryRowContext(context.Background(),
				`SELECT COUNT(*) FROM contacts WHERE email = $1`, recipient).Scan(&contacts))
			assert.Equal(t, 0, contacts)
		})
	})

	// Real broadcasts are not sent through broadcasts.sendToIndividual but scheduled and
	// delivered by the email queue worker, which builds the header on its own path.
	t.Run("scheduled broadcast delivered by the email queue carries a working header", func(t *testing.T) {
		email, listID := subscribedContact(t)
		template, err := factory.CreateTemplate(workspace.ID)
		require.NoError(t, err)
		broadcast, err := factory.CreateBroadcast(workspace.ID,
			testutil.WithBroadcastAudience(domain.AudienceSettings{
				List:                listID,
				ExcludeUnsubscribed: true,
			}))
		require.NoError(t, err)

		broadcast.TestSettings.Variations[0].TemplateID = template.ID
		updateResp, err := client.UpdateBroadcast(map[string]interface{}{
			"workspace_id":  workspace.ID,
			"id":            broadcast.ID,
			"name":          broadcast.Name,
			"audience":      broadcast.Audience,
			"schedule":      broadcast.Schedule,
			"test_settings": broadcast.TestSettings,
		})
		require.NoError(t, err)
		_ = updateResp.Body.Close()

		scheduleResp, err := client.ScheduleBroadcast(map[string]interface{}{
			"workspace_id": workspace.ID,
			"id":           broadcast.ID,
			"send_now":     true,
		})
		require.NoError(t, err)
		_ = scheduleResp.Body.Close()

		_, err = testutil.WaitForBroadcastStatusWithExecution(t, client, broadcast.ID,
			[]string{"processed", "completed"}, 60*time.Second)
		require.NoError(t, err)

		msg, err := waitForEmailAndGetMessage(t, email, 20*time.Second)
		require.NoError(t, err, "Should receive the broadcast email in Mailpit")

		u := listUnsubscribeURL(t, msg)
		assert.Equal(t, serverURL, u.Scheme+"://"+u.Host)
		listUnsubscribePost := msg.Headers["List-Unsubscribe-Post"]
		require.NotEmpty(t, listUnsubscribePost)
		assert.Equal(t, "List-Unsubscribe=One-Click", listUnsubscribePost[0])

		resp := requestListUnsubscribeURL(t, http.MethodPost, u,
			"application/x-www-form-urlencoded", strings.NewReader("List-Unsubscribe=One-Click"))
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, domain.ContactListStatusUnsubscribed, statusOf(t, email, listID))
	})
}

// listUnsubscribeURL returns the URL in the delivered List-Unsubscribe header as a mail
// receiver reads it, failing unless it is the absolute, angle-bracketed URL RFC 2369
// and RFC 8058 require.
func listUnsubscribeURL(t *testing.T, msg *testutil.MailpitMessage) *url.URL {
	t.Helper()
	header := msg.Headers["List-Unsubscribe"]
	require.NotEmpty(t, header, "Email should have List-Unsubscribe header")

	value := strings.TrimSpace(header[0])
	require.True(t, strings.HasPrefix(value, "<") && strings.HasSuffix(value, ">"),
		"List-Unsubscribe should enclose its URL in angle brackets, got %q", value)

	u, err := url.Parse(strings.TrimSuffix(strings.TrimPrefix(value, "<"), ">"))
	require.NoError(t, err)
	require.True(t, u.IsAbs() && u.Host != "", "a mail client can only use an absolute URL, got %q", value)
	return u
}

// requestListUnsubscribeURL sends a request to the List-Unsubscribe URL exactly as
// delivered and does not follow redirects: RFC 8058 forbids answering the one-click POST
// with one, and the GET's redirect is asserted rather than followed. It uses curl's
// User-Agent, which is on the legacy bot blocklist, to prove automated one-click POSTs
// are not filtered out.
func requestListUnsubscribeURL(t *testing.T, method string, u *url.URL, contentType string, body io.Reader) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, u.String(), body)
	require.NoError(t, err)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("User-Agent", "curl/8.4.0")

	client := &http.Client{
		Timeout: 5 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

// waitForEmailAndGetMessage polls Mailpit for an email to the specified recipient
// and returns the full message with headers
func waitForEmailAndGetMessage(t *testing.T, recipientEmail string, timeout time.Duration) (*testutil.MailpitMessage, error) {
	deadline := time.Now().Add(timeout)
	pollInterval := 500 * time.Millisecond
	mailpitURL := "http://localhost:8025/api/v1/messages"

	httpClient := &http.Client{Timeout: 5 * time.Second}

	for time.Now().Before(deadline) {
		resp, err := httpClient.Get(mailpitURL)
		if err != nil {
			t.Logf("Failed to connect to Mailpit API: %v", err)
			time.Sleep(pollInterval)
			continue
		}

		var apiResp testutil.MailpitMessagesResponse
		err = json.NewDecoder(resp.Body).Decode(&apiResp)
		resp.Body.Close()

		if err != nil {
			t.Logf("Failed to decode Mailpit response: %v", err)
			time.Sleep(pollInterval)
			continue
		}

		t.Logf("Mailpit has %d messages", len(apiResp.Messages))

		// Check each message for matching recipient. A failed read of the newest match
		// polls again rather than falling back to an older email to the same recipient.
	scan:
		for _, msg := range apiResp.Messages {
			for _, to := range msg.To {
				if strings.EqualFold(recipientEmail, to.Address) {
					t.Logf("Found email for recipient: %s", recipientEmail)
					// Get the full message with headers
					fullMsg, err := testutil.GetMailpitMessage(t, msg.ID)
					if err != nil {
						t.Logf("Failed to get full message: %v", err)
						break scan
					}
					return fullMsg, nil
				}
			}
		}

		time.Sleep(pollInterval)
	}

	return nil, fmt.Errorf("timeout waiting for email to %s after %v", recipientEmail, timeout)
}
