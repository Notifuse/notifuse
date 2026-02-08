package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// IntegrationSyncHandler defines the interface for integration-specific sync handlers
type IntegrationSyncHandler interface {
	// Sync performs the sync operation for the integration
	// Returns the number of events/records synced and any error
	Sync(ctx context.Context, workspaceID, integrationID string, state *domain.IntegrationSyncState) (int, error)
}

// IntegrationSyncProcessor handles the execution of integration sync tasks
type IntegrationSyncProcessor struct {
	logger   logger.Logger
	handlers map[string]IntegrationSyncHandler
}

// NewIntegrationSyncProcessor creates a new integration sync processor
func NewIntegrationSyncProcessor(logger logger.Logger) *IntegrationSyncProcessor {
	return &IntegrationSyncProcessor{
		logger:   logger,
		handlers: make(map[string]IntegrationSyncHandler),
	}
}

// RegisterHandler registers a handler for a specific integration type
func (p *IntegrationSyncProcessor) RegisterHandler(integrationType string, handler IntegrationSyncHandler) {
	p.handlers[integrationType] = handler
}

// CanProcess returns whether this processor can handle the given task type
func (p *IntegrationSyncProcessor) CanProcess(taskType string) bool {
	return taskType == "sync_integration"
}

// Process executes an integration sync task
func (p *IntegrationSyncProcessor) Process(ctx context.Context, task *domain.Task, timeoutAt time.Time) (completed bool, err error) {
	p.logger.WithFields(map[string]interface{}{
		"task_id":      task.ID,
		"workspace_id": task.WorkspaceID,
		"type":         task.Type,
	}).Info("Processing integration sync task")

	// Validate task state
	if task.State == nil || task.State.IntegrationSync == nil {
		return false, fmt.Errorf("task missing IntegrationSync state")
	}

	state := task.State.IntegrationSync

	if state.IntegrationType == "" {
		return false, fmt.Errorf("task missing IntegrationType in state")
	}

	// Update last sync attempt time
	now := time.Now().UTC()
	state.LastSyncAt = &now

	// Get the handler for this integration type
	handler, ok := p.handlers[state.IntegrationType]
	if !ok {
		// No handler registered - this is a placeholder until specific integrations are implemented
		p.logger.WithFields(map[string]interface{}{
			"integration_type": state.IntegrationType,
			"integration_id":   state.IntegrationID,
		}).Info("No sync handler registered for integration type, skipping")

		// Return completed=true so the recurring task reschedules
		// Once handlers are registered, actual sync will happen
		return true, nil
	}

	// Execute the sync
	eventCount, syncErr := handler.Sync(ctx, task.WorkspaceID, state.IntegrationID, state)

	if syncErr != nil {
		// Classify the error
		p.updateSyncStateError(state, syncErr)

		// Check if we've exceeded max consecutive errors (10 by default)
		maxConsecErrors := 10
		if state.ConsecErrors >= maxConsecErrors {
			// Too many errors - fail the task permanently
			p.logger.WithFields(map[string]interface{}{
				"integration_id":   state.IntegrationID,
				"integration_type": state.IntegrationType,
				"consec_errors":    state.ConsecErrors,
				"error":            syncErr.Error(),
			}).Error("Integration sync failed permanently after too many consecutive errors")

			return false, fmt.Errorf("integration sync failed after %d consecutive errors: %w", maxConsecErrors, syncErr)
		}

		// Transient or unknown error - let the task reschedule with backoff
		if state.LastErrorType == domain.ErrorTypePermanent {
			// Permanent error - fail immediately
			p.logger.WithFields(map[string]interface{}{
				"integration_id":   state.IntegrationID,
				"integration_type": state.IntegrationType,
				"error":            syncErr.Error(),
				"error_type":       state.LastErrorType,
			}).Error("Integration sync failed with permanent error")

			return false, fmt.Errorf("integration sync failed with permanent error: %w", syncErr)
		}

		// Transient error - complete true to reschedule with backoff
		p.logger.WithFields(map[string]interface{}{
			"integration_id":   state.IntegrationID,
			"integration_type": state.IntegrationType,
			"error":            syncErr.Error(),
			"error_type":       state.LastErrorType,
			"consec_errors":    state.ConsecErrors,
		}).Warn("Integration sync failed with transient error, will retry")

		return true, nil
	}

	// Success - update state
	p.updateSyncStateSuccess(state, eventCount)

	p.logger.WithFields(map[string]interface{}{
		"integration_id":   state.IntegrationID,
		"integration_type": state.IntegrationType,
		"events_synced":    eventCount,
		"total_imported":   state.EventsImported,
	}).Info("Integration sync completed successfully")

	return true, nil
}

// updateSyncStateSuccess updates the sync state after a successful sync
func (p *IntegrationSyncProcessor) updateSyncStateSuccess(state *domain.IntegrationSyncState, eventCount int) {
	now := time.Now().UTC()
	state.LastSuccessAt = &now
	state.LastEventCount = eventCount
	state.EventsImported += int64(eventCount)
	state.ConsecErrors = 0
	state.LastError = nil
	state.LastErrorType = ""
}

// updateSyncStateError updates the sync state after a failed sync
func (p *IntegrationSyncProcessor) updateSyncStateError(state *domain.IntegrationSyncState, err error) {
	state.ConsecErrors++
	errStr := err.Error()
	state.LastError = &errStr
	state.LastErrorType = classifyError(err)
}

// classifyError classifies an error as transient, permanent, or unknown
func classifyError(err error) string {
	if err == nil {
		return domain.ErrorTypeUnknown
	}

	errLower := strings.ToLower(err.Error())

	// Transient errors - can be retried
	transientPatterns := []string{
		"timeout",
		"rate limit",
		"rate limited",
		"429",
		"503",
		"502",
		"temporary",
		"connection refused",
		"connection reset",
		"network unreachable",
		"no such host",
		"eof",
		"service unavailable",
		"bad gateway",
		"try again",
		"retry",
	}

	for _, pattern := range transientPatterns {
		if strings.Contains(errLower, pattern) {
			return domain.ErrorTypeTransient
		}
	}

	// Permanent errors - should not be retried
	permanentPatterns := []string{
		"invalid api key",
		"invalid credentials",
		"authentication failed",
		"401",
		"403",
		"forbidden",
		"unauthorized",
		"access denied",
		"permission denied",
		"disabled",
		"suspended",
		"revoked",
		"expired",
		"invalid token",
	}

	for _, pattern := range permanentPatterns {
		if strings.Contains(errLower, pattern) {
			return domain.ErrorTypePermanent
		}
	}

	return domain.ErrorTypeUnknown
}
