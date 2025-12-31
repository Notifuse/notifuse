package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/pkg/crypto"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/Notifuse/notifuse/pkg/ratelimiter"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

type mockEmailSender struct {
	shouldError bool
}

func (m *mockEmailSender) SendMagicCode(email, code string) error {
	if m.shouldError {
		return errors.New("mock error")
	}
	return nil
}

func setupUserTest(t *testing.T) (
	*mocks.MockUserRepository,
	*mocks.MockAuthService,
	*UserService,
	*mockEmailSender,
) {
	ctrl := gomock.NewController(t)
	mockRepo := mocks.NewMockUserRepository(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockTracer := pkgmocks.NewMockTracer(ctrl)
	mockSender := &mockEmailSender{}

	// Create mock logger with AnyTimes to ignore logging
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Fatal(gomock.Any()).AnyTimes()

	// Create mock tracer with AnyTimes to ignore tracing
	mockTracer.EXPECT().StartServiceSpan(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, serviceName, methodName string) (context.Context, *interface{}) {
			return ctx, nil
		}).AnyTimes()
	mockTracer.EXPECT().AddAttribute(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	mockTracer.EXPECT().MarkSpanError(gomock.Any(), gomock.Any()).AnyTimes()
	mockTracer.EXPECT().TraceMethodWithResultAny(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, service, method string, f func(context.Context) (interface{}, error)) (interface{}, error) {
			return f(ctx)
		}).AnyTimes()
	mockTracer.EXPECT().EndSpan(gomock.Any(), gomock.Any()).AnyTimes()
	mockTracer.EXPECT().StartSpan(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, name string) (context.Context, *interface{}) {
			return ctx, nil
		}).AnyTimes()
	mockTracer.EXPECT().StartSpanWithAttributes(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, name string, attrs ...interface{}) (context.Context, *interface{}) {
			return ctx, nil
		}).AnyTimes()
	mockTracer.EXPECT().TraceMethod(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, service, method string, f func(context.Context) error) error {
			return f(ctx)
		}).AnyTimes()
	mockTracer.EXPECT().WrapHTTPClient(gomock.Any()).
		DoAndReturn(func(client *interface{}) *interface{} {
			return client
		}).AnyTimes()

	// Create rate limiter with policies for testing
	rl := ratelimiter.NewRateLimiter()
	rl.SetPolicy("signin", 5, 5*time.Minute)
	rl.SetPolicy("verify", 5, 5*time.Minute)

	service, err := NewUserService(UserServiceConfig{
		Repository:    mockRepo,
		AuthService:   mockAuthService,
		EmailSender:   mockSender,
		SessionExpiry: 24 * time.Hour,
		Logger:        mockLogger,
		IsProduction:  true,
		Tracer:        mockTracer,
		RateLimiter:   rl,
		SecretKey:     "test-secret-key-for-hmac-verification",
		RootEmail:     "root@example.com",
	})
	require.NoError(t, err)

	return mockRepo, mockAuthService, service, mockSender
}

func TestUserService_SignIn(t *testing.T) {
	mockRepo, _, service, mockSender := setupUserTest(t)

	email := "test@example.com"

	t.Run("successful sign in - existing user", func(t *testing.T) {
		mockSender.shouldError = true // Force email sending error for logging
		user := &domain.User{
			ID:    "user123",
			Email: email,
		}

		mockRepo.EXPECT().
			GetUserByEmail(gomock.Any(), email).
			Return(user, nil)

		mockRepo.EXPECT().
			CreateSession(gomock.Any(), gomock.Any()).
			Return(nil)

		code, err := service.SignIn(context.Background(), domain.SignInInput{Email: email})
		require.Error(t, err)
		require.Equal(t, "mock error", err.Error())
		require.Empty(t, code)
	})

	t.Run("sign in fails - user does not exist", func(t *testing.T) {
		mockRepo.EXPECT().
			GetUserByEmail(gomock.Any(), email).
			Return(nil, &domain.ErrUserNotFound{})

		code, err := service.SignIn(context.Background(), domain.SignInInput{Email: email})
		require.Error(t, err)
		require.Equal(t, "user does not exist", err.Error())
		require.Empty(t, code)

		// Verify it's the correct error type
		_, ok := err.(*domain.ErrUserNotFound)
		require.True(t, ok, "Expected ErrUserNotFound error type")
	})

	t.Run("development mode returns code directly", func(t *testing.T) {
		service.isProduction = false
		mockSender.shouldError = false // No email sending in dev mode
		user := &domain.User{
			ID:    "user123",
			Email: email,
		}

		mockRepo.EXPECT().
			GetUserByEmail(gomock.Any(), email).
			Return(user, nil)

		mockRepo.EXPECT().
			CreateSession(gomock.Any(), gomock.Any()).
			Return(nil)

		code, err := service.SignIn(context.Background(), domain.SignInInput{Email: email})
		require.NoError(t, err)
		require.NotEmpty(t, code)
		require.Len(t, code, 6) // Should be 6 digits
	})

	t.Run("repository error", func(t *testing.T) {
		mockRepo.EXPECT().
			GetUserByEmail(gomock.Any(), email).
			Return(nil, errors.New("db error"))

		code, err := service.SignIn(context.Background(), domain.SignInInput{Email: email})
		require.Error(t, err)
		require.Empty(t, code)
	})
}

