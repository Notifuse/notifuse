package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

func setupContactListTest(t *testing.T) (*mocks.MockWorkspaceRepository, *contactListRepository, sqlmock.Sqlmock, *sql.DB, func()) {
	ctrl := gomock.NewController(t)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)

	// Create a real DB connection with sqlmock
	db, mock, err := sqlmock.New()
	require.NoError(t, err)

	repo := NewContactListRepository(mockWorkspaceRepo)

	// Set up cleanup function
	cleanup := func() {
		_ = db.Close()
		ctrl.Finish()
	}

	return mockWorkspaceRepo, repo.(*contactListRepository), mock, db, cleanup
}

func TestContactListRepository_AddContactToList(t *testing.T) {
	mockWorkspaceRepo, repo, mock, db, cleanup := setupContactListTest(t)
	defer cleanup()

	ctx := context.Background()
	workspaceID := "workspace123"
	contactList := &domain.ContactList{
		Email:  "test@example.com",
		ListID: "list123",
		Status: domain.ContactListStatusActive,
	}

	t.Run("successful add", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		mock.ExpectExec(`INSERT INTO contact_lists`).
			WithArgs(contactList.Email, contactList.ListID, contactList.Status, sqlmock.AnyArg(), sqlmock.AnyArg()).
			WillReturnResult(sqlmock.NewResult(1, 1))

		err := repo.AddContactToList(ctx, workspaceID, contactList)
		require.NoError(t, err)
	})

	t.Run("workspace connection error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(nil, errors.New("connection error"))

		err := repo.AddContactToList(ctx, workspaceID, contactList)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to get workspace connection")
	})

	t.Run("execution error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		mock.ExpectExec(`INSERT INTO contact_lists`).
			WithArgs(contactList.Email, contactList.ListID, contactList.Status, sqlmock.AnyArg(), sqlmock.AnyArg()).
			WillReturnError(errors.New("execution error"))

		err := repo.AddContactToList(ctx, workspaceID, contactList)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to add contact to list")
	})
}

func TestContactListRepository_GetContactListByIDs(t *testing.T) {
	mockWorkspaceRepo, repo, mock, db, cleanup := setupContactListTest(t)
	defer cleanup()

	ctx := context.Background()
	workspaceID := "workspace123"
	email := "test@example.com"
	listID := "list123"

	t.Run("successful retrieval", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		rows := sqlmock.NewRows([]string{"email", "list_id", "status", "created_at", "updated_at", "deleted_at"}).
			AddRow(email, listID, domain.ContactListStatusActive, time.Now(), time.Now(), nil)

		mock.ExpectQuery(`SELECT email, list_id, status, created_at, updated_at, deleted_at`).
			WithArgs(email, listID).
			WillReturnRows(rows)

		result, err := repo.GetContactListByIDs(ctx, workspaceID, email, listID)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, email, result.Email)
		require.Equal(t, listID, result.ListID)
	})

	t.Run("not found", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		mock.ExpectQuery(`SELECT email, list_id, status, created_at, updated_at, deleted_at`).
			WithArgs(email, listID).
			WillReturnError(sql.ErrNoRows)

		result, err := repo.GetContactListByIDs(ctx, workspaceID, email, listID)
		require.Error(t, err)
		require.IsType(t, &domain.ErrContactListNotFound{}, err)
		require.Nil(t, result)
	})

	t.Run("workspace connection error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(nil, errors.New("connection error"))

		result, err := repo.GetContactListByIDs(ctx, workspaceID, email, listID)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to get workspace connection")
		require.Nil(t, result)
	})

	t.Run("scan error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		rows := sqlmock.NewRows([]string{"email", "list_id", "status", "created_at", "updated_at", "deleted_at"}).
			AddRow(nil, nil, nil, nil, nil, nil) // Invalid data to cause scan error

		mock.ExpectQuery(`SELECT email, list_id, status, created_at, updated_at, deleted_at`).
			WithArgs(email, listID).
			WillReturnRows(rows)

		result, err := repo.GetContactListByIDs(ctx, workspaceID, email, listID)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to get contact list")
		require.Nil(t, result)
	})
}

