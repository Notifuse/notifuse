package mcp

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
)

func TestListWorkspaces_TheKeysWorkspace(t *testing.T) {
	f := newFixture(t)
	f.reachable(domain.UserPermissions{
		domain.PermissionResourceContacts: {Read: true},
		domain.PermissionResourceLists:    {Read: true, Write: true},
	})

	result := f.callTool("list_workspaces", nil)
	require.False(t, result.IsError, string(result.StructuredContent))
	// The permissions are those of the resources the catalogue uses.
	assert.JSONEq(t, `{"workspaces":[{
		"id":"ws1","name":"Acme","timezone":"Europe/Paris",
		"permissions":{
			"automations":{"read":false,"write":false},
			"blog":{"read":false,"write":false},
			"broadcasts":{"read":false,"write":false},
			"contacts":{"read":true,"write":false},
			"lists":{"read":true,"write":true},
			"message_history":{"read":false,"write":false},
			"segments":{"read":false,"write":false},
			"templates":{"read":false,"write":false},
			"transactional":{"read":false,"write":false},
			"workspace":{"read":false,"write":false}
		}
	}]}`, string(result.StructuredContent))
	assert.JSONEq(t, string(result.StructuredContent), result.Content[0].Text)
}

func TestListWorkspaces_EmptyWhenSwitchedOff(t *testing.T) {
	f := newFixture(t)
	f.switchedOff()

	result := f.callTool("list_workspaces", nil)
	require.False(t, result.IsError)
	assert.JSONEq(t, `{"workspaces":[]}`, string(result.StructuredContent))
}

func TestListWorkspaces_TakesNoArgument(t *testing.T) {
	f := newFixture(t)
	toolErr := f.callTool("list_workspaces", map[string]any{"workspace_id": testWorkspaceID}).errorOf(t)
	assert.Equal(t, CodeValidation, toolErr.Code)
}

func TestListWorkspaces_AFailedLookupIsInternal(t *testing.T) {
	f := newFixture(t)
	f.workspaces.EXPECT().GetUserWorkspaces(gomock.Any(), testKeyID).Return(nil, errors.New("connection refused to db-host-7"))

	toolErr := f.callTool("list_workspaces", nil).errorOf(t)
	assert.Equal(t, ToolError{Code: CodeInternal, Message: "internal error"}, toolErr)
}

func workspaceAndListsRead() domain.UserPermissions {
	return domain.UserPermissions{
		domain.PermissionResourceWorkspace: {Read: true},
		domain.PermissionResourceLists:     {Read: true},
	}
}

func intPtr(v int) *int { return &v }

// overviewWorkspace is a workspace with everything get_workspace_overview must
// leave out: integration IDs and credentials, storage keys, the secret key.
func overviewWorkspace() *domain.Workspace {
	return &domain.Workspace{
		ID:   testWorkspaceID,
		Name: "Acme",
		Settings: domain.WorkspaceSettings{
			WebsiteURL:                   "https://acme.example",
			LogoURL:                      "https://acme.example/logo.png",
			Timezone:                     "Europe/Paris",
			DefaultLanguage:              "en",
			Languages:                    []string{"en", "fr"},
			CustomFieldLabels:            map[string]string{"custom_string_1": "Plan", "custom_number_1": "Seats"},
			BlogEnabled:                  true,
			WebAnalytics:                 &domain.WebAnalyticsSettings{Enabled: true, AllowedDomains: []string{"acme.example"}},
			MarketingEmailProviderID:     "int-ses",
			TransactionalEmailProviderID: "int-gone",
			EncryptedSecretKey:           "enc-workspace-secret",
			SecretKey:                    "workspace-secret",
			FileManager:                  domain.FileManagerSettings{Bucket: "media", AccessKey: "AKIA-storage", EncryptedSecretKey: "enc-storage"},
		},
		Integrations: []domain.Integration{{
			ID:   "int-ses",
			Name: "SES",
			Type: domain.IntegrationTypeEmail,
			EmailProvider: domain.EmailProvider{
				Kind: domain.EmailProviderKindSES,
				SES:  &domain.AmazonSESSettings{Region: "eu-west-1", AccessKey: "AKIA-ses", EncryptedSecretKey: "enc-ses"},
			},
		}},
	}
}

