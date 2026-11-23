package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
)

func automationsReadPermission() domain.UserPermissions {
	return domain.UserPermissions{domain.PermissionResourceAutomations: {Read: true}}
}

var automationsEpoch = time.Date(2026, 10, 2, 9, 12, 0, 0, time.UTC)

// welcomeAutomation has a webhook step whose secret no output may carry.
func welcomeAutomation() *domain.Automation {
	sql := "NEW.kind = 'list.subscribed'"
	return &domain.Automation{
		ID: "a1", WorkspaceID: "ws-internal", Name: "Welcome series", Status: domain.AutomationStatusLive, ListID: "newsletter",
		Trigger: &domain.TimelineTriggerConfig{
			EventKind: "list.subscribed", ListID: strPtr("newsletter"), Frequency: domain.TriggerFrequencyOnce,
			Conditions: &domain.TreeNode{Kind: "branch", Branch: &domain.TreeNodeBranch{Operator: "and", Leaves: []*domain.TreeNode{}}},
		},
		TriggerSQL: &sql,
		RootNodeID: "n1",
		Nodes: []*domain.AutomationNode{
			{ID: "n1", AutomationID: "a1", Type: domain.NodeTypeTrigger, Config: map[string]any{}, NextNodeID: strPtr("n2"), Position: domain.NodePosition{X: 10, Y: 20}},
			{ID: "n2", AutomationID: "a1", Type: domain.NodeTypeEmail, Config: map[string]any{"template_id": "welcome-1"}, NextNodeID: strPtr("n3")},
			{ID: "n3", AutomationID: "a1", Type: domain.NodeTypeWebhook, Config: map[string]any{"url": "https://hooks.example.com/x", "secret": "hook-bearer-secret"}},
		},
		Stats:     &domain.AutomationStats{Enrolled: 120, Completed: 80, Exited: 5, Failed: 1},
		CreatedAt: automationsEpoch,
		UpdatedAt: automationsEpoch.Add(time.Hour),
	}
}

// threeAutomations are returned as automations.list does, created_at alone
// deciding the order: "b" and "c" share a creation time.
func threeAutomations() []*domain.Automation {
	return []*domain.Automation{
		{ID: "c", Name: "C", Status: domain.AutomationStatusDraft, CreatedAt: automationsEpoch.Add(time.Hour)},
		{ID: "b", Name: "B", Status: domain.AutomationStatusDraft, CreatedAt: automationsEpoch.Add(time.Hour)},
		{ID: "a", Name: "A", Status: domain.AutomationStatusDraft, CreatedAt: automationsEpoch},
	}
}

func TestAutomationTools_Definitions(t *testing.T) {
	automations := []Pair{{Resource: "automations", Verb: "read"}}
	assertReadTool(t, catalogueTool(t, "list_automations"), "list_automations", automations, []string{"workspace_id"})
	assertReadTool(t, catalogueTool(t, "get_automation"), "get_automation", automations, []string{"workspace_id", "automation_id"})
	assertReadTool(t, catalogueTool(t, "get_automation_journey"), "get_automation_journey", automations, []string{"workspace_id", "automation_id", "email"})
	assertReadTool(t, catalogueTool(t, "get_automation_stats"), "get_automation_stats", automations, []string{"workspace_id", "automation_id"})
}

