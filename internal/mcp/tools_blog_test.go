package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Notifuse/notifuse/internal/domain"
)

const (
	testPostID     = "7d0c0f2e-3c1a-4b8e-9f00-1a2b3c4d5e6f"
	testCategoryID = "1f2e3d4c-5b6a-4978-8a9b-0c1d2e3f4a5b"
)

func blogReadPermission() domain.UserPermissions {
	return domain.UserPermissions{domain.PermissionResourceBlog: {Read: true}}
}

func blogAndTemplatesRead() domain.UserPermissions {
	return domain.UserPermissions{domain.PermissionResourceBlog: {Read: true}, domain.PermissionResourceTemplates: {Read: true}}
}

var blogEpoch = time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)

// expectBlogWorkspace checks that a blog service call carries the workspace in
// its context, where the blog service reads it.
func expectBlogWorkspace(t *testing.T, ctx context.Context) {
	t.Helper()
	assert.Equal(t, testWorkspaceID, ctx.Value(domain.WorkspaceIDKey))
}

func scheduledPost() *domain.BlogPost {
	publishAt := time.Now().Add(240 * time.Hour).UTC().Truncate(time.Second)
	return &domain.BlogPost{
		ID: testPostID, CategoryID: testCategoryID, Slug: "spring-release", PublishedAt: &publishAt,
		Settings: domain.BlogPostSettings{
			Title: "What's new this spring", Excerpt: "Three changes to the editor",
			Template:           domain.BlogPostTemplateReference{TemplateID: "tpl-body", TemplateVersion: 0},
			FeaturedImageURL:   "https://images.example.com/spring.jpg",
			Authors:            []domain.BlogAuthor{{Name: "Jane Doe", AvatarURL: "https://images.example.com/jane.jpg"}},
			ReadingTimeMinutes: 4,
			SEO:                &domain.SEOSettings{MetaTitle: "Spring", Keywords: []string{"editor"}},
		},
		CreatedAt: blogEpoch,
		UpdatedAt: blogEpoch.Add(time.Hour),
	}
}

func bodyTemplate() *domain.Template {
	return &domain.Template{
		ID: "tpl-body", Name: "Spring body", Version: 5, Channel: "web", Category: "blog",
		Web: &domain.WebTemplate{
			Content:   domain.MapOfAny{"type": "doc", "content": []any{map[string]any{"type": "paragraph", "content": []any{map[string]any{"type": "text", "text": "Hello"}}}}},
			HTML:      "<p>rendered-html</p>",
			PlainText: "plain-text-index",
		},
		TestData: domain.MapOfAny{"unused": "test-data"},
	}
}

func TestBlogTools_Definitions(t *testing.T) {
	blog := []Pair{{Resource: "blog", Verb: "read"}}
	assertReadTool(t, catalogueTool(t, "list_blog_posts"), "list_blog_posts", blog, []string{"workspace_id"})
	assertReadTool(t, catalogueTool(t, "get_blog_post"), "get_blog_post", blog, []string{"workspace_id"})
	assertReadTool(t, catalogueTool(t, "list_blog_categories"), "list_blog_categories", blog, []string{"workspace_id"})
}

