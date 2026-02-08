package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewWebhookDeliveryWorker(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSubRepo := mocks.NewMockWebhookSubscriptionRepository(ctrl)
	mockDeliveryRepo := mocks.NewMockWebhookDeliveryRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	t.Run("creates worker with provided HTTP client", func(t *testing.T) {
		customClient := &http.Client{Timeout: 45 * time.Second}
		worker := NewWebhookDeliveryWorker(mockSubRepo, mockDeliveryRepo, mockWorkspaceRepo, mockLogger, customClient)

		assert.NotNil(t, worker)
		assert.Equal(t, customClient, worker.httpClient)
		assert.Equal(t, mockSubRepo, worker.subscriptionRepo)
		assert.Equal(t, mockDeliveryRepo, worker.deliveryRepo)
		assert.Equal(t, mockWorkspaceRepo, worker.workspaceRepo)
		assert.Equal(t, mockLogger, worker.logger)
		assert.Equal(t, 10*time.Second, worker.pollInterval)
		assert.Equal(t, 100, worker.batchSize)
		assert.Equal(t, 1*time.Hour, worker.cleanupInterval)
		assert.Equal(t, 7, worker.retentionDays)
	})

	t.Run("creates worker with default HTTP client when nil provided", func(t *testing.T) {
		worker := NewWebhookDeliveryWorker(mockSubRepo, mockDeliveryRepo, mockWorkspaceRepo, mockLogger, nil)

		assert.NotNil(t, worker)
		assert.NotNil(t, worker.httpClient)
		assert.Equal(t, 30*time.Second, worker.httpClient.Timeout)
	})
}

func TestWebhookDeliveryWorker_Start(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSubRepo := mocks.NewMockWebhookSubscriptionRepository(ctrl)
	mockDeliveryRepo := mocks.NewMockWebhookDeliveryRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Configure logger to handle all log calls
	mockLogger.EXPECT().Info("Webhook delivery worker started").Times(1)
	mockLogger.EXPECT().Info("Webhook delivery worker stopping...").Times(1)
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	t.Run("stops when context is cancelled", func(t *testing.T) {
		worker := NewWebhookDeliveryWorker(mockSubRepo, mockDeliveryRepo, mockWorkspaceRepo, mockLogger, nil)
		worker.pollInterval = 50 * time.Millisecond // Speed up for testing

		ctx, cancel := context.WithCancel(context.Background())

		// No workspaces to process
		mockWorkspaceRepo.EXPECT().List(gomock.Any()).Return([]*domain.Workspace{}, nil).AnyTimes()

		done := make(chan bool)
		go func() {
			worker.Start(ctx)
			done <- true
		}()

		// Let it run for a bit
		time.Sleep(100 * time.Millisecond)
		cancel()

		// Wait for it to stop
		select {
		case <-done:
			// Success
		case <-time.After(2 * time.Second):
			t.Fatal("Worker did not stop in time")
		}
	})
}

func TestWebhookDeliveryWorker_processDeliveries(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSubRepo := mocks.NewMockWebhookSubscriptionRepository(ctrl)
	mockDeliveryRepo := mocks.NewMockWebhookDeliveryRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Configure logger
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	ctx := context.Background()

	t.Run("successfully processes deliveries for multiple workspaces", func(t *testing.T) {
		worker := NewWebhookDeliveryWorker(mockSubRepo, mockDeliveryRepo, mockWorkspaceRepo, mockLogger, nil)
		worker.lastCleanupTime = time.Now() // Prevent cleanup from running during this test

		workspaces := []*domain.Workspace{
			{ID: "workspace1", Name: "Workspace 1"},
			{ID: "workspace2", Name: "Workspace 2"},
		}

		mockWorkspaceRepo.EXPECT().List(ctx).Return(workspaces, nil)
		mockDeliveryRepo.EXPECT().GetPendingForWorkspace(ctx, "workspace1", 100).Return([]*domain.WebhookDelivery{}, nil)
		mockDeliveryRepo.EXPECT().GetPendingForWorkspace(ctx, "workspace2", 100).Return([]*domain.WebhookDelivery{}, nil)

		worker.processDeliveries(ctx)
	})

	t.Run("handles workspace list error", func(t *testing.T) {
		worker := NewWebhookDeliveryWorker(mockSubRepo, mockDeliveryRepo, mockWorkspaceRepo, mockLogger, nil)
		worker.lastCleanupTime = time.Now() // Prevent cleanup from running during this test

		mockWorkspaceRepo.EXPECT().List(ctx).Return(nil, errors.New("database error"))

		worker.processDeliveries(ctx)
		// Should log error but not panic
	})

	t.Run("continues processing other workspaces on error", func(t *testing.T) {
		worker := NewWebhookDeliveryWorker(mockSubRepo, mockDeliveryRepo, mockWorkspaceRepo, mockLogger, nil)
		worker.lastCleanupTime = time.Now() // Prevent cleanup from running during this test

		workspaces := []*domain.Workspace{
			{ID: "workspace1", Name: "Workspace 1"},
			{ID: "workspace2", Name: "Workspace 2"},
		}

		mockWorkspaceRepo.EXPECT().List(ctx).Return(workspaces, nil)
		mockDeliveryRepo.EXPECT().GetPendingForWorkspace(ctx, "workspace1", 100).Return(nil, errors.New("error"))
		mockDeliveryRepo.EXPECT().GetPendingForWorkspace(ctx, "workspace2", 100).Return([]*domain.WebhookDelivery{}, nil)

		worker.processDeliveries(ctx)
	})
}

