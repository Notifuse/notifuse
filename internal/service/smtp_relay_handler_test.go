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
)

func TestSMTPRelayHandlerService_Authenticate_Success(t *testing.T) {
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
	service := NewSMTPRelayHandlerService(nil, nil, mockRepo, log, jwtSecret, rl)

	userID, err := service.Authenticate(apiEmail, apiKey)

	assert.NoError(t, err)
	assert.Equal(t, "api-user-123", userID)
}

func TestSMTPRelayHandlerService_Authenticate_InvalidToken(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	jwtSecret := []byte("test-secret-key-for-jwt-signing-minimum-32-chars")

	mockRepo := mocks.NewMockWorkspaceRepository(ctrl)
	log := logger.NewLogger()
	rl := ratelimiter.NewRateLimiter()
	rl.SetPolicy("smtp", 5, 1*time.Minute)
	defer rl.Stop()
	service := NewSMTPRelayHandlerService(nil, nil, mockRepo, log, jwtSecret, rl)

	_, err := service.Authenticate("workspace123", "invalid-token")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid API key")
}

func TestSMTPRelayHandlerService_Authenticate_WrongUserType(t *testing.T) {
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
	service := NewSMTPRelayHandlerService(nil, nil, mockRepo, log, jwtSecret, rl)

	_, err := service.Authenticate(apiEmail, apiKey)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must be an API key")
}

func TestSMTPRelayHandlerService_Authenticate_NoWorkspaceAccess(t *testing.T) {
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
	service := NewSMTPRelayHandlerService(nil, nil, mockRepo, log, jwtSecret, rl)

	userID, err := service.Authenticate(apiEmail, apiKey)

	assert.NoError(t, err)
	assert.Equal(t, "api-user-123", userID)
}

func TestSMTPRelayHandlerService_HandleMessage_Success(t *testing.T) {
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
	service := NewSMTPRelayHandlerService(nil, mockTransactionalService, mockRepo, log, jwtSecret, rl)

	err := service.HandleMessage(userID, "sender@example.com", []string{"test@example.com"}, []byte(emailBody))

	assert.NoError(t, err)
}

func TestSMTPRelayHandlerService_HandleMessage_InvalidJSON(t *testing.T) {
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
	service := NewSMTPRelayHandlerService(nil, nil, mockRepo, log, jwtSecret, rl)

	err := service.HandleMessage(userID, "sender@example.com", []string{"test@example.com"}, []byte(emailBody))

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not valid JSON")
}

func TestSMTPRelayHandlerService_HandleMessage_MissingNotificationID(t *testing.T) {
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
	service := NewSMTPRelayHandlerService(nil, nil, mockRepo, log, jwtSecret, rl)

	err := service.HandleMessage(userID, "sender@example.com", []string{"test@example.com"}, []byte(emailBody))

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "notification.id is required")
}

func TestSMTPRelayHandlerService_HandleMessage_MissingContact(t *testing.T) {
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
	service := NewSMTPRelayHandlerService(nil, nil, mockRepo, log, jwtSecret, rl)

	err := service.HandleMessage(userID, "sender@example.com", []string{"test@example.com"}, []byte(emailBody))

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "notification.contact is required")
}

func TestSMTPRelayHandlerService_HandleMessage_WithEmailHeaders(t *testing.T) {
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

	service := NewSMTPRelayHandlerService(
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

func TestSMTPRelayHandlerService_HandleMessage_JSONOverridesHeaders(t *testing.T) {
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

	service := NewSMTPRelayHandlerService(
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

func TestSMTPRelayHandlerService_ExtractJSONPayload_Multipart(t *testing.T) {
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
	service := NewSMTPRelayHandlerService(nil, mockTransactionalService, mockRepo, log, jwtSecret, rl)

	// Parse the email
	msg, err := mail.ReadMessage(bytes.NewReader([]byte(emailBody)))
	assert.NoError(t, err)

	jsonPayload, err := service.extractJSONPayload(msg)
	assert.NoError(t, err)
	assert.Contains(t, string(jsonPayload), `"id": "test"`)
}
