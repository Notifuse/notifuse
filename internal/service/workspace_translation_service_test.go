package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	domainmocks "github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/internal/service"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func setupWorkspaceTranslationServiceTest(ctrl *gomock.Controller) (
	*service.WorkspaceTranslationService,
	*domainmocks.MockWorkspaceTranslationRepository,
	*domainmocks.MockAuthService,
	*pkgmocks.MockLogger,
) {
	mockRepo := domainmocks.NewMockWorkspaceTranslationRepository(ctrl)
	mockAuthService := domainmocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	svc := service.NewWorkspaceTranslationService(mockRepo, mockAuthService, mockLogger)
	return svc, mockRepo, mockAuthService, mockLogger
}

// ---------------------------------------------------------------------------
// Upsert
// ---------------------------------------------------------------------------

func TestWorkspaceTranslationService_Upsert(t *testing.T) {
	ctx := context.Background()
	workspaceID := "ws-123"
	userID := "user-1"

	validReq := domain.UpsertWorkspaceTranslationRequest{
		WorkspaceID: workspaceID,
		Locale:      "fr",
		Content:     domain.MapOfAny{"greeting": "Bonjour"},
	}

	t.Run("Success", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		svc, mockRepo, mockAuth, _ := setupWorkspaceTranslationServiceTest(ctrl)

		mockAuth.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceWorkspace: {Read: true, Write: true},
			},
		}, nil)

		mockRepo.EXPECT().Upsert(ctx, workspaceID, gomock.Any()).Return(nil)

		err := svc.Upsert(ctx, validReq)
		assert.NoError(t, err)
	})

	t.Run("Validation error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		svc, _, _, _ := setupWorkspaceTranslationServiceTest(ctrl)

		invalidReq := domain.UpsertWorkspaceTranslationRequest{
			WorkspaceID: "",
			Locale:      "fr",
			Content:     domain.MapOfAny{"greeting": "Bonjour"},
		}

		err := svc.Upsert(ctx, invalidReq)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "workspace_id is required")
	})

	t.Run("Auth error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		svc, _, mockAuth, _ := setupWorkspaceTranslationServiceTest(ctrl)

		authErr := errors.New("auth error")
		mockAuth.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, nil, nil, authErr)

		err := svc.Upsert(ctx, validReq)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to authenticate user")
	})

	t.Run("Permission error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		svc, _, mockAuth, _ := setupWorkspaceTranslationServiceTest(ctrl)

		mockAuth.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceWorkspace: {Read: true, Write: false},
			},
		}, nil)

		err := svc.Upsert(ctx, validReq)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Insufficient permissions")
	})

	t.Run("System call bypass", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		svc, mockRepo, _, _ := setupWorkspaceTranslationServiceTest(ctrl)

		systemCtx := context.WithValue(ctx, domain.SystemCallKey, true)
		mockRepo.EXPECT().Upsert(systemCtx, workspaceID, gomock.Any()).Return(nil)

		err := svc.Upsert(systemCtx, validReq)
		assert.NoError(t, err)
	})

	t.Run("Repo error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		svc, mockRepo, mockAuth, _ := setupWorkspaceTranslationServiceTest(ctrl)

		mockAuth.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceWorkspace: {Read: true, Write: true},
			},
		}, nil)

		repoErr := errors.New("db error")
		mockRepo.EXPECT().Upsert(ctx, workspaceID, gomock.Any()).Return(repoErr)

		err := svc.Upsert(ctx, validReq)
		assert.Error(t, err)
		assert.Equal(t, repoErr, err)
	})
}

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------

func TestWorkspaceTranslationService_List(t *testing.T) {
	ctx := context.Background()
	workspaceID := "ws-123"
	userID := "user-1"

	t.Run("Success", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		svc, mockRepo, mockAuth, _ := setupWorkspaceTranslationServiceTest(ctrl)

		mockAuth.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceWorkspace: {Read: true, Write: false},
			},
		}, nil)

		expected := []*domain.WorkspaceTranslation{
			{Locale: "fr", Content: domain.MapOfAny{"greeting": "Bonjour"}},
			{Locale: "es", Content: domain.MapOfAny{"greeting": "Hola"}},
		}
		mockRepo.EXPECT().List(ctx, workspaceID).Return(expected, nil)

		result, err := svc.List(ctx, workspaceID)
		assert.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("Auth error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		svc, _, mockAuth, _ := setupWorkspaceTranslationServiceTest(ctrl)

		authErr := errors.New("auth error")
		mockAuth.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, nil, nil, authErr)

		result, err := svc.List(ctx, workspaceID)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to authenticate user")
	})

	t.Run("Permission error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		svc, _, mockAuth, _ := setupWorkspaceTranslationServiceTest(ctrl)

		mockAuth.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceWorkspace: {Read: false, Write: false},
			},
		}, nil)

		result, err := svc.List(ctx, workspaceID)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "Insufficient permissions")
	})

	t.Run("System call bypass", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		svc, mockRepo, _, _ := setupWorkspaceTranslationServiceTest(ctrl)

		systemCtx := context.WithValue(ctx, domain.SystemCallKey, true)
		expected := []*domain.WorkspaceTranslation{
			{Locale: "fr", Content: domain.MapOfAny{"greeting": "Bonjour"}},
		}
		mockRepo.EXPECT().List(systemCtx, workspaceID).Return(expected, nil)

		result, err := svc.List(systemCtx, workspaceID)
		assert.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("Repo error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		svc, mockRepo, mockAuth, _ := setupWorkspaceTranslationServiceTest(ctrl)

		mockAuth.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceWorkspace: {Read: true, Write: false},
			},
		}, nil)

		repoErr := errors.New("db error")
		mockRepo.EXPECT().List(ctx, workspaceID).Return(nil, repoErr)

		result, err := svc.List(ctx, workspaceID)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, repoErr, err)
	})
}

