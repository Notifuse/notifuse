package service_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	domainmocks "github.com/Notifuse/notifuse/internal/domain/mocks" // Corrected import path
	"github.com/Notifuse/notifuse/internal/service"                  // Added logger import
	"github.com/Notifuse/notifuse/pkg/logger"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks" // Corrected import path
	"github.com/Notifuse/notifuse/pkg/notifuse_mjml"
	"github.com/golang/mock/gomock" // Added gomock import
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	// Keep testify/assert
)

// Updated setup function to use gomock controller
func setupTemplateServiceTest(ctrl *gomock.Controller) (*service.TemplateService, *domainmocks.MockTemplateRepository, *domainmocks.MockWorkspaceRepository, *domainmocks.MockAuthService, *pkgmocks.MockLogger) {
	mockRepo := domainmocks.NewMockTemplateRepository(ctrl)
	mockWorkspaceRepo := domainmocks.NewMockWorkspaceRepository(ctrl)
	mockAuthService := domainmocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	templateService := service.NewTemplateService(mockRepo, mockWorkspaceRepo, mockAuthService, mockLogger, "https://api.example.com")
	return templateService, mockRepo, mockWorkspaceRepo, mockAuthService, mockLogger
}

// Gomock matcher for validating the template passed to CreateTemplate
type templateMatcher struct {
	expected *domain.Template
}

func (m *templateMatcher) Matches(x interface{}) bool {
	tmpl, ok := x.(*domain.Template)
	if !ok {
		return false
	}
	// Check essential fields and that Version is set to 1
	return tmpl.ID == m.expected.ID &&
		tmpl.Name == m.expected.Name &&
		tmpl.Channel == m.expected.Channel &&
		tmpl.Category == m.expected.Category &&
		tmpl.Email != nil &&
		tmpl.Email.Subject == m.expected.Email.Subject &&
		tmpl.Version == 1 // Crucial check
}

func (m *templateMatcher) String() string {
	return fmt.Sprintf("is a template with ID %s and version 1", m.expected.ID)
}

func EqTemplateWithVersion1(expected *domain.Template) gomock.Matcher {
	return &templateMatcher{expected: expected}
}

func TestTemplateService_CreateTemplate(t *testing.T) {
	ctx := context.Background()
	workspaceID := "ws-123"
	userID := "user-456"
	templateID := "tmpl-abc"
	templateToCreate := &domain.Template{
		ID:       templateID,
		Name:     "Test Template",
		Channel:  "email",
		Category: "transactional",
		Email: &domain.EmailTemplate{
			SenderID:        "sender-123",
			Subject:         "Test Email",
			CompiledPreview: "<p>Test</p>",
			VisualEditorTree: func() notifuse_mjml.EmailBlock {
				bodyBase := notifuse_mjml.NewBaseBlock("body", notifuse_mjml.MJMLComponentMjBody)
				bodyBlock := &notifuse_mjml.MJBodyBlock{BaseBlock: bodyBase}
				rootBase := notifuse_mjml.NewBaseBlock("root", notifuse_mjml.MJMLComponentMjml)
				rootBase.Children = []notifuse_mjml.EmailBlock{bodyBlock}
				return &notifuse_mjml.MJMLBlock{BaseBlock: rootBase}
			}(),
		},
		// Version should be set to 1 by the service
		// CreatedAt and UpdatedAt should be set by the service
	}

	t.Run("Success", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		templateService, mockRepo, _, mockAuthService, _ := setupTemplateServiceTest(ctrl)
		templateToPass := *templateToCreate // Use a copy

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceTemplates: {Read: true, Write: true},
			},
		}, nil)
		// Expect CreateTemplate with Version 1 set
		mockRepo.EXPECT().CreateTemplate(ctx, workspaceID, EqTemplateWithVersion1(&templateToPass)).Return(nil)

		err := templateService.CreateTemplate(ctx, workspaceID, &templateToPass)

		assert.NoError(t, err)
		assert.Equal(t, int64(1), templateToPass.Version)
		assert.WithinDuration(t, time.Now().UTC(), templateToPass.CreatedAt, 5*time.Second)
		assert.WithinDuration(t, time.Now().UTC(), templateToPass.UpdatedAt, 5*time.Second)
	})

	t.Run("Authentication Failure", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		templateService, _, _, mockAuthService, _ := setupTemplateServiceTest(ctrl)
		authErr := errors.New("auth error")
		templateToPass := *templateToCreate // Use a copy

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, nil, nil, authErr)

		err := templateService.CreateTemplate(ctx, workspaceID, &templateToPass)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to authenticate user")
		assert.ErrorIs(t, err, authErr)
	})

	t.Run("Validation Failure - Missing Name", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		templateService, _, _, mockAuthService, _ := setupTemplateServiceTest(ctrl)
		invalidTemplate := *templateToCreate // Copy
		invalidTemplate.Name = ""            // Make invalid

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceTemplates: {Read: true, Write: true},
			},
		}, nil)

		err := templateService.CreateTemplate(ctx, workspaceID, &invalidTemplate)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid template: name is required")
	})

	t.Run("Validation Failure - Missing Email Details", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		templateService, _, _, mockAuthService, _ := setupTemplateServiceTest(ctrl)
		invalidTemplate := *templateToCreate // Copy
		invalidTemplate.Email = nil          // Make invalid

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceTemplates: {Read: true, Write: true},
			},
		}, nil)

		err := templateService.CreateTemplate(ctx, workspaceID, &invalidTemplate)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid template: email is required")
	})

	t.Run("Repository Failure", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		templateService, mockRepo, _, mockAuthService, mockLogger := setupTemplateServiceTest(ctrl)
		repoErr := errors.New("db error")
		templateToPass := *templateToCreate // Use a copy

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceTemplates: {Read: true, Write: true},
			},
		}, nil)
		mockRepo.EXPECT().CreateTemplate(ctx, workspaceID, gomock.Any()).Return(repoErr)
		mockLogger.EXPECT().WithField("template_id", templateID).Return(mockLogger)
		mockLogger.EXPECT().Error(fmt.Sprintf("Failed to create template: %v", repoErr)).Return()

		err := templateService.CreateTemplate(ctx, workspaceID, &templateToPass)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create template")
		assert.ErrorIs(t, err, repoErr)
	})

	t.Run("Translation language not in workspace languages", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		templateService, _, mockWorkspaceRepo, mockAuthService, _ := setupTemplateServiceTest(ctrl)

		templateWithTranslation := &domain.Template{
			ID:       templateID,
			Name:     "Test Template",
			Channel:  "email",
			Category: "transactional",
			Email: &domain.EmailTemplate{
				SenderID:        "sender-123",
				Subject:         "Test Email",
				CompiledPreview: "<p>Test</p>",
				VisualEditorTree: func() notifuse_mjml.EmailBlock {
					bodyBase := notifuse_mjml.NewBaseBlock("body", notifuse_mjml.MJMLComponentMjBody)
					bodyBlock := &notifuse_mjml.MJBodyBlock{BaseBlock: bodyBase}
					rootBase := notifuse_mjml.NewBaseBlock("root", notifuse_mjml.MJMLComponentMjml)
					rootBase.Children = []notifuse_mjml.EmailBlock{bodyBlock}
					return &notifuse_mjml.MJMLBlock{BaseBlock: rootBase}
				}(),
			},
			Translations: map[string]domain.TemplateTranslation{
				"de": {Email: &domain.EmailTemplate{
					SenderID:        "sender-123",
					Subject:         "Betreff DE",
					CompiledPreview: "<p>Test DE</p>",
					VisualEditorTree: func() notifuse_mjml.EmailBlock {
						bodyBase := notifuse_mjml.NewBaseBlock("body", notifuse_mjml.MJMLComponentMjBody)
						bodyBlock := &notifuse_mjml.MJBodyBlock{BaseBlock: bodyBase}
						rootBase := notifuse_mjml.NewBaseBlock("root", notifuse_mjml.MJMLComponentMjml)
						rootBase.Children = []notifuse_mjml.EmailBlock{bodyBlock}
						return &notifuse_mjml.MJMLBlock{BaseBlock: rootBase}
					}(),
				}},
			},
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceTemplates: {Read: true, Write: true},
			},
		}, nil)

		// Workspace only allows "en" and "fr"
		mockWorkspaceRepo.EXPECT().GetByID(ctx, workspaceID).Return(&domain.Workspace{
			ID:   workspaceID,
			Name: "Test Workspace",
			Settings: domain.WorkspaceSettings{
				Timezone:        "UTC",
				DefaultLanguage: "en",
				Languages:       []string{"en", "fr"},
			},
		}, nil)

		err := templateService.CreateTemplate(ctx, workspaceID, templateWithTranslation)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "translation language 'de' is not in workspace's configured languages")
	})

	t.Run("Translation language allowed by workspace", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		templateService, mockRepo, mockWorkspaceRepo, mockAuthService, _ := setupTemplateServiceTest(ctrl)

		templateWithTranslation := &domain.Template{
			ID:       templateID,
			Name:     "Test Template",
			Channel:  "email",
			Category: "transactional",
			Email: &domain.EmailTemplate{
				SenderID:        "sender-123",
				Subject:         "Test Email",
				CompiledPreview: "<p>Test</p>",
				VisualEditorTree: func() notifuse_mjml.EmailBlock {
					bodyBase := notifuse_mjml.NewBaseBlock("body", notifuse_mjml.MJMLComponentMjBody)
					bodyBlock := &notifuse_mjml.MJBodyBlock{BaseBlock: bodyBase}
					rootBase := notifuse_mjml.NewBaseBlock("root", notifuse_mjml.MJMLComponentMjml)
					rootBase.Children = []notifuse_mjml.EmailBlock{bodyBlock}
					return &notifuse_mjml.MJMLBlock{BaseBlock: rootBase}
				}(),
			},
			Translations: map[string]domain.TemplateTranslation{
				"fr": {Email: &domain.EmailTemplate{
					SenderID:        "sender-123",
					Subject:         "Sujet FR",
					CompiledPreview: "<p>Test FR</p>",
					VisualEditorTree: func() notifuse_mjml.EmailBlock {
						bodyBase := notifuse_mjml.NewBaseBlock("body", notifuse_mjml.MJMLComponentMjBody)
						bodyBlock := &notifuse_mjml.MJBodyBlock{BaseBlock: bodyBase}
						rootBase := notifuse_mjml.NewBaseBlock("root", notifuse_mjml.MJMLComponentMjml)
						rootBase.Children = []notifuse_mjml.EmailBlock{bodyBlock}
						return &notifuse_mjml.MJMLBlock{BaseBlock: rootBase}
					}(),
				}},
			},
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceTemplates: {Read: true, Write: true},
			},
		}, nil)

		// Workspace allows "en" and "fr"
		mockWorkspaceRepo.EXPECT().GetByID(ctx, workspaceID).Return(&domain.Workspace{
			ID:   workspaceID,
			Name: "Test Workspace",
			Settings: domain.WorkspaceSettings{
				Timezone:        "UTC",
				DefaultLanguage: "en",
				Languages:       []string{"en", "fr"},
			},
		}, nil)

		mockRepo.EXPECT().CreateTemplate(ctx, workspaceID, gomock.Any()).Return(nil)

		err := templateService.CreateTemplate(ctx, workspaceID, templateWithTranslation)
		assert.NoError(t, err)
	})

	t.Run("Translation rejected when workspace has empty Languages but non-default key", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		templateService, _, mockWorkspaceRepo, mockAuthService, _ := setupTemplateServiceTest(ctrl)

		templateWithTranslation := &domain.Template{
			ID:       templateID,
			Name:     "Test Template",
			Channel:  "email",
			Category: "transactional",
			Email: &domain.EmailTemplate{
				SenderID:        "sender-123",
				Subject:         "Test Email",
				CompiledPreview: "<p>Test</p>",
				VisualEditorTree: func() notifuse_mjml.EmailBlock {
					bodyBase := notifuse_mjml.NewBaseBlock("body", notifuse_mjml.MJMLComponentMjBody)
					bodyBlock := &notifuse_mjml.MJBodyBlock{BaseBlock: bodyBase}
					rootBase := notifuse_mjml.NewBaseBlock("root", notifuse_mjml.MJMLComponentMjml)
					rootBase.Children = []notifuse_mjml.EmailBlock{bodyBlock}
					return &notifuse_mjml.MJMLBlock{BaseBlock: rootBase}
				}(),
			},
			Translations: map[string]domain.TemplateTranslation{
				"fr": {Email: &domain.EmailTemplate{
					SenderID:        "sender-123",
					Subject:         "Sujet FR",
					CompiledPreview: "<p>FR</p>",
					VisualEditorTree: func() notifuse_mjml.EmailBlock {
						bodyBase := notifuse_mjml.NewBaseBlock("body", notifuse_mjml.MJMLComponentMjBody)
						bodyBlock := &notifuse_mjml.MJBodyBlock{BaseBlock: bodyBase}
						rootBase := notifuse_mjml.NewBaseBlock("root", notifuse_mjml.MJMLComponentMjml)
						rootBase.Children = []notifuse_mjml.EmailBlock{bodyBlock}
						return &notifuse_mjml.MJMLBlock{BaseBlock: rootBase}
					}(),
				}},
			},
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceTemplates: {Read: true, Write: true},
			},
		}, nil)

		// Workspace has no Languages configured, only DefaultLanguage
		mockWorkspaceRepo.EXPECT().GetByID(ctx, workspaceID).Return(&domain.Workspace{
			ID:   workspaceID,
			Name: "Test Workspace",
			Settings: domain.WorkspaceSettings{
				Timezone:        "UTC",
				DefaultLanguage: "en",
				Languages:       nil,
			},
		}, nil)

		err := templateService.CreateTemplate(ctx, workspaceID, templateWithTranslation)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not in workspace's configured languages")
	})
}

