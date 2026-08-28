package service

import (
	"context"
	"encoding/json"
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

func TestCustomEventService_UpsertEvent(t *testing.T) {
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

	req := &domain.UpsertCustomEventRequest{
		WorkspaceID: workspaceID,
		Email:       "user@example.com",
		EventName:   "orders/fulfilled",
		ExternalID:  "order_12345",
		Properties: map[string]interface{}{
			"total": 99.99,
		},
		OccurredAt: &now,
	}

	// The response is read back from the row the write left behind; see
	// TestCustomEventService_UpsertEvent_AnswersWithTheStoredRow for why.
	storedAfterWrite := func() *domain.CustomEvent {
		return &domain.CustomEvent{
			EventName:  req.EventName,
			ExternalID: req.ExternalID,
			Email:      req.Email,
			Properties: map[string]interface{}{"total": 99.99},
			OccurredAt: now,
			Source:     "api",
		}
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
			Upsert(gomock.Any(), workspaceID, gomock.Any()).
			Return(nil)

		mockRepo.EXPECT().
			GetByID(gomock.Any(), workspaceID, req.EventName, req.ExternalID).
			Return(storedAfterWrite(), nil)

		result, err := service.UpsertEvent(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, req.Email, result.Email)
		assert.Equal(t, req.EventName, result.EventName)
		assert.Equal(t, req.ExternalID, result.ExternalID)
	})

	t.Run("successful upsert with auto-created contact", func(t *testing.T) {
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
			Upsert(gomock.Any(), workspaceID, gomock.Any()).
			Return(nil)

		mockRepo.EXPECT().
			GetByID(gomock.Any(), workspaceID, req.EventName, req.ExternalID).
			Return(storedAfterWrite(), nil)

		result, err := service.UpsertEvent(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, req.Email, result.Email)
	})

	t.Run("authentication error", func(t *testing.T) {
		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, nil, nil, errors.New("auth error"))

		result, err := service.UpsertEvent(ctx, req)
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

		result, err := service.UpsertEvent(ctx, req)
		require.Error(t, err)
		assert.IsType(t, &domain.PermissionError{}, err)
		assert.Nil(t, result)
	})
}

