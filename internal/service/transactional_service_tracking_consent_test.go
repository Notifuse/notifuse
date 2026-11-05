package service

import (
	"context"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransactionalNotificationService_SendNotification_TrackingConsent(t *testing.T) {
	const workspaceID = "test-workspace"
	const recipientEmail = "recipient@example.com"
	notificationID := uuid.New().String()
	templateID := uuid.New().String()

	userWith := func(contactsWrite bool) *domain.UserWorkspace {
		perms := domain.UserPermissions{domain.PermissionResourceTransactional: {Write: true}}
		if contactsWrite {
			perms[domain.PermissionResourceContacts] = domain.ResourcePermissions{Read: true, Write: true}
		}
		return &domain.UserWorkspace{UserID: "api-key-user", WorkspaceID: workspaceID, Role: "member", Permissions: perms}
	}

	// run sends one notification and returns the contact the upsert received and the
	// request handed to the email service.
	run := func(t *testing.T, user *domain.UserWorkspace, posted *domain.Contact, stored *domain.Contact, mode string) (*domain.Contact, domain.SendEmailRequest, *domain.TransactionalNotification) {
		t.Helper()
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		service, m := newSendOnlyTestService(t, ctrl)
		ctx := context.Background()
		m.authService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(ctx, &domain.User{ID: "api-key-user"}, user, nil)

		m.workspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(&domain.Workspace{
			ID: workspaceID,
			Settings: domain.WorkspaceSettings{
				TransactionalEmailProviderID: "integration-1",
				SecretKey:                    "test-secret-key",
				EmailTrackingEnabled:         true,
				EmailTracking:                domain.DefaultEmailTrackingSettings(),
			},
			Integrations: []domain.Integration{{
				ID:   "integration-1",
				Type: "email",
				EmailProvider: domain.EmailProvider{
					Kind:      domain.EmailProviderKindSparkPost,
					Senders:   []domain.EmailSender{domain.NewEmailSender("sender@example.com", "Test Sender")},
					SparkPost: &domain.SparkPostSettings{EncryptedAPIKey: "encrypted-api-key"},
				},
			}},
		}, nil)

		notification := &domain.TransactionalNotification{
			ID:               notificationID,
			Name:             "Receipt",
			Channels:         map[domain.TransactionalChannel]domain.ChannelTemplate{domain.TransactionalChannelEmail: {TemplateID: templateID}},
			TrackingSettings: notifuse_mjml.TrackingSettings{TrackingMode: mode, UTMSource: "app"},
		}
		m.repo.EXPECT().Get(gomock.Any(), workspaceID, notificationID).Return(notification, nil)

		var upserted *domain.Contact
		m.contactService.EXPECT().UpsertContact(gomock.Any(), workspaceID, gomock.Any()).DoAndReturn(func(_ context.Context, _ string, c *domain.Contact) domain.UpsertContactOperation {
			upserted = c
			return domain.UpsertContactOperation{Email: recipientEmail, Action: domain.UpsertContactOperationUpdate}
		})
		m.contactService.EXPECT().GetContactByEmail(gomock.Any(), workspaceID, recipientEmail).Return(stored, nil)

		var sent domain.SendEmailRequest
		m.emailService.EXPECT().SendEmailForTemplate(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, req domain.SendEmailRequest) error {
			sent = req
			return nil
		})

		_, err := service.SendNotification(ctx, workspaceID, domain.TransactionalNotificationSendParams{ID: notificationID, Contact: posted})
		require.NoError(t, err)
		return upserted, sent, notification
	}

	denied := func() *domain.NullableString { return &domain.NullableString{String: domain.TrackingConsentDenied} }

	t.Run("consent from a key with contacts:write is recorded with source api", func(t *testing.T) {
		posted := &domain.Contact{Email: recipientEmail, TrackingClicksConsent: denied()}
		upserted, _, _ := run(t, userWith(true), posted, &domain.Contact{Email: recipientEmail}, "")

		require.NotNil(t, upserted.TrackingClicksConsent)
		assert.Equal(t, domain.TrackingConsentDenied, upserted.TrackingClicksConsent.String)
		require.NotNil(t, upserted.TrackingConsentSource)
		assert.Equal(t, domain.TrackingConsentSourceAPI, upserted.TrackingConsentSource.String)
		assert.Nil(t, posted.TrackingConsentSource, "the caller's contact is not modified")
	})

	t.Run("a source given by the caller is kept", func(t *testing.T) {
		posted := &domain.Contact{Email: recipientEmail, TrackingClicksConsent: denied(), TrackingConsentSource: &domain.NullableString{String: "legacy_crm"}}
		upserted, _, _ := run(t, userWith(true), posted, &domain.Contact{Email: recipientEmail}, "")
		assert.Equal(t, "legacy_crm", upserted.TrackingConsentSource.String)
	})

	t.Run("no consent, no source", func(t *testing.T) {
		posted := &domain.Contact{Email: recipientEmail, FirstName: &domain.NullableString{String: "Ann"}}
		upserted, _, _ := run(t, userWith(true), posted, &domain.Contact{Email: recipientEmail}, "")
		assert.Same(t, posted, upserted)
		assert.Nil(t, upserted.TrackingConsentSource)
	})

	t.Run("consent from a send-only key is dropped", func(t *testing.T) {
		posted := &domain.Contact{Email: recipientEmail, TrackingOpensConsent: denied(), TrackingClicksConsent: denied()}
		upserted, _, _ := run(t, userWith(false), posted, &domain.Contact{Email: recipientEmail}, "")
		assert.Nil(t, upserted.TrackingOpensConsent)
		assert.Nil(t, upserted.TrackingClicksConsent)
		assert.Nil(t, upserted.TrackingConsentSource)
	})

	t.Run("the stored contact's consent decides, on a copy of the settings", func(t *testing.T) {
		stored := &domain.Contact{Email: recipientEmail, TrackingOpensConsent: denied()}
		_, sent, notification := run(t, userWith(false), &domain.Contact{Email: recipientEmail}, stored, "")

		assert.True(t, sent.TrackingSettings.EnableTracking)
		assert.True(t, sent.TrackingSettings.SuppressOpens)
		assert.False(t, sent.TrackingSettings.SuppressClicks)
		assert.False(t, notification.TrackingSettings.SuppressOpens, "the notification's settings are not modified")
		assert.False(t, notification.TrackingSettings.EnableTracking)
		assert.Empty(t, notification.TrackingSettings.MessageID)
	})

	t.Run("no_tracking keeps the UTM parameters and tracks nothing", func(t *testing.T) {
		_, sent, _ := run(t, userWith(false), &domain.Contact{Email: recipientEmail}, &domain.Contact{Email: recipientEmail}, notifuse_mjml.TrackingModeNoTracking)

		assert.False(t, sent.TrackingSettings.EnableTracking)
		assert.True(t, sent.TrackingSettings.SuppressIdentify)
		assert.Equal(t, notifuse_mjml.TrackingModeNoTracking, sent.TrackingSettings.TrackingMode)
		assert.Equal(t, "app", sent.TrackingSettings.UTMSource)
	})
}
