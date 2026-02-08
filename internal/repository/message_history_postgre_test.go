package repository

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/golang/mock/gomock"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testSecretKey = "test-secret-key-for-encryption-tests"

// StringArrayConverter is a custom ValueConverter for pq.StringArray to handle it in sqlmock.
// This is needed because sqlmock doesn't natively support PostgreSQL array types.
//
// Usage in tests:
//   - Use pq.Array(value) when expecting array arguments in WithArgs()
//   - Use "{}" string format for empty arrays in mock return rows (AddRow)
type StringArrayConverter struct{}

// ConvertValue converts pq.StringArray to a driver.Value for sqlmock compatibility.
func (s StringArrayConverter) ConvertValue(v interface{}) (driver.Value, error) {
	switch x := v.(type) {
	case pq.StringArray:
		return x, nil
	default:
		return driver.DefaultParameterConverter.ConvertValue(v)
	}
}

func setupMessageHistoryTest(t *testing.T) (*mocks.MockWorkspaceRepository, domain.MessageHistoryRepository, sqlmock.Sqlmock, *sql.DB, func()) {
	ctrl := gomock.NewController(t)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)

	// Create a real DB connection with sqlmock.
	// The custom ValueConverter helps sqlmock handle pq.StringArray types.
	db, mock, err := sqlmock.New(sqlmock.ValueConverterOption(StringArrayConverter{}))
	require.NoError(t, err)

	repo := NewMessageHistoryRepository(mockWorkspaceRepo)

	// Set up cleanup function
	cleanup := func() {
		_ = db.Close()
		ctrl.Finish()
	}

	return mockWorkspaceRepo, repo, mock, db, cleanup
}

func createSampleMessageHistory() *domain.MessageHistory {
	// Use a fixed timestamp to avoid timing issues in CI
	now := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)
	externalID := "ext-123"
	broadcastID := "broadcast-123"
	messageData := domain.MessageData{
		Data: map[string]interface{}{
			"subject": "Test Subject",
			"body":    "Test Body",
		},
	}

	return &domain.MessageHistory{
		ID:              "msg-123",
		ExternalID:      &externalID,
		ContactEmail:    "test@example.com",
		BroadcastID:     &broadcastID,
		ListID:          nil, // No list
		TemplateID:      "template-123",
		TemplateVersion: 1,
		Channel:         "email",
		StatusInfo:      nil,
		MessageData:     messageData,
		SentAt:          now,
		DeliveredAt:     nil,
		FailedAt:        nil,
		OpenedAt:        nil,
		ClickedAt:       nil,
		BouncedAt:       nil,
		ComplainedAt:    nil,
		UnsubscribedAt:  nil,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
}

func TestMessageHistoryRepository_Create(t *testing.T) {
	mockWorkspaceRepo, repo, mock, db, cleanup := setupMessageHistoryTest(t)
	defer cleanup()

	ctx := context.Background()
	workspaceID := "workspace-123"
	message := createSampleMessageHistory()

	t.Run("successful creation", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		mock.ExpectExec(`INSERT INTO message_history`).
			WithArgs(
				message.ID,
				message.ExternalID,
				message.ContactEmail,
				message.BroadcastID,
				message.AutomationID,
				message.ListID,
				message.TemplateID,
				message.TemplateVersion,
				message.Channel,
				message.StatusInfo,
				sqlmock.AnyArg(), // message_data
				sqlmock.AnyArg(), // channel_options
				sqlmock.AnyArg(), // attachments
				message.SentAt,
				message.DeliveredAt,
				message.FailedAt,
				message.OpenedAt,
				message.ClickedAt,
				message.BouncedAt,
				message.ComplainedAt,
				message.UnsubscribedAt,
				message.CreatedAt,
				message.UpdatedAt,
			).
			WillReturnResult(sqlmock.NewResult(1, 1))

		err := repo.Create(ctx, workspaceID, testSecretKey, message)
		require.NoError(t, err)
	})

	t.Run("workspace connection error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(nil, errors.New("connection error"))

		err := repo.Create(ctx, workspaceID, testSecretKey, message)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to get workspace connection")
	})

	t.Run("execution error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		mock.ExpectExec(`INSERT INTO message_history`).
			WithArgs(
				message.ID,
				message.ExternalID,
				message.ContactEmail,
				message.BroadcastID,
				message.AutomationID,
				message.ListID,
				message.TemplateID,
				message.TemplateVersion,
				message.Channel,
				message.StatusInfo,
				sqlmock.AnyArg(), // message_data
				sqlmock.AnyArg(), // channel_options
				sqlmock.AnyArg(), // attachments
				message.SentAt,
				message.DeliveredAt,
				message.FailedAt,
				message.OpenedAt,
				message.ClickedAt,
				message.BouncedAt,
				message.ComplainedAt,
				message.UnsubscribedAt,
				message.CreatedAt,
				message.UpdatedAt,
			).
			WillReturnError(errors.New("execution error"))

		err := repo.Create(ctx, workspaceID, testSecretKey, message)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to create message history")
	})
}

func TestMessageHistoryRepository_Update(t *testing.T) {
	mockWorkspaceRepo, repo, mock, db, cleanup := setupMessageHistoryTest(t)
	defer cleanup()

	ctx := context.Background()
	workspaceID := "workspace-123"
	message := createSampleMessageHistory()

	t.Run("successful update", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		mock.ExpectExec(`UPDATE message_history SET`).
			WithArgs(
				message.ID,
				message.ExternalID,
				message.ContactEmail,
				message.BroadcastID,
				message.AutomationID,
				message.ListID,
				message.TemplateID,
				message.TemplateVersion,
				message.Channel,
				message.StatusInfo,
				sqlmock.AnyArg(), // message_data
				sqlmock.AnyArg(), // channel_options
				sqlmock.AnyArg(), // attachments
				message.SentAt,
				message.DeliveredAt,
				message.FailedAt,
				message.OpenedAt,
				message.ClickedAt,
				message.BouncedAt,
				message.ComplainedAt,
				message.UnsubscribedAt,
				sqlmock.AnyArg(), // updated_at
			).
			WillReturnResult(sqlmock.NewResult(1, 1))

		err := repo.Update(ctx, workspaceID, message)
		require.NoError(t, err)
	})

	t.Run("workspace connection error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(nil, errors.New("connection error"))

		err := repo.Update(ctx, workspaceID, message)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to get workspace connection")
	})

	t.Run("execution error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		mock.ExpectExec(`UPDATE message_history SET`).
			WithArgs(
				message.ID,
				message.ExternalID,
				message.ContactEmail,
				message.BroadcastID,
				message.AutomationID,
				message.ListID,
				message.TemplateID,
				message.TemplateVersion,
				message.Channel,
				message.StatusInfo,
				sqlmock.AnyArg(), // message_data
				sqlmock.AnyArg(), // channel_options
				sqlmock.AnyArg(), // attachments
				message.SentAt,
				message.DeliveredAt,
				message.FailedAt,
				message.OpenedAt,
				message.ClickedAt,
				message.BouncedAt,
				message.ComplainedAt,
				message.UnsubscribedAt,
				sqlmock.AnyArg(), // updated_at
			).
			WillReturnError(errors.New("execution error"))

		err := repo.Update(ctx, workspaceID, message)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to update message history")
	})
}

func TestMessageHistoryRepository_Get(t *testing.T) {
	mockWorkspaceRepo, repo, mock, db, cleanup := setupMessageHistoryTest(t)
	defer cleanup()

	ctx := context.Background()
	workspaceID := "workspace-123"
	messageID := "msg-123"
	message := createSampleMessageHistory()

	// Convert the MessageData to JSON for proper DB response mocking
	messageDataJSON, _ := json.Marshal(message.MessageData)

	t.Run("successful retrieval", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		rows := sqlmock.NewRows([]string{
			"id", "external_id", "contact_email", "broadcast_id", "automation_id", "list_id", "template_id", "template_version",
			"channel", "status_info", "message_data", "channel_options", "attachments", "sent_at", "delivered_at",
			"failed_at", "opened_at", "clicked_at", "bounced_at", "complained_at",
			"unsubscribed_at", "created_at", "updated_at",
		}).AddRow(
			message.ID,
			message.ExternalID,
			message.ContactEmail,
			message.BroadcastID,
			message.AutomationID,
			nil, // list_id (empty array)
			message.TemplateID,
			message.TemplateVersion,
			message.Channel,
			message.StatusInfo,
			messageDataJSON, // Use the actual JSON bytes
			nil,             // channel_options (null)
			[]byte("[]"),    // attachments (empty array)
			message.SentAt,
			message.DeliveredAt,
			message.FailedAt,
			message.OpenedAt,
			message.ClickedAt,
			message.BouncedAt,
			message.ComplainedAt,
			message.UnsubscribedAt,
			message.CreatedAt,
			message.UpdatedAt,
		)

		mock.ExpectQuery(`SELECT .* FROM message_history WHERE id = \$1`).
			WithArgs(messageID).
			WillReturnRows(rows)

		result, err := repo.Get(ctx, workspaceID, testSecretKey, messageID)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, message.ID, result.ID)
		assert.Equal(t, message.ContactEmail, result.ContactEmail)
		assert.Equal(t, *message.BroadcastID, *result.BroadcastID)
	})

	t.Run("not found", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		mock.ExpectQuery(`SELECT .* FROM message_history WHERE id = \$1`).
			WithArgs(messageID).
			WillReturnError(sql.ErrNoRows)

		result, err := repo.Get(ctx, workspaceID, testSecretKey, messageID)
		require.Error(t, err)
		require.Nil(t, result)
		require.Contains(t, err.Error(), "message history with id msg-123 not found")
	})

	t.Run("workspace connection error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(nil, errors.New("connection error"))

		result, err := repo.Get(ctx, workspaceID, testSecretKey, messageID)
		require.Error(t, err)
		require.Nil(t, result)
		require.Contains(t, err.Error(), "failed to get workspace connection")
	})

	t.Run("scan error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		rows := sqlmock.NewRows([]string{
			"id", "external_id", "contact_email", "broadcast_id", "automation_id", "list_id", "template_id", "template_version",
		}).AddRow(
			message.ID,
			message.ExternalID,
			message.ContactEmail,
			message.BroadcastID,
			message.AutomationID,
			nil, // list_id (empty array)
			message.TemplateID,
			message.TemplateVersion,
		) // Incomplete row to cause scan error

		mock.ExpectQuery(`SELECT .* FROM message_history WHERE id = \$1`).
			WithArgs(messageID).
			WillReturnRows(rows)

		result, err := repo.Get(ctx, workspaceID, testSecretKey, messageID)
		require.Error(t, err)
		require.Nil(t, result)
		require.Contains(t, err.Error(), "failed to get message history")
	})
}

func TestMessageHistoryRepository_GetByExternalID(t *testing.T) {
	mockWorkspaceRepo, repo, mock, db, cleanup := setupMessageHistoryTest(t)
	defer cleanup()

	ctx := context.Background()
	workspaceID := "workspace-123"
	externalID := "ext-123"
	message := createSampleMessageHistory()

	// Convert the MessageData to JSON for proper DB response mocking
	messageDataJSON, _ := json.Marshal(message.MessageData)

	t.Run("successful retrieval", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		rows := sqlmock.NewRows([]string{
			"id", "external_id", "contact_email", "broadcast_id", "automation_id", "list_id", "template_id", "template_version",
			"channel", "status_info", "message_data", "channel_options", "attachments", "sent_at", "delivered_at",
			"failed_at", "opened_at", "clicked_at", "bounced_at", "complained_at",
			"unsubscribed_at", "created_at", "updated_at",
		}).AddRow(
			message.ID,
			message.ExternalID,
			message.ContactEmail,
			message.BroadcastID,
			message.AutomationID,
			nil, // list_id (empty array)
			message.TemplateID,
			message.TemplateVersion,
			message.Channel,
			message.StatusInfo,
			messageDataJSON, // Use the actual JSON bytes
			nil,             // channel_options (null)
			[]byte("[]"),    // attachments (empty array)
			message.SentAt,
			message.DeliveredAt,
			message.FailedAt,
			message.OpenedAt,
			message.ClickedAt,
			message.BouncedAt,
			message.ComplainedAt,
			message.UnsubscribedAt,
			message.CreatedAt,
			message.UpdatedAt,
		)

		mock.ExpectQuery(`SELECT .* FROM message_history WHERE external_id = \$1`).
			WithArgs(externalID).
			WillReturnRows(rows)

		result, err := repo.GetByExternalID(ctx, workspaceID, testSecretKey, externalID)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, message.ID, result.ID)
		assert.Equal(t, message.ContactEmail, result.ContactEmail)
		assert.Equal(t, *message.ExternalID, *result.ExternalID)
	})

	t.Run("not found", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		mock.ExpectQuery(`SELECT .* FROM message_history WHERE external_id = \$1`).
			WithArgs(externalID).
			WillReturnError(sql.ErrNoRows)

		result, err := repo.GetByExternalID(ctx, workspaceID, testSecretKey, externalID)
		require.Error(t, err)
		require.Nil(t, result)
		require.Contains(t, err.Error(), "message history with external_id ext-123 not found")
	})

	t.Run("workspace connection error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(nil, errors.New("connection error"))

		result, err := repo.GetByExternalID(ctx, workspaceID, testSecretKey, externalID)
		require.Error(t, err)
		require.Nil(t, result)
		require.Contains(t, err.Error(), "failed to get workspace connection")
	})

	t.Run("scan error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		rows := sqlmock.NewRows([]string{
			"id", "external_id", "contact_email", "broadcast_id", "automation_id", "list_id", "template_id", "template_version",
		}).AddRow(
			message.ID,
			message.ExternalID,
			message.ContactEmail,
			message.BroadcastID,
			message.AutomationID,
			nil, // list_id (empty array)
			message.TemplateID,
			message.TemplateVersion,
		) // Incomplete row to cause scan error

		mock.ExpectQuery(`SELECT .* FROM message_history WHERE external_id = \$1`).
			WithArgs(externalID).
			WillReturnRows(rows)

		result, err := repo.GetByExternalID(ctx, workspaceID, testSecretKey, externalID)
		require.Error(t, err)
		require.Nil(t, result)
		require.Contains(t, err.Error(), "failed to get message history by external_id")
	})
}

