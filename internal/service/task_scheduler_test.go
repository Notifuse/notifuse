package service

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

// mockTaskExecutor is a simple mock for ExecutePendingTasks
type mockTaskExecutor struct {
	executeFn func(ctx context.Context, maxTasks int) error
	callCount int32
}

func (m *mockTaskExecutor) ExecutePendingTasks(ctx context.Context, maxTasks int) error {
	atomic.AddInt32(&m.callCount, 1)
	if m.executeFn != nil {
		return m.executeFn(ctx, maxTasks)
	}
	return nil
}

func (m *mockTaskExecutor) getCallCount() int32 {
	return atomic.LoadInt32(&m.callCount)
}

// taskExecutorWrapper wraps a TaskService to match the interface needed
type taskExecutorWrapper struct {
	service *TaskService
}

func (w *taskExecutorWrapper) ExecutePendingTasks(ctx context.Context, maxTasks int) error {
	return w.service.ExecutePendingTasks(ctx, maxTasks)
}

func TestNewTaskScheduler(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockExecutor := &mockTaskExecutor{}
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	scheduler := NewTaskScheduler(mockExecutor, mockLogger, 1*time.Second, 50)

	assert.NotNil(t, scheduler)
	assert.Equal(t, 1*time.Second, scheduler.interval)
	assert.Equal(t, 50, scheduler.maxTasks)
	assert.False(t, scheduler.IsRunning())
	assert.NotNil(t, scheduler.stopChan)
	assert.NotNil(t, scheduler.stoppedChan)
}

func TestTaskScheduler_StartAndStop(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockExecutor := &mockTaskExecutor{}
	mockLogger := createMockLoggerForScheduler(ctrl)

	scheduler := NewTaskScheduler(mockExecutor, mockLogger, 100*time.Millisecond, 50)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start scheduler
	scheduler.Start(ctx)
	assert.True(t, scheduler.IsRunning())

	// Wait a bit for it to tick at least once
	time.Sleep(250 * time.Millisecond)

	// Stop scheduler
	scheduler.Stop()

	// Verify it stopped
	time.Sleep(100 * time.Millisecond)
	assert.False(t, scheduler.IsRunning())
	
	// Should have executed at least once
	assert.GreaterOrEqual(t, mockExecutor.getCallCount(), int32(1))
}

func TestTaskScheduler_ExecutesImmediatelyOnStart(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockExecutor := &mockTaskExecutor{}
	mockLogger := createMockLoggerForScheduler(ctrl)

	scheduler := NewTaskScheduler(mockExecutor, mockLogger, 5*time.Second, 50)

	ctx := context.Background()
	scheduler.Start(ctx)

	// Should execute immediately, not wait for first tick
	time.Sleep(50 * time.Millisecond)

	count := mockExecutor.getCallCount()
	assert.GreaterOrEqual(t, count, int32(1), "Should execute at least once immediately")

	scheduler.Stop()
}

func TestTaskScheduler_ExecutesTasksPeriodically(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockExecutor := &mockTaskExecutor{}
	mockLogger := createMockLoggerForScheduler(ctrl)

	scheduler := NewTaskScheduler(mockExecutor, mockLogger, 100*time.Millisecond, 50)

	ctx, cancel := context.WithTimeout(context.Background(), 350*time.Millisecond)
	defer cancel()

	scheduler.Start(ctx)

	// Wait for context to expire
	<-ctx.Done()
	scheduler.Stop()

	// Should have executed multiple times (immediate + ~3 ticks)
	count := mockExecutor.getCallCount()
	assert.GreaterOrEqual(t, count, int32(3), "Should execute at least 3 times")
}

func TestTaskScheduler_StopsOnContextCancellation(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockExecutor := &mockTaskExecutor{}
	mockLogger := createMockLoggerForScheduler(ctrl)

	scheduler := NewTaskScheduler(mockExecutor, mockLogger, 1*time.Second, 50)

	ctx, cancel := context.WithCancel(context.Background())

	scheduler.Start(ctx)
	assert.True(t, scheduler.IsRunning())

	// Cancel context
	cancel()

	// Wait for scheduler to stop
	time.Sleep(200 * time.Millisecond)

	// Scheduler should have stopped
	assert.False(t, scheduler.IsRunning())
}

func TestTaskScheduler_StopsOnStopSignal(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockExecutor := &mockTaskExecutor{}
	mockLogger := createMockLoggerForScheduler(ctrl)

	scheduler := NewTaskScheduler(mockExecutor, mockLogger, 1*time.Second, 50)

	ctx := context.Background()

	scheduler.Start(ctx)
	assert.True(t, scheduler.IsRunning())

	// Stop via Stop() method
	scheduler.Stop()

	// Scheduler should have stopped
	time.Sleep(100 * time.Millisecond)
	assert.False(t, scheduler.IsRunning())
}

