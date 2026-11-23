package mcp

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
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

	out := projectBlogPost(post, time.Now())
	if err := s.readBlogBody(ctx, call, post, &out); err != nil {
		return nil, err
	}
	return getBlogPostOutput{Workspace: call.WorkspaceRef(), Post: out}, nil
}

// projectBlogPost is a post as get_blog_post and the blog write tools return
// it, without its body.
func projectBlogPost(post *domain.BlogPost, now time.Time) blogPostDetail {
	out := blogPostDetail{
		ID:                 post.ID,
		CategoryID:         post.CategoryID,
		Slug:               post.Slug,
		Authors:            []domain.BlogAuthor{},
		ReadingTimeMinutes: post.Settings.ReadingTimeMinutes,
		Status:             blogPostStatus(post, now),
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
	return out
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

var (
	blogWrite      = Pair{Resource: domain.PermissionResourceBlog, Verb: domain.PermissionTypeWrite}
	writeTemplates = Pair{Resource: domain.PermissionResourceTemplates, Verb: domain.PermissionTypeWrite}
)

// blogTemplateNameMaxLength is the console's TEMPLATE_NAME_MAX_LENGTH.
const blogTemplateNameMaxLength = 255

// blogTemplateName is the name of the template that holds a post's body, as
// the console's blogTemplateName gives it: cut by code points.
func blogTemplateName(title string) string {
	runes := []rune("Blog: " + title)
	if len(runes) > blogTemplateNameMaxLength {
		runes = runes[:blogTemplateNameMaxLength]
	}
	return string(runes)
}

// readingTimeMinutes estimates a body's reading time at 200 words a minute,
// one minute at least.
func readingTimeMinutes(plainText string) int {
	return max(1, (len(strings.Fields(plainText))+199)/200)
}

// blogBodyDescription describes the body argument like the console's blog AI
// assistant describes it to its model.
const blogBodyDescription = `The post's body as a Tiptap JSON document, the format of the console's blog editor: {"type":"doc","content":[...]}. ` +
	"Block nodes: paragraph and heading (attrs.level 1 to 6, optional attrs.textAlign) hold text and hardBreak nodes. " +
	"bulletList and orderedList (optional attrs.start) hold listItem nodes, and each listItem holds a paragraph or other blocks. " +
	"blockquote holds blocks. codeBlock (attrs.language) holds text without marks. horizontalRule has no content. " +
	"image takes attrs.src, the http or https URL of a real image, and optional attrs.alt, attrs.width and attrs.align. " +
	"youtube takes attrs.src, the 11-character video ID and not a URL. " +
	`A text node is {"type":"text","text":"..."} with optional marks: bold, italic, underline, strike, code, subscript, superscript, ` +
	"link (attrs.href, an http, https or mailto URL, optional attrs.target), textStyle (attrs.color) and highlight (attrs.color). " +
	`Example: {"type":"doc","content":[{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"Title"}]},{"type":"paragraph","content":[{"type":"text","text":"Text."}]}]}.`

// blogPostProperties are the arguments create_blog_post and update_blog_post
// share.
func blogPostProperties() map[string]any {
	return map[string]any{
		"workspace_id":       map[string]any{"type": "string", "description": "Workspace ID from list_workspaces."},
		"category_id":        map[string]any{"type": "string", "description": "Category ID (a UUID) from list_blog_categories."},
		"slug":               map[string]any{"type": "string", "description": "The post's URL path: lowercase letters, digits and single hyphens, at most 100 characters. It must be unique among the posts that are not deleted."},
		"title":              map[string]any{"type": "string", "description": "The post's title, shown as its heading. At most 500 characters."},
		"body":               map[string]any{"type": "object", "description": blogBodyDescription},
		"excerpt":            map[string]any{"type": "string", "description": "Summary shown in post lists."},
		"featured_image_url": map[string]any{"type": "string", "description": "http or https URL of the cover image."},
		"authors": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":       map[string]any{"type": "string"},
					"avatar_url": map[string]any{"type": "string", "description": "http or https URL."},
				},
				"required":             []string{"name"},
				"additionalProperties": false,
			},
			"description": "The post's authors, in order.",
		},
		"reading_time_minutes": map[string]any{"type": "integer", "description": "Reading time shown on the post, 1 to 600 minutes."},
		"seo": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"meta_title":       map[string]any{"type": "string"},
				"meta_description": map[string]any{"type": "string"},
				"og_title":         map[string]any{"type": "string"},
				"og_description":   map[string]any{"type": "string"},
				"og_image":         map[string]any{"type": "string", "description": "http or https URL."},
				"canonical_url":    map[string]any{"type": "string", "description": "http or https URL."},
				"keywords":         map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"meta_robots":      map[string]any{"type": "string", "description": "Such as index,follow or noindex,nofollow."},
			},
			"additionalProperties": false,
			"description":          "Search and sharing settings. Given, it replaces the stored settings as a whole.",
		},
	}
}

