package integration

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOAuth2SMTP_Microsoft_EndToEnd tests the complete Microsoft OAuth2 SMTP flow
// This verifies:
// 1. Token is fetched from Microsoft token endpoint
// 2. XOAUTH2 authentication string is correctly formatted
// 3. Email is sent successfully through SMTP
func TestOAuth2SMTP_Microsoft_EndToEnd(t *testing.T) {
	// Setup mock OAuth2 token server
	oauth2Server := testutil.NewMockOAuth2Server()
	defer oauth2Server.Close()

	// Configure Microsoft token response
	testAccessToken := "microsoft-access-token-12345"
	testUsername := "user@company.onmicrosoft.com"
	testClientID := "client-abc-123"
	testTenantID := "tenant-xyz-456"

	oauth2Server.SetMicrosoftToken(testClientID, testutil.MockTokenResponse{
		AccessToken: testAccessToken,
		ExpiresIn:   3600,
	})

	// Setup mock SMTP server with XOAUTH2
	smtpServer := testutil.NewMockOAuth2SMTPServer(map[string]string{
		testAccessToken: testUsername,
	})
	defer smtpServer.Close()

	// Create SMTP service with custom token URLs
	log := logger.NewLogger()
	tokenService := service.NewOAuth2TokenService(log)
	tokenService.SetTokenURLs(
		oauth2Server.MicrosoftURL(testTenantID),
		oauth2Server.GoogleURL(),
	)
	smtpService := service.NewSMTPServiceWithOAuth2(log, tokenService)

	// Create test email request with OAuth2 settings
	request := domain.SendEmailProviderRequest{
		WorkspaceID:   "test-workspace",
		IntegrationID: "test-integration",
		MessageID:     "test-oauth2-msg-001",
		FromAddress:   testUsername,
		FromName:      "Test Sender",
		To:            "recipient@example.com",
		Subject:       "OAuth2 Test Email",
		Content:       "<html><body><p>Test email via OAuth2 SMTP</p></body></html>",
		Provider: &domain.EmailProvider{
			Kind: domain.EmailProviderKindSMTP,
			SMTP: &domain.SMTPSettings{
				Host:               smtpServer.Host(),
				Port:               smtpServer.Port(),
				Username:           testUsername,
				AuthType:           "oauth2",
				OAuth2Provider:     "microsoft",
				OAuth2TenantID:     testTenantID,
				OAuth2ClientID:     testClientID,
				OAuth2ClientSecret: "client-secret-xyz",
			},
		},
	}

	// Send email
	err := smtpService.SendEmail(context.Background(), request)
	require.NoError(t, err)

	// Give server time to process
	time.Sleep(100 * time.Millisecond)

	// Verify OAuth2 token was requested from Microsoft endpoint
	tokenRequests := oauth2Server.GetRequests()
	require.Len(t, tokenRequests, 1, "Should have made exactly one token request")

	tokenReq := tokenRequests[0]
	assert.Equal(t, "microsoft", tokenReq.Provider)
	assert.Equal(t, "client_credentials", tokenReq.GrantType)
	assert.Equal(t, testClientID, tokenReq.ClientID)
	assert.Equal(t, "https://outlook.office365.com/.default", tokenReq.Scope)

	// Verify XOAUTH2 authentication was used
	authAttempts := smtpServer.GetAuthAttempts()
	require.Len(t, authAttempts, 1, "Should have made exactly one auth attempt")

	authAttempt := authAttempts[0]
	assert.True(t, authAttempt.Success, "Authentication should have succeeded")
	assert.Equal(t, testUsername, authAttempt.Username)
	assert.Equal(t, testAccessToken, authAttempt.Token)

	// Verify XOAUTH2 format: user={email}\x01auth=Bearer {token}\x01\x01
	assert.Contains(t, authAttempt.RawXOAuth2, "user="+testUsername)
	assert.Contains(t, authAttempt.RawXOAuth2, "auth=Bearer "+testAccessToken)

	// Verify email was received
	messages := smtpServer.GetMessages()
	require.Len(t, messages, 1, "Should have received exactly one message")
	assert.Contains(t, string(messages[0].Data), "OAuth2 Test Email")
}

