package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
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
		},
		"blog_enabled":false
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

// not_found names the value the caller gave, slug or ID.
func TestGetBlogPost_NotFound(t *testing.T) {
	f := newFixture(t)
	f.reachable(blogAndTemplatesRead())
	f.blog.EXPECT().GetPostBySlug(gomock.Any(), "nope").Return(nil, &domain.ErrNotFound{Entity: "blog_post", ID: "nope"}).Times(2)
	assert.Equal(t, ToolError{Code: CodeNotFound, Message: "blog post not found: nope"},
		f.callTool("get_blog_post", map[string]any{"workspace_id": testWorkspaceID, "slug": "nope"}).errorOf(t))
	assert.Equal(t, ToolError{Code: CodeNotFound, Message: "blog post not found: nope"},
		f.callTool("get_blog_post", map[string]any{"workspace_id": testWorkspaceID, "id": "nope"}).errorOf(t), "a slug given as id")

	deleted := scheduledPost()
	deleted.DeletedAt = &blogEpoch
	f.blog.EXPECT().GetPost(gomock.Any(), testPostID).Return(deleted, nil)
	f.blog.EXPECT().GetPostBySlug(gomock.Any(), testPostID).Return(nil, &domain.ErrNotFound{Entity: "blog_post", ID: testPostID})
	assert.Equal(t, ToolError{Code: CodeNotFound, Message: "blog post not found: " + testPostID},
		f.callTool("get_blog_post", map[string]any{"workspace_id": testWorkspaceID, "id": testPostID}).errorOf(t))
}

// id takes a slug too, as the blog write tools do.
func TestGetBlogPost_IDTakesASlug(t *testing.T) {
	f := newFixture(t)
	f.reachable(blogReadPermission())
	f.blog.EXPECT().GetPostBySlug(gomock.Any(), "spring-release").Return(scheduledPost(), nil)

	var out struct {
		Post struct {
			ID string `json:"id"`
		} `json:"post"`
	}
	f.callTool("get_blog_post", map[string]any{"workspace_id": testWorkspaceID, "id": "spring-release"}).decode(t, &out)
	assert.Equal(t, testPostID, out.Post.ID)
}

// blog_enabled is there, false, only when the workspace's blog is turned off.
func TestBlogTools_ReportATurnedOffBlog(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		f := newFixture(t)
		workspace := testWorkspace()
		workspace.Settings.BlogEnabled = enabled
		f.reachableIn(blogReadPermission(), workspace)
		f.blog.EXPECT().ListCategories(gomock.Any()).Return(&domain.BlogCategoryListResponse{}, nil)
		f.blog.EXPECT().ListPosts(gomock.Any(), gomock.Any()).Return(&domain.BlogPostListResponse{}, nil)
		f.blog.EXPECT().GetPost(gomock.Any(), testPostID).Return(scheduledPost(), nil)

		for tool, args := range map[string]map[string]any{
			"list_blog_categories": {"workspace_id": testWorkspaceID},
			"list_blog_posts":      {"workspace_id": testWorkspaceID},
			"get_blog_post":        {"workspace_id": testWorkspaceID, "id": testPostID},
		} {
			var out map[string]json.RawMessage
			f.callTool(tool, args).decode(t, &out)
			if enabled {
				assert.NotContains(t, out, "blog_enabled", tool)
			} else {
				assert.Equal(t, "false", string(out["blog_enabled"]), tool)
			}
		}
	}
	for _, tool := range newBareServer(Deps{}).catalogue() {
		if strings.Contains(tool.Name, "blog") {
			assert.Contains(t, tool.Description, "blog_enabled set to false: the blog then serves no page", tool.Name)
		}
	}
}

