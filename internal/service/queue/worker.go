package queue

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// EmailQueueWorkerConfig holds configuration for the worker pool
type EmailQueueWorkerConfig struct {
	WorkerCount  int           // Number of concurrent workers per workspace (default: 5)
	PollInterval time.Duration // How often to poll for new work (default: 1s)
	BatchSize    int           // How many emails to fetch per poll (default: 50)
	MaxRetries   int           // Max retry attempts before dead letter (default: 3)
}

// DefaultWorkerConfig returns sensible default configuration
func DefaultWorkerConfig() *EmailQueueWorkerConfig {
	return &EmailQueueWorkerConfig{
		WorkerCount:  5,
		PollInterval: 1 * time.Second,
		BatchSize:    50,
		MaxRetries:   3,
	}
}

// EmailSentCallback is called when an email is successfully sent
type EmailSentCallback func(workspaceID string, sourceType domain.EmailQueueSourceType, sourceID string, messageID string)

// EmailFailedCallback is called when an email fails to send
type EmailFailedCallback func(workspaceID string, sourceType domain.EmailQueueSourceType, sourceID string, messageID string, err error, isDeadLetter bool)

// EmailQueueWorker processes queued emails
type EmailQueueWorker struct {
	queueRepo     domain.EmailQueueRepository
	workspaceRepo domain.WorkspaceRepository
	emailService  domain.EmailServiceInterface
	rateLimiter   *IntegrationRateLimiter
	config        *EmailQueueWorkerConfig
	logger        logger.Logger

	// Control
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	running bool
	mu      sync.RWMutex

	// Callbacks for progress tracking
	onEmailSent   EmailSentCallback
	onEmailFailed EmailFailedCallback
}

// NewEmailQueueWorker creates a new EmailQueueWorker
func NewEmailQueueWorker(
	queueRepo domain.EmailQueueRepository,
	workspaceRepo domain.WorkspaceRepository,
	emailService domain.EmailServiceInterface,
	config *EmailQueueWorkerConfig,
	log logger.Logger,
) *EmailQueueWorker {
	if config == nil {
		config = DefaultWorkerConfig()
	}

	return &EmailQueueWorker{
		queueRepo:     queueRepo,
		workspaceRepo: workspaceRepo,
		emailService:  emailService,
		rateLimiter:   NewIntegrationRateLimiter(),
		config:        config,
		logger:        log,
	}
}

// SetCallbacks sets callback functions for progress tracking
func (w *EmailQueueWorker) SetCallbacks(onSent EmailSentCallback, onFailed EmailFailedCallback) {
	w.onEmailSent = onSent
	w.onEmailFailed = onFailed
}

// Start begins processing queued emails
func (w *EmailQueueWorker) Start(ctx context.Context) error {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return nil
	}
	w.ctx, w.cancel = context.WithCancel(ctx)
	w.running = true
	w.mu.Unlock()

	w.logger.WithFields(map[string]interface{}{
		"worker_count":  w.config.WorkerCount,
		"poll_interval": w.config.PollInterval.String(),
		"batch_size":    w.config.BatchSize,
	}).Info("Starting email queue worker")

	// Start the main processing loop
	w.wg.Add(1)
	go w.processLoop()

	return nil
}

// Stop gracefully stops all workers
func (w *EmailQueueWorker) Stop() {
	w.mu.Lock()
	if !w.running {
		w.mu.Unlock()
		return
	}
	w.running = false
	w.cancel()
	w.mu.Unlock()

	w.logger.Info("Stopping email queue worker...")
	w.wg.Wait()
	w.logger.Info("Email queue worker stopped")
}

// IsRunning returns whether the worker is currently running
func (w *EmailQueueWorker) IsRunning() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.running
}

// processLoop is the main processing loop that polls for work
func (w *EmailQueueWorker) processLoop() {
	defer w.wg.Done()

	ticker := time.NewTicker(w.config.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-w.ctx.Done():
			return
		case <-ticker.C:
			w.processAllWorkspaces()
		}
	}
}

// processAllWorkspaces processes pending emails from all workspaces
func (w *EmailQueueWorker) processAllWorkspaces() {
	// Get list of all workspaces
	workspaces, err := w.workspaceRepo.List(w.ctx)
	if err != nil {
		w.logger.WithField("error", err.Error()).Error("Failed to list workspaces")
		return
	}

	// Process each workspace concurrently
	var processWg sync.WaitGroup
	semaphore := make(chan struct{}, w.config.WorkerCount)

	for _, workspace := range workspaces {
		select {
		case <-w.ctx.Done():
			return
		default:
		}

		semaphore <- struct{}{}
		processWg.Add(1)

		go func(ws *domain.Workspace) {
			defer processWg.Done()
			defer func() { <-semaphore }()

			w.processWorkspace(ws)
		}(workspace)
	}

	processWg.Wait()
}

// processWorkspace processes pending emails for a single workspace
func (w *EmailQueueWorker) processWorkspace(workspace *domain.Workspace) {
	// Fetch pending emails
	entries, err := w.queueRepo.FetchPending(w.ctx, workspace.ID, w.config.BatchSize)
	if err != nil {
		w.logger.WithFields(map[string]interface{}{
			"workspace_id": workspace.ID,
			"error":        err.Error(),
		}).Error("Failed to fetch pending emails")
		return
	}

	if len(entries) == 0 {
		return
	}

	w.logger.WithFields(map[string]interface{}{
		"workspace_id": workspace.ID,
		"count":        len(entries),
	}).Debug("Processing queued emails")

	// Process each entry
	for _, entry := range entries {
		select {
		case <-w.ctx.Done():
			return
		default:
		}

		w.processEntry(workspace, entry)
	}
}