func TestWebhookDeliveryWorker_processWorkspaceDeliveries(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSubRepo := mocks.NewMockWebhookSubscriptionRepository(ctrl)
	mockDeliveryRepo := mocks.NewMockWebhookDeliveryRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Configure logger
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	ctx := context.Background()
	workspaceID := "workspace1"

	t.Run("returns error when getting pending deliveries fails", func(t *testing.T) {
		worker := NewWebhookDeliveryWorker(mockSubRepo, mockDeliveryRepo, mockWorkspaceRepo, mockLogger, nil)

		mockDeliveryRepo.EXPECT().GetPendingForWorkspace(ctx, workspaceID, 100).
			Return(nil, errors.New("database error"))

		err := worker.processWorkspaceDeliveries(ctx, workspaceID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get pending deliveries")
	})

	t.Run("returns nil when no pending deliveries", func(t *testing.T) {
		worker := NewWebhookDeliveryWorker(mockSubRepo, mockDeliveryRepo, mockWorkspaceRepo, mockLogger, nil)

		mockDeliveryRepo.EXPECT().GetPendingForWorkspace(ctx, workspaceID, 100).
			Return([]*domain.WebhookDelivery{}, nil)

		err := worker.processWorkspaceDeliveries(ctx, workspaceID)
		assert.NoError(t, err)
	})

	t.Run("skips delivery when subscription not found", func(t *testing.T) {
		worker := NewWebhookDeliveryWorker(mockSubRepo, mockDeliveryRepo, mockWorkspaceRepo, mockLogger, nil)

		delivery := &domain.WebhookDelivery{
			ID:             "delivery1",
			SubscriptionID: "sub1",
			EventType:      "contact.created",
			Payload:        map[string]interface{}{"email": "test@example.com"},
			Attempts:       0,
			MaxAttempts:    10,
		}

		mockDeliveryRepo.EXPECT().GetPendingForWorkspace(ctx, workspaceID, 100).
			Return([]*domain.WebhookDelivery{delivery}, nil)
		mockSubRepo.EXPECT().GetByID(ctx, workspaceID, "sub1").
			Return(nil, errors.New("not found"))

		err := worker.processWorkspaceDeliveries(ctx, workspaceID)
		assert.NoError(t, err)
	})

	t.Run("skips delivery when subscription is disabled", func(t *testing.T) {
		worker := NewWebhookDeliveryWorker(mockSubRepo, mockDeliveryRepo, mockWorkspaceRepo, mockLogger, nil)

		delivery := &domain.WebhookDelivery{
			ID:             "delivery1",
			SubscriptionID: "sub1",
			EventType:      "contact.created",
			Payload:        map[string]interface{}{"email": "test@example.com"},
			Attempts:       0,
			MaxAttempts:    10,
		}

		subscription := &domain.WebhookSubscription{
			ID:      "sub1",
			URL:     "https://example.com/webhook",
			Secret:  "secret123",
			Enabled: false,
		}

		mockDeliveryRepo.EXPECT().GetPendingForWorkspace(ctx, workspaceID, 100).
			Return([]*domain.WebhookDelivery{delivery}, nil)
		mockSubRepo.EXPECT().GetByID(ctx, workspaceID, "sub1").
			Return(subscription, nil)

		err := worker.processWorkspaceDeliveries(ctx, workspaceID)
		assert.NoError(t, err)
	})

	t.Run("returns on context cancellation", func(t *testing.T) {
		worker := NewWebhookDeliveryWorker(mockSubRepo, mockDeliveryRepo, mockWorkspaceRepo, mockLogger, nil)

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		delivery := &domain.WebhookDelivery{
			ID:             "delivery1",
			SubscriptionID: "sub1",
			EventType:      "contact.created",
			Payload:        map[string]interface{}{"email": "test@example.com"},
			Attempts:       0,
			MaxAttempts:    10,
		}

		mockDeliveryRepo.EXPECT().GetPendingForWorkspace(ctx, workspaceID, 100).
			Return([]*domain.WebhookDelivery{delivery}, nil)

		err := worker.processWorkspaceDeliveries(ctx, workspaceID)
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("caches subscriptions to avoid repeated lookups", func(t *testing.T) {
		worker := NewWebhookDeliveryWorker(mockSubRepo, mockDeliveryRepo, mockWorkspaceRepo, mockLogger, nil)

		// Create a test server that will receive the webhooks
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		subscription := &domain.WebhookSubscription{
			ID:      "sub1",
			URL:     server.URL,
			Secret:  "secret123",
			Enabled: true,
		}

		deliveries := []*domain.WebhookDelivery{
			{
				ID:             "delivery1",
				SubscriptionID: "sub1",
				EventType:      "contact.created",
				Payload:        map[string]interface{}{"email": "test1@example.com"},
				Attempts:       0,
				MaxAttempts:    10,
			},
			{
				ID:             "delivery2",
				SubscriptionID: "sub1",
				EventType:      "contact.created",
				Payload:        map[string]interface{}{"email": "test2@example.com"},
				Attempts:       0,
				MaxAttempts:    10,
			},
		}

		mockDeliveryRepo.EXPECT().GetPendingForWorkspace(ctx, workspaceID, 100).
			Return(deliveries, nil)
		// Should only be called once due to caching
		mockSubRepo.EXPECT().GetByID(ctx, workspaceID, "sub1").
			Return(subscription, nil).Times(1)

		// Expect delivery success for both
		mockDeliveryRepo.EXPECT().MarkDelivered(ctx, workspaceID, "delivery1", gomock.Any(), gomock.Any()).Return(nil)
		mockDeliveryRepo.EXPECT().MarkDelivered(ctx, workspaceID, "delivery2", gomock.Any(), gomock.Any()).Return(nil)
		mockSubRepo.EXPECT().UpdateLastDeliveryAt(ctx, workspaceID, "sub1", gomock.Any()).Return(nil).Times(2)

		err := worker.processWorkspaceDeliveries(ctx, workspaceID)
		assert.NoError(t, err)
	})
}

func TestWebhookDeliveryWorker_deliverWebhook(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSubRepo := mocks.NewMockWebhookSubscriptionRepository(ctrl)
	mockDeliveryRepo := mocks.NewMockWebhookDeliveryRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Configure logger
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()

	ctx := context.Background()
	workspaceID := "workspace1"

	t.Run("successfully delivers webhook with 200 status", func(t *testing.T) {
		// Create a test server that returns success
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Verify headers
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
			assert.NotEmpty(t, r.Header.Get("webhook-id"))
			assert.NotEmpty(t, r.Header.Get("webhook-timestamp"))
			assert.NotEmpty(t, r.Header.Get("webhook-signature"))

			// Read and verify payload structure
			body, _ := io.ReadAll(r.Body)
			assert.Contains(t, string(body), "contact.created")
			assert.Contains(t, string(body), "test@example.com")

			w.WriteHeader(http.StatusOK)
			w.Write([]byte("OK"))
		}))
		defer server.Close()

		worker := NewWebhookDeliveryWorker(mockSubRepo, mockDeliveryRepo, mockWorkspaceRepo, mockLogger, nil)

		delivery := &domain.WebhookDelivery{
			ID:             "delivery1",
			SubscriptionID: "sub1",
			EventType:      "contact.created",
			Payload:        map[string]interface{}{"email": "test@example.com"},
			Attempts:       0,
			MaxAttempts:    10,
		}

		subscription := &domain.WebhookSubscription{
			ID:      "sub1",
			URL:     server.URL,
			Secret:  "secret123",
			Enabled: true,
		}

		mockDeliveryRepo.EXPECT().MarkDelivered(ctx, workspaceID, "delivery1", http.StatusOK, "OK").Return(nil)
		mockSubRepo.EXPECT().UpdateLastDeliveryAt(ctx, workspaceID, "sub1", gomock.Any()).Return(nil)

		worker.processDelivery(ctx, workspaceID, delivery, subscription)
	})

	t.Run("handles 4xx error status", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Bad Request"))
		}))
		defer server.Close()

		worker := NewWebhookDeliveryWorker(mockSubRepo, mockDeliveryRepo, mockWorkspaceRepo, mockLogger, nil)

		delivery := &domain.WebhookDelivery{
			ID:             "delivery1",
			SubscriptionID: "sub1",
			EventType:      "contact.created",
			Payload:        map[string]interface{}{"email": "test@example.com"},
			Attempts:       0,
			MaxAttempts:    10,
		}

		subscription := &domain.WebhookSubscription{
			ID:      "sub1",
			URL:     server.URL,
			Secret:  "secret123",
			Enabled: true,
		}

		statusCode := http.StatusBadRequest
		responseBody := "Bad Request"
		mockDeliveryRepo.EXPECT().ScheduleRetry(
			ctx, workspaceID, "delivery1", gomock.Any(), 1, &statusCode, &responseBody, gomock.Any(),
		).Return(nil)

		worker.processDelivery(ctx, workspaceID, delivery, subscription)
	})

	t.Run("handles network error", func(t *testing.T) {
		worker := NewWebhookDeliveryWorker(mockSubRepo, mockDeliveryRepo, mockWorkspaceRepo, mockLogger, nil)

		delivery := &domain.WebhookDelivery{
			ID:             "delivery1",
			SubscriptionID: "sub1",
			EventType:      "contact.created",
			Payload:        map[string]interface{}{"email": "test@example.com"},
			Attempts:       0,
			MaxAttempts:    10,
		}

		subscription := &domain.WebhookSubscription{
			ID:      "sub1",
			URL:     "http://invalid-domain-that-does-not-exist.example.com/webhook",
			Secret:  "secret123",
			Enabled: true,
		}

		// Network errors don't have status codes but have error messages
		mockDeliveryRepo.EXPECT().ScheduleRetry(
			ctx, workspaceID, "delivery1", gomock.Any(), 1, nil, gomock.Any(), gomock.Any(),
		).Return(nil)

		worker.processDelivery(ctx, workspaceID, delivery, subscription)
	})

	t.Run("marks as permanently failed after max attempts", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Server Error"))
		}))
		defer server.Close()

		worker := NewWebhookDeliveryWorker(mockSubRepo, mockDeliveryRepo, mockWorkspaceRepo, mockLogger, nil)

		delivery := &domain.WebhookDelivery{
			ID:             "delivery1",
			SubscriptionID: "sub1",
			EventType:      "contact.created",
			Payload:        map[string]interface{}{"email": "test@example.com"},
			Attempts:       9, // One before max
			MaxAttempts:    10,
		}

		subscription := &domain.WebhookSubscription{
			ID:      "sub1",
			URL:     server.URL,
			Secret:  "secret123",
			Enabled: true,
		}

		statusCode := http.StatusInternalServerError
		responseBody := "Server Error"
		mockDeliveryRepo.EXPECT().MarkFailed(
			ctx, workspaceID, "delivery1", 10, gomock.Any(), &statusCode, &responseBody,
		).Return(nil)

		worker.processDelivery(ctx, workspaceID, delivery, subscription)
	})

	t.Run("handles context cancellation", func(t *testing.T) {
		// Create a server that delays response
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(100 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		customClient := &http.Client{Timeout: 1 * time.Second}
		worker := NewWebhookDeliveryWorker(mockSubRepo, mockDeliveryRepo, mockWorkspaceRepo, mockLogger, customClient)

		delivery := &domain.WebhookDelivery{
			ID:             "delivery1",
			SubscriptionID: "sub1",
			EventType:      "contact.created",
			Payload:        map[string]interface{}{"email": "test@example.com"},
			Attempts:       0,
			MaxAttempts:    10,
		}

		subscription := &domain.WebhookSubscription{
			ID:      "sub1",
			URL:     server.URL,
			Secret:  "secret123",
			Enabled: true,
		}

		// Expect a ScheduleRetry call with the cancelled context
		mockDeliveryRepo.EXPECT().ScheduleRetry(
			gomock.Any(), workspaceID, "delivery1", gomock.Any(), 1, nil, gomock.Any(), gomock.Any(),
		).Return(nil)

		worker.processDelivery(ctx, workspaceID, delivery, subscription)
	})
}

