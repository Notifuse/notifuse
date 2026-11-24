package mcp

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
)

func TestCatalogue_ToolNames(t *testing.T) {
	s := newBareServer(Deps{})
	assert.Equal(t, []string{
		"list_workspaces",
		"get_workspace_overview",
		"search_contacts",
		"get_contact",
		"count_contacts",
		"get_contact_timeline",
		"upsert_contact",
		"delete_contact",
		"track_custom_event",
		"list_lists",
		"create_list",
		"update_list",
		"delete_list",
		"subscribe_contact_to_list",
		"set_contact_list_status",
		"remove_contact_from_list",
		"list_segments",
		"get_segment",
		"preview_segment",
		"create_segment",
		"update_segment",
		"delete_segment",
		"list_templates",
		"get_template",
		"render_template",
		"create_template",
		"update_template",
		"delete_template",
		"list_broadcasts",
		"get_broadcast",
		"get_broadcast_report",
		"create_broadcast",
		"update_broadcast",
		"send_broadcast_to_contact",
		"schedule_broadcast",
		"select_broadcast_winner",
		"pause_broadcast",
		"resume_broadcast",
		"cancel_broadcast",
		"unschedule_broadcast",
		"delete_broadcast",
		"list_transactional_notifications",
		"get_transactional_notification",
		"create_transactional_notification",
		"update_transactional_notification",
		"delete_transactional_notification",
		"send_transactional",
		"send_template_test",
		"search_messages",
		"list_analytics_schemas",
		"query_analytics",
		"list_automations",
		"get_automation",
		"get_automation_journey",
		"get_automation_stats",
		"activate_automation",
		"pause_automation",
		"delete_automation",
		"list_blog_posts",
		"get_blog_post",
		"list_blog_categories",
		"create_blog_post",
		"update_blog_post",
		"publish_blog_post",
		"unpublish_blog_post",
		"delete_blog_post",
	}, toolNames(s.catalogue()))
}

// A description names only tools of the catalogue, or arguments of its own
// tool: a write tool named before it exists sends an assistant nowhere.
func TestCatalogue_DescriptionsNameOnlyExistingTools(t *testing.T) {
	s := newBareServer(Deps{})
	names := map[string]bool{}
	for _, tool := range s.catalogue() {
		names[tool.Name] = true
	}
	toolName := regexp.MustCompile(`\b(create|update|delete|send|upsert|import|cancel|schedule|pause|resume|publish|unpublish|archive|remove|add|subscribe|set|track|activate|select|unschedule)_[a-z_]+`)
	for _, tool := range s.catalogue() {
		properties, _ := tool.InputSchema["properties"].(map[string]any)
		var texts func(v any)
		texts = func(v any) {
			switch v := v.(type) {
			case string:
				for _, match := range toolName.FindAllString(v, -1) {
					_, argument := properties[match]
					assert.True(t, names[match] || argument, "%s names %s, which is neither a tool nor one of its arguments", tool.Name, match)
				}
				assert.NotContains(t, v, "through MCP", tool.Name)
				assert.NotContains(t, v, "deletion", tool.Name)
			case map[string]any:
				for _, item := range v {
					texts(item)
				}
			case []any:
				for _, item := range v {
					texts(item)
				}
			}
		}
		texts(tool.Description)
		texts(tool.InputSchema)
	}
}

