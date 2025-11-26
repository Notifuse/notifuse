package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupCustomEventServiceTest(t *testing.T) (
	*mocks.MockCustomEventRepository,
	*mocks.MockContactRepository,
	*mocks.MockAuthService,
	*CustomEventService,
	*gomock.Controller,
) {
	ctrl := gomock.NewController(t)
	mockRepo := mocks.NewMockCustomEventRepository(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Set up logger expectations
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	service := NewCustomEventService(mockRepo, mockContactRepo, mockAuthService, mockLogger)

	return mockRepo, mockContactRepo, mockAuthService, service, ctrl
}

func TestCustomEventService_CreateEvent(t *testing.T) {
	mockRepo, mockContactRepo, mockAuthService, service, ctrl := setupCustomEventServiceTest(t)
	defer ctrl.Finish()

	ctx := context.Background()
	workspaceID := "workspace123"
	now := time.Now()

	userWorkspace := &domain.UserWorkspace{
		WorkspaceID: workspaceID,
		UserID:      "user123",
		Permissions: domain.UserPermissions{
			domain.PermissionResourceContacts: domain.ResourcePermissions{
				Read:  true,
				Write: true,
			},
		},
	}

	req := &domain.CreateCustomEventRequest{
		WorkspaceID: workspaceID,
		Email:       "user@example.com",
		EventName:   "orders/fulfilled",
		ExternalID:  "order_12345",
		Properties: map[string]interface{}{
			"total": 99.99,
		},
		OccurredAt: &now,
	}

	t.Run("successful creation with existing contact", func(t *testing.T) {
		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user123"}, userWorkspace, nil)

		existingContact := &domain.Contact{Email: req.Email}
		mockContactRepo.EXPECT().
			GetContactByEmail(gomock.Any(), workspaceID, req.Email).
			Return(existingContact, nil)

		mockRepo.EXPECT().
			Create(gomock.Any(), workspaceID, gomock.Any()).
			Return(nil)

		result, err := service.CreateEvent(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, req.Email, result.Email)
		assert.Equal(t, req.EventName, result.EventName)
		assert.Equal(t, req.ExternalID, result.ExternalID)
	})

	t.Run("successful creation with auto-created contact", func(t *testing.T) {
		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user123"}, userWorkspace, nil)

		mockContactRepo.EXPECT().
			GetContactByEmail(gomock.Any(), workspaceID, req.Email).
			Return(nil, errors.New("contact not found"))

		mockContactRepo.EXPECT().
			UpsertContact(gomock.Any(), workspaceID, gomock.Any()).
			Return(true, nil)

		mockRepo.EXPECT().
			Create(gomock.Any(), workspaceID, gomock.Any()).
			Return(nil)

		result, err := service.CreateEvent(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, req.Email, result.Email)
	})

	t.Run("authentication error", func(t *testing.T) {
		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, nil, nil, errors.New("auth error"))

		result, err := service.CreateEvent(ctx, req)
		require.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("permission denied", func(t *testing.T) {
		noPermWorkspace := &domain.UserWorkspace{
			WorkspaceID: workspaceID,
			UserID:      "user123",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceContacts: domain.ResourcePermissions{
					Read:  true,
					Write: false,
				},
			},
		}

		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user123"}, noPermWorkspace, nil)

		result, err := service.CreateEvent(ctx, req)
		require.Error(t, err)
		assert.IsType(t, &domain.PermissionError{}, err)
		assert.Nil(t, result)
	})
}

func TestCustomEventService_BatchCreateEvents(t *testing.T) {
	mockRepo, _, mockAuthService, service, ctrl := setupCustomEventServiceTest(t)
	defer ctrl.Finish()

	ctx := context.Background()
	workspaceID := "workspace123"
	now := time.Now()

	userWorkspace := &domain.UserWorkspace{
		WorkspaceID: workspaceID,
		UserID:      "user123",
		Permissions: domain.UserPermissions{
			domain.PermissionResourceContacts: domain.ResourcePermissions{
				Read:  true,
				Write: true,
			},
		},
	}

	events := []*domain.CustomEvent{
		{
			ExternalID: "event_1",
			Email:      "user1@example.com",
			EventName:  "orders/fulfilled",
			Properties: map[string]interface{}{"total": 99.99},
			OccurredAt: now,
			Source:     "api",
			CreatedAt:  now,
			UpdatedAt:  now,
		},
		{
			ExternalID: "event_2",
			Email:      "user2@example.com",
			EventName:  "payment.succeeded",
			Properties: map[string]interface{}{"amount": 50.00},
			OccurredAt: now,
			Source:     "api",
			CreatedAt:  now,
			UpdatedAt:  now,
		},
	}

	req := &domain.BatchCreateCustomEventsRequest{
		WorkspaceID: workspaceID,
		Events:      events,
	}

	t.Run("successful batch create", func(t *testing.T) {
		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user123"}, userWorkspace, nil)

		mockRepo.EXPECT().
			BatchCreate(gomock.Any(), workspaceID, events).
			Return(nil)

		result, err := service.BatchCreateEvents(ctx, req)
		require.NoError(t, err)
		require.Len(t, result, 2)
		assert.Equal(t, "event_1", result[0])
		assert.Equal(t, "event_2", result[1])
	})
}

func TestCustomEventService_GetEvent(t *testing.T) {
	mockRepo, _, mockAuthService, service, ctrl := setupCustomEventServiceTest(t)
	defer ctrl.Finish()

	ctx := context.Background()
	workspaceID := "workspace123"
	eventName := "orders/fulfilled"
	externalID := "order_12345"

	userWorkspace := &domain.UserWorkspace{
		WorkspaceID: workspaceID,
		UserID:      "user123",
		Permissions: domain.UserPermissions{
			domain.PermissionResourceContacts: domain.ResourcePermissions{
				Read:  true,
				Write: false,
			},
		},
	}

	expectedEvent := &domain.CustomEvent{
		ExternalID: externalID,
		Email:      "user@example.com",
		EventName:  eventName,
		Properties: map[string]interface{}{"total": 99.99},
		OccurredAt: time.Now(),
		Source:     "api",
	}

	t.Run("successful retrieval", func(t *testing.T) {
		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user123"}, userWorkspace, nil)

		mockRepo.EXPECT().
			GetByID(gomock.Any(), workspaceID, eventName, externalID).
			Return(expectedEvent, nil)

		result, err := service.GetEvent(ctx, workspaceID, eventName, externalID)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, expectedEvent, result)
	})

	t.Run("permission denied", func(t *testing.T) {
		noPermWorkspace := &domain.UserWorkspace{
			WorkspaceID: workspaceID,
			UserID:      "user123",
			Permissions: domain.UserPermissions{},
		}

		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user123"}, noPermWorkspace, nil)

		result, err := service.GetEvent(ctx, workspaceID, eventName, externalID)
		require.Error(t, err)
		assert.IsType(t, &domain.PermissionError{}, err)
		assert.Nil(t, result)
	})
}