func TestGetBlogPost_IDOrSlugNotBoth(t *testing.T) {
	f := newFixture(t)
	f.reachable(blogAndTemplatesRead())
	for name, args := range map[string]map[string]any{
		"neither":      {},
		"both":         {"id": testPostID, "slug": "spring-release"},
		"empty values": {"id": "", "slug": ""},
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
		"next_cursor":"`+first.NextCursor+`",
		"blog_enabled":false
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

func blogWritePermissions(templates domain.ResourcePermissions) domain.UserPermissions {
	return domain.UserPermissions{
		domain.PermissionResourceBlog:      {Read: true, Write: true},
		domain.PermissionResourceTemplates: templates,
	}
}

func blogWriteFixture(t *testing.T, permissions domain.UserPermissions) (*fixture, *auditLog) {
	f := newFixture(t)
	f.reachable(permissions)
	return f, f.captureAudit()
}

const helloBody = `{"type":"doc","content":[{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"Hello"}]},` +
	`{"type":"paragraph","content":[{"type":"text","text":"Read the "},{"type":"text","text":"docs","marks":[{"type":"link","attrs":{"href":"https://docs.example.com"}}]}]}]}`

func helloBodyMap(t *testing.T) map[string]any {
	return decodeDoc(t, helloBody)
}

// expectCategory answers the category check with a live category.
func (f *fixture) expectCategory(id string) {
	f.categories.EXPECT().GetCategoriesByIDs(gomock.Any(), []string{id}).
		DoAndReturn(func(ctx context.Context, _ []string) ([]*domain.BlogCategory, error) {
			expectBlogWorkspace(f.t, ctx)
			return []*domain.BlogCategory{{ID: id, Slug: "product"}}, nil
		})
}

func TestBlogWriteTools_Definitions(t *testing.T) {
	blogWrite := Pair{Resource: "blog", Verb: "write"}
	id := []string{"workspace_id", "id"}
	assertWriteTool(t, catalogueTool(t, "create_blog_post"), "create_blog_post", ClassWrite, writeHints{destructive: false, openWorld: false, idempotent: false},
		[]Pair{blogWrite, {Resource: "templates", Verb: "write"}}, "blogPosts.create", []string{"workspace_id", "category_id", "slug", "title", "body"})
	update := catalogueTool(t, "update_blog_post")
	assertWriteToolDefinition(t, update, "update_blog_post", ClassWrite, writeHints{destructive: false, openWorld: false, idempotent: true},
		[]Pair{blogWrite, {Resource: "blog", Verb: "read"}}, "blogPosts.update", id)
	assert.Equal(t, []Pair{{Resource: "templates", Verb: "write"}, {Resource: "templates", Verb: "read"}}, update.ArgumentPairs(map[string]any{"body": map[string]any{}}))
	assert.Empty(t, update.ArgumentPairs(map[string]any{"title": "x"}))
	// Publishing sends no email: a write that does not count in mcp_send.
	assertWriteToolDefinition(t, catalogueTool(t, "publish_blog_post"), "publish_blog_post", ClassWrite, writeHints{destructive: false, openWorld: false, idempotent: true},
		[]Pair{blogWrite}, "blogPosts.publish", id)
	assertWriteToolDefinition(t, catalogueTool(t, "unpublish_blog_post"), "unpublish_blog_post", ClassWrite, writeHints{destructive: false, openWorld: false, idempotent: true},
		[]Pair{blogWrite}, "blogPosts.unpublish", id)
	assertWriteToolDefinition(t, catalogueTool(t, "delete_blog_post"), "delete_blog_post", ClassDestructive, writeHints{destructive: true, openWorld: false, idempotent: true},
		[]Pair{blogWrite}, "blogPosts.delete", id)
	for _, name := range []string{"create_blog_post", "update_blog_post", "publish_blog_post", "unpublish_blog_post", "delete_blog_post"} {
		assert.False(t, catalogueTool(t, name).countsAsSend(), name)
	}
	body := catalogueTool(t, "create_blog_post").InputSchema["properties"].(map[string]any)["body"].(map[string]any)
	assert.Equal(t, "object", body["type"])
}

func TestCreateBlogPost_WritesTheBodyTemplateThenThePost(t *testing.T) {
	f, audit := blogWriteFixture(t, blogWritePermissions(domain.ResourcePermissions{Write: true}))
	f.expectCategory(testCategoryID)
	var templateID string
	f.templates.EXPECT().CreateTemplate(gomock.Any(), testWorkspaceID, gomock.Any()).DoAndReturn(func(_ context.Context, _ string, template *domain.Template) error {
		templateID = template.ID
		assert.Regexp(t, regexp.MustCompile(`^[0-9a-f]{32}$`), template.ID)
		assert.Equal(t, "Blog: Spring release", template.Name)
		assert.Equal(t, int64(1), template.Version)
		assert.Equal(t, domain.ChannelWeb, template.Channel)
		assert.Equal(t, "blog", template.Category)
		assert.Nil(t, template.Email)
		require.NotNil(t, template.Web)
		assert.Equal(t, domain.MapOfAny(helloBodyMap(t)), template.Web.Content)
		assert.Equal(t, `<h2>Hello</h2><p>Read the <a href="https://docs.example.com">docs</a></p>`, template.Web.HTML)
		assert.Equal(t, "Hello Read the  docs", template.Web.PlainText)
		return nil
	})
	f.blog.EXPECT().CreatePost(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, req *domain.CreateBlogPostRequest) (*domain.BlogPost, error) {
		expectBlogWorkspace(t, ctx)
		assert.Equal(t, &domain.CreateBlogPostRequest{
			CategoryID: testCategoryID, Slug: "spring-release", Title: "Spring release",
			TemplateID: templateID, TemplateVersion: 0,
			Excerpt:            "Three changes",
			Authors:            []domain.BlogAuthor{{Name: "Jane Doe", AvatarURL: "https://images.example.com/jane.jpg"}},
			ReadingTimeMinutes: 1,
			SEO:                &domain.SEOSettings{MetaTitle: "Spring", Keywords: []string{"editor"}},
		}, req)
		return &domain.BlogPost{
			ID: testPostID, CategoryID: req.CategoryID, Slug: req.Slug, CreatedAt: blogEpoch, UpdatedAt: blogEpoch,
			Settings: domain.BlogPostSettings{
				Title: req.Title, Excerpt: req.Excerpt, Authors: req.Authors, ReadingTimeMinutes: req.ReadingTimeMinutes, SEO: req.SEO,
				Template: domain.BlogPostTemplateReference{TemplateID: req.TemplateID, TemplateVersion: req.TemplateVersion},
			},
		}, nil
	})

	result := f.callTool("create_blog_post", map[string]any{
		"workspace_id": testWorkspaceID, "category_id": strings.ToUpper(testCategoryID), "slug": "spring-release", "title": "Spring release",
		"body": helloBodyMap(t), "excerpt": "Three changes",
		"authors": []map[string]any{{"name": "Jane Doe", "avatar_url": "https://images.example.com/jane.jpg"}, {"name": "  "}},
		"seo":     map[string]any{"meta_title": "Spring", "keywords": []string{"editor"}},
	})
	var out struct {
		Post map[string]any `json:"post"`
	}
	result.decode(t, &out)
	assert.Equal(t, testPostID, out.Post["id"])
	assert.Equal(t, "draft", out.Post["status"])
	assert.Equal(t, float64(1), out.Post["body_version"])
	assert.Equal(t, true, out.Post["follows_latest_body"])
	assert.Equal(t, helloBodyMap(t), out.Post["body"])
	assert.NotContains(t, out.Post, "body_unavailable")

	row := audit.last(t)
	assert.Equal(t, "blogPosts.create", row.Action)
	assert.Equal(t, domain.AuditTargetBlogPost, row.TargetType)
	assert.Equal(t, testPostID, row.TargetID)
	assert.Equal(t, "Spring release", row.TargetName)
	assert.Equal(t, templateID, row.Metadata["template_id"])
	assert.Equal(t, http.StatusOK, row.StatusCode)
}

// A replay meets its own slug: the post is refused with conflict, and the
// body template made for it is deleted again.
func TestCreateBlogPost_ATakenSlugAnswersConflictAndLeavesNoTemplate(t *testing.T) {
	f, audit := blogWriteFixture(t, blogWritePermissions(domain.ResourcePermissions{Write: true}))
	f.expectCategory(testCategoryID)
	var templateID string
	f.templates.EXPECT().CreateTemplate(gomock.Any(), testWorkspaceID, gomock.Any()).DoAndReturn(func(_ context.Context, _ string, template *domain.Template) error {
		templateID = template.ID
		return nil
	})
	f.blog.EXPECT().CreatePost(gomock.Any(), gomock.Any()).Return(nil, domain.NewAlreadyExistsError("blog_post", "spring-release", true))
	f.templates.EXPECT().DeleteTemplate(gomock.Any(), testWorkspaceID, gomock.Any()).DoAndReturn(func(_ context.Context, _ string, id string) error {
		assert.Equal(t, templateID, id)
		return nil
	})

	toolErr := f.callTool("create_blog_post", map[string]any{
		"workspace_id": testWorkspaceID, "category_id": testCategoryID, "slug": "spring-release", "title": "Spring", "body": helloBodyMap(t),
	}).errorOf(t)
	assert.Equal(t, ToolError{Code: CodeConflict, Message: "blog_post spring-release already exists and was deleted: its identifier cannot be reused"}, toolErr)
	assert.Equal(t, http.StatusConflict, audit.last(t).StatusCode)
}

func TestCreateBlogPost_RefusesBeforeAnyWrite(t *testing.T) {
	valid := func() map[string]any {
		return map[string]any{
			"workspace_id": testWorkspaceID, "category_id": testCategoryID, "slug": "spring", "title": "Spring", "body": decodeDoc(t, helloBody),
		}
	}
	with := func(key string, value any) map[string]any {
		args := valid()
		args[key] = value
		return args
	}
	for name, tc := range map[string]struct {
		args    map[string]any
		message string
	}{
		"a javascript link": {with("body", decodeDoc(t, paragraphWith(`{"type":"text","text":"x","marks":[{"type":"link","attrs":{"href":"javascript:alert(1)"}}]}`))),
			"body.content[0].content[0].marks[0].attrs.href: a link needs an http, https or mailto URL"},
		"a data image":                  {with("body", decodeDoc(t, `{"type":"doc","content":[{"type":"image","attrs":{"src":"data:image/png;base64,AAAA"}}]}`)), "body.content[0].attrs.src: an image needs an http or https URL"},
		"a slug with capitals":          {with("slug", "Spring Release"), "slug must contain only lowercase letters, numbers, and hyphens"},
		"a category that is not a UUID": {with("category_id", "product"), "category_id must be a UUID, from list_blog_categories"},
		"a javascript cover":            {with("featured_image_url", "javascript:alert(1)"), "featured_image_url must be an http or https URL"},
		"a javascript avatar":           {with("authors", []map[string]any{{"name": "Jane", "avatar_url": "javascript:x"}}), "authors[0].avatar_url must be an http or https URL"},
		"a reading time of zero":        {with("reading_time_minutes", 0), "reading_time_minutes must be from 1 to 600"},
		"arguments over 1 MiB":          {with("excerpt", strings.Repeat("x", 1<<20)), "arguments are limited to 1048576 bytes"},
	} {
		t.Run(name, func(t *testing.T) {
			f, audit := blogWriteFixture(t, blogWritePermissions(domain.ResourcePermissions{Write: true}))
			assert.Equal(t, ToolError{Code: CodeValidation, Message: tc.message}, f.callTool("create_blog_post", tc.args).errorOf(t))
			assert.Equal(t, http.StatusBadRequest, audit.last(t).StatusCode)
		})
	}

	t.Run("a category that does not exist", func(t *testing.T) {
		f, _ := blogWriteFixture(t, blogWritePermissions(domain.ResourcePermissions{Write: true}))
		deleted := time.Now()
		f.categories.EXPECT().GetCategoriesByIDs(gomock.Any(), []string{testCategoryID}).Return([]*domain.BlogCategory{{ID: testCategoryID, DeletedAt: &deleted}}, nil)
		assert.Equal(t, ToolError{Code: CodeNotFound, Message: "blog category not found: list_blog_categories gives the IDs"}, f.callTool("create_blog_post", valid()).errorOf(t))
	})
}

func TestCreateBlogPost_NeedsTemplatesWrite(t *testing.T) {
	f, audit := blogWriteFixture(t, blogWritePermissions(domain.ResourcePermissions{Read: true}))
	assert.NotContains(t, f.listToolNames(), "create_blog_post")
	toolErr := f.callTool("create_blog_post", map[string]any{
		"workspace_id": testWorkspaceID, "category_id": testCategoryID, "slug": "spring", "title": "Spring", "body": helloBodyMap(t),
	}).errorOf(t)
	assert.Equal(t, ToolError{Code: CodePermissionDenied, Message: "this API key lacks the templates write permission", Resource: "templates", Permission: "write"}, toolErr)
	assert.Equal(t, http.StatusForbidden, audit.last(t).StatusCode)
}

// storedUpdateRequest is the update request of scheduledPost with nothing
// changed.
func storedUpdateRequest() *domain.UpdateBlogPostRequest {
	post := scheduledPost()
	return &domain.UpdateBlogPostRequest{
		ID: post.ID, CategoryID: post.CategoryID, Slug: post.Slug, Title: post.Settings.Title,
		TemplateID: post.Settings.Template.TemplateID, TemplateVersion: post.Settings.Template.TemplateVersion,
		Excerpt: post.Settings.Excerpt, FeaturedImageURL: post.Settings.FeaturedImageURL, Authors: post.Settings.Authors,
		ReadingTimeMinutes: post.Settings.ReadingTimeMinutes, SEO: post.Settings.SEO,
	}
}

// updatedPost is what the service answers for a request.
func updatedPost(req *domain.UpdateBlogPostRequest) *domain.BlogPost {
	post := scheduledPost()
	post.CategoryID, post.Slug = req.CategoryID, req.Slug
	post.Settings = domain.BlogPostSettings{
		Title: req.Title, Excerpt: req.Excerpt, FeaturedImageURL: req.FeaturedImageURL, Authors: req.Authors,
		ReadingTimeMinutes: req.ReadingTimeMinutes, SEO: req.SEO,
		Template: domain.BlogPostTemplateReference{TemplateID: req.TemplateID, TemplateVersion: req.TemplateVersion},
	}
	return post
}

// blogPosts.update replaces every field: the fields not given come from the
// stored post.
func TestUpdateBlogPost_KeepsWhatItIsNotGiven(t *testing.T) {
	f, audit := blogWriteFixture(t, blogWritePermissions(domain.ResourcePermissions{}))
	f.blog.EXPECT().GetPost(gomock.Any(), testPostID).DoAndReturn(func(ctx context.Context, _ string) (*domain.BlogPost, error) {
		expectBlogWorkspace(t, ctx)
		return scheduledPost(), nil
	})
	want := storedUpdateRequest()
	want.Title = "What's new"
	want.Excerpt = ""
	f.blog.EXPECT().UpdatePost(gomock.Any(), want).DoAndReturn(func(_ context.Context, req *domain.UpdateBlogPostRequest) (*domain.BlogPost, error) {
		return updatedPost(req), nil
	})

	result := f.callTool("update_blog_post", map[string]any{"workspace_id": testWorkspaceID, "id": testPostID, "title": "What's new", "excerpt": ""})
	var out struct {
		Post map[string]any `json:"post"`
	}
	result.decode(t, &out)
	assert.Equal(t, "What's new", out.Post["title"])
	assert.Equal(t, "https://images.example.com/spring.jpg", out.Post["featured_image_url"])
	assert.Nil(t, out.Post["body"])
	assert.Equal(t, "permission_denied", out.Post["body_unavailable"], "a key without templates:read reads no body")
	row := audit.last(t)
	assert.Equal(t, "blogPosts.update", row.Action)
	assert.Equal(t, testPostID, row.TargetID)
	assert.Equal(t, "What's new", row.TargetName)
	assert.NotContains(t, row.Metadata, "template_id")
}

func TestUpdateBlogPost_ABodySavesANewVersionAsTheConsoleDoes(t *testing.T) {
	for name, tc := range map[string]struct {
		stored, sent int
	}{
		"a post that follows the latest version keeps following it": {stored: 0, sent: 0},
		"a pinned post moves to the version just saved":             {stored: 5, sent: 6},
	} {
		t.Run(name, func(t *testing.T) {
			f, audit := blogWriteFixture(t, blogWritePermissions(domain.ResourcePermissions{Read: true, Write: true}))
			post := scheduledPost()
			post.Settings.Template.TemplateVersion = tc.stored
			f.blog.EXPECT().GetPost(gomock.Any(), testPostID).Return(post, nil)
			f.templates.EXPECT().GetTemplateByID(gomock.Any(), testWorkspaceID, "tpl-body", int64(0)).Return(bodyTemplate(), nil)
			f.templates.EXPECT().UpdateTemplate(gomock.Any(), testWorkspaceID, gomock.Any()).DoAndReturn(func(_ context.Context, _ string, template *domain.Template) error {
				assert.Equal(t, "tpl-body", template.ID)
				assert.Equal(t, "Blog: What's new this spring", template.Name)
				assert.Equal(t, int64(5), template.Version, "base_version rides on Version")
				assert.Equal(t, domain.ChannelWeb, template.Channel)
				assert.Equal(t, "blog", template.Category)
				assert.Equal(t, domain.MapOfAny{"unused": "test-data"}, template.TestData, "the stored test data is kept")
				assert.Equal(t, domain.MapOfAny(helloBodyMap(t)), template.Web.Content)
				assert.Equal(t, `<h2>Hello</h2><p>Read the <a href="https://docs.example.com">docs</a></p>`, template.Web.HTML)
				template.Version = 6
				return nil
			})
			want := storedUpdateRequest()
			want.TemplateVersion = tc.sent
			f.blog.EXPECT().UpdatePost(gomock.Any(), want).DoAndReturn(func(_ context.Context, req *domain.UpdateBlogPostRequest) (*domain.BlogPost, error) {
				return updatedPost(req), nil
			})

			result := f.callTool("update_blog_post", map[string]any{"workspace_id": testWorkspaceID, "id": testPostID, "body": helloBodyMap(t), "base_version": 5})
			var out struct {
				Post map[string]any `json:"post"`
			}
			result.decode(t, &out)
			assert.Equal(t, float64(6), out.Post["body_version"])
			assert.Equal(t, tc.sent == 0, out.Post["follows_latest_body"])
			assert.Equal(t, helloBodyMap(t), out.Post["body"])
			row := audit.last(t)
			assert.Equal(t, "tpl-body", row.Metadata["template_id"])
			assert.Equal(t, int64(6), row.Metadata["body_version"])
		})
	}
}

// A body saved before a failed UpdatePost must not stay live: a post that
// follows the latest version gets its previous body saved back, and a pinned
// post still points at its old version, so nothing is saved back for it.
func TestUpdateBlogPost_AFailedPostUpdateLeavesTheServedBody(t *testing.T) {
	for name, tc := range map[string]struct {
		stored   int
		restores bool
	}{
		"a post that follows the latest version": {stored: 0, restores: true},
		"a pinned post":                          {stored: 5, restores: false},
	} {
		t.Run(name, func(t *testing.T) {
			f, audit := blogWriteFixture(t, blogWritePermissions(domain.ResourcePermissions{Read: true, Write: true}))
			post := scheduledPost()
			post.Settings.Template.TemplateVersion = tc.stored
			f.blog.EXPECT().GetPost(gomock.Any(), testPostID).Return(post, nil)
			f.templates.EXPECT().GetTemplateByID(gomock.Any(), testWorkspaceID, "tpl-body", int64(0)).Return(bodyTemplate(), nil)
			var saved []domain.Template
			f.templates.EXPECT().UpdateTemplate(gomock.Any(), testWorkspaceID, gomock.Any()).DoAndReturn(func(_ context.Context, _ string, template *domain.Template) error {
				saved = append(saved, *template)
				template.Version = int64(6 + len(saved) - 1)
				return nil
			}).AnyTimes()
			f.blog.EXPECT().UpdatePost(gomock.Any(), gomock.Any()).Return(nil, domain.NewAlreadyExistsError("blog_post", "spring-release", false))

			toolErr := f.callTool("update_blog_post", map[string]any{"workspace_id": testWorkspaceID, "id": testPostID, "body": helloBodyMap(t)}).errorOf(t)
			assert.Equal(t, CodeConflict, toolErr.Code)
			require.NotEmpty(t, saved)
			assert.Equal(t, domain.MapOfAny(helloBodyMap(t)), saved[0].Web.Content, "the new body is saved first")
			row := audit.last(t)
			if !tc.restores {
				assert.Len(t, saved, 1, "a pinned post never served the new version")
				assert.NotContains(t, row.Metadata, "body_restored_version")
				return
			}
			require.Len(t, saved, 2, "the previous body is saved back")
			previous := bodyTemplate()
			assert.Equal(t, previous.Web.Content, saved[1].Web.Content)
			assert.Equal(t, previous.Name, saved[1].Name)
			assert.Equal(t, int64(6), saved[1].Version, "the restore starts from the version just saved, so a later edit is not overwritten")
			assert.Equal(t, int64(7), row.Metadata["body_restored_version"])
		})
	}
}

func TestUpdateBlogPost_ABodyNeedsTheTemplatePairs(t *testing.T) {
	for name, tc := range map[string]struct {
		templates  domain.ResourcePermissions
		permission string
	}{
		"without templates:write": {domain.ResourcePermissions{Read: true}, "write"},
		"without templates:read":  {domain.ResourcePermissions{Write: true}, "read"},
	} {
		t.Run(name, func(t *testing.T) {
			f, audit := blogWriteFixture(t, blogWritePermissions(tc.templates))
			assert.Contains(t, f.listToolNames(), "update_blog_post", "the metadata stays editable")
			toolErr := f.callTool("update_blog_post", map[string]any{"workspace_id": testWorkspaceID, "id": testPostID, "body": helloBodyMap(t)}).errorOf(t)
			assert.Equal(t, CodePermissionDenied, toolErr.Code)
			assert.Equal(t, "templates", toolErr.Resource)
			assert.Equal(t, tc.permission, toolErr.Permission)
			assert.Equal(t, http.StatusForbidden, audit.last(t).StatusCode)
		})
	}
}

func TestUpdateBlogPost_RefusalsComeBeforeTheBodyIsSaved(t *testing.T) {
	t.Run("a stale base_version", func(t *testing.T) {
		f, audit := blogWriteFixture(t, blogWritePermissions(domain.ResourcePermissions{Read: true, Write: true}))
		f.blog.EXPECT().GetPost(gomock.Any(), testPostID).Return(scheduledPost(), nil)
		f.templates.EXPECT().GetTemplateByID(gomock.Any(), testWorkspaceID, "tpl-body", int64(0)).Return(bodyTemplate(), nil)
		f.templates.EXPECT().UpdateTemplate(gomock.Any(), testWorkspaceID, gomock.Any()).Return(&domain.ErrTemplateVersionConflict{BaseVersion: 4, LatestVersion: 5})
		toolErr := f.callTool("update_blog_post", map[string]any{"workspace_id": testWorkspaceID, "id": testPostID, "body": helloBodyMap(t), "base_version": 4}).errorOf(t)
		assert.Equal(t, CodeConflict, toolErr.Code)
		require.NotNil(t, toolErr.LatestVersion)
		assert.Equal(t, int64(5), *toolErr.LatestVersion)
		assert.Equal(t, http.StatusConflict, audit.last(t).StatusCode)
	})

	t.Run("a slug another post holds", func(t *testing.T) {
		f, _ := blogWriteFixture(t, blogWritePermissions(domain.ResourcePermissions{Read: true, Write: true}))
		f.blog.EXPECT().GetPost(gomock.Any(), testPostID).Return(scheduledPost(), nil)
		f.blog.EXPECT().GetPostBySlug(gomock.Any(), "taken").Return(&domain.BlogPost{ID: "other"}, nil)
		toolErr := f.callTool("update_blog_post", map[string]any{"workspace_id": testWorkspaceID, "id": testPostID, "slug": "taken", "body": helloBodyMap(t)}).errorOf(t)
		assert.Equal(t, ToolError{Code: CodeConflict, Message: "blog_post taken already exists"}, toolErr)
	})

	t.Run("a free slug and an unknown category", func(t *testing.T) {
		f, _ := blogWriteFixture(t, blogWritePermissions(domain.ResourcePermissions{Read: true, Write: true}))
		f.blog.EXPECT().GetPost(gomock.Any(), testPostID).Return(scheduledPost(), nil)
		other := "0e2f3d4c-5b6a-4978-8a9b-0c1d2e3f4a5b"
		f.categories.EXPECT().GetCategoriesByIDs(gomock.Any(), []string{other}).Return(nil, nil)
		toolErr := f.callTool("update_blog_post", map[string]any{"workspace_id": testWorkspaceID, "id": testPostID, "category_id": other, "body": helloBodyMap(t)}).errorOf(t)
		assert.Equal(t, CodeNotFound, toolErr.Code)
	})

	t.Run("arguments that change nothing or misuse base_version", func(t *testing.T) {
		f, _ := blogWriteFixture(t, blogWritePermissions(domain.ResourcePermissions{Read: true, Write: true}))
		for args, message := range map[string]string{
			`{"id":"` + testPostID + `"}`:                              "give at least one field to change besides id and base_version",
			`{"id":"` + testPostID + `","title":"x","base_version":3}`: "base_version goes with body, and must be 1 or more",
			`{"id":" ","title":"x"}`:                                   "id is required: the post's ID or slug, from list_blog_posts",
		} {
			var decoded map[string]any
			require.NoError(t, json.Unmarshal([]byte(args), &decoded))
			decoded["workspace_id"] = testWorkspaceID
			assert.Equal(t, ToolError{Code: CodeValidation, Message: message}, f.callTool("update_blog_post", decoded).errorOf(t), args)
		}
	})

	t.Run("a post that does not exist", func(t *testing.T) {
		f, audit := blogWriteFixture(t, blogWritePermissions(domain.ResourcePermissions{}))
		f.blog.EXPECT().GetPost(gomock.Any(), testPostID).Return(nil, &domain.ErrNotFound{Entity: "blog_post", ID: testPostID})
		f.blog.EXPECT().GetPostBySlug(gomock.Any(), testPostID).Return(nil, &domain.ErrNotFound{Entity: "blog_post", ID: testPostID})
		assert.Equal(t, ToolError{Code: CodeNotFound, Message: "blog post not found: " + testPostID},
			f.callTool("update_blog_post", map[string]any{"workspace_id": testWorkspaceID, "id": testPostID, "title": "x"}).errorOf(t))
		assert.Equal(t, http.StatusNotFound, audit.last(t).StatusCode)

		f.blog.EXPECT().GetPostBySlug(gomock.Any(), "spring").Return(nil, &domain.ErrNotFound{Entity: "blog_post", ID: "spring"})
		assert.Equal(t, ToolError{Code: CodeNotFound, Message: "blog post not found: spring"},
			f.callTool("update_blog_post", map[string]any{"workspace_id": testWorkspaceID, "id": "spring", "title": "x"}).errorOf(t))
		assert.Equal(t, "spring", audit.last(t).Metadata["slug"])
	})

	// The post is found by its current slug and written under its ID.
	t.Run("by slug", func(t *testing.T) {
		f, audit := blogWriteFixture(t, blogWritePermissions(domain.ResourcePermissions{Read: true}))
		f.blog.EXPECT().GetPostBySlug(gomock.Any(), "spring-release").Return(scheduledPost(), nil)
		f.blog.EXPECT().UpdatePost(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, req *domain.UpdateBlogPostRequest) (*domain.BlogPost, error) {
			assert.Equal(t, testPostID, req.ID)
			assert.Equal(t, "Spring", req.Title)
			assert.Equal(t, "spring-release", req.Slug, "the slug the post is found by is kept")
			updated := scheduledPost()
			updated.Settings.Title = req.Title
			return updated, nil
		})
		f.templates.EXPECT().GetTemplateByID(gomock.Any(), testWorkspaceID, "tpl-body", int64(0)).Return(bodyTemplate(), nil)

		var out struct {
			Post struct {
				ID    string `json:"id"`
				Title string `json:"title"`
			} `json:"post"`
		}
		f.callTool("update_blog_post", map[string]any{"workspace_id": testWorkspaceID, "id": "spring-release", "title": "Spring"}).decode(t, &out)
		assert.Equal(t, testPostID, out.Post.ID)
		assert.Equal(t, "Spring", out.Post.Title)
		row := audit.last(t)
		assert.Equal(t, testPostID, row.TargetID)
		assert.Equal(t, http.StatusOK, row.StatusCode)
	})
}