// Each tool's class, permission pairs and workspace argument, as spec §6.2
// states them.
func TestCatalogue_ToolDefinitions(t *testing.T) {
	s := newBareServer(Deps{})
	type definition struct {
		class            Class
		pairs            []Pair
		withoutWorkspace bool
	}
	want := map[string]definition{
		"list_workspaces":        {class: ClassRead, pairs: nil, withoutWorkspace: true},
		"get_workspace_overview": {class: ClassRead, pairs: []Pair{{Resource: "workspace", Verb: "read"}, {Resource: "lists", Verb: "read"}}},
		"search_contacts":        {class: ClassRead, pairs: []Pair{{Resource: "contacts", Verb: "read"}}},
		"get_contact":            {class: ClassRead, pairs: []Pair{{Resource: "contacts", Verb: "read"}}},
		"count_contacts":         {class: ClassRead, pairs: []Pair{{Resource: "contacts", Verb: "read"}}},
		"get_contact_timeline":   {class: ClassRead, pairs: []Pair{{Resource: "contacts", Verb: "read"}}},
		"upsert_contact":         {class: ClassWriteOpenWorld, pairs: []Pair{{Resource: "contacts", Verb: "write"}}},
		"delete_contact":         {class: ClassDestructive, pairs: []Pair{{Resource: "contacts", Verb: "write"}}},
		"track_custom_event":     {class: ClassWriteOpenWorld, pairs: []Pair{{Resource: "contacts", Verb: "write"}}},
		"list_lists":             {class: ClassRead, pairs: []Pair{{Resource: "lists", Verb: "read"}}},
		"create_list":            {class: ClassWrite, pairs: []Pair{{Resource: "lists", Verb: "write"}}},
		// A patch: the list is read before it is written.
		"update_list":               {class: ClassWriteOpenWorld, pairs: []Pair{{Resource: "lists", Verb: "write"}, {Resource: "lists", Verb: "read"}}},
		"delete_list":               {class: ClassDestructive, pairs: []Pair{{Resource: "lists", Verb: "write"}}},
		"subscribe_contact_to_list": {class: ClassWriteOpenWorld, pairs: []Pair{{Resource: "lists", Verb: "write"}, {Resource: "contacts", Verb: "write"}}},
		"set_contact_list_status":   {class: ClassWriteOpenWorld, pairs: []Pair{{Resource: "lists", Verb: "write"}, {Resource: "contacts", Verb: "write"}}},
		"remove_contact_from_list":  {class: ClassDestructive, pairs: []Pair{{Resource: "lists", Verb: "write"}, {Resource: "contacts", Verb: "write"}}},
		"list_segments":             {class: ClassRead, pairs: []Pair{{Resource: "segments", Verb: "read"}}},
		"get_segment":               {class: ClassRead, pairs: []Pair{{Resource: "segments", Verb: "read"}}},
		"preview_segment":           {class: ClassRead, pairs: []Pair{{Resource: "segments", Verb: "read"}, {Resource: "contacts", Verb: "read"}}},
		"create_segment":            {class: ClassWrite, pairs: []Pair{{Resource: "segments", Verb: "write"}}},
		// A patch: segments.update requires every field, so the segment is
		// read before it is written.
		"update_segment":  {class: ClassWrite, pairs: []Pair{{Resource: "segments", Verb: "write"}, {Resource: "segments", Verb: "read"}}},
		"delete_segment":  {class: ClassDestructive, pairs: []Pair{{Resource: "segments", Verb: "write"}}},
		"list_templates":  {class: ClassRead, pairs: []Pair{{Resource: "templates", Verb: "read"}}},
		"get_template":    {class: ClassRead, pairs: []Pair{{Resource: "templates", Verb: "read"}}},
		"render_template": {class: ClassRead, pairs: []Pair{{Resource: "templates", Verb: "read"}}},
		"create_template": {class: ClassWrite, pairs: []Pair{{Resource: "templates", Verb: "write"}}},
		// A patch: templates.update replaces the whole template.
		"update_template":                  {class: ClassWriteOpenWorld, pairs: []Pair{{Resource: "templates", Verb: "write"}, {Resource: "templates", Verb: "read"}}},
		"delete_template":                  {class: ClassDestructive, pairs: []Pair{{Resource: "templates", Verb: "write"}}},
		"list_broadcasts":                  {class: ClassRead, pairs: []Pair{{Resource: "broadcasts", Verb: "read"}}},
		"get_broadcast":                    {class: ClassRead, pairs: []Pair{{Resource: "broadcasts", Verb: "read"}}},
		"get_broadcast_report":             {class: ClassRead, pairs: []Pair{{Resource: "broadcasts", Verb: "read"}, {Resource: "message_history", Verb: "read"}}},
		"list_transactional_notifications": {class: ClassRead, pairs: []Pair{{Resource: "transactional", Verb: "read"}}},
		"get_transactional_notification":   {class: ClassRead, pairs: []Pair{{Resource: "transactional", Verb: "read"}}},
		"create_broadcast":                 {class: ClassWrite, pairs: []Pair{{Resource: "broadcasts", Verb: "write"}}},
		"update_broadcast":                 {class: ClassWriteOpenWorld, pairs: []Pair{{Resource: "broadcasts", Verb: "write"}}},
		// The template is read and compiled under the key's permissions.
		"send_broadcast_to_contact": {class: ClassSend, pairs: []Pair{{Resource: "broadcasts", Verb: "write"}, {Resource: "templates", Verb: "read"}}},
		"schedule_broadcast":        {class: ClassSend, pairs: []Pair{{Resource: "broadcasts", Verb: "write"}}},
		"select_broadcast_winner":   {class: ClassSend, pairs: []Pair{{Resource: "broadcasts", Verb: "write"}}},
		"pause_broadcast":           {class: ClassWrite, pairs: []Pair{{Resource: "broadcasts", Verb: "write"}}},
		"resume_broadcast":          {class: ClassSend, pairs: []Pair{{Resource: "broadcasts", Verb: "write"}}},
		"cancel_broadcast":          {class: ClassDestructive, pairs: []Pair{{Resource: "broadcasts", Verb: "write"}}},
		"unschedule_broadcast":      {class: ClassWrite, pairs: []Pair{{Resource: "broadcasts", Verb: "write"}}},
		"delete_broadcast":          {class: ClassDestructive, pairs: []Pair{{Resource: "broadcasts", Verb: "write"}}},
		// The template is read under the key's permissions.
		"create_transactional_notification": {class: ClassWrite, pairs: []Pair{{Resource: "transactional", Verb: "write"}, {Resource: "templates", Verb: "read"}}},
		// + templates:read with email_template_id.
		"update_transactional_notification": {class: ClassWriteOpenWorld, pairs: []Pair{{Resource: "transactional", Verb: "write"}}},
		"delete_transactional_notification": {class: ClassDestructive, pairs: []Pair{{Resource: "transactional", Verb: "write"}}},
		// + contacts:read with cc or bcc.
		"send_transactional": {class: ClassSend, pairs: []Pair{{Resource: "transactional", Verb: "write"}}},
		// + lists:read with list_id, contacts:read with cc or bcc.
		"send_template_test": {class: ClassSend, pairs: []Pair{{Resource: "transactional", Verb: "write"}, {Resource: "templates", Verb: "read"}}},
		"search_messages":    {class: ClassRead, pairs: []Pair{{Resource: "message_history", Verb: "read"}}},
		// Per schema: query_analytics checks the pair of its schema argument,
		// and analytics.schemas lists only the schemas the key may read.
		"list_analytics_schemas": {class: ClassRead, pairs: nil},
		"query_analytics":        {class: ClassRead, pairs: nil},
		"list_automations":       {class: ClassRead, pairs: []Pair{{Resource: "automations", Verb: "read"}}},
		"get_automation":         {class: ClassRead, pairs: []Pair{{Resource: "automations", Verb: "read"}}},
		"get_automation_journey": {class: ClassRead, pairs: []Pair{{Resource: "automations", Verb: "read"}}},
		"get_automation_stats":   {class: ClassRead, pairs: []Pair{{Resource: "automations", Verb: "read"}}},
		// D10: it arms the emails of every later matching event.
		"activate_automation": {class: ClassSend, pairs: []Pair{{Resource: "automations", Verb: "write"}}},
		"pause_automation":    {class: ClassWrite, pairs: []Pair{{Resource: "automations", Verb: "write"}}},
		"delete_automation":   {class: ClassDestructive, pairs: []Pair{{Resource: "automations", Verb: "write"}}},
		"list_blog_posts":     {class: ClassRead, pairs: []Pair{{Resource: "blog", Verb: "read"}}},
		// The body also needs templates:read, which the tool checks itself: a
		// key without it reads the post with body null.
		"get_blog_post":        {class: ClassRead, pairs: []Pair{{Resource: "blog", Verb: "read"}}},
		"list_blog_categories": {class: ClassRead, pairs: []Pair{{Resource: "blog", Verb: "read"}}},
		// The body is a web template the tool creates (D26).
		"create_blog_post": {class: ClassWrite, pairs: []Pair{{Resource: "blog", Verb: "write"}, {Resource: "templates", Verb: "write"}}},
		// A patch: the post is read before it is written. + templates:write
		// and templates:read with body.
		"update_blog_post": {class: ClassWrite, pairs: []Pair{{Resource: "blog", Verb: "write"}, {Resource: "blog", Verb: "read"}}},
		// It sends no email (spec §6.2).
		"publish_blog_post":   {class: ClassWrite, pairs: []Pair{{Resource: "blog", Verb: "write"}}},
		"unpublish_blog_post": {class: ClassWrite, pairs: []Pair{{Resource: "blog", Verb: "write"}}},
		"delete_blog_post":    {class: ClassDestructive, pairs: []Pair{{Resource: "blog", Verb: "write"}}},
	}
	for _, tool := range s.catalogue() {
		expected, ok := want[tool.Name]
		require.True(t, ok, tool.Name)
		assert.Equal(t, expected.class, tool.Class, tool.Name)
		assert.Equal(t, expected.pairs, tool.Pairs, tool.Name)
		assert.Equal(t, expected.withoutWorkspace, tool.WithoutWorkspace, tool.Name)
		assert.Equal(t, "object", tool.InputSchema["type"], tool.Name)
		assert.Equal(t, false, tool.InputSchema["additionalProperties"], tool.Name)
		if !tool.WithoutWorkspace {
			assert.Contains(t, tool.InputSchema["required"], "workspace_id", tool.Name)
		}
	}
}

