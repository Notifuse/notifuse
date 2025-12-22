package queue

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultWorkerConfig(t *testing.T) {
	config := DefaultWorkerConfig()

	require.NotNil(t, config)
	assert.Equal(t, 5, config.WorkerCount)
	assert.Equal(t, 1*time.Second, config.PollInterval)
	assert.Equal(t, 50, config.BatchSize)
	assert.Equal(t, 3, config.MaxRetries)
}

func TestNewEmailQueueWorker(t *testing.T) {
	t.Run("creates worker with all dependencies", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockQueueRepo := mocks.NewMockEmailQueueRepository(ctrl)
		mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)

		config := &EmailQueueWorkerConfig{
			WorkerCount:  3,
			PollInterval: 2 * time.Second,
			BatchSize:    25,
			MaxRetries:   5,
		}

		worker := NewEmailQueueWorker(
			mockQueueRepo,
			mockWorkspaceRepo,
			mockEmailService,
			config,
			mockLogger,
		)

		require.NotNil(t, worker)
		assert.Equal(t, config, worker.config)
		assert.NotNil(t, worker.rateLimiter)
	})

	t.Run("uses default config when nil provided", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockQueueRepo := mocks.NewMockEmailQueueRepository(ctrl)
		mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)

		worker := NewEmailQueueWorker(
			mockQueueRepo,
			mockWorkspaceRepo,
			mockEmailService,
			nil, // nil config
			mockLogger,
		)

		require.NotNil(t, worker)
		assert.Equal(t, 5, worker.config.WorkerCount)
		assert.Equal(t, 1*time.Second, worker.config.PollInterval)
	})
}

func TestEmailQueueWorker_SetCallbacks(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueueRepo := mocks.NewMockEmailQueueRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	worker := NewEmailQueueWorker(
		mockQueueRepo,
		mockWorkspaceRepo,
		mockEmailService,
		nil,
		mockLogger,
	)

	var sentCalled, failedCalled bool

	onSent := func(workspaceID string, sourceType domain.EmailQueueSourceType, sourceID string, messageID string) {
		sentCalled = true
	}

	onFailed := func(workspaceID string, sourceType domain.EmailQueueSourceType, sourceID string, messageID string, err error, isDeadLetter bool) {
		failedCalled = true
	}

	worker.SetCallbacks(onSent, onFailed)

	// Verify callbacks are set (we can't access them directly, but we can verify they were accepted)
	assert.NotNil(t, worker.onEmailSent)
	assert.NotNil(t, worker.onEmailFailed)

	// Call them to verify they work
	worker.onEmailSent("ws1", domain.EmailQueueSourceBroadcast, "bc1", "msg1")
	worker.onEmailFailed("ws1", domain.EmailQueueSourceBroadcast, "bc1", "msg1", errors.New("test"), false)

	assert.True(t, sentCalled)
	assert.True(t, failedCalled)
}

