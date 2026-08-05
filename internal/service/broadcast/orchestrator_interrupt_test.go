package broadcast_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/service/broadcast"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Coverage for a run cut short by its context going away — the shape of the
// TWOSTONE&Sons incident. Once the HTTP entry points stopped tying execution to
// the caller's connection there is no request-driven way to cancel a run, so
// this behaviour is pinned here rather than end-to-end.
//
// Plan: plans/broadcast-interruption-resilience-plan.md

// newInterruptOrchestrator wires an orchestrator whose first GetBroadcast fails
// the way a cancelled context does, and whose second (the terminal status write
// in Process's defer) succeeds. Returns the recorded terminal update.
type terminalUpdate struct {
	called    bool
	status    domain.BroadcastStatus
	reason    string
	pausedAt  *time.Time
	ctxErrSet bool
}

func TestProcess_LastRetryInterrupted_PausesBroadcastAndWritesOnALiveContext(t *testing.T) {
	ctrl, mockMessageSender, mockBroadcastRepository, mockTemplateRepo,
		mockContactRepo, mockTaskRepo, mockWorkspaceRepo, mockLogger, mockTimeProvider, mockEventBus := setupTestEnvironment(t)
	defer ctrl.Finish()

	mockTimeProvider.EXPECT().Now().Return(time.Now()).AnyTimes()

	broadcastID := "broadcast-interrupted"
	stored := createMockBroadcast(broadcastID, []string{"template-1"})

	// The recipient-count lookup is cut short by the cancellation.
	mockBroadcastRepository.EXPECT().
		GetBroadcast(gomock.Any(), "workspace-123", broadcastID).
		Return(nil, context.Canceled).
		Times(1)

	// The terminal status write in Process's defer then reloads the broadcast.
	mockBroadcastRepository.EXPECT().
		GetBroadcast(gomock.Any(), "workspace-123", broadcastID).
		DoAndReturn(func(ctx context.Context, _, _ string) (*domain.Broadcast, error) {
			assert.NoError(t, ctx.Err(),
				"the terminal read must not ride on the cancelled context")
			return stored, nil
		}).
		Times(1)

	var update terminalUpdate
	mockBroadcastRepository.EXPECT().
		UpdateBroadcast(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, b *domain.Broadcast) error {
			update.called = true
			update.status = b.Status
			update.pausedAt = b.PausedAt
			if b.PauseReason != nil {
				update.reason = *b.PauseReason
			}
			update.ctxErrSet = ctx.Err() != nil
			return nil
		}).
		Times(1)

	orchestrator := broadcast.NewBroadcastOrchestrator(
		mockMessageSender,
		mockBroadcastRepository,
		mockTemplateRepo,
		mockContactRepo,
		mockTaskRepo,
		mockWorkspaceRepo,
		nil, // emailQueueRepo
		nil, // abTestEvaluator
		mockLogger,
		createTestConfig(),
		mockTimeProvider,
		"https://api.example.com",
		mockEventBus,
	)

	task := &domain.Task{
		ID:          "task-123",
		WorkspaceID: "workspace-123",
		BroadcastID: &broadcastID,
		// Last attempt: 2 >= MaxRetries-1.
		RetryCount: 2,
		MaxRetries: 3,
		State: &domain.TaskState{
			SendBroadcast: &domain.SendBroadcastState{
				BroadcastID:     broadcastID,
				TotalRecipients: 0, // forces the recipient-count lookup
			},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // the dispatcher hung up / the server is shutting down

	completed, err := orchestrator.Process(ctx, task, time.Now().Add(30*time.Second))

	require.Error(t, err)
	assert.False(t, completed)
	assert.True(t, broadcast.IsInterrupted(err),
		"a cancelled run must be classified as interrupted, got %v", err)

	require.True(t, update.called,
		"the broadcast must be finalized; writing on the cancelled context silently left it stranded in processing")
	assert.False(t, update.ctxErrSet, "the terminal write must use a live context")
	assert.Equal(t, domain.BroadcastStatusPaused, update.status,
		"an interrupted broadcast must stay resumable, not be marked failed")
	assert.NotEmpty(t, update.reason, "the pause reason must explain the interruption")
	assert.NotNil(t, update.pausedAt)
}

// TestProcess_LastRetryGenuineError_StillMarksFailed guards the other side of
// the split: a real problem with the broadcast is terminal, not resumable.
func TestProcess_LastRetryGenuineError_StillMarksFailed(t *testing.T) {
	ctrl, mockMessageSender, mockBroadcastRepository, mockTemplateRepo,
		mockContactRepo, mockTaskRepo, mockWorkspaceRepo, mockLogger, mockTimeProvider, mockEventBus := setupTestEnvironment(t)
	defer ctrl.Finish()

	mockTimeProvider.EXPECT().Now().Return(time.Now()).AnyTimes()

	broadcastID := "broadcast-broken"
	stored := createMockBroadcast(broadcastID, []string{"template-1"})

	mockBroadcastRepository.EXPECT().
		GetBroadcast(gomock.Any(), "workspace-123", broadcastID).
		Return(nil, errors.New("no such broadcast")).
		Times(1)

	mockBroadcastRepository.EXPECT().
		GetBroadcast(gomock.Any(), "workspace-123", broadcastID).
		Return(stored, nil).
		Times(1)

	var update terminalUpdate
	mockBroadcastRepository.EXPECT().
		UpdateBroadcast(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, b *domain.Broadcast) error {
			update.called = true
			update.status = b.Status
			return nil
		}).
		Times(1)

	orchestrator := broadcast.NewBroadcastOrchestrator(
		mockMessageSender, mockBroadcastRepository, mockTemplateRepo, mockContactRepo,
		mockTaskRepo, mockWorkspaceRepo, nil, nil, mockLogger, createTestConfig(),
		mockTimeProvider, "https://api.example.com", mockEventBus,
	)

	task := &domain.Task{
		ID:          "task-123",
		WorkspaceID: "workspace-123",
		BroadcastID: &broadcastID,
		RetryCount:  2,
		MaxRetries:  3,
		State: &domain.TaskState{
			SendBroadcast: &domain.SendBroadcastState{
				BroadcastID:     broadcastID,
				TotalRecipients: 0,
			},
		},
	}

	completed, err := orchestrator.Process(context.Background(), task, time.Now().Add(30*time.Second))

	require.Error(t, err)
	assert.False(t, completed)
	assert.False(t, broadcast.IsInterrupted(err))
	require.True(t, update.called)
	assert.Equal(t, domain.BroadcastStatusFailed, update.status)
}

