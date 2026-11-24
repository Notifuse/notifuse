package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/analytics"
)

// Bounds of query_analytics.
const (
	maxAnalyticsRows     = 1000
	defaultAnalyticsRows = 100
)

// analyticsTimeout bounds one query_analytics call. A variable so that a test
// can shorten it.
var analyticsTimeout = 10 * time.Second

// excludedAnalyticsSchema is the delivery log of the outbound webhooks, left
// out of MCP with the webhook subscriptions themselves.
const excludedAnalyticsSchema = "webhook_deliveries"

var analyticsFilterOperators = []string{
	"equals", "notEquals", "contains", "notContains", "startsWith", "notStartsWith", "endsWith", "notEndsWith",
	"gt", "gte", "lt", "lte", "in", "notIn", "set", "notSet", "inDateRange", "notInDateRange", "beforeDate", "afterDate",
}

// listAnalyticsSchemasTool lists the analytics schemas the key may query.
// analytics.schemas keeps only the schemas whose resource the key may read.
func (s *Server) listAnalyticsSchemasTool() *Tool {
	return &Tool{
		Name:  "list_analytics_schemas",
		Title: "List analytics schemas",
		Description: "Lists the analytics schemas this key may query with query_analytics. " +
			"Each schema has its measures (counts and aggregates) and its dimensions (the fields to group or filter by, time dimensions included). " +
			"A member has a name, a type and a title, and a description when the schema gives one. " +
			"A schema is listed only when the key can read its resource: " +
			"message_history and email_queue need message_history, contacts needs contacts, broadcasts needs broadcasts, automation_node_executions needs automations, and the web analytics schemas need web_analytics.",
		Class: ClassRead,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"workspace_id": map[string]any{"type": "string", "description": "Workspace ID from list_workspaces."},
			},
			"required":             []string{"workspace_id"},
			"additionalProperties": false,
		},
		Handler: s.listAnalyticsSchemas,
	}
}

type analyticsMember struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
}

// analyticsSchema is a schema without its SQL fragments.
type analyticsSchema struct {
	Name       string            `json:"name"`
	Measures   []analyticsMember `json:"measures"`
	Dimensions []analyticsMember `json:"dimensions"`
}

type listAnalyticsSchemasOutput struct {
	Workspace WorkspaceRef      `json:"workspace"`
	Schemas   []analyticsSchema `json:"schemas"`
	Truncation
}

func (s *Server) listAnalyticsSchemas(ctx context.Context, call *Call) (any, error) {
	schemas, err := s.deps.Analytics.GetSchemas(ctx, call.WorkspaceID)
	if err != nil {
		return nil, err
	}
	out := listAnalyticsSchemasOutput{Workspace: call.WorkspaceRef(), Schemas: []analyticsSchema{}}
	for _, name := range slices.Sorted(maps.Keys(schemas)) {
		if name == excludedAnalyticsSchema {
			continue
		}
		schema := schemas[name]
		index := len(out.Schemas)
		projected := analyticsSchema{Name: name, Measures: []analyticsMember{}, Dimensions: []analyticsMember{}}
		for _, measureName := range slices.Sorted(maps.Keys(schema.Measures)) {
			measure := schema.Measures[measureName]
			path := fmt.Sprintf("schemas[%d].measures[%d]", index, len(projected.Measures))
			projected.Measures = append(projected.Measures, analyticsMember{
				Name:        measureName,
				Type:        measure.Type,
				Title:       out.Text(path+".title", measure.Title),
				Description: out.Text(path+".description", measure.Description),
			})
		}
		for _, dimensionName := range slices.Sorted(maps.Keys(schema.Dimensions)) {
			dimension := schema.Dimensions[dimensionName]
			path := fmt.Sprintf("schemas[%d].dimensions[%d]", index, len(projected.Dimensions))
			projected.Dimensions = append(projected.Dimensions, analyticsMember{
				Name:        dimensionName,
				Type:        dimension.Type,
				Title:       out.Text(path+".title", dimension.Title),
				Description: out.Text(path+".description", dimension.Description),
			})
		}
		out.Schemas = append(out.Schemas, projected)
	}
	return out, nil
}

