package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	domainmocks "github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/internal/service"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper function to create a test EmailBlock
func createTestEmailBlock() notifuse_mjml.EmailBlock {
	blockJSON := []byte(`{"id":"b1","type":"mj-text","content":"Hello","attributes":{"fontSize":"16px"}}`)
	blk, _ := notifuse_mjml.UnmarshalEmailBlock(blockJSON)
	return blk
}

// Helper function to create a test TemplateBlock
func createTestTemplateBlock(id, name string) *domain.TemplateBlock {
	now := time.Now().UTC()
	return &domain.TemplateBlock{
		ID:      id,
		Name:    name,
		Block:   createTestEmailBlock(),
		Created: now,
		Updated: now,
	}
}

// Setup function for template block service tests
func setupTemplateBlockServiceTest(ctrl *gomock.Controller) (*service.TemplateBlockService, *domainmocks.MockWorkspaceRepository, *domainmocks.MockAuthService, *pkgmocks.MockLogger) {
	mockRepo := domainmocks.NewMockWorkspaceRepository(ctrl)
	mockAuthService := domainmocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Setup common logger expectations
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	templateBlockService := service.NewTemplateBlockService(mockRepo, mockAuthService, mockLogger)
	return templateBlockService, mockRepo, mockAuthService, mockLogger
}

func TestTemplateBlockService_CreateTemplateBlock(t *testing.T) {
	ctx := context.Background()
	workspaceID := "ws-123"
	userID := "user-456"
	blockName := "Test Block"

	t.Run("Success", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		service, mockRepo, mockAuthService, _ := setupTemplateBlockServiceTest(ctrl)

		block := createTestTemplateBlock("", blockName)

		// Mock authentication
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceTemplates: {Read: true, Write: true},
			},
		}, nil)

		// Mock workspace retrieval
		existingWorkspace := &domain.Workspace{
			ID:   workspaceID,
			Name: "Test Workspace",
			Settings: domain.WorkspaceSettings{
				Timezone:       "UTC",
				TemplateBlocks: []domain.TemplateBlock{},
			},
		}
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(existingWorkspace, nil)

		// Mock workspace update
		mockRepo.EXPECT().Update(ctx, gomock.Any()).DoAndReturn(func(ctx context.Context, ws *domain.Workspace) error {
			assert.Len(t, ws.Settings.TemplateBlocks, 1)
			assert.NotEmpty(t, ws.Settings.TemplateBlocks[0].ID)
			assert.Equal(t, blockName, ws.Settings.TemplateBlocks[0].Name)
			return nil
		})

		err := service.CreateTemplateBlock(ctx, workspaceID, block)

		assert.NoError(t, err)
		assert.NotEmpty(t, block.ID)
	})

	t.Run("Authentication Failure", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		service, _, mockAuthService, _ := setupTemplateBlockServiceTest(ctrl)

		block := createTestTemplateBlock("", blockName)
		authErr := errors.New("auth error")

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, nil, nil, authErr)

		err := service.CreateTemplateBlock(ctx, workspaceID, block)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to authenticate user")
	})

	t.Run("Permission Denied", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		service, _, mockAuthService, _ := setupTemplateBlockServiceTest(ctrl)

		block := createTestTemplateBlock("", blockName)

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceTemplates: {Read: true, Write: false},
			},
		}, nil)

		err := service.CreateTemplateBlock(ctx, workspaceID, block)

		assert.Error(t, err)
		var permErr *domain.PermissionError
		assert.ErrorAs(t, err, &permErr)
	})

	t.Run("Validation Failure - Missing Name", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := domainmocks.NewMockWorkspaceRepository(ctrl)
		mockAuthService := domainmocks.NewMockAuthService(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)

		// Setup common logger expectations
		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

		blockService := service.NewTemplateBlockService(mockRepo, mockAuthService, mockLogger)

		block := createTestTemplateBlock("", "")
		block.Name = ""

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceTemplates: {Read: true, Write: true},
			},
		}, nil)

		existingWorkspace := &domain.Workspace{
			ID:   workspaceID,
			Name: "Test Workspace",
			Settings: domain.WorkspaceSettings{
				Timezone:       "UTC",
				TemplateBlocks: []domain.TemplateBlock{},
			},
		}
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(existingWorkspace, nil)

		err := blockService.CreateTemplateBlock(ctx, workspaceID, block)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "name is required")
	})

	t.Run("Duplicate ID", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		service, mockRepo, mockAuthService, _ := setupTemplateBlockServiceTest(ctrl)

		existingBlockID := uuid.New().String()
		block := createTestTemplateBlock(existingBlockID, blockName)

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceTemplates: {Read: true, Write: true},
			},
		}, nil)

		existingWorkspace := &domain.Workspace{
			ID:   workspaceID,
			Name: "Test Workspace",
			Settings: domain.WorkspaceSettings{
				Timezone: "UTC",
				TemplateBlocks: []domain.TemplateBlock{
					*createTestTemplateBlock(existingBlockID, "Existing Block"),
				},
			},
		}
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(existingWorkspace, nil)

		err := service.CreateTemplateBlock(ctx, workspaceID, block)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already exists")
	})

	t.Run("Repository Error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		service, mockRepo, mockAuthService, _ := setupTemplateBlockServiceTest(ctrl)

		block := createTestTemplateBlock("", blockName)

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceTemplates: {Read: true, Write: true},
			},
		}, nil)

		existingWorkspace := &domain.Workspace{
			ID:   workspaceID,
			Name: "Test Workspace",
			Settings: domain.WorkspaceSettings{
				Timezone:       "UTC",
				TemplateBlocks: []domain.TemplateBlock{},
			},
		}
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(existingWorkspace, nil)
		mockRepo.EXPECT().Update(ctx, gomock.Any()).Return(errors.New("database error"))

		err := service.CreateTemplateBlock(ctx, workspaceID, block)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create template block")
	})
}

