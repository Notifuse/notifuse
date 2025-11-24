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

func TestBlogCategoryRepository(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewBlogCategoryRepository(mockWorkspaceRepo)

	db, sqlMock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	ctx := context.WithValue(context.Background(), domain.WorkspaceIDKey, "workspace123")

	testCategory := &domain.BlogCategory{
		ID:   "cat123",
		Slug: "tech-blog",
		Settings: domain.BlogCategorySettings{
			Name:        "Tech Blog",
			Description: "Technology articles",
			SEO: &domain.SEOSettings{
				MetaTitle: "Tech Blog",
			},
		},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	t.Run("CreateCategory", func(t *testing.T) {
		t.Run("successful creation", func(t *testing.T) {
			mockWorkspaceRepo.EXPECT().
				GetConnection(gomock.Any(), "workspace123").
				Return(db, nil)

			sqlMock.ExpectBegin()
			sqlMock.ExpectExec(regexp.QuoteMeta(`
		INSERT INTO blog_categories (
			id, slug, settings, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5)
	`)).WithArgs(
				testCategory.ID,
				testCategory.Slug,
				sqlmock.AnyArg(),
				sqlmock.AnyArg(),
				sqlmock.AnyArg(),
			).WillReturnResult(sqlmock.NewResult(1, 1))
			sqlMock.ExpectCommit()

			err := repo.CreateCategory(ctx, testCategory)
			require.NoError(t, err)
			assert.NoError(t, sqlMock.ExpectationsWereMet())
		})

		t.Run("workspace connection error", func(t *testing.T) {
			mockWorkspaceRepo.EXPECT().
				GetConnection(gomock.Any(), "workspace123").
				Return(nil, errors.New("connection error"))

			err := repo.CreateCategory(ctx, testCategory)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "failed to get workspace connection")
		})

		t.Run("database error", func(t *testing.T) {
			mockWorkspaceRepo.EXPECT().
				GetConnection(gomock.Any(), "workspace123").
				Return(db, nil)

			sqlMock.ExpectBegin()
			sqlMock.ExpectExec(regexp.QuoteMeta(`INSERT INTO blog_categories`)).
				WillReturnError(errors.New("database error"))
			sqlMock.ExpectRollback()

			err := repo.CreateCategory(ctx, testCategory)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "failed to create blog category")
		})
	})

	t.Run("GetCategory", func(t *testing.T) {
		t.Run("category found", func(t *testing.T) {
			mockWorkspaceRepo.EXPECT().
				GetConnection(gomock.Any(), "workspace123").
				Return(db, nil)

			rows := sqlmock.NewRows([]string{
				"id", "slug", "settings", "created_at", "updated_at", "deleted_at",
			}).AddRow(
				testCategory.ID,
				testCategory.Slug,
				[]byte(`{"name":"Tech Blog","description":"Technology articles"}`),
				testCategory.CreatedAt,
				testCategory.UpdatedAt,
				nil,
			)

			sqlMock.ExpectQuery(regexp.QuoteMeta(`
		SELECT id, slug, settings, created_at, updated_at, deleted_at
		FROM blog_categories
		WHERE id = $1 AND deleted_at IS NULL
	`)).WithArgs(testCategory.ID).WillReturnRows(rows)

			category, err := repo.GetCategory(ctx, testCategory.ID)
			require.NoError(t, err)
			assert.Equal(t, testCategory.ID, category.ID)
			assert.Equal(t, testCategory.Slug, category.Slug)
			assert.NoError(t, sqlMock.ExpectationsWereMet())
		})

		t.Run("category not found", func(t *testing.T) {
			mockWorkspaceRepo.EXPECT().
				GetConnection(gomock.Any(), "workspace123").
				Return(db, nil)

			sqlMock.ExpectQuery(regexp.QuoteMeta(`SELECT id, slug, settings`)).
				WithArgs("nonexistent").
				WillReturnError(sql.ErrNoRows)

			category, err := repo.GetCategory(ctx, "nonexistent")
			require.Error(t, err)
			assert.Nil(t, category)
			assert.Contains(t, err.Error(), "blog category not found")
		})
	})

	t.Run("GetCategoryBySlug", func(t *testing.T) {
		t.Run("category found", func(t *testing.T) {
			mockWorkspaceRepo.EXPECT().
				GetConnection(gomock.Any(), "workspace123").
				Return(db, nil)

			rows := sqlmock.NewRows([]string{
				"id", "slug", "settings", "created_at", "updated_at", "deleted_at",
			}).AddRow(
				testCategory.ID,
				testCategory.Slug,
				[]byte(`{"name":"Tech Blog"}`),
				testCategory.CreatedAt,
				testCategory.UpdatedAt,
				nil,
			)

			sqlMock.ExpectQuery(regexp.QuoteMeta(`WHERE slug = $1`)).
				WithArgs(testCategory.Slug).
				WillReturnRows(rows)

			category, err := repo.GetCategoryBySlug(ctx, testCategory.Slug)
			require.NoError(t, err)
			assert.Equal(t, testCategory.Slug, category.Slug)
		})
	})

	t.Run("UpdateCategory", func(t *testing.T) {
		t.Run("successful update", func(t *testing.T) {
			mockWorkspaceRepo.EXPECT().
				GetConnection(gomock.Any(), "workspace123").
				Return(db, nil)

			sqlMock.ExpectBegin()
			sqlMock.ExpectExec(regexp.QuoteMeta(`
		UPDATE blog_categories
		SET slug = $1, settings = $2, updated_at = $3
		WHERE id = $4 AND deleted_at IS NULL
	`)).WithArgs(
				testCategory.Slug,
				sqlmock.AnyArg(),
				sqlmock.AnyArg(),
				testCategory.ID,
			).WillReturnResult(sqlmock.NewResult(0, 1))
			sqlMock.ExpectCommit()

			err := repo.UpdateCategory(ctx, testCategory)
			require.NoError(t, err)
			assert.NoError(t, sqlMock.ExpectationsWereMet())
		})

		t.Run("category not found", func(t *testing.T) {
			mockWorkspaceRepo.EXPECT().
				GetConnection(gomock.Any(), "workspace123").
				Return(db, nil)

			sqlMock.ExpectBegin()
			sqlMock.ExpectExec(regexp.QuoteMeta(`UPDATE blog_categories`)).
				WillReturnResult(sqlmock.NewResult(0, 0))
			sqlMock.ExpectRollback()

			err := repo.UpdateCategory(ctx, testCategory)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "blog category not found")
		})
	})

	t.Run("DeleteCategory", func(t *testing.T) {
		t.Run("successful deletion", func(t *testing.T) {
			mockWorkspaceRepo.EXPECT().
				GetConnection(gomock.Any(), "workspace123").
				Return(db, nil)

			sqlMock.ExpectBegin()
			sqlMock.ExpectExec(regexp.QuoteMeta(`
		UPDATE blog_categories
		SET deleted_at = $1
		WHERE id = $2 AND deleted_at IS NULL
	`)).WithArgs(sqlmock.AnyArg(), testCategory.ID).
				WillReturnResult(sqlmock.NewResult(0, 1))
			sqlMock.ExpectCommit()

			err := repo.DeleteCategory(ctx, testCategory.ID)
			require.NoError(t, err)
			assert.NoError(t, sqlMock.ExpectationsWereMet())
		})

		t.Run("category not found", func(t *testing.T) {
			mockWorkspaceRepo.EXPECT().
				GetConnection(gomock.Any(), "workspace123").
				Return(db, nil)

			sqlMock.ExpectBegin()
			sqlMock.ExpectExec(regexp.QuoteMeta(`UPDATE blog_categories`)).
				WillReturnResult(sqlmock.NewResult(0, 0))
			sqlMock.ExpectRollback()

			err := repo.DeleteCategory(ctx, "nonexistent")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "blog category not found")
		})
	})

	t.Run("GetCategoriesByIDs", func(t *testing.T) {
		t.Run("successful retrieval", func(t *testing.T) {
			mockWorkspaceRepo.EXPECT().
				GetConnection(gomock.Any(), "workspace123").
				Return(db, nil)

			categoryIDs := []string{"cat123", "cat456"}
			testCategory2 := &domain.BlogCategory{
				ID:   "cat456",
				Slug: "another-blog",
				Settings: domain.BlogCategorySettings{
					Name: "Another Blog",
				},
				CreatedAt: time.Now().UTC(),
				UpdatedAt: time.Now().UTC(),
			}

			rows := sqlmock.NewRows([]string{
				"id", "slug", "settings", "created_at", "updated_at", "deleted_at",
			}).AddRow(
				testCategory.ID,
				testCategory.Slug,
				[]byte(`{"name":"Tech Blog"}`),
				testCategory.CreatedAt,
				testCategory.UpdatedAt,
				nil,
			).AddRow(
				testCategory2.ID,
				testCategory2.Slug,
				[]byte(`{"name":"Another Blog"}`),
				testCategory2.CreatedAt,
				testCategory2.UpdatedAt,
				nil,
			)

			sqlMock.ExpectQuery(regexp.QuoteMeta(`WHERE id IN ($1, $2)`)).
				WithArgs("cat123", "cat456").
				WillReturnRows(rows)

			categories, err := repo.GetCategoriesByIDs(ctx, categoryIDs)
			require.NoError(t, err)
			require.Len(t, categories, 2)
			assert.Equal(t, testCategory.Slug, categories[0].Slug)
			assert.Equal(t, testCategory2.Slug, categories[1].Slug)
			assert.NoError(t, sqlMock.ExpectationsWereMet())
		})

		t.Run("empty IDs list", func(t *testing.T) {
			categories, err := repo.GetCategoriesByIDs(ctx, []string{})
			require.NoError(t, err)
			assert.Empty(t, categories)
		})

		t.Run("includes deleted categories", func(t *testing.T) {
			mockWorkspaceRepo.EXPECT().
				GetConnection(gomock.Any(), "workspace123").
				Return(db, nil)

			deletedAt := time.Now().UTC()
			categoryIDs := []string{"cat123"}

			rows := sqlmock.NewRows([]string{
				"id", "slug", "settings", "created_at", "updated_at", "deleted_at",
			}).AddRow(
				testCategory.ID,
				testCategory.Slug,
				[]byte(`{"name":"Tech Blog"}`),
				testCategory.CreatedAt,
				testCategory.UpdatedAt,
				deletedAt, // deleted category
			)

			sqlMock.ExpectQuery(regexp.QuoteMeta(`WHERE id IN ($1)`)).
				WithArgs("cat123").
				WillReturnRows(rows)

			categories, err := repo.GetCategoriesByIDs(ctx, categoryIDs)
			require.NoError(t, err)
			require.Len(t, categories, 1)
			assert.Equal(t, testCategory.Slug, categories[0].Slug)
			assert.NotNil(t, categories[0].DeletedAt)
			assert.NoError(t, sqlMock.ExpectationsWereMet())
		})

		t.Run("workspace connection error", func(t *testing.T) {
			mockWorkspaceRepo.EXPECT().
				GetConnection(gomock.Any(), "workspace123").
				Return(nil, errors.New("connection failed"))

			categories, err := repo.GetCategoriesByIDs(ctx, []string{"cat123"})
			assert.Error(t, err)
			assert.Nil(t, categories)
		})
	})

	t.Run("ListCategories", func(t *testing.T) {
		t.Run("successful retrieval", func(t *testing.T) {
			mockWorkspaceRepo.EXPECT().
				GetConnection(gomock.Any(), "workspace123").
				Return(db, nil)

			rows := sqlmock.NewRows([]string{
				"id", "slug", "settings", "created_at", "updated_at", "deleted_at",
			}).AddRow(
				testCategory.ID,
				testCategory.Slug,
				[]byte(`{"name":"Tech Blog"}`),
				testCategory.CreatedAt,
				testCategory.UpdatedAt,
				nil,
			).AddRow(
				"cat456",
				"news",
				[]byte(`{"name":"News"}`),
				time.Now().UTC(),
				time.Now().UTC(),
				nil,
			)

			sqlMock.ExpectQuery(regexp.QuoteMeta(`
		SELECT id, slug, settings, created_at, updated_at, deleted_at
		FROM blog_categories
		WHERE deleted_at IS NULL
		ORDER BY created_at DESC
	`)).WillReturnRows(rows)

			categories, err := repo.ListCategories(ctx)
			require.NoError(t, err)
			assert.Len(t, categories, 2)
			assert.NoError(t, sqlMock.ExpectationsWereMet())
		})

		t.Run("no categories", func(t *testing.T) {
			mockWorkspaceRepo.EXPECT().
				GetConnection(gomock.Any(), "workspace123").
				Return(db, nil)

			rows := sqlmock.NewRows([]string{
				"id", "slug", "settings", "created_at", "updated_at", "deleted_at",
			})

			sqlMock.ExpectQuery(regexp.QuoteMeta(`SELECT id, slug, settings`)).
				WillReturnRows(rows)

			categories, err := repo.ListCategories(ctx)
			require.NoError(t, err)
			assert.Empty(t, categories)
		})
	})
}

