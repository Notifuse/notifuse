//go:build integration
// +build integration

package integration

import (
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// The broadcasts table shows one tab per status with its own number, and its 30-day results
// keep the broadcasts that started in the window: broadcasts.list returns status_counts on
// request and filters on started_after.
func TestBroadcastListCounts(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()
	factory := suite.DataFactory
	client := suite.APIClient

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))
	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(workspace.ID)

	now := time.Now().UTC().Truncate(time.Second)
	recent := now.Add(-2 * 24 * time.Hour)
	old := now.Add(-40 * 24 * time.Hour)
	startedAt := func(at time.Time) testutil.BroadcastOption {
		return func(b *domain.Broadcast) { b.StartedAt = &at }
	}
	create := func(name string, status domain.BroadcastStatus, opts ...testutil.BroadcastOption) string {
		t.Helper()
		b, err := factory.CreateBroadcast(workspace.ID, append([]testutil.BroadcastOption{
			testutil.WithBroadcastName(name), testutil.WithBroadcastStatus(status),
		}, opts...)...)
		require.NoError(t, err)
		return b.ID
	}

	draft := create("Autumn draft", domain.BroadcastStatusDraft)
	scheduled := create("Winter sale scheduled", domain.BroadcastStatusScheduled)
	processedRecent := create("Autumn sale", domain.BroadcastStatusProcessed, startedAt(recent))
	processedOld := create("Summer sale", domain.BroadcastStatusProcessed, startedAt(old))
	pausedRecent := create("Autumn newsletter", domain.BroadcastStatusPaused, startedAt(recent))

	type listResponse struct {
		Broadcasts []struct {
			ID string `json:"id"`
		} `json:"broadcasts"`
		TotalCount   int             `json:"total_count"`
		StatusCounts *map[string]int `json:"status_counts"`
	}
	list := func(t *testing.T, params map[string]string) listResponse {
		t.Helper()
		resp, err := client.ListBroadcasts(params)
		require.NoError(t, err)
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode, "broadcasts.list: %s", body)
		var got listResponse // fresh on each call: decoding into a reused map merges keys
		require.NoError(t, json.Unmarshal(body, &got))
		return got
	}
	ids := func(r listResponse) []string {
		out := make([]string, 0, len(r.Broadcasts))
		for _, b := range r.Broadcasts {
			out = append(out, b.ID)
		}
		sort.Strings(out)
		return out
	}
	sorted := func(in ...string) []string {
		sort.Strings(in)
		return in
	}

	t.Run("status_counts counts every status, and is absent unless asked for", func(t *testing.T) {
		got := list(t, map[string]string{"with_status_counts": "true"})
		require.NotNil(t, got.StatusCounts)
		assert.Equal(t, map[string]int{"draft": 1, "scheduled": 1, "processed": 2, "paused": 1}, *got.StatusCounts)
		assert.Equal(t, 5, got.TotalCount)

		assert.Nil(t, list(t, map[string]string{}).StatusCounts)
		assert.Nil(t, list(t, map[string]string{"with_status_counts": "false"}).StatusCounts)
	})

	t.Run("the counts follow the search, not the requested status", func(t *testing.T) {
		got := list(t, map[string]string{"with_status_counts": "true", "search": "autumn", "status": "processed"})
		require.NotNil(t, got.StatusCounts)
		assert.Equal(t, map[string]int{"draft": 1, "processed": 1, "paused": 1}, *got.StatusCounts)
		assert.Equal(t, []string{processedRecent}, ids(got))
		assert.Equal(t, 1, got.TotalCount)
	})

	t.Run("started_after leaves out older and never-started broadcasts", func(t *testing.T) {
		got := list(t, map[string]string{"started_after": now.Add(-30 * 24 * time.Hour).Format(time.RFC3339)})
		assert.Equal(t, sorted(processedRecent, pausedRecent), ids(got))
		assert.Equal(t, 2, got.TotalCount)
		assert.NotContains(t, ids(got), processedOld)
		assert.NotContains(t, ids(got), draft)
		assert.NotContains(t, ids(got), scheduled)
	})

	t.Run("status, search, started_after and status_counts together", func(t *testing.T) {
		got := list(t, map[string]string{
			"with_status_counts": "true",
			"status":             "processed",
			"search":             "sale",
			"started_after":      now.Add(-30 * 24 * time.Hour).Format(time.RFC3339),
		})
		assert.Equal(t, []string{processedRecent}, ids(got))
		assert.Equal(t, 1, got.TotalCount)
		require.NotNil(t, got.StatusCounts)
		assert.Equal(t, map[string]int{"processed": 1}, *got.StatusCounts)
	})

	t.Run("nothing matching gives an empty object", func(t *testing.T) {
		got := list(t, map[string]string{"with_status_counts": "true", "search": "no broadcast has this name"})
		require.NotNil(t, got.StatusCounts)
		assert.Empty(t, *got.StatusCounts)
		assert.Empty(t, got.Broadcasts)
	})

	t.Run("an unreadable started_after or with_status_counts is refused", func(t *testing.T) {
		for _, params := range []map[string]string{
			{"started_after": "yesterday"},
			{"with_status_counts": "yes"},
		} {
			resp, err := client.ListBroadcasts(params)
			require.NoError(t, err)
			_ = resp.Body.Close()
			assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "%v", params)
		}
	})
}
