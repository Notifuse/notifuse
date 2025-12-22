package integration

import (
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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBroadcastPagination100Contacts is a fast CI test for Issue #157
// Tests broadcast delivery to 100 contacts with identical created_at timestamps
// With default batch size of 50, this forces 2 pagination rounds
func TestBroadcastPagination100Contacts(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	runBroadcastPaginationTest(t, suite, 100)
}

// TestBroadcastPagination1000Contacts is the full regression test for Issue #157
// Tests broadcast delivery to 1000 contacts with identical created_at timestamps
// With default batch size of 50, this forces 20 pagination rounds - sufficient to expose the bug
func TestBroadcastPagination1000Contacts(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	runBroadcastPaginationTest(t, suite, 1000)
}

// runBroadcastPaginationTest is the shared test logic for Issue #157
// It verifies that ALL contacts receive the broadcast email even when:
// 1. All contacts have identical created_at timestamps (bulk import scenario)
// 2. Multiple pagination rounds occur (default batch size is 50)
func runBroadcastPaginationTest(t *testing.T, suite *testutil.IntegrationTestSuite, contactCount int) {
	client := suite.APIClient
	factory := suite.DataFactory

	// Default batch size is 50 in broadcast config
	batchSize := 50
	t.Logf("=== Starting Broadcast Pagination Test (Issue #157) ===")
	t.Logf("Contact count: %d", contactCount)
	t.Logf("Expected pagination rounds: %d (with default batch size %d)", (contactCount+batchSize-1)/batchSize, batchSize)

	// Step 1: Create user and workspace
	t.Log("Step 1: Creating user and workspace...")
	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)

	err = factory.AddUserToWorkspace(user.ID, workspace.ID, "owner")
	require.NoError(t, err)

	// Step 2: Setup SMTP provider with HIGH rate limit for fast testing
	t.Log("Step 2: Setting up SMTP provider with high rate limit...")
	integration, err := factory.SetupWorkspaceWithSMTPProvider(workspace.ID,
		testutil.WithIntegrationEmailProvider(domain.EmailProvider{
			Kind: domain.EmailProviderKindSMTP,
			Senders: []domain.EmailSender{
				domain.NewEmailSender("noreply@notifuse.test", "Notifuse Pagination Test"),
			},
			SMTP: &domain.SMTPSettings{
				Host:     "localhost",
				Port:     1025,
				Username: "",
				Password: "",
				UseTLS:   false,
			},
			RateLimitPerMinute: 2000, // High rate limit for fast test execution
		}))
	require.NoError(t, err)
	t.Logf("Created SMTP integration: %s", integration.ID)

	// Step 3: Login
	t.Log("Step 3: Logging in...")
	err = client.Login(user.Email, "password")
	require.NoError(t, err)
	client.SetWorkspaceID(workspace.ID)

	// Step 4: Clear Mailpit messages from previous tests
	t.Log("Step 4: Clearing Mailpit messages...")
	err = testutil.ClearMailpitMessages(t)
	require.NoError(t, err)

	// Step 5: Create a contact list
	t.Log("Step 5: Creating contact list...")
	list, err := factory.CreateList(workspace.ID,
		testutil.WithListName("Pagination Test List"))
	require.NoError(t, err)
	t.Logf("Created list: %s", list.ID)

	// Step 6: Generate contacts with IDENTICAL created_at timestamp
	// This is the key condition that triggers Issue #157
	t.Logf("Step 6: Generating %d contacts with identical created_at timestamp...", contactCount)
	sameTimestamp := time.Now().UTC()
	contacts := make([]map[string]interface{}, contactCount)
	expectedEmails := make(map[string]bool)

	for i := 0; i < contactCount; i++ {
		email := fmt.Sprintf("pagination-test-%04d@example.com", i)
		contacts[i] = map[string]interface{}{
			"email":      email,
			"first_name": fmt.Sprintf("User%d", i),
			"last_name":  "PaginationTest",
			"created_at": sameTimestamp.Format(time.RFC3339),
		}
		expectedEmails[email] = false
	}

	// Step 7: Bulk import contacts via API with list subscription
	t.Logf("Step 7: Bulk importing %d contacts...", contactCount)
	resp, err := client.BatchImportContacts(contacts, []string{list.ID})
	require.NoError(t, err)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Failed to import contacts: %d - %s", resp.StatusCode, string(body))
	}

	var importResult map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&importResult)
	require.NoError(t, err)

	// Verify import succeeded
	operations, ok := importResult["operations"].([]interface{})
	if ok {
		createCount := 0
		errorCount := 0
		for _, op := range operations {
			opMap := op.(map[string]interface{})
			action := opMap["action"].(string)
			if action == "create" {
				createCount++
			} else if action == "error" {
				errorCount++
				t.Logf("Import error for %s: %s", opMap["email"], opMap["error"])
			}
		}
		t.Logf("Import result: %d created, %d errors", createCount, errorCount)
		require.Equal(t, contactCount, createCount, "All contacts should be created")
		require.Equal(t, 0, errorCount, "No import errors expected")
	}

	// Step 8: Create template with unique subject for Mailpit verification
	t.Log("Step 8: Creating email template...")
	uniqueSubject := fmt.Sprintf("Pagination Test %s", uuid.New().String()[:8])
	template, err := factory.CreateTemplate(workspace.ID,
		testutil.WithTemplateName("Pagination Test Template"),
		testutil.WithTemplateSubject(uniqueSubject))
	require.NoError(t, err)
	t.Logf("Created template with subject: %s", uniqueSubject)

	// Step 9: Create broadcast targeting the list
	t.Log("Step 9: Creating broadcast...")
	broadcast, err := factory.CreateBroadcast(workspace.ID,
		testutil.WithBroadcastName(fmt.Sprintf("Pagination Test Broadcast %d contacts", contactCount)),
		testutil.WithBroadcastAudience(domain.AudienceSettings{
			List:                list.ID,
			ExcludeUnsubscribed: true,
		}))
	require.NoError(t, err)
	t.Logf("Created broadcast: %s", broadcast.ID)

	// Step 10: Update broadcast to use our template
	t.Log("Step 10: Updating broadcast with template...")
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
	defer updateResp.Body.Close()
	require.Equal(t, http.StatusOK, updateResp.StatusCode, "Broadcast update should succeed")

	// Step 11: Schedule broadcast for immediate sending
	t.Log("Step 11: Scheduling broadcast for immediate sending...")
	scheduleRequest := map[string]interface{}{
		"workspace_id": workspace.ID,
		"id":           broadcast.ID,
		"send_now":     true,
	}

	scheduleResp, err := client.ScheduleBroadcast(scheduleRequest)
	require.NoError(t, err)
	defer scheduleResp.Body.Close()

	if scheduleResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(scheduleResp.Body)
		t.Fatalf("Failed to schedule broadcast: %d - %s", scheduleResp.StatusCode, string(body))
	}

	// Step 12: Wait for broadcast completion
	timeout := 5 * time.Minute
	if contactCount >= 1000 {
		timeout = 10 * time.Minute
	}
	t.Logf("Step 12: Waiting for broadcast completion (timeout: %v)...", timeout)

	finalStatus, err := testutil.WaitForBroadcastStatusWithExecution(t, client, broadcast.ID,
		[]string{"processed", "completed"}, timeout)
	require.NoError(t, err, "Broadcast should complete successfully")
	t.Logf("Broadcast completed with status: %s", finalStatus)

	// Step 13: Wait for all emails to arrive in Mailpit
	mailpitTimeout := 3 * time.Minute
	if contactCount >= 1000 {
		mailpitTimeout = 5 * time.Minute
	}
	t.Logf("Step 13: Waiting for %d emails in Mailpit (timeout: %v)...", contactCount, mailpitTimeout)

	err = testutil.WaitForMailpitMessages(t, uniqueSubject, contactCount, mailpitTimeout)
	if err != nil {
		t.Logf("Warning: Not all emails arrived in Mailpit: %v", err)
		// Continue to verification to see exactly how many we got
	}

	// Step 14: Verify ALL recipients received the email
	t.Log("Step 14: Verifying all recipients received the email...")
	receivedEmails, err := testutil.GetAllMailpitRecipients(t, uniqueSubject)
	require.NoError(t, err)

	// Find missing recipients
	var missing []string
	for email := range expectedEmails {
		if !receivedEmails[email] {
			missing = append(missing, email)
		}
	}

	// Sort missing emails for easier debugging
	sort.Strings(missing)

	// Log detailed results
	t.Logf("=== VERIFICATION RESULTS ===")
	t.Logf("Expected recipients: %d", contactCount)
	t.Logf("Received emails: %d", len(receivedEmails))
	t.Logf("Missing recipients: %d", len(missing))

	// Log missing recipients for debugging
	if len(missing) > 0 {
		t.Logf("MISSING RECIPIENTS (%d):", len(missing))
		// Log first 50 missing for readability
		for i, email := range missing {
			if i >= 50 {
				t.Logf("  ... and %d more", len(missing)-50)
				break
			}
			t.Logf("  - %s", email)
		}
	}

	// Step 15: Verify broadcast status
	t.Log("Step 15: Verifying broadcast completed successfully...")
	broadcastResp, err := client.GetBroadcast(broadcast.ID)
	require.NoError(t, err)
	defer broadcastResp.Body.Close()

	var broadcastResult map[string]interface{}
	err = json.NewDecoder(broadcastResp.Body).Decode(&broadcastResult)
	require.NoError(t, err)

	broadcastData, ok := broadcastResult["broadcast"].(map[string]interface{})
	require.True(t, ok, "Broadcast data should be present")

	// Verify broadcast status is "processed" or "completed"
	status, _ := broadcastData["status"].(string)
	t.Logf("Broadcast final status: %s", status)

	// Final assertions
	t.Log("=== FINAL ASSERTIONS ===")

	// Assert broadcast completed with processed status
	assert.Contains(t, []string{"processed", "completed"}, status,
		"Broadcast should be in 'processed' or 'completed' status, got: %s", status)

	// Assert no missing recipients (the key Issue #157 verification)
	assert.Empty(t, missing,
		"Issue #157: All recipients should receive the broadcast. Missing: %d/%d",
		len(missing), contactCount)

	// Assert correct email count in Mailpit
	assert.Equal(t, contactCount, len(receivedEmails),
		"Mailpit should have exactly %d unique recipients, got %d",
		contactCount, len(receivedEmails))

	t.Logf("=== Test completed successfully ===")
}