func TestWebhookDeliveryWorker_signPayload(t *testing.T) {
	t.Run("generates valid signature", func(t *testing.T) {
		msgID := "msg123"
		timestamp := int64(1234567890)
		payload := []byte(`{"test":"data"}`)
		secret := []byte("secret123")

		signature := signPayload(msgID, timestamp, payload, secret)

		assert.NotEmpty(t, signature)
		assert.True(t, strings.HasPrefix(signature, "v1,"))
		assert.Greater(t, len(signature), 10)
	})

	t.Run("generates consistent signatures for same input", func(t *testing.T) {
		msgID := "msg123"
		timestamp := int64(1234567890)
		payload := []byte(`{"test":"data"}`)
		secret := []byte("secret123")

		sig1 := signPayload(msgID, timestamp, payload, secret)
		sig2 := signPayload(msgID, timestamp, payload, secret)

		assert.Equal(t, sig1, sig2)
	})

	t.Run("generates different signatures for different inputs", func(t *testing.T) {
		timestamp := int64(1234567890)
		payload := []byte(`{"test":"data"}`)
		secret := []byte("secret123")

		sig1 := signPayload("msg1", timestamp, payload, secret)
		sig2 := signPayload("msg2", timestamp, payload, secret)

		assert.NotEqual(t, sig1, sig2)
	})

	t.Run("generates different signatures for different timestamps", func(t *testing.T) {
		msgID := "msg123"
		payload := []byte(`{"test":"data"}`)
		secret := []byte("secret123")

		sig1 := signPayload(msgID, 1234567890, payload, secret)
		sig2 := signPayload(msgID, 1234567891, payload, secret)

		assert.NotEqual(t, sig1, sig2)
	})

	t.Run("generates different signatures for different payloads", func(t *testing.T) {
		msgID := "msg123"
		timestamp := int64(1234567890)
		secret := []byte("secret123")

		sig1 := signPayload(msgID, timestamp, []byte(`{"test":"data1"}`), secret)
		sig2 := signPayload(msgID, timestamp, []byte(`{"test":"data2"}`), secret)

		assert.NotEqual(t, sig1, sig2)
	})

	t.Run("generates different signatures for different secrets", func(t *testing.T) {
		msgID := "msg123"
		timestamp := int64(1234567890)
		payload := []byte(`{"test":"data"}`)

		sig1 := signPayload(msgID, timestamp, payload, []byte("secret1"))
		sig2 := signPayload(msgID, timestamp, payload, []byte("secret2"))

		assert.NotEqual(t, sig1, sig2)
	})
}

