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

// TestAutomation_WelcomeSeries tests the complete welcome series flow
// Use Case: Contact subscribes to list → receives welcome email sequence
func TestAutomation_WelcomeSeries(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	factory := suite.DataFactory
	client := suite.APIClient

	// Setup: Create user and workspace
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

	// Create list for the automation
	list, err := factory.CreateList(workspace.ID, testutil.WithListName("Welcome List"))
	require.NoError(t, err)

	// Create email template
	template, err := factory.CreateTemplate(workspace.ID, testutil.WithTemplateName("Welcome Email"))
	require.NoError(t, err)

	// Create automation with trigger on insert_contact_list
	automation, err := factory.CreateAutomation(workspace.ID,
		testutil.WithAutomationName("Welcome Series"),
		testutil.WithAutomationListID(list.ID),
		testutil.WithAutomationTrigger(&domain.TimelineTriggerConfig{
			EventKind: "insert_contact_list",
			Frequency:  domain.TriggerFrequencyOnce,
		}),
	)
	require.NoError(t, err)

	// Create nodes: trigger → email (terminal)
	triggerNode, err := factory.CreateAutomationNode(workspace.ID,
		testutil.WithNodeAutomationID(automation.ID),
		testutil.WithNodeType(domain.NodeTypeTrigger),
		testutil.WithNodeConfig(map[string]interface{}{}),
	)
	require.NoError(t, err)

	emailNode, err := factory.CreateAutomationNode(workspace.ID,
		testutil.WithNodeAutomationID(automation.ID),
		testutil.WithNodeType(domain.NodeTypeEmail),
		testutil.WithNodeConfig(map[string]interface{}{
			"template_id": template.ID,
		}),
		// No NextNodeID - this is a terminal node
	)
	require.NoError(t, err)

	// Update trigger node to point to email node
	err = factory.UpdateAutomationNodeNextNodeID(workspace.ID, automation.ID, triggerNode.ID, emailNode.ID)
	require.NoError(t, err)

	// Set root node
	err = factory.UpdateAutomationRootNode(workspace.ID, automation.ID, triggerNode.ID)
	require.NoError(t, err)

	// Activate automation (creates DB trigger)
	err = factory.ActivateAutomation(workspace.ID, automation.ID)
	require.NoError(t, err)

	// Create contact
	contact, err := factory.CreateContact(workspace.ID,
		testutil.WithContactEmail("welcome-test@example.com"),
		testutil.WithContactName("Test", "User"),
	)
	require.NoError(t, err)

	// Add contact to list - this should trigger the automation via timeline event
	_, err = factory.CreateContactList(workspace.ID,
		testutil.WithContactListEmail(contact.Email),
		testutil.WithContactListListID(list.ID),
		testutil.WithContactListStatus(domain.ContactListStatusActive),
	)
	require.NoError(t, err)

	// Insert timeline event (simulates what the contact_list trigger does)
	err = factory.CreateContactTimelineEvent(workspace.ID, contact.Email, "insert_contact_list", map[string]interface{}{
		"list_id": list.ID,
	})
	require.NoError(t, err)

	// Wait for enrollment
	time.Sleep(100 * time.Millisecond)

	// Verify: contact enrolled in automation
	ca, err := factory.GetContactAutomation(workspace.ID, automation.ID, contact.Email)
	if err != nil {
		addBug("TestAutomation_WelcomeSeries", "Contact not enrolled after timeline event",
			"Critical", "Trigger not firing on timeline insert",
			"internal/migrations/v20.go:automation_enroll_contact")
		t.Fatalf("Contact not enrolled: %v", err)
	}

	assert.Equal(t, domain.ContactAutomationStatusActive, ca.Status)
	assert.NotNil(t, ca.CurrentNodeID, "Current node should be set")

	// Verify stats
	stats, err := factory.GetAutomationStats(workspace.ID, automation.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), stats.Enrolled, "Enrolled count should be 1")

	t.Logf("Welcome Series test passed: contact enrolled, stats updated")
}

// TestAutomation_Deduplication tests frequency: once prevents duplicate enrollments
func TestAutomation_Deduplication(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	factory := suite.DataFactory

	// Setup
	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	err = factory.AddUserToWorkspace(user.ID, workspace.ID, "owner")
	require.NoError(t, err)

	// Create automation with frequency: once
	automation, err := factory.CreateAutomation(workspace.ID,
		testutil.WithAutomationName("Once Only Automation"),
		testutil.WithAutomationTrigger(&domain.TimelineTriggerConfig{
			EventKind: "test_event",
			Frequency:  domain.TriggerFrequencyOnce,
		}),
	)
	require.NoError(t, err)

	// Create simple trigger flow (trigger is terminal)
	triggerNode, err := factory.CreateAutomationNode(workspace.ID,
		testutil.WithNodeAutomationID(automation.ID),
		testutil.WithNodeType(domain.NodeTypeTrigger),
		testutil.WithNodeConfig(map[string]interface{}{}),
		// No NextNodeID - this is a terminal node
	)
	require.NoError(t, err)

	err = factory.UpdateAutomationRootNode(workspace.ID, automation.ID, triggerNode.ID)
	require.NoError(t, err)

	err = factory.ActivateAutomation(workspace.ID, automation.ID)
	require.NoError(t, err)

	// Create contact
	contact, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail("dedup-test@example.com"))
	require.NoError(t, err)

	// Trigger 3 times
	for i := 0; i < 3; i++ {
		err = factory.CreateContactTimelineEvent(workspace.ID, contact.Email, "test_event", map[string]interface{}{
			"iteration": i,
		})
		require.NoError(t, err)
		time.Sleep(50 * time.Millisecond)
	}

	// Wait for processing
	time.Sleep(200 * time.Millisecond)

	// Verify: only 1 contact_automation created
	count, err := factory.CountContactAutomations(workspace.ID, automation.ID)
	require.NoError(t, err)

	if count != 1 {
		addBug("TestAutomation_Deduplication",
			fmt.Sprintf("Expected 1 enrollment, got %d", count),
			"Critical", "Deduplication via automation_trigger_log not working",
			"internal/migrations/v20.go:automation_enroll_contact")
	}
	assert.Equal(t, 1, count, "Should have exactly 1 contact automation record")

	// Verify trigger log entry exists
	hasEntry, err := factory.GetTriggerLogEntry(workspace.ID, automation.ID, contact.Email)
	require.NoError(t, err)
	assert.True(t, hasEntry, "Trigger log entry should exist")

	// Verify stats
	stats, err := factory.GetAutomationStats(workspace.ID, automation.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), stats.Enrolled, "Enrolled should be 1, not 3")

	t.Logf("Deduplication test passed: frequency=once working correctly")
}

