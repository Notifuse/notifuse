package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
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

func listsWrite() domain.UserPermissions {
	return domain.UserPermissions{domain.PermissionResourceLists: {Write: true}}
}

func listsReadWrite() domain.UserPermissions {
	return domain.UserPermissions{domain.PermissionResourceLists: {Read: true, Write: true}}
}

func TestCreateList_Definition(t *testing.T) {
	assertWriteTool(t, catalogueTool(t, "create_list"), "create_list", ClassWrite, writeHints{destructive: false, openWorld: false, idempotent: false},
		[]Pair{{Resource: "lists", Verb: "write"}}, "lists.create", []string{"workspace_id", "id", "name"})
}

func TestCreateList_BuildsTheList(t *testing.T) {
	f := newFixture(t)
	f.reachable(listsWrite())
	audit := f.captureAudit()
	var sent *domain.List
	f.lists.EXPECT().CreateList(gomock.Any(), testWorkspaceID, gomock.Any()).DoAndReturn(func(_ context.Context, _ string, list *domain.List) error {
		sent = list
		list.CreatedAt, list.UpdatedAt = listsEpoch, listsEpoch
		return nil
	})

	result := f.callTool("create_list", map[string]any{
		"workspace_id": testWorkspaceID, "id": "news", "name": "Newsletter", "is_double_optin": true,
		"double_optin_template_id": "doi",
	})
	require.False(t, result.IsError, string(result.StructuredContent))
	assert.Equal(t, &domain.List{
		ID: "news", Name: "Newsletter", IsDoubleOptin: true, IsPublic: false,
		DoubleOptInTemplate: &domain.TemplateReference{ID: "doi", Version: 0},
		CreatedAt:           listsEpoch, UpdatedAt: listsEpoch,
	}, sent)
	assert.JSONEq(t, `{"workspace":{"id":"ws1","name":"Acme"},"list":{
		"id":"news","name":"Newsletter","is_public":false,"is_double_optin":true,"double_optin_template":{"id":"doi","version":0},
		"created_at":"2026-10-01T09:00:00Z","updated_at":"2026-10-01T09:00:00Z"
	}}`, string(result.StructuredContent))

	row := audit.last(t)
	assert.Equal(t, "lists.create", row.Action)
	assert.Equal(t, domain.AuditCategoryLists, row.Category)
	assert.Equal(t, domain.AuditTargetList, row.TargetType)
	assert.Equal(t, "news", row.TargetID)
	assert.Equal(t, "Newsletter", row.TargetName)
	assert.Equal(t, http.StatusOK, row.StatusCode)
}

// The ID is the caller's, so a repeated create meets the list it made.
func TestCreateList_ReplayIsAConflict(t *testing.T) {
	for name, tc := range map[string]struct {
		deleted bool
		message string
	}{
		"a live list":    {deleted: false, message: "list news already exists"},
		"a deleted list": {deleted: true, message: "list news already exists and was deleted: its identifier cannot be reused"},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			f.reachable(listsWrite())
			audit := f.captureAudit()
			f.lists.EXPECT().CreateList(gomock.Any(), testWorkspaceID, gomock.Any()).
				Return(fmt.Errorf("failed to create list: %w", &domain.ErrListExists{Message: "list id already exists", Deleted: tc.deleted}))

			toolErr := f.callTool("create_list", map[string]any{"workspace_id": testWorkspaceID, "id": "news", "name": "Newsletter"}).errorOf(t)
			assert.Equal(t, ToolError{Code: CodeConflict, Message: tc.message}, toolErr)
			assert.Equal(t, http.StatusConflict, audit.last(t).StatusCode)
		})
	}
}

func TestCreateList_RefusesBadArgumentsBeforeTheService(t *testing.T) {
	f := newFixture(t)
	f.reachable(listsWrite())
	f.captureAudit()
	for name, args := range map[string]map[string]any{
		"id not alphanumeric":            {"id": "news-letter", "name": "Newsletter"},
		"id too long":                    {"id": strings.Repeat("a", 33), "name": "Newsletter"},
		"double opt-in without template": {"id": "news", "name": "Newsletter", "is_double_optin": true},
		"version without template":       {"id": "news", "name": "Newsletter", "double_optin_template_version": 2},
		"negative version":               {"id": "news", "name": "Newsletter", "double_optin_template_id": "doi", "double_optin_template_version": -1},
	} {
		t.Run(name, func(t *testing.T) {
			args["workspace_id"] = testWorkspaceID
			assert.Equal(t, CodeValidation, f.callTool("create_list", args).errorOf(t).Code)
		})
	}
}