// TestOAuth2SMTP_Google_EndToEnd tests the complete Google OAuth2 SMTP flow
// This verifies:
// 1. Token is fetched from Google token endpoint using refresh_token grant
// 2. XOAUTH2 authentication string is correctly formatted
// 3. Email is sent successfully through SMTP
func TestOAuth2SMTP_Google_EndToEnd(t *testing.T) {
	// Setup mock OAuth2 token server
	oauth2Server := testutil.NewMockOAuth2Server()
	defer oauth2Server.Close()

	// Configure Google token response
	testAccessToken := "google-access-token-67890"
	testUsername := "user@gmail.com"
	testClientID := "google-client-id.apps.googleusercontent.com"
	testRefreshToken := "google-refresh-token-abc"

	oauth2Server.SetGoogleToken(testRefreshToken, testutil.MockTokenResponse{
		AccessToken: testAccessToken,
		ExpiresIn:   3600,
	})

	// Setup mock SMTP server with XOAUTH2
	smtpServer := testutil.NewMockOAuth2SMTPServer(map[string]string{
		testAccessToken: testUsername,
	})
	defer smtpServer.Close()

	// Create SMTP service with custom token URLs
	log := logger.NewLogger()
	tokenService := service.NewOAuth2TokenService(log)
	tokenService.SetTokenURLs("", oauth2Server.GoogleURL())
	smtpService := service.NewSMTPServiceWithOAuth2(log, tokenService)

	// Create test email request with OAuth2 settings
	request := domain.SendEmailProviderRequest{
		WorkspaceID:   "test-workspace",
		IntegrationID: "test-integration",
		MessageID:     "test-oauth2-msg-002",
		FromAddress:   testUsername,
		FromName:      "Google Test Sender",
		To:            "recipient@example.com",
		Subject:       "Google OAuth2 Test Email",
		Content:       "<html><body><p>Test email via Google OAuth2 SMTP</p></body></html>",
		Provider: &domain.EmailProvider{
			Kind: domain.EmailProviderKindSMTP,
			SMTP: &domain.SMTPSettings{
				Host:               smtpServer.Host(),
				Port:               smtpServer.Port(),
				Username:           testUsername,
				AuthType:           "oauth2",
				OAuth2Provider:     "google",
				OAuth2ClientID:     testClientID,
				OAuth2ClientSecret: "google-client-secret",
				OAuth2RefreshToken: testRefreshToken,
			},
		},
	}

	// Send email
	err := smtpService.SendEmail(context.Background(), request)
	require.NoError(t, err)

	// Give server time to process
	time.Sleep(100 * time.Millisecond)

	// Verify OAuth2 token was requested from Google endpoint
	tokenRequests := oauth2Server.GetRequests()
	require.Len(t, tokenRequests, 1, "Should have made exactly one token request")

	tokenReq := tokenRequests[0]
	assert.Equal(t, "google", tokenReq.Provider)
	assert.Equal(t, "refresh_token", tokenReq.GrantType)
	assert.Equal(t, testClientID, tokenReq.ClientID)
	assert.Equal(t, testRefreshToken, tokenReq.RefreshToken)

	// Verify XOAUTH2 authentication was used
	authAttempts := smtpServer.GetAuthAttempts()
	require.Len(t, authAttempts, 1, "Should have made exactly one auth attempt")

	authAttempt := authAttempts[0]
	assert.True(t, authAttempt.Success, "Authentication should have succeeded")
	assert.Equal(t, testUsername, authAttempt.Username)
	assert.Equal(t, testAccessToken, authAttempt.Token)

	// Verify email was received
	messages := smtpServer.GetMessages()
	require.Len(t, messages, 1, "Should have received exactly one message")
}

