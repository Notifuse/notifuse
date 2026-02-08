package integration

import (
	"context"
	"strings"
	"testing"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAutomationStatsScalarBug reproduces the bug where automation enrollment fails
// with "pq: cannot set path in scalar" when the automation's stats field contains
// a scalar JSONB value instead of an object.
//
// Root cause: The automation_enroll_contact function uses:
//
//	SET stats = jsonb_set(COALESCE(stats, '{}'::jsonb), '{enrolled}', ...)
//
// COALESCE handles SQL NULL, but NOT a stored JSONB scalar value.
// If stats contains a scalar like 0, "null", or true, jsonb_set fails.
func TestAutomationStatsScalarBug(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	factory := suite.DataFactory

	// Setup: Create user, workspace, and add user to workspace
	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	err = factory.AddUserToWorkspace(user.ID, workspace.ID, "owner")
	require.NoError(t, err)

	// Setup email provider (required for automation)
	_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID)
	require.NoError(t, err)

	workspaceID := workspace.ID

	t.Run("Scalar stats value causes enrollment failure", func(t *testing.T) {
		// 1. Create a list
		list, err := factory.CreateList(workspaceID, testutil.WithListName("Bug Test List"))
		require.NoError(t, err)

		// 2. Create email template (required for email node)
		template, err := factory.CreateTemplate(workspaceID, testutil.WithTemplateName("Bug Test Email"))
		require.NoError(t, err)

		// 3. Create automation with trigger on list.subscribed
		automation, err := factory.CreateAutomation(workspaceID,
			testutil.WithAutomationName("Bug Test Automation"),
			testutil.WithAutomationListID(list.ID),
			testutil.WithAutomationTrigger(&domain.TimelineTriggerConfig{
				EventKind: "list.subscribed",
				Frequency: domain.TriggerFrequencyEveryTime,
			}),
		)
		require.NoError(t, err)

		// 4. Create nodes: trigger → email (terminal)
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

		// Link nodes
		err = factory.UpdateAutomationNodeNextNodeID(workspaceID, automation.ID, triggerNode.ID, emailNode.ID)
		require.NoError(t, err)

		// Set root node
		err = factory.UpdateAutomationRootNode(workspaceID, automation.ID, triggerNode.ID)
		require.NoError(t, err)

		// 5. CORRUPT the stats field with a scalar value (simulate the bug)
		workspaceDB, err := factory.GetWorkspaceDB(workspaceID)
		require.NoError(t, err)

		// Set stats to a scalar JSONB value (0) - this is the bug condition
		_, err = workspaceDB.ExecContext(context.Background(),
			`UPDATE automations SET stats = '0'::jsonb WHERE id = $1`,
			automation.ID,
		)
		require.NoError(t, err)

		// Verify the corruption
		var statsType string
		err = workspaceDB.QueryRowContext(context.Background(),
			`SELECT jsonb_typeof(stats) FROM automations WHERE id = $1`,
			automation.ID,
		).Scan(&statsType)
		require.NoError(t, err)
		assert.Equal(t, "number", statsType, "Stats should be a scalar number (corrupted state)")

		// 6. Activate automation (creates DB trigger)
		err = factory.ActivateAutomation(workspaceID, automation.ID)
		require.NoError(t, err)

		// 7. Create a contact
		contact, err := factory.CreateContact(workspaceID,
			testutil.WithContactEmail("scalar-bug-test@example.com"),
			testutil.WithContactName("Bug", "Test"),
		)
		require.NoError(t, err)

		// 8. Add contact to list - this should trigger the automation enrollment
		// and fail with "cannot set path in scalar" because stats is a scalar
		_, err = factory.CreateContactList(workspaceID,
			testutil.WithContactListEmail(contact.Email),
			testutil.WithContactListListID(list.ID),
			testutil.WithContactListStatus(domain.ContactListStatusActive),
		)

		// The bug manifests here - the error should contain "cannot set path in scalar"
		if err != nil {
			assert.True(t, strings.Contains(err.Error(), "cannot set path in scalar"),
				"Expected 'cannot set path in scalar' error, got: %v", err)
			t.Logf("BUG CONFIRMED: %v", err)
		} else {
			// If no error, check if the contact was actually enrolled
			// (the trigger might have silently failed)
			ca, _ := factory.GetContactAutomation(workspaceID, automation.ID, contact.Email)
			if ca == nil {
				t.Log("BUG CONFIRMED: Contact not enrolled (trigger failed silently)")
			} else {
				t.Log("Bug may have been fixed - contact was enrolled successfully")
			}
		}
	})

	t.Run("Null JSONB value also causes enrollment failure", func(t *testing.T) {
		// Same test but with null JSONB value instead of 0
		list, err := factory.CreateList(workspaceID, testutil.WithListName("Null Stats Test List"))
		require.NoError(t, err)

		template, err := factory.CreateTemplate(workspaceID, testutil.WithTemplateName("Null Stats Email"))
		require.NoError(t, err)

		automation, err := factory.CreateAutomation(workspaceID,
			testutil.WithAutomationName("Null Stats Automation"),
			testutil.WithAutomationListID(list.ID),
			testutil.WithAutomationTrigger(&domain.TimelineTriggerConfig{
				EventKind: "list.subscribed",
				Frequency: domain.TriggerFrequencyEveryTime,
			}),
		)
		require.NoError(t, err)

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

		err = factory.UpdateAutomationNodeNextNodeID(workspaceID, automation.ID, triggerNode.ID, emailNode.ID)
		require.NoError(t, err)

		err = factory.UpdateAutomationRootNode(workspaceID, automation.ID, triggerNode.ID)
		require.NoError(t, err)

		// Corrupt stats with JSONB null
		workspaceDB, err := factory.GetWorkspaceDB(workspaceID)
		require.NoError(t, err)

		_, err = workspaceDB.ExecContext(context.Background(),
			`UPDATE automations SET stats = 'null'::jsonb WHERE id = $1`,
			automation.ID,
		)
		require.NoError(t, err)

		// Verify corruption
		var statsType string
		err = workspaceDB.QueryRowContext(context.Background(),
			`SELECT jsonb_typeof(stats) FROM automations WHERE id = $1`,
			automation.ID,
		).Scan(&statsType)
		require.NoError(t, err)
		assert.Equal(t, "null", statsType, "Stats should be JSONB null (corrupted state)")

		err = factory.ActivateAutomation(workspaceID, automation.ID)
		require.NoError(t, err)

		contact, err := factory.CreateContact(workspaceID,
			testutil.WithContactEmail("null-stats-test@example.com"),
			testutil.WithContactName("Null", "Test"),
		)
		require.NoError(t, err)

		_, err = factory.CreateContactList(workspaceID,
			testutil.WithContactListEmail(contact.Email),
			testutil.WithContactListListID(list.ID),
			testutil.WithContactListStatus(domain.ContactListStatusActive),
		)

		if err != nil {
			assert.True(t, strings.Contains(err.Error(), "cannot set path in scalar"),
				"Expected 'cannot set path in scalar' error, got: %v", err)
			t.Logf("BUG CONFIRMED with null stats: %v", err)
		} else {
			ca, _ := factory.GetContactAutomation(workspaceID, automation.ID, contact.Email)
			if ca == nil {
				t.Log("BUG CONFIRMED: Contact not enrolled with null stats (trigger failed silently)")
			} else {
				t.Log("Bug may have been fixed - contact was enrolled successfully with null stats")
			}
		}
	})
}
