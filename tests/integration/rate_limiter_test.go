package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSignInRateLimiter_Integration(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	client := suite.APIClient
	email := testUserEmail // Pre-seeded test user

	t.Run("allows first 5 signin attempts", func(t *testing.T) {
		// Make 5 signin requests - all should succeed (not be rate limited)
		for i := 0; i < 5; i++ {
			signinReq := domain.SignInInput{Email: email}

			resp, err := client.Post("/api/user.signin", signinReq)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			// Should succeed (200) - not rate limited
			assert.Equal(t, http.StatusOK, resp.StatusCode, "Attempt %d should succeed", i+1)

			var response map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&response)
			require.NoError(t, err)

			// Should NOT contain rate limit error
			if errorMsg, ok := response["error"].(string); ok {
				assert.NotContains(t, errorMsg, "too many sign-in attempts", "Attempt %d should not be rate limited", i+1)
			}
		}
	})

	t.Run("blocks 6th signin attempt with rate limit error", func(t *testing.T) {
		// 6th attempt should be rate limited
		signinReq := domain.SignInInput{Email: email}

		resp, err := client.Post("/api/user.signin", signinReq)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		// Should return error status
		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

		var response map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&response)
		require.NoError(t, err)

		// Should contain rate limit error message
		errorMsg, ok := response["error"].(string)
		require.True(t, ok, "Response should have error field")
		assert.Contains(t, errorMsg, "too many sign-in attempts", "Error message should indicate rate limit")
	})

	t.Run("different user has independent rate limit", func(t *testing.T) {
		// Create a different test user
		differentEmail := "different-user@example.com"

		app := suite.ServerManager.GetApp()
		userRepo := app.GetUserRepository()

		// Create user
		user := &domain.User{
			ID:        uuid.New().String(),
			Email:     differentEmail,
			Name:      "Rate Limit Test User",
			Type:      domain.UserTypeUser,
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		}
		err := userRepo.CreateUser(context.Background(), user)
		require.NoError(t, err)

		// Different user should be able to signin even though first user is rate limited
		signinReq := domain.SignInInput{Email: differentEmail}

		resp, err := client.Post("/api/user.signin", signinReq)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		// Should succeed - different user has separate rate limit
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var response map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&response)
		require.NoError(t, err)

		// Should NOT be rate limited
		if errorMsg, ok := response["error"].(string); ok {
			assert.NotContains(t, errorMsg, "too many sign-in attempts")
		}
	})
}

