package repository

import (
	"context"
	"database/sql"
	"database/sql/driver"
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
				                   double_optin_template, created_at, updated_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			`)).WithArgs(
				testList.ID,
				testList.Name,
				testList.IsDoubleOptin,
				testList.IsPublic,
				testList.Description,
				testList.DoubleOptInTemplate,
				sqlmock.AnyArg(),
				sqlmock.AnyArg(),
			).WillReturnResult(sqlmock.NewResult(1, 1))

			err := repo.CreateList(context.Background(), "workspace123", testList)
			require.NoError(t, err)
		})

		t.Run("database error", func(t *testing.T) {
			sqlMock.ExpectExec(regexp.QuoteMeta(`
				INSERT INTO lists (id, name, is_double_optin, is_public, description,
				                   double_optin_template, created_at, updated_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			`)).WithArgs(
				testList.ID,
				testList.Name,
				testList.IsDoubleOptin,
				testList.IsPublic,
				testList.Description,
				testList.DoubleOptInTemplate,
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
				"created_at", "updated_at", "deleted_at",
			}).AddRow(
				testList.ID,
				testList.Name,
				testList.IsDoubleOptin,
				testList.IsPublic,
				testList.Description,
				testList.DoubleOptInTemplate,
				testList.CreatedAt,
				testList.UpdatedAt,
				nil,
			)

			sqlMock.ExpectQuery(regexp.QuoteMeta(`
				SELECT id, name, is_double_optin, is_public, description, double_optin_template,
				created_at, updated_at, deleted_at
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
				created_at, updated_at, deleted_at
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
				created_at, updated_at, deleted_at
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
				"double_optin_template", "created_at", "updated_at", "deleted_at",
			}).AddRow(
				testList.ID,
				testList.Name,
				testList.IsDoubleOptin,
				testList.IsPublic,
				testList.Description,
				testList.DoubleOptInTemplate,
				testList.CreatedAt,
				testList.UpdatedAt,
				nil,
			)

			sqlMock.ExpectQuery(regexp.QuoteMeta(`
				SELECT id, name, is_double_optin, is_public, description, double_optin_template,
				created_at, updated_at, deleted_at
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
				created_at, updated_at, deleted_at
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
				    double_optin_template = $6
				WHERE id = $7 AND deleted_at IS NULL
			`)).WithArgs(
				testList.Name,
				testList.IsDoubleOptin,
				testList.IsPublic,
				testList.Description,
				sqlmock.AnyArg(),
				testList.DoubleOptInTemplate,
				testList.ID,
			).WillReturnResult(sqlmock.NewResult(0, 1))

			err := repo.UpdateList(context.Background(), "workspace123", testList)
			require.NoError(t, err)
		})

		t.Run("list not found", func(t *testing.T) {
			sqlMock.ExpectExec(regexp.QuoteMeta(`
				UPDATE lists
				SET name = $1, is_double_optin = $2, is_public = $3, description = $4, updated_at = $5,
				    double_optin_template = $6
				WHERE id = $7 AND deleted_at IS NULL
			`)).WithArgs(
				testList.Name,
				testList.IsDoubleOptin,
				testList.IsPublic,
				testList.Description,
				sqlmock.AnyArg(),
				testList.DoubleOptInTemplate,
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
				    double_optin_template = $6
				WHERE id = $7 AND deleted_at IS NULL
			`)).WithArgs(
				testList.Name,
				testList.IsDoubleOptin,
				testList.IsPublic,
				testList.Description,
				sqlmock.AnyArg(),
				testList.DoubleOptInTemplate,
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
			sqlMock.ExpectQuery(regexp.QuoteMeta(`UPDATE lists SET deleted_at = $1 WHERE id = $2 AND deleted_at IS NULL RETURNING name`)).
				WithArgs(sqlmock.AnyArg(), testList.ID).
				WillReturnRows(sqlmock.NewRows([]string{"name"}).AddRow(testList.Name))

			// Expect the in-use check: no live automation
			sqlMock.ExpectQuery(`SELECT id, name, COALESCE\(list_id, ''\), trigger_config, nodes\s+FROM automations`).
				WithArgs("workspace123").
				WillReturnRows(sqlmock.NewRows([]string{"id", "name", "list_id", "trigger_config", "nodes"}))

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
			sqlMock.ExpectQuery(regexp.QuoteMeta(`UPDATE lists SET deleted_at = $1 WHERE id = $2 AND deleted_at IS NULL RETURNING name`)).
				WithArgs(sqlmock.AnyArg(), testList.ID).
				WillReturnRows(sqlmock.NewRows([]string{"name"}))

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
			sqlMock.ExpectQuery(regexp.QuoteMeta(`UPDATE lists SET deleted_at = $1 WHERE id = $2 AND deleted_at IS NULL RETURNING name`)).
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
			sqlMock.ExpectQuery(regexp.QuoteMeta(`UPDATE lists SET deleted_at = $1 WHERE id = $2 AND deleted_at IS NULL RETURNING name`)).
				WithArgs(sqlmock.AnyArg(), testList.ID).
				WillReturnRows(sqlmock.NewRows([]string{"name"}))

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
					COALESCE(SUM(CASE WHEN status = 'active' THEN 1 ELSE 0 END), 0) as total_active,
					COALESCE(SUM(CASE WHEN status = 'pending' THEN 1 ELSE 0 END), 0) as total_pending,
					COALESCE(SUM(CASE WHEN status = 'unsubscribed' THEN 1 ELSE 0 END), 0) as total_unsubscribed,
					COALESCE(SUM(CASE WHEN status = 'bounced' THEN 1 ELSE 0 END), 0) as total_bounced,
					COALESCE(SUM(CASE WHEN status = 'complained' THEN 1 ELSE 0 END), 0) as total_complained
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
					COALESCE(SUM(CASE WHEN status = 'active' THEN 1 ELSE 0 END), 0) as total_active,
					COALESCE(SUM(CASE WHEN status = 'pending' THEN 1 ELSE 0 END), 0) as total_pending,
					COALESCE(SUM(CASE WHEN status = 'unsubscribed' THEN 1 ELSE 0 END), 0) as total_unsubscribed,
					COALESCE(SUM(CASE WHEN status = 'bounced' THEN 1 ELSE 0 END), 0) as total_bounced,
					COALESCE(SUM(CASE WHEN status = 'complained' THEN 1 ELSE 0 END), 0) as total_complained
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
					COALESCE(SUM(CASE WHEN status = 'active' THEN 1 ELSE 0 END), 0) as total_active,
					COALESCE(SUM(CASE WHEN status = 'pending' THEN 1 ELSE 0 END), 0) as total_pending,
					COALESCE(SUM(CASE WHEN status = 'unsubscribed' THEN 1 ELSE 0 END), 0) as total_unsubscribed,
					COALESCE(SUM(CASE WHEN status = 'bounced' THEN 1 ELSE 0 END), 0) as total_bounced,
					COALESCE(SUM(CASE WHEN status = 'complained' THEN 1 ELSE 0 END), 0) as total_complained
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

// DeleteList refuses a list that live automations require. The check runs in the deletion's
// transaction, after the update that locks the list row and before any membership is touched,
// so a refusal leaves the list and its members exactly as they were.
func TestListRepository_DeleteList_LiveAutomations(t *testing.T) {
	const workspaceID = "workspace123"
	const listID = "list123"

	setup := func(t *testing.T) (*listRepository, sqlmock.Sqlmock) {
		repo, mockWorkspaceRepo := setupListRepositoryTest(t)
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		t.Cleanup(func() { _ = db.Close() })
		mockWorkspaceRepo.EXPECT().GetConnection(gomock.Any(), workspaceID).Return(db, nil)
		return repo, mock
	}

	expectListLocked := func(mock sqlmock.Sqlmock) {
		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(`UPDATE lists SET deleted_at = $1 WHERE id = $2 AND deleted_at IS NULL RETURNING name`)).
			WithArgs(sqlmock.AnyArg(), listID).
			WillReturnRows(sqlmock.NewRows([]string{"name"}).AddRow("Newsletter"))
	}

	// Live, undeleted automations of the workspace, and nothing else.
	const liveAutomationsQuery = `SELECT id, name, COALESCE\(list_id, ''\), trigger_config, nodes\s+` +
		`FROM automations\s+WHERE workspace_id = \$1 AND status = 'live' AND deleted_at IS NULL`

	columns := []string{"id", "name", "list_id", "trigger_config", "nodes"}
	custom := `{"event_kind": "custom_event", "custom_event_name": "signed_up", "frequency": "every_time"}`

	// What a live automation can hold the list with, and what it cannot.
	sendsEmail := []driver.Value{"a-email", "Welcome series", listID, []byte(custom),
		[]byte(`[{"id": "t", "type": "trigger", "config": {}}, {"id": "e", "type": "email", "config": {"template_id": "tpl"}}]`)}
	listens := []driver.Value{"a-trigger", "Listener", "",
		[]byte(`{"event_kind": "list.removed", "list_id": "` + listID + `", "frequency": "every_time"}`), []byte(`[]`)}
	adds := []driver.Value{"a-adds", "Adder", "", []byte(custom),
		[]byte(`[{"id": "a", "type": "add_to_list", "config": {"list_id": "` + listID + `", "status": "active"}}]`)}
	reads := []driver.Value{"a-reader", "Reader", "", []byte(custom),
		[]byte(`[{"id": "r", "type": "remove_from_list", "config": {"list_id": "` + listID + `"}},
			{"id": "b", "type": "list_status_branch", "config": {"list_id": "` + listID + `", "active_node_id": "r"}}]`)}
	sendsNoEmail := []driver.Value{"a-silent", "Silent", listID, []byte(custom),
		[]byte(`[{"id": "d", "type": "delay", "config": {"duration": 1, "unit": "days"}}]`)}
	otherList := []driver.Value{"a-other", "Other list", "other", []byte(custom),
		[]byte(`[{"id": "e", "type": "email", "config": {"template_id": "tpl"}}]`)}

	t.Run("refused while live automations require the list, naming each one", func(t *testing.T) {
		repo, mock := setup(t)
		expectListLocked(mock)
		mock.ExpectQuery(liveAutomationsQuery).WithArgs(workspaceID).
			WillReturnRows(sqlmock.NewRows(columns).
				AddRow(sendsEmail...).AddRow(reads...).AddRow(listens...).
				AddRow(sendsNoEmail...).AddRow(adds...).AddRow(otherList...))
		// No membership is touched, and nothing is committed.
		mock.ExpectRollback()

		err := repo.DeleteList(context.Background(), workspaceID, listID)

		var inUse *domain.ErrListInUse
		require.True(t, errors.As(err, &inUse), "got %v", err)
		assert.Equal(t, listID, inUse.ListID)
		assert.Equal(t, "Newsletter", inUse.ListName)
		assert.Equal(t, []domain.AutomationRef{
			{ID: "a-email", Name: "Welcome series"},
			{ID: "a-trigger", Name: "Listener"},
			{ID: "a-adds", Name: "Adder"},
		}, inUse.Automations)
		assert.Equal(t, 3, inUse.AutomationCount)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("goes ahead when no live automation requires the list", func(t *testing.T) {
		repo, mock := setup(t)
		expectListLocked(mock)
		mock.ExpectQuery(liveAutomationsQuery).WithArgs(workspaceID).
			WillReturnRows(sqlmock.NewRows(columns).AddRow(reads...).AddRow(sendsNoEmail...).AddRow(otherList...))
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE contact_lists SET deleted_at = $1 WHERE list_id = $2 AND deleted_at IS NULL`)).
			WithArgs(sqlmock.AnyArg(), listID).
			WillReturnResult(sqlmock.NewResult(0, 3))
		mock.ExpectCommit()

		require.NoError(t, repo.DeleteList(context.Background(), workspaceID, listID))
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("a stored row it cannot read refuses the deletion", func(t *testing.T) {
		cases := []struct {
			name string
			row  []driver.Value
			want string
		}{
			{
				name: "trigger",
				row:  []driver.Value{"a-broken", "Broken", "", []byte(`["not", "a", "trigger"]`), []byte(`[]`)},
				want: "failed to read the trigger of automation a-broken",
			},
			{
				name: "nodes",
				row:  []driver.Value{"a-broken", "Broken", "", []byte(custom), []byte(`{"not": "a list of nodes"}`)},
				want: "failed to read the nodes of automation a-broken",
			},
			{
				name: "columns",
				row:  []driver.Value{"a-broken", nil, "", []byte(custom), []byte(`[]`)},
				want: "failed to scan live automation",
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				repo, mock := setup(t)
				expectListLocked(mock)
				mock.ExpectQuery(liveAutomationsQuery).WithArgs(workspaceID).
					WillReturnRows(sqlmock.NewRows(columns).AddRow(tc.row...))
				mock.ExpectRollback()

				err := repo.DeleteList(context.Background(), workspaceID, listID)
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.want)
				assert.NoError(t, mock.ExpectationsWereMet())
			})
		}
	})

	t.Run("a failed read refuses the deletion", func(t *testing.T) {
		repo, mock := setup(t)
		expectListLocked(mock)
		mock.ExpectQuery(liveAutomationsQuery).WithArgs(workspaceID).
			WillReturnError(errors.New("connection reset"))
		mock.ExpectRollback()

		err := repo.DeleteList(context.Background(), workspaceID, listID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read live automations")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("a read that breaks off refuses the deletion", func(t *testing.T) {
		repo, mock := setup(t)
		expectListLocked(mock)
		mock.ExpectQuery(liveAutomationsQuery).WithArgs(workspaceID).
			WillReturnRows(sqlmock.NewRows(columns).AddRow(reads...).RowError(0, errors.New("connection reset")))
		mock.ExpectRollback()

		err := repo.DeleteList(context.Background(), workspaceID, listID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "error iterating live automations")
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}
