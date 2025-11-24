package domain

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

//go:generate mockgen -destination mocks/mock_blog_service.go -package mocks github.com/Notifuse/notifuse/internal/domain BlogService
//go:generate mockgen -destination mocks/mock_blog_category_repository.go -package mocks github.com/Notifuse/notifuse/internal/domain BlogCategoryRepository
//go:generate mockgen -destination mocks/mock_blog_post_repository.go -package mocks github.com/Notifuse/notifuse/internal/domain BlogPostRepository
//go:generate mockgen -destination mocks/mock_blog_theme_repository.go -package mocks github.com/Notifuse/notifuse/internal/domain BlogThemeRepository

// Regular expression for validating slugs (lowercase letters, numbers, and hyphens)
var slugRegex = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// SEOSettings contains web page SEO configuration (without slug)
// Reusable across workspace (homepage), blog categories, and blog posts
type SEOSettings struct {
	MetaTitle       string   `json:"meta_title,omitempty"`       // SEO meta title
	MetaDescription string   `json:"meta_description,omitempty"` // SEO meta description
	OGTitle         string   `json:"og_title,omitempty"`         // Open Graph title
	OGDescription   string   `json:"og_description,omitempty"`   // Open Graph description
	OGImage         string   `json:"og_image,omitempty"`         // Open Graph image URL
	CanonicalURL    string   `json:"canonical_url,omitempty"`    // Canonical URL
	Keywords        []string `json:"keywords,omitempty"`         // SEO keywords
	MetaRobots      string   `json:"meta_robots,omitempty"`      // Robots meta tag content (e.g., "index,follow", "noindex,nofollow")
}

// Value implements the driver.Valuer interface for database serialization
func (s SEOSettings) Value() (driver.Value, error) {
	return json.Marshal(s)
}

// Scan implements the sql.Scanner interface for database deserialization
func (s *SEOSettings) Scan(value interface{}) error {
	if value == nil {
		return nil
	}

	b, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("type assertion to []byte failed")
	}

	cloned := bytes.Clone(b)
	return json.Unmarshal(cloned, s)
}

// MergeWithDefaults merges the current SEO settings with defaults, preferring non-empty values
func (s *SEOSettings) MergeWithDefaults(defaults *SEOSettings) *SEOSettings {
	if s == nil && defaults == nil {
		return &SEOSettings{}
	}
	if s == nil {
		return defaults
	}
	if defaults == nil {
		return s
	}

	result := &SEOSettings{
		MetaTitle:       s.MetaTitle,
		MetaDescription: s.MetaDescription,
		OGTitle:         s.OGTitle,
		OGDescription:   s.OGDescription,
		OGImage:         s.OGImage,
		CanonicalURL:    s.CanonicalURL,
		Keywords:        s.Keywords,
		MetaRobots:      s.MetaRobots,
	}

	// Use defaults if current values are empty
	if result.MetaTitle == "" {
		result.MetaTitle = defaults.MetaTitle
	}
	if result.MetaDescription == "" {
		result.MetaDescription = defaults.MetaDescription
	}
	if result.OGTitle == "" {
		result.OGTitle = defaults.OGTitle
	}
	if result.OGDescription == "" {
		result.OGDescription = defaults.OGDescription
	}
	if result.OGImage == "" {
		result.OGImage = defaults.OGImage
	}
	if result.CanonicalURL == "" {
		result.CanonicalURL = defaults.CanonicalURL
	}
	if result.MetaRobots == "" {
		result.MetaRobots = defaults.MetaRobots
	}
	if len(result.Keywords) == 0 {
		result.Keywords = defaults.Keywords
	}

	return result
}

// BlogAuthor represents an author of a blog post
type BlogAuthor struct {
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url,omitempty"`
}

// BlogCategorySettings contains the settings for a blog category
type BlogCategorySettings struct {
	Name        string       `json:"name"`
	Description string       `json:"description,omitempty"`
	SEO         *SEOSettings `json:"seo,omitempty"` // SEO metadata
}

// Value implements the driver.Valuer interface for database serialization
func (s BlogCategorySettings) Value() (driver.Value, error) {
	return json.Marshal(s)
}

// Scan implements the sql.Scanner interface for database deserialization
func (s *BlogCategorySettings) Scan(value interface{}) error {
	if value == nil {
		return nil
	}

	b, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("type assertion to []byte failed")
	}

	cloned := bytes.Clone(b)
	return json.Unmarshal(cloned, s)
}

// BlogCategory represents a blog category
type BlogCategory struct {
	ID        string               `json:"id"`
	Slug      string               `json:"slug"` // URL identifier
	Settings  BlogCategorySettings `json:"settings"`
	CreatedAt time.Time            `json:"created_at"`
	UpdatedAt time.Time            `json:"updated_at"`
	DeletedAt *time.Time           `json:"deleted_at,omitempty"`
}

// Validate validates the blog category
func (c *BlogCategory) Validate() error {
	if c.ID == "" {
		return fmt.Errorf("id is required")
	}

	if c.Slug == "" {
		return fmt.Errorf("slug is required")
	}

	if !slugRegex.MatchString(c.Slug) {
		return fmt.Errorf("slug must contain only lowercase letters, numbers, and hyphens")
	}

	if len(c.Slug) > 100 {
		return fmt.Errorf("slug must be less than 100 characters")
	}

	if c.Settings.Name == "" {
		return fmt.Errorf("name is required")
	}

	if len(c.Settings.Name) > 255 {
		return fmt.Errorf("name must be less than 255 characters")
	}

	return nil
}