func TestVerifyCodeRateLimiter_Integration(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	client := suite.APIClient

	t.Run("allows first 5 verify attempts then blocks 6th", func(t *testing.T) {
		// Use a fresh email for this test to avoid interference
		email := "verify-rate-limit-test@example.com"

		app := suite.ServerManager.GetApp()
		userRepo := app.GetUserRepository()

		// Create test user
		user := &domain.User{
			ID:        uuid.New().String(),
			Email:     email,
			Name:      "Verify Rate Limit Test User",
			Type:      domain.UserTypeUser,
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		}
		err := userRepo.CreateUser(context.Background(), user)
		require.NoError(t, err)

		// Create a session with a magic code
		magicCode := "123456"
		magicCodeExpires := time.Now().UTC().Add(15 * time.Minute)
		session := &domain.Session{
			ID:               uuid.New().String(),
			UserID:           user.ID,
			ExpiresAt:        time.Now().UTC().Add(24 * time.Hour),
			CreatedAt:        time.Now().UTC(),
			MagicCode:        &magicCode,
			MagicCodeExpires: &magicCodeExpires,
		}
		err = userRepo.CreateSession(context.Background(), session)
		require.NoError(t, err)

		// Try 5 wrong codes - should all fail but NOT be rate limited
		for i := 0; i < 5; i++ {
			verifyReq := domain.VerifyCodeInput{
				Email: email,
				Code:  "999999", // Wrong code
			}

			resp, err := client.Post("/api/user.verify", verifyReq)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			// Should fail (wrong code) but NOT with rate limit error
			var response map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&response)
			require.NoError(t, err)

			if errorMsg, ok := response["error"].(string); ok {
				// Should be "invalid magic code", not rate limit error
				assert.NotContains(t, errorMsg, "too many verification attempts", "Attempt %d should not be rate limited", i+1)
				// Could be "invalid magic code" or "user not found" depending on timing
			}
		}

		// 6th attempt should be rate limited
		verifyReq := domain.VerifyCodeInput{
			Email: email,
			Code:  "999999",
		}

		resp, err := client.Post("/api/user.verify", verifyReq)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		// Should return unauthorized status (all verify errors return 401)
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

		var response map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&response)
		require.NoError(t, err)

		// Should contain rate limit error message
		errorMsg, ok := response["error"].(string)
		require.True(t, ok, "Response should have error field")
		assert.Contains(t, errorMsg, "too many verification attempts", "Error message should indicate rate limit")
	})

	t.Run("successful verification resets rate limit", func(t *testing.T) {
		// Use a different email to avoid rate limit from previous test
		email := "verify-reset-test@example.com"

		app := suite.ServerManager.GetApp()
		userRepo := app.GetUserRepository()

		// Create test user
		user := &domain.User{
			ID:        uuid.New().String(),
			Email:     email,
			Name:      "Reset Test User",
			Type:      domain.UserTypeUser,
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		}
		err := userRepo.CreateUser(context.Background(), user)
		require.NoError(t, err)

		// Sign in to get a magic code
		signinReq := domain.SignInInput{Email: email}
		signinResp, err := client.Post("/api/user.signin", signinReq)
		require.NoError(t, err)
		defer func() { _ = signinResp.Body.Close() }()

		var signinResponse map[string]interface{}
		err = json.NewDecoder(signinResp.Body).Decode(&signinResponse)
		require.NoError(t, err)

		code, ok := signinResponse["code"].(string)
		require.True(t, ok, "Magic code should be returned in test environment")

		// Try 4 wrong codes first
		for i := 0; i < 4; i++ {
			verifyReq := domain.VerifyCodeInput{
				Email: email,
				Code:  "000000", // Wrong code
			}

			resp, err := client.Post("/api/user.verify", verifyReq)
			require.NoError(t, err)
			_ = resp.Body.Close()
		}

		// Now verify with correct code - should succeed and reset limiter
		verifyReq := domain.VerifyCodeInput{
			Email: email,
			Code:  code,
		}

		verifyResp, err := client.Post("/api/user.verify", verifyReq)
		require.NoError(t, err)
		defer func() { _ = verifyResp.Body.Close() }()

		assert.Equal(t, http.StatusOK, verifyResp.StatusCode, "Correct code should succeed")

		// After successful verification, rate limiter should be reset
		// Try to signin again (this would be blocked if limiter wasn't reset)
		signinReq2 := domain.SignInInput{Email: email}
		signinResp2, err := client.Post("/api/user.signin", signinReq2)
		require.NoError(t, err)
		defer func() { _ = signinResp2.Body.Close() }()

		// Should succeed - rate limiter was reset
		var signinResponse2 map[string]interface{}
		err = json.NewDecoder(signinResp2.Body).Decode(&signinResponse2)
		require.NoError(t, err)

		// Get new code
		code2, ok := signinResponse2["code"].(string)
		require.True(t, ok)

		// Should be able to verify multiple times again (not rate limited)
		verifyReq2 := domain.VerifyCodeInput{
			Email: email,
			Code:  code2,
		}

		verifyResp2, err := client.Post("/api/user.verify", verifyReq2)
		require.NoError(t, err)
		defer func() { _ = verifyResp2.Body.Close() }()

		assert.Equal(t, http.StatusOK, verifyResp2.StatusCode, "Should not be rate limited after reset")
	})
}

func TestRateLimiter_CrossEndpoint_Independence(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	client := suite.APIClient

	t.Run("signin rate limit does not affect verify rate limit", func(t *testing.T) {
		email := "cross-endpoint-test@example.com"

		app := suite.ServerManager.GetApp()
		userRepo := app.GetUserRepository()

		// Create test user
		user := &domain.User{
			ID:        uuid.New().String(),
			Email:     email,
			Name:      "Cross Endpoint Test User",
			Type:      domain.UserTypeUser,
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		}
		err := userRepo.CreateUser(context.Background(), user)
		require.NoError(t, err)

		// Exhaust signin rate limit (5 attempts)
		for i := 0; i < 5; i++ {
			signinReq := domain.SignInInput{Email: email}
			resp, err := client.Post("/api/user.signin", signinReq)
			require.NoError(t, err)
			_ = resp.Body.Close()
		}

		// Verify signin is rate limited
		signinReq := domain.SignInInput{Email: email}
		signinResp, err := client.Post("/api/user.signin", signinReq)
		require.NoError(t, err)
		defer func() { _ = signinResp.Body.Close() }()

		assert.Equal(t, http.StatusInternalServerError, signinResp.StatusCode)

		// But verify code should still work (independent rate limiter)
		// First get a session with a code (use repository)
		magicCode2 := "123456"
		magicCodeExpires2 := time.Now().UTC().Add(15 * time.Minute)
		session := &domain.Session{
			ID:               uuid.New().String(),
			UserID:           user.ID,
			ExpiresAt:        time.Now().UTC().Add(24 * time.Hour),
			CreatedAt:        time.Now().UTC(),
			MagicCode:        &magicCode2,
			MagicCodeExpires: &magicCodeExpires2,
		}
		err = userRepo.CreateSession(context.Background(), session)
		require.NoError(t, err)

		// Verify should not be rate limited (independent limiter)
		verifyReq := domain.VerifyCodeInput{
			Email: email,
			Code:  "999999", // Wrong code
		}

		verifyResp, err := client.Post("/api/user.verify", verifyReq)
		require.NoError(t, err)
		defer func() { _ = verifyResp.Body.Close() }()

		// Should fail for wrong code, but NOT with rate limit error
		var response map[string]interface{}
		err = json.NewDecoder(verifyResp.Body).Decode(&response)
		require.NoError(t, err)

		if errorMsg, ok := response["error"].(string); ok {
			// Should be "invalid magic code", not rate limit error
			assert.NotContains(t, errorMsg, "too many verification attempts", "Verify should not be rate limited yet")
		}
	})
}

