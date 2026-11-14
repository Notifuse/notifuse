package broadcast_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	domainmocks "github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/internal/service/broadcast"
	"github.com/Notifuse/notifuse/internal/service/broadcast/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
)

// The progress of a run is written on the run's lease: it survives a cancellation of the
// run's context (a shutdown keeps the cursor of the last batch), never runs past the lease,
// and once another execution holds the task the run stops instead of sending on.

func newLeaseOrchestrator(t *testing.T, taskRepo *domainmocks.MockTaskRepository) broadcast.BroadcastOrchestratorInterface {
	t.Helper()
	ctrl := gomock.NewController(t)
	logger := pkgmocks.NewMockLogger(ctrl)
	logger.EXPECT().WithFields(gomock.Any()).Return(logger).AnyTimes()
	logger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(logger).AnyTimes()
	logger.EXPECT().Info(gomock.Any()).AnyTimes()
	logger.EXPECT().Debug(gomock.Any()).AnyTimes()
	logger.EXPECT().Error(gomock.Any()).AnyTimes()
	logger.EXPECT().Warn(gomock.Any()).AnyTimes()
	timeProvider := mocks.NewMockTimeProvider(ctrl)
	timeProvider.EXPECT().Now().Return(time.Now()).AnyTimes()
	timeProvider.EXPECT().Since(gomock.Any()).Return(time.Second).AnyTimes()
	return broadcast.NewBroadcastOrchestrator(
		mocks.NewMockMessageSender(ctrl), domainmocks.NewMockBroadcastRepository(ctrl), domainmocks.NewMockTemplateRepository(ctrl),
		domainmocks.NewMockContactRepository(ctrl), taskRepo, domainmocks.NewMockWorkspaceRepository(ctrl),
		nil, nil, logger, nil, timeProvider, "https://api.example.com", domainmocks.NewMockEventBus(ctrl),
	)
}

func saveProgress(o broadcast.BroadcastOrchestratorInterface, ctx context.Context) error {
	state := &domain.SendBroadcastState{BroadcastID: "broadcast-123", TotalRecipients: 100}
	_, err := o.SaveProgressState(ctx, "workspace-123", "task-123", state, 10, 0, 10, time.Now().Add(-time.Minute), time.Now().Add(-time.Minute))
	return err
}

func TestSaveProgressState_WritesOnTheRunsLease(t *testing.T) {
	lease := time.Now().Add(time.Minute).UTC().Truncate(time.Microsecond)

	t.Run("a cancelled run still writes, bounded by its lease, with the lease in the context", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		taskRepo := domainmocks.NewMockTaskRepository(ctrl)
		parent, cancelDeadline := context.WithDeadline(domain.WithTaskLease(context.Background(), lease), lease)
		defer cancelDeadline()
		ctx, cancel := context.WithCancel(parent)
		cancel()

		taskRepo.EXPECT().SaveState(gomock.Any(), "workspace-123", "task-123", gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, _, _ string, _ float64, _ *domain.TaskState) error {
				assert.NoError(t, ctx.Err(), "a shutdown's cancellation must not lose the last cursor")
				deadline, ok := ctx.Deadline()
				assert.True(t, ok && deadline.Equal(lease), "the write never runs past the lease")
				got, ok := domain.TaskLeaseFrom(ctx)
				assert.True(t, ok && got.Equal(lease), "the repository matches the run's lease")
				return nil
			})

		require.NoError(t, saveProgress(newLeaseOrchestrator(t, taskRepo), ctx))
	})

	t.Run("past the lease the write's context is done", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		taskRepo := domainmocks.NewMockTaskRepository(ctrl)
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		defer cancel()

		taskRepo.EXPECT().SaveState(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, _, _ string, _ float64, _ *domain.TaskState) error {
				assert.ErrorIs(t, ctx.Err(), context.DeadlineExceeded)
				return ctx.Err()
			})

		assert.Error(t, saveProgress(newLeaseOrchestrator(t, taskRepo), ctx))
	})

	t.Run("a lost lease is reported as such", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		taskRepo := domainmocks.NewMockTaskRepository(ctrl)
		taskRepo.EXPECT().SaveState(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(domain.ErrTaskNotRunning)

		assert.ErrorIs(t, saveProgress(newLeaseOrchestrator(t, taskRepo), context.Background()), domain.ErrTaskNotRunning)
	})
}