func TestMessageHistoryRepository_GetByContact(t *testing.T) {
	mockWorkspaceRepo, repo, mock, db, cleanup := setupMessageHistoryTest(t)
	defer cleanup()

	ctx := context.Background()
	workspaceID := "workspace-123"
	contactEmail := "contact@example.com"
	message := createSampleMessageHistory()
	limit := 10
	offset := 0

	// Convert the MessageData to JSON for proper DB response mocking
	messageDataJSON, _ := json.Marshal(message.MessageData)

	t.Run("successful retrieval", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		// Set up count query
		countRows := sqlmock.NewRows([]string{"count"}).AddRow(1)
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM message_history WHERE contact_email = \$1`).
			WithArgs(contactEmail).
			WillReturnRows(countRows)

		// Set up data query
		dataRows := sqlmock.NewRows([]string{
			"id", "external_id", "contact_email", "broadcast_id", "automation_id", "list_id", "template_id", "template_version",
			"channel", "status_info", "message_data", "channel_options", "attachments", "sent_at", "delivered_at",
			"failed_at", "opened_at", "clicked_at", "bounced_at", "complained_at",
			"unsubscribed_at", "created_at", "updated_at",
		}).AddRow(
			message.ID,
			message.ExternalID,
			message.ContactEmail,
			message.BroadcastID,
			message.AutomationID,
			nil, // list_id (empty array)
			message.TemplateID,
			message.TemplateVersion,
			message.Channel,
			message.StatusInfo,
			messageDataJSON, // Use the actual JSON bytes
			nil,             // channel_options (null)
			[]byte("[]"),    // attachments (empty array)
			message.SentAt,
			message.DeliveredAt,
			message.FailedAt,
			message.OpenedAt,
			message.ClickedAt,
			message.BouncedAt,
			message.ComplainedAt,
			message.UnsubscribedAt,
			message.CreatedAt,
			message.UpdatedAt,
		)

		mock.ExpectQuery(`SELECT .* FROM message_history WHERE contact_email = \$1 ORDER BY sent_at DESC LIMIT \$2 OFFSET \$3`).
			WithArgs(contactEmail, limit, offset).
			WillReturnRows(dataRows)

		results, count, err := repo.GetByContact(ctx, workspaceID, testSecretKey, contactEmail, limit, offset)
		require.NoError(t, err)
		require.NotNil(t, results)
		require.Equal(t, 1, count)
		require.Len(t, results, 1)
		assert.Equal(t, message.ID, results[0].ID)
		assert.Equal(t, message.ContactEmail, results[0].ContactEmail)
	})

	t.Run("workspace connection error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(nil, errors.New("connection error"))

		results, count, err := repo.GetByContact(ctx, workspaceID, testSecretKey, contactEmail, limit, offset)
		require.Error(t, err)
		require.Nil(t, results)
		require.Zero(t, count)
		require.Contains(t, err.Error(), "failed to get workspace connection")
	})

	t.Run("count query error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM message_history WHERE contact_email = \$1`).
			WithArgs(contactEmail).
			WillReturnError(errors.New("count error"))

		results, count, err := repo.GetByContact(ctx, workspaceID, testSecretKey, contactEmail, limit, offset)
		require.Error(t, err)
		require.Nil(t, results)
		require.Zero(t, count)
		require.Contains(t, err.Error(), "failed to count message history")
	})

	t.Run("data query error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		// Set up count query
		countRows := sqlmock.NewRows([]string{"count"}).AddRow(1)
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM message_history WHERE contact_email = \$1`).
			WithArgs(contactEmail).
			WillReturnRows(countRows)

		// But data query fails
		mock.ExpectQuery(`SELECT .* FROM message_history WHERE contact_email = \$1 ORDER BY sent_at DESC LIMIT \$2 OFFSET \$3`).
			WithArgs(contactEmail, limit, offset).
			WillReturnError(errors.New("query error"))

		results, count, err := repo.GetByContact(ctx, workspaceID, testSecretKey, contactEmail, limit, offset)
		require.Error(t, err)
		require.Nil(t, results)
		require.Zero(t, count)
		require.Contains(t, err.Error(), "failed to query message history")
	})

	t.Run("scan error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		// Set up count query
		countRows := sqlmock.NewRows([]string{"count"}).AddRow(1)
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM message_history WHERE contact_email = \$1`).
			WithArgs(contactEmail).
			WillReturnRows(countRows)

		// Return incomplete row to cause scan error
		dataRows := sqlmock.NewRows([]string{"id", "contact_email"}).
			AddRow("msg-123", "contact-123")

		mock.ExpectQuery(`SELECT .* FROM message_history WHERE contact_email = \$1 ORDER BY sent_at DESC LIMIT \$2 OFFSET \$3`).
			WithArgs(contactEmail, limit, offset).
			WillReturnRows(dataRows)

		results, count, err := repo.GetByContact(ctx, workspaceID, testSecretKey, contactEmail, limit, offset)
		require.Error(t, err)
		require.Nil(t, results)
		require.Zero(t, count)
		require.Contains(t, err.Error(), "failed to scan message history")
	})

	t.Run("default limit and offset", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		// Set up count query
		countRows := sqlmock.NewRows([]string{"count"}).AddRow(1)
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM message_history WHERE contact_email = \$1`).
			WithArgs(contactEmail).
			WillReturnRows(countRows)

		// Should use default limit of 50 and offset of 0
		mock.ExpectQuery(`SELECT .* FROM message_history WHERE contact_email = \$1 ORDER BY sent_at DESC LIMIT \$2 OFFSET \$3`).
			WithArgs(contactEmail, 50, 0).
			WillReturnRows(sqlmock.NewRows([]string{
				"id", "external_id", "contact_email", "broadcast_id", "automation_id", "list_id", "template_id", "template_version",
				"channel", "status_info", "message_data", "channel_options", "attachments", "sent_at", "delivered_at",
				"failed_at", "opened_at", "clicked_at", "bounced_at", "complained_at",
				"unsubscribed_at", "created_at", "updated_at",
			}).AddRow(
				message.ID,
				message.ExternalID,
				message.ContactEmail,
				message.BroadcastID,
				message.AutomationID,
				nil, // list_id (empty array)
				message.TemplateID,
				message.TemplateVersion,
				message.Channel,
				message.StatusInfo,
				messageDataJSON, // Use the actual JSON bytes
				nil,             // channel_options (null)
				[]byte("[]"),    // attachments (empty array)
				message.SentAt,
				message.DeliveredAt,
				message.FailedAt,
				message.OpenedAt,
				message.ClickedAt,
				message.BouncedAt,
				message.ComplainedAt,
				message.UnsubscribedAt,
				message.CreatedAt,
				message.UpdatedAt,
			))

		// Call with negative limit and offset
		results, count, err := repo.GetByContact(ctx, workspaceID, testSecretKey, contactEmail, -5, -10)
		require.NoError(t, err)
		require.NotNil(t, results)
		require.Equal(t, 1, count)
		require.Len(t, results, 1)
	})
}

func TestMessageHistoryRepository_GetByBroadcast(t *testing.T) {
	mockWorkspaceRepo, repo, mock, db, cleanup := setupMessageHistoryTest(t)
	defer cleanup()

	ctx := context.Background()
	workspaceID := "workspace-123"
	broadcastID := "broadcast-123"
	message := createSampleMessageHistory()
	limit := 10
	offset := 0

	// Convert the MessageData to JSON for proper DB response mocking
	messageDataJSON, _ := json.Marshal(message.MessageData)

	t.Run("successful retrieval", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		// Set up count query
		countRows := sqlmock.NewRows([]string{"count"}).AddRow(1)
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM message_history WHERE broadcast_id = \$1`).
			WithArgs(broadcastID).
			WillReturnRows(countRows)

		// Set up data query
		dataRows := sqlmock.NewRows([]string{
			"id", "external_id", "contact_email", "broadcast_id", "automation_id", "list_id", "template_id", "template_version",
			"channel", "status_info", "message_data", "channel_options", "attachments", "sent_at", "delivered_at",
			"failed_at", "opened_at", "clicked_at", "bounced_at", "complained_at",
			"unsubscribed_at", "created_at", "updated_at",
		}).AddRow(
			message.ID,
			message.ExternalID,
			message.ContactEmail,
			message.BroadcastID,
			message.AutomationID,
			nil, // list_id (empty array)
			message.TemplateID,
			message.TemplateVersion,
			message.Channel,
			message.StatusInfo,
			messageDataJSON, // Use the actual JSON bytes
			nil,             // channel_options (null)
			[]byte("[]"),    // attachments (empty array)
			message.SentAt,
			message.DeliveredAt,
			message.FailedAt,
			message.OpenedAt,
			message.ClickedAt,
			message.BouncedAt,
			message.ComplainedAt,
			message.UnsubscribedAt,
			message.CreatedAt,
			message.UpdatedAt,
		)

		mock.ExpectQuery(`SELECT .* FROM message_history WHERE broadcast_id = \$1 ORDER BY sent_at DESC LIMIT \$2 OFFSET \$3`).
			WithArgs(broadcastID, limit, offset).
			WillReturnRows(dataRows)

		results, count, err := repo.GetByBroadcast(ctx, workspaceID, testSecretKey, broadcastID, limit, offset)
		require.NoError(t, err)
		require.NotNil(t, results)
		require.Equal(t, 1, count)
		require.Len(t, results, 1)
		assert.Equal(t, message.ID, results[0].ID)
		assert.Equal(t, *message.BroadcastID, *results[0].BroadcastID)
	})

	t.Run("workspace connection error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(nil, errors.New("connection error"))

		results, count, err := repo.GetByBroadcast(ctx, workspaceID, testSecretKey, broadcastID, limit, offset)
		require.Error(t, err)
		require.Nil(t, results)
		require.Zero(t, count)
		require.Contains(t, err.Error(), "failed to get workspace connection")
	})

	t.Run("count query error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM message_history WHERE broadcast_id = \$1`).
			WithArgs(broadcastID).
			WillReturnError(errors.New("count error"))

		results, count, err := repo.GetByBroadcast(ctx, workspaceID, testSecretKey, broadcastID, limit, offset)
		require.Error(t, err)
		require.Nil(t, results)
		require.Zero(t, count)
		require.Contains(t, err.Error(), "failed to count message history")
	})

	t.Run("data query error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		// Set up count query
		countRows := sqlmock.NewRows([]string{"count"}).AddRow(1)
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM message_history WHERE broadcast_id = \$1`).
			WithArgs(broadcastID).
			WillReturnRows(countRows)

		// But data query fails
		mock.ExpectQuery(`SELECT .* FROM message_history WHERE broadcast_id = \$1 ORDER BY sent_at DESC LIMIT \$2 OFFSET \$3`).
			WithArgs(broadcastID, limit, offset).
			WillReturnError(errors.New("query error"))

		results, count, err := repo.GetByBroadcast(ctx, workspaceID, testSecretKey, broadcastID, limit, offset)
		require.Error(t, err)
		require.Nil(t, results)
		require.Zero(t, count)
		require.Contains(t, err.Error(), "failed to query message history")
	})

	t.Run("scan error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		// Set up count query
		countRows := sqlmock.NewRows([]string{"count"}).AddRow(1)
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM message_history WHERE broadcast_id = \$1`).
			WithArgs(broadcastID).
			WillReturnRows(countRows)

		// Return incomplete row to cause scan error
		dataRows := sqlmock.NewRows([]string{"id", "contact_email"}).
			AddRow("msg-123", "contact-123")

		mock.ExpectQuery(`SELECT .* FROM message_history WHERE broadcast_id = \$1 ORDER BY sent_at DESC LIMIT \$2 OFFSET \$3`).
			WithArgs(broadcastID, limit, offset).
			WillReturnRows(dataRows)

		results, count, err := repo.GetByBroadcast(ctx, workspaceID, testSecretKey, broadcastID, limit, offset)
		require.Error(t, err)
		require.Nil(t, results)
		require.Zero(t, count)
		require.Contains(t, err.Error(), "failed to scan message history")
	})

	t.Run("default limit and offset", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		// Set up count query
		countRows := sqlmock.NewRows([]string{"count"}).AddRow(1)
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM message_history WHERE broadcast_id = \$1`).
			WithArgs(broadcastID).
			WillReturnRows(countRows)

		// Should use default limit of 50 and offset of 0
		mock.ExpectQuery(`SELECT .* FROM message_history WHERE broadcast_id = \$1 ORDER BY sent_at DESC LIMIT \$2 OFFSET \$3`).
			WithArgs(broadcastID, 50, 0).
			WillReturnRows(sqlmock.NewRows([]string{
				"id", "external_id", "contact_email", "broadcast_id", "automation_id", "list_id", "template_id", "template_version",
				"channel", "status_info", "message_data", "channel_options", "attachments", "sent_at", "delivered_at",
				"failed_at", "opened_at", "clicked_at", "bounced_at", "complained_at",
				"unsubscribed_at", "created_at", "updated_at",
			}).AddRow(
				message.ID,
				message.ExternalID,
				message.ContactEmail,
				message.BroadcastID,
				message.AutomationID,
				nil, // list_id (empty array)
				message.TemplateID,
				message.TemplateVersion,
				message.Channel,
				message.StatusInfo,
				messageDataJSON, // Use the actual JSON bytes
				nil,             // channel_options (null)
				[]byte("[]"),    // attachments (empty array)
				message.SentAt,
				message.DeliveredAt,
				message.FailedAt,
				message.OpenedAt,
				message.ClickedAt,
				message.BouncedAt,
				message.ComplainedAt,
				message.UnsubscribedAt,
				message.CreatedAt,
				message.UpdatedAt,
			))

		// Call with negative limit and offset
		results, count, err := repo.GetByBroadcast(ctx, workspaceID, testSecretKey, broadcastID, -5, -10)
		require.NoError(t, err)
		require.NotNil(t, results)
		require.Equal(t, 1, count)
		require.Len(t, results, 1)
	})
}

func TestMessageHistoryRepository_SetClicked(t *testing.T) {
	mockWorkspaceRepo, repo, mock, db, cleanup := setupMessageHistoryTest(t)
	defer cleanup()

	ctx := context.Background()
	workspaceID := "workspace-123"
	messageID := "msg-123"
	// Use a fixed timestamp to avoid timing issues in CI
	timestamp := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)

	t.Run("successful click update", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		// Expect the clicked_at update query
		mock.ExpectExec(`UPDATE message_history SET clicked_at = \$1, updated_at = NOW\(\) WHERE id = \$2 AND clicked_at IS NULL`).
			WithArgs(timestamp, messageID).
			WillReturnResult(sqlmock.NewResult(1, 1))

		// Expect the opened_at update query
		mock.ExpectExec(`UPDATE message_history SET opened_at = \$1, updated_at = NOW\(\) WHERE id = \$2 AND opened_at IS NULL`).
			WithArgs(timestamp, messageID).
			WillReturnResult(sqlmock.NewResult(1, 1))

		err := repo.SetClicked(ctx, workspaceID, messageID, timestamp)
		require.NoError(t, err)
	})

	t.Run("workspace connection error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(nil, errors.New("connection error"))

		err := repo.SetClicked(ctx, workspaceID, messageID, timestamp)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to get workspace connection")
	})

	t.Run("clicked update error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		// First query fails
		mock.ExpectExec(`UPDATE message_history SET clicked_at = \$1, updated_at = NOW\(\) WHERE id = \$2 AND clicked_at IS NULL`).
			WithArgs(timestamp, messageID).
			WillReturnError(errors.New("execution error"))

		err := repo.SetClicked(ctx, workspaceID, messageID, timestamp)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to set clicked")
	})

	t.Run("opened update error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		// First query succeeds
		mock.ExpectExec(`UPDATE message_history SET clicked_at = \$1, updated_at = NOW\(\) WHERE id = \$2 AND clicked_at IS NULL`).
			WithArgs(timestamp, messageID).
			WillReturnResult(sqlmock.NewResult(1, 1))

		// Second query fails
		mock.ExpectExec(`UPDATE message_history SET opened_at = \$1, updated_at = NOW\(\) WHERE id = \$2 AND opened_at IS NULL`).
			WithArgs(timestamp, messageID).
			WillReturnError(errors.New("execution error"))

		err := repo.SetClicked(ctx, workspaceID, messageID, timestamp)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to set opened")
	})
}

func TestMessageHistoryRepository_SetOpened(t *testing.T) {
	mockWorkspaceRepo, repo, mock, db, cleanup := setupMessageHistoryTest(t)
	defer cleanup()

	ctx := context.Background()
	workspaceID := "workspace-123"
	messageID := "msg-123"
	// Use a fixed timestamp to avoid timing issues in CI
	timestamp := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)

	t.Run("successful open update", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		// Expect the opened_at update query
		mock.ExpectExec(`UPDATE message_history SET opened_at = \$1, updated_at = NOW\(\) WHERE id = \$2 AND opened_at IS NULL`).
			WithArgs(timestamp, messageID).
			WillReturnResult(sqlmock.NewResult(1, 1))

		err := repo.SetOpened(ctx, workspaceID, messageID, timestamp)
		require.NoError(t, err)
	})

	t.Run("workspace connection error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(nil, errors.New("connection error"))

		err := repo.SetOpened(ctx, workspaceID, messageID, timestamp)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to get workspace connection")
	})

	t.Run("opened update error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		// Query fails
		mock.ExpectExec(`UPDATE message_history SET opened_at = \$1, updated_at = NOW\(\) WHERE id = \$2 AND opened_at IS NULL`).
			WithArgs(timestamp, messageID).
			WillReturnError(errors.New("execution error"))

		err := repo.SetOpened(ctx, workspaceID, messageID, timestamp)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to set opened")
	})
}

func TestMessageHistoryRepository_GetBroadcastStats(t *testing.T) {
	mockWorkspaceRepo, repo, mock, db, cleanup := setupMessageHistoryTest(t)
	defer cleanup()

	ctx := context.Background()
	workspaceID := "workspace-123"
	broadcastID := "broadcast-123"

	t.Run("successful retrieval", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		rows := sqlmock.NewRows([]string{
			"total_sent", "total_delivered", "total_failed", "total_opened",
			"total_clicked", "total_bounced", "total_complained", "total_unsubscribed",
		}).AddRow(10, 8, 2, 5, 3, 1, 0, 1)

		mock.ExpectQuery(`SELECT .* FROM message_history WHERE broadcast_id = \$1`).
			WithArgs(broadcastID).
			WillReturnRows(rows)

		stats, err := repo.GetBroadcastStats(ctx, workspaceID, broadcastID)

		require.NoError(t, err)
		require.NotNil(t, stats)
		assert.Equal(t, 10, stats.TotalSent)
		assert.Equal(t, 8, stats.TotalDelivered)
		assert.Equal(t, 2, stats.TotalFailed)
		assert.Equal(t, 5, stats.TotalOpened)
		assert.Equal(t, 3, stats.TotalClicked)
		assert.Equal(t, 1, stats.TotalBounced)
		assert.Equal(t, 0, stats.TotalComplained)
		assert.Equal(t, 1, stats.TotalUnsubscribed)
	})

	t.Run("workspace connection error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(nil, errors.New("connection error"))

		stats, err := repo.GetBroadcastStats(ctx, workspaceID, broadcastID)
		require.Error(t, err)
		require.Nil(t, stats)
		require.Contains(t, err.Error(), "failed to get workspace connection")
	})

	t.Run("sql error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		mock.ExpectQuery(`SELECT .* FROM message_history WHERE broadcast_id = \$1`).
			WithArgs(broadcastID).
			WillReturnError(errors.New("sql error"))

		stats, err := repo.GetBroadcastStats(ctx, workspaceID, broadcastID)
		require.Error(t, err)
		require.Nil(t, stats)
		require.Contains(t, err.Error(), "failed to get broadcast stats")
	})

	t.Run("no rows", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		mock.ExpectQuery(`SELECT .* FROM message_history WHERE broadcast_id = \$1`).
			WithArgs(broadcastID).
			WillReturnError(sql.ErrNoRows)

		stats, err := repo.GetBroadcastStats(ctx, workspaceID, broadcastID)
		require.NoError(t, err)
		require.NotNil(t, stats)
		assert.Equal(t, 0, stats.TotalSent)
		assert.Equal(t, 0, stats.TotalDelivered)
		assert.Equal(t, 0, stats.TotalFailed)
		assert.Equal(t, 0, stats.TotalOpened)
		assert.Equal(t, 0, stats.TotalClicked)
		assert.Equal(t, 0, stats.TotalBounced)
		assert.Equal(t, 0, stats.TotalComplained)
		assert.Equal(t, 0, stats.TotalUnsubscribed)
	})

	t.Run("null values", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		// Create mock rows with some NULL values
		rows := sqlmock.NewRows([]string{
			"total_sent", "total_delivered", "total_failed", "total_opened",
			"total_clicked", "total_bounced", "total_complained", "total_unsubscribed",
		}).AddRow(10, nil, 2, nil, 3, nil, nil, 1)

		mock.ExpectQuery(`SELECT .* FROM message_history WHERE broadcast_id = \$1`).
			WithArgs(broadcastID).
			WillReturnRows(rows)

		stats, err := repo.GetBroadcastStats(ctx, workspaceID, broadcastID)
		require.NoError(t, err)
		require.NotNil(t, stats)
		assert.Equal(t, 10, stats.TotalSent)
		assert.Equal(t, 0, stats.TotalDelivered) // Should be 0 for NULL
		assert.Equal(t, 2, stats.TotalFailed)
		assert.Equal(t, 0, stats.TotalOpened) // Should be 0 for NULL
		assert.Equal(t, 3, stats.TotalClicked)
		assert.Equal(t, 0, stats.TotalBounced)    // Should be 0 for NULL
		assert.Equal(t, 0, stats.TotalComplained) // Should be 0 for NULL
		assert.Equal(t, 1, stats.TotalUnsubscribed)
	})
}

func TestMessageHistoryRepository_GetBroadcastVariationStats(t *testing.T) {
	mockWorkspaceRepo, repo, mock, db, cleanup := setupMessageHistoryTest(t)
	defer cleanup()

	ctx := context.Background()
	workspaceID := "workspace-123"
	broadcastID := "broadcast-123"
	templateID := "template-123"

	t.Run("successful retrieval", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		rows := sqlmock.NewRows([]string{
			"total_sent", "total_delivered", "total_failed", "total_opened",
			"total_clicked", "total_bounced", "total_complained", "total_unsubscribed",
		}).AddRow(10, 8, 2, 5, 3, 1, 0, 1)

		mock.ExpectQuery(`SELECT .* FROM message_history WHERE broadcast_id = \$1 AND template_id = \$2`).
			WithArgs(broadcastID, templateID).
			WillReturnRows(rows)

		stats, err := repo.GetBroadcastVariationStats(ctx, workspaceID, broadcastID, templateID)

		require.NoError(t, err)
		require.NotNil(t, stats)
		assert.Equal(t, 10, stats.TotalSent)
		assert.Equal(t, 8, stats.TotalDelivered)
		assert.Equal(t, 2, stats.TotalFailed)
		assert.Equal(t, 5, stats.TotalOpened)
		assert.Equal(t, 3, stats.TotalClicked)
		assert.Equal(t, 1, stats.TotalBounced)
		assert.Equal(t, 0, stats.TotalComplained)
		assert.Equal(t, 1, stats.TotalUnsubscribed)
	})

	t.Run("workspace connection error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(nil, errors.New("connection error"))

		stats, err := repo.GetBroadcastVariationStats(ctx, workspaceID, broadcastID, templateID)
		require.Error(t, err)
		require.Nil(t, stats)
		require.Contains(t, err.Error(), "failed to get workspace connection")
	})

	t.Run("sql error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		mock.ExpectQuery(`SELECT .* FROM message_history WHERE broadcast_id = \$1 AND template_id = \$2`).
			WithArgs(broadcastID, templateID).
			WillReturnError(errors.New("sql error"))

		stats, err := repo.GetBroadcastVariationStats(ctx, workspaceID, broadcastID, templateID)
		require.Error(t, err)
		require.Nil(t, stats)
		require.Contains(t, err.Error(), "failed to get broadcast variation stats")
	})

	t.Run("no rows", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		mock.ExpectQuery(`SELECT .* FROM message_history WHERE broadcast_id = \$1 AND template_id = \$2`).
			WithArgs(broadcastID, templateID).
			WillReturnError(sql.ErrNoRows)

		stats, err := repo.GetBroadcastVariationStats(ctx, workspaceID, broadcastID, templateID)
		require.NoError(t, err)
		require.NotNil(t, stats)
		assert.Equal(t, 0, stats.TotalSent)
		assert.Equal(t, 0, stats.TotalDelivered)
		assert.Equal(t, 0, stats.TotalFailed)
		assert.Equal(t, 0, stats.TotalOpened)
		assert.Equal(t, 0, stats.TotalClicked)
		assert.Equal(t, 0, stats.TotalBounced)
		assert.Equal(t, 0, stats.TotalComplained)
		assert.Equal(t, 0, stats.TotalUnsubscribed)
	})

	t.Run("null values", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		// Create mock rows with some NULL values
		rows := sqlmock.NewRows([]string{
			"total_sent", "total_delivered", "total_failed", "total_opened",
			"total_clicked", "total_bounced", "total_complained", "total_unsubscribed",
		}).AddRow(10, nil, 2, nil, 3, nil, nil, 1)

		mock.ExpectQuery(`SELECT .* FROM message_history WHERE broadcast_id = \$1 AND template_id = \$2`).
			WithArgs(broadcastID, templateID).
			WillReturnRows(rows)

		stats, err := repo.GetBroadcastVariationStats(ctx, workspaceID, broadcastID, templateID)
		require.NoError(t, err)
		require.NotNil(t, stats)
		assert.Equal(t, 10, stats.TotalSent)
		assert.Equal(t, 0, stats.TotalDelivered) // Should be 0 for NULL
		assert.Equal(t, 2, stats.TotalFailed)
		assert.Equal(t, 0, stats.TotalOpened) // Should be 0 for NULL
		assert.Equal(t, 3, stats.TotalClicked)
		assert.Equal(t, 0, stats.TotalBounced)    // Should be 0 for NULL
		assert.Equal(t, 0, stats.TotalComplained) // Should be 0 for NULL
		assert.Equal(t, 1, stats.TotalUnsubscribed)
	})
}

func TestMessageHistoryRepository_SetStatusesIfNotSet(t *testing.T) {
	mockWorkspaceRepo, repo, mock, db, cleanup := setupMessageHistoryTest(t)
	defer cleanup()

	ctx := context.Background()
	workspaceID := "workspace-123"
	// Use a fixed timestamp to avoid timing issues in CI
	now := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)

	// Create a batch of status updates
	updates := []domain.MessageEventUpdate{
		{
			ID:        "msg-123",
			Event:     domain.MessageEventDelivered,
			Timestamp: now,
		},
		{
			ID:        "msg-456",
			Event:     domain.MessageEventDelivered,
			Timestamp: now,
		},
		{
			ID:        "msg-789",
			Event:     domain.MessageEventBounced,
			Timestamp: now,
		},
	}

	t.Run("successful batch update - delivered status only", func(t *testing.T) {
		// Test with only delivered status updates to avoid order dependency
		deliveredUpdates := []domain.MessageEventUpdate{
			{
				ID:        "msg-123",
				Event:     domain.MessageEventDelivered,
				Timestamp: now,
			},
			{
				ID:        "msg-456",
				Event:     domain.MessageEventDelivered,
				Timestamp: now,
			},
		}

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		// Expect batch query for delivered status updates (2 messages)
		mock.ExpectExec(`UPDATE message_history SET delivered_at = updates\.timestamp, status_info = COALESCE\(LEFT\(updates\.status_info, 255\), message_history\.status_info\), updated_at = \$1::TIMESTAMP WITH TIME ZONE FROM \(VALUES \(\$2, \$3::TIMESTAMP WITH TIME ZONE, \$4\), \(\$5, \$6::TIMESTAMP WITH TIME ZONE, \$7\)\) AS updates\(id, timestamp, status_info\) WHERE message_history\.id = updates\.id AND delivered_at IS NULL`).
			WithArgs(
				sqlmock.AnyArg(), // updated_at timestamp
				"msg-123",
				now,
				nil, // status_info for msg-123
				"msg-456",
				now,
				nil, // status_info for msg-456
			).
			WillReturnResult(sqlmock.NewResult(0, 2))

		err := repo.SetStatusesIfNotSet(ctx, workspaceID, deliveredUpdates)
		require.NoError(t, err)
	})

	t.Run("successful batch update - bounced status only", func(t *testing.T) {
		// Test with only bounced status updates to avoid order dependency
		bouncedUpdates := []domain.MessageEventUpdate{
			{
				ID:        "msg-789",
				Event:     domain.MessageEventBounced,
				Timestamp: now,
			},
		}

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		// Expect batch query for bounced status updates (1 message)
		mock.ExpectExec(`UPDATE message_history SET bounced_at = updates\.timestamp, status_info = COALESCE\(LEFT\(updates\.status_info, 255\), message_history\.status_info\), updated_at = \$1::TIMESTAMP WITH TIME ZONE FROM \(VALUES \(\$2, \$3::TIMESTAMP WITH TIME ZONE, \$4\)\) AS updates\(id, timestamp, status_info\) WHERE message_history\.id = updates\.id AND bounced_at IS NULL`).
			WithArgs(
				sqlmock.AnyArg(), // updated_at timestamp
				"msg-789",
				now,
				nil, // status_info for msg-789
			).
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := repo.SetStatusesIfNotSet(ctx, workspaceID, bouncedUpdates)
		require.NoError(t, err)
	})

	t.Run("successful batch update - single status", func(t *testing.T) {
		// Only one status type
		singleStatusUpdates := []domain.MessageEventUpdate{
			{
				ID:        "msg-123",
				Event:     domain.MessageEventOpened,
				Timestamp: now,
			},
			{
				ID:        "msg-456",
				Event:     domain.MessageEventOpened,
				Timestamp: now.Add(1 * time.Second),
			},
		}

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		// Expect single batch query for opened status updates (2 messages)
		mock.ExpectExec(`UPDATE message_history SET opened_at = updates\.timestamp, status_info = COALESCE\(LEFT\(updates\.status_info, 255\), message_history\.status_info\), updated_at = \$1::TIMESTAMP WITH TIME ZONE FROM \(VALUES \(\$2, \$3::TIMESTAMP WITH TIME ZONE, \$4\), \(\$5, \$6::TIMESTAMP WITH TIME ZONE, \$7\)\) AS updates\(id, timestamp, status_info\) WHERE message_history\.id = updates\.id AND opened_at IS NULL`).
			WithArgs(
				sqlmock.AnyArg(), // updated_at timestamp
				"msg-123",
				now,
				nil, // status_info for msg-123
				"msg-456",
				now.Add(1*time.Second),
				nil, // status_info for msg-456
			).
			WillReturnResult(sqlmock.NewResult(0, 2))

		err := repo.SetStatusesIfNotSet(ctx, workspaceID, singleStatusUpdates)
		require.NoError(t, err)
	})

	t.Run("empty updates", func(t *testing.T) {
		// No database calls should be made when the updates slice is empty
		err := repo.SetStatusesIfNotSet(ctx, workspaceID, []domain.MessageEventUpdate{})
		require.NoError(t, err)
	})

	t.Run("workspace connection error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(nil, errors.New("connection error"))

		err := repo.SetStatusesIfNotSet(ctx, workspaceID, updates)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to get workspace connection")
	})

	t.Run("invalid status", func(t *testing.T) {
		invalidUpdates := []domain.MessageEventUpdate{
			{
				ID:        "msg-123",
				Event:     domain.MessageEvent("invalid"),
				Timestamp: now,
			},
		}

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		err := repo.SetStatusesIfNotSet(ctx, workspaceID, invalidUpdates)
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid status")
	})

	t.Run("database execution error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		mock.ExpectExec(`UPDATE message_history SET delivered_at = updates\.timestamp, status_info = COALESCE\(LEFT\(updates\.status_info, 255\), message_history\.status_info\), updated_at = \$1::TIMESTAMP WITH TIME ZONE FROM \(VALUES \(\$2, \$3::TIMESTAMP WITH TIME ZONE, \$4\), \(\$5, \$6::TIMESTAMP WITH TIME ZONE, \$7\)\) AS updates\(id, timestamp, status_info\) WHERE message_history\.id = updates\.id AND delivered_at IS NULL`).
			WithArgs(
				sqlmock.AnyArg(),
				"msg-123",
				now,
				nil, // status_info for msg-123
				"msg-456",
				now,
				nil, // status_info for msg-456
			).
			WillReturnError(errors.New("database error"))

		// Only include the delivered status updates to trigger the first query error
		deliveredUpdates := []domain.MessageEventUpdate{
			{
				ID:        "msg-123",
				Event:     domain.MessageEventDelivered,
				Timestamp: now,
			},
			{
				ID:        "msg-456",
				Event:     domain.MessageEventDelivered,
				Timestamp: now,
			},
		}

		err := repo.SetStatusesIfNotSet(ctx, workspaceID, deliveredUpdates)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to batch update message statuses for status")
	})

	t.Run("integration with single status method", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		// Expect the batch version to be called with a single update
		mock.ExpectExec(`UPDATE message_history SET delivered_at = updates\.timestamp, status_info = COALESCE\(LEFT\(updates\.status_info, 255\), message_history\.status_info\), updated_at = \$1::TIMESTAMP WITH TIME ZONE FROM \(VALUES \(\$2, \$3::TIMESTAMP WITH TIME ZONE, \$4\)\) AS updates\(id, timestamp, status_info\) WHERE message_history\.id = updates\.id AND delivered_at IS NULL`).
			WithArgs(
				sqlmock.AnyArg(),
				"msg-123",
				now,
				nil, // status_info for msg-123
			).
			WillReturnResult(sqlmock.NewResult(0, 1))

		// Call the single status method
		err := repo.SetStatusesIfNotSet(ctx, workspaceID, []domain.MessageEventUpdate{
			{
				ID:        "msg-123",
				Event:     domain.MessageEventDelivered,
				Timestamp: now,
			},
		})
		require.NoError(t, err)
	})

	t.Run("successful batch update with status_info", func(t *testing.T) {
		// Test updates with status_info provided - use only one event type to avoid order issues
		statusInfo1 := "Hard bounce: mailbox does not exist"

		updatesWithStatusInfo := []domain.MessageEventUpdate{
			{
				ID:         "msg-123",
				Event:      domain.MessageEventBounced,
				Timestamp:  now,
				StatusInfo: &statusInfo1,
			},
		}

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		// Expect batch query for bounced status updates (1 message with status_info)
		mock.ExpectExec(`UPDATE message_history SET bounced_at = updates\.timestamp, status_info = COALESCE\(LEFT\(updates\.status_info, 255\), message_history\.status_info\), updated_at = \$1::TIMESTAMP WITH TIME ZONE FROM \(VALUES \(\$2, \$3::TIMESTAMP WITH TIME ZONE, \$4\)\) AS updates\(id, timestamp, status_info\) WHERE message_history\.id = updates\.id AND bounced_at IS NULL`).
			WithArgs(
				sqlmock.AnyArg(), // updated_at timestamp
				"msg-123",
				now,
				&statusInfo1, // status_info for msg-123
			).
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := repo.SetStatusesIfNotSet(ctx, workspaceID, updatesWithStatusInfo)
		require.NoError(t, err)
	})
}

func TestMessageHistoryRepository_ListMessages(t *testing.T) {
	mockWorkspaceRepo, repo, mock, db, cleanup := setupMessageHistoryTest(t)
	defer cleanup()

	ctx := context.Background()
	workspaceID := "workspace-123"

	// Create sample messages for testing
	// Use a fixed timestamp to avoid timing issues in CI
	now := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)
	oneHourAgo := now.Add(-1 * time.Hour)
	twoHoursAgo := now.Add(-2 * time.Hour)

	message1 := &domain.MessageHistory{
		ID:              "msg-1",
		ContactEmail:    "user1@example.com",
		BroadcastID:     stringPtr("broadcast-1"),
		TemplateID:      "template-1",
		TemplateVersion: 1,
		Channel:         "email",
		StatusInfo:      nil,
		MessageData:     domain.MessageData{Data: map[string]interface{}{"subject": "Test 1"}},
		SentAt:          twoHoursAgo,
		DeliveredAt:     &oneHourAgo,
		CreatedAt:       twoHoursAgo,
		UpdatedAt:       twoHoursAgo,
	}

	message2 := &domain.MessageHistory{
		ID:              "msg-2",
		ContactEmail:    "user2@example.com",
		BroadcastID:     stringPtr("broadcast-2"),
		TemplateID:      "template-2",
		TemplateVersion: 1,
		Channel:         "sms",
		StatusInfo:      nil,
		MessageData:     domain.MessageData{Data: map[string]interface{}{"body": "Test SMS"}},
		SentAt:          oneHourAgo,
		DeliveredAt:     &now,
		OpenedAt:        &now,
		CreatedAt:       oneHourAgo,
		UpdatedAt:       oneHourAgo,
	}

	t.Run("successful listing with default parameters", func(t *testing.T) {
		params := domain.MessageListParams{
			Limit: 20,
		}

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		// Convert MessageData to JSON for proper DB response mocking
		messageData1JSON, _ := json.Marshal(message1.MessageData)
		messageData2JSON, _ := json.Marshal(message2.MessageData)

		rows := sqlmock.NewRows([]string{
			"id", "external_id", "contact_email", "broadcast_id", "automation_id", "list_id", "template_id", "template_version",
			"channel", "status_info", "message_data", "channel_options", "attachments", "sent_at", "delivered_at",
			"failed_at", "opened_at", "clicked_at", "bounced_at", "complained_at",
			"unsubscribed_at", "created_at", "updated_at",
		}).
			AddRow(
				message2.ID, message2.ExternalID, message2.ContactEmail, message2.BroadcastID, nil, "{}", message2.TemplateID, message2.TemplateVersion,
				message2.Channel, message2.StatusInfo, messageData2JSON, nil, []byte("[]"), message2.SentAt, message2.DeliveredAt,
				message2.FailedAt, message2.OpenedAt, message2.ClickedAt, message2.BouncedAt, message2.ComplainedAt,
				message2.UnsubscribedAt, message2.CreatedAt, message2.UpdatedAt,
			).
			AddRow(
				message1.ID, message1.ExternalID, message1.ContactEmail, message1.BroadcastID, nil, "{}", message1.TemplateID, message1.TemplateVersion,
				message1.Channel, message1.StatusInfo, messageData1JSON, nil, []byte("[]"), message1.SentAt, message1.DeliveredAt,
				message1.FailedAt, message1.OpenedAt, message1.ClickedAt, message1.BouncedAt, message1.ComplainedAt,
				message1.UnsubscribedAt, message1.CreatedAt, message1.UpdatedAt,
			)

		mock.ExpectQuery(`SELECT id, external_id, contact_email, broadcast_id, automation_id, list_id, template_id, template_version, channel, status_info, message_data, channel_options, attachments, sent_at, delivered_at, failed_at, opened_at, clicked_at, bounced_at, complained_at, unsubscribed_at, created_at, updated_at FROM message_history ORDER BY created_at DESC, id DESC LIMIT 21`).
			WillReturnRows(rows)

		messages, nextCursor, err := repo.ListMessages(ctx, workspaceID, testSecretKey, params)
		require.NoError(t, err)
		require.Len(t, messages, 2)
		assert.Equal(t, "", nextCursor) // No next cursor since we have fewer than limit+1 results
		assert.Equal(t, "msg-2", messages[0].ID)
		assert.Equal(t, "msg-1", messages[1].ID)
	})

	t.Run("successful listing with channel filter", func(t *testing.T) {
		params := domain.MessageListParams{
			Limit:   10,
			Channel: "email",
		}

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		messageData1JSON, _ := json.Marshal(message1.MessageData)

		rows := sqlmock.NewRows([]string{
			"id", "external_id", "contact_email", "broadcast_id", "automation_id", "list_id", "template_id", "template_version",
			"channel", "status_info", "message_data", "channel_options", "attachments", "sent_at", "delivered_at",
			"failed_at", "opened_at", "clicked_at", "bounced_at", "complained_at",
			"unsubscribed_at", "created_at", "updated_at",
		}).
			AddRow(
				message1.ID, message1.ExternalID, message1.ContactEmail, message1.BroadcastID, nil, "{}", message1.TemplateID, message1.TemplateVersion,
				message1.Channel, message1.StatusInfo, messageData1JSON, nil, []byte("[]"), message1.SentAt, message1.DeliveredAt,
				message1.FailedAt, message1.OpenedAt, message1.ClickedAt, message1.BouncedAt, message1.ComplainedAt,
				message1.UnsubscribedAt, message1.CreatedAt, message1.UpdatedAt,
			)

		mock.ExpectQuery(`SELECT id, external_id, contact_email, broadcast_id, automation_id, list_id, template_id, template_version, channel, status_info, message_data, channel_options, attachments, sent_at, delivered_at, failed_at, opened_at, clicked_at, bounced_at, complained_at, unsubscribed_at, created_at, updated_at FROM message_history WHERE channel = \$1 ORDER BY created_at DESC, id DESC LIMIT 11`).
			WithArgs("email").
			WillReturnRows(rows)

		messages, nextCursor, err := repo.ListMessages(ctx, workspaceID, testSecretKey, params)
		require.NoError(t, err)
		require.Len(t, messages, 1)
		assert.Equal(t, "", nextCursor)
		assert.Equal(t, "email", messages[0].Channel)
	})

	t.Run("successful listing with contact email filter", func(t *testing.T) {
		params := domain.MessageListParams{
			Limit:        10,
			ContactEmail: "user1@example.com",
		}

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		messageData1JSON, _ := json.Marshal(message1.MessageData)

		rows := sqlmock.NewRows([]string{
			"id", "external_id", "contact_email", "broadcast_id", "automation_id", "list_id", "template_id", "template_version",
			"channel", "status_info", "message_data", "channel_options", "attachments", "sent_at", "delivered_at",
			"failed_at", "opened_at", "clicked_at", "bounced_at", "complained_at",
			"unsubscribed_at", "created_at", "updated_at",
		}).
			AddRow(
				message1.ID, message1.ExternalID, message1.ContactEmail, message1.BroadcastID, nil, "{}", message1.TemplateID, message1.TemplateVersion,
				message1.Channel, message1.StatusInfo, messageData1JSON, nil, []byte("[]"), message1.SentAt, message1.DeliveredAt,
				message1.FailedAt, message1.OpenedAt, message1.ClickedAt, message1.BouncedAt, message1.ComplainedAt,
				message1.UnsubscribedAt, message1.CreatedAt, message1.UpdatedAt,
			)

		mock.ExpectQuery(`SELECT id, external_id, contact_email, broadcast_id, automation_id, list_id, template_id, template_version, channel, status_info, message_data, channel_options, attachments, sent_at, delivered_at, failed_at, opened_at, clicked_at, bounced_at, complained_at, unsubscribed_at, created_at, updated_at FROM message_history WHERE contact_email = \$1 ORDER BY created_at DESC, id DESC LIMIT 11`).
			WithArgs("user1@example.com").
			WillReturnRows(rows)

		messages, nextCursor, err := repo.ListMessages(ctx, workspaceID, testSecretKey, params)
		require.NoError(t, err)
		require.Len(t, messages, 1)
		assert.Equal(t, "", nextCursor)
		assert.Equal(t, "user1@example.com", messages[0].ContactEmail)
	})

	t.Run("successful listing with broadcast ID filter", func(t *testing.T) {
		params := domain.MessageListParams{
			Limit:       10,
			BroadcastID: "broadcast-1",
		}

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		messageData1JSON, _ := json.Marshal(message1.MessageData)

		rows := sqlmock.NewRows([]string{
			"id", "external_id", "contact_email", "broadcast_id", "automation_id", "list_id", "template_id", "template_version",
			"channel", "status_info", "message_data", "channel_options", "attachments", "sent_at", "delivered_at",
			"failed_at", "opened_at", "clicked_at", "bounced_at", "complained_at",
			"unsubscribed_at", "created_at", "updated_at",
		}).
			AddRow(
				message1.ID, message1.ExternalID, message1.ContactEmail, message1.BroadcastID, nil, "{}", message1.TemplateID, message1.TemplateVersion,
				message1.Channel, message1.StatusInfo, messageData1JSON, nil, []byte("[]"), message1.SentAt, message1.DeliveredAt,
				message1.FailedAt, message1.OpenedAt, message1.ClickedAt, message1.BouncedAt, message1.ComplainedAt,
				message1.UnsubscribedAt, message1.CreatedAt, message1.UpdatedAt,
			)

		mock.ExpectQuery(`SELECT id, external_id, contact_email, broadcast_id, automation_id, list_id, template_id, template_version, channel, status_info, message_data, channel_options, attachments, sent_at, delivered_at, failed_at, opened_at, clicked_at, bounced_at, complained_at, unsubscribed_at, created_at, updated_at FROM message_history WHERE broadcast_id = \$1 ORDER BY created_at DESC, id DESC LIMIT 11`).
			WithArgs("broadcast-1").
			WillReturnRows(rows)

		messages, nextCursor, err := repo.ListMessages(ctx, workspaceID, testSecretKey, params)
		require.NoError(t, err)
		require.Len(t, messages, 1)
		assert.Equal(t, "", nextCursor)
		assert.Equal(t, "broadcast-1", *messages[0].BroadcastID)
	})

	t.Run("successful listing with template ID filter", func(t *testing.T) {
		params := domain.MessageListParams{
			Limit:      10,
			TemplateID: "template-1",
		}

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		messageData1JSON, _ := json.Marshal(message1.MessageData)

		rows := sqlmock.NewRows([]string{
			"id", "external_id", "contact_email", "broadcast_id", "automation_id", "list_id", "template_id", "template_version",
			"channel", "status_info", "message_data", "channel_options", "attachments", "sent_at", "delivered_at",
			"failed_at", "opened_at", "clicked_at", "bounced_at", "complained_at",
			"unsubscribed_at", "created_at", "updated_at",
		}).
			AddRow(
				message1.ID, message1.ExternalID, message1.ContactEmail, message1.BroadcastID, nil, "{}", message1.TemplateID, message1.TemplateVersion,
				message1.Channel, message1.StatusInfo, messageData1JSON, nil, []byte("[]"), message1.SentAt, message1.DeliveredAt,
				message1.FailedAt, message1.OpenedAt, message1.ClickedAt, message1.BouncedAt, message1.ComplainedAt,
				message1.UnsubscribedAt, message1.CreatedAt, message1.UpdatedAt,
			)

		mock.ExpectQuery(`SELECT id, external_id, contact_email, broadcast_id, automation_id, list_id, template_id, template_version, channel, status_info, message_data, channel_options, attachments, sent_at, delivered_at, failed_at, opened_at, clicked_at, bounced_at, complained_at, unsubscribed_at, created_at, updated_at FROM message_history WHERE template_id = \$1 ORDER BY created_at DESC, id DESC LIMIT 11`).
			WithArgs("template-1").
			WillReturnRows(rows)

		messages, nextCursor, err := repo.ListMessages(ctx, workspaceID, testSecretKey, params)
		require.NoError(t, err)
		require.Len(t, messages, 1)
		assert.Equal(t, "", nextCursor)
		assert.Equal(t, "template-1", messages[0].TemplateID)
	})

	t.Run("successful listing with boolean filters", func(t *testing.T) {
		isDelivered := true
		isOpened := false

		params := domain.MessageListParams{
			Limit:       10,
			IsDelivered: &isDelivered,
			IsOpened:    &isOpened,
		}

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		messageData1JSON, _ := json.Marshal(message1.MessageData)

		rows := sqlmock.NewRows([]string{
			"id", "external_id", "contact_email", "broadcast_id", "automation_id", "list_id", "template_id", "template_version",
			"channel", "status_info", "message_data", "channel_options", "attachments", "sent_at", "delivered_at",
			"failed_at", "opened_at", "clicked_at", "bounced_at", "complained_at",
			"unsubscribed_at", "created_at", "updated_at",
		}).
			AddRow(
				message1.ID, message1.ExternalID, message1.ContactEmail, message1.BroadcastID, nil, "{}", message1.TemplateID, message1.TemplateVersion,
				message1.Channel, message1.StatusInfo, messageData1JSON, nil, []byte("[]"), message1.SentAt, message1.DeliveredAt,
				message1.FailedAt, message1.OpenedAt, message1.ClickedAt, message1.BouncedAt, message1.ComplainedAt,
				message1.UnsubscribedAt, message1.CreatedAt, message1.UpdatedAt,
			)

		// Boolean filters are correctly implemented as IS NOT NULL / IS NULL checks
		mock.ExpectQuery(`SELECT id, external_id, contact_email, broadcast_id, automation_id, list_id, template_id, template_version, channel, status_info, message_data, channel_options, attachments, sent_at, delivered_at, failed_at, opened_at, clicked_at, bounced_at, complained_at, unsubscribed_at, created_at, updated_at FROM message_history WHERE delivered_at IS NOT NULL AND opened_at IS NULL ORDER BY created_at DESC, id DESC LIMIT 11`).
			WillReturnRows(rows)

		messages, nextCursor, err := repo.ListMessages(ctx, workspaceID, testSecretKey, params)
		require.NoError(t, err)
		require.Len(t, messages, 1)
		assert.Equal(t, "", nextCursor)
	})

	t.Run("successful listing with time range filters", func(t *testing.T) {
		sentAfter := twoHoursAgo.Add(-30 * time.Minute)
		sentBefore := now.Add(-30 * time.Minute)

		params := domain.MessageListParams{
			Limit:      10,
			SentAfter:  &sentAfter,
			SentBefore: &sentBefore,
		}

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		messageData1JSON, _ := json.Marshal(message1.MessageData)

		rows := sqlmock.NewRows([]string{
			"id", "external_id", "contact_email", "broadcast_id", "automation_id", "list_id", "template_id", "template_version",
			"channel", "status_info", "message_data", "channel_options", "attachments", "sent_at", "delivered_at",
			"failed_at", "opened_at", "clicked_at", "bounced_at", "complained_at",
			"unsubscribed_at", "created_at", "updated_at",
		}).
			AddRow(
				message1.ID, message1.ExternalID, message1.ContactEmail, message1.BroadcastID, nil, "{}", message1.TemplateID, message1.TemplateVersion,
				message1.Channel, message1.StatusInfo, messageData1JSON, nil, []byte("[]"), message1.SentAt, message1.DeliveredAt,
				message1.FailedAt, message1.OpenedAt, message1.ClickedAt, message1.BouncedAt, message1.ComplainedAt,
				message1.UnsubscribedAt, message1.CreatedAt, message1.UpdatedAt,
			)

		mock.ExpectQuery(`SELECT id, external_id, contact_email, broadcast_id, automation_id, list_id, template_id, template_version, channel, status_info, message_data, channel_options, attachments, sent_at, delivered_at, failed_at, opened_at, clicked_at, bounced_at, complained_at, unsubscribed_at, created_at, updated_at FROM message_history WHERE sent_at >= \$1 AND sent_at <= \$2 ORDER BY created_at DESC, id DESC LIMIT 11`).
			WithArgs(sentAfter, sentBefore).
			WillReturnRows(rows)

		messages, nextCursor, err := repo.ListMessages(ctx, workspaceID, testSecretKey, params)
		require.NoError(t, err)
		require.Len(t, messages, 1)
		assert.Equal(t, "", nextCursor)
	})

	t.Run("successful listing with cursor pagination", func(t *testing.T) {
		// Create a cursor based on message1's timestamp and ID
		// Use the exact same time format that will be parsed back
		cursorTime := message1.CreatedAt.Truncate(time.Second) // Remove nanoseconds to match RFC3339 precision
		cursorStr := cursorTime.Format(time.RFC3339) + "~" + message1.ID
		cursor := base64.StdEncoding.EncodeToString([]byte(cursorStr))

		params := domain.MessageListParams{
			Limit:  10,
			Cursor: cursor,
		}

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		messageData2JSON, _ := json.Marshal(message2.MessageData)

		rows := sqlmock.NewRows([]string{
			"id", "external_id", "contact_email", "broadcast_id", "automation_id", "list_id", "template_id", "template_version",
			"channel", "status_info", "message_data", "channel_options", "attachments", "sent_at", "delivered_at",
			"failed_at", "opened_at", "clicked_at", "bounced_at", "complained_at",
			"unsubscribed_at", "created_at", "updated_at",
		}).
			AddRow(
				message2.ID, message2.ExternalID, message2.ContactEmail, message2.BroadcastID, nil, "{}", message2.TemplateID, message2.TemplateVersion,
				message2.Channel, message2.StatusInfo, messageData2JSON, nil, []byte("[]"), message2.SentAt, message2.DeliveredAt,
				message2.FailedAt, message2.OpenedAt, message2.ClickedAt, message2.BouncedAt, message2.ComplainedAt,
				message2.UnsubscribedAt, message2.CreatedAt, message2.UpdatedAt,
			)

		// The query should include cursor-based WHERE clause
		mock.ExpectQuery(`SELECT id, external_id, contact_email, broadcast_id, automation_id, list_id, template_id, template_version, channel, status_info, message_data, channel_options, attachments, sent_at, delivered_at, failed_at, opened_at, clicked_at, bounced_at, complained_at, unsubscribed_at, created_at, updated_at FROM message_history WHERE \(created_at < \$1 OR \(created_at = \$2 AND id < \$3\)\) ORDER BY created_at DESC, id DESC LIMIT 11`).
			WithArgs(cursorTime, cursorTime, message1.ID).
			WillReturnRows(rows)

		messages, nextCursor, err := repo.ListMessages(ctx, workspaceID, testSecretKey, params)
		require.NoError(t, err)
		require.Len(t, messages, 1)
		assert.Equal(t, "", nextCursor)
		assert.Equal(t, "msg-2", messages[0].ID)
	})

	t.Run("successful listing with pagination and next cursor", func(t *testing.T) {
		params := domain.MessageListParams{
			Limit: 1, // Small limit to trigger pagination
		}

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		messageData1JSON, _ := json.Marshal(message1.MessageData)
		messageData2JSON, _ := json.Marshal(message2.MessageData)

		// Return 2 rows (limit + 1) to indicate there are more results
		rows := sqlmock.NewRows([]string{
			"id", "external_id", "contact_email", "broadcast_id", "automation_id", "list_id", "template_id", "template_version",
			"channel", "status_info", "message_data", "channel_options", "attachments", "sent_at", "delivered_at",
			"failed_at", "opened_at", "clicked_at", "bounced_at", "complained_at",
			"unsubscribed_at", "created_at", "updated_at",
		}).
			AddRow(
				message2.ID, message2.ExternalID, message2.ContactEmail, message2.BroadcastID, nil, "{}", message2.TemplateID, message2.TemplateVersion,
				message2.Channel, message2.StatusInfo, messageData2JSON, nil, []byte("[]"), message2.SentAt, message2.DeliveredAt,
				message2.FailedAt, message2.OpenedAt, message2.ClickedAt, message2.BouncedAt, message2.ComplainedAt,
				message2.UnsubscribedAt, message2.CreatedAt, message2.UpdatedAt,
			).
			AddRow(
				message1.ID, message1.ExternalID, message1.ContactEmail, message1.BroadcastID, nil, "{}", message1.TemplateID, message1.TemplateVersion,
				message1.Channel, message1.StatusInfo, messageData1JSON, nil, []byte("[]"), message1.SentAt, message1.DeliveredAt,
				message1.FailedAt, message1.OpenedAt, message1.ClickedAt, message1.BouncedAt, message1.ComplainedAt,
				message1.UnsubscribedAt, message1.CreatedAt, message1.UpdatedAt,
			)

		// No cursor provided, so no WHERE clause for cursor pagination
		mock.ExpectQuery(`SELECT id, external_id, contact_email, broadcast_id, automation_id, list_id, template_id, template_version, channel, status_info, message_data, channel_options, attachments, sent_at, delivered_at, failed_at, opened_at, clicked_at, bounced_at, complained_at, unsubscribed_at, created_at, updated_at FROM message_history ORDER BY created_at DESC, id DESC LIMIT 2`).
			WillReturnRows(rows)

		messages, nextCursor, err := repo.ListMessages(ctx, workspaceID, testSecretKey, params)
		require.NoError(t, err)
		require.Len(t, messages, 1)        // Should return only the limit, not the extra row
		assert.NotEqual(t, "", nextCursor) // Should have a next cursor
		assert.Equal(t, "msg-2", messages[0].ID)

		// Verify cursor is properly encoded
		decodedCursor, err := base64.StdEncoding.DecodeString(nextCursor)
		require.NoError(t, err)
		cursorParts := strings.Split(string(decodedCursor), "~")
		require.Len(t, cursorParts, 2)
		assert.Equal(t, message2.ID, cursorParts[1])
	})

	t.Run("workspace connection error", func(t *testing.T) {
		params := domain.MessageListParams{
			Limit: 10,
		}

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(nil, errors.New("connection error"))

		messages, nextCursor, err := repo.ListMessages(ctx, workspaceID, testSecretKey, params)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to get workspace connection")
		require.Nil(t, messages)
		require.Equal(t, "", nextCursor)
	})

	t.Run("invalid cursor encoding", func(t *testing.T) {
		params := domain.MessageListParams{
			Limit:  10,
			Cursor: "invalid-base64!@#",
		}

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		messages, nextCursor, err := repo.ListMessages(ctx, workspaceID, testSecretKey, params)
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid cursor encoding")
		require.Nil(t, messages)
		require.Equal(t, "", nextCursor)
	})

	t.Run("invalid cursor format", func(t *testing.T) {
		// Create a cursor with invalid format (missing ~)
		invalidCursor := base64.StdEncoding.EncodeToString([]byte("invalid-format"))

		params := domain.MessageListParams{
			Limit:  10,
			Cursor: invalidCursor,
		}

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		messages, nextCursor, err := repo.ListMessages(ctx, workspaceID, testSecretKey, params)
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid cursor format")
		require.Nil(t, messages)
		require.Equal(t, "", nextCursor)
	})

	t.Run("invalid cursor timestamp", func(t *testing.T) {
		// Create a cursor with invalid timestamp
		invalidCursor := base64.StdEncoding.EncodeToString([]byte("invalid-timestamp~msg-1"))

		params := domain.MessageListParams{
			Limit:  10,
			Cursor: invalidCursor,
		}

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		messages, nextCursor, err := repo.ListMessages(ctx, workspaceID, testSecretKey, params)
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid cursor timestamp format")
		require.Nil(t, messages)
		require.Equal(t, "", nextCursor)
	})

	t.Run("query execution error", func(t *testing.T) {
		params := domain.MessageListParams{
			Limit: 10,
		}

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		mock.ExpectQuery(`SELECT id, external_id, contact_email, broadcast_id, automation_id, list_id, template_id, template_version, channel, status_info, message_data, channel_options, attachments, sent_at, delivered_at, failed_at, opened_at, clicked_at, bounced_at, complained_at, unsubscribed_at, created_at, updated_at FROM message_history ORDER BY created_at DESC, id DESC LIMIT 11`).
			WillReturnError(errors.New("query execution error"))

		messages, nextCursor, err := repo.ListMessages(ctx, workspaceID, testSecretKey, params)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to query message history")
		require.Nil(t, messages)
		require.Equal(t, "", nextCursor)
	})

	t.Run("row scanning error", func(t *testing.T) {
		params := domain.MessageListParams{
			Limit: 10,
		}

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		// Create rows with invalid data that will cause scanning error
		rows := sqlmock.NewRows([]string{
			"id", "external_id", "contact_email", "broadcast_id", "automation_id", "list_id", "template_id", "template_version",
			"channel", "status_info", "message_data", "channel_options", "attachments", "sent_at", "delivered_at",
			"failed_at", "opened_at", "clicked_at", "bounced_at", "complained_at",
			"unsubscribed_at", "created_at", "updated_at",
		}).
			AddRow(
				"msg-1", "external-123", "user@example.com", nil, nil, "{}", "template-1", "invalid-version", // invalid template_version type
				"email", nil, `{"data":{}}`, nil, []byte("[]"), now, nil,
				nil, nil, nil, nil, nil,
				nil, now, now,
			)

		mock.ExpectQuery(`SELECT id, external_id, contact_email, broadcast_id, automation_id, list_id, template_id, template_version, channel, status_info, message_data, channel_options, attachments, sent_at, delivered_at, failed_at, opened_at, clicked_at, bounced_at, complained_at, unsubscribed_at, created_at, updated_at FROM message_history ORDER BY created_at DESC, id DESC LIMIT 11`).
			WillReturnRows(rows)

		messages, nextCursor, err := repo.ListMessages(ctx, workspaceID, testSecretKey, params)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to scan message history row")
		require.Nil(t, messages)
		require.Equal(t, "", nextCursor)
	})

	t.Run("rows iteration error", func(t *testing.T) {
		params := domain.MessageListParams{
			Limit: 10,
		}

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		messageData1JSON, _ := json.Marshal(message1.MessageData)

		// Add a valid row and use CloseError to trigger rows.Err()
		rows := sqlmock.NewRows([]string{
			"id", "external_id", "contact_email", "broadcast_id", "automation_id", "list_id", "template_id", "template_version",
			"channel", "status_info", "message_data", "channel_options", "attachments", "sent_at", "delivered_at",
			"failed_at", "opened_at", "clicked_at", "bounced_at", "complained_at",
			"unsubscribed_at", "created_at", "updated_at",
		}).
			AddRow(
				message1.ID, message1.ExternalID, message1.ContactEmail, message1.BroadcastID, nil, "{}", message1.TemplateID, message1.TemplateVersion,
				message1.Channel, message1.StatusInfo, messageData1JSON, nil, []byte("[]"), message1.SentAt, message1.DeliveredAt,
				message1.FailedAt, message1.OpenedAt, message1.ClickedAt, message1.BouncedAt, message1.ComplainedAt,
				message1.UnsubscribedAt, message1.CreatedAt, message1.UpdatedAt,
			).
			CloseError(errors.New("row iteration error"))

		mock.ExpectQuery(`SELECT id, external_id, contact_email, broadcast_id, automation_id, list_id, template_id, template_version, channel, status_info, message_data, channel_options, attachments, sent_at, delivered_at, failed_at, opened_at, clicked_at, bounced_at, complained_at, unsubscribed_at, created_at, updated_at FROM message_history ORDER BY created_at DESC, id DESC LIMIT 11`).
			WillReturnRows(rows)

		messages, nextCursor, err := repo.ListMessages(ctx, workspaceID, testSecretKey, params)
		require.Error(t, err)
		require.Contains(t, err.Error(), "error iterating message history rows")
		require.Nil(t, messages)
		require.Equal(t, "", nextCursor)
	})

	t.Run("default limit when zero provided", func(t *testing.T) {
		params := domain.MessageListParams{
			Limit: 0, // Should default to 20
		}

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		rows := sqlmock.NewRows([]string{
			"id", "external_id", "contact_email", "broadcast_id", "automation_id", "list_id", "template_id", "template_version",
			"channel", "status_info", "message_data", "channel_options", "attachments", "sent_at", "delivered_at",
			"failed_at", "opened_at", "clicked_at", "bounced_at", "complained_at",
			"unsubscribed_at", "created_at", "updated_at",
		})

		mock.ExpectQuery(`SELECT id, external_id, contact_email, broadcast_id, automation_id, list_id, template_id, template_version, channel, status_info, message_data, channel_options, attachments, sent_at, delivered_at, failed_at, opened_at, clicked_at, bounced_at, complained_at, unsubscribed_at, created_at, updated_at FROM message_history ORDER BY created_at DESC, id DESC LIMIT 21`).
			WillReturnRows(rows)

		messages, nextCursor, err := repo.ListMessages(ctx, workspaceID, testSecretKey, params)
		require.NoError(t, err)
		require.Len(t, messages, 0)
		assert.Equal(t, "", nextCursor)
	})

	t.Run("successful listing with IsSent filter", func(t *testing.T) {
		isSent := true
		params := domain.MessageListParams{
			Limit:  10,
			IsSent: &isSent,
		}

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		messageData1JSON, _ := json.Marshal(message1.MessageData)

		rows := sqlmock.NewRows([]string{
			"id", "external_id", "contact_email", "broadcast_id", "automation_id", "list_id", "template_id", "template_version",
			"channel", "status_info", "message_data", "channel_options", "attachments", "sent_at", "delivered_at",
			"failed_at", "opened_at", "clicked_at", "bounced_at", "complained_at",
			"unsubscribed_at", "created_at", "updated_at",
		}).
			AddRow(
				message1.ID, message1.ExternalID, message1.ContactEmail, message1.BroadcastID, nil, "{}", message1.TemplateID, message1.TemplateVersion,
				message1.Channel, message1.StatusInfo, messageData1JSON, nil, []byte("[]"), message1.SentAt, message1.DeliveredAt,
				message1.FailedAt, message1.OpenedAt, message1.ClickedAt, message1.BouncedAt, message1.ComplainedAt,
				message1.UnsubscribedAt, message1.CreatedAt, message1.UpdatedAt,
			)

		mock.ExpectQuery(`SELECT id, external_id, contact_email, broadcast_id, automation_id, list_id, template_id, template_version, channel, status_info, message_data, channel_options, attachments, sent_at, delivered_at, failed_at, opened_at, clicked_at, bounced_at, complained_at, unsubscribed_at, created_at, updated_at FROM message_history WHERE sent_at IS NOT NULL ORDER BY created_at DESC, id DESC LIMIT 11`).
			WillReturnRows(rows)

		messages, nextCursor, err := repo.ListMessages(ctx, workspaceID, testSecretKey, params)
		require.NoError(t, err)
		require.Len(t, messages, 1)
		assert.Equal(t, "", nextCursor)
	})

	t.Run("successful listing with IsSent false filter", func(t *testing.T) {
		isSent := false
		params := domain.MessageListParams{
			Limit:  10,
			IsSent: &isSent,
		}

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		rows := sqlmock.NewRows([]string{
			"id", "external_id", "contact_email", "broadcast_id", "automation_id", "list_id", "template_id", "template_version",
			"channel", "status_info", "message_data", "channel_options", "attachments", "sent_at", "delivered_at",
			"failed_at", "opened_at", "clicked_at", "bounced_at", "complained_at",
			"unsubscribed_at", "created_at", "updated_at",
		})

		mock.ExpectQuery(`SELECT id, external_id, contact_email, broadcast_id, automation_id, list_id, template_id, template_version, channel, status_info, message_data, channel_options, attachments, sent_at, delivered_at, failed_at, opened_at, clicked_at, bounced_at, complained_at, unsubscribed_at, created_at, updated_at FROM message_history WHERE sent_at IS NULL ORDER BY created_at DESC, id DESC LIMIT 11`).
			WillReturnRows(rows)

		messages, nextCursor, err := repo.ListMessages(ctx, workspaceID, testSecretKey, params)
		require.NoError(t, err)
		require.Len(t, messages, 0)
		assert.Equal(t, "", nextCursor)
	})

	t.Run("successful listing with IsFailed filter", func(t *testing.T) {
		isFailed := true
		params := domain.MessageListParams{
			Limit:    10,
			IsFailed: &isFailed,
		}

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		rows := sqlmock.NewRows([]string{
			"id", "external_id", "contact_email", "broadcast_id", "automation_id", "list_id", "template_id", "template_version",
			"channel", "status_info", "message_data", "channel_options", "attachments", "sent_at", "delivered_at",
			"failed_at", "opened_at", "clicked_at", "bounced_at", "complained_at",
			"unsubscribed_at", "created_at", "updated_at",
		})

		mock.ExpectQuery(`SELECT id, external_id, contact_email, broadcast_id, automation_id, list_id, template_id, template_version, channel, status_info, message_data, channel_options, attachments, sent_at, delivered_at, failed_at, opened_at, clicked_at, bounced_at, complained_at, unsubscribed_at, created_at, updated_at FROM message_history WHERE failed_at IS NOT NULL ORDER BY created_at DESC, id DESC LIMIT 11`).
			WillReturnRows(rows)

		messages, nextCursor, err := repo.ListMessages(ctx, workspaceID, testSecretKey, params)
		require.NoError(t, err)
		require.Len(t, messages, 0)
		assert.Equal(t, "", nextCursor)
	})

	t.Run("successful listing with IsClicked filter", func(t *testing.T) {
		isClicked := false
		params := domain.MessageListParams{
			Limit:     10,
			IsClicked: &isClicked,
		}

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		messageData1JSON, _ := json.Marshal(message1.MessageData)

		rows := sqlmock.NewRows([]string{
			"id", "external_id", "contact_email", "broadcast_id", "automation_id", "list_id", "template_id", "template_version",
			"channel", "status_info", "message_data", "channel_options", "attachments", "sent_at", "delivered_at",
			"failed_at", "opened_at", "clicked_at", "bounced_at", "complained_at",
			"unsubscribed_at", "created_at", "updated_at",
		}).
			AddRow(
				message1.ID, message1.ExternalID, message1.ContactEmail, message1.BroadcastID, nil, "{}", message1.TemplateID, message1.TemplateVersion,
				message1.Channel, message1.StatusInfo, messageData1JSON, nil, []byte("[]"), message1.SentAt, message1.DeliveredAt,
				message1.FailedAt, message1.OpenedAt, message1.ClickedAt, message1.BouncedAt, message1.ComplainedAt,
				message1.UnsubscribedAt, message1.CreatedAt, message1.UpdatedAt,
			)

		mock.ExpectQuery(`SELECT id, external_id, contact_email, broadcast_id, automation_id, list_id, template_id, template_version, channel, status_info, message_data, channel_options, attachments, sent_at, delivered_at, failed_at, opened_at, clicked_at, bounced_at, complained_at, unsubscribed_at, created_at, updated_at FROM message_history WHERE clicked_at IS NULL ORDER BY created_at DESC, id DESC LIMIT 11`).
			WillReturnRows(rows)

		messages, nextCursor, err := repo.ListMessages(ctx, workspaceID, testSecretKey, params)
		require.NoError(t, err)
		require.Len(t, messages, 1)
		assert.Equal(t, "", nextCursor)
	})

	t.Run("successful listing with IsBounced filter", func(t *testing.T) {
		isBounced := true
		params := domain.MessageListParams{
			Limit:     10,
			IsBounced: &isBounced,
		}

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		rows := sqlmock.NewRows([]string{
			"id", "external_id", "contact_email", "broadcast_id", "automation_id", "list_id", "template_id", "template_version",
			"channel", "status_info", "message_data", "channel_options", "attachments", "sent_at", "delivered_at",
			"failed_at", "opened_at", "clicked_at", "bounced_at", "complained_at",
			"unsubscribed_at", "created_at", "updated_at",
		})

		mock.ExpectQuery(`SELECT id, external_id, contact_email, broadcast_id, automation_id, list_id, template_id, template_version, channel, status_info, message_data, channel_options, attachments, sent_at, delivered_at, failed_at, opened_at, clicked_at, bounced_at, complained_at, unsubscribed_at, created_at, updated_at FROM message_history WHERE bounced_at IS NOT NULL ORDER BY created_at DESC, id DESC LIMIT 11`).
			WillReturnRows(rows)

		messages, nextCursor, err := repo.ListMessages(ctx, workspaceID, testSecretKey, params)
		require.NoError(t, err)
		require.Len(t, messages, 0)
		assert.Equal(t, "", nextCursor)
	})

	t.Run("successful listing with IsComplained filter", func(t *testing.T) {
		isComplained := false
		params := domain.MessageListParams{
			Limit:        10,
			IsComplained: &isComplained,
		}

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		messageData1JSON, _ := json.Marshal(message1.MessageData)

		rows := sqlmock.NewRows([]string{
			"id", "external_id", "contact_email", "broadcast_id", "automation_id", "list_id", "template_id", "template_version",
			"channel", "status_info", "message_data", "channel_options", "attachments", "sent_at", "delivered_at",
			"failed_at", "opened_at", "clicked_at", "bounced_at", "complained_at",
			"unsubscribed_at", "created_at", "updated_at",
		}).
			AddRow(
				message1.ID, message1.ExternalID, message1.ContactEmail, message1.BroadcastID, nil, "{}", message1.TemplateID, message1.TemplateVersion,
				message1.Channel, message1.StatusInfo, messageData1JSON, nil, []byte("[]"), message1.SentAt, message1.DeliveredAt,
				message1.FailedAt, message1.OpenedAt, message1.ClickedAt, message1.BouncedAt, message1.ComplainedAt,
				message1.UnsubscribedAt, message1.CreatedAt, message1.UpdatedAt,
			)

		mock.ExpectQuery(`SELECT id, external_id, contact_email, broadcast_id, automation_id, list_id, template_id, template_version, channel, status_info, message_data, channel_options, attachments, sent_at, delivered_at, failed_at, opened_at, clicked_at, bounced_at, complained_at, unsubscribed_at, created_at, updated_at FROM message_history WHERE complained_at IS NULL ORDER BY created_at DESC, id DESC LIMIT 11`).
			WillReturnRows(rows)

		messages, nextCursor, err := repo.ListMessages(ctx, workspaceID, testSecretKey, params)
		require.NoError(t, err)
		require.Len(t, messages, 1)
		assert.Equal(t, "", nextCursor)
	})

	t.Run("successful listing with IsUnsubscribed filter", func(t *testing.T) {
		isUnsubscribed := true
		params := domain.MessageListParams{
			Limit:          10,
			IsUnsubscribed: &isUnsubscribed,
		}

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		rows := sqlmock.NewRows([]string{
			"id", "external_id", "contact_email", "broadcast_id", "automation_id", "list_id", "template_id", "template_version",
			"channel", "status_info", "message_data", "channel_options", "attachments", "sent_at", "delivered_at",
			"failed_at", "opened_at", "clicked_at", "bounced_at", "complained_at",
			"unsubscribed_at", "created_at", "updated_at",
		})

		mock.ExpectQuery(`SELECT id, external_id, contact_email, broadcast_id, automation_id, list_id, template_id, template_version, channel, status_info, message_data, channel_options, attachments, sent_at, delivered_at, failed_at, opened_at, clicked_at, bounced_at, complained_at, unsubscribed_at, created_at, updated_at FROM message_history WHERE unsubscribed_at IS NOT NULL ORDER BY created_at DESC, id DESC LIMIT 11`).
			WillReturnRows(rows)

		messages, nextCursor, err := repo.ListMessages(ctx, workspaceID, testSecretKey, params)
		require.NoError(t, err)
		require.Len(t, messages, 0)
		assert.Equal(t, "", nextCursor)
	})

	t.Run("successful listing with UpdatedAfter and UpdatedBefore filters", func(t *testing.T) {
		updatedAfter := twoHoursAgo.Add(-30 * time.Minute)
		updatedBefore := now.Add(-30 * time.Minute)

		params := domain.MessageListParams{
			Limit:         10,
			UpdatedAfter:  &updatedAfter,
			UpdatedBefore: &updatedBefore,
		}

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		messageData1JSON, _ := json.Marshal(message1.MessageData)

		rows := sqlmock.NewRows([]string{
			"id", "external_id", "contact_email", "broadcast_id", "automation_id", "list_id", "template_id", "template_version",
			"channel", "status_info", "message_data", "channel_options", "attachments", "sent_at", "delivered_at",
			"failed_at", "opened_at", "clicked_at", "bounced_at", "complained_at",
			"unsubscribed_at", "created_at", "updated_at",
		}).
			AddRow(
				message1.ID, message1.ExternalID, message1.ContactEmail, message1.BroadcastID, nil, "{}", message1.TemplateID, message1.TemplateVersion,
				message1.Channel, message1.StatusInfo, messageData1JSON, nil, []byte("[]"), message1.SentAt, message1.DeliveredAt,
				message1.FailedAt, message1.OpenedAt, message1.ClickedAt, message1.BouncedAt, message1.ComplainedAt,
				message1.UnsubscribedAt, message1.CreatedAt, message1.UpdatedAt,
			)

		mock.ExpectQuery(`SELECT id, external_id, contact_email, broadcast_id, automation_id, list_id, template_id, template_version, channel, status_info, message_data, channel_options, attachments, sent_at, delivered_at, failed_at, opened_at, clicked_at, bounced_at, complained_at, unsubscribed_at, created_at, updated_at FROM message_history WHERE updated_at >= \$1 AND updated_at <= \$2 ORDER BY created_at DESC, id DESC LIMIT 11`).
			WithArgs(updatedAfter, updatedBefore).
			WillReturnRows(rows)

		messages, nextCursor, err := repo.ListMessages(ctx, workspaceID, testSecretKey, params)
		require.NoError(t, err)
		require.Len(t, messages, 1)
		assert.Equal(t, "", nextCursor)
	})

	t.Run("successful listing with ID filter", func(t *testing.T) {
		params := domain.MessageListParams{
			Limit: 10,
			ID:    "msg-1",
		}

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		messageData1JSON, _ := json.Marshal(message1.MessageData)

		rows := sqlmock.NewRows([]string{
			"id", "external_id", "contact_email", "broadcast_id", "automation_id", "list_id", "template_id", "template_version",
			"channel", "status_info", "message_data", "channel_options", "attachments", "sent_at", "delivered_at",
			"failed_at", "opened_at", "clicked_at", "bounced_at", "complained_at",
			"unsubscribed_at", "created_at", "updated_at",
		}).
			AddRow(
				message1.ID, message1.ExternalID, message1.ContactEmail, message1.BroadcastID, nil, "{}", message1.TemplateID, message1.TemplateVersion,
				message1.Channel, message1.StatusInfo, messageData1JSON, nil, []byte("[]"), message1.SentAt, message1.DeliveredAt,
				message1.FailedAt, message1.OpenedAt, message1.ClickedAt, message1.BouncedAt, message1.ComplainedAt,
				message1.UnsubscribedAt, message1.CreatedAt, message1.UpdatedAt,
			)

		mock.ExpectQuery(`SELECT id, external_id, contact_email, broadcast_id, automation_id, list_id, template_id, template_version, channel, status_info, message_data, channel_options, attachments, sent_at, delivered_at, failed_at, opened_at, clicked_at, bounced_at, complained_at, unsubscribed_at, created_at, updated_at FROM message_history WHERE id = \$1 ORDER BY created_at DESC, id DESC LIMIT 11`).
			WithArgs("msg-1").
			WillReturnRows(rows)

		messages, nextCursor, err := repo.ListMessages(ctx, workspaceID, testSecretKey, params)
		require.NoError(t, err)
		require.Len(t, messages, 1)
		assert.Equal(t, "", nextCursor)
		assert.Equal(t, "msg-1", messages[0].ID)
	})

	t.Run("successful listing with ExternalID filter", func(t *testing.T) {
		params := domain.MessageListParams{
			Limit:      10,
			ExternalID: "ext-123",
		}

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		messageData1JSON, _ := json.Marshal(message1.MessageData)
		externalID := "ext-123"
		message1.ExternalID = &externalID

		rows := sqlmock.NewRows([]string{
			"id", "external_id", "contact_email", "broadcast_id", "automation_id", "list_id", "template_id", "template_version",
			"channel", "status_info", "message_data", "channel_options", "attachments", "sent_at", "delivered_at",
			"failed_at", "opened_at", "clicked_at", "bounced_at", "complained_at",
			"unsubscribed_at", "created_at", "updated_at",
		}).
			AddRow(
				message1.ID, message1.ExternalID, message1.ContactEmail, message1.BroadcastID, nil, "{}", message1.TemplateID, message1.TemplateVersion,
				message1.Channel, message1.StatusInfo, messageData1JSON, nil, []byte("[]"), message1.SentAt, message1.DeliveredAt,
				message1.FailedAt, message1.OpenedAt, message1.ClickedAt, message1.BouncedAt, message1.ComplainedAt,
				message1.UnsubscribedAt, message1.CreatedAt, message1.UpdatedAt,
			)

		mock.ExpectQuery(`SELECT id, external_id, contact_email, broadcast_id, automation_id, list_id, template_id, template_version, channel, status_info, message_data, channel_options, attachments, sent_at, delivered_at, failed_at, opened_at, clicked_at, bounced_at, complained_at, unsubscribed_at, created_at, updated_at FROM message_history WHERE external_id = \$1 ORDER BY created_at DESC, id DESC LIMIT 11`).
			WithArgs("ext-123").
			WillReturnRows(rows)

		messages, nextCursor, err := repo.ListMessages(ctx, workspaceID, testSecretKey, params)
		require.NoError(t, err)
		require.Len(t, messages, 1)
		assert.Equal(t, "", nextCursor)
		assert.Equal(t, "ext-123", *messages[0].ExternalID)
	})

	t.Run("successful listing with ListID filter", func(t *testing.T) {
		params := domain.MessageListParams{
			Limit:  10,
			ListID: "list-abc",
		}

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		messageData1JSON, _ := json.Marshal(message1.MessageData)

		rows := sqlmock.NewRows([]string{
			"id", "external_id", "contact_email", "broadcast_id", "automation_id", "list_id", "template_id", "template_version",
			"channel", "status_info", "message_data", "channel_options", "attachments", "sent_at", "delivered_at",
			"failed_at", "opened_at", "clicked_at", "bounced_at", "complained_at",
			"unsubscribed_at", "created_at", "updated_at",
		}).
			AddRow(
				message1.ID, message1.ExternalID, message1.ContactEmail, message1.BroadcastID, nil, "list-abc", message1.TemplateID, message1.TemplateVersion,
				message1.Channel, message1.StatusInfo, messageData1JSON, nil, []byte("[]"), message1.SentAt, message1.DeliveredAt,
				message1.FailedAt, message1.OpenedAt, message1.ClickedAt, message1.BouncedAt, message1.ComplainedAt,
				message1.UnsubscribedAt, message1.CreatedAt, message1.UpdatedAt,
			)

		// The query should use simple equality check for list_id
		mock.ExpectQuery(`SELECT id, external_id, contact_email, broadcast_id, automation_id, list_id, template_id, template_version, channel, status_info, message_data, channel_options, attachments, sent_at, delivered_at, failed_at, opened_at, clicked_at, bounced_at, complained_at, unsubscribed_at, created_at, updated_at FROM message_history WHERE list_id = \$1 ORDER BY created_at DESC, id DESC LIMIT 11`).
			WithArgs("list-abc").
			WillReturnRows(rows)

		messages, nextCursor, err := repo.ListMessages(ctx, workspaceID, testSecretKey, params)
		require.NoError(t, err)
		require.Len(t, messages, 1)
		assert.Equal(t, "", nextCursor)
		// Verify the list_id equals the filtered list ID
		require.NotNil(t, messages[0].ListID)
		assert.Equal(t, "list-abc", *messages[0].ListID)
	})

	t.Run("multiple filters combined", func(t *testing.T) {
		isDelivered := true

		params := domain.MessageListParams{
			Limit:        10,
			Channel:      "email",
			ContactEmail: "user1@example.com",
			BroadcastID:  "broadcast-1",
			TemplateID:   "template-1",
			IsDelivered:  &isDelivered,
			SentAfter:    &twoHoursAgo,
		}

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		messageData1JSON, _ := json.Marshal(message1.MessageData)

		rows := sqlmock.NewRows([]string{
			"id", "external_id", "contact_email", "broadcast_id", "automation_id", "list_id", "template_id", "template_version",
			"channel", "status_info", "message_data", "channel_options", "attachments", "sent_at", "delivered_at",
			"failed_at", "opened_at", "clicked_at", "bounced_at", "complained_at",
			"unsubscribed_at", "created_at", "updated_at",
		}).
			AddRow(
				message1.ID, message1.ExternalID, message1.ContactEmail, message1.BroadcastID, nil, "{}", message1.TemplateID, message1.TemplateVersion,
				message1.Channel, message1.StatusInfo, messageData1JSON, nil, []byte("[]"), message1.SentAt, message1.DeliveredAt,
				message1.FailedAt, message1.OpenedAt, message1.ClickedAt, message1.BouncedAt, message1.ComplainedAt,
				message1.UnsubscribedAt, message1.CreatedAt, message1.UpdatedAt,
			)

		// The query should include all the filters with IS NOT NULL for boolean delivered filter
		mock.ExpectQuery(`SELECT id, external_id, contact_email, broadcast_id, automation_id, list_id, template_id, template_version, channel, status_info, message_data, channel_options, attachments, sent_at, delivered_at, failed_at, opened_at, clicked_at, bounced_at, complained_at, unsubscribed_at, created_at, updated_at FROM message_history WHERE channel = \$1 AND contact_email = \$2 AND broadcast_id = \$3 AND template_id = \$4 AND delivered_at IS NOT NULL AND sent_at >= \$5 ORDER BY created_at DESC, id DESC LIMIT 11`).
			WithArgs("email", "user1@example.com", "broadcast-1", "template-1", twoHoursAgo).
			WillReturnRows(rows)

		messages, nextCursor, err := repo.ListMessages(ctx, workspaceID, testSecretKey, params)
		require.NoError(t, err)
		require.Len(t, messages, 1)
		assert.Equal(t, "", nextCursor)
		assert.Equal(t, "email", messages[0].Channel)
	})
}

func TestMessageHistoryRepository_DeleteForEmail(t *testing.T) {
	mockWorkspaceRepo, repo, mock, db, cleanup := setupMessageHistoryTest(t)
	defer cleanup()

	ctx := context.Background()
	workspaceID := "workspace-123"
	email := "test@example.com"

	t.Run("successful deletion", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		mock.ExpectExec(`UPDATE message_history SET contact_email = \$1 WHERE contact_email = \$2`).
			WithArgs("DELETED_EMAIL", email).
			WillReturnResult(sqlmock.NewResult(0, 3)) // 3 rows affected

		err := repo.DeleteForEmail(ctx, workspaceID, email)
		require.NoError(t, err)
	})

	t.Run("no rows affected", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		mock.ExpectExec(`UPDATE message_history SET contact_email = \$1 WHERE contact_email = \$2`).
			WithArgs("DELETED_EMAIL", email).
			WillReturnResult(sqlmock.NewResult(0, 0)) // 0 rows affected

		err := repo.DeleteForEmail(ctx, workspaceID, email)
		require.NoError(t, err) // Should not error even if no rows affected
	})

	t.Run("workspace connection error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(nil, errors.New("connection error"))

		err := repo.DeleteForEmail(ctx, workspaceID, email)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to get workspace connection")
	})

	t.Run("execution error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		mock.ExpectExec(`UPDATE message_history SET contact_email = \$1 WHERE contact_email = \$2`).
			WithArgs("DELETED_EMAIL", email).
			WillReturnError(errors.New("execution error"))

		err := repo.DeleteForEmail(ctx, workspaceID, email)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to redact email in message history")
	})

	t.Run("rows affected error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		// Create a result that will error when RowsAffected is called
		result := sqlmock.NewErrorResult(errors.New("rows affected error"))
		mock.ExpectExec(`UPDATE message_history SET contact_email = \$1 WHERE contact_email = \$2`).
			WithArgs("DELETED_EMAIL", email).
			WillReturnResult(result)

		err := repo.DeleteForEmail(ctx, workspaceID, email)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to get affected rows")
	})
}

// Helper function to create string pointers
func stringPtr(s string) *string {
	return &s
}
