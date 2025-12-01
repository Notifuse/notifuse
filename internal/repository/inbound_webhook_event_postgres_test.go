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

func TestInboundWebhookEventRepository_ListEvents(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewInboundWebhookEventRepository(mockWorkspaceRepo)

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspaceID := "ws-123"
	now := time.Now().UTC()

	// Set up the workspace connection expectation
	mockWorkspaceRepo.EXPECT().
		GetConnection(gomock.Any(), workspaceID).
		Return(db, nil)

	// Test with various filter parameters
	params := domain.InboundWebhookEventListParams{
		Limit:          10,
		EventType:      domain.EmailEventBounce,
		RecipientEmail: "test@example.com",
		MessageID:      "msg-123",
	}

	// Set up rows for the SQL query result
	rows := sqlmock.NewRows([]string{
		"id", "type", "source", "integration_id", "recipient_email",
		"message_id", "timestamp", "raw_payload",
		"bounce_type", "bounce_category", "bounce_diagnostic", "complaint_feedback_type",
		"created_at",
	}).
		AddRow(
			"evt-1", domain.EmailEventBounce, domain.WebhookSourceSES, "integration-1", "test@example.com",
			"msg-1", now, `{"key": "value1"}`,
			"hard", "unknown", "550 user unknown", "", now,
		).
		AddRow(
			"evt-2", domain.EmailEventBounce, domain.WebhookSourceSES, "integration-2", "test@example.com",
			"msg-2", now, `{"key": "value2"}`,
			"soft", "mailbox_full", "452 mailbox full", "", now,
		)

	// Expect a SQL query with filters
	mock.ExpectQuery(`SELECT .+ FROM inbound_webhook_events WHERE`).
		WillReturnRows(rows)

	// Call the method
	result, err := repo.ListEvents(ctx, workspaceID, params)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result.Events, 2)
	assert.Equal(t, "evt-1", result.Events[0].ID)
	assert.Equal(t, "evt-2", result.Events[1].ID)
	assert.Equal(t, "hard", result.Events[0].BounceType)
	assert.Equal(t, "soft", result.Events[1].BounceType)

	// Verify all expectations were met
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestInboundWebhookEventRepository_ListEvents_WithCursor(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewInboundWebhookEventRepository(mockWorkspaceRepo)

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspaceID := "ws-123"
	now := time.Now().UTC()

	// Create a valid cursor (base64 encoded "timestamp~id")
	cursor := "MjAyMy0xMS0xMFQxMjozNDo1NiswMDowMH5ldnQtcHJldmlvdXM=" // Example base64 encoded cursor

	// Set up the workspace connection expectation
	mockWorkspaceRepo.EXPECT().
		GetConnection(gomock.Any(), workspaceID).
		Return(db, nil)

	// Test with cursor parameter
	params := domain.InboundWebhookEventListParams{
		Limit:  10,
		Cursor: cursor,
	}

	// Set up rows for the SQL query result
	rows := sqlmock.NewRows([]string{
		"id", "type", "source", "integration_id", "recipient_email",
		"message_id", "timestamp", "raw_payload",
		"bounce_type", "bounce_category", "bounce_diagnostic", "complaint_feedback_type",
		"created_at",
	}).
		AddRow(
			"evt-3", domain.EmailEventDelivered, domain.WebhookSourceSES, "integration-3", "test3@example.com",
			"msg-3", now, `{"key": "value3"}`,
			"", "", "", "", now,
		).
		AddRow(
			"evt-4", domain.EmailEventDelivered, domain.WebhookSourceSES, "integration-4", "test4@example.com",
			"msg-4", now, `{"key": "value4"}`,
			"", "", "", "", now,
		).
		AddRow(
			"evt-5", domain.EmailEventDelivered, domain.WebhookSourceSES, "integration-5", "test5@example.com",
			"msg-5", now, `{"key": "value5"}`,
			"", "", "", "", now,
		)

	// Expect a SQL query with cursor condition
	mock.ExpectQuery(`SELECT .+ FROM inbound_webhook_events WHERE`).
		WillReturnRows(rows)

	// Call the method
	result, err := repo.ListEvents(ctx, workspaceID, params)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result.Events, 3)
	assert.False(t, result.HasMore) // With 3 results and limit 10, HasMore should be false

	// Verify all expectations were met
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestInboundWebhookEventRepository_ListEvents_InvalidCursor(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewInboundWebhookEventRepository(mockWorkspaceRepo)

	ctx := context.Background()
	workspaceID := "ws-123"

	// Test cases for invalid cursors
	testCases := []struct {
		name   string
		cursor string
	}{
		{
			name:   "Invalid base64",
			cursor: "not-base64!",
		},
		{
			name:   "Invalid format",
			cursor: "aW52YWxpZC1mb3JtYXQ=", // base64 of "invalid-format"
		},
		{
			name:   "Invalid timestamp",
			cursor: "bm90LWEtdGltZXN0YW1wfmV2dC0xMjM=", // base64 of "not-a-timestamp~evt-123"
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			params := domain.InboundWebhookEventListParams{
				Limit:  10,
				Cursor: tc.cursor,
			}

			// Set up the workspace connection expectation for each test case
			mockWorkspaceRepo.EXPECT().
				GetConnection(gomock.Any(), workspaceID).
				Return(nil, errors.New("connection error"))

			_, err := repo.ListEvents(ctx, workspaceID, params)
			assert.Error(t, err)
		})
	}
}

