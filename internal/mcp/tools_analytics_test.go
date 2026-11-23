package mcp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/analytics"
)

// testSchemas are what analytics.schemas answers, SQL fragments included.
func testSchemas() map[string]analytics.SchemaDefinition {
	return map[string]analytics.SchemaDefinition{
		"message_history": {
			Name: "message_history",
			Measures: map[string]analytics.MeasureDefinition{
				"count_opened": {Type: "count", Title: "Opened", SQL: "secret_measure_sql", Description: "Opened messages",
					Filters: []analytics.MeasureFilter{{SQL: "secret_filter_sql"}}},
				"count": {Type: "count", Title: "Total", SQL: "COUNT(*)", Description: "All messages"},
			},
			Dimensions: map[string]analytics.DimensionDefinition{
				"sent_at":       {Type: "time", Title: "Sent At", SQL: "secret_dimension_sql", Description: "When it was sent"},
				"contact_email": {Type: "string", Title: "Contact", SQL: "contact_email", Description: "Recipient"},
			},
		},
		"webhook_deliveries": {
			Name:     "webhook_deliveries",
			Measures: map[string]analytics.MeasureDefinition{"count": {Type: "count", Title: "Deliveries"}},
		},
	}
}

func analyticsArgs(extra map[string]any) map[string]any {
	args := map[string]any{"workspace_id": testWorkspaceID, "schema": "message_history", "measures": []string{"count"}}
	for key, value := range extra {
		args[key] = value
	}
	return args
}

func TestAnalyticsTools_Definitions(t *testing.T) {
	for name, required := range map[string][]string{
		"list_analytics_schemas": {"workspace_id"},
		"query_analytics":        {"workspace_id", "schema", "measures"},
	} {
		tool := catalogueTool(t, name)
		assert.Equal(t, name, tool.Name)
		assert.Equal(t, ClassRead, tool.Class)
		assert.Empty(t, tool.Pairs, "the pair depends on the schema")
		assert.Empty(t, tool.AuditAction)
		assert.NotEmpty(t, tool.Title)
		assert.NotEmpty(t, tool.Description)
		listed := tool.sdkTool()
		assert.True(t, listed.Annotations.ReadOnlyHint)
		assert.False(t, *listed.Annotations.DestructiveHint)
		assert.False(t, *listed.Annotations.OpenWorldHint)
		assert.True(t, listed.Annotations.IdempotentHint)
		assert.Equal(t, required, tool.InputSchema["required"])
		assert.Equal(t, false, tool.InputSchema["additionalProperties"])
		assert.NotPanics(t, tool.prepare)
	}
	assert.Nil(t, catalogueTool(t, "list_analytics_schemas").ArgumentPairs, "analytics.schemas filters by permission itself")
	require.NotNil(t, catalogueTool(t, "query_analytics").ArgumentPairs)
}

// The pair of a query is the read pair of its schema's resource; a schema MCP
// leaves out, or one without a resource, adds none and is refused as invalid.
func TestQueryAnalytics_ArgumentPairs(t *testing.T) {
	pairs := catalogueTool(t, "query_analytics").ArgumentPairs
	for schema, want := range map[string][]Pair{
		"message_history":            {{Resource: "message_history", Verb: "read"}},
		"email_queue":                {{Resource: "message_history", Verb: "read"}},
		"contacts":                   {{Resource: "contacts", Verb: "read"}},
		"broadcasts":                 {{Resource: "broadcasts", Verb: "read"}},
		"automation_node_executions": {{Resource: "automations", Verb: "read"}},
		"web_sessions":               {{Resource: "web_analytics", Verb: "read"}},
		"webhook_deliveries":         nil,
		"no_such_schema":             nil,
	} {
		assert.Equal(t, want, pairs(map[string]any{"schema": schema}), schema)
	}
}

func TestListAnalyticsSchemas_SortedWithoutSQLOrWebhookDeliveries(t *testing.T) {
	f := newFixture(t)
	f.reachable(domain.UserPermissions{domain.PermissionResourceMessageHistory: {Read: true}})
	f.analytics.EXPECT().GetSchemas(gomock.Any(), testWorkspaceID).Return(testSchemas(), nil)

	result := f.callTool("list_analytics_schemas", map[string]any{"workspace_id": testWorkspaceID})
	require.False(t, result.IsError, string(result.StructuredContent))
	assert.JSONEq(t, `{
		"workspace":{"id":"ws1","name":"Acme"},
		"schemas":[{
			"name":"message_history",
			"measures":[
				{"name":"count","type":"count","title":"Total","description":"All messages"},
				{"name":"count_opened","type":"count","title":"Opened","description":"Opened messages"}
			],
			"dimensions":[
				{"name":"contact_email","type":"string","title":"Contact","description":"Recipient"},
				{"name":"sent_at","type":"time","title":"Sent At","description":"When it was sent"}
			]
		}]
	}`, string(result.StructuredContent))
	for _, dropped := range []string{"secret_", "webhook_deliveries", "COUNT(*)", `"sql"`} {
		assert.NotContains(t, result.Content[0].Text, dropped)
	}
}

