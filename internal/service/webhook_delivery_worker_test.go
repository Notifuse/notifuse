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
		// Both auto-disable durations, because an unassigned one is not an
		// obviously broken worker: a zero failureRunMaxAge makes every failure
		// look like the start of a new run, so the counter never climbs and no
		// endpoint is ever retired.
		assert.Equal(t, webhookFailureWindow, worker.failureWindow)
		assert.Equal(t, webhookFailureRunMaxAge, worker.failureRunMaxAge)
		// Derived from the client that was passed, so raising the timeout cannot
		// leave the reclaim sweep short of it.
		assert.Equal(t, 50*time.Second, worker.claimLease)
		// And the request keeps the operator's own ceiling rather than being
		// stretched to the lease: the lease bounds the request, it does not license
		// a longer one.
		assert.Equal(t, 45*time.Second, worker.requestTimeout)
	})

	t.Run("creates worker with default HTTP client when nil provided", func(t *testing.T) {
		worker := NewWebhookDeliveryWorker(mockSubRepo, mockDeliveryRepo, mockWorkspaceRepo, mockLogger, nil)

		assert.NotNil(t, worker)
		assert.NotNil(t, worker.httpClient)
		assert.Equal(t, 30*time.Second, worker.httpClient.Timeout)
		assert.Equal(t, 35*time.Second, worker.claimLease)
		assert.Equal(t, 30*time.Second, worker.requestTimeout)
	})

	t.Run("options override the timings they name and nothing else", func(t *testing.T) {
		worker := NewWebhookDeliveryWorker(mockSubRepo, mockDeliveryRepo, mockWorkspaceRepo, mockLogger, nil,
			WithWebhookPollInterval(50*time.Millisecond),
			WithWebhookClaimLease(2*time.Second))

		assert.Equal(t, 50*time.Millisecond, worker.pollInterval)
		// The lease is derived from the client's timeout by default, so an explicit
		// one is only injectable if the options run after that derivation.
		assert.Equal(t, 2*time.Second, worker.claimLease)

		// READ THIS BEFORE "RESTORING" ANYTHING HERE. The two lines above used to
		// stand next to a comment calling a shortened lease "exactly the
		// combination that turns the sweep into a duplicate factory" — while
		// asserting that the combination was installed. It pinned the defect as
		// correct, so the fix for it arrived looking like the regression. The
		// assertion below is the half that was missing: a short lease is fine
		// BECAUSE the request is shortened to fit inside it. Delete that and this
		// subtest goes back to guarding the bug.
		assert.Less(t, worker.requestTimeout, worker.claimLease,
			"a request allowed to outlive its claim is delivered twice: the sweep returns the row mid-POST")
		assert.Positive(t, worker.requestTimeout, "and a request with no time at all delivers nothing")

		// Everything the options did not name keeps its production value.
		assert.Equal(t, 100, worker.batchSize)
		assert.Equal(t, 1*time.Hour, worker.cleanupInterval)
		assert.Equal(t, 7, worker.retentionDays)
		assert.Equal(t, webhookFailureThreshold, worker.failureThreshold)
		assert.Equal(t, webhookFailureWindow, worker.failureWindow)
		assert.Equal(t, webhookFailureRunMaxAge, worker.failureRunMaxAge)
	})

	t.Run("a non-positive timing is refused rather than installed", func(t *testing.T) {
		worker := NewWebhookDeliveryWorker(mockSubRepo, mockDeliveryRepo, mockWorkspaceRepo, mockLogger, nil,
			WithWebhookPollInterval(0),
			WithWebhookClaimLease(-time.Second))

		// A zero interval panics time.NewTicker, on a goroutine nothing above it
		// recovers, and a zero lease makes every claimed row instantly stale — so
		// both fall back to the default rather than reach Start.
		assert.Equal(t, 10*time.Second, worker.pollInterval)
		assert.Equal(t, 35*time.Second, worker.claimLease)
		assert.Equal(t, 30*time.Second, worker.requestTimeout)
	})
}

// TestWebhookDeliveryWorker_theRequestNeverOutlivesTheClaim walks the
// combinations a caller can actually build and asserts the one property that
// makes the reclaim sweep safe: the POST is bounded, and its bound is strictly
// inside the lease that authorises it.
//
// Break it and the failure is silent on both sides. The sweep returns the row to
// 'pending' with a request still open, a second worker claims and delivers it,
// and the subscriber has the event twice — while the row ends 'delivered' with a
// single attempt, because MarkDelivered's UPDATE carries no claim predicate.
// Nothing in the delivery log records the duplicate.
func TestWebhookDeliveryWorker_theRequestNeverOutlivesTheClaim(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	build := func(client *http.Client, opts ...WebhookDeliveryWorkerOption) *WebhookDeliveryWorker {
		return NewWebhookDeliveryWorker(
			mocks.NewMockWebhookSubscriptionRepository(ctrl),
			mocks.NewMockWebhookDeliveryRepository(ctrl),
			mocks.NewMockWorkspaceRepository(ctrl),
			permissiveWebhookLogger(ctrl),
			client, opts...)
	}

	cases := map[string]*WebhookDeliveryWorker{
		"production's client, no overrides": build(&http.Client{Timeout: 10 * time.Second}),
		"the nil-client fallback":           build(nil),
		// The combination the option made installable: a lease five times shorter
		// than the request timeout, accepted because the option is applied after
		// the lease is derived and guarded only against a non-positive value.
		"a lease far shorter than the client timeout": build(
			&http.Client{Timeout: 10 * time.Second}, WithWebhookClaimLease(2*time.Second)),
		// Sub-second, which is what an integration case needs to drive the sweep
		// without waiting out a real lease.
		"a lease shorter than the slack a long lease gets": build(
			&http.Client{Timeout: 5 * time.Second}, WithWebhookClaimLease(1200*time.Millisecond)),
		// In net/http a zero Timeout is no timeout at all, so this is the case where
		// an unbounded request would run under a bounded lease. Nothing about the
		// client can fix it; the lease has to.
		"a client that never times out": build(&http.Client{}),
	}

	for name, worker := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Positive(t, worker.requestTimeout,
				"a delivery with no time to run never leaves the process")
			assert.Less(t, worker.requestTimeout, worker.claimLease,
				"the response, and the write that records it, both have to land inside the claim")
			if worker.httpClient.Timeout > 0 {
				assert.LessOrEqual(t, worker.requestTimeout, worker.httpClient.Timeout,
					"the client's timeout is the operator's ceiling; the lease may lower it, never raise it")
			}
		})
	}
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
	expectClaimRenewal(mockDeliveryRepo)

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

		// Back to pending through ReleaseClaim, never UpdateStatus: a pool
		// exhaustion says nothing about the endpoint, so it must spend no
		// attempt AND leave the delivery log alone. UpdateStatus writes
		// last_attempt_at, last_response_status and last_response_body on every
		// call, so releasing through it stamped an attempt that never happened
		// and erased the receiver's last real status and body.
		mockDeliveryRepo.EXPECT().ReleaseClaim(ctx, workspaceID, "delivery1",
			gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _, _ string, claimedAt *time.Time, lastError string) error {
				assert.Contains(t, lastError, "too many clients")
				// The release carries the claim it is giving back, so it cannot
				// land on a row this worker no longer holds.
				assert.NotNil(t, claimedAt, "a release without its claim token releases nothing")
				return nil
			})

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

		worker.processDelivery(ctx, workspaceID, delivery, subscription, time.Now())
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

		worker.processDelivery(ctx, workspaceID, delivery, subscription, time.Now())
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

		worker.processDelivery(ctx, workspaceID, delivery, subscription, time.Now())
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

		worker.processDelivery(ctx, workspaceID, delivery, subscription, time.Now())
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

		worker.processDelivery(ctx, workspaceID, delivery, subscription, time.Now())
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
			worker.processDelivery(ctx, workspaceID, delivery, subscription, time.Now())

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