func TestTemplateBlockService_GetTemplateBlock(t *testing.T) {
	ctx := context.Background()
	workspaceID := "ws-123"
	userID := "user-456"
	blockID := uuid.New().String()

	t.Run("Success", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		service, mockRepo, mockAuthService, _ := setupTemplateBlockServiceTest(ctrl)

		expectedBlock := createTestTemplateBlock(blockID, "Test Block")

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceTemplates: {Read: true, Write: true},
			},
		}, nil)

		existingWorkspace := &domain.Workspace{
			ID:   workspaceID,
			Name: "Test Workspace",
			Settings: domain.WorkspaceSettings{
				Timezone: "UTC",
				TemplateBlocks: []domain.TemplateBlock{
					*expectedBlock,
				},
			},
		}
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(existingWorkspace, nil)

		block, err := service.GetTemplateBlock(ctx, workspaceID, blockID)

		assert.NoError(t, err)
		assert.NotNil(t, block)
		assert.Equal(t, blockID, block.ID)
		assert.Equal(t, "Test Block", block.Name)
	})

	t.Run("Authentication Failure", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		service, _, mockAuthService, _ := setupTemplateBlockServiceTest(ctrl)

		authErr := errors.New("auth error")
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, nil, nil, authErr)

		block, err := service.GetTemplateBlock(ctx, workspaceID, blockID)

		assert.Error(t, err)
		assert.Nil(t, block)
		assert.Contains(t, err.Error(), "failed to authenticate user")
	})

	t.Run("Permission Denied", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		service, _, mockAuthService, _ := setupTemplateBlockServiceTest(ctrl)

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceTemplates: {Read: false, Write: false},
			},
		}, nil)

		block, err := service.GetTemplateBlock(ctx, workspaceID, blockID)

		assert.Error(t, err)
		assert.Nil(t, block)
		var permErr *domain.PermissionError
		assert.ErrorAs(t, err, &permErr)
	})

	t.Run("Block Not Found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		service, mockRepo, mockAuthService, _ := setupTemplateBlockServiceTest(ctrl)

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceTemplates: {Read: true, Write: true},
			},
		}, nil)

		existingWorkspace := &domain.Workspace{
			ID:   workspaceID,
			Name: "Test Workspace",
			Settings: domain.WorkspaceSettings{
				Timezone:       "UTC",
				TemplateBlocks: []domain.TemplateBlock{},
			},
		}
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(existingWorkspace, nil)

		block, err := service.GetTemplateBlock(ctx, workspaceID, blockID)

		assert.Error(t, err)
		assert.Nil(t, block)
		var notFoundErr *domain.ErrTemplateBlockNotFound
		assert.ErrorAs(t, err, &notFoundErr)
	})

	t.Run("Repository Error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		service, mockRepo, mockAuthService, _ := setupTemplateBlockServiceTest(ctrl)

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceTemplates: {Read: true, Write: true},
			},
		}, nil)

		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(nil, errors.New("database error"))

		block, err := service.GetTemplateBlock(ctx, workspaceID, blockID)

		assert.Error(t, err)
		assert.Nil(t, block)
		assert.Contains(t, err.Error(), "failed to get workspace")
	})
}

