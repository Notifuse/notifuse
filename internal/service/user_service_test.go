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

func (m *mockEmailSender) SendMagicCode(email, code, language string) error {
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
	return setupUserTestWithRootEmail(t, "root@example.com")
}

// userTxMocks are the sign-in transaction's dependencies of a UserService built by
// setupUserTestWithRootEmail.
type userTxMocks struct {
	tx        *fakeTx
	authTx    *mocks.MockAuthTxRepository
	twoFactor *mocks.MockTwoFactorRepository
}

var userServiceTxMocks = map[*UserService]*userTxMocks{}

func txMocksOf(s *UserService) *userTxMocks { return userServiceTxMocks[s] }

func setupUserTestWithRootEmail(t *testing.T, rootEmail string) (
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
	txMocks := &userTxMocks{
		tx:        &fakeTx{},
		authTx:    mocks.NewMockAuthTxRepository(ctrl),
		twoFactor: mocks.NewMockTwoFactorRepository(ctrl),
	}

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
		RootEmail:     rootEmail,
		Tx:            txMocks.tx,
		AuthTx:        txMocks.authTx,
		TwoFactorRepo: txMocks.twoFactor,
	})
	require.NoError(t, err)
	userServiceTxMocks[service] = txMocks
	t.Cleanup(func() { delete(userServiceTxMocks, service) })

	return mockRepo, mockAuthService, service, mockSender
}

func TestUserService_UpdateUserLanguage(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mockRepo, _, service, _ := setupUserTest(t)

		mockRepo.EXPECT().
			UpdateUserLanguage(gomock.Any(), "user123", "fr").
			Return(nil)

		err := service.UpdateUserLanguage(context.Background(), "user123", "fr")
		require.NoError(t, err)
	})

	t.Run("rejects unsupported language", func(t *testing.T) {
		_, _, service, _ := setupUserTest(t)

		err := service.UpdateUserLanguage(context.Background(), "user123", "xx")
		var unsupported *domain.ErrUnsupportedLanguage
		require.ErrorAs(t, err, &unsupported)
		require.Equal(t, "xx", unsupported.Language)
	})

	t.Run("repository error", func(t *testing.T) {
		mockRepo, _, service, _ := setupUserTest(t)

		mockRepo.EXPECT().
			UpdateUserLanguage(gomock.Any(), "user123", "de").
			Return(errors.New("database error"))

		err := service.UpdateUserLanguage(context.Background(), "user123", "de")
		require.Error(t, err)
	})
}

// expectSignInSession: a sign-in purges the user's stale pending sessions and creates the
// one waiting for the code, under the user's lock.
func expectSignInSession(m *userTxMocks, userID string) {
	lock := m.authTx.EXPECT().LockUser(gomock.Any(), gomock.Any(), userID).Return(nil)
	purge := m.authTx.EXPECT().PurgeExpiredPendingSessions(gomock.Any(), gomock.Any(), userID).Return(nil).After(lock)
	m.authTx.EXPECT().CreateSession(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ domain.DBExecutor, s *domain.Session) error {
			if s.MagicCode == nil || s.AuthMethod != "" {
				return errors.New("a sign-in session holds a code and no method yet")
			}
			return nil
		}).After(purge)
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

		expectSignInSession(txMocksOf(service), "user123")

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

		expectSignInSession(txMocksOf(service), "user123")

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

func TestUserService_SignIn_RefusesAPIKeyIdentity(t *testing.T) {
	mockRepo, _, service, mockSender := setupUserTest(t)

	// Non-production is where the plain magic code comes back in the response
	// body, so that is the configuration the refusal has to hold in.
	service.isProduction = false
	mockSender.shouldError = false

	apiKeyEmail := "zapier@api.example.com"
	mockRepo.EXPECT().
		GetUserByEmail(gomock.Any(), apiKeyEmail).
		Return(&domain.User{ID: "apikey123", Email: apiKeyEmail, Type: domain.UserTypeAPIKey}, nil)
	// No CreateSession expectation: minting a session for an API key fails the test.

	code, err := service.SignIn(context.Background(), domain.SignInInput{Email: apiKeyEmail})
	require.Error(t, err)
	require.Empty(t, code, "no magic code may be handed back for an API key address")

	var notFound *domain.ErrUserNotFound
	require.ErrorAs(t, err, &notFound)

	// The refusal must be indistinguishable from an address nobody holds.
	unknownEmail := "nobody@example.com"
	mockRepo.EXPECT().
		GetUserByEmail(gomock.Any(), unknownEmail).
		Return(nil, &domain.ErrUserNotFound{Message: "user not found"})

	_, unknownErr := service.SignIn(context.Background(), domain.SignInInput{Email: unknownEmail})
	require.Error(t, unknownErr)
	require.Equal(t, unknownErr.Error(), err.Error())
}

