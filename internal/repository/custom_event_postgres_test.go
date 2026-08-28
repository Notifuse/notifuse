package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
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

func TestCustomEventRepository_Upsert(t *testing.T) {
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
				event.GoalName,
				event.GoalType,
				event.GoalValue,
				event.DeletedAt,
				sqlmock.AnyArg(),
				sqlmock.AnyArg(),
				true,
			).
			WillReturnResult(sqlmock.NewResult(1, 1))

		err := repo.Upsert(ctx, workspaceID, event)
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("successful upsert with integration_id", func(t *testing.T) {
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
				eventWithIntegration.GoalName,
				eventWithIntegration.GoalType,
				eventWithIntegration.GoalValue,
				eventWithIntegration.DeletedAt,
				sqlmock.AnyArg(),
				sqlmock.AnyArg(),
				true,
			).
			WillReturnResult(sqlmock.NewResult(1, 1))

		err := repo.Upsert(ctx, workspaceID, eventWithIntegration)
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("workspace connection error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(nil, errors.New("connection error"))

		err := repo.Upsert(ctx, workspaceID, event)
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
				event.GoalName,
				event.GoalType,
				event.GoalValue,
				event.DeletedAt,
				sqlmock.AnyArg(),
				sqlmock.AnyArg(),
				true,
			).
			WillReturnError(errors.New("execution error"))

		err := repo.Upsert(ctx, workspaceID, event)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to upsert custom event")
	})
}

func TestCustomEventRepository_BatchUpsert(t *testing.T) {
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

	t.Run("successful batch upsert", func(t *testing.T) {
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
					event.GoalName,
					event.GoalType,
					event.GoalValue,
					event.DeletedAt,
					sqlmock.AnyArg(),
					sqlmock.AnyArg(),
					true,
				).
				WillReturnResult(sqlmock.NewResult(1, 1))
		}

		mock.ExpectCommit()

		err := repo.BatchUpsert(ctx, workspaceID, events)
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("workspace connection error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(nil, errors.New("connection error"))

		err := repo.BatchUpsert(ctx, workspaceID, events)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get workspace connection")
	})

	t.Run("begin transaction error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		mock.ExpectBegin().WillReturnError(errors.New("begin error"))

		err := repo.BatchUpsert(ctx, workspaceID, events)
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
				events[0].GoalName,
				events[0].GoalType,
				events[0].GoalValue,
				events[0].DeletedAt,
				sqlmock.AnyArg(),
				sqlmock.AnyArg(),
				true,
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
				events[1].GoalName,
				events[1].GoalType,
				events[1].GoalValue,
				events[1].DeletedAt,
				sqlmock.AnyArg(),
				sqlmock.AnyArg(),
				true,
			).
			WillReturnError(errors.New("execution error"))

		mock.ExpectRollback()

		err := repo.BatchUpsert(ctx, workspaceID, events)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to upsert event")
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
			"occurred_at", "source", "integration_id",
			"goal_name", "goal_type", "goal_value", "deleted_at",
			"created_at", "updated_at",
		}).AddRow(
			eventName, externalID, "user@example.com", propertiesJSON,
			now, "api", nil,
			nil, nil, nil, nil,
			now, now,
		)

		mock.ExpectQuery(`SELECT (.+) FROM custom_events WHERE event_name = \$1 AND external_id = \$2 AND deleted_at IS NULL`).
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

		mock.ExpectQuery(`SELECT (.+) FROM custom_events WHERE event_name = \$1 AND external_id = \$2 AND deleted_at IS NULL`).
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
			"occurred_at", "source", "integration_id",
			"goal_name", "goal_type", "goal_value", "deleted_at",
			"created_at", "updated_at",
		}).
			AddRow("orders/fulfilled", "order_1", email, json1, now, "api", nil, nil, nil, nil, nil, now, now).
			AddRow("payment.succeeded", "payment_1", email, json2, now, "integration", nil, nil, nil, nil, nil, now, now)

		mock.ExpectQuery(`SELECT (.+) FROM custom_events WHERE email = \$1 AND deleted_at IS NULL ORDER BY occurred_at DESC LIMIT \$2 OFFSET \$3`).
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
			"occurred_at", "source", "integration_id",
			"goal_name", "goal_type", "goal_value", "deleted_at",
			"created_at", "updated_at",
		})

		mock.ExpectQuery(`SELECT (.+) FROM custom_events WHERE email = \$1 AND deleted_at IS NULL ORDER BY occurred_at DESC LIMIT \$2 OFFSET \$3`).
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

		mock.ExpectQuery(`SELECT (.+) FROM custom_events WHERE email = \$1 AND deleted_at IS NULL ORDER BY occurred_at DESC LIMIT \$2 OFFSET \$3`).
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
			"occurred_at", "source", "integration_id",
			"goal_name", "goal_type", "goal_value", "deleted_at",
			"created_at", "updated_at",
		}).
			AddRow(eventName, "order_1", "user1@example.com", json1, now, "api", nil, nil, nil, nil, nil, now, now).
			AddRow(eventName, "order_2", "user2@example.com", json2, now, "api", nil, nil, nil, nil, nil, now, now)

		mock.ExpectQuery(`SELECT (.+) FROM custom_events WHERE event_name = \$1 AND deleted_at IS NULL ORDER BY occurred_at DESC LIMIT \$2 OFFSET \$3`).
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

