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

// Public blog rendering resolves posts/categories by slug on every page
// view. The not-found contract ("blog post not found", a fresh error that
// does not wrap sql.ErrNoRows) is what the renderer maps to a 404, so it
// is pinned exactly, along with workspace scoping on every lookup.

var blogPostCols = []string{"id", "category_id", "slug", "settings", "published_at", "created_at", "updated_at", "deleted_at"}

func blogCtx() context.Context {
	return context.WithValue(context.Background(), domain.WorkspaceIDKey, "ws-1")
}

func blogPostRepoWithDB(t *testing.T, db *sql.DB) (*blogPostRepository, *mocks.MockWorkspaceRepository, *gomock.Controller) {
	t.Helper()
	ctrl := gomock.NewController(t)
	wsRepo := mocks.NewMockWorkspaceRepository(ctrl)
	wsRepo.EXPECT().GetConnection(gomock.Any(), "ws-1").Return(db, nil).AnyTimes()
	return NewBlogPostRepository(wsRepo).(*blogPostRepository), wsRepo, ctrl
}

func TestBlogPostRepository_GetBySlug_NotFoundContract(t *testing.T) {
	now := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)

	t.Run("GetPostBySlug maps no rows to the plain not-found error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		repo, _, _ := blogPostRepoWithDB(t, db)
		mock.ExpectQuery(regexp.QuoteMeta("FROM blog_posts")).
			WithArgs("missing").
			WillReturnRows(sqlmock.NewRows(blogPostCols))

		post, err := repo.GetPostBySlug(blogCtx(), "missing")
		assert.Nil(t, post)
		require.EqualError(t, err, "blog post not found")
		assert.NotErrorIs(t, err, sql.ErrNoRows,
			"not-found must not wrap ErrNoRows: the renderer branches on the message")
	})

	t.Run("GetPostBySlug returns the scanned row verbatim", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		repo, _, _ := blogPostRepoWithDB(t, db)
		mock.ExpectQuery(regexp.QuoteMeta("FROM blog_posts")).
			WithArgs("hello-world").
			WillReturnRows(sqlmock.NewRows(blogPostCols).
				AddRow("post-1", "cat-1", "hello-world", []byte(`{"title":"Hello"}`), now, now, now, nil))

		post, err := repo.GetPostBySlug(blogCtx(), "hello-world")
		require.NoError(t, err)
		assert.Equal(t, "post-1", post.ID)
		assert.Equal(t, "cat-1", post.CategoryID)
		assert.Equal(t, "hello-world", post.Slug)
		assert.Equal(t, "Hello", post.Settings.Title)
		require.NotNil(t, post.PublishedAt)
		assert.Equal(t, now, *post.PublishedAt)
	})

	t.Run("GetPostBySlug wraps driver errors but keeps the operation", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		repo, _, _ := blogPostRepoWithDB(t, db)
		sentinel := errors.New("connection reset")
		mock.ExpectQuery(regexp.QuoteMeta("FROM blog_posts")).WillReturnError(sentinel)

		_, err = repo.GetPostBySlug(blogCtx(), "hello-world")
		require.ErrorContains(t, err, "failed to get blog post:")
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("lookups refuse to run without a workspace in context", func(t *testing.T) {
		db, _, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		repo, _, _ := blogPostRepoWithDB(t, db)

		_, err = repo.GetPostBySlug(context.Background(), "hello-world")
		require.EqualError(t, err, "workspace_id not found in context")
		_, err = repo.GetPostByCategoryAndSlug(context.Background(), "tech", "hello-world")
		require.EqualError(t, err, "workspace_id not found in context")
	})
}

func TestBlogPostRepository_GetPostByCategoryAndSlug(t *testing.T) {
	now := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)

	t.Run("both slugs are bound in category-then-post order", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		repo, _, _ := blogPostRepoWithDB(t, db)
		// The join must scope the post to its category: a post slug that
		// exists under another category must not resolve here.
		mock.ExpectQuery(regexp.QuoteMeta("JOIN blog_categories")).
			WithArgs("tech", "hello-world").
			WillReturnRows(sqlmock.NewRows(blogPostCols).
				AddRow("post-1", "cat-1", "hello-world", []byte(`{}`), nil, now, now, nil))

		post, err := repo.GetPostByCategoryAndSlug(blogCtx(), "tech", "hello-world")
		require.NoError(t, err)
		assert.Equal(t, "post-1", post.ID)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("no matching category/post pair is the same not-found contract", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		repo, _, _ := blogPostRepoWithDB(t, db)
		mock.ExpectQuery(regexp.QuoteMeta("JOIN blog_categories")).
			WillReturnRows(sqlmock.NewRows(blogPostCols))

		_, err = repo.GetPostByCategoryAndSlug(blogCtx(), "tech", "ghost")
		require.EqualError(t, err, "blog post not found")
	})
}

func TestBlogCategoryRepository_GetCategoryBySlug(t *testing.T) {
	now := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	catCols := []string{"id", "slug", "settings", "created_at", "updated_at", "deleted_at"}

	newCatRepo := func(t *testing.T, db *sql.DB) domain.BlogCategoryRepository {
		ctrl := gomock.NewController(t)
		wsRepo := mocks.NewMockWorkspaceRepository(ctrl)
		wsRepo.EXPECT().GetConnection(gomock.Any(), "ws-1").Return(db, nil).AnyTimes()
		return NewBlogCategoryRepository(wsRepo)
	}

	t.Run("resolves by slug with the row mapped verbatim", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		mock.ExpectQuery(regexp.QuoteMeta("FROM blog_categories")).
			WithArgs("tech").
			WillReturnRows(sqlmock.NewRows(catCols).
				AddRow("cat-1", "tech", []byte(`{"name":"Tech"}`), now, now, nil))

		cat, err := newCatRepo(t, db).GetCategoryBySlug(blogCtx(), "tech")
		require.NoError(t, err)
		assert.Equal(t, "cat-1", cat.ID)
		assert.Equal(t, "tech", cat.Slug)
		assert.Equal(t, "Tech", cat.Settings.Name)
	})

	t.Run("unknown slug and missing workspace fail distinctly", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		mock.ExpectQuery(regexp.QuoteMeta("FROM blog_categories")).
			WillReturnRows(sqlmock.NewRows(catCols))

		repo := newCatRepo(t, db)
		_, err = repo.GetCategoryBySlug(blogCtx(), "ghost")
		require.Error(t, err)

		_, err = repo.GetCategoryBySlug(context.Background(), "tech")
		require.EqualError(t, err, "workspace_id not found in context")
	})

	t.Run("connection failure is wrapped before any query", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		wsRepo := mocks.NewMockWorkspaceRepository(ctrl)
		sentinel := errors.New("no workspace db")
		wsRepo.EXPECT().GetConnection(gomock.Any(), "ws-1").Return(nil, sentinel).Times(1)
		repo := NewBlogCategoryRepository(wsRepo)

		_, err := repo.GetCategoryBySlug(blogCtx(), "tech")
		require.ErrorContains(t, err, "failed to get workspace connection:")
		assert.ErrorIs(t, err, sentinel)
	})
}
