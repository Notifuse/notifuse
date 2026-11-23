package mcp

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
)

const listSegmentsName = "list_segments"

// previewTimeout bounds a segment preview: neither the service nor the
// repository sets one, and the count runs over every contact. A variable so a
// test can shorten it.
var previewTimeout = 10 * time.Second

// segmentTreeDescription is the grammar of a segment tree, which the input
// schema cannot carry without $ref. The decoder refuses unknown keys and the
// tree's own validation runs before the service is called.
const segmentTreeDescription = `Segment condition tree. A node is {"kind":"branch","branch":{"operator":"and"|"or","leaves":[node,...]}} or {"kind":"leaf","leaf":LEAF}. A branch needs at least one node. Groups nest.
LEAF is one of:
- {"source":"contacts","contact":{"filters":[FILTER,...]}}: all filters must match.
- {"source":"contact_lists","contact_list":{"operator":"in"|"not_in","list_id":"<list id>","status":"active"|"pending"|"unsubscribed"|"bounced"|"complained" (optional)}}
- {"source":"contact_timeline","contact_timeline":{"kind":"email.sent"|"email.opened"|"email.clicked"|"email.bounced"|"email.complained"|"email.unsubscribed"|"web.pageview"|"web.session","count_operator":"at_least"|"at_most"|"exactly","count_value":<integer >= 0>, optional "template_id" and "broadcast_id" (email kinds), "link_url" (email.clicked, substring), "timeframe_operator":"anytime"|"in_the_last_days"|"in_date_range"|"before_date"|"after_date", "timeframe_values":[...], "filters":[FILTER,...]}}
- {"source":"custom_events_goals","custom_events_goal":{"goal_type":"purchase"|"subscription"|"lead"|"signup"|"booking"|"trial"|"other"|"*", optional "goal_name" and "event_name", "aggregate_operator":"sum"|"count"|"avg"|"min"|"max","operator":"gte"|"lte"|"eq"|"between","value":<number>,"value_2":<number, for between>,"timeframe_operator":"anytime"|"in_the_last_days"|"in_date_range"|"before_date"|"after_date","timeframe_values":[...] (unless anytime),"negate":<boolean, optional>,"filters":[FILTER,...] (optional, on event properties)}}
FILTER: {"field_name":...,"field_type":"string"|"number"|"time"|"json","operator":...,"string_values":[...],"number_values":[...],"json_path":[...]}.
Contact fields: string: email, external_id, timezone, language, first_name, last_name, phone, address_line_1, address_line_2, country, postcode, state, job_title, custom_string_1 to custom_string_5, tracking_opens_consent, tracking_clicks_consent, tracking_consent_source. number: custom_number_1 to custom_number_5. time: created_at, updated_at, tracking_consent_updated_at, custom_datetime_1 to custom_datetime_5. json: custom_json_1 to custom_json_5, with json_path (keys, or array indexes as strings).
Operators: string: equals, not_equals, contains, not_contains, is_set, is_not_set. number: equals, not_equals, gt, gte, lt, lte, is_set, is_not_set. time: before_date, after_date, in_date_range, not_in_date_range, in_the_last_days, not_in_the_last_days, is_set, is_not_set. json also: in_array.
Values: string, time and json filters use string_values; number filters use number_values. Dates are RFC 3339 or YYYY-MM-DD; in_date_range takes two; in_the_last_days takes a day count such as "30". is_set and is_not_set take none.
Unknown keys are refused. Read an existing segment with get_segment for a full example.`

// listSegmentsTool pages through a workspace's segments. segments.list
// returns every segment at once, so the tool pages them by offset on a
// stable order.
func (s *Server) listSegmentsTool() *Tool {
	return &Tool{
		Name:  listSegmentsName,
		Title: "List segments",
		Description: "Lists the segments of a workspace, newest first. " +
			"Each has its name, color, status (active or building), timezone, whether it is rebuilt every day because its conditions use relative dates, and a summary of its last computation error. " +
			"get_segment has the conditions tree and the full error. Set with_count to add how many contacts each segment holds. " +
			"To get the next page, pass next_cursor back as cursor with the same with_count. A segment created between two pages can show up twice.",
		Class: ClassRead,
		Pairs: []Pair{{Resource: domain.PermissionResourceSegments, Verb: domain.PermissionTypeRead}},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"workspace_id": map[string]any{"type": "string", "description": "Workspace ID from list_workspaces."},
				"with_count":   map[string]any{"type": "boolean", "description": "Add users_count, the number of contacts in each segment. Slower on large workspaces. Default false."},
				"limit":        map[string]any{"type": "integer", "description": "1 to 50, default 20."},
				"cursor":       map[string]any{"type": "string", "description": "next_cursor of the previous page, with the same with_count."},
			},
			"required":             []string{"workspace_id"},
			"additionalProperties": false,
		},
		Handler: s.listSegments,
	}
}

