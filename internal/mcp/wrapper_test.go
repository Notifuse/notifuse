package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
)

var (
	contactsReadPair  = Pair{Resource: domain.PermissionResourceContacts, Verb: domain.PermissionTypeRead}
	contactsWritePair = Pair{Resource: domain.PermissionResourceContacts, Verb: domain.PermissionTypeWrite}
)

// Authentication runs before the pre-check, and the pre-check before the
// handler: an unreachable workspace answers not_found even to a key that also
// lacks the permission, and a refused pre-check never reaches the handler.
func TestWrapper_AuthenticatesThenPrechecksThenRunsTheHandler(t *testing.T) {
	t.Run("authentication first", func(t *testing.T) {
		handled := 0
		f := newFixture(t, fakeTool("probe", ClassRead, []Pair{contactsWritePair}, "", func(context.Context, *Call) (any, error) {
			handled++
			return map[string]any{}, nil
		}))
		f.workspaces.EXPECT().GetUserWorkspaces(gomock.Any(), testKeyID).Return(nil, nil).AnyTimes()
		f.auth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "elsewhere").Return(nil, nil, nil, domain.ErrUserNotInWorkspace)

		toolErr := f.callTool("probe", map[string]any{"workspace_id": "elsewhere"}).errorOf(t)
		assert.Equal(t, CodeNotFound, toolErr.Code)
		assert.Equal(t, 0, handled)
	})

	t.Run("pre-check before the handler", func(t *testing.T) {
		var steps []string
		f := newFixture(t, fakeTool("probe", ClassRead, []Pair{contactsWritePair}, "", func(context.Context, *Call) (any, error) {
			steps = append(steps, "handler")
			return map[string]any{}, nil
		}))
		f.workspaces.EXPECT().GetByID(gomock.Any(), testWorkspaceID).Return(testWorkspace(), nil).AnyTimes()
		permissions := contactsRead()
		f.auth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), testWorkspaceID).
			DoAndReturn(func(ctx context.Context, _ string) (context.Context, *domain.User, *domain.UserWorkspace, error) {
				steps = append(steps, "authenticate")
				return ctx, apiKeyUser(), &domain.UserWorkspace{UserID: testKeyID, WorkspaceID: testWorkspaceID, Role: "member", Permissions: permissions}, nil
			}).Times(2)

		toolErr := f.callTool("probe", map[string]any{"workspace_id": testWorkspaceID}).errorOf(t)
		assert.Equal(t, CodePermissionDenied, toolErr.Code)
		assert.Equal(t, "contacts", toolErr.Resource)
		assert.Equal(t, "write", toolErr.Permission)
		assert.Equal(t, []string{"authenticate"}, steps)

		permissions[domain.PermissionResourceContacts] = domain.ResourcePermissions{Read: true, Write: true}
		assert.False(t, f.callTool("probe", map[string]any{"workspace_id": testWorkspaceID}).IsError)
		assert.Equal(t, []string{"authenticate", "authenticate", "handler"}, steps)
	})
}

func TestWrapper_ArgumentDependentPairsAreChecked(t *testing.T) {
	tool := fakeTool("probe", ClassRead, []Pair{contactsReadPair}, "", nil)
	tool.ArgumentPairs = func(args map[string]any) []Pair {
		if args["note"] != nil {
			return []Pair{{Resource: domain.PermissionResourceLists, Verb: domain.PermissionTypeRead}}
		}
		return nil
	}
	f := newFixture(t, tool)
	f.reachable(contactsRead())

	assert.False(t, f.callTool("probe", map[string]any{"workspace_id": testWorkspaceID}).IsError)
	toolErr := f.callTool("probe", map[string]any{"workspace_id": testWorkspaceID, "note": "x"}).errorOf(t)
	assert.Equal(t, CodePermissionDenied, toolErr.Code)
	assert.Equal(t, "lists", toolErr.Resource)
	assert.Equal(t, "read", toolErr.Permission)
}

func TestWrapper_InvalidArgumentsGiveTheValidationCode(t *testing.T) {
	f := newFixture(t)
	f.reachable(contactsRead())

	for name, args := range map[string]map[string]any{
		"wrong type":       {"workspace_id": testWorkspaceID, "limit": "x"},
		"unknown argument": {"workspace_id": testWorkspaceID, "colour": "red"},
		"missing required": {},
		"value off enum":   {"workspace_id": testWorkspaceID, "list_status": "gone"},
	} {
		t.Run(name, func(t *testing.T) {
			result := f.callTool("search_contacts", args)
			toolErr := result.errorOf(t)
			assert.Equal(t, CodeValidation, toolErr.Code)
			assert.NotContains(t, toolErr.Message, `validating "arguments"`, "the SDK's own text is not the answer")
		})
	}
}

