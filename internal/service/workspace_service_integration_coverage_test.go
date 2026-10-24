package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
)

// covWorkspaceDeps bundles the mocks a coverage subtest drives so each
// subtest can set exactly the expectations its branch needs — and, just as
// importantly, leave the downstream mocks expectation-free so gomock fails
// the test if a guard lets a call through.
type covWorkspaceDeps struct {
	repo    *mocks.MockWorkspaceRepository
	auth    *mocks.MockAuthService
	webhook *mocks.MockWebhookRegistrationService
}

func covWorkspaceService(ctrl *gomock.Controller, webhook *mocks.MockWebhookRegistrationService, supabase *SupabaseService, entitlements domain.EntitlementProvider) (*WorkspaceService, covWorkspaceDeps) {
	deps := covWorkspaceDeps{
		repo:    mocks.NewMockWorkspaceRepository(ctrl),
		auth:    mocks.NewMockAuthService(ctrl),
		webhook: webhook,
	}
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

	if webhook == nil {
		webhook = mocks.NewMockWebhookRegistrationService(ctrl)
		deps.webhook = webhook
	}
	if supabase == nil {
		supabase = &SupabaseService{}
	}
	if entitlements == nil {
		entitlements = fullyLicensedProvider(ctrl)
	}

	svc := NewWorkspaceService(
		deps.repo,
		mocks.NewMockUserRepository(ctrl),
		mocks.NewMockTaskRepository(ctrl),
		mockLogger,
		mocks.NewMockUserServiceInterface(ctrl),
		deps.auth,
		pkgmocks.NewMockMailer(ctrl),
		&config.Config{RootEmail: "root@example.com"},
		mocks.NewMockContactService(ctrl),
		mocks.NewMockListService(ctrl),
		mocks.NewMockContactListService(ctrl),
		mocks.NewMockTemplateService(ctrl),
		webhook,
		"secret_key",
		supabase,
		&DNSVerificationService{},
		&BlogService{},
		entitlements,
	)
	return svc, deps
}

func (d covWorkspaceDeps) expectOwner(ctx context.Context, workspaceID, userID string) {
	d.auth.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).
		Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{UserID: userID, WorkspaceID: workspaceID, Role: "owner"}, nil)
}

func (d covWorkspaceDeps) expectMember(ctx context.Context, workspaceID, userID string) {
	d.auth.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).
		Return(ctx, &domain.User{ID: userID}, &domain.UserWorkspace{UserID: userID, WorkspaceID: workspaceID, Role: "member"}, nil)
}

// ---------------------------------------------------------------------------
// isLicensedFor / licensedWorkspaceQuota
// ---------------------------------------------------------------------------

func TestWorkspaceServiceCoverage_IsLicensedFor(t *testing.T) {
	t.Run("nil provider leaves the gate inert for every feature", func(t *testing.T) {
		svc := &WorkspaceService{}
		assert.True(t, svc.isLicensedFor(domain.FeatureRBAC))
		assert.True(t, svc.isLicensedFor(domain.FeatureSESTenant))
		assert.True(t, svc.isLicensedFor(domain.FeatureSSO))
		assert.Equal(t, domain.UnlimitedWorkspaces, svc.licensedWorkspaceQuota())
	})

	t.Run("community entitlements refuse every gated feature", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		svc := &WorkspaceService{entitlements: stubEntitlements(ctrl, domain.CommunityEntitlements())}

		assert.False(t, svc.isLicensedFor(domain.FeatureRBAC))
		assert.False(t, svc.isLicensedFor(domain.FeatureSESTenant))
		assert.False(t, svc.isLicensedFor(domain.FeatureSSO))
		assert.False(t, svc.isLicensedFor(domain.FeatureAuditLogs))
		assert.Equal(t, domain.CommunityMaxWorkspaces, svc.licensedWorkspaceQuota())
	})

	t.Run("licensed entitlements grant only the listed features", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		svc := &WorkspaceService{entitlements: stubEntitlements(ctrl, licensedEntitlements(7, domain.FeatureRBAC))}

		assert.True(t, svc.isLicensedFor(domain.FeatureRBAC))
		assert.False(t, svc.isLicensedFor(domain.FeatureSESTenant))
		assert.False(t, svc.isLicensedFor(domain.FeatureTemplateI18n))
		assert.Equal(t, 7, svc.licensedWorkspaceQuota())
	})

	t.Run("unlimited licensed quota is passed through verbatim", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		svc := &WorkspaceService{entitlements: stubEntitlements(ctrl, licensedEntitlements(domain.UnlimitedWorkspaces, domain.FeatureRBAC))}

		assert.Equal(t, -1, svc.licensedWorkspaceQuota())
		assert.Equal(t, domain.UnlimitedWorkspaces, svc.licensedWorkspaceQuota())
	})
}

// ---------------------------------------------------------------------------
// auditGrantedResources / auditPermissionScope
// ---------------------------------------------------------------------------

