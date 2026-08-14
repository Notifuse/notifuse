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
			EventKind: "email.opened",
			Frequency: domain.TriggerFrequencyOnce,
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

// Helper function to build a valid trigger condition tree
func testTriggerConditionsService(field, value string) *domain.TreeNode {
	return &domain.TreeNode{
		Kind: "leaf",
		Leaf: &domain.TreeNodeLeaf{
			Source: "contacts",
			Contact: &domain.ContactCondition{
				Filters: []*domain.DimensionFilter{
					{
						FieldName:    field,
						FieldType:    "string",
						Operator:     "equals",
						StringValues: []string{value},
					},
				},
			},
		},
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
			Permissions: domain.FullPermissions,
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
			Permissions: domain.FullPermissions,
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
			Permissions: domain.FullPermissions,
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
			Permissions: domain.FullPermissions,
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
			Permissions: domain.FullPermissions,
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
			Permissions: domain.FullPermissions,
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
		stored := createTestAutomationService("auto-123", workspaceID)

		userWorkspace := &domain.UserWorkspace{
			UserID:      "user-123",
			WorkspaceID: workspaceID,
			Role:        "admin",
			Permissions: domain.FullPermissions,
		}
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)
		// No GetNodes call needed when list_id is set
		mockRepo.EXPECT().GetByID(ctx, workspaceID, automation.ID).Return(stored, nil)
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
			Permissions: domain.FullPermissions,
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
			Permissions: domain.FullPermissions,
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

		stored := createTestAutomationService("auto-123", workspaceID)
		stored.RootNodeID = "node-1"

		userWorkspace := &domain.UserWorkspace{
			UserID:      "user-123",
			WorkspaceID: workspaceID,
			Role:        "admin",
			Permissions: domain.FullPermissions,
		}
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID, automation.ID).Return(stored, nil)
		mockRepo.EXPECT().Update(ctx, workspaceID, automation).Return(nil)

		err := service.Update(ctx, workspaceID, automation)
		assert.NoError(t, err)
	})

	t.Run("live automation, trigger inputs unchanged - trigger not regenerated", func(t *testing.T) {
		// DROP/CREATE TRIGGER takes ACCESS EXCLUSIVE on contact_timeline, the table every
		// contact event passes through, so an edit that leaves the compiled trigger
		// identical must not touch it.
		automation := createTestAutomationService("auto-123", workspaceID)
		automation.Status = domain.AutomationStatusLive
		automation.Name = "Renamed while live"

		stored := createTestAutomationService("auto-123", workspaceID)
		stored.Status = domain.AutomationStatusLive

		userWorkspace := &domain.UserWorkspace{
			UserID:      "user-123",
			WorkspaceID: workspaceID,
			Role:        "admin",
			Permissions: domain.FullPermissions,
		}
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID, automation.ID).Return(stored, nil)
		mockRepo.EXPECT().CreateAutomationTrigger(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
		mockRepo.EXPECT().Update(ctx, workspaceID, automation).Return(nil)

		err := service.Update(ctx, workspaceID, automation)
		assert.NoError(t, err)
	})

	// The row is written before the trigger is installed. The reverse order would, on a
	// failed row write, leave a trigger compiled from a configuration the database does
	// not store — and nothing would repair it, because the next edit compares against
	// that stale stored row, finds no change, and skips regeneration.
	t.Run("live automation, event_kind changed - row written before the trigger", func(t *testing.T) {
		automation := createTestAutomationService("auto-123", workspaceID)
		automation.Status = domain.AutomationStatusLive
		automation.Trigger.EventKind = "email.clicked"

		stored := createTestAutomationService("auto-123", workspaceID)
		stored.Status = domain.AutomationStatusLive

		userWorkspace := &domain.UserWorkspace{
			UserID:      "user-123",
			WorkspaceID: workspaceID,
			Role:        "admin",
			Permissions: domain.FullPermissions,
		}
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID, automation.ID).Return(stored, nil)
		gomock.InOrder(
			mockRepo.EXPECT().Update(ctx, workspaceID, automation).Return(nil),
			mockRepo.EXPECT().CreateAutomationTrigger(ctx, workspaceID, automation).Return(nil),
		)

		err := service.Update(ctx, workspaceID, automation)
		assert.NoError(t, err)
	})

	// The compensating write is what keeps the stored configuration and the installed
	// trigger describing the same automation.
	t.Run("live automation, trigger install fails - the stored row is restored", func(t *testing.T) {
		automation := createTestAutomationService("auto-123", workspaceID)
		automation.Status = domain.AutomationStatusLive
		automation.Trigger.EventKind = "email.clicked"

		stored := createTestAutomationService("auto-123", workspaceID)
		stored.Status = domain.AutomationStatusLive

		userWorkspace := &domain.UserWorkspace{
			UserID:      "user-123",
			WorkspaceID: workspaceID,
			Role:        "admin",
			Permissions: domain.FullPermissions,
		}
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID, automation.ID).Return(stored, nil)
		gomock.InOrder(
			mockRepo.EXPECT().Update(ctx, workspaceID, automation).Return(nil),
			mockRepo.EXPECT().CreateAutomationTrigger(ctx, workspaceID, automation).
				Return(domain.NewTriggerConditionError("invalid trigger conditions: nope")),
			// Detached context: the restore must not be cancelled by the request that failed.
			mockRepo.EXPECT().Update(gomock.Not(gomock.Eq(ctx)), workspaceID, stored).Return(nil),
		)

		err := service.Update(ctx, workspaceID, automation)
		assert.Error(t, err)

		var conditionErr *domain.TriggerConditionError
		assert.True(t, errors.As(err, &conditionErr), "the reason must survive to the handler")
	})

	t.Run("live automation, conditions changed - trigger regenerated", func(t *testing.T) {
		automation := createTestAutomationService("auto-123", workspaceID)
		automation.Status = domain.AutomationStatusLive
		automation.Trigger.Conditions = testTriggerConditionsService("country", "US")

		stored := createTestAutomationService("auto-123", workspaceID)
		stored.Status = domain.AutomationStatusLive

		userWorkspace := &domain.UserWorkspace{
			UserID:      "user-123",
			WorkspaceID: workspaceID,
			Role:        "admin",
			Permissions: domain.FullPermissions,
		}
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID, automation.ID).Return(stored, nil)
		gomock.InOrder(
			mockRepo.EXPECT().Update(ctx, workspaceID, automation).Return(nil),
			mockRepo.EXPECT().CreateAutomationTrigger(ctx, workspaceID, automation).Return(nil),
		)

		err := service.Update(ctx, workspaceID, automation)
		assert.NoError(t, err)
	})

	t.Run("live automation, root_node_id changed - trigger regenerated", func(t *testing.T) {
		automation := createTestAutomationService("auto-123", workspaceID)
		automation.Status = domain.AutomationStatusLive
		automation.RootNodeID = "node-other"

		stored := createTestAutomationService("auto-123", workspaceID)
		stored.Status = domain.AutomationStatusLive

		userWorkspace := &domain.UserWorkspace{
			UserID:      "user-123",
			WorkspaceID: workspaceID,
			Role:        "admin",
			Permissions: domain.FullPermissions,
		}
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID, automation.ID).Return(stored, nil)
		gomock.InOrder(
			mockRepo.EXPECT().Update(ctx, workspaceID, automation).Return(nil),
			mockRepo.EXPECT().CreateAutomationTrigger(ctx, workspaceID, automation).Return(nil),
		)

		err := service.Update(ctx, workspaceID, automation)
		assert.NoError(t, err)
	})

	t.Run("draft automation, trigger inputs changed - no trigger to regenerate", func(t *testing.T) {
		automation := createTestAutomationService("auto-123", workspaceID)
		automation.Trigger.EventKind = "email.clicked"

		stored := createTestAutomationService("auto-123", workspaceID)

		userWorkspace := &domain.UserWorkspace{
			UserID:      "user-123",
			WorkspaceID: workspaceID,
			Role:        "admin",
			Permissions: domain.FullPermissions,
		}
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID, automation.ID).Return(stored, nil)
		mockRepo.EXPECT().CreateAutomationTrigger(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
		mockRepo.EXPECT().Update(ctx, workspaceID, automation).Return(nil)

		err := service.Update(ctx, workspaceID, automation)
		assert.NoError(t, err)
	})

	t.Run("an incoming status is ignored in favour of the stored one", func(t *testing.T) {
		// Update installs no trigger, so writing the incoming "live" status would leave a
		// row that claims to be live while nothing listens on contact_timeline — an
		// automation that never fires and that nothing later repairs.
		//
		// The write still happens, with the stored status: the whole object is overwritten
		// on update, so every read-modify-write client sends back the status it read, and
		// the console sends it on every save. Failing those saves over a field the caller
		// never meant to touch would be worse than ignoring it.
		automation := createTestAutomationService("auto-123", workspaceID)
		automation.Status = domain.AutomationStatusLive

		stored := createTestAutomationService("auto-123", workspaceID)
		stored.Status = domain.AutomationStatusDraft

		userWorkspace := &domain.UserWorkspace{
			UserID:      "user-123",
			WorkspaceID: workspaceID,
			Role:        "admin",
			Permissions: domain.FullPermissions,
		}
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID, automation.ID).Return(stored, nil)
		// No trigger work: the automation is not live, whatever the request claimed.
		mockRepo.EXPECT().CreateAutomationTrigger(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
		mockRepo.EXPECT().Update(ctx, workspaceID, gomock.Any()).DoAndReturn(
			func(_ context.Context, _ string, saved *domain.Automation) error {
				assert.Equal(t, domain.AutomationStatusDraft, saved.Status,
					"the stored status must survive an update that tried to change it")
				return nil
			})

		err := service.Update(ctx, workspaceID, automation)
		assert.NoError(t, err)
	})

	// The one case the compensating write cannot fix. It leaves the stored configuration
	// describing an automation the installed trigger does not implement, which nothing
	// detects on its own — so it has to be logged rather than swallowed.
	t.Run("a failed restore after a failed trigger install is reported", func(t *testing.T) {
		automation := createTestAutomationService("auto-123", workspaceID)
		automation.Status = domain.AutomationStatusLive
		automation.Trigger.Conditions = testTriggerConditionsService("country", "US")

		stored := createTestAutomationService("auto-123", workspaceID)
		stored.Status = domain.AutomationStatusLive

		userWorkspace := &domain.UserWorkspace{
			UserID:      "user-123",
			WorkspaceID: workspaceID,
			Role:        "admin",
			Permissions: domain.FullPermissions,
		}
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID, automation.ID).Return(stored, nil)
		gomock.InOrder(
			mockRepo.EXPECT().Update(ctx, workspaceID, automation).Return(nil),
			mockRepo.EXPECT().CreateAutomationTrigger(ctx, workspaceID, automation).
				Return(domain.NewTriggerConditionError("invalid trigger conditions: column does not exist")),
			mockRepo.EXPECT().Update(gomock.Not(gomock.Eq(ctx)), workspaceID, stored).Return(errors.New("db gone")),
		)
		// Both fields: with one database per workspace, the automation id alone does not
		// say which database holds the mismatched trigger.
		mockLogger.EXPECT().WithField("automation_id", automation.ID).Return(mockLogger)
		mockLogger.EXPECT().WithField("workspace_id", workspaceID).Return(mockLogger)
		mockLogger.EXPECT().Error(gomock.Any())

		err := service.Update(ctx, workspaceID, automation)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to update automation trigger")

		var conditionErr *domain.TriggerConditionError
		assert.True(t, errors.As(err, &conditionErr), "the condition error must survive wrapping so the handler can answer 400")
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
			Permissions: domain.FullPermissions,
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
			Permissions: domain.FullPermissions,
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
			Permissions: domain.FullPermissions,
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
			Permissions: domain.FullPermissions,
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
			Permissions: domain.FullPermissions,
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
			Permissions: domain.FullPermissions,
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
			Permissions: domain.FullPermissions,
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
			Permissions: domain.FullPermissions,
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

	t.Run("trigger creation failure rolls a paused automation back to paused", func(t *testing.T) {
		userWorkspace := &domain.UserWorkspace{
			UserID:      "user-123",
			WorkspaceID: workspaceID,
			Role:        "admin",
			Permissions: domain.FullPermissions,
		}
		existingAutomation := createTestAutomationService(automationID, workspaceID)
		existingAutomation.Status = domain.AutomationStatusPaused

		var writtenStatuses []domain.AutomationStatus
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID, automationID).Return(existingAutomation, nil)
		mockRepo.EXPECT().Update(gomock.Any(), workspaceID, gomock.Any()).Times(2).
			DoAndReturn(func(_ context.Context, _ string, a *domain.Automation) error {
				writtenStatuses = append(writtenStatuses, a.Status)
				return nil
			})
		mockRepo.EXPECT().CreateAutomationTrigger(ctx, workspaceID, gomock.Any()).Return(errors.New("cannot use subquery in trigger WHEN condition"))

		err := service.Activate(ctx, workspaceID, automationID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create automation trigger")
		assert.Equal(t, []domain.AutomationStatus{domain.AutomationStatusLive, domain.AutomationStatusPaused}, writtenStatuses)
		assert.Equal(t, domain.AutomationStatusPaused, existingAutomation.Status)
	})

	t.Run("trigger creation failure rolls a draft automation back to draft", func(t *testing.T) {
		userWorkspace := &domain.UserWorkspace{
			UserID:      "user-123",
			WorkspaceID: workspaceID,
			Role:        "admin",
			Permissions: domain.FullPermissions,
		}
		existingAutomation := createTestAutomationService(automationID, workspaceID)
		existingAutomation.Status = domain.AutomationStatusDraft

		var writtenStatuses []domain.AutomationStatus
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID, automationID).Return(existingAutomation, nil)
		mockRepo.EXPECT().Update(gomock.Any(), workspaceID, gomock.Any()).Times(2).
			DoAndReturn(func(_ context.Context, _ string, a *domain.Automation) error {
				writtenStatuses = append(writtenStatuses, a.Status)
				return nil
			})
		mockRepo.EXPECT().CreateAutomationTrigger(ctx, workspaceID, gomock.Any()).Return(errors.New("trigger install failed"))

		err := service.Activate(ctx, workspaceID, automationID)
		assert.Error(t, err)
		assert.Equal(t, []domain.AutomationStatus{domain.AutomationStatusLive, domain.AutomationStatusDraft}, writtenStatuses)
		assert.Equal(t, domain.AutomationStatusDraft, existingAutomation.Status)
	})

	t.Run("stored automation that fails validation is refused before any DDL", func(t *testing.T) {
		userWorkspace := &domain.UserWorkspace{
			UserID:      "user-123",
			WorkspaceID: workspaceID,
			Role:        "admin",
			Permissions: domain.FullPermissions,
		}
		existingAutomation := createTestAutomationService(automationID, workspaceID)
		existingAutomation.Status = domain.AutomationStatusDraft
		// A leaf node carrying no leaf payload: storable, but the generator would
		// dereference it while compiling the guard.
		existingAutomation.Trigger.Conditions = &domain.TreeNode{Kind: "leaf"}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID, automationID).Return(existingAutomation, nil)
		mockRepo.EXPECT().CreateAutomationTrigger(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
		mockRepo.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

		err := service.Activate(ctx, workspaceID, automationID)
		assert.Error(t, err)

		var conditionErr *domain.TriggerConditionError
		assert.True(t, errors.As(err, &conditionErr), "an unusable stored configuration is a 400, not a 500")
		assert.Contains(t, err.Error(), "invalid trigger conditions")
		assert.Equal(t, domain.AutomationStatusDraft, existingAutomation.Status)
	})
}