func TestUpdateList_Definition(t *testing.T) {
	assertWriteTool(t, catalogueTool(t, "update_list"), "update_list", ClassWriteOpenWorld, writeHints{destructive: false, openWorld: true, idempotent: true},
		[]Pair{{Resource: "lists", Verb: "write"}, {Resource: "lists", Verb: "read"}}, "lists.update", []string{"workspace_id", "id"})
}

func storedList() *domain.List {
	return &domain.List{
		ID: "news", Name: "Newsletter", Description: "Monthly news", IsPublic: true, IsDoubleOptin: true,
		DoubleOptInTemplate: &domain.TemplateReference{ID: "doi", Version: 3},
		CreatedAt:           listsEpoch, UpdatedAt: listsEpoch,
	}
}

// lists.update replaces the whole list: the tool reads it, changes the given
// fields and writes everything back, so a field left out keeps its value.
func TestUpdateList_KeepsTheFieldsNotGiven(t *testing.T) {
	for name, tc := range map[string]struct {
		args map[string]any
		want domain.List
	}{
		"a rename": {
			args: map[string]any{"name": "Monthly"},
			want: domain.List{ID: "news", Name: "Monthly", Description: "Monthly news", IsPublic: true, IsDoubleOptin: true, DoubleOptInTemplate: &domain.TemplateReference{ID: "doi", Version: 3}},
		},
		"an empty description": {
			args: map[string]any{"description": ""},
			want: domain.List{ID: "news", Name: "Newsletter", IsPublic: true, IsDoubleOptin: true, DoubleOptInTemplate: &domain.TemplateReference{ID: "doi", Version: 3}},
		},
		"double opt-in off, template removed": {
			args: map[string]any{"is_double_optin": false, "remove_double_optin_template": true},
			want: domain.List{ID: "news", Name: "Newsletter", Description: "Monthly news", IsPublic: true, IsDoubleOptin: false},
		},
		"another template": {
			args: map[string]any{"double_optin_template_id": "doi2"},
			want: domain.List{ID: "news", Name: "Newsletter", Description: "Monthly news", IsPublic: true, IsDoubleOptin: true, DoubleOptInTemplate: &domain.TemplateReference{ID: "doi2", Version: 0}},
		},
		"another version of the same template": {
			args: map[string]any{"double_optin_template_version": 4},
			want: domain.List{ID: "news", Name: "Newsletter", Description: "Monthly news", IsPublic: true, IsDoubleOptin: true, DoubleOptInTemplate: &domain.TemplateReference{ID: "doi", Version: 4}},
		},
		"private": {
			args: map[string]any{"is_public": false},
			want: domain.List{ID: "news", Name: "Newsletter", Description: "Monthly news", IsPublic: false, IsDoubleOptin: true, DoubleOptInTemplate: &domain.TemplateReference{ID: "doi", Version: 3}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			f.reachable(listsReadWrite())
			f.captureAudit()
			f.lists.EXPECT().GetListByID(gomock.Any(), testWorkspaceID, "news").Return(storedList(), nil)
			var sent *domain.List
			f.lists.EXPECT().UpdateList(gomock.Any(), testWorkspaceID, gomock.Any()).DoAndReturn(func(_ context.Context, _ string, list *domain.List) error {
				sent = list
				return nil
			})

			tc.args["workspace_id"], tc.args["id"] = testWorkspaceID, "news"
			result := f.callTool("update_list", tc.args)
			require.False(t, result.IsError, string(result.StructuredContent))
			require.NotNil(t, sent)
			sent.CreatedAt, sent.UpdatedAt = time.Time{}, time.Time{}
			assert.Equal(t, tc.want, *sent)
		})
	}
}

func TestUpdateList_ReturnsTheListAsWritten(t *testing.T) {
	f := newFixture(t)
	f.reachable(listsReadWrite())
	audit := f.captureAudit()
	f.lists.EXPECT().GetListByID(gomock.Any(), testWorkspaceID, "news").Return(storedList(), nil)
	f.lists.EXPECT().UpdateList(gomock.Any(), testWorkspaceID, gomock.Any()).DoAndReturn(func(_ context.Context, _ string, list *domain.List) error {
		list.UpdatedAt = listsEpoch.Add(time.Hour)
		return nil
	})

	result := f.callTool("update_list", map[string]any{"workspace_id": testWorkspaceID, "id": "news", "name": "Monthly"})
	require.False(t, result.IsError, string(result.StructuredContent))
	assert.JSONEq(t, `{"workspace":{"id":"ws1","name":"Acme"},"list":{
		"id":"news","name":"Monthly","description":"Monthly news","is_public":true,"is_double_optin":true,
		"double_optin_template":{"id":"doi","version":3},
		"created_at":"2026-10-01T09:00:00Z","updated_at":"2026-10-01T10:00:00Z"
	}}`, string(result.StructuredContent))
	row := audit.last(t)
	assert.Equal(t, "lists.update", row.Action)
	assert.Equal(t, domain.AuditTargetList, row.TargetType)
	assert.Equal(t, "news", row.TargetID)
	assert.Equal(t, "Monthly", row.TargetName)
	assert.Equal(t, http.StatusOK, row.StatusCode)
}

