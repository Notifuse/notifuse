package queue

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/emailerror"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// EmailQueueWorkerConfig holds configuration for the worker pool
type EmailQueueWorkerConfig struct {
	WorkerCount  int           // Number of concurrent workers per workspace (default: 5)
	PollInterval time.Duration // How often to poll for new work (default: 1s)
	BatchSize    int           // How many emails to fetch per poll (default: 50)
	MaxRetries   int           // Max retry attempts before permanent failure (default: 3)

	// Circuit breaker settings
	CircuitBreakerThreshold int           // Provider errors before opening circuit (default: 5)
	CircuitBreakerCooldown  time.Duration // Time before auto-reset attempt (default: 1 minute)
}

// DefaultWorkerConfig returns sensible default configuration
func DefaultWorkerConfig() *EmailQueueWorkerConfig {
	return &EmailQueueWorkerConfig{
		WorkerCount:             5,
		PollInterval:            1 * time.Second,
		BatchSize:               50,
		MaxRetries:              3,
		CircuitBreakerThreshold: 5,
		CircuitBreakerCooldown:  getCircuitBreakerCooldown(),
	}
}

// EmailSentCallback is called when an email is successfully sent
type EmailSentCallback func(workspaceID string, sourceType domain.EmailQueueSourceType, sourceID string, messageID string)

// EmailFailedCallback is called when an email fails to send
type EmailFailedCallback func(workspaceID string, sourceType domain.EmailQueueSourceType, sourceID string, messageID string, err error, isPermanent bool)

