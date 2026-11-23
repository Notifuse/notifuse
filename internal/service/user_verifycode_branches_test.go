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
// limiter must trip before the repository is touched, a code is consumed in the
// same transaction that makes the session usable, and lookup failures must not
// be mistaken for bad codes.

func TestUserService_VerifyCode_Branches(t *testing.T) {
	ctx := context.Background()
	email := "user@example.com"
	user := &domain.User{ID: "user-1", Email: email}
	notFound := &domain.ErrSessionNotFound{Message: "no session holds this code"}

	t.Run("unknown email surfaces the repository error, not 'invalid code'", func(t *testing.T) {
		mockRepo, _, service, _ := setupUserTest(t)
		sentinel := &domain.ErrUserNotFound{Message: "user not found"}
		mockRepo.EXPECT().GetUserByEmail(gomock.Any(), email).Return(nil, sentinel).Times(1)

		_, err := service.VerifyCode(ctx, domain.VerifyCodeInput{Email: email, Code: "123456"})
		require.Error(t, err)
		var nf *domain.ErrUserNotFound
		assert.ErrorAs(t, err, &nf)
		assert.NotEqual(t, "invalid magic code", err.Error())
	})

	t.Run("a lookup failure is passed through and rolls back", func(t *testing.T) {
		mockRepo, _, service, _ := setupUserTest(t)
		m := txMocksOf(service)
		mockRepo.EXPECT().GetUserByEmail(gomock.Any(), email).Return(user, nil).Times(1)
		sentinel := errors.New("sessions table gone")
		expectMagicCode(m, "user-1", "123456", nil, nil, sentinel)

		_, err := service.VerifyCode(ctx, domain.VerifyCodeInput{Email: email, Code: "123456"})
		assert.ErrorIs(t, err, sentinel)
		assert.Equal(t, 1, m.tx.rollbacks)
	})

	t.Run("a write failure after the code matched rolls the consumption back", func(t *testing.T) {
		mockRepo, _, service, _ := setupUserTest(t)
		m := txMocksOf(service)
		mockRepo.EXPECT().GetUserByEmail(gomock.Any(), email).Return(user, nil).Times(1)
		expectMagicCode(m, "user-1", "123456", nil, &domain.Session{ID: "sess-1", UserID: "user-1"}, nil)
		sentinel := errors.New("write failed")
		m.authTx.EXPECT().PurgeExpiredPendingSessions(gomock.Any(), gomock.Any(), "user-1").Return(sentinel)

		_, err := service.VerifyCode(ctx, domain.VerifyCodeInput{Email: email, Code: "123456"})
		assert.ErrorIs(t, err, sentinel)
		assert.Equal(t, 1, m.tx.rollbacks)
		assert.Zero(t, m.tx.commits)
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
		m := txMocksOf(service)
		session := &domain.Session{ID: "sess-1", UserID: "user-1", ExpiresAt: time.Now().Add(24 * time.Hour)}

		mockRepo.EXPECT().GetUserByEmail(gomock.Any(), email).Return(user, nil).Times(6)
		m.authTx.EXPECT().LockUser(gomock.Any(), gomock.Any(), "user-1").Return(nil).Times(6)
		m.twoFactor.EXPECT().GetTOTPForUpdate(gomock.Any(), gomock.Any(), "user-1").Return(nil, nil).Times(6)
		good := crypto.HashMagicCode("123456", verifyTestSecret)
		m.authTx.EXPECT().ConsumeMagicCode(gomock.Any(), gomock.Any(), "user-1", good).Return(session, nil).Times(1)
		m.authTx.EXPECT().ConsumeMagicCode(gomock.Any(), gomock.Any(), "user-1", gomock.Not(good)).Return(nil, notFound).Times(5)
		m.authTx.EXPECT().HasExpiredMagicCode(gomock.Any(), gomock.Any(), "user-1", gomock.Any()).Return(false, nil).Times(5)
		m.authTx.EXPECT().PurgeExpiredPendingSessions(gomock.Any(), gomock.Any(), "user-1").Return(nil)
		m.twoFactor.EXPECT().ClockTimestamp(gomock.Any(), gomock.Any()).Return(time.Now(), nil)
		m.authTx.EXPECT().ActivateSession(gomock.Any(), gomock.Any(), "sess-1", domain.AuthMethodMagicCode, gomock.Any()).Return(nil)
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

const verifyTestSecret = "test-secret-key-for-hmac-verification"