func TestUserService_VerifyCode(t *testing.T) {
	mockRepo, mockAuthService, service, _ := setupUserTest(t)

	email := "test@example.com"
	code := "123456"
	userID := "user123"

	t.Run("successful verification", func(t *testing.T) {
		user := &domain.User{
			ID:    userID,
			Email: email,
		}

		// Use the secret key from the service to hash the magic code
		hashedCode := crypto.HashMagicCode(code, "test-secret-key-for-hmac-verification")
		magicCodeExpires := time.Now().Add(15 * time.Minute)

		session := &domain.Session{
			ID:               "session123",
			UserID:           userID,
			MagicCode:        &hashedCode,
			MagicCodeExpires: &magicCodeExpires,
			ExpiresAt:        time.Now().Add(24 * time.Hour),
		}

		mockRepo.EXPECT().
			GetUserByEmail(gomock.Any(), email).
			Return(user, nil)

		mockRepo.EXPECT().
			GetSessionsByUserID(gomock.Any(), userID).
			Return([]*domain.Session{session}, nil)

		mockRepo.EXPECT().
			UpdateSession(gomock.Any(), gomock.Any()).
			Return(nil)

		mockAuthService.EXPECT().
			GenerateUserAuthToken(user, session.ID, session.ExpiresAt).
			Return("token123")

		result, err := service.VerifyCode(context.Background(), domain.VerifyCodeInput{
			Email: email,
			Code:  code,
		})

		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, "token123", result.Token)
		require.Equal(t, *user, result.User)
		require.Equal(t, session.ExpiresAt, result.ExpiresAt)
	})

	t.Run("invalid code", func(t *testing.T) {
		user := &domain.User{
			ID:    userID,
			Email: email,
		}

		magicCode2 := "654321"
		magicCodeExpires2 := time.Now().Add(15 * time.Minute)
		session := &domain.Session{
			ID:               "session123",
			UserID:           userID,
			MagicCode:        &magicCode2,
			MagicCodeExpires: &magicCodeExpires2,
		}

		mockRepo.EXPECT().
			GetUserByEmail(gomock.Any(), email).
			Return(user, nil)

		mockRepo.EXPECT().
			GetSessionsByUserID(gomock.Any(), userID).
			Return([]*domain.Session{session}, nil)

		result, err := service.VerifyCode(context.Background(), domain.VerifyCodeInput{
			Email: email,
			Code:  code,
		})

		require.Error(t, err)
		require.Nil(t, result)
		require.Equal(t, "invalid magic code", err.Error())
	})

	t.Run("expired code", func(t *testing.T) {
		user := &domain.User{
			ID:    userID,
			Email: email,
		}

		// Use the secret key from the service to hash the magic code
		hashedCode := crypto.HashMagicCode(code, "test-secret-key-for-hmac-verification")
		expiredTime := time.Now().Add(-1 * time.Minute)

		session := &domain.Session{
			ID:               "session123",
			UserID:           userID,
			MagicCode:        &hashedCode,
			MagicCodeExpires: &expiredTime,
		}

		mockRepo.EXPECT().
			GetUserByEmail(gomock.Any(), email).
			Return(user, nil)

		mockRepo.EXPECT().
			GetSessionsByUserID(gomock.Any(), userID).
			Return([]*domain.Session{session}, nil)

		result, err := service.VerifyCode(context.Background(), domain.VerifyCodeInput{
			Email: email,
			Code:  code,
		})

		require.Error(t, err)
		require.Nil(t, result)
		require.Equal(t, "magic code expired", err.Error())
	})
}