// A nil Properties map is the caller saying nothing about properties; an empty map
// is the caller emptying them. json.Marshal flattens the first into the literal
// null and the second into {}, so the distinction has to travel as its own
// parameter rather than inside the payload.
func TestCustomEventRepository_Upsert_OmittedPropertiesPreserveTheStoredRow(t *testing.T) {
	mockWorkspaceRepo, repo, mock, db, cleanup := setupCustomEventTest(t)
	defer cleanup()

	ctx := context.Background()
	workspaceID := "workspace123"
	now := time.Now()

	t.Run("nil properties send an inert payload and do not claim the column", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().GetConnection(ctx, workspaceID).Return(db, nil)

		event := &domain.CustomEvent{
			ExternalID: "order_12345",
			Email:      "user@example.com",
			EventName:  "orders/fulfilled",
			OccurredAt: now,
			Source:     "api",
		}

		mock.ExpectExec(`properties = CASE WHEN`).
			WithArgs(
				event.EventName,
				event.ExternalID,
				event.Email,
				[]byte("{}"),
				event.OccurredAt,
				event.Source,
				event.IntegrationID,
				event.GoalName,
				event.GoalType,
				event.GoalValue,
				event.DeletedAt,
				sqlmock.AnyArg(),
				sqlmock.AnyArg(),
				false,
			).
			WillReturnResult(sqlmock.NewResult(1, 1))

		require.NoError(t, repo.Upsert(ctx, workspaceID, event))
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("an explicit empty map still claims the column", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().GetConnection(ctx, workspaceID).Return(db, nil)

		event := &domain.CustomEvent{
			ExternalID: "order_12345",
			Email:      "user@example.com",
			EventName:  "orders/fulfilled",
			Properties: map[string]interface{}{},
			OccurredAt: now,
			Source:     "api",
		}

		mock.ExpectExec(`properties = CASE WHEN`).
			WithArgs(
				event.EventName,
				event.ExternalID,
				event.Email,
				[]byte("{}"),
				event.OccurredAt,
				event.Source,
				event.IntegrationID,
				event.GoalName,
				event.GoalType,
				event.GoalValue,
				event.DeletedAt,
				sqlmock.AnyArg(),
				sqlmock.AnyArg(),
				true,
			).
			WillReturnResult(sqlmock.NewResult(1, 1))

		require.NoError(t, repo.Upsert(ctx, workspaceID, event))
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

// integration_id already arrives as a faithful *string, so it only needs the
// COALESCE its goal_* neighbours in the same clause already have: an API upsert
// that never mentions the integration must not unlink the event from it.
func TestCustomEventRepository_Upsert_OmittedIntegrationIDIsCoalesced(t *testing.T) {
	mockWorkspaceRepo, repo, mock, db, cleanup := setupCustomEventTest(t)
	defer cleanup()

	ctx := context.Background()
	workspaceID := "workspace123"

	mockWorkspaceRepo.EXPECT().GetConnection(ctx, workspaceID).Return(db, nil)

	event := &domain.CustomEvent{
		ExternalID: "order_12345",
		Email:      "user@example.com",
		EventName:  "orders/fulfilled",
		Properties: map[string]interface{}{"total": 1},
		OccurredAt: time.Now(),
		Source:     "api",
	}

	mock.ExpectExec(`integration_id = COALESCE\(EXCLUDED\.integration_id, custom_events\.integration_id\)`).
		WillReturnResult(sqlmock.NewResult(1, 1))

	require.NoError(t, repo.Upsert(ctx, workspaceID, event))
	require.NoError(t, mock.ExpectationsWereMet())
}

// The batch import path writes through the same ON CONFLICT clause, and the public
// batch-import examples omit properties entirely.
func TestCustomEventRepository_BatchUpsert_OmittedPropertiesPreserveTheStoredRow(t *testing.T) {
	mockWorkspaceRepo, repo, mock, db, cleanup := setupCustomEventTest(t)
	defer cleanup()

	ctx := context.Background()
	workspaceID := "workspace123"
	now := time.Now()

	mockWorkspaceRepo.EXPECT().GetConnection(ctx, workspaceID).Return(db, nil)

	events := []*domain.CustomEvent{
		{
			ExternalID: "order_1",
			Email:      "user@example.com",
			EventName:  "orders/fulfilled",
			OccurredAt: now,
			Source:     "import",
		},
		{
			ExternalID: "order_2",
			Email:      "user@example.com",
			EventName:  "orders/fulfilled",
			Properties: map[string]interface{}{"total": 12},
			OccurredAt: now,
			Source:     "import",
		},
	}

	mock.ExpectBegin()
	mock.ExpectPrepare(`properties = CASE WHEN`)
	mock.ExpectExec(`properties = CASE WHEN`).
		WithArgs(
			events[0].EventName, events[0].ExternalID, events[0].Email, []byte("{}"), events[0].OccurredAt,
			events[0].Source, events[0].IntegrationID, events[0].GoalName, events[0].GoalType, events[0].GoalValue,
			events[0].DeletedAt, sqlmock.AnyArg(), sqlmock.AnyArg(), false,
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	propertiesJSON, err := json.Marshal(events[1].Properties)
	require.NoError(t, err)
	mock.ExpectExec(`properties = CASE WHEN`).
		WithArgs(
			events[1].EventName, events[1].ExternalID, events[1].Email, propertiesJSON, events[1].OccurredAt,
			events[1].Source, events[1].IntegrationID, events[1].GoalName, events[1].GoalType, events[1].GoalValue,
			events[1].DeletedAt, sqlmock.AnyArg(), sqlmock.AnyArg(), true,
		).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	require.NoError(t, repo.BatchUpsert(ctx, workspaceID, events))
	require.NoError(t, mock.ExpectationsWereMet())
}

// conflictAssignedColumns returns, in order, the columns the ON CONFLICT clause of
// a custom-events upsert rewrites. It reads the statement rather than a database
// because sqlmock never runs the SQL, and which columns a second write is allowed
// to touch is the whole substance of the clause.
func conflictAssignedColumns(t *testing.T, query string) []string {
	t.Helper()

	_, conflict, found := strings.Cut(query, "DO UPDATE SET")
	require.True(t, found, "the upsert must still be an ON CONFLICT ... DO UPDATE")

	assignment := regexp.MustCompile(`^([a-z_]+) =`)
	columns := []string{}
	for _, line := range strings.Split(conflict, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "WHERE ") {
			// The clause's own guard; the assignments end here.
			break
		}
		if match := assignment.FindStringSubmatch(line); match != nil {
			columns = append(columns, match[1])
		}
	}
	return columns
}

// source records where a row came from, and webhook_custom_events_trigger gates
// its entire web-analytics exclusion on it: flip a bridged row's source to 'api'
// and it starts fanning pageview-scale conversions — client-supplied properties
// included — out to third-party subscribers that asked for commerce events.
// Nothing a caller sends is meant to move a row's origin: UpsertCustomEventRequest
// has no source field at all and the service stamps "api" for the insert, while an
// import entry's source describes the entry rather than the resource it names. So
// a conflict leaves the stored origin alone, and this pins the exact set of
// columns a second write may rewrite.
func TestCustomEventRepository_UpsertQuery_ConflictLeavesTheStoredOriginAlone(t *testing.T) {
	assert.Equal(t, []string{
		"email",
		"properties",
		"occurred_at",
		"integration_id",
		"goal_name",
		"goal_type",
		"goal_value",
		"deleted_at",
		"updated_at",
	}, conflictAssignedColumns(t, upsertCustomEventQuery))

	// A first write still has to record where the row came from.
	insert, _, _ := strings.Cut(upsertCustomEventQuery, "ON CONFLICT")
	assert.Contains(t, insert, "source", "a brand-new row must still record its origin")
}
