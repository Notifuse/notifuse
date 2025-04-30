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
	mockAuthService := mocks.NewMockAuthService(ctrl)
	mockContactRepo := mocks.NewMockContactRepository(ctrl)
	mockListRepo := mocks.NewMockListRepository(ctrl)
	mockLogger := pkgmocks.NewMockLogger(ctrl)

	// Set up logger expectations
	mockLogger.EXPECT().WithField(gomock.Any(), gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().WithFields(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Debug(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any()).AnyTimes()

	service := NewContactListService(mockRepo, mockAuthService, mockContactRepo, mockListRepo, mockLogger)

	return mockRepo, mockAuthService, mockContactRepo, mockListRepo, service, ctrl
}

func TestContactListService_AddContactToList(t *testing.T) {
	mockRepo, mockAuthService, mockContactRepo, mockListRepo, service, ctrl := setupTest(t)
	defer ctrl.Finish()

	ctx := context.Background()
	workspaceID := "workspace123"
	email := "test@example.com"
	listID := "list123"

	t.Run("successful addition", func(t *testing.T) {
		contactList := &domain.ContactList{
			Email:  email,
			ListID: listID,
			Status: domain.ContactListStatusActive,
		}

		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, &domain.User{}, nil)

		mockContactRepo.EXPECT().
			GetContactByEmail(gomock.Any(), workspaceID, email).
			Return(&domain.Contact{Email: email}, nil)

		mockListRepo.EXPECT().
			GetListByID(gomock.Any(), workspaceID, listID).
			Return(&domain.List{
				ID:            listID,
				IsDoubleOptin: false,
			}, nil)

		mockRepo.EXPECT().
			AddContactToList(gomock.Any(), workspaceID, gomock.Any()).
			Return(nil)

		mockListRepo.EXPECT().
			IncrementTotal(gomock.Any(), workspaceID, listID, domain.TotalTypeActive).
			Return(nil)

		err := service.AddContactToList(ctx, workspaceID, contactList)
		require.NoError(t, err)
	})

	t.Run("double opt-in list", func(t *testing.T) {
		contactList := &domain.ContactList{
			Email:  email,
			ListID: listID,
			Status: domain.ContactListStatusActive,
		}

		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, &domain.User{}, nil)

		mockContactRepo.EXPECT().
			GetContactByEmail(gomock.Any(), workspaceID, email).
			Return(&domain.Contact{Email: email}, nil)

		mockListRepo.EXPECT().
			GetListByID(gomock.Any(), workspaceID, listID).
			Return(&domain.List{
				ID:            listID,
				IsDoubleOptin: true,
			}, nil)

		mockRepo.EXPECT().
			AddContactToList(gomock.Any(), workspaceID, gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, cl *domain.ContactList) error {
				require.Equal(t, domain.ContactListStatusPending, cl.Status)
				return nil
			})

		mockListRepo.EXPECT().
			IncrementTotal(gomock.Any(), workspaceID, listID, domain.TotalTypeActive).
			Return(nil)

		err := service.AddContactToList(ctx, workspaceID, contactList)
		require.NoError(t, err)
	})

	t.Run("authentication error", func(t *testing.T) {
		contactList := &domain.ContactList{
			Email:  email,
			ListID: listID,
			Status: domain.ContactListStatusActive,
		}

		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, nil, errors.New("auth error"))

		err := service.AddContactToList(ctx, workspaceID, contactList)
		require.Error(t, err)
	})

	t.Run("contact not found", func(t *testing.T) {
		contactList := &domain.ContactList{
			Email:  email,
			ListID: listID,
			Status: domain.ContactListStatusActive,
		}

		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, &domain.User{}, nil)

		mockContactRepo.EXPECT().
			GetContactByEmail(gomock.Any(), workspaceID, email).
			Return(nil, errors.New("not found"))

		err := service.AddContactToList(ctx, workspaceID, contactList)
		require.Error(t, err)
	})

	t.Run("list not found", func(t *testing.T) {
		contactList := &domain.ContactList{
			Email:  email,
			ListID: listID,
			Status: domain.ContactListStatusActive,
		}

		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, &domain.User{}, nil)

		mockContactRepo.EXPECT().
			GetContactByEmail(gomock.Any(), workspaceID, email).
			Return(&domain.Contact{Email: email}, nil)

		mockListRepo.EXPECT().
			GetListByID(gomock.Any(), workspaceID, listID).
			Return(nil, errors.New("not found"))

		err := service.AddContactToList(ctx, workspaceID, contactList)
		require.Error(t, err)
	})
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
			Return(ctx, &domain.User{}, nil)

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
			Return(ctx, nil, errors.New("auth error"))

		result, err := service.GetContactListByIDs(ctx, workspaceID, email, listID)
		require.Error(t, err)
		require.Nil(t, result)
	})

	t.Run("not found error", func(t *testing.T) {
		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, &domain.User{}, nil)

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
			Return(ctx, &domain.User{}, nil)

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
			Return(ctx, nil, errors.New("auth error"))

		result, err := service.GetContactsByListID(ctx, workspaceID, listID)
		require.Error(t, err)
		require.Nil(t, result)
	})

	t.Run("list not found", func(t *testing.T) {
		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, &domain.User{}, nil)

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
			Return(ctx, &domain.User{}, nil)

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
			Return(ctx, nil, errors.New("auth error"))

		result, err := service.GetListsByEmail(ctx, workspaceID, email)
		require.Error(t, err)
		require.Nil(t, result)
	})

	t.Run("contact not found", func(t *testing.T) {
		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, &domain.User{}, nil)

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
			Return(ctx, &domain.User{}, nil)

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

		err := service.UpdateContactListStatus(ctx, workspaceID, email, listID, newStatus)
		require.NoError(t, err)
	})

	t.Run("authentication error", func(t *testing.T) {
		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, nil, errors.New("auth error"))

		err := service.UpdateContactListStatus(ctx, workspaceID, email, listID, newStatus)
		require.Error(t, err)
	})

	t.Run("contact list not found", func(t *testing.T) {
		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, &domain.User{}, nil)

		mockRepo.EXPECT().
			GetContactListByIDs(gomock.Any(), workspaceID, email, listID).
			Return(nil, &domain.ErrContactListNotFound{Message: "not found"})

		err := service.UpdateContactListStatus(ctx, workspaceID, email, listID, newStatus)
		require.Error(t, err)
	})

	t.Run("update error", func(t *testing.T) {
		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, &domain.User{}, nil)

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

		err := service.UpdateContactListStatus(ctx, workspaceID, email, listID, newStatus)
		require.Error(t, err)
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
			Return(ctx, &domain.User{}, nil)

		mockRepo.EXPECT().
			RemoveContactFromList(gomock.Any(), workspaceID, email, listID).
			Return(nil)

		err := service.RemoveContactFromList(ctx, workspaceID, email, listID)
		require.NoError(t, err)
	})

	t.Run("authentication error", func(t *testing.T) {
		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, nil, errors.New("auth error"))

		err := service.RemoveContactFromList(ctx, workspaceID, email, listID)
		require.Error(t, err)
	})

	t.Run("not found error", func(t *testing.T) {
		mockAuthService.EXPECT().
			AuthenticateUserForWorkspace(ctx, workspaceID).
			Return(ctx, &domain.User{}, nil)

		mockRepo.EXPECT().
			RemoveContactFromList(gomock.Any(), workspaceID, email, listID).
			Return(&domain.ErrContactListNotFound{Message: "not found"})

		err := service.RemoveContactFromList(ctx, workspaceID, email, listID)
		require.Error(t, err)
	})
}
