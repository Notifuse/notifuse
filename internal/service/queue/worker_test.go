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
		mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
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
			mockMessageHistoryRepo,
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
		mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)

		worker := NewEmailQueueWorker(
			mockQueueRepo,
			mockWorkspaceRepo,
			mockEmailService,
			mockMessageHistoryRepo,
			nil, // nil config
			mockLogger,
		)

		require.NotNil(t, worker)
		assert.Equal(t, 5, worker.config.WorkerCount)
		assert.Equal(t, 1*time.Second, worker.config.PollInterval)
	})
}

func TestEmailQueueWorker_StartStop(t *testing.T) {
	t.Run("start sets running to true", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockQueueRepo := mocks.NewMockEmailQueueRepository(ctrl)
		mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
		mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
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
			mockMessageHistoryRepo,
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
		mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
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
			mockMessageHistoryRepo,
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
		mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)

		mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()

		mockWorkspaceRepo.EXPECT().List(gomock.Any()).Return([]*domain.Workspace{}, nil).AnyTimes()

		worker := NewEmailQueueWorker(
			mockQueueRepo,
			mockWorkspaceRepo,
			mockEmailService,
			mockMessageHistoryRepo,
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
		mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)

		mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()

		mockWorkspaceRepo.EXPECT().List(gomock.Any()).Return([]*domain.Workspace{}, nil).AnyTimes()

		worker := NewEmailQueueWorker(
			mockQueueRepo,
			mockWorkspaceRepo,
			mockEmailService,
			mockMessageHistoryRepo,
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
	mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
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
	mockMessageHistoryRepo.EXPECT().Upsert(gomock.Any(), workspaceID, gomock.Any(), gomock.Any()).Return(nil)
	mockQueueRepo.EXPECT().MarkAsSent(gomock.Any(), workspaceID, entryID).Return(nil)

	worker := NewEmailQueueWorker(
		mockQueueRepo,
		mockWorkspaceRepo,
		mockEmailService,
		mockMessageHistoryRepo,
		DefaultWorkerConfig(),
		mockLogger,
	)
	worker.ctx = context.Background()

	// Process the entry
	worker.processEntry(workspace, entry)
}

func TestEmailQueueWorker_ProcessEntry_SendFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueueRepo := mocks.NewMockEmailQueueRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
	mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
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
	mockMessageHistoryRepo.EXPECT().Upsert(gomock.Any(), workspaceID, gomock.Any(), gomock.Any()).Return(nil)
	// After failure, should schedule retry
	mockQueueRepo.EXPECT().MarkAsFailed(gomock.Any(), workspaceID, entryID, sendErr.Error(), gomock.Any()).Return(nil)

	worker := NewEmailQueueWorker(
		mockQueueRepo,
		mockWorkspaceRepo,
		mockEmailService,
		mockMessageHistoryRepo,
		DefaultWorkerConfig(),
		mockLogger,
	)
	worker.ctx = context.Background()

	// Process the entry
	worker.processEntry(workspace, entry)
}

func TestEmailQueueWorker_ProcessEntry_MaxAttemptsExceeded(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueueRepo := mocks.NewMockEmailQueueRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
	mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
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
		MaxAttempts: 3, // Max is 3, so after this attempt it should be deleted
	}

	sendErr := errors.New("SMTP connection failed")

	// Expect calls in order
	mockQueueRepo.EXPECT().MarkAsProcessing(gomock.Any(), workspaceID, entryID).Return(nil)
	mockEmailService.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).Return(sendErr)
	mockMessageHistoryRepo.EXPECT().Upsert(gomock.Any(), workspaceID, gomock.Any(), gomock.Any()).Return(nil)
	// Should delete the entry since attempts >= maxAttempts after increment (message_history tracks failure)
	mockQueueRepo.EXPECT().Delete(gomock.Any(), workspaceID, entryID).Return(nil)

	worker := NewEmailQueueWorker(
		mockQueueRepo,
		mockWorkspaceRepo,
		mockEmailService,
		mockMessageHistoryRepo,
		DefaultWorkerConfig(),
		mockLogger,
	)
	worker.ctx = context.Background()

	// Process the entry
	worker.processEntry(workspace, entry)
}

