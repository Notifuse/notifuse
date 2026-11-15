package service

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/google/uuid"
)

// AutomationExecutor processes contacts through automation workflows
type AutomationExecutor struct {
	automationRepo  domain.AutomationRepository
	contactRepo     domain.ContactRepository
	workspaceRepo   domain.WorkspaceRepository
	contactListRepo domain.ContactListRepository
	templateRepo    domain.TemplateRepository
	emailQueueRepo  domain.EmailQueueRepository
	timelineRepo    domain.ContactTimelineRepository
	nodeExecutors   map[domain.NodeType]NodeExecutor
	logger          logger.Logger
	apiEndpoint     string
	claimErrors     repeatedErrorLog // a claim failing in some workspaces, logged once per window
	// shutdownDrain overrides automationShutdownDrain when positive (tests).
	shutdownDrain time.Duration
}

// NewAutomationExecutor creates a new AutomationExecutor
func NewAutomationExecutor(
	automationRepo domain.AutomationRepository,
	contactRepo domain.ContactRepository,
	workspaceRepo domain.WorkspaceRepository,
	contactListRepo domain.ContactListRepository,
	listRepo domain.ListRepository,
	templateRepo domain.TemplateRepository,
	emailQueueRepo domain.EmailQueueRepository,
	timelineRepo domain.ContactTimelineRepository,
	log logger.Logger,
	apiEndpoint string,
	webhookHTTPClient *http.Client,
) *AutomationExecutor {
	qb := NewQueryBuilder()

	executors := map[domain.NodeType]NodeExecutor{
		domain.NodeTypeTrigger:          NewTriggerNodeExecutor(),
		domain.NodeTypeDelay:            NewDelayNodeExecutor(),
		domain.NodeTypeEmail:            NewEmailNodeExecutor(emailQueueRepo, templateRepo, workspaceRepo, listRepo, contactListRepo, apiEndpoint, log),
		domain.NodeTypeBranch:           NewBranchNodeExecutor(qb, workspaceRepo),
		domain.NodeTypeFilter:           NewFilterNodeExecutor(qb, workspaceRepo),
		domain.NodeTypeAddToList:        NewAddToListNodeExecutor(contactListRepo),
		domain.NodeTypeRemoveFromList:   NewRemoveFromListNodeExecutor(contactListRepo),
		domain.NodeTypeABTest:           NewABTestNodeExecutor(),
		domain.NodeTypeWebhook:          NewWebhookNodeExecutor(log, webhookHTTPClient),
		domain.NodeTypeListStatusBranch: NewListStatusBranchNodeExecutor(contactListRepo),
	}

	return &AutomationExecutor{
		automationRepo:  automationRepo,
		contactRepo:     contactRepo,
		workspaceRepo:   workspaceRepo,
		contactListRepo: contactListRepo,
		templateRepo:    templateRepo,
		emailQueueRepo:  emailQueueRepo,
		timelineRepo:    timelineRepo,
		nodeExecutors:   executors,
		logger:          log,
		apiEndpoint:     apiEndpoint,
	}
}

// inTickResumeLease is how far ahead scheduled_at is set after a step when the same tick goes
// on to the next one. It keeps another instance from picking the contact up for the rest of
// the tick, and resumes the journey if the tick dies before its next persist. It is renewed
// after every step, so it only has to outlast one node: a webhook node waits WebhookNodeTimeout
// at most.
// The claim of a batch (ProcessBatch) uses the same lease, renewed before each journey.
const inTickResumeLease = 5 * time.Minute

// automationShutdownDrain is how long, once the app starts stopping, the node under way in a
// batch keeps its context to finish and record itself. Cutting it would leave the journey on
// that node, and the node would run again once the claim ends: the same email sent, or the same
// webhook posted, twice. It is WebhookNodeTimeout plus five seconds for what can still come
// before the call once the stop began (the node's execution row and its context), which have no
// deadline of their own: a database slow at that moment can still push a node past the drain.
const automationShutdownDrain = WebhookNodeTimeout + 5*time.Second

func (e *AutomationExecutor) drainTimeout() time.Duration {
	if e != nil && e.shutdownDrain > 0 {
		return e.shutdownDrain
	}
	return automationShutdownDrain
}