func TestTemplateBlockService_ListTemplateBlocks(t *testing.T) {
	ctx := context.Background()
	workspaceID := "ws-123"
	userID := "user-456"

	t.Run("Success - Empty List", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		service, mockRepo, mockAuthService, _ := setupTemplateBlockServiceTest(ctrl)

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceTemplates: {Read: true, Write: true},
			},
		}, nil)

		existingWorkspace := &domain.Workspace{
			ID:   workspaceID,
			Name: "Test Workspace",
			Settings: domain.WorkspaceSettings{
				Timezone:       "UTC",
				TemplateBlocks: []domain.TemplateBlock{},
			},
		}
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(existingWorkspace, nil)

		blocks, err := service.ListTemplateBlocks(ctx, workspaceID)

		assert.NoError(t, err)
		assert.NotNil(t, blocks)
		assert.Len(t, blocks, 0)
	})

	t.Run("Success - With Blocks", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		service, mockRepo, mockAuthService, _ := setupTemplateBlockServiceTest(ctrl)

		block1 := createTestTemplateBlock(uuid.New().String(), "Block 1")
		block2 := createTestTemplateBlock(uuid.New().String(), "Block 2")

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceTemplates: {Read: true, Write: true},
			},
		}, nil)

		existingWorkspace := &domain.Workspace{
			ID:   workspaceID,
			Name: "Test Workspace",
			Settings: domain.WorkspaceSettings{
				Timezone: "UTC",
				TemplateBlocks: []domain.TemplateBlock{
					*block1,
					*block2,
				},
			},
		}
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(existingWorkspace, nil)

		blocks, err := service.ListTemplateBlocks(ctx, workspaceID)

		assert.NoError(t, err)
		assert.NotNil(t, blocks)
		assert.Len(t, blocks, 2)
	})

	t.Run("Authentication Failure", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		service, _, mockAuthService, _ := setupTemplateBlockServiceTest(ctrl)

		authErr := errors.New("auth error")
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, nil, nil, authErr)

		blocks, err := service.ListTemplateBlocks(ctx, workspaceID)

		assert.Error(t, err)
		assert.Nil(t, blocks)
		assert.Contains(t, err.Error(), "failed to authenticate user")
	})

	t.Run("Permission Denied", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		service, _, mockAuthService, _ := setupTemplateBlockServiceTest(ctrl)

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceTemplates: {Read: false, Write: false},
			},
		}, nil)

		blocks, err := service.ListTemplateBlocks(ctx, workspaceID)

		assert.Error(t, err)
		assert.Nil(t, blocks)
		var permErr *domain.PermissionError
		assert.ErrorAs(t, err, &permErr)
	})

	t.Run("Repository Error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		service, mockRepo, mockAuthService, _ := setupTemplateBlockServiceTest(ctrl)

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceTemplates: {Read: true, Write: true},
			},
		}, nil)

		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(nil, errors.New("database error"))

		blocks, err := service.ListTemplateBlocks(ctx, workspaceID)

		assert.Error(t, err)
		assert.Nil(t, blocks)
		assert.Contains(t, err.Error(), "failed to get workspace")
	})
}

