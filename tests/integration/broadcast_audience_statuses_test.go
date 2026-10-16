package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBroadcastAudienceStatusesAsDelivered sends real broadcasts, created through the API,
// to a list holding one contact of each status, and reads from Mailpit who received them.
// A create body that left exclude_unsubscribed out used to read it as false and mail every
// row on the list, bounced and complained addresses included.
func TestBroadcastAudienceStatusesAsDelivered(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

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

	list, err := factory.CreateList(workspace.ID)
	require.NoError(t, err)

	tag := uuid.New().String()[:8]
	emailWith := map[domain.ContactListStatus]string{}
	for _, status := range []domain.ContactListStatus{
		domain.ContactListStatusActive,
		domain.ContactListStatusUnsubscribed,
		domain.ContactListStatusBounced,
		domain.ContactListStatusComplained,
	} {
		email := fmt.Sprintf("audience-%s-%s@example.com", status, tag)
		_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
		require.NoError(t, err)
		_, err = factory.CreateContactList(workspace.ID,
			testutil.WithContactListEmail(email),
			testutil.WithContactListListID(list.ID),
			testutil.WithContactListStatus(status))
		require.NoError(t, err)
		emailWith[status] = email
	}

	// sendToList creates a broadcast through the API with the given audience body, sends it
	// now, and returns who Mailpit received it for and how many emails the broadcast reports.
	sendToList := func(t *testing.T, audience map[string]interface{}) (received map[string]bool, enqueued int) {
		t.Helper()
		subject := "audience-statuses-" + uuid.New().String()[:8]
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

		resp, err = client.ScheduleBroadcast(map[string]interface{}{
			"workspace_id": workspace.ID,
			"id":           created.Broadcast.ID,
			"send_now":     true,
		})
		require.NoError(t, err)
		body, _ = io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode, "schedule broadcast: %s", body)

		_, err = testutil.WaitForBroadcastStatusWithExecution(t, client, created.Broadcast.ID,
			[]string{"processed", "completed"}, 60*time.Second)
		require.NoError(t, err)

		resp, err = client.GetBroadcast(created.Broadcast.ID)
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
		enqueued = got.Broadcast.EnqueuedCount

		// The queue worker sends after the broadcast is processed. Mailpit can't hold more
		// emails for this subject than were enqueued, so once it holds that many the set of
		// recipients is final.
		testutil.WaitForCondition(t, func() bool {
			received, err = testutil.GetAllMailpitRecipients(t, subject)
			return err == nil && len(received) >= enqueued
		}, 30*time.Second, "waiting for the broadcast emails of "+subject)
		return received, enqueued
	}

	t.Run("exclude_unsubscribed omitted: only active contacts", func(t *testing.T) {
		received, enqueued := sendToList(t, map[string]interface{}{"list": list.ID})

		assert.Equal(t, map[string]bool{emailWith[domain.ContactListStatusActive]: true}, received)
		assert.Equal(t, 1, enqueued)
	})

	t.Run("exclude_unsubscribed true: only active contacts", func(t *testing.T) {
		received, enqueued := sendToList(t, map[string]interface{}{"list": list.ID, "exclude_unsubscribed": true})

		assert.Equal(t, map[string]bool{emailWith[domain.ContactListStatusActive]: true}, received)
		assert.Equal(t, 1, enqueued)
	})

	t.Run("exclude_unsubscribed false: unsubscribed contacts too, never bounced or complained", func(t *testing.T) {
		received, enqueued := sendToList(t, map[string]interface{}{"list": list.ID, "exclude_unsubscribed": false})

		assert.Equal(t, map[string]bool{
			emailWith[domain.ContactListStatusActive]:       true,
			emailWith[domain.ContactListStatusUnsubscribed]: true,
		}, received)
		assert.Equal(t, 2, enqueued)
	})
}
