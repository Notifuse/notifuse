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

// TestWebhookSubscriptionStatusE2E tests the end-to-end flow of:
// 1. Webhook receiving bounce/complaint events
// 2. Message history being updated with bounced_at/complained_at
// 3. Database trigger automatically updating contact_lists status
// This test verifies the v10 migration feature works correctly
func TestWebhookSubscriptionStatusE2E(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

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

	// Login to get auth token
	err = client.Login(user.Email, "password")
	require.NoError(t, err)
	client.SetWorkspaceID(workspace.ID)

	t.Run("Hard Bounce Updates Contact List Status", func(t *testing.T) {
		testHardBounceUpdatesContactListStatus(t, suite, client, factory, workspace.ID)
	})

	t.Run("Complaint Updates Contact List Status", func(t *testing.T) {
		testComplaintUpdatesContactListStatus(t, suite, client, factory, workspace.ID)
	})

	t.Run("Soft Bounce Does Not Update Contact List Status", func(t *testing.T) {
		testSoftBounceDoesNotUpdateContactListStatus(t, suite, client, factory, workspace.ID)
	})

	t.Run("Webhook Updates Multiple Lists", func(t *testing.T) {
		testWebhookUpdatesMultipleLists(t, suite, client, factory, workspace.ID)
	})

	t.Run("Complaint Takes Priority Over Bounce", func(t *testing.T) {
		testComplaintTakesPriorityOverBounce(t, suite, client, factory, workspace.ID)
	})

	t.Run("Webhook Without List IDs Does Not Update Status", func(t *testing.T) {
		testWebhookWithoutListIDsDoesNotUpdate(t, suite, client, factory, workspace.ID)
	})
}

func testHardBounceUpdatesContactListStatus(t *testing.T, suite *testutil.IntegrationTestSuite, client *testutil.APIClient, factory *testutil.TestDataFactory, workspaceID string) {
	// Create SES integration for webhook testing
	integration, err := factory.CreateSESIntegration(workspaceID)
	require.NoError(t, err)

	// Create list
	list, err := factory.CreateList(workspaceID)
	require.NoError(t, err)

	// Create contact
	contactEmail := fmt.Sprintf("bounce-test-%d@example.com", time.Now().UnixNano())
	contact, err := factory.CreateContact(workspaceID, testutil.WithContactEmail(contactEmail))
	require.NoError(t, err)

	// Add contact to list with active status
	_, err = factory.CreateContactList(workspaceID,
		testutil.WithContactListEmail(contact.Email),
		testutil.WithContactListListID(list.ID),
		testutil.WithContactListStatus(domain.ContactListStatusActive))
	require.NoError(t, err)

	// Create broadcast
	broadcast, err := factory.CreateBroadcast(workspaceID)
	require.NoError(t, err)

	// Update broadcast audience to include the list
	broadcast.Audience.Lists = []string{list.ID}
	app := suite.ServerManager.GetApp()
	broadcastRepo := app.GetBroadcastRepository()
	err = broadcastRepo.UpdateBroadcast(context.Background(), broadcast)
	require.NoError(t, err)

	// Create message history with list_ids
	messageID := fmt.Sprintf("msg-%d", time.Now().UnixNano())
	template, err := factory.CreateTemplate(workspaceID)
	require.NoError(t, err)

	message, err := factory.CreateMessageHistory(workspaceID,
		testutil.WithMessageHistoryContactEmail(contact.Email),
		testutil.WithMessageTemplate(template.ID),
		testutil.WithMessageBroadcast(broadcast.ID),
		func(m *domain.MessageHistory) {
			m.ID = messageID
			m.ListIDs = []string{list.ID}
		})
	require.NoError(t, err)
	require.Len(t, message.ListIDs, 1)
	require.Equal(t, list.ID, message.ListIDs[0])

	// Verify contact list status is initially active
	contactListRepo := app.GetContactListRepository()
	initialContactList, err := contactListRepo.GetContactListByIDs(context.Background(), workspaceID, contact.Email, list.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.ContactListStatusActive, initialContactList.Status)

	// Send webhook with hard bounce
	bouncePayload := createSESHardBouncePayload(messageID, contact.Email)
	webhookURL := fmt.Sprintf("/webhooks/email?provider=ses&workspace_id=%s&integration_id=%s", workspaceID, integration.ID)

	resp, err := client.PostRaw(webhookURL, bouncePayload)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Give the database trigger time to process
	time.Sleep(100 * time.Millisecond)

	// Verify message history was updated with bounced_at
	messageHistoryRepo := app.GetMessageHistoryRepository()
	workspaceRepo := app.GetWorkspaceRepository()
	workspace, err := workspaceRepo.GetByID(context.Background(), workspaceID)
	require.NoError(t, err)
	updatedMessage, err := messageHistoryRepo.Get(context.Background(), workspaceID, workspace.Settings.SecretKey, messageID)
	require.NoError(t, err)
	assert.NotNil(t, updatedMessage.BouncedAt, "Message should have bounced_at set")

	// Verify contact list status was updated to bounced by the trigger
	updatedContactList, err := contactListRepo.GetContactListByIDs(context.Background(), workspaceID, contact.Email, list.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.ContactListStatusBounced, updatedContactList.Status, "Contact list status should be updated to bounced")
}

