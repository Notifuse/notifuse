package service

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
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
)

// requeueHarness runs ExecuteTask on one send_broadcast task whose processor returns what
// the test gives it. The repository mock is strict: a write the test does not expect fails it.
type requeueHarness struct {
	svc  *TaskService
	repo *mocks.MockTaskRepository
	task *domain.Task
}

func newRequeueHarness(t *testing.T, processErr error) *requeueHarness {
	t.Helper()
	ctrl := gomock.NewController(t)
	repo := mocks.NewMockTaskRepository(ctrl)
	logger := pkgmocks.NewMockLogger(ctrl)
	logger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(logger).AnyTimes()
	logger.EXPECT().WithFields(gomock.Any()).Return(logger).AnyTimes()
	logger.EXPECT().Debug(gomock.Any()).AnyTimes()
	logger.EXPECT().Info(gomock.Any()).AnyTimes()
	logger.EXPECT().Warn(gomock.Any()).AnyTimes()
	// A requeue is not a failure: nothing is logged at Error.
	logger.EXPECT().Error(gomock.Any()).Times(0)

	svc := NewTaskService(repo, mocks.NewMockSettingRepository(ctrl), logger, nil, "http://localhost:8080")
	svc.SetAutoExecuteImmediate(false)

	task := &domain.Task{
		ID:          "task-1",
		WorkspaceID: "ws-1",
		Type:        "send_broadcast",
		Status:      domain.TaskStatusPending,
		MaxRuntime:  60,
		RetryCount:  2,
		MaxRetries:  3,
	}
	processor := mocks.NewMockTaskProcessor(ctrl)
	for _, taskType := range getTaskTypes() {
		processor.EXPECT().CanProcess(taskType).Return(taskType == "send_broadcast").AnyTimes()
	}
	svc.RegisterProcessor(processor)

	repo.EXPECT().WithTransaction(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, fn func(*sql.Tx) error) error { return fn(nil) })
	repo.EXPECT().GetTx(gomock.Any(), gomock.Any(), "ws-1", "task-1").Return(task, nil)
	repo.EXPECT().MarkAsRunningTx(gomock.Any(), gomock.Any(), "ws-1", "task-1", gomock.Any()).Return(nil)
	processor.EXPECT().Process(gomock.Any(), task, gomock.Any()).DoAndReturn(
		func(_ context.Context, task *domain.Task, _ time.Time) (bool, error) {
			task.Progress = 40
			task.State = &domain.TaskState{SendBroadcast: &domain.SendBroadcastState{LastProcessedEmail: "cursor@example.com"}}
			return false, processErr
		})
	return &requeueHarness{svc: svc, repo: repo, task: task}
}

// nextRunAbout matches a next run about d from now.
func nextRunAbout(d time.Duration) gomock.Matcher {
	return &timeNear{want: time.Now().Add(d)}
}

type timeNear struct{ want time.Time }

func (m *timeNear) Matches(x interface{}) bool {
	got, ok := x.(time.Time)
	if !ok {
		return false
	}
	diff := got.Sub(m.want)
	return diff > -5*time.Second && diff < 5*time.Second
}

func (m *timeNear) String() string { return fmt.Sprintf("about %s", m.want.Format(time.RFC3339)) }

func TestTaskService_ExecuteTask_Requeue(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		status domain.TaskStatus
		after  time.Duration
	}{
		{"a requeue hands the task back as pending", &domain.TaskRequeueError{After: time.Minute, Reason: "draft"}, domain.TaskStatusPending, time.Minute},
		{"a parked requeue hands it back as paused", &domain.TaskRequeueError{After: 24 * time.Hour, Park: true, Reason: "paused"}, domain.TaskStatusPaused, 24 * time.Hour},
		{"a wrapped requeue is read too", fmt.Errorf("broadcast: %w", &domain.TaskRequeueError{After: time.Minute}), domain.TaskStatusPending, time.Minute},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newRequeueHarness(t, tc.err)
			// RequeueTask keeps the retry count: no MarkAsPending (which resets it), no MarkAsFailed.
			h.repo.EXPECT().RequeueTask(gomock.Any(), "ws-1", "task-1", tc.status, nextRunAbout(tc.after), 40.0, gomock.Any()).
				DoAndReturn(func(_ context.Context, _, _ string, _ domain.TaskStatus, _ time.Time, _ float64, state *domain.TaskState) error {
					assert.Equal(t, "cursor@example.com", state.SendBroadcast.LastProcessedEmail, "the run's state is kept")
					return nil
				})

			assert.NoError(t, h.svc.ExecuteTask(context.Background(), "ws-1", "task-1", time.Now().Add(time.Minute)))
		})
	}

	t.Run("AfterRequeue runs once the requeue is written", func(t *testing.T) {
		var order []string
		h := newRequeueHarness(t, &domain.TaskRequeueError{After: time.Hour, Park: true, AfterRequeue: func(ctx context.Context) {
			order = append(order, "after")
			assert.NoError(t, ctx.Err(), "it runs on a context the run's deadline does not cut")
		}})
		h.repo.EXPECT().RequeueTask(gomock.Any(), "ws-1", "task-1", domain.TaskStatusPaused, gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(context.Context, string, string, domain.TaskStatus, time.Time, float64, *domain.TaskState) error {
				order = append(order, "requeue")
				return nil
			})

		require.NoError(t, h.svc.ExecuteTask(context.Background(), "ws-1", "task-1", time.Now().Add(time.Minute)))
		assert.Equal(t, []string{"requeue", "after"}, order)
	})

	t.Run("a task no longer running is left as another decision put it", func(t *testing.T) {
		called := false
		h := newRequeueHarness(t, &domain.TaskRequeueError{After: time.Minute, AfterRequeue: func(context.Context) { called = true }})
		h.repo.EXPECT().RequeueTask(gomock.Any(), "ws-1", "task-1", domain.TaskStatusPending, gomock.Any(), gomock.Any(), gomock.Any()).
			Return(domain.ErrTaskNotRunning)

		assert.NoError(t, h.svc.ExecuteTask(context.Background(), "ws-1", "task-1", time.Now().Add(time.Minute)))
		assert.False(t, called, "nothing follows a requeue that did not happen")
	})

	t.Run("a failed requeue write is returned", func(t *testing.T) {
		h := newRequeueHarness(t, &domain.TaskRequeueError{After: time.Minute})
		// This failure is logged at Error; the harness forbids that, so allow it here.
		h.svc.logger = loggerAllowingErrors(t)
		dbErr := errors.New("connection reset")
		h.repo.EXPECT().RequeueTask(gomock.Any(), "ws-1", "task-1", domain.TaskStatusPending, gomock.Any(), gomock.Any(), gomock.Any()).
			Return(dbErr)

		err := h.svc.ExecuteTask(context.Background(), "ws-1", "task-1", time.Now().Add(time.Minute))
		assert.ErrorIs(t, err, dbErr)
	})
}

