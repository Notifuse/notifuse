package service

import (
	"bytes"
	"context"
	"net/mail"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/Notifuse/notifuse/pkg/ratelimiter"
	"github.com/golang-jwt/jwt/v5"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSMTPBridgeHandlerService_Authenticate_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create a test JWT secret
	jwtSecret := []byte("test-secret-key-for-jwt-signing-minimum-32-chars")
	apiEmail := "api@example.com"

	// Create a valid API key token
	claims := UserClaims{
		UserID: "api-user-123",
		Email:  apiEmail,
		Type:   string(domain.UserTypeAPIKey),
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	apiKey, err := token.SignedString(jwtSecret)
	if err != nil {
		t.Fatalf("Failed to create test token: %v", err)
	}

	log := logger.NewLogger()
	rl := ratelimiter.NewRateLimiter()
	rl.SetPolicy("smtp", 5, 1*time.Minute)
	defer rl.Stop()
	mockRepo := mocks.NewMockWorkspaceRepository(ctrl)
	service := NewSMTPBridgeHandlerService(nil, nil, mockRepo, log, jwtSecret, rl)

	userID, err := service.Authenticate(apiEmail, apiKey)

	assert.NoError(t, err)
	assert.Equal(t, "api-user-123", userID)
}

func TestSMTPBridgeHandlerService_Authenticate_InvalidToken(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	jwtSecret := []byte("test-secret-key-for-jwt-signing-minimum-32-chars")

	mockRepo := mocks.NewMockWorkspaceRepository(ctrl)
	log := logger.NewLogger()
	rl := ratelimiter.NewRateLimiter()
	rl.SetPolicy("smtp", 5, 1*time.Minute)
	defer rl.Stop()
	service := NewSMTPBridgeHandlerService(nil, nil, mockRepo, log, jwtSecret, rl)

	_, err := service.Authenticate("workspace123", "invalid-token")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid API key")
}

func TestSMTPBridgeHandlerService_Authenticate_WrongUserType(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	jwtSecret := []byte("test-secret-key-for-jwt-signing-minimum-32-chars")
	apiEmail := "user@example.com"

	// Create a token with wrong user type
	claims := UserClaims{
		UserID: "regular-user-123",
		Email:  apiEmail,
		Type:   string(domain.UserTypeUser), // Not an API key
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	apiKey, _ := token.SignedString(jwtSecret)

	mockRepo := mocks.NewMockWorkspaceRepository(ctrl)
	log := logger.NewLogger()
	rl := ratelimiter.NewRateLimiter()
	rl.SetPolicy("smtp", 5, 1*time.Minute)
	defer rl.Stop()
	service := NewSMTPBridgeHandlerService(nil, nil, mockRepo, log, jwtSecret, rl)

	_, err := service.Authenticate(apiEmail, apiKey)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must be an API key")
}

func TestSMTPBridgeHandlerService_Authenticate_NoWorkspaceAccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	jwtSecret := []byte("test-secret-key-for-jwt-signing-minimum-32-chars")
	apiEmail := "api@example.com"

	claims := UserClaims{
		UserID: "api-user-123",
		Email:  apiEmail,
		Type:   string(domain.UserTypeAPIKey),
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	apiKey, _ := token.SignedString(jwtSecret)

	log := logger.NewLogger()
	rl := ratelimiter.NewRateLimiter()
	rl.SetPolicy("smtp", 5, 1*time.Minute)
	defer rl.Stop()
	mockRepo := mocks.NewMockWorkspaceRepository(ctrl)
	service := NewSMTPBridgeHandlerService(nil, nil, mockRepo, log, jwtSecret, rl)

	userID, err := service.Authenticate(apiEmail, apiKey)

	assert.NoError(t, err)
	assert.Equal(t, "api-user-123", userID)
}

func TestSMTPBridgeHandlerService_HandleMessage_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	jwtSecret := []byte("test-secret-key-for-jwt-signing-minimum-32-chars")
	userID := "api-user-123"
	workspaceID := "workspace123"

	// Create a simple email with JSON body
	emailBody := `From: sender@example.com
To: test@example.com
Subject: Test Email
Content-Type: text/plain

{
  "workspace_id": "workspace123",
  "notification": {
    "id": "password_reset",
    "contact": {
      "email": "user@example.com"
    },
    "data": {
      "reset_token": "abc123"
    }
  }
}`

	mockRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockRepo.EXPECT().
		GetUserWorkspace(gomock.Any(), userID, workspaceID).
		Return(&domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
		}, nil)

	mockTransactionalService := mocks.NewMockTransactionalNotificationService(ctrl)
	mockTransactionalService.EXPECT().
		SendNotification(gomock.Any(), workspaceID, gomock.Any()).
		DoAndReturn(func(ctx context.Context, wid string, params domain.TransactionalNotificationSendParams) (string, error) {
			assert.Equal(t, "password_reset", params.ID)
			assert.Equal(t, "user@example.com", params.Contact.Email)
			return "msg-123", nil
		})

	log := logger.NewLogger()
	rl := ratelimiter.NewRateLimiter()
	rl.SetPolicy("smtp", 5, 1*time.Minute)
	defer rl.Stop()
	service := NewSMTPBridgeHandlerService(nil, mockTransactionalService, mockRepo, log, jwtSecret, rl)

	err := service.HandleMessage(userID, "sender@example.com", []string{"test@example.com"}, []byte(emailBody))

	assert.NoError(t, err)
}

