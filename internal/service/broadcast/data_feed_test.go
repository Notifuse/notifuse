package broadcast

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDataFeedFetcher_FetchGlobal_NilSettings(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()

	fetcher := NewDataFeedFetcher(mockLogger)

	// Test: nil settings returns nil, nil
	result, err := fetcher.FetchGlobal(context.Background(), nil, nil)

	assert.NoError(t, err)
	assert.Nil(t, result)
}

func TestDataFeedFetcher_FetchGlobal_Disabled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()

	fetcher := NewDataFeedFetcher(mockLogger)

	// Test: disabled settings returns nil, nil
	settings := &domain.GlobalFeedSettings{
		Enabled: false,
		URL:     "https://example.com/feed",
	}

	result, err := fetcher.FetchGlobal(context.Background(), settings, nil)

	assert.NoError(t, err)
	assert.Nil(t, result)
}

func TestDataFeedFetcher_FetchGlobal_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

	// Create a test server
	var receivedMethod string
	var receivedContentType string
	var receivedUserAgent string
	var receivedBody map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		receivedContentType = r.Header.Get("Content-Type")
		receivedUserAgent = r.Header.Get("User-Agent")

		// Parse request body
		if err := json.NewDecoder(r.Body).Decode(&receivedBody); err != nil {
			t.Fatalf("Failed to parse request body: %v", err)
		}

		// Return successful response
		response := map[string]interface{}{
			"products": []interface{}{
				map[string]interface{}{"id": "1", "name": "Product 1"},
				map[string]interface{}{"id": "2", "name": "Product 2"},
			},
			"featured_item": "Featured Product",
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	fetcher := NewDataFeedFetcher(mockLogger)

	settings := &domain.GlobalFeedSettings{
		Enabled: true,
		URL:     server.URL,
		Headers: []domain.DataFeedHeader{},
	}

	payload := &domain.GlobalFeedRequestPayload{
		Broadcast: domain.GlobalFeedBroadcast{
			ID:   "broadcast-123",
			Name: "Test Broadcast",
		},
		List: domain.GlobalFeedList{
			ID:   "list-456",
			Name: "Test List",
		},
		Workspace: domain.GlobalFeedWorkspace{
			ID:   "workspace-789",
			Name: "Test Workspace",
		},
	}

	result, err := fetcher.FetchGlobal(context.Background(), settings, payload)

	// Verify request
	assert.Equal(t, "POST", receivedMethod)
	assert.Equal(t, "application/json", receivedContentType)
	assert.Contains(t, receivedUserAgent, "Notifuse")

	// Verify payload was sent
	require.NotNil(t, receivedBody)
	broadcast, ok := receivedBody["broadcast"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "broadcast-123", broadcast["id"])
	assert.Equal(t, "Test Broadcast", broadcast["name"])

	// Verify response
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify _success metadata
	assert.Equal(t, true, result["_success"])
	assert.NotNil(t, result["_fetched_at"])

	// Verify data
	products, ok := result["products"].([]interface{})
	require.True(t, ok)
	assert.Len(t, products, 2)
	assert.Equal(t, "Featured Product", result["featured_item"])
}

func TestDataFeedFetcher_FetchGlobal_CustomHeaders(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

	// Create a test server that captures headers
	var receivedHeaders http.Header

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header

		response := map[string]interface{}{"status": "ok"}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	fetcher := NewDataFeedFetcher(mockLogger)

	settings := &domain.GlobalFeedSettings{
		Enabled: true,
		URL:     server.URL,
		Headers: []domain.DataFeedHeader{
			{Name: "Authorization", Value: "Bearer test-token"},
			{Name: "X-Custom-Header", Value: "custom-value"},
		},
	}

	payload := &domain.GlobalFeedRequestPayload{
		Broadcast: domain.GlobalFeedBroadcast{ID: "b-1", Name: "B"},
		List:      domain.GlobalFeedList{ID: "l-1", Name: "L"},
		Workspace: domain.GlobalFeedWorkspace{ID: "w-1", Name: "W"},
	}

	result, err := fetcher.FetchGlobal(context.Background(), settings, payload)

	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify custom headers were sent
	assert.Equal(t, "Bearer test-token", receivedHeaders.Get("Authorization"))
	assert.Equal(t, "custom-value", receivedHeaders.Get("X-Custom-Header"))
}

