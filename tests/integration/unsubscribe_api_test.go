package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// postUnsubscribeJSON replays what the notification center SPA (widget + console) sends:
// a JSON body with the identifying params and no query string. It is the shape the
// handler must accept for a first-party unsubscribe.
func postUnsubscribeJSON(baseURL, path string, req domain.UnsubscribeFromListsRequest) (*http.Response, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequest(http.MethodPost, baseURL+path, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	return (&http.Client{Timeout: 5 * time.Second}).Do(httpReq)
}

// TestUnsubscribe_Integration is the full-stack regression guard for the notification
// center unsubscribe. Unlike the handler unit tests (which mock ListService), this drives
// the real HMAC-verifying service against a real database and asserts the contact's
// contact_lists row actually flips to "unsubscribed". The mail-provider RFC 8058
// one-click round trip (url-encoded and multipart, plus the GET redirect) is covered by
// TestListUnsubscribeHeaders; this
// covers the JSON body that the SPA actually posts - the path that had no full-stack
// coverage and silently broke when the endpoint was made RFC 8058-strict.
func TestUnsubscribe_Integration(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	workspace, err := suite.DataFactory.CreateWorkspace()
	require.NoError(t, err)

	baseURL := suite.ServerManager.GetURL()
	contactListRepo := suite.ServerManager.GetApp().GetContactListRepository()
	secretKey := workspace.Settings.SecretKey

	// subscribeContact creates a fresh list + contact and subscribes the contact (status
	// "active"), returning the email and list ID so the test can unsubscribe and verify.
	subscribeContact := func(t *testing.T, emailPrefix string) (string, string) {
		t.Helper()
		list, err := suite.DataFactory.CreateList(workspace.ID)
		require.NoError(t, err)
		email := fmt.Sprintf("%s-%d@example.com", emailPrefix, time.Now().UnixNano())
		_, err = suite.DataFactory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
		require.NoError(t, err)
		_, err = suite.DataFactory.CreateContactList(workspace.ID,
			testutil.WithContactListEmail(email),
			testutil.WithContactListListID(list.ID),
			testutil.WithContactListStatus(domain.ContactListStatusActive))
		require.NoError(t, err)
		return email, list.ID
	}

	statusOf := func(t *testing.T, email, listID string) domain.ContactListStatus {
		t.Helper()
		got, err := contactListRepo.GetContactListByIDs(context.Background(), workspace.ID, email, listID)
		require.NoError(t, err)
		return got.Status
	}

	// The dedicated first-party endpoint: a valid JSON POST must reach the real service
	// and flip the subscription to "unsubscribed". This is the assertion a mocked-service
	// handler test cannot make, and the one that would have caught the regression.
	t.Run("SPA JSON unsubscribe flips status (POST /unsubscribe)", func(t *testing.T) {
		email, listID := subscribeContact(t, "spa-unsub")

		resp, err := postUnsubscribeJSON(baseURL, "/unsubscribe", domain.UnsubscribeFromListsRequest{
			WorkspaceID: workspace.ID,
			Email:       email,
			EmailHMAC:   domain.ComputeEmailHMAC(email, secretKey),
			ListIDs:     []string{listID},
		})
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		var result map[string]interface{}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
		assert.Equal(t, true, result["success"])

		assert.Equal(t, domain.ContactListStatusUnsubscribed, statusOf(t, email, listID),
			"POST /unsubscribe must flip contact_lists.status to 'unsubscribed'")
	})

	// Backward-compat shim: /unsubscribe-oneclick still accepts the SPA's JSON body (for
	// already-cached widget bundles), alongside the RFC 8058 form-encoded one-click.
	t.Run("JSON shim still flips status (POST /unsubscribe-oneclick)", func(t *testing.T) {
		email, listID := subscribeContact(t, "shim-unsub")

		resp, err := postUnsubscribeJSON(baseURL, "/unsubscribe-oneclick", domain.UnsubscribeFromListsRequest{
			WorkspaceID: workspace.ID,
			Email:       email,
			EmailHMAC:   domain.ComputeEmailHMAC(email, secretKey),
			ListIDs:     []string{listID},
		})
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, domain.ContactListStatusUnsubscribed, statusOf(t, email, listID))
	})

	// The email_hmac is the only authorization on this public endpoint, so a wrong HMAC
	// must not unsubscribe anyone - the subscription stays "active".
	t.Run("invalid email_hmac does not unsubscribe (POST /unsubscribe)", func(t *testing.T) {
		email, listID := subscribeContact(t, "badhmac-unsub")

		resp, err := postUnsubscribeJSON(baseURL, "/unsubscribe", domain.UnsubscribeFromListsRequest{
			WorkspaceID: workspace.ID,
			Email:       email,
			EmailHMAC:   "bad-hmac",
			ListIDs:     []string{listID},
		})
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.NotEqual(t, http.StatusOK, resp.StatusCode, "a wrong HMAC must not succeed")
		assert.Equal(t, domain.ContactListStatusActive, statusOf(t, email, listID),
			"a wrong HMAC must leave the contact subscribed")
	})
}
