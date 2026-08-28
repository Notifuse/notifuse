package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		mockRepo.EXPECT().UpdateIfStatus(ctx, workspaceID, automation, stored.Status).Return(true, nil)

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
		// The check now runs on the nodes that would actually be stored, which is known
		// only after the stored row has been read.
		mockRepo.EXPECT().GetByID(ctx, workspaceID, automation.ID).Return(createTestAutomationService("auto-123", workspaceID), nil)

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
		mockRepo.EXPECT().UpdateIfStatus(ctx, workspaceID, automation, stored.Status).Return(true, nil)

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
		mockRepo.EXPECT().UpdateIfStatus(ctx, workspaceID, automation, stored.Status).Return(true, nil)

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
			mockRepo.EXPECT().UpdateIfStatus(ctx, workspaceID, automation, stored.Status).Return(true, nil),
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
			mockRepo.EXPECT().UpdateIfStatus(ctx, workspaceID, automation, stored.Status).Return(true, nil),
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
			mockRepo.EXPECT().UpdateIfStatus(ctx, workspaceID, automation, stored.Status).Return(true, nil),
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
			mockRepo.EXPECT().UpdateIfStatus(ctx, workspaceID, automation, stored.Status).Return(true, nil),
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
		mockRepo.EXPECT().UpdateIfStatus(ctx, workspaceID, automation, stored.Status).Return(true, nil)

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
		mockRepo.EXPECT().UpdateIfStatus(ctx, workspaceID, gomock.Any(), domain.AutomationStatusDraft).DoAndReturn(
			func(_ context.Context, _ string, saved *domain.Automation, _ domain.AutomationStatus) (bool, error) {
				assert.Equal(t, domain.AutomationStatusDraft, saved.Status,
					"the stored status must survive an update that tried to change it")
				return true, nil
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
			mockRepo.EXPECT().UpdateIfStatus(ctx, workspaceID, automation, stored.Status).Return(true, nil),
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

// decodeUpdateAutomationRequest builds the request the handler builds, from a body, so the
// tests can express a key the caller never sent. A Go literal cannot: Nodes is a plain
// slice on a struct with no optional fields, so "not part of this edit" and "delete every
// node" are the same value once it is built by hand.
func decodeUpdateAutomationRequest(t *testing.T, body string) *domain.UpdateAutomationRequest {
	t.Helper()
	var req domain.UpdateAutomationRequest
	require.NoError(t, json.Unmarshal([]byte(body), &req))
	return &req
}

// TestAutomationService_Update_OmittedNodesKeepTheStoredWorkflow covers a body that says
// nothing about the workflow. automations.update rewrites the whole row, and Validate skips
// the root-node check when the set is empty, so such a body is accepted and stores an
// automation with no steps — while a live one keeps enrolling contacts into a journey that
// has nothing to run. Nodes live only in that row, so nothing can put them back.
func TestAutomationService_Update_OmittedNodesKeepTheStoredWorkflow(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockAutomationRepository(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	service := NewAutomationService(mockRepo, mockAuthService, mockLogger)

	ctx := context.Background()
	workspaceID := "workspace-123"

	// A rename, hand-written against the documented shape. Everything Validate insists on
	// is here — id, name, status, trigger — which is why it reaches the write at all.
	const renameBody = `{
		"workspace_id": "workspace-123",
		"automation": {
			"id": "auto-123",
			"workspace_id": "workspace-123",
			"name": "Renamed from a script",
			"status": "draft",
			"list_id": "list-123",
			"trigger": {"event_kind": "email.opened", "frequency": "once"}
		}
	}`

	storedWithWorkflow := func() *domain.Automation {
		stored := createTestAutomationService("auto-123", workspaceID)
		stored.Nodes = []*domain.AutomationNode{
			createTestAutomationNodeService("node-root", "auto-123", domain.NodeTypeEmail),
			createTestAutomationNodeService("node-2", "auto-123", domain.NodeTypeDelay),
		}
		stored.RootNodeID = "node-root"
		return stored
	}

	userWorkspace := func() *domain.UserWorkspace {
		return &domain.UserWorkspace{
			UserID:      "user-123",
			WorkspaceID: workspaceID,
			Role:        "admin",
			Permissions: domain.FullPermissions,
		}
	}

	t.Run("a body with no nodes key keeps the stored workflow", func(t *testing.T) {
		req := decodeUpdateAutomationRequest(t, renameBody)
		require.NoError(t, req.Validate(), "the body is accepted as it stands: the root-node check is skipped for an empty set")
		require.Nil(t, req.Automation.Nodes, "the request must carry no nodes, or this proves nothing")

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(ctx, &domain.User{}, userWorkspace(), nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID, "auto-123").Return(storedWithWorkflow(), nil)

		var persisted *domain.Automation
		mockRepo.EXPECT().
			UpdateIfStatus(ctx, workspaceID, gomock.Any(), domain.AutomationStatusDraft).
			DoAndReturn(func(_ context.Context, _ string, a *domain.Automation, _ domain.AutomationStatus) (bool, error) {
				persisted = a
				return true, nil
			})

		require.NoError(t, service.Update(ctx, workspaceID, req.Automation))

		// Against literals, not against the fixture: the preserved slice is the stored
		// one, so comparing it to the fixture's would compare it to itself.
		require.NotNil(t, persisted)
		require.Len(t, persisted.Nodes, 2)
		assert.Equal(t, "node-root", persisted.Nodes[0].ID)
		assert.Equal(t, "node-2", persisted.Nodes[1].ID)
		assert.Equal(t, "node-root", persisted.RootNodeID)
		assert.Equal(t, "Renamed from a script", persisted.Name, "the edit the caller did ask for must still land")
	})

	t.Run("an explicitly empty nodes array still clears the workflow", func(t *testing.T) {
		body := `{
			"workspace_id": "workspace-123",
			"automation": {
				"id": "auto-123",
				"workspace_id": "workspace-123",
				"name": "Emptied on purpose",
				"status": "draft",
				"list_id": "list-123",
				"root_node_id": "",
				"nodes": [],
				"trigger": {"event_kind": "email.opened", "frequency": "once"}
			}
		}`
		req := decodeUpdateAutomationRequest(t, body)
		require.NoError(t, req.Validate())
		require.NotNil(t, req.Automation.Nodes, "an empty array must decode to a non-nil slice, which is what separates it from an absent key")

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(ctx, &domain.User{}, userWorkspace(), nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID, "auto-123").Return(storedWithWorkflow(), nil)

		var persisted *domain.Automation
		mockRepo.EXPECT().
			UpdateIfStatus(ctx, workspaceID, gomock.Any(), domain.AutomationStatusDraft).
			DoAndReturn(func(_ context.Context, _ string, a *domain.Automation, _ domain.AutomationStatus) (bool, error) {
				persisted = a
				return true, nil
			})

		require.NoError(t, service.Update(ctx, workspaceID, req.Automation))

		require.NotNil(t, persisted)
		assert.Empty(t, persisted.Nodes, "deleting the whole workflow stays expressible")
	})

	// The list-less check ran against the request's own (empty) node set, so preserving the
	// stored nodes afterwards could slip email nodes into an automation with no list —
	// exactly what that check exists to prevent.
	t.Run("preserved email nodes are still checked against a removed list_id", func(t *testing.T) {
		body := `{
			"workspace_id": "workspace-123",
			"automation": {
				"id": "auto-123",
				"workspace_id": "workspace-123",
				"name": "List removed from a script",
				"status": "draft",
				"list_id": "",
				"trigger": {"event_kind": "email.opened", "frequency": "once"}
			}
		}`
		req := decodeUpdateAutomationRequest(t, body)
		require.NoError(t, req.Validate())

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(ctx, &domain.User{}, userWorkspace(), nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID, "auto-123").Return(storedWithWorkflow(), nil)
		mockRepo.EXPECT().UpdateIfStatus(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

		err := service.Update(ctx, workspaceID, req.Automation)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot remove list_id from automation with email nodes")
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
		mockRepo.EXPECT().UpdateIfStatus(ctx, workspaceID, gomock.Any(), domain.AutomationStatusDraft).Return(true, nil)
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
		mockRepo.EXPECT().UpdateIfStatus(ctx, workspaceID, gomock.Any(), domain.AutomationStatusDraft).Return(true, nil)
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
		mockRepo.EXPECT().UpdateIfStatus(ctx, workspaceID, gomock.Any(), domain.AutomationStatusDraft).Return(true, nil)
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
		mockRepo.EXPECT().UpdateIfStatus(gomock.Any(), workspaceID, gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, a *domain.Automation, _ domain.AutomationStatus) (bool, error) {
				writtenStatuses = append(writtenStatuses, a.Status)
				return true, nil
			})
		mockRepo.EXPECT().Update(gomock.Any(), workspaceID, gomock.Any()).
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
		mockRepo.EXPECT().UpdateIfStatus(gomock.Any(), workspaceID, gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, a *domain.Automation, _ domain.AutomationStatus) (bool, error) {
				writtenStatuses = append(writtenStatuses, a.Status)
				return true, nil
			})
		mockRepo.EXPECT().Update(gomock.Any(), workspaceID, gomock.Any()).
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
		mockRepo.EXPECT().UpdateIfStatus(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

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
		mockRepo.EXPECT().UpdateIfStatus(ctx, workspaceID, gomock.Any(), domain.AutomationStatusLive).Return(true, nil)
		// Detached: the drop must not be cancelled by the disconnect it exists to survive.
		mockRepo.EXPECT().DropAutomationTrigger(gomock.Not(gomock.Eq(ctx)), workspaceID, automationID).Return(nil)

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

// newAutomationTransitionMocks gives each subtest its own controller so an expectation set
// for one interleaving cannot be satisfied by another subtest's call.
func newAutomationTransitionMocks(t *testing.T) (*AutomationService, *mocks.MockAutomationRepository, *mocks.MockAuthService, *pkgmocks.MockLogger) {
	t.Helper()
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	repo := mocks.NewMockAutomationRepository(ctrl)
	auth := mocks.NewMockAuthService(ctrl)
	log := pkgmocks.NewMockLogger(ctrl)
	return NewAutomationService(repo, auth, log), repo, auth, log
}

func automationTransitionUserWorkspace(workspaceID string) *domain.UserWorkspace {
	return &domain.UserWorkspace{
		UserID:      "user-123",
		WorkspaceID: workspaceID,
		Role:        "admin",
		Permissions: domain.FullPermissions,
	}
}

// Pause has to fail toward "paused with a trigger still installed", never toward "live with
// no trigger". The first is inert — automation_enroll_contact refuses to enrol for a non-live
// automation — and a retry repairs it. The second shows a Live badge, enrols nobody, and
// nothing in the product ever detects or repairs it.
func TestAutomationService_Pause_OrderingAndCompensation(t *testing.T) {
	workspaceID := "workspace-123"
	automationID := "auto-123"

	t.Run("writes the paused status before dropping the trigger", func(t *testing.T) {
		svc, repo, auth, _ := newAutomationTransitionMocks(t)
		ctx := context.Background()

		stored := createTestAutomationService(automationID, workspaceID)
		stored.Status = domain.AutomationStatusLive

		auth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{}, automationTransitionUserWorkspace(workspaceID), nil)
		repo.EXPECT().GetByID(ctx, workspaceID, automationID).Return(stored, nil)

		statusWrite := repo.EXPECT().
			UpdateIfStatus(gomock.Any(), workspaceID, gomock.Any(), domain.AutomationStatusLive).
			DoAndReturn(func(_ context.Context, _ string, saved *domain.Automation, _ domain.AutomationStatus) (bool, error) {
				assert.Equal(t, domain.AutomationStatusPaused, saved.Status)
				return true, nil
			})
		dropTrigger := repo.EXPECT().DropAutomationTrigger(gomock.Any(), workspaceID, automationID).Return(nil)
		gomock.InOrder(statusWrite, dropTrigger)

		assert.NoError(t, svc.Pause(ctx, workspaceID, automationID))
	})

	// The drop is the only DDL path with no lock_timeout, so it can block on a busy
	// contact_timeline for longer than the browser waits. If it inherited the request
	// context, the disconnect would cancel the very drop the pause exists to perform.
	t.Run("drops the trigger on a context detached from the caller's", func(t *testing.T) {
		svc, repo, auth, _ := newAutomationTransitionMocks(t)
		reqCtx, cancel := context.WithCancel(context.Background())
		defer cancel()

		stored := createTestAutomationService(automationID, workspaceID)
		stored.Status = domain.AutomationStatusLive

		auth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(reqCtx, &domain.User{}, automationTransitionUserWorkspace(workspaceID), nil)
		repo.EXPECT().GetByID(reqCtx, workspaceID, automationID).Return(stored, nil)
		repo.EXPECT().UpdateIfStatus(reqCtx, workspaceID, gomock.Any(), domain.AutomationStatusLive).
			DoAndReturn(func(_ context.Context, _ string, _ *domain.Automation, _ domain.AutomationStatus) (bool, error) {
				cancel() // the admin's browser gives up between the two writes
				return true, nil
			})
		repo.EXPECT().DropAutomationTrigger(gomock.Not(gomock.Eq(reqCtx)), workspaceID, automationID).
			DoAndReturn(func(dropCtx context.Context, _, _ string) error {
				assert.NoError(t, dropCtx.Err(), "the drop must not inherit the cancelled request context")
				return nil
			})

		assert.NoError(t, svc.Pause(reqCtx, workspaceID, automationID))
	})

	// The repair path for the state this ordering can leave behind. Without it the orphan
	// trigger is undroppable: pause refuses a non-live automation, so the only way out
	// would be to activate it again first.
	t.Run("an already-paused automation has its trigger dropped without a second write", func(t *testing.T) {
		svc, repo, auth, _ := newAutomationTransitionMocks(t)
		ctx := context.Background()

		stored := createTestAutomationService(automationID, workspaceID)
		stored.Status = domain.AutomationStatusPaused

		auth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{}, automationTransitionUserWorkspace(workspaceID), nil)
		repo.EXPECT().GetByID(ctx, workspaceID, automationID).Return(stored, nil)
		repo.EXPECT().UpdateIfStatus(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
		repo.EXPECT().DropAutomationTrigger(gomock.Any(), workspaceID, automationID).Return(nil)

		assert.NoError(t, svc.Pause(ctx, workspaceID, automationID))
	})

	t.Run("a draft automation is still refused", func(t *testing.T) {
		svc, repo, auth, _ := newAutomationTransitionMocks(t)
		ctx := context.Background()

		stored := createTestAutomationService(automationID, workspaceID)
		stored.Status = domain.AutomationStatusDraft

		auth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{}, automationTransitionUserWorkspace(workspaceID), nil)
		repo.EXPECT().GetByID(ctx, workspaceID, automationID).Return(stored, nil)
		repo.EXPECT().DropAutomationTrigger(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

		err := svc.Pause(ctx, workspaceID, automationID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not live")
	})

	// The row stays paused: the scheduler and the executor both stop at it, and the leftover
	// trigger enrols nobody. Rolling the status back to live to "match" the trigger would
	// resume an automation the admin asked to stop.
	t.Run("a failed drop is reported and leaves the automation paused", func(t *testing.T) {
		svc, repo, auth, log := newAutomationTransitionMocks(t)
		ctx := context.Background()

		stored := createTestAutomationService(automationID, workspaceID)
		stored.Status = domain.AutomationStatusLive

		auth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{}, automationTransitionUserWorkspace(workspaceID), nil)
		repo.EXPECT().GetByID(ctx, workspaceID, automationID).Return(stored, nil)
		repo.EXPECT().UpdateIfStatus(gomock.Any(), workspaceID, gomock.Any(), domain.AutomationStatusLive).
			Return(true, nil)
		repo.EXPECT().DropAutomationTrigger(gomock.Any(), workspaceID, automationID).
			Return(errors.New("canceling statement due to lock timeout"))
		// Both fields: one database per workspace, so the automation id alone does not say
		// which database holds the orphan trigger.
		log.EXPECT().WithField("automation_id", automationID).Return(log)
		log.EXPECT().WithField("workspace_id", workspaceID).Return(log)
		log.EXPECT().Error(gomock.Any())

		err := svc.Pause(ctx, workspaceID, automationID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to drop automation trigger")
	})
}

// Every transition writes through a status predicate, so a row another admin has already
// moved is never overwritten from a stale read. Losing the race is a 409 the caller can
// retry after reloading — not a silent revert, and never DDL emitted from a stale decision.
func TestAutomationService_TransitionsRejectStaleStatus(t *testing.T) {
	workspaceID := "workspace-123"
	automationID := "auto-123"

	assertConflict := func(t *testing.T, err error) {
		t.Helper()
		assert.Error(t, err)
		var conflictErr *domain.AutomationConflictError
		assert.True(t, errors.As(err, &conflictErr), "the conflict must survive wrapping so the handler can answer 409")
		assert.Equal(t, automationID, conflictErr.AutomationID)
	}

	t.Run("update does not resurrect the status it read", func(t *testing.T) {
		svc, repo, auth, _ := newAutomationTransitionMocks(t)
		ctx := context.Background()

		automation := createTestAutomationService(automationID, workspaceID)
		automation.Trigger.Conditions = testTriggerConditionsService("country", "US")
		stored := createTestAutomationService(automationID, workspaceID)
		stored.Status = domain.AutomationStatusLive

		auth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{}, automationTransitionUserWorkspace(workspaceID), nil)
		repo.EXPECT().GetByID(ctx, workspaceID, automationID).Return(stored, nil)
		repo.EXPECT().UpdateIfStatus(ctx, workspaceID, automation, domain.AutomationStatusLive).
			Return(false, nil)
		// The decision to regenerate was taken from the row this write just failed to match.
		repo.EXPECT().CreateAutomationTrigger(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

		assertConflict(t, svc.Update(ctx, workspaceID, automation))
	})

	t.Run("activate installs no trigger when the row moved under it", func(t *testing.T) {
		svc, repo, auth, _ := newAutomationTransitionMocks(t)
		ctx := context.Background()

		stored := createTestAutomationService(automationID, workspaceID)
		stored.Status = domain.AutomationStatusDraft

		auth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{}, automationTransitionUserWorkspace(workspaceID), nil)
		repo.EXPECT().GetByID(ctx, workspaceID, automationID).Return(stored, nil)
		repo.EXPECT().UpdateIfStatus(ctx, workspaceID, stored, domain.AutomationStatusDraft).
			Return(false, nil)
		repo.EXPECT().CreateAutomationTrigger(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

		assertConflict(t, svc.Activate(ctx, workspaceID, automationID))
	})

	t.Run("pause drops no trigger when the row moved under it", func(t *testing.T) {
		svc, repo, auth, _ := newAutomationTransitionMocks(t)
		ctx := context.Background()

		stored := createTestAutomationService(automationID, workspaceID)
		stored.Status = domain.AutomationStatusLive

		auth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{}, automationTransitionUserWorkspace(workspaceID), nil)
		repo.EXPECT().GetByID(ctx, workspaceID, automationID).Return(stored, nil)
		repo.EXPECT().UpdateIfStatus(gomock.Any(), workspaceID, gomock.Any(), domain.AutomationStatusLive).
			Return(false, nil)
		// Dropping here would disarm an automation another admin has just re-armed.
		repo.EXPECT().DropAutomationTrigger(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

		assertConflict(t, svc.Pause(ctx, workspaceID, automationID))
	})
}

// TestAutomationService_Update_OmittedExitOnReplyKeepsTheStoredSetting covers a body that
// says nothing about exit_on_reply. The field is a plain bool and automations.update
// rewrites the whole row, so an absent key decodes as false and switches the setting off —
// on a live automation that silently ends the only thing that stops a journey from mailing
// a contact who has already answered.
func TestAutomationService_Update_OmittedExitOnReplyKeepsTheStoredSetting(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockAutomationRepository(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	service := NewAutomationService(mockRepo, mockAuthService, mockLogger)

	ctx := context.Background()
	workspaceID := "workspace-123"

	storedWithExitOnReply := func(exitOnReply bool) *domain.Automation {
		stored := createTestAutomationService("auto-123", workspaceID)
		stored.ExitOnReply = exitOnReply
		return stored
	}

	userWorkspace := func() *domain.UserWorkspace {
		return &domain.UserWorkspace{
			UserID:      "user-123",
			WorkspaceID: workspaceID,
			Role:        "admin",
			Permissions: domain.FullPermissions,
		}
	}

	// One body per case, so the only thing that differs between them is the exit_on_reply
	// key itself.
	body := func(exitOnReply string) string {
		return `{
			"workspace_id": "workspace-123",
			"automation": {
				"id": "auto-123",
				"workspace_id": "workspace-123",
				"name": "Renamed from a script",
				"status": "draft",
				"list_id": "list-123",
				"trigger": {"event_kind": "email.opened", "frequency": "once"}` + exitOnReply + `
			}
		}`
	}

	updateFrom := func(t *testing.T, requestBody string, stored *domain.Automation) *domain.Automation {
		t.Helper()
		req := decodeUpdateAutomationRequest(t, requestBody)
		require.NoError(t, req.Validate())

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(ctx, &domain.User{}, userWorkspace(), nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID, "auto-123").Return(stored, nil)

		var persisted *domain.Automation
		mockRepo.EXPECT().
			UpdateIfStatus(ctx, workspaceID, gomock.Any(), domain.AutomationStatusDraft).
			DoAndReturn(func(_ context.Context, _ string, a *domain.Automation, _ domain.AutomationStatus) (bool, error) {
				persisted = a
				return true, nil
			})

		require.NoError(t, service.Update(ctx, workspaceID, req.Automation))
		require.NotNil(t, persisted)
		return persisted
	}

	t.Run("a body with no exit_on_reply key keeps the stored setting", func(t *testing.T) {
		persisted := updateFrom(t, body(""), storedWithExitOnReply(true))

		assert.True(t, persisted.ExitOnReply, "a rename must not switch off reply detection")
		assert.Equal(t, "Renamed from a script", persisted.Name, "the edit the caller did ask for must still land")
	})

	t.Run("a null exit_on_reply is read as absent", func(t *testing.T) {
		persisted := updateFrom(t, body(`, "exit_on_reply": null`), storedWithExitOnReply(true))

		assert.True(t, persisted.ExitOnReply, "there is no bool a null could have meant")
	})

	t.Run("an explicit false switches it off", func(t *testing.T) {
		persisted := updateFrom(t, body(`, "exit_on_reply": false`), storedWithExitOnReply(true))

		assert.False(t, persisted.ExitOnReply, "turning reply detection off must stay expressible")
	})

	t.Run("an explicit true switches it on", func(t *testing.T) {
		persisted := updateFrom(t, body(`, "exit_on_reply": true`), storedWithExitOnReply(false))

		assert.True(t, persisted.ExitOnReply)
	})

	// An automation assembled in Go has no body to have left a key out of, so its fields
	// are the whole of what it means — otherwise every non-HTTP caller would find its
	// exit_on_reply quietly replaced by the stored one.
	t.Run("an automation built in Go applies its own value", func(t *testing.T) {
		automation := createTestAutomationService("auto-123", workspaceID)
		automation.ExitOnReply = false

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(ctx, &domain.User{}, userWorkspace(), nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID, "auto-123").Return(storedWithExitOnReply(true), nil)

		var persisted *domain.Automation
		mockRepo.EXPECT().
			UpdateIfStatus(ctx, workspaceID, gomock.Any(), domain.AutomationStatusDraft).
			DoAndReturn(func(_ context.Context, _ string, a *domain.Automation, _ domain.AutomationStatus) (bool, error) {
				persisted = a
				return true, nil
			})

		require.NoError(t, service.Update(ctx, workspaceID, automation))

		require.NotNil(t, persisted)
		assert.False(t, persisted.ExitOnReply)
	})
}

// TestAutomationService_Update_OmittedListIDKeepsTheStoredList covers a body that says
// nothing about list_id. The field is a plain string on an update that rewrites the whole
// row, so an absent key decodes as "" — which is how the automation says it has no list.
// That decides who gets enrolled, and it is also what the email-node restriction is read
// from, so the same omission either silently retargets the automation or gets refused for
// a removal the caller never asked for.
func TestAutomationService_Update_OmittedListIDKeepsTheStoredList(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockAutomationRepository(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	service := NewAutomationService(mockRepo, mockAuthService, mockLogger)

	ctx := context.Background()
	workspaceID := "workspace-123"

	// Two stored shapes, because the omission plays out differently either side of the
	// email-node restriction: with email nodes it is refused, without them it goes
	// through and blanks the list.
	storedWithNodes := func(nodeType domain.NodeType) *domain.Automation {
		stored := createTestAutomationService("auto-123", workspaceID)
		stored.Nodes = []*domain.AutomationNode{
			createTestAutomationNodeService("node-root", "auto-123", nodeType),
		}
		stored.RootNodeID = "node-root"
		return stored
	}

	userWorkspace := func() *domain.UserWorkspace {
		return &domain.UserWorkspace{
			UserID:      "user-123",
			WorkspaceID: workspaceID,
			Role:        "admin",
			Permissions: domain.FullPermissions,
		}
	}

	// One body per case, so the only thing that differs between them is the list_id key
	// itself.
	body := func(listID string) string {
		return `{
			"workspace_id": "workspace-123",
			"automation": {
				"id": "auto-123",
				"workspace_id": "workspace-123",
				"name": "Renamed from a script",
				"status": "draft",
				"trigger": {"event_kind": "email.opened", "frequency": "once"}` + listID + `
			}
		}`
	}

	updateFrom := func(t *testing.T, requestBody string, stored *domain.Automation) *domain.Automation {
		t.Helper()
		req := decodeUpdateAutomationRequest(t, requestBody)
		require.NoError(t, req.Validate())

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(ctx, &domain.User{}, userWorkspace(), nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID, "auto-123").Return(stored, nil)

		var persisted *domain.Automation
		mockRepo.EXPECT().
			UpdateIfStatus(ctx, workspaceID, gomock.Any(), domain.AutomationStatusDraft).
			DoAndReturn(func(_ context.Context, _ string, a *domain.Automation, _ domain.AutomationStatus) (bool, error) {
				persisted = a
				return true, nil
			})

		require.NoError(t, service.Update(ctx, workspaceID, req.Automation))
		require.NotNil(t, persisted)
		return persisted
	}

	// Against a literal, not against the fixture: the service writes back the stored
	// list onto the request, and the mock hands it the very automation the fixture
	// returned, so an expectation read off that fixture would compare the value to
	// itself.
	t.Run("a body with no list_id key keeps the stored list", func(t *testing.T) {
		persisted := updateFrom(t, body(""), storedWithNodes(domain.NodeTypeDelay))

		assert.Equal(t, "list-123", persisted.ListID, "a rename must not change who the automation enrols")
		assert.Equal(t, "Renamed from a script", persisted.Name, "the edit the caller did ask for must still land")
	})

	// The same omission on an automation that mails: the preserved nodes make the
	// restriction fire, so the request is rejected over a removal nobody asked for.
	t.Run("a body with no list_id key is not read as removing the list from a mailing automation", func(t *testing.T) {
		persisted := updateFrom(t, body(""), storedWithNodes(domain.NodeTypeEmail))

		assert.Equal(t, "list-123", persisted.ListID)
		assert.Equal(t, "Renamed from a script", persisted.Name)
	})

	t.Run("a null list_id is read as absent", func(t *testing.T) {
		persisted := updateFrom(t, body(`, "list_id": null`), storedWithNodes(domain.NodeTypeEmail))

		assert.Equal(t, "list-123", persisted.ListID, "a null is a serializer writing out an absent optional")
	})

	t.Run("an explicit list_id replaces the stored one", func(t *testing.T) {
		persisted := updateFrom(t, body(`, "list_id": "list-456"`), storedWithNodes(domain.NodeTypeEmail))

		assert.Equal(t, "list-456", persisted.ListID)
	})

	t.Run("an explicitly empty list_id still removes the list", func(t *testing.T) {
		persisted := updateFrom(t, body(`, "list_id": ""`), storedWithNodes(domain.NodeTypeDelay))

		assert.Empty(t, persisted.ListID, "detaching an automation from its list must stay expressible")
	})

	// An automation assembled in Go has no body to have left a key out of, so its fields
	// are the whole of what it means — otherwise every non-HTTP caller would find its
	// list_id quietly replaced by the stored one.
	t.Run("an automation built in Go applies its own value", func(t *testing.T) {
		automation := createTestAutomationService("auto-123", workspaceID)
		automation.ListID = ""

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(ctx, &domain.User{}, userWorkspace(), nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID, "auto-123").Return(storedWithNodes(domain.NodeTypeDelay), nil)

		var persisted *domain.Automation
		mockRepo.EXPECT().
			UpdateIfStatus(ctx, workspaceID, gomock.Any(), domain.AutomationStatusDraft).
			DoAndReturn(func(_ context.Context, _ string, a *domain.Automation, _ domain.AutomationStatus) (bool, error) {
				persisted = a
				return true, nil
			})

		require.NoError(t, service.Update(ctx, workspaceID, automation))

		require.NotNil(t, persisted)
		assert.Empty(t, persisted.ListID)
	})
}