// RenewClaim mirrors the repository's renewal, token and all: it matches on the
// claimed_at the caller was handed, so a row the sweep returned and someone else
// re-claimed answers false instead of being quietly stolen back.
func (f *fakeDeliveryStore) RenewClaim(_ context.Context, _, id string, claimedAt *time.Time) (bool, *time.Time, error) {
	row, ok := f.rows[id]
	if !ok || claimedAt == nil {
		return false, nil, nil
	}
	if row.Status != domain.WebhookDeliveryStatusDelivering {
		return false, nil, nil
	}
	if row.ClaimedAt == nil || !row.ClaimedAt.Equal(*claimedAt) {
		return false, nil, nil
	}
	renewed := f.now
	row.ClaimedAt = &renewed
	return true, &renewed, nil
}

// ReleaseClaim mirrors the repository's release, ownership predicate included:
// status and claimed_at move, last_error is recorded, the delivery log's own
// fields are left alone — and a row this caller no longer holds is left exactly
// as its owner left it. Without the predicate here the fake would happily
// resurrect a delivered row, which is the bug this mirrors away.
func (f *fakeDeliveryStore) ReleaseClaim(_ context.Context, _, id string, claimedAt *time.Time, lastError string) error {
	row, ok := f.rows[id]
	if !ok || claimedAt == nil {
		return nil
	}
	if row.Status != domain.WebhookDeliveryStatusDelivering {
		return nil
	}
	if row.ClaimedAt == nil || !row.ClaimedAt.Equal(*claimedAt) {
		return nil
	}
	row.Status = domain.WebhookDeliveryStatusPending
	row.ClaimedAt = nil
	row.LastError = &lastError
	return nil
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

// failingFor builds the FailingSince a subscription would carry after `d` of
// back-to-back failures. Every auto-disable case needs one: the threshold is
// only half the rule, and a run of failures that started a moment ago never
// retires an endpoint however many deliveries it swallowed.
func failingFor(d time.Duration) *time.Time {
	started := time.Now().UTC().Add(-d)
	return &started
}

// failingLongEnoughToRetire dates a run that is old enough to have opened the
// window and young enough to still be the same run. That interval is the ONLY
// one in which a subscription can be retired at all, and it is bounded at both
// ends by constants that have moved: these cases used to name a literal thirteen
// hours, which sat inside the old window and outside the run's maximum age, so
// the same literal would have quietly stopped meaning "retirable" the moment
// either number changed. Derived from both, it cannot.
func failingLongEnoughToRetire() *time.Time {
	return failingFor(retirableRunAge)
}

// retirableRunAge is the midpoint of that interval.
var retirableRunAge = webhookFailureWindow + (webhookFailureRunMaxAge-webhookFailureWindow)/2

// expectClaimRenewal arms the per-row lease renewal the loop now performs before
// it writes to a row or puts bytes on the wire for it. Tests whose subject IS
// the renewal arm it themselves and assert on it; this is for the many whose
// subject is elsewhere.
func expectClaimRenewal(repo *mocks.MockWebhookDeliveryRepository) {
	repo.EXPECT().RenewClaim(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ string, _ *time.Time) (bool, *time.Time, error) {
			renewed := time.Now().UTC()
			return true, &renewed, nil
		}).AnyTimes()
}

// respondWith serves one fixed status to every request.
func respondWith(t *testing.T, status int) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte("body"))
	}))
	t.Cleanup(server.Close)
	return server
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

		worker.processDelivery(context.Background(), workspaceID, newDelivery(), sub, time.Now())
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

		worker.processDelivery(context.Background(), workspaceID, newDelivery(), sub, time.Now())

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

		worker.processDelivery(context.Background(), workspaceID, newDelivery(), sub, time.Now())
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

		worker.processDelivery(context.Background(), workspaceID, newDelivery(), sub, time.Now())
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
			// Sustained means sustained: this endpoint has been answering 404
			// for longer than a delivery's whole retry ladder can run.
			FailingSince: failingLongEnoughToRetire(),
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

		worker.processDelivery(context.Background(), workspaceID, newDelivery(), sub, time.Now())

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

		worker.processDelivery(context.Background(), workspaceID, newDelivery(), sub, time.Now())

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

		worker.processDelivery(context.Background(), workspaceID, newDelivery(), sub, time.Now())
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

		worker.processDelivery(context.Background(), workspaceID, newDelivery(), sub, time.Now())
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
			FailingSince:        failingLongEnoughToRetire(),
		}

		subRepo.EXPECT().IncrementFailures(gomock.Any(), workspaceID, "sub-1").Return(nil)
		subRepo.EXPECT().DisableWithReason(gomock.Any(), workspaceID, "sub-1", gomock.Any()).Return(nil)
		// Scheduling a retry instead would have the row claimed once more for a
		// subscription that is now switched off, only to be drained then.
		deliveryRepo.EXPECT().MarkFailed(gomock.Any(), workspaceID, "delivery-1", 10, gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

		worker.processDelivery(context.Background(), workspaceID, newDelivery(), sub, time.Now())
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

		sub := &domain.WebhookSubscription{
			ID: "sub-1", URL: server.URL, Secret: testWebhookSecret, Enabled: true,
			FailingSince: failingLongEnoughToRetire(),
		}

		subRepo.EXPECT().IncrementFailures(gomock.Any(), workspaceID, "sub-1").Return(nil)
		subRepo.EXPECT().DisableWithReason(gomock.Any(), workspaceID, "sub-1", gomock.Any()).
			Return(errors.New("database error"))
		deliveryRepo.EXPECT().ScheduleRetry(gomock.Any(), workspaceID, "delivery-1", gomock.Any(), 1, gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

		worker.processDelivery(context.Background(), workspaceID, newDelivery(), sub, time.Now())
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
		}, sub, time.Now())
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

