package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sendTimeSenders: the integration's default sender, and a second one a send can choose.
// domain.NewEmailSender marks every sender default, so the second is unmarked by hand.
func sendTimeSenders() (byDefault, chosen domain.EmailSender) {
	byDefault = domain.NewEmailSender("hello@example.com", "Default")
	chosen = domain.NewEmailSender("billing@example.com", "Billing")
	chosen.IsDefault = false
	return byDefault, chosen
}

// sendForTemplateWithOptions sends a template whose own sender is the default one, with the
// given email options, and returns what reached the provider (nil when nothing did).
func sendForTemplateWithOptions(t *testing.T, senders []domain.EmailSender, templateSenderID string, options domain.EmailOptions) (*domain.SendEmailProviderRequest, error) {
	t.Helper()
	ctrl := gomock.NewController(t)

	logger := pkgmocks.NewMockLogger(ctrl)
	logger.EXPECT().WithFields(gomock.Any()).Return(logger).AnyTimes()
	logger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(logger).AnyTimes()
	logger.EXPECT().Debug(gomock.Any()).AnyTimes()
	logger.EXPECT().Info(gomock.Any()).AnyTimes()
	logger.EXPECT().Warn(gomock.Any()).AnyTimes()
	logger.EXPECT().Error(gomock.Any()).AnyTimes()

	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	templateService := mocks.NewMockTemplateService(ctrl)
	messageRepo := mocks.NewMockMessageHistoryRepository(ctrl)
	sesService := mocks.NewMockEmailProviderService(ctrl)
	emailService := EmailService{
		logger:          logger,
		workspaceRepo:   workspaceRepo,
		templateService: templateService,
		messageRepo:     messageRepo,
		sesService:      sesService,
		apiEndpoint:     "https://api.example.com",
	}

	const workspaceID = "workspace-123"
	template := &domain.Template{
		ID: "template-1",
		Email: &domain.EmailTemplate{
			Subject:  "Your invoice",
			SenderID: templateSenderID,
			VisualEditorTree: &notifuse_mjml.MJMLBlock{
				BaseBlock: notifuse_mjml.NewBaseBlock("root", notifuse_mjml.MJMLComponentMjml),
			},
		},
	}
	compiledHTML := "<p>Invoice</p>"
	workspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(&domain.Workspace{
		ID:       workspaceID,
		Settings: domain.WorkspaceSettings{DefaultLanguage: "en", SecretKey: "workspace-secret-key-32-chars-min!"},
	}, nil).AnyTimes()
	templateService.EXPECT().GetTemplateByID(gomock.Any(), workspaceID, template.ID, int64(0)).Return(template, nil)
	templateService.EXPECT().CompileTemplate(gomock.Any(), gomock.Any()).
		Return(&domain.CompileTemplateResponse{Success: true, HTML: &compiledHTML}, nil).AnyTimes()
	messageRepo.EXPECT().Create(gomock.Any(), workspaceID, gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	var sent *domain.SendEmailProviderRequest
	sesService.EXPECT().SendEmail(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, req domain.SendEmailProviderRequest) error {
		sent = &req
		return nil
	}).AnyTimes()

	err := emailService.SendEmailForTemplate(context.Background(), domain.SendEmailRequest{
		WorkspaceID:      workspaceID,
		IntegrationID:    "integration-1",
		MessageID:        "message-1",
		Contact:          &domain.Contact{Email: "customer@example.com"},
		TemplateConfig:   domain.ChannelTemplate{TemplateID: template.ID},
		TrackingSettings: notifuse_mjml.TrackingSettings{EnableTracking: true},
		EmailProvider: &domain.EmailProvider{
			Kind:    domain.EmailProviderKindSES,
			Senders: senders,
			SES:     &domain.AmazonSESSettings{Region: "us-east-1"},
		},
		EmailOptions: options,
	})
	return sent, err
}

func TestEmailService_SendEmailForTemplate_SendTimeSender(t *testing.T) {
	byDefault, chosen := sendTimeSenders()
	senders := []domain.EmailSender{byDefault, chosen}

	t.Run("the email leaves from the sender the send chooses", func(t *testing.T) {
		sent, err := sendForTemplateWithOptions(t, senders, byDefault.ID, domain.EmailOptions{SenderID: chosen.ID})
		require.NoError(t, err)
		require.NotNil(t, sent)
		assert.Equal(t, "billing@example.com", sent.FromAddress)
		assert.Equal(t, "Billing", sent.FromName)
	})

	t.Run("from_name still applies on top of the chosen sender", func(t *testing.T) {
		name := "Billing team"
		sent, err := sendForTemplateWithOptions(t, senders, byDefault.ID, domain.EmailOptions{SenderID: chosen.ID, FromName: &name})
		require.NoError(t, err)
		require.NotNil(t, sent)
		assert.Equal(t, "billing@example.com", sent.FromAddress)
		assert.Equal(t, "Billing team", sent.FromName)
	})

	t.Run("without a chosen sender the template's is used", func(t *testing.T) {
		sent, err := sendForTemplateWithOptions(t, senders, chosen.ID, domain.EmailOptions{})
		require.NoError(t, err)
		require.NotNil(t, sent)
		assert.Equal(t, "billing@example.com", sent.FromAddress)
	})

	t.Run("an unknown chosen sender is an error, never the default sender", func(t *testing.T) {
		sent, err := sendForTemplateWithOptions(t, senders, byDefault.ID, domain.EmailOptions{SenderID: "no-such-sender"})
		require.Error(t, err)
		assert.True(t, errors.Is(err, domain.ErrSenderNotFound))
		assert.Contains(t, err.Error(), "sender not found")
		assert.Nil(t, sent, "nothing reaches the provider")
	})
}

