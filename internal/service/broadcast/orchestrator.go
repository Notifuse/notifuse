package broadcast

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
)

//go:generate mockgen -destination=./mocks/mock_broadcast_orchestrator.go -package=mocks github.com/Notifuse/notifuse/internal/service/broadcast BroadcastOrchestratorInterface

// BroadcastOrchestratorInterface defines the interface for broadcast orchestration
type BroadcastOrchestratorInterface interface {
	// CanProcess returns true if this processor can handle the given task type
	CanProcess(taskType string) bool

	// Process executes or continues a broadcast sending task
	Process(ctx context.Context, task *domain.Task, timeoutAt time.Time) (bool, error)

	// LoadTemplates loads all templates for a broadcast's variations
	LoadTemplates(ctx context.Context, workspaceID string, templateIDs []string) (map[string]*domain.Template, error)

	// ValidateTemplates validates that the required templates are loaded and valid
	ValidateTemplates(templates map[string]*domain.Template) error

	// GetTotalRecipientCount gets the total number of recipients for a broadcast
	GetTotalRecipientCount(ctx context.Context, workspaceID, broadcastID string) (int, error)

	// FetchBatch retrieves a batch of recipients for a broadcast using cursor-based pagination
	// afterEmail is the last email from the previous batch (empty for first batch)
	FetchBatch(ctx context.Context, workspaceID, broadcastID, afterEmail string, limit int) ([]*domain.ContactWithList, error)

	// SaveProgressState saves the current task progress to the repository
	SaveProgressState(ctx context.Context, workspaceID, taskID string, broadcastState *domain.SendBroadcastState, sentCount, failedCount, processedCount int, lastSaveTime time.Time, startTime time.Time) (time.Time, error)
}

// BroadcastOrchestrator is the main processor for sending broadcasts
type BroadcastOrchestrator struct {
	messageSender   MessageSender
	broadcastRepo   domain.BroadcastRepository
	templateRepo    domain.TemplateRepository
	contactRepo     domain.ContactRepository
	taskRepo        domain.TaskRepository
	workspaceRepo   domain.WorkspaceRepository
	emailQueueRepo  domain.EmailQueueRepository
	abTestEvaluator *ABTestEvaluator
	logger          logger.Logger
	config          *Config
	timeProvider    TimeProvider
	apiEndpoint     string
	eventBus        domain.EventBus
}

// NewBroadcastOrchestrator creates a new broadcast orchestrator
func NewBroadcastOrchestrator(
	messageSender MessageSender,
	broadcastRepo domain.BroadcastRepository,
	templateRepo domain.TemplateRepository,
	contactRepo domain.ContactRepository,
	taskRepo domain.TaskRepository,
	workspaceRepo domain.WorkspaceRepository,
	emailQueueRepo domain.EmailQueueRepository,
	abTestEvaluator *ABTestEvaluator,
	logger logger.Logger,
	config *Config,
	timeProvider TimeProvider,
	apiEndpoint string,
	eventBus domain.EventBus,
) BroadcastOrchestratorInterface {
	if config == nil {
		config = DefaultConfig()
	}

	if timeProvider == nil {
		timeProvider = NewRealTimeProvider()
	}

	return &BroadcastOrchestrator{
		messageSender:   messageSender,
		broadcastRepo:   broadcastRepo,
		templateRepo:    templateRepo,
		contactRepo:     contactRepo,
		taskRepo:        taskRepo,
		workspaceRepo:   workspaceRepo,
		emailQueueRepo:  emailQueueRepo,
		abTestEvaluator: abTestEvaluator,
		logger:          logger,
		config:          config,
		timeProvider:    timeProvider,
		apiEndpoint:     apiEndpoint,
		eventBus:        eventBus,
	}
}

// CanProcess returns true if this processor can handle the given task type
func (o *BroadcastOrchestrator) CanProcess(taskType string) bool {
	return taskType == "send_broadcast"
}

// pauseQueueEntries pauses any pending/failed queue rows for the given broadcast.
// Nil-safe so tests without a queue repo continue to work.
func (o *BroadcastOrchestrator) pauseQueueEntries(ctx context.Context, workspaceID, broadcastID string) {
	if o.emailQueueRepo == nil {
		return
	}
	n, err := o.emailQueueRepo.PauseBySource(ctx, workspaceID, domain.EmailQueueSourceBroadcast, broadcastID)
	if err != nil {
		o.logger.WithFields(map[string]interface{}{
			"broadcast_id": broadcastID,
			"error":        err.Error(),
		}).Warn("Failed to pause email queue entries")
		return
	}
	if n > 0 {
		o.logger.WithFields(map[string]interface{}{
			"broadcast_id":       broadcastID,
			"paused_queue_count": n,
		}).Info("Paused email queue entries for broadcast")
	}
}

// settleQueuedAfterLostTask deals with the emails a run queued before it found its task gone:
// a cancelled broadcast's are deleted and a paused one's paused, as the cancel or pause did to
// those already queued. Any other status leaves them to be sent. The broadcast is read with its
// lock: a cancel marks the task failed before its own transaction commits, so an unlocked read
// could still see the broadcast sending; the locked read waits for that commit.
func (o *BroadcastOrchestrator) settleQueuedAfterLostTask(ctx context.Context, workspaceID, broadcastID string) {
	writeCtx, cancel := leaseContext(ctx)
	defer cancel()
	err := o.broadcastRepo.WithTransaction(writeCtx, workspaceID, func(tx *sql.Tx) error {
		broadcast, err := o.broadcastRepo.GetBroadcastForUpdateTx(writeCtx, tx, workspaceID, broadcastID)
		if err != nil {
			return err
		}
		switch broadcast.Status {
		case domain.BroadcastStatusCancelled:
			return o.settleQueueEntriesTx(writeCtx, tx, broadcastID, o.emailQueueDeleteTx, "cancelled_queue_count")
		case domain.BroadcastStatusPaused:
			return o.settleQueueEntriesTx(writeCtx, tx, broadcastID, o.emailQueuePauseTx, "paused_queue_count")
		}
		return nil
	})
	if err != nil {
		o.logger.WithFields(map[string]interface{}{
			"broadcast_id": broadcastID,
			"error":        err.Error(),
		}).Warn("Failed to settle the queued emails after losing the task")
	}
}

// deleteQueueEntries removes pending/failed/paused queue rows for the given broadcast.
// Nil-safe. Used when the orchestrator observes an external cancellation.
func (o *BroadcastOrchestrator) deleteQueueEntries(ctx context.Context, workspaceID, broadcastID string) {
	if o.emailQueueRepo == nil {
		return
	}
	n, err := o.emailQueueRepo.DeleteBySource(ctx, workspaceID, domain.EmailQueueSourceBroadcast, broadcastID)
	if err != nil {
		o.logger.WithFields(map[string]interface{}{
			"broadcast_id": broadcastID,
			"error":        err.Error(),
		}).Warn("Failed to delete email queue entries")
		return
	}
	if n > 0 {
		o.logger.WithFields(map[string]interface{}{
			"broadcast_id":          broadcastID,
			"cancelled_queue_count": n,
		}).Info("Deleted email queue entries for broadcast")
	}
}

// LoadTemplates loads all templates for a broadcast's variations
func (o *BroadcastOrchestrator) LoadTemplates(ctx context.Context, workspaceID string, templateIDs []string) (map[string]*domain.Template, error) {

	// Load all templates
	templates := make(map[string]*domain.Template)
	for _, templateID := range templateIDs {
		template, err := o.templateRepo.GetTemplateByID(ctx, workspaceID, templateID, 0) // Always use version 0
		if err != nil {
			// codecov:ignore:start
			o.logger.WithFields(map[string]interface{}{
				"workspace_id": workspaceID,
				"template_id":  templateID,
				"error":        err.Error(),
			}).Error("Failed to load template for broadcast")
			// codecov:ignore:end
			continue // Don't fail the whole broadcast for one template
		}
		templates[templateID] = template
	}

	// Validate that we found at least one template
	if len(templates) == 0 {
		// codecov:ignore:start
		o.logger.WithFields(map[string]interface{}{
			"workspace_id": workspaceID,
		}).Error("No valid templates found for broadcast")
		// codecov:ignore:end
		return nil, NewBroadcastError(ErrCodeTemplateMissing, "no valid templates found for broadcast", false, nil)
	}

	return templates, nil
}

// winnerTemplates returns the templates a winner-phase batch is sent with: the
// winning template alone. A run that loaded every variation for the test phase
// has it already, unless it failed to load then; it is loaded and validated here
// in that case, and kept in loaded for the batches that follow.
func (o *BroadcastOrchestrator) winnerTemplates(ctx context.Context, workspaceID string, broadcast *domain.Broadcast, loaded map[string]*domain.Template) (map[string]*domain.Template, error) {
	if broadcast.WinningTemplate == nil {
		return nil, fmt.Errorf("winner phase but no winning template selected")
	}
	winnerID := *broadcast.WinningTemplate
	if template, ok := loaded[winnerID]; ok {
		return map[string]*domain.Template{winnerID: template}, nil
	}

	templates, err := o.LoadTemplates(ctx, workspaceID, []string{winnerID})
	if err != nil {
		return nil, err
	}
	if err := o.ValidateTemplates(templates); err != nil {
		return nil, err
	}
	loaded[winnerID] = templates[winnerID]
	return templates, nil
}

