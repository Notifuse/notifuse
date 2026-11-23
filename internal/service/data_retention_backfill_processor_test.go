package service

import (
	"context"
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

func newDataRetentionBackfillTest(t *testing.T) (*DataRetentionBackfillProcessor, *mocks.MockContactEngagementRepository, *mocks.MockTaskRepository) {
	t.Helper()
	ctrl := gomock.NewController(t)
	engagement := mocks.NewMockContactEngagementRepository(ctrl)
	taskRepo := mocks.NewMockTaskRepository(ctrl)
	log := pkgmocks.NewMockLogger(ctrl)
	log.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(log).AnyTimes()
	log.EXPECT().Error(gomock.Any()).AnyTimes()
	return NewDataRetentionBackfillProcessor(engagement, taskRepo, log), engagement, taskRepo
}

func TestDataRetentionBackfillProcessor(t *testing.T) {
	ctx := context.Background()
	later := time.Now().Add(time.Minute)

	t.Run("handles its own task type only", func(t *testing.T) {
		p, _, _ := newDataRetentionBackfillTest(t)
		assert.True(t, p.CanProcess(domain.DataRetentionBackfillTaskType))
		assert.False(t, p.CanProcess(domain.WebAnalyticsBackfillTaskType))
	})

	t.Run("walks every batch, saves the cursor, then marks the workspace ready", func(t *testing.T) {
		p, engagement, taskRepo := newDataRetentionBackfillTest(t)
		task := &domain.Task{ID: "t1", WorkspaceID: "ws"}
		gomock.InOrder(
			engagement.EXPECT().BackfillBatch(ctx, "ws", "", dataRetentionBackfillBatch).Return("m@x", int64(2000), int64(300), nil),
			taskRepo.EXPECT().SaveState(ctx, "ws", "t1", gomock.Any(), gomock.Any()).Return(nil),
			engagement.EXPECT().BackfillBatch(ctx, "ws", "m@x", dataRetentionBackfillBatch).Return("z@x", int64(10), int64(1), nil),
			taskRepo.EXPECT().SaveState(ctx, "ws", "t1", gomock.Any(), gomock.Any()).Return(nil),
			engagement.EXPECT().BackfillBatch(ctx, "ws", "z@x", dataRetentionBackfillBatch).Return("", int64(0), int64(0), nil),
			engagement.EXPECT().CompleteBackfill(ctx, "ws").Return(nil),
		)
		done, err := p.Process(ctx, task, later)
		require.NoError(t, err)
		assert.True(t, done)
		state := task.State.DataRetentionBackfill
		assert.Equal(t, "z@x", state.AfterEmail)
		assert.Equal(t, int64(2010), state.Contacts)
		assert.Equal(t, int64(301), state.Engaged)
	})

	t.Run("resumes after the saved email", func(t *testing.T) {
		p, engagement, _ := newDataRetentionBackfillTest(t)
		task := &domain.Task{ID: "t1", WorkspaceID: "ws", State: &domain.TaskState{
			DataRetentionBackfill: &domain.DataRetentionBackfillState{AfterEmail: "k@x", Contacts: 5},
		}}
		engagement.EXPECT().BackfillBatch(ctx, "ws", "k@x", dataRetentionBackfillBatch).Return("", int64(0), int64(0), nil)
		engagement.EXPECT().CompleteBackfill(ctx, "ws").Return(nil)
		done, err := p.Process(ctx, task, later)
		require.NoError(t, err)
		assert.True(t, done)
	})

	t.Run("stops before the budget is spent, without marking ready", func(t *testing.T) {
		p, _, taskRepo := newDataRetentionBackfillTest(t)
		task := &domain.Task{ID: "t1", WorkspaceID: "ws"}
		taskRepo.EXPECT().SaveState(ctx, "ws", "t1", gomock.Any(), gomock.Any()).Return(nil)
		done, err := p.Process(ctx, task, time.Now().Add(time.Second))
		require.NoError(t, err)
		assert.False(t, done)
	})

	t.Run("a failed batch is an error and does not mark ready", func(t *testing.T) {
		p, engagement, taskRepo := newDataRetentionBackfillTest(t)
		task := &domain.Task{ID: "t1", WorkspaceID: "ws"}
		engagement.EXPECT().BackfillBatch(ctx, "ws", "", dataRetentionBackfillBatch).Return("", int64(0), int64(0), errors.New("boom"))
		taskRepo.EXPECT().SaveState(ctx, "ws", "t1", gomock.Any(), gomock.Any()).Return(nil)
		done, err := p.Process(ctx, task, later)
		assert.Error(t, err)
		assert.False(t, done)
	})
}
