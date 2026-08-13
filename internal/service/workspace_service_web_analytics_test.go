package service

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
)

func newWorkspaceServiceForWebAnalyticsTest(t *testing.T) (*WorkspaceService, *mocks.MockWorkspaceRepository, *mocks.MockAuthService, *mocks.MockUserServiceInterface, *mocks.MockContactService, *mocks.MockListService, *mocks.MockTaskRepository) {
	t.Helper()
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	mockRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockTaskRepo := mocks.NewMockTaskRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockUserService := mocks.NewMockUserServiceInterface(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockContactService := mocks.NewMockContactService(ctrl)
	mockListService := mocks.NewMockListService(ctrl)

	service := NewWorkspaceService(
		mockRepo,
		mocks.NewMockUserRepository(ctrl),
		mockTaskRepo,
		mockLogger,
		mockUserService,
		mockAuthService,
		pkgmocks.NewMockMailer(ctrl),
		&config.Config{RootEmail: "test@example.com"},
		mockContactService,
		mockListService,
		mocks.NewMockContactListService(ctrl),
		mocks.NewMockTemplateService(ctrl),
		mocks.NewMockWebhookRegistrationService(ctrl),
		"secret_key",
		&SupabaseService{},
		&DNSVerificationService{},
		&BlogService{},
	)

	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

	return service, mockRepo, mockAuthService, mockUserService, mockContactService, mockListService, mockTaskRepo
}

func TestWorkspaceService_CreateWorkspace_SeedsWebAnalyticsDefaults(t *testing.T) {
	service, mockRepo, mockAuthService, mockUserService, mockContactService, mockListService, mockTaskRepo := newWorkspaceServiceForWebAnalyticsTest(t)

	ctx := context.Background()
	workspaceID := "testworkspace"
	owner := &domain.User{ID: "owner", Email: "test@example.com", Name: "Owner"}

	mockAuthService.EXPECT().AuthenticateUserFromContext(ctx).Return(owner, nil)
	mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(nil, nil)

	var created *domain.Workspace
	mockRepo.EXPECT().Create(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, ws *domain.Workspace) error {
		created = ws
		return nil
	})
	mockRepo.EXPECT().AddUserToWorkspace(ctx, gomock.Any()).Return(nil)
	mockUserService.EXPECT().GetUserByID(ctx, owner.ID).Return(owner, nil)
	mockContactService.EXPECT().UpsertContact(ctx, workspaceID, gomock.Any()).Return(domain.UpsertContactOperation{Action: domain.UpsertContactOperationCreate})
	mockListService.EXPECT().CreateList(ctx, workspaceID, gomock.Any()).Return(nil)
	mockListService.EXPECT().SubscribeToLists(ctx, gomock.Any(), true).Return(nil)
	mockTaskRepo.EXPECT().List(ctx, workspaceID, gomock.Any()).Return([]*domain.Task{}, 0, nil).AnyTimes()
	mockTaskRepo.EXPECT().Create(ctx, workspaceID, gomock.Any()).Return(nil).AnyTimes()

	workspace, err := service.CreateWorkspace(ctx, workspaceID, "WS", "https://example.com", "", "", "UTC", domain.FileManagerSettings{}, "en", []string{"en"})
	require.NoError(t, err)

	for name, wa := range map[string]*domain.WebAnalyticsSettings{"persisted": created.Settings.WebAnalytics, "returned": workspace.Settings.WebAnalytics} {
		require.NotNil(t, wa, name)
		assert.False(t, wa.Enabled, "web analytics starts disabled")
		assert.Equal(t, domain.WebAnalyticsDefaultBounceThresholdSeconds, wa.BounceThresholdSeconds)
		assert.Len(t, wa.Filters, 40, "default attribution rules seeded")
		assert.Equal(t, domain.ComputeWebFiltersVersion(wa.Filters), wa.FiltersVersion)
		assert.True(t, wa.GeoEnabled)
		assert.True(t, wa.GeoStoreCity)
		assert.True(t, wa.GeoStoreRegion)
		assert.Equal(t, 2, wa.GeoCoordsPrecision)
	}
}

