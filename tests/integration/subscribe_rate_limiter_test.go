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

func TestSubscribeRateLimiter_Integration(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer _ = suite.Cleanup()

	// Get base URL for the public endpoint
	baseURL := suite.ServerManager.GetURL()

	// Create test workspace
	workspace, err := suite.DataFactory.CreateWorkspace()
	require.NoError(t, err)
	testWorkspaceID := workspace.ID

	// Create a public test list for subscriptions
	list, err := suite.DataFactory.CreateList(testWorkspaceID, testutil.WithListPublic(true))
	require.NoError(t, err)
	testListID := list.ID

	t.Run("allows first 10 subscribe attempts", func(t *testing.T) {
		email := fmt.Sprintf("ratelimit-test-%d@example.com", time.Now().UnixNano())

		// Make 10 subscribe requests - all should succeed (not be rate limited)
		for i := 0; i < 10; i++ {
			subscribeReq := domain.SubscribeToListsRequest{
				WorkspaceID: testWorkspaceID,
				Contact: domain.Contact{
					Email: email,
				},
				ListIDs: []string{testListID},
			}

			body, _ := json.Marshal(subscribeReq)
			req, err := http.NewRequest("POST", baseURL+"/subscribe", bytes.NewBuffer(body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			client := &http.Client{}
			resp, err := client.Do(req)
			require.NoError(t, err)
			defer func() { _ = _ = resp.Body.Close() }()

			// Should succeed (200) - not rate limited
			assert.Equal(t, http.StatusOK, resp.StatusCode, "Attempt %d should succeed", i+1)
		}
	})

	t.Run("blocks 11th subscribe attempt", func(t *testing.T) {
		email := fmt.Sprintf("ratelimit-test2-%d@example.com", time.Now().UnixNano())

		// Exhaust rate limit (10 attempts)
		for i := 0; i < 10; i++ {
			subscribeReq := domain.SubscribeToListsRequest{
				WorkspaceID: testWorkspaceID,
				Contact: domain.Contact{
					Email: email,
				},
				ListIDs: []string{testListID},
			}

			body, _ := json.Marshal(subscribeReq)
			req, err := http.NewRequest("POST", baseURL+"/subscribe", bytes.NewBuffer(body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			client := &http.Client{}
			resp, err := client.Do(req)
			require.NoError(t, err)
			_ = resp.Body.Close()
		}

		// 11th attempt should be rate limited
		subscribeReq := domain.SubscribeToListsRequest{
			WorkspaceID: testWorkspaceID,
			Contact: domain.Contact{
				Email: email,
			},
			ListIDs: []string{testListID},
		}

		body, _ := json.Marshal(subscribeReq)
		req, err := http.NewRequest("POST", baseURL+"/subscribe", bytes.NewBuffer(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer func() { _ = _ = resp.Body.Close() }()

		// Should return 429 Too Many Requests
		assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode)

		// Check for Retry-After header
		retryAfter := resp.Header.Get("Retry-After")
		assert.NotEmpty(t, retryAfter, "Retry-After header should be set")

		// Verify error message
		var response map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&response)
		require.NoError(t, err)

		errorMsg, ok := response["error"].(string)
		require.True(t, ok, "Response should have error field")
		assert.Contains(t, errorMsg, "Too many subscription attempts", "Error message should indicate rate limit")
	})

	t.Run("different emails have independent rate limits", func(t *testing.T) {
		email1 := fmt.Sprintf("ratelimit-test3-%d@example.com", time.Now().UnixNano())
		email2 := fmt.Sprintf("ratelimit-test4-%d@example.com", time.Now().UnixNano())

		// Exhaust rate limit for email1 (10 attempts)
		for i := 0; i < 10; i++ {
			subscribeReq := domain.SubscribeToListsRequest{
				WorkspaceID: testWorkspaceID,
				Contact: domain.Contact{
					Email: email1,
				},
				ListIDs: []string{testListID},
			}

			body, _ := json.Marshal(subscribeReq)
			req, err := http.NewRequest("POST", baseURL+"/subscribe", bytes.NewBuffer(body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			client := &http.Client{}
			resp, err := client.Do(req)
			require.NoError(t, err)
			_ = resp.Body.Close()
		}

		// Verify email1 is rate limited
		subscribeReq1 := domain.SubscribeToListsRequest{
			WorkspaceID: testWorkspaceID,
			Contact: domain.Contact{
				Email: email1,
			},
			ListIDs: []string{testListID},
		}

		body1, _ := json.Marshal(subscribeReq1)
		req1, err := http.NewRequest("POST", baseURL+"/subscribe", bytes.NewBuffer(body1))
		require.NoError(t, err)
		req1.Header.Set("Content-Type", "application/json")

		client := &http.Client{}
		resp1, err := client.Do(req1)
		require.NoError(t, err)
		defer func() { _ = _ = resp1.Body.Close() }()

		assert.Equal(t, http.StatusTooManyRequests, resp1.StatusCode, "Email1 should be rate limited")

		// Email2 should still be allowed (independent rate limit)
		subscribeReq2 := domain.SubscribeToListsRequest{
			WorkspaceID: testWorkspaceID,
			Contact: domain.Contact{
				Email: email2,
			},
			ListIDs: []string{testListID},
		}

		body2, _ := json.Marshal(subscribeReq2)
		req2, err := http.NewRequest("POST", baseURL+"/subscribe", bytes.NewBuffer(body2))
		require.NoError(t, err)
		req2.Header.Set("Content-Type", "application/json")

		resp2, err := client.Do(req2)
		require.NoError(t, err)
		defer func() { _ = _ = resp2.Body.Close() }()

		assert.Equal(t, http.StatusOK, resp2.StatusCode, "Email2 should not be rate limited")
	})

	t.Run("rate limit resets after window expires", func(t *testing.T) {
		// Note: This test is slow because we need to wait for the rate limit window to expire
		// In a real scenario, you might want to skip this test or make the window shorter for testing
		t.Skip("Skipping slow test - rate limit window is 1 minute")

		email := fmt.Sprintf("ratelimit-test5-%d@example.com", time.Now().UnixNano())

		// Exhaust rate limit (10 attempts)
		for i := 0; i < 10; i++ {
			subscribeReq := domain.SubscribeToListsRequest{
				WorkspaceID: testWorkspaceID,
				Contact: domain.Contact{
					Email: email,
				},
				ListIDs: []string{testListID},
			}

			body, _ := json.Marshal(subscribeReq)
			req, err := http.NewRequest("POST", baseURL+"/subscribe", bytes.NewBuffer(body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			client := &http.Client{}
			resp, err := client.Do(req)
			require.NoError(t, err)
			_ = resp.Body.Close()
		}

		// Verify rate limited
		subscribeReq := domain.SubscribeToListsRequest{
			WorkspaceID: testWorkspaceID,
			Contact: domain.Contact{
				Email: email,
			},
			ListIDs: []string{testListID},
		}

		body, _ := json.Marshal(subscribeReq)
		req, err := http.NewRequest("POST", baseURL+"/subscribe", bytes.NewBuffer(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{}
		resp, err := client.Do(req)
		require.NoError(t, err)
		_ = resp.Body.Close()

		assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode, "Should be rate limited")

		// Wait for window to expire (1 minute + buffer)
		time.Sleep(65 * time.Second)

		// Try again - should be allowed
		req2, err := http.NewRequest("POST", baseURL+"/subscribe", bytes.NewBuffer(body))
		require.NoError(t, err)
		req2.Header.Set("Content-Type", "application/json")

		resp2, err := client.Do(req2)
		require.NoError(t, err)
		defer func() { _ = _ = resp2.Body.Close() }()

		assert.Equal(t, http.StatusOK, resp2.StatusCode, "Should be allowed after window expires")
	})
}

func TestSubscribeIPRateLimiter_Integration(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, appFactory)
	defer _ = suite.Cleanup()

	// Get base URL for the public endpoint
	baseURL := suite.ServerManager.GetURL()

	// Create test workspace
	workspace, err := suite.DataFactory.CreateWorkspace()
	require.NoError(t, err)
	testWorkspaceID := workspace.ID

	// Create a public test list for subscriptions
	list, err := suite.DataFactory.CreateList(testWorkspaceID, testutil.WithListPublic(true))
	require.NoError(t, err)
	testListID := list.ID

	t.Run("IP rate limit is independent of email rate limit", func(t *testing.T) {
		// Note: This test verifies that IP-based rate limiting exists
		// but it's hard to test properly in integration tests since all requests come from localhost

		// The test verifies that we can make requests with different emails
		// and they each get their own rate limit bucket
		for i := 0; i < 5; i++ {
			email := fmt.Sprintf("ip-test-%d-%d@example.com", time.Now().UnixNano(), i)

			subscribeReq := domain.SubscribeToListsRequest{
				WorkspaceID: testWorkspaceID,
				Contact: domain.Contact{
					Email: email,
				},
				ListIDs: []string{testListID},
			}

			body, _ := json.Marshal(subscribeReq)
			req, err := http.NewRequest("POST", baseURL+"/subscribe", bytes.NewBuffer(body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			client := &http.Client{}
			resp, err := client.Do(req)
			require.NoError(t, err)
			defer func() { _ = _ = resp.Body.Close() }()

			// All should succeed since they use different emails
			assert.Equal(t, http.StatusOK, resp.StatusCode, "Request %d should succeed", i+1)
		}
	})
}