func testComplaintUpdatesContactListStatus(t *testing.T, suite *testutil.IntegrationTestSuite, client *testutil.APIClient, factory *testutil.TestDataFactory, workspaceID string) {
	// Create SES integration for webhook testing
	integration, err := factory.CreateSESIntegration(workspaceID)
	require.NoError(t, err)

	// Create list
	list, err := factory.CreateList(workspaceID)
	require.NoError(t, err)

	// Create contact
	contactEmail := fmt.Sprintf("complaint-test-%d@example.com", time.Now().UnixNano())
	contact, err := factory.CreateContact(workspaceID, testutil.WithContactEmail(contactEmail))
	require.NoError(t, err)

	// Add contact to list with active status
	_, err = factory.CreateContactList(workspaceID,
		testutil.WithContactListEmail(contact.Email),
		testutil.WithContactListListID(list.ID),
		testutil.WithContactListStatus(domain.ContactListStatusActive))
	require.NoError(t, err)

	// Create broadcast
	broadcast, err := factory.CreateBroadcast(workspaceID)
	require.NoError(t, err)

	// Update broadcast audience to include the list
	broadcast.Audience.Lists = []string{list.ID}
	app := suite.ServerManager.GetApp()
	broadcastRepo := app.GetBroadcastRepository()
	err = broadcastRepo.UpdateBroadcast(context.Background(), broadcast)
	require.NoError(t, err)

	// Create message history with list_ids
	messageID := fmt.Sprintf("msg-%d", time.Now().UnixNano())
	template, err := factory.CreateTemplate(workspaceID)
	require.NoError(t, err)

	_, err = factory.CreateMessageHistory(workspaceID,
		testutil.WithMessageHistoryContactEmail(contact.Email),
		testutil.WithMessageTemplate(template.ID),
		testutil.WithMessageBroadcast(broadcast.ID),
		func(m *domain.MessageHistory) {
			m.ID = messageID
			m.ListIDs = []string{list.ID}
		})
	require.NoError(t, err)

	// Send webhook with complaint
	complaintPayload := createSESComplaintPayload(messageID, contact.Email)
	webhookURL := fmt.Sprintf("/webhooks/email?provider=ses&workspace_id=%s&integration_id=%s", workspaceID, integration.ID)

	resp, err := client.PostRaw(webhookURL, complaintPayload)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Give the database trigger time to process
	time.Sleep(100 * time.Millisecond)

	// Verify message history was updated with complained_at
	messageHistoryRepo := app.GetMessageHistoryRepository()
	workspaceRepo := app.GetWorkspaceRepository()
	workspace, err := workspaceRepo.GetByID(context.Background(), workspaceID)
	require.NoError(t, err)
	updatedMessage, err := messageHistoryRepo.Get(context.Background(), workspaceID, workspace.Settings.SecretKey, messageID)
	require.NoError(t, err)
	assert.NotNil(t, updatedMessage.ComplainedAt, "Message should have complained_at set")

	// Verify contact list status was updated to complained by the trigger
	contactListRepo := app.GetContactListRepository()
	updatedContactList, err := contactListRepo.GetContactListByIDs(context.Background(), workspaceID, contact.Email, list.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.ContactListStatusComplained, updatedContactList.Status, "Contact list status should be updated to complained")
}