func TestWebhookDeliveryWorker_retryScheduling(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSubRepo := mocks.NewMockWebhookSubscriptionRepository(ctrl)
	mockDeliveryRepo := mocks.NewMockWebhookDeliveryRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Configure logger
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	ctx := context.Background()
	workspaceID := "workspace1"

	testCases := []struct {
		name             string
		attempts         int
		expectedDelayMin time.Duration
		expectedDelayMax time.Duration
	}{
		{
			name:             "first retry - 30 seconds",
			attempts:         0,
			expectedDelayMin: 29 * time.Second,
			expectedDelayMax: 31 * time.Second,
		},
		{
			name:             "second retry - 1 minute",
			attempts:         1,
			expectedDelayMin: 59 * time.Second,
			expectedDelayMax: 61 * time.Second,
		},
		{
			name:             "third retry - 2 minutes",
			attempts:         2,
			expectedDelayMin: 119 * time.Second,
			expectedDelayMax: 121 * time.Second,
		},
		{
			name:             "tenth retry - uses last delay (24 hours)",
			attempts:         10,
			expectedDelayMin: 23*time.Hour + 59*time.Minute,
			expectedDelayMax: 24*time.Hour + 1*time.Minute,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer server.Close()

			worker := NewWebhookDeliveryWorker(mockSubRepo, mockDeliveryRepo, mockWorkspaceRepo, mockLogger, nil)

			delivery := &domain.WebhookDelivery{
				ID:             "delivery1",
				SubscriptionID: "sub1",
				EventType:      "contact.created",
				Payload:        map[string]interface{}{"email": "test@example.com"},
				Attempts:       tc.attempts,
				MaxAttempts:    20,
			}

			subscription := &domain.WebhookSubscription{
				ID:      "sub1",
				URL:     server.URL,
				Secret:  "secret123",
				Enabled: true,
			}

			var capturedNextAttempt time.Time
			mockDeliveryRepo.EXPECT().ScheduleRetry(
				ctx, workspaceID, "delivery1", gomock.Any(), tc.attempts+1, gomock.Any(), gomock.Any(), gomock.Any(),
			).Do(func(_ context.Context, _ string, _ string, nextAttempt time.Time, _ int, _ *int, _ *string, _ *string) {
				capturedNextAttempt = nextAttempt
			}).Return(nil)

			now := time.Now()
			worker.processDelivery(ctx, workspaceID, delivery, subscription)

			actualDelay := capturedNextAttempt.Sub(now)
			assert.GreaterOrEqual(t, actualDelay, tc.expectedDelayMin, "Delay should be at least minimum")
			assert.LessOrEqual(t, actualDelay, tc.expectedDelayMax, "Delay should be at most maximum")
		})
	}
}

