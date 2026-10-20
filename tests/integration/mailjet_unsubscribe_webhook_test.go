//go:build integration
// +build integration

package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
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
)

// TestMailjetUnsubscribeWebhook sends real broadcast and automation emails through
// Mailpit, then reports unsubscribes on them with Mailjet "unsub" events posted to the
// inbound webhook endpoint, as the JSON array Mailjet's Version 2 webhooks post. Mailjet
// names the email by the CustomID Notifuse gives every email it sends through Mailjet
// (internal/service/mailjet_service.go); the emails here go out through SMTP, whose
// X-Message-ID header carries the same ID.
//
// The Mailjet parser recorded an "unsub" event as a spam complaint: the contact became
// complained on the email's list, could not subscribe to it again, and the email counted
// as a complaint, with the email.complained events of one. It is now recorded as the
// email's own unsubscribe link records an opt-out.
func TestMailjetUnsubscribeWebhook(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		// The webhook subscription below targets a placeholder URL; nothing delivers it
		// (the harness starts no delivery worker), the queued deliveries are read instead.
		cfg.Webhook.AllowPrivateDeliveryHosts = true
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	// Broadcasts and automation emails are sent by the email queue worker and the
	// automation scheduler, which the harness only runs on request.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, suite.ServerManager.StartBackgroundWorkers(ctx))

	client := suite.APIClient
	factory := suite.DataFactory
	bg := context.Background()

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))
	_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID)
	require.NoError(t, err)
	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(workspace.ID)

	// Emails go out through SMTP to Mailpit; the Mailjet integration only receives the
	// webhook events, which name the email, not the integration that sent it.
	mailjet, err := factory.CreateIntegration(workspace.ID,
		testutil.WithIntegrationName("Test Mailjet Integration"),
		testutil.WithIntegrationEmailProvider(domain.EmailProvider{
			Kind:    domain.EmailProviderKindMailjet,
			Senders: []domain.EmailSender{domain.NewEmailSender("test@example.com", "Test Sender")},
			Mailjet: &domain.MailjetSettings{APIKey: "test-api-key", SecretKey: "test-secret-key"},
		}))
	require.NoError(t, err)
	webhookURL := fmt.Sprintf("/webhooks/email?provider=mailjet&workspace_id=%s&integration_id=%s", workspace.ID, mailjet.ID)

	db, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	subscriptionID := createZapierSubscription(t, suite, workspace.ID, "Unsubscribes and complaints",
		"https://hooks.example.com/mailjet-unsub",
		[]string{"email.unsubscribed", "email.complained", "list.unsubscribed", "list.complained"}, nil, nil)

	post := func(t *testing.T, path string, body map[string]interface{}) []byte {
		t.Helper()
		resp, err := client.Post(path, body)
		require.NoError(t, err)
		raw, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode, "%s: %s", path, raw)
		return raw
	}

	// mailjetEvent builds an event with the fields Mailjet documents for its kind.
	// mj_list_id names a Mailjet contact list, not a Notifuse one.
	mailjetEvent := func(kind, messageID, email string) map[string]interface{} {
		event := map[string]interface{}{
			"event":          kind,
			"time":           time.Now().Unix(),
			"MessageID":      int64(288230376151711744),
			"Message_GUID":   uuid.New().String(),
			"email":          email,
			"mj_campaign_id": 7173,
			"mj_contact_id":  320,
			"customcampaign": "",
			"CustomID":       messageID,
			"Payload":        "",
		}
		switch kind {
		case "unsub":
			event["mj_list_id"] = 1
			event["ip"] = "127.0.0.1"
			event["geo"] = "FR"
			event["agent"] = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36"
		case "spam":
			event["source"] = "JMRPP"
		}
		return event
	}
	// postMailjet posts one webhook delivery and expects 200: Mailjet posts a delivery
	// again when it gets another answer.
	postMailjet := func(t *testing.T, body []byte) {
		t.Helper()
		resp, err := client.PostRaw(webhookURL, string(body))
		require.NoError(t, err)
		raw, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode, "Mailjet webhook POST: %s", raw)
	}
	postMailjetEvents := func(t *testing.T, events ...map[string]interface{}) {
		t.Helper()
		body, err := json.Marshal(events)
		require.NoError(t, err)
		postMailjet(t, body)
	}
	reportUnsub := func(t *testing.T, messageID, email string) {
		t.Helper()
		postMailjetEvents(t, mailjetEvent("unsub", messageID, email))
	}
	reportSpam := func(t *testing.T, messageID, email string) {
		t.Helper()
		postMailjetEvents(t, mailjetEvent("spam", messageID, email))
	}

	newContact := func(t *testing.T, name string) string {
		t.Helper()
		email := fmt.Sprintf("mailjet-unsub-%s-%s@example.com", name, uuid.New().String()[:8])
		_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
		require.NoError(t, err)
		return email
	}
	addToList := func(t *testing.T, email, listID string) {
		t.Helper()
		_, err := factory.CreateContactList(workspace.ID,
			testutil.WithContactListEmail(email),
			testutil.WithContactListListID(listID),
			testutil.WithContactListStatus(domain.ContactListStatusActive))
		require.NoError(t, err)
	}

	type listRow struct {
		status  domain.ContactListStatus
		removed bool
	}
	rowOf := func(t *testing.T, email, listID string) *listRow {
		t.Helper()
		var status string
		var deletedAt sql.NullTime
		err := db.QueryRowContext(bg,
			`SELECT status, deleted_at FROM contact_lists WHERE email = $1 AND list_id = $2`,
			email, listID).Scan(&status, &deletedAt)
		if err == sql.ErrNoRows {
			return nil
		}
		require.NoError(t, err)
		return &listRow{status: domain.ContactListStatus(status), removed: deletedAt.Valid}
	}

	// marks reports which of the two events the email is marked with
	type marks struct {
		unsubscribed bool
		complained   bool
	}
	marksOf := func(t *testing.T, messageID string) marks {
		t.Helper()
		var unsubscribedAt, complainedAt sql.NullTime
		require.NoError(t, db.QueryRowContext(bg,
			`SELECT unsubscribed_at, complained_at FROM message_history WHERE id = $1`,
			messageID).Scan(&unsubscribedAt, &complainedAt))
		return marks{unsubscribed: unsubscribedAt.Valid, complained: complainedAt.Valid}
	}

	// queued returns the event types of the webhook deliveries queued about email, sorted
	queued := func(t *testing.T, email string) []string {
		t.Helper()
		kinds := []string{}
		for _, d := range b24Deliveries(t, db, subscriptionID, email) {
			kinds = append(kinds, d.eventType)
		}
		sort.Strings(kinds)
		return kinds
	}

	// An SMTP send records its message_history row only once the server accepted it
	waitForMessageRow := func(t *testing.T, messageID string) {
		t.Helper()
		testutil.WaitForCondition(t, func() bool {
			var n int
			_ = db.QueryRowContext(bg, `SELECT COUNT(*) FROM message_history WHERE id = $1`, messageID).Scan(&n)
			return n == 1
		}, 10*time.Second, "waiting for the message_history row of "+messageID)
	}

	// sendBroadcast sends a broadcast to the list now, through the API, and returns its ID
	// and the Notifuse message ID the email of each recipient carries.
	sendBroadcast := func(t *testing.T, listID string, recipients ...string) (string, map[string]string) {
		t.Helper()
		subject := "mailjet-unsub-" + uuid.New().String()[:8]
		template, err := factory.CreateTemplate(workspace.ID, testutil.WithTemplateSubject(subject))
		require.NoError(t, err)

		resp, err := client.CreateBroadcast(map[string]interface{}{
			"workspace_id": workspace.ID,
			"name":         subject,
			"audience":     map[string]interface{}{"list": listID},
			"test_settings": map[string]interface{}{
				"enabled":    false,
				"variations": []map[string]interface{}{{"template_id": template.ID}},
			},
		})
		require.NoError(t, err)
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusCreated, resp.StatusCode, "create broadcast: %s", body)
		var created struct {
			Broadcast struct {
				ID string `json:"id"`
			} `json:"broadcast"`
		}
		require.NoError(t, json.Unmarshal(body, &created))
		broadcastID := created.Broadcast.ID

		resp, err = client.ScheduleBroadcast(map[string]interface{}{
			"workspace_id": workspace.ID,
			"id":           broadcastID,
			"send_now":     true,
		})
		require.NoError(t, err)
		body, _ = io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode, "schedule broadcast: %s", body)
		_, err = testutil.WaitForBroadcastStatusWithExecution(t, client, broadcastID,
			[]string{"processed", "completed"}, 60*time.Second)
		require.NoError(t, err)

		messageIDs := make(map[string]string, len(recipients))
		for _, email := range recipients {
			msg, err := waitForEmailAndGetMessage(t, email, 30*time.Second)
			require.NoError(t, err, "the broadcast email should reach Mailpit")
			messageIDs[email] = deliveredHeader(t, msg, "X-Message-ID")
			waitForMessageRow(t, messageIDs[email])
		}
		return broadcastID, messageIDs
	}

	broadcastStats := func(t *testing.T, broadcastID string) domain.MessageHistoryStatusSum {
		t.Helper()
		resp, err := client.Get("/api/messages.broadcastStats", map[string]string{
			"workspace_id": workspace.ID,
			"broadcast_id": broadcastID,
		})
		require.NoError(t, err)
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode, "broadcast stats: %s", body)
		var got struct {
			Stats domain.MessageHistoryStatusSum `json:"stats"`
		}
		require.NoError(t, json.Unmarshal(body, &got))
		return got.Stats
	}

	t.Run("broadcast email: an unsubscribe is recorded as the email's unsubscribe link records it, not as a complaint", func(t *testing.T) {
		list, err := factory.CreateList(workspace.ID)
		require.NoError(t, err)
		email := newContact(t, "broadcast")
		addToList(t, email, list.ID)
		broadcastID, messageIDs := sendBroadcast(t, list.ID, email)
		messageID := messageIDs[email]

		reportUnsub(t, messageID, email)

		assert.Equal(t, &listRow{status: domain.ContactListStatusUnsubscribed}, rowOf(t, email, list.ID))
		assert.Equal(t, marks{unsubscribed: true}, marksOf(t, messageID), "the email is marked unsubscribed, not complained")
		assert.Equal(t, 1, timelineKindCount(t, db, email, "email.unsubscribed", messageID))
		assert.Equal(t, 1, timelineKindCount(t, db, email, "list.unsubscribed", list.ID))
		assert.Zero(t, timelineKindCount(t, db, email, "email.complained", messageID))
		assert.Zero(t, timelineKindCount(t, db, email, "list.complained", list.ID))
		assert.Equal(t, []string{"email.unsubscribed", "list.unsubscribed"}, queued(t, email))

		stats := broadcastStats(t, broadcastID)
		assert.Equal(t, 1, stats.TotalUnsubscribed)
		assert.Zero(t, stats.TotalComplained)

		var storedType string
		require.NoError(t, db.QueryRowContext(bg,
			`SELECT type FROM inbound_webhook_events WHERE message_id = $1`, messageID).Scan(&storedType))
		assert.Equal(t, "unsubscribe", storedType, "the Mailjet event is stored as an unsubscribe")

		// The webhook event log lists it under its own type, which its filter asks the API for
		resp, err := client.Get("/api/inboundWebhookEvents.list", map[string]string{
			"workspace_id":    workspace.ID,
			"event_type":      string(domain.EmailEventUnsubscribe),
			"recipient_email": email,
		})
		require.NoError(t, err)
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode, "list the unsubscribe events: %s", body)
		var listed domain.InboundWebhookEventListResult
		require.NoError(t, json.Unmarshal(body, &listed))
		require.Len(t, listed.Events, 1)
		assert.Equal(t, domain.EmailEventUnsubscribe, listed.Events[0].Type)
		require.NotNil(t, listed.Events[0].MessageID)
		assert.Equal(t, messageID, *listed.Events[0].MessageID)

		// An unsubscribed contact can subscribe again, confirming by double opt-in; a
		// complained one is refused
		raw := post(t, "/api/lists.subscribe", map[string]interface{}{
			"workspace_id": workspace.ID,
			"contact":      map[string]interface{}{"email": email},
			"list_ids":     []string{list.ID},
		})
		var subscribed struct {
			ContactLists []struct {
				Status string `json:"status"`
			} `json:"contact_lists"`
		}
		require.NoError(t, json.Unmarshal(raw, &subscribed))
		require.Len(t, subscribed.ContactLists, 1)
		assert.Equal(t, string(domain.ContactListStatusPending), subscribed.ContactLists[0].Status)
	})

	// A broadcast for the cases below, each with a recipient of its own
	edgeList, err := factory.CreateList(workspace.ID)
	require.NoError(t, err)
	names := []string{"twice", "complained-first", "unsubscribed-first", "bounced", "other-recipient", "erased"}
	recipient := make(map[string]string, len(names))
	recipients := make([]string, 0, len(names))
	for _, name := range names {
		recipient[name] = newContact(t, name)
		addToList(t, recipient[name], edgeList.ID)
		recipients = append(recipients, recipient[name])
	}
	_, edgeMessageIDs := sendBroadcast(t, edgeList.ID, recipients...)

	t.Run("the same delivery posted twice is recorded once", func(t *testing.T) {
		email := recipient["twice"]
		messageID := edgeMessageIDs[email]
		body, err := json.Marshal([]map[string]interface{}{mailjetEvent("unsub", messageID, email)})
		require.NoError(t, err)

		// As when Mailjet did not get the answer to the first one
		postMailjet(t, body)
		postMailjet(t, body)

		assert.Equal(t, &listRow{status: domain.ContactListStatusUnsubscribed}, rowOf(t, email, edgeList.ID))
		assert.Equal(t, marks{unsubscribed: true}, marksOf(t, messageID))
		assert.Equal(t, 1, timelineKindCount(t, db, email, "email.unsubscribed", messageID))
		assert.Equal(t, 1, timelineKindCount(t, db, email, "list.unsubscribed", edgeList.ID))
		assert.Equal(t, []string{"email.unsubscribed", "list.unsubscribed"}, queued(t, email))
	})

	// People who report spam often unsubscribe too
	t.Run("a complaint, then an unsubscribe: the complaint is kept", func(t *testing.T) {
		email := recipient["complained-first"]
		messageID := edgeMessageIDs[email]

		reportSpam(t, messageID, email)
		reportUnsub(t, messageID, email)

		assert.Equal(t, &listRow{status: domain.ContactListStatusComplained}, rowOf(t, email, edgeList.ID),
			"an unsubscribe does not turn a complaint into an opt-out")
		assert.Equal(t, marks{unsubscribed: true, complained: true}, marksOf(t, messageID))
		assert.Zero(t, timelineKindCount(t, db, email, "list.unsubscribed", edgeList.ID))
		assert.Equal(t, []string{"email.complained", "email.unsubscribed", "list.complained"}, queued(t, email))
	})

	t.Run("an unsubscribe, then a complaint: the complaint replaces it", func(t *testing.T) {
		email := recipient["unsubscribed-first"]
		messageID := edgeMessageIDs[email]

		reportUnsub(t, messageID, email)
		require.Equal(t, &listRow{status: domain.ContactListStatusUnsubscribed}, rowOf(t, email, edgeList.ID))
		reportSpam(t, messageID, email)

		assert.Equal(t, &listRow{status: domain.ContactListStatusComplained}, rowOf(t, email, edgeList.ID))
		assert.Equal(t, marks{unsubscribed: true, complained: true}, marksOf(t, messageID))
		assert.Equal(t, []string{"email.complained", "email.unsubscribed", "list.complained", "list.unsubscribed"}, queued(t, email))
	})

	t.Run("a contact whose address bounced since the email: the bounce is kept", func(t *testing.T) {
		email := recipient["bounced"]
		messageID := edgeMessageIDs[email]
		// A hard bounce on another email marks the address bounced
		require.NoError(t, suite.ServerManager.GetApp().GetContactRepository().
			MarkEmailsAsBounced(bg, workspace.ID, []string{email}, time.Now().UTC()))
		require.Equal(t, &listRow{status: domain.ContactListStatusBounced}, rowOf(t, email, edgeList.ID))

		reportUnsub(t, messageID, email)

		assert.Equal(t, &listRow{status: domain.ContactListStatusBounced}, rowOf(t, email, edgeList.ID))
		assert.Equal(t, marks{unsubscribed: true}, marksOf(t, messageID))
	})

	// Such as someone the email was copied to
	t.Run("an unsubscribe of another address than the email's recipient records nothing", func(t *testing.T) {
		email := recipient["other-recipient"]
		messageID := edgeMessageIDs[email]

		reportUnsub(t, messageID, fmt.Sprintf("someone-else-%s@example.com", uuid.New().String()[:8]))

		assert.Equal(t, &listRow{status: domain.ContactListStatusActive}, rowOf(t, email, edgeList.ID))
		assert.Equal(t, marks{}, marksOf(t, messageID))
		assert.Empty(t, queued(t, email))
	})

	t.Run("a contact erased since the email: nothing is stored", func(t *testing.T) {
		email := recipient["erased"]
		messageID := edgeMessageIDs[email]
		post(t, "/api/contacts.delete", map[string]interface{}{"workspace_id": workspace.ID, "email": email})

		reportUnsub(t, messageID, email)

		var n int
		require.NoError(t, db.QueryRowContext(bg,
			`SELECT COUNT(*) FROM contact_lists WHERE email = $1 OR email = 'DELETED_EMAIL'`, email).Scan(&n))
		assert.Zero(t, n, "neither the erased address nor its redacted stand-in is stored")
		assert.Equal(t, marks{}, marksOf(t, messageID))
	})

	t.Run("a list deleted since the email: nothing is recorded", func(t *testing.T) {
		list, err := factory.CreateList(workspace.ID)
		require.NoError(t, err)
		email := newContact(t, "deleted-list")
		addToList(t, email, list.ID)
		_, messageIDs := sendBroadcast(t, list.ID, email)
		messageID := messageIDs[email]
		post(t, "/api/lists.delete", map[string]interface{}{"workspace_id": workspace.ID, "id": list.ID})

		reportUnsub(t, messageID, email)

		assert.Equal(t, &listRow{status: domain.ContactListStatusActive, removed: true}, rowOf(t, email, list.ID),
			"the row the deletion removed is left as it was")
		assert.Equal(t, marks{}, marksOf(t, messageID))
		assert.Empty(t, queued(t, email))
	})

	t.Run("an email sent for no list, as a transactional one is: nothing is recorded", func(t *testing.T) {
		list, err := factory.CreateList(workspace.ID)
		require.NoError(t, err)
		email := newContact(t, "no-list")
		addToList(t, email, list.ID)
		messageID := uuid.New().String()
		_, err = db.ExecContext(bg, `INSERT INTO message_history (id, contact_email, template_id,
				template_version, channel, message_data, sent_at)
			VALUES ($1, $2, 'tpl-transactional', 1, 'email', '{}'::jsonb, NOW())`, messageID, email)
		require.NoError(t, err)

		reportUnsub(t, messageID, email)

		assert.Equal(t, &listRow{status: domain.ContactListStatusActive}, rowOf(t, email, list.ID),
			"the contact's lists are left as they were")
		assert.Equal(t, marks{}, marksOf(t, messageID))
		assert.Empty(t, queued(t, email))
	})

	// Mailjet's webhooks cover the whole Mailjet account, so they report emails Notifuse
	// did not send too
	t.Run("an email Notifuse did not send: answered with 200, nothing is recorded", func(t *testing.T) {
		list, err := factory.CreateList(workspace.ID)
		require.NoError(t, err)
		email := newContact(t, "unknown-message")
		addToList(t, email, list.ID)
		withoutCustomID := mailjetEvent("unsub", "", email)
		delete(withoutCustomID, "CustomID")

		postMailjetEvents(t, mailjetEvent("unsub", uuid.New().String(), email), withoutCustomID)

		assert.Equal(t, &listRow{status: domain.ContactListStatusActive}, rowOf(t, email, list.ID))
		assert.Empty(t, queued(t, email))
	})

	automationList, err := factory.CreateList(workspace.ID)
	require.NoError(t, err)
	template, err := factory.CreateTemplate(workspace.ID)
	require.NoError(t, err)
	require.Equal(t, "marketing", template.Category, "only marketing and blog emails check the list status")
	automationID := shortuuid.New()
	eventName := "mailjet_unsub_" + uuid.New().String()[:8]
	createActiveAutomation(t, client, workspace.ID, automationID, automationList.ID, eventName, template.ID)

	// sendAutomationEmail fires the automation's event and returns the Notifuse message ID
	// the delivered email carries.
	sendAutomationEmail := func(t *testing.T, email string) string {
		t.Helper()
		require.NoError(t, factory.CreateCustomEvent(workspace.ID, email, eventName, map[string]interface{}{}))
		msg, err := waitForEmailAndGetMessage(t, email, 20*time.Second)
		require.NoError(t, err, "the automation email should reach Mailpit")
		messageID := deliveredHeader(t, msg, "X-Message-ID")
		waitForMessageRow(t, messageID)

		var storedList sql.NullString
		require.NoError(t, db.QueryRowContext(bg,
			`SELECT list_id FROM message_history WHERE id = $1`, messageID).Scan(&storedList))
		require.Equal(t, automationList.ID, storedList.String, "the automation email records its list")
		return messageID
	}
	// expectNextEmailSkipped fires the event again and checks the automation skips its email.
	expectNextEmailSkipped := func(t *testing.T, email string) {
		t.Helper()
		require.NoError(t, factory.CreateCustomEvent(workspace.ID, email, eventName, map[string]interface{}{}))
		// The first enrolment completed, so only the new one can report exited
		ca := waitForAutomationStatus(t, factory, workspace.ID, automationID, email, domain.ContactAutomationStatusExited, 15*time.Second)
		require.NotNil(t, ca.ExitReason)
		assert.Equal(t, string(domain.ContactListStatusUnsubscribed), *ca.ExitReason, "the next email is skipped, not sent")
		assert.Equal(t, 1, deliveredCountTo(t, email), "only the first email was delivered")
	}

	t.Run("automation email to a member of its list: unsubscribed there, and the next email is skipped", func(t *testing.T) {
		email := newContact(t, "automation-member")
		addToList(t, email, automationList.ID)
		messageID := sendAutomationEmail(t, email)

		reportUnsub(t, messageID, email)

		assert.Equal(t, &listRow{status: domain.ContactListStatusUnsubscribed}, rowOf(t, email, automationList.ID))
		assert.Equal(t, marks{unsubscribed: true}, marksOf(t, messageID))
		assert.Equal(t, []string{"email.unsubscribed", "list.unsubscribed"}, queued(t, email))
		expectNextEmailSkipped(t, email)
	})

	t.Run("automation email to a contact not on its list: an unsubscribed row is recorded there", func(t *testing.T) {
		email := newContact(t, "automation-non-member")
		messageID := sendAutomationEmail(t, email)
		require.Nil(t, rowOf(t, email, automationList.ID), "a non-member has no row before the unsubscribe")

		reportUnsub(t, messageID, email)

		assert.Equal(t, &listRow{status: domain.ContactListStatusUnsubscribed}, rowOf(t, email, automationList.ID))
		assert.Equal(t, 1, b24TimelineInserts(t, db, email, "list.unsubscribed", automationList.ID))
		assert.Equal(t, marks{unsubscribed: true}, marksOf(t, messageID))
		assert.Equal(t, []string{"email.unsubscribed", "list.unsubscribed"}, queued(t, email))
		expectNextEmailSkipped(t, email)
	})

	t.Run("automation email to a contact removed from its list: the opt-out is kept on the removed row", func(t *testing.T) {
		email := newContact(t, "automation-removed")
		addToList(t, email, automationList.ID)
		post(t, "/api/contactLists.removeContact", map[string]interface{}{
			"workspace_id": workspace.ID, "email": email, "list_id": automationList.ID,
		})
		messageID := sendAutomationEmail(t, email)

		reportUnsub(t, messageID, email)

		assert.Equal(t, &listRow{status: domain.ContactListStatusUnsubscribed, removed: true}, rowOf(t, email, automationList.ID),
			"the opt-out is recorded, and the contact is not put back on the list")
		assert.Zero(t, timelineKindCount(t, db, email, "list.unsubscribed", automationList.ID),
			"a removed contact is not on the list, so the status on their row is no list event")
		assert.Equal(t, marks{unsubscribed: true}, marksOf(t, messageID))
		assert.Equal(t, []string{"email.unsubscribed"}, queued(t, email))
		expectNextEmailSkipped(t, email)
	})
}
