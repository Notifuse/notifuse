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

// The schedule request writes the broadcast's task itself, in one system-database
// transaction, and returns its error: a task that cannot be written fails the request.
func TestTaskService_ScheduleBroadcastTask(t *testing.T) {
	setup := func(t *testing.T) (*TaskService, *mocks.MockTaskRepository) {
		ctrl := gomock.NewController(t)
		repo := mocks.NewMockTaskRepository(ctrl)
		svc := NewTaskService(repo, mocks.NewMockSettingRepository(ctrl), loggerAllowingErrors(t), nil, "http://localhost:8080")
		repo.EXPECT().WithTransaction(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, fn func(*sql.Tx) error) error { return fn(nil) })
		return svc, repo
	}
	notFound := fmt.Errorf("%w for broadcast ID b1", domain.ErrTaskNotFound)

	t.Run("a send-now broadcast gets a task due shortly, which RunTaskNow starts at once", func(t *testing.T) {
		svc, repo := setup(t)
		repo.EXPECT().GetTaskByBroadcastIDTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(nil, notFound)
		var created *domain.Task
		repo.EXPECT().CreateTx(gomock.Any(), gomock.Any(), "w1", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ *sql.Tx, _ string, task *domain.Task) error {
				created = task
				task.ID = "task-new"
				return nil
			})

		id, err := svc.ScheduleBroadcastTask(context.Background(), "w1", "b1", true, nil)
		require.NoError(t, err)
		assert.Equal(t, "task-new", id)
		require.NotNil(t, created)
		assert.Equal(t, "send_broadcast", created.Type)
		assert.Equal(t, domain.TaskStatusPending, created.Status)
		require.NotNil(t, created.BroadcastID)
		assert.Equal(t, "b1", *created.BroadcastID)
		require.NotNil(t, created.State.SendBroadcast)
		assert.Equal(t, "b1", created.State.SendBroadcast.BroadcastID)
		require.NotNil(t, created.NextRunAfter)
		assert.True(t, (&timeNear{want: time.Now().Add(sendNowTaskDelay)}).Matches(*created.NextRunAfter),
			"due shortly: not before the broadcast has committed")
	})

	// The send-now delay: 5s by default, shorter when the instance configures it. The
	// due time is bracketed by the clock read before and after the call, so the check
	// is exact whatever the machine's speed.
	assertSendNowDue := func(t *testing.T, configure func(*TaskService), want time.Duration) {
		t.Helper()
		svc, repo := setup(t)
		configure(svc)
		repo.EXPECT().GetTaskByBroadcastIDTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(nil, notFound)
		var created *domain.Task
		repo.EXPECT().CreateTx(gomock.Any(), gomock.Any(), "w1", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ *sql.Tx, _ string, task *domain.Task) error {
				created = task
				task.ID = "task-new"
				return nil
			})
		before := time.Now()
		_, err := svc.ScheduleBroadcastTask(context.Background(), "w1", "b1", true, nil)
		after := time.Now()
		require.NoError(t, err)
		require.NotNil(t, created.NextRunAfter)
		due := *created.NextRunAfter
		assert.False(t, due.Before(before.Add(want)), "due %v, want no earlier than %v", due, before.Add(want))
		assert.False(t, due.After(after.Add(want)), "due %v, want no later than %v", due, after.Add(want))
	}

	t.Run("without configuration a send-now task is due 5s after it is scheduled", func(t *testing.T) {
		assertSendNowDue(t, func(*TaskService) {}, 5*time.Second)
	})

	t.Run("a configured send-now delay replaces the default", func(t *testing.T) {
		assertSendNowDue(t, func(s *TaskService) { s.SetSendNowDelay(time.Second) }, time.Second)
	})

	t.Run("a zero or negative send-now delay keeps the default", func(t *testing.T) {
		for _, d := range []time.Duration{0, -time.Second} {
			assertSendNowDue(t, func(s *TaskService) { s.SetSendNowDelay(d) }, 5*time.Second)
		}
	})

	t.Run("the send-now delay does not move a broadcast scheduled for later", func(t *testing.T) {
		svc, repo := setup(t)
		svc.SetSendNowDelay(time.Second)
		at := time.Date(2030, 1, 2, 9, 30, 0, 0, time.UTC)
		repo.EXPECT().GetTaskByBroadcastIDTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(nil, notFound)
		repo.EXPECT().CreateTx(gomock.Any(), gomock.Any(), "w1", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ *sql.Tx, _ string, task *domain.Task) error {
				require.NotNil(t, task.NextRunAfter)
				assert.True(t, task.NextRunAfter.Equal(at))
				task.ID = "task-later"
				return nil
			})
		_, err := svc.ScheduleBroadcastTask(context.Background(), "w1", "b1", false, &at)
		require.NoError(t, err)
	})

	t.Run("a broadcast scheduled for later gets a task due at its time", func(t *testing.T) {
		svc, repo := setup(t)
		at := time.Date(2030, 1, 2, 9, 30, 0, 0, time.UTC)
		repo.EXPECT().GetTaskByBroadcastIDTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(nil, notFound)
		repo.EXPECT().CreateTx(gomock.Any(), gomock.Any(), "w1", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ *sql.Tx, _ string, task *domain.Task) error {
				require.NotNil(t, task.NextRunAfter)
				assert.True(t, task.NextRunAfter.Equal(at))
				task.ID = "task-later"
				return nil
			})

		id, err := svc.ScheduleBroadcastTask(context.Background(), "w1", "b1", false, &at)
		require.NoError(t, err)
		assert.Equal(t, "task-later", id)
	})

	t.Run("an existing task starts over", func(t *testing.T) {
		svc, repo := setup(t)
		at := time.Date(2030, 1, 2, 9, 30, 0, 0, time.UTC)
		repo.EXPECT().GetTaskByBroadcastIDTx(gomock.Any(), gomock.Any(), "w1", "b1").
			Return(&domain.Task{ID: "task-old", Status: domain.TaskStatusCompleted}, nil)
		repo.EXPECT().ResetBroadcastTaskTx(gomock.Any(), gomock.Any(), "w1", "task-old", gomock.Any(), at).
			DoAndReturn(func(_ context.Context, _ *sql.Tx, _, _ string, state *domain.TaskState, _ time.Time) error {
				require.NotNil(t, state.SendBroadcast)
				assert.Equal(t, "b1", state.SendBroadcast.BroadcastID)
				assert.Zero(t, state.SendBroadcast.TotalRecipients, "a fresh state: the next run counts again")
				return nil
			})

		id, err := svc.ScheduleBroadcastTask(context.Background(), "w1", "b1", false, &at)
		require.NoError(t, err)
		assert.Equal(t, "task-old", id)
	})

	t.Run("a task an execution holds is refused", func(t *testing.T) {
		svc, repo := setup(t)
		repo.EXPECT().GetTaskByBroadcastIDTx(gomock.Any(), gomock.Any(), "w1", "b1").
			Return(&domain.Task{ID: "task-held", Status: domain.TaskStatusRunning}, nil)
		repo.EXPECT().ResetBroadcastTaskTx(gomock.Any(), gomock.Any(), "w1", "task-held", gomock.Any(), gomock.Any()).
			Return(&domain.ErrTaskAlreadyRunning{TaskID: "task-held"})

		_, err := svc.ScheduleBroadcastTask(context.Background(), "w1", "b1", true, nil)
		var running *domain.ErrTaskAlreadyRunning
		assert.True(t, errors.As(err, &running), "got %v", err)
	})

	t.Run("a failed read is not taken for a missing task", func(t *testing.T) {
		svc, repo := setup(t)
		readErr := errors.New("connection reset")
		repo.EXPECT().GetTaskByBroadcastIDTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(nil, readErr)

		_, err := svc.ScheduleBroadcastTask(context.Background(), "w1", "b1", true, nil)
		assert.ErrorIs(t, err, readErr)
	})

	t.Run("a failed create is returned", func(t *testing.T) {
		svc, repo := setup(t)
		createErr := errors.New("disk full")
		repo.EXPECT().GetTaskByBroadcastIDTx(gomock.Any(), gomock.Any(), "w1", "b1").Return(nil, notFound)
		repo.EXPECT().CreateTx(gomock.Any(), gomock.Any(), "w1", gomock.Any()).Return(createErr)

		_, err := svc.ScheduleBroadcastTask(context.Background(), "w1", "b1", true, nil)
		assert.ErrorIs(t, err, createErr)
	})
}