// blogPostWriteArgs are the arguments of create_blog_post and
// update_blog_post. A pointer tells an omitted field from an empty one.
type blogPostWriteArgs struct {
	ID                 string               `json:"id"`
	CategoryID         *string              `json:"category_id"`
	Slug               *string              `json:"slug"`
	Title              *string              `json:"title"`
	Body               map[string]any       `json:"body"`
	BaseVersion        *int64               `json:"base_version"`
	Excerpt            *string              `json:"excerpt"`
	FeaturedImageURL   *string              `json:"featured_image_url"`
	Authors            *[]domain.BlogAuthor `json:"authors"`
	ReadingTimeMinutes *int                 `json:"reading_time_minutes"`
	SEO                *domain.SEOSettings  `json:"seo"`
}

// decodeBlogPostWrite applies the 1 MiB cap of templates.create and
// templates.update, which write the body, then decodes and checks the
// arguments that are given.
func decodeBlogPostWrite(call *Call) (blogPostWriteArgs, *ToolError) {
	var args blogPostWriteArgs
	if len(call.RawArgs) > maxTemplateArgumentBytes {
		return args, validationError("arguments are limited to %d bytes", maxTemplateArgumentBytes)
	}
	if err := call.DecodeArgs(&args); err != nil {
		return args, validationError("invalid arguments: %v", err)
	}
	if args.CategoryID != nil {
		id, err := uuid.Parse(*args.CategoryID)
		if err != nil {
			return args, validationError("category_id must be a UUID, from list_blog_categories")
		}
		canonical := id.String()
		args.CategoryID = &canonical
	}
	if args.ReadingTimeMinutes != nil && (*args.ReadingTimeMinutes < 1 || *args.ReadingTimeMinutes > 600) {
		return args, validationError("reading_time_minutes must be from 1 to 600")
	}
	urls := map[string]*string{"featured_image_url": args.FeaturedImageURL}
	if args.SEO != nil {
		urls["seo.og_image"] = &args.SEO.OGImage
		urls["seo.canonical_url"] = &args.SEO.CanonicalURL
	}
	if args.Authors != nil {
		// The console drops an author without a name.
		authors := slices.DeleteFunc(slices.Clone(*args.Authors), func(a domain.BlogAuthor) bool { return strings.TrimSpace(a.Name) == "" })
		args.Authors = &authors
		for i := range authors {
			urls[fmt.Sprintf("authors[%d].avatar_url", i)] = &authors[i].AvatarURL
		}
	}
	for _, key := range slices.Sorted(maps.Keys(urls)) {
		if value := urls[key]; value != nil && *value != "" && !hasScheme(*value, "http://", "https://") {
			return args, validationError("%s must be an http or https URL", key)
		}
	}
	return args, nil
}

// renderBlogBody checks a body and renders what its web template stores.
func renderBlogBody(body map[string]any) (*domain.WebTemplate, *ToolError) {
	if toolErr := checkTiptapDocument(body); toolErr != nil {
		return nil, toolErr
	}
	return &domain.WebTemplate{Content: body, HTML: renderTiptapHTML(body), PlainText: tiptapPlainText(body)}, nil
}

// requireBlogCategory refuses a category that does not exist or is deleted.
func (s *Server) requireBlogCategory(ctx context.Context, id string) error {
	categories, err := s.deps.BlogCategories.GetCategoriesByIDs(ctx, []string{id})
	if err != nil {
		return err
	}
	for _, category := range categories {
		if category != nil && category.ID == id && category.DeletedAt == nil {
			return nil
		}
	}
	return &ToolError{Code: CodeNotFound, Message: "blog category not found: list_blog_categories gives the IDs"}
}

