package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"net/http"
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

// TestWebhookRedelivery: a provider that gets no answer, or a server error, posts the same
// events to /webhooks/email again. The redelivery stores no second event and no second
// timeline entry, so a soft bounce counts once toward the threshold. It still applies a
// bounce or an unsubscribe to a list row unchanged since Notifuse first received the
// event, which is what repairs a delivery whose processing failed after the events were
// stored, and leaves a row changed since, such as a bounce cleared or a contact subscribed
// again, as it is.
func TestWebhookRedelivery(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer func() { suite.Cleanup() }()

	client := suite.APIClient
	factory := suite.DataFactory
	bg := context.Background()

	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	db, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)
	template, err := factory.CreateTemplate(workspace.ID)
	require.NoError(t, err)
	list, err := factory.CreateList(workspace.ID)
	require.NoError(t, err)
	contactListRepo := suite.ServerManager.GetApp().GetContactListRepository()

	sender := []domain.EmailSender{domain.NewEmailSender("test@example.com", "Test Sender")}
	webhookURL := func(t *testing.T, provider domain.EmailProvider) string {
		t.Helper()
		integration, err := factory.CreateIntegration(workspace.ID,
			testutil.WithIntegrationName("Redelivery "+string(provider.Kind)),
			testutil.WithIntegrationEmailProvider(provider))
		require.NoError(t, err)
		return fmt.Sprintf("/webhooks/email?provider=%s&workspace_id=%s&integration_id=%s", provider.Kind, workspace.ID, integration.ID)
	}
	ses, err := factory.CreateSESIntegration(workspace.ID)
	require.NoError(t, err)
	sesURL := fmt.Sprintf("/webhooks/email?provider=ses&workspace_id=%s&integration_id=%s", workspace.ID, ses.ID)

	newContact := func(t *testing.T) string {
		t.Helper()
		email := fmt.Sprintf("redelivery-%s@example.com", uuid.New().String()[:8])
		_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
		require.NoError(t, err)
		_, err = factory.CreateContactList(workspace.ID,
			testutil.WithContactListEmail(email),
			testutil.WithContactListListID(list.ID),
			testutil.WithContactListStatus(domain.ContactListStatusActive))
		require.NoError(t, err)
		return email
	}
	post := func(t *testing.T, url, body string) {
		t.Helper()
		resp, err := client.PostRaw(url, body)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		require.Equal(t, http.StatusOK, resp.StatusCode, "webhook POST must return 200")
	}
	storedFor := func(t *testing.T, messageID string) int {
		t.Helper()
		var n int
		require.NoError(t, db.QueryRowContext(bg, `SELECT COUNT(*) FROM inbound_webhook_events WHERE message_id = $1`, messageID).Scan(&n))
		return n
	}
	timelineEventsOf := func(t *testing.T, email string) int {
		t.Helper()
		var n int
		require.NoError(t, db.QueryRowContext(bg,
			`SELECT COUNT(*) FROM contact_timeline WHERE email = $1 AND entity_type = 'inbound_webhook_event'`, email).Scan(&n))
		return n
	}
	statusOf := func(t *testing.T, email string) domain.ContactListStatus {
		t.Helper()
		row, err := contactListRepo.GetContactListByIDs(bg, workspace.ID, email, list.ID)
		require.NoError(t, err)
		return row.Status
	}
	at := time.Now().UTC().Truncate(time.Second)

	// Each provider's soft bounce of the email; eventID sets what identifies the event for
	// the provider: its event ID, or its message ID for Mailjet (SMTP has none of its own)
	providers := []struct {
		name       string
		url        string
		softBounce func(messageID, email, eventID string) string
	}{
		{"SES", sesURL, func(messageID, email, eventID string) string {
			return sesSoftBounceWithSNSID(messageID, email, "sns-"+eventID, at)
		}},
		{"Postmark", webhookURL(t, domain.EmailProvider{Kind: domain.EmailProviderKindPostmark, Senders: sender,
			Postmark: &domain.PostmarkSettings{ServerToken: "test-token"}}),
			func(messageID, email, eventID string) string {
				return postmarkBouncePayload(messageID, email, "SoftBounce", numericID(eventID), at)
			}},
		{"Mailgun", webhookURL(t, domain.EmailProvider{Kind: domain.EmailProviderKindMailgun, Senders: sender,
			Mailgun: &domain.MailgunSettings{APIKey: "test-key", Domain: "example.com"}}),
			func(messageID, email, eventID string) string {
				return fmt.Sprintf(`{"event-data":{"event":"failed","id":%q,"severity":"temporary","timestamp":%d,"recipient":%q,"reason":"greylisted","message":{"headers":{"message-id":"mg-%s"}},"user-variables":{"notifuse_message_id":%q}}}`,
					eventID, at.Unix(), email, messageID, messageID)
			}},
		{"SparkPost", webhookURL(t, domain.EmailProvider{Kind: domain.EmailProviderKindSparkPost, Senders: sender,
			SparkPost: &domain.SparkPostSettings{APIKey: "test-api-key", Endpoint: "https://api.sparkpost.com"}}),
			func(messageID, email, eventID string) string {
				return fmt.Sprintf(`[{"msys":{"message_event":{"type":"bounce","event_id":%q,"rcpt_to":%q,"message_id":"sp-%s","timestamp":%q,"bounce_class":"21","rcpt_meta":{"notifuse_message_id":%q}}}}]`,
					eventID, email, messageID, at.Format(time.RFC3339), messageID)
			}},
		{"Mailjet", webhookURL(t, domain.EmailProvider{Kind: domain.EmailProviderKindMailjet, Senders: sender,
			Mailjet: &domain.MailjetSettings{APIKey: "test-api-key", SecretKey: "test-secret-key"}}),
			func(messageID, email, eventID string) string {
				return fmt.Sprintf(`[{"event":"bounce","time":%d,"MessageID":%s,"email":%q,"hard_bounce":false,"error":"mailbox full","CustomID":%q}]`,
					at.Unix(), numericID(eventID), email, messageID)
			}},
		{"SMTP", webhookURL(t, domain.EmailProvider{Kind: domain.EmailProviderKindSMTP, Senders: sender,
			SMTP: &domain.SMTPSettings{Host: "localhost", Port: 1025}}),
			func(messageID, email, eventID string) string {
				return fmt.Sprintf(`{"event":"bounce","timestamp":%q,"message_id":%q,"recipient":%q,"bounce_category":"soft","diagnostic_code":"452 mailbox full"}`,
					at.Format(time.RFC3339), messageID, email)
			}},
		{"SendGrid", webhookURL(t, domain.EmailProvider{Kind: domain.EmailProviderKindSendGrid, Senders: sender,
			SendGrid: &domain.SendGridSettings{APIKey: "test-key"}}),
			func(messageID, email, eventID string) string {
				return fmt.Sprintf(`[{"email":%q,"timestamp":%d,"event":"dropped","reason":"Bounced Address","sg_event_id":%q,"sg_message_id":"sg-%s","notifuse_message_id":%q}]`,
					email, at.Unix(), eventID, messageID, messageID)
			}},
	}

	for _, p := range providers {
		t.Run(p.name+": a soft bounce posted twice is stored once", func(t *testing.T) {
			email := newContact(t)
			messageID := sentEmailID(t, factory, workspace.ID, template.ID, email)
			body := p.softBounce(messageID, email, uuid.New().String())

			post(t, p.url, body)
			post(t, p.url, body)

			assert.Equal(t, 1, storedFor(t, messageID))
			assert.Equal(t, 1, timelineEventsOf(t, email))
			assert.Equal(t, domain.ContactListStatusActive, statusOf(t, email))
		})
	}

	t.Run("soft bounces each posted twice reach the threshold only with as many emails", func(t *testing.T) {
		email := newContact(t)
		for i := 1; i < domain.DefaultSoftBounceThreshold; i++ {
			messageID := sentEmailID(t, factory, workspace.ID, template.ID, email)
			body := sesSoftBounceWithSNSID(messageID, email, "sns-"+uuid.New().String(), at)
			post(t, sesURL, body)
			post(t, sesURL, body)
			require.Equalf(t, domain.ContactListStatusActive, statusOf(t, email), "after soft bounce %d, posted twice", i)
		}

		messageID := sentEmailID(t, factory, workspace.ID, template.ID, email)
		post(t, sesURL, sesSoftBounceWithSNSID(messageID, email, "sns-"+uuid.New().String(), at))
		assert.Equal(t, domain.ContactListStatusBounced, statusOf(t, email))
	})

	t.Run("a Postmark hard bounce, with the numeric ID Postmark sends, is applied and stored once", func(t *testing.T) {
		email := newContact(t)
		messageID := sentEmailID(t, factory, workspace.ID, template.ID, email)
		body := postmarkBouncePayload(messageID, email, "HardBounce", numericID(uuid.New().String()), at)

		post(t, providers[1].url, body)
		post(t, providers[1].url, body)

		assert.Equal(t, 1, storedFor(t, messageID))
		assert.Equal(t, domain.ContactListStatusBounced, statusOf(t, email))
	})

	mailjetURL := providers[4].url
	mailjetUnsub := func(messageID, email string) string {
		return fmt.Sprintf(`[{"event":"unsub","time":%d,"MessageID":%s,"email":%q,"mj_list_id":1,"CustomID":%q}]`,
			at.Unix(), numericID(messageID), email, messageID)
	}
	sentForList := func(t *testing.T, email string) string {
		t.Helper()
		message, err := factory.CreateMessageHistory(workspace.ID,
			testutil.WithMessageHistoryContactEmail(email),
			testutil.WithMessageTemplate(template.ID),
			testutil.WithMessageListID(list.ID))
		require.NoError(t, err)
		return message.ID
	}
	clearBounce := func(t *testing.T, email string) {
		t.Helper()
		require.NoError(t, contactListRepo.UpdateContactListStatus(bg, workspace.ID, email, list.ID, domain.ContactListStatusActive))
	}
	// unapplied puts the row back as it was before the event's delivery applied anything,
	// unchanged since the event was stored: the state a delivery that failed after storing
	// its events leaves
	unapplied := func(t *testing.T, email, messageID string) {
		t.Helper()
		var storedAt time.Time
		require.NoError(t, db.QueryRowContext(bg,
			`SELECT created_at FROM inbound_webhook_events WHERE message_id = $1`, messageID).Scan(&storedAt))
		_, err := db.ExecContext(bg,
			`UPDATE contact_lists SET status = 'active', updated_at = $3 WHERE email = $1 AND list_id = $2`,
			email, list.ID, storedAt.Add(-time.Second))
		require.NoError(t, err)
		_, err = db.ExecContext(bg, `UPDATE message_history SET unsubscribed_at = NULL WHERE id = $1`, messageID)
		require.NoError(t, err)
	}
	unsubscribedAt := func(t *testing.T, messageID string) *time.Time {
		t.Helper()
		var at *time.Time
		require.NoError(t, db.QueryRowContext(bg, `SELECT unsubscribed_at FROM message_history WHERE id = $1`, messageID).Scan(&at))
		return at
	}

	t.Run("a hard bounce posted again keeps a bounce cleared since", func(t *testing.T) {
		email := newContact(t)
		messageID := sentEmailID(t, factory, workspace.ID, template.ID, email)
		body := sesBouncePayloadAt(messageID, email, "Permanent", "General", "smtp; 550 5.1.1 user unknown", at)

		post(t, sesURL, body)
		require.Equal(t, domain.ContactListStatusBounced, statusOf(t, email))
		clearBounce(t, email)

		post(t, sesURL, body)

		assert.Equal(t, domain.ContactListStatusActive, statusOf(t, email))
		assert.Equal(t, 1, storedFor(t, messageID))
	})

	t.Run("a Mailjet unsubscribe posted again keeps a contact subscribed since", func(t *testing.T) {
		email := newContact(t)
		messageID := sentForList(t, email)
		body := mailjetUnsub(messageID, email)

		post(t, mailjetURL, body)
		require.Equal(t, domain.ContactListStatusUnsubscribed, statusOf(t, email))
		require.NoError(t, contactListRepo.AddContactToList(bg, workspace.ID, &domain.ContactList{
			Email: email, ListID: list.ID, Status: domain.ContactListStatusActive,
		}))

		post(t, mailjetURL, body)

		assert.Equal(t, domain.ContactListStatusActive, statusOf(t, email))
		assert.Equal(t, 1, storedFor(t, messageID))
	})

	// A delivery whose events were stored before a later step failed is answered 500, and
	// only the provider's redelivery applies its events
	t.Run("a hard bounce posted again repairs a delivery that stored it without applying it", func(t *testing.T) {
		email := newContact(t)
		messageID := sentEmailID(t, factory, workspace.ID, template.ID, email)
		body := sesBouncePayloadAt(messageID, email, "Permanent", "General", "smtp; 550 5.1.1 user unknown", at)

		post(t, sesURL, body)
		unapplied(t, email, messageID)
		require.Equal(t, domain.ContactListStatusActive, statusOf(t, email))

		post(t, sesURL, body)

		assert.Equal(t, domain.ContactListStatusBounced, statusOf(t, email))
		assert.Equal(t, 1, storedFor(t, messageID))
	})

	t.Run("a Mailjet unsubscribe posted again repairs a delivery that stored it without applying it", func(t *testing.T) {
		email := newContact(t)
		messageID := sentForList(t, email)
		body := mailjetUnsub(messageID, email)

		post(t, mailjetURL, body)
		unapplied(t, email, messageID)
		require.Equal(t, domain.ContactListStatusActive, statusOf(t, email))
		require.Nil(t, unsubscribedAt(t, messageID))

		post(t, mailjetURL, body)

		assert.Equal(t, domain.ContactListStatusUnsubscribed, statusOf(t, email))
		assert.NotNil(t, unsubscribedAt(t, messageID), "the email is marked unsubscribed")
		assert.Equal(t, 1, storedFor(t, messageID))
	})

	t.Run("the soft bounce that reached the threshold, posted again, keeps a bounce cleared since", func(t *testing.T) {
		email := newContact(t)
		var last string
		for i := 0; i < domain.DefaultSoftBounceThreshold; i++ {
			messageID := sentEmailID(t, factory, workspace.ID, template.ID, email)
			last = sesSoftBounceWithSNSID(messageID, email, "sns-"+uuid.New().String(), at)
			post(t, sesURL, last)
		}
		require.Equal(t, domain.ContactListStatusBounced, statusOf(t, email))
		clearBounce(t, email)

		post(t, sesURL, last)

		assert.Equal(t, domain.ContactListStatusActive, statusOf(t, email))
	})
}

