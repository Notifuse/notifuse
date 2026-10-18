package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// segmentAudience is a list and two segments, A and B, that share contacts:
//
//	alice  on the list, in A and B
//	bob    on the list, in A
//	carol  on the list, in no segment
//	dave   in A and B, not on the list
//	erin   on the list as unsubscribed, in A
//
// The memberships are written straight to contact_segments. The integration server runs
// without the task scheduler, and factory workspaces have no segment queue task, so
// nothing re-evaluates them.
type segmentAudience struct {
	listID                        string
	a, b                          string
	alice, bob, carol, dave, erin string
}

func seedSegmentAudience(t *testing.T, factory *testutil.TestDataFactory, workspaceID string) segmentAudience {
	t.Helper()
	db, err := factory.GetWorkspaceDB(workspaceID)
	require.NoError(t, err)
	list, err := factory.CreateList(workspaceID)
	require.NoError(t, err)

	ts := time.Now().UnixNano()
	email := func(name string) string { return fmt.Sprintf("%s-%d@example.com", name, ts) }
	s := segmentAudience{
		listID: list.ID,
		a:      "segA",
		b:      "segB",
		alice:  email("alice"),
		bob:    email("bob"),
		carol:  email("carol"),
		dave:   email("dave"),
		erin:   email("erin"),
	}

	for _, id := range []string{s.a, s.b} {
		_, err := db.Exec(`
			INSERT INTO segments (id, name, color, tree, timezone, version, status, generated_sql, generated_args)
			VALUES ($1, $1, 'blue', '{}'::jsonb, 'UTC', 1, 'active', 'SELECT 1', '[]'::jsonb)`, id)
		require.NoError(t, err)
	}

	for _, c := range []struct {
		email    string
		onList   domain.ContactListStatus // empty: not on the list
		segments []string
	}{
		{s.alice, domain.ContactListStatusActive, []string{s.a, s.b}},
		{s.bob, domain.ContactListStatusActive, []string{s.a}},
		{s.carol, domain.ContactListStatusActive, nil},
		{s.dave, "", []string{s.a, s.b}},
		{s.erin, domain.ContactListStatusUnsubscribed, []string{s.a}},
	} {
		_, err := factory.CreateContact(workspaceID, testutil.WithContactEmail(c.email))
		require.NoError(t, err)
		if c.onList != "" {
			_, err = factory.CreateContactList(workspaceID,
				testutil.WithContactListEmail(c.email),
				testutil.WithContactListListID(list.ID),
				testutil.WithContactListStatus(c.onList))
			require.NoError(t, err)
		}
		for _, segmentID := range c.segments {
			_, err := db.Exec(`
				INSERT INTO contact_segments (email, segment_id, version, matched_at)
				VALUES ($1, $2, 1, NOW())`, c.email, segmentID)
			require.NoError(t, err)
		}
	}
	return s
}

// TestBroadcastSegmentAudience_Integration runs the recipient queries of a broadcast
// narrowed to segments against a real Postgres database, the way the broadcast
// orchestrator does (GetTotalRecipientCount, FetchBatch). A broadcast sends to the
// contacts on its list who are in at least one of its segments, each once: a contact in
// two of them used to come back once per segment, and was sent the broadcast twice.
func TestBroadcastSegmentAudience_Integration(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	factory := suite.DataFactory
	contactRepo := suite.ServerManager.GetApp().GetContactRepository()
	ctx := context.Background()

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))

	s := seedSegmentAudience(t, factory, workspace.ID)

	recipients := func(t *testing.T, audience domain.AudienceSettings, limit int, afterEmail string) []string {
		t.Helper()
		contacts, err := contactRepo.GetContactsForBroadcast(ctx, workspace.ID, audience, limit, afterEmail)
		require.NoError(t, err)
		emails := make([]string, 0, len(contacts))
		for _, c := range contacts {
			emails = append(emails, c.Contact.Email)
		}
		return emails
	}
	counted := func(t *testing.T, audience domain.AudienceSettings) int {
		t.Helper()
		n, err := contactRepo.CountContactsForBroadcast(ctx, workspace.ID, audience)
		require.NoError(t, err)
		return n
	}

	both := domain.AudienceSettings{List: s.listID, Segments: []string{s.a, s.b}, ExcludeUnsubscribed: true}

	t.Run("a contact in both segments is counted once", func(t *testing.T) {
		assert.Equal(t, 2, counted(t, both), "alice and bob")
	})

	t.Run("a contact in both segments is returned once", func(t *testing.T) {
		assert.Equal(t, []string{s.alice, s.bob}, recipients(t, both, 100, ""))
	})

	t.Run("pages of one return each recipient once, then nothing", func(t *testing.T) {
		assert.Equal(t, []string{s.alice}, recipients(t, both, 1, ""))
		assert.Equal(t, []string{s.bob}, recipients(t, both, 1, s.alice))
		assert.Empty(t, recipients(t, both, 1, s.bob))
	})

	t.Run("one segment", func(t *testing.T) {
		audience := domain.AudienceSettings{List: s.listID, Segments: []string{s.a}, ExcludeUnsubscribed: true}
		assert.Equal(t, []string{s.alice, s.bob}, recipients(t, audience, 100, ""))
		assert.Equal(t, 2, counted(t, audience))
	})

	t.Run("a broadcast that keeps unsubscribed contacts adds the unsubscribed member", func(t *testing.T) {
		audience := domain.AudienceSettings{List: s.listID, Segments: []string{s.a}, ExcludeUnsubscribed: false}
		assert.Equal(t, []string{s.alice, s.bob, s.erin}, recipients(t, audience, 100, ""))
		assert.Equal(t, 3, counted(t, audience))

		audience.Segments = []string{s.a, s.b}
		assert.Equal(t, []string{s.alice, s.bob, s.erin}, recipients(t, audience, 100, ""))
		assert.Equal(t, 3, counted(t, audience))
	})

	// Broadcasts stored before a list became required can still name segments alone
	t.Run("segments without a list return every contact in either segment once", func(t *testing.T) {
		audience := domain.AudienceSettings{Segments: []string{s.a, s.b}}
		want := []string{s.alice, s.bob, s.dave, s.erin}
		assert.Equal(t, want, recipients(t, audience, 100, ""))
		assert.Equal(t, 4, counted(t, audience))

		// Pages of one: each later page is bounded by its cursor
		var paged []string
		cursor := ""
		for range want {
			page := recipients(t, audience, 1, cursor)
			require.Len(t, page, 1)
			paged = append(paged, page[0])
			cursor = page[0]
		}
		assert.Equal(t, want, paged)
		assert.Empty(t, recipients(t, audience, 1, cursor))
	})
}

