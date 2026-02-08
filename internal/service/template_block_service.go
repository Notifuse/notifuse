package service

import (
	"context"
	"fmt"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/google/uuid"
)

type TemplateBlockService struct {
	repo        domain.WorkspaceRepository
	authService domain.AuthService
	logger      logger.Logger
}

func NewTemplateBlockService(
	repo domain.WorkspaceRepository,
	authService domain.AuthService,
	logger logger.Logger,
) *TemplateBlockService {
	return &TemplateBlockService{
		repo:        repo,
		authService: authService,
		logger:      logger,
	}
}

func (s *TemplateBlockService) CreateTemplateBlock(ctx context.Context, workspaceID string, block *domain.TemplateBlock) error {
	// Authenticate user for workspace
	var err error
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Check permission for writing templates
	if !userWorkspace.HasPermission(domain.PermissionResourceTemplates, domain.PermissionTypeWrite) {
		return domain.NewPermissionError(
			domain.PermissionResourceTemplates,
			domain.PermissionTypeWrite,
			"Insufficient permissions: write access to templates required",
		)
	}

	// Get the existing workspace
	workspace, err := s.repo.GetByID(ctx, workspaceID)
	if err != nil {
		s.logger.WithField("workspace_id", workspaceID).WithField("error", err.Error()).Error("Failed to get workspace")
		return fmt.Errorf("failed to get workspace: %w", err)
	}

	// Ensure block has an ID and timestamps
	if block.ID == "" {
		block.ID = uuid.New().String()
	}
	if block.Created.IsZero() {
		block.Created = time.Now().UTC()
	}
	block.Updated = time.Now().UTC()

	// Validate block
	if block.Name == "" {
		return fmt.Errorf("invalid template block: name is required")
	}
	if len(block.Name) > 255 {
		return fmt.Errorf("invalid template block: name length must be between 1 and 255")
	}
	if block.Block == nil || block.Block.GetType() == "" {
		return fmt.Errorf("invalid template block: block kind is required")
	}

	// Check for duplicate ID
	for _, existingBlock := range workspace.Settings.TemplateBlocks {
		if existingBlock.ID == block.ID {
			return fmt.Errorf("template block with id %s already exists", block.ID)
		}
	}

	// Add block to workspace settings
	workspace.Settings.TemplateBlocks = append(workspace.Settings.TemplateBlocks, *block)
	workspace.UpdatedAt = time.Now().UTC()

	// Update workspace
	if err := s.repo.Update(ctx, workspace); err != nil {
		s.logger.WithField("workspace_id", workspaceID).WithField("block_id", block.ID).WithField("error", err.Error()).Error("Failed to create template block")
		return fmt.Errorf("failed to create template block: %w", err)
	}

	return nil
}

func (s *TemplateBlockService) GetTemplateBlock(ctx context.Context, workspaceID string, id string) (*domain.TemplateBlock, error) {
	// Authenticate user for workspace
	var err error
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Check permission for reading templates
	if !userWorkspace.HasPermission(domain.PermissionResourceTemplates, domain.PermissionTypeRead) {
		return nil, domain.NewPermissionError(
			domain.PermissionResourceTemplates,
			domain.PermissionTypeRead,
			"Insufficient permissions: read access to templates required",
		)
	}

	// Get the workspace
	workspace, err := s.repo.GetByID(ctx, workspaceID)
	if err != nil {
		s.logger.WithField("workspace_id", workspaceID).WithField("error", err.Error()).Error("Failed to get workspace")
		return nil, fmt.Errorf("failed to get workspace: %w", err)
	}

	// Find the block
	for _, block := range workspace.Settings.TemplateBlocks {
		if block.ID == id {
			return &block, nil
		}
	}

	return nil, &domain.ErrTemplateBlockNotFound{Message: fmt.Sprintf("template block with id %s not found", id)}
}

