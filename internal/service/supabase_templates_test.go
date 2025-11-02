package service

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
)

func TestCreateDefaultSupabaseTemplates_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTemplateRepo := mocks.NewMockTemplateRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	service := NewSupabaseService(
		nil, nil, nil, nil, nil,
		mockTemplateRepo,
		nil, nil, nil,
		mockLogger,
	)

	// Mock successful creation of all templates
	mockTemplateRepo.EXPECT().CreateTemplate(gomock.Any(), "workspace-123", gomock.Any()).
		DoAndReturn(func(ctx context.Context, workspaceID string, template *domain.Template) error {
			// Verify template has expected properties
			assert.Equal(t, "email", template.Channel)
			assert.Equal(t, "transactional", template.Category)
			assert.NotNil(t, template.IntegrationID)
			assert.Equal(t, "integration-456", *template.IntegrationID)
			return nil
		}).Times(6) // 6 templates total: signup, magiclink, recovery, email_change, invite, reauthentication

	mappings, err := service.CreateDefaultSupabaseTemplates(context.Background(), "workspace-123", "integration-456")

	require.NoError(t, err)
	require.NotNil(t, mappings)

	// Verify all mappings are populated
	assert.NotEmpty(t, mappings.Signup)
	assert.NotEmpty(t, mappings.MagicLink)
	assert.NotEmpty(t, mappings.Recovery)
	assert.NotEmpty(t, mappings.EmailChange)
	assert.NotEmpty(t, mappings.Invite)
	assert.NotEmpty(t, mappings.Reauthentication)

	// Verify all templates have the correct prefix
	assert.Contains(t, mappings.Signup, "supabase_signup_")
	assert.Contains(t, mappings.MagicLink, "supabase_magiclink_")
	assert.Contains(t, mappings.Recovery, "supabase_recovery_")
	assert.Contains(t, mappings.EmailChange, "supabase_email_change_")
	assert.Contains(t, mappings.Invite, "supabase_invite_")
	assert.Contains(t, mappings.Reauthentication, "supabase_reauth_")
}

func TestCreateDefaultSupabaseTemplates_FailureOnSignup(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTemplateRepo := mocks.NewMockTemplateRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	service := NewSupabaseService(
		nil, nil, nil, nil, nil,
		mockTemplateRepo,
		nil, nil, nil,
		mockLogger,
	)

	// Mock failure on first template (signup)
	mockTemplateRepo.EXPECT().CreateTemplate(gomock.Any(), "workspace-123", gomock.Any()).
		Return(assert.AnError)

	mappings, err := service.CreateDefaultSupabaseTemplates(context.Background(), "workspace-123", "integration-456")

	assert.Error(t, err)
	assert.Nil(t, mappings)
	assert.Contains(t, err.Error(), "failed to create signup template")
}

func TestCreateSignupTemplate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTemplateRepo := mocks.NewMockTemplateRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	service := NewSupabaseService(
		nil, nil, nil, nil, nil,
		mockTemplateRepo,
		nil, nil, nil,
		mockLogger,
	)

	mockTemplateRepo.EXPECT().CreateTemplate(gomock.Any(), "workspace-123", gomock.Any()).
		DoAndReturn(func(ctx context.Context, workspaceID string, template *domain.Template) error {
			assert.Equal(t, "Signup Confirmation", template.Name)
			assert.Equal(t, "email", template.Channel)
			assert.Equal(t, "transactional", template.Category)
			assert.NotNil(t, template.IntegrationID)
			assert.Equal(t, "integration-456", *template.IntegrationID)
			assert.NotNil(t, template.Email)
			assert.Equal(t, "Confirm your email address", template.Email.Subject)
			assert.NotNil(t, template.Email.VisualEditorTree)
			return nil
		})

	templateID, err := service.createSignupTemplate(context.Background(), "workspace-123", "integration-456")

	assert.NoError(t, err)
	assert.Contains(t, templateID, "supabase_signup_")
}

func TestCreateMagicLinkTemplate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTemplateRepo := mocks.NewMockTemplateRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	service := NewSupabaseService(
		nil, nil, nil, nil, nil,
		mockTemplateRepo,
		nil, nil, nil,
		mockLogger,
	)

	mockTemplateRepo.EXPECT().CreateTemplate(gomock.Any(), "workspace-123", gomock.Any()).
		DoAndReturn(func(ctx context.Context, workspaceID string, template *domain.Template) error {
			assert.Equal(t, "Magic Link", template.Name)
			assert.NotNil(t, template.Email)
			assert.Equal(t, "Your sign-in link", template.Email.Subject)
			return nil
		})

	templateID, err := service.createMagicLinkTemplate(context.Background(), "workspace-123", "integration-456")

	assert.NoError(t, err)
	assert.Contains(t, templateID, "supabase_magiclink_")
}

