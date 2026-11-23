package integration

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// The write tools of contacts, custom events and lists through /mcp: each one
// changes real state, a replay answers as spec §5.8 says, a key lacking a pair
// neither sees nor runs the tool, and every call leaves an audit row with
// channel mcp (the harness licence carries audit_logs).
func TestMCPWriteToolsContactsAndLists(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, mcpAppFactory)
	defer func() { suite.Cleanup() }()
	suite.Config.APIEndpoint = suite.ServerManager.GetURL()

	factory := suite.DataFactory
	authService := suite.ServerManager.GetApp().GetAuthService().(*service.AuthService)
	url := suite.ServerManager.GetURL()

	owner, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(owner.ID, workspace.ID, "owner"))
	db, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)
	ownerClient := testutil.NewAPIClient(url)
	require.NoError(t, ownerClient.Login(owner.Email, "password"))

	newKey := func(permissions domain.UserPermissions) *mcpClient {
		var options []testutil.APIKeyOption
		if permissions != nil {
			options = append(options, testutil.WithAPIKeyPermissions(permissions))
		}
		key, err := factory.CreateAPIKey(workspace.ID, options...)
		require.NoError(t, err)
		return &mcpClient{t: t, url: url, token: authService.GenerateAPIAuthToken(key)}
	}
	full := newKey(nil)
	listsOnly := newKey(domain.UserPermissions{domain.PermissionResourceLists: {Read: true, Write: true}})
	contactsOnly := newKey(domain.UserPermissions{domain.PermissionResourceContacts: {Read: true, Write: true}})
	listsWriteOnly := newKey(domain.UserPermissions{
		domain.PermissionResourceLists: {Write: true}, domain.PermissionResourceContacts: {Write: true},
	})

	args := func(fields string) string {
		return fmt.Sprintf(`{"workspace_id":%q,%s}`, workspace.ID, fields)
	}
	structured := func(t *testing.T, result mcpToolResult) map[string]any {
		t.Helper()
		require.False(t, result.IsError, string(result.StructuredContent))
		var out map[string]any
		require.NoError(t, json.Unmarshal(result.StructuredContent, &out))
		return out
	}
	toolError := func(t *testing.T, result mcpToolResult) map[string]any {
		t.Helper()
		require.True(t, result.IsError, string(result.StructuredContent))
		var out struct {
			Error map[string]any `json:"error"`
		}
		require.NoError(t, json.Unmarshal(result.StructuredContent, &out))
		return out.Error
	}
	membership := func(t *testing.T, email, listID string) string {
		t.Helper()
		var status string
		var removed bool
		err := db.QueryRow(`SELECT status, deleted_at IS NOT NULL FROM contact_lists WHERE email = $1 AND list_id = $2`, email, listID).Scan(&status, &removed)
		if err == sql.ErrNoRows {
			return "none"
		}
		require.NoError(t, err)
		if removed {
			return "removed " + status
		}
		return status
	}
	// auditRows reads the MCP rows of an action on a target, newest first.
	auditRows := func(t *testing.T, action, targetID string) []*domain.AuditLog {
		t.Helper()
		page := listAuditLogs(t, ownerClient, map[string]string{
			"workspace_id": workspace.ID, "actions": action, "target_id": targetID, "limit": "100",
		})
		var rows []*domain.AuditLog
		for _, row := range page.Logs {
			if row.Metadata["channel"] == "mcp" {
				rows = append(rows, row)
			}
		}
		return rows
	}
	statuses := func(rows []*domain.AuditLog) []int {
		out := []int{}
		for i := len(rows) - 1; i >= 0; i-- {
			out = append(out, rows[i].StatusCode)
		}
		return out
	}
	unique := func(prefix string) string {
		return fmt.Sprintf("%s-%s@example.com", prefix, strings.ToLower(uuid.New().String()[:8]))
	}

	t.Run("upsert_contact creates, then changes only the given fields", func(t *testing.T) {
		email := unique("mcp-upsert")
		out := structured(t, full.call("upsert_contact", args(fmt.Sprintf(`"email":%q,"first_name":"Jane","custom_string_1":"gold"`, strings.ToUpper(email)))))
		assert.Equal(t, "create", out["action"])

		out = structured(t, full.call("upsert_contact", args(fmt.Sprintf(`"email":%q,"last_name":"Doe","clear_fields":["custom_string_1"]`, email))))
		assert.Equal(t, "update", out["action"])
		var firstName, lastName sql.NullString
		var custom sql.NullString
		require.NoError(t, db.QueryRow(`SELECT first_name, last_name, custom_string_1 FROM contacts WHERE email = $1`, email).Scan(&firstName, &lastName, &custom))
		assert.Equal(t, "Jane", firstName.String, "a field left out keeps its value")
		assert.Equal(t, "Doe", lastName.String)
		assert.False(t, custom.Valid, "a cleared field is null")

		// The replay writes the same values again.
		out = structured(t, full.call("upsert_contact", args(fmt.Sprintf(`"email":%q,"last_name":"Doe","clear_fields":["custom_string_1"]`, email))))
		assert.Equal(t, "update", out["action"])
		got := structured(t, full.call("get_contact", args(fmt.Sprintf(`"email":%q`, email))))
		contact := got["contact"].(map[string]any)
		assert.Equal(t, "Jane", contact["first_name"])
		assert.Equal(t, "Doe", contact["last_name"])
		assert.NotContains(t, contact, "custom_string_1")

		assert.Equal(t, []int{200, 200, 200}, statuses(auditRows(t, "contacts.upsert", email)))
	})

	t.Run("delete_contact keeps the opt-outs and answers not_found on replay", func(t *testing.T) {
		email := unique("mcp-delete")
		_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
		require.NoError(t, err)
		list, err := factory.CreateList(workspace.ID)
		require.NoError(t, err)
		_, err = factory.CreateContactList(workspace.ID, testutil.WithContactListEmail(email), testutil.WithContactListListID(list.ID),
			testutil.WithContactListStatus(domain.ContactListStatusUnsubscribed))
		require.NoError(t, err)

		out := structured(t, full.call("delete_contact", args(fmt.Sprintf(`"email":%q`, email))))
		assert.Equal(t, true, out["deleted"])
		var contacts int
		require.NoError(t, db.QueryRow(`SELECT count(*) FROM contacts WHERE email = $1`, email).Scan(&contacts))
		assert.Equal(t, 0, contacts)
		var hashes int
		require.NoError(t, db.QueryRow(`SELECT count(*) FROM contact_opt_out_hashes`).Scan(&hashes))
		assert.Positive(t, hashes, "the unsubscribe is kept as a hash")

		assert.Equal(t, "not_found", toolError(t, full.call("delete_contact", args(fmt.Sprintf(`"email":%q`, email))))["code"])
		assert.Equal(t, []int{200, 404}, statuses(auditRows(t, "contacts.delete", email)))
	})

	t.Run("track_custom_event records once under the lowercase address", func(t *testing.T) {
		email := unique("mcp-event")
		call := args(fmt.Sprintf(`"email":%q,"event_name":"order.completed","external_id":"ord_42","occurred_at":"2026-11-20T10:00:00Z","properties":{"total":59.9}`,
			strings.ToUpper(email)))
		out := structured(t, full.call("track_custom_event", call))
		event := out["event"].(map[string]any)
		assert.Equal(t, email, event["email"])
		assert.NotContains(t, event, "source")

		// The same occurred_at again: the event is not newer, so nothing is
		// written and no timeline row is added.
		structured(t, full.call("track_custom_event", call))
		var events, timeline int
		require.NoError(t, db.QueryRow(`SELECT count(*) FROM custom_events WHERE email = $1`, email).Scan(&events))
		require.NoError(t, db.QueryRow(`SELECT count(*) FROM contact_timeline WHERE email = $1 AND kind = 'custom_event.order.completed'`, email).Scan(&timeline))
		assert.Equal(t, 1, events)
		assert.Equal(t, 1, timeline)

		rows := auditRows(t, "customEvents.upsert", email)
		assert.Equal(t, []int{200, 200}, statuses(rows))
		assert.Equal(t, "order.completed", rows[0].Metadata["event_name"])

		missing := toolError(t, full.call("track_custom_event", args(fmt.Sprintf(`"email":%q,"event_name":"order.completed","external_id":"ord_43"`, email))))
		assert.Equal(t, "validation", missing["code"], "occurred_at is required")
	})

	t.Run("create_list, update_list and delete_list, with their replays", func(t *testing.T) {
		id := "mcp" + strings.ReplaceAll(uuid.New().String()[:8], "-", "")
		created := args(fmt.Sprintf(`"id":%q,"name":"Weekly","description":"Every Monday","is_public":true`, id))
		structured(t, full.call("create_list", created))
		assert.Equal(t, "conflict", toolError(t, full.call("create_list", created))["code"])

		out := structured(t, full.call("update_list", args(fmt.Sprintf(`"id":%q,"name":"Weekly digest"`, id))))
		list := out["list"].(map[string]any)
		assert.Equal(t, "Weekly digest", list["name"])
		var name, description string
		var public bool
		require.NoError(t, db.QueryRow(`SELECT name, description, is_public FROM lists WHERE id = $1`, id).Scan(&name, &description, &public))
		assert.Equal(t, "Weekly digest", name)
		assert.Equal(t, "Every Monday", description, "a field left out keeps its value")
		assert.True(t, public)

		structured(t, full.call("delete_list", args(fmt.Sprintf(`"id":%q`, id))))
		assert.Equal(t, "not_found", toolError(t, full.call("delete_list", args(fmt.Sprintf(`"id":%q`, id))))["code"])
		assert.Equal(t, "not_found", toolError(t, full.call("update_list", args(fmt.Sprintf(`"id":%q,"name":"Gone"`, id))))["code"])

		assert.Equal(t, []int{200, 409}, statuses(auditRows(t, "lists.create", id)))
		assert.Equal(t, []int{200, 404}, statuses(auditRows(t, "lists.update", id)))
		assert.Equal(t, []int{200, 404}, statuses(auditRows(t, "lists.delete", id)))
	})

	t.Run("the subscription tools follow the opt-out and answer their replays", func(t *testing.T) {
		email := unique("mcp-subscribe")
		list, err := factory.CreateList(workspace.ID, testutil.WithListDoubleOptin(false))
		require.NoError(t, err)
		subscribe := func(resubscribe bool) mcpToolResult {
			return full.call("subscribe_contact_to_list", args(fmt.Sprintf(`"email":%q,"list_id":%q,"resubscribe":%t`, email, list.ID, resubscribe)))
		}

		out := structured(t, subscribe(false))
		assert.Equal(t, "active", out["status"])
		assert.Equal(t, "none", out["previous_status"])
		out = structured(t, subscribe(false))
		assert.Equal(t, "active", out["previous_status"], "a replay finds the contact active")

		structured(t, full.call("set_contact_list_status", args(fmt.Sprintf(`"email":%q,"list_id":%q,"status":"unsubscribed"`, email, list.ID))))
		assert.Equal(t, "unsubscribed", membership(t, email, list.ID))

		refused := toolError(t, subscribe(false))
		assert.Equal(t, "invalid_state", refused["code"])
		assert.Equal(t, "unsubscribed", refused["status"])
		assert.Contains(t, refused["message"], "resubscribe")
		refused = toolError(t, full.call("set_contact_list_status", args(fmt.Sprintf(`"email":%q,"list_id":%q,"status":"active"`, email, list.ID))))
		assert.Equal(t, "invalid_state", refused["code"])
		assert.Equal(t, "unsubscribed", membership(t, email, list.ID))

		out = structured(t, subscribe(true))
		assert.Equal(t, "active", out["status"])
		assert.Equal(t, "active", membership(t, email, list.ID))

		out = structured(t, full.call("remove_contact_from_list", args(fmt.Sprintf(`"email":%q,"list_id":%q`, email, list.ID))))
		assert.Equal(t, true, out["removed"])
		assert.Equal(t, "removed active", membership(t, email, list.ID))
		assert.Equal(t, "not_found", toolError(t, full.call("remove_contact_from_list", args(fmt.Sprintf(`"email":%q,"list_id":%q`, email, list.ID))))["code"])
		assert.Equal(t, "not_found", toolError(t, full.call("set_contact_list_status", args(fmt.Sprintf(`"email":%q,"list_id":%q,"status":"active"`, email, list.ID))))["code"])

		rows := auditRows(t, "lists.subscribe", email)
		assert.Equal(t, []int{200, 200, 409, 200}, statuses(rows))
		assert.Equal(t, "unsubscribed", rows[0].Metadata["previous_status"])
		assert.Equal(t, "active", rows[0].Metadata["new_status"])
		assert.Equal(t, []any{list.ID}, rows[0].Metadata[domain.AuditResubscribedListIDs], "the service marks the re-subscription")
		assert.Equal(t, []int{200, 409, 404}, statuses(auditRows(t, "contactLists.updateStatus", email)))
		assert.Equal(t, []int{200, 404}, statuses(auditRows(t, "contactLists.removeContact", email)))
	})

	t.Run("a key lacking a pair neither sees nor runs the tool", func(t *testing.T) {
		list, err := factory.CreateList(workspace.ID)
		require.NoError(t, err)
		email := unique("mcp-refused")
		for _, refusal := range []struct {
			client     *mcpClient
			tool       string
			args       string
			resource   string
			permission string
			action     string
		}{
			{listsOnly, "upsert_contact", args(fmt.Sprintf(`"email":%q`, email)), "contacts", "write", "contacts.upsert"},
			{listsOnly, "delete_contact", args(fmt.Sprintf(`"email":%q`, email)), "contacts", "write", "contacts.delete"},
			{listsOnly, "track_custom_event", args(fmt.Sprintf(`"email":%q,"event_name":"signup","external_id":"s1","occurred_at":"2026-11-20T10:00:00Z"`, email)), "contacts", "write", "customEvents.upsert"},
			{listsOnly, "subscribe_contact_to_list", args(fmt.Sprintf(`"email":%q,"list_id":%q`, email, list.ID)), "contacts", "write", "lists.subscribe"},
			{listsOnly, "set_contact_list_status", args(fmt.Sprintf(`"email":%q,"list_id":%q,"status":"active"`, email, list.ID)), "contacts", "write", "contactLists.updateStatus"},
			{listsOnly, "remove_contact_from_list", args(fmt.Sprintf(`"email":%q,"list_id":%q`, email, list.ID)), "contacts", "write", "contactLists.removeContact"},
			{contactsOnly, "create_list", args(`"id":"refusedlist","name":"Refused"`), "lists", "write", "lists.create"},
			{contactsOnly, "delete_list", args(fmt.Sprintf(`"id":%q`, list.ID)), "lists", "write", "lists.delete"},
			{listsWriteOnly, "update_list", args(fmt.Sprintf(`"id":%q,"name":"Refused"`, list.ID)), "lists", "read", "lists.update"},
		} {
			assert.NotContains(t, refusal.client.toolNames(), refusal.tool)
			result := refusal.client.call(refusal.tool, refusal.args)
			assert.JSONEq(t, fmt.Sprintf(`{"error":{"code":"permission_denied","message":"this API key lacks the %s %s permission","resource":%q,"permission":%q}}`,
				refusal.resource, refusal.permission, refusal.resource, refusal.permission), string(result.StructuredContent), refusal.tool)

			// The refusal comes before the arguments are read, so the row names
			// no target.
			page := listAuditLogs(t, ownerClient, map[string]string{"workspace_id": workspace.ID, "actions": refusal.action, "outcome": domain.AuditOutcomeDenied, "limit": "100"})
			found := false
			for _, row := range page.Logs {
				if row.Metadata["mcp_tool"] == refusal.tool && row.StatusCode == http.StatusForbidden {
					found = true
					assert.Equal(t, refusal.resource, row.Metadata["resource"], refusal.tool)
				}
			}
			assert.True(t, found, "%s: no audit row with status 403", refusal.tool)
		}
		assert.Equal(t, "none", membership(t, email, list.ID))
	})
}