// EmailQueueWorker processes queued emails
type EmailQueueWorker struct {
	queueRepo          domain.EmailQueueRepository
	workspaceRepo      domain.WorkspaceRepository
	emailService       domain.EmailServiceInterface
	messageHistoryRepo domain.MessageHistoryRepository
	rateLimiter        *IntegrationRateLimiter
	circuitBreaker     *IntegrationCircuitBreaker
	errorClassifier    *emailerror.Classifier
	config             *EmailQueueWorkerConfig
	logger             logger.Logger

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
	messageHistoryRepo domain.MessageHistoryRepository,
	config *EmailQueueWorkerConfig,
	log logger.Logger,
) *EmailQueueWorker {
	if config == nil {
		config = DefaultWorkerConfig()
	}

	// Setup circuit breaker config with defaults
	cbConfig := CircuitBreakerConfig{
		Threshold:      config.CircuitBreakerThreshold,
		CooldownPeriod: config.CircuitBreakerCooldown,
	}
	if cbConfig.Threshold == 0 {
		cbConfig.Threshold = 5
	}
	if cbConfig.CooldownPeriod == 0 {
		cbConfig.CooldownPeriod = getCircuitBreakerCooldown()
	}

	return &EmailQueueWorker{
		queueRepo:          queueRepo,
		workspaceRepo:      workspaceRepo,
		emailService:       emailService,
		messageHistoryRepo: messageHistoryRepo,
		rateLimiter:        NewIntegrationRateLimiter(),
		circuitBreaker:     NewIntegrationCircuitBreaker(cbConfig),
		errorClassifier:    emailerror.NewClassifier(),
		config:             config,
		logger:             log,
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
	// Calculate dynamic batch size based on rate limit
	// Use 45 seconds as time budget (leave 15s buffer for shutdown)
	minRate := w.getMinEmailRateLimit(workspace)
	effectiveBatchSize := (minRate * 45) / 60 // 75% of what we can send in 1 minute
	if effectiveBatchSize < 1 {
		effectiveBatchSize = 1
	}
	if effectiveBatchSize > w.config.BatchSize {
		effectiveBatchSize = w.config.BatchSize
	}

	// Fetch pending emails
	entries, err := w.queueRepo.FetchPending(w.ctx, workspace.ID, effectiveBatchSize)
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
	// Get the integration to retrieve the email provider (needed for circuit breaker check)
	integration := workspace.GetIntegrationByID(entry.IntegrationID)
	if integration == nil {
		// Mark as processing first to increment attempts, then handle error
		if err := w.queueRepo.MarkAsProcessing(w.ctx, workspace.ID, entry.ID); err != nil {
			w.logger.WithFields(map[string]interface{}{
				"entry_id": entry.ID,
				"error":    err.Error(),
			}).Warn("Failed to mark entry as processing")
			return
		}
		w.handleError(workspace, entry, fmt.Errorf("integration not found: %s", entry.IntegrationID), nil)
		return
	}

	// Check circuit breaker BEFORE MarkAsProcessing to avoid incrementing attempts
	if w.circuitBreaker.IsOpen(entry.IntegrationID) {
		w.logger.WithFields(map[string]interface{}{
			"entry_id":       entry.ID,
			"integration_id": entry.IntegrationID,
		}).Debug("Circuit breaker open, scheduling retry without incrementing attempts")

		// Schedule for retry after cooldown WITHOUT incrementing attempts
		nextRetry := time.Now().Add(w.circuitBreaker.GetConfig().CooldownPeriod)
		if err := w.queueRepo.SetNextRetry(w.ctx, workspace.ID, entry.ID, nextRetry); err != nil {
			w.logger.WithFields(map[string]interface{}{
				"entry_id": entry.ID,
				"error":    err.Error(),
			}).Warn("Failed to set next retry for circuit breaker skip")
		}
		return
	}

	// Mark as processing (this increments attempts)
	if err := w.queueRepo.MarkAsProcessing(w.ctx, workspace.ID, entry.ID); err != nil {
		w.logger.WithFields(map[string]interface{}{
			"entry_id": entry.ID,
			"error":    err.Error(),
		}).Warn("Failed to mark entry as processing, may be processed by another worker")
		return
	}

	// Wait for rate limiter - always use current integration rate limit (not stale payload value)
	ratePerMinute := integration.EmailProvider.RateLimitPerMinute
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
		// Classify the error
		classifiedErr := w.errorClassifier.Classify(err, integration.EmailProvider.Kind)

		// Log the classification for debugging
		w.logger.WithFields(map[string]interface{}{
			"entry_id":    entry.ID,
			"error_type":  classifiedErr.Type,
			"provider":    classifiedErr.Provider,
			"http_status": classifiedErr.HTTPStatus,
			"retryable":   classifiedErr.Retryable,
			"original":    err.Error(),
		}).Debug("Classified send error")

		// Record failure to circuit breaker (only counts provider errors)
		w.circuitBreaker.RecordFailure(entry.IntegrationID, classifiedErr)

		w.handleError(workspace, entry, err, classifiedErr)
		return
	}

	// Record success to reset circuit breaker
	w.circuitBreaker.RecordSuccess(entry.IntegrationID)

	// Mark as sent
	if err := w.queueRepo.MarkAsSent(w.ctx, workspace.ID, entry.ID); err != nil {
		w.logger.WithFields(map[string]interface{}{
			"entry_id": entry.ID,
			"error":    err.Error(),
		}).Error("Failed to mark email as sent")
		return
	}

	// Upsert message history (success - clears any previous failure)
	w.upsertMessageHistory(w.ctx, workspace.ID, workspace.Settings.SecretKey, entry, nil)

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

// handleError handles a send error, scheduling retry or deleting permanently failed entries
// classifiedErr may be nil for internal errors (e.g., integration not found)
func (w *EmailQueueWorker) handleError(workspace *domain.Workspace, entry *domain.EmailQueueEntry, sendErr error, classifiedErr *emailerror.ClassifiedError) {
	entry.Attempts++ // Increment since MarkAsProcessing already did this

	// Determine if this is a permanent failure (non-retryable recipient error or max attempts)
	isPermanent := entry.Attempts >= entry.MaxAttempts
	if classifiedErr != nil && !classifiedErr.Retryable {
		isPermanent = true
	}

	logFields := map[string]interface{}{
		"entry_id":     entry.ID,
		"message_id":   entry.MessageID,
		"recipient":    entry.ContactEmail,
		"attempts":     entry.Attempts,
		"max_attempts": entry.MaxAttempts,
		"error":        sendErr.Error(),
		"is_permanent": isPermanent,
	}
	if classifiedErr != nil {
		logFields["error_type"] = classifiedErr.Type
	}
	w.logger.WithFields(logFields).Warn("Failed to send email")

	// Upsert message history with failure info
	w.upsertMessageHistory(w.ctx, workspace.ID, workspace.Settings.SecretKey, entry, sendErr)

	if isPermanent {
		// Permanent failure - delete the queue entry
		// Message history already tracks this permanent failure via upsertMessageHistory above
		w.logger.WithFields(map[string]interface{}{
			"entry_id":   entry.ID,
			"message_id": entry.MessageID,
			"attempts":   entry.Attempts,
		}).Warn("Email permanently failed")

		if err := w.queueRepo.Delete(w.ctx, workspace.ID, entry.ID); err != nil {
			w.logger.WithFields(map[string]interface{}{
				"entry_id": entry.ID,
				"error":    err.Error(),
			}).Error("Failed to delete permanently failed queue entry")
		}

		// Call failure callback (isPermanent = true)
		if w.onEmailFailed != nil {
			w.onEmailFailed(workspace.ID, entry.SourceType, entry.SourceID, entry.MessageID, sendErr, true)
		}
		return
	}

	// Schedule retry with exponential backoff
	nextRetry := domain.CalculateNextRetryTime(entry.Attempts)
	if err := w.queueRepo.MarkAsFailed(w.ctx, workspace.ID, entry.ID, sendErr.Error(), &nextRetry); err != nil {
		w.logger.WithFields(map[string]interface{}{
			"entry_id": entry.ID,
			"error":    err.Error(),
		}).Error("Failed to mark as failed for retry")
	}

	// Call failure callback (isPermanent = false, will retry)
	if w.onEmailFailed != nil {
		w.onEmailFailed(workspace.ID, entry.SourceType, entry.SourceID, entry.MessageID, sendErr, false)
	}
}

// upsertMessageHistory creates or updates a message history record after a send attempt
// On success: FailedAt and StatusInfo are nil (clears any previous failure)
// On failure: FailedAt is set to now, StatusInfo contains the error
func (w *EmailQueueWorker) upsertMessageHistory(
	ctx context.Context,
	workspaceID string,
	secretKey string,
	entry *domain.EmailQueueEntry,
	sendErr error,
) {
	now := time.Now().UTC()

	message := &domain.MessageHistory{
		ID:              entry.MessageID,
		ContactEmail:    entry.ContactEmail,
		TemplateID:      entry.TemplateID,
		TemplateVersion: int64(entry.Payload.TemplateVersion),
		Channel:         "email",
		MessageData:     domain.MessageData{Data: entry.Payload.TemplateData}, // Include template data for logging
		SentAt:          entry.CreatedAt,                                      // Use queue entry creation time (stable across retries)
		CreatedAt:       entry.CreatedAt,
		UpdatedAt:       now,
	}

	// Set source (broadcast or automation)
	if entry.SourceType == domain.EmailQueueSourceBroadcast {
		message.BroadcastID = &entry.SourceID
		if entry.Payload.ListID != "" {
			message.ListID = &entry.Payload.ListID
		}
	} else if entry.SourceType == domain.EmailQueueSourceAutomation {
		message.AutomationID = &entry.SourceID
	}

	// Set failure info if send failed (will be cleared on retry success via UPSERT)
	if sendErr != nil {
		message.FailedAt = &now
		errStr := sendErr.Error()
		if len(errStr) > 255 {
			errStr = errStr[:255]
		}
		message.StatusInfo = &errStr
	}
	// On success: FailedAt and StatusInfo remain nil, clearing any previous failure

	// Upsert record (log errors but don't fail the send operation)
	if err := w.messageHistoryRepo.Upsert(ctx, workspaceID, secretKey, message); err != nil {
		w.logger.WithFields(map[string]interface{}{
			"entry_id":   entry.ID,
			"message_id": entry.MessageID,
			"error":      err.Error(),
		}).Warn("Failed to upsert message history")
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

// GetCircuitBreakerStats returns statistics about all circuit breakers
func (w *EmailQueueWorker) GetCircuitBreakerStats() map[string]CircuitBreakerStats {
	return w.circuitBreaker.GetStats()
}

// getMinEmailRateLimit returns the minimum rate limit across all email integrations
// Returns default of 60 if no email integrations found
func (w *EmailQueueWorker) getMinEmailRateLimit(workspace *domain.Workspace) int {
	emailIntegrations := workspace.GetIntegrationsByType(domain.IntegrationTypeEmail)
	if len(emailIntegrations) == 0 {
		return 60 // Default: 1 per second
	}

	minRate := emailIntegrations[0].EmailProvider.RateLimitPerMinute
	for _, integration := range emailIntegrations[1:] {
		if integration.EmailProvider.RateLimitPerMinute < minRate {
			minRate = integration.EmailProvider.RateLimitPerMinute
		}
	}
	return minRate
}
