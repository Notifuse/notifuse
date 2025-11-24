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

func createTestSegmentWithSQL(id, name string, version int64, status string) *domain.Segment {
	sql := "SELECT email FROM contacts WHERE email LIKE $1"
	return &domain.Segment{
		ID:            id,
		Name:          name,
		Status:        status,
		Version:       version,
		Tree:          createValidSegmentTree(),
		GeneratedSQL:  &sql,
		GeneratedArgs: domain.JSONArray{"%test%"},
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
	assert.Equal(t, 100, processor.batchSize)
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
		Return(segment, nil).
		AnyTimes() // Called at start and before each batch

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
		Return(segment, nil).
		AnyTimes() // Called at start and before each batch

	completed, err := processor.Process(ctx, task, timeoutAt)
	assert.False(t, completed)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "segment has no generated SQL")
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

	segment := createTestSegmentWithSQL("segment1", "Test Segment", 1, string(domain.SegmentStatusBuilding))

	mockSegmentRepo.EXPECT().
		GetSegmentByID(ctx, "workspace1", "segment1").
		Return(segment, nil).
		AnyTimes() // Called at start and before each batch

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

	segment := createTestSegmentWithSQL("segment1", "Test Segment", 1, string(domain.SegmentStatusBuilding))

	mockSegmentRepo.EXPECT().
		GetSegmentByID(ctx, "workspace1", "segment1").
		Return(segment, nil).
		AnyTimes() // Called at start and before each batch

	mockSegmentRepo.EXPECT().
		UpdateSegment(ctx, "workspace1", gomock.Any()).
		Return(nil).
		Times(1) // For final status update only

	mockContactRepo.EXPECT().
		Count(ctx, "workspace1").
		Return(0, nil)

	mockContactRepo.EXPECT().
		GetBatchForSegment(ctx, "workspace1", int64(0), 100).
		Return([]string{}, nil)

	// RemoveOldMemberships should NOT be called when there are 0 matches

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

	segment := createTestSegmentWithSQL("segment1", "Test Segment", 1, string(domain.SegmentStatusBuilding))

	mockSegmentRepo.EXPECT().
		GetSegmentByID(ctx, "workspace1", "segment1").
		Return(segment, nil).
		AnyTimes() // Called at start and before each batch

	mockSegmentRepo.EXPECT().
		UpdateSegment(ctx, "workspace1", gomock.Any()).
		Return(nil).
		AnyTimes()

	mockContactRepo.EXPECT().
		Count(ctx, "workspace1").
		Return(100, nil)

	mockContactRepo.EXPECT().
		GetBatchForSegment(ctx, "workspace1", int64(0), 100).
		Return(nil, errors.New("db error"))

	completed, err := processor.Process(ctx, task, timeoutAt)
	assert.False(t, completed)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to fetch email batch")
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

	segment := createTestSegmentWithSQL("segment1", "Test Segment", 5, string(domain.SegmentStatusBuilding)) // Current version

	mockSegmentRepo.EXPECT().
		GetSegmentByID(ctx, "workspace1", "segment1").
		Return(segment, nil).
		AnyTimes() // Called at start and before each batch

	mockSegmentRepo.EXPECT().
		UpdateSegment(ctx, "workspace1", gomock.Any()).
		Return(nil).
		Times(1) // For final status update only

	mockContactRepo.EXPECT().
		Count(ctx, "workspace1").
		Return(0, nil)

	mockContactRepo.EXPECT().
		GetBatchForSegment(ctx, "workspace1", int64(0), 100).
		Return([]string{}, nil)

	// RemoveOldMemberships should NOT be called when there are 0 matches

	completed, err := processor.Process(ctx, task, timeoutAt)
	assert.True(t, completed)
	assert.NoError(t, err)

	// Verify state was initialized
	assert.Equal(t, 100, task.State.BuildSegment.BatchSize)
	assert.NotEmpty(t, task.State.BuildSegment.StartedAt)
	assert.Equal(t, int64(5), task.State.BuildSegment.Version) // Should use segment's version
}

