package service

import (
	"context"
	"fmt"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/google/uuid"
)

// SegmentService handles segment operations
type SegmentService struct {
	segmentRepo   domain.SegmentRepository
	workspaceRepo domain.WorkspaceRepository
	taskService   domain.TaskService
	queryBuilder  *QueryBuilder
	logger        logger.Logger
}

// NewSegmentService creates a new segment service
func NewSegmentService(
	segmentRepo domain.SegmentRepository,
	workspaceRepo domain.WorkspaceRepository,
	taskService domain.TaskService,
	logger logger.Logger,
) *SegmentService {
	return &SegmentService{
		segmentRepo:   segmentRepo,
		workspaceRepo: workspaceRepo,
		taskService:   taskService,
		queryBuilder:  NewQueryBuilder(),
		logger:        logger,
	}
}

// CreateSegment creates a new segment
func (s *SegmentService) CreateSegment(ctx context.Context, req *domain.CreateSegmentRequest) (*domain.Segment, error) {
	// Validate the request
	segment, workspaceID, err := req.Validate()
	if err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	// Generate ID if not provided
	if segment.ID == "" {
		segment.ID = uuid.New().String()[:8]
	}

	// Set initial version
	segment.Version = 1

	// Set status to building (will be updated by the build task)
	segment.Status = string(domain.SegmentStatusBuilding)

	// Set timestamps
	now := time.Now().UTC()
	segment.DBCreatedAt = now
	segment.DBUpdatedAt = now

	// Validate and generate SQL from the tree
	sqlQuery, args, err := s.queryBuilder.BuildSQL(segment.Tree)
	if err != nil {
		return nil, fmt.Errorf("failed to build SQL from tree: %w", err)
	}

	// Store generated SQL and args for debugging and reuse
	segment.GeneratedSQL = &sqlQuery
	segment.GeneratedArgs = domain.JSONArray(args)

	// Create the segment in the database
	if err := s.segmentRepo.CreateSegment(ctx, workspaceID, segment); err != nil {
		return nil, fmt.Errorf("failed to create segment: %w", err)
	}

	// Create a task to build the segment membership
	task := &domain.Task{
		ID:          uuid.New().String(),
		WorkspaceID: workspaceID,
		Type:        "build_segment",
		Status:      domain.TaskStatusPending,
		Progress:    0,
		State: &domain.TaskState{
			BuildSegment: &domain.BuildSegmentState{
				SegmentID: segment.ID,
				Version:   segment.Version,
				BatchSize: 100,
				StartedAt: time.Now().Format(time.RFC3339),
			},
		},
		MaxRuntime: 300, // 5 minutes
		MaxRetries: 3,
	}

	if err := s.taskService.CreateTask(ctx, workspaceID, task); err != nil {
		s.logger.WithFields(map[string]interface{}{
			"error":      err.Error(),
			"segment_id": segment.ID,
		}).Warn("Failed to create build task for segment (non-fatal)")
	} else {
		// Immediately trigger task execution after segment creation
		go func() {
			// Small delay to ensure transaction is committed
			time.Sleep(100 * time.Millisecond)
			if execErr := s.taskService.ExecutePendingTasks(context.Background(), 1); execErr != nil {
				s.logger.WithFields(map[string]interface{}{
					"segment_id": segment.ID,
					"task_id":    task.ID,
					"error":      execErr.Error(),
				}).Error("Failed to trigger immediate task execution after segment creation")
			}
		}()
	}

	s.logger.WithFields(map[string]interface{}{
		"segment_id":   segment.ID,
		"workspace_id": workspaceID,
	}).Info("Segment created")

	return segment, nil
}

// GetSegment retrieves a segment by ID
func (s *SegmentService) GetSegment(ctx context.Context, req *domain.GetSegmentRequest) (*domain.Segment, error) {
	if req.WorkspaceID == "" {
		return nil, fmt.Errorf("workspace_id is required")
	}
	if req.ID == "" {
		return nil, fmt.Errorf("segment id is required")
	}

	segment, err := s.segmentRepo.GetSegmentByID(ctx, req.WorkspaceID, req.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get segment: %w", err)
	}

	// Fetch contact count for the segment
	count, err := s.segmentRepo.GetSegmentContactCount(ctx, req.WorkspaceID, req.ID)
	if err != nil {
		s.logger.WithField("error", err.Error()).Warn("Failed to get segment contact count")
	} else {
		segment.UsersCount = count
	}

	return segment, nil
}