// TestFetchBatch_CancelledContextIsInterrupted pins the error code. Reporting a
// cancellation as BROADCAST_NOT_FOUND is what sent the incident investigation
// looking for a deleted broadcast.
func TestFetchBatch_CancelledContextIsInterrupted(t *testing.T) {
	ctrl, mockMessageSender, mockBroadcastRepository, mockTemplateRepo,
		mockContactRepo, mockTaskRepo, mockWorkspaceRepo, mockLogger, mockTimeProvider, mockEventBus := setupTestEnvironment(t)
	defer ctrl.Finish()

	mockTimeProvider.EXPECT().Now().Return(time.Now()).AnyTimes()

	orchestrator := broadcast.NewBroadcastOrchestrator(
		mockMessageSender, mockBroadcastRepository, mockTemplateRepo, mockContactRepo,
		mockTaskRepo, mockWorkspaceRepo, nil, nil, mockLogger, createTestConfig(),
		mockTimeProvider, "https://api.example.com", mockEventBus,
	)

	t.Run("cancellation", func(t *testing.T) {
		mockBroadcastRepository.EXPECT().
			GetBroadcast(gomock.Any(), "workspace-123", "broadcast-1").
			Return(nil, context.Canceled).
			Times(1)

		_, err := orchestrator.FetchBatch(context.Background(), "workspace-123", "broadcast-1", "", 10)
		require.Error(t, err)

		var bErr *broadcast.BroadcastError
		require.ErrorAs(t, err, &bErr)
		assert.Equal(t, broadcast.ErrCodeInterrupted, bErr.Code)
		assert.NotEqual(t, broadcast.ErrCodeBroadcastNotFound, bErr.Code)
	})

	t.Run("genuinely missing broadcast", func(t *testing.T) {
		mockBroadcastRepository.EXPECT().
			GetBroadcast(gomock.Any(), "workspace-123", "broadcast-1").
			Return(nil, errors.New("sql: no rows in result set")).
			Times(1)

		_, err := orchestrator.FetchBatch(context.Background(), "workspace-123", "broadcast-1", "", 10)
		require.Error(t, err)

		var bErr *broadcast.BroadcastError
		require.ErrorAs(t, err, &bErr)
		assert.Equal(t, broadcast.ErrCodeBroadcastNotFound, bErr.Code)
	})
}

