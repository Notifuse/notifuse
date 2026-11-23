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

func segmentsRead() domain.UserPermissions {
	return domain.UserPermissions{domain.PermissionResourceSegments: {Read: true}}
}

func segmentsAndContactsRead() domain.UserPermissions {
	return domain.UserPermissions{
		domain.PermissionResourceSegments: {Read: true},
		domain.PermissionResourceContacts: {Read: true},
	}
}

var segmentsEpoch = time.Date(2026, 10, 2, 9, 14, 0, 0, time.UTC)

func vipTree() *domain.TreeNode {
	return &domain.TreeNode{Kind: "branch", Branch: &domain.TreeNodeBranch{Operator: "and", Leaves: []*domain.TreeNode{{
		Kind: "leaf",
		Leaf: &domain.TreeNodeLeaf{Source: "contacts", Contact: &domain.ContactCondition{Filters: []*domain.DimensionFilter{
			{FieldName: "country", FieldType: "string", Operator: "equals", StringValues: []string{"FR"}},
		}}},
	}}}}
}

const vipTreeJSON = `{"kind":"branch","branch":{"operator":"and","leaves":[{"kind":"leaf","leaf":{"source":"contacts","contact":{"filters":[
	{"field_name":"country","field_type":"string","operator":"equals","string_values":["FR"]}]}}}]}}`

// vipSegment carries every field segments.get returns, internal ones included.
func vipSegment() *domain.Segment {
	sql := "SELECT email FROM contacts WHERE country = $1"
	recompute := segmentsEpoch.Add(24 * time.Hour)
	failedAt := segmentsEpoch.Add(2 * time.Hour)
	return &domain.Segment{
		ID: "vip", Name: "VIP", Color: "gold", Tree: vipTree(), Timezone: "Europe/Paris", Version: 4,
		Status: "active", GeneratedSQL: &sql, GeneratedArgs: domain.JSONArray{"FR"}, RecomputeAfter: &recompute,
		DBCreatedAt: segmentsEpoch, DBUpdatedAt: segmentsEpoch.Add(time.Hour), UsersCount: 1240,
		LastError: &domain.SegmentLastError{
			Kind: "value_conversion", Column: "custom_json_1", Path: []string{"plan"}, Type: "number",
			Email: "jane@example.com", Value: `"gold"`, Cause: "invalid input syntax for type numeric", Source: "build",
		},
		LastErrorAt: &failedAt,
	}
}

func TestListSegments_Definition(t *testing.T) {
	assertReadTool(t, catalogueTool(t, "list_segments"), "list_segments", []Pair{{Resource: "segments", Verb: "read"}}, []string{"workspace_id"})
}

func TestListSegments_Projection(t *testing.T) {
	f := newFixture(t)
	f.reachable(segmentsRead())
	f.segments.EXPECT().ListSegments(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req *domain.GetSegmentsRequest) ([]*domain.Segment, error) {
			assert.Equal(t, &domain.GetSegmentsRequest{WorkspaceID: testWorkspaceID}, req)
			plain := &domain.Segment{ID: "all", Name: "All", Color: "blue", Timezone: "UTC", Status: "building", DBCreatedAt: segmentsEpoch.Add(-time.Hour), DBUpdatedAt: segmentsEpoch}
			return []*domain.Segment{vipSegment(), plain}, nil
		})

	result := f.callTool("list_segments", map[string]any{"workspace_id": testWorkspaceID})
	require.False(t, result.IsError, string(result.StructuredContent))
	// Without with_count the repository answers 0 for every segment, so the
	// count is left out rather than shown as empty.
	assert.JSONEq(t, `{
		"workspace":{"id":"ws1","name":"Acme"},
		"segments":[
			{"id":"vip","name":"VIP","color":"gold","status":"active","timezone":"Europe/Paris","recomputes_daily":true,
			 "last_error":{"kind":"value_conversion","column":"custom_json_1","source":"build"},
			 "created_at":"2026-10-02T09:14:00Z","updated_at":"2026-10-02T10:14:00Z"},
			{"id":"all","name":"All","color":"blue","status":"building","timezone":"UTC","recomputes_daily":false,
			 "last_error":null,"created_at":"2026-10-02T08:14:00Z","updated_at":"2026-10-02T09:14:00Z"}
		]
	}`, string(result.StructuredContent))
	for _, dropped := range []string{`"tree"`, `"generated_sql"`, `"generated_args"`, `"version"`, `"recompute_after"`, `"users_count"`, "jane@example.com", "invalid input"} {
		assert.NotContains(t, result.Content[0].Text, dropped)
	}
}