func TestEmailQueueWorker_ProcessEntry_IntegrationNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueueRepo := mocks.NewMockEmailQueueRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
	mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
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
	// Upsert message history is called even when integration not found (error case)
	mockMessageHistoryRepo.EXPECT().Upsert(gomock.Any(), workspaceID, gomock.Any(), gomock.Any()).Return(nil)
	// Expect mark as failed due to integration not found (will retry)
	mockQueueRepo.EXPECT().MarkAsFailed(gomock.Any(), workspaceID, entryID, gomock.Any(), gomock.Any()).Return(nil)

	worker := NewEmailQueueWorker(
		mockQueueRepo,
		mockWorkspaceRepo,
		mockEmailService,
		mockMessageHistoryRepo,
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
	mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
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
		mockMessageHistoryRepo,
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
	mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	worker := NewEmailQueueWorker(
		mockQueueRepo,
		mockWorkspaceRepo,
		mockEmailService,
		mockMessageHistoryRepo,
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
	mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
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
		mockMessageHistoryRepo,
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
		mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
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
		mockMessageHistoryRepo.EXPECT().Upsert(gomock.Any(), workspaceID, gomock.Any(), gomock.Any()).Return(nil)
		mockQueueRepo.EXPECT().MarkAsSent(gomock.Any(), workspaceID, "entry-1").Return(nil)

		worker := NewEmailQueueWorker(
			mockQueueRepo,
			mockWorkspaceRepo,
			mockEmailService,
			mockMessageHistoryRepo,
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
		mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)

		workspaceID := "workspace-1"
		workspace := &domain.Workspace{ID: workspaceID}

		// Return empty entries
		mockQueueRepo.EXPECT().FetchPending(gomock.Any(), workspaceID, gomock.Any()).Return([]*domain.EmailQueueEntry{}, nil)

		worker := NewEmailQueueWorker(
			mockQueueRepo,
			mockWorkspaceRepo,
			mockEmailService,
			mockMessageHistoryRepo,
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
		mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
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
			mockMessageHistoryRepo,
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
		mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
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
			mockMessageHistoryRepo,
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
		mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
		mockLogger := pkgmocks.NewMockLogger(ctrl)

		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

		mockWorkspaceRepo.EXPECT().List(gomock.Any()).Return(nil, errors.New("database error"))

		worker := NewEmailQueueWorker(
			mockQueueRepo,
			mockWorkspaceRepo,
			mockEmailService,
			mockMessageHistoryRepo,
			DefaultWorkerConfig(),
			mockLogger,
		)

		worker.ctx = context.Background()
		worker.processAllWorkspaces()
		// Should log error and return
	})
}

func TestEmailQueueWorker_ProcessWithoutCallbacks(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueueRepo := mocks.NewMockEmailQueueRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
	mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
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
	mockMessageHistoryRepo.EXPECT().Upsert(gomock.Any(), workspaceID, gomock.Any(), gomock.Any()).Return(nil)
	mockQueueRepo.EXPECT().MarkAsSent(gomock.Any(), workspaceID, entryID).Return(nil)

	worker := NewEmailQueueWorker(
		mockQueueRepo,
		mockWorkspaceRepo,
		mockEmailService,
		mockMessageHistoryRepo,
		DefaultWorkerConfig(),
		mockLogger,
	)
	worker.ctx = context.Background()

	// Should work without panicking (no callbacks needed anymore)
	worker.processEntry(workspace, entry)
}

