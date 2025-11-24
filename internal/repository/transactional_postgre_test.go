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
	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTransactionalTest(t *testing.T) (*mocks.MockWorkspaceRepository, domain.TransactionalNotificationRepository, sqlmock.Sqlmock, *sql.DB, func()) {
	ctrl := gomock.NewController(t)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)

	// Create a real DB connection with sqlmock
	db, mock, err := sqlmock.New()
	require.NoError(t, err)

	repo := NewTransactionalNotificationRepository(mockWorkspaceRepo)

	// Set up cleanup function
	cleanup := func() {
		_ = db.Close()
		ctrl.Finish()
	}

	return mockWorkspaceRepo, repo, mock, db, cleanup
}

func createSampleTransactionalNotification() *domain.TransactionalNotification {
	now := time.Now().UTC()
	integrationID := "integration-123"
	return &domain.TransactionalNotification{
		ID:            "trans-123",
		Name:          "Welcome Email",
		Description:   "Sent when a user signs up",
		IntegrationID: &integrationID,
		Channels: domain.ChannelTemplates{
			domain.TransactionalChannelEmail: domain.ChannelTemplate{
				TemplateID: "template-123",
				Settings: domain.MapOfAny{
					"subject": "Welcome",
				},
			},
		},
		TrackingSettings: notifuse_mjml.TrackingSettings{
			EnableTracking: true,
			UTMSource:      "notifuse",
			UTMMedium:      "email",
			UTMCampaign:    "welcome",
		},
		Metadata: map[string]interface{}{
			"category": "onboarding",
			"priority": "high",
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func TestTransactionalNotificationRepository_Create(t *testing.T) {
	mockWorkspaceRepo, repo, mock, db, cleanup := setupTransactionalTest(t)
	defer cleanup()

	ctx := context.Background()
	workspaceID := "workspace-123"
	notification := createSampleTransactionalNotification()

	t.Run("successful creation", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		mock.ExpectExec(`INSERT INTO transactional_notifications`).
			WithArgs(
				notification.ID,
				notification.Name,
				notification.Description,
				sqlmock.AnyArg(), // channels (complex type)
				sqlmock.AnyArg(), // tracking_settings (complex type)
				sqlmock.AnyArg(), // metadata (complex type)
				notification.IntegrationID,
				sqlmock.AnyArg(), // created_at
				sqlmock.AnyArg(), // updated_at
			).
			WillReturnResult(sqlmock.NewResult(1, 1))

		err := repo.Create(ctx, workspaceID, notification)
		require.NoError(t, err)
	})

	t.Run("workspace connection error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(nil, errors.New("connection error"))

		err := repo.Create(ctx, workspaceID, notification)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to get workspace db")
	})

	t.Run("execution error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		mock.ExpectExec(`INSERT INTO transactional_notifications`).
			WithArgs(
				notification.ID,
				notification.Name,
				notification.Description,
				sqlmock.AnyArg(), // channels (complex type)
				sqlmock.AnyArg(), // tracking_settings (complex type)
				sqlmock.AnyArg(), // metadata (complex type)
				notification.IntegrationID,
				sqlmock.AnyArg(), // created_at
				sqlmock.AnyArg(), // updated_at
			).
			WillReturnError(errors.New("execution error"))

		err := repo.Create(ctx, workspaceID, notification)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to create transactional notification")
	})
}