// pauseForSendError pauses a broadcast whose send cannot go on until the user acts (a
// data feed that failed, a chosen sender that has left the integration), pauses its
// queued rows and announces the pause. Paused, unlike failed, can be edited and resumed.
func (o *BroadcastOrchestrator) pauseForSendError(ctx context.Context, task *domain.Task, broadcastID string, offset int64, cause error) {
	// The senders wrap the feed's error as "<ErrBroadcastShouldPause>: ...": the user reads
	// only what follows the internal sentinel.
	reason := "Recipient feed failed: " + strings.TrimPrefix(cause.Error(), ErrBroadcastShouldPause.Error()+": ")
	eventReason := "recipient_feed_failed"
	var senderErr *SenderUnavailableError
	if errors.As(cause, &senderErr) {
		reason = senderErr.PauseReason()
		eventReason = "sender_unavailable"
	}

	o.logger.WithFields(map[string]interface{}{
		"task_id":      task.ID,
		"broadcast_id": broadcastID,
		"offset":       offset,
		"reason":       eventReason,
		"error":        cause.Error(),
	}).Warn("Pausing broadcast")

	currentBroadcast, getBroadcastErr := o.broadcastRepo.GetBroadcast(ctx, task.WorkspaceID, broadcastID)
	if getBroadcastErr != nil || currentBroadcast == nil {
		return
	}
	currentBroadcast.Status = domain.BroadcastStatusPaused
	currentBroadcast.PausedAt = &[]time.Time{time.Now().UTC()}[0]
	currentBroadcast.PauseReason = &reason
	currentBroadcast.UpdatedAt = time.Now().UTC()

	if updateErr := o.broadcastRepo.UpdateBroadcast(ctx, currentBroadcast); updateErr != nil {
		return
	}
	// Pause already-queued rows so the worker stops sending.
	o.pauseQueueEntries(ctx, task.WorkspaceID, broadcastID)

	if o.eventBus != nil {
		o.eventBus.Publish(context.Background(), domain.EventPayload{
			Type:        domain.EventBroadcastPaused,
			WorkspaceID: task.WorkspaceID,
			EntityID:    broadcastID,
			Data: map[string]interface{}{
				"broadcast_id": broadcastID,
				"task_id":      task.ID,
				"reason":       eventReason,
			},
		})
	}
}

// SenderUnavailableError reports a variation whose chosen sender is no longer one of the
// marketing email integration's. It matches both ErrBroadcastShouldPause and
// domain.ErrSenderNotFound.
type SenderUnavailableError struct {
	VariationName string
	// VariationNumber counts the broadcast's variations from 1; VariationCount is how
	// many it has. They name the variation when it has no name, as the console's do not.
	VariationNumber int
	VariationCount  int
	TemplateID      string
	SenderID        string
}

func (e *SenderUnavailableError) Error() string {
	return fmt.Sprintf("sender %s chosen for the variation sending template %s is not one of the marketing email integration's senders", e.SenderID, e.TemplateID)
}

func (e *SenderUnavailableError) Unwrap() []error {
	return []error{ErrBroadcastShouldPause, domain.ErrSenderNotFound}
}

// PauseReason is the reason shown on the paused broadcast.
func (e *SenderUnavailableError) PauseReason() string {
	variation := "this broadcast"
	switch {
	case e.VariationName != "":
		variation = fmt.Sprintf("variation “%s”", e.VariationName)
	case e.VariationCount > 1:
		variation = fmt.Sprintf("variation %d", e.VariationNumber)
	}
	return fmt.Sprintf("Sender unavailable: the sender chosen for %s (sender %s) is no longer one of the marketing email integration's senders. Choose another sender, then resume.", variation, e.SenderID)
}

// findUnavailableVariationSender returns a *SenderUnavailableError when a variation this run sends
// chooses a sender that is no longer one of the integration's.
func findUnavailableVariationSender(broadcast *domain.Broadcast, templates map[string]*domain.Template, provider *domain.EmailProvider) error {
	variations := broadcast.TestSettings.Variations
	for i, variation := range variations {
		if _, sends := templates[variation.TemplateID]; !sends {
			continue
		}
		if variation.SenderID == "" || provider.HasSender(variation.SenderID) {
			continue
		}
		return &SenderUnavailableError{
			VariationName:   variation.VariationName,
			VariationNumber: i + 1,
			VariationCount:  len(variations),
			TemplateID:      variation.TemplateID,
			SenderID:        variation.SenderID,
		}
	}
	return nil
}

// ValidateTemplates validates that the required templates are loaded and valid
func (o *BroadcastOrchestrator) ValidateTemplates(templates map[string]*domain.Template) error {
	if len(templates) == 0 {
		return NewBroadcastError(ErrCodeTemplateMissing, "no templates provided for validation", false, nil)
	}

	// Validate each template
	for id, template := range templates {
		if template == nil {
			return NewBroadcastError(ErrCodeTemplateInvalid, "template is nil", false, nil)
		}

		// Ensure the template has the required fields for sending emails
		if template.Email == nil {
			// codecov:ignore:start
			o.logger.WithField("template_id", id).Error("Template missing email configuration")
			// codecov:ignore:end
			return NewBroadcastError(ErrCodeTemplateInvalid, "template missing email configuration", false, nil)
		}

		if template.Email.Subject == "" {
			// codecov:ignore:start
			o.logger.WithField("template_id", id).Error("Template missing subject")
			// codecov:ignore:end
			return NewBroadcastError(ErrCodeTemplateInvalid, "template missing subject", false, nil)
		}

		// Code mode templates use MjmlSource; visual mode templates use VisualEditorTree
		if template.Email.EditorMode == domain.EditorModeCode {
			if template.Email.MjmlSource == nil || *template.Email.MjmlSource == "" {
				// codecov:ignore:start
				o.logger.WithField("template_id", id).Error("Code mode template missing mjml_source")
				// codecov:ignore:end
				return NewBroadcastError(ErrCodeTemplateInvalid, "template missing content", false, nil)
			}
		} else if template.Email.VisualEditorTree.GetType() == "" {
			// codecov:ignore:start
			o.logger.WithField("template_id", id).Error("Template missing content")
			// codecov:ignore:end
			return NewBroadcastError(ErrCodeTemplateInvalid, "template missing content", false, nil)
		}
	}

	return nil
}

// GetTotalRecipientCount gets the total number of recipients for a broadcast
func (o *BroadcastOrchestrator) GetTotalRecipientCount(ctx context.Context, workspaceID, broadcastID string) (int, error) {
	startTime := time.Now()
	defer func() {
		// codecov:ignore:start
		o.logger.WithFields(map[string]interface{}{
			"duration_ms":  time.Since(startTime).Milliseconds(),
			"broadcast_id": broadcastID,
			"workspace_id": workspaceID,
		}).Debug("Recipient count completed")
		// codecov:ignore:end
	}()

	broadcast, err := o.getBroadcastForCount(ctx, workspaceID, broadcastID)
	if err != nil {
		return 0, err
	}
	return o.countRecipients(ctx, workspaceID, broadcast)
}

// getBroadcastForCount reads the broadcast whose recipients are about to be counted.
func (o *BroadcastOrchestrator) getBroadcastForCount(ctx context.Context, workspaceID, broadcastID string) (*domain.Broadcast, error) {
	broadcast, err := o.broadcastRepo.GetBroadcast(ctx, workspaceID, broadcastID)
	if err != nil {
		// codecov:ignore:start
		o.logger.WithFields(map[string]interface{}{
			"broadcast_id": broadcastID,
			"workspace_id": workspaceID,
			"error":        err.Error(),
		}).Error("Failed to get broadcast for recipient count")
		// codecov:ignore:end
		if interrupted, ok := wrapIfInterrupted(err); ok {
			return nil, interrupted
		}
		return nil, NewBroadcastError(ErrCodeBroadcastNotFound, "broadcast not found", false, err)
	}
	return broadcast, nil
}

// countRecipients counts the recipients of the broadcast's audience.
func (o *BroadcastOrchestrator) countRecipients(ctx context.Context, workspaceID string, broadcast *domain.Broadcast) (int, error) {
	broadcastID := broadcast.ID
	count, err := o.contactRepo.CountContactsForBroadcast(ctx, workspaceID, broadcast.Audience)
	if err != nil {
		// codecov:ignore:start
		o.logger.WithFields(map[string]interface{}{
			"broadcast_id": broadcastID,
			"workspace_id": workspaceID,
			"error":        err.Error(),
		}).Error("Failed to count recipients for broadcast")
		// codecov:ignore:end
		return 0, NewBroadcastError(ErrCodeRecipientFetch, "failed to count recipients", true, err)
	}

	// codecov:ignore:start
	o.logger.WithFields(map[string]interface{}{
		"broadcast_id":      broadcastID,
		"workspace_id":      workspaceID,
		"recipient_count":   count,
		"audience_list":     broadcast.Audience.List,
		"audience_segments": len(broadcast.Audience.Segments),
	}).Info("Got recipient count for broadcast")
	// codecov:ignore:end

	return count, nil
}

// FetchBatch retrieves a batch of recipients for a broadcast using cursor-based pagination
// afterEmail is the last email from the previous batch (empty for first batch)
func (o *BroadcastOrchestrator) FetchBatch(ctx context.Context, workspaceID, broadcastID, afterEmail string, limit int) ([]*domain.ContactWithList, error) {
	startTime := time.Now()
	defer func() {
		// codecov:ignore:start
		o.logger.WithFields(map[string]interface{}{
			"duration_ms":  time.Since(startTime).Milliseconds(),
			"broadcast_id": broadcastID,
			"workspace_id": workspaceID,
			"after_email":  afterEmail,
			"limit":        limit,
		}).Debug("Recipient batch fetch completed")
		// codecov:ignore:end
	}()

	// Get the broadcast to access audience settings
	broadcast, err := o.broadcastRepo.GetBroadcast(ctx, workspaceID, broadcastID)
	if err != nil {
		// codecov:ignore:start
		o.logger.WithFields(map[string]interface{}{
			"broadcast_id": broadcastID,
			"workspace_id": workspaceID,
			"error":        err.Error(),
		}).Error("Failed to get broadcast for recipient fetch")
		// codecov:ignore:end
		if interrupted, ok := wrapIfInterrupted(err); ok {
			return nil, interrupted
		}
		return nil, NewBroadcastError(ErrCodeBroadcastNotFound, "broadcast not found", false, err)
	}

	// Check if broadcast is cancelled and return specific error
	if broadcast.Status == domain.BroadcastStatusCancelled {
		return nil, NewBroadcastError(ErrCodeBroadcastCancelled, "broadcast has been cancelled", false, nil)
	}

	// Apply the actual batch limit from config if not specified
	if limit <= 0 {
		limit = o.config.FetchBatchSize
	}

	// Fetch contacts based on broadcast audience using cursor-based pagination
	contactsWithList, err := o.contactRepo.GetContactsForBroadcast(ctx, workspaceID, broadcast.Audience, limit, afterEmail)
	if err != nil {
		// codecov:ignore:start
		o.logger.WithFields(map[string]interface{}{
			"broadcast_id": broadcastID,
			"workspace_id": workspaceID,
			"audience":     broadcast.Audience,
			"after_email":  afterEmail,
			"limit":        limit,
			"error":        err.Error(),
		}).Error("Failed to fetch recipients for broadcast")
		// codecov:ignore:end
		return nil, NewBroadcastError(ErrCodeRecipientFetch, "failed to fetch recipients", true, err)
	}

	// codecov:ignore:start
	o.logger.WithFields(map[string]interface{}{
		"broadcast_id":     broadcastID,
		"workspace_id":     workspaceID,
		"after_email":      afterEmail,
		"limit":            limit,
		"contacts_fetched": len(contactsWithList),
		"with_list_info":   true,
	}).Info("Fetched recipient batch with list info")

	return contactsWithList, nil
}