// blogPostArgument reads a post's id, which must be a UUID, and names it as
// the audit row's target.
func blogPostArgument(call *Call) (string, *ToolError) {
	raw, _ := call.Args["id"].(string)
	id, err := uuid.Parse(raw)
	if err != nil {
		return "", validationError("id must be the post's ID (a UUID), from list_blog_posts")
	}
	call.Audit.SetTarget(domain.AuditTargetBlogPost, id.String(), "")
	return id.String(), nil
}

func (s *Server) createBlogPostTool() *Tool {
	properties := blogPostProperties()
	return &Tool{
		Name:  "create_blog_post",
		Title: "Create blog post",
		Description: "Creates a draft blog post with its body. The body is saved as a web template, which the post follows at its latest version, and its HTML is made from the Tiptap JSON you give. " +
			"Nothing is published and nothing is sent: publish_blog_post makes the post public. " +
			"The post opens in the console's blog editor as if it was written there. " +
			"The slug must be unique among the posts that are not deleted, so a repeated call answers conflict. " +
			"reading_time_minutes is estimated from the body when you leave it out. All arguments together are limited to 1 MiB.",
		Class:       ClassWrite,
		Idempotent:  boolPtr(false),
		Pairs:       []Pair{blogWrite, writeTemplates},
		AuditAction: "blogPosts.create",
		InputSchema: map[string]any{
			"type":                 "object",
			"properties":           properties,
			"required":             []string{"workspace_id", "category_id", "slug", "title", "body"},
			"additionalProperties": false,
		},
		Handler: s.createBlogPost,
	}
}

func (s *Server) createBlogPost(ctx context.Context, call *Call) (any, error) {
	args, toolErr := decodeBlogPostWrite(call)
	if toolErr != nil {
		return nil, toolErr
	}
	title := valueOr(args.Title, "")
	call.Audit.SetTarget(domain.AuditTargetBlogPost, "", title)
	web, toolErr := renderBlogBody(args.Body)
	if toolErr != nil {
		return nil, toolErr
	}

	// The console names the body template from a UUID the same way.
	templateID := strings.ReplaceAll(uuid.NewString(), "-", "")
	fields := map[string]any{
		"category_id":          valueOr(args.CategoryID, ""),
		"slug":                 valueOr(args.Slug, ""),
		"title":                title,
		"template_id":          templateID,
		"template_version":     0,
		"reading_time_minutes": readingTimeMinutes(web.PlainText),
	}
	if args.ReadingTimeMinutes != nil {
		fields["reading_time_minutes"] = *args.ReadingTimeMinutes
	}
	if args.Excerpt != nil {
		fields["excerpt"] = *args.Excerpt
	}
	if args.FeaturedImageURL != nil {
		fields["featured_image_url"] = *args.FeaturedImageURL
	}
	if args.Authors != nil {
		fields["authors"] = *args.Authors
	}
	if args.SEO != nil {
		fields["seo"] = args.SEO
	}
	var req domain.CreateBlogPostRequest
	if err := requestFromJSON(fields, &req); err != nil {
		return nil, validationError("invalid arguments: %v", err)
	}
	if err := req.Validate(); err != nil {
		return nil, validationError("%v", err)
	}
	blogCtx := blogContext(ctx, call)
	if err := s.requireBlogCategory(blogCtx, req.CategoryID); err != nil {
		return nil, err
	}

	var templateReq domain.CreateTemplateRequest
	if err := requestFromJSON(map[string]any{
		"workspace_id": call.WorkspaceID,
		"id":           templateID,
		"name":         blogTemplateName(title),
		"channel":      domain.ChannelWeb,
		"category":     domain.TemplateCategoryBlog,
		"web":          web,
	}, &templateReq); err != nil {
		return nil, validationError("invalid arguments: %v", err)
	}
	template, _, err := templateReq.Validate()
	if err != nil {
		return nil, validationError("%v", err)
	}
	if err := s.deps.Templates.CreateTemplate(ctx, call.WorkspaceID, template); err != nil {
		return nil, err
	}
	call.Audit.AddMetadata("template_id", template.ID)

	post, err := s.deps.Blog.CreatePost(blogCtx, &req)
	if err != nil {
		// A post that was not created leaves no body behind, a taken slug
		// included.
		s.discardBlogTemplate(ctx, call, template.ID)
		return nil, err
	}
	call.Audit.SetTarget("", post.ID, "")
	out := projectBlogPost(post, time.Now())
	out.Body = web.Content
	version := template.Version
	out.BodyVersion = &version
	return getBlogPostOutput{Workspace: call.WorkspaceRef(), Post: out}, nil
}

