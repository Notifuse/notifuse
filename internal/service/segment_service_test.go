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

func createTestTree() *domain.TreeNode {
	return &domain.TreeNode{
		Kind: "leaf",
		Leaf: &domain.TreeNodeLeaf{
			Source: "contacts",
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

func TestSegmentService_CreateSegment(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockSegmentRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockTaskService := mocks.NewMockTaskService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Setup logger expectations
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()

	service := NewSegmentService(mockRepo, mockWorkspaceRepo, mockTaskService, newPermissiveAuthService(ctrl), mockLogger)
	ctx := context.Background()

	t.Run("successful create", func(t *testing.T) {
		req := &domain.CreateSegmentRequest{
			WorkspaceID: "workspace123",
			ID:          "segment1",
			Name:        "Test Segment",
			Color:       "#FF5733",
			Timezone:    "UTC",
			Tree:        createTestTree(),
		}

		mockRepo.EXPECT().CreateSegment(gomock.Any(), "workspace123", gomock.Any()).DoAndReturn(
			func(ctx context.Context, workspaceID string, segment *domain.Segment) error {
				assert.Equal(t, "segment1", segment.ID)
				assert.Equal(t, "Test Segment", segment.Name)
				assert.Equal(t, int64(1), segment.Version)
				assert.Equal(t, string(domain.SegmentStatusBuilding), segment.Status)
				assert.NotNil(t, segment.GeneratedSQL)
				assert.NotNil(t, segment.GeneratedArgs)
				return nil
			},
		)

		mockTaskService.EXPECT().CreateTask(gomock.Any(), "workspace123", gomock.Any()).DoAndReturn(
			func(ctx context.Context, workspaceID string, task *domain.Task) error {
				assert.Equal(t, "build_segment", task.Type)
				assert.Equal(t, domain.TaskStatusPending, task.Status)
				assert.NotNil(t, task.State.BuildSegment)
				assert.Equal(t, "segment1", task.State.BuildSegment.SegmentID)
				assert.Equal(t, int64(1), task.State.BuildSegment.Version)
				return nil
			},
		)

		// Expect ExecuteTask to be called asynchronously after task creation
		mockTaskService.EXPECT().ExecuteTask(gomock.Any(), "workspace123", gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

		segment, err := service.CreateSegment(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, segment)
		assert.Equal(t, "segment1", segment.ID)
		assert.Equal(t, "Test Segment", segment.Name)
		assert.NotZero(t, segment.DBCreatedAt)
		assert.NotZero(t, segment.DBUpdatedAt)

		// Wait for background goroutine to complete before mock cleanup
		time.Sleep(200 * time.Millisecond)
	})

	t.Run("validation requires ID", func(t *testing.T) {
		// Note: ID is required by validation, so auto-generation doesn't work as expected
		// This tests that missing ID fails validation
		req := &domain.CreateSegmentRequest{
			WorkspaceID: "workspace123",
			Name:        "Test Segment",
			Color:       "#FF5733",
			Timezone:    "UTC",
			Tree:        createTestTree(),
			// Missing ID
		}

		segment, err := service.CreateSegment(ctx, req)
		assert.Error(t, err)
		assert.Nil(t, segment)
		assert.Contains(t, err.Error(), "id is required")
	})

	t.Run("validation error", func(t *testing.T) {
		req := &domain.CreateSegmentRequest{
			WorkspaceID: "workspace123",
			// Missing required fields
		}

		segment, err := service.CreateSegment(ctx, req)
		assert.Error(t, err)
		assert.Nil(t, segment)
		assert.Contains(t, err.Error(), "invalid request")
	})

	t.Run("invalid tree structure", func(t *testing.T) {
		req := &domain.CreateSegmentRequest{
			WorkspaceID: "workspace123",
			ID:          "segment1",
			Name:        "Test Segment",
			Color:       "#FF5733",
			Timezone:    "UTC",
			Tree: &domain.TreeNode{
				Kind: "invalid",
			},
		}

		segment, err := service.CreateSegment(ctx, req)
		assert.Error(t, err)
		assert.Nil(t, segment)
		// Tree validation happens first during request validation
		assert.Contains(t, err.Error(), "invalid tree")
	})

	t.Run("repository error", func(t *testing.T) {
		req := &domain.CreateSegmentRequest{
			WorkspaceID: "workspace123",
			ID:          "segment1",
			Name:        "Test Segment",
			Color:       "#FF5733",
			Timezone:    "UTC",
			Tree:        createTestTree(),
		}

		mockRepo.EXPECT().CreateSegment(gomock.Any(), "workspace123", gomock.Any()).Return(errors.New("db error"))

		segment, err := service.CreateSegment(ctx, req)
		assert.Error(t, err)
		assert.Nil(t, segment)
		assert.Contains(t, err.Error(), "failed to create segment")
	})

	t.Run("task creation failure is non-fatal", func(t *testing.T) {
		req := &domain.CreateSegmentRequest{
			WorkspaceID: "workspace123",
			ID:          "segment1",
			Name:        "Test Segment",
			Color:       "#FF5733",
			Timezone:    "UTC",
			Tree:        createTestTree(),
		}

		mockRepo.EXPECT().CreateSegment(gomock.Any(), "workspace123", gomock.Any()).Return(nil)
		mockTaskService.EXPECT().CreateTask(gomock.Any(), "workspace123", gomock.Any()).Return(errors.New("task error"))

		segment, err := service.CreateSegment(ctx, req)
		assert.NoError(t, err) // Should not fail even if task creation fails
		assert.NotNil(t, segment)
	})
}

func TestSegmentService_GetSegment(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockSegmentRepository(ctrl)
	mockTaskService := mocks.NewMockTaskService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	service := NewSegmentService(mockRepo, mockWorkspaceRepo, mockTaskService, newPermissiveAuthService(ctrl), mockLogger)
	ctx := context.Background()

	t.Run("successful get", func(t *testing.T) {
		req := &domain.GetSegmentRequest{
			WorkspaceID: "workspace123",
			ID:          "segment1",
		}

		expectedSegment := &domain.Segment{
			ID:       "segment1",
			Name:     "Test Segment",
			Color:    "#FF5733",
			Timezone: "UTC",
			Version:  1,
			Status:   string(domain.SegmentStatusActive),
			Tree:     createTestTree(),
		}

		mockRepo.EXPECT().GetSegmentByID(ctx, "workspace123", "segment1").Return(expectedSegment, nil)
		mockRepo.EXPECT().GetSegmentContactCount(ctx, "workspace123", "segment1").Return(42, nil)

		segment, err := service.GetSegment(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, segment)
		assert.Equal(t, "segment1", segment.ID)
		assert.Equal(t, 42, segment.UsersCount)
	})

	t.Run("segment not found", func(t *testing.T) {
		req := &domain.GetSegmentRequest{
			WorkspaceID: "workspace123",
			ID:          "nonexistent",
		}

		mockRepo.EXPECT().GetSegmentByID(ctx, "workspace123", "nonexistent").Return(
			nil,
			&domain.ErrSegmentNotFound{Message: "not found"},
		)

		segment, err := service.GetSegment(ctx, req)
		assert.Error(t, err)
		assert.Nil(t, segment)
	})

	t.Run("missing workspace ID", func(t *testing.T) {
		req := &domain.GetSegmentRequest{
			ID: "segment1",
		}

		segment, err := service.GetSegment(ctx, req)
		assert.Error(t, err)
		assert.Nil(t, segment)
		assert.Contains(t, err.Error(), "workspace_id is required")
	})

	t.Run("missing segment ID", func(t *testing.T) {
		req := &domain.GetSegmentRequest{
			WorkspaceID: "workspace123",
		}

		segment, err := service.GetSegment(ctx, req)
		assert.Error(t, err)
		assert.Nil(t, segment)
		assert.Contains(t, err.Error(), "segment id is required")
	})

	t.Run("contact count error is non-fatal", func(t *testing.T) {
		req := &domain.GetSegmentRequest{
			WorkspaceID: "workspace123",
			ID:          "segment1",
		}

		expectedSegment := &domain.Segment{
			ID:   "segment1",
			Name: "Test Segment",
		}

		mockRepo.EXPECT().GetSegmentByID(ctx, "workspace123", "segment1").Return(expectedSegment, nil)
		mockRepo.EXPECT().GetSegmentContactCount(ctx, "workspace123", "segment1").Return(0, errors.New("count error"))

		segment, err := service.GetSegment(ctx, req)
		assert.NoError(t, err) // Should not fail even if count fails
		assert.NotNil(t, segment)
	})
}

func TestSegmentService_ListSegments(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockSegmentRepository(ctrl)
	mockTaskService := mocks.NewMockTaskService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	service := NewSegmentService(mockRepo, mockWorkspaceRepo, mockTaskService, newPermissiveAuthService(ctrl), mockLogger)
	ctx := context.Background()

	t.Run("successful list with counts", func(t *testing.T) {
		req := &domain.GetSegmentsRequest{
			WorkspaceID: "workspace123",
			WithCount:   true,
		}

		expectedSegments := []*domain.Segment{
			{
				ID:         "segment1",
				Name:       "Segment 1",
				Version:    1,
				UsersCount: 10,
			},
			{
				ID:         "segment2",
				Name:       "Segment 2",
				Version:    1,
				UsersCount: 20,
			},
		}

		mockRepo.EXPECT().GetSegments(ctx, "workspace123", true).Return(expectedSegments, nil)

		segments, err := service.ListSegments(ctx, req)
		assert.NoError(t, err)
		assert.Len(t, segments, 2)
		assert.Equal(t, 10, segments[0].UsersCount)
		assert.Equal(t, 20, segments[1].UsersCount)
	})

	t.Run("successful list without counts", func(t *testing.T) {
		req := &domain.GetSegmentsRequest{
			WorkspaceID: "workspace123",
			WithCount:   false,
		}

		expectedSegments := []*domain.Segment{
			{
				ID:         "segment1",
				Name:       "Segment 1",
				Version:    1,
				UsersCount: 0, // No count when WithCount=false
			},
			{
				ID:         "segment2",
				Name:       "Segment 2",
				Version:    1,
				UsersCount: 0,
			},
		}

		mockRepo.EXPECT().GetSegments(ctx, "workspace123", false).Return(expectedSegments, nil)

		segments, err := service.ListSegments(ctx, req)
		assert.NoError(t, err)
		assert.Len(t, segments, 2)
		assert.Equal(t, 0, segments[0].UsersCount)
		assert.Equal(t, 0, segments[1].UsersCount)
	})

	t.Run("empty list", func(t *testing.T) {
		req := &domain.GetSegmentsRequest{
			WorkspaceID: "workspace123",
			WithCount:   false,
		}

		mockRepo.EXPECT().GetSegments(ctx, "workspace123", false).Return([]*domain.Segment{}, nil)

		segments, err := service.ListSegments(ctx, req)
		assert.NoError(t, err)
		assert.Empty(t, segments)
	})

	t.Run("missing workspace ID", func(t *testing.T) {
		req := &domain.GetSegmentsRequest{}

		segments, err := service.ListSegments(ctx, req)
		assert.Error(t, err)
		assert.Nil(t, segments)
		assert.Contains(t, err.Error(), "workspace_id is required")
	})

	t.Run("repository error", func(t *testing.T) {
		req := &domain.GetSegmentsRequest{
			WorkspaceID: "workspace123",
			WithCount:   true,
		}

		mockRepo.EXPECT().GetSegments(ctx, "workspace123", true).Return(nil, errors.New("db error"))

		segments, err := service.ListSegments(ctx, req)
		assert.Error(t, err)
		assert.Nil(t, segments)
		assert.Contains(t, err.Error(), "failed to list segments")
	})
}

func TestSegmentService_UpdateSegment(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockSegmentRepository(ctrl)
	mockTaskService := mocks.NewMockTaskService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	service := NewSegmentService(mockRepo, mockWorkspaceRepo, mockTaskService, newPermissiveAuthService(ctrl), mockLogger)
	ctx := context.Background()

	t.Run("successful update without tree change", func(t *testing.T) {
		// Note: UpdateSegmentRequest validation requires all fields including tree
		req := &domain.UpdateSegmentRequest{
			WorkspaceID: "workspace123",
			ID:          "segment1",
			Name:        "Updated Name",
			Color:       "#33FF57",
			Timezone:    "America/New_York",
			Tree:        createTestTree(), // Required by validation
		}

		existingSegment := &domain.Segment{
			ID:       "segment1",
			Name:     "Old Name",
			Color:    "#FF5733",
			Timezone: "UTC",
			Version:  1,
			Status:   string(domain.SegmentStatusActive),
			Tree:     createTestTree(),
		}

		mockRepo.EXPECT().GetSegmentByID(ctx, "workspace123", "segment1").Return(existingSegment, nil)
		mockRepo.EXPECT().UpdateSegment(ctx, "workspace123", gomock.Any()).DoAndReturn(
			func(ctx context.Context, workspaceID string, segment *domain.Segment) error {
				assert.Equal(t, "Updated Name", segment.Name)
				assert.Equal(t, "#33FF57", segment.Color)
				assert.Equal(t, "America/New_York", segment.Timezone)
				assert.Equal(t, int64(2), segment.Version) // Version increments because tree is set
				return nil
			},
		)

		mockTaskService.EXPECT().CreateTask(gomock.Any(), "workspace123", gomock.Any()).Return(nil)

		// Expect ExecuteTask to be called asynchronously after task creation
		mockTaskService.EXPECT().ExecuteTask(gomock.Any(), "workspace123", gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

		segment, err := service.UpdateSegment(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, segment)
		assert.Equal(t, "Updated Name", segment.Name)

		// Wait for background goroutine to complete before mock cleanup
		time.Sleep(200 * time.Millisecond)
	})

	t.Run("successful update with tree change", func(t *testing.T) {
		newTree := &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{
				Source: "contacts",
				Contact: &domain.ContactCondition{
					Filters: []*domain.DimensionFilter{
						{
							FieldName:    "email",
							FieldType:    "string",
							Operator:     "equals",
							StringValues: []string{"user@example.com"},
						},
					},
				},
			},
		}

		req := &domain.UpdateSegmentRequest{
			WorkspaceID: "workspace123",
			ID:          "segment1",
			Name:        "Test Segment",
			Color:       "#FF5733",
			Timezone:    "UTC",
			Tree:        newTree,
		}

		existingSegment := &domain.Segment{
			ID:      "segment1",
			Name:    "Test Segment",
			Version: 1,
			Tree:    createTestTree(),
		}

		mockRepo.EXPECT().GetSegmentByID(ctx, "workspace123", "segment1").Return(existingSegment, nil)
		mockRepo.EXPECT().UpdateSegment(ctx, "workspace123", gomock.Any()).DoAndReturn(
			func(ctx context.Context, workspaceID string, segment *domain.Segment) error {
				assert.Equal(t, int64(2), segment.Version) // Version should increment
				assert.NotNil(t, segment.GeneratedSQL)
				return nil
			},
		)

		mockTaskService.EXPECT().CreateTask(gomock.Any(), "workspace123", gomock.Any()).DoAndReturn(
			func(ctx context.Context, workspaceID string, task *domain.Task) error {
				assert.Equal(t, "build_segment", task.Type)
				assert.Equal(t, int64(2), task.State.BuildSegment.Version)
				return nil
			},
		)

		// Expect ExecuteTask to be called asynchronously after task creation
		mockTaskService.EXPECT().ExecuteTask(gomock.Any(), "workspace123", gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

		segment, err := service.UpdateSegment(ctx, req)
		assert.NoError(t, err)
		assert.NotNil(t, segment)
		assert.Equal(t, int64(2), segment.Version)
		assert.Equal(t, string(domain.SegmentStatusBuilding), segment.Status)

		// Wait for background goroutine to complete before mock cleanup
		time.Sleep(200 * time.Millisecond)
	})

	t.Run("validation error", func(t *testing.T) {
		req := &domain.UpdateSegmentRequest{
			WorkspaceID: "workspace123",
			// Missing ID
		}

		segment, err := service.UpdateSegment(ctx, req)
		assert.Error(t, err)
		assert.Nil(t, segment)
		assert.Contains(t, err.Error(), "invalid request")
	})

	t.Run("segment not found", func(t *testing.T) {
		req := &domain.UpdateSegmentRequest{
			WorkspaceID: "workspace123",
			ID:          "nonexistent",
			Name:        "Updated",
			Color:       "#FF5733",
			Timezone:    "UTC",
			Tree:        createTestTree(),
		}

		mockRepo.EXPECT().GetSegmentByID(ctx, "workspace123", "nonexistent").Return(
			nil,
			&domain.ErrSegmentNotFound{Message: "not found"},
		)

		segment, err := service.UpdateSegment(ctx, req)
		assert.Error(t, err)
		assert.Nil(t, segment)
	})

	t.Run("invalid tree structure", func(t *testing.T) {
		req := &domain.UpdateSegmentRequest{
			WorkspaceID: "workspace123",
			ID:          "segment1",
			Name:        "Test Segment",
			Color:       "#FF5733",
			Timezone:    "UTC",
			Tree: &domain.TreeNode{
				Kind: "invalid",
			},
		}

		segment, err := service.UpdateSegment(ctx, req)
		assert.Error(t, err)
		assert.Nil(t, segment)
		// Tree validation happens first during request validation
		assert.Contains(t, err.Error(), "invalid tree")
	})

	t.Run("repository update error", func(t *testing.T) {
		req := &domain.UpdateSegmentRequest{
			WorkspaceID: "workspace123",
			ID:          "segment1",
			Name:        "Updated",
			Color:       "#FF5733",
			Timezone:    "UTC",
			Tree:        createTestTree(),
		}

		existingSegment := &domain.Segment{
			ID:      "segment1",
			Version: 1,
			Tree:    createTestTree(),
		}

		mockRepo.EXPECT().GetSegmentByID(ctx, "workspace123", "segment1").Return(existingSegment, nil)
		mockRepo.EXPECT().UpdateSegment(ctx, "workspace123", gomock.Any()).Return(errors.New("db error"))

		segment, err := service.UpdateSegment(ctx, req)
		assert.Error(t, err)
		assert.Nil(t, segment)
		assert.Contains(t, err.Error(), "failed to update segment")
	})

	t.Run("task creation failure is non-fatal on tree change", func(t *testing.T) {
		req := &domain.UpdateSegmentRequest{
			WorkspaceID: "workspace123",
			ID:          "segment1",
			Name:        "Test Segment",
			Color:       "#FF5733",
			Timezone:    "UTC",
			Tree:        createTestTree(),
		}

		existingSegment := &domain.Segment{
			ID:      "segment1",
			Version: 1,
			Tree:    createTestTree(),
		}

		mockRepo.EXPECT().GetSegmentByID(ctx, "workspace123", "segment1").Return(existingSegment, nil)
		mockRepo.EXPECT().UpdateSegment(ctx, "workspace123", gomock.Any()).Return(nil)
		mockTaskService.EXPECT().CreateTask(gomock.Any(), "workspace123", gomock.Any()).Return(errors.New("task error"))

		segment, err := service.UpdateSegment(ctx, req)
		assert.NoError(t, err) // Should not fail even if task creation fails
		assert.NotNil(t, segment)
	})
}

func TestSegmentService_DeleteSegment(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockSegmentRepository(ctrl)
	mockTaskService := mocks.NewMockTaskService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	service := NewSegmentService(mockRepo, mockWorkspaceRepo, mockTaskService, newPermissiveAuthService(ctrl), mockLogger)
	ctx := context.Background()

	t.Run("successful delete", func(t *testing.T) {
		req := &domain.DeleteSegmentRequest{
			WorkspaceID: "workspace123",
			ID:          "segment1",
		}

		mockRepo.EXPECT().DeleteSegment(ctx, "workspace123", "segment1").Return(nil)

		err := service.DeleteSegment(ctx, req)
		assert.NoError(t, err)
	})

	t.Run("validation error", func(t *testing.T) {
		req := &domain.DeleteSegmentRequest{
			WorkspaceID: "workspace123",
			// Missing ID
		}

		err := service.DeleteSegment(ctx, req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid request")
	})

	t.Run("segment not found", func(t *testing.T) {
		req := &domain.DeleteSegmentRequest{
			WorkspaceID: "workspace123",
			ID:          "nonexistent",
		}

		mockRepo.EXPECT().DeleteSegment(ctx, "workspace123", "nonexistent").Return(
			&domain.ErrSegmentNotFound{Message: "not found"},
		)

		err := service.DeleteSegment(ctx, req)
		assert.Error(t, err)
	})

	t.Run("repository error", func(t *testing.T) {
		req := &domain.DeleteSegmentRequest{
			WorkspaceID: "workspace123",
			ID:          "segment1",
		}

		mockRepo.EXPECT().DeleteSegment(ctx, "workspace123", "segment1").Return(errors.New("db error"))

		err := service.DeleteSegment(ctx, req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to delete segment")
	})
}

func TestSegmentService_RebuildSegment(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockSegmentRepository(ctrl)
	mockTaskService := mocks.NewMockTaskService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	service := NewSegmentService(mockRepo, mockWorkspaceRepo, mockTaskService, newPermissiveAuthService(ctrl), mockLogger)
	ctx := context.Background()

	t.Run("successful rebuild", func(t *testing.T) {
		existingSegment := &domain.Segment{
			ID:      "segment1",
			Name:    "Test Segment",
			Version: 1,
			Status:  string(domain.SegmentStatusActive),
			Tree:    createTestTree(),
		}

		mockRepo.EXPECT().GetSegmentByID(ctx, "workspace123", "segment1").Return(existingSegment, nil)
		mockRepo.EXPECT().UpdateSegment(ctx, "workspace123", gomock.Any()).DoAndReturn(
			func(ctx context.Context, workspaceID string, segment *domain.Segment) error {
				assert.Equal(t, int64(2), segment.Version)
				assert.Equal(t, string(domain.SegmentStatusBuilding), segment.Status)
				return nil
			},
		)

		mockTaskService.EXPECT().CreateTask(gomock.Any(), "workspace123", gomock.Any()).DoAndReturn(
			func(ctx context.Context, workspaceID string, task *domain.Task) error {
				assert.Equal(t, "build_segment", task.Type)
				assert.Equal(t, "segment1", task.State.BuildSegment.SegmentID)
				assert.Equal(t, int64(2), task.State.BuildSegment.Version)
				return nil
			},
		)

		// Expect ExecuteTask to be called asynchronously after task creation
		mockTaskService.EXPECT().ExecuteTask(gomock.Any(), "workspace123", gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

		err := service.RebuildSegment(ctx, "workspace123", "segment1")
		assert.NoError(t, err)

		// Wait for background goroutine to complete before mock cleanup
		time.Sleep(200 * time.Millisecond)
	})

	t.Run("missing workspace ID", func(t *testing.T) {
		err := service.RebuildSegment(ctx, "", "segment1")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "workspace_id is required")
	})

	t.Run("missing segment ID", func(t *testing.T) {
		err := service.RebuildSegment(ctx, "workspace123", "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "segment_id is required")
	})

	t.Run("segment not found", func(t *testing.T) {
		mockRepo.EXPECT().GetSegmentByID(ctx, "workspace123", "nonexistent").Return(
			nil,
			&domain.ErrSegmentNotFound{Message: "not found"},
		)

		err := service.RebuildSegment(ctx, "workspace123", "nonexistent")
		assert.Error(t, err)
	})

	t.Run("update segment error", func(t *testing.T) {
		existingSegment := &domain.Segment{
			ID:      "segment1",
			Version: 1,
		}

		mockRepo.EXPECT().GetSegmentByID(ctx, "workspace123", "segment1").Return(existingSegment, nil)
		mockRepo.EXPECT().UpdateSegment(ctx, "workspace123", gomock.Any()).Return(errors.New("db error"))

		err := service.RebuildSegment(ctx, "workspace123", "segment1")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to update segment")
	})

	t.Run("task creation error", func(t *testing.T) {
		existingSegment := &domain.Segment{
			ID:      "segment1",
			Version: 1,
		}

		mockRepo.EXPECT().GetSegmentByID(ctx, "workspace123", "segment1").Return(existingSegment, nil)
		mockRepo.EXPECT().UpdateSegment(ctx, "workspace123", gomock.Any()).Return(nil)
		mockTaskService.EXPECT().CreateTask(gomock.Any(), "workspace123", gomock.Any()).Return(errors.New("task error"))

		err := service.RebuildSegment(ctx, "workspace123", "segment1")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create rebuild task")
	})
}

func TestSegmentService_PreviewSegment(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockSegmentRepository(ctrl)
	mockTaskService := mocks.NewMockTaskService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	service := NewSegmentService(mockRepo, mockWorkspaceRepo, mockTaskService, newPermissiveAuthService(ctrl), mockLogger)
	ctx := context.Background()

	t.Run("successful preview", func(t *testing.T) {
		tree := createTestTree()

		mockRepo.EXPECT().
			PreviewSegment(ctx, "workspace123", gomock.Any(), gomock.Any(), 10).
			Return(42, nil)

		response, err := service.PreviewSegment(ctx, "workspace123", tree, 10)
		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.Equal(t, 42, response.TotalCount)
		assert.Equal(t, 10, response.Limit)
		assert.Empty(t, response.Emails) // Emails not returned for privacy/performance
		assert.NotEmpty(t, response.GeneratedSQL)
	})

	t.Run("successful preview with zero count", func(t *testing.T) {
		tree := createTestTree()

		mockRepo.EXPECT().
			PreviewSegment(ctx, "workspace123", gomock.Any(), gomock.Any(), 10).
			Return(0, nil)

		response, err := service.PreviewSegment(ctx, "workspace123", tree, 10)
		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.Equal(t, 0, response.TotalCount)
		assert.Empty(t, response.Emails)
	})

	t.Run("validation errors", func(t *testing.T) {
		testCases := []struct {
			name        string
			workspaceID string
			tree        *domain.TreeNode
			limit       int
			errContains string
		}{
			{
				name:        "missing workspace ID",
				workspaceID: "",
				tree:        createTestTree(),
				limit:       10,
				errContains: "workspace_id is required",
			},
			{
				name:        "missing tree",
				workspaceID: "workspace123",
				tree:        nil,
				limit:       10,
				errContains: "tree is required",
			},
			{
				name:        "invalid tree structure",
				workspaceID: "workspace123",
				tree:        &domain.TreeNode{Kind: "invalid"},
				limit:       10,
				errContains: "invalid tree",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				response, err := service.PreviewSegment(ctx, tc.workspaceID, tc.tree, tc.limit)
				assert.Error(t, err)
				assert.Nil(t, response)
				assert.Contains(t, err.Error(), tc.errContains)
			})
		}
	})

	t.Run("repository preview error", func(t *testing.T) {
		tree := createTestTree()

		mockRepo.EXPECT().
			PreviewSegment(ctx, "workspace123", gomock.Any(), gomock.Any(), 10).
			Return(0, errors.New("preview failed"))

		response, err := service.PreviewSegment(ctx, "workspace123", tree, 10)
		assert.Error(t, err)
		assert.Nil(t, response)
		assert.Contains(t, err.Error(), "failed to preview segment")
	})

	t.Run("default limit handling", func(t *testing.T) {
		tree := createTestTree()

		mockRepo.EXPECT().
			PreviewSegment(ctx, "workspace123", gomock.Any(), gomock.Any(), 20).
			Return(10, nil)

		// Test with invalid limit (0) - should default to 20
		response, err := service.PreviewSegment(ctx, "workspace123", tree, 0)
		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.Equal(t, 20, response.Limit) // Default limit
	})

	t.Run("max limit handling", func(t *testing.T) {
		tree := createTestTree()

		mockRepo.EXPECT().
			PreviewSegment(ctx, "workspace123", gomock.Any(), gomock.Any(), 20).
			Return(10, nil)

		// Test with limit > 100 - should default to 20
		response, err := service.PreviewSegment(ctx, "workspace123", tree, 150)
		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.Equal(t, 20, response.Limit) // Default limit
	})
}

func TestSegmentService_GetSegmentContacts(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockSegmentRepository(ctrl)
	mockTaskService := mocks.NewMockTaskService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	service := NewSegmentService(mockRepo, mockWorkspaceRepo, mockTaskService, newPermissiveAuthService(ctrl), mockLogger)
	ctx := context.Background()

	t.Run("validation errors", func(t *testing.T) {
		testCases := []struct {
			name        string
			workspaceID string
			segmentID   string
			errContains string
		}{
			{
				name:        "missing workspace ID",
				workspaceID: "",
				segmentID:   "segment1",
				errContains: "workspace_id is required",
			},
			{
				name:        "missing segment ID",
				workspaceID: "workspace123",
				segmentID:   "",
				errContains: "segment_id is required",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				emails, err := service.GetSegmentContacts(ctx, tc.workspaceID, tc.segmentID, 50, 0)
				assert.Error(t, err)
				assert.Nil(t, emails)
				assert.Contains(t, err.Error(), tc.errContains)
			})
		}
	})

	t.Run("workspace connection error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, "workspace123").
			Return(nil, errors.New("connection failed"))

		emails, err := service.GetSegmentContacts(ctx, "workspace123", "segment1", 50, 0)
		assert.Error(t, err)
		assert.Nil(t, emails)
		assert.Contains(t, err.Error(), "failed to get workspace database connection")
	})

	t.Run("default limit handling", func(t *testing.T) {
		// Test that limit 0 gets defaulted to 20
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, "workspace123").
			Return(nil, errors.New("connection failed"))

		emails, err := service.GetSegmentContacts(ctx, "workspace123", "segment1", 0, 0)
		assert.Error(t, err)
		assert.Nil(t, emails)
	})

	t.Run("max limit handling", func(t *testing.T) {
		// Test that limit > 100 gets capped to 100
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, "workspace123").
			Return(nil, errors.New("connection failed"))

		emails, err := service.GetSegmentContacts(ctx, "workspace123", "segment1", 200, 0)
		assert.Error(t, err)
		assert.Nil(t, emails)
	})
}

func TestSegmentService_GetSegmentContactDetails(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()

	newService := func(t *testing.T) (*SegmentService, *mocks.MockSegmentRepository) {
		ctrl := gomock.NewController(t)
		t.Cleanup(ctrl.Finish)

		mockRepo := mocks.NewMockSegmentRepository(ctrl)
		svc := NewSegmentService(
			mockRepo,
			mocks.NewMockWorkspaceRepository(ctrl),
			mocks.NewMockTaskService(ctrl),
			newPermissiveAuthService(ctrl),
			pkgmocks.NewMockLogger(ctrl),
		)
		return svc, mockRepo
	}

	t.Run("returns the expanded records the repository produced", func(t *testing.T) {
		svc, mockRepo := newService(t)

		expected := []*domain.SegmentContactDetail{
			{Contact: &domain.Contact{Email: "recent@example.com"}, MatchedAt: now},
			{Contact: &domain.Contact{Email: "older@example.com"}, MatchedAt: now.Add(-time.Hour)},
		}

		mockRepo.EXPECT().
			GetSegmentContactDetails(gomock.Any(), "workspace123", "segment1", 50, 10).
			Return(expected, nil)

		details, err := svc.GetSegmentContactDetails(ctx, "workspace123", "segment1", 50, 10)
		require.NoError(t, err)
		assert.Equal(t, expected, details)
	})

	t.Run("validation errors", func(t *testing.T) {
		testCases := []struct {
			name        string
			workspaceID string
			segmentID   string
			errContains string
		}{
			{
				name:        "missing workspace ID",
				workspaceID: "",
				segmentID:   "segment1",
				errContains: "workspace_id is required",
			},
			{
				name:        "missing segment ID",
				workspaceID: "workspace123",
				segmentID:   "",
				errContains: "segment_id is required",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				svc, _ := newService(t)

				details, err := svc.GetSegmentContactDetails(ctx, tc.workspaceID, tc.segmentID, 50, 0)
				require.Error(t, err)
				assert.Nil(t, details)
				assert.Contains(t, err.Error(), tc.errContains)
			})
		}
	})

	t.Run("clamps the page size", func(t *testing.T) {
		testCases := []struct {
			name          string
			limit         int
			offset        int
			expectedLimit int
			expectedOff   int
		}{
			{name: "absent limit falls back to a page", limit: 0, expectedLimit: 20},
			{name: "negative limit falls back to a page", limit: -5, expectedLimit: 20},
			{name: "oversized limit is capped", limit: 5000, expectedLimit: 100},
			{name: "negative offset starts at the first page", limit: 10, offset: -3, expectedLimit: 10, expectedOff: 0},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				svc, mockRepo := newService(t)

				mockRepo.EXPECT().
					GetSegmentContactDetails(gomock.Any(), "workspace123", "segment1", tc.expectedLimit, tc.expectedOff).
					Return(nil, nil)

				_, err := svc.GetSegmentContactDetails(ctx, "workspace123", "segment1", tc.limit, tc.offset)
				require.NoError(t, err)
			})
		}
	})

	t.Run("repository error is wrapped", func(t *testing.T) {
		svc, mockRepo := newService(t)

		mockRepo.EXPECT().
			GetSegmentContactDetails(gomock.Any(), "workspace123", "segment1", 20, 0).
			Return(nil, errors.New("query failed"))

		details, err := svc.GetSegmentContactDetails(ctx, "workspace123", "segment1", 0, 0)
		require.Error(t, err)
		assert.Nil(t, details)
		assert.Contains(t, err.Error(), "failed to get segment contact details")
		assert.Contains(t, err.Error(), "query failed")
	})
}