func TestSMTPBridgeHandlerService_HandleMessage_InvalidJSON(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	jwtSecret := []byte("test-secret-key-for-jwt-signing-minimum-32-chars")
	userID := "api-user-123"

	emailBody := `From: sender@example.com
To: test@example.com
Subject: Test Email
Content-Type: text/plain

This is not JSON`

	mockRepo := mocks.NewMockWorkspaceRepository(ctrl)
	log := logger.NewLogger()
	rl := ratelimiter.NewRateLimiter()
	rl.SetPolicy("smtp", 5, 1*time.Minute)
	defer rl.Stop()
	service := NewSMTPBridgeHandlerService(nil, nil, mockRepo, log, jwtSecret, rl)

	err := service.HandleMessage(userID, "sender@example.com", []string{"test@example.com"}, []byte(emailBody))

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not valid JSON")
}

func TestSMTPBridgeHandlerService_HandleMessage_MissingNotificationID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	jwtSecret := []byte("test-secret-key-for-jwt-signing-minimum-32-chars")
	userID := "api-user-123"
	workspaceID := "workspace123"

	emailBody := `From: sender@example.com
To: test@example.com
Subject: Test Email
Content-Type: text/plain

{
  "workspace_id": "workspace123",
  "notification": {
    "contact": {
      "email": "user@example.com"
    }
  }
}`

	mockRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockRepo.EXPECT().
		GetUserWorkspace(gomock.Any(), userID, workspaceID).
		Return(&domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
		}, nil)

	log := logger.NewLogger()
	rl := ratelimiter.NewRateLimiter()
	rl.SetPolicy("smtp", 5, 1*time.Minute)
	defer rl.Stop()
	service := NewSMTPBridgeHandlerService(nil, nil, mockRepo, log, jwtSecret, rl)

	err := service.HandleMessage(userID, "sender@example.com", []string{"test@example.com"}, []byte(emailBody))

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "notification.id is required")
}