// expectMagicCode sets the sign-in transaction up to the code's lookup: the user's lock, an
// authenticator row (nil: none), and what ConsumeMagicCode answers.
func expectMagicCode(m *userTxMocks, userID, code string, totpRow *domain.UserTOTP, session *domain.Session, consumeErr error) {
	m.authTx.EXPECT().LockUser(gomock.Any(), gomock.Any(), userID).Return(nil)
	m.twoFactor.EXPECT().GetTOTPForUpdate(gomock.Any(), gomock.Any(), userID).Return(totpRow, nil)
	m.authTx.EXPECT().ConsumeMagicCode(gomock.Any(), gomock.Any(), userID, crypto.HashMagicCode(code, "test-secret-key-for-hmac-verification")).
		Return(session, consumeErr)
}

func TestUserService_VerifyCode(t *testing.T) {
	email := "test@example.com"
	code := "123456"
	userID := "user123"
	user := &domain.User{ID: userID, Email: email}
	notFound := &domain.ErrSessionNotFound{Message: "no session holds this code"}

	t.Run("successful verification activates the session", func(t *testing.T) {
		mockRepo, mockAuthService, service, _ := setupUserTest(t)
		m := txMocksOf(service)
		session := &domain.Session{ID: "session123", UserID: userID, ExpiresAt: time.Now().Add(24 * time.Hour)}
		now := time.Now()

		mockRepo.EXPECT().GetUserByEmail(gomock.Any(), email).Return(user, nil)
		expectMagicCode(m, userID, code, nil, session, nil)
		m.authTx.EXPECT().PurgeExpiredPendingSessions(gomock.Any(), gomock.Any(), userID).Return(nil)
		m.twoFactor.EXPECT().ClockTimestamp(gomock.Any(), gomock.Any()).Return(now, nil)
		m.authTx.EXPECT().ActivateSession(gomock.Any(), gomock.Any(), "session123", domain.AuthMethodMagicCode, now).Return(nil)
		mockAuthService.EXPECT().GenerateUserAuthToken(user, session.ID, session.ExpiresAt).Return("token123")

		result, err := service.VerifyCode(context.Background(), domain.VerifyCodeInput{Email: email, Code: code})
		require.NoError(t, err)
		require.Equal(t, "token123", result.Token)
		require.Equal(t, *user, result.User)
		require.Equal(t, session.ExpiresAt, result.ExpiresAt)
		require.Nil(t, result.Challenge)
		require.Equal(t, 1, m.tx.commits)
	})

	// The email code is right, but the account has an authenticator: no token, a challenge.
	t.Run("with two-factor authentication on, the session waits for the second factor", func(t *testing.T) {
		mockRepo, _, service, _ := setupUserTest(t)
		m := txMocksOf(service)
		session := &domain.Session{ID: "session123", UserID: userID, ExpiresAt: time.Now().Add(24 * time.Hour)}
		now := time.Now()
		enabledAt := now.Add(-time.Hour)
		secret := "sealed"

		mockRepo.EXPECT().GetUserByEmail(gomock.Any(), email).Return(user, nil)
		expectMagicCode(m, userID, code, &domain.UserTOTP{UserID: userID, EnabledAt: &enabledAt, SecretEncrypted: &secret}, session, nil)
		m.authTx.EXPECT().PurgeExpiredPendingSessions(gomock.Any(), gomock.Any(), userID).Return(nil)
		m.twoFactor.EXPECT().ClockTimestamp(gomock.Any(), gomock.Any()).Return(now, nil)
		var storedHash string
		m.authTx.EXPECT().SetSessionChallenge(gomock.Any(), gomock.Any(), "session123", domain.AuthMethodMagicCode, gomock.Any(), now.Add(domain.ChallengeTTL), nil).
			DoAndReturn(func(_ context.Context, _ domain.DBExecutor, _ string, _ domain.AuthMethod, hash string, _ time.Time, _ *string) error {
				storedHash = hash
				return nil
			})

		result, err := service.VerifyCode(context.Background(), domain.VerifyCodeInput{Email: email, Code: code})
		require.NoError(t, err)
		require.Empty(t, result.Token, "no token before the second factor")
		require.NotNil(t, result.Challenge)
		require.Equal(t, HashChallenge(result.Challenge.Token), storedHash, "only the hash of the challenge is stored")
		require.Equal(t, now.Add(domain.ChallengeTTL), result.Challenge.ExpiresAt)
	})

	t.Run("invalid code", func(t *testing.T) {
		mockRepo, _, service, _ := setupUserTest(t)
		m := txMocksOf(service)
		mockRepo.EXPECT().GetUserByEmail(gomock.Any(), email).Return(user, nil)
		expectMagicCode(m, userID, code, nil, nil, notFound)
		m.authTx.EXPECT().HasExpiredMagicCode(gomock.Any(), gomock.Any(), userID, gomock.Any()).Return(false, nil)

		result, err := service.VerifyCode(context.Background(), domain.VerifyCodeInput{Email: email, Code: code})
		require.Nil(t, result)
		require.EqualError(t, err, "invalid magic code")
		require.Equal(t, 1, m.tx.rollbacks)
	})

	t.Run("expired code", func(t *testing.T) {
		mockRepo, _, service, _ := setupUserTest(t)
		m := txMocksOf(service)
		mockRepo.EXPECT().GetUserByEmail(gomock.Any(), email).Return(user, nil)
		expectMagicCode(m, userID, code, nil, nil, notFound)
		m.authTx.EXPECT().HasExpiredMagicCode(gomock.Any(), gomock.Any(), userID, gomock.Any()).Return(true, nil)

		result, err := service.VerifyCode(context.Background(), domain.VerifyCodeInput{Email: email, Code: code})
		require.Nil(t, result)
		require.EqualError(t, err, "magic code expired")
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
			Type:  domain.UserTypeUser,
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
		require.ErrorIs(t, err, ErrSessionExpired)
		require.ErrorIs(t, err, domain.ErrSessionAuthFailed)
	})

	// The handler answers 401 for ErrSessionAuthFailed and 500 otherwise, and the
	// console signs the user out on any 401 — so which errors carry the marker is
	// the contract, not an implementation detail.
	t.Run("unknown session is an auth failure", func(t *testing.T) {
		mockRepo.EXPECT().
			GetSessionByID(gomock.Any(), sessionID).
			Return(nil, &domain.ErrSessionNotFound{Message: "session not found"})

		result, err := service.VerifyUserSession(context.Background(), userID, sessionID)

		require.Error(t, err)
		require.Nil(t, result)
		require.ErrorIs(t, err, domain.ErrSessionAuthFailed)
	})

	t.Run("a session never authenticates an API key account", func(t *testing.T) {
		mockRepo.EXPECT().
			GetSessionByID(gomock.Any(), sessionID).
			Return(&domain.Session{
				ID:        sessionID,
				UserID:    userID,
				ExpiresAt: time.Now().Add(1 * time.Hour),
			}, nil)
		mockRepo.EXPECT().
			GetUserByID(gomock.Any(), userID).
			Return(&domain.User{ID: userID, Email: "key@example.com", Type: domain.UserTypeAPIKey}, nil)

		result, err := service.VerifyUserSession(context.Background(), userID, sessionID)

		require.Error(t, err)
		require.Nil(t, result)
		require.ErrorIs(t, err, domain.ErrSessionAuthFailed)
	})

	t.Run("session belonging to another user is an auth failure", func(t *testing.T) {
		mockRepo.EXPECT().
			GetSessionByID(gomock.Any(), sessionID).
			Return(&domain.Session{
				ID:        sessionID,
				UserID:    "someone-else",
				ExpiresAt: time.Now().Add(1 * time.Hour),
			}, nil)

		result, err := service.VerifyUserSession(context.Background(), userID, sessionID)

		require.Error(t, err)
		require.Nil(t, result)
		require.ErrorIs(t, err, domain.ErrSessionAuthFailed)
	})

	t.Run("missing user record is an auth failure", func(t *testing.T) {
		mockRepo.EXPECT().
			GetSessionByID(gomock.Any(), sessionID).
			Return(&domain.Session{
				ID:        sessionID,
				UserID:    userID,
				ExpiresAt: time.Now().Add(1 * time.Hour),
			}, nil)
		mockRepo.EXPECT().
			GetUserByID(gomock.Any(), userID).
			Return(nil, &domain.ErrUserNotFound{Message: "user not found"})

		result, err := service.VerifyUserSession(context.Background(), userID, sessionID)

		require.Error(t, err)
		require.Nil(t, result)
		require.ErrorIs(t, err, domain.ErrSessionAuthFailed)
	})

	t.Run("session lookup failure is not an auth failure", func(t *testing.T) {
		mockRepo.EXPECT().
			GetSessionByID(gomock.Any(), sessionID).
			Return(nil, errors.New("pq: password authentication failed for user"))

		result, err := service.VerifyUserSession(context.Background(), userID, sessionID)

		require.Error(t, err)
		require.Nil(t, result)
		require.NotErrorIs(t, err, domain.ErrSessionAuthFailed)
	})

	t.Run("user lookup failure is not an auth failure", func(t *testing.T) {
		mockRepo.EXPECT().
			GetSessionByID(gomock.Any(), sessionID).
			Return(&domain.Session{
				ID:        sessionID,
				UserID:    userID,
				ExpiresAt: time.Now().Add(1 * time.Hour),
			}, nil)
		mockRepo.EXPECT().
			GetUserByID(gomock.Any(), userID).
			Return(nil, errors.New("connection reset by peer"))

		result, err := service.VerifyUserSession(context.Background(), userID, sessionID)

		require.Error(t, err)
		require.Nil(t, result)
		require.NotErrorIs(t, err, domain.ErrSessionAuthFailed)
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

		// The root signature proves possession of SECRET_KEY: the session meets the
		// two-factor requirement and counts as fresh.
		mockRepo.EXPECT().
			CreateSession(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, session *domain.Session) error {
				require.Equal(t, domain.AuthMethodRootSignature, session.AuthMethod)
				require.NotNil(t, session.AuthenticatedAt)
				return nil
			})

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

func TestUserService_RootSignin_MultipleRootEmails(t *testing.T) {
	mockRepo, mockAuthService, service, _ := setupUserTestWithRootEmail(t, "root1@example.com,root2@example.com")
	secretKey := "test-secret-key-for-hmac-verification"

	t.Run("second listed root can sign in", func(t *testing.T) {
		secondRoot := "root2@example.com"
		timestamp := time.Now().Unix()
		message := fmt.Sprintf("%s:%d", secondRoot, timestamp)
		signature := crypto.ComputeHMAC256([]byte(message), secretKey)

		user := &domain.User{ID: "root2-id", Email: secondRoot}

		mockRepo.EXPECT().GetUserByEmail(gomock.Any(), secondRoot).Return(user, nil)
		mockRepo.EXPECT().CreateSession(gomock.Any(), gomock.Any()).Return(nil)
		mockAuthService.EXPECT().GenerateUserAuthToken(user, gomock.Any(), gomock.Any()).Return("jwt-token")

		result, err := service.RootSignin(context.Background(), domain.RootSigninInput{
			Email:     secondRoot,
			Timestamp: timestamp,
			Signature: signature,
		})

		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, secondRoot, result.User.Email)
	})

	t.Run("non-listed email is rejected", func(t *testing.T) {
		outsider := "outsider@example.com"
		timestamp := time.Now().Unix()
		message := fmt.Sprintf("%s:%d", outsider, timestamp)
		signature := crypto.ComputeHMAC256([]byte(message), secretKey)

		result, err := service.RootSignin(context.Background(), domain.RootSigninInput{
			Email:     outsider,
			Timestamp: timestamp,
			Signature: signature,
		})

		require.Error(t, err)
		require.Nil(t, result)
		require.Contains(t, err.Error(), "invalid credentials")
	})

	t.Run("case-sensitive mismatch is rejected", func(t *testing.T) {
		mixedCase := "Root1@example.com" // configured as lowercase root1@example.com
		timestamp := time.Now().Unix()
		message := fmt.Sprintf("%s:%d", mixedCase, timestamp)
		signature := crypto.ComputeHMAC256([]byte(message), secretKey)

		result, err := service.RootSignin(context.Background(), domain.RootSigninInput{
			Email:     mixedCase,
			Timestamp: timestamp,
			Signature: signature,
		})

		require.Error(t, err)
		require.Nil(t, result)
		require.Contains(t, err.Error(), "invalid credentials")
	})
}

func TestUserService_Logout(t *testing.T) {
	_, _, service, _ := setupUserTest(t)

	ctx := context.Background()
	userID := "user123"

	m := txMocksOf(service)

	t.Run("Success - Logs out user", func(t *testing.T) {
		lock := m.authTx.EXPECT().LockUser(gomock.Any(), gomock.Any(), userID).Return(nil)
		m.authTx.EXPECT().DeleteAllSessions(gomock.Any(), gomock.Any(), userID).Return(nil).After(lock)

		err := service.Logout(ctx, userID)
		require.NoError(t, err)
	})

	t.Run("a user deleted meanwhile is signed out already", func(t *testing.T) {
		m.authTx.EXPECT().LockUser(gomock.Any(), gomock.Any(), "gone").Return(&domain.ErrUserNotFound{Message: "user not found"})
		require.NoError(t, service.Logout(ctx, "gone"))
	})

	t.Run("Error - Repository error", func(t *testing.T) {
		m.authTx.EXPECT().LockUser(gomock.Any(), gomock.Any(), userID).Return(nil)
		m.authTx.EXPECT().DeleteAllSessions(gomock.Any(), gomock.Any(), userID).Return(errors.New("database error"))

		err := service.Logout(ctx, userID)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to logout")
	})
}
