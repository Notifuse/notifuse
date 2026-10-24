package service

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/internal/domain/mocks"
)

// The blog read paths share one guard chain: workspace in context, then
// authentication, then the read permission, then the repository. A regression
// that reorders or drops a step (e.g. hitting the repository before the
// permission check) leaks data across workspaces, so each test below either
// expects the guarded error or relies on gomock failing the test if the
// repository is touched after a guard rejects.

func blogCtxNoWorkspace() context.Context { return context.Background() }

func blogCtxWithWorkspace(workspaceID string) context.Context {
	return context.WithValue(context.Background(), domain.WorkspaceIDKey, workspaceID)
}

func expectBlogAuthError(auth *mocks.MockAuthService, workspaceID string, err error) {
	auth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
		Return(context.Background(), nil, nil, err).Times(1)
}

func expectBlogAuth(auth *mocks.MockAuthService, workspaceID string, read, write bool) context.Context {
	ctx := blogCtxWithWorkspace(workspaceID)
	userWorkspace := &domain.UserWorkspace{
		UserID:      "user-1",
		WorkspaceID: workspaceID,
		Role:        "member",
		Permissions: domain.UserPermissions{
			domain.PermissionResourceBlog: domain.ResourcePermissions{Read: read, Write: write},
		},
	}
	auth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), workspaceID).
		Return(ctx, &domain.User{ID: "user-1"}, userWorkspace, nil).Times(1)
	return ctx
}