func TestGetWorkspaceOverview_Definition(t *testing.T) {
	assertReadTool(t, catalogueTool(t, "get_workspace_overview"), "get_workspace_overview",
		[]Pair{{Resource: "workspace", Verb: "read"}, {Resource: "lists", Verb: "read"}},
		[]string{"workspace_id"})
}

func TestGetWorkspaceOverview_Projection(t *testing.T) {
	f := newFixture(t)
	f.reachable(workspaceAndListsRead())
	f.wsService.EXPECT().GetWorkspace(gomock.Any(), testWorkspaceID).Return(overviewWorkspace(), nil)
	f.lists.EXPECT().GetLists(gomock.Any(), testWorkspaceID, true).Return([]*domain.List{
		{ID: "news", Name: "Newsletter", Description: "Monthly news", IsPublic: true, IsDoubleOptin: true, ActiveCount: intPtr(1250),
			DoubleOptInTemplate: &domain.TemplateReference{ID: "doi", Version: 2}},
		{ID: "beta", Name: "Beta"},
	}, nil)

	result := f.callTool("get_workspace_overview", map[string]any{"workspace_id": testWorkspaceID})
	require.False(t, result.IsError, string(result.StructuredContent))
	assert.JSONEq(t, `{
		"workspace":{"id":"ws1","name":"Acme"},
		"timezone":"Europe/Paris",
		"default_language":"en",
		"languages":["en","fr"],
		"website_url":"https://acme.example",
		"custom_field_labels":{"custom_number_1":"Seats","custom_string_1":"Plan"},
		"blog_enabled":true,
		"web_analytics_enabled":true,
		"email_providers":{"marketing":"ses","transactional":null},
		"lists":[
			{"id":"news","name":"Newsletter","is_public":true,"is_double_optin":true,"active_count":1250},
			{"id":"beta","name":"Beta","is_public":false,"is_double_optin":false,"active_count":0}
		]
	}`, string(result.StructuredContent))
	for _, secret := range []string{"int-ses", "int-gone", "AKIA", "enc-", "workspace-secret", "logo.png", "media"} {
		assert.NotContains(t, result.Content[0].Text, secret)
	}
}

func TestGetWorkspaceOverview_EmptyWorkspace(t *testing.T) {
	f := newFixture(t)
	f.reachable(workspaceAndListsRead())
	f.wsService.EXPECT().GetWorkspace(gomock.Any(), testWorkspaceID).Return(&domain.Workspace{ID: testWorkspaceID, Name: "Acme", Settings: domain.WorkspaceSettings{Timezone: "UTC"}}, nil)
	f.lists.EXPECT().GetLists(gomock.Any(), testWorkspaceID, true).Return(nil, nil)

	result := f.callTool("get_workspace_overview", map[string]any{"workspace_id": testWorkspaceID})
	require.False(t, result.IsError, string(result.StructuredContent))
	assert.JSONEq(t, `{
		"workspace":{"id":"ws1","name":"Acme"},"timezone":"UTC","default_language":"","languages":[],
		"custom_field_labels":{},"blog_enabled":false,"web_analytics_enabled":false,
		"email_providers":{"marketing":null,"transactional":null},"lists":[]
	}`, string(result.StructuredContent))
}

func TestGetWorkspaceOverview_CapsFreeText(t *testing.T) {
	f := newFixture(t)
	f.reachable(workspaceAndListsRead())
	workspace := overviewWorkspace()
	workspace.Settings.CustomFieldLabels["custom_string_2"] = strings.Repeat("L", 2100)
	f.wsService.EXPECT().GetWorkspace(gomock.Any(), testWorkspaceID).Return(workspace, nil)
	f.lists.EXPECT().GetLists(gomock.Any(), testWorkspaceID, true).Return([]*domain.List{{ID: "a", Name: "ok"}, {ID: "b", Name: strings.Repeat("N", 2001)}}, nil)

	var out struct {
		CustomFieldLabels map[string]string `json:"custom_field_labels"`
		Lists             []struct {
			Name string `json:"name"`
		} `json:"lists"`
		Truncated       bool     `json:"truncated"`
		TruncatedFields []string `json:"truncated_fields"`
	}
	f.callTool("get_workspace_overview", map[string]any{"workspace_id": testWorkspaceID}).decode(t, &out)
	assert.Len(t, out.CustomFieldLabels["custom_string_2"], 2000)
	assert.Len(t, out.Lists[1].Name, 2000)
	assert.True(t, out.Truncated)
	assert.Equal(t, []string{"custom_field_labels.custom_string_2", "lists[1].name"}, out.TruncatedFields)
}