// FormatDuration formats a duration in a human-readable form
func FormatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	} else if d < time.Hour {
		m := int(d.Minutes())
		s := int(d.Seconds()) % 60
		return fmt.Sprintf("%dm %ds", m, s)
	} else {
		h := int(d.Hours())
		m := int(d.Minutes()) % 60
		return fmt.Sprintf("%dh %dm", h, m)
	}
}

// CalculateProgress calculates the progress percentage (0-100)
func CalculateProgress(processed, total int) float64 {
	if total <= 0 {
		return 100.0 // Avoid division by zero
	}

	progress := float64(processed) / float64(total) * 100.0
	if progress > 100.0 {
		progress = 100.0
	}
	return progress
}

// FormatProgressMessage creates a human-readable progress message
func FormatProgressMessage(processed, total int, elapsed time.Duration) string {
	progress := CalculateProgress(processed, total)

	// Calculate remaining time when in progress (not completed) and we have enough data
	var eta string
	if progress > 5.0 && processed > 0 && processed < total {
		estimatedTotalSeconds := elapsed.Seconds() * float64(total) / float64(processed)
		remainingSeconds := estimatedTotalSeconds - elapsed.Seconds()
		if remainingSeconds > 0 {
			eta = fmt.Sprintf(", ETA: %s", FormatDuration(time.Duration(remainingSeconds)*time.Second))
		}
	}

	return fmt.Sprintf("Processed %d/%d recipients (%.1f%%)%s",
		processed, total, progress, eta)
}

// leaseContext returns a context for a write the run must make even when its context is
// cancelled (a shutdown keeps the cursor of the last batch), still bounded by that context's
// deadline, which ExecuteTask sets to the run's lease: past it another execution may hold
// the task. It keeps the context's values, among them the lease SaveState matches
// (domain.WithTaskLease).
func leaseContext(ctx context.Context) (context.Context, context.CancelFunc) {
	detached := context.WithoutCancel(ctx)
	if deadline, ok := ctx.Deadline(); ok {
		return context.WithDeadline(detached, deadline)
	}
	return detached, func() {}
}

// SaveProgressState saves the current task progress to the repository
func (o *BroadcastOrchestrator) SaveProgressState(
	ctx context.Context,
	workspaceID, taskID string,
	broadcastState *domain.SendBroadcastState,
	sentCount, failedCount, processedCount int,
	lastSaveTime time.Time,
	startTime time.Time,
) (time.Time, error) {
	currentTime := o.timeProvider.Now()

	// Calculate progress
	elapsedSinceStart := currentTime.Sub(startTime)
	progress := CalculateProgress(processedCount, broadcastState.TotalRecipients)
	message := FormatProgressMessage(processedCount, broadcastState.TotalRecipients, elapsedSinceStart)

	// Create a copy of the broadcast state and update counters
	updatedBroadcastState := *broadcastState // Copy the struct
	updatedBroadcastState.EnqueuedCount = sentCount
	updatedBroadcastState.FailedCount = failedCount

	// Create state with preserved A/B testing fields
	state := &domain.TaskState{
		Progress:      progress,
		Message:       message,
		SendBroadcast: &updatedBroadcastState,
	}

	// Save state
	saveCtx, cancel := leaseContext(ctx)
	defer cancel()
	err := o.taskRepo.SaveState(saveCtx, workspaceID, taskID, progress, state)
	if err != nil {
		// codecov:ignore:start
		o.logger.WithFields(map[string]interface{}{
			"task_id":      taskID,
			"workspace_id": workspaceID,
			"error":        err.Error(),
		}).Error("Failed to save progress state")
		// codecov:ignore:end
		return lastSaveTime, NewBroadcastError(ErrCodeTaskStateInvalid, "failed to save task state", true, err)
	}

	// Log progress
	// codecov:ignore:start
	o.logger.WithFields(map[string]interface{}{
		"task_id":      taskID,
		"workspace_id": workspaceID,
		"progress":     progress,
		"sent":         sentCount,
		"failed":       failedCount,
	}).Debug("Saved progress state")

	return currentTime, nil
}

// statusesKeptOnLastAttempt are the broadcast statuses a failed last attempt
// leaves as they are. A cancel or a pause that lands during the run is only seen
// by the send loop on its next refresh, so a run that failed before then must
// not overwrite it: cancelled is the user's final word, and a paused broadcast is
// resumed with its task, failed or not, where a failed one is never resumed. A
// processed broadcast has finished sending. The list names the statuses to keep
// rather than the sending ones: a broadcast scheduled for later keeps
// "scheduled" until its task's first sending run (startScheduledSend), and one
// already sending when the upgrade ran keeps it until its next run.
var statusesKeptOnLastAttempt = []domain.BroadcastStatus{
	domain.BroadcastStatusCancelled,
	domain.BroadcastStatusPaused,
	domain.BroadcastStatusProcessed,
}

// stillLastAttempt reads the task back before a run that started as its last
// attempt finalizes the broadcast. A resume while the run held the task gave it
// a fresh retry budget (ResetRetryBudget), and the task service then retries the
// failure. The stored count is right in every other case too: an interrupted run
// is released without consuming the budget, and a run whose failure the task
// service recorded first started with none left, so the count is still past the
// limit. A failed read keeps the decision the run started with: a broadcast left
// sending under a task that failed for good would be stranded.
func (o *BroadcastOrchestrator) stillLastAttempt(ctx context.Context, task *domain.Task) bool {
	stored, err := o.taskRepo.Get(ctx, task.WorkspaceID, task.ID)
	if err != nil {
		o.logger.WithFields(map[string]interface{}{
			"task_id":      task.ID,
			"workspace_id": task.WorkspaceID,
			"error":        err.Error(),
		}).Warn("Failed to read the task back on its last attempt - finalizing the broadcast")
		return true
	}
	return !stored.HasRetriesLeft()
}