func TestTemplateService_GetTemplateByID(t *testing.T) {
	ctx := context.Background()
	workspaceID := "ws-123"
	userID := "user-456"
	templateID := "tmpl-abc"
	version := int64(1)
	now := time.Now().UTC()

	expectedTemplate := &domain.Template{
		ID:        templateID,
		Name:      "Test Template",
		Version:   version,
		Channel:   "email",
		Category:  "transactional",
		CreatedAt: now,
		UpdatedAt: now,
		Email: &domain.EmailTemplate{
			SenderID:        "sender-123",
			Subject:         "Test Email",
			CompiledPreview: "<html><body>Test</body></html>",
			VisualEditorTree: func() notifuse_mjml.EmailBlock {
				bodyBase := notifuse_mjml.NewBaseBlock("body", notifuse_mjml.MJMLComponentMjBody)
				bodyBlock := &notifuse_mjml.MJBodyBlock{BaseBlock: bodyBase}
				rootBase := notifuse_mjml.NewBaseBlock("root", notifuse_mjml.MJMLComponentMjml)
				rootBase.Children = []notifuse_mjml.EmailBlock{bodyBlock}
				return &notifuse_mjml.MJMLBlock{BaseBlock: rootBase}
			}(),
		},
	}

	t.Run("Success", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		templateService, mockRepo, _, mockAuthService, _ := setupTemplateServiceTest(ctrl)

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceTemplates: {Read: true, Write: true},
			},
		}, nil)
		mockRepo.EXPECT().GetTemplateByID(ctx, workspaceID, templateID, version).Return(expectedTemplate, nil)

		template, err := templateService.GetTemplateByID(ctx, workspaceID, templateID, version)

		assert.NoError(t, err)
		assert.Equal(t, expectedTemplate, template)
	})

	t.Run("Authentication Failure", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		templateService, _, _, mockAuthService, _ := setupTemplateServiceTest(ctrl)
		authErr := errors.New("auth error")

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, nil, nil, authErr)

		template, err := templateService.GetTemplateByID(ctx, workspaceID, templateID, version)

		assert.Error(t, err)
		assert.Nil(t, template)
		assert.Contains(t, err.Error(), "failed to authenticate user")
		assert.ErrorIs(t, err, authErr)
	})

	t.Run("System Call Bypasses Authentication", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		templateService, mockRepo, _, _, _ := setupTemplateServiceTest(ctrl)

		// Create a system context that should bypass authentication
		systemCtx := context.WithValue(ctx, domain.SystemCallKey, true)

		// No auth service call expected since this is a system call
		mockRepo.EXPECT().GetTemplateByID(systemCtx, workspaceID, templateID, version).Return(expectedTemplate, nil)

		template, err := templateService.GetTemplateByID(systemCtx, workspaceID, templateID, version)

		assert.NoError(t, err)
		assert.Equal(t, expectedTemplate, template)
	})

	t.Run("Not Found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		templateService, mockRepo, _, mockAuthService, _ := setupTemplateServiceTest(ctrl)
		notFoundErr := &domain.ErrTemplateNotFound{Message: "not found"}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceTemplates: {Read: true, Write: true},
			},
		}, nil)
		mockRepo.EXPECT().GetTemplateByID(ctx, workspaceID, templateID, version).Return(nil, notFoundErr)

		template, err := templateService.GetTemplateByID(ctx, workspaceID, templateID, version)

		assert.Error(t, err)
		assert.Nil(t, template)
		assert.ErrorIs(t, err, notFoundErr)
	})

	t.Run("Repository Failure", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		templateService, mockRepo, _, mockAuthService, mockLogger := setupTemplateServiceTest(ctrl)
		repoErr := errors.New("db error")

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceTemplates: {Read: true, Write: true},
			},
		}, nil)
		mockRepo.EXPECT().GetTemplateByID(ctx, workspaceID, templateID, version).Return(nil, repoErr)
		mockLogger.EXPECT().WithField("template_id", templateID).Return(mockLogger)
		mockLogger.EXPECT().Error(fmt.Sprintf("Failed to get template: %v", repoErr)).Return()

		template, err := templateService.GetTemplateByID(ctx, workspaceID, templateID, version)

		assert.Error(t, err)
		assert.Nil(t, template)
		assert.Contains(t, err.Error(), "failed to get template")
		assert.ErrorIs(t, err, repoErr)
	})
}

