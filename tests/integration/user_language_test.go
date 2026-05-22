package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUserLanguagePreference exercises the per-user language feature end-to-end
// against a real database: the /api/user.updateLanguage endpoint, persistence to
// the users.language column, and /api/user.me reflecting the stored value.
func TestUserLanguagePreference(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	client := suite.APIClient
	baseURL := suite.ServerManager.GetURL()

	// doAuth issues an HTTP request with an optional bearer token and JSON body.
	doAuth := func(method, path, token string, body any) *http.Response {
		var reqBody io.Reader
		if body != nil {
			b, err := json.Marshal(body)
			require.NoError(t, err)
			reqBody = bytes.NewReader(b)
		}
		req, err := http.NewRequest(method, baseURL+path, reqBody)
		require.NoError(t, err)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		return resp
	}

	token := performCompleteSignInFlow(t, client, testUserEmail)

	currentUserLanguage := func() string {
		resp := doAuth(http.MethodGet, "/api/user.me", token, nil)
		defer func() { _ = resp.Body.Close() }()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var body struct {
			User struct {
				Language string `json:"language"`
			} `json:"user"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		return body.User.Language
	}

	t.Run("user.me returns the language, defaulting to English", func(t *testing.T) {
		assert.Equal(t, "en", currentUserLanguage())
	})

	t.Run("updateLanguage persists to the DB and is reflected by user.me", func(t *testing.T) {
		resp := doAuth(http.MethodPost, "/api/user.updateLanguage", token, map[string]string{"language": "fr"})
		defer func() { _ = resp.Body.Close() }()
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		// Reflected by the API
		assert.Equal(t, "fr", currentUserLanguage())

		// Persisted in the users table
		user, err := suite.ServerManager.GetApp().GetUserRepository().
			GetUserByEmail(context.Background(), testUserEmail)
		require.NoError(t, err)
		assert.Equal(t, "fr", user.Language)
	})

	t.Run("updateLanguage rejects an unsupported locale with 400", func(t *testing.T) {
		resp := doAuth(http.MethodPost, "/api/user.updateLanguage", token, map[string]string{"language": "xx"})
		defer func() { _ = resp.Body.Close() }()
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

		// The rejected value must not have been persisted.
		user, err := suite.ServerManager.GetApp().GetUserRepository().
			GetUserByEmail(context.Background(), testUserEmail)
		require.NoError(t, err)
		assert.Equal(t, "fr", user.Language)
	})

	t.Run("updateLanguage rejects an unauthenticated request with 401", func(t *testing.T) {
		resp := doAuth(http.MethodPost, "/api/user.updateLanguage", "", map[string]string{"language": "de"})
		defer func() { _ = resp.Body.Close() }()
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})
}
