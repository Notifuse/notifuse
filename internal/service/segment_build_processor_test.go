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
)

func createValidSegmentTree() *domain.TreeNode {
	return &domain.TreeNode{
		Kind: "leaf",
		Leaf: &domain.TreeNodeLeaf{
			Table: "contacts",
			Contact: &domain.ContactCondition{
				Filters: []*domain.DimensionFilter{
					{
						FieldName:    "email",
						FieldType:    "string",
						Operator:     "contains",
						StringValues: []string{"@test.com"},
					},
				},
			},
		},
	}
}

func TestNewSegmentBuildProcessor(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSegmentRepo := mocks.NewMockSegmentRepository(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockTaskRepo := mocks.NewMockTaskRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	processor := NewSegmentBuildProcessor(mockSegmentRepo, mockContactRepo, mockTaskRepo, mockWorkspaceRepo, mockLogger)

	assert.NotNil(t, processor)
	assert.NotNil(t, processor.segmentRepo)
	assert.NotNil(t, processor.contactRepo)
	assert.NotNil(t, processor.taskRepo)
	assert.NotNil(t, processor.workspaceRepo)
	assert.NotNil(t, processor.queryBuilder)
	assert.NotNil(t, processor.logger)
	assert.Equal(t, 1000, processor.batchSize)
}

func TestSegmentBuildProcessor_CanProcess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSegmentRepo := mocks.NewMockSegmentRepository(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockTaskRepo := mocks.NewMockTaskRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	processor := NewSegmentBuildProcessor(mockSegmentRepo, mockContactRepo, mockTaskRepo, mockWorkspaceRepo, mockLogger)

	t.Run("can process build_segment", func(t *testing.T) {
		assert.True(t, processor.CanProcess("build_segment"))
	})

	t.Run("cannot process other types", func(t *testing.T) {
		assert.False(t, processor.CanProcess("other_task"))
		assert.False(t, processor.CanProcess(""))
		assert.False(t, processor.CanProcess("send_email"))
	})
}

func TestSegmentBuildProcessor_Process_ValidationErrors(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSegmentRepo := mocks.NewMockSegmentRepository(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockTaskRepo := mocks.NewMockTaskRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Setup logger expectations
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()

	processor := NewSegmentBuildProcessor(mockSegmentRepo, mockContactRepo, mockTaskRepo, mockWorkspaceRepo, mockLogger)

	ctx := context.Background()
	timeoutAt := time.Now().Add(5 * time.Minute)

	t.Run("missing BuildSegment state", func(t *testing.T) {
		task := &domain.Task{
			ID:          "task1",
			WorkspaceID: "workspace1",
			Type:        "build_segment",
			State:       &domain.TaskState{
				// BuildSegment is nil
			},
		}

		completed, err := processor.Process(ctx, task, timeoutAt)
		assert.False(t, completed)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "missing BuildSegment data")
	})

	t.Run("missing segment_id in state", func(t *testing.T) {
		task := &domain.Task{
			ID:          "task1",
			WorkspaceID: "workspace1",
			Type:        "build_segment",
			State: &domain.TaskState{
				BuildSegment: &domain.BuildSegmentState{
					// SegmentID is empty
				},
			},
		}

		completed, err := processor.Process(ctx, task, timeoutAt)
		assert.False(t, completed)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "missing segment_id")
	})
}

func TestSegmentBuildProcessor_Process_SegmentNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSegmentRepo := mocks.NewMockSegmentRepository(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockTaskRepo := mocks.NewMockTaskRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

	processor := NewSegmentBuildProcessor(mockSegmentRepo, mockContactRepo, mockTaskRepo, mockWorkspaceRepo, mockLogger)

	ctx := context.Background()
	timeoutAt := time.Now().Add(5 * time.Minute)

	task := &domain.Task{
		ID:          "task1",
		WorkspaceID: "workspace1",
		Type:        "build_segment",
		State: &domain.TaskState{
			BuildSegment: &domain.BuildSegmentState{
				SegmentID: "segment1",
				Version:   1,
			},
		},
	}

	mockSegmentRepo.EXPECT().
		GetSegmentByID(ctx, "workspace1", "segment1").
		Return(nil, &domain.ErrSegmentNotFound{Message: "not found"})

	completed, err := processor.Process(ctx, task, timeoutAt)
	assert.False(t, completed)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to fetch segment")
}

