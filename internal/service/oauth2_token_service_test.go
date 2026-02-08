package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOAuth2TokenService_GetAccessToken_CachedToken(t *testing.T) {
	// Test that a valid cached token is returned without making HTTP request
	service := NewOAuth2TokenService(&noopLogger{})

	// Pre-populate cache with a valid token
	// Cache key format: provider:tenantID:clientID (username no longer included)
	cacheKey := "microsoft:tenant-123:client-123"
	service.tokenCache[cacheKey] = &cachedToken{
		accessToken: "cached-token-123",
		expiresAt:   time.Now().Add(10 * time.Minute), // Not expired
	}

	settings := &domain.SMTPSettings{
		AuthType:           "oauth2",
		OAuth2Provider:     "microsoft",
		OAuth2TenantID:     "tenant-123",
		OAuth2ClientID:     "client-123",
		OAuth2ClientSecret: "secret-123",
		// Username is no longer required for OAuth2
	}

	token, err := service.GetAccessToken(settings)
	require.NoError(t, err)
	assert.Equal(t, "cached-token-123", token)
}

func TestOAuth2TokenService_GetAccessToken_ExpiredToken(t *testing.T) {
	// Set up a mock HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request method and content type
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))

		response := map[string]interface{}{
			"access_token": "new-token-123",
			"token_type":   "Bearer",
			"expires_in":   3600,
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	service := NewOAuth2TokenService(&noopLogger{})
	service.microsoftTokenURL = server.URL // Override for testing

	// Pre-populate cache with an expired token
	// Cache key format: provider:tenantID:clientID (username no longer included)
	cacheKey := "microsoft:tenant-123:client-123"
	service.tokenCache[cacheKey] = &cachedToken{
		accessToken: "expired-token",
		expiresAt:   time.Now().Add(-10 * time.Minute), // Already expired
	}

	settings := &domain.SMTPSettings{
		AuthType:           "oauth2",
		OAuth2Provider:     "microsoft",
		OAuth2TenantID:     "tenant-123",
		OAuth2ClientID:     "client-123",
		OAuth2ClientSecret: "secret-123",
		// Username is no longer required for OAuth2
	}

	token, err := service.GetAccessToken(settings)
	require.NoError(t, err)
	assert.Equal(t, "new-token-123", token)
}

func TestOAuth2TokenService_FetchMicrosoftToken(t *testing.T) {
	// Set up a mock HTTP server that mimics Microsoft's OAuth2 endpoint
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the request
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))

		err := r.ParseForm()
		require.NoError(t, err)

		// Check required fields
		assert.Equal(t, "client_credentials", r.FormValue("grant_type"))
		assert.Equal(t, "client-123", r.FormValue("client_id"))
		assert.Equal(t, "secret-123", r.FormValue("client_secret"))
		assert.Equal(t, "https://outlook.office365.com/.default", r.FormValue("scope"))

		response := map[string]interface{}{
			"access_token": "microsoft-token-xyz",
			"token_type":   "Bearer",
			"expires_in":   3600,
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	service := NewOAuth2TokenService(&noopLogger{})
	service.microsoftTokenURL = server.URL

	settings := &domain.SMTPSettings{
		AuthType:           "oauth2",
		OAuth2Provider:     "microsoft",
		OAuth2TenantID:     "tenant-123",
		OAuth2ClientID:     "client-123",
		OAuth2ClientSecret: "secret-123",
		// Username is no longer required for OAuth2
	}

	token, expiresAt, err := service.fetchMicrosoftToken(settings)
	require.NoError(t, err)
	assert.Equal(t, "microsoft-token-xyz", token)
	assert.True(t, expiresAt.After(time.Now().Add(50*time.Minute))) // ~1 hour minus buffer
}

