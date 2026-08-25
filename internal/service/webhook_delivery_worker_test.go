package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
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
		assert.Equal(t, 20, worker.failureThreshold)
		// Derived from the client that was passed, so raising the timeout cannot
		// leave the reclaim sweep short of it.
		assert.Equal(t, 50*time.Second, worker.claimLease)
	})

	t.Run("creates worker with default HTTP client when nil provided", func(t *testing.T) {
		worker := NewWebhookDeliveryWorker(mockSubRepo, mockDeliveryRepo, mockWorkspaceRepo, mockLogger, nil)

		assert.NotNil(t, worker)
		assert.NotNil(t, worker.httpClient)
		assert.Equal(t, 30*time.Second, worker.httpClient.Timeout)
		assert.Equal(t, 35*time.Second, worker.claimLease)
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

	// Every poll sweeps each workspace for claims a dead worker left behind
	// before it claims anything new. Nothing is stranded in these cases.
	mockDeliveryRepo.EXPECT().ReclaimStale(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(int64(0), nil).AnyTimes()

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
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()

	mockDeliveryRepo.EXPECT().ReclaimStale(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(int64(0), nil).AnyTimes()

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

	t.Run("drains the row when the subscription is genuinely gone", func(t *testing.T) {
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
			Return(nil, fmt.Errorf("looking up sub1: %w", domain.ErrWebhookSubscriptionNotFound))

		// Attempts pinned to the ceiling, which is what takes the row out of the
		// claim predicate for good.
		mockDeliveryRepo.EXPECT().MarkFailed(ctx, workspaceID, "delivery1", 10,
			"subscription no longer exists", nil, nil).Return(nil)

		err := worker.processWorkspaceDeliveries(ctx, workspaceID)
		assert.NoError(t, err)
	})

	t.Run("releases the row when the lookup fails for any other reason", func(t *testing.T) {
		worker := NewWebhookDeliveryWorker(mockSubRepo, mockDeliveryRepo, mockWorkspaceRepo, mockLogger, nil)

		delivery := &domain.WebhookDelivery{
			ID:             "delivery1",
			SubscriptionID: "sub1",
			EventType:      "contact.created",
			Payload:        map[string]interface{}{"email": "test@example.com"},
			Attempts:       3,
			MaxAttempts:    10,
		}

		mockDeliveryRepo.EXPECT().GetPendingForWorkspace(ctx, workspaceID, 100).
			Return([]*domain.WebhookDelivery{delivery}, nil)
		mockSubRepo.EXPECT().GetByID(ctx, workspaceID, "sub1").
			Return(nil, errors.New("pq: sorry, too many clients already"))

		// Back to pending with the attempt count untouched: a pool exhaustion
		// says nothing about the endpoint, so it must not spend an attempt.
		mockDeliveryRepo.EXPECT().UpdateStatus(ctx, workspaceID, "delivery1",
			domain.WebhookDeliveryStatusPending, 3, nil, nil, gomock.Any()).Return(nil)

		err := worker.processWorkspaceDeliveries(ctx, workspaceID)
		assert.NoError(t, err)
	})

	t.Run("drains the row when subscription is disabled", func(t *testing.T) {
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
			Secret:  "whsec_YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXowMTIzNDU=",
			Enabled: false,
		}

		mockDeliveryRepo.EXPECT().GetPendingForWorkspace(ctx, workspaceID, 100).
			Return([]*domain.WebhookDelivery{delivery}, nil)
		mockSubRepo.EXPECT().GetByID(ctx, workspaceID, "sub1").
			Return(subscription, nil)
		mockDeliveryRepo.EXPECT().MarkFailed(ctx, workspaceID, "delivery1", 10,
			"subscription is disabled", nil, nil).Return(nil)

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
			Secret:  "whsec_YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXowMTIzNDU=",
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
			Secret:  "whsec_YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXowMTIzNDU=",
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
			Secret:  "whsec_YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXowMTIzNDU=",
			Enabled: true,
		}

		statusCode := http.StatusBadRequest
		responseBody := "Bad Request"
		mockSubRepo.EXPECT().IncrementFailures(ctx, workspaceID, "sub1").Return(nil)
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
			Secret:  "whsec_YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXowMTIzNDU=",
			Enabled: true,
		}

		// Network errors don't have status codes but have error messages
		mockSubRepo.EXPECT().IncrementFailures(ctx, workspaceID, "sub1").Return(nil)
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
			Secret:  "whsec_YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXowMTIzNDU=",
			Enabled: true,
		}

		statusCode := http.StatusInternalServerError
		responseBody := "Server Error"
		mockSubRepo.EXPECT().IncrementFailures(ctx, workspaceID, "sub1").Return(nil)
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
			Secret:  "whsec_YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXowMTIzNDU=",
			Enabled: true,
		}

		// Expect a ScheduleRetry call with the cancelled context
		mockSubRepo.EXPECT().IncrementFailures(gomock.Any(), workspaceID, "sub1").Return(nil)
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

	// Verifies the chain `decodeSecret(whsec_…) -> signPayload` produces the
	// same signature a spec-compliant consumer would compute independently.
	// This is the regression guard for the Standard Webhooks alignment.
	t.Run("matches spec-compliant consumer verification", func(t *testing.T) {
		rawKey := make([]byte, 32)
		for i := range rawKey {
			rawKey[i] = byte(i)
		}
		stored := "whsec_" + base64.StdEncoding.EncodeToString(rawKey)

		msgID := "msg_2KWPBgLlAfxdpx2AI54pPJ85f4W"
		timestamp := int64(1674087231)
		payload := []byte(`{"type":"contact.created"}`)

		// What signPayload produces (after decodeSecret in the worker).
		key, err := decodeSecret(stored)
		require.NoError(t, err)
		got := signPayload(msgID, timestamp, payload, key)

		// What a spec-compliant consumer computes.
		signedContent := msgID + "." + strconv.FormatInt(timestamp, 10) + "." + string(payload)
		mac := hmac.New(sha256.New, rawKey)
		mac.Write([]byte(signedContent))
		want := "v1," + base64.StdEncoding.EncodeToString(mac.Sum(nil))

		assert.Equal(t, want, got)
	})

	t.Run("handles unicode / multi-byte payloads", func(t *testing.T) {
		secret := []byte("secret")
		payload := []byte(`{"note":"café 🌶️ 中文"}`)

		sig1 := signPayload("m", 1, payload, secret)
		sig2 := signPayload("m", 1, payload, secret)
		assert.Equal(t, sig1, sig2, "same bytes must yield same signature")

		// Hand-computed HMAC over the exact bytes — guards the bytes.Buffer/string
		// round-trip removal in signPayload.
		mac := hmac.New(sha256.New, secret)
		mac.Write([]byte("m.1."))
		mac.Write(payload)
		want := "v1," + base64.StdEncoding.EncodeToString(mac.Sum(nil))
		assert.Equal(t, want, sig1)
	})

	t.Run("handles empty payload", func(t *testing.T) {
		sig := signPayload("m", 1, []byte{}, []byte("secret"))
		assert.True(t, strings.HasPrefix(sig, "v1,"))
		assert.Greater(t, len(sig), len("v1,"))
	})

	t.Run("handles large payload", func(t *testing.T) {
		payload := bytes.Repeat([]byte("x"), 1024*1024) // 1 MB
		sig1 := signPayload("m", 1, payload, []byte("secret"))
		sig2 := signPayload("m", 1, payload, []byte("secret"))
		assert.Equal(t, sig1, sig2)
		assert.True(t, strings.HasPrefix(sig1, "v1,"))
	})

	// Unix seconds are ~1.7e9 in 2026; Unix millis would be ~1.7e12. A sane
	// signPayload call produces a short decimal suffix. This guards against a
	// future regression that passes UnixMilli() instead of Unix().
	t.Run("timestamp is formatted as decimal seconds", func(t *testing.T) {
		sig := signPayload("m", 1700000000, []byte("{}"), []byte("k"))

		// Rebuild the signed content the spec way and assert it matches.
		mac := hmac.New(sha256.New, []byte("k"))
		mac.Write([]byte("m.1700000000.{}"))
		want := "v1," + base64.StdEncoding.EncodeToString(mac.Sum(nil))
		assert.Equal(t, want, sig)
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
				Secret:  "whsec_YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXowMTIzNDU=",
				Enabled: true,
			}

			mockSubRepo.EXPECT().IncrementFailures(ctx, workspaceID, "sub1").Return(nil)

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
			Secret:  "whsec_YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXowMTIzNDU=",
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
			Secret:  "whsec_YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXowMTIzNDU=",
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
			Secret:  "whsec_YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXowMTIzNDU=",
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
			Secret:  "whsec_YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXowMTIzNDU=",
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
			Secret:  "whsec_YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXowMTIzNDU=",
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
			Secret:  "whsec_YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXowMTIzNDU=",
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
			Secret:  "whsec_YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXowMTIzNDU=",
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

// fakeDeliveryStore is a small in-memory stand-in for the delivery repository
// that models the three pieces of SQL the worker's correctness rests on: the
// claim predicate, the claim itself, and the release.
//
// gomock can prove which repository call a code path made. Only a store can
// prove what the NEXT poll sees, and that is the whole question behind "does
// this path drain the row" — a row skipped without a write keeps matching the
// predicate and comes back in every batch for the rest of the retention window.
type fakeDeliveryStore struct {
	rows        map[string]*domain.WebhookDelivery
	order       []string
	now         time.Time
	lastClaimed int
}

func newFakeDeliveryStore(rows ...*domain.WebhookDelivery) *fakeDeliveryStore {
	store := &fakeDeliveryStore{
		rows: make(map[string]*domain.WebhookDelivery, len(rows)),
		now:  time.Now().UTC(),
	}
	for _, row := range rows {
		store.rows[row.ID] = row
		store.order = append(store.order, row.ID)
	}
	return store
}

func (f *fakeDeliveryStore) row(t *testing.T, id string) *domain.WebhookDelivery {
	t.Helper()
	row, ok := f.rows[id]
	require.True(t, ok, "row %s no longer exists", id)
	return row
}

// GetPendingForWorkspace mirrors the repository's claim: the predicate is
// `status IN ('pending','failed') AND attempts < max_attempts AND
// next_attempt_at <= NOW()`, and selecting a row moves it to 'delivering'.
func (f *fakeDeliveryStore) GetPendingForWorkspace(_ context.Context, _ string, limit int) ([]*domain.WebhookDelivery, error) {
	var claimed []*domain.WebhookDelivery
	for _, id := range f.order {
		if len(claimed) >= limit {
			break
		}
		row := f.rows[id]
		if row.Status != domain.WebhookDeliveryStatusPending && row.Status != domain.WebhookDeliveryStatusFailed {
			continue
		}
		if row.Attempts >= row.MaxAttempts || row.NextAttemptAt.After(f.now) {
			continue
		}
		row.Status = domain.WebhookDeliveryStatusDelivering
		claimedAt := f.now
		row.ClaimedAt = &claimedAt

		handed := *row
		claimed = append(claimed, &handed)
	}
	f.lastClaimed = len(claimed)
	return claimed, nil
}

func (f *fakeDeliveryStore) UpdateStatus(_ context.Context, _, id string, status string, attempts int, _ *int, _, lastError *string) error {
	row, ok := f.rows[id]
	if !ok {
		return nil
	}
	row.Status = status
	row.Attempts = attempts
	row.LastError = lastError
	if status != domain.WebhookDeliveryStatusDelivering {
		row.ClaimedAt = nil
	}
	return nil
}

func (f *fakeDeliveryStore) MarkDelivered(_ context.Context, _, id string, responseStatus int, responseBody string) error {
	row, ok := f.rows[id]
	if !ok {
		return nil
	}
	row.Status = domain.WebhookDeliveryStatusDelivered
	row.Attempts++
	row.LastResponseStatus = &responseStatus
	row.LastResponseBody = &responseBody
	row.ClaimedAt = nil
	return nil
}

func (f *fakeDeliveryStore) ScheduleRetry(_ context.Context, _, id string, nextAttempt time.Time, attempts int, responseStatus *int, responseBody, lastError *string) error {
	row, ok := f.rows[id]
	if !ok {
		return nil
	}
	row.Status = domain.WebhookDeliveryStatusFailed
	row.Attempts = attempts
	row.NextAttemptAt = nextAttempt
	row.LastResponseStatus = responseStatus
	row.LastResponseBody = responseBody
	row.LastError = lastError
	row.ClaimedAt = nil
	return nil
}

func (f *fakeDeliveryStore) MarkFailed(_ context.Context, _, id string, attempts int, lastError string, responseStatus *int, responseBody *string) error {
	row, ok := f.rows[id]
	if !ok {
		return nil
	}
	row.Status = domain.WebhookDeliveryStatusFailed
	row.Attempts = attempts
	row.LastError = &lastError
	row.LastResponseStatus = responseStatus
	row.LastResponseBody = responseBody
	row.ClaimedAt = nil
	return nil
}

// ReclaimStale mirrors the repository's sweep, including its treatment of a
// 'delivering' row with no claimed_at as infinitely stale.
func (f *fakeDeliveryStore) ReclaimStale(_ context.Context, _ string, lease time.Duration) (int64, error) {
	var reclaimed int64
	for _, id := range f.order {
		row := f.rows[id]
		if row.Status != domain.WebhookDeliveryStatusDelivering {
			continue
		}
		if row.ClaimedAt != nil && f.now.Sub(*row.ClaimedAt) < lease {
			continue
		}
		row.Status = domain.WebhookDeliveryStatusPending
		row.ClaimedAt = nil
		reclaimed++
	}
	return reclaimed, nil
}

func (f *fakeDeliveryStore) DeleteBySubscriptionID(_ context.Context, _, subscriptionID string) error {
	kept := f.order[:0]
	for _, id := range f.order {
		if f.rows[id].SubscriptionID == subscriptionID {
			delete(f.rows, id)
			continue
		}
		kept = append(kept, id)
	}
	f.order = kept
	return nil
}

func (f *fakeDeliveryStore) Create(_ context.Context, _ string, delivery *domain.WebhookDelivery) error {
	f.rows[delivery.ID] = delivery
	f.order = append(f.order, delivery.ID)
	return nil
}

func (f *fakeDeliveryStore) ListAll(_ context.Context, _ string, _ *string, _, _ int) ([]*domain.WebhookDelivery, int, error) {
	return nil, 0, nil
}

func (f *fakeDeliveryStore) CleanupOldDeliveries(_ context.Context, _ string, _ int) (int64, error) {
	return 0, nil
}

// permissiveWebhookLogger is the logger every worker test wants: it records
// nothing and accepts everything, because none of these cases are about logging.
func permissiveWebhookLogger(ctrl *gomock.Controller) *pkgmocks.MockLogger {
	l := pkgmocks.NewMockLogger(ctrl)
	l.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(l).AnyTimes()
	l.EXPECT().WithFields(gomock.Any()).Return(l).AnyTimes()
	l.EXPECT().Info(gomock.Any()).AnyTimes()
	l.EXPECT().Debug(gomock.Any()).AnyTimes()
	l.EXPECT().Warn(gomock.Any()).AnyTimes()
	l.EXPECT().Error(gomock.Any()).AnyTimes()
	return l
}

const testWebhookSecret = "whsec_YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXowMTIzNDU="

// TestWebhookDeliveryWorker_everySkipPathDrainsTheRow drives each of the four
// exits that used to leave a claimed row untouched, and then polls a second
// time.
//
// The second poll is the assertion that matters. A row that is skipped rather
// than written keeps matching the claim predicate, so it is handed back on every
// ten-second poll for the whole seven-day retention window while it can never be
// delivered — one of a hundred batch slots, occupied forever. A workspace that
// turns integrations on and off normally accumulates enough of them to stop
// delivering anything at all.
func TestWebhookDeliveryWorker_everySkipPathDrainsTheRow(t *testing.T) {
	const workspaceID = "ws-1"

	newDelivery := func(payload map[string]interface{}) *domain.WebhookDelivery {
		return &domain.WebhookDelivery{
			ID:             "delivery-1",
			SubscriptionID: "sub-1",
			EventType:      "contact.created",
			Payload:        payload,
			Status:         domain.WebhookDeliveryStatusPending,
			Attempts:       0,
			MaxAttempts:    10,
			NextAttemptAt:  time.Now().UTC().Add(-time.Minute),
		}
	}

	goodPayload := map[string]interface{}{"email": "test@example.com"}

	cases := []struct {
		name string
		// arrange returns the store the worker runs against and arms the
		// subscription repository for the single lookup this path should make.
		arrange func(*mocks.MockWebhookSubscriptionRepository) *fakeDeliveryStore
	}{
		{
			// performUnsubscribe deletes a subscription on every integration
			// turn-off, so this is the common case, not the exotic one.
			name: "subscription was deleted",
			arrange: func(repo *mocks.MockWebhookSubscriptionRepository) *fakeDeliveryStore {
				repo.EXPECT().GetByID(gomock.Any(), workspaceID, "sub-1").
					Return(nil, fmt.Errorf("loading sub-1: %w", domain.ErrWebhookSubscriptionNotFound)).Times(1)
				return newFakeDeliveryStore(newDelivery(goodPayload))
			},
		},
		{
			// The one that matters most: a dead endpoint now disables its own
			// subscription, so this path is walked by the automatic sweep and
			// not only by a user flipping a switch.
			name: "subscription is disabled",
			arrange: func(repo *mocks.MockWebhookSubscriptionRepository) *fakeDeliveryStore {
				repo.EXPECT().GetByID(gomock.Any(), workspaceID, "sub-1").
					Return(&domain.WebhookSubscription{ID: "sub-1", URL: "https://example.com/h", Secret: testWebhookSecret, Enabled: false}, nil).Times(1)
				return newFakeDeliveryStore(newDelivery(goodPayload))
			},
		},
		{
			name: "envelope cannot be marshalled",
			arrange: func(repo *mocks.MockWebhookSubscriptionRepository) *fakeDeliveryStore {
				repo.EXPECT().GetByID(gomock.Any(), workspaceID, "sub-1").
					Return(&domain.WebhookSubscription{ID: "sub-1", URL: "https://example.com/h", Secret: testWebhookSecret, Enabled: true}, nil).Times(1)
				// A channel has no JSON representation, so encoding this row
				// fails now and would fail identically on every retry.
				return newFakeDeliveryStore(newDelivery(map[string]interface{}{"unencodable": make(chan int)}))
			},
		},
		{
			name: "request cannot be built",
			arrange: func(repo *mocks.MockWebhookSubscriptionRepository) *fakeDeliveryStore {
				repo.EXPECT().GetByID(gomock.Any(), workspaceID, "sub-1").
					Return(&domain.WebhookSubscription{
						ID: "sub-1",
						// A control character in the path: url.Parse refuses it,
						// so no request is ever built for this subscription.
						URL:     "https://example.com/hook\x7f",
						Secret:  testWebhookSecret,
						Enabled: true,
					}, nil).Times(1)
				return newFakeDeliveryStore(newDelivery(goodPayload))
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			subRepo := mocks.NewMockWebhookSubscriptionRepository(ctrl)
			store := tc.arrange(subRepo)
			worker := NewWebhookDeliveryWorker(subRepo, store, mocks.NewMockWorkspaceRepository(ctrl), permissiveWebhookLogger(ctrl), nil)

			require.NoError(t, worker.processWorkspaceDeliveries(context.Background(), workspaceID))
			require.Equal(t, 1, store.lastClaimed, "the first poll should claim the row")

			row := store.row(t, "delivery-1")
			assert.Equal(t, domain.WebhookDeliveryStatusFailed, row.Status)
			assert.Nil(t, row.ClaimedAt, "a terminal row must not stay claimed")
			require.NotNil(t, row.LastError, "the delivery log has to say why the row was dropped")
			assert.NotEmpty(t, *row.LastError)

			// The second poll is the point. GetByID is armed Times(1), so gomock
			// also fails here if the row was handed out again.
			require.NoError(t, worker.processWorkspaceDeliveries(context.Background(), workspaceID))
			assert.Equal(t, 0, store.lastClaimed, "the drained row must not be claimed again")
		})
	}
}

// A failure to reach the database says nothing about the delivery, and the
// difference between "this subscription is gone" and "Postgres is restarting" is
// the difference between dropping one row and destroying every in-flight
// delivery in every workspace during a five-second blip.
func TestWebhookDeliveryWorker_transientLookupErrorLeavesTheRowPending(t *testing.T) {
	const workspaceID = "ws-1"

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	store := newFakeDeliveryStore(&domain.WebhookDelivery{
		ID:             "delivery-1",
		SubscriptionID: "sub-1",
		EventType:      "contact.created",
		Payload:        map[string]interface{}{"email": "test@example.com"},
		Status:         domain.WebhookDeliveryStatusPending,
		Attempts:       4,
		MaxAttempts:    10,
		NextAttemptAt:  time.Now().UTC().Add(-time.Minute),
	})

	subRepo := mocks.NewMockWebhookSubscriptionRepository(ctrl)
	subRepo.EXPECT().GetByID(gomock.Any(), workspaceID, "sub-1").
		Return(nil, errors.New("pq: sorry, too many clients already")).Times(2)

	worker := NewWebhookDeliveryWorker(subRepo, store, mocks.NewMockWorkspaceRepository(ctrl), permissiveWebhookLogger(ctrl), nil)

	require.NoError(t, worker.processWorkspaceDeliveries(context.Background(), workspaceID))

	row := store.row(t, "delivery-1")
	assert.Equal(t, domain.WebhookDeliveryStatusPending, row.Status)
	assert.Nil(t, row.ClaimedAt)
	assert.Equal(t, 4, row.Attempts, "a database outage must not spend one of the delivery's attempts")

	// And it is still there for the next poll, which is the whole difference
	// from the drained cases above.
	require.NoError(t, worker.processWorkspaceDeliveries(context.Background(), workspaceID))
	assert.Equal(t, 1, store.lastClaimed)
}

func TestWebhookDeliveryWorker_reclaimStale(t *testing.T) {
	const workspaceID = "ws-1"

	strandedRow := func(id string, claimedAgo time.Duration, now time.Time) *domain.WebhookDelivery {
		claimedAt := now.Add(-claimedAgo)
		return &domain.WebhookDelivery{
			ID:             id,
			SubscriptionID: "sub-1",
			Status:         domain.WebhookDeliveryStatusDelivering,
			ClaimedAt:      &claimedAt,
			MaxAttempts:    10,
			NextAttemptAt:  now.Add(-time.Minute),
		}
	}

	t.Run("returns a claim that outlived the lease and leaves a live one alone", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		now := time.Now().UTC()
		store := newFakeDeliveryStore(
			strandedRow("dead-worker", time.Minute, now),
			strandedRow("in-flight", 2*time.Second, now),
		)
		store.now = now

		worker := NewWebhookDeliveryWorker(
			mocks.NewMockWebhookSubscriptionRepository(ctrl), store,
			mocks.NewMockWorkspaceRepository(ctrl), permissiveWebhookLogger(ctrl),
			&http.Client{Timeout: 10 * time.Second})

		worker.reclaimStaleDeliveries(context.Background(), workspaceID)

		assert.Equal(t, domain.WebhookDeliveryStatusPending, store.row(t, "dead-worker").Status)
		assert.Nil(t, store.row(t, "dead-worker").ClaimedAt)
		assert.Equal(t, domain.WebhookDeliveryStatusDelivering, store.row(t, "in-flight").Status,
			"a POST that is still in flight must not be reclaimed and sent twice")
	})

	t.Run("sweeps before claiming", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		deliveryRepo := mocks.NewMockWebhookDeliveryRepository(ctrl)
		worker := NewWebhookDeliveryWorker(
			mocks.NewMockWebhookSubscriptionRepository(ctrl), deliveryRepo,
			mocks.NewMockWorkspaceRepository(ctrl), permissiveWebhookLogger(ctrl),
			&http.Client{Timeout: 10 * time.Second})

		// Reclaiming after the claim would leave every reclaimed row waiting a
		// further poll for no reason.
		gomock.InOrder(
			deliveryRepo.EXPECT().ReclaimStale(gomock.Any(), workspaceID, 15*time.Second).Return(int64(2), nil),
			deliveryRepo.EXPECT().GetPendingForWorkspace(gomock.Any(), workspaceID, 100).
				Return([]*domain.WebhookDelivery{}, nil),
		)

		require.NoError(t, worker.processWorkspaceDeliveries(context.Background(), workspaceID))
	})

	t.Run("a failed sweep does not stop the poll", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		deliveryRepo := mocks.NewMockWebhookDeliveryRepository(ctrl)
		worker := NewWebhookDeliveryWorker(
			mocks.NewMockWebhookSubscriptionRepository(ctrl), deliveryRepo,
			mocks.NewMockWorkspaceRepository(ctrl), permissiveWebhookLogger(ctrl), nil)

		deliveryRepo.EXPECT().ReclaimStale(gomock.Any(), workspaceID, gomock.Any()).
			Return(int64(0), errors.New("database error"))
		deliveryRepo.EXPECT().GetPendingForWorkspace(gomock.Any(), workspaceID, 100).
			Return([]*domain.WebhookDelivery{}, nil)

		require.NoError(t, worker.processWorkspaceDeliveries(context.Background(), workspaceID))
	})
}

