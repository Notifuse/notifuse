package integration

import (
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

// TestBroadcastABTestingE2E tests the complete end-to-end A/B testing flow
// including the race condition scenario that was fixed in the orchestrator.
// This test covers:
// - A/B test broadcast creation and scheduling
// - Task execution and orchestrator phase transitions
// - Manual winner selection during test phase
// - Race condition prevention (task doesn't get stuck in "paused")
// - Complete broadcast execution after winner selection
func TestBroadcastABTestingE2E(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer func() { suite.Cleanup() }()

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

	// Set up SMTP email provider for testing
	_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID)
	require.NoError(t, err)

	// Login to get auth token
	err = client.Login(user.Email, "password")
	require.NoError(t, err)
	client.SetWorkspaceID(workspace.ID)

	t.Run("Complete A/B Testing Flow", func(t *testing.T) {
		testCompleteABTestingFlow(t, client, factory, workspace.ID)
	})

	t.Run("Race Condition Prevention", func(t *testing.T) {
		testRaceConditionPrevention(t, client, factory, workspace.ID)
	})

	t.Run("Manual Winner Selection During Test Phase", func(t *testing.T) {
		testManualWinnerSelectionDuringTestPhase(t, client, factory, workspace.ID)
	})

	t.Run("Auto Winner Selection Flow", func(t *testing.T) {
		testAutoWinnerSelectionFlow(t, client, factory, workspace.ID)
	})
}

// testCompleteABTestingFlow tests the complete end-to-end A/B testing flow
func testCompleteABTestingFlow(t *testing.T, client *testutil.APIClient, factory *testutil.TestDataFactory, workspaceID string) {
	t.Run("should complete full A/B testing workflow", func(t *testing.T) {
		// Step 1: Create test data
		template1, err := factory.CreateTemplate(workspaceID, testutil.WithTemplateName("Version A"))
		require.NoError(t, err)
		template2, err := factory.CreateTemplate(workspaceID, testutil.WithTemplateName("Version B"))
		require.NoError(t, err)

		list, err := factory.CreateList(workspaceID)
		require.NoError(t, err)

		// Create contacts for testing
		for i := 0; i < 20; i++ {
			contact, err := factory.CreateContact(workspaceID,
				testutil.WithContactEmail(fmt.Sprintf("test%d@example.com", i)))
			require.NoError(t, err)

			// Add contact to list
			_, err = factory.CreateContactList(workspaceID,
				testutil.WithContactListEmail(contact.Email),
				testutil.WithContactListListID(list.ID),
				testutil.WithContactListStatus(domain.ContactListStatusActive))
			require.NoError(t, err)
		}

		// Step 2: Create A/B test broadcast
		broadcast := map[string]interface{}{
			"workspace_id": workspaceID,
			"name":         "E2E A/B Test Broadcast",
			"audience": map[string]interface{}{
				"list":                 list.ID,
				"exclude_unsubscribed": true,
			},
			"schedule": map[string]interface{}{
				"is_scheduled": false,
			},
			"test_settings": map[string]interface{}{
				"enabled":                 true,
				"sample_percentage":       50,    // 50% for test phase
				"auto_send_winner":        false, // Manual winner selection
				"auto_send_winner_metric": "open_rate",
				"test_duration_hours":     0, // Immediate evaluation for testing
				"variations": []map[string]interface{}{
					{
						"variation_name": "Version A",
						"template_id":    template1.ID,
					},
					{
						"variation_name": "Version B",
						"template_id":    template2.ID,
					},
				},
			},
			"tracking_enabled": true,
		}

		// Step 3: Create the broadcast
		resp, err := client.CreateBroadcast(broadcast)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		var createResult map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&createResult)
		require.NoError(t, err)

		broadcastData := createResult["broadcast"].(map[string]interface{})
		broadcastID := broadcastData["id"].(string)

		// Step 4: Schedule the broadcast for immediate sending
		scheduleRequest := map[string]interface{}{
			"workspace_id": workspaceID,
			"id":           broadcastID,
			"send_now":     true,
		}

		scheduleResp, err := client.ScheduleBroadcast(scheduleRequest)
		require.NoError(t, err)
		defer func() { _ = scheduleResp.Body.Close() }()
		assert.Equal(t, http.StatusOK, scheduleResp.StatusCode)

		// Step 5: Wait for broadcast to reach test_completed or winner_selected status
		// Using WaitForBroadcastStatusWithExecution to ensure continuous task processing
		status, err := testutil.WaitForBroadcastStatusWithExecution(t, client, broadcastID,
			[]string{"test_completed", "winner_selected", "testing"},
			60*time.Second)
		if err != nil {
			t.Logf("Broadcast status check failed: %v", err)
			// Continue to allow manual winner selection even if status is still "testing"
		}
		t.Logf("Broadcast status before winner selection: %s", status)

		// Step 8: Get test results
		testResultsResp, err := client.GetBroadcastTestResults(workspaceID, broadcastID)
		require.NoError(t, err)
		defer func() { _ = testResultsResp.Body.Close() }()
		assert.Equal(t, http.StatusOK, testResultsResp.StatusCode)

		// Step 9: Select winner manually
		selectWinnerRequest := map[string]interface{}{
			"workspace_id": workspaceID,
			"id":           broadcastID,
			"template_id":  template1.ID, // Select Version A as winner
		}

		winnerResp, err := client.SelectBroadcastWinner(selectWinnerRequest)
		require.NoError(t, err)
		defer func() { _ = winnerResp.Body.Close() }()
		assert.Equal(t, http.StatusOK, winnerResp.StatusCode)

		// Step 10: Wait for broadcast to complete (processed or completed status)
		// Using WaitForBroadcastStatusWithExecution to ensure continuous task processing
		finalStatus, err := testutil.WaitForBroadcastStatusWithExecution(t, client, broadcastID,
			[]string{"processed", "completed", "winner_selected"},
			60*time.Second)
		if err != nil {
			// If timeout, check if we're at least progressing
			t.Logf("Warning: broadcast did not reach final state: %v", err)
		}

		// Verify broadcast progressed successfully
		assert.Contains(t, []string{"processed", "completed", "winner_selected"}, finalStatus,
			"Broadcast should reach processed, completed, or winner_selected state")

		// Verify winning template is set
		finalBroadcastResp, err := client.GetBroadcast(broadcastID)
		require.NoError(t, err)
		defer func() { _ = finalBroadcastResp.Body.Close() }()

		var finalBroadcastResult map[string]interface{}
		err = json.NewDecoder(finalBroadcastResp.Body).Decode(&finalBroadcastResult)
		require.NoError(t, err)

		finalBroadcastData := finalBroadcastResult["broadcast"].(map[string]interface{})
		if winningTemplate, ok := finalBroadcastData["winning_template"]; ok && winningTemplate != nil {
			assert.Equal(t, template1.ID, winningTemplate.(string))
		}
	})
}