func TestTransactionalNotificationRepository_Update(t *testing.T) {
	mockWorkspaceRepo, repo, mock, db, cleanup := setupTransactionalTest(t)
	defer cleanup()

	ctx := context.Background()
	workspaceID := "workspace-123"
	notification := createSampleTransactionalNotification()

	t.Run("successful update", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		mock.ExpectExec(`UPDATE transactional_notifications SET`).
			WithArgs(
				notification.Name,
				notification.Description,
				sqlmock.AnyArg(), // channels (complex type)
				sqlmock.AnyArg(), // tracking_settings (complex type)
				sqlmock.AnyArg(), // metadata (complex type)
				notification.IntegrationID,
				sqlmock.AnyArg(), // updated_at
				notification.ID,
			).
			WillReturnResult(sqlmock.NewResult(1, 1))

		err := repo.Update(ctx, workspaceID, notification)
		require.NoError(t, err)
	})

	t.Run("workspace connection error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(nil, errors.New("connection error"))

		err := repo.Update(ctx, workspaceID, notification)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to get workspace db")
	})

	t.Run("execution error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		mock.ExpectExec(`UPDATE transactional_notifications SET`).
			WithArgs(
				notification.Name,
				notification.Description,
				sqlmock.AnyArg(), // channels (complex type)
				sqlmock.AnyArg(), // tracking_settings (complex type)
				sqlmock.AnyArg(), // metadata (complex type)
				notification.IntegrationID,
				sqlmock.AnyArg(), // updated_at
				notification.ID,
			).
			WillReturnError(errors.New("execution error"))

		err := repo.Update(ctx, workspaceID, notification)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to update transactional notification")
	})

	t.Run("notification not found", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		mock.ExpectExec(`UPDATE transactional_notifications SET`).
			WithArgs(
				notification.Name,
				notification.Description,
				sqlmock.AnyArg(), // channels (complex type)
				sqlmock.AnyArg(), // tracking_settings (complex type)
				sqlmock.AnyArg(), // metadata (complex type)
				notification.IntegrationID,
				sqlmock.AnyArg(), // updated_at
				notification.ID,
			).
			WillReturnResult(sqlmock.NewResult(0, 0))

		err := repo.Update(ctx, workspaceID, notification)
		require.Error(t, err)
		require.Contains(t, err.Error(), "transactional notification not found")
	})

	t.Run("rows affected error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		// Create a custom result that returns an error for RowsAffected
		result := sqlmock.NewErrorResult(errors.New("rows affected error"))

		mock.ExpectExec(`UPDATE transactional_notifications SET`).
			WithArgs(
				notification.Name,
				notification.Description,
				sqlmock.AnyArg(), // channels (complex type)
				sqlmock.AnyArg(), // tracking_settings (complex type)
				sqlmock.AnyArg(), // metadata (complex type)
				notification.IntegrationID,
				sqlmock.AnyArg(), // updated_at
				notification.ID,
			).
			WillReturnResult(result)

		err := repo.Update(ctx, workspaceID, notification)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to get rows affected")
	})
}