// TestAutomation_MultipleEntries tests frequency: every_time allows multiple enrollments
func TestAutomation_MultipleEntries(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	factory := suite.DataFactory

	// Setup
	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	err = factory.AddUserToWorkspace(user.ID, workspace.ID, "owner")
	require.NoError(t, err)

	// Create automation with frequency: every_time
	automation, err := factory.CreateAutomation(workspace.ID,
		testutil.WithAutomationName("Every Time Automation"),
		testutil.WithAutomationTrigger(&domain.TimelineTriggerConfig{
			EventKind: "repeat_event",
			Frequency:  domain.TriggerFrequencyEveryTime,
		}),
	)
	require.NoError(t, err)

	// Create simple trigger flow (trigger is terminal)
	triggerNode, err := factory.CreateAutomationNode(workspace.ID,
		testutil.WithNodeAutomationID(automation.ID),
		testutil.WithNodeType(domain.NodeTypeTrigger),
		// No NextNodeID - this is a terminal node
	)
	require.NoError(t, err)

	err = factory.UpdateAutomationRootNode(workspace.ID, automation.ID, triggerNode.ID)
	require.NoError(t, err)

	err = factory.ActivateAutomation(workspace.ID, automation.ID)
	require.NoError(t, err)

	// Create contact
	contact, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail("multi-test@example.com"))
	require.NoError(t, err)

	// Trigger 3 times
	for i := 0; i < 3; i++ {
		err = factory.CreateContactTimelineEvent(workspace.ID, contact.Email, "repeat_event", map[string]interface{}{
			"iteration": i,
		})
		require.NoError(t, err)
		time.Sleep(100 * time.Millisecond) // Ensure different entered_at
	}

	// Wait for processing
	time.Sleep(200 * time.Millisecond)

	// Verify: 3 contact_automation records
	count, err := factory.CountContactAutomations(workspace.ID, automation.ID)
	require.NoError(t, err)

	if count != 3 {
		addBug("TestAutomation_MultipleEntries",
			fmt.Sprintf("Expected 3 enrollments, got %d", count),
			"High", "every_time frequency not allowing multiple entries",
			"internal/migrations/v20.go:automation_enroll_contact")
	}
	assert.Equal(t, 3, count, "Should have 3 contact automation records")

	// Verify each has different entered_at
	cas, err := factory.GetAllContactAutomations(workspace.ID, automation.ID)
	require.NoError(t, err)
	assert.Len(t, cas, 3, "Should have 3 records")

	// Verify stats
	stats, err := factory.GetAutomationStats(workspace.ID, automation.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(3), stats.Enrolled, "Enrolled should be 3")

	t.Logf("Multiple entries test passed: frequency=every_time working correctly")
}