func TestEmailQueueWorker_StartStop(t *testing.T) {
	t.Run("start sets running to true", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockQueueRepo := mocks.NewMockEmailQueueRepository(ctrl)
		mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)

		// Expect log calls
		mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()

		// Expect List to be called (worker polls workspaces)
		mockWorkspaceRepo.EXPECT().List(gomock.Any()).Return([]*domain.Workspace{}, nil).AnyTimes()

		worker := NewEmailQueueWorker(
			mockQueueRepo,
			mockWorkspaceRepo,
			mockEmailService,
			&EmailQueueWorkerConfig{
				WorkerCount:  1,
				PollInterval: 100 * time.Millisecond, // Short interval for test
				BatchSize:    10,
				MaxRetries:   3,
			},
			mockLogger,
		)

		assert.False(t, worker.IsRunning())

		err := worker.Start(context.Background())
		assert.NoError(t, err)
		assert.True(t, worker.IsRunning())

		// Clean up
		worker.Stop()
	})

	t.Run("stop sets running to false", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockQueueRepo := mocks.NewMockEmailQueueRepository(ctrl)
		mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)

		// Expect log calls
		mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()

		mockWorkspaceRepo.EXPECT().List(gomock.Any()).Return([]*domain.Workspace{}, nil).AnyTimes()

		worker := NewEmailQueueWorker(
			mockQueueRepo,
			mockWorkspaceRepo,
			mockEmailService,
			&EmailQueueWorkerConfig{
				WorkerCount:  1,
				PollInterval: 100 * time.Millisecond,
				BatchSize:    10,
				MaxRetries:   3,
			},
			mockLogger,
		)

		_ = worker.Start(context.Background())
		assert.True(t, worker.IsRunning())

		worker.Stop()
		assert.False(t, worker.IsRunning())
	})

	t.Run("start is idempotent", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockQueueRepo := mocks.NewMockEmailQueueRepository(ctrl)
		mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)

		mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()

		mockWorkspaceRepo.EXPECT().List(gomock.Any()).Return([]*domain.Workspace{}, nil).AnyTimes()

		worker := NewEmailQueueWorker(
			mockQueueRepo,
			mockWorkspaceRepo,
			mockEmailService,
			&EmailQueueWorkerConfig{
				WorkerCount:  1,
				PollInterval: 100 * time.Millisecond,
				BatchSize:    10,
				MaxRetries:   3,
			},
			mockLogger,
		)

		// Start twice
		err1 := worker.Start(context.Background())
		err2 := worker.Start(context.Background())

		assert.NoError(t, err1)
		assert.NoError(t, err2)
		assert.True(t, worker.IsRunning())

		worker.Stop()
	})

	t.Run("stop is idempotent", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockQueueRepo := mocks.NewMockEmailQueueRepository(ctrl)
		mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)

		mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()

		mockWorkspaceRepo.EXPECT().List(gomock.Any()).Return([]*domain.Workspace{}, nil).AnyTimes()

		worker := NewEmailQueueWorker(
			mockQueueRepo,
			mockWorkspaceRepo,
			mockEmailService,
			&EmailQueueWorkerConfig{
				WorkerCount:  1,
				PollInterval: 100 * time.Millisecond,
				BatchSize:    10,
				MaxRetries:   3,
			},
			mockLogger,
		)

		_ = worker.Start(context.Background())

		// Stop twice - should not panic
		worker.Stop()
		worker.Stop()

		assert.False(t, worker.IsRunning())
	})
}

func TestEmailQueueWorker_ProcessEntry_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueueRepo := mocks.NewMockEmailQueueRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Setup logger
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

	integrationID := "integration-1"
	entryID := "entry-1"
	workspaceID := "workspace-1"

	workspace := &domain.Workspace{
		ID: workspaceID,
		Integrations: []domain.Integration{
			{
				ID: integrationID,
				EmailProvider: domain.EmailProvider{
					Kind:               domain.EmailProviderKindSMTP,
					RateLimitPerMinute: 100,
				},
			},
		},
	}

	entry := &domain.EmailQueueEntry{
		ID:            entryID,
		Status:        domain.EmailQueueStatusPending,
		SourceType:    domain.EmailQueueSourceBroadcast,
		SourceID:      "broadcast-1",
		IntegrationID: integrationID,
		ContactEmail:  "test@example.com",
		MessageID:     "msg-1",
		Payload: domain.EmailQueuePayload{
			FromAddress:        "sender@example.com",
			FromName:           "Sender",
			Subject:            "Test Subject",
			HTMLContent:        "<p>Hello</p>",
			RateLimitPerMinute: 100,
		},
		Attempts:    0,
		MaxAttempts: 3,
	}

	// Expect calls in order
	mockQueueRepo.EXPECT().MarkAsProcessing(gomock.Any(), workspaceID, entryID).Return(nil)
	mockEmailService.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).Return(nil)
	mockQueueRepo.EXPECT().MarkAsSent(gomock.Any(), workspaceID, entryID).Return(nil)

	worker := NewEmailQueueWorker(
		mockQueueRepo,
		mockWorkspaceRepo,
		mockEmailService,
		DefaultWorkerConfig(),
		mockLogger,
	)
	worker.ctx = context.Background()

	var sentCallbackCalled bool
	worker.SetCallbacks(
		func(wsID string, sourceType domain.EmailQueueSourceType, sourceID string, messageID string) {
			sentCallbackCalled = true
			assert.Equal(t, workspaceID, wsID)
			assert.Equal(t, domain.EmailQueueSourceBroadcast, sourceType)
			assert.Equal(t, "broadcast-1", sourceID)
			assert.Equal(t, "msg-1", messageID)
		},
		nil,
	)

	// Process the entry
	worker.processEntry(workspace, entry)

	assert.True(t, sentCallbackCalled)
}