func testSoftBounceDoesNotUpdateContactListStatus(t *testing.T, suite *testutil.IntegrationTestSuite, client *testutil.APIClient, factory *testutil.TestDataFactory, workspaceID string) {
	// Create SES integration for webhook testing
	integration, err := factory.CreateSESIntegration(workspaceID)
	require.NoError(t, err)

	// Create list
	list, err := factory.CreateList(workspaceID)
	require.NoError(t, err)

	// Create contact
	contactEmail := fmt.Sprintf("soft-bounce-test-%d@example.com", time.Now().UnixNano())
	contact, err := factory.CreateContact(workspaceID, testutil.WithContactEmail(contactEmail))
	require.NoError(t, err)

	// Add contact to list with active status
	_, err = factory.CreateContactList(workspaceID,
		testutil.WithContactListEmail(contact.Email),
		testutil.WithContactListListID(list.ID),
		testutil.WithContactListStatus(domain.ContactListStatusActive))
	require.NoError(t, err)

	// Create broadcast
	broadcast, err := factory.CreateBroadcast(workspaceID)
	require.NoError(t, err)

	// Update broadcast audience to include the list
	broadcast.Audience.Lists = []string{list.ID}
	app := suite.ServerManager.GetApp()
	broadcastRepo := app.GetBroadcastRepository()
	err = broadcastRepo.UpdateBroadcast(context.Background(), broadcast)
	require.NoError(t, err)

	// Create message history with list_ids
	messageID := fmt.Sprintf("msg-%d", time.Now().UnixNano())
	template, err := factory.CreateTemplate(workspaceID)
	require.NoError(t, err)

	_, err = factory.CreateMessageHistory(workspaceID,
		testutil.WithMessageHistoryContactEmail(contact.Email),
		testutil.WithMessageTemplate(template.ID),
		testutil.WithMessageBroadcast(broadcast.ID),
		func(m *domain.MessageHistory) {
			m.ID = messageID
			m.ListIDs = []string{list.ID}
		})
	require.NoError(t, err)

	// Send webhook with soft bounce
	softBouncePayload := createSESSoftBouncePayload(messageID, contact.Email)
	webhookURL := fmt.Sprintf("/webhooks/email?provider=ses&workspace_id=%s&integration_id=%s", workspaceID, integration.ID)

	resp, err := client.PostRaw(webhookURL, softBouncePayload)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Give time for any potential processing
	time.Sleep(100 * time.Millisecond)

	// Verify message history was NOT updated with bounced_at (soft bounces don't set it)
	messageHistoryRepo := app.GetMessageHistoryRepository()
	workspaceRepo := app.GetWorkspaceRepository()
	workspace, err := workspaceRepo.GetByID(context.Background(), workspaceID)
	require.NoError(t, err)
	updatedMessage, err := messageHistoryRepo.Get(context.Background(), workspaceID, workspace.Settings.SecretKey, messageID)
	require.NoError(t, err)
	assert.Nil(t, updatedMessage.BouncedAt, "Soft bounce should not set bounced_at")

	// Verify contact list status remains active
	contactListRepo := app.GetContactListRepository()
	updatedContactList, err := contactListRepo.GetContactListByIDs(context.Background(), workspaceID, contact.Email, list.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.ContactListStatusActive, updatedContactList.Status, "Contact list status should remain active for soft bounces")
}