// discardBlogTemplate deletes the body template of a post that could not be
// written, detached from the request so a cancelled call still cleans up.
func (s *Server) discardBlogTemplate(ctx context.Context, call *Call, templateID string) {
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := s.deps.Templates.DeleteTemplate(cleanup, call.WorkspaceID, templateID); err != nil {
		s.logError(call.Tool.Name, call.WorkspaceID, err)
	}
}

func (s *Server) updateBlogPostTool() *Tool {
	properties := blogPostProperties()
	properties["id"] = map[string]any{"type": "string", "description": "The post's ID (a UUID) from list_blog_posts."}
	properties["body"] = map[string]any{"type": "object", "description": "The whole new body, in the format create_blog_post describes. Leave it out to keep the body."}
	properties["base_version"] = map[string]any{"type": "integer", "description": "body_version from get_blog_post. With body, the update is refused with conflict when someone saved the body since."}
	properties["slug"] = map[string]any{"type": "string", "description": "New URL path: lowercase letters, digits and single hyphens. It changes the address of a published post."}
	properties["reading_time_minutes"] = map[string]any{"type": "integer", "description": "Reading time shown on the post, 1 to 600 minutes. A new body does not change it."}
	return &Tool{
		Name:  "update_blog_post",
		Title: "Update blog post",
		Description: "Changes the fields you give on a blog post and keeps the others. seo, when given, replaces the stored settings as a whole. " +
			"A published post shows the change at once, so confirm it with the user first. " +
			"With body, the body template saves a new version: a post that follows the latest version shows it, and a post pinned to a version moves to the new one, as in the console. " +
			"Changing the body also needs the templates write and read permissions. " +
			"The post is read then written, so a save made in the console in between can be lost. " +
			"Calling it again with the same arguments changes nothing more, but saves one more body version when body is given. " +
			"All arguments together are limited to 1 MiB.",
		Class:      ClassWrite,
		Idempotent: boolPtr(true),
		Pairs:      []Pair{blogWrite, blogRead},
		ArgumentPairs: func(args map[string]any) []Pair {
			if _, ok := args["body"]; ok {
				return []Pair{writeTemplates, readTemplates}
			}
			return nil
		},
		AuditAction: "blogPosts.update",
		InputSchema: map[string]any{
			"type":                 "object",
			"properties":           properties,
			"required":             []string{"workspace_id", "id"},
			"additionalProperties": false,
		},
		Handler: s.updateBlogPost,
	}
}