// ListSegments retrieves all segments for a workspace
func (s *SegmentService) ListSegments(ctx context.Context, req *domain.GetSegmentsRequest) ([]*domain.Segment, error) {
	if req.WorkspaceID == "" {
		return nil, fmt.Errorf("workspace_id is required")
	}

	// Pass withCount parameter to repository - counts are fetched in a single efficient query
	segments, err := s.segmentRepo.GetSegments(ctx, req.WorkspaceID, req.WithCount)
	if err != nil {
		return nil, fmt.Errorf("failed to list segments: %w", err)
	}

	return segments, nil
}

// UpdateSegment updates an existing segment
func (s *SegmentService) UpdateSegment(ctx context.Context, req *domain.UpdateSegmentRequest) (*domain.Segment, error) {
	// Validate the request
	updates, workspaceID, err := req.Validate()
	if err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	// Fetch the existing segment
	existing, err := s.segmentRepo.GetSegmentByID(ctx, workspaceID, req.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get existing segment: %w", err)
	}

	// Track if tree has changed (requires rebuild)
	treeChanged := false

	// Apply updates
	if updates.Name != "" {
		existing.Name = updates.Name
	}
	if updates.Color != "" {
		existing.Color = updates.Color
	}
	if updates.Timezone != "" {
		existing.Timezone = updates.Timezone
	}
	if updates.Tree != nil {
		existing.Tree = updates.Tree
		treeChanged = true

		// Regenerate SQL and args
		sqlQuery, args, err := s.queryBuilder.BuildSQL(existing.Tree)
		if err != nil {
			return nil, fmt.Errorf("failed to build SQL from updated tree: %w", err)
		}
		existing.GeneratedSQL = &sqlQuery
		existing.GeneratedArgs = domain.JSONArray(args)

		// Increment version since the criteria changed
		existing.Version++
	}

	// Update timestamp
	existing.DBUpdatedAt = time.Now().UTC()

	// Save the updated segment
	if err := s.segmentRepo.UpdateSegment(ctx, workspaceID, existing); err != nil {
		return nil, fmt.Errorf("failed to update segment: %w", err)
	}

	// If the tree changed, create a new build task
	if treeChanged {
		task := &domain.Task{
			ID:          uuid.New().String(),
			WorkspaceID: workspaceID,
			Type:        "build_segment",
			Status:      domain.TaskStatusPending,
			Progress:    0,
			State: &domain.TaskState{
				BuildSegment: &domain.BuildSegmentState{
					SegmentID: existing.ID,
					Version:   existing.Version,
					BatchSize: 100,
					StartedAt: time.Now().Format(time.RFC3339),
				},
			},
			MaxRuntime: 300, // 5 minutes
			MaxRetries: 3,
		}

		if err := s.taskService.CreateTask(ctx, workspaceID, task); err != nil {
			s.logger.WithFields(map[string]interface{}{
				"error":      err.Error(),
				"segment_id": existing.ID,
			}).Warn("Failed to create rebuild task for segment (non-fatal)")
		} else {
			// Immediately trigger task execution after segment update
			go func() {
				// Small delay to ensure transaction is committed
				time.Sleep(100 * time.Millisecond)
				if execErr := s.taskService.ExecutePendingTasks(context.Background(), 1); execErr != nil {
					s.logger.WithFields(map[string]interface{}{
						"segment_id": existing.ID,
						"task_id":    task.ID,
						"error":      execErr.Error(),
					}).Error("Failed to trigger immediate task execution after segment update")
				}
			}()
		}

		existing.Status = string(domain.SegmentStatusBuilding)
	}

	s.logger.WithFields(map[string]interface{}{
		"segment_id":   existing.ID,
		"workspace_id": workspaceID,
		"tree_changed": treeChanged,
	}).Info("Segment updated")

	return existing, nil
}

// DeleteSegment deletes a segment
func (s *SegmentService) DeleteSegment(ctx context.Context, req *domain.DeleteSegmentRequest) error {
	workspaceID, id, err := req.Validate()
	if err != nil {
		return fmt.Errorf("invalid request: %w", err)
	}

	if err := s.segmentRepo.DeleteSegment(ctx, workspaceID, id); err != nil {
		return fmt.Errorf("failed to delete segment: %w", err)
	}

	s.logger.WithFields(map[string]interface{}{
		"segment_id":   id,
		"workspace_id": workspaceID,
	}).Info("Segment deleted")

	return nil
}