// TestOAuth2SMTP_TokenCaching verifies that tokens are cached and reused
func TestOAuth2SMTP_TokenCaching(t *testing.T) {
	// Setup mock OAuth2 token server
	oauth2Server := testutil.NewMockOAuth2Server()
	defer oauth2Server.Close()

	// Configure token response
	testAccessToken := "cached-access-token-xyz"
	testUsername := "user@company.onmicrosoft.com"
	testClientID := "client-for-caching"
	testTenantID := "tenant-for-caching"

	oauth2Server.SetMicrosoftToken(testClientID, testutil.MockTokenResponse{
		AccessToken: testAccessToken,
		ExpiresIn:   3600, // 1 hour - plenty of time for caching
	})

	// Setup mock SMTP server
	smtpServer := testutil.NewMockOAuth2SMTPServer(map[string]string{
		testAccessToken: testUsername,
	})
	defer smtpServer.Close()

	// Create SMTP service with custom token URLs
	log := logger.NewLogger()
	tokenService := service.NewOAuth2TokenService(log)
	tokenService.SetTokenURLs(
		oauth2Server.MicrosoftURL(testTenantID),
		oauth2Server.GoogleURL(),
	)
	smtpService := service.NewSMTPServiceWithOAuth2(log, tokenService)

	// Create base request
	createRequest := func(msgID string) domain.SendEmailProviderRequest {
		return domain.SendEmailProviderRequest{
			WorkspaceID:   "test-workspace",
			IntegrationID: "test-integration",
			MessageID:     msgID,
			FromAddress:   testUsername,
			FromName:      "Test Sender",
			To:            "recipient@example.com",
			Subject:       "Token Caching Test",
			Content:       "<html><body><p>Test email</p></body></html>",
			Provider: &domain.EmailProvider{
				Kind: domain.EmailProviderKindSMTP,
				SMTP: &domain.SMTPSettings{
					Host:               smtpServer.Host(),
					Port:               smtpServer.Port(),
					Username:           testUsername,
					AuthType:           "oauth2",
					OAuth2Provider:     "microsoft",
					OAuth2TenantID:     testTenantID,
					OAuth2ClientID:     testClientID,
					OAuth2ClientSecret: "client-secret",
				},
			},
		}
	}

	// Send first email
	err := smtpService.SendEmail(context.Background(), createRequest("msg-1"))
	require.NoError(t, err)

	// Send second email
	err = smtpService.SendEmail(context.Background(), createRequest("msg-2"))
	require.NoError(t, err)

	// Send third email
	err = smtpService.SendEmail(context.Background(), createRequest("msg-3"))
	require.NoError(t, err)

	// Give server time to process
	time.Sleep(100 * time.Millisecond)

	// KEY ASSERTION: Token should only be fetched ONCE due to caching
	tokenRequests := oauth2Server.GetRequests()
	assert.Len(t, tokenRequests, 1, "Token should only be fetched once (cached for subsequent requests)")

	// Verify all three emails were sent
	messages := smtpServer.GetMessages()
	assert.Len(t, messages, 3, "All three emails should have been sent")

	// Verify all three SMTP auths succeeded
	authAttempts := smtpServer.GetAuthAttempts()
	assert.Len(t, authAttempts, 3, "Should have three auth attempts")
	for i, attempt := range authAttempts {
		assert.True(t, attempt.Success, "Auth attempt %d should have succeeded", i+1)
	}
}