func TestListAutomations_Projection(t *testing.T) {
	f := newFixture(t)
	f.reachable(automationsReadPermission())
	f.automations.EXPECT().List(gomock.Any(), testWorkspaceID, domain.AutomationFilter{}).Return([]*domain.Automation{welcomeAutomation()}, 1, nil)
	f.automations.EXPECT().CountJourneys(gomock.Any(), testWorkspaceID, []string{"a1"}).
		Return(map[string]domain.JourneyCounts{"a1": {Active: 34, Completed: 80, Exited: 5, Failed: 1, Total: 120}}, nil)

	result := f.callTool("list_automations", map[string]any{"workspace_id": testWorkspaceID})
	require.False(t, result.IsError, string(result.StructuredContent))
	assert.JSONEq(t, `{
		"workspace":{"id":"ws1","name":"Acme"},
		"automations":[{
			"id":"a1","name":"Welcome series","status":"live","list_id":"newsletter","exit_on_reply":false,
			"trigger":{"event_kind":"list.subscribed","list_id":"newsletter","segment_id":null,"custom_event_name":null,"frequency":"once"},
			"node_count":3,"sends_email":true,
			"stats":{"enrolled":120,"completed":80,"exited":5,"failed":1},
			"journeys":{"active":34,"completed":80,"exited":5,"failed":1,"total":120},
			"created_at":"2026-10-02T09:12:00Z","updated_at":"2026-10-02T10:12:00Z"
		}],
		"total":1
	}`, string(result.StructuredContent))
	for _, dropped := range []string{"hook-bearer-secret", "trigger_sql", "NEW.kind", "ws-internal", "nodes", "conditions"} {
		assert.NotContains(t, result.Content[0].Text, dropped)
	}
}

func TestListAutomations_PassesTheFilters(t *testing.T) {
	f := newFixture(t)
	f.reachable(automationsReadPermission())
	f.automations.EXPECT().List(gomock.Any(), testWorkspaceID, domain.AutomationFilter{
		Status: []domain.AutomationStatus{domain.AutomationStatusDraft, domain.AutomationStatusLive}, ListID: "newsletter",
	}).Return(nil, 0, nil)

	result := f.callTool("list_automations", map[string]any{"workspace_id": testWorkspaceID, "status": []string{"live", "draft", "live"}, "list_id": "newsletter"})
	require.False(t, result.IsError, string(result.StructuredContent))
	assert.JSONEq(t, `{"workspace":{"id":"ws1","name":"Acme"},"automations":[],"total":0}`, string(result.StructuredContent))
}

// automations.list orders by created_at alone; the tool reads every
// automation and pages them by offset, newest first with the ID breaking
// ties. The journeys are counted for the page only.
func TestListAutomations_PagesOnAStableOrder(t *testing.T) {
	f := newFixture(t)
	f.reachable(automationsReadPermission())
	f.automations.EXPECT().List(gomock.Any(), testWorkspaceID, gomock.Any()).
		DoAndReturn(func(context.Context, string, domain.AutomationFilter) ([]*domain.Automation, int, error) {
			gone := &domain.Automation{ID: "gone", CreatedAt: automationsEpoch.Add(2 * time.Hour), DeletedAt: &automationsEpoch}
			return append(threeAutomations(), gone, nil), 5, nil
		}).Times(2)
	f.automations.EXPECT().CountJourneys(gomock.Any(), testWorkspaceID, []string{"b", "c"}).Return(map[string]domain.JourneyCounts{}, nil)
	f.automations.EXPECT().CountJourneys(gomock.Any(), testWorkspaceID, []string{"a"}).Return(map[string]domain.JourneyCounts{}, nil)

	type page struct {
		Automations []struct {
			ID string `json:"id"`
		} `json:"automations"`
		Total      int    `json:"total"`
		NextCursor string `json:"next_cursor"`
	}
	var first page
	f.callTool("list_automations", map[string]any{"workspace_id": testWorkspaceID, "status": []string{"draft"}, "limit": 2}).decode(t, &first)
	require.Len(t, first.Automations, 2)
	assert.Equal(t, "b", first.Automations[0].ID)
	assert.Equal(t, "c", first.Automations[1].ID)
	assert.Equal(t, 3, first.Total, "a deleted automation is not counted")
	require.NotEmpty(t, first.NextCursor)

	var second page
	f.callTool("list_automations", map[string]any{"workspace_id": testWorkspaceID, "status": []string{"draft"}, "limit": 2, "cursor": first.NextCursor}).decode(t, &second)
	require.Len(t, second.Automations, 1)
	assert.Equal(t, "a", second.Automations[0].ID)
	assert.Empty(t, second.NextCursor)

	toolErr := f.callTool("list_automations", map[string]any{"workspace_id": testWorkspaceID, "status": []string{"live"}, "cursor": first.NextCursor}).errorOf(t)
	assert.Equal(t, CodeValidation, toolErr.Code, "a cursor is bound to its filters")
}

