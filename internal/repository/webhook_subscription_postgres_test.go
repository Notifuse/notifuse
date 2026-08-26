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

func TestWebhookSubscriptionRepository_Create(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewWebhookSubscriptionRepository(mockWorkspaceRepo)

	ctx := context.Background()
	workspaceID := "ws-123"

	customFilters := &domain.CustomEventFilters{
		GoalTypes:  []string{"goal1", "goal2"},
		EventNames: []string{"event1", "event2"},
	}

	testCases := []struct {
		name          string
		subscription  *domain.WebhookSubscription
		setupMock     func(*sqlmock.Sqlmock)
		expectedError string
	}{
		{
			name: "Success - with custom event filters",
			subscription: &domain.WebhookSubscription{
				ID:     "sub-1",
				Name:   "Test Subscription",
				URL:    "https://example.com/webhook",
				Secret: "secret-key",
				Settings: domain.WebhookSubscriptionSettings{
					EventTypes:         []string{"email.delivered", "email.bounced"},
					CustomEventFilters: customFilters,
				},
				Enabled: true,
			},
			setupMock: func(mock *sqlmock.Sqlmock) {
				(*mock).ExpectExec(`INSERT INTO webhook_subscriptions`).
					WithArgs(
						"sub-1",
						"Test Subscription",
						"https://example.com/webhook",
						"secret-key",
						sqlmock.AnyArg(), // settings JSON
						true,
						nil,              // source: hand-made subscriptions store NULL, not ''
						0,                // consecutive_failures
						nil,              // disabled_reason
						sqlmock.AnyArg(), // created_at
						sqlmock.AnyArg(), // updated_at
					).
					WillReturnResult(sqlmock.NewResult(1, 1))
			},
			expectedError: "",
		},
		{
			name: "Success - without custom event filters",
			subscription: &domain.WebhookSubscription{
				ID:     "sub-2",
				Name:   "Simple Subscription",
				URL:    "https://example.com/webhook2",
				Secret: "secret-key-2",
				Settings: domain.WebhookSubscriptionSettings{
					EventTypes:         []string{"email.delivered"},
					CustomEventFilters: nil,
				},
				Enabled: false,
			},
			setupMock: func(mock *sqlmock.Sqlmock) {
				(*mock).ExpectExec(`INSERT INTO webhook_subscriptions`).
					WithArgs(
						"sub-2",
						"Simple Subscription",
						"https://example.com/webhook2",
						"secret-key-2",
						sqlmock.AnyArg(), // settings JSON
						false,
						nil, // source
						0,   // consecutive_failures
						nil, // disabled_reason
						sqlmock.AnyArg(),
						sqlmock.AnyArg(),
					).
					WillReturnResult(sqlmock.NewResult(1, 1))
			},
			expectedError: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer func() { _ = db.Close() }()

			mockWorkspaceRepo.EXPECT().
				GetConnection(gomock.Any(), workspaceID).
				Return(db, nil)

			tc.setupMock(&mock)

			err = repo.Create(ctx, workspaceID, tc.subscription)

			if tc.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedError)
			} else {
				assert.NoError(t, err)
				// Verify that timestamps were set
				assert.False(t, tc.subscription.CreatedAt.IsZero())
				assert.False(t, tc.subscription.UpdatedAt.IsZero())
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestWebhookSubscriptionRepository_Create_Errors(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewWebhookSubscriptionRepository(mockWorkspaceRepo)

	ctx := context.Background()
	workspaceID := "ws-123"

	sub := &domain.WebhookSubscription{
		ID:     "sub-1",
		Name:   "Test",
		URL:    "https://example.com",
		Secret: "secret",
		Settings: domain.WebhookSubscriptionSettings{
			EventTypes: []string{"email.delivered"},
		},
		Enabled: true,
	}

	t.Run("Workspace connection error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(nil, errors.New("connection error"))

		err := repo.Create(ctx, workspaceID, sub)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get workspace connection")
	})

	t.Run("SQL execution error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		mock.ExpectExec(`INSERT INTO webhook_subscriptions`).
			WillReturnError(errors.New("database error"))

		err = repo.Create(ctx, workspaceID, sub)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create webhook subscription")
	})
}