// RunTaskNow runs the task it is given, whatever its next run, and no other due task.
func TestTaskService_RunTaskNow(t *testing.T) {
	t.Run("the given task is executed, by its ID", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		repo := mocks.NewMockTaskRepository(ctrl)
		svc := NewTaskService(repo, mocks.NewMockSettingRepository(ctrl), loggerAllowingErrors(t), nil, "http://localhost:8080")

		later := time.Now().Add(time.Hour)
		task := &domain.Task{ID: "task-1", WorkspaceID: "w1", Type: "send_broadcast", MaxRuntime: 50, NextRunAfter: &later}
		repo.EXPECT().Get(gomock.Any(), "w1", "task-1").Return(task, nil)
		repo.EXPECT().WithTransaction(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, fn func(*sql.Tx) error) error { return fn(nil) })
		// The execution reads the task it was given; the strict mock refuses GetNextBatch.
		repo.EXPECT().GetTx(gomock.Any(), gomock.Any(), "w1", "task-1").Return(nil, domain.ErrTaskNotFound)

		svc.runTaskNow(context.Background(), "w1", "task-1")
	})

	// A task deleted before it runs is logged at Warn, by either read; any other failure at Error.
	t.Run("a missing task is logged at Warn, a failed read at Error", func(t *testing.T) {
		for _, tc := range []struct {
			name     string
			setup    func(repo *mocks.MockTaskRepository)
			expected func(log *pkgmocks.MockLogger)
		}{
			{"gone before its first read", func(repo *mocks.MockTaskRepository) {
				repo.EXPECT().Get(gomock.Any(), "w1", "task-1").Return(nil, domain.ErrTaskNotFound)
			}, func(log *pkgmocks.MockLogger) {
				log.EXPECT().Warn("Failed to read the task to run now: the task no longer exists").Times(1)
			}},
			{"gone when the execution reads it", func(repo *mocks.MockTaskRepository) {
				repo.EXPECT().Get(gomock.Any(), "w1", "task-1").Return(&domain.Task{ID: "task-1", MaxRuntime: 50}, nil)
				repo.EXPECT().WithTransaction(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, fn func(*sql.Tx) error) error { return fn(nil) })
				repo.EXPECT().GetTx(gomock.Any(), gomock.Any(), "w1", "task-1").Return(nil, domain.ErrTaskNotFound)
			}, func(log *pkgmocks.MockLogger) {
				log.EXPECT().Warn("Failed to run the task now: the task no longer exists").Times(1)
			}},
			{"a failed read", func(repo *mocks.MockTaskRepository) {
				repo.EXPECT().Get(gomock.Any(), "w1", "task-1").Return(nil, errors.New("pq: connection refused"))
			}, func(log *pkgmocks.MockLogger) {
				log.EXPECT().Error("Failed to read the task to run now").Times(1)
			}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				ctrl := gomock.NewController(t)
				repo := mocks.NewMockTaskRepository(ctrl)
				log := pkgmocks.NewMockLogger(ctrl)
				log.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(log).AnyTimes()
				log.EXPECT().WithFields(gomock.Any()).Return(log).AnyTimes()
				log.EXPECT().Info(gomock.Any()).AnyTimes()
				log.EXPECT().Debug(gomock.Any()).AnyTimes()
				tc.expected(log)
				svc := NewTaskService(repo, mocks.NewMockSettingRepository(ctrl), log, nil, "http://localhost:8080")
				tc.setup(repo)

				svc.runTaskNow(context.Background(), "w1", "task-1")
			})
		}
	})

	t.Run("nothing runs when this instance does not execute its tasks itself", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		repo := mocks.NewMockTaskRepository(ctrl)
		svc := NewTaskService(repo, mocks.NewMockSettingRepository(ctrl), loggerAllowingErrors(t), nil, "http://localhost:8080")
		svc.SetAutoExecuteImmediate(false)

		svc.RunTaskNow("w1", "task-1")
		// Give a stray goroutine the time to hit the strict repository mock.
		time.Sleep(50 * time.Millisecond)
	})
}