func TestRateLimiter_ErrorMessages(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer func() { suite.Cleanup() }()

	client := suite.APIClient

	t.Run("signin rate limit returns correct error message", func(t *testing.T) {
		email := "error-message-test@example.com"

		app := suite.ServerManager.GetApp()
		userRepo := app.GetUserRepository()

		// Create test user
		user := &domain.User{
			ID:        uuid.New().String(),
			Email:     email,
			Name:      "Error Message Test User",
			Type:      domain.UserTypeUser,
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		}
		err := userRepo.CreateUser(context.Background(), user)
		require.NoError(t, err)

		// Exhaust rate limit
		for i := 0; i < 5; i++ {
			signinReq := domain.SignInInput{Email: email}
			resp, err := client.Post("/api/user.signin", signinReq)
			require.NoError(t, err)
			_ = resp.Body.Close()
		}

		// Next attempt should return proper error
		signinReq := domain.SignInInput{Email: email}
		resp, err := client.Post("/api/user.signin", signinReq)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		var response map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&response)
		require.NoError(t, err)

		errorMsg, ok := response["error"].(string)
		require.True(t, ok, "Response should have error field")

		// Verify error message is user-friendly and informative
		assert.Contains(t, errorMsg, "too many sign-in attempts")
		assert.Contains(t, errorMsg, "try again in a few minutes")
	})

	t.Run("verify rate limit returns correct error message", func(t *testing.T) {
		email := "verify-error-message-test@example.com"

		app := suite.ServerManager.GetApp()
		userRepo := app.GetUserRepository()

		// Create test user and session
		user := &domain.User{
			ID:        uuid.New().String(),
			Email:     email,
			Name:      "Verify Error Test User",
			Type:      domain.UserTypeUser,
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		}
		err := userRepo.CreateUser(context.Background(), user)
		require.NoError(t, err)

		magicCode3 := "123456"
		magicCodeExpires3 := time.Now().UTC().Add(15 * time.Minute)
		session := &domain.Session{
			ID:               uuid.New().String(),
			UserID:           user.ID,
			ExpiresAt:        time.Now().UTC().Add(24 * time.Hour),
			CreatedAt:        time.Now().UTC(),
			MagicCode:        &magicCode3,
			MagicCodeExpires: &magicCodeExpires3,
		}
		err = userRepo.CreateSession(context.Background(), session)
		require.NoError(t, err)

		// Exhaust rate limit
		for i := 0; i < 5; i++ {
			verifyReq := domain.VerifyCodeInput{
				Email: email,
				Code:  "999999",
			}
			resp, err := client.Post("/api/user.verify", verifyReq)
			require.NoError(t, err)
			_ = resp.Body.Close()
		}

		// Next attempt should return proper error
		verifyReq := domain.VerifyCodeInput{
			Email: email,
			Code:  "999999",
		}
		resp, err := client.Post("/api/user.verify", verifyReq)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		// Should return unauthorized status (all verify errors return 401)
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

		var response map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&response)
		require.NoError(t, err)

		errorMsg, ok := response["error"].(string)
		require.True(t, ok, "Response should have error field")

		// Verify error message is user-friendly and informative
		assert.Contains(t, errorMsg, "too many verification attempts")
		assert.Contains(t, errorMsg, "try again in a few minutes")
	})
}