func TestWebhookSubscriptionRepository_GetByID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewWebhookSubscriptionRepository(mockWorkspaceRepo)

	ctx := context.Background()
	workspaceID := "ws-123"
	subscriptionID := "sub-1"

	now := time.Now().UTC()
	lastDelivery := now.Add(-1 * time.Hour)
	failingSince := now.Add(-13 * time.Hour)

	settings := domain.WebhookSubscriptionSettings{
		EventTypes: []string{"email.delivered", "email.bounced"},
		CustomEventFilters: &domain.CustomEventFilters{
			GoalTypes:  []string{"goal1"},
			EventNames: []string{"event1"},
		},
	}
	settingsJSON, _ := json.Marshal(settings)

	t.Run("Success - with custom filters and last delivery", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		rows := sqlmock.NewRows([]string{
			"id", "name", "url", "secret", "settings",
			"enabled", "source", "consecutive_failures", "disabled_reason",
			"created_at", "updated_at", "last_delivery_at", "failing_since",
		}).AddRow(
			subscriptionID,
			"Test Subscription",
			"https://example.com/webhook",
			"secret-key",
			settingsJSON,
			true,
			domain.WebhookSubscriptionSourceZapier,
			3,
			"endpoint returned 410 Gone",
			now,
			now,
			lastDelivery,
			failingSince,
		)

		mock.ExpectQuery(`SELECT .+ FROM webhook_subscriptions WHERE id = \$1`).
			WithArgs(subscriptionID).
			WillReturnRows(rows)

		result, err := repo.GetByID(ctx, workspaceID, subscriptionID)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, subscriptionID, result.ID)
		assert.Equal(t, "Test Subscription", result.Name)
		assert.Equal(t, "https://example.com/webhook", result.URL)
		assert.Equal(t, "secret-key", result.Secret)
		assert.Equal(t, []string{"email.delivered", "email.bounced"}, result.Settings.EventTypes)
		assert.NotNil(t, result.Settings.CustomEventFilters)
		assert.Equal(t, []string{"goal1"}, result.Settings.CustomEventFilters.GoalTypes)
		assert.Equal(t, []string{"event1"}, result.Settings.CustomEventFilters.EventNames)
		assert.True(t, result.Enabled)
		assert.Equal(t, domain.WebhookSubscriptionSourceZapier, result.Source)
		assert.Equal(t, 3, result.ConsecutiveFailures)
		require.NotNil(t, result.DisabledReason)
		assert.Equal(t, "endpoint returned 410 Gone", *result.DisabledReason)
		assert.NotNil(t, result.LastDeliveryAt)
		assert.Equal(t, lastDelivery.Unix(), result.LastDeliveryAt.Unix())

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Success - without custom filters and last delivery", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		simpleSettings := domain.WebhookSubscriptionSettings{
			EventTypes: []string{"email.delivered"},
		}
		simpleSettingsJSON, _ := json.Marshal(simpleSettings)

		rows := sqlmock.NewRows([]string{
			"id", "name", "url", "secret", "settings",
			"enabled", "source", "consecutive_failures", "disabled_reason",
			"created_at", "updated_at", "last_delivery_at", "failing_since",
		}).AddRow(
			subscriptionID,
			"Simple Subscription",
			"https://example.com/webhook",
			"secret-key",
			simpleSettingsJSON,
			false,
			nil, // source: created by hand
			0,
			nil, // never auto-disabled
			now,
			now,
			nil, // no last delivery
			nil, // not currently failing
		)

		mock.ExpectQuery(`SELECT .+ FROM webhook_subscriptions WHERE id = \$1`).
			WithArgs(subscriptionID).
			WillReturnRows(rows)

		result, err := repo.GetByID(ctx, workspaceID, subscriptionID)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Nil(t, result.Settings.CustomEventFilters)
		assert.Nil(t, result.LastDeliveryAt)
		// A NULL source reads back as the user-created value, not as a
		// separate third state callers would have to handle.
		assert.Equal(t, domain.WebhookSubscriptionSourceUser, result.Source)
		assert.Nil(t, result.DisabledReason)

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Not found", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		mock.ExpectQuery(`SELECT .+ FROM webhook_subscriptions WHERE id = \$1`).
			WithArgs(subscriptionID).
			WillReturnError(sql.ErrNoRows)

		result, err := repo.GetByID(ctx, workspaceID, subscriptionID)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "webhook subscription not found")
	})

	t.Run("Workspace connection error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(nil, errors.New("connection error"))

		result, err := repo.GetByID(ctx, workspaceID, subscriptionID)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to get workspace connection")
	})

	t.Run("Invalid settings JSON", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		rows := sqlmock.NewRows([]string{
			"id", "name", "url", "secret", "settings",
			"enabled", "source", "consecutive_failures", "disabled_reason",
			"created_at", "updated_at", "last_delivery_at", "failing_since",
		}).AddRow(
			subscriptionID,
			"Test",
			"https://example.com",
			"secret",
			[]byte("{invalid json}"), // invalid JSON
			true,
			nil,
			0,
			nil,
			now,
			now,
			nil,
			nil,
		)

		mock.ExpectQuery(`SELECT .+ FROM webhook_subscriptions WHERE id = \$1`).
			WithArgs(subscriptionID).
			WillReturnRows(rows)

		result, err := repo.GetByID(ctx, workspaceID, subscriptionID)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to unmarshal settings")
	})
}