func TestPublishBlogPost(t *testing.T) {
	args := func(extra map[string]any) map[string]any {
		out := map[string]any{"workspace_id": testWorkspaceID, "id": testPostID}
		for key, value := range extra {
			out[key] = value
		}
		return out
	}

	t.Run("now", func(t *testing.T) {
		f, audit := blogWriteFixture(t, blogWritePermissions(domain.ResourcePermissions{}))
		before := time.Now().UTC().Add(-time.Second)
		var sent time.Time
		f.blog.EXPECT().PublishPost(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, req *domain.PublishBlogPostRequest) error {
			expectBlogWorkspace(t, ctx)
			assert.Equal(t, testPostID, req.ID)
			require.NotNil(t, req.PublishedAt)
			sent = *req.PublishedAt
			return nil
		})
		var out blogPostStateOutput
		f.callTool("publish_blog_post", args(nil)).decode(t, &out)
		assert.Equal(t, "published", out.Status)
		assert.False(t, out.Already)
		require.NotNil(t, out.PublishedAt)
		assert.True(t, out.PublishedAt.Equal(sent))
		assert.False(t, sent.Before(before))
		row := audit.last(t)
		assert.Equal(t, "blogPosts.publish", row.Action)
		assert.Equal(t, domain.AuditTargetBlogPost, row.TargetType)
		assert.Equal(t, testPostID, row.TargetID)
		assert.Equal(t, sent.Format(time.RFC3339), row.Metadata["published_at"])
	})

	t.Run("at a later date", func(t *testing.T) {
		f, _ := blogWriteFixture(t, blogWritePermissions(domain.ResourcePermissions{}))
		later := time.Now().Add(48 * time.Hour).UTC().Truncate(time.Second)
		f.blog.EXPECT().PublishPost(gomock.Any(), &domain.PublishBlogPostRequest{ID: testPostID, PublishedAt: &later}).Return(nil)
		result := f.callTool("publish_blog_post", args(map[string]any{"published_at": later.In(time.FixedZone("CET", 3600)).Format(time.RFC3339)}))
		assert.JSONEq(t, fmt.Sprintf(`{"workspace":{"id":"ws1","name":"Acme"},"post_id":%q,"status":"scheduled","published_at":%q,"already":false,"blog_enabled":false}`,
			testPostID, later.Format(time.RFC3339)), string(result.StructuredContent))
	})

	t.Run("a replay on a published or scheduled post", func(t *testing.T) {
		for _, status := range []string{"published", "scheduled"} {
			f, audit := blogWriteFixture(t, blogWritePermissions(domain.ResourcePermissions{}))
			f.blog.EXPECT().PublishPost(gomock.Any(), gomock.Any()).Return(fmt.Errorf("failed to publish post: %w", domain.NewInvalidStateError("blog_post", testPostID, "publish", status)))
			result := f.callTool("publish_blog_post", args(nil))
			assert.JSONEq(t, fmt.Sprintf(`{"workspace":{"id":"ws1","name":"Acme"},"post_id":%q,"status":%q,"published_at":null,"already":true,"blog_enabled":false}`, testPostID, status),
				string(result.StructuredContent))
			row := audit.last(t)
			assert.Equal(t, true, row.Metadata["already"])
			assert.Equal(t, http.StatusOK, row.StatusCode)
		}
	})

	t.Run("refusals", func(t *testing.T) {
		f, _ := blogWriteFixture(t, blogWritePermissions(domain.ResourcePermissions{}))
		assert.Equal(t, CodeValidation, f.callTool("publish_blog_post", args(map[string]any{"published_at": "tomorrow"})).errorOf(t).Code)
		f.blog.EXPECT().PublishPost(gomock.Any(), gomock.Any()).Return(fmt.Errorf("failed to publish post: %w", &domain.ErrNotFound{Entity: "blog_post", ID: testPostID}))
		f.blog.EXPECT().GetPostBySlug(gomock.Any(), testPostID).Return(nil, &domain.ErrNotFound{Entity: "blog_post", ID: testPostID})
		assert.Equal(t, ToolError{Code: CodeNotFound, Message: "blog post not found: " + testPostID}, f.callTool("publish_blog_post", args(nil)).errorOf(t))
	})

	// A slug is looked up, then the post is published under its ID.
	t.Run("by slug", func(t *testing.T) {
		f, audit := blogWriteFixture(t, blogWritePermissions(domain.ResourcePermissions{}))
		draft := scheduledPost()
		draft.PublishedAt = nil
		f.blog.EXPECT().GetPostBySlug(gomock.Any(), "spring-release").Return(draft, nil)
		f.blog.EXPECT().PublishPost(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, req *domain.PublishBlogPostRequest) error {
			assert.Equal(t, testPostID, req.ID)
			return nil
		})
		var out blogPostStateOutput
		f.callTool("publish_blog_post", args(map[string]any{"id": "spring-release"})).decode(t, &out)
		assert.Equal(t, testPostID, out.PostID)
		assert.Equal(t, "published", out.Status)
		row := audit.last(t)
		assert.Equal(t, testPostID, row.TargetID)
		assert.Equal(t, "What's new this spring", row.TargetName)
		assert.Equal(t, "spring-release", row.Metadata["slug"])

		f.blog.EXPECT().GetPostBySlug(gomock.Any(), "autumn").Return(nil, &domain.ErrNotFound{Entity: "blog_post", ID: "autumn"})
		assert.Equal(t, ToolError{Code: CodeNotFound, Message: "blog post not found: autumn"}, f.callTool("publish_blog_post", args(map[string]any{"id": "autumn"})).errorOf(t))
	})
}