func TestWorkspaceServiceCoverage_AuditGrantedResources(t *testing.T) {
	t.Run("nil permissions grant nothing", func(t *testing.T) {
		got := auditGrantedResources(nil)
		assert.NotNil(t, got)
		assert.Empty(t, got)
		assert.Equal(t, "restricted", auditPermissionScope(nil))
	})

	t.Run("empty non-nil permissions grant nothing", func(t *testing.T) {
		got := auditGrantedResources(domain.UserPermissions{})
		assert.NotNil(t, got)
		assert.Empty(t, got)
		assert.Equal(t, "restricted", auditPermissionScope(domain.UserPermissions{}))
	})

	t.Run("resources with neither read nor write are excluded", func(t *testing.T) {
		perms := domain.UserPermissions{
			domain.PermissionResourceContacts:   {Read: false, Write: false},
			domain.PermissionResourceBroadcasts: {Read: false, Write: false},
		}
		assert.Empty(t, auditGrantedResources(perms))
	})

	t.Run("read-only and write-only grants both count, sorted", func(t *testing.T) {
		perms := domain.UserPermissions{
			domain.PermissionResourceWorkspace:    {Read: true, Write: true},
			domain.PermissionResourceContacts:     {Write: true},
			domain.PermissionResourceBroadcasts:   {Read: true},
			domain.PermissionResourceWebAnalytics: {Read: false, Write: false},
		}
		assert.Equal(t, []string{"broadcasts", "contacts", "workspace"}, auditGrantedResources(perms))
	})

	t.Run("full permissions list every resource sorted and scope is full", func(t *testing.T) {
		perms := domain.NewFullPermissions()
		got := auditGrantedResources(perms)
		require.NotEmpty(t, got)
		assert.IsIncreasing(t, got)
		assert.Contains(t, got, "contacts")
		assert.Contains(t, got, "workspace")
		assert.Contains(t, got, "web_analytics")
		assert.Equal(t, "full", auditPermissionScope(perms))
	})

	t.Run("a single dropped verb makes the scope restricted", func(t *testing.T) {
		perms := domain.NewFullPermissions()
		perms[domain.PermissionResourceContacts] = domain.ResourcePermissions{Read: true, Write: false}
		assert.Equal(t, "restricted", auditPermissionScope(perms))
		// The resource is still granted (read remains), so it stays listed.
		assert.Contains(t, auditGrantedResources(perms), "contacts")
	})
}

// ---------------------------------------------------------------------------
// CreateIntegration
// ---------------------------------------------------------------------------

