package service

import (
	"context"
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

const bridgeTestSecret = "test-secret-key-for-jwt-signing-minimum-32-chars"

// bridgeAPIKey signs an API key token for email with method and key.
func bridgeAPIKey(t *testing.T, email string, method jwt.SigningMethod, key interface{}, expiresAt time.Time) string {
	t.Helper()
	claims := UserClaims{
		UserID: "api-user-123",
		Email:  email,
		Type:   string(domain.UserTypeAPIKey),
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	signed, err := jwt.NewWithClaims(method, claims).SignedString(key)
	require.NoError(t, err)
	return signed
}

// newBridgeLimiter returns a limiter allowing maxAttempts per minute on the bridge.
func newBridgeLimiter(t *testing.T, maxAttempts int) *ratelimiter.RateLimiter {
	t.Helper()
	rl := ratelimiter.NewRateLimiter()
	rl.SetPolicy("smtp", maxAttempts, time.Minute)
	t.Cleanup(rl.Stop)
	return rl
}

// The refusals below happen before the account lookup: the bridge is built with
// a nil auth service, so reaching the lookup would panic the test.
func TestSMTPBridgeHandlerService_Authenticate_RefusedBeforeAccountLookup(t *testing.T) {
	apiEmail := "api@example.com"
	valid := time.Now().Add(time.Hour)

	cases := []struct {
		name     string
		username string
		apiKey   func(t *testing.T) string
		wantErr  string
	}{
		{
			name:     "unsigned token (alg none)",
			username: apiEmail,
			apiKey: func(t *testing.T) string {
				return bridgeAPIKey(t, apiEmail, jwt.SigningMethodNone, jwt.UnsafeAllowNoneSignatureType, valid)
			},
			wantErr: "invalid API key",
		},
		{
			name:     "expired token",
			username: apiEmail,
			apiKey: func(t *testing.T) string {
				return bridgeAPIKey(t, apiEmail, jwt.SigningMethodHS256, []byte(bridgeTestSecret), time.Now().Add(-time.Hour))
			},
			wantErr: "invalid API key",
		},
		{
			name:     "username is not the token's email",
			username: "someone-else@example.com",
			apiKey: func(t *testing.T) string {
				return bridgeAPIKey(t, apiEmail, jwt.SigningMethodHS256, []byte(bridgeTestSecret), valid)
			},
			wantErr: "email does not match token",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			service := NewSMTPBridgeHandlerService(nil, nil, mocks.NewMockWorkspaceRepository(ctrl),
				logger.NewLogger(), []byte(bridgeTestSecret), newBridgeLimiter(t, 5))

			userID, err := service.Authenticate(tc.username, tc.apiKey(t))

			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
			assert.Empty(t, userID)
		})
	}
}

// Once the limit is spent, even a valid key is refused, and no lookup is made.
func TestSMTPBridgeHandlerService_Authenticate_RateLimited(t *testing.T) {
	ctrl := gomock.NewController(t)
	apiEmail := "api@example.com"
	service := NewSMTPBridgeHandlerService(nil, nil, mocks.NewMockWorkspaceRepository(ctrl),
		logger.NewLogger(), []byte(bridgeTestSecret), newBridgeLimiter(t, 1))

	_, err := service.Authenticate(apiEmail, "not-a-token")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid API key")

	validKey := bridgeAPIKey(t, apiEmail, jwt.SigningMethodHS256, []byte(bridgeTestSecret), time.Now().Add(time.Hour))
	userID, err := service.Authenticate(apiEmail, validKey)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rate limit exceeded")
	assert.Empty(t, userID)
}

// A successful authentication clears the attempts, so a limit of one still lets
// the same key authenticate again.
func TestSMTPBridgeHandlerService_Authenticate_SuccessResetsTheLimit(t *testing.T) {
	ctrl := gomock.NewController(t)
	apiEmail := "api@example.com"
	authRepo := mocks.NewMockAuthRepository(ctrl)
	authRepo.EXPECT().GetUserByID(gomock.Any(), "api-user-123").
		Return(&domain.User{ID: "api-user-123", Email: apiEmail, Type: domain.UserTypeAPIKey}, nil).Times(2)
	authService := NewAuthService(AuthServiceConfig{
		Repository: authRepo,
		GetSecret:  func() ([]byte, error) { return []byte(bridgeTestSecret), nil },
		Logger:     logger.NewLogger(),
	})
	service := NewSMTPBridgeHandlerService(authService, nil, mocks.NewMockWorkspaceRepository(ctrl),
		logger.NewLogger(), []byte(bridgeTestSecret), newBridgeLimiter(t, 1))
	validKey := bridgeAPIKey(t, apiEmail, jwt.SigningMethodHS256, []byte(bridgeTestSecret), time.Now().Add(time.Hour))

	for attempt := 1; attempt <= 2; attempt++ {
		userID, err := service.Authenticate(apiEmail, validKey)
		require.NoError(t, err, "attempt %d", attempt)
		assert.Equal(t, "api-user-123", userID)
	}
}

func bridgeMessage(headers, body string) []byte {
	return []byte("From: sender@example.com\nTo: test@example.com\nSubject: Test\n" + headers + "Content-Type: text/plain\n\n" + body)
}

const bridgePayload = `{"workspace_id":"workspace123","notification":{"id":"password_reset","contact":{"email":"user@example.com"}}}`

// A message the bridge refuses never reaches the transactional service: it is
// built with a nil one, so a send would panic the test.
func TestSMTPBridgeHandlerService_HandleMessage_RefusedBeforeSending(t *testing.T) {
	t.Run("payload without workspace_id", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		service := NewSMTPBridgeHandlerService(nil, nil, mocks.NewMockWorkspaceRepository(ctrl),
			logger.NewLogger(), []byte(bridgeTestSecret), newBridgeLimiter(t, 5))

		err := service.HandleMessage("api-user-123", "sender@example.com", []string{"test@example.com"},
			bridgeMessage("", `{"notification":{"id":"password_reset","contact":{"email":"user@example.com"}}}`))

		require.Error(t, err)
		assert.Contains(t, err.Error(), "workspace_id is required")
	})

	t.Run("key has no access to the workspace", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		workspaceRepo.EXPECT().GetUserWorkspace(gomock.Any(), "api-user-123", "workspace123").
			Return(nil, domain.ErrUserNotInWorkspace)
		service := NewSMTPBridgeHandlerService(nil, nil, workspaceRepo,
			logger.NewLogger(), []byte(bridgeTestSecret), newBridgeLimiter(t, 5))

		err := service.HandleMessage("api-user-123", "sender@example.com", []string{"test@example.com"},
			bridgeMessage("", bridgePayload))

		require.Error(t, err)
		assert.Contains(t, err.Error(), "user does not have access to workspace")
		assert.ErrorIs(t, err, domain.ErrUserNotInWorkspace, "the repository's error is kept")
	})

	// Defensive: the repository answers ErrUserNotInWorkspace rather than nil, nil.
	t.Run("membership lookup returns nothing", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		workspaceRepo.EXPECT().GetUserWorkspace(gomock.Any(), "api-user-123", "workspace123").Return(nil, nil)
		service := NewSMTPBridgeHandlerService(nil, nil, workspaceRepo,
			logger.NewLogger(), []byte(bridgeTestSecret), newBridgeLimiter(t, 5))

		err := service.HandleMessage("api-user-123", "sender@example.com", []string{"test@example.com"},
			bridgeMessage("", bridgePayload))

		require.Error(t, err)
		assert.Contains(t, err.Error(), "user does not have access to workspace")
	})
}