type listSegmentsArgs struct {
	WithCount bool   `json:"with_count"`
	Limit     *int   `json:"limit"`
	Cursor    string `json:"cursor"`
}

// segmentErrorSummary is the part of a segment's last error a listing shows.
type segmentErrorSummary struct {
	Kind   string `json:"kind"`
	Column string `json:"column,omitempty"`
	Source string `json:"source"`
}

// segmentSummary is the projection list_segments returns. The tree is left
// to get_segment, and the SQL compiled from it, its arguments and the build
// version are never returned.
type segmentSummary struct {
	ID              string               `json:"id"`
	Name            string               `json:"name"`
	Color           string               `json:"color"`
	Status          string               `json:"status"`
	Timezone        string               `json:"timezone"`
	UsersCount      *int                 `json:"users_count,omitempty"`
	RecomputesDaily bool                 `json:"recomputes_daily"`
	LastError       *segmentErrorSummary `json:"last_error"`
	CreatedAt       time.Time            `json:"created_at"`
	UpdatedAt       time.Time            `json:"updated_at"`
	Truncation
}

type listSegmentsOutput struct {
	Workspace  WorkspaceRef     `json:"workspace"`
	Segments   []segmentSummary `json:"segments"`
	NextCursor string           `json:"next_cursor,omitempty"`
}

func (s *Server) listSegments(ctx context.Context, call *Call) (any, error) {
	var args listSegmentsArgs
	if err := call.DecodeArgs(&args); err != nil {
		return nil, validationError("invalid arguments: %v", err)
	}
	limit, toolErr := PageLimit(args.Limit)
	if toolErr != nil {
		return nil, toolErr
	}
	filters := map[string]any{}
	if args.WithCount {
		filters["with_count"] = true
	}
	offset := 0
	if args.Cursor != "" {
		if toolErr := DecodeCursor(args.Cursor, listSegmentsName, call.WorkspaceID, filters, &offset); toolErr != nil {
			return nil, toolErr
		}
		if offset < 0 {
			return nil, validationError("cursor is not valid for this call: drop it to start from the first page")
		}
	}

	var req domain.GetSegmentsRequest
	if err := decodeRequest(map[string]any{"workspace_id": call.WorkspaceID, "with_count": args.WithCount}, &req); err != nil {
		return nil, err
	}
	segments, err := s.deps.Segments.ListSegments(ctx, &req)
	if err != nil {
		return nil, err
	}
	segments = slices.DeleteFunc(segments, func(segment *domain.Segment) bool {
		return segment == nil || segment.Status == string(domain.SegmentStatusDeleted)
	})
	// segments.list orders by creation time alone; the ID breaks ties so an
	// offset always lands on the same segment.
	slices.SortFunc(segments, func(a, b *domain.Segment) int {
		if c := b.DBCreatedAt.Compare(a.DBCreatedAt); c != 0 {
			return c
		}
		return cmp.Compare(a.ID, b.ID)
	})

	out := listSegmentsOutput{Workspace: call.WorkspaceRef(), Segments: []segmentSummary{}}
	if offset >= len(segments) {
		return out, nil
	}
	end := min(offset+limit, len(segments))
	for _, segment := range segments[offset:end] {
		out.Segments = append(out.Segments, summarizeSegment(segment, args.WithCount))
	}
	if end < len(segments) {
		next, err := EncodeCursor(listSegmentsName, call.WorkspaceID, filters, end)
		if err != nil {
			return nil, err
		}
		out.NextCursor = next
	}
	return out, nil
}

