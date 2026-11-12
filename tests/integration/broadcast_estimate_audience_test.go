//go:build integration
// +build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
)

// The composer counts the recipients of the audience being built with
// broadcasts.estimateAudience. The count must be the one the send then uses, so it is
// checked against a real send of the same audience, not against the function it calls.
func TestBroadcastEstimateAudience(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	// The send is carried by the email queue worker, which the harness only runs on request.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, suite.ServerManager.StartBackgroundWorkers(ctx))

	client := suite.APIClient
	factory := suite.DataFactory

	owner, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(owner.ID, workspace.ID, "owner"))
	_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID)
	require.NoError(t, err)
	db, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	// A double opt-in list, so the contact who never confirmed is left out:
	//
	//	alice       active, in segment A
	//	bob         active
	//	erin        unsubscribed, in segment A
	//	bounced     bounced
	//	complained  complained
	//	pending     pending double opt-in, in segment A
	list, err := factory.CreateList(workspace.ID, testutil.WithListDoubleOptin(true))
	require.NoError(t, err)
	ts := time.Now().UnixNano()
	email := func(name string) string { return fmt.Sprintf("%s-%d@example.com", name, ts) }
	segmentA, segmentNobody := "segEstimateA", "segEstimateNone"
	for _, id := range []string{segmentA, segmentNobody} {
		_, err := db.Exec(`
			INSERT INTO segments (id, name, color, tree, timezone, version, status, generated_sql, generated_args)
			VALUES ($1, $1, 'blue', '{}'::jsonb, 'UTC', 1, 'active', 'SELECT 1', '[]'::jsonb)`, id)
		require.NoError(t, err)
	}
	for _, c := range []struct {
		name      string
		status    domain.ContactListStatus
		inSegment bool
	}{
		{"alice", domain.ContactListStatusActive, true},
		{"bob", domain.ContactListStatusActive, false},
		{"erin", domain.ContactListStatusUnsubscribed, true},
		{"bounced", domain.ContactListStatusBounced, false},
		{"complained", domain.ContactListStatusComplained, false},
		{"pending", domain.ContactListStatusPending, true},
	} {
		_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(email(c.name)))
		require.NoError(t, err)
		_, err = factory.CreateContactList(workspace.ID,
			testutil.WithContactListEmail(email(c.name)),
			testutil.WithContactListListID(list.ID),
			testutil.WithContactListStatus(c.status))
		require.NoError(t, err)
		if c.inSegment {
			_, err := db.Exec(`INSERT INTO contact_segments (email, segment_id, version, matched_at) VALUES ($1, $2, 1, NOW())`,
				email(c.name), segmentA)
			require.NoError(t, err)
		}
	}

	require.NoError(t, client.Login(owner.Email, "password"))
	client.SetWorkspaceID(workspace.ID)

	estimateRaw := func(t *testing.T, body map[string]interface{}) (int, int) {
		t.Helper()
		resp, err := client.Post("/api/broadcasts.estimateAudience", body)
		require.NoError(t, err)
		raw, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return resp.StatusCode, -1
		}
		var got struct {
			Count *int `json:"count"`
		}
		require.NoError(t, json.Unmarshal(raw, &got), "%s", raw)
		require.NotNil(t, got.Count, "%s", raw)
		return resp.StatusCode, *got.Count
	}
	estimate := func(t *testing.T, audience map[string]interface{}) int {
		t.Helper()
		status, count := estimateRaw(t, map[string]interface{}{"workspace_id": workspace.ID, "audience": audience})
		require.Equal(t, http.StatusOK, status)
		return count
	}

	t.Run("the estimate of the list is the recipient count of a real send", func(t *testing.T) {
		audience := map[string]interface{}{"list": list.ID, "exclude_unsubscribed": true}
		estimated := estimate(t, audience)
		assert.Equal(t, 2, estimated, "alice and bob")

		subject := "estimate-audience-" + uuid.New().String()[:8]
		template, err := factory.CreateTemplate(workspace.ID, testutil.WithTemplateSubject(subject))
		require.NoError(t, err)
		resp, err := client.CreateBroadcast(map[string]interface{}{
			"workspace_id": workspace.ID,
			"name":         subject,
			"audience":     audience,
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
			"workspace_id": workspace.ID, "id": broadcastID, "send_now": true,
		})
		require.NoError(t, err)
		body, _ = io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode, "schedule broadcast: %s", body)

		_, err = testutil.WaitForBroadcastStatusWithExecution(t, client, broadcastID,
			[]string{"processed", "completed"}, 60*time.Second)
		require.NoError(t, err)

		resp, err = client.GetBroadcast(broadcastID)
		require.NoError(t, err)
		body, _ = io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode, "get broadcast: %s", body)
		var sent struct {
			Broadcast struct {
				EnqueuedCount int `json:"enqueued_count"`
			} `json:"broadcast"`
		}
		require.NoError(t, json.Unmarshal(body, &sent))
		assert.Equal(t, estimated, sent.Broadcast.EnqueuedCount)

		task, err := suite.ServerManager.GetApp().GetTaskRepository().GetTaskByBroadcastID(ctx, workspace.ID, broadcastID)
		require.NoError(t, err)
		require.NotNil(t, task.State)
		require.NotNil(t, task.State.SendBroadcast)
		assert.Equal(t, estimated, task.State.SendBroadcast.TotalRecipients)
	})

	t.Run("exclude_unsubscribed omitted counts as true", func(t *testing.T) {
		assert.Equal(t, 2, estimate(t, map[string]interface{}{"list": list.ID}))
	})

	t.Run("keeping unsubscribed contacts adds the unsubscribed one only", func(t *testing.T) {
		assert.Equal(t, 3, estimate(t, map[string]interface{}{"list": list.ID, "exclude_unsubscribed": false}),
			"alice, bob and erin; never the bounced, complained or pending contact")
	})

	t.Run("a segment narrows the list", func(t *testing.T) {
		assert.Equal(t, 1, estimate(t, map[string]interface{}{"list": list.ID, "segments": []string{segmentA}}), "alice")
		assert.Equal(t, 2, estimate(t, map[string]interface{}{
			"list": list.ID, "segments": []string{segmentA}, "exclude_unsubscribed": false,
		}), "alice and erin")
	})

	t.Run("a segment matching nobody, or an unknown list, gives 0", func(t *testing.T) {
		assert.Equal(t, 0, estimate(t, map[string]interface{}{"list": list.ID, "segments": []string{segmentNobody}}))
		assert.Equal(t, 0, estimate(t, map[string]interface{}{"list": "no-such-list"}))
	})

	t.Run("an audience without a list is refused", func(t *testing.T) {
		status, _ := estimateRaw(t, map[string]interface{}{"workspace_id": workspace.ID, "audience": map[string]interface{}{}})
		assert.Equal(t, http.StatusBadRequest, status)
	})

	t.Run("a member without the read permission it needs gets 403", func(t *testing.T) {
		for _, tc := range []struct {
			name     string
			resource domain.PermissionResource
			audience map[string]interface{}
		}{
			{"without lists.read", domain.PermissionResourceLists, map[string]interface{}{"list": list.ID}},
			{"without broadcasts.read", domain.PermissionResourceBroadcasts, map[string]interface{}{"list": list.ID}},
			{"without segments.read, for an audience with segments", domain.PermissionResourceSegments,
				map[string]interface{}{"list": list.ID, "segments": []string{segmentA}}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				member, err := factory.CreateUser()
				require.NoError(t, err)
				perms := domain.NewFullPermissions()
				perms[tc.resource] = domain.ResourcePermissions{Read: false, Write: false}
				require.NoError(t, factory.AddUserToWorkspaceWithPermissions(member.ID, workspace.ID, "member", perms))
				require.NoError(t, client.Login(member.Email, "password"))
				defer func() { require.NoError(t, client.Login(owner.Email, "password")) }()

				status, _ := estimateRaw(t, map[string]interface{}{"workspace_id": workspace.ID, "audience": tc.audience})
				assert.Equal(t, http.StatusForbidden, status)
			})
		}
	})

	t.Run("a member without segments.read can still count an audience without segments", func(t *testing.T) {
		member, err := factory.CreateUser()
		require.NoError(t, err)
		perms := domain.NewFullPermissions()
		perms[domain.PermissionResourceSegments] = domain.ResourcePermissions{Read: false, Write: false}
		require.NoError(t, factory.AddUserToWorkspaceWithPermissions(member.ID, workspace.ID, "member", perms))
		require.NoError(t, client.Login(member.Email, "password"))
		defer func() { require.NoError(t, client.Login(owner.Email, "password")) }()

		assert.Equal(t, 2, estimate(t, map[string]interface{}{"list": list.ID}))
	})
}