func TestDataFeedFetcher_FetchGlobal_Timeout(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	// Create a test server that delays response longer than the hardcoded 5s timeout
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Sleep longer than the 5 second hardcoded timeout
		time.Sleep(7 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	fetcher := NewDataFeedFetcher(mockLogger)

	settings := &domain.GlobalFeedSettings{
		Enabled: true,
		URL:     server.URL,
		Headers: []domain.DataFeedHeader{},
	}

	payload := &domain.GlobalFeedRequestPayload{
		Broadcast: domain.GlobalFeedBroadcast{ID: "b-1", Name: "B"},
		List:      domain.GlobalFeedList{ID: "l-1", Name: "L"},
		Workspace: domain.GlobalFeedWorkspace{ID: "w-1", Name: "W"},
	}

	result, err := fetcher.FetchGlobal(context.Background(), settings, payload)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "timeout")
}

func TestDataFeedFetcher_FetchGlobal_HTTPErrors(t *testing.T) {
	testCases := []struct {
		name       string
		statusCode int
	}{
		{"BadRequest", http.StatusBadRequest},
		{"Unauthorized", http.StatusUnauthorized},
		{"Forbidden", http.StatusForbidden},
		{"NotFound", http.StatusNotFound},
		{"InternalServerError", http.StatusInternalServerError},
		{"BadGateway", http.StatusBadGateway},
		{"ServiceUnavailable", http.StatusServiceUnavailable},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockLogger := pkgmocks.NewMockLogger(ctrl)
			mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
			mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
			mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
			mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.statusCode)
				w.Write([]byte(`{"error": "test error"}`))
			}))
			defer server.Close()

			fetcher := NewDataFeedFetcher(mockLogger)

			settings := &domain.GlobalFeedSettings{
				Enabled: true,
				URL:     server.URL,
				Headers: []domain.DataFeedHeader{},
			}

			payload := &domain.GlobalFeedRequestPayload{
				Broadcast: domain.GlobalFeedBroadcast{ID: "b-1", Name: "B"},
				List:      domain.GlobalFeedList{ID: "l-1", Name: "L"},
				Workspace: domain.GlobalFeedWorkspace{ID: "w-1", Name: "W"},
			}

			result, err := fetcher.FetchGlobal(context.Background(), settings, payload)

			assert.Error(t, err)
			assert.Nil(t, result)
			assert.Contains(t, err.Error(), "HTTP")
		})
	}
}

func TestDataFeedFetcher_FetchGlobal_InvalidJSON(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`invalid json response`))
	}))
	defer server.Close()

	fetcher := NewDataFeedFetcher(mockLogger)

	settings := &domain.GlobalFeedSettings{
		Enabled: true,
		URL:     server.URL,
		Headers: []domain.DataFeedHeader{},
	}

	payload := &domain.GlobalFeedRequestPayload{
		Broadcast: domain.GlobalFeedBroadcast{ID: "b-1", Name: "B"},
		List:      domain.GlobalFeedList{ID: "l-1", Name: "L"},
		Workspace: domain.GlobalFeedWorkspace{ID: "w-1", Name: "W"},
	}

	result, err := fetcher.FetchGlobal(context.Background(), settings, payload)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "JSON")
}

func TestDataFeedFetcher_FetchGlobal_EmptyDataResponse(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Response with empty JSON object
		response := map[string]interface{}{}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	fetcher := NewDataFeedFetcher(mockLogger)

	settings := &domain.GlobalFeedSettings{
		Enabled: true,
		URL:     server.URL,
		Headers: []domain.DataFeedHeader{},
	}

	payload := &domain.GlobalFeedRequestPayload{
		Broadcast: domain.GlobalFeedBroadcast{ID: "b-1", Name: "B"},
		List:      domain.GlobalFeedList{ID: "l-1", Name: "L"},
		Workspace: domain.GlobalFeedWorkspace{ID: "w-1", Name: "W"},
	}

	result, err := fetcher.FetchGlobal(context.Background(), settings, payload)

	require.NoError(t, err)
	require.NotNil(t, result)

	// Should still have metadata
	assert.Equal(t, true, result["_success"])
	assert.NotNil(t, result["_fetched_at"])
}

