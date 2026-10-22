package queue

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/pkg/emailerror"

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
	assert.Equal(t, 200, config.BatchSize)
	assert.Equal(t, 3, config.MaxRetries)
	assert.Equal(t, 10, config.SendConcurrency)
	assert.Equal(t, 1, config.SMTPSendConcurrency)
	assert.Equal(t, 2*time.Second, config.RoundWindow)
	assert.Equal(t, 10*time.Second, config.ShutdownGrace)
	assert.Equal(t, 5*time.Second, config.AbortWait)
	assert.Equal(t, time.Minute, config.OrphanCheckInterval)
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
		assert.Equal(t, 3, worker.config.WorkerCount)
		assert.Equal(t, 2*time.Second, worker.config.PollInterval)
		assert.Equal(t, 25, worker.config.BatchSize)
		assert.Equal(t, 5, worker.config.MaxRetries)
		assert.NotNil(t, worker.rateLimiter)
	})

	t.Run("fills the sending settings left at zero, without changing the caller's config", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		config := &EmailQueueWorkerConfig{WorkerCount: 3, PollInterval: time.Second}
		worker := NewEmailQueueWorker(
			mocks.NewMockEmailQueueRepository(ctrl),
			mocks.NewMockWorkspaceRepository(ctrl),
			mocks.NewMockEmailServiceInterface(ctrl),
			mocks.NewMockMessageHistoryRepository(ctrl),
			config,
			pkgmocks.NewMockLogger(ctrl),
		)

		defaults := DefaultWorkerConfig()
		got := worker.GetConfig()
		assert.Equal(t, 3, got.WorkerCount)
		assert.Equal(t, defaults.BatchSize, got.BatchSize)
		assert.Equal(t, defaults.SendConcurrency, got.SendConcurrency)
		assert.Equal(t, defaults.SMTPSendConcurrency, got.SMTPSendConcurrency)
		assert.Equal(t, defaults.RoundWindow, got.RoundWindow)
		assert.Equal(t, defaults.ShutdownGrace, got.ShutdownGrace)
		assert.Equal(t, defaults.AbortWait, got.AbortWait)
		assert.Equal(t, defaults.OrphanCheckInterval, got.OrphanCheckInterval)

		assert.Zero(t, config.BatchSize)
		assert.Zero(t, config.SendConcurrency)
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
	worker.initContexts(context.Background())

	// Process the entry
	processOne(worker, workspace, entry)
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
	worker.initContexts(context.Background())

	// Process the entry
	processOne(worker, workspace, entry)
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
		MaxAttempts: 3, // Max is 3, so this attempt exhausts the budget
	}

	sendErr := errors.New("SMTP connection failed")

	// Expect calls in order
	mockQueueRepo.EXPECT().MarkAsProcessing(gomock.Any(), workspaceID, entryID).Return(nil)
	mockEmailService.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).Return(sendErr)
	mockMessageHistoryRepo.EXPECT().Upsert(gomock.Any(), workspaceID, gomock.Any(), gomock.Any()).Return(nil)
	// The entry is held in a terminal state rather than deleted. Deleting it left no
	// record of who the campaign failed to reach and nothing a retry could revive.
	mockQueueRepo.EXPECT().
		MarkAsPermanentlyFailed(gomock.Any(), workspaceID, entryID, sendErr.Error()).
		Return(nil)

	worker := NewEmailQueueWorker(
		mockQueueRepo,
		mockWorkspaceRepo,
		mockEmailService,
		mockMessageHistoryRepo,
		DefaultWorkerConfig(),
		mockLogger,
	)
	worker.initContexts(context.Background())

	// Process the entry
	processOne(worker, workspace, entry)
}

// TestEmailQueueWorker_FailOrphan covers a row whose email integration was deleted: it
// spends an attempt like any failure, so it ends in a terminal state instead of waiting.
func TestEmailQueueWorker_FailOrphan(t *testing.T) {
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
	worker.initContexts(context.Background())

	worker.failOrphan(workspace, entry)
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
		Integrations: []domain.Integration{{
			ID:            "integration-1",
			Type:          domain.IntegrationTypeEmail,
			EmailProvider: domain.EmailProvider{Kind: domain.EmailProviderKindSMTP, RateLimitPerMinute: 100},
		}},
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
	worker.initContexts(context.Background())

	// Should return early without sending
	processOne(worker, workspace, entry)
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
			ID:           workspaceID,
			Integrations: []domain.Integration{emailIntegration(integrationID, domain.EmailProviderKindSMTP, 6000)},
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

		// 6,000/min fills a 2 s round with 200 rows, the per-lane maximum.
		mockQueueRepo.EXPECT().FetchPendingForIntegration(gomock.Any(), workspaceID, integrationID, 200).Return(entries, nil)
		mockQueueRepo.EXPECT().FetchPendingOrphans(gomock.Any(), workspaceID, []string{integrationID}, orphanBatchSize).Return(nil, nil)
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

		worker.initContexts(context.Background())
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
		workspace := &domain.Workspace{
			ID:           workspaceID,
			Integrations: []domain.Integration{emailIntegration("integration-1", domain.EmailProviderKindSMTP, 100)},
		}

		// Return empty entries
		mockQueueRepo.EXPECT().FetchPendingForIntegration(gomock.Any(), workspaceID, "integration-1", gomock.Any()).Return([]*domain.EmailQueueEntry{}, nil)
		mockQueueRepo.EXPECT().FetchPendingOrphans(gomock.Any(), workspaceID, []string{"integration-1"}, orphanBatchSize).Return(nil, nil)

		worker := NewEmailQueueWorker(
			mockQueueRepo,
			mockWorkspaceRepo,
			mockEmailService,
			mockMessageHistoryRepo,
			DefaultWorkerConfig(),
			mockLogger,
		)

		worker.initContexts(context.Background())
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
		mockLogger.EXPECT().Error(gomock.Any()).Times(1)

		workspaceID := "workspace-1"
		workspace := &domain.Workspace{
			ID:           workspaceID,
			Integrations: []domain.Integration{emailIntegration("integration-1", domain.EmailProviderKindSMTP, 100)},
		}

		// Return error
		mockQueueRepo.EXPECT().FetchPendingForIntegration(gomock.Any(), workspaceID, "integration-1", gomock.Any()).
			Return(nil, errors.New("database error"))
		mockQueueRepo.EXPECT().FetchPendingOrphans(gomock.Any(), workspaceID, gomock.Any(), gomock.Any()).Return(nil, nil)

		worker := NewEmailQueueWorker(
			mockQueueRepo,
			mockWorkspaceRepo,
			mockEmailService,
			mockMessageHistoryRepo,
			DefaultWorkerConfig(),
			mockLogger,
		)

		worker.initContexts(context.Background())
		worker.processWorkspace(workspace)
		// Should log error and return
	})

	t.Run("fetches once per email integration, sized to its rate, and not for other integrations", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		queueRepo := mocks.NewMockEmailQueueRepository(ctrl)
		workspace := &domain.Workspace{
			ID: "ws-1",
			Integrations: []domain.Integration{
				emailIntegration("ses-1", domain.EmailProviderKindSES, 3600),
				// Left at the console default: one row per round, without holding up
				// the integration above.
				emailIntegration("smtp-1", domain.EmailProviderKindSMTP, 25),
				{ID: "supabase-1", Type: domain.IntegrationTypeSupabase},
			},
		}

		queueRepo.EXPECT().FetchPendingForIntegration(gomock.Any(), "ws-1", "ses-1", 120).Return(nil, nil)
		queueRepo.EXPECT().FetchPendingForIntegration(gomock.Any(), "ws-1", "smtp-1", 1).Return(nil, nil)
		queueRepo.EXPECT().FetchPendingOrphans(gomock.Any(), "ws-1", gomock.Any(), orphanBatchSize).
			DoAndReturn(func(_ context.Context, _ string, ids []string, _ int) ([]*domain.EmailQueueEntry, error) {
				assert.ElementsMatch(t, []string{"ses-1", "smtp-1"}, ids)
				return nil, nil
			})

		worker := NewEmailQueueWorker(queueRepo, mocks.NewMockWorkspaceRepository(ctrl),
			mocks.NewMockEmailServiceInterface(ctrl), mocks.NewMockMessageHistoryRepository(ctrl),
			DefaultWorkerConfig(), quietLogger(ctrl))
		worker.initContexts(context.Background())
		worker.processWorkspace(workspace)
	})

	t.Run("a workspace without email integrations only looks for orphans", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		queueRepo := mocks.NewMockEmailQueueRepository(ctrl)
		queueRepo.EXPECT().FetchPendingOrphans(gomock.Any(), "ws-1", []string{}, orphanBatchSize).Return(nil, nil)

		worker := NewEmailQueueWorker(queueRepo, mocks.NewMockWorkspaceRepository(ctrl),
			mocks.NewMockEmailServiceInterface(ctrl), mocks.NewMockMessageHistoryRepository(ctrl),
			DefaultWorkerConfig(), quietLogger(ctrl))
		worker.initContexts(context.Background())
		worker.processWorkspace(&domain.Workspace{ID: "ws-1"})
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
			{ID: "workspace-1", Integrations: []domain.Integration{emailIntegration("integration-1", domain.EmailProviderKindSMTP, 100)}},
			{ID: "workspace-2", Integrations: []domain.Integration{emailIntegration("integration-2", domain.EmailProviderKindSMTP, 100)}},
		}

		mockWorkspaceRepo.EXPECT().List(gomock.Any()).Return(workspaces, nil)

		// Each workspace will fetch (and return empty)
		mockQueueRepo.EXPECT().FetchPendingForIntegration(gomock.Any(), "workspace-1", "integration-1", gomock.Any()).Return([]*domain.EmailQueueEntry{}, nil)
		mockQueueRepo.EXPECT().FetchPendingForIntegration(gomock.Any(), "workspace-2", "integration-2", gomock.Any()).Return([]*domain.EmailQueueEntry{}, nil)
		mockQueueRepo.EXPECT().FetchPendingOrphans(gomock.Any(), "workspace-1", gomock.Any(), gomock.Any()).Return(nil, nil)
		mockQueueRepo.EXPECT().FetchPendingOrphans(gomock.Any(), "workspace-2", gomock.Any(), gomock.Any()).Return(nil, nil)

		worker := NewEmailQueueWorker(
			mockQueueRepo,
			mockWorkspaceRepo,
			mockEmailService,
			mockMessageHistoryRepo,
			DefaultWorkerConfig(),
			mockLogger,
		)

		worker.initContexts(context.Background())
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

		worker.initContexts(context.Background())
		worker.processAllWorkspaces()
		// Should log error and return
	})

	t.Run("a large queue in one workspace does not add fetches elsewhere", func(t *testing.T) {
		cfg := DefaultWorkerConfig()
		cfg.RoundWindow = 200 * time.Millisecond
		lt := newLaneTest(t, cfg, append(queuedRows("ses-1", 50), queuedRows("ses-2", 1)...)...)
		lt.email.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).Return(nil).AnyTimes()
		lt.workspaceRepo.EXPECT().List(gomock.Any()).Return([]*domain.Workspace{
			{ID: "ws-1", Integrations: []domain.Integration{emailIntegration("ses-1", domain.EmailProviderKindSES, 6000)}},
			{ID: "ws-2", Integrations: []domain.Integration{emailIntegration("ses-2", domain.EmailProviderKindSES, 6000)}},
		}, nil)

		within(t, 5*time.Second, lt.w.processAllWorkspaces)

		// 6,000/min over a 200 ms round is 20 rows: one fetch each, whatever is left.
		assert.Equal(t, []int{20}, lt.queue.fetchLimits("ses-1"))
		assert.Equal(t, []int{20}, lt.queue.fetchLimits("ses-2"))
		assert.Equal(t, 1, lt.queue.sentCount("ses-2"))
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
	worker.initContexts(context.Background())

	// Should work without panicking (no callbacks needed anymore)
	processOne(worker, workspace, entry)
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
	worker.initContexts(context.Background())

	// Process the entry
	processOne(worker, workspace, entry)
}