// The lease has to sit just past the request, on both sides: shorter than the
// HTTP timeout and the sweep reclaims rows whose POST is still in flight, which
// manufactures the duplicate the claim exists to prevent; measured in minutes
// and it silently overrides the first rungs of the retry ladder.
func TestClaimLeaseFor(t *testing.T) {
	assert.Equal(t, 15*time.Second, claimLeaseFor(10*time.Second),
		"production's 10s client gets the 15s lease")
	assert.Equal(t, 35*time.Second, claimLeaseFor(30*time.Second),
		"a longer timeout has to push the lease out with it")
	assert.Equal(t, 15*time.Second, claimLeaseFor(0),
		"a client with no timeout falls back rather than leaving a 5s lease")
}

// TestWebhookDeliveryWorker_responsePolicy pins the table in handleResponseStatus.
func TestWebhookDeliveryWorker_responsePolicy(t *testing.T) {
	const workspaceID = "ws-1"

	newDelivery := func() *domain.WebhookDelivery {
		return &domain.WebhookDelivery{
			ID:             "delivery-1",
			SubscriptionID: "sub-1",
			EventType:      "contact.created",
			Payload:        map[string]interface{}{"email": "test@example.com"},
			Attempts:       0,
			MaxAttempts:    10,
		}
	}

	// respondWith serves one fixed status to every request.
	respondWith := func(t *testing.T, status int) *httptest.Server {
		t.Helper()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
			_, _ = w.Write([]byte("body"))
		}))
		t.Cleanup(server.Close)
		return server
	}

	t.Run("410 deletes a Zapier subscription and its queue", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		server := respondWith(t, http.StatusGone)
		subRepo := mocks.NewMockWebhookSubscriptionRepository(ctrl)
		deliveryRepo := mocks.NewMockWebhookDeliveryRepository(ctrl)
		worker := NewWebhookDeliveryWorker(subRepo, deliveryRepo, mocks.NewMockWorkspaceRepository(ctrl), permissiveWebhookLogger(ctrl), nil)

		sub := &domain.WebhookSubscription{
			ID: "sub-1", URL: server.URL, Secret: testWebhookSecret, Enabled: true,
			Source: domain.WebhookSubscriptionSourceZapier,
		}

		// Deleting is right for a Zapier row and only for a Zapier row: a Zap
		// that comes back re-creates its subscription through performSubscribe,
		// so nothing is lost, and the user is spared a webhook they never made.
		subRepo.EXPECT().Delete(gomock.Any(), workspaceID, "sub-1").Return(nil)
		deliveryRepo.EXPECT().DeleteBySubscriptionID(gomock.Any(), workspaceID, "sub-1").Return(nil)

		worker.processDelivery(context.Background(), workspaceID, newDelivery(), sub)
		assert.False(t, sub.Enabled, "the rest of the batch must not be POSTed at a dead endpoint")
	})

	t.Run("410 disables a hand-made subscription and drains the row", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		server := respondWith(t, http.StatusGone)
		subRepo := mocks.NewMockWebhookSubscriptionRepository(ctrl)
		deliveryRepo := mocks.NewMockWebhookDeliveryRepository(ctrl)
		worker := NewWebhookDeliveryWorker(subRepo, deliveryRepo, mocks.NewMockWorkspaceRepository(ctrl), permissiveWebhookLogger(ctrl), nil)

		sub := &domain.WebhookSubscription{
			ID: "sub-1", URL: server.URL, Secret: testWebhookSecret, Enabled: true,
			Source: domain.WebhookSubscriptionSourceUser,
		}

		// Reversible and visible, because a user typed this URL in and may want
		// it back — unlike the Zapier row, nothing will re-create it.
		var reason string
		subRepo.EXPECT().DisableWithReason(gomock.Any(), workspaceID, "sub-1", gomock.Any()).
			DoAndReturn(func(_ context.Context, _, _, r string) error {
				reason = r
				return nil
			})
		// Terminal, not retried: 410 means the endpoint has said it is finished.
		status := http.StatusGone
		body := "body"
		deliveryRepo.EXPECT().MarkFailed(gomock.Any(), workspaceID, "delivery-1", 10, gomock.Any(), &status, &body).Return(nil)

		worker.processDelivery(context.Background(), workspaceID, newDelivery(), sub)

		assert.Contains(t, reason, "410")
		assert.False(t, sub.Enabled)
	})

	t.Run("410 still drains the row when the Zapier subscription cannot be deleted", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		server := respondWith(t, http.StatusGone)
		subRepo := mocks.NewMockWebhookSubscriptionRepository(ctrl)
		deliveryRepo := mocks.NewMockWebhookDeliveryRepository(ctrl)
		worker := NewWebhookDeliveryWorker(subRepo, deliveryRepo, mocks.NewMockWorkspaceRepository(ctrl), permissiveWebhookLogger(ctrl), nil)

		sub := &domain.WebhookSubscription{
			ID: "sub-1", URL: server.URL, Secret: testWebhookSecret, Enabled: true,
			Source: domain.WebhookSubscriptionSourceZapier,
		}

		subRepo.EXPECT().Delete(gomock.Any(), workspaceID, "sub-1").Return(errors.New("database error"))
		status := http.StatusGone
		body := "body"
		deliveryRepo.EXPECT().MarkFailed(gomock.Any(), workspaceID, "delivery-1", 10, gomock.Any(), &status, &body).Return(nil)

		worker.processDelivery(context.Background(), workspaceID, newDelivery(), sub)
	})

	// Zapier authored the REST Hooks spec, and it says an endpoint may only be
	// marked bad once a consistent 404 has been proven over time. A Zap that is
	// switched back on resumes answering 200.
	t.Run("a single 404 retries and disables nothing", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		server := respondWith(t, http.StatusNotFound)
		subRepo := mocks.NewMockWebhookSubscriptionRepository(ctrl)
		deliveryRepo := mocks.NewMockWebhookDeliveryRepository(ctrl)
		worker := NewWebhookDeliveryWorker(subRepo, deliveryRepo, mocks.NewMockWorkspaceRepository(ctrl), permissiveWebhookLogger(ctrl), nil)

		sub := &domain.WebhookSubscription{ID: "sub-1", URL: server.URL, Secret: testWebhookSecret, Enabled: true}

		subRepo.EXPECT().IncrementFailures(gomock.Any(), workspaceID, "sub-1").Return(nil)
		// No DisableWithReason and no MarkFailed armed: either would fail here.
		deliveryRepo.EXPECT().ScheduleRetry(gomock.Any(), workspaceID, "delivery-1", gomock.Any(), 1, gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

		worker.processDelivery(context.Background(), workspaceID, newDelivery(), sub)
		assert.True(t, sub.Enabled)
	})

	t.Run("a sustained 404 disables the subscription but keeps it", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		server := respondWith(t, http.StatusNotFound)
		subRepo := mocks.NewMockWebhookSubscriptionRepository(ctrl)
		deliveryRepo := mocks.NewMockWebhookDeliveryRepository(ctrl)
		worker := NewWebhookDeliveryWorker(subRepo, deliveryRepo, mocks.NewMockWorkspaceRepository(ctrl), permissiveWebhookLogger(ctrl), nil)
		worker.failureThreshold = 3

		sub := &domain.WebhookSubscription{
			ID: "sub-1", URL: server.URL, Secret: testWebhookSecret, Enabled: true,
			ConsecutiveFailures: 2,
		}

		subRepo.EXPECT().IncrementFailures(gomock.Any(), workspaceID, "sub-1").Return(nil)
		var reason string
		// Disabled, never deleted — a 404 is not proof the subscription is gone,
		// only that this endpoint has been answering badly for a long time.
		subRepo.EXPECT().DisableWithReason(gomock.Any(), workspaceID, "sub-1", gomock.Any()).
			DoAndReturn(func(_ context.Context, _, _, r string) error {
				reason = r
				return nil
			})
		deliveryRepo.EXPECT().MarkFailed(gomock.Any(), workspaceID, "delivery-1", 10, gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

		worker.processDelivery(context.Background(), workspaceID, newDelivery(), sub)

		assert.Contains(t, reason, "404")
		assert.False(t, sub.Enabled)
		assert.Equal(t, 3, sub.ConsecutiveFailures)
	})

	// A workspace busy enough to be throttled must not have its integration
	// switched off for being busy.
	t.Run("429 retries without counting a failure", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		server := respondWith(t, http.StatusTooManyRequests)
		subRepo := mocks.NewMockWebhookSubscriptionRepository(ctrl)
		deliveryRepo := mocks.NewMockWebhookDeliveryRepository(ctrl)
		worker := NewWebhookDeliveryWorker(subRepo, deliveryRepo, mocks.NewMockWorkspaceRepository(ctrl), permissiveWebhookLogger(ctrl), nil)
		worker.failureThreshold = 1

		sub := &domain.WebhookSubscription{ID: "sub-1", URL: server.URL, Secret: testWebhookSecret, Enabled: true}

		// No IncrementFailures armed. With the threshold at one, an increment
		// here would also disable the subscription outright.
		deliveryRepo.EXPECT().ScheduleRetry(gomock.Any(), workspaceID, "delivery-1", gomock.Any(), 1, gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

		worker.processDelivery(context.Background(), workspaceID, newDelivery(), sub)

		assert.True(t, sub.Enabled)
		assert.Equal(t, 0, sub.ConsecutiveFailures)
	})

	t.Run("a success clears a counter that was above zero", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		server := respondWith(t, http.StatusOK)
		subRepo := mocks.NewMockWebhookSubscriptionRepository(ctrl)
		deliveryRepo := mocks.NewMockWebhookDeliveryRepository(ctrl)
		worker := NewWebhookDeliveryWorker(subRepo, deliveryRepo, mocks.NewMockWorkspaceRepository(ctrl), permissiveWebhookLogger(ctrl), nil)

		sub := &domain.WebhookSubscription{
			ID: "sub-1", URL: server.URL, Secret: testWebhookSecret, Enabled: true,
			ConsecutiveFailures: 7,
		}

		subRepo.EXPECT().ResetFailures(gomock.Any(), workspaceID, "sub-1").Return(nil)
		deliveryRepo.EXPECT().MarkDelivered(gomock.Any(), workspaceID, "delivery-1", http.StatusOK, "body").Return(nil)
		subRepo.EXPECT().UpdateLastDeliveryAt(gomock.Any(), workspaceID, "sub-1", gomock.Any()).Return(nil)

		worker.processDelivery(context.Background(), workspaceID, newDelivery(), sub)
		assert.Equal(t, 0, sub.ConsecutiveFailures)
	})

	// Every delivery already writes to webhook_subscriptions through
	// UpdateLastDeliveryAt; resetting a counter that is already zero would double
	// that write for the healthy case, which is nearly every delivery.
	t.Run("a success on a healthy subscription writes no reset", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		server := respondWith(t, http.StatusOK)
		subRepo := mocks.NewMockWebhookSubscriptionRepository(ctrl)
		deliveryRepo := mocks.NewMockWebhookDeliveryRepository(ctrl)
		worker := NewWebhookDeliveryWorker(subRepo, deliveryRepo, mocks.NewMockWorkspaceRepository(ctrl), permissiveWebhookLogger(ctrl), nil)

		sub := &domain.WebhookSubscription{ID: "sub-1", URL: server.URL, Secret: testWebhookSecret, Enabled: true}

		// No ResetFailures armed.
		deliveryRepo.EXPECT().MarkDelivered(gomock.Any(), workspaceID, "delivery-1", http.StatusOK, "body").Return(nil)
		subRepo.EXPECT().UpdateLastDeliveryAt(gomock.Any(), workspaceID, "sub-1", gomock.Any()).Return(nil)

		worker.processDelivery(context.Background(), workspaceID, newDelivery(), sub)
	})

	t.Run("a 5xx past the threshold disables and drains rather than retrying", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		server := respondWith(t, http.StatusInternalServerError)
		subRepo := mocks.NewMockWebhookSubscriptionRepository(ctrl)
		deliveryRepo := mocks.NewMockWebhookDeliveryRepository(ctrl)
		worker := NewWebhookDeliveryWorker(subRepo, deliveryRepo, mocks.NewMockWorkspaceRepository(ctrl), permissiveWebhookLogger(ctrl), nil)
		worker.failureThreshold = 2

		sub := &domain.WebhookSubscription{
			ID: "sub-1", URL: server.URL, Secret: testWebhookSecret, Enabled: true,
			ConsecutiveFailures: 1,
		}

		subRepo.EXPECT().IncrementFailures(gomock.Any(), workspaceID, "sub-1").Return(nil)
		subRepo.EXPECT().DisableWithReason(gomock.Any(), workspaceID, "sub-1", gomock.Any()).Return(nil)
		// Scheduling a retry instead would have the row claimed once more for a
		// subscription that is now switched off, only to be drained then.
		deliveryRepo.EXPECT().MarkFailed(gomock.Any(), workspaceID, "delivery-1", 10, gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

		worker.processDelivery(context.Background(), workspaceID, newDelivery(), sub)
	})

	// A failed disable must not be reported as a disable, or the row is drained
	// while the subscription keeps firing.
	t.Run("a failed disable falls back to a retry", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		server := respondWith(t, http.StatusInternalServerError)
		subRepo := mocks.NewMockWebhookSubscriptionRepository(ctrl)
		deliveryRepo := mocks.NewMockWebhookDeliveryRepository(ctrl)
		worker := NewWebhookDeliveryWorker(subRepo, deliveryRepo, mocks.NewMockWorkspaceRepository(ctrl), permissiveWebhookLogger(ctrl), nil)
		worker.failureThreshold = 1

		sub := &domain.WebhookSubscription{ID: "sub-1", URL: server.URL, Secret: testWebhookSecret, Enabled: true}

		subRepo.EXPECT().IncrementFailures(gomock.Any(), workspaceID, "sub-1").Return(nil)
		subRepo.EXPECT().DisableWithReason(gomock.Any(), workspaceID, "sub-1", gomock.Any()).
			Return(errors.New("database error"))
		deliveryRepo.EXPECT().ScheduleRetry(gomock.Any(), workspaceID, "delivery-1", gomock.Any(), 1, gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

		worker.processDelivery(context.Background(), workspaceID, newDelivery(), sub)
		assert.True(t, sub.Enabled)
	})
}