func TestOAuth2TokenService_FetchGoogleToken(t *testing.T) {
	// Set up a mock HTTP server that mimics Google's OAuth2 endpoint
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the request
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))

		err := r.ParseForm()
		require.NoError(t, err)

		// Check required fields
		assert.Equal(t, "refresh_token", r.FormValue("grant_type"))
		assert.Equal(t, "client-123", r.FormValue("client_id"))
		assert.Equal(t, "secret-123", r.FormValue("client_secret"))
		assert.Equal(t, "refresh-token-xyz", r.FormValue("refresh_token"))

		response := map[string]interface{}{
			"access_token": "google-token-xyz",
			"token_type":   "Bearer",
			"expires_in":   3600,
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	service := NewOAuth2TokenService(&noopLogger{})
	service.googleTokenURL = server.URL

	settings := &domain.SMTPSettings{
		AuthType:           "oauth2",
		OAuth2Provider:     "google",
		OAuth2ClientID:     "client-123",
		OAuth2ClientSecret: "secret-123",
		OAuth2RefreshToken: "refresh-token-xyz",
		// Username is no longer required for OAuth2
	}

	token, expiresAt, err := service.fetchGoogleToken(settings)
	require.NoError(t, err)
	assert.Equal(t, "google-token-xyz", token)
	assert.True(t, expiresAt.After(time.Now().Add(50*time.Minute))) // ~1 hour minus buffer
}

func TestOAuth2TokenService_CacheKeyUniqueness(t *testing.T) {
	service := NewOAuth2TokenService(&noopLogger{})

	// Test that cache keys are unique for different configurations
	// Note: Username is no longer part of the cache key since OAuth2 tokens
	// are per-application (service principal), not per-mailbox
	tests := []struct {
		name     string
		settings *domain.SMTPSettings
		expected string
	}{
		{
			name: "Microsoft with tenant-a",
			settings: &domain.SMTPSettings{
				OAuth2Provider: "microsoft",
				OAuth2TenantID: "tenant-a",
				OAuth2ClientID: "client-123",
			},
			expected: "microsoft:tenant-a:client-123",
		},
		{
			name: "Microsoft with same tenant, different client",
			settings: &domain.SMTPSettings{
				OAuth2Provider: "microsoft",
				OAuth2TenantID: "tenant-a",
				OAuth2ClientID: "client-456",
			},
			expected: "microsoft:tenant-a:client-456",
		},
		{
			name: "Microsoft with different tenant",
			settings: &domain.SMTPSettings{
				OAuth2Provider: "microsoft",
				OAuth2TenantID: "tenant-b",
				OAuth2ClientID: "client-123",
			},
			expected: "microsoft:tenant-b:client-123",
		},
		{
			name: "Google",
			settings: &domain.SMTPSettings{
				OAuth2Provider: "google",
				OAuth2ClientID: "client-123",
			},
			expected: "google::client-123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := service.getCacheKey(tt.settings)
			assert.Equal(t, tt.expected, key)
		})
	}

	// Verify all keys are unique
	keys := make(map[string]bool)
	for _, tt := range tests {
		key := service.getCacheKey(tt.settings)
		assert.False(t, keys[key], "Duplicate cache key found: %s", key)
		keys[key] = true
	}
}

func TestOAuth2TokenService_ThreadSafety(t *testing.T) {
	// Set up a mock HTTP server
	requestCount := 0
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestCount++
		count := requestCount
		mu.Unlock()

		// Add small delay to simulate network latency
		time.Sleep(10 * time.Millisecond)

		response := map[string]interface{}{
			"access_token": "token-" + string(rune('0'+count)),
			"token_type":   "Bearer",
			"expires_in":   3600,
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	service := NewOAuth2TokenService(&noopLogger{})
	service.microsoftTokenURL = server.URL

	settings := &domain.SMTPSettings{
		AuthType:           "oauth2",
		OAuth2Provider:     "microsoft",
		OAuth2TenantID:     "tenant-123",
		OAuth2ClientID:     "client-123",
		OAuth2ClientSecret: "secret-123",
		// Username is no longer required for OAuth2
	}

	// Launch multiple goroutines trying to get tokens concurrently
	var wg sync.WaitGroup
	tokens := make([]string, 10)
	errors := make([]error, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			token, err := service.GetAccessToken(settings)
			tokens[idx] = token
			errors[idx] = err
		}(i)
	}

	wg.Wait()

	// All requests should succeed
	for i, err := range errors {
		assert.NoError(t, err, "Request %d failed", i)
	}

	// All tokens should be the same (cached) - only one HTTP request should have been made
	// due to the mutex protection in GetAccessToken
	firstToken := tokens[0]
	for i, token := range tokens {
		assert.Equal(t, firstToken, token, "Token %d differs from first token", i)
	}
}