// Process executes or continues a broadcast sending task
// Its results are named so the deferred finalizer sees whatever error it
// returns: a return that left a local err unset used to skip it.
func (o *BroadcastOrchestrator) Process(ctx context.Context, task *domain.Task, timeoutAt time.Time) (allDone bool, err error) {
	o.logger.WithField("task_id", task.ID).Info("Processing send_broadcast task")

	// Store initial state for use in the defer function
	var broadcastID string
	// The task service retries a failed run while the task has retries left
	// (MarkAsFailedTx), so only a run that starts with none can be the last one.
	// Deciding one attempt earlier marked the broadcast failed while its retry
	// went on to send the rest of the list and flip it back to processed.
	isLastAttempt := !task.HasRetriesLeft()

	// Defer function to mark broadcast as failed if we're returning an error on the last attempt
	defer func() {
		if err != nil && isLastAttempt && broadcastID != "" {
			// A run handing its task back, or one that lost its lease, decides nothing
			// about the broadcast.
			var requeue *domain.TaskRequeueError
			if errors.As(err, &requeue) || errors.Is(err, domain.ErrTaskNotRunning) {
				return
			}

			// retryFinalization hands the task back for its retry interval when the outcome
			// could not be recorded and nothing was written, keeping its retry count: the
			// next run is the last attempt again and finalizes again. Failing the task here
			// left the broadcast in its sending status for good.
			runErr := err
			retryFinalization := func(cause error) {
				after := time.Duration(task.RetryInterval) * time.Second
				if after <= 0 {
					after = time.Minute
				}
				err = &domain.TaskRequeueError{
					After:  after,
					Reason: fmt.Sprintf("the last attempt's outcome could not be recorded (%v) after: %v", cause, runErr),
				}
			}

			// Skip if the broadcast was paused for a failed data feed or a chosen
			// sender that left the integration
			if errors.Is(err, ErrBroadcastShouldPause) {
				o.logger.WithFields(map[string]interface{}{
					"task_id":      task.ID,
					"broadcast_id": broadcastID,
					"error":        err.Error(),
				}).Info("Task failed on a pausing send error - broadcast already paused, not marking as failed")
				return
			}

			// An interruption is not the broadcast's fault, so it ends up paused
			// (with the reason on the row) rather than failed: paused is
			// resumable from the console and picks up at the saved cursor,
			// while failed is terminal. Already-enqueued rows are deliberately
			// left alone — those emails are committed and should keep going out.
			// Ask the context, not just the error. A cancellation that lands
			// inside the enqueue transaction surfaces as sql.ErrTxDone — the
			// database/sql watchdog has already rolled back, so Commit reports
			// "transaction has already been committed or rolled back" and the
			// chain carries no context error at all. That is the single most
			// likely shape of a real interruption, and classifying it off the
			// error alone marks the broadcast failed instead of resumable.
			interrupted := IsInterrupted(err) || ctx.Err() != nil

			// These reads and writes must not ride on ctx: the most common way
			// to get here is that very context being cancelled, and the failed
			// write left the broadcast stranded in "processing" for good, with
			// its task dead and no way back.
			finalizeCtx := context.WithoutCancel(ctx)

			// The broadcast is read before the task: a resume writes the task's
			// fresh retry budget before its own transaction commits (ResumeBroadcast
			// calls ResetRetryBudget inside it), so a broadcast read past a pause
			// means the task read below sees that budget, unless that reset or the
			// read of the task before it failed (both logged).
			broadcast, getBroadcastErr := o.broadcastRepo.GetBroadcast(finalizeCtx, task.WorkspaceID, broadcastID)
			if getBroadcastErr != nil {
				o.logger.WithFields(map[string]interface{}{
					"task_id":      task.ID,
					"broadcast_id": broadcastID,
					"error":        getBroadcastErr.Error(),
				}).Error("Failed to get broadcast for status update on last retry")
				// A deleted broadcast has nothing to finalize: its task fails, not loops.
				var notFound *domain.ErrBroadcastNotFound
				if !errors.As(getBroadcastErr, &notFound) {
					retryFinalization(getBroadcastErr)
				}
				return
			}

			if slices.Contains(statusesKeptOnLastAttempt, broadcast.Status) {
				o.logger.WithFields(map[string]interface{}{
					"task_id":      task.ID,
					"broadcast_id": broadcastID,
					"status":       string(broadcast.Status),
				}).Info("Broadcast was cancelled, paused or completed during its last attempt - leaving its status")
				return
			}

			if !o.stillLastAttempt(finalizeCtx, task) {
				o.logger.WithFields(map[string]interface{}{
					"task_id":      task.ID,
					"broadcast_id": broadcastID,
					"error":        err.Error(),
				}).Info("Task got a fresh retry budget during its last attempt - leaving the broadcast to the retry")
				return
			}

			// A cancellation landing during the reads above makes the task
			// service release the task without spending its budget, so the
			// broadcast must stay resumable. Only a cancellation: past its
			// deadline the task service marks the task failed.
			if errors.Is(ctx.Err(), context.Canceled) {
				interrupted = true
			}

			o.logger.WithFields(map[string]interface{}{
				"task_id":      task.ID,
				"broadcast_id": broadcastID,
				"retry_count":  task.RetryCount,
				"max_retries":  task.MaxRetries,
				"error":        err.Error(),
				"interrupted":  interrupted,
			}).Info("Task failed on last retry attempt, finalizing broadcast status")

			// Only the status fields, and only if the row still holds none of
			// the kept statuses: a pause committing after the read above is
			// kept, as are the other columns (a winner picked meanwhile).
			writeStatus := func(interrupted bool) (bool, error) {
				broadcast.Status = domain.BroadcastStatusFailed
				if interrupted {
					broadcast.Status = domain.BroadcastStatusPaused
					pausedAt := time.Now().UTC()
					broadcast.PausedAt = &pausedAt
					reason := fmt.Sprintf("Sending was interrupted and stopped after %d attempts: %v. Resume to continue where it left off.",
						task.RetryCount+1, err)
					broadcast.PauseReason = &reason
				}
				return o.broadcastRepo.UpdateBroadcastStatusUnless(finalizeCtx, broadcast, statusesKeptOnLastAttempt)
			}
			// Each write is logged as it ends, so a failed status committed
			// before the pause below stays in the log whatever the pause does.
			logWrite := func(written bool, updateErr error) {
				fields := map[string]interface{}{
					"task_id":      task.ID,
					"broadcast_id": broadcastID,
					"status":       string(broadcast.Status),
				}
				switch {
				case updateErr != nil:
					fields["error"] = updateErr.Error()
					o.logger.WithFields(fields).Error("Failed to update broadcast status after max retries")
				case !written:
					o.logger.WithFields(fields).Info("Broadcast was cancelled, paused, completed or deleted before its last attempt finalized it - leaving it")
				default:
					o.logger.WithFields(fields).Info("Broadcast finalized after max retries reached")
				}
			}
			written, updateErr := writeStatus(interrupted)
			logWrite(written, updateErr)
			if updateErr != nil {
				retryFinalization(updateErr)
				return
			}
			// A cancellation landing while that write ran, or waited on the row
			// lock, makes the task service release the task too: the broadcast
			// written failed is then paused. Failed is not a kept status, so
			// this write goes through.
			if updateErr == nil && written && !interrupted && errors.Is(ctx.Err(), context.Canceled) {
				logWrite(writeStatus(true))
			}
		}
	}()

	// Initialize structured state if needed
	if task.State == nil {
		// Initialize a new state for the broadcast task
		task.State = &domain.TaskState{
			Progress: 0,
			Message:  "Starting broadcast",
			SendBroadcast: &domain.SendBroadcastState{
				EnqueuedCount:             0,
				FailedCount:               0,
				RecipientOffset:           0,        // Track how many recipients we've processed
				Phase:                     "single", // Default phase
				TestPhaseCompleted:        false,
				WinnerPhaseRecipientCount: 0,
			},
		}
	}

	// Initialize the SendBroadcast state if it doesn't exist yet
	if task.State.SendBroadcast == nil {
		task.State.SendBroadcast = &domain.SendBroadcastState{
			EnqueuedCount:             0,
			FailedCount:               0,
			RecipientOffset:           0,
			Phase:                     "single", // Default phase
			TestPhaseCompleted:        false,
			WinnerPhaseRecipientCount: 0,
		}
	}

	// Extract broadcast ID from task state or context
	broadcastState := task.State.SendBroadcast
	if broadcastState.BroadcastID == "" && task.BroadcastID != nil {
		// If state doesn't have broadcast ID but task does, use it
		broadcastState.BroadcastID = *task.BroadcastID
	}

	if broadcastState.BroadcastID == "" {
		// In a real implementation, we'd expect the broadcast ID to be set when creating the task
		err = NewBroadcastErrorWithTask(
			ErrCodeTaskStateInvalid,
			"broadcast ID is missing in task state",
			task.ID,
			false,
			nil,
		)
		return false, err
	}

	// Store the broadcast ID for the defer function
	broadcastID = broadcastState.BroadcastID

	// Track progress
	sentCount := broadcastState.EnqueuedCount
	failedCount := broadcastState.FailedCount
	// processedCount will be calculated later when needed
	var processedCount int
	startTime := o.timeProvider.Now()
	lastSaveTime := o.timeProvider.Now()
	lastLogTime := o.timeProvider.Now()

	// Track if broadcast was cancelled during processing
	broadcastCancelledDuringProcessing := false
	pausedDuringProcessing := false

	// A run that has neither moved the cursor nor enqueued anything is the run that
	// begins sending. Captured here because it is about to stop being true: the send
	// loop below advances both fields. TotalRecipients cannot be used for this — the
	// counting block right below returns on every one of its exits, so by the time
	// sending starts TotalRecipients is always non-zero.
	isFirstSendingRun := broadcastState.RecipientOffset == 0 && broadcastState.EnqueuedCount == 0

	// Phase 1: Get recipient count if not already set
	if broadcastState.TotalRecipients == 0 {
		counted, readErr := o.getBroadcastForCount(ctx, task.WorkspaceID, broadcastState.BroadcastID)
		if readErr != nil {
			err = readErr
			return false, err
		}
		if refused, done, draftErr := o.refuseDraft(task, counted); refused {
			err = draftErr
			return done, err
		}
		count, countErr := o.countRecipients(ctx, task.WorkspaceID, counted)
		if countErr != nil {
			// codecov:ignore:start
			o.logger.WithFields(map[string]interface{}{
				"task_id":      task.ID,
				"broadcast_id": broadcastState.BroadcastID,
				"error":        countErr.Error(),
			}).Error("Failed to get recipient count for broadcast")
			// codecov:ignore:end
			err = countErr
			return false, err
		}

		broadcastState.TotalRecipients = count
		broadcastState.ChannelType = "email" // Hardcoded for now

		task.State.Message = "Preparing to send broadcast"
		task.Progress = 0

		// codecov:ignore:start
		o.logger.WithFields(map[string]interface{}{
			"task_id":          task.ID,
			"broadcast_id":     broadcastState.BroadcastID,
			"total_recipients": broadcastState.TotalRecipients,
			"channel_type":     broadcastState.ChannelType,
		}).Info("Broadcast sending initialized")
		// codecov:ignore:end

		// If there are no recipients, we can mark as completed immediately
		if broadcastState.TotalRecipients == 0 {
			task.State.Message = "Broadcast completed: No recipients found"
			task.Progress = 100.0
			task.State.Progress = 100.0

			// codecov:ignore:start
			o.logger.WithFields(map[string]interface{}{
				"task_id":      task.ID,
				"broadcast_id": broadcastState.BroadcastID,
			}).Info("Broadcast completed with no recipients")
			// codecov:ignore:end

			allDone, err = o.finishRun(ctx, task, broadcastState.BroadcastID, func(broadcast *domain.Broadcast) {
				completedAt := time.Now().UTC()
				broadcast.Status = domain.BroadcastStatusProcessed
				broadcast.UpdatedAt = completedAt
				broadcast.CompletedAt = &completedAt
				broadcast.EnqueuedCount = 0
			})
			return allDone, err
		}

		// Update the task state with the broadcast state
		task.State.SendBroadcast = broadcastState

		// Early return to save state before processing
		allDone = false
		return allDone, err
	}

	// Get the workspace to retrieve email provider settings
	workspace, workspaceErr := o.workspaceRepo.GetByID(ctx, task.WorkspaceID)
	if workspaceErr != nil {
		err = fmt.Errorf("failed to get workspace: %w", workspaceErr)
		return false, err
	}

	// Get the email provider and integration ID using the workspace's GetEmailProviderWithIntegrationID method
	emailProvider, integrationID, providerErr := workspace.GetEmailProviderWithIntegrationID(true)
	if providerErr != nil {
		err = providerErr
		return false, err
	}

	// Validate that the provider is configured
	if emailProvider == nil || emailProvider.Kind == "" {
		err = fmt.Errorf("no email provider configured for marketing emails")
		return false, err
	}

	// Get the broadcast to access its template variations
	broadcast, err := o.broadcastRepo.GetBroadcast(ctx, task.WorkspaceID, broadcastState.BroadcastID)
	if err != nil {
		return false, err
	}
	if refused, done, draftErr := o.refuseDraft(task, broadcast); refused {
		err = draftErr
		return done, err
	}

	// A scheduled broadcast starts sending with this run: it moves to testing (an A/B test)
	// or processing before a phase is chosen, so an A/B test takes its test phase, as one
	// sent right away does, and the run goes on with the broadcast read again after that
	// write (a save committed just before it included).
	if broadcast.Status == domain.BroadcastStatusScheduled {
		broadcast, err = o.startScheduledSend(ctx, task, broadcast.ID)
		if err != nil {
			return false, err
		}
		if refused, done, draftErr := o.refuseDraft(task, broadcast); refused {
			err = draftErr
			return done, err
		}
		if broadcast.Status == domain.BroadcastStatusCancelled {
			o.logger.WithFields(map[string]interface{}{
				"task_id":      task.ID,
				"broadcast_id": broadcast.ID,
			}).Info("Broadcast was cancelled before its sending started - task completed without sending")
			return true, nil
		}
	}

	// Check if we should perform auto winner evaluation
	if broadcastState.Phase == "test" && broadcast.Status == domain.BroadcastStatusTestCompleted {
		if o.shouldEvaluateWinner(broadcast) {
			if err := o.evaluateWinner(ctx, broadcast, broadcastState); err != nil {
				// Log error but continue - will fall back to manual selection
				o.logger.WithFields(map[string]interface{}{
					"broadcast_id": broadcast.ID,
					"error":        err.Error(),
				}).Error("Auto winner evaluation failed, continuing with manual selection")
			}

			// Refresh broadcast after potential evaluation
			broadcast, err = o.broadcastRepo.GetBroadcast(ctx, task.WorkspaceID, broadcastState.BroadcastID)
			if err != nil {
				return false, err
			}
		}
	}

	// Determine phase based on broadcast status and test settings
	if broadcast.TestSettings.Enabled {
		// A/B testing enabled
		if broadcastState.Phase == "" {
			// Initialize phase based on current status
			switch broadcast.Status {
			case domain.BroadcastStatusProcessing:
				if broadcast.WinningTemplate != nil {
					// Winner already selected, proceed to winner phase
					enterWinnerPhase(broadcastState)
				} else {
					// Start test phase
					broadcastState.Phase = "test"
					// Update broadcast status to testing when test phase begins
					if broadcast.Status != domain.BroadcastStatusTesting {
						broadcast.Status = domain.BroadcastStatusTesting
						broadcast.UpdatedAt = time.Now().UTC()
						if err := o.broadcastRepo.UpdateBroadcast(ctx, broadcast); err != nil {
							o.logger.WithFields(map[string]interface{}{
								"broadcast_id": broadcast.ID,
								"error":        err.Error(),
							}).Error("Failed to update broadcast status to testing")
							return false, fmt.Errorf("failed to update broadcast status to testing: %w", err)
						}
						o.logger.WithField("broadcast_id", broadcast.ID).Info("A/B test phase started - broadcast status updated to testing")
					}
				}
			case domain.BroadcastStatusTesting:
				// The test phase is already under way, but this run has no phase
				// in its state — the run that started the phase ended before it
				// could persist one (a failed run writes no state). Without this
				// the phase falls through to the default below and the run sends
				// the *entire* audience as a single blast, mixing variations,
				// then leaves the broadcast stuck in "testing" because none of
				// the completion branches match an empty phase.
				//
				// Deliberately not extended to test_completed: reaching that
				// status persists the phase on the way out, so the same gap is
				// far narrower there, and what a recovered run should do with an
				// already-finished test phase is not established.
				broadcastState.Phase = "test"
			case domain.BroadcastStatusWinnerSelected:
				// Winner has been selected, proceed to winner phase
				enterWinnerPhase(broadcastState)
			}
		}
	} else {
		// Single template broadcast
		broadcastState.Phase = "single"
	}

	// Calculate recipient counts based on phase
	testRecipientCount := 0
	winnerRecipientCount := 0

	if broadcastState.Phase == "test" || broadcastState.Phase == "winner" {
		// Calculate test sample size
		testRecipientCount = (broadcastState.TotalRecipients * broadcast.TestSettings.SamplePercentage) / 100
		if testRecipientCount < 1 {
			testRecipientCount = 1 // Minimum test size
		}
		winnerRecipientCount = broadcastState.TotalRecipients - testRecipientCount

		// Store calculated counts in state
		broadcastState.TestPhaseRecipientCount = testRecipientCount
		broadcastState.WinnerPhaseRecipientCount = winnerRecipientCount
	}

	// A winner selected manually after the test phase started moves this run to
	// the winner phase before the templates load, so the losing variations are
	// not loaded: the queue sender picks at random among the templates it holds.
	if broadcastState.Phase == "test" {
		if broadcast.WinningTemplate != nil || broadcast.Status == domain.BroadcastStatusWinnerSelected {
			enterWinnerPhase(broadcastState)
		}
	}

	// Determine which templates to load based on phase
	var templateIDs []string

	switch broadcastState.Phase {
	case "test":
		// Load all variations for testing
		templateIDs = make([]string, len(broadcast.TestSettings.Variations))
		for i, variation := range broadcast.TestSettings.Variations {
			templateIDs[i] = variation.TemplateID
		}
	case "winner":
		// Load only the winning template
		if broadcast.WinningTemplate != nil {
			templateIDs = []string{*broadcast.WinningTemplate}
		} else {
			return false, fmt.Errorf("winner phase but no winning template selected")
		}
	default:
		// Single template broadcast
		templateIDs = make([]string, len(broadcast.TestSettings.Variations))
		for i, variation := range broadcast.TestSettings.Variations {
			templateIDs[i] = variation.TemplateID
		}
	}

	// Phase 2: Load templates

	templates, templatesErr := o.LoadTemplates(ctx, task.WorkspaceID, templateIDs)
	if templatesErr != nil {
		// codecov:ignore:start
		o.logger.WithFields(map[string]interface{}{
			"task_id":      task.ID,
			"broadcast_id": broadcastState.BroadcastID,
			"error":        templatesErr.Error(),
		}).Error("Failed to load templates for broadcast")
		// codecov:ignore:end
		err = templatesErr
		return false, err
	}

	// Validate templates
	if validateErr := o.ValidateTemplates(templates); validateErr != nil {
		// codecov:ignore:start
		o.logger.WithFields(map[string]interface{}{
			"task_id":      task.ID,
			"broadcast_id": broadcastState.BroadcastID,
			"error":        validateErr.Error(),
		}).Error("Templates validation failed")
		// codecov:ignore:end
		err = validateErr
		return false, err
	}

	// A sender chosen on a variation may have left the integration since the broadcast
	// was scheduled. Pause before anyone is sent from another address. A broadcast already
	// paused stops in the loop below, and one whose test phase is over waits for a winner:
	// neither is paused again for a variation it may never send. SendBatch runs the same
	// check on the templates of each batch it is given, so nothing leaves from another
	// address either way. A run with every recipient queued already (resumed after a pause
	// that came while its last batch was queued) has no email left to address, so it is not
	// paused for a sender either, as ResumeBroadcast does not refuse it.
	nothingLeftToQueue := (broadcastState.Phase == "single" || broadcastState.Phase == "winner") &&
		broadcastState.TotalRecipients > 0 && broadcastState.RecipientOffset >= int64(broadcastState.TotalRecipients)
	if broadcast.Status != domain.BroadcastStatusPaused && broadcast.Status != domain.BroadcastStatusTestCompleted && !nothingLeftToQueue {
		if senderErr := findUnavailableVariationSender(broadcast, templates, emailProvider); senderErr != nil {
			o.pauseForSendError(ctx, task, broadcastState.BroadcastID, broadcastState.RecipientOffset, senderErr)
			err = senderErr
			return false, err
		}
	}

	// Announce that this broadcast has started sending, once every gate that can still
	// abort the run without mailing anyone has been cleared: the workspace and provider
	// lookup, the winner-selection branches, and the two template checks directly above.
	// Announcing any earlier writes an annotation for a send that never happened — a
	// deleted template is enough — and nothing ever retracts one, so the chart would
	// carry a vertical for a broadcast that mailed nobody.
	//
	// isFirstSendingRun was captured far above, before the counting block: that block
	// returns on every exit, so the broadcast row is only ever in hand on a later task
	// run. context.Background() matches the other publishes in this file — the
	// announcement must outlive the request that triggered the run.
	//
	// A paused or cancelled broadcast stops in the loop below without sending: it is not
	// announced.
	//
	// Every sending run records the start date when none is stored yet. "Send now" and a
	// resume set it themselves; a scheduled broadcast reaches sending only through these
	// runs, so without this it never got one. Not only the first run: a scheduled send
	// already under way when the upgrade to v44 ran, or a first write that failed, is dated
	// by the next run. The write sets started_at alone and only while it is NULL, so once a
	// date is stored the check below costs nothing. A full-row write later in this run keeps
	// the stored date: those writes leave started_at alone when the copy has none. Failing to
	// record the date does not stop the send. A paused or cancelled broadcast stops in the
	// loop below without sending: it is neither dated nor announced.
	startedAt := o.timeProvider.Now().UTC()
	if broadcast.StartedAt == nil && broadcast.Status != domain.BroadcastStatusPaused &&
		broadcast.Status != domain.BroadcastStatusCancelled {
		if stampErr := o.broadcastRepo.MarkBroadcastStarted(ctx, task.WorkspaceID, broadcast.ID, startedAt); stampErr != nil {
			// codecov:ignore:start
			o.logger.WithFields(map[string]interface{}{
				"task_id":      task.ID,
				"broadcast_id": broadcast.ID,
				"error":        stampErr.Error(),
			}).Error("Failed to record the broadcast start date")
			// codecov:ignore:end
		}
	}

	if isFirstSendingRun && broadcast.Status != domain.BroadcastStatusPaused &&
		broadcast.Status != domain.BroadcastStatusCancelled {
		if o.eventBus != nil {
			o.eventBus.Publish(context.Background(), domain.EventPayload{
				Type:        domain.EventBroadcastSendingStarted,
				WorkspaceID: task.WorkspaceID,
				EntityID:    broadcast.ID,
				Data: map[string]interface{}{
					"broadcast_name": broadcast.Name,
					"started_at":     startedAt.Format(time.RFC3339),
				},
			})
		}
	}

	// Phase 3: Process recipients in batches with a timeout
	// Use the timeoutAt parameter passed from task service
	processTimeoutAt := timeoutAt

	// Whether we've processed all recipients
	allDone = false

	// Determine phase-specific limits
	var recipientLimit int
	var currentOffset int

	// All phases use the same RecipientOffset for continuity (progress tracking)
	currentOffset = int(broadcastState.RecipientOffset)
	// Cursor for keyset pagination - tracks the last processed email
	cursor := broadcastState.LastProcessedEmail

	// Set recipient limit based on current phase (after potential transition)
	switch broadcastState.Phase {
	case "test":
		recipientLimit = broadcastState.TestPhaseRecipientCount
	case "winner":
		// Winner phase processes remaining recipients after test phase
		recipientLimit = broadcastState.TotalRecipients
	default:
		// Single template - process all recipients
		recipientLimit = broadcastState.TotalRecipients
	}

	// Process until timeout or completion
	for {
		// Refresh broadcast each iteration to observe external changes (e.g., manual winner selection, cancellation)
		if refreshed, refreshErr := o.broadcastRepo.GetBroadcast(ctx, task.WorkspaceID, broadcastState.BroadcastID); refreshErr == nil && refreshed != nil {
			broadcast = refreshed

			// Check if broadcast has been cancelled
			if broadcast.Status == domain.BroadcastStatusCancelled {
				o.logger.WithFields(map[string]interface{}{
					"broadcast_id": broadcast.ID,
					"task_id":      task.ID,
				}).Info("Broadcast cancelled during processing - stopping task")
				// Delete any rows we enqueued between the service-layer DeleteBySourceTx
				// commit and this poll — closes the late-enqueue race window.
				o.deleteQueueEntries(ctx, task.WorkspaceID, broadcast.ID)
				// Task should stop processing as broadcast is cancelled
				broadcastCancelledDuringProcessing = true
				allDone = true
				break
			}

			// Check if broadcast has been paused
			if broadcast.Status == domain.BroadcastStatusPaused {
				o.logger.WithFields(map[string]interface{}{
					"broadcast_id": broadcast.ID,
					"task_id":      task.ID,
				}).Info("Broadcast paused during processing - stopping task")
				// Pause any rows we enqueued between the service-layer PauseBySourceTx
				// commit and this poll — closes the late-enqueue race window.
				o.pauseQueueEntries(ctx, task.WorkspaceID, broadcast.ID)
				// The task is parked once the progress below is recorded: a resume wakes it.
				pausedDuringProcessing = true
				allDone = false
				break
			}

			// If currently in test phase and a winner was selected meanwhile, transition to winner phase
			if broadcastState.Phase == "test" && (broadcast.WinningTemplate != nil || broadcast.Status == domain.BroadcastStatusWinnerSelected) {
				enterWinnerPhase(broadcastState)
				recipientLimit = broadcastState.TotalRecipients
				o.logger.WithFields(map[string]interface{}{
					"broadcast_id": broadcast.ID,
					"task_id":      task.ID,
				}).Info("Winner selected during test phase - transitioning to winner phase")
			}
		}
		// Check time-based timeout
		if time.Now().After(processTimeoutAt) {
			o.logger.WithField("task_id", task.ID).Info("Processing time limit reached - pausing task")
			allDone = false // Task will be paused and resumed in next cron run
			break
		}

		// Check if phase is complete
		if broadcastState.Phase == "test" && currentOffset >= recipientLimit {
			// A winner the refresh above saw has moved the run to the winner phase
			// already; one picked since is found on the locked row by the completion.
			allDone = o.handleTestPhaseCompletion(ctx, broadcast, broadcastState)
			break
		} else if broadcastState.Phase == "winner" && currentOffset >= recipientLimit {
			// Winner phase complete
			allDone = true
			break
		} else if broadcastState.Phase == "single" && currentOffset >= recipientLimit {
			// Single template broadcast complete
			allDone = true
			break
		}

		// Calculate remaining recipients for this phase
		remainingInPhase := recipientLimit - currentOffset
		batchSize := o.config.FetchBatchSize
		if remainingInPhase < batchSize {
			batchSize = remainingInPhase
		}

		if batchSize <= 0 {
			// Phase complete
			if broadcastState.Phase == "test" {
				// A winner the refresh above saw has moved the run to the winner phase
				// already; one picked since is found on the locked row by the completion.
				allDone = o.handleTestPhaseCompletion(ctx, broadcast, broadcastState)
			} else {
				allDone = true
			}
			break
		}

		// A run that moved to the winner phase after loading every variation
		// sends its batches with the winner alone.
		batchTemplates := templates
		if broadcastState.Phase == "winner" {
			var winnerErr error
			batchTemplates, winnerErr = o.winnerTemplates(ctx, task.WorkspaceID, broadcast, templates)
			if winnerErr != nil {
				err = winnerErr
				return false, err
			}
		}

		// Fetch the next batch of recipients using cursor-based pagination
		recipients, batchErr := o.FetchBatch(
			ctx,
			task.WorkspaceID,
			broadcastState.BroadcastID,
			cursor,
			batchSize,
		)
		if batchErr != nil {
			// Check for cancellation error specifically
			if broadcastErr, ok := batchErr.(*BroadcastError); ok && broadcastErr.Code == ErrCodeBroadcastCancelled {
				o.logger.WithFields(map[string]interface{}{
					"task_id":      task.ID,
					"broadcast_id": broadcastState.BroadcastID,
				}).Info("Broadcast cancelled - stopping task execution")
				// Return true (task complete) with nil error to avoid marking as failed
				allDone = true
				err = nil
				return allDone, err
			}

			err = batchErr
			return false, err
		}

		// If no more recipients, we're done
		if len(recipients) == 0 {
			// codecov:ignore:start
			o.logger.WithFields(map[string]interface{}{
				"task_id":      task.ID,
				"broadcast_id": broadcastState.BroadcastID,
				"offset":       currentOffset,
				"phase":        broadcastState.Phase,
			}).Info("No more recipients to process")
			// codecov:ignore:end
			if broadcastState.Phase == "test" {
				// A winner the refresh above saw has moved the run to the winner phase
				// already; one picked since is found on the locked row by the completion.
				allDone = o.handleTestPhaseCompletion(ctx, broadcast, broadcastState)
			} else {
				allDone = true
			}
			break
		}

		// Each batch is sent with the provider as it is now: a sender removed from the
		// integration since the run began is then refused by SendBatch, which pauses the
		// broadcast, instead of the run sending from it until its end.
		workspace, emailProvider, integrationID = o.refreshProvider(ctx, task, workspace, emailProvider, integrationID)

		// Resolve the tracking/base endpoint: custom endpoint if set, else the API endpoint.
		endpoint := workspace.Settings.ResolveEndpoint(o.apiEndpoint)

		// Process this batch of recipients
		sent, failed, sendErr := o.messageSender.SendBatch(
			ctx,
			task.WorkspaceID,
			integrationID,
			workspace.Settings.SecretKey,
			endpoint,
			workspace.Settings.WebsiteURL,
			workspace.Settings.TrackingPolicy(),
			workspace.Settings.WebAnalytics,
			broadcastState.BroadcastID,
			recipients,
			batchTemplates,
			emailProvider,
			processTimeoutAt,
			workspace.Settings.DefaultLanguage,
		)

		// Handle errors during sending
		if sendErr != nil {
			// A recipient feed that failed, or a sender chosen on a variation that has
			// left the integration, pauses the broadcast for the user to fix.
			if errors.Is(sendErr, ErrBroadcastShouldPause) {
				o.pauseForSendError(ctx, task, broadcastState.BroadcastID, int64(currentOffset), sendErr)

				err = sendErr
				return false, err
			}

			// codecov:ignore:start
			o.logger.WithFields(map[string]interface{}{
				"task_id":      task.ID,
				"broadcast_id": broadcastState.BroadcastID,
				"offset":       currentOffset,
				"error":        sendErr.Error(),
			}).Error("Error sending batch")
			// codecov:ignore:end

			// End the run here. A batch-level failure means the enqueue
			// transaction never committed, so nothing was written for anyone in
			// the batch; carrying on would advance the offset and the keyset
			// cursor past recipients who were never enqueued and never will be.
			// Leaving both untouched lets the next run re-process the same
			// batch from the same cursor.
			err = sendErr
			return false, err
		}

		// Update progress counters
		sentCount += sent
		failedCount += failed

		// Update recipient offset - used across all phases for continuity (progress tracking)
		broadcastState.RecipientOffset += int64(sent + failed)
		currentOffset = int(broadcastState.RecipientOffset)

		// Update cursor for next batch - use the email of the last PROCESSED contact
		// This is critical: we must use sent+failed (what was actually processed), not len(recipients) (what was fetched)
		// If SendBatch times out mid-batch, only part of the fetched batch may be processed
		if len(recipients) > 0 {
			processedInBatch := sent + failed
			if processedInBatch > 0 && processedInBatch <= len(recipients) {
				cursor = recipients[processedInBatch-1].Contact.Email
				broadcastState.LastProcessedEmail = cursor
			}
			// If nothing was processed, don't update cursor (will retry same batch on next run)
		}

		// Use sent + failed as the number of recipients processed/attempted
		processedCount = sentCount + failedCount

		// Log progress at regular intervals
		if o.timeProvider.Since(lastLogTime) >= o.config.ProgressLogInterval {
			// codecov:ignore:start
			o.logger.WithFields(map[string]interface{}{
				"task_id":         task.ID,
				"broadcast_id":    broadcastState.BroadcastID,
				"phase":           broadcastState.Phase,
				"sent_count":      sentCount,
				"failed_count":    failedCount,
				"processed_count": processedCount,
				"phase_offset":    currentOffset,
				"phase_limit":     recipientLimit,
				"total_count":     broadcastState.TotalRecipients,
				"cursor":          cursor,
				"progress":        CalculateProgress(processedCount, broadcastState.TotalRecipients),
				"elapsed":         o.timeProvider.Since(startTime).String(),
			}).Info("Broadcast progress update")
			// codecov:ignore:end
			lastLogTime = o.timeProvider.Now()
		}

		// Save progress to the task
		var saveErr error
		lastSaveTime, saveErr = o.SaveProgressState(
			ctx,
			task.WorkspaceID,
			task.ID,
			broadcastState,
			sentCount,
			failedCount,
			processedCount,
			lastSaveTime,
			startTime,
		)
		if errors.Is(saveErr, domain.ErrTaskNotRunning) {
			// The task is no longer this run's: another execution holds it, a shutdown released
			// it while this save was under way, or a cancel marked it failed (a pause leaves a
			// held task to its run). The next batch would go to
			// recipients that are not this run's to send, so the run stops and writes nothing
			// more to the task. The batch it just queued may have committed after the cancel
			// cleared the queue: it is settled as the broadcast now stands.
			o.logger.WithFields(map[string]interface{}{
				"task_id":      task.ID,
				"broadcast_id": broadcastState.BroadcastID,
			}).Warn("Task is no longer this run's - stopping this run")
			o.settleQueuedAfterLostTask(ctx, task.WorkspaceID, broadcastState.BroadcastID)
			err = saveErr
			return false, err
		}
		if saveErr != nil {
			// codecov:ignore:start
			o.logger.WithFields(map[string]interface{}{
				"task_id":      task.ID,
				"broadcast_id": broadcastState.BroadcastID,
				"error":        saveErr.Error(),
			}).Error("Failed to save progress state")
			// codecov:ignore:end
			// Continue processing despite save error
		}

		// Update local counters for state
		broadcastState.EnqueuedCount = sentCount
		broadcastState.FailedCount = failedCount

		// Continue to next batch; do not prematurely mark done based on returned count
	}

	// Update task state with the latest progress data
	// Use sent + failed as the number of recipients processed/attempted for final progress
	processedCount = sentCount + failedCount
	progress := CalculateProgress(processedCount, broadcastState.TotalRecipients)
	message := FormatProgressMessage(processedCount, broadcastState.TotalRecipients, time.Since(startTime))

	task.State.Progress = progress
	task.State.Message = message
	task.Progress = progress

	if pausedDuringProcessing {
		err = o.parkPausedTask(task, broadcastState.BroadcastID)
		return false, err
	}

	// If the task is complete, update the broadcast status appropriately
	if allDone {
		// Don't update status if broadcast was cancelled during processing
		if broadcastCancelledDuringProcessing {
			o.logger.WithFields(map[string]interface{}{
				"task_id":      task.ID,
				"broadcast_id": broadcastState.BroadcastID,
			}).Info("Broadcast was cancelled during processing - task completed without status update")
			return true, nil
		}

		switch broadcastState.Phase {
		case "winner", "single":
			// Winner phase or single template complete: the broadcast is processed.
			allDone, err = o.finishRun(ctx, task, broadcastState.BroadcastID, func(broadcast *domain.Broadcast) {
				completedAt := time.Now().UTC()
				broadcast.Status = domain.BroadcastStatusProcessed
				broadcast.UpdatedAt = completedAt
				broadcast.CompletedAt = &completedAt
				if broadcastState.Phase == "winner" {
					broadcast.WinnerSentAt = &completedAt
				}
				broadcast.EnqueuedCount = broadcastState.EnqueuedCount
				// A copy SelectWinner wrote may still hold no phase counts
				setPhaseRecipientCounts(broadcast, broadcastState)
			})
			if err != nil || !allDone {
				return allDone, err
			}
			// codecov:ignore:start
			o.logger.WithFields(map[string]interface{}{
				"task_id":        task.ID,
				"broadcast_id":   broadcastState.BroadcastID,
				"enqueued_count": sentCount,
				"failed_count":   failedCount,
				"phase":          broadcastState.Phase,
			}).Info("Broadcast processing finished")
			// codecov:ignore:end
		case "test":
			// Test phase complete - should have been handled by handleTestPhaseCompletion
			// This shouldn't happen, but handle it gracefully
			o.logger.WithField("broadcast_id", broadcastState.BroadcastID).Warn("Test phase marked as complete in final processing - this should have been handled earlier")
			return false, nil // Pause task for winner selection
		}
	}

	// codecov:ignore:start
	o.logger.WithFields(map[string]interface{}{
		"task_id":          task.ID,
		"broadcast_id":     broadcastState.BroadcastID,
		"enqueued_total":   broadcastState.EnqueuedCount,
		"failed_total":     broadcastState.FailedCount,
		"total_recipients": broadcastState.TotalRecipients,
		"progress":         task.Progress,
		"all_done":         allDone,
	}).Info("Broadcast processing cycle completed")
	// codecov:ignore:end
	return allDone, err
}

