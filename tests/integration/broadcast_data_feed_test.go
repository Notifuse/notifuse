package integration

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// FeedRequest represents a logged request to the mock feed server
type FeedRequest struct {
	Method  string
	Path    string
	Headers http.Header
	Body    map[string]interface{}
}

// MockFeedServer simulates an external feed endpoint for testing
type MockFeedServer struct {
	server             *httptest.Server
	mutex              sync.Mutex
	requestLog         []FeedRequest
	responseData       map[string]interface{}
	recipientResponses map[string]map[string]interface{} // email -> response data for per-recipient feeds
	responseStatus     int
	responseDelay      time.Duration
	failureCount       int // Fail N times then succeed
	requestCount       int // Track total requests
}

// NewMockFeedServer creates a new mock feed server
func NewMockFeedServer() *MockFeedServer {
	m := &MockFeedServer{
		requestLog:     make([]FeedRequest, 0),
		responseData:   map[string]interface{}{},
		responseStatus: http.StatusOK,
	}

	m.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.mutex.Lock()
		defer m.mutex.Unlock()

		// Log the request
		body, _ := io.ReadAll(r.Body)
		var bodyMap map[string]interface{}
		_ = json.Unmarshal(body, &bodyMap)

		m.requestLog = append(m.requestLog, FeedRequest{
			Method:  r.Method,
			Path:    r.URL.Path,
			Headers: r.Header.Clone(),
			Body:    bodyMap,
		})
		m.requestCount++

		// Simulate delay if set (copy to local var to avoid race after unlock)
		delay := m.responseDelay
		if delay > 0 {
			m.mutex.Unlock()
			time.Sleep(delay)
			m.mutex.Lock()
		}

		// Handle retry behavior (fail N times, then succeed)
		if m.failureCount > 0 && m.requestCount <= m.failureCount {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "simulated failure"})
			return
		}

		// Check for per-recipient responses based on contact.email in request body
		if m.recipientResponses != nil {
			if contact, ok := bodyMap["contact"].(map[string]interface{}); ok {
				if email, ok := contact["email"].(string); ok {
					if recipientData, exists := m.recipientResponses[email]; exists {
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(http.StatusOK)
						json.NewEncoder(w).Encode(recipientData)
						return
					}
				}
			}
		}

		// Return configured response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(m.responseStatus)
		json.NewEncoder(w).Encode(m.responseData)
	}))

	return m
}

// NewMockFeedServerTLS creates a mock feed server with TLS (for RecipientFeed HTTPS requirement)
func NewMockFeedServerTLS() *MockFeedServer {
	m := &MockFeedServer{
		requestLog:     make([]FeedRequest, 0),
		responseData:   map[string]interface{}{},
		responseStatus: http.StatusOK,
	}

	m.server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.mutex.Lock()
		defer m.mutex.Unlock()

		// Log the request
		body, _ := io.ReadAll(r.Body)
		var bodyMap map[string]interface{}
		_ = json.Unmarshal(body, &bodyMap)

		m.requestLog = append(m.requestLog, FeedRequest{
			Method:  r.Method,
			Path:    r.URL.Path,
			Headers: r.Header.Clone(),
			Body:    bodyMap,
		})
		m.requestCount++

		// Simulate delay if set (copy to local var to avoid race after unlock)
		delay := m.responseDelay
		if delay > 0 {
			m.mutex.Unlock()
			time.Sleep(delay)
			m.mutex.Lock()
		}

		// Handle retry behavior (fail N times, then succeed)
		if m.failureCount > 0 && m.requestCount <= m.failureCount {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "simulated failure"})
			return
		}

		// Check for per-recipient responses based on contact.email in request body
		if m.recipientResponses != nil {
			if contact, ok := bodyMap["contact"].(map[string]interface{}); ok {
				if email, ok := contact["email"].(string); ok {
					if recipientData, exists := m.recipientResponses[email]; exists {
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(http.StatusOK)
						json.NewEncoder(w).Encode(recipientData)
						return
					}
				}
			}
		}

		// Return configured response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(m.responseStatus)
		json.NewEncoder(w).Encode(m.responseData)
	}))

	return m
}

// SetResponse sets the response data
func (m *MockFeedServer) SetResponse(data map[string]interface{}) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.responseData = data
}

// SetError sets the response status to simulate an error
func (m *MockFeedServer) SetError(status int) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.responseStatus = status
}

// SetDelay sets the response delay for timeout testing
func (m *MockFeedServer) SetDelay(delay time.Duration) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.responseDelay = delay
}

// SetRetryBehavior sets how many times to fail before succeeding
func (m *MockFeedServer) SetRetryBehavior(failCount int) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.failureCount = failCount
	m.requestCount = 0 // Reset request counter
}

// SetRecipientResponses configures per-recipient feed data
// The key is the recipient email, and the value is the response data for that recipient
func (m *MockFeedServer) SetRecipientResponses(responses map[string]map[string]interface{}) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.recipientResponses = responses
}

// GetRequests returns all logged requests
func (m *MockFeedServer) GetRequests() []FeedRequest {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	result := make([]FeedRequest, len(m.requestLog))
	copy(result, m.requestLog)
	return result
}

// ClearRequests clears the request log
func (m *MockFeedServer) ClearRequests() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.requestLog = make([]FeedRequest, 0)
	m.requestCount = 0
}

// URL returns the mock server URL
func (m *MockFeedServer) URL() string {
	return m.server.URL
}

// Close shuts down the mock server
func (m *MockFeedServer) Close() {
	m.server.Close()
}