func TestWebhookSubscriptionRepository_List(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewWebhookSubscriptionRepository(mockWorkspaceRepo)

	ctx := context.Background()
	workspaceID := "ws-123"
	now := time.Now().UTC()

	settings1 := domain.WebhookSubscriptionSettings{
		EventTypes: []string{"email.delivered"},
		CustomEventFilters: &domain.CustomEventFilters{
			GoalTypes: []string{"goal1"},
		},
	}
	settings1JSON, _ := json.Marshal(settings1)

	settings2 := domain.WebhookSubscriptionSettings{
		EventTypes: []string{"email.bounced"},
	}
	settings2JSON, _ := json.Marshal(settings2)
	failingSince := now.Add(-13 * time.Hour)

	t.Run("Success - multiple subscriptions", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		rows := sqlmock.NewRows([]string{
			"id", "name", "url", "secret", "settings",
			"enabled", "source", "consecutive_failures", "disabled_reason",
			"created_at", "updated_at", "last_delivery_at", "failing_since",
		}).
			AddRow(
				"sub-1",
				"Subscription 1",
				"https://example.com/webhook1",
				"secret1",
				settings1JSON,
				true,
				domain.WebhookSubscriptionSourceZapier,
				0,
				nil,
				now,
				now,
				now,
				nil, // healthy: no run of failures in progress
			).
			AddRow(
				"sub-2",
				"Subscription 2",
				"https://example.com/webhook2",
				"secret2",
				settings2JSON,
				false,
				nil,
				7,
				"20 consecutive delivery failures",
				now,
				now,
				nil,
				failingSince,
			)

		mock.ExpectQuery(`SELECT .+ FROM webhook_subscriptions ORDER BY created_at DESC`).
			WillReturnRows(rows)

		result, err := repo.List(ctx, workspaceID)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Len(t, result, 2)

		// Verify first subscription
		assert.Equal(t, "sub-1", result[0].ID)
		assert.Equal(t, "Subscription 1", result[0].Name)
		assert.NotNil(t, result[0].Settings.CustomEventFilters)
		assert.NotNil(t, result[0].LastDeliveryAt)
		assert.Equal(t, domain.WebhookSubscriptionSourceZapier, result[0].Source)
		assert.Equal(t, 0, result[0].ConsecutiveFailures)
		assert.Nil(t, result[0].DisabledReason)

		// Verify second subscription
		assert.Equal(t, "sub-2", result[1].ID)
		assert.Equal(t, "Subscription 2", result[1].Name)
		assert.Nil(t, result[1].Settings.CustomEventFilters)
		assert.Nil(t, result[1].LastDeliveryAt)
		assert.Equal(t, domain.WebhookSubscriptionSourceUser, result[1].Source)
		assert.Equal(t, 7, result[1].ConsecutiveFailures)
		require.NotNil(t, result[1].DisabledReason)
		assert.Equal(t, "20 consecutive delivery failures", *result[1].DisabledReason)

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Success - empty list", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		rows := sqlmock.NewRows([]string{
			"id", "name", "url", "secret", "settings",
			"enabled", "source", "consecutive_failures", "disabled_reason",
			"created_at", "updated_at", "last_delivery_at", "failing_since",
		})

		mock.ExpectQuery(`SELECT .+ FROM webhook_subscriptions ORDER BY created_at DESC`).
			WillReturnRows(rows)

		result, err := repo.List(ctx, workspaceID)
		assert.NoError(t, err)
		// Empty list returns nil slice in Go when using var declaration
		assert.Nil(t, result)

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Workspace connection error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(nil, errors.New("connection error"))

		result, err := repo.List(ctx, workspaceID)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to get workspace connection")
	})

	t.Run("Query error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		mock.ExpectQuery(`SELECT .+ FROM webhook_subscriptions ORDER BY created_at DESC`).
			WillReturnError(errors.New("query error"))

		result, err := repo.List(ctx, workspaceID)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to list webhook subscriptions")
	})

	t.Run("Scan error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		// Wrong number of columns
		rows := sqlmock.NewRows([]string{"id"}).AddRow("sub-1")

		mock.ExpectQuery(`SELECT .+ FROM webhook_subscriptions ORDER BY created_at DESC`).
			WillReturnRows(rows)

		result, err := repo.List(ctx, workspaceID)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to scan webhook subscription")
	})

	t.Run("Rows iteration error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		rows := sqlmock.NewRows([]string{
			"id", "name", "url", "secret", "settings",
			"enabled", "source", "consecutive_failures", "disabled_reason",
			"created_at", "updated_at", "last_delivery_at", "failing_since",
		}).
			AddRow(
				"sub-1", "Test", "https://example.com", "secret",
				settings1JSON, true,
				nil, 0, nil,
				now, now, nil, nil,
			).
			RowError(0, errors.New("rows iteration error"))

		mock.ExpectQuery(`SELECT .+ FROM webhook_subscriptions ORDER BY created_at DESC`).
			WillReturnRows(rows)

		result, err := repo.List(ctx, workspaceID)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "error iterating webhook subscriptions")
	})
}

