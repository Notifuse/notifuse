package service

import (
	"context"
	"fmt"
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
	messageRepo     domain.MessageHistoryRepository
	timelineRepo    domain.ContactTimelineRepository
	nodeExecutors   map[domain.NodeType]NodeExecutor
	logger          logger.Logger
	apiEndpoint     string
}

// NewAutomationExecutor creates a new AutomationExecutor
func NewAutomationExecutor(
	automationRepo domain.AutomationRepository,
	contactRepo domain.ContactRepository,
	workspaceRepo domain.WorkspaceRepository,
	contactListRepo domain.ContactListRepository,
	templateRepo domain.TemplateRepository,
	emailQueueRepo domain.EmailQueueRepository,
	messageRepo domain.MessageHistoryRepository,
	timelineRepo domain.ContactTimelineRepository,
	log logger.Logger,
	apiEndpoint string,
) *AutomationExecutor {
	qb := NewQueryBuilder()

	executors := map[domain.NodeType]NodeExecutor{
		domain.NodeTypeTrigger:          NewTriggerNodeExecutor(),
		domain.NodeTypeDelay:            NewDelayNodeExecutor(),
		domain.NodeTypeEmail:            NewEmailNodeExecutor(emailQueueRepo, templateRepo, workspaceRepo, apiEndpoint, log),
		domain.NodeTypeBranch:           NewBranchNodeExecutor(qb, workspaceRepo),
		domain.NodeTypeFilter:           NewFilterNodeExecutor(qb, workspaceRepo),
		domain.NodeTypeAddToList:        NewAddToListNodeExecutor(contactListRepo),
		domain.NodeTypeRemoveFromList:   NewRemoveFromListNodeExecutor(contactListRepo),
		domain.NodeTypeABTest:           NewABTestNodeExecutor(),
		domain.NodeTypeWebhook:          NewWebhookNodeExecutor(log),
		domain.NodeTypeListStatusBranch: NewListStatusBranchNodeExecutor(contactListRepo),
	}

	return &AutomationExecutor{
		automationRepo:  automationRepo,
		contactRepo:     contactRepo,
		workspaceRepo:   workspaceRepo,
		contactListRepo: contactListRepo,
		templateRepo:    templateRepo,
		emailQueueRepo:  emailQueueRepo,
		messageRepo:     messageRepo,
		timelineRepo:    timelineRepo,
		nodeExecutors:   executors,
		logger:          log,
		apiEndpoint:     apiEndpoint,
	}
}

// Execute processes a contact through their automation nodes until a delay or completion.
// It loops through multiple nodes in a single tick for efficiency, persisting state after each node.
func (e *AutomationExecutor) Execute(ctx context.Context, workspaceID string, contactAutomation *domain.ContactAutomation) error {
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

	// Early exit if already completed (no current node) - avoid fetching contact unnecessarily
	if contactAutomation.CurrentNodeID == nil {
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

		// Handle execution error
		if execErr != nil {
			nodeExecution.Action = domain.NodeActionFailed
			nodeExecution.Error = strPtr(execErr.Error())
			completedAt := time.Now().UTC()
			nodeExecution.CompletedAt = &completedAt
			_ = e.automationRepo.UpdateNodeExecution(ctx, workspaceID, nodeExecution)
			return e.handleError(ctx, workspaceID, contactAutomation, execErr, "node execution failed")
		}

		// Update contact automation state
		contactAutomation.CurrentNodeID = result.NextNodeID
		contactAutomation.ScheduledAt = result.ScheduledAt

		// Determine status (terminal node = completed, unless waiting for a delay)
		isTerminalNode := result.NextNodeID == nil && result.Status == domain.ContactAutomationStatusActive
		isWaitingDelay := result.ScheduledAt != nil && result.ScheduledAt.After(time.Now())
		if isTerminalNode && !isWaitingDelay {
			contactAutomation.Status = domain.ContactAutomationStatusCompleted
		} else {
			contactAutomation.Status = result.Status
		}

		// PERSIST STATE (critical for crash recovery)
		if err := e.automationRepo.UpdateContactAutomation(ctx, workspaceID, contactAutomation); err != nil {
			return e.handleError(ctx, workspaceID, contactAutomation, err, "failed to update contact automation")
		}

		// Update node execution to completed
		duration := time.Since(nodeStartTime).Milliseconds()
		nodeExecution.Action = domain.NodeActionCompleted
		completedAt := time.Now().UTC()
		nodeExecution.CompletedAt = &completedAt
		nodeExecution.DurationMs = &duration
		nodeExecution.Output = result.Output
		_ = e.automationRepo.UpdateNodeExecution(ctx, workspaceID, nodeExecution)

		// EXIT: Completed (terminal node reached)
		if contactAutomation.Status == domain.ContactAutomationStatusCompleted {
			_ = e.automationRepo.IncrementAutomationStat(ctx, workspaceID, automation.ID, "completed")
			e.createAutomationEndEvent(ctx, workspaceID, contactAutomation, "completed")
			return nil
		}

		// EXIT: Exited (filter/branch exit)
		if contactAutomation.Status == domain.ContactAutomationStatusExited {
			_ = e.automationRepo.IncrementAutomationStat(ctx, workspaceID, automation.ID, "exited")
			reason := "exited"
			if contactAutomation.ExitReason != nil {
				reason = *contactAutomation.ExitReason
			}
			e.createAutomationEndEvent(ctx, workspaceID, contactAutomation, reason)
			return nil
		}

		// EXIT: Delay node (ScheduledAt is in the future)
		if result.ScheduledAt != nil && result.ScheduledAt.After(time.Now()) {
			return nil
		}

		// CONTINUE: Process next node immediately
	}

	// Hit max iterations - remaining nodes picked up next tick
	// State already persisted, so this is safe
	return nil
}