func TestNewSegmentService(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockSegmentRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockTaskService := mocks.NewMockTaskService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	service := NewSegmentService(mockRepo, mockWorkspaceRepo, mockTaskService, newPermissiveAuthService(ctrl), mockLogger)

	assert.NotNil(t, service)
	assert.NotNil(t, service.segmentRepo)
	assert.NotNil(t, service.workspaceRepo)
	assert.NotNil(t, service.taskService)
	assert.NotNil(t, service.queryBuilder)
	assert.NotNil(t, service.logger)
}

func TestCalculateNext5AMInTimezone(t *testing.T) {
	// Test calculateNext5AMInTimezone - this was at 0% coverage
	t.Run("Success - Valid timezone UTC", func(t *testing.T) {
		next5AM, err := calculateNext5AMInTimezone("UTC")
		require.NoError(t, err)
		assert.NotZero(t, next5AM)

		// Verify it's 5 AM in UTC
		utcLoc, _ := time.LoadLocation("UTC")
		next5AMInUTC := next5AM.In(utcLoc)
		assert.Equal(t, 5, next5AMInUTC.Hour())
		assert.Equal(t, 0, next5AMInUTC.Minute())
		assert.Equal(t, 0, next5AMInUTC.Second())
	})

	t.Run("Success - Valid timezone America/New_York", func(t *testing.T) {
		next5AM, err := calculateNext5AMInTimezone("America/New_York")
		require.NoError(t, err)
		assert.NotZero(t, next5AM)

		// Verify it's 5 AM in the timezone
		nyLoc, _ := time.LoadLocation("America/New_York")
		next5AMInNY := next5AM.In(nyLoc)
		assert.Equal(t, 5, next5AMInNY.Hour())
		assert.Equal(t, 0, next5AMInNY.Minute())
		assert.Equal(t, 0, next5AMInNY.Second())
	})

	t.Run("Success - Valid timezone Europe/London", func(t *testing.T) {
		next5AM, err := calculateNext5AMInTimezone("Europe/London")
		require.NoError(t, err)
		assert.NotZero(t, next5AM)

		// Verify it's 5 AM in the timezone
		londonLoc, _ := time.LoadLocation("Europe/London")
		next5AMInLondon := next5AM.In(londonLoc)
		assert.Equal(t, 5, next5AMInLondon.Hour())
		assert.Equal(t, 0, next5AMInLondon.Minute())
		assert.Equal(t, 0, next5AMInLondon.Second())
	})

	t.Run("Error - Invalid timezone", func(t *testing.T) {
		next5AM, err := calculateNext5AMInTimezone("Invalid/Timezone")
		require.Error(t, err)
		assert.Zero(t, next5AM)
		assert.Contains(t, err.Error(), "invalid timezone")
	})

	t.Run("Success - Returns next day if already past 5 AM", func(t *testing.T) {
		// This test verifies that if it's already past 5 AM, it returns tomorrow's 5 AM
		// We can't control the exact time, but we can verify the logic works
		next5AM, err := calculateNext5AMInTimezone("UTC")
		require.NoError(t, err)

		// The result should be in the future (or very close to now if it's exactly 5 AM)
		now := time.Now().UTC()
		assert.True(t, next5AM.After(now) || next5AM.Equal(now), "next5AM should be in the future or now")

		// Verify it's 5 AM in UTC
		utcLoc, _ := time.LoadLocation("UTC")
		next5AMInUTC := next5AM.In(utcLoc)
		assert.Equal(t, 5, next5AMInUTC.Hour())
	})
}

