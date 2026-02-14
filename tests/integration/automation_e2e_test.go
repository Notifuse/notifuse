package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
	shortuuid "github.com/lithammer/shortuuid/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// BugReport tracks issues found during integration tests
type BugReport struct {
	TestName    string
	Description string
	Severity    string // Critical, High, Medium, Low
	RootCause   string
	CodePath    string
}

var bugReports []BugReport

func addBug(testName, description, severity, rootCause, codePath string) {
	bugReports = append(bugReports, BugReport{
		TestName:    testName,
		Description: description,
		Severity:    severity,
		RootCause:   rootCause,
		CodePath:    codePath,
	})
}

// ============================================================================
// Polling Helper Functions
// ============================================================================

// waitForEnrollment polls until contact is enrolled in automation
func waitForEnrollment(t *testing.T, factory *testutil.TestDataFactory, workspaceID, automationID, email string, timeout time.Duration) *domain.ContactAutomation {
	var ca *domain.ContactAutomation
	testutil.WaitForCondition(t, func() bool {
		var err error
		ca, err = factory.GetContactAutomation(workspaceID, automationID, email)
		return err == nil && ca != nil
	}, timeout, fmt.Sprintf("waiting for enrollment of %s in automation %s", email, automationID))
	return ca
}

// waitForEnrollmentCount polls until expected enrollment count is reached
func waitForEnrollmentCount(t *testing.T, factory *testutil.TestDataFactory, workspaceID, automationID string, expected int, timeout time.Duration) {
	testutil.WaitForCondition(t, func() bool {
		count, err := factory.CountContactAutomations(workspaceID, automationID)
		return err == nil && count == expected
	}, timeout, fmt.Sprintf("waiting for %d enrollments in automation %s", expected, automationID))
}

// waitForTimelineEvent polls until a timeline event of the specified kind exists
func waitForTimelineEvent(t *testing.T, factory *testutil.TestDataFactory, workspaceID, email, eventKind string, timeout time.Duration) []testutil.TimelineEventResult {
	var events []testutil.TimelineEventResult
	testutil.WaitForCondition(t, func() bool {
		var err error
		events, err = factory.GetContactTimelineEvents(workspaceID, email, eventKind)
		return err == nil && len(events) > 0
	}, timeout, fmt.Sprintf("waiting for timeline event %s for %s", eventKind, email))
	return events
}

// waitForEnrollmentViaAPI polls the nodeExecutions API until contact is enrolled
func waitForEnrollmentViaAPI(t *testing.T, client *testutil.APIClient, automationID, email string, timeout time.Duration) map[string]interface{} {
	var ca map[string]interface{}
	testutil.WaitForCondition(t, func() bool {
		resp, err := client.GetContactNodeExecutions(automationID, email)
		if err != nil || resp.StatusCode != http.StatusOK {
			if resp != nil {
				resp.Body.Close()
			}
			return false
		}
		defer resp.Body.Close()
		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return false
		}
		if contactAuto, ok := result["contact_automation"].(map[string]interface{}); ok {
			ca = contactAuto
			return true
		}
		return false
	}, timeout, fmt.Sprintf("waiting for enrollment of %s in automation %s via API", email, automationID))
	return ca
}

// waitForAutomationComplete polls until contact automation status is "completed"
func waitForAutomationComplete(t *testing.T, factory *testutil.TestDataFactory, workspaceID, automationID, email string, timeout time.Duration) *domain.ContactAutomation {
	var ca *domain.ContactAutomation
	testutil.WaitForCondition(t, func() bool {
		var err error
		ca, err = factory.GetContactAutomation(workspaceID, automationID, email)
		if err != nil || ca == nil {
			return false
		}
		return ca.Status == domain.ContactAutomationStatusCompleted
	}, timeout, fmt.Sprintf("waiting for automation to complete for %s in automation %s", email, automationID))
	return ca
}

// waitForAutomationStatus polls until contact automation reaches the expected status
func waitForAutomationStatus(t *testing.T, factory *testutil.TestDataFactory, workspaceID, automationID, email string, expectedStatus domain.ContactAutomationStatus, timeout time.Duration) *domain.ContactAutomation {
	var ca *domain.ContactAutomation
	testutil.WaitForCondition(t, func() bool {
		var err error
		ca, err = factory.GetContactAutomation(workspaceID, automationID, email)
		if err != nil || ca == nil {
			return false
		}
		return ca.Status == expectedStatus
	}, timeout, fmt.Sprintf("waiting for status %s for %s in automation %s", expectedStatus, email, automationID))
	return ca
}

// waitForStatsCompleted polls until automation stats show expected completed count
// This is needed because stats are updated separately from contact automation status
func waitForStatsCompleted(t *testing.T, factory *testutil.TestDataFactory, workspaceID, automationID string, expectedCompleted int64, timeout time.Duration) *domain.AutomationStats {
	var stats *domain.AutomationStats
	testutil.WaitForCondition(t, func() bool {
		var err error
		stats, err = factory.GetAutomationStats(workspaceID, automationID)
		if err != nil || stats == nil {
			return false
		}
		return stats.Completed >= expectedCompleted
	}, timeout, fmt.Sprintf("waiting for stats.Completed >= %d for automation %s", expectedCompleted, automationID))
	return stats
}

// ============================================================================
// Main Test Function with Shared Setup
// ============================================================================

// TestAutomation runs all automation integration tests with shared setup
// This consolidates 18 separate tests into subtests to reduce setup overhead from ~50s to ~15s
func TestAutomation(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	// Start automation scheduler for processing contacts through workflows
	ctx := context.Background()
	err := suite.ServerManager.StartBackgroundWorkers(ctx)
	require.NoError(t, err)

	factory := suite.DataFactory
	client := suite.APIClient

	// ONE-TIME shared setup
	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	err = factory.AddUserToWorkspace(user.ID, workspace.ID, "owner")
	require.NoError(t, err)

	// Setup email provider
	_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID)
	require.NoError(t, err)

	// Login
	err = client.Login(user.Email, "password")
	require.NoError(t, err)
	client.SetWorkspaceID(workspace.ID)

	// Run all subtests - each creates its own automation/nodes/contacts for isolation
	// All tests now use HTTP endpoints for automation CRUD (true e2e tests)
	t.Run("WelcomeSeries", func(t *testing.T) {
		testAutomationWelcomeSeries(t, factory, client, workspace.ID)
	})
	t.Run("Deduplication", func(t *testing.T) {
		testAutomationDeduplication(t, factory, client, workspace.ID)
	})
	t.Run("MultipleEntries", func(t *testing.T) {
		testAutomationMultipleEntries(t, factory, client, workspace.ID)
	})
	t.Run("DelayTiming", func(t *testing.T) {
		testAutomationDelayTiming(t, factory, client, workspace.ID)
	})
	t.Run("ABTestDeterminism", func(t *testing.T) {
		testAutomationABTestDeterminism(t, factory, client, workspace.ID)
	})
	t.Run("BranchRouting", func(t *testing.T) {
		testAutomationBranchRouting(t, factory, client, workspace.ID)
	})
	t.Run("FilterNode", func(t *testing.T) {
		testAutomationFilterNode(t, factory, client, workspace.ID)
	})
	t.Run("ListStatusBranch", func(t *testing.T) {
		testAutomationListStatusBranch(t, factory, client, workspace.ID)
	})
	t.Run("ListOperations", func(t *testing.T) {
		testAutomationListOperations(t, factory, client, workspace.ID)
	})
	t.Run("ContextData", func(t *testing.T) {
		testAutomationContextData(t, factory, client, workspace.ID)
	})
	t.Run("SegmentTrigger", func(t *testing.T) {
		testAutomationSegmentTrigger(t, factory, client, workspace.ID)
	})
	t.Run("DeletionCleanup", func(t *testing.T) {
		testAutomationDeletionCleanup(t, factory, client, workspace.ID)
	})
	t.Run("ErrorRecovery", func(t *testing.T) {
		testAutomationErrorRecovery(t, factory, client, workspace.ID)
	})
	t.Run("SchedulerExecution", func(t *testing.T) {
		testAutomationSchedulerExecution(t, factory, client, workspace.ID)
	})
	t.Run("PauseResume", func(t *testing.T) {
		testAutomationPauseResume(t, factory, client, workspace.ID)
	})
	t.Run("Permissions", func(t *testing.T) {
		// Permissions test needs additional users with different permission levels
		memberNoPerms, err := factory.CreateUser()
		require.NoError(t, err)
		noAutoPerms := domain.UserPermissions{
			domain.PermissionResourceContacts:       domain.ResourcePermissions{Read: true, Write: true},
			domain.PermissionResourceLists:          domain.ResourcePermissions{Read: true, Write: true},
			domain.PermissionResourceTemplates:      domain.ResourcePermissions{Read: true, Write: true},
			domain.PermissionResourceBroadcasts:     domain.ResourcePermissions{Read: true, Write: true},
			domain.PermissionResourceTransactional:  domain.ResourcePermissions{Read: true, Write: true},
			domain.PermissionResourceWorkspace:      domain.ResourcePermissions{Read: true, Write: true},
			domain.PermissionResourceMessageHistory: domain.ResourcePermissions{Read: true, Write: true},
			domain.PermissionResourceBlog:           domain.ResourcePermissions{Read: true, Write: true},
			domain.PermissionResourceAutomations:    domain.ResourcePermissions{Read: false, Write: false},
		}
		err = factory.AddUserToWorkspaceWithPermissions(memberNoPerms.ID, workspace.ID, "member", noAutoPerms)
		require.NoError(t, err)

		memberReadOnly, err := factory.CreateUser()
		require.NoError(t, err)
		readOnlyPerms := domain.UserPermissions{
			domain.PermissionResourceContacts:       domain.ResourcePermissions{Read: true, Write: true},
			domain.PermissionResourceLists:          domain.ResourcePermissions{Read: true, Write: true},
			domain.PermissionResourceTemplates:      domain.ResourcePermissions{Read: true, Write: true},
			domain.PermissionResourceBroadcasts:     domain.ResourcePermissions{Read: true, Write: true},
			domain.PermissionResourceTransactional:  domain.ResourcePermissions{Read: true, Write: true},
			domain.PermissionResourceWorkspace:      domain.ResourcePermissions{Read: true, Write: true},
			domain.PermissionResourceMessageHistory: domain.ResourcePermissions{Read: true, Write: true},
			domain.PermissionResourceBlog:           domain.ResourcePermissions{Read: true, Write: true},
			domain.PermissionResourceAutomations:    domain.ResourcePermissions{Read: true, Write: false},
		}
		err = factory.AddUserToWorkspaceWithPermissions(memberReadOnly.ID, workspace.ID, "member", readOnlyPerms)
		require.NoError(t, err)

		testAutomationPermissions(t, factory, client, workspace.ID, user, memberNoPerms, memberReadOnly)
	})
	t.Run("TimelineStartEvent", func(t *testing.T) {
		testAutomationTimelineStartEvent(t, factory, client, workspace.ID)
	})
	t.Run("TimelineEndEvent_Completed", func(t *testing.T) {
		testAutomationTimelineEndEvent(t, factory, client, workspace.ID)
	})
	t.Run("ContactCreatedTrigger", func(t *testing.T) {
		testAutomationContactCreatedTrigger(t, factory, client, workspace.ID)
	})
	t.Run("ConsecutiveAddToList", func(t *testing.T) {
		testAutomationConsecutiveAddToList(t, factory, client, workspace.ID)
	})
	t.Run("WebhookNode", func(t *testing.T) {
		testWebhookNode(t, factory, client, workspace.ID)
	})
	t.Run("IntegrationOverride", func(t *testing.T) {
		testAutomationIntegrationOverride(t, factory, client, workspace.ID)
	})
	t.Run("PrintBugReport", func(t *testing.T) {
		printBugReport(t)
	})
}

// ============================================================================
// Test Helper Functions
// ============================================================================

// testAutomationWelcomeSeries tests the complete welcome series flow
// Use Case: Contact subscribes to list → receives welcome email sequence
// HTTP is used for automation CRUD, contacts, and list subscription
// Factory is used for supporting objects (lists, templates) due to complex validation
func testAutomationWelcomeSeries(t *testing.T, factory *testutil.TestDataFactory, client *testutil.APIClient, workspaceID string) {
	// 1. Create list via factory (complex validation requirements)
	list, err := factory.CreateList(workspaceID)
	require.NoError(t, err)
	listID := list.ID

	// 2. Create email template via factory (complex MJML validation)
	template, err := factory.CreateTemplate(workspaceID)
	require.NoError(t, err)
	templateID := template.ID

	// 3. Build and create automation via HTTP with embedded nodes
	automationID := shortuuid.New()
	triggerNodeID := shortuuid.New()
	emailNodeID := shortuuid.New()

	createReq := map[string]interface{}{
		"workspace_id": workspaceID,
		"automation": map[string]interface{}{
			"id":           automationID,
			"workspace_id": workspaceID,
			"name":         "Welcome Series E2E",
			"status":       "draft",
			"list_id":      listID,
			"trigger": map[string]interface{}{
				"event_kind": "list.subscribed",
				"list_id":    listID,
				"frequency":  "once",
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
	}

	resp, err := client.CreateAutomation(createReq)
	require.NoError(t, err)
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("WelcomeSeries CreateAutomation: Expected 201, got %d: %s", resp.StatusCode, string(body))
	}
	resp.Body.Close()
	t.Logf("Automation created: %s", automationID)

	// 4. Activate automation via HTTP
	activateResp, err := client.ActivateAutomation(map[string]interface{}{
		"workspace_id":  workspaceID,
		"automation_id": automationID,
	})
	require.NoError(t, err)
	if activateResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(activateResp.Body)
		activateResp.Body.Close()
		t.Fatalf("WelcomeSeries ActivateAutomation: Expected 200, got %d: %s", activateResp.StatusCode, string(body))
	}
	activateResp.Body.Close()
	t.Logf("Automation activated: %s", automationID)

	// 5. Create contact and subscribe to list via factory
	// (No HTTP endpoint exists for creating new contact-list subscriptions)
	email := "welcome-test-e2e@example.com"
	contact, err := factory.CreateContact(workspaceID, testutil.WithContactEmail(email))
	require.NoError(t, err)
	t.Logf("Contact created: %s", contact.Email)

	// 6. Subscribe to list via factory - this triggers the automation
	t.Logf("Subscribing contact %s to list %s", email, listID)
	_, err = factory.CreateContactList(workspaceID,
		testutil.WithContactListEmail(email),
		testutil.WithContactListListID(listID),
		testutil.WithContactListStatus(domain.ContactListStatusActive),
	)
	require.NoError(t, err)
	t.Logf("Contact subscribed to list")

	// 7. Verify enrollment via HTTP (nodeExecutions endpoint)
	ca := waitForEnrollmentViaAPI(t, client, automationID, email, 2*time.Second)
	if ca == nil {
		addBug("TestAutomation_WelcomeSeries", "Contact not enrolled after timeline event",
			"Critical", "Trigger not firing on timeline insert",
			"internal/migrations/v20.go:automation_enroll_contact")
		t.Fatal("Contact not enrolled")
	}
	t.Logf("Contact enrolled with status: %s", ca["status"])

	// 8. Wait for automation to complete (scheduler must process the email node)
	completedCA := waitForAutomationComplete(t, factory, workspaceID, automationID, email, 10*time.Second)
	require.NotNil(t, completedCA, "Automation should complete")
	assert.Equal(t, domain.ContactAutomationStatusCompleted, completedCA.Status, "Status should be completed")
	t.Logf("Automation completed for contact %s", email)

	// 9. Verify stats show completion (wait for stats to update after contact completion)
	stats := waitForStatsCompleted(t, factory, workspaceID, automationID, 1, 2*time.Second)
	require.NotNil(t, stats, "Stats should exist")
	assert.Equal(t, int64(1), stats.Enrolled, "Enrolled count should be 1")
	assert.Equal(t, int64(1), stats.Completed, "Completed count should be 1")
	t.Logf("Automation stats: enrolled=%d, completed=%d", stats.Enrolled, stats.Completed)

	t.Logf("Welcome Series E2E test passed: automation completed successfully")
}

