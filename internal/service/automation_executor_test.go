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
	"github.com/stretchr/testify/require"
)

func setupMockLogger(ctrl *gomock.Controller) *pkgmocks.MockLogger {
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	// Set up chainable WithField and WithFields calls
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	return mockLogger
}

func TestAutomationExecutor_Execute_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAutomationRepo := mocks.NewMockAutomationRepository(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := setupMockLogger(ctrl)

	// Create executor with minimal dependencies (no email service for delay test)
	executor := &AutomationExecutor{
		automationRepo: mockAutomationRepo,
		contactRepo:    mockContactRepo,
		workspaceRepo:  mockWorkspaceRepo,
		nodeExecutors: map[domain.NodeType]NodeExecutor{
			domain.NodeTypeDelay: NewDelayNodeExecutor(),
			domain.NodeTypeExit:  NewExitNodeExecutor(),
		},
		logger: mockLogger,
	}

	workspaceID := "ws1"
	automationID := "auto1"
	nodeID := "node1"
	nextNodeID := "node2"

	contactAutomation := &domain.ContactAutomation{
		ID:            "ca1",
		AutomationID:  automationID,
		ContactEmail:  "test@example.com",
		CurrentNodeID: &nodeID,
		Status:        domain.ContactAutomationStatusActive,
		MaxRetries:    3,
	}

	delayNode := &domain.AutomationNode{
		ID:         nodeID,
		Type:       domain.NodeTypeDelay,
		NextNodeID: &nextNodeID,
		Config: map[string]interface{}{
			"duration": 30,
			"unit":     "minutes",
		},
	}

	automation := &domain.Automation{
		ID:     automationID,
		Name:   "Test Automation",
		Status: domain.AutomationStatusLive,
		Nodes:  []*domain.AutomationNode{delayNode},
	}

	contact := &domain.Contact{
		Email: "test@example.com",
	}

	// Set expectations
	mockAutomationRepo.EXPECT().GetByID(gomock.Any(), workspaceID, automationID).Return(automation, nil)
	mockContactRepo.EXPECT().GetContactByEmail(gomock.Any(), workspaceID, "test@example.com").Return(contact, nil)
	mockAutomationRepo.EXPECT().CreateNodeExecution(gomock.Any(), workspaceID, gomock.Any()).Return(nil)
	mockAutomationRepo.EXPECT().GetNodeExecutions(gomock.Any(), workspaceID, "ca1").Return([]*domain.NodeExecution{}, nil)
	mockAutomationRepo.EXPECT().UpdateContactAutomation(gomock.Any(), workspaceID, gomock.Any()).Return(nil)
	mockAutomationRepo.EXPECT().UpdateNodeExecution(gomock.Any(), workspaceID, gomock.Any()).Return(nil)

	// Execute
	err := executor.Execute(context.Background(), workspaceID, contactAutomation)
	require.NoError(t, err)

	// Verify contact automation was updated
	assert.Equal(t, &nextNodeID, contactAutomation.CurrentNodeID)
	assert.NotNil(t, contactAutomation.ScheduledAt)
	assert.Equal(t, domain.ContactAutomationStatusActive, contactAutomation.Status)
}

func TestAutomationExecutor_Execute_AutomationPaused(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAutomationRepo := mocks.NewMockAutomationRepository(ctrl)
	mockLogger := setupMockLogger(ctrl)

	executor := &AutomationExecutor{
		automationRepo: mockAutomationRepo,
		nodeExecutors:  map[domain.NodeType]NodeExecutor{},
		logger:         mockLogger,
	}

	workspaceID := "ws1"
	nodeID := "node1"

	contactAutomation := &domain.ContactAutomation{
		ID:            "ca1",
		AutomationID:  "auto1",
		ContactEmail:  "test@example.com",
		CurrentNodeID: &nodeID,
		Status:        domain.ContactAutomationStatusActive,
	}

	automation := &domain.Automation{
		ID:     "auto1",
		Name:   "Test Automation",
		Status: domain.AutomationStatusPaused,
	}

	mockAutomationRepo.EXPECT().GetByID(gomock.Any(), workspaceID, "auto1").Return(automation, nil)
	// No UpdateContactAutomation or IncrementAutomationStat - contact freezes in place

	err := executor.Execute(context.Background(), workspaceID, contactAutomation)
	require.NoError(t, err)

	// Contact stays active and frozen at current node
	assert.Equal(t, domain.ContactAutomationStatusActive, contactAutomation.Status)
	assert.Equal(t, &nodeID, contactAutomation.CurrentNodeID)
}