func TestListAnalyticsSchemas_AKeyWithoutAnyResourceGetsNone(t *testing.T) {
	f := newFixture(t)
	f.reachable(domain.UserPermissions{domain.PermissionResourceLists: {Read: true}})
	f.analytics.EXPECT().GetSchemas(gomock.Any(), testWorkspaceID).Return(map[string]analytics.SchemaDefinition{}, nil)

	result := f.callTool("list_analytics_schemas", map[string]any{"workspace_id": testWorkspaceID})
	require.False(t, result.IsError, string(result.StructuredContent))
	assert.JSONEq(t, `{"workspace":{"id":"ws1","name":"Acme"},"schemas":[]}`, string(result.StructuredContent))
}

func TestQueryAnalytics_SendsTheQueryAndDropsTheSQL(t *testing.T) {
	f := newFixture(t)
	f.reachable(domain.UserPermissions{domain.PermissionResourceMessageHistory: {Read: true}})
	limit, offset, timezone := 10, 20, "Europe/Paris"
	// One row past the limit is asked for, to tell a complete page from a cut one.
	queried := limit + 1
	dateRange := [2]string{"2026-11-01", "2026-11-30"}
	want := analytics.Query{
		Schema: "message_history", Measures: []string{"count", "count_opened"}, Dimensions: []string{"contact_email"},
		Timezone:       &timezone,
		TimeDimensions: []analytics.TimeDimension{{Dimension: "sent_at", Granularity: "day", DateRange: &dateRange}},
		Filters:        []analytics.Filter{{Member: "contact_email", Operator: "endsWith", Values: []string{"@acme.com"}}},
		Having:         []analytics.Filter{{Member: "count", Operator: "gt", Values: []string{"1"}}},
		Order:          map[string]string{"count": "desc"},
		Limit:          &queried, Offset: &offset,
	}
	f.analytics.EXPECT().GetSchemas(gomock.Any(), testWorkspaceID).Return(testSchemas(), nil)
	f.analytics.EXPECT().Query(gomock.Any(), testWorkspaceID, want).
		DoAndReturn(func(ctx context.Context, _ string, _ analytics.Query) (*analytics.Response, error) {
			deadline, ok := ctx.Deadline()
			assert.True(t, ok, "the query runs under a deadline")
			assert.WithinDuration(t, time.Now().Add(analyticsTimeout), deadline, 2*time.Second)
			return &analytics.Response{
				Data: []map[string]any{{"contact_email": "ada@acme.com", "count": 3, "sent_at": "2026-11-20T00:00:00Z"}},
				Meta: analytics.Meta{Total: 1, ExecutionTime: time.Second, Query: "SELECT secret_generated_sql", Params: []any{"secret_param"}},
			}, nil
		})

	result := f.callTool("query_analytics", analyticsArgs(map[string]any{
		"measures": []string{"count", "count_opened"}, "dimensions": []string{"contact_email"}, "timezone": timezone,
		"time_dimensions": []map[string]any{{"dimension": "sent_at", "granularity": "day", "date_range": []string{"2026-11-01", "2026-11-30"}}},
		"filters":         []map[string]any{{"member": "contact_email", "operator": "endsWith", "values": []string{"@acme.com"}}},
		"having":          []map[string]any{{"member": "count", "operator": "gt", "values": []string{"1"}}},
		"order":           []map[string]any{{"member": "count", "direction": "desc"}},
		"limit":           limit, "offset": offset,
	}))
	require.False(t, result.IsError, string(result.StructuredContent))
	assert.JSONEq(t, `{
		"workspace":{"id":"ws1","name":"Acme"},
		"schema":"message_history",
		"rows":[{"contact_email":"ada@acme.com","count":3,"sent_at":"2026-11-20T00:00:00Z"}],
		"row_count":1,
		"limit_reached":false
	}`, string(result.StructuredContent))
	for _, dropped := range []string{"secret_generated_sql", "secret_param", "executionTime", "meta"} {
		assert.NotContains(t, result.Content[0].Text, dropped)
	}
}

