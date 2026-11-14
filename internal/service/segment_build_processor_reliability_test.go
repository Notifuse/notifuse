package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildProcessorHarness wires a SegmentBuildProcessor to strict mocks: any repository call a
// test does not expect fails it, so "nothing else is written" is asserted by omission.
type buildProcessorHarness struct {
	processor   *SegmentBuildProcessor
	segmentRepo *mocks.MockSegmentRepository
	contactRepo *mocks.MockContactRepository
	taskRepo    *mocks.MockTaskRepository
	wsRepo      *mocks.MockWorkspaceRepository
}

func newBuildProcessorHarness(t *testing.T) *buildProcessorHarness {
	t.Helper()
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	h := &buildProcessorHarness{
		segmentRepo: mocks.NewMockSegmentRepository(ctrl),
		contactRepo: mocks.NewMockContactRepository(ctrl),
		taskRepo:    mocks.NewMockTaskRepository(ctrl),
		wsRepo:      mocks.NewMockWorkspaceRepository(ctrl),
	}
	log := pkgmocks.NewMockLogger(ctrl)
	log.EXPECT().WithFields(gomock.Any()).Return(log).AnyTimes()
	log.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(log).AnyTimes()
	log.EXPECT().Info(gomock.Any()).AnyTimes()
	log.EXPECT().Warn(gomock.Any()).AnyTimes()
	log.EXPECT().Error(gomock.Any()).AnyTimes()

	h.processor = NewSegmentBuildProcessor(h.segmentRepo, h.contactRepo, h.taskRepo, h.wsRepo, log)
	return h
}

// buildTask returns a build task at version for segment1, with retries left unless
// lastAttempt is set.
func buildTask(version int64, lastAttempt bool) *domain.Task {
	task := &domain.Task{
		ID:          "task1",
		WorkspaceID: "workspace1",
		Type:        "build_segment",
		MaxRetries:  3,
		State: &domain.TaskState{
			BuildSegment: &domain.BuildSegmentState{
				SegmentID: "segment1",
				Version:   version,
			},
		},
	}
	if lastAttempt {
		task.RetryCount = 3
	}
	return task
}

// expectQuery makes the next segment query of processBatch return emails, in that order.
func (h *buildProcessorHarness) expectQuery(t *testing.T, emails ...string) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	rows := sqlmock.NewRows([]string{"email"})
	for _, e := range emails {
		rows.AddRow(e)
	}
	mock.ExpectQuery("SELECT email FROM contacts").WillReturnRows(rows)
	h.wsRepo.EXPECT().GetConnection(gomock.Any(), "workspace1").Return(db, nil)
}

func TestSegmentBuildProcessor_Process_AddContactError(t *testing.T) {
	h := newBuildProcessorHarness(t)
	ctx := context.Background()
	task := buildTask(1, false)
	segment := createTestSegmentWithSQL("segment1", "S", 1, string(domain.SegmentStatusBuilding))

	h.segmentRepo.EXPECT().GetSegmentByID(ctx, "workspace1", "segment1").Return(segment, nil).AnyTimes()
	h.contactRepo.EXPECT().Count(ctx, "workspace1").Return(2, nil)
	h.contactRepo.EXPECT().GetBatchForSegmentAfter(ctx, "workspace1", "", 100).
		Return([]string{"a@test.com", "b@test.com"}, nil)
	h.expectQuery(t, "a@test.com", "b@test.com")
	// The first add fails: the second contact is not attempted, the batch is not counted as
	// processed, nothing is saved, and the end-of-build steps never run.
	h.segmentRepo.EXPECT().AddContactToSegment(ctx, "workspace1", "a@test.com", "segment1", int64(1)).
		Return(errors.New("insert failed"))

	completed, err := h.processor.Process(ctx, task, time.Now().Add(5*time.Minute))
	require.Error(t, err)
	assert.False(t, completed)
	assert.Contains(t, err.Error(), "insert failed")
	assert.Equal(t, int64(0), task.State.BuildSegment.ContactOffset)
	assert.Equal(t, "", task.State.BuildSegment.LastEmail)
}

