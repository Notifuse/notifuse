package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
)

func TestTemplateService_GetTemplateUsage(t *testing.T) {
	ctx := context.Background()
	workspaceID := "ws-123"

	member := func(perms domain.UserPermissions) *domain.UserWorkspace {
		return &domain.UserWorkspace{UserID: "user-1", WorkspaceID: workspaceID, Role: "member", Permissions: perms}
	}
	read := domain.ResourcePermissions{Read: true}

	t.Run("authentication failure is returned", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		svc, _, _, mockAuth, _ := setupTemplateServiceTest(ctrl)
		mockAuth.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, nil, nil, errors.New("bad token"))

		_, err := svc.GetTemplateUsage(ctx, workspaceID, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "bad token")
	})

	t.Run("refused without templates read", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		svc, _, _, mockAuth, _ := setupTemplateServiceTest(ctrl)
		mockAuth.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, &domain.User{ID: "user-1"}, member(domain.UserPermissions{domain.PermissionResourceBroadcasts: read}), nil)

		_, err := svc.GetTemplateUsage(ctx, workspaceID, "")
		var permErr *domain.PermissionError
		require.ErrorAs(t, err, &permErr)
	})

	t.Run("an unreadable type is not queried and is listed as hidden", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		svc, mockRepo, _, mockAuth, _ := setupTemplateServiceTest(ctrl)
		mockAuth.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, &domain.User{ID: "user-1"}, member(domain.UserPermissions{
				domain.PermissionResourceTemplates:     read,
				domain.PermissionResourceBroadcasts:    read,
				domain.PermissionResourceTransactional: read,
			}), nil)
		repoUsage := domain.NewTemplateUsage(true)
		mockRepo.EXPECT().GetTemplateUsage(ctx, workspaceID, "welcome", []domain.TemplateUsageType{
			domain.TemplateUsageTypeBroadcasts, domain.TemplateUsageTypeTransactionalNotifications,
		}).Return(&repoUsage, nil)

		usage, err := svc.GetTemplateUsage(ctx, workspaceID, "welcome")
		require.NoError(t, err)
		assert.Equal(t, []domain.TemplateUsageType{domain.TemplateUsageTypeAutomations, domain.TemplateUsageTypeLists}, usage.HiddenTypes)
	})

	t.Run("an owner sees every type", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		svc, mockRepo, _, mockAuth, _ := setupTemplateServiceTest(ctrl)
		mockAuth.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, &domain.User{ID: "user-1"}, &domain.UserWorkspace{UserID: "user-1", WorkspaceID: workspaceID, Role: "owner"}, nil)
		repoUsage := domain.NewTemplateUsage(false)
		mockRepo.EXPECT().GetTemplateUsage(ctx, workspaceID, "", domain.TemplateUsageTypes).Return(&repoUsage, nil)

		usage, err := svc.GetTemplateUsage(ctx, workspaceID, "")
		require.NoError(t, err)
		assert.Equal(t, []domain.TemplateUsageType{}, usage.HiddenTypes)
	})

	t.Run("nothing readable: no query, every type hidden", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		svc, _, _, mockAuth, _ := setupTemplateServiceTest(ctrl)
		mockAuth.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, &domain.User{ID: "user-1"}, member(domain.UserPermissions{domain.PermissionResourceTemplates: read}), nil)

		usage, err := svc.GetTemplateUsage(ctx, workspaceID, "welcome")
		require.NoError(t, err)
		assert.Equal(t, domain.TemplateUsageTypes, usage.HiddenTypes)
		assert.Empty(t, usage.Usage)
		require.NotNil(t, usage.References)
		assert.Empty(t, *usage.References)
	})

	t.Run("repository error is wrapped", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		svc, mockRepo, _, mockAuth, mockLogger := setupTemplateServiceTest(ctrl)
		mockAuth.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, &domain.User{ID: "user-1"}, &domain.UserWorkspace{UserID: "user-1", WorkspaceID: workspaceID, Role: "owner"}, nil)
		mockRepo.EXPECT().GetTemplateUsage(ctx, workspaceID, "", gomock.Any()).Return(nil, errors.New("db down"))
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

		_, err := svc.GetTemplateUsage(ctx, workspaceID, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "db down")
	})
}