func TestTaskScheduler_DoubleStart(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockExecutor := &mockTaskExecutor{}
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Set up logging expectations
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	// Expect warning log for second start attempt
	mockLogger.EXPECT().Warn("Task scheduler already running").Times(1)

	scheduler := NewTaskScheduler(mockExecutor, mockLogger, 1*time.Second, 50)

	ctx := context.Background()

	scheduler.Start(ctx)
	assert.True(t, scheduler.IsRunning())

	// Try to start again - should log warning and not start second goroutine
	scheduler.Start(ctx)
	assert.True(t, scheduler.IsRunning())

	scheduler.Stop()
}

func TestTaskScheduler_StopBeforeStart(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockExecutor := &mockTaskExecutor{}
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	scheduler := NewTaskScheduler(mockExecutor, mockLogger, 1*time.Second, 50)

	// Stop before starting - should be no-op
	scheduler.Stop()
	assert.False(t, scheduler.IsRunning())
}

func TestTaskScheduler_MultipleStopCalls(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockExecutor := &mockTaskExecutor{}
	mockLogger := createMockLoggerForScheduler(ctrl)

	scheduler := NewTaskScheduler(mockExecutor, mockLogger, 1*time.Second, 50)

	ctx := context.Background()
	scheduler.Start(ctx)

	// First stop
	scheduler.Stop()
	assert.False(t, scheduler.IsRunning())

	// Second stop - should be no-op
	scheduler.Stop()
	assert.False(t, scheduler.IsRunning())
}

func TestTaskScheduler_HandlesExecutionErrors(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockExecutor := &mockTaskExecutor{
		executeFn: func(ctx context.Context, maxTasks int) error {
			return errors.New("execution failed")
		},
	}
	mockLogger := createMockLoggerForScheduler(ctrl)

	// Set up error expectations
	mockLoggerWithError := pkgmocks.NewMockLogger(ctrl)
	mockLoggerWithElapsed := pkgmocks.NewMockLogger(ctrl)

	mockLoggerWithError.EXPECT().
		WithField("elapsed", gomock.Any()).
		Return(mockLoggerWithElapsed).
		AnyTimes()
	mockLoggerWithElapsed.EXPECT().Error(gomock.Any()).AnyTimes()

	mockLogger.EXPECT().
		WithField("error", gomock.Any()).
		Return(mockLoggerWithError).
		AnyTimes()

	scheduler := NewTaskScheduler(mockExecutor, mockLogger, 100*time.Millisecond, 50)

	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()

	scheduler.Start(ctx)

	// Wait for a few ticks
	<-ctx.Done()
	scheduler.Stop()

	// Should have executed at least twice despite errors
	assert.GreaterOrEqual(t, mockExecutor.getCallCount(), int32(2))
}

func TestTaskScheduler_GracefulShutdownTimeout(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockExecutor := &mockTaskExecutor{}
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Expect warning about timeout
	mockLogger.EXPECT().Warn("Task scheduler stop timeout exceeded").Times(1)
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

	// Make a scheduler that takes forever to stop
	scheduler := NewTaskScheduler(mockExecutor, mockLogger, 1*time.Second, 50)

	// Don't actually start it, just close the stoppedChan after delay
	go func() {
		time.Sleep(10 * time.Second) // Longer than timeout
		close(scheduler.stoppedChan)
	}()

	// Mark as running so Stop() will try to stop it
	scheduler.mu.Lock()
	scheduler.running = true
	scheduler.mu.Unlock()

	// This should timeout after 5 seconds
	start := time.Now()
	scheduler.Stop()
	elapsed := time.Since(start)

	// Should have waited for the 5-second timeout
	assert.GreaterOrEqual(t, elapsed, 5*time.Second)
	assert.Less(t, elapsed, 6*time.Second)
}

func TestTaskScheduler_RespectsCancellation(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockExecutor := &mockTaskExecutor{}
	mockLogger := createMockLoggerForScheduler(ctrl)

	scheduler := NewTaskScheduler(mockExecutor, mockLogger, 50*time.Millisecond, 50)

	ctx, cancel := context.WithCancel(context.Background())

	scheduler.Start(ctx)
	assert.True(t, scheduler.IsRunning())

	// Let it run for a bit
	time.Sleep(150 * time.Millisecond)

	countBeforeCancel := mockExecutor.getCallCount()

	// Cancel context
	cancel()

	// Wait for it to stop
	time.Sleep(200 * time.Millisecond)

	countAfterCancel := mockExecutor.getCallCount()

	// Should have stopped and not executed more tasks
	assert.False(t, scheduler.IsRunning())
	// Count should be the same or only slightly higher (race condition on final tick)
	assert.LessOrEqual(t, countAfterCancel-countBeforeCancel, int32(1))
}

