package service

import (
	"context"
	"testing"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Updating an integration without re-supplying its credential.
//
// Clients no longer receive an integration's plaintext credential, so an editor
// changing a host or a sender name has nothing to send back in the secret field.
// This exercises the REAL UpdateIntegration path — preservation, then
// Integration.Validate, then the repository write — because preservation alone
// proves nothing if validation then rejects the blank field.
//
// One case per provider: a validation rule that demands a non-empty secret would
// make that provider uneditable, and it would only show up in production.

func TestUpdateIntegration_SucceedsWithBlankCredential(t *testing.T) {
	senders := []domain.EmailSender{domain.NewEmailSender("sender@example.com", "Sender")}

	cases := []struct {
		name     string
		stored   domain.EmailProvider
		incoming domain.EmailProvider
		verify   func(*testing.T, *domain.Integration)
	}{
		{
			name: "smtp",
			stored: domain.EmailProvider{
				Kind: domain.EmailProviderKindSMTP, RateLimitPerMinute: 25, Senders: senders,
				SMTP: &domain.SMTPSettings{Host: "old.example.com", Port: 587, Username: "u", EncryptedPassword: "STORED"},
			},
			incoming: domain.EmailProvider{
				Kind: domain.EmailProviderKindSMTP, RateLimitPerMinute: 25, Senders: senders,
				SMTP: &domain.SMTPSettings{Host: "new.example.com", Port: 587, Username: "u"},
			},
			verify: func(t *testing.T, got *domain.Integration) {
				assert.Equal(t, "STORED", got.EmailProvider.SMTP.EncryptedPassword)
				assert.Equal(t, "new.example.com", got.EmailProvider.SMTP.Host)
			},
		},
		{
			name: "ses",
			stored: domain.EmailProvider{
				Kind: domain.EmailProviderKindSES, RateLimitPerMinute: 25, Senders: senders,
				SES: &domain.AmazonSESSettings{Region: "eu-west-1", AccessKey: "AKIA", EncryptedSecretKey: "STORED"},
			},
			incoming: domain.EmailProvider{
				Kind: domain.EmailProviderKindSES, RateLimitPerMinute: 25, Senders: senders,
				SES: &domain.AmazonSESSettings{Region: "eu-west-3", AccessKey: "AKIA"},
			},
			verify: func(t *testing.T, got *domain.Integration) {
				assert.Equal(t, "STORED", got.EmailProvider.SES.EncryptedSecretKey)
				assert.Equal(t, "eu-west-3", got.EmailProvider.SES.Region)
			},
		},
		{
			name: "sparkpost",
			stored: domain.EmailProvider{
				Kind: domain.EmailProviderKindSparkPost, RateLimitPerMinute: 25, Senders: senders,
				SparkPost: &domain.SparkPostSettings{Endpoint: "https://api.sparkpost.com", EncryptedAPIKey: "STORED"},
			},
			incoming: domain.EmailProvider{
				Kind: domain.EmailProviderKindSparkPost, RateLimitPerMinute: 25, Senders: senders,
				SparkPost: &domain.SparkPostSettings{Endpoint: "https://api.eu.sparkpost.com"},
			},
			verify: func(t *testing.T, got *domain.Integration) {
				assert.Equal(t, "STORED", got.EmailProvider.SparkPost.EncryptedAPIKey)
			},
		},
		{
			name: "postmark",
			stored: domain.EmailProvider{
				Kind: domain.EmailProviderKindPostmark, RateLimitPerMinute: 25, Senders: senders,
				Postmark: &domain.PostmarkSettings{EncryptedServerToken: "STORED"},
			},
			incoming: domain.EmailProvider{
				Kind: domain.EmailProviderKindPostmark, RateLimitPerMinute: 30, Senders: senders,
				Postmark: &domain.PostmarkSettings{},
			},
			verify: func(t *testing.T, got *domain.Integration) {
				assert.Equal(t, "STORED", got.EmailProvider.Postmark.EncryptedServerToken)
			},
		},
		{
			name: "mailgun",
			stored: domain.EmailProvider{
				Kind: domain.EmailProviderKindMailgun, RateLimitPerMinute: 25, Senders: senders,
				Mailgun: &domain.MailgunSettings{Domain: "mg.example.com", EncryptedAPIKey: "STORED"},
			},
			incoming: domain.EmailProvider{
				Kind: domain.EmailProviderKindMailgun, RateLimitPerMinute: 25, Senders: senders,
				Mailgun: &domain.MailgunSettings{Domain: "mg2.example.com"},
			},
			verify: func(t *testing.T, got *domain.Integration) {
				assert.Equal(t, "STORED", got.EmailProvider.Mailgun.EncryptedAPIKey)
			},
		},
		{
			name: "mailjet",
			stored: domain.EmailProvider{
				Kind: domain.EmailProviderKindMailjet, RateLimitPerMinute: 25, Senders: senders,
				Mailjet: &domain.MailjetSettings{EncryptedAPIKey: "STORED-API", EncryptedSecretKey: "STORED-SECRET"},
			},
			incoming: domain.EmailProvider{
				Kind: domain.EmailProviderKindMailjet, RateLimitPerMinute: 30, Senders: senders,
				Mailjet: &domain.MailjetSettings{},
			},
			verify: func(t *testing.T, got *domain.Integration) {
				assert.Equal(t, "STORED-API", got.EmailProvider.Mailjet.EncryptedAPIKey)
				assert.Equal(t, "STORED-SECRET", got.EmailProvider.Mailjet.EncryptedSecretKey)
			},
		},
		{
			name: "sendgrid",
			stored: domain.EmailProvider{
				Kind: domain.EmailProviderKindSendGrid, RateLimitPerMinute: 25, Senders: senders,
				SendGrid: &domain.SendGridSettings{EncryptedAPIKey: "STORED"},
			},
			incoming: domain.EmailProvider{
				Kind: domain.EmailProviderKindSendGrid, RateLimitPerMinute: 30, Senders: senders,
				SendGrid: &domain.SendGridSettings{},
			},
			verify: func(t *testing.T, got *domain.Integration) {
				assert.Equal(t, "STORED", got.EmailProvider.SendGrid.EncryptedAPIKey)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			const workspaceID = "testworkspace"
			const integrationID = "integration123"

			mockRepo := mocks.NewMockWorkspaceRepository(ctrl)
			mockAuthService := mocks.NewMockAuthService(ctrl)
			mockLogger := pkgmocks.NewMockLogger(ctrl)
			mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
			mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
			mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

			service := NewWorkspaceService(
				mockRepo,
				mocks.NewMockUserRepository(ctrl),
				mocks.NewMockTaskRepository(ctrl),
				mockLogger,
				mocks.NewMockUserServiceInterface(ctrl),
				mockAuthService,
				pkgmocks.NewMockMailer(ctrl),
				&config.Config{RootEmail: "root@example.com"},
				mocks.NewMockContactService(ctrl),
				mocks.NewMockListService(ctrl),
				mocks.NewMockContactListService(ctrl),
				mocks.NewMockTemplateService(ctrl),
				mocks.NewMockWebhookRegistrationService(ctrl),
				"secret_key",
				&SupabaseService{},
				&DNSVerificationService{},
				&BlogService{},
			)

			ctx := context.Background()
			mockAuthService.EXPECT().
				AuthenticateUserForWorkspace(ctx, workspaceID).
				Return(ctx, &domain.User{ID: "u1"}, &domain.UserWorkspace{
					UserID: "u1", WorkspaceID: workspaceID, Role: "owner",
				}, nil)

			mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(&domain.Workspace{
				ID:   workspaceID,
				Name: "Test",
				Integrations: domain.Integrations{{
					ID:            integrationID,
					Name:          "Original",
					Type:          domain.IntegrationTypeEmail,
					EmailProvider: tc.stored,
				}},
			}, nil)

			var saved *domain.Workspace
			mockRepo.EXPECT().Update(ctx, gomock.Any()).
				DoAndReturn(func(_ context.Context, w *domain.Workspace) error {
					saved = w
					return nil
				})

			err := service.UpdateIntegration(ctx, domain.UpdateIntegrationRequest{
				WorkspaceID:   workspaceID,
				IntegrationID: integrationID,
				Name:          "Renamed",
				Provider:      tc.incoming,
			})

			require.NoError(t, err, "an editor without the credential must still be able to edit")
			require.NotNil(t, saved)

			got := saved.GetIntegrationByID(integrationID)
			require.NotNil(t, got)
			assert.Equal(t, "Renamed", got.Name, "the edit itself must apply")
			tc.verify(t, got)
		})
	}
}

// The counterpart: a credential actually typed in must replace the stored one.
func TestUpdateIntegration_RotatesWhenACredentialIsSupplied(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	const workspaceID = "testworkspace"
	const integrationID = "integration123"

	mockRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

	service := NewWorkspaceService(
		mockRepo,
		mocks.NewMockUserRepository(ctrl),
		mocks.NewMockTaskRepository(ctrl),
		mockLogger,
		mocks.NewMockUserServiceInterface(ctrl),
		mockAuthService,
		pkgmocks.NewMockMailer(ctrl),
		&config.Config{RootEmail: "root@example.com"},
		mocks.NewMockContactService(ctrl),
		mocks.NewMockListService(ctrl),
		mocks.NewMockContactListService(ctrl),
		mocks.NewMockTemplateService(ctrl),
		mocks.NewMockWebhookRegistrationService(ctrl),
		"secret_key",
		&SupabaseService{},
		&DNSVerificationService{},
		&BlogService{},
	)

	senders := []domain.EmailSender{domain.NewEmailSender("sender@example.com", "Sender")}
	ctx := context.Background()

	mockAuthService.EXPECT().
		AuthenticateUserForWorkspace(ctx, workspaceID).
		Return(ctx, &domain.User{ID: "u1"}, &domain.UserWorkspace{
			UserID: "u1", WorkspaceID: workspaceID, Role: "owner",
		}, nil)

	mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(&domain.Workspace{
		ID: workspaceID, Name: "Test",
		Integrations: domain.Integrations{{
			ID: integrationID, Name: "Original", Type: domain.IntegrationTypeEmail,
			EmailProvider: domain.EmailProvider{
				Kind: domain.EmailProviderKindSMTP, RateLimitPerMinute: 25, Senders: senders,
				SMTP: &domain.SMTPSettings{Host: "h", Port: 587, EncryptedPassword: "STORED"},
			},
		}},
	}, nil)

	var saved *domain.Workspace
	mockRepo.EXPECT().Update(ctx, gomock.Any()).
		DoAndReturn(func(_ context.Context, w *domain.Workspace) error {
			saved = w
			return nil
		})

	err := service.UpdateIntegration(ctx, domain.UpdateIntegrationRequest{
		WorkspaceID:   workspaceID,
		IntegrationID: integrationID,
		Name:          "Renamed",
		Provider: domain.EmailProvider{
			Kind: domain.EmailProviderKindSMTP, RateLimitPerMinute: 25, Senders: senders,
			SMTP: &domain.SMTPSettings{Host: "h", Port: 587, Password: "a-new-password"},
		},
	})

	require.NoError(t, err)
	got := saved.GetIntegrationByID(integrationID)
	require.NotNil(t, got)
	assert.NotEmpty(t, got.EmailProvider.SMTP.EncryptedPassword)
	assert.NotEqual(t, "STORED", got.EmailProvider.SMTP.EncryptedPassword,
		"a typed-in credential must actually replace the stored one")
}