// TestDataFeedRefreshGlobalFeed tests the refresh global feed endpoint
func TestDataFeedRefreshGlobalFeed(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	client := suite.APIClient
	factory := suite.DataFactory

	// Create test user and workspace
	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)

	err = factory.AddUserToWorkspace(user.ID, workspace.ID, "owner")
	require.NoError(t, err)

	_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID)
	require.NoError(t, err)

	err = client.Login(user.Email, "password")
	require.NoError(t, err)
	client.SetWorkspaceID(workspace.ID)

	// Create mock feed server
	mockServer := NewMockFeedServer()
	defer mockServer.Close()

	t.Run("should refresh global feed data successfully", func(t *testing.T) {
		mockServer.ClearRequests()
		mockServer.SetResponse(map[string]interface{}{
			"promo_code": "SUMMER25",
			"discount":   25,
		})

		// Create list and broadcast with GlobalFeed enabled
		list, err := factory.CreateList(workspace.ID)
		require.NoError(t, err)

		broadcast, err := factory.CreateBroadcast(workspace.ID,
			testutil.WithBroadcastGlobalFeed(&domain.GlobalFeedSettings{
				Enabled: true,
				URL:     mockServer.URL(),
				Headers: []domain.DataFeedHeader{},
			}),
			testutil.WithBroadcastAudience(domain.AudienceSettings{
				List:                list.ID,
				ExcludeUnsubscribed: true,
			}),
		)
		require.NoError(t, err)

		// Call refresh endpoint
		resp, err := client.RefreshGlobalFeed(map[string]interface{}{
			"workspace_id": workspace.ID,
			"broadcast_id": broadcast.ID,
			"url":          mockServer.URL(),
			"headers":      []interface{}{},
		})
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		// Verify response
		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		assert.Contains(t, result, "data")
		data := result["data"].(map[string]interface{})
		assert.Equal(t, "SUMMER25", data["promo_code"])
		assert.Equal(t, float64(25), data["discount"])

		// Verify mock server received request
		requests := mockServer.GetRequests()
		require.Len(t, requests, 1)
		assert.Equal(t, "POST", requests[0].Method)

		// Verify request payload contains broadcast, list, workspace info
		assert.Contains(t, requests[0].Body, "broadcast")
		assert.Contains(t, requests[0].Body, "list")
		assert.Contains(t, requests[0].Body, "workspace")
	})

	t.Run("should fail when URL missing", func(t *testing.T) {
		list, err := factory.CreateList(workspace.ID)
		require.NoError(t, err)

		broadcast, err := factory.CreateBroadcast(workspace.ID,
			testutil.WithBroadcastAudience(domain.AudienceSettings{
				List:                list.ID,
				ExcludeUnsubscribed: true,
			}),
		)
		require.NoError(t, err)

		// Request without URL should fail validation
		resp, err := client.RefreshGlobalFeed(map[string]interface{}{
			"workspace_id": workspace.ID,
			"broadcast_id": broadcast.ID,
		})
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("should return success false when feed URL is unreachable", func(t *testing.T) {
		list, err := factory.CreateList(workspace.ID)
		require.NoError(t, err)

		// Create broadcast with unreachable URL (not localhost to pass SSRF check)
		broadcast, err := factory.CreateBroadcast(workspace.ID,
			testutil.WithBroadcastGlobalFeed(&domain.GlobalFeedSettings{
				Enabled: true,
				URL:     "http://nonexistent.invalid:12345/feed",
				Headers: []domain.DataFeedHeader{},
			}),
			testutil.WithBroadcastAudience(domain.AudienceSettings{
				List:                list.ID,
				ExcludeUnsubscribed: true,
			}),
		)
		require.NoError(t, err)

		resp, err := client.RefreshGlobalFeed(map[string]interface{}{
			"workspace_id": workspace.ID,
			"broadcast_id": broadcast.ID,
			"url":          "http://nonexistent.invalid:12345/feed",
			"headers":      []interface{}{},
		})
		require.NoError(t, err)
		defer resp.Body.Close()

		// The API returns 200 with success:false for fetch errors
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		assert.Equal(t, false, result["success"])
		assert.Contains(t, result, "error")
	})
}

// TestDataFeedTestRecipientFeed tests the test recipient feed endpoint
func TestDataFeedTestRecipientFeed(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	client := suite.APIClient
	factory := suite.DataFactory

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)

	err = factory.AddUserToWorkspace(user.ID, workspace.ID, "owner")
	require.NoError(t, err)

	_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID)
	require.NoError(t, err)

	err = client.Login(user.Email, "password")
	require.NoError(t, err)
	client.SetWorkspaceID(workspace.ID)

	// Note: RecipientFeed requires HTTPS. Integration tests with TLS mock servers
	// fail due to self-signed certificate validation. The actual fetch logic is
	// covered by unit tests. Here we test the API behavior and error handling.

	t.Run("should return success false when TLS certificate is not trusted", func(t *testing.T) {
		// Create TLS mock server (uses self-signed cert)
		mockServer := NewMockFeedServerTLS()
		defer mockServer.Close()

		mockServer.SetResponse(map[string]interface{}{
			"loyalty_points": 1500,
		})

		// Create contact
		contact, err := factory.CreateContact(workspace.ID, testutil.WithContactEmail("testfeed@example.com"))
		require.NoError(t, err)

		list, err := factory.CreateList(workspace.ID)
		require.NoError(t, err)

		broadcast, err := factory.CreateBroadcast(workspace.ID,
			testutil.WithBroadcastRecipientFeed(&domain.RecipientFeedSettings{
				Enabled: true,
				URL:     mockServer.URL(),
				Headers: []domain.DataFeedHeader{},
			}),
			testutil.WithBroadcastAudience(domain.AudienceSettings{
				List:                list.ID,
				ExcludeUnsubscribed: true,
			}),
		)
		require.NoError(t, err)

		// Test endpoint returns 200 with success:false due to TLS error
		resp, err := client.TestRecipientFeed(map[string]interface{}{
			"workspace_id":  workspace.ID,
			"broadcast_id":  broadcast.ID,
			"contact_email": contact.Email,
			"url":           mockServer.URL(),
			"headers":       []interface{}{},
		})
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		// Should return success: false with error message
		assert.Equal(t, false, result["success"])
		assert.Contains(t, result, "error")
	})

	t.Run("should fail for non-existent contact", func(t *testing.T) {
		list, err := factory.CreateList(workspace.ID)
		require.NoError(t, err)

		broadcast, err := factory.CreateBroadcast(workspace.ID,
			testutil.WithBroadcastRecipientFeed(&domain.RecipientFeedSettings{
				Enabled: true,
				URL:     "https://example.com/feed", // URL doesn't matter, contact check happens first
				Headers: []domain.DataFeedHeader{},
			}),
			testutil.WithBroadcastAudience(domain.AudienceSettings{
				List:                list.ID,
				ExcludeUnsubscribed: true,
			}),
		)
		require.NoError(t, err)

		resp, err := client.TestRecipientFeed(map[string]interface{}{
			"workspace_id":  workspace.ID,
			"broadcast_id":  broadcast.ID,
			"contact_email": "nonexistent@example.com",
			"url":           "https://example.com/feed",
			"headers":       []interface{}{},
		})
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("should fail when URL missing", func(t *testing.T) {
		list, err := factory.CreateList(workspace.ID)
		require.NoError(t, err)

		broadcast, err := factory.CreateBroadcast(workspace.ID,
			testutil.WithBroadcastAudience(domain.AudienceSettings{
				List:                list.ID,
				ExcludeUnsubscribed: true,
			}),
		)
		require.NoError(t, err)

		// Request without URL should fail validation
		resp, err := client.TestRecipientFeed(map[string]interface{}{
			"workspace_id": workspace.ID,
			"broadcast_id": broadcast.ID,
		})
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}

// TestDataFeedCustomHeaders tests that custom headers are sent to the feed endpoint
func TestDataFeedCustomHeaders(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	client := suite.APIClient
	factory := suite.DataFactory

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)

	err = factory.AddUserToWorkspace(user.ID, workspace.ID, "owner")
	require.NoError(t, err)

	_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID)
	require.NoError(t, err)

	err = client.Login(user.Email, "password")
	require.NoError(t, err)
	client.SetWorkspaceID(workspace.ID)

	mockServer := NewMockFeedServer()
	defer mockServer.Close()

	t.Run("should send custom headers to feed endpoint", func(t *testing.T) {
		mockServer.ClearRequests()
		mockServer.SetResponse(map[string]interface{}{
			"message": "authenticated",
		})

		list, err := factory.CreateList(workspace.ID)
		require.NoError(t, err)

		broadcast, err := factory.CreateBroadcast(workspace.ID,
			testutil.WithBroadcastGlobalFeed(&domain.GlobalFeedSettings{
				Enabled: true,
				URL:     mockServer.URL(),
				Headers: []domain.DataFeedHeader{
					{Name: "Authorization", Value: "Bearer test-token-123"},
					{Name: "X-Custom-Header", Value: "custom-value"},
				},
			}),
			testutil.WithBroadcastAudience(domain.AudienceSettings{
				List:                list.ID,
				ExcludeUnsubscribed: true,
			}),
		)
		require.NoError(t, err)

		resp, err := client.RefreshGlobalFeed(map[string]interface{}{
			"workspace_id": workspace.ID,
			"broadcast_id": broadcast.ID,
			"url":          mockServer.URL(),
			"headers": []map[string]string{
				{"name": "Authorization", "value": "Bearer test-token-123"},
				{"name": "X-Custom-Header", "value": "custom-value"},
			},
		})
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		// Verify custom headers were sent
		requests := mockServer.GetRequests()
		require.Len(t, requests, 1)

		assert.Equal(t, "Bearer test-token-123", requests[0].Headers.Get("Authorization"))
		assert.Equal(t, "custom-value", requests[0].Headers.Get("X-Custom-Header"))
	})
}

// TestDataFeedTimeout tests timeout handling for feed requests
func TestDataFeedTimeout(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	client := suite.APIClient
	factory := suite.DataFactory

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)

	err = factory.AddUserToWorkspace(user.ID, workspace.ID, "owner")
	require.NoError(t, err)

	_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID)
	require.NoError(t, err)

	err = client.Login(user.Email, "password")
	require.NoError(t, err)
	client.SetWorkspaceID(workspace.ID)

	mockServer := NewMockFeedServer()
	defer mockServer.Close()

	t.Run("should timeout when feed takes too long", func(t *testing.T) {
		mockServer.ClearRequests()
		mockServer.SetDelay(5 * time.Second) // Delay longer than timeout
		mockServer.SetResponse(map[string]interface{}{
			"message": "should not see this",
		})

		list, err := factory.CreateList(workspace.ID)
		require.NoError(t, err)

		broadcast, err := factory.CreateBroadcast(workspace.ID,
			testutil.WithBroadcastGlobalFeed(&domain.GlobalFeedSettings{
				Enabled: true,
				URL:     mockServer.URL(),
				Headers: []domain.DataFeedHeader{},
			}),
			testutil.WithBroadcastAudience(domain.AudienceSettings{
				List:                list.ID,
				ExcludeUnsubscribed: true,
			}),
		)
		require.NoError(t, err)

		start := time.Now()
		resp, err := client.RefreshGlobalFeed(map[string]interface{}{
			"workspace_id": workspace.ID,
			"broadcast_id": broadcast.ID,
			"url":          mockServer.URL(),
			"headers":      []interface{}{},
		})
		require.NoError(t, err)
		defer resp.Body.Close()

		elapsed := time.Since(start)

		// API returns 200 with success:false for timeout errors
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		assert.Equal(t, false, result["success"])
		assert.Contains(t, result, "error")

		// Should complete in approximately 5 seconds (the hardcoded timeout) with some buffer
		assert.True(t, elapsed < 7*time.Second, "request should timeout within expected time")

		// Reset delay for other tests
		mockServer.SetDelay(0)
	})
}