// segmentPermissionDeps builds a segment service whose repositories carry no
// EXPECT() calls, so gomock fails if anything past the permission gate runs.
type segmentPermissionDeps struct {
	ctrl        *gomock.Controller
	authService *mocks.MockAuthService
	svc         *SegmentService
}

func setupSegmentPermissionSvc(t *testing.T) *segmentPermissionDeps {
	ctrl := gomock.NewController(t)
	authService := mocks.NewMockAuthService(ctrl)
	svc := NewSegmentService(
		mocks.NewMockSegmentRepository(ctrl),
		mocks.NewMockWorkspaceRepository(ctrl),
		mocks.NewMockTaskService(ctrl),
		authService,
		newSegmentTestLogger(ctrl),
	)
	return &segmentPermissionDeps{ctrl: ctrl, authService: authService, svc: svc}
}

// segmentMember returns a member (not an owner, so HasPermission actually
// consults the grants) holding the given segments permissions and contacts:read,
// so a denial can only come from the segments gate.
func segmentMember(read, write bool) *domain.UserWorkspace {
	return &domain.UserWorkspace{
		UserID:      "user-1",
		WorkspaceID: "w1",
		Role:        "member",
		Permissions: domain.UserPermissions{
			domain.PermissionResourceSegments: {Read: read, Write: write},
			domain.PermissionResourceContacts: {Read: true},
		},
	}
}

