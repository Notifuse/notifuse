package integration

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/smtp"
	"path/filepath"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/Notifuse/notifuse/pkg/smtp_relay"
	"github.com/Notifuse/notifuse/tests/testutil"
	"github.com/golang-jwt/jwt/v5"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// loadTestTLSConfig loads the test TLS certificates
func loadTestTLSConfig(t *testing.T) *tls.Config {
	certPath := filepath.Join("..", "testdata", "certs", "test_cert.pem")
	keyPath := filepath.Join("..", "testdata", "certs", "test_key.pem")

	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	require.NoError(t, err, "Failed to load test certificates")

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}
}

// TestSMTPRelayE2E_FullFlow tests the complete SMTP relay flow from client to notification
func TestSMTPRelayE2E_FullFlow(t *testing.T) {
	testutil.SkipIfShort(t)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create JWT secret and API key
	jwtSecret := []byte("test-secret-key-for-jwt-signing-minimum-32-chars")
	workspaceID := "workspace123"
	apiUserID := "api-user-123"
	apiEmail := "api@example.com"

	// Create a valid API key token
	claims := service.UserClaims{
		UserID: apiUserID,
		Email:  apiEmail,
		Type:   string(domain.UserTypeAPIKey),
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	apiKey, err := token.SignedString(jwtSecret)
	require.NoError(t, err)

	// Setup mocks
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockWorkspaceRepo.EXPECT().
		GetUserWorkspace(gomock.Any(), apiUserID, workspaceID).
		Return(&domain.UserWorkspace{
			UserID:      apiUserID,
			WorkspaceID: workspaceID,
			Role:        "member",
		}, nil).
		AnyTimes()

	var capturedParams domain.TransactionalNotificationSendParams
	var capturedWorkspaceID string

	mockTransactionalService := mocks.NewMockTransactionalNotificationService(ctrl)
	mockTransactionalService.EXPECT().
		SendNotification(gomock.Any(), workspaceID, gomock.Any()).
		DoAndReturn(func(ctx context.Context, wsID string, params domain.TransactionalNotificationSendParams) (string, error) {
			capturedWorkspaceID = wsID
			capturedParams = params
			return "msg-123", nil
		}).
		Times(1)

	log := logger.NewLogger()

	// Create rate limiter
	rateLimiter := service.NewRateLimiter(5, 1*time.Minute)
	defer rateLimiter.Stop()

	// Create SMTP relay handler service
	handlerService := service.NewSMTPRelayHandlerService(nil, mockTransactionalService, mockWorkspaceRepo, log, jwtSecret, rateLimiter)

	// Create SMTP backend with the handler
	backend := smtp_relay.NewBackend(
		handlerService.Authenticate,
		handlerService.HandleMessage,
		log,
	)

	// Find an available port
	testPort := testutil.FindAvailablePort(t)

	// Load TLS config for testing
	tlsConfig := loadTestTLSConfig(t)

	// Create and start SMTP server
	serverConfig := smtp_relay.ServerConfig{
		Host:      "127.0.0.1",
		Port:      testPort,
		Domain:    "test.localhost",
		TLSConfig: tlsConfig,
		Logger:    log,
	}

	server, err := smtp_relay.NewServer(serverConfig, backend)
	require.NoError(t, err)

	// Start server in goroutine
	go func() {
		_ = server.Start()
	}()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	// Ensure server is shut down after test
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()

	// Connect to SMTP server using net/smtp (connect to localhost, not 127.0.0.1)
	addr := fmt.Sprintf("localhost:%d", testPort)
	client, err := smtp.Dial(addr)
	require.NoError(t, err)
	defer client.Close()

	// Start TLS
	tlsClientConfig := &tls.Config{
		InsecureSkipVerify: true, // Skip verification for self-signed cert in tests
		ServerName:         "localhost",
	}
	err = client.StartTLS(tlsClientConfig)
	require.NoError(t, err)

	// Authenticate (using API email as username)
	auth := smtp.PlainAuth("", apiEmail, apiKey, "localhost")
	err = client.Auth(auth)
	require.NoError(t, err)

	// Set sender
	err = client.Mail("sender@example.com")
	require.NoError(t, err)

	// Set recipient
	err = client.Rcpt("recipient@example.com")
	require.NoError(t, err)

	// Send message body
	wc, err := client.Data()
	require.NoError(t, err)

	// Set JSON payload as email body (with workspace_id)
	emailMessage := `From: sender@example.com
To: recipient@example.com
Subject: Test Notification
Content-Type: text/plain

{
  "workspace_id": "workspace123",
  "notification": {
    "id": "password_reset",
    "contact": {
      "email": "user@example.com",
      "first_name": "John",
      "last_name": "Doe"
    },
    "data": {
      "reset_token": "abc123",
      "expires_in": "1 hour"
    },
    "metadata": {
      "source": "smtp_relay_test"
    }
  }
}`
	_, err = wc.Write([]byte(emailMessage))
	require.NoError(t, err)

	err = wc.Close()
	require.NoError(t, err)

	// Quit
	err = client.Quit()
	require.NoError(t, err)

	// Wait a bit for message processing
	time.Sleep(100 * time.Millisecond)

	// Verify the notification was sent
	assert.Equal(t, workspaceID, capturedWorkspaceID)
	assert.Equal(t, "password_reset", capturedParams.ID)
	assert.Equal(t, "user@example.com", capturedParams.Contact.Email)
	assert.NotNil(t, capturedParams.Contact.FirstName)
	assert.Equal(t, "John", capturedParams.Contact.FirstName.String)
	assert.NotNil(t, capturedParams.Contact.LastName)
	assert.Equal(t, "Doe", capturedParams.Contact.LastName.String)
	assert.Equal(t, "abc123", capturedParams.Data["reset_token"])
	assert.Equal(t, "1 hour", capturedParams.Data["expires_in"])
	assert.Equal(t, "smtp_relay_test", capturedParams.Metadata["source"])
}

// TestSMTPRelayE2E_WithEmailHeaders tests CC, BCC, and Reply-To header extraction
func TestSMTPRelayE2E_WithEmailHeaders(t *testing.T) {
	testutil.SkipIfShort(t)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create JWT secret and API key
	jwtSecret := []byte("test-secret-key-for-jwt-signing-minimum-32-chars")
	workspaceID := "workspace123"
	apiUserID := "api-user-123"
	apiEmail := "api@example.com"

	claims := service.UserClaims{
		UserID: apiUserID,
		Email:  apiEmail,
		Type:   string(domain.UserTypeAPIKey),
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	apiKey, err := token.SignedString(jwtSecret)
	require.NoError(t, err)

	// Setup mocks
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockWorkspaceRepo.EXPECT().
		GetUserWorkspace(gomock.Any(), apiUserID, workspaceID).
		Return(&domain.UserWorkspace{
			UserID:      apiUserID,
			WorkspaceID: workspaceID,
			Role:        "member",
		}, nil).
		AnyTimes()

	var capturedParams domain.TransactionalNotificationSendParams

	mockTransactionalService := mocks.NewMockTransactionalNotificationService(ctrl)
	mockTransactionalService.EXPECT().
		SendNotification(gomock.Any(), workspaceID, gomock.Any()).
		DoAndReturn(func(ctx context.Context, wsID string, params domain.TransactionalNotificationSendParams) (string, error) {
			capturedParams = params
			return "msg-456", nil
		}).
		Times(1)

	log := logger.NewLogger()

	// Create rate limiter
	rateLimiter := service.NewRateLimiter(5, 1*time.Minute)
	defer rateLimiter.Stop()

	handlerService := service.NewSMTPRelayHandlerService(nil, mockTransactionalService, mockWorkspaceRepo, log, jwtSecret, rateLimiter)
	backend := smtp_relay.NewBackend(handlerService.Authenticate, handlerService.HandleMessage, log)

	testPort := testutil.FindAvailablePort(t)

	tlsConfig := loadTestTLSConfig(t)

	serverConfig := smtp_relay.ServerConfig{
		Host:      "127.0.0.1",
		Port:      testPort,
		Domain:    "test.localhost",
		TLSConfig: tlsConfig,
		Logger:    log,
	}

	server, err := smtp_relay.NewServer(serverConfig, backend)
	require.NoError(t, err)

	go func() {
		_ = server.Start()
	}()

	time.Sleep(100 * time.Millisecond)

	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()

	// Connect and authenticate
	addr := fmt.Sprintf("localhost:%d", testPort)
	client, err := smtp.Dial(addr)
	require.NoError(t, err)
	defer client.Close()

	tlsClientConfig := &tls.Config{
		InsecureSkipVerify: true,
		ServerName:         "localhost",
	}
	err = client.StartTLS(tlsClientConfig)
	require.NoError(t, err)

	auth := smtp.PlainAuth("", apiEmail, apiKey, "localhost")
	err = client.Auth(auth)
	require.NoError(t, err)

	err = client.Mail("sender@example.com")
	require.NoError(t, err)

	// Send to multiple recipients (To, Cc, Bcc)
	err = client.Rcpt("recipient@example.com")
	require.NoError(t, err)
	err = client.Rcpt("cc1@example.com")
	require.NoError(t, err)
	err = client.Rcpt("cc2@example.com")
	require.NoError(t, err)
	err = client.Rcpt("bcc@example.com")
	require.NoError(t, err)

	wc, err := client.Data()
	require.NoError(t, err)

	// Email with CC, BCC, and Reply-To headers (with workspace_id)
	emailMessage := `From: sender@example.com
To: recipient@example.com
Cc: cc1@example.com, cc2@example.com
Bcc: bcc@example.com
Reply-To: replyto@example.com
Subject: Test with Headers
Content-Type: text/plain

{
  "workspace_id": "workspace123",
  "notification": {
    "id": "welcome_email",
    "contact": {
      "email": "user@example.com"
    }
  }
}`
	_, err = wc.Write([]byte(emailMessage))
	require.NoError(t, err)

	err = wc.Close()
	require.NoError(t, err)

	err = client.Quit()
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// Verify email headers were extracted
	assert.Equal(t, "welcome_email", capturedParams.ID)
	assert.Equal(t, "user@example.com", capturedParams.Contact.Email)
	assert.Len(t, capturedParams.EmailOptions.CC, 2)
	assert.Contains(t, capturedParams.EmailOptions.CC, "cc1@example.com")
	assert.Contains(t, capturedParams.EmailOptions.CC, "cc2@example.com")
	assert.Len(t, capturedParams.EmailOptions.BCC, 1)
	assert.Equal(t, "bcc@example.com", capturedParams.EmailOptions.BCC[0])
	assert.Equal(t, "replyto@example.com", capturedParams.EmailOptions.ReplyTo)
}

// TestSMTPRelayE2E_InvalidAuthentication tests authentication failure
func TestSMTPRelayE2E_InvalidAuthentication(t *testing.T) {
	testutil.SkipIfShort(t)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	jwtSecret := []byte("test-secret-key-for-jwt-signing-minimum-32-chars")

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockTransactionalService := mocks.NewMockTransactionalNotificationService(ctrl)

	log := logger.NewLogger()

	// Create rate limiter
	rateLimiter := service.NewRateLimiter(5, 1*time.Minute)
	defer rateLimiter.Stop()

	handlerService := service.NewSMTPRelayHandlerService(nil, mockTransactionalService, mockWorkspaceRepo, log, jwtSecret, rateLimiter)
	backend := smtp_relay.NewBackend(handlerService.Authenticate, handlerService.HandleMessage, log)

	testPort := testutil.FindAvailablePort(t)

	tlsConfig := loadTestTLSConfig(t)

	serverConfig := smtp_relay.ServerConfig{
		Host:      "127.0.0.1",
		Port:      testPort,
		Domain:    "test.localhost",
		TLSConfig: tlsConfig,
		Logger:    log,
	}

	server, err := smtp_relay.NewServer(serverConfig, backend)
	require.NoError(t, err)

	go func() {
		_ = server.Start()
	}()

	time.Sleep(100 * time.Millisecond)

	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()

	addr := fmt.Sprintf("localhost:%d", testPort)
	client, err := smtp.Dial(addr)
	require.NoError(t, err)
	defer client.Close()

	tlsClientConfig := &tls.Config{
		InsecureSkipVerify: true,
		ServerName:         "localhost",
	}
	err = client.StartTLS(tlsClientConfig)
	require.NoError(t, err)

	// Try to authenticate with invalid credentials
	auth := smtp.PlainAuth("", "invalid@example.com", "invalid-api-key", "localhost")
	err = client.Auth(auth)

	// Should fail
	assert.Error(t, err)
}

// TestSMTPRelayE2E_InvalidJSON tests handling of non-JSON payload
func TestSMTPRelayE2E_InvalidJSON(t *testing.T) {
	testutil.SkipIfShort(t)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	jwtSecret := []byte("test-secret-key-for-jwt-signing-minimum-32-chars")
	workspaceID := "workspace123"
	apiUserID := "api-user-123"
	apiEmail := "api@example.com"

	claims := service.UserClaims{
		UserID: apiUserID,
		Email:  apiEmail,
		Type:   string(domain.UserTypeAPIKey),
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	apiKey, err := token.SignedString(jwtSecret)
	require.NoError(t, err)

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockWorkspaceRepo.EXPECT().
		GetUserWorkspace(gomock.Any(), apiUserID, workspaceID).
		Return(&domain.UserWorkspace{
			UserID:      apiUserID,
			WorkspaceID: workspaceID,
			Role:        "member",
		}, nil).
		AnyTimes()

	mockTransactionalService := mocks.NewMockTransactionalNotificationService(ctrl)

	log := logger.NewLogger()

	// Create rate limiter
	rateLimiter := service.NewRateLimiter(5, 1*time.Minute)
	defer rateLimiter.Stop()

	handlerService := service.NewSMTPRelayHandlerService(nil, mockTransactionalService, mockWorkspaceRepo, log, jwtSecret, rateLimiter)
	backend := smtp_relay.NewBackend(handlerService.Authenticate, handlerService.HandleMessage, log)

	testPort := testutil.FindAvailablePort(t)

	tlsConfig := loadTestTLSConfig(t)

	serverConfig := smtp_relay.ServerConfig{
		Host:      "127.0.0.1",
		Port:      testPort,
		Domain:    "test.localhost",
		TLSConfig: tlsConfig,
		Logger:    log,
	}

	server, err := smtp_relay.NewServer(serverConfig, backend)
	require.NoError(t, err)

	go func() {
		_ = server.Start()
	}()

	time.Sleep(100 * time.Millisecond)

	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()

	addr := fmt.Sprintf("localhost:%d", testPort)
	client, err := smtp.Dial(addr)
	require.NoError(t, err)
	defer client.Close()

	tlsClientConfig := &tls.Config{
		InsecureSkipVerify: true,
		ServerName:         "localhost",
	}
	err = client.StartTLS(tlsClientConfig)
	require.NoError(t, err)

	auth := smtp.PlainAuth("", apiEmail, apiKey, "localhost")
	err = client.Auth(auth)
	require.NoError(t, err)

	err = client.Mail("sender@example.com")
	require.NoError(t, err)

	err = client.Rcpt("recipient@example.com")
	require.NoError(t, err)

	wc, err := client.Data()
	require.NoError(t, err)

	// Send invalid JSON
	emailMessage := `From: sender@example.com
To: recipient@example.com
Subject: Invalid JSON Test
Content-Type: text/plain

This is not valid JSON`
	_, err = wc.Write([]byte(emailMessage))
	require.NoError(t, err)

	// The handler will return an error which should propagate
	err = wc.Close()
	assert.Error(t, err)
}

// TestSMTPRelayE2E_MultipleMessages tests sending multiple messages in sequence
func TestSMTPRelayE2E_MultipleMessages(t *testing.T) {
	testutil.SkipIfShort(t)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	jwtSecret := []byte("test-secret-key-for-jwt-signing-minimum-32-chars")
	workspaceID := "workspace123"
	apiUserID := "api-user-123"
	apiEmail := "api@example.com"

	claims := service.UserClaims{
		UserID: apiUserID,
		Email:  apiEmail,
		Type:   string(domain.UserTypeAPIKey),
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	apiKey, err := token.SignedString(jwtSecret)
	require.NoError(t, err)

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockWorkspaceRepo.EXPECT().
		GetUserWorkspace(gomock.Any(), apiUserID, workspaceID).
		Return(&domain.UserWorkspace{
			UserID:      apiUserID,
			WorkspaceID: workspaceID,
			Role:        "member",
		}, nil).
		AnyTimes()

	capturedIDs := []string{}

	mockTransactionalService := mocks.NewMockTransactionalNotificationService(ctrl)
	mockTransactionalService.EXPECT().
		SendNotification(gomock.Any(), workspaceID, gomock.Any()).
		DoAndReturn(func(ctx context.Context, wsID string, params domain.TransactionalNotificationSendParams) (string, error) {
			capturedIDs = append(capturedIDs, params.ID)
			return fmt.Sprintf("msg-%d", len(capturedIDs)), nil
		}).
		Times(3)

	log := logger.NewLogger()

	// Create rate limiter
	rateLimiter := service.NewRateLimiter(5, 1*time.Minute)
	defer rateLimiter.Stop()

	handlerService := service.NewSMTPRelayHandlerService(nil, mockTransactionalService, mockWorkspaceRepo, log, jwtSecret, rateLimiter)
	backend := smtp_relay.NewBackend(handlerService.Authenticate, handlerService.HandleMessage, log)

	testPort := testutil.FindAvailablePort(t)

	tlsConfig := loadTestTLSConfig(t)

	serverConfig := smtp_relay.ServerConfig{
		Host:      "127.0.0.1",
		Port:      testPort,
		Domain:    "test.localhost",
		TLSConfig: tlsConfig,
		Logger:    log,
	}

	server, err := smtp_relay.NewServer(serverConfig, backend)
	require.NoError(t, err)

	go func() {
		_ = server.Start()
	}()

	time.Sleep(100 * time.Millisecond)

	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()

	// Send three different notifications
	notificationIDs := []string{"welcome_email", "password_reset", "order_confirmation"}

	for _, notifID := range notificationIDs {
		addr := fmt.Sprintf("localhost:%d", testPort)
		client, err := smtp.Dial(addr)
		require.NoError(t, err)

		tlsClientConfig := &tls.Config{
			InsecureSkipVerify: true,
			ServerName:         "localhost",
		}
		err = client.StartTLS(tlsClientConfig)
		require.NoError(t, err)

		auth := smtp.PlainAuth("", apiEmail, apiKey, "localhost")
		err = client.Auth(auth)
		require.NoError(t, err)

		err = client.Mail("sender@example.com")
		require.NoError(t, err)

		err = client.Rcpt("recipient@example.com")
		require.NoError(t, err)

		wc, err := client.Data()
		require.NoError(t, err)

		emailMessage := fmt.Sprintf(`From: sender@example.com
To: recipient@example.com
Subject: Test %s
Content-Type: text/plain

{
  "workspace_id": "workspace123",
  "notification": {
    "id": "%s",
    "contact": {
      "email": "user@example.com"
    }
  }
}`, notifID, notifID)

		_, err = wc.Write([]byte(emailMessage))
		require.NoError(t, err)

		err = wc.Close()
		require.NoError(t, err)

		err = client.Quit()
		require.NoError(t, err)

		time.Sleep(50 * time.Millisecond)
	}

	// Verify all three notifications were processed
	assert.Len(t, capturedIDs, 3)
	assert.Contains(t, capturedIDs, "welcome_email")
	assert.Contains(t, capturedIDs, "password_reset")
	assert.Contains(t, capturedIDs, "order_confirmation")
}

