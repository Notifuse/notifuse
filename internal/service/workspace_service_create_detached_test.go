package service

import (
	"context"
	"fmt"
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

// liveCtxMatcher matches a context that has not ended, whatever the request's context
// became: the one repo.Create gets, which the repository bounds itself.
type liveCtxMatcher struct{}

func liveCtx() gomock.Matcher { return liveCtxMatcher{} }

func (liveCtxMatcher) Matches(x interface{}) bool {
	ctx, ok := x.(context.Context)
	return ok && ctx.Err() == nil
}

func (liveCtxMatcher) String() string { return "is a live context detached from the request" }

// detachedCtxMatcher matches the context CreateWorkspace uses after repo.Create: not
// ended, whatever the request's context became, and bounded by workspaceSetupTimeout.
type detachedCtxMatcher struct{}

func detachedCtx() gomock.Matcher { return detachedCtxMatcher{} }

func (detachedCtxMatcher) Matches(x interface{}) bool {
	ctx, ok := x.(context.Context)
	if !ok || ctx.Err() != nil {
		return false
	}
	deadline, ok := ctx.Deadline()
	return ok && time.Until(deadline) > workspaceSetupTimeout-time.Minute && time.Until(deadline) <= workspaceSetupTimeout
}

func (detachedCtxMatcher) String() string {
	return fmt.Sprintf("is a live context detached from the request, bounded by %s", workspaceSetupTimeout)
}

// A client disconnecting while the workspace is created must not leave it without its
// owner membership, contact or list: everything from repo.Create on runs detached.
func TestWorkspaceService_CreateWorkspace_DetachedFromTheRequest(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockUserRepo := mocks.NewMockUserRepository(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockUserService := mocks.NewMockUserServiceInterface(ctrl)
	mockContactService := mocks.NewMockContactService(ctrl)
	mockListService := mocks.NewMockListService(ctrl)
	mockTaskRepo := mocks.NewMockTaskRepository(ctrl)

	service := newWorkspaceServiceForCreateTest(ctrl, mockRepo, mockUserRepo, mockTaskRepo, mockLogger, mockUserService, mockAuthService, mockContactService, mockListService)

	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	user := &domain.User{ID: "owner", Email: "owner@example.com", Name: "Owner"}
	mockAuthService.EXPECT().AuthenticateUserFromContext(ctx).Return(user, nil)
	mockRepo.EXPECT().GetByID(ctx, "acme").Return(nil, nil)
	mockRepo.EXPECT().Create(liveCtx(), gomock.Any()).Return(nil)
	mockRepo.EXPECT().AddUserToWorkspace(detachedCtx(), gomock.Any()).Return(nil)
	mockUserService.EXPECT().GetUserByID(detachedCtx(), "owner").Return(user, nil)
	mockContactService.EXPECT().UpsertContact(detachedCtx(), "acme", gomock.Any()).Return(domain.UpsertContactOperation{Action: domain.UpsertContactOperationCreate})
	mockListService.EXPECT().CreateList(detachedCtx(), "acme", gomock.Any()).Return(nil)
	mockListService.EXPECT().SubscribeToLists(detachedCtx(), gomock.Any(), true).Return(nil)
	mockTaskRepo.EXPECT().List(detachedCtx(), "acme", gomock.Any()).Return([]*domain.Task{}, 0, nil).AnyTimes()
	mockTaskRepo.EXPECT().Create(detachedCtx(), "acme", gomock.Any()).Return(nil).AnyTimes()

	workspace, err := service.CreateWorkspace(ctx, "acme", "Acme", "", "", "", "UTC", domain.FileManagerSettings{}, "en", []string{"en"})
	require.NoError(t, err)
	assert.Equal(t, "acme", workspace.ID)
}

func TestWorkspaceService_CreateWorkspace_AlreadyExistsIsTheDomainError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	service := newWorkspaceServiceForCreateTest(ctrl, mockRepo, mocks.NewMockUserRepository(ctrl), mocks.NewMockTaskRepository(ctrl), mockLogger, mocks.NewMockUserServiceInterface(ctrl), mockAuthService, mocks.NewMockContactService(ctrl), mocks.NewMockListService(ctrl))

	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	user := &domain.User{ID: "owner", Email: "owner@example.com"}
	mockAuthService.EXPECT().AuthenticateUserFromContext(gomock.Any()).Return(user, nil)
	mockRepo.EXPECT().GetByID(gomock.Any(), "acme").Return(&domain.Workspace{ID: "acme"}, nil)

	_, err := service.CreateWorkspace(context.Background(), "acme", "Acme", "", "", "", "UTC", domain.FileManagerSettings{}, "en", []string{"en"})
	assert.ErrorIs(t, err, domain.ErrWorkspaceAlreadyExists)
	assert.EqualError(t, err, "workspace already exists")
}

func newWorkspaceServiceForCreateTest(ctrl *gomock.Controller, repo *mocks.MockWorkspaceRepository, userRepo *mocks.MockUserRepository, taskRepo *mocks.MockTaskRepository, logger *pkgmocks.MockLogger, userService *mocks.MockUserServiceInterface, authService *mocks.MockAuthService, contactService *mocks.MockContactService, listService *mocks.MockListService) *WorkspaceService {
	return NewWorkspaceService(
		repo,
		userRepo,
		taskRepo,
		logger,
		userService,
		authService,
		pkgmocks.NewMockMailer(ctrl),
		&config.Config{RootEmail: "owner@example.com"},
		contactService,
		listService,
		mocks.NewMockContactListService(ctrl),
		mocks.NewMockTemplateService(ctrl),
		mocks.NewMockWebhookRegistrationService(ctrl),
		"secret_key",
		&SupabaseService{},
		&DNSVerificationService{},
		&BlogService{},
		fullyLicensedProvider(ctrl),
	)
}