func TestTemplateService_GetTemplates(t *testing.T) {
	ctx := context.Background()
	workspaceID := "ws-123"
	userID := "user-456"
	now := time.Now().UTC()

	expectedTemplates := []*domain.Template{
		{
			ID:        "tmpl-abc",
			Name:      "Test Template 1",
			Version:   1,
			Channel:   "email",
			Category:  "transactional",
			CreatedAt: now,
			UpdatedAt: now,
			Email: &domain.EmailTemplate{
				Subject: "Subject 1",
			},
		},
		{
			ID:        "tmpl-def",
			Name:      "Test Template 2",
			Version:   2,
			Channel:   "email",
			Category:  "marketing",
			CreatedAt: now,
			UpdatedAt: now,
			Email: &domain.EmailTemplate{
				Subject: "Subject 2",
			},
		},
	}

	t.Run("Success", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		templateService, mockRepo, _, mockAuthService, _ := setupTemplateServiceTest(ctrl)

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceTemplates: {Read: true, Write: true},
			},
		}, nil)
		mockRepo.EXPECT().GetTemplates(ctx, workspaceID, "", "").Return(expectedTemplates, nil)

		templates, err := templateService.GetTemplates(ctx, workspaceID, "", "")

		assert.NoError(t, err)
		assert.Equal(t, expectedTemplates, templates)
	})

	t.Run("Authentication Failure", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		templateService, _, _, mockAuthService, _ := setupTemplateServiceTest(ctrl)
		authErr := errors.New("auth error")

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, nil, nil, authErr)

		templates, err := templateService.GetTemplates(ctx, workspaceID, "", "")

		assert.Error(t, err)
		assert.Nil(t, templates)
		assert.Contains(t, err.Error(), "failed to authenticate user")
		assert.ErrorIs(t, err, authErr)
	})

	t.Run("Repository Failure", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		templateService, mockRepo, _, mockAuthService, mockLogger := setupTemplateServiceTest(ctrl)
		repoErr := errors.New("db error")

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceTemplates: {Read: true, Write: true},
			},
		}, nil)
		mockRepo.EXPECT().GetTemplates(ctx, workspaceID, "", "").Return(nil, repoErr)
		mockLogger.EXPECT().Error(fmt.Sprintf("Failed to get templates: %v", repoErr)).Return()

		templates, err := templateService.GetTemplates(ctx, workspaceID, "", "")

		assert.Error(t, err)
		assert.Nil(t, templates)
		assert.Contains(t, err.Error(), "failed to get templates")
		assert.ErrorIs(t, err, repoErr)
	})
}

func TestTemplateService_UpdateTemplate(t *testing.T) {
	ctx := context.Background()
	workspaceID := "ws-123"
	userID := "user-456"
	templateID := "tmpl-abc"
	existingCreatedAt := time.Now().Add(-1 * time.Hour).UTC()

	existingTemplate := &domain.Template{
		ID:        templateID,
		Name:      "Old Name",
		Version:   1,
		Channel:   "email",
		Category:  "transactional",
		CreatedAt: existingCreatedAt,
		Email: &domain.EmailTemplate{
			SenderID:        "sender-123",
			Subject:         "Old Subject",
			CompiledPreview: "<p>Old</p>",
			VisualEditorTree: func() notifuse_mjml.EmailBlock {
				bodyBase := notifuse_mjml.NewBaseBlock("body", notifuse_mjml.MJMLComponentMjBody)
				bodyBlock := &notifuse_mjml.MJBodyBlock{BaseBlock: bodyBase}
				rootBase := notifuse_mjml.NewBaseBlock("root", notifuse_mjml.MJMLComponentMjml)
				rootBase.Children = []notifuse_mjml.EmailBlock{bodyBlock}
				return &notifuse_mjml.MJMLBlock{BaseBlock: rootBase}
			}(),
		},
	}

	updatedTemplateData := &domain.Template{
		ID:       templateID,
		Name:     "New Name", // Updated field
		Channel:  "email",
		Category: "transactional",
		Email: &domain.EmailTemplate{
			SenderID:        "sender-123",   // Updated field
			Subject:         "New Subject",  // Updated field
			CompiledPreview: "<h1>New</h1>", // Updated field
			VisualEditorTree: func() notifuse_mjml.EmailBlock {
				bodyBase := notifuse_mjml.NewBaseBlock("body", notifuse_mjml.MJMLComponentMjBody)
				bodyBlock := &notifuse_mjml.MJBodyBlock{BaseBlock: bodyBase}
				rootBase := notifuse_mjml.NewBaseBlock("root", notifuse_mjml.MJMLComponentMjml)
				rootBase.Children = []notifuse_mjml.EmailBlock{bodyBlock}
				return &notifuse_mjml.MJMLBlock{BaseBlock: rootBase}
			}(),
		},
		// Version, CreatedAt, UpdatedAt should be handled by the service
	}

	t.Run("Success", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		templateService, mockRepo, _, mockAuthService, _ := setupTemplateServiceTest(ctrl)
		templateToUpdate := *updatedTemplateData // Use a copy

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceTemplates: {Read: true, Write: true},
			},
		}, nil)
		// GetByID is called first to check existence and preserve CreatedAt (version 0 means latest)
		mockRepo.EXPECT().GetTemplateByID(ctx, workspaceID, templateID, int64(0)).Return(existingTemplate, nil)
		// Expect UpdateTemplate call with correct fields preserved/updated
		mockRepo.EXPECT().UpdateTemplate(ctx, workspaceID, gomock.Any()).DoAndReturn(func(_ context.Context, _ string, tmpl *domain.Template) error {
			assert.Equal(t, templateToUpdate.ID, tmpl.ID)
			assert.Equal(t, templateToUpdate.Name, tmpl.Name)
			assert.Equal(t, templateToUpdate.Channel, tmpl.Channel)
			assert.Equal(t, templateToUpdate.Category, tmpl.Category)
			assert.Equal(t, templateToUpdate.Email, tmpl.Email)                       // Check nested struct
			assert.Equal(t, existingTemplate.CreatedAt, tmpl.CreatedAt)               // Check CreatedAt preserved
			assert.WithinDuration(t, time.Now().UTC(), tmpl.UpdatedAt, 5*time.Second) // Check UpdatedAt is recent
			// Version should be handled by the repository layer during update (not checked here)
			return nil
		})

		err := templateService.UpdateTemplate(ctx, workspaceID, &templateToUpdate)

		assert.NoError(t, err)
		// Check that the passed-in template's CreatedAt and UpdatedAt were updated by the service
		assert.Equal(t, existingTemplate.CreatedAt, templateToUpdate.CreatedAt)
		assert.WithinDuration(t, time.Now().UTC(), templateToUpdate.UpdatedAt, 5*time.Second)
	})

	t.Run("Authentication Failure", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		templateService, _, _, mockAuthService, _ := setupTemplateServiceTest(ctrl)
		authErr := errors.New("auth error")
		templateToUpdate := *updatedTemplateData // Use a copy

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, nil, nil, authErr)

		err := templateService.UpdateTemplate(ctx, workspaceID, &templateToUpdate)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to authenticate user")
		assert.ErrorIs(t, err, authErr)
	})

	t.Run("Get Existing Template Not Found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		templateService, mockRepo, _, mockAuthService, _ := setupTemplateServiceTest(ctrl)
		notFoundErr := &domain.ErrTemplateNotFound{Message: "not found"}
		templateToUpdate := *updatedTemplateData // Use a copy

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceTemplates: {Read: true, Write: true},
			},
		}, nil)
		mockRepo.EXPECT().GetTemplateByID(ctx, workspaceID, templateID, int64(0)).Return(nil, notFoundErr)

		err := templateService.UpdateTemplate(ctx, workspaceID, &templateToUpdate)

		assert.Error(t, err)
		assert.ErrorIs(t, err, notFoundErr) // Service should return the exact error
	})

	t.Run("Get Existing Template Repository Failure", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		templateService, mockRepo, _, mockAuthService, mockLogger := setupTemplateServiceTest(ctrl)
		repoErr := errors.New("get db error")
		templateToUpdate := *updatedTemplateData // Use a copy

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceTemplates: {Read: true, Write: true},
			},
		}, nil)
		mockRepo.EXPECT().GetTemplateByID(ctx, workspaceID, templateID, int64(0)).Return(nil, repoErr)
		mockLogger.EXPECT().WithField("template_id", templateID).Return(mockLogger)
		mockLogger.EXPECT().Error(fmt.Sprintf("Failed to check if template exists: %v", repoErr)).Return()

		err := templateService.UpdateTemplate(ctx, workspaceID, &templateToUpdate)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to check if template exists")
		assert.ErrorIs(t, err, repoErr)
	})

	t.Run("Validation Failure", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		templateService, mockRepo, _, mockAuthService, _ := setupTemplateServiceTest(ctrl)
		invalidTemplate := *updatedTemplateData // Copy
		invalidTemplate.Name = ""               // Make invalid

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceTemplates: {Read: true, Write: true},
			},
		}, nil)
		// Expect GetByID to be called and succeed before validation happens
		mockRepo.EXPECT().GetTemplateByID(ctx, workspaceID, templateID, int64(0)).Return(existingTemplate, nil)

		err := templateService.UpdateTemplate(ctx, workspaceID, &invalidTemplate)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid template: name is required")
	})

	t.Run("Update Repository Failure", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		templateService, mockRepo, _, mockAuthService, mockLogger := setupTemplateServiceTest(ctrl)
		repoErr := errors.New("update db error")
		templateToUpdate := *updatedTemplateData // Use a copy

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceTemplates: {Read: true, Write: true},
			},
		}, nil)
		mockRepo.EXPECT().GetTemplateByID(ctx, workspaceID, templateID, int64(0)).Return(existingTemplate, nil)
		mockRepo.EXPECT().UpdateTemplate(ctx, workspaceID, gomock.Any()).Return(repoErr)
		mockLogger.EXPECT().WithField("template_id", templateID).Return(mockLogger)
		mockLogger.EXPECT().Error(fmt.Sprintf("Failed to update template: %v", repoErr)).Return()

		err := templateService.UpdateTemplate(ctx, workspaceID, &templateToUpdate)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to update template")
		assert.ErrorIs(t, err, repoErr)
	})
}

