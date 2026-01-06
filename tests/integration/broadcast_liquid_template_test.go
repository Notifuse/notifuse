//go:build integration

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
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBroadcastLiquidTemplateSubstitution tests that Liquid template variables
// are correctly substituted with contact data in broadcast emails (Issue #169)
func TestBroadcastLiquidTemplateSubstitution(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	client := suite.APIClient
	factory := suite.DataFactory

	// Setup: Create user and workspace
	t.Log("Setting up user and workspace...")
	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)

	err = factory.AddUserToWorkspace(user.ID, workspace.ID, "owner")
	require.NoError(t, err)

	// Setup SMTP provider
	t.Log("Setting up SMTP provider...")
	_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID,
		testutil.WithIntegrationEmailProvider(domain.EmailProvider{
			Kind: domain.EmailProviderKindSMTP,
			Senders: []domain.EmailSender{
				domain.NewEmailSender("noreply@notifuse.test", "Notifuse Liquid Test"),
			},
			SMTP: &domain.SMTPSettings{
				Host:     "localhost",
				Port:     1025,
				Username: "",
				Password: "",
				UseTLS:   false,
			},
			RateLimitPerMinute: 2000,
		}))
	require.NoError(t, err)

	// Login
	err = client.Login(user.Email, "password")
	require.NoError(t, err)
	client.SetWorkspaceID(workspace.ID)

	// Clear Mailpit before tests
	t.Log("Clearing Mailpit messages...")
	err = testutil.ClearMailpitMessages(t)
	require.NoError(t, err)

	t.Run("substitutes contact fields in subject and body", func(t *testing.T) {
		// Clear Mailpit
		err := testutil.ClearMailpitMessages(t)
		require.NoError(t, err)

		// Create list
		list, err := factory.CreateList(workspace.ID,
			testutil.WithListName("Liquid Test List"))
		require.NoError(t, err)

		// Create contact with known first_name/last_name
		contactEmail := fmt.Sprintf("liquid-test-%s@example.com", uuid.New().String()[:8])
		contact, err := factory.CreateContact(workspace.ID,
			testutil.WithContactEmail(contactEmail),
			testutil.WithContactName("John", "Doe"))
		require.NoError(t, err)
		t.Logf("Created contact: %s with name John Doe", contact.Email)

		// Add contact to list
		_, err = factory.CreateContactList(workspace.ID,
			testutil.WithContactListEmail(contact.Email),
			testutil.WithContactListListID(list.ID),
			testutil.WithContactListStatus(domain.ContactListStatusActive))
		require.NoError(t, err)

		// Create template with Liquid variables
		uniqueID := uuid.New().String()[:8]
		template, err := factory.CreateTemplate(workspace.ID,
			testutil.WithTemplateName("Liquid Test Template"),
			testutil.WithTemplateSubject(fmt.Sprintf("Hello {{ contact.first_name }} - %s", uniqueID)),
			testutil.WithTemplateEmailContent("Welcome {{ contact.first_name }} {{ contact.last_name }}!"))
		require.NoError(t, err)
		t.Logf("Created template with subject containing Liquid: Hello {{ contact.first_name }} - %s", uniqueID)

		// Create broadcast
		broadcast, err := factory.CreateBroadcast(workspace.ID,
			testutil.WithBroadcastName("Liquid Test Broadcast"),
			testutil.WithBroadcastAudience(domain.AudienceSettings{
				List:                list.ID,
				ExcludeUnsubscribed: true,
			}))
		require.NoError(t, err)

		// Update broadcast to use our template
		broadcast.TestSettings.Variations[0].TemplateID = template.ID
		updateReq := map[string]interface{}{
			"workspace_id":  workspace.ID,
			"id":            broadcast.ID,
			"name":          broadcast.Name,
			"audience":      broadcast.Audience,
			"schedule":      broadcast.Schedule,
			"test_settings": broadcast.TestSettings,
		}
		updateResp, err := client.UpdateBroadcast(updateReq)
		require.NoError(t, err)
		updateResp.Body.Close()

		// Schedule broadcast to send now
		t.Log("Scheduling broadcast to send now...")
		scheduleResp, err := client.ScheduleBroadcast(map[string]interface{}{
			"workspace_id": workspace.ID,
			"id":           broadcast.ID,
			"send_now":     true,
		})
		require.NoError(t, err)
		scheduleResp.Body.Close()

		// Wait for broadcast completion
		t.Log("Waiting for broadcast completion...")
		_, err = testutil.WaitForBroadcastStatusWithExecution(t, client, broadcast.ID,
			[]string{"processed", "completed"}, 60*time.Second)
		require.NoError(t, err)

		// Fetch email from Mailpit
		t.Log("Fetching email from Mailpit...")
		msg, err := waitForEmailByRecipient(t, contactEmail, 15*time.Second)
		require.NoError(t, err, "Should receive email in Mailpit")

		t.Logf("Email subject: %s", msg.Subject)
		t.Logf("Email HTML (first 200 chars): %s", truncateString(msg.HTML, 200))

		// Assert Liquid substitution worked in subject
		assert.Contains(t, msg.Subject, "Hello John", "Subject should contain interpolated first name")
		assert.NotContains(t, msg.Subject, "{{", "Subject should NOT contain raw Liquid syntax")
		assert.Contains(t, msg.Subject, uniqueID, "Subject should contain unique ID")

		// Assert Liquid substitution worked in body
		assert.Contains(t, msg.HTML, "Welcome John Doe!", "HTML body should contain interpolated name")
		assert.NotContains(t, msg.HTML, "{{ contact.first_name }}", "HTML body should NOT contain raw Liquid")
		assert.NotContains(t, msg.HTML, "{{ contact.last_name }}", "HTML body should NOT contain raw Liquid")
	})

	t.Run("handles missing contact fields gracefully", func(t *testing.T) {
		// Clear Mailpit
		err := testutil.ClearMailpitMessages(t)
		require.NoError(t, err)

		// Create list
		list, err := factory.CreateList(workspace.ID,
			testutil.WithListName("Liquid Test List No Name"))
		require.NoError(t, err)

		// Create contact WITHOUT first_name/last_name (only email)
		contactEmail := fmt.Sprintf("no-name-test-%s@example.com", uuid.New().String()[:8])
		contact, err := factory.CreateContact(workspace.ID,
			testutil.WithContactEmail(contactEmail))
		require.NoError(t, err)
		t.Logf("Created contact: %s with NO name set", contact.Email)

		// Add contact to list
		_, err = factory.CreateContactList(workspace.ID,
			testutil.WithContactListEmail(contact.Email),
			testutil.WithContactListListID(list.ID),
			testutil.WithContactListStatus(domain.ContactListStatusActive))
		require.NoError(t, err)

		// Create template with Liquid variables
		uniqueID := uuid.New().String()[:8]
		template, err := factory.CreateTemplate(workspace.ID,
			testutil.WithTemplateName("Liquid Test Template No Name"),
			testutil.WithTemplateSubject(fmt.Sprintf("Hello {{ contact.first_name }} - %s", uniqueID)),
			testutil.WithTemplateEmailContent("Welcome {{ contact.first_name }} {{ contact.last_name }}!"))
		require.NoError(t, err)

		// Create broadcast
		broadcast, err := factory.CreateBroadcast(workspace.ID,
			testutil.WithBroadcastName("Liquid Test Broadcast No Name"),
			testutil.WithBroadcastAudience(domain.AudienceSettings{
				List:                list.ID,
				ExcludeUnsubscribed: true,
			}))
		require.NoError(t, err)

		// Update broadcast to use our template
		broadcast.TestSettings.Variations[0].TemplateID = template.ID
		updateReq := map[string]interface{}{
			"workspace_id":  workspace.ID,
			"id":            broadcast.ID,
			"name":          broadcast.Name,
			"audience":      broadcast.Audience,
			"schedule":      broadcast.Schedule,
			"test_settings": broadcast.TestSettings,
		}
		updateResp, err := client.UpdateBroadcast(updateReq)
		require.NoError(t, err)
		updateResp.Body.Close()

		// Schedule broadcast to send now
		t.Log("Scheduling broadcast to send now...")
		scheduleResp, err := client.ScheduleBroadcast(map[string]interface{}{
			"workspace_id": workspace.ID,
			"id":           broadcast.ID,
			"send_now":     true,
		})
		require.NoError(t, err)
		scheduleResp.Body.Close()

		// Wait for broadcast completion
		t.Log("Waiting for broadcast completion...")
		_, err = testutil.WaitForBroadcastStatusWithExecution(t, client, broadcast.ID,
			[]string{"processed", "completed"}, 60*time.Second)
		require.NoError(t, err)

		// Fetch email from Mailpit
		t.Log("Fetching email from Mailpit...")
		msg, err := waitForEmailByRecipient(t, contactEmail, 15*time.Second)
		require.NoError(t, err, "Should receive email in Mailpit")

		t.Logf("Email subject: %s", msg.Subject)
		t.Logf("Email HTML (first 200 chars): %s", truncateString(msg.HTML, 200))

		// Assert: No raw Liquid syntax should appear (should render as empty/blank)
		assert.NotContains(t, msg.Subject, "{{", "Subject should NOT contain raw Liquid syntax")
		assert.NotContains(t, msg.Subject, "}}", "Subject should NOT contain raw Liquid syntax")
		assert.NotContains(t, msg.HTML, "{{ contact.first_name }}", "HTML should NOT contain raw Liquid")
		assert.NotContains(t, msg.HTML, "{{ contact.last_name }}", "HTML should NOT contain raw Liquid")

		// Subject should still have the unique ID (template rendered, just with empty name)
		assert.Contains(t, msg.Subject, uniqueID, "Subject should contain unique ID (template was rendered)")
	})

	t.Run("renders system variables like unsubscribe_url and notification_center_url", func(t *testing.T) {
		// Clear Mailpit
		err := testutil.ClearMailpitMessages(t)
		require.NoError(t, err)

		// Create list with a specific name to verify list.name variable
		listName := fmt.Sprintf("Subscribers-%s", uuid.New().String()[:8])
		list, err := factory.CreateList(workspace.ID,
			testutil.WithListName(listName))
		require.NoError(t, err)

		// Create contact
		contactEmail := fmt.Sprintf("sysvar-test-%s@example.com", uuid.New().String()[:8])
		contact, err := factory.CreateContact(workspace.ID,
			testutil.WithContactEmail(contactEmail),
			testutil.WithContactName("Jane", "Smith"))
		require.NoError(t, err)
		t.Logf("Created contact: %s", contact.Email)

		// Add contact to list
		_, err = factory.CreateContactList(workspace.ID,
			testutil.WithContactListEmail(contact.Email),
			testutil.WithContactListListID(list.ID),
			testutil.WithContactListStatus(domain.ContactListStatusActive))
		require.NoError(t, err)

		// Create template with system variables (Issue #180)
		uniqueID := uuid.New().String()[:8]
		template, err := factory.CreateTemplate(workspace.ID,
			testutil.WithTemplateName("System Vars Template"),
			testutil.WithTemplateSubject(fmt.Sprintf("Newsletter - %s", uniqueID)),
			testutil.WithTemplateEmailContent(`Hello {{ contact.first_name }}! <a href="{{ unsubscribe_url }}">Unsubscribe</a> | <a href="{{ notification_center_url }}">Manage Preferences</a>`))
		require.NoError(t, err)
		t.Log("Created template with system variables: unsubscribe_url, notification_center_url")

		// Create broadcast with a specific name to verify broadcast.name variable
		broadcastName := fmt.Sprintf("Weekly Newsletter-%s", uuid.New().String()[:8])
		broadcast, err := factory.CreateBroadcast(workspace.ID,
			testutil.WithBroadcastName(broadcastName),
			testutil.WithBroadcastAudience(domain.AudienceSettings{
				List:                list.ID,
				ExcludeUnsubscribed: true,
			}))
		require.NoError(t, err)

		// Update broadcast to use our template
		broadcast.TestSettings.Variations[0].TemplateID = template.ID
		updateReq := map[string]interface{}{
			"workspace_id":  workspace.ID,
			"id":            broadcast.ID,
			"name":          broadcast.Name,
			"audience":      broadcast.Audience,
			"schedule":      broadcast.Schedule,
			"test_settings": broadcast.TestSettings,
		}
		updateResp, err := client.UpdateBroadcast(updateReq)
		require.NoError(t, err)
		updateResp.Body.Close()

		// Schedule broadcast to send now
		t.Log("Scheduling broadcast to send now...")
		scheduleResp, err := client.ScheduleBroadcast(map[string]interface{}{
			"workspace_id": workspace.ID,
			"id":           broadcast.ID,
			"send_now":     true,
		})
		require.NoError(t, err)
		scheduleResp.Body.Close()

		// Wait for broadcast completion
		t.Log("Waiting for broadcast completion...")
		_, err = testutil.WaitForBroadcastStatusWithExecution(t, client, broadcast.ID,
			[]string{"processed", "completed"}, 60*time.Second)
		require.NoError(t, err)

		// Fetch email from Mailpit
		t.Log("Fetching email from Mailpit...")
		msg, err := waitForEmailByRecipient(t, contactEmail, 15*time.Second)
		require.NoError(t, err, "Should receive email in Mailpit")

		t.Logf("Email subject: %s", msg.Subject)
		t.Logf("Email HTML (first 500 chars): %s", truncateString(msg.HTML, 500))

		// Assert system variables are rendered (not empty or raw Liquid)
		// URLs are wrapped in click tracking, so check for URL-encoded notification-center
		assert.Contains(t, msg.HTML, "notification-center",
			"notification_center_url should be rendered to actual URL")
		assert.Contains(t, msg.HTML, "action",
			"unsubscribe_url should contain action parameter")
		assert.NotContains(t, msg.HTML, `href=""`,
			"System variable URLs should not be empty")
		assert.NotContains(t, msg.HTML, "{{ unsubscribe_url }}",
			"Raw Liquid syntax should not appear for unsubscribe_url")
		assert.NotContains(t, msg.HTML, "{{ notification_center_url }}",
			"Raw Liquid syntax should not appear for notification_center_url")

		// Verify contact variable still works alongside system variables
		assert.Contains(t, msg.HTML, "Hello Jane!",
			"Contact first_name should be rendered")
	})
}

// waitForEmailByRecipient waits for an email to arrive for a specific recipient
func waitForEmailByRecipient(t *testing.T, recipientEmail string, timeout time.Duration) (*testutil.MailpitMessage, error) {
	deadline := time.Now().Add(timeout)
	httpClient := &http.Client{Timeout: 5 * time.Second}
	pollInterval := 500 * time.Millisecond

	for time.Now().Before(deadline) {
		resp, err := httpClient.Get("http://localhost:8025/api/v1/messages")
		if err != nil {
			t.Logf("Failed to connect to Mailpit: %v", err)
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

		for _, msg := range apiResp.Messages {
			for _, to := range msg.To {
				if strings.EqualFold(recipientEmail, to.Address) {
					t.Logf("Found email for recipient: %s (message ID: %s)", recipientEmail, msg.ID)
					return testutil.GetMailpitMessage(t, msg.ID)
				}
			}
		}
		time.Sleep(pollInterval)
	}
	return nil, fmt.Errorf("timeout waiting for email to %s after %v", recipientEmail, timeout)
}

// truncateString truncates a string to maxLen characters and adds "..." if truncated
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
