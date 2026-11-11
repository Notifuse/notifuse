package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// conditionalTemplateTree is a visual template tree: a text block always shown, and a
// block of blockType under condition.
func conditionalTemplateTree(blockType string, condition map[string]interface{}) map[string]interface{} {
	conditional := map[string]interface{}{"id": "conditional", "type": blockType, "display_condition": condition}
	switch blockType {
	case "mj-image":
		conditional["attributes"] = map[string]interface{}{"src": "https://cdn.example.com/conditional-image.png"}
	case "mj-section":
		conditional["children"] = []interface{}{map[string]interface{}{"id": "inner-col", "type": "mj-column", "children": []interface{}{
			map[string]interface{}{"id": "inner-text", "type": "mj-text", "content": "<p>VIP-ONLY-SECTION</p>"},
		}}}
	}
	always := map[string]interface{}{"id": "always-sec", "type": "mj-section", "children": []interface{}{
		map[string]interface{}{"id": "always-col", "type": "mj-column", "children": []interface{}{
			map[string]interface{}{"id": "always-text", "type": "mj-text", "content": "<p>ALWAYS-SHOWN</p>"},
		}},
	}}
	body := []interface{}{always}
	if blockType == "mj-section" {
		body = append(body, conditional)
	} else {
		always["children"].([]interface{})[0].(map[string]interface{})["children"] = append(
			always["children"].([]interface{})[0].(map[string]interface{})["children"].([]interface{}), conditional)
	}
	return map[string]interface{}{"id": "root", "type": "mjml", "children": []interface{}{
		map[string]interface{}{"id": "body", "type": "mj-body", "children": body},
	}}
}

func createVisualTemplate(t *testing.T, client *testutil.APIClient, workspaceID, id string, tree map[string]interface{}, translations map[string]interface{}) (int, string) {
	t.Helper()
	template := map[string]interface{}{
		"workspace_id": workspaceID, "id": id, "name": "Conditional " + id, "channel": "email", "category": "marketing",
		"email": map[string]interface{}{"subject": "Conditional " + id, "visual_editor_tree": tree},
	}
	if translations != nil {
		template["translations"] = translations
	}
	resp, err := client.CreateTemplate(template)
	require.NoError(t, err)
	raw, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	return resp.StatusCode, string(raw)
}