func TestDataFeedFetcher_FetchGlobal_ContextCancelled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	// Create a test server that delays response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	fetcher := NewDataFeedFetcher(mockLogger)

	settings := &domain.GlobalFeedSettings{
		Enabled: true,
		URL:     server.URL,
		Headers: []domain.DataFeedHeader{},
	}

	payload := &domain.GlobalFeedRequestPayload{
		Broadcast: domain.GlobalFeedBroadcast{ID: "b-1", Name: "B"},
		List:      domain.GlobalFeedList{ID: "l-1", Name: "L"},
		Workspace: domain.GlobalFeedWorkspace{ID: "w-1", Name: "W"},
	}

	// Create a context that gets cancelled quickly
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	result, err := fetcher.FetchGlobal(ctx, settings, payload)

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestDataFeedFetcher_FetchGlobal_NilPayload(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

	var receivedBody map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)

		response := map[string]interface{}{"status": "ok"}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	fetcher := NewDataFeedFetcher(mockLogger)

	settings := &domain.GlobalFeedSettings{
		Enabled: true,
		URL:     server.URL,
		Headers: []domain.DataFeedHeader{},
	}

	// Pass nil payload - should still work with empty payload
	result, err := fetcher.FetchGlobal(context.Background(), settings, nil)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, true, result["_success"])
}

func TestNewDataFeedFetcher(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := pkgmocks.NewMockLogger(ctrl)

	fetcher := NewDataFeedFetcher(mockLogger)

	assert.NotNil(t, fetcher)
	assert.Implements(t, (*DataFeedFetcher)(nil), fetcher)
}

func TestDataFeedFetcher_FetchGlobal_HardcodedTimeout(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{"status": "ok"}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	fetcher := NewDataFeedFetcher(mockLogger)

	// Timeout is now hardcoded to 5 seconds
	settings := &domain.GlobalFeedSettings{
		Enabled: true,
		URL:     server.URL,
		Headers: []domain.DataFeedHeader{},
	}

	// Verify GetTimeout returns hardcoded 5 seconds
	assert.Equal(t, 5, settings.GetTimeout())

	payload := &domain.GlobalFeedRequestPayload{
		Broadcast: domain.GlobalFeedBroadcast{ID: "b-1", Name: "B"},
		List:      domain.GlobalFeedList{ID: "l-1", Name: "L"},
		Workspace: domain.GlobalFeedWorkspace{ID: "w-1", Name: "W"},
	}

	result, err := fetcher.FetchGlobal(context.Background(), settings, payload)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, true, result["_success"])
}

func TestDataFeedFetcher_FetchGlobal_LargeResponse(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

	// Create large data set
	largeData := make([]interface{}, 1000)
	for i := 0; i < 1000; i++ {
		largeData[i] = map[string]interface{}{
			"id":          i,
			"name":        "Product " + string(rune(i)),
			"description": "This is a longer description for product to test larger payloads",
			"price":       99.99,
			"tags":        []string{"tag1", "tag2", "tag3"},
		}
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"products": largeData,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	fetcher := NewDataFeedFetcher(mockLogger)

	settings := &domain.GlobalFeedSettings{
		Enabled: true,
		URL:     server.URL,
		Headers: []domain.DataFeedHeader{},
	}

	payload := &domain.GlobalFeedRequestPayload{
		Broadcast: domain.GlobalFeedBroadcast{ID: "b-1", Name: "B"},
		List:      domain.GlobalFeedList{ID: "l-1", Name: "L"},
		Workspace: domain.GlobalFeedWorkspace{ID: "w-1", Name: "W"},
	}

	result, err := fetcher.FetchGlobal(context.Background(), settings, payload)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, true, result["_success"])

	products, ok := result["products"].([]interface{})
	require.True(t, ok)
	assert.Len(t, products, 1000)
}

// ==================== FetchRecipient Tests ====================

func TestDataFeedFetcher_FetchRecipient_Disabled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()

	fetcher := NewDataFeedFetcher(mockLogger)

	// Test: disabled settings returns nil, nil
	settings := &domain.RecipientFeedSettings{
		Enabled: false,
		URL:     "https://example.com/feed",
	}

	result, err := fetcher.FetchRecipient(context.Background(), settings, nil)

	assert.NoError(t, err)
	assert.Nil(t, result)
}

func TestDataFeedFetcher_FetchRecipient_NilSettings(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()

	fetcher := NewDataFeedFetcher(mockLogger)

	// Test: nil settings returns nil, nil
	result, err := fetcher.FetchRecipient(context.Background(), nil, nil)

	assert.NoError(t, err)
	assert.Nil(t, result)
}