// TestProcess_InterruptionMaskedByTxDone_StillPauses covers the shape a real
// interruption almost always takes, and the one the error-only check missed:
// when the context dies inside the enqueue transaction, database/sql has
// already rolled back by the time Commit is called, so the failure surfaces as
// "transaction has already been committed or rolled back" and the error chain
// carries no context error at all. Classifying off the error alone marked the
// broadcast failed — permanently — instead of paused and resumable.
func TestProcess_InterruptionMaskedByTxDone_StillPauses(t *testing.T) {
	ctrl, mockMessageSender, mockBroadcastRepository, mockTemplateRepo,
		mockContactRepo, mockTaskRepo, mockWorkspaceRepo, mockLogger, mockTimeProvider, mockEventBus := setupTestEnvironment(t)
	defer ctrl.Finish()

	mockTimeProvider.EXPECT().Now().Return(time.Now()).AnyTimes()

	broadcastID := "broadcast-txdone"
	stored := createMockBroadcast(broadcastID, []string{"template-1"})

	// The error a cancelled transaction actually produces: no context anywhere
	// in its chain.
	txDone := errors.New("failed to commit transaction: sql: transaction has already been committed or rolled back")
	require.False(t, broadcast.IsInterrupted(txDone),
		"precondition: this error does not look like a cancellation")

	mockBroadcastRepository.EXPECT().
		GetBroadcast(gomock.Any(), "workspace-123", broadcastID).
		Return(nil, txDone).
		Times(1)

	mockBroadcastRepository.EXPECT().
		GetBroadcast(gomock.Any(), "workspace-123", broadcastID).
		Return(stored, nil).
		Times(1)

	var update terminalUpdate
	mockBroadcastRepository.EXPECT().
		UpdateBroadcast(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, b *domain.Broadcast) error {
			update.called = true
			update.status = b.Status
			if b.PauseReason != nil {
				update.reason = *b.PauseReason
			}
			return nil
		}).
		Times(1)

	orchestrator := broadcast.NewBroadcastOrchestrator(
		mockMessageSender, mockBroadcastRepository, mockTemplateRepo, mockContactRepo,
		mockTaskRepo, mockWorkspaceRepo, nil, nil, mockLogger, createTestConfig(),
		mockTimeProvider, "https://api.example.com", mockEventBus,
	)

	task := &domain.Task{
		ID: "task-123", WorkspaceID: "workspace-123", BroadcastID: &broadcastID,
		RetryCount: 2, MaxRetries: 3,
		State: &domain.TaskState{
			SendBroadcast: &domain.SendBroadcastState{BroadcastID: broadcastID, TotalRecipients: 0},
		},
	}

	// The context is dead even though the error cannot say so.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := orchestrator.Process(ctx, task, time.Now().Add(30*time.Second))
	require.Error(t, err)

	require.True(t, update.called)
	assert.Equal(t, domain.BroadcastStatusPaused, update.status,
		"the run was interrupted — ask the context, not just the error")
	assert.NotEmpty(t, update.reason)
}