func summarizeSegment(segment *domain.Segment, withCount bool) segmentSummary {
	out := segmentSummary{
		ID:              segment.ID,
		Status:          segment.Status,
		RecomputesDaily: segment.RecomputeAfter != nil,
		CreatedAt:       segment.DBCreatedAt,
		UpdatedAt:       segment.DBUpdatedAt,
	}
	out.Name = out.Text("name", segment.Name)
	out.Color = out.Text("color", segment.Color)
	out.Timezone = out.Text("timezone", segment.Timezone)
	// Without with_count the repository answers 0, which would read as empty.
	if withCount {
		count := segment.UsersCount
		out.UsersCount = &count
	}
	if segment.LastError != nil {
		out.LastError = &segmentErrorSummary{
			Kind:   segment.LastError.Kind,
			Column: out.Text("last_error.column", segment.LastError.Column),
			Source: segment.LastError.Source,
		}
	}
	return out
}

// getSegmentTool reads one segment with its conditions tree.
func (s *Server) getSegmentTool() *Tool {
	return &Tool{
		Name:  "get_segment",
		Title: "Get segment",
		Description: "Reads one segment: " +
			"its name, color, status, timezone, full conditions tree, how many contacts it holds, whether it is rebuilt every day because its conditions use relative dates, and its last computation error. " +
			"The error's email, value and cause are empty when the key lacks the contacts read permission. " +
			"Returns a not_found error for a deleted segment.",
		Class: ClassRead,
		Pairs: []Pair{{Resource: domain.PermissionResourceSegments, Verb: domain.PermissionTypeRead}},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"workspace_id": map[string]any{"type": "string", "description": "Workspace ID from list_workspaces."},
				"id":           map[string]any{"type": "string", "description": "Segment ID: lowercase letters, digits and underscores, 1 to 32 characters."},
			},
			"required":             []string{"workspace_id", "id"},
			"additionalProperties": false,
		},
		Handler: s.getSegment,
	}
}

type getSegmentArgs struct {
	ID string `json:"id"`
}

// segmentError is a segment's last computation error. Email, value and cause
// quote contact data and a database message, so they are capped.
type segmentError struct {
	Kind   string   `json:"kind"`
	Source string   `json:"source"`
	Column string   `json:"column,omitempty"`
	Path   []string `json:"path"`
	Type   string   `json:"type,omitempty"`
	Email  string   `json:"email,omitempty"`
	Value  string   `json:"value,omitempty"`
	Cause  string   `json:"cause,omitempty"`
}

// segmentFull is the projection get_segment returns: the tree whole, without
// the SQL compiled from it, its arguments or the build version.
type segmentFull struct {
	ID              string           `json:"id"`
	Name            string           `json:"name"`
	Color           string           `json:"color"`
	Status          string           `json:"status"`
	Timezone        string           `json:"timezone"`
	Tree            *domain.TreeNode `json:"tree"`
	UsersCount      int              `json:"users_count"`
	RecomputesDaily bool             `json:"recomputes_daily"`
	LastError       *segmentError    `json:"last_error"`
	LastErrorAt     *time.Time       `json:"last_error_at,omitempty"`
	CreatedAt       time.Time        `json:"created_at"`
	UpdatedAt       time.Time        `json:"updated_at"`
	Truncation
}

type getSegmentOutput struct {
	Workspace WorkspaceRef `json:"workspace"`
	Segment   segmentFull  `json:"segment"`
}

func (s *Server) getSegment(ctx context.Context, call *Call) (any, error) {
	var args getSegmentArgs
	if err := call.DecodeArgs(&args); err != nil {
		return nil, validationError("invalid arguments: %v", err)
	}
	var req domain.GetSegmentRequest
	if err := decodeRequest(map[string]any{"workspace_id": call.WorkspaceID, "id": args.ID}, &req); err != nil {
		return nil, err
	}
	// segments.get checks only that the ID is present.
	if _, _, err := req.Validate(); err != nil {
		return nil, validationError("%v", err)
	}

	segment, err := s.deps.Segments.GetSegment(ctx, &req)
	if err != nil {
		return nil, err
	}
	// A deleted segment keeps its row, and segments.get returns it.
	if segment == nil || segment.Status == string(domain.SegmentStatusDeleted) {
		return nil, &domain.ErrSegmentNotFound{Message: "segment not found"}
	}
	return getSegmentOutput{Workspace: call.WorkspaceRef(), Segment: projectSegment(segment)}, nil
}

