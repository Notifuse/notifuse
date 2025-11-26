package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupCustomEventTest(t *testing.T) (*mocks.MockWorkspaceRepository, *customEventRepository, sqlmock.Sqlmock, *sql.DB, func()) {
	ctrl := gomock.NewController(t)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)

	// Create a real DB connection with sqlmock
	db, mock, err := sqlmock.New()
	require.NoError(t, err)

	repo := NewCustomEventRepository(mockWorkspaceRepo)

	// Set up cleanup function
	cleanup := func() {
		_ = db.Close()
		ctrl.Finish()
	}

	return mockWorkspaceRepo, repo.(*customEventRepository), mock, db, cleanup
}

func TestCustomEventRepository_Create(t *testing.T) {
	mockWorkspaceRepo, repo, mock, db, cleanup := setupCustomEventTest(t)
	defer cleanup()

	ctx := context.Background()
	workspaceID := "workspace123"
	now := time.Now()

	event := &domain.CustomEvent{
		ExternalID: "order_12345",
		Email:      "user@example.com",
		EventName:  "orders/fulfilled",
		Properties: map[string]interface{}{
			"total":    99.99,
			"items":    3,
			"currency": "USD",
		},
		OccurredAt: now,
		Source:     "api",
	}

	t.Run("successful create", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		propertiesJSON, _ := json.Marshal(event.Properties)

		mock.ExpectExec(`INSERT INTO custom_events`).
			WithArgs(
				event.EventName,
				event.ExternalID,
				event.Email,
				propertiesJSON,
				event.OccurredAt,
				event.Source,
				event.IntegrationID,
				sqlmock.AnyArg(),
				sqlmock.AnyArg(),
			).
			WillReturnResult(sqlmock.NewResult(1, 1))

		err := repo.Create(ctx, workspaceID, event)
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("successful create with integration_id", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		integrationID := "shopify_integration_1"
		eventWithIntegration := &domain.CustomEvent{
			ExternalID:    "webhook_123",
			Email:         "user@example.com",
			EventName:     "customers/create",
			Properties:    map[string]interface{}{},
			OccurredAt:    now,
			Source:        "integration",
			IntegrationID: &integrationID,
		}

		propertiesJSON, _ := json.Marshal(eventWithIntegration.Properties)

		mock.ExpectExec(`INSERT INTO custom_events`).
			WithArgs(
				eventWithIntegration.EventName,
				eventWithIntegration.ExternalID,
				eventWithIntegration.Email,
				propertiesJSON,
				eventWithIntegration.OccurredAt,
				eventWithIntegration.Source,
				eventWithIntegration.IntegrationID,
				sqlmock.AnyArg(),
				sqlmock.AnyArg(),
			).
			WillReturnResult(sqlmock.NewResult(1, 1))

		err := repo.Create(ctx, workspaceID, eventWithIntegration)
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("workspace connection error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(nil, errors.New("connection error"))

		err := repo.Create(ctx, workspaceID, event)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get workspace connection")
	})

	t.Run("execution error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		propertiesJSON, _ := json.Marshal(event.Properties)

		mock.ExpectExec(`INSERT INTO custom_events`).
			WithArgs(
				event.EventName,
				event.ExternalID,
				event.Email,
				propertiesJSON,
				event.OccurredAt,
				event.Source,
				event.IntegrationID,
				sqlmock.AnyArg(),
				sqlmock.AnyArg(),
			).
			WillReturnError(errors.New("execution error"))

		err := repo.Create(ctx, workspaceID, event)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create custom event")
	})
}