// An error result carries the same error object in structuredContent and in
// the text content.
func TestWrapper_ErrorResultCarriesTheObjectTwice(t *testing.T) {
	f := newFixture(t)
	f.reachable(domain.UserPermissions{})

	result := f.callTool("search_contacts", map[string]any{"workspace_id": testWorkspaceID})
	require.True(t, result.IsError)
	require.Len(t, result.Content, 1)
	assert.Equal(t, "text", result.Content[0].Type)
	assert.JSONEq(t, string(result.StructuredContent), result.Content[0].Text)
	assert.JSONEq(t, `{"error":{"code":"permission_denied","message":"this API key lacks the contacts read permission","resource":"contacts","permission":"read"}}`, result.Content[0].Text)
}

// A catalogue tool missing from the key's tools/list is still refused when
// called, with permission_denied naming the grant.
func TestWrapper_AToolNotListedForTheKeyIsRefused(t *testing.T) {
	f := newFixture(t)
	f.reachable(domain.UserPermissions{domain.PermissionResourceLists: {Read: true}})

	assert.Equal(t, []string{"list_analytics_schemas", "list_lists", "list_workspaces", "query_analytics"}, f.listToolNames())
	toolErr := f.callTool("search_contacts", map[string]any{"workspace_id": testWorkspaceID}).errorOf(t)
	assert.Equal(t, ToolError{Code: CodePermissionDenied, Message: "this API key lacks the contacts read permission", Resource: "contacts", Permission: "read"}, toolErr)
}

func TestWrapper_RefusesAnyAccountButAMemberAPIKey(t *testing.T) {
	for name, account := range map[string]struct {
		user *domain.User
		role string
	}{
		"a person's account": {user: &domain.User{ID: testKeyID, Type: domain.UserTypeUser}, role: "member"},
		"an owner role":      {user: apiKeyUser(), role: "owner"},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			f.membership(&domain.UserWorkspace{UserID: testKeyID, WorkspaceID: testWorkspaceID, Role: account.role, Permissions: domain.NewFullPermissions()}, account.user)

			toolErr := f.callTool("search_contacts", map[string]any{"workspace_id": testWorkspaceID}).errorOf(t)
			assert.Equal(t, CodePermissionDenied, toolErr.Code)
			assert.Equal(t, []string{"list_workspaces"}, f.listToolNames(), "nothing but list_workspaces is listed")
		})
	}
}

// effective is a copy: a handler that changes it changes nothing stored.
func TestPrecheck_EffectiveIsACopy(t *testing.T) {
	stored := domain.UserPermissions{domain.PermissionResourceContacts: {Read: true}}
	membership := &domain.UserWorkspace{Role: "member", Permissions: stored}

	effective, toolErr := precheck(fakeTool("t", ClassRead, []Pair{contactsReadPair}, "", nil), nil, apiKeyUser(), membership)
	require.Nil(t, toolErr)
	assert.Equal(t, stored, effective)

	effective[domain.PermissionResourceContacts] = domain.ResourcePermissions{Read: true, Write: true}
	effective[domain.PermissionResourceLists] = domain.ResourcePermissions{Read: true}
	assert.Equal(t, domain.UserPermissions{domain.PermissionResourceContacts: {Read: true}}, membership.Permissions)
}

func TestWrapper_HandlerSeesACopyOfThePermissions(t *testing.T) {
	stored := contactsRead()
	f := newFixture(t, fakeTool("probe", ClassRead, []Pair{contactsReadPair}, "", func(_ context.Context, call *Call) (any, error) {
		call.Effective[domain.PermissionResourceContacts] = domain.ResourcePermissions{Read: true, Write: true}
		return map[string]any{}, nil
	}))
	f.reachable(stored)

	assert.False(t, f.callTool("probe", map[string]any{"workspace_id": testWorkspaceID}).IsError)
	assert.Equal(t, contactsRead(), stored)
}

// Read tools write no audit row, whatever their outcome: the recorder mock
// fails the test on any call.
func TestWrapper_ReadToolsWriteNoAuditRow(t *testing.T) {
	f := newFixture(t)
	f.reachable(contactsRead())
	f.contacts.EXPECT().GetContacts(gomock.Any(), gomock.Any()).Return(&domain.GetContactsResponse{}, nil)

	assert.False(t, f.callTool("search_contacts", map[string]any{"workspace_id": testWorkspaceID}).IsError)
	assert.True(t, f.callTool("search_contacts", map[string]any{"workspace_id": testWorkspaceID, "limit": 0}).IsError)
	assert.False(t, f.callTool("list_workspaces", nil).IsError)
}