// An automation email is sent for the automation's list. Its message_history row must
// record that list, because the complaint/bounce trigger acts only when list_id is set.
func TestEmailQueueWorker_ProcessEntry_AutomationListIDInMessageHistory(t *testing.T) {
	listOne := "list-1"
	cases := []struct {
		name   string
		listID string
		want   *string
	}{
		{name: "automation with a list", listID: "list-1", want: &listOne},
		{name: "automation without a list", listID: "", want: nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockQueueRepo := mocks.NewMockEmailQueueRepository(ctrl)
			mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
			mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
			mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
			mockLogger := pkgmocks.NewMockLogger(ctrl)
			mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
			mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()

			workspace := &domain.Workspace{
				ID:       "workspace-1",
				Settings: domain.WorkspaceSettings{SecretKey: "test-secret"},
				Integrations: []domain.Integration{{
					ID: "integration-1",
					EmailProvider: domain.EmailProvider{
						Kind:               domain.EmailProviderKindSMTP,
						RateLimitPerMinute: 100,
					},
				}},
			}
			entry := &domain.EmailQueueEntry{
				ID:            "entry-1",
				Status:        domain.EmailQueueStatusPending,
				SourceType:    domain.EmailQueueSourceAutomation,
				SourceID:      "auto-1",
				IntegrationID: "integration-1",
				ProviderKind:  domain.EmailProviderKindSMTP,
				ContactEmail:  "test@example.com",
				MessageID:     "msg-1",
				TemplateID:    "template-1",
				Payload: domain.EmailQueuePayload{
					FromAddress:        "sender@example.com",
					FromName:           "Sender",
					Subject:            "Test Subject",
					HTMLContent:        "<p>Hello</p>",
					RateLimitPerMinute: 100,
					ListID:             tc.listID,
				},
				MaxAttempts: 3,
			}

			mockQueueRepo.EXPECT().MarkAsProcessing(gomock.Any(), "workspace-1", "entry-1").Return(nil)
			mockEmailService.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).Return(nil)
			mockMessageHistoryRepo.EXPECT().Upsert(gomock.Any(), "workspace-1", gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, wid, secretKey string, msg *domain.MessageHistory) error {
					if assert.NotNil(t, msg.AutomationID) {
						assert.Equal(t, "auto-1", *msg.AutomationID)
					}
					assert.Nil(t, msg.BroadcastID)
					assert.Equal(t, tc.want, msg.ListID)
					return nil
				})
			mockQueueRepo.EXPECT().MarkAsSent(gomock.Any(), "workspace-1", "entry-1").Return(nil)

			worker := NewEmailQueueWorker(mockQueueRepo, mockWorkspaceRepo, mockEmailService,
				mockMessageHistoryRepo, DefaultWorkerConfig(), mockLogger)
			worker.initContexts(context.Background())
			processOne(worker, workspace, entry)
		})
	}
}

func TestEmailQueueWorker_ProcessEntry_SMTPMessageIDForReplyMatching(t *testing.T) {
	cases := []struct {
		name         string
		providerKind domain.EmailProviderKind
		sourceType   domain.EmailQueueSourceType
		wantSet      bool
		wantValue    string
	}{
		// All automation sends on a set_own provider record the message-id (so a
		// reply to ANY automation is precisely attributable, not just exit_on_reply ones).
		{"mailgun automation records message-id", domain.EmailProviderKindMailgun, domain.EmailQueueSourceAutomation, true, "msg-1@example.com"},
		{"sendgrid automation leaves it nil (sender-match)", domain.EmailProviderKindSendGrid, domain.EmailQueueSourceAutomation, false, ""},
		{"mailgun broadcast leaves it nil (not an automation)", domain.EmailProviderKindMailgun, domain.EmailQueueSourceBroadcast, false, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockQueueRepo := mocks.NewMockEmailQueueRepository(ctrl)
			mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
			mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
			mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
			mockLogger := pkgmocks.NewMockLogger(ctrl)
			mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
			mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()

			workspaceID, integrationID, entryID := "ws-1", "int-1", "entry-1"
			workspace := &domain.Workspace{
				ID:       workspaceID,
				Settings: domain.WorkspaceSettings{SecretKey: "secret"},
				Integrations: []domain.Integration{{
					ID:            integrationID,
					EmailProvider: domain.EmailProvider{Kind: tc.providerKind, RateLimitPerMinute: 100},
				}},
			}

			entry := &domain.EmailQueueEntry{
				ID:            entryID,
				Status:        domain.EmailQueueStatusPending,
				SourceType:    tc.sourceType,
				SourceID:      "source-1",
				IntegrationID: integrationID,
				ProviderKind:  tc.providerKind,
				ContactEmail:  "jane@contact.com",
				MessageID:     "msg-1",
				TemplateID:    "tpl-1",
				Payload: domain.EmailQueuePayload{
					FromAddress:        "hello@example.com",
					FromName:           "Hello",
					Subject:            "Hi",
					HTMLContent:        "<p>Hi</p>",
					RateLimitPerMinute: 100,
					TemplateVersion:    1,
				},
				MaxAttempts: 3,
			}

			// Track that, for the reply-matchable case, the smtp_message_id row is persisted
			// BEFORE the email physically leaves the provider (the fix for the send/store race).
			matchableRowWritten := false
			mockQueueRepo.EXPECT().MarkAsProcessing(gomock.Any(), workspaceID, entryID).Return(nil)
			mockEmailService.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).
				DoAndReturn(func(_ context.Context, _ domain.SendEmailProviderRequest, _ bool) error {
					if tc.wantSet {
						assert.True(t, matchableRowWritten, "matchable smtp_message_id row must be persisted BEFORE SendEmail")
					}
					return nil
				})
			mockMessageHistoryRepo.EXPECT().Upsert(gomock.Any(), workspaceID, gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, wid, secretKey string, msg *domain.MessageHistory) error {
					if tc.wantSet {
						if assert.NotNil(t, msg.SMTPMessageID, "smtp_message_id should be set") {
							assert.Equal(t, tc.wantValue, *msg.SMTPMessageID)
						}
						matchableRowWritten = true
					} else {
						assert.Nil(t, msg.SMTPMessageID, "smtp_message_id should be nil")
					}
					return nil
				}).AnyTimes()
			mockQueueRepo.EXPECT().MarkAsSent(gomock.Any(), workspaceID, entryID).Return(nil)

			worker := NewEmailQueueWorker(
				mockQueueRepo,
				mockWorkspaceRepo,
				mockEmailService,
				mockMessageHistoryRepo,
				DefaultWorkerConfig(),
				mockLogger,
			)
			worker.initContexts(context.Background())
			processOne(worker, workspace, entry)
		})
	}
}

// TestEmailQueueWorker_ProcessEntry_SESCapturesMessageID covers the capture strategy: SES
// overwrites the Message-ID and returns its own, so (unlike set_own providers) there is no
// pre-send upsert — the worker must hand SES a capture pointer, then store the returned id
// reconstructed into the recipient-visible RFC Message-ID on the post-send upsert.
func TestEmailQueueWorker_ProcessEntry_SESCapturesMessageID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueueRepo := mocks.NewMockEmailQueueRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
	mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()

	workspaceID, integrationID, entryID := "ws-1", "int-1", "entry-1"
	workspace := &domain.Workspace{
		ID:       workspaceID,
		Settings: domain.WorkspaceSettings{SecretKey: "secret"},
		Integrations: []domain.Integration{{
			ID:            integrationID,
			EmailProvider: domain.EmailProvider{Kind: domain.EmailProviderKindSES, RateLimitPerMinute: 100},
		}},
	}
	entry := &domain.EmailQueueEntry{
		ID: entryID, Status: domain.EmailQueueStatusPending,
		SourceType: domain.EmailQueueSourceAutomation, SourceID: "auto-1",
		IntegrationID: integrationID, ProviderKind: domain.EmailProviderKindSES,
		ContactEmail: "jane@contact.com", MessageID: "msg-1", TemplateID: "tpl-1",
		Payload: domain.EmailQueuePayload{
			FromAddress: "hello@example.com", FromName: "Hello", Subject: "Hi",
			HTMLContent: "<p>Hi</p>", RateLimitPerMinute: 100, TemplateVersion: 1,
		},
		MaxAttempts: 3,
	}

	mockQueueRepo.EXPECT().MarkAsProcessing(gomock.Any(), workspaceID, entryID).Return(nil)
	// Simulate the SES provider writing its returned MessageId into the capture pointer.
	mockEmailService.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).
		DoAndReturn(func(_ context.Context, req domain.SendEmailProviderRequest, _ bool) error {
			if assert.NotNil(t, req.CapturedMessageID, "worker must provide a capture pointer for SES") {
				*req.CapturedMessageID = "ses-returned-id"
			}
			return nil
		})
	var stored *string
	mockMessageHistoryRepo.EXPECT().Upsert(gomock.Any(), workspaceID, gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ string, msg *domain.MessageHistory) error {
			stored = msg.SMTPMessageID
			return nil
		}).AnyTimes()
	mockQueueRepo.EXPECT().MarkAsSent(gomock.Any(), workspaceID, entryID).Return(nil)

	worker := NewEmailQueueWorker(mockQueueRepo, mockWorkspaceRepo, mockEmailService, mockMessageHistoryRepo, DefaultWorkerConfig(), mockLogger)
	worker.initContexts(context.Background())
	processOne(worker, workspace, entry)

	if assert.NotNil(t, stored, "SES automation send must store smtp_message_id post-send") {
		// Stored bare (host-independent): SES does not contractually pin the Message-ID host,
		// so we key matching off the unique local part SES returns.
		assert.Equal(t, "ses-returned-id", *stored)
	}
}