func TestDataFeedFetcher_FetchRecipient_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

	// Create a test server
	var receivedMethod string
	var receivedContentType string
	var receivedUserAgent string
	var receivedBody map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		receivedContentType = r.Header.Get("Content-Type")
		receivedUserAgent = r.Header.Get("User-Agent")

		// Parse request body
		if err := json.NewDecoder(r.Body).Decode(&receivedBody); err != nil {
			t.Fatalf("Failed to parse request body: %v", err)
		}

		// Return successful response
		response := map[string]interface{}{
			"recommendations": []interface{}{
				map[string]interface{}{"id": "1", "name": "Recommended Product 1"},
				map[string]interface{}{"id": "2", "name": "Recommended Product 2"},
			},
			"discount_code": "SAVE10",
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	fetcher := NewDataFeedFetcher(mockLogger)

	settings := &domain.RecipientFeedSettings{
		Enabled: true,
		URL:     server.URL,
		Headers: []domain.DataFeedHeader{},
	}

	payload := &domain.RecipientFeedRequestPayload{
		Contact: domain.RecipientFeedContact{
			Email:     "test@example.com",
			FirstName: "John",
			LastName:  "Doe",
		},
		Broadcast: domain.RecipientFeedBroadcast{
			ID:   "broadcast-123",
			Name: "Test Broadcast",
		},
		List: domain.RecipientFeedList{
			ID:   "list-456",
			Name: "Test List",
		},
		Workspace: domain.RecipientFeedWorkspace{
			ID:   "workspace-789",
			Name: "Test Workspace",
		},
	}

	result, err := fetcher.FetchRecipient(context.Background(), settings, payload)

	// Verify request
	assert.Equal(t, "POST", receivedMethod)
	assert.Equal(t, "application/json", receivedContentType)
	assert.Contains(t, receivedUserAgent, "Notifuse")

	// Verify payload was sent with contact data
	require.NotNil(t, receivedBody)
	contact, ok := receivedBody["contact"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "test@example.com", contact["email"])
	assert.Equal(t, "John", contact["first_name"])
	assert.Equal(t, "Doe", contact["last_name"])

	broadcast, ok := receivedBody["broadcast"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "broadcast-123", broadcast["id"])

	// Verify response
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify _success metadata
	assert.Equal(t, true, result["_success"])
	assert.NotNil(t, result["_fetched_at"])

	// Verify data
	recommendations, ok := result["recommendations"].([]interface{})
	require.True(t, ok)
	assert.Len(t, recommendations, 2)
	assert.Equal(t, "SAVE10", result["discount_code"])
}

func TestDataFeedFetcher_FetchRecipient_Retry(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

	// Track number of requests
	requestCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount <= 2 {
			// First 2 requests fail with 503
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"error": "service unavailable"}`))
			return
		}
		// Third request succeeds
		response := map[string]interface{}{"status": "ok"}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	fetcher := NewDataFeedFetcher(mockLogger)

	settings := &domain.RecipientFeedSettings{
		Enabled: true,
		URL:     server.URL,
		Headers: []domain.DataFeedHeader{},
	}

	payload := &domain.RecipientFeedRequestPayload{
		Contact: domain.RecipientFeedContact{
			Email: "test@example.com",
		},
		Broadcast: domain.RecipientFeedBroadcast{ID: "b-1", Name: "B"},
		List:      domain.RecipientFeedList{ID: "l-1", Name: "L"},
		Workspace: domain.RecipientFeedWorkspace{ID: "w-1", Name: "W"},
	}

	result, err := fetcher.FetchRecipient(context.Background(), settings, payload)

	// Verify retries happened and final result is success
	assert.Equal(t, 3, requestCount)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, true, result["_success"])
}

func TestDataFeedFetcher_FetchRecipient_RetryExhausted(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

	// Track number of requests
	requestCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		// Always return 503
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"error": "service unavailable"}`))
	}))
	defer server.Close()

	fetcher := NewDataFeedFetcher(mockLogger)

	settings := &domain.RecipientFeedSettings{
		Enabled: true,
		URL:     server.URL,
		Headers: []domain.DataFeedHeader{},
	}

	payload := &domain.RecipientFeedRequestPayload{
		Contact: domain.RecipientFeedContact{
			Email: "test@example.com",
		},
		Broadcast: domain.RecipientFeedBroadcast{ID: "b-1", Name: "B"},
		List:      domain.RecipientFeedList{ID: "l-1", Name: "L"},
		Workspace: domain.RecipientFeedWorkspace{ID: "w-1", Name: "W"},
	}

	result, err := fetcher.FetchRecipient(context.Background(), settings, payload)

	// Verify retries happened (1 initial + 2 retries = 3 total)
	assert.Equal(t, 3, requestCount)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "HTTP")
}