// TestWebhookDeliveryWorker_neverDeliversARowItNoLongerHolds is the duplicate
// the claim exists to prevent, arriving through the claim's own reaper.
//
// The claim stamps claimed_at ONCE, for a whole batch of up to a hundred rows,
// and the worker then walks them serially with a ten-second ceiling per POST. So
// the binding constraint on the lease is batchSize x httpTimeout, not
// httpTimeout — and against a fifteen-second lease the sweep starts returning
// rows this worker is still holding within the first two deliveries. Whoever
// picks them up delivers them, and then this worker reaches them and delivers
// them again: a second Salesforce record, a second Slack message, a second
// charge.
//
// The fix is to renew the claim on each row immediately before its POST, keyed
// on the claimed_at this worker was handed, so a row that has moved on is left
// to its new owner. This drives exactly that: the receiver reclaims the second
// row while the first is being delivered.
func TestWebhookDeliveryWorker_neverDeliversARowItNoLongerHolds(t *testing.T) {
	const workspaceID = "ws-1"

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	pending := func(id string) *domain.WebhookDelivery {
		return &domain.WebhookDelivery{
			ID:             id,
			SubscriptionID: "sub-1",
			EventType:      "contact.created",
			Payload:        map[string]interface{}{"email": "a@b.c"},
			Status:         domain.WebhookDeliveryStatusPending,
			MaxAttempts:    10,
			NextAttemptAt:  time.Now().UTC().Add(-time.Minute),
		}
	}

	store := newFakeDeliveryStore(pending("delivery-1"), pending("delivery-2"))

	var delivered []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		delivered = append(delivered, r.Header.Get("webhook-id"))

		// While this delivery is in flight, the lease on the rest of the batch
		// runs out: the sweep returns delivery-2 to 'pending' and a second
		// worker claims it. Its claimed_at has moved, so the token this worker
		// holds is stale.
		if row, ok := store.rows["delivery-2"]; ok && row.Status == domain.WebhookDeliveryStatusDelivering {
			stolen := store.now.Add(time.Second)
			row.ClaimedAt = &stolen
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	subRepo := mocks.NewMockWebhookSubscriptionRepository(ctrl)
	subRepo.EXPECT().GetByID(gomock.Any(), workspaceID, "sub-1").
		Return(&domain.WebhookSubscription{
			ID: "sub-1", URL: server.URL, Secret: testWebhookSecret, Enabled: true,
		}, nil).AnyTimes()
	subRepo.EXPECT().UpdateLastDeliveryAt(gomock.Any(), workspaceID, "sub-1", gomock.Any()).
		Return(nil).AnyTimes()

	worker := NewWebhookDeliveryWorker(subRepo, store, mocks.NewMockWorkspaceRepository(ctrl),
		permissiveWebhookLogger(ctrl), &http.Client{Timeout: 10 * time.Second})

	require.NoError(t, worker.processWorkspaceDeliveries(context.Background(), workspaceID))

	require.Equal(t, []string{"delivery-1"}, delivered,
		"a row whose claim was reclaimed mid-batch must not be POSTed by the worker that lost it")

	// And the row is left exactly as its new owner found it: no status write, no
	// attempt spent, no terminal state. Writing to it here would stamp over
	// whatever the worker that now holds it is doing.
	stolen := store.row(t, "delivery-2")
	assert.Equal(t, domain.WebhookDeliveryStatusDelivering, stolen.Status)
	assert.Equal(t, 0, stolen.Attempts)
	assert.Nil(t, stolen.LastError)
}

// TestWebhookDeliveryWorker_aBurstDoesNotRetireAHealthyEndpoint is the other half
// of the auto-disable rule: the threshold counts deliveries, and deliveries are
// not time.
//
// A hundred rows go out per poll, so a receiver that is restarting for thirty
// seconds during an import fails twenty of them inside one ten-second poll. On
// the count alone that clears the threshold, the subscription is switched off,
// and — because a disabled subscription drains its queue — every remaining
// delivery is marked terminally failed and never retried, for an endpoint that
// is healthy again a minute later.
func TestWebhookDeliveryWorker_aBurstDoesNotRetireAHealthyEndpoint(t *testing.T) {
	const workspaceID = "ws-1"

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	const queued = 25

	rows := make([]*domain.WebhookDelivery, 0, queued)
	for i := 0; i < queued; i++ {
		rows = append(rows, &domain.WebhookDelivery{
			ID:             fmt.Sprintf("delivery-%02d", i),
			SubscriptionID: "sub-1",
			EventType:      "contact.created",
			Payload:        map[string]interface{}{"email": "a@b.c"},
			Status:         domain.WebhookDeliveryStatusPending,
			MaxAttempts:    10,
			NextAttemptAt:  time.Now().UTC().Add(-time.Minute),
		})
	}
	store := newFakeDeliveryStore(rows...)

	// The receiver is mid-restart: every delivery in this burst fails.
	server := respondWith(t, http.StatusInternalServerError)

	sub := &domain.WebhookSubscription{
		ID: "sub-1", URL: server.URL, Secret: testWebhookSecret, Enabled: true,
	}

	subRepo := mocks.NewMockWebhookSubscriptionRepository(ctrl)
	subRepo.EXPECT().GetByID(gomock.Any(), workspaceID, "sub-1").Return(sub, nil).AnyTimes()
	subRepo.EXPECT().IncrementFailures(gomock.Any(), workspaceID, "sub-1").Return(nil).AnyTimes()
	// Nothing arms DisableWithReason. Reaching it in a single poll is the bug.

	worker := NewWebhookDeliveryWorker(subRepo, store, mocks.NewMockWorkspaceRepository(ctrl),
		permissiveWebhookLogger(ctrl), &http.Client{Timeout: 10 * time.Second})
	// Production's threshold, unmodified: the point is that twenty failures are
	// reachable inside one batch, which is why the count cannot be the whole rule.
	require.Equal(t, 20, worker.failureThreshold)
	require.Equal(t, 2*time.Hour, worker.failureWindow)

	require.NoError(t, worker.processWorkspaceDeliveries(context.Background(), workspaceID))

	assert.True(t, sub.Enabled,
		"twenty failures inside one ten-second poll are not a sustained outage")

	// Every row is rescheduled rather than drained, so the endpoint coming back
	// a minute later delivers all of them.
	for _, row := range rows {
		stored := store.row(t, row.ID)
		assert.Equal(t, domain.WebhookDeliveryStatusFailed, stored.Status, row.ID)
		assert.Less(t, stored.Attempts, stored.MaxAttempts,
			"%s was drained: it can never be retried", row.ID)
		assert.True(t, stored.NextAttemptAt.After(time.Now().UTC()),
			"%s must be scheduled for a later attempt", row.ID)
	}

	// And the counter did start running: the same endpoint still failing twelve
	// hours from now is retired, which is what the sweep is for.
	require.NotNil(t, sub.FailingSince, "the run of failures has to be dated for the window to mean anything")
	assert.WithinDuration(t, time.Now().UTC(), *sub.FailingSince, time.Minute)
}

// The window is the only thing between the two outcomes: identical counts,
// identical responses, different answers.
func TestWebhookDeliveryWorker_retiresAnEndpointOnlyOnceTheFailuresPersist(t *testing.T) {
	const workspaceID = "ws-1"

	run := func(t *testing.T, failingFor time.Duration) (*domain.WebhookSubscription, bool) {
		t.Helper()
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		server := respondWith(t, http.StatusInternalServerError)
		subRepo := mocks.NewMockWebhookSubscriptionRepository(ctrl)
		deliveryRepo := mocks.NewMockWebhookDeliveryRepository(ctrl)
		worker := NewWebhookDeliveryWorker(subRepo, deliveryRepo,
			mocks.NewMockWorkspaceRepository(ctrl), permissiveWebhookLogger(ctrl), nil)
		worker.failureThreshold = 3

		startedAt := time.Now().UTC().Add(-failingFor)
		sub := &domain.WebhookSubscription{
			ID: "sub-1", URL: server.URL, Secret: testWebhookSecret, Enabled: true,
			ConsecutiveFailures: 2,
			FailingSince:        &startedAt,
		}

		subRepo.EXPECT().IncrementFailures(gomock.Any(), workspaceID, "sub-1").Return(nil)

		disabled := false
		subRepo.EXPECT().DisableWithReason(gomock.Any(), workspaceID, "sub-1", gomock.Any()).
			DoAndReturn(func(_ context.Context, _, _, _ string) error {
				disabled = true
				return nil
			}).AnyTimes()
		deliveryRepo.EXPECT().MarkFailed(gomock.Any(), workspaceID, "delivery-1", 10,
			gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
		deliveryRepo.EXPECT().ScheduleRetry(gomock.Any(), workspaceID, "delivery-1",
			gomock.Any(), 1, gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

		worker.processDelivery(context.Background(), workspaceID, &domain.WebhookDelivery{
			ID: "delivery-1", SubscriptionID: "sub-1", EventType: "contact.created",
			Payload: map[string]interface{}{"email": "a@b.c"}, MaxAttempts: 10,
		}, sub, time.Now())

		return sub, disabled
	}

	t.Run("an hour of failures is a bad hour, not a dead endpoint", func(t *testing.T) {
		sub, disabled := run(t, time.Hour)
		assert.False(t, disabled)
		assert.True(t, sub.Enabled)
		assert.Equal(t, 3, sub.ConsecutiveFailures, "the count still moves; only the action waits")
	})

	t.Run("past the window the same count retires it", func(t *testing.T) {
		sub, disabled := run(t, retirableRunAge)
		assert.True(t, disabled)
		assert.False(t, sub.Enabled)
	})
}

// The reason describes the run of failures, not whichever response happened to
// land last. Attributing the cause to the final status labelled a subscription
// retired by nineteen 500s as a 404 problem, pointing the user at a URL that was
// never wrong.
func TestWebhookDeliveryWorker_disableReasonDoesNotBlameTheLastStatus(t *testing.T) {
	const workspaceID = "ws-1"

	reasonFor := func(t *testing.T, status int) string {
		t.Helper()
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		server := respondWith(t, status)
		subRepo := mocks.NewMockWebhookSubscriptionRepository(ctrl)
		deliveryRepo := mocks.NewMockWebhookDeliveryRepository(ctrl)
		worker := NewWebhookDeliveryWorker(subRepo, deliveryRepo,
			mocks.NewMockWorkspaceRepository(ctrl), permissiveWebhookLogger(ctrl), nil)
		worker.failureThreshold = 1

		sub := &domain.WebhookSubscription{
			ID: "sub-1", URL: server.URL, Secret: testWebhookSecret, Enabled: true,
			FailingSince: failingLongEnoughToRetire(),
		}

		var reason string
		subRepo.EXPECT().IncrementFailures(gomock.Any(), workspaceID, "sub-1").Return(nil)
		subRepo.EXPECT().DisableWithReason(gomock.Any(), workspaceID, "sub-1", gomock.Any()).
			DoAndReturn(func(_ context.Context, _, _, r string) error {
				reason = r
				return nil
			})
		deliveryRepo.EXPECT().MarkFailed(gomock.Any(), workspaceID, "delivery-1", 10,
			gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

		worker.processDelivery(context.Background(), workspaceID, &domain.WebhookDelivery{
			ID: "delivery-1", SubscriptionID: "sub-1", EventType: "contact.created",
			Payload: map[string]interface{}{"email": "a@b.c"}, MaxAttempts: 10,
		}, sub, time.Now())
		return reason
	}

	notFound := reasonFor(t, http.StatusNotFound)
	serverError := reasonFor(t, http.StatusInternalServerError)

	// Both name the persistence that actually retired the subscription...
	for _, reason := range []string{notFound, serverError} {
		assert.Contains(t, reason, "consecutive delivery failures")
		assert.Contains(t, reason, "more than 2 hours", "the reason has to say over what period")
	}

	// ...and report the last response as an observation rather than a diagnosis.
	assert.Contains(t, notFound, "most recent response: HTTP 404")
	assert.Contains(t, serverError, "most recent response: HTTP 500")
	assert.NotContains(t, notFound, "sustained HTTP 404",
		"one 404 at the end of a run of 500s is not evidence the URL is wrong")
}

// The two auto-disable numbers are not independent of the retry ladder, and the
// ladder is the thing that moves: a rung added or a delay lengthened silently
// re-decides both of them.
//
// webhookFailureWindow above the ladder is the failure that actually shipped. At
// twelve hours it exceeded the roughly 9h53m a queued delivery can be retried
// for, so every delivery waiting when an endpoint died was permanently failed
// before the window could open, and the only mechanism that retires a dead
// endpoint without being told it is dead depended entirely on new events still
// arriving two hours later — from a workspace that had usually gone quiet at the
// same moment the receiver was torn down.
//
// webhookFailureRunMaxAge below the ladder is the mirror mistake: it would end a
// genuine outage's run mid-ladder and restart the count from one while the
// endpoint was still dead.
func TestWebhookAutoDisableNumbersBracketTheRetryLadder(t *testing.T) {
	// What the PL/pgSQL webhook triggers insert into max_attempts.
	const maxAttempts = 10

	ladder := reachableRetryWindow(maxAttempts)
	assert.Equal(t, 9*time.Hour+53*time.Minute+30*time.Second, ladder,
		"the reachable ladder is nine rungs, not the ten the table lists")

	assert.Less(t, webhookFailureWindow, ladder,
		"a window longer than the ladder can only open after the whole backlog has been given up on")
	assert.Greater(t, webhookFailureRunMaxAge, ladder,
		"a run that expires before the ladder ends cuts a real outage in half and restarts its count")

	// The last attempt is followed by MarkFailed rather than by a delay, so a
	// one-attempt row waits for nothing, and a ceiling past the end of the table
	// cannot walk off it.
	assert.Zero(t, reachableRetryWindow(1))
	assert.Equal(t, reachableRetryWindow(len(retryDelays)+1), reachableRetryWindow(len(retryDelays)+50))
}

// A success clears the run of failures, and clearing it has to reach the copy
// the batch is holding.
//
// One *sub is cached for up to a hundred deliveries, so zeroing only the counter
// left that copy carrying a FailingSince from hours ago. The next failure then
// found it already set, left it alone, and the window it measured was the age of
// an outage that had ended — so a receiver that had just proved it was alive was
// retired by twenty seconds of a transient blip, and the disabled branch drained
// everything it still had queued.
func TestWebhookDeliveryWorker_aSuccessEndsTheRunForTheCachedSubscription(t *testing.T) {
	const workspaceID = "ws-1"

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		if requests == 1 {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
		_, _ = w.Write([]byte("body"))
	}))
	defer server.Close()

	subRepo := mocks.NewMockWebhookSubscriptionRepository(ctrl)
	deliveryRepo := mocks.NewMockWebhookDeliveryRepository(ctrl)
	worker := NewWebhookDeliveryWorker(subRepo, deliveryRepo,
		mocks.NewMockWorkspaceRepository(ctrl), permissiveWebhookLogger(ctrl), nil)
	// One failure is enough, so the only thing standing between this endpoint
	// and retirement is the window.
	worker.failureThreshold = 1

	// The subscription arrives carrying a run old enough to retire it.
	sub := &domain.WebhookSubscription{
		ID: "sub-1", URL: server.URL, Secret: testWebhookSecret, Enabled: true,
		ConsecutiveFailures: 5,
		FailingSince:        failingLongEnoughToRetire(),
	}

	subRepo.EXPECT().ResetFailures(gomock.Any(), workspaceID, "sub-1").Return(nil)
	subRepo.EXPECT().UpdateLastDeliveryAt(gomock.Any(), workspaceID, "sub-1", gomock.Any()).Return(nil)
	subRepo.EXPECT().IncrementFailures(gomock.Any(), workspaceID, "sub-1").Return(nil)
	deliveryRepo.EXPECT().MarkDelivered(gomock.Any(), workspaceID, "delivery-1", http.StatusOK, gomock.Any()).Return(nil)
	deliveryRepo.EXPECT().ScheduleRetry(gomock.Any(), workspaceID, "delivery-2",
		gomock.Any(), 1, gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	deliveryRepo.EXPECT().MarkFailed(gomock.Any(), workspaceID, "delivery-2",
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	disabled := false
	subRepo.EXPECT().DisableWithReason(gomock.Any(), workspaceID, "sub-1", gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _, _ string) error {
			disabled = true
			return nil
		}).AnyTimes()

	delivery := func(id string) *domain.WebhookDelivery {
		return &domain.WebhookDelivery{
			ID: id, SubscriptionID: "sub-1", EventType: "contact.created",
			Payload: map[string]interface{}{"email": "a@b.c"}, MaxAttempts: 10,
		}
	}

	// The endpoint answers, which ends the run...
	worker.processDelivery(context.Background(), workspaceID, delivery("delivery-1"), sub, time.Now())
	require.Nil(t, sub.FailingSince, "a success has to clear the cached window as well as the cached count")

	// ...and the blip that follows starts a new one, on the same cached object.
	worker.processDelivery(context.Background(), workspaceID, delivery("delivery-2"), sub, time.Now())

	assert.False(t, disabled,
		"an endpoint that answered a moment ago must not be retired by the next failure")
	assert.True(t, sub.Enabled)
	require.NotNil(t, sub.FailingSince)
	assert.WithinDuration(t, time.Now().UTC(), *sub.FailingSince, time.Minute,
		"the new run is dated from this failure, not from the outage that ended")
}

// A run of failures that nothing has touched for longer than the whole retry
// ladder is over, whatever the counter still says.
//
// failing_since was only ever cleared by a success or a manual re-enable, so a
// run never ended, it only got older. A ten-hour outage that fell short of the
// threshold left the counter high and failing_since pointing at the start of it;
// once the workspace went quiet — exactly what happens when someone tears a
// receiver down — a single transient 500 days later satisfied the count and the
// window at once and retired the subscription, under a reason describing a
// sustained outage that was three days of silence.
func TestWebhookDeliveryWorker_aFailureLongAfterTheLastOneStartsANewRun(t *testing.T) {
	const workspaceID = "ws-1"

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	server := respondWith(t, http.StatusInternalServerError)

	subRepo := mocks.NewMockWebhookSubscriptionRepository(ctrl)
	deliveryRepo := mocks.NewMockWebhookDeliveryRepository(ctrl)
	worker := NewWebhookDeliveryWorker(subRepo, deliveryRepo,
		mocks.NewMockWorkspaceRepository(ctrl), permissiveWebhookLogger(ctrl), nil)

	// Everything the old run left behind is still on the row: one more failure
	// crosses the threshold, and the window it inherits is days wide.
	sub := &domain.WebhookSubscription{
		ID: "sub-1", URL: server.URL, Secret: testWebhookSecret, Enabled: true,
		ConsecutiveFailures: worker.failureThreshold - 1,
		FailingSince:        failingFor(3 * 24 * time.Hour),
	}

	// The expiry is a real write, not just a fix-up of the cached copy: the row
	// carries the same dead run and every other worker reads it.
	reset := false
	subRepo.EXPECT().ResetFailures(gomock.Any(), workspaceID, "sub-1").
		DoAndReturn(func(_ context.Context, _, _ string) error {
			reset = true
			return nil
		})
	subRepo.EXPECT().IncrementFailures(gomock.Any(), workspaceID, "sub-1").Return(nil)
	deliveryRepo.EXPECT().ScheduleRetry(gomock.Any(), workspaceID, "delivery-1",
		gomock.Any(), 1, gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	deliveryRepo.EXPECT().MarkFailed(gomock.Any(), workspaceID, "delivery-1",
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	disabled := false
	subRepo.EXPECT().DisableWithReason(gomock.Any(), workspaceID, "sub-1", gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _, _ string) error {
			disabled = true
			return nil
		}).AnyTimes()

	worker.processDelivery(context.Background(), workspaceID, &domain.WebhookDelivery{
		ID: "delivery-1", SubscriptionID: "sub-1", EventType: "contact.created",
		Payload: map[string]interface{}{"email": "a@b.c"}, MaxAttempts: 10,
	}, sub, time.Now())

	assert.True(t, reset, "the expired run has to be cleared on the row, not only in the cache")
	assert.False(t, disabled,
		"one failure days after the last one is not twenty consecutive failures over a sustained period")
	assert.True(t, sub.Enabled)
	assert.Equal(t, 1, sub.ConsecutiveFailures, "this failure is the first of the new run")
	require.NotNil(t, sub.FailingSince)
	assert.WithinDuration(t, time.Now().UTC(), *sub.FailingSince, time.Minute)
}

// A run still inside its maximum age keeps counting, which is the other half of
// the rule above: the expiry must not become a way for a genuinely dead endpoint
// to dodge retirement forever.
func TestWebhookDeliveryWorker_aRunInsideItsMaximumAgeStillRetires(t *testing.T) {
	const workspaceID = "ws-1"

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	server := respondWith(t, http.StatusInternalServerError)

	subRepo := mocks.NewMockWebhookSubscriptionRepository(ctrl)
	deliveryRepo := mocks.NewMockWebhookDeliveryRepository(ctrl)
	worker := NewWebhookDeliveryWorker(subRepo, deliveryRepo,
		mocks.NewMockWorkspaceRepository(ctrl), permissiveWebhookLogger(ctrl), nil)

	sub := &domain.WebhookSubscription{
		ID: "sub-1", URL: server.URL, Secret: testWebhookSecret, Enabled: true,
		ConsecutiveFailures: worker.failureThreshold - 1,
		FailingSince:        failingLongEnoughToRetire(),
	}

	subRepo.EXPECT().IncrementFailures(gomock.Any(), workspaceID, "sub-1").Return(nil)
	// Nothing arms ResetFailures: expiring this run would be the bug.
	subRepo.EXPECT().DisableWithReason(gomock.Any(), workspaceID, "sub-1", gomock.Any()).Return(nil)
	deliveryRepo.EXPECT().MarkFailed(gomock.Any(), workspaceID, "delivery-1", 10,
		gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

	worker.processDelivery(context.Background(), workspaceID, &domain.WebhookDelivery{
		ID: "delivery-1", SubscriptionID: "sub-1", EventType: "contact.created",
		Payload: map[string]interface{}{"email": "a@b.c"}, MaxAttempts: 10,
	}, sub, time.Now())

	assert.False(t, sub.Enabled)
}

// A panic after the delivery has been marked delivered must not send it again.
//
// deliverOne's recover releases the claim unconditionally — a panic is a bug in
// us, not a verdict on the delivery — and on `id` alone that release moved a row
// already marked delivered back to 'pending', with attempts under its ceiling
// and next_attempt_at in the past. The next poll claimed it and POSTed it a
// second time: a duplicate side effect in whatever consumes the webhook,
// manufactured by the guard that exists to prevent duplicates.
func TestWebhookDeliveryWorker_aPanicAfterDeliveryDoesNotSendItTwice(t *testing.T) {
	const workspaceID = "ws-1"

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	store := newFakeDeliveryStore(&domain.WebhookDelivery{
		ID: "delivery-1", SubscriptionID: "sub-1", EventType: "contact.created",
		Payload: map[string]interface{}{"email": "a@b.c"},
		Status:  domain.WebhookDeliveryStatusPending,
		// Nowhere near the ceiling, so a row handed back to 'pending' is
		// immediately claimable again.
		MaxAttempts:   10,
		NextAttemptAt: time.Now().UTC().Add(-time.Minute),
	})

	var posted int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		posted++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	subRepo := mocks.NewMockWebhookSubscriptionRepository(ctrl)
	subRepo.EXPECT().GetByID(gomock.Any(), workspaceID, "sub-1").
		Return(&domain.WebhookSubscription{
			ID: "sub-1", URL: server.URL, Secret: testWebhookSecret, Enabled: true,
		}, nil).AnyTimes()
	// The bookkeeping write that follows MarkDelivered blows up. Anything after
	// the commit would do; this is the one that is actually there.
	subRepo.EXPECT().UpdateLastDeliveryAt(gomock.Any(), workspaceID, "sub-1", gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ string, _ time.Time) error {
			panic("boom")
		}).AnyTimes()

	worker := NewWebhookDeliveryWorker(subRepo, store, mocks.NewMockWorkspaceRepository(ctrl),
		permissiveWebhookLogger(ctrl), &http.Client{Timeout: 10 * time.Second})

	require.NoError(t, worker.processWorkspaceDeliveries(context.Background(), workspaceID))

	delivered := store.row(t, "delivery-1")
	assert.Equal(t, domain.WebhookDeliveryStatusDelivered, delivered.Status,
		"a panic in our own bookkeeping does not un-deliver a delivered webhook")

	// The proof, rather than the symptom: poll again and see whether the endpoint
	// is asked to accept the same event twice.
	require.NoError(t, worker.processWorkspaceDeliveries(context.Background(), workspaceID))
	assert.Equal(t, 1, posted, "the same event must not be POSTed a second time")
}

// The cross-worker shape of the same write: this worker stalls past its lease,
// the sweep hands the row to another, and the late release must not yank it back
// out from under a POST already in flight.
func TestWebhookDeliveryWorker_aLateReleaseDoesNotStealBackAReclaimedRow(t *testing.T) {
	const workspaceID = "ws-1"

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	store := newFakeDeliveryStore(&domain.WebhookDelivery{
		ID: "delivery-1", SubscriptionID: "sub-1", EventType: "contact.created",
		Payload: map[string]interface{}{"email": "a@b.c"},
		Status:  domain.WebhookDeliveryStatusPending,
		// Enough attempts spent that a row handed back to 'pending' would still
		// be claimable, which is what makes the theft observable.
		Attempts: 2, MaxAttempts: 10,
		NextAttemptAt: time.Now().UTC().Add(-time.Minute),
	})

	subRepo := mocks.NewMockWebhookSubscriptionRepository(ctrl)
	subRepo.EXPECT().GetByID(gomock.Any(), workspaceID, "sub-1").
		DoAndReturn(func(_ context.Context, _, _ string) (*domain.WebhookSubscription, error) {
			// While we were looking the row up, our lease expired: the sweep
			// returned the row and a second worker claimed it. Its claimed_at has
			// moved, so the token we are still holding is stale.
			stolen := store.now.Add(time.Minute)
			row := store.rows["delivery-1"]
			row.Status = domain.WebhookDeliveryStatusDelivering
			row.ClaimedAt = &stolen
			// And the lookup fails transiently, which is what sends us down the
			// release path rather than a terminal one.
			return nil, errors.New("pq: sorry, too many clients already")
		})

	worker := NewWebhookDeliveryWorker(subRepo, store, mocks.NewMockWorkspaceRepository(ctrl),
		permissiveWebhookLogger(ctrl), &http.Client{Timeout: 10 * time.Second})

	require.NoError(t, worker.processWorkspaceDeliveries(context.Background(), workspaceID))

	row := store.row(t, "delivery-1")
	assert.Equal(t, domain.WebhookDeliveryStatusDelivering, row.Status,
		"the row belongs to the worker that reclaimed it; releasing it here puts it back in the queue mid-flight")
	require.NotNil(t, row.ClaimedAt)
	assert.Nil(t, row.LastError, "and its owner's delivery log is not ours to write to")
}

// Every naked path on the worker's goroutine, not just the one delivery.
//
// Start is launched by internal/app/app.go with a plain `go func()` and no
// recover of its own, so a panic that escapes the poll takes the entire server
// down: every in-flight HTTP request, every other worker. deliverOne guards one
// call inside the batch loop and nothing else — listing workspaces, the loop
// itself, the reclaim sweep and the retention cleanup all ran bare.
func TestWebhookDeliveryWorker_aPanicAnywhereInThePollIsContained(t *testing.T) {
	ctx := context.Background()

	cases := map[string]func(t *testing.T) *WebhookDeliveryWorker{
		"listing the workspaces": func(t *testing.T) *WebhookDeliveryWorker {
			ctrl := gomock.NewController(t)
			workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
			workspaceRepo.EXPECT().List(gomock.Any()).
				DoAndReturn(func(_ context.Context) ([]*domain.Workspace, error) {
					panic("boom")
				}).AnyTimes()
			worker := NewWebhookDeliveryWorker(mocks.NewMockWebhookSubscriptionRepository(ctrl),
				mocks.NewMockWebhookDeliveryRepository(ctrl), workspaceRepo,
				permissiveWebhookLogger(ctrl), nil)
			// Skip the retention sweep, so this case exercises one naked path.
			worker.lastCleanupTime = time.Now()
			return worker
		},
		"the reclaim sweep": func(t *testing.T) *WebhookDeliveryWorker {
			ctrl := gomock.NewController(t)
			workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
			workspaceRepo.EXPECT().List(gomock.Any()).
				Return([]*domain.Workspace{{ID: "ws-1"}}, nil).AnyTimes()
			deliveryRepo := mocks.NewMockWebhookDeliveryRepository(ctrl)
			deliveryRepo.EXPECT().ReclaimStale(gomock.Any(), "ws-1", gomock.Any()).
				DoAndReturn(func(_ context.Context, _ string, _ time.Duration) (int64, error) {
					panic("boom")
				}).AnyTimes()
			worker := NewWebhookDeliveryWorker(mocks.NewMockWebhookSubscriptionRepository(ctrl),
				deliveryRepo, workspaceRepo, permissiveWebhookLogger(ctrl), nil)
			worker.lastCleanupTime = time.Now()
			return worker
		},
		"the retention cleanup": func(t *testing.T) *WebhookDeliveryWorker {
			ctrl := gomock.NewController(t)
			workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
			workspaceRepo.EXPECT().List(gomock.Any()).
				Return([]*domain.Workspace{{ID: "ws-1"}}, nil).AnyTimes()
			deliveryRepo := mocks.NewMockWebhookDeliveryRepository(ctrl)
			deliveryRepo.EXPECT().CleanupOldDeliveries(gomock.Any(), "ws-1", gomock.Any()).
				DoAndReturn(func(_ context.Context, _ string, _ int) (int64, error) {
					panic("boom")
				}).AnyTimes()
			return NewWebhookDeliveryWorker(mocks.NewMockWebhookSubscriptionRepository(ctrl),
				deliveryRepo, workspaceRepo, permissiveWebhookLogger(ctrl), nil)
		},
	}

	for name, build := range cases {
		t.Run(name, func(t *testing.T) {
			// The boundary, stated rather than assumed: the poll itself is not
			// protected, so the recover has to sit outside it.
			assert.Panics(t, func() { build(t).processDeliveries(ctx) })

			worker := build(t)
			assert.NotPanics(t, func() { worker.processDeliveriesGuarded(ctx) },
				"a panic here would take the whole server down with it")
		})
	}
}

// webhookPanickyReleaseStore is a delivery store whose release path blows up.
//
// The vector does not matter and is not the point: ReleaseClaim reaches a
// workspace connection, a pool and a driver, and the worker's contract is that a
// bug down there costs one delivery rather than the batch. What is under test is
// where the panic lands, which is a property of the code around the call.
type webhookPanickyReleaseStore struct {
	*fakeDeliveryStore
	released int
}

func (s *webhookPanickyReleaseStore) ReleaseClaim(context.Context, string, string, *time.Time, string) error {
	s.released++
	panic("boom in the release path")
}

// TestWebhookDeliveryWorker_aPanicInTheReleasePathCostsOneDelivery covers the
// gap between what deliverOne's comment promises — "a panic costs that delivery
// rather than the rest of the batch" — and what Go's semantics deliver.
//
// Once recover() has returned, the panic it caught is finished. A panic raised
// afterwards in the same deferred function is a NEW panic on a normal unwind: it
// does not reach the recover it looks like it is standing inside. So a release
// that blew up left deliverOne, abandoned every remaining row of the batch in
// 'delivering' holding a live claim, left the workspace loop, and was caught
// only by processDeliveriesGuarded — which drops the entire poll. One buggy
// delivery cost every workspace's batch.
//
// Both callers of the release are driven, because they fail the same way for the
// same reason and only one of them is a deferred call.
func TestWebhookDeliveryWorker_aPanicInTheReleasePathCostsOneDelivery(t *testing.T) {
	const workspaceID = "ws-1"

	newBatch := func() *webhookPanickyReleaseStore {
		rows := make([]*domain.WebhookDelivery, 0, 2)
		for _, id := range []string{"delivery-1", "delivery-2"} {
			rows = append(rows, &domain.WebhookDelivery{
				ID: id, SubscriptionID: "sub-1", EventType: "contact.created",
				Payload: map[string]interface{}{"email": "a@b.c"},
				Status:  domain.WebhookDeliveryStatusPending,
				// Well below the ceiling, so nothing here is terminal by arithmetic.
				MaxAttempts:   10,
				NextAttemptAt: time.Now().UTC().Add(-time.Minute),
			})
		}
		return &webhookPanickyReleaseStore{fakeDeliveryStore: newFakeDeliveryStore(rows...)}
	}

	t.Run("released from deliverOne's recover", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		store := newBatch()

		var posted int
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			posted++
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		}))
		defer server.Close()

		subRepo := mocks.NewMockWebhookSubscriptionRepository(ctrl)
		subRepo.EXPECT().GetByID(gomock.Any(), workspaceID, "sub-1").
			Return(&domain.WebhookSubscription{
				ID: "sub-1", URL: server.URL, Secret: testWebhookSecret, Enabled: true,
			}, nil).AnyTimes()

		// The first delivery's bookkeeping panics, which is what sends it into
		// deliverOne's recover and from there into the release. The second one is
		// left alone: it is the rest of the batch, and whether it goes out is the
		// question.
		var bookkeeping int
		subRepo.EXPECT().UpdateLastDeliveryAt(gomock.Any(), workspaceID, "sub-1", gomock.Any()).
			DoAndReturn(func(_ context.Context, _, _ string, _ time.Time) error {
				bookkeeping++
				if bookkeeping == 1 {
					panic("boom")
				}
				return nil
			}).AnyTimes()

		worker := NewWebhookDeliveryWorker(subRepo, store, mocks.NewMockWorkspaceRepository(ctrl),
			permissiveWebhookLogger(ctrl), &http.Client{Timeout: 10 * time.Second})

		require.NotPanics(t, func() {
			require.NoError(t, worker.processWorkspaceDeliveries(context.Background(), workspaceID))
		}, "a panic in the release path must not escape the delivery it belongs to")

		require.Equal(t, 1, store.released, "the case is vacuous unless the release actually ran")
		assert.Equal(t, 2, posted, "the rest of the batch goes out")
		assert.Equal(t, domain.WebhookDeliveryStatusDelivered, store.row(t, "delivery-2").Status,
			"and reaches a terminal state rather than being abandoned mid-claim")
	})

	t.Run("released from a transient subscription lookup failure", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		store := newBatch()

		var posted int
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			posted++
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		}))
		defer server.Close()

		// This release is not deferred at all — it runs bare in the batch loop — so
		// it never had even the appearance of a guard around it.
		subRepo := mocks.NewMockWebhookSubscriptionRepository(ctrl)
		gomock.InOrder(
			subRepo.EXPECT().GetByID(gomock.Any(), workspaceID, "sub-1").
				Return(nil, errors.New("pq: sorry, too many clients already")),
			subRepo.EXPECT().GetByID(gomock.Any(), workspaceID, "sub-1").
				Return(&domain.WebhookSubscription{
					ID: "sub-1", URL: server.URL, Secret: testWebhookSecret, Enabled: true,
				}, nil),
		)
		subRepo.EXPECT().UpdateLastDeliveryAt(gomock.Any(), workspaceID, "sub-1", gomock.Any()).
			Return(nil).AnyTimes()

		worker := NewWebhookDeliveryWorker(subRepo, store, mocks.NewMockWorkspaceRepository(ctrl),
			permissiveWebhookLogger(ctrl), &http.Client{Timeout: 10 * time.Second})

		require.NotPanics(t, func() {
			require.NoError(t, worker.processWorkspaceDeliveries(context.Background(), workspaceID))
		})

		require.Equal(t, 1, store.released)
		assert.Equal(t, 1, posted, "the row whose lookup failed is not POSTed; the one after it is")
		assert.Equal(t, domain.WebhookDeliveryStatusDelivered, store.row(t, "delivery-2").Status)
	})
}

