package service

import (
	"context"
	"fmt"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
)

// The templates the demo's automations send, all built from the template gallery
// (demo_gallery_templates.go).
//
// A template that silently never exists leaves the automation referencing it with a dangling
// template_id, so the limits on Template.Name (255 characters) and Template.ID (32) are
// load-bearing. The template IDs are the demoTemplate* constants declared alongside the
// automations that reference them: the two halves have to agree.

// demoAutomationTemplates builds the templates the demo's automations reference. It is split from
// createAutomationTemplates so the definitions can be inspected without a template service: the
// assertions that matter (name length, ID charset, category, translations) are made against the
// definitions themselves.
func (s *DemoService) demoAutomationTemplates(workspaceID string) ([]*domain.Template, error) {
	templates := make([]*domain.Template, 0, len(demoGalleryAutomationTemplates))
	for _, spec := range demoGalleryAutomationTemplates {
		template, err := s.buildDemoGalleryTemplate(workspaceID, spec)
		if err != nil {
			return nil, err
		}
		templates = append(templates, template)
	}
	return templates, nil
}

// createAutomationTemplates creates the templates the demo's automations send.
//
// A failure is logged and skipped rather than returned, matching
// createSampleTemplates: one rejected template must not cost the demo the rest of
// its seed.
func (s *DemoService) createAutomationTemplates(ctx context.Context, workspaceID string) error {
	s.logger.WithField("workspace_id", workspaceID).Info("Creating automation templates")

	templates, err := s.demoAutomationTemplates(workspaceID)
	if err != nil {
		return fmt.Errorf("demo: automation templates: %w", err)
	}
	for _, template := range templates {
		if err := s.templateService.CreateTemplate(ctx, workspaceID, withSubjectPreviews(template)); err != nil {
			return fmt.Errorf("demo: automation template %s: %w", template.ID, err)
		}
	}

	s.logger.WithField("workspace_id", workspaceID).Info("Automation templates created successfully")
	return nil
}

// mjPreviewText returns the content of the tree's mj-preview block, nil when it has none.
func mjPreviewText(block notifuse_mjml.EmailBlock) *string {
	if block == nil {
		return nil
	}
	if block.GetType() == notifuse_mjml.MJMLComponentMjPreview {
		if content := block.GetContent(); content != nil && *content != "" {
			text := *content
			return &text
		}
		return nil
	}
	for _, child := range block.GetChildren() {
		if text := mjPreviewText(child); text != nil {
			return text
		}
	}
	return nil
}

// withSubjectPreviews gives a demo template, and each of its translations, the preview text its
// mj-preview block already holds. The console requires a preview text in every language: without
// it, a duplicated demo template could not be saved until each one was typed in again.
func withSubjectPreviews(template *domain.Template) *domain.Template {
	fill := func(email *domain.EmailTemplate) {
		if email != nil && email.SubjectPreview == nil {
			email.SubjectPreview = mjPreviewText(email.VisualEditorTree)
		}
	}
	fill(template.Email)
	for lang, translation := range template.Translations {
		fill(translation.Email)
		template.Translations[lang] = translation
	}
	return template
}