func TestCustomEventService_ImportEvents(t *testing.T) {
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

	req := &domain.ImportCustomEventsRequest{
		WorkspaceID: workspaceID,
		Events:      events,
	}

	t.Run("successful import", func(t *testing.T) {
		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user123"}, userWorkspace, nil)

		mockRepo.EXPECT().
			BatchUpsert(gomock.Any(), workspaceID, events).
			Return(nil)

		result, err := service.ImportEvents(ctx, req)
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

// customEvents.upsert is a patch on the (event_name, external_id) pair, so the row
// that exists afterwards is not the row the request describes: every field the body
// left out kept the value already stored, and source is never rewritten at all.
// Echoing the request back therefore puts "properties": null in a 200 body over a
// row that still holds its properties — and null is also how this endpoint reads
// "say nothing about properties", so the caller cannot even send the response back
// to reconcile. openapi declares properties as a non-nullable object.
func TestCustomEventService_UpsertEvent_AnswersWithTheStoredRow(t *testing.T) {
	ctx := context.Background()
	workspaceID := "workspace123"
	email := "user@example.com"
	eventName := "subscription.updated"
	externalID := "sub_12345"
	integrationID := "shopify_integration_123"

	userWorkspace := &domain.UserWorkspace{
		WorkspaceID: workspaceID,
		UserID:      "user123",
		Permissions: domain.UserPermissions{
			domain.PermissionResourceContacts: domain.ResourcePermissions{Read: true, Write: true},
		},
	}

	// Seeded richly on purpose: against an empty stored row a wipe and a correct
	// merge look identical.
	newStoredEvent := func() *domain.CustomEvent {
		occurred := time.Now().Add(-3 * time.Hour)
		return &domain.CustomEvent{
			EventName:     eventName,
			ExternalID:    externalID,
			Email:         email,
			Properties:    map[string]interface{}{"plan": "premium", "seats": float64(5)},
			OccurredAt:    occurred,
			Source:        "web_analytics",
			IntegrationID: &integrationID,
			CreatedAt:     occurred,
			UpdatedAt:     occurred,
		}
	}

	t.Run("a body that omits properties answers with the stored ones", func(t *testing.T) {
		mockRepo, mockContactRepo, mockAuthService, service, ctrl := setupCustomEventServiceTest(t)
		defer ctrl.Finish()

		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user123"}, userWorkspace, nil)
		mockContactRepo.EXPECT().
			GetContactByEmail(gomock.Any(), workspaceID, email).
			Return(&domain.Contact{Email: email}, nil)

		mockRepo.EXPECT().
			GetByID(gomock.Any(), workspaceID, eventName, externalID).
			Return(newStoredEvent(), nil).
			AnyTimes()

		mockRepo.EXPECT().
			Upsert(gomock.Any(), workspaceID, gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, written *domain.CustomEvent) error {
				assert.Nil(t, written.Properties,
					"the write itself must still say nothing about properties, or the SQL cannot preserve them")
				return nil
			})

		goalType := domain.GoalTypeSubscription
		goalValue := 49.0
		result, err := service.UpsertEvent(ctx, &domain.UpsertCustomEventRequest{
			WorkspaceID: workspaceID,
			Email:       email,
			EventName:   eventName,
			ExternalID:  externalID,
			GoalType:    &goalType,
			GoalValue:   &goalValue,
		})
		require.NoError(t, err)
		require.NotNil(t, result)

		// Compared against literals, never against the fixture: the repository mock
		// hands the service the very object the response may be built from.
		assert.Equal(t, map[string]interface{}{"plan": "premium", "seats": float64(5)}, result.Properties,
			"the response must describe the row that now exists, not the request")
		assert.Equal(t, "web_analytics", result.Source,
			"an upsert never rewrites a row's origin, so the response must not claim it did")
		require.NotNil(t, result.IntegrationID)
		assert.Equal(t, integrationID, *result.IntegrationID)
	})

	t.Run("the encoded response carries an object, never null", func(t *testing.T) {
		mockRepo, mockContactRepo, mockAuthService, service, ctrl := setupCustomEventServiceTest(t)
		defer ctrl.Finish()

		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user123"}, userWorkspace, nil)
		mockContactRepo.EXPECT().
			GetContactByEmail(gomock.Any(), workspaceID, email).
			Return(&domain.Contact{Email: email}, nil)
		mockRepo.EXPECT().
			GetByID(gomock.Any(), workspaceID, eventName, externalID).
			Return(newStoredEvent(), nil).
			AnyTimes()
		mockRepo.EXPECT().Upsert(gomock.Any(), workspaceID, gomock.Any()).Return(nil)

		result, err := service.UpsertEvent(ctx, &domain.UpsertCustomEventRequest{
			WorkspaceID: workspaceID,
			Email:       email,
			EventName:   eventName,
			ExternalID:  externalID,
		})
		require.NoError(t, err)

		body, err := json.Marshal(result)
		require.NoError(t, err)
		assert.NotContains(t, string(body), `"properties":null`,
			"a JS client reading event.properties.plan throws on null, and null re-POSTed reads as an omission")
		assert.Contains(t, string(body), `"properties":{"plan":"premium","seats":5}`)
	})

	t.Run("a row that cannot be read back still answers with an object", func(t *testing.T) {
		mockRepo, mockContactRepo, mockAuthService, service, ctrl := setupCustomEventServiceTest(t)
		defer ctrl.Finish()

		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user123"}, userWorkspace, nil)
		mockContactRepo.EXPECT().
			GetContactByEmail(gomock.Any(), workspaceID, email).
			Return(&domain.Contact{Email: email}, nil)

		// The write landed and the row is gone or unreadable. There is nothing left
		// to preserve, so an empty object is the honest floor — and the 200 body has
		// to carry one either way.
		mockRepo.EXPECT().
			GetByID(gomock.Any(), workspaceID, eventName, "brand_new").
			Return(nil, errors.New("custom event not found")).
			AnyTimes()
		mockRepo.EXPECT().Upsert(gomock.Any(), workspaceID, gomock.Any()).Return(nil)

		result, err := service.UpsertEvent(ctx, &domain.UpsertCustomEventRequest{
			WorkspaceID: workspaceID,
			Email:       email,
			EventName:   eventName,
			ExternalID:  "brand_new",
		})
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, map[string]interface{}{}, result.Properties)
	})

	t.Run("a soft-delete answers with the row it deleted", func(t *testing.T) {
		mockRepo, _, mockAuthService, service, ctrl := setupCustomEventServiceTest(t)
		defer ctrl.Finish()

		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "user123"}, userWorkspace, nil)

		// GetByID hides deleted rows, so a delete has nothing to read afterwards.
		mockRepo.EXPECT().
			GetByID(gomock.Any(), workspaceID, eventName, externalID).
			Return(newStoredEvent(), nil).
			AnyTimes()
		mockRepo.EXPECT().Upsert(gomock.Any(), workspaceID, gomock.Any()).Return(nil)

		deletedAt := time.Now()
		result, err := service.UpsertEvent(ctx, &domain.UpsertCustomEventRequest{
			WorkspaceID: workspaceID,
			Email:       email,
			EventName:   eventName,
			ExternalID:  externalID,
			DeletedAt:   &deletedAt,
		})
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, map[string]interface{}{"plan": "premium", "seats": float64(5)}, result.Properties)
		assert.Equal(t, "web_analytics", result.Source)
		require.NotNil(t, result.DeletedAt)
		assert.Equal(t, deletedAt, *result.DeletedAt, "the deletion the caller asked for still applies")
	})
}
