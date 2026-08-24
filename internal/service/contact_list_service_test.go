package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	pkgmocks "github.com/Notifuse/notifuse/pkg/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

func setupTest(t *testing.T) (
	*mocks.MockContactListRepository,
	*mocks.MockAuthService,
	*mocks.MockContactRepository,
	*mocks.MockListRepository,
	*ContactListService,
	*gomock.Controller,
) {
	ctrl := gomock.NewController(t)
	mockRepo := mocks.NewMockContactListRepository(ctrl)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockListRepo := mocks.NewMockListRepository(ctrl)
	mockContactListRepo := mocks.NewMockContactListRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Set up logger expectations
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	service := NewContactListService(mockRepo, mockWorkspaceRepo, mockAuthService, mockContactRepo, mockListRepo, mockContactListRepo, mockLogger)

	return mockRepo, mockAuthService, mockContactRepo, mockListRepo, service, ctrl
}

// contactListFullAccess builds a member row — role "member", not "owner", so
// HasPermission actually consults the grants — holding read+write on every
// resource, for the cases that exercise the code past the permission gates.
func contactListFullAccess() *domain.UserWorkspace {
	return &domain.UserWorkspace{
		UserID:      "user1",
		WorkspaceID: "workspace123",
		Role:        "member",
		Permissions: domain.NewFullPermissions(),
	}
}

func TestContactListService_GetContactListByIDs(t *testing.T) {
	mockRepo, mockAuthService, _, _, service, ctrl := setupTest(t)
	defer ctrl.Finish()

	ctx := context.Background()
	workspaceID := "workspace123"
	email := "test@example.com"
	listID := "list123"

	t.Run("successful retrieval", func(t *testing.T) {
		expectedContactList := &domain.ContactList{
			Email:  email,
			ListID: listID,
			Status: domain.ContactListStatusActive,
		}

		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, &domain.User{}, contactListFullAccess(), nil)

		mockRepo.EXPECT().
			GetContactListByIDs(gomock.Any(), workspaceID, email, listID).
			Return(expectedContactList, nil)

		result, err := service.GetContactListByIDs(ctx, workspaceID, email, listID)
		require.NoError(t, err)
		require.Equal(t, expectedContactList, result)
	})

	t.Run("authentication error", func(t *testing.T) {
		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, nil, nil, errors.New("auth error"))

		result, err := service.GetContactListByIDs(ctx, workspaceID, email, listID)
		require.Error(t, err)
		require.Nil(t, result)
	})

	t.Run("not found error", func(t *testing.T) {
		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, &domain.User{}, contactListFullAccess(), nil)

		mockRepo.EXPECT().
			GetContactListByIDs(gomock.Any(), workspaceID, email, listID).
			Return(nil, &domain.ErrContactListNotFound{Message: "not found"})

		result, err := service.GetContactListByIDs(ctx, workspaceID, email, listID)
		require.Error(t, err)
		require.Nil(t, result)
	})
}

func TestContactListService_GetContactsByListID(t *testing.T) {
	mockRepo, mockAuthService, _, mockListRepo, service, ctrl := setupTest(t)
	defer ctrl.Finish()

	ctx := context.Background()
	workspaceID := "workspace123"
	listID := "list123"

	t.Run("successful retrieval", func(t *testing.T) {
		expectedContacts := []*domain.ContactList{
			{
				Email:  "test1@example.com",
				ListID: listID,
				Status: domain.ContactListStatusActive,
			},
			{
				Email:  "test2@example.com",
				ListID: listID,
				Status: domain.ContactListStatusActive,
			},
		}

		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, &domain.User{}, contactListFullAccess(), nil)

		mockListRepo.EXPECT().
			GetListByID(gomock.Any(), workspaceID, listID).
			Return(&domain.List{ID: listID}, nil)

		mockRepo.EXPECT().
			GetContactsByListID(gomock.Any(), workspaceID, listID).
			Return(expectedContacts, nil)

		result, err := service.GetContactsByListID(ctx, workspaceID, listID)
		require.NoError(t, err)
		require.Equal(t, expectedContacts, result)
	})

	t.Run("authentication error", func(t *testing.T) {
		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, nil, nil, errors.New("auth error"))

		result, err := service.GetContactsByListID(ctx, workspaceID, listID)
		require.Error(t, err)
		require.Nil(t, result)
	})

	t.Run("list not found", func(t *testing.T) {
		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, &domain.User{}, contactListFullAccess(), nil)

		mockListRepo.EXPECT().
			GetListByID(gomock.Any(), workspaceID, listID).
			Return(nil, errors.New("not found"))

		result, err := service.GetContactsByListID(ctx, workspaceID, listID)
		require.Error(t, err)
		require.Nil(t, result)
	})
}