func TestQueryAnalytics_DefaultLimitAndTheCap(t *testing.T) {
	f := newFixture(t)
	f.reachable(domain.UserPermissions{domain.PermissionResourceMessageHistory: {Read: true}})
	f.analytics.EXPECT().GetSchemas(gomock.Any(), testWorkspaceID).Return(testSchemas(), nil).Times(3)
	rows := func(n int) []map[string]any {
		out := []map[string]any{}
		for i := range n {
			out = append(out, map[string]any{"count": i})
		}
		return out
	}
	// Gap filling can answer more rows than the limit.
	f.analytics.EXPECT().Query(gomock.Any(), testWorkspaceID, gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, query analytics.Query) (*analytics.Response, error) {
			require.NotNil(t, query.Limit)
			assert.Equal(t, 101, *query.Limit)
			return &analytics.Response{Data: rows(120)}, nil
		})
	f.analytics.EXPECT().Query(gomock.Any(), testWorkspaceID, gomock.Any()).Return(&analytics.Response{Data: rows(999)}, nil)
	// The extra row asked for at the cap never reaches the client.
	f.analytics.EXPECT().Query(gomock.Any(), testWorkspaceID, gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, query analytics.Query) (*analytics.Response, error) {
			require.NotNil(t, query.Limit)
			assert.Equal(t, 1001, *query.Limit)
			return &analytics.Response{Data: rows(1001)}, nil
		})

	var out struct {
		Rows         []map[string]any `json:"rows"`
		RowCount     int              `json:"row_count"`
		LimitReached bool             `json:"limit_reached"`
	}
	f.callTool("query_analytics", analyticsArgs(nil)).decode(t, &out)
	assert.Len(t, out.Rows, 100)
	assert.Equal(t, 100, out.RowCount)
	assert.True(t, out.LimitReached)

	f.callTool("query_analytics", analyticsArgs(map[string]any{"limit": 1000})).decode(t, &out)
	assert.Equal(t, 999, out.RowCount)
	assert.False(t, out.LimitReached)

	f.callTool("query_analytics", analyticsArgs(map[string]any{"limit": 1000})).decode(t, &out)
	assert.Len(t, out.Rows, 1000)
	assert.Equal(t, 1000, out.RowCount)
	assert.True(t, out.LimitReached)
}

func TestQueryAnalytics_ACompletePageIsNotCut(t *testing.T) {
	f := newFixture(t)
	f.reachable(domain.UserPermissions{domain.PermissionResourceMessageHistory: {Read: true}})
	f.analytics.EXPECT().GetSchemas(gomock.Any(), testWorkspaceID).Return(testSchemas(), nil)
	f.analytics.EXPECT().Query(gomock.Any(), testWorkspaceID, gomock.Any()).
		Return(&analytics.Response{Data: []map[string]any{{"count": 1}, {"count": 2}, {"count": 3}}}, nil)

	var out struct {
		RowCount     int  `json:"row_count"`
		LimitReached bool `json:"limit_reached"`
	}
	f.callTool("query_analytics", analyticsArgs(map[string]any{"limit": 3})).decode(t, &out)
	assert.Equal(t, 3, out.RowCount)
	assert.False(t, out.LimitReached, "a page of exactly limit rows is complete")
}

func TestQueryAnalytics_CapsTextCells(t *testing.T) {
	f := newFixture(t)
	f.reachable(domain.UserPermissions{domain.PermissionResourceMessageHistory: {Read: true}})
	f.analytics.EXPECT().GetSchemas(gomock.Any(), testWorkspaceID).Return(testSchemas(), nil)
	f.analytics.EXPECT().Query(gomock.Any(), testWorkspaceID, gomock.Any()).Return(&analytics.Response{Data: []map[string]any{
		{"contact_email": "ok@acme.com"},
		{"contact_email": "a\x1b" + strings.Repeat("e", 2000)},
	}}, nil)

	var out struct {
		Rows            []map[string]string `json:"rows"`
		TruncatedFields []string            `json:"truncated_fields"`
	}
	f.callTool("query_analytics", analyticsArgs(nil)).decode(t, &out)
	require.Len(t, out.Rows, 2)
	assert.Equal(t, "a"+strings.Repeat("e", 1999), out.Rows[1]["contact_email"])
	assert.Equal(t, []string{"rows[1].contact_email"}, out.TruncatedFields)
}

