package service

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/templates"
)

// CreateDefaultSupabaseTemplates creates the default Supabase auth email templates
func (s *SupabaseService) CreateDefaultSupabaseTemplates(ctx context.Context, workspaceID, integrationID string) (*domain.SupabaseTemplateMappings, error) {
	// Use system context to bypass authentication
	systemCtx := context.WithValue(ctx, domain.SystemCallKey, true)

	mappings := &domain.SupabaseTemplateMappings{}

	// Create signup template
	signupID, err := s.createSignupTemplate(systemCtx, workspaceID, integrationID)
	if err != nil {
		return nil, fmt.Errorf("failed to create signup template: %w", err)
	}
	mappings.Signup = signupID

	// Create magic link template
	magicLinkID, err := s.createMagicLinkTemplate(systemCtx, workspaceID, integrationID)
	if err != nil {
		return nil, fmt.Errorf("failed to create magic link template: %w", err)
	}
	mappings.MagicLink = magicLinkID

	// Create recovery template
	recoveryID, err := s.createRecoveryTemplate(systemCtx, workspaceID, integrationID)
	if err != nil {
		return nil, fmt.Errorf("failed to create recovery template: %w", err)
	}
	mappings.Recovery = recoveryID

	// Create email change template (single template for both current and new email addresses)
	emailChangeID, err := s.createEmailChangeTemplate(systemCtx, workspaceID, integrationID)
	if err != nil {
		return nil, fmt.Errorf("failed to create email change template: %w", err)
	}
	mappings.EmailChange = emailChangeID

	// Create invite template
	inviteID, err := s.createInviteTemplate(systemCtx, workspaceID, integrationID)
	if err != nil {
		return nil, fmt.Errorf("failed to create invite template: %w", err)
	}
	mappings.Invite = inviteID

	// Create reauthentication template
	reauthID, err := s.createReauthenticationTemplate(systemCtx, workspaceID, integrationID)
	if err != nil {
		return nil, fmt.Errorf("failed to create reauthentication template: %w", err)
	}
	mappings.Reauthentication = reauthID

	return mappings, nil
}

func strPtr(s string) *string {
	return &s
}

// createSupabaseTestData creates test data for Supabase templates
func createSupabaseTestData(emailActionType string) domain.MapOfAny {
	return domain.MapOfAny{
		"user": domain.MapOfAny{
			"id":    "8484b834-f29e-4af2-bf42-80644d154f76",
			"aud":   "authenticated",
			"role":  "authenticated",
			"email": "valid.email@supabase.io",
			"phone": "",
			"app_metadata": domain.MapOfAny{
				"provider":  "email",
				"providers": []string{"email"},
			},
			"user_metadata": domain.MapOfAny{
				"email":          "valid.email@supabase.io",
				"email_verified": false,
				"phone_verified": false,
				"sub":            "8484b834-f29e-4af2-bf42-80644d154f76",
			},
			"identities": []domain.MapOfAny{
				{
					"identity_id": "bc26d70b-517d-4826-bce4-413a5ff257e7",
					"id":          "8484b834-f29e-4af2-bf42-80644d154f76",
					"user_id":     "8484b834-f29e-4af2-bf42-80644d154f76",
					"identity_data": domain.MapOfAny{
						"email":          "valid.email@supabase.io",
						"email_verified": false,
						"phone_verified": false,
						"sub":            "8484b834-f29e-4af2-bf42-80644d154f76",
					},
					"provider":        "email",
					"last_sign_in_at": "2024-05-14T12:56:33.824231484Z",
					"created_at":      "2024-05-14T12:56:33.824261Z",
					"updated_at":      "2024-05-14T12:56:33.824261Z",
					"email":           "valid.email@supabase.io",
				},
			},
			"created_at":   "2024-05-14T12:56:33.821567Z",
			"updated_at":   "2024-05-14T12:56:33.825595Z",
			"is_anonymous": false,
		},
		"email_data": domain.MapOfAny{
			"token":             "305805",
			"token_hash":        "7d5b7b1964cf5d388340a7f04f1dbb5eeb6c7b52ef8270e1737a58d0",
			"redirect_to":       "http://localhost:3000/",
			"email_action_type": emailActionType,
			"site_url":          "http://localhost:9999",
			"token_new":         "",
			"token_hash_new":    "",
		},
	}
}