func TestAnnotations_PerClass(t *testing.T) {
	type hints struct {
		readOnly, destructive, openWorld, idempotent bool
	}
	want := map[Class]hints{
		ClassRead:           {readOnly: true, destructive: false, openWorld: false, idempotent: true},
		ClassWrite:          {readOnly: false, destructive: false, openWorld: false, idempotent: false},
		ClassWriteOpenWorld: {readOnly: false, destructive: false, openWorld: true, idempotent: false},
		ClassSend:           {readOnly: false, destructive: true, openWorld: true, idempotent: false},
		ClassDestructive:    {readOnly: false, destructive: true, openWorld: false, idempotent: true},
	}
	for class, expected := range want {
		annotations := (&Tool{Class: class}).Annotations()
		assert.Equal(t, expected.readOnly, annotations.ReadOnlyHint, class)
		require.NotNil(t, annotations.DestructiveHint, class)
		assert.Equal(t, expected.destructive, *annotations.DestructiveHint, class)
		require.NotNil(t, annotations.OpenWorldHint, class)
		assert.Equal(t, expected.openWorld, *annotations.OpenWorldHint, class)
		assert.Equal(t, expected.idempotent, annotations.IdempotentHint, class)
	}
	assert.Len(t, classAnnotations, 5, "a new class needs its annotations pinned above")
}