// startScheduledSend moves a scheduled broadcast to the status its sending starts in
// (StartScheduledBroadcast: testing for an A/B test with no winner yet, processing
// otherwise) and returns the broadcast read again after that write. A write that finds the
// broadcast no longer scheduled writes nothing: the broadcast read again tells the run what
// it became (back to draft, cancelled). One still scheduled after the write fails the run:
// sent on, it would have no phase, which sent an A/B test's every variation to the whole
// audience.
func (o *BroadcastOrchestrator) startScheduledSend(ctx context.Context, task *domain.Task, broadcastID string) (*domain.Broadcast, error) {
	started, err := o.broadcastRepo.StartScheduledBroadcast(ctx, task.WorkspaceID, broadcastID, time.Now().UTC())
	if err != nil {
		o.logger.WithFields(map[string]interface{}{
			"task_id":      task.ID,
			"broadcast_id": broadcastID,
			"error":        err.Error(),
		}).Error("Failed to start the scheduled broadcast")
		if interrupted, ok := wrapIfInterrupted(err); ok {
			return nil, interrupted
		}
		return nil, err
	}
	broadcast, err := o.broadcastRepo.GetBroadcast(ctx, task.WorkspaceID, broadcastID)
	if err != nil {
		if interrupted, ok := wrapIfInterrupted(err); ok {
			return nil, interrupted
		}
		return nil, err
	}
	if broadcast.Status == domain.BroadcastStatusScheduled {
		return nil, fmt.Errorf("broadcast %s is still scheduled after its sending started", broadcastID)
	}
	o.logger.WithFields(map[string]interface{}{
		"task_id":      task.ID,
		"broadcast_id": broadcastID,
		"started":      started,
		"status":       string(broadcast.Status),
	}).Info("Scheduled broadcast started sending")
	return broadcast, nil
}