func TestInboundWebhookEventRepository_ListEvents_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewInboundWebhookEventRepository(mockWorkspaceRepo)

	ctx := context.Background()
	workspaceID := "ws-123"
	params := domain.InboundWebhookEventListParams{Limit: 10}

	// Test case 1: Database connection error
	mockWorkspaceRepo.EXPECT().
		GetConnection(gomock.Any(), workspaceID).
		Return(nil, errors.New("connection error"))

	result, err := repo.ListEvents(ctx, workspaceID, params)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get workspace connection")
	assert.Nil(t, result)

	// Test case 2: SQL execution error
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mockWorkspaceRepo.EXPECT().
		GetConnection(gomock.Any(), workspaceID).
		Return(db, nil)

	mock.ExpectQuery("SELECT").
		WillReturnError(errors.New("database error"))

	result, err = repo.ListEvents(ctx, workspaceID, params)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to query inbound webhook events")
	assert.Nil(t, result)

	// Test case 3: Scan error
	mockWorkspaceRepo.EXPECT().
		GetConnection(gomock.Any(), workspaceID).
		Return(db, nil)

	rows := sqlmock.NewRows([]string{"id"}). // Deliberately wrong number of columns
							AddRow("evt-1")

	mock.ExpectQuery("SELECT").
		WillReturnRows(rows)

	result, err = repo.ListEvents(ctx, workspaceID, params)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to scan inbound webhook event row")
	assert.Nil(t, result)
}