func TestContactListRepository_GetContactsByListID(t *testing.T) {
	mockWorkspaceRepo, repo, mock, db, cleanup := setupContactListTest(t)
	defer cleanup()

	ctx := context.Background()
	workspaceID := "workspace123"
	listID := "list123"

	t.Run("successful retrieval", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		rows := sqlmock.NewRows([]string{"email", "list_id", "status", "created_at", "updated_at", "deleted_at"}).
			AddRow("test@example.com", listID, domain.ContactListStatusActive, time.Now(), time.Now(), nil)

		mock.ExpectQuery(`SELECT email, list_id, status, created_at, updated_at, deleted_at`).
			WithArgs(listID).
			WillReturnRows(rows)

		results, err := repo.GetContactsByListID(ctx, workspaceID, listID)
		require.NoError(t, err)
		require.Len(t, results, 1)
		require.Equal(t, listID, results[0].ListID)
	})

	t.Run("query error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		mock.ExpectQuery(`SELECT email, list_id, status, created_at, updated_at, deleted_at`).
			WithArgs(listID).
			WillReturnError(errors.New("query error"))

		results, err := repo.GetContactsByListID(ctx, workspaceID, listID)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to get contacts for list")
		require.Nil(t, results)
	})

	t.Run("workspace connection error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(nil, errors.New("connection error"))

		results, err := repo.GetContactsByListID(ctx, workspaceID, listID)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to get workspace connection")
		require.Nil(t, results)
	})

	t.Run("empty result set", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		rows := sqlmock.NewRows([]string{"email", "list_id", "status", "created_at", "updated_at", "deleted_at"})

		mock.ExpectQuery(`SELECT email, list_id, status, created_at, updated_at, deleted_at`).
			WithArgs(listID).
			WillReturnRows(rows)

		results, err := repo.GetContactsByListID(ctx, workspaceID, listID)
		require.NoError(t, err)
		require.Empty(t, results)
	})

	t.Run("scan error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		rows := sqlmock.NewRows([]string{"email", "list_id", "status", "created_at", "updated_at", "deleted_at"}).
			AddRow(nil, nil, nil, nil, nil, nil) // Invalid data to cause scan error

		mock.ExpectQuery(`SELECT email, list_id, status, created_at, updated_at, deleted_at`).
			WithArgs(listID).
			WillReturnRows(rows)

		results, err := repo.GetContactsByListID(ctx, workspaceID, listID)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to scan contact list")
		require.Nil(t, results)
	})
}

func TestContactListRepository_GetListsByEmail(t *testing.T) {
	mockWorkspaceRepo, repo, mock, db, cleanup := setupContactListTest(t)
	defer cleanup()

	ctx := context.Background()
	workspaceID := "workspace123"
	email := "test@example.com"

	t.Run("successful retrieval", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		rows := sqlmock.NewRows([]string{"email", "list_id", "status", "created_at", "updated_at", "deleted_at"}).
			AddRow(email, "list123", domain.ContactListStatusActive, time.Now(), time.Now(), nil)

		mock.ExpectQuery(`SELECT email, list_id, status, created_at, updated_at, deleted_at`).
			WithArgs(email).
			WillReturnRows(rows)

		results, err := repo.GetListsByEmail(ctx, workspaceID, email)
		require.NoError(t, err)
		require.Len(t, results, 1)
		require.Equal(t, email, results[0].Email)
	})

	t.Run("query error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		mock.ExpectQuery(`SELECT email, list_id, status, created_at, updated_at, deleted_at`).
			WithArgs(email).
			WillReturnError(errors.New("query error"))

		results, err := repo.GetListsByEmail(ctx, workspaceID, email)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to get lists for contact")
		require.Nil(t, results)
	})

	t.Run("workspace connection error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(nil, errors.New("connection error"))

		results, err := repo.GetListsByEmail(ctx, workspaceID, email)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to get workspace connection")
		require.Nil(t, results)
	})

	t.Run("empty result set", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		rows := sqlmock.NewRows([]string{"email", "list_id", "status", "created_at", "updated_at", "deleted_at"})

		mock.ExpectQuery(`SELECT email, list_id, status, created_at, updated_at, deleted_at`).
			WithArgs(email).
			WillReturnRows(rows)

		results, err := repo.GetListsByEmail(ctx, workspaceID, email)
		require.NoError(t, err)
		require.Empty(t, results)
	})

	t.Run("scan error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		rows := sqlmock.NewRows([]string{"email", "list_id", "status", "created_at", "updated_at", "deleted_at"}).
			AddRow(nil, nil, nil, nil, nil, nil) // Invalid data to cause scan error

		mock.ExpectQuery(`SELECT email, list_id, status, created_at, updated_at, deleted_at`).
			WithArgs(email).
			WillReturnRows(rows)

		results, err := repo.GetListsByEmail(ctx, workspaceID, email)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to scan contact list")
		require.Nil(t, results)
	})
}

func TestContactListRepository_UpdateContactListStatus(t *testing.T) {
	mockWorkspaceRepo, repo, mock, db, cleanup := setupContactListTest(t)
	defer cleanup()

	ctx := context.Background()
	workspaceID := "workspace123"
	email := "test@example.com"
	listID := "list123"
	status := domain.ContactListStatusActive

	t.Run("successful update", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		mock.ExpectExec(`UPDATE contact_lists`).
			WithArgs(status, sqlmock.AnyArg(), email, listID).
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := repo.UpdateContactListStatus(ctx, workspaceID, email, listID, status)
		require.NoError(t, err)
	})

	t.Run("not found", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		mock.ExpectExec(`UPDATE contact_lists`).
			WithArgs(status, sqlmock.AnyArg(), email, listID).
			WillReturnResult(sqlmock.NewResult(0, 0))

		err := repo.UpdateContactListStatus(ctx, workspaceID, email, listID, status)
		require.Error(t, err)
		require.IsType(t, &domain.ErrContactListNotFound{}, err)
	})

	t.Run("workspace connection error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(nil, errors.New("connection error"))

		err := repo.UpdateContactListStatus(ctx, workspaceID, email, listID, status)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to get workspace connection")
	})

	t.Run("execution error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		mock.ExpectExec(`UPDATE contact_lists`).
			WithArgs(status, sqlmock.AnyArg(), email, listID).
			WillReturnError(errors.New("execution error"))

		err := repo.UpdateContactListStatus(ctx, workspaceID, email, listID, status)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to update contact list status")
	})

	t.Run("rows affected error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		mock.ExpectExec(`UPDATE contact_lists`).
			WithArgs(status, sqlmock.AnyArg(), email, listID).
			WillReturnResult(sqlmock.NewErrorResult(errors.New("rows affected error")))

		err := repo.UpdateContactListStatus(ctx, workspaceID, email, listID, status)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to get affected rows")
	})
}

