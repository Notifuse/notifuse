package integration

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/migrations"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/google/uuid"
	shortuuid "github.com/lithammer/shortuuid/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAutomationTriggerEvent sends a custom event through the API, as an application would
// when a cart is abandoned, and reads the automation email Mailpit received: it lists the
// cart's items, read by the template from {{ trigger_event }}.
//
// Until 44.0 an automation email received only the contact, the list and the automation's
// name, so a template could not show anything about the event that started the journey.
func TestAutomationTriggerEvent(t *testing.T) {
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

	list, err := factory.CreateList(workspace.ID)
	require.NoError(t, err)
	template, err := factory.CreateTemplate(workspace.ID, testutil.WithTemplateEmailContent(
		`{% if trigger_event.properties.items %}CART:{% for item in trigger_event.properties.items %}[{{ item.name }} x{{ item.quantity }}]{% endfor %}`+
			` FROM:{{ trigger_event.name }}/{{ trigger_event.external_id }}{% else %}NO-ITEMS{% endif %}`))
	require.NoError(t, err)

	automationID := shortuuid.New()
	eventName := "cart_abandoned_" + uuid.New().String()[:8]
	createActiveAutomation(t, client, workspace.ID, automationID, list.ID, eventName, template.ID)

	// member creates a contact on the automation's list.
	member := func(t *testing.T) string {
		t.Helper()
		email := fmt.Sprintf("cart-%s@example.com", uuid.New().String()[:8])
		_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
		require.NoError(t, err)
		_, err = factory.CreateContactList(workspace.ID,
			testutil.WithContactListEmail(email),
			testutil.WithContactListListID(list.ID),
			testutil.WithContactListStatus(domain.ContactListStatusActive))
		require.NoError(t, err)
		return email
	}

	// track posts the event the way an application does, as raw JSON.
	track := func(t *testing.T, email, externalID string, properties map[string]interface{}) {
		t.Helper()
		resp, err := client.Post("/api/customEvents.upsert", map[string]interface{}{
			"workspace_id": workspace.ID,
			"email":        email,
			"event_name":   eventName,
			"external_id":  externalID,
			"properties":   properties,
		})
		require.NoError(t, err)
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		require.Less(t, resp.StatusCode, 300, "customEvents.upsert: %s", body)
	}

	t.Run("the email lists the items of the event that started the journey", func(t *testing.T) {
		email := member(t)
		externalID := "cart-" + uuid.New().String()[:8]
		track(t, email, externalID, map[string]interface{}{
			"items": []map[string]interface{}{
				{"name": "Linen shirt", "quantity": 1},
				{"name": "Canvas tote bag", "quantity": 2},
			},
		})

		msg, err := testutil.WaitForMailpitMessageByRecipient(t, email, 30*time.Second)
		require.NoError(t, err, "the automation email should reach Mailpit")
		assert.Contains(t, msg.HTML, "CART:[Linen shirt x1][Canvas tote bag x2]")
		assert.Contains(t, msg.HTML, "FROM:"+eventName+"/"+externalID)
	})

	t.Run("an event without the properties the template reads still sends", func(t *testing.T) {
		email := member(t)
		track(t, email, "cart-"+uuid.New().String()[:8], map[string]interface{}{"total": 12})

		msg, err := testutil.WaitForMailpitMessageByRecipient(t, email, 30*time.Second)
		require.NoError(t, err, "the automation email should reach Mailpit")
		assert.Contains(t, msg.HTML, "NO-ITEMS")
		assert.NotContains(t, msg.HTML, "CART:")
	})

	// A workspace upgraded to 44 has triggers installed before it, which call the four-argument
	// enrolment: they enrol without the event until v44 replaces their bodies.
	t.Run("v44 replaces the body of a trigger installed before it so it passes the event", func(t *testing.T) {
		workspaceDB, err := factory.GetWorkspaceDB(workspace.ID)
		require.NoError(t, err)
		function := "automation_trigger_" + strings.ReplaceAll(automationID, "-", "")

		// The body every trigger had before 44.0: no event, the four-argument call.
		_, err = workspaceDB.ExecContext(context.Background(), fmt.Sprintf(`CREATE OR REPLACE FUNCTION %s() RETURNS TRIGGER AS $$
BEGIN
    PERFORM automation_enroll_contact('%s', NEW.email, (SELECT root_node_id FROM automations WHERE id = '%s'), 'every_time');
    RETURN NEW;
END;
$$ LANGUAGE plpgsql`, strings.ToLower(function), automationID, automationID))
		require.NoError(t, err)

		before := member(t)
		track(t, before, "cart-"+uuid.New().String()[:8], map[string]interface{}{"items": []map[string]interface{}{{"name": "Linen shirt", "quantity": 1}}})
		msg, err := testutil.WaitForMailpitMessageByRecipient(t, before, 30*time.Second)
		require.NoError(t, err)
		assert.Contains(t, msg.HTML, "NO-ITEMS", "a trigger from before 44 enrols without the event")

		tx, err := workspaceDB.BeginTx(context.Background(), nil)
		require.NoError(t, err)
		require.NoError(t, (&migrations.V44Migration{}).UpdateWorkspace(context.Background(), &config.Config{}, workspace, tx))
		require.NoError(t, tx.Commit())

		after := member(t)
		track(t, after, "cart-"+uuid.New().String()[:8], map[string]interface{}{"items": []map[string]interface{}{{"name": "Linen shirt", "quantity": 1}}})
		msg, err = testutil.WaitForMailpitMessageByRecipient(t, after, 30*time.Second)
		require.NoError(t, err)
		assert.Contains(t, msg.HTML, "CART:[Linen shirt x1]")
	})
}