func TestTransactionalNotificationRepository_Get(t *testing.T) {
	mockWorkspaceRepo, repo, mock, db, cleanup := setupTransactionalTest(t)
	defer cleanup()

	ctx := context.Background()
	workspaceID := "workspace-123"
	notificationID := "trans-123"
	notification := createSampleTransactionalNotification()

	t.Run("successful retrieval", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		// Create JSON representation of complex types for row response
		channelsJSON, err := json.Marshal(notification.Channels)
		require.NoError(t, err)
		trackingSettingsJSON, err := json.Marshal(notification.TrackingSettings)
		require.NoError(t, err)
		metadataJSON, err := json.Marshal(notification.Metadata)
		require.NoError(t, err)

		rows := sqlmock.NewRows([]string{
			"id", "name", "description", "channels", "tracking_settings",
			"metadata", "integration_id", "created_at", "updated_at", "deleted_at",
		}).AddRow(
			notification.ID,
			notification.Name,
			notification.Description,
			channelsJSON,
			trackingSettingsJSON,
			metadataJSON,
			notification.IntegrationID,
			notification.CreatedAt,
			notification.UpdatedAt,
			nil, // deleted_at is nil
		)

		mock.ExpectQuery(`SELECT .* FROM transactional_notifications WHERE id = \$1 AND deleted_at IS NULL`).
			WithArgs(notificationID).
			WillReturnRows(rows)

		result, err := repo.Get(ctx, workspaceID, notificationID)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, notification.ID, result.ID)
		assert.Equal(t, notification.Name, result.Name)
		assert.Equal(t, notification.Description, result.Description)
		assert.Equal(t, notification.TrackingSettings.EnableTracking, result.TrackingSettings.EnableTracking)
	})

	t.Run("not found", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		mock.ExpectQuery(`SELECT .* FROM transactional_notifications WHERE id = \$1 AND deleted_at IS NULL`).
			WithArgs(notificationID).
			WillReturnError(sql.ErrNoRows)

		result, err := repo.Get(ctx, workspaceID, notificationID)
		require.Error(t, err)
		require.Nil(t, result)
		require.Contains(t, err.Error(), "transactional notification not found")
	})

	t.Run("workspace connection error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(nil, errors.New("connection error"))

		result, err := repo.Get(ctx, workspaceID, notificationID)
		require.Error(t, err)
		require.Nil(t, result)
		require.Contains(t, err.Error(), "failed to get workspace db")
	})

	t.Run("query error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		mock.ExpectQuery(`SELECT .* FROM transactional_notifications WHERE id = \$1 AND deleted_at IS NULL`).
			WithArgs(notificationID).
			WillReturnError(errors.New("database error"))

		result, err := repo.Get(ctx, workspaceID, notificationID)
		require.Error(t, err)
		require.Nil(t, result)
		require.Contains(t, err.Error(), "failed to get transactional notification")
	})
}