// A tool that is not a read writes one row per call, refusals included, under
// its API route's action, in the key's own workspace when the named one is
// unreachable.
func TestWrapper_NonReadToolsWriteAnAuditRow(t *testing.T) {
	f := newFixture(t, fakeTool("fake_delete", ClassDestructive, []Pair{contactsWritePair}, "contacts.delete", nil))
	f.reachable(domain.UserPermissions{domain.PermissionResourceContacts: {Read: true, Write: true}})
	f.auth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "elsewhere").Return(nil, nil, nil, domain.ErrUserNotInWorkspace).AnyTimes()

	var rows []domain.AuditLog
	f.recorder.EXPECT().Record(gomock.Any(), gomock.Any()).Do(func(_ context.Context, row *domain.AuditLog) {
		rows = append(rows, *row)
	}).AnyTimes()

	assert.False(t, f.callTool("fake_delete", map[string]any{"workspace_id": testWorkspaceID}).IsError)
	assert.True(t, f.callTool("fake_delete", map[string]any{"workspace_id": testWorkspaceID, "note": 3}).IsError)
	assert.True(t, f.callTool("fake_delete", map[string]any{"workspace_id": "elsewhere"}).IsError)

	require.Len(t, rows, 3)
	for _, row := range rows {
		assert.Equal(t, "contacts.delete", row.Action)
		assert.Equal(t, domain.AuditCategoryContacts, row.Category)
		assert.Equal(t, domain.AuditTargetContact, row.TargetType)
		assert.Equal(t, testWorkspaceID, row.WorkspaceID)
		assert.Equal(t, domain.AuditActorAPIKey, row.ActorType)
		assert.Equal(t, testKeyID, row.ActorID)
		assert.Equal(t, domain.AuditAuthAPIKey, row.AuthMethod)
		assert.Equal(t, "203.0.113.7", row.IPAddress)
		assert.Equal(t, "test-agent", row.UserAgent)
		assert.Equal(t, "req-1", row.RequestID)
		assert.Equal(t, "mcp", row.Metadata["channel"])
		assert.Equal(t, "fake_delete", row.Metadata["mcp_tool"])
	}
	assert.Equal(t, http.StatusOK, rows[0].StatusCode)
	assert.Equal(t, domain.AuditOutcomeSuccess, rows[0].Outcome)
	assert.Equal(t, http.StatusBadRequest, rows[1].StatusCode)
	assert.Equal(t, "validation", rows[1].Metadata["error_code"])
	assert.Equal(t, http.StatusNotFound, rows[2].StatusCode)
}

func TestWrapper_PermissionRefusalIsAuditedWithTheGrant(t *testing.T) {
	f := newFixture(t, fakeTool("fake_delete", ClassDestructive, []Pair{contactsWritePair}, "contacts.delete", nil))
	f.reachable(contactsRead())
	var row domain.AuditLog
	f.recorder.EXPECT().Record(gomock.Any(), gomock.Any()).Do(func(_ context.Context, recorded *domain.AuditLog) { row = *recorded })

	assert.True(t, f.callTool("fake_delete", map[string]any{"workspace_id": testWorkspaceID}).IsError)
	assert.Equal(t, http.StatusForbidden, row.StatusCode)
	assert.Equal(t, domain.AuditOutcomeDenied, row.Outcome)
	assert.Equal(t, "contacts", row.Metadata["resource"])
	assert.Equal(t, "write", row.Metadata["permission"])
}

// The mcp_send limit: thirty send or open-world write calls a minute per key;
// the thirty-first is refused before its handler runs, and is audited.
func TestWrapper_SendLimit(t *testing.T) {
	sent := 0
	f := newFixture(t, fakeTool("fake_send", ClassSend, []Pair{contactsWritePair}, "broadcasts.schedule", func(context.Context, *Call) (any, error) {
		sent++
		return map[string]any{}, nil
	}), fakeTool("fake_write", ClassWrite, []Pair{contactsWritePair}, "contacts.delete", nil))
	f.reachable(domain.UserPermissions{domain.PermissionResourceContacts: {Read: true, Write: true}})
	var statuses []int
	f.recorder.EXPECT().Record(gomock.Any(), gomock.Any()).Do(func(_ context.Context, row *domain.AuditLog) {
		statuses = append(statuses, row.StatusCode)
	}).AnyTimes()

	for i := 0; i < 30; i++ {
		require.False(t, f.callTool("fake_send", map[string]any{"workspace_id": testWorkspaceID}).IsError, "call %d", i+1)
	}
	toolErr := f.callTool("fake_send", map[string]any{"workspace_id": testWorkspaceID}).errorOf(t)
	assert.Equal(t, CodeRateLimited, toolErr.Code)
	assert.Equal(t, 30, sent)
	assert.Equal(t, http.StatusTooManyRequests, statuses[len(statuses)-1])

	assert.False(t, f.callTool("fake_write", map[string]any{"workspace_id": testWorkspaceID}).IsError, "a plain write does not count")
}