func TestWebhookSubscriptionRepository_Update(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewWebhookSubscriptionRepository(mockWorkspaceRepo)

	ctx := context.Background()
	workspaceID := "ws-123"

	customFilters := &domain.CustomEventFilters{
		GoalTypes:  []string{"goal1", "goal2"},
		EventNames: []string{"event1"},
	}

	now := time.Now().UTC()
	sub := &domain.WebhookSubscription{
		ID:     "sub-1",
		Name:   "Updated Subscription",
		URL:    "https://example.com/webhook-updated",
		Secret: "new-secret",
		Settings: domain.WebhookSubscriptionSettings{
			EventTypes:         []string{"email.delivered", "email.opened"},
			CustomEventFilters: customFilters,
		},
		Enabled:   false,
		CreatedAt: now.Add(-24 * time.Hour),
		UpdatedAt: now.Add(-1 * time.Hour),
	}

	t.Run("Success - with custom filters", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		mock.ExpectExec(`UPDATE webhook_subscriptions SET name = \$2, url = \$3, secret = \$4, settings = \$5, enabled = \$6, consecutive_failures = CASE WHEN \$6 AND NOT enabled THEN 0 ELSE consecutive_failures END, failing_since = CASE WHEN \$6 AND NOT enabled THEN NULL ELSE failing_since END, disabled_reason = CASE WHEN \$6 AND NOT enabled THEN NULL ELSE disabled_reason END, updated_at = \$7 WHERE id = \$1`).
			WithArgs(
				"sub-1",
				"Updated Subscription",
				"https://example.com/webhook-updated",
				"new-secret",
				sqlmock.AnyArg(), // settings JSON
				false,
				sqlmock.AnyArg(), // updated_at
			).
			WillReturnResult(sqlmock.NewResult(0, 1))

		err = repo.Update(ctx, workspaceID, sub)
		assert.NoError(t, err)
		// Verify updated_at was modified
		assert.True(t, sub.UpdatedAt.After(now.Add(-1*time.Hour)))

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Success - without custom filters", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		subNoFilters := &domain.WebhookSubscription{
			ID:     "sub-2",
			Name:   "Simple Update",
			URL:    "https://example.com/simple",
			Secret: "secret",
			Settings: domain.WebhookSubscriptionSettings{
				EventTypes:         []string{"email.delivered"},
				CustomEventFilters: nil,
			},
			Enabled: true,
		}

		mock.ExpectExec(`UPDATE webhook_subscriptions SET name = \$2, url = \$3, secret = \$4, settings = \$5, enabled = \$6, consecutive_failures = CASE WHEN \$6 AND NOT enabled THEN 0 ELSE consecutive_failures END, failing_since = CASE WHEN \$6 AND NOT enabled THEN NULL ELSE failing_since END, disabled_reason = CASE WHEN \$6 AND NOT enabled THEN NULL ELSE disabled_reason END, updated_at = \$7 WHERE id = \$1`).
			WithArgs(
				"sub-2",
				"Simple Update",
				"https://example.com/simple",
				"secret",
				sqlmock.AnyArg(), // settings JSON
				true,
				sqlmock.AnyArg(), // updated_at
			).
			WillReturnResult(sqlmock.NewResult(0, 1))

		err = repo.Update(ctx, workspaceID, subNoFilters)
		assert.NoError(t, err)

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Not found - zero rows affected", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		mock.ExpectExec(`UPDATE webhook_subscriptions`).
			WillReturnResult(sqlmock.NewResult(0, 0))

		err = repo.Update(ctx, workspaceID, sub)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "webhook subscription not found")
	})

	t.Run("Workspace connection error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(nil, errors.New("connection error"))

		err := repo.Update(ctx, workspaceID, sub)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get workspace connection")
	})

	t.Run("SQL execution error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		mock.ExpectExec(`UPDATE webhook_subscriptions`).
			WillReturnError(errors.New("database error"))

		err = repo.Update(ctx, workspaceID, sub)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to update webhook subscription")
	})

	t.Run("RowsAffected error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		mock.ExpectExec(`UPDATE webhook_subscriptions`).
			WillReturnResult(sqlmock.NewErrorResult(errors.New("rows affected error")))

		err = repo.Update(ctx, workspaceID, sub)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get rows affected")
	})
}

