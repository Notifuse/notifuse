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

func TestWorkspaceService_AddUserToWorkspace(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockUserSvc := mocks.NewMockUserServiceInterface(ctrl)
	mockAuthSvc := mocks.NewMockAuthService(ctrl)
	mockMailer := pkgmocks.NewMockMailer(ctrl)
	cfg := &config.Config{}

	service := NewWorkspaceService(mockRepo, mockLogger, mockUserSvc, mockAuthSvc, mockMailer, cfg)

	ctx := context.Background()
	workspaceID := "workspace1"
	userID := "user1"
	requesterID := "requester1"

	// Setup common logger expectations
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	tests := []struct {
		name          string
		userID        string
		workspaceID   string
		role          string
		setupMock     func()
		expectedError error
	}{
		{
			name:        "successful add user to workspace",
			userID:      userID,
			workspaceID: workspaceID,
			role:        "member",
			setupMock: func() {
				mockAuthSvc.EXPECT().
					AuthenticateUserForWorkspace(ctx, workspaceID).
					Return(&domain.User{ID: requesterID}, nil)

				mockRepo.EXPECT().
					GetUserWorkspace(ctx, requesterID, workspaceID).
					Return(&domain.UserWorkspace{
						UserID:      requesterID,
						WorkspaceID: workspaceID,
						Role:        "owner",
					}, nil)

				mockRepo.EXPECT().
					AddUserToWorkspace(gomock.Any(), gomock.Any()).
					Return(nil)
			},
			expectedError: nil,
		},
		{
			name:        "authentication error",
			userID:      userID,
			workspaceID: workspaceID,
			role:        "member",
			setupMock: func() {
				mockAuthSvc.EXPECT().
					AuthenticateUserForWorkspace(ctx, workspaceID).
					Return(nil, fmt.Errorf("authentication failed"))
			},
			expectedError: fmt.Errorf("failed to authenticate user: authentication failed"),
		},
		{
			name:        "requester not found in workspace",
			userID:      userID,
			workspaceID: workspaceID,
			role:        "member",
			setupMock: func() {
				mockAuthSvc.EXPECT().
					AuthenticateUserForWorkspace(ctx, workspaceID).
					Return(&domain.User{ID: requesterID}, nil)

				mockRepo.EXPECT().
					GetUserWorkspace(ctx, requesterID, workspaceID).
					Return(nil, fmt.Errorf("user workspace not found"))
			},
			expectedError: fmt.Errorf("user workspace not found"),
		},
		{
			name:        "requester not an owner",
			userID:      userID,
			workspaceID: workspaceID,
			role:        "member",
			setupMock: func() {
				mockAuthSvc.EXPECT().
					AuthenticateUserForWorkspace(ctx, workspaceID).
					Return(&domain.User{ID: requesterID}, nil)

				mockRepo.EXPECT().
					GetUserWorkspace(ctx, requesterID, workspaceID).
					Return(&domain.UserWorkspace{
						UserID:      requesterID,
						WorkspaceID: workspaceID,
						Role:        "member",
					}, nil)
			},
			expectedError: &domain.ErrUnauthorized{Message: "user is not an owner of the workspace"},
		},
		{
			name:        "invalid role",
			userID:      userID,
			workspaceID: workspaceID,
			role:        "invalid_role",
			setupMock: func() {
				mockAuthSvc.EXPECT().
					AuthenticateUserForWorkspace(ctx, workspaceID).
					Return(&domain.User{ID: requesterID}, nil)

				mockRepo.EXPECT().
					GetUserWorkspace(ctx, requesterID, workspaceID).
					Return(&domain.UserWorkspace{
						UserID:      requesterID,
						WorkspaceID: workspaceID,
						Role:        "owner",
					}, nil)
			},
			expectedError: fmt.Errorf("invalid user workspace: role: invalid_role does not validate as in(owner|member)"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			err := service.AddUserToWorkspace(ctx, tt.workspaceID, tt.userID, tt.role)

			if tt.expectedError != nil {
				assert.Error(t, err)
				if _, ok := tt.expectedError.(*domain.ErrUnauthorized); ok {
					assert.IsType(t, &domain.ErrUnauthorized{}, err)
				} else {
					assert.Equal(t, tt.expectedError.Error(), err.Error())
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestWorkspaceService_RemoveUserFromWorkspace(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockUserSvc := mocks.NewMockUserServiceInterface(ctrl)
	mockAuthSvc := mocks.NewMockAuthService(ctrl)
	mockMailer := pkgmocks.NewMockMailer(ctrl)
	cfg := &config.Config{}

	service := NewWorkspaceService(mockRepo, mockLogger, mockUserSvc, mockAuthSvc, mockMailer, cfg)

	// Setup common logger expectations
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	ctx := context.Background()
	workspaceID := "workspace1"
	userID := "user1"
	requesterID := "requester1"

	t.Run("successful_remove_user_from_workspace", func(t *testing.T) {
		// Set up mock expectations
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(&domain.User{ID: requesterID}, nil)

		mockRepo.EXPECT().
			GetUserWorkspace(ctx, requesterID, workspaceID).
			Return(&domain.UserWorkspace{
				UserID:      requesterID,
				WorkspaceID: workspaceID,
				Role:        "owner",
			}, nil)

		mockRepo.EXPECT().
			RemoveUserFromWorkspace(ctx, userID, workspaceID).
			Return(nil)

		err := service.RemoveUserFromWorkspace(ctx, workspaceID, userID)
		require.NoError(t, err)
	})

	t.Run("authentication_error", func(t *testing.T) {
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(nil, fmt.Errorf("authentication failed"))

		err := service.RemoveUserFromWorkspace(ctx, workspaceID, userID)
		require.Error(t, err)
		assert.Equal(t, "failed to authenticate user: authentication failed", err.Error())
	})

	t.Run("requester_not_found_in_workspace", func(t *testing.T) {
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(&domain.User{ID: requesterID}, nil)

		mockRepo.EXPECT().
			GetUserWorkspace(ctx, requesterID, workspaceID).
			Return(nil, fmt.Errorf("user is not a member of the workspace"))

		err := service.RemoveUserFromWorkspace(ctx, workspaceID, userID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "user is not a member of the workspace")
	})

	t.Run("requester_not_an_owner", func(t *testing.T) {
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(&domain.User{ID: requesterID}, nil)

		mockRepo.EXPECT().
			GetUserWorkspace(ctx, requesterID, workspaceID).
			Return(&domain.UserWorkspace{
				UserID:      requesterID,
				WorkspaceID: workspaceID,
				Role:        "member",
			}, nil)

		err := service.RemoveUserFromWorkspace(ctx, workspaceID, userID)
		require.Error(t, err)
		assert.Equal(t, "user is not an owner of the workspace", err.Error())
	})

	t.Run("target_user_not_found", func(t *testing.T) {
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(&domain.User{ID: requesterID}, nil)

		mockRepo.EXPECT().
			GetUserWorkspace(ctx, requesterID, workspaceID).
			Return(&domain.UserWorkspace{
				UserID:      requesterID,
				WorkspaceID: workspaceID,
				Role:        "owner",
			}, nil)

		mockRepo.EXPECT().
			RemoveUserFromWorkspace(ctx, userID, workspaceID).
			Return(fmt.Errorf("user is not a member of the workspace"))

		err := service.RemoveUserFromWorkspace(ctx, workspaceID, userID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "user is not a member of the workspace")
	})

	t.Run("cannot_remove_owner", func(t *testing.T) {
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(&domain.User{ID: requesterID}, nil)

		mockRepo.EXPECT().
			GetUserWorkspace(ctx, requesterID, workspaceID).
			Return(&domain.UserWorkspace{
				UserID:      requesterID,
				WorkspaceID: workspaceID,
				Role:        "owner",
			}, nil)

		err := service.RemoveUserFromWorkspace(ctx, workspaceID, requesterID)
		require.Error(t, err)
		assert.Equal(t, "cannot remove yourself from the workspace", err.Error())
	})

	t.Run("cannot remove self", func(t *testing.T) {
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(&domain.User{ID: requesterID}, nil)

		mockRepo.EXPECT().
			GetUserWorkspace(ctx, requesterID, workspaceID).
			Return(&domain.UserWorkspace{
				UserID:      requesterID,
				WorkspaceID: workspaceID,
				Role:        "owner",
			}, nil)

		err := service.RemoveUserFromWorkspace(ctx, workspaceID, requesterID)
		require.Error(t, err)
		assert.Equal(t, "cannot remove yourself from the workspace", err.Error())
	})
}

func TestWorkspaceService_TransferOwnership(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockUserSvc := mocks.NewMockUserServiceInterface(ctrl)
	mockAuthSvc := mocks.NewMockAuthService(ctrl)
	mockMailer := pkgmocks.NewMockMailer(ctrl)
	cfg := &config.Config{}

	service := NewWorkspaceService(mockRepo, mockLogger, mockUserSvc, mockAuthSvc, mockMailer, cfg)

	ctx := context.Background()
	workspaceID := "workspace1"
	userID := "user1"
	requesterID := "requester1"

	// Setup common logger expectations
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	t.Run("successful transfer ownership", func(t *testing.T) {
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(&domain.User{ID: requesterID}, nil)

		mockRepo.EXPECT().
			GetUserWorkspace(ctx, requesterID, workspaceID).
			Return(&domain.UserWorkspace{
				UserID:      requesterID,
				WorkspaceID: workspaceID,
				Role:        "owner",
			}, nil)

		mockRepo.EXPECT().
			GetUserWorkspace(ctx, userID, workspaceID).
			Return(&domain.UserWorkspace{
				UserID:      userID,
				WorkspaceID: workspaceID,
				Role:        "member",
			}, nil)

		mockRepo.EXPECT().
			AddUserToWorkspace(ctx, gomock.Any()).
			DoAndReturn(func(_ context.Context, uw *domain.UserWorkspace) error {
				assert.Equal(t, userID, uw.UserID)
				assert.Equal(t, workspaceID, uw.WorkspaceID)
				assert.Equal(t, "owner", uw.Role)
				return nil
			})

		mockRepo.EXPECT().
			AddUserToWorkspace(ctx, gomock.Any()).
			DoAndReturn(func(_ context.Context, uw *domain.UserWorkspace) error {
				assert.Equal(t, requesterID, uw.UserID)
				assert.Equal(t, workspaceID, uw.WorkspaceID)
				assert.Equal(t, "member", uw.Role)
				return nil
			})

		err := service.TransferOwnership(ctx, workspaceID, userID, requesterID)
		require.NoError(t, err)
	})

	t.Run("authentication error", func(t *testing.T) {
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(nil, fmt.Errorf("authentication failed"))

		err := service.TransferOwnership(ctx, workspaceID, userID, requesterID)
		require.Error(t, err)
		assert.Equal(t, "failed to authenticate user: authentication failed", err.Error())
	})

	t.Run("requester not found in workspace", func(t *testing.T) {
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(&domain.User{ID: requesterID}, nil)

		mockRepo.EXPECT().
			GetUserWorkspace(ctx, requesterID, workspaceID).
			Return(nil, fmt.Errorf("user workspace not found"))

		err := service.TransferOwnership(ctx, workspaceID, userID, requesterID)
		require.Error(t, err)
		assert.Equal(t, "user workspace not found", err.Error())
	})

	t.Run("requester not an owner", func(t *testing.T) {
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(&domain.User{ID: requesterID}, nil)

		mockRepo.EXPECT().
			GetUserWorkspace(ctx, requesterID, workspaceID).
			Return(&domain.UserWorkspace{
				UserID:      requesterID,
				WorkspaceID: workspaceID,
				Role:        "member",
			}, nil)

		err := service.TransferOwnership(ctx, workspaceID, userID, requesterID)
		require.Error(t, err)
		assert.IsType(t, &domain.ErrUnauthorized{}, err)
		assert.Equal(t, "user is not an owner of the workspace", err.Error())
	})

	t.Run("target user not found in workspace", func(t *testing.T) {
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(&domain.User{ID: requesterID}, nil)

		mockRepo.EXPECT().
			GetUserWorkspace(ctx, requesterID, workspaceID).
			Return(&domain.UserWorkspace{
				UserID:      requesterID,
				WorkspaceID: workspaceID,
				Role:        "owner",
			}, nil)

		mockRepo.EXPECT().
			GetUserWorkspace(ctx, userID, workspaceID).
			Return(nil, fmt.Errorf("user workspace not found"))

		err := service.TransferOwnership(ctx, workspaceID, userID, requesterID)
		require.Error(t, err)
		assert.Equal(t, "user workspace not found", err.Error())
	})

	t.Run("target user is already an owner", func(t *testing.T) {
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(&domain.User{ID: requesterID}, nil)

		mockRepo.EXPECT().
			GetUserWorkspace(ctx, requesterID, workspaceID).
			Return(&domain.UserWorkspace{
				UserID:      requesterID,
				WorkspaceID: workspaceID,
				Role:        "owner",
			}, nil)

		mockRepo.EXPECT().
			GetUserWorkspace(ctx, userID, workspaceID).
			Return(&domain.UserWorkspace{
				UserID:      userID,
				WorkspaceID: workspaceID,
				Role:        "owner",
			}, nil)

		err := service.TransferOwnership(ctx, workspaceID, userID, requesterID)
		require.Error(t, err)
		assert.Equal(t, "new owner must be a current member of the workspace", err.Error())
	})
}

func TestWorkspaceService_GetWorkspaceMembersWithEmail(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)
	mockUserSvc := mocks.NewMockUserServiceInterface(ctrl)
	mockAuthSvc := mocks.NewMockAuthService(ctrl)
	mockMailer := pkgmocks.NewMockMailer(ctrl)
	cfg := &config.Config{}

	service := NewWorkspaceService(mockRepo, mockLogger, mockUserSvc, mockAuthSvc, mockMailer, cfg)

	ctx := context.Background()
	workspaceID := "workspace1"
	userID := "user1"
	now := time.Now()

	// Setup common logger expectations
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	t.Run("successful get members with email", func(t *testing.T) {
		expectedUser := &domain.User{
			ID: userID,
		}

		expectedMembers := []*domain.UserWorkspaceWithEmail{
			{
				UserWorkspace: domain.UserWorkspace{
					UserID:      "user1",
					WorkspaceID: workspaceID,
					Role:        "owner",
					CreatedAt:   now,
					UpdatedAt:   now,
				},
				Email: "user1@example.com",
			},
			{
				UserWorkspace: domain.UserWorkspace{
					UserID:      "user2",
					WorkspaceID: workspaceID,
					Role:        "member",
					CreatedAt:   now,
					UpdatedAt:   now,
				},
				Email: "user2@example.com",
			},
		}

		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(expectedUser, nil)

		mockRepo.EXPECT().
			GetUserWorkspace(ctx, userID, workspaceID).
			Return(&domain.UserWorkspace{
				UserID:      userID,
				WorkspaceID: workspaceID,
				Role:        "owner",
			}, nil)

		mockRepo.EXPECT().
			GetWorkspaceUsersWithEmail(ctx, workspaceID).
			Return(expectedMembers, nil)

		members, err := service.GetWorkspaceMembersWithEmail(ctx, workspaceID)
		require.NoError(t, err)
		assert.Equal(t, expectedMembers, members)
	})

	t.Run("authentication error", func(t *testing.T) {
		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(nil, fmt.Errorf("authentication failed"))

		members, err := service.GetWorkspaceMembersWithEmail(ctx, workspaceID)
		require.Error(t, err)
		assert.Nil(t, members)
		assert.Equal(t, "failed to authenticate user: authentication failed", err.Error())
	})

	t.Run("repository error", func(t *testing.T) {
		expectedUser := &domain.User{
			ID: userID,
		}

		mockAuthSvc.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(expectedUser, nil)

		mockRepo.EXPECT().
			GetUserWorkspace(ctx, userID, workspaceID).
			Return(&domain.UserWorkspace{
				UserID:      userID,
				WorkspaceID: workspaceID,
				Role:        "owner",
			}, nil)

		mockRepo.EXPECT().
			GetWorkspaceUsersWithEmail(ctx, workspaceID).
			Return(nil, fmt.Errorf("database error"))

		members, err := service.GetWorkspaceMembersWithEmail(ctx, workspaceID)
		require.Error(t, err)
		assert.Nil(t, members)
		assert.Equal(t, "database error", err.Error())
	})
}