func TestSMTPBridgeHandlerService_HandleMessage_MissingContact(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	jwtSecret := []byte("test-secret-key-for-jwt-signing-minimum-32-chars")
	userID := "api-user-123"
	workspaceID := "workspace123"

	emailBody := `From: sender@example.com
To: test@example.com
Subject: Test Email
Content-Type: text/plain

{
  "workspace_id": "workspace123",
  "notification": {
    "id": "test"
  }
}`

	mockRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockRepo.EXPECT().
		GetUserWorkspace(gomock.Any(), userID, workspaceID).
		Return(&domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
		}, nil)

	log := logger.NewLogger()
	rl := ratelimiter.NewRateLimiter()
	rl.SetPolicy("smtp", 5, 1*time.Minute)
	defer rl.Stop()
	service := NewSMTPBridgeHandlerService(nil, nil, mockRepo, log, jwtSecret, rl)

	err := service.HandleMessage(userID, "sender@example.com", []string{"test@example.com"}, []byte(emailBody))

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "notification.contact is required")
}

func TestSMTPBridgeHandlerService_HandleMessage_WithEmailHeaders(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	userID := "api-user-123"
	workspaceID := "workspace123"

	// Setup mocks
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockWorkspaceRepo.EXPECT().
		GetUserWorkspace(gomock.Any(), userID, workspaceID).
		Return(&domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
		}, nil)

	mockTransactionalService := mocks.NewMockTransactionalNotificationService(ctrl)

	var capturedParams domain.TransactionalNotificationSendParams
	mockTransactionalService.EXPECT().
		SendNotification(gomock.Any(), workspaceID, gomock.Any()).
		DoAndReturn(func(ctx context.Context, wid string, params domain.TransactionalNotificationSendParams) (string, error) {
			capturedParams = params
			return "msg-123", nil
		})

	mockAuth := &AuthService{}
	log := logger.NewLogger()
	jwtSecret := []byte("test-secret")
	rl := ratelimiter.NewRateLimiter()
	rl.SetPolicy("smtp", 5, 1*time.Minute)
	defer rl.Stop()

	service := NewSMTPBridgeHandlerService(
		mockAuth,
		mockTransactionalService,
		mockWorkspaceRepo,
		log,
		jwtSecret,
		rl,
	)

	// Create test email with CC, BCC, and Reply-To headers
	emailData := `From: sender@example.com
To: recipient@example.com
Cc: cc1@example.com, CC User <cc2@example.com>
Bcc: bcc@example.com
Reply-To: replyto@example.com
Subject: Test
Content-Type: text/plain

{
  "workspace_id": "workspace123",
  "notification": {
    "id": "password_reset",
    "contact": {
      "email": "test@example.com"
    }
  }
}`

	// Handle message
	err := service.HandleMessage(userID, "sender@example.com", []string{"recipient@example.com"}, []byte(emailData))
	assert.NoError(t, err)

	// Verify CC was extracted
	assert.Len(t, capturedParams.EmailOptions.CC, 2)
	assert.Equal(t, "cc1@example.com", capturedParams.EmailOptions.CC[0])
	assert.Equal(t, "cc2@example.com", capturedParams.EmailOptions.CC[1])

	// Verify BCC was extracted
	assert.Len(t, capturedParams.EmailOptions.BCC, 1)
	assert.Equal(t, "bcc@example.com", capturedParams.EmailOptions.BCC[0])

	// Verify Reply-To was extracted
	assert.Equal(t, "replyto@example.com", capturedParams.EmailOptions.ReplyTo)
}