// TestDataFeedEmptyResponse tests handling of empty feed responses
func TestDataFeedEmptyResponse(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	client := suite.APIClient
	factory := suite.DataFactory

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)

	err = factory.AddUserToWorkspace(user.ID, workspace.ID, "owner")
	require.NoError(t, err)

	_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID)
	require.NoError(t, err)

	err = client.Login(user.Email, "password")
	require.NoError(t, err)
	client.SetWorkspaceID(workspace.ID)

	mockServer := NewMockFeedServer()
	defer mockServer.Close()

	t.Run("should handle empty data response", func(t *testing.T) {
		mockServer.ClearRequests()
		mockServer.SetResponse(map[string]interface{}{})

		list, err := factory.CreateList(workspace.ID)
		require.NoError(t, err)

		broadcast, err := factory.CreateBroadcast(workspace.ID,
			testutil.WithBroadcastGlobalFeed(&domain.GlobalFeedSettings{
				Enabled: true,
				URL:     mockServer.URL(),
				Headers: []domain.DataFeedHeader{},
			}),
			testutil.WithBroadcastAudience(domain.AudienceSettings{
				List:                list.ID,
				ExcludeUnsubscribed: true,
			}),
		)
		require.NoError(t, err)

		resp, err := client.RefreshGlobalFeed(map[string]interface{}{
			"workspace_id": workspace.ID,
			"broadcast_id": broadcast.ID,
			"url":          mockServer.URL(),
			"headers":      []interface{}{},
		})
		require.NoError(t, err)
		defer resp.Body.Close()

		// Empty data should still be successful
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		// Verify success and data exists (may contain metadata like _success, _fetched_at)
		assert.Equal(t, true, result["success"])
		assert.Contains(t, result, "data")
	})
}