func TestUpdateList_RefusesBadArgumentsBeforeTheWrite(t *testing.T) {
	for name, tc := range map[string]struct {
		stored *domain.List
		args   map[string]any
	}{
		"remove the template while double opt-in stays on": {stored: storedList(), args: map[string]any{"remove_double_optin_template": true}},
		"remove and set a template":                        {stored: storedList(), args: map[string]any{"remove_double_optin_template": true, "double_optin_template_id": "doi2"}},
		"a version without any template": {
			stored: &domain.List{ID: "news", Name: "Newsletter"},
			args:   map[string]any{"double_optin_template_version": 2},
		},
		"an empty name": {stored: storedList(), args: map[string]any{"name": ""}},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			f.reachable(listsReadWrite())
			f.captureAudit()
			f.lists.EXPECT().GetListByID(gomock.Any(), testWorkspaceID, "news").Return(tc.stored, nil).MaxTimes(1)
			tc.args["workspace_id"], tc.args["id"] = testWorkspaceID, "news"
			assert.Equal(t, CodeValidation, f.callTool("update_list", tc.args).errorOf(t).Code)
		})
	}
}

func TestUpdateList_UnknownListIsNotFound(t *testing.T) {
	f := newFixture(t)
	f.reachable(listsReadWrite())
	audit := f.captureAudit()
	f.lists.EXPECT().GetListByID(gomock.Any(), testWorkspaceID, "gone").Return(nil, &domain.ErrListNotFound{Message: "list not found"})

	toolErr := f.callTool("update_list", map[string]any{"workspace_id": testWorkspaceID, "id": "gone", "name": "X"}).errorOf(t)
	assert.Equal(t, CodeNotFound, toolErr.Code)
	assert.Equal(t, http.StatusNotFound, audit.last(t).StatusCode)
}

// The read costs lists:read, which the pairs include: a key with write alone
// is refused before anything is read.
func TestUpdateList_NeedsTheReadPair(t *testing.T) {
	f := newFixture(t)
	f.reachable(listsWrite())
	audit := f.captureAudit()
	toolErr := f.callTool("update_list", map[string]any{"workspace_id": testWorkspaceID, "id": "news", "name": "X"}).errorOf(t)
	assert.Equal(t, ToolError{Code: CodePermissionDenied, Message: "this API key lacks the lists read permission", Resource: "lists", Permission: "read"}, toolErr)
	assert.Equal(t, http.StatusForbidden, audit.last(t).StatusCode)
}

func TestDeleteList_Definition(t *testing.T) {
	assertWriteTool(t, catalogueTool(t, "delete_list"), "delete_list", ClassDestructive, writeHints{destructive: true, openWorld: false, idempotent: true},
		[]Pair{{Resource: "lists", Verb: "write"}}, "lists.delete", []string{"workspace_id", "id"})
}

func TestDeleteList(t *testing.T) {
	f := newFixture(t)
	f.reachable(listsWrite())
	audit := f.captureAudit()
	f.lists.EXPECT().DeleteList(gomock.Any(), testWorkspaceID, "news").Return(nil)

	result := f.callTool("delete_list", map[string]any{"workspace_id": testWorkspaceID, "id": "news"})
	require.False(t, result.IsError, string(result.StructuredContent))
	assert.JSONEq(t, `{"workspace":{"id":"ws1","name":"Acme"},"deleted":true,"id":"news"}`, string(result.StructuredContent))
	row := audit.last(t)
	assert.Equal(t, "lists.delete", row.Action)
	assert.Equal(t, domain.AuditTargetList, row.TargetType)
	assert.Equal(t, "news", row.TargetID)
	assert.Equal(t, http.StatusOK, row.StatusCode)
}

