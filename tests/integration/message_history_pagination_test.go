package integration

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
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

// TestMessagesListOrderBy pages through a broadcast's messages sorted by a message date,
// as the broadcast drawer's recipients list does: every row carrying the date comes back
// once, in order, including rows whose dates are equal or apart by microseconds.
func TestMessagesListOrderBy(t *testing.T) {
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

	broadcastID := "bc0000000000000000000000000000ob"
	sent := time.Date(2026, 11, 24, 14, 27, 0, 0, time.UTC)
	tie := sent.Add(5 * time.Minute)
	sameSecond := sent.Add(9 * time.Minute)

	type row struct {
		id      string
		opened  *time.Time
		clicked *time.Time
		failed  bool
	}
	var rows []row
	at := func(t time.Time) *time.Time { return &t }
	for i := 0; i < 130; i++ {
		r := row{id: fmt.Sprintf("msg-%04d", i)}
		switch {
		case i < 40: // the same open date
			r.opened = at(tie)
		case i < 80: // opens in the same second, apart by microseconds
			r.opened = at(sameSecond.Add(time.Duration(i*7919%1000000) * time.Microsecond))
		case i < 100: // opens spread over hours
			r.opened = at(sent.Add(time.Duration(i) * 17 * time.Minute))
		case i < 120: // never opened
		default: // failed
			r.failed = true
		}
		if r.opened != nil && i%3 == 0 {
			r.clicked = at(r.opened.Add(time.Minute))
		}
		rows = append(rows, r)
		_, err := factory.CreateMessageHistory(workspace.ID,
			testutil.WithMessageBroadcast(broadcastID),
			testutil.WithMessageTemplate("tpl0000000000000000000000000000"),
			testutil.WithMessageContact(fmt.Sprintf("recipient-%d@example.com", i)),
			func(m *domain.MessageHistory) {
				m.ID = r.id
				m.CreatedAt = sent
				m.UpdatedAt = sent
				m.SentAt = sent
				m.DeliveredAt = nil
				m.OpenedAt = r.opened
				m.ClickedAt = r.clicked
				m.FailedAt = nil
				if r.failed {
					m.FailedAt = at(sent)
				}
			},
		)
		require.NoError(t, err)
	}

	list := func(t *testing.T, params map[string]string) []string {
		var ids []string
		cursor := ""
		for pages := 0; ; pages++ {
			require.Less(t, pages, 100, "pagination did not terminate")
			query := map[string]string{"workspace_id": workspace.ID, "broadcast_id": broadcastID, "limit": "7"}
			for k, v := range params {
				query[k] = v
			}
			if cursor != "" {
				query["cursor"] = cursor
			}
			resp, err := client.Get("/api/messages.list", query)
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, resp.StatusCode)
			var result domain.MessageListResult
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
			_ = resp.Body.Close()
			for _, m := range result.Messages {
				ids = append(ids, m.ID)
			}
			if !result.HasMore {
				return ids
			}
			cursor = result.NextCursor
		}
	}
	expected := func(date func(row) *time.Time, desc bool) []string {
		var kept []row
		for _, r := range rows {
			if date(r) != nil {
				kept = append(kept, r)
			}
		}
		sort.Slice(kept, func(i, j int) bool {
			a, b := *date(kept[i]), *date(kept[j])
			if !a.Equal(b) {
				return a.After(b) == desc
			}
			return (kept[i].id > kept[j].id) == desc
		})
		ids := make([]string, len(kept))
		for i, r := range kept {
			ids[i] = r.id
		}
		return ids
	}
	opened := func(r row) *time.Time { return r.opened }
	clicked := func(r row) *time.Time { return r.clicked }

	t.Run("opened_at descending", func(t *testing.T) {
		got := list(t, map[string]string{"is_opened": "true", "order_by": "opened_at"})
		assert.Equal(t, expected(opened, true), got)
	})

	t.Run("opened_at ascending", func(t *testing.T) {
		got := list(t, map[string]string{"order_by": "opened_at", "order": "asc"})
		assert.Equal(t, expected(opened, false), got, "rows without opened_at are left out")
	})

	t.Run("clicked_at with an exact email", func(t *testing.T) {
		got := list(t, map[string]string{"order_by": "clicked_at", "contact_email": "recipient-3@example.com"})
		assert.Equal(t, []string{"msg-0003"}, got)
		got = list(t, map[string]string{"order_by": "clicked_at"})
		assert.Equal(t, expected(clicked, true), got)
	})

	notFailed := func(r row) *time.Time {
		if r.failed {
			return nil
		}
		return &sent
	}
	everyRow := func(r row) *time.Time { return &sent }

	t.Run("sent_at without the failed messages", func(t *testing.T) {
		got := list(t, map[string]string{"order_by": "sent_at", "is_failed": "false"})
		assert.Equal(t, expected(notFailed, true), got)
	})

	t.Run("created_at ascending", func(t *testing.T) {
		got := list(t, map[string]string{"order": "asc"})
		assert.Equal(t, expected(everyRow, false), got)
	})

	t.Run("a cursor of another order answers 400", func(t *testing.T) {
		resp, err := client.Get("/api/messages.list", map[string]string{
			"workspace_id": workspace.ID, "broadcast_id": broadcastID, "limit": "7", "order_by": "opened_at",
		})
		require.NoError(t, err)
		var first domain.MessageListResult
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&first))
		_ = resp.Body.Close()
		require.NotEmpty(t, first.NextCursor)

		resp, err = client.Get("/api/messages.list", map[string]string{
			"workspace_id": workspace.ID, "broadcast_id": broadcastID, "limit": "7", "order_by": "clicked_at", "cursor": first.NextCursor,
		})
		require.NoError(t, err)
		_ = resp.Body.Close()
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}