func TestAutomationExecutor_Execute_NoCurrentNode(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAutomationRepo := mocks.NewMockAutomationRepository(ctrl)
	mockLogger := setupMockLogger(ctrl)

	executor := &AutomationExecutor{
		automationRepo: mockAutomationRepo,
		nodeExecutors:  map[domain.NodeType]NodeExecutor{},
		logger:         mockLogger,
	}

	workspaceID := "ws1"

	contactAutomation := &domain.ContactAutomation{
		ID:            "ca1",
		AutomationID:  "auto1",
		ContactEmail:  "test@example.com",
		CurrentNodeID: nil, // No current node
		Status:        domain.ContactAutomationStatusActive,
	}

	automation := &domain.Automation{
		ID:     "auto1",
		Name:   "Test Automation",
		Status: domain.AutomationStatusLive,
	}

	mockAutomationRepo.EXPECT().GetByID(gomock.Any(), workspaceID, "auto1").Return(automation, nil)
	mockAutomationRepo.EXPECT().UpdateContactAutomation(gomock.Any(), workspaceID, gomock.Any()).Return(nil)
	mockAutomationRepo.EXPECT().IncrementAutomationStat(gomock.Any(), workspaceID, "auto1", "completed").Return(nil)

	err := executor.Execute(context.Background(), workspaceID, contactAutomation)
	require.NoError(t, err)

	assert.Equal(t, domain.ContactAutomationStatusCompleted, contactAutomation.Status)
}

func TestAutomationExecutor_Execute_AutomationNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAutomationRepo := mocks.NewMockAutomationRepository(ctrl)
	mockLogger := setupMockLogger(ctrl)

	executor := &AutomationExecutor{
		automationRepo: mockAutomationRepo,
		nodeExecutors:  map[domain.NodeType]NodeExecutor{},
		logger:         mockLogger,
	}

	workspaceID := "ws1"
	nodeID := "node1"

	contactAutomation := &domain.ContactAutomation{
		ID:            "ca1",
		AutomationID:  "auto1",
		ContactEmail:  "test@example.com",
		CurrentNodeID: &nodeID,
		Status:        domain.ContactAutomationStatusActive,
		MaxRetries:    3,
	}

	mockAutomationRepo.EXPECT().GetByID(gomock.Any(), workspaceID, "auto1").Return(nil, errors.New("not found"))
	mockAutomationRepo.EXPECT().CreateNodeExecution(gomock.Any(), workspaceID, gomock.Any()).Return(nil)
	mockAutomationRepo.EXPECT().UpdateContactAutomation(gomock.Any(), workspaceID, gomock.Any()).Return(nil)

	err := executor.Execute(context.Background(), workspaceID, contactAutomation)
	require.NoError(t, err) // Error is handled internally

	assert.Equal(t, 1, contactAutomation.RetryCount)
	assert.NotNil(t, contactAutomation.LastError)
}

func TestAutomationExecutor_Execute_NodeNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAutomationRepo := mocks.NewMockAutomationRepository(ctrl)
	mockLogger := setupMockLogger(ctrl)

	executor := &AutomationExecutor{
		automationRepo: mockAutomationRepo,
		nodeExecutors:  map[domain.NodeType]NodeExecutor{},
		logger:         mockLogger,
	}

	workspaceID := "ws1"
	nodeID := "node1"

	contactAutomation := &domain.ContactAutomation{
		ID:            "ca1",
		AutomationID:  "auto1",
		ContactEmail:  "test@example.com",
		CurrentNodeID: &nodeID,
		Status:        domain.ContactAutomationStatusActive,
		MaxRetries:    3,
	}

	// Automation has no nodes - simulates node being deleted
	automation := &domain.Automation{
		ID:     "auto1",
		Name:   "Test Automation",
		Status: domain.AutomationStatusLive,
		Nodes:  []*domain.AutomationNode{}, // Empty nodes - current node not found
	}

	mockAutomationRepo.EXPECT().GetByID(gomock.Any(), workspaceID, "auto1").Return(automation, nil)
	mockAutomationRepo.EXPECT().UpdateContactAutomation(gomock.Any(), workspaceID, gomock.Any()).Return(nil)
	mockAutomationRepo.EXPECT().IncrementAutomationStat(gomock.Any(), workspaceID, "auto1", "exited").Return(nil)

	err := executor.Execute(context.Background(), workspaceID, contactAutomation)
	require.NoError(t, err)

	// Contact should be exited with automation_node_deleted reason
	assert.Equal(t, domain.ContactAutomationStatusExited, contactAutomation.Status)
	assert.NotNil(t, contactAutomation.ExitReason)
	assert.Equal(t, "automation_node_deleted", *contactAutomation.ExitReason)
}