// TestWebhookDeliveryWorker_drainsResponseBody covers both halves of the limited
// read: what is stored, and what happens to the bytes that are not.
func TestWebhookDeliveryWorker_drainsResponseBody(t *testing.T) {
	const workspaceID = "ws-1"

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Far more than the kilobyte the delivery log keeps, so the connection can
	// only be reused if the remainder is drained: closing a body with unread
	// bytes makes Go's client throw the connection away and pay a fresh TCP
	// connect and TLS handshake for the next delivery.
	oversizedBody := strings.Repeat("x", 64*1024)

	var remoteAddrs []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		remoteAddrs = append(remoteAddrs, r.RemoteAddr)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(oversizedBody))
	}))
	defer server.Close()

	subRepo := mocks.NewMockWebhookSubscriptionRepository(ctrl)
	deliveryRepo := mocks.NewMockWebhookDeliveryRepository(ctrl)
	worker := NewWebhookDeliveryWorker(subRepo, deliveryRepo, mocks.NewMockWorkspaceRepository(ctrl), permissiveWebhookLogger(ctrl), nil)

	sub := &domain.WebhookSubscription{ID: "sub-1", URL: server.URL, Secret: testWebhookSecret, Enabled: true}

	var storedBodies []string
	subRepo.EXPECT().IncrementFailures(gomock.Any(), workspaceID, "sub-1").Return(nil).Times(2)
	deliveryRepo.EXPECT().ScheduleRetry(gomock.Any(), workspaceID, gomock.Any(), gomock.Any(), 1, gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ string, _ time.Time, _ int, _ *int, body, _ *string) error {
			storedBodies = append(storedBodies, *body)
			return nil
		}).Times(2)

	for _, id := range []string{"delivery-1", "delivery-2"} {
		worker.processDelivery(context.Background(), workspaceID, &domain.WebhookDelivery{
			ID: id, SubscriptionID: "sub-1", EventType: "contact.created",
			Payload: map[string]interface{}{"email": "test@example.com"}, MaxAttempts: 10,
		}, sub)
	}

	require.Len(t, storedBodies, 2)
	for _, body := range storedBodies {
		assert.Len(t, body, 1024, "only the first kilobyte belongs in the delivery log")
	}

	require.Len(t, remoteAddrs, 2)
	assert.Equal(t, remoteAddrs[0], remoteAddrs[1],
		"the second delivery should reuse the keep-alive connection")
}