// TestWebhookDeliveryWorker_aShortLeaseCutsTheRequestNotTheOtherWayRound is the
// behavioural half of the invariant: the derived budget is not just a field, it
// actually ends the request.
//
// Built exactly as the option made possible — a lease far shorter than the
// client's timeout — against a receiver that holds the connection open past the
// budget and answers well inside the client timeout. Without the budget the POST
// runs to completion, the row is marked delivered somewhere past its lease, and
// the sweep has already handed it to whoever polls next. With it, the attempt is
// abandoned inside the claim and the ladder schedules another.
func TestWebhookDeliveryWorker_aShortLeaseCutsTheRequestNotTheOtherWayRound(t *testing.T) {
	const workspaceID = "ws-1"

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// The lease gives a budget of two thirds of itself; the receiver answers at
	// twice that. Both sides of the gap are timer-driven, so the margin does not
	// depend on how fast the machine is.
	const (
		claimLease   = 300 * time.Millisecond
		receiverHold = 400 * time.Millisecond
	)

	held := make(chan struct{}, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		held <- struct{}{}
		select {
		case <-time.After(receiverHold):
		case <-r.Context().Done():
			// The proof from the receiver's side: the client hung up first.
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	store := newFakeDeliveryStore(&domain.WebhookDelivery{
		ID: "delivery-1", SubscriptionID: "sub-1", EventType: "contact.created",
		Payload: map[string]interface{}{"email": "a@b.c"},
		Status:  domain.WebhookDeliveryStatusPending,
		// Room on the ladder, so a cut-off attempt is scheduled rather than retired.
		MaxAttempts:   10,
		NextAttemptAt: time.Now().UTC().Add(-time.Minute),
	})

	subRepo := mocks.NewMockWebhookSubscriptionRepository(ctrl)
	subRepo.EXPECT().GetByID(gomock.Any(), workspaceID, "sub-1").
		Return(&domain.WebhookSubscription{
			ID: "sub-1", URL: server.URL, Secret: testWebhookSecret, Enabled: true,
		}, nil).AnyTimes()
	subRepo.EXPECT().IncrementFailures(gomock.Any(), workspaceID, "sub-1").Return(nil).AnyTimes()
	subRepo.EXPECT().UpdateLastDeliveryAt(gomock.Any(), workspaceID, "sub-1", gomock.Any()).
		Return(nil).AnyTimes()

	// A ten-second client, so nothing but the lease can be what stops this
	// request.
	worker := NewWebhookDeliveryWorker(subRepo, store, mocks.NewMockWorkspaceRepository(ctrl),
		permissiveWebhookLogger(ctrl), &http.Client{Timeout: 10 * time.Second},
		WithWebhookClaimLease(claimLease))

	started := time.Now()
	require.NoError(t, worker.processWorkspaceDeliveries(context.Background(), workspaceID))
	elapsed := time.Since(started)

	require.Len(t, held, 1, "the receiver has to have been reached, or this proves nothing")
	assert.Less(t, elapsed, claimLease,
		"the attempt has to be abandoned inside the claim that authorises it")

	row := store.row(t, "delivery-1")
	assert.NotEqual(t, domain.WebhookDeliveryStatusDelivered, row.Status,
		"a response that arrives after the lease expired belongs to whoever holds the row now")
	require.NotNil(t, row.LastError)
	assert.Contains(t, *row.LastError, context.DeadlineExceeded.Error(),
		"and the delivery log says the request ran out of time rather than that the endpoint failed")
}

// TestWebhookDeliveryWorker_theRequestBudgetIsSpentFromTheClaim covers the half
// of the invariant normaliseTimings could not enforce on its own: the budget is
// derived from the lease, but a budget only bounds the claim if its clock starts
// where the claim's does.
//
// It does not start at the POST. Between the renewal that re-stamps claimed_at
// and the first byte on the wire sit a subscription lookup — a real query, on a
// pool Go's sql.DB blocks on for as long as it takes — a marshal and a
// signature, and every one of them is spent inside the same lease. Production
// leaves five seconds of slack under a fifteen-second lease, and that slack also
// has to pay for the write that records the outcome, so one slow lookup is
// enough to put the end of the request past the claim: the sweep hands the row
// to a second worker while the first one's request is still open, both POST it,
// and the delivery log still says one attempt.
func TestWebhookDeliveryWorker_theRequestBudgetIsSpentFromTheClaim(t *testing.T) {
	const workspaceID = "ws-1"

	newRow := func() *domain.WebhookDelivery {
		return &domain.WebhookDelivery{
			ID: "delivery-1", SubscriptionID: "sub-1", EventType: "contact.created",
			Payload: map[string]interface{}{"email": "a@b.c"},
			Status:  domain.WebhookDeliveryStatusPending,
			// Room on the ladder, so an abandoned attempt is rescheduled rather
			// than retired.
			MaxAttempts:   10,
			NextAttemptAt: time.Now().UTC().Add(-time.Minute),
		}
	}

	t.Run("a slow lookup is charged to the request, not to the lease", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		// A two-second lease buys a budget of two thirds of itself. The lookup
		// takes three quarters of that budget, which still leaves the POST a third
		// of a second to reach a receiver on loopback — and leaves the whole
		// attempt two thirds of a second inside its lease. Charge the lookup to
		// the lease instead and the request alone ends a third of a second past
		// it. Every bound here is timer-driven, so the margins do not depend on
		// how fast the machine is.
		const (
			claimLease  = 2 * time.Second
			lookupDelay = 1 * time.Second
		)

		reached := make(chan struct{}, 4)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			reached <- struct{}{}
			// Drained before the wait, or the hang-up below is never noticed: Go's
			// server only starts watching a connection for a client that went away
			// once the handler has consumed the request body, so an undrained
			// receiver sits out the whole hold and the test pays for it.
			_, _ = io.Copy(io.Discard, r.Body)
			select {
			case <-time.After(2 * time.Second):
			case <-r.Context().Done():
				// The proof from the receiver's side: the client hung up first.
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		}))
		defer server.Close()

		store := newFakeDeliveryStore(newRow())

		subRepo := mocks.NewMockWebhookSubscriptionRepository(ctrl)
		subRepo.EXPECT().GetByID(gomock.Any(), workspaceID, "sub-1").
			DoAndReturn(func(_ context.Context, _, _ string) (*domain.WebhookSubscription, error) {
				time.Sleep(lookupDelay)
				return &domain.WebhookSubscription{
					ID: "sub-1", URL: server.URL, Secret: testWebhookSecret, Enabled: true,
				}, nil
			}).AnyTimes()
		subRepo.EXPECT().IncrementFailures(gomock.Any(), workspaceID, "sub-1").Return(nil).AnyTimes()
		subRepo.EXPECT().UpdateLastDeliveryAt(gomock.Any(), workspaceID, "sub-1", gomock.Any()).
			Return(nil).AnyTimes()

		// A ten-second client, so nothing but the claim can be what stops this
		// request.
		worker := NewWebhookDeliveryWorker(subRepo, store, mocks.NewMockWorkspaceRepository(ctrl),
			permissiveWebhookLogger(ctrl), &http.Client{Timeout: 10 * time.Second},
			WithWebhookClaimLease(claimLease))

		started := time.Now()
		require.NoError(t, worker.processWorkspaceDeliveries(context.Background(), workspaceID))
		elapsed := time.Since(started)

		require.Len(t, reached, 1,
			"the receiver has to have been reached on what was left of the budget, or this proves nothing")
		assert.Less(t, elapsed, claimLease,
			"everything the claim authorises — the lookup and the request both — has to fit inside it")

		row := store.row(t, "delivery-1")
		assert.NotEqual(t, domain.WebhookDeliveryStatusDelivered, row.Status,
			"a response that arrives past the lease belongs to whoever holds the row by then")
	})

	t.Run("a budget already spent releases the row instead of POSTing past the lease", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		// The lookup outlasts the whole budget here, so there is no request left
		// to cut short — only the choice of what to do with a claim whose safe
		// window is gone.
		const (
			claimLease  = 600 * time.Millisecond
			lookupDelay = 700 * time.Millisecond
		)

		reached := make(chan struct{}, 4)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			reached <- struct{}{}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		}))
		defer server.Close()

		store := newFakeDeliveryStore(newRow())

		subRepo := mocks.NewMockWebhookSubscriptionRepository(ctrl)
		subRepo.EXPECT().GetByID(gomock.Any(), workspaceID, "sub-1").
			DoAndReturn(func(_ context.Context, _, _ string) (*domain.WebhookSubscription, error) {
				time.Sleep(lookupDelay)
				return &domain.WebhookSubscription{
					ID: "sub-1", URL: server.URL, Secret: testWebhookSecret, Enabled: true,
				}, nil
			}).AnyTimes()
		subRepo.EXPECT().IncrementFailures(gomock.Any(), workspaceID, "sub-1").Return(nil).AnyTimes()
		subRepo.EXPECT().UpdateLastDeliveryAt(gomock.Any(), workspaceID, "sub-1", gomock.Any()).
			Return(nil).AnyTimes()

		worker := NewWebhookDeliveryWorker(subRepo, store, mocks.NewMockWorkspaceRepository(ctrl),
			permissiveWebhookLogger(ctrl), &http.Client{Timeout: 10 * time.Second},
			WithWebhookClaimLease(claimLease))

		require.NoError(t, worker.processWorkspaceDeliveries(context.Background(), workspaceID))

		assert.Len(t, reached, 0,
			"a request that cannot finish inside the claim must not be started under it")

		row := store.row(t, "delivery-1")
		assert.Equal(t, domain.WebhookDeliveryStatusPending, row.Status,
			"the row goes back where the next poll can find it")
		assert.Nil(t, row.ClaimedAt)
		// Released, not failed: nothing was sent, so nothing was attempted, and
		// spending one of ten attempts on our own database having a bad minute is
		// how a transient outage turns into lost deliveries.
		assert.Equal(t, 0, row.Attempts)
		require.NotNil(t, row.LastError)
		assert.Contains(t, *row.LastError, "ran out of time before the request could be sent")
	})
}