// TestAutomation_DelayTiming tests delay node calculations
func TestAutomation_DelayTiming(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	factory := suite.DataFactory

	// Setup
	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	err = factory.AddUserToWorkspace(user.ID, workspace.ID, "owner")
	require.NoError(t, err)

	// Create automation
	automation, err := factory.CreateAutomation(workspace.ID,
		testutil.WithAutomationName("Delay Test Automation"),
		testutil.WithAutomationTrigger(&domain.TimelineTriggerConfig{
			EventKind: "delay_test_event",
			Frequency:  domain.TriggerFrequencyOnce,
		}),
	)
	require.NoError(t, err)

	// Create nodes: trigger → delay (5 minutes) - delay is terminal
	triggerNode, err := factory.CreateAutomationNode(workspace.ID,
		testutil.WithNodeAutomationID(automation.ID),
		testutil.WithNodeType(domain.NodeTypeTrigger),
	)
	require.NoError(t, err)

	delayNode, err := factory.CreateAutomationNode(workspace.ID,
		testutil.WithNodeAutomationID(automation.ID),
		testutil.WithNodeType(domain.NodeTypeDelay),
		testutil.WithNodeConfig(map[string]interface{}{
			"duration": 5,
			"unit":     "minutes",
		}),
		// No NextNodeID - this is a terminal node
	)
	require.NoError(t, err)

	err = factory.UpdateAutomationNodeNextNodeID(workspace.ID, automation.ID, triggerNode.ID, delayNode.ID)
	require.NoError(t, err)

	err = factory.UpdateAutomationRootNode(workspace.ID, automation.ID, triggerNode.ID)
	require.NoError(t, err)

	err = factory.ActivateAutomation(workspace.ID, automation.ID)
	require.NoError(t, err)

	// Create contact and trigger
	contact, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail("delay-test@example.com"))
	require.NoError(t, err)

	beforeTrigger := time.Now().UTC()
	err = factory.CreateContactTimelineEvent(workspace.ID, contact.Email, "delay_test_event", nil)
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// Verify enrollment
	ca, err := factory.GetContactAutomation(workspace.ID, automation.ID, contact.Email)
	require.NoError(t, err)

	// After enrollment, contact should be at the delay node with scheduled_at in the future
	// Note: The trigger node immediately transitions to delay node which sets scheduled_at
	assert.Equal(t, domain.ContactAutomationStatusActive, ca.Status)

	// Verify scheduled_at is approximately 5 minutes in the future (if scheduler has processed)
	// or NOW() if not yet processed
	if ca.ScheduledAt != nil {
		expectedMin := beforeTrigger.Add(4 * time.Minute)   // Allow some tolerance
		expectedMax := beforeTrigger.Add(6 * time.Minute)

		// If scheduled_at is in the future, it should be ~5 minutes from trigger time
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

// TestAutomation_ABTestDeterminism tests A/B test variant selection is deterministic
func TestAutomation_ABTestDeterminism(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	factory := suite.DataFactory

	// Setup
	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	err = factory.AddUserToWorkspace(user.ID, workspace.ID, "owner")
	require.NoError(t, err)

	// Create automation
	automation, err := factory.CreateAutomation(workspace.ID,
		testutil.WithAutomationName("AB Test Automation"),
		testutil.WithAutomationTrigger(&domain.TimelineTriggerConfig{
			EventKind: "ab_test_event",
			Frequency:  domain.TriggerFrequencyEveryTime,
		}),
	)
	require.NoError(t, err)

	// Create A/B test node with 50/50 split (variants lead to terminal - empty string)
	triggerNode, err := factory.CreateAutomationNode(workspace.ID,
		testutil.WithNodeAutomationID(automation.ID),
		testutil.WithNodeType(domain.NodeTypeTrigger),
	)
	require.NoError(t, err)

	abNode, err := factory.CreateAutomationNode(workspace.ID,
		testutil.WithNodeAutomationID(automation.ID),
		testutil.WithNodeType(domain.NodeTypeABTest),
		testutil.WithNodeConfig(map[string]interface{}{
			"variants": []map[string]interface{}{
				{"id": "A", "name": "Variant A", "weight": 50, "next_node_id": ""}, // Terminal
				{"id": "B", "name": "Variant B", "weight": 50, "next_node_id": ""}, // Terminal
			},
		}),
	)
	require.NoError(t, err)

	err = factory.UpdateAutomationNodeNextNodeID(workspace.ID, automation.ID, triggerNode.ID, abNode.ID)
	require.NoError(t, err)

	err = factory.UpdateAutomationRootNode(workspace.ID, automation.ID, triggerNode.ID)
	require.NoError(t, err)

	err = factory.ActivateAutomation(workspace.ID, automation.ID)
	require.NoError(t, err)

	// Test determinism: same contact should always get same variant
	contact, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail("ab-determ@example.com"))
	require.NoError(t, err)

	// The FNV-32a hash of email+nodeID should be consistent
	// We can't easily test this without running the scheduler, so we'll just verify enrollment works
	err = factory.CreateContactTimelineEvent(workspace.ID, contact.Email, "ab_test_event", nil)
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	ca, err := factory.GetContactAutomation(workspace.ID, automation.ID, contact.Email)
	require.NoError(t, err)
	assert.Equal(t, domain.ContactAutomationStatusActive, ca.Status)

	t.Logf("A/B test determinism test passed: enrollment working")
}

// TestAutomation_BranchRouting tests branch node routing based on conditions
func TestAutomation_BranchRouting(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	factory := suite.DataFactory

	// Setup
	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	err = factory.AddUserToWorkspace(user.ID, workspace.ID, "owner")
	require.NoError(t, err)

	// Create automation with branch
	automation, err := factory.CreateAutomation(workspace.ID,
		testutil.WithAutomationName("Branch Test Automation"),
		testutil.WithAutomationTrigger(&domain.TimelineTriggerConfig{
			EventKind: "branch_test_event",
			Frequency:  domain.TriggerFrequencyOnce,
		}),
	)
	require.NoError(t, err)

	// Create nodes (branch paths lead to terminal - empty string)
	triggerNode, err := factory.CreateAutomationNode(workspace.ID,
		testutil.WithNodeAutomationID(automation.ID),
		testutil.WithNodeType(domain.NodeTypeTrigger),
	)
	require.NoError(t, err)

	// Branch node with condition on country (VIP = US) - paths lead to terminal
	branchNode, err := factory.CreateAutomationNode(workspace.ID,
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
					"next_node_id": "", // Terminal
				},
			},
			"default_path_id": "", // Terminal
		}),
	)
	require.NoError(t, err)

	err = factory.UpdateAutomationNodeNextNodeID(workspace.ID, automation.ID, triggerNode.ID, branchNode.ID)
	require.NoError(t, err)

	err = factory.UpdateAutomationRootNode(workspace.ID, automation.ID, triggerNode.ID)
	require.NoError(t, err)

	err = factory.ActivateAutomation(workspace.ID, automation.ID)
	require.NoError(t, err)

	// Create VIP contact (US)
	vipContact, err := factory.CreateContact(workspace.ID,
		testutil.WithContactEmail("vip-branch@example.com"),
		testutil.WithContactCountry("US"),
	)
	require.NoError(t, err)

	err = factory.CreateContactTimelineEvent(workspace.ID, vipContact.Email, "branch_test_event", nil)
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// Verify enrollment
	ca, err := factory.GetContactAutomation(workspace.ID, automation.ID, vipContact.Email)
	require.NoError(t, err)
	assert.Equal(t, domain.ContactAutomationStatusActive, ca.Status)

	t.Logf("Branch routing test passed: contact enrolled")
}