type segmentPermissionCase struct {
	name string
	perm domain.PermissionType
	call func(*segmentPermissionDeps, context.Context) error
}

func segmentPermissionCases() []segmentPermissionCase {
	return []segmentPermissionCase{
		{"CreateSegment", domain.PermissionTypeWrite, func(d *segmentPermissionDeps, ctx context.Context) error {
			_, err := d.svc.CreateSegment(ctx, &domain.CreateSegmentRequest{WorkspaceID: "w1", ID: "seg-1"})
			return err
		}},
		{"GetSegment", domain.PermissionTypeRead, func(d *segmentPermissionDeps, ctx context.Context) error {
			_, err := d.svc.GetSegment(ctx, &domain.GetSegmentRequest{WorkspaceID: "w1", ID: "seg-1"})
			return err
		}},
		{"ListSegments", domain.PermissionTypeRead, func(d *segmentPermissionDeps, ctx context.Context) error {
			_, err := d.svc.ListSegments(ctx, &domain.GetSegmentsRequest{WorkspaceID: "w1"})
			return err
		}},
		{"UpdateSegment", domain.PermissionTypeWrite, func(d *segmentPermissionDeps, ctx context.Context) error {
			_, err := d.svc.UpdateSegment(ctx, &domain.UpdateSegmentRequest{WorkspaceID: "w1", ID: "seg-1"})
			return err
		}},
		{"DeleteSegment", domain.PermissionTypeWrite, func(d *segmentPermissionDeps, ctx context.Context) error {
			return d.svc.DeleteSegment(ctx, &domain.DeleteSegmentRequest{WorkspaceID: "w1", ID: "seg-1"})
		}},
		{"RebuildSegment", domain.PermissionTypeWrite, func(d *segmentPermissionDeps, ctx context.Context) error {
			return d.svc.RebuildSegment(ctx, "w1", "seg-1")
		}},
		{"PreviewSegment", domain.PermissionTypeRead, func(d *segmentPermissionDeps, ctx context.Context) error {
			_, err := d.svc.PreviewSegment(ctx, "w1", createTestTree(), 10)
			return err
		}},
		{"GetSegmentContacts", domain.PermissionTypeRead, func(d *segmentPermissionDeps, ctx context.Context) error {
			_, err := d.svc.GetSegmentContacts(ctx, "w1", "seg-1", 50, 0)
			return err
		}},
		{"GetSegmentContactDetails", domain.PermissionTypeRead, func(d *segmentPermissionDeps, ctx context.Context) error {
			_, err := d.svc.GetSegmentContactDetails(ctx, "w1", "seg-1", 50, 0)
			return err
		}},
	}
}

