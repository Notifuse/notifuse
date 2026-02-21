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

func postPreferences(baseURL string, req domain.UpdateContactPreferencesRequest) (*http.Response, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequest("POST", baseURL+"/preferences", bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	return (&http.Client{}).Do(httpReq)
}

func TestUpdateContactPreferences_Integration(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	workspace, err := suite.DataFactory.CreateWorkspace()
	require.NoError(t, err)

	baseURL := suite.ServerManager.GetURL()
	appInstance := suite.ServerManager.GetApp()
	secretKey := workspace.Settings.SecretKey

	t.Run("updates both language and timezone", func(t *testing.T) {
		email := fmt.Sprintf("pref-both-%d@example.com", time.Now().UnixNano())
		_, err := suite.DataFactory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
		require.NoError(t, err)

		resp, err := postPreferences(baseURL, domain.UpdateContactPreferencesRequest{
			WorkspaceID: workspace.ID,
			Email:       email,
			EmailHMAC:   domain.ComputeEmailHMAC(email, secretKey),
			Language:    "fr",
			Timezone:    "Europe/Paris",
		})
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
		assert.Equal(t, true, result["success"])

		contact, err := appInstance.GetContactRepository().GetContactByEmail(context.Background(), workspace.ID, email)
		require.NoError(t, err)
		assert.Equal(t, "fr", contact.Language.String)
		assert.Equal(t, "Europe/Paris", contact.Timezone.String)
	})

	t.Run("updates language only preserves timezone", func(t *testing.T) {
		email := fmt.Sprintf("pref-lang-%d@example.com", time.Now().UnixNano())
		_, err := suite.DataFactory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
		require.NoError(t, err)

		resp, err := postPreferences(baseURL, domain.UpdateContactPreferencesRequest{
			WorkspaceID: workspace.ID,
			Email:       email,
			EmailHMAC:   domain.ComputeEmailHMAC(email, secretKey),
			Language:    "de",
		})
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		contact, err := appInstance.GetContactRepository().GetContactByEmail(context.Background(), workspace.ID, email)
		require.NoError(t, err)
		assert.Equal(t, "de", contact.Language.String)
		assert.Equal(t, "UTC", contact.Timezone.String)
	})

	t.Run("updates timezone only preserves language", func(t *testing.T) {
		email := fmt.Sprintf("pref-tz-%d@example.com", time.Now().UnixNano())
		_, err := suite.DataFactory.CreateContact(workspace.ID,
			testutil.WithContactEmail(email),
			testutil.WithContactLanguage("fr"),
		)
		require.NoError(t, err)

		resp, err := postPreferences(baseURL, domain.UpdateContactPreferencesRequest{
			WorkspaceID: workspace.ID,
			Email:       email,
			EmailHMAC:   domain.ComputeEmailHMAC(email, secretKey),
			Timezone:    "Asia/Tokyo",
		})
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		contact, err := appInstance.GetContactRepository().GetContactByEmail(context.Background(), workspace.ID, email)
		require.NoError(t, err)
		assert.Equal(t, "Asia/Tokyo", contact.Timezone.String)
		assert.Equal(t, "fr", contact.Language.String)
	})

	t.Run("sets fields on contact with nil language and timezone", func(t *testing.T) {
		email := fmt.Sprintf("pref-nil-%d@example.com", time.Now().UnixNano())
		_, err := suite.DataFactory.CreateContact(workspace.ID,
			testutil.WithContactEmail(email),
			testutil.WithContactLanguageNil(),
			testutil.WithContactTimezoneNil(),
		)
		require.NoError(t, err)

		resp, err := postPreferences(baseURL, domain.UpdateContactPreferencesRequest{
			WorkspaceID: workspace.ID,
			Email:       email,
			EmailHMAC:   domain.ComputeEmailHMAC(email, secretKey),
			Language:    "es",
			Timezone:    "Europe/Madrid",
		})
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		contact, err := appInstance.GetContactRepository().GetContactByEmail(context.Background(), workspace.ID, email)
		require.NoError(t, err)
		require.NotNil(t, contact.Language)
		require.NotNil(t, contact.Timezone)
		assert.Equal(t, "es", contact.Language.String)
		assert.Equal(t, "Europe/Madrid", contact.Timezone.String)
	})

	t.Run("rejects invalid HMAC", func(t *testing.T) {
		email := fmt.Sprintf("pref-hmac-%d@example.com", time.Now().UnixNano())
		_, err := suite.DataFactory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
		require.NoError(t, err)

		resp, err := postPreferences(baseURL, domain.UpdateContactPreferencesRequest{
			WorkspaceID: workspace.ID,
			Email:       email,
			EmailHMAC:   "bad-hmac",
			Language:    "fr",
		})
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("rejects invalid language format", func(t *testing.T) {
		email := fmt.Sprintf("pref-badlang-%d@example.com", time.Now().UnixNano())
		_, err := suite.DataFactory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
		require.NoError(t, err)

		resp, err := postPreferences(baseURL, domain.UpdateContactPreferencesRequest{
			WorkspaceID: workspace.ID,
			Email:       email,
			EmailHMAC:   domain.ComputeEmailHMAC(email, secretKey),
			Language:    "FRA",
		})
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("rejects when neither language nor timezone provided", func(t *testing.T) {
		email := fmt.Sprintf("pref-empty-%d@example.com", time.Now().UnixNano())
		_, err := suite.DataFactory.CreateContact(workspace.ID, testutil.WithContactEmail(email))
		require.NoError(t, err)

		resp, err := postPreferences(baseURL, domain.UpdateContactPreferencesRequest{
			WorkspaceID: workspace.ID,
			Email:       email,
			EmailHMAC:   domain.ComputeEmailHMAC(email, secretKey),
		})
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

		var result map[string]interface{}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
		errMsg, ok := result["error"].(string)
		assert.True(t, ok)
		assert.Contains(t, errMsg, "at least one")
	})

	t.Run("rejects invalid request body", func(t *testing.T) {
		req, err := http.NewRequest("POST", baseURL+"/preferences", bytes.NewBufferString("not json"))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		resp, err := (&http.Client{}).Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}