func TestWorkspaceService_SetWebAnalyticsSettings(t *testing.T) {
	ctx := context.Background()
	workspaceID := "testworkspace"
	userID := "member"

	validSettings := func() *domain.WebAnalyticsSettings {
		return &domain.WebAnalyticsSettings{
			Enabled:                true,
			AllowedDomains:         []string{"example.com"},
			BounceThresholdSeconds: 12,
			Filters:                domain.DefaultWebFilters(),
			FiltersVersion:         "forged00", // must be recomputed server-side
		}
	}

	t.Run("member with web_analytics write saves; version recomputed server-side", func(t *testing.T) {
		service, mockRepo, mockAuthService, _, _, _, _ := newWorkspaceServiceForWebAnalyticsTest(t)

		member := &domain.UserWorkspace{
			UserID: userID, WorkspaceID: workspaceID, Role: "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceWebAnalytics: {Read: true, Write: true},
			},
		}
		existing := &domain.Workspace{ID: workspaceID, Name: "WS", Settings: domain.WorkspaceSettings{Timezone: "UTC"}}
		settings := validSettings()

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, member, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(existing, nil)
		mockRepo.EXPECT().Update(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, ws *domain.Workspace) error {
			require.NotNil(t, ws.Settings.WebAnalytics)
			assert.True(t, ws.Settings.WebAnalytics.Enabled)
			assert.Equal(t, domain.ComputeWebFiltersVersion(ws.Settings.WebAnalytics.Filters), ws.Settings.WebAnalytics.FiltersVersion)
			assert.NotEqual(t, "forged00", ws.Settings.WebAnalytics.FiltersVersion)
			return nil
		})

		require.NoError(t, service.SetWebAnalyticsSettings(ctx, workspaceID, settings))
	})

	// The six subtests that used to sit here covered a contacts:write gate on
	// turning the two contact-timeline settings on. Both settings are gone —
	// calling identify() is the opt-in now, and that decision is made in the
	// customer's own code with the workspace secret rather than in this panel —
	// so there is no transition left to gate. Base permission coverage continues
	// below.

	t.Run("member without web_analytics write is rejected", func(t *testing.T) {
		service, _, mockAuthService, _, _, _, _ := newWorkspaceServiceForWebAnalyticsTest(t)

		member := &domain.UserWorkspace{
			UserID: userID, WorkspaceID: workspaceID, Role: "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceWebAnalytics: {Read: true, Write: false},
			},
		}
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, member, nil)

		err := service.SetWebAnalyticsSettings(ctx, workspaceID, validSettings())
		require.Error(t, err)
		var permErr *domain.PermissionError
		assert.ErrorAs(t, err, &permErr)
	})

	t.Run("invalid settings rejected before any write", func(t *testing.T) {
		service, _, mockAuthService, _, _, _, _ := newWorkspaceServiceForWebAnalyticsTest(t)

		owner := &domain.UserWorkspace{UserID: userID, WorkspaceID: workspaceID, Role: "owner"}
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, owner, nil)

		bad := validSettings()
		bad.GeoCoordsPrecision = 3
		err := service.SetWebAnalyticsSettings(ctx, workspaceID, bad)
		assert.ErrorContains(t, err, "geo_coordinates_precision")
	})

	t.Run("enabling with no allowed domain is rejected before any write", func(t *testing.T) {
		service, _, mockAuthService, _, _, _, _ := newWorkspaceServiceForWebAnalyticsTest(t)

		owner := &domain.UserWorkspace{UserID: userID, WorkspaceID: workspaceID, Role: "owner"}
		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, owner, nil)

		bad := validSettings()
		bad.AllowedDomains = nil
		err := service.SetWebAnalyticsSettings(ctx, workspaceID, bad)
		assert.ErrorContains(t, err, "allowed_domains")
	})

	t.Run("switching collection off with no allowed domain still saves", func(t *testing.T) {
		service, mockRepo, mockAuthService, _, _, _, _ := newWorkspaceServiceForWebAnalyticsTest(t)

		owner := &domain.UserWorkspace{UserID: userID, WorkspaceID: workspaceID, Role: "owner"}
		existing := &domain.Workspace{ID: workspaceID, Name: "WS", Settings: domain.WorkspaceSettings{Timezone: "UTC"}}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, owner, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(existing, nil)
		mockRepo.EXPECT().Update(ctx, gomock.Any()).Return(nil)

		settings := validSettings()
		settings.Enabled = false
		settings.AllowedDomains = nil
		require.NoError(t, service.SetWebAnalyticsSettings(ctx, workspaceID, settings))
	})

	t.Run("owner can clear settings with nil", func(t *testing.T) {
		service, mockRepo, mockAuthService, _, _, _, _ := newWorkspaceServiceForWebAnalyticsTest(t)

		owner := &domain.UserWorkspace{UserID: userID, WorkspaceID: workspaceID, Role: "owner"}
		existing := &domain.Workspace{ID: workspaceID, Name: "WS", Settings: domain.WorkspaceSettings{
			Timezone:     "UTC",
			WebAnalytics: &domain.WebAnalyticsSettings{Enabled: true},
		}}

		mockAuthService.EXPECT().AuthenticateUserForWorkspace(ctx, workspaceID).Return(ctx, &domain.User{ID: userID}, owner, nil)
		mockRepo.EXPECT().GetByID(ctx, workspaceID).Return(existing, nil)
		mockRepo.EXPECT().Update(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, ws *domain.Workspace) error {
			assert.Nil(t, ws.Settings.WebAnalytics)
			return nil
		})

		require.NoError(t, service.SetWebAnalyticsSettings(ctx, workspaceID, nil))
	})
}