func (s *Server) updateBlogPost(ctx context.Context, call *Call) (any, error) {
	id, toolErr := blogPostArgument(call)
	if toolErr != nil {
		return nil, toolErr
	}
	args, toolErr := decodeBlogPostWrite(call)
	if toolErr != nil {
		return nil, toolErr
	}
	if args.CategoryID == nil && args.Slug == nil && args.Title == nil && args.Body == nil && args.Excerpt == nil &&
		args.FeaturedImageURL == nil && args.Authors == nil && args.ReadingTimeMinutes == nil && args.SEO == nil {
		return nil, validationError("give at least one field to change besides id and base_version")
	}
	if args.BaseVersion != nil && (args.Body == nil || *args.BaseVersion < 1) {
		return nil, validationError("base_version goes with body, and must be 1 or more")
	}
	var web *domain.WebTemplate
	if args.Body != nil {
		if web, toolErr = renderBlogBody(args.Body); toolErr != nil {
			return nil, toolErr
		}
	}

	// blogPosts.update replaces every field, so the post is read and written
	// back with the given fields changed.
	blogCtx := blogContext(ctx, call)
	post, err := s.deps.Blog.GetPost(blogCtx, id)
	if err != nil {
		return nil, err
	}
	if post == nil || post.DeletedAt != nil {
		return nil, &domain.ErrNotFound{Entity: "blog post", ID: id}
	}
	call.Audit.SetTarget("", "", post.Settings.Title)
	fields := map[string]any{
		"id":                   post.ID,
		"category_id":          post.CategoryID,
		"slug":                 post.Slug,
		"title":                post.Settings.Title,
		"template_id":          post.Settings.Template.TemplateID,
		"template_version":     post.Settings.Template.TemplateVersion,
		"excerpt":              post.Settings.Excerpt,
		"featured_image_url":   post.Settings.FeaturedImageURL,
		"authors":              post.Settings.Authors,
		"reading_time_minutes": post.Settings.ReadingTimeMinutes,
	}
	if post.Settings.SEO != nil {
		fields["seo"] = post.Settings.SEO
	}
	for key, value := range map[string]*string{
		"category_id": args.CategoryID, "slug": args.Slug, "title": args.Title, "excerpt": args.Excerpt, "featured_image_url": args.FeaturedImageURL,
	} {
		if value != nil {
			fields[key] = *value
		}
	}
	if args.Authors != nil {
		fields["authors"] = *args.Authors
	}
	if args.ReadingTimeMinutes != nil {
		fields["reading_time_minutes"] = *args.ReadingTimeMinutes
	}
	if args.SEO != nil {
		fields["seo"] = args.SEO
	}
	var req domain.UpdateBlogPostRequest
	if err := requestFromJSON(fields, &req); err != nil {
		return nil, validationError("invalid arguments: %v", err)
	}
	if err := req.Validate(); err != nil {
		return nil, validationError("%v", err)
	}
	// The refusals the service would give after the body is saved come first.
	if req.CategoryID != post.CategoryID {
		if err := s.requireBlogCategory(blogCtx, req.CategoryID); err != nil {
			return nil, err
		}
	}
	if req.Slug != post.Slug {
		taken, err := s.deps.Blog.GetPostBySlug(blogCtx, req.Slug)
		var notFound *domain.ErrNotFound
		switch {
		case errors.As(err, &notFound):
		case err != nil:
			return nil, err
		case taken != nil && taken.ID != post.ID:
			return nil, domain.NewAlreadyExistsError("blog_post", req.Slug, false)
		}
	}

	// The body is saved before the post because a pinned post moves to the
	// version the save creates, and only the save knows its number. A pinned
	// post keeps serving its old version until UpdatePost moves it, so a failed
	// UpdatePost leaves it as it was. A post that follows the latest version
	// serves the new body as soon as it is saved, so a failed UpdatePost saves
	// the previous body back over it.
	var bodyVersion *int64
	var previousBody *domain.Template
	if web != nil {
		version, stored, err := s.saveBlogBody(ctx, call, post, req.Title, web, args.BaseVersion)
		if err != nil {
			return nil, err
		}
		bodyVersion, previousBody = &version, stored
		call.Audit.AddMetadata("template_id", post.Settings.Template.TemplateID)
		call.Audit.AddMetadata("body_version", version)
		// As the console does: a post that follows the latest version keeps
		// following it, and a pinned one moves to the version just saved.
		if req.TemplateVersion != 0 {
			req.TemplateVersion = int(version)
		}
	}

	updated, err := s.deps.Blog.UpdatePost(blogCtx, &req)
	if err != nil {
		if previousBody != nil && req.TemplateVersion == 0 {
			restored, restoreErr := s.writeBlogBody(ctx, call, previousBody, previousBody.Name, previousBody.Web, bodyVersion)
			if restoreErr != nil {
				return nil, fmt.Errorf("%w (saving the previous body back also failed: %v)", err, restoreErr)
			}
			call.Audit.AddMetadata("body_restored_version", restored)
		}
		return nil, err
	}
	call.Audit.SetTarget("", "", updated.Settings.Title)
	out := projectBlogPost(updated, time.Now())
	if web != nil {
		out.Body = web.Content
		out.BodyVersion = bodyVersion
	} else if err := s.readBlogBody(ctx, call, updated, &out); err != nil {
		return nil, err
	}
	return getBlogPostOutput{Workspace: call.WorkspaceRef(), Post: out}, nil
}

// saveBlogBody writes a new version of a post's body template and returns its
// number, with the version it replaced. templates.update replaces the whole
// template, so the stored one is read and written back with the new body and
// the name the title gives.
func (s *Server) saveBlogBody(ctx context.Context, call *Call, post *domain.BlogPost, title string, web *domain.WebTemplate, baseVersion *int64) (int64, *domain.Template, error) {
	stored, toolErr, err := s.readTemplate(ctx, call.WorkspaceID, post.Settings.Template.TemplateID, nil)
	var notFound *domain.ErrTemplateNotFound
	switch {
	case toolErr != nil:
		return 0, nil, toolErr
	case errors.As(err, &notFound):
		return 0, nil, &ToolError{Code: CodeNotFound, Message: "the post's body template was not found: edit the post in the console"}
	case err != nil:
		return 0, nil, err
	}
	if stored.Channel != domain.ChannelWeb {
		return 0, nil, validationError("the post's body is a %s template, not a web one: edit the post in the console", stored.Channel)
	}
	version, err := s.writeBlogBody(ctx, call, stored, blogTemplateName(title), web, baseVersion)
	if err != nil {
		return 0, nil, err
	}
	return version, stored, nil
}

