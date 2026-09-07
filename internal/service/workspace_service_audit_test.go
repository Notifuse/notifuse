package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
)

type workspaceAuditFixture struct {
	repo     *mocks.MockWorkspaceRepository
	userRepo *mocks.MockUserRepository
	auth     *mocks.MockAuthService
	service  *WorkspaceService
}

func newWorkspaceAuditFixture(t *testing.T) *workspaceAuditFixture {
	t.Helper()
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

	f := &workspaceAuditFixture{
		repo:     mocks.NewMockWorkspaceRepository(ctrl),
		userRepo: mocks.NewMockUserRepository(ctrl),
		auth:     mocks.NewMockAuthService(ctrl),
	}
	f.service = NewWorkspaceService(
		f.repo,
		f.userRepo,
		mocks.NewMockTaskRepository(ctrl),
		mockLogger,
		mocks.NewMockUserServiceInterface(ctrl),
		f.auth,
		pkgmocks.NewMockMailer(ctrl),
		&config.Config{RootEmail: "root@example.com"},
		mocks.NewMockContactService(ctrl),
		mocks.NewMockListService(ctrl),
		mocks.NewMockContactListService(ctrl),
		mocks.NewMockTemplateService(ctrl),
		mocks.NewMockWebhookRegistrationService(ctrl),
		"secret_key",
		&SupabaseService{},
		&DNSVerificationService{},
		&BlogService{},
		fullyLicensedProvider(ctrl),
	)
	return f
}

// auditContext returns a context carrying a fresh record, the way the audit
// middleware hands one to every audited request, and the record to inspect.
func auditContext() (context.Context, *domain.AuditRecord) {
	record := domain.NewAuditRecord("203.0.113.9", "test", "req-1")
	return domain.WithAuditRecord(context.Background(), record), record
}

func TestWorkspaceService_SetUserPermissions_EnrichesTheAuditRecord(t *testing.T) {
	f := newWorkspaceAuditFixture(t)
	ctx, record := auditContext()
	const workspaceID, targetUserID = "ws1", "user123"

	before := domain.UserPermissions{domain.PermissionResourceContacts: {Read: true, Write: true}}
	after := domain.UserPermissions{domain.PermissionResourceContacts: {Read: true, Write: false}}

	f.auth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
		Return(ctx, &domain.User{ID: "owner"}, &domain.UserWorkspace{UserID: "owner", WorkspaceID: workspaceID, Role: "owner"}, nil)
	f.repo.EXPECT().GetUserWorkspace(gomock.Any(), targetUserID, workspaceID).
		Return(&domain.UserWorkspace{UserID: targetUserID, WorkspaceID: workspaceID, Role: "member", Permissions: before}, nil)
	f.repo.EXPECT().UpdateUserWorkspacePermissions(gomock.Any(), gomock.Any()).Return(nil)
	f.userRepo.EXPECT().GetSessionsByUserID(gomock.Any(), targetUserID).Return(nil, nil)

	require.NoError(t, f.service.SetUserPermissions(ctx, workspaceID, targetUserID, after))

	event, skipped := record.Finalize(200)
	require.False(t, skipped)
	assert.Equal(t, domain.AuditTargetUser, event.TargetType)
	assert.Equal(t, targetUserID, event.TargetID)
	assert.Equal(t, "restricted", event.Metadata["scope"])
	change, ok := event.Changes["permissions"]
	require.True(t, ok, "the old and new permission maps are the change")
	assert.Equal(t, before, change.Old)
	assert.Equal(t, after, change.New)
}