func TestContactListService_GetListsByEmail(t *testing.T) {
	mockRepo, mockAuthService, mockContactRepo, _, service, ctrl := setupTest(t)
	defer ctrl.Finish()

	ctx := context.Background()
	workspaceID := "workspace123"
	email := "test@example.com"

	t.Run("successful retrieval", func(t *testing.T) {
		expectedLists := []*domain.ContactList{
			{
				Email:  email,
				ListID: "list1",
				Status: domain.ContactListStatusActive,
			},
			{
				Email:  email,
				ListID: "list2",
				Status: domain.ContactListStatusActive,
			},
		}

		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, &domain.User{}, contactListFullAccess(), nil)

		mockContactRepo.EXPECT().
			GetContactByEmail(gomock.Any(), workspaceID, email).
			Return(&domain.Contact{Email: email}, nil)

		mockRepo.EXPECT().
			GetListsByEmail(gomock.Any(), workspaceID, email).
			Return(expectedLists, nil)

		result, err := service.GetListsByEmail(ctx, workspaceID, email)
		require.NoError(t, err)
		require.Equal(t, expectedLists, result)
	})

	t.Run("authentication error", func(t *testing.T) {
		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, nil, nil, errors.New("auth error"))

		result, err := service.GetListsByEmail(ctx, workspaceID, email)
		require.Error(t, err)
		require.Nil(t, result)
	})

	t.Run("contact not found", func(t *testing.T) {
		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, &domain.User{}, contactListFullAccess(), nil)

		mockContactRepo.EXPECT().
			GetContactByEmail(gomock.Any(), workspaceID, email).
			Return(nil, errors.New("not found"))

		result, err := service.GetListsByEmail(ctx, workspaceID, email)
		require.Error(t, err)
		require.Nil(t, result)
	})
}

