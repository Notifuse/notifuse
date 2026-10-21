package integration

import (
	"context"
	"encoding/json"
	"fmt"
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

// TestEmailEventMessageMatch posts provider events to /webhooks/email. An event changes
// nothing, and is not stored, unless it names an email of the workspace sent to the address
// the event reports.
func TestEmailEventMessageMatch(t *testing.T) {
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

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))
	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(workspace.ID)

	ses, err := factory.CreateSESIntegration(workspace.ID)
	require.NoError(t, err)
	sesURL := fmt.Sprintf("/webhooks/email?provider=ses&workspace_id=%s&integration_id=%s", workspace.ID, ses.ID)
	sparkPost, err := factory.CreateIntegration(workspace.ID,
		testutil.WithIntegrationName("Test SparkPost Integration"),
		testutil.WithIntegrationEmailProvider(domain.EmailProvider{
			Kind:      domain.EmailProviderKindSparkPost,
			Senders:   []domain.EmailSender{domain.NewEmailSender("test@example.com", "Test Sender")},
			SparkPost: &domain.SparkPostSettings{APIKey: "test-api-key", Endpoint: "https://api.sparkpost.com"},
		}))
	require.NoError(t, err)
	sparkPostURL := fmt.Sprintf("/webhooks/email?provider=sparkpost&workspace_id=%s&integration_id=%s", workspace.ID, sparkPost.ID)

	db, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)
	template, err := factory.CreateTemplate(workspace.ID)
	require.NoError(t, err)
	list, err := factory.CreateList(workspace.ID)
	require.NoError(t, err)
	contactListRepo := suite.ServerManager.GetApp().GetContactListRepository()

	// newContact creates a contact, active on the list
	newContact := func(t *testing.T, prefix string) string {
		t.Helper()
		email := fmt.Sprintf("%s-%s@example.com", prefix, uuid.New().String()[:8])
		_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
		require.NoError(t, err)
		_, err = factory.CreateContactList(workspace.ID,
			testutil.WithContactListEmail(email),
			testutil.WithContactListListID(list.ID),
			testutil.WithContactListStatus(domain.ContactListStatusActive))
		require.NoError(t, err)
		return email
	}
	// sendTo records an email sent to the address, not delivered yet
	sendTo := func(t *testing.T, email string) string {
		t.Helper()
		message, err := factory.CreateMessageHistory(workspace.ID,
			testutil.WithMessageHistoryContactEmail(email),
			testutil.WithMessageTemplate(template.ID))
		require.NoError(t, err)
		return message.ID
	}
	statusOf := func(t *testing.T, email string) domain.ContactListStatus {
		t.Helper()
		row, err := contactListRepo.GetContactListByIDs(bg, workspace.ID, email, list.ID)
		require.NoError(t, err)
		return row.Status
	}
	type marks struct{ delivered, bounced, complained bool }
	marksOf := func(t *testing.T, messageID string) marks {
		t.Helper()
		var m marks
		require.NoError(t, db.QueryRowContext(bg,
			`SELECT delivered_at IS NOT NULL, bounced_at IS NOT NULL, complained_at IS NOT NULL FROM message_history WHERE id = $1`,
			messageID).Scan(&m.delivered, &m.bounced, &m.complained))
		return m
	}
	// storedFor returns the addresses of the events stored for the email
	storedFor := func(t *testing.T, messageID string) []string {
		t.Helper()
		rows, err := db.QueryContext(bg, `SELECT recipient_email FROM inbound_webhook_events WHERE message_id = $1`, messageID)
		require.NoError(t, err)
		defer func() { _ = rows.Close() }()
		var emails []string
		for rows.Next() {
			var email string
			require.NoError(t, rows.Scan(&email))
			emails = append(emails, email)
		}
		require.NoError(t, rows.Err())
		return emails
	}
	timelineEventsOf := func(t *testing.T, email string) int {
		t.Helper()
		var n int
		require.NoError(t, db.QueryRowContext(bg,
			`SELECT COUNT(*) FROM contact_timeline WHERE email = $1 AND entity_type = 'inbound_webhook_event'`, email).Scan(&n))
		return n
	}
	hardBounce := func(messageID, email string) string {
		return sesBouncePayload(messageID, email, "Permanent", "General", "smtp; 550 5.1.1 user unknown")
	}
	softBounce := func(messageID, email string) string {
		return sesBouncePayload(messageID, email, "Transient", "MailboxFull", "smtp; 452 4.2.2 mailbox full")
	}

	t.Run("a hard bounce for an address no email was sent to: nothing is stored or changed", func(t *testing.T) {
		email := newContact(t, "no-email")
		messageID := uuid.New().String()

		postSESBounce(t, client, sesURL, hardBounce(messageID, email))

		assert.Equal(t, domain.ContactListStatusActive, statusOf(t, email))
		assert.Empty(t, storedFor(t, messageID))
		assert.Zero(t, timelineEventsOf(t, email))
	})

	t.Run("a hard bounce naming another contact's email: neither contact is bounced, the email is not marked", func(t *testing.T) {
		recipient := newContact(t, "recipient")
		other := newContact(t, "other")
		messageID := sendTo(t, recipient)

		postSESBounce(t, client, sesURL, hardBounce(messageID, other))

		assert.Equal(t, domain.ContactListStatusActive, statusOf(t, recipient))
		assert.Equal(t, domain.ContactListStatusActive, statusOf(t, other))
		assert.Equal(t, marks{}, marksOf(t, messageID))
		assert.Empty(t, storedFor(t, messageID))
		assert.Zero(t, timelineEventsOf(t, other))
	})

	t.Run("a hard bounce naming an erased contact's email, for the redacted address: nothing is stored or changed", func(t *testing.T) {
		// An erasure keeps the contact's emails with the address redacted
		messageID := sendTo(t, "DELETED_EMAIL")

		postSESBounce(t, client, sesURL, hardBounce(messageID, "DELETED_EMAIL"))

		assert.Equal(t, marks{}, marksOf(t, messageID))
		assert.Empty(t, storedFor(t, messageID))
	})

	t.Run("a hard bounce naming the contact's email, the address in capitals: the contact is bounced, the event stored under the contact's address", func(t *testing.T) {
		email := newContact(t, "recipient")
		messageID := sendTo(t, email)

		postSESBounce(t, client, sesURL, hardBounce(messageID, strings.ToUpper(email)))

		assert.Equal(t, domain.ContactListStatusBounced, statusOf(t, email))
		assert.Equal(t, marks{bounced: true}, marksOf(t, messageID))
		assert.Equal(t, []string{email}, storedFor(t, messageID))
		assert.Equal(t, 1, timelineEventsOf(t, email))
	})

	t.Run("an SES bounce listing an address the email was copied to before the contact: the contact is bounced", func(t *testing.T) {
		email := newContact(t, "copied")
		copied := newContact(t, "copy")
		messageID := sendTo(t, email)

		postSESBounce(t, client, sesURL, sesBounceListing(messageID, copied, email))

		assert.Equal(t, domain.ContactListStatusBounced, statusOf(t, email))
		assert.Equal(t, domain.ContactListStatusActive, statusOf(t, copied))
		assert.Equal(t, marks{bounced: true}, marksOf(t, messageID))
		assert.Equal(t, []string{email}, storedFor(t, messageID))
	})

	t.Run("an SES bounce giving the contact's international domain in Punycode, as SES is sent it: the contact is bounced", func(t *testing.T) {
		local := "idn-" + uuid.New().String()[:8]
		email := local + "@münchen.de"
		_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
		require.NoError(t, err)
		_, err = factory.CreateContactList(workspace.ID,
			testutil.WithContactListEmail(email),
			testutil.WithContactListListID(list.ID),
			testutil.WithContactListStatus(domain.ContactListStatusActive))
		require.NoError(t, err)
		messageID := sendTo(t, email)

		postSESBounce(t, client, sesURL, hardBounce(messageID, local+"@xn--mnchen-3ya.de"))

		assert.Equal(t, domain.ContactListStatusBounced, statusOf(t, email))
		assert.Equal(t, marks{bounced: true}, marksOf(t, messageID))
		assert.Equal(t, []string{email}, storedFor(t, messageID))
	})

	t.Run("soft bounces for an address no email was sent to never reach the threshold", func(t *testing.T) {
		email := newContact(t, "soft-no-email")

		for i := 0; i < domain.DefaultSoftBounceThreshold+1; i++ {
			postSESBounce(t, client, sesURL, softBounce(uuid.New().String(), email))
		}

		assert.Equal(t, domain.ContactListStatusActive, statusOf(t, email))
		assert.Zero(t, timelineEventsOf(t, email))
	})

	t.Run("soft bounces stored without an email sent to the address are not counted", func(t *testing.T) {
		email := newContact(t, "soft-stored")
		// As stored before events were checked: one names no email, and the others, which
		// would reach the threshold with the one counted bounce, another contact's email
		otherMessageID := sendTo(t, newContact(t, "soft-stored-other"))
		future := time.Now().UTC().Add(24 * time.Hour)
		for i := 0; i < domain.DefaultSoftBounceThreshold; i++ {
			messageID := otherMessageID
			if i == 0 {
				messageID = uuid.New().String()
			}
			_, err := db.ExecContext(bg, `INSERT INTO inbound_webhook_events
				(id, type, source, integration_id, recipient_email, message_id, timestamp, raw_payload, bounce_type, bounce_category, bounce_diagnostic, complaint_feedback_type, created_at)
				VALUES ($1, 'bounce', 'ses', $2, $3, $4, $5, '{}', 'Transient', 'MailboxFull', 'smtp; 452 mailbox full', '', NOW())`,
				uuid.New().String(), ses.ID, email, messageID, future)
			require.NoError(t, err)
		}

		// The one counted bounce: under the threshold with the others left out, at it if either
		// those naming no email or those naming another contact's were counted
		postSESBounce(t, client, sesURL, softBounce(sendTo(t, email), email))

		assert.Equal(t, domain.ContactListStatusActive, statusOf(t, email))
	})

	t.Run("soft bounces naming emails sent to the address reach the threshold", func(t *testing.T) {
		email := newContact(t, "soft-sent")

		for i := 0; i < domain.DefaultSoftBounceThreshold; i++ {
			postSESBounce(t, client, sesURL, softBounce(sendTo(t, email), email))
		}

		assert.Equal(t, domain.ContactListStatusBounced, statusOf(t, email))
	})

	t.Run("a complaint naming the email of another address: the email is not marked, the contact not complained", func(t *testing.T) {
		recipient := newContact(t, "complaint-recipient")
		messageID := sendTo(t, recipient)

		postSESNotification(t, client, sesURL, sesComplaintPayload(messageID, newContact(t, "complaint-other")))

		assert.Equal(t, marks{}, marksOf(t, messageID))
		assert.Equal(t, domain.ContactListStatusActive, statusOf(t, recipient))
		assert.Empty(t, storedFor(t, messageID))
	})

	t.Run("a delivery naming the email of another address: the email is not marked delivered", func(t *testing.T) {
		recipient := newContact(t, "delivery-recipient")
		messageID := sendTo(t, recipient)

		postSESNotification(t, client, sesURL, sesDeliveryPayload(messageID, "someone-else@example.com"))

		assert.Equal(t, marks{}, marksOf(t, messageID))
		assert.Empty(t, storedFor(t, messageID))
	})

	t.Run("a delivery naming the contact's email: the email is marked delivered", func(t *testing.T) {
		recipient := newContact(t, "delivered")
		messageID := sendTo(t, recipient)

		postSESNotification(t, client, sesURL, sesDeliveryPayload(messageID, recipient))

		assert.Equal(t, marks{delivered: true}, marksOf(t, messageID))
		assert.Equal(t, []string{recipient}, storedFor(t, messageID))
	})

	t.Run("a SparkPost batch: only the bounce of an email sent to its address is stored and applied", func(t *testing.T) {
		recipient := newContact(t, "sparkpost-recipient")
		// SparkPost reports the address in lowercase. Contacts created before addresses
		// were lowercased can hold capitals.
		mixedCase := "Mixed-" + recipient
		_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(mixedCase))
		require.NoError(t, err)
		var stored string
		require.NoError(t, db.QueryRowContext(bg, `SELECT email FROM contacts WHERE lower(email) = lower($1)`, mixedCase).Scan(&stored))
		require.Equal(t, mixedCase, stored, "the contact keeps its capitals")
		_, err = factory.CreateContactList(workspace.ID,
			testutil.WithContactListEmail(mixedCase),
			testutil.WithContactListListID(list.ID),
			testutil.WithContactListStatus(domain.ContactListStatusActive))
		require.NoError(t, err)
		victim := newContact(t, "sparkpost-victim")
		messageID := sendTo(t, recipient)
		mixedCaseMessageID := sendTo(t, mixedCase)
		unknownID := uuid.New().String()

		postSESNotification(t, client, sparkPostURL, sparkPostBatch(
			sparkPostBounce(messageID, recipient),
			sparkPostBounce(mixedCaseMessageID, strings.ToLower(mixedCase)),
			sparkPostBounce(unknownID, victim),
		))

		assert.Equal(t, domain.ContactListStatusBounced, statusOf(t, recipient))
		assert.Equal(t, domain.ContactListStatusBounced, statusOf(t, mixedCase),
			"the bounce reaches the contact whose address has capitals")
		assert.Equal(t, domain.ContactListStatusActive, statusOf(t, victim))
		assert.Equal(t, []string{recipient}, storedFor(t, messageID))
		assert.Equal(t, []string{mixedCase}, storedFor(t, mixedCaseMessageID))
		assert.Empty(t, storedFor(t, unknownID))
		assert.Zero(t, timelineEventsOf(t, victim))
	})
}