func TestDataFeedFetcher_FetchRecipient_NoRetryOn4xx(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

	// Track number of requests
	requestCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		// Return 400 Bad Request
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error": "bad request"}`))
	}))
	defer server.Close()

	fetcher := NewDataFeedFetcher(mockLogger)

	settings := &domain.RecipientFeedSettings{
		Enabled: true,
		URL:     server.URL,
		Headers: []domain.DataFeedHeader{},
	}

	payload := &domain.RecipientFeedRequestPayload{
		Contact: domain.RecipientFeedContact{
			Email: "test@example.com",
		},
		Broadcast: domain.RecipientFeedBroadcast{ID: "b-1", Name: "B"},
		List:      domain.RecipientFeedList{ID: "l-1", Name: "L"},
		Workspace: domain.RecipientFeedWorkspace{ID: "w-1", Name: "W"},
	}

	result, err := fetcher.FetchRecipient(context.Background(), settings, payload)

	// Verify no retries happened (4xx should not retry)
	assert.Equal(t, 1, requestCount)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "HTTP")
}

func TestDataFeedFetcher_FetchRecipient_CustomHeaders(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

	// Create a test server that captures headers
	var receivedHeaders http.Header

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header

		response := map[string]interface{}{"status": "ok"}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	fetcher := NewDataFeedFetcher(mockLogger)

	settings := &domain.RecipientFeedSettings{
		Enabled: true,
		URL:     server.URL,
		Headers: []domain.DataFeedHeader{
			{Name: "Authorization", Value: "Bearer recipient-token"},
			{Name: "X-Recipient-Header", Value: "recipient-value"},
		},
	}

	payload := &domain.RecipientFeedRequestPayload{
		Contact: domain.RecipientFeedContact{
			Email: "test@example.com",
		},
		Broadcast: domain.RecipientFeedBroadcast{ID: "b-1", Name: "B"},
		List:      domain.RecipientFeedList{ID: "l-1", Name: "L"},
		Workspace: domain.RecipientFeedWorkspace{ID: "w-1", Name: "W"},
	}

	result, err := fetcher.FetchRecipient(context.Background(), settings, payload)

	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify custom headers were sent
	assert.Equal(t, "Bearer recipient-token", receivedHeaders.Get("Authorization"))
	assert.Equal(t, "recipient-value", receivedHeaders.Get("X-Recipient-Header"))
}

func TestDataFeedFetcher_FetchRecipient_RetryOn408(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

	// Track number of requests
	requestCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount == 1 {
			// First request fails with 408 Request Timeout
			w.WriteHeader(http.StatusRequestTimeout)
			w.Write([]byte(`{"error": "request timeout"}`))
			return
		}
		// Second request succeeds
		response := map[string]interface{}{"status": "ok"}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	fetcher := NewDataFeedFetcher(mockLogger)

	settings := &domain.RecipientFeedSettings{
		Enabled: true,
		URL:     server.URL,
		Headers: []domain.DataFeedHeader{},
	}

	payload := &domain.RecipientFeedRequestPayload{
		Contact: domain.RecipientFeedContact{
			Email: "test@example.com",
		},
		Broadcast: domain.RecipientFeedBroadcast{ID: "b-1", Name: "B"},
		List:      domain.RecipientFeedList{ID: "l-1", Name: "L"},
		Workspace: domain.RecipientFeedWorkspace{ID: "w-1", Name: "W"},
	}

	result, err := fetcher.FetchRecipient(context.Background(), settings, payload)

	// Verify retries happened for 408
	assert.Equal(t, 2, requestCount)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, true, result["_success"])
}

func TestDataFeedFetcher_FetchRecipient_RetryOn429(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

	// Track number of requests
	requestCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount == 1 {
			// First request fails with 429 Too Many Requests
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error": "too many requests"}`))
			return
		}
		// Second request succeeds
		response := map[string]interface{}{"status": "ok"}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	fetcher := NewDataFeedFetcher(mockLogger)

	settings := &domain.RecipientFeedSettings{
		Enabled: true,
		URL:     server.URL,
		Headers: []domain.DataFeedHeader{},
	}

	payload := &domain.RecipientFeedRequestPayload{
		Contact: domain.RecipientFeedContact{
			Email: "test@example.com",
		},
		Broadcast: domain.RecipientFeedBroadcast{ID: "b-1", Name: "B"},
		List:      domain.RecipientFeedList{ID: "l-1", Name: "L"},
		Workspace: domain.RecipientFeedWorkspace{ID: "w-1", Name: "W"},
	}

	result, err := fetcher.FetchRecipient(context.Background(), settings, payload)

	// Verify retries happened for 429
	assert.Equal(t, 2, requestCount)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, true, result["_success"])
}