func TestContactListRepository_RemoveContactFromList(t *testing.T) {
	mockWorkspaceRepo, repo, mock, db, cleanup := setupContactListTest(t)
	defer cleanup()

	ctx := context.Background()
	workspaceID := "workspace123"
	email := "test@example.com"
	listID := "list123"

	t.Run("successful removal", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		mock.ExpectExec(`UPDATE contact_lists SET deleted_at = (.+) WHERE email = (.+) AND list_id = (.+)`).
			WithArgs(sqlmock.AnyArg(), email, listID).
			WillReturnResult(sqlmock.NewResult(1, 1))

		err := repo.RemoveContactFromList(ctx, workspaceID, email, listID)
		require.NoError(t, err)
	})

	t.Run("not found", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		mock.ExpectExec(`UPDATE contact_lists SET deleted_at = (.+) WHERE email = (.+) AND list_id = (.+)`).
			WithArgs(sqlmock.AnyArg(), email, listID).
			WillReturnResult(sqlmock.NewResult(0, 0))

		err := repo.RemoveContactFromList(ctx, workspaceID, email, listID)
		require.Error(t, err)
		require.IsType(t, &domain.ErrContactListNotFound{}, err)
	})

	t.Run("workspace connection error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(nil, errors.New("connection error"))

		err := repo.RemoveContactFromList(ctx, workspaceID, email, listID)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to get workspace connection")
	})

	t.Run("execution error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		mock.ExpectExec(`UPDATE contact_lists SET deleted_at = (.+) WHERE email = (.+) AND list_id = (.+)`).
			WithArgs(sqlmock.AnyArg(), email, listID).
			WillReturnError(errors.New("execution error"))

		err := repo.RemoveContactFromList(ctx, workspaceID, email, listID)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to remove contact from list")
	})

	t.Run("rows affected error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		result := sqlmock.NewErrorResult(errors.New("rows affected error"))
		mock.ExpectExec(`UPDATE contact_lists SET deleted_at = (.+) WHERE email = (.+) AND list_id = (.+)`).
			WithArgs(sqlmock.AnyArg(), email, listID).
			WillReturnResult(result)

		err := repo.RemoveContactFromList(ctx, workspaceID, email, listID)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to get affected rows")
	})
}

func TestContactListRepository_DeleteForEmail(t *testing.T) {
	mockWorkspaceRepo, repo, mock, db, cleanup := setupContactListTest(t)
	defer cleanup()

	ctx := context.Background()
	workspaceID := "workspace123"
	email := "test@example.com"

	t.Run("successful deletion", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		mock.ExpectExec(`DELETE FROM contact_lists WHERE email = \$1`).
			WithArgs(email).
			WillReturnResult(sqlmock.NewResult(0, 2))

		err := repo.DeleteForEmail(ctx, workspaceID, email)
		require.NoError(t, err)
	})

	t.Run("successful deletion with no rows affected", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		mock.ExpectExec(`DELETE FROM contact_lists WHERE email = \$1`).
			WithArgs(email).
			WillReturnResult(sqlmock.NewResult(0, 0))

		err := repo.DeleteForEmail(ctx, workspaceID, email)
		require.NoError(t, err)
	})

	t.Run("workspace connection error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(nil, errors.New("connection error"))

		err := repo.DeleteForEmail(ctx, workspaceID, email)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to get workspace connection")
	})

	t.Run("execution error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		mock.ExpectExec(`DELETE FROM contact_lists WHERE email = \$1`).
			WithArgs(email).
			WillReturnError(errors.New("execution error"))

		err := repo.DeleteForEmail(ctx, workspaceID, email)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to delete contact list relationships")
	})

	t.Run("rows affected error", func(t *testing.T) {
		mockWorkspaceRepo.EXPECT().
			GetConnection(ctx, workspaceID).
			Return(db, nil)

		mock.ExpectExec(`DELETE FROM contact_lists WHERE email = \$1`).
			WithArgs(email).
			WillReturnResult(sqlmock.NewErrorResult(errors.New("rows affected error")))

		err := repo.DeleteForEmail(ctx, workspaceID, email)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to get affected rows")
	})
}
