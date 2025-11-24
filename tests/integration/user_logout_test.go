package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserLogout(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	client := suite.APIClient

	t.Run("successful logout deletes all sessions", func(t *testing.T) {
		email := testUserEmail

		// Create multiple sessions by signing in 3 times
		for i := 0; i < 3; i++ {
			signinReq := domain.SignInInput{Email: email}
			resp, err := client.Post("/api/user.signin", signinReq)
			require.NoError(t, err)
			_ = resp.Body.Close()
		}

		// Verify multiple sessions exist
		app := suite.ServerManager.GetApp()
		userRepo := app.GetUserRepository()
		user, err := userRepo.GetUserByEmail(context.Background(), email)
		require.NoError(t, err)

		sessionsBefore, err := userRepo.GetSessionsByUserID(context.Background(), user.ID)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(sessionsBefore), 3, "Should have at least 3 sessions before logout")

		// Get a valid token for authentication
		token := performCompleteSignInFlow(t, client, email)

		// Logout
		req, err := http.NewRequest("POST", suite.ServerManager.GetURL()+"/api/user.logout", nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var response map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&response)
		require.NoError(t, err)

		// Verify response message
		message, ok := response["message"].(string)
		require.True(t, ok, "Response should have message field")
		assert.Equal(t, "Logged out successfully", message)

		// Verify all sessions are deleted
		sessionsAfter, err := userRepo.GetSessionsByUserID(context.Background(), user.ID)
		require.NoError(t, err)
		assert.Equal(t, 0, len(sessionsAfter), "All sessions should be deleted after logout")
	})

	t.Run("logout with token after logout fails", func(t *testing.T) {
		email := testUserEmail

		// Get a valid token
		token := performCompleteSignInFlow(t, client, email)

		// First logout should succeed
		req1, err := http.NewRequest("POST", suite.ServerManager.GetURL()+"/api/user.logout", nil)
		require.NoError(t, err)
		req1.Header.Set("Authorization", "Bearer "+token)

		resp1, err := http.DefaultClient.Do(req1)
		require.NoError(t, err)
		_ = resp1.Body.Close()
		assert.Equal(t, http.StatusOK, resp1.StatusCode)

		// Try to use the same token again - should fail because session is deleted
		req2, err := http.NewRequest("GET", suite.ServerManager.GetURL()+"/api/user.me", nil)
		require.NoError(t, err)
		req2.Header.Set("Authorization", "Bearer "+token)

		resp2, err := http.DefaultClient.Do(req2)
		require.NoError(t, err)
		defer func() { _ = resp2.Body.Close() }()

		// Should be unauthorized because session no longer exists
		assert.Equal(t, http.StatusUnauthorized, resp2.StatusCode)
	})

	t.Run("logout without token fails", func(t *testing.T) {
		req, err := http.NewRequest("POST", suite.ServerManager.GetURL()+"/api/user.logout", nil)
		require.NoError(t, err)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("logout with invalid token fails", func(t *testing.T) {
		req, err := http.NewRequest("POST", suite.ServerManager.GetURL()+"/api/user.logout", nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer invalid-token")

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("logout deletes only user's own sessions", func(t *testing.T) {
		// Create two different users with sessions
		email1 := testUserEmail
		email2 := "other-user@example.com"

		app := suite.ServerManager.GetApp()
		userRepo := app.GetUserRepository()

		// Ensure second user exists
		user2, err := userRepo.GetUserByEmail(context.Background(), email2)
		if err != nil {
			// If user doesn't exist, this test scenario may not be applicable
			// Skip or create the user
			t.Skip("Second user not found")
		}

		// Create sessions for both users
		token1 := performCompleteSignInFlow(t, client, email1)
		performCompleteSignInFlow(t, client, email2)

		// Get user IDs
		user1, err := userRepo.GetUserByEmail(context.Background(), email1)
		require.NoError(t, err)

		// Count sessions before logout
		sessions1Before, err := userRepo.GetSessionsByUserID(context.Background(), user1.ID)
		require.NoError(t, err)
		sessions2Before, err := userRepo.GetSessionsByUserID(context.Background(), user2.ID)
		require.NoError(t, err)

		user1SessionCountBefore := len(sessions1Before)
		user2SessionCountBefore := len(sessions2Before)

		assert.Greater(t, user1SessionCountBefore, 0, "User 1 should have sessions")
		assert.Greater(t, user2SessionCountBefore, 0, "User 2 should have sessions")

		// User 1 logs out
		req, err := http.NewRequest("POST", suite.ServerManager.GetURL()+"/api/user.logout", nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+token1)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		_ = resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		// Verify user 1 has no sessions
		sessions1After, err := userRepo.GetSessionsByUserID(context.Background(), user1.ID)
		require.NoError(t, err)
		assert.Equal(t, 0, len(sessions1After), "User 1 should have no sessions after logout")

		// Verify user 2 still has sessions (unaffected)
		sessions2After, err := userRepo.GetSessionsByUserID(context.Background(), user2.ID)
		require.NoError(t, err)
		assert.Equal(t, user2SessionCountBefore, len(sessions2After), "User 2's sessions should be unaffected")
	})
}
