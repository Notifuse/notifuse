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
)

// Write-path chains for posts: publish/unpublish/delete verify existence
// before mutating, require write (not read) permission, and wrap repo
// failures with the operation name. A chain that mutated before verifying
// — or accepted a read-only user — would corrupt or leak the blog.

func TestBlogService_PublishPost_Chain(t *testing.T) {
	publishedAt := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)

	t.Run("write permission is required", func(t *testing.T) {
		svc, _, _, _, _, _, _, auth := setupBlogServiceTest(t)
		ctx := expectBlogAuth(auth, "ws-1", true, false)
		err := svc.PublishPost(ctx, &domain.PublishBlogPostRequest{ID: "post-1"})
		require.EqualError(t, err, "Insufficient permissions: write access to blog required")
	})

	t.Run("missing id fails validation before any repo call", func(t *testing.T) {
		svc, _, _, _, _, _, _, auth := setupBlogServiceTest(t)
		ctx := expectBlogAuth(auth, "ws-1", true, true)
		err := svc.PublishPost(ctx, &domain.PublishBlogPostRequest{})
		require.EqualError(t, err, "id is required")
	})

	t.Run("a missing post aborts before publishing", func(t *testing.T) {
		svc, _, postRepo, _, _, _, _, auth := setupBlogServiceTest(t)
		ctx := expectBlogAuth(auth, "ws-1", true, true)
		sentinel := errors.New("no rows")
		postRepo.EXPECT().GetPost(gomock.Any(), "post-1").Return(nil, sentinel).Times(1)
		err := svc.PublishPost(ctx, &domain.PublishBlogPostRequest{ID: "post-1"})
		require.ErrorContains(t, err, "failed to get post:")
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("publish failure is wrapped with the operation", func(t *testing.T) {
		svc, _, postRepo, _, _, _, _, auth := setupBlogServiceTest(t)
		ctx := expectBlogAuth(auth, "ws-1", true, true)
		postRepo.EXPECT().GetPost(gomock.Any(), "post-1").Return(&domain.BlogPost{ID: "post-1"}, nil).Times(1)
		sentinel := errors.New("update failed")
		postRepo.EXPECT().PublishPost(gomock.Any(), "post-1", &publishedAt).Return(sentinel).Times(1)
		err := svc.PublishPost(ctx, &domain.PublishBlogPostRequest{ID: "post-1", PublishedAt: &publishedAt})
		require.ErrorContains(t, err, "failed to publish post:")
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("the requested timestamp is forwarded verbatim", func(t *testing.T) {
		svc, _, postRepo, _, _, _, _, auth := setupBlogServiceTest(t)
		ctx := expectBlogAuth(auth, "ws-1", true, true)
		postRepo.EXPECT().GetPost(gomock.Any(), "post-1").Return(&domain.BlogPost{ID: "post-1"}, nil).Times(1)
		postRepo.EXPECT().PublishPost(gomock.Any(), "post-1", &publishedAt).Return(nil).Times(1)
		require.NoError(t, svc.PublishPost(ctx, &domain.PublishBlogPostRequest{ID: "post-1", PublishedAt: &publishedAt}))
	})
}

func TestBlogService_UnpublishPost_Chain(t *testing.T) {
	t.Run("guards and existence run before the mutation", func(t *testing.T) {
		svc, _, _, _, _, _, _, _ := setupBlogServiceTest(t)
		err := svc.UnpublishPost(blogCtxNoWorkspace(), &domain.UnpublishBlogPostRequest{ID: "post-1"})
		require.EqualError(t, err, "workspace_id not found in context")

		svc2, _, postRepo2, _, _, _, _, auth2 := setupBlogServiceTest(t)
		ctx2 := expectBlogAuth(auth2, "ws-1", true, true)
		sentinel := errors.New("no rows")
		postRepo2.EXPECT().GetPost(gomock.Any(), "post-1").Return(nil, sentinel).Times(1)
		err = svc2.UnpublishPost(ctx2, &domain.UnpublishBlogPostRequest{ID: "post-1"})
		require.ErrorContains(t, err, "failed to get post:")
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("unpublish failure is wrapped, success passes", func(t *testing.T) {
		svc, _, postRepo, _, _, _, _, auth := setupBlogServiceTest(t)
		ctx := expectBlogAuth(auth, "ws-1", true, true)
		postRepo.EXPECT().GetPost(gomock.Any(), "post-1").Return(&domain.BlogPost{ID: "post-1"}, nil).Times(1)
		sentinel := errors.New("update failed")
		postRepo.EXPECT().UnpublishPost(gomock.Any(), "post-1").Return(sentinel).Times(1)
		err := svc.UnpublishPost(ctx, &domain.UnpublishBlogPostRequest{ID: "post-1"})
		require.ErrorContains(t, err, "failed to unpublish post:")
		assert.ErrorIs(t, err, sentinel)

		svc2, _, postRepo2, _, _, _, _, auth2 := setupBlogServiceTest(t)
		ctx2 := expectBlogAuth(auth2, "ws-1", true, true)
		postRepo2.EXPECT().GetPost(gomock.Any(), "post-1").Return(&domain.BlogPost{ID: "post-1"}, nil).Times(1)
		postRepo2.EXPECT().UnpublishPost(gomock.Any(), "post-1").Return(nil).Times(1)
		require.NoError(t, svc2.UnpublishPost(ctx2, &domain.UnpublishBlogPostRequest{ID: "post-1"}))
	})
}

func TestBlogService_DeletePost_Chain(t *testing.T) {
	t.Run("a missing post is reported as not found, not deleted blindly", func(t *testing.T) {
		svc, _, postRepo, _, _, _, _, auth := setupBlogServiceTest(t)
		ctx := expectBlogAuth(auth, "ws-1", true, true)
		sentinel := errors.New("no rows")
		postRepo.EXPECT().GetPost(gomock.Any(), "post-1").Return(nil, sentinel).Times(1)
		err := svc.DeletePost(ctx, &domain.DeleteBlogPostRequest{ID: "post-1"})
		require.ErrorContains(t, err, "post not found:")
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("delete failure is wrapped with the operation", func(t *testing.T) {
		svc, _, postRepo, _, _, _, _, auth := setupBlogServiceTest(t)
		ctx := expectBlogAuth(auth, "ws-1", true, true)
		postRepo.EXPECT().GetPost(gomock.Any(), "post-1").Return(&domain.BlogPost{ID: "post-1"}, nil).Times(1)
		sentinel := errors.New("fk violation")
		postRepo.EXPECT().DeletePost(gomock.Any(), "post-1").Return(sentinel).Times(1)
		err := svc.DeletePost(ctx, &domain.DeleteBlogPostRequest{ID: "post-1"})
		require.ErrorContains(t, err, "failed to delete post:")
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("write permission and validation gate the delete", func(t *testing.T) {
		svc, _, _, _, _, _, _, auth := setupBlogServiceTest(t)
		ctx := expectBlogAuth(auth, "ws-1", true, false)
		err := svc.DeletePost(ctx, &domain.DeleteBlogPostRequest{ID: "post-1"})
		require.EqualError(t, err, "Insufficient permissions: write access to blog required")

		svc2, _, _, _, _, _, _, auth2 := setupBlogServiceTest(t)
		ctx2 := expectBlogAuth(auth2, "ws-1", true, true)
		err = svc2.DeletePost(ctx2, &domain.DeleteBlogPostRequest{})
		require.EqualError(t, err, "id is required")
	})
}

func TestBlogService_ListPosts_Chain(t *testing.T) {
	t.Run("defaults are applied before the repo sees the params", func(t *testing.T) {
		svc, _, postRepo, _, _, _, _, auth := setupBlogServiceTest(t)
		ctx := expectBlogAuth(auth, "ws-1", true, false)
		postRepo.EXPECT().ListPosts(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, params domain.ListBlogPostsRequest) (*domain.BlogPostListResponse, error) {
				assert.Equal(t, domain.BlogPostStatusAll, params.Status, "empty status defaults to all")
				return &domain.BlogPostListResponse{TotalCount: 0}, nil
			}).Times(1)
		resp, err := svc.ListPosts(ctx, &domain.ListBlogPostsRequest{})
		require.NoError(t, err)
		assert.Equal(t, 0, resp.TotalCount)
	})

	t.Run("read permission suffices, repo errors pass through", func(t *testing.T) {
		svc, _, postRepo, _, _, _, _, auth := setupBlogServiceTest(t)
		ctx := expectBlogAuth(auth, "ws-1", true, false)
		sentinel := errors.New("query failed")
		postRepo.EXPECT().ListPosts(gomock.Any(), gomock.Any()).Return(nil, sentinel).Times(1)
		_, err := svc.ListPosts(ctx, &domain.ListBlogPostsRequest{Status: domain.BlogPostStatusPublished})
		assert.ErrorIs(t, err, sentinel)
	})
}

func TestBlogService_ThemeChains(t *testing.T) {
	t.Run("GetPublishedTheme guards then delegates", func(t *testing.T) {
		svc, _, _, _, _, _, _, _ := setupBlogServiceTest(t)
		_, err := svc.GetPublishedTheme(blogCtxNoWorkspace())
		require.EqualError(t, err, "workspace_id not found in context")

		svc2, _, _, themeRepo2, _, _, _, auth2 := setupBlogServiceTest(t)
		ctx2 := expectBlogAuth(auth2, "ws-1", true, false)
		want := &domain.BlogTheme{Version: 3}
		themeRepo2.EXPECT().GetPublishedTheme(gomock.Any()).Return(want, nil).Times(1)
		got, err := svc2.GetPublishedTheme(ctx2)
		require.NoError(t, err)
		assert.Same(t, want, got)

		svc3, _, _, _, _, _, _, auth3 := setupBlogServiceTest(t)
		ctx3 := expectBlogAuth(auth3, "ws-1", false, false)
		_, err = svc3.GetPublishedTheme(ctx3)
		require.EqualError(t, err, "Insufficient permissions: read access to blog required")
	})

	t.Run("GetTheme forwards the version and propagates repo errors", func(t *testing.T) {
		svc, _, _, themeRepo, _, _, _, auth := setupBlogServiceTest(t)
		ctx := expectBlogAuth(auth, "ws-1", true, false)
		sentinel := errors.New("no theme at that version")
		themeRepo.EXPECT().GetTheme(gomock.Any(), 9).Return(nil, sentinel).Times(1)
		_, err := svc.GetTheme(ctx, 9)
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("CreateTheme requires write and wraps repo failure", func(t *testing.T) {
		svc, _, _, _, _, _, _, auth := setupBlogServiceTest(t)
		ctx := expectBlogAuth(auth, "ws-1", true, false)
		_, err := svc.CreateTheme(ctx, &domain.CreateBlogThemeRequest{})
		require.EqualError(t, err, "Insufficient permissions: write access to blog required")

		svc2, _, _, themeRepo2, _, _, _, auth2 := setupBlogServiceTest(t)
		ctx2 := expectBlogAuth(auth2, "ws-1", true, true)
		sentinel := errors.New("insert failed")
		themeRepo2.EXPECT().CreateTheme(gomock.Any(), gomock.Any()).Return(sentinel).Times(1)
		_, err = svc2.CreateTheme(ctx2, &domain.CreateBlogThemeRequest{})
		require.ErrorContains(t, err, "failed to create theme:")
		assert.ErrorIs(t, err, sentinel)
	})
}

func TestBlogService_ListPublicPosts_ForcesPublished(t *testing.T) {
	svc, _, postRepo, _, _, _, _, _ := setupBlogServiceTest(t)
	postRepo.EXPECT().ListPosts(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, params domain.ListBlogPostsRequest) (*domain.BlogPostListResponse, error) {
			assert.Equal(t, domain.BlogPostStatusPublished, params.Status,
				"public listing must never leak drafts, whatever the caller asked for")
			return &domain.BlogPostListResponse{TotalCount: 2}, nil
		}).Times(1)

	params := &domain.ListBlogPostsRequest{Status: domain.BlogPostStatusDraft, Limit: 10, Page: 1}
	resp, err := svc.ListPublicPosts(context.Background(), params)
	require.NoError(t, err)
	assert.Equal(t, 2, resp.TotalCount)
	assert.Equal(t, domain.BlogPostStatusPublished, params.Status,
		"the caller's params are rewritten in place — pin it so it stays deliberate")
}

func TestBlogService_UpdatePost_Chain(t *testing.T) {
	t.Run("update requires the post to exist and the slug to stay unique", func(t *testing.T) {
		svc, _, postRepo, _, _, _, _, auth := setupBlogServiceTest(t)
		ctx := expectBlogAuth(auth, "ws-1", true, true)
		req := &domain.UpdateBlogPostRequest{ID: "post-1", CategoryID: "cat-1", Slug: "fresh-slug", Title: "T", TemplateID: "tpl-1"}
		postRepo.EXPECT().GetPost(gomock.Any(), "post-1").
			Return(&domain.BlogPost{ID: "post-1", Slug: "old-slug"}, nil).Times(1)
		postRepo.EXPECT().GetPostBySlug(gomock.Any(), "fresh-slug").
			Return(&domain.BlogPost{ID: "post-2"}, nil).Times(1)
		_, err := svc.UpdatePost(ctx, req)
		require.ErrorContains(t, err, "already exists")
	})

	t.Run("keeping the same slug does not self-conflict", func(t *testing.T) {
		svc, catRepo, postRepo, _, _, _, _, auth := setupBlogServiceTest(t)
		ctx := expectBlogAuth(auth, "ws-1", true, true)
		req := &domain.UpdateBlogPostRequest{ID: "post-1", CategoryID: "cat-1", Slug: "same-slug", Title: "T", TemplateID: "tpl-1"}
		postRepo.EXPECT().GetPost(gomock.Any(), "post-1").
			Return(&domain.BlogPost{ID: "post-1", Slug: "same-slug"}, nil).Times(1)
		catRepo.EXPECT().GetCategory(gomock.Any(), "cat-1").
			Return(&domain.BlogCategory{ID: "cat-1"}, nil).Times(1)
		postRepo.EXPECT().UpdatePost(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, post *domain.BlogPost) error {
				assert.Equal(t, "same-slug", post.Slug)
				assert.Equal(t, "T", post.Settings.Title)
				return nil
			}).Times(1)
		_, err := svc.UpdatePost(ctx, req)
		require.NoError(t, err)
	})
}
