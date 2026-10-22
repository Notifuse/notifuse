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

// Which failed run finalizes its broadcast. The task service retries a failed
// run while the task has retries left (MarkAsFailedTx), so only the run it gives
// up on may mark the broadcast failed, or paused when it was interrupted: a
// broadcast marked failed one attempt early was then sent to the rest of its
// list by the retry and flipped back to processed.
//
// Plan: plans/fix-broadcast-last-attempt-plan.md

// lastAttemptRun describes one failing run: the counts it starts with, what the
// finalizer reads back, and how the run fails.
type lastAttemptRun struct {
	retryCount int
	maxRetries int

	// storedTask and storedErr answer the finalizer's re-read of the task. When
	// both are nil the run must not read the task at all.
	storedTask *domain.Task
	storedErr  error

	// statusAtEnd is the broadcast's status when the finalizer reads it back.
	statusAtEnd domain.BroadcastStatus

	// refusedByRow makes the conditional write find an excluded status on the
	// row, as when a pause commits after the finalizer's read.
	refusedByRow bool

	// ctx and countErr shape the failure: the recipient count, the first thing
	// a run does, fails with countErr. cancelWhileFinalizing cancels ctx while
	// the finalizer reads the task back, cancelWhileWriting while it writes.
	ctx                   context.Context
	countErr              error
	cancelWhileFinalizing bool
	cancelWhileWriting    bool

	// abTest runs an A/B broadcast past the recipient count instead, with
	// phase in its state; testingWriteErr fails its move to testing. wantErr,
	// when set, is part of the error the run must return.
	abTest          bool
	phase           string
	testingWriteErr error
	wantErr         string
}

// broadcastWrite records what the finalizer wrote to the broadcast last, every
// status it wrote, and the order of its reads.
type broadcastWrite struct {
	called   bool
	status   domain.BroadcastStatus
	statuses []domain.BroadcastStatus
	reason   string
	unless   []domain.BroadcastStatus
	reads    []string
}