func TestListAutomations_RefusesBadArgumentsBeforeTheService(t *testing.T) {
	f := newFixture(t)
	f.reachable(automationsReadPermission())
	negative, err := EncodeCursor("list_automations", testWorkspaceID, map[string]any{}, -1)
	require.NoError(t, err)
	otherTool, err := EncodeCursor("list_templates", testWorkspaceID, map[string]any{}, 2)
	require.NoError(t, err)
	for name, args := range map[string]map[string]any{
		"unknown status":     {"status": []string{"archived"}},
		"status not a list":  {"status": "live"},
		"limit above 50":     {"limit": 51},
		"garbage cursor":     {"cursor": "%%%"},
		"negative offset":    {"cursor": negative},
		"another tool's one": {"cursor": otherTool},
	} {
		t.Run(name, func(t *testing.T) {
			args["workspace_id"] = testWorkspaceID
			assert.Equal(t, CodeValidation, f.callTool("list_automations", args).errorOf(t).Code)
		})
	}
}

func TestListAutomations_ErrorsAreMapped(t *testing.T) {
	f := newFixture(t)
	f.reachable(automationsReadPermission())
	f.automations.EXPECT().List(gomock.Any(), testWorkspaceID, gomock.Any()).Return([]*domain.Automation{welcomeAutomation()}, 1, nil)
	f.automations.EXPECT().CountJourneys(gomock.Any(), testWorkspaceID, gomock.Any()).Return(nil, errors.New("pq: timeout on db-host-7"))
	assert.Equal(t, ToolError{Code: CodeInternal, Message: "internal error"}, f.callTool("list_automations", map[string]any{"workspace_id": testWorkspaceID}).errorOf(t))

	g := newFixture(t)
	g.reachable(domain.UserPermissions{domain.PermissionResourceAutomations: {Write: true}})
	assert.Equal(t, ToolError{Code: CodePermissionDenied, Message: "this API key lacks the automations read permission", Resource: "automations", Permission: "read"},
		g.callTool("list_automations", map[string]any{"workspace_id": testWorkspaceID}).errorOf(t))
}

func TestGetAutomation_Projection(t *testing.T) {
	f := newFixture(t)
	f.reachable(automationsReadPermission())
	stored := welcomeAutomation()
	f.automations.EXPECT().Get(gomock.Any(), testWorkspaceID, "a1").Return(stored, nil)

	result := f.callTool("get_automation", map[string]any{"workspace_id": testWorkspaceID, "automation_id": "a1"})
	require.False(t, result.IsError, string(result.StructuredContent))
	assert.JSONEq(t, `{
		"workspace":{"id":"ws1","name":"Acme"},
		"automation":{
			"id":"a1","name":"Welcome series","status":"live","list_id":"newsletter","exit_on_reply":false,
			"trigger":{
				"event_kind":"list.subscribed","list_id":"newsletter","segment_id":null,"custom_event_name":null,
				"updated_fields":[],"link_url":null,"broadcast_id":null,"template_id":null,
				"conditions":{"kind":"branch","branch":{"operator":"and","leaves":[]}},
				"frequency":"once"
			},
			"root_node_id":"n1",
			"nodes":[
				{"id":"n1","type":"trigger","config":{},"next_node_id":"n2"},
				{"id":"n2","type":"email","config":{"template_id":"welcome-1"},"next_node_id":"n3"},
				{"id":"n3","type":"webhook","config":{"url":"https://hooks.example.com/x"},"next_node_id":null}
			],
			"stats":{"enrolled":120,"completed":80,"exited":5,"failed":1},
			"created_at":"2026-10-02T09:12:00Z","updated_at":"2026-10-02T10:12:00Z"
		}
	}`, string(result.StructuredContent))
	for _, dropped := range []string{"hook-bearer-secret", "secret", "trigger_sql", "ws-internal", "position", "automation_id"} {
		assert.NotContains(t, result.Content[0].Text, dropped)
	}
	assert.Equal(t, "hook-bearer-secret", stored.Nodes[2].Config["secret"], "the service's object is left as it was")
}