func TestBlogPostRepository(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewBlogPostRepository(mockWorkspaceRepo)

	db, sqlMock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	ctx := context.WithValue(context.Background(), domain.WorkspaceIDKey, "workspace123")

	testPost := &domain.BlogPost{
		ID:         "post123",
		CategoryID: "cat123",
		Slug:       "my-first-post",
		Settings: domain.BlogPostSettings{
			Title: "My First Post",
			Template: domain.BlogPostTemplateReference{
				TemplateID:      "tpl123",
				TemplateVersion: 1,
			},
			Authors:            []domain.BlogAuthor{{Name: "John Doe"}},
			ReadingTimeMinutes: 5,
		},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	t.Run("CreatePost", func(t *testing.T) {
		t.Run("successful creation", func(t *testing.T) {
			mockWorkspaceRepo.EXPECT().
				GetConnection(gomock.Any(), "workspace123").
				Return(db, nil)

			sqlMock.ExpectBegin()
			sqlMock.ExpectExec(regexp.QuoteMeta(`
		INSERT INTO blog_posts (
			id, category_id, slug, settings, published_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
	`)).WithArgs(
				testPost.ID,
				testPost.CategoryID,
				testPost.Slug,
				sqlmock.AnyArg(),
				testPost.PublishedAt,
				sqlmock.AnyArg(),
				sqlmock.AnyArg(),
			).WillReturnResult(sqlmock.NewResult(1, 1))
			sqlMock.ExpectCommit()

			err := repo.CreatePost(ctx, testPost)
			require.NoError(t, err)
			assert.NoError(t, sqlMock.ExpectationsWereMet())
		})

		t.Run("database error", func(t *testing.T) {
			mockWorkspaceRepo.EXPECT().
				GetConnection(gomock.Any(), "workspace123").
				Return(db, nil)

			sqlMock.ExpectBegin()
			sqlMock.ExpectExec(regexp.QuoteMeta(`INSERT INTO blog_posts`)).
				WillReturnError(errors.New("database error"))
			sqlMock.ExpectRollback()

			err := repo.CreatePost(ctx, testPost)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "failed to create blog post")
		})
	})

	t.Run("GetPost", func(t *testing.T) {
		t.Run("post found", func(t *testing.T) {
			mockWorkspaceRepo.EXPECT().
				GetConnection(gomock.Any(), "workspace123").
				Return(db, nil)

			rows := sqlmock.NewRows([]string{
				"id", "category_id", "slug", "settings", "published_at", "created_at", "updated_at", "deleted_at",
			}).AddRow(
				testPost.ID,
				testPost.CategoryID,
				testPost.Slug,
				[]byte(`{"title":"My First Post","template":{"template_id":"tpl123","template_version":1,"template_data":{}},"authors":[],"reading_time_minutes":0}`),
				testPost.PublishedAt,
				testPost.CreatedAt,
				testPost.UpdatedAt,
				nil,
			)

			sqlMock.ExpectQuery(regexp.QuoteMeta(`
		SELECT id, category_id, slug, settings, published_at, created_at, updated_at, deleted_at
		FROM blog_posts
		WHERE id = $1 AND deleted_at IS NULL
	`)).WithArgs(testPost.ID).WillReturnRows(rows)

			post, err := repo.GetPost(ctx, testPost.ID)
			require.NoError(t, err)
			assert.Equal(t, testPost.ID, post.ID)
			assert.Equal(t, testPost.Slug, post.Slug)
			assert.NoError(t, sqlMock.ExpectationsWereMet())
		})

		t.Run("post not found", func(t *testing.T) {
			mockWorkspaceRepo.EXPECT().
				GetConnection(gomock.Any(), "workspace123").
				Return(db, nil)

			sqlMock.ExpectQuery(regexp.QuoteMeta(`SELECT id, category_id`)).
				WithArgs("nonexistent").
				WillReturnError(sql.ErrNoRows)

			post, err := repo.GetPost(ctx, "nonexistent")
			require.Error(t, err)
			assert.Nil(t, post)
			assert.Contains(t, err.Error(), "blog post not found")
		})
	})

	t.Run("GetPostBySlug", func(t *testing.T) {
		t.Run("post found", func(t *testing.T) {
			mockWorkspaceRepo.EXPECT().
				GetConnection(gomock.Any(), "workspace123").
				Return(db, nil)

			rows := sqlmock.NewRows([]string{
				"id", "category_id", "slug", "settings", "published_at", "created_at", "updated_at", "deleted_at",
			}).AddRow(
				testPost.ID,
				testPost.CategoryID,
				testPost.Slug,
				[]byte(`{"title":"My First Post","template":{"template_id":"tpl123","template_version":1,"template_data":{}},"authors":[],"reading_time_minutes":0}`),
				testPost.PublishedAt,
				testPost.CreatedAt,
				testPost.UpdatedAt,
				nil,
			)

			sqlMock.ExpectQuery(regexp.QuoteMeta(`WHERE slug = $1`)).
				WithArgs(testPost.Slug).
				WillReturnRows(rows)

			post, err := repo.GetPostBySlug(ctx, testPost.Slug)
			require.NoError(t, err)
			assert.Equal(t, testPost.Slug, post.Slug)
		})
	})

	t.Run("GetPostByCategoryAndSlug", func(t *testing.T) {
		t.Run("post found", func(t *testing.T) {
			mockWorkspaceRepo.EXPECT().
				GetConnection(gomock.Any(), "workspace123").
				Return(db, nil)

			rows := sqlmock.NewRows([]string{
				"id", "category_id", "slug", "settings", "published_at", "created_at", "updated_at", "deleted_at",
			}).AddRow(
				testPost.ID,
				testPost.CategoryID,
				testPost.Slug,
				[]byte(`{"title":"My First Post","template":{"template_id":"tpl123","template_version":1,"template_data":{}},"authors":[],"reading_time_minutes":0}`),
				testPost.PublishedAt,
				testPost.CreatedAt,
				testPost.UpdatedAt,
				nil,
			)

			sqlMock.ExpectQuery(regexp.QuoteMeta(`FROM blog_posts p`)).
				WithArgs("tech", "my-first-post").
				WillReturnRows(rows)

			post, err := repo.GetPostByCategoryAndSlug(ctx, "tech", "my-first-post")
			require.NoError(t, err)
			assert.Equal(t, testPost.Slug, post.Slug)
		})
	})

	t.Run("UpdatePost", func(t *testing.T) {
		t.Run("successful update", func(t *testing.T) {
			mockWorkspaceRepo.EXPECT().
				GetConnection(gomock.Any(), "workspace123").
				Return(db, nil)

			sqlMock.ExpectBegin()
			sqlMock.ExpectExec(regexp.QuoteMeta(`
		UPDATE blog_posts
		SET category_id = $1, slug = $2, settings = $3, published_at = $4, updated_at = $5
		WHERE id = $6 AND deleted_at IS NULL
	`)).WithArgs(
				testPost.CategoryID,
				testPost.Slug,
				sqlmock.AnyArg(),
				testPost.PublishedAt,
				sqlmock.AnyArg(),
				testPost.ID,
			).WillReturnResult(sqlmock.NewResult(0, 1))
			sqlMock.ExpectCommit()

			err := repo.UpdatePost(ctx, testPost)
			require.NoError(t, err)
			assert.NoError(t, sqlMock.ExpectationsWereMet())
		})

		t.Run("post not found", func(t *testing.T) {
			mockWorkspaceRepo.EXPECT().
				GetConnection(gomock.Any(), "workspace123").
				Return(db, nil)

			sqlMock.ExpectBegin()
			sqlMock.ExpectExec(regexp.QuoteMeta(`UPDATE blog_posts`)).
				WillReturnResult(sqlmock.NewResult(0, 0))
			sqlMock.ExpectRollback()

			err := repo.UpdatePost(ctx, testPost)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "blog post not found")
		})
	})

	t.Run("DeletePost", func(t *testing.T) {
		t.Run("successful deletion", func(t *testing.T) {
			mockWorkspaceRepo.EXPECT().
				GetConnection(gomock.Any(), "workspace123").
				Return(db, nil)

			sqlMock.ExpectBegin()
			sqlMock.ExpectExec(regexp.QuoteMeta(`
		UPDATE blog_posts
		SET deleted_at = $1
		WHERE id = $2 AND deleted_at IS NULL
	`)).WithArgs(sqlmock.AnyArg(), testPost.ID).
				WillReturnResult(sqlmock.NewResult(0, 1))
			sqlMock.ExpectCommit()

			err := repo.DeletePost(ctx, testPost.ID)
			require.NoError(t, err)
			assert.NoError(t, sqlMock.ExpectationsWereMet())
		})
	})

	t.Run("ListPosts", func(t *testing.T) {
		t.Run("list all posts", func(t *testing.T) {
			// Create new mocks for this test
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			testMockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
			testRepo := NewBlogPostRepository(testMockWorkspaceRepo)
			testDB, testSqlMock, err := sqlmock.New()
			require.NoError(t, err)
			defer func() { _ = testDB.Close() }()

			testMockWorkspaceRepo.EXPECT().
				GetConnection(gomock.Any(), "workspace123").
				Return(testDB, nil)

			// Count query
			countRows := sqlmock.NewRows([]string{"count"}).AddRow(2)
			testSqlMock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*)`)).
				WillReturnRows(countRows)

			// List query
			rows := sqlmock.NewRows([]string{
				"id", "category_id", "slug", "settings", "published_at", "created_at", "updated_at", "deleted_at",
			}).AddRow(
				testPost.ID,
				testPost.CategoryID,
				testPost.Slug,
				[]byte(`{"title":"My First Post","template":{"template_id":"tpl123","template_version":1,"template_data":{}},"authors":[],"reading_time_minutes":0}`),
				testPost.PublishedAt,
				testPost.CreatedAt,
				testPost.UpdatedAt,
				nil,
			).AddRow(
				"post456",
				testPost.CategoryID,
				"second-post",
				[]byte(`{"title":"Second Post","template":{"template_id":"tpl123","template_version":1,"template_data":{}},"authors":[],"reading_time_minutes":0}`),
				nil,
				time.Now().UTC(),
				time.Now().UTC(),
				nil,
			)

			testSqlMock.ExpectQuery(regexp.QuoteMeta(`SELECT id, category_id, slug, settings`)).
				WillReturnRows(rows)

			params := domain.ListBlogPostsRequest{
				Status: domain.BlogPostStatusAll,
				Limit:  50,
				Offset: 0,
			}
			result, err := testRepo.ListPosts(ctx, params)
			require.NoError(t, err)
			assert.Equal(t, 2, result.TotalCount)
			assert.Len(t, result.Posts, 2)
			assert.NoError(t, testSqlMock.ExpectationsWereMet())
		})

		t.Run("filter by category", func(t *testing.T) {
			// Create new mocks for this test
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			testMockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
			testRepo := NewBlogPostRepository(testMockWorkspaceRepo)
			testDB, testSqlMock, err := sqlmock.New()
			require.NoError(t, err)
			defer func() { _ = testDB.Close() }()

			testMockWorkspaceRepo.EXPECT().
				GetConnection(gomock.Any(), "workspace123").
				Return(testDB, nil)

			countRows := sqlmock.NewRows([]string{"count"}).AddRow(1)
			testSqlMock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*)`)).
				WithArgs("cat123").
				WillReturnRows(countRows)

			rows := sqlmock.NewRows([]string{
				"id", "category_id", "slug", "settings", "published_at", "created_at", "updated_at", "deleted_at",
			}).AddRow(
				testPost.ID,
				testPost.CategoryID,
				testPost.Slug,
				[]byte(`{"title":"My First Post","template":{"template_id":"tpl123","template_version":1,"template_data":{}},"authors":[],"reading_time_minutes":0}`),
				testPost.PublishedAt,
				testPost.CreatedAt,
				testPost.UpdatedAt,
				nil,
			)

			testSqlMock.ExpectQuery(regexp.QuoteMeta(`SELECT id, category_id`)).
				WithArgs("cat123", 50, 0).
				WillReturnRows(rows)

			params := domain.ListBlogPostsRequest{
				CategoryID: "cat123",
				Status:     domain.BlogPostStatusAll,
				Limit:      50,
				Offset:     0,
			}
			result, err := testRepo.ListPosts(ctx, params)
			require.NoError(t, err)
			assert.Equal(t, 1, result.TotalCount)
			assert.NoError(t, testSqlMock.ExpectationsWereMet())
		})

		t.Run("includes pagination metadata", func(t *testing.T) {
			// Create new mocks for this test
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			testMockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
			testRepo := NewBlogPostRepository(testMockWorkspaceRepo)
			testDB, testSqlMock, err := sqlmock.New()
			require.NoError(t, err)
			defer func() { _ = testDB.Close() }()

			testMockWorkspaceRepo.EXPECT().
				GetConnection(gomock.Any(), "workspace123").
				Return(testDB, nil)

			// Total count: 25 posts
			countRows := sqlmock.NewRows([]string{"count"}).AddRow(25)
			testSqlMock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*)`)).
				WillReturnRows(countRows)

			// List query - page 2 with 10 per page
			rows := sqlmock.NewRows([]string{
				"id", "category_id", "slug", "settings", "published_at", "created_at", "updated_at", "deleted_at",
			}).AddRow(
				"post1",
				"cat123",
				"post1",
				[]byte(`{"title":"Post 1","template":{"template_id":"tpl123","template_version":1,"template_data":{}},"authors":[],"reading_time_minutes":0}`),
				nil,
				time.Now().UTC(),
				time.Now().UTC(),
				nil,
			)

			testSqlMock.ExpectQuery(regexp.QuoteMeta(`SELECT id, category_id, slug, settings`)).
				WillReturnRows(rows)

			params := domain.ListBlogPostsRequest{
				Page:   2,
				Limit:  10,
				Offset: 10,
			}
			result, err := testRepo.ListPosts(ctx, params)
			require.NoError(t, err)

			// Check pagination metadata
			assert.Equal(t, 25, result.TotalCount)
			assert.Equal(t, 2, result.CurrentPage)
			assert.Equal(t, 3, result.TotalPages)         // ceiling(25/10) = 3
			assert.Equal(t, true, result.HasNextPage)     // page 2 of 3
			assert.Equal(t, true, result.HasPreviousPage) // page 2 > 1
			assert.NoError(t, testSqlMock.ExpectationsWereMet())
		})

		t.Run("filter by status - published only", func(t *testing.T) {
			// Create new mocks for this test
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			testMockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
			testRepo := NewBlogPostRepository(testMockWorkspaceRepo)
			testDB, testSqlMock, err := sqlmock.New()
			require.NoError(t, err)
			defer func() { _ = testDB.Close() }()

			testMockWorkspaceRepo.EXPECT().
				GetConnection(gomock.Any(), "workspace123").
				Return(testDB, nil)

			countRows := sqlmock.NewRows([]string{"count"}).AddRow(1)
			testSqlMock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*)`)).
				WillReturnRows(countRows)

			now := time.Now().UTC()
			rows := sqlmock.NewRows([]string{
				"id", "category_id", "slug", "settings", "published_at", "created_at", "updated_at", "deleted_at",
			}).AddRow(
				testPost.ID,
				testPost.CategoryID,
				testPost.Slug,
				[]byte(`{"title":"My First Post","template":{"template_id":"tpl123","template_version":1,"template_data":{}},"authors":[],"reading_time_minutes":0}`),
				&now,
				testPost.CreatedAt,
				testPost.UpdatedAt,
				nil,
			)

			testSqlMock.ExpectQuery(regexp.QuoteMeta(`SELECT id, category_id`)).
				WillReturnRows(rows)

			params := domain.ListBlogPostsRequest{
				Status: domain.BlogPostStatusPublished,
				Limit:  50,
				Offset: 0,
			}
			result, err := testRepo.ListPosts(ctx, params)
			require.NoError(t, err)
			assert.Equal(t, 1, result.TotalCount)
			assert.NoError(t, testSqlMock.ExpectationsWereMet())
		})
	})

	t.Run("PublishPost", func(t *testing.T) {
		t.Run("successful publish", func(t *testing.T) {
			mockWorkspaceRepo.EXPECT().
				GetConnection(gomock.Any(), "workspace123").
				Return(db, nil)

			sqlMock.ExpectBegin()
			sqlMock.ExpectExec(regexp.QuoteMeta(`
		UPDATE blog_posts
		SET published_at = $1, updated_at = $2
		WHERE id = $3 AND deleted_at IS NULL AND published_at IS NULL
	`)).WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), testPost.ID).
				WillReturnResult(sqlmock.NewResult(0, 1))
			sqlMock.ExpectCommit()

			err := repo.PublishPost(ctx, testPost.ID)
			require.NoError(t, err)
			assert.NoError(t, sqlMock.ExpectationsWereMet())
		})

		t.Run("already published", func(t *testing.T) {
			mockWorkspaceRepo.EXPECT().
				GetConnection(gomock.Any(), "workspace123").
				Return(db, nil)

			sqlMock.ExpectBegin()
			sqlMock.ExpectExec(regexp.QuoteMeta(`UPDATE blog_posts`)).
				WillReturnResult(sqlmock.NewResult(0, 0))
			sqlMock.ExpectRollback()

			err := repo.PublishPost(ctx, testPost.ID)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "not found or already published")
		})
	})

	t.Run("UnpublishPost", func(t *testing.T) {
		t.Run("successful unpublish", func(t *testing.T) {
			mockWorkspaceRepo.EXPECT().
				GetConnection(gomock.Any(), "workspace123").
				Return(db, nil)

			sqlMock.ExpectBegin()
			sqlMock.ExpectExec(regexp.QuoteMeta(`
		UPDATE blog_posts
		SET published_at = NULL, updated_at = $1
		WHERE id = $2 AND deleted_at IS NULL AND published_at IS NOT NULL
	`)).WithArgs(sqlmock.AnyArg(), testPost.ID).
				WillReturnResult(sqlmock.NewResult(0, 1))
			sqlMock.ExpectCommit()

			err := repo.UnpublishPost(ctx, testPost.ID)
			require.NoError(t, err)
			assert.NoError(t, sqlMock.ExpectationsWereMet())
		})

		t.Run("not published", func(t *testing.T) {
			mockWorkspaceRepo.EXPECT().
				GetConnection(gomock.Any(), "workspace123").
				Return(db, nil)

			sqlMock.ExpectBegin()
			sqlMock.ExpectExec(regexp.QuoteMeta(`UPDATE blog_posts`)).
				WillReturnResult(sqlmock.NewResult(0, 0))
			sqlMock.ExpectRollback()

			err := repo.UnpublishPost(ctx, testPost.ID)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "not found or not published")
		})
	})

	t.Run("DeletePostsByCategoryIDTx", func(t *testing.T) {
		t.Run("successful deletion of multiple posts", func(t *testing.T) {
			sqlMock.ExpectBegin()
			sqlMock.ExpectExec(regexp.QuoteMeta(`
		UPDATE blog_posts
		SET deleted_at = $1
		WHERE category_id = $2 AND deleted_at IS NULL
	`)).WithArgs(sqlmock.AnyArg(), "cat123").
				WillReturnResult(sqlmock.NewResult(0, 3)) // 3 posts deleted
			sqlMock.ExpectCommit()

			tx, err := db.Begin()
			require.NoError(t, err)
			rowsAffected, err := repo.(*blogPostRepository).DeletePostsByCategoryIDTx(ctx, tx, "cat123")
			require.NoError(t, err)
			assert.Equal(t, int64(3), rowsAffected)
			err = tx.Commit()
			require.NoError(t, err)
			assert.NoError(t, sqlMock.ExpectationsWereMet())
		})

		t.Run("no posts found for category", func(t *testing.T) {
			sqlMock.ExpectBegin()
			sqlMock.ExpectExec(regexp.QuoteMeta(`
		UPDATE blog_posts
		SET deleted_at = $1
		WHERE category_id = $2 AND deleted_at IS NULL
	`)).WithArgs(sqlmock.AnyArg(), "cat456").
				WillReturnResult(sqlmock.NewResult(0, 0)) // 0 posts deleted
			sqlMock.ExpectCommit()

			tx, err := db.Begin()
			require.NoError(t, err)
			rowsAffected, err := repo.(*blogPostRepository).DeletePostsByCategoryIDTx(ctx, tx, "cat456")
			require.NoError(t, err)
			assert.Equal(t, int64(0), rowsAffected)
			err = tx.Commit()
			require.NoError(t, err)
			assert.NoError(t, sqlMock.ExpectationsWereMet())
		})

		t.Run("database error", func(t *testing.T) {
			sqlMock.ExpectBegin()
			sqlMock.ExpectExec(regexp.QuoteMeta(`UPDATE blog_posts`)).
				WillReturnError(errors.New("database error"))
			sqlMock.ExpectRollback()

			tx, err := db.Begin()
			require.NoError(t, err)
			rowsAffected, err := repo.(*blogPostRepository).DeletePostsByCategoryIDTx(ctx, tx, "cat123")
			require.Error(t, err)
			assert.Equal(t, int64(0), rowsAffected)
			assert.Contains(t, err.Error(), "failed to delete blog posts by category")
			err = tx.Rollback()
			require.NoError(t, err)
		})
	})
}

