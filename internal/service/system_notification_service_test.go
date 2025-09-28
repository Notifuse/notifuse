package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestNewSystemNotificationService(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockBroadcastRepo := mocks.NewMockBroadcastRepository(ctrl)
	mockMailer := pkgmocks.NewMockMailer(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	service := NewSystemNotificationService(
		mockWorkspaceRepo,
		mockBroadcastRepo,
		mockMailer,
		mockLogger,
	)

	assert.NotNil(t, service, "Service should not be nil")
	assert.Equal(t, mockWorkspaceRepo, service.workspaceRepo)
	assert.Equal(t, mockBroadcastRepo, service.broadcastRepo)
	assert.Equal(t, mockMailer, service.mailer)
	assert.Equal(t, mockLogger, service.logger)
}

func TestSystemNotificationService_HandleCircuitBreakerEvent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockBroadcastRepo := mocks.NewMockBroadcastRepository(ctrl)
	mockMailer := pkgmocks.NewMockMailer(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	service := NewSystemNotificationService(
		mockWorkspaceRepo,
		mockBroadcastRepo,
		mockMailer,
		mockLogger,
	)

	ctx := context.Background()

	t.Run("Success - Circuit breaker event processed", func(t *testing.T) {
		payload := domain.EventPayload{
			Type:        "circuit_breaker",
			WorkspaceID: "workspace-123",
			EntityID:    "broadcast-456",
			Data: map[string]interface{}{
				"broadcast_id": "broadcast-456",
				"reason":       "High bounce rate detected",
			},
		}

		broadcast := &domain.Broadcast{
			ID:   "broadcast-456",
			Name: "Test Broadcast",
		}

		workspace := &domain.Workspace{
			ID:   "workspace-123",
			Name: "Test Workspace",
		}

		workspaceUsers := []*domain.UserWorkspaceWithEmail{
			{
				UserWorkspace: domain.UserWorkspace{
					UserID:      "user-1",
					WorkspaceID: "workspace-123",
					Role:        "owner",
				},
				Email: "owner@example.com",
			},
		}

		// Setup expectations
		mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger)
		mockLogger.EXPECT().Info("Processing circuit breaker event")

		mockBroadcastRepo.EXPECT().GetBroadcast(ctx, "workspace-123", "broadcast-456").Return(broadcast, nil)
		mockWorkspaceRepo.EXPECT().GetByID(ctx, "workspace-123").Return(workspace, nil)
		mockWorkspaceRepo.EXPECT().GetWorkspaceUsersWithEmail(ctx, "workspace-123").Return(workspaceUsers, nil)

		mockMailer.EXPECT().SendCircuitBreakerAlert("owner@example.com", "Test Workspace", "Test Broadcast", "High bounce rate detected").Return(nil)

		mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger)
		mockLogger.EXPECT().Info("Notification sent successfully to workspace owner")

		// Execute
		service.HandleCircuitBreakerEvent(ctx, payload)
	})

	t.Run("Error - Missing broadcast_id", func(t *testing.T) {
		payload := domain.EventPayload{
			Type:        "circuit_breaker",
			WorkspaceID: "workspace-123",
			EntityID:    "broadcast-456",
			Data: map[string]interface{}{
				"reason": "High bounce rate detected",
				// Missing broadcast_id
			},
		}

		mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger)
		mockLogger.EXPECT().Info("Processing circuit breaker event")

		mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger)
		mockLogger.EXPECT().Error("Circuit breaker event missing broadcast_id")

		service.HandleCircuitBreakerEvent(ctx, payload)
	})

	t.Run("Error - Missing reason", func(t *testing.T) {
		payload := domain.EventPayload{
			Type:        "circuit_breaker",
			WorkspaceID: "workspace-123",
			EntityID:    "broadcast-456",
			Data: map[string]interface{}{
				"broadcast_id": "broadcast-456",
				// Missing reason
			},
		}

		mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger)
		mockLogger.EXPECT().Info("Processing circuit breaker event")

		mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger)
		mockLogger.EXPECT().Error("Circuit breaker event missing reason")

		service.HandleCircuitBreakerEvent(ctx, payload)
	})

	t.Run("Error - Broadcast not found", func(t *testing.T) {
		payload := domain.EventPayload{
			Type:        "circuit_breaker",
			WorkspaceID: "workspace-123",
			EntityID:    "broadcast-456",
			Data: map[string]interface{}{
				"broadcast_id": "broadcast-456",
				"reason":       "High bounce rate detected",
			},
		}

		mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger)
		mockLogger.EXPECT().Info("Processing circuit breaker event")

		mockBroadcastRepo.EXPECT().GetBroadcast(ctx, "workspace-123", "broadcast-456").Return(nil, errors.New("broadcast not found"))

		mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger)
		mockLogger.EXPECT().Error("Failed to get broadcast for circuit breaker notification")

		service.HandleCircuitBreakerEvent(ctx, payload)
	})

	t.Run("Error - Workspace not found", func(t *testing.T) {
		payload := domain.EventPayload{
			Type:        "circuit_breaker",
			WorkspaceID: "workspace-123",
			EntityID:    "broadcast-456",
			Data: map[string]interface{}{
				"broadcast_id": "broadcast-456",
				"reason":       "High bounce rate detected",
			},
		}

		broadcast := &domain.Broadcast{
			ID:   "broadcast-456",
			Name: "Test Broadcast",
		}

		mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger)
		mockLogger.EXPECT().Info("Processing circuit breaker event")

		mockBroadcastRepo.EXPECT().GetBroadcast(ctx, "workspace-123", "broadcast-456").Return(broadcast, nil)
		mockWorkspaceRepo.EXPECT().GetByID(ctx, "workspace-123").Return(nil, errors.New("workspace not found"))

		mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger)
		mockLogger.EXPECT().Error("Failed to get workspace for circuit breaker notification")

		service.HandleCircuitBreakerEvent(ctx, payload)
	})

	t.Run("Error - No workspace owners found", func(t *testing.T) {
		payload := domain.EventPayload{
			Type:        "circuit_breaker",
			WorkspaceID: "workspace-123",
			EntityID:    "broadcast-456",
			Data: map[string]interface{}{
				"broadcast_id": "broadcast-456",
				"reason":       "High bounce rate detected",
			},
		}

		broadcast := &domain.Broadcast{
			ID:   "broadcast-456",
			Name: "Test Broadcast",
		}

		workspace := &domain.Workspace{
			ID:   "workspace-123",
			Name: "Test Workspace",
		}

		// No owners in workspace users
		workspaceUsers := []*domain.UserWorkspaceWithEmail{
			{
				UserWorkspace: domain.UserWorkspace{
					UserID:      "user-1",
					WorkspaceID: "workspace-123",
					Role:        "member", // Not an owner
				},
				Email: "member@example.com",
			},
		}

		mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger)
		mockLogger.EXPECT().Info("Processing circuit breaker event")

		mockBroadcastRepo.EXPECT().GetBroadcast(ctx, "workspace-123", "broadcast-456").Return(broadcast, nil)
		mockWorkspaceRepo.EXPECT().GetByID(ctx, "workspace-123").Return(workspace, nil)
		mockWorkspaceRepo.EXPECT().GetWorkspaceUsersWithEmail(ctx, "workspace-123").Return(workspaceUsers, nil)

		mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger)
		mockLogger.EXPECT().Warn("No workspace owners with email found for notification")

		service.HandleCircuitBreakerEvent(ctx, payload)
	})
}

