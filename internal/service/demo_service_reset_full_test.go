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
	"github.com/Notifuse/notifuse/pkg/cache"
	"github.com/Notifuse/notifuse/pkg/logger"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
)

// Full ResetDemo over the real service graph (real UserService,
// WorkspaceService, TemplateService, ListService, WebhookSubscriptionService
// on mock repositories): the old workspace is wiped, the demo workspace is
// created with its SMTP integration wired as both email providers, a sample
// -data failure (lists) degrades instead of aborting, and the webhook
// subscription ends up disabled.

func TestDemoService_ResetDemo_FullGraph(t *testing.T) {
	ctrl := gomock.NewController(t)
	log := logger.NewLoggerWithLevel("disabled")
	ctx := context.Background()
	cfg := &config.Config{RootEmail: "root@example.com"}
	root := &domain.User{ID: "root-1", Email: "root@example.com", Name: "Root", Type: domain.UserTypeAPIKey}
	owner := &domain.UserWorkspace{UserID: "root-1", WorkspaceID: "demo", Role: "owner"}

	// --- user service (real, over a mock user repo) ---
	userRepo := mocks.NewMockUserRepository(ctrl)
	userRepo.EXPECT().GetUserByEmail(gomock.Any(), "root@example.com").Return(root, nil).Times(1)
	userService, err := NewUserService(UserServiceConfig{Repository: userRepo, Logger: log})
	require.NoError(t, err)

	// --- auth (shared by every real service below) ---
	authService := mocks.NewMockAuthService(ctrl)
	authService.EXPECT().AuthenticateUserFromContext(gomock.Any()).Return(root, nil).Times(1)
	authService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), gomock.Any()).
		Return(ctx, root, owner, nil).AnyTimes()

	// --- workspace repository (stateful: serves back what Create stored) ---
	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	var stored *domain.Workspace
	workspaceRepo.EXPECT().List(gomock.Any()).
		Return([]*domain.Workspace{{ID: "oldws"}}, nil).Times(1)
	workspaceRepo.EXPECT().Delete(gomock.Any(), "oldws").Return(nil).Times(1)
	workspaceRepo.EXPECT().GetByID(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string) (*domain.Workspace, error) {
			if stored == nil {
				return nil, errors.New("not found")
			}
			return stored, nil
		}).AnyTimes()
	workspaceRepo.EXPECT().Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, ws *domain.Workspace) error {
			stored = ws
			return nil
		}).Times(1)
	workspaceRepo.EXPECT().AddUserToWorkspace(gomock.Any(), gomock.Any()).Return(nil).Times(1)
	workspaceRepo.EXPECT().Update(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, ws *domain.Workspace) error {
			stored = ws
			return nil
		}).AnyTimes()

	// --- task repository: permanent tasks already exist and are healthy ---
	taskRepo := mocks.NewMockTaskRepository(ctrl)
	taskRepo.EXPECT().DeleteAll(gomock.Any(), "oldws").Return(nil).Times(1)
	past := time.Now().Add(-time.Hour)
	taskRepo.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).
		Return([]*domain.Task{{Status: domain.TaskStatusPending, NextRunAfter: &past}}, 1, nil).AnyTimes()
	taskRepo.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	taskRepo.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	// --- workspace service (real) ---
	userServiceIface := mocks.NewMockUserServiceInterface(ctrl)
	userServiceIface.EXPECT().GetUserByID(gomock.Any(), "root-1").Return(root, nil).Times(1)
	ownerContact := mocks.NewMockContactService(ctrl)
	ownerContact.EXPECT().UpsertContact(gomock.Any(), "demo", gomock.Any()).
		Return(domain.UpsertContactOperation{Action: domain.UpsertContactOperationCreate}).Times(1)
	wsListService := mocks.NewMockListService(ctrl)
	wsListService.EXPECT().CreateList(gomock.Any(), "demo", gomock.Any()).Return(nil).Times(1)
	wsListService.EXPECT().SubscribeToLists(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)

	workspaceService := NewWorkspaceService(
		workspaceRepo,
		mocks.NewMockUserRepository(ctrl),
		taskRepo,
		log,
		userServiceIface,
		authService,
		pkgmocks.NewMockMailer(ctrl),
		cfg,
		ownerContact,
		wsListService,
		mocks.NewMockContactListService(ctrl),
		mocks.NewMockTemplateService(ctrl),
		mocks.NewMockWebhookRegistrationService(ctrl),
		"secret_key",
		&SupabaseService{},
		&DNSVerificationService{},
		&BlogService{},
		nil,
	)

	// --- template service (real): sample templates are created ---
	templateRepo := mocks.NewMockTemplateRepository(ctrl)
	templateRepo.EXPECT().CreateTemplate(gomock.Any(), "demo", gomock.Any()).Return(nil).AnyTimes()
	templateService := NewTemplateService(templateRepo, workspaceRepo, authService, log, "", nil)

	// --- list service (real): the newsletter list creation fails, which
	// aborts addSampleData — a degradation the reset must survive. ---
	listRepo := mocks.NewMockListRepository(ctrl)
	listRepo.EXPECT().CreateList(gomock.Any(), "demo", gomock.Any()).
		Return(errors.New("lists table unavailable")).Times(1)
	listService := NewListService(
		listRepo, workspaceRepo,
		mocks.NewMockContactListRepository(ctrl),
		mocks.NewMockContactRepository(ctrl),
		mocks.NewMockMessageHistoryRepository(ctrl),
		authService, nil, log, "", cache.NewInMemoryCache(time.Hour),
	)

	// --- webhook subscription service (real) ---
	subscriptionRepo := mocks.NewMockWebhookSubscriptionRepository(ctrl)
	var subscription *domain.WebhookSubscription
	subscriptionRepo.EXPECT().Create(gomock.Any(), "demo", gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, sub *domain.WebhookSubscription) error {
			subscription = sub
			return nil
		}).Times(1)
	subscriptionRepo.EXPECT().GetByID(gomock.Any(), "demo", gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ string) (*domain.WebhookSubscription, error) {
			return subscription, nil
		}).AnyTimes()
	subscriptionRepo.EXPECT().Update(gomock.Any(), "demo", gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, sub *domain.WebhookSubscription) error {
			subscription = sub
			return nil
		}).Times(1)
	webhookSubscriptionService := NewWebhookSubscriptionService(
		subscriptionRepo, nil, authService, log, false)

	svc := &DemoService{
		logger:                     log,
		config:                     cfg,
		workspaceService:           workspaceService,
		userService:                userService,
		templateService:            templateService,
		listService:                listService,
		workspaceRepo:              workspaceRepo,
		taskRepo:                   taskRepo,
		webhookSubscriptionService: webhookSubscriptionService,
		authService:                authService,
	}

	require.NoError(t, svc.ResetDemo(ctx),
		"a sample-data failure degrades the demo, it must not fail the reset")

	require.NotNil(t, stored, "the demo workspace was created")
	assert.Equal(t, "demo", stored.ID)
	require.Len(t, stored.Integrations, 1, "the SMTP integration was added")
	assert.Equal(t, stored.Integrations[0].ID, stored.Settings.TransactionalEmailProviderID,
		"the demo integration sends transactional mail")
	assert.Equal(t, stored.Integrations[0].ID, stored.Settings.MarketingEmailProviderID,
		"and marketing mail")

	require.NotNil(t, subscription, "the demo webhook subscription was created")
	assert.False(t, subscription.Enabled, "and immediately disabled so it cannot firehose")
}
