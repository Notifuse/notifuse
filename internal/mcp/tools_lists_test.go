package mcp

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
)

func listsRead() domain.UserPermissions {
	return domain.UserPermissions{domain.PermissionResourceLists: {Read: true}}
}

var listsEpoch = time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)

// threeLists are returned as lists.list does, created_at alone deciding the
// order: "b" and "c" share a creation time.
func threeLists() []*domain.List {
	return []*domain.List{
		{ID: "c", Name: "C", CreatedAt: listsEpoch.Add(time.Hour), UpdatedAt: listsEpoch.Add(time.Hour)},
		{ID: "b", Name: "B", CreatedAt: listsEpoch.Add(time.Hour), UpdatedAt: listsEpoch.Add(time.Hour)},
		{ID: "a", Name: "A", CreatedAt: listsEpoch, UpdatedAt: listsEpoch},
	}
}

func TestListLists_Definition(t *testing.T) {
	assertReadTool(t, catalogueTool(t, "list_lists"), "list_lists", []Pair{{Resource: "lists", Verb: "read"}}, []string{"workspace_id"})
}

func TestListLists_Projection(t *testing.T) {
	f := newFixture(t)
	f.reachable(listsRead())
	f.lists.EXPECT().GetLists(gomock.Any(), testWorkspaceID, false).Return([]*domain.List{{
		ID: "news", Name: "Newsletter", Description: "Monthly news", IsPublic: true, IsDoubleOptin: true,
		DoubleOptInTemplate: &domain.TemplateReference{ID: "doi", Version: 2},
		CreatedAt:           listsEpoch, UpdatedAt: listsEpoch.Add(time.Hour), ActiveCount: intPtr(9),
		DeletedAt: &listsEpoch,
	}}, nil)
	f.lists.EXPECT().GetListStats(gomock.Any(), testWorkspaceID, "news").Return(&domain.ListStats{
		TotalActive: 1250, TotalPending: 12, TotalUnsubscribed: 80, TotalBounced: 4, TotalComplained: 1,
	}, nil)

	result := f.callTool("list_lists", map[string]any{"workspace_id": testWorkspaceID})
	require.False(t, result.IsError, string(result.StructuredContent))
	assert.JSONEq(t, `{
		"workspace":{"id":"ws1","name":"Acme"},
		"lists":[{
			"id":"news","name":"Newsletter","description":"Monthly news","is_public":true,"is_double_optin":true,
			"double_optin_template":{"id":"doi","version":2},
			"stats":{"active":1250,"pending":12,"unsubscribed":80,"bounced":4,"complained":1},
			"created_at":"2026-10-01T09:00:00Z","updated_at":"2026-10-01T10:00:00Z"
		}]
	}`, string(result.StructuredContent))
	for _, dropped := range []string{"active_count", "deleted_at", "total_active"} {
		assert.NotContains(t, result.Content[0].Text, dropped)
	}
}

func TestListLists_Empty(t *testing.T) {
	f := newFixture(t)
	f.reachable(listsRead())
	f.lists.EXPECT().GetLists(gomock.Any(), testWorkspaceID, false).Return(nil, nil)
	result := f.callTool("list_lists", map[string]any{"workspace_id": testWorkspaceID})
	require.False(t, result.IsError, string(result.StructuredContent))
	assert.JSONEq(t, `{"workspace":{"id":"ws1","name":"Acme"},"lists":[]}`, string(result.StructuredContent))
}

// lists.list returns every list; the tool pages them by offset, newest first
// with the ID breaking ties, and asks stats only for the lists of the page.
func TestListLists_PagesOnAStableOrder(t *testing.T) {
	f := newFixture(t)
	f.reachable(listsRead())
	f.lists.EXPECT().GetLists(gomock.Any(), testWorkspaceID, false).DoAndReturn(func(any, string, bool) ([]*domain.List, error) {
		return threeLists(), nil
	}).Times(2)
	f.lists.EXPECT().GetListStats(gomock.Any(), testWorkspaceID, "b").Return(&domain.ListStats{TotalActive: 2}, nil)
	f.lists.EXPECT().GetListStats(gomock.Any(), testWorkspaceID, "c").Return(&domain.ListStats{TotalActive: 3}, nil)
	f.lists.EXPECT().GetListStats(gomock.Any(), testWorkspaceID, "a").Return(&domain.ListStats{TotalActive: 1}, nil)

	type page struct {
		Lists []struct {
			ID    string `json:"id"`
			Stats struct {
				Active int `json:"active"`
			} `json:"stats"`
		} `json:"lists"`
		NextCursor string `json:"next_cursor"`
	}
	var first page
	f.callTool("list_lists", map[string]any{"workspace_id": testWorkspaceID, "limit": 2}).decode(t, &first)
	require.Len(t, first.Lists, 2)
	assert.Equal(t, "b", first.Lists[0].ID)
	assert.Equal(t, 2, first.Lists[0].Stats.Active)
	assert.Equal(t, "c", first.Lists[1].ID)
	require.NotEmpty(t, first.NextCursor)

	var second page
	f.callTool("list_lists", map[string]any{"workspace_id": testWorkspaceID, "limit": 2, "cursor": first.NextCursor}).decode(t, &second)
	require.Len(t, second.Lists, 1)
	assert.Equal(t, "a", second.Lists[0].ID)
	assert.Empty(t, second.NextCursor, "absent on the last page")
}