// sesDeliveryPayload builds an SNS-wrapped SES delivery notification naming the Notifuse
// message in its tag.
func sesDeliveryPayload(notifuseMessageID, recipientEmail string) string {
	now := time.Now().UTC().Format(time.RFC3339)
	message := map[string]interface{}{
		"eventType": "Delivery",
		"delivery": map[string]interface{}{
			"timestamp":  now,
			"recipients": []string{recipientEmail},
		},
		"mail": map[string]interface{}{
			"timestamp": now,
			"messageId": "ses-" + uuid.New().String(),
			"tags":      map[string][]string{"notifuse_message_id": {notifuseMessageID}},
		},
	}
	messageBytes, _ := json.Marshal(message)
	envelope := map[string]interface{}{
		"Type":      "Notification",
		"MessageId": "sns-" + uuid.New().String(),
		"TopicArn":  "arn:aws:sns:us-east-1:123456789012:test-topic",
		"Message":   string(messageBytes),
	}
	envelopeBytes, _ := json.Marshal(envelope)
	return string(envelopeBytes)
}

// sesBounceListing builds an SNS-wrapped SES hard bounce of the Notifuse message listing
// these recipients, as SES does for an email sent to several addresses.
func sesBounceListing(notifuseMessageID string, recipientEmails ...string) string {
	recipients := make([]map[string]interface{}, 0, len(recipientEmails))
	for _, email := range recipientEmails {
		recipients = append(recipients, map[string]interface{}{"emailAddress": email, "diagnosticCode": "smtp; 550 5.1.1 user unknown"})
	}
	message := map[string]interface{}{
		"eventType": "Bounce",
		"bounce": map[string]interface{}{
			"bounceType":        "Permanent",
			"bounceSubType":     "General",
			"bouncedRecipients": recipients,
			"timestamp":         time.Now().UTC().Format(time.RFC3339),
		},
		"mail": map[string]interface{}{
			"messageId": "ses-" + uuid.New().String(),
			"tags":      map[string][]string{"notifuse_message_id": {notifuseMessageID}},
		},
	}
	messageBytes, _ := json.Marshal(message)
	envelope := map[string]interface{}{
		"Type":      "Notification",
		"MessageId": "sns-" + uuid.New().String(),
		"TopicArn":  "arn:aws:sns:us-east-1:123456789012:test-topic",
		"Message":   string(messageBytes),
	}
	envelopeBytes, _ := json.Marshal(envelope)
	return string(envelopeBytes)
}

// sparkPostBounce builds a SparkPost hard bounce (class 10) event, the Notifuse id in
// rcpt_meta where SparkPost copies the transmission's metadata.
func sparkPostBounce(notifuseMessageID, recipientEmail string) map[string]interface{} {
	return map[string]interface{}{
		"msys": map[string]interface{}{
			"message_event": map[string]interface{}{
				"type":         "bounce",
				"message_id":   "sp-" + uuid.New().String(),
				"timestamp":    fmt.Sprintf("%d", time.Now().Unix()),
				"rcpt_to":      recipientEmail,
				"rcpt_meta":    map[string]interface{}{"notifuse_message_id": notifuseMessageID},
				"bounce_class": "10",
				"reason":       "550 5.1.1 user unknown",
			},
		},
	}
}

func sparkPostBatch(events ...map[string]interface{}) string {
	body, _ := json.Marshal(events)
	return string(body)
}