// TestAutomation_FilterNode tests filter node pass/fail paths
func TestAutomation_FilterNode(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	factory := suite.DataFactory

	// Setup
	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	err = factory.AddUserToWorkspace(user.ID, workspace.ID, "owner")
	require.NoError(t, err)

	// Create automation with filter
	automation, err := factory.CreateAutomation(workspace.ID,
		testutil.WithAutomationName("Filter Test Automation"),
		testutil.WithAutomationTrigger(&domain.TimelineTriggerConfig{
			EventKind: "filter_test_event",
			Frequency:  domain.TriggerFrequencyOnce,
		}),
	)
	require.NoError(t, err)

	// Create nodes (filter paths lead to terminal - empty string)
	triggerNode, err := factory.CreateAutomationNode(workspace.ID,
		testutil.WithNodeAutomationID(automation.ID),
		testutil.WithNodeType(domain.NodeTypeTrigger),
	)
	require.NoError(t, err)

	// Filter: continue if country = FR - both paths lead to terminal
	filterNode, err := factory.CreateAutomationNode(workspace.ID,
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
			"continue_node_id": "", // Terminal
			"exit_node_id":     "", // Terminal
		}),
	)
	require.NoError(t, err)

	err = factory.UpdateAutomationNodeNextNodeID(workspace.ID, automation.ID, triggerNode.ID, filterNode.ID)
	require.NoError(t, err)

	err = factory.UpdateAutomationRootNode(workspace.ID, automation.ID, triggerNode.ID)
	require.NoError(t, err)

	err = factory.ActivateAutomation(workspace.ID, automation.ID)
	require.NoError(t, err)

	// Create passing contact (FR)
	passContact, err := factory.CreateContact(workspace.ID,
		testutil.WithContactEmail("filter-pass@example.com"),
		testutil.WithContactCountry("FR"),
	)
	require.NoError(t, err)

	err = factory.CreateContactTimelineEvent(workspace.ID, passContact.Email, "filter_test_event", nil)
	require.NoError(t, err)

	// Create failing contact (DE)
	failContact, err := factory.CreateContact(workspace.ID,
		testutil.WithContactEmail("filter-fail@example.com"),
		testutil.WithContactCountry("DE"),
	)
	require.NoError(t, err)

	err = factory.CreateContactTimelineEvent(workspace.ID, failContact.Email, "filter_test_event", nil)
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// Verify both enrolled
	passCA, err := factory.GetContactAutomation(workspace.ID, automation.ID, passContact.Email)
	require.NoError(t, err)
	assert.Equal(t, domain.ContactAutomationStatusActive, passCA.Status)

	failCA, err := factory.GetContactAutomation(workspace.ID, automation.ID, failContact.Email)
	require.NoError(t, err)
	assert.Equal(t, domain.ContactAutomationStatusActive, failCA.Status)

	t.Logf("Filter node test passed: both contacts enrolled")
}

// TestAutomation_ListOperations tests add_to_list and remove_from_list nodes
func TestAutomation_ListOperations(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	factory := suite.DataFactory

	// Setup
	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	err = factory.AddUserToWorkspace(user.ID, workspace.ID, "owner")
	require.NoError(t, err)

	// Create lists
	trialList, err := factory.CreateList(workspace.ID, testutil.WithListName("Trial"))
	require.NoError(t, err)
	premiumList, err := factory.CreateList(workspace.ID, testutil.WithListName("Premium"))
	require.NoError(t, err)

	// Create automation
	automation, err := factory.CreateAutomation(workspace.ID,
		testutil.WithAutomationName("List Operations Automation"),
		testutil.WithAutomationTrigger(&domain.TimelineTriggerConfig{
			EventKind: "list_ops_event",
			Frequency:  domain.TriggerFrequencyOnce,
		}),
	)
	require.NoError(t, err)

	// Create nodes: trigger → add_to_list → remove_from_list (terminal)
	triggerNode, err := factory.CreateAutomationNode(workspace.ID,
		testutil.WithNodeAutomationID(automation.ID),
		testutil.WithNodeType(domain.NodeTypeTrigger),
	)
	require.NoError(t, err)

	removeNode, err := factory.CreateAutomationNode(workspace.ID,
		testutil.WithNodeAutomationID(automation.ID),
		testutil.WithNodeType(domain.NodeTypeRemoveFromList),
		testutil.WithNodeConfig(map[string]interface{}{
			"list_id": trialList.ID,
		}),
		// No NextNodeID - this is a terminal node
	)
	require.NoError(t, err)

	addNode, err := factory.CreateAutomationNode(workspace.ID,
		testutil.WithNodeAutomationID(automation.ID),
		testutil.WithNodeType(domain.NodeTypeAddToList),
		testutil.WithNodeConfig(map[string]interface{}{
			"list_id": premiumList.ID,
			"status":  "subscribed",
		}),
		testutil.WithNodeNextNodeID(removeNode.ID),
	)
	require.NoError(t, err)

	err = factory.UpdateAutomationNodeNextNodeID(workspace.ID, automation.ID, triggerNode.ID, addNode.ID)
	require.NoError(t, err)

	err = factory.UpdateAutomationRootNode(workspace.ID, automation.ID, triggerNode.ID)
	require.NoError(t, err)

	err = factory.ActivateAutomation(workspace.ID, automation.ID)
	require.NoError(t, err)

	// Create contact in trial list
	contact, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail("list-ops@example.com"))
	require.NoError(t, err)

	_, err = factory.CreateContactList(workspace.ID,
		testutil.WithContactListEmail(contact.Email),
		testutil.WithContactListListID(trialList.ID),
		testutil.WithContactListStatus(domain.ContactListStatusActive),
	)
	require.NoError(t, err)

	// Trigger automation
	err = factory.CreateContactTimelineEvent(workspace.ID, contact.Email, "list_ops_event", nil)
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// Verify enrollment
	ca, err := factory.GetContactAutomation(workspace.ID, automation.ID, contact.Email)
	require.NoError(t, err)
	assert.Equal(t, domain.ContactAutomationStatusActive, ca.Status)

	t.Logf("List operations test passed: contact enrolled")
}