// The service keeps the secret for a key that may write automations; the tool
// drops it all the same.
func TestGetAutomation_DropsTheSecretForAWriter(t *testing.T) {
	f := newFixture(t)
	f.reachable(domain.UserPermissions{domain.PermissionResourceAutomations: {Read: true, Write: true}})
	f.automations.EXPECT().Get(gomock.Any(), testWorkspaceID, "a1").Return(welcomeAutomation(), nil)

	result := f.callTool("get_automation", map[string]any{"workspace_id": testWorkspaceID, "automation_id": "a1"})
	require.False(t, result.IsError, string(result.StructuredContent))
	assert.NotContains(t, result.Content[0].Text, "hook-bearer-secret")
	assert.NotContains(t, string(result.StructuredContent), "hook-bearer-secret")
}

func TestGetAutomation_NotFound(t *testing.T) {
	t.Run("unknown", func(t *testing.T) {
		f := newFixture(t)
		f.reachable(automationsReadPermission())
		f.automations.EXPECT().Get(gomock.Any(), testWorkspaceID, "nope").
			Return(nil, fmt.Errorf("failed to get automation: %w", &domain.ErrNotFound{Entity: "automation", ID: "nope"}))
		toolErr := f.callTool("get_automation", map[string]any{"workspace_id": testWorkspaceID, "automation_id": "nope"}).errorOf(t)
		assert.Equal(t, CodeNotFound, toolErr.Code)
	})
	t.Run("deleted", func(t *testing.T) {
		f := newFixture(t)
		f.reachable(automationsReadPermission())
		deleted := welcomeAutomation()
		deleted.DeletedAt = &automationsEpoch
		f.automations.EXPECT().Get(gomock.Any(), testWorkspaceID, "a1").Return(deleted, nil)
		toolErr := f.callTool("get_automation", map[string]any{"workspace_id": testWorkspaceID, "automation_id": "a1"}).errorOf(t)
		assert.Equal(t, CodeNotFound, toolErr.Code)
	})
	t.Run("empty ID", func(t *testing.T) {
		f := newFixture(t)
		f.reachable(automationsReadPermission())
		assert.Equal(t, CodeValidation, f.callTool("get_automation", map[string]any{"workspace_id": testWorkspaceID, "automation_id": ""}).errorOf(t).Code)
	})
}

func journeyFixture() (*domain.ContactAutomation, []*domain.NodeExecution) {
	entered := time.Date(2026, 11, 21, 8, 0, 0, 0, time.UTC)
	scheduled := entered.Add(24 * time.Hour)
	completed := entered.Add(time.Second)
	duration := int64(4)
	journey := &domain.ContactAutomation{
		ID: "ca-internal", AutomationID: "a1", ContactEmail: "jane@example.com", CurrentNodeID: strPtr("n3"),
		Status: domain.ContactAutomationStatusActive, EnteredAt: entered, ScheduledAt: &scheduled,
		Context: map[string]any{
			"trigger":  map[string]any{"kind": "list.subscribed", "entity_id": "newsletter"},
			"ab_tests": map[string]any{"bookkeeping": "internal"},
		},
		MaxRetries: 3,
	}
	steps := []*domain.NodeExecution{
		{ID: "ex-internal", ContactAutomationID: "ca-internal", NodeID: "n2", NodeType: domain.NodeTypeDelay, Action: domain.NodeActionCompleted,
			EnteredAt: completed, CompletedAt: &completed, DurationMs: &duration, Output: map[string]any{"node_type": "delay", "duration": float64(1), "unit": "days"}},
	}
	return journey, steps
}

