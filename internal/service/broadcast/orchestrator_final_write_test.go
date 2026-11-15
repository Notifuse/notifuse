package broadcast_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	domainmocks "github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/internal/service/broadcast"
)

// expectFinalWrite expects the end of a run that queued every recipient: the broadcast's
// row read with its lock (locked is that row), then written with UpdateBroadcastTx. write
// gets the broadcast written.
func expectFinalWrite(repo *domainmocks.MockBroadcastRepository, workspaceID string, locked *domain.Broadcast, write func(context.Context, *domain.Broadcast) error) {
	repo.EXPECT().WithTransaction(gomock.Any(), workspaceID, gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, fn func(*sql.Tx) error) error { return fn(nil) })
	repo.EXPECT().GetBroadcastForUpdateTx(gomock.Any(), gomock.Any(), workspaceID, locked.ID).Return(locked, nil)
	repo.EXPECT().UpdateBroadcastTx(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, _ *sql.Tx, b *domain.Broadcast) error { return write(ctx, b) })
}

// finalWriteRun is a single-template run over abAudience that queues every recipient, so
// it reaches the final write. The run's own copy of the broadcast is "Autumn sale".
type finalWriteRun struct {
	m     *sendingStartedMocks
	queue *domainmocks.MockEmailQueueRepository
}