func TestSegmentBuildProcessor_Process_AddContactErrorLastAttempt(t *testing.T) {
	h := newBuildProcessorHarness(t)
	ctx := context.Background()
	task := buildTask(1, true)
	segment := createTestSegmentWithSQL("segment1", "S", 1, string(domain.SegmentStatusBuilding))
	due := time.Now().Add(-time.Hour)
	segment.RecomputeAfter = &due
	segment.Timezone = "UTC"

	h.segmentRepo.EXPECT().GetSegmentByID(ctx, "workspace1", "segment1").Return(segment, nil).AnyTimes()
	h.contactRepo.EXPECT().Count(ctx, "workspace1").Return(1, nil)
	h.contactRepo.EXPECT().GetBatchForSegmentAfter(ctx, "workspace1", "", 100).Return([]string{"a@test.com"}, nil)
	h.expectQuery(t, "a@test.com")
	h.segmentRepo.EXPECT().AddContactToSegment(ctx, "workspace1", "a@test.com", "segment1", int64(1)).
		Return(errors.New("insert failed"))

	// No retry is left: the segment goes back to active, so the queue and the daily rebuild
	// pick it up again, and its next rebuild waits for the next day instead of every tick.
	h.segmentRepo.EXPECT().UpdateSegmentStatus(ctx, "workspace1", "segment1", int64(1), string(domain.SegmentStatusActive)).
		Return(true, nil)
	h.segmentRepo.EXPECT().UpdateRecomputeAfter(ctx, "workspace1", "segment1", gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ string, at *time.Time) error {
			require.NotNil(t, at)
			assert.True(t, at.After(time.Now()), "the next rebuild must be in the future")
			return nil
		})

	completed, err := h.processor.Process(ctx, task, time.Now().Add(5*time.Minute))
	require.Error(t, err)
	assert.False(t, completed)
}

func TestSegmentBuildProcessor_Process_LastAttemptWithoutRecompute(t *testing.T) {
	h := newBuildProcessorHarness(t)
	ctx := context.Background()
	task := buildTask(1, true)
	segment := createTestSegmentWithSQL("segment1", "S", 1, string(domain.SegmentStatusBuilding))

	h.segmentRepo.EXPECT().GetSegmentByID(ctx, "workspace1", "segment1").Return(segment, nil).AnyTimes()
	h.contactRepo.EXPECT().Count(ctx, "workspace1").Return(1, nil)
	h.contactRepo.EXPECT().GetBatchForSegmentAfter(ctx, "workspace1", "", 100).Return(nil, errors.New("db down"))
	// A failed reactivation is only logged; the build's own error is the one returned.
	h.segmentRepo.EXPECT().UpdateSegmentStatus(ctx, "workspace1", "segment1", int64(1), string(domain.SegmentStatusActive)).
		Return(false, errors.New("still down"))

	completed, err := h.processor.Process(ctx, task, time.Now().Add(5*time.Minute))
	require.Error(t, err)
	assert.False(t, completed)
	assert.Contains(t, err.Error(), "db down")
}

func TestSegmentBuildProcessor_Process_MatchedContactsAddedInQueryOrder(t *testing.T) {
	h := newBuildProcessorHarness(t)
	ctx := context.Background()
	task := buildTask(1, false)
	segment := createTestSegmentWithSQL("segment1", "S", 1, string(domain.SegmentStatusBuilding))

	h.segmentRepo.EXPECT().GetSegmentByID(ctx, "workspace1", "segment1").Return(segment, nil).AnyTimes()
	h.contactRepo.EXPECT().Count(ctx, "workspace1").Return(3, nil)
	h.contactRepo.EXPECT().GetBatchForSegmentAfter(ctx, "workspace1", "", 100).
		Return([]string{"a@test.com", "b@test.com", "c@test.com"}, nil)
	// The query returns c then a, a second time c (a join can repeat a row): each is added
	// once, in the order returned.
	h.expectQuery(t, "c@test.com", "a@test.com", "c@test.com")
	gomock.InOrder(
		h.segmentRepo.EXPECT().AddContactToSegment(ctx, "workspace1", "c@test.com", "segment1", int64(1)).Return(nil),
		h.segmentRepo.EXPECT().AddContactToSegment(ctx, "workspace1", "a@test.com", "segment1", int64(1)).Return(nil),
	)
	h.taskRepo.EXPECT().SaveState(ctx, "workspace1", "task1", gomock.Any(), gomock.Any()).Return(nil)
	// The next page starts after the last email of the batch, not at an offset.
	h.contactRepo.EXPECT().GetBatchForSegmentAfter(ctx, "workspace1", "c@test.com", 100).Return(nil, nil)
	h.segmentRepo.EXPECT().RemoveOldMemberships(ctx, "workspace1", "segment1", int64(1)).Return(nil)
	h.segmentRepo.EXPECT().UpdateSegmentStatus(ctx, "workspace1", "segment1", int64(1), string(domain.SegmentStatusActive)).
		Return(true, nil)

	completed, err := h.processor.Process(ctx, task, time.Now().Add(5*time.Minute))
	require.NoError(t, err)
	assert.True(t, completed)
	assert.Equal(t, 2, task.State.BuildSegment.MatchedCount)
	assert.Equal(t, "c@test.com", task.State.BuildSegment.LastEmail)
	assert.Equal(t, int64(3), task.State.BuildSegment.ContactOffset)
}

