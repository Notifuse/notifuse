package integration

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
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

	// Scoped keys for the parity subtests below. The bridge seeds the identity it
	// resolved instead of stamping SystemCallKey, so SendNotification runs the same
	// transactional:write gate it runs on /api/transactional.send.
	sendOnlyUser, err := factory.CreateAPIKey(workspace.ID, testutil.WithAPIKeyPermissions(domain.UserPermissions{
		domain.PermissionResourceTransactional: {Write: true},
	}))
	require.NoError(t, err)
	sendOnlyKey := authService.GenerateAPIAuthToken(sendOnlyUser)
	require.NotEmpty(t, sendOnlyKey)

	zeroScopeUser, err := factory.CreateAPIKey(workspace.ID, testutil.WithAPIKeyPermissions(domain.UserPermissions{}))
	require.NoError(t, err)
	zeroScopeKey := authService.GenerateAPIAuthToken(zeroScopeUser)
	require.NotEmpty(t, zeroScopeKey)

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

		// The bridge is the second send path for the same notification. It must apply
		// the same permission gate as /api/transactional.send, or a scoped key would
		// be narrowed over HTTP and unlimited over SMTP.
		t.Run("ScopedKey", func(t *testing.T) {
			// sendAs runs one MAIL/RCPT/DATA exchange with the given key and returns
			// the error the bridge reports at end-of-DATA, which is where a
			// permission denial surfaces — AUTH succeeds for any valid key.
			// contactJSON is the notification's contact object verbatim, and
			// extraHeaders are added to the message head — the bridge fills CC and BCC
			// from Cc:/Bcc: there.
			sendBodyAs := func(t *testing.T, email, key, contactJSON string, extraHeaders ...string) error {
				t.Helper()
				smtpClient := smtpBridgeDialAndAuth(t, addr, email, key)
				defer func() { _ = smtpClient.Close() }()

				require.NoError(t, smtpClient.Mail("sender@example.com"))
				require.NoError(t, smtpClient.Rcpt("recipient@example.com"))

				wc, err := smtpClient.Data()
				require.NoError(t, err)

				head := ""
				for _, header := range extraHeaders {
					head += header + "\n"
				}

				emailMessage := fmt.Sprintf(`From: sender@example.com
To: recipient@example.com
%sSubject: Scoped Key Test
Content-Type: text/plain

{
  "workspace_id": "%s",
  "notification": {
    "id": "password_reset",
    "contact": %s
  }
}`, head, workspace.ID, contactJSON)

				_, err = wc.Write([]byte(emailMessage))
				require.NoError(t, err)

				return wc.Close()
			}

			sendAs := func(t *testing.T, email, key, contactEmail string) error {
				t.Helper()
				return sendBodyAs(t, email, key, fmt.Sprintf(`{"email": %q}`, contactEmail))
			}

			t.Run("transactional write only key sends", func(t *testing.T) {
				// The send path's nested contact upsert runs under a system subcontext,
				// so this key needs no contacts grant to create its recipient.
				require.NoError(t, sendAs(t, sendOnlyUser.Email, sendOnlyKey, "send-only-user@example.com"))

				time.Sleep(500 * time.Millisecond)

				contact, err := appInstance.GetContactRepository().GetContactByEmail(
					context.Background(),
					workspace.ID,
					"send-only-user@example.com",
				)
				require.NoError(t, err)
				assert.Equal(t, "send-only-user@example.com", contact.Email)
			})

			t.Run("zero permission key is rejected", func(t *testing.T) {
				err := sendAs(t, zeroScopeUser.Email, zeroScopeKey, "zero-scope-user@example.com")
				require.Error(t, err, "a key with an empty permissions map must not send over the bridge")
				assert.Contains(t, err.Error(), "Insufficient permissions",
					"the rejection must come from the permission gate, not from an unrelated failure")

				time.Sleep(500 * time.Millisecond)

				_, err = appInstance.GetContactRepository().GetContactByEmail(
					context.Background(),
					workspace.ID,
					"zero-scope-user@example.com",
				)
				assert.Error(t, err, "a rejected send must not create its recipient contact")
			})

			// The nested upsert runs under a system subcontext, so these two subtests
			// are the only thing standing between a send-only credential and every
			// field of an arbitrary contact record.
			t.Run("send only key cannot rewrite an existing contact", func(t *testing.T) {
				const victim = "bridge-victim@example.com"

				// Seed the record with the full-access key, the way a real integration
				// would have created it.
				require.NoError(t, sendBodyAs(t, apiUser.Email, apiKey,
					fmt.Sprintf(`{"email": %q, "first_name": "Original"}`, victim)))
				time.Sleep(500 * time.Millisecond)

				require.NoError(t, sendBodyAs(t, sendOnlyUser.Email, sendOnlyKey,
					fmt.Sprintf(`{"email": %q, "first_name": "Overwritten"}`, victim)))
				time.Sleep(500 * time.Millisecond)

				contact, err := appInstance.GetContactRepository().GetContactByEmail(
					context.Background(), workspace.ID, victim)
				require.NoError(t, err)
				require.NotNil(t, contact.FirstName)
				assert.Equal(t, "Original", contact.FirstName.String,
					"a key without contacts:write must not merge its posted fields onto a stored contact")
			})

			t.Run("send only key cannot set bcc", func(t *testing.T) {
				err := sendBodyAs(t, sendOnlyUser.Email, sendOnlyKey,
					`{"email": "bridge-bcc@example.com"}`, "Bcc: attacker@example.com")
				require.Error(t, err, "the rendered subject is evaluated against the contact, so an extra recipient is a contact read")
				assert.Contains(t, err.Error(), "Insufficient permissions")

				time.Sleep(500 * time.Millisecond)

				_, err = appInstance.GetContactRepository().GetContactByEmail(
					context.Background(), workspace.ID, "bridge-bcc@example.com")
				assert.Error(t, err, "a rejected send must not create its recipient contact")
			})
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

	// --- The same two keys over /api/transactional.send, for the parity claim ---
	t.Run("HTTPParity", func(t *testing.T) {
		client := suite.APIClient

		sendOverHTTP := func(t *testing.T, key, contactEmail string) int {
			t.Helper()
			client.SetToken(key)
			resp, err := client.Post("/api/transactional.send", map[string]interface{}{
				"workspace_id": workspace.ID,
				"notification": map[string]interface{}{
					"id":       "password_reset",
					"contact":  map[string]interface{}{"email": contactEmail},
					"channels": []string{string(domain.TransactionalChannelEmail)},
				},
			})
			require.NoError(t, err)
			defer resp.Body.Close()
			return resp.StatusCode
		}

		assert.Equal(t, http.StatusOK, sendOverHTTP(t, sendOnlyKey, "http-send-only@example.com"),
			"a transactional:write key sends over HTTP as it does over the bridge")
		assert.Equal(t, http.StatusForbidden, sendOverHTTP(t, zeroScopeKey, "http-zero-scope@example.com"),
			"a zero-permission key is refused over HTTP as it is over the bridge")
	})
}