func TestListBlogPosts_Projection(t *testing.T) {
	f := newFixture(t)
	f.reachable(blogReadPermission())
	published := blogEpoch
	draft := &domain.BlogPost{ID: "d", CategoryID: testCategoryID, Slug: "draft", Settings: domain.BlogPostSettings{Title: "Draft"}, CreatedAt: blogEpoch, UpdatedAt: blogEpoch}
	live := &domain.BlogPost{ID: "l", CategoryID: testCategoryID, Slug: "live", Settings: domain.BlogPostSettings{Title: "Live"}, PublishedAt: &published, CreatedAt: blogEpoch, UpdatedAt: blogEpoch}
	scheduled := scheduledPost()
	f.blog.EXPECT().ListPosts(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, req *domain.ListBlogPostsRequest) (*domain.BlogPostListResponse, error) {
			expectBlogWorkspace(t, ctx)
			assert.Equal(t, &domain.ListBlogPostsRequest{Page: 1, Limit: 20}, req)
			return &domain.BlogPostListResponse{Posts: []*domain.BlogPost{scheduled, live, draft}, TotalCount: 3, CurrentPage: 1, TotalPages: 1}, nil
		})

	result := f.callTool("list_blog_posts", map[string]any{"workspace_id": testWorkspaceID})
	require.False(t, result.IsError, string(result.StructuredContent))
	var out struct {
		Workspace WorkspaceRef                 `json:"workspace"`
		Posts     []map[string]json.RawMessage `json:"posts"`
		Total     int                          `json:"total"`
	}
	require.NoError(t, json.Unmarshal(result.StructuredContent, &out))
	assert.Equal(t, WorkspaceRef{ID: "ws1", Name: "Acme"}, out.Workspace)
	assert.Equal(t, 3, out.Total)
	require.Len(t, out.Posts, 3)
	publishedAt, err := json.Marshal(scheduled.PublishedAt)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"id":"`+testPostID+`","category_id":"`+testCategoryID+`","slug":"spring-release",
		"title":"What's new this spring","excerpt":"Three changes to the editor","status":"scheduled",
		"published_at":`+string(publishedAt)+`,"created_at":"2026-11-20T10:00:00Z","updated_at":"2026-11-20T11:00:00Z"
	}`, mustJSON(t, out.Posts[0]))
	assert.Equal(t, `"published"`, string(out.Posts[1]["status"]))
	assert.Equal(t, `"draft"`, string(out.Posts[2]["status"]))
	assert.Equal(t, `null`, string(out.Posts[2]["published_at"]))
	for _, dropped := range []string{"tpl-body", "authors", "seo", "featured_image_url", "current_page", "total_pages"} {
		assert.NotContains(t, result.Content[0].Text, dropped)
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)
	return string(data)
}

// blogPosts.list pages by page number: the cursor carries the next page and
// the size of the first, and is bound to the filters.
func TestListBlogPosts_Cursor(t *testing.T) {
	f := newFixture(t)
	f.reachable(blogReadPermission())
	var got []*domain.ListBlogPostsRequest
	f.blog.EXPECT().ListPosts(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req *domain.ListBlogPostsRequest) (*domain.BlogPostListResponse, error) {
			copied := *req
			got = append(got, &copied)
			return &domain.BlogPostListResponse{Posts: []*domain.BlogPost{scheduledPost()}, TotalCount: 5, HasNextPage: len(got) == 1}, nil
		}).Times(2)

	args := map[string]any{"workspace_id": testWorkspaceID, "category_id": "{" + "1F2E3D4C-5B6A-4978-8A9B-0C1D2E3F4A5B" + "}", "status": "scheduled", "search": "  spring ", "limit": 2}
	var first struct {
		Total      int    `json:"total"`
		NextCursor string `json:"next_cursor"`
	}
	f.callTool("list_blog_posts", args).decode(t, &first)
	assert.Equal(t, 5, first.Total)
	require.NotEmpty(t, first.NextCursor)

	// The same filters spelled another way, and another limit, which the
	// cursor's page size wins over.
	var second map[string]json.RawMessage
	f.callTool("list_blog_posts", map[string]any{
		"workspace_id": testWorkspaceID, "category_id": testCategoryID, "status": "scheduled", "search": "spring", "limit": 7, "cursor": first.NextCursor,
	}).decode(t, &second)
	assert.NotContains(t, second, "next_cursor", "absent on the last page")

	require.Len(t, got, 2)
	assert.Equal(t, &domain.ListBlogPostsRequest{CategoryID: testCategoryID, Status: "scheduled", Search: "spring", Page: 1, Limit: 2}, got[0])
	assert.Equal(t, &domain.ListBlogPostsRequest{CategoryID: testCategoryID, Status: "scheduled", Search: "spring", Page: 2, Limit: 2}, got[1])

	toolErr := f.callTool("list_blog_posts", map[string]any{"workspace_id": testWorkspaceID, "status": "draft", "cursor": first.NextCursor}).errorOf(t)
	assert.Equal(t, CodeValidation, toolErr.Code, "a cursor is bound to its filters")
}

// status all is the default, so a cursor issued without it still serves a
// page asked with it.
func TestListBlogPosts_StatusAllIsTheDefault(t *testing.T) {
	f := newFixture(t)
	f.reachable(blogReadPermission())
	f.blog.EXPECT().ListPosts(gomock.Any(), gomock.Any()).Return(&domain.BlogPostListResponse{HasNextPage: true}, nil)
	f.blog.EXPECT().ListPosts(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req *domain.ListBlogPostsRequest) (*domain.BlogPostListResponse, error) {
			assert.Equal(t, 2, req.Page)
			assert.Equal(t, domain.BlogPostStatusAll, req.Status)
			return &domain.BlogPostListResponse{}, nil
		})

	var first struct {
		NextCursor string `json:"next_cursor"`
	}
	f.callTool("list_blog_posts", map[string]any{"workspace_id": testWorkspaceID}).decode(t, &first)
	require.NotEmpty(t, first.NextCursor)
	result := f.callTool("list_blog_posts", map[string]any{"workspace_id": testWorkspaceID, "status": "all", "cursor": first.NextCursor})
	require.False(t, result.IsError, string(result.StructuredContent))
}

func TestListBlogPosts_RefusesBadArgumentsBeforeTheService(t *testing.T) {
	f := newFixture(t)
	f.reachable(blogReadPermission())
	pageZero, err := EncodeCursor("list_blog_posts", testWorkspaceID, map[string]any{}, blogPostsPage{Page: 0, Limit: 20})
	require.NoError(t, err)
	hugePage, err := EncodeCursor("list_blog_posts", testWorkspaceID, map[string]any{}, blogPostsPage{Page: 2, Limit: 500})
	require.NoError(t, err)
	for name, args := range map[string]map[string]any{
		"category not a UUID": {"category_id": "product"},
		"unknown status":      {"status": "archived"},
		"long search":         {"search": strings.Repeat("a", 201)},
		"NUL in search":       {"search": "a\x00b"},
		"limit above 50":      {"limit": 51},
		"garbage cursor":      {"cursor": "%%%"},
		"page 0 cursor":       {"cursor": pageZero},
		"oversized cursor":    {"cursor": hugePage},
	} {
		t.Run(name, func(t *testing.T) {
			args["workspace_id"] = testWorkspaceID
			assert.Equal(t, CodeValidation, f.callTool("list_blog_posts", args).errorOf(t).Code)
		})
	}
}

func TestListBlogPosts_ErrorsAreMapped(t *testing.T) {
	f := newFixture(t)
	f.reachable(blogReadPermission())
	f.blog.EXPECT().ListPosts(gomock.Any(), gomock.Any()).Return(nil, errors.New("pq: relation blog_posts on db-host-7"))
	assert.Equal(t, ToolError{Code: CodeInternal, Message: "internal error"}, f.callTool("list_blog_posts", map[string]any{"workspace_id": testWorkspaceID}).errorOf(t))

	g := newFixture(t)
	g.reachable(domain.UserPermissions{domain.PermissionResourceTemplates: {Read: true}})
	assert.Equal(t, ToolError{Code: CodePermissionDenied, Message: "this API key lacks the blog read permission", Resource: "blog", Permission: "read"},
		g.callTool("list_blog_posts", map[string]any{"workspace_id": testWorkspaceID}).errorOf(t))
}

func TestGetBlogPost_WithBody(t *testing.T) {
	f := newFixture(t)
	f.reachable(blogAndTemplatesRead())
	post := scheduledPost()
	f.blog.EXPECT().GetPost(gomock.Any(), testPostID).
		DoAndReturn(func(ctx context.Context, _ string) (*domain.BlogPost, error) {
			expectBlogWorkspace(t, ctx)
			return post, nil
		})
	// Version 0: the post follows the latest version of its body.
	f.templates.EXPECT().GetTemplateByID(gomock.Any(), testWorkspaceID, "tpl-body", int64(0)).Return(bodyTemplate(), nil)

	// An ID in capitals is read as the same post.
	result := f.callTool("get_blog_post", map[string]any{"workspace_id": testWorkspaceID, "id": "7D0C0F2E-3C1A-4B8E-9F00-1A2B3C4D5E6F"})
	require.False(t, result.IsError, string(result.StructuredContent))
	publishedAt := mustJSON(t, post.PublishedAt)
	assert.JSONEq(t, `{
		"workspace":{"id":"ws1","name":"Acme"},
		"post":{
			"id":"`+testPostID+`","category_id":"`+testCategoryID+`","slug":"spring-release",
			"title":"What's new this spring","excerpt":"Three changes to the editor",
			"featured_image_url":"https://images.example.com/spring.jpg",
			"authors":[{"name":"Jane Doe","avatar_url":"https://images.example.com/jane.jpg"}],
			"reading_time_minutes":4,
			"seo":{"meta_title":"Spring","keywords":["editor"]},
			"status":"scheduled","published_at":`+publishedAt+`,
			"created_at":"2026-11-20T10:00:00Z","updated_at":"2026-11-20T11:00:00Z",
			"body":{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Hello"}]}]},
			"body_version":5,
			"follows_latest_body":true
		}
	}`, string(result.StructuredContent))
	for _, dropped := range []string{"rendered-html", "plain-text-index", "test-data", "tpl-body", "Spring body", "deleted_at"} {
		assert.NotContains(t, result.Content[0].Text, dropped)
	}
}

func TestGetBlogPost_BySlugWithAPinnedBody(t *testing.T) {
	f := newFixture(t)
	f.reachable(blogAndTemplatesRead())
	post := scheduledPost()
	post.Settings.Template.TemplateVersion = 3
	f.blog.EXPECT().GetPostBySlug(gomock.Any(), "spring-release").
		DoAndReturn(func(ctx context.Context, _ string) (*domain.BlogPost, error) {
			expectBlogWorkspace(t, ctx)
			return post, nil
		})
	pinned := bodyTemplate()
	pinned.Version = 3
	f.templates.EXPECT().GetTemplateByID(gomock.Any(), testWorkspaceID, "tpl-body", int64(3)).Return(pinned, nil)

	var out struct {
		Post struct {
			BodyVersion       int64 `json:"body_version"`
			FollowsLatestBody bool  `json:"follows_latest_body"`
		} `json:"post"`
	}
	f.callTool("get_blog_post", map[string]any{"workspace_id": testWorkspaceID, "slug": "spring-release"}).decode(t, &out)
	assert.Equal(t, int64(3), out.Post.BodyVersion)
	assert.False(t, out.Post.FollowsLatestBody)
}

// Without templates:read the post is still answered, its body null.
func TestGetBlogPost_WithoutTemplatesRead(t *testing.T) {
	f := newFixture(t)
	f.reachable(blogReadPermission())
	f.blog.EXPECT().GetPost(gomock.Any(), testPostID).Return(scheduledPost(), nil)

	var out struct {
		Post map[string]json.RawMessage `json:"post"`
	}
	f.callTool("get_blog_post", map[string]any{"workspace_id": testWorkspaceID, "id": testPostID}).decode(t, &out)
	assert.Equal(t, `null`, string(out.Post["body"]))
	assert.Equal(t, `null`, string(out.Post["body_version"]))
	assert.Equal(t, `"permission_denied"`, string(out.Post["body_unavailable"]))
	assert.Equal(t, `"What's new this spring"`, string(out.Post["title"]))
}

func TestGetBlogPost_BodyNotFound(t *testing.T) {
	deleted := bodyTemplate()
	deleted.DeletedAt = &blogEpoch
	emailTemplate := bodyTemplate()
	emailTemplate.Channel, emailTemplate.Web = "email", nil
	for name, answer := range map[string]struct {
		template *domain.Template
		err      error
	}{
		"missing":                {err: &domain.ErrTemplateNotFound{Message: "template not found"}},
		"deleted":                {template: deleted},
		"not a web one":          {template: emailTemplate},
		"refused by its service": {err: domain.NewPermissionError(domain.PermissionResourceTemplates, domain.PermissionTypeRead, "Insufficient permissions")},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			f.reachable(blogAndTemplatesRead())
			f.blog.EXPECT().GetPost(gomock.Any(), testPostID).Return(scheduledPost(), nil)
			f.templates.EXPECT().GetTemplateByID(gomock.Any(), testWorkspaceID, "tpl-body", int64(0)).Return(answer.template, answer.err)

			var out struct {
				Post map[string]json.RawMessage `json:"post"`
			}
			f.callTool("get_blog_post", map[string]any{"workspace_id": testWorkspaceID, "id": testPostID}).decode(t, &out)
			assert.Equal(t, `null`, string(out.Post["body"]))
			assert.NotEmpty(t, string(out.Post["body_unavailable"]))
		})
	}

	f := newFixture(t)
	f.reachable(blogAndTemplatesRead())
	f.blog.EXPECT().GetPost(gomock.Any(), testPostID).Return(scheduledPost(), nil)
	f.templates.EXPECT().GetTemplateByID(gomock.Any(), testWorkspaceID, "tpl-body", int64(0)).Return(nil, errors.New("pq: connection reset by db-host-7"))
	assert.Equal(t, ToolError{Code: CodeInternal, Message: "internal error"},
		f.callTool("get_blog_post", map[string]any{"workspace_id": testWorkspaceID, "id": testPostID}).errorOf(t), "a failure is not an absent body")
}

func TestGetBlogPost_NotFound(t *testing.T) {
	f := newFixture(t)
	f.reachable(blogAndTemplatesRead())
	f.blog.EXPECT().GetPostBySlug(gomock.Any(), "nope").Return(nil, &domain.ErrNotFound{Entity: "blog post", ID: "nope"})
	assert.Equal(t, CodeNotFound, f.callTool("get_blog_post", map[string]any{"workspace_id": testWorkspaceID, "slug": "nope"}).errorOf(t).Code)

	deleted := scheduledPost()
	deleted.DeletedAt = &blogEpoch
	f.blog.EXPECT().GetPost(gomock.Any(), testPostID).Return(deleted, nil)
	assert.Equal(t, CodeNotFound, f.callTool("get_blog_post", map[string]any{"workspace_id": testWorkspaceID, "id": testPostID}).errorOf(t).Code)
}

func TestGetBlogPost_IDOrSlugNotBoth(t *testing.T) {
	f := newFixture(t)
	f.reachable(blogAndTemplatesRead())
	for name, args := range map[string]map[string]any{
		"neither":      {},
		"both":         {"id": testPostID, "slug": "spring-release"},
		"empty values": {"id": "", "slug": ""},
		"ID not UUID":  {"id": "spring-release"},
	} {
		t.Run(name, func(t *testing.T) {
			args["workspace_id"] = testWorkspaceID
			assert.Equal(t, CodeValidation, f.callTool("get_blog_post", args).errorOf(t).Code)
		})
	}
}

func TestListBlogCategories(t *testing.T) {
	f := newFixture(t)
	f.reachable(blogReadPermission())
	categories := []*domain.BlogCategory{
		{ID: "c", Slug: "c", Settings: domain.BlogCategorySettings{Name: "C"}, CreatedAt: blogEpoch.Add(time.Hour)},
		{ID: "b", Slug: "b", Settings: domain.BlogCategorySettings{Name: "B", Description: "Release notes", SEO: &domain.SEOSettings{MetaTitle: "seo-default"}}, CreatedAt: blogEpoch.Add(time.Hour)},
		{ID: "a", Slug: "a", Settings: domain.BlogCategorySettings{Name: "A"}, CreatedAt: blogEpoch},
		{ID: "gone", Slug: "gone", CreatedAt: blogEpoch.Add(2 * time.Hour), DeletedAt: &blogEpoch},
		nil,
	}
	f.blog.EXPECT().ListCategories(gomock.Any()).
		DoAndReturn(func(ctx context.Context) (*domain.BlogCategoryListResponse, error) {
			expectBlogWorkspace(t, ctx)
			return &domain.BlogCategoryListResponse{Categories: categories, TotalCount: len(categories)}, nil
		}).Times(2)

	result := f.callTool("list_blog_categories", map[string]any{"workspace_id": testWorkspaceID, "limit": 2})
	require.False(t, result.IsError, string(result.StructuredContent))
	var first struct {
		NextCursor string `json:"next_cursor"`
	}
	require.NoError(t, json.Unmarshal(result.StructuredContent, &first))
	require.NotEmpty(t, first.NextCursor)
	assert.JSONEq(t, `{
		"workspace":{"id":"ws1","name":"Acme"},
		"categories":[
			{"id":"b","slug":"b","name":"B","description":"Release notes"},
			{"id":"c","slug":"c","name":"C","description":""}
		],
		"total":3,
		"next_cursor":"`+first.NextCursor+`"
	}`, string(result.StructuredContent))
	assert.NotContains(t, result.Content[0].Text, "seo-default")

	var second struct {
		Categories []struct {
			ID string `json:"id"`
		} `json:"categories"`
		NextCursor string `json:"next_cursor"`
	}
	f.callTool("list_blog_categories", map[string]any{"workspace_id": testWorkspaceID, "limit": 2, "cursor": first.NextCursor}).decode(t, &second)
	require.Len(t, second.Categories, 1)
	assert.Equal(t, "a", second.Categories[0].ID)
	assert.Empty(t, second.NextCursor)

	otherTool, err := EncodeCursor("list_automations", testWorkspaceID, map[string]any{}, 2)
	require.NoError(t, err)
	assert.Equal(t, CodeValidation, f.callTool("list_blog_categories", map[string]any{"workspace_id": testWorkspaceID, "cursor": otherTool}).errorOf(t).Code)
}

func TestListBlogCategories_ErrorsAreMapped(t *testing.T) {
	f := newFixture(t)
	f.reachable(blogReadPermission())
	f.blog.EXPECT().ListCategories(gomock.Any()).
		Return(nil, domain.NewPermissionError(domain.PermissionResourceBlog, domain.PermissionTypeRead, "Insufficient permissions: read access to blog required"))
	toolErr := f.callTool("list_blog_categories", map[string]any{"workspace_id": testWorkspaceID}).errorOf(t)
	assert.Equal(t, CodePermissionDenied, toolErr.Code)
	assert.Equal(t, "blog", toolErr.Resource)

	g := newFixture(t)
	g.switchedOff()
	assert.Equal(t, ToolError{Code: CodeNotFound, Message: "workspace not found"},
		g.callTool("list_blog_categories", map[string]any{"workspace_id": testWorkspaceID}).errorOf(t))
}

// A post's title, excerpt, image, authors and SEO fields and a category's name
// and description are free text, capped like the rest. Only the body stays
// whole.
func TestBlogTools_CapTheFreeText(t *testing.T) {
	long, capped := strings.Repeat("n", FreeTextLimit+1), strings.Repeat("n", FreeTextLimit)
	longPost := func() *domain.BlogPost {
		post := scheduledPost()
		post.Settings.Title, post.Settings.Excerpt, post.Settings.FeaturedImageURL = long, long, long
		post.Settings.Authors = []domain.BlogAuthor{{Name: long, AvatarURL: long}}
		post.Settings.SEO = &domain.SEOSettings{
			MetaTitle: long, MetaDescription: long, OGTitle: long, OGDescription: long,
			OGImage: long, CanonicalURL: long, MetaRobots: long, Keywords: []string{"short", long},
		}
		return post
	}

	t.Run("list_blog_posts", func(t *testing.T) {
		f := newFixture(t)
		f.reachable(blogReadPermission())
		f.blog.EXPECT().ListPosts(gomock.Any(), gomock.Any()).
			Return(&domain.BlogPostListResponse{Posts: []*domain.BlogPost{longPost()}, TotalCount: 1, CurrentPage: 1, TotalPages: 1}, nil)
		var out struct {
			Posts []struct {
				Title           string   `json:"title"`
				Excerpt         string   `json:"excerpt"`
				TruncatedFields []string `json:"truncated_fields"`
			} `json:"posts"`
		}
		f.callTool("list_blog_posts", map[string]any{"workspace_id": testWorkspaceID}).decode(t, &out)
		require.Len(t, out.Posts, 1)
		assert.Equal(t, capped, out.Posts[0].Title)
		assert.Equal(t, capped, out.Posts[0].Excerpt)
		assert.Equal(t, []string{"title", "excerpt"}, out.Posts[0].TruncatedFields)
	})

	t.Run("get_blog_post", func(t *testing.T) {
		f := newFixture(t)
		f.reachable(blogReadPermission())
		f.blog.EXPECT().GetPost(gomock.Any(), testPostID).Return(longPost(), nil)
		var out struct {
			Post struct {
				Title            string              `json:"title"`
				Excerpt          string              `json:"excerpt"`
				FeaturedImageURL string              `json:"featured_image_url"`
				Authors          []domain.BlogAuthor `json:"authors"`
				SEO              domain.SEOSettings  `json:"seo"`
				TruncatedFields  []string            `json:"truncated_fields"`
			} `json:"post"`
		}
		f.callTool("get_blog_post", map[string]any{"workspace_id": testWorkspaceID, "id": testPostID}).decode(t, &out)
		post := out.Post
		assert.Equal(t, capped, post.Title)
		assert.Equal(t, capped, post.Excerpt)
		assert.Equal(t, capped, post.FeaturedImageURL)
		assert.Equal(t, []domain.BlogAuthor{{Name: capped, AvatarURL: capped}}, post.Authors)
		assert.Equal(t, domain.SEOSettings{
			MetaTitle: capped, MetaDescription: capped, OGTitle: capped, OGDescription: capped,
			OGImage: capped, CanonicalURL: capped, MetaRobots: capped, Keywords: []string{"short", capped},
		}, post.SEO)
		assert.Equal(t, []string{
			"title", "excerpt", "featured_image_url", "authors[0].name", "authors[0].avatar_url",
			"seo.meta_title", "seo.meta_description", "seo.og_title", "seo.og_description", "seo.og_image",
			"seo.canonical_url", "seo.meta_robots", "seo.keywords[1]",
		}, post.TruncatedFields)
	})

	t.Run("list_blog_categories", func(t *testing.T) {
		f := newFixture(t)
		f.reachable(blogReadPermission())
		f.blog.EXPECT().ListCategories(gomock.Any()).Return(&domain.BlogCategoryListResponse{Categories: []*domain.BlogCategory{
			{ID: "a", Slug: "a", Settings: domain.BlogCategorySettings{Name: long, Description: long}, CreatedAt: blogEpoch},
		}, TotalCount: 1}, nil)
		var out struct {
			Categories []struct {
				Name            string   `json:"name"`
				Description     string   `json:"description"`
				TruncatedFields []string `json:"truncated_fields"`
			} `json:"categories"`
		}
		f.callTool("list_blog_categories", map[string]any{"workspace_id": testWorkspaceID}).decode(t, &out)
		require.Len(t, out.Categories, 1)
		assert.Equal(t, capped, out.Categories[0].Name)
		assert.Equal(t, capped, out.Categories[0].Description)
		assert.Equal(t, []string{"name", "description"}, out.Categories[0].TruncatedFields)
	})
}