// refreshProvider reads the workspace again for the next batch. A read that fails, or that
// finds no marketing provider, keeps the run's copy (logged at Warn): the batch then goes as
// it would have, and a provider that is really gone fails it there.
func (o *BroadcastOrchestrator) refreshProvider(ctx context.Context, task *domain.Task, workspace *domain.Workspace,
	provider *domain.EmailProvider, integrationID string) (*domain.Workspace, *domain.EmailProvider, string) {
	fresh, err := o.workspaceRepo.GetByID(ctx, task.WorkspaceID)
	if err != nil {
		o.logger.WithFields(map[string]interface{}{
			"task_id": task.ID,
			"error":   err.Error(),
		}).Warn("Failed to read the workspace again for the next batch - keeping the run's email provider")
		return workspace, provider, integrationID
	}
	freshProvider, freshID, err := fresh.GetEmailProviderWithIntegrationID(true)
	if err != nil || freshProvider == nil || freshProvider.Kind == "" {
		o.logger.WithField("task_id", task.ID).
			Warn("No marketing email provider found for the next batch - keeping the run's email provider")
		return workspace, provider, integrationID
	}
	return fresh, freshProvider, freshID
}

// A task is created in the schedule request before the broadcast's own write commits, so
// the scheduler can pick it while the broadcast still reads as a draft, and a schedule whose
// broadcast write rolled back leaves a task for a draft. A draft is never sent: while the
// task is young it is handed back for draftRecheckInterval (the schedule is committing),
// and once it is older than orphanTaskAge it is completed without sending. A later
// schedule of the broadcast resets the task.
const (
	draftRecheckInterval = 30 * time.Second
	orphanTaskAge        = 10 * time.Minute
)