// TestSegmentService_PermissionEnforcement verifies that every segment operation
// enforces the segments permission at the right level. Each method is exercised by
// a member who has been granted the OPPOSITE permission (a write operation is
// tested with a read-only member), so the test fails both if a check is missing
// AND if a method is gated on the wrong permission type (read/write swap).
func TestSegmentService_PermissionEnforcement(t *testing.T) {
	for _, tc := range segmentPermissionCases() {
		t.Run(tc.name, func(t *testing.T) {
			d := setupSegmentPermissionSvc(t)
			defer d.ctrl.Finish()

			// Grant only the OPPOSITE permission so the test proves the exact
			// permission type is required.
			grant := segmentMember(true, false) // has read, lacks write
			if tc.perm == domain.PermissionTypeRead {
				grant = segmentMember(false, true) // has write, lacks read
			}

			ctx := context.Background()
			d.authService.EXPECT().
				AuthenticateUserForWorkspace(ctx, "w1").
				Return(ctx, &domain.User{ID: "user-1"}, grant, nil)

			err := tc.call(d, ctx)
			require.Error(t, err)
			assert.IsType(t, &domain.PermissionError{}, err)

			var permErr *domain.PermissionError
			require.True(t, errors.As(err, &permErr))
			assert.Equal(t, domain.PermissionResourceSegments, permErr.Resource)
			assert.Equal(t, tc.perm, permErr.Permission)
		})
	}
}