func TestTemplateService_DeleteTemplate(t *testing.T) {
	ctx := context.Background()
	workspaceID := "ws-123"
	userID := "user-456"
	templateID := "tmpl-abc"

	regularTemplate := &domain.Template{
		ID:       templateID,
		Name:     "Regular Template",
		Channel:  "email",
		Category: "transactional",
		Email: &domain.EmailTemplate{
			Subject: "Test",
		},
		// No IntegrationID, so it can be deleted
	}

	t.Run("Success", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		templateService, mockRepo, _, mockAuthService, _ := setupTemplateServiceTest(ctrl)

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceTemplates: {Read: true, Write: true},
			},
		}, nil)
		// Now expects GetTemplateByID to be called first to check if integration-managed
		mockRepo.EXPECT().GetTemplateByID(ctx, workspaceID, templateID, int64(0)).Return(regularTemplate, nil)
		mockRepo.EXPECT().DeleteTemplate(ctx, workspaceID, templateID).Return(nil)

		err := templateService.DeleteTemplate(ctx, workspaceID, templateID)

		assert.NoError(t, err)
	})

	t.Run("Authentication Failure", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		templateService, _, _, mockAuthService, _ := setupTemplateServiceTest(ctrl)
		authErr := errors.New("auth error")

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, nil, nil, authErr)

		err := templateService.DeleteTemplate(ctx, workspaceID, templateID)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to authenticate user")
		assert.ErrorIs(t, err, authErr)
	})

	t.Run("Cannot Delete Integration-Managed Template", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		templateService, mockRepo, _, mockAuthService, _ := setupTemplateServiceTest(ctrl)
		integrationID := "integration-123"
		integrationManagedTemplate := &domain.Template{
			ID:            templateID,
			Name:          "Integration Template",
			Channel:       "email",
			Category:      "transactional",
			IntegrationID: &integrationID,
			Email: &domain.EmailTemplate{
				Subject: "Test",
			},
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceTemplates: {Read: true, Write: true},
			},
		}, nil)
		mockRepo.EXPECT().GetTemplateByID(ctx, workspaceID, templateID, int64(0)).Return(integrationManagedTemplate, nil)
		// DeleteTemplate should NOT be called

		err := templateService.DeleteTemplate(ctx, workspaceID, templateID)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot delete integration-managed template")
		assert.Contains(t, err.Error(), integrationID)
	})

	t.Run("Get Template Not Found Before Delete", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		templateService, mockRepo, _, mockAuthService, _ := setupTemplateServiceTest(ctrl)
		notFoundErr := &domain.ErrTemplateNotFound{Message: "not found"}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceTemplates: {Read: true, Write: true},
			},
		}, nil)
		mockRepo.EXPECT().GetTemplateByID(ctx, workspaceID, templateID, int64(0)).Return(nil, notFoundErr)

		err := templateService.DeleteTemplate(ctx, workspaceID, templateID)

		assert.Error(t, err)
		assert.ErrorIs(t, err, notFoundErr)
	})

	t.Run("Get Template Repository Failure Before Delete", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		templateService, mockRepo, _, mockAuthService, mockLogger := setupTemplateServiceTest(ctrl)
		repoErr := errors.New("db error")

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceTemplates: {Read: true, Write: true},
			},
		}, nil)
		mockRepo.EXPECT().GetTemplateByID(ctx, workspaceID, templateID, int64(0)).Return(nil, repoErr)
		mockLogger.EXPECT().WithField("template_id", templateID).Return(mockLogger)
		mockLogger.EXPECT().Error(fmt.Sprintf("Failed to get template: %v", repoErr)).Return()

		err := templateService.DeleteTemplate(ctx, workspaceID, templateID)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get template")
		assert.ErrorIs(t, err, repoErr)
	})

	t.Run("Delete Not Found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		templateService, mockRepo, _, mockAuthService, _ := setupTemplateServiceTest(ctrl)
		notFoundErr := &domain.ErrTemplateNotFound{Message: "not found"}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceTemplates: {Read: true, Write: true},
			},
		}, nil)
		mockRepo.EXPECT().GetTemplateByID(ctx, workspaceID, templateID, int64(0)).Return(regularTemplate, nil)
		mockRepo.EXPECT().DeleteTemplate(ctx, workspaceID, templateID).Return(notFoundErr)

		err := templateService.DeleteTemplate(ctx, workspaceID, templateID)

		assert.Error(t, err)
		assert.ErrorIs(t, err, notFoundErr) // Service should return the exact error
	})

	t.Run("Delete Repository Failure", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		templateService, mockRepo, _, mockAuthService, mockLogger := setupTemplateServiceTest(ctrl)
		repoErr := errors.New("db error")

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceTemplates: {Read: true, Write: true},
			},
		}, nil)
		mockRepo.EXPECT().GetTemplateByID(ctx, workspaceID, templateID, int64(0)).Return(regularTemplate, nil)
		mockRepo.EXPECT().DeleteTemplate(ctx, workspaceID, templateID).Return(repoErr)
		mockLogger.EXPECT().WithField("template_id", templateID).Return(mockLogger)
		mockLogger.EXPECT().Error(fmt.Sprintf("Failed to delete template: %v", repoErr)).Return()

		err := templateService.DeleteTemplate(ctx, workspaceID, templateID)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to delete template")
		assert.ErrorIs(t, err, repoErr)
	})
}

// Helper types/funcs from other tests or define locally if needed
type MockLogger struct{}

func (l *MockLogger) Debug(msg string)                                       {}
func (l *MockLogger) Info(msg string)                                        {}
func (l *MockLogger) Warn(msg string)                                        {}
func (l *MockLogger) Error(msg string)                                       {}
func (l *MockLogger) Fatal(msg string)                                       {}
func (l *MockLogger) WithField(key string, value interface{}) logger.Logger  { return l }
func (l *MockLogger) WithFields(fields map[string]interface{}) logger.Logger { return l }

// --- Helper to create a basic text block ---
func createTestTextBlock(id, textContent string) notifuse_mjml.EmailBlock {
	content := textContent
	base := notifuse_mjml.NewBaseBlock(id, notifuse_mjml.MJMLComponentMjText)
	base.Content = &content
	return &notifuse_mjml.MJTextBlock{BaseBlock: base}
}

// --- Helper to create a valid nested structure for testing success ---
func createValidTestTree(textBlock notifuse_mjml.EmailBlock) notifuse_mjml.EmailBlock {
	columnBase := notifuse_mjml.NewBaseBlock("col1", notifuse_mjml.MJMLComponentMjColumn)
	columnBase.Children = []notifuse_mjml.EmailBlock{textBlock}
	columnBlock := &notifuse_mjml.MJColumnBlock{BaseBlock: columnBase}

	sectionBase := notifuse_mjml.NewBaseBlock("sec1", notifuse_mjml.MJMLComponentMjSection)
	sectionBase.Children = []notifuse_mjml.EmailBlock{columnBlock}
	sectionBlock := &notifuse_mjml.MJSectionBlock{BaseBlock: sectionBase}

	bodyBase := notifuse_mjml.NewBaseBlock("body1", notifuse_mjml.MJMLComponentMjBody)
	bodyBase.Children = []notifuse_mjml.EmailBlock{sectionBlock}
	bodyBlock := &notifuse_mjml.MJBodyBlock{BaseBlock: bodyBase}

	rootBase := notifuse_mjml.NewBaseBlock("root", notifuse_mjml.MJMLComponentMjml)
	rootBase.Children = []notifuse_mjml.EmailBlock{bodyBlock}
	return &notifuse_mjml.MJMLBlock{BaseBlock: rootBase}
}