func TestBlogCategoryRepository_ContextErrors(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewBlogCategoryRepository(mockWorkspaceRepo)

	ctx := context.Background() // No workspace_id in context

	t.Run("CreateCategory without workspace_id", func(t *testing.T) {
		err := repo.CreateCategory(ctx, &domain.BlogCategory{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "workspace_id not found in context")
	})

	t.Run("GetCategory without workspace_id", func(t *testing.T) {
		_, err := repo.GetCategory(ctx, "cat123")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "workspace_id not found in context")
	})

	t.Run("ListCategories without workspace_id", func(t *testing.T) {
		_, err := repo.ListCategories(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "workspace_id not found in context")
	})
}

func TestBlogPostRepository_ContextErrors(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewBlogPostRepository(mockWorkspaceRepo)

	ctx := context.Background() // No workspace_id in context

	t.Run("CreatePost without workspace_id", func(t *testing.T) {
		err := repo.CreatePost(ctx, &domain.BlogPost{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "workspace_id not found in context")
	})

	t.Run("GetPost without workspace_id", func(t *testing.T) {
		_, err := repo.GetPost(ctx, "post123")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "workspace_id not found in context")
	})

	t.Run("ListPosts without workspace_id", func(t *testing.T) {
		_, err := repo.ListPosts(ctx, domain.ListBlogPostsRequest{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "workspace_id not found in context")
	})
}

func TestBlogCategoryRepository_TxMethods(t *testing.T) {
	// Test blog repository Tx methods - these were at 0% coverage
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewBlogCategoryRepository(mockWorkspaceRepo)

	db, sqlMock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	ctx := context.Background()

	testCategory := &domain.BlogCategory{
		ID:   "cat123",
		Slug: "tech-blog",
		Settings: domain.BlogCategorySettings{
			Name: "Tech Blog",
		},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	t.Run("GetCategoryTx", func(t *testing.T) {
		sqlMock.ExpectBegin()
		sqlMock.ExpectQuery(regexp.QuoteMeta(`
		SELECT id, slug, settings, created_at, updated_at, deleted_at
		FROM blog_categories
		WHERE id = $1 AND deleted_at IS NULL
	`)).WithArgs("cat123").
			WillReturnRows(sqlmock.NewRows([]string{"id", "slug", "settings", "created_at", "updated_at", "deleted_at"}).
				AddRow(testCategory.ID, testCategory.Slug, []byte(`{"name":"Tech Blog"}`), testCategory.CreatedAt, testCategory.UpdatedAt, nil))
		sqlMock.ExpectCommit()

		tx, err := db.Begin()
		require.NoError(t, err)
		defer func() { _ = tx.Rollback() }()

		category, err := repo.GetCategoryTx(ctx, tx, "cat123")
		assert.NoError(t, err)
		assert.NotNil(t, category)
		assert.Equal(t, "cat123", category.ID)
		_ = tx.Commit()
	})

	t.Run("GetCategoryBySlugTx", func(t *testing.T) {
		sqlMock.ExpectBegin()
		sqlMock.ExpectQuery(regexp.QuoteMeta(`
		SELECT id, slug, settings, created_at, updated_at, deleted_at
		FROM blog_categories
		WHERE slug = $1 AND deleted_at IS NULL
	`)).WithArgs("tech-blog").
			WillReturnRows(sqlmock.NewRows([]string{"id", "slug", "settings", "created_at", "updated_at", "deleted_at"}).
				AddRow(testCategory.ID, testCategory.Slug, []byte(`{"name":"Tech Blog"}`), testCategory.CreatedAt, testCategory.UpdatedAt, nil))
		sqlMock.ExpectCommit()

		tx, err := db.Begin()
		require.NoError(t, err)
		defer func() { _ = tx.Rollback() }()

		category, err := repo.GetCategoryBySlugTx(ctx, tx, "tech-blog")
		assert.NoError(t, err)
		assert.NotNil(t, category)
		assert.Equal(t, "tech-blog", category.Slug)
		_ = tx.Commit()
	})
}

func TestBlogPostRepository_TxMethods(t *testing.T) {
	// Test blog post repository Tx methods - these were at 0% coverage
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockWorkspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	repo := NewBlogPostRepository(mockWorkspaceRepo)

	db, sqlMock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	ctx := context.Background()

	testPost := &domain.BlogPost{
		ID:         "post123",
		CategoryID: "cat123",
		Slug:       "test-post",
		Settings: domain.BlogPostSettings{
			Title: "Test Post",
		},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	t.Run("GetPostTx", func(t *testing.T) {
		sqlMock.ExpectBegin()
		sqlMock.ExpectQuery(regexp.QuoteMeta(`
		SELECT id, category_id, slug, settings, published_at, created_at, updated_at, deleted_at
		FROM blog_posts
		WHERE id = $1 AND deleted_at IS NULL
	`)).WithArgs("post123").
			WillReturnRows(sqlmock.NewRows([]string{"id", "category_id", "slug", "settings", "published_at", "created_at", "updated_at", "deleted_at"}).
				AddRow(testPost.ID, testPost.CategoryID, testPost.Slug, []byte(`{"title":"Test Post"}`), nil, testPost.CreatedAt, testPost.UpdatedAt, nil))
		sqlMock.ExpectCommit()

		tx, err := db.Begin()
		require.NoError(t, err)
		defer func() { _ = tx.Rollback() }()

		post, err := repo.GetPostTx(ctx, tx, "post123")
		assert.NoError(t, err)
		assert.NotNil(t, post)
		assert.Equal(t, "post123", post.ID)
	})

	t.Run("GetPostBySlugTx", func(t *testing.T) {
		// Create a fresh mock for this test case
		db2, sqlMock2, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db2.Close() }()

		sqlMock2.ExpectBegin()
		sqlMock2.ExpectQuery(regexp.QuoteMeta(`
		SELECT id, category_id, slug, settings, published_at, created_at, updated_at, deleted_at
		FROM blog_posts
		WHERE slug = $1 AND deleted_at IS NULL
	`)).WithArgs("test-post").
			WillReturnRows(sqlmock.NewRows([]string{"id", "category_id", "slug", "settings", "published_at", "created_at", "updated_at", "deleted_at"}).
				AddRow(testPost.ID, testPost.CategoryID, testPost.Slug, []byte(`{"title":"Test Post"}`), nil, testPost.CreatedAt, testPost.UpdatedAt, nil))
		sqlMock2.ExpectCommit()

		tx, err := db2.Begin()
		require.NoError(t, err)
		defer func() { _ = tx.Rollback() }()

		post, err := repo.GetPostBySlugTx(ctx, tx, "test-post")
		_ = tx.Commit()
		assert.NoError(t, err)
		assert.NotNil(t, post)
		assert.Equal(t, "test-post", post.Slug)
		assert.NoError(t, sqlMock2.ExpectationsWereMet())
	})
}