func TestWebhookDeliveryWorker_handleDeliverySuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSubRepo := mocks.NewMockWebhookSubscriptionRepository(ctrl)
	mockDeliveryRepo := mocks.NewMockWebhookDeliveryRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Configure logger
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	worker := NewWebhookDeliveryWorker(mockSubRepo, mockDeliveryRepo, mockWorkspaceRepo, mockLogger, nil)
	ctx := context.Background()
	workspaceID := "workspace1"

	t.Run("updates all stats on success", func(t *testing.T) {
		delivery := &domain.WebhookDelivery{ID: "delivery1"}
		subscription := &domain.WebhookSubscription{ID: "sub1"}

		mockDeliveryRepo.EXPECT().MarkDelivered(ctx, workspaceID, "delivery1", 200, "OK").Return(nil)
		mockSubRepo.EXPECT().UpdateLastDeliveryAt(ctx, workspaceID, "sub1", gomock.Any()).Return(nil)

		worker.handleDeliverySuccess(ctx, workspaceID, delivery, subscription, 200, "OK")
	})

	t.Run("logs error when MarkDelivered fails", func(t *testing.T) {
		delivery := &domain.WebhookDelivery{ID: "delivery1"}
		subscription := &domain.WebhookSubscription{ID: "sub1"}

		mockDeliveryRepo.EXPECT().MarkDelivered(ctx, workspaceID, "delivery1", 200, "OK").
			Return(errors.New("database error"))

		worker.handleDeliverySuccess(ctx, workspaceID, delivery, subscription, 200, "OK")
	})

	t.Run("continues even if UpdateLastDeliveryAt fails", func(t *testing.T) {
		delivery := &domain.WebhookDelivery{ID: "delivery1"}
		subscription := &domain.WebhookSubscription{ID: "sub1"}

		mockDeliveryRepo.EXPECT().MarkDelivered(ctx, workspaceID, "delivery1", 200, "OK").Return(nil)
		mockSubRepo.EXPECT().UpdateLastDeliveryAt(ctx, workspaceID, "sub1", gomock.Any()).
			Return(errors.New("error"))

		worker.handleDeliverySuccess(ctx, workspaceID, delivery, subscription, 200, "OK")
	})
}