// TestDisplayCondition_TransactionalE2E sends a visual template whose image has a
// display condition on the notification's data, through SMTP to Mailpit.
func TestDisplayCondition_TransactionalE2E(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()
	client := suite.APIClient
	factory := suite.DataFactory

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))
	_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID, smtpToMailpit("Conditions"))
	require.NoError(t, err)
	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(workspace.ID)
	require.NoError(t, testutil.ClearMailpitMessages(t))

	t.Run("templates.create refuses an invalid condition", func(t *testing.T) {
		status, body := createVisualTemplate(t, client, workspace.ID, "cond-invalid", conditionalTemplateTree("mj-image",
			map[string]interface{}{"match": "all", "rules": []interface{}{
				map[string]interface{}{"variable": "order.has_image", "operator": "between", "value": "1"},
			}}), nil)
		assert.Equal(t, http.StatusBadRequest, status, body)
		assert.Contains(t, body, "conditional")

		status, body = createVisualTemplate(t, client, workspace.ID, "cond-column", map[string]interface{}{
			"id": "root", "type": "mjml", "children": []interface{}{map[string]interface{}{"id": "body", "type": "mj-body", "children": []interface{}{
				map[string]interface{}{"id": "s", "type": "mj-section", "children": []interface{}{
					map[string]interface{}{"id": "col-1", "type": "mj-column", "children": []interface{}{},
						"display_condition": map[string]interface{}{"match": "all", "rules": []interface{}{
							map[string]interface{}{"variable": "a", "operator": "is_true"}}}},
				}},
			}}},
		}, nil)
		assert.Equal(t, http.StatusBadRequest, status, body)
		assert.Contains(t, body, "col-1")
	})

	status, body := createVisualTemplate(t, client, workspace.ID, "cond-image", conditionalTemplateTree("mj-image",
		map[string]interface{}{"match": "all", "rules": []interface{}{
			map[string]interface{}{"variable": "order.has_image", "operator": "is_true"},
		}}), nil)
	require.Equal(t, http.StatusCreated, status, body)

	notification, err := factory.CreateTransactionalNotification(workspace.ID,
		testutil.WithTransactionalNotificationChannels(domain.ChannelTemplates{
			domain.TransactionalChannelEmail: {TemplateID: "cond-image", Settings: map[string]interface{}{}},
		}))
	require.NoError(t, err)

	tag := uuid.New().String()[:8]
	send := func(t *testing.T, email string, hasImage bool) string {
		t.Helper()
		resp, err := client.SendTransactionalNotification(map[string]interface{}{
			"id":       notification.ID,
			"contact":  map[string]interface{}{"email": email},
			"channels": []string{"email"},
			"data":     map[string]interface{}{"order": map[string]interface{}{"has_image": hasImage}},
		})
		require.NoError(t, err)
		raw, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode, string(raw))
		message, err := waitForEmailByRecipientAddr(t, email, 30*time.Second)
		require.NoError(t, err)
		return message.HTML
	}

	shown := send(t, fmt.Sprintf("cond-shown-%s@example.com", tag), true)
	hidden := send(t, fmt.Sprintf("cond-hidden-%s@example.com", tag), false)
	assert.Contains(t, shown, "conditional-image.png")
	assert.NotContains(t, hidden, "conditional-image.png")
	assert.Contains(t, shown, "ALWAYS-SHOWN")
	assert.Contains(t, hidden, "ALWAYS-SHOWN")
}

// TestDisplayCondition_BroadcastE2E broadcasts a template whose section only VIP
// contacts see.
func TestDisplayCondition_BroadcastE2E(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()
	client := suite.APIClient
	factory := suite.DataFactory

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))
	_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID, smtpToMailpit("Conditions Broadcast"))
	require.NoError(t, err)

	workerCtx, cancelWorker := context.WithCancel(context.Background())
	defer cancelWorker()
	require.NoError(t, suite.ServerManager.StartBackgroundWorkers(workerCtx))

	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(workspace.ID)
	require.NoError(t, testutil.ClearMailpitMessages(t))

	list, err := factory.CreateList(workspace.ID, testutil.WithListName("Conditions List"))
	require.NoError(t, err)
	tag := uuid.New().String()[:8]
	vip := fmt.Sprintf("cond-vip-%s@example.com", tag)
	regular := fmt.Sprintf("cond-regular-%s@example.com", tag)
	for email, segment := range map[string]string{vip: "vip", regular: "regular"} {
		status, body := postStatus(t, client, "/api/contacts.upsert", map[string]interface{}{
			"workspace_id": workspace.ID,
			"contact":      map[string]interface{}{"email": email, "custom_string_1": segment},
		})
		require.Equal(t, http.StatusOK, status, body)
		_, err = factory.CreateContactList(workspace.ID,
			testutil.WithContactListEmail(email),
			testutil.WithContactListListID(list.ID),
			testutil.WithContactListStatus(domain.ContactListStatusActive))
		require.NoError(t, err)
	}

	status, body := createVisualTemplate(t, client, workspace.ID, "cond-vip", conditionalTemplateTree("mj-section",
		map[string]interface{}{"match": "all", "rules": []interface{}{
			map[string]interface{}{"variable": "contact.custom_string_1", "operator": "eq", "value": "vip"},
		}}), nil)
	require.Equal(t, http.StatusCreated, status, body)

	broadcast, err := factory.CreateBroadcast(workspace.ID,
		testutil.WithBroadcastName("Conditions Broadcast"),
		testutil.WithBroadcastTemplateID("cond-vip"),
		testutil.WithBroadcastAudience(domain.AudienceSettings{List: list.ID, ExcludeUnsubscribed: true}))
	require.NoError(t, err)
	resp, err := client.ScheduleBroadcast(map[string]interface{}{"workspace_id": workspace.ID, "id": broadcast.ID, "send_now": true})
	require.NoError(t, err)
	_ = resp.Body.Close()
	_, err = testutil.WaitForBroadcastStatusWithExecution(t, client, broadcast.ID, []string{"processed", "completed", "sent"}, 60*time.Second)
	require.NoError(t, err)

	vipEmail, err := waitForEmailByRecipientAddr(t, vip, 30*time.Second)
	require.NoError(t, err)
	regularEmail, err := waitForEmailByRecipientAddr(t, regular, 30*time.Second)
	require.NoError(t, err)
	assert.Contains(t, vipEmail.HTML, "VIP-ONLY-SECTION")
	assert.NotContains(t, regularEmail.HTML, "VIP-ONLY-SECTION")
	assert.Contains(t, regularEmail.HTML, "ALWAYS-SHOWN")
}