// BlogPostTemplateReference contains template information for a blog post
type BlogPostTemplateReference struct {
	TemplateID      string `json:"template_id"`
	TemplateVersion int    `json:"template_version"`
}

// BlogPostSettings contains the settings for a blog post
type BlogPostSettings struct {
	Title              string                    `json:"title"` // H1 displayed on page
	Template           BlogPostTemplateReference `json:"template"`
	Excerpt            string                    `json:"excerpt,omitempty"`
	FeaturedImageURL   string                    `json:"featured_image_url,omitempty"`
	Authors            []BlogAuthor              `json:"authors"`
	ReadingTimeMinutes int                       `json:"reading_time_minutes"`
	SEO                *SEOSettings              `json:"seo,omitempty"` // SEO metadata
}

// Value implements the driver.Valuer interface for database serialization
func (s BlogPostSettings) Value() (driver.Value, error) {
	return json.Marshal(s)
}

// Scan implements the sql.Scanner interface for database deserialization
func (s *BlogPostSettings) Scan(value interface{}) error {
	if value == nil {
		return nil
	}

	b, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("type assertion to []byte failed")
	}

	cloned := bytes.Clone(b)
	return json.Unmarshal(cloned, s)
}

// BlogPost represents a blog post
type BlogPost struct {
	ID          string           `json:"id"`
	CategoryID  string           `json:"category_id"`
	Slug        string           `json:"slug"` // URL identifier
	Settings    BlogPostSettings `json:"settings"`
	PublishedAt *time.Time       `json:"published_at,omitempty"` // null = draft
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
	DeletedAt   *time.Time       `json:"deleted_at,omitempty"`
}

// Validate validates the blog post
func (p *BlogPost) Validate() error {
	if p.ID == "" {
		return fmt.Errorf("id is required")
	}

	if p.CategoryID == "" {
		return fmt.Errorf("category_id is required")
	}

	if p.Slug == "" {
		return fmt.Errorf("slug is required")
	}

	if !slugRegex.MatchString(p.Slug) {
		return fmt.Errorf("slug must contain only lowercase letters, numbers, and hyphens")
	}

	if len(p.Slug) > 100 {
		return fmt.Errorf("slug must be less than 100 characters")
	}

	if p.Settings.Title == "" {
		return fmt.Errorf("title is required")
	}

	if len(p.Settings.Title) > 500 {
		return fmt.Errorf("title must be less than 500 characters")
	}

	if p.Settings.Template.TemplateID == "" {
		return fmt.Errorf("template_id is required")
	}

	return nil
}

// IsDraft returns true if the post is a draft
func (p *BlogPost) IsDraft() bool {
	return p.PublishedAt == nil
}

// IsPublished returns true if the post is published
func (p *BlogPost) IsPublished() bool {
	return p.PublishedAt != nil
}

// GetEffectiveSEOSettings merges the post's SEO settings with the category's defaults
func (p *BlogPost) GetEffectiveSEOSettings(category *BlogCategory) *SEOSettings {
	if category == nil {
		return p.Settings.SEO
	}

	if p.Settings.SEO == nil {
		return category.Settings.SEO
	}

	return p.Settings.SEO.MergeWithDefaults(category.Settings.SEO)
}

// TOCItem represents a single item in the table of contents
type TOCItem struct {
	ID    string `json:"id"`    // Anchor ID for linking (e.g., "introduction")
	Level int    `json:"level"` // Heading level (2-6)
	Text  string `json:"text"`  // Heading text content
}

// CreateBlogCategoryRequest defines the request to create a blog category
type CreateBlogCategoryRequest struct {
	Name        string       `json:"name"`
	Slug        string       `json:"slug"`
	Description string       `json:"description,omitempty"`
	SEO         *SEOSettings `json:"seo,omitempty"`
}

// Validate validates the create blog category request
func (r *CreateBlogCategoryRequest) Validate() error {

	if r.Name == "" {
		return fmt.Errorf("name is required")
	}

	if len(r.Name) > 255 {
		return fmt.Errorf("name must be less than 255 characters")
	}

	if r.Slug == "" {
		return fmt.Errorf("slug is required")
	}

	if !slugRegex.MatchString(r.Slug) {
		return fmt.Errorf("slug must contain only lowercase letters, numbers, and hyphens")
	}

	if len(r.Slug) > 100 {
		return fmt.Errorf("slug must be less than 100 characters")
	}

	return nil
}

// UpdateBlogCategoryRequest defines the request to update a blog category
type UpdateBlogCategoryRequest struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Slug        string       `json:"slug"`
	Description string       `json:"description,omitempty"`
	SEO         *SEOSettings `json:"seo,omitempty"`
}

