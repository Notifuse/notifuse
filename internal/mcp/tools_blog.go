package mcp

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/Notifuse/notifuse/internal/domain"
)

const (
	listBlogPostsName      = "list_blog_posts"
	listBlogCategoriesName = "list_blog_categories"
)

var blogRead = Pair{Resource: domain.PermissionResourceBlog, Verb: domain.PermissionTypeRead}

// Why get_blog_post answers body: null.
const (
	bodyUnavailablePermission = "permission_denied"
	bodyUnavailableNotFound   = "not_found"
)

// blogContext is the context the blog service reads the workspace from.
func blogContext(ctx context.Context, call *Call) context.Context {
	return context.WithValue(ctx, domain.WorkspaceIDKey, call.WorkspaceID)
}

// blogPostStatus is the status of a post as the tools name it: draft without
// a publication date, scheduled with one still to come, published otherwise.
func blogPostStatus(post *domain.BlogPost, now time.Time) string {
	switch {
	case post.IsDraft():
		return string(domain.BlogPostStatusDraft)
	case post.IsLive(now):
		return string(domain.BlogPostStatusPublished)
	default:
		return string(domain.BlogPostStatusScheduled)
	}
}

// listBlogPostsTool pages through a workspace's blog posts. blogPosts.list
// pages by page number on an order with a tie-breaker, so the cursor carries
// the page and its size.
func (s *Server) listBlogPostsTool() *Tool {
	return &Tool{
		Name:  listBlogPostsName,
		Title: "List blog posts",
		Description: "Lists the posts of the workspace's blog: ID, category, slug, title, excerpt, status (draft, scheduled or published) and publication date. " +
			"Drafts are sorted by last update, scheduled posts by the date they go live, and the others by publication date, newest first. " +
			"get_blog_post has the authors, SEO settings and body. total counts every post that matches the filters. " +
			"To get the next page, pass next_cursor back as cursor with the same filters. Later pages keep the page size of the first page.",
		Class: ClassRead,
		Pairs: []Pair{blogRead},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"workspace_id": map[string]any{"type": "string", "description": "Workspace ID from list_workspaces."},
				"category_id":  map[string]any{"type": "string", "description": "Only posts in this category, by its ID (a UUID) from list_blog_categories."},
				"status": map[string]any{
					"type":        "string",
					"enum":        []string{"all", "draft", "published", "scheduled"},
					"description": "Default: all. published means live now. scheduled means published with a date still to come.",
				},
				"search": map[string]any{"type": "string", "description": "Part of the title to match, case-insensitive. At most 200 characters."},
				"limit":  map[string]any{"type": "integer", "description": "1 to 50, default 20."},
				"cursor": map[string]any{"type": "string", "description": "next_cursor of the previous page, with the same filters."},
			},
			"required":             []string{"workspace_id"},
			"additionalProperties": false,
		},
		Handler: s.listBlogPosts,
	}
}

type listBlogPostsArgs struct {
	CategoryID string `json:"category_id"`
	Status     string `json:"status"`
	Search     string `json:"search"`
	Limit      *int   `json:"limit"`
	Cursor     string `json:"cursor"`
}

// blogPostsPage is the inner cursor of list_blog_posts.
type blogPostsPage struct {
	Page  int `json:"p"`
	Limit int `json:"l"`
}