func runFailingAttempt(t *testing.T, run lastAttemptRun) broadcastWrite {
	t.Helper()

	ctrl, mockMessageSender, mockBroadcastRepository, mockTemplateRepo,
		mockContactRepo, mockTaskRepo, mockWorkspaceRepo, mockLogger, mockTimeProvider, mockEventBus := setupTestEnvironment(t)
	defer ctrl.Finish()

	mockTimeProvider.EXPECT().Now().Return(time.Now()).AnyTimes()

	ctx := run.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	broadcastID := "broadcast-last-attempt"
	var write broadcastWrite
	reads := 0
	mockBroadcastRepository.EXPECT().
		GetBroadcast(gomock.Any(), "workspace-123", broadcastID).
		DoAndReturn(func(ctx context.Context, _, _ string) (*domain.Broadcast, error) {
			reads++
			b := createMockBroadcast(broadcastID, []string{"template-1"})
			if run.abTest {
				b.TestSettings.Enabled = true
				b.TestSettings.SamplePercentage = 10
			}
			if reads > 1 {
				assert.NoError(t, ctx.Err(), "the finalizer's read must not ride on the run's context")
				write.reads = append(write.reads, "broadcast")
				b.Status = run.statusAtEnd
			}
			return b, nil
		}).
		AnyTimes()

	totalRecipients := 0 // forces the recipient count
	if run.abTest {
		totalRecipients = 100
		mockWorkspaceRepo.EXPECT().GetByID(gomock.Any(), "workspace-123").Return(&domain.Workspace{
			ID: "workspace-123",
			Settings: domain.WorkspaceSettings{
				SecretKey:                "k",
				MarketingEmailProviderID: "marketing-provider-id",
			},
			Integrations: []domain.Integration{{
				ID:   "marketing-provider-id",
				Type: domain.IntegrationTypeEmail,
				EmailProvider: domain.EmailProvider{
					Kind: domain.EmailProviderKindSES,
					SES:  &domain.AmazonSESSettings{AccessKey: "a", SecretKey: "b", Region: "us-east-1"},
				},
			}},
		}, nil)
		if run.testingWriteErr != nil {
			mockBroadcastRepository.EXPECT().
				UpdateBroadcast(gomock.Any(), gomock.Any()).
				Return(run.testingWriteErr).
				Times(1)
		}
	} else {
		mockContactRepo.EXPECT().
			CountContactsForBroadcast(gomock.Any(), "workspace-123", gomock.Any()).
			Return(0, run.countErr).
			Times(1)
	}

	if run.storedTask != nil || run.storedErr != nil {
		mockTaskRepo.EXPECT().
			Get(gomock.Any(), "workspace-123", "task-123").
			DoAndReturn(func(ctx context.Context, _, _ string) (*domain.Task, error) {
				assert.NoError(t, ctx.Err(), "the re-read must not ride on the run's context")
				write.reads = append(write.reads, "task")
				if run.cancelWhileFinalizing {
					cancel()
				}
				return run.storedTask, run.storedErr
			}).
			Times(1)
	}

	// The finalizer writes through the conditional status write only: the
	// whole-row UpdateBroadcast decided on its read, and is not expected.
	mockBroadcastRepository.EXPECT().
		UpdateBroadcastStatusUnless(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, b *domain.Broadcast, unless []domain.BroadcastStatus) (bool, error) {
			assert.NoError(t, ctx.Err(), "the write must not ride on the run's context")
			write.called = true
			write.status = b.Status
			write.statuses = append(write.statuses, b.Status)
			write.unless = unless
			if run.cancelWhileWriting {
				cancel()
			}
			if b.PauseReason != nil {
				write.reason = *b.PauseReason
			}
			return !run.refusedByRow, nil
		}).
		AnyTimes()

	orchestrator := broadcast.NewBroadcastOrchestrator(
		mockMessageSender, mockBroadcastRepository, mockTemplateRepo, mockContactRepo,
		mockTaskRepo, mockWorkspaceRepo, nil, nil, mockLogger, createTestConfig(),
		mockTimeProvider, "https://api.example.com", mockEventBus,
	)

	task := &domain.Task{
		ID:          "task-123",
		WorkspaceID: "workspace-123",
		BroadcastID: &broadcastID,
		RetryCount:  run.retryCount,
		MaxRetries:  run.maxRetries,
		State: &domain.TaskState{
			SendBroadcast: &domain.SendBroadcastState{
				BroadcastID:     broadcastID,
				TotalRecipients: totalRecipients,
				Phase:           run.phase,
			},
		},
	}

	completed, err := orchestrator.Process(ctx, task, time.Now().Add(30*time.Second))
	require.Error(t, err)
	assert.False(t, completed)
	if run.wantErr != "" {
		assert.Contains(t, err.Error(), run.wantErr)
	}

	return write
}

func cancelledContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func TestProcess_FailedRunFinalizesOnlyTheLastAttempt(t *testing.T) {
	genuine := errors.New("database is down")

	t.Run("a failure with a retry left leaves the broadcast to that retry", func(t *testing.T) {
		// 2 of 3: MarkAsFailedTx retries it, so the broadcast must not be
		// marked failed, and the task need not be read back.
		write := runFailingAttempt(t, lastAttemptRun{
			retryCount: 2, maxRetries: 3,
			countErr: genuine,
		})
		assert.False(t, write.called, "the broadcast was finalized on an attempt the task service retries")
	})

	t.Run("the last attempt marks the broadcast failed", func(t *testing.T) {
		write := runFailingAttempt(t, lastAttemptRun{
			retryCount: 3, maxRetries: 3,
			storedTask:  &domain.Task{ID: "task-123", RetryCount: 3, MaxRetries: 3},
			statusAtEnd: domain.BroadcastStatusProcessing,
			countErr:    genuine,
		})
		require.True(t, write.called)
		assert.Equal(t, domain.BroadcastStatusFailed, write.status)
		assert.ElementsMatch(t, []domain.BroadcastStatus{
			domain.BroadcastStatusCancelled, domain.BroadcastStatusPaused, domain.BroadcastStatusProcessed,
		}, write.unless, "the write must keep a cancel, a pause or a completion committed after the read")
		assert.Equal(t, []string{"broadcast", "task"}, write.reads,
			"the broadcast is read first: a read past a pause then implies the resume, and its fresh budget, committed")
	})

	t.Run("a task with no retries allowed fails on its first attempt", func(t *testing.T) {
		write := runFailingAttempt(t, lastAttemptRun{
			retryCount: 0, maxRetries: 0,
			storedTask:  &domain.Task{ID: "task-123", RetryCount: 0, MaxRetries: 0},
			statusAtEnd: domain.BroadcastStatusProcessing,
			countErr:    genuine,
		})
		require.True(t, write.called)
		assert.Equal(t, domain.BroadcastStatusFailed, write.status)
	})

	t.Run("a resume during the last attempt gives it a fresh budget", func(t *testing.T) {
		// ResetRetryBudget ran while the run held the task: the failure is
		// retried, so the broadcast stays as the resume left it.
		write := runFailingAttempt(t, lastAttemptRun{
			retryCount: 3, maxRetries: 3,
			storedTask:  &domain.Task{ID: "task-123", RetryCount: 0, MaxRetries: 3},
			statusAtEnd: domain.BroadcastStatusProcessing,
			countErr:    genuine,
		})
		assert.False(t, write.called, "the broadcast was marked failed although its task will be retried")
	})

	t.Run("a failed re-read keeps the decision the run started with", func(t *testing.T) {
		write := runFailingAttempt(t, lastAttemptRun{
			retryCount: 3, maxRetries: 3,
			storedErr:   errors.New("connection refused"),
			statusAtEnd: domain.BroadcastStatusProcessing,
			countErr:    genuine,
		})
		require.True(t, write.called, "a broadcast left sending under a failed task is stranded")
		assert.Equal(t, domain.BroadcastStatusFailed, write.status)
	})

	for _, kept := range []domain.BroadcastStatus{
		domain.BroadcastStatusCancelled,
		domain.BroadcastStatusPaused,
		domain.BroadcastStatusProcessed,
	} {
		t.Run("a broadcast read back "+string(kept)+" keeps its status", func(t *testing.T) {
			// No task read either: the decision does not depend on it.
			write := runFailingAttempt(t, lastAttemptRun{
				retryCount: 3, maxRetries: 3,
				statusAtEnd: kept,
				countErr:    genuine,
			})
			assert.False(t, write.called, "a status committed during the last attempt was overwritten")
		})
	}

	t.Run("a status the row holds by the time of the write is kept", func(t *testing.T) {
		// A pause committing after the finalizer's read: the write finds it on
		// the row and leaves it, and the run's own error is what it returns.
		write := runFailingAttempt(t, lastAttemptRun{
			retryCount: 3, maxRetries: 3,
			storedTask:   &domain.Task{ID: "task-123", RetryCount: 3, MaxRetries: 3},
			statusAtEnd:  domain.BroadcastStatusProcessing,
			refusedByRow: true,
			countErr:     genuine,
		})
		assert.True(t, write.called, "the write must be attempted, and refused by the row")
	})
}

func TestProcess_EveryErrorOfTheLastAttemptFinalizes(t *testing.T) {
	// The finalizer reads the error Process returns through its own err, so an
	// error returned without it left the broadcast sending under a task that
	// failed for good.
	lastAttempt := func(run lastAttemptRun) lastAttemptRun {
		run.retryCount, run.maxRetries = 3, 3
		run.storedTask = &domain.Task{ID: "task-123", RetryCount: 3, MaxRetries: 3}
		run.statusAtEnd = domain.BroadcastStatusProcessing
		run.abTest = true
		return run
	}

	t.Run("an A/B broadcast whose move to testing fails", func(t *testing.T) {
		write := runFailingAttempt(t, lastAttempt(lastAttemptRun{
			testingWriteErr: errors.New("database is down"),
			wantErr:         "failed to update broadcast status to testing",
		}))
		require.True(t, write.called, "the broadcast was left sending")
		assert.Equal(t, domain.BroadcastStatusFailed, write.status)
	})

	t.Run("a winner phase with no winner picked", func(t *testing.T) {
		write := runFailingAttempt(t, lastAttempt(lastAttemptRun{
			phase:   "winner",
			wantErr: "no winning template selected",
		}))
		require.True(t, write.called, "the broadcast was left sending")
		assert.Equal(t, domain.BroadcastStatusFailed, write.status)
	})
}