func TestGetAutomationJourney_Projection(t *testing.T) {
	f := newFixture(t)
	f.reachable(automationsReadPermission())
	journey, steps := journeyFixture()
	// Matched verbatim, so the email is normalized first.
	f.automations.EXPECT().GetContactNodeExecutions(gomock.Any(), testWorkspaceID, "a1", "jane@example.com").Return(journey, steps, nil)

	result := f.callTool("get_automation_journey", map[string]any{"workspace_id": testWorkspaceID, "automation_id": "a1", "email": " Jane@Example.com "})
	require.False(t, result.IsError, string(result.StructuredContent))
	assert.JSONEq(t, `{
		"workspace":{"id":"ws1","name":"Acme"},
		"automation_id":"a1",
		"journey":{
			"contact_email":"jane@example.com","status":"active","current_node_id":"n3","exit_reason":null,
			"entered_at":"2026-11-21T08:00:00Z","scheduled_at":"2026-11-22T08:00:00Z",
			"retry_count":0,"max_retries":3,"last_error":null,"last_retry_at":null,
			"trigger_event":{"kind":"list.subscribed","entity_id":"newsletter"}
		},
		"steps":[{
			"node_id":"n2","node_type":"delay","action":"completed",
			"entered_at":"2026-11-21T08:00:01Z","completed_at":"2026-11-21T08:00:01Z","duration_ms":4,
			"output":{"node_type":"delay","duration":1,"unit":"days"},"error":null
		}],
		"steps_truncated":false
	}`, string(result.StructuredContent))
	for _, dropped := range []string{"ca-internal", "ex-internal", "bookkeeping", "contact_automation_id"} {
		assert.NotContains(t, result.Content[0].Text, dropped)
	}
}

// A webhook step's error and output quote the third party's response.
func TestGetAutomationJourney_CapsWebhookText(t *testing.T) {
	f := newFixture(t)
	f.reachable(automationsReadPermission())
	journey, steps := journeyFixture()
	journey.LastError = strPtr("webhook returned 500: " + strings.Repeat("e", 3000))
	journey.Context["trigger"] = map[string]any{"properties": strings.Repeat("p", 2500)}
	steps[0].NodeType = domain.NodeTypeWebhook
	steps[0].Error = strPtr("upstream said \x1b[31m" + strings.Repeat("x", 2100))
	steps[0].Output = map[string]any{"raw": strings.Repeat("r", 5000)}
	f.automations.EXPECT().GetContactNodeExecutions(gomock.Any(), testWorkspaceID, "a1", "jane@example.com").Return(journey, steps, nil)

	var out struct {
		Journey struct {
			LastError       string          `json:"last_error"`
			TriggerEvent    json.RawMessage `json:"trigger_event"`
			Truncated       bool            `json:"truncated"`
			TruncatedFields []string        `json:"truncated_fields"`
		} `json:"journey"`
		Steps []struct {
			Error           string          `json:"error"`
			Output          json.RawMessage `json:"output"`
			Truncated       bool            `json:"truncated"`
			TruncatedFields []string        `json:"truncated_fields"`
		} `json:"steps"`
	}
	f.callTool("get_automation_journey", map[string]any{"workspace_id": testWorkspaceID, "automation_id": "a1", "email": "jane@example.com"}).decode(t, &out)
	assert.Len(t, []rune(out.Journey.LastError), FreeTextLimit)
	assert.Equal(t, "null", string(out.Journey.TriggerEvent))
	assert.True(t, out.Journey.Truncated)
	assert.Equal(t, []string{"last_error", "trigger_event"}, out.Journey.TruncatedFields)
	require.Len(t, out.Steps, 1)
	assert.Len(t, []rune(out.Steps[0].Error), FreeTextLimit)
	assert.NotContains(t, out.Steps[0].Error, "\x1b")
	assert.Equal(t, "null", string(out.Steps[0].Output))
	assert.True(t, out.Steps[0].Truncated)
	assert.Equal(t, []string{"output", "error"}, out.Steps[0].TruncatedFields)
}