func TestContactListService_UpdateContactListStatus(t *testing.T) {
	mockRepo, mockAuthService, _, _, service, ctrl := setupTest(t)
	defer ctrl.Finish()

	ctx := context.Background()
	workspaceID := "workspace123"
	email := "test@example.com"
	listID := "list123"
	newStatus := domain.ContactListStatusUnsubscribed

	t.Run("successful update", func(t *testing.T) {
		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, &domain.User{}, contactListFullAccess(), nil)

		mockRepo.EXPECT().
			GetContactListByIDs(gomock.Any(), workspaceID, email, listID).
			Return(&domain.ContactList{
				Email:  email,
				ListID: listID,
				Status: domain.ContactListStatusActive,
			}, nil)

		mockRepo.EXPECT().
			UpdateContactListStatus(gomock.Any(), workspaceID, email, listID, newStatus).
			Return(nil)

		result, err := service.UpdateContactListStatus(ctx, workspaceID, email, listID, newStatus)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.True(t, result.Success)
		require.Equal(t, "status updated successfully", result.Message)
		require.True(t, result.Found)
	})

	t.Run("authentication error", func(t *testing.T) {
		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, nil, nil, errors.New("auth error"))

		result, err := service.UpdateContactListStatus(ctx, workspaceID, email, listID, newStatus)
		require.Error(t, err)
		require.Nil(t, result)
	})

	t.Run("contact list not found - returns success with message", func(t *testing.T) {
		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, &domain.User{}, contactListFullAccess(), nil)

		mockRepo.EXPECT().
			GetContactListByIDs(gomock.Any(), workspaceID, email, listID).
			Return(nil, &domain.ErrContactListNotFound{Message: "not found"})

		result, err := service.UpdateContactListStatus(ctx, workspaceID, email, listID, newStatus)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.True(t, result.Success)
		require.Equal(t, "contact not in list", result.Message)
		require.False(t, result.Found)
	})

	t.Run("update error", func(t *testing.T) {
		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, &domain.User{}, contactListFullAccess(), nil)

		mockRepo.EXPECT().
			GetContactListByIDs(gomock.Any(), workspaceID, email, listID).
			Return(&domain.ContactList{
				Email:  email,
				ListID: listID,
				Status: domain.ContactListStatusActive,
			}, nil)

		mockRepo.EXPECT().
			UpdateContactListStatus(gomock.Any(), workspaceID, email, listID, newStatus).
			Return(errors.New("update error"))

		result, err := service.UpdateContactListStatus(ctx, workspaceID, email, listID, newStatus)
		require.Error(t, err)
		require.Nil(t, result)
	})
}

func TestContactListService_RemoveContactFromList(t *testing.T) {
	mockRepo, mockAuthService, _, _, service, ctrl := setupTest(t)
	defer ctrl.Finish()

	ctx := context.Background()
	workspaceID := "workspace123"
	email := "test@example.com"
	listID := "list123"

	t.Run("successful removal", func(t *testing.T) {
		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, &domain.User{}, contactListFullAccess(), nil)

		mockRepo.EXPECT().
			RemoveContactFromList(gomock.Any(), workspaceID, email, listID).
			Return(nil)

		err := service.RemoveContactFromList(ctx, workspaceID, email, listID)
		require.NoError(t, err)
	})

	t.Run("authentication error", func(t *testing.T) {
		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, nil, nil, errors.New("auth error"))

		err := service.RemoveContactFromList(ctx, workspaceID, email, listID)
		require.Error(t, err)
	})

	t.Run("not found error", func(t *testing.T) {
		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, &domain.User{}, contactListFullAccess(), nil)

		mockRepo.EXPECT().
			RemoveContactFromList(gomock.Any(), workspaceID, email, listID).
			Return(&domain.ErrContactListNotFound{Message: "not found"})

		err := service.RemoveContactFromList(ctx, workspaceID, email, listID)
		require.Error(t, err)
	})
}

