package integration

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	shortuuid "github.com/lithammer/shortuuid/v4"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// mcpAppFactory turns MCP on, which MCP_ENABLED leaves off by default.
func mcpAppFactory(cfg *config.Config) testutil.AppInterface {
	cfg.MCP.Enabled = true
	return appFactory(cfg)
}

// mcpClient speaks raw JSON-RPC to /mcp the way a 2026-07-28 client does:
// the Mcp-* headers plus the protocol fields in _meta. Bodies are written
// by hand, as a client sends them.
type mcpClient struct {
	t     *testing.T
	url   string
	token string
}

type mcpRPCResponse struct {
	status int
	body   string
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
	} `json:"error"`
}

const mcpMeta = `"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{},"io.modelcontextprotocol/clientInfo":{"name":"integration","version":"1"}}`

func (c *mcpClient) post(method, name, params string) mcpRPCResponse {
	c.t.Helper()
	if params != "" {
		params += ","
	}
	body := fmt.Sprintf(`{"jsonrpc":"2.0","id":7,"method":%q,"params":{%s%s}}`, method, params, mcpMeta)
	req, err := http.NewRequest(http.MethodPost, c.url+"/mcp", strings.NewReader(body))
	require.NoError(c.t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Mcp-Protocol-Version", "2026-07-28")
	req.Header.Set("Mcp-Method", method)
	if name != "" {
		req.Header.Set("Mcp-Name", name)
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	require.NoError(c.t, err)
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	require.NoError(c.t, err)

	out := mcpRPCResponse{status: resp.StatusCode, body: string(data)}
	if resp.StatusCode != http.StatusOK {
		return out
	}
	payload := data
	if strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		scanner := bufio.NewScanner(bytes.NewReader(data))
		scanner.Buffer(make([]byte, 1<<20), 1<<22)
		for scanner.Scan() {
			if line, ok := strings.CutPrefix(scanner.Text(), "data: "); ok {
				payload = []byte(line)
			}
		}
	}
	require.NoError(c.t, json.Unmarshal(payload, &out), string(data))
	return out
}

func (c *mcpClient) toolNames() []string {
	c.t.Helper()
	resp := c.post("tools/list", "", "")
	require.Equal(c.t, http.StatusOK, resp.status, resp.body)
	require.Nil(c.t, resp.Error, resp.body)
	var list struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	require.NoError(c.t, json.Unmarshal(resp.Result, &list))
	names := []string{}
	for _, tool := range list.Tools {
		names = append(names, tool.Name)
	}
	return names
}

type mcpToolResult struct {
	IsError           bool            `json:"isError"`
	StructuredContent json.RawMessage `json:"structuredContent"`
	Content           []struct {
		Text string `json:"text"`
	} `json:"content"`
}

func (c *mcpClient) call(name, arguments string) mcpToolResult {
	c.t.Helper()
	resp := c.post("tools/call", name, fmt.Sprintf(`"name":%q,"arguments":%s`, name, arguments))
	require.Equal(c.t, http.StatusOK, resp.status, resp.body)
	require.Nil(c.t, resp.Error, resp.body)
	var result mcpToolResult
	require.NoError(c.t, json.Unmarshal(resp.Result, &result))
	return result
}

func (r mcpToolResult) errorCode(t *testing.T) string {
	t.Helper()
	require.True(t, r.IsError, string(r.StructuredContent))
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(r.StructuredContent, &body))
	return body.Error.Code
}

