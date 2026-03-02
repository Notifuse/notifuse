package service

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
)

type TemplateService struct {
	repo          domain.TemplateRepository
	workspaceRepo domain.WorkspaceRepository
	authService   domain.AuthService
	logger        logger.Logger
	apiEndpoint   string
}

// updateEmailMetadataBlocks updates mj-title and mj-preview blocks in the email tree
// based on template name and subject preview
func (s *TemplateService) updateEmailMetadataBlocks(template *domain.Template) {
	if template.Email == nil {
		return
	}

	// Code mode: override mj-title/mj-preview in the raw MJML source string
	if template.Email.EditorMode == domain.EditorModeCode {
		if template.Email.MjmlSource != nil && *template.Email.MjmlSource != "" {
			mjml := *template.Email.MjmlSource
			mjml = overrideMjmlTag(mjml, "mj-title", template.Name)

			previewText := template.Name
			if template.Email.SubjectPreview != nil && *template.Email.SubjectPreview != "" {
				previewText = *template.Email.SubjectPreview
			}
			mjml = overrideMjmlTag(mjml, "mj-preview", previewText)

			template.Email.MjmlSource = &mjml
		}
		return
	}

	// Visual mode: traverse block tree (existing logic)
	if template.Email.VisualEditorTree == nil {
		return
	}

	s.updateBlockContentRecursively(template.Email.VisualEditorTree, notifuse_mjml.MJMLComponentMjTitle, template.Name)

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

// Pre-compiled regexes for MJML structural tags (don't depend on tagName)
var (
	mjmlHeadRe = regexp.MustCompile(`(?i)<mj-head[^>]*>`)
	mjmlRootRe = regexp.MustCompile(`(?i)<mjml[^>]*>`)
)

// escapeXMLElementContent escapes &, <, > for safe insertion as XML element text content.
// It does not escape quotes since they don't need escaping in element content (only in attributes).
func escapeXMLElementContent(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// overrideMjmlTag replaces the content of an MJML tag in raw MJML source.
// If tag exists: replace first occurrence. If not: inject after <mj-head>. If no <mj-head>: create one.
// tagName is escaped with regexp.QuoteMeta to prevent regex injection.
func overrideMjmlTag(mjml string, tagName string, content string) string {
	escaped := escapeXMLElementContent(content)
	quotedTag := regexp.QuoteMeta(tagName)

	// Try to replace first occurrence of existing tag
	re := regexp.MustCompile(`(?i)(<` + quotedTag + `\s*>)([\s\S]*?)(</` + quotedTag + `\s*>)`)
	loc := re.FindStringSubmatchIndex(mjml)
	if loc != nil {
		// loc[2]:loc[3] = opening tag, loc[6]:loc[7] = closing tag
		openTag := mjml[loc[2]:loc[3]]
		closeTag := mjml[loc[6]:loc[7]]
		return mjml[:loc[0]] + openTag + escaped + closeTag + mjml[loc[1]:]
	}

	// Tag not found — inject into existing <mj-head> (first occurrence only)
	newTag := fmt.Sprintf("<%s>%s</%s>", tagName, escaped, tagName)
	loc = mjmlHeadRe.FindStringIndex(mjml)
	if loc != nil {
		return mjml[:loc[1]] + "\n    " + newTag + mjml[loc[1]:]
	}

	// No <mj-head> — create one after <mjml> (first occurrence only)
	loc = mjmlRootRe.FindStringIndex(mjml)
	if loc != nil {
		return mjml[:loc[1]] + "\n  <mj-head>\n    " + newTag + "\n  </mj-head>" + mjml[loc[1]:]
	}

	return mjml
}

func NewTemplateService(repo domain.TemplateRepository, workspaceRepo domain.WorkspaceRepository, authService domain.AuthService, logger logger.Logger, apiEndpoint string) *TemplateService {
	return &TemplateService{
		repo:          repo,
		workspaceRepo: workspaceRepo,
		authService:   authService,
		logger:        logger,
		apiEndpoint:   apiEndpoint,
	}
}

// validateTranslationLanguages checks that all translation language keys are in the workspace's configured languages.
func (s *TemplateService) validateTranslationLanguages(ctx context.Context, workspaceID string, translations map[string]domain.TemplateTranslation) error {
	if len(translations) == 0 {
		return nil
	}

	workspace, err := s.workspaceRepo.GetByID(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace: %w", err)
	}
	if workspace == nil {
		return fmt.Errorf("workspace not found: %s", workspaceID)
	}

	// Build allowed languages: use configured Languages, or fall back to DefaultLanguage only
	allowedLangs := make(map[string]bool)
	if len(workspace.Settings.Languages) > 0 {
		for _, lang := range workspace.Settings.Languages {
			allowedLangs[lang] = true
		}
	} else {
		allowedLangs[workspace.Settings.DefaultLanguage] = true
	}

	for lang := range translations {
		if !allowedLangs[lang] {
			return fmt.Errorf("translation language '%s' is not in workspace's configured languages", lang)
		}
	}

	return nil
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

	// Cross-validate translation languages against workspace languages
	if err := s.validateTranslationLanguages(ctx, workspaceID, template.Translations); err != nil {
		return err
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

func (s *TemplateService) GetTemplates(ctx context.Context, workspaceID string, category string, channel string) ([]*domain.Template, error) {
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
	templates, err := s.repo.GetTemplates(ctx, workspaceID, category, channel)
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

	// Verify editor_mode hasn't changed (prevent switching between visual and code)
	if template.Email != nil && existingTemplate.Email != nil {
		existingMode := existingTemplate.Email.EditorMode
		if existingMode == "" {
			existingMode = domain.EditorModeVisual
		}
		newMode := template.Email.EditorMode
		if newMode == "" {
			newMode = domain.EditorModeVisual
		}
		if existingMode != newMode {
			return &domain.ErrEditorModeChange{Message: fmt.Sprintf("cannot change editor mode: template was created in '%s' mode", existingMode)}
		}
	}

	// Update mj-title and mj-preview blocks with template metadata
	s.updateEmailMetadataBlocks(template)

	// Validate template
	if err := template.Validate(); err != nil {
		return fmt.Errorf("invalid template: %w", err)
	}

	// Cross-validate translation languages against workspace languages
	if err := s.validateTranslationLanguages(ctx, workspaceID, template.Translations); err != nil {
		return err
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

	// Get the template to check if it's integration-managed
	template, err := s.repo.GetTemplateByID(ctx, workspaceID, id, 0)
	if err != nil {
		if _, ok := err.(*domain.ErrTemplateNotFound); ok {
			return err
		}
		s.logger.WithField("template_id", id).Error(fmt.Sprintf("Failed to get template: %v", err))
		return fmt.Errorf("failed to get template: %w", err)
	}

	// Prevent deletion of integration-managed templates
	if template.IntegrationID != nil && *template.IntegrationID != "" {
		return fmt.Errorf("cannot delete integration-managed template: template is managed by integration %s", *template.IntegrationID)
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
		if user := ctx.Value(domain.WorkspaceUserKey(payload.WorkspaceID)); user == nil {
			// Authenticate user for workspace
			var userWorkspace *domain.UserWorkspace
			var err error
			_, _, userWorkspace, err = s.authService.AuthenticateUserForWorkspace(ctx, payload.WorkspaceID)
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
		}
	}

	// Set endpoint as fallback if not already set
	if payload.TrackingSettings.Endpoint == "" {
		payload.TrackingSettings.Endpoint = s.apiEndpoint
	}

	return notifuse_mjml.CompileTemplate(payload)
}
