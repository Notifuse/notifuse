package service

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// tokenRefreshBuffer is the time before token expiry when we should refresh
const tokenRefreshBuffer = 5 * time.Minute

// cachedToken holds a cached OAuth2 access token with its expiration time
type cachedToken struct {
	accessToken string
	expiresAt   time.Time
}

// OAuth2TokenService handles OAuth2 token fetching and caching for SMTP authentication
type OAuth2TokenService struct {
	httpClient *http.Client
	logger     logger.Logger
	mu         sync.Mutex
	tokenCache map[string]*cachedToken

	// Token URLs - can be overridden for testing
	microsoftTokenURL string
	googleTokenURL    string
}

// NewOAuth2TokenService creates a new instance of OAuth2TokenService
func NewOAuth2TokenService(logger logger.Logger) *OAuth2TokenService {
	return &OAuth2TokenService{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger:            logger,
		tokenCache:        make(map[string]*cachedToken),
		microsoftTokenURL: "", // Will be constructed dynamically with tenant ID
		googleTokenURL:    "https://oauth2.googleapis.com/token",
	}
}

// GetAccessToken returns a valid OAuth2 access token for the given SMTP settings.
// It first checks the cache and returns a cached token if valid.
// Otherwise, it fetches a new token from the OAuth2 provider.
func (s *OAuth2TokenService) GetAccessToken(settings *domain.SMTPSettings) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check cache first
	if valid, token := s.getCachedToken(settings); valid {
		return token, nil
	}

	// Fetch new token based on provider
	var accessToken string
	var expiresAt time.Time
	var err error

	switch settings.OAuth2Provider {
	case "microsoft":
		accessToken, expiresAt, err = s.fetchMicrosoftToken(settings)
	case "google":
		accessToken, expiresAt, err = s.fetchGoogleToken(settings)
	default:
		return "", fmt.Errorf("unsupported OAuth2 provider: %s", settings.OAuth2Provider)
	}

	if err != nil {
		return "", err
	}

	// Cache the token
	cacheKey := s.getCacheKey(settings)
	s.tokenCache[cacheKey] = &cachedToken{
		accessToken: accessToken,
		expiresAt:   expiresAt,
	}

	s.logger.WithFields(map[string]interface{}{
		"provider":   settings.OAuth2Provider,
		"username":   settings.Username,
		"expires_at": expiresAt.Format(time.RFC3339),
	}).Info("OAuth2 token fetched and cached")

	return accessToken, nil
}

// getCacheKey generates a unique cache key for the given SMTP settings
// Format: provider:tenantID:clientID
// Note: Username is not included because OAuth2 tokens are per-application, not per-mailbox
func (s *OAuth2TokenService) getCacheKey(settings *domain.SMTPSettings) string {
	return fmt.Sprintf("%s:%s:%s",
		settings.OAuth2Provider,
		settings.OAuth2TenantID,
		settings.OAuth2ClientID,
	)
}

// getCachedToken checks if there's a valid cached token for the given settings
// Returns (true, token) if valid, (false, "") otherwise
func (s *OAuth2TokenService) getCachedToken(settings *domain.SMTPSettings) (bool, string) {
	cacheKey := s.getCacheKey(settings)
	cached, exists := s.tokenCache[cacheKey]

	if !exists {
		return false, ""
	}

	// Check if token is still valid (with buffer)
	if time.Now().Add(tokenRefreshBuffer).Before(cached.expiresAt) {
		return true, cached.accessToken
	}

	return false, ""
}

// fetchMicrosoftToken fetches an access token from Microsoft Azure AD using client credentials flow
func (s *OAuth2TokenService) fetchMicrosoftToken(settings *domain.SMTPSettings) (string, time.Time, error) {
	// Construct token URL with tenant ID
	tokenURL := s.microsoftTokenURL
	if tokenURL == "" {
		tokenURL = fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", settings.OAuth2TenantID)
	}

	// Prepare form data
	data := url.Values{}
	data.Set("grant_type", "client_credentials")
	data.Set("client_id", settings.OAuth2ClientID)
	data.Set("client_secret", settings.OAuth2ClientSecret)
	data.Set("scope", "https://outlook.office365.com/.default")

	return s.fetchToken(tokenURL, data)
}

// fetchGoogleToken fetches an access token from Google using refresh token flow
func (s *OAuth2TokenService) fetchGoogleToken(settings *domain.SMTPSettings) (string, time.Time, error) {
	tokenURL := s.googleTokenURL

	// Prepare form data
	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("client_id", settings.OAuth2ClientID)
	data.Set("client_secret", settings.OAuth2ClientSecret)
	data.Set("refresh_token", settings.OAuth2RefreshToken)

	return s.fetchToken(tokenURL, data)
}

// fetchToken makes the HTTP request to fetch an OAuth2 token
func (s *OAuth2TokenService) fetchToken(tokenURL string, data url.Values) (string, time.Time, error) {
	req, err := http.NewRequest(http.MethodPost, tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to create token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to fetch OAuth2 token: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to read token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", time.Time{}, fmt.Errorf("OAuth2 token request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp tokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", time.Time{}, fmt.Errorf("failed to decode token response: %w", err)
	}

	// Calculate expiration time
	expiresAt := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

	return tokenResp.AccessToken, expiresAt, nil
}

// tokenResponse represents the OAuth2 token response from providers
type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// InvalidateCache removes all cached tokens (useful for testing or force refresh)
func (s *OAuth2TokenService) InvalidateCache() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokenCache = make(map[string]*cachedToken)
}

// InvalidateCacheForSettings removes the cached token for specific settings
func (s *OAuth2TokenService) InvalidateCacheForSettings(settings *domain.SMTPSettings) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cacheKey := s.getCacheKey(settings)
	delete(s.tokenCache, cacheKey)
}

// SetTokenURLs overrides the OAuth2 token endpoints (for testing)
func (s *OAuth2TokenService) SetTokenURLs(microsoft, google string) {
	s.microsoftTokenURL = microsoft
	s.googleTokenURL = google
}
