package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/internal/service"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeRunStarter stands in for the task service's run tracking: its runs are cancelled by stop,
// and it counts the runs not yet done.
type fakeRunStarter struct {
	mu       sync.Mutex
	stopCtx  context.Context
	inFlight int
	refuse   bool
}

func (f *fakeRunStarter) BeginRun(parent context.Context) (context.Context, func(), bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.refuse {
		return nil, nil, false
	}
	f.inFlight++
	ctx, cancel := context.WithCancel(context.WithoutCancel(parent))
	stop := context.AfterFunc(f.stopCtx, cancel)
	return ctx, func() {
		stop() // like BeginRun, done leaves the context to what the run handed off
		f.mu.Lock()
		defer f.mu.Unlock()
		f.inFlight--
	}, true
}

func (f *fakeRunStarter) running() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.inFlight
}

// The cron's background run is a run the app's stop cancels and waits for: the HTTP shutdown
// does not wait for it, since the handler has already answered.
func TestTaskHandler_ExecutePendingTasks_StopsWithTheApp(t *testing.T) {
	newHandler := func(t *testing.T) (*TaskHandler, *mocks.MockTaskService) {
		ctrl := gomock.NewController(t)
		svc := mocks.NewMockTaskService(ctrl)
		log := pkgmocks.NewMockLogger(ctrl)
		log.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(log).AnyTimes()
		log.EXPECT().WithFields(gomock.Any()).Return(log).AnyTimes()
		log.EXPECT().Info(gomock.Any()).AnyTimes()
		log.EXPECT().Warn(gomock.Any()).AnyTimes()
		log.EXPECT().Error(gomock.Any()).AnyTimes()
		return NewTaskHandler(svc, mocks.NewMockAuthService(ctrl), func() ([]byte, error) { return nil, nil }, log, "secret", true), svc
	}

	t.Run("the run is tracked until it ends, and cancelled by the stop", func(t *testing.T) {
		handler, svc := newHandler(t)
		stopCtx, stop := context.WithCancel(context.Background())
		runs := &fakeRunStarter{stopCtx: stopCtx}
		handler.SetRunStarter(runs)

		running := make(chan struct{})
		svc.EXPECT().ExecutePendingTasks(gomock.Any(), 10).DoAndReturn(func(ctx context.Context, _ int) error {
			close(running)
			<-ctx.Done()
			return ctx.Err()
		})

		reqCtx, hangUp := context.WithCancel(context.Background())
		rec := httptest.NewRecorder()
		handler.ExecutePendingTasks(rec, httptest.NewRequest(http.MethodGet, "/api/cron?max_tasks=10", nil).WithContext(reqCtx))
		assert.Equal(t, http.StatusAccepted, rec.Code)
		<-running
		hangUp()
		time.Sleep(20 * time.Millisecond)
		assert.Equal(t, 1, runs.running(), "the caller hanging up does not end the run")

		stop()
		require.Eventually(t, func() bool { return runs.running() == 0 }, 2*time.Second, 5*time.Millisecond)
		require.Eventually(t, func() bool { return !handler.cronRunning.Load() }, 2*time.Second, 5*time.Millisecond)
	})

	t.Run("once the stop began no run starts", func(t *testing.T) {
		handler, _ := newHandler(t) // ExecutePendingTasks is not expected
		handler.SetRunStarter(&fakeRunStarter{stopCtx: context.Background(), refuse: true})

		rec := httptest.NewRecorder()
		handler.ExecutePendingTasks(rec, httptest.NewRequest(http.MethodGet, "/api/cron?max_tasks=10", nil))
		assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
		assert.False(t, handler.cronRunning.Load(), "a refused run leaves the next call free")
	})
}