func TestCreateRecoveryTemplate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTemplateRepo := mocks.NewMockTemplateRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	service := NewSupabaseService(
		nil, nil, nil, nil, nil,
		mockTemplateRepo,
		nil, nil, nil,
		mockLogger,
	)

	mockTemplateRepo.EXPECT().CreateTemplate(gomock.Any(), "workspace-123", gomock.Any()).
		DoAndReturn(func(ctx context.Context, workspaceID string, template *domain.Template) error {
			assert.Equal(t, "Password Recovery", template.Name)
			assert.NotNil(t, template.Email)
			assert.Equal(t, "Reset your password", template.Email.Subject)
			return nil
		})

	templateID, err := service.createRecoveryTemplate(context.Background(), "workspace-123", "integration-456")

	assert.NoError(t, err)
	assert.Contains(t, templateID, "supabase_recovery_")
}

func TestCreateEmailChangeTemplate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTemplateRepo := mocks.NewMockTemplateRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	service := NewSupabaseService(
		nil, nil, nil, nil, nil,
		mockTemplateRepo,
		nil, nil, nil,
		mockLogger,
	)

	mockTemplateRepo.EXPECT().CreateTemplate(gomock.Any(), "workspace-123", gomock.Any()).
		DoAndReturn(func(ctx context.Context, workspaceID string, template *domain.Template) error {
			assert.Equal(t, "Email Change", template.Name)
			assert.NotNil(t, template.Email)
			assert.Equal(t, "Confirm email change", template.Email.Subject)
			return nil
		})

	templateID, err := service.createEmailChangeTemplate(context.Background(), "workspace-123", "integration-456")

	assert.NoError(t, err)
	assert.Contains(t, templateID, "supabase_email_change_")
}

func TestCreateInviteTemplate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTemplateRepo := mocks.NewMockTemplateRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	service := NewSupabaseService(
		nil, nil, nil, nil, nil,
		mockTemplateRepo,
		nil, nil, nil,
		mockLogger,
	)

	mockTemplateRepo.EXPECT().CreateTemplate(gomock.Any(), "workspace-123", gomock.Any()).
		DoAndReturn(func(ctx context.Context, workspaceID string, template *domain.Template) error {
			assert.Equal(t, "User Invitation", template.Name)
			assert.NotNil(t, template.Email)
			assert.Equal(t, "You've been invited", template.Email.Subject)
			return nil
		})

	templateID, err := service.createInviteTemplate(context.Background(), "workspace-123", "integration-456")

	assert.NoError(t, err)
	assert.Contains(t, templateID, "supabase_invite_")
}

func TestCreateReauthenticationTemplate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTemplateRepo := mocks.NewMockTemplateRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	service := NewSupabaseService(
		nil, nil, nil, nil, nil,
		mockTemplateRepo,
		nil, nil, nil,
		mockLogger,
	)

	mockTemplateRepo.EXPECT().CreateTemplate(gomock.Any(), "workspace-123", gomock.Any()).
		DoAndReturn(func(ctx context.Context, workspaceID string, template *domain.Template) error {
			assert.Equal(t, "Reauthentication", template.Name)
			assert.NotNil(t, template.Email)
			assert.Equal(t, "Verify your identity", template.Email.Subject)
			return nil
		})

	templateID, err := service.createReauthenticationTemplate(context.Background(), "workspace-123", "integration-456")

	assert.NoError(t, err)
	assert.Contains(t, templateID, "supabase_reauth_")
}

func TestStrPtr(t *testing.T) {
	str := "test"
	ptr := strPtr(str)

	assert.NotNil(t, ptr)
	assert.Equal(t, str, *ptr)
}

func TestTemplateIDUniqueness(t *testing.T) {
	// Test that generated template IDs are unique
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTemplateRepo := mocks.NewMockTemplateRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	service := NewSupabaseService(
		nil, nil, nil, nil, nil,
		mockTemplateRepo,
		nil, nil, nil,
		mockLogger,
	)

	// Create multiple templates and collect their IDs
	templateIDs := make(map[string]bool)

	mockTemplateRepo.EXPECT().CreateTemplate(gomock.Any(), "workspace-123", gomock.Any()).Return(nil).Times(3)

	id1, _ := service.createSignupTemplate(context.Background(), "workspace-123", "integration-456")
	id2, _ := service.createMagicLinkTemplate(context.Background(), "workspace-123", "integration-456")
	id3, _ := service.createRecoveryTemplate(context.Background(), "workspace-123", "integration-456")

	templateIDs[id1] = true
	templateIDs[id2] = true
	templateIDs[id3] = true

	// All IDs should be unique
	assert.Len(t, templateIDs, 3)
}