// TestContactListService_PermissionEnforcement verifies that every contact-list
// operation enforces the permissions it needs. Each case grants everything the
// method requires except one, so the test fails both if a check is missing AND if
// a method is gated on the wrong permission type (a read/write swap). No repo
// expectations are set, so gomock also fails if anything beyond the gate runs.
//
// Each case also pins WHICH resource and verb the refusal names. The fixture
// holds two resources, so a gate on the wrong one still denies and a
// type-only assertion would pass; and both fields reach the client, since
// writePermissionError puts them on the wire.
//
// The two enumerating reads and both writes require a contacts grant on top of
// the lists one: they read or edit subscriber identities, not just list metadata.
func TestContactListService_PermissionEnforcement(t *testing.T) {
	// role "member" (not "owner") so HasPermission actually consults the grants.
	member := func(lists, contacts domain.ResourcePermissions) *domain.UserWorkspace {
		return &domain.UserWorkspace{
			UserID:      "user1",
			WorkspaceID: "workspace123",
			Role:        "member",
			Permissions: domain.UserPermissions{
				domain.PermissionResourceLists:    lists,
				domain.PermissionResourceContacts: contacts,
			},
		}
	}
	readOnly := domain.ResourcePermissions{Read: true}
	writeOnly := domain.ResourcePermissions{Write: true}
	full := domain.ResourcePermissions{Read: true, Write: true}

	ctx := context.Background()
	workspaceID := "workspace123"
	email := "test@example.com"
	listID := "list123"

	cases := []struct {
		name          string
		userWorkspace *domain.UserWorkspace
		resource      domain.PermissionResource
		permission    domain.PermissionType
		call          func(s *ContactListService, ctx context.Context) error
	}{
		{"GetContactListByIDs without lists read", member(writeOnly, full),
			domain.PermissionResourceLists, domain.PermissionTypeRead,
			func(s *ContactListService, ctx context.Context) error {
				_, err := s.GetContactListByIDs(ctx, workspaceID, email, listID)
				return err
			}},
		{"GetContactsByListID without lists read", member(writeOnly, full),
			domain.PermissionResourceLists, domain.PermissionTypeRead,
			func(s *ContactListService, ctx context.Context) error {
				_, err := s.GetContactsByListID(ctx, workspaceID, listID)
				return err
			}},
		{"GetContactsByListID with lists read but without contacts read", member(readOnly, writeOnly),
			domain.PermissionResourceContacts, domain.PermissionTypeRead,
			func(s *ContactListService, ctx context.Context) error {
				_, err := s.GetContactsByListID(ctx, workspaceID, listID)
				return err
			}},
		{"GetListsByEmail without lists read", member(writeOnly, full),
			domain.PermissionResourceLists, domain.PermissionTypeRead,
			func(s *ContactListService, ctx context.Context) error {
				_, err := s.GetListsByEmail(ctx, workspaceID, email)
				return err
			}},
		{"GetListsByEmail with lists read but without contacts read", member(readOnly, writeOnly),
			domain.PermissionResourceContacts, domain.PermissionTypeRead,
			func(s *ContactListService, ctx context.Context) error {
				_, err := s.GetListsByEmail(ctx, workspaceID, email)
				return err
			}},
		{"UpdateContactListStatus without lists write", member(readOnly, full),
			domain.PermissionResourceLists, domain.PermissionTypeWrite,
			func(s *ContactListService, ctx context.Context) error {
				_, err := s.UpdateContactListStatus(ctx, workspaceID, email, listID, domain.ContactListStatusActive)
				return err
			}},
		{"UpdateContactListStatus with lists write but without contacts write", member(writeOnly, readOnly),
			domain.PermissionResourceContacts, domain.PermissionTypeWrite,
			func(s *ContactListService, ctx context.Context) error {
				_, err := s.UpdateContactListStatus(ctx, workspaceID, email, listID, domain.ContactListStatusActive)
				return err
			}},
		{"RemoveContactFromList without lists write", member(readOnly, full),
			domain.PermissionResourceLists, domain.PermissionTypeWrite,
			func(s *ContactListService, ctx context.Context) error {
				return s.RemoveContactFromList(ctx, workspaceID, email, listID)
			}},
		{"RemoveContactFromList with lists write but without contacts write", member(writeOnly, readOnly),
			domain.PermissionResourceContacts, domain.PermissionTypeWrite,
			func(s *ContactListService, ctx context.Context) error {
				return s.RemoveContactFromList(ctx, workspaceID, email, listID)
			}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, mockAuthService, _, _, service, ctrl := setupTest(t)
			defer ctrl.Finish()

			mockAuthService.EXPECT().
				AuthenticateUserForWorkspace(ctx, workspaceID).
				Return(ctx, &domain.User{ID: "user1"}, tc.userWorkspace, nil)

			err := tc.call(service, ctx)
			require.Error(t, err)

			var permErr *domain.PermissionError
			require.True(t, errors.As(err, &permErr), "expected a *domain.PermissionError, got %T: %v", err, err)
			require.Equal(t, tc.resource, permErr.Resource)
			require.Equal(t, tc.permission, permErr.Permission)
		})
	}
}