func projectSegment(segment *domain.Segment) segmentFull {
	out := segmentFull{
		ID:              segment.ID,
		Status:          segment.Status,
		Tree:            segment.Tree,
		UsersCount:      segment.UsersCount,
		RecomputesDaily: segment.RecomputeAfter != nil,
		LastErrorAt:     segment.LastErrorAt,
		CreatedAt:       segment.DBCreatedAt,
		UpdatedAt:       segment.DBUpdatedAt,
	}
	out.Name = out.Text("name", segment.Name)
	out.Color = out.Text("color", segment.Color)
	out.Timezone = out.Text("timezone", segment.Timezone)
	if last := segment.LastError; last != nil {
		path := make([]string, 0, len(last.Path))
		for i, key := range last.Path {
			path = append(path, out.Text(fmt.Sprintf("last_error.path[%d]", i), key))
		}
		out.LastError = &segmentError{
			Kind:   last.Kind,
			Source: last.Source,
			Column: out.Text("last_error.column", last.Column),
			Path:   path,
			Type:   last.Type,
			Email:  out.Text("last_error.email", last.Email),
			Value:  out.Text("last_error.value", last.Value),
			Cause:  out.Text("last_error.cause", last.Cause),
		}
	}
	return out
}

// previewSegmentTool counts the contacts a conditions tree matches, without
// saving anything.
func (s *Server) previewSegmentTool() *Tool {
	return &Tool{
		Name:  "preview_segment",
		Title: "Preview segment",
		Description: "Counts the contacts a segment conditions tree matches right now, without saving a segment. " +
			"Use it to check conditions. It returns total_count only, never the contacts. " +
			"If the tree does not follow the grammar, or a condition cannot compare a contact value (such as text in a number field), the call returns a validation error with the reason. " +
			"The count gives up after 10 seconds.",
		Class: ClassRead,
		Pairs: []Pair{
			{Resource: domain.PermissionResourceSegments, Verb: domain.PermissionTypeRead},
			{Resource: domain.PermissionResourceContacts, Verb: domain.PermissionTypeRead},
		},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"workspace_id": map[string]any{"type": "string", "description": "Workspace ID from list_workspaces."},
				"tree":         map[string]any{"type": "object", "description": segmentTreeDescription},
			},
			"required":             []string{"workspace_id", "tree"},
			"additionalProperties": false,
		},
		Handler: s.previewSegment,
	}
}

type previewSegmentArgs struct {
	Tree json.RawMessage `json:"tree"`
}

type previewSegmentOutput struct {
	Workspace  WorkspaceRef `json:"workspace"`
	TotalCount int          `json:"total_count"`
}

func (s *Server) previewSegment(ctx context.Context, call *Call) (any, error) {
	var args previewSegmentArgs
	if err := call.DecodeArgs(&args); err != nil {
		return nil, validationError("invalid arguments: %v", err)
	}
	tree, toolErr := decodeSegmentTree(args.Tree)
	if toolErr != nil {
		return nil, toolErr
	}

	ctx, cancel := context.WithTimeout(ctx, previewTimeout)
	defer cancel()
	// The limit is the number of emails the preview would list; it lists none,
	// so the smallest one is asked.
	resp, err := s.deps.Segments.PreviewSegment(ctx, call.WorkspaceID, tree, 1)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, &ToolError{Code: CodeInternal, Message: "the count did not finish within 10 seconds: narrow the conditions and try again"}
		}
		return nil, err
	}
	if resp == nil {
		return nil, errors.New("segment preview returned no result")
	}
	return previewSegmentOutput{Workspace: call.WorkspaceRef(), TotalCount: resp.TotalCount}, nil
}

// decodeSegmentTree reads a tree argument the way the schema cannot say: an
// unknown key at any level is refused, since encoding/json would otherwise
// drop a misspelt optional key and with it a condition. The tree's own
// validation follows, so a bad tree never reaches the service.
func decodeSegmentTree(raw json.RawMessage) (*domain.TreeNode, *ToolError) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var tree domain.TreeNode
	if err := decoder.Decode(&tree); err != nil {
		return nil, validationError("invalid segment tree: %v", err)
	}
	if err := tree.Validate(); err != nil {
		return nil, validationError("invalid segment tree: %v", err)
	}
	return &tree, nil
}

// decodeRequest builds a service request from JSON, as every request built
// from tool arguments is, so a key left out stays the request's zero value.
func decodeRequest(fields map[string]any, req any) error {
	body, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, req)
}