func TestGetAutomationJourney_KeepsTheLatestSteps(t *testing.T) {
	f := newFixture(t)
	f.reachable(automationsReadPermission())
	journey, _ := journeyFixture()
	steps := make([]*domain.NodeExecution, 0, maxJourneySteps+5)
	for i := range maxJourneySteps + 5 {
		steps = append(steps, &domain.NodeExecution{NodeID: fmt.Sprintf("n%d", i), NodeType: domain.NodeTypeDelay, Action: domain.NodeActionCompleted})
	}
	f.automations.EXPECT().GetContactNodeExecutions(gomock.Any(), testWorkspaceID, "a1", "jane@example.com").Return(journey, steps, nil)

	var out struct {
		Steps []struct {
			NodeID string `json:"node_id"`
		} `json:"steps"`
		StepsTruncated bool `json:"steps_truncated"`
	}
	f.callTool("get_automation_journey", map[string]any{"workspace_id": testWorkspaceID, "automation_id": "a1", "email": "jane@example.com"}).decode(t, &out)
	require.Len(t, out.Steps, maxJourneySteps)
	assert.Equal(t, "n5", out.Steps[0].NodeID)
	assert.Equal(t, fmt.Sprintf("n%d", maxJourneySteps+4), out.Steps[maxJourneySteps-1].NodeID)
	assert.True(t, out.StepsTruncated)
}

func TestGetAutomationJourney_NotFound(t *testing.T) {
	f := newFixture(t)
	f.reachable(automationsReadPermission())
	f.automations.EXPECT().GetContactNodeExecutions(gomock.Any(), testWorkspaceID, "a1", "ghost@example.com").
		Return(nil, nil, fmt.Errorf("failed to get contact automation: %w", &domain.ErrNotFound{Entity: "contact_automation", ID: "ghost@example.com"}))
	f.automations.EXPECT().GetContactNodeExecutions(gomock.Any(), testWorkspaceID, "gone", "jane@example.com").
		Return(nil, nil, fmt.Errorf("failed to get automation: %w", &domain.ErrNotFound{Entity: "automation", ID: "gone"}))

	assert.Equal(t, ToolError{Code: CodeNotFound, Message: "this contact never entered this automation"},
		f.callTool("get_automation_journey", map[string]any{"workspace_id": testWorkspaceID, "automation_id": "a1", "email": "ghost@example.com"}).errorOf(t))
	toolErr := f.callTool("get_automation_journey", map[string]any{"workspace_id": testWorkspaceID, "automation_id": "gone", "email": "jane@example.com"}).errorOf(t)
	assert.Equal(t, CodeNotFound, toolErr.Code, "an unknown or deleted automation")
	assert.Contains(t, toolErr.Message, "automation")
}

func TestGetAutomationJourney_RefusesBadArguments(t *testing.T) {
	f := newFixture(t)
	f.reachable(automationsReadPermission())
	for name, args := range map[string]map[string]any{
		"blank email":      {"automation_id": "a1", "email": "  "},
		"no email":         {"automation_id": "a1"},
		"empty automation": {"automation_id": "", "email": "jane@example.com"},
		"unknown argument": {"automation_id": "a1", "email": "jane@example.com", "limit": 5},
	} {
		t.Run(name, func(t *testing.T) {
			args["workspace_id"] = testWorkspaceID
			assert.Equal(t, CodeValidation, f.callTool("get_automation_journey", args).errorOf(t).Code)
		})
	}
}

