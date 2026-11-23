package mcp

import (
	"context"
	"fmt"
	"sort"

	"github.com/Notifuse/notifuse/internal/domain"
)

// listWorkspacesTool lists the workspaces the key reaches over MCP: its own,
// unless the workspace's MCP switch is off. It is the one tool without a
// workspace_id.
func (s *Server) listWorkspacesTool() *Tool {
	return &Tool{
		Name:  "list_workspaces",
		Title: "List workspaces",
		Description: "Lists the Notifuse workspaces this connection can work in. Each has its timezone and the permissions the API key holds there. " +
			"Every other tool takes the workspace_id of one of them. Dates you write are read in the workspace's timezone.",
		Class:            ClassRead,
		WithoutWorkspace: true,
		InputSchema: map[string]any{
			"type":                 "object",
			"properties":           map[string]any{},
			"additionalProperties": false,
		},
		Handler: s.listWorkspaces,
	}
}

type workspaceSummary struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Timezone    string                 `json:"timezone"`
	Permissions domain.UserPermissions `json:"permissions"`
	Truncation
}

type listWorkspacesOutput struct {
	Workspaces []workspaceSummary `json:"workspaces"`
}

func (s *Server) listWorkspaces(ctx context.Context, call *Call) (any, error) {
	reachableWorkspaces, err := s.reachableWorkspaces(ctx, call.Caller)
	if err != nil {
		return nil, err
	}
	resources := s.catalogueResources()
	out := listWorkspacesOutput{Workspaces: []workspaceSummary{}}
	for _, reached := range reachableWorkspaces {
		workspace, err := s.deps.Workspaces.GetByID(ctx, reached.workspaceID)
		if err != nil {
			return nil, err
		}
		permissions := make(domain.UserPermissions, len(resources))
		for resource := range resources {
			permissions[resource] = reached.effective[resource]
		}
		summary := workspaceSummary{ID: workspace.ID, Timezone: workspace.Settings.Timezone, Permissions: permissions}
		summary.Name = summary.Text("name", workspace.Name)
		out.Workspaces = append(out.Workspaces, summary)
	}
	return out, nil
}

// getWorkspaceOverviewTool describes one workspace: its settings an assistant
// needs to write content, the kinds of email provider it sends with, and its
// lists. Never an ID or a credential of an integration.
func (s *Server) getWorkspaceOverviewTool() *Tool {
	return &Tool{
		Name:  "get_workspace_overview",
		Title: "Get workspace overview",
		Description: "Describes a workspace: " +
			"timezone, default and other languages, website URL, the labels of custom contact fields, and whether the blog and web analytics are on. " +
			"It also gives the kind of email provider used for marketing email and for transactional email (null when none is set), and every list with its active subscriber count. " +
			"list_lists gives each list's description and full subscription counts.",
		Class: ClassRead,
		Pairs: []Pair{
			{Resource: domain.PermissionResourceWorkspace, Verb: domain.PermissionTypeRead},
			{Resource: domain.PermissionResourceLists, Verb: domain.PermissionTypeRead},
		},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"workspace_id": map[string]any{"type": "string", "description": "Workspace ID from list_workspaces."},
			},
			"required":             []string{"workspace_id"},
			"additionalProperties": false,
		},
		Handler: s.getWorkspaceOverview,
	}
}

type emailProviderKinds struct {
	Marketing     *string `json:"marketing"`
	Transactional *string `json:"transactional"`
}

type overviewList struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	IsPublic      bool   `json:"is_public"`
	IsDoubleOptin bool   `json:"is_double_optin"`
	ActiveCount   int    `json:"active_count"`
}

type workspaceOverviewOutput struct {
	Workspace           WorkspaceRef       `json:"workspace"`
	Timezone            string             `json:"timezone"`
	DefaultLanguage     string             `json:"default_language"`
	Languages           []string           `json:"languages"`
	WebsiteURL          string             `json:"website_url,omitempty"`
	CustomFieldLabels   map[string]string  `json:"custom_field_labels"`
	BlogEnabled         bool               `json:"blog_enabled"`
	WebAnalyticsEnabled bool               `json:"web_analytics_enabled"`
	EmailProviders      emailProviderKinds `json:"email_providers"`
	Lists               []overviewList     `json:"lists"`
	Truncation
}

func (s *Server) getWorkspaceOverview(ctx context.Context, call *Call) (any, error) {
	workspace, err := s.deps.WorkspaceService.GetWorkspace(ctx, call.WorkspaceID)
	if err != nil {
		return nil, err
	}
	lists, err := s.deps.Lists.GetLists(ctx, call.WorkspaceID, true)
	if err != nil {
		return nil, err
	}

	settings := workspace.Settings
	out := workspaceOverviewOutput{
		Workspace:           WorkspaceRef{ID: workspace.ID, Name: workspace.Name},
		Timezone:            settings.Timezone,
		DefaultLanguage:     settings.DefaultLanguage,
		Languages:           append([]string{}, settings.Languages...),
		CustomFieldLabels:   map[string]string{},
		BlogEnabled:         settings.BlogEnabled,
		WebAnalyticsEnabled: settings.WebAnalytics != nil && settings.WebAnalytics.Enabled,
		EmailProviders: emailProviderKinds{
			Marketing:     providerKind(workspace, settings.MarketingEmailProviderID),
			Transactional: providerKind(workspace, settings.TransactionalEmailProviderID),
		},
		Lists: make([]overviewList, 0, len(lists)),
	}
	out.WebsiteURL = out.Text("website_url", settings.WebsiteURL)
	// Sorted, so truncated_fields comes out in the same order every time.
	fields := make([]string, 0, len(settings.CustomFieldLabels))
	for field := range settings.CustomFieldLabels {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	for _, field := range fields {
		out.CustomFieldLabels[field] = out.Text("custom_field_labels."+field, settings.CustomFieldLabels[field])
	}
	for _, list := range lists {
		if list == nil {
			continue
		}
		active := 0
		if list.ActiveCount != nil {
			active = *list.ActiveCount
		}
		out.Lists = append(out.Lists, overviewList{
			ID:            list.ID,
			Name:          out.Text(fmt.Sprintf("lists[%d].name", len(out.Lists)), list.Name),
			IsPublic:      list.IsPublic,
			IsDoubleOptin: list.IsDoubleOptin,
			ActiveCount:   active,
		})
	}
	return out, nil
}

// providerKind is the kind of the email provider an integration ID names, or
// nil when none is set or the integration is gone. The ID itself is never
// returned.
func providerKind(workspace *domain.Workspace, integrationID string) *string {
	if integrationID == "" {
		return nil
	}
	integration := workspace.GetIntegrationByID(integrationID)
	if integration == nil || integration.EmailProvider.Kind == "" {
		return nil
	}
	kind := string(integration.EmailProvider.Kind)
	return &kind
}
