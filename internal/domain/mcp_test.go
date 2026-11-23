package domain

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMCPCredentialKinds(t *testing.T) {
	assert.Equal(t, MCPCredentialKind("header"), MCPCredentialHeader)
	assert.Equal(t, MCPCredentialKind("oauth"), MCPCredentialOAuth)
	assert.NotEqual(t, MCPCredentialHeader, MCPCredentialOAuth)
}

func TestMCPCallerContext(t *testing.T) {
	t.Run("a context without a caller has none", func(t *testing.T) {
		assert.Nil(t, MCPCallerFromContext(context.Background()))
		//nolint:staticcheck // a nil context must not panic
		assert.Nil(t, MCPCallerFromContext(nil))
	})

	t.Run("a header caller is read back", func(t *testing.T) {
		caller := &MCPCaller{Kind: MCPCredentialHeader, APIKeyUserID: "key-user"}
		ctx := WithMCPCaller(context.Background(), caller)

		got := MCPCallerFromContext(ctx)
		require.NotNil(t, got)
		assert.Same(t, caller, got)
		assert.Equal(t, MCPCredentialHeader, got.Kind)
		assert.Equal(t, "key-user", got.APIKeyUserID)
		assert.Empty(t, got.GrantID)
		assert.Empty(t, got.ClientName)
	})

	t.Run("an OAuth caller carries its grant and client", func(t *testing.T) {
		ctx := WithMCPCaller(context.Background(), &MCPCaller{
			Kind:         MCPCredentialOAuth,
			APIKeyUserID: "key-user",
			GrantID:      "grant-1",
			ClientName:   "Claude",
		})

		got := MCPCallerFromContext(ctx)
		require.NotNil(t, got)
		assert.Equal(t, MCPCredentialOAuth, got.Kind)
		assert.Equal(t, "grant-1", got.GrantID)
		assert.Equal(t, "Claude", got.ClientName)
	})

	t.Run("a same-named string key does not stand for the caller", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), contextKey("mcp_caller"), &MCPCaller{Kind: MCPCredentialHeader})
		assert.Nil(t, MCPCallerFromContext(ctx))
	})
}

func TestMCPSettings_MCPEnabled(t *testing.T) {
	var unset *MCPSettings
	assert.True(t, unset.MCPEnabled(), "nil settings mean enabled")
	assert.True(t, (&MCPSettings{Disabled: false}).MCPEnabled())
	assert.False(t, (&MCPSettings{Disabled: true}).MCPEnabled())
}

func TestWorkspace_MCPEnabled(t *testing.T) {
	assert.True(t, (&Workspace{}).MCPEnabled(), "a workspace saved before the switch existed is enabled")
	assert.True(t, (&Workspace{Settings: WorkspaceSettings{MCP: &MCPSettings{Disabled: false}}}).MCPEnabled())
	assert.False(t, (&Workspace{Settings: WorkspaceSettings{MCP: &MCPSettings{Disabled: true}}}).MCPEnabled())

	var missing *Workspace
	assert.False(t, missing.MCPEnabled(), "a workspace that does not exist is not reachable")
}

func TestSetMCPSettingsRequest_Validate(t *testing.T) {
	decode := func(t *testing.T, body string) SetMCPSettingsRequest {
		var req SetMCPSettingsRequest
		require.NoError(t, json.Unmarshal([]byte(body), &req))
		return req
	}

	t.Run("switching off", func(t *testing.T) {
		req := decode(t, `{"workspace_id":"ws1","disabled":true}`)
		workspaceID, settings, err := req.Validate()
		require.NoError(t, err)
		assert.Equal(t, "ws1", workspaceID)
		assert.Equal(t, &MCPSettings{Disabled: true}, settings)
	})

	t.Run("switching on", func(t *testing.T) {
		req := decode(t, `{"workspace_id":"ws1","disabled":false}`)
		_, settings, err := req.Validate()
		require.NoError(t, err)
		assert.Equal(t, &MCPSettings{Disabled: false}, settings)
	})

	t.Run("workspace_id is required", func(t *testing.T) {
		req := decode(t, `{"disabled":true}`)
		_, _, err := req.Validate()
		assert.EqualError(t, err, "workspace_id is required")
	})

	t.Run("a body without disabled is refused, not read as enabling", func(t *testing.T) {
		req := decode(t, `{"workspace_id":"ws1"}`)
		_, _, err := req.Validate()
		assert.EqualError(t, err, "disabled is required")
	})
}