// TestOAuth2SMTP_TokenExpiry_Retry verifies retry logic when token expires mid-session
func TestOAuth2SMTP_TokenExpiry_Retry(t *testing.T) {
	// Setup mock OAuth2 token server
	oauth2Server := testutil.NewMockOAuth2Server()
	defer oauth2Server.Close()

	// Configure token responses - first token and retry token
	firstToken := "first-access-token"
	retryToken := "retry-access-token"
	testUsername := "user@company.onmicrosoft.com"
	testClientID := "client-for-retry"
	testTenantID := "tenant-for-retry"

	// Initially set up the first token
	oauth2Server.SetMicrosoftToken(testClientID, testutil.MockTokenResponse{
		AccessToken: firstToken,
		ExpiresIn:   3600,
	})

	// Setup mock SMTP server that fails first auth, succeeds on second
	smtpServer := testutil.NewMockOAuth2SMTPServer(map[string]string{
		firstToken: testUsername,
		retryToken: testUsername,
	})
	defer smtpServer.Close()
	smtpServer.SetFailFirstAuth(true)

	// Create SMTP service with custom token URLs
	log := logger.NewLogger()
	tokenService := service.NewOAuth2TokenService(log)
	tokenService.SetTokenURLs(
		oauth2Server.MicrosoftURL(testTenantID),
		oauth2Server.GoogleURL(),
	)
	smtpService := service.NewSMTPServiceWithOAuth2(log, tokenService)

	// Update token for retry
	oauth2Server.SetMicrosoftToken(testClientID, testutil.MockTokenResponse{
		AccessToken: retryToken,
		ExpiresIn:   3600,
	})

	// Create test request
	request := domain.SendEmailProviderRequest{
		WorkspaceID:   "test-workspace",
		IntegrationID: "test-integration",
		MessageID:     "test-retry-msg",
		FromAddress:   testUsername,
		FromName:      "Test Sender",
		To:            "recipient@example.com",
		Subject:       "Retry Test Email",
		Content:       "<html><body><p>Test email with retry</p></body></html>",
		Provider: &domain.EmailProvider{
			Kind: domain.EmailProviderKindSMTP,
			SMTP: &domain.SMTPSettings{
				Host:               smtpServer.Host(),
				Port:               smtpServer.Port(),
				Username:           testUsername,
				AuthType:           "oauth2",
				OAuth2Provider:     "microsoft",
				OAuth2TenantID:     testTenantID,
				OAuth2ClientID:     testClientID,
				OAuth2ClientSecret: "client-secret",
			},
		},
	}

	// Send email - should fail first auth but retry with fresh token
	err := smtpService.SendEmail(context.Background(), request)
	require.NoError(t, err, "Email should succeed after retry")

	// Give server time to process
	time.Sleep(100 * time.Millisecond)

	// Verify token was fetched twice (initial + retry after invalidation)
	tokenRequests := oauth2Server.GetRequests()
	assert.GreaterOrEqual(t, len(tokenRequests), 2, "Token should be fetched at least twice (initial + retry)")

	// Verify auth attempts (first failed, second succeeded)
	authAttempts := smtpServer.GetAuthAttempts()
	require.GreaterOrEqual(t, len(authAttempts), 2, "Should have at least two auth attempts")
	assert.False(t, authAttempts[0].Success, "First auth should have failed")
	assert.True(t, authAttempts[1].Success, "Second auth should have succeeded")
}

