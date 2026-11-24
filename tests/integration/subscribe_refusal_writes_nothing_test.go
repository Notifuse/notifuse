//go:build integration
// +build integration

package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSubscribeRefusalWritesNothing checks that a subscribe refused on the only list it
// names leaves the contact as it was: lists.subscribe with an API key on a contact who
// opted out of the list, without resubscribe, and the MCP tool subscribe_contact_to_list,
// which answers invalid_state. The contact's fields in the request are not applied and
// its updated_at does not move. The same request with resubscribe does write them, so the
// check can see a write. A deleted contact who comes back through lists.subscribe on a list
// they had opted out of is refused before the insert: nothing is inserted, and the stored
// opt-out stays for the next insert to restore. Accepted on another list, they are
// inserted, and the opt-out comes back on the first.
func TestSubscribeRefusalWritesNothing(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		cfg.MCP.Enabled = true
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()
	// /mcp answers on the API endpoint's host, which the harness leaves empty.
	suite.Config.APIEndpoint = suite.ServerManager.GetURL()

	ctx := context.Background()
	factory := suite.DataFactory
	serverURL := suite.ServerManager.GetURL()

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	require.NoError(t, factory.AddUserToWorkspace(user.ID, workspace.ID, "owner"))
	db, err := factory.GetWorkspaceDB(workspace.ID)
	require.NoError(t, err)

	authService := suite.ServerManager.GetApp().GetAuthService().(*service.AuthService)
	key, err := factory.CreateAPIKey(workspace.ID)
	require.NoError(t, err)
	apiKey := testutil.NewAPIClient(serverURL)
	apiKey.SetToken(authService.GenerateAPIAuthToken(key))
	apiKey.SetWorkspaceID(workspace.ID)
	mcpKey, err := factory.CreateAPIKey(workspace.ID)
	require.NoError(t, err)
	mcp := &mcpClient{t: t, url: serverURL, token: authService.GenerateAPIAuthToken(mcpKey)}

	list, err := factory.CreateList(workspace.ID, testutil.WithListPublic(true), testutil.WithListDoubleOptin(false))
	require.NoError(t, err)

	type contactRow struct {
		firstName sql.NullString
		updatedAt time.Time
	}
	readContact := func(t *testing.T, email string) contactRow {
		t.Helper()
		var row contactRow
		require.NoError(t, db.QueryRowContext(ctx,
			`SELECT first_name, updated_at FROM contacts WHERE email = $1`, email).Scan(&row.firstName, &row.updatedAt))
		return row
	}
	listStatus := func(t *testing.T, email string) string {
		t.Helper()
		var status string
		require.NoError(t, db.QueryRowContext(ctx,
			`SELECT status FROM contact_lists WHERE email = $1 AND list_id = $2`, email, list.ID).Scan(&status))
		return status
	}
	// optedOut creates a contact named Before with the given status on the list.
	optedOut := func(t *testing.T, status domain.ContactListStatus) string {
		t.Helper()
		email := fmt.Sprintf("refusal-%s@example.com", strings.ToLower(uuid.New().String()[:12]))
		_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(email), testutil.WithContactName("Before", "Contact"))
		require.NoError(t, err)
		_, err = factory.CreateContactList(workspace.ID, testutil.WithContactListEmail(email),
			testutil.WithContactListListID(list.ID), testutil.WithContactListStatus(status))
		require.NoError(t, err)
		return email
	}
	// subscribe posts lists.subscribe as an API client writes it, and returns the status
	// code and the one membership it reports.
	subscribe := func(t *testing.T, email string, resubscribe bool) (int, string) {
		t.Helper()
		resp, err := apiKey.PostRaw("/api/lists.subscribe", fmt.Sprintf(
			`{"workspace_id":%q,"contact":{"email":%q,"first_name":"After"},"list_ids":[%q],"resubscribe":%t}`,
			workspace.ID, email, list.ID, resubscribe))
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		raw, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		var body struct {
			ContactLists []struct {
				Status string `json:"status"`
			} `json:"contact_lists"`
		}
		require.NoError(t, json.Unmarshal(raw, &body), string(raw))
		require.Len(t, body.ContactLists, 1, string(raw))
		return resp.StatusCode, body.ContactLists[0].Status
	}

	for _, status := range []domain.ContactListStatus{domain.ContactListStatusUnsubscribed, domain.ContactListStatusBounced, domain.ContactListStatusComplained} {
		t.Run("lists.subscribe on a "+string(status)+" contact leaves the contact as it was", func(t *testing.T) {
			email := optedOut(t, status)
			before := readContact(t, email)

			code, reported := subscribe(t, email, false)
			assert.Equal(t, http.StatusOK, code)
			assert.Equal(t, string(status), reported)

			after := readContact(t, email)
			assert.Equal(t, "Before", after.firstName.String, "the refused request's fields are not applied")
			assert.True(t, before.updatedAt.Equal(after.updatedAt), "updated_at moved from %s to %s", before.updatedAt, after.updatedAt)
			assert.Equal(t, string(status), listStatus(t, email))
		})
	}

	t.Run("lists.subscribe with resubscribe writes the contact", func(t *testing.T) {
		email := optedOut(t, domain.ContactListStatusUnsubscribed)
		before := readContact(t, email)

		code, reported := subscribe(t, email, true)
		assert.Equal(t, http.StatusOK, code)
		assert.Equal(t, "active", reported)

		after := readContact(t, email)
		assert.Equal(t, "After", after.firstName.String)
		assert.True(t, after.updatedAt.After(before.updatedAt))
	})

	t.Run("subscribe_contact_to_list refused leaves the contact as it was", func(t *testing.T) {
		email := optedOut(t, domain.ContactListStatusUnsubscribed)
		before := readContact(t, email)

		result := mcp.call("subscribe_contact_to_list", fmt.Sprintf(`{"workspace_id":%q,"email":%q,"list_id":%q}`,
			workspace.ID, email, list.ID))
		assert.Equal(t, "invalid_state", result.errorCode(t))

		after := readContact(t, email)
		assert.True(t, before.updatedAt.Equal(after.updatedAt), "updated_at moved from %s to %s", before.updatedAt, after.updatedAt)
		assert.Equal(t, "unsubscribed", listStatus(t, email))
	})

	// deleted creates a contact unsubscribed from the list and deletes it, which stores the
	// opt-out as a hash
	deleted := func(t *testing.T) string {
		t.Helper()
		email := optedOut(t, domain.ContactListStatusUnsubscribed)
		resp, err := apiKey.PostRaw("/api/contacts.delete", fmt.Sprintf(`{"workspace_id":%q,"email":%q}`, workspace.ID, email))
		require.NoError(t, err)
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		return email
	}
	count := func(t *testing.T, query string, args ...interface{}) int {
		t.Helper()
		var n int
		require.NoError(t, db.QueryRowContext(ctx, query, args...).Scan(&n))
		return n
	}
	contactCount := func(t *testing.T, email string) int {
		return count(t, `SELECT COUNT(*) FROM contacts WHERE email = $1`, email)
	}
	storedOptOuts := func(t *testing.T, email string) int {
		return count(t, `SELECT COUNT(*) FROM contact_opt_out_hashes WHERE email_hash = contact_opt_out_hash($1)`, email)
	}

	t.Run("a deleted contact who returns on a list they opted out of is not inserted", func(t *testing.T) {
		email := deleted(t)
		require.Equal(t, 1, storedOptOuts(t, email))

		code, reported := subscribe(t, email, false)
		assert.Equal(t, http.StatusOK, code)
		assert.Equal(t, "unsubscribed", reported)

		assert.Equal(t, 0, contactCount(t, email), "the refused request inserts no contact")
		assert.Equal(t, 0, count(t, `SELECT COUNT(*) FROM contact_lists WHERE email = $1`, email))
		assert.Equal(t, 1, storedOptOuts(t, email), "the opt-out stays stored for the next insert")
	})

	t.Run("a deleted contact who returns through a signed link to a private list is not inserted", func(t *testing.T) {
		private, err := factory.CreateList(workspace.ID, testutil.WithListPublic(false), testutil.WithListDoubleOptin(false))
		require.NoError(t, err)
		email := fmt.Sprintf("refusal-%s@example.com", strings.ToLower(uuid.New().String()[:12]))
		_, err = factory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
		require.NoError(t, err)
		_, err = factory.CreateContactList(workspace.ID, testutil.WithContactListEmail(email),
			testutil.WithContactListListID(private.ID), testutil.WithContactListStatus(domain.ContactListStatusUnsubscribed))
		require.NoError(t, err)
		resp, err := apiKey.PostRaw("/api/contacts.delete", fmt.Sprintf(`{"workspace_id":%q,"email":%q}`, workspace.ID, email))
		require.NoError(t, err)
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		signed := testutil.NewAPIClient(serverURL)
		resp, err = signed.PostRaw("/subscribe", fmt.Sprintf(
			`{"workspace_id":%q,"contact":{"email":%q,"email_hmac":%q,"first_name":"After"},"list_ids":[%q]}`,
			workspace.ID, email, domain.ComputeEmailHMAC(email, workspace.Settings.SecretKey), private.ID))
		require.NoError(t, err)
		raw, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		assert.NotEqual(t, http.StatusOK, resp.StatusCode, string(raw))
		assert.Contains(t, string(raw), "list is not public")

		assert.Equal(t, 0, contactCount(t, email), "the refused request inserts no contact")
		assert.Equal(t, 1, storedOptOuts(t, email))
	})

	t.Run("a deleted contact who returns on another list too is inserted, and keeps the opt-out", func(t *testing.T) {
		email := deleted(t)
		other, err := factory.CreateList(workspace.ID, testutil.WithListPublic(true), testutil.WithListDoubleOptin(false))
		require.NoError(t, err)

		resp, err := apiKey.PostRaw("/api/lists.subscribe", fmt.Sprintf(
			`{"workspace_id":%q,"contact":{"email":%q,"first_name":"After"},"list_ids":[%q,%q]}`,
			workspace.ID, email, list.ID, other.ID))
		require.NoError(t, err)
		raw, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(raw))
		var body struct {
			ContactLists []struct {
				Status string `json:"status"`
			} `json:"contact_lists"`
		}
		require.NoError(t, json.Unmarshal(raw, &body), string(raw))
		require.Len(t, body.ContactLists, 2, string(raw))
		assert.Equal(t, "unsubscribed", body.ContactLists[0].Status, "the restored opt-out is reported")
		assert.Equal(t, "active", body.ContactLists[1].Status)

		assert.Equal(t, "After", readContact(t, email).firstName.String)
		assert.Equal(t, "unsubscribed", listStatus(t, email))
		assert.Equal(t, 0, storedOptOuts(t, email), "the insert restored the opt-out")
	})
}