func TestWebhookDeliveryWorker_handleDeliveryFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSubRepo := mocks.NewMockWebhookSubscriptionRepository(ctrl)
	mockDeliveryRepo := mocks.NewMockWebhookDeliveryRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Configure logger
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()

	worker := NewWebhookDeliveryWorker(mockSubRepo, mockDeliveryRepo, mockWorkspaceRepo, mockLogger, nil)
	ctx := context.Background()
	workspaceID := "workspace1"

	t.Run("schedules retry when attempts < max", func(t *testing.T) {
		delivery := &domain.WebhookDelivery{
			ID:          "delivery1",
			Attempts:    2,
			MaxAttempts: 10,
		}
		subscription := &domain.WebhookSubscription{ID: "sub1"}
		statusCode := 500
		responseBody := "Error"

		mockDeliveryRepo.EXPECT().ScheduleRetry(
			ctx, workspaceID, "delivery1", gomock.Any(), 3, &statusCode, &responseBody, gomock.Any(),
		).Return(nil)

		worker.handleDeliveryFailure(ctx, workspaceID, delivery, subscription, &statusCode, responseBody, "HTTP 500")
	})

	t.Run("marks as failed when max attempts reached", func(t *testing.T) {
		delivery := &domain.WebhookDelivery{
			ID:          "delivery1",
			Attempts:    9,
			MaxAttempts: 10,
		}
		subscription := &domain.WebhookSubscription{ID: "sub1"}
		statusCode := 500
		responseBody := "Error"

		mockDeliveryRepo.EXPECT().MarkFailed(
			ctx, workspaceID, "delivery1", 10, "HTTP 500", &statusCode, &responseBody,
		).Return(nil)

		worker.handleDeliveryFailure(ctx, workspaceID, delivery, subscription, &statusCode, responseBody, "HTTP 500")
	})

	t.Run("handles ScheduleRetry error", func(t *testing.T) {
		delivery := &domain.WebhookDelivery{
			ID:          "delivery1",
			Attempts:    2,
			MaxAttempts: 10,
		}
		subscription := &domain.WebhookSubscription{ID: "sub1"}
		statusCode := 500
		responseBody := "Error"

		mockDeliveryRepo.EXPECT().ScheduleRetry(
			ctx, workspaceID, "delivery1", gomock.Any(), 3, &statusCode, &responseBody, gomock.Any(),
		).Return(errors.New("database error"))

		worker.handleDeliveryFailure(ctx, workspaceID, delivery, subscription, &statusCode, responseBody, "HTTP 500")
	})

	t.Run("handles MarkFailed error", func(t *testing.T) {
		delivery := &domain.WebhookDelivery{
			ID:          "delivery1",
			Attempts:    9,
			MaxAttempts: 10,
		}
		subscription := &domain.WebhookSubscription{ID: "sub1"}
		statusCode := 500
		responseBody := "Error"

		mockDeliveryRepo.EXPECT().MarkFailed(
			ctx, workspaceID, "delivery1", 10, "HTTP 500", &statusCode, &responseBody,
		).Return(errors.New("database error"))

		worker.handleDeliveryFailure(ctx, workspaceID, delivery, subscription, &statusCode, responseBody, "HTTP 500")
	})

	t.Run("handles network failure without status code", func(t *testing.T) {
		delivery := &domain.WebhookDelivery{
			ID:          "delivery1",
			Attempts:    2,
			MaxAttempts: 10,
		}
		subscription := &domain.WebhookSubscription{ID: "sub1"}

		// Network failures have no status code but do have error messages
		mockDeliveryRepo.EXPECT().ScheduleRetry(
			ctx, workspaceID, "delivery1", gomock.Any(), 3, nil, gomock.Any(), gomock.Any(),
		).Return(nil)

		worker.handleDeliveryFailure(ctx, workspaceID, delivery, subscription, nil, "", "connection refused")
	})
}