func TestInboundWebhookEventRepository_StoreEvents(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewInboundWebhookEventRepository(mockWorkspaceRepo)

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspaceID := "ws-123"
	now := time.Now().UTC()

	// Create a batch of events
	msg1 := "msg-1"
	msg2 := "msg-2"
	events := []*domain.InboundWebhookEvent{
		{
			ID:                    "evt-1",
			Type:                  domain.EmailEventBounce,
			Source:                domain.WebhookSourceSES,
			IntegrationID:         "integration-1",
			RecipientEmail:        "test1@example.com",
			MessageID:             &msg1,
			Timestamp:             now,
			RawPayload:            `{"key": "value1"}`,
			BounceType:            "hard",
			BounceCategory:        "unknown",
			BounceDiagnostic:      "550 user unknown",
			ComplaintFeedbackType: "",
		},
		{
			ID:                    "evt-2",
			Type:                  domain.EmailEventDelivered,
			Source:                domain.WebhookSourceSES,
			IntegrationID:         "integration-1",
			RecipientEmail:        "test2@example.com",
			MessageID:             &msg2,
			Timestamp:             now,
			RawPayload:            `{"key": "value2"}`,
			BounceType:            "",
			BounceCategory:        "",
			BounceDiagnostic:      "",
			ComplaintFeedbackType: "",
		},
	}

	t.Run("Success - Multiple Events", func(t *testing.T) {
		// Set up the workspace connection expectation
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		// We're expecting a multi-value INSERT statement
		mock.ExpectExec("INSERT INTO inbound_webhook_events").
			WithArgs(
				events[0].ID, events[0].Type, events[0].Source, events[0].IntegrationID, events[0].RecipientEmail,
				events[0].MessageID, events[0].Timestamp, events[0].RawPayload,
				events[0].BounceType, events[0].BounceCategory, events[0].BounceDiagnostic, events[0].ComplaintFeedbackType, sqlmock.AnyArg(),
				events[1].ID, events[1].Type, events[1].Source, events[1].IntegrationID, events[1].RecipientEmail,
				events[1].MessageID, events[1].Timestamp, events[1].RawPayload,
				events[1].BounceType, events[1].BounceCategory, events[1].BounceDiagnostic, events[1].ComplaintFeedbackType, sqlmock.AnyArg(),
			).
			WillReturnResult(sqlmock.NewResult(0, 2))

		// Call the method
		err := repo.StoreEvents(ctx, workspaceID, events)
		assert.NoError(t, err)

		// Verify all expectations were met
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Success - Single Event", func(t *testing.T) {
		// Create a new mock setup
		db, mock, err = sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		// Set up the workspace connection expectation
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		// We're expecting a single-value INSERT statement
		mock.ExpectExec("INSERT INTO inbound_webhook_events").
			WithArgs(
				events[0].ID, events[0].Type, events[0].Source, events[0].IntegrationID, events[0].RecipientEmail,
				events[0].MessageID, events[0].Timestamp, events[0].RawPayload,
				events[0].BounceType, events[0].BounceCategory, events[0].BounceDiagnostic, events[0].ComplaintFeedbackType, sqlmock.AnyArg(),
			).
			WillReturnResult(sqlmock.NewResult(0, 1))

		// Call the method with a single event
		err := repo.StoreEvents(ctx, workspaceID, events[:1])
		assert.NoError(t, err)

		// Verify all expectations were met
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Success - Empty Events", func(t *testing.T) {
		// No database calls should occur for empty events slice
		err := repo.StoreEvents(ctx, workspaceID, []*domain.InboundWebhookEvent{})
		assert.NoError(t, err)
	})

	t.Run("Error - Connection Failure", func(t *testing.T) {
		// Set up the workspace connection to return an error
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(nil, errors.New("connection error"))

		err := repo.StoreEvents(ctx, workspaceID, events)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get workspace connection")
	})

	t.Run("Error - SQL Execution Failure", func(t *testing.T) {
		// Create a new mock setup
		db, mock, err = sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		// Set up the workspace connection expectation
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		// Expect the SQL query but return an error
		mock.ExpectExec("INSERT INTO inbound_webhook_events").
			WithArgs(
				events[0].ID, events[0].Type, events[0].Source, events[0].IntegrationID, events[0].RecipientEmail,
				events[0].MessageID, events[0].Timestamp, events[0].RawPayload,
				events[0].BounceType, events[0].BounceCategory, events[0].BounceDiagnostic, events[0].ComplaintFeedbackType, sqlmock.AnyArg(),
				events[1].ID, events[1].Type, events[1].Source, events[1].IntegrationID, events[1].RecipientEmail,
				events[1].MessageID, events[1].Timestamp, events[1].RawPayload,
				events[1].BounceType, events[1].BounceCategory, events[1].BounceDiagnostic, events[1].ComplaintFeedbackType, sqlmock.AnyArg(),
			).
			WillReturnError(errors.New("database error"))

		err := repo.StoreEvents(ctx, workspaceID, events)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to store inbound webhook events batch")
	})
}