// afterStepWriteTimeout is how long the writes that record a step go on once the run is
// cancelled (at the end of a shutdown's drain): the step's work is done, and a journey left on the step would run it
// again after the restart. While the run is live they have no deadline of their own, as the
// run's other writes.
const afterStepWriteTimeout = 5 * time.Second

// releaseClaimsTimeout bounds the release of the claims a cancelled batch did not run.
const releaseClaimsTimeout = 3 * time.Second

// writeContext returns a context for the writes that record what a step did: it is not
// cancelled with the run, but afterStepWriteTimeout after the run's context is done, or after
// this call when it is already done.
func writeContext(ctx context.Context) (context.Context, context.CancelFunc) {
	writeCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	stop := context.AfterFunc(ctx, func() {
		time.AfterFunc(afterStepWriteTimeout, cancel)
	})
	return writeCtx, func() {
		stop()
		cancel()
	}
}

// Execute processes a contact through their automation nodes until a delay or completion.
// It loops through multiple nodes in a single tick for efficiency, persisting state after each node.
func (e *AutomationExecutor) Execute(ctx context.Context, workspaceID string, contactAutomation *domain.ContactAutomation) error {
	return e.execute(ctx, nil, workspaceID, contactAutomation)
}

// execute is Execute with a stop signal: once stop is closed, no further node starts. A journey
// still active with no time from the node that finished after the stop is handed back due now
// (a delay or a retry keeps its own time); a journey stopped before its next node keeps the time
// of its last write. A nil stop never fires.
func (e *AutomationExecutor) execute(ctx context.Context, stop <-chan struct{}, workspaceID string, contactAutomation *domain.ContactAutomation) error {
	stopping := func() bool {
		select {
		case <-stop:
			return true
		default:
			return false
		}
	}

	// Get automation once (outside loop)
	automation, err := e.automationRepo.GetByID(ctx, workspaceID, contactAutomation.AutomationID)
	if err != nil {
		return e.handleError(ctx, workspaceID, contactAutomation, err, "failed to get automation")
	}

	// Check if automation is paused/not live
	// When paused, contacts stay frozen at their current node (they don't get exited)
	if automation.Status != domain.AutomationStatusLive {
		return nil
	}

	// No current node: the journey's last step ran and the journey was left active, because
	// its save failed and was retried, or because that step was a delay that has now ended.
	// It ends as that step decided: exited with the reason it gave, else completed.
	if contactAutomation.CurrentNodeID == nil {
		if contactAutomation.ExitReason != nil {
			return e.markAsExited(ctx, workspaceID, contactAutomation, *contactAutomation.ExitReason)
		}
		return e.markAsCompleted(ctx, workspaceID, contactAutomation, "completed")
	}

	// Get contact data once (outside loop) - only if we have nodes to process
	contactData, err := e.contactRepo.GetContactByEmail(ctx, workspaceID, contactAutomation.ContactEmail)
	if err != nil {
		return e.handleError(ctx, workspaceID, contactAutomation, err, "failed to get contact")
	}

	// LOOP: Process nodes until delay, completion, or max iterations
	const maxNodesPerTick = 10
	for iterations := 0; iterations < maxNodesPerTick; iterations++ {
		// Stopping, or a cancelled run: start no node. The journey keeps the time of its last
		// write (its claim or the hand-back below) and resumes then.
		if stopping() || ctx.Err() != nil {
			return nil
		}

		// Get current node from embedded nodes
		node := automation.GetNodeByID(*contactAutomation.CurrentNodeID)
		if node == nil {
			return e.markAsExited(ctx, workspaceID, contactAutomation, "automation_node_deleted")
		}

		// Get executor for node type
		executor, ok := e.nodeExecutors[node.Type]
		if !ok {
			return e.handleError(ctx, workspaceID, contactAutomation,
				fmt.Errorf("unsupported node type: %s", node.Type), "unsupported node type")
		}

		// Create node execution entry (processing)
		nodeExecution := e.createNodeExecution(contactAutomation, node, domain.NodeActionProcessing)
		nodeStartTime := time.Now()
		_ = e.automationRepo.CreateNodeExecution(ctx, workspaceID, nodeExecution)
		// A run cancelled while the row was written runs no step: a step that does not read
		// the context (trigger, delay, A/B split) would otherwise be saved with no row of its
		// own. The row, if written, is closed as a step cut short.
		if ctx.Err() != nil {
			e.recordStepFailure(ctx, workspaceID, nodeExecution, ctx.Err())
			return nil
		}

		// Build context from previous node executions
		executionContext, err := e.buildContextFromNodeExecutions(ctx, workspaceID, contactAutomation.ID)
		if err != nil {
			e.logger.WithField("error", err).Warn("Failed to build context from node executions")
			executionContext = make(map[string]interface{})
		}

		// Execute the node
		params := NodeExecutionParams{
			WorkspaceID:      workspaceID,
			Contact:          contactAutomation,
			Node:             node,
			Automation:       automation,
			ContactData:      contactData,
			ExecutionContext: executionContext,
		}
		result, execErr := executor.Execute(ctx, params)

		if execErr != nil {
			done := e.recordStepFailure(ctx, workspaceID, nodeExecution, execErr)
			if done {
				// The run was cancelled under the step: not a failed attempt. The journey is
				// due again when its claim ends, with no retry spent.
				return nil
			}
			return e.handleError(ctx, workspaceID, contactAutomation, execErr, "node execution failed")
		}

		if stop, err := e.recordStep(ctx, workspaceID, automation, contactAutomation, nodeExecution, nodeStartTime, result, iterations < maxNodesPerTick-1 && !stopping()); stop {
			return err
		}

		// CONTINUE: Process next node immediately
	}

	// Hit max iterations - remaining nodes picked up next tick
	// State already persisted, so this is safe
	return nil
}