func TestSegmentBuildProcessor_Process_BatchesButNoMatch(t *testing.T) {
	h := newBuildProcessorHarness(t)
	ctx := context.Background()
	task := buildTask(2, false)
	segment := createTestSegmentWithSQL("segment1", "S", 2, string(domain.SegmentStatusBuilding))
	due := time.Now().Add(-time.Hour)
	segment.RecomputeAfter = &due
	segment.Timezone = "UTC"

	h.segmentRepo.EXPECT().GetSegmentByID(ctx, "workspace1", "segment1").Return(segment, nil).AnyTimes()
	h.contactRepo.EXPECT().Count(ctx, "workspace1").Return(1, nil)
	h.contactRepo.EXPECT().GetBatchForSegmentAfter(ctx, "workspace1", "", 100).Return([]string{"a@test.com"}, nil)
	h.expectQuery(t) // nobody matches
	h.taskRepo.EXPECT().SaveState(ctx, "workspace1", "task1", gomock.Any(), gomock.Any()).Return(nil)
	h.contactRepo.EXPECT().GetBatchForSegmentAfter(ctx, "workspace1", "a@test.com", 100).Return(nil, nil)

	// Nobody matches: the members of the previous version are still dropped, and the next
	// daily rebuild is still scheduled.
	h.segmentRepo.EXPECT().RemoveOldMemberships(ctx, "workspace1", "segment1", int64(2)).Return(nil)
	h.segmentRepo.EXPECT().UpdateSegmentStatus(ctx, "workspace1", "segment1", int64(2), string(domain.SegmentStatusActive)).
		Return(true, nil)
	h.segmentRepo.EXPECT().UpdateRecomputeAfter(ctx, "workspace1", "segment1", gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ string, at *time.Time) error {
			require.NotNil(t, at)
			assert.True(t, at.After(time.Now()))
			return nil
		})

	completed, err := h.processor.Process(ctx, task, time.Now().Add(5*time.Minute))
	require.NoError(t, err)
	assert.True(t, completed)
	assert.Equal(t, 0, task.State.BuildSegment.MatchedCount)
}

func TestSegmentBuildProcessor_Process_BumpNotLanded(t *testing.T) {
	t.Run("at the start", func(t *testing.T) {
		h := newBuildProcessorHarness(t)
		ctx := context.Background()
		// The task writes version 4 but the segment is still at 3: nothing may be written,
		// or every member would sit at a version the segment does not have.
		task := buildTask(4, false)
		segment := createTestSegmentWithSQL("segment1", "S", 3, string(domain.SegmentStatusActive))
		h.segmentRepo.EXPECT().GetSegmentByID(ctx, "workspace1", "segment1").Return(segment, nil)

		completed, err := h.processor.Process(ctx, task, time.Now().Add(5*time.Minute))
		require.Error(t, err)
		assert.False(t, completed)
		assert.Contains(t, err.Error(), "behind")
	})

	t.Run("on the last attempt the segment is left alone", func(t *testing.T) {
		h := newBuildProcessorHarness(t)
		ctx := context.Background()
		task := buildTask(4, true)
		segment := createTestSegmentWithSQL("segment1", "S", 3, string(domain.SegmentStatusActive))
		h.segmentRepo.EXPECT().GetSegmentByID(ctx, "workspace1", "segment1").Return(segment, nil)

		completed, err := h.processor.Process(ctx, task, time.Now().Add(5*time.Minute))
		require.Error(t, err)
		assert.False(t, completed)
	})

	t.Run("before a batch", func(t *testing.T) {
		h := newBuildProcessorHarness(t)
		ctx := context.Background()
		task := buildTask(4, false)
		current := createTestSegmentWithSQL("segment1", "S", 4, string(domain.SegmentStatusBuilding))
		reverted := createTestSegmentWithSQL("segment1", "S", 3, string(domain.SegmentStatusActive))
		gomock.InOrder(
			h.segmentRepo.EXPECT().GetSegmentByID(ctx, "workspace1", "segment1").Return(current, nil),
			h.segmentRepo.EXPECT().GetSegmentByID(ctx, "workspace1", "segment1").Return(reverted, nil),
		)
		h.contactRepo.EXPECT().Count(ctx, "workspace1").Return(1, nil)

		completed, err := h.processor.Process(ctx, task, time.Now().Add(5*time.Minute))
		require.Error(t, err)
		assert.False(t, completed)
		assert.Contains(t, err.Error(), "behind")
	})
}