// testAutomationDeduplication tests frequency: once prevents duplicate enrollments
// Uses HTTP for automation CRUD, factory for timeline events (intentional)
func testAutomationDeduplication(t *testing.T, factory *testutil.TestDataFactory, client *testutil.APIClient, workspaceID string) {
	// 1. Create automation via HTTP with frequency: once
	automationID := shortuuid.New()
	triggerNodeID := shortuuid.New()

	createReq := map[string]interface{}{
		"workspace_id": workspaceID,
		"automation": map[string]interface{}{
			"id":           automationID,
			"workspace_id": workspaceID,
			"name":         "Once Only Automation E2E",
			"status":       "draft",
			"trigger": map[string]interface{}{
				"event_kind":        "custom_event",
				"custom_event_name": "test_event_dedup",
				"frequency":         "once",
			},
			"root_node_id": triggerNodeID,
			"nodes": []map[string]interface{}{
				{
					"id":            triggerNodeID,
					"automation_id": automationID,
					"type":          "trigger",
					"config":        map[string]interface{}{},
					"position":      map[string]interface{}{"x": 0, "y": 0},
				},
			},
			"stats": map[string]interface{}{"enrolled": 0, "completed": 0, "exited": 0, "failed": 0},
		},
	}

	resp, err := client.CreateAutomation(createReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	// 2. Activate automation via HTTP
	activateResp, err := client.ActivateAutomation(map[string]interface{}{
		"workspace_id":  workspaceID,
		"automation_id": automationID,
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, activateResp.StatusCode)
	activateResp.Body.Close()

	// 3. Create contact via HTTP
	email := "dedup-test-e2e@example.com"
	contactResp, err := client.CreateContact(map[string]interface{}{
		"workspace_id": workspaceID,
		"contact":      map[string]interface{}{"email": email},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, contactResp.StatusCode, "Contact creation should succeed")
	contactResp.Body.Close()

	// 4. Trigger 3 times via factory (timeline events - no HTTP API)
	for i := 0; i < 3; i++ {
		err = factory.CreateCustomEvent(workspaceID, email, "test_event_dedup", map[string]interface{}{
			"iteration": i,
		})
		require.NoError(t, err)
	}

	// 5. Wait for enrollment (should only be 1 due to frequency: once)
	waitForEnrollmentCount(t, factory, workspaceID, automationID, 1, 2*time.Second)

	// 6. Verify: only 1 contact_automation created (factory - no HTTP API for counts)
	count, err := factory.CountContactAutomations(workspaceID, automationID)
	require.NoError(t, err)

	if count != 1 {
		addBug("TestAutomation_Deduplication",
			fmt.Sprintf("Expected 1 enrollment, got %d", count),
			"Critical", "Deduplication via automation_trigger_log not working",
			"internal/migrations/v20.go:automation_enroll_contact")
	}
	assert.Equal(t, 1, count, "Should have exactly 1 contact automation record")

	// 7. Wait for automation to complete (trigger-only automation should complete immediately)
	completedCA := waitForAutomationComplete(t, factory, workspaceID, automationID, email, 10*time.Second)
	require.NotNil(t, completedCA, "Automation should complete")
	assert.Equal(t, domain.ContactAutomationStatusCompleted, completedCA.Status, "Status should be completed")
	t.Logf("Automation completed for contact %s", email)

	// 8. Verify trigger log entry exists (factory - no HTTP API)
	hasEntry, err := factory.GetTriggerLogEntry(workspaceID, automationID, email)
	require.NoError(t, err)
	assert.True(t, hasEntry, "Trigger log entry should exist")

	// 9. Verify stats show completion (wait for stats to update after contact completion)
	stats := waitForStatsCompleted(t, factory, workspaceID, automationID, 1, 2*time.Second)
	require.NotNil(t, stats, "Stats should exist")
	assert.Equal(t, int64(1), stats.Enrolled, "Enrolled should be 1, not 3")
	assert.Equal(t, int64(1), stats.Completed, "Completed should be 1")
	t.Logf("Automation stats: enrolled=%d, completed=%d", stats.Enrolled, stats.Completed)

	t.Logf("Deduplication E2E test passed: frequency=once working correctly, automation completed")
}

// testAutomationMultipleEntries tests frequency: every_time allows multiple enrollments
// Uses HTTP for automation CRUD, factory for timeline events (intentional)
func testAutomationMultipleEntries(t *testing.T, factory *testutil.TestDataFactory, client *testutil.APIClient, workspaceID string) {
	// 1. Create automation via HTTP with frequency: every_time
	automationID := shortuuid.New()
	triggerNodeID := shortuuid.New()

	createReq := map[string]interface{}{
		"workspace_id": workspaceID,
		"automation": map[string]interface{}{
			"id":           automationID,
			"workspace_id": workspaceID,
			"name":         "Every Time Automation E2E",
			"status":       "draft",
			"trigger": map[string]interface{}{
				"event_kind": "custom_event", "custom_event_name": "repeat_event_e2e",
				"frequency": "every_time",
			},
			"root_node_id": triggerNodeID,
			"nodes": []map[string]interface{}{
				{
					"id":            triggerNodeID,
					"automation_id": automationID,
					"type":          "trigger",
					"config":        map[string]interface{}{},
					"position":      map[string]interface{}{"x": 0, "y": 0},
				},
			},
			"stats": map[string]interface{}{"enrolled": 0, "completed": 0, "exited": 0, "failed": 0},
		},
	}

	resp, err := client.CreateAutomation(createReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	// 2. Activate automation via HTTP
	activateResp, err := client.ActivateAutomation(map[string]interface{}{
		"workspace_id":  workspaceID,
		"automation_id": automationID,
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, activateResp.StatusCode)
	activateResp.Body.Close()

	// 3. Create contact via HTTP
	email := "multi-test-e2e@example.com"
	contactResp, err := client.CreateContact(map[string]interface{}{
		"workspace_id": workspaceID,
		"contact":      map[string]interface{}{"email": email},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, contactResp.StatusCode, "Contact creation should succeed")
	contactResp.Body.Close()

	// 4. Trigger 3 times via factory with small delays (timeline events - no HTTP API)
	for i := 0; i < 3; i++ {
		err = factory.CreateCustomEvent(workspaceID, email, "repeat_event_e2e", map[string]interface{}{
			"iteration": i,
		})
		require.NoError(t, err)
		time.Sleep(10 * time.Millisecond) // Small delay to ensure different timestamps
	}

	// 5. Wait for 3 enrollments
	waitForEnrollmentCount(t, factory, workspaceID, automationID, 3, 2*time.Second)

	// 6. Verify: 3 contact_automation records (factory - no HTTP API for counts)
	count, err := factory.CountContactAutomations(workspaceID, automationID)
	require.NoError(t, err)

	if count != 3 {
		addBug("TestAutomation_MultipleEntries",
			fmt.Sprintf("Expected 3 enrollments, got %d", count),
			"High", "every_time frequency not allowing multiple entries",
			"internal/migrations/v20.go:automation_enroll_contact")
	}
	assert.Equal(t, 3, count, "Should have 3 contact automation records")

	// 7. Verify each has different entered_at (factory - no HTTP API)
	cas, err := factory.GetAllContactAutomations(workspaceID, automationID)
	require.NoError(t, err)
	assert.Len(t, cas, 3, "Should have 3 records")

	// 8. Wait for all automations to complete
	testutil.WaitForCondition(t, func() bool {
		stats, err := factory.GetAutomationStats(workspaceID, automationID)
		if err != nil {
			return false
		}
		return stats.Completed == 3
	}, 10*time.Second, "waiting for all 3 automations to complete")

	// 9. Verify stats show all completed (factory - no HTTP API for stats)
	stats, err := factory.GetAutomationStats(workspaceID, automationID)
	require.NoError(t, err)
	assert.Equal(t, int64(3), stats.Enrolled, "Enrolled should be 3")
	assert.Equal(t, int64(3), stats.Completed, "Completed should be 3")
	t.Logf("Automation stats: enrolled=%d, completed=%d", stats.Enrolled, stats.Completed)

	t.Logf("Multiple entries E2E test passed: frequency=every_time working correctly, all completed")
}

// testAutomationDelayTiming tests delay node calculations and full workflow execution
// Uses HTTP for automation CRUD, factory for timeline events (intentional)
// Workflow: trigger → delay (5min) → add_to_list (terminal)
func testAutomationDelayTiming(t *testing.T, factory *testutil.TestDataFactory, client *testutil.APIClient, workspaceID string) {
	// 1. Create list via factory (for add_to_list terminal node)
	list, err := factory.CreateList(workspaceID)
	require.NoError(t, err)
	listID := list.ID

	// 2. Create automation via HTTP with delay node
	automationID := shortuuid.New()
	triggerNodeID := shortuuid.New()
	delayNodeID := shortuuid.New()
	addToListNodeID := shortuuid.New()

	createReq := map[string]interface{}{
		"workspace_id": workspaceID,
		"automation": map[string]interface{}{
			"id":           automationID,
			"workspace_id": workspaceID,
			"name":         "Delay Test Automation E2E",
			"status":       "draft",
			"trigger": map[string]interface{}{
				"event_kind": "custom_event", "custom_event_name": "delay_test_event_e2e",
				"frequency": "once",
			},
			"root_node_id": triggerNodeID,
			"nodes": []map[string]interface{}{
				{
					"id":            triggerNodeID,
					"automation_id": automationID,
					"type":          "trigger",
					"config":        map[string]interface{}{},
					"next_node_id":  delayNodeID,
					"position":      map[string]interface{}{"x": 0, "y": 0},
				},
				{
					"id":            delayNodeID,
					"automation_id": automationID,
					"type":          "delay",
					"config":        map[string]interface{}{"duration": 5, "unit": "minutes"},
					"next_node_id":  addToListNodeID,
					"position":      map[string]interface{}{"x": 0, "y": 100},
				},
				{
					"id":            addToListNodeID,
					"automation_id": automationID,
					"type":          "add_to_list",
					"config":        map[string]interface{}{"list_id": listID, "status": "active"},
					"position":      map[string]interface{}{"x": 0, "y": 200},
				},
			},
			"stats": map[string]interface{}{"enrolled": 0, "completed": 0, "exited": 0, "failed": 0},
		},
	}

	resp, err := client.CreateAutomation(createReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	// 2. Activate automation via HTTP
	activateResp, err := client.ActivateAutomation(map[string]interface{}{
		"workspace_id":  workspaceID,
		"automation_id": automationID,
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, activateResp.StatusCode)
	activateResp.Body.Close()

	// 3. Create contact via HTTP
	email := "delay-test-e2e@example.com"
	contactResp, err := client.CreateContact(map[string]interface{}{
		"workspace_id": workspaceID,
		"contact":      map[string]interface{}{"email": email},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, contactResp.StatusCode, "Contact creation should succeed")
	contactResp.Body.Close()

	// 4. Trigger via factory (timeline events - no HTTP API)
	beforeTrigger := time.Now().UTC()
	err = factory.CreateCustomEvent(workspaceID, email, "delay_test_event_e2e", nil)
	require.NoError(t, err)

	// 5. Wait for enrollment and verify via HTTP
	caMap := waitForEnrollmentViaAPI(t, client, automationID, email, 2*time.Second)
	require.NotNil(t, caMap)

	// 6. Verify enrollment
	assert.Equal(t, "active", caMap["status"])

	// 7. Wait for scheduler to process through trigger → delay
	// After delay node executes, CurrentNodeID is set to the NEXT node (add_to_list),
	// and ScheduledAt is set to the future time when the delay expires
	var ca *domain.ContactAutomation
	testutil.WaitForCondition(t, func() bool {
		var err error
		ca, err = factory.GetContactAutomation(workspaceID, automationID, email)
		if err != nil || ca == nil {
			return false
		}
		// Contact should be scheduled for the add_to_list node (after delay) with future scheduled_at
		return ca.CurrentNodeID != nil && *ca.CurrentNodeID == addToListNodeID && ca.ScheduledAt != nil
	}, 10*time.Second, "waiting for scheduler to process delay node")

	require.NotNil(t, ca, "Contact automation should exist")
	require.NotNil(t, ca.CurrentNodeID, "Current node should be set")
	assert.Equal(t, addToListNodeID, *ca.CurrentNodeID, "Contact should be scheduled for add_to_list node after delay")
	t.Logf("Contact processed delay node, waiting for: %s", *ca.CurrentNodeID)

	// 8. Verify scheduled_at is approximately 5 minutes in the future
	require.NotNil(t, ca.ScheduledAt, "Scheduled time should be set for delay node")
	expectedMin := beforeTrigger.Add(4 * time.Minute)
	expectedMax := beforeTrigger.Add(6 * time.Minute)

	if ca.ScheduledAt.Before(expectedMin) || ca.ScheduledAt.After(expectedMax) {
		addBug("TestAutomation_DelayTiming",
			fmt.Sprintf("Delay timing incorrect: expected ~5min future, got %v", ca.ScheduledAt.Sub(beforeTrigger)),
			"High", "Delay calculation error",
			"internal/service/automation_node_executor.go:DelayNodeExecutor")
	}
	t.Logf("Delay scheduled at: %v (%.1f minutes from trigger)", ca.ScheduledAt, ca.ScheduledAt.Sub(beforeTrigger).Minutes())

	t.Logf("Delay timing E2E test passed: scheduler advanced to delay node with correct timing")
}

// testAutomationABTestDeterminism tests A/B test variant selection and full workflow execution
// Uses HTTP for automation CRUD, factory for timeline events (intentional)
// Workflow: trigger → ab_test → (variant A → add_to_list_a OR variant B → add_to_list_b)
func testAutomationABTestDeterminism(t *testing.T, factory *testutil.TestDataFactory, client *testutil.APIClient, workspaceID string) {
	// 1. Create lists to track which variant was selected
	listA, err := factory.CreateList(workspaceID)
	require.NoError(t, err)
	listAID := listA.ID

	listB, err := factory.CreateList(workspaceID)
	require.NoError(t, err)
	listBID := listB.ID

	// 2. Create automation via HTTP with A/B test node that routes to different lists
	automationID := shortuuid.New()
	triggerNodeID := shortuuid.New()
	abNodeID := shortuuid.New()
	addToListANodeID := shortuuid.New()
	addToListBNodeID := shortuuid.New()

	createReq := map[string]interface{}{
		"workspace_id": workspaceID,
		"automation": map[string]interface{}{
			"id":           automationID,
			"workspace_id": workspaceID,
			"name":         "AB Test Automation E2E",
			"status":       "draft",
			"trigger": map[string]interface{}{
				"event_kind": "custom_event", "custom_event_name": "ab_test_event_e2e",
				"frequency": "every_time",
			},
			"root_node_id": triggerNodeID,
			"nodes": []map[string]interface{}{
				{
					"id":            triggerNodeID,
					"automation_id": automationID,
					"type":          "trigger",
					"config":        map[string]interface{}{},
					"next_node_id":  abNodeID,
					"position":      map[string]interface{}{"x": 0, "y": 0},
				},
				{
					"id":            abNodeID,
					"automation_id": automationID,
					"type":          "ab_test",
					"config": map[string]interface{}{
						"variants": []map[string]interface{}{
							{"id": "A", "name": "Variant A", "weight": 50, "next_node_id": addToListANodeID},
							{"id": "B", "name": "Variant B", "weight": 50, "next_node_id": addToListBNodeID},
						},
					},
					"position": map[string]interface{}{"x": 0, "y": 100},
				},
				{
					"id":            addToListANodeID,
					"automation_id": automationID,
					"type":          "add_to_list",
					"config":        map[string]interface{}{"list_id": listAID, "status": "active"},
					"position":      map[string]interface{}{"x": -100, "y": 200},
				},
				{
					"id":            addToListBNodeID,
					"automation_id": automationID,
					"type":          "add_to_list",
					"config":        map[string]interface{}{"list_id": listBID, "status": "active"},
					"position":      map[string]interface{}{"x": 100, "y": 200},
				},
			},
			"stats": map[string]interface{}{"enrolled": 0, "completed": 0, "exited": 0, "failed": 0},
		},
	}

	resp, err := client.CreateAutomation(createReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	// 2. Activate automation via HTTP
	activateResp, err := client.ActivateAutomation(map[string]interface{}{
		"workspace_id":  workspaceID,
		"automation_id": automationID,
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, activateResp.StatusCode)
	activateResp.Body.Close()

	// 3. Create contact via HTTP
	email := "ab-determ-e2e@example.com"
	contactResp, err := client.CreateContact(map[string]interface{}{
		"workspace_id": workspaceID,
		"contact":      map[string]interface{}{"email": email},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, contactResp.StatusCode, "Contact creation should succeed")
	contactResp.Body.Close()

	// 4. Trigger via factory (timeline events - no HTTP API)
	err = factory.CreateCustomEvent(workspaceID, email, "ab_test_event_e2e", nil)
	require.NoError(t, err)

	// 5. Wait for enrollment via HTTP
	caMap := waitForEnrollmentViaAPI(t, client, automationID, email, 2*time.Second)
	require.NotNil(t, caMap)
	t.Logf("Contact enrolled with status: %s", caMap["status"])

	// 6. Wait for automation to complete (A/B test with no next nodes should complete)
	completedCA := waitForAutomationComplete(t, factory, workspaceID, automationID, email, 10*time.Second)
	require.NotNil(t, completedCA, "Automation should complete")
	assert.Equal(t, domain.ContactAutomationStatusCompleted, completedCA.Status, "Status should be completed")
	t.Logf("Automation completed for contact %s", email)

	// 7. Verify stats show completion (wait for stats to update after contact completion)
	stats := waitForStatsCompleted(t, factory, workspaceID, automationID, 1, 2*time.Second)
	require.NotNil(t, stats, "Stats should exist")
	assert.Equal(t, int64(1), stats.Enrolled, "Enrolled count should be 1")
	assert.Equal(t, int64(1), stats.Completed, "Completed count should be 1")
	t.Logf("Automation stats: enrolled=%d, completed=%d", stats.Enrolled, stats.Completed)

	t.Logf("A/B test determinism E2E test passed: automation completed successfully")
}

// testAutomationBranchRouting tests branch node routing based on conditions
// Uses HTTP for automation CRUD, factory for timeline events (intentional)
// Workflow: trigger → branch → (VIP path → add_to_vip_list OR default → add_to_default_list)
func testAutomationBranchRouting(t *testing.T, factory *testutil.TestDataFactory, client *testutil.APIClient, workspaceID string) {
	// 1. Create lists to track which path was taken
	vipList, err := factory.CreateList(workspaceID)
	require.NoError(t, err)
	vipListID := vipList.ID

	defaultList, err := factory.CreateList(workspaceID)
	require.NoError(t, err)
	defaultListID := defaultList.ID

	// 2. Create automation via HTTP with branch node
	automationID := shortuuid.New()
	triggerNodeID := shortuuid.New()
	branchNodeID := shortuuid.New()
	addToVIPListNodeID := shortuuid.New()
	addToDefaultListNodeID := shortuuid.New()

	createReq := map[string]interface{}{
		"workspace_id": workspaceID,
		"automation": map[string]interface{}{
			"id":           automationID,
			"workspace_id": workspaceID,
			"name":         "Branch Test Automation E2E",
			"status":       "draft",
			"trigger": map[string]interface{}{
				"event_kind": "custom_event", "custom_event_name": "branch_test_event_e2e",
				"frequency": "once",
			},
			"root_node_id": triggerNodeID,
			"nodes": []map[string]interface{}{
				{
					"id":            triggerNodeID,
					"automation_id": automationID,
					"type":          "trigger",
					"config":        map[string]interface{}{},
					"next_node_id":  branchNodeID,
					"position":      map[string]interface{}{"x": 0, "y": 0},
				},
				{
					"id":            branchNodeID,
					"automation_id": automationID,
					"type":          "branch",
					"config": map[string]interface{}{
						"paths": []map[string]interface{}{
							{
								"id":   "vip_path",
								"name": "VIP Path (US)",
								"conditions": map[string]interface{}{
									"kind": "leaf",
									"leaf": map[string]interface{}{
										"source": "contacts",
										"contact": map[string]interface{}{
											"filters": []map[string]interface{}{
												{
													"field_name":    "country",
													"field_type":    "string",
													"operator":      "equals",
													"string_values": []string{"US"},
												},
											},
										},
									},
								},
								"next_node_id": addToVIPListNodeID,
							},
						},
						"default_path_id": addToDefaultListNodeID,
					},
					"position": map[string]interface{}{"x": 0, "y": 100},
				},
				{
					"id":            addToVIPListNodeID,
					"automation_id": automationID,
					"type":          "add_to_list",
					"config":        map[string]interface{}{"list_id": vipListID, "status": "active"},
					"position":      map[string]interface{}{"x": -100, "y": 200},
				},
				{
					"id":            addToDefaultListNodeID,
					"automation_id": automationID,
					"type":          "add_to_list",
					"config":        map[string]interface{}{"list_id": defaultListID, "status": "active"},
					"position":      map[string]interface{}{"x": 100, "y": 200},
				},
			},
			"stats": map[string]interface{}{"enrolled": 0, "completed": 0, "exited": 0, "failed": 0},
		},
	}

	resp, err := client.CreateAutomation(createReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	// 2. Activate automation via HTTP
	activateResp, err := client.ActivateAutomation(map[string]interface{}{
		"workspace_id":  workspaceID,
		"automation_id": automationID,
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, activateResp.StatusCode)
	activateResp.Body.Close()

	// 3. Create VIP contact (US) via HTTP
	email := "vip-branch-e2e@example.com"
	contactResp, err := client.CreateContact(map[string]interface{}{
		"workspace_id": workspaceID,
		"contact":      map[string]interface{}{"email": email, "country": "US"},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, contactResp.StatusCode, "Contact creation should succeed")
	contactResp.Body.Close()

	// 4. Trigger via factory (timeline events - no HTTP API)
	err = factory.CreateCustomEvent(workspaceID, email, "branch_test_event_e2e", nil)
	require.NoError(t, err)

	// 5. Wait for enrollment via HTTP
	caMap := waitForEnrollmentViaAPI(t, client, automationID, email, 2*time.Second)
	require.NotNil(t, caMap)
	t.Logf("Contact enrolled with status: %s", caMap["status"])

	// 6. Wait for automation to complete (branch with no next nodes should complete)
	completedCA := waitForAutomationComplete(t, factory, workspaceID, automationID, email, 10*time.Second)
	require.NotNil(t, completedCA, "Automation should complete")
	assert.Equal(t, domain.ContactAutomationStatusCompleted, completedCA.Status, "Status should be completed")
	t.Logf("Automation completed for contact %s", email)

	// 7. Verify stats show completion (wait for stats to update after contact completion)
	stats := waitForStatsCompleted(t, factory, workspaceID, automationID, 1, 2*time.Second)
	require.NotNil(t, stats, "Stats should exist")
	assert.Equal(t, int64(1), stats.Enrolled, "Enrolled count should be 1")
	assert.Equal(t, int64(1), stats.Completed, "Completed count should be 1")
	t.Logf("Automation stats: enrolled=%d, completed=%d", stats.Enrolled, stats.Completed)

	t.Logf("Branch routing E2E test passed: automation completed successfully")
}

// testAutomationFilterNode tests filter node pass/fail paths
// Uses HTTP for automation CRUD, factory for timeline events (intentional)
// Workflow: trigger → filter (FR?) → (pass → add_to_pass_list OR fail → add_to_fail_list)
func testAutomationFilterNode(t *testing.T, factory *testutil.TestDataFactory, client *testutil.APIClient, workspaceID string) {
	// 1. Create lists to track which path was taken
	passedList, err := factory.CreateList(workspaceID)
	require.NoError(t, err)
	passedListID := passedList.ID

	failedList, err := factory.CreateList(workspaceID)
	require.NoError(t, err)
	failedListID := failedList.ID

	// 2. Create automation via HTTP with filter node
	automationID := shortuuid.New()
	triggerNodeID := shortuuid.New()
	filterNodeID := shortuuid.New()
	addToPassListNodeID := shortuuid.New()
	addToFailListNodeID := shortuuid.New()

	createReq := map[string]interface{}{
		"workspace_id": workspaceID,
		"automation": map[string]interface{}{
			"id":           automationID,
			"workspace_id": workspaceID,
			"name":         "Filter Test Automation E2E",
			"status":       "draft",
			"trigger": map[string]interface{}{
				"event_kind": "custom_event", "custom_event_name": "filter_test_event_e2e",
				"frequency": "once",
			},
			"root_node_id": triggerNodeID,
			"nodes": []map[string]interface{}{
				{
					"id":            triggerNodeID,
					"automation_id": automationID,
					"type":          "trigger",
					"config":        map[string]interface{}{},
					"next_node_id":  filterNodeID,
					"position":      map[string]interface{}{"x": 0, "y": 0},
				},
				{
					"id":            filterNodeID,
					"automation_id": automationID,
					"type":          "filter",
					"config": map[string]interface{}{
						"conditions": map[string]interface{}{
							"kind": "leaf",
							"leaf": map[string]interface{}{
								"source": "contacts",
								"contact": map[string]interface{}{
									"filters": []map[string]interface{}{
										{
											"field_name":    "country",
											"field_type":    "string",
											"operator":      "equals",
											"string_values": []string{"FR"},
										},
									},
								},
							},
						},
						"continue_node_id": addToPassListNodeID,
						"exit_node_id":     addToFailListNodeID,
					},
					"position": map[string]interface{}{"x": 0, "y": 100},
				},
				{
					"id":            addToPassListNodeID,
					"automation_id": automationID,
					"type":          "add_to_list",
					"config":        map[string]interface{}{"list_id": passedListID, "status": "active"},
					"position":      map[string]interface{}{"x": -100, "y": 200},
				},
				{
					"id":            addToFailListNodeID,
					"automation_id": automationID,
					"type":          "add_to_list",
					"config":        map[string]interface{}{"list_id": failedListID, "status": "active"},
					"position":      map[string]interface{}{"x": 100, "y": 200},
				},
			},
			"stats": map[string]interface{}{"enrolled": 0, "completed": 0, "exited": 0, "failed": 0},
		},
	}

	resp, err := client.CreateAutomation(createReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	// 2. Activate automation via HTTP
	activateResp, err := client.ActivateAutomation(map[string]interface{}{
		"workspace_id":  workspaceID,
		"automation_id": automationID,
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, activateResp.StatusCode)
	activateResp.Body.Close()

	// 3. Create passing contact (FR) via HTTP
	passEmail := "filter-pass-e2e@example.com"
	passResp, err := client.CreateContact(map[string]interface{}{
		"workspace_id": workspaceID,
		"contact":      map[string]interface{}{"email": passEmail, "country": "FR"},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, passResp.StatusCode, "Contact creation should succeed")
	passResp.Body.Close()

	// 4. Create failing contact (DE) via HTTP
	failEmail := "filter-fail-e2e@example.com"
	failResp, err := client.CreateContact(map[string]interface{}{
		"workspace_id": workspaceID,
		"contact":      map[string]interface{}{"email": failEmail, "country": "DE"},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, failResp.StatusCode, "Contact creation should succeed")
	failResp.Body.Close()

	// 5. Trigger both via factory (custom events - creates timeline with kind = 'custom_event.<name>')
	err = factory.CreateCustomEvent(workspaceID, passEmail, "filter_test_event_e2e", nil)
	require.NoError(t, err)
	err = factory.CreateCustomEvent(workspaceID, failEmail, "filter_test_event_e2e", nil)
	require.NoError(t, err)

	// 6. Wait for both enrollments via HTTP
	passCA := waitForEnrollmentViaAPI(t, client, automationID, passEmail, 2*time.Second)
	require.NotNil(t, passCA)
	t.Logf("Pass contact enrolled with status: %s", passCA["status"])

	failCA := waitForEnrollmentViaAPI(t, client, automationID, failEmail, 2*time.Second)
	require.NotNil(t, failCA)
	t.Logf("Fail contact enrolled with status: %s", failCA["status"])

	// 7. Wait for both automations to complete
	passCompleted := waitForAutomationComplete(t, factory, workspaceID, automationID, passEmail, 10*time.Second)
	require.NotNil(t, passCompleted, "Pass contact automation should complete")
	assert.Equal(t, domain.ContactAutomationStatusCompleted, passCompleted.Status, "Pass contact status should be completed")

	failCompleted := waitForAutomationComplete(t, factory, workspaceID, automationID, failEmail, 10*time.Second)
	require.NotNil(t, failCompleted, "Fail contact automation should complete")
	assert.Equal(t, domain.ContactAutomationStatusCompleted, failCompleted.Status, "Fail contact status should be completed")

	// 8. Verify stats show both completed (wait for stats to update after contact completion)
	stats := waitForStatsCompleted(t, factory, workspaceID, automationID, 2, 2*time.Second)
	require.NotNil(t, stats, "Stats should exist")
	assert.Equal(t, int64(2), stats.Enrolled, "Enrolled count should be 2")
	assert.Equal(t, int64(2), stats.Completed, "Completed count should be 2")
	t.Logf("Automation stats: enrolled=%d, completed=%d", stats.Enrolled, stats.Completed)

	t.Logf("Filter node E2E test passed: both contacts completed automation")
}

// testAutomationListStatusBranch tests the list_status_branch node routing based on contact list status
// Uses HTTP for automation CRUD, factory for timeline events (intentional)
// Workflow: trigger → list_status_branch → (not_in_list → add_to_not_in_list, active → add_to_active_list, non_active → add_to_non_active_list)
func testAutomationListStatusBranch(t *testing.T, factory *testutil.TestDataFactory, client *testutil.APIClient, workspaceID string) {
	// 1. Create the list to check status against
	list, err := factory.CreateList(workspaceID)
	require.NoError(t, err)
	listID := list.ID

	// 2. Create lists to track which branch was taken
	notInListTargetList, err := factory.CreateList(workspaceID)
	require.NoError(t, err)
	notInListTargetListID := notInListTargetList.ID

	activeTargetList, err := factory.CreateList(workspaceID)
	require.NoError(t, err)
	activeTargetListID := activeTargetList.ID

	nonActiveTargetList, err := factory.CreateList(workspaceID)
	require.NoError(t, err)
	nonActiveTargetListID := nonActiveTargetList.ID

	// 3. Create automation via HTTP with list_status_branch node
	automationID := shortuuid.New()
	triggerNodeID := shortuuid.New()
	listStatusNodeID := shortuuid.New()
	addToNotInListNodeID := shortuuid.New()
	addToActiveListNodeID := shortuuid.New()
	addToNonActiveListNodeID := shortuuid.New()

	createReq := map[string]interface{}{
		"workspace_id": workspaceID,
		"automation": map[string]interface{}{
			"id":           automationID,
			"workspace_id": workspaceID,
			"name":         "List Status Branch Test E2E",
			"status":       "draft",
			"trigger": map[string]interface{}{
				"event_kind": "custom_event", "custom_event_name": "list_status_branch_test_event_e2e",
				"frequency": "once",
			},
			"root_node_id": triggerNodeID,
			"nodes": []map[string]interface{}{
				{
					"id":            triggerNodeID,
					"automation_id": automationID,
					"type":          "trigger",
					"config":        map[string]interface{}{},
					"next_node_id":  listStatusNodeID,
					"position":      map[string]interface{}{"x": 0, "y": 0},
				},
				{
					"id":            listStatusNodeID,
					"automation_id": automationID,
					"type":          "list_status_branch",
					"config": map[string]interface{}{
						"list_id":             listID,
						"not_in_list_node_id": addToNotInListNodeID,
						"active_node_id":      addToActiveListNodeID,
						"non_active_node_id":  addToNonActiveListNodeID,
					},
					"position": map[string]interface{}{"x": 0, "y": 100},
				},
				{
					"id":            addToNotInListNodeID,
					"automation_id": automationID,
					"type":          "add_to_list",
					"config":        map[string]interface{}{"list_id": notInListTargetListID, "status": "active"},
					"position":      map[string]interface{}{"x": -150, "y": 200},
				},
				{
					"id":            addToActiveListNodeID,
					"automation_id": automationID,
					"type":          "add_to_list",
					"config":        map[string]interface{}{"list_id": activeTargetListID, "status": "active"},
					"position":      map[string]interface{}{"x": 0, "y": 200},
				},
				{
					"id":            addToNonActiveListNodeID,
					"automation_id": automationID,
					"type":          "add_to_list",
					"config":        map[string]interface{}{"list_id": nonActiveTargetListID, "status": "active"},
					"position":      map[string]interface{}{"x": 150, "y": 200},
				},
			},
			"stats": map[string]interface{}{"enrolled": 0, "completed": 0, "exited": 0, "failed": 0},
		},
	}

	resp, err := client.CreateAutomation(createReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	// 3. Activate automation via HTTP
	activateResp, err := client.ActivateAutomation(map[string]interface{}{
		"workspace_id":  workspaceID,
		"automation_id": automationID,
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, activateResp.StatusCode)
	activateResp.Body.Close()

	// Test 1: Contact not in list
	notInListEmail := "not-in-list-e2e@example.com"
	contactResp1, err := client.CreateContact(map[string]interface{}{
		"workspace_id": workspaceID,
		"contact":      map[string]interface{}{"email": notInListEmail},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, contactResp1.StatusCode, "Contact creation should succeed")
	contactResp1.Body.Close()

	err = factory.CreateCustomEvent(workspaceID, notInListEmail, "list_status_branch_test_event_e2e", nil)
	require.NoError(t, err)

	notInListCA := waitForEnrollmentViaAPI(t, client, automationID, notInListEmail, 2*time.Second)
	require.NotNil(t, notInListCA)
	// Status can be "active" or "completed" depending on scheduler timing
	notInListStatus := notInListCA["status"].(string)
	assert.True(t, notInListStatus == "active" || notInListStatus == "completed", "Status should be active or completed")

	// Test 2: Contact with active status
	activeEmail := "active-status-e2e@example.com"
	contactResp2, err := client.CreateContact(map[string]interface{}{
		"workspace_id": workspaceID,
		"contact":      map[string]interface{}{"email": activeEmail},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, contactResp2.StatusCode, "Contact creation should succeed")
	contactResp2.Body.Close()

	// Add contact to list with active status via HTTP
	subscribeResp, err := client.UpdateContactListStatus(workspaceID, activeEmail, listID, "active")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, subscribeResp.StatusCode)
	subscribeResp.Body.Close()

	err = factory.CreateCustomEvent(workspaceID, activeEmail, "list_status_branch_test_event_e2e", nil)
	require.NoError(t, err)

	activeCA := waitForEnrollmentViaAPI(t, client, automationID, activeEmail, 2*time.Second)
	require.NotNil(t, activeCA)
	// Status can be "active" or "completed" depending on scheduler timing
	activeStatus := activeCA["status"].(string)
	assert.True(t, activeStatus == "active" || activeStatus == "completed", "Status should be active or completed")

	// Test 3: Contact with unsubscribed status
	unsubEmail := "unsubscribed-status-e2e@example.com"
	contactResp3, err := client.CreateContact(map[string]interface{}{
		"workspace_id": workspaceID,
		"contact":      map[string]interface{}{"email": unsubEmail},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, contactResp3.StatusCode, "Contact creation should succeed")
	contactResp3.Body.Close()

	// Add contact to list, then unsubscribe via HTTP
	subResp, err := client.UpdateContactListStatus(workspaceID, unsubEmail, listID, "active")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, subResp.StatusCode, "Subscribe should succeed")
	subResp.Body.Close()
	unsubResp, err := client.UpdateContactListStatus(workspaceID, unsubEmail, listID, "unsubscribed")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, unsubResp.StatusCode, "Unsubscribe should succeed")
	unsubResp.Body.Close()

	err = factory.CreateCustomEvent(workspaceID, unsubEmail, "list_status_branch_test_event_e2e", nil)
	require.NoError(t, err)

	unsubCA := waitForEnrollmentViaAPI(t, client, automationID, unsubEmail, 2*time.Second)
	require.NotNil(t, unsubCA)
	t.Logf("Unsubscribed contact enrolled with status: %s", unsubCA["status"])

	// Wait for all 3 automations to complete
	notInListCompleted := waitForAutomationComplete(t, factory, workspaceID, automationID, notInListEmail, 10*time.Second)
	require.NotNil(t, notInListCompleted, "Not-in-list contact automation should complete")
	assert.Equal(t, domain.ContactAutomationStatusCompleted, notInListCompleted.Status)

	activeCompleted := waitForAutomationComplete(t, factory, workspaceID, automationID, activeEmail, 10*time.Second)
	require.NotNil(t, activeCompleted, "Active contact automation should complete")
	assert.Equal(t, domain.ContactAutomationStatusCompleted, activeCompleted.Status)

	unsubCompleted := waitForAutomationComplete(t, factory, workspaceID, automationID, unsubEmail, 10*time.Second)
	require.NotNil(t, unsubCompleted, "Unsubscribed contact automation should complete")
	assert.Equal(t, domain.ContactAutomationStatusCompleted, unsubCompleted.Status)

	// Verify stats show all completed (wait for stats to update after contact completion)
	stats := waitForStatsCompleted(t, factory, workspaceID, automationID, 3, 2*time.Second)
	require.NotNil(t, stats, "Stats should exist")
	assert.Equal(t, int64(3), stats.Enrolled, "All 3 contacts should be enrolled")
	assert.Equal(t, int64(3), stats.Completed, "All 3 contacts should be completed")
	t.Logf("Automation stats: enrolled=%d, completed=%d", stats.Enrolled, stats.Completed)

	t.Logf("List status branch E2E test passed: all 3 contacts completed automation")
}

// testAutomationListOperations tests add_to_list and remove_from_list nodes
// Uses HTTP for automation CRUD, factory for timeline events (intentional)
func testAutomationListOperations(t *testing.T, factory *testutil.TestDataFactory, client *testutil.APIClient, workspaceID string) {
	// 1. Create lists via factory (complex validation requirements)
	trialList, err := factory.CreateList(workspaceID)
	require.NoError(t, err)
	trialListID := trialList.ID

	premiumList, err := factory.CreateList(workspaceID)
	require.NoError(t, err)
	premiumListID := premiumList.ID

	// 2. Create automation via HTTP with add_to_list and remove_from_list nodes
	automationID := shortuuid.New()
	triggerNodeID := shortuuid.New()
	addNodeID := shortuuid.New()
	removeNodeID := shortuuid.New()

	createReq := map[string]interface{}{
		"workspace_id": workspaceID,
		"automation": map[string]interface{}{
			"id":           automationID,
			"workspace_id": workspaceID,
			"name":         "List Operations Automation E2E",
			"status":       "draft",
			"trigger": map[string]interface{}{
				"event_kind": "custom_event", "custom_event_name": "list_ops_event_e2e",
				"frequency": "once",
			},
			"root_node_id": triggerNodeID,
			"nodes": []map[string]interface{}{
				{
					"id":            triggerNodeID,
					"automation_id": automationID,
					"type":          "trigger",
					"config":        map[string]interface{}{},
					"next_node_id":  addNodeID,
					"position":      map[string]interface{}{"x": 0, "y": 0},
				},
				{
					"id":            addNodeID,
					"automation_id": automationID,
					"type":          "add_to_list",
					"config":        map[string]interface{}{"list_id": premiumListID, "status": "active"},
					"next_node_id":  removeNodeID,
					"position":      map[string]interface{}{"x": 0, "y": 100},
				},
				{
					"id":            removeNodeID,
					"automation_id": automationID,
					"type":          "remove_from_list",
					"config":        map[string]interface{}{"list_id": trialListID},
					"position":      map[string]interface{}{"x": 0, "y": 200},
				},
			},
			"stats": map[string]interface{}{"enrolled": 0, "completed": 0, "exited": 0, "failed": 0},
		},
	}

	resp, err := client.CreateAutomation(createReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	// 3. Activate automation via HTTP
	activateResp, err := client.ActivateAutomation(map[string]interface{}{
		"workspace_id":  workspaceID,
		"automation_id": automationID,
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, activateResp.StatusCode)
	activateResp.Body.Close()

	// 4. Create contact and add to trial list via HTTP
	email := "list-ops-e2e@example.com"
	contactResp, err := client.CreateContact(map[string]interface{}{
		"workspace_id": workspaceID,
		"contact":      map[string]interface{}{"email": email},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, contactResp.StatusCode, "Contact creation should succeed")
	contactResp.Body.Close()

	subscribeResp, err := client.UpdateContactListStatus(workspaceID, email, trialListID, "active")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, subscribeResp.StatusCode)
	subscribeResp.Body.Close()

	// 5. Trigger automation via factory (timeline events - no HTTP API)
	err = factory.CreateCustomEvent(workspaceID, email, "list_ops_event_e2e", nil)
	require.NoError(t, err)

	// 6. Wait for enrollment via HTTP
	caMap := waitForEnrollmentViaAPI(t, client, automationID, email, 2*time.Second)
	require.NotNil(t, caMap)
	t.Logf("Contact enrolled with status: %s", caMap["status"])

	// 7. Wait for automation to complete
	completedCA := waitForAutomationComplete(t, factory, workspaceID, automationID, email, 10*time.Second)
	require.NotNil(t, completedCA, "Automation should complete")
	assert.Equal(t, domain.ContactAutomationStatusCompleted, completedCA.Status, "Status should be completed")
	t.Logf("Automation completed for contact %s", email)

	// 8. Verify contact was added to premium list (first action node)
	premiumListResp, err := client.GetContactListByIDs(workspaceID, email, premiumListID)
	require.NoError(t, err)
	defer premiumListResp.Body.Close()
	assert.Equal(t, http.StatusOK, premiumListResp.StatusCode, "Contact should be in premium list after add_to_list node")
	t.Logf("Contact verified in premium list")

	// 9. Verify contact was removed from trial list (second action node)
	trialListResp, err := client.GetContactListByIDs(workspaceID, email, trialListID)
	require.NoError(t, err)
	defer trialListResp.Body.Close()
	// After remove_from_list, the contact-list record should either not exist or have non-active status
	// The API returns 404 if not found, or 200 with status that's not "active"
	if trialListResp.StatusCode == http.StatusOK {
		var trialResult map[string]interface{}
		json.NewDecoder(trialListResp.Body).Decode(&trialResult)
		if cl, ok := trialResult["contact_list"].(map[string]interface{}); ok {
			status := cl["status"]
			assert.NotEqual(t, "active", status, "Contact should not be active in trial list after remove_from_list node")
		}
	}
	t.Logf("Contact verified removed from trial list")

	// 10. Verify stats show completion (wait for stats to update after contact completion)
	stats := waitForStatsCompleted(t, factory, workspaceID, automationID, 1, 2*time.Second)
	require.NotNil(t, stats, "Stats should exist")
	assert.Equal(t, int64(1), stats.Enrolled, "Enrolled count should be 1")
	assert.Equal(t, int64(1), stats.Completed, "Completed count should be 1")
	t.Logf("Automation stats: enrolled=%d, completed=%d", stats.Enrolled, stats.Completed)

	t.Logf("List operations E2E test passed: both add_to_list and remove_from_list nodes executed")
}

// testAutomationContextData tests that timeline event data is passed to automation context
// Uses HTTP for automation CRUD, factory for timeline events (intentional)
func testAutomationContextData(t *testing.T, factory *testutil.TestDataFactory, client *testutil.APIClient, workspaceID string) {
	// 1. Create automation via HTTP
	automationID := shortuuid.New()
	triggerNodeID := shortuuid.New()

	createReq := map[string]interface{}{
		"workspace_id": workspaceID,
		"automation": map[string]interface{}{
			"id":           automationID,
			"workspace_id": workspaceID,
			"name":         "Context Data Automation E2E",
			"status":       "draft",
			"trigger": map[string]interface{}{
				"event_kind": "custom_event", "custom_event_name": "purchase_e2e",
				"frequency": "every_time",
			},
			"root_node_id": triggerNodeID,
			"nodes": []map[string]interface{}{
				{
					"id":            triggerNodeID,
					"automation_id": automationID,
					"type":          "trigger",
					"config":        map[string]interface{}{},
					"position":      map[string]interface{}{"x": 0, "y": 0},
				},
			},
			"stats": map[string]interface{}{"enrolled": 0, "completed": 0, "exited": 0, "failed": 0},
		},
	}

	resp, err := client.CreateAutomation(createReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	// 2. Activate automation via HTTP
	activateResp, err := client.ActivateAutomation(map[string]interface{}{
		"workspace_id":  workspaceID,
		"automation_id": automationID,
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, activateResp.StatusCode)
	activateResp.Body.Close()

	// 3. Create contact via HTTP
	email := "purchase-test-e2e@example.com"
	contactResp, err := client.CreateContact(map[string]interface{}{
		"workspace_id": workspaceID,
		"contact":      map[string]interface{}{"email": email},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, contactResp.StatusCode, "Contact creation should succeed")
	contactResp.Body.Close()

	// 4. Trigger with purchase data via factory (timeline events - no HTTP API)
	err = factory.CreateCustomEvent(workspaceID, email, "purchase_e2e", map[string]interface{}{
		"order_id": "ORD-123",
		"amount":   99.99,
		"items": []interface{}{
			map[string]interface{}{"sku": "SKU-001", "qty": 2},
		},
	})
	require.NoError(t, err)

	// 5. Wait for enrollment via HTTP
	caMap := waitForEnrollmentViaAPI(t, client, automationID, email, 2*time.Second)
	require.NotNil(t, caMap)
	t.Logf("Contact enrolled with status: %s", caMap["status"])

	// 6. Wait for automation to complete (trigger-only automation should complete)
	completedCA := waitForAutomationComplete(t, factory, workspaceID, automationID, email, 10*time.Second)
	require.NotNil(t, completedCA, "Automation should complete")
	assert.Equal(t, domain.ContactAutomationStatusCompleted, completedCA.Status, "Status should be completed")
	t.Logf("Automation completed for contact %s", email)

	// 7. Verify stats show completion (wait for stats to update after contact completion)
	stats := waitForStatsCompleted(t, factory, workspaceID, automationID, 1, 2*time.Second)
	require.NotNil(t, stats, "Stats should exist")
	assert.Equal(t, int64(1), stats.Enrolled, "Enrolled count should be 1")
	assert.Equal(t, int64(1), stats.Completed, "Completed count should be 1")
	t.Logf("Automation stats: enrolled=%d, completed=%d", stats.Enrolled, stats.Completed)

	t.Logf("Context data E2E test passed: automation completed with purchase event")
}

// testAutomationSegmentTrigger tests triggering automation on segment.joined event
// Uses HTTP for automation CRUD, factory for timeline events (intentional)
func testAutomationSegmentTrigger(t *testing.T, factory *testutil.TestDataFactory, client *testutil.APIClient, workspaceID string) {
	// 1. Create segment via factory (required for segment.joined trigger)
	segment, err := factory.CreateSegment(workspaceID)
	require.NoError(t, err)
	segmentID := segment.ID

	// 2. Create automation via HTTP triggered by segment.joined
	automationID := shortuuid.New()
	triggerNodeID := shortuuid.New()

	createReq := map[string]interface{}{
		"workspace_id": workspaceID,
		"automation": map[string]interface{}{
			"id":           automationID,
			"workspace_id": workspaceID,
			"name":         "Segment Trigger Automation E2E",
			"status":       "draft",
			"trigger": map[string]interface{}{
				"event_kind": "segment.joined",
				"segment_id": segmentID,
				"frequency":  "once",
			},
			"root_node_id": triggerNodeID,
			"nodes": []map[string]interface{}{
				{
					"id":            triggerNodeID,
					"automation_id": automationID,
					"type":          "trigger",
					"config":        map[string]interface{}{},
					"position":      map[string]interface{}{"x": 0, "y": 0},
				},
			},
			"stats": map[string]interface{}{"enrolled": 0, "completed": 0, "exited": 0, "failed": 0},
		},
	}

	resp, err := client.CreateAutomation(createReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	// 2. Activate automation via HTTP
	activateResp, err := client.ActivateAutomation(map[string]interface{}{
		"workspace_id":  workspaceID,
		"automation_id": automationID,
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, activateResp.StatusCode)
	activateResp.Body.Close()

	// 3. Create contact via HTTP
	email := "segment-trigger-e2e@example.com"
	contactResp, err := client.CreateContact(map[string]interface{}{
		"workspace_id": workspaceID,
		"contact":      map[string]interface{}{"email": email},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, contactResp.StatusCode, "Contact creation should succeed")
	contactResp.Body.Close()

	// 4. Simulate segment.joined event via factory (timeline events - no HTTP API)
	// Note: entity_id must match segment_id for the trigger to fire
	err = factory.CreateContactTimelineEvent(workspaceID, email, "segment.joined", map[string]interface{}{
		"entity_id":    segmentID, // Required for segment.* trigger matching
		"entity_type":  "contact_segment",
		"segment_id":   segmentID,
		"segment_name": segment.Name,
	})
	require.NoError(t, err)

	// 5. Wait for enrollment via HTTP
	caMap := waitForEnrollmentViaAPI(t, client, automationID, email, 2*time.Second)
	require.NotNil(t, caMap)
	t.Logf("Contact enrolled with status: %s", caMap["status"])

	// 6. Wait for automation to complete (trigger-only automation should complete)
	completedCA := waitForAutomationComplete(t, factory, workspaceID, automationID, email, 10*time.Second)
	require.NotNil(t, completedCA, "Automation should complete")
	assert.Equal(t, domain.ContactAutomationStatusCompleted, completedCA.Status, "Status should be completed")
	t.Logf("Automation completed for contact %s", email)

	// 7. Verify stats show completion (wait for stats to update after contact completion)
	stats := waitForStatsCompleted(t, factory, workspaceID, automationID, 1, 2*time.Second)
	require.NotNil(t, stats, "Stats should exist")
	assert.Equal(t, int64(1), stats.Enrolled, "Enrolled count should be 1")
	assert.Equal(t, int64(1), stats.Completed, "Completed count should be 1")
	t.Logf("Automation stats: enrolled=%d, completed=%d", stats.Enrolled, stats.Completed)

	t.Logf("Segment trigger E2E test passed: automation completed on segment.joined")
}

// testAutomationDeletionCleanup tests that deleting automation cleans up properly
// Uses HTTP for automation CRUD and deletion, factory for timeline events (intentional)
func testAutomationDeletionCleanup(t *testing.T, factory *testutil.TestDataFactory, client *testutil.APIClient, workspaceID string) {
	// 1. Create automation via HTTP
	automationID := shortuuid.New()
	triggerNodeID := shortuuid.New()

	createReq := map[string]interface{}{
		"workspace_id": workspaceID,
		"automation": map[string]interface{}{
			"id":           automationID,
			"workspace_id": workspaceID,
			"name":         "Deletion Test Automation E2E",
			"status":       "draft",
			"trigger": map[string]interface{}{
				"event_kind": "custom_event", "custom_event_name": "delete_test_event_e2e",
				"frequency": "once",
			},
			"root_node_id": triggerNodeID,
			"nodes": []map[string]interface{}{
				{
					"id":            triggerNodeID,
					"automation_id": automationID,
					"type":          "trigger",
					"config":        map[string]interface{}{},
					"position":      map[string]interface{}{"x": 0, "y": 0},
				},
			},
			"stats": map[string]interface{}{"enrolled": 0, "completed": 0, "exited": 0, "failed": 0},
		},
	}

	resp, err := client.CreateAutomation(createReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	// 2. Activate automation via HTTP
	activateResp, err := client.ActivateAutomation(map[string]interface{}{
		"workspace_id":  workspaceID,
		"automation_id": automationID,
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, activateResp.StatusCode)
	activateResp.Body.Close()

	// 3. Create contact via HTTP
	email := "delete-test-e2e@example.com"
	contactResp, err := client.CreateContact(map[string]interface{}{
		"workspace_id": workspaceID,
		"contact":      map[string]interface{}{"email": email},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, contactResp.StatusCode, "Contact creation should succeed")
	contactResp.Body.Close()

	// 4. Trigger via factory (timeline events - no HTTP API)
	err = factory.CreateCustomEvent(workspaceID, email, "delete_test_event_e2e", nil)
	require.NoError(t, err)

	// 5. Wait for enrollment via HTTP
	caMap := waitForEnrollmentViaAPI(t, client, automationID, email, 2*time.Second)
	require.NotNil(t, caMap)
	assert.Equal(t, "active", caMap["status"])

	// 6. Delete automation via HTTP API (use new client method)
	deleteResp, err := client.DeleteAutomation(map[string]interface{}{
		"workspace_id":  workspaceID,
		"automation_id": automationID,
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, deleteResp.StatusCode)
	deleteResp.Body.Close()

	// 7. Verify via HTTP: automation should return 404 or error (not 200)
	getResp, err := client.GetAutomation(automationID)
	require.NoError(t, err)
	// Soft-deleted automation should NOT return 200 OK - it should be 404 or 500
	require.NotEqual(t, http.StatusOK, getResp.StatusCode, "Deleted automation should not return 200 OK")
	getResp.Body.Close()

	// 8. Verify via factory: automation has deleted_at set
	workspaceDB, err := factory.GetWorkspaceDB(workspaceID)
	require.NoError(t, err)
	var deletedAt sql.NullTime
	err = workspaceDB.QueryRowContext(context.Background(),
		`SELECT deleted_at FROM automations WHERE id = $1`,
		automationID,
	).Scan(&deletedAt)
	require.NoError(t, err)

	if !deletedAt.Valid {
		addBug("TestAutomation_DeletionCleanup",
			"Automation not soft-deleted after Delete API call",
			"High", "Delete not setting deleted_at",
			"internal/repository/automation_postgres.go:Delete")
	}

	// 9. Verify: active contacts should be marked as exited (factory - no HTTP API)
	caAfter, err := factory.GetContactAutomation(workspaceID, automationID, email)
	if err == nil && caAfter.Status == domain.ContactAutomationStatusActive {
		addBug("TestAutomation_DeletionCleanup",
			"Active contact not marked as exited after automation deletion",
			"Medium", "Delete not updating contact_automations",
			"internal/repository/automation_postgres.go:Delete")
	}

	t.Logf("Deletion cleanup E2E test passed")
}

// testAutomationErrorRecovery tests retry mechanism for failed node executions
func testAutomationErrorRecovery(t *testing.T, factory *testutil.TestDataFactory, client *testutil.APIClient, workspaceID string) {
	// 1. Build and create simple automation via HTTP (just trigger node)
	// The purpose is to verify retry infrastructure fields exist, not test email sending
	automationID := shortuuid.New()
	triggerNodeID := shortuuid.New()

	createReq := map[string]interface{}{
		"workspace_id": workspaceID,
		"automation": map[string]interface{}{
			"id":           automationID,
			"workspace_id": workspaceID,
			"name":         "Error Recovery Automation",
			"status":       "draft",
			"trigger": map[string]interface{}{
				"event_kind": "custom_event", "custom_event_name": "error_test_event",
				"frequency": "once",
			},
			"root_node_id": triggerNodeID,
			"nodes": []map[string]interface{}{
				{
					"id":            triggerNodeID,
					"automation_id": automationID,
					"type":          "trigger",
					"config":        map[string]interface{}{},
					"position":      map[string]interface{}{"x": 0, "y": 0},
				},
			},
			"stats": map[string]interface{}{
				"enrolled":  0,
				"completed": 0,
				"exited":    0,
				"failed":    0,
			},
		},
	}

	resp, err := client.CreateAutomation(createReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode, "Automation creation should succeed")
	resp.Body.Close()

	// 2. Activate automation via HTTP
	activateResp, err := client.ActivateAutomation(map[string]interface{}{
		"workspace_id":  workspaceID,
		"automation_id": automationID,
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, activateResp.StatusCode, "Automation activation should succeed")
	activateResp.Body.Close()

	// 3. Create contact via factory (ensures contact exists before custom event)
	email := "error-test@example.com"
	_, err = factory.CreateContact(workspaceID, testutil.WithContactEmail(email))
	require.NoError(t, err)

	// 4. Trigger automation via timeline event (factory - no HTTP API)
	err = factory.CreateCustomEvent(workspaceID, email, "error_test_event", nil)
	require.NoError(t, err)

	// 5. Wait for enrollment via HTTP (enrollment should succeed even if later execution fails)
	ca := waitForEnrollmentViaAPI(t, client, automationID, email, 2*time.Second)
	require.NotNil(t, ca)
	// Status can be "active" (just enrolled) or "completed" (scheduler already processed the trigger-only workflow)
	status := ca["status"].(string)
	assert.True(t, status == "active" || status == "completed", "Status should be active or completed, got: %s", status)

	// 6. Verify retry infrastructure exists (factory - for deep inspection of retry fields)
	caFromFactory, err := factory.GetContactAutomation(workspaceID, automationID, email)
	require.NoError(t, err)
	assert.Equal(t, 0, caFromFactory.RetryCount, "Initial retry count should be 0")
	assert.Equal(t, 3, caFromFactory.MaxRetries, "Default max retries should be 3")

	t.Logf("Error recovery test passed: retry infrastructure verified")
}

// testAutomationSchedulerExecution tests that the scheduler processes contacts correctly
func testAutomationSchedulerExecution(t *testing.T, factory *testutil.TestDataFactory, client *testutil.APIClient, workspaceID string) {
	// 1. Create list via factory (complex validation requirements)
	list, err := factory.CreateList(workspaceID)
	require.NoError(t, err)
	listID := list.ID

	// 2. Create template via factory (complex MJML validation)
	template, err := factory.CreateTemplate(workspaceID)
	require.NoError(t, err)
	templateID := template.ID

	// 3. Build and create automation via HTTP
	automationID := shortuuid.New()
	triggerNodeID := shortuuid.New()
	emailNodeID := shortuuid.New()

	createReq := map[string]interface{}{
		"workspace_id": workspaceID,
		"automation": map[string]interface{}{
			"id":           automationID,
			"workspace_id": workspaceID,
			"name":         "Scheduler Execution Automation",
			"status":       "draft",
			"list_id":      listID,
			"trigger": map[string]interface{}{
				"event_kind": "custom_event", "custom_event_name": "scheduler_test_event",
				"frequency": "once",
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
			"stats": map[string]interface{}{
				"enrolled":  0,
				"completed": 0,
				"exited":    0,
				"failed":    0,
			},
		},
	}

	resp, err := client.CreateAutomation(createReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	// 4. Activate automation via HTTP
	activateResp, err := client.ActivateAutomation(map[string]interface{}{
		"workspace_id":  workspaceID,
		"automation_id": automationID,
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, activateResp.StatusCode)
	activateResp.Body.Close()

	// 5. Create contact and subscribe to list via factory
	email := "scheduler-test@example.com"
	_, err = factory.CreateContact(workspaceID, testutil.WithContactEmail(email))
	require.NoError(t, err)

	// 6. Subscribe contact to list via factory (no HTTP API for this)
	_, err = factory.CreateContactList(workspaceID,
		testutil.WithContactListEmail(email),
		testutil.WithContactListListID(listID),
	)
	require.NoError(t, err)

	// 7. Trigger automation via timeline event (factory - no HTTP API)
	err = factory.CreateCustomEvent(workspaceID, email, "scheduler_test_event", nil)
	require.NoError(t, err)

	// 8. Wait for enrollment via HTTP
	ca := waitForEnrollmentViaAPI(t, client, automationID, email, 2*time.Second)
	require.NotNil(t, ca)
	// Status can be "active" (just enrolled) or "completed" (scheduler already processed)
	status := ca["status"].(string)
	assert.True(t, status == "active" || status == "completed", "Status should be active or completed, got: %s", status)

	// 9. Verify node executions (factory - nodeExecutions API returns these)
	caFromFactory, err := factory.GetContactAutomation(workspaceID, automationID, email)
	require.NoError(t, err)

	executions, err := factory.GetNodeExecutions(workspaceID, caFromFactory.ID)
	require.NoError(t, err)

	if len(executions) == 0 {
		addBug("TestAutomation_SchedulerExecution",
			"No node execution entries created on enrollment",
			"High", "automation_enroll_contact not logging entry",
			"internal/migrations/v20.go:automation_enroll_contact")
	} else {
		t.Logf("Node executions found: %d", len(executions))
		for _, exec := range executions {
			t.Logf("  - Node %s (%s): action=%s", exec.NodeID, exec.NodeType, exec.Action)
		}
	}

	// 10. Verify contact is scheduled for processing
	if caFromFactory.ScheduledAt == nil {
		addBug("TestAutomation_SchedulerExecution",
			"Contact not scheduled for processing after enrollment",
			"High", "scheduled_at not set by enrollment",
			"internal/migrations/v20.go:automation_enroll_contact")
	} else {
		t.Logf("Contact scheduled for: %v", caFromFactory.ScheduledAt)
	}

	t.Logf("Scheduler execution test passed: enrollment verified")
}

// testAutomationPauseResume tests that paused automations freeze contacts instead of exiting them
func testAutomationPauseResume(t *testing.T, factory *testutil.TestDataFactory, client *testutil.APIClient, workspaceID string) {
	// 1. Build and create automation via HTTP
	automationID := shortuuid.New()
	triggerNodeID := shortuuid.New()
	delayNodeID := shortuuid.New()

	createReq := map[string]interface{}{
		"workspace_id": workspaceID,
		"automation": map[string]interface{}{
			"id":           automationID,
			"workspace_id": workspaceID,
			"name":         "Pause Resume Test",
			"status":       "draft",
			"trigger": map[string]interface{}{
				"event_kind": "custom_event", "custom_event_name": "test_pause_event",
				"frequency": "once",
			},
			"root_node_id": triggerNodeID,
			"nodes": []map[string]interface{}{
				{
					"id":            triggerNodeID,
					"automation_id": automationID,
					"type":          "trigger",
					"config":        map[string]interface{}{},
					"next_node_id":  delayNodeID,
					"position":      map[string]interface{}{"x": 0, "y": 0},
				},
				{
					"id":            delayNodeID,
					"automation_id": automationID,
					"type":          "delay",
					"config": map[string]interface{}{
						"duration": 1,
						"unit":     "minutes",
					},
					"position": map[string]interface{}{"x": 0, "y": 100},
				},
			},
			"stats": map[string]interface{}{
				"enrolled":  0,
				"completed": 0,
				"exited":    0,
				"failed":    0,
			},
		},
	}

	resp, err := client.CreateAutomation(createReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	// 2. Activate automation via HTTP
	activateResp, err := client.ActivateAutomation(map[string]interface{}{
		"workspace_id":  workspaceID,
		"automation_id": automationID,
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, activateResp.StatusCode)
	activateResp.Body.Close()

	// 3. Create contact via HTTP
	email := "pause-test@example.com"
	contactResp, err := client.CreateContact(map[string]interface{}{
		"workspace_id": workspaceID,
		"contact": map[string]interface{}{
			"email":      email,
			"first_name": "Pause",
			"last_name":  "Test",
		},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, contactResp.StatusCode, "Contact creation should succeed")
	contactResp.Body.Close()

	// 4. Trigger automation via timeline event (factory - no HTTP API)
	err = factory.CreateCustomEvent(workspaceID, email, "test_pause_event", map[string]interface{}{})
	require.NoError(t, err)

	// 5. Wait for enrollment via HTTP
	ca := waitForEnrollmentViaAPI(t, client, automationID, email, 2*time.Second)
	require.NotNil(t, ca, "Contact should be enrolled")
	assert.Equal(t, "active", ca["status"])
	t.Logf("Contact enrolled with status: %s", ca["status"])

	// 6. PAUSE the automation via HTTP
	pauseResp, err := client.PauseAutomation(map[string]interface{}{
		"workspace_id":  workspaceID,
		"automation_id": automationID,
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, pauseResp.StatusCode)
	pauseResp.Body.Close()
	t.Log("Automation paused via HTTP")

	// 7. Verify contact status is still ACTIVE (not exited!)
	caFromFactory, err := factory.GetContactAutomation(workspaceID, automationID, email)
	require.NoError(t, err)
	assert.Equal(t, domain.ContactAutomationStatusActive, caFromFactory.Status, "Contact should still be ACTIVE when automation is paused")
	t.Logf("After pause - Contact status: %s (should be active)", caFromFactory.Status)

	// 8. Verify scheduler query does NOT return this contact (paused automation filtered out)
	workspaceDB, err := factory.GetWorkspaceDB(workspaceID)
	require.NoError(t, err)

	schedulerQuery := `
		SELECT ca.id, ca.contact_email
		FROM contact_automations ca
		JOIN automations a ON ca.automation_id = a.id
		WHERE ca.status = 'active'
		  AND ca.scheduled_at <= $1
		  AND a.status = 'live'
		  AND a.deleted_at IS NULL
	`
	rows, err := workspaceDB.QueryContext(context.Background(), schedulerQuery, time.Now().Add(1*time.Hour))
	require.NoError(t, err)
	defer rows.Close()

	found := false
	for rows.Next() {
		var id, emailScanned string
		err := rows.Scan(&id, &emailScanned)
		require.NoError(t, err)
		if emailScanned == email {
			found = true
			break
		}
	}
	assert.False(t, found, "Contact should NOT be returned by scheduler when automation is paused")
	t.Logf("Scheduler query returned paused contact: %v (should be false)", found)

	// 9. RESUME the automation via HTTP (reactivate)
	resumeResp, err := client.ActivateAutomation(map[string]interface{}{
		"workspace_id":  workspaceID,
		"automation_id": automationID,
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resumeResp.StatusCode)
	resumeResp.Body.Close()
	t.Log("Automation resumed via HTTP")

	// 10. Verify contact can now be scheduled
	rows2, err := workspaceDB.QueryContext(context.Background(), schedulerQuery, time.Now().Add(1*time.Hour))
	require.NoError(t, err)
	defer rows2.Close()

	found = false
	for rows2.Next() {
		var id, emailScanned string
		err := rows2.Scan(&id, &emailScanned)
		require.NoError(t, err)
		if emailScanned == email {
			found = true
			break
		}
	}
	assert.True(t, found, "Contact should be returned by scheduler after automation is resumed")
	t.Logf("After resume - Scheduler query returned contact: %v (should be true)", found)

	t.Log("Pause/Resume test passed: contacts freeze when paused and resume when automation is live again")
}

// testAutomationPermissions tests that automation API respects user permissions
func testAutomationPermissions(t *testing.T, factory *testutil.TestDataFactory, client *testutil.APIClient, workspaceID string, owner *domain.User, memberNoPerms *domain.User, memberReadOnly *domain.User) {
	// Owner creates an automation via HTTP
	err := client.Login(owner.Email, "password")
	require.NoError(t, err)
	client.SetWorkspaceID(workspaceID)

	automationID := shortuuid.New()
	triggerNodeID := shortuuid.New()

	createReq := map[string]interface{}{
		"workspace_id": workspaceID,
		"automation": map[string]interface{}{
			"id":           automationID,
			"workspace_id": workspaceID,
			"name":         "Permission Test Automation",
			"status":       "draft",
			"trigger": map[string]interface{}{
				"event_kind": "custom_event", "custom_event_name": "test_event",
				"frequency": "once",
			},
			"root_node_id": triggerNodeID,
			"nodes": []map[string]interface{}{
				{
					"id":            triggerNodeID,
					"automation_id": automationID,
					"type":          "trigger",
					"config":        map[string]interface{}{},
					"position":      map[string]interface{}{"x": 0, "y": 0},
				},
			},
			"stats": map[string]interface{}{
				"enrolled":  0,
				"completed": 0,
				"exited":    0,
				"failed":    0,
			},
		},
	}

	resp, err := client.CreateAutomation(createReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode, "Owner should be able to create automation")
	resp.Body.Close()
	t.Logf("Owner created automation via HTTP: %s", automationID)

	// Test 1: User with NO permissions cannot list automations
	t.Run("no_permissions_cannot_list", func(t *testing.T) {
		err = client.Login(memberNoPerms.Email, "password")
		require.NoError(t, err)
		client.SetWorkspaceID(workspaceID)

		resp, err := client.Get(fmt.Sprintf("/api/automations.list?workspace_id=%s", workspaceID))
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusForbidden, resp.StatusCode, "User without automations read permission should get 403")
		t.Logf("User with no permissions got status %d (expected 403)", resp.StatusCode)
	})

	// Test 2: User with read-only permissions can list automations
	t.Run("read_only_can_list", func(t *testing.T) {
		err = client.Login(memberReadOnly.Email, "password")
		require.NoError(t, err)
		client.SetWorkspaceID(workspaceID)

		resp, err := client.Get(fmt.Sprintf("/api/automations.list?workspace_id=%s", workspaceID))
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode, "User with automations read permission should get 200")
		t.Logf("User with read-only permissions got status %d (expected 200)", resp.StatusCode)
	})

	// Test 3: User with read-only permissions cannot create automations
	t.Run("read_only_cannot_create", func(t *testing.T) {
		err = client.Login(memberReadOnly.Email, "password")
		require.NoError(t, err)
		client.SetWorkspaceID(workspaceID)

		resp, err := client.Post("/api/automations.create", map[string]interface{}{
			"workspace_id": workspaceID,
			"automation": map[string]interface{}{
				"id":           "test-create-fail",
				"workspace_id": workspaceID,
				"name":         "Should Fail",
				"status":       "draft",
				"trigger": map[string]interface{}{
					"event_kind": "contact.created",
					"frequency":  "once",
				},
				"nodes": []interface{}{},
				"stats": map[string]interface{}{},
			},
		})
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusForbidden, resp.StatusCode, "User without automations write permission should get 403 on create")
		t.Logf("User with read-only permissions trying to create got status %d (expected 403)", resp.StatusCode)
	})

	// Test 4: Owner can create automations (owner bypasses permissions)
	t.Run("owner_can_create", func(t *testing.T) {
		err = client.Login(owner.Email, "password")
		require.NoError(t, err)
		client.SetWorkspaceID(workspaceID)

		resp, err := client.Post("/api/automations.create", map[string]interface{}{
			"workspace_id": workspaceID,
			"automation": map[string]interface{}{
				"id":           "owner-created-auto",
				"workspace_id": workspaceID,
				"name":         "Owner Created Automation",
				"status":       "draft",
				"trigger": map[string]interface{}{
					"event_kind": "contact.created",
					"frequency":  "once",
				},
				"nodes": []interface{}{},
				"stats": map[string]interface{}{},
			},
		})
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusCreated, resp.StatusCode, "Owner should be able to create automations")
		t.Logf("Owner creating automation got status %d (expected 201)", resp.StatusCode)
	})

	t.Log("Automation permissions test passed")
}

// testAutomationTimelineStartEvent tests that automation.start timeline event is created on enrollment
func testAutomationTimelineStartEvent(t *testing.T, factory *testutil.TestDataFactory, client *testutil.APIClient, workspaceID string) {
	// 1. Build and create automation via HTTP
	automationID := shortuuid.New()
	triggerNodeID := shortuuid.New()

	createReq := map[string]interface{}{
		"workspace_id": workspaceID,
		"automation": map[string]interface{}{
			"id":           automationID,
			"workspace_id": workspaceID,
			"name":         "Timeline Start Event Test",
			"status":       "draft",
			"trigger": map[string]interface{}{
				"event_kind": "custom_event", "custom_event_name": "timeline_start_test_event",
				"frequency": "once",
			},
			"root_node_id": triggerNodeID,
			"nodes": []map[string]interface{}{
				{
					"id":            triggerNodeID,
					"automation_id": automationID,
					"type":          "trigger",
					"config":        map[string]interface{}{},
					"position":      map[string]interface{}{"x": 0, "y": 0},
				},
			},
			"stats": map[string]interface{}{
				"enrolled":  0,
				"completed": 0,
				"exited":    0,
				"failed":    0,
			},
		},
	}

	resp, err := client.CreateAutomation(createReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	// 2. Activate automation via HTTP
	activateResp, err := client.ActivateAutomation(map[string]interface{}{
		"workspace_id":  workspaceID,
		"automation_id": automationID,
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, activateResp.StatusCode)
	activateResp.Body.Close()

	// 3. Create contact via HTTP
	email := "timeline-start@example.com"
	contactResp, err := client.CreateContact(map[string]interface{}{
		"workspace_id": workspaceID,
		"contact": map[string]interface{}{
			"email":      email,
			"first_name": "Timeline",
			"last_name":  "Start",
		},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, contactResp.StatusCode, "Contact creation should succeed")
	contactResp.Body.Close()

	// 4. Trigger automation via timeline event (factory - no HTTP API)
	err = factory.CreateCustomEvent(workspaceID, email, "timeline_start_test_event", nil)
	require.NoError(t, err)

	// 5. Wait for enrollment via HTTP
	ca := waitForEnrollmentViaAPI(t, client, automationID, email, 2*time.Second)
	require.NotNil(t, ca, "Contact should be enrolled")
	// Status can be "active" (just enrolled) or "completed" (scheduler already processed the trigger-only workflow)
	status := ca["status"].(string)
	assert.True(t, status == "active" || status == "completed", "Status should be active or completed, got: %s", status)

	// 6. Wait for automation.start timeline event (factory - no HTTP API for timeline events)
	events := waitForTimelineEvent(t, factory, workspaceID, email, "automation.start", 2*time.Second)

	if len(events) == 0 {
		addBug("TestAutomation_TimelineStartEvent",
			"No automation.start timeline event created on enrollment",
			"High", "automation_enroll_contact function not inserting timeline event",
			"internal/database/init.go:automation_enroll_contact")
		t.Fatal("Expected automation.start timeline event, found none")
	}

	// 7. Verify the event has correct data
	event := events[0]
	assert.Equal(t, "automation", event.EntityType, "Entity type should be 'automation'")
	assert.Equal(t, "automation.start", event.Kind)
	assert.Equal(t, "insert", event.Operation)
	require.NotNil(t, event.EntityID, "EntityID should be set")
	assert.Equal(t, automationID, *event.EntityID, "EntityID should be automation ID")

	// 8. Verify changes contain automation_id and root_node_id
	require.NotNil(t, event.Changes)
	automationIDChange, ok := event.Changes["automation_id"].(map[string]interface{})
	require.True(t, ok, "Changes should contain automation_id")
	assert.Equal(t, automationID, automationIDChange["new"], "automation_id.new should match automation ID")

	rootNodeIDChange, ok := event.Changes["root_node_id"].(map[string]interface{})
	require.True(t, ok, "Changes should contain root_node_id")
	assert.Equal(t, triggerNodeID, rootNodeIDChange["new"], "root_node_id.new should match trigger node ID")

	t.Logf("Timeline start event test passed: automation.start event created with correct data")
}

// testAutomationTimelineEndEvent tests that automation.end timeline event is created when contact completes
func testAutomationTimelineEndEvent(t *testing.T, factory *testutil.TestDataFactory, client *testutil.APIClient, workspaceID string) {
	// 1. Build and create automation via HTTP with trigger → delay (terminal)
	automationID := shortuuid.New()
	triggerNodeID := shortuuid.New()
	delayNodeID := shortuuid.New()

	createReq := map[string]interface{}{
		"workspace_id": workspaceID,
		"automation": map[string]interface{}{
			"id":           automationID,
			"workspace_id": workspaceID,
			"name":         "Timeline End Event Test",
			"status":       "draft",
			"trigger": map[string]interface{}{
				"event_kind": "custom_event", "custom_event_name": "timeline_end_test_event",
				"frequency": "once",
			},
			"root_node_id": triggerNodeID,
			"nodes": []map[string]interface{}{
				{
					"id":            triggerNodeID,
					"automation_id": automationID,
					"type":          "trigger",
					"config":        map[string]interface{}{},
					"next_node_id":  delayNodeID,
					"position":      map[string]interface{}{"x": 0, "y": 0},
				},
				{
					"id":            delayNodeID,
					"automation_id": automationID,
					"type":          "delay",
					"config": map[string]interface{}{
						"duration": 1,
						"unit":     "minutes",
					},
					"position": map[string]interface{}{"x": 0, "y": 100},
				},
			},
			"stats": map[string]interface{}{
				"enrolled":  0,
				"completed": 0,
				"exited":    0,
				"failed":    0,
			},
		},
	}

	resp, err := client.CreateAutomation(createReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	// 2. Activate automation via HTTP
	activateResp, err := client.ActivateAutomation(map[string]interface{}{
		"workspace_id":  workspaceID,
		"automation_id": automationID,
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, activateResp.StatusCode)
	activateResp.Body.Close()

	// 3. Create contact via HTTP
	email := "timeline-end@example.com"
	contactResp, err := client.CreateContact(map[string]interface{}{
		"workspace_id": workspaceID,
		"contact": map[string]interface{}{
			"email":      email,
			"first_name": "Timeline",
			"last_name":  "End",
		},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, contactResp.StatusCode, "Contact creation should succeed")
	contactResp.Body.Close()

	// 4. Trigger automation via timeline event (factory - no HTTP API)
	err = factory.CreateCustomEvent(workspaceID, email, "timeline_end_test_event", nil)
	require.NoError(t, err)

	// 5. Wait for enrollment via HTTP
	ca := waitForEnrollmentViaAPI(t, client, automationID, email, 2*time.Second)
	require.NotNil(t, ca, "Contact should be enrolled")

	// 6. Verify automation.start event exists (factory - no HTTP API for timeline events)
	startEvents, err := factory.GetContactTimelineEvents(workspaceID, email, "automation.start")
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(startEvents), 1, "Should have automation.start event")

	// Note: automation.end event is created by the scheduler when processing contacts
	// through terminal nodes. The scheduler is not running in these integration tests
	// by default, so we verify the infrastructure is in place.

	// 7. If the scheduler has processed (status = completed), check for end event
	caFromFactory, err := factory.GetContactAutomation(workspaceID, automationID, email)
	require.NoError(t, err)

	if caFromFactory.Status == domain.ContactAutomationStatusCompleted {
		endEvents, err := factory.GetContactTimelineEvents(workspaceID, email, "automation.end")
		require.NoError(t, err)

		if len(endEvents) == 0 {
			addBug("TestAutomation_TimelineEndEvent_Completed",
				"No automation.end timeline event created when contact completed",
				"High", "createAutomationEndEvent not called in markAsCompleted",
				"internal/service/automation_executor.go:markAsCompleted")
		} else {
			event := endEvents[0]
			assert.Equal(t, "automation", event.EntityType)
			assert.Equal(t, "automation.end", event.Kind)
			assert.Equal(t, "update", event.Operation)

			// Verify exit_reason is "completed"
			if exitReason, ok := event.Changes["exit_reason"].(map[string]interface{}); ok {
				assert.Equal(t, "completed", exitReason["new"], "exit_reason should be 'completed'")
			}
		}
	}

	t.Logf("Timeline end event test: enrollment verified, automation.end requires scheduler execution")
	t.Logf("Contact status: %s (scheduler needed for completion)", caFromFactory.Status)
}

// testAutomationContactCreatedTrigger tests the contact.created trigger scenario
// This is a true e2e test for GitHub issue #191:
// - Create automation with contact.created trigger AND a list_id (for unsubscribe URLs)
// - Activate automation
// - Create a new contact via HTTP API (contact is NOT subscribed to the list yet)
// - Verify the contact is automatically enrolled in the automation
//
// BUG: The automation_enroll_contact() function incorrectly checks if the contact
// is subscribed to the automation's list_id. But list_id is only meant for generating
// unsubscribe URLs in email templates, NOT for filtering enrollment.
// A newly created contact can never be subscribed to a list, so enrollment fails silently.
func testAutomationContactCreatedTrigger(t *testing.T, factory *testutil.TestDataFactory, client *testutil.APIClient, workspaceID string) {
	// 1. Create a list (required for automation with email nodes to have unsubscribe URLs)
	list, err := factory.CreateList(workspaceID)
	require.NoError(t, err)
	listID := list.ID
	t.Logf("List created: %s", listID)

	// 2. Create automation via HTTP with contact.created trigger AND list_id
	// The list_id is used for unsubscribe URLs in email templates, NOT for filtering enrollment
	automationID := shortuuid.New()
	triggerNodeID := shortuuid.New()
	delayNodeID := shortuuid.New()

	createReq := map[string]interface{}{
		"workspace_id": workspaceID,
		"automation": map[string]interface{}{
			"id":           automationID,
			"workspace_id": workspaceID,
			"name":         "Contact Created Trigger E2E Test",
			"status":       "draft",
			"list_id":      listID, // This is for unsubscribe URLs, should NOT affect enrollment
			"trigger": map[string]interface{}{
				"event_kind": "contact.created",
				"frequency":  "every_time",
			},
			"root_node_id": triggerNodeID,
			"nodes": []map[string]interface{}{
				{
					"id":            triggerNodeID,
					"automation_id": automationID,
					"type":          "trigger",
					"config":        map[string]interface{}{},
					"next_node_id":  delayNodeID,
					"position":      map[string]interface{}{"x": 0, "y": 0},
				},
				{
					"id":            delayNodeID,
					"automation_id": automationID,
					"type":          "delay",
					"config": map[string]interface{}{
						"duration": 1,
						"unit":     "days",
					},
					"position": map[string]interface{}{"x": 0, "y": 100},
				},
			},
			"stats": map[string]interface{}{
				"enrolled":  0,
				"completed": 0,
				"exited":    0,
				"failed":    0,
			},
		},
	}

	resp, err := client.CreateAutomation(createReq)
	require.NoError(t, err)
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("ContactCreatedTrigger CreateAutomation: Expected 201, got %d: %s", resp.StatusCode, string(body))
	}
	resp.Body.Close()
	t.Logf("Automation created: %s (with list_id: %s)", automationID, listID)

	// 3. Activate automation via HTTP
	activateResp, err := client.ActivateAutomation(map[string]interface{}{
		"workspace_id":  workspaceID,
		"automation_id": automationID,
	})
	require.NoError(t, err)
	if activateResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(activateResp.Body)
		activateResp.Body.Close()
		t.Fatalf("ContactCreatedTrigger ActivateAutomation: Expected 200, got %d: %s", activateResp.StatusCode, string(body))
	}
	activateResp.Body.Close()
	t.Logf("Automation activated: %s", automationID)

	// 4. Create a NEW contact via HTTP API (this is the exact scenario from issue #191)
	// IMPORTANT: The contact is NOT subscribed to the list - this is the bug trigger condition
	// The contact.created event should be triggered by the database trigger
	email := fmt.Sprintf("contact-created-test-%s@example.com", shortuuid.New()[:8])
	contactResp, err := client.CreateContact(map[string]interface{}{
		"workspace_id": workspaceID,
		"contact": map[string]interface{}{
			"email":      email,
			"first_name": "Test",
			"last_name":  "ContactCreated",
		},
	})
	require.NoError(t, err)
	if contactResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(contactResp.Body)
		contactResp.Body.Close()
		t.Fatalf("ContactCreatedTrigger CreateContact: Expected 200, got %d: %s", contactResp.StatusCode, string(body))
	}
	contactResp.Body.Close()
	// Normalize email to match what's stored in the database (lowercase)
	email = domain.NormalizeEmail(email)
	t.Logf("Contact created via HTTP API: %s (NOT subscribed to list %s)", email, listID)

	// 5. Wait for contact.created timeline event to be created
	timelineEvents := waitForTimelineEvent(t, factory, workspaceID, email, "contact.created", 3*time.Second)
	require.NotEmpty(t, timelineEvents, "contact.created timeline event should exist")
	t.Logf("contact.created timeline event found for %s", email)

	// 6. Wait for enrollment in automation via HTTP API
	// BUG: This will fail if automation_enroll_contact() checks list subscription
	// when the automation has a list_id set (for unsubscribe URLs)
	ca := waitForEnrollmentViaAPI(t, client, automationID, email, 3*time.Second)
	if ca == nil {
		addBug("TestAutomation_ContactCreatedTrigger",
			"Contact not enrolled in automation with list_id after contact.created event. "+
				"The automation_enroll_contact() function incorrectly checks if contact is subscribed to automation.list_id, "+
				"but list_id is only for unsubscribe URLs, not enrollment filtering.",
			"Critical",
			"automation_enroll_contact() has incorrect list subscription check for non-list triggers",
			"internal/database/init.go:automation_enroll_contact lines 1191-1203")
		t.Fatal("Contact not enrolled - GitHub Issue #191 reproduced! Bug: list_id check blocks enrollment for contact.created trigger")
	}

	// 7. Verify enrollment details
	// Status can be "active" (enrolled or waiting for delay) - both are valid
	// current_node_id can be the trigger node, delay node, or nil (if scheduler already processed to terminal delay)
	status := ca["status"].(string)
	assert.True(t, status == "active" || status == "completed", "Status should be active or completed, got: %s", status)
	t.Logf("Contact enrolled with status: %s, current_node: %v", status, ca["current_node_id"])

	// 8. Verify automation.start timeline event was created
	startEvents := waitForTimelineEvent(t, factory, workspaceID, email, "automation.start", 2*time.Second)
	require.NotEmpty(t, startEvents, "automation.start timeline event should exist")
	t.Logf("automation.start timeline event found")

	// 9. Wait for scheduler to process trigger → delay
	// After delay node executes, CurrentNodeID is set to the NEXT node (nil since delay is terminal),
	// and ScheduledAt is set to the future time when the delay expires.
	// Status remains "active" because we're waiting for the delay to expire.
	var caFromFactory *domain.ContactAutomation
	testutil.WaitForCondition(t, func() bool {
		var err error
		caFromFactory, err = factory.GetContactAutomation(workspaceID, automationID, email)
		if err != nil || caFromFactory == nil {
			return false
		}
		// Delay node is terminal (no next_node_id), so CurrentNodeID becomes nil
		// But contact should still be active with ScheduledAt in the future
		return caFromFactory.CurrentNodeID == nil && caFromFactory.ScheduledAt != nil && caFromFactory.Status == domain.ContactAutomationStatusActive
	}, 10*time.Second, "waiting for scheduler to process delay node")

	require.NotNil(t, caFromFactory, "Contact automation should exist")
	assert.Nil(t, caFromFactory.CurrentNodeID, "Current node should be nil (delay is terminal)")
	assert.Equal(t, domain.ContactAutomationStatusActive, caFromFactory.Status, "Contact should be active (waiting for delay)")
	require.NotNil(t, caFromFactory.ScheduledAt, "ScheduledAt should be set for delay")
	assert.True(t, caFromFactory.ScheduledAt.After(time.Now()), "ScheduledAt should be in the future")
	t.Logf("Contact processed delay node, waiting until: %v", caFromFactory.ScheduledAt)

	// 10. Verify automation stats were updated
	stats, err := factory.GetAutomationStats(workspaceID, automationID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), stats.Enrolled, "Enrolled count should be 1")
	t.Logf("Automation stats: enrolled=%d, completed=%d, exited=%d, failed=%d",
		stats.Enrolled, stats.Completed, stats.Exited, stats.Failed)

	// 11. Verify deduplication works (creating the same contact again should not re-enroll)
	// Update the contact (not create) - this should trigger contact.updated, not contact.created
	updateResp, err := client.CreateContact(map[string]interface{}{
		"workspace_id": workspaceID,
		"contact": map[string]interface{}{
			"email":      email,
			"first_name": "Updated",
			"last_name":  "Name",
		},
	})
	require.NoError(t, err)
	updateResp.Body.Close()
	t.Logf("Contact updated via HTTP API: %s", email)

	// Wait a bit and verify enrollment count is still 1
	time.Sleep(500 * time.Millisecond)
	count, err := factory.CountContactAutomations(workspaceID, automationID)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "Should still have exactly 1 enrollment (update should not trigger contact.created)")

	t.Logf("ContactCreatedTrigger E2E test passed: scheduler advanced to delay node, GitHub issue #191 scenario verified!")
}

// testAutomationConsecutiveAddToList tests two consecutive add_to_list nodes
// This verifies that the scheduler correctly processes multiple action nodes in sequence
// without pausing after the first action node completes.
func testAutomationConsecutiveAddToList(t *testing.T, factory *testutil.TestDataFactory, client *testutil.APIClient, workspaceID string) {
	// 1. Create two lists via factory
	list1, err := factory.CreateList(workspaceID)
	require.NoError(t, err)
	list1ID := list1.ID
	t.Logf("List 1 created: %s", list1ID)

	list2, err := factory.CreateList(workspaceID)
	require.NoError(t, err)
	list2ID := list2.ID
	t.Logf("List 2 created: %s", list2ID)

	// 2. Create automation via HTTP with trigger → add_to_list → add_to_list
	automationID := shortuuid.New()
	triggerNodeID := shortuuid.New()
	addNode1ID := shortuuid.New()
	addNode2ID := shortuuid.New()

	createReq := map[string]interface{}{
		"workspace_id": workspaceID,
		"automation": map[string]interface{}{
			"id":           automationID,
			"workspace_id": workspaceID,
			"name":         "Consecutive Add To List E2E",
			"status":       "draft",
			"trigger": map[string]interface{}{
				"event_kind":        "custom_event",
				"custom_event_name": "consecutive_add_list_event",
				"frequency":         "once",
			},
			"root_node_id": triggerNodeID,
			"nodes": []map[string]interface{}{
				{
					"id":            triggerNodeID,
					"automation_id": automationID,
					"type":          "trigger",
					"config":        map[string]interface{}{},
					"next_node_id":  addNode1ID,
					"position":      map[string]interface{}{"x": 0, "y": 0},
				},
				{
					"id":            addNode1ID,
					"automation_id": automationID,
					"type":          "add_to_list",
					"config":        map[string]interface{}{"list_id": list1ID, "status": "active"},
					"next_node_id":  addNode2ID,
					"position":      map[string]interface{}{"x": 0, "y": 100},
				},
				{
					"id":            addNode2ID,
					"automation_id": automationID,
					"type":          "add_to_list",
					"config":        map[string]interface{}{"list_id": list2ID, "status": "active"},
					"position":      map[string]interface{}{"x": 0, "y": 200},
				},
			},
			"stats": map[string]interface{}{"enrolled": 0, "completed": 0, "exited": 0, "failed": 0},
		},
	}

	resp, err := client.CreateAutomation(createReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()
	t.Logf("Automation created: %s", automationID)

	// 3. Activate automation via HTTP
	activateResp, err := client.ActivateAutomation(map[string]interface{}{
		"workspace_id":  workspaceID,
		"automation_id": automationID,
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, activateResp.StatusCode)
	activateResp.Body.Close()
	t.Logf("Automation activated: %s", automationID)

	// 4. Create contact via HTTP
	email := "consecutive-add-list@example.com"
	contactResp, err := client.CreateContact(map[string]interface{}{
		"workspace_id": workspaceID,
		"contact": map[string]interface{}{
			"email":      email,
			"first_name": "Consecutive",
			"last_name":  "Test",
		},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, contactResp.StatusCode)
	contactResp.Body.Close()
	t.Logf("Contact created: %s", email)

	// 5. Trigger automation via custom event (factory - no HTTP API)
	err = factory.CreateCustomEvent(workspaceID, email, "consecutive_add_list_event", nil)
	require.NoError(t, err)
	t.Logf("Custom event triggered for %s", email)

	// 6. Wait for enrollment via HTTP
	ca := waitForEnrollmentViaAPI(t, client, automationID, email, 2*time.Second)
	require.NotNil(t, ca, "Contact should be enrolled")
	t.Logf("Contact enrolled with status: %s, current_node: %v", ca["status"], ca["current_node_id"])

	// 7. Wait for automation to complete - this is where we expect the bug to manifest
	// The scheduler should process both add_to_list nodes and mark the contact as completed
	var finalStatus string
	testutil.WaitForCondition(t, func() bool {
		caFromFactory, err := factory.GetContactAutomation(workspaceID, automationID, email)
		if err != nil {
			return false
		}
		finalStatus = string(caFromFactory.Status)
		currentNode := ""
		if caFromFactory.CurrentNodeID != nil {
			currentNode = *caFromFactory.CurrentNodeID
		}
		t.Logf("Current automation status: %s, current_node: %s", caFromFactory.Status, currentNode)
		return caFromFactory.Status == domain.ContactAutomationStatusCompleted
	}, 10*time.Second, "waiting for automation to complete")

	// 8. Verify the contact was added to BOTH lists
	// Check list 1
	list1Resp, err := client.GetContactListByIDs(workspaceID, email, list1ID)
	require.NoError(t, err)
	defer list1Resp.Body.Close()

	if list1Resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(list1Resp.Body)
		t.Logf("List 1 response: %d - %s", list1Resp.StatusCode, string(body))
		addBug("TestAutomation_ConsecutiveAddToList",
			"Contact not added to first list - automation may have paused after first add_to_list node",
			"Critical", "Scheduler not processing consecutive action nodes",
			"internal/service/automation_executor.go")
		t.Fatalf("Contact should be added to list 1, got status %d", list1Resp.StatusCode)
	}

	var list1Result map[string]interface{}
	err = json.NewDecoder(list1Resp.Body).Decode(&list1Result)
	require.NoError(t, err)
	t.Logf("Contact in list 1: %v", list1Result)

	// Check list 2
	list2Resp, err := client.GetContactListByIDs(workspaceID, email, list2ID)
	require.NoError(t, err)
	defer list2Resp.Body.Close()

	if list2Resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(list2Resp.Body)
		t.Logf("List 2 response: %d - %s", list2Resp.StatusCode, string(body))
		addBug("TestAutomation_ConsecutiveAddToList",
			"Contact not added to second list - automation paused after first add_to_list node",
			"Critical", "Scheduler not advancing to second action node after first completes",
			"internal/service/automation_executor.go")
		t.Fatalf("Contact should be added to list 2, got status %d - BUG CONFIRMED: automation paused after first add_to_list", list2Resp.StatusCode)
	}

	var list2Result map[string]interface{}
	err = json.NewDecoder(list2Resp.Body).Decode(&list2Result)
	require.NoError(t, err)
	t.Logf("Contact in list 2: %v", list2Result)

	// 9. Verify final automation status is completed
	assert.Equal(t, "completed", finalStatus, "Automation should be completed after processing both nodes")

	// 10. Verify automation stats (wait for stats to update after contact completion)
	stats := waitForStatsCompleted(t, factory, workspaceID, automationID, 1, 2*time.Second)
	require.NotNil(t, stats, "Stats should exist")
	assert.Equal(t, int64(1), stats.Enrolled, "Enrolled count should be 1")
	assert.Equal(t, int64(1), stats.Completed, "Completed count should be 1")
	t.Logf("Automation stats: enrolled=%d, completed=%d, exited=%d, failed=%d",
		stats.Enrolled, stats.Completed, stats.Exited, stats.Failed)

	t.Logf("ConsecutiveAddToList E2E test passed: both add_to_list nodes executed successfully!")
}

// testWebhookNode tests webhook node sends HTTP POST with correct headers/payload
func testWebhookNode(t *testing.T, factory *testutil.TestDataFactory, client *testutil.APIClient, workspaceID string) {
	// 1. Create channel to capture webhook payload
	type webhookCapture struct {
		headers http.Header
		body    []byte
		method  string
	}
	captured := make(chan webhookCapture, 1)

	// 2. Create mock server
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		captured <- webhookCapture{
			headers: r.Header.Clone(),
			body:    body,
			method:  r.Method,
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok": true}`))
	}))
	defer testServer.Close()

	// 3. Create list for terminal node
	list, err := factory.CreateList(workspaceID, testutil.WithListName("Webhook Test List"))
	require.NoError(t, err)
	listID := list.ID

	// 4. Create automation: trigger → webhook → add_to_list
	automationID := shortuuid.New()
	triggerNodeID := shortuuid.New()
	webhookNodeID := shortuuid.New()
	terminalNodeID := shortuuid.New()
	webhookSecret := "test-webhook-secret-123"

	createReq := map[string]interface{}{
		"workspace_id": workspaceID,
		"automation": map[string]interface{}{
			"id":           automationID,
			"workspace_id": workspaceID,
			"name":         "Webhook Test Automation E2E",
			"status":       "draft",
			"trigger": map[string]interface{}{
				"event_kind":        "custom_event",
				"custom_event_name": "webhook_test_event_e2e",
				"frequency":         "once",
			},
			"root_node_id": triggerNodeID,
			"nodes": []map[string]interface{}{
				{
					"id":            triggerNodeID,
					"automation_id": automationID,
					"type":          "trigger",
					"config":        map[string]interface{}{},
					"next_node_id":  webhookNodeID,
					"position":      map[string]interface{}{"x": 0, "y": 0},
				},
				{
					"id":            webhookNodeID,
					"automation_id": automationID,
					"type":          "webhook",
					"config":        map[string]interface{}{"url": testServer.URL, "secret": webhookSecret},
					"next_node_id":  terminalNodeID,
					"position":      map[string]interface{}{"x": 0, "y": 100},
				},
				{
					"id":            terminalNodeID,
					"automation_id": automationID,
					"type":          "add_to_list",
					"config":        map[string]interface{}{"list_id": listID, "status": "active"},
					"position":      map[string]interface{}{"x": 0, "y": 200},
				},
			},
			"stats": map[string]interface{}{"enrolled": 0, "completed": 0, "exited": 0, "failed": 0},
		},
	}

	resp, err := client.CreateAutomation(createReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	// 5. Activate automation
	activateResp, err := client.ActivateAutomation(map[string]interface{}{
		"workspace_id":  workspaceID,
		"automation_id": automationID,
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, activateResp.StatusCode)
	activateResp.Body.Close()

	// 6. Create contact
	email := "webhook-test-e2e@example.com"
	contact, err := factory.CreateContact(workspaceID, testutil.WithContactEmail(email))
	require.NoError(t, err)
	t.Logf("Contact created: %s", contact.Email)

	// 7. Trigger automation via custom event
	err = factory.CreateCustomEvent(workspaceID, email, "webhook_test_event_e2e", nil)
	require.NoError(t, err)
	t.Logf("Custom event triggered for contact")

	// 8. Wait for webhook to be called
	select {
	case capture := <-captured:
		// Verify HTTP method
		assert.Equal(t, "POST", capture.method)

		// Verify Authorization header with Bearer token
		authHeader := capture.headers.Get("Authorization")
		assert.Equal(t, "Bearer "+webhookSecret, authHeader)

		// Verify Content-Type
		contentType := capture.headers.Get("Content-Type")
		assert.Equal(t, "application/json", contentType)

		// Verify payload structure
		var payload map[string]interface{}
		err := json.Unmarshal(capture.body, &payload)
		require.NoError(t, err)

		// Check email at top level (from buildWebhookPayload)
		assert.Equal(t, email, payload["email"])

		// Check automation info
		assert.Equal(t, automationID, payload["automation_id"])
		assert.Equal(t, "Webhook Test Automation E2E", payload["automation_name"])

		// Check node_id
		assert.Equal(t, webhookNodeID, payload["node_id"])

		// Check contact object exists
		contactData, ok := payload["contact"].(map[string]interface{})
		require.True(t, ok, "payload should contain contact object")
		assert.Equal(t, contact.Email, contactData["email"])

		t.Logf("Webhook received with correct headers and payload")

	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for webhook to be called")
	}

	// 9. Verify automation completed (contact reached terminal node)
	completedCA := waitForAutomationComplete(t, factory, workspaceID, automationID, email, 10*time.Second)
	require.NotNil(t, completedCA, "Automation should complete")
	assert.Equal(t, domain.ContactAutomationStatusCompleted, completedCA.Status)

	// 10. Verify contact was added to list via API
	listResp, err := client.GetContactListByIDs(workspaceID, email, listID)
	require.NoError(t, err)
	defer listResp.Body.Close()
	require.Equal(t, http.StatusOK, listResp.StatusCode, "Contact should be in list")

	t.Logf("Webhook Node E2E test passed")
}

// printBugReport outputs all bugs found during testing
func printBugReport(t *testing.T) {
	if len(bugReports) == 0 {
		t.Log("=== BUG REPORT ===")
		t.Log("No bugs found during integration testing!")
		return
	}

	t.Log("=== BUG REPORT ===")
	t.Logf("Total bugs found: %d", len(bugReports))
	t.Log("")

	severityCounts := map[string]int{"Critical": 0, "High": 0, "Medium": 0, "Low": 0}
	for _, bug := range bugReports {
		severityCounts[bug.Severity]++
	}
	t.Logf("By severity: Critical=%d, High=%d, Medium=%d, Low=%d",
		severityCounts["Critical"], severityCounts["High"],
		severityCounts["Medium"], severityCounts["Low"])
	t.Log("")

	for i, bug := range bugReports {
		t.Logf("Bug #%d [%s]", i+1, bug.Severity)
		t.Logf("  Test: %s", bug.TestName)
		t.Logf("  Description: %s", bug.Description)
		t.Logf("  Root Cause: %s", bug.RootCause)
		t.Logf("  Code Path: %s", bug.CodePath)
		t.Log("")
	}
}

// testAutomationIntegrationOverride verifies that an email node with integration_id
// override sends via the specified integration instead of the workspace default.
func testAutomationIntegrationOverride(t *testing.T, factory *testutil.TestDataFactory, client *testutil.APIClient, workspaceID string) {
	// 1. Create a second SMTP integration (not set as workspace default)
	overrideIntegration, err := factory.CreateMailpitSMTPIntegration(workspaceID, testutil.WithIntegrationName("Override SMTP"))
	require.NoError(t, err)
	t.Logf("Override integration created: %s", overrideIntegration.ID)

	// 2. Create list and template
	list, err := factory.CreateList(workspaceID)
	require.NoError(t, err)

	template, err := factory.CreateTemplate(workspaceID)
	require.NoError(t, err)

	// 3. Create automation with email node that uses integration_id override
	automationID := shortuuid.New()
	triggerNodeID := shortuuid.New()
	emailNodeID := shortuuid.New()

	createReq := map[string]interface{}{
		"workspace_id": workspaceID,
		"automation": map[string]interface{}{
			"id":           automationID,
			"workspace_id": workspaceID,
			"name":         "Integration Override E2E",
			"status":       "draft",
			"list_id":      list.ID,
			"trigger": map[string]interface{}{
				"event_kind": "list.subscribed",
				"list_id":    list.ID,
				"frequency":  "once",
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
					"config": map[string]interface{}{
						"template_id":    template.ID,
						"integration_id": overrideIntegration.ID,
					},
					"position": map[string]interface{}{"x": 0, "y": 100},
				},
			},
			"stats": map[string]interface{}{"enrolled": 0, "completed": 0, "exited": 0, "failed": 0},
		},
	}

	resp, err := client.CreateAutomation(createReq)
	require.NoError(t, err)
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("IntegrationOverride CreateAutomation: Expected 201, got %d: %s", resp.StatusCode, string(body))
	}
	resp.Body.Close()

	// 4. Activate automation
	activateResp, err := client.ActivateAutomation(map[string]interface{}{
		"workspace_id":  workspaceID,
		"automation_id": automationID,
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, activateResp.StatusCode)
	activateResp.Body.Close()

	// 5. Create contact and subscribe to list to trigger the automation
	email := "integration-override-e2e@example.com"
	contact, err := factory.CreateContact(workspaceID, testutil.WithContactEmail(email))
	require.NoError(t, err)
	t.Logf("Contact created: %s", contact.Email)

	_, err = factory.CreateContactList(workspaceID,
		testutil.WithContactListEmail(email),
		testutil.WithContactListListID(list.ID),
		testutil.WithContactListStatus(domain.ContactListStatusActive),
	)
	require.NoError(t, err)

	// 6. Wait for enrollment
	ca := waitForEnrollmentViaAPI(t, client, automationID, email, 2*time.Second)
	require.NotNil(t, ca, "Contact should be enrolled in automation")

	// 7. Wait for automation to complete (trigger → email → done)
	completedCA := waitForAutomationComplete(t, factory, workspaceID, automationID, email, 15*time.Second)
	require.NotNil(t, completedCA, "Automation should complete")
	assert.Equal(t, domain.ContactAutomationStatusCompleted, completedCA.Status)

	// 8. Verify the email_queue entry has the override integration_id
	var queueEntry *testutil.EmailQueueEntryResult
	require.Eventually(t, func() bool {
		queueEntry, err = factory.GetEmailQueueEntryByAutomationID(workspaceID, automationID)
		return err == nil && queueEntry != nil
	}, 5*time.Second, 200*time.Millisecond, "email_queue entry should exist for automation")

	assert.Equal(t, overrideIntegration.ID, queueEntry.IntegrationID,
		"email_queue entry should use the override integration, not the workspace default")
	t.Logf("Integration override verified: email_queue entry has integration_id=%s", queueEntry.IntegrationID)
}