// Test StoreEvent separately - the implementation that calls StoreEvents
func TestInboundWebhookEventRepository_StoreEvent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewInboundWebhookEventRepository(mockWorkspaceRepo).(*inboundWebhookEventRepository)

	ctx := context.Background()
	workspaceID := "ws-123"
	now := time.Now().UTC()

	// Create a single event
	msgID := "msg-1"
	event := &domain.InboundWebhookEvent{
		ID:                    "evt-1",
		Type:                  domain.EmailEventBounce,
		Source:                domain.WebhookSourceSES,
		IntegrationID:         "integration-1",
		RecipientEmail:        "test1@example.com",
		MessageID:             &msgID,
		Timestamp:             now,
		RawPayload:            `{"key": "value1"}`,
		BounceType:            "hard",
		BounceCategory:        "unknown",
		BounceDiagnostic:      "550 user unknown",
		ComplaintFeedbackType: "",
	}

	t.Run("successful store", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		// Set up the workspace connection expectation
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		// We're expecting a single-value INSERT statement
		mock.ExpectExec("INSERT INTO inbound_webhook_events").
			WithArgs(
				event.ID, event.Type, event.Source, event.IntegrationID, event.RecipientEmail,
				event.MessageID, event.Timestamp, event.RawPayload,
				event.BounceType, event.BounceCategory, event.BounceDiagnostic, event.ComplaintFeedbackType, sqlmock.AnyArg(),
			).
			WillReturnResult(sqlmock.NewResult(0, 1))

		// Call the StoreEvent method
		err = repo.StoreEvent(ctx, workspaceID, event)
		assert.NoError(t, err)

		// Verify all expectations were met
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("connection error", func(t *testing.T) {
		// Set up the workspace connection to return an error
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(nil, errors.New("connection error"))

		err := repo.StoreEvent(ctx, workspaceID, event)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get workspace connection")
	})

	t.Run("execution error", func(t *testing.T) {
		// Create a new mock setup
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		// Set up the workspace connection expectation
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		// Expect the SQL query but return an error
		mock.ExpectExec("INSERT INTO inbound_webhook_events").
			WithArgs(
				event.ID, event.Type, event.Source, event.IntegrationID, event.RecipientEmail,
				event.MessageID, event.Timestamp, event.RawPayload,
				event.BounceType, event.BounceCategory, event.BounceDiagnostic, event.ComplaintFeedbackType, sqlmock.AnyArg(),
			).
			WillReturnError(errors.New("database error"))

		err = repo.StoreEvent(ctx, workspaceID, event)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to store inbound webhook events batch")
	})
}

func TestInboundWebhookEventRepository_DeleteForEmail(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewInboundWebhookEventRepository(mockWorkspaceRepo)

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	workspaceID := "ws-123"
	email := "test@example.com"
	redactedEmail := "DELETED_EMAIL"

	t.Run("successful redaction", func(t *testing.T) {
		// Set up the workspace connection expectation
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		mock.ExpectExec(`UPDATE inbound_webhook_events SET recipient_email = \$1 WHERE recipient_email = \$2`).
			WithArgs(redactedEmail, email).
			WillReturnResult(sqlmock.NewResult(0, 3))

		err := repo.DeleteForEmail(ctx, workspaceID, email)
		assert.NoError(t, err)

		// Verify all expectations were met
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("successful redaction with no rows affected", func(t *testing.T) {
		// Create a new mock setup
		db2, mock2, err2 := sqlmock.New()
		require.NoError(t, err2)
		defer func() { _ = db2.Close() }()

		// Set up the workspace connection expectation
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db2, nil)

		mock2.ExpectExec(`UPDATE inbound_webhook_events SET recipient_email = \$1 WHERE recipient_email = \$2`).
			WithArgs(redactedEmail, email).
			WillReturnResult(sqlmock.NewResult(0, 0))

		err := repo.DeleteForEmail(ctx, workspaceID, email)
		assert.NoError(t, err)

		// Verify all expectations were met
		assert.NoError(t, mock2.ExpectationsWereMet())
	})

	t.Run("workspace connection error", func(t *testing.T) {
		// Set up the workspace connection to return an error
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(nil, errors.New("connection error"))

		err := repo.DeleteForEmail(ctx, workspaceID, email)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get workspace connection")
	})

	t.Run("execution error", func(t *testing.T) {
		// Create a new mock setup
		db3, mock3, err3 := sqlmock.New()
		require.NoError(t, err3)
		defer func() { _ = db3.Close() }()

		// Set up the workspace connection expectation
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db3, nil)

		mock3.ExpectExec(`UPDATE inbound_webhook_events SET recipient_email = \$1 WHERE recipient_email = \$2`).
			WithArgs(redactedEmail, email).
			WillReturnError(errors.New("execution error"))

		err := repo.DeleteForEmail(ctx, workspaceID, email)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to redact email in inbound webhook events")
	})

	t.Run("rows affected error", func(t *testing.T) {
		// Create a new mock setup
		db4, mock4, err4 := sqlmock.New()
		require.NoError(t, err4)
		defer func() { _ = db4.Close() }()

		// Set up the workspace connection expectation
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db4, nil)

		mock4.ExpectExec(`UPDATE inbound_webhook_events SET recipient_email = \$1 WHERE recipient_email = \$2`).
			WithArgs(redactedEmail, email).
			WillReturnResult(sqlmock.NewErrorResult(errors.New("rows affected error")))

		err := repo.DeleteForEmail(ctx, workspaceID, email)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get affected rows")
	})
}