// The Message-ID header is the idempotency key when the JSON names none, and the
// JSON's external_id wins when it does.
func TestSMTPBridgeHandlerService_HandleMessage_MessageIDAsExternalID(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		want    string
	}{
		{name: "no external_id in JSON", payload: bridgePayload, want: "abc-123@mail.example.com"},
		{
			name:    "external_id in JSON",
			payload: `{"workspace_id":"workspace123","notification":{"id":"password_reset","external_id":"from-json","contact":{"email":"user@example.com"}}}`,
			want:    "from-json",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
			workspaceRepo.EXPECT().GetUserWorkspace(gomock.Any(), "api-user-123", "workspace123").
				Return(&domain.UserWorkspace{UserID: "api-user-123", WorkspaceID: "workspace123", Role: "member"}, nil)
			transactional := mocks.NewMockTransactionalNotificationService(ctrl)
			var got *string
			transactional.EXPECT().SendNotification(gomock.Any(), "workspace123", gomock.Any()).
				DoAndReturn(func(_ context.Context, _ string, params domain.TransactionalNotificationSendParams) (string, error) {
					got = params.ExternalID
					return "msg-1", nil
				})
			service := NewSMTPBridgeHandlerService(nil, transactional, workspaceRepo,
				logger.NewLogger(), []byte(bridgeTestSecret), newBridgeLimiter(t, 5))

			err := service.HandleMessage("api-user-123", "sender@example.com", []string{"test@example.com"},
				bridgeMessage("Message-ID: <abc-123@mail.example.com>\n", tc.payload))

			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, tc.want, *got)
		})
	}
}
