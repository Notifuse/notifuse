package service

import (
	"context"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
)

// The login services name the reason a sign-in failed on the request's audit
// record, so the row can say why while the response gives nothing away.
func TestUserService_LoginFailuresNameTheirReason(t *testing.T) {
	t.Run("unknown email on sign-in", func(t *testing.T) {
		mockRepo, _, service, _ := setupUserTest(t)
		mockRepo.EXPECT().GetUserByEmail(gomock.Any(), "nobody@example.com").Return(nil, &domain.ErrUserNotFound{Message: "user does not exist"})

		record := domain.NewAuditRecord("", "", "")
		_, err := service.SignIn(domain.WithAuditRecord(context.Background(), record), domain.SignInInput{Email: "nobody@example.com"})
		require.Error(t, err)

		event, _ := record.Finalize(200)
		assert.Equal(t, domain.AuditOutcomeFailure, event.Outcome, "whatever status the handler answers")
		assert.Equal(t, "unknown_email", event.Metadata["reason"])
	})

	t.Run("wrong magic code", func(t *testing.T) {
		mockRepo, _, service, _ := setupUserTest(t)
		code := "654321"
		expires := time.Now().Add(15 * time.Minute)
		mockRepo.EXPECT().GetUserByEmail(gomock.Any(), "ann@example.com").Return(&domain.User{ID: "u1", Email: "ann@example.com"}, nil)
		mockRepo.EXPECT().GetSessionsByUserID(gomock.Any(), "u1").Return([]*domain.Session{{ID: "s1", UserID: "u1", MagicCode: &code, MagicCodeExpires: &expires}}, nil)

		record := domain.NewAuditRecord("", "", "")
		_, err := service.VerifyCode(domain.WithAuditRecord(context.Background(), record), domain.VerifyCodeInput{Email: "ann@example.com", Code: "000000"})
		require.Error(t, err)

		event, _ := record.Finalize(401)
		assert.Equal(t, domain.AuditOutcomeFailure, event.Outcome)
		assert.Equal(t, "invalid_code", event.Metadata["reason"])
		assert.True(t, record.Failed())
	})

	t.Run("root sign-in with a bad signature", func(t *testing.T) {
		_, _, service, _ := setupUserTestWithRootEmail(t, "root@example.com")
		record := domain.NewAuditRecord("", "", "")
		_, err := service.RootSignin(domain.WithAuditRecord(context.Background(), record), domain.RootSigninInput{
			Email: "root@example.com", Timestamp: time.Now().Unix(), Signature: "not-the-signature",
		})
		require.Error(t, err)

		event, _ := record.Finalize(401)
		assert.Equal(t, "bad_signature", event.Metadata["reason"])
	})

	t.Run("root sign-in from a non-root address", func(t *testing.T) {
		_, _, service, _ := setupUserTestWithRootEmail(t, "root@example.com")
		record := domain.NewAuditRecord("", "", "")
		_, err := service.RootSignin(domain.WithAuditRecord(context.Background(), record), domain.RootSigninInput{
			Email: "ann@example.com", Timestamp: time.Now().Unix(), Signature: "x",
		})
		require.Error(t, err)
		event, _ := record.Finalize(401)
		assert.Equal(t, "not_root", event.Metadata["reason"])
	})
}