func TestTaskScheduler_ConfigurableInterval(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	testCases := []struct {
		name             string
		interval         time.Duration
		testDuration     time.Duration
		minExpectedCalls int32
		maxExpectedCalls int32
	}{
		{
			name:             "Fast interval (50ms)",
			interval:         50 * time.Millisecond,
			testDuration:     250 * time.Millisecond,
			minExpectedCalls: 4, // immediate + ~4 ticks
			maxExpectedCalls: 7,
		},
		{
			name:             "Medium interval (100ms)",
			interval:         100 * time.Millisecond,
			testDuration:     350 * time.Millisecond,
			minExpectedCalls: 3, // immediate + ~3 ticks
			maxExpectedCalls: 5,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockExecutor := &mockTaskExecutor{}
			mockLogger := createMockLoggerForScheduler(ctrl)

			scheduler := NewTaskScheduler(mockExecutor, mockLogger, tc.interval, 50)

			ctx, cancel := context.WithTimeout(context.Background(), tc.testDuration)
			defer cancel()

			scheduler.Start(ctx)

			<-ctx.Done()
			scheduler.Stop()

			count := mockExecutor.getCallCount()
			assert.GreaterOrEqual(t, count, tc.minExpectedCalls, "Should execute at least %d times", tc.minExpectedCalls)
			assert.LessOrEqual(t, count, tc.maxExpectedCalls, "Should execute at most %d times", tc.maxExpectedCalls)
		})
	}
}

func TestTaskScheduler_ConfigurableMaxTasks(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	testCases := []struct {
		name     string
		maxTasks int
	}{
		{"Default (100)", 100},
		{"Low (10)", 10},
		{"High (500)", 500},
		{"Custom (75)", 75},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Verify maxTasks is passed correctly
			var passedMaxTasks int
			mockExecutor := &mockTaskExecutor{
				executeFn: func(ctx context.Context, maxTasks int) error {
					passedMaxTasks = maxTasks
					return nil
				},
			}
			mockLogger := createMockLoggerForScheduler(ctrl)

			scheduler := NewTaskScheduler(mockExecutor, mockLogger, 1*time.Hour, tc.maxTasks)

			ctx := context.Background()
			scheduler.Start(ctx)

			// Wait for immediate execution
			time.Sleep(50 * time.Millisecond)

			scheduler.Stop()

			// Verify the correct maxTasks value was passed
			assert.Equal(t, tc.maxTasks, passedMaxTasks)
		})
	}
}

func TestTaskScheduler_ConcurrentStartCalls(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockExecutor := &mockTaskExecutor{}
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Set up logging expectations
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	// Expect warning for concurrent start attempts (at least one)
	mockLogger.EXPECT().Warn("Task scheduler already running").MinTimes(1)

	scheduler := NewTaskScheduler(mockExecutor, mockLogger, 100*time.Millisecond, 50)

	ctx := context.Background()

	// Start from multiple goroutines concurrently
	var wg atomic.Int32
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Add(-1)
			scheduler.Start(ctx)
		}()
	}

	// Wait for all goroutines to complete
	for wg.Load() > 0 {
		time.Sleep(10 * time.Millisecond)
	}

	// Should still be running
	assert.True(t, scheduler.IsRunning())

	scheduler.Stop()
}

func TestTaskScheduler_StopWaitsForCompletion(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockExecutor := &mockTaskExecutor{
		executeFn: func(ctx context.Context, maxTasks int) error {
			time.Sleep(100 * time.Millisecond)
			return nil
		},
	}
	mockLogger := createMockLoggerForScheduler(ctrl)

	scheduler := NewTaskScheduler(mockExecutor, mockLogger, 50*time.Millisecond, 50)

	ctx := context.Background()
	scheduler.Start(ctx)

	// Let it start executing
	time.Sleep(25 * time.Millisecond)

	// Stop should wait for current execution to complete
	start := time.Now()
	scheduler.Stop()
	elapsed := time.Since(start)

	// Should have waited for the execution to complete (but not the full 5s timeout)
	// Using 65ms to allow for timing variance (100ms task - 25ms head start - ~10ms variance)
	assert.GreaterOrEqual(t, elapsed, 65*time.Millisecond) // Some wait time
	assert.Less(t, elapsed, 5*time.Second)                 // But less than timeout
	assert.False(t, scheduler.IsRunning())
}

