package service

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
)

func renderHomeCtx() context.Context {
	return context.WithValue(context.Background(), domain.WorkspaceIDKey, "workspace123")
}

// A published-theme lookup failure renders a distinct error code from a
// missing workspace, and the sentinel "no published theme found" string from
// repositories maps to THEME_NOT_PUBLISHED (theme_version forces versioned
// lookup).
func TestRenderHomePage_ErrorTaxonomy(t *testing.T) {
	ctx := renderHomeCtx()
	workspace := &domain.Workspace{ID: "workspace123", Name: "WS", Settings: domain.WorkspaceSettings{Timezone: "UTC"}}
	_ = ctx
	t.Run("workspace failure is RENDER_FAILED", func(t *testing.T) {
		s, _, _, _, wr, _, _, _ := setupBlogServiceTest(t)
		wr.EXPECT().GetByID(ctx, "workspace123").Return(nil, errors.New("db down"))
		_, err := s.RenderHomePage(ctx, "workspace123", 1, nil)
		var re *domain.BlogRenderError
		require.ErrorAs(t, err, &re)
		assert.Equal(t, domain.ErrCodeRenderFailed, re.Code)
		assert.Equal(t, "Failed to get workspace", re.Message)
	})

	t.Run("no published theme maps to THEME_NOT_PUBLISHED", func(t *testing.T) {
		s, _, _, th, wr, _, _, _ := setupBlogServiceTest(t)
		wr.EXPECT().GetByID(ctx, "workspace123").Return(workspace, nil)
		th.EXPECT().GetPublishedTheme(ctx).Return(nil, errors.New("no published theme found"))
		_, err := s.RenderHomePage(ctx, "workspace123", 1, nil)
		var re *domain.BlogRenderError
		require.ErrorAs(t, err, &re)
		assert.Equal(t, domain.ErrCodeThemeNotPublished, re.Code)
		assert.Equal(t, "No published theme available", re.Message)
	})

	t.Run("other theme failures map to THEME_NOT_FOUND", func(t *testing.T) {
		s, _, _, th, wr, _, _, _ := setupBlogServiceTest(t)
		wr.EXPECT().GetByID(ctx, "workspace123").Return(workspace, nil)
		th.EXPECT().GetPublishedTheme(ctx).Return(nil, errors.New("redis down"))
		_, err := s.RenderHomePage(ctx, "workspace123", 1, nil)
		var re *domain.BlogRenderError
		require.ErrorAs(t, err, &re)
		assert.Equal(t, domain.ErrCodeThemeNotFound, re.Code)
	})
}

// Post/list fetch failures degrade to an empty home page (never a 500), while
// asking for a page beyond the last one is a real 404 — the two paths must
// not be conflated.
func TestRenderHomePage_DegradesInsteadOfFailing(t *testing.T) {
	ctx := renderHomeCtx()
	workspace := &domain.Workspace{ID: "workspace123", Name: "WS", Settings: domain.WorkspaceSettings{Timezone: "UTC"}}
	theme := &domain.BlogTheme{Version: 1, Files: domain.BlogThemeFiles{
		HomeLiquid:   "lists={{ public_lists | size }} posts={{ posts | size }}",
		HeaderLiquid: "", FooterLiquid: "", SharedLiquid: "",
	}}

	t.Run("posts failure renders an empty page", func(t *testing.T) {
		s, catRepo, postRepo, th, wr, listRepo, _, _ := setupBlogServiceTest(t)
		wr.EXPECT().GetByID(ctx, "workspace123").Return(workspace, nil)
		th.EXPECT().GetPublishedTheme(ctx).Return(theme, nil)
		listRepo.EXPECT().GetLists(ctx, "workspace123").Return(nil, errors.New("lists down"))
		postRepo.EXPECT().ListPosts(ctx, gomock.Any()).Return(nil, errors.New("posts down"))
		catRepo.EXPECT().ListCategories(ctx).Return(nil, errors.New("cats down"))
		html, err := s.RenderHomePage(ctx, "workspace123", 1, nil)
		require.NoError(t, err)
		assert.Contains(t, html, "lists=0", "failed lists must surface as an empty collection, not a render error")
		assert.Contains(t, html, "posts=0")
	})

	t.Run("page beyond the last is a 404 render error", func(t *testing.T) {
		s, _, postRepo, th, wr, listRepo, _, _ := setupBlogServiceTest(t)
		wr.EXPECT().GetByID(ctx, "workspace123").Return(workspace, nil)
		th.EXPECT().GetPublishedTheme(ctx).Return(theme, nil)
		listRepo.EXPECT().GetLists(ctx, "workspace123").Return([]*domain.List{}, nil)
		postRepo.EXPECT().ListPosts(ctx, gomock.Any()).
			Return(&domain.BlogPostListResponse{Posts: []*domain.BlogPost{}, TotalCount: 0, TotalPages: 1}, nil)
		// The 404 is returned before navigation categories are loaded.
		_, err := s.RenderHomePage(ctx, "workspace123", 3, nil)
		var re *domain.BlogRenderError
		require.ErrorAs(t, err, &re)
		assert.Equal(t, domain.ErrCodePostNotFound, re.Code)
		assert.Equal(t, "Page 3 does not exist (total pages: 1)", re.Message)
	})
}

// A missing post on the post page is POST_NOT_FOUND regardless of the
// repository error (repository errors must not leak HTTP-style statuses).
func TestRenderPostPage_PostLookupTaxonomy(t *testing.T) {
	ctx := renderHomeCtx()
	workspace := &domain.Workspace{ID: "workspace123", Name: "WS", Settings: domain.WorkspaceSettings{Timezone: "UTC"}}
	theme := &domain.BlogTheme{Version: 1, Files: domain.BlogThemeFiles{HomeLiquid: "x"}}

	s, _, postRepo, th, wr, _, _, _ := setupBlogServiceTest(t)
	wr.EXPECT().GetByID(ctx, "workspace123").Return(workspace, nil)
	th.EXPECT().GetPublishedTheme(ctx).Return(theme, nil)
	postRepo.EXPECT().GetPostByCategoryAndSlug(ctx, "tech", "missing").
		Return(nil, errors.New("sql: no rows in result set"))
	_, err := s.RenderPostPage(ctx, "workspace123", "tech", "missing", nil)
	var re *domain.BlogRenderError
	require.ErrorAs(t, err, &re)
	assert.Equal(t, domain.ErrCodePostNotFound, re.Code)
}