func TestEmailQueueWorker_ProcessEntry_SendFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueueRepo := mocks.NewMockEmailQueueRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Setup logger
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	integrationID := "integration-1"
	entryID := "entry-1"
	workspaceID := "workspace-1"

	workspace := &domain.Workspace{
		ID: workspaceID,
		Integrations: []domain.Integration{
			{
				ID: integrationID,
				EmailProvider: domain.EmailProvider{
					Kind:               domain.EmailProviderKindSMTP,
					RateLimitPerMinute: 100,
				},
			},
		},
	}

	entry := &domain.EmailQueueEntry{
		ID:            entryID,
		Status:        domain.EmailQueueStatusPending,
		SourceType:    domain.EmailQueueSourceBroadcast,
		SourceID:      "broadcast-1",
		IntegrationID: integrationID,
		ContactEmail:  "test@example.com",
		MessageID:     "msg-1",
		Payload: domain.EmailQueuePayload{
			FromAddress:        "sender@example.com",
			FromName:           "Sender",
			Subject:            "Test Subject",
			HTMLContent:        "<p>Hello</p>",
			RateLimitPerMinute: 100,
		},
		Attempts:    0,
		MaxAttempts: 3,
	}

	sendErr := errors.New("SMTP connection failed")

	// Expect calls in order
	mockQueueRepo.EXPECT().MarkAsProcessing(gomock.Any(), workspaceID, entryID).Return(nil)
	mockEmailService.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).Return(sendErr)
	// After failure, should schedule retry
	mockQueueRepo.EXPECT().MarkAsFailed(gomock.Any(), workspaceID, entryID, sendErr.Error(), gomock.Any()).Return(nil)

	worker := NewEmailQueueWorker(
		mockQueueRepo,
		mockWorkspaceRepo,
		mockEmailService,
		DefaultWorkerConfig(),
		mockLogger,
	)
	worker.ctx = context.Background()

	var failedCallbackCalled bool
	worker.SetCallbacks(
		nil,
		func(wsID string, sourceType domain.EmailQueueSourceType, sourceID string, messageID string, err error, isDeadLetter bool) {
			failedCallbackCalled = true
			assert.Equal(t, workspaceID, wsID)
			assert.False(t, isDeadLetter) // Should not be dead letter yet
		},
	)

	// Process the entry
	worker.processEntry(workspace, entry)

	assert.True(t, failedCallbackCalled)
}

func TestEmailQueueWorker_ProcessEntry_MaxAttemptsExceeded(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueueRepo := mocks.NewMockEmailQueueRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Setup logger
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	integrationID := "integration-1"
	entryID := "entry-1"
	workspaceID := "workspace-1"

	workspace := &domain.Workspace{
		ID: workspaceID,
		Integrations: []domain.Integration{
			{
				ID: integrationID,
				EmailProvider: domain.EmailProvider{
					Kind:               domain.EmailProviderKindSMTP,
					RateLimitPerMinute: 100,
				},
			},
		},
	}

	entry := &domain.EmailQueueEntry{
		ID:            entryID,
		Status:        domain.EmailQueueStatusPending,
		SourceType:    domain.EmailQueueSourceBroadcast,
		SourceID:      "broadcast-1",
		IntegrationID: integrationID,
		ContactEmail:  "test@example.com",
		MessageID:     "msg-1",
		Payload: domain.EmailQueuePayload{
			FromAddress:        "sender@example.com",
			FromName:           "Sender",
			Subject:            "Test Subject",
			HTMLContent:        "<p>Hello</p>",
			RateLimitPerMinute: 100,
		},
		Attempts:    2, // Already 2 attempts
		MaxAttempts: 3, // Max is 3, so after this attempt it should go to dead letter
	}

	sendErr := errors.New("SMTP connection failed")

	// Expect calls in order
	mockQueueRepo.EXPECT().MarkAsProcessing(gomock.Any(), workspaceID, entryID).Return(nil)
	mockEmailService.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).Return(sendErr)
	// Should move to dead letter since attempts >= maxAttempts after increment
	mockQueueRepo.EXPECT().MoveToDeadLetter(gomock.Any(), workspaceID, entry, sendErr.Error()).Return(nil)

	worker := NewEmailQueueWorker(
		mockQueueRepo,
		mockWorkspaceRepo,
		mockEmailService,
		DefaultWorkerConfig(),
		mockLogger,
	)
	worker.ctx = context.Background()

	var failedCallbackCalled bool
	worker.SetCallbacks(
		nil,
		func(wsID string, sourceType domain.EmailQueueSourceType, sourceID string, messageID string, err error, isDeadLetter bool) {
			failedCallbackCalled = true
			assert.Equal(t, workspaceID, wsID)
			assert.True(t, isDeadLetter) // Should be dead letter now
		},
	)

	// Process the entry
	worker.processEntry(workspace, entry)

	assert.True(t, failedCallbackCalled)
}