func TestDeleteList_RefusalsAreMapped(t *testing.T) {
	for name, tc := range map[string]struct {
		err    error
		want   ToolError
		status int
	}{
		// The repository answers not found for a list already deleted.
		"a replay": {
			err:    fmt.Errorf("failed to delete list: %w", &domain.ErrListNotFound{Message: "list not found or already deleted"}),
			want:   ToolError{Code: CodeNotFound, Message: "list not found or already deleted"},
			status: http.StatusNotFound,
		},
		"a list live automations use": {
			err: fmt.Errorf("failed to delete list: %w", domain.NewErrListInUse("news", "Newsletter", []domain.AutomationRef{{ID: "a1", Name: "Welcome"}})),
			want: ToolError{Code: CodeConflict,
				Message: `cannot delete list "Newsletter" while the live automation "Welcome" uses it: pause the automation first, or change it to stop using the list`},
			status: http.StatusConflict,
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			f.reachable(listsWrite())
			audit := f.captureAudit()
			f.lists.EXPECT().DeleteList(gomock.Any(), testWorkspaceID, "news").Return(tc.err)
			assert.Equal(t, tc.want, f.callTool("delete_list", map[string]any{"workspace_id": testWorkspaceID, "id": "news"}).errorOf(t))
			assert.Equal(t, tc.status, audit.last(t).StatusCode)
		})
	}
}

func TestSubscribeContactToList_Definition(t *testing.T) {
	tool := catalogueTool(t, "subscribe_contact_to_list")
	assertWriteTool(t, tool, "subscribe_contact_to_list", ClassWriteOpenWorld, writeHints{destructive: false, openWorld: true, idempotent: true},
		[]Pair{{Resource: "lists", Verb: "write"}, {Resource: "contacts", Verb: "write"}}, "lists.subscribe", []string{"workspace_id", "email", "list_id"})
	resubscribe := tool.InputSchema["properties"].(map[string]any)["resubscribe"].(map[string]any)
	assert.Equal(t, "boolean", resubscribe["type"])
	assert.Contains(t, resubscribe["description"], "only when the contact has given their consent again")
}

// membershipRow is a contact's row on a list as the repository reads it.
func membershipRow(status domain.ContactListStatus, removed bool) *domain.ContactList {
	row := &domain.ContactList{Email: "jane@example.com", ListID: "news", Status: status, CreatedAt: listsEpoch, UpdatedAt: listsEpoch}
	if removed {
		row.DeletedAt = &listsEpoch
	}
	return row
}

// The call is lists.subscribe's: an authenticated caller, the email
// normalized, one list, and resubscribe passed as given, false by default.
func TestSubscribeContactToList_BuildsTheRequest(t *testing.T) {
	for name, tc := range map[string]struct {
		args        map[string]any
		resubscribe bool
	}{
		"without resubscribe": {args: map[string]any{}, resubscribe: false},
		"resubscribe false":   {args: map[string]any{"resubscribe": false}, resubscribe: false},
		"resubscribe true":    {args: map[string]any{"resubscribe": true}, resubscribe: true},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			f.reachable(listsAndContactsWrite())
			f.captureAudit()
			f.memberships.EXPECT().GetContactListIncludingRemoved(gomock.Any(), testWorkspaceID, "jane@example.com", "news").
				Return(nil, &domain.ErrContactListNotFound{Message: "contact list not found"})
			f.subscribe.respond = func(*domain.SubscribeToListsRequest) ([]*domain.ContactList, error) {
				return []*domain.ContactList{{Email: "jane@example.com", ListID: "news", ListName: "Newsletter", Status: domain.ContactListStatusActive}}, nil
			}

			tc.args["workspace_id"], tc.args["email"], tc.args["list_id"] = testWorkspaceID, "Jane@Example.com", "news"
			require.False(t, f.callTool("subscribe_contact_to_list", tc.args).IsError)
			require.Len(t, f.subscribe.requests, 1)
			sent := f.subscribe.requests[0]
			assert.Equal(t, []bool{true}, f.subscribe.bearer)
			assert.Equal(t, testWorkspaceID, sent.WorkspaceID)
			assert.Equal(t, "jane@example.com", sent.Contact.Email)
			assert.Equal(t, []string{"news"}, sent.ListIDs)
			assert.Equal(t, tc.resubscribe, sent.Resubscribe)
			// Only the email: field writes belong to upsert_contact.
			assert.Nil(t, sent.Contact.FirstName)
			assert.Nil(t, sent.Contact.TrackingOpensConsent)
		})
	}
}

