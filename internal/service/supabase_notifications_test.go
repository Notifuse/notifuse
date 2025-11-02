package service

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
)

func TestCreateDefaultSupabaseNotifications_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTransactionalRepo := mocks.NewMockTransactionalNotificationRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	service := NewSupabaseService(
		nil, nil, nil, nil, nil, nil, nil,
		mockTransactionalRepo,
		nil,
		mockLogger,
	)

	mappings := &domain.SupabaseTemplateMappings{
		Signup:           "template-signup",
		MagicLink:        "template-magiclink",
		Recovery:         "template-recovery",
		EmailChange:      "template-email-change",
		Invite:           "template-invite",
		Reauthentication: "template-reauthentication",
	}

	// Mock successful creation of all notifications
	mockTransactionalRepo.EXPECT().Create(gomock.Any(), "workspace-123", gomock.Any()).
		DoAndReturn(func(ctx context.Context, workspaceID string, notification *domain.TransactionalNotification) error {
			// Verify common properties
			assert.NotNil(t, notification.IntegrationID)
			assert.Equal(t, "integration-456", *notification.IntegrationID)
			assert.False(t, notification.TrackingSettings.EnableTracking)

			// Verify email channel is configured
			emailChannel, exists := notification.Channels[domain.TransactionalChannelEmail]
			assert.True(t, exists)
			assert.NotEmpty(t, emailChannel.TemplateID)

			return nil
		}).Times(6) // 6 notifications total: signup, magiclink, recovery, email_change, invite, reauthentication

	err := service.CreateDefaultSupabaseNotifications(context.Background(), "workspace-123", "integration-456", mappings)

	assert.NoError(t, err)
}

func TestCreateDefaultSupabaseNotifications_FailureOnSignup(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTransactionalRepo := mocks.NewMockTransactionalNotificationRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	service := NewSupabaseService(
		nil, nil, nil, nil, nil, nil, nil,
		mockTransactionalRepo,
		nil,
		mockLogger,
	)

	mappings := &domain.SupabaseTemplateMappings{
		Signup: "template-signup",
	}

	// Mock failure on signup notification
	mockTransactionalRepo.EXPECT().Create(gomock.Any(), "workspace-123", gomock.Any()).
		Return(assert.AnError)

	err := service.CreateDefaultSupabaseNotifications(context.Background(), "workspace-123", "integration-456", mappings)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create signup notification")
}

func TestCreateDefaultSupabaseNotifications_FailureOnMagicLink(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTransactionalRepo := mocks.NewMockTransactionalNotificationRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	service := NewSupabaseService(
		nil, nil, nil, nil, nil, nil, nil,
		mockTransactionalRepo,
		nil,
		mockLogger,
	)

	mappings := &domain.SupabaseTemplateMappings{
		Signup:    "template-signup",
		MagicLink: "template-magiclink",
	}

	// Mock successful signup
	mockTransactionalRepo.EXPECT().Create(gomock.Any(), "workspace-123", gomock.Any()).
		DoAndReturn(func(ctx context.Context, workspaceID string, notification *domain.TransactionalNotification) error {
			if notification.Name == "Signup Confirmation" {
				return nil
			}
			return assert.AnError
		}).Times(2)

	err := service.CreateDefaultSupabaseNotifications(context.Background(), "workspace-123", "integration-456", mappings)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create magic link notification")
}