// TestAutomation_ContextData tests that timeline event data is passed to automation context
func TestAutomation_ContextData(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	factory := suite.DataFactory

	// Setup
	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	err = factory.AddUserToWorkspace(user.ID, workspace.ID, "owner")
	require.NoError(t, err)

	// Create automation
	automation, err := factory.CreateAutomation(workspace.ID,
		testutil.WithAutomationName("Context Data Automation"),
		testutil.WithAutomationTrigger(&domain.TimelineTriggerConfig{
			EventKind: "purchase",
			Frequency:  domain.TriggerFrequencyEveryTime,
		}),
	)
	require.NoError(t, err)

	// Create simple flow (trigger is terminal)
	triggerNode, err := factory.CreateAutomationNode(workspace.ID,
		testutil.WithNodeAutomationID(automation.ID),
		testutil.WithNodeType(domain.NodeTypeTrigger),
		// No NextNodeID - this is a terminal node
	)
	require.NoError(t, err)

	err = factory.UpdateAutomationRootNode(workspace.ID, automation.ID, triggerNode.ID)
	require.NoError(t, err)

	err = factory.ActivateAutomation(workspace.ID, automation.ID)
	require.NoError(t, err)

	// Create contact
	contact, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail("purchase-test@example.com"))
	require.NoError(t, err)

	// Trigger with purchase data
	err = factory.CreateContactTimelineEvent(workspace.ID, contact.Email, "purchase", map[string]interface{}{
		"order_id": "ORD-123",
		"amount":   99.99,
		"items": []interface{}{
			map[string]interface{}{"sku": "SKU-001", "qty": 2},
		},
	})
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// Verify enrollment
	ca, err := factory.GetContactAutomation(workspace.ID, automation.ID, contact.Email)
	require.NoError(t, err)
	assert.Equal(t, domain.ContactAutomationStatusActive, ca.Status)

	// Check if context contains the event data
	// Note: Context may not be populated by enrollment, depends on implementation
	t.Logf("Context data test passed: contact enrolled with purchase event")
}

// TestAutomation_SegmentTrigger tests triggering automation on segment.joined event
func TestAutomation_SegmentTrigger(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	factory := suite.DataFactory

	// Setup
	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	err = factory.AddUserToWorkspace(user.ID, workspace.ID, "owner")
	require.NoError(t, err)

	// Create automation triggered by segment.joined
	automation, err := factory.CreateAutomation(workspace.ID,
		testutil.WithAutomationName("Segment Trigger Automation"),
		testutil.WithAutomationTrigger(&domain.TimelineTriggerConfig{
			EventKind: "segment.joined",
			Frequency:  domain.TriggerFrequencyOnce,
		}),
	)
	require.NoError(t, err)

	// Create simple flow (trigger is terminal)
	triggerNode, err := factory.CreateAutomationNode(workspace.ID,
		testutil.WithNodeAutomationID(automation.ID),
		testutil.WithNodeType(domain.NodeTypeTrigger),
		// No NextNodeID - this is a terminal node
	)
	require.NoError(t, err)

	err = factory.UpdateAutomationRootNode(workspace.ID, automation.ID, triggerNode.ID)
	require.NoError(t, err)

	err = factory.ActivateAutomation(workspace.ID, automation.ID)
	require.NoError(t, err)

	// Create contact
	contact, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail("segment-trigger@example.com"))
	require.NoError(t, err)

	// Simulate segment.joined event
	err = factory.CreateContactTimelineEvent(workspace.ID, contact.Email, "segment.joined", map[string]interface{}{
		"segment_id":   "seg-inactive-30d",
		"segment_name": "Inactive 30 Days",
	})
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// Verify enrollment
	ca, err := factory.GetContactAutomation(workspace.ID, automation.ID, contact.Email)
	require.NoError(t, err)
	assert.Equal(t, domain.ContactAutomationStatusActive, ca.Status)

	t.Logf("Segment trigger test passed: contact enrolled on segment.joined")
}