// recordStepFailure closes the execution row of a step that returned an error. It reports
// whether the run's context was done, in which case the error is the cancellation and not a
// failed attempt.
func (e *AutomationExecutor) recordStepFailure(ctx context.Context, workspaceID string, nodeExecution *domain.NodeExecution, execErr error) bool {
	nodeExecution.Action = domain.NodeActionFailed
	nodeExecution.Error = strPtr(execErr.Error())
	completedAt := time.Now().UTC()
	nodeExecution.CompletedAt = &completedAt
	writeCtx, cancel := writeContext(ctx)
	defer cancel()
	_ = e.automationRepo.UpdateNodeExecution(writeCtx, workspaceID, nodeExecution)
	return ctx.Err() != nil
}

// recordStep saves what a step that ran decided: the journey's next state, the step's execution
// and, when the journey ended, its stat and end event. These writes run on a context that
// outlives the run. It returns stop when the tick must end, with the error to return.
func (e *AutomationExecutor) recordStep(
	ctx context.Context,
	workspaceID string,
	automation *domain.Automation,
	contactAutomation *domain.ContactAutomation,
	nodeExecution *domain.NodeExecution,
	nodeStartTime time.Time,
	result *NodeExecutionResult,
	holdForThisTick bool,
) (stop bool, err error) {
	writeCtx, cancel := writeContext(ctx)
	defer cancel()

	// Update contact automation state
	contactAutomation.CurrentNodeID = result.NextNodeID
	contactAutomation.ScheduledAt = result.ScheduledAt
	if result.ExitReason != nil {
		contactAutomation.ExitReason = result.ExitReason
	}

	// Determine status (terminal node = completed, unless waiting for a delay)
	isTerminalNode := result.NextNodeID == nil && result.Status == domain.ContactAutomationStatusActive
	isWaitingDelay := result.ScheduledAt != nil && result.ScheduledAt.After(time.Now())
	if isTerminalNode && !isWaitingDelay {
		contactAutomation.Status = domain.ContactAutomationStatusCompleted
	} else {
		contactAutomation.Status = result.Status
	}

	// A contact still active with no time from its node (every node but a delay) must not
	// be persisted with scheduled_at NULL: the scheduler never selects NULL, so a tick that
	// stops before its next persist would strand the journey. The last iteration hands the
	// contact to the next tick, due now; earlier ones hold it with a lease while this tick
	// carries on. Once stopping, the tick does not carry on: it hands the contact back due now.
	if contactAutomation.Status == domain.ContactAutomationStatusActive && result.ScheduledAt == nil {
		resumeAt := time.Now().UTC()
		if holdForThisTick {
			resumeAt = resumeAt.Add(inTickResumeLease)
		}
		contactAutomation.ScheduledAt = &resumeAt
	}

	// The step ran: its execution is completed whatever happens to the journey's save, since
	// the email counters and a delay's waiting state read that row.
	completeStep := func() {
		duration := time.Since(nodeStartTime).Milliseconds()
		nodeExecution.Action = domain.NodeActionCompleted
		completedAt := time.Now().UTC()
		nodeExecution.CompletedAt = &completedAt
		nodeExecution.DurationMs = &duration
		nodeExecution.Output = result.Output
		_ = e.automationRepo.UpdateNodeExecution(writeCtx, workspaceID, nodeExecution)
	}

	// PERSIST STATE (critical for crash recovery). Optimistic lock on
	// status='active' so a concurrent exit (e.g. a stop-on-reply interrupt that
	// landed mid-tick) is not clobbered back to active/next-node.
	updated, err := e.automationRepo.UpdateContactAutomationIfActive(writeCtx, workspaceID, contactAutomation)
	if err != nil {
		completeStep()
		return true, e.retryAfterFailedSave(writeCtx, workspaceID, contactAutomation, result.ScheduledAt, err)
	}
	completeStep()
	if !updated {
		// The journey was exited concurrently; stop processing this tick.
		e.logger.WithField("contact_automation_id", contactAutomation.ID).
			Info("Contact automation no longer active (exited concurrently); aborting tick")
		return true, nil
	}

	// EXIT: Completed (terminal node reached)
	if contactAutomation.Status == domain.ContactAutomationStatusCompleted {
		_ = e.automationRepo.IncrementAutomationStat(writeCtx, workspaceID, automation.ID, "completed")
		e.createAutomationEndEvent(writeCtx, workspaceID, contactAutomation, "completed")
		return true, nil
	}

	// EXIT: Exited (filter/branch exit)
	if contactAutomation.Status == domain.ContactAutomationStatusExited {
		_ = e.automationRepo.IncrementAutomationStat(writeCtx, workspaceID, automation.ID, "exited")
		reason := "exited"
		if contactAutomation.ExitReason != nil {
			reason = *contactAutomation.ExitReason
		}
		e.createAutomationEndEvent(writeCtx, workspaceID, contactAutomation, reason)
		return true, nil
	}

	// EXIT: Delay node (ScheduledAt is in the future)
	if result.ScheduledAt != nil && result.ScheduledAt.After(time.Now()) {
		return true, nil
	}
	return false, nil
}

