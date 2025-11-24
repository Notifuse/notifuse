package repository

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
	"github.com/golang/mock/gomock"
)

func TestListRepository(t *testing.T) {
	repo, mockWorkspaceRepo := setupListRepositoryTest(t)

	// Create a test list
	testList := &domain.List{
		ID:                  "list123",
		Name:                "Test List",
		IsDoubleOptin:       true,
		IsPublic:            true,
		Description:         "This is a test list",
		DoubleOptInTemplate: nil,
		WelcomeTemplate:     nil,
		UnsubscribeTemplate: nil,
		CreatedAt:           time.Now().UTC(),
		UpdatedAt:           time.Now().UTC(),
	}

	// Setup workspace connection mock
	db, sqlMock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mockWorkspaceRepo.EXPECT().
		GetConnection(gomock.Any(), "workspace123").
		Return(db, nil).
		AnyTimes()

	t.Run("CreateList", func(t *testing.T) {
		t.Run("successful creation", func(t *testing.T) {
			sqlMock.ExpectExec(regexp.QuoteMeta(`
				INSERT INTO lists (id, name, is_double_optin, is_public, description, 
				                   double_optin_template, welcome_template, unsubscribe_template,
				                   created_at, updated_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			`)).WithArgs(
				testList.ID,
				testList.Name,
				testList.IsDoubleOptin,
				testList.IsPublic,
				testList.Description,
				testList.DoubleOptInTemplate,
				testList.WelcomeTemplate,
				testList.UnsubscribeTemplate,
				sqlmock.AnyArg(),
				sqlmock.AnyArg(),
			).WillReturnResult(sqlmock.NewResult(1, 1))

			err := repo.CreateList(context.Background(), "workspace123", testList)
			require.NoError(t, err)
		})

		t.Run("database error", func(t *testing.T) {
			sqlMock.ExpectExec(regexp.QuoteMeta(`
				INSERT INTO lists (id, name, is_double_optin, is_public, description, 
				                   double_optin_template, welcome_template, unsubscribe_template,
				                   created_at, updated_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			`)).WithArgs(
				testList.ID,
				testList.Name,
				testList.IsDoubleOptin,
				testList.IsPublic,
				testList.Description,
				testList.DoubleOptInTemplate,
				testList.WelcomeTemplate,
				testList.UnsubscribeTemplate,
				sqlmock.AnyArg(),
				sqlmock.AnyArg(),
			).WillReturnError(errors.New("database error"))

			err := repo.CreateList(context.Background(), "workspace123", testList)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "failed to create list")
		})
	})

	t.Run("GetListByID", func(t *testing.T) {
		t.Run("list found", func(t *testing.T) {
			rows := sqlmock.NewRows([]string{
				"id", "name", "is_double_optin", "is_public", "description", "double_optin_template",
				"welcome_template", "unsubscribe_template", "created_at", "updated_at", "deleted_at",
			}).AddRow(
				testList.ID,
				testList.Name,
				testList.IsDoubleOptin,
				testList.IsPublic,
				testList.Description,
				testList.DoubleOptInTemplate,
				testList.WelcomeTemplate,
				testList.UnsubscribeTemplate,
				testList.CreatedAt,
				testList.UpdatedAt,
				nil,
			)

			sqlMock.ExpectQuery(regexp.QuoteMeta(`
				SELECT id, name, is_double_optin, is_public, description, double_optin_template, 
				welcome_template, unsubscribe_template, created_at, updated_at, deleted_at
				FROM lists
				WHERE id = $1 AND deleted_at IS NULL
			`)).WithArgs(testList.ID).WillReturnRows(rows)

			list, err := repo.GetListByID(context.Background(), "workspace123", testList.ID)
			require.NoError(t, err)
			assert.Equal(t, testList.ID, list.ID)
			assert.Equal(t, testList.Name, list.Name)
			assert.Equal(t, testList.IsDoubleOptin, list.IsDoubleOptin)
			assert.Equal(t, testList.IsPublic, list.IsPublic)
			assert.Equal(t, testList.Description, list.Description)
		})

		t.Run("list not found", func(t *testing.T) {
			sqlMock.ExpectQuery(regexp.QuoteMeta(`
				SELECT id, name, is_double_optin, is_public, description, double_optin_template, 
				welcome_template, unsubscribe_template, created_at, updated_at, deleted_at
				FROM lists
				WHERE id = $1 AND deleted_at IS NULL
			`)).WithArgs(testList.ID).WillReturnError(sql.ErrNoRows)

			list, err := repo.GetListByID(context.Background(), "workspace123", testList.ID)
			require.Error(t, err)
			assert.Nil(t, list)
			assert.IsType(t, &domain.ErrListNotFound{}, err)
		})

		t.Run("database error", func(t *testing.T) {
			sqlMock.ExpectQuery(regexp.QuoteMeta(`
				SELECT id, name, is_double_optin, is_public, description, double_optin_template, 
				welcome_template, unsubscribe_template, created_at, updated_at, deleted_at
				FROM lists
				WHERE id = $1 AND deleted_at IS NULL
			`)).WithArgs(testList.ID).WillReturnError(errors.New("database error"))

			list, err := repo.GetListByID(context.Background(), "workspace123", testList.ID)
			require.Error(t, err)
			assert.Nil(t, list)
			assert.Contains(t, err.Error(), "failed to get list")
		})
	})

	t.Run("GetLists", func(t *testing.T) {
		t.Run("successful retrieval", func(t *testing.T) {
			rows := sqlmock.NewRows([]string{
				"id", "name", "is_double_optin", "is_public", "description",
				"double_optin_template",
				"welcome_template", "unsubscribe_template", "created_at", "updated_at", "deleted_at",
			}).AddRow(
				testList.ID,
				testList.Name,
				testList.IsDoubleOptin,
				testList.IsPublic,
				testList.Description,
				testList.DoubleOptInTemplate,
				testList.WelcomeTemplate,
				testList.UnsubscribeTemplate,
				testList.CreatedAt,
				testList.UpdatedAt,
				nil,
			)

			sqlMock.ExpectQuery(regexp.QuoteMeta(`
				SELECT id, name, is_double_optin, is_public, description, double_optin_template, 
				welcome_template, unsubscribe_template, created_at, updated_at, deleted_at
				FROM lists
				WHERE deleted_at IS NULL
				ORDER BY created_at DESC
			`)).WillReturnRows(rows)

			lists, err := repo.GetLists(context.Background(), "workspace123")
			require.NoError(t, err)
			require.Len(t, lists, 1)
			assert.Equal(t, testList.ID, lists[0].ID)
			assert.Equal(t, testList.Name, lists[0].Name)
			assert.Equal(t, testList.IsDoubleOptin, lists[0].IsDoubleOptin)
			assert.Equal(t, testList.IsPublic, lists[0].IsPublic)
			assert.Equal(t, testList.Description, lists[0].Description)
		})

		t.Run("database error", func(t *testing.T) {
			sqlMock.ExpectQuery(regexp.QuoteMeta(`
				SELECT id, name, is_double_optin, is_public, description, double_optin_template, 
				welcome_template, unsubscribe_template, created_at, updated_at, deleted_at
				FROM lists
				WHERE deleted_at IS NULL
				ORDER BY created_at DESC
			`)).WillReturnError(errors.New("database error"))

			lists, err := repo.GetLists(context.Background(), "workspace123")
			require.Error(t, err)
			assert.Nil(t, lists)
			assert.Contains(t, err.Error(), "failed to get lists")
		})
	})

	t.Run("UpdateList", func(t *testing.T) {
		t.Run("successful update", func(t *testing.T) {
			sqlMock.ExpectExec(regexp.QuoteMeta(`
				UPDATE lists
				SET name = $1, is_double_optin = $2, is_public = $3, description = $4, updated_at = $5,
				    double_optin_template = $6, welcome_template = $7, unsubscribe_template = $8
				WHERE id = $9 AND deleted_at IS NULL
			`)).WithArgs(
				testList.Name,
				testList.IsDoubleOptin,
				testList.IsPublic,
				testList.Description,
				sqlmock.AnyArg(),
				testList.DoubleOptInTemplate,
				testList.WelcomeTemplate,
				testList.UnsubscribeTemplate,
				testList.ID,
			).WillReturnResult(sqlmock.NewResult(0, 1))

			err := repo.UpdateList(context.Background(), "workspace123", testList)
			require.NoError(t, err)
		})

		t.Run("list not found", func(t *testing.T) {
			sqlMock.ExpectExec(regexp.QuoteMeta(`
				UPDATE lists
				SET name = $1, is_double_optin = $2, is_public = $3, description = $4, updated_at = $5,
				    double_optin_template = $6, welcome_template = $7, unsubscribe_template = $8
				WHERE id = $9 AND deleted_at IS NULL
			`)).WithArgs(
				testList.Name,
				testList.IsDoubleOptin,
				testList.IsPublic,
				testList.Description,
				sqlmock.AnyArg(),
				testList.DoubleOptInTemplate,
				testList.WelcomeTemplate,
				testList.UnsubscribeTemplate,
				testList.ID,
			).WillReturnResult(sqlmock.NewResult(0, 0))

			err := repo.UpdateList(context.Background(), "workspace123", testList)
			require.Error(t, err)
			assert.IsType(t, &domain.ErrListNotFound{}, err)
		})

		t.Run("database error", func(t *testing.T) {
			sqlMock.ExpectExec(regexp.QuoteMeta(`
				UPDATE lists
				SET name = $1, is_double_optin = $2, is_public = $3, description = $4, updated_at = $5,
				    double_optin_template = $6, welcome_template = $7, unsubscribe_template = $8
				WHERE id = $9 AND deleted_at IS NULL
			`)).WithArgs(
				testList.Name,
				testList.IsDoubleOptin,
				testList.IsPublic,
				testList.Description,
				sqlmock.AnyArg(),
				testList.DoubleOptInTemplate,
				testList.WelcomeTemplate,
				testList.UnsubscribeTemplate,
				testList.ID,
			).WillReturnError(errors.New("database error"))

			err := repo.UpdateList(context.Background(), "workspace123", testList)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "failed to update list")
		})
	})

	t.Run("DeleteList", func(t *testing.T) {
		t.Run("successful deletion", func(t *testing.T) {
			// Expect begin transaction
			sqlMock.ExpectBegin()

			// Expect list update
			sqlMock.ExpectExec(regexp.QuoteMeta(`UPDATE lists SET deleted_at = $1 WHERE id = $2 AND deleted_at IS NULL`)).
				WithArgs(sqlmock.AnyArg(), testList.ID).
				WillReturnResult(sqlmock.NewResult(0, 1))

			// Expect contact_list update
			sqlMock.ExpectExec(regexp.QuoteMeta(`UPDATE contact_lists SET deleted_at = $1 WHERE list_id = $2 AND deleted_at IS NULL`)).
				WithArgs(sqlmock.AnyArg(), testList.ID).
				WillReturnResult(sqlmock.NewResult(0, 0)) // No rows affected is fine for contact lists

			// Expect commit
			sqlMock.ExpectCommit()

			err := repo.DeleteList(context.Background(), "workspace123", testList.ID)
			require.NoError(t, err)
		})

		t.Run("list not found", func(t *testing.T) {
			// Expect begin transaction
			sqlMock.ExpectBegin()

			// Expect list update - no rows affected
			sqlMock.ExpectExec(regexp.QuoteMeta(`UPDATE lists SET deleted_at = $1 WHERE id = $2 AND deleted_at IS NULL`)).
				WithArgs(sqlmock.AnyArg(), testList.ID).
				WillReturnResult(sqlmock.NewResult(0, 0))

			// Expect rollback since list not found
			sqlMock.ExpectRollback()

			err := repo.DeleteList(context.Background(), "workspace123", testList.ID)
			require.Error(t, err)
			assert.IsType(t, &domain.ErrListNotFound{}, err)
		})

		t.Run("database error", func(t *testing.T) {
			// Expect begin transaction
			sqlMock.ExpectBegin()

			// Expect list update - error
			sqlMock.ExpectExec(regexp.QuoteMeta(`UPDATE lists SET deleted_at = $1 WHERE id = $2 AND deleted_at IS NULL`)).
				WithArgs(sqlmock.AnyArg(), testList.ID).
				WillReturnError(errors.New("database error"))

			// Expect rollback
			sqlMock.ExpectRollback()

			err := repo.DeleteList(context.Background(), "workspace123", testList.ID)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "failed to soft delete list")
		})

		t.Run("list already deleted", func(t *testing.T) {
			// Expect begin transaction
			sqlMock.ExpectBegin()

			// Expect list update - no rows affected
			sqlMock.ExpectExec(regexp.QuoteMeta(`UPDATE lists SET deleted_at = $1 WHERE id = $2 AND deleted_at IS NULL`)).
				WithArgs(sqlmock.AnyArg(), testList.ID).
				WillReturnResult(sqlmock.NewResult(0, 0))

			// Expect rollback
			sqlMock.ExpectRollback()

			err := repo.DeleteList(context.Background(), "workspace123", testList.ID)
			require.Error(t, err)
			assert.IsType(t, &domain.ErrListNotFound{}, err)
			assert.Contains(t, err.Error(), "list not found or already deleted")
		})
	})

	t.Run("GetListStats", func(t *testing.T) {
		t.Run("successful retrieval", func(t *testing.T) {
			rows := sqlmock.NewRows([]string{
				"total_active", "total_pending", "total_unsubscribed", "total_bounced", "total_complained",
			}).AddRow(10, 5, 3, 1, 0)

			sqlMock.ExpectQuery(regexp.QuoteMeta(`
				SELECT 
					SUM(CASE WHEN status = 'active' THEN 1 ELSE 0 END) as total_active,
					SUM(CASE WHEN status = 'pending' THEN 1 ELSE 0 END) as total_pending,
					SUM(CASE WHEN status = 'unsubscribed' THEN 1 ELSE 0 END) as total_unsubscribed,
					SUM(CASE WHEN status = 'bounced' THEN 1 ELSE 0 END) as total_bounced,
					SUM(CASE WHEN status = 'complained' THEN 1 ELSE 0 END) as total_complained
				FROM contact_lists
				WHERE list_id = $1 AND deleted_at IS NULL
			`)).WithArgs(testList.ID).WillReturnRows(rows)

			stats, err := repo.GetListStats(context.Background(), "workspace123", testList.ID)
			require.NoError(t, err)
			assert.Equal(t, 10, stats.TotalActive)
			assert.Equal(t, 5, stats.TotalPending)
			assert.Equal(t, 3, stats.TotalUnsubscribed)
			assert.Equal(t, 1, stats.TotalBounced)
			assert.Equal(t, 0, stats.TotalComplained)
		})

		t.Run("list not found or no contacts", func(t *testing.T) {
			// When the list exists but has no contacts, the database will return 0s for all counts, not an error
			rows := sqlmock.NewRows([]string{
				"total_active", "total_pending", "total_unsubscribed", "total_bounced", "total_complained",
			}).AddRow(0, 0, 0, 0, 0)

			sqlMock.ExpectQuery(regexp.QuoteMeta(`
				SELECT 
					SUM(CASE WHEN status = 'active' THEN 1 ELSE 0 END) as total_active,
					SUM(CASE WHEN status = 'pending' THEN 1 ELSE 0 END) as total_pending,
					SUM(CASE WHEN status = 'unsubscribed' THEN 1 ELSE 0 END) as total_unsubscribed,
					SUM(CASE WHEN status = 'bounced' THEN 1 ELSE 0 END) as total_bounced,
					SUM(CASE WHEN status = 'complained' THEN 1 ELSE 0 END) as total_complained
				FROM contact_lists
				WHERE list_id = $1 AND deleted_at IS NULL
			`)).WithArgs("nonexistent-list").WillReturnRows(rows)

			stats, err := repo.GetListStats(context.Background(), "workspace123", "nonexistent-list")
			require.NoError(t, err)
			assert.Equal(t, 0, stats.TotalActive)
			assert.Equal(t, 0, stats.TotalPending)
			assert.Equal(t, 0, stats.TotalUnsubscribed)
			assert.Equal(t, 0, stats.TotalBounced)
			assert.Equal(t, 0, stats.TotalComplained)
		})

		t.Run("database error", func(t *testing.T) {
			sqlMock.ExpectQuery(regexp.QuoteMeta(`
				SELECT 
					SUM(CASE WHEN status = 'active' THEN 1 ELSE 0 END) as total_active,
					SUM(CASE WHEN status = 'pending' THEN 1 ELSE 0 END) as total_pending,
					SUM(CASE WHEN status = 'unsubscribed' THEN 1 ELSE 0 END) as total_unsubscribed,
					SUM(CASE WHEN status = 'bounced' THEN 1 ELSE 0 END) as total_bounced,
					SUM(CASE WHEN status = 'complained' THEN 1 ELSE 0 END) as total_complained
				FROM contact_lists
				WHERE list_id = $1 AND deleted_at IS NULL
			`)).WithArgs(testList.ID).WillReturnError(errors.New("database error"))

			stats, err := repo.GetListStats(context.Background(), "workspace123", testList.ID)
			require.Error(t, err)
			assert.Nil(t, stats)
			assert.Contains(t, err.Error(), "failed to get list stats")
		})
	})
}

func setupListRepositoryTest(t *testing.T) (*listRepository, *mocks.MockWorkspaceRepository) {
	ctrl := gomock.NewController(t)
	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)

	repo := NewListRepository(mockWorkspaceRepo).(*listRepository)

	return repo, mockWorkspaceRepo
}