// createSignupTemplate creates the signup confirmation template
func (s *SupabaseService) createSignupTemplate(ctx context.Context, workspaceID, integrationID string) (string, error) {
	// Use random number to avoid collisions in concurrent tests
	templateID := fmt.Sprintf("supabase_signup_%06d", rand.Intn(1000000))

	visualEditorTree, err := templates.CreateSupabaseSignupEmailStructure()
	if err != nil {
		return "", fmt.Errorf("failed to create signup email structure: %w", err)
	}

	template := &domain.Template{
		ID:            templateID,
		Name:          "Signup Confirmation",
		Version:       1,
		Channel:       "email",
		Category:      "transactional",
		IntegrationID: &integrationID,
		Email: &domain.EmailTemplate{
			Subject:          "Confirm your email address",
			SubjectPreview:   strPtr("Click the button below to verify your email"),
			VisualEditorTree: visualEditorTree,
			SenderID:         "", // Will be set by user
		},
		TestData:  createSupabaseTestData("signup"),
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	if err := s.templateRepo.CreateTemplate(ctx, workspaceID, template); err != nil {
		return "", err
	}

	return templateID, nil
}

// createMagicLinkTemplate creates the magic link authentication template
func (s *SupabaseService) createMagicLinkTemplate(ctx context.Context, workspaceID, integrationID string) (string, error) {
	templateID := fmt.Sprintf("supabase_magiclink_%06d", rand.Intn(1000000))

	visualEditorTree, err := templates.CreateSupabaseMagicLinkEmailStructure()
	if err != nil {
		return "", fmt.Errorf("failed to create magic link email structure: %w", err)
	}

	template := &domain.Template{
		ID:            templateID,
		Name:          "Magic Link",
		Version:       1,
		Channel:       "email",
		Category:      "transactional",
		IntegrationID: &integrationID,
		Email: &domain.EmailTemplate{
			Subject:          "Your sign-in link",
			SubjectPreview:   strPtr("Click the button below to sign in to your account"),
			VisualEditorTree: visualEditorTree,
			SenderID:         "",
		},
		TestData:  createSupabaseTestData("magiclink"),
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	if err := s.templateRepo.CreateTemplate(ctx, workspaceID, template); err != nil {
		return "", err
	}

	return templateID, nil
}

// createRecoveryTemplate creates the password recovery template
func (s *SupabaseService) createRecoveryTemplate(ctx context.Context, workspaceID, integrationID string) (string, error) {
	templateID := fmt.Sprintf("supabase_recovery_%06d", rand.Intn(1000000))

	visualEditorTree, err := templates.CreateSupabaseRecoveryEmailStructure()
	if err != nil {
		return "", fmt.Errorf("failed to create recovery email structure: %w", err)
	}

	template := &domain.Template{
		ID:            templateID,
		Name:          "Password Recovery",
		Version:       1,
		Channel:       "email",
		Category:      "transactional",
		IntegrationID: &integrationID,
		Email: &domain.EmailTemplate{
			Subject:          "Reset your password",
			SubjectPreview:   strPtr("Click the button below to reset your password"),
			VisualEditorTree: visualEditorTree,
			SenderID:         "",
		},
		TestData:  createSupabaseTestData("recovery"),
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	if err := s.templateRepo.CreateTemplate(ctx, workspaceID, template); err != nil {
		return "", err
	}

	return templateID, nil
}

// createEmailChangeTemplate creates the email change confirmation template
// Note: This single template is used for both current and new email addresses
// matching Supabase's behavior where there's only one customizable email_change template
func (s *SupabaseService) createEmailChangeTemplate(ctx context.Context, workspaceID, integrationID string) (string, error) {
	templateID := fmt.Sprintf("supabase_email_change_%06d", rand.Intn(1000000))

	visualEditorTree, err := templates.CreateSupabaseEmailChangeEmailStructure()
	if err != nil {
		return "", fmt.Errorf("failed to create email change email structure: %w", err)
	}

	template := &domain.Template{
		ID:            templateID,
		Name:          "Email Change",
		Version:       1,
		Channel:       "email",
		Category:      "transactional",
		IntegrationID: &integrationID,
		Email: &domain.EmailTemplate{
			Subject:          "Confirm email change",
			SubjectPreview:   strPtr("Click the button below to confirm your email change"),
			VisualEditorTree: visualEditorTree,
			SenderID:         "",
		},
		TestData:  createSupabaseTestData("email_change"),
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	if err := s.templateRepo.CreateTemplate(ctx, workspaceID, template); err != nil {
		return "", err
	}

	return templateID, nil
}

// createInviteTemplate creates the user invitation template
func (s *SupabaseService) createInviteTemplate(ctx context.Context, workspaceID, integrationID string) (string, error) {
	templateID := fmt.Sprintf("supabase_invite_%06d", rand.Intn(1000000))

	visualEditorTree, err := templates.CreateSupabaseInviteEmailStructure()
	if err != nil {
		return "", fmt.Errorf("failed to create invite email structure: %w", err)
	}

	template := &domain.Template{
		ID:            templateID,
		Name:          "User Invitation",
		Version:       1,
		Channel:       "email",
		Category:      "transactional",
		IntegrationID: &integrationID,
		Email: &domain.EmailTemplate{
			Subject:          "You've been invited",
			SubjectPreview:   strPtr("Click the button below to accept your invitation"),
			VisualEditorTree: visualEditorTree,
			SenderID:         "",
		},
		TestData:  createSupabaseTestData("invite"),
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	if err := s.templateRepo.CreateTemplate(ctx, workspaceID, template); err != nil {
		return "", err
	}

	return templateID, nil
}

// createReauthenticationTemplate creates the reauthentication template
func (s *SupabaseService) createReauthenticationTemplate(ctx context.Context, workspaceID, integrationID string) (string, error) {
	templateID := fmt.Sprintf("supabase_reauth_%06d", rand.Intn(1000000))

	visualEditorTree, err := templates.CreateSupabaseReauthenticationEmailStructure()
	if err != nil {
		return "", fmt.Errorf("failed to create reauthentication email structure: %w", err)
	}

	template := &domain.Template{
		ID:            templateID,
		Name:          "Reauthentication",
		Version:       1,
		Channel:       "email",
		Category:      "transactional",
		IntegrationID: &integrationID,
		Email: &domain.EmailTemplate{
			Subject:          "Verify your identity",
			SubjectPreview:   strPtr("Enter the verification code to confirm your identity"),
			VisualEditorTree: visualEditorTree,
			SenderID:         "",
		},
		TestData:  createSupabaseTestData("reauthentication"),
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	if err := s.templateRepo.CreateTemplate(ctx, workspaceID, template); err != nil {
		return "", err
	}

	return templateID, nil
}