// queryAnalyticsTool runs one analytics query. Its permission depends on the
// schema argument, so it has no static pair: tools/list shows it to every key,
// and the call checks the read pair of the schema's resource.
func (s *Server) queryAnalyticsTool() *Tool {
	return &Tool{
		Name:  "query_analytics",
		Title: "Query analytics",
		Description: "Runs an aggregate query on one analytics schema from list_analytics_schemas. " +
			"You pick the measures to compute and the dimensions to group by. Time dimensions group by hour, day, week, month or year over a date range. " +
			"To count a period, give the time dimension a date_range: both bounds are included, a date as the end covers that whole day, and the bounds are read in timezone. " +
			"Date filters (inDateRange, notInDateRange, beforeDate, afterDate) read their values the same way: in timezone, with a date standing for that whole day. " +
			"beforeDate and afterDate leave out the value given, and for a date its whole day. " +
			"Hour periods are clock hours in timezone: when the clocks go back, the repeated hour is one period that holds both, and the hour skipped when they go forward has no period. " +
			"You can filter on dimensions, filter on measures (having), and set one sort order. " +
			"On a time series with a date_range, the sort order, offset and limit apply to the filled periods. " +
			"Name members as list_analytics_schemas names them, without the schema prefix. " +
			"A query returns at most 1000 rows (default 100) and runs for at most 10 seconds. limit_reached is true when the row limit cut the result; " +
			"use offset to read further rows. Text cells are cut at 2000 characters and named in truncated_fields. " +
			"The key needs read access to the schema's resource.",
		Class:         ClassRead,
		ArgumentPairs: analyticsSchemaPairs,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"workspace_id": map[string]any{"type": "string", "description": "Workspace ID from list_workspaces."},
				"schema":       map[string]any{"type": "string", "description": "A schema name from list_analytics_schemas."},
				"measures":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Measures to compute. Give at least one."},
				"dimensions":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Dimensions to group by."},
				"time_dimensions": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"dimension":   map[string]any{"type": "string", "description": "A time dimension of the schema."},
							"granularity": map[string]any{"type": "string", "enum": []string{"hour", "day", "week", "month", "year"}},
							"date_range":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Exactly two bounds, [from, to], both included, such as [\"2026-11-01\", \"2026-11-30\"]. A bound is a date, a date and time, or an RFC 3339 time, read in timezone. A date as the end covers that whole day."},
						},
						"required":             []string{"dimension", "granularity"},
						"additionalProperties": false,
					},
					"description": "The time dimension to group by, at most one, over an optional date range. Periods without data are filled: counts and sums with 0, other measures (medians, rates, averages) with null.",
				},
				"filters": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"member":   map[string]any{"type": "string", "description": "A dimension of the schema. Conditions on measures go in having."},
							"operator": map[string]any{"type": "string", "enum": analyticsFilterOperators},
							"values":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Values to compare with. set and notSet take none."},
						},
						"required":             []string{"member", "operator"},
						"additionalProperties": false,
					},
					"description": "Conditions that every counted row must meet.",
				},
				"having": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"member":   map[string]any{"type": "string", "description": "A measure of the schema."},
							"operator": map[string]any{"type": "string", "enum": []string{"equals", "notEquals", "gt", "gte", "lt", "lte"}},
							"values":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
						},
						"required":             []string{"member", "operator", "values"},
						"additionalProperties": false,
					},
					"description": "Conditions on the computed measures of each group.",
				},
				"order": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"member":    map[string]any{"type": "string", "description": "A measure, a dimension of the query or its time dimension, which sorts by period."},
							"direction": map[string]any{"type": "string", "enum": []string{"asc", "desc"}},
						},
						"required":             []string{"member", "direction"},
						"additionalProperties": false,
					},
					"description": "The sort order: at most one entry.",
				},
				"timezone": map[string]any{"type": "string", "description": "IANA time zone the time dimensions are grouped in, such as Europe/Paris. Default UTC."},
				"limit":    map[string]any{"type": "integer", "description": "1 to 1000, default 100."},
				"offset":   map[string]any{"type": "integer", "description": "Rows to skip, default 0."},
			},
			"required":             []string{"workspace_id", "schema", "measures"},
			"additionalProperties": false,
		},
		Handler: s.queryAnalytics,
	}
}