func TestSMTPBridgeHandlerService_HandleMessage_JSONOverridesHeaders(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	userID := "api-user-123"
	workspaceID := "workspace123"

	// Setup mocks
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockWorkspaceRepo.EXPECT().
		GetUserWorkspace(gomock.Any(), userID, workspaceID).
		Return(&domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
		}, nil)

	mockTransactionalService := mocks.NewMockTransactionalNotificationService(ctrl)

	var capturedParams domain.TransactionalNotificationSendParams
	mockTransactionalService.EXPECT().
		SendNotification(gomock.Any(), workspaceID, gomock.Any()).
		DoAndReturn(func(ctx context.Context, wid string, params domain.TransactionalNotificationSendParams) (string, error) {
			capturedParams = params
			return "msg-123", nil
		})

	mockAuth := &AuthService{}
	log := logger.NewLogger()
	jwtSecret := []byte("test-secret")
	rl := ratelimiter.NewRateLimiter()
	rl.SetPolicy("smtp", 5, 1*time.Minute)
	defer rl.Stop()

	service := NewSMTPBridgeHandlerService(
		mockAuth,
		mockTransactionalService,
		mockWorkspaceRepo,
		log,
		jwtSecret,
		rl,
	)

	// Create test email with headers AND JSON payload specifying email options
	emailData := `From: sender@example.com
To: recipient@example.com
Cc: header-cc@example.com
Reply-To: header-reply@example.com
Subject: Test
Content-Type: text/plain

{
  "workspace_id": "workspace123",
  "notification": {
    "id": "password_reset",
    "contact": {
      "email": "test@example.com"
    },
    "email_options": {
      "cc": ["json-cc@example.com"],
      "reply_to": "json-reply@example.com"
    }
  }
}`

	// Handle message
	err := service.HandleMessage(userID, "sender@example.com", []string{"recipient@example.com"}, []byte(emailData))
	assert.NoError(t, err)

	// Verify JSON payload took precedence
	assert.Len(t, capturedParams.EmailOptions.CC, 1)
	assert.Equal(t, "json-cc@example.com", capturedParams.EmailOptions.CC[0])
	assert.Equal(t, "json-reply@example.com", capturedParams.EmailOptions.ReplyTo)
}