// writeBlogBody saves stored as a new version with the given name and body,
// keeping its other fields, and returns the new version's number.
func (s *Server) writeBlogBody(ctx context.Context, call *Call, stored *domain.Template, name string, web *domain.WebTemplate, baseVersion *int64) (int64, error) {
	fields := map[string]any{
		"workspace_id": call.WorkspaceID,
		"id":           stored.ID,
		"name":         name,
		"channel":      domain.ChannelWeb,
		"category":     stored.Category,
		"web":          web,
	}
	if stored.TemplateMacroID != nil {
		fields["template_macro_id"] = *stored.TemplateMacroID
	}
	if len(stored.TestData) > 0 {
		fields["test_data"] = map[string]any(stored.TestData)
	}
	if len(stored.Settings) > 0 {
		fields["settings"] = map[string]any(stored.Settings)
	}
	if baseVersion != nil {
		fields["base_version"] = *baseVersion
	}
	// Translations stay out of the request: the service keeps the stored ones.
	var req domain.UpdateTemplateRequest
	if err := requestFromJSON(fields, &req); err != nil {
		return 0, validationError("invalid arguments: %v", err)
	}
	template, _, err := req.Validate()
	if err != nil {
		return 0, validationError("%v", err)
	}
	if err := s.deps.Templates.UpdateTemplate(ctx, call.WorkspaceID, template); err != nil {
		return 0, err
	}
	return template.Version, nil
}

// blogPostStateOutput is what publish_blog_post and unpublish_blog_post
// return. Already is true when the post was in the target state before the
// call.
type blogPostStateOutput struct {
	Workspace   WorkspaceRef `json:"workspace"`
	PostID      string       `json:"post_id"`
	Status      string       `json:"status"`
	PublishedAt *time.Time   `json:"published_at"`
	Already     bool         `json:"already"`
}

func blogPostIDSchema(description string) map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"workspace_id": map[string]any{"type": "string", "description": "Workspace ID from list_workspaces."},
			"id":           map[string]any{"type": "string", "description": description},
		},
		"required":             []string{"workspace_id", "id"},
		"additionalProperties": false,
	}
}

func (s *Server) publishBlogPostTool() *Tool {
	schema := blogPostIDSchema("The draft's ID (a UUID) from list_blog_posts.")
	schema["properties"].(map[string]any)["published_at"] = map[string]any{
		"type": "string", "description": "RFC 3339 date-time. Leave it out to publish now. A later date schedules the post, and an earlier one backdates it.",
	}
	return &Tool{
		Name:  "publish_blog_post",
		Title: "Publish blog post",
		Description: "Publishes a draft on the public blog, now or at published_at. Nothing is emailed: the post appears on the blog and in its feed. " +
			"Confirm with the user first. status is published, or scheduled when published_at is still ahead. " +
			"A post already published or scheduled answers success with already set to true, its status, and published_at null, so calling it again is safe. " +
			"To change the date of a published post, unpublish it with unpublish_blog_post, then publish it again.",
		Class:       ClassWrite,
		Idempotent:  boolPtr(true),
		Pairs:       []Pair{blogWrite},
		AuditAction: "blogPosts.publish",
		InputSchema: schema,
		Handler:     s.publishBlogPost,
	}
}