// Validate validates the update blog category request
func (r *UpdateBlogCategoryRequest) Validate() error {

	if r.ID == "" {
		return fmt.Errorf("id is required")
	}

	if r.Name == "" {
		return fmt.Errorf("name is required")
	}

	if len(r.Name) > 255 {
		return fmt.Errorf("name must be less than 255 characters")
	}

	if r.Slug == "" {
		return fmt.Errorf("slug is required")
	}

	if !slugRegex.MatchString(r.Slug) {
		return fmt.Errorf("slug must contain only lowercase letters, numbers, and hyphens")
	}

	if len(r.Slug) > 100 {
		return fmt.Errorf("slug must be less than 100 characters")
	}

	return nil
}

// DeleteBlogCategoryRequest defines the request to delete a blog category
type DeleteBlogCategoryRequest struct {
	ID string `json:"id"`
}

// Validate validates the delete blog category request
func (r *DeleteBlogCategoryRequest) Validate() error {

	if r.ID == "" {
		return fmt.Errorf("id is required")
	}

	return nil
}

// GetBlogCategoryRequest defines the request to get a blog category
type GetBlogCategoryRequest struct {
	ID   string `json:"id,omitempty"`
	Slug string `json:"slug,omitempty"`
}

// Validate validates the get blog category request
func (r *GetBlogCategoryRequest) Validate() error {
	if r.ID == "" && r.Slug == "" {
		return fmt.Errorf("either id or slug is required")
	}

	return nil
}

// BlogCategoryListResponse defines the response for listing blog categories
type BlogCategoryListResponse struct {
	Categories []*BlogCategory `json:"categories"`
	TotalCount int             `json:"total_count"`
}

// CreateBlogPostRequest defines the request to create a blog post
type CreateBlogPostRequest struct {
	CategoryID         string       `json:"category_id"`
	Slug               string       `json:"slug"`
	Title              string       `json:"title"`
	TemplateID         string       `json:"template_id"`
	TemplateVersion    int          `json:"template_version"`
	Excerpt            string       `json:"excerpt,omitempty"`
	FeaturedImageURL   string       `json:"featured_image_url,omitempty"`
	Authors            []BlogAuthor `json:"authors"`
	ReadingTimeMinutes int          `json:"reading_time_minutes"`
	SEO                *SEOSettings `json:"seo,omitempty"`
}

// Validate validates the create blog post request
func (r *CreateBlogPostRequest) Validate() error {
	if r.CategoryID == "" {
		return fmt.Errorf("category_id is required")
	}

	if r.Slug == "" {
		return fmt.Errorf("slug is required")
	}

	if !slugRegex.MatchString(r.Slug) {
		return fmt.Errorf("slug must contain only lowercase letters, numbers, and hyphens")
	}

	if len(r.Slug) > 100 {
		return fmt.Errorf("slug must be less than 100 characters")
	}

	if r.Title == "" {
		return fmt.Errorf("title is required")
	}

	if len(r.Title) > 500 {
		return fmt.Errorf("title must be less than 500 characters")
	}

	if r.TemplateID == "" {
		return fmt.Errorf("template_id is required")
	}

	return nil
}

// UpdateBlogPostRequest defines the request to update a blog post
type UpdateBlogPostRequest struct {
	ID                 string       `json:"id"`
	CategoryID         string       `json:"category_id"`
	Slug               string       `json:"slug"`
	Title              string       `json:"title"`
	TemplateID         string       `json:"template_id"`
	TemplateVersion    int          `json:"template_version"`
	Excerpt            string       `json:"excerpt,omitempty"`
	FeaturedImageURL   string       `json:"featured_image_url,omitempty"`
	Authors            []BlogAuthor `json:"authors"`
	ReadingTimeMinutes int          `json:"reading_time_minutes"`
	SEO                *SEOSettings `json:"seo,omitempty"`
}

// Validate validates the update blog post request
func (r *UpdateBlogPostRequest) Validate() error {
	if r.ID == "" {
		return fmt.Errorf("id is required")
	}

	if r.CategoryID == "" {
		return fmt.Errorf("category_id is required")
	}

	if r.Slug == "" {
		return fmt.Errorf("slug is required")
	}

	if !slugRegex.MatchString(r.Slug) {
		return fmt.Errorf("slug must contain only lowercase letters, numbers, and hyphens")
	}

	if len(r.Slug) > 100 {
		return fmt.Errorf("slug must be less than 100 characters")
	}

	if r.Title == "" {
		return fmt.Errorf("title is required")
	}

	if len(r.Title) > 500 {
		return fmt.Errorf("title must be less than 500 characters")
	}

	if r.TemplateID == "" {
		return fmt.Errorf("template_id is required")
	}

	return nil
}

// DeleteBlogPostRequest defines the request to delete a blog post
type DeleteBlogPostRequest struct {
	ID string `json:"id"`
}

// Validate validates the delete blog post request
func (r *DeleteBlogPostRequest) Validate() error {

	if r.ID == "" {
		return fmt.Errorf("id is required")
	}

	return nil
}

// PublishBlogPostRequest defines the request to publish a blog post
type PublishBlogPostRequest struct {
	ID string `json:"id"`
}

// Validate validates the publish blog post request
func (r *PublishBlogPostRequest) Validate() error {

	if r.ID == "" {
		return fmt.Errorf("id is required")
	}

	return nil
}

// UnpublishBlogPostRequest defines the request to unpublish a blog post
type UnpublishBlogPostRequest struct {
	ID string `json:"id"`
}