func TestCustomEventService_ListEvents(t *testing.T) {
	mockRepo, _, mockAuthService, service, ctrl := setupCustomEventServiceTest(t)
	defer ctrl.Finish()

	ctx := context.Background()
	workspaceID := "workspace123"
	email := "user@example.com"

	userWorkspace := &domain.UserWorkspace{
		WorkspaceID: workspaceID,
		UserID:      "user123",
		Permissions: domain.UserPermissions{
			domain.PermissionResourceContacts: domain.ResourcePermissions{
				Read:  true,
				Write: false,
			},
		},
	}

	expectedEvents := []*domain.CustomEvent{
		{
			ExternalID: "event_1",
			Email:      email,
			EventName:  "orders/fulfilled",
			Properties: map[string]interface{}{"total": 99.99},
			OccurredAt: time.Now(),
			Source:     "api",
		},
	}

	t.Run("successful list by email", func(t *testing.T) {
		req := &domain.ListCustomEventsRequest{
			WorkspaceID: workspaceID,
			Email:       email,
			Limit:       50,
			Offset:      0,
		}

		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user123"}, userWorkspace, nil)

		mockRepo.EXPECT().
			ListByEmail(gomock.Any(), workspaceID, email, 50, 0).
			Return(expectedEvents, nil)

		result, err := service.ListEvents(ctx, req)
		require.NoError(t, err)
		require.Len(t, result, 1)
		assert.Equal(t, expectedEvents, result)
	})
}