func TestParseEmailAddresses(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "Single email",
			input:    "user@example.com",
			expected: []string{"user@example.com"},
		},
		{
			name:     "Multiple emails",
			input:    "user1@example.com, user2@example.com",
			expected: []string{"user1@example.com", "user2@example.com"},
		},
		{
			name:     "Email with name",
			input:    "John Doe <john@example.com>",
			expected: []string{"john@example.com"},
		},
		{
			name:     "Mixed format",
			input:    "user1@example.com, John Doe <john@example.com>, user3@example.com",
			expected: []string{"user1@example.com", "john@example.com", "user3@example.com"},
		},
		{
			name:     "Empty string",
			input:    "",
			expected: []string{},
		},
		{
			name:     "With extra spaces",
			input:    "  user1@example.com  ,  user2@example.com  ",
			expected: []string{"user1@example.com", "user2@example.com"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseEmailAddresses(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSMTPBridgeHandlerService_ExtractJSONPayload_Multipart(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	jwtSecret := []byte("test-secret-key-for-jwt-signing-minimum-32-chars")

	// Create a multipart email
	emailBody := `From: sender@example.com
To: test@example.com
Subject: Test Email
MIME-Version: 1.0
Content-Type: multipart/mixed; boundary="boundary123"

--boundary123
Content-Type: text/plain

{"notification": {"id": "test", "contact": {"email": "user@example.com"}}}
--boundary123--
`

	mockRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockTransactionalService := mocks.NewMockTransactionalNotificationService(ctrl)
	log := logger.NewLogger()
	rl := ratelimiter.NewRateLimiter()
	rl.SetPolicy("smtp", 5, 1*time.Minute)
	defer rl.Stop()
	service := NewSMTPBridgeHandlerService(nil, mockTransactionalService, mockRepo, log, jwtSecret, rl)

	// Parse the email
	msg, err := mail.ReadMessage(bytes.NewReader([]byte(emailBody)))
	assert.NoError(t, err)

	jsonPayload, err := service.extractJSONPayload(msg)
	assert.NoError(t, err)
	assert.Contains(t, string(jsonPayload), `"id": "test"`)
}

// TestSMTPBridgeHandlerService_HandleMessage_SeedsAuthContext pins the shape of the
// context the bridge hands to SendNotification. The bridge seeds the identity it has
// already resolved rather than stamping SystemCallKey, which is what makes the send run
// the same permission gate as the HTTP path instead of skipping authorization. The
// workspace-scoped user key must be keyed on the workspace the send is for: keyed on
// anything else the short-circuit misses and authentication falls through to a context
// built from context.Background(), which carries no user at all.
func TestSMTPBridgeHandlerService_HandleMessage_SeedsAuthContext(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	jwtSecret := []byte("test-secret-key-for-jwt-signing-minimum-32-chars")
	userID := "api-user-123"
	workspaceID := "workspace123"

	emailBody := `From: sender@example.com
To: test@example.com
Subject: Test Email
Content-Type: text/plain

{
  "workspace_id": "workspace123",
  "notification": {
    "id": "password_reset",
    "contact": {"email": "user@example.com"}
  }
}`

	userWorkspace := &domain.UserWorkspace{
		UserID:      userID,
		WorkspaceID: workspaceID,
		Role:        "member",
		Permissions: domain.UserPermissions{
			domain.PermissionResourceTransactional: {Write: true},
		},
	}

	mockRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockRepo.EXPECT().
		GetUserWorkspace(gomock.Any(), userID, workspaceID).
		Return(userWorkspace, nil)

	mockTransactionalService := mocks.NewMockTransactionalNotificationService(ctrl)
	mockTransactionalService.EXPECT().
		SendNotification(gomock.Any(), workspaceID, gomock.Any()).
		DoAndReturn(func(ctx context.Context, _ string, _ domain.TransactionalNotificationSendParams) (string, error) {
			assert.Nil(t, ctx.Value(domain.SystemCallKey),
				"stamping SystemCallKey would disable every downstream permission check")

			seededUser, ok := ctx.Value(domain.WorkspaceUserKey(workspaceID)).(*domain.User)
			if assert.True(t, ok, "context must carry the workspace-scoped user for the workspace being sent to") {
				assert.Equal(t, userID, seededUser.ID)
				assert.Equal(t, domain.UserTypeAPIKey, seededUser.Type)
			}

			seededWorkspace, ok := ctx.Value(domain.UserWorkspaceKey).(*domain.UserWorkspace)
			if assert.True(t, ok, "context must carry the membership row the gate reads") {
				assert.Same(t, userWorkspace, seededWorkspace)
			}
			return "msg-123", nil
		})

	log := logger.NewLogger()
	rl := ratelimiter.NewRateLimiter()
	rl.SetPolicy("smtp", 5, 1*time.Minute)
	defer rl.Stop()
	service := NewSMTPBridgeHandlerService(nil, mockTransactionalService, mockRepo, log, jwtSecret, rl)

	err := service.HandleMessage(userID, "sender@example.com", []string{"test@example.com"}, []byte(emailBody))

	assert.NoError(t, err)
}

// TestSMTPBridgeHandlerService_HandleMessage_PermissionParity drives a real
// TransactionalNotificationService and a real AuthService behind the bridge, and asserts
// the SMTP path reaches the same verdict as the HTTP path for the same membership row.
// The bridge used to stamp SystemCallKey, which made every SMTP send unconditional
// regardless of the key's permissions; a mocked transactional service cannot catch a
// regression to that, because the gate lives in the service the mock replaces.
func TestSMTPBridgeHandlerService_HandleMessage_PermissionParity(t *testing.T) {
	const (
		userID         = "api-user-123"
		workspaceID    = "workspace123"
		notificationID = "password_reset"
		recipientEmail = "user@example.com"
		templateID     = "template-1"
	)

	emailBody := `From: sender@example.com
To: test@example.com
Subject: Test Email
Content-Type: text/plain

{
  "workspace_id": "workspace123",
  "notification": {
    "id": "password_reset",
    "contact": {"email": "user@example.com"}
  }
}`

	apiKeyUser := &domain.User{ID: userID, Email: "key@api.example.com", Type: domain.UserTypeAPIKey}

	workspace := &domain.Workspace{
		ID: workspaceID,
		Settings: domain.WorkspaceSettings{
			TransactionalEmailProviderID: "integration-1",
			SecretKey:                    "test-secret-key",
		},
		Integrations: []domain.Integration{{
			ID:   "integration-1",
			Type: "email",
			EmailProvider: domain.EmailProvider{
				Kind:      domain.EmailProviderKindSparkPost,
				Senders:   []domain.EmailSender{domain.NewEmailSender("sender@example.com", "Test Sender")},
				SparkPost: &domain.SparkPostSettings{EncryptedAPIKey: "encrypted-api-key"},
			},
		}},
	}

	notification := &domain.TransactionalNotification{
		ID:   notificationID,
		Name: "Password reset",
		Channels: map[domain.TransactionalChannel]domain.ChannelTemplate{
			domain.TransactionalChannelEmail: {TemplateID: templateID},
		},
	}

	cases := []struct {
		name        string
		permissions domain.UserPermissions
		allowed     bool
	}{
		{
			name:        "zero permission key",
			permissions: domain.UserPermissions{},
			allowed:     false,
		},
		{
			// The key a customer would build to send and nothing else.
			name: "send only key",
			permissions: domain.UserPermissions{
				domain.PermissionResourceTransactional: {Write: true},
			},
			allowed: true,
		},
		{
			// Everything but transactional: the resource that owns the send.
			name: "contacts only key",
			permissions: domain.UserPermissions{
				domain.PermissionResourceContacts: {Read: true, Write: true},
			},
			allowed: false,
		},
	}

	// sendViaSMTP and sendViaHTTP drive the same service through the two entry points.
	// Both set up their own mocks so the expectations describe exactly the calls their
	// path makes.
	sendViaSMTP := func(t *testing.T, permissions domain.UserPermissions, allowed bool) error {
		t.Helper()
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		deps := newParityDeps(t, ctrl, workspace, notification, allowed)
		deps.workspaceRepo.EXPECT().
			GetUserWorkspace(gomock.Any(), userID, workspaceID).
			Return(&domain.UserWorkspace{
				UserID:      userID,
				WorkspaceID: workspaceID,
				Role:        "member",
				Permissions: permissions,
			}, nil)

		rl := ratelimiter.NewRateLimiter()
		rl.SetPolicy("smtp", 5, 1*time.Minute)
		defer rl.Stop()

		bridge := NewSMTPBridgeHandlerService(nil, deps.transactionalService, deps.workspaceRepo,
			logger.NewLogger(), []byte("test-secret-key-for-jwt-signing-minimum-32-chars"), rl)

		return bridge.HandleMessage(userID, "sender@example.com", []string{"test@example.com"}, []byte(emailBody))
	}

	sendViaHTTP := func(t *testing.T, permissions domain.UserPermissions, allowed bool) error {
		t.Helper()
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		deps := newParityDeps(t, ctrl, workspace, notification, allowed)
		deps.authRepo.EXPECT().GetUserByID(gomock.Any(), userID).Return(apiKeyUser, nil)
		// AuthenticateUserForWorkspace resolves the workspace and the membership row,
		// then the send fetches the workspace again for its provider settings.
		deps.workspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(workspace, nil)
		deps.workspaceRepo.EXPECT().
			GetUserWorkspace(gomock.Any(), userID, workspaceID).
			Return(&domain.UserWorkspace{
				UserID:      userID,
				WorkspaceID: workspaceID,
				Role:        "member",
				Permissions: permissions,
			}, nil)

		// The context the auth middleware builds for an API key.
		ctx := context.WithValue(context.Background(), domain.UserIDKey, userID)
		ctx = context.WithValue(ctx, domain.UserTypeKey, string(domain.UserTypeAPIKey))

		_, err := deps.transactionalService.SendNotification(ctx, workspaceID, domain.TransactionalNotificationSendParams{
			ID:      notificationID,
			Contact: &domain.Contact{Email: recipientEmail},
		})
		return err
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			transports := map[string]func(*testing.T, domain.UserPermissions, bool) error{
				"smtp": sendViaSMTP,
				"http": sendViaHTTP,
			}

			for transport, send := range transports {
				t.Run(transport, func(t *testing.T) {
					err := send(t, tc.permissions, tc.allowed)

					if tc.allowed {
						assert.NoError(t, err)
						return
					}

					require.Error(t, err)
					var permErr *domain.PermissionError
					require.ErrorAs(t, err, &permErr)
					assert.Equal(t, domain.PermissionResourceTransactional, permErr.Resource)
					assert.Equal(t, domain.PermissionTypeWrite, permErr.Permission)
				})
			}
		})
	}
}

type parityDeps struct {
	transactionalService *TransactionalNotificationService
	workspaceRepo        *mocks.MockWorkspaceRepository
	authRepo             *mocks.MockAuthRepository
	// upsertedContact is what the send actually handed the contact service, which is
	// where the caller's posted body is either passed through or reduced to the email.
	upsertedContact *domain.Contact
}

// newParityDeps builds a real transactional service on a real auth service, with
// repository expectations for the send only when it is expected to get past the gate.
func newParityDeps(t *testing.T, ctrl *gomock.Controller, workspace *domain.Workspace, notification *domain.TransactionalNotification, allowed bool) *parityDeps {
	t.Helper()

	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	authRepo := mocks.NewMockAuthRepository(ctrl)
	transactionalRepo := mocks.NewMockTransactionalNotificationRepository(ctrl)
	contactService := mocks.NewMockContactService(ctrl)
	emailService := mocks.NewMockEmailServiceInterface(ctrl)
	log := logger.NewLogger()

	deps := &parityDeps{workspaceRepo: workspaceRepo, authRepo: authRepo}

	authService := NewAuthService(AuthServiceConfig{
		Repository:          authRepo,
		WorkspaceRepository: workspaceRepo,
		GetSecret:           func() ([]byte, error) { return []byte("test-secret-key-for-jwt-signing-minimum-32-chars"), nil },
		Logger:              log,
	})

	if allowed {
		workspaceRepo.EXPECT().GetByID(gomock.Any(), workspace.ID).Return(workspace, nil)
		transactionalRepo.EXPECT().Get(gomock.Any(), workspace.ID, notification.ID).Return(notification, nil)
		contactService.EXPECT().
			UpsertContact(gomock.Any(), workspace.ID, gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, contact *domain.Contact) domain.UpsertContactOperation {
				deps.upsertedContact = contact
				return domain.UpsertContactOperation{Action: domain.UpsertContactOperationUpdate}
			})
		contactService.EXPECT().
			GetContactByEmail(gomock.Any(), workspace.ID, gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, email string) (*domain.Contact, error) {
				return &domain.Contact{Email: email}, nil
			})
		emailService.EXPECT().SendEmailForTemplate(gomock.Any(), gomock.Any()).Return(nil)
	}

	deps.transactionalService = NewTransactionalNotificationService(
		transactionalRepo,
		mocks.NewMockMessageHistoryRepository(ctrl),
		mocks.NewMockTemplateService(ctrl),
		contactService,
		emailService,
		authService,
		log,
		workspaceRepo,
		"https://api.example.com",
	)

	return deps
}