func TestWorkspaceServiceCoverage_CreateIntegration(t *testing.T) {
	ctx := context.Background()
	const workspaceID = "ws-cov"
	const userID = "user-1"

	mailgunProvider := func() domain.EmailProvider {
		return domain.EmailProvider{
			Kind:               domain.EmailProviderKindMailgun,
			RateLimitPerMinute: 25,
			Mailgun: &domain.MailgunSettings{
				APIKey: "cov-mailgun-key",
				Domain: "mg.example.com",
			},
			Senders: []domain.EmailSender{domain.NewEmailSender("sender@example.com", "Sender")},
		}
	}

	t.Run("authentication failure wraps the sentinel and touches no repository", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		svc, deps := covWorkspaceService(ctrl, nil, nil, nil)

		sentinel := errors.New("cov auth exploded")
		deps.auth.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, nil, nil, sentinel)
		// No repo expectations: gomock fails if GetByID/Update is reached.

		id, err := svc.CreateIntegration(ctx, domain.CreateIntegrationRequest{
			WorkspaceID: workspaceID, Name: "X", Type: domain.IntegrationTypeEmail, Provider: mailgunProvider(),
		})
		assert.Empty(t, id)
		require.Error(t, err)
		assert.EqualError(t, err, "failed to authenticate user: cov auth exploded")
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("non-owner is refused before any repository call", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		svc, deps := covWorkspaceService(ctrl, nil, nil, nil)
		deps.expectMember(ctx, workspaceID, userID)

		id, err := svc.CreateIntegration(ctx, domain.CreateIntegrationRequest{
			WorkspaceID: workspaceID, Name: "X", Type: domain.IntegrationTypeEmail, Provider: mailgunProvider(),
		})
		assert.Empty(t, id)
		require.Error(t, err)
		var unauth *domain.ErrUnauthorized
		require.ErrorAs(t, err, &unauth)
		assert.EqualError(t, err, "user is not an owner of the workspace")
	})

	t.Run("empty name fails validation inside addIntegration and never writes", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		svc, deps := covWorkspaceService(ctrl, nil, nil, nil)
		deps.expectOwner(ctx, workspaceID, userID)
		deps.repo.EXPECT().GetByID(ctx, workspaceID).Return(&domain.Workspace{ID: workspaceID, Name: "WS"}, nil)
		// No Update expectation: validation failure must stop the write.

		id, err := svc.CreateIntegration(ctx, domain.CreateIntegrationRequest{
			WorkspaceID: workspaceID, Name: "", Type: domain.IntegrationTypeEmail, Provider: mailgunProvider(),
		})
		assert.Empty(t, id)
		require.Error(t, err)
		assert.EqualError(t, err, "integration name is required")
	})

	t.Run("unsupported type fails validation and never writes", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		svc, deps := covWorkspaceService(ctrl, nil, nil, nil)
		deps.expectOwner(ctx, workspaceID, userID)
		deps.repo.EXPECT().GetByID(ctx, workspaceID).Return(&domain.Workspace{ID: workspaceID, Name: "WS"}, nil)

		id, err := svc.CreateIntegration(ctx, domain.CreateIntegrationRequest{
			WorkspaceID: workspaceID, Name: "Bogus", Type: domain.IntegrationType("bogus-type"),
		})
		assert.Empty(t, id)
		require.Error(t, err)
		assert.EqualError(t, err, "unsupported integration type: bogus-type")
	})

	t.Run("non-SMTP email integration registers webhooks with the exact event set", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		webhook := mocks.NewMockWebhookRegistrationService(ctrl)
		svc, deps := covWorkspaceService(ctrl, webhook, nil, nil)
		deps.expectOwner(ctx, workspaceID, userID)
		deps.repo.EXPECT().GetByID(ctx, workspaceID).Return(&domain.Workspace{ID: workspaceID, Name: "WS"}, nil)

		var written *domain.Workspace
		deps.repo.EXPECT().Update(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, ws *domain.Workspace) error {
			written = ws
			return nil
		})
		webhook.EXPECT().RegisterWebhooks(ctx, workspaceID, gomock.Any()).
			DoAndReturn(func(_ context.Context, wsID string, cfg *domain.WebhookRegistrationConfig) (*domain.WebhookRegistrationStatus, error) {
				assert.Equal(t, workspaceID, wsID)
				require.NotNil(t, cfg)
				assert.NotEmpty(t, cfg.IntegrationID)
				assert.Equal(t, []domain.EmailEventType{
					domain.EmailEventDelivered, domain.EmailEventBounce, domain.EmailEventComplaint,
				}, cfg.EventTypes)
				return &domain.WebhookRegistrationStatus{}, nil
			})

		id, err := svc.CreateIntegration(ctx, domain.CreateIntegrationRequest{
			WorkspaceID: workspaceID, Name: "Mailgun Cov", Type: domain.IntegrationTypeEmail, Provider: mailgunProvider(),
		})
		require.NoError(t, err)
		require.NotEmpty(t, id)
		require.NotNil(t, written)
		require.Len(t, written.Integrations, 1)
		assert.Equal(t, id, written.Integrations[0].ID)
		assert.Equal(t, "Mailgun Cov", written.Integrations[0].Name)
		assert.Equal(t, domain.IntegrationTypeEmail, written.Integrations[0].Type)
		assert.Equal(t, domain.EmailProviderKindMailgun, written.Integrations[0].EmailProvider.Kind)
	})

	t.Run("webhook registration failure is logged and the creation still succeeds", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		webhook := mocks.NewMockWebhookRegistrationService(ctrl)
		svc, deps := covWorkspaceService(ctrl, webhook, nil, nil)
		deps.expectOwner(ctx, workspaceID, userID)
		deps.repo.EXPECT().GetByID(ctx, workspaceID).Return(&domain.Workspace{ID: workspaceID, Name: "WS"}, nil)
		deps.repo.EXPECT().Update(ctx, gomock.Any()).Return(nil)
		webhook.EXPECT().RegisterWebhooks(ctx, workspaceID, gomock.Any()).Return(nil, errors.New("cov webhook down"))

		id, err := svc.CreateIntegration(ctx, domain.CreateIntegrationRequest{
			WorkspaceID: workspaceID, Name: "Mailgun Cov", Type: domain.IntegrationTypeEmail, Provider: mailgunProvider(),
		})
		require.NoError(t, err)
		assert.NotEmpty(t, id)
	})

	t.Run("LLM integration is stored with its provider intact", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		svc, deps := covWorkspaceService(ctrl, nil, nil, nil)
		deps.expectOwner(ctx, workspaceID, userID)
		deps.repo.EXPECT().GetByID(ctx, workspaceID).Return(&domain.Workspace{ID: workspaceID, Name: "WS"}, nil)
		deps.repo.EXPECT().Update(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, ws *domain.Workspace) error {
			require.Len(t, ws.Integrations, 1)
			require.NotNil(t, ws.Integrations[0].LLMProvider)
			assert.Equal(t, domain.LLMProviderKindAnthropic, ws.Integrations[0].LLMProvider.Kind)
			require.NotNil(t, ws.Integrations[0].LLMProvider.Anthropic)
			assert.Equal(t, "claude-cov-model", ws.Integrations[0].LLMProvider.Anthropic.Model)
			return nil
		})

		id, err := svc.CreateIntegration(ctx, domain.CreateIntegrationRequest{
			WorkspaceID: workspaceID, Name: "Claude Cov", Type: domain.IntegrationTypeLLM,
			LLMProvider: &domain.LLMProvider{
				Kind:      domain.LLMProviderKindAnthropic,
				Anthropic: &domain.AnthropicSettings{APIKey: "cov-anthropic-key", Model: "claude-cov-model"},
			},
		})
		require.NoError(t, err)
		assert.NotEmpty(t, id)
	})

	t.Run("supabase template failure is logged and the creation still succeeds", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		templateRepo := mocks.NewMockTemplateRepository(ctrl)
		templateRepo.EXPECT().CreateTemplate(gomock.Any(), workspaceID, gomock.Any()).Return(errors.New("cov template store down"))
		mockLogger := pkgmocks.NewMockLogger(ctrl)
		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
		supabase := NewSupabaseService(nil, nil, nil, nil, nil, templateRepo, nil, nil, nil, nil, mockLogger)

		svc, deps := covWorkspaceService(ctrl, nil, supabase, nil)
		deps.expectOwner(ctx, workspaceID, userID)
		deps.repo.EXPECT().GetByID(ctx, workspaceID).Return(&domain.Workspace{ID: workspaceID, Name: "WS"}, nil)
		deps.repo.EXPECT().Update(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, ws *domain.Workspace) error {
			require.Len(t, ws.Integrations, 1)
			assert.Equal(t, domain.IntegrationTypeSupabase, ws.Integrations[0].Type)
			require.NotNil(t, ws.Integrations[0].SupabaseSettings)
			return nil
		})

		id, err := svc.CreateIntegration(ctx, domain.CreateIntegrationRequest{
			WorkspaceID: workspaceID, Name: "Supabase Cov", Type: domain.IntegrationTypeSupabase,
			SupabaseSettings: &domain.SupabaseIntegrationSettings{},
		})
		require.NoError(t, err)
		assert.NotEmpty(t, id)
	})

	t.Run("supabase happy path creates templates and notifications", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		templateRepo := mocks.NewMockTemplateRepository(ctrl)
		templateRepo.EXPECT().CreateTemplate(gomock.Any(), workspaceID, gomock.Any()).Return(nil).Times(6)
		txRepo := mocks.NewMockTransactionalNotificationRepository(ctrl)
		txRepo.EXPECT().Create(gomock.Any(), workspaceID, gomock.Any()).Return(nil).Times(6)
		mockLogger := pkgmocks.NewMockLogger(ctrl)
		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
		supabase := NewSupabaseService(nil, nil, nil, nil, nil, templateRepo, nil, txRepo, nil, nil, mockLogger)

		svc, deps := covWorkspaceService(ctrl, nil, supabase, nil)
		deps.expectOwner(ctx, workspaceID, userID)
		deps.repo.EXPECT().GetByID(ctx, workspaceID).Return(&domain.Workspace{ID: workspaceID, Name: "WS"}, nil)
		deps.repo.EXPECT().Update(ctx, gomock.Any()).Return(nil)

		id, err := svc.CreateIntegration(ctx, domain.CreateIntegrationRequest{
			WorkspaceID: workspaceID, Name: "Supabase Cov", Type: domain.IntegrationTypeSupabase,
			SupabaseSettings: &domain.SupabaseIntegrationSettings{},
		})
		require.NoError(t, err)
		assert.NotEmpty(t, id)
	})

	t.Run("supabase notification failure is logged and the creation still succeeds", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		templateRepo := mocks.NewMockTemplateRepository(ctrl)
		templateRepo.EXPECT().CreateTemplate(gomock.Any(), workspaceID, gomock.Any()).Return(nil).Times(6)
		txRepo := mocks.NewMockTransactionalNotificationRepository(ctrl)
		txRepo.EXPECT().Create(gomock.Any(), workspaceID, gomock.Any()).Return(errors.New("cov notification store down")).Times(1)
		mockLogger := pkgmocks.NewMockLogger(ctrl)
		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
		supabase := NewSupabaseService(nil, nil, nil, nil, nil, templateRepo, nil, txRepo, nil, nil, mockLogger)

		svc, deps := covWorkspaceService(ctrl, nil, supabase, nil)
		deps.expectOwner(ctx, workspaceID, userID)
		deps.repo.EXPECT().GetByID(ctx, workspaceID).Return(&domain.Workspace{ID: workspaceID, Name: "WS"}, nil)
		deps.repo.EXPECT().Update(ctx, gomock.Any()).Return(nil)

		id, err := svc.CreateIntegration(ctx, domain.CreateIntegrationRequest{
			WorkspaceID: workspaceID, Name: "Supabase Cov", Type: domain.IntegrationTypeSupabase,
			SupabaseSettings: &domain.SupabaseIntegrationSettings{},
		})
		require.NoError(t, err)
		assert.NotEmpty(t, id)
	})

	t.Run("creation enriches the audit record with target, type and provider", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		webhook := mocks.NewMockWebhookRegistrationService(ctrl)
		webhook.EXPECT().RegisterWebhooks(gomock.Any(), workspaceID, gomock.Any()).Return(&domain.WebhookRegistrationStatus{}, nil)
		svc, deps := covWorkspaceService(ctrl, webhook, nil, nil)

		record := domain.NewAuditRecord("203.0.113.9", "cov-agent", "cov-req")
		auditCtx := domain.WithAuditRecord(ctx, record)
		deps.auth.EXPECT().AuthenticateUserForWorkspace(auditCtx, workspaceID).
			Return(auditCtx, &domain.User{ID: userID}, &domain.UserWorkspace{UserID: userID, WorkspaceID: workspaceID, Role: "owner"}, nil)
		deps.repo.EXPECT().GetByID(auditCtx, workspaceID).Return(&domain.Workspace{ID: workspaceID, Name: "WS"}, nil)
		deps.repo.EXPECT().Update(auditCtx, gomock.Any()).Return(nil)

		id, err := svc.CreateIntegration(auditCtx, domain.CreateIntegrationRequest{
			WorkspaceID: workspaceID, Name: "Audited Mailgun", Type: domain.IntegrationTypeEmail, Provider: mailgunProvider(),
		})
		require.NoError(t, err)
		require.NotEmpty(t, id)

		event, skipped := record.Finalize(200)
		require.False(t, skipped)
		assert.Equal(t, domain.AuditTargetIntegration, event.TargetType)
		assert.Equal(t, id, event.TargetID)
		assert.Equal(t, "Audited Mailgun", event.TargetName)
		assert.Equal(t, "email", event.Metadata["type"])
		assert.Equal(t, "mailgun", event.Metadata["provider"])
	})
}