func TestSegmentBuildProcessor_Process_UpdateStatusError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSegmentRepo := mocks.NewMockSegmentRepository(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockTaskRepo := mocks.NewMockTaskRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

	processor := NewSegmentBuildProcessor(mockSegmentRepo, mockContactRepo, mockTaskRepo, mockWorkspaceRepo, mockLogger)

	ctx := context.Background()
	timeoutAt := time.Now().Add(5 * time.Minute)

	task := &domain.Task{
		ID:          "task1",
		WorkspaceID: "workspace1",
		Type:        "build_segment",
		State: &domain.TaskState{
			BuildSegment: &domain.BuildSegmentState{
				SegmentID: "segment1",
				Version:   1,
			},
		},
	}

	segment := &domain.Segment{
		ID:      "segment1",
		Name:    "Test Segment",
		Status:  string(domain.SegmentStatusActive), // Not building
		Version: 1,
		Tree:    createValidSegmentTree(),
	}

	mockSegmentRepo.EXPECT().
		GetSegmentByID(ctx, "workspace1", "segment1").
		Return(segment, nil)

	mockSegmentRepo.EXPECT().
		UpdateSegment(ctx, "workspace1", gomock.Any()).
		Return(errors.New("db error"))

	completed, err := processor.Process(ctx, task, timeoutAt)
	assert.False(t, completed)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to update segment status")
}

