package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/crypto"
)

// VerifyCode is the brute-force surface for magic-code sign-in: the rate
// limiter must trip before the repository is touched, a code must be
// consumed (cleared) exactly when it matches, and lookup failures must not
// be mistaken for bad codes.

const verifyTestSecret = "test-secret-key-for-hmac-verification"

func sessionWithCode(t *testing.T, code string, expires time.Time) *domain.Session {
	t.Helper()
	hashed := crypto.HashMagicCode(code, verifyTestSecret)
	return &domain.Session{
		ID:               "sess-1",
		UserID:           "user-1",
		MagicCode:        &hashed,
		MagicCodeExpires: &expires,
		ExpiresAt:        time.Now().Add(24 * time.Hour),
	}
}

func TestUserService_VerifyCode_Branches(t *testing.T) {
	ctx := context.Background()
	email := "user@example.com"
	user := &domain.User{ID: "user-1", Email: email}

	t.Run("unknown email surfaces the repository error, not 'invalid code'", func(t *testing.T) {
		mockRepo, _, service, _ := setupUserTest(t)
		sentinel := &domain.ErrUserNotFound{Message: "user not found"}
		mockRepo.EXPECT().GetUserByEmail(gomock.Any(), email).Return(nil, sentinel).Times(1)

		_, err := service.VerifyCode(ctx, domain.VerifyCodeInput{Email: email, Code: "123456"})
		require.Error(t, err)
		var notFound *domain.ErrUserNotFound
		assert.ErrorAs(t, err, &notFound)
		assert.NotEqual(t, "invalid magic code", err.Error())
	})

	t.Run("session lookup failure is passed through", func(t *testing.T) {
		mockRepo, _, service, _ := setupUserTest(t)
		mockRepo.EXPECT().GetUserByEmail(gomock.Any(), email).Return(user, nil).Times(1)
		sentinel := errors.New("sessions table gone")
		mockRepo.EXPECT().GetSessionsByUserID(gomock.Any(), "user-1").Return(nil, sentinel).Times(1)

		_, err := service.VerifyCode(ctx, domain.VerifyCodeInput{Email: email, Code: "123456"})
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("sessions without a magic code cannot match", func(t *testing.T) {
		mockRepo, _, service, _ := setupUserTest(t)
		mockRepo.EXPECT().GetUserByEmail(gomock.Any(), email).Return(user, nil).Times(1)
		empty := ""
		mockRepo.EXPECT().GetSessionsByUserID(gomock.Any(), "user-1").
			Return([]*domain.Session{
				{ID: "s1", UserID: "user-1"},
				{ID: "s2", UserID: "user-1", MagicCode: &empty},
			}, nil).Times(1)

		_, err := service.VerifyCode(ctx, domain.VerifyCodeInput{Email: email, Code: "123456"})
		require.EqualError(t, err, "invalid magic code")
	})

	t.Run("a matching code is cleared before the session is stored", func(t *testing.T) {
		mockRepo, mockAuth, service, _ := setupUserTest(t)
		session := sessionWithCode(t, "123456", time.Now().Add(15*time.Minute))
		mockRepo.EXPECT().GetUserByEmail(gomock.Any(), email).Return(user, nil).Times(1)
		mockRepo.EXPECT().GetSessionsByUserID(gomock.Any(), "user-1").
			Return([]*domain.Session{session}, nil).Times(1)
		mockRepo.EXPECT().UpdateSession(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, s *domain.Session) error {
				assert.Nil(t, s.MagicCode, "the code is consumed on use: it must never verify twice")
				assert.Nil(t, s.MagicCodeExpires)
				return nil
			}).Times(1)
		mockAuth.EXPECT().GenerateUserAuthToken(user, "sess-1", gomock.Any()).Return("tok").Times(1)

		resp, err := service.VerifyCode(ctx, domain.VerifyCodeInput{Email: email, Code: "123456"})
		require.NoError(t, err)
		assert.Equal(t, "tok", resp.Token)
	})

	t.Run("a store failure after a match still consumes nothing twice", func(t *testing.T) {
		mockRepo, _, service, _ := setupUserTest(t)
		session := sessionWithCode(t, "123456", time.Now().Add(15*time.Minute))
		mockRepo.EXPECT().GetUserByEmail(gomock.Any(), email).Return(user, nil).Times(1)
		mockRepo.EXPECT().GetSessionsByUserID(gomock.Any(), "user-1").
			Return([]*domain.Session{session}, nil).Times(1)
		sentinel := errors.New("write failed")
		mockRepo.EXPECT().UpdateSession(gomock.Any(), gomock.Any()).Return(sentinel).Times(1)

		_, err := service.VerifyCode(ctx, domain.VerifyCodeInput{Email: email, Code: "123456"})
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("the rate limiter trips before the repository is touched", func(t *testing.T) {
		mockRepo, _, service, _ := setupUserTest(t)
		// Policy is 5 attempts per window (see setupUserTest). Five failing
		// lookups consume the budget; the sixth must be refused up front.
		mockRepo.EXPECT().GetUserByEmail(gomock.Any(), email).
			Return(nil, errors.New("user not found")).Times(5)

		for i := 0; i < 5; i++ {
			_, err := service.VerifyCode(ctx, domain.VerifyCodeInput{Email: email, Code: "000000"})
			require.Error(t, err)
		}
		_, err := service.VerifyCode(ctx, domain.VerifyCodeInput{Email: email, Code: "000000"})
		require.EqualError(t, err, "too many verification attempts, please try again in a few minutes")
	})

	t.Run("a successful verification resets the rate-limit budget", func(t *testing.T) {
		mockRepo, mockAuth, service, _ := setupUserTest(t)
		session := sessionWithCode(t, "123456", time.Now().Add(15*time.Minute))

		// Four failures, then success on the fifth (last allowed) attempt.
		mockRepo.EXPECT().GetUserByEmail(gomock.Any(), email).Return(user, nil).Times(6)
		mockRepo.EXPECT().GetSessionsByUserID(gomock.Any(), "user-1").
			Return([]*domain.Session{session}, nil).Times(6)
		mockRepo.EXPECT().UpdateSession(gomock.Any(), gomock.Any()).Return(nil).Times(1)
		mockAuth.EXPECT().GenerateUserAuthToken(user, "sess-1", gomock.Any()).Return("tok").Times(1)

		for i := 0; i < 4; i++ {
			_, err := service.VerifyCode(ctx, domain.VerifyCodeInput{Email: email, Code: "000000"})
			require.EqualError(t, err, "invalid magic code")
		}
		_, err := service.VerifyCode(ctx, domain.VerifyCodeInput{Email: email, Code: "123456"})
		require.NoError(t, err)

		// The budget was reset by the success: a fresh failure is a code
		// error, not a rate-limit refusal.
		_, err = service.VerifyCode(ctx, domain.VerifyCodeInput{Email: email, Code: "000000"})
		require.EqualError(t, err, "invalid magic code")
	})
}