func TestAutomationService_TriggerInputsChanged(t *testing.T) {
	t.Run("identical automations", func(t *testing.T) {
		existing := createTestAutomationService("auto-123", "workspace-123")
		updated := createTestAutomationService("auto-123", "workspace-123")

		assert.False(t, triggerInputsChanged(existing, updated))
	})

	t.Run("root node changed", func(t *testing.T) {
		existing := createTestAutomationService("auto-123", "workspace-123")
		updated := createTestAutomationService("auto-123", "workspace-123")
		updated.RootNodeID = "node-other"

		assert.True(t, triggerInputsChanged(existing, updated))
	})

	t.Run("event kind changed", func(t *testing.T) {
		existing := createTestAutomationService("auto-123", "workspace-123")
		updated := createTestAutomationService("auto-123", "workspace-123")
		updated.Trigger.EventKind = "email.clicked"

		assert.True(t, triggerInputsChanged(existing, updated))
	})

	t.Run("conditions changed", func(t *testing.T) {
		existing := createTestAutomationService("auto-123", "workspace-123")
		existing.Trigger.Conditions = testTriggerConditionsService("country", "US")
		updated := createTestAutomationService("auto-123", "workspace-123")
		updated.Trigger.Conditions = testTriggerConditionsService("country", "FR")

		assert.True(t, triggerInputsChanged(existing, updated))
	})

	t.Run("conditions removed", func(t *testing.T) {
		existing := createTestAutomationService("auto-123", "workspace-123")
		existing.Trigger.Conditions = testTriggerConditionsService("country", "US")
		updated := createTestAutomationService("auto-123", "workspace-123")

		assert.True(t, triggerInputsChanged(existing, updated))
	})

	t.Run("field the generator does not read", func(t *testing.T) {
		// Renaming a live automation must not cost an ACCESS EXCLUSIVE lock on
		// contact_timeline.
		existing := createTestAutomationService("auto-123", "workspace-123")
		updated := createTestAutomationService("auto-123", "workspace-123")
		updated.Name = "Renamed"
		updated.Stats = &domain.AutomationStats{Enrolled: 42}

		assert.False(t, triggerInputsChanged(existing, updated))
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
			Permissions: domain.FullPermissions,
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
			Permissions: domain.FullPermissions,
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
			Permissions: domain.FullPermissions,
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
			Permissions: domain.FullPermissions,
		}
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(ctx, &domain.User{}, userWorkspace, nil)
		mockRepo.EXPECT().GetContactAutomationByEmail(ctx, workspaceID, automationID, email).Return(nil, errors.New("not found"))

		ca, entries, err := service.GetContactNodeExecutions(ctx, workspaceID, automationID, email)
		assert.Error(t, err)
		assert.Nil(t, ca)
		assert.Nil(t, entries)
	})
}