func TestSegmentBuildProcessor_Process_InvalidTree(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSegmentRepo := mocks.NewMockSegmentRepository(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockTaskRepo := mocks.NewMockTaskRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()

	processor := NewSegmentBuildProcessor(mockSegmentRepo, mockContactRepo, mockTaskRepo, mockWorkspaceRepo, mockLogger)

	ctx := context.Background()
	timeoutAt := time.Now().Add(5 * time.Minute)

	task := &domain.Task{
		ID:          "task1",
		WorkspaceID: "workspace1",
		Type:        "build_segment",
		State: &domain.TaskState{
			BuildSegment: &domain.BuildSegmentState{
				SegmentID: "segment1",
				Version:   1,
			},
		},
	}

	segment := &domain.Segment{
		ID:      "segment1",
		Name:    "Test Segment",
		Status:  string(domain.SegmentStatusBuilding),
		Version: 1,
		Tree: &domain.TreeNode{
			Kind: "invalid", // Invalid tree
		},
	}

	mockSegmentRepo.EXPECT().
		GetSegmentByID(ctx, "workspace1", "segment1").
		Return(segment, nil)

	completed, err := processor.Process(ctx, task, timeoutAt)
	assert.False(t, completed)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to build SQL query")
}

func TestSegmentBuildProcessor_Process_CountError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSegmentRepo := mocks.NewMockSegmentRepository(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockTaskRepo := mocks.NewMockTaskRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()

	processor := NewSegmentBuildProcessor(mockSegmentRepo, mockContactRepo, mockTaskRepo, mockWorkspaceRepo, mockLogger)

	ctx := context.Background()
	timeoutAt := time.Now().Add(5 * time.Minute)

	task := &domain.Task{
		ID:          "task1",
		WorkspaceID: "workspace1",
		Type:        "build_segment",
		State: &domain.TaskState{
			BuildSegment: &domain.BuildSegmentState{
				SegmentID: "segment1",
				Version:   1,
			},
		},
	}

	segment := &domain.Segment{
		ID:      "segment1",
		Name:    "Test Segment",
		Status:  string(domain.SegmentStatusBuilding),
		Version: 1,
		Tree:    createValidSegmentTree(),
	}

	mockSegmentRepo.EXPECT().
		GetSegmentByID(ctx, "workspace1", "segment1").
		Return(segment, nil)

	mockSegmentRepo.EXPECT().
		UpdateSegment(ctx, "workspace1", gomock.Any()).
		Return(nil).
		AnyTimes()

	mockContactRepo.EXPECT().
		Count(ctx, "workspace1").
		Return(0, errors.New("db error"))

	completed, err := processor.Process(ctx, task, timeoutAt)
	assert.False(t, completed)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to count contacts")
}

func TestSegmentBuildProcessor_Process_NoContacts(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSegmentRepo := mocks.NewMockSegmentRepository(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockTaskRepo := mocks.NewMockTaskRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()

	processor := NewSegmentBuildProcessor(mockSegmentRepo, mockContactRepo, mockTaskRepo, mockWorkspaceRepo, mockLogger)

	ctx := context.Background()
	timeoutAt := time.Now().Add(5 * time.Minute)

	task := &domain.Task{
		ID:          "task1",
		WorkspaceID: "workspace1",
		Type:        "build_segment",
		State: &domain.TaskState{
			BuildSegment: &domain.BuildSegmentState{
				SegmentID: "segment1",
				Version:   1,
			},
		},
	}

	segment := &domain.Segment{
		ID:      "segment1",
		Name:    "Test Segment",
		Status:  string(domain.SegmentStatusBuilding),
		Version: 1,
		Tree:    createValidSegmentTree(),
	}

	mockSegmentRepo.EXPECT().
		GetSegmentByID(ctx, "workspace1", "segment1").
		Return(segment, nil)

	mockSegmentRepo.EXPECT().
		UpdateSegment(ctx, "workspace1", gomock.Any()).
		Return(nil).
		Times(2) // Once for SQL storage, once for final status update

	mockContactRepo.EXPECT().
		Count(ctx, "workspace1").
		Return(0, nil)

	mockContactRepo.EXPECT().
		GetBatchForSegment(ctx, "workspace1", int64(0), 1000).
		Return([]*domain.Contact{}, nil)

	mockSegmentRepo.EXPECT().
		RemoveOldMemberships(ctx, "workspace1", "segment1", int64(1)).
		Return(nil)

	completed, err := processor.Process(ctx, task, timeoutAt)
	assert.True(t, completed)
	assert.NoError(t, err)
}

func TestSegmentBuildProcessor_Process_GetBatchError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSegmentRepo := mocks.NewMockSegmentRepository(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockTaskRepo := mocks.NewMockTaskRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()

	processor := NewSegmentBuildProcessor(mockSegmentRepo, mockContactRepo, mockTaskRepo, mockWorkspaceRepo, mockLogger)

	ctx := context.Background()
	timeoutAt := time.Now().Add(5 * time.Minute)

	task := &domain.Task{
		ID:          "task1",
		WorkspaceID: "workspace1",
		Type:        "build_segment",
		State: &domain.TaskState{
			BuildSegment: &domain.BuildSegmentState{
				SegmentID: "segment1",
				Version:   1,
			},
		},
	}

	segment := &domain.Segment{
		ID:      "segment1",
		Name:    "Test Segment",
		Status:  string(domain.SegmentStatusBuilding),
		Version: 1,
		Tree:    createValidSegmentTree(),
	}

	mockSegmentRepo.EXPECT().
		GetSegmentByID(ctx, "workspace1", "segment1").
		Return(segment, nil)

	mockSegmentRepo.EXPECT().
		UpdateSegment(ctx, "workspace1", gomock.Any()).
		Return(nil).
		AnyTimes()

	mockContactRepo.EXPECT().
		Count(ctx, "workspace1").
		Return(100, nil)

	mockContactRepo.EXPECT().
		GetBatchForSegment(ctx, "workspace1", int64(0), 1000).
		Return(nil, errors.New("db error"))

	completed, err := processor.Process(ctx, task, timeoutAt)
	assert.False(t, completed)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to fetch contact batch")
}

func TestSegmentBuildProcessor_Process_StateInitialization(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSegmentRepo := mocks.NewMockSegmentRepository(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockTaskRepo := mocks.NewMockTaskRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()

	processor := NewSegmentBuildProcessor(mockSegmentRepo, mockContactRepo, mockTaskRepo, mockWorkspaceRepo, mockLogger)

	ctx := context.Background()
	timeoutAt := time.Now().Add(5 * time.Minute)

	task := &domain.Task{
		ID:          "task1",
		WorkspaceID: "workspace1",
		Type:        "build_segment",
		State: &domain.TaskState{
			BuildSegment: &domain.BuildSegmentState{
				SegmentID: "segment1",
				// Version, BatchSize, StartedAt not set - should be initialized
			},
		},
	}

	segment := &domain.Segment{
		ID:      "segment1",
		Name:    "Test Segment",
		Status:  string(domain.SegmentStatusBuilding),
		Version: 5, // Current version
		Tree:    createValidSegmentTree(),
	}

	mockSegmentRepo.EXPECT().
		GetSegmentByID(ctx, "workspace1", "segment1").
		Return(segment, nil)

	mockSegmentRepo.EXPECT().
		UpdateSegment(ctx, "workspace1", gomock.Any()).
		Return(nil).
		Times(2)

	mockContactRepo.EXPECT().
		Count(ctx, "workspace1").
		Return(0, nil)

	mockContactRepo.EXPECT().
		GetBatchForSegment(ctx, "workspace1", int64(0), 1000).
		Return([]*domain.Contact{}, nil)

	mockSegmentRepo.EXPECT().
		RemoveOldMemberships(ctx, "workspace1", "segment1", int64(5)).
		Return(nil)

	completed, err := processor.Process(ctx, task, timeoutAt)
	assert.True(t, completed)
	assert.NoError(t, err)

	// Verify state was initialized
	assert.Equal(t, 1000, task.State.BuildSegment.BatchSize)
	assert.NotEmpty(t, task.State.BuildSegment.StartedAt)
	assert.Equal(t, int64(5), task.State.BuildSegment.Version) // Should use segment's version
}

func TestSegmentBuildProcessor_Process_RemoveMembershipsError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSegmentRepo := mocks.NewMockSegmentRepository(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockTaskRepo := mocks.NewMockTaskRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()

	processor := NewSegmentBuildProcessor(mockSegmentRepo, mockContactRepo, mockTaskRepo, mockWorkspaceRepo, mockLogger)

	ctx := context.Background()
	timeoutAt := time.Now().Add(5 * time.Minute)

	task := &domain.Task{
		ID:          "task1",
		WorkspaceID: "workspace1",
		Type:        "build_segment",
		State: &domain.TaskState{
			BuildSegment: &domain.BuildSegmentState{
				SegmentID: "segment1",
				Version:   1,
			},
		},
	}

	segment := &domain.Segment{
		ID:      "segment1",
		Name:    "Test Segment",
		Status:  string(domain.SegmentStatusBuilding),
		Version: 1,
		Tree:    createValidSegmentTree(),
	}

	mockSegmentRepo.EXPECT().
		GetSegmentByID(ctx, "workspace1", "segment1").
		Return(segment, nil)

	mockSegmentRepo.EXPECT().
		UpdateSegment(ctx, "workspace1", gomock.Any()).
		Return(nil).
		AnyTimes()

	mockContactRepo.EXPECT().
		Count(ctx, "workspace1").
		Return(0, nil)

	mockContactRepo.EXPECT().
		GetBatchForSegment(ctx, "workspace1", int64(0), 1000).
		Return([]*domain.Contact{}, nil)

	mockSegmentRepo.EXPECT().
		RemoveOldMemberships(ctx, "workspace1", "segment1", int64(1)).
		Return(errors.New("db error"))

	completed, err := processor.Process(ctx, task, timeoutAt)
	assert.False(t, completed)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to remove old memberships")
}

func TestSegmentBuildProcessor_Process_FinalStatusUpdateError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSegmentRepo := mocks.NewMockSegmentRepository(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockTaskRepo := mocks.NewMockTaskRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()

	processor := NewSegmentBuildProcessor(mockSegmentRepo, mockContactRepo, mockTaskRepo, mockWorkspaceRepo, mockLogger)

	ctx := context.Background()
	timeoutAt := time.Now().Add(5 * time.Minute)

	task := &domain.Task{
		ID:          "task1",
		WorkspaceID: "workspace1",
		Type:        "build_segment",
		State: &domain.TaskState{
			BuildSegment: &domain.BuildSegmentState{
				SegmentID: "segment1",
				Version:   1,
			},
		},
	}

	segment := &domain.Segment{
		ID:      "segment1",
		Name:    "Test Segment",
		Status:  string(domain.SegmentStatusBuilding),
		Version: 1,
		Tree:    createValidSegmentTree(),
	}

	mockSegmentRepo.EXPECT().
		GetSegmentByID(ctx, "workspace1", "segment1").
		Return(segment, nil)

	mockSegmentRepo.EXPECT().
		UpdateSegment(ctx, "workspace1", gomock.Any()).
		Return(nil).
		Times(1) // First update succeeds

	mockContactRepo.EXPECT().
		Count(ctx, "workspace1").
		Return(0, nil)

	mockContactRepo.EXPECT().
		GetBatchForSegment(ctx, "workspace1", int64(0), 1000).
		Return([]*domain.Contact{}, nil)

	mockSegmentRepo.EXPECT().
		RemoveOldMemberships(ctx, "workspace1", "segment1", int64(1)).
		Return(nil)

	mockSegmentRepo.EXPECT().
		UpdateSegment(ctx, "workspace1", gomock.Any()).
		DoAndReturn(func(ctx context.Context, workspaceID string, seg *domain.Segment) error {
			// Verify status is being set to active
			assert.Equal(t, string(domain.SegmentStatusActive), seg.Status)
			return errors.New("db error")
		})

	completed, err := processor.Process(ctx, task, timeoutAt)
	assert.False(t, completed)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to update segment status to active")
}

func TestSegmentBuildProcessor_SaveProgress(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSegmentRepo := mocks.NewMockSegmentRepository(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockTaskRepo := mocks.NewMockTaskRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	processor := NewSegmentBuildProcessor(mockSegmentRepo, mockContactRepo, mockTaskRepo, mockWorkspaceRepo, mockLogger)

	ctx := context.Background()

	t.Run("successful save", func(t *testing.T) {
		task := &domain.Task{
			ID:          "task1",
			WorkspaceID: "workspace1",
			Progress:    0.5,
			State: &domain.TaskState{
				BuildSegment: &domain.BuildSegmentState{
					SegmentID:      "segment1",
					MatchedCount:   50,
					ProcessedCount: 100,
				},
			},
		}

		mockTaskRepo.EXPECT().
			SaveState(ctx, "workspace1", "task1", 0.5, gomock.Any()).
			DoAndReturn(func(ctx context.Context, workspaceID, taskID string, progress float64, state *domain.TaskState) error {
				assert.Contains(t, state.Message, "50/100")
				return nil
			})

		err := processor.saveProgress(ctx, task, task.State.BuildSegment)
		assert.NoError(t, err)
		assert.Contains(t, task.State.Message, "Processing contacts")
	})

	t.Run("save error", func(t *testing.T) {
		task := &domain.Task{
			ID:          "task1",
			WorkspaceID: "workspace1",
			Progress:    0.5,
			State: &domain.TaskState{
				BuildSegment: &domain.BuildSegmentState{
					SegmentID:      "segment1",
					MatchedCount:   50,
					ProcessedCount: 100,
				},
			},
		}

		mockTaskRepo.EXPECT().
			SaveState(ctx, "workspace1", "task1", 0.5, gomock.Any()).
			Return(errors.New("db error"))

		err := processor.saveProgress(ctx, task, task.State.BuildSegment)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to save task state")
	})
}

func TestSegmentBuildProcessor_ExecuteSegmentQuery(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSegmentRepo := mocks.NewMockSegmentRepository(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockTaskRepo := mocks.NewMockTaskRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	processor := NewSegmentBuildProcessor(mockSegmentRepo, mockContactRepo, mockTaskRepo, mockWorkspaceRepo, mockLogger)

	ctx := context.Background()

	t.Run("connection error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, "workspace1").
			Return(nil, errors.New("connection failed"))

		rows, err := processor.executeSegmentQuery(ctx, "workspace1", "SELECT * FROM contacts", []interface{}{})
		assert.Nil(t, rows)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get workspace database connection")
	})

	// Note: We can't easily test the successful case without a real database connection
	// The method is now implemented and will be tested via integration tests
}