func TestEmailQueueWorker_ProcessEntry_JITGuardCancelsExitedJourney(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueueRepo := mocks.NewMockEmailQueueRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
	mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
	mockAutomationRepo := mocks.NewMockAutomationRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()

	workspaceID, integrationID, entryID, caID := "ws-1", "int-1", "entry-1", "ca-1"
	workspace := &domain.Workspace{
		ID:       workspaceID,
		Settings: domain.WorkspaceSettings{SecretKey: "secret"},
		Integrations: []domain.Integration{{
			ID:            integrationID,
			EmailProvider: domain.EmailProvider{Kind: domain.EmailProviderKindMailgun, RateLimitPerMinute: 1000},
		}},
	}
	entry := &domain.EmailQueueEntry{
		ID:            entryID,
		Status:        domain.EmailQueueStatusPending,
		SourceType:    domain.EmailQueueSourceAutomation,
		SourceID:      "auto-1",
		IntegrationID: integrationID,
		ProviderKind:  domain.EmailProviderKindMailgun,
		ContactEmail:  "jane@x.com",
		MessageID:     "m1",
		TemplateID:    "t1",
		Payload: domain.EmailQueuePayload{
			FromAddress: "h@x.com", FromName: "H", Subject: "s", HTMLContent: "<p>x</p>",
			RateLimitPerMinute: 1000, ContactAutomationID: &caID,
		},
		MaxAttempts: 3,
	}

	mockQueueRepo.EXPECT().MarkAsProcessing(gomock.Any(), workspaceID, entryID).Return(nil)
	// Journey was exited (stop-on-reply) after enqueue → cancel the send.
	mockAutomationRepo.EXPECT().GetContactAutomation(gomock.Any(), workspaceID, caID).
		Return(&domain.ContactAutomation{ID: caID, Status: domain.ContactAutomationStatusExited}, nil)
	mockQueueRepo.EXPECT().MarkAsSent(gomock.Any(), workspaceID, entryID).Return(nil)
	mockEmailService.EXPECT().SendEmail(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
	mockMessageHistoryRepo.EXPECT().Upsert(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	worker := NewEmailQueueWorker(mockQueueRepo, mockWorkspaceRepo, mockEmailService, mockMessageHistoryRepo, DefaultWorkerConfig(), mockLogger)
	worker.automationRepo = mockAutomationRepo
	worker.initContexts(context.Background())
	processOne(worker, workspace, entry)
}

// jitGuardWorker builds a worker + entry flagged with a contact_automation_id, returning
// the mocks so each JIT-guard test can program the GetContactAutomation result.
func jitGuardWorker(t *testing.T) (*EmailQueueWorker, *mocks.MockEmailQueueRepository, *mocks.MockEmailServiceInterface, *mocks.MockMessageHistoryRepository, *mocks.MockAutomationRepository, *domain.Workspace, *domain.EmailQueueEntry) {
	t.Helper()
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	mockQueueRepo := mocks.NewMockEmailQueueRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
	mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
	mockAutomationRepo := mocks.NewMockAutomationRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()

	workspaceID, integrationID, caID := "ws-1", "int-1", "ca-1"
	workspace := &domain.Workspace{
		ID:       workspaceID,
		Settings: domain.WorkspaceSettings{SecretKey: "secret"},
		Integrations: []domain.Integration{{
			ID:            integrationID,
			EmailProvider: domain.EmailProvider{Kind: domain.EmailProviderKindMailgun, RateLimitPerMinute: 1000},
		}},
	}
	entry := &domain.EmailQueueEntry{
		ID: "entry-1", Status: domain.EmailQueueStatusPending,
		SourceType: domain.EmailQueueSourceAutomation, SourceID: "auto-1",
		IntegrationID: integrationID, ProviderKind: domain.EmailProviderKindMailgun,
		ContactEmail: "jane@x.com", MessageID: "m1", TemplateID: "t1",
		Payload: domain.EmailQueuePayload{
			FromAddress: "h@x.com", FromName: "H", Subject: "s", HTMLContent: "<p>x</p>",
			RateLimitPerMinute: 1000, ContactAutomationID: &caID,
		},
		MaxAttempts: 3,
	}
	mockQueueRepo.EXPECT().MarkAsProcessing(gomock.Any(), workspaceID, "entry-1").Return(nil)

	worker := NewEmailQueueWorker(mockQueueRepo, mockWorkspaceRepo, mockEmailService, mockMessageHistoryRepo, DefaultWorkerConfig(), mockLogger)
	worker.automationRepo = mockAutomationRepo
	worker.initContexts(context.Background())
	return worker, mockQueueRepo, mockEmailService, mockMessageHistoryRepo, mockAutomationRepo, workspace, entry
}

// TestEmailQueueWorker_ProcessEntry_JITGuardActiveJourneyProceeds is the control for the
// cancel test: when the journey is still active, the email MUST be sent. Together they pin
// the guard's behavior so a mutation that always-cancels OR never-cancels fails.
func TestEmailQueueWorker_ProcessEntry_JITGuardActiveJourneyProceeds(t *testing.T) {
	worker, mockQueueRepo, mockEmailService, mockMessageHistoryRepo, mockAutomationRepo, workspace, entry := jitGuardWorker(t)
	caID := *entry.Payload.ContactAutomationID

	mockAutomationRepo.EXPECT().GetContactAutomation(gomock.Any(), workspace.ID, caID).
		Return(&domain.ContactAutomation{ID: caID, Status: domain.ContactAutomationStatusActive}, nil)
	mockEmailService.EXPECT().SendEmail(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)
	mockQueueRepo.EXPECT().MarkAsSent(gomock.Any(), workspace.ID, entry.ID).Return(nil)
	mockMessageHistoryRepo.EXPECT().Upsert(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	processOne(worker, workspace, entry)
}

// TestEmailQueueWorker_ProcessEntry_JITGuardLookupErrorFailsOpen documents the fail-open
// contract: if the guard lookup errors, the send still proceeds (a missed lookup must not
// silently drop a legitimate email).
func TestEmailQueueWorker_ProcessEntry_JITGuardLookupErrorFailsOpen(t *testing.T) {
	worker, mockQueueRepo, mockEmailService, mockMessageHistoryRepo, mockAutomationRepo, workspace, entry := jitGuardWorker(t)
	caID := *entry.Payload.ContactAutomationID

	mockAutomationRepo.EXPECT().GetContactAutomation(gomock.Any(), workspace.ID, caID).
		Return(nil, errors.New("db down"))
	mockEmailService.EXPECT().SendEmail(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)
	mockQueueRepo.EXPECT().MarkAsSent(gomock.Any(), workspace.ID, entry.ID).Return(nil)
	mockMessageHistoryRepo.EXPECT().Upsert(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	processOne(worker, workspace, entry)
}

// TestEmailQueueWorker_DeadMailboxIsTerminalOnTheFirstAttempt joins the classifier to
// the worker. A mailbox that does not exist will not start existing, so retrying it
// twice more only delays the campaign and — while every SMTP rejection classified as
// unknown — pushed the workspace's circuit breaker toward opening on ordinary bounces.
func TestEmailQueueWorker_DeadMailboxIsTerminalOnTheFirstAttempt(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockQueueRepo := mocks.NewMockEmailQueueRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockEmailService := mocks.NewMockEmailServiceInterface(ctrl)
	mockMessageHistoryRepo := mocks.NewMockMessageHistoryRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	workspace := &domain.Workspace{
		ID: "workspace-1",
		Integrations: []domain.Integration{{
			ID: "integration-1",
			EmailProvider: domain.EmailProvider{
				Kind:               domain.EmailProviderKindSMTP,
				RateLimitPerMinute: 100,
			},
		}},
	}

	entry := &domain.EmailQueueEntry{
		ID:            "entry-1",
		Status:        domain.EmailQueueStatusPending,
		SourceType:    domain.EmailQueueSourceBroadcast,
		SourceID:      "broadcast-1",
		IntegrationID: "integration-1",
		ContactEmail:  "dead@example.com",
		MessageID:     "msg-1",
		Attempts:      0,
		MaxAttempts:   3,
	}

	sendErr := errors.New("RCPT TO rejected for dead@example.com with code 550: 5.1.1 User unknown")

	mockQueueRepo.EXPECT().MarkAsProcessing(gomock.Any(), "workspace-1", "entry-1").Return(nil)
	mockEmailService.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).Return(sendErr)
	mockMessageHistoryRepo.EXPECT().Upsert(gomock.Any(), "workspace-1", gomock.Any(), gomock.Any()).Return(nil)
	mockQueueRepo.EXPECT().
		MarkAsPermanentlyFailed(gomock.Any(), "workspace-1", "entry-1", sendErr.Error()).
		Return(nil)

	worker := NewEmailQueueWorker(mockQueueRepo, mockWorkspaceRepo, mockEmailService,
		mockMessageHistoryRepo, DefaultWorkerConfig(), mockLogger)
	worker.initContexts(context.Background())

	processOne(worker, workspace, entry)

	// A recipient rejection is the recipient's problem, not the provider's, so it must
	// leave the breaker alone: five dead addresses in a row must not throttle a whole
	// workspace, and once a breaker opens it pauses the campaign.
	assert.False(t, worker.circuitBreaker.IsOpen("integration-1"))
	assert.Equal(t, 0, worker.GetCircuitBreakerStats()["integration-1"].Failures)
}

// TestEmailQueueWorker_RetentionSweep covers the cost of keeping abandoned entries:
// each holds one recipient's compiled email, so the sweep has to run — but it walks
// every workspace, so it must not run on every one-second poll.
func TestEmailQueueWorker_RetentionSweep(t *testing.T) {
	newWorker := func(t *testing.T, cfg *EmailQueueWorkerConfig) (*EmailQueueWorker, *mocks.MockEmailQueueRepository, *mocks.MockWorkspaceRepository) {
		ctrl := gomock.NewController(t)
		t.Cleanup(ctrl.Finish)

		queueRepo := mocks.NewMockEmailQueueRepository(ctrl)
		workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		log := pkgmocks.NewMockLogger(ctrl)
		log.EXPECT().WithFields(gomock.Any()).Return(log).AnyTimes()
		log.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(log).AnyTimes()
		log.EXPECT().Info(gomock.Any()).AnyTimes()
		log.EXPECT().Error(gomock.Any()).AnyTimes()

		w := NewEmailQueueWorker(queueRepo, workspaceRepo,
			mocks.NewMockEmailServiceInterface(ctrl), mocks.NewMockMessageHistoryRepository(ctrl),
			cfg, log)
		w.initContexts(context.Background())
		// The worker seeds lastCleanupTime so the sweep does not fire during startup;
		// wind it back so these cases can exercise the sweep itself.
		w.lastCleanupTime = time.Time{}
		return w, queueRepo, workspaceRepo
	}

	t.Run("sweeps each workspace past the retention window", func(t *testing.T) {
		cfg := DefaultWorkerConfig()
		w, queueRepo, workspaceRepo := newWorker(t, cfg)

		workspaceRepo.EXPECT().List(gomock.Any()).
			Return([]*domain.Workspace{{ID: "ws-1"}, {ID: "ws-2"}}, nil)
		queueRepo.EXPECT().DeleteTerminallyFailedOlderThan(gomock.Any(), "ws-1", 7).Return(int64(4), nil)
		queueRepo.EXPECT().DeleteTerminallyFailedOlderThan(gomock.Any(), "ws-2", 7).Return(int64(0), nil)

		w.cleanupTerminallyFailed()
	})

	t.Run("does not sweep during startup", func(t *testing.T) {
		// Left at the zero time it would DELETE across every workspace database on the
		// first poll tick, a second after boot, when nothing can have aged out yet.
		cfg := DefaultWorkerConfig()
		w, _, _ := newWorker(t, cfg)
		w.lastCleanupTime = time.Now()

		w.cleanupTerminallyFailed()
	})

	t.Run("does not run again inside the interval", func(t *testing.T) {
		cfg := DefaultWorkerConfig()
		w, queueRepo, workspaceRepo := newWorker(t, cfg)

		workspaceRepo.EXPECT().List(gomock.Any()).Return([]*domain.Workspace{{ID: "ws-1"}}, nil).Times(1)
		queueRepo.EXPECT().DeleteTerminallyFailedOlderThan(gomock.Any(), "ws-1", 7).Return(int64(1), nil).Times(1)

		w.cleanupTerminallyFailed()
		w.cleanupTerminallyFailed()
		w.cleanupTerminallyFailed()
	})

	t.Run("retention switched off means nothing is ever swept", func(t *testing.T) {
		cfg := DefaultWorkerConfig()
		cfg.TerminalRetentionDays = 0
		w, _, _ := newWorker(t, cfg)

		w.cleanupTerminallyFailed()
	})
}

// TestEmailQueueWorker_CircuitOpenCallback covers the signal that stops a campaign when
// a provider starts refusing everything. It has to fire on the transition, not on the
// state: every send in flight sees one outage, so a large queue would otherwise trigger
// a pause storm for it.
func TestEmailQueueWorker_CircuitOpenCallback(t *testing.T) {
	newLane := func(t *testing.T, threshold, rows int) (*laneTest, *recorder, *atomic.Int32) {
		cfg := DefaultWorkerConfig()
		cfg.CircuitBreakerThreshold = threshold
		cfg.CircuitBreakerCooldown = 50 * time.Millisecond
		lt := newLaneTest(t, cfg, queuedRows("intg-1", rows)...)

		sends := &atomic.Int32{}
		lt.email.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).
			DoAndReturn(func(context.Context, domain.SendEmailProviderRequest, bool) error {
				sends.Add(1)
				return errors.New("message rejected with code 452: 4.3.1 quota exceeded")
			}).AnyTimes()

		opened := &recorder{}
		lt.w.SetCircuitOpenCallback(func(workspaceID, integrationID string, _ *emailerror.ClassifiedError) {
			opened.add(workspaceID + "/" + integrationID)
		})
		return lt, opened, sends
	}

	workspace := &domain.Workspace{ID: "ws-1", Integrations: []domain.Integration{
		emailIntegration("intg-1", domain.EmailProviderKindSMTP, 6000),
	}}
	round := func(t *testing.T, lt *laneTest) {
		within(t, 5*time.Second, func() { lt.w.processWorkspace(workspace) })
	}

	t.Run("fires once when the circuit opens, not per entry", func(t *testing.T) {
		lt, opened, sends := newLane(t, 3, 8)

		round(t, lt)

		// Entries after the third are left queued by the open breaker, so they never
		// reach the provider and never record another failure.
		assert.Equal(t, []string{"ws-1/intg-1"}, opened.list())
		assert.Equal(t, int32(3), sends.Load())
	})

	t.Run("fires again when the probe fails after the cooldown", func(t *testing.T) {
		lt, opened, sends := newLane(t, 3, 8)

		round(t, lt)
		time.Sleep(60 * time.Millisecond)
		round(t, lt)

		assert.Equal(t, []string{"ws-1/intg-1", "ws-1/intg-1"}, opened.list())
		assert.Equal(t, int32(4), sends.Load(), "one probe after the cooldown")
	})

	t.Run("an unset callback leaves the breaker throttling silently", func(t *testing.T) {
		lt, _, _ := newLane(t, 2, 4)
		lt.w.SetCircuitOpenCallback(nil)

		round(t, lt)

		assert.True(t, lt.w.circuitBreaker.IsOpen("intg-1"))
	})
}

// processOne sends one row the way a lane does once the row's token is due.
func processOne(w *EmailQueueWorker, workspace *domain.Workspace, entry *domain.EmailQueueEntry) *emailerror.ClassifiedError {
	return w.processEntry(workspace, workspace.GetIntegrationByID(entry.IntegrationID), entry, nil)
}

// quietLogger accepts every log call.
func quietLogger(ctrl *gomock.Controller) *pkgmocks.MockLogger {
	log := pkgmocks.NewMockLogger(ctrl)
	log.EXPECT().WithFields(gomock.Any()).Return(log).AnyTimes()
	log.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(log).AnyTimes()
	log.EXPECT().Debug(gomock.Any()).AnyTimes()
	log.EXPECT().Info(gomock.Any()).AnyTimes()
	log.EXPECT().Warn(gomock.Any()).AnyTimes()
	log.EXPECT().Error(gomock.Any()).AnyTimes()
	return log
}

func emailIntegration(id string, kind domain.EmailProviderKind, ratePerMinute int) domain.Integration {
	return domain.Integration{
		ID:            id,
		Type:          domain.IntegrationTypeEmail,
		EmailProvider: domain.EmailProvider{Kind: kind, RateLimitPerMinute: ratePerMinute},
	}
}

// queuedRows returns n pending broadcast rows of one integration, oldest first.
func queuedRows(integrationID string, n int) []*domain.EmailQueueEntry {
	rows := make([]*domain.EmailQueueEntry, n)
	for i := range rows {
		id := fmt.Sprintf("%s-row-%03d", integrationID, i)
		rows[i] = &domain.EmailQueueEntry{
			ID:            id,
			Status:        domain.EmailQueueStatusPending,
			SourceType:    domain.EmailQueueSourceBroadcast,
			SourceID:      "bc-1",
			IntegrationID: integrationID,
			ContactEmail:  id + "@example.com",
			MessageID:     "msg-" + id,
			MaxAttempts:   3,
		}
	}
	return rows
}

// throttled is what SES answers above the account's sending rate: a provider error.
var throttled = errors.New("ThrottlingException: Maximum sending rate exceeded")

// accessDenied is a provider error SES gives as permanent: the account must be fixed.
var accessDenied = errors.New("AccessDeniedException: User is not authorized to perform ses:SendEmail")

// fakeQueue keeps queue rows in memory behind the repository mock. Claims are
// exclusive, as MarkAsProcessing's conditional update makes them, and every call is
// counted per row.
type fakeQueue struct {
	mu       sync.Mutex
	rows     []*domain.EmailQueueEntry
	fetches  map[string][]int // integration ID -> the limit of each fetch
	claims   map[string]int
	sent     map[string]int
	failed   map[string]int
	given    map[string]int // MarkAsPermanentlyFailed calls, also counted in failed
	ctxErrs  []error        // the context error of every outcome written
	takenBy  map[string]bool
	onFailed func(id string)
}

func newFakeQueue(rows ...*domain.EmailQueueEntry) *fakeQueue {
	return &fakeQueue{
		rows:    rows,
		fetches: map[string][]int{},
		claims:  map[string]int{},
		sent:    map[string]int{},
		failed:  map[string]int{},
		given:   map[string]int{},
		takenBy: map[string]bool{},
	}
}

func (q *fakeQueue) wire(m *mocks.MockEmailQueueRepository) {
	m.EXPECT().FetchPendingForIntegration(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, integrationID string, limit int) ([]*domain.EmailQueueEntry, error) {
			q.mu.Lock()
			defer q.mu.Unlock()
			q.fetches[integrationID] = append(q.fetches[integrationID], limit)
			var due []*domain.EmailQueueEntry
			for _, row := range q.rows {
				if len(due) == limit {
					break
				}
				if row.IntegrationID == integrationID && row.Status == domain.EmailQueueStatusPending {
					copied := *row
					due = append(due, &copied)
				}
			}
			return due, nil
		}).AnyTimes()
	m.EXPECT().FetchPendingOrphans(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	m.EXPECT().MarkAsProcessing(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, id string) error {
			q.mu.Lock()
			defer q.mu.Unlock()
			row := q.find(id)
			if row == nil || row.Status != domain.EmailQueueStatusPending || q.takenBy[id] {
				return fmt.Errorf("entry %s is not claimable", id)
			}
			row.Status = domain.EmailQueueStatusProcessing
			row.Attempts++
			q.claims[id]++
			return nil
		}).AnyTimes()
	m.EXPECT().MarkAsSent(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, _ string, id string) error {
			q.mu.Lock()
			defer q.mu.Unlock()
			q.sent[id]++
			q.ctxErrs = append(q.ctxErrs, ctx.Err())
			for i, row := range q.rows {
				if row.ID == id {
					q.rows = append(q.rows[:i], q.rows[i+1:]...)
					break
				}
			}
			return nil
		}).AnyTimes()
	m.EXPECT().MarkAsFailed(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, _ string, id string, _ string, _ *time.Time) error {
			return q.fail(ctx, id)
		}).AnyTimes()
	m.EXPECT().MarkAsPermanentlyFailed(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, _ string, id string, _ string) error {
			q.mu.Lock()
			q.given[id]++
			q.mu.Unlock()
			return q.fail(ctx, id)
		}).AnyTimes()
}

