package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

// Helper function to create test automation
func createTestAutomationService(id, workspaceID string) *domain.Automation {
	now := time.Now().UTC()
	return &domain.Automation{
		ID:          id,
		WorkspaceID: workspaceID,
		Name:        "Test Automation",
		Status:      domain.AutomationStatusDraft,
		ListID:      "list-123",
		Trigger: &domain.TimelineTriggerConfig{
			EventKinds: []string{"email.opened"},
			Frequency:  domain.TriggerFrequencyOnce,
		},
		RootNodeID: "node-root",
		Stats:      &domain.AutomationStats{},
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}

// Helper function to create test automation node
func createTestAutomationNodeService(id, automationID string, nodeType domain.NodeType) *domain.AutomationNode {
	now := time.Now().UTC()
	return &domain.AutomationNode{
		ID:           id,
		AutomationID: automationID,
		Type:         nodeType,
		Config: map[string]interface{}{
			"key": "value",
		},
		Position: domain.NodePosition{
			X: 100,
			Y: 200,
		},
		CreatedAt: now,
	}
}

func TestAutomationService_Create(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockAutomationRepository(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	service := NewAutomationService(mockRepo, mockAuthService, mockLogger)

	ctx := context.Background()
	workspaceID := "workspace-123"

	t.Run("successful create", func(t *testing.T) {
		automation := createTestAutomationService("auto-123", workspaceID)

		userWorkspace := &domain.UserWorkspace{
			UserID:      "user-123",
			WorkspaceID: workspaceID,
			Role:        "admin",
			Permissions: domain.UserPermissions{},
		}
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)
		mockRepo.EXPECT().Create(ctx, workspaceID, automation).Return(nil)

		err := service.Create(ctx, workspaceID, automation)
		assert.NoError(t, err)
	})

	t.Run("authentication failure", func(t *testing.T) {
		automation := createTestAutomationService("auto-123", workspaceID)

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(ctx, nil, nil, errors.New("auth error"))

		err := service.Create(ctx, workspaceID, automation)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to authenticate")
	})

	t.Run("validation failure", func(t *testing.T) {
		invalidAutomation := &domain.Automation{} // Missing required fields

		userWorkspace := &domain.UserWorkspace{
			UserID:      "user-123",
			WorkspaceID: workspaceID,
			Role:        "admin",
		}
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)

		err := service.Create(ctx, workspaceID, invalidAutomation)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid automation")
	})

	t.Run("repository failure", func(t *testing.T) {
		automation := createTestAutomationService("auto-123", workspaceID)

		userWorkspace := &domain.UserWorkspace{
			UserID:      "user-123",
			WorkspaceID: workspaceID,
			Role:        "admin",
		}
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)
		mockRepo.EXPECT().Create(ctx, workspaceID, automation).Return(errors.New("db error"))
		mockLogger.EXPECT().WithField("automation_id", automation.ID).Return(mockLogger)
		mockLogger.EXPECT().Error(gomock.Any())

		err := service.Create(ctx, workspaceID, automation)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create automation")
	})
}

func TestAutomationService_Get(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockAutomationRepository(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	service := NewAutomationService(mockRepo, mockAuthService, mockLogger)

	ctx := context.Background()
	workspaceID := "workspace-123"
	automationID := "auto-123"

	t.Run("successful get", func(t *testing.T) {
		expectedAutomation := createTestAutomationService(automationID, workspaceID)

		userWorkspace := &domain.UserWorkspace{
			UserID:      "user-123",
			WorkspaceID: workspaceID,
			Role:        "admin",
		}
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID, automationID).Return(expectedAutomation, nil)

		result, err := service.Get(ctx, workspaceID, automationID)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, automationID, result.ID)
	})

	t.Run("authentication failure", func(t *testing.T) {
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(ctx, nil, nil, errors.New("auth error"))

		result, err := service.Get(ctx, workspaceID, automationID)
		assert.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("not found", func(t *testing.T) {
		userWorkspace := &domain.UserWorkspace{
			UserID:      "user-123",
			WorkspaceID: workspaceID,
			Role:        "admin",
		}
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID, automationID).Return(nil, errors.New("not found"))

		result, err := service.Get(ctx, workspaceID, automationID)
		assert.Error(t, err)
		assert.Nil(t, result)
	})
}