func TestTransactionalNotificationService_SendNotification_SendTimeSender(t *testing.T) {
	const workspaceID = "test-workspace"
	const recipientEmail = "customer@example.com"
	notificationID := uuid.New().String()
	byDefault, chosen := sendTimeSenders()

	setup := func(t *testing.T) (*TransactionalNotificationService, *serviceMocksForSendOnly) {
		t.Helper()
		ctrl := gomock.NewController(t)
		service, m := newSendOnlyTestService(t, ctrl)
		user := &domain.UserWorkspace{UserID: "api-key-user", WorkspaceID: workspaceID, Role: "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceTransactional: {Write: true},
				domain.PermissionResourceContacts:      {Read: true, Write: true},
			}}
		m.authService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(context.Background(), &domain.User{ID: "api-key-user"}, user, nil)
		m.workspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(&domain.Workspace{
			ID: workspaceID,
			Settings: domain.WorkspaceSettings{
				TransactionalEmailProviderID: "integration-1",
				SecretKey:                    "test-secret-key",
			},
			Integrations: []domain.Integration{{
				ID:   "integration-1",
				Type: domain.IntegrationTypeEmail,
				EmailProvider: domain.EmailProvider{
					Kind:      domain.EmailProviderKindSparkPost,
					Senders:   []domain.EmailSender{byDefault, chosen},
					SparkPost: &domain.SparkPostSettings{EncryptedAPIKey: "encrypted-api-key"},
				},
			}},
		}, nil)
		m.repo.EXPECT().Get(gomock.Any(), workspaceID, notificationID).Return(&domain.TransactionalNotification{
			ID:       notificationID,
			Name:     "Invoice",
			Channels: map[domain.TransactionalChannel]domain.ChannelTemplate{domain.TransactionalChannelEmail: {TemplateID: "template-1"}},
		}, nil)
		return service, m
	}

	t.Run("the chosen sender reaches the email service", func(t *testing.T) {
		service, m := setup(t)
		m.contactService.EXPECT().UpsertContact(gomock.Any(), workspaceID, gomock.Any()).
			Return(domain.UpsertContactOperation{Email: recipientEmail, Action: domain.UpsertContactOperationUpdate})
		m.contactService.EXPECT().GetContactByEmail(gomock.Any(), workspaceID, recipientEmail).Return(&domain.Contact{Email: recipientEmail}, nil)
		m.msgHistoryRepo.EXPECT().GetByExternalID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
		m.emailService.EXPECT().SendEmailForTemplate(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, req domain.SendEmailRequest) error {
			assert.Equal(t, chosen.ID, req.EmailOptions.SenderID)
			return nil
		})

		_, err := service.SendNotification(context.Background(), workspaceID, domain.TransactionalNotificationSendParams{
			ID:           notificationID,
			Contact:      &domain.Contact{Email: recipientEmail},
			EmailOptions: domain.EmailOptions{SenderID: chosen.ID},
		})
		require.NoError(t, err)
	})

	for name, externalID := range map[string]*string{"without an external_id": nil, "with an external_id": strPtr("order-42")} {
		t.Run("an unknown sender is refused before the contact is touched, "+name, func(t *testing.T) {
			service, m := setup(t)
			m.contactService.EXPECT().UpsertContact(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
			m.contactService.EXPECT().GetContactByEmail(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
			m.msgHistoryRepo.EXPECT().GetByExternalID(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
			m.emailService.EXPECT().SendEmailForTemplate(gomock.Any(), gomock.Any()).Times(0)

			_, err := service.SendNotification(context.Background(), workspaceID, domain.TransactionalNotificationSendParams{
				ID:           notificationID,
				ExternalID:   externalID,
				Contact:      &domain.Contact{Email: recipientEmail},
				EmailOptions: domain.EmailOptions{SenderID: "no-such-sender"},
			})
			require.Error(t, err)
			assert.True(t, errors.Is(err, domain.ErrSenderNotFound))
			assert.Contains(t, err.Error(), "sender not found: no-such-sender")
		})
	}
}