func TestUserService_VerifyUserSession(t *testing.T) {
	mockRepo, _, service, _ := setupUserTest(t)

	userID := "user123"
	sessionID := "session123"

	t.Run("successful verification", func(t *testing.T) {
		session := &domain.Session{
			ID:        sessionID,
			UserID:    userID,
			ExpiresAt: time.Now().Add(1 * time.Hour),
		}

		user := &domain.User{
			ID:    userID,
			Email: "test@example.com",
		}

		mockRepo.EXPECT().
			GetSessionByID(gomock.Any(), sessionID).
			Return(session, nil)

		mockRepo.EXPECT().
			GetUserByID(gomock.Any(), userID).
			Return(user, nil)

		result, err := service.VerifyUserSession(context.Background(), userID, sessionID)

		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, userID, result.ID)
	})

	t.Run("session not found", func(t *testing.T) {
		mockRepo.EXPECT().
			GetSessionByID(gomock.Any(), sessionID).
			Return(nil, errors.New("session not found"))

		result, err := service.VerifyUserSession(context.Background(), userID, sessionID)

		require.Error(t, err)
		require.Nil(t, result)
	})

	t.Run("session expired", func(t *testing.T) {
		session := &domain.Session{
			ID:        sessionID,
			UserID:    userID,
			ExpiresAt: time.Now().Add(-1 * time.Hour),
		}

		mockRepo.EXPECT().
			GetSessionByID(gomock.Any(), sessionID).
			Return(session, nil)

		result, err := service.VerifyUserSession(context.Background(), userID, sessionID)

		require.Error(t, err)
		require.Nil(t, result)
		require.Equal(t, ErrSessionExpired, err)
	})
}

func TestUserService_GetUserByID(t *testing.T) {
	mockRepo, _, service, _ := setupUserTest(t)

	userID := "user123"

	t.Run("successful retrieval", func(t *testing.T) {
		user := &domain.User{
			ID:    userID,
			Email: "test@example.com",
		}

		mockRepo.EXPECT().
			GetUserByID(gomock.Any(), userID).
			Return(user, nil)

		result, err := service.GetUserByID(context.Background(), userID)

		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, userID, result.ID)
	})

	t.Run("user not found", func(t *testing.T) {
		mockRepo.EXPECT().
			GetUserByID(gomock.Any(), userID).
			Return(nil, errors.New("user not found"))

		result, err := service.GetUserByID(context.Background(), userID)

		require.Error(t, err)
		require.Nil(t, result)
	})
}

func TestUserService_GetUserByEmail(t *testing.T) {
	mockRepo, _, service, _ := setupUserTest(t)

	email := "test@example.com"

	t.Run("successful retrieval", func(t *testing.T) {
		user := &domain.User{
			ID:    "user123",
			Email: email,
		}

		mockRepo.EXPECT().
			GetUserByEmail(gomock.Any(), email).
			Return(user, nil)

		result, err := service.GetUserByEmail(context.Background(), email)

		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, email, result.Email)
	})

	t.Run("user not found", func(t *testing.T) {
		mockRepo.EXPECT().
			GetUserByEmail(gomock.Any(), email).
			Return(nil, &domain.ErrUserNotFound{Message: "user not found"})

		result, err := service.GetUserByEmail(context.Background(), email)

		require.Error(t, err)
		require.Nil(t, result)
		// Verify it's the correct error type
		_, ok := err.(*domain.ErrUserNotFound)
		require.True(t, ok, "Expected ErrUserNotFound error type")
	})
}

