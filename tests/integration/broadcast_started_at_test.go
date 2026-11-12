//go:build integration
// +build integration

package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/migrations"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// A scheduled broadcast reaches sending only through the task that sends it, and that path
// never set started_at: only "send now" and a resume did. So broadcasts.list?started_after
// left every scheduled broadcast out of the 30-day results. The first sending run now
// records the date.
func TestBroadcastScheduledSendRecordsStartedAt(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()
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
	db, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	list, err := factory.CreateList(workspace.ID)
	require.NoError(t, err)
	contact, err := factory.CreateContact(workspace.ID)
	require.NoError(t, err)
	_, err = factory.CreateContactList(workspace.ID,
		testutil.WithContactListEmail(contact.Email),
		testutil.WithContactListListID(list.ID),
		testutil.WithContactListStatus(domain.ContactListStatusActive))
	require.NoError(t, err)
	template, err := factory.CreateTemplate(workspace.ID)
	require.NoError(t, err)

	resp, err := client.CreateBroadcast(map[string]interface{}{
		"workspace_id": workspace.ID,
		"name":         "Scheduled start date",
		"audience":     map[string]interface{}{"list": list.ID, "exclude_unsubscribed": true},
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

	// Past due, so the task runs at the next tick. Minutes only: the schedule has no seconds.
	scheduledFor := time.Now().UTC().Add(-2 * time.Minute)
	resp, err = client.ScheduleBroadcast(map[string]interface{}{
		"workspace_id":           workspace.ID,
		"id":                     broadcastID,
		"send_now":               false,
		"scheduled_date":         scheduledFor.Format("2006-01-02"),
		"scheduled_time":         scheduledFor.Format("15:04"),
		"timezone":               "UTC",
		"use_recipient_timezone": false,
	})
	require.NoError(t, err)
	body, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "schedule broadcast: %s", body)

	startedAtOf := func(t *testing.T) sql.NullTime {
		t.Helper()
		var startedAt sql.NullTime
		require.NoError(t, db.QueryRowContext(ctx, `SELECT started_at FROM broadcasts WHERE id = $1`, broadcastID).Scan(&startedAt))
		return startedAt
	}
	require.False(t, startedAtOf(t).Valid, "scheduling sets no start date")

	beforeSend := time.Now().UTC()
	_, err = testutil.WaitForBroadcastStatusWithExecution(t, client, broadcastID,
		[]string{"processed", "completed"}, 60*time.Second)
	require.NoError(t, err)

	startedAt := startedAtOf(t)
	require.True(t, startedAt.Valid, "the sending run records the start date")
	assert.False(t, startedAt.Time.Before(beforeSend.Add(-time.Second)), "started_at %s, sending from %s", startedAt.Time, beforeSend)
	assert.False(t, startedAt.Time.After(time.Now().UTC()))

	resp, err = client.ListBroadcasts(map[string]string{
		"started_after": beforeSend.Add(-time.Hour).Format(time.RFC3339),
	})
	require.NoError(t, err)
	body, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "broadcasts.list: %s", body)
	var listed struct {
		Broadcasts []struct {
			ID string `json:"id"`
		} `json:"broadcasts"`
	}
	require.NoError(t, json.Unmarshal(body, &listed))
	require.Len(t, listed.Broadcasts, 1)
	assert.Equal(t, broadcastID, listed.Broadcasts[0].ID, "kept by started_after")
}

// v44 dates the broadcasts that started sending before the start date was recorded on the
// first sending run, from their first message.
func TestV44BroadcastStartedAtBackfill(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()
	factory := suite.DataFactory
	ctx := context.Background()

	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	db, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)
	template, err := factory.CreateTemplate(workspace.ID)
	require.NoError(t, err)

	// PostgreSQL keeps microseconds.
	firstMessage := time.Now().UTC().Add(-10 * 24 * time.Hour).Truncate(time.Microsecond)
	alreadySet := time.Now().UTC().Add(-20 * 24 * time.Hour).Truncate(time.Microsecond)

	broadcast := func(status domain.BroadcastStatus, startedAt *time.Time) string {
		t.Helper()
		b, err := factory.CreateBroadcast(workspace.ID, testutil.WithBroadcastStatus(status),
			func(b *domain.Broadcast) { b.StartedAt = startedAt })
		require.NoError(t, err)
		return b.ID
	}
	message := func(broadcastID string, at time.Time) {
		t.Helper()
		_, err := factory.CreateMessageHistory(workspace.ID,
			testutil.WithMessageTemplate(template.ID),
			testutil.WithMessageBroadcast(broadcastID),
			func(m *domain.MessageHistory) { m.CreatedAt, m.SentAt, m.UpdatedAt = at, at, at })
		require.NoError(t, err)
	}

	// A scheduled send of a released version: processed, messages, no start date.
	scheduledSend := broadcast(domain.BroadcastStatusProcessed, nil)
	message(scheduledSend, firstMessage.Add(time.Hour))
	message(scheduledSend, firstMessage)
	message(scheduledSend, firstMessage.Add(2*time.Hour))
	// Paused in the middle of a scheduled send.
	pausedSend := broadcast(domain.BroadcastStatusPaused, nil)
	message(pausedSend, firstMessage)
	// Sent now: its start date is kept.
	sentNow := broadcast(domain.BroadcastStatusProcessed, &alreadySet)
	message(sentNow, firstMessage)
	// Nothing was ever sent.
	noMessage := broadcast(domain.BroadcastStatusProcessed, nil)
	// A draft and a scheduled broadcast whose only messages are test sends to one recipient.
	draftWithTest := broadcast(domain.BroadcastStatusDraft, nil)
	message(draftWithTest, firstMessage)
	scheduledWithTest := broadcast(domain.BroadcastStatusScheduled, nil)
	message(scheduledWithTest, firstMessage)

	startedAtOf := func(t *testing.T, id string) sql.NullTime {
		t.Helper()
		var startedAt sql.NullTime
		require.NoError(t, db.QueryRowContext(ctx, `SELECT started_at FROM broadcasts WHERE id = $1`, id).Scan(&startedAt))
		return startedAt
	}

	for run := 1; run <= 2; run++ {
		tx, err := db.BeginTx(ctx, nil)
		require.NoError(t, err)
		require.NoError(t, (&migrations.V44Migration{}).UpdateWorkspace(ctx, &config.Config{}, workspace, tx), "run %d", run)
		require.NoError(t, tx.Commit())

		for name, want := range map[string]struct {
			id   string
			want *time.Time
		}{
			"a scheduled send gets its first message's time": {scheduledSend, &firstMessage},
			"a paused send gets its first message's time":    {pausedSend, &firstMessage},
			"a start date already set is kept":               {sentNow, &alreadySet},
			"a broadcast without messages keeps no date":     {noMessage, nil},
			"a draft with a test send keeps no date":         {draftWithTest, nil},
			"a scheduled broadcast with a test send too":     {scheduledWithTest, nil},
		} {
			got := startedAtOf(t, want.id)
			if want.want == nil {
				assert.False(t, got.Valid, "run %d: %s", run, name)
				continue
			}
			require.True(t, got.Valid, "run %d: %s", run, name)
			assert.True(t, want.want.Equal(got.Time), "run %d: %s: got %s, want %s", run, name, got.Time, *want.want)
		}
	}
}
