package repository

import (
	"context"
	"errors"
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
			"delivered_at", "last_response_status", "last_response_body", "last_error",
			"claimed_at", "created_at",
		}).
			AddRow(
				"delivery-1", "sub-1", "contact.created", `{"test": "data1"}`, domain.WebhookDeliveryStatusDelivering,
				0, 5, now, nil, nil, nil, nil, nil, now, now,
			).
			AddRow(
				"delivery-2", "sub-2", "contact.updated", `{"test": "data2"}`, domain.WebhookDeliveryStatusDelivering,
				2, 5, now, now, nil, 500, "Server Error", "connection timeout", now, now,
			)

		mock.ExpectQuery(`UPDATE webhook_deliveries SET status = 'delivering', claimed_at = NOW\(\)`).
			WithArgs(limit).
			WillReturnRows(rows)

		deliveries, err := repo.GetPendingForWorkspace(ctx, workspaceID, limit)
		assert.NoError(t, err)
		assert.Len(t, deliveries, 2)
		assert.Equal(t, "delivery-1", deliveries[0].ID)
		assert.Equal(t, "sub-1", deliveries[0].SubscriptionID)
		// RETURNING reports the row as the claim left it, so every delivery
		// handed to the caller is already this worker's to finish.
		assert.Equal(t, domain.WebhookDeliveryStatusDelivering, deliveries[0].Status)
		assert.NotNil(t, deliveries[0].ClaimedAt)
		assert.Equal(t, 0, deliveries[0].Attempts)
		assert.Nil(t, deliveries[0].LastAttemptAt)

		assert.Equal(t, "delivery-2", deliveries[1].ID)
		assert.Equal(t, domain.WebhookDeliveryStatusDelivering, deliveries[1].Status)
		assert.NotNil(t, deliveries[1].ClaimedAt)
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
			"delivered_at", "last_response_status", "last_response_body", "last_error",
			"claimed_at", "created_at",
		})

		mock.ExpectQuery(`UPDATE webhook_deliveries SET status = 'delivering', claimed_at = NOW\(\)`).
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
		mock.ExpectQuery(`UPDATE webhook_deliveries SET status = 'delivering', claimed_at = NOW\(\)`).
			WithArgs(limit).
			WillReturnError(expectedErr)

		deliveries, err := repo.GetPendingForWorkspace(ctx, workspaceID, limit)
		assert.Error(t, err)
		assert.Nil(t, deliveries)
		assert.Contains(t, err.Error(), "failed to claim pending deliveries")
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

		mock.ExpectQuery(`UPDATE webhook_deliveries SET status = 'delivering', claimed_at = NOW\(\)`).
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
			"delivered_at", "last_response_status", "last_response_body", "last_error",
			"claimed_at", "created_at",
		}).
			AddRow(
				"delivery-1", "sub-1", "contact.created", `{invalid json}`, domain.WebhookDeliveryStatusPending,
				0, 5, now, nil, nil, nil, nil, nil, nil, now,
			)

		mock.ExpectQuery(`UPDATE webhook_deliveries SET status = 'delivering', claimed_at = NOW\(\)`).
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
			"delivered_at", "last_response_status", "last_response_body", "last_error",
			"claimed_at", "created_at",
		}).
			AddRow(
				"delivery-1", "sub-1", "contact.created", `{"test": "data1"}`, domain.WebhookDeliveryStatusDelivered,
				1, 5, now, now, now, 200, "OK", nil, nil, now,
			).
			AddRow(
				"delivery-2", "sub-2", "contact.updated", `{"test": "data2"}`, domain.WebhookDeliveryStatusPending,
				0, 5, now, nil, nil, nil, nil, nil, nil, now,
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
			"delivered_at", "last_response_status", "last_response_body", "last_error",
			"claimed_at", "created_at",
		}).
			AddRow(
				"delivery-1", subscriptionID, "contact.created", `{"test": "data1"}`, domain.WebhookDeliveryStatusDelivered,
				1, 5, now, now, now, 200, "OK", nil, nil, now,
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

// normalizeWebhookSQL collapses the whitespace of a multi-line statement so a
// test can assert on its text without also asserting on its indentation.
func normalizeWebhookSQL(query string) string {
	return strings.Join(strings.Fields(query), " ")
}

// captureWebhookSQL records the statement the repository actually sent before
// delegating to sqlmock's usual regexp matching. Some of the properties these
// repositories depend on live in the SQL itself — which status values the claim
// predicate excludes, whether an UPDATE touches a column at all — and sqlmock
// cannot execute a statement to find out.
func captureWebhookSQL(captured *string) sqlmock.QueryMatcher {
	return sqlmock.QueryMatcherFunc(func(expectedSQL, actualSQL string) error {
		*captured = normalizeWebhookSQL(actualSQL)
		return sqlmock.QueryMatcherRegexp.Match(expectedSQL, actualSQL)
	})
}

// claimSubquery returns the body of the `WHERE id IN ( ... )` subquery that
// picks the rows a claim updates, so a test can assert on the selection
// predicate separately from the SET clause — the SET clause names 'delivering'
// legitimately, and the predicate must not.
func claimSubquery(t *testing.T, query string) string {
	t.Helper()

	normalized := normalizeWebhookSQL(query)
	const marker = "WHERE id IN ("
	start := strings.Index(normalized, marker)
	require.NotEqual(t, -1, start, "the claim must select the rows it updates in a subquery: %s", normalized)

	depth := 1
	for i := start + len(marker); i < len(normalized); i++ {
		switch normalized[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return normalized[start+len(marker) : i]
			}
		}
	}

	t.Fatalf("unbalanced parentheses in claim statement: %s", normalized)
	return ""
}