func TestSystemNotificationService_HandleBroadcastFailedEvent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockBroadcastRepo := mocks.NewMockBroadcastRepository(ctrl)
	mockMailer := pkgmocks.NewMockMailer(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	service := NewSystemNotificationService(
		mockWorkspaceRepo,
		mockBroadcastRepo,
		mockMailer,
		mockLogger,
	)

	ctx := context.Background()
	payload := domain.EventPayload{
		Type:        "broadcast_failed",
		WorkspaceID: "workspace-123",
		EntityID:    "broadcast-456",
	}

	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger)
	mockLogger.EXPECT().Info("Processing broadcast failed event")

	service.HandleBroadcastFailedEvent(ctx, payload)
}

func TestSystemNotificationService_HandleSystemAlert(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockBroadcastRepo := mocks.NewMockBroadcastRepository(ctrl)
	mockMailer := pkgmocks.NewMockMailer(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	service := NewSystemNotificationService(
		mockWorkspaceRepo,
		mockBroadcastRepo,
		mockMailer,
		mockLogger,
	)

	ctx := context.Background()
	payload := domain.EventPayload{
		Type:        "system_alert",
		WorkspaceID: "workspace-123",
		EntityID:    "alert-789",
	}

	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger)
	mockLogger.EXPECT().Info("Processing system alert event")

	service.HandleSystemAlert(ctx, payload)
}