func TestEmailQueueWorker_ProcessEntry_IntegrationNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueueRepo := mocks.NewMockEmailQueueRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Setup logger
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	workspaceID := "workspace-1"
	entryID := "entry-1"

	workspace := &domain.Workspace{
		ID:           workspaceID,
		Integrations: []domain.Integration{}, // No integrations
	}

	entry := &domain.EmailQueueEntry{
		ID:            entryID,
		Status:        domain.EmailQueueStatusPending,
		SourceType:    domain.EmailQueueSourceBroadcast,
		SourceID:      "broadcast-1",
		IntegrationID: "non-existent-integration",
		ContactEmail:  "test@example.com",
		MessageID:     "msg-1",
		Payload:       domain.EmailQueuePayload{},
		Attempts:      0,
		MaxAttempts:   3,
	}

	// Expect mark as processing
	mockQueueRepo.EXPECT().MarkAsProcessing(gomock.Any(), workspaceID, entryID).Return(nil)
	// Expect mark as failed due to integration not found (but won't go to dead letter yet)
	mockQueueRepo.EXPECT().MarkAsFailed(gomock.Any(), workspaceID, entryID, gomock.Any(), gomock.Any()).Return(nil)

	worker := NewEmailQueueWorker(
		mockQueueRepo,
		mockWorkspaceRepo,
		mockEmailService,
		DefaultWorkerConfig(),
		mockLogger,
	)
	worker.ctx = context.Background()

	worker.processEntry(workspace, entry)
}

func TestEmailQueueWorker_ProcessEntry_MarkAsProcessingFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueueRepo := mocks.NewMockEmailQueueRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Setup logger
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()

	workspaceID := "workspace-1"
	entryID := "entry-1"

	workspace := &domain.Workspace{
		ID: workspaceID,
	}

	entry := &domain.EmailQueueEntry{
		ID:            entryID,
		Status:        domain.EmailQueueStatusPending,
		IntegrationID: "integration-1",
	}

	// MarkAsProcessing fails (maybe another worker grabbed it)
	mockQueueRepo.EXPECT().MarkAsProcessing(gomock.Any(), workspaceID, entryID).
		Return(errors.New("entry already processing"))

	worker := NewEmailQueueWorker(
		mockQueueRepo,
		mockWorkspaceRepo,
		mockEmailService,
		DefaultWorkerConfig(),
		mockLogger,
	)
	worker.ctx = context.Background()

	// Should return early without sending
	worker.processEntry(workspace, entry)
	// No further expectations means the test passes if it doesn't try to send
}

