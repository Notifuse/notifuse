package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
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

// TestNotificationCenterPrivateListsAsDelivered checks what the signature in a contact's
// email lets the notification center do with private lists. The workspace decides who is
// on a private list, so a contact can see and leave the private lists they are on, but
// never join one, nor rejoin one they left. The signature comes from an email Mailpit
// received, and every request is the one the notification center sends.
func TestNotificationCenterPrivateListsAsDelivered(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

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

	// The harness runs with an empty API endpoint, which puts relative URLs in the email.
	// Point the workspace endpoint at the live server so every delivered link is absolute.
	serverURL := suite.ServerManager.GetURL()
	require.NoError(t, factory.SetWorkspaceCustomEndpointURL(workspace.ID, serverURL))

	workspaceDB, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)
	statusOf := func(t *testing.T, email, listID string) string {
		t.Helper()
		var status string
		err := workspaceDB.QueryRowContext(context.Background(),
			`SELECT status FROM contact_lists WHERE email = $1 AND list_id = $2`, email, listID).Scan(&status)
		require.NoError(t, err)
		return status
	}

	// A newsletter the contact is on, to have a real email carrying their signed link
	newsletter, err := factory.CreateList(workspace.ID, testutil.WithListPublic(true))
	require.NoError(t, err)
	email := fmt.Sprintf("private-lists-%s@example.com", uuid.New().String()[:8])
	_, err = factory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
	require.NoError(t, err)
	onList := func(t *testing.T, listID string, status domain.ContactListStatus) {
		t.Helper()
		_, err := factory.CreateContactList(workspace.ID,
			testutil.WithContactListEmail(email),
			testutil.WithContactListListID(listID),
			testutil.WithContactListStatus(status))
		require.NoError(t, err)
	}
	onList(t, newsletter.ID, domain.ContactListStatusActive)

	template1, err := factory.CreateTemplate(workspace.ID)
	require.NoError(t, err)
	template2, err := factory.CreateTemplate(workspace.ID)
	require.NoError(t, err)
	broadcast, err := factory.CreateBroadcast(workspace.ID,
		testutil.WithBroadcastABTesting([]string{template1.ID, template2.ID}),
		testutil.WithBroadcastAudience(domain.AudienceSettings{List: newsletter.ID, ExcludeUnsubscribed: true}))
	require.NoError(t, err)
	resp, err := client.SendBroadcastToIndividual(map[string]interface{}{
		"workspace_id":    workspace.ID,
		"broadcast_id":    broadcast.ID,
		"recipient_email": email,
		"template_id":     template1.ID,
	})
	require.NoError(t, err)
	sendBody, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "send broadcast: %s", sendBody)
	newsletterEmail, err := waitForEmailAndGetMessage(t, email, 20*time.Second)
	require.NoError(t, err, "the newsletter should reach Mailpit")
	signed := listUnsubscribeURL(t, newsletterEmail).Query()

	httpClient := &http.Client{Timeout: 10 * time.Second}

	// The notification center's requests (notification_center/src/api), built from the
	// signed link's parameters
	subscribe := func(t *testing.T, listID string) (int, string) {
		t.Helper()
		payload, err := json.Marshal(map[string]interface{}{
			"workspace_id": signed.Get("wid"),
			"contact":      map[string]interface{}{"id": "", "email": signed.Get("email"), "email_hmac": signed.Get("email_hmac")},
			"list_ids":     []string{listID},
		})
		require.NoError(t, err)
		resp, err := httpClient.Post(serverURL+"/subscribe", "application/json", bytes.NewReader(payload))
		require.NoError(t, err)
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return resp.StatusCode, string(body)
	}
	unsubscribe := func(t *testing.T, listID string) {
		t.Helper()
		payload, err := json.Marshal(map[string]interface{}{
			"wid":        signed.Get("wid"),
			"email":      signed.Get("email"),
			"email_hmac": signed.Get("email_hmac"),
			"lids":       []string{listID},
			"mid":        signed.Get("mid"),
		})
		require.NoError(t, err)
		resp, err := httpClient.Post(serverURL+"/unsubscribe", "application/json", bytes.NewReader(payload))
		require.NoError(t, err)
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode, "unsubscribe: %s", body)
	}
	// shownLists returns the list IDs the preferences page receives, failing unless the
	// contact object carries the same ones
	shownLists := func(t *testing.T) []string {
		t.Helper()
		query := url.Values{
			"workspace_id": {signed.Get("wid")},
			"email":        {signed.Get("email")},
			"email_hmac":   {signed.Get("email_hmac")},
		}
		resp, err := httpClient.Get(serverURL + "/preferences?" + query.Encode())
		require.NoError(t, err)
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode, "preferences: %s", body)

		var preferences struct {
			ContactLists []struct {
				ListID string `json:"list_id"`
			} `json:"contact_lists"`
			Contact struct {
				ContactLists []struct {
					ListID string `json:"list_id"`
				} `json:"contact_lists"`
			} `json:"contact"`
		}
		require.NoError(t, json.Unmarshal(body, &preferences))
		var ids, inContact []string
		for _, cl := range preferences.ContactLists {
			ids = append(ids, cl.ListID)
		}
		for _, cl := range preferences.Contact.ContactLists {
			inContact = append(inContact, cl.ListID)
		}
		require.Equal(t, ids, inContact, "the contact object must carry the same lists")
		return ids
	}

	t.Run("a private list the contact isn't on: joining is refused", func(t *testing.T) {
		private, err := factory.CreateList(workspace.ID)
		require.NoError(t, err)
		require.False(t, private.IsPublic)

		status, body := subscribe(t, private.ID)

		assert.Equal(t, http.StatusBadRequest, status)
		assert.JSONEq(t, `{"error":"list is not public"}`, body)
		var rows int
		require.NoError(t, workspaceDB.QueryRowContext(context.Background(),
			`SELECT COUNT(*) FROM contact_lists WHERE email = $1 AND list_id = $2`, email, private.ID).Scan(&rows))
		assert.Zero(t, rows)
		assert.NotContains(t, shownLists(t), private.ID)
	})

	t.Run("a private list the contact is on: shown, and they can leave it but not rejoin it", func(t *testing.T) {
		private, err := factory.CreateList(workspace.ID)
		require.NoError(t, err)
		onList(t, private.ID, domain.ContactListStatusActive)
		require.Contains(t, shownLists(t), private.ID)

		unsubscribe(t, private.ID)

		assert.Equal(t, string(domain.ContactListStatusUnsubscribed), statusOf(t, email, private.ID))
		assert.NotContains(t, shownLists(t), private.ID, "a private list they left is no longer theirs to see")

		status, body := subscribe(t, private.ID)

		assert.Equal(t, http.StatusBadRequest, status)
		assert.JSONEq(t, `{"error":"list is not public"}`, body)
		assert.Equal(t, string(domain.ContactListStatusUnsubscribed), statusOf(t, email, private.ID))
	})

	// Through the authenticated API the workspace still decides: it adds a contact to a
	// private list, including one they left
	t.Run("the API still adds the contact to a private list, even one they left", func(t *testing.T) {
		private, err := factory.CreateList(workspace.ID)
		require.NoError(t, err)
		onList(t, private.ID, domain.ContactListStatusUnsubscribed)

		resp, err := client.Post("/api/lists.subscribe", map[string]interface{}{
			"workspace_id": workspace.ID,
			"contact":      map[string]interface{}{"email": email},
			"list_ids":     []string{private.ID},
		})
		require.NoError(t, err)
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		require.Equal(t, http.StatusOK, resp.StatusCode, "lists.subscribe: %s", body)
		assert.NotEqual(t, string(domain.ContactListStatusUnsubscribed), statusOf(t, email, private.ID),
			"the workspace's API call is not refused like a link's")
	})

	t.Run("a list the contact was removed from isn't shown as subscribed", func(t *testing.T) {
		other, err := factory.CreateList(workspace.ID, testutil.WithListPublic(true))
		require.NoError(t, err)
		onList(t, other.ID, domain.ContactListStatusActive)
		require.Contains(t, shownLists(t), other.ID)

		resp, err := client.Post("/api/contactLists.removeContact", map[string]interface{}{
			"workspace_id": workspace.ID,
			"email":        email,
			"list_id":      other.ID,
		})
		require.NoError(t, err)
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode, "remove contact: %s", body)

		// The removed row keeps its "active" status
		assert.NotContains(t, shownLists(t), other.ID)
	})
}