func TestTaskScheduler_LogsExecutionTime(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockExecutor := &mockTaskExecutor{}
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Set up detailed logger expectations
	mockLoggerWithInterval := pkgmocks.NewMockLogger(ctrl)
	mockLoggerWithMaxTasks := pkgmocks.NewMockLogger(ctrl)
	mockLoggerWithElapsed := pkgmocks.NewMockLogger(ctrl)

	mockLogger.EXPECT().
		WithField("interval", gomock.Any()).
		Return(mockLoggerWithInterval).
		Times(1)

	mockLoggerWithInterval.EXPECT().
		WithField("max_tasks", gomock.Any()).
		Return(mockLoggerWithMaxTasks).
		Times(1)

	mockLoggerWithMaxTasks.EXPECT().
		Info("Starting internal task scheduler").
		Times(1)

	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()

	mockLogger.EXPECT().
		WithField("elapsed", gomock.Any()).
		Return(mockLoggerWithElapsed).
		AnyTimes()

	mockLoggerWithElapsed.EXPECT().Debug(gomock.Any()).AnyTimes()

	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

	scheduler := NewTaskScheduler(mockExecutor, mockLogger, 1*time.Hour, 50)

	ctx := context.Background()
	scheduler.Start(ctx)

	// Wait for immediate execution
	time.Sleep(50 * time.Millisecond)

	scheduler.Stop()
}

func TestTaskScheduler_LogsErrors(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockExecutor := &mockTaskExecutor{
		executeFn: func(ctx context.Context, maxTasks int) error {
			return errors.New("execution failed")
		},
	}
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Set up error logging expectations
	mockLoggerWithError := pkgmocks.NewMockLogger(ctrl)
	mockLoggerWithElapsed := pkgmocks.NewMockLogger(ctrl)

	mockLogger.EXPECT().
		WithField("error", "execution failed").
		Return(mockLoggerWithError).
		Times(1)

	mockLoggerWithError.EXPECT().
		WithField("elapsed", gomock.Any()).
		Return(mockLoggerWithElapsed).
		Times(1)

	mockLoggerWithElapsed.EXPECT().Error(gomock.Any()).Times(1)

	// Also expect startup logging
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()

	scheduler := NewTaskScheduler(mockExecutor, mockLogger, 1*time.Hour, 50)

	ctx := context.Background()
	scheduler.Start(ctx)

	// Wait for immediate execution
	time.Sleep(50 * time.Millisecond)

	scheduler.Stop()
}

func TestTaskScheduler_RespectsMaxTasksParameter(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	maxTasks := 42
	var receivedMaxTasks int

	mockExecutor := &mockTaskExecutor{
		executeFn: func(ctx context.Context, maxTasks int) error {
			receivedMaxTasks = maxTasks
			return nil
		},
	}
	mockLogger := createMockLoggerForScheduler(ctrl)

	scheduler := NewTaskScheduler(mockExecutor, mockLogger, 100*time.Millisecond, maxTasks)

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	scheduler.Start(ctx)

	<-ctx.Done()
	scheduler.Stop()

	// Verify the correct maxTasks value was passed
	assert.Equal(t, maxTasks, receivedMaxTasks)
}

func TestTaskScheduler_IsRunningThreadSafe(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockExecutor := &mockTaskExecutor{}
	mockLogger := createMockLoggerForScheduler(ctrl)

	scheduler := NewTaskScheduler(mockExecutor, mockLogger, 100*time.Millisecond, 50)

	ctx := context.Background()

	// Call IsRunning from multiple goroutines while starting/stopping
	done := make(chan bool)
	go func() {
		for i := 0; i < 100; i++ {
			_ = scheduler.IsRunning()
			time.Sleep(1 * time.Millisecond)
		}
		done <- true
	}()

	scheduler.Start(ctx)
	time.Sleep(50 * time.Millisecond)
	scheduler.Stop()

	<-done

	// No race conditions should occur (verified by -race flag)
	assert.False(t, scheduler.IsRunning())
}

// Helper functions for creating properly configured mocks

func createMockLoggerForScheduler(ctrl *gomock.Controller) *pkgmocks.MockLogger {
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Set up common expectations for scheduler logging
	mockLoggerWithField := pkgmocks.NewMockLogger(ctrl)

	mockLogger.EXPECT().
		WithField(gomock.Any(), gomock.Any()).
		Return(mockLoggerWithField).
		AnyTimes()

	mockLoggerWithField.EXPECT().
		WithField(gomock.Any(), gomock.Any()).
		Return(mockLoggerWithField).
		AnyTimes()

	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()

	mockLoggerWithField.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLoggerWithField.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLoggerWithField.EXPECT().Error(gomock.Any()).AnyTimes()

	return mockLogger
}