// refuseDraft reports whether the run stops because its broadcast is a draft, and with what.
func (o *BroadcastOrchestrator) refuseDraft(task *domain.Task, broadcast *domain.Broadcast) (refused bool, done bool, err error) {
	if broadcast.Status != domain.BroadcastStatusDraft {
		return false, false, nil
	}
	if time.Since(task.CreatedAt) < orphanTaskAge {
		return true, false, &domain.TaskRequeueError{After: draftRecheckInterval, Reason: "broadcast is still a draft"}
	}
	o.logger.WithFields(map[string]interface{}{
		"task_id":      task.ID,
		"broadcast_id": broadcast.ID,
	}).Warn("Task of a broadcast that is still a draft - completing it without sending")
	return true, true, nil
}

// emailQueuePauseTx and emailQueueDeleteTx are the queue writes settleQueueEntriesTx makes.
func (o *BroadcastOrchestrator) emailQueuePauseTx(ctx context.Context, tx *sql.Tx, broadcastID string) (int64, error) {
	return o.emailQueueRepo.PauseBySourceTx(ctx, tx, domain.EmailQueueSourceBroadcast, broadcastID)
}

func (o *BroadcastOrchestrator) emailQueueDeleteTx(ctx context.Context, tx *sql.Tx, broadcastID string) (int64, error) {
	return o.emailQueueRepo.DeleteBySourceTx(ctx, tx, domain.EmailQueueSourceBroadcast, broadcastID)
}

// settleQueueEntriesTx pauses or deletes the broadcast's queue rows inside tx. A failure is
// returned: the transaction cannot go on after a failed statement. Nil-safe.
func (o *BroadcastOrchestrator) settleQueueEntriesTx(ctx context.Context, tx *sql.Tx, broadcastID string,
	write func(context.Context, *sql.Tx, string) (int64, error), countField string) error {
	if o.emailQueueRepo == nil {
		return nil
	}
	n, err := write(ctx, tx, broadcastID)
	if err != nil {
		return fmt.Errorf("failed to settle the queue entries of the broadcast: %w", err)
	}
	if n > 0 {
		o.logger.WithFields(map[string]interface{}{
			"broadcast_id": broadcastID,
			countField:     n,
		}).Info("Settled email queue entries for broadcast")
	}
	return nil
}

// finishRun writes the end of a run that queued every recipient: setFinal sets the final
// fields on the broadcast's row read with its lock, so a column another writer changed
// since the run's last refresh is not written back, and a pause or a cancel committed
// meanwhile is kept. A paused broadcast parks the task and its queue entries are paused
// again (the last batch's rows may have been committed after the pause paused the others);
// a cancelled one has them removed. Both happen under the broadcast's lock. The write is made on the run's lease (leaseContext).
func (o *BroadcastOrchestrator) finishRun(ctx context.Context, task *domain.Task, broadcastID string, setFinal func(*domain.Broadcast)) (bool, error) {
	writeCtx, cancel := leaseContext(ctx)
	defer cancel()

	var left domain.BroadcastStatus
	err := o.broadcastRepo.WithTransaction(writeCtx, task.WorkspaceID, func(tx *sql.Tx) error {
		latest, err := o.broadcastRepo.GetBroadcastForUpdateTx(writeCtx, tx, task.WorkspaceID, broadcastID)
		if err != nil {
			return err
		}
		// The queue rows of a paused or cancelled broadcast are settled under its lock:
		// settled after the commit, a resume committed in between had its rows paused
		// again by this run and the broadcast sent nothing more.
		switch latest.Status {
		case domain.BroadcastStatusPaused:
			left = latest.Status
			return o.settleQueueEntriesTx(writeCtx, tx, broadcastID, o.emailQueuePauseTx, "paused_queue_count")
		case domain.BroadcastStatusCancelled:
			left = latest.Status
			return o.settleQueueEntriesTx(writeCtx, tx, broadcastID, o.emailQueueDeleteTx, "cancelled_queue_count")
		case domain.BroadcastStatusProcessed:
			left = latest.Status
			return nil
		}
		setFinal(latest)
		return o.broadcastRepo.UpdateBroadcastTx(writeCtx, tx, latest)
	})
	if err != nil {
		o.logger.WithFields(map[string]interface{}{
			"task_id":      task.ID,
			"broadcast_id": broadcastID,
			"error":        err.Error(),
		}).Error("Failed to write the end of the run")
		return false, fmt.Errorf("failed to write the end of the run: %w", err)
	}

	switch left {
	case domain.BroadcastStatusPaused:
		return false, o.parkPausedTask(task, broadcastID)
	case domain.BroadcastStatusCancelled:
		o.logger.WithFields(map[string]interface{}{
			"task_id":      task.ID,
			"broadcast_id": broadcastID,
		}).Info("Broadcast was cancelled before its run finished - task completed without status update")
	case domain.BroadcastStatusProcessed:
		o.logger.WithFields(map[string]interface{}{
			"task_id":      task.ID,
			"broadcast_id": broadcastID,
		}).Info("Broadcast already processed - task completed without status update")
	default:
		o.logger.WithFields(map[string]interface{}{
			"task_id":      task.ID,
			"broadcast_id": broadcastID,
		}).Info("Broadcast marked as processed")
	}
	return true, nil
}