// TestDataFeedHTTPErrors tests handling of HTTP errors from feed endpoint
func TestDataFeedHTTPErrors(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	client := suite.APIClient
	factory := suite.DataFactory

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)

	err = factory.AddUserToWorkspace(user.ID, workspace.ID, "owner")
	require.NoError(t, err)

	_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID)
	require.NoError(t, err)

	err = client.Login(user.Email, "password")
	require.NoError(t, err)
	client.SetWorkspaceID(workspace.ID)

	mockServer := NewMockFeedServer()
	defer mockServer.Close()

	testCases := []struct {
		name       string
		httpStatus int
	}{
		{"should handle 400 Bad Request", http.StatusBadRequest},
		{"should handle 401 Unauthorized", http.StatusUnauthorized},
		{"should handle 403 Forbidden", http.StatusForbidden},
		{"should handle 404 Not Found", http.StatusNotFound},
		{"should handle 500 Internal Server Error", http.StatusInternalServerError},
		{"should handle 502 Bad Gateway", http.StatusBadGateway},
		{"should handle 503 Service Unavailable", http.StatusServiceUnavailable},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockServer.ClearRequests()
			mockServer.SetError(tc.httpStatus)

			list, err := factory.CreateList(workspace.ID)
			require.NoError(t, err)

			broadcast, err := factory.CreateBroadcast(workspace.ID,
				testutil.WithBroadcastGlobalFeed(&domain.GlobalFeedSettings{
					Enabled: true,
					URL:     mockServer.URL(),
					Headers: []domain.DataFeedHeader{},
				}),
				testutil.WithBroadcastAudience(domain.AudienceSettings{
					List:                list.ID,
					ExcludeUnsubscribed: true,
				}),
			)
			require.NoError(t, err)

			resp, err := client.RefreshGlobalFeed(map[string]interface{}{
				"workspace_id": workspace.ID,
				"broadcast_id": broadcast.ID,
				"url":          mockServer.URL(),
				"headers":      []interface{}{},
			})
			require.NoError(t, err)
			defer resp.Body.Close()

			// API returns 200 OK with success: false for feed errors
			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var result map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&result)
			require.NoError(t, err)
			assert.Equal(t, false, result["success"])
			assert.Contains(t, result, "error")

			// Reset for next test
			mockServer.SetError(http.StatusOK)
		})
	}
}

// TestDataFeedRetryLogic tests retry behavior for recipient feed
// Note: This test is skipped because RecipientFeed requires HTTPS and TLS mock servers
// use self-signed certificates that aren't trusted. Retry logic is covered by unit tests.
func TestDataFeedRetryLogic(t *testing.T) {
	t.Skip("Skipped: RecipientFeed requires trusted HTTPS certificates. Retry logic is covered by unit tests.")
}

// TestDataFeedInvalidJSON tests handling of invalid JSON responses
func TestDataFeedInvalidJSON(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	client := suite.APIClient
	factory := suite.DataFactory

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)

	err = factory.AddUserToWorkspace(user.ID, workspace.ID, "owner")
	require.NoError(t, err)

	_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID)
	require.NoError(t, err)

	err = client.Login(user.Email, "password")
	require.NoError(t, err)
	client.SetWorkspaceID(workspace.ID)

	// Create a mock server that returns invalid JSON
	invalidJSONServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("not valid json{{{"))
	}))
	defer invalidJSONServer.Close()

	t.Run("should handle invalid JSON response", func(t *testing.T) {
		list, err := factory.CreateList(workspace.ID)
		require.NoError(t, err)

		broadcast, err := factory.CreateBroadcast(workspace.ID,
			testutil.WithBroadcastGlobalFeed(&domain.GlobalFeedSettings{
				Enabled: true,
				URL:     invalidJSONServer.URL,
				Headers: []domain.DataFeedHeader{},
			}),
			testutil.WithBroadcastAudience(domain.AudienceSettings{
				List:                list.ID,
				ExcludeUnsubscribed: true,
			}),
		)
		require.NoError(t, err)

		resp, err := client.RefreshGlobalFeed(map[string]interface{}{
			"workspace_id": workspace.ID,
			"broadcast_id": broadcast.ID,
			"url":          invalidJSONServer.URL,
			"headers":      []interface{}{},
		})
		require.NoError(t, err)
		defer resp.Body.Close()

		// API returns 200 OK with success: false for invalid JSON
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)
		assert.Equal(t, false, result["success"])
		assert.Contains(t, result, "error")
	})
}

