package repository

import (
	"context"
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

// TestWebhookDeliveryRepository_Create tests the Create method
func TestWebhookDeliveryRepository_Create(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewWebhookDeliveryRepository(mockWorkspaceRepo)

	ctx := context.Background()
	workspaceID := "ws-123"

	payload := map[string]interface{}{
		"test": "data",
		"key":  "value",
	}

	delivery := &WebhookDelivery{
		ID:             "delivery-123",
		SubscriptionID: "sub-456",
		EventType:      "contact.created",
		Payload:        payload,
		Status:         domain.WebhookDeliveryStatusPending,
		Attempts:       0,
		MaxAttempts:    5,
	}

	t.Run("Success", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		mock.ExpectExec("INSERT INTO webhook_deliveries").
			WithArgs(
				delivery.ID,
				delivery.SubscriptionID,
				delivery.EventType,
				sqlmock.AnyArg(), // payload JSON
				delivery.Status,
				delivery.Attempts,
				delivery.MaxAttempts,
				sqlmock.AnyArg(), // next_attempt_at
				sqlmock.AnyArg(), // created_at
			).
			WillReturnResult(sqlmock.NewResult(1, 1))

		err = repo.Create(ctx, workspaceID, delivery)
		assert.NoError(t, err)
		assert.False(t, delivery.CreatedAt.IsZero())
		assert.False(t, delivery.NextAttemptAt.IsZero())
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("ConnectionError", func(t *testing.T) {
		expectedErr := errors.New("connection error")
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(nil, expectedErr)

		err := repo.Create(ctx, workspaceID, delivery)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get workspace connection")
	})

	t.Run("PayloadMarshalError", func(t *testing.T) {
		db, _, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		// Create a delivery with an unmarshalable payload
		invalidDelivery := &WebhookDelivery{
			ID:             "delivery-456",
			SubscriptionID: "sub-456",
			EventType:      "test",
			Payload:        map[string]interface{}{"channel": make(chan int)}, // channels can't be marshaled
			Status:         domain.WebhookDeliveryStatusPending,
		}

		err = repo.Create(ctx, workspaceID, invalidDelivery)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to marshal payload")
	})

	t.Run("ExecError", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		expectedErr := errors.New("database error")
		mock.ExpectExec("INSERT INTO webhook_deliveries").
			WillReturnError(expectedErr)

		err = repo.Create(ctx, workspaceID, delivery)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create webhook delivery")
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

// TestWebhookDeliveryRepository_GetPendingForWorkspace tests the GetPendingForWorkspace method
func TestWebhookDeliveryRepository_GetPendingForWorkspace(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewWebhookDeliveryRepository(mockWorkspaceRepo)

	ctx := context.Background()
	workspaceID := "ws-123"
	limit := 10
	now := time.Now().UTC()

	t.Run("Success - Multiple Deliveries", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		rows := sqlmock.NewRows([]string{
			"id", "subscription_id", "event_type", "payload", "status",
			"attempts", "max_attempts", "next_attempt_at", "last_attempt_at",
			"delivered_at", "last_response_status", "last_response_body", "last_error", "created_at",
		}).
			AddRow(
				"delivery-1", "sub-1", "contact.created", `{"test": "data1"}`, domain.WebhookDeliveryStatusPending,
				0, 5, now, nil, nil, nil, nil, nil, now,
			).
			AddRow(
				"delivery-2", "sub-2", "contact.updated", `{"test": "data2"}`, domain.WebhookDeliveryStatusFailed,
				2, 5, now, now, nil, 500, "Server Error", "connection timeout", now,
			)

		mock.ExpectQuery(`SELECT .+ FROM webhook_deliveries WHERE status IN`).
			WithArgs(limit).
			WillReturnRows(rows)

		deliveries, err := repo.GetPendingForWorkspace(ctx, workspaceID, limit)
		assert.NoError(t, err)
		assert.Len(t, deliveries, 2)
		assert.Equal(t, "delivery-1", deliveries[0].ID)
		assert.Equal(t, "sub-1", deliveries[0].SubscriptionID)
		assert.Equal(t, domain.WebhookDeliveryStatusPending, deliveries[0].Status)
		assert.Equal(t, 0, deliveries[0].Attempts)
		assert.Nil(t, deliveries[0].LastAttemptAt)

		assert.Equal(t, "delivery-2", deliveries[1].ID)
		assert.Equal(t, domain.WebhookDeliveryStatusFailed, deliveries[1].Status)
		assert.Equal(t, 2, deliveries[1].Attempts)
		assert.NotNil(t, deliveries[1].LastAttemptAt)
		assert.NotNil(t, deliveries[1].LastResponseStatus)
		assert.Equal(t, 500, *deliveries[1].LastResponseStatus)
		assert.NotNil(t, deliveries[1].LastResponseBody)
		assert.Equal(t, "Server Error", *deliveries[1].LastResponseBody)
		assert.NotNil(t, deliveries[1].LastError)
		assert.Equal(t, "connection timeout", *deliveries[1].LastError)

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Success - Empty Result", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		rows := sqlmock.NewRows([]string{
			"id", "subscription_id", "event_type", "payload", "status",
			"attempts", "max_attempts", "next_attempt_at", "last_attempt_at",
			"delivered_at", "last_response_status", "last_response_body", "last_error", "created_at",
		})

		mock.ExpectQuery(`SELECT .+ FROM webhook_deliveries WHERE status IN`).
			WithArgs(limit).
			WillReturnRows(rows)

		deliveries, err := repo.GetPendingForWorkspace(ctx, workspaceID, limit)
		assert.NoError(t, err)
		assert.Empty(t, deliveries)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("ConnectionError", func(t *testing.T) {
		expectedErr := errors.New("connection error")
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(nil, expectedErr)

		deliveries, err := repo.GetPendingForWorkspace(ctx, workspaceID, limit)
		assert.Error(t, err)
		assert.Nil(t, deliveries)
		assert.Contains(t, err.Error(), "failed to get workspace connection")
	})

	t.Run("QueryError", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		expectedErr := errors.New("query error")
		mock.ExpectQuery(`SELECT .+ FROM webhook_deliveries WHERE status IN`).
			WithArgs(limit).
			WillReturnError(expectedErr)

		deliveries, err := repo.GetPendingForWorkspace(ctx, workspaceID, limit)
		assert.Error(t, err)
		assert.Nil(t, deliveries)
		assert.Contains(t, err.Error(), "failed to query pending deliveries")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("ScanError", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		// Wrong number of columns to cause scan error
		rows := sqlmock.NewRows([]string{"id", "subscription_id"}).
			AddRow("delivery-1", "sub-1")

		mock.ExpectQuery(`SELECT .+ FROM webhook_deliveries WHERE status IN`).
			WithArgs(limit).
			WillReturnRows(rows)

		deliveries, err := repo.GetPendingForWorkspace(ctx, workspaceID, limit)
		assert.Error(t, err)
		assert.Nil(t, deliveries)
		assert.Contains(t, err.Error(), "failed to scan delivery")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("InvalidJSONPayload", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		rows := sqlmock.NewRows([]string{
			"id", "subscription_id", "event_type", "payload", "status",
			"attempts", "max_attempts", "next_attempt_at", "last_attempt_at",
			"delivered_at", "last_response_status", "last_response_body", "last_error", "created_at",
		}).
			AddRow(
				"delivery-1", "sub-1", "contact.created", `{invalid json}`, domain.WebhookDeliveryStatusPending,
				0, 5, now, nil, nil, nil, nil, nil, now,
			)

		mock.ExpectQuery(`SELECT .+ FROM webhook_deliveries WHERE status IN`).
			WithArgs(limit).
			WillReturnRows(rows)

		deliveries, err := repo.GetPendingForWorkspace(ctx, workspaceID, limit)
		assert.Error(t, err)
		assert.Nil(t, deliveries)
		assert.Contains(t, err.Error(), "failed to scan delivery")
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

// TestWebhookDeliveryRepository_ListAll tests the ListAll method
func TestWebhookDeliveryRepository_ListAll(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewWebhookDeliveryRepository(mockWorkspaceRepo)

	ctx := context.Background()
	workspaceID := "ws-123"
	subscriptionID := "sub-456"
	limit := 10
	offset := 0
	now := time.Now().UTC()

	t.Run("Success - All deliveries without filter", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		// Mock count query (no WHERE clause)
		countRows := sqlmock.NewRows([]string{"count"}).AddRow(25)
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM webhook_deliveries$`).
			WillReturnRows(countRows)

		// Mock data query (no WHERE clause)
		rows := sqlmock.NewRows([]string{
			"id", "subscription_id", "event_type", "payload", "status",
			"attempts", "max_attempts", "next_attempt_at", "last_attempt_at",
			"delivered_at", "last_response_status", "last_response_body", "last_error", "created_at",
		}).
			AddRow(
				"delivery-1", "sub-1", "contact.created", `{"test": "data1"}`, domain.WebhookDeliveryStatusDelivered,
				1, 5, now, now, now, 200, "OK", nil, now,
			).
			AddRow(
				"delivery-2", "sub-2", "contact.updated", `{"test": "data2"}`, domain.WebhookDeliveryStatusPending,
				0, 5, now, nil, nil, nil, nil, nil, now,
			)

		mock.ExpectQuery(`SELECT .+ FROM webhook_deliveries\s+ORDER BY`).
			WithArgs(limit, offset).
			WillReturnRows(rows)

		deliveries, total, err := repo.ListAll(ctx, workspaceID, nil, limit, offset)
		assert.NoError(t, err)
		assert.Equal(t, 25, total)
		assert.Len(t, deliveries, 2)
		assert.Equal(t, "delivery-1", deliveries[0].ID)
		assert.Equal(t, "sub-1", deliveries[0].SubscriptionID)
		assert.Equal(t, "delivery-2", deliveries[1].ID)
		assert.Equal(t, "sub-2", deliveries[1].SubscriptionID)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Success - Filtered by subscription", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		// Mock count query (with WHERE clause)
		countRows := sqlmock.NewRows([]string{"count"}).AddRow(10)
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM webhook_deliveries WHERE subscription_id`).
			WithArgs(subscriptionID).
			WillReturnRows(countRows)

		// Mock data query (with WHERE clause)
		rows := sqlmock.NewRows([]string{
			"id", "subscription_id", "event_type", "payload", "status",
			"attempts", "max_attempts", "next_attempt_at", "last_attempt_at",
			"delivered_at", "last_response_status", "last_response_body", "last_error", "created_at",
		}).
			AddRow(
				"delivery-1", subscriptionID, "contact.created", `{"test": "data1"}`, domain.WebhookDeliveryStatusDelivered,
				1, 5, now, now, now, 200, "OK", nil, now,
			)

		mock.ExpectQuery(`SELECT .+ FROM webhook_deliveries\s+WHERE subscription_id`).
			WithArgs(subscriptionID, limit, offset).
			WillReturnRows(rows)

		deliveries, total, err := repo.ListAll(ctx, workspaceID, &subscriptionID, limit, offset)
		assert.NoError(t, err)
		assert.Equal(t, 10, total)
		assert.Len(t, deliveries, 1)
		assert.Equal(t, subscriptionID, deliveries[0].SubscriptionID)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("ConnectionError", func(t *testing.T) {
		expectedErr := errors.New("connection error")
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(nil, expectedErr)

		deliveries, total, err := repo.ListAll(ctx, workspaceID, nil, limit, offset)
		assert.Error(t, err)
		assert.Nil(t, deliveries)
		assert.Equal(t, 0, total)
		assert.Contains(t, err.Error(), "failed to get workspace connection")
	})

	t.Run("CountQueryError - No filter", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		expectedErr := errors.New("count query error")
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM webhook_deliveries$`).
			WillReturnError(expectedErr)

		deliveries, total, err := repo.ListAll(ctx, workspaceID, nil, limit, offset)
		assert.Error(t, err)
		assert.Nil(t, deliveries)
		assert.Equal(t, 0, total)
		assert.Contains(t, err.Error(), "failed to count deliveries")
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

// TestWebhookDeliveryRepository_UpdateStatus tests the UpdateStatus method
func TestWebhookDeliveryRepository_UpdateStatus(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewWebhookDeliveryRepository(mockWorkspaceRepo)

	ctx := context.Background()
	workspaceID := "ws-123"
	deliveryID := "delivery-456"
	status := domain.WebhookDeliveryStatusDelivering
	attempts := 1
	responseStatus := 200
	responseBody := "Success"
	lastError := "timeout"

	t.Run("Success - All Fields", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		mock.ExpectExec(`UPDATE webhook_deliveries SET status`).
			WithArgs(
				deliveryID,
				status,
				attempts,
				sqlmock.AnyArg(), // last_attempt_at
				&responseStatus,
				&responseBody,
				&lastError,
			).
			WillReturnResult(sqlmock.NewResult(0, 1))

		err = repo.UpdateStatus(ctx, workspaceID, deliveryID, status, attempts, &responseStatus, &responseBody, &lastError)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Success - Nil Optional Fields", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		mock.ExpectExec(`UPDATE webhook_deliveries SET status`).
			WithArgs(
				deliveryID,
				status,
				attempts,
				sqlmock.AnyArg(), // last_attempt_at
				nil,
				nil,
				nil,
			).
			WillReturnResult(sqlmock.NewResult(0, 1))

		err = repo.UpdateStatus(ctx, workspaceID, deliveryID, status, attempts, nil, nil, nil)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("ConnectionError", func(t *testing.T) {
		expectedErr := errors.New("connection error")
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(nil, expectedErr)

		err := repo.UpdateStatus(ctx, workspaceID, deliveryID, status, attempts, nil, nil, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get workspace connection")
	})

	t.Run("ExecError", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		expectedErr := errors.New("database error")
		mock.ExpectExec(`UPDATE webhook_deliveries SET status`).
			WillReturnError(expectedErr)

		err = repo.UpdateStatus(ctx, workspaceID, deliveryID, status, attempts, nil, nil, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to update delivery status")
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

// TestWebhookDeliveryRepository_MarkDelivered tests the MarkDelivered method
func TestWebhookDeliveryRepository_MarkDelivered(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewWebhookDeliveryRepository(mockWorkspaceRepo)

	ctx := context.Background()
	workspaceID := "ws-123"
	deliveryID := "delivery-456"
	responseStatus := 200
	responseBody := "Success"

	t.Run("Success", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		mock.ExpectExec(`UPDATE webhook_deliveries SET status = 'delivered'`).
			WithArgs(
				deliveryID,
				sqlmock.AnyArg(), // delivered_at/last_attempt_at (same timestamp)
				responseStatus,
				responseBody,
			).
			WillReturnResult(sqlmock.NewResult(0, 1))

		err = repo.MarkDelivered(ctx, workspaceID, deliveryID, responseStatus, responseBody)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Success - Truncate Long Response Body", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		// Create a response body longer than 1024 bytes
		longResponseBody := string(make([]byte, 2048))
		for i := range longResponseBody {
			longResponseBody = longResponseBody[:i] + "x" + longResponseBody[i+1:]
		}

		mock.ExpectExec(`UPDATE webhook_deliveries SET status = 'delivered'`).
			WithArgs(
				deliveryID,
				sqlmock.AnyArg(), // delivered_at/last_attempt_at
				responseStatus,
				longResponseBody[:1024], // Should be truncated
			).
			WillReturnResult(sqlmock.NewResult(0, 1))

		err = repo.MarkDelivered(ctx, workspaceID, deliveryID, responseStatus, longResponseBody)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("ConnectionError", func(t *testing.T) {
		expectedErr := errors.New("connection error")
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(nil, expectedErr)

		err := repo.MarkDelivered(ctx, workspaceID, deliveryID, responseStatus, responseBody)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get workspace connection")
	})

	t.Run("ExecError", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		expectedErr := errors.New("database error")
		mock.ExpectExec(`UPDATE webhook_deliveries SET status = 'delivered'`).
			WillReturnError(expectedErr)

		err = repo.MarkDelivered(ctx, workspaceID, deliveryID, responseStatus, responseBody)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to mark delivery as delivered")
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

// TestWebhookDeliveryRepository_ScheduleRetry tests the ScheduleRetry method
func TestWebhookDeliveryRepository_ScheduleRetry(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewWebhookDeliveryRepository(mockWorkspaceRepo)

	ctx := context.Background()
	workspaceID := "ws-123"
	deliveryID := "delivery-456"
	nextAttempt := time.Now().UTC().Add(5 * time.Minute)
	attempts := 2
	responseStatus := 500
	responseBody := "Internal Server Error"
	lastError := "connection timeout"

	t.Run("Success - All Fields", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		mock.ExpectExec(`UPDATE webhook_deliveries SET status = 'failed'`).
			WithArgs(
				deliveryID,
				attempts,
				nextAttempt,
				sqlmock.AnyArg(), // last_attempt_at
				&responseStatus,
				&responseBody,
				&lastError,
			).
			WillReturnResult(sqlmock.NewResult(0, 1))

		err = repo.ScheduleRetry(ctx, workspaceID, deliveryID, nextAttempt, attempts, &responseStatus, &responseBody, &lastError)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Success - Nil Optional Fields", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		mock.ExpectExec(`UPDATE webhook_deliveries SET status = 'failed'`).
			WithArgs(
				deliveryID,
				attempts,
				nextAttempt,
				sqlmock.AnyArg(), // last_attempt_at
				nil,
				nil,
				nil,
			).
			WillReturnResult(sqlmock.NewResult(0, 1))

		err = repo.ScheduleRetry(ctx, workspaceID, deliveryID, nextAttempt, attempts, nil, nil, nil)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Success - Truncate Long Response Body", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		// Create a response body longer than 1024 bytes
		longBody := make([]byte, 2048)
		for i := range longBody {
			longBody[i] = 'x'
		}
		longResponseBody := string(longBody)
		truncatedBody := longResponseBody[:1024]

		mock.ExpectExec(`UPDATE webhook_deliveries SET status = 'failed'`).
			WithArgs(
				deliveryID,
				attempts,
				nextAttempt,
				sqlmock.AnyArg(),
				&responseStatus,
				&truncatedBody,
				&lastError,
			).
			WillReturnResult(sqlmock.NewResult(0, 1))

		err = repo.ScheduleRetry(ctx, workspaceID, deliveryID, nextAttempt, attempts, &responseStatus, &longResponseBody, &lastError)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("ConnectionError", func(t *testing.T) {
		expectedErr := errors.New("connection error")
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(nil, expectedErr)

		err := repo.ScheduleRetry(ctx, workspaceID, deliveryID, nextAttempt, attempts, nil, nil, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get workspace connection")
	})

	t.Run("ExecError", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		expectedErr := errors.New("database error")
		mock.ExpectExec(`UPDATE webhook_deliveries SET status = 'failed'`).
			WillReturnError(expectedErr)

		err = repo.ScheduleRetry(ctx, workspaceID, deliveryID, nextAttempt, attempts, nil, nil, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to schedule retry")
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

// TestWebhookDeliveryRepository_MarkFailed tests the MarkFailed method
func TestWebhookDeliveryRepository_MarkFailed(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewWebhookDeliveryRepository(mockWorkspaceRepo)

	ctx := context.Background()
	workspaceID := "ws-123"
	deliveryID := "delivery-456"
	attempts := 5
	lastError := "max retries exceeded"
	responseStatus := 500
	responseBody := "Internal Server Error"

	t.Run("Success - All Fields", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		mock.ExpectExec(`UPDATE webhook_deliveries SET status = 'failed'`).
			WithArgs(
				deliveryID,
				attempts,
				sqlmock.AnyArg(), // last_attempt_at
				&responseStatus,
				&responseBody,
				lastError,
			).
			WillReturnResult(sqlmock.NewResult(0, 1))

		err = repo.MarkFailed(ctx, workspaceID, deliveryID, attempts, lastError, &responseStatus, &responseBody)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Success - Nil Optional Fields", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		mock.ExpectExec(`UPDATE webhook_deliveries SET status = 'failed'`).
			WithArgs(
				deliveryID,
				attempts,
				sqlmock.AnyArg(), // last_attempt_at
				nil,
				nil,
				lastError,
			).
			WillReturnResult(sqlmock.NewResult(0, 1))

		err = repo.MarkFailed(ctx, workspaceID, deliveryID, attempts, lastError, nil, nil)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Success - Truncate Long Response Body", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		// Create a response body longer than 1024 bytes
		longBody := make([]byte, 2048)
		for i := range longBody {
			longBody[i] = 'y'
		}
		longResponseBody := string(longBody)
		truncatedBody := longResponseBody[:1024]

		mock.ExpectExec(`UPDATE webhook_deliveries SET status = 'failed'`).
			WithArgs(
				deliveryID,
				attempts,
				sqlmock.AnyArg(),
				&responseStatus,
				&truncatedBody,
				lastError,
			).
			WillReturnResult(sqlmock.NewResult(0, 1))

		err = repo.MarkFailed(ctx, workspaceID, deliveryID, attempts, lastError, &responseStatus, &longResponseBody)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("ConnectionError", func(t *testing.T) {
		expectedErr := errors.New("connection error")
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(nil, expectedErr)

		err := repo.MarkFailed(ctx, workspaceID, deliveryID, attempts, lastError, nil, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get workspace connection")
	})

	t.Run("ExecError", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		expectedErr := errors.New("database error")
		mock.ExpectExec(`UPDATE webhook_deliveries SET status = 'failed'`).
			WillReturnError(expectedErr)

		err = repo.MarkFailed(ctx, workspaceID, deliveryID, attempts, lastError, nil, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to mark delivery as failed")
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}