func (s *TemplateBlockService) ListTemplateBlocks(ctx context.Context, workspaceID string) ([]*domain.TemplateBlock, error) {
	// Authenticate user for workspace
	var err error
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Check permission for reading templates
	if !userWorkspace.HasPermission(domain.PermissionResourceTemplates, domain.PermissionTypeRead) {
		return nil, domain.NewPermissionError(
			domain.PermissionResourceTemplates,
			domain.PermissionTypeRead,
			"Insufficient permissions: read access to templates required",
		)
	}

	// Get the workspace
	workspace, err := s.repo.GetByID(ctx, workspaceID)
	if err != nil {
		s.logger.WithField("workspace_id", workspaceID).WithField("error", err.Error()).Error("Failed to get workspace")
		return nil, fmt.Errorf("failed to get workspace: %w", err)
	}

	// Return all blocks
	blocks := make([]*domain.TemplateBlock, len(workspace.Settings.TemplateBlocks))
	for i := range workspace.Settings.TemplateBlocks {
		blocks[i] = &workspace.Settings.TemplateBlocks[i]
	}

	return blocks, nil
}

func (s *TemplateBlockService) UpdateTemplateBlock(ctx context.Context, workspaceID string, block *domain.TemplateBlock) error {
	// Authenticate user for workspace
	var err error
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Check permission for writing templates
	if !userWorkspace.HasPermission(domain.PermissionResourceTemplates, domain.PermissionTypeWrite) {
		return domain.NewPermissionError(
			domain.PermissionResourceTemplates,
			domain.PermissionTypeWrite,
			"Insufficient permissions: write access to templates required",
		)
	}

	// Get the existing workspace
	workspace, err := s.repo.GetByID(ctx, workspaceID)
	if err != nil {
		s.logger.WithField("workspace_id", workspaceID).WithField("error", err.Error()).Error("Failed to get workspace")
		return fmt.Errorf("failed to get workspace: %w", err)
	}

	// Validate block
	if block.Name == "" {
		return fmt.Errorf("invalid template block: name is required")
	}
	if len(block.Name) > 255 {
		return fmt.Errorf("invalid template block: name length must be between 1 and 255")
	}
	if block.Block == nil || block.Block.GetType() == "" {
		return fmt.Errorf("invalid template block: block kind is required")
	}

	// Find and update the block
	found := false
	for i := range workspace.Settings.TemplateBlocks {
		if workspace.Settings.TemplateBlocks[i].ID == block.ID {
			// Preserve Created timestamp
			block.Created = workspace.Settings.TemplateBlocks[i].Created
			block.Updated = time.Now().UTC()
			workspace.Settings.TemplateBlocks[i] = *block
			found = true
			break
		}
	}

	if !found {
		return &domain.ErrTemplateBlockNotFound{Message: fmt.Sprintf("template block with id %s not found", block.ID)}
	}

	workspace.UpdatedAt = time.Now().UTC()

	// Update workspace
	if err := s.repo.Update(ctx, workspace); err != nil {
		s.logger.WithField("workspace_id", workspaceID).WithField("block_id", block.ID).WithField("error", err.Error()).Error("Failed to update template block")
		return fmt.Errorf("failed to update template block: %w", err)
	}

	return nil
}

func (s *TemplateBlockService) DeleteTemplateBlock(ctx context.Context, workspaceID string, id string) error {
	// Authenticate user for workspace
	var err error
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Check permission for writing templates
	if !userWorkspace.HasPermission(domain.PermissionResourceTemplates, domain.PermissionTypeWrite) {
		return domain.NewPermissionError(
			domain.PermissionResourceTemplates,
			domain.PermissionTypeWrite,
			"Insufficient permissions: write access to templates required",
		)
	}

	// Get the existing workspace
	workspace, err := s.repo.GetByID(ctx, workspaceID)
	if err != nil {
		s.logger.WithField("workspace_id", workspaceID).WithField("error", err.Error()).Error("Failed to get workspace")
		return fmt.Errorf("failed to get workspace: %w", err)
	}

	// Find and remove the block
	found := false
	newBlocks := make([]domain.TemplateBlock, 0, len(workspace.Settings.TemplateBlocks))
	for _, block := range workspace.Settings.TemplateBlocks {
		if block.ID == id {
			found = true
			continue
		}
		newBlocks = append(newBlocks, block)
	}

	if !found {
		return &domain.ErrTemplateBlockNotFound{Message: fmt.Sprintf("template block with id %s not found", id)}
	}

	workspace.Settings.TemplateBlocks = newBlocks
	workspace.UpdatedAt = time.Now().UTC()

	// Update workspace
	if err := s.repo.Update(ctx, workspace); err != nil {
		s.logger.WithField("workspace_id", workspaceID).WithField("block_id", id).WithField("error", err.Error()).Error("Failed to delete template block")
		return fmt.Errorf("failed to delete template block: %w", err)
	}

	return nil
}