// ---------------------------------------------------------------------------
// GetByLocale
// ---------------------------------------------------------------------------

func TestWorkspaceTranslationService_GetByLocale(t *testing.T) {
	ctx := context.Background()
	workspaceID := "ws-123"

	t.Run("Success", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		svc, mockRepo, _, _ := setupWorkspaceTranslationServiceTest(ctrl)

		expected := &domain.WorkspaceTranslation{
			Locale:  "fr",
			Content: domain.MapOfAny{"greeting": "Bonjour"},
		}
		mockRepo.EXPECT().GetByLocale(ctx, workspaceID, "fr").Return(expected, nil)

		result, err := svc.GetByLocale(ctx, workspaceID, "fr")
		assert.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("Repo error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		svc, mockRepo, _, _ := setupWorkspaceTranslationServiceTest(ctrl)

		repoErr := errors.New("db error")
		mockRepo.EXPECT().GetByLocale(ctx, workspaceID, "fr").Return(nil, repoErr)

		result, err := svc.GetByLocale(ctx, workspaceID, "fr")
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, repoErr, err)
	})

	t.Run("Not found returns nil", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		svc, mockRepo, _, _ := setupWorkspaceTranslationServiceTest(ctrl)

		mockRepo.EXPECT().GetByLocale(ctx, workspaceID, "xx").Return(nil, nil)

		result, err := svc.GetByLocale(ctx, workspaceID, "xx")
		assert.NoError(t, err)
		assert.Nil(t, result)
	})
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------

func TestWorkspaceTranslationService_Delete(t *testing.T) {
	ctx := context.Background()
	workspaceID := "ws-123"
	userID := "user-1"
	locale := "fr"

	t.Run("Success", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		svc, mockRepo, mockAuth, _ := setupWorkspaceTranslationServiceTest(ctrl)

		mockAuth.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceWorkspace: {Read: true, Write: true},
			},
		}, nil)

		mockRepo.EXPECT().Delete(ctx, workspaceID, locale).Return(nil)

		err := svc.Delete(ctx, workspaceID, locale)
		assert.NoError(t, err)
	})

	t.Run("Auth error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		svc, _, mockAuth, _ := setupWorkspaceTranslationServiceTest(ctrl)

		authErr := errors.New("auth error")
		mockAuth.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, nil, nil, authErr)

		err := svc.Delete(ctx, workspaceID, locale)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to authenticate user")
	})

	t.Run("Permission error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		svc, _, mockAuth, _ := setupWorkspaceTranslationServiceTest(ctrl)

		mockAuth.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceWorkspace: {Read: true, Write: false},
			},
		}, nil)

		err := svc.Delete(ctx, workspaceID, locale)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Insufficient permissions")
	})

	t.Run("System call bypass", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		svc, mockRepo, _, _ := setupWorkspaceTranslationServiceTest(ctrl)

		systemCtx := context.WithValue(ctx, domain.SystemCallKey, true)
		mockRepo.EXPECT().Delete(systemCtx, workspaceID, locale).Return(nil)

		err := svc.Delete(systemCtx, workspaceID, locale)
		assert.NoError(t, err)
	})

	t.Run("Repo error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		svc, mockRepo, mockAuth, _ := setupWorkspaceTranslationServiceTest(ctrl)

		mockAuth.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceWorkspace: {Read: true, Write: true},
			},
		}, nil)

		repoErr := errors.New("db error")
		mockRepo.EXPECT().Delete(ctx, workspaceID, locale).Return(repoErr)

		err := svc.Delete(ctx, workspaceID, locale)
		assert.Error(t, err)
		assert.Equal(t, repoErr, err)
	})
}