func TestSegmentBuildProcessor_Process_ZeroMatchedContacts(t *testing.T) {
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

	segment := createTestSegmentWithSQL("segment1", "Test Segment", 1, string(domain.SegmentStatusBuilding))

	mockSegmentRepo.EXPECT().
		GetSegmentByID(ctx, "workspace1", "segment1").
		Return(segment, nil).
		AnyTimes() // Called at start and before each batch

	mockSegmentRepo.EXPECT().
		UpdateSegment(ctx, "workspace1", gomock.Any()).
		Return(nil).
		Times(1) // For final status update only

	mockContactRepo.EXPECT().
		Count(ctx, "workspace1").
		Return(100, nil)

	mockContactRepo.EXPECT().
		GetBatchForSegment(ctx, "workspace1", int64(0), 100).
		Return([]string{}, nil)

	// RemoveOldMemberships should NOT be called when there are 0 matches

	completed, err := processor.Process(ctx, task, timeoutAt)
	assert.True(t, completed)
	assert.NoError(t, err)

	// Verify matched count is 0
	assert.Equal(t, 0, task.State.BuildSegment.MatchedCount)
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

	segment := createTestSegmentWithSQL("segment1", "Test Segment", 1, string(domain.SegmentStatusBuilding))

	// Create mock DB connection
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer func() { _ = db.Close() }()

	// Setup mock to return one matching contact
	rows := sqlmock.NewRows([]string{"email"}).AddRow("test@test.com")
	mock.ExpectQuery("SELECT email FROM contacts").WillReturnRows(rows)

	mockSegmentRepo.EXPECT().
		GetSegmentByID(ctx, "workspace1", "segment1").
		Return(segment, nil).
		AnyTimes() // Called at start and before each batch

	mockSegmentRepo.EXPECT().
		UpdateSegment(ctx, "workspace1", gomock.Any()).
		Return(nil).
		AnyTimes()

	mockContactRepo.EXPECT().
		Count(ctx, "workspace1").
		Return(1, nil)

	mockContactRepo.EXPECT().
		GetBatchForSegment(ctx, "workspace1", int64(0), 100).
		Return([]string{"test@test.com"}, nil)

	mockContactRepo.EXPECT().
		GetBatchForSegment(ctx, "workspace1", int64(1), 100).
		Return([]string{}, nil)

	mockWorkspaceRepo.EXPECT().
		GetConnection(ctx, "workspace1").
		Return(db, nil)

	mockSegmentRepo.EXPECT().
		AddContactToSegment(ctx, "workspace1", "test@test.com", "segment1", int64(1)).
		Return(nil)

	mockTaskRepo.EXPECT().
		SaveState(ctx, "workspace1", "task1", gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	mockSegmentRepo.EXPECT().
		RemoveOldMemberships(ctx, "workspace1", "segment1", int64(1)).
		Return(errors.New("db error"))

	completed, err := processor.Process(ctx, task, timeoutAt)
	assert.False(t, completed)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to remove old memberships")

	// Verify all expectations were met
	assert.NoError(t, mock.ExpectationsWereMet())
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

	segment := createTestSegmentWithSQL("segment1", "Test Segment", 1, string(domain.SegmentStatusBuilding))

	// Create mock DB connection
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer func() { _ = db.Close() }()

	// Setup mock to return one matching contact
	rows := sqlmock.NewRows([]string{"email"}).AddRow("test@test.com")
	mock.ExpectQuery("SELECT email FROM contacts").WillReturnRows(rows)

	mockSegmentRepo.EXPECT().
		GetSegmentByID(ctx, "workspace1", "segment1").
		Return(segment, nil).
		AnyTimes() // Called at start and before each batch

	mockContactRepo.EXPECT().
		Count(ctx, "workspace1").
		Return(1, nil)

	mockContactRepo.EXPECT().
		GetBatchForSegment(ctx, "workspace1", int64(0), 100).
		Return([]string{"test@test.com"}, nil)

	mockContactRepo.EXPECT().
		GetBatchForSegment(ctx, "workspace1", int64(1), 100).
		Return([]string{}, nil)

	mockWorkspaceRepo.EXPECT().
		GetConnection(ctx, "workspace1").
		Return(db, nil)

	mockSegmentRepo.EXPECT().
		AddContactToSegment(ctx, "workspace1", "test@test.com", "segment1", int64(1)).
		Return(nil)

	mockTaskRepo.EXPECT().
		SaveState(ctx, "workspace1", "task1", gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

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

	// Verify all expectations were met
	assert.NoError(t, mock.ExpectationsWereMet())
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

func TestSegmentBuildProcessor_Process_VersionChanged(t *testing.T) {
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
				Version:   1, // Task is building version 1
			},
		},
	}

	initialSegment := createTestSegmentWithSQL("segment1", "Test Segment", 1, string(domain.SegmentStatusBuilding))
	updatedSegment := createTestSegmentWithSQL("segment1", "Test Segment", 2, string(domain.SegmentStatusBuilding)) // Version changed to 2

	mockSegmentRepo.EXPECT().
		GetSegmentByID(ctx, "workspace1", "segment1").
		Return(initialSegment, nil)

	mockSegmentRepo.EXPECT().
		UpdateSegment(ctx, "workspace1", gomock.Any()).
		Return(nil).
		AnyTimes()

	mockContactRepo.EXPECT().
		Count(ctx, "workspace1").
		Return(100, nil)

	// When checking version before batch processing, return updated segment
	mockSegmentRepo.EXPECT().
		GetSegmentByID(ctx, "workspace1", "segment1").
		Return(updatedSegment, nil)

	mockSegmentRepo.EXPECT().
		RemoveOldMemberships(ctx, "workspace1", "segment1", int64(2)).
		Return(nil)

	completed, err := processor.Process(ctx, task, timeoutAt)
	assert.True(t, completed) // Task is completed (superseded)
	assert.NoError(t, err)
}