func TestUserService_RootSignin(t *testing.T) {
	mockRepo, mockAuthService, service, _ := setupUserTest(t)

	rootEmail := "root@example.com"
	secretKey := "test-secret-key-for-hmac-verification"

	t.Run("successful root signin with valid HMAC", func(t *testing.T) {
		timestamp := time.Now().Unix()
		message := fmt.Sprintf("%s:%d", rootEmail, timestamp)
		signature := crypto.ComputeHMAC256([]byte(message), secretKey)

		user := &domain.User{
			ID:    "root-user-id",
			Email: rootEmail,
		}

		mockRepo.EXPECT().
			GetUserByEmail(gomock.Any(), rootEmail).
			Return(user, nil)

		mockRepo.EXPECT().
			CreateSession(gomock.Any(), gomock.Any()).
			Return(nil)

		mockAuthService.EXPECT().
			GenerateUserAuthToken(user, gomock.Any(), gomock.Any()).
			Return("jwt-token")

		result, err := service.RootSignin(context.Background(), domain.RootSigninInput{
			Email:     rootEmail,
			Timestamp: timestamp,
			Signature: signature,
		})

		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, "jwt-token", result.Token)
		require.Equal(t, rootEmail, result.User.Email)
	})

	t.Run("fails with wrong email (not root)", func(t *testing.T) {
		timestamp := time.Now().Unix()
		wrongEmail := "notroot@example.com"
		message := fmt.Sprintf("%s:%d", wrongEmail, timestamp)
		signature := crypto.ComputeHMAC256([]byte(message), secretKey)

		result, err := service.RootSignin(context.Background(), domain.RootSigninInput{
			Email:     wrongEmail,
			Timestamp: timestamp,
			Signature: signature,
		})

		require.Error(t, err)
		require.Nil(t, result)
		require.Contains(t, err.Error(), "invalid credentials")
	})

	t.Run("fails with expired timestamp (too old)", func(t *testing.T) {
		// Timestamp more than 60 seconds ago
		timestamp := time.Now().Unix() - 120
		message := fmt.Sprintf("%s:%d", rootEmail, timestamp)
		signature := crypto.ComputeHMAC256([]byte(message), secretKey)

		result, err := service.RootSignin(context.Background(), domain.RootSigninInput{
			Email:     rootEmail,
			Timestamp: timestamp,
			Signature: signature,
		})

		require.Error(t, err)
		require.Nil(t, result)
		require.Contains(t, err.Error(), "invalid credentials")
	})

	t.Run("fails with future timestamp (too far ahead)", func(t *testing.T) {
		// Timestamp more than 60 seconds in the future
		timestamp := time.Now().Unix() + 120
		message := fmt.Sprintf("%s:%d", rootEmail, timestamp)
		signature := crypto.ComputeHMAC256([]byte(message), secretKey)

		result, err := service.RootSignin(context.Background(), domain.RootSigninInput{
			Email:     rootEmail,
			Timestamp: timestamp,
			Signature: signature,
		})

		require.Error(t, err)
		require.Nil(t, result)
		require.Contains(t, err.Error(), "invalid credentials")
	})

	t.Run("fails with invalid signature", func(t *testing.T) {
		timestamp := time.Now().Unix()

		result, err := service.RootSignin(context.Background(), domain.RootSigninInput{
			Email:     rootEmail,
			Timestamp: timestamp,
			Signature: "invalid-signature-abc123",
		})

		require.Error(t, err)
		require.Nil(t, result)
		require.Contains(t, err.Error(), "invalid credentials")
	})

	t.Run("fails when root user not found in database", func(t *testing.T) {
		timestamp := time.Now().Unix()
		message := fmt.Sprintf("%s:%d", rootEmail, timestamp)
		signature := crypto.ComputeHMAC256([]byte(message), secretKey)

		mockRepo.EXPECT().
			GetUserByEmail(gomock.Any(), rootEmail).
			Return(nil, &domain.ErrUserNotFound{Message: "user not found"})

		result, err := service.RootSignin(context.Background(), domain.RootSigninInput{
			Email:     rootEmail,
			Timestamp: timestamp,
			Signature: signature,
		})

		require.Error(t, err)
		require.Nil(t, result)
		require.Contains(t, err.Error(), "invalid credentials")
	})

	t.Run("fails when session creation fails", func(t *testing.T) {
		timestamp := time.Now().Unix()
		message := fmt.Sprintf("%s:%d", rootEmail, timestamp)
		signature := crypto.ComputeHMAC256([]byte(message), secretKey)

		user := &domain.User{
			ID:    "root-user-id",
			Email: rootEmail,
		}

		mockRepo.EXPECT().
			GetUserByEmail(gomock.Any(), rootEmail).
			Return(user, nil)

		mockRepo.EXPECT().
			CreateSession(gomock.Any(), gomock.Any()).
			Return(errors.New("database error"))

		result, err := service.RootSignin(context.Background(), domain.RootSigninInput{
			Email:     rootEmail,
			Timestamp: timestamp,
			Signature: signature,
		})

		require.Error(t, err)
		require.Nil(t, result)
		require.Contains(t, err.Error(), "failed to create session")
	})
}

func TestUserService_Logout(t *testing.T) {
	// Test UserService.Logout - this was at 0% coverage
	mockRepo, _, service, _ := setupUserTest(t)

	ctx := context.Background()
	userID := "user123"

	t.Run("Success - Logs out user", func(t *testing.T) {
		mockRepo.EXPECT().
			DeleteAllSessionsByUserID(ctx, userID).
			Return(nil)

		err := service.Logout(ctx, userID)
		require.NoError(t, err)
	})

	t.Run("Error - Repository error", func(t *testing.T) {
		mockRepo.EXPECT().
			DeleteAllSessionsByUserID(ctx, userID).
			Return(errors.New("database error"))

		err := service.Logout(ctx, userID)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to logout")
	})
}
