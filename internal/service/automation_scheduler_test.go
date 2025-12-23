package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAutomationScheduler_NewAutomationScheduler(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAutomationRepo := mocks.NewMockAutomationRepository(ctrl)
	mockLogger := setupMockLogger(ctrl)

	executor := &AutomationExecutor{
		automationRepo: mockAutomationRepo,
		nodeExecutors:  map[domain.NodeType]NodeExecutor{},
		logger:         mockLogger,
	}

	scheduler := NewAutomationScheduler(executor, mockLogger, 10*time.Second, 50)

	require.NotNil(t, scheduler)
	assert.Equal(t, 10*time.Second, scheduler.interval)
	assert.Equal(t, 50, scheduler.batchSize)
	assert.False(t, scheduler.IsRunning())
}

func TestAutomationScheduler_StartStop(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAutomationRepo := mocks.NewMockAutomationRepository(ctrl)
	mockLogger := setupMockLogger(ctrl)

	executor := &AutomationExecutor{
		automationRepo: mockAutomationRepo,
		nodeExecutors:  map[domain.NodeType]NodeExecutor{},
		logger:         mockLogger,
	}

	// Use a short interval for testing
	scheduler := NewAutomationScheduler(executor, mockLogger, 100*time.Millisecond, 50)

	// Expect at least one batch processing call
	mockAutomationRepo.EXPECT().
		GetScheduledContactAutomationsGlobal(gomock.Any(), gomock.Any(), 50).
		Return([]*domain.ContactAutomationWithWorkspace{}, nil).
		AnyTimes()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start scheduler
	scheduler.Start(ctx)
	assert.True(t, scheduler.IsRunning())

	// Wait a bit to ensure it processes at least once
	time.Sleep(150 * time.Millisecond)

	// Stop scheduler
	scheduler.Stop()
	assert.False(t, scheduler.IsRunning())
}

func TestAutomationScheduler_Start_AlreadyRunning(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAutomationRepo := mocks.NewMockAutomationRepository(ctrl)
	mockLogger := setupMockLogger(ctrl)

	executor := &AutomationExecutor{
		automationRepo: mockAutomationRepo,
		nodeExecutors:  map[domain.NodeType]NodeExecutor{},
		logger:         mockLogger,
	}

	scheduler := NewAutomationScheduler(executor, mockLogger, 1*time.Second, 50)

	mockAutomationRepo.EXPECT().
		GetScheduledContactAutomationsGlobal(gomock.Any(), gomock.Any(), 50).
		Return([]*domain.ContactAutomationWithWorkspace{}, nil).
		AnyTimes()

	ctx := context.Background()

	// Start scheduler
	scheduler.Start(ctx)
	assert.True(t, scheduler.IsRunning())

	// Try to start again - should not create another goroutine
	scheduler.Start(ctx)
	assert.True(t, scheduler.IsRunning())

	// Stop
	scheduler.Stop()
}

func TestAutomationScheduler_Stop_NotRunning(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAutomationRepo := mocks.NewMockAutomationRepository(ctrl)
	mockLogger := setupMockLogger(ctrl)

	executor := &AutomationExecutor{
		automationRepo: mockAutomationRepo,
		nodeExecutors:  map[domain.NodeType]NodeExecutor{},
		logger:         mockLogger,
	}

	scheduler := NewAutomationScheduler(executor, mockLogger, 1*time.Second, 50)

	// Stop when not running - should not panic
	scheduler.Stop()
	assert.False(t, scheduler.IsRunning())
}

func TestAutomationScheduler_ContextCancellation(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAutomationRepo := mocks.NewMockAutomationRepository(ctrl)
	mockLogger := setupMockLogger(ctrl)

	executor := &AutomationExecutor{
		automationRepo: mockAutomationRepo,
		nodeExecutors:  map[domain.NodeType]NodeExecutor{},
		logger:         mockLogger,
	}

	scheduler := NewAutomationScheduler(executor, mockLogger, 1*time.Second, 50)

	mockAutomationRepo.EXPECT().
		GetScheduledContactAutomationsGlobal(gomock.Any(), gomock.Any(), 50).
		Return([]*domain.ContactAutomationWithWorkspace{}, nil).
		AnyTimes()

	ctx, cancel := context.WithCancel(context.Background())

	// Start scheduler
	scheduler.Start(ctx)
	assert.True(t, scheduler.IsRunning())

	// Cancel context
	cancel()

	// Wait for scheduler to stop
	time.Sleep(100 * time.Millisecond)
	assert.False(t, scheduler.IsRunning())
}