func TestGetAutomationStats(t *testing.T) {
	f := newFixture(t)
	f.reachable(automationsReadPermission())
	email := domain.NewNodeStats(domain.NodeTypeEmail)
	email.Visits, email.Completed = 10, 9
	email.Email = &domain.NodeEmailStats{Sent: 9, Delivered: 8, Opened: 4}
	branch := domain.NewNodeStats(domain.NodeTypeFilter)
	branch.Branches = map[string]int64{"yes": 6, "no": 4}
	f.automations.EXPECT().GetNodeStats(gomock.Any(), testWorkspaceID, "a1", (*time.Time)(nil), (*time.Time)(nil)).
		Return(&domain.NodeStatsResponse{AutomationID: "a1", Nodes: map[string]*domain.NodeStats{"n2": email, "n3": branch}}, nil)

	result := f.callTool("get_automation_stats", map[string]any{"workspace_id": testWorkspaceID, "automation_id": "a1"})
	require.False(t, result.IsError, string(result.StructuredContent))
	assert.JSONEq(t, `{
		"workspace":{"id":"ws1","name":"Acme"},
		"automation_id":"a1",
		"nodes":{
			"n2":{"node_type":"email","visits":10,"completed":9,"failed_attempts":0,"skipped":0,"skip_reasons":{},"errors":0,
				"failed_journeys":0,"in_progress":0,"waiting":0,"retrying":0,"branches":{},
				"email":{"executions":0,"queued":0,"not_sent":0,"sent":9,"delivered":8,"opened":4,"opens_tracked":0,"clicked":0,
					"clicks_tracked":0,"failed":0,"bounced":0,"complained":0,"unsubscribed":0}},
			"n3":{"node_type":"filter","visits":0,"completed":0,"failed_attempts":0,"skipped":0,"skip_reasons":{},"errors":0,
				"failed_journeys":0,"in_progress":0,"waiting":0,"retrying":0,"branches":{"yes":6,"no":4}}
		}
	}`, string(result.StructuredContent))
}

func TestGetAutomationStats_Period(t *testing.T) {
	f := newFixture(t)
	f.reachable(automationsReadPermission())
	since := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	until := time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC)
	f.automations.EXPECT().GetNodeStats(gomock.Any(), testWorkspaceID, "a1", &since, &until).
		Return(domain.NewNodeStatsResponse("a1", &since, &until), nil)

	var out map[string]json.RawMessage
	f.callTool("get_automation_stats", map[string]any{
		"workspace_id": testWorkspaceID, "automation_id": "a1", "since": "2026-11-01T01:00:00+01:00", "until": "2026-11-08T00:00:00Z",
	}).decode(t, &out)
	assert.Equal(t, `"2026-11-01T00:00:00Z"`, string(out["since"]))
	assert.Equal(t, `"2026-11-08T00:00:00Z"`, string(out["until"]))
	assert.Equal(t, `{}`, string(out["nodes"]))

	for name, args := range map[string]map[string]any{
		"since alone":    {"since": "2026-11-01T00:00:00Z"},
		"until alone":    {"until": "2026-11-01T00:00:00Z"},
		"not RFC 3339":   {"since": "yesterday", "until": "2026-11-01T00:00:00Z"},
		"since > until":  {"since": "2026-11-09T00:00:00Z", "until": "2026-11-01T00:00:00Z"},
		"no automation":  {"automation_id": ""},
		"unknown option": {"group_by": "day"},
	} {
		t.Run(name, func(t *testing.T) {
			args["workspace_id"] = testWorkspaceID
			if _, ok := args["automation_id"]; !ok {
				args["automation_id"] = "a1"
			}
			assert.Equal(t, CodeValidation, f.callTool("get_automation_stats", args).errorOf(t).Code)
		})
	}
}

func TestGetAutomationStats_NotFound(t *testing.T) {
	f := newFixture(t)
	f.reachable(automationsReadPermission())
	f.automations.EXPECT().GetNodeStats(gomock.Any(), testWorkspaceID, "gone", gomock.Any(), gomock.Any()).
		Return(nil, fmt.Errorf("failed to get automation: %w", &domain.ErrNotFound{Entity: "automation", ID: "gone"}))
	toolErr := f.callTool("get_automation_stats", map[string]any{"workspace_id": testWorkspaceID, "automation_id": "gone"}).errorOf(t)
	assert.Equal(t, CodeNotFound, toolErr.Code)
}