func TestCompileTemplate_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAuthService := domainmocks.NewMockAuthService(ctrl)
	mockRepo := domainmocks.NewMockTemplateRepository(ctrl)
	mockLogger := &MockLogger{}
	mockWorkspaceRepo := domainmocks.NewMockWorkspaceRepository(ctrl)

	svc := service.NewTemplateService(mockRepo, mockWorkspaceRepo, mockAuthService, mockLogger, "https://api.example.com")

	ctx := context.Background()
	workspaceID := "ws_123"
	userID := "user_abc"
	testTree := createValidTestTree(createTestTextBlock("txt1", "Hello {{name}}"))
	testData := notifuse_mjml.MapOfAny{"name": "Tester"}

	// Mock expectations
	mockAuthService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
		UserID:      userID,
		WorkspaceID: workspaceID,
		Role:        "member",
		Permissions: domain.UserPermissions{
			domain.PermissionResourceTemplates: {Read: true, Write: true},
		},
	}, nil)

	// Workspace is loaded to inject the workspace.base_url / website_url template vars
	mockWorkspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(&domain.Workspace{
		ID:       workspaceID,
		Settings: domain.WorkspaceSettings{WebsiteURL: "https://example.com"},
	}, nil)

	// --- Act ---
	resp, err := svc.CompileTemplate(ctx, domain.CompileTemplateRequest{
		WorkspaceID:      workspaceID,
		VisualEditorTree: testTree,
		TemplateData:     testData,
	})

	// --- Assert ---
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.True(t, resp.Success, "Success should be true")
	assert.Nil(t, resp.Error, "Error should be nil on success")
	require.NotNil(t, resp.MJML, "MJML should not be nil on success")
	require.NotNil(t, resp.HTML, "HTML should not be nil on success")

	assert.Contains(t, *resp.MJML, "<mj-section", "MJML should contain <mj-section>")
	assert.Contains(t, *resp.MJML, "<mj-column", "MJML should contain <mj-column>")
	assert.Contains(t, *resp.MJML, "<mj-text", "MJML should contain <mj-text>")
	assert.Contains(t, *resp.MJML, "Hello Tester", "MJML should contain processed liquid variable")

	assert.Contains(t, *resp.HTML, "<html", "HTML should contain <html>")
	assert.Contains(t, *resp.HTML, "Hello Tester", "HTML should contain processed liquid variable")

	// t.Logf("Generated MJML:\n%s", *resp.MJML)
	// t.Logf("Generated HTML:\n%s", *resp.HTML)
}

// Renamed test to focus on TreeToMjml internal errors (like Liquid)
func TestCompileTemplate_TreeToMjmlError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAuthService := domainmocks.NewMockAuthService(ctrl)
	mockRepo := domainmocks.NewMockTemplateRepository(ctrl)
	mockLogger := &MockLogger{}
	mockWorkspaceRepo := domainmocks.NewMockWorkspaceRepository(ctrl)
	svc := service.NewTemplateService(mockRepo, mockWorkspaceRepo, mockAuthService, mockLogger, "https://api.example.com")

	ctx := context.Background()
	workspaceID := "ws_123"
	userID := "user_abc"

	// Create a tree containing a block that will cause TreeToMjml to return an error (e.g., bad liquid)
	invalidContent := "{% invalid tag %}"
	badLiquidBase := notifuse_mjml.NewBaseBlock("badliq", notifuse_mjml.MJMLComponentMjText)
	badLiquidBase.Content = &invalidContent
	badLiquidBlock := &notifuse_mjml.MJTextBlock{BaseBlock: badLiquidBase}
	badLiquidTree := createValidTestTree(badLiquidBlock) // Embed the bad block in a valid structure

	// Mock Auth
	mockAuthService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
		UserID:      userID,
		WorkspaceID: workspaceID,
		Role:        "member",
		Permissions: domain.UserPermissions{
			domain.PermissionResourceTemplates: {Read: true, Write: true},
		},
	}, nil)

	mockWorkspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(&domain.Workspace{
		ID:       workspaceID,
		Settings: domain.WorkspaceSettings{WebsiteURL: "https://example.com"},
	}, nil)

	// --- Act ---
	resp, err := svc.CompileTemplate(ctx, domain.CompileTemplateRequest{
		WorkspaceID:      workspaceID,
		VisualEditorTree: badLiquidTree,
		TemplateData:     notifuse_mjml.MapOfAny{"name": "test"}, // Provide template data to trigger liquid processing
	})

	// --- Assert ---
	require.NoError(t, err, "CompileTemplate should return nil error even on internal failure")
	require.NotNil(t, resp, "CompileTemplate should return a response struct even on internal failure")
	assert.False(t, resp.Success, "Response Success should be false on TreeToMjml failure")
	require.NotNil(t, resp.Error, "Response Error should not be nil on TreeToMjml failure")
	// Check that the error message originates from the TreeToMjml function and indicates a liquid error
	assert.Contains(t, resp.Error.Message, "liquid processing failed for block badliq", "Error message should wrap the liquid error")

	// Note: Testing the specific mjmlgo.Error path (where err is nil but resp.Success is false)
	// would ideally involve mocking mjmlgo.ToHTML or using specific input known to cause mjmlgo.Error.
}

func TestCompileTemplate_AuthError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAuthService := domainmocks.NewMockAuthService(ctrl)
	mockRepo := domainmocks.NewMockTemplateRepository(ctrl)
	mockLogger := &MockLogger{}
	mockWorkspaceRepo := domainmocks.NewMockWorkspaceRepository(ctrl)

	svc := service.NewTemplateService(mockRepo, mockWorkspaceRepo, mockAuthService, mockLogger, "https://api.example.com")

	ctx := context.Background()
	workspaceID := "ws_123"
	// Use a valid tree for this auth error test
	testTree := createValidTestTree(createTestTextBlock("txt1", "Test"))
	authErr := errors.New("authentication failed")

	// Mock expectations
	mockAuthService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(ctx, nil, nil, authErr)

	// --- Act ---
	resp, err := svc.CompileTemplate(ctx, domain.CompileTemplateRequest{
		WorkspaceID:      workspaceID,
		VisualEditorTree: testTree,
		TemplateData:     nil,
	})

	// --- Assert ---
	require.Error(t, err)
	require.Nil(t, resp)
	assert.Contains(t, err.Error(), "failed to authenticate user", "Error message should indicate auth failure")
	assert.ErrorIs(t, err, authErr, "Original auth error should be wrapped")
}

func TestCompileTemplate_SystemCallBypassesAuth(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAuthService := domainmocks.NewMockAuthService(ctrl)
	mockRepo := domainmocks.NewMockTemplateRepository(ctrl)
	mockLogger := &MockLogger{}
	mockWorkspaceRepo := domainmocks.NewMockWorkspaceRepository(ctrl)

	svc := service.NewTemplateService(mockRepo, mockWorkspaceRepo, mockAuthService, mockLogger, "https://api.example.com")

	// Create a system context that should bypass authentication
	ctx := context.WithValue(context.Background(), domain.SystemCallKey, true)
	workspaceID := "ws_123"
	testTree := createValidTestTree(createTestTextBlock("txt1", "Test"))

	// No auth service call expected since this is a system call

	// Workspace is still loaded to inject the workspace template vars
	mockWorkspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(&domain.Workspace{
		ID:       workspaceID,
		Settings: domain.WorkspaceSettings{WebsiteURL: "https://example.com"},
	}, nil)

	// --- Act ---
	resp, err := svc.CompileTemplate(ctx, domain.CompileTemplateRequest{
		WorkspaceID:      workspaceID,
		VisualEditorTree: testTree,
		TemplateData:     nil,
		TrackingSettings: notifuse_mjml.TrackingSettings{},
	})

	// --- Assert ---
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.True(t, resp.Success)
}

func TestCompileTemplate_InvalidTreeData(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAuthService := domainmocks.NewMockAuthService(ctrl)
	mockRepo := domainmocks.NewMockTemplateRepository(ctrl)
	mockLogger := &MockLogger{}
	mockWorkspaceRepo := domainmocks.NewMockWorkspaceRepository(ctrl)

	svc := service.NewTemplateService(mockRepo, mockWorkspaceRepo, mockAuthService, mockLogger, "https://api.example.com")

	ctx := context.Background()
	workspaceID := "ws_123"
	userID := "user_abc"
	invalidTree := &notifuse_mjml.MJMLBlock{
		BaseBlock: notifuse_mjml.NewBaseBlock("root_invalid", notifuse_mjml.MJMLComponentMjml),
	}

	// Mock expectations
	mockAuthService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
		UserID:      userID,
		WorkspaceID: workspaceID,
		Role:        "member",
		Permissions: domain.UserPermissions{
			domain.PermissionResourceTemplates: {Read: true, Write: true},
		},
	}, nil)

	mockWorkspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(&domain.Workspace{
		ID:       workspaceID,
		Settings: domain.WorkspaceSettings{WebsiteURL: "https://example.com"},
	}, nil)

	// --- Act ---
	resp, err := svc.CompileTemplate(ctx, domain.CompileTemplateRequest{
		WorkspaceID:      workspaceID,
		VisualEditorTree: invalidTree,
		TemplateData:     nil,
	})

	// --- Assert ---
	require.NoError(t, err)
	require.NotNil(t, resp)
	// gomjml succeeds with minimal/empty MJML structures and produces valid HTML
	assert.True(t, resp.Success, "gomjml should succeed with empty MJML block")
	require.NotNil(t, resp.HTML, "Response HTML should not be nil")
	assert.NotEmpty(t, *resp.HTML, "HTML output should not be empty")
}