func TestCreateDefaultSupabaseNotifications_VerifyNotificationIDs(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTransactionalRepo := mocks.NewMockTransactionalNotificationRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	service := NewSupabaseService(
		nil, nil, nil, nil, nil, nil, nil,
		mockTransactionalRepo,
		nil,
		mockLogger,
	)

	mappings := &domain.SupabaseTemplateMappings{
		Signup:           "template-signup",
		MagicLink:        "template-magiclink",
		Recovery:         "template-recovery",
		EmailChange:      "template-email-change",
		Reauthentication: "template-reauthentication",
		Invite:           "template-invite",
	}

	// Capture notification IDs to verify they have correct prefixes
	var notificationIDs []string

	mockTransactionalRepo.EXPECT().Create(gomock.Any(), "workspace-123", gomock.Any()).
		DoAndReturn(func(ctx context.Context, workspaceID string, notification *domain.TransactionalNotification) error {
			notificationIDs = append(notificationIDs, notification.ID)
			return nil
		}).Times(6)

	err := service.CreateDefaultSupabaseNotifications(context.Background(), "workspace-123", "integration-456", mappings)

	assert.NoError(t, err)
	assert.Len(t, notificationIDs, 6)

	// Verify all IDs have correct prefixes
	prefixes := []string{
		"supabase_signup_",
		"supabase_magiclink_",
		"supabase_recovery_",
		"supabase_email_change_",
		"supabase_invite_",
		"supabase_reauth_",
	}

	for i, id := range notificationIDs {
		assert.Contains(t, id, prefixes[i], "Notification ID should have correct prefix")
	}
}

func TestCreateDefaultSupabaseNotifications_VerifyTrackingDisabled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTransactionalRepo := mocks.NewMockTransactionalNotificationRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	service := NewSupabaseService(
		nil, nil, nil, nil, nil, nil, nil,
		mockTransactionalRepo,
		nil,
		mockLogger,
	)

	mappings := &domain.SupabaseTemplateMappings{
		Signup:           "template-signup",
		MagicLink:        "template-magiclink",
		Recovery:         "template-recovery",
		EmailChange:      "template-email-change",
		Reauthentication: "template-reauthentication",
		Invite:           "template-invite",
	}

	// Verify tracking is disabled for all notifications
	mockTransactionalRepo.EXPECT().Create(gomock.Any(), "workspace-123", gomock.Any()).
		DoAndReturn(func(ctx context.Context, workspaceID string, notification *domain.TransactionalNotification) error {
			assert.False(t, notification.TrackingSettings.EnableTracking, "Tracking should be disabled")
			return nil
		}).Times(6)

	err := service.CreateDefaultSupabaseNotifications(context.Background(), "workspace-123", "integration-456", mappings)

	assert.NoError(t, err)
}

func TestCreateDefaultSupabaseNotifications_VerifyIntegrationID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTransactionalRepo := mocks.NewMockTransactionalNotificationRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	service := NewSupabaseService(
		nil, nil, nil, nil, nil, nil, nil,
		mockTransactionalRepo,
		nil,
		mockLogger,
	)

	mappings := &domain.SupabaseTemplateMappings{
		Signup:           "template-signup",
		MagicLink:        "template-magiclink",
		Recovery:         "template-recovery",
		EmailChange:      "template-email-change",
		Reauthentication: "template-reauthentication",
		Invite:           "template-invite",
	}

	integrationID := "integration-456"

	// Verify all notifications have the correct integration ID
	mockTransactionalRepo.EXPECT().Create(gomock.Any(), "workspace-123", gomock.Any()).
		DoAndReturn(func(ctx context.Context, workspaceID string, notification *domain.TransactionalNotification) error {
			assert.NotNil(t, notification.IntegrationID)
			assert.Equal(t, integrationID, *notification.IntegrationID)
			return nil
		}).Times(6)

	err := service.CreateDefaultSupabaseNotifications(context.Background(), "workspace-123", integrationID, mappings)

	assert.NoError(t, err)
}