// Validate validates the unpublish blog post request
func (r *UnpublishBlogPostRequest) Validate() error {

	if r.ID == "" {
		return fmt.Errorf("id is required")
	}

	return nil
}

// GetBlogPostRequest defines the request to get a blog post
type GetBlogPostRequest struct {
	ID           string `json:"id,omitempty"`
	Slug         string `json:"slug,omitempty"`
	CategorySlug string `json:"category_slug,omitempty"`
}

// Validate validates the get blog post request
func (r *GetBlogPostRequest) Validate() error {
	if r.ID == "" && r.Slug == "" {
		return fmt.Errorf("either id or slug is required")
	}

	return nil
}

// BlogPostStatus represents the status filter for listing posts
type BlogPostStatus string

const (
	BlogPostStatusAll       BlogPostStatus = "all"
	BlogPostStatusDraft     BlogPostStatus = "draft"
	BlogPostStatusPublished BlogPostStatus = "published"
)

// ListBlogPostsRequest defines the request to list blog posts
type ListBlogPostsRequest struct {
	CategoryID string         `json:"category_id,omitempty"`
	Status     BlogPostStatus `json:"status,omitempty"`
	Page       int            `json:"page,omitempty"`   // Page number (1-indexed)
	Limit      int            `json:"limit,omitempty"`  // Posts per page
	Offset     int            `json:"offset,omitempty"` // Calculated from Page
}

// Validate validates the list blog posts request
func (r *ListBlogPostsRequest) Validate() error {

	// Default to "all" if not specified
	if r.Status == "" {
		r.Status = BlogPostStatusAll
	}

	// Validate status
	switch r.Status {
	case BlogPostStatusAll, BlogPostStatusDraft, BlogPostStatusPublished:
		// Valid
	default:
		return fmt.Errorf("invalid status: %s", r.Status)
	}

	// Default page to 1 if not specified
	if r.Page <= 0 {
		r.Page = 1
	}

	// Default limit if not specified
	if r.Limit <= 0 {
		r.Limit = 50
	}

	// Max limit
	if r.Limit > 100 {
		r.Limit = 100
	}

	// Calculate offset from page number
	r.Offset = (r.Page - 1) * r.Limit

	return nil
}

// BlogPostListResponse defines the response for listing blog posts
type BlogPostListResponse struct {
	Posts           []*BlogPost `json:"posts"`
	TotalCount      int         `json:"total_count"`
	CurrentPage     int         `json:"current_page"`
	TotalPages      int         `json:"total_pages"`
	HasNextPage     bool        `json:"has_next_page"`
	HasPreviousPage bool        `json:"has_previous_page"`
}

// BlogCategoryRepository defines the data access layer for blog categories
type BlogCategoryRepository interface {
	CreateCategory(ctx context.Context, category *BlogCategory) error
	GetCategory(ctx context.Context, id string) (*BlogCategory, error)
	GetCategoryBySlug(ctx context.Context, slug string) (*BlogCategory, error)
	GetCategoriesByIDs(ctx context.Context, ids []string) ([]*BlogCategory, error)
	UpdateCategory(ctx context.Context, category *BlogCategory) error
	DeleteCategory(ctx context.Context, id string) error
	ListCategories(ctx context.Context) ([]*BlogCategory, error)

	// Transaction management
	WithTransaction(ctx context.Context, workspaceID string, fn func(*sql.Tx) error) error
	CreateCategoryTx(ctx context.Context, tx *sql.Tx, category *BlogCategory) error
	GetCategoryTx(ctx context.Context, tx *sql.Tx, id string) (*BlogCategory, error)
	GetCategoryBySlugTx(ctx context.Context, tx *sql.Tx, slug string) (*BlogCategory, error)
	UpdateCategoryTx(ctx context.Context, tx *sql.Tx, category *BlogCategory) error
	DeleteCategoryTx(ctx context.Context, tx *sql.Tx, id string) error
}

// BlogPostRepository defines the data access layer for blog posts
type BlogPostRepository interface {
	CreatePost(ctx context.Context, post *BlogPost) error
	GetPost(ctx context.Context, id string) (*BlogPost, error)
	GetPostBySlug(ctx context.Context, slug string) (*BlogPost, error)
	GetPostByCategoryAndSlug(ctx context.Context, categorySlug, postSlug string) (*BlogPost, error)
	UpdatePost(ctx context.Context, post *BlogPost) error
	DeletePost(ctx context.Context, id string) error
	ListPosts(ctx context.Context, params ListBlogPostsRequest) (*BlogPostListResponse, error)
	PublishPost(ctx context.Context, id string) error
	UnpublishPost(ctx context.Context, id string) error

	// Transaction management
	WithTransaction(ctx context.Context, workspaceID string, fn func(*sql.Tx) error) error
	CreatePostTx(ctx context.Context, tx *sql.Tx, post *BlogPost) error
	GetPostTx(ctx context.Context, tx *sql.Tx, id string) (*BlogPost, error)
	GetPostBySlugTx(ctx context.Context, tx *sql.Tx, slug string) (*BlogPost, error)
	UpdatePostTx(ctx context.Context, tx *sql.Tx, post *BlogPost) error
	DeletePostTx(ctx context.Context, tx *sql.Tx, id string) error
	DeletePostsByCategoryIDTx(ctx context.Context, tx *sql.Tx, categoryID string) (int64, error)
	PublishPostTx(ctx context.Context, tx *sql.Tx, id string) error
	UnpublishPostTx(ctx context.Context, tx *sql.Tx, id string) error
}