func TestToToolError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want ToolError
	}{
		{"unreachable workspace", fmt.Errorf("failed to authenticate user: %w", domain.ErrUserNotInWorkspace), ToolError{Code: "not_found", Message: "workspace not found"}},
		{"missing workspace", &domain.ErrWorkspaceNotFound{WorkspaceID: "x"}, ToolError{Code: "not_found", Message: "workspace not found"}},
		{"permission", fmt.Errorf("wrapped: %w", domain.NewPermissionError(domain.PermissionResourceLists, domain.PermissionTypeWrite, "no")), ToolError{Code: "permission_denied", Message: "no", Resource: "lists", Permission: "write"}},
		{"licence", &domain.ErrFeatureNotLicensed{Feature: "rbac", RequiredTier: "business", Message: "buy"}, ToolError{Code: "licence_required", Message: "buy", Feature: "rbac", RequiredTier: "business"}},
		{"entity not found", &domain.ErrNotFound{Entity: "broadcast", ID: "b1"}, ToolError{Code: "not_found", Message: "broadcast not found with ID: b1"}},
		{"contact not found", fmt.Errorf("x: %w", domain.ErrContactNotFound), ToolError{Code: "not_found", Message: "contact not found"}},
		{"invalid state", domain.NewInvalidStateError("broadcast", "b1", "schedule", "sending"), ToolError{Code: "invalid_state", Message: "cannot schedule broadcast b1: its status is sending", Status: "sending"}},
		{"conflict", domain.NewAlreadyExistsError("list", "news", false), ToolError{Code: "conflict", Message: "list news already exists"}},
		{"validation", fmt.Errorf("x: %w", domain.NewValidationError("bad name")), ToolError{Code: "validation", Message: "bad name"}},
		{"segment value", fmt.Errorf("x: %w", &domain.SegmentValueError{Cause: "invalid input syntax for type numeric"}), ToolError{Code: "validation", Message: "the segment query could not convert a value: invalid input syntax for type numeric"}},
		{"list id taken", fmt.Errorf("x: %w", &domain.ErrListExists{Message: "list id already exists"}), ToolError{Code: "conflict", Message: "list id already exists"}},
		{"list in use", fmt.Errorf("x: %w", domain.NewErrListInUse("news", "Newsletter", []domain.AutomationRef{{ID: "a1", Name: "Welcome"}}).WithoutAutomations()), ToolError{Code: "conflict", Message: "cannot delete list \"Newsletter\" while 1 live automation uses it: ask someone with access to automations to pause it"}},
		{"opted out", fmt.Errorf("x: %w", &domain.ErrContactListOptedOut{Status: domain.ContactListStatusUnsubscribed}), ToolError{Code: "invalid_state", Message: "the contact unsubscribed from this list: set resubscribe to true to subscribe them again", Status: "unsubscribed"}},
		{"render", fmt.Errorf("failed to send notification through any channel: %w", &domain.TemplateRenderError{Message: "liquid: unknown filter money"}), ToolError{Code: "validation", Message: "liquid: unknown filter money"}},
		{"internal", errors.New("pq: relation \"secret_table\" does not exist"), ToolError{Code: "internal", Message: "internal error"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, *toToolError(tc.err))
		})
	}
}

func TestToolError_AuditStatus(t *testing.T) {
	want := map[string]int{
		"validation":        400,
		"licence_required":  402,
		"permission_denied": 403,
		"not_found":         404,
		"invalid_state":     409,
		"conflict":          409,
		"rate_limited":      429,
		"internal":          500,
	}
	for code, status := range want {
		assert.Equal(t, status, (&ToolError{Code: code}).auditStatus(), code)
	}
}

func TestDecodeArguments_AbsentArgumentsAreAnEmptyObject(t *testing.T) {
	tool := &Tool{Name: "t", Class: ClassRead, WithoutWorkspace: true, InputSchema: map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}, Handler: func(context.Context, *Call) (any, error) { return nil, nil }}
	tool.prepare()
	for _, raw := range []string{"", "null", "{}"} {
		args, _, toolErr := decodeArguments(tool, json.RawMessage(raw))
		require.Nil(t, toolErr, raw)
		assert.Empty(t, args)
	}
	_, _, toolErr := decodeArguments(tool, json.RawMessage(`[1]`))
	require.NotNil(t, toolErr)
	assert.Equal(t, CodeValidation, toolErr.Code)
}