// The pre-check refuses a key missing either pair before any service call.
func TestGetWorkspaceOverview_NeedsBothPairs(t *testing.T) {
	for name, permissions := range map[string]domain.UserPermissions{
		"without lists:read":     {domain.PermissionResourceWorkspace: {Read: true}},
		"without workspace:read": {domain.PermissionResourceLists: {Read: true}, domain.PermissionResourceWorkspace: {Write: true}},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			f.reachable(permissions)
			toolErr := f.callTool("get_workspace_overview", map[string]any{"workspace_id": testWorkspaceID}).errorOf(t)
			assert.Equal(t, CodePermissionDenied, toolErr.Code)
			assert.Equal(t, "read", toolErr.Permission)
		})
	}
}

func TestGetWorkspaceOverview_ServiceErrorsAreMapped(t *testing.T) {
	t.Run("a permission refusal of the service", func(t *testing.T) {
		f := newFixture(t)
		f.reachable(workspaceAndListsRead())
		f.wsService.EXPECT().GetWorkspace(gomock.Any(), testWorkspaceID).
			Return(nil, domain.NewPermissionError(domain.PermissionResourceWorkspace, domain.PermissionTypeRead, "Insufficient permissions: read access to workspace required"))
		toolErr := f.callTool("get_workspace_overview", map[string]any{"workspace_id": testWorkspaceID}).errorOf(t)
		assert.Equal(t, CodePermissionDenied, toolErr.Code)
		assert.Equal(t, "workspace", toolErr.Resource)
	})

	t.Run("a failed list read", func(t *testing.T) {
		f := newFixture(t)
		f.reachable(workspaceAndListsRead())
		f.wsService.EXPECT().GetWorkspace(gomock.Any(), testWorkspaceID).Return(overviewWorkspace(), nil)
		f.lists.EXPECT().GetLists(gomock.Any(), testWorkspaceID, true).Return(nil, errors.New("pq: relation lists on db-host-7"))
		toolErr := f.callTool("get_workspace_overview", map[string]any{"workspace_id": testWorkspaceID}).errorOf(t)
		assert.Equal(t, ToolError{Code: CodeInternal, Message: "internal error"}, toolErr)
	})

	t.Run("a switched-off workspace", func(t *testing.T) {
		f := newFixture(t)
		f.switchedOff()
		toolErr := f.callTool("get_workspace_overview", map[string]any{"workspace_id": testWorkspaceID}).errorOf(t)
		assert.Equal(t, ToolError{Code: CodeNotFound, Message: "workspace not found"}, toolErr)
	})
}

// The workspace name is free text, capped like the rest.
func TestListWorkspaces_CapsTheName(t *testing.T) {
	f := newFixture(t)
	f.workspaces.EXPECT().GetUserWorkspaces(gomock.Any(), testKeyID).
		Return([]*domain.UserWorkspace{{UserID: testKeyID, WorkspaceID: testWorkspaceID}}, nil).AnyTimes()
	f.auth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), testWorkspaceID).
		DoAndReturn(func(ctx context.Context, _ string) (context.Context, *domain.User, *domain.UserWorkspace, error) {
			return ctx, apiKeyUser(), &domain.UserWorkspace{UserID: testKeyID, WorkspaceID: testWorkspaceID, Role: "member"}, nil
		}).AnyTimes()
	workspace := testWorkspace()
	workspace.Name = strings.Repeat("n", FreeTextLimit+1)
	f.workspaces.EXPECT().GetByID(gomock.Any(), testWorkspaceID).Return(workspace, nil).AnyTimes()

	var out struct {
		Workspaces []struct {
			Name            string   `json:"name"`
			TruncatedFields []string `json:"truncated_fields"`
		} `json:"workspaces"`
	}
	f.callTool("list_workspaces", nil).decode(t, &out)
	require.Len(t, out.Workspaces, 1)
	assert.Equal(t, strings.Repeat("n", FreeTextLimit), out.Workspaces[0].Name)
	assert.Equal(t, []string{"name"}, out.Workspaces[0].TruncatedFields)
}