// BlogService defines the business logic layer for blog operations
type BlogService interface {
	// Category operations
	CreateCategory(ctx context.Context, request *CreateBlogCategoryRequest) (*BlogCategory, error)
	GetCategory(ctx context.Context, id string) (*BlogCategory, error)
	GetCategoryBySlug(ctx context.Context, slug string) (*BlogCategory, error)
	UpdateCategory(ctx context.Context, request *UpdateBlogCategoryRequest) (*BlogCategory, error)
	DeleteCategory(ctx context.Context, request *DeleteBlogCategoryRequest) error
	ListCategories(ctx context.Context) (*BlogCategoryListResponse, error)

	// Post operations
	CreatePost(ctx context.Context, request *CreateBlogPostRequest) (*BlogPost, error)
	GetPost(ctx context.Context, id string) (*BlogPost, error)
	GetPostBySlug(ctx context.Context, slug string) (*BlogPost, error)
	GetPostByCategoryAndSlug(ctx context.Context, categorySlug, postSlug string) (*BlogPost, error)
	UpdatePost(ctx context.Context, request *UpdateBlogPostRequest) (*BlogPost, error)
	DeletePost(ctx context.Context, request *DeleteBlogPostRequest) error
	ListPosts(ctx context.Context, params *ListBlogPostsRequest) (*BlogPostListResponse, error)
	PublishPost(ctx context.Context, request *PublishBlogPostRequest) error
	UnpublishPost(ctx context.Context, request *UnpublishBlogPostRequest) error

	// Public operations (no auth required)
	GetPublicCategoryBySlug(ctx context.Context, slug string) (*BlogCategory, error)
	GetPublicPostByCategoryAndSlug(ctx context.Context, categorySlug, postSlug string) (*BlogPost, error)
	ListPublicPosts(ctx context.Context, params *ListBlogPostsRequest) (*BlogPostListResponse, error)

	// Theme operations
	CreateTheme(ctx context.Context, request *CreateBlogThemeRequest) (*BlogTheme, error)
	GetTheme(ctx context.Context, version int) (*BlogTheme, error)
	GetPublishedTheme(ctx context.Context) (*BlogTheme, error)
	UpdateTheme(ctx context.Context, request *UpdateBlogThemeRequest) (*BlogTheme, error)
	PublishTheme(ctx context.Context, request *PublishBlogThemeRequest) error
	ListThemes(ctx context.Context, params *ListBlogThemesRequest) (*BlogThemeListResponse, error)

	// Blog page rendering (public, no auth	// Rendering
	RenderHomePage(ctx context.Context, workspaceID string, page int, themeVersion *int) (string, error)
	RenderPostPage(ctx context.Context, workspaceID, categorySlug, postSlug string, themeVersion *int) (string, error)
	RenderCategoryPage(ctx context.Context, workspaceID, categorySlug string, page int, themeVersion *int) (string, error)
}

// NormalizeSlug normalizes a string to be a valid slug
func NormalizeSlug(s string) string {
	s = strings.ToLower(s)
	s = strings.TrimSpace(s)
	// Replace spaces and underscores with hyphens
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, "_", "-")
	// Remove any characters that aren't lowercase letters, numbers, or hyphens
	var result strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			result.WriteRune(r)
		}
	}
	s = result.String()

	// Collapse consecutive hyphens into a single hyphen
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}

	// Remove leading and trailing hyphens
	s = strings.Trim(s, "-")

	return s
}

// BlogThemeFileType represents the type of blog theme file
type BlogThemeFileType string

const (
	BlogThemeFileTypeHome     BlogThemeFileType = "home"
	BlogThemeFileTypeCategory BlogThemeFileType = "category"
	BlogThemeFileTypePost     BlogThemeFileType = "post"
	BlogThemeFileTypeHeader   BlogThemeFileType = "header"
	BlogThemeFileTypeFooter   BlogThemeFileType = "footer"
	BlogThemeFileTypeShared   BlogThemeFileType = "shared"
)

// BlogThemeFiles contains Liquid template files and CSS for a blog theme
type BlogThemeFiles struct {
	HomeLiquid     string `json:"home.liquid"`
	CategoryLiquid string `json:"category.liquid"`
	PostLiquid     string `json:"post.liquid"`
	HeaderLiquid   string `json:"header.liquid"`
	FooterLiquid   string `json:"footer.liquid"`
	SharedLiquid   string `json:"shared.liquid"`
	StylesCSS      string `json:"styles.css"`
	ScriptsJS      string `json:"scripts.js"`
}

// Value implements the driver.Valuer interface for database serialization
func (f BlogThemeFiles) Value() (driver.Value, error) {
	return json.Marshal(f)
}

// Scan implements the sql.Scanner interface for database deserialization
func (f *BlogThemeFiles) Scan(value interface{}) error {
	if value == nil {
		return nil
	}

	b, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("type assertion to []byte failed")
	}

	cloned := bytes.Clone(b)
	return json.Unmarshal(cloned, f)
}

