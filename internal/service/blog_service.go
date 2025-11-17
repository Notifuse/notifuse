package service

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/liquid"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/google/uuid"
)

// BlogService handles all blog-related operations
type BlogService struct {
	logger        logger.Logger
	categoryRepo  domain.BlogCategoryRepository
	postRepo      domain.BlogPostRepository
	themeRepo     domain.BlogThemeRepository
	workspaceRepo domain.WorkspaceRepository
	listRepo      domain.ListRepository
	authService   domain.AuthService
}

// NewBlogService creates a new blog service
func NewBlogService(
	logger logger.Logger,
	categoryRepository domain.BlogCategoryRepository,
	postRepository domain.BlogPostRepository,
	themeRepository domain.BlogThemeRepository,
	workspaceRepository domain.WorkspaceRepository,
	listRepository domain.ListRepository,
	authService domain.AuthService,
) *BlogService {
	return &BlogService{
		logger:        logger,
		categoryRepo:  categoryRepository,
		postRepo:      postRepository,
		themeRepo:     themeRepository,
		workspaceRepo: workspaceRepository,
		listRepo:      listRepository,
		authService:   authService,
	}
}

// ====================
// Category Operations
// ====================

// CreateCategory creates a new blog category
func (s *BlogService) CreateCategory(ctx context.Context, request *domain.CreateBlogCategoryRequest) (*domain.BlogCategory, error) {
	// Get workspace ID from context
	workspaceID, ok := ctx.Value("workspace_id").(string)
	if !ok {
		return nil, fmt.Errorf("workspace_id not found in context")
	}

	// Authenticate user for workspace
	var err error
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Check permission for writing blog
	if !userWorkspace.HasPermission(domain.PermissionResourceBlog, domain.PermissionTypeWrite) {
		return nil, domain.NewPermissionError(
			domain.PermissionResourceBlog,
			domain.PermissionTypeWrite,
			"Insufficient permissions: write access to blog required",
		)
	}

	// Validate the request
	if err := request.Validate(); err != nil {
		s.logger.Error("Failed to validate category creation request")
		return nil, err
	}

	// Check if slug already exists
	existing, err := s.categoryRepo.GetCategoryBySlug(ctx, request.Slug)
	if err == nil && existing != nil {
		return nil, fmt.Errorf("category with slug '%s' already exists", request.Slug)
	}

	// Generate a unique ID
	id := uuid.New().String()

	// Create the category
	category := &domain.BlogCategory{
		ID:   id,
		Slug: request.Slug,
		Settings: domain.BlogCategorySettings{
			Name:        request.Name,
			Description: request.Description,
			SEO:         request.SEO,
		},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	// Validate the category
	if err := category.Validate(); err != nil {
		s.logger.Error("Failed to validate category")
		return nil, err
	}

	// Persist the category
	if err := s.categoryRepo.CreateCategory(ctx, category); err != nil {
		s.logger.Error("Failed to create category")
		return nil, fmt.Errorf("failed to create category: %w", err)
	}

	return category, nil
}

// GetCategory retrieves a blog category by ID
func (s *BlogService) GetCategory(ctx context.Context, id string) (*domain.BlogCategory, error) {
	// Get workspace ID from context
	workspaceID, ok := ctx.Value("workspace_id").(string)
	if !ok {
		return nil, fmt.Errorf("workspace_id not found in context")
	}

	// Authenticate user for workspace
	var err error
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Check permission for reading blog
	if !userWorkspace.HasPermission(domain.PermissionResourceBlog, domain.PermissionTypeRead) {
		return nil, domain.NewPermissionError(
			domain.PermissionResourceBlog,
			domain.PermissionTypeRead,
			"Insufficient permissions: read access to blog required",
		)
	}

	return s.categoryRepo.GetCategory(ctx, id)
}

// GetCategoryBySlug retrieves a blog category by slug
func (s *BlogService) GetCategoryBySlug(ctx context.Context, slug string) (*domain.BlogCategory, error) {
	// Get workspace ID from context
	workspaceID, ok := ctx.Value("workspace_id").(string)
	if !ok {
		return nil, fmt.Errorf("workspace_id not found in context")
	}

	// Authenticate user for workspace
	var err error
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Check permission for reading blog
	if !userWorkspace.HasPermission(domain.PermissionResourceBlog, domain.PermissionTypeRead) {
		return nil, domain.NewPermissionError(
			domain.PermissionResourceBlog,
			domain.PermissionTypeRead,
			"Insufficient permissions: read access to blog required",
		)
	}

	return s.categoryRepo.GetCategoryBySlug(ctx, slug)
}

// UpdateCategory updates an existing blog category
func (s *BlogService) UpdateCategory(ctx context.Context, request *domain.UpdateBlogCategoryRequest) (*domain.BlogCategory, error) {
	// Get workspace ID from context
	workspaceID, ok := ctx.Value("workspace_id").(string)
	if !ok {
		return nil, fmt.Errorf("workspace_id not found in context")
	}

	// Authenticate user for workspace
	var err error
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Check permission for writing blog
	if !userWorkspace.HasPermission(domain.PermissionResourceBlog, domain.PermissionTypeWrite) {
		return nil, domain.NewPermissionError(
			domain.PermissionResourceBlog,
			domain.PermissionTypeWrite,
			"Insufficient permissions: write access to blog required",
		)
	}

	// Validate the request
	if err := request.Validate(); err != nil {
		s.logger.Error("Failed to validate category update request")
		return nil, err
	}

	// Get the existing category
	category, err := s.categoryRepo.GetCategory(ctx, request.ID)
	if err != nil {
		s.logger.Error("Failed to get existing category")
		return nil, fmt.Errorf("category not found: %w", err)
	}

	// Check if slug is changing and if new slug already exists
	if category.Slug != request.Slug {
		existing, err := s.categoryRepo.GetCategoryBySlug(ctx, request.Slug)
		if err == nil && existing != nil && existing.ID != request.ID {
			return nil, fmt.Errorf("category with slug '%s' already exists", request.Slug)
		}
	}

	// Update the category fields
	category.Slug = request.Slug
	category.Settings.Name = request.Name
	category.Settings.Description = request.Description
	category.Settings.SEO = request.SEO
	category.UpdatedAt = time.Now().UTC()

	// Validate the updated category
	if err := category.Validate(); err != nil {
		s.logger.Error("Failed to validate updated category")
		return nil, err
	}

	// Persist the changes
	if err := s.categoryRepo.UpdateCategory(ctx, category); err != nil {
		s.logger.Error("Failed to update category")
		return nil, fmt.Errorf("failed to update category: %w", err)
	}

	return category, nil
}

// DeleteCategory deletes a blog category and cascade deletes all posts in that category
func (s *BlogService) DeleteCategory(ctx context.Context, request *domain.DeleteBlogCategoryRequest) error {
	// Get workspace ID from context
	workspaceID, ok := ctx.Value("workspace_id").(string)
	if !ok {
		return fmt.Errorf("workspace_id not found in context")
	}

	// Authenticate user for workspace
	var err error
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Check permission for writing blog
	if !userWorkspace.HasPermission(domain.PermissionResourceBlog, domain.PermissionTypeWrite) {
		return domain.NewPermissionError(
			domain.PermissionResourceBlog,
			domain.PermissionTypeWrite,
			"Insufficient permissions: write access to blog required",
		)
	}

	// Validate the request
	if err := request.Validate(); err != nil {
		s.logger.Error("Failed to validate category deletion request")
		return err
	}

	// Use a transaction to cascade delete all posts and the category atomically
	err = s.categoryRepo.WithTransaction(ctx, workspaceID, func(tx *sql.Tx) error {
		// First, soft-delete all posts belonging to this category
		postsDeleted, err := s.postRepo.DeletePostsByCategoryIDTx(ctx, tx, request.ID)
		if err != nil {
			s.logger.Error("Failed to cascade delete posts for category")
			return fmt.Errorf("failed to delete posts: %w", err)
		}

		// Log the cascade operation
		if postsDeleted > 0 {
			s.logger.Info(fmt.Sprintf("Cascade deleted %d posts from category %s", postsDeleted, request.ID))
		}

		// Then, soft-delete the category itself
		if err := s.categoryRepo.DeleteCategoryTx(ctx, tx, request.ID); err != nil {
			s.logger.Error("Failed to delete category")
			return fmt.Errorf("failed to delete category: %w", err)
		}

		return nil
	})

	if err != nil {
		return err
	}

	return nil
}

// ListCategories retrieves all blog categories for a workspace
func (s *BlogService) ListCategories(ctx context.Context) (*domain.BlogCategoryListResponse, error) {
	// Get workspace ID from context
	workspaceID, ok := ctx.Value("workspace_id").(string)
	if !ok {
		return nil, fmt.Errorf("workspace_id not found in context")
	}

	// Authenticate user for workspace
	var err error
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Check permission for reading blog
	if !userWorkspace.HasPermission(domain.PermissionResourceBlog, domain.PermissionTypeRead) {
		return nil, domain.NewPermissionError(
			domain.PermissionResourceBlog,
			domain.PermissionTypeRead,
			"Insufficient permissions: read access to blog required",
		)
	}

	categories, err := s.categoryRepo.ListCategories(ctx)
	if err != nil {
		s.logger.Error("Failed to list categories")
		return nil, fmt.Errorf("failed to list categories: %w", err)
	}

	return &domain.BlogCategoryListResponse{
		Categories: categories,
		TotalCount: len(categories),
	}, nil
}

// ====================
// Post Operations
// ====================

// CreatePost creates a new blog post
func (s *BlogService) CreatePost(ctx context.Context, request *domain.CreateBlogPostRequest) (*domain.BlogPost, error) {
	// Get workspace ID from context
	workspaceID, ok := ctx.Value("workspace_id").(string)
	if !ok {
		return nil, fmt.Errorf("workspace_id not found in context")
	}

	// Authenticate user for workspace
	var err error
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Check permission for writing blog
	if !userWorkspace.HasPermission(domain.PermissionResourceBlog, domain.PermissionTypeWrite) {
		return nil, domain.NewPermissionError(
			domain.PermissionResourceBlog,
			domain.PermissionTypeWrite,
			"Insufficient permissions: write access to blog required",
		)
	}

	// Validate the request
	if err := request.Validate(); err != nil {
		s.logger.Error("Failed to validate post creation request")
		return nil, err
	}

	// Check if slug already exists
	existing, err := s.postRepo.GetPostBySlug(ctx, request.Slug)
	if err == nil && existing != nil {
		return nil, fmt.Errorf("post with slug '%s' already exists", request.Slug)
	}

	// Verify category exists
	_, err = s.categoryRepo.GetCategory(ctx, request.CategoryID)
	if err != nil {
		return nil, fmt.Errorf("category not found: %w", err)
	}

	// Generate a unique ID
	id := uuid.New().String()

	// Create the post
	post := &domain.BlogPost{
		ID:         id,
		CategoryID: request.CategoryID,
		Slug:       request.Slug,
		Settings: domain.BlogPostSettings{
			Title: request.Title,
			Template: domain.BlogPostTemplateReference{
				TemplateID:      request.TemplateID,
				TemplateVersion: request.TemplateVersion,
			},
			Excerpt:            request.Excerpt,
			FeaturedImageURL:   request.FeaturedImageURL,
			Authors:            request.Authors,
			ReadingTimeMinutes: request.ReadingTimeMinutes,
			SEO:                request.SEO,
		},
		PublishedAt: nil, // Draft by default
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}

	// Validate the post
	if err := post.Validate(); err != nil {
		s.logger.Error("Failed to validate post")
		return nil, err
	}

	// Persist the post
	if err := s.postRepo.CreatePost(ctx, post); err != nil {
		s.logger.Error("Failed to create post")
		return nil, fmt.Errorf("failed to create post: %w", err)
	}

	return post, nil
}

// GetPost retrieves a blog post by ID
func (s *BlogService) GetPost(ctx context.Context, id string) (*domain.BlogPost, error) {
	// Get workspace ID from context
	workspaceID, ok := ctx.Value("workspace_id").(string)
	if !ok {
		return nil, fmt.Errorf("workspace_id not found in context")
	}

	// Authenticate user for workspace
	var err error
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Check permission for reading blog
	if !userWorkspace.HasPermission(domain.PermissionResourceBlog, domain.PermissionTypeRead) {
		return nil, domain.NewPermissionError(
			domain.PermissionResourceBlog,
			domain.PermissionTypeRead,
			"Insufficient permissions: read access to blog required",
		)
	}

	return s.postRepo.GetPost(ctx, id)
}

// GetPostBySlug retrieves a blog post by slug
func (s *BlogService) GetPostBySlug(ctx context.Context, slug string) (*domain.BlogPost, error) {
	// Get workspace ID from context
	workspaceID, ok := ctx.Value("workspace_id").(string)
	if !ok {
		return nil, fmt.Errorf("workspace_id not found in context")
	}

	// Authenticate user for workspace
	var err error
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Check permission for reading blog
	if !userWorkspace.HasPermission(domain.PermissionResourceBlog, domain.PermissionTypeRead) {
		return nil, domain.NewPermissionError(
			domain.PermissionResourceBlog,
			domain.PermissionTypeRead,
			"Insufficient permissions: read access to blog required",
		)
	}

	return s.postRepo.GetPostBySlug(ctx, slug)
}

// GetPostByCategoryAndSlug retrieves a blog post by category slug and post slug
func (s *BlogService) GetPostByCategoryAndSlug(ctx context.Context, categorySlug, postSlug string) (*domain.BlogPost, error) {
	// Get workspace ID from context
	workspaceID, ok := ctx.Value("workspace_id").(string)
	if !ok {
		return nil, fmt.Errorf("workspace_id not found in context")
	}

	// Authenticate user for workspace
	var err error
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Check permission for reading blog
	if !userWorkspace.HasPermission(domain.PermissionResourceBlog, domain.PermissionTypeRead) {
		return nil, domain.NewPermissionError(
			domain.PermissionResourceBlog,
			domain.PermissionTypeRead,
			"Insufficient permissions: read access to blog required",
		)
	}

	return s.postRepo.GetPostByCategoryAndSlug(ctx, categorySlug, postSlug)
}

// UpdatePost updates an existing blog post
func (s *BlogService) UpdatePost(ctx context.Context, request *domain.UpdateBlogPostRequest) (*domain.BlogPost, error) {
	// Get workspace ID from context
	workspaceID, ok := ctx.Value("workspace_id").(string)
	if !ok {
		return nil, fmt.Errorf("workspace_id not found in context")
	}

	// Authenticate user for workspace
	var err error
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Check permission for writing blog
	if !userWorkspace.HasPermission(domain.PermissionResourceBlog, domain.PermissionTypeWrite) {
		return nil, domain.NewPermissionError(
			domain.PermissionResourceBlog,
			domain.PermissionTypeWrite,
			"Insufficient permissions: write access to blog required",
		)
	}

	// Validate the request
	if err := request.Validate(); err != nil {
		s.logger.Error("Failed to validate post update request")
		return nil, err
	}

	// Get the existing post
	post, err := s.postRepo.GetPost(ctx, request.ID)
	if err != nil {
		s.logger.Error("Failed to get existing post")
		return nil, fmt.Errorf("post not found: %w", err)
	}

	// Check if slug is changing and if new slug already exists
	if post.Slug != request.Slug {
		existing, err := s.postRepo.GetPostBySlug(ctx, request.Slug)
		if err == nil && existing != nil && existing.ID != request.ID {
			return nil, fmt.Errorf("post with slug '%s' already exists", request.Slug)
		}
	}

	// Verify category exists
	_, err = s.categoryRepo.GetCategory(ctx, request.CategoryID)
	if err != nil {
		return nil, fmt.Errorf("category not found: %w", err)
	}

	// Update the post fields
	post.CategoryID = request.CategoryID
	post.Slug = request.Slug
	post.Settings.Title = request.Title
	post.Settings.Template.TemplateID = request.TemplateID
	post.Settings.Template.TemplateVersion = request.TemplateVersion
	post.Settings.Excerpt = request.Excerpt
	post.Settings.FeaturedImageURL = request.FeaturedImageURL
	post.Settings.Authors = request.Authors
	post.Settings.ReadingTimeMinutes = request.ReadingTimeMinutes
	post.Settings.SEO = request.SEO
	post.UpdatedAt = time.Now().UTC()

	// Validate the updated post
	if err := post.Validate(); err != nil {
		s.logger.Error("Failed to validate updated post")
		return nil, err
	}

	// Persist the changes
	if err := s.postRepo.UpdatePost(ctx, post); err != nil {
		s.logger.Error("Failed to update post")
		return nil, fmt.Errorf("failed to update post: %w", err)
	}

	return post, nil
}

// DeletePost deletes a blog post
func (s *BlogService) DeletePost(ctx context.Context, request *domain.DeleteBlogPostRequest) error {
	// Get workspace ID from context
	workspaceID, ok := ctx.Value("workspace_id").(string)
	if !ok {
		return fmt.Errorf("workspace_id not found in context")
	}

	// Authenticate user for workspace
	var err error
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Check permission for writing blog
	if !userWorkspace.HasPermission(domain.PermissionResourceBlog, domain.PermissionTypeWrite) {
		return domain.NewPermissionError(
			domain.PermissionResourceBlog,
			domain.PermissionTypeWrite,
			"Insufficient permissions: write access to blog required",
		)
	}

	// Validate the request
	if err := request.Validate(); err != nil {
		s.logger.Error("Failed to validate post deletion request")
		return err
	}

	// Delete the post
	if err := s.postRepo.DeletePost(ctx, request.ID); err != nil {
		s.logger.Error("Failed to delete post")
		return fmt.Errorf("failed to delete post: %w", err)
	}

	return nil
}

// ListPosts retrieves blog posts with filtering and pagination
func (s *BlogService) ListPosts(ctx context.Context, params *domain.ListBlogPostsRequest) (*domain.BlogPostListResponse, error) {
	// Get workspace ID from context
	workspaceID, ok := ctx.Value("workspace_id").(string)
	if !ok {
		return nil, fmt.Errorf("workspace_id not found in context")
	}

	// Authenticate user for workspace
	var err error
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Check permission for reading blog
	if !userWorkspace.HasPermission(domain.PermissionResourceBlog, domain.PermissionTypeRead) {
		return nil, domain.NewPermissionError(
			domain.PermissionResourceBlog,
			domain.PermissionTypeRead,
			"Insufficient permissions: read access to blog required",
		)
	}

	// Validate the request
	if err := params.Validate(); err != nil {
		s.logger.Error("Failed to validate post list request")
		return nil, err
	}

	return s.postRepo.ListPosts(ctx, *params)
}

// PublishPost publishes a draft blog post
func (s *BlogService) PublishPost(ctx context.Context, request *domain.PublishBlogPostRequest) error {
	// Get workspace ID from context
	workspaceID, ok := ctx.Value("workspace_id").(string)
	if !ok {
		return fmt.Errorf("workspace_id not found in context")
	}

	// Authenticate user for workspace
	var err error
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Check permission for writing blog
	if !userWorkspace.HasPermission(domain.PermissionResourceBlog, domain.PermissionTypeWrite) {
		return domain.NewPermissionError(
			domain.PermissionResourceBlog,
			domain.PermissionTypeWrite,
			"Insufficient permissions: write access to blog required",
		)
	}

	// Validate the request
	if err := request.Validate(); err != nil {
		s.logger.Error("Failed to validate post publish request")
		return err
	}

	// Publish the post
	if err := s.postRepo.PublishPost(ctx, request.ID); err != nil {
		s.logger.Error("Failed to publish post")
		return fmt.Errorf("failed to publish post: %w", err)
	}

	return nil
}

// UnpublishPost unpublishes a published blog post
func (s *BlogService) UnpublishPost(ctx context.Context, request *domain.UnpublishBlogPostRequest) error {
	// Get workspace ID from context
	workspaceID, ok := ctx.Value("workspace_id").(string)
	if !ok {
		return fmt.Errorf("workspace_id not found in context")
	}

	// Authenticate user for workspace
	var err error
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Check permission for writing blog
	if !userWorkspace.HasPermission(domain.PermissionResourceBlog, domain.PermissionTypeWrite) {
		return domain.NewPermissionError(
			domain.PermissionResourceBlog,
			domain.PermissionTypeWrite,
			"Insufficient permissions: write access to blog required",
		)
	}

	// Validate the request
	if err := request.Validate(); err != nil {
		s.logger.Error("Failed to validate post unpublish request")
		return err
	}

	// Unpublish the post
	if err := s.postRepo.UnpublishPost(ctx, request.ID); err != nil {
		s.logger.Error("Failed to unpublish post")
		return fmt.Errorf("failed to unpublish post: %w", err)
	}

	return nil
}

// GetPublicPostByCategoryAndSlug retrieves a published blog post by category slug and post slug (no auth required)
func (s *BlogService) GetPublicPostByCategoryAndSlug(ctx context.Context, categorySlug, postSlug string) (*domain.BlogPost, error) {
	post, err := s.postRepo.GetPostByCategoryAndSlug(ctx, categorySlug, postSlug)
	if err != nil {
		return nil, err
	}

	// Only return published posts
	if !post.IsPublished() {
		return nil, fmt.Errorf("post not found")
	}

	return post, nil
}

// ListPublicPosts retrieves published blog posts (no auth required)
func (s *BlogService) ListPublicPosts(ctx context.Context, params *domain.ListBlogPostsRequest) (*domain.BlogPostListResponse, error) {
	// Force status to published
	params.Status = domain.BlogPostStatusPublished

	// Validate the request
	if err := params.Validate(); err != nil {
		return nil, err
	}

	return s.postRepo.ListPosts(ctx, *params)
}

// ====================
// Theme Operations
// ====================

// CreateTheme creates a new blog theme
func (s *BlogService) CreateTheme(ctx context.Context, request *domain.CreateBlogThemeRequest) (*domain.BlogTheme, error) {
	// Get workspace ID from context
	workspaceID, ok := ctx.Value("workspace_id").(string)
	if !ok {
		return nil, fmt.Errorf("workspace_id not found in context")
	}

	// Authenticate user for workspace
	var err error
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Check permission for writing blog
	if !userWorkspace.HasPermission(domain.PermissionResourceBlog, domain.PermissionTypeWrite) {
		return nil, domain.NewPermissionError(
			domain.PermissionResourceBlog,
			domain.PermissionTypeWrite,
			"Insufficient permissions: write access to blog required",
		)
	}

	// Validate the request
	if err := request.Validate(); err != nil {
		s.logger.Error("Failed to validate theme creation request")
		return nil, err
	}

	// Create the theme (version will be auto-generated by the repository)
	theme := &domain.BlogTheme{
		PublishedAt: nil, // Unpublished by default
		Files:       request.Files,
		Notes:       request.Notes,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}

	// Persist the theme (repository will assign version)
	if err := s.themeRepo.CreateTheme(ctx, theme); err != nil {
		s.logger.Error("Failed to create theme")
		return nil, fmt.Errorf("failed to create theme: %w", err)
	}

	return theme, nil
}

// GetTheme retrieves a blog theme by version
func (s *BlogService) GetTheme(ctx context.Context, version int) (*domain.BlogTheme, error) {
	// Get workspace ID from context
	workspaceID, ok := ctx.Value("workspace_id").(string)
	if !ok {
		return nil, fmt.Errorf("workspace_id not found in context")
	}

	// Authenticate user for workspace
	var err error
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Check permission for reading blog
	if !userWorkspace.HasPermission(domain.PermissionResourceBlog, domain.PermissionTypeRead) {
		return nil, domain.NewPermissionError(
			domain.PermissionResourceBlog,
			domain.PermissionTypeRead,
			"Insufficient permissions: read access to blog required",
		)
	}

	return s.themeRepo.GetTheme(ctx, version)
}

// GetPublishedTheme retrieves the currently published blog theme
func (s *BlogService) GetPublishedTheme(ctx context.Context) (*domain.BlogTheme, error) {
	// Get workspace ID from context
	workspaceID, ok := ctx.Value("workspace_id").(string)
	if !ok {
		return nil, fmt.Errorf("workspace_id not found in context")
	}

	// Authenticate user for workspace
	var err error
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Check permission for reading blog
	if !userWorkspace.HasPermission(domain.PermissionResourceBlog, domain.PermissionTypeRead) {
		return nil, domain.NewPermissionError(
			domain.PermissionResourceBlog,
			domain.PermissionTypeRead,
			"Insufficient permissions: read access to blog required",
		)
	}

	return s.themeRepo.GetPublishedTheme(ctx)
}

// UpdateTheme updates an existing blog theme
func (s *BlogService) UpdateTheme(ctx context.Context, request *domain.UpdateBlogThemeRequest) (*domain.BlogTheme, error) {
	// Get workspace ID from context
	workspaceID, ok := ctx.Value("workspace_id").(string)
	if !ok {
		return nil, fmt.Errorf("workspace_id not found in context")
	}

	// Authenticate user for workspace
	var err error
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Check permission for writing blog
	if !userWorkspace.HasPermission(domain.PermissionResourceBlog, domain.PermissionTypeWrite) {
		return nil, domain.NewPermissionError(
			domain.PermissionResourceBlog,
			domain.PermissionTypeWrite,
			"Insufficient permissions: write access to blog required",
		)
	}

	// Validate the request
	if err := request.Validate(); err != nil {
		s.logger.Error("Failed to validate theme update request")
		return nil, err
	}

	// Get the existing theme
	theme, err := s.themeRepo.GetTheme(ctx, request.Version)
	if err != nil {
		s.logger.Error("Failed to get existing theme")
		return nil, fmt.Errorf("theme not found: %w", err)
	}

	// Check if theme is already published
	if theme.IsPublished() {
		return nil, fmt.Errorf("cannot update published theme")
	}

	// Update the theme fields
	theme.Files = request.Files
	theme.Notes = request.Notes
	theme.UpdatedAt = time.Now().UTC()

	// Validate the updated theme
	if err := theme.Validate(); err != nil {
		s.logger.Error("Failed to validate updated theme")
		return nil, err
	}

	// Persist the changes
	if err := s.themeRepo.UpdateTheme(ctx, theme); err != nil {
		s.logger.Error("Failed to update theme")
		return nil, fmt.Errorf("failed to update theme: %w", err)
	}

	return theme, nil
}

// PublishTheme publishes a blog theme
func (s *BlogService) PublishTheme(ctx context.Context, request *domain.PublishBlogThemeRequest) error {
	// Get workspace ID from context
	workspaceID, ok := ctx.Value("workspace_id").(string)
	if !ok {
		return fmt.Errorf("workspace_id not found in context")
	}

	// Authenticate user for workspace
	var err error
	ctx, user, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Check permission for writing blog
	if !userWorkspace.HasPermission(domain.PermissionResourceBlog, domain.PermissionTypeWrite) {
		return domain.NewPermissionError(
			domain.PermissionResourceBlog,
			domain.PermissionTypeWrite,
			"Insufficient permissions: write access to blog required",
		)
	}

	// Validate the request
	if err := request.Validate(); err != nil {
		s.logger.Error("Failed to validate theme publish request")
		return err
	}

	// Publish the theme (this will atomically unpublish others)
	if err := s.themeRepo.PublishTheme(ctx, request.Version, user.ID); err != nil {
		s.logger.Error("Failed to publish theme")
		return fmt.Errorf("failed to publish theme: %w", err)
	}

	return nil
}

// ListThemes retrieves blog themes with pagination
func (s *BlogService) ListThemes(ctx context.Context, params *domain.ListBlogThemesRequest) (*domain.BlogThemeListResponse, error) {
	// Get workspace ID from context
	workspaceID, ok := ctx.Value("workspace_id").(string)
	if !ok {
		return nil, fmt.Errorf("workspace_id not found in context")
	}

	// Authenticate user for workspace
	var err error
	ctx, _, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Check permission for reading blog
	if !userWorkspace.HasPermission(domain.PermissionResourceBlog, domain.PermissionTypeRead) {
		return nil, domain.NewPermissionError(
			domain.PermissionResourceBlog,
			domain.PermissionTypeRead,
			"Insufficient permissions: read access to blog required",
		)
	}

	// Validate the request
	if err := params.Validate(); err != nil {
		s.logger.Error("Failed to validate theme list request")
		return nil, err
	}

	return s.themeRepo.ListThemes(ctx, *params)
}

// ====================
// Blog Page Rendering
// ====================

// getPublicListsForWorkspace fetches all public lists for a workspace
// This is a private helper method used by rendering methods
func (s *BlogService) getPublicListsForWorkspace(ctx context.Context, workspaceID string) ([]*domain.List, error) {
	// Get all lists for the workspace
	allLists, err := s.listRepo.GetLists(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get lists: %w", err)
	}

	// Filter to only public lists
	publicLists := make([]*domain.List, 0)
	for _, list := range allLists {
		if list.IsPublic && list.DeletedAt == nil {
			publicLists = append(publicLists, list)
		}
	}

	return publicLists, nil
}

// RenderHomePage renders the blog home page with published posts
func (s *BlogService) RenderHomePage(ctx context.Context, workspaceID string, page int) (string, error) {
	// Validate page number
	if page < 1 {
		page = 1
	}

	// Get workspace
	workspace, err := s.workspaceRepo.GetByID(ctx, workspaceID)
	if err != nil {
		return "", &domain.BlogRenderError{
			Code:    domain.ErrCodeRenderFailed,
			Message: "Failed to get workspace",
			Details: err,
		}
	}

	// Get published theme
	theme, err := s.themeRepo.GetPublishedTheme(ctx)
	if err != nil {
		if err.Error() == "no published theme found" || err.Error() == "sql: no rows in result set" {
			return "", &domain.BlogRenderError{
				Code:    domain.ErrCodeThemeNotPublished,
				Message: "No published theme available",
				Details: err,
			}
		}
		return "", &domain.BlogRenderError{
			Code:    domain.ErrCodeThemeNotFound,
			Message: "Failed to get theme",
			Details: err,
		}
	}

	// Get public lists
	publicLists, err := s.getPublicListsForWorkspace(ctx, workspaceID)
	if err != nil {
		s.logger.WithField("error", err.Error()).Warn("Failed to get public lists for blog home page")
		// Don't fail rendering if lists can't be fetched, just use empty array
		publicLists = []*domain.List{}
	}

	// Get page size from workspace settings
	pageSize := 20 // default
	if workspace.Settings.BlogSettings != nil {
		pageSize = workspace.Settings.BlogSettings.GetHomePageSize()
	}

	// Get published posts for home page
	params := &domain.ListBlogPostsRequest{
		Status: domain.BlogPostStatusPublished,
		Page:   page,
		Limit:  pageSize,
	}
	// Validate will calculate offset
	if err := params.Validate(); err != nil {
		return "", &domain.BlogRenderError{
			Code:    domain.ErrCodeRenderFailed,
			Message: "Invalid pagination parameters",
			Details: err,
		}
	}

	postsResponse, err := s.postRepo.ListPosts(ctx, *params)
	if err != nil {
		s.logger.WithField("error", err.Error()).Warn("Failed to get posts for blog home page")
		// Don't fail rendering if posts can't be fetched
		postsResponse = &domain.BlogPostListResponse{Posts: []*domain.BlogPost{}, TotalCount: 0}
	}

	// Return 404 if page > total_pages (and not page 1)
	if page > 1 && postsResponse.TotalPages > 0 && page > postsResponse.TotalPages {
		return "", &domain.BlogRenderError{
			Code:    domain.ErrCodePostNotFound, // Reuse for page not found
			Message: fmt.Sprintf("Page %d does not exist (total pages: %d)", page, postsResponse.TotalPages),
			Details: nil,
		}
	}

	// Get all categories for navigation
	categories, err := s.categoryRepo.ListCategories(ctx)
	if err != nil {
		s.logger.WithField("error", err.Error()).Warn("Failed to get categories for blog home page")
		categories = []*domain.BlogCategory{}
	}

	// Build template data with pagination
	templateData, err := domain.BuildBlogTemplateData(domain.BlogTemplateDataRequest{
		Workspace:      workspace,
		PublicLists:    publicLists,
		Posts:          postsResponse.Posts,
		Categories:     categories,
		ThemeVersion:   theme.Version,
		PaginationData: postsResponse,
	})
	if err != nil {
		return "", &domain.BlogRenderError{
			Code:    domain.ErrCodeRenderFailed,
			Message: "Failed to build template data",
			Details: err,
		}
	}

	// Add per_page to pagination data
	if paginationMap, ok := templateData["pagination"].(domain.MapOfAny); ok {
		paginationMap["per_page"] = pageSize
	}

	// Prepare partials map for the template engine
	partials := map[string]string{
		"shared": theme.Files.SharedLiquid,
		"header": theme.Files.HeaderLiquid,
		"footer": theme.Files.FooterLiquid,
	}

	// Render the home template with partials
	html, err := liquid.RenderBlogTemplate(theme.Files.HomeLiquid, templateData, partials)
	if err != nil {
		return "", &domain.BlogRenderError{
			Code:    domain.ErrCodeInvalidLiquidSyntax,
			Message: "Failed to render home template",
			Details: err,
		}
	}

	return html, nil
}

// RenderPostPage renders a single blog post page
func (s *BlogService) RenderPostPage(ctx context.Context, workspaceID, categorySlug, postSlug string) (string, error) {
	// Get workspace
	workspace, err := s.workspaceRepo.GetByID(ctx, workspaceID)
	if err != nil {
		return "", &domain.BlogRenderError{
			Code:    domain.ErrCodeRenderFailed,
			Message: "Failed to get workspace",
			Details: err,
		}
	}

	// Get published theme
	theme, err := s.themeRepo.GetPublishedTheme(ctx)
	if err != nil {
		if err.Error() == "no published theme found" || err.Error() == "sql: no rows in result set" {
			return "", &domain.BlogRenderError{
				Code:    domain.ErrCodeThemeNotPublished,
				Message: "No published theme available",
				Details: err,
			}
		}
		return "", &domain.BlogRenderError{
			Code:    domain.ErrCodeThemeNotFound,
			Message: "Failed to get theme",
			Details: err,
		}
	}

	// Get post by category and slug
	post, err := s.postRepo.GetPostByCategoryAndSlug(ctx, categorySlug, postSlug)
	if err != nil {
		return "", &domain.BlogRenderError{
			Code:    domain.ErrCodePostNotFound,
			Message: "Post not found",
			Details: err,
		}
	}

	// Check if post is published
	if !post.IsPublished() {
		return "", &domain.BlogRenderError{
			Code:    domain.ErrCodePostNotFound,
			Message: "Post is not published",
			Details: nil,
		}
	}

	// Get category
	category, err := s.categoryRepo.GetCategory(ctx, post.CategoryID)
	if err != nil {
		s.logger.WithField("error", err.Error()).Warn("Failed to get category for blog post page")
		category = nil
	}

	// Get public lists
	publicLists, err := s.getPublicListsForWorkspace(ctx, workspaceID)
	if err != nil {
		s.logger.WithField("error", err.Error()).Warn("Failed to get public lists for blog post page")
		publicLists = []*domain.List{}
	}

	// Get all categories for navigation
	categories, err := s.categoryRepo.ListCategories(ctx)
	if err != nil {
		s.logger.WithField("error", err.Error()).Warn("Failed to get categories for blog post page")
		categories = []*domain.BlogCategory{}
	}

	// Build template data
	templateData, err := domain.BuildBlogTemplateData(domain.BlogTemplateDataRequest{
		Workspace:    workspace,
		Post:         post,
		Category:     category,
		PublicLists:  publicLists,
		Categories:   categories,
		ThemeVersion: theme.Version,
	})
	if err != nil {
		return "", &domain.BlogRenderError{
			Code:    domain.ErrCodeRenderFailed,
			Message: "Failed to build template data",
			Details: err,
		}
	}

	// Prepare partials map for the template engine
	partials := map[string]string{
		"shared": theme.Files.SharedLiquid,
		"header": theme.Files.HeaderLiquid,
		"footer": theme.Files.FooterLiquid,
	}

	// Render the post template with partials
	html, err := liquid.RenderBlogTemplate(theme.Files.PostLiquid, templateData, partials)
	if err != nil {
		return "", &domain.BlogRenderError{
			Code:    domain.ErrCodeInvalidLiquidSyntax,
			Message: "Failed to render post template",
			Details: err,
		}
	}

	return html, nil
}

// RenderCategoryPage renders a category page with posts in that category
func (s *BlogService) RenderCategoryPage(ctx context.Context, workspaceID, categorySlug string, page int) (string, error) {
	// Validate page number
	if page < 1 {
		page = 1
	}

	// Get workspace
	workspace, err := s.workspaceRepo.GetByID(ctx, workspaceID)
	if err != nil {
		return "", &domain.BlogRenderError{
			Code:    domain.ErrCodeRenderFailed,
			Message: "Failed to get workspace",
			Details: err,
		}
	}

	// Get published theme
	theme, err := s.themeRepo.GetPublishedTheme(ctx)
	if err != nil {
		if err.Error() == "no published theme found" || err.Error() == "sql: no rows in result set" {
			return "", &domain.BlogRenderError{
				Code:    domain.ErrCodeThemeNotPublished,
				Message: "No published theme available",
				Details: err,
			}
		}
		return "", &domain.BlogRenderError{
			Code:    domain.ErrCodeThemeNotFound,
			Message: "Failed to get theme",
			Details: err,
		}
	}

	// Get category by slug
	category, err := s.categoryRepo.GetCategoryBySlug(ctx, categorySlug)
	if err != nil {
		return "", &domain.BlogRenderError{
			Code:    domain.ErrCodeCategoryNotFound,
			Message: "Category not found",
			Details: err,
		}
	}

	// Get public lists
	publicLists, err := s.getPublicListsForWorkspace(ctx, workspaceID)
	if err != nil {
		s.logger.WithField("error", err.Error()).Warn("Failed to get public lists for blog category page")
		publicLists = []*domain.List{}
	}

	// Get page size from workspace settings
	pageSize := 20 // default
	if workspace.Settings.BlogSettings != nil {
		pageSize = workspace.Settings.BlogSettings.GetCategoryPageSize()
	}

	// Get published posts in this category
	params := &domain.ListBlogPostsRequest{
		CategoryID: category.ID,
		Status:     domain.BlogPostStatusPublished,
		Page:       page,
		Limit:      pageSize,
	}
	// Validate will calculate offset
	if err := params.Validate(); err != nil {
		return "", &domain.BlogRenderError{
			Code:    domain.ErrCodeRenderFailed,
			Message: "Invalid pagination parameters",
			Details: err,
		}
	}

	postsResponse, err := s.postRepo.ListPosts(ctx, *params)
	if err != nil {
		s.logger.WithField("error", err.Error()).Warn("Failed to get posts for blog category page")
		postsResponse = &domain.BlogPostListResponse{Posts: []*domain.BlogPost{}, TotalCount: 0}
	}

	// Return 404 if page > total_pages (and not page 1)
	if page > 1 && postsResponse.TotalPages > 0 && page > postsResponse.TotalPages {
		return "", &domain.BlogRenderError{
			Code:    domain.ErrCodePostNotFound, // Reuse for page not found
			Message: fmt.Sprintf("Page %d does not exist (total pages: %d)", page, postsResponse.TotalPages),
			Details: nil,
		}
	}

	// Get all categories for navigation
	categories, err := s.categoryRepo.ListCategories(ctx)
	if err != nil {
		s.logger.WithField("error", err.Error()).Warn("Failed to get categories for blog category page")
		categories = []*domain.BlogCategory{}
	}

	// Build template data with pagination
	templateData, err := domain.BuildBlogTemplateData(domain.BlogTemplateDataRequest{
		Workspace:      workspace,
		Category:       category,
		PublicLists:    publicLists,
		Posts:          postsResponse.Posts,
		Categories:     categories,
		ThemeVersion:   theme.Version,
		PaginationData: postsResponse,
	})
	if err != nil {
		return "", &domain.BlogRenderError{
			Code:    domain.ErrCodeRenderFailed,
			Message: "Failed to build template data",
			Details: err,
		}
	}

	// Add per_page to pagination data
	if paginationMap, ok := templateData["pagination"].(domain.MapOfAny); ok {
		paginationMap["per_page"] = pageSize
	}

	// Prepare partials map for the template engine
	partials := map[string]string{
		"shared": theme.Files.SharedLiquid,
		"header": theme.Files.HeaderLiquid,
		"footer": theme.Files.FooterLiquid,
	}

	// Render the category template with partials
	html, err := liquid.RenderBlogTemplate(theme.Files.CategoryLiquid, templateData, partials)
	if err != nil {
		return "", &domain.BlogRenderError{
			Code:    domain.ErrCodeInvalidLiquidSyntax,
			Message: "Failed to render category template",
			Details: err,
		}
	}

	return html, nil
}
