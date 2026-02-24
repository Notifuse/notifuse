package service

import (
	"context"
	"fmt"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
)

type WorkspaceTranslationService struct {
	repo        domain.WorkspaceTranslationRepository
	authService domain.AuthService
	logger      logger.Logger
}

func NewWorkspaceTranslationService(
	repo domain.WorkspaceTranslationRepository,
	authService domain.AuthService,
	logger logger.Logger,
) *WorkspaceTranslationService {
	return &WorkspaceTranslationService{
		repo:        repo,
		authService: authService,
		logger:      logger,
	}
}

func (s *WorkspaceTranslationService) Upsert(ctx context.Context, req domain.UpsertWorkspaceTranslationRequest) error {
	if err := req.Validate(); err != nil {
		return err
	}

	if ctx.Value(domain.SystemCallKey) == nil {
		var err error
		var userWorkspace *domain.UserWorkspace
		ctx, _, userWorkspace, err = s.authService.AuthenticateUserForWorkspace(ctx, req.WorkspaceID)
		if err != nil {
			return fmt.Errorf("failed to authenticate user: %w", err)
		}
		if !userWorkspace.HasPermission(domain.PermissionResourceWorkspace, domain.PermissionTypeWrite) {
			return domain.NewPermissionError(
				domain.PermissionResourceWorkspace,
				domain.PermissionTypeWrite,
				"Insufficient permissions: write access to workspace required",
			)
		}
	}

	now := time.Now().UTC()
	translation := &domain.WorkspaceTranslation{
		Locale:    req.Locale,
		Content:   req.Content,
		CreatedAt: now,
		UpdatedAt: now,
	}

	return s.repo.Upsert(ctx, req.WorkspaceID, translation)
}

func (s *WorkspaceTranslationService) List(ctx context.Context, workspaceID string) ([]*domain.WorkspaceTranslation, error) {
	if ctx.Value(domain.SystemCallKey) == nil {
		var err error
		var userWorkspace *domain.UserWorkspace
		ctx, _, userWorkspace, err = s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
		if err != nil {
			return nil, fmt.Errorf("failed to authenticate user: %w", err)
		}
		if !userWorkspace.HasPermission(domain.PermissionResourceWorkspace, domain.PermissionTypeRead) {
			return nil, domain.NewPermissionError(
				domain.PermissionResourceWorkspace,
				domain.PermissionTypeRead,
				"Insufficient permissions: read access to workspace required",
			)
		}
	}

	return s.repo.List(ctx, workspaceID)
}

func (s *WorkspaceTranslationService) GetByLocale(ctx context.Context, workspaceID string, locale string) (*domain.WorkspaceTranslation, error) {
	return s.repo.GetByLocale(ctx, workspaceID, locale)
}

func (s *WorkspaceTranslationService) Delete(ctx context.Context, workspaceID string, locale string) error {
	if ctx.Value(domain.SystemCallKey) == nil {
		var err error
		var userWorkspace *domain.UserWorkspace
		ctx, _, userWorkspace, err = s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
		if err != nil {
			return fmt.Errorf("failed to authenticate user: %w", err)
		}
		if !userWorkspace.HasPermission(domain.PermissionResourceWorkspace, domain.PermissionTypeWrite) {
			return domain.NewPermissionError(
				domain.PermissionResourceWorkspace,
				domain.PermissionTypeWrite,
				"Insufficient permissions: write access to workspace required",
			)
		}
	}

	return s.repo.Delete(ctx, workspaceID, locale)
}
