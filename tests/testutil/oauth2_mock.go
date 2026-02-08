package testutil

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"
)

// MockOAuth2Server provides mock OAuth2 token endpoints for Microsoft and Google
// Validates grant_type, client_id, client_secret, scope (Microsoft) / refresh_token (Google)
type MockOAuth2Server struct {
	Server          *httptest.Server
	MicrosoftTokens map[string]MockTokenResponse // key: clientID
	GoogleTokens    map[string]MockTokenResponse // key: refreshToken
	RequestLog      []TokenRequest
	mu              sync.Mutex
}

// MockTokenResponse defines the response for a token request
type MockTokenResponse struct {
	AccessToken string
	ExpiresIn   int    // seconds (default 3600)
	Error       string // if set, return OAuth2 error response
	ErrorDesc   string // error_description field
}

// TokenRequest logs details of a token request
type TokenRequest struct {
	Provider     string // "microsoft" or "google"
	GrantType    string // "client_credentials" or "refresh_token"
	ClientID     string
	ClientSecret string
	Scope        string // Microsoft: "https://outlook.office365.com/.default"
	RefreshToken string // Google only
	TenantID     string // Microsoft only (extracted from URL path)
	Timestamp    time.Time
}

// NewMockOAuth2Server creates a new mock OAuth2 token server
func NewMockOAuth2Server() *MockOAuth2Server {
	s := &MockOAuth2Server{
		MicrosoftTokens: make(map[string]MockTokenResponse),
		GoogleTokens:    make(map[string]MockTokenResponse),
		RequestLog:      make([]TokenRequest, 0),
	}

	mux := http.NewServeMux()

	// Microsoft token endpoint: /{tenant}/oauth2/v2.0/token
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Parse path to determine if this is Microsoft or Google
		path := r.URL.Path

		// Check for Microsoft pattern: /{tenant}/oauth2/v2.0/token
		// /oauth2/v2.0/token is 18 characters
		if len(path) > 18 && strings.HasSuffix(path, "/oauth2/v2.0/token") {
			s.handleMicrosoftToken(w, r, path)
			return
		}

		// Check for Google pattern: /token
		if path == "/token" {
			s.handleGoogleToken(w, r)
			return
		}

		http.Error(w, "Not found", http.StatusNotFound)
	})

	s.Server = httptest.NewServer(mux)
	return s
}

// handleMicrosoftToken handles Microsoft OAuth2 token requests
func (s *MockOAuth2Server) handleMicrosoftToken(w http.ResponseWriter, r *http.Request, path string) {
	if err := r.ParseForm(); err != nil {
		s.writeOAuth2Error(w, "invalid_request", "Failed to parse form data")
		return
	}

	// Extract tenant ID from path
	// Path format: /{tenant}/oauth2/v2.0/token
	// The suffix /oauth2/v2.0/token is 18 characters
	tenantID := path[1 : len(path)-18]

	grantType := r.FormValue("grant_type")
	clientID := r.FormValue("client_id")
	clientSecret := r.FormValue("client_secret")
	scope := r.FormValue("scope")

	// Log the request
	s.mu.Lock()
	s.RequestLog = append(s.RequestLog, TokenRequest{
		Provider:     "microsoft",
		GrantType:    grantType,
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Scope:        scope,
		TenantID:     tenantID,
		Timestamp:    time.Now(),
	})
	s.mu.Unlock()

	// Validate grant type
	if grantType != "client_credentials" {
		s.writeOAuth2Error(w, "unsupported_grant_type", "Grant type must be client_credentials")
		return
	}

	// Validate scope
	if scope != "https://outlook.office365.com/.default" {
		s.writeOAuth2Error(w, "invalid_scope", "Scope must be https://outlook.office365.com/.default")
		return
	}

	// Look up token response
	s.mu.Lock()
	tokenResp, exists := s.MicrosoftTokens[clientID]
	s.mu.Unlock()

	if !exists {
		s.writeOAuth2Error(w, "invalid_client", "Client ID not found")
		return
	}

	// Check for configured error
	if tokenResp.Error != "" {
		s.writeOAuth2Error(w, tokenResp.Error, tokenResp.ErrorDesc)
		return
	}

	// Return success response
	expiresIn := tokenResp.ExpiresIn
	if expiresIn == 0 {
		expiresIn = 3600
	}

	s.writeTokenResponse(w, tokenResp.AccessToken, expiresIn)
}