// A new or pending membership becomes active, with no confirmation email: the
// service is called as an authenticated caller, which never sends one there.
func TestSubscribeContactToList_ActivatesANewOrPendingMembership(t *testing.T) {
	for name, tc := range map[string]struct {
		stored   *domain.ContactList
		previous string
	}{
		"new":            {stored: nil, previous: "none"},
		"pending":        {stored: membershipRow(domain.ContactListStatusPending, false), previous: "pending"},
		"removed active": {stored: membershipRow(domain.ContactListStatusActive, true), previous: "none"},
		"already active": {stored: membershipRow(domain.ContactListStatusActive, false), previous: "active"},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			f.reachable(listsAndContactsWrite())
			audit := f.captureAudit()
			var readErr error
			if tc.stored == nil {
				readErr = &domain.ErrContactListNotFound{Message: "contact list not found"}
			}
			f.memberships.EXPECT().GetContactListIncludingRemoved(gomock.Any(), testWorkspaceID, "jane@example.com", "news").Return(tc.stored, readErr)
			f.subscribe.respond = func(*domain.SubscribeToListsRequest) ([]*domain.ContactList, error) {
				return []*domain.ContactList{{Email: "jane@example.com", ListID: "news", ListName: "Newsletter", Status: domain.ContactListStatusActive}}, nil
			}

			result := f.callTool("subscribe_contact_to_list", map[string]any{"workspace_id": testWorkspaceID, "email": "jane@example.com", "list_id": "news"})
			require.False(t, result.IsError, string(result.StructuredContent))
			assert.JSONEq(t, fmt.Sprintf(`{"workspace":{"id":"ws1","name":"Acme"},"email":"jane@example.com","list_id":"news","list_name":"Newsletter",
				"status":"active","previous_status":%q}`, tc.previous), string(result.StructuredContent))

			row := audit.last(t)
			assert.Equal(t, "lists.subscribe", row.Action)
			assert.Equal(t, domain.AuditCategoryContacts, row.Category)
			assert.Equal(t, domain.AuditTargetContact, row.TargetType)
			assert.Equal(t, "jane@example.com", row.TargetID)
			assert.Equal(t, http.StatusOK, row.StatusCode)
			assert.Equal(t, map[string]any{
				"channel": "mcp", "mcp_tool": "subscribe_contact_to_list",
				"email": "jane@example.com", "list_id": "news", "previous_status": tc.previous, "new_status": "active",
			}, row.Metadata)
		})
	}
}

// D30: without resubscribe an unsubscribed contact stays so, and the tool
// answers with its status and a message naming the argument. A bounce or a
// complaint never changes.
func TestSubscribeContactToList_OptedOutContactsStayOptedOut(t *testing.T) {
	for name, tc := range map[string]struct {
		status  domain.ContactListStatus
		message string
	}{
		"unsubscribed": {status: domain.ContactListStatusUnsubscribed, message: "the contact unsubscribed from this list: set resubscribe to true to subscribe them again"},
		"bounced":      {status: domain.ContactListStatusBounced, message: "the contact's address bounced on this list: only a console user can re-subscribe them"},
		"complained":   {status: domain.ContactListStatusComplained, message: "the contact is complained on this list, which cannot be changed"},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			f.reachable(listsAndContactsWrite())
			audit := f.captureAudit()
			f.memberships.EXPECT().GetContactListIncludingRemoved(gomock.Any(), testWorkspaceID, "jane@example.com", "news").Return(membershipRow(tc.status, false), nil)
			f.subscribe.respond = func(*domain.SubscribeToListsRequest) ([]*domain.ContactList, error) {
				return []*domain.ContactList{membershipRow(tc.status, false)}, nil
			}

			toolErr := f.callTool("subscribe_contact_to_list", map[string]any{"workspace_id": testWorkspaceID, "email": "jane@example.com", "list_id": "news"}).errorOf(t)
			assert.Equal(t, ToolError{Code: CodeInvalidState, Message: tc.message, Status: string(tc.status)}, toolErr)
			row := audit.last(t)
			assert.Equal(t, http.StatusConflict, row.StatusCode)
			assert.Equal(t, string(tc.status), row.Metadata["previous_status"])
			assert.Equal(t, string(tc.status), row.Metadata["new_status"])
		})
	}
}

// With resubscribe the service brings an unsubscribed contact back and marks
// the record; the row says so.
func TestSubscribeContactToList_ResubscribeIsAudited(t *testing.T) {
	f := newFixture(t)
	f.reachable(listsAndContactsWrite())
	audit := f.captureAudit()
	f.memberships.EXPECT().GetContactListIncludingRemoved(gomock.Any(), testWorkspaceID, "jane@example.com", "news").
		Return(membershipRow(domain.ContactListStatusUnsubscribed, true), nil)
	f.subscribe.respond = func(req *domain.SubscribeToListsRequest) ([]*domain.ContactList, error) {
		require.True(t, req.Resubscribe)
		return []*domain.ContactList{{Email: "jane@example.com", ListID: "news", ListName: "Newsletter", Status: domain.ContactListStatusActive}}, nil
	}

	result := f.callTool("subscribe_contact_to_list", map[string]any{"workspace_id": testWorkspaceID, "email": "jane@example.com", "list_id": "news", "resubscribe": true})
	require.False(t, result.IsError, string(result.StructuredContent))
	row := audit.last(t)
	assert.Equal(t, "unsubscribed", row.Metadata["previous_status"])
	assert.Equal(t, "active", row.Metadata["new_status"])
	assert.Equal(t, true, row.Metadata["resubscribe"])
}

