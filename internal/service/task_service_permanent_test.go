package service

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newPermanentTaskTestService(t *testing.T) (*TaskService, *mocks.MockTaskRepository, *mocks.MockSettingRepository, *gomock.Controller) {
	t.Helper()
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

	// No API endpoint: due tasks would run in-process; the tests return none.
	svc := NewTaskService(repo, settings, logger, nil, "")
	svc.SetAutoExecuteImmediate(false)
	settings.EXPECT().SetLastCronRun(gomock.Any()).Return(nil).AnyTimes()
	return svc, repo, settings, ctrl
}

func TestTaskService_ExecutePendingTasks_RestoresPermanentTasks(t *testing.T) {
	ctx := context.Background()

	t.Run("on the first pass, then again once the interval has passed", func(t *testing.T) {
		svc, repo, _, ctrl := newPermanentTaskTestService(t)
		defer ctrl.Finish()
		svc.SetRestorePermanentTasks(true)

		gomock.InOrder(
			repo.EXPECT().RestorePermanentTasks(gomock.Any(), "").Return(domain.PermanentTaskRestore{Revived: 1, Created: 2}, nil).Times(1),
			repo.EXPECT().GetNextBatch(gomock.Any(), 100).Return(nil, nil).Times(1),
		)
		require.NoError(t, svc.ExecutePendingTasks(ctx, 0))

		// Within the interval: no restore.
		repo.EXPECT().GetNextBatch(gomock.Any(), 100).Return(nil, nil).Times(2)
		require.NoError(t, svc.ExecutePendingTasks(ctx, 0))
		require.NoError(t, svc.ExecutePendingTasks(ctx, 0))

		// The last success is an interval old: a workspace whose tasks went missing since,
		// such as one whose creation could not create them, gets them back.
		svc.permanentTasksRestoredAt.Store(time.Now().Add(-permanentTaskRestoreInterval - time.Second).UnixNano())
		gomock.InOrder(
			repo.EXPECT().RestorePermanentTasks(gomock.Any(), "").Return(domain.PermanentTaskRestore{Created: 2}, nil).Times(1),
			repo.EXPECT().GetNextBatch(gomock.Any(), 100).Return(nil, nil).Times(1),
		)
		require.NoError(t, svc.ExecutePendingTasks(ctx, 0))
		assert.WithinDuration(t, time.Now(), time.Unix(0, svc.permanentTasksRestoredAt.Load()), time.Second)
	})

	t.Run("the interval is an hour", func(t *testing.T) {
		assert.Equal(t, time.Hour, permanentTaskRestoreInterval)
	})

	t.Run("a failed restore does not hold the batch and is tried again on the next pass", func(t *testing.T) {
		svc, repo, _, ctrl := newPermanentTaskTestService(t)
		defer ctrl.Finish()
		svc.SetRestorePermanentTasks(true)

		gomock.InOrder(
			repo.EXPECT().RestorePermanentTasks(gomock.Any(), "").Return(domain.PermanentTaskRestore{}, errors.New("database starting")),
			repo.EXPECT().GetNextBatch(gomock.Any(), 100).Return(nil, nil),
			repo.EXPECT().RestorePermanentTasks(gomock.Any(), "").Return(domain.PermanentTaskRestore{}, nil),
			repo.EXPECT().GetNextBatch(gomock.Any(), 100).Return(nil, nil),
			repo.EXPECT().GetNextBatch(gomock.Any(), 100).Return(nil, nil),
		)
		require.NoError(t, svc.ExecutePendingTasks(ctx, 0))
		require.NoError(t, svc.ExecutePendingTasks(ctx, 0))
		require.NoError(t, svc.ExecutePendingTasks(ctx, 0))
	})

	t.Run("off unless the app turns it on", func(t *testing.T) {
		svc, repo, _, ctrl := newPermanentTaskTestService(t)
		defer ctrl.Finish()

		// No RestorePermanentTasks expected: gomock fails the test on an unexpected call.
		repo.EXPECT().GetNextBatch(gomock.Any(), 100).Return(nil, nil)
		require.NoError(t, svc.ExecutePendingTasks(ctx, 0))

		svc.SetRestorePermanentTasks(true)
		svc.SetRestorePermanentTasks(false)
		repo.EXPECT().GetNextBatch(gomock.Any(), 100).Return(nil, nil)
		require.NoError(t, svc.ExecutePendingTasks(ctx, 0))
	})
}

func TestTaskService_ExecuteTask_PermanentTaskNeverCompletes(t *testing.T) {
	for _, taskType := range domain.PermanentTaskTypes {
		t.Run(taskType, func(t *testing.T) {
			svc, repo, _, ctrl := newPermanentTaskTestService(t)
			defer ctrl.Finish()

			processor := mocks.NewMockTaskProcessor(ctrl)
			processor.EXPECT().CanProcess(gomock.Any()).DoAndReturn(func(tt string) bool { return tt == taskType }).AnyTimes()
			svc.RegisterProcessor(processor)

			repo.EXPECT().WithTransaction(gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, fn func(*sql.Tx) error) error { return fn(nil) }).AnyTimes()

			task := &domain.Task{ID: "t1", WorkspaceID: "ws1", Type: taskType, Status: domain.TaskStatusPending, MaxRuntime: 50}
			repo.EXPECT().GetTx(gomock.Any(), gomock.Any(), "ws1", "t1").Return(task, nil)
			repo.EXPECT().MarkAsRunningTx(gomock.Any(), gomock.Any(), "ws1", "t1", gomock.Any()).Return(nil)
			// A processor that reports done through a bug.
			processor.EXPECT().Process(gomock.Any(), task, gomock.Any()).Return(true, nil)
			// Kept pending and due now; MarkAsCompleted is not expected.
			repo.EXPECT().MarkAsPending(gomock.Any(), "ws1", "t1", gomock.Any(), gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, _, _ string, nextRun time.Time, _ float64, _ *domain.TaskState) error {
					assert.WithinDuration(t, time.Now().UTC(), nextRun, 2*time.Second)
					return nil
				})

			require.NoError(t, svc.ExecuteTask(context.Background(), "ws1", "t1", time.Now().Add(time.Minute)))
		})
	}
}