func testWebhookUpdatesMultipleLists(t *testing.T, suite *testutil.IntegrationTestSuite, client *testutil.APIClient, factory *testutil.TestDataFactory, workspaceID string) {
	// Create SES integration for webhook testing
	integration, err := factory.CreateSESIntegration(workspaceID)
	require.NoError(t, err)

	// Create multiple lists
	list1, err := factory.CreateList(workspaceID, testutil.WithListName("List 1"))
	require.NoError(t, err)
	list2, err := factory.CreateList(workspaceID, testutil.WithListName("List 2"))
	require.NoError(t, err)
	list3, err := factory.CreateList(workspaceID, testutil.WithListName("List 3"))
	require.NoError(t, err)

	// Create contact
	contactEmail := fmt.Sprintf("multi-list-test-%d@example.com", time.Now().UnixNano())
	contact, err := factory.CreateContact(workspaceID, testutil.WithContactEmail(contactEmail))
	require.NoError(t, err)

	// Add contact to multiple lists
	_, err = factory.CreateContactList(workspaceID,
		testutil.WithContactListEmail(contact.Email),
		testutil.WithContactListListID(list1.ID),
		testutil.WithContactListStatus(domain.ContactListStatusActive))
	require.NoError(t, err)

	_, err = factory.CreateContactList(workspaceID,
		testutil.WithContactListEmail(contact.Email),
		testutil.WithContactListListID(list2.ID),
		testutil.WithContactListStatus(domain.ContactListStatusActive))
	require.NoError(t, err)

	// Add to list3 but don't include in the message (to verify only relevant lists are updated)
	_, err = factory.CreateContactList(workspaceID,
		testutil.WithContactListEmail(contact.Email),
		testutil.WithContactListListID(list3.ID),
		testutil.WithContactListStatus(domain.ContactListStatusActive))
	require.NoError(t, err)

	// Create broadcast
	broadcast, err := factory.CreateBroadcast(workspaceID)
	require.NoError(t, err)

	// Update broadcast audience to include list1 and list2 only
	broadcast.Audience.Lists = []string{list1.ID, list2.ID}
	app := suite.ServerManager.GetApp()
	broadcastRepo := app.GetBroadcastRepository()
	err = broadcastRepo.UpdateBroadcast(context.Background(), broadcast)
	require.NoError(t, err)

	// Create message history with list_ids for list1 and list2
	messageID := fmt.Sprintf("msg-%d", time.Now().UnixNano())
	template, err := factory.CreateTemplate(workspaceID)
	require.NoError(t, err)

	_, err = factory.CreateMessageHistory(workspaceID,
		testutil.WithMessageHistoryContactEmail(contact.Email),
		testutil.WithMessageTemplate(template.ID),
		testutil.WithMessageBroadcast(broadcast.ID),
		func(m *domain.MessageHistory) {
			m.ID = messageID
			m.ListIDs = []string{list1.ID, list2.ID}
		})
	require.NoError(t, err)

	// Send webhook with hard bounce
	bouncePayload := createSESHardBouncePayload(messageID, contact.Email)
	webhookURL := fmt.Sprintf("/webhooks/email?provider=ses&workspace_id=%s&integration_id=%s", workspaceID, integration.ID)

	resp, err := client.PostRaw(webhookURL, bouncePayload)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Give the database trigger time to process
	time.Sleep(100 * time.Millisecond)

	// Verify both list1 and list2 statuses were updated
	contactListRepo := app.GetContactListRepository()

	list1ContactList, err := contactListRepo.GetContactListByIDs(context.Background(), workspaceID, contact.Email, list1.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.ContactListStatusBounced, list1ContactList.Status, "List1 status should be bounced")

	list2ContactList, err := contactListRepo.GetContactListByIDs(context.Background(), workspaceID, contact.Email, list2.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.ContactListStatusBounced, list2ContactList.Status, "List2 status should be bounced")

	// Verify list3 status remains active (not included in the message)
	list3ContactList, err := contactListRepo.GetContactListByIDs(context.Background(), workspaceID, contact.Email, list3.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.ContactListStatusActive, list3ContactList.Status, "List3 status should remain active")
}