func TestSubscribeContactToList_DisposableAddressIsDropped(t *testing.T) {
	f := newFixture(t)
	f.reachable(listsAndContactsWrite())
	f.captureAudit()
	f.memberships.EXPECT().GetContactListIncludingRemoved(gomock.Any(), testWorkspaceID, "jane@mailinator.com", "news").
		Return(nil, &domain.ErrContactListNotFound{Message: "contact list not found"})
	f.subscribe.respond = func(*domain.SubscribeToListsRequest) ([]*domain.ContactList, error) {
		return []*domain.ContactList{}, nil
	}

	result := f.callTool("subscribe_contact_to_list", map[string]any{"workspace_id": testWorkspaceID, "email": "jane@mailinator.com", "list_id": "news"})
	require.False(t, result.IsError, string(result.StructuredContent))
	assert.JSONEq(t, `{"workspace":{"id":"ws1","name":"Acme"},"email":"jane@mailinator.com","list_id":"news","previous_status":"none","dropped":true}`,
		string(result.StructuredContent))
}

func TestSubscribeContactToList_ErrorsAreMapped(t *testing.T) {
	f := newFixture(t)
	f.reachable(listsAndContactsWrite())
	audit := f.captureAudit()
	f.memberships.EXPECT().GetContactListIncludingRemoved(gomock.Any(), testWorkspaceID, "jane@example.com", "gone").
		Return(nil, &domain.ErrContactListNotFound{Message: "contact list not found"})
	f.subscribe.respond = func(*domain.SubscribeToListsRequest) ([]*domain.ContactList, error) {
		return nil, &domain.ErrListNotFound{Message: "list gone not found"}
	}
	toolErr := f.callTool("subscribe_contact_to_list", map[string]any{"workspace_id": testWorkspaceID, "email": "jane@example.com", "list_id": "gone"}).errorOf(t)
	assert.Equal(t, ToolError{Code: CodeNotFound, Message: "list gone not found"}, toolErr)
	assert.Equal(t, http.StatusNotFound, audit.last(t).StatusCode)

	assert.Equal(t, CodeValidation, f.callTool("subscribe_contact_to_list", map[string]any{"workspace_id": testWorkspaceID, "email": "jane", "list_id": "news"}).errorOf(t).Code)
	assert.Empty(t, f.subscribe.requests[1:], "an invalid email never reaches the service")
}

// contacts:write is checked by the pre-check alone: the service creates the
// contact while checking lists:write only.
func TestSubscribeContactToList_NeedsContactsWrite(t *testing.T) {
	f := newFixture(t)
	f.reachable(listsWrite())
	audit := f.captureAudit()
	toolErr := f.callTool("subscribe_contact_to_list", map[string]any{"workspace_id": testWorkspaceID, "email": "jane@example.com", "list_id": "news"}).errorOf(t)
	assert.Equal(t, ToolError{Code: CodePermissionDenied, Message: "this API key lacks the contacts write permission", Resource: "contacts", Permission: "write"}, toolErr)
	assert.Empty(t, f.subscribe.requests)
	assert.Equal(t, http.StatusForbidden, audit.last(t).StatusCode)
}

func TestSetContactListStatus_Definition(t *testing.T) {
	tool := catalogueTool(t, "set_contact_list_status")
	assertWriteTool(t, tool, "set_contact_list_status", ClassWriteOpenWorld, writeHints{destructive: false, openWorld: true, idempotent: true},
		[]Pair{{Resource: "lists", Verb: "write"}, {Resource: "contacts", Verb: "write"}}, "contactLists.updateStatus", []string{"workspace_id", "email", "list_id", "status"})
	properties := tool.InputSchema["properties"].(map[string]any)
	assert.Equal(t, []string{"active", "pending", "unsubscribed", "bounced", "complained"}, properties["status"].(map[string]any)["enum"])
	assert.Contains(t, properties["resubscribe"].(map[string]any)["description"], "only when the contact has given their consent again")
}