// A dispatched execution runs inside its request: the stop cancels it, and the run is tracked
// until the handler returns.
func TestTaskHandler_ExecuteTask_StopsWithTheApp(t *testing.T) {
	ctrl := gomock.NewController(t)
	svc := mocks.NewMockTaskService(ctrl)
	log := pkgmocks.NewMockLogger(ctrl)
	log.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(log).AnyTimes()
	log.EXPECT().WithFields(gomock.Any()).Return(log).AnyTimes()
	log.EXPECT().Info(gomock.Any()).AnyTimes()
	log.EXPECT().Warn(gomock.Any()).AnyTimes()
	log.EXPECT().Error(gomock.Any()).AnyTimes()
	log.EXPECT().Debug(gomock.Any()).AnyTimes()
	const secret = "secret"
	handler := NewTaskHandler(svc, mocks.NewMockAuthService(ctrl), func() ([]byte, error) { return nil, nil }, log, secret, true)
	stopCtx, stop := context.WithCancel(context.Background())
	runs := &fakeRunStarter{stopCtx: stopCtx}
	handler.SetRunStarter(runs)

	const taskID = "12312312-3123-4123-8123-123123123123"
	svc.EXPECT().GetTask(gomock.Any(), "ws1", taskID).Return(&domain.Task{ID: taskID, WorkspaceID: "ws1", MaxRuntime: 50}, nil)
	running := make(chan struct{})
	svc.EXPECT().ExecuteTask(gomock.Any(), "ws1", taskID, gomock.Any()).
		DoAndReturn(func(ctx context.Context, _, _ string, _ time.Time) error {
			close(running)
			<-ctx.Done()
			return ctx.Err()
		})

	body := []byte(`{"workspace_id":"ws1","id":"` + taskID + `"}`)
	done := make(chan struct{})
	go func() {
		defer close(done)
		handler.ExecuteTask(httptest.NewRecorder(), newSignedExecuteRequest(secret, body))
	}()
	<-running
	assert.Equal(t, 1, runs.running())
	stop()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the stop did not end the execution")
	}
	assert.Zero(t, runs.running())
}

// With the internal scheduler off, /api/cron is what runs tasks: it dispatches each one to
// tasks.execute over HTTP and returns before the dispatches answer. Those dispatches must not end
// with the cron run.
func TestExecutePendingTasks_HTTPDispatchesOutliveTheCronRun(t *testing.T) {
	const due = 8
	var received atomic.Int32
	allReceived := make(chan struct{})
	executeEndpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if received.Add(1) == due {
			close(allReceived)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer executeEndpoint.Close()

	ctrl := gomock.NewController(t)
	repo := mocks.NewMockTaskRepository(ctrl)
	settings := mocks.NewMockSettingRepository(ctrl)
	logger := pkgmocks.NewMockLogger(ctrl)
	logger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(logger).AnyTimes()
	logger.EXPECT().WithFields(gomock.Any()).Return(logger).AnyTimes()
	logger.EXPECT().Debug(gomock.Any()).AnyTimes()
	logger.EXPECT().Info(gomock.Any()).AnyTimes()
	logger.EXPECT().Warn(gomock.Any()).AnyTimes()
	logger.EXPECT().Error(gomock.Any()).AnyTimes()

	svc := service.NewTaskService(repo, settings, logger, nil, executeEndpoint.URL)
	svc.SetDirectExecution(false)
	stopCtx, stop := context.WithCancel(context.Background())
	defer stop()
	svc.SetShutdownContext(stopCtx)

	tasks := make([]*domain.Task, 0, due)
	for i := 0; i < due; i++ {
		tasks = append(tasks, &domain.Task{ID: string(rune('a' + i)), WorkspaceID: "ws1", Type: "send_broadcast", MaxRuntime: 50})
	}
	settings.EXPECT().SetLastCronRun(gomock.Any()).Return(nil).AnyTimes()
	repo.EXPECT().GetNextBatch(gomock.Any(), gomock.Any()).Return(tasks, nil)

	handler := NewTaskHandler(svc, mocks.NewMockAuthService(ctrl), func() ([]byte, error) { return nil, nil }, logger, "secret", true)
	handler.SetRunStarter(svc)
	rec := httptest.NewRecorder()
	handler.ExecutePendingTasks(rec, httptest.NewRequest(http.MethodGet, "/api/cron?max_tasks=10", nil))
	require.Equal(t, http.StatusAccepted, rec.Code)

	select {
	case <-allReceived:
	case <-time.After(5 * time.Second):
		t.Fatalf("tasks.execute received %d of the %d dispatches", received.Load(), due)
	}
	assert.True(t, svc.WaitRuns(2*time.Second), "the cron run ended")
}
