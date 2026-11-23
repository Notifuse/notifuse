package domain

import (
	"context"
	"fmt"
)

// MCPSettings is the workspace's MCP switch, stored in WorkspaceSettings.MCP.
// A nil value means MCP is enabled, so workspaces saved before the field
// existed need no backfill. Written only through workspaces.setMCPSettings.
type MCPSettings struct {
	// Disabled hides the workspace from every MCP credential without revoking
	// anything: the same API keys keep working on /api/*.
	Disabled bool `json:"disabled"`
}

// MCPEnabled reports whether MCP credentials may reach a workspace with these
// settings. nil settings mean enabled.
func (s *MCPSettings) MCPEnabled() bool {
	return s == nil || !s.Disabled
}

// SetMCPSettingsRequest is the body of workspaces.setMCPSettings. Disabled is
// a pointer so that a body without it is refused instead of read as "enable".
type SetMCPSettingsRequest struct {
	WorkspaceID string `json:"workspace_id"`
	Disabled    *bool  `json:"disabled"`
}

// Validate returns the workspace ID and the settings to store.
func (r *SetMCPSettingsRequest) Validate() (workspaceID string, settings *MCPSettings, err error) {
	if r.WorkspaceID == "" {
		return "", nil, fmt.Errorf("workspace_id is required")
	}
	if r.Disabled == nil {
		return "", nil, fmt.Errorf("disabled is required")
	}
	return r.WorkspaceID, &MCPSettings{Disabled: *r.Disabled}, nil
}

// MCPCredentialKind names how an MCP caller presented its API key.
type MCPCredentialKind string

const (
	// MCPCredentialHeader is an API key JWT sent in the Authorization header.
	MCPCredentialHeader MCPCredentialKind = "header"
	// MCPCredentialOAuth is an OAuth access token leading, through a grant, to
	// an API key.
	MCPCredentialOAuth MCPCredentialKind = "oauth"
)

// MCPCaller identifies a request that came through the MCP endpoint. Both
// credential kinds resolve to an API key account; GrantID and ClientName are
// set for OAuth only.
type MCPCaller struct {
	Kind         MCPCredentialKind
	APIKeyUserID string
	GrantID      string
	ClientName   string
}

// mcpCallerContextKey is unexported so that only WithMCPCaller can place a
// caller in a context: nothing on the /api/* path can claim to be MCP.
type mcpCallerContextKey struct{}

// WithMCPCaller returns a context carrying the MCP caller.
func WithMCPCaller(ctx context.Context, caller *MCPCaller) context.Context {
	return context.WithValue(ctx, mcpCallerContextKey{}, caller)
}

// MCPCallerFromContext returns the MCP caller of the request, or nil when the
// request did not come through the MCP endpoint.
func MCPCallerFromContext(ctx context.Context) *MCPCaller {
	if ctx == nil {
		return nil
	}
	caller, _ := ctx.Value(mcpCallerContextKey{}).(*MCPCaller)
	return caller
}

// MCPEnabled reports whether MCP credentials may reach the workspace. A nil
// workspace is not reachable.
func (w *Workspace) MCPEnabled() bool {
	return w != nil && w.Settings.MCP.MCPEnabled()
}
