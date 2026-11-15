package service

import (
	"context"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newRunsTestService(t *testing.T) (*TaskService, *mocks.MockTaskRepository) {
	t.Helper()
	ctrl := gomock.NewController(t)
	repo := mocks.NewMockTaskRepository(ctrl)
	log := pkgmocks.NewMockLogger(ctrl)
	log.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(log).AnyTimes()
	log.EXPECT().WithFields(gomock.Any()).Return(log).AnyTimes()
	log.EXPECT().Debug(gomock.Any()).AnyTimes()
	log.EXPECT().Info(gomock.Any()).AnyTimes()
	log.EXPECT().Warn(gomock.Any()).AnyTimes()
	log.EXPECT().Error(gomock.Any()).AnyTimes()
	return NewTaskService(repo, mocks.NewMockSettingRepository(ctrl), log, nil, "http://localhost:8080"), repo
}

// Runs started outside the task scheduler ("send now", a cron call) stop with the app and are
// waited for: cut at the exit instead, a broadcast run could lose the cursor of a batch it had
// just queued, and the next run would queue that batch again.
func TestTaskService_RunsOutsideTheScheduler(t *testing.T) {
	t.Run("a run's context is cancelled by the stop, not by its caller", func(t *testing.T) {
		svc, _ := newRunsTestService(t)
		stopCtx, stop := context.WithCancel(context.Background())
		svc.SetShutdownContext(stopCtx)

		caller, callerGone := context.WithCancel(context.Background())
		ctx, done, ok := svc.BeginRun(caller)
		require.True(t, ok)
		defer done()
		callerGone()
		assert.NoError(t, ctx.Err(), "a caller that gives up does not cut the run")
		stop()
		select {
		case <-ctx.Done():
		case <-time.After(time.Second):
			t.Fatal("the stop did not cancel the run")
		}
		assert.ErrorIs(t, ctx.Err(), context.Canceled)
	})

	t.Run("a run's end leaves its context to the work it handed off", func(t *testing.T) {
		svc, _ := newRunsTestService(t)
		stopCtx, stop := context.WithCancel(context.Background())
		defer stop()
		svc.SetShutdownContext(stopCtx)

		ctx, done, ok := svc.BeginRun(context.Background())
		require.True(t, ok)
		done()
		assert.NoError(t, ctx.Err(), "the HTTP dispatches a cron run started outlive it")
		assert.True(t, svc.WaitRuns(time.Millisecond), "an ended run is not waited for")
	})

	t.Run("the stop waits for a run in flight", func(t *testing.T) {
		svc, _ := newRunsTestService(t)
		stopCtx, stop := context.WithCancel(context.Background())
		svc.SetShutdownContext(stopCtx)
		_, done, ok := svc.BeginRun(context.Background())
		require.True(t, ok)
		stop()
		go func() {
			time.Sleep(50 * time.Millisecond)
			done()
		}()
		start := time.Now()
		assert.True(t, svc.WaitRuns(2*time.Second))
		assert.GreaterOrEqual(t, time.Since(start), 50*time.Millisecond)
	})

	t.Run("the wait ends at its timeout when a run outlasts it", func(t *testing.T) {
		svc, _ := newRunsTestService(t)
		_, done, ok := svc.BeginRun(context.Background())
		require.True(t, ok)
		defer done()
		start := time.Now()
		assert.False(t, svc.WaitRuns(50*time.Millisecond))
		assert.Less(t, time.Since(start), time.Second)
	})

	t.Run("with no run the wait returns at once", func(t *testing.T) {
		svc, _ := newRunsTestService(t)
		assert.True(t, svc.WaitRuns(time.Second))
	})

	t.Run("no run starts once the stop began", func(t *testing.T) {
		svc, _ := newRunsTestService(t) // the repository fails the test if a run reads the task
		stopCtx, stop := context.WithCancel(context.Background())
		svc.SetShutdownContext(stopCtx)
		stop()
		_, _, ok := svc.BeginRun(context.Background())
		assert.False(t, ok)

		svc.SetAutoExecuteImmediate(true)
		svc.RunTaskNow("ws-1", "task-1")
		assert.True(t, svc.WaitRuns(time.Second))
	})

	t.Run("a run asked for now is waited for by the stop", func(t *testing.T) {
		svc, repo := newRunsTestService(t)
		stopCtx, stop := context.WithCancel(context.Background())
		svc.SetShutdownContext(stopCtx)
		svc.SetAutoExecuteImmediate(true)
		reading := make(chan struct{})
		repo.EXPECT().Get(gomock.Any(), "ws-1", "task-1").
			DoAndReturn(func(ctx context.Context, _, _ string) (*domain.Task, error) {
				close(reading)
				<-ctx.Done() // the stop cancels the run
				time.Sleep(50 * time.Millisecond)
				return nil, ctx.Err()
			})

		svc.RunTaskNow("ws-1", "task-1")
		<-reading
		stop()
		start := time.Now()
		assert.True(t, svc.WaitRuns(2*time.Second))
		assert.GreaterOrEqual(t, time.Since(start), 50*time.Millisecond)
	})
}