// ---------------------------------------------------------------------------
// UpdateIntegration
// ---------------------------------------------------------------------------

func TestWorkspaceServiceCoverage_UpdateIntegration(t *testing.T) {
	ctx := context.Background()
	const workspaceID = "ws-cov"
	const userID = "user-1"
	const integrationID = "int-cov-1"
	createdAt := time.Now().Add(-48 * time.Hour).UTC()

	smtpWorkspace := func() *domain.Workspace {
		return &domain.Workspace{
			ID: workspaceID, Name: "WS",
			Integrations: []domain.Integration{{
				ID: integrationID, Name: "Original", Type: domain.IntegrationTypeEmail,
				EmailProvider: domain.EmailProvider{
					Kind:    domain.EmailProviderKindSMTP,
					SMTP:    &domain.SMTPSettings{Host: "smtp.old.example.com", Port: 587, Username: "old", Password: "oldpw", UseTLS: true},
					Senders: []domain.EmailSender{domain.NewEmailSender("old@example.com", "Old")},
				},
				CreatedAt: createdAt, UpdatedAt: createdAt,
			}},
		}
	}

	t.Run("authentication failure wraps the sentinel and touches no repository", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		svc, deps := covWorkspaceService(ctrl, nil, nil, nil)
		sentinel := errors.New("cov auth exploded")
		deps.auth.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, nil, nil, sentinel)

		err := svc.UpdateIntegration(ctx, domain.UpdateIntegrationRequest{WorkspaceID: workspaceID, IntegrationID: integrationID, Name: "New"})
		require.Error(t, err)
		assert.EqualError(t, err, "failed to authenticate user: cov auth exploded")
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("non-owner is refused before any repository call", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		svc, deps := covWorkspaceService(ctrl, nil, nil, nil)
		deps.expectMember(ctx, workspaceID, userID)

		err := svc.UpdateIntegration(ctx, domain.UpdateIntegrationRequest{WorkspaceID: workspaceID, IntegrationID: integrationID, Name: "New"})
		require.Error(t, err)
		var unauth *domain.ErrUnauthorized
		require.ErrorAs(t, err, &unauth)
		assert.EqualError(t, err, "user is not an owner of the workspace")
	})

	t.Run("workspace load failure propagates verbatim", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		svc, deps := covWorkspaceService(ctrl, nil, nil, nil)
		deps.expectOwner(ctx, workspaceID, userID)
		sentinel := errors.New("cov workspace store down")
		deps.repo.EXPECT().GetByID(ctx, workspaceID).Return(nil, sentinel)

		err := svc.UpdateIntegration(ctx, domain.UpdateIntegrationRequest{WorkspaceID: workspaceID, IntegrationID: integrationID, Name: "New"})
		require.Error(t, err)
		assert.EqualError(t, err, "cov workspace store down")
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("blank name fails validation and never writes", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		svc, deps := covWorkspaceService(ctrl, nil, nil, nil)
		deps.expectOwner(ctx, workspaceID, userID)
		deps.repo.EXPECT().GetByID(ctx, workspaceID).Return(smtpWorkspace(), nil)
		// No Update expectation.

		err := svc.UpdateIntegration(ctx, domain.UpdateIntegrationRequest{WorkspaceID: workspaceID, IntegrationID: integrationID, Name: ""})
		require.Error(t, err)
		assert.EqualError(t, err, "integration name is required")
	})

	t.Run("repository write failure propagates verbatim", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		svc, deps := covWorkspaceService(ctrl, nil, nil, nil)
		deps.expectOwner(ctx, workspaceID, userID)
		deps.repo.EXPECT().GetByID(ctx, workspaceID).Return(smtpWorkspace(), nil)
		sentinel := errors.New("cov workspace write down")
		deps.repo.EXPECT().Update(ctx, gomock.Any()).Return(sentinel)

		err := svc.UpdateIntegration(ctx, domain.UpdateIntegrationRequest{
			WorkspaceID: workspaceID, IntegrationID: integrationID, Name: "Renamed",
			Provider: domain.EmailProvider{
				Kind:               domain.EmailProviderKindSMTP,
				RateLimitPerMinute: 25,
				SMTP:               &domain.SMTPSettings{Host: "smtp.new.example.com", Port: 2525, Username: "new", Password: "newpw", UseTLS: false},
				Senders:            []domain.EmailSender{domain.NewEmailSender("new@example.com", "New")},
			},
		})
		require.Error(t, err)
		assert.EqualError(t, err, "cov workspace write down")
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("supabase update preserves stored encrypted hook keys when new keys are blank", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		svc, deps := covWorkspaceService(ctrl, nil, nil, nil)
		deps.expectOwner(ctx, workspaceID, userID)
		ws := &domain.Workspace{
			ID: workspaceID, Name: "WS",
			Integrations: []domain.Integration{{
				ID: integrationID, Name: "Original Supabase", Type: domain.IntegrationTypeSupabase,
				SupabaseSettings: &domain.SupabaseIntegrationSettings{
					AuthEmailHook:         domain.SupabaseAuthEmailHookSettings{EncryptedSignatureKey: "cov-enc-auth-key"},
					BeforeUserCreatedHook: domain.SupabaseUserCreatedHookSettings{EncryptedSignatureKey: "cov-enc-before-key"},
				},
				CreatedAt: createdAt, UpdatedAt: createdAt,
			}},
		}
		deps.repo.EXPECT().GetByID(ctx, workspaceID).Return(ws, nil)
		deps.repo.EXPECT().Update(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, got *domain.Workspace) error {
			require.Len(t, got.Integrations, 1)
			updated := got.Integrations[0]
			assert.Equal(t, "Renamed Supabase", updated.Name)
			assert.Equal(t, createdAt, updated.CreatedAt)
			require.NotNil(t, updated.SupabaseSettings)
			assert.Equal(t, "cov-enc-auth-key", updated.SupabaseSettings.AuthEmailHook.EncryptedSignatureKey)
			assert.Equal(t, "cov-enc-before-key", updated.SupabaseSettings.BeforeUserCreatedHook.EncryptedSignatureKey)
			return nil
		})

		err := svc.UpdateIntegration(ctx, domain.UpdateIntegrationRequest{
			WorkspaceID: workspaceID, IntegrationID: integrationID, Name: "Renamed Supabase",
			SupabaseSettings: &domain.SupabaseIntegrationSettings{},
		})
		require.NoError(t, err)
	})

	t.Run("supabase update with fresh plaintext keys does not resurrect the stored ciphertext", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		svc, deps := covWorkspaceService(ctrl, nil, nil, nil)
		deps.expectOwner(ctx, workspaceID, userID)
		ws := &domain.Workspace{
			ID: workspaceID, Name: "WS",
			Integrations: []domain.Integration{{
				ID: integrationID, Name: "Original Supabase", Type: domain.IntegrationTypeSupabase,
				SupabaseSettings: &domain.SupabaseIntegrationSettings{
					AuthEmailHook:         domain.SupabaseAuthEmailHookSettings{EncryptedSignatureKey: "cov-enc-auth-key"},
					BeforeUserCreatedHook: domain.SupabaseUserCreatedHookSettings{EncryptedSignatureKey: "cov-enc-before-key"},
				},
				CreatedAt: createdAt, UpdatedAt: createdAt,
			}},
		}
		deps.repo.EXPECT().GetByID(ctx, workspaceID).Return(ws, nil)
		deps.repo.EXPECT().Update(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, got *domain.Workspace) error {
			updated := got.Integrations[0]
			require.NotNil(t, updated.SupabaseSettings)
			assert.Equal(t, "cov-new-auth-key", updated.SupabaseSettings.AuthEmailHook.SignatureKey)
			assert.Equal(t, "cov-new-before-key", updated.SupabaseSettings.BeforeUserCreatedHook.SignatureKey)
			assert.Empty(t, updated.SupabaseSettings.AuthEmailHook.EncryptedSignatureKey)
			assert.Empty(t, updated.SupabaseSettings.BeforeUserCreatedHook.EncryptedSignatureKey)
			return nil
		})

		err := svc.UpdateIntegration(ctx, domain.UpdateIntegrationRequest{
			WorkspaceID: workspaceID, IntegrationID: integrationID, Name: "Rotated Supabase",
			SupabaseSettings: &domain.SupabaseIntegrationSettings{
				AuthEmailHook:         domain.SupabaseAuthEmailHookSettings{SignatureKey: "cov-new-auth-key"},
				BeforeUserCreatedHook: domain.SupabaseUserCreatedHookSettings{SignatureKey: "cov-new-before-key"},
			},
		})
		require.NoError(t, err)
	})

	t.Run("supabase update with no settings preserves the stored settings pointer values", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		svc, deps := covWorkspaceService(ctrl, nil, nil, nil)
		deps.expectOwner(ctx, workspaceID, userID)
		ws := &domain.Workspace{
			ID: workspaceID, Name: "WS",
			Integrations: []domain.Integration{{
				ID: integrationID, Name: "Original Supabase", Type: domain.IntegrationTypeSupabase,
				SupabaseSettings: &domain.SupabaseIntegrationSettings{
					AuthEmailHook: domain.SupabaseAuthEmailHookSettings{EncryptedSignatureKey: "cov-enc-kept"},
				},
				CreatedAt: createdAt, UpdatedAt: createdAt,
			}},
		}
		deps.repo.EXPECT().GetByID(ctx, workspaceID).Return(ws, nil)
		deps.repo.EXPECT().Update(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, got *domain.Workspace) error {
			require.NotNil(t, got.Integrations[0].SupabaseSettings)
			assert.Equal(t, "cov-enc-kept", got.Integrations[0].SupabaseSettings.AuthEmailHook.EncryptedSignatureKey)
			return nil
		})

		err := svc.UpdateIntegration(ctx, domain.UpdateIntegrationRequest{
			WorkspaceID: workspaceID, IntegrationID: integrationID, Name: "Renamed Only",
		})
		require.NoError(t, err)
	})

	t.Run("SES update preserves server-owned tenant fields and a caller-supplied topic ARN wins", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		svc, deps := covWorkspaceService(ctrl, nil, nil, nil)
		deps.expectOwner(ctx, workspaceID, userID)
		ws := &domain.Workspace{
			ID: workspaceID, Name: "WS",
			Integrations: []domain.Integration{{
				ID: integrationID, Name: "Original SES", Type: domain.IntegrationTypeEmail,
				EmailProvider: domain.EmailProvider{
					Kind:               domain.EmailProviderKindSES,
					RateLimitPerMinute: 25,
					SES: &domain.AmazonSESSettings{
						Region: "us-east-1", AccessKey: "AKIDOLD", SecretKey: "oldsecret",
						InboundTopicARN: "arn:aws:sns:old-topic", ManagedConfigurationSet: "cov-config-set", ManagedTenantName: "cov-tenant",
					},
					Senders: []domain.EmailSender{domain.NewEmailSender("old@example.com", "Old")},
				},
				CreatedAt: createdAt, UpdatedAt: createdAt,
			}},
		}
		deps.repo.EXPECT().GetByID(ctx, workspaceID).Return(ws, nil)
		deps.repo.EXPECT().Update(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, got *domain.Workspace) error {
			ses := got.Integrations[0].EmailProvider.SES
			require.NotNil(t, ses)
			assert.Equal(t, "cov-config-set", ses.ManagedConfigurationSet)
			assert.Equal(t, "cov-tenant", ses.ManagedTenantName)
			assert.Equal(t, "arn:aws:sns:new-topic", ses.InboundTopicARN)
			return nil
		})

		err := svc.UpdateIntegration(ctx, domain.UpdateIntegrationRequest{
			WorkspaceID: workspaceID, IntegrationID: integrationID, Name: "Renamed SES",
			Provider: domain.EmailProvider{
				Kind:               domain.EmailProviderKindSES,
				RateLimitPerMinute: 25,
				SES: &domain.AmazonSESSettings{
					Region: "eu-west-1", AccessKey: "AKIDNEW", SecretKey: "newsecret",
					InboundTopicARN: "arn:aws:sns:new-topic",
				},
				Senders: []domain.EmailSender{domain.NewEmailSender("new@example.com", "New")},
			},
		})
		require.NoError(t, err)
	})

	t.Run("anthropic LLM update preserves the stored encrypted key when blank", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		svc, deps := covWorkspaceService(ctrl, nil, nil, nil)
		deps.expectOwner(ctx, workspaceID, userID)
		ws := &domain.Workspace{
			ID: workspaceID, Name: "WS",
			Integrations: []domain.Integration{{
				ID: integrationID, Name: "Original Claude", Type: domain.IntegrationTypeLLM,
				LLMProvider: &domain.LLMProvider{
					Kind:      domain.LLMProviderKindAnthropic,
					Anthropic: &domain.AnthropicSettings{EncryptedAPIKey: "cov-enc-anthropic", Model: "claude-old"},
				},
				CreatedAt: createdAt, UpdatedAt: createdAt,
			}},
		}
		deps.repo.EXPECT().GetByID(ctx, workspaceID).Return(ws, nil)
		deps.repo.EXPECT().Update(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, got *domain.Workspace) error {
			anthropic := got.Integrations[0].LLMProvider.Anthropic
			require.NotNil(t, anthropic)
			assert.Equal(t, "cov-enc-anthropic", anthropic.EncryptedAPIKey)
			assert.Equal(t, "claude-new", anthropic.Model)
			return nil
		})

		err := svc.UpdateIntegration(ctx, domain.UpdateIntegrationRequest{
			WorkspaceID: workspaceID, IntegrationID: integrationID, Name: "Renamed Claude",
			LLMProvider: &domain.LLMProvider{
				Kind:      domain.LLMProviderKindAnthropic,
				Anthropic: &domain.AnthropicSettings{APIKey: "", Model: "claude-new"},
			},
		})
		require.NoError(t, err)
	})
}

