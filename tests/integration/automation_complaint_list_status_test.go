package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
)

// TestAutomationComplaintRecordedOnList sends an automation email to a member of the
// automation's list, then reports it as spam through an SES complaint notification that
// identifies the message the way SES does: by the X-Message-ID header of the email
// Mailpit received.
//
// Automation emails used to store no list on their message_history row, and the
// trigger that turns a complaint into a list status acts only when list_id is set. The
// contact stayed active on the list and the automation kept emailing them.
func TestAutomationComplaintRecordedOnList(t *testing.T) {
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
	require.NoError(t, factory.SetWorkspaceCustomEndpointURL(workspace.ID, suite.ServerManager.GetURL()))

	// Emails go out through SMTP to Mailpit; the SES integration only receives the
	// complaint notification, which names the message, not the integration that sent it.
	ses, err := factory.CreateSESIntegration(workspace.ID)
	require.NoError(t, err)
	webhookURL := fmt.Sprintf("/webhooks/email?provider=ses&workspace_id=%s&integration_id=%s", workspace.ID, ses.ID)

	workspaceDB, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)
	messageHistoryRepo := suite.ServerManager.GetApp().GetMessageHistoryRepository()

	list, err := factory.CreateList(workspace.ID)
	require.NoError(t, err)
	template, err := factory.CreateTemplate(workspace.ID)
	require.NoError(t, err)
	require.Equal(t, "marketing", template.Category, "only marketing and blog emails check the list status")

	automationID := shortuuid.New()
	eventName := "complaint_list_status_" + uuid.New().String()[:8]
	createActiveAutomation(t, client, workspace.ID, automationID, list.ID, eventName, template.ID)

	// deliver fires the automation's event for a new member of the list and returns the
	// address and the Notifuse message ID the delivered email carries.
	deliver := func(t *testing.T) (email, messageID string) {
		t.Helper()
		email = fmt.Sprintf("complaint-%s@example.com", uuid.New().String()[:8])
		_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
		require.NoError(t, err)
		_, err = factory.CreateContactList(workspace.ID,
			testutil.WithContactListEmail(email),
			testutil.WithContactListListID(list.ID),
			testutil.WithContactListStatus(domain.ContactListStatusActive))
		require.NoError(t, err)

		require.NoError(t, factory.CreateCustomEvent(workspace.ID, email, eventName, map[string]interface{}{}))
		msg, err := waitForEmailAndGetMessage(t, email, 20*time.Second)
		require.NoError(t, err, "the automation email should reach Mailpit")
		messageID = deliveredHeader(t, msg, "X-Message-ID")

		// An SMTP send records its message_history row only once the server accepted it
		testutil.WaitForCondition(t, func() bool {
			var n int
			_ = workspaceDB.QueryRowContext(context.Background(),
				`SELECT COUNT(*) FROM message_history WHERE id = $1`, messageID).Scan(&n)
			return n == 1
		}, 10*time.Second, "waiting for the message_history row of "+messageID)
		return email, messageID
	}

	statusOf := func(t *testing.T, email string) domain.ContactListStatus {
		t.Helper()
		var status string
		require.NoError(t, workspaceDB.QueryRowContext(context.Background(),
			`SELECT status FROM contact_lists WHERE email = $1 AND list_id = $2`, email, list.ID).Scan(&status))
		return domain.ContactListStatus(status)
	}

	wasSentFor := func(t *testing.T, messageID, email, listID string) bool {
		t.Helper()
		sent, err := messageHistoryRepo.WasSentForList(context.Background(), workspace.ID, messageID, email, listID)
		require.NoError(t, err)
		return sent
	}

	t.Run("complaint marks the contact complained and the next email is skipped", func(t *testing.T) {
		email, messageID := deliver(t)

		var storedList *string
		require.NoError(t, workspaceDB.QueryRowContext(context.Background(),
			`SELECT list_id FROM message_history WHERE id = $1`, messageID).Scan(&storedList))
		require.NotNil(t, storedList, "the automation email records the list it was sent for")
		assert.Equal(t, list.ID, *storedList)

		postSESNotification(t, client, webhookURL, sesComplaintPayload(messageID, email))

		assert.Equal(t, domain.ContactListStatusComplained, statusOf(t, email))

		require.NoError(t, factory.CreateCustomEvent(workspace.ID, email, eventName, map[string]interface{}{}))
		// The first enrolment completed, so only the new one can report exited
		ca := waitForAutomationStatus(t, factory, workspace.ID, automationID, email, domain.ContactAutomationStatusExited, 15*time.Second)
		require.NotNil(t, ca.ExitReason)
		assert.Equal(t, "complained", *ca.ExitReason, "the next email is skipped, not sent")
		assert.Equal(t, 1, deliveredCountTo(t, email))
	})

	t.Run("WasSentForList reads the stored list", func(t *testing.T) {
		email, messageID := deliver(t)
		other, err := factory.CreateList(workspace.ID)
		require.NoError(t, err)

		assert.True(t, wasSentFor(t, messageID, email, list.ID))
		assert.False(t, wasSentFor(t, messageID, email, other.ID))
		assert.False(t, wasSentFor(t, messageID, "someone-else@example.com", list.ID))
	})

	t.Run("WasSentForList falls back to the automation's list for a message stored without one", func(t *testing.T) {
		email, messageID := deliver(t)
		// Messages sent before automations recorded their list have a NULL list_id
		_, err := workspaceDB.ExecContext(context.Background(),
			`UPDATE message_history SET list_id = NULL WHERE id = $1`, messageID)
		require.NoError(t, err)
		other, err := factory.CreateList(workspace.ID)
		require.NoError(t, err)

		assert.True(t, wasSentFor(t, messageID, email, list.ID))
		assert.False(t, wasSentFor(t, messageID, email, other.ID))
	})

	// Last: it moves the automation to another list
	t.Run("WasSentForList keeps an email on its list after the automation moves", func(t *testing.T) {
		email, messageID := deliver(t)
		moved, err := factory.CreateList(workspace.ID)
		require.NoError(t, err)
		_, err = workspaceDB.ExecContext(context.Background(),
			`UPDATE automations SET list_id = $1 WHERE id = $2`, moved.ID, automationID)
		require.NoError(t, err)

		assert.True(t, wasSentFor(t, messageID, email, list.ID), "the email was sent for the old list")
		assert.False(t, wasSentFor(t, messageID, email, moved.ID), "it was never sent for the new one")
	})
}