func TestListLists_CursorPastTheEndIsAnEmptyPage(t *testing.T) {
	f := newFixture(t)
	f.reachable(listsRead())
	f.lists.EXPECT().GetLists(gomock.Any(), testWorkspaceID, false).Return(threeLists(), nil)
	cursor, err := EncodeCursor("list_lists", testWorkspaceID, nil, 3)
	require.NoError(t, err)

	result := f.callTool("list_lists", map[string]any{"workspace_id": testWorkspaceID, "cursor": cursor})
	require.False(t, result.IsError, string(result.StructuredContent))
	assert.JSONEq(t, `{"workspace":{"id":"ws1","name":"Acme"},"lists":[]}`, string(result.StructuredContent))
}

func TestListLists_RefusesBadArgumentsBeforeTheService(t *testing.T) {
	f := newFixture(t)
	f.reachable(listsRead())
	negative, err := EncodeCursor("list_lists", testWorkspaceID, nil, -1)
	require.NoError(t, err)
	notAnOffset, err := EncodeCursor("list_lists", testWorkspaceID, nil, "2")
	require.NoError(t, err)
	otherWorkspace, err := EncodeCursor("list_lists", "ws2", nil, 2)
	require.NoError(t, err)
	otherTool, err := EncodeCursor("search_contacts", testWorkspaceID, nil, 2)
	require.NoError(t, err)

	for name, args := range map[string]map[string]any{
		"limit above 50":            {"limit": 51},
		"limit of 0":                {"limit": 0},
		"garbage cursor":            {"cursor": "%%%"},
		"negative offset":           {"cursor": negative},
		"offset that is a string":   {"cursor": notAnOffset},
		"another workspace's page":  {"cursor": otherWorkspace},
		"another tool's cursor":     {"cursor": otherTool},
		"a filter it does not take": {"search": "news"},
	} {
		t.Run(name, func(t *testing.T) {
			args["workspace_id"] = testWorkspaceID
			assert.Equal(t, CodeValidation, f.callTool("list_lists", args).errorOf(t).Code)
		})
	}
}

func TestListLists_CapsFreeText(t *testing.T) {
	f := newFixture(t)
	f.reachable(listsRead())
	f.lists.EXPECT().GetLists(gomock.Any(), testWorkspaceID, false).Return([]*domain.List{
		{ID: "long", Name: "Long\x1b", Description: strings.Repeat("d", 2001), CreatedAt: listsEpoch},
	}, nil)
	f.lists.EXPECT().GetListStats(gomock.Any(), testWorkspaceID, "long").Return(&domain.ListStats{}, nil)

	var out struct {
		Lists []map[string]json.RawMessage `json:"lists"`
	}
	f.callTool("list_lists", map[string]any{"workspace_id": testWorkspaceID}).decode(t, &out)
	require.Len(t, out.Lists, 1)
	assert.Equal(t, `"Long"`, string(out.Lists[0]["name"]))
	assert.Equal(t, `"`+strings.Repeat("d", 2000)+`"`, string(out.Lists[0]["description"]))
	assert.JSONEq(t, `["description"]`, string(out.Lists[0]["truncated_fields"]))
}

func TestListLists_ErrorsAreMapped(t *testing.T) {
	t.Run("a failed stats read", func(t *testing.T) {
		f := newFixture(t)
		f.reachable(listsRead())
		f.lists.EXPECT().GetLists(gomock.Any(), testWorkspaceID, false).Return(threeLists(), nil)
		f.lists.EXPECT().GetListStats(gomock.Any(), testWorkspaceID, "b").Return(nil, errors.New("failed to get list stats: pq on db-host-7"))
		assert.Equal(t, ToolError{Code: CodeInternal, Message: "internal error"}, f.callTool("list_lists", map[string]any{"workspace_id": testWorkspaceID}).errorOf(t))
	})

	t.Run("a permission refusal of the service", func(t *testing.T) {
		f := newFixture(t)
		f.reachable(listsRead())
		f.lists.EXPECT().GetLists(gomock.Any(), testWorkspaceID, false).
			Return(nil, domain.NewPermissionError(domain.PermissionResourceLists, domain.PermissionTypeRead, "Insufficient permissions: read access to lists required"))
		toolErr := f.callTool("list_lists", map[string]any{"workspace_id": testWorkspaceID}).errorOf(t)
		assert.Equal(t, CodePermissionDenied, toolErr.Code)
		assert.Equal(t, "lists", toolErr.Resource)
	})

	t.Run("a key without lists:read", func(t *testing.T) {
		f := newFixture(t)
		f.reachable(domain.UserPermissions{domain.PermissionResourceLists: {Write: true}})
		toolErr := f.callTool("list_lists", map[string]any{"workspace_id": testWorkspaceID}).errorOf(t)
		assert.Equal(t, ToolError{Code: CodePermissionDenied, Message: "this API key lacks the lists read permission", Resource: "lists", Permission: "read"}, toolErr)
	})

	t.Run("a switched-off workspace", func(t *testing.T) {
		f := newFixture(t)
		f.switchedOff()
		assert.Equal(t, ToolError{Code: CodeNotFound, Message: "workspace not found"}, f.callTool("list_lists", map[string]any{"workspace_id": testWorkspaceID}).errorOf(t))
	})
}