// BlogTheme represents a blog theme with versioned Liquid template files
type BlogTheme struct {
	Version           int            `json:"version"`
	PublishedAt       *time.Time     `json:"published_at,omitempty"`         // non-null = published
	PublishedByUserID *string        `json:"published_by_user_id,omitempty"` // user who published this theme
	Files             BlogThemeFiles `json:"files"`
	Notes             *string        `json:"notes,omitempty"` // optional notes/description for this version
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
}

// Validate validates the blog theme
func (t *BlogTheme) Validate() error {
	if t.Version <= 0 {
		return fmt.Errorf("version must be positive")
	}

	// Validate that all required files are present (can be empty strings but must exist)
	// Note: The JSON unmarshaling will ensure they exist, but we check them anyway
	return nil
}

// IsPublished returns true if the theme is published
func (t *BlogTheme) IsPublished() bool {
	return t.PublishedAt != nil
}

// CreateBlogThemeRequest defines the request to create a blog theme
type CreateBlogThemeRequest struct {
	Files BlogThemeFiles `json:"files"`
	Notes *string        `json:"notes,omitempty"`
}

// Validate validates the create blog theme request
func (r *CreateBlogThemeRequest) Validate() error {
	// Files can be empty strings, no validation needed
	return nil
}

// UpdateBlogThemeRequest defines the request to update a blog theme
type UpdateBlogThemeRequest struct {
	Version int            `json:"version"`
	Files   BlogThemeFiles `json:"files"`
	Notes   *string        `json:"notes,omitempty"`
}

// Validate validates the update blog theme request
func (r *UpdateBlogThemeRequest) Validate() error {
	if r.Version <= 0 {
		return fmt.Errorf("version must be positive")
	}
	return nil
}

// PublishBlogThemeRequest defines the request to publish a blog theme
type PublishBlogThemeRequest struct {
	Version int `json:"version"`
}

// Validate validates the publish blog theme request
func (r *PublishBlogThemeRequest) Validate() error {
	if r.Version <= 0 {
		return fmt.Errorf("version must be positive")
	}
	return nil
}

// GetBlogThemeRequest defines the request to get a blog theme
type GetBlogThemeRequest struct {
	Version int `json:"version"`
}

// Validate validates the get blog theme request
func (r *GetBlogThemeRequest) Validate() error {
	if r.Version <= 0 {
		return fmt.Errorf("version must be positive")
	}
	return nil
}

// ListBlogThemesRequest defines the request to list blog themes
type ListBlogThemesRequest struct {
	Limit  int `json:"limit,omitempty"`
	Offset int `json:"offset,omitempty"`
}

// Validate validates the list blog themes request
func (r *ListBlogThemesRequest) Validate() error {
	// Default limit if not specified
	if r.Limit <= 0 {
		r.Limit = 50
	}

	// Max limit
	if r.Limit > 100 {
		r.Limit = 100
	}

	if r.Offset < 0 {
		r.Offset = 0
	}

	return nil
}

// BlogThemeListResponse defines the response for listing blog themes
type BlogThemeListResponse struct {
	Themes     []*BlogTheme `json:"themes"`
	TotalCount int          `json:"total_count"`
}

// BlogThemeRepository defines the data access layer for blog themes
type BlogThemeRepository interface {
	CreateTheme(ctx context.Context, theme *BlogTheme) error
	GetTheme(ctx context.Context, version int) (*BlogTheme, error)
	GetPublishedTheme(ctx context.Context) (*BlogTheme, error)
	UpdateTheme(ctx context.Context, theme *BlogTheme) error
	PublishTheme(ctx context.Context, version int, publishedByUserID string) error
	ListThemes(ctx context.Context, params ListBlogThemesRequest) (*BlogThemeListResponse, error)

	// Transaction management
	WithTransaction(ctx context.Context, workspaceID string, fn func(*sql.Tx) error) error
	CreateThemeTx(ctx context.Context, tx *sql.Tx, theme *BlogTheme) error
	GetThemeTx(ctx context.Context, tx *sql.Tx, version int) (*BlogTheme, error)
	GetPublishedThemeTx(ctx context.Context, tx *sql.Tx) (*BlogTheme, error)
	UpdateThemeTx(ctx context.Context, tx *sql.Tx, theme *BlogTheme) error
	PublishThemeTx(ctx context.Context, tx *sql.Tx, version int, publishedByUserID string) error
}

// BlogRenderError represents a structured error for blog page rendering
type BlogRenderError struct {
	Code    string // Error code for handler mapping
	Message string // Human-readable message
	Details error  // Underlying error
}