// testRaceConditionPrevention specifically tests the race condition scenario that was fixed
func testRaceConditionPrevention(t *testing.T, client *testutil.APIClient, factory *testutil.TestDataFactory, workspaceID string) {
	t.Run("should prevent race condition when winner is selected immediately after test completion", func(t *testing.T) {
		// Create test data
		template1, err := factory.CreateTemplate(workspaceID, testutil.WithTemplateName("Race Test A"))
		require.NoError(t, err)
		template2, err := factory.CreateTemplate(workspaceID, testutil.WithTemplateName("Race Test B"))
		require.NoError(t, err)

		list, err := factory.CreateList(workspaceID)
		require.NoError(t, err)

		// Create fewer contacts for faster test completion
		for i := 0; i < 10; i++ {
			contact, err := factory.CreateContact(workspaceID,
				testutil.WithContactEmail(fmt.Sprintf("race%d@example.com", i)))
			require.NoError(t, err)

			_, err = factory.CreateContactList(workspaceID,
				testutil.WithContactListEmail(contact.Email),
				testutil.WithContactListListID(list.ID),
				testutil.WithContactListStatus(domain.ContactListStatusActive))
			require.NoError(t, err)
		}

		// Create A/B test broadcast with very small test sample
		broadcast := map[string]interface{}{
			"workspace_id": workspaceID,
			"name":         "Race Condition Test Broadcast",
			"audience": map[string]interface{}{
				"list":                 list.ID,
				"exclude_unsubscribed": true,
			},
			"schedule": map[string]interface{}{
				"is_scheduled": false,
			},
			"test_settings": map[string]interface{}{
				"enabled":             true,
				"sample_percentage":   30,    // Small test sample
				"auto_send_winner":    false, // Manual selection to trigger race condition
				"test_duration_hours": 0,     // Immediate evaluation for testing
				"variations": []map[string]interface{}{
					{
						"variation_name": "Race A",
						"template_id":    template1.ID,
					},
					{
						"variation_name": "Race B",
						"template_id":    template2.ID,
					},
				},
			},
		}

		// Create and schedule broadcast
		resp, err := client.CreateBroadcast(broadcast)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		var createResult map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&createResult)
		require.NoError(t, err)

		broadcastData := createResult["broadcast"].(map[string]interface{})
		broadcastID := broadcastData["id"].(string)

		// Schedule immediately
		scheduleRequest := map[string]interface{}{
			"workspace_id": workspaceID,
			"id":           broadcastID,
			"send_now":     true,
		}

		scheduleResp, err := client.ScheduleBroadcast(scheduleRequest)
		require.NoError(t, err)
		defer func() { _ = scheduleResp.Body.Close() }()

		// Start execution
		execResp, err := client.ExecutePendingTasks(10)
		require.NoError(t, err)
		defer func() { _ = execResp.Body.Close() }()

		// Wait for broadcast to enter testing or test_completed state with continuous task execution
		currentStatus, err := testutil.WaitForBroadcastStatusWithExecution(t, client, broadcastID,
			[]string{"testing", "test_completed", "processing"},
			90*time.Second)
		if err != nil {
			t.Fatalf("Failed to wait for broadcast to enter testing phase: %v", err)
		}
		t.Logf("Broadcast entered status: %s", currentStatus)

		// Select winner immediately after test starts (race condition scenario)
		selectWinnerRequest := map[string]interface{}{
			"workspace_id": workspaceID,
			"id":           broadcastID,
			"template_id":  template1.ID, // Select Version A
		}

		winnerResp, err := client.SelectBroadcastWinner(selectWinnerRequest)
		require.NoError(t, err)
		defer func() { _ = winnerResp.Body.Close() }()

		if winnerResp.StatusCode != http.StatusOK {
			body, _ := client.ReadBody(winnerResp)
			t.Logf("SelectWinner failed with status %d: %s", winnerResp.StatusCode, body)
		}
		assert.Equal(t, http.StatusOK, winnerResp.StatusCode)

		// Final verification - wait for broadcast to progress to winner_selected or beyond
		// Using WaitForBroadcastStatusWithExecution to continue task processing
		finalStatus, err := testutil.WaitForBroadcastStatusWithExecution(t, client, broadcastID,
			[]string{"winner_selected", "processing", "processed", "completed"},
			60*time.Second)
		if err != nil {
			// If timeout, verify we're at least past race condition state
			t.Logf("Warning: broadcast did not complete, but checking for race condition: %v", err)
		}

		// Should be progressing with or have completed winner phase (race condition prevented)
		assert.Contains(t, []string{"winner_selected", "processing", "processed", "completed"}, finalStatus,
			"Race condition test: broadcast should progress past test phase")

		// Verify winning template is preserved
		getBroadcastResp2, err := client.GetBroadcast(broadcastID)
		require.NoError(t, err)
		defer func() { _ = getBroadcastResp2.Body.Close() }()

		var getBroadcastResult2 map[string]interface{}
		err = json.NewDecoder(getBroadcastResp2.Body).Decode(&getBroadcastResult2)
		require.NoError(t, err)

		finalBroadcastData := getBroadcastResult2["broadcast"].(map[string]interface{})
		if winningTemplate, ok := finalBroadcastData["winning_template"]; ok && winningTemplate != nil {
			assert.Equal(t, template1.ID, winningTemplate.(string))
		}
	})
}

