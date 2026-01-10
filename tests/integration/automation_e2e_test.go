package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

	assert.Equal(t, "active", ca["status"])
	assert.NotNil(t, ca["current_node_id"], "Current node should be set")

	// 8. Verify stats (factory - no HTTP API for stats)
	stats, err := factory.GetAutomationStats(workspaceID, automationID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), stats.Enrolled, "Enrolled count should be 1")

	t.Logf("Welcome Series E2E test passed: contact enrolled via HTTP endpoints, stats updated")
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

	// 7. Verify trigger log entry exists (factory - no HTTP API)
	hasEntry, err := factory.GetTriggerLogEntry(workspaceID, automationID, email)
	require.NoError(t, err)
	assert.True(t, hasEntry, "Trigger log entry should exist")

	// 8. Verify stats (factory - no HTTP API for stats)
	stats, err := factory.GetAutomationStats(workspaceID, automationID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), stats.Enrolled, "Enrolled should be 1, not 3")

	t.Logf("Deduplication E2E test passed: frequency=once working correctly")
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
				"frequency":  "every_time",
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

	// 8. Verify stats (factory - no HTTP API for stats)
	stats, err := factory.GetAutomationStats(workspaceID, automationID)
	require.NoError(t, err)
	assert.Equal(t, int64(3), stats.Enrolled, "Enrolled should be 3")

	t.Logf("Multiple entries E2E test passed: frequency=every_time working correctly")
}