func TestCustomEventRepository_BatchCreate(t *testing.T) {
	mockWorkspaceRepo, repo, mock, db, cleanup := setupCustomEventTest(t)
	defer cleanup()

	ctx := context.Background()
	workspaceID := "workspace123"
	now := time.Now()

	events := []*domain.CustomEvent{
		{
			ExternalID: "event_1",
			Email:      "user1@example.com",
			EventName:  "orders/fulfilled",
			Properties: map[string]interface{}{"total": 99.99},
			OccurredAt: now,
			Source:     "api",
		},
		{
			ExternalID: "event_2",
			Email:      "user2@example.com",
			EventName:  "payment.succeeded",
			Properties: map[string]interface{}{"amount": 50.00},
			OccurredAt: now,
			Source:     "integration",
		},
	}

	t.Run("successful batch create", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		mock.ExpectBegin()
		mock.ExpectPrepare(`INSERT INTO custom_events`)

		for _, event := range events {
			propertiesJSON, _ := json.Marshal(event.Properties)
			mock.ExpectExec(`INSERT INTO custom_events`).
				WithArgs(
					event.EventName,
					event.ExternalID,
					event.Email,
					propertiesJSON,
					event.OccurredAt,
					event.Source,
					event.IntegrationID,
					sqlmock.AnyArg(),
					sqlmock.AnyArg(),
				).
				WillReturnResult(sqlmock.NewResult(1, 1))
		}

		mock.ExpectCommit()

		err := repo.BatchCreate(ctx, workspaceID, events)
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("workspace connection error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(nil, errors.New("connection error"))

		err := repo.BatchCreate(ctx, workspaceID, events)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get workspace connection")
	})

	t.Run("begin transaction error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		mock.ExpectBegin().WillReturnError(errors.New("begin error"))

		err := repo.BatchCreate(ctx, workspaceID, events)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to begin transaction")
	})

	t.Run("execution error triggers rollback", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		mock.ExpectBegin()
		mock.ExpectPrepare(`INSERT INTO custom_events`)

		// First event succeeds
		propertiesJSON, _ := json.Marshal(events[0].Properties)
		mock.ExpectExec(`INSERT INTO custom_events`).
			WithArgs(
				events[0].EventName,
				events[0].ExternalID,
				events[0].Email,
				propertiesJSON,
				events[0].OccurredAt,
				events[0].Source,
				events[0].IntegrationID,
				sqlmock.AnyArg(),
				sqlmock.AnyArg(),
			).
			WillReturnResult(sqlmock.NewResult(1, 1))

		// Second event fails
		propertiesJSON2, _ := json.Marshal(events[1].Properties)
		mock.ExpectExec(`INSERT INTO custom_events`).
			WithArgs(
				events[1].EventName,
				events[1].ExternalID,
				events[1].Email,
				propertiesJSON2,
				events[1].OccurredAt,
				events[1].Source,
				events[1].IntegrationID,
				sqlmock.AnyArg(),
				sqlmock.AnyArg(),
			).
			WillReturnError(errors.New("execution error"))

		mock.ExpectRollback()

		err := repo.BatchCreate(ctx, workspaceID, events)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to insert event")
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestCustomEventRepository_GetByID(t *testing.T) {
	mockWorkspaceRepo, repo, mock, db, cleanup := setupCustomEventTest(t)
	defer cleanup()

	ctx := context.Background()
	workspaceID := "workspace123"
	eventName := "orders/fulfilled"
	externalID := "order_12345"
	now := time.Now()

	t.Run("successful retrieval", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		properties := map[string]interface{}{"total": 99.99, "items": 3}
		propertiesJSON, _ := json.Marshal(properties)

		rows := sqlmock.NewRows([]string{
			"event_name", "external_id", "email", "properties",
			"occurred_at", "source", "integration_id", "created_at", "updated_at",
		}).AddRow(
			eventName, externalID, "user@example.com", propertiesJSON,
			now, "api", nil, now, now,
		)

		mock.ExpectQuery(`SELECT (.+) FROM custom_events WHERE event_name = \$1 AND external_id = \$2`).
			WithArgs(eventName, externalID).
			WillReturnRows(rows)

		result, err := repo.GetByID(ctx, workspaceID, eventName, externalID)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, eventName, result.EventName)
		assert.Equal(t, externalID, result.ExternalID)
		assert.Equal(t, "user@example.com", result.Email)
		assert.Equal(t, 99.99, result.Properties["total"])
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("not found", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		mock.ExpectQuery(`SELECT (.+) FROM custom_events WHERE event_name = \$1 AND external_id = \$2`).
			WithArgs(eventName, externalID).
			WillReturnError(sql.ErrNoRows)

		result, err := repo.GetByID(ctx, workspaceID, eventName, externalID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "custom event not found")
		assert.Nil(t, result)
	})

	t.Run("workspace connection error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(nil, errors.New("connection error"))

		result, err := repo.GetByID(ctx, workspaceID, eventName, externalID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get workspace connection")
		assert.Nil(t, result)
	})
}