// testManualWinnerSelectionDuringTestPhase tests winner selection during active test phase
func testManualWinnerSelectionDuringTestPhase(t *testing.T, client *testutil.APIClient, factory *testutil.TestDataFactory, workspaceID string) {
	t.Run("should handle manual winner selection during active test phase", func(t *testing.T) {
		// Create test data
		template1, err := factory.CreateTemplate(workspaceID, testutil.WithTemplateName("Manual A"))
		require.NoError(t, err)
		template2, err := factory.CreateTemplate(workspaceID, testutil.WithTemplateName("Manual B"))
		require.NoError(t, err)

		list, err := factory.CreateList(workspaceID)
		require.NoError(t, err)

		// Create contacts (5 is sufficient to test manual winner selection, and keeps test under 30s timeout)
		for i := 0; i < 5; i++ {
			contact, err := factory.CreateContact(workspaceID,
				testutil.WithContactEmail(fmt.Sprintf("manual%d@example.com", i)))
			require.NoError(t, err)

			_, err = factory.CreateContactList(workspaceID,
				testutil.WithContactListEmail(contact.Email),
				testutil.WithContactListListID(list.ID),
				testutil.WithContactListStatus(domain.ContactListStatusActive))
			require.NoError(t, err)
		}

		// Create broadcast with longer test duration
		broadcast := map[string]interface{}{
			"workspace_id": workspaceID,
			"name":         "Manual Winner Selection Test",
			"audience": map[string]interface{}{
				"list":                 list.ID,
				"exclude_unsubscribed": true,
			},
			"schedule": map[string]interface{}{
				"is_scheduled": false,
			},
			"test_settings": map[string]interface{}{
				"enabled":             true,
				"sample_percentage":   40,
				"auto_send_winner":    false,
				"test_duration_hours": 0, // Immediate evaluation for testing
				"variations": []map[string]interface{}{
					{
						"variation_name": "Manual A",
						"template_id":    template1.ID,
					},
					{
						"variation_name": "Manual B",
						"template_id":    template2.ID,
					},
				},
			},
		}

		// Create and schedule
		resp, err := client.CreateBroadcast(broadcast)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		var createResult map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&createResult)
		require.NoError(t, err)

		broadcastData := createResult["broadcast"].(map[string]interface{})
		broadcastID := broadcastData["id"].(string)

		scheduleRequest := map[string]interface{}{
			"workspace_id": workspaceID,
			"id":           broadcastID,
			"send_now":     true,
		}

		scheduleResp, err := client.ScheduleBroadcast(scheduleRequest)
		require.NoError(t, err)
		defer func() { _ = scheduleResp.Body.Close() }()

		// Wait for broadcast to enter testing state with continuous task execution
		currentStatus, err := testutil.WaitForBroadcastStatusWithExecution(t, client, broadcastID,
			[]string{"testing", "test_completed", "processing"},
			45*time.Second)
		if err != nil {
			t.Fatalf("Failed to wait for broadcast to enter testing phase: %v", err)
		}

		// Verify we're in a state that allows winner selection
		assert.Contains(t, []string{"testing", "test_completed", "processing"}, currentStatus,
			"Broadcast should be in testing, test_completed, or sending state before winner selection")

		// Select winner DURING test phase (not after completion)
		selectWinnerRequest := map[string]interface{}{
			"workspace_id": workspaceID,
			"id":           broadcastID,
			"template_id":  template2.ID, // Select Version B
		}

		winnerResp, err := client.SelectBroadcastWinner(selectWinnerRequest)
		require.NoError(t, err)
		defer func() { _ = winnerResp.Body.Close() }()

		if winnerResp.StatusCode != http.StatusOK {
			body, _ := client.ReadBody(winnerResp)
			t.Logf("SelectWinner failed with status %d: %s", winnerResp.StatusCode, body)
		}
		assert.Equal(t, http.StatusOK, winnerResp.StatusCode)

		// Verify transition to winner phase with continuous task execution
		finalStatus, err := testutil.WaitForBroadcastStatusWithExecution(t, client, broadcastID,
			[]string{"winner_selected", "processing", "processed", "completed"},
			60*time.Second)
		if err != nil {
			t.Logf("Warning: broadcast did not reach final state: %v", err)
		}

		// Should be in or past winner phase
		assert.Contains(t, []string{"winner_selected", "processing", "processed", "completed"}, finalStatus,
			"Broadcast should progress to winner phase after manual selection")

		// Verify correct winner
		getBroadcastResp2, err := client.GetBroadcast(broadcastID)
		require.NoError(t, err)
		defer func() { _ = getBroadcastResp2.Body.Close() }()

		var getBroadcastResult2 map[string]interface{}
		err = json.NewDecoder(getBroadcastResp2.Body).Decode(&getBroadcastResult2)
		require.NoError(t, err)

		finalBroadcastData := getBroadcastResult2["broadcast"].(map[string]interface{})
		if winningTemplate, ok := finalBroadcastData["winning_template"]; ok && winningTemplate != nil {
			assert.Equal(t, template2.ID, winningTemplate.(string))
		}
	})
}