// TestOAuth2SMTP_InvalidCredentials verifies proper error handling for invalid OAuth2 credentials
func TestOAuth2SMTP_InvalidCredentials(t *testing.T) {
	// Setup mock OAuth2 token server that returns an error
	oauth2Server := testutil.NewMockOAuth2Server()
	defer oauth2Server.Close()

	// Configure error response for invalid client
	testClientID := "invalid-client-id"
	testTenantID := "tenant-xyz"

	oauth2Server.SetMicrosoftToken(testClientID, testutil.MockTokenResponse{
		Error:     "invalid_client",
		ErrorDesc: "Client authentication failed",
	})

	// Setup mock SMTP server (shouldn't be reached)
	smtpServer := testutil.NewMockOAuth2SMTPServer(map[string]string{})
	defer smtpServer.Close()

	// Create SMTP service with custom token URLs
	log := logger.NewLogger()
	tokenService := service.NewOAuth2TokenService(log)
	tokenService.SetTokenURLs(
		oauth2Server.MicrosoftURL(testTenantID),
		oauth2Server.GoogleURL(),
	)
	smtpService := service.NewSMTPServiceWithOAuth2(log, tokenService)

	// Create test request with invalid credentials
	request := domain.SendEmailProviderRequest{
		WorkspaceID:   "test-workspace",
		IntegrationID: "test-integration",
		MessageID:     "test-invalid-msg",
		FromAddress:   "user@company.com",
		FromName:      "Test Sender",
		To:            "recipient@example.com",
		Subject:       "Should Fail",
		Content:       "<html><body><p>This should not be sent</p></body></html>",
		Provider: &domain.EmailProvider{
			Kind: domain.EmailProviderKindSMTP,
			SMTP: &domain.SMTPSettings{
				Host:               smtpServer.Host(),
				Port:               smtpServer.Port(),
				Username:           "user@company.com",
				AuthType:           "oauth2",
				OAuth2Provider:     "microsoft",
				OAuth2TenantID:     testTenantID,
				OAuth2ClientID:     testClientID,
				OAuth2ClientSecret: "invalid-secret",
			},
		},
	}

	// Send email - should fail with OAuth2 error
	err := smtpService.SendEmail(context.Background(), request)
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "invalid_client",
		"Error should mention invalid_client")

	// Verify no emails were sent
	messages := smtpServer.GetMessages()
	assert.Len(t, messages, 0, "No emails should have been sent")

	// Verify no SMTP auth was attempted
	authAttempts := smtpServer.GetAuthAttempts()
	assert.Len(t, authAttempts, 0, "No SMTP auth should have been attempted")
}

// TestOAuth2SMTP_TokenEndpointFailure verifies handling of OAuth2 server errors
func TestOAuth2SMTP_TokenEndpointFailure(t *testing.T) {
	// Setup mock OAuth2 token server
	oauth2Server := testutil.NewMockOAuth2Server()
	defer oauth2Server.Close()

	// Configure server error response
	testClientID := "client-for-error"
	testTenantID := "tenant-xyz"

	oauth2Server.SetMicrosoftToken(testClientID, testutil.MockTokenResponse{
		Error:     "server_error",
		ErrorDesc: "Internal server error",
	})

	// Setup mock SMTP server (shouldn't be reached)
	smtpServer := testutil.NewMockOAuth2SMTPServer(map[string]string{})
	defer smtpServer.Close()

	// Create SMTP service
	log := logger.NewLogger()
	tokenService := service.NewOAuth2TokenService(log)
	tokenService.SetTokenURLs(
		oauth2Server.MicrosoftURL(testTenantID),
		oauth2Server.GoogleURL(),
	)
	smtpService := service.NewSMTPServiceWithOAuth2(log, tokenService)

	// Create test request
	request := domain.SendEmailProviderRequest{
		WorkspaceID:   "test-workspace",
		IntegrationID: "test-integration",
		MessageID:     "test-server-error-msg",
		FromAddress:   "user@company.com",
		FromName:      "Test Sender",
		To:            "recipient@example.com",
		Subject:       "Should Fail",
		Content:       "<html><body><p>This should not be sent</p></body></html>",
		Provider: &domain.EmailProvider{
			Kind: domain.EmailProviderKindSMTP,
			SMTP: &domain.SMTPSettings{
				Host:               smtpServer.Host(),
				Port:               smtpServer.Port(),
				Username:           "user@company.com",
				AuthType:           "oauth2",
				OAuth2Provider:     "microsoft",
				OAuth2TenantID:     testTenantID,
				OAuth2ClientID:     testClientID,
				OAuth2ClientSecret: "some-secret",
			},
		},
	}

	// Send email - should fail
	err := smtpService.SendEmail(context.Background(), request)
	require.Error(t, err)

	// Verify no emails were sent
	messages := smtpServer.GetMessages()
	assert.Len(t, messages, 0, "No emails should have been sent")
}

