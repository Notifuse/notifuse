package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSESBounceHandling exercises the consecutive-soft-bounce threshold and
// the 4.4.7 retry-exhaustion path introduced for issue #323. Each subtest
// creates an isolated contact + list so they can run independently.
func TestSESBounceHandling(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer func() { suite.Cleanup() }()

	client := suite.APIClient
	factory := suite.DataFactory

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))
	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(workspace.ID)

	integration, err := factory.CreateSESIntegration(workspace.ID)
	require.NoError(t, err)
	webhookURL := fmt.Sprintf("/webhooks/email?provider=ses&workspace_id=%s&integration_id=%s", workspace.ID, integration.ID)

	contactListRepo := suite.ServerManager.GetApp().GetContactListRepository()

	t.Run("SoftBounceThresholdEscalation", func(t *testing.T) {
		testSESSoftBounceThresholdEscalation(t, client, factory, contactListRepo, workspace.ID, webhookURL)
	})

	t.Run("MessageExpired_447_ImmediateHard", func(t *testing.T) {
		testSES447MessageExpiredImmediateHard(t, client, factory, contactListRepo, workspace.ID, webhookURL)
	})

	t.Run("SoftBouncesAfterDeliveryReset", func(t *testing.T) {
		testSESSoftBouncesAfterDelivery(t, client, factory, contactListRepo, workspace.ID, webhookURL)
	})

	t.Run("MessageTooLargeIgnored", func(t *testing.T) {
		testSESMessageTooLargeIgnored(t, client, factory, contactListRepo, workspace.ID, webhookURL)
	})
}

// testSESSoftBounceThresholdEscalation verifies that 5 consecutive SES
// Transient/MailboxFull bounces flip contact_lists.status to 'bounced', and
// that the first 4 leave the contact active.
func testSESSoftBounceThresholdEscalation(t *testing.T, client *testutil.APIClient, factory *testutil.TestDataFactory, contactListRepo domain.ContactListRepository, workspaceID, webhookURL string) {
	list, err := factory.CreateList(workspaceID)
	require.NoError(t, err)

	email := fmt.Sprintf("threshold-%d@example.com", time.Now().UnixNano())
	contact, err := factory.CreateContact(workspaceID, testutil.WithContactEmail(email))
	require.NoError(t, err)

	_, err = factory.CreateContactList(workspaceID,
		testutil.WithContactListEmail(contact.Email),
		testutil.WithContactListListID(list.ID),
		testutil.WithContactListStatus(domain.ContactListStatusActive))
	require.NoError(t, err)

	// Bounces 1..4 — each must leave the contact list active.
	for i := 1; i <= domain.DefaultSoftBounceThreshold-1; i++ {
		postSESBounce(t, client, webhookURL, sesBouncePayload(fmt.Sprintf("msg-%d-%d", time.Now().UnixNano(), i), contact.Email, "Transient", "MailboxFull", "smtp; 452 4.2.2 mailbox full"))
		got, err := contactListRepo.GetContactListByIDs(context.Background(), workspaceID, contact.Email, list.ID)
		require.NoError(t, err)
		assert.Equalf(t, domain.ContactListStatusActive, got.Status,
			"contact_lists.status should still be 'active' after bounce #%d (threshold=%d)", i, domain.DefaultSoftBounceThreshold)
	}

	// Bounce #5 (== threshold) must escalate.
	postSESBounce(t, client, webhookURL, sesBouncePayload(fmt.Sprintf("msg-%d-final", time.Now().UnixNano()), contact.Email, "Transient", "MailboxFull", "smtp; 452 4.2.2 mailbox full"))
	got, err := contactListRepo.GetContactListByIDs(context.Background(), workspaceID, contact.Email, list.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.ContactListStatusBounced, got.Status, "the %dth consecutive soft bounce must escalate to 'bounced'", domain.DefaultSoftBounceThreshold)
}

// testSES447MessageExpiredImmediateHard verifies the diagnostic-based
// promotion path: a single Transient/General with the `4.4.7 Message expired`
// diagnostic suppresses the recipient on the first event, with no waiting for
// repeated bounces.
func testSES447MessageExpiredImmediateHard(t *testing.T, client *testutil.APIClient, factory *testutil.TestDataFactory, contactListRepo domain.ContactListRepository, workspaceID, webhookURL string) {
	list, err := factory.CreateList(workspaceID)
	require.NoError(t, err)

	email := fmt.Sprintf("447-%d@example.com", time.Now().UnixNano())
	contact, err := factory.CreateContact(workspaceID, testutil.WithContactEmail(email))
	require.NoError(t, err)

	_, err = factory.CreateContactList(workspaceID,
		testutil.WithContactListEmail(contact.Email),
		testutil.WithContactListListID(list.ID),
		testutil.WithContactListStatus(domain.ContactListStatusActive))
	require.NoError(t, err)

	// The exact diagnostic from the original issue.
	diagnostic := "smtp; 550 4.4.7 Message expired: unable to deliver in 840 minutes. <421 4.4.1 Failed to establish connection>"
	postSESBounce(t, client, webhookURL, sesBouncePayload(fmt.Sprintf("msg-%d", time.Now().UnixNano()), contact.Email, "Transient", "General", diagnostic))

	got, err := contactListRepo.GetContactListByIDs(context.Background(), workspaceID, contact.Email, list.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.ContactListStatusBounced, got.Status, "4.4.7 Message expired must escalate immediately, not wait for the threshold")
}