func TestUnpublishBlogPost(t *testing.T) {
	f, audit := blogWriteFixture(t, blogWritePermissions(domain.ResourcePermissions{}))
	args := map[string]any{"workspace_id": testWorkspaceID, "id": testPostID}
	f.blog.EXPECT().UnpublishPost(gomock.Any(), &domain.UnpublishBlogPostRequest{ID: testPostID}).Return(nil)
	result := f.callTool("unpublish_blog_post", args)
	assert.JSONEq(t, fmt.Sprintf(`{"workspace":{"id":"ws1","name":"Acme"},"post_id":%q,"status":"draft","published_at":null,"already":false,"blog_enabled":false}`, testPostID), string(result.StructuredContent))
	assert.Equal(t, "blogPosts.unpublish", audit.last(t).Action)

	f.blog.EXPECT().UnpublishPost(gomock.Any(), gomock.Any()).Return(fmt.Errorf("x: %w", domain.NewInvalidStateError("blog_post", testPostID, "unpublish", "draft")))
	result = f.callTool("unpublish_blog_post", args)
	assert.JSONEq(t, fmt.Sprintf(`{"workspace":{"id":"ws1","name":"Acme"},"post_id":%q,"status":"draft","published_at":null,"already":true,"blog_enabled":false}`, testPostID), string(result.StructuredContent))
	assert.Equal(t, true, audit.last(t).Metadata["already"])

	f.blog.EXPECT().UnpublishPost(gomock.Any(), gomock.Any()).Return(&domain.ErrNotFound{Entity: "blog_post", ID: testPostID})
	f.blog.EXPECT().GetPostBySlug(gomock.Any(), testPostID).Return(nil, &domain.ErrNotFound{Entity: "blog_post", ID: testPostID})
	assert.Equal(t, CodeNotFound, f.callTool("unpublish_blog_post", args).errorOf(t).Code)
	assert.Equal(t, http.StatusNotFound, audit.last(t).StatusCode)
}