// RebuildSegment triggers a rebuild of segment membership
func (s *SegmentService) RebuildSegment(ctx context.Context, workspaceID, segmentID string) error {
	if workspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}
	if segmentID == "" {
		return fmt.Errorf("segment_id is required")
	}

	// Fetch the segment
	segment, err := s.segmentRepo.GetSegmentByID(ctx, workspaceID, segmentID)
	if err != nil {
		return fmt.Errorf("failed to get segment: %w", err)
	}

	// Increment version for the rebuild
	segment.Version++
	segment.Status = string(domain.SegmentStatusBuilding)
	segment.DBUpdatedAt = time.Now().UTC()

	if err := s.segmentRepo.UpdateSegment(ctx, workspaceID, segment); err != nil {
		return fmt.Errorf("failed to update segment: %w", err)
	}

	// Create a build task
	task := &domain.Task{
		ID:          uuid.New().String(),
		WorkspaceID: workspaceID,
		Type:        "build_segment",
		Status:      domain.TaskStatusPending,
		Progress:    0,
		State: &domain.TaskState{
			BuildSegment: &domain.BuildSegmentState{
				SegmentID: segment.ID,
				Version:   segment.Version,
				BatchSize: 100,
				StartedAt: time.Now().Format(time.RFC3339),
			},
		},
		MaxRuntime: 300, // 5 minutes
		MaxRetries: 3,
	}

	if err := s.taskService.CreateTask(ctx, workspaceID, task); err != nil {
		return fmt.Errorf("failed to create rebuild task: %w", err)
	}

	// Immediately trigger task execution after segment rebuild
	go func() {
		// Small delay to ensure transaction is committed
		time.Sleep(100 * time.Millisecond)
		if execErr := s.taskService.ExecutePendingTasks(context.Background(), 1); execErr != nil {
			s.logger.WithFields(map[string]interface{}{
				"segment_id": segmentID,
				"task_id":    task.ID,
				"error":      execErr.Error(),
			}).Error("Failed to trigger immediate task execution after segment rebuild")
		}
	}()

	s.logger.WithFields(map[string]interface{}{
		"segment_id":   segmentID,
		"workspace_id": workspaceID,
		"version":      segment.Version,
	}).Info("Segment rebuild initiated")

	return nil
}

// PreviewSegment executes the segment query and returns a preview of matching contacts
// This does NOT save the results to the database
func (s *SegmentService) PreviewSegment(ctx context.Context, workspaceID string, tree *domain.TreeNode, limit int) (*domain.PreviewSegmentResponse, error) {
	if workspaceID == "" {
		return nil, fmt.Errorf("workspace_id is required")
	}
	if tree == nil {
		return nil, fmt.Errorf("tree is required")
	}
	if limit <= 0 || limit > 100 {
		limit = 20 // Default preview limit
	}

	// Validate the tree
	if err := tree.Validate(); err != nil {
		return nil, fmt.Errorf("invalid tree: %w", err)
	}

	// Build SQL from tree
	sqlQuery, args, err := s.queryBuilder.BuildSQL(tree)
	if err != nil {
		return nil, fmt.Errorf("failed to build SQL: %w", err)
	}

	s.logger.WithFields(map[string]interface{}{
		"workspace_id": workspaceID,
		"sql":          sqlQuery,
		"args":         args,
	}).Debug("Preview segment SQL generated")

	// Get count using repository method
	totalCount, err := s.segmentRepo.PreviewSegment(ctx, workspaceID, sqlQuery, args)
	if err != nil {
		return nil, fmt.Errorf("failed to preview segment: %w", err)
	}

	return &domain.PreviewSegmentResponse{
		Emails:       []string{}, // Don't return emails for privacy/performance
		TotalCount:   totalCount,
		Limit:        limit,
		GeneratedSQL: sqlQuery,
		SQLArgs:      args,
	}, nil
}

// GetSegmentContacts retrieves contacts that belong to a segment
func (s *SegmentService) GetSegmentContacts(ctx context.Context, workspaceID, segmentID string, limit, offset int) ([]string, error) {
	if workspaceID == "" {
		return nil, fmt.Errorf("workspace_id is required")
	}
	if segmentID == "" {
		return nil, fmt.Errorf("segment_id is required")
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	// Get workspace database connection
	workspaceDB, err := s.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace database connection: %w", err)
	}

	// Query contact_segments table
	query := `
		SELECT email 
		FROM contact_segments 
		WHERE segment_id = $1 
		ORDER BY created_at DESC 
		LIMIT $2 OFFSET $3
	`

	rows, err := workspaceDB.QueryContext(ctx, query, segmentID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query segment contacts: %w", err)
	}
	defer rows.Close()

	var emails []string
	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err != nil {
			return nil, fmt.Errorf("failed to scan email: %w", err)
		}
		emails = append(emails, email)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating segment contacts: %w", err)
	}

	return emails, nil
}