// processEntry processes a single queue entry
func (w *EmailQueueWorker) processEntry(workspace *domain.Workspace, entry *domain.EmailQueueEntry) {
	// Mark as processing
	if err := w.queueRepo.MarkAsProcessing(w.ctx, workspace.ID, entry.ID); err != nil {
		w.logger.WithFields(map[string]interface{}{
			"entry_id": entry.ID,
			"error":    err.Error(),
		}).Warn("Failed to mark entry as processing, may be processed by another worker")
		return
	}

	// Get the integration to retrieve the email provider
	integration := workspace.GetIntegrationByID(entry.IntegrationID)
	if integration == nil {
		w.handleError(workspace.ID, entry, fmt.Errorf("integration not found: %s", entry.IntegrationID))
		return
	}

	// Wait for rate limiter
	ratePerMinute := entry.Payload.RateLimitPerMinute
	if ratePerMinute <= 0 {
		ratePerMinute = integration.EmailProvider.RateLimitPerMinute
	}
	if ratePerMinute <= 0 {
		ratePerMinute = 60 // Default to 1 per second if not configured
	}

	if err := w.rateLimiter.Wait(w.ctx, entry.IntegrationID, ratePerMinute); err != nil {
		// Context cancelled, don't mark as failed
		w.logger.WithFields(map[string]interface{}{
			"entry_id": entry.ID,
			"error":    err.Error(),
		}).Debug("Rate limit wait cancelled")
		return
	}

	// Build the send request
	request := entry.Payload.ToSendEmailProviderRequest(
		workspace.ID,
		entry.IntegrationID,
		entry.MessageID,
		entry.ContactEmail,
		&integration.EmailProvider,
	)

	// Send the email
	err := w.emailService.SendEmail(w.ctx, *request, true) // isMarketing = true
	if err != nil {
		w.handleError(workspace.ID, entry, err)
		return
	}

	// Mark as sent
	if err := w.queueRepo.MarkAsSent(w.ctx, workspace.ID, entry.ID); err != nil {
		w.logger.WithFields(map[string]interface{}{
			"entry_id": entry.ID,
			"error":    err.Error(),
		}).Error("Failed to mark email as sent")
		return
	}

	w.logger.WithFields(map[string]interface{}{
		"entry_id":     entry.ID,
		"message_id":   entry.MessageID,
		"recipient":    entry.ContactEmail,
		"source_type":  entry.SourceType,
		"source_id":    entry.SourceID,
		"workspace_id": workspace.ID,
	}).Debug("Email sent successfully")

	// Call success callback
	if w.onEmailSent != nil {
		w.onEmailSent(workspace.ID, entry.SourceType, entry.SourceID, entry.MessageID)
	}
}

// handleError handles a send error, scheduling retry or moving to dead letter
func (w *EmailQueueWorker) handleError(workspaceID string, entry *domain.EmailQueueEntry, sendErr error) {
	entry.Attempts++ // Increment since MarkAsProcessing already did this

	w.logger.WithFields(map[string]interface{}{
		"entry_id":     entry.ID,
		"message_id":   entry.MessageID,
		"recipient":    entry.ContactEmail,
		"attempts":     entry.Attempts,
		"max_attempts": entry.MaxAttempts,
		"error":        sendErr.Error(),
	}).Warn("Failed to send email")

	if entry.Attempts >= entry.MaxAttempts {
		// Move to dead letter queue
		if err := w.queueRepo.MoveToDeadLetter(w.ctx, workspaceID, entry, sendErr.Error()); err != nil {
			w.logger.WithFields(map[string]interface{}{
				"entry_id": entry.ID,
				"error":    err.Error(),
			}).Error("Failed to move to dead letter queue")
		}

		// Call failure callback
		if w.onEmailFailed != nil {
			w.onEmailFailed(workspaceID, entry.SourceType, entry.SourceID, entry.MessageID, sendErr, true)
		}
		return
	}

	// Schedule retry with exponential backoff
	nextRetry := domain.CalculateNextRetryTime(entry.Attempts)
	if err := w.queueRepo.MarkAsFailed(w.ctx, workspaceID, entry.ID, sendErr.Error(), &nextRetry); err != nil {
		w.logger.WithFields(map[string]interface{}{
			"entry_id": entry.ID,
			"error":    err.Error(),
		}).Error("Failed to mark as failed for retry")
	}

	// Call failure callback
	if w.onEmailFailed != nil {
		w.onEmailFailed(workspaceID, entry.SourceType, entry.SourceID, entry.MessageID, sendErr, false)
	}
}

// GetStats returns statistics about the rate limiters
func (w *EmailQueueWorker) GetStats() map[string]RateLimiterStats {
	return w.rateLimiter.GetStats()
}

// GetConfig returns the worker configuration
func (w *EmailQueueWorker) GetConfig() *EmailQueueWorkerConfig {
	return w.config
}