func TestEmailQueueWorker_GetStats(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueueRepo := mocks.NewMockEmailQueueRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	worker := NewEmailQueueWorker(
		mockQueueRepo,
		mockWorkspaceRepo,
		mockEmailService,
		nil,
		mockLogger,
	)

	// Prime the rate limiter with some entries
	worker.rateLimiter.GetOrCreateLimiter("integration-1", 60)
	worker.rateLimiter.GetOrCreateLimiter("integration-2", 120)

	stats := worker.GetStats()

	assert.Len(t, stats, 2)
	_, ok1 := stats["integration-1"]
	assert.True(t, ok1)
	_, ok2 := stats["integration-2"]
	assert.True(t, ok2)
}

func TestEmailQueueWorker_GetConfig(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueueRepo := mocks.NewMockEmailQueueRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	customConfig := &EmailQueueWorkerConfig{
		WorkerCount:  10,
		PollInterval: 5 * time.Second,
		BatchSize:    100,
		MaxRetries:   5,
	}

	worker := NewEmailQueueWorker(
		mockQueueRepo,
		mockWorkspaceRepo,
		mockEmailService,
		customConfig,
		mockLogger,
	)

	config := worker.GetConfig()

	assert.Equal(t, customConfig, config)
	assert.Equal(t, 10, config.WorkerCount)
	assert.Equal(t, 5*time.Second, config.PollInterval)
	assert.Equal(t, 100, config.BatchSize)
	assert.Equal(t, 5, config.MaxRetries)
}

func TestEmailQueueWorker_ProcessWorkspace(t *testing.T) {
	t.Run("processes pending entries", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockQueueRepo := mocks.NewMockEmailQueueRepository(ctrl)
		mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)

		mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

		integrationID := "integration-1"
		workspaceID := "workspace-1"

		workspace := &domain.Workspace{
			ID: workspaceID,
			Integrations: []domain.Integration{
				{
					ID: integrationID,
					EmailProvider: domain.EmailProvider{
						Kind:               domain.EmailProviderKindSMTP,
						RateLimitPerMinute: 6000, // High rate for test
					},
				},
			},
		}

		entries := []*domain.EmailQueueEntry{
			{
				ID:            "entry-1",
				Status:        domain.EmailQueueStatusPending,
				SourceType:    domain.EmailQueueSourceBroadcast,
				SourceID:      "broadcast-1",
				IntegrationID: integrationID,
				ContactEmail:  "test1@example.com",
				MessageID:     "msg-1",
				Payload: domain.EmailQueuePayload{
					RateLimitPerMinute: 6000,
				},
				Attempts:    0,
				MaxAttempts: 3,
			},
		}

		mockQueueRepo.EXPECT().FetchPending(gomock.Any(), workspaceID, gomock.Any()).Return(entries, nil)
		mockQueueRepo.EXPECT().MarkAsProcessing(gomock.Any(), workspaceID, "entry-1").Return(nil)
		mockEmailService.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).Return(nil)
		mockQueueRepo.EXPECT().MarkAsSent(gomock.Any(), workspaceID, "entry-1").Return(nil)

		worker := NewEmailQueueWorker(
			mockQueueRepo,
			mockWorkspaceRepo,
			mockEmailService,
			DefaultWorkerConfig(),
			mockLogger,
		)

		worker.ctx = context.Background()
		worker.processWorkspace(workspace)
	})

	t.Run("handles empty queue", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockQueueRepo := mocks.NewMockEmailQueueRepository(ctrl)
		mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)

		workspaceID := "workspace-1"
		workspace := &domain.Workspace{ID: workspaceID}

		// Return empty entries
		mockQueueRepo.EXPECT().FetchPending(gomock.Any(), workspaceID, gomock.Any()).Return([]*domain.EmailQueueEntry{}, nil)

		worker := NewEmailQueueWorker(
			mockQueueRepo,
			mockWorkspaceRepo,
			mockEmailService,
			DefaultWorkerConfig(),
			mockLogger,
		)

		worker.ctx = context.Background()
		worker.processWorkspace(workspace)
		// Should complete without processing anything
	})

	t.Run("handles fetch error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockQueueRepo := mocks.NewMockEmailQueueRepository(ctrl)
		mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)

		mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

		workspaceID := "workspace-1"
		workspace := &domain.Workspace{ID: workspaceID}

		// Return error
		mockQueueRepo.EXPECT().FetchPending(gomock.Any(), workspaceID, gomock.Any()).
			Return(nil, errors.New("database error"))

		worker := NewEmailQueueWorker(
			mockQueueRepo,
			mockWorkspaceRepo,
			mockEmailService,
			DefaultWorkerConfig(),
			mockLogger,
		)

		worker.ctx = context.Background()
		worker.processWorkspace(workspace)
		// Should log error and return
	})
}