// TestDisplayCondition_TemplateRoundTrip saves conditions, including a translation's
// own, and reads them back unchanged through get, update and get.
func TestDisplayCondition_TemplateRoundTrip(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()
	client := suite.APIClient
	factory := suite.DataFactory

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace(testutil.WithWorkspaceDefaultLanguage("en", []string{"en", "fr"}))
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))
	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(workspace.ID)

	condition := map[string]interface{}{"match": "any", "rules": []interface{}{
		map[string]interface{}{"variable": "contact.custom_number_1", "operator": "gte", "value": "100"},
		map[string]interface{}{"variable": "contact.first_name", "operator": "is_present"},
	}}
	frCondition := map[string]interface{}{"match": "all", "rules": []interface{}{
		map[string]interface{}{"variable": "contact.custom_string_1", "operator": "contains", "value": "R&D <x>"},
	}}
	status, body := createVisualTemplate(t, client, workspace.ID, "cond-roundtrip", conditionalTemplateTree("mj-image", condition),
		map[string]interface{}{"fr": map[string]interface{}{"email": map[string]interface{}{
			"subject": "FR", "visual_editor_tree": conditionalTemplateTree("mj-image", frCondition),
		}}})
	require.Equal(t, http.StatusCreated, status, body)

	get := func(t *testing.T) map[string]interface{} {
		t.Helper()
		resp, err := client.Get("/api/templates.get", map[string]string{"workspace_id": workspace.ID, "id": "cond-roundtrip"})
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		var result struct {
			Template map[string]interface{} `json:"template"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
		return result.Template
	}
	conditionOf := func(email map[string]interface{}) interface{} {
		tree := email["visual_editor_tree"].(map[string]interface{})
		body := tree["children"].([]interface{})[0].(map[string]interface{})
		column := body["children"].([]interface{})[0].(map[string]interface{})["children"].([]interface{})[0].(map[string]interface{})
		return column["children"].([]interface{})[1].(map[string]interface{})["display_condition"]
	}
	check := func(t *testing.T, template map[string]interface{}) {
		t.Helper()
		assert.Equal(t, condition, conditionOf(template["email"].(map[string]interface{})))
		fr := template["translations"].(map[string]interface{})["fr"].(map[string]interface{})
		assert.Equal(t, frCondition, conditionOf(fr["email"].(map[string]interface{})))
	}

	stored := get(t)
	check(t, stored)

	stored["workspace_id"] = workspace.ID
	stored["name"] = "Conditional roundtrip, renamed"
	resp, err := client.Post("/api/templates.update", stored)
	require.NoError(t, err)
	raw, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, string(raw))
	updated := get(t)
	check(t, updated)
	assert.True(t, strings.HasPrefix(updated["name"].(string), "Conditional roundtrip"))
}