// retryAfterFailedSave schedules a retry when the journey's save failed after its step ran. The
// retry goes on from the step that comes next, so the step is not run again. A journey that the
// step ended stays active with no current node, so the retry ends it the way the step decided
// (Execute: exited with its reason, else completed). The save failure spends a retry like any
// other (recordFailure): a journey with no retry left fails. No failed execution is written: no
// step failed, and retry_count and last_error record the failure.
func (e *AutomationExecutor) retryAfterFailedSave(ctx context.Context, workspaceID string, ca *domain.ContactAutomation, stepScheduledAt *time.Time, saveErr error) error {
	if ca.Status == domain.ContactAutomationStatusExited && ca.ExitReason == nil {
		reason := "exited"
		ca.ExitReason = &reason
	}
	ca.Status = domain.ContactAutomationStatusActive
	return e.recordFailure(ctx, workspaceID, ca, saveErr, "failed to update contact automation", nil, stepScheduledAt)
}

// ProcessBatch claims a batch of due journeys and runs them one after the other. A batch can
// outlast its claim (a webhook step waits up to WebhookNodeTimeout), after which another instance may claim
// a journey again, so each journey's claim is renewed right before it runs, against the token
// the claim returned: a journey whose claim was lost meanwhile is left to the instance that
// holds it.
//
// ctx is the app's shutdown context. It bounds the claim and stops the batch between two
// journeys, but the journeys run on a context detached from it: a node under way when the stop
// begins has drainTimeout from the stop to finish, and the writes that move the journey past it
// up to afterStepWriteTimeout more. Cutting the node instead would leave the journey on it, and
// the node would run again once the claim ends.
func (e *AutomationExecutor) ProcessBatch(ctx context.Context, limit int) (int, error) {
	contacts, err := e.automationRepo.ClaimScheduledContactAutomationsGlobal(ctx, limit, inTickResumeLease)
	if err != nil {
		if len(contacts) == 0 {
			return 0, fmt.Errorf("failed to get scheduled contacts: %w", err)
		}
		// Some workspaces failed; run what the others gave.
		e.claimErrors.log(e.logger.WithField("error", err.Error()),
			"Failed to claim scheduled contacts in some workspaces")
	}

	if len(contacts) == 0 {
		return 0, nil
	}

	runCtx, cancelRun := context.WithCancel(context.WithoutCancel(ctx))
	defer cancelRun()
	go func() {
		select {
		case <-ctx.Done():
		case <-runCtx.Done():
			return
		}
		timer := time.NewTimer(e.drainTimeout())
		defer timer.Stop()
		select {
		case <-timer.C:
			cancelRun()
		case <-runCtx.Done():
		}
	}()

	processed := 0
	for i, ca := range contacts {
		// Shutting down: the journeys not run yet are handed back, due now.
		if ctx.Err() != nil {
			e.releaseClaims(ctx, contacts[i:])
			break
		}
		fields := map[string]interface{}{
			"contact_email": ca.ContactEmail,
			"automation_id": ca.AutomationID,
			"workspace_id":  ca.WorkspaceID,
		}
		if ca.ScheduledAt == nil {
			e.logger.WithFields(fields).Error("Claimed contact automation has no claim token; skipping")
			continue
		}
		renewed, err := e.automationRepo.RenewContactAutomationClaim(ctx, ca.WorkspaceID, ca.ID, *ca.ScheduledAt, inTickResumeLease)
		if err != nil {
			if ctx.Err() != nil {
				e.releaseClaims(ctx, contacts[i:])
				break
			}
			fields["error"] = err.Error()
			e.logger.WithFields(fields).Error("Failed to renew the claim of a contact automation; skipping")
			continue
		}
		if !renewed {
			e.logger.WithFields(fields).Debug("Contact automation claimed by another instance or ended; skipping")
			continue
		}
		if err := e.execute(runCtx, ctx.Done(), ca.WorkspaceID, &ca.ContactAutomation); err != nil {
			fields["error"] = err.Error()
			e.logger.WithFields(fields).Error("Failed to execute automation for contact")
			// Continue with other contacts
			continue
		}
		processed++
	}

	return processed, nil
}