func TestSegmentBuildProcessor_Process_DeletedSegment(t *testing.T) {
	t.Run("at the start", func(t *testing.T) {
		h := newBuildProcessorHarness(t)
		ctx := context.Background()
		task := buildTask(1, false)
		segment := createTestSegmentWithSQL("segment1", "S", 1, string(domain.SegmentStatusDeleted))
		h.segmentRepo.EXPECT().GetSegmentByID(ctx, "workspace1", "segment1").Return(segment, nil)

		completed, err := h.processor.Process(ctx, task, time.Now().Add(5*time.Minute))
		require.NoError(t, err)
		assert.True(t, completed)
	})

	t.Run("during the build", func(t *testing.T) {
		h := newBuildProcessorHarness(t)
		ctx := context.Background()
		task := buildTask(1, false)
		building := createTestSegmentWithSQL("segment1", "S", 1, string(domain.SegmentStatusBuilding))
		deleted := createTestSegmentWithSQL("segment1", "S", 1, string(domain.SegmentStatusDeleted))
		gomock.InOrder(
			h.segmentRepo.EXPECT().GetSegmentByID(ctx, "workspace1", "segment1").Return(building, nil),
			h.segmentRepo.EXPECT().GetSegmentByID(ctx, "workspace1", "segment1").Return(deleted, nil),
		)
		h.contactRepo.EXPECT().Count(ctx, "workspace1").Return(1, nil)

		completed, err := h.processor.Process(ctx, task, time.Now().Add(5*time.Minute))
		require.NoError(t, err)
		assert.True(t, completed)
	})
}

func TestSegmentBuildProcessor_Process_StatusWriteFindsSegmentMoved(t *testing.T) {
	t.Run("at the start", func(t *testing.T) {
		h := newBuildProcessorHarness(t)
		ctx := context.Background()
		task := buildTask(1, false)
		segment := createTestSegmentWithSQL("segment1", "S", 1, string(domain.SegmentStatusActive))
		h.segmentRepo.EXPECT().GetSegmentByID(ctx, "workspace1", "segment1").Return(segment, nil)
		// Edited or deleted between the read and the write: another build owns it now.
		h.segmentRepo.EXPECT().UpdateSegmentStatus(ctx, "workspace1", "segment1", int64(1), string(domain.SegmentStatusBuilding)).
			Return(false, nil)

		completed, err := h.processor.Process(ctx, task, time.Now().Add(5*time.Minute))
		require.NoError(t, err)
		assert.True(t, completed)
	})

	t.Run("at the end", func(t *testing.T) {
		h := newBuildProcessorHarness(t)
		ctx := context.Background()
		task := buildTask(1, false)
		segment := createTestSegmentWithSQL("segment1", "S", 1, string(domain.SegmentStatusBuilding))
		due := time.Now().Add(-time.Hour)
		segment.RecomputeAfter = &due
		h.segmentRepo.EXPECT().GetSegmentByID(ctx, "workspace1", "segment1").Return(segment, nil).AnyTimes()
		h.contactRepo.EXPECT().Count(ctx, "workspace1").Return(0, nil)
		h.contactRepo.EXPECT().GetBatchForSegmentAfter(ctx, "workspace1", "", 100).Return(nil, nil)
		h.segmentRepo.EXPECT().RemoveOldMemberships(ctx, "workspace1", "segment1", int64(1)).Return(nil)
		// The final write misses: recompute_after is not moved for a segment this build no
		// longer owns.
		h.segmentRepo.EXPECT().UpdateSegmentStatus(ctx, "workspace1", "segment1", int64(1), string(domain.SegmentStatusActive)).
			Return(false, nil)

		completed, err := h.processor.Process(ctx, task, time.Now().Add(5*time.Minute))
		require.NoError(t, err)
		assert.True(t, completed)
	})
}

