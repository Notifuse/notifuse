package integration

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"path/filepath"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/app"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/service"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/Notifuse/notifuse/pkg/ratelimiter"
	"github.com/Notifuse/notifuse/pkg/smtp_bridge"
	"github.com/Notifuse/notifuse/tests/testutil"
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

// smtpBridgeDialAndAuth dials, performs STARTTLS, and authenticates.
func smtpBridgeDialAndAuth(t *testing.T, addr, email, apiKey string) *smtp.Client {
	t.Helper()
	smtpClient, err := smtp.Dial(addr)
	require.NoError(t, err)

	tlsClientConfig := &tls.Config{
		InsecureSkipVerify: true,
		ServerName:         "localhost",
	}
	err = smtpClient.StartTLS(tlsClientConfig)
	require.NoError(t, err)

	auth := smtp.PlainAuth("", email, apiKey, "localhost")
	err = smtpClient.Auth(auth)
	require.NoError(t, err)

	return smtpClient
}

// smtpBridgeDialPlain dials a plaintext SMTP connection and authenticates — for Mode=off.
// NOTE: Go's net/smtp.PlainAuth refuses to send credentials over plaintext unless the
// dial host is "localhost"/"127.0.0.1"/"::1" (see net/smtp/auth.go isLocalhost). The
// caller must pass a localhost-flavoured addr for auth to succeed.
func smtpBridgeDialPlain(t *testing.T, addr, email, apiKey string) *smtp.Client {
	t.Helper()
	smtpClient, err := smtp.Dial(addr)
	require.NoError(t, err)

	auth := smtp.PlainAuth("", email, apiKey, "localhost")
	err = smtpClient.Auth(auth)
	require.NoError(t, err)

	return smtpClient
}

// smtpBridgeDialImplicit dials over TLS directly (SMTPS) and authenticates — for Mode=implicit.
func smtpBridgeDialImplicit(t *testing.T, addr, email, apiKey string, tlsCfg *tls.Config) *smtp.Client {
	t.Helper()
	conn, err := tls.Dial("tcp", addr, tlsCfg)
	require.NoError(t, err)

	smtpClient, err := smtp.NewClient(conn, "localhost")
	require.NoError(t, err)

	auth := smtp.PlainAuth("", email, apiKey, "localhost")
	err = smtpClient.Auth(auth)
	require.NoError(t, err)

	return smtpClient
}

// startBridge spins up an SMTP bridge server on a fresh ephemeral port with
// the given mode. Returns the listener addr and a cleanup func.
func startBridge(t *testing.T, mode string, tlsCfg *tls.Config, handlerService *service.SMTPBridgeHandlerService, log logger.Logger) (string, func()) {
	t.Helper()
	backend := smtp_bridge.NewBackend(handlerService.Authenticate, handlerService.HandleMessage, log)
	port := testutil.FindAvailablePort(t)

	serverConfig := smtp_bridge.ServerConfig{
		Host:      "127.0.0.1",
		Port:      port,
		Domain:    "test.localhost",
		Mode:      mode,
		TLSConfig: tlsCfg,
		Logger:    log,
	}

	server, err := smtp_bridge.NewServer(serverConfig, backend)
	require.NoError(t, err)

	go func() { _ = server.Start() }()
	time.Sleep(100 * time.Millisecond)

	return net.JoinHostPort("localhost", fmt.Sprintf("%d", port)), func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}
}

