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
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
)

// A transactional send that cannot go out returns an error instead of a message
// ID, so the caller knows to retry or alert. The email service mock only expects
// a send where a case says so: any other send fails the test.
func TestTransactionalNotificationService_SendNotification_NothingSent(t *testing.T) {
	const workspaceID = "test-workspace"
	contact := &domain.Contact{Email: "recipient@example.com"}
	provider := domain.Integration{
		ID:   "integration-1",
		Name: "Test Integration",
		Type: "email",
		EmailProvider: domain.EmailProvider{
			Kind:      domain.EmailProviderKindSparkPost,
			Senders:   []domain.EmailSender{domain.NewEmailSender("test@example.com", "Test Sender")},
			SparkPost: &domain.SparkPostSettings{EncryptedAPIKey: "encrypted-api-key"},
		},
	}

	cases := []struct {
		name            string
		transactionalID string
		integrations    []domain.Integration
		channels        []domain.TransactionalChannel
		sendErr         error
		expectSend      bool
		wantErr         string
	}{
		{
			name:            "requested channel is not configured on the notification",
			transactionalID: "integration-1",
			integrations:    []domain.Integration{provider},
			channels:        []domain.TransactionalChannel{"sms"},
			wantErr:         "no valid channels",
		},
		{
			name:            "no transactional provider selected",
			transactionalID: "",
			integrations:    []domain.Integration{provider},
			wantErr:         "no email provider configured",
		},
		{
			name:            "selected provider was deleted",
			transactionalID: "integration-gone",
			integrations:    []domain.Integration{provider},
			wantErr:         "integration with ID integration-gone not found",
		},
		{
			name:            "the provider refuses the email",
			transactionalID: "integration-1",
			integrations:    []domain.Integration{provider},
			sendErr:         errors.New("provider rejected the message"),
			expectSend:      true,
			wantErr:         "failed to send notification through any channel",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			transactionalRepo := mocks.NewMockTransactionalNotificationRepository(ctrl)
			contactService := mocks.NewMockContactService(ctrl)
			emailService := mocks.NewMockEmailServiceInterface(ctrl)
			workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
			log := pkgmocks.NewMockLogger(ctrl)
			log.EXPECT().WithFields(gomock.Any()).Return(log).AnyTimes()
			log.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(log).AnyTimes()
			log.EXPECT().Debug(gomock.Any()).AnyTimes()
			log.EXPECT().Info(gomock.Any()).AnyTimes()
			log.EXPECT().Error(gomock.Any()).AnyTimes()

			service := &TransactionalNotificationService{
				transactionalRepo:  transactionalRepo,
				messageHistoryRepo: mocks.NewMockMessageHistoryRepository(ctrl),
				templateService:    mocks.NewMockTemplateService(ctrl),
				contactService:     contactService,
				emailService:       emailService,
				logger:             log,
				workspaceRepo:      workspaceRepo,
				apiEndpoint:        "https://api.example.com",
				authService:        mocks.NewMockAuthService(ctrl),
			}

			workspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(&domain.Workspace{
				ID:   workspaceID,
				Name: "Test Workspace",
				Settings: domain.WorkspaceSettings{
					TransactionalEmailProviderID: tc.transactionalID,
					SecretKey:                    "test-secret-key",
				},
				Integrations: tc.integrations,
			}, nil)
			transactionalRepo.EXPECT().Get(gomock.Any(), workspaceID, "welcome").Return(&domain.TransactionalNotification{
				ID:   "welcome",
				Name: "Welcome",
				Channels: map[domain.TransactionalChannel]domain.ChannelTemplate{
					domain.TransactionalChannelEmail: {TemplateID: "template-1"},
				},
			}, nil)
			contactService.EXPECT().UpsertContact(gomock.Any(), workspaceID, contact).
				Return(domain.UpsertContactOperation{Email: contact.Email, Action: domain.UpsertContactOperationUpdate})
			contactService.EXPECT().GetContactByEmail(gomock.Any(), workspaceID, contact.Email).Return(contact, nil)
			if tc.expectSend {
				emailService.EXPECT().SendEmailForTemplate(gomock.Any(), gomock.Any()).Return(tc.sendErr)
			}

			systemCtx := context.WithValue(context.Background(), domain.SystemCallKey, true)
			messageID, err := service.SendNotification(systemCtx, workspaceID, domain.TransactionalNotificationSendParams{
				ID:       "welcome",
				Contact:  contact,
				Channels: tc.channels,
			})

			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
			assert.Empty(t, messageID)
		})
	}
}