// handleGoogleToken handles Google OAuth2 token requests
func (s *MockOAuth2Server) handleGoogleToken(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.writeOAuth2Error(w, "invalid_request", "Failed to parse form data")
		return
	}

	grantType := r.FormValue("grant_type")
	clientID := r.FormValue("client_id")
	clientSecret := r.FormValue("client_secret")
	refreshToken := r.FormValue("refresh_token")

	// Log the request
	s.mu.Lock()
	s.RequestLog = append(s.RequestLog, TokenRequest{
		Provider:     "google",
		GrantType:    grantType,
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RefreshToken: refreshToken,
		Timestamp:    time.Now(),
	})
	s.mu.Unlock()

	// Validate grant type
	if grantType != "refresh_token" {
		s.writeOAuth2Error(w, "unsupported_grant_type", "Grant type must be refresh_token")
		return
	}

	// Validate refresh token present
	if refreshToken == "" {
		s.writeOAuth2Error(w, "invalid_request", "refresh_token is required")
		return
	}

	// Look up token response
	s.mu.Lock()
	tokenResp, exists := s.GoogleTokens[refreshToken]
	s.mu.Unlock()

	if !exists {
		s.writeOAuth2Error(w, "invalid_grant", "Refresh token not found")
		return
	}

	// Check for configured error
	if tokenResp.Error != "" {
		s.writeOAuth2Error(w, tokenResp.Error, tokenResp.ErrorDesc)
		return
	}

	// Return success response
	expiresIn := tokenResp.ExpiresIn
	if expiresIn == 0 {
		expiresIn = 3600
	}

	s.writeTokenResponse(w, tokenResp.AccessToken, expiresIn)
}

// writeOAuth2Error writes an OAuth2 error response
func (s *MockOAuth2Server) writeOAuth2Error(w http.ResponseWriter, errorCode, errorDesc string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(map[string]string{
		"error":             errorCode,
		"error_description": errorDesc,
	})
}

// writeTokenResponse writes a successful token response
func (s *MockOAuth2Server) writeTokenResponse(w http.ResponseWriter, accessToken string, expiresIn int) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"access_token": accessToken,
		"token_type":   "Bearer",
		"expires_in":   expiresIn,
	})
}

// MicrosoftURL returns the Microsoft token endpoint URL for a given tenant
func (s *MockOAuth2Server) MicrosoftURL(tenantID string) string {
	return fmt.Sprintf("%s/%s/oauth2/v2.0/token", s.Server.URL, tenantID)
}

// GoogleURL returns the Google token endpoint URL
func (s *MockOAuth2Server) GoogleURL() string {
	return s.Server.URL + "/token"
}

// Close shuts down the mock server
func (s *MockOAuth2Server) Close() {
	s.Server.Close()
}

// SetMicrosoftToken configures a token response for a Microsoft client ID
func (s *MockOAuth2Server) SetMicrosoftToken(clientID string, response MockTokenResponse) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.MicrosoftTokens[clientID] = response
}

// SetGoogleToken configures a token response for a Google refresh token
func (s *MockOAuth2Server) SetGoogleToken(refreshToken string, response MockTokenResponse) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.GoogleTokens[refreshToken] = response
}

// GetRequestCount returns the total number of token requests received
func (s *MockOAuth2Server) GetRequestCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.RequestLog)
}

// GetRequests returns a copy of all token requests received
func (s *MockOAuth2Server) GetRequests() []TokenRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]TokenRequest, len(s.RequestLog))
	copy(result, s.RequestLog)
	return result
}

// ClearRequests clears the request log
func (s *MockOAuth2Server) ClearRequests() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.RequestLog = make([]TokenRequest, 0)
}
