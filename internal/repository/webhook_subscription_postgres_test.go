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
			"enabled", "created_at", "updated_at",
			"last_delivery_at",
		}).AddRow(
			subscriptionID,
			"Test Subscription",
			"https://example.com/webhook",
			"secret-key",
			settingsJSON,
			true,
			now,
			now,
			lastDelivery,
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
			"enabled", "created_at", "updated_at",
			"last_delivery_at",
		}).AddRow(
			subscriptionID,
			"Simple Subscription",
			"https://example.com/webhook",
			"secret-key",
			simpleSettingsJSON,
			false,
			now,
			now,
			nil, // no last delivery
		)

		mock.ExpectQuery(`SELECT .+ FROM webhook_subscriptions WHERE id = \$1`).
			WithArgs(subscriptionID).
			WillReturnRows(rows)

		result, err := repo.GetByID(ctx, workspaceID, subscriptionID)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Nil(t, result.Settings.CustomEventFilters)
		assert.Nil(t, result.LastDeliveryAt)

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
			"enabled", "created_at", "updated_at",
			"last_delivery_at",
		}).AddRow(
			subscriptionID,
			"Test",
			"https://example.com",
			"secret",
			[]byte("{invalid json}"), // invalid JSON
			true,
			now,
			now,
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

	t.Run("Success - multiple subscriptions", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mockWorkspaceRepo.EXPECT().
			GetConnection(gomock.Any(), workspaceID).
			Return(db, nil)

		rows := sqlmock.NewRows([]string{
			"id", "name", "url", "secret", "settings",
			"enabled", "created_at", "updated_at",
			"last_delivery_at",
		}).
			AddRow(
				"sub-1",
				"Subscription 1",
				"https://example.com/webhook1",
				"secret1",
				settings1JSON,
				true,
				now,
				now,
				now,
			).
			AddRow(
				"sub-2",
				"Subscription 2",
				"https://example.com/webhook2",
				"secret2",
				settings2JSON,
				false,
				now,
				now,
				nil,
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

		// Verify second subscription
		assert.Equal(t, "sub-2", result[1].ID)
		assert.Equal(t, "Subscription 2", result[1].Name)
		assert.Nil(t, result[1].Settings.CustomEventFilters)
		assert.Nil(t, result[1].LastDeliveryAt)

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
			"enabled", "created_at", "updated_at",
			"last_delivery_at",
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
			"enabled", "created_at", "updated_at",
			"last_delivery_at",
		}).
			AddRow(
				"sub-1", "Test", "https://example.com", "secret",
				settings1JSON, true,
				now, now, nil,
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

		mock.ExpectExec(`UPDATE webhook_subscriptions SET name = \$2, url = \$3, secret = \$4, settings = \$5, enabled = \$6, updated_at = \$7 WHERE id = \$1`).
			WithArgs(
				"sub-1",
				"Updated Subscription",
				"https://example.com/webhook-updated",
				"new-secret",
				sqlmock.AnyArg(), // settings JSON
				false,
				sqlmock.AnyArg(),
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

		mock.ExpectExec(`UPDATE webhook_subscriptions SET name = \$2, url = \$3, secret = \$4, settings = \$5, enabled = \$6, updated_at = \$7 WHERE id = \$1`).
			WithArgs(
				"sub-2",
				"Simple Update",
				"https://example.com/simple",
				"secret",
				sqlmock.AnyArg(), // settings JSON
				true,
				sqlmock.AnyArg(),
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