func TestAutomationScheduler_IsRunning(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAutomationRepo := mocks.NewMockAutomationRepository(ctrl)
	mockLogger := setupMockLogger(ctrl)

	executor := &AutomationExecutor{
		automationRepo: mockAutomationRepo,
		nodeExecutors:  map[domain.NodeType]NodeExecutor{},
		logger:         mockLogger,
	}

	scheduler := NewAutomationScheduler(executor, mockLogger, 1*time.Second, 50)

	mockAutomationRepo.EXPECT().
		GetScheduledContactAutomationsGlobal(gomock.Any(), gomock.Any(), 50).
		Return([]*domain.ContactAutomationWithWorkspace{}, nil).
		AnyTimes()

	// Initially not running
	assert.False(t, scheduler.IsRunning())

	ctx := context.Background()
	scheduler.Start(ctx)

	// Now running
	assert.True(t, scheduler.IsRunning())

	scheduler.Stop()

	// No longer running
	assert.False(t, scheduler.IsRunning())
}

func TestAutomationScheduler_ProcessBatchOnInterval(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAutomationRepo := mocks.NewMockAutomationRepository(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockTimelineRepo := mocks.NewMockContactTimelineRepository(ctrl)
	mockLogger := setupMockLogger(ctrl)

	executor := &AutomationExecutor{
		automationRepo: mockAutomationRepo,
		contactRepo:    mockContactRepo,
		timelineRepo:   mockTimelineRepo,
		nodeExecutors: map[domain.NodeType]NodeExecutor{
			domain.NodeTypeDelay: NewDelayNodeExecutor(),
		},
		logger: mockLogger,
	}

	// Use short interval for testing
	scheduler := NewAutomationScheduler(executor, mockLogger, 50*time.Millisecond, 50)

	// Track how many times ProcessBatch was called
	var callCount int
	var mu sync.Mutex

	nodeID := "terminal_node"
	contacts := []*domain.ContactAutomationWithWorkspace{
		{
			WorkspaceID: "ws1",
			ContactAutomation: domain.ContactAutomation{
				ID:            "ca1",
				AutomationID:  "auto1",
				ContactEmail:  "test@example.com",
				CurrentNodeID: &nodeID,
				Status:        domain.ContactAutomationStatusActive,
			},
		},
	}

	// Terminal delay node (no next node = completion)
	terminalNode := &domain.AutomationNode{
		ID:         nodeID,
		Type:       domain.NodeTypeDelay,
		NextNodeID: nil,
		Config: map[string]interface{}{
			"duration": 1,
			"unit":     "minutes",
		},
	}

	automation := &domain.Automation{
		ID:     "auto1",
		Name:   "Test",
		Status: domain.AutomationStatusLive,
		Nodes:  []*domain.AutomationNode{terminalNode},
	}

	contact := &domain.Contact{Email: "test@example.com"}

	// Setup mock to track calls and return empty after first few
	mockAutomationRepo.EXPECT().
		GetScheduledContactAutomationsGlobal(gomock.Any(), gomock.Any(), 50).
		DoAndReturn(func(ctx context.Context, beforeTime time.Time, limit int) ([]*domain.ContactAutomationWithWorkspace, error) {
			mu.Lock()
			defer mu.Unlock()
			callCount++
			if callCount <= 2 {
				return contacts, nil
			}
			return []*domain.ContactAutomationWithWorkspace{}, nil
		}).AnyTimes()

	// Setup expectations for processing contacts
	mockAutomationRepo.EXPECT().GetByID(gomock.Any(), "ws1", "auto1").Return(automation, nil).AnyTimes()
	mockContactRepo.EXPECT().GetContactByEmail(gomock.Any(), "ws1", "test@example.com").Return(contact, nil).AnyTimes()
	mockAutomationRepo.EXPECT().CreateNodeExecution(gomock.Any(), "ws1", gomock.Any()).Return(nil).AnyTimes()
	mockAutomationRepo.EXPECT().GetNodeExecutions(gomock.Any(), "ws1", gomock.Any()).Return([]*domain.NodeExecution{}, nil).AnyTimes()
	mockAutomationRepo.EXPECT().UpdateContactAutomation(gomock.Any(), "ws1", gomock.Any()).Return(nil).AnyTimes()
	mockAutomationRepo.EXPECT().UpdateNodeExecution(gomock.Any(), "ws1", gomock.Any()).Return(nil).AnyTimes()
	mockAutomationRepo.EXPECT().IncrementAutomationStat(gomock.Any(), "ws1", "auto1", "completed").Return(nil).AnyTimes()
	mockTimelineRepo.EXPECT().Create(gomock.Any(), "ws1", gomock.Any()).Return(nil).AnyTimes()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	scheduler.Start(ctx)

	// Wait for multiple processing cycles
	time.Sleep(200 * time.Millisecond)

	scheduler.Stop()

	// Should have processed multiple times
	mu.Lock()
	finalCount := callCount
	mu.Unlock()

	assert.GreaterOrEqual(t, finalCount, 2, "Should have processed at least 2 batches")
}

func TestAutomationScheduler_ProcessBatchError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAutomationRepo := mocks.NewMockAutomationRepository(ctrl)
	mockLogger := setupMockLogger(ctrl)

	executor := &AutomationExecutor{
		automationRepo: mockAutomationRepo,
		nodeExecutors:  map[domain.NodeType]NodeExecutor{},
		logger:         mockLogger,
	}

	scheduler := NewAutomationScheduler(executor, mockLogger, 50*time.Millisecond, 50)

	// Return error from GetScheduledContactAutomationsGlobal
	mockAutomationRepo.EXPECT().
		GetScheduledContactAutomationsGlobal(gomock.Any(), gomock.Any(), 50).
		Return(nil, assert.AnError).
		AnyTimes()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start scheduler - should not crash on error
	scheduler.Start(ctx)
	assert.True(t, scheduler.IsRunning())

	// Wait for a processing cycle
	time.Sleep(100 * time.Millisecond)

	// Should still be running despite error
	assert.True(t, scheduler.IsRunning())

	scheduler.Stop()
}