func TestTemplateService_CreateTemplate_CodeMode(t *testing.T) {
	ctx := context.Background()
	workspaceID := "ws-123"
	userID := "user-456"
	mjmlSrc := "<mjml><mj-body><mj-section><mj-column><mj-text>Hello</mj-text></mj-column></mj-section></mj-body></mjml>"

	t.Run("Success", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		templateService, mockRepo, _, mockAuthService, _ := setupTemplateServiceTest(ctrl)

		template := &domain.Template{
			ID:       "tmpl-code",
			Name:     "Code Template",
			Channel:  "email",
			Category: "transactional",
			Email: &domain.EmailTemplate{
				EditorMode: domain.EditorModeCode,
				MjmlSource: &mjmlSrc,
				Subject:    "Test Email",
			},
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceTemplates: {Read: true, Write: true},
			},
		}, nil)
		mockRepo.EXPECT().CreateTemplate(ctx, workspaceID, gomock.Any()).Return(nil)

		err := templateService.CreateTemplate(ctx, workspaceID, template)

		assert.NoError(t, err)
		assert.Equal(t, int64(1), template.Version)
		assert.NotEmpty(t, template.Email.CompiledPreview)
	})
}

func TestTemplateService_UpdateTemplate_CodeMode(t *testing.T) {
	ctx := context.Background()
	workspaceID := "ws-123"
	userID := "user-456"
	templateID := "tmpl-code"
	mjmlSrc := "<mjml><mj-body><mj-section><mj-column><mj-text>Hello</mj-text></mj-column></mj-section></mj-body></mjml>"
	existingCreatedAt := time.Now().Add(-1 * time.Hour).UTC()

	existingCodeTemplate := &domain.Template{
		ID:        templateID,
		Name:      "Code Template",
		Version:   1,
		Channel:   "email",
		Category:  "transactional",
		CreatedAt: existingCreatedAt,
		Email: &domain.EmailTemplate{
			EditorMode:      domain.EditorModeCode,
			MjmlSource:      &mjmlSrc,
			Subject:         "Old Subject",
			CompiledPreview: mjmlSrc,
		},
	}

	t.Run("Success - update code mode template", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		templateService, mockRepo, _, mockAuthService, _ := setupTemplateServiceTest(ctrl)

		newMjml := "<mjml><mj-body><mj-section><mj-column><mj-text>Updated</mj-text></mj-column></mj-section></mj-body></mjml>"
		templateToUpdate := &domain.Template{
			ID:       templateID,
			Name:     "Code Template",
			Channel:  "email",
			Category: "transactional",
			Email: &domain.EmailTemplate{
				EditorMode: domain.EditorModeCode,
				MjmlSource: &newMjml,
				Subject:    "New Subject",
			},
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceTemplates: {Read: true, Write: true},
			},
		}, nil)
		mockRepo.EXPECT().GetTemplateByID(ctx, workspaceID, templateID, int64(0)).Return(existingCodeTemplate, nil)
		mockRepo.EXPECT().UpdateTemplate(ctx, workspaceID, gomock.Any()).Return(nil)

		err := templateService.UpdateTemplate(ctx, workspaceID, templateToUpdate)
		assert.NoError(t, err)
	})

	t.Run("Fails - switch from visual to code", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		templateService, mockRepo, _, mockAuthService, _ := setupTemplateServiceTest(ctrl)

		existingVisualTemplate := &domain.Template{
			ID:        templateID,
			Name:      "Visual Template",
			Version:   1,
			Channel:   "email",
			Category:  "transactional",
			CreatedAt: existingCreatedAt,
			Email: &domain.EmailTemplate{
				EditorMode:      "", // visual (default)
				Subject:         "Subject",
				CompiledPreview: "<html>Test</html>",
				VisualEditorTree: func() notifuse_mjml.EmailBlock {
					bodyBase := notifuse_mjml.NewBaseBlock("body", notifuse_mjml.MJMLComponentMjBody)
					bodyBlock := &notifuse_mjml.MJBodyBlock{BaseBlock: bodyBase}
					rootBase := notifuse_mjml.NewBaseBlock("root", notifuse_mjml.MJMLComponentMjml)
					rootBase.Children = []notifuse_mjml.EmailBlock{bodyBlock}
					return &notifuse_mjml.MJMLBlock{BaseBlock: rootBase}
				}(),
			},
		}

		templateToUpdate := &domain.Template{
			ID:       templateID,
			Name:     "Visual Template",
			Channel:  "email",
			Category: "transactional",
			Email: &domain.EmailTemplate{
				EditorMode: domain.EditorModeCode, // trying to switch
				MjmlSource: &mjmlSrc,
				Subject:    "Subject",
			},
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceTemplates: {Read: true, Write: true},
			},
		}, nil)
		mockRepo.EXPECT().GetTemplateByID(ctx, workspaceID, templateID, int64(0)).Return(existingVisualTemplate, nil)

		err := templateService.UpdateTemplate(ctx, workspaceID, templateToUpdate)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot change editor mode")
	})

	t.Run("Fails - switch from code to visual", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		templateService, mockRepo, _, mockAuthService, _ := setupTemplateServiceTest(ctrl)

		templateToUpdate := &domain.Template{
			ID:       templateID,
			Name:     "Code Template",
			Channel:  "email",
			Category: "transactional",
			Email: &domain.EmailTemplate{
				EditorMode:      domain.EditorModeVisual, // trying to switch
				Subject:         "Subject",
				CompiledPreview: "<html>Test</html>",
				VisualEditorTree: func() notifuse_mjml.EmailBlock {
					bodyBase := notifuse_mjml.NewBaseBlock("body", notifuse_mjml.MJMLComponentMjBody)
					bodyBlock := &notifuse_mjml.MJBodyBlock{BaseBlock: bodyBase}
					rootBase := notifuse_mjml.NewBaseBlock("root", notifuse_mjml.MJMLComponentMjml)
					rootBase.Children = []notifuse_mjml.EmailBlock{bodyBlock}
					return &notifuse_mjml.MJMLBlock{BaseBlock: rootBase}
				}(),
			},
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceTemplates: {Read: true, Write: true},
			},
		}, nil)
		mockRepo.EXPECT().GetTemplateByID(ctx, workspaceID, templateID, int64(0)).Return(existingCodeTemplate, nil)

		err := templateService.UpdateTemplate(ctx, workspaceID, templateToUpdate)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot change editor mode")
	})
}