func TestAnnotations_ToolOverridesIdempotentHint(t *testing.T) {
	assert.True(t, (&Tool{Class: ClassSend, Idempotent: boolPtr(true)}).Annotations().IdempotentHint)
	assert.False(t, (&Tool{Class: ClassDestructive, Idempotent: boolPtr(false)}).Annotations().IdempotentHint)
}

func TestPrepare_RefusesASendOrDestructiveToolWithoutAWritePair(t *testing.T) {
	read := []Pair{{Resource: domain.PermissionResourceContacts, Verb: domain.PermissionTypeRead}}
	write := []Pair{{Resource: domain.PermissionResourceContacts, Verb: domain.PermissionTypeWrite}}

	for _, class := range []Class{ClassSend, ClassDestructive} {
		assert.Panics(t, func() { fakeTool("t", class, read, "contacts.delete", nil).prepare() }, class)
		assert.Panics(t, func() { fakeTool("t", class, nil, "contacts.delete", nil).prepare() }, class)
		assert.NotPanics(t, func() { fakeTool("t", class, write, "contacts.delete", nil).prepare() }, class)
	}
}

// Every catalogue tool passes the registration checks, so a send or
// destructive tool added without a write pair fails this test before it ships.
func TestCatalogue_EveryToolPassesRegistration(t *testing.T) {
	for _, tool := range newBareServer(Deps{}).catalogue() {
		assert.NotPanics(t, tool.prepare, tool.Name)
		if tool.Class == ClassSend || tool.Class == ClassDestructive {
			assert.True(t, tool.declaresWrite(), "%s must declare a write pair", tool.Name)
		}
	}
}