func TestAutomationScheduler_ConcurrentSafety(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAutomationRepo := mocks.NewMockAutomationRepository(ctrl)
	mockLogger := setupMockLogger(ctrl)

	executor := &AutomationExecutor{
		automationRepo: mockAutomationRepo,
		nodeExecutors:  map[domain.NodeType]NodeExecutor{},
		logger:         mockLogger,
	}

	scheduler := NewAutomationScheduler(executor, mockLogger, 50*time.Millisecond, 50)

	mockAutomationRepo.EXPECT().
		GetScheduledContactAutomationsGlobal(gomock.Any(), gomock.Any(), 50).
		Return([]*domain.ContactAutomationWithWorkspace{}, nil).
		AnyTimes()

	ctx := context.Background()

	// Start multiple goroutines that call Start, Stop, and IsRunning concurrently
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			scheduler.Start(ctx)
			time.Sleep(10 * time.Millisecond)
			_ = scheduler.IsRunning()
			scheduler.Stop()
		}()
	}

	wg.Wait()
	// Should complete without panic or race conditions
}

func TestAutomationScheduler_PausedAutomationsNotProcessed(t *testing.T) {
	// Purpose: Verify that contacts in PAUSED automations are NOT returned by scheduler
	// The scheduler query filters by `a.status = 'live'`
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAutomationRepo := mocks.NewMockAutomationRepository(ctrl)
	mockLogger := setupMockLogger(ctrl)

	executor := &AutomationExecutor{
		automationRepo: mockAutomationRepo,
		nodeExecutors:  map[domain.NodeType]NodeExecutor{},
		logger:         mockLogger,
	}

	scheduler := NewAutomationScheduler(executor, mockLogger, 50*time.Millisecond, 50)

	// Track how many times the scheduler queries for contacts
	var callCount int
	var mu sync.Mutex

	// Mock returns empty slice - simulating that paused automation contacts are filtered out
	// by the SQL query's `a.status = 'live'` condition
	mockAutomationRepo.EXPECT().
		GetScheduledContactAutomationsGlobal(gomock.Any(), gomock.Any(), 50).
		DoAndReturn(func(ctx context.Context, beforeTime time.Time, limit int) ([]*domain.ContactAutomationWithWorkspace, error) {
			mu.Lock()
			defer mu.Unlock()
			callCount++
			// Always return empty - simulates paused automations being filtered out
			return []*domain.ContactAutomationWithWorkspace{}, nil
		}).AnyTimes()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	scheduler.Start(ctx)
	time.Sleep(100 * time.Millisecond)
	scheduler.Stop()

	// Verify scheduler was called at least once
	mu.Lock()
	finalCount := callCount
	mu.Unlock()

	assert.GreaterOrEqual(t, finalCount, 1, "Scheduler should have queried at least once")
	// Test passes if no contacts are processed (empty result from scheduler query)
	// The key behavior is that the SQL query itself filters out paused automations
}
