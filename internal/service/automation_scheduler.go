package service

import (
	"context"
	"sync"
	"time"

	"github.com/Notifuse/notifuse/pkg/logger"
)

// AutomationScheduler manages periodic automation execution
type AutomationScheduler struct {
	executor    *AutomationExecutor
	logger      logger.Logger
	interval    time.Duration
	batchSize   int
	stopChan    chan struct{}
	stoppedChan chan struct{}
	mu          sync.Mutex
	running     bool
}

// NewAutomationScheduler creates a new automation scheduler
func NewAutomationScheduler(
	executor *AutomationExecutor,
	log logger.Logger,
	interval time.Duration,
	batchSize int,
) *AutomationScheduler {
	return &AutomationScheduler{
		executor:    executor,
		logger:      log,
		interval:    interval,
		batchSize:   batchSize,
		stopChan:    make(chan struct{}),
		stoppedChan: make(chan struct{}),
	}
}

// Start begins the automation execution scheduler
func (s *AutomationScheduler) Start(ctx context.Context) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		s.logger.Warn("Automation scheduler already running")
		return
	}
	s.running = true
	s.mu.Unlock()

	s.logger.WithField("interval", s.interval).
		WithField("batch_size", s.batchSize).
		Info("Starting automation scheduler")

	go s.run(ctx)
}

// Stop gracefully stops the scheduler
func (s *AutomationScheduler) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	s.mu.Unlock()

	s.logger.Info("Stopping automation scheduler...")
	close(s.stopChan)

	select {
	case <-s.stoppedChan:
		s.logger.Info("Automation scheduler stopped successfully")
	case <-time.After(5 * time.Second):
		s.logger.Warn("Automation scheduler stop timeout exceeded")
	}
}

func (s *AutomationScheduler) run(ctx context.Context) {
	defer close(s.stoppedChan)
	defer func() {
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
	}()

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	// Execute immediately on start
	s.processBatch(ctx)

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("Automation scheduler context cancelled")
			return
		case <-s.stopChan:
			s.logger.Info("Automation scheduler received stop signal")
			return
		case <-ticker.C:
			s.processBatch(ctx)
		}
	}
}

func (s *AutomationScheduler) processBatch(ctx context.Context) {
	startTime := time.Now()

	processed, err := s.executor.ProcessBatch(ctx, s.batchSize)
	elapsed := time.Since(startTime)

	if err != nil {
		s.logger.WithField("error", err.Error()).
			WithField("elapsed", elapsed).
			Error("Failed to process automation batch")
	} else if processed > 0 {
		s.logger.WithField("processed", processed).
			WithField("elapsed", elapsed).
			Info("Processed automation batch")
	}
}

// IsRunning returns whether the scheduler is currently running
func (s *AutomationScheduler) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}