// find returns the row with that ID. Callers hold q.mu.
func (q *fakeQueue) find(id string) *domain.EmailQueueEntry {
	for _, row := range q.rows {
		if row.ID == id {
			return row
		}
	}
	return nil
}

func (q *fakeQueue) fail(ctx context.Context, id string) error {
	if q.onFailed != nil {
		q.onFailed(id)
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if row := q.find(id); row != nil {
		row.Status = domain.EmailQueueStatusFailed
	}
	q.failed[id]++
	q.ctxErrs = append(q.ctxErrs, ctx.Err())
	return nil
}

func (q *fakeQueue) fetchLimits(integrationID string) []int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return append([]int(nil), q.fetches[integrationID]...)
}

// count sums a counter over the rows of one integration ("" for all).
func (q *fakeQueue) count(counter func(*fakeQueue) map[string]int, integrationID string) int {
	q.mu.Lock()
	defer q.mu.Unlock()
	total := 0
	for id, n := range counter(q) {
		if integrationID == "" || strings.HasPrefix(id, integrationID+"-row-") {
			total += n
		}
	}
	return total
}

func (q *fakeQueue) claimCount(integrationID string) int {
	return q.count(func(q *fakeQueue) map[string]int { return q.claims }, integrationID)
}

func (q *fakeQueue) sentCount(integrationID string) int {
	return q.count(func(q *fakeQueue) map[string]int { return q.sent }, integrationID)
}

func (q *fakeQueue) failedCount(integrationID string) int {
	return q.count(func(q *fakeQueue) map[string]int { return q.failed }, integrationID)
}

func (q *fakeQueue) perRow(counter func(*fakeQueue) map[string]int, id string) int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return counter(q)[id]
}

// pending counts the rows still waiting to be sent.
func (q *fakeQueue) pending() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	n := 0
	for _, row := range q.rows {
		if row.Status == domain.EmailQueueStatusPending {
			n++
		}
	}
	return n
}

func (q *fakeQueue) writeCtxErrs() []error {
	q.mu.Lock()
	defer q.mu.Unlock()
	return append([]error(nil), q.ctxErrs...)
}