func TestEmailQueueWorker_ProcessAllWorkspaces(t *testing.T) {
	t.Run("processes multiple workspaces", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockQueueRepo := mocks.NewMockEmailQueueRepository(ctrl)
		mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)

		workspaces := []*domain.Workspace{
			{ID: "workspace-1"},
			{ID: "workspace-2"},
		}

		mockWorkspaceRepo.EXPECT().List(gomock.Any()).Return(workspaces, nil)

		// Each workspace will fetch (and return empty)
		mockQueueRepo.EXPECT().FetchPending(gomock.Any(), "workspace-1", gomock.Any()).Return([]*domain.EmailQueueEntry{}, nil)
		mockQueueRepo.EXPECT().FetchPending(gomock.Any(), "workspace-2", gomock.Any()).Return([]*domain.EmailQueueEntry{}, nil)

		worker := NewEmailQueueWorker(
			mockQueueRepo,
			mockWorkspaceRepo,
			mockEmailService,
			DefaultWorkerConfig(),
			mockLogger,
		)

		worker.ctx = context.Background()
		worker.processAllWorkspaces()
	})

	t.Run("handles workspace list error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockQueueRepo := mocks.NewMockEmailQueueRepository(ctrl)
		mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)

		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

		mockWorkspaceRepo.EXPECT().List(gomock.Any()).Return(nil, errors.New("database error"))

		worker := NewEmailQueueWorker(
			mockQueueRepo,
			mockWorkspaceRepo,
			mockEmailService,
			DefaultWorkerConfig(),
			mockLogger,
		)

		worker.ctx = context.Background()
		worker.processAllWorkspaces()
		// Should log error and return
	})
}

func TestEmailQueueWorker_CallbacksOptional(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueueRepo := mocks.NewMockEmailQueueRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()

	integrationID := "integration-1"
	workspaceID := "workspace-1"
	entryID := "entry-1"

	workspace := &domain.Workspace{
		ID: workspaceID,
		Integrations: []domain.Integration{
			{
				ID: integrationID,
				EmailProvider: domain.EmailProvider{
					Kind:               domain.EmailProviderKindSMTP,
					RateLimitPerMinute: 100,
				},
			},
		},
	}

	entry := &domain.EmailQueueEntry{
		ID:            entryID,
		Status:        domain.EmailQueueStatusPending,
		SourceType:    domain.EmailQueueSourceBroadcast,
		SourceID:      "broadcast-1",
		IntegrationID: integrationID,
		ContactEmail:  "test@example.com",
		MessageID:     "msg-1",
		Payload: domain.EmailQueuePayload{
			RateLimitPerMinute: 100,
		},
		Attempts:    0,
		MaxAttempts: 3,
	}

	mockQueueRepo.EXPECT().MarkAsProcessing(gomock.Any(), workspaceID, entryID).Return(nil)
	mockEmailService.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).Return(nil)
	mockQueueRepo.EXPECT().MarkAsSent(gomock.Any(), workspaceID, entryID).Return(nil)

	worker := NewEmailQueueWorker(
		mockQueueRepo,
		mockWorkspaceRepo,
		mockEmailService,
		DefaultWorkerConfig(),
		mockLogger,
	)
	worker.ctx = context.Background()

	// Don't set callbacks - should work without panicking
	worker.processEntry(workspace, entry)
}