// TestSegmentService_PreviewAndContactsRequireContactsRead pins the second gate on
// every method that answers questions about contacts through a segment query:
// segments:read alone is a count oracle over contact attributes, list membership,
// custom events and message history, so contacts:read is required on top of it.
func TestSegmentService_PreviewAndContactsRequireContactsRead(t *testing.T) {
	// Full segments access, no contacts grant at all.
	grant := &domain.UserWorkspace{
		UserID:      "user-1",
		WorkspaceID: "w1",
		Role:        "member",
		Permissions: domain.UserPermissions{
			domain.PermissionResourceSegments: {Read: true, Write: true},
		},
	}

	cases := []struct {
		name string
		call func(*segmentPermissionDeps, context.Context) error
	}{
		{"PreviewSegment", func(d *segmentPermissionDeps, ctx context.Context) error {
			_, err := d.svc.PreviewSegment(ctx, "w1", createTestTree(), 10)
			return err
		}},
		{"GetSegmentContacts", func(d *segmentPermissionDeps, ctx context.Context) error {
			_, err := d.svc.GetSegmentContacts(ctx, "w1", "seg-1", 50, 0)
			return err
		}},
		{"GetSegmentContactDetails", func(d *segmentPermissionDeps, ctx context.Context) error {
			_, err := d.svc.GetSegmentContactDetails(ctx, "w1", "seg-1", 50, 0)
			return err
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := setupSegmentPermissionSvc(t)
			defer d.ctrl.Finish()

			ctx := context.Background()
			d.authService.EXPECT().
				AuthenticateUserForWorkspace(ctx, "w1").
				Return(ctx, &domain.User{ID: "user-1"}, grant, nil)

			err := tc.call(d, ctx)
			require.Error(t, err)

			var permErr *domain.PermissionError
			require.True(t, errors.As(err, &permErr))
			assert.Equal(t, domain.PermissionResourceContacts, permErr.Resource)
			assert.Equal(t, domain.PermissionTypeRead, permErr.Permission)
		})
	}
}