func TestBlogService_GetCategory_Guards(t *testing.T) {
	t.Run("missing workspace in context fails before auth", func(t *testing.T) {
		svc, _, _, _, _, _, _, _ := setupBlogServiceTest(t)
		_, err := svc.GetCategory(blogCtxNoWorkspace(), "cat-1")
		require.EqualError(t, err, "workspace_id not found in context")
	})

	t.Run("auth failure is wrapped and the repo is not called", func(t *testing.T) {
		svc, _, _, _, _, _, _, auth := setupBlogServiceTest(t)
		sentinel := errors.New("token expired")
		expectBlogAuthError(auth, "ws-1", sentinel)
		_, err := svc.GetCategory(blogCtxWithWorkspace("ws-1"), "cat-1")
		require.ErrorContains(t, err, "failed to authenticate user:")
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("without read permission the repo is not called", func(t *testing.T) {
		svc, _, _, _, _, _, _, auth := setupBlogServiceTest(t)
		ctx := expectBlogAuth(auth, "ws-1", false, true)
		_, err := svc.GetCategory(ctx, "cat-1")
		require.Error(t, err)
		assert.Equal(t, "Insufficient permissions: read access to blog required", err.Error())
		var permErr *domain.PermissionError
		assert.ErrorAs(t, err, &permErr)
	})

	t.Run("repo error reaches the caller unwrapped", func(t *testing.T) {
		svc, catRepo, _, _, _, _, _, auth := setupBlogServiceTest(t)
		ctx := expectBlogAuth(auth, "ws-1", true, false)
		sentinel := errors.New("category gone")
		catRepo.EXPECT().GetCategory(gomock.Any(), "cat-9").Return(nil, sentinel).Times(1)
		_, err := svc.GetCategory(ctx, "cat-9")
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("the exact category from the repo is returned", func(t *testing.T) {
		svc, catRepo, _, _, _, _, _, auth := setupBlogServiceTest(t)
		ctx := expectBlogAuth(auth, "ws-1", true, false)
		want := &domain.BlogCategory{ID: "cat-1"}
		catRepo.EXPECT().GetCategory(gomock.Any(), "cat-1").Return(want, nil).Times(1)
		got, err := svc.GetCategory(ctx, "cat-1")
		require.NoError(t, err)
		assert.Same(t, want, got)
	})
}

func TestBlogService_GetCategoryBySlug_Guards(t *testing.T) {
	t.Run("missing workspace in context fails before auth", func(t *testing.T) {
		svc, _, _, _, _, _, _, _ := setupBlogServiceTest(t)
		_, err := svc.GetCategoryBySlug(blogCtxNoWorkspace(), "tech")
		require.EqualError(t, err, "workspace_id not found in context")
	})

	t.Run("auth failure is wrapped", func(t *testing.T) {
		svc, _, _, _, _, _, _, auth := setupBlogServiceTest(t)
		expectBlogAuthError(auth, "ws-1", errors.New("no session"))
		_, err := svc.GetCategoryBySlug(blogCtxWithWorkspace("ws-1"), "tech")
		require.ErrorContains(t, err, "failed to authenticate user:")
	})

	t.Run("without read permission the repo is not called", func(t *testing.T) {
		svc, _, _, _, _, _, _, auth := setupBlogServiceTest(t)
		ctx := expectBlogAuth(auth, "ws-1", false, false)
		_, err := svc.GetCategoryBySlug(ctx, "tech")
		require.EqualError(t, err, "Insufficient permissions: read access to blog required")
	})

	t.Run("slug is forwarded verbatim and the result returned", func(t *testing.T) {
		svc, catRepo, _, _, _, _, _, auth := setupBlogServiceTest(t)
		ctx := expectBlogAuth(auth, "ws-1", true, false)
		want := &domain.BlogCategory{ID: "cat-1", Slug: "tech"}
		catRepo.EXPECT().GetCategoryBySlug(gomock.Any(), "tech").Return(want, nil).Times(1)
		got, err := svc.GetCategoryBySlug(ctx, "tech")
		require.NoError(t, err)
		assert.Same(t, want, got)
	})
}

func TestBlogService_GetPost_Guards(t *testing.T) {
	t.Run("missing workspace in context fails before auth", func(t *testing.T) {
		svc, _, _, _, _, _, _, _ := setupBlogServiceTest(t)
		_, err := svc.GetPost(blogCtxNoWorkspace(), "post-1")
		require.EqualError(t, err, "workspace_id not found in context")

		svc2, _, _, _, _, _, _, _ := setupBlogServiceTest(t)
		_, err = svc2.GetPostBySlug(blogCtxNoWorkspace(), "hello")
		require.EqualError(t, err, "workspace_id not found in context")
	})

	t.Run("auth failure is wrapped and the repo is not called", func(t *testing.T) {
		svc, _, _, _, _, _, _, auth := setupBlogServiceTest(t)
		expectBlogAuthError(auth, "ws-1", errors.New("banned"))
		_, err := svc.GetPost(blogCtxWithWorkspace("ws-1"), "post-1")
		require.ErrorContains(t, err, "failed to authenticate user:")
	})

	t.Run("without read permission neither post lookup reaches the repo", func(t *testing.T) {
		svc, _, _, _, _, _, _, auth := setupBlogServiceTest(t)
		ctx := expectBlogAuth(auth, "ws-1", false, true)
		_, err := svc.GetPost(ctx, "post-1")
		require.EqualError(t, err, "Insufficient permissions: read access to blog required")

		svc2, _, _, _, _, _, _, auth2 := setupBlogServiceTest(t)
		ctx2 := expectBlogAuth(auth2, "ws-1", false, true)
		_, err = svc2.GetPostBySlug(ctx2, "hello")
		require.EqualError(t, err, "Insufficient permissions: read access to blog required")
	})

	t.Run("id and slug are forwarded and results returned", func(t *testing.T) {
		svc, _, postRepo, _, _, _, _, auth := setupBlogServiceTest(t)
		ctx := expectBlogAuth(auth, "ws-1", true, false)
		byID := &domain.BlogPost{ID: "post-1"}
		postRepo.EXPECT().GetPost(gomock.Any(), "post-1").Return(byID, nil).Times(1)
		got, err := svc.GetPost(ctx, "post-1")
		require.NoError(t, err)
		assert.Same(t, byID, got)

		svc2, _, postRepo2, _, _, _, _, auth2 := setupBlogServiceTest(t)
		ctx2 := expectBlogAuth(auth2, "ws-1", true, false)
		bySlug := &domain.BlogPost{ID: "post-2", Slug: "hello"}
		postRepo2.EXPECT().GetPostBySlug(gomock.Any(), "hello").Return(bySlug, nil).Times(1)
		got, err = svc2.GetPostBySlug(ctx2, "hello")
		require.NoError(t, err)
		assert.Same(t, bySlug, got)
	})

	t.Run("repo not-found error reaches the caller", func(t *testing.T) {
		svc, _, postRepo, _, _, _, _, auth := setupBlogServiceTest(t)
		ctx := expectBlogAuth(auth, "ws-1", true, false)
		sentinel := errors.New("no such post")
		postRepo.EXPECT().GetPost(gomock.Any(), "ghost").Return(nil, sentinel).Times(1)
		_, err := svc.GetPost(ctx, "ghost")
		assert.ErrorIs(t, err, sentinel)
	})
}

func TestBlogService_ListCategories_Guards(t *testing.T) {
	t.Run("guards run in order", func(t *testing.T) {
		svc, _, _, _, _, _, _, _ := setupBlogServiceTest(t)
		_, err := svc.ListCategories(blogCtxNoWorkspace())
		require.EqualError(t, err, "workspace_id not found in context")

		svc2, _, _, _, _, _, _, auth2 := setupBlogServiceTest(t)
		expectBlogAuthError(auth2, "ws-1", errors.New("expired"))
		_, err = svc2.ListCategories(blogCtxWithWorkspace("ws-1"))
		require.ErrorContains(t, err, "failed to authenticate user:")

		svc3, _, _, _, _, _, _, auth3 := setupBlogServiceTest(t)
		ctx3 := expectBlogAuth(auth3, "ws-1", false, false)
		_, err = svc3.ListCategories(ctx3)
		require.EqualError(t, err, "Insufficient permissions: read access to blog required")
	})

	t.Run("repo error is wrapped with the operation name", func(t *testing.T) {
		svc, catRepo, _, _, _, _, _, auth := setupBlogServiceTest(t)
		ctx := expectBlogAuth(auth, "ws-1", true, false)
		sentinel := errors.New("db down")
		catRepo.EXPECT().ListCategories(gomock.Any()).Return(nil, sentinel).Times(1)
		_, err := svc.ListCategories(ctx)
		require.ErrorContains(t, err, "failed to list categories:")
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("total count matches the returned slice", func(t *testing.T) {
		svc, catRepo, _, _, _, _, _, auth := setupBlogServiceTest(t)
		ctx := expectBlogAuth(auth, "ws-1", true, false)
		cats := []*domain.BlogCategory{{ID: "c1"}, {ID: "c2"}, {ID: "c3"}}
		catRepo.EXPECT().ListCategories(gomock.Any()).Return(cats, nil).Times(1)
		resp, err := svc.ListCategories(ctx)
		require.NoError(t, err)
		assert.Equal(t, 3, resp.TotalCount)
		assert.Equal(t, cats, resp.Categories)
	})
}

func TestBlogService_CreatePost_Branches(t *testing.T) {
	validReq := func() *domain.CreateBlogPostRequest {
		return &domain.CreateBlogPostRequest{
			CategoryID: "cat-1", Slug: "hello-world", Title: "Hello", TemplateID: "tpl-1",
		}
	}

	t.Run("write permission is required, not just read", func(t *testing.T) {
		svc, _, _, _, _, _, _, auth := setupBlogServiceTest(t)
		ctx := expectBlogAuth(auth, "ws-1", true, false)
		_, err := svc.CreatePost(ctx, validReq())
		require.EqualError(t, err, "Insufficient permissions: write access to blog required")
	})

	t.Run("an invalid request fails before any repo call", func(t *testing.T) {
		svc, _, _, _, _, _, _, auth := setupBlogServiceTest(t)
		ctx := expectBlogAuth(auth, "ws-1", true, true)
		req := validReq()
		req.Slug = "Bad Slug!"
		_, err := svc.CreatePost(ctx, req)
		require.EqualError(t, err, "slug must contain only lowercase letters, numbers, and hyphens")
	})

	t.Run("an existing slug is a conflict naming the slug", func(t *testing.T) {
		svc, _, postRepo, _, _, _, _, auth := setupBlogServiceTest(t)
		ctx := expectBlogAuth(auth, "ws-1", true, true)
		postRepo.EXPECT().GetPostBySlug(gomock.Any(), "hello-world").
			Return(&domain.BlogPost{ID: "old"}, nil).Times(1)
		_, err := svc.CreatePost(ctx, validReq())
		require.EqualError(t, err, "post with slug 'hello-world' already exists")
	})

	t.Run("a slug-lookup error does not block creation", func(t *testing.T) {
		// GetPostBySlug errors are treated as "not found": creation proceeds
		// to the category check. Pin it so a future change does not turn a
		// flaky lookup into a hard failure — or silently skip the check.
		svc, catRepo, postRepo, _, _, _, _, auth := setupBlogServiceTest(t)
		ctx := expectBlogAuth(auth, "ws-1", true, true)
		postRepo.EXPECT().GetPostBySlug(gomock.Any(), "hello-world").
			Return(nil, errors.New("not found")).Times(1)
		catRepo.EXPECT().GetCategory(gomock.Any(), "cat-1").
			Return(&domain.BlogCategory{ID: "cat-1"}, nil).Times(1)
		postRepo.EXPECT().CreatePost(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, p *domain.BlogPost) error {
				assert.Equal(t, "hello-world", p.Slug)
				assert.Nil(t, p.PublishedAt, "new posts start as drafts")
				return nil
			}).Times(1)
		post, err := svc.CreatePost(ctx, validReq())
		require.NoError(t, err)
		assert.Equal(t, "cat-1", post.CategoryID)
		assert.NotEmpty(t, post.ID)
	})

	t.Run("a missing category aborts before persisting", func(t *testing.T) {
		svc, catRepo, postRepo, _, _, _, _, auth := setupBlogServiceTest(t)
		ctx := expectBlogAuth(auth, "ws-1", true, true)
		postRepo.EXPECT().GetPostBySlug(gomock.Any(), "hello-world").
			Return(nil, errors.New("not found")).Times(1)
		sentinel := errors.New("no category")
		catRepo.EXPECT().GetCategory(gomock.Any(), "cat-1").Return(nil, sentinel).Times(1)
		_, err := svc.CreatePost(ctx, validReq())
		require.ErrorContains(t, err, "category not found:")
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("a persist failure is wrapped with the operation name", func(t *testing.T) {
		svc, catRepo, postRepo, _, _, _, _, auth := setupBlogServiceTest(t)
		ctx := expectBlogAuth(auth, "ws-1", true, true)
		postRepo.EXPECT().GetPostBySlug(gomock.Any(), "hello-world").
			Return(nil, errors.New("not found")).Times(1)
		catRepo.EXPECT().GetCategory(gomock.Any(), "cat-1").
			Return(&domain.BlogCategory{ID: "cat-1"}, nil).Times(1)
		sentinel := errors.New("insert failed")
		postRepo.EXPECT().CreatePost(gomock.Any(), gomock.Any()).Return(sentinel).Times(1)
		_, err := svc.CreatePost(ctx, validReq())
		require.ErrorContains(t, err, "failed to create post:")
		assert.ErrorIs(t, err, sentinel)
	})
}

func TestBlogService_ListThemes_GuardsAndClamping(t *testing.T) {
	t.Run("guards run in order", func(t *testing.T) {
		svc, _, _, _, _, _, _, _ := setupBlogServiceTest(t)
		_, err := svc.ListThemes(blogCtxNoWorkspace(), &domain.ListBlogThemesRequest{})
		require.EqualError(t, err, "workspace_id not found in context")

		svc2, _, _, _, _, _, _, auth2 := setupBlogServiceTest(t)
		ctx2 := expectBlogAuth(auth2, "ws-1", false, false)
		_, err = svc2.ListThemes(ctx2, &domain.ListBlogThemesRequest{})
		require.EqualError(t, err, "Insufficient permissions: read access to blog required")
	})

	t.Run("limit and offset are clamped before reaching the repo", func(t *testing.T) {
		svc, _, _, themeRepo, _, _, _, auth := setupBlogServiceTest(t)
		ctx := expectBlogAuth(auth, "ws-1", true, false)
		themeRepo.EXPECT().ListThemes(gomock.Any(), gomock.AssignableToTypeOf(domain.ListBlogThemesRequest{})).
			DoAndReturn(func(_ context.Context, req domain.ListBlogThemesRequest) (*domain.BlogThemeListResponse, error) {
				assert.Equal(t, 100, req.Limit, "limit clamps to the 100 maximum")
				assert.Equal(t, 0, req.Offset, "negative offset clamps to 0")
				return &domain.BlogThemeListResponse{}, nil
			}).Times(1)
		_, err := svc.ListThemes(ctx, &domain.ListBlogThemesRequest{Limit: 500, Offset: -3})
		require.NoError(t, err)
	})
}