func TestSegmentBuildProcessor_Process_VersionChangedCleanupFails(t *testing.T) {
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

	initialSegment := createTestSegmentWithSQL("segment1", "Test Segment", 1, string(domain.SegmentStatusBuilding))
	updatedSegment := createTestSegmentWithSQL("segment1", "Test Segment", 2, string(domain.SegmentStatusBuilding))

	mockSegmentRepo.EXPECT().
		GetSegmentByID(ctx, "workspace1", "segment1").
		Return(initialSegment, nil)

	mockSegmentRepo.EXPECT().
		UpdateSegment(ctx, "workspace1", gomock.Any()).
		Return(nil).
		AnyTimes()

	mockContactRepo.EXPECT().
		Count(ctx, "workspace1").
		Return(100, nil)

	mockSegmentRepo.EXPECT().
		GetSegmentByID(ctx, "workspace1", "segment1").
		Return(updatedSegment, nil)

	// Cleanup fails but should not fail the task
	mockSegmentRepo.EXPECT().
		RemoveOldMemberships(ctx, "workspace1", "segment1", int64(2)).
		Return(errors.New("cleanup failed"))

	completed, err := processor.Process(ctx, task, timeoutAt)
	assert.True(t, completed) // Task is still completed despite cleanup failure
	assert.NoError(t, err)
}

func TestSegmentBuildProcessor_Process_Timeout(t *testing.T) {
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
	// Set timeout very close to now to trigger timeout
	timeoutAt := time.Now().Add(1 * time.Second)

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

	segment := createTestSegmentWithSQL("segment1", "Test Segment", 1, string(domain.SegmentStatusBuilding))

	mockSegmentRepo.EXPECT().
		GetSegmentByID(ctx, "workspace1", "segment1").
		Return(segment, nil).
		AnyTimes()

	mockSegmentRepo.EXPECT().
		UpdateSegment(ctx, "workspace1", gomock.Any()).
		Return(nil).
		AnyTimes()

	mockContactRepo.EXPECT().
		Count(ctx, "workspace1").
		Return(100, nil)

	mockTaskRepo.EXPECT().
		SaveState(ctx, "workspace1", "task1", gomock.Any(), gomock.Any()).
		Return(nil)

	completed, err := processor.Process(ctx, task, timeoutAt)
	assert.False(t, completed) // Task not completed due to timeout
	assert.NoError(t, err)
}

func TestSegmentBuildProcessor_Process_SuccessfulCompletion(t *testing.T) {
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

	segment := createTestSegmentWithSQL("segment1", "Test Segment", 1, string(domain.SegmentStatusBuilding))

	// Create mock DB connection
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer func() { _ = db.Close() }()

	// Setup mock to return one matching contact
	rows := sqlmock.NewRows([]string{"email"}).AddRow("test@test.com")
	mock.ExpectQuery("SELECT email FROM contacts").WillReturnRows(rows)

	mockSegmentRepo.EXPECT().
		GetSegmentByID(ctx, "workspace1", "segment1").
		Return(segment, nil).
		AnyTimes() // Initial load and refetch for each batch

	mockSegmentRepo.EXPECT().
		UpdateSegment(ctx, "workspace1", gomock.Any()).
		Return(nil).
		AnyTimes()

	mockContactRepo.EXPECT().
		Count(ctx, "workspace1").
		Return(1, nil)

	// First batch returns one email
	mockContactRepo.EXPECT().
		GetBatchForSegment(ctx, "workspace1", int64(0), 100).
		Return([]string{"test@test.com"}, nil)

	// Second batch returns empty (done)
	mockContactRepo.EXPECT().
		GetBatchForSegment(ctx, "workspace1", int64(1), 100).
		Return([]string{}, nil)

	mockWorkspaceRepo.EXPECT().
		GetConnection(ctx, "workspace1").
		Return(db, nil)

	mockSegmentRepo.EXPECT().
		AddContactToSegment(ctx, "workspace1", "test@test.com", "segment1", int64(1)).
		Return(nil)

	mockTaskRepo.EXPECT().
		SaveState(ctx, "workspace1", "task1", gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	mockSegmentRepo.EXPECT().
		RemoveOldMemberships(ctx, "workspace1", "segment1", int64(1)).
		Return(nil)

	completed, err := processor.Process(ctx, task, timeoutAt)
	assert.True(t, completed)
	assert.NoError(t, err)

	// Verify all expectations were met
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSegmentBuildProcessor_ProcessBatch_QueryError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSegmentRepo := mocks.NewMockSegmentRepository(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockTaskRepo := mocks.NewMockTaskRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()

	processor := NewSegmentBuildProcessor(mockSegmentRepo, mockContactRepo, mockTaskRepo, mockWorkspaceRepo, mockLogger)

	ctx := context.Background()

	// Create mock DB that will fail on query
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectQuery("SELECT email FROM contacts").WillReturnError(errors.New("query failed"))

	mockWorkspaceRepo.EXPECT().
		GetConnection(ctx, "workspace1").
		Return(db, nil)

	state := &domain.BuildSegmentState{
		SegmentID: "segment1",
		Version:   1,
	}

	err = processor.processBatch(ctx, "workspace1", "SELECT email FROM contacts WHERE email LIKE $1", []interface{}{"%test%"}, []string{"test@test.com"}, state)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to execute segment query")
	assert.NoError(t, mock.ExpectationsWereMet())
}