func TestListSegments_WithCount(t *testing.T) {
	f := newFixture(t)
	f.reachable(segmentsRead())
	f.segments.EXPECT().ListSegments(gomock.Any(), &domain.GetSegmentsRequest{WorkspaceID: testWorkspaceID, WithCount: true}).
		Return([]*domain.Segment{{ID: "empty", Name: "Empty", DBCreatedAt: segmentsEpoch}}, nil)

	var out struct {
		Segments []map[string]json.RawMessage `json:"segments"`
	}
	f.callTool("list_segments", map[string]any{"workspace_id": testWorkspaceID, "with_count": true}).decode(t, &out)
	require.Len(t, out.Segments, 1)
	assert.Equal(t, "0", string(out.Segments[0]["users_count"]), "asked for, a zero is a count")
}

// segments.list orders by creation time alone and returns every segment; the
// tool pages them newest first with the ID breaking ties, and drops a deleted
// one the service would let through.
func TestListSegments_PagesOnAStableOrder(t *testing.T) {
	f := newFixture(t)
	f.reachable(segmentsRead())
	f.segments.EXPECT().ListSegments(gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, *domain.GetSegmentsRequest) ([]*domain.Segment, error) {
		return []*domain.Segment{
			{ID: "c", DBCreatedAt: segmentsEpoch.Add(time.Hour)},
			{ID: "gone", Status: "deleted", DBCreatedAt: segmentsEpoch.Add(2 * time.Hour)},
			{ID: "b", DBCreatedAt: segmentsEpoch.Add(time.Hour)},
			{ID: "a", DBCreatedAt: segmentsEpoch},
		}, nil
	}).Times(2)

	type page struct {
		Segments []struct {
			ID string `json:"id"`
		} `json:"segments"`
		NextCursor string `json:"next_cursor"`
	}
	var first page
	f.callTool("list_segments", map[string]any{"workspace_id": testWorkspaceID, "limit": 2}).decode(t, &first)
	require.Len(t, first.Segments, 2)
	assert.Equal(t, "b", first.Segments[0].ID)
	assert.Equal(t, "c", first.Segments[1].ID)
	require.NotEmpty(t, first.NextCursor)

	var second page
	f.callTool("list_segments", map[string]any{"workspace_id": testWorkspaceID, "limit": 2, "cursor": first.NextCursor}).decode(t, &second)
	require.Len(t, second.Segments, 1)
	assert.Equal(t, "a", second.Segments[0].ID)
	assert.Empty(t, second.NextCursor, "absent on the last page")

	toolErr := f.callTool("list_segments", map[string]any{"workspace_id": testWorkspaceID, "with_count": true, "cursor": first.NextCursor}).errorOf(t)
	assert.Equal(t, CodeValidation, toolErr.Code, "a cursor is bound to with_count")
}

func TestListSegments_RefusesBadArgumentsBeforeTheService(t *testing.T) {
	f := newFixture(t)
	f.reachable(segmentsRead())
	negative, err := EncodeCursor("list_segments", testWorkspaceID, nil, -1)
	require.NoError(t, err)
	otherTool, err := EncodeCursor("list_lists", testWorkspaceID, nil, 2)
	require.NoError(t, err)

	for name, args := range map[string]map[string]any{
		"limit above 50":        {"limit": 51},
		"garbage cursor":        {"cursor": "%%%"},
		"negative offset":       {"cursor": negative},
		"another tool's cursor": {"cursor": otherTool},
		"with_count not a bool": {"with_count": "yes"},
	} {
		t.Run(name, func(t *testing.T) {
			args["workspace_id"] = testWorkspaceID
			assert.Equal(t, CodeValidation, f.callTool("list_segments", args).errorOf(t).Code)
		})
	}
}

func TestListSegments_CapsFreeText(t *testing.T) {
	f := newFixture(t)
	f.reachable(segmentsRead())
	f.segments.EXPECT().ListSegments(gomock.Any(), gomock.Any()).
		Return([]*domain.Segment{{ID: "long", Name: strings.Repeat("n", 2001), Color: "re\x07d", DBCreatedAt: segmentsEpoch}}, nil)

	var out struct {
		Segments []map[string]json.RawMessage `json:"segments"`
	}
	f.callTool("list_segments", map[string]any{"workspace_id": testWorkspaceID}).decode(t, &out)
	require.Len(t, out.Segments, 1)
	assert.Equal(t, `"`+strings.Repeat("n", 2000)+`"`, string(out.Segments[0]["name"]))
	assert.Equal(t, `"red"`, string(out.Segments[0]["color"]))
	assert.JSONEq(t, `["name"]`, string(out.Segments[0]["truncated_fields"]))
}