// createActiveAutomation creates and activates an automation on listID: a custom event,
// then an email with templateID.
func createActiveAutomation(t *testing.T, client *testutil.APIClient, workspaceID, automationID, listID, eventName, templateID string) {
	t.Helper()
	triggerNodeID := shortuuid.New()
	emailNodeID := shortuuid.New()

	resp, err := client.CreateAutomation(map[string]interface{}{
		"workspace_id": workspaceID,
		"automation": map[string]interface{}{
			"id":           automationID,
			"workspace_id": workspaceID,
			"name":         "Complaint list status",
			"status":       "draft",
			"list_id":      listID,
			"trigger": map[string]interface{}{
				"event_kind":        "custom_event",
				"custom_event_name": eventName,
				"frequency":         "every_time",
			},
			"root_node_id": triggerNodeID,
			"nodes": []map[string]interface{}{
				{
					"id":            triggerNodeID,
					"automation_id": automationID,
					"type":          "trigger",
					"config":        map[string]interface{}{},
					"next_node_id":  emailNodeID,
					"position":      map[string]interface{}{"x": 0, "y": 0},
				},
				{
					"id":            emailNodeID,
					"automation_id": automationID,
					"type":          "email",
					"config":        map[string]interface{}{"template_id": templateID},
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
		"workspace_id":  workspaceID,
		"automation_id": automationID,
	})
	require.NoError(t, err)
	body, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "activate automation: %s", body)
}

// deliveredHeader returns the one value of a header of a delivered email, matching the
// name case-insensitively as mail headers are.
func deliveredHeader(t *testing.T, msg *testutil.MailpitMessage, name string) string {
	t.Helper()
	for key, values := range msg.Headers {
		if strings.EqualFold(key, name) {
			require.Len(t, values, 1, "expected one %s header", name)
			return strings.TrimSpace(values[0])
		}
	}
	require.Failf(t, "missing header", "the delivered email has no %s header", name)
	return ""
}

// postSESNotification POSTs an SNS-wrapped SES notification and expects 200.
func postSESNotification(t *testing.T, client *testutil.APIClient, webhookURL, payload string) {
	t.Helper()
	resp, err := client.PostRaw(webhookURL, payload)
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "webhook POST: %s", body)
}

// sesComplaintPayload builds an SNS-wrapped SES complaint notification in the format AWS
// publishes for a configuration set's event destination. SES copies the sent email's
// headers into mail.headers, which is how the complaint names the Notifuse message.
func sesComplaintPayload(notifuseMessageID, recipientEmail string) string {
	now := time.Now().UTC().Format(time.RFC3339)
	message := map[string]interface{}{
		"eventType": "Complaint",
		"complaint": map[string]interface{}{
			"complainedRecipients":  []map[string]interface{}{{"emailAddress": recipientEmail}},
			"timestamp":             now,
			"feedbackId":            "feedback-" + uuid.New().String(),
			"complaintFeedbackType": "abuse",
		},
		"mail": map[string]interface{}{
			"timestamp":   now,
			"messageId":   "ses-" + uuid.New().String(),
			"source":      "test@example.com",
			"destination": []string{recipientEmail},
			"headers": []map[string]string{
				{"name": "X-Message-ID", "value": notifuseMessageID},
				{"name": "To", "value": recipientEmail},
			},
		},
	}
	messageBytes, _ := json.Marshal(message)

	envelope := map[string]interface{}{
		"Type":      "Notification",
		"MessageId": "sns-" + uuid.New().String(),
		"TopicArn":  "arn:aws:sns:us-east-1:123456789012:test-topic",
		"Message":   string(messageBytes),
		"Timestamp": now,
	}
	envelopeBytes, _ := json.Marshal(envelope)
	return string(envelopeBytes)
}