type blogPostSummary struct {
	ID          string     `json:"id"`
	CategoryID  string     `json:"category_id"`
	Slug        string     `json:"slug"`
	Title       string     `json:"title"`
	Excerpt     string     `json:"excerpt"`
	Status      string     `json:"status"`
	PublishedAt *time.Time `json:"published_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	Truncation
}

type listBlogPostsOutput struct {
	Workspace  WorkspaceRef      `json:"workspace"`
	Posts      []blogPostSummary `json:"posts"`
	Total      int               `json:"total"`
	NextCursor string            `json:"next_cursor,omitempty"`
}

func (s *Server) listBlogPosts(ctx context.Context, call *Call) (any, error) {
	var args listBlogPostsArgs
	if err := call.DecodeArgs(&args); err != nil {
		return nil, validationError("invalid arguments: %v", err)
	}
	limit, toolErr := PageLimit(args.Limit)
	if toolErr != nil {
		return nil, toolErr
	}
	search, err := domain.NormalizeBlogPostSearch(args.Search)
	if err != nil {
		return nil, err
	}
	// The column is a UUID: blogPosts.list reads it as one in its handler, and
	// the service would pass anything else to PostgreSQL.
	categoryID := ""
	if args.CategoryID != "" {
		id, err := uuid.Parse(args.CategoryID)
		if err != nil {
			return nil, validationError("category_id must be a UUID, from list_blog_categories")
		}
		categoryID = id.String()
	}

	filters := map[string]any{}
	if categoryID != "" {
		filters["category_id"] = categoryID
	}
	if args.Status != "" && args.Status != string(domain.BlogPostStatusAll) {
		filters["status"] = args.Status
	}
	if search != "" {
		filters["search"] = search
	}
	page := blogPostsPage{Page: 1, Limit: limit}
	if args.Cursor != "" {
		if toolErr := DecodeCursor(args.Cursor, listBlogPostsName, call.WorkspaceID, filters, &page); toolErr != nil {
			return nil, toolErr
		}
		if page.Page < 1 || page.Limit < 1 || page.Limit > MaxPageLimit {
			return nil, validationError("cursor is not valid for this call: drop it to start from the first page")
		}
	}

	req := &domain.ListBlogPostsRequest{
		CategoryID: categoryID,
		Status:     domain.BlogPostStatus(args.Status),
		Search:     search,
		Page:       page.Page,
		Limit:      page.Limit,
	}
	resp, err := s.deps.Blog.ListPosts(blogContext(ctx, call), req)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	out := listBlogPostsOutput{Workspace: call.WorkspaceRef(), Posts: []blogPostSummary{}}
	if resp == nil {
		return out, nil
	}
	out.Total = resp.TotalCount
	for _, post := range resp.Posts {
		if post == nil {
			continue
		}
		summary := blogPostSummary{
			ID:          post.ID,
			CategoryID:  post.CategoryID,
			Slug:        post.Slug,
			Status:      blogPostStatus(post, now),
			PublishedAt: post.PublishedAt,
			CreatedAt:   post.CreatedAt,
			UpdatedAt:   post.UpdatedAt,
		}
		summary.Title = summary.Text("title", post.Settings.Title)
		summary.Excerpt = summary.Text("excerpt", post.Settings.Excerpt)
		out.Posts = append(out.Posts, summary)
	}
	if resp.HasNextPage {
		next, err := EncodeCursor(listBlogPostsName, call.WorkspaceID, filters, blogPostsPage{Page: page.Page + 1, Limit: page.Limit})
		if err != nil {
			return nil, err
		}
		out.NextCursor = next
	}
	return out, nil
}

// getBlogPostTool reads one blog post with its body.
func (s *Server) getBlogPostTool() *Tool {
	return &Tool{
		Name:  "get_blog_post",
		Title: "Get blog post",
		Description: "Reads one blog post by ID or slug. Give one, not both. " +
			"It returns the category, slug, title, excerpt, featured image, authors, reading time, SEO settings, status, publication date, and the body as a Tiptap (ProseMirror) JSON document. " +
			"Reading the body also needs the templates read permission. " +
			"Without it, or when the body cannot be found, body is null and body_unavailable says why. " +
			"body_version is the version of the body that was read. " +
			"follows_latest_body is true when the post shows the latest version of its body, and false when it shows a pinned version.",
		Class: ClassRead,
		Pairs: []Pair{blogRead},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"workspace_id": map[string]any{"type": "string", "description": "Workspace ID from list_workspaces."},
				"id":           map[string]any{"type": "string", "description": "The post's ID (a UUID) from list_blog_posts. Give id or slug, not both."},
				"slug":         map[string]any{"type": "string", "description": "The post's slug, unique across the blog. Give id or slug, not both."},
			},
			"required":             []string{"workspace_id"},
			"additionalProperties": false,
		},
		Handler: s.getBlogPost,
	}
}

type getBlogPostArgs struct {
	ID   string `json:"id"`
	Slug string `json:"slug"`
}

type blogPostDetail struct {
	ID                 string              `json:"id"`
	CategoryID         string              `json:"category_id"`
	Slug               string              `json:"slug"`
	Title              string              `json:"title"`
	Excerpt            string              `json:"excerpt"`
	FeaturedImageURL   string              `json:"featured_image_url"`
	Authors            []domain.BlogAuthor `json:"authors"`
	ReadingTimeMinutes int                 `json:"reading_time_minutes"`
	SEO                *domain.SEOSettings `json:"seo"`
	Status             string              `json:"status"`
	PublishedAt        *time.Time          `json:"published_at"`
	CreatedAt          time.Time           `json:"created_at"`
	UpdatedAt          time.Time           `json:"updated_at"`

	// Body is the Tiptap JSON of the post's web template, whole.
	Body              domain.MapOfAny `json:"body"`
	BodyVersion       *int64          `json:"body_version"`
	FollowsLatestBody bool            `json:"follows_latest_body"`
	BodyUnavailable   string          `json:"body_unavailable,omitempty"`
	Truncation
}

type getBlogPostOutput struct {
	Workspace WorkspaceRef   `json:"workspace"`
	Post      blogPostDetail `json:"post"`
}

func (s *Server) getBlogPost(ctx context.Context, call *Call) (any, error) {
	var args getBlogPostArgs
	if err := call.DecodeArgs(&args); err != nil {
		return nil, validationError("invalid arguments: %v", err)
	}
	if (args.ID == "") == (args.Slug == "") {
		return nil, validationError("give either id or slug, not both")
	}

	blogCtx := blogContext(ctx, call)
	var post *domain.BlogPost
	var err error
	if args.ID != "" {
		// The column is a UUID, and PostgreSQL refuses anything else.
		id, parseErr := uuid.Parse(args.ID)
		if parseErr != nil {
			return nil, validationError("id must be a UUID, from list_blog_posts; give a slug as slug")
		}
		post, err = s.deps.Blog.GetPost(blogCtx, id.String())
	} else {
		post, err = s.deps.Blog.GetPostBySlug(blogCtx, args.Slug)
	}
	if err != nil {
		return nil, err
	}
	if post == nil || post.DeletedAt != nil {
		return nil, &domain.ErrNotFound{Entity: "blog post", ID: args.ID + args.Slug}
	}

	out := blogPostDetail{
		ID:                 post.ID,
		CategoryID:         post.CategoryID,
		Slug:               post.Slug,
		Authors:            []domain.BlogAuthor{},
		ReadingTimeMinutes: post.Settings.ReadingTimeMinutes,
		Status:             blogPostStatus(post, time.Now()),
		PublishedAt:        post.PublishedAt,
		CreatedAt:          post.CreatedAt,
		UpdatedAt:          post.UpdatedAt,
		FollowsLatestBody:  post.Settings.Template.TemplateVersion == 0,
	}
	// The body is editable content and stays whole; the rest is capped.
	out.Title = out.Text("title", post.Settings.Title)
	out.Excerpt = out.Text("excerpt", post.Settings.Excerpt)
	out.FeaturedImageURL = out.Text("featured_image_url", post.Settings.FeaturedImageURL)
	for i, author := range post.Settings.Authors {
		out.Authors = append(out.Authors, domain.BlogAuthor{
			Name:      out.Text(fmt.Sprintf("authors[%d].name", i), author.Name),
			AvatarURL: out.Text(fmt.Sprintf("authors[%d].avatar_url", i), author.AvatarURL),
		})
	}
	if seo := post.Settings.SEO; seo != nil {
		out.SEO = &domain.SEOSettings{
			MetaTitle:       out.Text("seo.meta_title", seo.MetaTitle),
			MetaDescription: out.Text("seo.meta_description", seo.MetaDescription),
			OGTitle:         out.Text("seo.og_title", seo.OGTitle),
			OGDescription:   out.Text("seo.og_description", seo.OGDescription),
			OGImage:         out.Text("seo.og_image", seo.OGImage),
			CanonicalURL:    out.Text("seo.canonical_url", seo.CanonicalURL),
			MetaRobots:      out.Text("seo.meta_robots", seo.MetaRobots),
		}
		for i, keyword := range seo.Keywords {
			out.SEO.Keywords = append(out.SEO.Keywords, out.Text(fmt.Sprintf("seo.keywords[%d]", i), keyword))
		}
	}
	if err := s.readBlogBody(ctx, call, post, &out); err != nil {
		return nil, err
	}
	return getBlogPostOutput{Workspace: call.WorkspaceRef(), Post: out}, nil
}

// readBlogBody fills the body from the post's web template. Reading it needs
// templates:read, which the tool does not require: without it the post is
// answered with body null rather than refused. A version of 0 reads the
// latest one, as the public page does.
func (s *Server) readBlogBody(ctx context.Context, call *Call, post *domain.BlogPost, out *blogPostDetail) error {
	if !call.Effective[domain.PermissionResourceTemplates].Read {
		out.BodyUnavailable = bodyUnavailablePermission
		return nil
	}
	ref := post.Settings.Template
	if ref.TemplateID == "" {
		out.BodyUnavailable = bodyUnavailableNotFound
		return nil
	}
	template, err := s.deps.Templates.GetTemplateByID(ctx, call.WorkspaceID, ref.TemplateID, int64(ref.TemplateVersion))
	var notFound *domain.ErrTemplateNotFound
	var denied *domain.PermissionError
	switch {
	case errors.As(err, &notFound):
		out.BodyUnavailable = bodyUnavailableNotFound
		return nil
	case errors.As(err, &denied):
		out.BodyUnavailable = bodyUnavailablePermission
		return nil
	case err != nil:
		return err
	}
	if template == nil || template.DeletedAt != nil || template.Web == nil || len(template.Web.Content) == 0 {
		out.BodyUnavailable = bodyUnavailableNotFound
		return nil
	}
	out.Body = template.Web.Content
	version := template.Version
	out.BodyVersion = &version
	return nil
}

// listBlogCategoriesTool pages through the blog's categories.
// blogCategories.list returns every category at once, so the tool pages them
// by offset on (created_at, id).
func (s *Server) listBlogCategoriesTool() *Tool {
	return &Tool{
		Name:  listBlogCategoriesName,
		Title: "List blog categories",
		Description: "Lists the categories of the workspace's blog, newest first: ID (the value list_blog_posts filters on), slug, name and description. " +
			"total counts every category. To get the next page, pass next_cursor back as cursor.",
		Class: ClassRead,
		Pairs: []Pair{blogRead},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"workspace_id": map[string]any{"type": "string", "description": "Workspace ID from list_workspaces."},
				"limit":        map[string]any{"type": "integer", "description": "1 to 50, default 20."},
				"cursor":       map[string]any{"type": "string", "description": "next_cursor of the previous page."},
			},
			"required":             []string{"workspace_id"},
			"additionalProperties": false,
		},
		Handler: s.listBlogCategories,
	}
}

type listBlogCategoriesArgs struct {
	Limit  *int   `json:"limit"`
	Cursor string `json:"cursor"`
}

type blogCategorySummary struct {
	ID          string `json:"id"`
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Truncation
}

type listBlogCategoriesOutput struct {
	Workspace  WorkspaceRef          `json:"workspace"`
	Categories []blogCategorySummary `json:"categories"`
	Total      int                   `json:"total"`
	NextCursor string                `json:"next_cursor,omitempty"`
}

func (s *Server) listBlogCategories(ctx context.Context, call *Call) (any, error) {
	var args listBlogCategoriesArgs
	if err := call.DecodeArgs(&args); err != nil {
		return nil, validationError("invalid arguments: %v", err)
	}
	limit, toolErr := PageLimit(args.Limit)
	if toolErr != nil {
		return nil, toolErr
	}
	filters := map[string]any{}
	offset := 0
	if args.Cursor != "" {
		if toolErr := DecodeCursor(args.Cursor, listBlogCategoriesName, call.WorkspaceID, filters, &offset); toolErr != nil {
			return nil, toolErr
		}
		if offset < 0 {
			return nil, validationError("cursor is not valid for this call: drop it to start from the first page")
		}
	}

	resp, err := s.deps.Blog.ListCategories(blogContext(ctx, call))
	if err != nil {
		return nil, err
	}
	var categories []*domain.BlogCategory
	if resp != nil {
		categories = slices.DeleteFunc(slices.Clone(resp.Categories), func(c *domain.BlogCategory) bool {
			return c == nil || c.DeletedAt != nil
		})
	}
	slices.SortFunc(categories, func(a, b *domain.BlogCategory) int {
		if c := b.CreatedAt.Compare(a.CreatedAt); c != 0 {
			return c
		}
		return cmp.Compare(a.ID, b.ID)
	})

	out := listBlogCategoriesOutput{
		Workspace:  call.WorkspaceRef(),
		Categories: []blogCategorySummary{},
		Total:      len(categories),
	}
	if offset >= len(categories) {
		return out, nil
	}
	end := min(offset+limit, len(categories))
	for _, category := range categories[offset:end] {
		summary := blogCategorySummary{ID: category.ID, Slug: category.Slug}
		summary.Name = summary.Text("name", category.Settings.Name)
		summary.Description = summary.Text("description", category.Settings.Description)
		out.Categories = append(out.Categories, summary)
	}
	if end < len(categories) {
		next, err := EncodeCursor(listBlogCategoriesName, call.WorkspaceID, filters, end)
		if err != nil {
			return nil, err
		}
		out.NextCursor = next
	}
	return out, nil
}