func TestWebhookSubscriptionRepository_Delete(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewWebhookSubscriptionRepository(mockWorkspaceRepo)

	ctx := context.Background()
	workspaceID := "ws-123"
	subscriptionID := "sub-1"

	t.Run("Success", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		mock.ExpectExec(`DELETE FROM webhook_subscriptions WHERE id = \$1`).
			WithArgs(subscriptionID).
			WillReturnResult(sqlmock.NewResult(0, 1))

		err = repo.Delete(ctx, workspaceID, subscriptionID)
		assert.NoError(t, err)

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Not found - zero rows affected", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		mock.ExpectExec(`DELETE FROM webhook_subscriptions WHERE id = \$1`).
			WithArgs(subscriptionID).
			WillReturnResult(sqlmock.NewResult(0, 0))

		err = repo.Delete(ctx, workspaceID, subscriptionID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "webhook subscription not found")
	})

	t.Run("Workspace connection error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(nil, errors.New("connection error"))

		err := repo.Delete(ctx, workspaceID, subscriptionID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get workspace connection")
	})

	t.Run("SQL execution error", func(t *testing.T) {
		db2, mock2, err2 := sqlmock.New()
		require.NoError(t, err2)
		defer func() { _ = db2.Close() }()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db2, nil)

		mock2.ExpectExec(`DELETE FROM webhook_subscriptions WHERE id = \$1`).
			WithArgs(subscriptionID).
			WillReturnError(errors.New("database error"))

		err := repo.Delete(ctx, workspaceID, subscriptionID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to delete webhook subscription")
	})

	t.Run("RowsAffected error", func(t *testing.T) {
		db3, mock3, err3 := sqlmock.New()
		require.NoError(t, err3)
		defer func() { _ = db3.Close() }()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db3, nil)

		mock3.ExpectExec(`DELETE FROM webhook_subscriptions WHERE id = \$1`).
			WithArgs(subscriptionID).
			WillReturnResult(sqlmock.NewErrorResult(errors.New("rows affected error")))

		err := repo.Delete(ctx, workspaceID, subscriptionID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get rows affected")
	})
}