func TestListSegments_ErrorsAreMapped(t *testing.T) {
	t.Run("a key without segments:read", func(t *testing.T) {
		f := newFixture(t)
		f.reachable(contactsRead())
		toolErr := f.callTool("list_segments", map[string]any{"workspace_id": testWorkspaceID}).errorOf(t)
		assert.Equal(t, ToolError{Code: CodePermissionDenied, Message: "this API key lacks the segments read permission", Resource: "segments", Permission: "read"}, toolErr)
	})

	t.Run("a failed read", func(t *testing.T) {
		f := newFixture(t)
		f.reachable(segmentsRead())
		f.segments.EXPECT().ListSegments(gomock.Any(), gomock.Any()).Return(nil, errors.New("failed to list segments: pq on db-host-7"))
		assert.Equal(t, ToolError{Code: CodeInternal, Message: "internal error"}, f.callTool("list_segments", map[string]any{"workspace_id": testWorkspaceID}).errorOf(t))
	})

	t.Run("a switched-off workspace", func(t *testing.T) {
		f := newFixture(t)
		f.switchedOff()
		assert.Equal(t, CodeNotFound, f.callTool("list_segments", map[string]any{"workspace_id": testWorkspaceID}).errorOf(t).Code)
	})
}

func TestGetSegment_Definition(t *testing.T) {
	assertReadTool(t, catalogueTool(t, "get_segment"), "get_segment", []Pair{{Resource: "segments", Verb: "read"}}, []string{"workspace_id", "id"})
}

func TestGetSegment_Projection(t *testing.T) {
	f := newFixture(t)
	f.reachable(segmentsRead())
	f.segments.EXPECT().GetSegment(gomock.Any(), &domain.GetSegmentRequest{WorkspaceID: testWorkspaceID, ID: "vip"}).Return(vipSegment(), nil)

	result := f.callTool("get_segment", map[string]any{"workspace_id": testWorkspaceID, "id": "vip"})
	require.False(t, result.IsError, string(result.StructuredContent))
	assert.JSONEq(t, `{
		"workspace":{"id":"ws1","name":"Acme"},
		"segment":{
			"id":"vip","name":"VIP","color":"gold","status":"active","timezone":"Europe/Paris",
			"tree":`+vipTreeJSON+`,
			"users_count":1240,"recomputes_daily":true,
			"last_error":{"kind":"value_conversion","source":"build","column":"custom_json_1","path":["plan"],"type":"number",
				"email":"jane@example.com","value":"\"gold\"","cause":"invalid input syntax for type numeric"},
			"last_error_at":"2026-10-02T11:14:00Z",
			"created_at":"2026-10-02T09:14:00Z","updated_at":"2026-10-02T10:14:00Z"
		}
	}`, string(result.StructuredContent))
	for _, dropped := range []string{`"generated_sql"`, `"generated_args"`, "SELECT", `"version"`, `"recompute_after"`, `"db_created_at"`} {
		assert.NotContains(t, result.Content[0].Text, dropped)
	}
}

// The tree is editable content: returned whole, however long, while the
// error's contact value is capped.
func TestGetSegment_TreeWholeErrorCapped(t *testing.T) {
	f := newFixture(t)
	f.reachable(segmentsRead())
	segment := vipSegment()
	long := strings.Repeat("x", 3000)
	segment.Tree.Branch.Leaves[0].Leaf.Contact.Filters[0].StringValues = []string{long}
	segment.LastError.Value = strings.Repeat("v", 2001)
	f.segments.EXPECT().GetSegment(gomock.Any(), gomock.Any()).Return(segment, nil)

	var out struct {
		Segment struct {
			Tree      *domain.TreeNode `json:"tree"`
			LastError struct {
				Value string `json:"value"`
			} `json:"last_error"`
			Truncated       bool     `json:"truncated"`
			TruncatedFields []string `json:"truncated_fields"`
		} `json:"segment"`
	}
	f.callTool("get_segment", map[string]any{"workspace_id": testWorkspaceID, "id": "vip"}).decode(t, &out)
	assert.Equal(t, long, out.Segment.Tree.Branch.Leaves[0].Leaf.Contact.Filters[0].StringValues[0])
	assert.Equal(t, strings.Repeat("v", 2000), out.Segment.LastError.Value)
	assert.True(t, out.Segment.Truncated)
	assert.Equal(t, []string{"last_error.value"}, out.Segment.TruncatedFields)
}