func TestSetContactListStatus_PassesResubscribeAsGiven(t *testing.T) {
	for name, tc := range map[string]struct {
		args        map[string]any
		resubscribe bool
	}{
		"without resubscribe": {args: map[string]any{}, resubscribe: false},
		"resubscribe true":    {args: map[string]any{"resubscribe": true}, resubscribe: true},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			f.reachable(listsAndContactsWrite())
			audit := f.captureAudit()
			f.memberships.EXPECT().GetContactListIncludingRemoved(gomock.Any(), testWorkspaceID, "jane@example.com", "news").
				Return(membershipRow(domain.ContactListStatusPending, false), nil)
			f.contactList.EXPECT().UpdateContactListStatus(gomock.Any(), testWorkspaceID, "jane@example.com", "news", domain.ContactListStatusActive, tc.resubscribe).
				Return(&domain.UpdateContactListStatusResult{Success: true, Found: true}, nil)

			tc.args["workspace_id"], tc.args["email"], tc.args["list_id"], tc.args["status"] = testWorkspaceID, "JANE@example.com", "news", "active"
			result := f.callTool("set_contact_list_status", tc.args)
			require.False(t, result.IsError, string(result.StructuredContent))
			assert.JSONEq(t, `{"workspace":{"id":"ws1","name":"Acme"},"email":"jane@example.com","list_id":"news","status":"active","previous_status":"pending"}`,
				string(result.StructuredContent))
			row := audit.last(t)
			assert.Equal(t, "contactLists.updateStatus", row.Action)
			assert.Equal(t, domain.AuditCategoryContacts, row.Category)
			assert.Equal(t, domain.AuditTargetContact, row.TargetType)
			assert.Equal(t, "jane@example.com", row.TargetID)
			assert.Equal(t, http.StatusOK, row.StatusCode)
			assert.Equal(t, "pending", row.Metadata["previous_status"])
			assert.Equal(t, "active", row.Metadata["new_status"])
			assert.Equal(t, "news", row.Metadata["list_id"])
		})
	}
}

// The service refuses to take a contact out of an opt-out without
// resubscribe, and never out of a complaint: invalid_state with the status.
func TestSetContactListStatus_OptOutRefusal(t *testing.T) {
	f := newFixture(t)
	f.reachable(listsAndContactsWrite())
	audit := f.captureAudit()
	f.memberships.EXPECT().GetContactListIncludingRemoved(gomock.Any(), testWorkspaceID, "jane@example.com", "news").
		Return(membershipRow(domain.ContactListStatusUnsubscribed, false), nil)
	f.contactList.EXPECT().UpdateContactListStatus(gomock.Any(), testWorkspaceID, "jane@example.com", "news", domain.ContactListStatusActive, false).
		Return(nil, &domain.ErrContactListOptedOut{Status: domain.ContactListStatusUnsubscribed})

	toolErr := f.callTool("set_contact_list_status", map[string]any{"workspace_id": testWorkspaceID, "email": "jane@example.com", "list_id": "news", "status": "active"}).errorOf(t)
	assert.Equal(t, ToolError{Code: CodeInvalidState, Message: "the contact unsubscribed from this list: set resubscribe to true to subscribe them again", Status: "unsubscribed"}, toolErr)
	row := audit.last(t)
	assert.Equal(t, http.StatusConflict, row.StatusCode)
	assert.Equal(t, "unsubscribed", row.Metadata["previous_status"])
	assert.NotContains(t, row.Metadata, "new_status")
}

// A contact not on the list answers not_found, where the service answers
// success with found false.
func TestSetContactListStatus_NotOnTheList(t *testing.T) {
	for name, tc := range map[string]struct {
		stored  *domain.ContactList
		readErr error
		found   *bool
	}{
		"no row":            {readErr: &domain.ErrContactListNotFound{Message: "contact list not found"}},
		"a removed row":     {stored: membershipRow(domain.ContactListStatusActive, true)},
		"removed meanwhile": {stored: membershipRow(domain.ContactListStatusActive, false), found: boolPtr(false)},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			f.reachable(listsAndContactsWrite())
			audit := f.captureAudit()
			f.memberships.EXPECT().GetContactListIncludingRemoved(gomock.Any(), testWorkspaceID, "jane@example.com", "news").Return(tc.stored, tc.readErr)
			if tc.found != nil {
				f.contactList.EXPECT().UpdateContactListStatus(gomock.Any(), testWorkspaceID, "jane@example.com", "news", domain.ContactListStatusUnsubscribed, false).
					Return(&domain.UpdateContactListStatusResult{Success: true, Message: "contact not in list", Found: *tc.found}, nil)
			}
			toolErr := f.callTool("set_contact_list_status", map[string]any{"workspace_id": testWorkspaceID, "email": "jane@example.com", "list_id": "news", "status": "unsubscribed"}).errorOf(t)
			assert.Equal(t, ToolError{Code: CodeNotFound, Message: "the contact is not on this list"}, toolErr)
			assert.Equal(t, http.StatusNotFound, audit.last(t).StatusCode)
		})
	}
}