func TestTransactionalNotificationRepository_List(t *testing.T) {
	mockWorkspaceRepo, repo, mock, db, cleanup := setupTransactionalTest(t)
	defer cleanup()

	ctx := context.Background()
	workspaceID := "workspace-123"
	notification := createSampleTransactionalNotification()
	limit := 10
	offset := 0

	t.Run("successful retrieval without filters", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		// Set up count query
		countRows := sqlmock.NewRows([]string{"count"}).AddRow(1)
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM transactional_notifications WHERE deleted_at IS NULL`).
			WillReturnRows(countRows)

		// Create JSON representation of complex types for row response
		channelsJSON, err := json.Marshal(notification.Channels)
		require.NoError(t, err)
		trackingSettingsJSON, err := json.Marshal(notification.TrackingSettings)
		require.NoError(t, err)
		metadataJSON, err := json.Marshal(notification.Metadata)
		require.NoError(t, err)

		// Set up data query
		dataRows := sqlmock.NewRows([]string{
			"id", "name", "description", "channels", "tracking_settings",
			"metadata", "integration_id", "created_at", "updated_at", "deleted_at",
		}).AddRow(
			notification.ID,
			notification.Name,
			notification.Description,
			channelsJSON,
			trackingSettingsJSON,
			metadataJSON,
			notification.IntegrationID,
			notification.CreatedAt,
			notification.UpdatedAt,
			nil, // deleted_at is nil
		)

		mock.ExpectQuery(`SELECT .* FROM transactional_notifications WHERE deleted_at IS NULL ORDER BY created_at DESC LIMIT \d+ OFFSET \d+`).
			WillReturnRows(dataRows)

		results, count, err := repo.List(ctx, workspaceID, map[string]interface{}{}, limit, offset)
		require.NoError(t, err)
		require.NotNil(t, results)
		require.Equal(t, 1, count)
		require.Len(t, results, 1)
		assert.Equal(t, notification.ID, results[0].ID)
		assert.Equal(t, notification.Name, results[0].Name)
	})

	t.Run("successful retrieval with search filter", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		filter := map[string]interface{}{
			"search": "welcome",
		}

		// Set up count query with search param
		countRows := sqlmock.NewRows([]string{"count"}).AddRow(1)
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM transactional_notifications WHERE deleted_at IS NULL AND \(name ILIKE \$1 OR id ILIKE \$1\)`).
			WithArgs("%welcome%").
			WillReturnRows(countRows)

		// Create JSON representation of complex types for row response
		channelsJSON, err := json.Marshal(notification.Channels)
		require.NoError(t, err)
		trackingSettingsJSON, err := json.Marshal(notification.TrackingSettings)
		require.NoError(t, err)
		metadataJSON, err := json.Marshal(notification.Metadata)
		require.NoError(t, err)

		// Set up data query with search param
		dataRows := sqlmock.NewRows([]string{
			"id", "name", "description", "channels", "tracking_settings",
			"metadata", "integration_id", "created_at", "updated_at", "deleted_at",
		}).AddRow(
			notification.ID,
			notification.Name,
			notification.Description,
			channelsJSON,
			trackingSettingsJSON,
			metadataJSON,
			notification.IntegrationID,
			notification.CreatedAt,
			notification.UpdatedAt,
			nil, // deleted_at is nil
		)

		mock.ExpectQuery(`SELECT .* FROM transactional_notifications WHERE deleted_at IS NULL AND \(name ILIKE \$1 OR id ILIKE \$1\) ORDER BY created_at DESC LIMIT \d+ OFFSET \d+`).
			WithArgs("%welcome%").
			WillReturnRows(dataRows)

		results, count, err := repo.List(ctx, workspaceID, filter, limit, offset)
		require.NoError(t, err)
		require.NotNil(t, results)
		require.Equal(t, 1, count)
		require.Len(t, results, 1)
	})

	t.Run("workspace connection error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(nil, errors.New("connection error"))

		results, count, err := repo.List(ctx, workspaceID, map[string]interface{}{}, limit, offset)
		require.Error(t, err)
		require.Nil(t, results)
		require.Zero(t, count)
		require.Contains(t, err.Error(), "failed to get workspace db")
	})

	t.Run("count query error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM transactional_notifications WHERE deleted_at IS NULL`).
			WillReturnError(errors.New("count error"))

		results, count, err := repo.List(ctx, workspaceID, map[string]interface{}{}, limit, offset)
		require.Error(t, err)
		require.Nil(t, results)
		require.Zero(t, count)
		require.Contains(t, err.Error(), "failed to count transactional notifications")
	})

	t.Run("data query error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		// Set up count query
		countRows := sqlmock.NewRows([]string{"count"}).AddRow(1)
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM transactional_notifications WHERE deleted_at IS NULL`).
			WillReturnRows(countRows)

		// But data query fails
		mock.ExpectQuery(`SELECT .* FROM transactional_notifications WHERE deleted_at IS NULL ORDER BY created_at DESC LIMIT \d+ OFFSET \d+`).
			WillReturnError(errors.New("query error"))

		results, count, err := repo.List(ctx, workspaceID, map[string]interface{}{}, limit, offset)
		require.Error(t, err)
		require.Nil(t, results)
		require.Zero(t, count)
		require.Contains(t, err.Error(), "failed to list transactional notifications")
	})

	t.Run("scan error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		// Set up count query
		countRows := sqlmock.NewRows([]string{"count"}).AddRow(1)
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM transactional_notifications WHERE deleted_at IS NULL`).
			WillReturnRows(countRows)

		// Return incomplete row to cause scan error
		dataRows := sqlmock.NewRows([]string{"id", "name"}).
			AddRow("trans-123", "Welcome Email")

		mock.ExpectQuery(`SELECT .* FROM transactional_notifications WHERE deleted_at IS NULL ORDER BY created_at DESC LIMIT \d+ OFFSET \d+`).
			WillReturnRows(dataRows)

		results, count, err := repo.List(ctx, workspaceID, map[string]interface{}{}, limit, offset)
		require.Error(t, err)
		require.Nil(t, results)
		require.Zero(t, count)
		require.Contains(t, err.Error(), "failed to scan transactional notification")
	})

	t.Run("rows iteration error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		// Set up count query
		countRows := sqlmock.NewRows([]string{"count"}).AddRow(1)
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM transactional_notifications WHERE deleted_at IS NULL`).
			WillReturnRows(countRows)

		// Create JSON representation of complex types for row response
		channelsJSON, err := json.Marshal(notification.Channels)
		require.NoError(t, err)
		trackingSettingsJSON, err := json.Marshal(notification.TrackingSettings)
		require.NoError(t, err)
		metadataJSON, err := json.Marshal(notification.Metadata)
		require.NoError(t, err)

		// Create rows with an error during iteration
		dataRows := sqlmock.NewRows([]string{
			"id", "name", "description", "channels", "tracking_settings",
			"metadata", "integration_id", "created_at", "updated_at", "deleted_at",
		}).AddRow(
			notification.ID,
			notification.Name,
			notification.Description,
			channelsJSON,
			trackingSettingsJSON,
			metadataJSON,
			notification.IntegrationID,
			notification.CreatedAt,
			notification.UpdatedAt,
			nil, // deleted_at is nil
		).RowError(0, errors.New("row iteration error"))

		mock.ExpectQuery(`SELECT .* FROM transactional_notifications WHERE deleted_at IS NULL ORDER BY created_at DESC LIMIT \d+ OFFSET \d+`).
			WillReturnRows(dataRows)

		results, count, err := repo.List(ctx, workspaceID, map[string]interface{}{}, limit, offset)
		require.Error(t, err)
		require.Nil(t, results)
		require.Zero(t, count)
		require.Contains(t, err.Error(), "error iterating transactional notification rows")
	})
}

func TestTransactionalNotificationRepository_Delete(t *testing.T) {
	mockWorkspaceRepo, repo, mock, db, cleanup := setupTransactionalTest(t)
	defer cleanup()

	ctx := context.Background()
	workspaceID := "workspace-123"
	notificationID := "trans-123"

	t.Run("successful deletion", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		mock.ExpectExec(`UPDATE transactional_notifications SET deleted_at = \$1, updated_at = \$1 WHERE id = \$2 AND deleted_at IS NULL`).
			WithArgs(sqlmock.AnyArg(), notificationID).
			WillReturnResult(sqlmock.NewResult(1, 1))

		err := repo.Delete(ctx, workspaceID, notificationID)
		require.NoError(t, err)
	})

	t.Run("workspace connection error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(nil, errors.New("connection error"))

		err := repo.Delete(ctx, workspaceID, notificationID)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to get workspace db")
	})

	t.Run("execution error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		mock.ExpectExec(`UPDATE transactional_notifications SET deleted_at = \$1, updated_at = \$1 WHERE id = \$2 AND deleted_at IS NULL`).
			WithArgs(sqlmock.AnyArg(), notificationID).
			WillReturnError(errors.New("execution error"))

		err := repo.Delete(ctx, workspaceID, notificationID)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to delete transactional notification")
	})

	t.Run("notification not found", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		mock.ExpectExec(`UPDATE transactional_notifications SET deleted_at = \$1, updated_at = \$1 WHERE id = \$2 AND deleted_at IS NULL`).
			WithArgs(sqlmock.AnyArg(), notificationID).
			WillReturnResult(sqlmock.NewResult(0, 0))

		err := repo.Delete(ctx, workspaceID, notificationID)
		require.Error(t, err)
		require.Contains(t, err.Error(), "transactional notification not found")
	})

	t.Run("rows affected error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		// Create a custom result that returns an error for RowsAffected
		result := sqlmock.NewErrorResult(errors.New("rows affected error"))

		mock.ExpectExec(`UPDATE transactional_notifications SET deleted_at = \$1, updated_at = \$1 WHERE id = \$2 AND deleted_at IS NULL`).
			WithArgs(sqlmock.AnyArg(), notificationID).
			WillReturnResult(result)

		err := repo.Delete(ctx, workspaceID, notificationID)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to get rows affected")
	})
}
