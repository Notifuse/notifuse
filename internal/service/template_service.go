package service

import (
	"context"
	"fmt"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
)

type TemplateService struct {
	repo        domain.TemplateRepository
	authService domain.AuthService
	logger      logger.Logger
	apiEndpoint string
}

// updateEmailMetadataBlocks updates mj-title and mj-preview blocks in the email tree
// based on template name and subject preview
func (s *TemplateService) updateEmailMetadataBlocks(template *domain.Template) {
	if template.Email == nil || template.Email.VisualEditorTree == nil {
		return
	}

	// Find mj-title and mj-preview blocks and update their content
	s.updateBlockContentRecursively(template.Email.VisualEditorTree, notifuse_mjml.MJMLComponentMjTitle, template.Name)

	// Use subject preview if available, otherwise use template name as fallback
	previewText := template.Name
	if template.Email.SubjectPreview != nil && *template.Email.SubjectPreview != "" {
		previewText = *template.Email.SubjectPreview
	}
	s.updateBlockContentRecursively(template.Email.VisualEditorTree, notifuse_mjml.MJMLComponentMjPreview, previewText)
}

// updateBlockContentRecursively traverses the email block tree and updates content for blocks of the specified type
func (s *TemplateService) updateBlockContentRecursively(block notifuse_mjml.EmailBlock, blockType notifuse_mjml.MJMLComponentType, content string) {
	if block == nil {
		return
	}

	// Check if this is the block type we're looking for
	if block.GetType() == blockType {
		switch typedBlock := block.(type) {
		case *notifuse_mjml.MJTitleBlock:
			typedBlock.Content = &content
		case *notifuse_mjml.MJPreviewBlock:
			typedBlock.Content = &content
		}
	}

	// Recursively check children
	children := block.GetChildren()
	for _, child := range children {
		s.updateBlockContentRecursively(child, blockType, content)
	}
}

func NewTemplateService(repo domain.TemplateRepository, authService domain.AuthService, logger logger.Logger, apiEndpoint string) *TemplateService {
	return &TemplateService{
		repo:        repo,
		authService: authService,
		logger:      logger,
		apiEndpoint: apiEndpoint,
	}
}

func (s *TemplateService) CreateTemplate(ctx context.Context, workspaceID string, template *domain.Template) error {
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

	// Set initial version and timestamps
	template.Version = 1
	now := time.Now().UTC()
	template.CreatedAt = now
	template.UpdatedAt = now

	// Update mj-title and mj-preview blocks with template metadata
	s.updateEmailMetadataBlocks(template)

	// Validate template after setting required fields
	if err := template.Validate(); err != nil {
		return fmt.Errorf("invalid template: %w", err)
	}

	// Create template in repository
	if err := s.repo.CreateTemplate(ctx, workspaceID, template); err != nil {
		s.logger.WithField("template_id", template.ID).Error(fmt.Sprintf("Failed to create template: %v", err))
		return fmt.Errorf("failed to create template: %w", err)
	}

	return nil
}

func (s *TemplateService) GetTemplateByID(ctx context.Context, workspaceID string, id string, version int64) (*domain.Template, error) {
	// Check if this is a system call that should bypass authentication
	var userWorkspace *domain.UserWorkspace
	if ctx.Value(domain.SystemCallKey) == nil {
		// Authenticate user for workspace for regular calls
		var err error
		ctx, _, userWorkspace, err = s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
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
	}

	// Get template by ID
	template, err := s.repo.GetTemplateByID(ctx, workspaceID, id, version)
	if err != nil {
		if _, ok := err.(*domain.ErrTemplateNotFound); ok {
			return nil, err
		}
		s.logger.WithField("template_id", id).Error(fmt.Sprintf("Failed to get template: %v", err))
		return nil, fmt.Errorf("failed to get template: %w", err)
	}

	return template, nil
}

func (s *TemplateService) GetTemplates(ctx context.Context, workspaceID string, category string) ([]*domain.Template, error) {
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

	// Get templates
	templates, err := s.repo.GetTemplates(ctx, workspaceID, category)
	if err != nil {
		s.logger.Error(fmt.Sprintf("Failed to get templates: %v", err))
		return nil, fmt.Errorf("failed to get templates: %w", err)
	}

	return templates, nil
}

func (s *TemplateService) UpdateTemplate(ctx context.Context, workspaceID string, template *domain.Template) error {
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

	// Check if template exists
	existingTemplate, err := s.repo.GetTemplateByID(ctx, workspaceID, template.ID, 0)
	if err != nil {
		if _, ok := err.(*domain.ErrTemplateNotFound); ok {
			return err
		}
		s.logger.WithField("template_id", template.ID).Error(fmt.Sprintf("Failed to check if template exists: %v", err))
		return fmt.Errorf("failed to check if template exists: %w", err)
	}

	// Set version from existing template *before* validation to satisfy the check
	template.Version = existingTemplate.Version

	// Update mj-title and mj-preview blocks with template metadata
	s.updateEmailMetadataBlocks(template)

	// Validate template
	if err := template.Validate(); err != nil {
		return fmt.Errorf("invalid template: %w", err)
	}

	// Preserve creation time from existing template
	template.CreatedAt = existingTemplate.CreatedAt
	template.UpdatedAt = time.Now().UTC()

	// Update template (this will create a new version in the repo)
	if err := s.repo.UpdateTemplate(ctx, workspaceID, template); err != nil {
		s.logger.WithField("template_id", template.ID).Error(fmt.Sprintf("Failed to update template: %v", err))
		return fmt.Errorf("failed to update template: %w", err)
	}

	return nil
}

func (s *TemplateService) DeleteTemplate(ctx context.Context, workspaceID string, id string) error {
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

	// Delete template
	if err := s.repo.DeleteTemplate(ctx, workspaceID, id); err != nil {
		if _, ok := err.(*domain.ErrTemplateNotFound); ok {
			return err
		}
		s.logger.WithField("template_id", id).Error(fmt.Sprintf("Failed to delete template: %v", err))
		return fmt.Errorf("failed to delete template: %w", err)
	}

	return nil
}

func (s *TemplateService) CompileTemplate(ctx context.Context, payload domain.CompileTemplateRequest) (*domain.CompileTemplateResponse, error) {
	// Check if this is a system call that should bypass authentication
	if ctx.Value(domain.SystemCallKey) == nil {
		// Check if user is already authenticated in context
		if user := ctx.Value("authenticated_user"); user == nil {
			// Authenticate user for workspace
			var user *domain.User
			var userWorkspace *domain.UserWorkspace
			var err error
			ctx, user, userWorkspace, err = s.authService.AuthenticateUserForWorkspace(ctx, payload.WorkspaceID)
			if err != nil {
				// Return standard Go error for non-compilation issues
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

			// Store user in context for future use
			ctx = context.WithValue(ctx, "authenticated_user", user)
		}
	}

	// Set endpoint as fallback if not already set
	if payload.TrackingSettings.Endpoint == "" {
		payload.TrackingSettings.Endpoint = s.apiEndpoint
	}

	return notifuse_mjml.CompileTemplate(payload)
}