func TestProcess_InterruptedRunPausesOnlyTheLastAttempt(t *testing.T) {
	t.Run("an interruption with a retry left does not pause", func(t *testing.T) {
		write := runFailingAttempt(t, lastAttemptRun{
			retryCount: 2, maxRetries: 3,
			ctx:      cancelledContext(),
			countErr: context.Canceled,
		})
		assert.False(t, write.called, "the broadcast was paused on an attempt the task service runs again")
	})

	t.Run("an interruption on the last attempt pauses, counting every attempt", func(t *testing.T) {
		write := runFailingAttempt(t, lastAttemptRun{
			retryCount: 3, maxRetries: 3,
			storedTask:  &domain.Task{ID: "task-123", RetryCount: 3, MaxRetries: 3},
			statusAtEnd: domain.BroadcastStatusProcessing,
			ctx:         cancelledContext(),
			countErr:    context.Canceled,
		})
		require.True(t, write.called)
		assert.Equal(t, domain.BroadcastStatusPaused, write.status)
		assert.Contains(t, write.reason, "after 4 attempts",
			"the first attempt and its three retries")
	})

	t.Run("a stuck run whose failure the task service recorded first still pauses", func(t *testing.T) {
		// Past its lease, the task service marks the task failed while the
		// processor unwinds: the finalizer then reads the count already raised.
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		defer cancel()

		write := runFailingAttempt(t, lastAttemptRun{
			retryCount: 3, maxRetries: 3,
			storedTask:  &domain.Task{ID: "task-123", RetryCount: 4, MaxRetries: 3, Status: domain.TaskStatusFailed},
			statusAtEnd: domain.BroadcastStatusProcessing,
			ctx:         ctx,
			countErr:    context.DeadlineExceeded,
		})
		require.True(t, write.called)
		assert.Equal(t, domain.BroadcastStatusPaused, write.status)
	})

	t.Run("a cancellation landing while the finalizer reads pauses", func(t *testing.T) {
		// The task service releases a run whose context was cancelled before
		// it returned, without spending its budget: the broadcast must stay
		// resumable, whatever error the run ended on.
		write := runFailingAttempt(t, lastAttemptRun{
			retryCount: 3, maxRetries: 3,
			storedTask:            &domain.Task{ID: "task-123", RetryCount: 3, MaxRetries: 3},
			statusAtEnd:           domain.BroadcastStatusProcessing,
			countErr:              errors.New("database is down"),
			cancelWhileFinalizing: true,
		})
		require.True(t, write.called)
		assert.Equal(t, domain.BroadcastStatusPaused, write.status,
			"a released task's broadcast was marked failed")
	})

	t.Run("a cancellation landing while the failed status is written pauses", func(t *testing.T) {
		write := runFailingAttempt(t, lastAttemptRun{
			retryCount: 3, maxRetries: 3,
			storedTask:         &domain.Task{ID: "task-123", RetryCount: 3, MaxRetries: 3},
			statusAtEnd:        domain.BroadcastStatusProcessing,
			countErr:           errors.New("database is down"),
			cancelWhileWriting: true,
		})
		assert.Equal(t, []domain.BroadcastStatus{domain.BroadcastStatusFailed, domain.BroadcastStatusPaused}, write.statuses,
			"a released task's broadcast was left failed")
		assert.Contains(t, write.reason, "interrupted")
	})

	t.Run("an interrupted last attempt leaves a cancelled broadcast cancelled", func(t *testing.T) {
		write := runFailingAttempt(t, lastAttemptRun{
			retryCount: 3, maxRetries: 3,
			statusAtEnd: domain.BroadcastStatusCancelled,
			ctx:         cancelledContext(),
			countErr:    context.Canceled,
		})
		assert.False(t, write.called, "the user's cancel was overwritten with a pause")
	})
}