func TestTemplateBlockService_UpdateTemplateBlock(t *testing.T) {
	ctx := context.Background()
	workspaceID := "ws-123"
	userID := "user-456"
	blockID := uuid.New().String()

	t.Run("Success", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		service, mockRepo, mockAuthService, _ := setupTemplateBlockServiceTest(ctrl)

		existingBlock := createTestTemplateBlock(blockID, "Old Name")
		updatedBlock := createTestTemplateBlock(blockID, "New Name")

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceTemplates: {Read: true, Write: true},
			},
		}, nil)

		existingWorkspace := &domain.Workspace{
			ID:   workspaceID,
			Name: "Test Workspace",
			Settings: domain.WorkspaceSettings{
				Timezone: "UTC",
				TemplateBlocks: []domain.TemplateBlock{
					*existingBlock,
				},
			},
		}
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(existingWorkspace, nil)
		mockRepo.EXPECT().Update(ctx, gomock.Any()).DoAndReturn(func(ctx context.Context, ws *domain.Workspace) error {
			require.Len(t, ws.Settings.TemplateBlocks, 1)
			assert.Equal(t, "New Name", ws.Settings.TemplateBlocks[0].Name)
			assert.Equal(t, existingBlock.Created, ws.Settings.TemplateBlocks[0].Created)
			return nil
		})

		err := service.UpdateTemplateBlock(ctx, workspaceID, updatedBlock)

		assert.NoError(t, err)
	})

	t.Run("Authentication Failure", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		service, _, mockAuthService, _ := setupTemplateBlockServiceTest(ctrl)

		block := createTestTemplateBlock(blockID, "Updated Block")
		authErr := errors.New("auth error")

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, nil, nil, authErr)

		err := service.UpdateTemplateBlock(ctx, workspaceID, block)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to authenticate user")
	})

	t.Run("Permission Denied", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		service, _, mockAuthService, _ := setupTemplateBlockServiceTest(ctrl)

		block := createTestTemplateBlock(blockID, "Updated Block")

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceTemplates: {Read: true, Write: false},
			},
		}, nil)

		err := service.UpdateTemplateBlock(ctx, workspaceID, block)

		assert.Error(t, err)
		var permErr *domain.PermissionError
		assert.ErrorAs(t, err, &permErr)
	})

	t.Run("Block Not Found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		service, mockRepo, mockAuthService, _ := setupTemplateBlockServiceTest(ctrl)

		block := createTestTemplateBlock(blockID, "Updated Block")

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceTemplates: {Read: true, Write: true},
			},
		}, nil)

		existingWorkspace := &domain.Workspace{
			ID:   workspaceID,
			Name: "Test Workspace",
			Settings: domain.WorkspaceSettings{
				Timezone:       "UTC",
				TemplateBlocks: []domain.TemplateBlock{},
			},
		}
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(existingWorkspace, nil)

		err := service.UpdateTemplateBlock(ctx, workspaceID, block)

		assert.Error(t, err)
		var notFoundErr *domain.ErrTemplateBlockNotFound
		assert.ErrorAs(t, err, &notFoundErr)
	})

	t.Run("Validation Failure", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		service, mockRepo, mockAuthService, _ := setupTemplateBlockServiceTest(ctrl)

		block := createTestTemplateBlock(blockID, "")

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceTemplates: {Read: true, Write: true},
			},
		}, nil)

		existingWorkspace := &domain.Workspace{
			ID:   workspaceID,
			Name: "Test Workspace",
			Settings: domain.WorkspaceSettings{
				Timezone:       "UTC",
				TemplateBlocks: []domain.TemplateBlock{},
			},
		}
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(existingWorkspace, nil)

		err := service.UpdateTemplateBlock(ctx, workspaceID, block)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "name is required")
	})

	t.Run("Repository Error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		service, mockRepo, mockAuthService, _ := setupTemplateBlockServiceTest(ctrl)

		existingBlock := createTestTemplateBlock(blockID, "Old Name")
		updatedBlock := createTestTemplateBlock(blockID, "New Name")

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceTemplates: {Read: true, Write: true},
			},
		}, nil)

		existingWorkspace := &domain.Workspace{
			ID:   workspaceID,
			Name: "Test Workspace",
			Settings: domain.WorkspaceSettings{
				Timezone: "UTC",
				TemplateBlocks: []domain.TemplateBlock{
					*existingBlock,
				},
			},
		}
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(existingWorkspace, nil)
		mockRepo.EXPECT().Update(ctx, gomock.Any()).Return(errors.New("database error"))

		err := service.UpdateTemplateBlock(ctx, workspaceID, updatedBlock)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to update template block")
	})
}

