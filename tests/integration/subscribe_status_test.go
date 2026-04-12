package integration

import (
	"bytes"
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

func TestSubscribeStatusProtection_Integration(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	baseURL := suite.ServerManager.GetURL()
	factory := suite.DataFactory
	client := suite.APIClient

	// Create user, workspace, and authenticate
	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	err = factory.AddUserToWorkspace(user.ID, workspace.ID, "owner")
	require.NoError(t, err)
	err = client.Login(user.Email, "password")
	require.NoError(t, err)
	client.SetWorkspaceID(workspace.ID)

	list, err := factory.CreateList(workspace.ID, testutil.WithListPublic(true))
	require.NoError(t, err)

	// Helper to subscribe via public endpoint. Pass a non-empty hmac to simulate
	// an authenticated request (e.g. DOI confirmation-link click); pass "" for
	// an anonymous subscribe-form submission.
	subscribe := func(t *testing.T, email, hmac string) *http.Response {
		t.Helper()
		reqBody := domain.SubscribeToListsRequest{
			WorkspaceID: workspace.ID,
			Contact:     domain.Contact{Email: email, EmailHMAC: hmac},
			ListIDs:     []string{list.ID},
		}
		body, _ := json.Marshal(reqBody)
		req, err := http.NewRequest("POST", baseURL+"/subscribe", bytes.NewBuffer(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		resp, err := (&http.Client{}).Do(req)
		require.NoError(t, err)
		return resp
	}

	// Helper to get subscription status via authenticated API
	getStatus := func(t *testing.T, email string) string {
		t.Helper()
		resp, err := client.GetContactListByIDs(workspace.ID, email, list.ID)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		cl, ok := result["contact_list"].(map[string]interface{})
		if !ok {
			return ""
		}
		status, _ := cl["status"].(string)
		return status
	}

	t.Run("active contact subscribe returns 200 and status unchanged", func(t *testing.T) {
		email := fmt.Sprintf("active-%d@example.com", time.Now().UnixNano())
		_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
		require.NoError(t, err)
		_, err = factory.CreateContactList(workspace.ID,
			testutil.WithContactListEmail(email),
			testutil.WithContactListListID(list.ID),
			testutil.WithContactListStatus(domain.ContactListStatusActive))
		require.NoError(t, err)

		resp := subscribe(t, email, "")
		defer func() { _ = resp.Body.Close() }()
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		assert.Equal(t, "active", getStatus(t, email))
	})

	t.Run("pending contact subscribe returns 200 and status unchanged", func(t *testing.T) {
		email := fmt.Sprintf("pending-%d@example.com", time.Now().UnixNano())
		_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
		require.NoError(t, err)
		_, err = factory.CreateContactList(workspace.ID,
			testutil.WithContactListEmail(email),
			testutil.WithContactListListID(list.ID),
			testutil.WithContactListStatus(domain.ContactListStatusActive))
		require.NoError(t, err)
		err = factory.UpdateContactListStatus(workspace.ID, email, list.ID, domain.ContactListStatusPending)
		require.NoError(t, err)

		resp := subscribe(t, email, "")
		defer func() { _ = resp.Body.Close() }()
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		assert.Equal(t, "pending", getStatus(t, email))
	})

	t.Run("bounced contact subscribe returns 200 and status unchanged", func(t *testing.T) {
		email := fmt.Sprintf("bounced-%d@example.com", time.Now().UnixNano())
		_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
		require.NoError(t, err)
		_, err = factory.CreateContactList(workspace.ID,
			testutil.WithContactListEmail(email),
			testutil.WithContactListListID(list.ID),
			testutil.WithContactListStatus(domain.ContactListStatusActive))
		require.NoError(t, err)
		err = factory.UpdateContactListStatus(workspace.ID, email, list.ID, domain.ContactListStatusBounced)
		require.NoError(t, err)

		resp := subscribe(t, email, "")
		defer func() { _ = resp.Body.Close() }()
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		assert.Equal(t, "bounced", getStatus(t, email))
	})

	t.Run("complained contact subscribe returns 200 and status unchanged", func(t *testing.T) {
		email := fmt.Sprintf("complained-%d@example.com", time.Now().UnixNano())
		_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
		require.NoError(t, err)
		_, err = factory.CreateContactList(workspace.ID,
			testutil.WithContactListEmail(email),
			testutil.WithContactListListID(list.ID),
			testutil.WithContactListStatus(domain.ContactListStatusActive))
		require.NoError(t, err)
		err = factory.UpdateContactListStatus(workspace.ID, email, list.ID, domain.ContactListStatusComplained)
		require.NoError(t, err)

		resp := subscribe(t, email, "")
		defer func() { _ = resp.Body.Close() }()
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		assert.Equal(t, "complained", getStatus(t, email))
	})

	t.Run("unsubscribed contact re-subscribes to pending", func(t *testing.T) {
		email := fmt.Sprintf("unsub-%d@example.com", time.Now().UnixNano())
		_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
		require.NoError(t, err)
		_, err = factory.CreateContactList(workspace.ID,
			testutil.WithContactListEmail(email),
			testutil.WithContactListListID(list.ID),
			testutil.WithContactListStatus(domain.ContactListStatusActive))
		require.NoError(t, err)
		err = factory.UpdateContactListStatus(workspace.ID, email, list.ID, domain.ContactListStatusUnsubscribed)
		require.NoError(t, err)

		resp := subscribe(t, email, "")
		defer func() { _ = resp.Body.Close() }()
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		assert.Equal(t, "pending", getStatus(t, email))
	})

	t.Run("new contact subscribe creates subscription", func(t *testing.T) {
		email := fmt.Sprintf("newcontact-%d@example.com", time.Now().UnixNano())
		_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
		require.NoError(t, err)

		resp := subscribe(t, email, "")
		defer func() { _ = resp.Body.Close() }()
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		status := getStatus(t, email)
		// Single opt-in list → active; double opt-in → pending
		assert.Contains(t, []string{"active", "pending"}, status)
	})
}

func TestBulkImportStatusProtection_Integration(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	factory := suite.DataFactory
	client := suite.APIClient

	// Create user, workspace, and authenticate
	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	err = factory.AddUserToWorkspace(user.ID, workspace.ID, "owner")
	require.NoError(t, err)
	err = client.Login(user.Email, "password")
	require.NoError(t, err)
	client.SetWorkspaceID(workspace.ID)

	list, err := factory.CreateList(workspace.ID, testutil.WithListPublic(true))
	require.NoError(t, err)

	// Helper to get subscription status
	getStatus := func(t *testing.T, email string) string {
		t.Helper()
		resp, err := client.GetContactListByIDs(workspace.ID, email, list.ID)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		cl, ok := result["contact_list"].(map[string]interface{})
		if !ok {
			return ""
		}
		status, _ := cl["status"].(string)
		return status
	}

	t.Run("bulk import skips bounced contacts", func(t *testing.T) {
		email := fmt.Sprintf("bulk-bounced-%d@example.com", time.Now().UnixNano())
		_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
		require.NoError(t, err)
		_, err = factory.CreateContactList(workspace.ID,
			testutil.WithContactListEmail(email),
			testutil.WithContactListListID(list.ID),
			testutil.WithContactListStatus(domain.ContactListStatusActive))
		require.NoError(t, err)
		err = factory.UpdateContactListStatus(workspace.ID, email, list.ID, domain.ContactListStatusBounced)
		require.NoError(t, err)

		contacts := []map[string]interface{}{
			{"email": email},
		}
		resp, err := client.BatchImportContacts(contacts, []string{list.ID})
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		assert.Equal(t, "bounced", getStatus(t, email))
	})

	t.Run("bulk import skips unsubscribed contacts", func(t *testing.T) {
		email := fmt.Sprintf("bulk-unsub-%d@example.com", time.Now().UnixNano())
		_, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
		require.NoError(t, err)
		_, err = factory.CreateContactList(workspace.ID,
			testutil.WithContactListEmail(email),
			testutil.WithContactListListID(list.ID),
			testutil.WithContactListStatus(domain.ContactListStatusActive))
		require.NoError(t, err)
		err = factory.UpdateContactListStatus(workspace.ID, email, list.ID, domain.ContactListStatusUnsubscribed)
		require.NoError(t, err)

		contacts := []map[string]interface{}{
			{"email": email},
		}
		resp, err := client.BatchImportContacts(contacts, []string{list.ID})
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		assert.Equal(t, "unsubscribed", getStatus(t, email))
	})
}

// TestDoubleOptInConfirmation_Integration is a regression test for issue #313.
// It exercises the end-to-end DOI confirmation flow that broke in v29.2:
//  1. Anonymous subscribe to a public DOI list → contact_list row is Pending.
//  2. Clicking the confirmation link (authenticated subscribe with valid HMAC)
//     must transition Pending → Active.
//  3. Re-clicking the same link must be idempotent (stay Active).
func TestDoubleOptInConfirmation_Integration(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	baseURL := suite.ServerManager.GetURL()
	factory := suite.DataFactory
	client := suite.APIClient

	// Create user, workspace, and authenticate (needed so client.GetContactListByIDs works).
	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	err = factory.AddUserToWorkspace(user.ID, workspace.ID, "owner")
	require.NoError(t, err)
	err = client.Login(user.Email, "password")
	require.NoError(t, err)
	client.SetWorkspaceID(workspace.ID)

	// Public DOI list — the combination that triggered the bug.
	doiList, err := factory.CreateList(workspace.ID,
		testutil.WithListPublic(true),
		testutil.WithListDoubleOptin(true))
	require.NoError(t, err)

	secretKey := workspace.Settings.SecretKey

	// Helper to subscribe via the public endpoint. Pass "" for hmac to simulate
	// an anonymous subscribe-form submission; pass a valid hmac to simulate a
	// DOI confirmation-link click.
	subscribe := func(t *testing.T, email, hmac string) *http.Response {
		t.Helper()
		reqBody := domain.SubscribeToListsRequest{
			WorkspaceID: workspace.ID,
			Contact:     domain.Contact{Email: email, EmailHMAC: hmac},
			ListIDs:     []string{doiList.ID},
		}
		body, _ := json.Marshal(reqBody)
		req, err := http.NewRequest("POST", baseURL+"/subscribe", bytes.NewBuffer(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		resp, err := (&http.Client{}).Do(req)
		require.NoError(t, err)
		return resp
	}

	// Helper to read the authoritative contact_list status via the authenticated API.
	getStatus := func(t *testing.T, email string) string {
		t.Helper()
		resp, err := client.GetContactListByIDs(workspace.ID, email, doiList.ID)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		cl, ok := result["contact_list"].(map[string]interface{})
		if !ok {
			return ""
		}
		status, _ := cl["status"].(string)
		return status
	}

	email := fmt.Sprintf("doi-confirm-%d@example.com", time.Now().UnixNano())

	// Step 1: anonymous subscribe — list is DOI, so status must be Pending.
	resp := subscribe(t, email, "")
	_ = resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, string(domain.ContactListStatusPending), getStatus(t, email),
		"new anonymous subscribe to a DOI list must create a Pending row")

	// Step 2: confirmation-link click — authenticated subscribe must activate.
	hmac := domain.ComputeEmailHMAC(email, secretKey)
	resp = subscribe(t, email, hmac)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, string(domain.ContactListStatusActive), getStatus(t, email),
		"issue #313: authenticated subscribe on a Pending row must transition to Active")

	// Step 3: re-clicking the confirmation link is an idempotent no-op.
	resp = subscribe(t, email, hmac)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, string(domain.ContactListStatusActive), getStatus(t, email),
		"a repeat confirmation click must remain Active")
}