func TestGetSegment_NotFound(t *testing.T) {
	t.Run("a deleted segment, which segments.get returns", func(t *testing.T) {
		f := newFixture(t)
		f.reachable(segmentsRead())
		deleted := vipSegment()
		deleted.Status = "deleted"
		f.segments.EXPECT().GetSegment(gomock.Any(), gomock.Any()).Return(deleted, nil)
		assert.Equal(t, ToolError{Code: CodeNotFound, Message: "segment not found"}, f.callTool("get_segment", map[string]any{"workspace_id": testWorkspaceID, "id": "vip"}).errorOf(t))
	})

	t.Run("an unknown segment", func(t *testing.T) {
		f := newFixture(t)
		f.reachable(segmentsRead())
		f.segments.EXPECT().GetSegment(gomock.Any(), gomock.Any()).
			Return(nil, fmt.Errorf("failed to get segment: %w", &domain.ErrSegmentNotFound{Message: "segment not found: nope"}))
		assert.Equal(t, CodeNotFound, f.callTool("get_segment", map[string]any{"workspace_id": testWorkspaceID, "id": "nope"}).errorOf(t).Code)
	})
}

// segments.get checks only that the ID is present; the tool applies the
// request's own rules before the service.
func TestGetSegment_RefusesABadIDBeforeTheService(t *testing.T) {
	f := newFixture(t)
	f.reachable(segmentsRead())
	for _, id := range []string{"", "VIP", "vip-buyers", strings.Repeat("a", 33)} {
		assert.Equal(t, CodeValidation, f.callTool("get_segment", map[string]any{"workspace_id": testWorkspaceID, "id": id}).errorOf(t).Code, id)
	}
	assert.Equal(t, CodeValidation, f.callTool("get_segment", map[string]any{"workspace_id": testWorkspaceID}).errorOf(t).Code, "id is required")
}

func TestGetSegment_AKeyWithoutSegmentsRead(t *testing.T) {
	f := newFixture(t)
	f.reachable(contactsRead())
	toolErr := f.callTool("get_segment", map[string]any{"workspace_id": testWorkspaceID, "id": "vip"}).errorOf(t)
	assert.Equal(t, "segments", toolErr.Resource)
	assert.Equal(t, CodePermissionDenied, toolErr.Code)
}

func TestPreviewSegment_Definition(t *testing.T) {
	tool := catalogueTool(t, "preview_segment")
	assertReadTool(t, tool, "preview_segment", []Pair{{Resource: "segments", Verb: "read"}, {Resource: "contacts", Verb: "read"}}, []string{"workspace_id", "tree"})
	tree := tool.InputSchema["properties"].(map[string]any)["tree"].(map[string]any)
	assert.Equal(t, "object", tree["type"])
	assert.Equal(t, segmentTreeDescription, tree["description"])
}

func TestPreviewSegment_CountsWithATimeout(t *testing.T) {
	f := newFixture(t)
	f.reachable(segmentsAndContactsRead())
	f.segments.EXPECT().PreviewSegment(gomock.Any(), testWorkspaceID, vipTree(), 1).
		DoAndReturn(func(ctx context.Context, _ string, _ *domain.TreeNode, _ int) (*domain.PreviewSegmentResponse, error) {
			deadline, ok := ctx.Deadline()
			assert.True(t, ok, "the count runs under a deadline")
			assert.WithinDuration(t, time.Now().Add(10*time.Second), deadline, 2*time.Second)
			return &domain.PreviewSegmentResponse{
				Emails: []string{"jane@example.com"}, TotalCount: 1240, Limit: 1,
				GeneratedSQL: "SELECT email FROM contacts WHERE country = $1", SQLArgs: []any{"FR"},
			}, nil
		})

	var tree map[string]any
	require.NoError(t, json.Unmarshal([]byte(vipTreeJSON), &tree))
	result := f.callTool("preview_segment", map[string]any{"workspace_id": testWorkspaceID, "tree": tree})
	require.False(t, result.IsError, string(result.StructuredContent))
	assert.JSONEq(t, `{"workspace":{"id":"ws1","name":"Acme"},"total_count":1240}`, string(result.StructuredContent))
}