// sesSoftBounceWithSNSID is an SES mailbox-full bounce of the email, in an SNS
// notification with this MessageId
func sesSoftBounceWithSNSID(messageID, email, snsMessageID string, at time.Time) string {
	var envelope map[string]interface{}
	if err := json.Unmarshal([]byte(sesBouncePayloadAt(messageID, email, "Transient", "MailboxFull", "smtp; 452 4.2.2 mailbox full", at)), &envelope); err != nil {
		panic(err)
	}
	envelope["MessageId"] = snsMessageID
	body, _ := json.Marshal(envelope)
	return string(body)
}

// numericID is a positive integer that the text sets, written as a JSON number, for the
// numeric IDs of Postmark and Mailjet
func numericID(text string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(text))
	return fmt.Sprint(h.Sum64() % (1 << 53))
}

// postmarkBouncePayload is a Postmark bounce webhook of the email, shaped as Postmark's
// documentation shows it, with its numeric ID
func postmarkBouncePayload(messageID, email, bounceType, id string, at time.Time) string {
	return fmt.Sprintf(`{"RecordType":"Bounce","ID":%s,"Type":%q,"TypeCode":1,"Name":"Bounce","MessageID":"pm-%s","ServerID":23,"MessageStream":"outbound","Email":%q,"From":"test@example.com","BouncedAt":%q,"Details":"mailbox full","Inactive":false,"CanActivate":true,"Subject":"Hello","Metadata":{"notifuse_message_id":%q}}`,
		id, bounceType, messageID, email, at.Format(time.RFC3339), messageID)
}