type laneTest struct {
	w             *EmailQueueWorker
	queue         *fakeQueue
	workspaceRepo *mocks.MockWorkspaceRepository
	email         *mocks.MockEmailServiceInterface
	automation    *mocks.MockAutomationRepository

	historyMu      sync.Mutex
	historyCtxErrs []error
}

// newLaneTest builds a worker over a fake queue, with the contexts Start sets but no
// loop running, so that a test can drive rounds itself or call Start.
func newLaneTest(t *testing.T, cfg *EmailQueueWorkerConfig, rows ...*domain.EmailQueueEntry) *laneTest {
	t.Helper()
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	lt := &laneTest{
		queue:         newFakeQueue(rows...),
		workspaceRepo: mocks.NewMockWorkspaceRepository(ctrl),
		email:         mocks.NewMockEmailServiceInterface(ctrl),
		automation:    mocks.NewMockAutomationRepository(ctrl),
	}
	queueRepo := mocks.NewMockEmailQueueRepository(ctrl)
	lt.queue.wire(queueRepo)
	history := mocks.NewMockMessageHistoryRepository(ctrl)
	history.EXPECT().Upsert(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, _, _ string, _ *domain.MessageHistory) error {
			lt.historyMu.Lock()
			defer lt.historyMu.Unlock()
			lt.historyCtxErrs = append(lt.historyCtxErrs, ctx.Err())
			return nil
		}).AnyTimes()

	lt.w = NewEmailQueueWorker(queueRepo, lt.workspaceRepo, lt.email, history, cfg, quietLogger(ctrl))
	lt.w.initContexts(context.Background())
	return lt
}

func (lt *laneTest) historyErrs() []error {
	lt.historyMu.Lock()
	defer lt.historyMu.Unlock()
	return append([]error(nil), lt.historyCtxErrs...)
}

// within runs fn, fails the test if it takes longer than limit, and returns how long it took.
func within(t *testing.T, limit time.Duration, fn func()) time.Duration {
	t.Helper()
	start := time.Now()
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()
	select {
	case <-done:
	case <-time.After(limit):
		t.Fatalf("still running after %s", limit)
	}
	return time.Since(start)
}

// openCircuit records provider failures until the integration's circuit opens.
func openCircuit(t *testing.T, w *EmailQueueWorker, integrationID string) {
	t.Helper()
	for i := 0; i < 100 && !w.circuitBreaker.IsOpen(integrationID); i++ {
		ticket, ok := w.circuitBreaker.Allow(integrationID)
		require.True(t, ok)
		w.circuitBreaker.RecordFailure(integrationID, ticket, providerErr)
	}
	require.True(t, w.circuitBreaker.IsOpen(integrationID))
}

// inFlight tracks the sends in progress per integration, and the most seen at once.
type inFlight struct {
	mu   sync.Mutex
	now  map[string]int
	peak map[string]int
}

func newInFlight() *inFlight {
	return &inFlight{now: map[string]int{}, peak: map[string]int{}}
}

func (f *inFlight) enter(integrationID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now[integrationID]++
	if f.now[integrationID] > f.peak[integrationID] {
		f.peak[integrationID] = f.now[integrationID]
	}
}

func (f *inFlight) leave(integrationID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now[integrationID]--
}

func (f *inFlight) current(integrationID string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now[integrationID]
}

func (f *inFlight) max(integrationID string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.peak[integrationID]
}

// recorder keeps the order in which things happened across goroutines.
type recorder struct {
	mu     sync.Mutex
	events []string
}

func (r *recorder) add(event string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
}

func (r *recorder) list() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.events...)
}

func TestLaneBatchSize(t *testing.T) {
	cases := []struct {
		name string
		rate int
		want int
	}{
		{"3,600/min fills two seconds", 3600, 120},
		{"25/min still fetches one row", 25, 1},
		{"6,000/min reaches the cap", 6000, 200},
		{"above 6,000/min stays at the cap", 50000, 200},
		{"rounds up", 61, 3},
		{"unset means 60/min", 0, 2},
		{"negative means 60/min", -5, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, laneBatchSize(tc.rate, 2*time.Second, 200))
		})
	}

	t.Run("follows the round window", func(t *testing.T) {
		assert.Equal(t, 30, laneBatchSize(3600, 500*time.Millisecond, 200))
	})
}

func TestEmailQueueWorker_Lane(t *testing.T) {
	t.Run("each row is claimed and sent exactly once across rounds", func(t *testing.T) {
		cfg := DefaultWorkerConfig()
		cfg.PollInterval = 10 * time.Millisecond
		cfg.RoundWindow = 200 * time.Millisecond
		rows := queuedRows("ses-1", 50)
		lt := newLaneTest(t, cfg, rows...)
		workspace := &domain.Workspace{ID: "ws-1", Integrations: []domain.Integration{
			emailIntegration("ses-1", domain.EmailProviderKindSES, 6000),
		}}
		lt.workspaceRepo.EXPECT().List(gomock.Any()).Return([]*domain.Workspace{workspace}, nil).AnyTimes()

		var sendsMu sync.Mutex
		sends := map[string]int{}
		lt.email.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).
			DoAndReturn(func(_ context.Context, req domain.SendEmailProviderRequest, _ bool) error {
				time.Sleep(5 * time.Millisecond)
				sendsMu.Lock()
				defer sendsMu.Unlock()
				sends[req.MessageID]++
				return nil
			}).AnyTimes()

		require.NoError(t, lt.w.Start(context.Background()))
		require.Eventually(t, func() bool { return lt.queue.sentCount("") == len(rows) }, 10*time.Second, 10*time.Millisecond)
		lt.w.Stop()

		sendsMu.Lock()
		defer sendsMu.Unlock()
		for _, row := range rows {
			assert.Equal(t, 1, lt.queue.perRow(func(q *fakeQueue) map[string]int { return q.claims }, row.ID), row.ID)
			assert.Equal(t, 1, lt.queue.perRow(func(q *fakeQueue) map[string]int { return q.sent }, row.ID), row.ID)
			assert.Equal(t, 1, sends[row.MessageID], row.ID)
		}
		// 6,000/min over a 200 ms round is 20 rows per fetch, so the queue took
		// several rounds.
		limits := lt.queue.fetchLimits("ses-1")
		assert.GreaterOrEqual(t, len(limits), 3)
		for _, limit := range limits {
			assert.Equal(t, 20, limit)
		}
	})

	t.Run("sends at most the integration's concurrency at once, and reaches it", func(t *testing.T) {
		cfg := DefaultWorkerConfig()
		cfg.SendConcurrency = 4
		lt := newLaneTest(t, cfg, append(queuedRows("ses-1", 8), queuedRows("smtp-1", 8)...)...)
		workspace := &domain.Workspace{ID: "ws-1", Integrations: []domain.Integration{
			emailIntegration("ses-1", domain.EmailProviderKindSES, 6000),
			emailIntegration("smtp-1", domain.EmailProviderKindSMTP, 6000),
		}}

		flight := newInFlight()
		release := make(chan struct{})
		lt.email.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).
			DoAndReturn(func(_ context.Context, req domain.SendEmailProviderRequest, _ bool) error {
				flight.enter(req.IntegrationID)
				defer flight.leave(req.IntegrationID)
				<-release
				return nil
			}).AnyTimes()

		done := make(chan struct{})
		go func() {
			defer close(done)
			lt.w.processWorkspace(workspace)
		}()

		require.Eventually(t, func() bool {
			return flight.current("ses-1") == 4 && flight.current("smtp-1") == 1
		}, 2*time.Second, 5*time.Millisecond)
		// Long enough for a fifth send to start if the bound leaked.
		time.Sleep(100 * time.Millisecond)
		assert.Equal(t, 4, flight.current("ses-1"))
		assert.Equal(t, 1, flight.current("smtp-1"))

		close(release)
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("round did not end")
		}
		assert.Equal(t, 4, flight.max("ses-1"))
		assert.Equal(t, 1, flight.max("smtp-1"))
		assert.Equal(t, 8, lt.queue.sentCount("ses-1"))
		assert.Equal(t, 8, lt.queue.sentCount("smtp-1"))
	})

	t.Run("no second fetch while a send of the round is in flight", func(t *testing.T) {
		cfg := DefaultWorkerConfig()
		cfg.PollInterval = 10 * time.Millisecond
		lt := newLaneTest(t, cfg, queuedRows("ses-1", 1)...)
		workspace := &domain.Workspace{ID: "ws-1", Integrations: []domain.Integration{
			emailIntegration("ses-1", domain.EmailProviderKindSES, 6000),
		}}
		lt.workspaceRepo.EXPECT().List(gomock.Any()).Return([]*domain.Workspace{workspace}, nil).AnyTimes()

		started := make(chan struct{})
		release := make(chan struct{})
		lt.email.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).
			DoAndReturn(func(context.Context, domain.SendEmailProviderRequest, bool) error {
				close(started)
				<-release
				return nil
			})

		require.NoError(t, lt.w.Start(context.Background()))
		<-started
		time.Sleep(200 * time.Millisecond) // many poll intervals
		assert.Len(t, lt.queue.fetchLimits("ses-1"), 1)
		close(release)
		require.Eventually(t, func() bool { return lt.queue.sentCount("") == 1 }, 2*time.Second, 5*time.Millisecond)
		lt.w.Stop()
	})

	t.Run("claims only the rows whose token falls within the round", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		queueRepo := mocks.NewMockEmailQueueRepository(ctrl)
		emailSvc := mocks.NewMockEmailServiceInterface(ctrl)
		history := mocks.NewMockMessageHistoryRepository(ctrl)
		history.EXPECT().Upsert(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

		// 50/min: tokens 1.2 s apart, so two fall within a 2 s round. The fetch returns
		// more rows than asked for, to show the rate, not the fetch, bounds the claims.
		rows := queuedRows("smtp-1", 5)
		queueRepo.EXPECT().FetchPendingForIntegration(gomock.Any(), "ws-1", "smtp-1", 2).Return(rows, nil)
		queueRepo.EXPECT().FetchPendingOrphans(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil)
		for _, row := range rows[:2] {
			queueRepo.EXPECT().MarkAsProcessing(gomock.Any(), "ws-1", row.ID).Return(nil)
			queueRepo.EXPECT().MarkAsSent(gomock.Any(), "ws-1", row.ID).Return(nil)
		}
		// No MarkAsProcessing is expected for the other rows: gomock fails on any.
		emailSvc.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).Return(nil).Times(2)

		w := NewEmailQueueWorker(queueRepo, mocks.NewMockWorkspaceRepository(ctrl), emailSvc, history,
			DefaultWorkerConfig(), quietLogger(ctrl))
		w.initContexts(context.Background())
		within(t, 5*time.Second, func() {
			w.processWorkspace(&domain.Workspace{ID: "ws-1", Integrations: []domain.Integration{
				emailIntegration("smtp-1", domain.EmailProviderKindSMTP, 50),
			}})
		})
	})

	t.Run("a lane whose next token falls after the round does not fetch", func(t *testing.T) {
		lt := newLaneTest(t, DefaultWorkerConfig(), queuedRows("ses-1", 3)...)
		lt.email.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).Return(nil).AnyTimes()
		// 10/min: tokens further apart than the round window.
		workspace := &domain.Workspace{ID: "ws-1", Integrations: []domain.Integration{
			emailIntegration("ses-1", domain.EmailProviderKindSES, 10),
		}}

		within(t, 5*time.Second, func() { lt.w.processWorkspace(workspace) })
		within(t, 5*time.Second, func() { lt.w.processWorkspace(workspace) })

		assert.Equal(t, []int{1}, lt.queue.fetchLimits("ses-1"), "only the round with a token fetched")
		assert.Equal(t, 1, lt.queue.sentCount("ses-1"))
	})

	t.Run("stops dispatching at the deadline when sends are slower than the rate", func(t *testing.T) {
		cfg := DefaultWorkerConfig()
		cfg.RoundWindow = 500 * time.Millisecond
		lt := newLaneTest(t, cfg, queuedRows("smtp-1", 20)...)
		const sendTime = 100 * time.Millisecond
		lt.email.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).
			DoAndReturn(func(context.Context, domain.SendEmailProviderRequest, bool) error {
				time.Sleep(sendTime)
				return nil
			}).AnyTimes()

		// 2,400/min allows 20 rows in a 500 ms round, but one SMTP connection at
		// 100 ms per email can only send about five.
		took := within(t, 5*time.Second, func() {
			lt.w.processWorkspace(&domain.Workspace{ID: "ws-1", Integrations: []domain.Integration{
				emailIntegration("smtp-1", domain.EmailProviderKindSMTP, 2400),
			}})
		})

		assert.Equal(t, []int{20}, lt.queue.fetchLimits("smtp-1"))
		claimed := lt.queue.claimCount("smtp-1")
		assert.GreaterOrEqual(t, claimed, 3)
		assert.LessOrEqual(t, claimed, 6)
		assert.Equal(t, 20-claimed, lt.queue.pending(), "the rows not dispatched are left untouched")
		assert.Less(t, took, cfg.RoundWindow+sendTime+200*time.Millisecond)
	})

	t.Run("a fetch slower than the round window still sends what it fetched", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		queueRepo := mocks.NewMockEmailQueueRepository(ctrl)
		emailSvc := mocks.NewMockEmailServiceInterface(ctrl)
		history := mocks.NewMockMessageHistoryRepository(ctrl)
		history.EXPECT().Upsert(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

		rows := queuedRows("ses-1", 3)
		queueRepo.EXPECT().FetchPendingForIntegration(gomock.Any(), "ws-1", "ses-1", gomock.Any()).
			DoAndReturn(func(context.Context, string, string, int) ([]*domain.EmailQueueEntry, error) {
				time.Sleep(300 * time.Millisecond)
				return rows, nil
			})
		queueRepo.EXPECT().FetchPendingOrphans(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil)
		for _, row := range rows {
			queueRepo.EXPECT().MarkAsProcessing(gomock.Any(), "ws-1", row.ID).Return(nil)
			queueRepo.EXPECT().MarkAsSent(gomock.Any(), "ws-1", row.ID).Return(nil)
		}
		emailSvc.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).Return(nil).Times(len(rows))

		cfg := DefaultWorkerConfig()
		cfg.RoundWindow = 200 * time.Millisecond
		w := NewEmailQueueWorker(queueRepo, mocks.NewMockWorkspaceRepository(ctrl), emailSvc, history, cfg, quietLogger(ctrl))
		w.initContexts(context.Background())
		within(t, 5*time.Second, func() {
			w.processWorkspace(&domain.Workspace{ID: "ws-1", Integrations: []domain.Integration{
				emailIntegration("ses-1", domain.EmailProviderKindSES, 6000),
			}})
		})
	})

	t.Run("an integration without a rate limit sends at 60 per minute", func(t *testing.T) {
		lt := newLaneTest(t, DefaultWorkerConfig(), queuedRows("smtp-1", 1)...)
		lt.email.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).Return(nil)

		within(t, 5*time.Second, func() {
			lt.w.processWorkspace(&domain.Workspace{ID: "ws-1", Integrations: []domain.Integration{
				emailIntegration("smtp-1", domain.EmailProviderKindSMTP, 0),
			}})
		})

		assert.Equal(t, []int{2}, lt.queue.fetchLimits("smtp-1"))
		assert.InDelta(t, 1.0, lt.w.rateLimiter.GetCurrentRate("smtp-1"), 0.001)
		assert.Equal(t, 1, lt.queue.sentCount("smtp-1"))
	})

	t.Run("sends concurrently", func(t *testing.T) {
		cfg := DefaultWorkerConfig()
		cfg.PollInterval = 10 * time.Millisecond
		rows := queuedRows("ses-1", 100)
		lt := newLaneTest(t, cfg, rows...)
		workspace := &domain.Workspace{ID: "ws-1", Integrations: []domain.Integration{
			emailIntegration("ses-1", domain.EmailProviderKindSES, 6000),
		}}
		lt.workspaceRepo.EXPECT().List(gomock.Any()).Return([]*domain.Workspace{workspace}, nil).AnyTimes()
		lt.email.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).
			DoAndReturn(func(context.Context, domain.SendEmailProviderRequest, bool) error {
				time.Sleep(50 * time.Millisecond)
				return nil
			}).AnyTimes()

		start := time.Now()
		require.NoError(t, lt.w.Start(context.Background()))
		require.Eventually(t, func() bool { return lt.queue.sentCount("") == len(rows) }, 10*time.Second, 10*time.Millisecond)
		took := time.Since(start)
		lt.w.Stop()

		// One at a time, these sends add up to well over this bound.
		assert.Less(t, took, 3*time.Second)
	})
}