// An automation's name and its trigger's custom event name and link URL are
// free text, capped like the rest.
func TestAutomationTools_CapTheNameAndTriggerText(t *testing.T) {
	long, capped := strings.Repeat("n", FreeTextLimit+1), strings.Repeat("n", FreeTextLimit)
	longAutomation := func() *domain.Automation {
		automation := welcomeAutomation()
		automation.Name = long
		automation.Trigger.CustomEventName = strPtr(long)
		automation.Trigger.LinkURL = strPtr(long)
		return automation
	}

	t.Run("list_automations", func(t *testing.T) {
		f := newFixture(t)
		f.reachable(automationsReadPermission())
		f.automations.EXPECT().List(gomock.Any(), testWorkspaceID, gomock.Any()).Return([]*domain.Automation{longAutomation()}, 1, nil)
		f.automations.EXPECT().CountJourneys(gomock.Any(), testWorkspaceID, []string{"a1"}).Return(map[string]domain.JourneyCounts{}, nil)
		var out struct {
			Automations []struct {
				Name    string `json:"name"`
				Trigger struct {
					CustomEventName string `json:"custom_event_name"`
				} `json:"trigger"`
				TruncatedFields []string `json:"truncated_fields"`
			} `json:"automations"`
		}
		f.callTool("list_automations", map[string]any{"workspace_id": testWorkspaceID}).decode(t, &out)
		require.Len(t, out.Automations, 1)
		assert.Equal(t, capped, out.Automations[0].Name)
		assert.Equal(t, capped, out.Automations[0].Trigger.CustomEventName)
		assert.Equal(t, []string{"name", "trigger.custom_event_name"}, out.Automations[0].TruncatedFields)
	})

	t.Run("get_automation", func(t *testing.T) {
		f := newFixture(t)
		f.reachable(automationsReadPermission())
		f.automations.EXPECT().Get(gomock.Any(), testWorkspaceID, "a1").Return(longAutomation(), nil)
		var out struct {
			Automation struct {
				Name    string `json:"name"`
				Trigger struct {
					CustomEventName string `json:"custom_event_name"`
					LinkURL         string `json:"link_url"`
				} `json:"trigger"`
				TruncatedFields []string `json:"truncated_fields"`
			} `json:"automation"`
		}
		f.callTool("get_automation", map[string]any{"workspace_id": testWorkspaceID, "automation_id": "a1"}).decode(t, &out)
		assert.Equal(t, capped, out.Automation.Name)
		assert.Equal(t, capped, out.Automation.Trigger.CustomEventName)
		assert.Equal(t, capped, out.Automation.Trigger.LinkURL)
		assert.Equal(t, []string{"name", "trigger.custom_event_name", "trigger.link_url"}, out.Automation.TruncatedFields)
	})

	t.Run("get_automation_journey", func(t *testing.T) {
		f := newFixture(t)
		f.reachable(automationsReadPermission())
		journey, steps := journeyFixture()
		journey.ContactEmail = "jane@example.com\x1b"
		journey.ExitReason = strPtr(long)
		f.automations.EXPECT().GetContactNodeExecutions(gomock.Any(), testWorkspaceID, "a1", "jane@example.com").Return(journey, steps, nil)
		var out struct {
			Journey struct {
				ContactEmail    string   `json:"contact_email"`
				ExitReason      string   `json:"exit_reason"`
				TruncatedFields []string `json:"truncated_fields"`
			} `json:"journey"`
		}
		f.callTool("get_automation_journey", map[string]any{"workspace_id": testWorkspaceID, "automation_id": "a1", "email": "jane@example.com"}).decode(t, &out)
		assert.Equal(t, "jane@example.com", out.Journey.ContactEmail, "control characters are removed")
		assert.Equal(t, capped, out.Journey.ExitReason)
		assert.Equal(t, []string{"exit_reason"}, out.Journey.TruncatedFields)
	})
}