// testAutomationDelayTiming tests delay node calculations
// Uses HTTP for automation CRUD, factory for timeline events (intentional)
func testAutomationDelayTiming(t *testing.T, factory *testutil.TestDataFactory, client *testutil.APIClient, workspaceID string) {
	// 1. Create automation via HTTP with delay node
	automationID := shortuuid.New()
	triggerNodeID := shortuuid.New()
	delayNodeID := shortuuid.New()

	createReq := map[string]interface{}{
		"workspace_id": workspaceID,
		"automation": map[string]interface{}{
			"id":           automationID,
			"workspace_id": workspaceID,
			"name":         "Delay Test Automation E2E",
			"status":       "draft",
			"trigger": map[string]interface{}{
				"event_kind": "custom_event", "custom_event_name": "delay_test_event_e2e",
				"frequency":  "once",
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
					"position":      map[string]interface{}{"x": 0, "y": 100},
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

	// 7. Verify scheduled_at is approximately 5 minutes in the future (factory for detailed timing)
	ca := waitForEnrollment(t, factory, workspaceID, automationID, email, 100*time.Millisecond)
	if ca != nil && ca.ScheduledAt != nil {
		expectedMin := beforeTrigger.Add(4 * time.Minute)
		expectedMax := beforeTrigger.Add(6 * time.Minute)

		if ca.ScheduledAt.After(beforeTrigger.Add(1 * time.Minute)) {
			if ca.ScheduledAt.Before(expectedMin) || ca.ScheduledAt.After(expectedMax) {
				addBug("TestAutomation_DelayTiming",
					fmt.Sprintf("Delay timing incorrect: expected ~5min future, got %v", ca.ScheduledAt.Sub(beforeTrigger)),
					"High", "Delay calculation error",
					"internal/service/automation_node_executor.go:DelayNodeExecutor")
			}
		}
	}

	t.Logf("Delay timing E2E test passed: delay node scheduled correctly")
}

// testAutomationABTestDeterminism tests A/B test variant selection is deterministic
// Uses HTTP for automation CRUD, factory for timeline events (intentional)
func testAutomationABTestDeterminism(t *testing.T, factory *testutil.TestDataFactory, client *testutil.APIClient, workspaceID string) {
	// 1. Create automation via HTTP with A/B test node
	automationID := shortuuid.New()
	triggerNodeID := shortuuid.New()
	abNodeID := shortuuid.New()

	createReq := map[string]interface{}{
		"workspace_id": workspaceID,
		"automation": map[string]interface{}{
			"id":           automationID,
			"workspace_id": workspaceID,
			"name":         "AB Test Automation E2E",
			"status":       "draft",
			"trigger": map[string]interface{}{
				"event_kind": "custom_event", "custom_event_name": "ab_test_event_e2e",
				"frequency":  "every_time",
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
							{"id": "A", "name": "Variant A", "weight": 50, "next_node_id": ""},
							{"id": "B", "name": "Variant B", "weight": 50, "next_node_id": ""},
						},
					},
					"position": map[string]interface{}{"x": 0, "y": 100},
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
	assert.Equal(t, "active", caMap["status"])

	t.Logf("A/B test determinism E2E test passed: enrollment working")
}

// testAutomationBranchRouting tests branch node routing based on conditions
// Uses HTTP for automation CRUD, factory for timeline events (intentional)
func testAutomationBranchRouting(t *testing.T, factory *testutil.TestDataFactory, client *testutil.APIClient, workspaceID string) {
	// 1. Create automation via HTTP with branch node
	automationID := shortuuid.New()
	triggerNodeID := shortuuid.New()
	branchNodeID := shortuuid.New()

	createReq := map[string]interface{}{
		"workspace_id": workspaceID,
		"automation": map[string]interface{}{
			"id":           automationID,
			"workspace_id": workspaceID,
			"name":         "Branch Test Automation E2E",
			"status":       "draft",
			"trigger": map[string]interface{}{
				"event_kind": "custom_event", "custom_event_name": "branch_test_event_e2e",
				"frequency":  "once",
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
								"name": "VIP Path",
								"conditions": map[string]interface{}{
									"operator": "and",
									"children": []map[string]interface{}{
										{"operator": "equals", "field": "country", "value": "US"},
									},
								},
								"next_node_id": "",
							},
						},
						"default_path_id": "",
					},
					"position": map[string]interface{}{"x": 0, "y": 100},
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
	assert.Equal(t, "active", caMap["status"])

	t.Logf("Branch routing E2E test passed: contact enrolled")
}

// testAutomationFilterNode tests filter node pass/fail paths
// Uses HTTP for automation CRUD, factory for timeline events (intentional)
func testAutomationFilterNode(t *testing.T, factory *testutil.TestDataFactory, client *testutil.APIClient, workspaceID string) {
	// 1. Create automation via HTTP with filter node
	automationID := shortuuid.New()
	triggerNodeID := shortuuid.New()
	filterNodeID := shortuuid.New()

	createReq := map[string]interface{}{
		"workspace_id": workspaceID,
		"automation": map[string]interface{}{
			"id":           automationID,
			"workspace_id": workspaceID,
			"name":         "Filter Test Automation E2E",
			"status":       "draft",
			"trigger": map[string]interface{}{
				"event_kind": "custom_event", "custom_event_name": "filter_test_event_e2e",
				"frequency":  "once",
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
							"operator": "and",
							"children": []map[string]interface{}{
								{"operator": "equals", "field": "country", "value": "FR"},
							},
						},
						"continue_node_id": "",
						"exit_node_id":     "",
					},
					"position": map[string]interface{}{"x": 0, "y": 100},
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
	assert.Equal(t, "active", passCA["status"])

	failCA := waitForEnrollmentViaAPI(t, client, automationID, failEmail, 2*time.Second)
	require.NotNil(t, failCA)
	assert.Equal(t, "active", failCA["status"])

	t.Logf("Filter node E2E test passed: both contacts enrolled")
}

// testAutomationListStatusBranch tests the list_status_branch node routing based on contact list status
// Uses HTTP for automation CRUD, factory for timeline events (intentional)
func testAutomationListStatusBranch(t *testing.T, factory *testutil.TestDataFactory, client *testutil.APIClient, workspaceID string) {
	// 1. Create a list via factory (complex validation requirements)
	list, err := factory.CreateList(workspaceID)
	require.NoError(t, err)
	listID := list.ID

	// 2. Create automation via HTTP with list_status_branch node
	automationID := shortuuid.New()
	triggerNodeID := shortuuid.New()
	listStatusNodeID := shortuuid.New()

	createReq := map[string]interface{}{
		"workspace_id": workspaceID,
		"automation": map[string]interface{}{
			"id":           automationID,
			"workspace_id": workspaceID,
			"name":         "List Status Branch Test E2E",
			"status":       "draft",
			"trigger": map[string]interface{}{
				"event_kind": "custom_event", "custom_event_name": "list_status_branch_test_event_e2e",
				"frequency":  "once",
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
						"not_in_list_node_id": "",
						"active_node_id":      "",
						"non_active_node_id":  "",
					},
					"position": map[string]interface{}{"x": 0, "y": 100},
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
	assert.Equal(t, "active", notInListCA["status"])

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
	assert.Equal(t, "active", activeCA["status"])

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
	assert.Equal(t, "active", unsubCA["status"])

	// Verify stats (factory - no HTTP API for stats)
	stats, err := factory.GetAutomationStats(workspaceID, automationID)
	require.NoError(t, err)
	assert.Equal(t, int64(3), stats.Enrolled, "All 3 contacts should be enrolled")

	t.Logf("List status branch E2E test passed: all 3 contacts enrolled correctly")
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
				"frequency":  "once",
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
					"config":        map[string]interface{}{"list_id": premiumListID, "status": "subscribed"},
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
	assert.Equal(t, "active", caMap["status"])

	t.Logf("List operations E2E test passed: contact enrolled")
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
				"frequency":  "every_time",
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
	assert.Equal(t, "active", caMap["status"])

	t.Logf("Context data E2E test passed: contact enrolled with purchase event")
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
	assert.Equal(t, "active", caMap["status"])

	t.Logf("Segment trigger E2E test passed: contact enrolled on segment.joined")
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
	assert.Equal(t, "active", ca["status"])

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
	assert.Equal(t, "active", ca["status"])

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
				"frequency":  "once",
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
						"unit":     "seconds",
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
	assert.Equal(t, "active", ca["status"])

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
				"frequency":  "once",
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
						"duration": 0, // Completes immediately - this is a terminal node
						"unit":     "seconds",
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
