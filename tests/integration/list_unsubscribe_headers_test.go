package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestListUnsubscribeHeaders tests that RFC-8058 List-Unsubscribe headers are properly
// included in broadcast emails. This verifies the one-click unsubscribe compliance
// required by Gmail and Yahoo since 2024.
func TestListUnsubscribeHeaders(t *testing.T) {
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

	// Set up SMTP email provider for testing (Mailpit)
	_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID)
	require.NoError(t, err)

	// Login to get auth token
	err = client.Login(user.Email, "password")
	require.NoError(t, err)
	client.SetWorkspaceID(workspace.ID)

	// Clear any existing messages in Mailpit
	err = testutil.ClearMailpitMessages(t)
	if err != nil {
		t.Logf("Warning: Could not clear Mailpit messages: %v", err)
	}

	t.Run("broadcast email should include List-Unsubscribe headers", func(t *testing.T) {
		// Create a list
		list, err := factory.CreateList(workspace.ID)
		require.NoError(t, err)

		// Create a contact and add to list
		contact, err := factory.CreateContact(workspace.ID)
		require.NoError(t, err)

		// Add contact to list using CreateContactList with options
		_, err = factory.CreateContactList(workspace.ID,
			testutil.WithContactListEmail(contact.Email),
			testutil.WithContactListListID(list.ID))
		require.NoError(t, err)

		// Create templates for A/B testing (requires at least 2)
		template1, err := factory.CreateTemplate(workspace.ID)
		require.NoError(t, err)
		template2, err := factory.CreateTemplate(workspace.ID)
		require.NoError(t, err)

		// Create broadcast with A/B testing AND audience list set
		broadcast, err := factory.CreateBroadcast(workspace.ID,
			testutil.WithBroadcastABTesting([]string{template1.ID, template2.ID}),
			testutil.WithBroadcastAudience(domain.AudienceSettings{
				List:                list.ID,
				ExcludeUnsubscribed: true,
			}))
		require.NoError(t, err)

		// Clear Mailpit before sending
		err = testutil.ClearMailpitMessages(t)
		if err != nil {
			t.Logf("Warning: Could not clear Mailpit messages: %v", err)
		}

		// Send broadcast to individual (this is what the existing test uses)
		sendRequest := map[string]interface{}{
			"workspace_id":    workspace.ID,
			"broadcast_id":    broadcast.ID,
			"recipient_email": contact.Email,
			"template_id":     template1.ID,
		}

		resp, err := client.SendBroadcastToIndividual(sendRequest)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		// With SMTP provider configured, this should succeed
		if resp.StatusCode != http.StatusOK {
			var errResult map[string]interface{}
			_ = json.NewDecoder(resp.Body).Decode(&errResult)
			t.Fatalf("Failed to send broadcast: status=%d, response=%+v", resp.StatusCode, errResult)
		}

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		if successInterface, ok := result["success"]; ok && successInterface != nil {
			assert.True(t, successInterface.(bool), "Send should succeed")
		} else {
			t.Errorf("success field is missing or nil in send individual response: %+v", result)
		}

		// Wait for email to arrive in Mailpit and get the full message with headers
		msg, err := waitForEmailAndGetMessage(t, contact.Email, 10*time.Second)
		require.NoError(t, err, "Should receive email in Mailpit")

		t.Logf("Email headers: %+v", msg.Headers)

		// Verify List-Unsubscribe header is present
		// Mailpit properly handles RFC 5322 header folding, so we can check parsed headers
		listUnsubscribe, hasListUnsubscribe := msg.Headers["List-Unsubscribe"]
		assert.True(t, hasListUnsubscribe, "Email should have List-Unsubscribe header")
		if hasListUnsubscribe && len(listUnsubscribe) > 0 {
			t.Logf("List-Unsubscribe header: %s", listUnsubscribe[0])
			// The header should contain a URL in angle brackets
			assert.Contains(t, listUnsubscribe[0], "<", "List-Unsubscribe should contain URL in angle brackets")
			assert.Contains(t, listUnsubscribe[0], ">", "List-Unsubscribe should contain URL in angle brackets")
			assert.Contains(t, listUnsubscribe[0], "unsubscribe", "List-Unsubscribe URL should contain 'unsubscribe'")
		}

		// Verify List-Unsubscribe-Post header is present
		listUnsubscribePost, hasListUnsubscribePost := msg.Headers["List-Unsubscribe-Post"]
		assert.True(t, hasListUnsubscribePost, "Email should have List-Unsubscribe-Post header")
		if hasListUnsubscribePost && len(listUnsubscribePost) > 0 {
			t.Logf("List-Unsubscribe-Post header: %s", listUnsubscribePost[0])
			// The header should be exactly "List-Unsubscribe=One-Click" per RFC-8058
			assert.Equal(t, "List-Unsubscribe=One-Click", listUnsubscribePost[0],
				"List-Unsubscribe-Post should be 'List-Unsubscribe=One-Click'")
		}
	})
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

		// Check each message for matching recipient
		for _, msg := range apiResp.Messages {
			for _, to := range msg.To {
				if strings.EqualFold(recipientEmail, to.Address) {
					t.Logf("Found email for recipient: %s", recipientEmail)
					// Get the full message with headers
					fullMsg, err := testutil.GetMailpitMessage(t, msg.ID)
					if err != nil {
						t.Logf("Failed to get full message: %v", err)
						continue
					}
					return fullMsg, nil
				}
			}
		}

		time.Sleep(pollInterval)
	}

	return nil, fmt.Errorf("timeout waiting for email to %s after %v", recipientEmail, timeout)
}