func TestWorkspaceService_SetAuditLogSettings(t *testing.T) {
	const workspaceID = "ws1"
	days := func(n int) *int { return &n }

	t.Run("owner only", func(t *testing.T) {
		f := newWorkspaceAuditFixture(t)
		f.auth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(context.Background(), &domain.User{ID: "m"}, &domain.UserWorkspace{Role: "member", Permissions: domain.NewFullPermissions()}, nil)
		err := f.service.SetAuditLogSettings(context.Background(), workspaceID, &domain.AuditLogSettings{RetentionDays: days(90)})
		var unauthorized *domain.ErrUnauthorized
		assert.ErrorAs(t, err, &unauthorized)
	})

	t.Run("validated before anything is loaded", func(t *testing.T) {
		f := newWorkspaceAuditFixture(t)
		f.auth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(context.Background(), &domain.User{ID: "o"}, &domain.UserWorkspace{Role: "owner"}, nil)
		err := f.service.SetAuditLogSettings(context.Background(), workspaceID, &domain.AuditLogSettings{RetentionDays: days(7)})
		assert.Error(t, err)
	})

	t.Run("persists the retention, keeps the other settings, records the change", func(t *testing.T) {
		f := newWorkspaceAuditFixture(t)
		ctx, record := auditContext()
		stored := &domain.Workspace{ID: workspaceID, Name: "Acme", Settings: domain.WorkspaceSettings{
			Timezone:  "Europe/Paris",
			AuditLogs: &domain.AuditLogSettings{RetentionDays: days(365)},
		}}
		f.auth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
			Return(ctx, &domain.User{ID: "o"}, &domain.UserWorkspace{Role: "owner"}, nil)
		f.repo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(stored, nil)
		var updated *domain.Workspace
		f.repo.EXPECT().Update(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, ws *domain.Workspace) error {
			updated = ws
			return nil
		})

		require.NoError(t, f.service.SetAuditLogSettings(ctx, workspaceID, &domain.AuditLogSettings{RetentionDays: days(90)}))
		require.NotNil(t, updated)
		assert.Equal(t, 90, *updated.Settings.AuditLogs.RetentionDays)
		assert.Equal(t, "Europe/Paris", updated.Settings.Timezone)

		event, _ := record.Finalize(200)
		assert.Equal(t, domain.AuditTargetWorkspace, event.TargetType)
		assert.Equal(t, "Acme", event.TargetName)
		change := event.Changes["retention_days"]
		assert.Equal(t, float64(365), change.Old)
		assert.Equal(t, float64(90), change.New)
	})
}

func TestWorkspaceService_UpdateIntegration_RedactsCredentialsInTheAuditRecord(t *testing.T) {
	f := newWorkspaceAuditFixture(t)
	ctx, record := auditContext()
	const workspaceID, integrationID = "ws1", "int-1"

	existing := domain.Integration{
		ID:   integrationID,
		Name: "Mailgun prod",
		Type: domain.IntegrationTypeEmail,
		EmailProvider: domain.EmailProvider{
			Kind:               domain.EmailProviderKindMailgun,
			Mailgun:            &domain.MailgunSettings{Domain: "mg.example.com", APIKey: "old-secret"},
			RateLimitPerMinute: 100,
			Senders:            []domain.EmailSender{{ID: "s1", Email: "no-reply@example.com", Name: "Acme"}},
		},
	}
	workspace := &domain.Workspace{ID: workspaceID, Name: "Acme", Integrations: []domain.Integration{existing}}

	f.auth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
		Return(ctx, &domain.User{ID: "o"}, &domain.UserWorkspace{Role: "owner"}, nil)
	f.repo.EXPECT().GetByID(gomock.Any(), workspaceID).Return(workspace, nil)
	f.repo.EXPECT().Update(gomock.Any(), gomock.Any()).Return(nil)

	err := f.service.UpdateIntegration(ctx, domain.UpdateIntegrationRequest{
		WorkspaceID:   workspaceID,
		IntegrationID: integrationID,
		Name:          "Mailgun staging",
		Provider: domain.EmailProvider{
			Kind:               domain.EmailProviderKindMailgun,
			Mailgun:            &domain.MailgunSettings{Domain: "mg.example.com", APIKey: "new-secret"},
			RateLimitPerMinute: 100,
			Senders:            []domain.EmailSender{{ID: "s1", Email: "no-reply@example.com", Name: "Acme"}},
		},
	})
	require.NoError(t, err)

	event, _ := record.Finalize(200)
	assert.Equal(t, domain.AuditTargetIntegration, event.TargetType)
	assert.Equal(t, integrationID, event.TargetID)
	assert.Equal(t, "Mailgun staging", event.TargetName)
	assert.Equal(t, domain.AuditChange{Old: "Mailgun prod", New: "Mailgun staging"}, event.Changes["name"])

	_, providerChanged := event.Changes["email_provider"]
	assert.True(t, providerChanged, "the provider block changed (its key) and is reported whole")

	// The security property: whatever the provider block serialises, no
	// credential value reaches the log.
	encoded, err := json.Marshal(event.Changes)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "old-secret")
	assert.NotContains(t, string(encoded), "new-secret")
	assert.Contains(t, string(encoded), domain.AuditRedactedValue, "the key change is kept, its values are not")
}