func TestAutomationExecutor_Execute_UnsupportedNodeType(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAutomationRepo := mocks.NewMockAutomationRepository(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockLogger := setupMockLogger(ctrl)

	executor := &AutomationExecutor{
		automationRepo: mockAutomationRepo,
		contactRepo:    mockContactRepo,
		nodeExecutors:  map[domain.NodeType]NodeExecutor{}, // Empty - no executors
		logger:         mockLogger,
	}

	workspaceID := "ws1"
	nodeID := "node1"

	contactAutomation := &domain.ContactAutomation{
		ID:            "ca1",
		AutomationID:  "auto1",
		ContactEmail:  "test@example.com",
		CurrentNodeID: &nodeID,
		Status:        domain.ContactAutomationStatusActive,
		MaxRetries:    3,
	}

	node := &domain.AutomationNode{
		ID:   nodeID,
		Type: domain.NodeTypeDelay, // No executor for this
	}

	automation := &domain.Automation{
		ID:     "auto1",
		Name:   "Test Automation",
		Status: domain.AutomationStatusLive,
		Nodes:  []*domain.AutomationNode{node},
	}

	mockAutomationRepo.EXPECT().GetByID(gomock.Any(), workspaceID, "auto1").Return(automation, nil)
	mockAutomationRepo.EXPECT().CreateNodeExecution(gomock.Any(), workspaceID, gomock.Any()).Return(nil)
	mockAutomationRepo.EXPECT().UpdateContactAutomation(gomock.Any(), workspaceID, gomock.Any()).Return(nil)

	err := executor.Execute(context.Background(), workspaceID, contactAutomation)
	require.NoError(t, err)

	assert.Equal(t, 1, contactAutomation.RetryCount)
	assert.Contains(t, *contactAutomation.LastError, "unsupported node type")
}

func TestAutomationExecutor_Execute_ExitNode(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAutomationRepo := mocks.NewMockAutomationRepository(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockLogger := setupMockLogger(ctrl)

	executor := &AutomationExecutor{
		automationRepo: mockAutomationRepo,
		contactRepo:    mockContactRepo,
		nodeExecutors: map[domain.NodeType]NodeExecutor{
			domain.NodeTypeExit: NewExitNodeExecutor(),
		},
		logger: mockLogger,
	}

	workspaceID := "ws1"
	nodeID := "exit_node"

	contactAutomation := &domain.ContactAutomation{
		ID:            "ca1",
		AutomationID:  "auto1",
		ContactEmail:  "test@example.com",
		CurrentNodeID: &nodeID,
		Status:        domain.ContactAutomationStatusActive,
	}

	exitNode := &domain.AutomationNode{
		ID:   nodeID,
		Type: domain.NodeTypeExit,
	}

	automation := &domain.Automation{
		ID:     "auto1",
		Name:   "Test Automation",
		Status: domain.AutomationStatusLive,
		Nodes:  []*domain.AutomationNode{exitNode},
	}

	contact := &domain.Contact{
		Email: "test@example.com",
	}

	mockAutomationRepo.EXPECT().GetByID(gomock.Any(), workspaceID, "auto1").Return(automation, nil)
	mockContactRepo.EXPECT().GetContactByEmail(gomock.Any(), workspaceID, "test@example.com").Return(contact, nil)
	mockAutomationRepo.EXPECT().CreateNodeExecution(gomock.Any(), workspaceID, gomock.Any()).Return(nil)
	mockAutomationRepo.EXPECT().GetNodeExecutions(gomock.Any(), workspaceID, "ca1").Return([]*domain.NodeExecution{}, nil)
	mockAutomationRepo.EXPECT().UpdateContactAutomation(gomock.Any(), workspaceID, gomock.Any()).Return(nil)
	mockAutomationRepo.EXPECT().UpdateNodeExecution(gomock.Any(), workspaceID, gomock.Any()).Return(nil)
	mockAutomationRepo.EXPECT().IncrementAutomationStat(gomock.Any(), workspaceID, "auto1", "completed").Return(nil)

	err := executor.Execute(context.Background(), workspaceID, contactAutomation)
	require.NoError(t, err)

	assert.Nil(t, contactAutomation.CurrentNodeID)
	assert.Equal(t, domain.ContactAutomationStatusCompleted, contactAutomation.Status)
}

func TestAutomationExecutor_Execute_MaxRetriesExceeded(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAutomationRepo := mocks.NewMockAutomationRepository(ctrl)
	mockLogger := setupMockLogger(ctrl)

	executor := &AutomationExecutor{
		automationRepo: mockAutomationRepo,
		nodeExecutors:  map[domain.NodeType]NodeExecutor{},
		logger:         mockLogger,
	}

	workspaceID := "ws1"
	nodeID := "node1"

	contactAutomation := &domain.ContactAutomation{
		ID:            "ca1",
		AutomationID:  "auto1",
		ContactEmail:  "test@example.com",
		CurrentNodeID: &nodeID,
		Status:        domain.ContactAutomationStatusActive,
		RetryCount:    2,
		MaxRetries:    3,
	}

	mockAutomationRepo.EXPECT().GetByID(gomock.Any(), workspaceID, "auto1").Return(nil, errors.New("not found"))
	mockAutomationRepo.EXPECT().IncrementAutomationStat(gomock.Any(), workspaceID, "auto1", "failed").Return(nil)
	mockAutomationRepo.EXPECT().CreateNodeExecution(gomock.Any(), workspaceID, gomock.Any()).Return(nil)
	mockAutomationRepo.EXPECT().UpdateContactAutomation(gomock.Any(), workspaceID, gomock.Any()).Return(nil)

	err := executor.Execute(context.Background(), workspaceID, contactAutomation)
	require.NoError(t, err)

	assert.Equal(t, domain.ContactAutomationStatusFailed, contactAutomation.Status)
	assert.Equal(t, 3, contactAutomation.RetryCount)
}

func TestAutomationExecutor_ProcessBatch(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAutomationRepo := mocks.NewMockAutomationRepository(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockLogger := setupMockLogger(ctrl)

	executor := &AutomationExecutor{
		automationRepo: mockAutomationRepo,
		contactRepo:    mockContactRepo,
		nodeExecutors: map[domain.NodeType]NodeExecutor{
			domain.NodeTypeExit: NewExitNodeExecutor(),
		},
		logger: mockLogger,
	}

	workspaceID := "ws1"
	nodeID := "exit_node"

	contacts := []*domain.ContactAutomationWithWorkspace{
		{
			WorkspaceID: workspaceID,
			ContactAutomation: domain.ContactAutomation{
				ID:            "ca1",
				AutomationID:  "auto1",
				ContactEmail:  "test1@example.com",
				CurrentNodeID: &nodeID,
				Status:        domain.ContactAutomationStatusActive,
			},
		},
		{
			WorkspaceID: workspaceID,
			ContactAutomation: domain.ContactAutomation{
				ID:            "ca2",
				AutomationID:  "auto1",
				ContactEmail:  "test2@example.com",
				CurrentNodeID: &nodeID,
				Status:        domain.ContactAutomationStatusActive,
			},
		},
	}

	exitNode := &domain.AutomationNode{
		ID:   nodeID,
		Type: domain.NodeTypeExit,
	}

	automation := &domain.Automation{
		ID:     "auto1",
		Name:   "Test Automation",
		Status: domain.AutomationStatusLive,
		Nodes:  []*domain.AutomationNode{exitNode},
	}

	contact1 := &domain.Contact{Email: "test1@example.com"}
	contact2 := &domain.Contact{Email: "test2@example.com"}

	mockAutomationRepo.EXPECT().GetScheduledContactAutomationsGlobal(gomock.Any(), gomock.Any(), 50).Return(contacts, nil)

	// For first contact
	mockAutomationRepo.EXPECT().GetByID(gomock.Any(), workspaceID, "auto1").Return(automation, nil)
	mockContactRepo.EXPECT().GetContactByEmail(gomock.Any(), workspaceID, "test1@example.com").Return(contact1, nil)
	mockAutomationRepo.EXPECT().CreateNodeExecution(gomock.Any(), workspaceID, gomock.Any()).Return(nil)
	mockAutomationRepo.EXPECT().GetNodeExecutions(gomock.Any(), workspaceID, "ca1").Return([]*domain.NodeExecution{}, nil)
	mockAutomationRepo.EXPECT().UpdateContactAutomation(gomock.Any(), workspaceID, gomock.Any()).Return(nil)
	mockAutomationRepo.EXPECT().UpdateNodeExecution(gomock.Any(), workspaceID, gomock.Any()).Return(nil)
	mockAutomationRepo.EXPECT().IncrementAutomationStat(gomock.Any(), workspaceID, "auto1", "completed").Return(nil)

	// For second contact
	mockAutomationRepo.EXPECT().GetByID(gomock.Any(), workspaceID, "auto1").Return(automation, nil)
	mockContactRepo.EXPECT().GetContactByEmail(gomock.Any(), workspaceID, "test2@example.com").Return(contact2, nil)
	mockAutomationRepo.EXPECT().CreateNodeExecution(gomock.Any(), workspaceID, gomock.Any()).Return(nil)
	mockAutomationRepo.EXPECT().GetNodeExecutions(gomock.Any(), workspaceID, "ca2").Return([]*domain.NodeExecution{}, nil)
	mockAutomationRepo.EXPECT().UpdateContactAutomation(gomock.Any(), workspaceID, gomock.Any()).Return(nil)
	mockAutomationRepo.EXPECT().UpdateNodeExecution(gomock.Any(), workspaceID, gomock.Any()).Return(nil)
	mockAutomationRepo.EXPECT().IncrementAutomationStat(gomock.Any(), workspaceID, "auto1", "completed").Return(nil)

	processed, err := executor.ProcessBatch(context.Background(), 50)
	require.NoError(t, err)
	assert.Equal(t, 2, processed)
}

func TestAutomationExecutor_ProcessBatch_Empty(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAutomationRepo := mocks.NewMockAutomationRepository(ctrl)
	mockLogger := setupMockLogger(ctrl)

	executor := &AutomationExecutor{
		automationRepo: mockAutomationRepo,
		nodeExecutors:  map[domain.NodeType]NodeExecutor{},
		logger:         mockLogger,
	}

	mockAutomationRepo.EXPECT().GetScheduledContactAutomationsGlobal(gomock.Any(), gomock.Any(), 50).Return([]*domain.ContactAutomationWithWorkspace{}, nil)

	processed, err := executor.ProcessBatch(context.Background(), 50)
	require.NoError(t, err)
	assert.Equal(t, 0, processed)
}

func TestAutomationExecutor_ProcessBatch_PartialFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAutomationRepo := mocks.NewMockAutomationRepository(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockLogger := setupMockLogger(ctrl)

	executor := &AutomationExecutor{
		automationRepo: mockAutomationRepo,
		contactRepo:    mockContactRepo,
		nodeExecutors: map[domain.NodeType]NodeExecutor{
			domain.NodeTypeExit: NewExitNodeExecutor(),
		},
		logger: mockLogger,
	}

	workspaceID := "ws1"
	nodeID := "exit_node"

	contacts := []*domain.ContactAutomationWithWorkspace{
		{
			WorkspaceID: workspaceID,
			ContactAutomation: domain.ContactAutomation{
				ID:            "ca1",
				AutomationID:  "auto1",
				ContactEmail:  "test1@example.com",
				CurrentNodeID: &nodeID,
				Status:        domain.ContactAutomationStatusActive,
				MaxRetries:    3,
			},
		},
		{
			WorkspaceID: workspaceID,
			ContactAutomation: domain.ContactAutomation{
				ID:            "ca2",
				AutomationID:  "auto1",
				ContactEmail:  "test2@example.com",
				CurrentNodeID: &nodeID,
				Status:        domain.ContactAutomationStatusActive,
			},
		},
	}

	exitNode := &domain.AutomationNode{
		ID:   nodeID,
		Type: domain.NodeTypeExit,
	}

	automation := &domain.Automation{
		ID:     "auto1",
		Name:   "Test Automation",
		Status: domain.AutomationStatusLive,
		Nodes:  []*domain.AutomationNode{exitNode},
	}

	contact2 := &domain.Contact{Email: "test2@example.com"}

	mockAutomationRepo.EXPECT().GetScheduledContactAutomationsGlobal(gomock.Any(), gomock.Any(), 50).Return(contacts, nil)

	// First contact fails
	mockAutomationRepo.EXPECT().GetByID(gomock.Any(), workspaceID, "auto1").Return(automation, nil)
	mockContactRepo.EXPECT().GetContactByEmail(gomock.Any(), workspaceID, "test1@example.com").Return(nil, errors.New("contact not found"))
	mockAutomationRepo.EXPECT().CreateNodeExecution(gomock.Any(), workspaceID, gomock.Any()).Return(nil)
	mockAutomationRepo.EXPECT().UpdateContactAutomation(gomock.Any(), workspaceID, gomock.Any()).Return(nil)

	// Second contact succeeds
	mockAutomationRepo.EXPECT().GetByID(gomock.Any(), workspaceID, "auto1").Return(automation, nil)
	mockContactRepo.EXPECT().GetContactByEmail(gomock.Any(), workspaceID, "test2@example.com").Return(contact2, nil)
	mockAutomationRepo.EXPECT().CreateNodeExecution(gomock.Any(), workspaceID, gomock.Any()).Return(nil)
	mockAutomationRepo.EXPECT().GetNodeExecutions(gomock.Any(), workspaceID, "ca2").Return([]*domain.NodeExecution{}, nil)
	mockAutomationRepo.EXPECT().UpdateContactAutomation(gomock.Any(), workspaceID, gomock.Any()).Return(nil)
	mockAutomationRepo.EXPECT().UpdateNodeExecution(gomock.Any(), workspaceID, gomock.Any()).Return(nil)
	mockAutomationRepo.EXPECT().IncrementAutomationStat(gomock.Any(), workspaceID, "auto1", "completed").Return(nil)

	processed, err := executor.ProcessBatch(context.Background(), 50)
	require.NoError(t, err)
	// Both are "processed" - first one scheduled for retry, second one completed
	// The handleError function handles errors internally and returns nil
	assert.Equal(t, 2, processed)
}

func TestAutomationExecutor_handleError_Retry(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAutomationRepo := mocks.NewMockAutomationRepository(ctrl)
	mockLogger := setupMockLogger(ctrl)

	executor := &AutomationExecutor{
		automationRepo: mockAutomationRepo,
		logger:         mockLogger,
	}

	workspaceID := "ws1"
	nodeID := "node1"

	ca := &domain.ContactAutomation{
		ID:            "ca1",
		AutomationID:  "auto1",
		ContactEmail:  "test@example.com",
		CurrentNodeID: &nodeID,
		Status:        domain.ContactAutomationStatusActive,
		RetryCount:    0,
		MaxRetries:    3,
	}

	mockAutomationRepo.EXPECT().CreateNodeExecution(gomock.Any(), workspaceID, gomock.Any()).Return(nil)
	mockAutomationRepo.EXPECT().UpdateContactAutomation(gomock.Any(), workspaceID, gomock.Any()).Return(nil)

	err := executor.handleError(context.Background(), workspaceID, ca, errors.New("test error"), "test context")
	require.NoError(t, err)

	assert.Equal(t, 1, ca.RetryCount)
	assert.NotNil(t, ca.ScheduledAt)
	assert.Contains(t, *ca.LastError, "test error")
	// Should have exponential backoff - 2 minutes for first retry (1<<1 = 2)
	expectedTime := time.Now().UTC().Add(2 * time.Minute)
	assert.WithinDuration(t, expectedTime, *ca.ScheduledAt, 10*time.Second)
}

func TestAutomationExecutor_handleError_MaxRetriesExceeded(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAutomationRepo := mocks.NewMockAutomationRepository(ctrl)
	mockLogger := setupMockLogger(ctrl)

	executor := &AutomationExecutor{
		automationRepo: mockAutomationRepo,
		logger:         mockLogger,
	}

	workspaceID := "ws1"
	nodeID := "node1"

	ca := &domain.ContactAutomation{
		ID:            "ca1",
		AutomationID:  "auto1",
		ContactEmail:  "test@example.com",
		CurrentNodeID: &nodeID,
		Status:        domain.ContactAutomationStatusActive,
		RetryCount:    2,
		MaxRetries:    3,
	}

	mockAutomationRepo.EXPECT().IncrementAutomationStat(gomock.Any(), workspaceID, "auto1", "failed").Return(nil)
	mockAutomationRepo.EXPECT().CreateNodeExecution(gomock.Any(), workspaceID, gomock.Any()).Return(nil)
	mockAutomationRepo.EXPECT().UpdateContactAutomation(gomock.Any(), workspaceID, gomock.Any()).Return(nil)

	err := executor.handleError(context.Background(), workspaceID, ca, errors.New("test error"), "test context")
	require.NoError(t, err)

	assert.Equal(t, 3, ca.RetryCount)
	assert.Equal(t, domain.ContactAutomationStatusFailed, ca.Status)
}

func TestAutomationExecutor_createNodeExecution(t *testing.T) {
	executor := &AutomationExecutor{}

	ca := &domain.ContactAutomation{
		ID:           "ca1",
		AutomationID: "auto1",
		ContactEmail: "test@example.com",
	}

	node := &domain.AutomationNode{
		ID:   "node1",
		Type: domain.NodeTypeDelay,
	}

	entry := executor.createNodeExecution(ca, node, domain.NodeActionProcessing)

	assert.NotEmpty(t, entry.ID)
	assert.Equal(t, "ca1", entry.ContactAutomationID)
	assert.Equal(t, "node1", entry.NodeID)
	assert.Equal(t, domain.NodeTypeDelay, entry.NodeType)
	assert.Equal(t, domain.NodeActionProcessing, entry.Action)
	assert.NotZero(t, entry.EnteredAt)
}

func TestAutomationExecutor_buildContextFromNodeExecutions(t *testing.T) {
	t.Run("aggregates completed entries by nodeID", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockAutomationRepo := mocks.NewMockAutomationRepository(ctrl)
		mockLogger := setupMockLogger(ctrl)

		executor := &AutomationExecutor{
			automationRepo: mockAutomationRepo,
			logger:         mockLogger,
		}

		workspaceID := "ws1"
		contactAutomationID := "ca1"

		entries := []*domain.NodeExecution{
			{
				ID:                  "exec1",
				ContactAutomationID: contactAutomationID,
				NodeID:              "delay_node1",
				NodeType:            domain.NodeTypeDelay,
				Action:              domain.NodeActionCompleted,
				Output: map[string]interface{}{
					"node_type":      "delay",
					"delay_duration": 30,
					"delay_unit":     "minutes",
				},
			},
			{
				ID:                  "exec2",
				ContactAutomationID: contactAutomationID,
				NodeID:              "email_node2",
				NodeType:            domain.NodeTypeEmail,
				Action:              domain.NodeActionCompleted,
				Output: map[string]interface{}{
					"node_type":   "email",
					"template_id": "tpl123",
					"message_id":  "msg456",
				},
			},
		}

		mockAutomationRepo.EXPECT().
			GetNodeExecutions(gomock.Any(), workspaceID, contactAutomationID).
			Return(entries, nil)

		ctx := context.Background()
		result, err := executor.buildContextFromNodeExecutions(ctx, workspaceID, contactAutomationID)
		require.NoError(t, err)

		// Verify context contains entries keyed by nodeID
		assert.Len(t, result, 2)

		delayOutput, ok := result["delay_node1"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "delay", delayOutput["node_type"])
		assert.Equal(t, 30, delayOutput["delay_duration"])

		emailOutput, ok := result["email_node2"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "email", emailOutput["node_type"])
		assert.Equal(t, "tpl123", emailOutput["template_id"])
	})

	t.Run("skips non-completed entries", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockAutomationRepo := mocks.NewMockAutomationRepository(ctrl)
		mockLogger := setupMockLogger(ctrl)

		executor := &AutomationExecutor{
			automationRepo: mockAutomationRepo,
			logger:         mockLogger,
		}

		workspaceID := "ws1"
		contactAutomationID := "ca1"

		entries := []*domain.NodeExecution{
			{
				ID:                  "exec1",
				ContactAutomationID: contactAutomationID,
				NodeID:              "delay_node1",
				NodeType:            domain.NodeTypeDelay,
				Action:              domain.NodeActionCompleted,
				Output: map[string]interface{}{
					"node_type": "delay",
				},
			},
			{
				ID:                  "exec2",
				ContactAutomationID: contactAutomationID,
				NodeID:              "email_node2",
				NodeType:            domain.NodeTypeEmail,
				Action:              domain.NodeActionProcessing, // Not completed
				Output: map[string]interface{}{
					"node_type": "email",
				},
			},
			{
				ID:                  "exec3",
				ContactAutomationID: contactAutomationID,
				NodeID:              "branch_node3",
				NodeType:            domain.NodeTypeBranch,
				Action:              domain.NodeActionFailed, // Not completed
				Output: map[string]interface{}{
					"node_type": "branch",
				},
			},
		}

		mockAutomationRepo.EXPECT().
			GetNodeExecutions(gomock.Any(), workspaceID, contactAutomationID).
			Return(entries, nil)

		ctx := context.Background()
		result, err := executor.buildContextFromNodeExecutions(ctx, workspaceID, contactAutomationID)
		require.NoError(t, err)

		// Only completed entries should be in context
		assert.Len(t, result, 1)
		_, hasDelay := result["delay_node1"]
		assert.True(t, hasDelay)
		_, hasEmail := result["email_node2"]
		assert.False(t, hasEmail)
		_, hasBranch := result["branch_node3"]
		assert.False(t, hasBranch)
	})

	t.Run("skips entries with nil output", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockAutomationRepo := mocks.NewMockAutomationRepository(ctrl)
		mockLogger := setupMockLogger(ctrl)

		executor := &AutomationExecutor{
			automationRepo: mockAutomationRepo,
			logger:         mockLogger,
		}

		workspaceID := "ws1"
		contactAutomationID := "ca1"

		entries := []*domain.NodeExecution{
			{
				ID:                  "exec1",
				ContactAutomationID: contactAutomationID,
				NodeID:              "delay_node1",
				NodeType:            domain.NodeTypeDelay,
				Action:              domain.NodeActionCompleted,
				Output:              nil, // No output
			},
			{
				ID:                  "exec2",
				ContactAutomationID: contactAutomationID,
				NodeID:              "email_node2",
				NodeType:            domain.NodeTypeEmail,
				Action:              domain.NodeActionCompleted,
				Output: map[string]interface{}{
					"node_type": "email",
				},
			},
		}

		mockAutomationRepo.EXPECT().
			GetNodeExecutions(gomock.Any(), workspaceID, contactAutomationID).
			Return(entries, nil)

		ctx := context.Background()
		result, err := executor.buildContextFromNodeExecutions(ctx, workspaceID, contactAutomationID)
		require.NoError(t, err)

		// Only entries with non-nil output should be in context
		assert.Len(t, result, 1)
		_, hasDelay := result["delay_node1"]
		assert.False(t, hasDelay)
		_, hasEmail := result["email_node2"]
		assert.True(t, hasEmail)
	})

	t.Run("returns empty map when no entries exist", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockAutomationRepo := mocks.NewMockAutomationRepository(ctrl)
		mockLogger := setupMockLogger(ctrl)

		executor := &AutomationExecutor{
			automationRepo: mockAutomationRepo,
			logger:         mockLogger,
		}

		workspaceID := "ws1"
		contactAutomationID := "ca1"

		mockAutomationRepo.EXPECT().
			GetNodeExecutions(gomock.Any(), workspaceID, contactAutomationID).
			Return([]*domain.NodeExecution{}, nil)

		ctx := context.Background()
		result, err := executor.buildContextFromNodeExecutions(ctx, workspaceID, contactAutomationID)
		require.NoError(t, err)

		assert.NotNil(t, result)
		assert.Len(t, result, 0)
	})

	t.Run("returns error when repository fails", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockAutomationRepo := mocks.NewMockAutomationRepository(ctrl)
		mockLogger := setupMockLogger(ctrl)

		executor := &AutomationExecutor{
			automationRepo: mockAutomationRepo,
			logger:         mockLogger,
		}

		workspaceID := "ws1"
		contactAutomationID := "ca1"

		mockAutomationRepo.EXPECT().
			GetNodeExecutions(gomock.Any(), workspaceID, contactAutomationID).
			Return(nil, errors.New("database error"))

		ctx := context.Background()
		result, err := executor.buildContextFromNodeExecutions(ctx, workspaceID, contactAutomationID)
		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "database error")
	})
}