// A run whose progress write finds the task held by another execution stops before its next
// batch: sending on would send the same recipients as the execution that holds the task.
// A run that finds its task no longer its own (another execution holds it, or a cancel marked
// it failed) stops after the batch it was queuing. That batch may have committed after the
// cancel cleared the queue: under the broadcast's lock, a cancelled broadcast's queued emails
// are deleted, and another status leaves them to be sent. A paused one's are paused: a pause
// leaves a held task alone, so the run lost its task another way first (a second instance took
// it past its lease, or a shutdown released it while the run was still saving).
func TestProcess_StopsWhenTheTaskIsNoLongerItsOwn(t *testing.T) {
	cases := []struct {
		name   string
		status domain.BroadcastStatus
		expect func(q *domainmocks.MockEmailQueueRepository)
	}{
		// The locked read fails: the queued emails stay as they are, and the run still stops.
		{"the broadcast cannot be read", "", func(*domainmocks.MockEmailQueueRepository) {}},
		{"another execution holds the task", domain.BroadcastStatusProcessing, func(*domainmocks.MockEmailQueueRepository) {}},
		{"cancelled during the batch", domain.BroadcastStatusCancelled, func(q *domainmocks.MockEmailQueueRepository) {
			q.EXPECT().DeleteBySourceTx(gomock.Any(), gomock.Any(), domain.EmailQueueSourceBroadcast, "broadcast-123").Return(int64(10), nil)
		}},
		{"paused, the task lost first", domain.BroadcastStatusPaused, func(q *domainmocks.MockEmailQueueRepository) {
			q.EXPECT().PauseBySourceTx(gomock.Any(), gomock.Any(), domain.EmailQueueSourceBroadcast, "broadcast-123").Return(int64(10), nil)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stopsWhenTheTaskIsNoLongerItsOwn(t, tc.status, tc.expect)
		})
	}
}