func TestCreateDefaultSupabaseNotifications_VerifyDescriptions(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTransactionalRepo := mocks.NewMockTransactionalNotificationRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	service := NewSupabaseService(
		nil, nil, nil, nil, nil, nil, nil,
		mockTransactionalRepo,
		nil,
		mockLogger,
	)

	mappings := &domain.SupabaseTemplateMappings{
		Signup:           "template-signup",
		MagicLink:        "template-magiclink",
		Recovery:         "template-recovery",
		EmailChange:      "template-email-change",
		Reauthentication: "template-reauthentication",
		Invite:           "template-invite",
	}

	expectedDescriptions := map[string]string{
		"Signup Confirmation": "Sends signup confirmation emails via Supabase integration",
		"Magic Link":          "Sends magic link authentication emails via Supabase integration",
		"Password Recovery":   "Sends password recovery emails via Supabase integration",
		"Email Change":        "Sends email change confirmation via Supabase integration",
		"User Invitation":     "Sends user invitation emails via Supabase integration",
		"Reauthentication":    "Sends reauthentication verification via Supabase integration",
	}

	// Verify descriptions match expected values
	mockTransactionalRepo.EXPECT().Create(gomock.Any(), "workspace-123", gomock.Any()).
		DoAndReturn(func(ctx context.Context, workspaceID string, notification *domain.TransactionalNotification) error {
			expectedDesc, ok := expectedDescriptions[notification.Name]
			assert.True(t, ok, "Unexpected notification name: %s", notification.Name)
			assert.Equal(t, expectedDesc, notification.Description)
			return nil
		}).Times(6)

	err := service.CreateDefaultSupabaseNotifications(context.Background(), "workspace-123", "integration-456", mappings)

	assert.NoError(t, err)
}

func TestCreateDefaultSupabaseNotifications_VerifyChannels(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTransactionalRepo := mocks.NewMockTransactionalNotificationRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	service := NewSupabaseService(
		nil, nil, nil, nil, nil, nil, nil,
		mockTransactionalRepo,
		nil,
		mockLogger,
	)

	mappings := &domain.SupabaseTemplateMappings{
		Signup:           "template-signup",
		MagicLink:        "template-magiclink",
		Recovery:         "template-recovery",
		EmailChange:      "template-email-change",
		Reauthentication: "template-reauthentication",
		Invite:           "template-invite",
	}

	// Verify all notifications have email channel configured
	mockTransactionalRepo.EXPECT().Create(gomock.Any(), "workspace-123", gomock.Any()).
		DoAndReturn(func(ctx context.Context, workspaceID string, notification *domain.TransactionalNotification) error {
			// Should have email channel
			emailChannel, exists := notification.Channels[domain.TransactionalChannelEmail]
			assert.True(t, exists, "Email channel should exist")
			assert.NotEmpty(t, emailChannel.TemplateID, "Template ID should not be empty")
			return nil
		}).Times(6)

	err := service.CreateDefaultSupabaseNotifications(context.Background(), "workspace-123", "integration-456", mappings)

	assert.NoError(t, err)
}

func TestCreateDefaultSupabaseNotifications_NotificationIDUniqueness(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTransactionalRepo := mocks.NewMockTransactionalNotificationRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	service := NewSupabaseService(
		nil, nil, nil, nil, nil, nil, nil,
		mockTransactionalRepo,
		nil,
		mockLogger,
	)

	mappings := &domain.SupabaseTemplateMappings{
		Signup:           "template-signup",
		MagicLink:        "template-magiclink",
		Recovery:         "template-recovery",
		EmailChange:      "template-email-change",
		Reauthentication: "template-reauthentication",
		Invite:           "template-invite",
	}

	notificationIDs := make(map[string]bool)

	mockTransactionalRepo.EXPECT().Create(gomock.Any(), "workspace-123", gomock.Any()).
		DoAndReturn(func(ctx context.Context, workspaceID string, notification *domain.TransactionalNotification) error {
			// Check that this ID hasn't been seen before
			_, exists := notificationIDs[notification.ID]
			assert.False(t, exists, "Notification ID %s should be unique", notification.ID)

			notificationIDs[notification.ID] = true
			return nil
		}).Times(6)

	err := service.CreateDefaultSupabaseNotifications(context.Background(), "workspace-123", "integration-456", mappings)

	assert.NoError(t, err)
	assert.Len(t, notificationIDs, 6, "Should have 6 unique notification IDs")
}