// An API key works through /mcp: discovery, a tools/list filtered by its
// permissions and its workspace's switch, and each read tool of workspaces,
// contacts, lists, segments, templates, broadcasts, transactional
// notifications, messages, analytics, automations and the blog.
func TestMCPAPIKey(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, mcpAppFactory)
	defer func() { suite.Cleanup() }()

	// The harness leaves API_ENDPOINT empty, which MCP refuses: it is the host
	// /mcp answers on. The task service read it at start, so setting it now
	// changes nothing else.
	suite.Config.APIEndpoint = suite.ServerManager.GetURL()

	factory := suite.DataFactory
	authService := suite.ServerManager.GetApp().GetAuthService().(*service.AuthService)

	owner, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(owner.ID, workspace.ID, "owner"))

	fullKey, err := factory.CreateAPIKey(workspace.ID)
	require.NoError(t, err)
	listsOnlyKey, err := factory.CreateAPIKey(workspace.ID, testutil.WithAPIKeyPermissions(domain.UserPermissions{
		domain.PermissionResourceLists: {Read: true, Write: true},
	}))
	require.NoError(t, err)
	contactsOnlyKey, err := factory.CreateAPIKey(workspace.ID, testutil.WithAPIKeyPermissions(domain.UserPermissions{
		domain.PermissionResourceContacts: {Read: true},
	}))
	require.NoError(t, err)
	segmentsOnlyKey, err := factory.CreateAPIKey(workspace.ID, testutil.WithAPIKeyPermissions(domain.UserPermissions{
		domain.PermissionResourceSegments: {Read: true},
	}))
	require.NoError(t, err)
	broadcastsOnlyKey, err := factory.CreateAPIKey(workspace.ID, testutil.WithAPIKeyPermissions(domain.UserPermissions{
		domain.PermissionResourceBroadcasts: {Read: true},
	}))
	require.NoError(t, err)
	automationsOnlyKey, err := factory.CreateAPIKey(workspace.ID, testutil.WithAPIKeyPermissions(domain.UserPermissions{
		domain.PermissionResourceAutomations: {Read: true},
	}))
	require.NoError(t, err)
	blogOnlyKey, err := factory.CreateAPIKey(workspace.ID, testutil.WithAPIKeyPermissions(domain.UserPermissions{
		domain.PermissionResourceBlog: {Read: true},
	}))
	require.NoError(t, err)

	// One of the three contacts has an external ID and is on a list.
	jane, err := factory.CreateContact(workspace.ID, testutil.WithContactExternalID("crm-jane"), testutil.WithContactName("Jane", "Doe"))
	require.NoError(t, err)
	for i := 0; i < 2; i++ {
		_, err := factory.CreateContact(workspace.ID)
		require.NoError(t, err)
	}
	newsletter, err := factory.CreateList(workspace.ID, testutil.WithListName("Newsletter"), testutil.WithListPublic(true))
	require.NoError(t, err)
	_, err = factory.CreateContactList(workspace.ID, testutil.WithContactListEmail(jane.Email), testutil.WithContactListListID(newsletter.ID))
	require.NoError(t, err)
	segment, err := factory.CreateSegment(workspace.ID)
	require.NoError(t, err)
	codeTemplate, err := factory.CreateTemplate(workspace.ID, testutil.WithTemplateName("Welcome"),
		testutil.WithTemplateSubject("Hello {{ contact.first_name }}"),
		testutil.WithCodeModeTemplate(`<mjml><mj-body><mj-section><mj-column><mj-text>Hi {{ contact.first_name }}</mj-text></mj-column></mj-section></mj-body></mjml>`))
	require.NoError(t, err)
	visualTemplate, err := factory.CreateTemplate(workspace.ID, testutil.WithTemplateName("Promo"))
	require.NoError(t, err)

	// A broadcast whose global feed carries a secret header and fetched data,
	// with two messages: one opened, one only sent.
	feedFetchedAt := time.Now().UTC()
	broadcast, err := factory.CreateBroadcast(workspace.ID, testutil.WithBroadcastName("November newsletter"),
		testutil.WithBroadcastStatus(domain.BroadcastStatusProcessed), testutil.WithBroadcastTemplateID(codeTemplate.ID),
		testutil.WithBroadcastGlobalFeed(&domain.GlobalFeedSettings{Enabled: true, URL: "https://feed.example.com/products",
			Headers: []domain.DataFeedHeader{{Name: "Authorization", Value: "Bearer feed-secret"}}}),
		testutil.WithBroadcastGlobalFeedData(map[string]interface{}{"product": "fetched-payload"}, &feedFetchedAt))
	require.NoError(t, err)
	_, err = factory.CreateMessageHistory(workspace.ID, testutil.WithMessageBroadcast(broadcast.ID), testutil.WithMessageTemplate(codeTemplate.ID),
		testutil.WithMessageContact(jane.Email), testutil.WithMessageOpened(true))
	require.NoError(t, err)
	_, err = factory.CreateMessageHistory(workspace.ID, testutil.WithMessageBroadcast(broadcast.ID), testutil.WithMessageTemplate(codeTemplate.ID))
	require.NoError(t, err)
	notification, err := factory.CreateTransactionalNotification(workspace.ID, testutil.WithTransactionalNotificationID("password_reset"),
		testutil.WithTransactionalNotificationName("Password reset"))
	require.NoError(t, err)

	// An automation whose webhook step carries a secret, and Jane's journey
	// through it: the webhook step failed, quoting the endpoint's answer.
	automationID, triggerNodeID, webhookNodeID := shortuuid.New(), shortuuid.New(), shortuuid.New()
	_, err = factory.CreateAutomation(workspace.ID, testutil.WithAutomationID(automationID), testutil.WithAutomationName("Welcome series"),
		testutil.WithAutomationStatus(domain.AutomationStatusLive), testutil.WithAutomationRootNodeID(triggerNodeID),
		testutil.WithAutomationNodes([]*domain.AutomationNode{
			{ID: triggerNodeID, AutomationID: automationID, Type: domain.NodeTypeTrigger, Config: map[string]interface{}{}, NextNodeID: &webhookNodeID},
			{ID: webhookNodeID, AutomationID: automationID, Type: domain.NodeTypeWebhook,
				Config: map[string]interface{}{"url": "https://hooks.example.com/welcome", "secret": "hook-bearer-secret"}},
		}))
	require.NoError(t, err)
	workspaceDB, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)
	journeyID := shortuuid.New()
	_, err = workspaceDB.Exec(`INSERT INTO contact_automations (id, automation_id, contact_email, current_node_id, status, context, last_error)
		VALUES ($1, $2, $3, $4, 'active', '{"trigger":{"kind":"contact.created"}}', 'webhook returned 502')`,
		journeyID, automationID, jane.Email, webhookNodeID)
	require.NoError(t, err)
	_, err = workspaceDB.Exec(`INSERT INTO automation_node_executions (id, contact_automation_id, automation_id, node_id, node_type, action, output, error)
		VALUES ($1, $2, $3, $4, 'webhook', 'failed', '{"raw":"bad gateway"}', 'webhook returned 502')`,
		shortuuid.New(), journeyID, automationID, webhookNodeID)
	require.NoError(t, err)

	// A blog post whose body is a web template, followed at its latest version.
	category, err := factory.CreateBlogCategory(workspace.ID)
	require.NoError(t, err)
	bodyTemplate, err := factory.CreateTemplate(workspace.ID, func(template *domain.Template) {
		template.Channel, template.Category, template.Email = "web", "blog", nil
		template.Web = &domain.WebTemplate{
			Content: domain.MapOfAny{"type": "doc", "content": []interface{}{map[string]interface{}{
				"type": "paragraph", "content": []interface{}{map[string]interface{}{"type": "text", "text": "Spring is here"}},
			}}},
			HTML: "<p>rendered-html</p>", PlainText: "Spring is here",
		}
	})
	require.NoError(t, err)
	// The factory makes an email template for the post, which the template
	// tools would list: it is removed once the post points at the web one.
	var factoryTemplateID string
	post, err := factory.CreateBlogPost(workspace.ID, category.ID, func(post *domain.BlogPost) {
		factoryTemplateID = post.Settings.Template.TemplateID
		post.Slug, post.Settings.Title = "spring-release", "What's new this spring"
		post.Settings.Template = domain.BlogPostTemplateReference{TemplateID: bodyTemplate.ID, TemplateVersion: 0}
	})
	require.NoError(t, err)
	_, err = workspaceDB.Exec(`DELETE FROM templates WHERE id = $1`, factoryTemplateID)
	require.NoError(t, err)

	url := suite.ServerManager.GetURL()
	full := &mcpClient{t: t, url: url, token: authService.GenerateAPIAuthToken(fullKey)}
	listsOnly := &mcpClient{t: t, url: url, token: authService.GenerateAPIAuthToken(listsOnlyKey)}
	contactsOnly := &mcpClient{t: t, url: url, token: authService.GenerateAPIAuthToken(contactsOnlyKey)}
	segmentsOnly := &mcpClient{t: t, url: url, token: authService.GenerateAPIAuthToken(segmentsOnlyKey)}
	broadcastsOnly := &mcpClient{t: t, url: url, token: authService.GenerateAPIAuthToken(broadcastsOnlyKey)}
	automationsOnly := &mcpClient{t: t, url: url, token: authService.GenerateAPIAuthToken(automationsOnlyKey)}
	blogOnly := &mcpClient{t: t, url: url, token: authService.GenerateAPIAuthToken(blogOnlyKey)}

	t.Run("server/discover advertises tools only", func(t *testing.T) {
		resp := full.post("server/discover", "", "")
		require.Equal(t, http.StatusOK, resp.status, resp.body)
		var discover struct {
			Capabilities map[string]json.RawMessage `json:"capabilities"`
		}
		require.NoError(t, json.Unmarshal(resp.Result, &discover))
		assert.Len(t, discover.Capabilities, 1)
		assert.JSONEq(t, `{}`, string(discover.Capabilities["tools"]))
	})

	t.Run("a credential that is not an API key gets 401", func(t *testing.T) {
		anonymous := &mcpClient{t: t, url: url}
		resp := anonymous.post("server/discover", "", "")
		assert.Equal(t, http.StatusUnauthorized, resp.status)

		session := &mcpClient{t: t, url: url, token: authService.GenerateUserAuthToken(owner, "session-id", time.Now().Add(time.Hour))}
		resp = session.post("server/discover", "", "")
		assert.Equal(t, http.StatusUnauthorized, resp.status)
	})

	t.Run("tools/list shows the key exactly what it may call", func(t *testing.T) {
		assert.Equal(t, allTools, full.toolNames())
		// The analytics tools have no static pair: the schema argument decides
		// the permission, at call time.
		// The subscription tools also need contacts:write.
		assert.Equal(t, []string{"create_list", "delete_list", "list_analytics_schemas", "list_lists", "list_workspaces", "query_analytics", "update_list"},
			listsOnly.toolNames())
		assert.Equal(t, []string{
			"count_contacts", "get_contact", "get_contact_timeline", "list_analytics_schemas", "list_workspaces", "query_analytics", "search_contacts",
		}, contactsOnly.toolNames())
		// preview_segment also needs contacts:read.
		assert.Equal(t, []string{"get_segment", "list_analytics_schemas", "list_segments", "list_workspaces", "query_analytics"}, segmentsOnly.toolNames())
		// get_broadcast_report also needs message_history:read.
		assert.Equal(t, []string{"get_broadcast", "list_analytics_schemas", "list_broadcasts", "list_workspaces", "query_analytics"}, broadcastsOnly.toolNames())
		assert.Equal(t, []string{
			"get_automation", "get_automation_journey", "get_automation_stats", "list_analytics_schemas", "list_automations", "list_workspaces", "query_analytics",
		}, automationsOnly.toolNames())
		// get_blog_post also reads the body with templates:read, but is listed
		// without it.
		assert.Equal(t, []string{"get_blog_post", "list_analytics_schemas", "list_blog_categories", "list_blog_posts", "list_workspaces", "query_analytics"}, blogOnly.toolNames())
	})

	t.Run("list_workspaces names the key's workspace", func(t *testing.T) {
		result := full.call("list_workspaces", `{}`)
		require.False(t, result.IsError, string(result.StructuredContent))
		assert.JSONEq(t, fmt.Sprintf(`{"workspaces":[{"id":%q,"name":%q,"timezone":"UTC","permissions":{
			"automations":{"read":true,"write":true},"blog":{"read":true,"write":true},
			"broadcasts":{"read":true,"write":true},"contacts":{"read":true,"write":true},"lists":{"read":true,"write":true},
			"message_history":{"read":true,"write":true},"segments":{"read":true,"write":true},
			"templates":{"read":true,"write":true},"transactional":{"read":true,"write":true},"workspace":{"read":true,"write":true}
		}}]}`,
			workspace.ID, workspace.Name), string(result.StructuredContent))
		assert.JSONEq(t, string(result.StructuredContent), result.Content[0].Text)
	})

	t.Run("search_contacts pages through the contacts", func(t *testing.T) {
		type page struct {
			Workspace struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"workspace"`
			Contacts []map[string]json.RawMessage `json:"contacts"`
			Next     string                       `json:"next_cursor"`
		}
		result := full.call("search_contacts", fmt.Sprintf(`{"workspace_id":%q,"limit":2}`, workspace.ID))
		require.False(t, result.IsError, string(result.StructuredContent))
		var first page
		require.NoError(t, json.Unmarshal(result.StructuredContent, &first))
		assert.Equal(t, workspace.ID, first.Workspace.ID)
		assert.Equal(t, workspace.Name, first.Workspace.Name)
		require.Len(t, first.Contacts, 2)
		require.NotEmpty(t, first.Next)
		for _, contact := range first.Contacts {
			assert.NotContains(t, contact, "email_hmac")
			assert.Contains(t, contact, "email")
		}

		result = full.call("search_contacts", fmt.Sprintf(`{"workspace_id":%q,"limit":2,"cursor":%q}`, workspace.ID, first.Next))
		require.False(t, result.IsError, string(result.StructuredContent))
		var second page
		require.NoError(t, json.Unmarshal(result.StructuredContent, &second))
		require.Len(t, second.Contacts, 1)
		assert.Empty(t, second.Next)
		assert.NotEqual(t, first.Contacts[0]["email"], second.Contacts[0]["email"])
		assert.NotEqual(t, first.Contacts[1]["email"], second.Contacts[0]["email"])

		result = full.call("search_contacts", fmt.Sprintf(`{"workspace_id":%q,"limit":2,"search":"x","cursor":%q}`, workspace.ID, first.Next))
		assert.Equal(t, "validation", result.errorCode(t), "a cursor is bound to its filters")
	})

	t.Run("a key without contacts:read is refused search_contacts", func(t *testing.T) {
		result := listsOnly.call("search_contacts", fmt.Sprintf(`{"workspace_id":%q}`, workspace.ID))
		assert.Equal(t, "permission_denied", result.errorCode(t))
		assert.JSONEq(t, `{"error":{"code":"permission_denied","message":"this API key lacks the contacts read permission","resource":"contacts","permission":"read"}}`,
			string(result.StructuredContent))
	})

	t.Run("get_workspace_overview describes the workspace and its lists", func(t *testing.T) {
		result := full.call("get_workspace_overview", fmt.Sprintf(`{"workspace_id":%q}`, workspace.ID))
		require.False(t, result.IsError, string(result.StructuredContent))
		var out struct {
			Workspace struct {
				ID string `json:"id"`
			} `json:"workspace"`
			Timezone       string                     `json:"timezone"`
			EmailProviders map[string]json.RawMessage `json:"email_providers"`
			Lists          []struct {
				ID          string `json:"id"`
				Name        string `json:"name"`
				IsPublic    bool   `json:"is_public"`
				ActiveCount int    `json:"active_count"`
			} `json:"lists"`
		}
		require.NoError(t, json.Unmarshal(result.StructuredContent, &out))
		assert.Equal(t, workspace.ID, out.Workspace.ID)
		assert.Equal(t, "UTC", out.Timezone)
		assert.Contains(t, out.EmailProviders, "marketing")
		require.Len(t, out.Lists, 1)
		assert.Equal(t, newsletter.ID, out.Lists[0].ID)
		assert.Equal(t, "Newsletter", out.Lists[0].Name)
		assert.True(t, out.Lists[0].IsPublic)
		assert.Equal(t, 1, out.Lists[0].ActiveCount)
		assert.NotContains(t, result.Content[0].Text, "integrations")
		assert.NotContains(t, result.Content[0].Text, "secret")
	})

	t.Run("get_contact reads a contact by email or external ID", func(t *testing.T) {
		type contactResult struct {
			Workspace struct {
				ID string `json:"id"`
			} `json:"workspace"`
			Contact struct {
				Email      string `json:"email"`
				ExternalID string `json:"external_id"`
				FirstName  string `json:"first_name"`
				Lists      []struct {
					ListID   string `json:"list_id"`
					ListName string `json:"list_name"`
					Status   string `json:"status"`
				} `json:"lists"`
			} `json:"contact"`
		}
		result := full.call("get_contact", fmt.Sprintf(`{"workspace_id":%q,"email":%q}`, workspace.ID, strings.ToUpper(jane.Email)))
		require.False(t, result.IsError, string(result.StructuredContent))
		var byEmail contactResult
		require.NoError(t, json.Unmarshal(result.StructuredContent, &byEmail))
		assert.Equal(t, workspace.ID, byEmail.Workspace.ID)
		assert.Equal(t, jane.Email, byEmail.Contact.Email)
		assert.Equal(t, "crm-jane", byEmail.Contact.ExternalID)
		assert.Equal(t, "Jane", byEmail.Contact.FirstName)
		require.Len(t, byEmail.Contact.Lists, 1)
		assert.Equal(t, newsletter.ID, byEmail.Contact.Lists[0].ListID)
		assert.Equal(t, "Newsletter", byEmail.Contact.Lists[0].ListName)
		assert.Equal(t, "active", byEmail.Contact.Lists[0].Status)
		assert.NotContains(t, result.Content[0].Text, "email_hmac")

		result = full.call("get_contact", fmt.Sprintf(`{"workspace_id":%q,"external_id":"crm-jane"}`, workspace.ID))
		require.False(t, result.IsError, string(result.StructuredContent))
		var byExternalID contactResult
		require.NoError(t, json.Unmarshal(result.StructuredContent, &byExternalID))
		assert.Equal(t, jane.Email, byExternalID.Contact.Email)

		result = full.call("get_contact", fmt.Sprintf(`{"workspace_id":%q,"email":"nobody@example.com"}`, workspace.ID))
		assert.Equal(t, "not_found", result.errorCode(t))
		result = full.call("get_contact", fmt.Sprintf(`{"workspace_id":%q,"email":%q,"external_id":"crm-jane"}`, workspace.ID, jane.Email))
		assert.Equal(t, "validation", result.errorCode(t))
	})

	t.Run("count_contacts counts every contact", func(t *testing.T) {
		result := full.call("count_contacts", fmt.Sprintf(`{"workspace_id":%q}`, workspace.ID))
		require.False(t, result.IsError, string(result.StructuredContent))
		assert.JSONEq(t, fmt.Sprintf(`{"workspace":{"id":%q,"name":%q},"total_contacts":3}`, workspace.ID, workspace.Name), string(result.StructuredContent))
	})

	t.Run("get_contact_timeline lists what happened to a contact", func(t *testing.T) {
		type timeline struct {
			Email   string `json:"email"`
			Entries []struct {
				EntityType string         `json:"entity_type"`
				Kind       string         `json:"kind"`
				Entity     map[string]any `json:"entity"`
			} `json:"entries"`
			Omitted []string `json:"omitted_entity_types"`
			Next    string   `json:"next_cursor"`
		}
		result := full.call("get_contact_timeline", fmt.Sprintf(`{"workspace_id":%q,"email":%q}`, workspace.ID, strings.ToUpper(jane.Email)))
		require.False(t, result.IsError, string(result.StructuredContent))
		var out timeline
		require.NoError(t, json.Unmarshal(result.StructuredContent, &out))
		assert.Equal(t, jane.Email, out.Email)
		assert.Equal(t, []string{}, out.Omitted)
		types := map[string]bool{}
		for _, entry := range out.Entries {
			types[entry.EntityType] = true
			if entry.EntityType == "contact_list" {
				assert.Equal(t, "Newsletter", entry.Entity["name"])
			}
		}
		assert.True(t, types["contact"], string(result.StructuredContent))
		assert.True(t, types["contact_list"], string(result.StructuredContent))

		// One entry a page: the cursor reaches the second.
		result = full.call("get_contact_timeline", fmt.Sprintf(`{"workspace_id":%q,"email":%q,"limit":1}`, workspace.ID, jane.Email))
		require.False(t, result.IsError, string(result.StructuredContent))
		var first timeline
		require.NoError(t, json.Unmarshal(result.StructuredContent, &first))
		require.Len(t, first.Entries, 1)
		require.NotEmpty(t, first.Next)
		result = full.call("get_contact_timeline", fmt.Sprintf(`{"workspace_id":%q,"email":%q,"limit":1,"cursor":%q}`, workspace.ID, jane.Email, first.Next))
		require.False(t, result.IsError, string(result.StructuredContent))
		var second timeline
		require.NoError(t, json.Unmarshal(result.StructuredContent, &second))
		require.Len(t, second.Entries, 1)

		// A key without message_history or web_analytics is told what it misses.
		result = contactsOnly.call("get_contact_timeline", fmt.Sprintf(`{"workspace_id":%q,"email":%q}`, workspace.ID, jane.Email))
		require.False(t, result.IsError, string(result.StructuredContent))
		var restricted timeline
		require.NoError(t, json.Unmarshal(result.StructuredContent, &restricted))
		assert.Equal(t, []string{"message_history", "inbound_webhook_event", "web_session", "web_page"}, restricted.Omitted)

		result = full.call("get_contact_timeline", fmt.Sprintf(`{"workspace_id":%q,"email":"nobody@example.com"}`, workspace.ID))
		assert.Equal(t, "not_found", result.errorCode(t))
	})

	t.Run("list_lists gives each list's counts", func(t *testing.T) {
		result := full.call("list_lists", fmt.Sprintf(`{"workspace_id":%q}`, workspace.ID))
		require.False(t, result.IsError, string(result.StructuredContent))
		var out struct {
			Workspace struct {
				ID string `json:"id"`
			} `json:"workspace"`
			Lists []struct {
				ID    string         `json:"id"`
				Name  string         `json:"name"`
				Stats map[string]int `json:"stats"`
			} `json:"lists"`
		}
		require.NoError(t, json.Unmarshal(result.StructuredContent, &out))
		assert.Equal(t, workspace.ID, out.Workspace.ID)
		require.Len(t, out.Lists, 1)
		assert.Equal(t, newsletter.ID, out.Lists[0].ID)
		assert.Equal(t, map[string]int{"active": 1, "pending": 0, "unsubscribed": 0, "bounced": 0, "complained": 0}, out.Lists[0].Stats)

		result = listsOnly.call("list_lists", fmt.Sprintf(`{"workspace_id":%q}`, workspace.ID))
		require.False(t, result.IsError, string(result.StructuredContent))
	})

	t.Run("list_segments and get_segment read the segment", func(t *testing.T) {
		result := full.call("list_segments", fmt.Sprintf(`{"workspace_id":%q,"with_count":true}`, workspace.ID))
		require.False(t, result.IsError, string(result.StructuredContent))
		var list struct {
			Workspace struct {
				ID string `json:"id"`
			} `json:"workspace"`
			Segments []struct {
				ID         string `json:"id"`
				Name       string `json:"name"`
				Status     string `json:"status"`
				UsersCount *int   `json:"users_count"`
			} `json:"segments"`
		}
		require.NoError(t, json.Unmarshal(result.StructuredContent, &list))
		assert.Equal(t, workspace.ID, list.Workspace.ID)
		require.Len(t, list.Segments, 1)
		assert.Equal(t, segment.ID, list.Segments[0].ID)
		assert.Equal(t, segment.Name, list.Segments[0].Name)
		assert.Equal(t, "active", list.Segments[0].Status)
		require.NotNil(t, list.Segments[0].UsersCount)
		assert.NotContains(t, result.Content[0].Text, "generated_sql")

		result = segmentsOnly.call("get_segment", fmt.Sprintf(`{"workspace_id":%q,"id":%q}`, workspace.ID, segment.ID))
		require.False(t, result.IsError, string(result.StructuredContent))
		var one struct {
			Segment struct {
				ID   string           `json:"id"`
				Tree *domain.TreeNode `json:"tree"`
			} `json:"segment"`
		}
		require.NoError(t, json.Unmarshal(result.StructuredContent, &one))
		assert.Equal(t, segment.ID, one.Segment.ID)
		require.NotNil(t, one.Segment.Tree)
		require.NotNil(t, one.Segment.Tree.Leaf)
		assert.Equal(t, "email", one.Segment.Tree.Leaf.Contact.Filters[0].FieldName)

		result = full.call("get_segment", fmt.Sprintf(`{"workspace_id":%q,"id":"no_such_segment"}`, workspace.ID))
		assert.Equal(t, "not_found", result.errorCode(t))
	})

	t.Run("preview_segment counts the contacts a tree matches", func(t *testing.T) {
		result := full.call("preview_segment", fmt.Sprintf(`{"workspace_id":%q,"tree":{"kind":"branch","branch":{"operator":"and","leaves":[
			{"kind":"leaf","leaf":{"source":"contacts","contact":{"filters":[{"field_name":"external_id","field_type":"string","operator":"equals","string_values":["crm-jane"]}]}}}]}}}`, workspace.ID))
		require.False(t, result.IsError, string(result.StructuredContent))
		assert.JSONEq(t, fmt.Sprintf(`{"workspace":{"id":%q,"name":%q},"total_count":1}`, workspace.ID, workspace.Name), string(result.StructuredContent))

		result = full.call("preview_segment", fmt.Sprintf(`{"workspace_id":%q,"tree":{"kind":"leaf","leaf":{"source":"contacts","contact":{"filters":[{"field_name":"email","field_type":"string","operator":"is_set","negate":true}]}}}}`, workspace.ID))
		assert.Equal(t, "validation", result.errorCode(t), "an unknown key is refused")

		result = full.call("preview_segment", fmt.Sprintf(`{"workspace_id":%q,"tree":{"kind":"leaf","leaf":{"source":"contacts","contact":{"filters":[{"field_name":"password","field_type":"string","operator":"is_set"}]}}}}`, workspace.ID))
		assert.Equal(t, "validation", result.errorCode(t), "a field the query builder refuses")
	})

	t.Run("list_templates and get_template read the templates", func(t *testing.T) {
		result := full.call("list_templates", fmt.Sprintf(`{"workspace_id":%q,"channel":"email"}`, workspace.ID))
		require.False(t, result.IsError, string(result.StructuredContent))
		var list struct {
			Templates []struct {
				ID         string `json:"id"`
				EditorMode string `json:"editor_mode"`
			} `json:"templates"`
		}
		require.NoError(t, json.Unmarshal(result.StructuredContent, &list))
		modes := map[string]string{}
		for _, template := range list.Templates {
			modes[template.ID] = template.EditorMode
		}
		assert.Equal(t, map[string]string{codeTemplate.ID: "code", visualTemplate.ID: "visual"}, modes)
		assert.NotContains(t, result.Content[0].Text, "mjml")

		result = full.call("get_template", fmt.Sprintf(`{"workspace_id":%q,"id":%q}`, workspace.ID, codeTemplate.ID))
		require.False(t, result.IsError, string(result.StructuredContent))
		var code struct {
			Template struct {
				Name       string `json:"name"`
				MJML       string `json:"mjml"`
				MJMLOrigin string `json:"mjml_origin"`
			} `json:"template"`
		}
		require.NoError(t, json.Unmarshal(result.StructuredContent, &code))
		assert.Equal(t, "Welcome", code.Template.Name)
		assert.Contains(t, code.Template.MJML, "Hi {{ contact.first_name }}")
		assert.Equal(t, "source", code.Template.MJMLOrigin)
		assert.NotContains(t, result.Content[0].Text, "editable_by_mcp")

		result = full.call("get_template", fmt.Sprintf(`{"workspace_id":%q,"id":%q}`, workspace.ID, visualTemplate.ID))
		require.False(t, result.IsError, string(result.StructuredContent))
		var visual struct {
			Template struct {
				MJML       string `json:"mjml"`
				MJMLOrigin string `json:"mjml_origin"`
			} `json:"template"`
		}
		require.NoError(t, json.Unmarshal(result.StructuredContent, &visual))
		assert.Contains(t, visual.Template.MJML, "Hello Test!")
		assert.Equal(t, "export", visual.Template.MJMLOrigin)
		assert.NotContains(t, result.Content[0].Text, "visual_editor_tree")

		result = full.call("get_template", fmt.Sprintf(`{"workspace_id":%q,"id":"no-such-template"}`, workspace.ID))
		assert.Equal(t, "not_found", result.errorCode(t))
	})

	t.Run("render_template renders a saved template and a draft", func(t *testing.T) {
		result := full.call("render_template", fmt.Sprintf(`{"workspace_id":%q,"template_id":%q,"test_data":{"contact":{"first_name":"Ada"}},"include_html":true}`, workspace.ID, codeTemplate.ID))
		require.False(t, result.IsError, string(result.StructuredContent))
		var saved struct {
			Success bool    `json:"success"`
			Subject string  `json:"subject"`
			Text    string  `json:"text"`
			HTML    string  `json:"html"`
			Error   *string `json:"error"`
		}
		require.NoError(t, json.Unmarshal(result.StructuredContent, &saved))
		assert.True(t, saved.Success, string(result.StructuredContent))
		assert.Equal(t, "Hello Ada", saved.Subject)
		assert.Contains(t, saved.Text, "Hi Ada")
		assert.Contains(t, saved.HTML, "Hi Ada")

		result = full.call("render_template", fmt.Sprintf(`{"workspace_id":%q,"template_id":%q}`, workspace.ID, visualTemplate.ID))
		require.False(t, result.IsError, string(result.StructuredContent))
		assert.Contains(t, result.Content[0].Text, `"success":true`)
		assert.Contains(t, result.Content[0].Text, "Hello Test!")

		// An MJML error is the answer, not a failed call.
		result = full.call("render_template", fmt.Sprintf(`{"workspace_id":%q,"mjml_source":"<mjml><mj-body><mj-section>","subject":"Hi"}`, workspace.ID))
		require.False(t, result.IsError, string(result.StructuredContent))
		var draft struct {
			Success bool `json:"success"`
			Error   *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		require.NoError(t, json.Unmarshal(result.StructuredContent, &draft))
		assert.False(t, draft.Success)
		require.NotNil(t, draft.Error, string(result.StructuredContent))
		assert.NotEmpty(t, draft.Error.Message)
	})

	t.Run("list_broadcasts, get_broadcast and get_broadcast_report read a broadcast", func(t *testing.T) {
		result := full.call("list_broadcasts", fmt.Sprintf(`{"workspace_id":%q,"statuses":["processed"],"search":"november"}`, workspace.ID))
		require.False(t, result.IsError, string(result.StructuredContent))
		var list struct {
			Workspace struct {
				ID string `json:"id"`
			} `json:"workspace"`
			Broadcasts []struct {
				ID          string   `json:"id"`
				Name        string   `json:"name"`
				TemplateIDs []string `json:"template_ids"`
			} `json:"broadcasts"`
			TotalCount int `json:"total_count"`
		}
		require.NoError(t, json.Unmarshal(result.StructuredContent, &list))
		assert.Equal(t, workspace.ID, list.Workspace.ID)
		require.Len(t, list.Broadcasts, 1)
		assert.Equal(t, broadcast.ID, list.Broadcasts[0].ID)
		assert.Equal(t, []string{codeTemplate.ID}, list.Broadcasts[0].TemplateIDs)
		assert.Equal(t, 1, list.TotalCount)
		assert.NotContains(t, result.Content[0].Text, "feed-secret")

		result = broadcastsOnly.call("get_broadcast", fmt.Sprintf(`{"workspace_id":%q,"broadcast_id":%q}`, workspace.ID, broadcast.ID))
		require.False(t, result.IsError, string(result.StructuredContent))
		var one struct {
			Broadcast struct {
				Name     string `json:"name"`
				DataFeed struct {
					GlobalFeed struct {
						URL         string   `json:"url"`
						HeaderNames []string `json:"header_names"`
					} `json:"global_feed"`
				} `json:"data_feed"`
			} `json:"broadcast"`
		}
		require.NoError(t, json.Unmarshal(result.StructuredContent, &one))
		assert.Equal(t, "November newsletter", one.Broadcast.Name)
		assert.Equal(t, "https://feed.example.com/products", one.Broadcast.DataFeed.GlobalFeed.URL)
		assert.Equal(t, []string{"Authorization"}, one.Broadcast.DataFeed.GlobalFeed.HeaderNames)
		for _, secret := range []string{"feed-secret", "fetched-payload", "global_feed_data"} {
			assert.NotContains(t, result.Content[0].Text, secret)
		}

		result = full.call("get_broadcast_report", fmt.Sprintf(`{"workspace_id":%q,"broadcast_id":%q}`, workspace.ID, broadcast.ID))
		require.False(t, result.IsError, string(result.StructuredContent))
		var report struct {
			Broadcast struct {
				ID string `json:"id"`
			} `json:"broadcast"`
			Totals     map[string]int    `json:"totals"`
			Variations []json.RawMessage `json:"variations"`
		}
		require.NoError(t, json.Unmarshal(result.StructuredContent, &report))
		assert.Equal(t, broadcast.ID, report.Broadcast.ID)
		assert.Equal(t, 2, report.Totals["sent"])
		assert.Equal(t, 1, report.Totals["delivered"])
		assert.Equal(t, 1, report.Totals["opened"])
		assert.Empty(t, report.Variations, "no A/B test")

		// The stats alone would answer zeros: the broadcast read answers not_found.
		result = full.call("get_broadcast_report", fmt.Sprintf(`{"workspace_id":%q,"broadcast_id":"no-such-broadcast"}`, workspace.ID))
		assert.Equal(t, "not_found", result.errorCode(t))
		result = full.call("get_broadcast", fmt.Sprintf(`{"workspace_id":%q,"broadcast_id":"no-such-broadcast"}`, workspace.ID))
		assert.Equal(t, "not_found", result.errorCode(t))
	})

	t.Run("list_transactional_notifications and get_transactional_notification read a notification", func(t *testing.T) {
		result := full.call("list_transactional_notifications", fmt.Sprintf(`{"workspace_id":%q,"search":"reset"}`, workspace.ID))
		require.False(t, result.IsError, string(result.StructuredContent))
		var list struct {
			Notifications []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"notifications"`
			Total int `json:"total"`
		}
		require.NoError(t, json.Unmarshal(result.StructuredContent, &list))
		require.Len(t, list.Notifications, 1)
		assert.Equal(t, "password_reset", list.Notifications[0].ID)
		assert.Equal(t, "Password reset", list.Notifications[0].Name)
		assert.Equal(t, 1, list.Total)

		result = full.call("get_transactional_notification", fmt.Sprintf(`{"workspace_id":%q,"notification_id":"password_reset"}`, workspace.ID))
		require.False(t, result.IsError, string(result.StructuredContent))
		var one struct {
			Notification struct {
				Channels map[string]struct {
					TemplateID string `json:"template_id"`
				} `json:"channels"`
				Tracking struct {
					EnableTracking bool `json:"enable_tracking"`
				} `json:"tracking"`
			} `json:"notification"`
		}
		require.NoError(t, json.Unmarshal(result.StructuredContent, &one))
		assert.Equal(t, notification.Channels[domain.TransactionalChannelEmail].TemplateID, one.Notification.Channels["email"].TemplateID)
		assert.True(t, one.Notification.Tracking.EnableTracking)

		result = full.call("get_transactional_notification", fmt.Sprintf(`{"workspace_id":%q,"notification_id":"no_such_notification"}`, workspace.ID))
		assert.Equal(t, "not_found", result.errorCode(t))
	})

	t.Run("search_messages finds a broadcast's messages without their data", func(t *testing.T) {
		result := full.call("search_messages", fmt.Sprintf(`{"workspace_id":%q,"broadcast_id":%q,"limit":1}`, workspace.ID, broadcast.ID))
		require.False(t, result.IsError, string(result.StructuredContent))
		type page struct {
			Messages []struct {
				ID           string `json:"id"`
				ContactEmail string `json:"contact_email"`
				Origin       string `json:"origin"`
			} `json:"messages"`
			Next string `json:"next_cursor"`
		}
		var first page
		require.NoError(t, json.Unmarshal(result.StructuredContent, &first))
		require.Len(t, first.Messages, 1)
		assert.Equal(t, "broadcast", first.Messages[0].Origin)
		require.NotEmpty(t, first.Next)
		for _, dropped := range []string{"message_data", "This is a test message"} {
			assert.NotContains(t, result.Content[0].Text, dropped)
		}

		result = full.call("search_messages", fmt.Sprintf(`{"workspace_id":%q,"broadcast_id":%q,"limit":1,"cursor":%q}`, workspace.ID, broadcast.ID, first.Next))
		require.False(t, result.IsError, string(result.StructuredContent))
		var second page
		require.NoError(t, json.Unmarshal(result.StructuredContent, &second))
		require.Len(t, second.Messages, 1)
		assert.NotEqual(t, first.Messages[0].ID, second.Messages[0].ID)

		result = full.call("search_messages", fmt.Sprintf(`{"workspace_id":%q,"contact_email":%q,"status":"opened"}`, workspace.ID, strings.ToUpper(jane.Email)))
		require.False(t, result.IsError, string(result.StructuredContent))
		var opened page
		require.NoError(t, json.Unmarshal(result.StructuredContent, &opened))
		require.Len(t, opened.Messages, 1)
		assert.Equal(t, jane.Email, opened.Messages[0].ContactEmail)
	})

	t.Run("list_analytics_schemas and query_analytics count the messages", func(t *testing.T) {
		result := full.call("list_analytics_schemas", fmt.Sprintf(`{"workspace_id":%q}`, workspace.ID))
		require.False(t, result.IsError, string(result.StructuredContent))
		var schemas struct {
			Schemas []struct {
				Name string `json:"name"`
			} `json:"schemas"`
		}
		require.NoError(t, json.Unmarshal(result.StructuredContent, &schemas))
		names := []string{}
		for _, schema := range schemas.Schemas {
			names = append(names, schema.Name)
		}
		assert.Contains(t, names, "message_history")
		assert.NotContains(t, names, "webhook_deliveries")
		assert.NotContains(t, result.Content[0].Text, `"sql"`)

		result = listsOnly.call("list_analytics_schemas", fmt.Sprintf(`{"workspace_id":%q}`, workspace.ID))
		require.False(t, result.IsError, string(result.StructuredContent))
		assert.Contains(t, result.Content[0].Text, `"schemas":[]`, "a key without any schema's resource")

		result = full.call("query_analytics", fmt.Sprintf(`{"workspace_id":%q,"schema":"message_history","measures":["count"],
			"filters":[{"member":"broadcast_id","operator":"equals","values":[%q]}]}`, workspace.ID, broadcast.ID))
		require.False(t, result.IsError, string(result.StructuredContent))
		var query struct {
			Schema       string           `json:"schema"`
			Rows         []map[string]any `json:"rows"`
			RowCount     int              `json:"row_count"`
			LimitReached bool             `json:"limit_reached"`
		}
		require.NoError(t, json.Unmarshal(result.StructuredContent, &query))
		assert.Equal(t, "message_history", query.Schema)
		require.Len(t, query.Rows, 1)
		assert.EqualValues(t, 2, query.Rows[0]["count"])
		assert.Equal(t, 1, query.RowCount)
		assert.False(t, query.LimitReached)
		assert.NotContains(t, result.Content[0].Text, "SELECT")

		// A complete page of exactly limit rows is not reported as cut.
		result = full.call("query_analytics", fmt.Sprintf(`{"workspace_id":%q,"schema":"message_history","measures":["count"],"limit":1,
			"filters":[{"member":"broadcast_id","operator":"equals","values":[%q]}]}`, workspace.ID, broadcast.ID))
		require.False(t, result.IsError, string(result.StructuredContent))
		require.NoError(t, json.Unmarshal(result.StructuredContent, &query))
		assert.Equal(t, 1, query.RowCount)
		assert.False(t, query.LimitReached)

		result = full.call("query_analytics", fmt.Sprintf(`{"workspace_id":%q,"schema":"webhook_deliveries","measures":["count"]}`, workspace.ID))
		assert.Equal(t, "validation", result.errorCode(t))
	})

	t.Run("list_automations and get_automation read the automation without its webhook secret", func(t *testing.T) {
		result := automationsOnly.call("list_automations", fmt.Sprintf(`{"workspace_id":%q,"status":["live"]}`, workspace.ID))
		require.False(t, result.IsError, string(result.StructuredContent))
		var list struct {
			Automations []struct {
				ID        string `json:"id"`
				Name      string `json:"name"`
				NodeCount int    `json:"node_count"`
				Journeys  struct {
					Active int `json:"active"`
				} `json:"journeys"`
			} `json:"automations"`
			Total int `json:"total"`
		}
		require.NoError(t, json.Unmarshal(result.StructuredContent, &list))
		require.Len(t, list.Automations, 1)
		assert.Equal(t, automationID, list.Automations[0].ID)
		assert.Equal(t, "Welcome series", list.Automations[0].Name)
		assert.Equal(t, 2, list.Automations[0].NodeCount)
		assert.Equal(t, 1, list.Automations[0].Journeys.Active)
		assert.Equal(t, 1, list.Total)

		// The full key may write automations, which the API answers with the
		// secret: the tool drops it all the same.
		for _, client := range []*mcpClient{full, automationsOnly} {
			result = client.call("get_automation", fmt.Sprintf(`{"workspace_id":%q,"automation_id":%q}`, workspace.ID, automationID))
			require.False(t, result.IsError, string(result.StructuredContent))
			var one struct {
				Automation struct {
					RootNodeID string `json:"root_node_id"`
					Nodes      []struct {
						ID     string                 `json:"id"`
						Config map[string]interface{} `json:"config"`
					} `json:"nodes"`
				} `json:"automation"`
			}
			require.NoError(t, json.Unmarshal(result.StructuredContent, &one))
			assert.Equal(t, triggerNodeID, one.Automation.RootNodeID)
			require.Len(t, one.Automation.Nodes, 2)
			assert.Equal(t, map[string]interface{}{"url": "https://hooks.example.com/welcome"}, one.Automation.Nodes[1].Config)
			assert.NotContains(t, result.Content[0].Text, "hook-bearer-secret")
		}

		result = full.call("get_automation", fmt.Sprintf(`{"workspace_id":%q,"automation_id":"no-such-automation"}`, workspace.ID))
		assert.Equal(t, "not_found", result.errorCode(t))
	})

	t.Run("get_automation_journey and get_automation_stats read Jane's path", func(t *testing.T) {
		result := automationsOnly.call("get_automation_journey", fmt.Sprintf(`{"workspace_id":%q,"automation_id":%q,"email":%q}`,
			workspace.ID, automationID, strings.ToUpper(jane.Email)))
		require.False(t, result.IsError, string(result.StructuredContent))
		var journey struct {
			Journey struct {
				ContactEmail  string          `json:"contact_email"`
				Status        string          `json:"status"`
				CurrentNodeID string          `json:"current_node_id"`
				LastError     string          `json:"last_error"`
				TriggerEvent  json.RawMessage `json:"trigger_event"`
			} `json:"journey"`
			Steps []struct {
				NodeID string          `json:"node_id"`
				Action string          `json:"action"`
				Output json.RawMessage `json:"output"`
				Error  string          `json:"error"`
			} `json:"steps"`
		}
		require.NoError(t, json.Unmarshal(result.StructuredContent, &journey))
		assert.Equal(t, jane.Email, journey.Journey.ContactEmail)
		assert.Equal(t, "active", journey.Journey.Status)
		assert.Equal(t, webhookNodeID, journey.Journey.CurrentNodeID)
		assert.Equal(t, "webhook returned 502", journey.Journey.LastError)
		assert.JSONEq(t, `{"kind":"contact.created"}`, string(journey.Journey.TriggerEvent))
		require.Len(t, journey.Steps, 1)
		assert.Equal(t, webhookNodeID, journey.Steps[0].NodeID)
		assert.Equal(t, "failed", journey.Steps[0].Action)
		assert.JSONEq(t, `{"raw":"bad gateway"}`, string(journey.Steps[0].Output))
		assert.Equal(t, "webhook returned 502", journey.Steps[0].Error)
		assert.NotContains(t, result.Content[0].Text, journeyID)

		result = automationsOnly.call("get_automation_journey", fmt.Sprintf(`{"workspace_id":%q,"automation_id":%q,"email":"nobody@example.com"}`, workspace.ID, automationID))
		assert.Equal(t, "not_found", result.errorCode(t))

		result = automationsOnly.call("get_automation_stats", fmt.Sprintf(`{"workspace_id":%q,"automation_id":%q}`, workspace.ID, automationID))
		require.False(t, result.IsError, string(result.StructuredContent))
		var stats struct {
			AutomationID string `json:"automation_id"`
			Nodes        map[string]struct {
				NodeType       string `json:"node_type"`
				Visits         int    `json:"visits"`
				FailedAttempts int    `json:"failed_attempts"`
			} `json:"nodes"`
		}
		require.NoError(t, json.Unmarshal(result.StructuredContent, &stats))
		assert.Equal(t, automationID, stats.AutomationID)
		require.Contains(t, stats.Nodes, webhookNodeID)
		assert.Equal(t, "webhook", stats.Nodes[webhookNodeID].NodeType)
		assert.Equal(t, 1, stats.Nodes[webhookNodeID].FailedAttempts)
		require.Contains(t, stats.Nodes, triggerNodeID)

		result = automationsOnly.call("get_automation_stats", fmt.Sprintf(`{"workspace_id":%q,"automation_id":%q,"since":"2026-11-01T00:00:00Z"}`, workspace.ID, automationID))
		assert.Equal(t, "validation", result.errorCode(t))
	})

	t.Run("list_blog_posts, get_blog_post and list_blog_categories read the blog", func(t *testing.T) {
		result := blogOnly.call("list_blog_categories", fmt.Sprintf(`{"workspace_id":%q}`, workspace.ID))
		require.False(t, result.IsError, string(result.StructuredContent))
		var categories struct {
			Categories []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"categories"`
			Total int `json:"total"`
		}
		require.NoError(t, json.Unmarshal(result.StructuredContent, &categories))
		require.Len(t, categories.Categories, 1)
		assert.Equal(t, category.ID, categories.Categories[0].ID)
		assert.Equal(t, category.Settings.Name, categories.Categories[0].Name)

		result = blogOnly.call("list_blog_posts", fmt.Sprintf(`{"workspace_id":%q,"category_id":%q,"status":"draft","search":"SPRING"}`,
			workspace.ID, strings.ToUpper(category.ID)))
		require.False(t, result.IsError, string(result.StructuredContent))
		var posts struct {
			Posts []struct {
				ID     string `json:"id"`
				Title  string `json:"title"`
				Status string `json:"status"`
			} `json:"posts"`
			Total int `json:"total"`
		}
		require.NoError(t, json.Unmarshal(result.StructuredContent, &posts))
		require.Len(t, posts.Posts, 1)
		assert.Equal(t, post.ID, posts.Posts[0].ID)
		assert.Equal(t, "What's new this spring", posts.Posts[0].Title)
		assert.Equal(t, "draft", posts.Posts[0].Status)
		assert.Equal(t, 1, posts.Total)

		type onePost struct {
			Post struct {
				Slug              string          `json:"slug"`
				Body              json.RawMessage `json:"body"`
				BodyVersion       *int64          `json:"body_version"`
				FollowsLatestBody bool            `json:"follows_latest_body"`
				BodyUnavailable   string          `json:"body_unavailable"`
			} `json:"post"`
		}
		result = full.call("get_blog_post", fmt.Sprintf(`{"workspace_id":%q,"slug":"spring-release"}`, workspace.ID))
		require.False(t, result.IsError, string(result.StructuredContent))
		var withBody onePost
		require.NoError(t, json.Unmarshal(result.StructuredContent, &withBody))
		assert.Equal(t, "spring-release", withBody.Post.Slug)
		assert.JSONEq(t, `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Spring is here"}]}]}`, string(withBody.Post.Body))
		require.NotNil(t, withBody.Post.BodyVersion)
		assert.Equal(t, int64(1), *withBody.Post.BodyVersion)
		assert.True(t, withBody.Post.FollowsLatestBody)
		assert.NotContains(t, result.Content[0].Text, "rendered-html")

		// Without templates:read, the post comes without its body.
		result = blogOnly.call("get_blog_post", fmt.Sprintf(`{"workspace_id":%q,"id":%q}`, workspace.ID, post.ID))
		require.False(t, result.IsError, string(result.StructuredContent))
		var withoutBody onePost
		require.NoError(t, json.Unmarshal(result.StructuredContent, &withoutBody))
		assert.Equal(t, "spring-release", withoutBody.Post.Slug)
		assert.Equal(t, "null", string(withoutBody.Post.Body))
		assert.Equal(t, "permission_denied", withoutBody.Post.BodyUnavailable)

		result = full.call("get_blog_post", fmt.Sprintf(`{"workspace_id":%q,"slug":"no-such-post"}`, workspace.ID))
		assert.Equal(t, "not_found", result.errorCode(t))
		result = full.call("get_blog_post", fmt.Sprintf(`{"workspace_id":%q,"id":"not-a-uuid"}`, workspace.ID))
		assert.Equal(t, "validation", result.errorCode(t))
	})

	t.Run("a key without the read pair is refused each tool", func(t *testing.T) {
		for _, refusal := range []struct {
			client   *mcpClient
			tool     string
			args     string
			resource string
		}{
			{listsOnly, "get_contact", fmt.Sprintf(`{"workspace_id":%q,"email":%q}`, workspace.ID, jane.Email), "contacts"},
			{listsOnly, "count_contacts", fmt.Sprintf(`{"workspace_id":%q}`, workspace.ID), "contacts"},
			{listsOnly, "get_contact_timeline", fmt.Sprintf(`{"workspace_id":%q,"email":%q}`, workspace.ID, jane.Email), "contacts"},
			{listsOnly, "get_workspace_overview", fmt.Sprintf(`{"workspace_id":%q}`, workspace.ID), "workspace"},
			{contactsOnly, "get_workspace_overview", fmt.Sprintf(`{"workspace_id":%q}`, workspace.ID), "workspace"},
			{contactsOnly, "list_lists", fmt.Sprintf(`{"workspace_id":%q}`, workspace.ID), "lists"},
			{contactsOnly, "list_segments", fmt.Sprintf(`{"workspace_id":%q}`, workspace.ID), "segments"},
			{contactsOnly, "get_segment", fmt.Sprintf(`{"workspace_id":%q,"id":%q}`, workspace.ID, segment.ID), "segments"},
			{contactsOnly, "preview_segment", fmt.Sprintf(`{"workspace_id":%q,"tree":{"kind":"leaf","leaf":{"source":"contacts","contact":{"filters":[{"field_name":"email","field_type":"string","operator":"is_set"}]}}}}`, workspace.ID), "segments"},
			{segmentsOnly, "preview_segment", fmt.Sprintf(`{"workspace_id":%q,"tree":{"kind":"leaf","leaf":{"source":"contacts","contact":{"filters":[{"field_name":"email","field_type":"string","operator":"is_set"}]}}}}`, workspace.ID), "contacts"},
			{contactsOnly, "list_templates", fmt.Sprintf(`{"workspace_id":%q}`, workspace.ID), "templates"},
			{contactsOnly, "get_template", fmt.Sprintf(`{"workspace_id":%q,"id":%q}`, workspace.ID, codeTemplate.ID), "templates"},
			{contactsOnly, "render_template", fmt.Sprintf(`{"workspace_id":%q,"template_id":%q}`, workspace.ID, codeTemplate.ID), "templates"},
			{listsOnly, "list_broadcasts", fmt.Sprintf(`{"workspace_id":%q}`, workspace.ID), "broadcasts"},
			{listsOnly, "get_broadcast", fmt.Sprintf(`{"workspace_id":%q,"broadcast_id":%q}`, workspace.ID, broadcast.ID), "broadcasts"},
			{listsOnly, "get_broadcast_report", fmt.Sprintf(`{"workspace_id":%q,"broadcast_id":%q}`, workspace.ID, broadcast.ID), "broadcasts"},
			{broadcastsOnly, "get_broadcast_report", fmt.Sprintf(`{"workspace_id":%q,"broadcast_id":%q}`, workspace.ID, broadcast.ID), "message_history"},
			{listsOnly, "list_transactional_notifications", fmt.Sprintf(`{"workspace_id":%q}`, workspace.ID), "transactional"},
			{listsOnly, "get_transactional_notification", fmt.Sprintf(`{"workspace_id":%q,"notification_id":%q}`, workspace.ID, notification.ID), "transactional"},
			{broadcastsOnly, "search_messages", fmt.Sprintf(`{"workspace_id":%q}`, workspace.ID), "message_history"},
			{broadcastsOnly, "query_analytics", fmt.Sprintf(`{"workspace_id":%q,"schema":"message_history","measures":["count"]}`, workspace.ID), "message_history"},
			{listsOnly, "query_analytics", fmt.Sprintf(`{"workspace_id":%q,"schema":"contacts","measures":["count"]}`, workspace.ID), "contacts"},
			{blogOnly, "list_automations", fmt.Sprintf(`{"workspace_id":%q}`, workspace.ID), "automations"},
			{blogOnly, "get_automation", fmt.Sprintf(`{"workspace_id":%q,"automation_id":%q}`, workspace.ID, automationID), "automations"},
			{blogOnly, "get_automation_journey", fmt.Sprintf(`{"workspace_id":%q,"automation_id":%q,"email":%q}`, workspace.ID, automationID, jane.Email), "automations"},
			{blogOnly, "get_automation_stats", fmt.Sprintf(`{"workspace_id":%q,"automation_id":%q}`, workspace.ID, automationID), "automations"},
			{automationsOnly, "list_blog_posts", fmt.Sprintf(`{"workspace_id":%q}`, workspace.ID), "blog"},
			{automationsOnly, "get_blog_post", fmt.Sprintf(`{"workspace_id":%q,"id":%q}`, workspace.ID, post.ID), "blog"},
			{automationsOnly, "list_blog_categories", fmt.Sprintf(`{"workspace_id":%q}`, workspace.ID), "blog"},
		} {
			result := refusal.client.call(refusal.tool, refusal.args)
			assert.JSONEq(t, fmt.Sprintf(`{"error":{"code":"permission_denied","message":"this API key lacks the %s read permission","resource":%q,"permission":"read"}}`, refusal.resource, refusal.resource),
				string(result.StructuredContent), refusal.tool)
		}
	})

	t.Run("invalid arguments answer validation", func(t *testing.T) {
		result := full.call("search_contacts", fmt.Sprintf(`{"workspace_id":%q,"limit":"ten"}`, workspace.ID))
		assert.Equal(t, "validation", result.errorCode(t))
	})

	t.Run("the Go SDK client at 2025-11-25 lists and calls tools", func(t *testing.T) {
		client := sdk.NewClient(&sdk.Implementation{Name: "integration", Version: "1"}, nil)
		transport := &sdk.StreamableClientTransport{
			Endpoint:   url + "/mcp",
			HTTPClient: &http.Client{Transport: bearerTransport{token: full.token}, Timeout: 30 * time.Second},
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		session, err := client.Connect(ctx, transport, &sdk.ClientSessionOptions{ProtocolVersion: "2025-11-25"})
		require.NoError(t, err)
		defer func() { _ = session.Close() }()
		assert.Equal(t, "2025-11-25", session.InitializeResult().ProtocolVersion)

		tools, err := session.ListTools(ctx, nil)
		require.NoError(t, err)
		names := []string{}
		for _, tool := range tools.Tools {
			names = append(names, tool.Name)
		}
		assert.Equal(t, allTools, names)

		result, err := session.CallTool(ctx, &sdk.CallToolParams{Name: "search_contacts", Arguments: map[string]any{"workspace_id": workspace.ID}})
		require.NoError(t, err)
		assert.False(t, result.IsError)
		structured, err := json.Marshal(result.StructuredContent)
		require.NoError(t, err)
		assert.Contains(t, string(structured), workspace.ID)
	})

	t.Run("a switched-off workspace is out of reach", func(t *testing.T) {
		ownerClient := testutil.NewAPIClient(url)
		require.NoError(t, ownerClient.Login(owner.Email, "password"))
		resp, err := ownerClient.PostRaw("/api/workspaces.setMCPSettings", fmt.Sprintf(`{"workspace_id":%q,"disabled":true}`, workspace.ID))
		require.NoError(t, err)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		result := full.call("list_workspaces", `{}`)
		require.False(t, result.IsError)
		assert.JSONEq(t, `{"workspaces":[]}`, string(result.StructuredContent))

		result = full.call("search_contacts", fmt.Sprintf(`{"workspace_id":%q}`, workspace.ID))
		assert.Equal(t, "not_found", result.errorCode(t))
		assert.Equal(t, []string{"list_workspaces"}, full.toolNames())

		// The same key keeps working on the API.
		apiClient := testutil.NewAPIClient(url)
		apiClient.SetToken(full.token)
		apiResp, err := apiClient.Get("/api/contacts.list", map[string]string{"workspace_id": workspace.ID})
		require.NoError(t, err)
		_ = apiResp.Body.Close()
		assert.Equal(t, http.StatusOK, apiResp.StatusCode)
	})
}

// MCP is off on demo deployments, whatever MCP_ENABLED says.
func TestMCPOffInDemo(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		cfg.Environment = "demo"
		return mcpAppFactory(cfg)
	})
	defer func() { suite.Cleanup() }()
	suite.Config.APIEndpoint = suite.ServerManager.GetURL()

	workspace, err := suite.DataFactory.CreateWorkspace()
	require.NoError(t, err)
	key, err := suite.DataFactory.CreateAPIKey(workspace.ID)
	require.NoError(t, err)
	authService := suite.ServerManager.GetApp().GetAuthService().(*service.AuthService)

	client := &mcpClient{t: t, url: suite.ServerManager.GetURL(), token: authService.GenerateAPIAuthToken(key)}
	resp := client.post("server/discover", "", "")
	assert.Equal(t, http.StatusNotFound, resp.status)
	assert.JSONEq(t, `{"error":"MCP is not enabled on this server"}`, resp.body)
}