func TestPrepare_RefusesANonReadToolWithoutAnAuditAction(t *testing.T) {
	write := []Pair{{Resource: domain.PermissionResourceContacts, Verb: domain.PermissionTypeWrite}}
	assert.Panics(t, func() { fakeTool("t", ClassWrite, write, "", nil).prepare() })
	assert.Panics(t, func() { fakeTool("t", ClassWrite, write, "no.such.route", nil).prepare() })
	// Excluded from the API's audit and absent from MCPAuditedActions.
	assert.Panics(t, func() { fakeTool("t", ClassWriteOpenWorld, write, "llm.chat", nil).prepare() })
	assert.NotPanics(t, func() { fakeTool("t", ClassWrite, write, "contacts.delete", nil).prepare() })
	assert.NotPanics(t, func() { fakeTool("t", ClassRead, nil, "", nil).prepare() })
}

func TestPrepare_RefusesAWorkspaceToolWithoutARequiredWorkspaceID(t *testing.T) {
	tool := fakeTool("t", ClassRead, nil, "", nil)
	tool.InputSchema = objectSchema(map[string]any{"workspace_id": map[string]any{"type": "string"}})
	assert.Panics(t, tool.prepare)

	open := fakeTool("t", ClassRead, nil, "", nil)
	open.InputSchema = map[string]any{"type": "object", "properties": map[string]any{}}
	assert.Panics(t, open.prepare, "additionalProperties must be false")
}

func TestAuditSpec_UsesTheMCPTableForARouteTheAPIDoesNotAudit(t *testing.T) {
	_, ok := auditSpec("llm.chat")
	assert.False(t, ok, "an excluded route outside MCPAuditedActions has no spec")

	spec, ok := auditSpec("contacts.upsert")
	require.True(t, ok)
	assert.Equal(t, domain.AuditActionSpec{Category: domain.AuditCategoryContacts, TargetType: domain.AuditTargetContact}, spec)

	spec, ok = auditSpec("contacts.delete")
	require.True(t, ok)
	assert.Equal(t, domain.AuditedActions["contacts.delete"], spec)
}

func TestRegister_RefusesADuplicateName(t *testing.T) {
	s := newBareServer(Deps{})
	s.register(fakeTool("dup", ClassRead, nil, "", nil))
	assert.Panics(t, func() { s.register(fakeTool("dup", ClassRead, nil, "", nil)) })
}

func TestCall_WorkspaceRef(t *testing.T) {
	call := &Call{WorkspaceID: "ws1", Workspace: &domain.Workspace{ID: "ws1", Name: "Acme"}}
	assert.Equal(t, WorkspaceRef{ID: "ws1", Name: "Acme"}, call.WorkspaceRef())
}

// The descriptions state the output facts an assistant asked about in use,
// each read from the code that produces the field.
func TestCatalogue_DescriptionsDocumentTheirOutputs(t *testing.T) {
	descriptions := map[string]string{}
	for _, tool := range newBareServer(Deps{}).catalogue() {
		descriptions[tool.Name] = tool.Description
	}
	for tool, facts := range map[string][]string{
		"get_automation_stats": {
			"errors counts runs that completed but recorded an error",
			"failed_attempts counts every attempt that failed, retries included",
			"failed_journeys counts the journeys that ended failed on the step",
		},
		"get_broadcast":        {"get_broadcast_report has each variation's results"},
		"search_messages":      {"subject is set only when the send replaced the template's subject", "It is null for transactional emails, for automations without a list"},
		"query_analytics":      {"both bounds are included, a date as the end covers that whole day", "a date as its end stops at midnight"},
		"render_template":      {"subject_preview is absent when there is no preview text", "auto is generated from the HTML"},
		"activate_automation":  {"never returns to draft", "stays paused"},
		"get_contact_timeline": {"a custom event is dated by its occurred_at"},
		"track_custom_event":   {"dates the event at occurred_at"},
		"delete_blog_post":     {"The post's body template is not deleted"},
		"get_template":         {"left out unless include_translations is true"},
		"send_transactional":   {"already set to true"},
	} {
		for _, fact := range facts {
			assert.Contains(t, descriptions[tool], fact, tool)
		}
	}
	for name, description := range descriptions {
		assert.NotContains(t, description, "\u2014", name)
		assert.NotContains(t, description, "\u2013", name)
	}
}