func TestQueryAnalytics_RefusesBeforeTheQuery(t *testing.T) {
	f := newFixture(t)
	f.reachable(domain.NewFullPermissions())
	f.analytics.EXPECT().GetSchemas(gomock.Any(), testWorkspaceID).Return(testSchemas(), nil).AnyTimes()

	for name, args := range map[string]map[string]any{
		"the webhook_deliveries schema": analyticsArgs(map[string]any{"schema": "webhook_deliveries"}),
		"a schema without a resource":   analyticsArgs(map[string]any{"schema": "no_such_schema"}),
		"a schema the workspace lacks":  analyticsArgs(map[string]any{"schema": "contacts"}),
		"no measure":                    analyticsArgs(map[string]any{"measures": []string{}}),
		"an unknown measure":            analyticsArgs(map[string]any{"measures": []string{"count_everything"}}),
		"two order entries": analyticsArgs(map[string]any{"order": []map[string]any{
			{"member": "count", "direction": "desc"}, {"member": "count_opened", "direction": "asc"},
		}}),
		"a date range of one date": analyticsArgs(map[string]any{"time_dimensions": []map[string]any{
			{"dimension": "sent_at", "granularity": "day", "date_range": []string{"2026-11-01"}},
		}}),
		"an unknown granularity": analyticsArgs(map[string]any{"time_dimensions": []map[string]any{{"dimension": "sent_at", "granularity": "minute"}}}),
		"an unknown timezone":    analyticsArgs(map[string]any{"timezone": "Mars/Olympus"}),
		"limit above 1000":       analyticsArgs(map[string]any{"limit": 1001}),
		"limit of 0":             analyticsArgs(map[string]any{"limit": 0}),
		"a negative offset":      analyticsArgs(map[string]any{"offset": -1}),
		"a filter without values": analyticsArgs(map[string]any{"filters": []map[string]any{
			{"member": "contact_email", "operator": "equals"},
		}}),
		"no schema": {"workspace_id": testWorkspaceID, "measures": []string{"count"}},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, CodeValidation, f.callTool("query_analytics", args).errorOf(t).Code)
		})
	}
}

// The pair is checked against the schema argument before any service call.
func TestQueryAnalytics_PermissionOfTheSchema(t *testing.T) {
	f := newFixture(t)
	f.reachable(domain.UserPermissions{domain.PermissionResourceMessageHistory: {Read: true}})

	for schema, resource := range map[string]string{"contacts": "contacts", "web_sessions": "web_analytics", "automation_node_executions": "automations"} {
		toolErr := f.callTool("query_analytics", analyticsArgs(map[string]any{"schema": schema})).errorOf(t)
		assert.Equal(t, ToolError{
			Code: CodePermissionDenied, Message: fmt.Sprintf("this API key lacks the %s read permission", resource), Resource: resource, Permission: "read",
		}, toolErr, schema)
	}
}

func TestQueryAnalytics_ATimeoutSaysToNarrowTheQuery(t *testing.T) {
	saved := analyticsTimeout
	analyticsTimeout = 50 * time.Millisecond
	t.Cleanup(func() { analyticsTimeout = saved })
	f := newFixture(t)
	f.reachable(domain.UserPermissions{domain.PermissionResourceMessageHistory: {Read: true}})
	f.analytics.EXPECT().GetSchemas(gomock.Any(), testWorkspaceID).Return(testSchemas(), nil)
	f.analytics.EXPECT().Query(gomock.Any(), testWorkspaceID, gomock.Any()).
		DoAndReturn(func(ctx context.Context, _ string, _ analytics.Query) (*analytics.Response, error) {
			// What lib/pq answers when the context ends mid-query.
			<-ctx.Done()
			return nil, errors.New("failed to execute query: pq: canceling statement due to user request")
		})

	toolErr := f.callTool("query_analytics", analyticsArgs(nil)).errorOf(t)
	assert.Equal(t, CodeValidation, toolErr.Code)
	assert.Equal(t, "the query ran past 50ms: narrow its date range or filters", toolErr.Message)
}

func TestQueryAnalytics_AFailedQueryIsInternal(t *testing.T) {
	f := newFixture(t)
	f.reachable(domain.UserPermissions{domain.PermissionResourceMessageHistory: {Read: true}})
	f.analytics.EXPECT().GetSchemas(gomock.Any(), testWorkspaceID).Return(testSchemas(), nil)
	f.analytics.EXPECT().Query(gomock.Any(), testWorkspaceID, gomock.Any()).Return(nil, errors.New("pq: relation message_history on db-host-7"))
	assert.Equal(t, ToolError{Code: CodeInternal, Message: "internal error"}, f.callTool("query_analytics", analyticsArgs(nil)).errorOf(t))
}