func TestDeleteBlogPost_DeletesThenAnswersNotFound(t *testing.T) {
	f, audit := blogWriteFixture(t, blogWritePermissions(domain.ResourcePermissions{}))
	args := map[string]any{"workspace_id": testWorkspaceID, "id": testPostID}
	f.blog.EXPECT().DeletePost(gomock.Any(), &domain.DeleteBlogPostRequest{ID: testPostID}).DoAndReturn(func(ctx context.Context, _ *domain.DeleteBlogPostRequest) error {
		expectBlogWorkspace(t, ctx)
		return nil
	})
	result := f.callTool("delete_blog_post", args)
	assert.JSONEq(t, fmt.Sprintf(`{"workspace":{"id":"ws1","name":"Acme"},"post_id":%q,"deleted":true,"blog_enabled":false}`, testPostID), string(result.StructuredContent))
	row := audit.last(t)
	assert.Equal(t, "blogPosts.delete", row.Action)
	assert.Equal(t, domain.AuditTargetBlogPost, row.TargetType)
	assert.Equal(t, testPostID, row.TargetID)
	assert.Equal(t, http.StatusOK, row.StatusCode)

	f.blog.EXPECT().DeletePost(gomock.Any(), gomock.Any()).Return(fmt.Errorf("post not found: %w", &domain.ErrNotFound{Entity: "blog_post", ID: testPostID}))
	f.blog.EXPECT().GetPostBySlug(gomock.Any(), testPostID).Return(nil, &domain.ErrNotFound{Entity: "blog_post", ID: testPostID})
	assert.Equal(t, ToolError{Code: CodeNotFound, Message: "blog post not found: " + testPostID}, f.callTool("delete_blog_post", args).errorOf(t))
	assert.Equal(t, http.StatusNotFound, audit.last(t).StatusCode)

	// A slug is looked up by slug, never sent to the UUID column.
	f.blog.EXPECT().GetPostBySlug(gomock.Any(), "spring-release").Return(scheduledPost(), nil)
	f.blog.EXPECT().DeletePost(gomock.Any(), &domain.DeleteBlogPostRequest{ID: testPostID}).Return(nil)
	result = f.callTool("delete_blog_post", map[string]any{"workspace_id": testWorkspaceID, "id": "spring-release"})
	assert.JSONEq(t, fmt.Sprintf(`{"workspace":{"id":"ws1","name":"Acme"},"post_id":%q,"deleted":true,"blog_enabled":false}`, testPostID), string(result.StructuredContent))
	f.blog.EXPECT().GetPostBySlug(gomock.Any(), "spring-release").Return(nil, &domain.ErrNotFound{Entity: "blog_post", ID: "spring-release"})
	assert.Equal(t, ToolError{Code: CodeNotFound, Message: "blog post not found: spring-release"},
		f.callTool("delete_blog_post", map[string]any{"workspace_id": testWorkspaceID, "id": "spring-release"}).errorOf(t))
}