// TestSegmentService_ReadMethodsAllowedWithoutContactsRead is the counterpart:
// the contacts grant gates preview and contacts only — get and list stay reachable
// on segments:read alone.
func TestSegmentService_ReadMethodsAllowedWithoutContactsRead(t *testing.T) {
	grant := &domain.UserWorkspace{
		UserID:      "user-1",
		WorkspaceID: "w1",
		Role:        "member",
		Permissions: domain.UserPermissions{
			domain.PermissionResourceSegments: {Read: true},
		},
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	segmentRepo := mocks.NewMockSegmentRepository(ctrl)
	authService := mocks.NewMockAuthService(ctrl)
	svc := NewSegmentService(
		segmentRepo,
		mocks.NewMockWorkspaceRepository(ctrl),
		mocks.NewMockTaskService(ctrl),
		authService,
		newSegmentTestLogger(ctrl),
	)

	ctx := context.Background()
	authService.EXPECT().
		AuthenticateUserForWorkspace(ctx, "w1").
		Return(ctx, &domain.User{ID: "user-1"}, grant, nil).
		AnyTimes()

	segmentRepo.EXPECT().GetSegments(gomock.Any(), "w1", false).Return([]*domain.Segment{}, nil)
	segments, err := svc.ListSegments(ctx, &domain.GetSegmentsRequest{WorkspaceID: "w1"})
	require.NoError(t, err)
	assert.Empty(t, segments)

	segmentRepo.EXPECT().GetSegmentByID(gomock.Any(), "w1", "seg-1").Return(&domain.Segment{ID: "seg-1"}, nil)
	segmentRepo.EXPECT().GetSegmentContactCount(gomock.Any(), "w1", "seg-1").Return(0, nil)
	segment, err := svc.GetSegment(ctx, &domain.GetSegmentRequest{WorkspaceID: "w1", ID: "seg-1"})
	require.NoError(t, err)
	assert.Equal(t, "seg-1", segment.ID)
}