func TestEmailQueueWorker_LaneCircuit(t *testing.T) {
	sesWorkspace := func(integrationIDs ...string) *domain.Workspace {
		ws := &domain.Workspace{ID: "ws-1"}
		for _, id := range integrationIDs {
			ws.Integrations = append(ws.Integrations, emailIntegration(id, domain.EmailProviderKindSES, 6000))
		}
		return ws
	}

	t.Run("an open circuit leaves its rows alone while another integration sends", func(t *testing.T) {
		lt := newLaneTest(t, DefaultWorkerConfig(), append(queuedRows("ses-1", 3), queuedRows("ses-2", 3)...)...)
		openCircuit(t, lt.w, "ses-1")
		lt.email.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).
			DoAndReturn(func(_ context.Context, req domain.SendEmailProviderRequest, _ bool) error {
				assert.Equal(t, "ses-2", req.IntegrationID)
				return nil
			}).AnyTimes()

		within(t, 5*time.Second, func() { lt.w.processWorkspace(sesWorkspace("ses-1", "ses-2")) })

		assert.Empty(t, lt.queue.fetchLimits("ses-1"), "no fetch while the circuit is open")
		assert.Zero(t, lt.queue.claimCount("ses-1"))
		assert.Zero(t, lt.queue.failedCount("ses-1"))
		assert.Equal(t, 3, lt.queue.sentCount("ses-2"))
	})

	t.Run("after the cooldown one row is sent as the probe, then the lane resumes", func(t *testing.T) {
		cfg := DefaultWorkerConfig()
		cfg.CircuitBreakerCooldown = 50 * time.Millisecond
		lt := newLaneTest(t, cfg, queuedRows("ses-1", 3)...)
		openCircuit(t, lt.w, "ses-1")
		time.Sleep(60 * time.Millisecond)
		lt.email.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).Return(nil).AnyTimes()

		within(t, 5*time.Second, func() { lt.w.processWorkspace(sesWorkspace("ses-1")) })
		assert.Equal(t, []int{1}, lt.queue.fetchLimits("ses-1"))
		assert.Equal(t, 1, lt.queue.sentCount("ses-1"))
		assert.False(t, lt.w.circuitBreaker.IsOpen("ses-1"), "the probe's success closes the circuit")

		within(t, 5*time.Second, func() { lt.w.processWorkspace(sesWorkspace("ses-1")) })
		assert.Equal(t, []int{1, 200}, lt.queue.fetchLimits("ses-1"))
		assert.Equal(t, 3, lt.queue.sentCount("ses-1"))
	})

	t.Run("a probe that is not sent is given back", func(t *testing.T) {
		caID := "ca-1"
		automationRow := func() *domain.EmailQueueEntry {
			row := queuedRows("ses-1", 1)[0]
			row.SourceType = domain.EmailQueueSourceAutomation
			row.Payload.ContactAutomationID = &caID
			return row
		}

		cases := []struct {
			name  string
			rows  []*domain.EmailQueueEntry
			setup func(t *testing.T, lt *laneTest)
		}{
			{
				name: "the fetch finds nothing",
			},
			{
				name: "another instance claimed the row",
				rows: queuedRows("ses-1", 1),
				setup: func(_ *testing.T, lt *laneTest) {
					lt.queue.takenBy["ses-1-row-000"] = true
				},
			},
			{
				name: "the stop-on-reply guard removes the row",
				rows: []*domain.EmailQueueEntry{automationRow()},
				setup: func(_ *testing.T, lt *laneTest) {
					lt.w.SetAutomationRepo(lt.automation)
					lt.automation.EXPECT().GetContactAutomation(gomock.Any(), "ws-1", caID).
						Return(&domain.ContactAutomation{ID: caID, Status: domain.ContactAutomationStatusExited}, nil)
				},
			},
			{
				name: "a stop ends the wait for its token",
				rows: queuedRows("ses-1", 1),
				setup: func(t *testing.T, lt *laneTest) {
					// Spend the banked token so that the probe's is a second away.
					_, _, ok := lt.w.rateLimiter.ReserveBefore("ses-1", 60, time.Now().Add(time.Minute))
					require.True(t, ok)
					go func() {
						time.Sleep(100 * time.Millisecond)
						lt.w.cancel()
					}()
				},
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				cfg := DefaultWorkerConfig()
				cfg.CircuitBreakerCooldown = 50 * time.Millisecond
				lt := newLaneTest(t, cfg, tc.rows...)
				if tc.setup != nil {
					tc.setup(t, lt)
				}
				openCircuit(t, lt.w, "ses-1")
				time.Sleep(60 * time.Millisecond)

				workspace := &domain.Workspace{ID: "ws-1", Integrations: []domain.Integration{
					emailIntegration("ses-1", domain.EmailProviderKindSES, 60),
				}}
				within(t, 5*time.Second, func() { lt.w.processWorkspace(workspace) })

				assert.Equal(t, []int{1}, lt.queue.fetchLimits("ses-1"))
				assert.True(t, lt.w.circuitBreaker.IsOpen("ses-1"), "nothing reached the provider")
				assert.False(t, lt.w.GetCircuitBreakerStats()["ses-1"].ProbeInFlight)
				ticket, ok := lt.w.circuitBreaker.Allow("ses-1")
				assert.True(t, ok, "the next round probes without another cooldown")
				assert.True(t, ticket.Probe)
			})
		}
	})

	t.Run("failures that open the circuit end the round, and rows waiting for their token are not claimed", func(t *testing.T) {
		cfg := DefaultWorkerConfig()
		cfg.CircuitBreakerThreshold = 3
		lt := newLaneTest(t, cfg, queuedRows("ses-1", 30)...)

		var sends atomic.Int32
		lt.email.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).
			DoAndReturn(func(context.Context, domain.SendEmailProviderRequest, bool) error {
				sends.Add(1)
				return throttled
			}).AnyTimes()
		var opened atomic.Int32
		lt.w.SetCircuitOpenCallback(func(string, string, *emailerror.ClassifiedError) { opened.Add(1) })

		// 3,000/min: one token every 20 ms, so the rows dispatched after the first
		// three are still waiting for theirs when the third failure opens the circuit.
		workspace := &domain.Workspace{ID: "ws-1", Integrations: []domain.Integration{
			emailIntegration("ses-1", domain.EmailProviderKindSES, 3000),
		}}
		within(t, 5*time.Second, func() { lt.w.processWorkspace(workspace) })

		assert.Equal(t, int32(3), sends.Load())
		assert.Equal(t, 3, lt.queue.claimCount("ses-1"))
		assert.Equal(t, 27, lt.queue.pending())
		assert.Equal(t, int32(1), opened.Load())
	})

	t.Run("the pause comes after the outcome of every send that was in flight", func(t *testing.T) {
		cfg := DefaultWorkerConfig()
		cfg.CircuitBreakerThreshold = 3
		cfg.SendConcurrency = 4
		rows := queuedRows("ses-1", 10)
		lt := newLaneTest(t, cfg, rows...)

		// The four sends in flight fail 10 ms apart, so the third opens the circuit,
		// and the fourth is the slowest to record its outcome. A pause called by the
		// send that opened the circuit would come before that outcome. Every outcome
		// takes long enough that no slot frees up before the circuit opens.
		order := map[string]int{}
		for i, row := range rows {
			order[row.MessageID] = i
		}
		events := &recorder{}
		lt.queue.onFailed = func(id string) {
			time.Sleep(40 * time.Millisecond)
			if id == rows[3].ID {
				time.Sleep(60 * time.Millisecond)
			}
			events.add("failed " + id)
		}
		lt.w.SetCircuitOpenCallback(func(string, string, *emailerror.ClassifiedError) { events.add("pause") })

		flight := newInFlight()
		lt.email.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).
			DoAndReturn(func(_ context.Context, req domain.SendEmailProviderRequest, _ bool) error {
				flight.enter(req.IntegrationID)
				for flight.max(req.IntegrationID) < 4 {
					time.Sleep(time.Millisecond)
				}
				time.Sleep(time.Duration(order[req.MessageID]) * 10 * time.Millisecond)
				return throttled
			}).AnyTimes()

		within(t, 5*time.Second, func() { lt.w.processWorkspace(sesWorkspace("ses-1")) })

		got := events.list()
		require.Len(t, got, 5)
		for _, event := range got[:4] {
			assert.True(t, strings.HasPrefix(event, "failed "), event)
		}
		assert.Equal(t, "pause", got[4])
		assert.Equal(t, 4, lt.queue.claimCount("ses-1"), "nothing is dispatched after the opening")
	})

	t.Run("a send allowed before the opening that succeeds after it leaves the circuit open", func(t *testing.T) {
		cfg := DefaultWorkerConfig()
		cfg.CircuitBreakerThreshold = 3
		cfg.SendConcurrency = 4
		rows := queuedRows("ses-1", 4)
		lt := newLaneTest(t, cfg, rows...)
		var opened atomic.Int32
		lt.w.SetCircuitOpenCallback(func(string, string, *emailerror.ClassifiedError) { opened.Add(1) })

		flight := newInFlight()
		lt.email.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).
			DoAndReturn(func(_ context.Context, req domain.SendEmailProviderRequest, _ bool) error {
				flight.enter(req.IntegrationID)
				for flight.max(req.IntegrationID) < 4 {
					time.Sleep(time.Millisecond)
				}
				if req.MessageID != rows[3].MessageID {
					return throttled
				}
				for !lt.w.circuitBreaker.IsOpen("ses-1") {
					time.Sleep(time.Millisecond)
				}
				return nil
			}).AnyTimes()

		within(t, 5*time.Second, func() { lt.w.processWorkspace(sesWorkspace("ses-1")) })

		assert.True(t, lt.w.circuitBreaker.IsOpen("ses-1"))
		assert.Equal(t, int32(1), opened.Load())
		assert.Equal(t, 1, lt.queue.sentCount("ses-1"))
		assert.Equal(t, 3, lt.queue.failedCount("ses-1"))
	})

	t.Run("a send in flight that fails after the opening is kept for a retry, even when the provider calls it permanent", func(t *testing.T) {
		cfg := DefaultWorkerConfig()
		cfg.CircuitBreakerThreshold = 3
		cfg.SendConcurrency = 4
		rows := queuedRows("ses-1", 4)
		lt := newLaneTest(t, cfg, rows...)

		// All four are refused for the same permanent reason. The first three open the
		// circuit and are given up, as before; the fourth is refused after the opening,
		// so the pause must be able to hold it: it is scheduled for a retry.
		flight := newInFlight()
		lt.email.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).
			DoAndReturn(func(_ context.Context, req domain.SendEmailProviderRequest, _ bool) error {
				flight.enter(req.IntegrationID)
				for flight.max(req.IntegrationID) < 4 {
					time.Sleep(time.Millisecond)
				}
				if req.MessageID == rows[3].MessageID {
					for !lt.w.circuitBreaker.IsOpen("ses-1") {
						time.Sleep(time.Millisecond)
					}
				}
				return accessDenied
			}).AnyTimes()

		within(t, 5*time.Second, func() { lt.w.processWorkspace(sesWorkspace("ses-1")) })

		require.True(t, lt.w.circuitBreaker.IsOpen("ses-1"))
		assert.Equal(t, 4, lt.queue.failedCount("ses-1"))
		for _, row := range rows[:3] {
			assert.Equal(t, 1, lt.queue.perRow(func(q *fakeQueue) map[string]int { return q.given }, row.ID), row.ID)
		}
		assert.Zero(t, lt.queue.perRow(func(q *fakeQueue) map[string]int { return q.given }, rows[3].ID),
			"refused after the opening, it is not given up")
	})
}