// TestWebhookDeliveryRepository_GetPendingForWorkspace_Claims pins the shape of
// the claim, because the properties that make it a claim are properties of the
// statement and sqlmock cannot execute one.
func TestWebhookDeliveryRepository_GetPendingForWorkspace_Claims(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewWebhookDeliveryRepository(mockWorkspaceRepo)

	ctx := context.Background()
	workspaceID := "ws-123"
	now := time.Now().UTC()

	var actualSQL string
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(captureWebhookSQL(&actualSQL)))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mockWorkspaceRepo.EXPECT().
		GetConnection(gomock.Any(), workspaceID).
		Return(db, nil)

	rows := sqlmock.NewRows([]string{
		"id", "subscription_id", "event_type", "payload", "status",
		"attempts", "max_attempts", "next_attempt_at", "last_attempt_at",
		"delivered_at", "last_response_status", "last_response_body", "last_error",
		"claimed_at", "created_at",
	}).AddRow(
		"delivery-1", "sub-1", "contact.created", `{"test": "data"}`, domain.WebhookDeliveryStatusDelivering,
		0, 5, now, nil, nil, nil, nil, nil, now, now,
	)

	mock.ExpectQuery(`UPDATE webhook_deliveries SET status = 'delivering', claimed_at = NOW\(\)`).
		WithArgs(100).
		WillReturnRows(rows)

	deliveries, err := repo.GetPendingForWorkspace(ctx, workspaceID, 100)
	require.NoError(t, err)
	require.Len(t, deliveries, 1)
	assert.Equal(t, domain.WebhookDeliveryStatusDelivering, deliveries[0].Status)
	require.NotNil(t, deliveries[0].ClaimedAt)

	// The row is taken and stamped by the same statement that selected it. A
	// separate UPDATE after the read would leave a window in which a second
	// worker sees the same row as pending.
	assert.Contains(t, actualSQL, "SET status = 'delivering', claimed_at = NOW()")
	assert.Contains(t, actualSQL, "RETURNING")

	selection := claimSubquery(t, actualSQL)
	// The load-bearing absence: were 'delivering' listed here, a claimed row
	// would keep matching the predicate and every poll would hand it out again,
	// which is the entire thing the claim exists to prevent. It also means no
	// lease comparison is needed to skip a row that is in flight — an in-flight
	// row is not in the predicate at all.
	assert.NotContains(t, selection, "delivering",
		"a claimed row must not match the selection predicate: %s", selection)
	// Without SKIP LOCKED a second worker queues behind the first instead of
	// walking past the contended rows, turning a concurrent poll into a stall.
	assert.Contains(t, selection, "FOR UPDATE SKIP LOCKED")
	assert.Contains(t, selection, "attempts < max_attempts")
	assert.Contains(t, selection, "next_attempt_at <= NOW()")

	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestWebhookDeliveryRepository_GetPendingForWorkspace_ConcurrentClaimsAreDisjoint