func stopsWhenTheTaskIsNoLongerItsOwn(t *testing.T, statusAfterBatch domain.BroadcastStatus, expectQueue func(q *domainmocks.MockEmailQueueRepository)) {
	ctrl := gomock.NewController(t)
	queue := domainmocks.NewMockEmailQueueRepository(ctrl)
	expectQueue(queue)
	sender := mocks.NewMockMessageSender(ctrl)
	broadcasts := domainmocks.NewMockBroadcastRepository(ctrl)
	templates := domainmocks.NewMockTemplateRepository(ctrl)
	contacts := domainmocks.NewMockContactRepository(ctrl)
	taskRepo := domainmocks.NewMockTaskRepository(ctrl)
	workspaces := domainmocks.NewMockWorkspaceRepository(ctrl)
	eventBus := domainmocks.NewMockEventBus(ctrl)
	logger := pkgmocks.NewMockLogger(ctrl)
	logger.EXPECT().WithFields(gomock.Any()).Return(logger).AnyTimes()
	logger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(logger).AnyTimes()
	logger.EXPECT().Info(gomock.Any()).AnyTimes()
	logger.EXPECT().Debug(gomock.Any()).AnyTimes()
	logger.EXPECT().Error(gomock.Any()).AnyTimes()
	logger.EXPECT().Warn(gomock.Any()).AnyTimes()
	timeProvider := mocks.NewMockTimeProvider(ctrl)
	timeProvider.EXPECT().Now().Return(time.Now()).AnyTimes()
	timeProvider.EXPECT().Since(gomock.Any()).Return(time.Second).AnyTimes()
	eventBus.EXPECT().Publish(gomock.Any(), gomock.Any()).AnyTimes()
	broadcasts.EXPECT().MarkBroadcastStarted(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	o := broadcast.NewBroadcastOrchestrator(sender, broadcasts, templates, contacts, taskRepo, workspaces,
		queue, nil, logger, &broadcast.Config{FetchBatchSize: 10}, timeProvider, "https://api.example.com", eventBus)

	broadcastID := "broadcast-123"
	workspace := &domain.Workspace{
		ID:       "workspace-123",
		Settings: domain.WorkspaceSettings{MarketingEmailProviderID: "ses-1", SecretKey: "k"},
	}
	workspace.AddIntegration(domain.Integration{
		ID: "ses-1", Type: domain.IntegrationTypeEmail,
		EmailProvider: domain.EmailProvider{Kind: domain.EmailProviderKindSES, SES: &domain.AmazonSESSettings{Region: "us-east-1"},
			Senders: []domain.EmailSender{{Name: "S", Email: "s@example.com"}}},
	})
	workspaces.EXPECT().GetByID(gomock.Any(), "workspace-123").Return(workspace, nil).AnyTimes()
	batchQueued := false
	broadcasts.EXPECT().GetBroadcast(gomock.Any(), "workspace-123", broadcastID).DoAndReturn(func(context.Context, string, string) (*domain.Broadcast, error) {
		status := domain.BroadcastStatusProcessing
		if batchQueued {
			status = statusAfterBatch
		}
		return &domain.Broadcast{
			ID: broadcastID, Status: status, Audience: domain.AudienceSettings{List: "list-1"},
			TestSettings: domain.BroadcastTestSettings{Variations: []domain.BroadcastVariation{{TemplateID: "template-1"}}},
		}, nil
	}).AnyTimes()
	templates.EXPECT().GetTemplateByID(gomock.Any(), "workspace-123", "template-1", int64(0)).Return(&domain.Template{
		ID: "template-1", Email: &domain.EmailTemplate{Subject: "S",
			VisualEditorTree: &notifuse_mjml.MJMLBlock{BaseBlock: notifuse_mjml.NewBaseBlock("root", notifuse_mjml.MJMLComponentMjml)}},
	}, nil).AnyTimes()

	var batch []*domain.ContactWithList
	for i := 0; i < 10; i++ {
		batch = append(batch, &domain.ContactWithList{Contact: &domain.Contact{Email: fmt.Sprintf("c%d@example.com", i)}, ListID: "list-1"})
	}
	// One batch only: the second would go to recipients the other execution sends to.
	contacts.EXPECT().GetContactsForBroadcast(gomock.Any(), "workspace-123", gomock.Any(), 10, gomock.Any()).Return(batch, nil).Times(1)
	sender.EXPECT().SendBatch(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(10, 0, nil).Times(1)
	// The queued emails are settled from the broadcast read with its lock, inside one
	// transaction (the queue writes are the Tx ones).
	// The run's context is cancelled meanwhile too (a shutdown): the settle runs on a context
	// that outlives it, bounded by the run's deadline.
	deadline := time.Now().Add(5 * time.Minute)
	runCtx, cancelRun := context.WithDeadline(context.Background(), deadline)
	defer cancelRun()
	broadcasts.EXPECT().WithTransaction(gomock.Any(), "workspace-123", gomock.Any()).
		DoAndReturn(func(ctx context.Context, _ string, fn func(*sql.Tx) error) error {
			assert.NoError(t, ctx.Err(), "the settle outlives the run's cancellation")
			got, ok := ctx.Deadline()
			assert.True(t, ok && got.Equal(deadline), "the settle is bounded by the run's deadline")
			return fn(nil)
		}).Times(1)
	broadcasts.EXPECT().GetBroadcastForUpdateTx(gomock.Any(), gomock.Any(), "workspace-123", broadcastID).
		DoAndReturn(func(context.Context, *sql.Tx, string, string) (*domain.Broadcast, error) {
			if statusAfterBatch == "" {
				return nil, errors.New("pq: connection refused")
			}
			return &domain.Broadcast{ID: broadcastID, Status: statusAfterBatch}, nil
		}).Times(1)
	// The cancel committed while the batch was queued: it marked the task failed, so the run's
	// save finds it no longer running.
	taskRepo.EXPECT().SaveState(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(context.Context, string, string, float64, *domain.TaskState) error {
			batchQueued = true
			cancelRun()
			return domain.ErrTaskNotRunning
		})

	task := &domain.Task{
		ID: "task-123", WorkspaceID: "workspace-123", BroadcastID: &broadcastID, MaxRetries: 3,
		State: &domain.TaskState{SendBroadcast: &domain.SendBroadcastState{
			BroadcastID: broadcastID, TotalRecipients: 30, Phase: "single", ChannelType: "email",
		}},
	}
	done, err := o.Process(runCtx, task, deadline)

	assert.False(t, done)
	assert.True(t, errors.Is(err, domain.ErrTaskNotRunning), "got %v", err)
}