func TestEmailQueueWorker_Stop(t *testing.T) {
	smtpWorkspace := func(id, integrationID string) *domain.Workspace {
		return &domain.Workspace{ID: id, Integrations: []domain.Integration{
			emailIntegration(integrationID, domain.EmailProviderKindSMTP, 6000),
		}}
	}
	waitForStop := func(w *EmailQueueWorker) {
		for w.IsRunning() {
			time.Sleep(time.Millisecond)
		}
	}

	t.Run("a send in flight finishes and is recorded, and no other row is claimed", func(t *testing.T) {
		cfg := DefaultWorkerConfig()
		cfg.PollInterval = 10 * time.Millisecond
		lt := newLaneTest(t, cfg, queuedRows("smtp-1", 3)...)
		lt.workspaceRepo.EXPECT().List(gomock.Any()).Return([]*domain.Workspace{smtpWorkspace("ws-1", "smtp-1")}, nil).AnyTimes()

		started := make(chan struct{})
		lt.email.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).
			DoAndReturn(func(ctx context.Context, _ domain.SendEmailProviderRequest, _ bool) error {
				close(started)
				waitForStop(lt.w)
				time.Sleep(50 * time.Millisecond)
				assert.NoError(t, ctx.Err(), "the stop does not cancel a send")
				return nil
			})

		require.NoError(t, lt.w.Start(context.Background()))
		<-started
		within(t, 5*time.Second, lt.w.Stop)

		assert.Equal(t, 1, lt.queue.claimCount("smtp-1"))
		assert.Equal(t, 1, lt.queue.sentCount("smtp-1"))
		assert.Equal(t, 2, lt.queue.pending())
		for _, err := range lt.queue.writeCtxErrs() {
			assert.NoError(t, err)
		}
		for _, err := range lt.historyErrs() {
			assert.NoError(t, err)
		}
	})

	t.Run("waits for the workspaces in progress and starts no other", func(t *testing.T) {
		cfg := DefaultWorkerConfig()
		cfg.PollInterval = 10 * time.Millisecond
		cfg.WorkerCount = 2
		var rows []*domain.EmailQueueEntry
		for _, id := range []string{"smtp-1", "smtp-2", "smtp-3", "smtp-4"} {
			rows = append(rows, queuedRows(id, 1)...)
		}
		lt := newLaneTest(t, cfg, rows...)
		lt.workspaceRepo.EXPECT().List(gomock.Any()).Return([]*domain.Workspace{
			smtpWorkspace("ws-1", "smtp-1"),
			smtpWorkspace("ws-2", "smtp-2"),
			smtpWorkspace("ws-3", "smtp-3"),
			smtpWorkspace("ws-4", "smtp-4"),
		}, nil).AnyTimes()

		var started sync.WaitGroup
		started.Add(2)
		lt.email.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).
			DoAndReturn(func(_ context.Context, req domain.SendEmailProviderRequest, _ bool) error {
				started.Done()
				waitForStop(lt.w)
				if req.IntegrationID == "smtp-1" {
					// Still sending well after the other workspace has finished.
					time.Sleep(200 * time.Millisecond)
				}
				return nil
			}).Times(2)

		require.NoError(t, lt.w.Start(context.Background()))
		started.Wait()
		within(t, 5*time.Second, lt.w.Stop)

		assert.Equal(t, 1, lt.queue.sentCount("smtp-1"), "Stop returned before the workspace's send was recorded")
		assert.Equal(t, 1, lt.queue.sentCount("smtp-2"))
		assert.Empty(t, lt.queue.fetchLimits("smtp-3"))
		assert.Empty(t, lt.queue.fetchLimits("smtp-4"))
	})

	t.Run("a wait for a token ended by Stop claims nothing", func(t *testing.T) {
		cfg := DefaultWorkerConfig()
		cfg.PollInterval = 10 * time.Millisecond
		lt := newLaneTest(t, cfg, queuedRows("ses-1", 2)...)
		lt.workspaceRepo.EXPECT().List(gomock.Any()).Return([]*domain.Workspace{{ID: "ws-1", Integrations: []domain.Integration{
			emailIntegration("ses-1", domain.EmailProviderKindSES, 60),
		}}}, nil).AnyTimes()
		lt.email.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).Return(nil)

		require.NoError(t, lt.w.Start(context.Background()))
		require.Eventually(t, func() bool { return lt.queue.sentCount("") == 1 }, 2*time.Second, 5*time.Millisecond)
		// The second row's token is a second away.
		took := within(t, 5*time.Second, lt.w.Stop)

		assert.Less(t, took, 500*time.Millisecond)
		assert.Equal(t, 1, lt.queue.claimCount("ses-1"))
		assert.Equal(t, 1, lt.queue.pending())
	})

	t.Run("past the grace, sends are cancelled without counting against the provider", func(t *testing.T) {
		cfg := DefaultWorkerConfig()
		cfg.PollInterval = 10 * time.Millisecond
		cfg.ShutdownGrace = 100 * time.Millisecond
		cfg.AbortWait = time.Second
		cfg.CircuitBreakerThreshold = 3
		lt := newLaneTest(t, cfg, queuedRows("ses-1", 10)...)
		lt.workspaceRepo.EXPECT().List(gomock.Any()).Return([]*domain.Workspace{{ID: "ws-1", Integrations: []domain.Integration{
			emailIntegration("ses-1", domain.EmailProviderKindSES, 6000),
		}}}, nil).AnyTimes()
		var opened atomic.Int32
		lt.w.SetCircuitOpenCallback(func(string, string, *emailerror.ClassifiedError) { opened.Add(1) })

		flight := newInFlight()
		lt.email.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).
			DoAndReturn(func(ctx context.Context, req domain.SendEmailProviderRequest, _ bool) error {
				flight.enter(req.IntegrationID)
				defer flight.leave(req.IntegrationID)
				<-ctx.Done()
				return fmt.Errorf("operation error SES: SendEmail, %w", ctx.Err())
			}).AnyTimes()

		require.NoError(t, lt.w.Start(context.Background()))
		require.Eventually(t, func() bool { return flight.current("ses-1") == 10 }, 2*time.Second, 5*time.Millisecond)
		took := within(t, 5*time.Second, lt.w.Stop)

		assert.Less(t, took, cfg.ShutdownGrace+cfg.AbortWait)
		assert.Equal(t, 10, lt.queue.claimCount("ses-1"))
		assert.Zero(t, lt.queue.failedCount("ses-1"), "an aborted send is not a failure")
		assert.Zero(t, lt.queue.sentCount("ses-1"))
		assert.False(t, lt.w.circuitBreaker.IsOpen("ses-1"))
		assert.Zero(t, lt.w.GetCircuitBreakerStats()["ses-1"].Failures)
		assert.Zero(t, opened.Load())
	})

	t.Run("a send that ignores cancellation and succeeds is recorded", func(t *testing.T) {
		cfg := DefaultWorkerConfig()
		cfg.PollInterval = 10 * time.Millisecond
		cfg.ShutdownGrace = 100 * time.Millisecond
		cfg.AbortWait = 2 * time.Second
		lt := newLaneTest(t, cfg, queuedRows("smtp-1", 1)...)
		lt.workspaceRepo.EXPECT().List(gomock.Any()).Return([]*domain.Workspace{smtpWorkspace("ws-1", "smtp-1")}, nil).AnyTimes()

		started := make(chan struct{})
		lt.email.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).
			DoAndReturn(func(context.Context, domain.SendEmailProviderRequest, bool) error {
				close(started)
				time.Sleep(300 * time.Millisecond) // like SMTP, deaf to its context
				return nil
			})

		require.NoError(t, lt.w.Start(context.Background()))
		<-started
		took := within(t, 5*time.Second, lt.w.Stop)

		assert.GreaterOrEqual(t, took, 250*time.Millisecond)
		assert.Less(t, took, cfg.ShutdownGrace+cfg.AbortWait)
		assert.Equal(t, 1, lt.queue.sentCount("smtp-1"))
		require.NotEmpty(t, lt.queue.writeCtxErrs())
		for _, err := range lt.queue.writeCtxErrs() {
			assert.NoError(t, err, "the outcome is written on a live context")
		}
		require.NotEmpty(t, lt.historyErrs())
		for _, err := range lt.historyErrs() {
			assert.NoError(t, err)
		}
	})

	t.Run("Stop returns after the grace and AbortWait when a send never ends", func(t *testing.T) {
		cfg := DefaultWorkerConfig()
		cfg.PollInterval = 10 * time.Millisecond
		cfg.ShutdownGrace = 100 * time.Millisecond
		cfg.AbortWait = 200 * time.Millisecond
		lt := newLaneTest(t, cfg, queuedRows("smtp-1", 1)...)
		lt.workspaceRepo.EXPECT().List(gomock.Any()).Return([]*domain.Workspace{smtpWorkspace("ws-1", "smtp-1")}, nil).AnyTimes()

		started := make(chan struct{})
		lt.email.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).
			DoAndReturn(func(context.Context, domain.SendEmailProviderRequest, bool) error {
				close(started)
				time.Sleep(800 * time.Millisecond)
				return nil
			})

		require.NoError(t, lt.w.Start(context.Background()))
		<-started
		took := within(t, 5*time.Second, lt.w.Stop)

		assert.GreaterOrEqual(t, took, cfg.ShutdownGrace+cfg.AbortWait)
		assert.Less(t, took, 700*time.Millisecond)
		assert.Zero(t, lt.queue.sentCount("smtp-1"), "Stop did not wait for the send")

		// When the send does end, its outcome is still written on a live context.
		require.Eventually(t, func() bool { return lt.queue.sentCount("smtp-1") == 1 }, 2*time.Second, 5*time.Millisecond)
		for _, err := range lt.queue.writeCtxErrs() {
			assert.NoError(t, err)
		}
		require.Eventually(t, func() bool { return len(lt.historyErrs()) == 1 }, time.Second, 5*time.Millisecond)
		assert.NoError(t, lt.historyErrs()[0])
	})

	t.Run("Start refuses to run beside a previous run still finishing a send", func(t *testing.T) {
		cfg := DefaultWorkerConfig()
		cfg.PollInterval = 10 * time.Millisecond
		cfg.ShutdownGrace = 50 * time.Millisecond
		cfg.AbortWait = 50 * time.Millisecond
		lt := newLaneTest(t, cfg, queuedRows("smtp-1", 1)...)
		lt.workspaceRepo.EXPECT().List(gomock.Any()).Return([]*domain.Workspace{smtpWorkspace("ws-1", "smtp-1")}, nil).AnyTimes()

		started := make(chan struct{})
		release := make(chan struct{})
		lt.email.EXPECT().SendEmail(gomock.Any(), gomock.Any(), true).
			DoAndReturn(func(context.Context, domain.SendEmailProviderRequest, bool) error {
				close(started)
				<-release
				return nil
			})

		require.NoError(t, lt.w.Start(context.Background()))
		<-started
		lt.w.Stop()

		assert.Error(t, lt.w.Start(context.Background()), "the previous run is still sending")
		assert.False(t, lt.w.IsRunning())

		close(release)
		require.Eventually(t, func() bool { return lt.w.Start(context.Background()) == nil }, 2*time.Second, 5*time.Millisecond)
		lt.w.Stop()
		assert.Equal(t, 1, lt.queue.sentCount("smtp-1"))
	})

	t.Run("no retention sweep runs after Stop", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		cfg := DefaultWorkerConfig()
		cfg.PollInterval = 10 * time.Millisecond
		cfg.CleanupInterval = time.Nanosecond

		workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		listing := make(chan struct{})
		// The round's listing is still running when Stop is called. A sweep would
		// list the workspaces a second time.
		workspaceRepo.EXPECT().List(gomock.Any()).
			DoAndReturn(func(ctx context.Context) ([]*domain.Workspace, error) {
				close(listing)
				<-ctx.Done()
				return nil, ctx.Err()
			}).Times(1)

		w := NewEmailQueueWorker(mocks.NewMockEmailQueueRepository(ctrl), workspaceRepo,
			mocks.NewMockEmailServiceInterface(ctrl), mocks.NewMockMessageHistoryRepository(ctrl),
			cfg, quietLogger(ctrl))
		w.lastCleanupTime = time.Time{}

		require.NoError(t, w.Start(context.Background()))
		<-listing
		within(t, 5*time.Second, w.Stop)
	})
}