// releaseClaims hands back the claims of journeys a cancelled batch did not run, so they are
// due now instead of when their lease ends. A journey claimed again since, or ended, is left
// alone; a release that fails leaves the claim to end on its own.
func (e *AutomationExecutor) releaseClaims(ctx context.Context, contacts []*domain.ContactAutomationWithWorkspace) {
	releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), releaseClaimsTimeout)
	defer cancel()
	for _, ca := range contacts {
		if ca.ScheduledAt == nil {
			continue
		}
		if _, err := e.automationRepo.ReleaseContactAutomationClaim(releaseCtx, ca.WorkspaceID, ca.ID, *ca.ScheduledAt); err != nil {
			e.logger.WithFields(map[string]interface{}{
				"contact_automation_id": ca.ID,
				"workspace_id":          ca.WorkspaceID,
				"error":                 err.Error(),
			}).Warn("Failed to release the claim of a contact automation; it is due when its claim ends")
		}
	}
}

// handleError records a failed attempt on the journey's current step and schedules a retry, or
// fails the journey once it has used its retries. When the run's context is done, the error is
// the cancellation (the end of a shutdown's drain), not a failed attempt: nothing is written, no retry is spent,
// and the journey is due again when its claim ends.
func (e *AutomationExecutor) handleError(ctx context.Context, workspaceID string, ca *domain.ContactAutomation, err error, context string) error {
	if ctx.Err() != nil {
		e.logger.WithFields(map[string]interface{}{
			"contact_automation_id": ca.ID,
			"workspace_id":          workspaceID,
			"error":                 err.Error(),
		}).Debug("Automation run cancelled; the journey is due again when its claim ends")
		return nil
	}
	writeCtx, cancel := writeContext(ctx)
	defer cancel()
	return e.recordFailure(writeCtx, workspaceID, ca, err, context, ca.CurrentNodeID, nil)
}