// TestSMTPBridgeE2E consolidates all SMTP bridge integration tests under a single
// shared setup to reduce suite overhead. Within that fixture it exercises all
// three TLS modes (STARTTLS, Off, Implicit).
func TestSMTPBridgeE2E(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SetupTestEnvironment()
	defer testutil.CleanupTestEnvironment()

	suite := testutil.NewIntegrationTestSuite(t, func(cfg *config.Config) testutil.AppInterface {
		return app.NewApp(cfg)
	})
	defer suite.Cleanup()

	factory := suite.DataFactory
	appInstance := suite.ServerManager.GetApp()

	// Shared setup: user, workspace, SMTP provider
	user, err := factory.CreateUser()
	require.NoError(t, err)
	workspace, err := factory.CreateWorkspace()
	require.NoError(t, err)
	err = factory.AddUserToWorkspace(user.ID, workspace.ID, "owner")
	require.NoError(t, err)
	_, err = factory.SetupWorkspaceWithSMTPProvider(workspace.ID)
	require.NoError(t, err)

	// Shared template
	template, err := factory.CreateTemplate(workspace.ID, testutil.WithTemplateName("SMTP Bridge Test"))
	require.NoError(t, err)

	// Create all notifications used across subtests
	notificationIDs := []string{"password_reset", "welcome_email", "order_confirmation"}
	for _, notifID := range notificationIDs {
		_, err = factory.CreateTransactionalNotification(workspace.ID,
			testutil.WithNotificationID(notifID),
			testutil.WithNotificationTemplateID(template.ID))
		require.NoError(t, err)
	}

	// Shared API key
	apiUser, err := factory.CreateAPIKey(workspace.ID)
	require.NoError(t, err)
	authService := appInstance.GetAuthService().(*service.AuthService)
	apiKey := authService.GenerateAPIAuthToken(apiUser)
	require.NotEmpty(t, apiKey)

	jwtSecret := suite.Config.Security.JWTSecret

	// Shared handler service
	log := logger.NewLogger()
	rl := ratelimiter.NewRateLimiter()
	rl.SetPolicy("smtp", 20, 1*time.Minute)
	defer rl.Stop()

	handlerService := service.NewSMTPBridgeHandlerService(
		authService,
		appInstance.GetTransactionalNotificationService(),
		appInstance.GetWorkspaceRepository(),
		log,
		jwtSecret,
		rl,
	)

	tlsConfig := loadTestTLSConfig(t)

	// --- STARTTLS mode (the main integration surface; exhaustive subtests) ---
	t.Run("STARTTLS", func(t *testing.T) {
		addr, stop := startBridge(t, smtp_bridge.ModeSTARTTLS, tlsConfig, handlerService, log)
		defer stop()

		t.Run("FullFlow", func(t *testing.T) {
			smtpClient := smtpBridgeDialAndAuth(t, addr, apiUser.Email, apiKey)
			defer func() { _ = smtpClient.Close() }()

			err := smtpClient.Mail("sender@example.com")
			require.NoError(t, err)

			err = smtpClient.Rcpt("recipient@example.com")
			require.NoError(t, err)

			wc, err := smtpClient.Data()
			require.NoError(t, err)

			emailMessage := fmt.Sprintf(`From: sender@example.com
To: recipient@example.com
Subject: Test Notification
Content-Type: text/plain

{
  "workspace_id": "%s",
  "notification": {
    "id": "password_reset",
    "contact": {
      "email": "user@example.com",
      "first_name": "John",
      "last_name": "Doe"
    },
    "data": {
      "reset_token": "abc123"
    }
  }
}`, workspace.ID)

			_, err = wc.Write([]byte(emailMessage))
			require.NoError(t, err)

			err = wc.Close()
			require.NoError(t, err)

			err = smtpClient.Quit()
			require.NoError(t, err)

			time.Sleep(500 * time.Millisecond)

			messages, _, err := appInstance.GetMessageHistoryRepository().ListMessages(
				context.Background(),
				workspace.ID,
				workspace.Settings.SecretKey,
				domain.MessageListParams{Limit: 10},
			)
			require.NoError(t, err)
			assert.GreaterOrEqual(t, len(messages), 1, "At least one message should be recorded")

			contact, err := appInstance.GetContactRepository().GetContactByEmail(
				context.Background(),
				workspace.ID,
				"user@example.com",
			)
			require.NoError(t, err)
			assert.Equal(t, "user@example.com", contact.Email)
			assert.Equal(t, "John", contact.FirstName.String)
			assert.Equal(t, "Doe", contact.LastName.String)
		})

		t.Run("WithEmailHeaders", func(t *testing.T) {
			smtpClient := smtpBridgeDialAndAuth(t, addr, apiUser.Email, apiKey)
			defer func() { _ = smtpClient.Close() }()

			err := smtpClient.Mail("sender@example.com")
			require.NoError(t, err)

			err = smtpClient.Rcpt("recipient@example.com")
			require.NoError(t, err)
			err = smtpClient.Rcpt("cc1@example.com")
			require.NoError(t, err)
			err = smtpClient.Rcpt("cc2@example.com")
			require.NoError(t, err)
			err = smtpClient.Rcpt("bcc@example.com")
			require.NoError(t, err)

			wc, err := smtpClient.Data()
			require.NoError(t, err)

			emailMessage := fmt.Sprintf(`From: sender@example.com
To: recipient@example.com
Cc: cc1@example.com, cc2@example.com
Bcc: bcc@example.com
Reply-To: replyto@example.com
Subject: Test with Headers
Content-Type: text/plain

{
  "workspace_id": "%s",
  "notification": {
    "id": "welcome_email",
    "contact": {
      "email": "user@example.com"
    }
  }
}`, workspace.ID)

			_, err = wc.Write([]byte(emailMessage))
			require.NoError(t, err)

			err = wc.Close()
			require.NoError(t, err)

			err = smtpClient.Quit()
			require.NoError(t, err)

			time.Sleep(500 * time.Millisecond)

			messages, _, err := appInstance.GetMessageHistoryRepository().ListMessages(
				context.Background(),
				workspace.ID,
				workspace.Settings.SecretKey,
				domain.MessageListParams{Limit: 10},
			)
			require.NoError(t, err)
			assert.GreaterOrEqual(t, len(messages), 1, "At least one message should be recorded")
			t.Log("Email with headers was processed successfully")
		})

		t.Run("InvalidAuthentication", func(t *testing.T) {
			smtpClient, err := smtp.Dial(addr)
			require.NoError(t, err)
			defer func() { _ = smtpClient.Close() }()

			tlsClientConfig := &tls.Config{
				InsecureSkipVerify: true,
				ServerName:         "localhost",
			}
			err = smtpClient.StartTLS(tlsClientConfig)
			require.NoError(t, err)

			auth := smtp.PlainAuth("", "invalid@example.com", "invalid-api-key", "localhost")
			err = smtpClient.Auth(auth)
			assert.Error(t, err)
		})

		t.Run("InvalidJSON", func(t *testing.T) {
			smtpClient := smtpBridgeDialAndAuth(t, addr, apiUser.Email, apiKey)
			defer func() { _ = smtpClient.Close() }()

			err := smtpClient.Mail("sender@example.com")
			require.NoError(t, err)

			err = smtpClient.Rcpt("recipient@example.com")
			require.NoError(t, err)

			wc, err := smtpClient.Data()
			require.NoError(t, err)

			emailMessage := `From: sender@example.com
To: recipient@example.com
Subject: Invalid JSON Test
Content-Type: text/plain

This is not valid JSON`

			_, err = wc.Write([]byte(emailMessage))
			require.NoError(t, err)

			err = wc.Close()
			assert.Error(t, err)
		})

		t.Run("MultipleMessages", func(t *testing.T) {
			for _, notifID := range notificationIDs {
				smtpClient := smtpBridgeDialAndAuth(t, addr, apiUser.Email, apiKey)

				err := smtpClient.Mail("sender@example.com")
				require.NoError(t, err)

				err = smtpClient.Rcpt("recipient@example.com")
				require.NoError(t, err)

				wc, err := smtpClient.Data()
				require.NoError(t, err)

				emailMessage := fmt.Sprintf(`From: sender@example.com
To: recipient@example.com
Subject: Test %s
Content-Type: text/plain

{
  "workspace_id": "%s",
  "notification": {
    "id": "%s",
    "contact": {
      "email": "user@example.com"
    }
  }
}`, notifID, workspace.ID, notifID)

				_, err = wc.Write([]byte(emailMessage))
				require.NoError(t, err)

				err = wc.Close()
				require.NoError(t, err)

				err = smtpClient.Quit()
				require.NoError(t, err)

				time.Sleep(50 * time.Millisecond)
			}

			time.Sleep(500 * time.Millisecond)

			messages, _, err := appInstance.GetMessageHistoryRepository().ListMessages(
				context.Background(),
				workspace.ID,
				workspace.Settings.SecretKey,
				domain.MessageListParams{Limit: 10},
			)
			require.NoError(t, err)
			assert.GreaterOrEqual(t, len(messages), 3, "At least three messages should be recorded")
		})
	})

	// --- Mode=off: plaintext AUTH + DATA over an unencrypted TCP socket ---
	t.Run("Off", func(t *testing.T) {
		addr, stop := startBridge(t, smtp_bridge.ModeOff, nil, handlerService, log)
		defer stop()

		smtpClient := smtpBridgeDialPlain(t, addr, apiUser.Email, apiKey)
		defer func() { _ = smtpClient.Close() }()

		err := smtpClient.Mail("sender@example.com")
		require.NoError(t, err)

		err = smtpClient.Rcpt("recipient@example.com")
		require.NoError(t, err)

		wc, err := smtpClient.Data()
		require.NoError(t, err)

		emailMessage := fmt.Sprintf(`From: sender@example.com
To: recipient@example.com
Subject: Test Plaintext
Content-Type: text/plain

{
  "workspace_id": "%s",
  "notification": {
    "id": "password_reset",
    "contact": {
      "email": "plaintext-user@example.com"
    }
  }
}`, workspace.ID)

		_, err = wc.Write([]byte(emailMessage))
		require.NoError(t, err)

		require.NoError(t, wc.Close())
		require.NoError(t, smtpClient.Quit())

		time.Sleep(500 * time.Millisecond)

		contact, err := appInstance.GetContactRepository().GetContactByEmail(
			context.Background(),
			workspace.ID,
			"plaintext-user@example.com",
		)
		require.NoError(t, err)
		assert.Equal(t, "plaintext-user@example.com", contact.Email)
	})

	// --- Mode=implicit: tls.Dial straight into the listener (SMTPS) ---
	t.Run("Implicit", func(t *testing.T) {
		addr, stop := startBridge(t, smtp_bridge.ModeImplicit, tlsConfig, handlerService, log)
		defer stop()

		clientTLS := &tls.Config{
			InsecureSkipVerify: true,
			ServerName:         "localhost",
		}
		smtpClient := smtpBridgeDialImplicit(t, addr, apiUser.Email, apiKey, clientTLS)
		defer func() { _ = smtpClient.Close() }()

		err := smtpClient.Mail("sender@example.com")
		require.NoError(t, err)

		err = smtpClient.Rcpt("recipient@example.com")
		require.NoError(t, err)

		wc, err := smtpClient.Data()
		require.NoError(t, err)

		emailMessage := fmt.Sprintf(`From: sender@example.com
To: recipient@example.com
Subject: Test Implicit TLS
Content-Type: text/plain

{
  "workspace_id": "%s",
  "notification": {
    "id": "password_reset",
    "contact": {
      "email": "implicit-user@example.com"
    }
  }
}`, workspace.ID)

		_, err = wc.Write([]byte(emailMessage))
		require.NoError(t, err)

		require.NoError(t, wc.Close())
		require.NoError(t, smtpClient.Quit())

		time.Sleep(500 * time.Millisecond)

		contact, err := appInstance.GetContactRepository().GetContactByEmail(
			context.Background(),
			workspace.ID,
			"implicit-user@example.com",
		)
		require.NoError(t, err)
		assert.Equal(t, "implicit-user@example.com", contact.Email)
	})
}
