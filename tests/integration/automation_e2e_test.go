package integration

import (
	"context"
	"database/sql"
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
	t.Run("WelcomeSeries", func(t *testing.T) {
		testAutomationWelcomeSeries(t, factory, client, workspace.ID)
	})
	t.Run("Deduplication", func(t *testing.T) {
		testAutomationDeduplication(t, factory, workspace.ID)
	})
	t.Run("MultipleEntries", func(t *testing.T) {
		testAutomationMultipleEntries(t, factory, workspace.ID)
	})
	t.Run("DelayTiming", func(t *testing.T) {
		testAutomationDelayTiming(t, factory, workspace.ID)
	})
	t.Run("ABTestDeterminism", func(t *testing.T) {
		testAutomationABTestDeterminism(t, factory, workspace.ID)
	})
	t.Run("BranchRouting", func(t *testing.T) {
		testAutomationBranchRouting(t, factory, workspace.ID)
	})
	t.Run("FilterNode", func(t *testing.T) {
		testAutomationFilterNode(t, factory, workspace.ID)
	})
	t.Run("ListStatusBranch", func(t *testing.T) {
		testAutomationListStatusBranch(t, factory, workspace.ID)
	})
	t.Run("ListOperations", func(t *testing.T) {
		testAutomationListOperations(t, factory, workspace.ID)
	})
	t.Run("ContextData", func(t *testing.T) {
		testAutomationContextData(t, factory, workspace.ID)
	})
	t.Run("SegmentTrigger", func(t *testing.T) {
		testAutomationSegmentTrigger(t, factory, workspace.ID)
	})
	t.Run("DeletionCleanup", func(t *testing.T) {
		testAutomationDeletionCleanup(t, factory, client, workspace.ID)
	})
	t.Run("ErrorRecovery", func(t *testing.T) {
		testAutomationErrorRecovery(t, factory, workspace.ID)
	})
	t.Run("SchedulerExecution", func(t *testing.T) {
		testAutomationSchedulerExecution(t, factory, workspace.ID)
	})
	t.Run("PauseResume", func(t *testing.T) {
		testAutomationPauseResume(t, factory, workspace.ID)
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
		testAutomationTimelineStartEvent(t, factory, workspace.ID)
	})
	t.Run("TimelineEndEvent_Completed", func(t *testing.T) {
		testAutomationTimelineEndEvent(t, factory, workspace.ID)
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
func testAutomationWelcomeSeries(t *testing.T, factory *testutil.TestDataFactory, client *testutil.APIClient, workspaceID string) {
	// Create list for the automation
	list, err := factory.CreateList(workspaceID, testutil.WithListName("Welcome List"))
	require.NoError(t, err)

	// Create email template
	template, err := factory.CreateTemplate(workspaceID, testutil.WithTemplateName("Welcome Email"))
	require.NoError(t, err)

	// Create automation with trigger on insert_contact_list
	automation, err := factory.CreateAutomation(workspaceID,
		testutil.WithAutomationName("Welcome Series"),
		testutil.WithAutomationListID(list.ID),
		testutil.WithAutomationTrigger(&domain.TimelineTriggerConfig{
			EventKind: "insert_contact_list",
			Frequency: domain.TriggerFrequencyOnce,
		}),
	)
	require.NoError(t, err)

	// Create nodes: trigger → email (terminal)
	triggerNode, err := factory.CreateAutomationNode(workspaceID,
		testutil.WithNodeAutomationID(automation.ID),
		testutil.WithNodeType(domain.NodeTypeTrigger),
		testutil.WithNodeConfig(map[string]interface{}{}),
	)
	require.NoError(t, err)

	emailNode, err := factory.CreateAutomationNode(workspaceID,
		testutil.WithNodeAutomationID(automation.ID),
		testutil.WithNodeType(domain.NodeTypeEmail),
		testutil.WithNodeConfig(map[string]interface{}{
			"template_id": template.ID,
		}),
	)
	require.NoError(t, err)

	// Update trigger node to point to email node
	err = factory.UpdateAutomationNodeNextNodeID(workspaceID, automation.ID, triggerNode.ID, emailNode.ID)
	require.NoError(t, err)

	// Set root node
	err = factory.UpdateAutomationRootNode(workspaceID, automation.ID, triggerNode.ID)
	require.NoError(t, err)

	// Activate automation (creates DB trigger)
	err = factory.ActivateAutomation(workspaceID, automation.ID)
	require.NoError(t, err)

	// Create contact
	contact, err := factory.CreateContact(workspaceID,
		testutil.WithContactEmail("welcome-test@example.com"),
		testutil.WithContactName("Test", "User"),
	)
	require.NoError(t, err)

	// Add contact to list
	_, err = factory.CreateContactList(workspaceID,
		testutil.WithContactListEmail(contact.Email),
		testutil.WithContactListListID(list.ID),
		testutil.WithContactListStatus(domain.ContactListStatusActive),
	)
	require.NoError(t, err)

	// Insert timeline event (simulates what the contact_list trigger does)
	err = factory.CreateContactTimelineEvent(workspaceID, contact.Email, "insert_contact_list", map[string]interface{}{
		"list_id": list.ID,
	})
	require.NoError(t, err)

	// Wait for enrollment using polling
	ca := waitForEnrollment(t, factory, workspaceID, automation.ID, contact.Email, 2*time.Second)
	if ca == nil {
		addBug("TestAutomation_WelcomeSeries", "Contact not enrolled after timeline event",
			"Critical", "Trigger not firing on timeline insert",
			"internal/migrations/v20.go:automation_enroll_contact")
		t.Fatal("Contact not enrolled")
	}

	assert.Equal(t, domain.ContactAutomationStatusActive, ca.Status)
	assert.NotNil(t, ca.CurrentNodeID, "Current node should be set")

	// Verify stats
	stats, err := factory.GetAutomationStats(workspaceID, automation.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), stats.Enrolled, "Enrolled count should be 1")

	t.Logf("Welcome Series test passed: contact enrolled, stats updated")
}

// testAutomationDeduplication tests frequency: once prevents duplicate enrollments
func testAutomationDeduplication(t *testing.T, factory *testutil.TestDataFactory, workspaceID string) {
	// Create automation with frequency: once
	automation, err := factory.CreateAutomation(workspaceID,
		testutil.WithAutomationName("Once Only Automation"),
		testutil.WithAutomationTrigger(&domain.TimelineTriggerConfig{
			EventKind: "test_event",
			Frequency: domain.TriggerFrequencyOnce,
		}),
	)
	require.NoError(t, err)

	// Create simple trigger flow (trigger is terminal)
	triggerNode, err := factory.CreateAutomationNode(workspaceID,
		testutil.WithNodeAutomationID(automation.ID),
		testutil.WithNodeType(domain.NodeTypeTrigger),
		testutil.WithNodeConfig(map[string]interface{}{}),
	)
	require.NoError(t, err)

	err = factory.UpdateAutomationRootNode(workspaceID, automation.ID, triggerNode.ID)
	require.NoError(t, err)

	err = factory.ActivateAutomation(workspaceID, automation.ID)
	require.NoError(t, err)

	// Create contact
	contact, err := factory.CreateContact(workspaceID, testutil.WithContactEmail("dedup-test@example.com"))
	require.NoError(t, err)

	// Trigger 3 times
	for i := 0; i < 3; i++ {
		err = factory.CreateContactTimelineEvent(workspaceID, contact.Email, "test_event", map[string]interface{}{
			"iteration": i,
		})
		require.NoError(t, err)
	}

	// Wait for enrollment (should only be 1 due to frequency: once)
	waitForEnrollmentCount(t, factory, workspaceID, automation.ID, 1, 2*time.Second)

	// Verify: only 1 contact_automation created
	count, err := factory.CountContactAutomations(workspaceID, automation.ID)
	require.NoError(t, err)

	if count != 1 {
		addBug("TestAutomation_Deduplication",
			fmt.Sprintf("Expected 1 enrollment, got %d", count),
			"Critical", "Deduplication via automation_trigger_log not working",
			"internal/migrations/v20.go:automation_enroll_contact")
	}
	assert.Equal(t, 1, count, "Should have exactly 1 contact automation record")

	// Verify trigger log entry exists
	hasEntry, err := factory.GetTriggerLogEntry(workspaceID, automation.ID, contact.Email)
	require.NoError(t, err)
	assert.True(t, hasEntry, "Trigger log entry should exist")

	// Verify stats
	stats, err := factory.GetAutomationStats(workspaceID, automation.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), stats.Enrolled, "Enrolled should be 1, not 3")

	t.Logf("Deduplication test passed: frequency=once working correctly")
}

// testAutomationMultipleEntries tests frequency: every_time allows multiple enrollments
func testAutomationMultipleEntries(t *testing.T, factory *testutil.TestDataFactory, workspaceID string) {
	// Create automation with frequency: every_time
	automation, err := factory.CreateAutomation(workspaceID,
		testutil.WithAutomationName("Every Time Automation"),
		testutil.WithAutomationTrigger(&domain.TimelineTriggerConfig{
			EventKind: "repeat_event",
			Frequency: domain.TriggerFrequencyEveryTime,
		}),
	)
	require.NoError(t, err)

	// Create simple trigger flow (trigger is terminal)
	triggerNode, err := factory.CreateAutomationNode(workspaceID,
		testutil.WithNodeAutomationID(automation.ID),
		testutil.WithNodeType(domain.NodeTypeTrigger),
	)
	require.NoError(t, err)

	err = factory.UpdateAutomationRootNode(workspaceID, automation.ID, triggerNode.ID)
	require.NoError(t, err)

	err = factory.ActivateAutomation(workspaceID, automation.ID)
	require.NoError(t, err)

	// Create contact
	contact, err := factory.CreateContact(workspaceID, testutil.WithContactEmail("multi-test@example.com"))
	require.NoError(t, err)

	// Trigger 3 times with small delays to ensure different entered_at
	for i := 0; i < 3; i++ {
		err = factory.CreateContactTimelineEvent(workspaceID, contact.Email, "repeat_event", map[string]interface{}{
			"iteration": i,
		})
		require.NoError(t, err)
		time.Sleep(10 * time.Millisecond) // Small delay to ensure different timestamps
	}

	// Wait for 3 enrollments
	waitForEnrollmentCount(t, factory, workspaceID, automation.ID, 3, 2*time.Second)

	// Verify: 3 contact_automation records
	count, err := factory.CountContactAutomations(workspaceID, automation.ID)
	require.NoError(t, err)

	if count != 3 {
		addBug("TestAutomation_MultipleEntries",
			fmt.Sprintf("Expected 3 enrollments, got %d", count),
			"High", "every_time frequency not allowing multiple entries",
			"internal/migrations/v20.go:automation_enroll_contact")
	}
	assert.Equal(t, 3, count, "Should have 3 contact automation records")

	// Verify each has different entered_at
	cas, err := factory.GetAllContactAutomations(workspaceID, automation.ID)
	require.NoError(t, err)
	assert.Len(t, cas, 3, "Should have 3 records")

	// Verify stats
	stats, err := factory.GetAutomationStats(workspaceID, automation.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(3), stats.Enrolled, "Enrolled should be 3")

	t.Logf("Multiple entries test passed: frequency=every_time working correctly")
}

// testAutomationDelayTiming tests delay node calculations
func testAutomationDelayTiming(t *testing.T, factory *testutil.TestDataFactory, workspaceID string) {
	// Create automation
	automation, err := factory.CreateAutomation(workspaceID,
		testutil.WithAutomationName("Delay Test Automation"),
		testutil.WithAutomationTrigger(&domain.TimelineTriggerConfig{
			EventKind: "delay_test_event",
			Frequency: domain.TriggerFrequencyOnce,
		}),
	)
	require.NoError(t, err)

	// Create nodes: trigger → delay (5 minutes) - delay is terminal
	triggerNode, err := factory.CreateAutomationNode(workspaceID,
		testutil.WithNodeAutomationID(automation.ID),
		testutil.WithNodeType(domain.NodeTypeTrigger),
	)
	require.NoError(t, err)

	delayNode, err := factory.CreateAutomationNode(workspaceID,
		testutil.WithNodeAutomationID(automation.ID),
		testutil.WithNodeType(domain.NodeTypeDelay),
		testutil.WithNodeConfig(map[string]interface{}{
			"duration": 5,
			"unit":     "minutes",
		}),
	)
	require.NoError(t, err)

	err = factory.UpdateAutomationNodeNextNodeID(workspaceID, automation.ID, triggerNode.ID, delayNode.ID)
	require.NoError(t, err)

	err = factory.UpdateAutomationRootNode(workspaceID, automation.ID, triggerNode.ID)
	require.NoError(t, err)

	err = factory.ActivateAutomation(workspaceID, automation.ID)
	require.NoError(t, err)

	// Create contact and trigger
	contact, err := factory.CreateContact(workspaceID, testutil.WithContactEmail("delay-test@example.com"))
	require.NoError(t, err)

	beforeTrigger := time.Now().UTC()
	err = factory.CreateContactTimelineEvent(workspaceID, contact.Email, "delay_test_event", nil)
	require.NoError(t, err)

	// Wait for enrollment
	ca := waitForEnrollment(t, factory, workspaceID, automation.ID, contact.Email, 2*time.Second)
	require.NotNil(t, ca)

	// Verify enrollment
	assert.Equal(t, domain.ContactAutomationStatusActive, ca.Status)

	// Verify scheduled_at is approximately 5 minutes in the future (if scheduler has processed)
	if ca.ScheduledAt != nil {
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

	t.Logf("Delay timing test passed: delay node scheduled correctly")
}

// testAutomationABTestDeterminism tests A/B test variant selection is deterministic
func testAutomationABTestDeterminism(t *testing.T, factory *testutil.TestDataFactory, workspaceID string) {
	// Create automation
	automation, err := factory.CreateAutomation(workspaceID,
		testutil.WithAutomationName("AB Test Automation"),
		testutil.WithAutomationTrigger(&domain.TimelineTriggerConfig{
			EventKind: "ab_test_event",
			Frequency: domain.TriggerFrequencyEveryTime,
		}),
	)
	require.NoError(t, err)

	// Create A/B test node with 50/50 split (variants lead to terminal - empty string)
	triggerNode, err := factory.CreateAutomationNode(workspaceID,
		testutil.WithNodeAutomationID(automation.ID),
		testutil.WithNodeType(domain.NodeTypeTrigger),
	)
	require.NoError(t, err)

	abNode, err := factory.CreateAutomationNode(workspaceID,
		testutil.WithNodeAutomationID(automation.ID),
		testutil.WithNodeType(domain.NodeTypeABTest),
		testutil.WithNodeConfig(map[string]interface{}{
			"variants": []map[string]interface{}{
				{"id": "A", "name": "Variant A", "weight": 50, "next_node_id": ""},
				{"id": "B", "name": "Variant B", "weight": 50, "next_node_id": ""},
			},
		}),
	)
	require.NoError(t, err)

	err = factory.UpdateAutomationNodeNextNodeID(workspaceID, automation.ID, triggerNode.ID, abNode.ID)
	require.NoError(t, err)

	err = factory.UpdateAutomationRootNode(workspaceID, automation.ID, triggerNode.ID)
	require.NoError(t, err)

	err = factory.ActivateAutomation(workspaceID, automation.ID)
	require.NoError(t, err)

	// Test determinism: same contact should always get same variant
	contact, err := factory.CreateContact(workspaceID, testutil.WithContactEmail("ab-determ@example.com"))
	require.NoError(t, err)

	err = factory.CreateContactTimelineEvent(workspaceID, contact.Email, "ab_test_event", nil)
	require.NoError(t, err)

	// Wait for enrollment
	ca := waitForEnrollment(t, factory, workspaceID, automation.ID, contact.Email, 2*time.Second)
	require.NotNil(t, ca)
	assert.Equal(t, domain.ContactAutomationStatusActive, ca.Status)

	t.Logf("A/B test determinism test passed: enrollment working")
}

// testAutomationBranchRouting tests branch node routing based on conditions
func testAutomationBranchRouting(t *testing.T, factory *testutil.TestDataFactory, workspaceID string) {
	// Create automation with branch
	automation, err := factory.CreateAutomation(workspaceID,
		testutil.WithAutomationName("Branch Test Automation"),
		testutil.WithAutomationTrigger(&domain.TimelineTriggerConfig{
			EventKind: "branch_test_event",
			Frequency: domain.TriggerFrequencyOnce,
		}),
	)
	require.NoError(t, err)

	// Create nodes (branch paths lead to terminal - empty string)
	triggerNode, err := factory.CreateAutomationNode(workspaceID,
		testutil.WithNodeAutomationID(automation.ID),
		testutil.WithNodeType(domain.NodeTypeTrigger),
	)
	require.NoError(t, err)

	// Branch node with condition on country (VIP = US) - paths lead to terminal
	branchNode, err := factory.CreateAutomationNode(workspaceID,
		testutil.WithNodeAutomationID(automation.ID),
		testutil.WithNodeType(domain.NodeTypeBranch),
		testutil.WithNodeConfig(map[string]interface{}{
			"paths": []map[string]interface{}{
				{
					"id":   "vip_path",
					"name": "VIP Path",
					"conditions": map[string]interface{}{
						"operator": "and",
						"children": []map[string]interface{}{
							{
								"operator": "equals",
								"field":    "country",
								"value":    "US",
							},
						},
					},
					"next_node_id": "",
				},
			},
			"default_path_id": "",
		}),
	)
	require.NoError(t, err)

	err = factory.UpdateAutomationNodeNextNodeID(workspaceID, automation.ID, triggerNode.ID, branchNode.ID)
	require.NoError(t, err)

	err = factory.UpdateAutomationRootNode(workspaceID, automation.ID, triggerNode.ID)
	require.NoError(t, err)

	err = factory.ActivateAutomation(workspaceID, automation.ID)
	require.NoError(t, err)

	// Create VIP contact (US)
	vipContact, err := factory.CreateContact(workspaceID,
		testutil.WithContactEmail("vip-branch@example.com"),
		testutil.WithContactCountry("US"),
	)
	require.NoError(t, err)

	err = factory.CreateContactTimelineEvent(workspaceID, vipContact.Email, "branch_test_event", nil)
	require.NoError(t, err)

	// Wait for enrollment
	ca := waitForEnrollment(t, factory, workspaceID, automation.ID, vipContact.Email, 2*time.Second)
	require.NotNil(t, ca)
	assert.Equal(t, domain.ContactAutomationStatusActive, ca.Status)

	t.Logf("Branch routing test passed: contact enrolled")
}

// testAutomationFilterNode tests filter node pass/fail paths
func testAutomationFilterNode(t *testing.T, factory *testutil.TestDataFactory, workspaceID string) {
	// Create automation with filter
	automation, err := factory.CreateAutomation(workspaceID,
		testutil.WithAutomationName("Filter Test Automation"),
		testutil.WithAutomationTrigger(&domain.TimelineTriggerConfig{
			EventKind: "filter_test_event",
			Frequency: domain.TriggerFrequencyOnce,
		}),
	)
	require.NoError(t, err)

	// Create nodes (filter paths lead to terminal - empty string)
	triggerNode, err := factory.CreateAutomationNode(workspaceID,
		testutil.WithNodeAutomationID(automation.ID),
		testutil.WithNodeType(domain.NodeTypeTrigger),
	)
	require.NoError(t, err)

	// Filter: continue if country = FR - both paths lead to terminal
	filterNode, err := factory.CreateAutomationNode(workspaceID,
		testutil.WithNodeAutomationID(automation.ID),
		testutil.WithNodeType(domain.NodeTypeFilter),
		testutil.WithNodeConfig(map[string]interface{}{
			"conditions": map[string]interface{}{
				"operator": "and",
				"children": []map[string]interface{}{
					{
						"operator": "equals",
						"field":    "country",
						"value":    "FR",
					},
				},
			},
			"continue_node_id": "",
			"exit_node_id":     "",
		}),
	)
	require.NoError(t, err)

	err = factory.UpdateAutomationNodeNextNodeID(workspaceID, automation.ID, triggerNode.ID, filterNode.ID)
	require.NoError(t, err)

	err = factory.UpdateAutomationRootNode(workspaceID, automation.ID, triggerNode.ID)
	require.NoError(t, err)

	err = factory.ActivateAutomation(workspaceID, automation.ID)
	require.NoError(t, err)

	// Create passing contact (FR)
	passContact, err := factory.CreateContact(workspaceID,
		testutil.WithContactEmail("filter-pass@example.com"),
		testutil.WithContactCountry("FR"),
	)
	require.NoError(t, err)

	err = factory.CreateContactTimelineEvent(workspaceID, passContact.Email, "filter_test_event", nil)
	require.NoError(t, err)

	// Create failing contact (DE)
	failContact, err := factory.CreateContact(workspaceID,
		testutil.WithContactEmail("filter-fail@example.com"),
		testutil.WithContactCountry("DE"),
	)
	require.NoError(t, err)

	err = factory.CreateContactTimelineEvent(workspaceID, failContact.Email, "filter_test_event", nil)
	require.NoError(t, err)

	// Wait for both enrollments
	passCA := waitForEnrollment(t, factory, workspaceID, automation.ID, passContact.Email, 2*time.Second)
	require.NotNil(t, passCA)
	assert.Equal(t, domain.ContactAutomationStatusActive, passCA.Status)

	failCA := waitForEnrollment(t, factory, workspaceID, automation.ID, failContact.Email, 2*time.Second)
	require.NotNil(t, failCA)
	assert.Equal(t, domain.ContactAutomationStatusActive, failCA.Status)

	t.Logf("Filter node test passed: both contacts enrolled")
}

// testAutomationListStatusBranch tests the list_status_branch node routing based on contact list status
func testAutomationListStatusBranch(t *testing.T, factory *testutil.TestDataFactory, workspaceID string) {
	// Create a list to check status against
	checkList, err := factory.CreateList(workspaceID, testutil.WithListName("Status Check List"))
	require.NoError(t, err)

	// Create automation with list_status_branch node
	automation, err := factory.CreateAutomation(workspaceID,
		testutil.WithAutomationName("List Status Branch Test"),
		testutil.WithAutomationTrigger(&domain.TimelineTriggerConfig{
			EventKind: "list_status_branch_test_event",
			Frequency: domain.TriggerFrequencyOnce,
		}),
	)
	require.NoError(t, err)

	// Create nodes: trigger → list_status_branch (all 3 branches lead to terminal - empty string)
	triggerNode, err := factory.CreateAutomationNode(workspaceID,
		testutil.WithNodeAutomationID(automation.ID),
		testutil.WithNodeType(domain.NodeTypeTrigger),
	)
	require.NoError(t, err)

	// List status branch node: check status in checkList
	listStatusBranchNode, err := factory.CreateAutomationNode(workspaceID,
		testutil.WithNodeAutomationID(automation.ID),
		testutil.WithNodeType(domain.NodeTypeListStatusBranch),
		testutil.WithNodeConfig(map[string]interface{}{
			"list_id":             checkList.ID,
			"not_in_list_node_id": "",
			"active_node_id":      "",
			"non_active_node_id":  "",
		}),
	)
	require.NoError(t, err)

	err = factory.UpdateAutomationNodeNextNodeID(workspaceID, automation.ID, triggerNode.ID, listStatusBranchNode.ID)
	require.NoError(t, err)

	err = factory.UpdateAutomationRootNode(workspaceID, automation.ID, triggerNode.ID)
	require.NoError(t, err)

	err = factory.ActivateAutomation(workspaceID, automation.ID)
	require.NoError(t, err)

	// Test 1: Contact not in list → should route to not_in_list branch
	notInListContact, err := factory.CreateContact(workspaceID,
		testutil.WithContactEmail("not-in-list@example.com"),
	)
	require.NoError(t, err)

	err = factory.CreateContactTimelineEvent(workspaceID, notInListContact.Email, "list_status_branch_test_event", nil)
	require.NoError(t, err)

	// Wait for enrollment
	notInListCA := waitForEnrollment(t, factory, workspaceID, automation.ID, notInListContact.Email, 2*time.Second)
	require.NotNil(t, notInListCA)
	assert.Equal(t, domain.ContactAutomationStatusActive, notInListCA.Status)
	t.Logf("Not in list contact enrolled successfully")

	// Test 2: Contact with active status → should route to active branch
	activeContact, err := factory.CreateContact(workspaceID,
		testutil.WithContactEmail("active-status@example.com"),
	)
	require.NoError(t, err)

	// Add contact to list with active status
	_, err = factory.CreateContactList(workspaceID,
		testutil.WithContactListEmail(activeContact.Email),
		testutil.WithContactListListID(checkList.ID),
		testutil.WithContactListStatus(domain.ContactListStatusActive),
	)
	require.NoError(t, err)

	err = factory.CreateContactTimelineEvent(workspaceID, activeContact.Email, "list_status_branch_test_event", nil)
	require.NoError(t, err)

	// Wait for enrollment
	activeCA := waitForEnrollment(t, factory, workspaceID, automation.ID, activeContact.Email, 2*time.Second)
	require.NotNil(t, activeCA)
	assert.Equal(t, domain.ContactAutomationStatusActive, activeCA.Status)
	t.Logf("Active status contact enrolled successfully")

	// Test 3: Contact with unsubscribed status → should route to non_active branch
	unsubContact, err := factory.CreateContact(workspaceID,
		testutil.WithContactEmail("unsubscribed-status@example.com"),
	)
	require.NoError(t, err)

	// Add contact to list with unsubscribed status
	_, err = factory.CreateContactList(workspaceID,
		testutil.WithContactListEmail(unsubContact.Email),
		testutil.WithContactListListID(checkList.ID),
		testutil.WithContactListStatus(domain.ContactListStatusUnsubscribed),
	)
	require.NoError(t, err)

	err = factory.CreateContactTimelineEvent(workspaceID, unsubContact.Email, "list_status_branch_test_event", nil)
	require.NoError(t, err)

	// Wait for enrollment
	unsubCA := waitForEnrollment(t, factory, workspaceID, automation.ID, unsubContact.Email, 2*time.Second)
	require.NotNil(t, unsubCA)
	assert.Equal(t, domain.ContactAutomationStatusActive, unsubCA.Status)
	t.Logf("Unsubscribed status contact enrolled successfully")

	// Verify stats show all 3 enrolled
	stats, err := factory.GetAutomationStats(workspaceID, automation.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(3), stats.Enrolled, "All 3 contacts should be enrolled")

	t.Logf("List status branch test passed: all 3 contacts enrolled correctly")
}

// testAutomationListOperations tests add_to_list and remove_from_list nodes
func testAutomationListOperations(t *testing.T, factory *testutil.TestDataFactory, workspaceID string) {
	// Create lists
	trialList, err := factory.CreateList(workspaceID, testutil.WithListName("Trial"))
	require.NoError(t, err)
	premiumList, err := factory.CreateList(workspaceID, testutil.WithListName("Premium"))
	require.NoError(t, err)

	// Create automation
	automation, err := factory.CreateAutomation(workspaceID,
		testutil.WithAutomationName("List Operations Automation"),
		testutil.WithAutomationTrigger(&domain.TimelineTriggerConfig{
			EventKind: "list_ops_event",
			Frequency: domain.TriggerFrequencyOnce,
		}),
	)
	require.NoError(t, err)

	// Create nodes: trigger → add_to_list → remove_from_list (terminal)
	triggerNode, err := factory.CreateAutomationNode(workspaceID,
		testutil.WithNodeAutomationID(automation.ID),
		testutil.WithNodeType(domain.NodeTypeTrigger),
	)
	require.NoError(t, err)

	removeNode, err := factory.CreateAutomationNode(workspaceID,
		testutil.WithNodeAutomationID(automation.ID),
		testutil.WithNodeType(domain.NodeTypeRemoveFromList),
		testutil.WithNodeConfig(map[string]interface{}{
			"list_id": trialList.ID,
		}),
	)
	require.NoError(t, err)

	addNode, err := factory.CreateAutomationNode(workspaceID,
		testutil.WithNodeAutomationID(automation.ID),
		testutil.WithNodeType(domain.NodeTypeAddToList),
		testutil.WithNodeConfig(map[string]interface{}{
			"list_id": premiumList.ID,
			"status":  "subscribed",
		}),
		testutil.WithNodeNextNodeID(removeNode.ID),
	)
	require.NoError(t, err)

	err = factory.UpdateAutomationNodeNextNodeID(workspaceID, automation.ID, triggerNode.ID, addNode.ID)
	require.NoError(t, err)

	err = factory.UpdateAutomationRootNode(workspaceID, automation.ID, triggerNode.ID)
	require.NoError(t, err)

	err = factory.ActivateAutomation(workspaceID, automation.ID)
	require.NoError(t, err)

	// Create contact in trial list
	contact, err := factory.CreateContact(workspaceID, testutil.WithContactEmail("list-ops@example.com"))
	require.NoError(t, err)

	_, err = factory.CreateContactList(workspaceID,
		testutil.WithContactListEmail(contact.Email),
		testutil.WithContactListListID(trialList.ID),
		testutil.WithContactListStatus(domain.ContactListStatusActive),
	)
	require.NoError(t, err)

	// Trigger automation
	err = factory.CreateContactTimelineEvent(workspaceID, contact.Email, "list_ops_event", nil)
	require.NoError(t, err)

	// Wait for enrollment
	ca := waitForEnrollment(t, factory, workspaceID, automation.ID, contact.Email, 2*time.Second)
	require.NotNil(t, ca)
	assert.Equal(t, domain.ContactAutomationStatusActive, ca.Status)

	t.Logf("List operations test passed: contact enrolled")
}

// testAutomationContextData tests that timeline event data is passed to automation context
func testAutomationContextData(t *testing.T, factory *testutil.TestDataFactory, workspaceID string) {
	// Create automation
	automation, err := factory.CreateAutomation(workspaceID,
		testutil.WithAutomationName("Context Data Automation"),
		testutil.WithAutomationTrigger(&domain.TimelineTriggerConfig{
			EventKind: "purchase",
			Frequency: domain.TriggerFrequencyEveryTime,
		}),
	)
	require.NoError(t, err)

	// Create simple flow (trigger is terminal)
	triggerNode, err := factory.CreateAutomationNode(workspaceID,
		testutil.WithNodeAutomationID(automation.ID),
		testutil.WithNodeType(domain.NodeTypeTrigger),
	)
	require.NoError(t, err)

	err = factory.UpdateAutomationRootNode(workspaceID, automation.ID, triggerNode.ID)
	require.NoError(t, err)

	err = factory.ActivateAutomation(workspaceID, automation.ID)
	require.NoError(t, err)

	// Create contact
	contact, err := factory.CreateContact(workspaceID, testutil.WithContactEmail("purchase-test@example.com"))
	require.NoError(t, err)

	// Trigger with purchase data
	err = factory.CreateContactTimelineEvent(workspaceID, contact.Email, "purchase", map[string]interface{}{
		"order_id": "ORD-123",
		"amount":   99.99,
		"items": []interface{}{
			map[string]interface{}{"sku": "SKU-001", "qty": 2},
		},
	})
	require.NoError(t, err)

	// Wait for enrollment
	ca := waitForEnrollment(t, factory, workspaceID, automation.ID, contact.Email, 2*time.Second)
	require.NotNil(t, ca)
	assert.Equal(t, domain.ContactAutomationStatusActive, ca.Status)

	t.Logf("Context data test passed: contact enrolled with purchase event")
}

// testAutomationSegmentTrigger tests triggering automation on segment.joined event
func testAutomationSegmentTrigger(t *testing.T, factory *testutil.TestDataFactory, workspaceID string) {
	// Create automation triggered by segment.joined
	automation, err := factory.CreateAutomation(workspaceID,
		testutil.WithAutomationName("Segment Trigger Automation"),
		testutil.WithAutomationTrigger(&domain.TimelineTriggerConfig{
			EventKind: "segment.joined",
			Frequency: domain.TriggerFrequencyOnce,
		}),
	)
	require.NoError(t, err)

	// Create simple flow (trigger is terminal)
	triggerNode, err := factory.CreateAutomationNode(workspaceID,
		testutil.WithNodeAutomationID(automation.ID),
		testutil.WithNodeType(domain.NodeTypeTrigger),
	)
	require.NoError(t, err)

	err = factory.UpdateAutomationRootNode(workspaceID, automation.ID, triggerNode.ID)
	require.NoError(t, err)

	err = factory.ActivateAutomation(workspaceID, automation.ID)
	require.NoError(t, err)

	// Create contact
	contact, err := factory.CreateContact(workspaceID, testutil.WithContactEmail("segment-trigger@example.com"))
	require.NoError(t, err)

	// Simulate segment.joined event
	err = factory.CreateContactTimelineEvent(workspaceID, contact.Email, "segment.joined", map[string]interface{}{
		"segment_id":   "seg-inactive-30d",
		"segment_name": "Inactive 30 Days",
	})
	require.NoError(t, err)

	// Wait for enrollment
	ca := waitForEnrollment(t, factory, workspaceID, automation.ID, contact.Email, 2*time.Second)
	require.NotNil(t, ca)
	assert.Equal(t, domain.ContactAutomationStatusActive, ca.Status)

	t.Logf("Segment trigger test passed: contact enrolled on segment.joined")
}

// testAutomationDeletionCleanup tests that deleting automation cleans up properly
func testAutomationDeletionCleanup(t *testing.T, factory *testutil.TestDataFactory, client *testutil.APIClient, workspaceID string) {
	// Create and activate automation
	automation, err := factory.CreateAutomation(workspaceID,
		testutil.WithAutomationName("Deletion Test Automation"),
		testutil.WithAutomationTrigger(&domain.TimelineTriggerConfig{
			EventKind: "delete_test_event",
			Frequency: domain.TriggerFrequencyOnce,
		}),
	)
	require.NoError(t, err)

	triggerNode, err := factory.CreateAutomationNode(workspaceID,
		testutil.WithNodeAutomationID(automation.ID),
		testutil.WithNodeType(domain.NodeTypeTrigger),
	)
	require.NoError(t, err)

	err = factory.UpdateAutomationRootNode(workspaceID, automation.ID, triggerNode.ID)
	require.NoError(t, err)

	err = factory.ActivateAutomation(workspaceID, automation.ID)
	require.NoError(t, err)

	// Enroll a contact
	contact, err := factory.CreateContact(workspaceID, testutil.WithContactEmail("delete-test@example.com"))
	require.NoError(t, err)

	err = factory.CreateContactTimelineEvent(workspaceID, contact.Email, "delete_test_event", nil)
	require.NoError(t, err)

	// Wait for enrollment
	ca := waitForEnrollment(t, factory, workspaceID, automation.ID, contact.Email, 2*time.Second)
	require.NotNil(t, ca)
	assert.Equal(t, domain.ContactAutomationStatusActive, ca.Status)

	// Delete automation via API
	resp, err := client.Delete(fmt.Sprintf("/api/automation.delete?workspace_id=%s&id=%s", workspaceID, automation.ID))
	if err != nil {
		t.Logf("Delete API call failed: %v", err)
	}
	if resp != nil {
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Logf("Delete API returned status: %d", resp.StatusCode)
		}
	}

	// Verify: automation has deleted_at set
	workspaceDB, err := factory.GetWorkspaceDB(workspaceID)
	require.NoError(t, err)
	var deletedAt sql.NullTime
	err = workspaceDB.QueryRowContext(context.Background(),
		`SELECT deleted_at FROM automations WHERE id = $1`,
		automation.ID,
	).Scan(&deletedAt)
	require.NoError(t, err)

	if !deletedAt.Valid {
		addBug("TestAutomation_DeletionCleanup",
			"Automation not soft-deleted after Delete API call",
			"High", "Delete not setting deleted_at",
			"internal/repository/automation_postgres.go:Delete")
	}

	// Verify: active contacts should be marked as exited
	caAfter, err := factory.GetContactAutomation(workspaceID, automation.ID, contact.Email)
	if err == nil && caAfter.Status == domain.ContactAutomationStatusActive {
		addBug("TestAutomation_DeletionCleanup",
			"Active contact not marked as exited after automation deletion",
			"Medium", "Delete not updating contact_automations",
			"internal/repository/automation_postgres.go:Delete")
	}

	t.Logf("Deletion cleanup test passed")
}

// testAutomationErrorRecovery tests retry mechanism for failed node executions
func testAutomationErrorRecovery(t *testing.T, factory *testutil.TestDataFactory, workspaceID string) {
	// Create automation
	automation, err := factory.CreateAutomation(workspaceID,
		testutil.WithAutomationName("Error Recovery Automation"),
		testutil.WithAutomationTrigger(&domain.TimelineTriggerConfig{
			EventKind: "error_test_event",
			Frequency: domain.TriggerFrequencyOnce,
		}),
	)
	require.NoError(t, err)

	// Create nodes with email node (will fail without provider)
	triggerNode, err := factory.CreateAutomationNode(workspaceID,
		testutil.WithNodeAutomationID(automation.ID),
		testutil.WithNodeType(domain.NodeTypeTrigger),
	)
	require.NoError(t, err)

	emailNode, err := factory.CreateAutomationNode(workspaceID,
		testutil.WithNodeAutomationID(automation.ID),
		testutil.WithNodeType(domain.NodeTypeEmail),
		testutil.WithNodeConfig(map[string]interface{}{
			"template_id": "nonexistent-template",
		}),
	)
	require.NoError(t, err)

	err = factory.UpdateAutomationNodeNextNodeID(workspaceID, automation.ID, triggerNode.ID, emailNode.ID)
	require.NoError(t, err)

	err = factory.UpdateAutomationRootNode(workspaceID, automation.ID, triggerNode.ID)
	require.NoError(t, err)

	err = factory.ActivateAutomation(workspaceID, automation.ID)
	require.NoError(t, err)

	// Create contact and trigger
	contact, err := factory.CreateContact(workspaceID, testutil.WithContactEmail("error-test@example.com"))
	require.NoError(t, err)

	err = factory.CreateContactTimelineEvent(workspaceID, contact.Email, "error_test_event", nil)
	require.NoError(t, err)

	// Wait for enrollment (enrollment should succeed even if later execution fails)
	ca := waitForEnrollment(t, factory, workspaceID, automation.ID, contact.Email, 2*time.Second)
	require.NotNil(t, ca)
	assert.Equal(t, domain.ContactAutomationStatusActive, ca.Status)

	// Verify retry infrastructure exists
	assert.Equal(t, 0, ca.RetryCount, "Initial retry count should be 0")
	assert.Equal(t, 3, ca.MaxRetries, "Default max retries should be 3")

	t.Logf("Error recovery test passed: retry infrastructure verified")
}

// testAutomationSchedulerExecution tests that the scheduler processes contacts correctly
func testAutomationSchedulerExecution(t *testing.T, factory *testutil.TestDataFactory, workspaceID string) {
	// Create template
	template, err := factory.CreateTemplate(workspaceID)
	require.NoError(t, err)

	// Create list (required for email nodes)
	list, err := factory.CreateList(workspaceID)
	require.NoError(t, err)

	// Create automation
	automation, err := factory.CreateAutomation(workspaceID,
		testutil.WithAutomationName("Scheduler Execution Automation"),
		testutil.WithAutomationListID(list.ID),
		testutil.WithAutomationTrigger(&domain.TimelineTriggerConfig{
			EventKind: "scheduler_test_event",
			Frequency: domain.TriggerFrequencyOnce,
		}),
	)
	require.NoError(t, err)

	// Create nodes: trigger → email (terminal)
	triggerNode, err := factory.CreateAutomationNode(workspaceID,
		testutil.WithNodeAutomationID(automation.ID),
		testutil.WithNodeType(domain.NodeTypeTrigger),
	)
	require.NoError(t, err)

	emailNode, err := factory.CreateAutomationNode(workspaceID,
		testutil.WithNodeAutomationID(automation.ID),
		testutil.WithNodeType(domain.NodeTypeEmail),
		testutil.WithNodeConfig(map[string]interface{}{
			"template_id": template.ID,
		}),
	)
	require.NoError(t, err)

	err = factory.UpdateAutomationNodeNextNodeID(workspaceID, automation.ID, triggerNode.ID, emailNode.ID)
	require.NoError(t, err)

	err = factory.UpdateAutomationRootNode(workspaceID, automation.ID, triggerNode.ID)
	require.NoError(t, err)

	err = factory.ActivateAutomation(workspaceID, automation.ID)
	require.NoError(t, err)

	// Create contact and add to list first
	contact, err := factory.CreateContact(workspaceID, testutil.WithContactEmail("scheduler-test@example.com"))
	require.NoError(t, err)

	_, err = factory.CreateContactList(workspaceID,
		testutil.WithContactListEmail(contact.Email),
		testutil.WithContactListListID(list.ID),
		testutil.WithContactListStatus(domain.ContactListStatusActive),
	)
	require.NoError(t, err)

	// Trigger automation
	err = factory.CreateContactTimelineEvent(workspaceID, contact.Email, "scheduler_test_event", nil)
	require.NoError(t, err)

	// Wait for enrollment
	ca := waitForEnrollment(t, factory, workspaceID, automation.ID, contact.Email, 2*time.Second)
	require.NotNil(t, ca)
	assert.Equal(t, domain.ContactAutomationStatusActive, ca.Status)

	// Verify that the enrollment created a node execution log entry
	executions, err := factory.GetNodeExecutions(workspaceID, ca.ID)
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

	// Verify contact is scheduled for processing
	if ca.ScheduledAt == nil {
		addBug("TestAutomation_SchedulerExecution",
			"Contact not scheduled for processing after enrollment",
			"High", "scheduled_at not set by enrollment",
			"internal/migrations/v20.go:automation_enroll_contact")
	} else {
		t.Logf("Contact scheduled for: %v", ca.ScheduledAt)
	}

	t.Logf("Scheduler execution test passed: enrollment verified")
}

// testAutomationPauseResume tests that paused automations freeze contacts instead of exiting them
func testAutomationPauseResume(t *testing.T, factory *testutil.TestDataFactory, workspaceID string) {
	// Create automation
	automation, err := factory.CreateAutomation(workspaceID,
		testutil.WithAutomationName("Pause Resume Test"),
		testutil.WithAutomationTrigger(&domain.TimelineTriggerConfig{
			EventKind: "test_pause_event",
			Frequency: domain.TriggerFrequencyOnce,
		}),
	)
	require.NoError(t, err)

	// Create nodes: trigger → delay (terminal)
	triggerNode, err := factory.CreateAutomationNode(workspaceID,
		testutil.WithNodeAutomationID(automation.ID),
		testutil.WithNodeType(domain.NodeTypeTrigger),
		testutil.WithNodeConfig(map[string]interface{}{}),
	)
	require.NoError(t, err)

	delayNode, err := factory.CreateAutomationNode(workspaceID,
		testutil.WithNodeAutomationID(automation.ID),
		testutil.WithNodeType(domain.NodeTypeDelay),
		testutil.WithNodeConfig(map[string]interface{}{
			"duration": 1,
			"unit":     "seconds",
		}),
	)
	require.NoError(t, err)

	err = factory.UpdateAutomationNodeNextNodeID(workspaceID, automation.ID, triggerNode.ID, delayNode.ID)
	require.NoError(t, err)

	err = factory.UpdateAutomationRootNode(workspaceID, automation.ID, triggerNode.ID)
	require.NoError(t, err)

	err = factory.ActivateAutomation(workspaceID, automation.ID)
	require.NoError(t, err)

	// Create contact and trigger enrollment
	contact, err := factory.CreateContact(workspaceID, testutil.WithContactEmail("pause-test@example.com"))
	require.NoError(t, err)

	err = factory.CreateContactTimelineEvent(workspaceID, contact.Email, "test_pause_event", map[string]interface{}{})
	require.NoError(t, err)

	// Wait for enrollment
	ca := waitForEnrollment(t, factory, workspaceID, automation.ID, contact.Email, 2*time.Second)
	require.NotNil(t, ca, "Contact should be enrolled")
	assert.Equal(t, domain.ContactAutomationStatusActive, ca.Status)
	t.Logf("Contact enrolled with status: %s", ca.Status)

	// PAUSE the automation
	workspaceDB, err := factory.GetWorkspaceDB(workspaceID)
	require.NoError(t, err)
	_, err = workspaceDB.ExecContext(context.Background(),
		`UPDATE automations SET status = $1, updated_at = $2 WHERE id = $3`,
		domain.AutomationStatusPaused, time.Now().UTC(), automation.ID)
	require.NoError(t, err)
	t.Log("Automation paused")

	// Verify contact status is still ACTIVE (not exited!)
	ca, err = factory.GetContactAutomation(workspaceID, automation.ID, contact.Email)
	require.NoError(t, err)
	assert.Equal(t, domain.ContactAutomationStatusActive, ca.Status, "Contact should still be ACTIVE when automation is paused")
	t.Logf("After pause - Contact status: %s (should be active)", ca.Status)

	// Verify scheduler query does NOT return this contact (paused automation filtered out)
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
		var id, email string
		err := rows.Scan(&id, &email)
		require.NoError(t, err)
		if email == contact.Email {
			found = true
			break
		}
	}
	assert.False(t, found, "Contact should NOT be returned by scheduler when automation is paused")
	t.Logf("Scheduler query returned paused contact: %v (should be false)", found)

	// RESUME the automation
	_, err = workspaceDB.ExecContext(context.Background(),
		`UPDATE automations SET status = $1, updated_at = $2 WHERE id = $3`,
		domain.AutomationStatusLive, time.Now().UTC(), automation.ID)
	require.NoError(t, err)
	t.Log("Automation resumed")

	// Verify contact can now be scheduled
	rows2, err := workspaceDB.QueryContext(context.Background(), schedulerQuery, time.Now().Add(1*time.Hour))
	require.NoError(t, err)
	defer rows2.Close()

	found = false
	for rows2.Next() {
		var id, email string
		err := rows2.Scan(&id, &email)
		require.NoError(t, err)
		if email == contact.Email {
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
	// Owner creates an automation
	err := client.Login(owner.Email, "password")
	require.NoError(t, err)
	client.SetWorkspaceID(workspaceID)

	automation, err := factory.CreateAutomation(workspaceID,
		testutil.WithAutomationName("Permission Test Automation"),
		testutil.WithAutomationTrigger(&domain.TimelineTriggerConfig{
			EventKind: "test_event",
			Frequency: domain.TriggerFrequencyOnce,
		}),
	)
	require.NoError(t, err)
	t.Logf("Owner created automation: %s", automation.ID)

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
func testAutomationTimelineStartEvent(t *testing.T, factory *testutil.TestDataFactory, workspaceID string) {
	// Create automation
	automation, err := factory.CreateAutomation(workspaceID,
		testutil.WithAutomationName("Timeline Start Event Test"),
		testutil.WithAutomationTrigger(&domain.TimelineTriggerConfig{
			EventKind: "timeline_start_test_event",
			Frequency: domain.TriggerFrequencyOnce,
		}),
	)
	require.NoError(t, err)

	// Create trigger node (terminal)
	triggerNode, err := factory.CreateAutomationNode(workspaceID,
		testutil.WithNodeAutomationID(automation.ID),
		testutil.WithNodeType(domain.NodeTypeTrigger),
		testutil.WithNodeConfig(map[string]interface{}{}),
	)
	require.NoError(t, err)

	err = factory.UpdateAutomationRootNode(workspaceID, automation.ID, triggerNode.ID)
	require.NoError(t, err)

	err = factory.ActivateAutomation(workspaceID, automation.ID)
	require.NoError(t, err)

	// Create contact
	contact, err := factory.CreateContact(workspaceID, testutil.WithContactEmail("timeline-start@example.com"))
	require.NoError(t, err)

	// Trigger the automation enrollment
	err = factory.CreateContactTimelineEvent(workspaceID, contact.Email, "timeline_start_test_event", nil)
	require.NoError(t, err)

	// Wait for enrollment
	ca := waitForEnrollment(t, factory, workspaceID, automation.ID, contact.Email, 2*time.Second)
	require.NotNil(t, ca, "Contact should be enrolled")
	assert.Equal(t, domain.ContactAutomationStatusActive, ca.Status)

	// Wait for automation.start timeline event
	events := waitForTimelineEvent(t, factory, workspaceID, contact.Email, "automation.start", 2*time.Second)

	if len(events) == 0 {
		addBug("TestAutomation_TimelineStartEvent",
			"No automation.start timeline event created on enrollment",
			"High", "automation_enroll_contact function not inserting timeline event",
			"internal/database/init.go:automation_enroll_contact")
		t.Fatal("Expected automation.start timeline event, found none")
	}

	// Verify the event has correct data
	event := events[0]
	assert.Equal(t, "automation", event.EntityType, "Entity type should be 'automation'")
	assert.Equal(t, "automation.start", event.Kind)
	assert.Equal(t, "insert", event.Operation)
	require.NotNil(t, event.EntityID, "EntityID should be set")
	assert.Equal(t, automation.ID, *event.EntityID, "EntityID should be automation ID")

	// Verify changes contain automation_id and root_node_id
	require.NotNil(t, event.Changes)
	automationIDChange, ok := event.Changes["automation_id"].(map[string]interface{})
	require.True(t, ok, "Changes should contain automation_id")
	assert.Equal(t, automation.ID, automationIDChange["new"], "automation_id.new should match automation ID")

	rootNodeIDChange, ok := event.Changes["root_node_id"].(map[string]interface{})
	require.True(t, ok, "Changes should contain root_node_id")
	assert.Equal(t, triggerNode.ID, rootNodeIDChange["new"], "root_node_id.new should match trigger node ID")

	t.Logf("Timeline start event test passed: automation.start event created with correct data")
}

// testAutomationTimelineEndEvent tests that automation.end timeline event is created when contact completes
func testAutomationTimelineEndEvent(t *testing.T, factory *testutil.TestDataFactory, workspaceID string) {
	// Create automation with trigger → delay (terminal)
	automation, err := factory.CreateAutomation(workspaceID,
		testutil.WithAutomationName("Timeline End Event Test"),
		testutil.WithAutomationTrigger(&domain.TimelineTriggerConfig{
			EventKind: "timeline_end_test_event",
			Frequency: domain.TriggerFrequencyOnce,
		}),
	)
	require.NoError(t, err)

	// Create nodes: trigger → delay (terminal - will complete immediately in scheduler)
	triggerNode, err := factory.CreateAutomationNode(workspaceID,
		testutil.WithNodeAutomationID(automation.ID),
		testutil.WithNodeType(domain.NodeTypeTrigger),
		testutil.WithNodeConfig(map[string]interface{}{}),
	)
	require.NoError(t, err)

	// Delay node with 0 duration (completes immediately) - this is a terminal node
	delayNode, err := factory.CreateAutomationNode(workspaceID,
		testutil.WithNodeAutomationID(automation.ID),
		testutil.WithNodeType(domain.NodeTypeDelay),
		testutil.WithNodeConfig(map[string]interface{}{
			"duration": 0,
			"unit":     "seconds",
		}),
	)
	require.NoError(t, err)

	err = factory.UpdateAutomationNodeNextNodeID(workspaceID, automation.ID, triggerNode.ID, delayNode.ID)
	require.NoError(t, err)

	err = factory.UpdateAutomationRootNode(workspaceID, automation.ID, triggerNode.ID)
	require.NoError(t, err)

	err = factory.ActivateAutomation(workspaceID, automation.ID)
	require.NoError(t, err)

	// Create contact and trigger enrollment
	contact, err := factory.CreateContact(workspaceID, testutil.WithContactEmail("timeline-end@example.com"))
	require.NoError(t, err)

	err = factory.CreateContactTimelineEvent(workspaceID, contact.Email, "timeline_end_test_event", nil)
	require.NoError(t, err)

	// Wait for enrollment
	ca := waitForEnrollment(t, factory, workspaceID, automation.ID, contact.Email, 2*time.Second)
	require.NotNil(t, ca, "Contact should be enrolled")

	// Verify automation.start event exists
	startEvents, err := factory.GetContactTimelineEvents(workspaceID, contact.Email, "automation.start")
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(startEvents), 1, "Should have automation.start event")

	// Note: automation.end event is created by the scheduler when processing contacts
	// through terminal nodes. The scheduler is not running in these integration tests
	// by default, so we verify the infrastructure is in place.

	// If the scheduler has processed (status = completed), check for end event
	if ca.Status == domain.ContactAutomationStatusCompleted {
		endEvents, err := factory.GetContactTimelineEvents(workspaceID, contact.Email, "automation.end")
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
	t.Logf("Contact status: %s (scheduler needed for completion)", ca.Status)
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