// recordFailure spends a retry: it writes a failed execution on failedNodeID when one is given,
// and either schedules the next attempt, no earlier than notBefore when one is given, or fails
// the journey once RetryCount reaches MaxRetries. The failed stat and end event are written
// only when the journey's row was still active.
func (e *AutomationExecutor) recordFailure(ctx context.Context, workspaceID string, ca *domain.ContactAutomation, err error, context string, failedNodeID *string, notBefore *time.Time) error {
	ca.RetryCount++
	errStr := fmt.Sprintf("%s: %s", context, err.Error())
	ca.LastError = &errStr
	now := time.Now().UTC()
	ca.LastRetryAt = &now

	failed := ca.RetryCount >= ca.MaxRetries
	if failed {
		ca.Status = domain.ContactAutomationStatusFailed
		ca.ScheduledAt = nil

		e.logger.WithFields(map[string]interface{}{
			"contact_email": ca.ContactEmail,
			"automation_id": ca.AutomationID,
			"workspace_id":  workspaceID,
			"retry_count":   ca.RetryCount,
			"error":         errStr,
		}).Error("Automation execution failed after max retries")
	} else {
		// Exponential backoff on the count just incremented: 2 min after the first
		// failure, then 4 min; the journey fails once RetryCount reaches MaxRetries
		// (3 by default). A delay step whose save failed keeps its own, later time.
		backoff := time.Duration(1<<uint(ca.RetryCount)) * time.Minute
		nextRetry := time.Now().UTC().Add(backoff)
		if notBefore != nil && notBefore.After(nextRetry) {
			nextRetry = notBefore.UTC()
		}
		ca.ScheduledAt = &nextRetry

		e.logger.WithFields(map[string]interface{}{
			"contact_email": ca.ContactEmail,
			"automation_id": ca.AutomationID,
			"workspace_id":  workspaceID,
			"retry_count":   ca.RetryCount,
			"next_retry":    nextRetry,
			"error":         errStr,
		}).Warn("Automation execution failed, scheduling retry")
	}

	// Log node execution entry with error
	if failedNodeID != nil {
		entry := &domain.NodeExecution{
			ID:                  uuid.NewString(),
			ContactAutomationID: ca.ID,
			AutomationID:        ca.AutomationID,
			NodeID:              *failedNodeID,
			NodeType:            domain.NodeTypeTrigger, // Placeholder - actual type not available in error context
			Action:              domain.NodeActionFailed,
			EnteredAt:           time.Now().UTC(),
			Error:               &errStr,
		}
		_ = e.automationRepo.CreateNodeExecution(ctx, workspaceID, entry)
	}

	updated, persistErr := e.persistIfActive(ctx, workspaceID, ca)
	if persistErr != nil {
		return persistErr
	}
	if failed && updated {
		_ = e.automationRepo.IncrementAutomationStat(ctx, workspaceID, ca.AutomationID, "failed")
		e.createAutomationEndEvent(ctx, workspaceID, ca, "failed")
	}
	return nil
}

// persistIfActive writes the contact automation only while the row is still active, so a
// concurrent stop-on-reply exit (ExitContactJourneysOnReply flips status to 'exited') is
// never clobbered back to active/failed/completed. A 0-row result means the journey was
// exited concurrently — we leave that exit in place and report false, rather than
// resurrecting it.
func (e *AutomationExecutor) persistIfActive(ctx context.Context, workspaceID string, ca *domain.ContactAutomation) (bool, error) {
	updated, err := e.automationRepo.UpdateContactAutomationIfActive(ctx, workspaceID, ca)
	if err != nil {
		return false, err
	}
	if !updated {
		e.logger.WithField("contact_automation_id", ca.ID).
			Info("Contact automation no longer active (exited concurrently); skipping terminal/retry write")
	}
	return updated, nil
}