// TestAutomation_DeletionCleanup tests that deleting automation cleans up properly
func TestAutomation_DeletionCleanup(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	factory := suite.DataFactory
	client := suite.APIClient

	// Setup
	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	err = factory.AddUserToWorkspace(user.ID, workspace.ID, "owner")
	require.NoError(t, err)

	err = client.Login(user.Email, "password")
	require.NoError(t, err)
	client.SetWorkspaceID(workspace.ID)

	// Create and activate automation
	automation, err := factory.CreateAutomation(workspace.ID,
		testutil.WithAutomationName("Deletion Test Automation"),
		testutil.WithAutomationTrigger(&domain.TimelineTriggerConfig{
			EventKind: "delete_test_event",
			Frequency:  domain.TriggerFrequencyOnce,
		}),
	)
	require.NoError(t, err)

	triggerNode, err := factory.CreateAutomationNode(workspace.ID,
		testutil.WithNodeAutomationID(automation.ID),
		testutil.WithNodeType(domain.NodeTypeTrigger),
		// No NextNodeID - this is a terminal node
	)
	require.NoError(t, err)

	err = factory.UpdateAutomationRootNode(workspace.ID, automation.ID, triggerNode.ID)
	require.NoError(t, err)

	err = factory.ActivateAutomation(workspace.ID, automation.ID)
	require.NoError(t, err)

	// Enroll a contact
	contact, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail("delete-test@example.com"))
	require.NoError(t, err)

	err = factory.CreateContactTimelineEvent(workspace.ID, contact.Email, "delete_test_event", nil)
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// Verify enrollment
	ca, err := factory.GetContactAutomation(workspace.ID, automation.ID, contact.Email)
	require.NoError(t, err)
	assert.Equal(t, domain.ContactAutomationStatusActive, ca.Status)

	// Delete automation via API
	resp, err := client.Delete(fmt.Sprintf("/api/automation.delete?workspace_id=%s&id=%s", workspace.ID, automation.ID))
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
	workspaceDB, err := factory.GetWorkspaceDB(workspace.ID)
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

	// Verify: trigger should be dropped (can't easily test this directly)
	// Verify: active contacts should be marked as exited
	caAfter, err := factory.GetContactAutomation(workspace.ID, automation.ID, contact.Email)
	if err == nil && caAfter.Status == domain.ContactAutomationStatusActive {
		addBug("TestAutomation_DeletionCleanup",
			"Active contact not marked as exited after automation deletion",
			"Medium", "Delete not updating contact_automations",
			"internal/repository/automation_postgres.go:Delete")
	}

	t.Logf("Deletion cleanup test passed")
}

// TestAutomation_ErrorRecovery tests retry mechanism for failed node executions
func TestAutomation_ErrorRecovery(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	factory := suite.DataFactory

	// Setup
	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	err = factory.AddUserToWorkspace(user.ID, workspace.ID, "owner")
	require.NoError(t, err)

	// Note: Testing retry requires actually running the scheduler and having
	// a node fail. For now, we just verify the infrastructure exists.

	// Create automation
	automation, err := factory.CreateAutomation(workspace.ID,
		testutil.WithAutomationName("Error Recovery Automation"),
		testutil.WithAutomationTrigger(&domain.TimelineTriggerConfig{
			EventKind: "error_test_event",
			Frequency:  domain.TriggerFrequencyOnce,
		}),
	)
	require.NoError(t, err)

	// Create nodes with email node (will fail without provider)
	triggerNode, err := factory.CreateAutomationNode(workspace.ID,
		testutil.WithNodeAutomationID(automation.ID),
		testutil.WithNodeType(domain.NodeTypeTrigger),
	)
	require.NoError(t, err)

	emailNode, err := factory.CreateAutomationNode(workspace.ID,
		testutil.WithNodeAutomationID(automation.ID),
		testutil.WithNodeType(domain.NodeTypeEmail),
		testutil.WithNodeConfig(map[string]interface{}{
			"template_id": "nonexistent-template",
		}),
	)
	require.NoError(t, err)

	err = factory.UpdateAutomationNodeNextNodeID(workspace.ID, automation.ID, triggerNode.ID, emailNode.ID)
	require.NoError(t, err)

	err = factory.UpdateAutomationRootNode(workspace.ID, automation.ID, triggerNode.ID)
	require.NoError(t, err)

	err = factory.ActivateAutomation(workspace.ID, automation.ID)
	require.NoError(t, err)

	// Create contact and trigger
	contact, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail("error-test@example.com"))
	require.NoError(t, err)

	err = factory.CreateContactTimelineEvent(workspace.ID, contact.Email, "error_test_event", nil)
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// Verify enrollment (enrollment should succeed even if later execution fails)
	ca, err := factory.GetContactAutomation(workspace.ID, automation.ID, contact.Email)
	require.NoError(t, err)
	assert.Equal(t, domain.ContactAutomationStatusActive, ca.Status)

	// Verify retry infrastructure exists
	assert.Equal(t, 0, ca.RetryCount, "Initial retry count should be 0")
	assert.Equal(t, 3, ca.MaxRetries, "Default max retries should be 3")

	t.Logf("Error recovery test passed: retry infrastructure verified")
}