func testComplaintTakesPriorityOverBounce(t *testing.T, suite *testutil.IntegrationTestSuite, client *testutil.APIClient, factory *testutil.TestDataFactory, workspaceID string) {
	// Create SES integration for webhook testing
	integration, err := factory.CreateSESIntegration(workspaceID)
	require.NoError(t, err)

	// Create list
	list, err := factory.CreateList(workspaceID)
	require.NoError(t, err)

	// Create contact
	contactEmail := fmt.Sprintf("priority-test-%d@example.com", time.Now().UnixNano())
	contact, err := factory.CreateContact(workspaceID, testutil.WithContactEmail(contactEmail))
	require.NoError(t, err)

	// Add contact to list with bounced status
	_, err = factory.CreateContactList(workspaceID,
		testutil.WithContactListEmail(contact.Email),
		testutil.WithContactListListID(list.ID),
		testutil.WithContactListStatus(domain.ContactListStatusBounced))
	require.NoError(t, err)

	// Create broadcast
	broadcast, err := factory.CreateBroadcast(workspaceID)
	require.NoError(t, err)

	// Update broadcast audience to include the list
	broadcast.Audience.Lists = []string{list.ID}
	app := suite.ServerManager.GetApp()
	broadcastRepo := app.GetBroadcastRepository()
	err = broadcastRepo.UpdateBroadcast(context.Background(), broadcast)
	require.NoError(t, err)

	// Create message history with list_ids
	messageID := fmt.Sprintf("msg-%d", time.Now().UnixNano())
	template, err := factory.CreateTemplate(workspaceID)
	require.NoError(t, err)

	_, err = factory.CreateMessageHistory(workspaceID,
		testutil.WithMessageHistoryContactEmail(contact.Email),
		testutil.WithMessageTemplate(template.ID),
		testutil.WithMessageBroadcast(broadcast.ID),
		func(m *domain.MessageHistory) {
			m.ID = messageID
			m.ListIDs = []string{list.ID}
		})
	require.NoError(t, err)

	// Send webhook with complaint
	complaintPayload := createSESComplaintPayload(messageID, contact.Email)
	webhookURL := fmt.Sprintf("/webhooks/email?provider=ses&workspace_id=%s&integration_id=%s", workspaceID, integration.ID)

	resp, err := client.PostRaw(webhookURL, complaintPayload)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Give the database trigger time to process
	time.Sleep(100 * time.Millisecond)

	// Verify contact list status was upgraded from bounced to complained
	contactListRepo := app.GetContactListRepository()
	updatedContactList, err := contactListRepo.GetContactListByIDs(context.Background(), workspaceID, contact.Email, list.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.ContactListStatusComplained, updatedContactList.Status, "Contact list status should be upgraded to complained")
}

func testWebhookWithoutListIDsDoesNotUpdate(t *testing.T, suite *testutil.IntegrationTestSuite, client *testutil.APIClient, factory *testutil.TestDataFactory, workspaceID string) {
	// Create SES integration for webhook testing
	integration, err := factory.CreateSESIntegration(workspaceID)
	require.NoError(t, err)

	// Create list
	list, err := factory.CreateList(workspaceID)
	require.NoError(t, err)

	// Create contact
	contactEmail := fmt.Sprintf("no-list-test-%d@example.com", time.Now().UnixNano())
	contact, err := factory.CreateContact(workspaceID, testutil.WithContactEmail(contactEmail))
	require.NoError(t, err)

	// Add contact to list with active status
	_, err = factory.CreateContactList(workspaceID,
		testutil.WithContactListEmail(contact.Email),
		testutil.WithContactListListID(list.ID),
		testutil.WithContactListStatus(domain.ContactListStatusActive))
	require.NoError(t, err)

	// Create message history WITHOUT list_ids (transactional email scenario)
	messageID := fmt.Sprintf("msg-%d", time.Now().UnixNano())
	template, err := factory.CreateTemplate(workspaceID)
	require.NoError(t, err)

	_, err = factory.CreateMessageHistory(workspaceID,
		testutil.WithMessageHistoryContactEmail(contact.Email),
		testutil.WithMessageTemplate(template.ID),
		func(m *domain.MessageHistory) {
			m.ID = messageID
			m.ListIDs = nil // No list IDs
		})
	require.NoError(t, err)

	// Send webhook with hard bounce
	bouncePayload := createSESHardBouncePayload(messageID, contact.Email)
	webhookURL := fmt.Sprintf("/webhooks/email?provider=ses&workspace_id=%s&integration_id=%s", workspaceID, integration.ID)

	resp, err := client.PostRaw(webhookURL, bouncePayload)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Give time for any potential processing
	time.Sleep(100 * time.Millisecond)

	// Verify contact list status remains active (trigger should not update without list_ids)
	app := suite.ServerManager.GetApp()
	contactListRepo := app.GetContactListRepository()
	updatedContactList, err := contactListRepo.GetContactListByIDs(context.Background(), workspaceID, contact.Email, list.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.ContactListStatusActive, updatedContactList.Status, "Contact list status should remain active when message has no list_ids")
}