func TestWebhookDeliveryWorker_SendTestWebhook(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSubRepo := mocks.NewMockWebhookSubscriptionRepository(ctrl)
	mockDeliveryRepo := mocks.NewMockWebhookDeliveryRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	worker := NewWebhookDeliveryWorker(mockSubRepo, mockDeliveryRepo, mockWorkspaceRepo, mockLogger, nil)
	ctx := context.Background()
	workspaceID := "workspace1"

	t.Run("successfully sends test webhook", func(t *testing.T) {
		// Create a test server
		var receivedHeaders http.Header
		var receivedBody []byte

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedHeaders = r.Header
			receivedBody, _ = io.ReadAll(r.Body)

			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Test webhook received"))
		}))
		defer server.Close()

		subscription := &domain.WebhookSubscription{
			ID:      "sub1",
			URL:     server.URL,
			Secret:  "secret123",
			Enabled: true,
		}

		statusCode, responseBody, err := worker.SendTestWebhook(ctx, workspaceID, subscription, "contact.created")

		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, statusCode)
		assert.Equal(t, "Test webhook received", responseBody)

		// Verify headers
		assert.Equal(t, "application/json", receivedHeaders.Get("Content-Type"))
		assert.NotEmpty(t, receivedHeaders.Get("webhook-id"))
		assert.NotEmpty(t, receivedHeaders.Get("webhook-timestamp"))
		assert.NotEmpty(t, receivedHeaders.Get("webhook-signature"))

		// Verify payload contains contact event data
		assert.Contains(t, string(receivedBody), "contact.created")
		assert.Contains(t, string(receivedBody), "test@example.com")
		assert.Contains(t, string(receivedBody), workspaceID)
	})

	t.Run("handles server error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Server error"))
		}))
		defer server.Close()

		subscription := &domain.WebhookSubscription{
			ID:      "sub1",
			URL:     server.URL,
			Secret:  "secret123",
			Enabled: true,
		}

		statusCode, responseBody, err := worker.SendTestWebhook(ctx, workspaceID, subscription, "email.sent")

		require.NoError(t, err)
		assert.Equal(t, http.StatusInternalServerError, statusCode)
		assert.Equal(t, "Server error", responseBody)
	})

	t.Run("handles network error", func(t *testing.T) {
		subscription := &domain.WebhookSubscription{
			ID:      "sub1",
			URL:     "http://invalid-domain-that-does-not-exist.example.com/webhook",
			Secret:  "secret123",
			Enabled: true,
		}

		statusCode, responseBody, err := worker.SendTestWebhook(ctx, workspaceID, subscription, "list.subscribed")

		require.Error(t, err)
		assert.Equal(t, 0, statusCode)
		assert.Empty(t, responseBody)
		assert.Contains(t, err.Error(), "request failed")
	})

	t.Run("handles invalid URL", func(t *testing.T) {
		subscription := &domain.WebhookSubscription{
			ID:      "sub1",
			URL:     "://invalid-url",
			Secret:  "secret123",
			Enabled: true,
		}

		statusCode, responseBody, err := worker.SendTestWebhook(ctx, workspaceID, subscription, "segment.joined")

		require.Error(t, err)
		assert.Equal(t, 0, statusCode)
		assert.Empty(t, responseBody)
		assert.Contains(t, err.Error(), "failed to create request")
	})

	t.Run("handles context cancellation", func(t *testing.T) {
		// Create a server that delays response
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(100 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		subscription := &domain.WebhookSubscription{
			ID:      "sub1",
			URL:     server.URL,
			Secret:  "secret123",
			Enabled: true,
		}

		customClient := &http.Client{Timeout: 1 * time.Second}
		worker := NewWebhookDeliveryWorker(mockSubRepo, mockDeliveryRepo, mockWorkspaceRepo, mockLogger, customClient)

		statusCode, responseBody, err := worker.SendTestWebhook(ctx, workspaceID, subscription, "custom_event.created")

		require.Error(t, err)
		assert.Equal(t, 0, statusCode)
		assert.Empty(t, responseBody)
		assert.Contains(t, err.Error(), "request failed")
	})

	t.Run("limits response body to 1KB", func(t *testing.T) {
		// Create a large response body
		largeBody := strings.Repeat("A", 2048) // 2KB

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(largeBody))
		}))
		defer server.Close()

		subscription := &domain.WebhookSubscription{
			ID:      "sub1",
			URL:     server.URL,
			Secret:  "secret123",
			Enabled: true,
		}

		statusCode, responseBody, err := worker.SendTestWebhook(ctx, workspaceID, subscription, "email.delivered")

		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, statusCode)
		assert.LessOrEqual(t, len(responseBody), 1024, "Response body should be limited to 1KB")
	})

	t.Run("uses default event type when empty", func(t *testing.T) {
		var receivedBody []byte

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedBody, _ = io.ReadAll(r.Body)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		subscription := &domain.WebhookSubscription{
			ID:      "sub1",
			URL:     server.URL,
			Secret:  "secret123",
			Enabled: true,
		}

		statusCode, _, err := worker.SendTestWebhook(ctx, workspaceID, subscription, "")

		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, statusCode)
		assert.Contains(t, string(receivedBody), `"type":"test"`)
	})
}