func TestSegmentBuildProcessor_Process_ResumesLegacyOffset(t *testing.T) {
	h := newBuildProcessorHarness(t)
	ctx := context.Background()
	task := buildTask(1, false)
	// A state saved before the build recorded its last email: it resumes once by offset.
	task.State.BuildSegment.ContactOffset = 200
	task.State.BuildSegment.TotalContacts = 201
	segment := createTestSegmentWithSQL("segment1", "S", 1, string(domain.SegmentStatusBuilding))

	h.segmentRepo.EXPECT().GetSegmentByID(ctx, "workspace1", "segment1").Return(segment, nil).AnyTimes()
	h.contactRepo.EXPECT().GetBatchForSegment(ctx, "workspace1", int64(200), 100).Return([]string{"z@test.com"}, nil)
	h.expectQuery(t, "z@test.com")
	h.segmentRepo.EXPECT().AddContactToSegment(ctx, "workspace1", "z@test.com", "segment1", int64(1)).Return(nil)
	h.taskRepo.EXPECT().SaveState(ctx, "workspace1", "task1", gomock.Any(), gomock.Any()).Return(nil)
	h.contactRepo.EXPECT().GetBatchForSegmentAfter(ctx, "workspace1", "z@test.com", 100).Return(nil, nil)
	h.segmentRepo.EXPECT().RemoveOldMemberships(ctx, "workspace1", "segment1", int64(1)).Return(nil)
	h.segmentRepo.EXPECT().UpdateSegmentStatus(ctx, "workspace1", "segment1", int64(1), string(domain.SegmentStatusActive)).
		Return(true, nil)

	completed, err := h.processor.Process(ctx, task, time.Now().Add(5*time.Minute))
	require.NoError(t, err)
	assert.True(t, completed)
	assert.Equal(t, int64(201), task.State.BuildSegment.ContactOffset)
}

func TestSegmentBuildProcessor_Process_FinalStepsDeferredNearTimeout(t *testing.T) {
	h := newBuildProcessorHarness(t)
	ctx := context.Background()
	task := buildTask(1, false)
	segment := createTestSegmentWithSQL("segment1", "S", 1, string(domain.SegmentStatusBuilding))

	h.segmentRepo.EXPECT().GetSegmentByID(ctx, "workspace1", "segment1").Return(segment, nil).AnyTimes()
	h.contactRepo.EXPECT().Count(ctx, "workspace1").Return(1, nil)
	h.contactRepo.EXPECT().GetBatchForSegmentAfter(ctx, "workspace1", "", 100).Return([]string{"a@test.com"}, nil)
	h.expectQuery(t, "a@test.com")
	h.segmentRepo.EXPECT().AddContactToSegment(ctx, "workspace1", "a@test.com", "segment1", int64(1)).Return(nil)
	h.contactRepo.EXPECT().GetBatchForSegmentAfter(ctx, "workspace1", "a@test.com", 100).Return(nil, nil)
	// Saved after the batch, then again when the end steps are put off to the next slice.
	h.taskRepo.EXPECT().SaveState(ctx, "workspace1", "task1", gomock.Any(), gomock.Any()).Return(nil).Times(2)

	completed, err := h.processor.Process(ctx, task, time.Now().Add(30*time.Second))
	require.NoError(t, err)
	assert.False(t, completed, "the end steps wait for a slice with a full budget")
	assert.Equal(t, "a@test.com", task.State.BuildSegment.LastEmail)
}

func TestSegmentBuildProcessor_Process_FinalStepsRunWhenSliceDidNoBatch(t *testing.T) {
	h := newBuildProcessorHarness(t)
	ctx := context.Background()
	task := buildTask(1, false)
	task.State.BuildSegment.LastEmail = "a@test.com"
	task.State.BuildSegment.ContactOffset = 1
	task.State.BuildSegment.TotalContacts = 1
	segment := createTestSegmentWithSQL("segment1", "S", 1, string(domain.SegmentStatusBuilding))

	h.segmentRepo.EXPECT().GetSegmentByID(ctx, "workspace1", "segment1").Return(segment, nil).AnyTimes()
	h.contactRepo.EXPECT().GetBatchForSegmentAfter(ctx, "workspace1", "a@test.com", 100).Return(nil, nil)
	// A slice that only has the end steps left runs them whatever its budget, so a short
	// budget can never put them off forever.
	h.segmentRepo.EXPECT().RemoveOldMemberships(ctx, "workspace1", "segment1", int64(1)).Return(nil)
	h.segmentRepo.EXPECT().UpdateSegmentStatus(ctx, "workspace1", "segment1", int64(1), string(domain.SegmentStatusActive)).
		Return(true, nil)

	completed, err := h.processor.Process(ctx, task, time.Now().Add(30*time.Second))
	require.NoError(t, err)
	assert.True(t, completed)
}