func TestAutomationService_List(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockAutomationRepository(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	service := NewAutomationService(mockRepo, mockAuthService, mockLogger)

	ctx := context.Background()
	workspaceID := "workspace-123"

	t.Run("successful list", func(t *testing.T) {
		filter := domain.AutomationFilter{Limit: 10, Offset: 0}
		expectedAutomations := []*domain.Automation{
			createTestAutomationService("auto-1", workspaceID),
			createTestAutomationService("auto-2", workspaceID),
		}

		userWorkspace := &domain.UserWorkspace{
			UserID:      "user-123",
			WorkspaceID: workspaceID,
			Role:        "admin",
		}
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)
		mockRepo.EXPECT().List(ctx, workspaceID, filter).Return(expectedAutomations, 2, nil)

		result, count, err := service.List(ctx, workspaceID, filter)
		assert.NoError(t, err)
		assert.Len(t, result, 2)
		assert.Equal(t, 2, count)
	})

	t.Run("authentication failure", func(t *testing.T) {
		filter := domain.AutomationFilter{Limit: 10, Offset: 0}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(ctx, nil, nil, errors.New("auth error"))

		result, count, err := service.List(ctx, workspaceID, filter)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, 0, count)
	})
}

func TestAutomationService_Update(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockAutomationRepository(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	service := NewAutomationService(mockRepo, mockAuthService, mockLogger)

	ctx := context.Background()
	workspaceID := "workspace-123"

	t.Run("successful update with list_id", func(t *testing.T) {
		automation := createTestAutomationService("auto-123", workspaceID)
		automation.Name = "Updated Automation"

		userWorkspace := &domain.UserWorkspace{
			UserID:      "user-123",
			WorkspaceID: workspaceID,
			Role:        "admin",
		}
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)
		// No GetNodes call needed when list_id is set
		mockRepo.EXPECT().Update(ctx, workspaceID, automation).Return(nil)

		err := service.Update(ctx, workspaceID, automation)
		assert.NoError(t, err)
	})

	t.Run("authentication failure", func(t *testing.T) {
		automation := createTestAutomationService("auto-123", workspaceID)

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(ctx, nil, nil, errors.New("auth error"))

		err := service.Update(ctx, workspaceID, automation)
		assert.Error(t, err)
	})

	t.Run("validation failure", func(t *testing.T) {
		invalidAutomation := &domain.Automation{ID: "auto-123"}

		userWorkspace := &domain.UserWorkspace{
			UserID:      "user-123",
			WorkspaceID: workspaceID,
			Role:        "admin",
		}
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)

		err := service.Update(ctx, workspaceID, invalidAutomation)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid automation")
	})

	t.Run("remove list_id with email nodes - rejected", func(t *testing.T) {
		automation := createTestAutomationService("auto-123", workspaceID)
		automation.ListID = "" // Removing list_id
		automation.Nodes = []*domain.AutomationNode{
			createTestAutomationNodeService("node-1", "auto-123", domain.NodeTypeEmail),
		}
		automation.RootNodeID = "node-1" // Must reference a valid node

		userWorkspace := &domain.UserWorkspace{
			UserID:      "user-123",
			WorkspaceID: workspaceID,
			Role:        "admin",
		}
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)

		err := service.Update(ctx, workspaceID, automation)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot remove list_id from automation with email nodes")
	})

	t.Run("remove list_id without email nodes - allowed", func(t *testing.T) {
		automation := createTestAutomationService("auto-123", workspaceID)
		automation.ListID = "" // Removing list_id
		automation.Nodes = []*domain.AutomationNode{
			createTestAutomationNodeService("node-1", "auto-123", domain.NodeTypeDelay),
		}
		automation.RootNodeID = "node-1" // Must reference a valid node

		userWorkspace := &domain.UserWorkspace{
			UserID:      "user-123",
			WorkspaceID: workspaceID,
			Role:        "admin",
		}
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)
		mockRepo.EXPECT().Update(ctx, workspaceID, automation).Return(nil)

		err := service.Update(ctx, workspaceID, automation)
		assert.NoError(t, err)
	})
}