func TestEmailQueueWorker_RateLimiting(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueueRepo := mocks.NewMockEmailQueueRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

	integrationID := "integration-1"
	workspaceID := "workspace-1"

	workspace := &domain.Workspace{
		ID: workspaceID,
		Integrations: []domain.Integration{
			{
				ID: integrationID,
				EmailProvider: domain.EmailProvider{
					Kind:               domain.EmailProviderKindSMTP,
					RateLimitPerMinute: 6000, // 100 per second
				},
			},
		},
	}

	// Process multiple entries to exercise rate limiting
	var processedCount int32

	for i := 0; i < 3; i++ {
		entry := &domain.EmailQueueEntry{
			ID:            "entry-" + string(rune('1'+i)),
			Status:        domain.EmailQueueStatusPending,
			SourceType:    domain.EmailQueueSourceBroadcast,
			SourceID:      "broadcast-1",
			IntegrationID: integrationID,
			ContactEmail:  "test@example.com",
			MessageID:     "msg-" + string(rune('1'+i)),
			Payload: domain.EmailQueuePayload{
				RateLimitPerMinute: 6000,
			},
			Attempts:    0,
			MaxAttempts: 3,
		}

		mockQueueRepo.EXPECT().MarkAsProcessing(gomock.Any(), workspaceID, entry.ID).Return(nil)
		mockEmailService.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).DoAndReturn(
			func(ctx context.Context, req domain.SendEmailProviderRequest, isMarketing bool) error {
				atomic.AddInt32(&processedCount, 1)
				return nil
			},
		)
		mockQueueRepo.EXPECT().MarkAsSent(gomock.Any(), workspaceID, entry.ID).Return(nil)
	}

	worker := NewEmailQueueWorker(
		mockQueueRepo,
		mockWorkspaceRepo,
		mockEmailService,
		DefaultWorkerConfig(),
		mockLogger,
	)
	worker.ctx = context.Background()

	// Process entries
	for i := 0; i < 3; i++ {
		entry := &domain.EmailQueueEntry{
			ID:            "entry-" + string(rune('1'+i)),
			Status:        domain.EmailQueueStatusPending,
			SourceType:    domain.EmailQueueSourceBroadcast,
			SourceID:      "broadcast-1",
			IntegrationID: integrationID,
			ContactEmail:  "test@example.com",
			MessageID:     "msg-" + string(rune('1'+i)),
			Payload: domain.EmailQueuePayload{
				RateLimitPerMinute: 6000,
			},
			Attempts:    0,
			MaxAttempts: 3,
		}
		worker.processEntry(workspace, entry)
	}

	assert.Equal(t, int32(3), processedCount)
}

func TestEmailQueueWorker_DefaultRateLimit(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueueRepo := mocks.NewMockEmailQueueRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

	integrationID := "integration-1"
	workspaceID := "workspace-1"

	// Workspace with no rate limit configured
	workspace := &domain.Workspace{
		ID: workspaceID,
		Integrations: []domain.Integration{
			{
				ID: integrationID,
				EmailProvider: domain.EmailProvider{
					Kind:               domain.EmailProviderKindSMTP,
					RateLimitPerMinute: 0, // No rate limit configured
				},
			},
		},
	}

	entry := &domain.EmailQueueEntry{
		ID:            "entry-1",
		Status:        domain.EmailQueueStatusPending,
		SourceType:    domain.EmailQueueSourceBroadcast,
		SourceID:      "broadcast-1",
		IntegrationID: integrationID,
		ContactEmail:  "test@example.com",
		MessageID:     "msg-1",
		Payload: domain.EmailQueuePayload{
			RateLimitPerMinute: 0, // No rate limit in payload either
		},
		Attempts:    0,
		MaxAttempts: 3,
	}

	mockQueueRepo.EXPECT().MarkAsProcessing(gomock.Any(), workspaceID, "entry-1").Return(nil)
	mockEmailService.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).Return(nil)
	mockQueueRepo.EXPECT().MarkAsSent(gomock.Any(), workspaceID, "entry-1").Return(nil)

	worker := NewEmailQueueWorker(
		mockQueueRepo,
		mockWorkspaceRepo,
		mockEmailService,
		DefaultWorkerConfig(),
		mockLogger,
	)
	worker.ctx = context.Background()

	// Should use default rate limit of 60 (1 per second)
	worker.processEntry(workspace, entry)

	// Check the rate limiter has the integration with default rate
	rate := worker.rateLimiter.GetCurrentRate(integrationID)
	// Default is 60/min = 1/sec
	assert.InDelta(t, 1.0, rate, 0.001)
}