// ---------------------------------------------------------------------------
// DeleteIntegration
// ---------------------------------------------------------------------------

func TestWorkspaceServiceCoverage_DeleteIntegration(t *testing.T) {
	ctx := context.Background()
	const workspaceID = "ws-cov"
	const userID = "user-1"
	const integrationID = "int-cov-1"

	t.Run("authentication failure wraps the sentinel and touches no repository", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		svc, deps := covWorkspaceService(ctrl, nil, nil, nil)
		sentinel := errors.New("cov auth exploded")
		deps.auth.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, nil, nil, sentinel)

		err := svc.DeleteIntegration(ctx, workspaceID, integrationID)
		require.Error(t, err)
		assert.EqualError(t, err, "failed to authenticate user: cov auth exploded")
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("non-owner is refused before any repository call", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		svc, deps := covWorkspaceService(ctrl, nil, nil, nil)
		deps.expectMember(ctx, workspaceID, userID)

		err := svc.DeleteIntegration(ctx, workspaceID, integrationID)
		require.Error(t, err)
		var unauth *domain.ErrUnauthorized
		require.ErrorAs(t, err, &unauth)
		assert.EqualError(t, err, "user is not an owner of the workspace")
	})

	t.Run("workspace load failure propagates verbatim", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		svc, deps := covWorkspaceService(ctrl, nil, nil, nil)
		deps.expectOwner(ctx, workspaceID, userID)
		sentinel := errors.New("cov workspace store down")
		deps.repo.EXPECT().GetByID(ctx, workspaceID).Return(nil, sentinel)

		err := svc.DeleteIntegration(ctx, workspaceID, integrationID)
		require.Error(t, err)
		assert.EqualError(t, err, "cov workspace store down")
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("repository write failure propagates verbatim and keeps the integration", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		svc, deps := covWorkspaceService(ctrl, nil, nil, nil)
		deps.expectOwner(ctx, workspaceID, userID)
		ws := &domain.Workspace{
			ID: workspaceID, Name: "WS",
			Integrations: []domain.Integration{{
				ID: integrationID, Name: "SMTP", Type: domain.IntegrationTypeEmail,
				EmailProvider: domain.EmailProvider{Kind: domain.EmailProviderKindSMTP, SMTP: &domain.SMTPSettings{Host: "h", Port: 587}},
			}},
		}
		deps.repo.EXPECT().GetByID(ctx, workspaceID).Return(ws, nil)
		sentinel := errors.New("cov workspace write down")
		deps.repo.EXPECT().Update(ctx, gomock.Any()).Return(sentinel)

		err := svc.DeleteIntegration(ctx, workspaceID, integrationID)
		require.Error(t, err)
		assert.EqualError(t, err, "cov workspace write down")
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("non-SMTP email deletion tears down provider resources with exact arguments", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		webhook := mocks.NewMockWebhookRegistrationService(ctrl)
		svc, deps := covWorkspaceService(ctrl, webhook, nil, nil)
		deps.expectOwner(ctx, workspaceID, userID)
		ws := &domain.Workspace{
			ID: workspaceID, Name: "WS",
			Settings: domain.WorkspaceSettings{
				TransactionalEmailProviderID: integrationID,
				MarketingEmailProviderID:     integrationID,
			},
			Integrations: []domain.Integration{{
				ID: integrationID, Name: "Mailgun", Type: domain.IntegrationTypeEmail,
				EmailProvider: domain.EmailProvider{
					Kind:    domain.EmailProviderKindMailgun,
					Mailgun: &domain.MailgunSettings{Domain: "mg.example.com", APIKey: "k"},
					Senders: []domain.EmailSender{domain.NewEmailSender("s@example.com", "S")},
				},
			}},
		}
		deps.repo.EXPECT().GetByID(ctx, workspaceID).Return(ws, nil)
		webhook.EXPECT().DeleteIntegrationResources(ctx, workspaceID, integrationID).Return(nil)
		deps.repo.EXPECT().Update(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, got *domain.Workspace) error {
			assert.Empty(t, got.Integrations)
			assert.Empty(t, got.Settings.TransactionalEmailProviderID)
			assert.Empty(t, got.Settings.MarketingEmailProviderID)
			return nil
		})

		require.NoError(t, svc.DeleteIntegration(ctx, workspaceID, integrationID))
	})

	t.Run("provider teardown failure is logged and deletion still succeeds", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		webhook := mocks.NewMockWebhookRegistrationService(ctrl)
		svc, deps := covWorkspaceService(ctrl, webhook, nil, nil)
		deps.expectOwner(ctx, workspaceID, userID)
		ws := &domain.Workspace{
			ID: workspaceID, Name: "WS",
			Integrations: []domain.Integration{{
				ID: integrationID, Name: "Mailgun", Type: domain.IntegrationTypeEmail,
				EmailProvider: domain.EmailProvider{
					Kind:    domain.EmailProviderKindMailgun,
					Mailgun: &domain.MailgunSettings{Domain: "mg.example.com", APIKey: "k"},
					Senders: []domain.EmailSender{domain.NewEmailSender("s@example.com", "S")},
				},
			}},
		}
		deps.repo.EXPECT().GetByID(ctx, workspaceID).Return(ws, nil)
		webhook.EXPECT().DeleteIntegrationResources(ctx, workspaceID, integrationID).Return(errors.New("cov provider teardown down"))
		deps.repo.EXPECT().Update(ctx, gomock.Any()).Return(nil)

		require.NoError(t, svc.DeleteIntegration(ctx, workspaceID, integrationID))
	})

	t.Run("supabase cleanup failure is logged and deletion still succeeds", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		templateRepo := mocks.NewMockTemplateRepository(ctrl)
		templateRepo.EXPECT().GetTemplates(gomock.Any(), workspaceID, "", "").Return(nil, errors.New("cov template list down"))
		mockLogger := pkgmocks.NewMockLogger(ctrl)
		mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
		mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
		mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
		supabase := NewSupabaseService(nil, nil, nil, nil, nil, templateRepo, nil, nil, nil, nil, mockLogger)

		svc, deps := covWorkspaceService(ctrl, nil, supabase, nil)
		deps.expectOwner(ctx, workspaceID, userID)
		ws := &domain.Workspace{
			ID: workspaceID, Name: "WS",
			Integrations: []domain.Integration{{
				ID: integrationID, Name: "Supabase", Type: domain.IntegrationTypeSupabase,
				SupabaseSettings: &domain.SupabaseIntegrationSettings{},
			}},
		}
		deps.repo.EXPECT().GetByID(ctx, workspaceID).Return(ws, nil)
		deps.repo.EXPECT().Update(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, got *domain.Workspace) error {
			assert.Empty(t, got.Integrations)
			return nil
		})

		require.NoError(t, svc.DeleteIntegration(ctx, workspaceID, integrationID))
	})

	t.Run("zapier integration with nil settings deletes without touching user repositories", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		svc, deps := covWorkspaceService(ctrl, nil, nil, nil)
		deps.expectOwner(ctx, workspaceID, userID)
		ws := &domain.Workspace{
			ID: workspaceID, Name: "WS",
			Integrations: []domain.Integration{{
				ID: integrationID, Name: "Zaps", Type: domain.IntegrationTypeZapier, ZapierSettings: nil,
			}},
		}
		deps.repo.EXPECT().GetByID(ctx, workspaceID).Return(ws, nil)
		deps.repo.EXPECT().Update(ctx, gomock.Any()).Return(nil)

		require.NoError(t, svc.DeleteIntegration(ctx, workspaceID, integrationID))
	})
}