// markAsCompleted marks a contact automation as completed. The completed stat and end event are
// written only when the journey's row was still active: one ended meanwhile keeps its own end.
func (e *AutomationExecutor) markAsCompleted(ctx context.Context, workspaceID string, ca *domain.ContactAutomation, reason string) error {
	ca.Status = domain.ContactAutomationStatusCompleted
	ca.ScheduledAt = nil
	ca.ExitReason = &reason

	e.logger.WithFields(map[string]interface{}{
		"contact_email": ca.ContactEmail,
		"automation_id": ca.AutomationID,
		"workspace_id":  workspaceID,
		"reason":        reason,
	}).Info("Contact automation completed")

	updated, err := e.persistIfActive(ctx, workspaceID, ca)
	if err != nil || !updated {
		return err
	}
	_ = e.automationRepo.IncrementAutomationStat(ctx, workspaceID, ca.AutomationID, "completed")
	e.createAutomationEndEvent(ctx, workspaceID, ca, reason)
	return nil
}

// markAsExited marks a contact automation as exited. The exited stat and end event are written
// only when the journey's row was still active: one ended meanwhile keeps its own end.
func (e *AutomationExecutor) markAsExited(ctx context.Context, workspaceID string, ca *domain.ContactAutomation, reason string) error {
	ca.Status = domain.ContactAutomationStatusExited
	ca.ScheduledAt = nil
	ca.ExitReason = &reason

	e.logger.WithFields(map[string]interface{}{
		"contact_email": ca.ContactEmail,
		"automation_id": ca.AutomationID,
		"workspace_id":  workspaceID,
		"reason":        reason,
	}).Info("Contact automation exited")

	updated, err := e.persistIfActive(ctx, workspaceID, ca)
	if err != nil || !updated {
		return err
	}
	_ = e.automationRepo.IncrementAutomationStat(ctx, workspaceID, ca.AutomationID, "exited")
	e.createAutomationEndEvent(ctx, workspaceID, ca, reason)
	return nil
}

// createNodeExecution creates a new node execution entry for logging
func (e *AutomationExecutor) createNodeExecution(ca *domain.ContactAutomation, node *domain.AutomationNode, action domain.NodeAction) *domain.NodeExecution {
	return &domain.NodeExecution{
		ID:                  uuid.NewString(),
		ContactAutomationID: ca.ID,
		AutomationID:        ca.AutomationID,
		NodeID:              node.ID,
		NodeType:            node.Type,
		Action:              action,
		EnteredAt:           time.Now().UTC(),
		Output:              make(map[string]interface{}),
	}
}

// buildContextFromNodeExecutions reconstructs context from completed node executions
// This allows nodes to access data from previous nodes in the workflow
func (e *AutomationExecutor) buildContextFromNodeExecutions(ctx context.Context, workspaceID, contactAutomationID string) (map[string]interface{}, error) {
	entries, err := e.automationRepo.GetNodeExecutions(ctx, workspaceID, contactAutomationID)
	if err != nil {
		return nil, err
	}

	result := make(map[string]interface{})
	for _, entry := range entries {
		if entry.Action == domain.NodeActionCompleted && entry.Output != nil {
			result[entry.NodeID] = entry.Output
		}
	}
	return result, nil
}

// createAutomationEndEvent creates an automation.end timeline event when a contact exits an automation
func (e *AutomationExecutor) createAutomationEndEvent(ctx context.Context, workspaceID string, ca *domain.ContactAutomation, exitReason string) {
	entry := &domain.ContactTimelineEntry{
		Email:      ca.ContactEmail,
		Operation:  "update",
		EntityType: "automation",
		Kind:       "automation.end",
		EntityID:   &ca.AutomationID,
		Changes: map[string]interface{}{
			"automation_id": map[string]interface{}{"new": ca.AutomationID},
			"exit_reason":   map[string]interface{}{"new": exitReason},
			"status":        map[string]interface{}{"new": string(ca.Status)},
		},
		CreatedAt: time.Now().UTC(),
	}
	if err := e.timelineRepo.Create(ctx, workspaceID, entry); err != nil {
		e.logger.WithFields(map[string]interface{}{
			"contact_email": ca.ContactEmail,
			"automation_id": ca.AutomationID,
			"error":         err.Error(),
		}).Warn("Failed to create automation.end timeline event")
	}
}