func TestWebhookSubscriptionRepository_UpdateLastDeliveryAt(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewWebhookSubscriptionRepository(mockWorkspaceRepo)

	ctx := context.Background()
	workspaceID := "ws-123"
	subscriptionID := "sub-1"
	deliveryTime := time.Now().UTC()

	t.Run("Success", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		mock.ExpectExec(`UPDATE webhook_subscriptions SET last_delivery_at = \$2 WHERE id = \$1`).
			WithArgs(subscriptionID, deliveryTime).
			WillReturnResult(sqlmock.NewResult(0, 1))

		err = repo.UpdateLastDeliveryAt(ctx, workspaceID, subscriptionID, deliveryTime)
		assert.NoError(t, err)

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Workspace connection error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(nil, errors.New("connection error"))

		err := repo.UpdateLastDeliveryAt(ctx, workspaceID, subscriptionID, deliveryTime)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get workspace connection")
	})

	t.Run("SQL execution error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		mock.ExpectExec(`UPDATE webhook_subscriptions SET last_delivery_at = \$2 WHERE id = \$1`).
			WithArgs(subscriptionID, deliveryTime).
			WillReturnError(errors.New("database error"))

		err = repo.UpdateLastDeliveryAt(ctx, workspaceID, subscriptionID, deliveryTime)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to update last delivery timestamp")
	})

	t.Run("Success - zero rows affected is OK", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		// Note: The implementation doesn't check rows affected for this method
		mock.ExpectExec(`UPDATE webhook_subscriptions SET last_delivery_at = \$2 WHERE id = \$1`).
			WithArgs(subscriptionID, deliveryTime).
			WillReturnResult(sqlmock.NewResult(0, 0))

		err = repo.UpdateLastDeliveryAt(ctx, workspaceID, subscriptionID, deliveryTime)
		assert.NoError(t, err)

		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

// TestWebhookSubscriptionRepository_Source covers the attribution column end to
// end: it survives a Create, comes back from both read paths, and an Update
// cannot change it. The last of those is the one that matters — the console
// badge, the deletion guard and the delete-versus-disable branch on a dead
// endpoint all read this column, so a subscription that could be relabelled by
// an ordinary edit would be attributed to whoever edited it last.
func TestWebhookSubscriptionRepository_Source(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewWebhookSubscriptionRepository(mockWorkspaceRepo)

	ctx := context.Background()
	workspaceID := "ws-123"

	t.Run("Create persists an integration source", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		mock.ExpectExec(`INSERT INTO webhook_subscriptions`).
			WithArgs(
				"sub-zap",
				"Zap subscription",
				"https://hooks.zapier.com/hooks/standard/1/abc/",
				"secret",
				sqlmock.AnyArg(), // settings JSON
				true,
				domain.WebhookSubscriptionSourceZapier,
				0,
				nil,
				sqlmock.AnyArg(),
				sqlmock.AnyArg(),
			).
			WillReturnResult(sqlmock.NewResult(1, 1))

		err = repo.Create(ctx, workspaceID, &domain.WebhookSubscription{
			ID:      "sub-zap",
			Name:    "Zap subscription",
			URL:     "https://hooks.zapier.com/hooks/standard/1/abc/",
			Secret:  "secret",
			Enabled: true,
			Source:  domain.WebhookSubscriptionSourceZapier,
		})
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Create stores a hand-made subscription as NULL", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		// Not '': one spelling of "nobody created this on the user's behalf"
		// keeps every query that filters on the column honest.
		mock.ExpectExec(`INSERT INTO webhook_subscriptions`).
			WithArgs(
				"sub-hand",
				"Hand made",
				"https://example.com/webhook",
				"secret",
				sqlmock.AnyArg(),
				true,
				nil,
				0,
				nil,
				sqlmock.AnyArg(),
				sqlmock.AnyArg(),
			).
			WillReturnResult(sqlmock.NewResult(1, 1))

		err = repo.Create(ctx, workspaceID, &domain.WebhookSubscription{
			ID:      "sub-hand",
			Name:    "Hand made",
			URL:     "https://example.com/webhook",
			Secret:  "secret",
			Enabled: true,
			Source:  domain.WebhookSubscriptionSourceUser,
		})
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Update never writes the source column", func(t *testing.T) {
		var actualSQL string
		db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(captureWebhookSQL(&actualSQL)))
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		// The argument list is pinned as well as the statement text: sqlmock
		// rejects a call whose argument count differs, so a source smuggled in
		// as an extra parameter would fail here too.
		mock.ExpectExec(`UPDATE webhook_subscriptions SET`).
			WithArgs(
				"sub-zap",
				"Renamed by the user",
				"https://example.com/webhook",
				"secret",
				sqlmock.AnyArg(),
				true,
				sqlmock.AnyArg(),
			).
			WillReturnResult(sqlmock.NewResult(0, 1))

		sub := &domain.WebhookSubscription{
			ID:      "sub-zap",
			Name:    "Renamed by the user",
			URL:     "https://example.com/webhook",
			Secret:  "secret",
			Enabled: true,
			Source:  domain.WebhookSubscriptionSourceZapier,
		}

		err = repo.Update(ctx, workspaceID, sub)
		require.NoError(t, err)
		assert.NotContains(t, actualSQL, "source",
			"Update must leave the attribution column exactly as Create wrote it")
		assert.Equal(t, domain.WebhookSubscriptionSourceZapier, sub.Source)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

// TestWebhookSubscriptionRepository_GetByID_NotFoundIsTyped pins the one
// distinction the delivery worker cannot make for itself: a subscription that
// is genuinely gone versus a lookup that failed. It destroys the queued
// delivery in the first case and retries in the second, so a repository that
// reported both the same way would turn a momentary database failure into
// thousands of permanently dead deliveries.
func TestWebhookSubscriptionRepository_GetByID_NotFoundIsTyped(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewWebhookSubscriptionRepository(mockWorkspaceRepo)

	ctx := context.Background()
	workspaceID := "ws-123"
	subscriptionID := "sub-1"
	now := time.Now().UTC()

	t.Run("Missing row satisfies errors.Is", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		mock.ExpectQuery(`SELECT .+ FROM webhook_subscriptions WHERE id = \$1`).
			WithArgs(subscriptionID).
			WillReturnError(sql.ErrNoRows)

		result, err := repo.GetByID(ctx, workspaceID, subscriptionID)
		assert.Nil(t, result)
		require.Error(t, err)
		assert.True(t, errors.Is(err, domain.ErrWebhookSubscriptionNotFound))
		assert.Contains(t, err.Error(), subscriptionID, "the id belongs in the message for the logs")
	})

	t.Run("Connection failure is not a missing row", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		// The shape of a Postgres restart or an exhausted pool, which must stay
		// retryable.
		mock.ExpectQuery(`SELECT .+ FROM webhook_subscriptions WHERE id = \$1`).
			WithArgs(subscriptionID).
			WillReturnError(errors.New("driver: bad connection"))

		result, err := repo.GetByID(ctx, workspaceID, subscriptionID)
		assert.Nil(t, result)
		require.Error(t, err)
		assert.False(t, errors.Is(err, domain.ErrWebhookSubscriptionNotFound))
	})

	t.Run("Scan failure is not a missing row", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		// A row that comes back with the wrong shape is a bug in this file, not
		// evidence that the subscription was deleted.
		rows := sqlmock.NewRows([]string{"id", "name"}).AddRow(subscriptionID, "Test")

		mock.ExpectQuery(`SELECT .+ FROM webhook_subscriptions WHERE id = \$1`).
			WithArgs(subscriptionID).
			WillReturnRows(rows)

		result, err := repo.GetByID(ctx, workspaceID, subscriptionID)
		assert.Nil(t, result)
		require.Error(t, err)
		assert.False(t, errors.Is(err, domain.ErrWebhookSubscriptionNotFound))
		assert.Contains(t, err.Error(), "failed to scan webhook subscription")
	})

	t.Run("Unreadable settings is not a missing row", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		rows := sqlmock.NewRows([]string{
			"id", "name", "url", "secret", "settings",
			"enabled", "source", "consecutive_failures", "disabled_reason",
			"created_at", "updated_at", "last_delivery_at", "failing_since",
		}).AddRow(
			subscriptionID, "Test", "https://example.com", "secret",
			[]byte("{invalid json}"), true,
			nil, 0, nil,
			now, now, nil, nil,
		)

		mock.ExpectQuery(`SELECT .+ FROM webhook_subscriptions WHERE id = \$1`).
			WithArgs(subscriptionID).
			WillReturnRows(rows)

		result, err := repo.GetByID(ctx, workspaceID, subscriptionID)
		assert.Nil(t, result)
		require.Error(t, err)
		assert.False(t, errors.Is(err, domain.ErrWebhookSubscriptionNotFound))
		assert.Contains(t, err.Error(), "failed to unmarshal settings")
	})
}