func TestAutomationExecutor_Execute_PassesExecutionContext(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAutomationRepo := mocks.NewMockAutomationRepository(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockLogger := setupMockLogger(ctrl)

	// Create a custom node executor that verifies ExecutionContext is passed
	var capturedContext map[string]interface{}
	customExecutor := &testNodeExecutor{
		nodeType: domain.NodeTypeDelay,
		execute: func(ctx context.Context, params NodeExecutionParams) (*NodeExecutionResult, error) {
			capturedContext = params.ExecutionContext
			nextNode := "next_node"
			return &NodeExecutionResult{
				NextNodeID: &nextNode,
				Status:     domain.ContactAutomationStatusActive,
				Output: map[string]interface{}{
					"node_type": "delay",
				},
			}, nil
		},
	}

	executor := &AutomationExecutor{
		automationRepo: mockAutomationRepo,
		contactRepo:    mockContactRepo,
		nodeExecutors: map[domain.NodeType]NodeExecutor{
			domain.NodeTypeDelay: customExecutor,
		},
		logger: mockLogger,
	}

	workspaceID := "ws1"
	nodeID := "delay_node"

	contactAutomation := &domain.ContactAutomation{
		ID:            "ca1",
		AutomationID:  "auto1",
		ContactEmail:  "test@example.com",
		CurrentNodeID: &nodeID,
		Status:        domain.ContactAutomationStatusActive,
	}

	delayNode := &domain.AutomationNode{
		ID:     nodeID,
		Type:   domain.NodeTypeDelay,
		Config: map[string]interface{}{},
	}

	automation := &domain.Automation{
		ID:     "auto1",
		Name:   "Test Automation",
		Status: domain.AutomationStatusLive,
		Nodes:  []*domain.AutomationNode{delayNode},
	}

	contact := &domain.Contact{Email: "test@example.com"}

	// Previous node executions that should be passed as context
	previousExecutions := []*domain.NodeExecution{
		{
			ID:                  "exec1",
			ContactAutomationID: "ca1",
			NodeID:              "trigger_node",
			NodeType:            domain.NodeTypeTrigger,
			Action:              domain.NodeActionCompleted,
			Output: map[string]interface{}{
				"node_type":    "trigger",
				"trigger_data": "test_value",
			},
		},
	}

	mockAutomationRepo.EXPECT().GetByID(gomock.Any(), workspaceID, "auto1").Return(automation, nil)
	mockContactRepo.EXPECT().GetContactByEmail(gomock.Any(), workspaceID, "test@example.com").Return(contact, nil)
	mockAutomationRepo.EXPECT().CreateNodeExecution(gomock.Any(), workspaceID, gomock.Any()).Return(nil)
	mockAutomationRepo.EXPECT().GetNodeExecutions(gomock.Any(), workspaceID, "ca1").Return(previousExecutions, nil)
	mockAutomationRepo.EXPECT().UpdateContactAutomation(gomock.Any(), workspaceID, gomock.Any()).Return(nil)
	mockAutomationRepo.EXPECT().UpdateNodeExecution(gomock.Any(), workspaceID, gomock.Any()).Return(nil)

	err := executor.Execute(context.Background(), workspaceID, contactAutomation)
	require.NoError(t, err)

	// Verify that ExecutionContext was populated with previous node outputs
	require.NotNil(t, capturedContext)
	assert.Len(t, capturedContext, 1)

	triggerOutput, ok := capturedContext["trigger_node"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "trigger", triggerOutput["node_type"])
	assert.Equal(t, "test_value", triggerOutput["trigger_data"])
}

// testNodeExecutor is a test helper that implements NodeExecutor
type testNodeExecutor struct {
	nodeType domain.NodeType
	execute  func(ctx context.Context, params NodeExecutionParams) (*NodeExecutionResult, error)
}

func (e *testNodeExecutor) NodeType() domain.NodeType {
	return e.nodeType
}

func (e *testNodeExecutor) Execute(ctx context.Context, params NodeExecutionParams) (*NodeExecutionResult, error) {
	return e.execute(ctx, params)
}