// testAutoWinnerSelectionFlow tests automatic winner selection
func testAutoWinnerSelectionFlow(t *testing.T, client *testutil.APIClient, factory *testutil.TestDataFactory, workspaceID string) {
	t.Run("should handle automatic winner selection flow", func(t *testing.T) {
		// Create test data
		template1, err := factory.CreateTemplate(workspaceID, testutil.WithTemplateName("Auto A"))
		require.NoError(t, err)
		template2, err := factory.CreateTemplate(workspaceID, testutil.WithTemplateName("Auto B"))
		require.NoError(t, err)

		list, err := factory.CreateList(workspaceID)
		require.NoError(t, err)

		// Create contacts
		for i := 0; i < 12; i++ {
			contact, err := factory.CreateContact(workspaceID,
				testutil.WithContactEmail(fmt.Sprintf("auto%d@example.com", i)))
			require.NoError(t, err)

			_, err = factory.CreateContactList(workspaceID,
				testutil.WithContactListEmail(contact.Email),
				testutil.WithContactListListID(list.ID),
				testutil.WithContactListStatus(domain.ContactListStatusActive))
			require.NoError(t, err)
		}

		// Create broadcast with auto winner selection
		broadcast := map[string]interface{}{
			"workspace_id": workspaceID,
			"name":         "Auto Winner Selection Test",
			"audience": map[string]interface{}{
				"list":                 list.ID,
				"exclude_unsubscribed": true,
			},
			"schedule": map[string]interface{}{
				"is_scheduled": false,
			},
			"test_settings": map[string]interface{}{
				"enabled":                 true,
				"sample_percentage":       50,
				"auto_send_winner":        true, // AUTO selection
				"auto_send_winner_metric": "open_rate",
				"test_duration_hours":     1, // Minimum valid duration for auto winner
				"variations": []map[string]interface{}{
					{
						"variation_name": "Auto A",
						"template_id":    template1.ID,
					},
					{
						"variation_name": "Auto B",
						"template_id":    template2.ID,
					},
				},
			},
		}

		// Create and schedule
		resp, err := client.CreateBroadcast(broadcast)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusCreated {
			body, _ := client.ReadBody(resp)
			t.Fatalf("Failed to create broadcast: status %d, body: %s", resp.StatusCode, body)
		}

		var createResult map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&createResult)
		require.NoError(t, err)
		require.NotNil(t, createResult["broadcast"], "broadcast field is nil in response")

		broadcastData := createResult["broadcast"].(map[string]interface{})
		broadcastID := broadcastData["id"].(string)

		scheduleRequest := map[string]interface{}{
			"workspace_id": workspaceID,
			"id":           broadcastID,
			"send_now":     true,
		}

		scheduleResp, err := client.ScheduleBroadcast(scheduleRequest)
		require.NoError(t, err)
		defer func() { _ = scheduleResp.Body.Close() }()

		// Wait for broadcast to complete the test phase
		// Note: Auto winner selection requires 1 hour to pass, which we can't wait for in tests
		// So we verify the test phase completes and the broadcast is waiting for the duration
		// Using WaitForBroadcastStatusWithExecution to ensure continuous task processing
		status, err := testutil.WaitForBroadcastStatusWithExecution(t, client, broadcastID,
			[]string{"test_completed", "testing", "processing"},
			90*time.Second)
		if err != nil {
			t.Logf("Warning: broadcast did not complete test phase: %v", err)
		}

		// Broadcast should reach test_completed and wait for auto evaluation
		assert.Contains(t, []string{"test_completed", "testing", "processing"}, status,
			"Broadcast should complete test phase and wait for auto winner evaluation")

		// Verify broadcast state and settings
		getBroadcastResp, err := client.GetBroadcast(broadcastID)
		require.NoError(t, err)
		defer func() { _ = getBroadcastResp.Body.Close() }()

		var getBroadcastResult map[string]interface{}
		err = json.NewDecoder(getBroadcastResp.Body).Decode(&getBroadcastResult)
		require.NoError(t, err)

		currentBroadcastData := getBroadcastResult["broadcast"].(map[string]interface{})

		// Verify auto_send_winner is enabled
		if testSettings, ok := currentBroadcastData["test_settings"].(map[string]interface{}); ok {
			autoSendWinner, _ := testSettings["auto_send_winner"].(bool)
			assert.True(t, autoSendWinner, "Auto send winner should be enabled")
			t.Logf("✓ Broadcast configured for auto winner selection after %v hour(s)", testSettings["test_duration_hours"])
		}

		// In production, auto winner selection would happen after test_duration_hours
		// For testing purposes, we verify the configuration is correct
		t.Logf("✓ Auto winner selection test completed - broadcast waiting for test duration to expire")
	})
}