// analyticsSchemaPairs is the read pair of the schema's resource. A schema
// MCP leaves out, or one without a resource, adds none: the handler refuses
// it as invalid.
func analyticsSchemaPairs(args map[string]any) []Pair {
	schema, _ := args["schema"].(string)
	if schema == excludedAnalyticsSchema {
		return nil
	}
	resource, ok := domain.AnalyticsSchemaResource(schema)
	if !ok {
		return nil
	}
	return []Pair{{Resource: resource, Verb: domain.PermissionTypeRead}}
}

type analyticsTimeDimensionArg struct {
	Dimension   string   `json:"dimension"`
	Granularity string   `json:"granularity"`
	DateRange   []string `json:"date_range"`
}

type analyticsFilterArg struct {
	Member   string   `json:"member"`
	Operator string   `json:"operator"`
	Values   []string `json:"values"`
}

type analyticsOrderArg struct {
	Member    string `json:"member"`
	Direction string `json:"direction"`
}

type queryAnalyticsArgs struct {
	Schema         string                      `json:"schema"`
	Measures       []string                    `json:"measures"`
	Dimensions     []string                    `json:"dimensions"`
	TimeDimensions []analyticsTimeDimensionArg `json:"time_dimensions"`
	Filters        []analyticsFilterArg        `json:"filters"`
	Having         []analyticsFilterArg        `json:"having"`
	Order          []analyticsOrderArg         `json:"order"`
	Timezone       string                      `json:"timezone"`
	Limit          *int                        `json:"limit"`
	Offset         *int                        `json:"offset"`
}

type queryAnalyticsOutput struct {
	Workspace    WorkspaceRef     `json:"workspace"`
	Schema       string           `json:"schema"`
	Rows         []map[string]any `json:"rows"`
	RowCount     int              `json:"row_count"`
	LimitReached bool             `json:"limit_reached"`
	Truncation
}

func (s *Server) queryAnalytics(ctx context.Context, call *Call) (any, error) {
	var args queryAnalyticsArgs
	if err := call.DecodeArgs(&args); err != nil {
		return nil, validationError("invalid arguments: %v", err)
	}
	if args.Schema == excludedAnalyticsSchema {
		return nil, validationError("the %s schema cannot be queried through MCP", excludedAnalyticsSchema)
	}
	if _, ok := domain.AnalyticsSchemaResource(args.Schema); !ok {
		return nil, validationError("unknown schema %q: list_analytics_schemas names the schemas this key may query", args.Schema)
	}
	request, limit, toolErr := args.request()
	if toolErr != nil {
		return nil, toolErr
	}
	// The request is decoded from JSON, as every service request built from
	// tool arguments is.
	body, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	var query analytics.Query
	if err := json.Unmarshal(body, &query); err != nil {
		return nil, err
	}

	// The repository bounds every statement at analytics.MaxQueryDuration; MCP answers
	// sooner.
	ctx, cancel := context.WithTimeout(ctx, analyticsTimeout)
	defer cancel()

	// analytics.query refuses an invalid query with an untyped error, so the
	// query is validated here first against the schemas the key may read.
	schemas, err := s.deps.Analytics.GetSchemas(ctx, call.WorkspaceID)
	if err != nil {
		return nil, timeoutOr(ctx, err)
	}
	if _, ok := schemas[query.Schema]; !ok {
		return nil, validationError("unknown schema %q: list_analytics_schemas names the schemas this key may query", args.Schema)
	}
	if err := analytics.DefaultValidate(query, schemas); err != nil {
		return nil, validationError("invalid query: %v", err)
	}

	resp, err := s.deps.Analytics.Query(ctx, call.WorkspaceID, query)
	if err != nil {
		return nil, timeoutOr(ctx, err)
	}

	// The generated SQL, its parameters and the execution time in resp.Meta
	// are never returned.
	out := queryAnalyticsOutput{Workspace: call.WorkspaceRef(), Schema: query.Schema, Rows: []map[string]any{}}
	var data []map[string]any
	if resp != nil {
		data = resp.Data
	}
	// The query asks for one row past the limit, so a page of exactly limit
	// rows is not reported as cut. On a gap-filled series the engine applies
	// that limit after the filling, so the extra row means the same there.
	if len(data) > limit {
		data = data[:limit]
		out.LimitReached = true
	}
	for i, row := range data {
		out.Rows = append(out.Rows, out.analyticsRow(i, row))
	}
	out.RowCount = len(out.Rows)
	return out, nil
}

