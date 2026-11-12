package domain

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListAutomationsRequest_WithJourneyCounts(t *testing.T) {
	parse := func(extra map[string][]string) (ListAutomationsRequest, error) {
		params := map[string][]string{"workspace_id": {"ws1"}}
		for k, v := range extra {
			params[k] = v
		}
		var req ListAutomationsRequest
		err := req.FromURLParams(params)
		return req, err
	}

	req, err := parse(map[string][]string{"with_journey_counts": {"true"}})
	require.NoError(t, err)
	assert.True(t, req.WithJourneyCounts)

	req, err = parse(map[string][]string{"with_journey_counts": {"false"}})
	require.NoError(t, err)
	assert.False(t, req.WithJourneyCounts)

	req, err = parse(nil)
	require.NoError(t, err)
	assert.False(t, req.WithJourneyCounts)

	_, err = parse(map[string][]string{"with_journey_counts": {"yes"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "with_journey_counts")
}

func TestNodeStatsRequest_FromURLParams(t *testing.T) {
	parse := func(extra map[string][]string) (NodeStatsRequest, error) {
		params := map[string][]string{"workspace_id": {"ws1"}, "automation_id": {"auto1"}}
		for k, v := range extra {
			params[k] = v
		}
		var req NodeStatsRequest
		err := req.FromURLParams(params)
		return req, err
	}

	t.Run("neither bound: all time", func(t *testing.T) {
		req, err := parse(nil)
		require.NoError(t, err)
		assert.Equal(t, "ws1", req.WorkspaceID)
		assert.Equal(t, "auto1", req.AutomationID)
		assert.Nil(t, req.Since)
		assert.Nil(t, req.Until)
	})

	t.Run("both bounds, read as UTC instants", func(t *testing.T) {
		req, err := parse(map[string][]string{
			"since": {"2026-10-12T00:00:00+02:00"},
			"until": {"2026-11-10T22:59:59Z"},
		})
		require.NoError(t, err)
		require.NotNil(t, req.Since)
		require.NotNil(t, req.Until)
		assert.Equal(t, time.Date(2026, 10, 11, 22, 0, 0, 0, time.UTC), *req.Since)
		assert.Equal(t, time.UTC, req.Since.Location())
		assert.Equal(t, time.Date(2026, 11, 10, 22, 59, 59, 0, time.UTC), *req.Until)
	})

	t.Run("since equal to until accepted", func(t *testing.T) {
		_, err := parse(map[string][]string{"since": {"2026-11-10T00:00:00Z"}, "until": {"2026-11-10T00:00:00Z"}})
		require.NoError(t, err)
	})

	for name, extra := range map[string]map[string][]string{
		"only since":          {"since": {"2026-11-01T00:00:00Z"}},
		"only until":          {"until": {"2026-11-01T00:00:00Z"}},
		"unreadable since":    {"since": {"yesterday"}, "until": {"2026-11-01T00:00:00Z"}},
		"unreadable until":    {"since": {"2026-11-01T00:00:00Z"}, "until": {"2026-11-01"}},
		"since after until":   {"since": {"2026-11-02T00:00:00Z"}, "until": {"2026-11-01T00:00:00Z"}},
		"automation_id empty": {"automation_id": {""}},
		"workspace_id empty":  {"workspace_id": {""}},
	} {
		t.Run(name+" refused", func(t *testing.T) {
			_, err := parse(extra)
			require.Error(t, err)
		})
	}
}

func TestNodeStatsResponse_JSON(t *testing.T) {
	t.Run("email absent when not readable, maps are objects", func(t *testing.T) {
		data, err := json.Marshal(NodeStatsResponse{
			AutomationID: "auto1",
			Nodes: map[string]*NodeStats{
				"n1": NewNodeStats(NodeTypeDelay),
			},
		})
		require.NoError(t, err)
		var got map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &got))
		assert.NotContains(t, got, "since", "all time has no bounds")
		assert.NotContains(t, got, "until")
		node := got["nodes"].(map[string]interface{})["n1"].(map[string]interface{})
		assert.NotContains(t, node, "email")
		assert.Equal(t, map[string]interface{}{}, node["branches"])
		assert.Equal(t, map[string]interface{}{}, node["skip_reasons"])
		assert.Equal(t, "delay", node["node_type"])
		for _, key := range []string{"visits", "completed", "failed_attempts", "skipped", "errors",
			"failed_journeys", "in_progress", "waiting", "retrying"} {
			assert.Equal(t, float64(0), node[key], key)
		}
	})

	t.Run("email counters keys", func(t *testing.T) {
		stats := NewNodeStats(NodeTypeEmail)
		stats.Email = &NodeEmailStats{Executions: 3, Queued: 1, NotSent: 1, Sent: 1}
		data, err := json.Marshal(stats)
		require.NoError(t, err)
		var got map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &got))
		email := got["email"].(map[string]interface{})
		for _, key := range []string{"executions", "queued", "not_sent", "sent", "delivered", "opened",
			"opens_tracked", "clicked", "clicks_tracked", "failed", "bounced", "complained", "unsubscribed"} {
			assert.Contains(t, email, key)
		}
		assert.Equal(t, float64(3), email["executions"])
	})

	t.Run("an empty response has nodes as an object", func(t *testing.T) {
		data, err := json.Marshal(NewNodeStatsResponse("auto1", nil, nil))
		require.NoError(t, err)
		assert.JSONEq(t, `{"automation_id":"auto1","nodes":{}}`, string(data))
	})
}

func TestJourneyCounts_Add(t *testing.T) {
	var c JourneyCounts
	c.Add(ContactAutomationStatusActive, 2)
	c.Add(ContactAutomationStatusCompleted, 3)
	c.Add(ContactAutomationStatusExited, 4)
	c.Add(ContactAutomationStatusFailed, 5)
	c.Add(ContactAutomationStatus("archived"), 6)
	assert.Equal(t, JourneyCounts{Active: 2, Completed: 3, Exited: 4, Failed: 5, Total: 20}, c,
		"an unknown status counts in total only")

	data, err := json.Marshal(JourneyCounts{})
	require.NoError(t, err)
	assert.JSONEq(t, `{"active":0,"completed":0,"exited":0,"failed":0,"total":0}`, string(data))
}