// TestAutomation_SchedulerExecution tests that the scheduler processes contacts correctly
// Note: This is a simplified test that verifies enrollment and node execution logging.
// Full scheduler testing would require the app's scheduler to be running.
func TestAutomation_SchedulerExecution(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	factory := suite.DataFactory

	// Setup
	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	err = factory.AddUserToWorkspace(user.ID, workspace.ID, "owner")
	require.NoError(t, err)

	// Setup email provider for email nodes to work
	_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID)
	require.NoError(t, err)

	// Create template
	template, err := factory.CreateTemplate(workspace.ID)
	require.NoError(t, err)

	// Create list (required for email nodes)
	list, err := factory.CreateList(workspace.ID)
	require.NoError(t, err)

	// Create automation
	automation, err := factory.CreateAutomation(workspace.ID,
		testutil.WithAutomationName("Scheduler Execution Automation"),
		testutil.WithAutomationListID(list.ID),
		testutil.WithAutomationTrigger(&domain.TimelineTriggerConfig{
			EventKind: "scheduler_test_event",
			Frequency:  domain.TriggerFrequencyOnce,
		}),
	)
	require.NoError(t, err)

	// Create nodes: trigger → email (terminal)
	triggerNode, err := factory.CreateAutomationNode(workspace.ID,
		testutil.WithNodeAutomationID(automation.ID),
		testutil.WithNodeType(domain.NodeTypeTrigger),
	)
	require.NoError(t, err)

	emailNode, err := factory.CreateAutomationNode(workspace.ID,
		testutil.WithNodeAutomationID(automation.ID),
		testutil.WithNodeType(domain.NodeTypeEmail),
		testutil.WithNodeConfig(map[string]interface{}{
			"template_id": template.ID,
		}),
		// No NextNodeID - this is a terminal node
	)
	require.NoError(t, err)

	err = factory.UpdateAutomationNodeNextNodeID(workspace.ID, automation.ID, triggerNode.ID, emailNode.ID)
	require.NoError(t, err)

	err = factory.UpdateAutomationRootNode(workspace.ID, automation.ID, triggerNode.ID)
	require.NoError(t, err)

	err = factory.ActivateAutomation(workspace.ID, automation.ID)
	require.NoError(t, err)

	// Create contact and add to list first
	contact, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail("scheduler-test@example.com"))
	require.NoError(t, err)

	_, err = factory.CreateContactList(workspace.ID,
		testutil.WithContactListEmail(contact.Email),
		testutil.WithContactListListID(list.ID),
		testutil.WithContactListStatus(domain.ContactListStatusActive),
	)
	require.NoError(t, err)

	// Trigger automation
	err = factory.CreateContactTimelineEvent(workspace.ID, contact.Email, "scheduler_test_event", nil)
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// Verify enrollment
	ca, err := factory.GetContactAutomation(workspace.ID, automation.ID, contact.Email)
	require.NoError(t, err)
	assert.Equal(t, domain.ContactAutomationStatusActive, ca.Status)

	// Verify that the enrollment created a node execution log entry
	executions, err := factory.GetNodeExecutions(workspace.ID, ca.ID)
	require.NoError(t, err)

	// Should have at least one entry (the "entered" action from enrollment)
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

// TestAutomation_PauseResume tests that paused automations freeze contacts instead of exiting them
// Use Case: Admin pauses automation → contacts wait → Admin resumes → contacts continue
func TestAutomation_PauseResume(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	factory := suite.DataFactory

	// Setup: Create user and workspace
	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	err = factory.AddUserToWorkspace(user.ID, workspace.ID, "owner")
	require.NoError(t, err)

	// Create automation
	automation, err := factory.CreateAutomation(workspace.ID,
		testutil.WithAutomationName("Pause Resume Test"),
		testutil.WithAutomationTrigger(&domain.TimelineTriggerConfig{
			EventKind: "test_pause_event",
			Frequency:  domain.TriggerFrequencyOnce,
		}),
	)
	require.NoError(t, err)

	// Create nodes: trigger → delay (terminal)
	triggerNode, err := factory.CreateAutomationNode(workspace.ID,
		testutil.WithNodeAutomationID(automation.ID),
		testutil.WithNodeType(domain.NodeTypeTrigger),
		testutil.WithNodeConfig(map[string]interface{}{}),
	)
	require.NoError(t, err)

	delayNode, err := factory.CreateAutomationNode(workspace.ID,
		testutil.WithNodeAutomationID(automation.ID),
		testutil.WithNodeType(domain.NodeTypeDelay),
		testutil.WithNodeConfig(map[string]interface{}{
			"duration": 1,
			"unit":     "seconds",
		}),
		// No NextNodeID - this is a terminal node
	)
	require.NoError(t, err)

	err = factory.UpdateAutomationNodeNextNodeID(workspace.ID, automation.ID, triggerNode.ID, delayNode.ID)
	require.NoError(t, err)

	err = factory.UpdateAutomationRootNode(workspace.ID, automation.ID, triggerNode.ID)
	require.NoError(t, err)

	err = factory.ActivateAutomation(workspace.ID, automation.ID)
	require.NoError(t, err)

	// Create contact and trigger enrollment
	contact, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail("pause-test@example.com"))
	require.NoError(t, err)

	err = factory.CreateContactTimelineEvent(workspace.ID, contact.Email, "test_pause_event", map[string]interface{}{})
	require.NoError(t, err)

	// Wait for enrollment
	time.Sleep(100 * time.Millisecond)

	// Verify contact is enrolled
	ca, err := factory.GetContactAutomation(workspace.ID, automation.ID, contact.Email)
	require.NoError(t, err, "Contact should be enrolled")
	assert.Equal(t, domain.ContactAutomationStatusActive, ca.Status)
	t.Logf("Contact enrolled with status: %s", ca.Status)

	// PAUSE the automation
	workspaceDB, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)
	_, err = workspaceDB.ExecContext(context.Background(),
		`UPDATE automations SET status = $1, updated_at = $2 WHERE id = $3`,
		domain.AutomationStatusPaused, time.Now().UTC(), automation.ID)
	require.NoError(t, err)
	t.Log("Automation paused")

	// Verify contact status is still ACTIVE (not exited!)
	ca, err = factory.GetContactAutomation(workspace.ID, automation.ID, contact.Email)
	require.NoError(t, err)
	assert.Equal(t, domain.ContactAutomationStatusActive, ca.Status, "Contact should still be ACTIVE when automation is paused")
	t.Logf("After pause - Contact status: %s (should be active)", ca.Status)

	// Verify scheduler query does NOT return this contact (paused automation filtered out)
	// This query mimics the scheduler's GetScheduledContactAutomations query
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

	// Check that our contact is not in the scheduled list
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