// TestOAuth2SMTP_UnsupportedProvider verifies error handling for unsupported OAuth2 providers
func TestOAuth2SMTP_UnsupportedProvider(t *testing.T) {
	// Setup mock SMTP server
	smtpServer := testutil.NewMockOAuth2SMTPServer(map[string]string{})
	defer smtpServer.Close()

	// Create SMTP service
	log := logger.NewLogger()
	tokenService := service.NewOAuth2TokenService(log)
	smtpService := service.NewSMTPServiceWithOAuth2(log, tokenService)

	// Create test request with unsupported provider
	request := domain.SendEmailProviderRequest{
		WorkspaceID:   "test-workspace",
		IntegrationID: "test-integration",
		MessageID:     "test-unsupported-msg",
		FromAddress:   "user@company.com",
		FromName:      "Test Sender",
		To:            "recipient@example.com",
		Subject:       "Should Fail",
		Content:       "<html><body><p>This should not be sent</p></body></html>",
		Provider: &domain.EmailProvider{
			Kind: domain.EmailProviderKindSMTP,
			SMTP: &domain.SMTPSettings{
				Host:               smtpServer.Host(),
				Port:               smtpServer.Port(),
				Username:           "user@company.com",
				AuthType:           "oauth2",
				OAuth2Provider:     "yahoo", // Unsupported
				OAuth2ClientID:     "some-client",
				OAuth2ClientSecret: "some-secret",
			},
		},
	}

	// Send email - should fail with unsupported provider error
	err := smtpService.SendEmail(context.Background(), request)
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "unsupported",
		"Error should mention unsupported provider")
}

// TestOAuth2SMTP_FallbackToBasicAuth verifies that non-OAuth2 auth still works
func TestOAuth2SMTP_FallbackToBasicAuth(t *testing.T) {
	// Setup regular mock SMTP server (from existing test infrastructure)
	server := newMockSMTPServerIntegration(t, true)
	defer server.Close()

	// Create SMTP service
	log := logger.NewLogger()
	tokenService := service.NewOAuth2TokenService(log)
	smtpService := service.NewSMTPServiceWithOAuth2(log, tokenService)

	// Create test request WITHOUT OAuth2 (basic auth)
	request := domain.SendEmailProviderRequest{
		WorkspaceID:   "test-workspace",
		IntegrationID: "test-integration",
		MessageID:     "test-basic-auth-msg",
		FromAddress:   "sender@example.com",
		FromName:      "Test Sender",
		To:            "recipient@example.com",
		Subject:       "Basic Auth Test",
		Content:       "<html><body><p>Test with basic auth</p></body></html>",
		Provider: &domain.EmailProvider{
			Kind: domain.EmailProviderKindSMTP,
			SMTP: &domain.SMTPSettings{
				Host:     "127.0.0.1",
				Port:     server.Port(),
				Username: "testuser",
				Password: "testpass",
				UseTLS:   false,
				// No AuthType = basic auth (default)
			},
		},
	}

	// Send email
	err := smtpService.SendEmail(context.Background(), request)
	require.NoError(t, err)

	// Give server time to process
	time.Sleep(100 * time.Millisecond)

	// Verify AUTH command was sent (basic auth)
	commands := server.GetAllCommands()
	hasAuth := false
	for _, cmd := range commands {
		if strings.HasPrefix(strings.ToUpper(cmd), "AUTH") {
			hasAuth = true
			// Basic auth should NOT be XOAUTH2
			assert.NotContains(t, strings.ToUpper(cmd), "XOAUTH2",
				"Basic auth should not use XOAUTH2")
			break
		}
	}
	assert.True(t, hasAuth, "Should have sent AUTH command")
}