func TestCompileTemplate_WithMjmlSource(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAuthService := domainmocks.NewMockAuthService(ctrl)
	mockRepo := domainmocks.NewMockTemplateRepository(ctrl)
	mockLogger := &MockLogger{}
	mockWorkspaceRepo := domainmocks.NewMockWorkspaceRepository(ctrl)

	svc := service.NewTemplateService(mockRepo, mockWorkspaceRepo, mockAuthService, mockLogger, "https://api.example.com")

	ctx := context.Background()
	workspaceID := "ws-123"
	userID := "user-456"

	mjmlSrc := "<mjml><mj-body><mj-section><mj-column><mj-text>Hello World</mj-text></mj-column></mj-section></mj-body></mjml>"

	mockAuthService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
		UserID:      userID,
		WorkspaceID: workspaceID,
		Role:        "member",
		Permissions: domain.UserPermissions{
			domain.PermissionResourceTemplates: {Read: true, Write: true},
		},
	}, nil)

	mockWorkspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(&domain.Workspace{
		ID:       workspaceID,
		Settings: domain.WorkspaceSettings{WebsiteURL: "https://example.com"},
	}, nil)

	resp, err := svc.CompileTemplate(ctx, domain.CompileTemplateRequest{
		WorkspaceID: workspaceID,
		MessageID:   "msg-123",
		MjmlSource:  &mjmlSrc,
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.True(t, resp.Success)
	require.NotNil(t, resp.HTML)
	assert.Contains(t, *resp.HTML, "Hello World")
}

// TestCompileTemplate_InjectsWorkspaceData verifies the preview exposes
// workspace.base_url / workspace.website_url (resolving the custom endpoint and
// trimming trailing slashes), renders them via Liquid, and echoes the effective
// data back so the console can display it.
func TestCompileTemplate_InjectsWorkspaceData(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAuthService := domainmocks.NewMockAuthService(ctrl)
	mockRepo := domainmocks.NewMockTemplateRepository(ctrl)
	mockLogger := &MockLogger{}
	mockWorkspaceRepo := domainmocks.NewMockWorkspaceRepository(ctrl)
	svc := service.NewTemplateService(mockRepo, mockWorkspaceRepo, mockAuthService, mockLogger, "https://api.example.com")

	ctx := context.Background()
	workspaceID := "ws_123"
	userID := "user_abc"
	testTree := createValidTestTree(createTestTextBlock("txt1", "Visit {{ workspace.website_url }}/verify"))
	customEndpoint := "https://track.example.com/"

	mockAuthService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
		UserID:      userID,
		WorkspaceID: workspaceID,
		Role:        "member",
		Permissions: domain.UserPermissions{
			domain.PermissionResourceTemplates: {Read: true, Write: true},
		},
	}, nil)
	mockWorkspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(&domain.Workspace{
		ID: workspaceID,
		Settings: domain.WorkspaceSettings{
			WebsiteURL:        "https://app.example.com/",
			CustomEndpointURL: &customEndpoint,
		},
	}, nil)

	resp, err := svc.CompileTemplate(ctx, domain.CompileTemplateRequest{
		WorkspaceID:      workspaceID,
		VisualEditorTree: testTree,
		TemplateData:     notifuse_mjml.MapOfAny{},
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.True(t, resp.Success)

	// Effective data is echoed back with the injected workspace object, trailing slashes trimmed.
	require.NotNil(t, resp.TemplateData)
	ws, ok := resp.TemplateData["workspace"].(notifuse_mjml.MapOfAny)
	require.True(t, ok, "workspace object should be injected into the effective template data")
	assert.Equal(t, "https://track.example.com", ws["base_url"], "base_url should resolve the custom endpoint with trailing slash trimmed")
	assert.Equal(t, "https://app.example.com", ws["website_url"], "website_url should be trimmed")

	// And it is actually rendered in the HTML via Liquid.
	require.NotNil(t, resp.HTML)
	assert.Contains(t, *resp.HTML, "https://app.example.com/verify")
}

// TestCompileTemplate_WorkspaceBaseURLFallsBackToAPIEndpoint verifies base_url falls
// back to the configured API endpoint when no custom endpoint is set — matching send time.
func TestCompileTemplate_WorkspaceBaseURLFallsBackToAPIEndpoint(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAuthService := domainmocks.NewMockAuthService(ctrl)
	mockRepo := domainmocks.NewMockTemplateRepository(ctrl)
	mockLogger := &MockLogger{}
	mockWorkspaceRepo := domainmocks.NewMockWorkspaceRepository(ctrl)
	svc := service.NewTemplateService(mockRepo, mockWorkspaceRepo, mockAuthService, mockLogger, "https://api.example.com")

	ctx := context.Background()
	workspaceID := "ws_123"
	userID := "user_abc"
	testTree := createValidTestTree(createTestTextBlock("txt1", "Hello"))

	mockAuthService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
		UserID:      userID,
		WorkspaceID: workspaceID,
		Role:        "member",
		Permissions: domain.UserPermissions{
			domain.PermissionResourceTemplates: {Read: true, Write: true},
		},
	}, nil)
	// No custom endpoint configured.
	mockWorkspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(&domain.Workspace{
		ID:       workspaceID,
		Settings: domain.WorkspaceSettings{WebsiteURL: "https://app.example.com"},
	}, nil)

	resp, err := svc.CompileTemplate(ctx, domain.CompileTemplateRequest{
		WorkspaceID:      workspaceID,
		VisualEditorTree: testTree,
		TemplateData:     notifuse_mjml.MapOfAny{},
		// A non-empty request endpoint distinct from the API endpoint proves the
		// fallback resolves to s.apiEndpoint rather than echoing the request value.
		TrackingSettings: notifuse_mjml.TrackingSettings{Endpoint: "https://other.example.com"},
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, resp.TemplateData)
	ws, ok := resp.TemplateData["workspace"].(notifuse_mjml.MapOfAny)
	require.True(t, ok)
	assert.Equal(t, "https://api.example.com", ws["base_url"], "base_url should fall back to the API endpoint, not the request endpoint")
}

// TestCompileTemplate_PreservesProvidedWorkspaceData verifies a complete caller-provided
// workspace object (e.g. a historical message preview carrying the send-time snapshot,
// decoded from JSON as a map[string]any) is preserved rather than overwritten — so the
// workspace is NOT reloaded.
func TestCompileTemplate_PreservesProvidedWorkspaceData(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAuthService := domainmocks.NewMockAuthService(ctrl)
	mockRepo := domainmocks.NewMockTemplateRepository(ctrl)
	mockLogger := &MockLogger{}
	mockWorkspaceRepo := domainmocks.NewMockWorkspaceRepository(ctrl)
	svc := service.NewTemplateService(mockRepo, mockWorkspaceRepo, mockAuthService, mockLogger, "https://api.example.com")

	ctx := context.Background()
	workspaceID := "ws_123"
	userID := "user_abc"
	testTree := createValidTestTree(createTestTextBlock("txt1", "Visit {{ workspace.website_url }}/verify"))

	mockAuthService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
		UserID:      userID,
		WorkspaceID: workspaceID,
		Role:        "member",
		Permissions: domain.UserPermissions{
			domain.PermissionResourceTemplates: {Read: true, Write: true},
		},
	}, nil)
	// GetByID must NOT be called when the caller already provides a complete workspace object.

	resp, err := svc.CompileTemplate(ctx, domain.CompileTemplateRequest{
		WorkspaceID:      workspaceID,
		VisualEditorTree: testTree,
		TemplateData: notifuse_mjml.MapOfAny{
			// map[string]any mirrors how json.Unmarshal decodes the request body.
			"workspace": map[string]any{
				"base_url":    "https://snapshot.example.com",
				"website_url": "https://old-app.example.com",
			},
		},
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.True(t, resp.Success)

	ws, ok := resp.TemplateData["workspace"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "https://old-app.example.com", ws["website_url"], "provided workspace data must be preserved")
	assert.Equal(t, "https://snapshot.example.com", ws["base_url"], "provided workspace data must be preserved")
	require.NotNil(t, resp.HTML)
	assert.Contains(t, *resp.HTML, "https://old-app.example.com/verify")
}

// TestCompileTemplate_FillsMissingWorkspaceKeys verifies that a PARTIAL workspace object
// (older templates whose saved test_data carries only base_url) gets the missing
// website_url filled in from the workspace, while the provided base_url is preserved.
func TestCompileTemplate_FillsMissingWorkspaceKeys(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAuthService := domainmocks.NewMockAuthService(ctrl)
	mockRepo := domainmocks.NewMockTemplateRepository(ctrl)
	mockLogger := &MockLogger{}
	mockWorkspaceRepo := domainmocks.NewMockWorkspaceRepository(ctrl)
	svc := service.NewTemplateService(mockRepo, mockWorkspaceRepo, mockAuthService, mockLogger, "https://api.example.com")

	ctx := context.Background()
	workspaceID := "ws_123"
	userID := "user_abc"
	testTree := createValidTestTree(createTestTextBlock("txt1", "Visit {{ workspace.website_url }}/verify"))

	mockAuthService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
		UserID:      userID,
		WorkspaceID: workspaceID,
		Role:        "member",
		Permissions: domain.UserPermissions{
			domain.PermissionResourceTemplates: {Read: true, Write: true},
		},
	}, nil)
	// The missing website_url is sourced from the workspace.
	mockWorkspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(&domain.Workspace{
		ID:       workspaceID,
		Settings: domain.WorkspaceSettings{WebsiteURL: "https://app.example.com/"},
	}, nil)

	resp, err := svc.CompileTemplate(ctx, domain.CompileTemplateRequest{
		WorkspaceID:      workspaceID,
		VisualEditorTree: testTree,
		TemplateData: notifuse_mjml.MapOfAny{
			// Only base_url provided (the pre-fix CreateTemplateDrawer shape); website_url is absent.
			"workspace": map[string]any{
				"base_url": "https://snapshot.example.com",
			},
		},
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.True(t, resp.Success)

	ws, ok := resp.TemplateData["workspace"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "https://snapshot.example.com", ws["base_url"], "provided base_url must be preserved")
	assert.Equal(t, "https://app.example.com", ws["website_url"], "missing website_url should be filled (trailing slash trimmed)")
	require.NotNil(t, resp.HTML)
	assert.Contains(t, *resp.HTML, "https://app.example.com/verify")
}

// TestCompileTemplate_WorkspaceLoadFailureUsesFallback verifies the preview still renders
// (with the request endpoint as base_url and an empty website_url) when the workspace
// lookup fails, rather than failing the whole compile.
func TestCompileTemplate_WorkspaceLoadFailureUsesFallback(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAuthService := domainmocks.NewMockAuthService(ctrl)
	mockRepo := domainmocks.NewMockTemplateRepository(ctrl)
	mockLogger := &MockLogger{}
	mockWorkspaceRepo := domainmocks.NewMockWorkspaceRepository(ctrl)
	svc := service.NewTemplateService(mockRepo, mockWorkspaceRepo, mockAuthService, mockLogger, "https://api.example.com")

	ctx := context.Background()
	workspaceID := "ws_123"
	userID := "user_abc"
	testTree := createValidTestTree(createTestTextBlock("txt1", "Hello"))

	mockAuthService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
		UserID:      userID,
		WorkspaceID: workspaceID,
		Role:        "member",
		Permissions: domain.UserPermissions{
			domain.PermissionResourceTemplates: {Read: true, Write: true},
		},
	}, nil)
	mockWorkspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(nil, errors.New("workspace unavailable"))

	resp, err := svc.CompileTemplate(ctx, domain.CompileTemplateRequest{
		WorkspaceID:      workspaceID,
		VisualEditorTree: testTree,
		TemplateData:     notifuse_mjml.MapOfAny{},
		TrackingSettings: notifuse_mjml.TrackingSettings{Endpoint: "https://fallback.example.com"},
	})

	require.NoError(t, err, "preview should not fail when the workspace lookup fails")
	require.NotNil(t, resp)
	require.NotNil(t, resp.TemplateData)
	ws, ok := resp.TemplateData["workspace"].(notifuse_mjml.MapOfAny)
	require.True(t, ok)
	assert.Equal(t, "https://fallback.example.com", ws["base_url"], "base_url falls back to the request endpoint")
	assert.Equal(t, "", ws["website_url"], "website_url is empty when the workspace can't be loaded")
}