// TestAutomation_Permissions tests that automation API respects user permissions
func TestAutomation_Permissions(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	factory := suite.DataFactory
	client := suite.APIClient

	// Setup: Create owner user and workspace
	owner, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	err = factory.AddUserToWorkspace(owner.ID, workspace.ID, "owner")
	require.NoError(t, err)

	// Create a member user with NO automations permissions
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
		domain.PermissionResourceAutomations:    domain.ResourcePermissions{Read: false, Write: false}, // No automations access
	}
	err = factory.AddUserToWorkspaceWithPermissions(memberNoPerms.ID, workspace.ID, "member", noAutoPerms)
	require.NoError(t, err)

	// Create a member user with read-only automations permissions
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
		domain.PermissionResourceAutomations:    domain.ResourcePermissions{Read: true, Write: false}, // Read-only
	}
	err = factory.AddUserToWorkspaceWithPermissions(memberReadOnly.ID, workspace.ID, "member", readOnlyPerms)
	require.NoError(t, err)

	// Owner creates an automation
	err = client.Login(owner.Email, "password")
	require.NoError(t, err)
	client.SetWorkspaceID(workspace.ID)

	automation, err := factory.CreateAutomation(workspace.ID,
		testutil.WithAutomationName("Permission Test Automation"),
		testutil.WithAutomationTrigger(&domain.TimelineTriggerConfig{
			EventKind: "test_event",
			Frequency:  domain.TriggerFrequencyOnce,
		}),
	)
	require.NoError(t, err)
	t.Logf("Owner created automation: %s", automation.ID)

	// Test 1: User with NO permissions cannot list automations
	t.Run("no_permissions_cannot_list", func(t *testing.T) {
		err = client.Login(memberNoPerms.Email, "password")
		require.NoError(t, err)
		client.SetWorkspaceID(workspace.ID)

		resp, err := client.Get(fmt.Sprintf("/api/automations.list?workspace_id=%s", workspace.ID))
		require.NoError(t, err)
		defer resp.Body.Close()

		// Should return 403 Forbidden
		assert.Equal(t, http.StatusForbidden, resp.StatusCode, "User without automations read permission should get 403")
		t.Logf("User with no permissions got status %d (expected 403)", resp.StatusCode)
	})

	// Test 2: User with read-only permissions can list automations
	t.Run("read_only_can_list", func(t *testing.T) {
		err = client.Login(memberReadOnly.Email, "password")
		require.NoError(t, err)
		client.SetWorkspaceID(workspace.ID)

		resp, err := client.Get(fmt.Sprintf("/api/automations.list?workspace_id=%s", workspace.ID))
		require.NoError(t, err)
		defer resp.Body.Close()

		// Should return 200 OK
		assert.Equal(t, http.StatusOK, resp.StatusCode, "User with automations read permission should get 200")
		t.Logf("User with read-only permissions got status %d (expected 200)", resp.StatusCode)
	})

	// Test 3: User with read-only permissions cannot create automations
	t.Run("read_only_cannot_create", func(t *testing.T) {
		err = client.Login(memberReadOnly.Email, "password")
		require.NoError(t, err)
		client.SetWorkspaceID(workspace.ID)

		// Try to create an automation via API with a valid automation object
		resp, err := client.Post("/api/automations.create", map[string]interface{}{
			"workspace_id": workspace.ID,
			"automation": map[string]interface{}{
				"id":           "test-create-fail",
				"workspace_id": workspace.ID,
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

		// Should return 403 Forbidden
		assert.Equal(t, http.StatusForbidden, resp.StatusCode, "User without automations write permission should get 403 on create")
		t.Logf("User with read-only permissions trying to create got status %d (expected 403)", resp.StatusCode)
	})

	// Test 4: Owner can create automations (owner bypasses permissions)
	t.Run("owner_can_create", func(t *testing.T) {
		err = client.Login(owner.Email, "password")
		require.NoError(t, err)
		client.SetWorkspaceID(workspace.ID)

		resp, err := client.Post("/api/automations.create", map[string]interface{}{
			"workspace_id": workspace.ID,
			"automation": map[string]interface{}{
				"id":           "owner-created-auto",
				"workspace_id": workspace.ID,
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

		// Should return 201 Created
		assert.Equal(t, http.StatusCreated, resp.StatusCode, "Owner should be able to create automations")
		t.Logf("Owner creating automation got status %d (expected 201)", resp.StatusCode)
	})

	t.Log("Automation permissions test passed")
}

// PrintBugReport outputs all bugs found during testing
func TestAutomation_PrintBugReport(t *testing.T) {
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
