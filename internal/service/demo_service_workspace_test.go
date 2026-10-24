package service

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// createDemoWorkspace drives the real UserService/WorkspaceService over mock
// repositories: the demo reset's first two failure modes (no root user,
// workspace creation refused) must abort with the step named, before any
// seeding happens.

func demoGraph(t *testing.T) (*DemoService, *mocks.MockUserRepository, *mocks.MockWorkspaceRepository, *mocks.MockAuthService) {
	t.Helper()
	ctrl := gomock.NewController(t)
	log := logger.NewLoggerWithLevel("disabled")

	userRepo := mocks.NewMockUserRepository(ctrl)
	userService, err := NewUserService(UserServiceConfig{Repository: userRepo, Logger: log})
	require.NoError(t, err)

	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	authService := mocks.NewMockAuthService(ctrl)
	cfg := &config.Config{RootEmail: "root@example.com"}
	userServiceIface := mocks.NewMockUserServiceInterface(ctrl)
	contactService := mocks.NewMockContactService(ctrl)
	workspaceService := NewWorkspaceService(
		workspaceRepo, nil, nil, log, userServiceIface, authService, nil, cfg,
		contactService, nil, nil, nil, nil, "", nil, nil, nil, nil,
	)

	svc := &DemoService{
		logger:           log,
		config:           cfg,
		userService:      userService,
		workspaceService: workspaceService,
	}
	return svc, userRepo, workspaceRepo, authService
}

func TestDemoService_CreateDemoWorkspace_RootUserMissing(t *testing.T) {
	svc, userRepo, _, _ := demoGraph(t)
	sentinel := errors.New("no such user")
	userRepo.EXPECT().GetUserByEmail(gomock.Any(), "root@example.com").Return(nil, sentinel).Times(1)

	err := svc.createDemoWorkspace(context.Background())
	require.ErrorContains(t, err, "failed to get root user with email 'root@example.com':")
	assert.ErrorIs(t, err, sentinel)
}

func TestDemoService_CreateDemoWorkspace_CreateRefused(t *testing.T) {
	svc, userRepo, _, authService := demoGraph(t)
	// An API-key root user skips the session dance in rootContext.
	userRepo.EXPECT().GetUserByEmail(gomock.Any(), "root@example.com").
		Return(&domain.User{ID: "root-1", Email: "root@example.com", Type: domain.UserTypeAPIKey}, nil).Times(1)
	sentinel := errors.New("no authenticated user")
	authService.EXPECT().AuthenticateUserFromContext(gomock.Any()).Return(nil, sentinel).Times(1)

	err := svc.createDemoWorkspace(context.Background())
	require.ErrorContains(t, err, "failed to create demo workspace:")
	assert.ErrorIs(t, err, sentinel)
}

func TestDemoService_CreateDemoSMTPIntegration_AuthFailure(t *testing.T) {
	svc, _, _, authService := demoGraph(t)
	sentinel := errors.New("session expired")
	authService.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "ws-1").
		Return(context.Background(), nil, nil, sentinel).Times(1)

	err := svc.createDemoSMTPIntegration(context.Background(), "ws-1")
	require.ErrorContains(t, err, "failed to create SMTP integration:")
	require.ErrorContains(t, err, "failed to authenticate user:")
	assert.ErrorIs(t, err, sentinel)
}

func TestDemoService_CreateDemoWorkspace_CreateFails(t *testing.T) {
	svc, userRepo, workspaceRepo, authService := demoGraph(t)
	root := &domain.User{ID: "root-1", Email: "root@example.com", Type: domain.UserTypeAPIKey}
	userRepo.EXPECT().GetUserByEmail(gomock.Any(), "root@example.com").Return(root, nil).Times(1)
	authService.EXPECT().AuthenticateUserFromContext(gomock.Any()).Return(root, nil).Times(1)
	workspaceRepo.EXPECT().GetByID(gomock.Any(), "demo").
		Return(nil, errors.New("not found")).Times(1)
	sentinel := errors.New("insert failed")
	workspaceRepo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(sentinel).Times(1)

	err := svc.createDemoWorkspace(context.Background())
	require.ErrorContains(t, err, "failed to create demo workspace:")
	assert.ErrorIs(t, err, sentinel)
}

func TestDemoService_CreateDemoWorkspace_AlreadyExists(t *testing.T) {
	svc, userRepo, workspaceRepo, authService := demoGraph(t)
	root := &domain.User{ID: "root-1", Email: "root@example.com", Type: domain.UserTypeAPIKey}
	userRepo.EXPECT().GetUserByEmail(gomock.Any(), "root@example.com").Return(root, nil).Times(1)
	authService.EXPECT().AuthenticateUserFromContext(gomock.Any()).Return(root, nil).Times(1)
	workspaceRepo.EXPECT().GetByID(gomock.Any(), "demo").
		Return(&domain.Workspace{ID: "demo"}, nil).Times(1)

	err := svc.createDemoWorkspace(context.Background())
	require.ErrorContains(t, err, "failed to create demo workspace:")
	require.ErrorContains(t, err, "workspace already exists")
}