func TestWebhookDeliveryWorker_cleanupOldDeliveries(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSubRepo := mocks.NewMockWebhookSubscriptionRepository(ctrl)
	mockDeliveryRepo := mocks.NewMockWebhookDeliveryRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Configure logger
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

	ctx := context.Background()

	t.Run("skips cleanup when interval has not passed", func(t *testing.T) {
		worker := NewWebhookDeliveryWorker(mockSubRepo, mockDeliveryRepo, mockWorkspaceRepo, mockLogger, nil)
		worker.lastCleanupTime = time.Now() // Set to now so interval hasn't passed

		// Should not call List or CleanupOldDeliveries
		worker.cleanupOldDeliveries(ctx)
	})

	t.Run("runs cleanup when interval has passed", func(t *testing.T) {
		worker := NewWebhookDeliveryWorker(mockSubRepo, mockDeliveryRepo, mockWorkspaceRepo, mockLogger, nil)
		worker.lastCleanupTime = time.Now().Add(-2 * time.Hour) // Set to 2 hours ago

		workspaces := []*domain.Workspace{
			{ID: "workspace1", Name: "Workspace 1"},
			{ID: "workspace2", Name: "Workspace 2"},
		}

		mockWorkspaceRepo.EXPECT().List(ctx).Return(workspaces, nil)
		mockDeliveryRepo.EXPECT().CleanupOldDeliveries(ctx, "workspace1", 7).Return(int64(5), nil)
		mockDeliveryRepo.EXPECT().CleanupOldDeliveries(ctx, "workspace2", 7).Return(int64(3), nil)

		worker.cleanupOldDeliveries(ctx)

		// Verify lastCleanupTime was updated
		assert.WithinDuration(t, time.Now(), worker.lastCleanupTime, time.Second)
	})

	t.Run("handles workspace list error", func(t *testing.T) {
		worker := NewWebhookDeliveryWorker(mockSubRepo, mockDeliveryRepo, mockWorkspaceRepo, mockLogger, nil)
		worker.lastCleanupTime = time.Now().Add(-2 * time.Hour)

		mockWorkspaceRepo.EXPECT().List(ctx).Return(nil, errors.New("database error"))

		worker.cleanupOldDeliveries(ctx)
		// Should log error but not panic
	})

	t.Run("continues cleanup for other workspaces on error", func(t *testing.T) {
		worker := NewWebhookDeliveryWorker(mockSubRepo, mockDeliveryRepo, mockWorkspaceRepo, mockLogger, nil)
		worker.lastCleanupTime = time.Now().Add(-2 * time.Hour)

		workspaces := []*domain.Workspace{
			{ID: "workspace1", Name: "Workspace 1"},
			{ID: "workspace2", Name: "Workspace 2"},
		}

		mockWorkspaceRepo.EXPECT().List(ctx).Return(workspaces, nil)
		mockDeliveryRepo.EXPECT().CleanupOldDeliveries(ctx, "workspace1", 7).Return(int64(0), errors.New("cleanup error"))
		mockDeliveryRepo.EXPECT().CleanupOldDeliveries(ctx, "workspace2", 7).Return(int64(10), nil)

		worker.cleanupOldDeliveries(ctx)
	})

	t.Run("does not log when no records deleted", func(t *testing.T) {
		worker := NewWebhookDeliveryWorker(mockSubRepo, mockDeliveryRepo, mockWorkspaceRepo, mockLogger, nil)
		worker.lastCleanupTime = time.Now().Add(-2 * time.Hour)

		workspaces := []*domain.Workspace{
			{ID: "workspace1", Name: "Workspace 1"},
		}

		mockWorkspaceRepo.EXPECT().List(ctx).Return(workspaces, nil)
		mockDeliveryRepo.EXPECT().CleanupOldDeliveries(ctx, "workspace1", 7).Return(int64(0), nil)

		worker.cleanupOldDeliveries(ctx)
		// Info log should not be called for 0 deleted records
	})

	t.Run("runs on first call (zero lastCleanupTime)", func(t *testing.T) {
		worker := NewWebhookDeliveryWorker(mockSubRepo, mockDeliveryRepo, mockWorkspaceRepo, mockLogger, nil)
		// lastCleanupTime is zero value

		workspaces := []*domain.Workspace{
			{ID: "workspace1", Name: "Workspace 1"},
		}

		mockWorkspaceRepo.EXPECT().List(ctx).Return(workspaces, nil)
		mockDeliveryRepo.EXPECT().CleanupOldDeliveries(ctx, "workspace1", 7).Return(int64(0), nil)

		worker.cleanupOldDeliveries(ctx)
	})
}