func TestSetContactListStatus_ResubscribeOnlyWithActive(t *testing.T) {
	f := newFixture(t)
	f.reachable(listsAndContactsWrite())
	f.captureAudit()
	toolErr := f.callTool("set_contact_list_status", map[string]any{
		"workspace_id": testWorkspaceID, "email": "jane@example.com", "list_id": "news", "status": "pending", "resubscribe": true,
	}).errorOf(t)
	assert.Equal(t, CodeValidation, toolErr.Code)
}

func TestRemoveContactFromList_Definition(t *testing.T) {
	assertWriteTool(t, catalogueTool(t, "remove_contact_from_list"), "remove_contact_from_list", ClassDestructive, writeHints{destructive: true, openWorld: false, idempotent: true},
		[]Pair{{Resource: "lists", Verb: "write"}, {Resource: "contacts", Verb: "write"}}, "contactLists.removeContact", []string{"workspace_id", "email", "list_id"})
}

func TestRemoveContactFromList(t *testing.T) {
	f := newFixture(t)
	f.reachable(listsAndContactsWrite())
	audit := f.captureAudit()
	f.memberships.EXPECT().GetContactListIncludingRemoved(gomock.Any(), testWorkspaceID, "jane@example.com", "news").
		Return(membershipRow(domain.ContactListStatusUnsubscribed, false), nil)
	f.contactList.EXPECT().RemoveContactFromList(gomock.Any(), testWorkspaceID, "jane@example.com", "news").Return(nil)

	result := f.callTool("remove_contact_from_list", map[string]any{"workspace_id": testWorkspaceID, "email": "Jane@Example.com", "list_id": "news"})
	require.False(t, result.IsError, string(result.StructuredContent))
	assert.JSONEq(t, `{"workspace":{"id":"ws1","name":"Acme"},"removed":true,"email":"jane@example.com","list_id":"news","status":"unsubscribed"}`,
		string(result.StructuredContent))
	row := audit.last(t)
	assert.Equal(t, "contactLists.removeContact", row.Action)
	assert.Equal(t, domain.AuditCategoryContacts, row.Category)
	assert.Equal(t, domain.AuditTargetContact, row.TargetType)
	assert.Equal(t, "jane@example.com", row.TargetID)
	assert.Equal(t, http.StatusOK, row.StatusCode)
	assert.Equal(t, map[string]any{
		"channel": "mcp", "mcp_tool": "remove_contact_from_list", "email": "jane@example.com", "list_id": "news", "previous_status": "unsubscribed",
	}, row.Metadata)
}

// The service answers success for a membership already removed; the tool
// reads first, so a repeated call answers not_found and writes nothing.
func TestRemoveContactFromList_ReplayIsNotFound(t *testing.T) {
	for name, tc := range map[string]struct {
		stored  *domain.ContactList
		readErr error
	}{
		"already removed":   {stored: membershipRow(domain.ContactListStatusActive, true)},
		"never on the list": {readErr: &domain.ErrContactListNotFound{Message: "contact list not found"}},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			f.reachable(listsAndContactsWrite())
			audit := f.captureAudit()
			f.memberships.EXPECT().GetContactListIncludingRemoved(gomock.Any(), testWorkspaceID, "jane@example.com", "news").Return(tc.stored, tc.readErr)
			toolErr := f.callTool("remove_contact_from_list", map[string]any{"workspace_id": testWorkspaceID, "email": "jane@example.com", "list_id": "news"}).errorOf(t)
			assert.Equal(t, ToolError{Code: CodeNotFound, Message: "the contact is not on this list"}, toolErr)
			assert.Equal(t, http.StatusNotFound, audit.last(t).StatusCode)
		})
	}
}

func TestRemoveContactFromList_ErrorsAreMapped(t *testing.T) {
	f := newFixture(t)
	f.reachable(listsAndContactsWrite())
	f.captureAudit()
	f.memberships.EXPECT().GetContactListIncludingRemoved(gomock.Any(), testWorkspaceID, "jane@example.com", "news").Return(nil, errors.New("connection refused"))
	assert.Equal(t, CodeInternal, f.callTool("remove_contact_from_list", map[string]any{"workspace_id": testWorkspaceID, "email": "jane@example.com", "list_id": "news"}).errorOf(t).Code)
}