// TestDataFeedDisabledFeed tests behavior when feed is explicitly disabled
func TestDataFeedDisabledFeed(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	client := suite.APIClient
	factory := suite.DataFactory

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)

	err = factory.AddUserToWorkspace(user.ID, workspace.ID, "owner")
	require.NoError(t, err)

	_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID)
	require.NoError(t, err)

	err = client.Login(user.Email, "password")
	require.NoError(t, err)
	client.SetWorkspaceID(workspace.ID)

	t.Run("should fail refresh when URL is missing from request", func(t *testing.T) {
		list, err := factory.CreateList(workspace.ID)
		require.NoError(t, err)

		broadcast, err := factory.CreateBroadcast(workspace.ID,
			testutil.WithBroadcastAudience(domain.AudienceSettings{
				List:                list.ID,
				ExcludeUnsubscribed: true,
			}),
		)
		require.NoError(t, err)

		// Request without URL should fail validation
		resp, err := client.RefreshGlobalFeed(map[string]interface{}{
			"workspace_id": workspace.ID,
			"broadcast_id": broadcast.ID,
		})
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("should fail test when URL is missing from request", func(t *testing.T) {
		list, err := factory.CreateList(workspace.ID)
		require.NoError(t, err)

		broadcast, err := factory.CreateBroadcast(workspace.ID,
			testutil.WithBroadcastAudience(domain.AudienceSettings{
				List:                list.ID,
				ExcludeUnsubscribed: true,
			}),
		)
		require.NoError(t, err)

		// Request without URL should fail validation
		resp, err := client.TestRecipientFeed(map[string]interface{}{
			"workspace_id": workspace.ID,
			"broadcast_id": broadcast.ID,
		})
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}

// TestDataFeedRequestPayload tests that the correct payload is sent to feed endpoints
func TestDataFeedRequestPayload(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	client := suite.APIClient
	factory := suite.DataFactory

	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)

	err = factory.AddUserToWorkspace(user.ID, workspace.ID, "owner")
	require.NoError(t, err)

	_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID)
	require.NoError(t, err)

	err = client.Login(user.Email, "password")
	require.NoError(t, err)
	client.SetWorkspaceID(workspace.ID)

	mockServer := NewMockFeedServer()
	defer mockServer.Close()

	t.Run("GlobalFeed should receive broadcast, list, and workspace info", func(t *testing.T) {
		mockServer.ClearRequests()
		mockServer.SetResponse(map[string]interface{}{
			"message": "ok",
		})

		list, err := factory.CreateList(workspace.ID, testutil.WithListName("Marketing List"))
		require.NoError(t, err)

		broadcast, err := factory.CreateBroadcast(workspace.ID,
			testutil.WithBroadcastName("Newsletter Campaign"),
			testutil.WithBroadcastGlobalFeed(&domain.GlobalFeedSettings{
				Enabled: true,
				URL:     mockServer.URL(),
				Headers: []domain.DataFeedHeader{},
			}),
			testutil.WithBroadcastAudience(domain.AudienceSettings{
				List:                list.ID,
				ExcludeUnsubscribed: true,
			}),
		)
		require.NoError(t, err)

		resp, err := client.RefreshGlobalFeed(map[string]interface{}{
			"workspace_id": workspace.ID,
			"broadcast_id": broadcast.ID,
			"url":          mockServer.URL(),
			"headers":      []interface{}{},
		})
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		requests := mockServer.GetRequests()
		require.Len(t, requests, 1)

		body := requests[0].Body

		// Verify broadcast info
		broadcastInfo := body["broadcast"].(map[string]interface{})
		assert.Equal(t, broadcast.ID, broadcastInfo["id"])
		assert.Equal(t, "Newsletter Campaign", broadcastInfo["name"])

		// Verify list info
		listInfo := body["list"].(map[string]interface{})
		assert.Equal(t, list.ID, listInfo["id"])
		assert.Equal(t, "Marketing List", listInfo["name"])

		// Verify workspace info
		workspaceInfo := body["workspace"].(map[string]interface{})
		assert.Equal(t, workspace.ID, workspaceInfo["id"])
		assert.Equal(t, workspace.Name, workspaceInfo["name"])
	})
}

// TestDataFeed_CreateBroadcastViaAPI tests that data_feed settings are properly
// persisted when creating a broadcast via the API (not via factory)
// Note: Uses public URLs to pass SSRF validation - actual feed fetching is tested elsewhere
func TestDataFeed_CreateBroadcastViaAPI(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	client := suite.APIClient
	factory := suite.DataFactory

	// Create test user and workspace
	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)

	err = factory.AddUserToWorkspace(user.ID, workspace.ID, "owner")
	require.NoError(t, err)

	_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID)
	require.NoError(t, err)

	err = client.Login(user.Email, "password")
	require.NoError(t, err)
	client.SetWorkspaceID(workspace.ID)

	// Create list and template
	list, err := factory.CreateList(workspace.ID)
	require.NoError(t, err)
	template, err := factory.CreateTemplate(workspace.ID)
	require.NoError(t, err)

	t.Run("creates broadcast with global feed via API", func(t *testing.T) {
		// Use a valid public URL that passes SSRF validation
		// This tests that data_feed JSON is properly deserialized in the create request
		testURL := "https://api.example.com/feed/global"

		resp, err := client.CreateBroadcast(map[string]interface{}{
			"workspace_id": workspace.ID,
			"name":         "API Created Broadcast With Feed",
			"audience": map[string]interface{}{
				"list":                 list.ID,
				"exclude_unsubscribed": true,
			},
			"test_settings": map[string]interface{}{
				"enabled": false,
				"variations": []map[string]interface{}{
					{"template_id": template.ID},
				},
			},
			"data_feed": map[string]interface{}{
				"global_feed": map[string]interface{}{
					"enabled": true,
					"url":     testURL,
					"headers": []map[string]interface{}{
						{"name": "X-API-Key", "value": "secret123"},
						{"name": "Authorization", "value": "Bearer token"},
					},
				},
			},
		})
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, http.StatusCreated, resp.StatusCode)

		// Parse response
		var createResult map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&createResult)
		require.NoError(t, err)

		broadcast := createResult["broadcast"].(map[string]interface{})
		broadcastID := broadcast["id"].(string)
		require.NotEmpty(t, broadcastID)

		// Verify data_feed was persisted by fetching the broadcast
		getResp, err := client.GetBroadcast(broadcastID)
		require.NoError(t, err)
		defer getResp.Body.Close()

		require.Equal(t, http.StatusOK, getResp.StatusCode)

		var getResult map[string]interface{}
		err = json.NewDecoder(getResp.Body).Decode(&getResult)
		require.NoError(t, err)

		fetchedBroadcast := getResult["broadcast"].(map[string]interface{})
		require.Contains(t, fetchedBroadcast, "data_feed")

		dataFeed := fetchedBroadcast["data_feed"].(map[string]interface{})
		require.Contains(t, dataFeed, "global_feed")

		globalFeed := dataFeed["global_feed"].(map[string]interface{})
		assert.True(t, globalFeed["enabled"].(bool))
		assert.Equal(t, testURL, globalFeed["url"])

		headers := globalFeed["headers"].([]interface{})
		assert.Len(t, headers, 2)

		// Verify headers are persisted correctly
		header1 := headers[0].(map[string]interface{})
		assert.Equal(t, "X-API-Key", header1["name"])
		assert.Equal(t, "secret123", header1["value"])

		header2 := headers[1].(map[string]interface{})
		assert.Equal(t, "Authorization", header2["name"])
		assert.Equal(t, "Bearer token", header2["value"])
	})

	t.Run("creates broadcast with recipient feed via API", func(t *testing.T) {
		// RecipientFeed requires HTTPS
		testURL := "https://api.example.com/feed/recipient"

		resp, err := client.CreateBroadcast(map[string]interface{}{
			"workspace_id": workspace.ID,
			"name":         "API Created Broadcast With Recipient Feed",
			"audience": map[string]interface{}{
				"list":                 list.ID,
				"exclude_unsubscribed": true,
			},
			"test_settings": map[string]interface{}{
				"enabled": false,
				"variations": []map[string]interface{}{
					{"template_id": template.ID},
				},
			},
			"data_feed": map[string]interface{}{
				"recipient_feed": map[string]interface{}{
					"enabled": true,
					"url":     testURL,
					"headers": []map[string]interface{}{
						{"name": "X-Custom-Header", "value": "custom-value"},
					},
				},
			},
		})
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, http.StatusCreated, resp.StatusCode)

		var createResult map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&createResult)
		require.NoError(t, err)

		broadcast := createResult["broadcast"].(map[string]interface{})
		broadcastID := broadcast["id"].(string)

		// Verify recipient feed was persisted
		getResp, err := client.GetBroadcast(broadcastID)
		require.NoError(t, err)
		defer getResp.Body.Close()

		var getResult map[string]interface{}
		err = json.NewDecoder(getResp.Body).Decode(&getResult)
		require.NoError(t, err)

		fetchedBroadcast := getResult["broadcast"].(map[string]interface{})
		dataFeed := fetchedBroadcast["data_feed"].(map[string]interface{})

		require.Contains(t, dataFeed, "recipient_feed")
		recipientFeed := dataFeed["recipient_feed"].(map[string]interface{})
		assert.True(t, recipientFeed["enabled"].(bool))
		assert.Equal(t, testURL, recipientFeed["url"])

		headers := recipientFeed["headers"].([]interface{})
		assert.Len(t, headers, 1)
	})

	t.Run("returns 400 for invalid recipient feed URL (HTTP instead of HTTPS)", func(t *testing.T) {
		resp, err := client.CreateBroadcast(map[string]interface{}{
			"workspace_id": workspace.ID,
			"name":         "Invalid Feed URL Test",
			"audience": map[string]interface{}{
				"list":                 list.ID,
				"exclude_unsubscribed": true,
			},
			"test_settings": map[string]interface{}{
				"enabled": false,
				"variations": []map[string]interface{}{
					{"template_id": template.ID},
				},
			},
			"data_feed": map[string]interface{}{
				"recipient_feed": map[string]interface{}{
					"enabled": true,
					"url":     "http://insecure.example.com/feed", // HTTP not HTTPS
					"headers": []interface{}{},
				},
			},
		})
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

		var errorResp map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&errorResp)
		require.NoError(t, err)

		assert.Contains(t, errorResp["error"], "https")
	})

	t.Run("returns 400 for localhost URL (SSRF protection)", func(t *testing.T) {
		resp, err := client.CreateBroadcast(map[string]interface{}{
			"workspace_id": workspace.ID,
			"name":         "Localhost URL Test",
			"audience": map[string]interface{}{
				"list":                 list.ID,
				"exclude_unsubscribed": true,
			},
			"test_settings": map[string]interface{}{
				"enabled": false,
				"variations": []map[string]interface{}{
					{"template_id": template.ID},
				},
			},
			"data_feed": map[string]interface{}{
				"global_feed": map[string]interface{}{
					"enabled": true,
					"url":     "http://localhost:8080/feed",
					"headers": []interface{}{},
				},
			},
		})
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

		var errorResp map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&errorResp)
		require.NoError(t, err)

		assert.Contains(t, errorResp["error"], "localhost")
	})
}