// TestWebhookRetryLadder walks the rungs a delivery can actually reach.
//
// The ladder reads as ten rungs over about 34 hours, and for a row the triggers
// wrote it is nine rungs over about 9h53m: MaxAttempts is 10, the permanent
// failure fires at attempts >= MaxAttempts, and the index is attempts-1. The
// last entry is reachable only for a row carrying a larger per-row ceiling,
// which is why it is still in the table.
func TestWebhookRetryLadder(t *testing.T) {
	reachable := []time.Duration{
		30 * time.Second,
		1 * time.Minute,
		2 * time.Minute,
		5 * time.Minute,
		15 * time.Minute,
		30 * time.Minute,
		1 * time.Hour,
		2 * time.Hour,
		6 * time.Hour,
	}

	const maxAttempts = 10
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	for priorAttempts, want := range reachable {
		deliveryRepo := mocks.NewMockWebhookDeliveryRepository(ctrl)
		worker := NewWebhookDeliveryWorker(
			mocks.NewMockWebhookSubscriptionRepository(ctrl), deliveryRepo,
			mocks.NewMockWorkspaceRepository(ctrl), permissiveWebhookLogger(ctrl), nil)

		var scheduled time.Time
		deliveryRepo.EXPECT().ScheduleRetry(gomock.Any(), "ws-1", "delivery-1", gomock.Any(), priorAttempts+1, gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _, _ string, nextAttempt time.Time, _ int, _ *int, _, _ *string) error {
				scheduled = nextAttempt
				return nil
			})

		before := time.Now().UTC()
		worker.handleDeliveryFailure(context.Background(), "ws-1",
			&domain.WebhookDelivery{ID: "delivery-1", Attempts: priorAttempts, MaxAttempts: maxAttempts},
			&domain.WebhookSubscription{ID: "sub-1"}, nil, "", "HTTP 500")

		delay := scheduled.Sub(before)
		assert.InDelta(t, want.Seconds(), delay.Seconds(), 2,
			"rung %d of the ladder", priorAttempts)
	}

	// The rung after the last reachable one is where the row is given up on
	// instead, which is what makes retryDelays[9] unreachable at this ceiling.
	deliveryRepo := mocks.NewMockWebhookDeliveryRepository(ctrl)
	worker := NewWebhookDeliveryWorker(
		mocks.NewMockWebhookSubscriptionRepository(ctrl), deliveryRepo,
		mocks.NewMockWorkspaceRepository(ctrl), permissiveWebhookLogger(ctrl), nil)
	deliveryRepo.EXPECT().MarkFailed(gomock.Any(), "ws-1", "delivery-1", maxAttempts, "HTTP 500", nil, gomock.Any()).Return(nil)
	worker.handleDeliveryFailure(context.Background(), "ws-1",
		&domain.WebhookDelivery{ID: "delivery-1", Attempts: len(reachable), MaxAttempts: maxAttempts},
		&domain.WebhookSubscription{ID: "sub-1"}, nil, "", "HTTP 500")
}

