package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	shortuuid "github.com/lithammer/shortuuid/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// The write tools of automations and the blog through /mcp: each one changes
// real state, a replay answers as spec §5.8 says, a patch keeps what it was
// not given, a key lacking a pair neither sees nor runs the tool, and every
// call leaves an audit row with channel mcp (the harness licence carries
// audit_logs).
func TestMCPWriteToolsAutomationsAndBlog(t *testing.T) {
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
	automationsRead := newKey(domain.UserPermissions{domain.PermissionResourceAutomations: {Read: true}})
	blogOnly := newKey(domain.UserPermissions{domain.PermissionResourceBlog: {Read: true, Write: true}})

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
	assertRefusalAudited := func(t *testing.T, action, tool, resource string) {
		t.Helper()
		page := listAuditLogs(t, ownerClient, map[string]string{"workspace_id": workspace.ID, "actions": action, "outcome": domain.AuditOutcomeDenied, "limit": "100"})
		found := false
		for _, row := range page.Logs {
			if row.Metadata["mcp_tool"] == tool && row.StatusCode == http.StatusForbidden {
				found = true
				assert.Equal(t, resource, row.Metadata["resource"], tool)
			}
		}
		assert.True(t, found, "%s: no audit row with status 403", tool)
	}

	t.Run("activate_automation, pause_automation and delete_automation, with their replays", func(t *testing.T) {
		id, rootID := shortuuid.New(), shortuuid.New()
		_, err := factory.CreateAutomation(workspace.ID, testutil.WithAutomationID(id), testutil.WithAutomationName("Onboarding"),
			testutil.WithAutomationRootNodeID(rootID),
			testutil.WithAutomationNodes([]*domain.AutomationNode{{ID: rootID, AutomationID: id, Type: domain.NodeTypeTrigger, Config: map[string]interface{}{}}}))
		require.NoError(t, err)
		target := args(fmt.Sprintf(`"automation_id":%q`, id))
		status := func() string {
			var status string
			require.NoError(t, db.QueryRow(`SELECT status FROM automations WHERE id = $1`, id).Scan(&status))
			return status
		}

		out := structured(t, full.call("activate_automation", target))
		assert.Equal(t, "live", out["status"])
		assert.Equal(t, "live", status())
		replay := toolError(t, full.call("activate_automation", target))
		assert.Equal(t, "invalid_state", replay["code"])
		assert.Equal(t, "live", replay["status"])

		out = structured(t, full.call("pause_automation", target))
		assert.Equal(t, false, out["already"])
		assert.Equal(t, "paused", status())
		out = structured(t, full.call("pause_automation", target))
		assert.Equal(t, true, out["already"], "a replay on a paused automation succeeds")
		assert.Equal(t, "paused", status())

		out = structured(t, full.call("delete_automation", target))
		assert.Equal(t, true, out["deleted"])
		var deleted bool
		require.NoError(t, db.QueryRow(`SELECT deleted_at IS NOT NULL FROM automations WHERE id = $1`, id).Scan(&deleted))
		assert.True(t, deleted)
		assert.Equal(t, "not_found", toolError(t, full.call("delete_automation", target))["code"])
		assert.Equal(t, "not_found", toolError(t, full.call("pause_automation", target))["code"])

		assert.Equal(t, []int{200, 409}, auditStatuses(t, "automations.activate", id))
		assert.Equal(t, []int{200, 200, 404}, auditStatuses(t, "automations.pause", id))
		assert.Equal(t, []int{200, 404}, auditStatuses(t, "automations.delete", id))
	})

	t.Run("a key that may only read automations neither sees nor runs the write tools", func(t *testing.T) {
		id := shortuuid.New()
		_, err := factory.CreateAutomation(workspace.ID, testutil.WithAutomationID(id))
		require.NoError(t, err)
		names := automationsRead.toolNames()
		for _, tool := range []string{"activate_automation", "pause_automation", "delete_automation"} {
			assert.NotContains(t, names, tool)
			result := automationsRead.call(tool, args(fmt.Sprintf(`"automation_id":%q`, id)))
			assert.JSONEq(t, `{"error":{"code":"permission_denied","message":"this API key lacks the automations write permission","resource":"automations","permission":"write"}}`,
				string(result.StructuredContent), tool)
		}
		assertRefusalAudited(t, "automations.activate", "activate_automation", "automations")
		assertRefusalAudited(t, "automations.delete", "delete_automation", "automations")
		var status string
		require.NoError(t, db.QueryRow(`SELECT status FROM automations WHERE id = $1 AND deleted_at IS NULL`, id).Scan(&status))
		assert.Equal(t, "draft", status)
	})

	category, err := factory.CreateBlogCategory(workspace.ID)
	require.NoError(t, err)
	const body = `{"type":"doc","content":[{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"Spring <release>"}]},` +
		`{"type":"paragraph","content":[{"type":"text","text":"Read the "},{"type":"text","text":"docs","marks":[{"type":"link","attrs":{"href":"https://docs.example.com"}}]}]}]}`

	t.Run("create_blog_post, update_blog_post, publish, unpublish and delete, with their replays", func(t *testing.T) {
		created := args(fmt.Sprintf(`"category_id":%q,"slug":"spring-release","title":"Spring release","excerpt":"Three changes","body":%s`, category.ID, body))
		out := structured(t, full.call("create_blog_post", created))
		post := out["post"].(map[string]any)
		postID := post["id"].(string)
		assert.Equal(t, "draft", post["status"])
		assert.Equal(t, float64(1), post["body_version"])

		var templateID string
		var templateVersion int
		require.NoError(t, db.QueryRow(`SELECT settings->'template'->>'template_id', (settings->'template'->>'template_version')::int FROM blog_posts WHERE id = $1`, postID).
			Scan(&templateID, &templateVersion))
		assert.Equal(t, 0, templateVersion, "the post follows the latest body")
		var channel, html, plainText, content string
		require.NoError(t, db.QueryRow(`SELECT channel, web->>'html', web->>'plain_text', web->'content'::text FROM templates WHERE id = $1 AND version = 1`, templateID).
			Scan(&channel, &html, &plainText, &content))
		assert.Equal(t, "web", channel)
		assert.Equal(t, `<h2>Spring &lt;release&gt;</h2><p>Read the <a href="https://docs.example.com">docs</a></p>`, html)
		assert.Equal(t, "Spring <release> Read the  docs", plainText)
		assert.JSONEq(t, body, content)

		// The replay meets its own slug, and leaves no second body behind.
		assert.Equal(t, "conflict", toolError(t, full.call("create_blog_post", created))["code"])
		var liveBodies int
		require.NoError(t, db.QueryRow(`SELECT count(DISTINCT id) FROM templates WHERE name = 'Blog: Spring release' AND deleted_at IS NULL`).Scan(&liveBodies))
		assert.Equal(t, 1, liveBodies)
		unsafe := toolError(t, full.call("create_blog_post", args(fmt.Sprintf(`"category_id":%q,"slug":"unsafe","title":"Unsafe","body":%s`, category.ID,
			`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"x","marks":[{"type":"link","attrs":{"href":"javascript:alert(1)"}}]}]}]}`))))
		assert.Equal(t, "validation", unsafe["code"])

		// A new title keeps the excerpt and the body.
		out = structured(t, full.call("update_blog_post", args(fmt.Sprintf(`"id":%q,"title":"Spring release notes"`, postID))))
		assert.Equal(t, "Spring release notes", out["post"].(map[string]any)["title"])
		var title, excerpt string
		require.NoError(t, db.QueryRow(`SELECT settings->>'title', settings->>'excerpt' FROM blog_posts WHERE id = $1`, postID).Scan(&title, &excerpt))
		assert.Equal(t, "Spring release notes", title)
		assert.Equal(t, "Three changes", excerpt, "a field left out keeps its value")

		// A new body saves version 2, which the post follows.
		newBody := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Updated","marks":[{"type":"bold"}]}]}]}`
		out = structured(t, full.call("update_blog_post", args(fmt.Sprintf(`"id":%q,"body":%s,"base_version":1`, postID, newBody))))
		assert.Equal(t, float64(2), out["post"].(map[string]any)["body_version"])
		require.NoError(t, db.QueryRow(`SELECT web->>'html' FROM templates WHERE id = $1 AND version = 2`, templateID).Scan(&html))
		assert.Equal(t, `<p><strong>Updated</strong></p>`, html)
		require.NoError(t, db.QueryRow(`SELECT (settings->'template'->>'template_version')::int, settings->>'excerpt' FROM blog_posts WHERE id = $1`, postID).
			Scan(&templateVersion, &excerpt))
		assert.Equal(t, 0, templateVersion)
		assert.Equal(t, "Three changes", excerpt)
		stale := toolError(t, full.call("update_blog_post", args(fmt.Sprintf(`"id":%q,"body":%s,"base_version":1`, postID, newBody))))
		assert.Equal(t, "conflict", stale["code"])
		assert.Equal(t, float64(2), stale["latest_version"])

		target := args(fmt.Sprintf(`"id":%q`, postID))
		out = structured(t, full.call("publish_blog_post", target))
		assert.Equal(t, "published", out["status"])
		assert.Equal(t, false, out["already"])
		var published bool
		require.NoError(t, db.QueryRow(`SELECT published_at IS NOT NULL FROM blog_posts WHERE id = $1`, postID).Scan(&published))
		assert.True(t, published)
		out = structured(t, full.call("publish_blog_post", target))
		assert.Equal(t, true, out["already"])
		assert.Equal(t, "published", out["status"])

		out = structured(t, full.call("unpublish_blog_post", target))
		assert.Equal(t, false, out["already"])
		require.NoError(t, db.QueryRow(`SELECT published_at IS NOT NULL FROM blog_posts WHERE id = $1`, postID).Scan(&published))
		assert.False(t, published)
		out = structured(t, full.call("unpublish_blog_post", target))
		assert.Equal(t, true, out["already"])

		out = structured(t, full.call("delete_blog_post", target))
		assert.Equal(t, true, out["deleted"])
		var deleted bool
		require.NoError(t, db.QueryRow(`SELECT deleted_at IS NOT NULL FROM blog_posts WHERE id = $1`, postID).Scan(&deleted))
		assert.True(t, deleted)
		assert.Equal(t, "not_found", toolError(t, full.call("delete_blog_post", target))["code"])
		assert.Equal(t, "not_found", toolError(t, full.call("publish_blog_post", target))["code"])

		assert.Equal(t, []int{200, 200, 409}, auditStatuses(t, "blogPosts.update", postID))
		assert.Equal(t, []int{200, 200, 404}, auditStatuses(t, "blogPosts.publish", postID))
		assert.Equal(t, []int{200, 200}, auditStatuses(t, "blogPosts.unpublish", postID))
		assert.Equal(t, []int{200, 404}, auditStatuses(t, "blogPosts.delete", postID))
		assert.Equal(t, []int{200}, auditStatuses(t, "blogPosts.create", postID))
	})

	t.Run("a key without the templates pairs edits metadata but no body", func(t *testing.T) {
		post, err := factory.CreateBlogPost(workspace.ID, category.ID)
		require.NoError(t, err)
		names := blogOnly.toolNames()
		assert.NotContains(t, names, "create_blog_post")
		for _, tool := range []string{"update_blog_post", "publish_blog_post", "unpublish_blog_post", "delete_blog_post"} {
			assert.Contains(t, names, tool)
		}

		result := blogOnly.call("create_blog_post", args(fmt.Sprintf(`"category_id":%q,"slug":"refused","title":"Refused","body":%s`, category.ID, body)))
		assert.JSONEq(t, `{"error":{"code":"permission_denied","message":"this API key lacks the templates write permission","resource":"templates","permission":"write"}}`,
			string(result.StructuredContent))
		assertRefusalAudited(t, "blogPosts.create", "create_blog_post", "templates")

		result = blogOnly.call("update_blog_post", args(fmt.Sprintf(`"id":%q,"body":%s`, post.ID, body)))
		assert.Equal(t, "permission_denied", toolError(t, result)["code"])
		assertRefusalAudited(t, "blogPosts.update", "update_blog_post", "templates")

		structured(t, blogOnly.call("update_blog_post", args(fmt.Sprintf(`"id":%q,"title":"Renamed"`, post.ID))))
		var title string
		var version int
		require.NoError(t, db.QueryRow(`SELECT settings->>'title', (settings->'template'->>'template_version')::int FROM blog_posts WHERE id = $1`, post.ID).Scan(&title, &version))
		assert.Equal(t, "Renamed", title)
		assert.Equal(t, 1, version, "the body is untouched")
		var count int
		require.NoError(t, db.QueryRow(`SELECT count(*) FROM blog_posts WHERE slug = 'refused'`).Scan(&count))
		assert.Equal(t, 0, count)
	})
}
