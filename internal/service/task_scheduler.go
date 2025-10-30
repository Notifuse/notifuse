package service

import (
	"context"
	"sync"
	"time"

	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/Notifuse/notifuse/pkg/tracing"
)

// TaskScheduler manages periodic task execution
// TaskExecutor defines the interface for executing tasks
type TaskExecutor interface {
	ExecutePendingTasks(ctx context.Context, maxTasks int) error
}

type TaskScheduler struct {
	taskExecutor TaskExecutor
	logger       logger.Logger
	interval     time.Duration
	maxTasks     int
	stopChan     chan struct{}
	stoppedChan  chan struct{}
	mu           sync.Mutex
	running      bool
}

// NewTaskScheduler creates a new task scheduler
func NewTaskScheduler(
	taskExecutor TaskExecutor,
	logger logger.Logger,
	interval time.Duration,
	maxTasks int,
) *TaskScheduler {
	return &TaskScheduler{
		taskExecutor: taskExecutor,
		logger:       logger,
		interval:     interval,
		maxTasks:     maxTasks,
		stopChan:     make(chan struct{}),
		stoppedChan:  make(chan struct{}),
	}
}

// Start begins the task execution scheduler
func (s *TaskScheduler) Start(ctx context.Context) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		s.logger.Warn("Task scheduler already running")
		return
	}
	s.running = true
	s.mu.Unlock()

	s.logger.WithField("interval", s.interval).
		WithField("max_tasks", s.maxTasks).
		Info("Starting internal task scheduler")

	go s.run(ctx)
}

// Stop gracefully stops the scheduler
func (s *TaskScheduler) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	s.mu.Unlock()

	s.logger.Info("Stopping task scheduler...")
	close(s.stopChan)

	// Wait for scheduler to stop (with timeout)
	select {
	case <-s.stoppedChan:
		s.logger.Info("Task scheduler stopped successfully")
	case <-time.After(5 * time.Second):
		s.logger.Warn("Task scheduler stop timeout exceeded")
	}
}

// run is the main scheduler loop
func (s *TaskScheduler) run(ctx context.Context) {
	defer close(s.stoppedChan)
	defer func() {
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
	}()

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	// Execute immediately on start
	s.executeTasks(ctx)

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("Task scheduler context cancelled")
			return
		case <-s.stopChan:
			s.logger.Info("Task scheduler received stop signal")
			return
		case <-ticker.C:
			s.executeTasks(ctx)
		}
	}
}

// executeTasks executes pending tasks
func (s *TaskScheduler) executeTasks(ctx context.Context) {
	// codecov:ignore:start
	execCtx, span := tracing.StartServiceSpan(ctx, "TaskScheduler", "executeTasks")
	defer tracing.EndSpan(span, nil)
	// codecov:ignore:end

	s.logger.Debug("Task scheduler tick - executing pending tasks")

	startTime := time.Now()
	err := s.taskExecutor.ExecutePendingTasks(execCtx, s.maxTasks)
	elapsed := time.Since(startTime)

	if err != nil {
		// codecov:ignore:start
		tracing.MarkSpanError(execCtx, err)
		// codecov:ignore:end
		s.logger.WithField("error", err.Error()).
			WithField("elapsed", elapsed).
			Error("Failed to execute pending tasks")
	} else {
		s.logger.WithField("elapsed", elapsed).
			Debug("Pending tasks execution completed")
	}
}

// IsRunning returns whether the scheduler is currently running
func (s *TaskScheduler) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}