// ---------------------------------------------------------------------------
// SetWebAnalyticsSettings
// ---------------------------------------------------------------------------

func TestWorkspaceServiceCoverage_SetWebAnalyticsSettings(t *testing.T) {
	ctx := context.Background()
	const workspaceID = "ws-cov"
	const userID = "user-1"

	valid := func() *domain.WebAnalyticsSettings {
		return &domain.WebAnalyticsSettings{
			Enabled:                true,
			AllowedDomains:         []string{"example.com"},
			BounceThresholdSeconds: 9,
			Filters:                domain.DefaultWebFilters(),
			FiltersVersion:         "cov-forged-version",
		}
	}

	t.Run("authentication failure wraps the sentinel and touches no repository", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		svc, deps := covWorkspaceService(ctrl, nil, nil, nil)
		sentinel := errors.New("cov auth exploded")
		deps.auth.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, nil, nil, sentinel)

		err := svc.SetWebAnalyticsSettings(ctx, workspaceID, valid())
		require.Error(t, err)
		assert.EqualError(t, err, "failed to authenticate user: cov auth exploded")
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("read-only member gets the exact permission error and no repository call", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		svc, deps := covWorkspaceService(ctrl, nil, nil, nil)
		member := &domain.UserWorkspace{
			UserID: userID, WorkspaceID: workspaceID, Role: "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceWebAnalytics: {Read: true, Write: false},
			},
		}
		deps.auth.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, member, nil)

		err := svc.SetWebAnalyticsSettings(ctx, workspaceID, valid())
		require.Error(t, err)
		assert.EqualError(t, err, "Insufficient permissions: write access to web_analytics required")
		var permErr *domain.PermissionError
		require.ErrorAs(t, err, &permErr)
		assert.Equal(t, domain.PermissionResourceWebAnalytics, permErr.Resource)
		assert.Equal(t, domain.PermissionTypeWrite, permErr.Permission)
		assert.Equal(t, "Insufficient permissions: write access to web_analytics required", permErr.Message)
	})

	t.Run("workspace load failure propagates verbatim and never writes", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		svc, deps := covWorkspaceService(ctrl, nil, nil, nil)
		deps.expectOwner(ctx, workspaceID, userID)
		sentinel := errors.New("cov workspace store down")
		deps.repo.EXPECT().GetByID(ctx, workspaceID).Return(nil, sentinel)

		err := svc.SetWebAnalyticsSettings(ctx, workspaceID, valid())
		require.Error(t, err)
		assert.EqualError(t, err, "cov workspace store down")
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("repository write failure propagates verbatim", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		svc, deps := covWorkspaceService(ctrl, nil, nil, nil)
		deps.expectOwner(ctx, workspaceID, userID)
		existing := &domain.Workspace{ID: workspaceID, Name: "WS", Settings: domain.WorkspaceSettings{Timezone: "Europe/Paris"}}
		deps.repo.EXPECT().GetByID(ctx, workspaceID).Return(existing, nil)
		sentinel := errors.New("cov workspace write down")
		deps.repo.EXPECT().Update(ctx, gomock.Any()).Return(sentinel)

		err := svc.SetWebAnalyticsSettings(ctx, workspaceID, valid())
		require.Error(t, err)
		assert.EqualError(t, err, "cov workspace write down")
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("success preserves unrelated settings and recomputes the filters version", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		svc, deps := covWorkspaceService(ctrl, nil, nil, nil)
		deps.expectOwner(ctx, workspaceID, userID)
		existing := &domain.Workspace{
			ID: workspaceID, Name: "Cov WS",
			Settings: domain.WorkspaceSettings{
				Timezone: "Europe/Paris", WebsiteURL: "https://cov.example.com", DefaultLanguage: "fr",
			},
		}
		deps.repo.EXPECT().GetByID(ctx, workspaceID).Return(existing, nil)
		settings := valid()
		deps.repo.EXPECT().Update(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, got *domain.Workspace) error {
			assert.Equal(t, "Europe/Paris", got.Settings.Timezone)
			assert.Equal(t, "https://cov.example.com", got.Settings.WebsiteURL)
			assert.Equal(t, "fr", got.Settings.DefaultLanguage)
			require.NotNil(t, got.Settings.WebAnalytics)
			assert.True(t, got.Settings.WebAnalytics.Enabled)
			assert.Equal(t, []string{"example.com"}, got.Settings.WebAnalytics.AllowedDomains)
			assert.Equal(t, 9, got.Settings.WebAnalytics.BounceThresholdSeconds)
			assert.Equal(t, domain.ComputeWebFiltersVersion(settings.Filters), got.Settings.WebAnalytics.FiltersVersion)
			assert.NotEqual(t, "cov-forged-version", got.Settings.WebAnalytics.FiltersVersion)
			return nil
		})

		require.NoError(t, svc.SetWebAnalyticsSettings(ctx, workspaceID, settings))
		// The caller's struct is stamped too: the service never trusts the client hash.
		assert.Equal(t, domain.ComputeWebFiltersVersion(settings.Filters), settings.FiltersVersion)
	})
}