func TestEmailQueueWorker_Orphans(t *testing.T) {
	t.Run("fails rows of unknown and non-email integrations, once per interval", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		queueRepo := mocks.NewMockEmailQueueRepository(ctrl)
		history := mocks.NewMockMessageHistoryRepository(ctrl)
		history.EXPECT().Upsert(gomock.Any(), "ws-1", gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

		workspace := &domain.Workspace{ID: "ws-1", Integrations: []domain.Integration{
			emailIntegration("ses-1", domain.EmailProviderKindSES, 6000),
			{ID: "supabase-1", Type: domain.IntegrationTypeSupabase},
		}}
		orphans := []*domain.EmailQueueEntry{
			{ID: "o-1", Status: domain.EmailQueueStatusPending, IntegrationID: "deleted-1", MaxAttempts: 3},
			{ID: "o-2", Status: domain.EmailQueueStatusPending, IntegrationID: "supabase-1", MaxAttempts: 3},
		}

		queueRepo.EXPECT().FetchPendingForIntegration(gomock.Any(), "ws-1", "ses-1", gomock.Any()).Return(nil, nil).Times(3)
		queueRepo.EXPECT().FetchPendingOrphans(gomock.Any(), "ws-1", []string{"ses-1"}, orphanBatchSize).Return(orphans, nil)
		queueRepo.EXPECT().FetchPendingOrphans(gomock.Any(), "ws-1", []string{"ses-1"}, orphanBatchSize).Return(nil, nil)
		for _, orphan := range orphans {
			queueRepo.EXPECT().MarkAsProcessing(gomock.Any(), "ws-1", orphan.ID).Return(nil)
			queueRepo.EXPECT().MarkAsFailed(gomock.Any(), "ws-1", orphan.ID, "integration not found: "+orphan.IntegrationID, gomock.Any()).Return(nil)
		}

		w := NewEmailQueueWorker(queueRepo, mocks.NewMockWorkspaceRepository(ctrl), mocks.NewMockEmailServiceInterface(ctrl),
			history, DefaultWorkerConfig(), quietLogger(ctrl))
		w.initContexts(context.Background())

		w.processWorkspace(workspace)
		w.processWorkspace(workspace) // within the interval: no orphan pass

		w.orphanMu.Lock()
		w.lastOrphanCheck["ws-1"] = time.Now().Add(-w.config.OrphanCheckInterval)
		w.orphanMu.Unlock()
		w.processWorkspace(workspace)
	})

	t.Run("a full batch is looked at again next round", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		queueRepo := mocks.NewMockEmailQueueRepository(ctrl)
		history := mocks.NewMockMessageHistoryRepository(ctrl)
		history.EXPECT().Upsert(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

		full := queuedRows("deleted-1", orphanBatchSize)
		queueRepo.EXPECT().FetchPendingOrphans(gomock.Any(), "ws-1", []string{}, orphanBatchSize).Return(full, nil)
		queueRepo.EXPECT().FetchPendingOrphans(gomock.Any(), "ws-1", []string{}, orphanBatchSize).Return(nil, nil)
		queueRepo.EXPECT().MarkAsProcessing(gomock.Any(), "ws-1", gomock.Any()).Return(nil).Times(len(full))
		queueRepo.EXPECT().MarkAsFailed(gomock.Any(), "ws-1", gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(len(full))

		w := NewEmailQueueWorker(queueRepo, mocks.NewMockWorkspaceRepository(ctrl), mocks.NewMockEmailServiceInterface(ctrl),
			history, DefaultWorkerConfig(), quietLogger(ctrl))
		w.initContexts(context.Background())

		w.processWorkspace(&domain.Workspace{ID: "ws-1"})
		w.processWorkspace(&domain.Workspace{ID: "ws-1"})
		w.processWorkspace(&domain.Workspace{ID: "ws-1"}) // the second pass found less than a batch
	})
}
