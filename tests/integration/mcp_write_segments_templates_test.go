package integration

import (
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

// The write tools of segments and templates through /mcp: each one changes
// real state, a replay answers as spec §5.8 says, a patch keeps what it was
// not given, a key lacking a pair neither sees nor runs the tool, and every
// call leaves an audit row with channel mcp (the harness licence carries
// audit_logs).
func TestMCPWriteToolsSegmentsAndTemplates(t *testing.T) {
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
	segmentsOnly := newKey(domain.UserPermissions{domain.PermissionResourceSegments: {Read: true, Write: true}})
	templatesOnly := newKey(domain.UserPermissions{domain.PermissionResourceTemplates: {Read: true, Write: true}})
	writeOnly := newKey(domain.UserPermissions{
		domain.PermissionResourceSegments: {Write: true}, domain.PermissionResourceTemplates: {Write: true},
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
	// auditStatuses reads the status of the MCP rows of an action on a
	// target, oldest first.
	auditStatuses := func(t *testing.T, action, targetID string) []int {
		t.Helper()
		page := listAuditLogs(t, ownerClient, map[string]string{
			"workspace_id": workspace.ID, "actions": action, "target_id": targetID, "limit": "100",
		})
		out := []int{}
		for i := len(page.Logs) - 1; i >= 0; i-- {
			if page.Logs[i].Metadata["channel"] == "mcp" {
				out = append(out, page.Logs[i].StatusCode)
			}
		}
		return out
	}
	newID := func(prefix string) string {
		return prefix + strings.ReplaceAll(uuid.New().String()[:8], "-", "")
	}
	const emailIsSet = `{"kind":"leaf","leaf":{"source":"contacts","contact":{"filters":[{"field_name":"email","field_type":"string","operator":"is_set"}]}}}`
	const countryIsFR = `{"kind":"leaf","leaf":{"source":"contacts","contact":{"filters":[{"field_name":"country","field_type":"string","operator":"equals","string_values":["FR"]}]}}}`

	t.Run("create_segment, update_segment and delete_segment, with their replays", func(t *testing.T) {
		id := newID("mcp_seg_")
		created := args(fmt.Sprintf(`"id":%q,"name":"Everyone","tree":%s`, id, emailIsSet))
		out := structured(t, full.call("create_segment", created))
		segment := out["segment"].(map[string]any)
		assert.Equal(t, "building", segment["status"])
		assert.Equal(t, "blue", segment["color"], "the console's default color")
		var name, color, timezone string
		var version int
		require.NoError(t, db.QueryRow(`SELECT name, color, timezone, version FROM segments WHERE id = $1`, id).Scan(&name, &color, &timezone, &version))
		assert.Equal(t, "Everyone", name)
		assert.Equal(t, "blue", color)
		assert.Equal(t, workspace.Settings.Timezone, timezone, "the workspace timezone when none is given")
		assert.Equal(t, 1, version)

		assert.Equal(t, "conflict", toolError(t, full.call("create_segment", created))["code"])
		misspelt := toolError(t, full.call("create_segment", args(fmt.Sprintf(`"id":%q,"name":"X","tree":%s`, newID("mcp_seg_"),
			`{"kind":"leaf","leaf":{"source":"contacts","contact":{"filters":[{"field_name":"email","field_type":"string","operater":"is_set"}]}}}`))))
		assert.Equal(t, "validation", misspelt["code"], "an unknown tree key is refused")

		// A rename keeps the tree, and still rebuilds.
		out = structured(t, full.call("update_segment", args(fmt.Sprintf(`"id":%q,"name":"All contacts"`, id))))
		assert.Equal(t, "All contacts", out["segment"].(map[string]any)["name"])
		var tree string
		require.NoError(t, db.QueryRow(`SELECT name, color, tree::text, version FROM segments WHERE id = $1`, id).Scan(&name, &color, &tree, &version))
		assert.Equal(t, "All contacts", name)
		assert.Equal(t, "blue", color, "a field left out keeps its value")
		assert.JSONEq(t, emailIsSet, tree, "the tree left out is kept")
		assert.Equal(t, 2, version, "every update rebuilds")

		out = structured(t, full.call("update_segment", args(fmt.Sprintf(`"id":%q,"tree":%s`, id, countryIsFR))))
		require.NoError(t, db.QueryRow(`SELECT name, tree::text FROM segments WHERE id = $1`, id).Scan(&name, &tree))
		assert.Equal(t, "All contacts", name)
		assert.JSONEq(t, countryIsFR, tree)

		out = structured(t, full.call("delete_segment", args(fmt.Sprintf(`"id":%q`, id))))
		assert.Equal(t, true, out["deleted"])
		var status string
		require.NoError(t, db.QueryRow(`SELECT status FROM segments WHERE id = $1`, id).Scan(&status))
		assert.Equal(t, "deleted", status)
		assert.Equal(t, "not_found", toolError(t, full.call("delete_segment", args(fmt.Sprintf(`"id":%q`, id))))["code"])
		assert.Equal(t, "not_found", toolError(t, full.call("update_segment", args(fmt.Sprintf(`"id":%q,"name":"Back"`, id))))["code"])
		require.NoError(t, db.QueryRow(`SELECT name, status FROM segments WHERE id = $1`, id).Scan(&name, &status))
		assert.Equal(t, "All contacts", name, "a deleted segment is not written")
		assert.Equal(t, "deleted", status)

		assert.Equal(t, []int{200, 409}, auditStatuses(t, "segments.create", id))
		assert.Equal(t, []int{200, 200, 404}, auditStatuses(t, "segments.update", id))
		assert.Equal(t, []int{200, 404}, auditStatuses(t, "segments.delete", id))
	})

	const welcomeMJML = `<mjml><mj-body><mj-section><mj-column><mj-text>Hi {{ contact.first_name }}</mj-text></mj-column></mj-section></mj-body></mjml>`

	t.Run("create_template, update_template and delete_template, with their replays", func(t *testing.T) {
		id := newID("mcp-tpl-")
		created := args(fmt.Sprintf(`"id":%q,"name":"Welcome","category":"welcome","subject":"Hi {{ contact.first_name }}","subject_preview":"Three things",`+
			`"mjml_source":%q,"reply_to":"help@example.com","test_data":{"contact":{"first_name":"Ada"}}`, id, welcomeMJML))
		out := structured(t, full.call("create_template", created))
		template := out["template"].(map[string]any)
		assert.Equal(t, "code", template["editor_mode"])
		assert.Equal(t, float64(1), template["version"])
		assert.Contains(t, template["mjml"], "<mj-title>Welcome</mj-title>", "the MJML as the save rewrote it")
		assert.Contains(t, template["mjml"], "<mj-preview>Three things</mj-preview>")
		var mode, stored string
		require.NoError(t, db.QueryRow(`SELECT email->>'editor_mode', email->>'mjml_source' FROM templates WHERE id = $1`, id).Scan(&mode, &stored))
		assert.Equal(t, "code", mode)
		assert.Equal(t, template["mjml"], stored)

		assert.Equal(t, "conflict", toolError(t, full.call("create_template", created))["code"])
		broken := newID("mcp-tpl-")
		refused := toolError(t, full.call("create_template", args(fmt.Sprintf(`"id":%q,"name":"Broken","category":"other","subject":"Hi {%% if contact %%}","mjml_source":%q`, broken, welcomeMJML))))
		assert.Equal(t, "validation", refused["code"], "a template that does not compile is refused")
		var rows int
		require.NoError(t, db.QueryRow(`SELECT count(*) FROM templates WHERE id = $1`, broken).Scan(&rows))
		assert.Equal(t, 0, rows)

		// A new subject keeps every other field, translations included.
		out = structured(t, full.call("update_template", args(fmt.Sprintf(`"id":%q,"subject":"Welcome aboard"`, id))))
		assert.Equal(t, float64(2), out["template"].(map[string]any)["version"])
		var subject, replyTo, preview, testData string
		require.NoError(t, db.QueryRow(`SELECT email->>'subject', email->>'reply_to', email->>'subject_preview', email->>'mjml_source', test_data::text
			FROM templates WHERE id = $1 ORDER BY version DESC LIMIT 1`, id).Scan(&subject, &replyTo, &preview, &stored, &testData))
		assert.Equal(t, "Welcome aboard", subject)
		assert.Equal(t, "help@example.com", replyTo, "a field left out keeps its value")
		assert.Equal(t, "Three things", preview)
		assert.Contains(t, stored, "Hi {{ contact.first_name }}")
		assert.JSONEq(t, `{"contact":{"first_name":"Ada"}}`, testData)

		stale := toolError(t, full.call("update_template", args(fmt.Sprintf(`"id":%q,"subject":"Stale","base_version":1`, id))))
		assert.Equal(t, "conflict", stale["code"])
		assert.Equal(t, float64(2), stale["latest_version"])

		structured(t, full.call("delete_template", args(fmt.Sprintf(`"id":%q`, id))))
		var deleted bool
		require.NoError(t, db.QueryRow(`SELECT bool_and(deleted_at IS NOT NULL) FROM templates WHERE id = $1`, id).Scan(&deleted))
		assert.True(t, deleted)
		assert.Equal(t, "not_found", toolError(t, full.call("delete_template", args(fmt.Sprintf(`"id":%q`, id))))["code"])
		assert.Equal(t, "not_found", toolError(t, full.call("update_template", args(fmt.Sprintf(`"id":%q,"subject":"Back"`, id))))["code"])

		assert.Equal(t, []int{200, 409}, auditStatuses(t, "templates.create", id))
		assert.Equal(t, []int{400}, auditStatuses(t, "templates.create", broken))
		assert.Equal(t, []int{200, 409, 404}, auditStatuses(t, "templates.update", id))
		assert.Equal(t, []int{200, 404}, auditStatuses(t, "templates.delete", id))
	})

	t.Run("update_template refuses a visual template", func(t *testing.T) {
		visual, err := factory.CreateTemplate(workspace.ID)
		require.NoError(t, err)
		refused := toolError(t, full.call("update_template", args(fmt.Sprintf(`"id":%q,"subject":"Changed"`, visual.ID))))
		assert.Equal(t, "validation", refused["code"])
		assert.Contains(t, refused["message"], "visual mode")
		var versions int
		require.NoError(t, db.QueryRow(`SELECT count(*) FROM templates WHERE id = $1`, visual.ID).Scan(&versions))
		assert.Equal(t, 1, versions, "nothing is written")
	})

	t.Run("a key lacking a pair neither sees nor runs the tool", func(t *testing.T) {
		segment, err := factory.CreateSegment(workspace.ID)
		require.NoError(t, err)
		template, err := factory.CreateTemplate(workspace.ID)
		require.NoError(t, err)
		for _, refusal := range []struct {
			client     *mcpClient
			tool       string
			args       string
			resource   string
			permission string
			action     string
		}{
			{templatesOnly, "create_segment", args(fmt.Sprintf(`"id":"refused_seg","name":"Refused","tree":%s`, emailIsSet)), "segments", "write", "segments.create"},
			{templatesOnly, "delete_segment", args(fmt.Sprintf(`"id":%q`, segment.ID)), "segments", "write", "segments.delete"},
			{writeOnly, "update_segment", args(fmt.Sprintf(`"id":%q,"name":"Refused"`, segment.ID)), "segments", "read", "segments.update"},
			{segmentsOnly, "create_template", args(fmt.Sprintf(`"id":"refused-tpl","name":"Refused","category":"other","subject":"S","mjml_source":%q`, welcomeMJML)), "templates", "write", "templates.create"},
			{segmentsOnly, "delete_template", args(fmt.Sprintf(`"id":%q`, template.ID)), "templates", "write", "templates.delete"},
			{writeOnly, "update_template", args(fmt.Sprintf(`"id":%q,"subject":"Refused"`, template.ID)), "templates", "read", "templates.update"},
		} {
			assert.NotContains(t, refusal.client.toolNames(), refusal.tool)
			result := refusal.client.call(refusal.tool, refusal.args)
			assert.JSONEq(t, fmt.Sprintf(`{"error":{"code":"permission_denied","message":"this API key lacks the %s %s permission","resource":%q,"permission":%q}}`,
				refusal.resource, refusal.permission, refusal.resource, refusal.permission), string(result.StructuredContent), refusal.tool)

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
		// A write-only key still sees the tools that need no read.
		names := writeOnly.toolNames()
		for _, tool := range []string{"create_segment", "delete_segment", "create_template", "delete_template"} {
			assert.Contains(t, names, tool)
		}

		var name, status string
		require.NoError(t, db.QueryRow(`SELECT name, status FROM segments WHERE id = $1`, segment.ID).Scan(&name, &status))
		assert.Equal(t, segment.Name, name)
		assert.Equal(t, "active", status)
		var live int
		require.NoError(t, db.QueryRow(`SELECT count(*) FROM templates WHERE id = $1 AND deleted_at IS NULL`, template.ID).Scan(&live))
		assert.Equal(t, 1, live)
	})
}