func newFinalWriteRun(t *testing.T) *finalWriteRun {
	m := newSendingStartedMocks(t)
	m.expectConfiguredWorkspace()
	m.expectAudience()
	m.broadcastRepo.EXPECT().MarkBroadcastStarted(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	m.eventBus.EXPECT().Publish(gomock.Any(), gomock.Any()).AnyTimes()
	m.broadcastRepo.EXPECT().GetBroadcast(gomock.Any(), "workspace-123", "broadcast-123").
		DoAndReturn(func(context.Context, string, string) (*domain.Broadcast, error) {
			return singleTemplateBroadcast(domain.BroadcastStatusProcessing), nil
		}).
		AnyTimes()
	m.templateRepo.EXPECT().GetTemplateByID(gomock.Any(), "workspace-123", "template-A", int64(0)).
		Return(abTemplate("template-A", "Variation A"), nil).AnyTimes()
	m.taskRepo.EXPECT().SaveState(gomock.Any(), "workspace-123", "task-123", gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	var batches []sentBatch
	m.recordBatches(&batches, nil)
	return &finalWriteRun{m: m, queue: domainmocks.NewMockEmailQueueRepository(m.ctrl)}
}

func singleTemplateBroadcast(status domain.BroadcastStatus) *domain.Broadcast {
	b := abBroadcast(status, nil)
	b.TestSettings = domain.BroadcastTestSettings{
		Variations: []domain.BroadcastVariation{{VariationName: "A", TemplateID: "template-A"}},
	}
	return b
}

func (r *finalWriteRun) process(ctx context.Context, state *domain.SendBroadcastState) (*domain.Task, bool, error) {
	return r.processTask(ctx, abTask(state))
}

func (r *finalWriteRun) processTask(ctx context.Context, task *domain.Task) (*domain.Task, bool, error) {
	m := r.m
	config := &broadcast.Config{FetchBatchSize: 50, MaxProcessTime: 30 * time.Second, ProgressLogInterval: 5 * time.Second}
	o := broadcast.NewBroadcastOrchestrator(m.messageSender, m.broadcastRepo, m.templateRepo, m.contactRepo,
		m.taskRepo, m.workspaceRepo, r.queue, nil, m.logger, config, m.timeProvider, "https://api.example.com", m.eventBus)
	done, err := o.Process(ctx, task, time.Now().Add(30*time.Second))
	return task, done, err
}

func TestProcess_FinalWriteKeepsWhatWasCommittedMeanwhile(t *testing.T) {
	t.Run("the final fields are set on the locked row, not on the run's copy", func(t *testing.T) {
		r := newFinalWriteRun(t)
		locked := singleTemplateBroadcast(domain.BroadcastStatusProcessing)
		locked.Name = "Renamed meanwhile"
		var written *domain.Broadcast
		expectFinalWrite(r.m.broadcastRepo, "workspace-123", locked, func(_ context.Context, b *domain.Broadcast) error {
			written = b
			return nil
		})

		_, done, err := r.process(context.Background(), &domain.SendBroadcastState{})
		require.NoError(t, err)
		assert.True(t, done)
		require.NotNil(t, written)
		assert.Equal(t, "Renamed meanwhile", written.Name, "a column another writer changed is not written back")
		assert.Equal(t, domain.BroadcastStatusProcessed, written.Status)
		assert.NotNil(t, written.CompletedAt)
		assert.Equal(t, len(abAudience), written.EnqueuedCount)
	})

	// The last rows are settled under the broadcast's lock: settled after the commit, a
	// resume committed in between had its rows paused again.
	inLockedTx := func(r *finalWriteRun) *bool {
		locked := new(bool)
		r.m.broadcastRepo.EXPECT().WithTransaction(gomock.Any(), "workspace-123", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, fn func(*sql.Tx) error) error {
				*locked = true
				defer func() { *locked = false }()
				return fn(nil)
			})
		return locked
	}

	t.Run("a pause committed meanwhile parks the task and pauses the last rows", func(t *testing.T) {
		r := newFinalWriteRun(t)
		locked := inLockedTx(r)
		r.m.broadcastRepo.EXPECT().GetBroadcastForUpdateTx(gomock.Any(), gomock.Any(), "workspace-123", "broadcast-123").
			Return(singleTemplateBroadcast(domain.BroadcastStatusPaused), nil)
		r.m.broadcastRepo.EXPECT().UpdateBroadcastTx(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
		r.queue.EXPECT().PauseBySourceTx(gomock.Any(), gomock.Any(), domain.EmailQueueSourceBroadcast, "broadcast-123").
			DoAndReturn(func(context.Context, *sql.Tx, domain.EmailQueueSourceType, string) (int64, error) {
				assert.True(t, *locked, "the rows are paused inside the locked transaction")
				return 2, nil
			})

		_, done, err := r.process(context.Background(), &domain.SendBroadcastState{})
		assert.False(t, done)
		var requeue *domain.TaskRequeueError
		require.True(t, errors.As(err, &requeue), "got %v", err)
		assert.True(t, requeue.Park)
	})

	t.Run("a cancel committed meanwhile completes the task and removes the last rows", func(t *testing.T) {
		r := newFinalWriteRun(t)
		locked := inLockedTx(r)
		r.m.broadcastRepo.EXPECT().GetBroadcastForUpdateTx(gomock.Any(), gomock.Any(), "workspace-123", "broadcast-123").
			Return(singleTemplateBroadcast(domain.BroadcastStatusCancelled), nil)
		r.m.broadcastRepo.EXPECT().UpdateBroadcastTx(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
		r.queue.EXPECT().DeleteBySourceTx(gomock.Any(), gomock.Any(), domain.EmailQueueSourceBroadcast, "broadcast-123").
			DoAndReturn(func(context.Context, *sql.Tx, domain.EmailQueueSourceType, string) (int64, error) {
				assert.True(t, *locked, "the rows are removed inside the locked transaction")
				return 2, nil
			})

		_, done, err := r.process(context.Background(), &domain.SendBroadcastState{})
		require.NoError(t, err)
		assert.True(t, done)
	})

	t.Run("a failed pause of the last rows fails the final write", func(t *testing.T) {
		r := newFinalWriteRun(t)
		inLockedTx(r)
		r.m.broadcastRepo.EXPECT().GetBroadcastForUpdateTx(gomock.Any(), gomock.Any(), "workspace-123", "broadcast-123").
			Return(singleTemplateBroadcast(domain.BroadcastStatusPaused), nil)
		r.queue.EXPECT().PauseBySourceTx(gomock.Any(), gomock.Any(), domain.EmailQueueSourceBroadcast, "broadcast-123").
			Return(int64(0), errors.New("connection reset"))

		_, done, err := r.process(context.Background(), &domain.SendBroadcastState{})
		assert.False(t, done)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "connection reset")
	})

	t.Run("a broadcast already processed is left as it is", func(t *testing.T) {
		r := newFinalWriteRun(t)
		r.m.broadcastRepo.EXPECT().WithTransaction(gomock.Any(), "workspace-123", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, fn func(*sql.Tx) error) error { return fn(nil) })
		r.m.broadcastRepo.EXPECT().GetBroadcastForUpdateTx(gomock.Any(), gomock.Any(), "workspace-123", "broadcast-123").
			Return(singleTemplateBroadcast(domain.BroadcastStatusProcessed), nil)
		r.m.broadcastRepo.EXPECT().UpdateBroadcastTx(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

		_, done, err := r.process(context.Background(), &domain.SendBroadcastState{})
		require.NoError(t, err)
		assert.True(t, done)
	})

	t.Run("a failed final transaction is returned", func(t *testing.T) {
		r := newFinalWriteRun(t)
		dbErr := errors.New("connection reset")
		r.m.broadcastRepo.EXPECT().WithTransaction(gomock.Any(), "workspace-123", gomock.Any()).Return(dbErr)

		_, done, err := r.process(context.Background(), &domain.SendBroadcastState{})
		assert.False(t, done)
		assert.ErrorIs(t, err, dbErr)
		assert.EqualError(t, err, "failed to write the end of the run: connection reset")
	})

	t.Run("a shutdown's cancellation does not lose the final write, bounded by the lease", func(t *testing.T) {
		r := newFinalWriteRun(t)
		lease := time.Now().Add(time.Minute)
		var writeCtx context.Context
		expectFinalWrite(r.m.broadcastRepo, "workspace-123", singleTemplateBroadcast(domain.BroadcastStatusProcessing), func(ctx context.Context, _ *domain.Broadcast) error {
			writeCtx = ctx
			assert.NoError(t, ctx.Err())
			return nil
		})
		ctx, cancel := context.WithDeadline(context.Background(), lease)
		defer cancel()

		_, done, err := r.process(ctx, &domain.SendBroadcastState{})
		require.NoError(t, err)
		assert.True(t, done)
		require.NotNil(t, writeCtx)
		deadline, ok := writeCtx.Deadline()
		assert.True(t, ok && deadline.Equal(lease), "the write never runs past the lease")
	})
}

func TestProcess_NoRecipientWriteKeepsWhatWasCommittedMeanwhile(t *testing.T) {
	newRun := func(t *testing.T) *finalWriteRun {
		m := newSendingStartedMocks(t)
		m.broadcastRepo.EXPECT().GetBroadcast(gomock.Any(), "workspace-123", "broadcast-123").
			Return(singleTemplateBroadcast(domain.BroadcastStatusProcessing), nil).AnyTimes()
		m.contactRepo.EXPECT().CountContactsForBroadcast(gomock.Any(), "workspace-123", gomock.Any()).Return(0, nil)
		return &finalWriteRun{m: m, queue: domainmocks.NewMockEmailQueueRepository(m.ctrl)}
	}
	noRecipients := func(r *finalWriteRun) (bool, error) {
		// The counting run: no recipient counted yet.
		_, done, err := r.processTask(context.Background(), sendingStartedTask(&domain.SendBroadcastState{BroadcastID: "broadcast-123"}))
		return done, err
	}

	t.Run("processed is written on the locked row", func(t *testing.T) {
		r := newRun(t)
		locked := singleTemplateBroadcast(domain.BroadcastStatusProcessing)
		locked.Name = "Renamed meanwhile"
		var written *domain.Broadcast
		expectFinalWrite(r.m.broadcastRepo, "workspace-123", locked, func(_ context.Context, b *domain.Broadcast) error {
			written = b
			return nil
		})

		done, err := noRecipients(r)
		require.NoError(t, err)
		assert.True(t, done)
		require.NotNil(t, written)
		assert.Equal(t, "Renamed meanwhile", written.Name)
		assert.Equal(t, domain.BroadcastStatusProcessed, written.Status)
		assert.NotNil(t, written.CompletedAt)
		assert.Zero(t, written.EnqueuedCount)
	})

	t.Run("a cancel committed meanwhile is kept", func(t *testing.T) {
		r := newRun(t)
		r.m.broadcastRepo.EXPECT().WithTransaction(gomock.Any(), "workspace-123", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, fn func(*sql.Tx) error) error { return fn(nil) })
		r.m.broadcastRepo.EXPECT().GetBroadcastForUpdateTx(gomock.Any(), gomock.Any(), "workspace-123", "broadcast-123").
			Return(singleTemplateBroadcast(domain.BroadcastStatusCancelled), nil)
		r.m.broadcastRepo.EXPECT().UpdateBroadcastTx(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
		r.queue.EXPECT().DeleteBySourceTx(gomock.Any(), gomock.Any(), domain.EmailQueueSourceBroadcast, "broadcast-123").Return(int64(0), nil)

		done, err := noRecipients(r)
		require.NoError(t, err)
		assert.True(t, done)
	})
}