// TestSMTPBridgeHandlerService_HandleMessage_ContactScope drives the bridge through a
// real transactional service to pin the SMTP half of the send-only key's contact
// containment. The bridge is the wider of the two entry points: it fills CC and BCC from
// the raw Cc:/Bcc: headers, so a credential that could name an extra recipient would get
// the rendered subject — Liquid-evaluated against the whole contact record — delivered
// somewhere the contact is not. A send-only credential setting those headers is refused;
// one that also holds contacts:read is not.
func TestSMTPBridgeHandlerService_HandleMessage_ContactScope(t *testing.T) {
	const (
		userID         = "api-user-123"
		workspaceID    = "workspace123"
		notificationID = "password_reset"
		recipientEmail = "victim@customer.com"
		templateID     = "template-1"
	)

	sendOnly := domain.UserPermissions{
		domain.PermissionResourceTransactional: {Write: true},
	}
	sendAndContacts := domain.UserPermissions{
		domain.PermissionResourceTransactional: {Write: true},
		domain.PermissionResourceContacts:      {Read: true, Write: true},
	}

	workspace := &domain.Workspace{
		ID: workspaceID,
		Settings: domain.WorkspaceSettings{
			TransactionalEmailProviderID: "integration-1",
			SecretKey:                    "test-secret-key",
		},
		Integrations: []domain.Integration{{
			ID:   "integration-1",
			Type: "email",
			EmailProvider: domain.EmailProvider{
				Kind:      domain.EmailProviderKindSparkPost,
				Senders:   []domain.EmailSender{domain.NewEmailSender("sender@example.com", "Test Sender")},
				SparkPost: &domain.SparkPostSettings{EncryptedAPIKey: "encrypted-api-key"},
			},
		}},
	}

	notification := &domain.TransactionalNotification{
		ID:   notificationID,
		Name: "Password reset",
		Channels: map[domain.TransactionalChannel]domain.ChannelTemplate{
			domain.TransactionalChannelEmail: {TemplateID: templateID},
		},
	}

	// A body carrying fields beyond the email, and a header naming a second reader.
	emailBody := func(bcc string) []byte {
		bccHeader := ""
		if bcc != "" {
			bccHeader = "Bcc: " + bcc + "\n"
		}
		return []byte("From: sender@example.com\n" +
			"To: test@example.com\n" +
			bccHeader +
			"Subject: Test Email\n" +
			"Content-Type: text/plain\n" +
			"\n" +
			`{
  "workspace_id": "workspace123",
  "notification": {
    "id": "password_reset",
    "contact": {"email": "victim@customer.com", "first_name": "Overwritten", "custom_string_1": "injected"}
  }
}`)
	}

	send := func(t *testing.T, permissions domain.UserPermissions, allowed bool, bcc string) (*parityDeps, error) {
		t.Helper()
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		deps := newParityDeps(t, ctrl, workspace, notification, allowed)
		deps.workspaceRepo.EXPECT().
			GetUserWorkspace(gomock.Any(), userID, workspaceID).
			Return(&domain.UserWorkspace{
				UserID:      userID,
				WorkspaceID: workspaceID,
				Role:        "member",
				Permissions: permissions,
			}, nil)

		rl := ratelimiter.NewRateLimiter()
		rl.SetPolicy("smtp", 5, 1*time.Minute)
		defer rl.Stop()

		bridge := NewSMTPBridgeHandlerService(nil, deps.transactionalService, deps.workspaceRepo,
			logger.NewLogger(), []byte("test-secret-key-for-jwt-signing-minimum-32-chars"), rl)

		return deps, bridge.HandleMessage(userID, "sender@example.com", []string{"test@example.com"}, emailBody(bcc))
	}

	t.Run("send-only credential upserts the email and nothing else", func(t *testing.T) {
		deps, err := send(t, sendOnly, true, "")
		require.NoError(t, err)

		require.NotNil(t, deps.upsertedContact)
		assert.Equal(t, recipientEmail, deps.upsertedContact.Email)
		assert.Nil(t, deps.upsertedContact.FirstName)
		assert.Nil(t, deps.upsertedContact.CustomString1)
	})

	t.Run("send-only credential cannot set bcc", func(t *testing.T) {
		_, err := send(t, sendOnly, false, "attacker@evil.example")

		require.Error(t, err)
		var permErr *domain.PermissionError
		require.ErrorAs(t, err, &permErr)
		assert.Equal(t, domain.PermissionResourceContacts, permErr.Resource)
		assert.Equal(t, domain.PermissionTypeRead, permErr.Permission)
	})

	t.Run("credential holding contacts read and write keeps both", func(t *testing.T) {
		deps, err := send(t, sendAndContacts, true, "archive@customer.com")
		require.NoError(t, err)

		require.NotNil(t, deps.upsertedContact)
		require.NotNil(t, deps.upsertedContact.FirstName)
		assert.Equal(t, "Overwritten", deps.upsertedContact.FirstName.String)
	})
}