// ProcessBatch processes a batch of scheduled contacts
func (e *AutomationExecutor) ProcessBatch(ctx context.Context, limit int) (int, error) {
	// Get scheduled contacts globally
	contacts, err := e.automationRepo.GetScheduledContactAutomationsGlobal(ctx, time.Now().UTC(), limit)
	if err != nil {
		return 0, fmt.Errorf("failed to get scheduled contacts: %w", err)
	}

	if len(contacts) == 0 {
		return 0, nil
	}

	processed := 0
	for _, ca := range contacts {
		if err := e.Execute(ctx, ca.WorkspaceID, &ca.ContactAutomation); err != nil {
			e.logger.WithFields(map[string]interface{}{
				"contact_email": ca.ContactEmail,
				"automation_id": ca.AutomationID,
				"workspace_id":  ca.WorkspaceID,
				"error":         err.Error(),
			}).Error("Failed to execute automation for contact")
			// Continue with other contacts
			continue
		}
		processed++
	}

	return processed, nil
}

// handleError handles an error during execution by updating retry count and status
func (e *AutomationExecutor) handleError(ctx context.Context, workspaceID string, ca *domain.ContactAutomation, err error, context string) error {
	ca.RetryCount++
	errStr := fmt.Sprintf("%s: %s", context, err.Error())
	ca.LastError = &errStr
	now := time.Now().UTC()
	ca.LastRetryAt = &now

	if ca.RetryCount >= ca.MaxRetries {
		ca.Status = domain.ContactAutomationStatusFailed
		_ = e.automationRepo.IncrementAutomationStat(ctx, workspaceID, ca.AutomationID, "failed")

		e.createAutomationEndEvent(ctx, workspaceID, ca, "failed")

		e.logger.WithFields(map[string]interface{}{
			"contact_email": ca.ContactEmail,
			"automation_id": ca.AutomationID,
			"workspace_id":  workspaceID,
			"retry_count":   ca.RetryCount,
			"error":         errStr,
		}).Error("Automation execution failed after max retries")
	} else {
		// Exponential backoff: 1min, 2min, 4min, etc.
		backoff := time.Duration(1<<uint(ca.RetryCount)) * time.Minute
		nextRetry := time.Now().UTC().Add(backoff)
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
	if ca.CurrentNodeID != nil {
		entry := &domain.NodeExecution{
			ID:                  uuid.NewString(),
			ContactAutomationID: ca.ID,
			AutomationID:        ca.AutomationID,
			NodeID:              *ca.CurrentNodeID,
			NodeType:            domain.NodeTypeTrigger, // Placeholder - actual type not available in error context
			Action:              domain.NodeActionFailed,
			EnteredAt:           time.Now().UTC(),
			Error:               &errStr,
		}
		_ = e.automationRepo.CreateNodeExecution(ctx, workspaceID, entry)
	}

	return e.automationRepo.UpdateContactAutomation(ctx, workspaceID, ca)
}

// markAsCompleted marks a contact automation as completed
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

	_ = e.automationRepo.IncrementAutomationStat(ctx, workspaceID, ca.AutomationID, "completed")

	e.createAutomationEndEvent(ctx, workspaceID, ca, reason)

	return e.automationRepo.UpdateContactAutomation(ctx, workspaceID, ca)
}

// markAsExited marks a contact automation as exited
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

	_ = e.automationRepo.IncrementAutomationStat(ctx, workspaceID, ca.AutomationID, "exited")

	e.createAutomationEndEvent(ctx, workspaceID, ca, reason)

	return e.automationRepo.UpdateContactAutomation(ctx, workspaceID, ca)
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
