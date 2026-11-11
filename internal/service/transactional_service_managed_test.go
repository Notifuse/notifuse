package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A notification managed by an integration keeps its name and channels, and cannot be deleted.
// Each refusal is a ValidationError, so the API answers 400 with the reason rather than 500.
// The mocks are strict: no template read, Update or Delete may happen on a refusal.
func TestTransactionalNotificationService_IntegrationManagedRefusals(t *testing.T) {
	ctx := context.Background()
	workspace := "test-workspace"
	notificationID := "magic-link"
	integrationID := "supabase-integration"

	setup := func(t *testing.T) (*TransactionalNotificationService, *mocks.MockTransactionalNotificationRepository) {
		ctrl := gomock.NewController(t)
		mockRepo := mocks.NewMockTransactionalNotificationRepository(ctrl)
		mockAuthService := mocks.NewMockAuthService(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)
		mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspace).
			Return(ctx, &domain.User{ID: "user-123"}, &domain.UserWorkspace{
				UserID:      "user-123",
				WorkspaceID: workspace,
				Role:        "member",
				Permissions: domain.UserPermissions{
					domain.PermissionResourceTransactional: {Read: true, Write: true},
				},
			}, nil)
		mockRepo.EXPECT().
			Get(gomock.Any(), workspace, notificationID).
			Return(&domain.TransactionalNotification{
				ID:            notificationID,
				Name:          "Magic Link",
				IntegrationID: &integrationID,
				Channels: domain.ChannelTemplates{
					domain.TransactionalChannelEmail: domain.ChannelTemplate{TemplateID: "tpl-magic-link"},
				},
			}, nil)

		return &TransactionalNotificationService{
			transactionalRepo: mockRepo,
			templateService:   mocks.NewMockTemplateService(ctrl),
			logger:            mockLogger,
			authService:       mockAuthService,
		}, mockRepo
	}

	requireValidationError := func(t *testing.T, err error, about string) {
		t.Helper()
		require.Error(t, err)
		var validationErr domain.ValidationError
		require.True(t, errors.As(err, &validationErr), "want a ValidationError, got %T: %v", err, err)
		assert.Contains(t, validationErr.Message, about)
		assert.Contains(t, validationErr.Message, integrationID)
	}

	t.Run("a changed name is refused", func(t *testing.T) {
		svc, _ := setup(t)
		result, err := svc.UpdateNotification(ctx, workspace, notificationID, domain.TransactionalNotificationUpdateParams{
			Name: "Renamed",
		})
		assert.Nil(t, result)
		requireValidationError(t, err, "cannot update name")
	})

	// The stored name rides along, as the console sends it, so only the channels can stop it.
	t.Run("channels are refused", func(t *testing.T) {
		svc, _ := setup(t)
		result, err := svc.UpdateNotification(ctx, workspace, notificationID, domain.TransactionalNotificationUpdateParams{
			Name: "Magic Link",
			Channels: domain.ChannelTemplates{
				domain.TransactionalChannelEmail: domain.ChannelTemplate{TemplateID: "tpl-other"},
			},
		})
		assert.Nil(t, result)
		requireValidationError(t, err, "cannot update channels")
	})

	t.Run("the stored name with a new description is saved", func(t *testing.T) {
		svc, mockRepo := setup(t)
		mockRepo.EXPECT().
			Update(gomock.Any(), workspace, gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, n *domain.TransactionalNotification) error {
				assert.Equal(t, "Magic Link", n.Name)
				assert.Equal(t, "Sent on sign-in", n.Description)
				assert.Equal(t, "tpl-magic-link", n.Channels[domain.TransactionalChannelEmail].TemplateID)
				return nil
			})
		result, err := svc.UpdateNotification(ctx, workspace, notificationID, domain.TransactionalNotificationUpdateParams{
			Name:        "Magic Link",
			Description: "Sent on sign-in",
		})
		require.NoError(t, err)
		assert.Equal(t, "Sent on sign-in", result.Description)
	})

	t.Run("delete is refused", func(t *testing.T) {
		svc, _ := setup(t)
		err := svc.DeleteNotification(ctx, workspace, notificationID)
		requireValidationError(t, err, "cannot delete")
	})
}