func TestAutomationService_Delete(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockAutomationRepository(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	service := NewAutomationService(mockRepo, mockAuthService, mockLogger)

	ctx := context.Background()
	workspaceID := "workspace-123"
	automationID := "auto-123"

	t.Run("successful delete draft automation", func(t *testing.T) {
		userWorkspace := &domain.UserWorkspace{
			UserID:      "user-123",
			WorkspaceID: workspaceID,
			Role:        "admin",
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)
		// Repository handles soft-delete directly (no GetByID needed anymore)
		mockRepo.EXPECT().Delete(ctx, workspaceID, automationID).Return(nil)

		err := service.Delete(ctx, workspaceID, automationID)
		assert.NoError(t, err)
	})

	t.Run("successful delete live automation", func(t *testing.T) {
		// Live automations can now be deleted - repository handles trigger removal
		userWorkspace := &domain.UserWorkspace{
			UserID:      "user-123",
			WorkspaceID: workspaceID,
			Role:        "admin",
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)
		mockRepo.EXPECT().Delete(ctx, workspaceID, automationID).Return(nil)

		err := service.Delete(ctx, workspaceID, automationID)
		assert.NoError(t, err)
	})

	t.Run("authentication failure", func(t *testing.T) {
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(ctx, nil, nil, errors.New("auth error"))

		err := service.Delete(ctx, workspaceID, automationID)
		assert.Error(t, err)
	})

	t.Run("delete repository error", func(t *testing.T) {
		userWorkspace := &domain.UserWorkspace{
			UserID:      "user-123",
			WorkspaceID: workspaceID,
			Role:        "admin",
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)
		mockRepo.EXPECT().Delete(ctx, workspaceID, automationID).Return(errors.New("delete failed"))
		mockLogger.EXPECT().WithField("automation_id", automationID).Return(mockLogger)
		mockLogger.EXPECT().Error(gomock.Any())

		err := service.Delete(ctx, workspaceID, automationID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to delete automation")
	})
}

func TestAutomationService_Activate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockAutomationRepository(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	service := NewAutomationService(mockRepo, mockAuthService, mockLogger)

	ctx := context.Background()
	workspaceID := "workspace-123"
	automationID := "auto-123"

	t.Run("successful activate", func(t *testing.T) {
		userWorkspace := &domain.UserWorkspace{
			UserID:      "user-123",
			WorkspaceID: workspaceID,
			Role:        "admin",
		}
		// Mock that automation is in draft status
		existingAutomation := createTestAutomationService(automationID, workspaceID)
		existingAutomation.Status = domain.AutomationStatusDraft

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID, automationID).Return(existingAutomation, nil)
		mockRepo.EXPECT().Update(ctx, workspaceID, gomock.Any()).Return(nil)
		mockRepo.EXPECT().CreateAutomationTrigger(ctx, workspaceID, gomock.Any()).Return(nil)

		err := service.Activate(ctx, workspaceID, automationID)
		assert.NoError(t, err)
	})

	t.Run("already live", func(t *testing.T) {
		userWorkspace := &domain.UserWorkspace{
			UserID:      "user-123",
			WorkspaceID: workspaceID,
			Role:        "admin",
		}
		existingAutomation := createTestAutomationService(automationID, workspaceID)
		existingAutomation.Status = domain.AutomationStatusLive

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID, automationID).Return(existingAutomation, nil)

		err := service.Activate(ctx, workspaceID, automationID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already live")
	})

	t.Run("email nodes with no list_id - rejected", func(t *testing.T) {
		userWorkspace := &domain.UserWorkspace{
			UserID:      "user-123",
			WorkspaceID: workspaceID,
			Role:        "admin",
		}
		existingAutomation := createTestAutomationService(automationID, workspaceID)
		existingAutomation.Status = domain.AutomationStatusDraft
		existingAutomation.ListID = "" // No list_id
		existingAutomation.Nodes = []*domain.AutomationNode{
			createTestAutomationNodeService("node-1", automationID, domain.NodeTypeEmail),
		}
		existingAutomation.RootNodeID = "node-1" // Must reference a valid node

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID, automationID).Return(existingAutomation, nil)

		err := service.Activate(ctx, workspaceID, automationID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot activate automation with email nodes when list_id is not set")
	})

	t.Run("email nodes with list_id - allowed", func(t *testing.T) {
		userWorkspace := &domain.UserWorkspace{
			UserID:      "user-123",
			WorkspaceID: workspaceID,
			Role:        "admin",
		}
		existingAutomation := createTestAutomationService(automationID, workspaceID)
		existingAutomation.Status = domain.AutomationStatusDraft
		existingAutomation.ListID = "list-123" // Has list_id

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID, automationID).Return(existingAutomation, nil)
		mockRepo.EXPECT().Update(ctx, workspaceID, gomock.Any()).Return(nil)
		mockRepo.EXPECT().CreateAutomationTrigger(ctx, workspaceID, gomock.Any()).Return(nil)

		err := service.Activate(ctx, workspaceID, automationID)
		assert.NoError(t, err)
	})

	t.Run("no email nodes with no list_id - allowed", func(t *testing.T) {
		userWorkspace := &domain.UserWorkspace{
			UserID:      "user-123",
			WorkspaceID: workspaceID,
			Role:        "admin",
		}
		existingAutomation := createTestAutomationService(automationID, workspaceID)
		existingAutomation.Status = domain.AutomationStatusDraft
		existingAutomation.ListID = "" // No list_id
		existingAutomation.Nodes = []*domain.AutomationNode{
			createTestAutomationNodeService("node-1", automationID, domain.NodeTypeDelay),
		}
		existingAutomation.RootNodeID = "node-1" // Must reference a valid node

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID, automationID).Return(existingAutomation, nil)
		mockRepo.EXPECT().Update(ctx, workspaceID, gomock.Any()).Return(nil)
		mockRepo.EXPECT().CreateAutomationTrigger(ctx, workspaceID, gomock.Any()).Return(nil)

		err := service.Activate(ctx, workspaceID, automationID)
		assert.NoError(t, err)
	})
}