// TestWebhookSubscriptionRepository_FailureCounter covers the three writes the
// delivery worker uses to decide a subscription is dead.
func TestWebhookSubscriptionRepository_FailureCounter(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewWebhookSubscriptionRepository(mockWorkspaceRepo)

	ctx := context.Background()
	workspaceID := "ws-123"
	subscriptionID := "sub-1"

	t.Run("IncrementFailures counts in SQL, not in Go", func(t *testing.T) {
		var actualSQL string
		db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(captureWebhookSQL(&actualSQL)))
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		mock.ExpectExec(`UPDATE webhook_subscriptions SET consecutive_failures = consecutive_failures \+ 1, failing_since = COALESCE\(failing_since, NOW\(\)\) WHERE id = \$1$`).
			WithArgs(subscriptionID).
			WillReturnResult(sqlmock.NewResult(0, 1))

		err = repo.IncrementFailures(ctx, workspaceID, subscriptionID)
		assert.NoError(t, err)
		// Deliveries for one subscription fail concurrently; a count read into
		// Go and written back would lose every increment but the last.
		assert.Contains(t, normalizeWebhookSQL(actualSQL), "consecutive_failures = consecutive_failures + 1")
		// COALESCE, never a bare NOW(): failing_since marks the START of the run
		// of failures, so only the first failure after a success may set it.
		// Re-stamping it on every failure would keep the window it measures
		// permanently at zero and give the threshold nothing to wait for.
		assert.Contains(t, normalizeWebhookSQL(actualSQL), "failing_since = COALESCE(failing_since, NOW())")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("IncrementFailures - connection error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(nil, errors.New("connection error"))

		err := repo.IncrementFailures(ctx, workspaceID, subscriptionID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get workspace connection")
	})

	t.Run("IncrementFailures - exec error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		mock.ExpectExec(`UPDATE webhook_subscriptions SET consecutive_failures`).
			WillReturnError(errors.New("database error"))

		err = repo.IncrementFailures(ctx, workspaceID, subscriptionID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to increment webhook subscription failures")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("ResetFailures skips subscriptions already at zero", func(t *testing.T) {
		var actualSQL string
		db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(captureWebhookSQL(&actualSQL)))
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		mock.ExpectExec(`UPDATE webhook_subscriptions SET consecutive_failures = 0, failing_since = NULL WHERE id = \$1`).
			WithArgs(subscriptionID).
			WillReturnResult(sqlmock.NewResult(0, 0))

		err = repo.ResetFailures(ctx, workspaceID, subscriptionID)
		assert.NoError(t, err)
		// A healthy subscription is already at zero on every delivery, so
		// without this guard each success writes a new row version to store the
		// value the row already held.
		assert.Contains(t, normalizeWebhookSQL(actualSQL), "consecutive_failures <> 0")
		// A success ends the run of failures, so the window has to be cleared
		// with the count. Leaving it set would have the next failure inherit a
		// window hours old and retire a subscription that just delivered.
		assert.Contains(t, normalizeWebhookSQL(actualSQL), "failing_since = NULL")
		assert.Contains(t, normalizeWebhookSQL(actualSQL), "failing_since IS NOT NULL")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("ResetFailures - connection error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(nil, errors.New("connection error"))

		err := repo.ResetFailures(ctx, workspaceID, subscriptionID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get workspace connection")
	})

	t.Run("ResetFailures - exec error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		mock.ExpectExec(`UPDATE webhook_subscriptions SET consecutive_failures = 0`).
			WillReturnError(errors.New("database error"))

		err = repo.ResetFailures(ctx, workspaceID, subscriptionID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to reset webhook subscription failures")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("DisableWithReason switches off and explains in one statement", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		// One statement, so a reader can never catch a subscription that has
		// been switched off without the explanation that goes with it.
		mock.ExpectExec(`UPDATE webhook_subscriptions SET enabled = false, disabled_reason = \$2, updated_at = \$3 WHERE id = \$1$`).
			WithArgs(subscriptionID, "endpoint returned 410 Gone", sqlmock.AnyArg()).
			WillReturnResult(sqlmock.NewResult(0, 1))

		err = repo.DisableWithReason(ctx, workspaceID, subscriptionID, "endpoint returned 410 Gone")
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("DisableWithReason - connection error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(nil, errors.New("connection error"))

		err := repo.DisableWithReason(ctx, workspaceID, subscriptionID, "reason")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get workspace connection")
	})

	t.Run("DisableWithReason - exec error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		mock.ExpectExec(`UPDATE webhook_subscriptions SET enabled = false`).
			WillReturnError(errors.New("database error"))

		err = repo.DisableWithReason(ctx, workspaceID, subscriptionID, "reason")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to disable webhook subscription")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("A subscription deleted mid-flight is not an error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Times(3).
			Return(db, nil)

		// These three run from the delivery worker, against a row a user may
		// have deleted while the delivery was in flight. Matching zero rows is
		// that race, not a failure, and the deliveries went with the row.
		mock.ExpectExec(`UPDATE webhook_subscriptions SET consecutive_failures = consecutive_failures`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`UPDATE webhook_subscriptions SET consecutive_failures = 0`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`UPDATE webhook_subscriptions SET enabled = false`).
			WillReturnResult(sqlmock.NewResult(0, 0))

		assert.NoError(t, repo.IncrementFailures(ctx, workspaceID, subscriptionID))
		assert.NoError(t, repo.ResetFailures(ctx, workspaceID, subscriptionID))
		assert.NoError(t, repo.DisableWithReason(ctx, workspaceID, subscriptionID, "reason"))
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

// TestWebhookSubscriptionRepository_Update_LeavesTheFailureStateToTheWorker pins
// which columns an edit is allowed to carry.
//
// consecutive_failures, failing_since and disabled_reason belong to the delivery
// worker, which writes them one at a time in SQL precisely so concurrent
// failures cannot lose counts. This statement carries a snapshot read at the top
// of somebody's request, so writing them here published a value that was already
// stale: an owner rotating a secret while the endpoint was being retired put
// back the counter it had reached and erased the reason it had recorded, and any
// client that touches its subscriptions on a timer re-armed failing_since
// forever — so an endpoint that was never coming back was never retired.
func TestWebhookSubscriptionRepository_Update_LeavesTheFailureStateToTheWorker(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewWebhookSubscriptionRepository(mockWorkspaceRepo)

	var actualSQL string
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(captureWebhookSQL(&actualSQL)))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mockWorkspaceRepo.EXPECT().GetConnection(gomock.Any(), "ws-123").Return(db, nil)

	reason := "automatically disabled after repeated delivery failures"
	failingSince := time.Now().UTC().Add(-9 * time.Hour)

	// A stale snapshot: the caller read this row before the worker touched it,
	// and every one of these three values is now wrong.
	sub := &domain.WebhookSubscription{
		ID: "sub-1", Name: "Renamed", URL: "https://example.com/hook", Secret: "s",
		Enabled:             true,
		ConsecutiveFailures: 3,
		FailingSince:        &failingSince,
		DisabledReason:      &reason,
	}

	// sqlmock rejects a call whose argument count differs, so this list is the
	// assertion that the three stale values are not sent at all.
	mock.ExpectExec(`UPDATE webhook_subscriptions SET`).
		WithArgs(
			"sub-1",
			"Renamed",
			"https://example.com/hook",
			"s",
			sqlmock.AnyArg(), // settings JSON
			true,
			sqlmock.AnyArg(), // updated_at
		).
		WillReturnResult(sqlmock.NewResult(0, 1))

	require.NoError(t, repo.Update(context.Background(), "ws-123", sub))

	normalized := normalizeWebhookSQL(actualSQL)
	for _, column := range []string{"consecutive_failures", "failing_since", "disabled_reason"} {
		assert.NotContains(t, normalized, column+" = $",
			"%s is the worker's to write, never the caller's snapshot", column)
	}

	// They are not simply dropped, though: the one legitimate user write to them
	// is clearing them when the subscription is switched back ON, which is a
	// statement that the endpoint has been fixed. That is expressed against the
	// row's own current `enabled` rather than against whatever the caller read,
	// so it fires on the real off-to-on transition and on nothing else.
	assert.Contains(t, normalized, "consecutive_failures = CASE WHEN $6 AND NOT enabled THEN 0 ELSE consecutive_failures END")
	assert.Contains(t, normalized, "failing_since = CASE WHEN $6 AND NOT enabled THEN NULL ELSE failing_since END")
	assert.Contains(t, normalized, "disabled_reason = CASE WHEN $6 AND NOT enabled THEN NULL ELSE disabled_reason END")
	assert.NoError(t, mock.ExpectationsWereMet())
}