func (s *Server) publishBlogPost(ctx context.Context, call *Call) (any, error) {
	id, toolErr := blogPostArgument(call)
	if toolErr != nil {
		return nil, toolErr
	}
	now := time.Now().UTC()
	at := now.Truncate(time.Second)
	if raw, _ := call.Args["published_at"].(string); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return nil, validationError("published_at must be an RFC 3339 date-time, such as 2026-12-01T09:00:00Z")
		}
		at = parsed.UTC()
	}
	var req domain.PublishBlogPostRequest
	if err := requestFromJSON(map[string]any{"id": id, "published_at": at}, &req); err != nil {
		return nil, validationError("invalid arguments: %v", err)
	}
	if err := req.Validate(); err != nil {
		return nil, validationError("%v", err)
	}
	err := s.deps.Blog.PublishPost(blogContext(ctx, call), &req)
	var invalid *domain.InvalidStateError
	switch {
	case errors.As(err, &invalid) && invalid.Status != string(domain.BlogPostStatusDraft):
		call.Audit.AddMetadata("already", true)
		return blogPostStateOutput{Workspace: call.WorkspaceRef(), PostID: id, Status: invalid.Status, Already: true}, nil
	case err != nil:
		return nil, err
	}
	call.Audit.AddMetadata("published_at", at.Format(time.RFC3339))
	status := domain.BlogPostStatusPublished
	if at.After(now) {
		status = domain.BlogPostStatusScheduled
	}
	return blogPostStateOutput{Workspace: call.WorkspaceRef(), PostID: id, Status: string(status), PublishedAt: &at}, nil
}

func (s *Server) unpublishBlogPostTool() *Tool {
	return &Tool{
		Name:  "unpublish_blog_post",
		Title: "Unpublish blog post",
		Description: "Takes a published or scheduled post back to draft: it leaves the public blog and its feed, and its publication date is cleared. " +
			"Confirm with the user first. A post already a draft answers success with already set to true, so calling it again is safe.",
		Class:       ClassWrite,
		Idempotent:  boolPtr(true),
		Pairs:       []Pair{blogWrite},
		AuditAction: "blogPosts.unpublish",
		InputSchema: blogPostIDSchema("The post's ID (a UUID) from list_blog_posts."),
		Handler:     s.unpublishBlogPost,
	}
}

func (s *Server) unpublishBlogPost(ctx context.Context, call *Call) (any, error) {
	id, toolErr := blogPostArgument(call)
	if toolErr != nil {
		return nil, toolErr
	}
	var req domain.UnpublishBlogPostRequest
	if err := requestFromJSON(map[string]any{"id": id}, &req); err != nil {
		return nil, validationError("invalid arguments: %v", err)
	}
	if err := req.Validate(); err != nil {
		return nil, validationError("%v", err)
	}
	out := blogPostStateOutput{Workspace: call.WorkspaceRef(), PostID: id, Status: string(domain.BlogPostStatusDraft)}
	err := s.deps.Blog.UnpublishPost(blogContext(ctx, call), &req)
	var invalid *domain.InvalidStateError
	switch {
	case errors.As(err, &invalid) && invalid.Status == string(domain.BlogPostStatusDraft):
		out.Already = true
		call.Audit.AddMetadata("already", true)
	case err != nil:
		return nil, err
	}
	return out, nil
}

func (s *Server) deleteBlogPostTool() *Tool {
	return &Tool{
		Name:  "delete_blog_post",
		Title: "Delete blog post",
		Description: "Deletes a blog post. A published post leaves the public blog at once, and another post can then use its slug. " +
			"The body template is kept. This cannot be undone, so confirm with the user first. " +
			"Returns not_found for a post that does not exist, which is also what a repeated call answers.",
		Class:       ClassDestructive,
		Pairs:       []Pair{blogWrite},
		AuditAction: "blogPosts.delete",
		InputSchema: blogPostIDSchema("The post's ID (a UUID) from list_blog_posts."),
		Handler:     s.deleteBlogPost,
	}
}

type deleteBlogPostOutput struct {
	Workspace WorkspaceRef `json:"workspace"`
	PostID    string       `json:"post_id"`
	Deleted   bool         `json:"deleted"`
}

func (s *Server) deleteBlogPost(ctx context.Context, call *Call) (any, error) {
	id, toolErr := blogPostArgument(call)
	if toolErr != nil {
		return nil, toolErr
	}
	var req domain.DeleteBlogPostRequest
	if err := requestFromJSON(map[string]any{"id": id}, &req); err != nil {
		return nil, validationError("invalid arguments: %v", err)
	}
	if err := req.Validate(); err != nil {
		return nil, validationError("%v", err)
	}
	// The service reads the post first and answers a typed not found for one
	// that is missing or deleted, so a repeated call answers not_found.
	if err := s.deps.Blog.DeletePost(blogContext(ctx, call), &req); err != nil {
		return nil, err
	}
	return deleteBlogPostOutput{Workspace: call.WorkspaceRef(), PostID: id, Deleted: true}, nil
}
