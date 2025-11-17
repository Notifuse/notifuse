package repository

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
)

func TestBlogThemeRepository(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewBlogThemeRepository(mockWorkspaceRepo)

	db, sqlMock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	ctx := context.WithValue(context.Background(), "workspace_id", "workspace123")

	testTheme := &domain.BlogTheme{
		Version: 1,
		Files: domain.BlogThemeFiles{
			HomeLiquid:     "home template",
			CategoryLiquid: "category template",
			PostLiquid:     "post template",
			HeaderLiquid:   "header template",
			FooterLiquid:   "footer template",
			SharedLiquid:   "shared template",
		},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	publishedTime := time.Now().UTC()
	publishedTheme := &domain.BlogTheme{
		Version:     2,
		PublishedAt: &publishedTime,
		Files: domain.BlogThemeFiles{
			HomeLiquid: "published home",
		},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	t.Run("CreateTheme", func(t *testing.T) {
		t.Run("successful creation", func(t *testing.T) {
			mockWorkspaceRepo.EXPECT().
				GetConnection(gomock.Any(), "workspace123").
				Return(db, nil)

			sqlMock.ExpectBegin()
			// Expect version query
			sqlMock.ExpectQuery(regexp.QuoteMeta(`SELECT MAX(version) FROM blog_themes`)).
				WillReturnRows(sqlmock.NewRows([]string{"max"}).AddRow(0))
			// Expect insert
			sqlMock.ExpectExec(regexp.QuoteMeta(`INSERT INTO blog_themes`)).
				WithArgs(
					1, // auto-generated version
					sqlmock.AnyArg(), // published_at
					sqlmock.AnyArg(), // published_by_user_id
					sqlmock.AnyArg(), // files
					sqlmock.AnyArg(), // notes
					sqlmock.AnyArg(), // created_at
					sqlmock.AnyArg(), // updated_at
				).WillReturnResult(sqlmock.NewResult(1, 1))
			sqlMock.ExpectCommit()

			err := repo.CreateTheme(ctx, testTheme)
			require.NoError(t, err)
			assert.Equal(t, 1, testTheme.Version)
			assert.NoError(t, sqlMock.ExpectationsWereMet())
		})

		t.Run("auto-increment version from existing", func(t *testing.T) {
			mockWorkspaceRepo.EXPECT().
				GetConnection(gomock.Any(), "workspace123").
				Return(db, nil)

			sqlMock.ExpectBegin()
			// Expect version query returning existing max version
			sqlMock.ExpectQuery(regexp.QuoteMeta(`SELECT MAX(version) FROM blog_themes`)).
				WillReturnRows(sqlmock.NewRows([]string{"max"}).AddRow(5))
			// Expect insert with version 6
			sqlMock.ExpectExec(regexp.QuoteMeta(`INSERT INTO blog_themes`)).
				WithArgs(
					6, // auto-incremented
					sqlmock.AnyArg(), // published_at
					sqlmock.AnyArg(), // published_by_user_id
					sqlmock.AnyArg(), // files
					sqlmock.AnyArg(), // notes
					sqlmock.AnyArg(), // created_at
					sqlmock.AnyArg(), // updated_at
				).WillReturnResult(sqlmock.NewResult(1, 1))
			sqlMock.ExpectCommit()

			theme := &domain.BlogTheme{Files: domain.BlogThemeFiles{}}
			err := repo.CreateTheme(ctx, theme)
			require.NoError(t, err)
			assert.Equal(t, 6, theme.Version)
			assert.NoError(t, sqlMock.ExpectationsWereMet())
		})

		t.Run("workspace connection error", func(t *testing.T) {
			mockWorkspaceRepo.EXPECT().
				GetConnection(gomock.Any(), "workspace123").
				Return(nil, errors.New("connection error"))

			err := repo.CreateTheme(ctx, testTheme)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "failed to get workspace connection")
		})
	})

	t.Run("GetTheme", func(t *testing.T) {
		t.Run("theme found", func(t *testing.T) {
			mockWorkspaceRepo.EXPECT().
				GetConnection(gomock.Any(), "workspace123").
				Return(db, nil)

			rows := sqlmock.NewRows([]string{"version", "published_at", "published_by_user_id", "files", "notes", "created_at", "updated_at"}).
				AddRow(testTheme.Version, nil, nil, []byte(`{"home":"home template"}`), nil, testTheme.CreatedAt, testTheme.UpdatedAt)

			sqlMock.ExpectQuery(regexp.QuoteMeta(`SELECT version, published_at, published_by_user_id, files, notes, created_at, updated_at FROM blog_themes WHERE version = $1`)).
				WithArgs(1).
				WillReturnRows(rows)

			theme, err := repo.GetTheme(ctx, 1)
			require.NoError(t, err)
			assert.NotNil(t, theme)
			assert.Equal(t, 1, theme.Version)
			assert.NoError(t, sqlMock.ExpectationsWereMet())
		})

		t.Run("theme not found", func(t *testing.T) {
			mockWorkspaceRepo.EXPECT().
				GetConnection(gomock.Any(), "workspace123").
				Return(db, nil)

			sqlMock.ExpectQuery(regexp.QuoteMeta(`SELECT version, published_at, published_by_user_id, files, notes, created_at, updated_at FROM blog_themes WHERE version = $1`)).
				WithArgs(999).
				WillReturnError(sql.ErrNoRows)

			theme, err := repo.GetTheme(ctx, 999)
			require.Error(t, err)
			assert.Nil(t, theme)
			assert.Contains(t, err.Error(), "blog theme not found")
			assert.NoError(t, sqlMock.ExpectationsWereMet())
		})

		t.Run("workspace connection error", func(t *testing.T) {
			mockWorkspaceRepo.EXPECT().
				GetConnection(gomock.Any(), "workspace123").
				Return(nil, errors.New("connection error"))

			theme, err := repo.GetTheme(ctx, 1)
			require.Error(t, err)
			assert.Nil(t, theme)
			assert.Contains(t, err.Error(), "failed to get workspace connection")
		})
	})

	t.Run("GetPublishedTheme", func(t *testing.T) {
		t.Run("published theme found", func(t *testing.T) {
			mockWorkspaceRepo.EXPECT().
				GetConnection(gomock.Any(), "workspace123").
				Return(db, nil)

			userID := "user123"
			rows := sqlmock.NewRows([]string{"version", "published_at", "published_by_user_id", "files", "notes", "created_at", "updated_at"}).
				AddRow(publishedTheme.Version, publishedTheme.PublishedAt, userID, []byte(`{"home":"published home"}`), nil, publishedTheme.CreatedAt, publishedTheme.UpdatedAt)

			sqlMock.ExpectQuery(regexp.QuoteMeta(`SELECT version, published_at, published_by_user_id, files, notes, created_at, updated_at FROM blog_themes WHERE published_at IS NOT NULL`)).
				WillReturnRows(rows)

			theme, err := repo.GetPublishedTheme(ctx)
			require.NoError(t, err)
			assert.NotNil(t, theme)
			assert.Equal(t, 2, theme.Version)
			assert.NotNil(t, theme.PublishedAt)
			assert.NotNil(t, theme.PublishedByUserID)
			assert.Equal(t, userID, *theme.PublishedByUserID)
			assert.NoError(t, sqlMock.ExpectationsWereMet())
		})

		t.Run("no published theme", func(t *testing.T) {
			mockWorkspaceRepo.EXPECT().
				GetConnection(gomock.Any(), "workspace123").
				Return(db, nil)

			sqlMock.ExpectQuery(regexp.QuoteMeta(`SELECT version, published_at, published_by_user_id, files, notes, created_at, updated_at FROM blog_themes WHERE published_at IS NOT NULL`)).
				WillReturnError(sql.ErrNoRows)

			theme, err := repo.GetPublishedTheme(ctx)
			require.Error(t, err)
			assert.Nil(t, theme)
			assert.Contains(t, err.Error(), "no published blog theme found")
			assert.NoError(t, sqlMock.ExpectationsWereMet())
		})
	})

	t.Run("UpdateTheme", func(t *testing.T) {
		t.Run("successful update", func(t *testing.T) {
			mockWorkspaceRepo.EXPECT().
				GetConnection(gomock.Any(), "workspace123").
				Return(db, nil)

			sqlMock.ExpectBegin()
			sqlMock.ExpectExec(regexp.QuoteMeta(`UPDATE blog_themes SET files = $1, notes = $2, updated_at = $3 WHERE version = $4 AND published_at IS NULL`)).
				WithArgs(
					sqlmock.AnyArg(),
					sqlmock.AnyArg(),
					sqlmock.AnyArg(),
					testTheme.Version,
				).WillReturnResult(sqlmock.NewResult(0, 1))
			sqlMock.ExpectCommit()

			err := repo.UpdateTheme(ctx, testTheme)
			require.NoError(t, err)
			assert.NoError(t, sqlMock.ExpectationsWereMet())
		})

		t.Run("theme not found or already published", func(t *testing.T) {
			mockWorkspaceRepo.EXPECT().
				GetConnection(gomock.Any(), "workspace123").
				Return(db, nil)

			sqlMock.ExpectBegin()
			sqlMock.ExpectExec(regexp.QuoteMeta(`UPDATE blog_themes SET files = $1, notes = $2, updated_at = $3 WHERE version = $4 AND published_at IS NULL`)).
				WithArgs(
					sqlmock.AnyArg(),
					sqlmock.AnyArg(),
					sqlmock.AnyArg(),
					testTheme.Version,
				).WillReturnResult(sqlmock.NewResult(0, 0)) // 0 rows affected
			sqlMock.ExpectRollback()

			err := repo.UpdateTheme(ctx, testTheme)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "blog theme not found or already published")
			assert.NoError(t, sqlMock.ExpectationsWereMet())
		})
	})

	t.Run("PublishTheme", func(t *testing.T) {
		t.Run("successful publish", func(t *testing.T) {
			// Create new mock for this specific test to avoid expectation conflicts
			db2, sqlMock2, err := sqlmock.New()
			require.NoError(t, err)
			defer db2.Close()

			mockWorkspaceRepo.EXPECT().
				GetConnection(gomock.Any(), "workspace123").
				Return(db2, nil)

			userID := "user123"
			sqlMock2.ExpectBegin()
			// Expect GetThemeTx to verify theme exists
			rows := sqlmock.NewRows([]string{"version", "published_at", "published_by_user_id", "files", "notes", "created_at", "updated_at"}).
				AddRow(testTheme.Version, nil, nil, []byte(`{"home":"home"}`), nil, testTheme.CreatedAt, testTheme.UpdatedAt)
			sqlMock2.ExpectQuery(regexp.QuoteMeta(`SELECT version, published_at, published_by_user_id, files, notes, created_at, updated_at FROM blog_themes WHERE version = $1`)).
				WithArgs(1).
				WillReturnRows(rows)
			// Expect unpublish all
			sqlMock2.ExpectExec(regexp.QuoteMeta(`UPDATE blog_themes SET published_at = NULL, published_by_user_id = NULL, updated_at = $1 WHERE published_at IS NOT NULL`)).
				WithArgs(sqlmock.AnyArg()).
				WillReturnResult(sqlmock.NewResult(0, 1))
			// Expect publish target
			sqlMock2.ExpectExec(regexp.QuoteMeta(`UPDATE blog_themes SET published_at = $1, published_by_user_id = $2, updated_at = $3 WHERE version = $4`)).
				WithArgs(sqlmock.AnyArg(), userID, sqlmock.AnyArg(), 1).
				WillReturnResult(sqlmock.NewResult(0, 1))
			sqlMock2.ExpectCommit()

			err = repo.PublishTheme(ctx, 1, userID)
			require.NoError(t, err)
			assert.NoError(t, sqlMock2.ExpectationsWereMet())
		})

		t.Run("theme not found", func(t *testing.T) {
			// Create new mock for this specific test to avoid expectation conflicts
			db3, sqlMock3, err := sqlmock.New()
			require.NoError(t, err)
			defer db3.Close()

			mockWorkspaceRepo.EXPECT().
				GetConnection(gomock.Any(), "workspace123").
				Return(db3, nil)

			sqlMock3.ExpectBegin()
			// Expect GetThemeTx to fail
			sqlMock3.ExpectQuery(regexp.QuoteMeta(`SELECT version, published_at, published_by_user_id, files, notes, created_at, updated_at FROM blog_themes WHERE version = $1`)).
				WithArgs(999).
				WillReturnError(sql.ErrNoRows)
			sqlMock3.ExpectRollback()

			err = repo.PublishTheme(ctx, 999, "user123")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "blog theme not found")
			assert.NoError(t, sqlMock3.ExpectationsWereMet())
		})
	})

	t.Run("ListThemes", func(t *testing.T) {
		t.Run("successful list", func(t *testing.T) {
			// Create new mock for this specific test to avoid expectation conflicts
			db4, sqlMock4, err := sqlmock.New()
			require.NoError(t, err)
			defer db4.Close()

			mockWorkspaceRepo.EXPECT().
				GetConnection(gomock.Any(), "workspace123").
				Return(db4, nil)

			// Expect count query
			sqlMock4.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*) FROM blog_themes`)).
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))

			// Expect list query
			rows := sqlmock.NewRows([]string{"version", "published_at", "published_by_user_id", "files", "notes", "created_at", "updated_at"}).
				AddRow(2, publishedTime, "user123", []byte(`{"home":"v2"}`), nil, time.Now(), time.Now()).
				AddRow(1, nil, nil, []byte(`{"home":"v1"}`), nil, time.Now(), time.Now())

			sqlMock4.ExpectQuery(regexp.QuoteMeta(`SELECT version, published_at, published_by_user_id, files, notes, created_at, updated_at FROM blog_themes ORDER BY version DESC LIMIT $1 OFFSET $2`)).
				WithArgs(50, 0).
				WillReturnRows(rows)

			params := domain.ListBlogThemesRequest{Limit: 50, Offset: 0}
			response, err := repo.ListThemes(ctx, params)
			require.NoError(t, err)
			assert.NotNil(t, response)
			assert.Equal(t, 2, response.TotalCount)
			assert.Len(t, response.Themes, 2)
			assert.Equal(t, 2, response.Themes[0].Version) // Ordered by version DESC
			assert.Equal(t, 1, response.Themes[1].Version)
			assert.NoError(t, sqlMock4.ExpectationsWereMet())
		})

		t.Run("empty list", func(t *testing.T) {
			// Create new mock for this specific test
			db5, sqlMock5, err := sqlmock.New()
			require.NoError(t, err)
			defer db5.Close()

			mockWorkspaceRepo.EXPECT().
				GetConnection(gomock.Any(), "workspace123").
				Return(db5, nil)

			// Expect count query
			sqlMock5.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*) FROM blog_themes`)).
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

			// Expect list query
			rows := sqlmock.NewRows([]string{"version", "published_at", "published_by_user_id", "files", "notes", "created_at", "updated_at"})

			sqlMock5.ExpectQuery(regexp.QuoteMeta(`SELECT version, published_at, published_by_user_id, files, notes, created_at, updated_at FROM blog_themes ORDER BY version DESC LIMIT $1 OFFSET $2`)).
				WithArgs(50, 0).
				WillReturnRows(rows)

			params := domain.ListBlogThemesRequest{Limit: 50, Offset: 0}
			response, err := repo.ListThemes(ctx, params)
			require.NoError(t, err)
			assert.NotNil(t, response)
			assert.Equal(t, 0, response.TotalCount)
			assert.Len(t, response.Themes, 0)
			assert.NoError(t, sqlMock5.ExpectationsWereMet())
		})

		t.Run("with pagination", func(t *testing.T) {
			// Create new mock for this specific test
			db6, sqlMock6, err := sqlmock.New()
			require.NoError(t, err)
			defer db6.Close()

			mockWorkspaceRepo.EXPECT().
				GetConnection(gomock.Any(), "workspace123").
				Return(db6, nil)

			// Expect count query
			sqlMock6.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*) FROM blog_themes`)).
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(10))

			// Expect list query with offset
			rows := sqlmock.NewRows([]string{"version", "published_at", "published_by_user_id", "files", "notes", "created_at", "updated_at"}).
				AddRow(3, nil, nil, []byte(`{"home":"v3"}`), nil, time.Now(), time.Now())

			sqlMock6.ExpectQuery(regexp.QuoteMeta(`SELECT version, published_at, published_by_user_id, files, notes, created_at, updated_at FROM blog_themes ORDER BY version DESC LIMIT $1 OFFSET $2`)).
				WithArgs(10, 20).
				WillReturnRows(rows)

			params := domain.ListBlogThemesRequest{Limit: 10, Offset: 20}
			response, err := repo.ListThemes(ctx, params)
			require.NoError(t, err)
			assert.NotNil(t, response)
			assert.Equal(t, 10, response.TotalCount)
			assert.Len(t, response.Themes, 1)
			assert.NoError(t, sqlMock6.ExpectationsWereMet())
		})
	})
}

func TestBlogThemeRepository_MissingWorkspaceID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewBlogThemeRepository(mockWorkspaceRepo)

	// Context without workspace_id
	ctx := context.Background()

	testTheme := &domain.BlogTheme{
		Version: 1,
		Files:   domain.BlogThemeFiles{},
	}

	t.Run("CreateTheme without workspace_id", func(t *testing.T) {
		err := repo.CreateTheme(ctx, testTheme)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "workspace_id not found in context")
	})

	t.Run("GetTheme without workspace_id", func(t *testing.T) {
		theme, err := repo.GetTheme(ctx, 1)
		require.Error(t, err)
		assert.Nil(t, theme)
		assert.Contains(t, err.Error(), "workspace_id not found in context")
	})

	t.Run("GetPublishedTheme without workspace_id", func(t *testing.T) {
		theme, err := repo.GetPublishedTheme(ctx)
		require.Error(t, err)
		assert.Nil(t, theme)
		assert.Contains(t, err.Error(), "workspace_id not found in context")
	})

	t.Run("UpdateTheme without workspace_id", func(t *testing.T) {
		err := repo.UpdateTheme(ctx, testTheme)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "workspace_id not found in context")
	})

	t.Run("PublishTheme without workspace_id", func(t *testing.T) {
		err := repo.PublishTheme(ctx, 1, "user123")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "workspace_id not found in context")
	})

	t.Run("ListThemes without workspace_id", func(t *testing.T) {
		params := domain.ListBlogThemesRequest{Limit: 50}
		response, err := repo.ListThemes(ctx, params)
		require.Error(t, err)
		assert.Nil(t, response)
		assert.Contains(t, err.Error(), "workspace_id not found in context")
	})
}