func TestSystemNotificationService_notifyWorkspaceOwners(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockBroadcastRepo := mocks.NewMockBroadcastRepository(ctrl)
	mockMailer := pkgmocks.NewMockMailer(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	service := NewSystemNotificationService(
		mockWorkspaceRepo,
		mockBroadcastRepo,
		mockMailer,
		mockLogger,
	)

	ctx := context.Background()
	workspaceID := "workspace-123"

	t.Run("Success - Notify single owner", func(t *testing.T) {
		workspaceUsers := []*domain.UserWorkspaceWithEmail{
			{
				UserWorkspace: domain.UserWorkspace{
					UserID:      "user-1",
					WorkspaceID: "workspace-123",
					Role:        "owner",
				},
				Email: "owner@example.com",
			},
		}

		mockWorkspaceRepo.EXPECT().GetWorkspaceUsersWithEmail(ctx, workspaceID).Return(workspaceUsers, nil)

		mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger)
		mockLogger.EXPECT().Info("Notification sent successfully to workspace owner")

		notificationFunc := func(email string) error {
			assert.Equal(t, "owner@example.com", email)
			return nil
		}

		err := service.notifyWorkspaceOwners(ctx, workspaceID, notificationFunc)
		assert.NoError(t, err)
	})

	t.Run("Success - Notify multiple owners", func(t *testing.T) {
		workspaceUsers := []*domain.UserWorkspaceWithEmail{
			{
				UserWorkspace: domain.UserWorkspace{
					UserID:      "user-1",
					WorkspaceID: "workspace-123",
					Role:        "owner",
				},
				Email: "owner1@example.com",
			},
			{
				UserWorkspace: domain.UserWorkspace{
					UserID:      "user-2",
					WorkspaceID: "workspace-123",
					Role:        "owner",
				},
				Email: "owner2@example.com",
			},
			{
				UserWorkspace: domain.UserWorkspace{
					UserID:      "user-3",
					WorkspaceID: "workspace-123",
					Role:        "member", // Not an owner
				},
				Email: "member@example.com",
			},
		}

		mockWorkspaceRepo.EXPECT().GetWorkspaceUsersWithEmail(ctx, workspaceID).Return(workspaceUsers, nil)

		// Expect two successful notifications
		mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).Times(2)
		mockLogger.EXPECT().Info("Notification sent successfully to workspace owner").Times(2)

		callCount := 0
		expectedEmails := []string{"owner1@example.com", "owner2@example.com"}

		notificationFunc := func(email string) error {
			assert.Contains(t, expectedEmails, email)
			callCount++
			return nil
		}

		err := service.notifyWorkspaceOwners(ctx, workspaceID, notificationFunc)
		assert.NoError(t, err)
		assert.Equal(t, 2, callCount, "Should call notification function for both owners")
	})

	t.Run("Error - Failed to get workspace users", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().GetWorkspaceUsersWithEmail(ctx, workspaceID).Return(nil, errors.New("database error"))

		notificationFunc := func(email string) error {
			t.Fatal("Should not call notification function")
			return nil
		}

		err := service.notifyWorkspaceOwners(ctx, workspaceID, notificationFunc)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "database error")
	})

	t.Run("Error - Notification function fails", func(t *testing.T) {
		workspaceUsers := []*domain.UserWorkspaceWithEmail{
			{
				UserWorkspace: domain.UserWorkspace{
					UserID:      "user-1",
					WorkspaceID: "workspace-123",
					Role:        "owner",
				},
				Email: "owner@example.com",
			},
		}

		mockWorkspaceRepo.EXPECT().GetWorkspaceUsersWithEmail(ctx, workspaceID).Return(workspaceUsers, nil)

		mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger)
		mockLogger.EXPECT().Error("Failed to send notification to workspace owner")

		notificationFunc := func(email string) error {
			return errors.New("email sending failed")
		}

		err := service.notifyWorkspaceOwners(ctx, workspaceID, notificationFunc)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "email sending failed")
	})

	t.Run("Warning - No owners found", func(t *testing.T) {
		workspaceUsers := []*domain.UserWorkspaceWithEmail{
			{
				UserWorkspace: domain.UserWorkspace{
					UserID:      "user-1",
					WorkspaceID: "workspace-123",
					Role:        "member",
				},
				Email: "member@example.com",
			},
		}

		mockWorkspaceRepo.EXPECT().GetWorkspaceUsersWithEmail(ctx, workspaceID).Return(workspaceUsers, nil)

		mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger)
		mockLogger.EXPECT().Warn("No workspace owners with email found for notification")

		notificationFunc := func(email string) error {
			t.Fatal("Should not call notification function")
			return nil
		}

		err := service.notifyWorkspaceOwners(ctx, workspaceID, notificationFunc)
		assert.NoError(t, err) // No error, just warning
	})

	t.Run("Warning - Mailer not available", func(t *testing.T) {
		// Create service without mailer
		serviceWithoutMailer := &SystemNotificationService{
			workspaceRepo: mockWorkspaceRepo,
			broadcastRepo: mockBroadcastRepo,
			mailer:        nil, // No mailer
			logger:        mockLogger,
		}

		workspaceUsers := []*domain.UserWorkspaceWithEmail{
			{
				UserWorkspace: domain.UserWorkspace{
					UserID:      "user-1",
					WorkspaceID: "workspace-123",
					Role:        "owner",
				},
				Email: "owner@example.com",
			},
		}

		mockWorkspaceRepo.EXPECT().GetWorkspaceUsersWithEmail(ctx, workspaceID).Return(workspaceUsers, nil)

		mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger)
		mockLogger.EXPECT().Warn("Cannot send notification - mailer not available")

		notificationFunc := func(email string) error {
			return nil
		}

		err := serviceWithoutMailer.notifyWorkspaceOwners(ctx, workspaceID, notificationFunc)
		assert.NoError(t, err)
	})
}

func TestSystemNotificationService_RegisterWithEventBus(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockBroadcastRepo := mocks.NewMockBroadcastRepository(ctrl)
	mockMailer := pkgmocks.NewMockMailer(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockEventBus := mocks.NewMockEventBus(ctrl)

	service := NewSystemNotificationService(
		mockWorkspaceRepo,
		mockBroadcastRepo,
		mockMailer,
		mockLogger,
	)

	// Expect subscriptions to be registered
	mockEventBus.EXPECT().Subscribe(domain.EventBroadcastCircuitBreaker, gomock.Any())
	mockEventBus.EXPECT().Subscribe(domain.EventBroadcastFailed, gomock.Any())

	mockLogger.EXPECT().Info("System notification service registered with event bus")

	service.RegisterWithEventBus(mockEventBus)
}