// TestBuildTestPayload pins the test payload to the shapes the PL/pgSQL triggers
// actually build.
//
// The payload is assembled inside PostgreSQL, so nothing in Go fails to compile
// when a trigger changes. Before this, the function invented `subject`, `url`,
// `bounce_type`, `contact_id`, `tags` and `id` — so the console's Test button
// taught a payload shape that never arrives, and a Zapier app deriving its
// sample records from it would publish those fields to every install.
func TestBuildTestPayload(t *testing.T) {
	keysOf := func(payload map[string]interface{}) []string {
		keys := make([]string, 0, len(payload))
		for key := range payload {
			keys = append(keys, key)
		}
		return keys
	}

	// webhook_contact_lists_trigger, internal/database/init.go.
	listKeys := []string{"email", "list_id", "list_name", "status", "previous_status"}
	// webhook_contact_segments_trigger.
	segmentKeys := []string{"email", "segment_id", "segment_name"}
	// webhook_message_history_trigger.
	emailKeys := []string{"email", "message_id", "template_id", "broadcast_id", "list_id", "channel", "event_timestamp"}

	cases := []struct {
		eventType string
		keys      []string
	}{
		// Both to_jsonb() triggers wrap the whole row under a single key.
		{"contact.created", []string{"contact"}},
		{"contact.updated", []string{"contact"}},
		{"contact.deleted", []string{"contact"}},
		{"list.subscribed", listKeys},
		{"list.confirmed", listKeys},
		{"list.unsubscribed", listKeys},
		{"list.removed", listKeys},
		{"segment.joined", segmentKeys},
		{"segment.left", segmentKeys},
		{"email.sent", emailKeys},
		{"email.clicked", emailKeys},
		{"email.bounced", emailKeys},
		{"custom_event.created", []string{"custom_event"}},
		{"custom_event.deleted", []string{"custom_event"}},
	}

	for _, tc := range cases {
		t.Run(tc.eventType, func(t *testing.T) {
			assert.ElementsMatch(t, tc.keys, keysOf(buildTestPayload(tc.eventType)))
		})
	}

	t.Run("the contact object carries every contacts column", func(t *testing.T) {
		contact, ok := buildTestPayload("contact.created")["contact"].(map[string]interface{})
		require.True(t, ok)

		// to_jsonb(contact_record) emits one key per column, unset ones present
		// and null — so a column missing here is a field a user cannot map.
		expected := []string{
			"email", "external_id", "timezone", "language",
			"first_name", "last_name", "full_name", "phone",
			"address_line_1", "address_line_2", "country", "postcode", "state", "job_title",
			"custom_string_1", "custom_string_2", "custom_string_3", "custom_string_4", "custom_string_5",
			"custom_number_1", "custom_number_2", "custom_number_3", "custom_number_4", "custom_number_5",
			"custom_datetime_1", "custom_datetime_2", "custom_datetime_3", "custom_datetime_4", "custom_datetime_5",
			"custom_json_1", "custom_json_2", "custom_json_3", "custom_json_4", "custom_json_5",
			"created_at", "updated_at", "db_created_at", "db_updated_at",
		}
		assert.ElementsMatch(t, expected, keysOf(contact))
	})

	t.Run("the custom_event object carries every custom_events column", func(t *testing.T) {
		event, ok := buildTestPayload("custom_event.created")["custom_event"].(map[string]interface{})
		require.True(t, ok)

		expected := []string{
			"event_name", "external_id", "email", "properties", "occurred_at",
			"source", "integration_id", "goal_name", "goal_type", "goal_value",
			"deleted_at", "created_at", "updated_at",
		}
		assert.ElementsMatch(t, expected, keysOf(event))
	})

	// The trigger derives the event kind from the transition, so the pair is not
	// free: list.confirmed can only come from pending, and a status inserted
	// directly has no previous one.
	t.Run("previous_status matches the transition that produced the event", func(t *testing.T) {
		confirmed := buildTestPayload("list.confirmed")
		assert.Equal(t, "active", confirmed["status"])
		assert.Equal(t, "pending", confirmed["previous_status"])

		resubscribed := buildTestPayload("list.resubscribed")
		assert.Equal(t, "active", resubscribed["status"])
		assert.Equal(t, "unsubscribed", resubscribed["previous_status"])

		subscribed := buildTestPayload("list.subscribed")
		assert.Equal(t, "active", subscribed["status"])
		require.Contains(t, subscribed, "previous_status",
			"the trigger always builds the key, null included")
		assert.Nil(t, subscribed["previous_status"])
	})

	t.Run("an unrecognised event type says so instead of inventing a shape", func(t *testing.T) {
		payload := buildTestPayload("test")
		assert.ElementsMatch(t, []string{"message", "event_type", "created_at"}, keysOf(payload))
	})
}
