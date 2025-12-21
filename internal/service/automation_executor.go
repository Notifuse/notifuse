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
	emailService    domain.EmailServiceInterface
	messageRepo     domain.MessageHistoryRepository
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
	emailService domain.EmailServiceInterface,
	messageRepo domain.MessageHistoryRepository,
	log logger.Logger,
	apiEndpoint string,
) *AutomationExecutor {
	qb := NewQueryBuilder()

	executors := map[domain.NodeType]NodeExecutor{
		domain.NodeTypeDelay:          NewDelayNodeExecutor(),
		domain.NodeTypeEmail:          NewEmailNodeExecutor(emailService, workspaceRepo, apiEndpoint, log),
		domain.NodeTypeBranch:         NewBranchNodeExecutor(qb, workspaceRepo),
		domain.NodeTypeFilter:         NewFilterNodeExecutor(qb, workspaceRepo),
		domain.NodeTypeAddToList:      NewAddToListNodeExecutor(contactListRepo),
		domain.NodeTypeRemoveFromList: NewRemoveFromListNodeExecutor(contactListRepo),
		domain.NodeTypeABTest:         NewABTestNodeExecutor(),
		domain.NodeTypeWebhook:        NewWebhookNodeExecutor(log),
	}

	return &AutomationExecutor{
		automationRepo:  automationRepo,
		contactRepo:     contactRepo,
		workspaceRepo:   workspaceRepo,
		contactListRepo: contactListRepo,
		templateRepo:    templateRepo,
		emailService:    emailService,
		messageRepo:     messageRepo,
		nodeExecutors:   executors,
		logger:          log,
		apiEndpoint:     apiEndpoint,
	}
}

// Execute processes a single contact through their current node
func (e *AutomationExecutor) Execute(ctx context.Context, workspaceID string, contactAutomation *domain.ContactAutomation) error {
	startTime := time.Now()

	// 1. Get automation (includes embedded nodes)
	automation, err := e.automationRepo.GetByID(ctx, workspaceID, contactAutomation.AutomationID)
	if err != nil {
		return e.handleError(ctx, workspaceID, contactAutomation, err, "failed to get automation")
	}

	// Check automation is still active
	// Note: This is a safety check - scheduler should already filter by automation status
	// When paused, contacts stay frozen at their current node (they don't get exited)
	if automation.Status != domain.AutomationStatusLive {
		// Don't exit - just skip processing. Contact stays at current node.
		return nil
	}

	// 2. Get current node from embedded nodes
	if contactAutomation.CurrentNodeID == nil {
		return e.markAsCompleted(ctx, workspaceID, contactAutomation, "completed")
	}

	node := automation.GetNodeByID(*contactAutomation.CurrentNodeID)
	if node == nil {
		// Node was deleted while contact was waiting - exit gracefully
		return e.markAsExited(ctx, workspaceID, contactAutomation, "automation_node_deleted")
	}

	// 3. Get node executor
	executor, ok := e.nodeExecutors[node.Type]
	if !ok {
		return e.handleError(ctx, workspaceID, contactAutomation,
			fmt.Errorf("unsupported node type: %s", node.Type), "unsupported node type")
	}

	// 4. Get contact data for template rendering
	contact, err := e.contactRepo.GetContactByEmail(ctx, workspaceID, contactAutomation.ContactEmail)
	if err != nil {
		return e.handleError(ctx, workspaceID, contactAutomation, err, "failed to get contact")
	}

	// 5. Create node execution entry (processing)
	nodeExecution := e.createNodeExecution(contactAutomation, node, domain.NodeActionProcessing)
	_ = e.automationRepo.CreateNodeExecution(ctx, workspaceID, nodeExecution)

	// 5.5 Build execution context from previous node executions
	executionContext, err := e.buildContextFromNodeExecutions(ctx, workspaceID, contactAutomation.ID)
	if err != nil {
		e.logger.WithField("error", err).Warn("Failed to build context from node executions")
		executionContext = make(map[string]interface{})
	}

	// 6. Execute node
	params := NodeExecutionParams{
		WorkspaceID:      workspaceID,
		Contact:          contactAutomation,
		Node:             node,
		Automation:       automation,
		ContactData:      contact,
		ExecutionContext: executionContext,
	}

	result, err := executor.Execute(ctx, params)
	if err != nil {
		// Update node execution entry with error
		nodeExecution.Action = domain.NodeActionFailed
		nodeExecution.Error = strPtr(err.Error())
		completedAt := time.Now().UTC()
		nodeExecution.CompletedAt = &completedAt
		_ = e.automationRepo.UpdateNodeExecution(ctx, workspaceID, nodeExecution)

		return e.handleError(ctx, workspaceID, contactAutomation, err, "node execution failed")
	}

	// 7. Update contact automation
	contactAutomation.CurrentNodeID = result.NextNodeID
	contactAutomation.ScheduledAt = result.ScheduledAt

	// If there's no next node, mark as completed (terminal node behavior)
	if result.NextNodeID == nil && result.Status == domain.ContactAutomationStatusActive {
		contactAutomation.Status = domain.ContactAutomationStatusCompleted
	} else {
		contactAutomation.Status = result.Status
	}
	// Note: Context is now reconstructed from node executions on demand,
	// no longer stored in contact_automations.context

	err = e.automationRepo.UpdateContactAutomation(ctx, workspaceID, contactAutomation)
	if err != nil {
		return e.handleError(ctx, workspaceID, contactAutomation, err, "failed to update contact automation")
	}

	// 8. Update node execution entry (completed)
	duration := time.Since(startTime).Milliseconds()
	nodeExecution.Action = domain.NodeActionCompleted
	completedAt := time.Now().UTC()
	nodeExecution.CompletedAt = &completedAt
	nodeExecution.DurationMs = &duration
	nodeExecution.Output = result.Output
	_ = e.automationRepo.UpdateNodeExecution(ctx, workspaceID, nodeExecution)

	// 9. Update automation stats if status changed
	if contactAutomation.Status == domain.ContactAutomationStatusCompleted {
		_ = e.automationRepo.IncrementAutomationStat(ctx, workspaceID, automation.ID, "completed")
	} else if contactAutomation.Status == domain.ContactAutomationStatusExited {
		_ = e.automationRepo.IncrementAutomationStat(ctx, workspaceID, automation.ID, "exited")
	}

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

	return e.automationRepo.UpdateContactAutomation(ctx, workspaceID, ca)
}

// createNodeExecution creates a new node execution entry for logging
func (e *AutomationExecutor) createNodeExecution(ca *domain.ContactAutomation, node *domain.AutomationNode, action domain.NodeAction) *domain.NodeExecution {
	return &domain.NodeExecution{
		ID:                  uuid.NewString(),
		ContactAutomationID: ca.ID,
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