// Helper functions to create SES webhook payloads

func createSESHardBouncePayload(messageID, recipientEmail string) string {
	payload := map[string]interface{}{
		"Type":      "Notification",
		"MessageId": "test-sns-message-id",
		"TopicArn":  "arn:aws:sns:us-east-1:123456789012:test-topic",
		"Message": map[string]interface{}{
			"eventType": "Bounce",
			"bounce": map[string]interface{}{
				"bounceType":    "Permanent",
				"bounceSubType": "General",
				"bouncedRecipients": []map[string]interface{}{
					{
						"emailAddress":   recipientEmail,
						"status":         "5.1.1",
						"diagnosticCode": "smtp; 550 5.1.1 user unknown",
					},
				},
				"timestamp": time.Now().UTC().Format(time.RFC3339),
			},
			"mail": map[string]interface{}{
				"messageId": "ses-message-id",
				"tags": map[string][]string{
					"notifuse_message_id": {messageID},
				},
			},
		},
	}

	// Convert Message to JSON string as SES does
	messageBytes, _ := json.Marshal(payload["Message"])
	payload["Message"] = string(messageBytes)

	payloadBytes, _ := json.Marshal(payload)
	return string(payloadBytes)
}

func createSESSoftBouncePayload(messageID, recipientEmail string) string {
	payload := map[string]interface{}{
		"Type":      "Notification",
		"MessageId": "test-sns-message-id",
		"TopicArn":  "arn:aws:sns:us-east-1:123456789012:test-topic",
		"Message": map[string]interface{}{
			"eventType": "Bounce",
			"bounce": map[string]interface{}{
				"bounceType":    "Transient",
				"bounceSubType": "MailboxFull",
				"bouncedRecipients": []map[string]interface{}{
					{
						"emailAddress":   recipientEmail,
						"status":         "4.2.2",
						"diagnosticCode": "smtp; 452 4.2.2 mailbox full",
					},
				},
				"timestamp": time.Now().UTC().Format(time.RFC3339),
			},
			"mail": map[string]interface{}{
				"messageId": "ses-message-id",
				"tags": map[string][]string{
					"notifuse_message_id": {messageID},
				},
			},
		},
	}

	// Convert Message to JSON string as SES does
	messageBytes, _ := json.Marshal(payload["Message"])
	payload["Message"] = string(messageBytes)

	payloadBytes, _ := json.Marshal(payload)
	return string(payloadBytes)
}

func createSESComplaintPayload(messageID, recipientEmail string) string {
	payload := map[string]interface{}{
		"Type":      "Notification",
		"MessageId": "test-sns-message-id",
		"TopicArn":  "arn:aws:sns:us-east-1:123456789012:test-topic",
		"Message": map[string]interface{}{
			"eventType": "Complaint",
			"complaint": map[string]interface{}{
				"complainedRecipients": []map[string]interface{}{
					{
						"emailAddress": recipientEmail,
					},
				},
				"timestamp":             time.Now().UTC().Format(time.RFC3339),
				"feedbackId":            "test-feedback-id",
				"complaintFeedbackType": "abuse",
			},
			"mail": map[string]interface{}{
				"messageId": "ses-message-id",
				"tags": map[string][]string{
					"notifuse_message_id": {messageID},
				},
			},
		},
	}

	// Convert Message to JSON string as SES does
	messageBytes, _ := json.Marshal(payload["Message"])
	payload["Message"] = string(messageBytes)

	payloadBytes, _ := json.Marshal(payload)
	return string(payloadBytes)
}