func TestCustomEventRepository_ListByEmail(t *testing.T) {
	mockWorkspaceRepo, repo, mock, db, cleanup := setupCustomEventTest(t)
	defer cleanup()

	ctx := context.Background()
	workspaceID := "workspace123"
	email := "user@example.com"
	limit := 50
	offset := 0
	now := time.Now()

	t.Run("successful list with results", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		properties1 := map[string]interface{}{"total": 99.99}
		properties2 := map[string]interface{}{"amount": 50.00}
		json1, _ := json.Marshal(properties1)
		json2, _ := json.Marshal(properties2)

		rows := sqlmock.NewRows([]string{
			"event_name", "external_id", "email", "properties",
			"occurred_at", "source", "integration_id", "created_at", "updated_at",
		}).
			AddRow("orders/fulfilled", "order_1", email, json1, now, "api", nil, now, now).
			AddRow("payment.succeeded", "payment_1", email, json2, now, "integration", nil, now, now)

		mock.ExpectQuery(`SELECT (.+) FROM custom_events WHERE email = \$1 ORDER BY occurred_at DESC LIMIT \$2 OFFSET \$3`).
			WithArgs(email, limit, offset).
			WillReturnRows(rows)

		results, err := repo.ListByEmail(ctx, workspaceID, email, limit, offset)
		require.NoError(t, err)
		require.Len(t, results, 2)
		assert.Equal(t, "orders/fulfilled", results[0].EventName)
		assert.Equal(t, "payment.succeeded", results[1].EventName)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("successful list with no results", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		rows := sqlmock.NewRows([]string{
			"event_name", "external_id", "email", "properties",
			"occurred_at", "source", "integration_id", "created_at", "updated_at",
		})

		mock.ExpectQuery(`SELECT (.+) FROM custom_events WHERE email = \$1 ORDER BY occurred_at DESC LIMIT \$2 OFFSET \$3`).
			WithArgs(email, limit, offset).
			WillReturnRows(rows)

		results, err := repo.ListByEmail(ctx, workspaceID, email, limit, offset)
		require.NoError(t, err)
		require.Empty(t, results)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("workspace connection error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(nil, errors.New("connection error"))

		results, err := repo.ListByEmail(ctx, workspaceID, email, limit, offset)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get workspace connection")
		assert.Nil(t, results)
	})

	t.Run("query error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		mock.ExpectQuery(`SELECT (.+) FROM custom_events WHERE email = \$1 ORDER BY occurred_at DESC LIMIT \$2 OFFSET \$3`).
			WithArgs(email, limit, offset).
			WillReturnError(errors.New("query error"))

		results, err := repo.ListByEmail(ctx, workspaceID, email, limit, offset)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to query custom events")
		assert.Nil(t, results)
	})
}

func TestCustomEventRepository_ListByEventName(t *testing.T) {
	mockWorkspaceRepo, repo, mock, db, cleanup := setupCustomEventTest(t)
	defer cleanup()

	ctx := context.Background()
	workspaceID := "workspace123"
	eventName := "orders/fulfilled"
	limit := 50
	offset := 0
	now := time.Now()

	t.Run("successful list with results", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		properties1 := map[string]interface{}{"total": 99.99}
		properties2 := map[string]interface{}{"total": 149.99}
		json1, _ := json.Marshal(properties1)
		json2, _ := json.Marshal(properties2)

		rows := sqlmock.NewRows([]string{
			"event_name", "external_id", "email", "properties",
			"occurred_at", "source", "integration_id", "created_at", "updated_at",
		}).
			AddRow(eventName, "order_1", "user1@example.com", json1, now, "api", nil, now, now).
			AddRow(eventName, "order_2", "user2@example.com", json2, now, "api", nil, now, now)

		mock.ExpectQuery(`SELECT (.+) FROM custom_events WHERE event_name = \$1 ORDER BY occurred_at DESC LIMIT \$2 OFFSET \$3`).
			WithArgs(eventName, limit, offset).
			WillReturnRows(rows)

		results, err := repo.ListByEventName(ctx, workspaceID, eventName, limit, offset)
		require.NoError(t, err)
		require.Len(t, results, 2)
		assert.Equal(t, "order_1", results[0].ExternalID)
		assert.Equal(t, "order_2", results[1].ExternalID)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("workspace connection error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(nil, errors.New("connection error"))

		results, err := repo.ListByEventName(ctx, workspaceID, eventName, limit, offset)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get workspace connection")
		assert.Nil(t, results)
	})
}

func TestCustomEventRepository_DeleteForEmail(t *testing.T) {
	mockWorkspaceRepo, repo, mock, db, cleanup := setupCustomEventTest(t)
	defer cleanup()

	ctx := context.Background()
	workspaceID := "workspace123"
	email := "user@example.com"

	t.Run("successful delete", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		mock.ExpectExec(`DELETE FROM custom_events WHERE email = \$1`).
			WithArgs(email).
			WillReturnResult(sqlmock.NewResult(0, 3))

		err := repo.DeleteForEmail(ctx, workspaceID, email)
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("workspace connection error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(nil, errors.New("connection error"))

		err := repo.DeleteForEmail(ctx, workspaceID, email)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get workspace connection")
	})

	t.Run("execution error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		mock.ExpectExec(`DELETE FROM custom_events WHERE email = \$1`).
			WithArgs(email).
			WillReturnError(errors.New("delete error"))

		err := repo.DeleteForEmail(ctx, workspaceID, email)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to delete custom events")
	})
}
