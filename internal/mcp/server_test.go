package mcp

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
)

func TestServerDiscover_AdvertisesToolsWithoutListChanged(t *testing.T) {
	f := newFixture(t)
	resp := f.rpc("server/discover", nil)
	require.Nil(t, resp.Error)
	var discover struct {
		Capabilities map[string]json.RawMessage `json:"capabilities"`
	}
	require.NoError(t, json.Unmarshal(resp.Result, &discover))
	assert.Equal(t, []string{"tools"}, mapKeys(discover.Capabilities))
	assert.JSONEq(t, `{}`, string(discover.Capabilities["tools"]))
}

func TestToolsList_IsPrivateForAMinute(t *testing.T) {
	f := newFixture(t)
	f.reachable(contactsRead())
	resp := f.rpc("tools/list", nil)
	require.Nil(t, resp.Error)
	var list struct {
		CacheScope string `json:"cacheScope"`
		TTLMs      int    `json:"ttlMs"`
	}
	require.NoError(t, json.Unmarshal(resp.Result, &list))
	assert.Equal(t, "private", list.CacheScope)
	assert.Equal(t, 60000, list.TTLMs)
}

func TestToolsList_FilteredByPermissionsAndSwitch(t *testing.T) {
	t.Run("a key that reads contacts sees the contact tools, by name", func(t *testing.T) {
		f := newFixture(t)
		f.reachable(contactsRead())
		assert.Equal(t, []string{
			"count_contacts", "get_contact", "get_contact_timeline", "list_analytics_schemas", "list_workspaces", "query_analytics", "search_contacts",
		}, f.listToolNames())
	})

	t.Run("get_workspace_overview needs both of its pairs", func(t *testing.T) {
		f := newFixture(t)
		f.reachable(domain.UserPermissions{domain.PermissionResourceWorkspace: {Read: true}})
		assert.Equal(t, []string{"list_analytics_schemas", "list_workspaces", "query_analytics"}, f.listToolNames())

		g := newFixture(t)
		g.reachable(domain.UserPermissions{domain.PermissionResourceWorkspace: {Read: true}, domain.PermissionResourceLists: {Read: true}})
		assert.Equal(t, []string{"get_workspace_overview", "list_analytics_schemas", "list_lists", "list_workspaces", "query_analytics"}, g.listToolNames())
	})

	t.Run("a key without contacts:read does not see the contact tools", func(t *testing.T) {
		f := newFixture(t)
		f.reachable(domain.UserPermissions{domain.PermissionResourceContacts: {Write: true}})
		// The analytics tools have no static pair: their permission is the
		// schema's, checked at call time.
		assert.Equal(t, []string{"list_analytics_schemas", "list_workspaces", "query_analytics"}, f.listToolNames())
	})

	t.Run("a switched-off workspace hides every workspace tool", func(t *testing.T) {
		f := newFixture(t)
		f.switchedOff()
		assert.Equal(t, []string{"list_workspaces"}, f.listToolNames())
	})
}

// The order is by name whatever the registration order.
func TestToolsList_DeterministicOrder(t *testing.T) {
	f := newFixture(t,
		fakeTool("zeta", ClassRead, nil, "", nil),
		fakeTool("alpha", ClassRead, nil, "", nil),
		fakeTool("mid", ClassRead, nil, "", nil),
	)
	f.reachable(domain.UserPermissions{})
	for range 3 {
		assert.Equal(t, []string{"alpha", "mid", "zeta"}, f.listToolNames())
	}
}

// The mcp limit: 120 calls a minute per key, tools/list included, checked
// before any database work. tools/list is then refused with a JSON-RPC error,
// tools/call with an isError result. Another key keeps its own budget.
func TestMCPLimit_120CallsAMinuteIncludingToolsList(t *testing.T) {
	f := newFixture(t)
	f.reachable(contactsRead())

	for i := 0; i < 60; i++ {
		require.Nil(t, f.rpc("tools/list", nil).Error, "tools/list %d", i+1)
		require.False(t, f.callTool("list_workspaces", nil).IsError, "tools/call %d", i+1)
	}

	resp := f.rpc("tools/list", nil)
	require.NotNil(t, resp.Error)
	assert.Equal(t, int64(-32000), resp.Error.Code)
	assert.JSONEq(t, `{"code":"rate_limited"}`, string(resp.Error.Data))

	toolErr := f.callTool("list_workspaces", nil).errorOf(t)
	assert.Equal(t, CodeRateLimited, toolErr.Code)

	f.keyID = "key-2"
	f.workspaces.EXPECT().GetUserWorkspaces(gomock.Any(), "key-2").Return(nil, nil).AnyTimes()
	assert.Equal(t, []string{"list_workspaces"}, f.listToolNames(), "the budget is per key")
}

// A protocol method other than tools/list and tools/call is not counted.
func TestMCPLimit_DoesNotCountDiscovery(t *testing.T) {
	f := newFixture(t)
	f.limiter.SetPolicy(RateLimitNamespace, 1, time.Minute)
	for range 3 {
		require.Nil(t, f.rpc("server/discover", nil).Error)
	}
	f.switchedOff()
	require.Nil(t, f.rpc("tools/list", nil).Error)
	require.NotNil(t, f.rpc("tools/list", nil).Error)
}

func mapKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	return keys
}