// request checks the arguments the schema cannot and composes the body of an
// analytics query, with the wire names of analytics.Query.
func (a *queryAnalyticsArgs) request() (map[string]any, int, *ToolError) {
	if len(a.Measures) == 0 {
		return nil, 0, validationError("measures needs at least one measure")
	}
	if len(a.Order) > 1 {
		return nil, 0, validationError("order takes at most one entry")
	}
	limit := defaultAnalyticsRows
	if a.Limit != nil {
		if *a.Limit < 1 || *a.Limit > maxAnalyticsRows {
			return nil, 0, validationError("limit must be between 1 and %d", maxAnalyticsRows)
		}
		limit = *a.Limit
	}
	// One row past the limit tells a complete page from a cut one; queryAnalytics
	// trims it, so the client never gets more than limit rows.
	request := map[string]any{"schema": a.Schema, "measures": a.Measures, "limit": limit + 1}
	if a.Offset != nil {
		if *a.Offset < 0 {
			return nil, 0, validationError("offset cannot be negative")
		}
		request["offset"] = *a.Offset
	}
	if len(a.Dimensions) > 0 {
		request["dimensions"] = a.Dimensions
	}
	if a.Timezone != "" {
		if err := analytics.ValidateTimezone(a.Timezone); err != nil {
			return nil, 0, validationError("timezone must be an IANA time zone, such as Europe/Paris")
		}
		request["timezone"] = a.Timezone
	}
	if len(a.TimeDimensions) > 0 {
		timeDimensions := make([]map[string]any, 0, len(a.TimeDimensions))
		for i, td := range a.TimeDimensions {
			entry := map[string]any{"dimension": td.Dimension, "granularity": td.Granularity}
			if td.DateRange != nil {
				if len(td.DateRange) != 2 {
					return nil, 0, validationError("time_dimensions[%d].date_range must hold exactly two dates, [from, to]", i)
				}
				entry["dateRange"] = td.DateRange
			}
			timeDimensions = append(timeDimensions, entry)
		}
		request["timeDimensions"] = timeDimensions
	}
	if len(a.Filters) > 0 {
		request["filters"] = a.Filters
	}
	if len(a.Having) > 0 {
		request["having"] = a.Having
	}
	if len(a.Order) == 1 {
		request["order"] = map[string]string{a.Order[0].Member: a.Order[0].Direction}
	}
	return request, limit, nil
}

// analyticsRow cleans the text cells of one row: they can hold contact
// emails, URLs or page paths.
func (o *queryAnalyticsOutput) analyticsRow(index int, row map[string]any) map[string]any {
	out := make(map[string]any, len(row))
	for _, key := range slices.Sorted(maps.Keys(row)) {
		path := fmt.Sprintf("rows[%d].%s", index, key)
		switch value := row[key].(type) {
		case string:
			out[key] = o.Text(path, value)
		case []byte:
			out[key] = o.Text(path, string(value))
		case map[string]any, []any:
			out[key] = o.JSON(path, value)
		default:
			out[key] = value
		}
	}
	return out
}

// timeoutOr answers a query cut by the timeout with what to do about it.
func timeoutOr(ctx context.Context, err error) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return validationError("the query ran past %s: narrow its date range or filters", analyticsTimeout)
	}
	return err
}