// encoding/json would drop a misspelt key and the condition with it, so an
// unknown key at any depth is refused before the service.
func TestPreviewSegment_RefusesABadTreeBeforeTheService(t *testing.T) {
	f := newFixture(t)
	f.reachable(segmentsAndContactsRead())
	for name, tree := range map[string]string{
		"unknown root key":     `{"kind":"leaf","leaf":{"source":"contacts","contact":{"filters":[{"field_name":"email","field_type":"string","operator":"is_set"}]}},"note":"x"}`,
		"misspelt nested key":  `{"kind":"leaf","leaf":{"source":"contact_timeline","contact_timeline":{"kind":"email.opened","count_operator":"at_least","count_value":1,"timeframe_operater":"in_the_last_days"}}}`,
		"empty branch":         `{"kind":"branch","branch":{"operator":"and","leaves":[]}}`,
		"unknown kind":         `{"kind":"twig"}`,
		"wrong type of value":  `{"kind":"leaf","leaf":{"source":"contacts","contact":{"filters":"all"}}}`,
		"leaf without filters": `{"kind":"leaf","leaf":{"source":"contacts","contact":{"filters":[]}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			var decoded map[string]any
			require.NoError(t, json.Unmarshal([]byte(tree), &decoded))
			toolErr := f.callTool("preview_segment", map[string]any{"workspace_id": testWorkspaceID, "tree": decoded}).errorOf(t)
			assert.Equal(t, CodeValidation, toolErr.Code)
			assert.True(t, strings.HasPrefix(toolErr.Message, "invalid segment tree: "), toolErr.Message)
		})
	}
	assert.Equal(t, CodeValidation, f.callTool("preview_segment", map[string]any{"workspace_id": testWorkspaceID, "tree": "all"}).errorOf(t).Code, "a tree is an object")
	assert.Equal(t, CodeValidation, f.callTool("preview_segment", map[string]any{"workspace_id": testWorkspaceID}).errorOf(t).Code, "tree is required")
}

func TestPreviewSegment_ErrorsAreMapped(t *testing.T) {
	var tree map[string]any
	require.NoError(t, json.Unmarshal([]byte(vipTreeJSON), &tree))
	args := map[string]any{"workspace_id": testWorkspaceID, "tree": tree}

	t.Run("a key with segments:read alone", func(t *testing.T) {
		f := newFixture(t)
		f.reachable(segmentsRead())
		toolErr := f.callTool("preview_segment", args).errorOf(t)
		assert.Equal(t, ToolError{Code: CodePermissionDenied, Message: "this API key lacks the contacts read permission", Resource: "contacts", Permission: "read"}, toolErr)
	})

	t.Run("a contact value the query cannot convert", func(t *testing.T) {
		f := newFixture(t)
		f.reachable(segmentsAndContactsRead())
		f.segments.EXPECT().PreviewSegment(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, &domain.SegmentValueError{Column: "custom_json_1", Path: []string{"plan"}, Type: "number", Email: "jane@example.com", Value: "gold", Cause: "invalid input syntax"})
		toolErr := f.callTool("preview_segment", args).errorOf(t)
		assert.Equal(t, ToolError{Code: CodeValidation, Message: `custom_json_1.plan: contact jane@example.com holds "gold", which is not a valid number`}, toolErr)
	})

	t.Run("a tree the query builder refuses", func(t *testing.T) {
		f := newFixture(t)
		f.reachable(segmentsAndContactsRead())
		f.segments.EXPECT().PreviewSegment(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, domain.NewValidationError("invalid segment tree: field not allowed: password"))
		assert.Equal(t, ToolError{Code: CodeValidation, Message: "invalid segment tree: field not allowed: password"}, f.callTool("preview_segment", args).errorOf(t))
	})

	t.Run("a count past the timeout", func(t *testing.T) {
		saved := previewTimeout
		previewTimeout = 50 * time.Millisecond
		t.Cleanup(func() { previewTimeout = saved })
		f := newFixture(t)
		f.reachable(segmentsAndContactsRead())
		f.segments.EXPECT().PreviewSegment(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, _ string, _ *domain.TreeNode, _ int) (*domain.PreviewSegmentResponse, error) {
				// Bounded, so a tool without its timeout fails this test instead of
				// hanging it.
				select {
				case <-ctx.Done():
				case <-time.After(2 * time.Second):
				}
				return nil, fmt.Errorf("failed to preview segment: pq: canceling statement due to user request")
			})
		toolErr := f.callTool("preview_segment", args).errorOf(t)
		assert.Equal(t, CodeInternal, toolErr.Code)
		assert.Contains(t, toolErr.Message, "10 seconds")
	})
}