func TestAutomationService_Pause(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockAutomationRepository(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	service := NewAutomationService(mockRepo, mockAuthService, mockLogger)

	ctx := context.Background()
	workspaceID := "workspace-123"
	automationID := "auto-123"

	t.Run("successful pause", func(t *testing.T) {
		userWorkspace := &domain.UserWorkspace{
			UserID:      "user-123",
			WorkspaceID: workspaceID,
			Role:        "admin",
		}
		// Mock that automation is in live status
		existingAutomation := createTestAutomationService(automationID, workspaceID)
		existingAutomation.Status = domain.AutomationStatusLive

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID, automationID).Return(existingAutomation, nil)
		mockRepo.EXPECT().DropAutomationTrigger(ctx, workspaceID, automationID).Return(nil)
		mockRepo.EXPECT().Update(ctx, workspaceID, gomock.Any()).Return(nil)

		err := service.Pause(ctx, workspaceID, automationID)
		assert.NoError(t, err)
	})

	t.Run("not live", func(t *testing.T) {
		userWorkspace := &domain.UserWorkspace{
			UserID:      "user-123",
			WorkspaceID: workspaceID,
			Role:        "admin",
		}
		existingAutomation := createTestAutomationService(automationID, workspaceID)
		existingAutomation.Status = domain.AutomationStatusDraft

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID, automationID).Return(existingAutomation, nil)

		err := service.Pause(ctx, workspaceID, automationID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not live")
	})
}

func TestAutomationService_GetContactNodeExecutions(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockAutomationRepository(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	service := NewAutomationService(mockRepo, mockAuthService, mockLogger)

	ctx := context.Background()
	workspaceID := "workspace-123"
	automationID := "auto-123"
	email := "test@example.com"

	t.Run("successful get contact node executions", func(t *testing.T) {
		contactAutomation := &domain.ContactAutomation{
			ID:           "ca-123",
			AutomationID: automationID,
			ContactEmail: email,
			Status:       domain.ContactAutomationStatusActive,
		}
		nodeExecutions := []*domain.NodeExecution{
			{
				ID:                  "entry-1",
				ContactAutomationID: "ca-123",
				NodeID:              "node-1",
				NodeType:            domain.NodeTypeTrigger,
				Action:              domain.NodeActionEntered,
			},
		}

		userWorkspace := &domain.UserWorkspace{
			UserID:      "user-123",
			WorkspaceID: workspaceID,
			Role:        "admin",
		}
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)
		mockRepo.EXPECT().GetContactAutomationByEmail(ctx, workspaceID, automationID, email).Return(contactAutomation, nil)
		mockRepo.EXPECT().GetNodeExecutions(ctx, workspaceID, "ca-123").Return(nodeExecutions, nil)

		ca, entries, err := service.GetContactNodeExecutions(ctx, workspaceID, automationID, email)
		assert.NoError(t, err)
		assert.NotNil(t, ca)
		assert.Equal(t, email, ca.ContactEmail)
		assert.Len(t, entries, 1)
	})

	t.Run("authentication failure", func(t *testing.T) {
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(ctx, nil, nil, errors.New("auth error"))

		ca, entries, err := service.GetContactNodeExecutions(ctx, workspaceID, automationID, email)
		assert.Error(t, err)
		assert.Nil(t, ca)
		assert.Nil(t, entries)
	})

	t.Run("contact automation not found", func(t *testing.T) {
		userWorkspace := &domain.UserWorkspace{
			UserID:      "user-123",
			WorkspaceID: workspaceID,
			Role:        "admin",
		}
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)
		mockRepo.EXPECT().GetContactAutomationByEmail(ctx, workspaceID, automationID, email).Return(nil, errors.New("not found"))

		ca, entries, err := service.GetContactNodeExecutions(ctx, workspaceID, automationID, email)
		assert.Error(t, err)
		assert.Nil(t, ca)
		assert.Nil(t, entries)
	})
}
