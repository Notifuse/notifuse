package service

import (
	"context"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/Notifuse/notifuse/pkg/mailer"
)

// SystemNotificationService handles system-wide notifications and alerts
type SystemNotificationService struct {
	workspaceRepo domain.WorkspaceRepository
	broadcastRepo domain.BroadcastRepository
	mailer        mailer.Mailer
	logger        logger.Logger
}

// NewSystemNotificationService creates a new system notification service
func NewSystemNotificationService(
	workspaceRepo domain.WorkspaceRepository,
	broadcastRepo domain.BroadcastRepository,
	mailerInstance mailer.Mailer,
	logger logger.Logger,
) *SystemNotificationService {
	return &SystemNotificationService{
		workspaceRepo: workspaceRepo,
		broadcastRepo: broadcastRepo,
		mailer:        mailerInstance,
		logger:        logger,
	}
}

// HandleCircuitBreakerEvent processes circuit breaker events and sends email notifications
func (s *SystemNotificationService) HandleCircuitBreakerEvent(ctx context.Context, payload domain.EventPayload) {
	s.logger.WithFields(map[string]interface{}{
		"event_type":   payload.Type,
		"workspace_id": payload.WorkspaceID,
		"entity_id":    payload.EntityID,
	}).Info("Processing circuit breaker event")

	// Extract broadcast ID and reason from event data
	broadcastID, ok := payload.Data["broadcast_id"].(string)
	if !ok || broadcastID == "" {
		s.logger.WithFields(map[string]interface{}{
			"event_type":   payload.Type,
			"workspace_id": payload.WorkspaceID,
			"entity_id":    payload.EntityID,
		}).Error("Circuit breaker event missing broadcast_id")
		return
	}

	reason, ok := payload.Data["reason"].(string)
	if !ok || reason == "" {
		s.logger.WithFields(map[string]interface{}{
			"event_type":   payload.Type,
			"workspace_id": payload.WorkspaceID,
			"broadcast_id": broadcastID,
		}).Error("Circuit breaker event missing reason")
		return
	}

	// Get broadcast details
	broadcast, err := s.broadcastRepo.GetBroadcast(ctx, payload.WorkspaceID, broadcastID)
	if err != nil {
		s.logger.WithFields(map[string]interface{}{
			"event_type":   payload.Type,
			"workspace_id": payload.WorkspaceID,
			"broadcast_id": broadcastID,
			"error":        err.Error(),
		}).Error("Failed to get broadcast for circuit breaker notification")
		return
	}

	if broadcast == nil {
		s.logger.WithFields(map[string]interface{}{
			"event_type":   payload.Type,
			"workspace_id": payload.WorkspaceID,
			"broadcast_id": broadcastID,
		}).Error("Broadcast not found for circuit breaker notification")
		return
	}

	// Get workspace details
	workspace, err := s.workspaceRepo.GetByID(ctx, payload.WorkspaceID)
	if err != nil {
		s.logger.WithFields(map[string]interface{}{
			"event_type":   payload.Type,
			"workspace_id": payload.WorkspaceID,
			"broadcast_id": broadcastID,
			"error":        err.Error(),
		}).Error("Failed to get workspace for circuit breaker notification")
		return
	}

	if workspace == nil {
		s.logger.WithFields(map[string]interface{}{
			"event_type":   payload.Type,
			"workspace_id": payload.WorkspaceID,
			"broadcast_id": broadcastID,
		}).Error("Workspace not found for circuit breaker notification")
		return
	}

	// Send notification to workspace owners
	err = s.notifyWorkspaceOwners(ctx, payload.WorkspaceID, func(ownerEmail string) error {
		return s.mailer.SendCircuitBreakerAlert(ownerEmail, workspace.Name, broadcast.Name, reason)
	})

	if err != nil {
		s.logger.WithFields(map[string]interface{}{
			"event_type":   payload.Type,
			"workspace_id": payload.WorkspaceID,
			"broadcast_id": broadcastID,
			"error":        err.Error(),
		}).Error("Failed to send circuit breaker notifications to workspace owners")
	}
}

// HandleBroadcastFailedEvent processes broadcast failure events (placeholder for future use)
func (s *SystemNotificationService) HandleBroadcastFailedEvent(ctx context.Context, payload domain.EventPayload) {
	s.logger.WithFields(map[string]interface{}{
		"event_type":   payload.Type,
		"workspace_id": payload.WorkspaceID,
		"entity_id":    payload.EntityID,
	}).Info("Processing broadcast failed event")

	// Future implementation for broadcast failure notifications
	// This could send different types of alerts for various failure reasons
}

// HandleSystemAlert processes generic system alerts (placeholder for future use)
func (s *SystemNotificationService) HandleSystemAlert(ctx context.Context, payload domain.EventPayload) {
	s.logger.WithFields(map[string]interface{}{
		"event_type":   payload.Type,
		"workspace_id": payload.WorkspaceID,
		"entity_id":    payload.EntityID,
	}).Info("Processing system alert event")

	// Future implementation for generic system alerts
	// This could handle various system-wide notifications
}

// notifyWorkspaceOwners is a helper function that sends notifications to all workspace owners
func (s *SystemNotificationService) notifyWorkspaceOwners(ctx context.Context, workspaceID string, notificationFunc func(string) error) error {
	// Get workspace owners with email
	workspaceUsers, err := s.workspaceRepo.GetWorkspaceUsersWithEmail(ctx, workspaceID)
	if err != nil {
		return err
	}

	// Find workspace owners and send notifications
	ownerCount := 0
	var lastError error

	for _, user := range workspaceUsers {
		if user.Role == "owner" && user.Email != "" {
			ownerCount++
			
			// Send notification using the provided function
			if s.mailer != nil {
				emailErr := notificationFunc(user.Email)
				if emailErr != nil {
					s.logger.WithFields(map[string]interface{}{
						"workspace_id": workspaceID,
						"owner_email":  user.Email,
						"error":        emailErr.Error(),
					}).Error("Failed to send notification to workspace owner")
					lastError = emailErr
				} else {
					s.logger.WithFields(map[string]interface{}{
						"workspace_id": workspaceID,
						"owner_email":  user.Email,
					}).Info("Notification sent successfully to workspace owner")
				}
			} else {
				s.logger.WithFields(map[string]interface{}{
					"workspace_id": workspaceID,
				}).Warn("Cannot send notification - mailer not available")
			}
		}
	}

	if ownerCount == 0 {
		s.logger.WithFields(map[string]interface{}{
			"workspace_id": workspaceID,
		}).Warn("No workspace owners with email found for notification")
	}

	return lastError
}

// RegisterWithEventBus registers the service to listen for various system events
func (s *SystemNotificationService) RegisterWithEventBus(eventBus domain.EventBus) {
	// Register for circuit breaker events
	eventBus.Subscribe(domain.EventBroadcastCircuitBreaker, s.HandleCircuitBreakerEvent)
	
	// Register for broadcast failure events (for future use)
	eventBus.Subscribe(domain.EventBroadcastFailed, s.HandleBroadcastFailedEvent)
	
	// Future: Add more event subscriptions as needed
	// eventBus.Subscribe(domain.EventSystemAlert, s.HandleSystemAlert)
	
	s.logger.Info("System notification service registered with event bus")
}