func TestTemplateBlockService_DeleteTemplateBlock(t *testing.T) {
	ctx := context.Background()
	workspaceID := "ws-123"
	userID := "user-456"
	blockID := uuid.New().String()

	t.Run("Success", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		service, mockRepo, mockAuthService, _ := setupTemplateBlockServiceTest(ctrl)

		block1 := createTestTemplateBlock(blockID, "Block 1")
		block2 := createTestTemplateBlock(uuid.New().String(), "Block 2")

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceTemplates: {Read: true, Write: true},
			},
		}, nil)

		existingWorkspace := &domain.Workspace{
			ID:   workspaceID,
			Name: "Test Workspace",
			Settings: domain.WorkspaceSettings{
				Timezone: "UTC",
				TemplateBlocks: []domain.TemplateBlock{
					*block1,
					*block2,
				},
			},
		}
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(existingWorkspace, nil)
		mockRepo.EXPECT().Update(ctx, gomock.Any()).DoAndReturn(func(ctx context.Context, ws *domain.Workspace) error {
			assert.Len(t, ws.Settings.TemplateBlocks, 1)
			assert.Equal(t, block2.ID, ws.Settings.TemplateBlocks[0].ID)
			return nil
		})

		err := service.DeleteTemplateBlock(ctx, workspaceID, blockID)

		assert.NoError(t, err)
	})

	t.Run("Authentication Failure", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		service, _, mockAuthService, _ := setupTemplateBlockServiceTest(ctrl)

		authErr := errors.New("auth error")
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, nil, nil, authErr)

		err := service.DeleteTemplateBlock(ctx, workspaceID, blockID)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to authenticate user")
	})

	t.Run("Permission Denied", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		service, _, mockAuthService, _ := setupTemplateBlockServiceTest(ctrl)

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceTemplates: {Read: true, Write: false},
			},
		}, nil)

		err := service.DeleteTemplateBlock(ctx, workspaceID, blockID)

		assert.Error(t, err)
		var permErr *domain.PermissionError
		assert.ErrorAs(t, err, &permErr)
	})

	t.Run("Block Not Found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		service, mockRepo, mockAuthService, _ := setupTemplateBlockServiceTest(ctrl)

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceTemplates: {Read: true, Write: true},
			},
		}, nil)

		existingWorkspace := &domain.Workspace{
			ID:   workspaceID,
			Name: "Test Workspace",
			Settings: domain.WorkspaceSettings{
				Timezone:       "UTC",
				TemplateBlocks: []domain.TemplateBlock{},
			},
		}
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(existingWorkspace, nil)

		err := service.DeleteTemplateBlock(ctx, workspaceID, blockID)

		assert.Error(t, err)
		var notFoundErr *domain.ErrTemplateBlockNotFound
		assert.ErrorAs(t, err, &notFoundErr)
	})

	t.Run("Repository Error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		service, mockRepo, mockAuthService, _ := setupTemplateBlockServiceTest(ctrl)

		block := createTestTemplateBlock(blockID, "Block 1")

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceTemplates: {Read: true, Write: true},
			},
		}, nil)

		existingWorkspace := &domain.Workspace{
			ID:   workspaceID,
			Name: "Test Workspace",
			Settings: domain.WorkspaceSettings{
				Timezone: "UTC",
				TemplateBlocks: []domain.TemplateBlock{
					*block,
				},
			},
		}
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(existingWorkspace, nil)
		mockRepo.EXPECT().Update(ctx, gomock.Any()).Return(errors.New("database error"))

		err := service.DeleteTemplateBlock(ctx, workspaceID, blockID)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to delete template block")
	})
}