// TestCompileTemplate_FillsMissingWorkspaceKeys_MapOfAny verifies the missing-key fill
// also works when the provided workspace object is a notifuse_mjml.MapOfAny (built in Go)
// rather than a map[string]any (decoded from JSON) — both underlying types are accepted.
func TestCompileTemplate_FillsMissingWorkspaceKeys_MapOfAny(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAuthService := domainmocks.NewMockAuthService(ctrl)
	mockRepo := domainmocks.NewMockTemplateRepository(ctrl)
	mockLogger := &MockLogger{}
	mockWorkspaceRepo := domainmocks.NewMockWorkspaceRepository(ctrl)
	svc := service.NewTemplateService(mockRepo, mockWorkspaceRepo, mockAuthService, mockLogger, "https://api.example.com")

	ctx := context.Background()
	workspaceID := "ws_123"
	userID := "user_abc"
	testTree := createValidTestTree(createTestTextBlock("txt1", "Visit {{ workspace.website_url }}/verify"))

	mockAuthService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
		UserID:      userID,
		WorkspaceID: workspaceID,
		Role:        "member",
		Permissions: domain.UserPermissions{
			domain.PermissionResourceTemplates: {Read: true, Write: true},
		},
	}, nil)
	mockWorkspaceRepo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(&domain.Workspace{
		ID:       workspaceID,
		Settings: domain.WorkspaceSettings{WebsiteURL: "https://app.example.com"},
	}, nil)

	resp, err := svc.CompileTemplate(ctx, domain.CompileTemplateRequest{
		WorkspaceID:      workspaceID,
		VisualEditorTree: testTree,
		TemplateData: notifuse_mjml.MapOfAny{
			// Partial workspace as a MapOfAny (not map[string]any) — missing website_url.
			"workspace": notifuse_mjml.MapOfAny{
				"base_url": "https://snapshot.example.com",
			},
		},
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.True(t, resp.Success)

	ws, ok := resp.TemplateData["workspace"].(notifuse_mjml.MapOfAny)
	require.True(t, ok)
	assert.Equal(t, "https://snapshot.example.com", ws["base_url"], "provided base_url must be preserved")
	assert.Equal(t, "https://app.example.com", ws["website_url"], "missing website_url should be filled even for a MapOfAny")
	require.NotNil(t, resp.HTML)
	assert.Contains(t, *resp.HTML, "https://app.example.com/verify")
}

func TestTemplateService_UpdateEmailMetadataBlocks_CodeMode(t *testing.T) {
	ctx := context.Background()
	workspaceID := "ws-123"
	userID := "user-456"

	starterMjml := `<mjml>
  <mj-head>
    <mj-attributes>
      <mj-all font-family="Arial, sans-serif" />
    </mj-attributes>
  </mj-head>
  <mj-body>
    <mj-section>
      <mj-column>
        <mj-text>Hello</mj-text>
      </mj-column>
    </mj-section>
  </mj-body>
</mjml>`

	subjectPreview := "Check this out"

	t.Run("CreateTemplate injects mj-title and mj-preview in code mode", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		svc, mockRepo, _, mockAuthService, _ := setupTemplateServiceTest(ctrl)

		tmpl := &domain.Template{
			ID:       "tmpl-code-1",
			Name:     "My Template",
			Channel:  "email",
			Category: "transactional",
			Email: &domain.EmailTemplate{
				EditorMode:     domain.EditorModeCode,
				MjmlSource:     &starterMjml,
				Subject:        "Test Subject",
				SubjectPreview: &subjectPreview,
			},
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceTemplates: {Read: true, Write: true},
			},
		}, nil)

		mockRepo.EXPECT().CreateTemplate(ctx, workspaceID, gomock.Any()).DoAndReturn(
			func(_ context.Context, _ string, tmplArg *domain.Template) error {
				// Verify that the MJML source was modified with mj-title and mj-preview
				require.NotNil(t, tmplArg.Email)
				require.NotNil(t, tmplArg.Email.MjmlSource)
				assert.Contains(t, *tmplArg.Email.MjmlSource, "<mj-title>My Template</mj-title>")
				assert.Contains(t, *tmplArg.Email.MjmlSource, "<mj-preview>Check this out</mj-preview>")
				return nil
			},
		)

		err := svc.CreateTemplate(ctx, workspaceID, tmpl)
		require.NoError(t, err)
	})

	t.Run("UpdateTemplate overrides existing mj-title and mj-preview in code mode", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		svc, mockRepo, _, mockAuthService, _ := setupTemplateServiceTest(ctrl)

		mjmlWithTags := `<mjml>
  <mj-head>
    <mj-title>Old Title</mj-title>
    <mj-preview>Old Preview</mj-preview>
  </mj-head>
  <mj-body>
    <mj-section><mj-column><mj-text>Hello</mj-text></mj-column></mj-section>
  </mj-body>
</mjml>`

		newPreview := "Updated Preview"
		tmpl := &domain.Template{
			ID:       "tmpl-code-2",
			Name:     "Updated Template",
			Channel:  "email",
			Category: "transactional",
			Email: &domain.EmailTemplate{
				EditorMode:     domain.EditorModeCode,
				MjmlSource:     &mjmlWithTags,
				Subject:        "Test Subject",
				SubjectPreview: &newPreview,
			},
		}

		existingMjml := mjmlWithTags
		existingTemplate := &domain.Template{
			ID:       "tmpl-code-2",
			Name:     "Old Template",
			Version:  1,
			Channel:  "email",
			Category: "transactional",
			Email: &domain.EmailTemplate{
				EditorMode: domain.EditorModeCode,
				MjmlSource: &existingMjml,
				Subject:    "Test Subject",
			},
			CreatedAt: time.Now().Add(-time.Hour),
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceTemplates: {Read: true, Write: true},
			},
		}, nil)

		mockRepo.EXPECT().GetTemplateByID(ctx, workspaceID, "tmpl-code-2", int64(0)).Return(existingTemplate, nil)

		mockRepo.EXPECT().UpdateTemplate(ctx, workspaceID, gomock.Any()).DoAndReturn(
			func(_ context.Context, _ string, tmplArg *domain.Template) error {
				require.NotNil(t, tmplArg.Email)
				require.NotNil(t, tmplArg.Email.MjmlSource)
				assert.Contains(t, *tmplArg.Email.MjmlSource, "<mj-title>Updated Template</mj-title>")
				assert.Contains(t, *tmplArg.Email.MjmlSource, "<mj-preview>Updated Preview</mj-preview>")
				assert.NotContains(t, *tmplArg.Email.MjmlSource, "Old Title")
				assert.NotContains(t, *tmplArg.Email.MjmlSource, "Old Preview")
				return nil
			},
		)

		err := svc.UpdateTemplate(ctx, workspaceID, tmpl)
		require.NoError(t, err)
	})

	t.Run("Code mode with no SubjectPreview uses template Name for mj-preview", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		svc, mockRepo, _, mockAuthService, _ := setupTemplateServiceTest(ctrl)

		tmpl := &domain.Template{
			ID:       "tmpl-code-3",
			Name:     "My Fallback Name",
			Channel:  "email",
			Category: "transactional",
			Email: &domain.EmailTemplate{
				EditorMode: domain.EditorModeCode,
				MjmlSource: &starterMjml,
				Subject:    "Test Subject",
			},
		}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{
			UserID:      userID,
			WorkspaceID: workspaceID,
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceTemplates: {Read: true, Write: true},
			},
		}, nil)

		mockRepo.EXPECT().CreateTemplate(ctx, workspaceID, gomock.Any()).DoAndReturn(
			func(_ context.Context, _ string, tmplArg *domain.Template) error {
				require.NotNil(t, tmplArg.Email)
				require.NotNil(t, tmplArg.Email.MjmlSource)
				assert.Contains(t, *tmplArg.Email.MjmlSource, "<mj-title>My Fallback Name</mj-title>")
				assert.Contains(t, *tmplArg.Email.MjmlSource, "<mj-preview>My Fallback Name</mj-preview>")
				return nil
			},
		)

		err := svc.CreateTemplate(ctx, workspaceID, tmpl)
		require.NoError(t, err)
	})
}