// allTools is tools/list for a key holding every permission, in its order.
var allTools = []string{
	"activate_automation", "cancel_broadcast", "count_contacts", "create_blog_post", "create_broadcast", "create_list",
	"create_segment", "create_template", "create_transactional_notification", "delete_automation", "delete_blog_post",
	"delete_broadcast", "delete_contact", "delete_list", "delete_segment", "delete_template", "delete_transactional_notification",
	"get_automation", "get_automation_journey", "get_automation_stats", "get_blog_post", "get_broadcast", "get_broadcast_report",
	"get_contact", "get_contact_timeline", "get_segment", "get_template", "get_transactional_notification",
	"get_workspace_overview", "list_analytics_schemas", "list_automations", "list_blog_categories", "list_blog_posts",
	"list_broadcasts", "list_lists", "list_segments", "list_templates", "list_transactional_notifications", "list_workspaces",
	"pause_automation", "pause_broadcast", "preview_segment", "publish_blog_post", "query_analytics", "remove_contact_from_list",
	"render_template", "resume_broadcast", "schedule_broadcast", "search_contacts", "search_messages", "select_broadcast_winner",
	"send_broadcast_to_contact", "send_template_test", "send_transactional", "set_contact_list_status", "subscribe_contact_to_list",
	"track_custom_event", "unpublish_blog_post", "update_blog_post", "update_broadcast", "update_list", "update_segment",
	"update_template", "update_transactional_notification", "upsert_contact",
}

type bearerTransport struct {
	token string
}

func (b bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return http.DefaultTransport.RoundTrip(r)
}