// TestDataFeed_RecipientFeedFailure_PausesBroadcast verifies that when the recipient
// feed endpoint persistently returns errors, the broadcast is paused (not failed or stuck sending).
func TestDataFeed_RecipientFeedFailure_PausesBroadcast(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	client := suite.APIClient
	factory := suite.DataFactory

	// Setup: Create user and workspace
	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)

	err = factory.AddUserToWorkspace(user.ID, workspace.ID, "owner")
	require.NoError(t, err)

	// Setup SMTP provider (Mailpit on localhost:1025)
	_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID,
		testutil.WithIntegrationEmailProvider(domain.EmailProvider{
			Kind: domain.EmailProviderKindSMTP,
			Senders: []domain.EmailSender{
				domain.NewEmailSender("noreply@notifuse.test", "Feed Failure Test"),
			},
			SMTP: &domain.SMTPSettings{
				Host:     "localhost",
				Port:     1025,
				Username: "",
				Password: "",
				UseTLS:   false,
			},
			RateLimitPerMinute: 2000,
		}))
	require.NoError(t, err)

	// Start email queue worker
	workerCtx, cancelWorker := context.WithCancel(context.Background())
	defer cancelWorker()
	err = suite.ServerManager.StartBackgroundWorkers(workerCtx)
	require.NoError(t, err)
	time.Sleep(100 * time.Millisecond)

	// Login
	err = client.Login(user.Email, "password")
	require.NoError(t, err)
	client.SetWorkspaceID(workspace.ID)

	t.Run("broadcast pauses when recipient feed always returns 500", func(t *testing.T) {
		// Create mock feed server that ALWAYS returns 500
		mockServer := NewMockFeedServer()
		defer mockServer.Close()
		mockServer.SetError(http.StatusInternalServerError)

		// Create list and contact
		list, err := factory.CreateList(workspace.ID)
		require.NoError(t, err)

		contact, err := factory.CreateContact(workspace.ID,
			testutil.WithContactEmail("feedfail@example.com"),
			testutil.WithContactName("Test", "User"))
		require.NoError(t, err)

		_, err = factory.CreateContactList(workspace.ID,
			testutil.WithContactListEmail(contact.Email),
			testutil.WithContactListListID(list.ID),
			testutil.WithContactListStatus(domain.ContactListStatusActive))
		require.NoError(t, err)

		// Create template
		template, err := factory.CreateTemplate(workspace.ID,
			testutil.WithTemplateName("Feed Failure Template"),
			testutil.WithTemplateSubject("Hello {{ contact.first_name }}"),
			testutil.WithTemplateEmailContent("Data: {{ recipient_feed.value }}"))
		require.NoError(t, err)

		// Create broadcast with RecipientFeed pointing to failing server
		// Factory bypasses SSRF/HTTPS validation, so HTTP mock URL works
		broadcast, err := factory.CreateBroadcast(workspace.ID,
			testutil.WithBroadcastName("Feed Failure Pause Test"),
			testutil.WithBroadcastTemplateID(template.ID),
			testutil.WithBroadcastRecipientFeed(&domain.RecipientFeedSettings{
				Enabled: true,
				URL:     mockServer.URL(),
				Headers: []domain.DataFeedHeader{},
			}),
			testutil.WithBroadcastAudience(domain.AudienceSettings{
				List:                list.ID,
				ExcludeUnsubscribed: true,
			}))
		require.NoError(t, err)

		// Schedule broadcast
		t.Log("Scheduling broadcast with failing recipient feed...")
		scheduleResp, err := client.ScheduleBroadcast(map[string]interface{}{
			"workspace_id": workspace.ID,
			"id":           broadcast.ID,
			"send_now":     true,
		})
		require.NoError(t, err)
		scheduleResp.Body.Close()

		// Wait for broadcast to reach "paused" status
		// FetchRecipient retries 2 times with 5s delay, so this takes ~10-15s
		t.Log("Waiting for broadcast to be paused due to feed failure...")
		status, err := testutil.WaitForBroadcastStatusWithExecution(t, client, broadcast.ID,
			[]string{"paused"}, 90*time.Second)
		require.NoError(t, err, "broadcast should be paused after recipient feed failure")
		assert.Equal(t, "paused", status)

		// Verify broadcast details
		getResp, err := client.GetBroadcast(broadcast.ID)
		require.NoError(t, err)
		defer getResp.Body.Close()

		var result map[string]interface{}
		err = json.NewDecoder(getResp.Body).Decode(&result)
		require.NoError(t, err)

		broadcastData := result["broadcast"].(map[string]interface{})
		assert.Equal(t, "paused", broadcastData["status"])
		assert.NotNil(t, broadcastData["paused_at"], "paused_at should be set")

		if pauseReason, ok := broadcastData["pause_reason"].(string); ok {
			assert.Contains(t, strings.ToLower(pauseReason), "recipient feed",
				"pause_reason should mention recipient feed")
		} else {
			t.Error("pause_reason should be set as a string")
		}

		// Verify mock server received requests (retries)
		requests := mockServer.GetRequests()
		assert.Greater(t, len(requests), 0, "Mock server should have received at least one request")
		t.Logf("Mock server received %d requests (retries included)", len(requests))
	})
}