func TestEmailQueueWorker_RateLimiting(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueueRepo := mocks.NewMockEmailQueueRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
	mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
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
		mockMessageHistoryRepo.EXPECT().Upsert(gomock.Any(), workspaceID, gomock.Any(), gomock.Any()).Return(nil)
		mockQueueRepo.EXPECT().MarkAsSent(gomock.Any(), workspaceID, entry.ID).Return(nil)
	}

	worker := NewEmailQueueWorker(
		mockQueueRepo,
		mockWorkspaceRepo,
		mockEmailService,
		mockMessageHistoryRepo,
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
	mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
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
	mockMessageHistoryRepo.EXPECT().Upsert(gomock.Any(), workspaceID, gomock.Any(), gomock.Any()).Return(nil)
	mockQueueRepo.EXPECT().MarkAsSent(gomock.Any(), workspaceID, "entry-1").Return(nil)

	worker := NewEmailQueueWorker(
		mockQueueRepo,
		mockWorkspaceRepo,
		mockEmailService,
		mockMessageHistoryRepo,
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

func TestEmailQueueWorker_ProcessEntry_StoresTemplateDataInMessageHistory(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueueRepo := mocks.NewMockEmailQueueRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
	mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Setup logger
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()

	integrationID := "integration-1"
	entryID := "entry-1"
	workspaceID := "workspace-1"

	workspace := &domain.Workspace{
		ID: workspaceID,
		Settings: domain.WorkspaceSettings{
			SecretKey: "test-secret",
		},
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

	// Create entry with template data (as it would be after the fix)
	templateData := map[string]interface{}{
		"contact": map[string]interface{}{
			"email":      "test@example.com",
			"first_name": "John",
		},
		"unsubscribe_url":         "https://example.com/unsub?token=abc",
		"notification_center_url": "https://example.com/notification-center?wid=workspace-1",
		"broadcast": map[string]interface{}{
			"id":   "broadcast-1",
			"name": "Weekly Newsletter",
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
		TemplateID:    "template-1",
		Payload: domain.EmailQueuePayload{
			FromAddress:        "sender@example.com",
			FromName:           "Sender",
			Subject:            "Test Subject",
			HTMLContent:        "<p>Hello</p>",
			RateLimitPerMinute: 100,
			TemplateVersion:    1,
			ListID:             "list-1",
			TemplateData:       templateData, // This is what we're testing
		},
		Attempts:    0,
		MaxAttempts: 3,
	}

	// Expect calls in order
	mockQueueRepo.EXPECT().MarkAsProcessing(gomock.Any(), workspaceID, entryID).Return(nil)
	mockEmailService.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).Return(nil)

	// Verify that message history receives the template data
	mockMessageHistoryRepo.EXPECT().Upsert(gomock.Any(), workspaceID, gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, wid, secretKey string, msg *domain.MessageHistory) error {
			// Verify template data is passed to message history
			assert.NotNil(t, msg.MessageData.Data, "MessageData.Data should not be nil")
			assert.Equal(t, templateData, msg.MessageData.Data, "Template data should be passed to message history")

			// Verify other fields are set correctly
			assert.Equal(t, "msg-1", msg.ID)
			assert.Equal(t, "test@example.com", msg.ContactEmail)
			assert.Equal(t, "template-1", msg.TemplateID)
			assert.Equal(t, int64(1), msg.TemplateVersion)
			assert.Equal(t, "email", msg.Channel)
			assert.NotNil(t, msg.BroadcastID)
			assert.Equal(t, "broadcast-1", *msg.BroadcastID)
			assert.NotNil(t, msg.ListID)
			assert.Equal(t, "list-1", *msg.ListID)

			return nil
		})

	mockQueueRepo.EXPECT().MarkAsSent(gomock.Any(), workspaceID, entryID).Return(nil)

	worker := NewEmailQueueWorker(
		mockQueueRepo,
		mockWorkspaceRepo,
		mockEmailService,
		mockMessageHistoryRepo,
		DefaultWorkerConfig(),
		mockLogger,
	)
	worker.ctx = context.Background()

	// Process the entry
	worker.processEntry(workspace, entry)
}

func TestEmailQueueWorker_GetMinEmailRateLimit(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueueRepo := mocks.NewMockEmailQueueRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
	mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	worker := NewEmailQueueWorker(
		mockQueueRepo,
		mockWorkspaceRepo,
		mockEmailService,
		mockMessageHistoryRepo,
		nil,
		mockLogger,
	)

	t.Run("returns default when no email integrations", func(t *testing.T) {
		workspace := &domain.Workspace{
			ID:           "workspace-1",
			Integrations: []domain.Integration{},
		}

		rate := worker.getMinEmailRateLimit(workspace)
		assert.Equal(t, 60, rate)
	})

	t.Run("returns single integration rate", func(t *testing.T) {
		workspace := &domain.Workspace{
			ID: "workspace-1",
			Integrations: []domain.Integration{
				{
					ID:   "integration-1",
					Type: domain.IntegrationTypeEmail,
					EmailProvider: domain.EmailProvider{
						Kind:               domain.EmailProviderKindSMTP,
						RateLimitPerMinute: 20,
					},
				},
			},
		}

		rate := worker.getMinEmailRateLimit(workspace)
		assert.Equal(t, 20, rate)
	})

	t.Run("returns minimum across multiple integrations", func(t *testing.T) {
		workspace := &domain.Workspace{
			ID: "workspace-1",
			Integrations: []domain.Integration{
				{
					ID:   "integration-1",
					Type: domain.IntegrationTypeEmail,
					EmailProvider: domain.EmailProvider{
						Kind:               domain.EmailProviderKindSMTP,
						RateLimitPerMinute: 100,
					},
				},
				{
					ID:   "integration-2",
					Type: domain.IntegrationTypeEmail,
					EmailProvider: domain.EmailProvider{
						Kind:               domain.EmailProviderKindSES,
						RateLimitPerMinute: 20, // This is the minimum
					},
				},
				{
					ID:   "integration-3",
					Type: domain.IntegrationTypeEmail,
					EmailProvider: domain.EmailProvider{
						Kind:               domain.EmailProviderKindSMTP,
						RateLimitPerMinute: 50,
					},
				},
			},
		}

		rate := worker.getMinEmailRateLimit(workspace)
		assert.Equal(t, 20, rate)
	})

	t.Run("ignores non-email integrations", func(t *testing.T) {
		workspace := &domain.Workspace{
			ID: "workspace-1",
			Integrations: []domain.Integration{
				{
					ID:   "integration-1",
					Type: domain.IntegrationTypeSupabase,
				},
				{
					ID:   "integration-2",
					Type: domain.IntegrationTypeEmail,
					EmailProvider: domain.EmailProvider{
						Kind:               domain.EmailProviderKindSMTP,
						RateLimitPerMinute: 30,
					},
				},
			},
		}

		rate := worker.getMinEmailRateLimit(workspace)
		assert.Equal(t, 30, rate)
	})
}

func TestEmailQueueWorker_DynamicBatchSize(t *testing.T) {
	t.Run("calculates effective batch size based on rate limit", func(t *testing.T) {
		// Test the formula: (minRate * 45) / 60
		testCases := []struct {
			name              string
			minRate           int
			configBatchSize   int
			expectedBatchSize int
		}{
			{"low rate 20/min", 20, 50, 15},      // (20*45)/60 = 15
			{"medium rate 60/min", 60, 50, 45},   // (60*45)/60 = 45
			{"high rate 1000/min", 1000, 50, 50}, // (1000*45)/60 = 750, capped at 50
			{"very low rate 10/min", 10, 50, 7},  // (10*45)/60 = 7
			{"rate 1/min", 1, 50, 1},             // (1*45)/60 = 0, min is 1
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// Apply the same formula as in processWorkspace
				effectiveBatchSize := (tc.minRate * 45) / 60
				if effectiveBatchSize < 1 {
					effectiveBatchSize = 1
				}
				if effectiveBatchSize > tc.configBatchSize {
					effectiveBatchSize = tc.configBatchSize
				}

				assert.Equal(t, tc.expectedBatchSize, effectiveBatchSize)
			})
		}
	})
}
