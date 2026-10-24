package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// Gap coverage for blog_service.go branches missed by the main, guards,
// write and degradation suites (see /tmp/coverage-ci2.txt): category/theme
// guard chains and slug-conflict edges, cache-clearing side effects, and
// the RenderCategoryPage/RenderPostPage BlogRenderError taxonomy. Every
// guard test relies on gomock: no repository expectation is set past a
// failing guard, so a downstream call fails the test outright.

func blogGapCtx() context.Context {
	return context.WithValue(context.Background(), domain.WorkspaceIDKey, "workspace123")
}

func blogGapWorkspace() *domain.Workspace {
	return &domain.Workspace{ID: "workspace123", Name: "WS", Settings: domain.WorkspaceSettings{Timezone: "UTC"}}
}

func blogGapPublishedPost() *domain.BlogPost {
	publishedAt := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	return &domain.BlogPost{
		ID: "post-1", Slug: "test-post", CategoryID: "cat-1", PublishedAt: &publishedAt,
		Settings: domain.BlogPostSettings{
			Title:    "Test Post",
			Template: domain.BlogPostTemplateReference{TemplateID: "tpl-1", TemplateVersion: 1},
		},
	}
}

func TestBlogGap_CreateCategoryGuardsAndSlug(t *testing.T) {
	validReq := func() *domain.CreateBlogCategoryRequest {
		return &domain.CreateBlogCategoryRequest{Name: "Tech Blog", Slug: "tech-blog", Description: "Tech"}
	}

	t.Run("missing workspace fails before auth or repo", func(t *testing.T) {
		svc, _, _, _, _, _, _, _ := setupBlogServiceTest(t)
		_, err := svc.CreateCategory(blogCtxNoWorkspace(), validReq())
		require.EqualError(t, err, "workspace_id not found in context")
	})

	t.Run("auth failure is wrapped and repo is not called", func(t *testing.T) {
		svc, _, _, _, _, _, _, auth := setupBlogServiceTest(t)
		sentinel := errors.New("token expired")
		expectBlogAuthError(auth, "ws-1", sentinel)
		_, err := svc.CreateCategory(blogCtxWithWorkspace("ws-1"), validReq())
		require.EqualError(t, err, "failed to authenticate user: token expired")
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("write permission is required and repo is not called", func(t *testing.T) {
		svc, _, _, _, _, _, _, auth := setupBlogServiceTest(t)
		ctx := expectBlogAuth(auth, "ws-1", true, false)
		_, err := svc.CreateCategory(ctx, validReq())
		require.EqualError(t, err, "Insufficient permissions: write access to blog required")
		var permErr *domain.PermissionError
		assert.ErrorAs(t, err, &permErr)
	})

	t.Run("slug conflict names the slug exactly and create is not called", func(t *testing.T) {
		svc, catRepo, _, _, _, _, _, auth := setupBlogServiceTest(t)
		ctx := expectBlogAuth(auth, "ws-1", true, true)
		catRepo.EXPECT().GetCategoryBySlug(gomock.Any(), "tech-blog").
			Return(&domain.BlogCategory{ID: "cat-old", Slug: "tech-blog"}, nil).Times(1)
		_, err := svc.CreateCategory(ctx, validReq())
		require.EqualError(t, err, "category with slug 'tech-blog' already exists")
	})

	t.Run("a slug lookup error does not block creation", func(t *testing.T) {
		// Real contract: only (existing != nil, err == nil) conflicts; a
		// lookup error is treated as "no conflict" and creation proceeds.
		svc, catRepo, _, _, _, _, _, auth := setupBlogServiceTest(t)
		ctx := expectBlogAuth(auth, "ws-1", true, true)
		catRepo.EXPECT().GetCategoryBySlug(gomock.Any(), "tech-blog").
			Return(nil, errors.New("not found")).Times(1)
		catRepo.EXPECT().CreateCategory(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, cat *domain.BlogCategory) error {
				assert.Equal(t, "tech-blog", cat.Slug)
				assert.Equal(t, "Tech Blog", cat.Settings.Name)
				assert.Equal(t, "Tech", cat.Settings.Description)
				assert.NotEmpty(t, cat.ID)
				return nil
			}).Times(1)
		got, err := svc.CreateCategory(ctx, validReq())
		require.NoError(t, err)
		assert.Equal(t, "tech-blog", got.Slug)
	})

	t.Run("existing row returned with an error still does not conflict", func(t *testing.T) {
		svc, catRepo, _, _, _, _, _, auth := setupBlogServiceTest(t)
		ctx := expectBlogAuth(auth, "ws-1", true, true)
		catRepo.EXPECT().GetCategoryBySlug(gomock.Any(), "tech-blog").
			Return(&domain.BlogCategory{ID: "cat-old"}, errors.New("ambiguous")).Times(1)
		catRepo.EXPECT().CreateCategory(gomock.Any(), gomock.Any()).Return(nil).Times(1)
		_, err := svc.CreateCategory(ctx, validReq())
		require.NoError(t, err)
	})

	t.Run("invalid slug fails validation before the slug lookup", func(t *testing.T) {
		svc, _, _, _, _, _, _, auth := setupBlogServiceTest(t)
		ctx := expectBlogAuth(auth, "ws-1", true, true)
		req := validReq()
		req.Slug = "Bad Slug!"
		_, err := svc.CreateCategory(ctx, req)
		require.EqualError(t, err, "slug must contain only lowercase letters, numbers, and hyphens")
	})
}

func TestBlogGap_UpdateCategorySlugAndPersist(t *testing.T) {
	validReq := func() *domain.UpdateBlogCategoryRequest {
		return &domain.UpdateBlogCategoryRequest{ID: "cat-1", Name: "New Name", Slug: "new-slug"}
	}

	t.Run("guards run before any repo call", func(t *testing.T) {
		svc, _, _, _, _, _, _, _ := setupBlogServiceTest(t)
		_, err := svc.UpdateCategory(blogCtxNoWorkspace(), validReq())
		require.EqualError(t, err, "workspace_id not found in context")

		svc2, _, _, _, _, _, _, auth2 := setupBlogServiceTest(t)
		sentinel := errors.New("no session")
		expectBlogAuthError(auth2, "ws-1", sentinel)
		_, err = svc2.UpdateCategory(blogCtxWithWorkspace("ws-1"), validReq())
		require.EqualError(t, err, "failed to authenticate user: no session")
		assert.ErrorIs(t, err, sentinel)

		svc3, _, _, _, _, _, _, auth3 := setupBlogServiceTest(t)
		ctx3 := expectBlogAuth(auth3, "ws-1", true, false)
		_, err = svc3.UpdateCategory(ctx3, validReq())
		require.EqualError(t, err, "Insufficient permissions: write access to blog required")
	})

	t.Run("missing category wraps the repo error and update is not called", func(t *testing.T) {
		svc, catRepo, _, _, _, _, _, auth := setupBlogServiceTest(t)
		ctx := expectBlogAuth(auth, "ws-1", true, true)
		sentinel := errors.New("no rows")
		catRepo.EXPECT().GetCategory(gomock.Any(), "cat-1").Return(nil, sentinel).Times(1)
		_, err := svc.UpdateCategory(ctx, validReq())
		require.EqualError(t, err, "category not found: no rows")
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("slug owned by the same category is not a conflict", func(t *testing.T) {
		svc, catRepo, _, _, _, _, _, auth := setupBlogServiceTest(t)
		ctx := expectBlogAuth(auth, "ws-1", true, true)
		catRepo.EXPECT().GetCategory(gomock.Any(), "cat-1").
			Return(&domain.BlogCategory{ID: "cat-1", Slug: "old-slug"}, nil).Times(1)
		catRepo.EXPECT().GetCategoryBySlug(gomock.Any(), "new-slug").
			Return(&domain.BlogCategory{ID: "cat-1", Slug: "new-slug"}, nil).Times(1)
		catRepo.EXPECT().UpdateCategory(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, cat *domain.BlogCategory) error {
				assert.Equal(t, "new-slug", cat.Slug)
				assert.Equal(t, "New Name", cat.Settings.Name)
				return nil
			}).Times(1)
		got, err := svc.UpdateCategory(ctx, validReq())
		require.NoError(t, err)
		assert.Equal(t, "new-slug", got.Slug)
	})

	t.Run("unchanged slug skips the slug lookup entirely", func(t *testing.T) {
		svc, catRepo, _, _, _, _, _, auth := setupBlogServiceTest(t)
		ctx := expectBlogAuth(auth, "ws-1", true, true)
		req := validReq()
		req.Slug = "same-slug"
		catRepo.EXPECT().GetCategory(gomock.Any(), "cat-1").
			Return(&domain.BlogCategory{ID: "cat-1", Slug: "same-slug"}, nil).Times(1)
		// No GetCategoryBySlug expectation: gomock fails if it is called.
		catRepo.EXPECT().UpdateCategory(gomock.Any(), gomock.Any()).Return(nil).Times(1)
		_, err := svc.UpdateCategory(ctx, req)
		require.NoError(t, err)
	})

	t.Run("persist failure is wrapped with the operation name", func(t *testing.T) {
		svc, catRepo, _, _, _, _, _, auth := setupBlogServiceTest(t)
		ctx := expectBlogAuth(auth, "ws-1", true, true)
		catRepo.EXPECT().GetCategory(gomock.Any(), "cat-1").
			Return(&domain.BlogCategory{ID: "cat-1", Slug: "old-slug"}, nil).Times(1)
		catRepo.EXPECT().GetCategoryBySlug(gomock.Any(), "new-slug").
			Return(nil, errors.New("not found")).Times(1)
		sentinel := errors.New("write failed")
		catRepo.EXPECT().UpdateCategory(gomock.Any(), gomock.Any()).Return(sentinel).Times(1)
		_, err := svc.UpdateCategory(ctx, validReq())
		require.EqualError(t, err, "failed to update category: write failed")
		assert.ErrorIs(t, err, sentinel)
	})
}

func TestBlogGap_GetPostByCategoryAndSlugGuards(t *testing.T) {
	t.Run("guards run before the repo", func(t *testing.T) {
		svc, _, _, _, _, _, _, _ := setupBlogServiceTest(t)
		_, err := svc.GetPostByCategoryAndSlug(blogCtxNoWorkspace(), "tech", "hello")
		require.EqualError(t, err, "workspace_id not found in context")

		svc2, _, _, _, _, _, _, auth2 := setupBlogServiceTest(t)
		sentinel := errors.New("expired")
		expectBlogAuthError(auth2, "ws-1", sentinel)
		_, err = svc2.GetPostByCategoryAndSlug(blogCtxWithWorkspace("ws-1"), "tech", "hello")
		require.EqualError(t, err, "failed to authenticate user: expired")
		assert.ErrorIs(t, err, sentinel)

		svc3, _, _, _, _, _, _, auth3 := setupBlogServiceTest(t)
		ctx3 := expectBlogAuth(auth3, "ws-1", false, true)
		_, err = svc3.GetPostByCategoryAndSlug(ctx3, "tech", "hello")
		require.EqualError(t, err, "Insufficient permissions: read access to blog required")
	})

	t.Run("both slugs are forwarded verbatim and the result returned", func(t *testing.T) {
		svc, _, postRepo, _, _, _, _, auth := setupBlogServiceTest(t)
		ctx := expectBlogAuth(auth, "ws-1", true, false)
		want := &domain.BlogPost{ID: "post-1", Slug: "hello"}
		postRepo.EXPECT().GetPostByCategoryAndSlug(gomock.Any(), "tech", "hello").Return(want, nil).Times(1)
		got, err := svc.GetPostByCategoryAndSlug(ctx, "tech", "hello")
		require.NoError(t, err)
		assert.Same(t, want, got)
	})

	t.Run("repo error reaches the caller unwrapped", func(t *testing.T) {
		svc, _, postRepo, _, _, _, _, auth := setupBlogServiceTest(t)
		ctx := expectBlogAuth(auth, "ws-1", true, false)
		sentinel := errors.New("no such post")
		postRepo.EXPECT().GetPostByCategoryAndSlug(gomock.Any(), "tech", "ghost").
			Return(nil, sentinel).Times(1)
		_, err := svc.GetPostByCategoryAndSlug(ctx, "tech", "ghost")
		require.EqualError(t, err, "no such post")
		assert.ErrorIs(t, err, sentinel)
	})
}

func TestBlogGap_ListPostsGuards(t *testing.T) {
	t.Run("guards and validation run before the repo", func(t *testing.T) {
		svc, _, _, _, _, _, _, _ := setupBlogServiceTest(t)
		_, err := svc.ListPosts(blogCtxNoWorkspace(), &domain.ListBlogPostsRequest{})
		require.EqualError(t, err, "workspace_id not found in context")

		svc2, _, _, _, _, _, _, auth2 := setupBlogServiceTest(t)
		expectBlogAuthError(auth2, "ws-1", errors.New("expired"))
		_, err = svc2.ListPosts(blogCtxWithWorkspace("ws-1"), &domain.ListBlogPostsRequest{})
		require.ErrorContains(t, err, "failed to authenticate user:")

		svc3, _, _, _, _, _, _, auth3 := setupBlogServiceTest(t)
		ctx3 := expectBlogAuth(auth3, "ws-1", false, false)
		_, err = svc3.ListPosts(ctx3, &domain.ListBlogPostsRequest{})
		require.EqualError(t, err, "Insufficient permissions: read access to blog required")

		svc4, _, _, _, _, _, _, auth4 := setupBlogServiceTest(t)
		ctx4 := expectBlogAuth(auth4, "ws-1", true, false)
		_, err = svc4.ListPosts(ctx4, &domain.ListBlogPostsRequest{Status: "bogus"})
		require.EqualError(t, err, "invalid status: bogus")
	})

	t.Run("validated params reach the repo and its error passes through", func(t *testing.T) {
		svc, _, postRepo, _, _, _, _, auth := setupBlogServiceTest(t)
		ctx := expectBlogAuth(auth, "ws-1", true, false)
		sentinel := errors.New("query failed")
		postRepo.EXPECT().ListPosts(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, p domain.ListBlogPostsRequest) (*domain.BlogPostListResponse, error) {
				assert.Equal(t, domain.BlogPostStatusPublished, p.Status)
				assert.Equal(t, 2, p.Page)
				assert.Equal(t, 10, p.Limit)
				assert.Equal(t, 10, p.Offset, "offset is (page-1)*limit after Validate")
				return nil, sentinel
			}).Times(1)
		_, err := svc.ListPosts(ctx, &domain.ListBlogPostsRequest{Status: domain.BlogPostStatusPublished, Page: 2, Limit: 10})
		require.EqualError(t, err, "query failed")
		assert.ErrorIs(t, err, sentinel)
	})
}

func TestBlogGap_CreateThemeGuardsAndShape(t *testing.T) {
	files := domain.BlogThemeFiles{HomeLiquid: "<h1>hi</h1>", StylesCSS: "body{}"}
	notes := "draft v2"

	t.Run("guards run before the repo", func(t *testing.T) {
		svc, _, _, _, _, _, _, _ := setupBlogServiceTest(t)
		_, err := svc.CreateTheme(blogCtxNoWorkspace(), &domain.CreateBlogThemeRequest{Files: files})
		require.EqualError(t, err, "workspace_id not found in context")

		svc2, _, _, _, _, _, _, auth2 := setupBlogServiceTest(t)
		sentinel := errors.New("expired")
		expectBlogAuthError(auth2, "ws-1", sentinel)
		_, err = svc2.CreateTheme(blogCtxWithWorkspace("ws-1"), &domain.CreateBlogThemeRequest{Files: files})
		require.EqualError(t, err, "failed to authenticate user: expired")
		assert.ErrorIs(t, err, sentinel)

		svc3, _, _, _, _, _, _, auth3 := setupBlogServiceTest(t)
		ctx3 := expectBlogAuth(auth3, "ws-1", true, false)
		_, err = svc3.CreateTheme(ctx3, &domain.CreateBlogThemeRequest{Files: files})
		require.EqualError(t, err, "Insufficient permissions: write access to blog required")
	})

	t.Run("new theme is unpublished and files and notes are forwarded verbatim", func(t *testing.T) {
		svc, _, _, themeRepo, _, _, _, auth := setupBlogServiceTest(t)
		ctx := expectBlogAuth(auth, "ws-1", true, true)
		themeRepo.EXPECT().CreateTheme(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, th *domain.BlogTheme) error {
				assert.Nil(t, th.PublishedAt, "new themes start unpublished")
				assert.Equal(t, files, th.Files)
				require.NotNil(t, th.Notes)
				assert.Equal(t, "draft v2", *th.Notes)
				assert.Equal(t, 0, th.Version, "version is assigned by the repository, not the service")
				return nil
			}).Times(1)
		got, err := svc.CreateTheme(ctx, &domain.CreateBlogThemeRequest{Files: files, Notes: &notes})
		require.NoError(t, err)
		assert.Equal(t, files, got.Files)
	})
}