// TestDataFeed_UpdateBroadcastViaAPI tests that data_feed settings can be updated
// and that GlobalFeedData is preserved across updates
// Note: Uses factory.CreateBroadcast to bypass SSRF validation for mock server URLs
func TestDataFeed_UpdateBroadcastViaAPI(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	client := suite.APIClient
	factory := suite.DataFactory

	// Create test user and workspace
	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)

	err = factory.AddUserToWorkspace(user.ID, workspace.ID, "owner")
	require.NoError(t, err)

	_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID)
	require.NoError(t, err)

	err = client.Login(user.Email, "password")
	require.NoError(t, err)
	client.SetWorkspaceID(workspace.ID)

	// Create list and template
	list, err := factory.CreateList(workspace.ID)
	require.NoError(t, err)
	template, err := factory.CreateTemplate(workspace.ID)
	require.NoError(t, err)

	// Create mock feed server
	mockServer := NewMockFeedServer()
	defer mockServer.Close()
	mockServer.SetResponse(map[string]interface{}{
		"cached_value": "important_data",
		"timestamp":    "2024-01-15T10:30:00Z",
	})

	t.Run("preserves GlobalFeedData when updating broadcast", func(t *testing.T) {
		fetchedAt := time.Now().UTC()

		// 1. Create broadcast with global feed and pre-populated data via FACTORY
		broadcast, err := factory.CreateBroadcast(workspace.ID,
			testutil.WithBroadcastGlobalFeed(&domain.GlobalFeedSettings{
				Enabled: true,
				URL:     mockServer.URL(),
				Headers: []domain.DataFeedHeader{},
			}),
			testutil.WithBroadcastGlobalFeedData(
				map[string]interface{}{
					"cached_value": "important_data",
					"timestamp":    "2024-01-15T10:30:00Z",
				},
				&fetchedAt,
			),
			testutil.WithBroadcastAudience(domain.AudienceSettings{
				List:                list.ID,
				ExcludeUnsubscribed: true,
			}),
		)
		require.NoError(t, err)

		// 2. Verify data exists
		getResp1, err := client.GetBroadcast(broadcast.ID)
		require.NoError(t, err)

		var getResult1 map[string]interface{}
		err = json.NewDecoder(getResp1.Body).Decode(&getResult1)
		getResp1.Body.Close()
		require.NoError(t, err)

		broadcastData1 := getResult1["broadcast"].(map[string]interface{})
		dataFeed1 := broadcastData1["data_feed"].(map[string]interface{})
		require.NotNil(t, dataFeed1["global_feed_data"])
		require.NotNil(t, dataFeed1["global_feed_fetched_at"])

		originalFetchedAt := dataFeed1["global_feed_fetched_at"].(string)
		originalName := broadcastData1["name"].(string)

		// Get template ID from the broadcast variations for the update request
		testSettings := broadcastData1["test_settings"].(map[string]interface{})
		variations := testSettings["variations"].([]interface{})
		firstVariation := variations[0].(map[string]interface{})
		templateID := firstVariation["template_id"].(string)

		// 4. Update broadcast via API (change name only)
		// Use a valid public URL that passes SSRF validation
		updateResp, err := client.UpdateBroadcast(map[string]interface{}{
			"workspace_id": workspace.ID,
			"id":           broadcast.ID,
			"name":         "Updated Broadcast Name",
			"audience": map[string]interface{}{
				"list":                 list.ID,
				"exclude_unsubscribed": true,
			},
			"test_settings": map[string]interface{}{
				"enabled": false,
				"variations": []map[string]interface{}{
					{"template_id": templateID},
				},
			},
			"data_feed": map[string]interface{}{
				"global_feed": map[string]interface{}{
					"enabled": true,
					"url":     "https://api.example.com/feed",
					"headers": []interface{}{},
				},
			},
		})
		require.NoError(t, err)
		defer updateResp.Body.Close()

		require.Equal(t, http.StatusOK, updateResp.StatusCode)

		// 5. Verify GlobalFeedData and GlobalFeedFetchedAt are preserved
		getResp2, err := client.GetBroadcast(broadcast.ID)
		require.NoError(t, err)
		defer getResp2.Body.Close()

		var getResult2 map[string]interface{}
		err = json.NewDecoder(getResp2.Body).Decode(&getResult2)
		require.NoError(t, err)

		broadcast2 := getResult2["broadcast"].(map[string]interface{})
		assert.Equal(t, "Updated Broadcast Name", broadcast2["name"])
		assert.NotEqual(t, originalName, broadcast2["name"])

		dataFeed2 := broadcast2["data_feed"].(map[string]interface{})

		// GlobalFeedData should be preserved
		assert.NotNil(t, dataFeed2["global_feed_data"])
		feedData := dataFeed2["global_feed_data"].(map[string]interface{})
		assert.Equal(t, "important_data", feedData["cached_value"])

		// GlobalFeedFetchedAt should be preserved
		assert.NotNil(t, dataFeed2["global_feed_fetched_at"])
		assert.Equal(t, originalFetchedAt, dataFeed2["global_feed_fetched_at"])

		// URL should be updated
		globalFeed := dataFeed2["global_feed"].(map[string]interface{})
		assert.Equal(t, "https://api.example.com/feed", globalFeed["url"])
	})

	t.Run("adds data_feed to existing broadcast without feed", func(t *testing.T) {
		// Create broadcast without data_feed via API (no SSRF issue since no URL)
		createResp, err := client.CreateBroadcast(map[string]interface{}{
			"workspace_id": workspace.ID,
			"name":         "Broadcast Without Feed",
			"audience": map[string]interface{}{
				"list":                 list.ID,
				"exclude_unsubscribed": true,
			},
			"test_settings": map[string]interface{}{
				"enabled": false,
				"variations": []map[string]interface{}{
					{"template_id": template.ID},
				},
			},
			// No data_feed
		})
		require.NoError(t, err)

		var createResult map[string]interface{}
		err = json.NewDecoder(createResp.Body).Decode(&createResult)
		createResp.Body.Close()
		require.NoError(t, err)

		broadcastID := createResult["broadcast"].(map[string]interface{})["id"].(string)

		// Update to add data_feed with valid public URL
		updateResp, err := client.UpdateBroadcast(map[string]interface{}{
			"workspace_id": workspace.ID,
			"id":           broadcastID,
			"name":         "Broadcast With Feed Now",
			"audience": map[string]interface{}{
				"list":                 list.ID,
				"exclude_unsubscribed": true,
			},
			"test_settings": map[string]interface{}{
				"enabled": false,
				"variations": []map[string]interface{}{
					{"template_id": template.ID},
				},
			},
			"data_feed": map[string]interface{}{
				"global_feed": map[string]interface{}{
					"enabled": true,
					"url":     "https://api.example.com/global-feed",
					"headers": []interface{}{},
				},
			},
		})
		require.NoError(t, err)
		defer updateResp.Body.Close()

		require.Equal(t, http.StatusOK, updateResp.StatusCode)

		// Verify data_feed was added
		getResp, err := client.GetBroadcast(broadcastID)
		require.NoError(t, err)
		defer getResp.Body.Close()

		var getResult map[string]interface{}
		err = json.NewDecoder(getResp.Body).Decode(&getResult)
		require.NoError(t, err)

		dataFeed := getResult["broadcast"].(map[string]interface{})["data_feed"].(map[string]interface{})
		require.NotNil(t, dataFeed["global_feed"])

		globalFeed := dataFeed["global_feed"].(map[string]interface{})
		assert.True(t, globalFeed["enabled"].(bool))
		assert.Equal(t, "https://api.example.com/global-feed", globalFeed["url"])
	})
}