// A slug is looked up with the blog service's read: a key that may write the
// blog but not read it gives the post's ID.
func TestBlogWriteTools_ASlugNeedsBlogRead(t *testing.T) {
	f, _ := blogWriteFixture(t, domain.UserPermissions{domain.PermissionResourceBlog: {Write: true}})
	for _, tool := range []string{"publish_blog_post", "unpublish_blog_post", "delete_blog_post"} {
		toolErr := f.callTool(tool, map[string]any{"workspace_id": testWorkspaceID, "id": "spring-release"}).errorOf(t)
		assert.Equal(t, ToolError{Code: CodePermissionDenied, Message: "this API key lacks the blog read permission", Resource: "blog", Permission: "read"}, toolErr, tool)
		assert.Equal(t, []Pair{{Resource: "blog", Verb: "read"}}, catalogueTool(t, tool).ArgumentPairs(map[string]any{"id": "spring-release"}), tool)
		assert.Empty(t, catalogueTool(t, tool).ArgumentPairs(map[string]any{"id": testPostID}), tool)
	}
}

// A slug can look like a UUID, hyphenated or as 32 hex digits. When no post has
// that ID, the value is tried as a slug, and kept as given for the lookup.
func TestBlogTools_ASlugThatLooksLikeAUUID(t *testing.T) {
	slugs := map[string]string{
		"8c1f6a2e-4b3d-4e5f-9a0b-1c2d3e4f5a6b": "8c1f6a2e-4b3d-4e5f-9a0b-1c2d3e4f5a6b",
		"8c1f6a2e4b3d4e5f9a0b1c2d3e4f5a6b":     "8c1f6a2e-4b3d-4e5f-9a0b-1c2d3e4f5a6b",
	}
	for slug, asID := range slugs {
		t.Run("get_blog_post "+slug, func(t *testing.T) {
			f := newFixture(t)
			f.reachable(blogReadPermission())
			f.blog.EXPECT().GetPost(gomock.Any(), asID).Return(nil, &domain.ErrNotFound{Entity: "blog_post", ID: asID})
			f.blog.EXPECT().GetPostBySlug(gomock.Any(), slug).Return(scheduledPost(), nil)
			var out struct {
				Post struct {
					ID string `json:"id"`
				} `json:"post"`
			}
			f.callTool("get_blog_post", map[string]any{"workspace_id": testWorkspaceID, "id": slug}).decode(t, &out)
			assert.Equal(t, testPostID, out.Post.ID)
		})

		t.Run("write tools "+slug, func(t *testing.T) {
			f, audit := blogWriteFixture(t, blogWritePermissions(domain.ResourcePermissions{}))
			missing := fmt.Errorf("failed to get post: %w", &domain.ErrNotFound{Entity: "blog_post", ID: asID})
			draft := scheduledPost()
			draft.PublishedAt = nil
			f.blog.EXPECT().GetPostBySlug(gomock.Any(), slug).Return(draft, nil).Times(3)
			gomock.InOrder(
				f.blog.EXPECT().PublishPost(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, req *domain.PublishBlogPostRequest) error {
					assert.Equal(t, asID, req.ID)
					return missing
				}),
				f.blog.EXPECT().PublishPost(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, req *domain.PublishBlogPostRequest) error {
					assert.Equal(t, testPostID, req.ID)
					return nil
				}),
			)
			gomock.InOrder(
				f.blog.EXPECT().UnpublishPost(gomock.Any(), &domain.UnpublishBlogPostRequest{ID: asID}).Return(missing),
				f.blog.EXPECT().UnpublishPost(gomock.Any(), &domain.UnpublishBlogPostRequest{ID: testPostID}).Return(nil),
			)
			gomock.InOrder(
				f.blog.EXPECT().DeletePost(gomock.Any(), &domain.DeleteBlogPostRequest{ID: asID}).Return(missing),
				f.blog.EXPECT().DeletePost(gomock.Any(), &domain.DeleteBlogPostRequest{ID: testPostID}).Return(nil),
			)
			for _, tool := range []string{"publish_blog_post", "unpublish_blog_post", "delete_blog_post"} {
				var out struct {
					PostID string `json:"post_id"`
				}
				f.callTool(tool, map[string]any{"workspace_id": testWorkspaceID, "id": slug}).decode(t, &out)
				assert.Equal(t, testPostID, out.PostID, tool)
				assert.Equal(t, testPostID, audit.last(t).TargetID, tool)
			}
		})
	}

	// The slug is read with the blog service's read: without blog:read, a UUID
	// that names no post answers not_found.
	t.Run("without blog read", func(t *testing.T) {
		f, _ := blogWriteFixture(t, domain.UserPermissions{domain.PermissionResourceBlog: {Write: true}})
		f.blog.EXPECT().DeletePost(gomock.Any(), gomock.Any()).Return(&domain.ErrNotFound{Entity: "blog_post", ID: testPostID})
		assert.Equal(t, ToolError{Code: CodeNotFound, Message: "blog post not found: " + testPostID},
			f.callTool("delete_blog_post", map[string]any{"workspace_id": testWorkspaceID, "id": testPostID}).errorOf(t))
	})
}

// A key that may only read the blog sees none of the write tools.
func TestBlogWriteTools_NeedBlogWrite(t *testing.T) {
	f := newFixture(t)
	f.reachable(blogReadPermission())
	names := f.listToolNames()
	for _, tool := range []string{"create_blog_post", "update_blog_post", "publish_blog_post", "unpublish_blog_post", "delete_blog_post"} {
		assert.NotContains(t, names, tool)
	}
}