// TestBroadcastSegmentAudienceSend_Integration sends a broadcast to a list narrowed to two
// segments that share a contact, and checks that contact is sent one email.
func TestBroadcastSegmentAudienceSend_Integration(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	// Scheduled broadcasts are sent by the email queue worker, which the harness only runs
	// on request.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, suite.ServerManager.StartBackgroundWorkers(ctx))

	client := suite.APIClient
	factory := suite.DataFactory

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))
	_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID)
	require.NoError(t, err)
	require.NoError(t, client.Login(user.Email, "password"))
	client.SetWorkspaceID(workspace.ID)

	s := seedSegmentAudience(t, factory, workspace.ID)
	db, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	subject := "segment-audience-" + uuid.New().String()[:8]
	template, err := factory.CreateTemplate(workspace.ID, testutil.WithTemplateSubject(subject))
	require.NoError(t, err)

	resp, err := client.CreateBroadcast(map[string]interface{}{
		"workspace_id": workspace.ID,
		"name":         subject,
		"audience": map[string]interface{}{
			"list":                 s.listID,
			"segments":             []string{s.a, s.b},
			"exclude_unsubscribed": true,
		},
		"test_settings": map[string]interface{}{
			"enabled":    false,
			"variations": []map[string]interface{}{{"template_id": template.ID}},
		},
	})
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode, "create broadcast: %s", body)
	var created struct {
		Broadcast struct {
			ID string `json:"id"`
		} `json:"broadcast"`
	}
	require.NoError(t, json.Unmarshal(body, &created))
	broadcastID := created.Broadcast.ID

	resp, err = client.ScheduleBroadcast(map[string]interface{}{
		"workspace_id": workspace.ID,
		"id":           broadcastID,
		"send_now":     true,
	})
	require.NoError(t, err)
	body, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "schedule broadcast: %s", body)

	_, err = testutil.WaitForBroadcastStatusWithExecution(t, client, broadcastID,
		[]string{"processed", "completed"}, 60*time.Second)
	require.NoError(t, err)

	// enqueued_count is final once the broadcast is processed
	resp, err = client.GetBroadcast(broadcastID)
	require.NoError(t, err)
	body, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "get broadcast: %s", body)
	var got struct {
		Broadcast struct {
			EnqueuedCount int `json:"enqueued_count"`
		} `json:"broadcast"`
	}
	require.NoError(t, json.Unmarshal(body, &got))
	assert.Equal(t, 2, got.Broadcast.EnqueuedCount, "one email each for alice and bob")

	// The send's progress and A/B split are computed from the recipient total
	task, err := suite.ServerManager.GetApp().GetTaskRepository().GetTaskByBroadcastID(ctx, workspace.ID, broadcastID)
	require.NoError(t, err)
	require.NotNil(t, task.State)
	require.NotNil(t, task.State.SendBroadcast)
	assert.Equal(t, 2, task.State.SendBroadcast.TotalRecipients)

	// The queue worker records a message for each email it sends, after the broadcast is
	// processed. It can't record more than were enqueued, so once it has that many the
	// messages are final.
	sentTo := map[string]int{}
	testutil.WaitForCondition(t, func() bool {
		rows, err := db.QueryContext(ctx,
			`SELECT contact_email, COUNT(*) FROM message_history WHERE broadcast_id = $1 GROUP BY contact_email`,
			broadcastID)
		if err != nil {
			return false
		}
		defer func() { _ = rows.Close() }()
		counts, total := map[string]int{}, 0
		for rows.Next() {
			var email string
			var n int
			if rows.Scan(&email, &n) != nil {
				return false
			}
			counts[email] = n
			total += n
		}
		if rows.Err() != nil {
			return false
		}
		sentTo = counts
		return total >= got.Broadcast.EnqueuedCount
	}, 30*time.Second, "waiting for the messages of broadcast "+broadcastID)
	assert.Equal(t, map[string]int{s.alice: 1, s.bob: 1}, sentTo, "alice, who is in both segments, is sent one email")
}