// walks two workers through the same queue. sqlmock cannot execute the claim,
// so the table's state is simulated here — rows the first call took have left
// the pending predicate by the time the second call runs, which is exactly what
// TestWebhookDeliveryRepository_GetPendingForWorkspace_Claims pins in the
// statement itself. What this adds is that the repository hands back precisely
// the rows its statement returned, with nothing carried over between calls.
func TestWebhookDeliveryRepository_GetPendingForWorkspace_ConcurrentClaimsAreDisjoint(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewWebhookDeliveryRepository(mockWorkspaceRepo)

	ctx := context.Background()
	workspaceID := "ws-123"
	now := time.Now().UTC()

	queue := []string{"delivery-1", "delivery-2", "delivery-3"}
	claimRows := func(ids []string) *sqlmock.Rows {
		rows := sqlmock.NewRows([]string{
			"id", "subscription_id", "event_type", "payload", "status",
			"attempts", "max_attempts", "next_attempt_at", "last_attempt_at",
			"delivered_at", "last_response_status", "last_response_body", "last_error",
			"claimed_at", "created_at",
		})
		for _, id := range ids {
			rows.AddRow(
				id, "sub-1", "contact.created", `{"test": "data"}`, domain.WebhookDeliveryStatusDelivering,
				0, 5, now, nil, nil, nil, nil, nil, now, now,
			)
		}
		return rows
	}

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mockWorkspaceRepo.EXPECT().
		GetConnection(gomock.Any(), workspaceID).
		Times(2).
		Return(db, nil)

	mock.ExpectQuery(`UPDATE webhook_deliveries SET status = 'delivering'`).
		WithArgs(2).
		WillReturnRows(claimRows(queue[:2]))
	mock.ExpectQuery(`UPDATE webhook_deliveries SET status = 'delivering'`).
		WithArgs(2).
		WillReturnRows(claimRows(queue[2:]))

	first, err := repo.GetPendingForWorkspace(ctx, workspaceID, 2)
	require.NoError(t, err)
	second, err := repo.GetPendingForWorkspace(ctx, workspaceID, 2)
	require.NoError(t, err)

	claimed := map[string]bool{}
	for _, d := range first {
		claimed[d.ID] = true
	}
	require.NotEmpty(t, second)
	for _, d := range second {
		assert.False(t, claimed[d.ID],
			"a row claimed by the first worker was handed to the second: %s", d.ID)
	}
	assert.Len(t, append(first, second...), len(queue))
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestWebhookDeliveryRepository_StatusWritersReleaseTheClaim covers the other
// half of the claim's contract. A worker that finishes with a row without
// clearing claimed_at leaves the table saying the row is still in flight, and
// the invariant that lets ReclaimStale recognise a stranded row — in
// 'delivering' if and only if claimed — stops holding.
func TestWebhookDeliveryRepository_StatusWritersReleaseTheClaim(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewWebhookDeliveryRepository(mockWorkspaceRepo)

	ctx := context.Background()
	workspaceID := "ws-123"
	deliveryID := "delivery-456"

	testCases := []struct {
		name     string
		expected []string
		call     func(repo domain.WebhookDeliveryRepository) error
	}{
		{
			name:     "MarkDelivered",
			expected: []string{"claimed_at = NULL"},
			call: func(repo domain.WebhookDeliveryRepository) error {
				return repo.MarkDelivered(ctx, workspaceID, deliveryID, 200, "OK")
			},
		},
		{
			name:     "ScheduleRetry",
			expected: []string{"claimed_at = NULL"},
			call: func(repo domain.WebhookDeliveryRepository) error {
				return repo.ScheduleRetry(ctx, workspaceID, deliveryID, time.Now().Add(time.Minute), 2, nil, nil, nil)
			},
		},
		{
			name:     "MarkFailed",
			expected: []string{"claimed_at = NULL"},
			call: func(repo domain.WebhookDeliveryRepository) error {
				return repo.MarkFailed(ctx, workspaceID, deliveryID, 10, "gave up", nil, nil)
			},
		},
		{
			// UpdateStatus is the one writer whose status is chosen by the
			// caller, so it releases the claim for every status but the one
			// that means "still mine". The cast on the assignment is what makes
			// that legal: reading the same parameter as text in the CASE and as
			// the column's varchar in the SET makes PostgreSQL refuse to deduce
			// a type at all, and nothing short of a real server notices.
			name: "UpdateStatus",
			expected: []string{
				"status = $2::varchar",
				"claimed_at = CASE WHEN $2 = 'delivering' THEN claimed_at ELSE NULL END",
			},
			call: func(repo domain.WebhookDeliveryRepository) error {
				return repo.UpdateStatus(ctx, workspaceID, deliveryID, domain.WebhookDeliveryStatusPending, 1, nil, nil, nil)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var actualSQL string
			db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(captureWebhookSQL(&actualSQL)))
			require.NoError(t, err)
			defer func() { _ = db.Close() }()

			mockWorkspaceRepo.EXPECT().
				GetConnection(gomock.Any(), workspaceID).
				Return(db, nil)

			mock.ExpectExec(`UPDATE webhook_deliveries SET`).
				WillReturnResult(sqlmock.NewResult(0, 1))

			require.NoError(t, tc.call(repo))
			for _, fragment := range tc.expected {
				assert.Contains(t, actualSQL, fragment)
			}
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// TestWebhookDeliveryRepository_DeleteBySubscriptionID covers the sweep that
// runs when a subscription goes away.
func TestWebhookDeliveryRepository_DeleteBySubscriptionID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewWebhookDeliveryRepository(mockWorkspaceRepo)

	ctx := context.Background()
	workspaceID := "ws-123"
	subscriptionID := "sub-456"

	t.Run("Deletes only that subscription's deliveries", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		// Anchored: the statement ends at the subscription filter, so a stray
		// OR or a dropped WHERE would take the whole workspace's queue with it.
		mock.ExpectExec(`^DELETE FROM webhook_deliveries WHERE subscription_id = \$1$`).
			WithArgs(subscriptionID).
			WillReturnResult(sqlmock.NewResult(0, 3))

		err = repo.DeleteBySubscriptionID(ctx, workspaceID, subscriptionID)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("A subscription with no queued deliveries is not an error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		mock.ExpectExec(`DELETE FROM webhook_deliveries WHERE subscription_id`).
			WithArgs(subscriptionID).
			WillReturnResult(sqlmock.NewResult(0, 0))

		err = repo.DeleteBySubscriptionID(ctx, workspaceID, subscriptionID)
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("ConnectionError", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(nil, errors.New("connection error"))

		err := repo.DeleteBySubscriptionID(ctx, workspaceID, subscriptionID)
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

		mock.ExpectExec(`DELETE FROM webhook_deliveries WHERE subscription_id`).
			WillReturnError(errors.New("database error"))

		err = repo.DeleteBySubscriptionID(ctx, workspaceID, subscriptionID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to delete deliveries for subscription")
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

// TestWebhookDeliveryRepository_ReclaimStale covers the sweep that undoes a
// claim its worker never released.
func TestWebhookDeliveryRepository_ReclaimStale(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewWebhookDeliveryRepository(mockWorkspaceRepo)

	ctx := context.Background()
	workspaceID := "ws-123"

	// Two different leases, because the lease belongs to the caller: it is the
	// HTTP timeout plus a little, and a repository that measured it against a
	// constant of its own would either strand rows or cut live deliveries short.
	testCases := []struct {
		name      string
		lease     time.Duration
		reclaimed int64
	}{
		{name: "short lease", lease: 15 * time.Second, reclaimed: 2},
		{name: "longer lease reclaims nothing yet", lease: 5 * time.Minute, reclaimed: 0},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var actualSQL string
			db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(captureWebhookSQL(&actualSQL)))
			require.NoError(t, err)
			defer func() { _ = db.Close() }()

			mockWorkspaceRepo.EXPECT().
				GetConnection(gomock.Any(), workspaceID).
				Return(db, nil)

			mock.ExpectExec(`UPDATE webhook_deliveries SET status = 'pending', claimed_at = NULL WHERE status = 'delivering'`).
				WithArgs(tc.lease.Seconds()).
				WillReturnResult(sqlmock.NewResult(0, tc.reclaimed))

			count, err := repo.ReclaimStale(ctx, workspaceID, tc.lease)
			require.NoError(t, err)
			assert.Equal(t, tc.reclaimed, count)

			// The row goes back to 'pending' with its claim cleared, so the next
			// poll can take it; a row still inside its lease fails the age test
			// and is left in flight.
			assert.Contains(t, actualSQL, "SET status = 'pending', claimed_at = NULL")
			assert.Contains(t, actualSQL, "claimed_at < NOW() - INTERVAL '1 second' * $1")
			// A row in 'delivering' with no claim at all — left by a build that
			// predates the claim — matches no age test, so it is swept too
			// rather than sitting outside every predicate forever.
			assert.Contains(t, actualSQL, "claimed_at IS NULL OR")

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}

	t.Run("ConnectionError", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(nil, errors.New("connection error"))

		count, err := repo.ReclaimStale(ctx, workspaceID, time.Minute)
		assert.Error(t, err)
		assert.Zero(t, count)
		assert.Contains(t, err.Error(), "failed to get workspace connection")
	})

	t.Run("ExecError", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		mock.ExpectExec(`UPDATE webhook_deliveries SET status = 'pending'`).
			WillReturnError(errors.New("database error"))

		count, err := repo.ReclaimStale(ctx, workspaceID, time.Minute)
		assert.Error(t, err)
		assert.Zero(t, count)
		assert.Contains(t, err.Error(), "failed to reclaim stale deliveries")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("RowsAffectedError", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		mock.ExpectExec(`UPDATE webhook_deliveries SET status = 'pending'`).
			WillReturnResult(sqlmock.NewErrorResult(errors.New("rows affected error")))

		count, err := repo.ReclaimStale(ctx, workspaceID, time.Minute)
		assert.Error(t, err)
		assert.Zero(t, count)
		assert.Contains(t, err.Error(), "failed to get rows affected")
	})
}