func (e *BlogRenderError) Error() string {
	if e.Details != nil {
		return fmt.Sprintf("%s: %s (%v)", e.Code, e.Message, e.Details)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Error codes for blog rendering
const (
	ErrCodeThemeNotFound       = "theme_not_found"
	ErrCodeThemeNotPublished   = "theme_not_published"
	ErrCodePostNotFound        = "post_not_found"
	ErrCodeCategoryNotFound    = "category_not_found"
	ErrCodeRenderFailed        = "render_failed"
	ErrCodeInvalidLiquidSyntax = "invalid_liquid_syntax"
)

// BlogTemplateDataRequest groups parameters for building blog template data
// Similar to TemplateDataRequest for email templates
type BlogTemplateDataRequest struct {
	Workspace      *Workspace
	Post           *BlogPost             // Optional, for post pages
	Category       *BlogCategory         // Optional, for category pages
	PublicLists    []*List               // Always included (empty array if none)
	Posts          []*BlogPost           // For listings (home/category pages)
	Categories     []*BlogCategory       // For navigation
	ThemeVersion   int                   // Theme version number for cache-busting
	CustomData     MapOfAny              // Optional additional data
	PaginationData *BlogPostListResponse // Pagination metadata (optional, for paginated pages)
}

// BuildBlogTemplateData creates a template data map for blog Liquid templates
// This ensures consistent data structure across all blog pages
func BuildBlogTemplateData(req BlogTemplateDataRequest) (MapOfAny, error) {
	templateData := MapOfAny{}

	// Add workspace data
	if req.Workspace != nil {
		workspaceData := MapOfAny{
			"id":   req.Workspace.ID,
			"name": req.Workspace.Name,
		}

		// Add logo_url from blog settings or workspace settings
		// If blog settings has a logo (and it's not empty), use it
		// Otherwise fallback to workspace logo
		if req.Workspace.Settings.BlogSettings != nil &&
			req.Workspace.Settings.BlogSettings.LogoURL != nil &&
			*req.Workspace.Settings.BlogSettings.LogoURL != "" {
			workspaceData["logo_url"] = *req.Workspace.Settings.BlogSettings.LogoURL
		} else if req.Workspace.Settings.LogoURL != "" {
			workspaceData["logo_url"] = req.Workspace.Settings.LogoURL
		}

		// Add icon_url from blog settings
		if req.Workspace.Settings.BlogSettings != nil && req.Workspace.Settings.BlogSettings.IconURL != nil {
			workspaceData["icon_url"] = *req.Workspace.Settings.BlogSettings.IconURL
		}

		// Add blog_title from blog settings
		if req.Workspace.Settings.BlogSettings != nil && req.Workspace.Settings.BlogSettings.Title != "" {
			workspaceData["blog_title"] = req.Workspace.Settings.BlogSettings.Title
		}

		// Add SEO settings from blog settings
		if req.Workspace.Settings.BlogSettings != nil && req.Workspace.Settings.BlogSettings.SEO != nil {
			seoData := MapOfAny{}
			if req.Workspace.Settings.BlogSettings.SEO.MetaTitle != "" {
				seoData["meta_title"] = req.Workspace.Settings.BlogSettings.SEO.MetaTitle
			}
			if req.Workspace.Settings.BlogSettings.SEO.MetaDescription != "" {
				seoData["meta_description"] = req.Workspace.Settings.BlogSettings.SEO.MetaDescription
			}
			if req.Workspace.Settings.BlogSettings.SEO.OGTitle != "" {
				seoData["og_title"] = req.Workspace.Settings.BlogSettings.SEO.OGTitle
			}
			if req.Workspace.Settings.BlogSettings.SEO.OGDescription != "" {
				seoData["og_description"] = req.Workspace.Settings.BlogSettings.SEO.OGDescription
			}
			if req.Workspace.Settings.BlogSettings.SEO.OGImage != "" {
				seoData["og_image"] = req.Workspace.Settings.BlogSettings.SEO.OGImage
			}
			if req.Workspace.Settings.BlogSettings.SEO.CanonicalURL != "" {
				seoData["canonical_url"] = req.Workspace.Settings.BlogSettings.SEO.CanonicalURL
			}
			if len(req.Workspace.Settings.BlogSettings.SEO.Keywords) > 0 {
				seoData["keywords"] = req.Workspace.Settings.BlogSettings.SEO.Keywords
			}
			if req.Workspace.Settings.BlogSettings.SEO.MetaRobots != "" {
				seoData["meta_robots"] = req.Workspace.Settings.BlogSettings.SEO.MetaRobots
			}
			if len(seoData) > 0 {
				workspaceData["seo"] = seoData
			}
		}

		templateData["workspace"] = workspaceData
	}

	// Add base_url from workspace website URL or custom endpoint
	baseURL := ""
	if req.Workspace != nil {
		if req.Workspace.Settings.CustomEndpointURL != nil && *req.Workspace.Settings.CustomEndpointURL != "" {
			baseURL = *req.Workspace.Settings.CustomEndpointURL
		} else if req.Workspace.Settings.WebsiteURL != "" {
			baseURL = req.Workspace.Settings.WebsiteURL
		}
	}
	templateData["base_url"] = baseURL

	// Add post data (for post pages)
	if req.Post != nil {
		// Convert authors to maps for liquidgo compatibility
		authorsData := make([]map[string]interface{}, len(req.Post.Settings.Authors))
		for i, author := range req.Post.Settings.Authors {
			authorMap := map[string]interface{}{
				"name": author.Name,
			}
			if author.AvatarURL != "" {
				authorMap["avatar_url"] = author.AvatarURL
			}
			authorsData[i] = authorMap
		}

		postData := MapOfAny{
			"id":                   req.Post.ID,
			"slug":                 req.Post.Slug,
			"category_id":          req.Post.CategoryID,
			"published_at":         req.Post.PublishedAt,
			"created_at":           req.Post.CreatedAt,
			"updated_at":           req.Post.UpdatedAt,
			"title":                req.Post.Settings.Title,
			"excerpt":              req.Post.Settings.Excerpt,
			"featured_image_url":   req.Post.Settings.FeaturedImageURL,
			"authors":              authorsData,
			"reading_time_minutes": req.Post.Settings.ReadingTimeMinutes,
		}

		// Add SEO data if available
		if req.Post.Settings.SEO != nil {
			postData["seo"] = MapOfAny{
				"meta_title":       req.Post.Settings.SEO.MetaTitle,
				"meta_description": req.Post.Settings.SEO.MetaDescription,
				"og_title":         req.Post.Settings.SEO.OGTitle,
				"og_description":   req.Post.Settings.SEO.OGDescription,
				"og_image":         req.Post.Settings.SEO.OGImage,
				"canonical_url":    req.Post.Settings.SEO.CanonicalURL,
				"keywords":         req.Post.Settings.SEO.Keywords,
			}
		}

		templateData["post"] = postData
	}

	// Add category data (for category pages)
	if req.Category != nil {
		categoryData := MapOfAny{
			"id":          req.Category.ID,
			"slug":        req.Category.Slug,
			"name":        req.Category.Settings.Name,
			"description": req.Category.Settings.Description,
		}

		// Add SEO data if available
		if req.Category.Settings.SEO != nil {
			categoryData["seo"] = MapOfAny{
				"meta_title":       req.Category.Settings.SEO.MetaTitle,
				"meta_description": req.Category.Settings.SEO.MetaDescription,
				"og_title":         req.Category.Settings.SEO.OGTitle,
				"og_description":   req.Category.Settings.SEO.OGDescription,
				"og_image":         req.Category.Settings.SEO.OGImage,
				"canonical_url":    req.Category.Settings.SEO.CanonicalURL,
				"keywords":         req.Category.Settings.SEO.Keywords,
			}
		}

		templateData["category"] = categoryData
	}

	// Add public lists (always included, even if empty)
	publicListsData := make([]map[string]interface{}, 0)
	for _, list := range req.PublicLists {
		listData := map[string]interface{}{
			"id":   list.ID,
			"name": list.Name,
		}
		if list.Description != "" {
			listData["description"] = list.Description
		}
		publicListsData = append(publicListsData, listData)
	}
	templateData["public_lists"] = publicListsData

	// Add posts array (for listings)
	if req.Posts != nil {
		// Create a category lookup map (category_id -> category_slug)
		categorySlugMap := make(map[string]string)
		if req.Categories != nil {
			for _, cat := range req.Categories {
				categorySlugMap[cat.ID] = cat.Slug
			}
		}

		postsData := make([]map[string]interface{}, 0)
		for _, post := range req.Posts {
			// Convert authors to maps for liquidgo compatibility
			authorsData := make([]map[string]interface{}, len(post.Settings.Authors))
			for i, author := range post.Settings.Authors {
				authorMap := map[string]interface{}{
					"name": author.Name,
				}
				if author.AvatarURL != "" {
					authorMap["avatar_url"] = author.AvatarURL
				}
				authorsData[i] = authorMap
			}

			postData := map[string]interface{}{
				"id":                   post.ID,
				"slug":                 post.Slug,
				"category_id":          post.CategoryID,
				"published_at":         post.PublishedAt,
				"title":                post.Settings.Title,
				"excerpt":              post.Settings.Excerpt,
				"featured_image_url":   post.Settings.FeaturedImageURL,
				"authors":              authorsData,
				"reading_time_minutes": post.Settings.ReadingTimeMinutes,
			}

			// Add category_slug for URL construction if available
			if categorySlug, ok := categorySlugMap[post.CategoryID]; ok {
				postData["category_slug"] = categorySlug
			}

			postsData = append(postsData, postData)
		}
		templateData["posts"] = postsData
	}

	// Add categories array (for navigation) - only non-deleted categories
	if req.Categories != nil {
		categoriesData := make([]map[string]interface{}, 0)
		for _, category := range req.Categories {
			// Skip deleted categories for navigation
			if category.DeletedAt != nil {
				continue
			}
			categoryData := map[string]interface{}{
				"id":          category.ID,
				"slug":        category.Slug,
				"name":        category.Settings.Name,
				"description": category.Settings.Description,
			}
			categoriesData = append(categoriesData, categoryData)
		}
		templateData["categories"] = categoriesData
	}

	// Add custom data if provided
	if req.CustomData != nil {
		for key, value := range req.CustomData {
			templateData[key] = value
		}
	}

	// Add pagination data if provided (for paginated pages)
	if req.PaginationData != nil {
		paginationData := map[string]interface{}{
			"current_page": req.PaginationData.CurrentPage,
			"total_pages":  req.PaginationData.TotalPages,
			"has_next":     req.PaginationData.HasNextPage,
			"has_previous": req.PaginationData.HasPreviousPage,
			"total_count":  req.PaginationData.TotalCount,
			"per_page":     0, // Will be set by caller if needed
		}
		templateData["pagination"] = paginationData
	}

	// Add current year for copyright notices, etc.
	templateData["current_year"] = time.Now().Year()

	// Add theme version for cache-busting
	templateData["theme"] = map[string]interface{}{
		"version": req.ThemeVersion,
	}

	return templateData, nil
}