func loggerAllowingErrors(t *testing.T) *pkgmocks.MockLogger {
	ctrl := gomock.NewController(t)
	logger := pkgmocks.NewMockLogger(ctrl)
	logger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(logger).AnyTimes()
	logger.EXPECT().WithFields(gomock.Any()).Return(logger).AnyTimes()
	logger.EXPECT().Debug(gomock.Any()).AnyTimes()
	logger.EXPECT().Info(gomock.Any()).AnyTimes()
	logger.EXPECT().Warn(gomock.Any()).AnyTimes()
	logger.EXPECT().Error(gomock.Any()).AnyTimes()
	return logger
}

// The run's context carries its lease, the value it stored; a processor that found its lease
// gone stops the run without any write.
func TestTaskService_ExecuteTask_Lease(t *testing.T) {
	t.Run("the processor's context carries the stored lease", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		repo := mocks.NewMockTaskRepository(ctrl)
		svc := NewTaskService(repo, mocks.NewMockSettingRepository(ctrl), loggerAllowingErrors(t), nil, "http://localhost:8080")
		svc.SetAutoExecuteImmediate(false)
		task := &domain.Task{ID: "task-1", WorkspaceID: "ws-1", Type: "send_broadcast", MaxRuntime: 60}
		processor := mocks.NewMockTaskProcessor(ctrl)
		for _, taskType := range getTaskTypes() {
			processor.EXPECT().CanProcess(taskType).Return(taskType == "send_broadcast").AnyTimes()
		}
		svc.RegisterProcessor(processor)

		var stored time.Time
		repo.EXPECT().WithTransaction(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, fn func(*sql.Tx) error) error { return fn(nil) })
		repo.EXPECT().GetTx(gomock.Any(), gomock.Any(), "ws-1", "task-1").Return(task, nil)
		repo.EXPECT().MarkAsRunningTx(gomock.Any(), gomock.Any(), "ws-1", "task-1", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ *sql.Tx, _, _ string, lease time.Time) error { stored = lease; return nil })
		processor.EXPECT().Process(gomock.Any(), task, gomock.Any()).DoAndReturn(
			func(ctx context.Context, _ *domain.Task, _ time.Time) (bool, error) {
				lease, ok := domain.TaskLeaseFrom(ctx)
				require.True(t, ok)
				assert.True(t, lease.Equal(stored), "the run holds the lease it stored")
				assert.Equal(t, lease, lease.Truncate(time.Microsecond), "as the column keeps it")
				deadline, _ := ctx.Deadline()
				assert.False(t, deadline.After(lease), "the run ends at its lease")
				return true, nil
			})
		repo.EXPECT().MarkAsCompleted(gomock.Any(), "ws-1", "task-1", gomock.Any()).Return(nil)

		require.NoError(t, svc.ExecuteTask(context.Background(), "ws-1", "task-1", time.Now().Add(time.Minute)))
	})

	t.Run("a processor that lost its lease stops without any write", func(t *testing.T) {
		h := newRequeueHarness(t, fmt.Errorf("save progress: %w", domain.ErrTaskNotRunning))
		// The strict repository mock fails on any MarkAs*, ReleaseTask or RequeueTask call.
		assert.NoError(t, h.svc.ExecuteTask(context.Background(), "ws-1", "task-1", time.Now().Add(time.Minute)))
	})
}