// testSESSoftBouncesAfterDelivery verifies the count window reset: 4 soft
// bounces + a successful delivery + 4 more soft bounces should NOT escalate,
// because the delivery resets the counter.
func testSESSoftBouncesAfterDelivery(t *testing.T, client *testutil.APIClient, factory *testutil.TestDataFactory, contactListRepo domain.ContactListRepository, workspaceID, webhookURL string) {
	list, err := factory.CreateList(workspaceID)
	require.NoError(t, err)

	email := fmt.Sprintf("reset-%d@example.com", time.Now().UnixNano())
	contact, err := factory.CreateContact(workspaceID, testutil.WithContactEmail(email))
	require.NoError(t, err)

	_, err = factory.CreateContactList(workspaceID,
		testutil.WithContactListEmail(contact.Email),
		testutil.WithContactListListID(list.ID),
		testutil.WithContactListStatus(domain.ContactListStatusActive))
	require.NoError(t, err)

	// First batch: 4 soft bounces (below threshold).
	for i := 1; i <= 4; i++ {
		postSESBounce(t, client, webhookURL, sesBouncePayload(fmt.Sprintf("pre-%d-%d", time.Now().UnixNano(), i), contact.Email, "Transient", "MailboxFull", "smtp; 452 mailbox full"))
	}

	// Successful delivery → resets the count via MAX(delivered_at).
	template, err := factory.CreateTemplate(workspaceID)
	require.NoError(t, err)
	_, err = factory.CreateMessageHistory(workspaceID,
		testutil.WithMessageHistoryContactEmail(contact.Email),
		testutil.WithMessageTemplate(template.ID),
		testutil.WithMessageDelivered(true))
	require.NoError(t, err)

	// Second batch: 4 more soft bounces. Counter window starts after the
	// delivery, so it's at 4 — still under threshold.
	for i := 1; i <= 4; i++ {
		postSESBounce(t, client, webhookURL, sesBouncePayload(fmt.Sprintf("post-%d-%d", time.Now().UnixNano(), i), contact.Email, "Transient", "MailboxFull", "smtp; 452 mailbox full"))
	}

	got, err := contactListRepo.GetContactListByIDs(context.Background(), workspaceID, contact.Email, list.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.ContactListStatusActive, got.Status, "delivery between two batches of soft bounces must reset the consecutive-bounce counter")
}

// testSESMessageTooLargeIgnored verifies that MessageTooLarge bounces never
// count toward the threshold no matter how many arrive — they're a problem
// with the message, not the recipient.
func testSESMessageTooLargeIgnored(t *testing.T, client *testutil.APIClient, factory *testutil.TestDataFactory, contactListRepo domain.ContactListRepository, workspaceID, webhookURL string) {
	list, err := factory.CreateList(workspaceID)
	require.NoError(t, err)

	email := fmt.Sprintf("toolarge-%d@example.com", time.Now().UnixNano())
	contact, err := factory.CreateContact(workspaceID, testutil.WithContactEmail(email))
	require.NoError(t, err)

	_, err = factory.CreateContactList(workspaceID,
		testutil.WithContactListEmail(contact.Email),
		testutil.WithContactListListID(list.ID),
		testutil.WithContactListStatus(domain.ContactListStatusActive))
	require.NoError(t, err)

	// 10 MessageTooLarge events — twice the threshold.
	for i := 1; i <= 10; i++ {
		postSESBounce(t, client, webhookURL, sesBouncePayload(fmt.Sprintf("big-%d-%d", time.Now().UnixNano(), i), contact.Email, "Transient", "MessageTooLarge", "552 5.3.4 message too big"))
	}

	got, err := contactListRepo.GetContactListByIDs(context.Background(), workspaceID, contact.Email, list.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.ContactListStatusActive, got.Status, "MessageTooLarge is a message-level rejection and must not count toward the recipient's soft-bounce ledger")
}

// postSESBounce POSTs an SES SNS bounce payload to the webhook endpoint and
// asserts a 200 OK.
func postSESBounce(t *testing.T, client *testutil.APIClient, webhookURL, payload string) {
	t.Helper()
	resp, err := client.PostRaw(webhookURL, payload)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode, "webhook POST must return 200")
}

// sesBouncePayload builds an SNS-wrapped SES bounce notification with the
// given bounceType, subtype, and diagnostic. Mirrors the format produced by
// AWS SNS for SES bounce events.
func sesBouncePayload(messageID, recipientEmail, bounceType, subtype, diagnostic string) string {
	message := map[string]interface{}{
		"eventType": "Bounce",
		"bounce": map[string]interface{}{
			"bounceType":    bounceType,
			"bounceSubType": subtype,
			"bouncedRecipients": []map[string]interface{}{
				{"emailAddress": recipientEmail, "diagnosticCode": diagnostic},
			},
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		},
		"mail": map[string]interface{}{
			"messageId": "ses-" + messageID,
			"tags":      map[string][]string{"notifuse_message_id": {messageID}},
		},
	}
	messageBytes, _ := json.Marshal(message)

	envelope := map[string]interface{}{
		"Type":      "Notification",
		"MessageId": "sns-" + messageID,
		"TopicArn":  "arn:aws:sns:us-east-1:123456789012:test-topic",
		"Message":   string(messageBytes),
	}
	envelopeBytes, _ := json.Marshal(envelope)
	return string(envelopeBytes)
}