// pausedParkInterval is how long a run that found its broadcast paused parks the task, as
// the pause handler does: a resume wakes it before then.
const pausedParkInterval = 24 * time.Hour

// parkPausedTask is what a run that found its broadcast paused returns: its task is parked
// (paused, pausedParkInterval ahead) instead of left due at once, where every scheduler tick
// ran it again for nothing. Once the task is parked, the broadcast is read again: a resume
// committed while this run still held the task could not wake it, so the run wakes it.
// Either the resume's wake finds the task parked, or this read sees the resume.
func (o *BroadcastOrchestrator) parkPausedTask(task *domain.Task, broadcastID string) error {
	return &domain.TaskRequeueError{
		After:  pausedParkInterval,
		Park:   true,
		Reason: "broadcast paused",
		AfterRequeue: func(ctx context.Context) {
			current, err := o.broadcastRepo.GetBroadcast(ctx, task.WorkspaceID, broadcastID)
			if err != nil {
				o.logger.WithFields(map[string]interface{}{
					"task_id":      task.ID,
					"broadcast_id": broadcastID,
					"error":        err.Error(),
				}).Warn("Failed to read the broadcast again after parking its task - a resume wakes it")
				return
			}
			if current.Status == domain.BroadcastStatusPaused {
				return
			}
			if _, err := o.taskRepo.WakeTask(ctx, task.WorkspaceID, task.ID, time.Now().UTC()); err != nil {
				o.logger.WithFields(map[string]interface{}{
					"task_id":      task.ID,
					"broadcast_id": broadcastID,
					"error":        err.Error(),
				}).Warn("Failed to wake the task of a broadcast resumed while its run held it")
				return
			}
			o.logger.WithFields(map[string]interface{}{
				"task_id":      task.ID,
				"broadcast_id": broadcastID,
				"status":       string(current.Status),
			}).Info("Broadcast was resumed while its run held the task - task woken")
		},
	}
}

// enterWinnerPhase moves the run to the winner phase and records, the first time,
// the offset it starts from: what the test phase actually sent, which a winner
// picked early cuts short of the planned sample.
func enterWinnerPhase(state *domain.SendBroadcastState) {
	state.Phase = "winner"
	if state.WinnerPhaseStartOffset == nil {
		start := state.RecipientOffset
		state.WinnerPhaseStartOffset = &start
	}
}

// setPhaseRecipientCounts puts the A/B test and winner recipient counts on the
// broadcast the run is about to write. The run reads the broadcast again before
// each batch, so counts set when they were worked out would not reach the copy
// written. In the winner phase they are what each phase sent; before it, or for a
// state saved before the winner phase start was recorded, the planned split. A
// broadcast with one template keeps 0 for both.
func setPhaseRecipientCounts(broadcast *domain.Broadcast, state *domain.SendBroadcastState) {
	if state.Phase == "winner" && state.WinnerPhaseStartOffset != nil {
		start := *state.WinnerPhaseStartOffset
		broadcast.TestPhaseRecipientCount = int(start)
		broadcast.WinnerPhaseRecipientCount = int(state.RecipientOffset - start)
		return
	}
	broadcast.TestPhaseRecipientCount = state.TestPhaseRecipientCount
	broadcast.WinnerPhaseRecipientCount = state.WinnerPhaseRecipientCount
}

// handleTestPhaseCompletion handles the transition from test phase to test_completed status
func (o *BroadcastOrchestrator) handleTestPhaseCompletion(ctx context.Context, broadcast *domain.Broadcast, broadcastState *domain.SendBroadcastState) bool {
	// The owner can pick a winner during the test phase. The row is read with its
	// lock, as SelectWinner and the automatic evaluation read it, so the three take
	// turns: a pick made first sends this run to the winner phase, and one made
	// after finds the broadcast in test_completed instead of being written over.
	// The run's own copy is what gets written, so the locked row also tells
	// whether the broadcast is still in its test phase.
	now := time.Now().UTC()
	var concurrent *domain.Broadcast
	var left domain.BroadcastStatus
	waiting := false
	// Written on the run's lease, as its progress is.
	writeCtx, cancel := leaseContext(ctx)
	defer cancel()
	err := o.broadcastRepo.WithTransaction(writeCtx, broadcast.WorkspaceID, func(tx *sql.Tx) error {
		latest, err := o.broadcastRepo.GetBroadcastForUpdateTx(writeCtx, tx, broadcast.WorkspaceID, broadcast.ID)
		if err != nil {
			return err
		}
		if latest.WinningTemplate != nil || latest.Status == domain.BroadcastStatusWinnerSelected {
			concurrent = latest
			return nil
		}
		if latest.Status == domain.BroadcastStatusTestCompleted {
			// An earlier run already completed the test phase; this one only
			// finds it still waiting for a winner.
			waiting = true
			return nil
		}
		if latest.Status != domain.BroadcastStatusTesting && latest.Status != domain.BroadcastStatusProcessing {
			// Paused or cancelled since this run last refreshed it. The run's copy
			// is written whole, so writing it now would undo that; the next run's
			// refresh handles the new status instead.
			left = latest.Status
			return nil
		}

		// Mark test phase as completed in state
		broadcastState.TestPhaseCompleted = true

		// Update broadcast status to test_completed
		broadcast.Status = domain.BroadcastStatusTestCompleted
		broadcast.UpdatedAt = now
		if broadcast.TestSentAt == nil {
			broadcast.TestSentAt = &now
		}
		setPhaseRecipientCounts(broadcast, broadcastState)
		return o.broadcastRepo.UpdateBroadcastTx(writeCtx, tx, broadcast)
	})
	if err != nil {
		o.logger.WithFields(map[string]interface{}{
			"broadcast_id": broadcast.ID,
			"error":        err.Error(),
		}).Error("Failed to update broadcast status to test_completed")
		return false // Continue processing despite error
	}

	if waiting {
		return false // Still waiting for winner selection
	}

	if left != "" {
		o.logger.WithFields(map[string]interface{}{
			"broadcast_id":     broadcast.ID,
			"broadcast_status": string(left),
		}).Info("Broadcast left the test phase before it completed - leaving its status as it is")
		return false
	}

	if concurrent != nil {
		// Winner was selected concurrently; transition to winner phase instead of marking test_completed
		enterWinnerPhase(broadcastState)
		winningTemplate := ""
		if concurrent.WinningTemplate != nil {
			winningTemplate = *concurrent.WinningTemplate
		}
		o.logger.WithFields(map[string]interface{}{
			"broadcast_id":     concurrent.ID,
			"winning_template": winningTemplate,
			"broadcast_status": string(concurrent.Status),
		}).Info("Concurrent winner selection detected during test completion - transitioning to winner phase")
		return false
	}

	o.logger.WithFields(map[string]interface{}{
		"broadcast_id":    broadcast.ID,
		"test_sent_count": broadcastState.RecipientOffset,
	}).Info("A/B test phase completed, awaiting winner selection")

	// Log completion - auto evaluation will happen on next task run if enabled
	if broadcast.TestSettings.AutoSendWinner {
		evaluationTime := now.Add(time.Duration(broadcast.TestSettings.TestDurationHours) * time.Hour)
		o.logger.WithFields(map[string]interface{}{
			"broadcast_id":  broadcast.ID,
			"evaluation_at": evaluationTime,
			"test_duration": broadcast.TestSettings.TestDurationHours,
		}).Info("A/B test phase completed, auto winner evaluation will occur after test duration")
	} else {
		o.logger.WithField("broadcast_id", broadcast.ID).Info("A/B test phase completed, awaiting manual winner selection")
	}

	// Task should pause here - winner selection will resume it
	return false // Not all done, waiting for winner selection
}

// shouldEvaluateWinner checks if auto winner evaluation should be performed
func (o *BroadcastOrchestrator) shouldEvaluateWinner(broadcast *domain.Broadcast) bool {
	// Check if auto winner is enabled
	if !broadcast.TestSettings.AutoSendWinner {
		return false
	}

	// Check if test was sent (required for timing calculation)
	if broadcast.TestSentAt == nil {
		return false
	}

	// Calculate evaluation time and check if it has passed
	evaluationTime := broadcast.TestSentAt.Add(time.Duration(broadcast.TestSettings.TestDurationHours) * time.Hour)
	now := o.timeProvider.Now()

	if now.Before(evaluationTime) {
		o.logger.WithFields(map[string]interface{}{
			"broadcast_id":    broadcast.ID,
			"current_time":    now,
			"evaluation_time": evaluationTime,
			"test_duration":   broadcast.TestSettings.TestDurationHours,
		}).Debug("Auto evaluation time has not yet arrived")
		return false
	}

	return true
}

// evaluateWinner performs automatic winner evaluation and updates the broadcast
func (o *BroadcastOrchestrator) evaluateWinner(ctx context.Context, broadcast *domain.Broadcast, broadcastState *domain.SendBroadcastState) error {
	o.logger.WithField("broadcast_id", broadcast.ID).Info("Performing automatic winner evaluation")

	// Perform the evaluation using the ABTestEvaluator
	winnerTemplateID, err := o.abTestEvaluator.EvaluateAndSelectWinner(ctx, broadcast.WorkspaceID, broadcast.ID)
	if err != nil {
		return fmt.Errorf("auto winner evaluation failed: %w", err)
	}

	// Update task state to proceed to winner phase
	enterWinnerPhase(broadcastState)

	o.logger.WithFields(map[string]interface{}{
		"broadcast_id":    broadcast.ID,
		"winner_template": winnerTemplateID,
	}).Info("Auto winner evaluation completed successfully")

	return nil
}