func TestOAuth2TokenService_TokenRefreshBuffer(t *testing.T) {
	// Test that tokens are refreshed 5 minutes before expiry
	service := NewOAuth2TokenService(&noopLogger{})

	// Cache key format: provider:tenantID:clientID (username no longer included)
	cacheKey := "microsoft:tenant-123:client-123"

	// Token expires in 4 minutes (less than 5-minute buffer)
	service.tokenCache[cacheKey] = &cachedToken{
		accessToken: "soon-expiring-token",
		expiresAt:   time.Now().Add(4 * time.Minute),
	}

	settings := &domain.SMTPSettings{
		AuthType:           "oauth2",
		OAuth2Provider:     "microsoft",
		OAuth2TenantID:     "tenant-123",
		OAuth2ClientID:     "client-123",
		OAuth2ClientSecret: "secret-123",
		// Username is no longer required for OAuth2
	}

	// Token should be considered invalid (within buffer)
	valid, _ := service.getCachedToken(settings)
	assert.False(t, valid, "Token within 5-minute buffer should be considered invalid")

	// Token expires in 10 minutes (more than 5-minute buffer)
	service.tokenCache[cacheKey] = &cachedToken{
		accessToken: "valid-token",
		expiresAt:   time.Now().Add(10 * time.Minute),
	}

	valid, token := service.getCachedToken(settings)
	assert.True(t, valid, "Token with 10 minutes remaining should be valid")
	assert.Equal(t, "valid-token", token)
}

func TestOAuth2TokenService_InvalidProvider(t *testing.T) {
	service := NewOAuth2TokenService(&noopLogger{})

	settings := &domain.SMTPSettings{
		AuthType:           "oauth2",
		OAuth2Provider:     "invalid-provider",
		OAuth2ClientID:     "client-123",
		OAuth2ClientSecret: "secret-123",
		// Username is no longer required for OAuth2
	}

	_, err := service.GetAccessToken(settings)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported OAuth2 provider")
}

func TestOAuth2TokenService_HTTPError(t *testing.T) {
	// Set up a mock HTTP server that returns an error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{
			"error":             "invalid_client",
			"error_description": "Client authentication failed",
		})
	}))
	defer server.Close()

	service := NewOAuth2TokenService(&noopLogger{})
	service.microsoftTokenURL = server.URL

	settings := &domain.SMTPSettings{
		AuthType:           "oauth2",
		OAuth2Provider:     "microsoft",
		OAuth2TenantID:     "tenant-123",
		OAuth2ClientID:     "client-123",
		OAuth2ClientSecret: "wrong-secret",
		// Username is no longer required for OAuth2
	}

	_, err := service.GetAccessToken(settings)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "401")
}

func TestOAuth2TokenService_InvalidJSONResponse(t *testing.T) {
	// Set up a mock HTTP server that returns invalid JSON
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not-valid-json"))
	}))
	defer server.Close()

	service := NewOAuth2TokenService(&noopLogger{})
	service.microsoftTokenURL = server.URL

	settings := &domain.SMTPSettings{
		AuthType:           "oauth2",
		OAuth2Provider:     "microsoft",
		OAuth2TenantID:     "tenant-123",
		OAuth2ClientID:     "client-123",
		OAuth2ClientSecret: "secret-123",
		// Username is no longer required for OAuth2
	}

	_, err := service.GetAccessToken(settings)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decode")
}

func TestNewOAuth2TokenService(t *testing.T) {
	logger := &noopLogger{}
	service := NewOAuth2TokenService(logger)

	require.NotNil(t, service)
	assert.NotNil(t, service.httpClient)
	assert.NotNil(t, service.tokenCache)
	assert.Equal(t, logger, service.logger)
}