func TestBlogGap_UpdateThemeGuards(t *testing.T) {
	t.Run("guards and request validation run before the repo", func(t *testing.T) {
		svc, _, _, _, _, _, _, _ := setupBlogServiceTest(t)
		_, err := svc.UpdateTheme(blogCtxNoWorkspace(), &domain.UpdateBlogThemeRequest{Version: 2})
		require.EqualError(t, err, "workspace_id not found in context")

		svc2, _, _, _, _, _, _, auth2 := setupBlogServiceTest(t)
		expectBlogAuthError(auth2, "ws-1", errors.New("expired"))
		_, err = svc2.UpdateTheme(blogCtxWithWorkspace("ws-1"), &domain.UpdateBlogThemeRequest{Version: 2})
		require.ErrorContains(t, err, "failed to authenticate user:")

		svc3, _, _, _, _, _, _, auth3 := setupBlogServiceTest(t)
		ctx3 := expectBlogAuth(auth3, "ws-1", true, false)
		_, err = svc3.UpdateTheme(ctx3, &domain.UpdateBlogThemeRequest{Version: 2})
		require.EqualError(t, err, "Insufficient permissions: write access to blog required")

		svc4, _, _, _, _, _, _, auth4 := setupBlogServiceTest(t)
		ctx4 := expectBlogAuth(auth4, "ws-1", true, true)
		_, err = svc4.UpdateTheme(ctx4, &domain.UpdateBlogThemeRequest{Version: 0})
		require.EqualError(t, err, "version must be positive")

		svc5, _, _, _, _, _, _, auth5 := setupBlogServiceTest(t)
		ctx5 := expectBlogAuth(auth5, "ws-1", true, true)
		_, err = svc5.UpdateTheme(ctx5, &domain.UpdateBlogThemeRequest{Version: -3})
		require.EqualError(t, err, "version must be positive")
	})

	t.Run("missing theme wraps the repo error and update is not called", func(t *testing.T) {
		svc, _, _, themeRepo, _, _, _, auth := setupBlogServiceTest(t)
		ctx := expectBlogAuth(auth, "ws-1", true, true)
		sentinel := errors.New("no rows")
		themeRepo.EXPECT().GetTheme(gomock.Any(), 9).Return(nil, sentinel).Times(1)
		_, err := svc.UpdateTheme(ctx, &domain.UpdateBlogThemeRequest{Version: 9})
		require.EqualError(t, err, "theme not found: no rows")
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("published theme cannot be updated and persist is not called", func(t *testing.T) {
		svc, _, _, themeRepo, _, _, _, auth := setupBlogServiceTest(t)
		ctx := expectBlogAuth(auth, "ws-1", true, true)
		publishedAt := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
		themeRepo.EXPECT().GetTheme(gomock.Any(), 3).
			Return(&domain.BlogTheme{Version: 3, PublishedAt: &publishedAt}, nil).Times(1)
		// No UpdateTheme expectation: a published theme must never be mutated.
		_, err := svc.UpdateTheme(ctx, &domain.UpdateBlogThemeRequest{Version: 3})
		require.EqualError(t, err, "cannot update published theme")
	})

	t.Run("stored theme failing validation aborts before persist", func(t *testing.T) {
		// Real contract: a stored row with version 0 fails
		// BlogTheme.Validate ("version must be positive") after the
		// published check, so UpdateTheme must not be reached.
		svc, _, _, themeRepo, _, _, _, auth := setupBlogServiceTest(t)
		ctx := expectBlogAuth(auth, "ws-1", true, true)
		themeRepo.EXPECT().GetTheme(gomock.Any(), 2).
			Return(&domain.BlogTheme{Version: 0}, nil).Times(1)
		_, err := svc.UpdateTheme(ctx, &domain.UpdateBlogThemeRequest{Version: 2})
		require.EqualError(t, err, "version must be positive")
	})

	t.Run("persist failure is wrapped with the operation name", func(t *testing.T) {
		svc, _, _, themeRepo, _, _, _, auth := setupBlogServiceTest(t)
		ctx := expectBlogAuth(auth, "ws-1", true, true)
		themeRepo.EXPECT().GetTheme(gomock.Any(), 2).
			Return(&domain.BlogTheme{Version: 2, Files: domain.BlogThemeFiles{HomeLiquid: "old"}}, nil).Times(1)
		sentinel := errors.New("write failed")
		themeRepo.EXPECT().UpdateTheme(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, th *domain.BlogTheme) error {
				assert.Equal(t, 2, th.Version)
				assert.Equal(t, "new", th.Files.HomeLiquid)
				return sentinel
			}).Times(1)
		req := &domain.UpdateBlogThemeRequest{Version: 2, Files: domain.BlogThemeFiles{HomeLiquid: "new"}}
		_, err := svc.UpdateTheme(ctx, req)
		require.EqualError(t, err, "failed to update theme: write failed")
		assert.ErrorIs(t, err, sentinel)
	})
}

func TestBlogGap_ListThemesAndPublishGuards(t *testing.T) {
	t.Run("ListThemes guards run before the repo", func(t *testing.T) {
		svc, _, _, _, _, _, _, _ := setupBlogServiceTest(t)
		_, err := svc.ListThemes(blogCtxNoWorkspace(), &domain.ListBlogThemesRequest{})
		require.EqualError(t, err, "workspace_id not found in context")

		svc2, _, _, _, _, _, _, auth2 := setupBlogServiceTest(t)
		expectBlogAuthError(auth2, "ws-1", errors.New("expired"))
		_, err = svc2.ListThemes(blogCtxWithWorkspace("ws-1"), &domain.ListBlogThemesRequest{})
		require.ErrorContains(t, err, "failed to authenticate user:")
	})

	t.Run("ListThemes defaults the limit and passes the repo error through", func(t *testing.T) {
		svc, _, _, themeRepo, _, _, _, auth := setupBlogServiceTest(t)
		ctx := expectBlogAuth(auth, "ws-1", true, false)
		sentinel := errors.New("query failed")
		themeRepo.EXPECT().ListThemes(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, p domain.ListBlogThemesRequest) (*domain.BlogThemeListResponse, error) {
				assert.Equal(t, 50, p.Limit, "zero limit defaults to 50")
				assert.Equal(t, 0, p.Offset)
				return nil, sentinel
			}).Times(1)
		_, err := svc.ListThemes(ctx, &domain.ListBlogThemesRequest{})
		require.EqualError(t, err, "query failed")
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("PublishTheme guards and validation run before the repo", func(t *testing.T) {
		svc, _, _, _, _, _, _, _ := setupBlogServiceTest(t)
		err := svc.PublishTheme(blogCtxNoWorkspace(), &domain.PublishBlogThemeRequest{Version: 1})
		require.EqualError(t, err, "workspace_id not found in context")

		svc2, _, _, _, _, _, _, auth2 := setupBlogServiceTest(t)
		expectBlogAuthError(auth2, "ws-1", errors.New("expired"))
		err = svc2.PublishTheme(blogCtxWithWorkspace("ws-1"), &domain.PublishBlogThemeRequest{Version: 1})
		require.ErrorContains(t, err, "failed to authenticate user:")

		svc3, _, _, _, _, _, _, auth3 := setupBlogServiceTest(t)
		ctx3 := expectBlogAuth(auth3, "ws-1", true, true)
		err = svc3.PublishTheme(ctx3, &domain.PublishBlogThemeRequest{Version: 0})
		require.EqualError(t, err, "version must be positive")
	})
}

func TestBlogGap_CacheClearingSideEffects(t *testing.T) {
	t.Run("successful PublishTheme clears the whole cache and forwards the user", func(t *testing.T) {
		svc, _, _, themeRepo, _, _, _, auth := setupBlogServiceTest(t)
		ctx := expectBlogAuth(auth, "ws-1", true, true)
		svc.cache.Set("blog:home", "x", time.Minute)
		svc.cache.Set("blog:post", "y", time.Minute)
		require.Equal(t, 2, svc.cache.Size())
		themeRepo.EXPECT().PublishTheme(gomock.Any(), 4, "user-1").Return(nil).Times(1)
		require.NoError(t, svc.PublishTheme(ctx, &domain.PublishBlogThemeRequest{Version: 4}))
		assert.Equal(t, 0, svc.cache.Size(), "publishing a theme must clear every cached page")
	})

	t.Run("failed PublishTheme leaves the cache untouched", func(t *testing.T) {
		svc, _, _, themeRepo, _, _, _, auth := setupBlogServiceTest(t)
		ctx := expectBlogAuth(auth, "ws-1", true, true)
		svc.cache.Set("blog:home", "x", time.Minute)
		sentinel := errors.New("publish failed")
		themeRepo.EXPECT().PublishTheme(gomock.Any(), 4, "user-1").Return(sentinel).Times(1)
		err := svc.PublishTheme(ctx, &domain.PublishBlogThemeRequest{Version: 4})
		require.EqualError(t, err, "failed to publish theme: publish failed")
		assert.ErrorIs(t, err, sentinel)
		assert.Equal(t, 1, svc.cache.Size(), "a failed publish must not invalidate cached pages")
	})

	t.Run("guard-rejected PublishTheme leaves the cache untouched", func(t *testing.T) {
		svc, _, _, _, _, _, _, auth := setupBlogServiceTest(t)
		ctx := expectBlogAuth(auth, "ws-1", true, false)
		svc.cache.Set("blog:home", "x", time.Minute)
		err := svc.PublishTheme(ctx, &domain.PublishBlogThemeRequest{Version: 4})
		require.EqualError(t, err, "Insufficient permissions: write access to blog required")
		assert.Equal(t, 1, svc.cache.Size())
	})

	t.Run("successful UpdatePost clears the cache", func(t *testing.T) {
		svc, catRepo, postRepo, _, _, _, _, auth := setupBlogServiceTest(t)
		ctx := expectBlogAuth(auth, "ws-1", true, true)
		svc.cache.Set("blog:home", "x", time.Minute)
		postRepo.EXPECT().GetPost(gomock.Any(), "post-1").
			Return(&domain.BlogPost{ID: "post-1", Slug: "same"}, nil).Times(1)
		catRepo.EXPECT().GetCategory(gomock.Any(), "cat-1").
			Return(&domain.BlogCategory{ID: "cat-1"}, nil).Times(1)
		postRepo.EXPECT().UpdatePost(gomock.Any(), gomock.Any()).Return(nil).Times(1)
		req := &domain.UpdateBlogPostRequest{ID: "post-1", CategoryID: "cat-1", Slug: "same", Title: "T", TemplateID: "tpl-1"}
		_, err := svc.UpdatePost(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, 0, svc.cache.Size())
	})

	t.Run("clearBlogCache with a nil cache does not panic", func(t *testing.T) {
		svc := &BlogService{logger: logger.NewLoggerWithLevel("disabled"), cache: nil}
		assert.NotPanics(t, func() { svc.clearBlogCache("ws-1") })
	})

	t.Run("clearBlogCache empties every key", func(t *testing.T) {
		svc, _, _, _, _, _, _, _ := setupBlogServiceTest(t)
		svc.cache.Set("a", 1, time.Minute)
		svc.cache.Set("b", 2, time.Minute)
		require.Equal(t, 2, svc.cache.Size())
		svc.clearBlogCache("ws-1")
		assert.Equal(t, 0, svc.cache.Size())
	})
}

func blogGapRenderErr(t *testing.T, err error) *domain.BlogRenderError {
	t.Helper()
	require.Error(t, err)
	var re *domain.BlogRenderError
	require.ErrorAs(t, err, &re)
	return re
}

func TestBlogGap_RenderCategoryPageTaxonomy(t *testing.T) {
	ctx := blogGapCtx()
	theme := &domain.BlogTheme{Version: 1, Files: domain.BlogThemeFiles{CategoryLiquid: "<h1>{{ category.name }}</h1>posts={{ posts | size }}"}}
	category := &domain.BlogCategory{ID: "cat-1", Slug: "tech", Settings: domain.BlogCategorySettings{Name: "Technology"}}

	t.Run("workspace failure is RENDER_FAILED with the sentinel as details", func(t *testing.T) {
		svc, _, _, _, wr, _, _, _ := setupBlogServiceTest(t)
		sentinel := errors.New("db down")
		wr.EXPECT().GetByID(ctx, "workspace123").Return(nil, sentinel).Times(1)
		html, err := svc.RenderCategoryPage(ctx, "workspace123", "tech", 1, nil)
		assert.Empty(t, html)
		re := blogGapRenderErr(t, err)
		assert.Equal(t, domain.ErrCodeRenderFailed, re.Code)
		assert.Equal(t, "Failed to get workspace", re.Message)
		assert.ErrorIs(t, re.Details, sentinel)
		assert.Equal(t, "render_failed: Failed to get workspace (db down)", err.Error())
	})

	t.Run("versioned theme lookup maps the unpublished sentinel too", func(t *testing.T) {
		// Real contract: the "no published theme found" string maps to
		// THEME_NOT_PUBLISHED even when a specific version was requested.
		svc, _, _, th, wr, _, _, _ := setupBlogServiceTest(t)
		wr.EXPECT().GetByID(ctx, "workspace123").Return(blogGapWorkspace(), nil).Times(1)
		version := 7
		th.EXPECT().GetTheme(ctx, 7).Return(nil, errors.New("no published theme found")).Times(1)
		_, err := svc.RenderCategoryPage(ctx, "workspace123", "tech", 1, &version)
		re := blogGapRenderErr(t, err)
		assert.Equal(t, domain.ErrCodeThemeNotPublished, re.Code)
		assert.Equal(t, "No published theme available", re.Message)
	})

	t.Run("other versioned theme failures are THEME_NOT_FOUND", func(t *testing.T) {
		svc, _, _, th, wr, _, _, _ := setupBlogServiceTest(t)
		wr.EXPECT().GetByID(ctx, "workspace123").Return(blogGapWorkspace(), nil).Times(1)
		version := 7
		sentinel := errors.New("redis down")
		th.EXPECT().GetTheme(ctx, 7).Return(nil, sentinel).Times(1)
		_, err := svc.RenderCategoryPage(ctx, "workspace123", "tech", 1, &version)
		re := blogGapRenderErr(t, err)
		assert.Equal(t, domain.ErrCodeThemeNotFound, re.Code)
		assert.Equal(t, "Failed to get theme", re.Message)
		assert.ErrorIs(t, re.Details, sentinel)
	})

	t.Run("missing category is CATEGORY_NOT_FOUND and lists are never fetched", func(t *testing.T) {
		svc, catRepo, _, th, wr, _, _, _ := setupBlogServiceTest(t)
		wr.EXPECT().GetByID(ctx, "workspace123").Return(blogGapWorkspace(), nil).Times(1)
		th.EXPECT().GetPublishedTheme(ctx).Return(theme, nil).Times(1)
		sentinel := errors.New("no rows")
		catRepo.EXPECT().GetCategoryBySlug(ctx, "ghost").Return(nil, sentinel).Times(1)
		_, err := svc.RenderCategoryPage(ctx, "workspace123", "ghost", 1, nil)
		re := blogGapRenderErr(t, err)
		assert.Equal(t, domain.ErrCodeCategoryNotFound, re.Code)
		assert.Equal(t, "Category not found", re.Message)
		assert.ErrorIs(t, re.Details, sentinel)
	})

	t.Run("page below one clamps to the first page", func(t *testing.T) {
		svc, catRepo, postRepo, th, wr, listRepo, _, _ := setupBlogServiceTest(t)
		wr.EXPECT().GetByID(ctx, "workspace123").Return(blogGapWorkspace(), nil).Times(1)
		th.EXPECT().GetPublishedTheme(ctx).Return(theme, nil).Times(1)
		catRepo.EXPECT().GetCategoryBySlug(ctx, "tech").Return(category, nil).Times(1)
		listRepo.EXPECT().GetLists(ctx, "workspace123").Return([]*domain.List{}, nil).Times(1)
		postRepo.EXPECT().ListPosts(ctx, gomock.Any()).
			DoAndReturn(func(_ context.Context, p domain.ListBlogPostsRequest) (*domain.BlogPostListResponse, error) {
				assert.Equal(t, 1, p.Page, "page 0 must clamp to 1")
				assert.Equal(t, "cat-1", p.CategoryID)
				assert.Equal(t, domain.BlogPostStatusPublished, p.Status)
				assert.Equal(t, 20, p.Limit, "default category page size")
				return &domain.BlogPostListResponse{Posts: []*domain.BlogPost{}, TotalCount: 0, TotalPages: 1}, nil
			}).Times(1)
		catRepo.EXPECT().ListCategories(ctx).Return([]*domain.BlogCategory{category}, nil).Times(1)
		html, err := svc.RenderCategoryPage(ctx, "workspace123", "tech", 0, nil)
		require.NoError(t, err)
		assert.Contains(t, html, "Technology")
		assert.Contains(t, html, "posts=0")
	})

	t.Run("list post and category failures degrade to an empty page", func(t *testing.T) {
		svc, catRepo, postRepo, th, wr, listRepo, _, _ := setupBlogServiceTest(t)
		wr.EXPECT().GetByID(ctx, "workspace123").Return(blogGapWorkspace(), nil).Times(1)
		th.EXPECT().GetPublishedTheme(ctx).Return(theme, nil).Times(1)
		catRepo.EXPECT().GetCategoryBySlug(ctx, "tech").Return(category, nil).Times(1)
		listRepo.EXPECT().GetLists(ctx, "workspace123").Return(nil, errors.New("lists down")).Times(1)
		postRepo.EXPECT().ListPosts(ctx, gomock.Any()).Return(nil, errors.New("posts down")).Times(1)
		catRepo.EXPECT().ListCategories(ctx).Return(nil, errors.New("cats down")).Times(1)
		html, err := svc.RenderCategoryPage(ctx, "workspace123", "tech", 1, nil)
		require.NoError(t, err, "secondary fetch failures must degrade, not fail the page")
		assert.Contains(t, html, "Technology")
		assert.Contains(t, html, "posts=0")
	})

	t.Run("invalid category liquid is INVALID_LIQUID_SYNTAX", func(t *testing.T) {
		badTheme := &domain.BlogTheme{Version: 1, Files: domain.BlogThemeFiles{CategoryLiquid: "{% invalid tag %}"}}
		svc, catRepo, postRepo, th, wr, listRepo, _, _ := setupBlogServiceTest(t)
		wr.EXPECT().GetByID(ctx, "workspace123").Return(blogGapWorkspace(), nil).Times(1)
		th.EXPECT().GetPublishedTheme(ctx).Return(badTheme, nil).Times(1)
		catRepo.EXPECT().GetCategoryBySlug(ctx, "tech").Return(category, nil).Times(1)
		listRepo.EXPECT().GetLists(ctx, "workspace123").Return([]*domain.List{}, nil).Times(1)
		postRepo.EXPECT().ListPosts(ctx, gomock.Any()).
			Return(&domain.BlogPostListResponse{Posts: []*domain.BlogPost{}, TotalCount: 0, TotalPages: 1}, nil).Times(1)
		catRepo.EXPECT().ListCategories(ctx).Return([]*domain.BlogCategory{category}, nil).Times(1)
		_, err := svc.RenderCategoryPage(ctx, "workspace123", "tech", 1, nil)
		re := blogGapRenderErr(t, err)
		assert.Equal(t, domain.ErrCodeInvalidLiquidSyntax, re.Code)
		assert.Contains(t, re.Message, "Failed to render category template:")
		require.Error(t, re.Details)
	})
}

func TestBlogGap_RenderPostPageTaxonomy(t *testing.T) {
	ctx := blogGapCtx()
	theme := &domain.BlogTheme{Version: 1, Files: domain.BlogThemeFiles{PostLiquid: "<h1>{{ post.title }}</h1><div>{{ post.content }}</div>"}}
	category := &domain.BlogCategory{ID: "cat-1", Slug: "tech", Settings: domain.BlogCategorySettings{Name: "Technology"}}

	t.Run("workspace failure is RENDER_FAILED", func(t *testing.T) {
		svc, _, _, _, wr, _, _, _ := setupBlogServiceTest(t)
		sentinel := errors.New("db down")
		wr.EXPECT().GetByID(ctx, "workspace123").Return(nil, sentinel).Times(1)
		_, err := svc.RenderPostPage(ctx, "workspace123", "tech", "test-post", nil)
		re := blogGapRenderErr(t, err)
		assert.Equal(t, domain.ErrCodeRenderFailed, re.Code)
		assert.Equal(t, "Failed to get workspace", re.Message)
		assert.ErrorIs(t, re.Details, sentinel)
	})

	t.Run("versioned theme is fetched by version, unpublished sentinel maps", func(t *testing.T) {
		svc, _, _, th, wr, _, _, _ := setupBlogServiceTest(t)
		wr.EXPECT().GetByID(ctx, "workspace123").Return(blogGapWorkspace(), nil).Times(1)
		version := 5
		th.EXPECT().GetTheme(ctx, 5).Return(nil, errors.New("sql: no rows in result set")).Times(1)
		// No GetPublishedTheme expectation: a versioned request must not
		// fall back to the published theme.
		_, err := svc.RenderPostPage(ctx, "workspace123", "tech", "test-post", &version)
		re := blogGapRenderErr(t, err)
		assert.Equal(t, domain.ErrCodeThemeNotPublished, re.Code)
		assert.Equal(t, "No published theme available", re.Message)
	})

	t.Run("other theme failures are THEME_NOT_FOUND", func(t *testing.T) {
		svc, _, _, th, wr, _, _, _ := setupBlogServiceTest(t)
		wr.EXPECT().GetByID(ctx, "workspace123").Return(blogGapWorkspace(), nil).Times(1)
		sentinel := errors.New("redis down")
		th.EXPECT().GetPublishedTheme(ctx).Return(nil, sentinel).Times(1)
		_, err := svc.RenderPostPage(ctx, "workspace123", "tech", "test-post", nil)
		re := blogGapRenderErr(t, err)
		assert.Equal(t, domain.ErrCodeThemeNotFound, re.Code)
		assert.Equal(t, "Failed to get theme", re.Message)
		assert.ErrorIs(t, re.Details, sentinel)
	})

	t.Run("draft post is POST_NOT_FOUND with no details", func(t *testing.T) {
		svc, _, postRepo, th, wr, _, _, _ := setupBlogServiceTest(t)
		wr.EXPECT().GetByID(ctx, "workspace123").Return(blogGapWorkspace(), nil).Times(1)
		th.EXPECT().GetPublishedTheme(ctx).Return(theme, nil).Times(1)
		draft := blogGapPublishedPost()
		draft.PublishedAt = nil
		postRepo.EXPECT().GetPostByCategoryAndSlug(ctx, "tech", "test-post").Return(draft, nil).Times(1)
		// No category/list/template expectations: an unpublished post
		// must stop before any of those lookups.
		_, err := svc.RenderPostPage(ctx, "workspace123", "tech", "test-post", nil)
		re := blogGapRenderErr(t, err)
		assert.Equal(t, domain.ErrCodePostNotFound, re.Code)
		assert.Equal(t, "Post is not published", re.Message)
		assert.Nil(t, re.Details)
		assert.Equal(t, "post_not_found: Post is not published", err.Error())
	})

	t.Run("category list and template-adjacent failures degrade", func(t *testing.T) {
		svc, catRepo, postRepo, th, wr, listRepo, tplRepo, _ := setupBlogServiceTest(t)
		wr.EXPECT().GetByID(ctx, "workspace123").Return(blogGapWorkspace(), nil).Times(1)
		th.EXPECT().GetPublishedTheme(ctx).Return(theme, nil).Times(1)
		postRepo.EXPECT().GetPostByCategoryAndSlug(ctx, "tech", "test-post").
			Return(blogGapPublishedPost(), nil).Times(1)
		catRepo.EXPECT().GetCategory(ctx, "cat-1").Return(nil, errors.New("cat gone")).Times(1)
		listRepo.EXPECT().GetLists(ctx, "workspace123").Return(nil, errors.New("lists down")).Times(1)
		catRepo.EXPECT().ListCategories(ctx).Return(nil, errors.New("cats down")).Times(1)
		tplRepo.EXPECT().GetTemplateByID(ctx, "workspace123", "tpl-1", int64(1)).
			Return(&domain.Template{ID: "tpl-1", Version: 1, Web: &domain.WebTemplate{HTML: "<p>Body</p>"}}, nil).Times(1)
		html, err := svc.RenderPostPage(ctx, "workspace123", "tech", "test-post", nil)
		require.NoError(t, err, "missing category/navigation must degrade, not fail the page")
		assert.Contains(t, html, "Test Post")
		assert.Contains(t, html, "Body")
	})

	t.Run("invalid post liquid is INVALID_LIQUID_SYNTAX", func(t *testing.T) {
		badTheme := &domain.BlogTheme{Version: 1, Files: domain.BlogThemeFiles{PostLiquid: "{% invalid tag %}"}}
		svc, catRepo, postRepo, th, wr, listRepo, tplRepo, _ := setupBlogServiceTest(t)
		wr.EXPECT().GetByID(ctx, "workspace123").Return(blogGapWorkspace(), nil).Times(1)
		th.EXPECT().GetPublishedTheme(ctx).Return(badTheme, nil).Times(1)
		postRepo.EXPECT().GetPostByCategoryAndSlug(ctx, "tech", "test-post").
			Return(blogGapPublishedPost(), nil).Times(1)
		catRepo.EXPECT().GetCategory(ctx, "cat-1").Return(category, nil).Times(1)
		listRepo.EXPECT().GetLists(ctx, "workspace123").Return([]*domain.List{}, nil).Times(1)
		catRepo.EXPECT().ListCategories(ctx).Return([]*domain.BlogCategory{category}, nil).Times(1)
		tplRepo.EXPECT().GetTemplateByID(ctx, "workspace123", "tpl-1", int64(1)).
			Return(&domain.Template{ID: "tpl-1", Version: 1, Web: &domain.WebTemplate{HTML: "<p>Body</p>"}}, nil).Times(1)
		_, err := svc.RenderPostPage(ctx, "workspace123", "tech", "test-post", nil)
		re := blogGapRenderErr(t, err)
		assert.Equal(t, domain.ErrCodeInvalidLiquidSyntax, re.Code)
		assert.Contains(t, re.Message, "Failed to render post template:")
		require.Error(t, re.Details)
	})
}
