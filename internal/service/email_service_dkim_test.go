package service

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/pkg/dkim"
	"github.com/Notifuse/notifuse/pkg/logger"
)

type emailServiceDKIMFixture struct {
	svc  *EmailService
	smtp *mocks.MockEmailProviderService
	ses  *mocks.MockEmailProviderService
	repo *fakeDKIMRepo
}

func newEmailServiceDKIMFixture(t *testing.T, keys ...domain.DKIMKey) *emailServiceDKIMFixture {
	t.Helper()
	ctrl := gomock.NewController(t)
	f := &emailServiceDKIMFixture{
		smtp: mocks.NewMockEmailProviderService(ctrl),
		ses:  mocks.NewMockEmailProviderService(ctrl),
		repo: &fakeDKIMRepo{keys: keys},
	}
	f.svc = &EmailService{
		logger:      logger.NewLoggerWithLevel("disabled"),
		secretKey:   dkimServiceSecret,
		smtpService: f.smtp,
		sesService:  f.ses,
	}
	f.svc.SetDKIMKeyStore(NewDKIMKeyStore(f.repo, dkimServiceSecret))
	return f
}

func dkimEmailRequest(kind domain.EmailProviderKind, from string) domain.SendEmailProviderRequest {
	return domain.SendEmailProviderRequest{
		WorkspaceID:   "ws-1",
		IntegrationID: "int-1",
		MessageID:     "msg-1",
		FromAddress:   from,
		FromName:      "Acme",
		To:            "zoe@example.com",
		Subject:       "Hello",
		Content:       "<p>Hello</p>",
		Provider:      &domain.EmailProvider{Kind: kind},
	}
}

func publishedKey(t *testing.T, domainName string) domain.DKIMKey {
	t.Helper()
	return domain.DKIMKey{Domain: domainName, Selector: dkimTestSelector, Status: domain.DKIMKeyStatusPublished, EncryptedPrivateKey: sealedServiceKey(t)}
}

func TestEmailService_SendEmail_ResolvesTheDKIMKey(t *testing.T) {
	ctx := context.Background()

	t.Run("an SMTP send from a published domain carries its key", func(t *testing.T) {
		f := newEmailServiceDKIMFixture(t, publishedKey(t, "acme.io"))
		f.smtp.EXPECT().SendEmail(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, r domain.SendEmailProviderRequest) error {
			require.NotNil(t, r.DKIM)
			assert.Equal(t, "acme.io", r.DKIM.Domain)
			assert.Equal(t, dkimTestSelector, r.DKIM.Selector)
			assert.NotNil(t, r.DKIM.Key)
			return nil
		})
		require.NoError(t, f.svc.SendEmail(ctx, dkimEmailRequest(domain.EmailProviderKindSMTP, "news@acme.io"), false))
		assert.Equal(t, 1, f.repo.reads, "no snapshot: the keys are read")
	})

	t.Run("an empty snapshot means no lookup and no key", func(t *testing.T) {
		f := newEmailServiceDKIMFixture(t, publishedKey(t, "acme.io"))
		f.smtp.EXPECT().SendEmail(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, r domain.SendEmailProviderRequest) error {
			assert.Nil(t, r.DKIM)
			return nil
		})
		request := dkimEmailRequest(domain.EmailProviderKindSMTP, "news@acme.io")
		request.DKIMSnapshot = (&domain.Workspace{}).DKIMSnapshot()
		require.NoError(t, f.svc.SendEmail(ctx, request, true))
		assert.Zero(t, f.repo.reads)
	})

	for _, from := range []string{"news@support.acme.io", "news@pending.io", "news@mismatch.io"} {
		t.Run("unsigned from "+from, func(t *testing.T) {
			f := newEmailServiceDKIMFixture(t,
				publishedKey(t, "acme.io"),
				domain.DKIMKey{Domain: "pending.io", Status: domain.DKIMKeyStatusPending, EncryptedPrivateKey: sealedServiceKey(t)},
				domain.DKIMKey{Domain: "mismatch.io", Status: domain.DKIMKeyStatusMismatch, EncryptedPrivateKey: sealedServiceKey(t)},
			)
			f.smtp.EXPECT().SendEmail(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, r domain.SendEmailProviderRequest) error {
				assert.Nil(t, r.DKIM)
				return nil
			})
			require.NoError(t, f.svc.SendEmail(ctx, dkimEmailRequest(domain.EmailProviderKindSMTP, from), false))
		})
	}

	t.Run("an API provider signs on its side: nothing is resolved", func(t *testing.T) {
		f := newEmailServiceDKIMFixture(t, publishedKey(t, "acme.io"))
		f.ses.EXPECT().SendEmail(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, r domain.SendEmailProviderRequest) error {
			assert.Nil(t, r.DKIM)
			return nil
		})
		require.NoError(t, f.svc.SendEmail(ctx, dkimEmailRequest(domain.EmailProviderKindSES, "news@acme.io"), false))
		assert.Zero(t, f.repo.reads)
	})

	t.Run("a key that does not open stops the send with a signing error", func(t *testing.T) {
		broken := publishedKey(t, "acme.io")
		broken.EncryptedPrivateKey = "zz"
		f := newEmailServiceDKIMFixture(t, broken)
		err := f.svc.SendEmail(ctx, dkimEmailRequest(domain.EmailProviderKindSMTP, "news@acme.io"), false)
		assert.True(t, errors.Is(err, dkim.ErrSign))
	})

	t.Run("a DKIM value set by the caller is never trusted", func(t *testing.T) {
		f := newEmailServiceDKIMFixture(t)
		f.smtp.EXPECT().SendEmail(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, r domain.SendEmailProviderRequest) error {
			assert.Nil(t, r.DKIM)
			return nil
		})
		request := dkimEmailRequest(domain.EmailProviderKindSMTP, "news@acme.io")
		request.DKIM = &domain.DKIMSigningKey{Domain: "acme.io"}
		require.NoError(t, f.svc.SendEmail(ctx, request, false))
	})
}
