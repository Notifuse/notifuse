package integration

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMessagesListCursorSubSecondTimestamps reproduces the broadcast log truncation
// seen on bulk sends: message_history rows inserted in tight batches share the same
// wall-clock second, and the list cursor must not collapse them together.
func TestMessagesListCursorSubSecondTimestamps(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer func() { suite.Cleanup() }()

	client := suite.APIClient
	factory := suite.DataFactory

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))
	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(workspace.ID)

	broadcastID := "09bdc804cbaf2539ee206c4e848e714b"

	// Same distribution as a real bulk send: 120 rows over 3 sub-second timestamps.
	base := time.Date(2026, 8, 6, 20, 53, 20, 0, time.UTC)
	batches := []struct {
		offset time.Duration
		count  int
	}{
		{760394 * time.Microsecond, 50},
		{843554 * time.Microsecond, 50},
		{903753 * time.Microsecond, 20},
	}

	total := 0
	for bi, b := range batches {
		ts := base.Add(b.offset)
		for i := 0; i < b.count; i++ {
			_, err := factory.CreateMessageHistory(workspace.ID,
				testutil.WithMessageBroadcast(broadcastID),
				testutil.WithMessageTemplate("tpl0000000000000000000000000000"),
				testutil.WithMessageContact(fmt.Sprintf("recipient-%d-%d@example.com", bi, i)),
				func(m *domain.MessageHistory) {
					m.CreatedAt = ts
					m.UpdatedAt = ts
					m.SentAt = ts
				},
			)
			require.NoError(t, err)
			total++
		}
	}
	require.Equal(t, 120, total)

	// Page through exactly like the console's "Load more" button does.
	seen := map[string]bool{}
	cursor := ""
	pages := 0
	for {
		params := map[string]string{
			"workspace_id": workspace.ID,
			"broadcast_id": broadcastID,
			"limit":        "20",
		}
		if cursor != "" {
			params["cursor"] = cursor
		}

		resp, err := client.Get("/api/messages.list", params)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var result domain.MessageListResult
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
		_ = resp.Body.Close()

		for _, m := range result.Messages {
			assert.False(t, seen[m.ID], "message %s returned on more than one page", m.ID)
			seen[m.ID] = true
		}

		decoded := ""
		if result.NextCursor != "" {
			raw, _ := base64.StdEncoding.DecodeString(result.NextCursor)
			decoded = string(raw)
		}
		t.Logf("page %d: got %d rows, has_more=%v, cursor=%q (running total %d)",
			pages, len(result.Messages), result.HasMore, decoded, len(seen))

		pages++
		if !result.HasMore || result.NextCursor == "" {
			break
		}
		cursor = result.NextCursor
		require.Less(t, pages, 50, "pagination did not terminate")
	}

	assert.Equal(t, 120, len(seen),
		"paginating messages.list must return every row of the broadcast; got %d of 120 across %d page(s)",
		len(seen), pages)
}
