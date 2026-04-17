package service

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/cache"
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
	templateRepo  domain.TemplateRepository
	authService   domain.AuthService
	cache         cache.Cache
}

// NewBlogService creates a new blog service
func NewBlogService(
	logger logger.Logger,
	categoryRepository domain.BlogCategoryRepository,
	postRepository domain.BlogPostRepository,
	themeRepository domain.BlogThemeRepository,
	workspaceRepository domain.WorkspaceRepository,
	listRepository domain.ListRepository,
	templateRepository domain.TemplateRepository,
	authService domain.AuthService,
	cache cache.Cache,
) *BlogService {
	return &BlogService{
		logger:        logger,
		categoryRepo:  categoryRepository,
		postRepo:      postRepository,
		themeRepo:     themeRepository,
		workspaceRepo: workspaceRepository,
		listRepo:      listRepository,
		templateRepo:  templateRepository,
		authService:   authService,
		cache:         cache,
	}
}

// ====================
// Category Operations
// ====================

// CreateCategory creates a new blog category
func (s *BlogService) CreateCategory(ctx context.Context, request *domain.CreateBlogCategoryRequest) (*domain.BlogCategory, error) {
	// Get workspace ID from context
	workspaceID, ok := ctx.Value(domain.WorkspaceIDKey).(string)
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
	workspaceID, ok := ctx.Value(domain.WorkspaceIDKey).(string)
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
	workspaceID, ok := ctx.Value(domain.WorkspaceIDKey).(string)
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

// GetPublicCategoryBySlug retrieves a blog category by slug for public blog pages (no authentication required)
func (s *BlogService) GetPublicCategoryBySlug(ctx context.Context, slug string) (*domain.BlogCategory, error) {
	// For public blog pages, we don't require authentication
	// Just get the category directly from the repository
	return s.categoryRepo.GetCategoryBySlug(ctx, slug)
}

// UpdateCategory updates an existing blog category
func (s *BlogService) UpdateCategory(ctx context.Context, request *domain.UpdateBlogCategoryRequest) (*domain.BlogCategory, error) {
	// Get workspace ID from context
	workspaceID, ok := ctx.Value(domain.WorkspaceIDKey).(string)
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
	workspaceID, ok := ctx.Value(domain.WorkspaceIDKey).(string)
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
	workspaceID, ok := ctx.Value(domain.WorkspaceIDKey).(string)
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
	workspaceID, ok := ctx.Value(domain.WorkspaceIDKey).(string)
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
	workspaceID, ok := ctx.Value(domain.WorkspaceIDKey).(string)
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
	workspaceID, ok := ctx.Value(domain.WorkspaceIDKey).(string)
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
	workspaceID, ok := ctx.Value(domain.WorkspaceIDKey).(string)
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
	workspaceID, ok := ctx.Value(domain.WorkspaceIDKey).(string)
	if !ok {
		return nil, fmt.Errorf("workspace_id not found in context")
	}

	s.logger.WithFields(map[string]interface{}{
		"workspace_id": workspaceID,
		"post_id":      request.ID,
		"category_id":  request.CategoryID,
		"slug":         request.Slug,
	}).Info("UpdatePost called")

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

	s.logger.WithField("post_id", post.ID).Info("Post updated successfully, clearing cache...")

	// Invalidate blog caches
	// Clear blog cache since post was updated
	s.clearBlogCache(workspaceID)

	s.logger.WithField("post_id", post.ID).Info("UpdatePost completed")

	return post, nil
}

// DeletePost deletes a blog post
func (s *BlogService) DeletePost(ctx context.Context, request *domain.DeleteBlogPostRequest) error {
	// Get workspace ID from context
	workspaceID, ok := ctx.Value(domain.WorkspaceIDKey).(string)
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

	// Verify post exists before deleting
	_, err = s.postRepo.GetPost(ctx, request.ID)
	if err != nil {
		s.logger.Error("Failed to get post for deletion")
		return fmt.Errorf("post not found: %w", err)
	}

	// Delete the post
	if err := s.postRepo.DeletePost(ctx, request.ID); err != nil {
		s.logger.Error("Failed to delete post")
		return fmt.Errorf("failed to delete post: %w", err)
	}

	// Clear blog cache
	s.clearBlogCache(workspaceID)

	return nil
}

// ListPosts retrieves blog posts with filtering and pagination
func (s *BlogService) ListPosts(ctx context.Context, params *domain.ListBlogPostsRequest) (*domain.BlogPostListResponse, error) {
	// Get workspace ID from context
	workspaceID, ok := ctx.Value(domain.WorkspaceIDKey).(string)
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
	workspaceID, ok := ctx.Value(domain.WorkspaceIDKey).(string)
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

	// Verify post exists
	_, err = s.postRepo.GetPost(ctx, request.ID)
	if err != nil {
		s.logger.Error("Failed to get post for publishing")
		return fmt.Errorf("failed to get post: %w", err)
	}

	// Publish the post with optional custom timestamp
	if err := s.postRepo.PublishPost(ctx, request.ID, request.PublishedAt); err != nil {
		s.logger.Error("Failed to publish post")
		return fmt.Errorf("failed to publish post: %w", err)
	}

	// Clear blog cache
	s.clearBlogCache(workspaceID)

	return nil
}

// UnpublishPost unpublishes a published blog post
func (s *BlogService) UnpublishPost(ctx context.Context, request *domain.UnpublishBlogPostRequest) error {
	// Get workspace ID from context
	workspaceID, ok := ctx.Value(domain.WorkspaceIDKey).(string)
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

	// Verify post exists
	_, err = s.postRepo.GetPost(ctx, request.ID)
	if err != nil {
		s.logger.Error("Failed to get post for unpublishing")
		return fmt.Errorf("failed to get post: %w", err)
	}

	// Unpublish the post
	if err := s.postRepo.UnpublishPost(ctx, request.ID); err != nil {
		s.logger.Error("Failed to unpublish post")
		return fmt.Errorf("failed to unpublish post: %w", err)
	}

	// Clear blog cache
	s.clearBlogCache(workspaceID)

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
	workspaceID, ok := ctx.Value(domain.WorkspaceIDKey).(string)
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
	workspaceID, ok := ctx.Value(domain.WorkspaceIDKey).(string)
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
	workspaceID, ok := ctx.Value(domain.WorkspaceIDKey).(string)
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
	workspaceID, ok := ctx.Value(domain.WorkspaceIDKey).(string)
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
	workspaceID, ok := ctx.Value(domain.WorkspaceIDKey).(string)
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

	// Clear all blog caches since theme affects all pages
	s.clearBlogCache(workspaceID)

	return nil
}

// ListThemes retrieves blog themes with pagination
func (s *BlogService) ListThemes(ctx context.Context, params *domain.ListBlogThemesRequest) (*domain.BlogThemeListResponse, error) {
	// Get workspace ID from context
	workspaceID, ok := ctx.Value(domain.WorkspaceIDKey).(string)
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

// invalidateBlogCaches clears all blog-related caches for a workspace
// This should be called when blog content changes (publish/unpublish posts, publish themes)
// categorySlug and postSlug are optional - when provided, invalidates the individual post page cache
// clearBlogCache clears the entire blog cache
// This is called for any blog CRUD operation to ensure cache consistency
func (s *BlogService) clearBlogCache(workspaceID string) {
	if s.cache == nil {
		s.logger.WithField("workspace_id", workspaceID).Warn("Blog cache is nil, cannot clear")
		return
	}

	// Log cache size before clearing
	sizeBefore := s.cache.Size()

	// Clear entire blog cache for any operation
	// This is simple, safe, and performant since blog writes are infrequent
	s.cache.Clear()

	sizeAfter := s.cache.Size()
	s.logger.WithFields(map[string]interface{}{
		"workspace_id": workspaceID,
		"size_before":  sizeBefore,
		"size_after":   sizeAfter,
	}).Info("Blog cache cleared")
}

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
func (s *BlogService) RenderHomePage(ctx context.Context, workspaceID string, page int, themeVersion *int) (string, error) {
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

	// Get theme (published or specific version)
	var theme *domain.BlogTheme
	if themeVersion != nil {
		theme, err = s.themeRepo.GetTheme(ctx, *themeVersion)
	} else {
		theme, err = s.themeRepo.GetPublishedTheme(ctx)
	}

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

	// Get all categories for navigation (non-deleted)
	categories, err := s.categoryRepo.ListCategories(ctx)
	if err != nil {
		s.logger.WithField("error", err.Error()).Warn("Failed to get categories for blog home page")
		categories = []*domain.BlogCategory{}
	}

	// Also fetch categories for posts (including deleted ones) to ensure category_slug is set
	// Collect unique category IDs from posts
	categoryIDSet := make(map[string]bool)
	for _, post := range postsResponse.Posts {
		if post.CategoryID != "" {
			categoryIDSet[post.CategoryID] = true
		}
	}

	// Fetch categories for posts (including deleted) for URL construction
	var postCategories []*domain.BlogCategory
	if len(categoryIDSet) > 0 {
		categoryIDs := make([]string, 0, len(categoryIDSet))
		for id := range categoryIDSet {
			categoryIDs = append(categoryIDs, id)
		}
		postCategories, err = s.categoryRepo.GetCategoriesByIDs(ctx, categoryIDs)
		if err != nil {
			s.logger.WithField("error", err.Error()).Warn("Failed to get categories for posts")
			postCategories = []*domain.BlogCategory{}
		}
	}

	// Merge categories: use postCategories for slug lookup (includes deleted), categories for navigation (non-deleted)
	// Create a map of all categories for slug lookup
	allCategoriesMap := make(map[string]*domain.BlogCategory)
	for _, cat := range categories {
		allCategoriesMap[cat.ID] = cat
	}
	for _, cat := range postCategories {
		// Only add if not already in map (prefer non-deleted)
		if _, exists := allCategoriesMap[cat.ID]; !exists {
			allCategoriesMap[cat.ID] = cat
		}
	}

	// Convert back to slice for BuildBlogTemplateData
	allCategoriesForSlugs := make([]*domain.BlogCategory, 0, len(allCategoriesMap))
	for _, cat := range allCategoriesMap {
		allCategoriesForSlugs = append(allCategoriesForSlugs, cat)
	}

	// Build template data with pagination
	templateData, err := domain.BuildBlogTemplateData(domain.BlogTemplateDataRequest{
		Workspace:      workspace,
		PublicLists:    publicLists,
		Posts:          postsResponse.Posts,
		Categories:     allCategoriesForSlugs, // Use all categories (including deleted) for slug lookup
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
		"shared":  theme.Files.SharedLiquid,
		"header":  theme.Files.HeaderLiquid,
		"footer":  theme.Files.FooterLiquid,
		"styles":  theme.Files.StylesCSS,
		"scripts": theme.Files.ScriptsJS,
	}

	// Render the home template with partials
	html, err := liquid.RenderBlogTemplate(theme.Files.HomeLiquid, templateData, partials)
	if err != nil {
		// Log detailed error for debugging
		s.logger.WithFields(map[string]interface{}{
			"error":                err.Error(),
			"workspace_id":         workspaceID,
			"theme_version":        theme.Version,
			"home_template_length": len(theme.Files.HomeLiquid),
			"partials":             []string{"shared", "header", "footer", "styles", "scripts"},
		}).Error("Failed to render home template - check DEBUG logs for template details")

		return "", &domain.BlogRenderError{
			Code:    domain.ErrCodeInvalidLiquidSyntax,
			Message: fmt.Sprintf("Failed to render home template: %v", err),
			Details: err,
		}
	}

	html = liquid.InjectFeedDiscoveryTags(html, blogTitleForDiscovery(workspace), "")
	return html, nil
}

// RenderPostPage renders a single blog post page
func (s *BlogService) RenderPostPage(ctx context.Context, workspaceID, categorySlug, postSlug string, themeVersion *int) (string, error) {
	// Get workspace
	workspace, err := s.workspaceRepo.GetByID(ctx, workspaceID)
	if err != nil {
		return "", &domain.BlogRenderError{
			Code:    domain.ErrCodeRenderFailed,
			Message: "Failed to get workspace",
			Details: err,
		}
	}

	// Get theme (published or specific version)
	var theme *domain.BlogTheme
	if themeVersion != nil {
		theme, err = s.themeRepo.GetTheme(ctx, *themeVersion)
	} else {
		theme, err = s.themeRepo.GetPublishedTheme(ctx)
	}

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

	// Fetch the web template for the post content
	var postContentHTML string
	template, err := s.templateRepo.GetTemplateByID(ctx, workspaceID, post.Settings.Template.TemplateID, int64(post.Settings.Template.TemplateVersion))
	if err != nil {
		s.logger.WithFields(map[string]interface{}{
			"error":            err.Error(),
			"template_id":      post.Settings.Template.TemplateID,
			"template_version": post.Settings.Template.TemplateVersion,
		}).Warn("Failed to get template for blog post - post content will be empty")
		postContentHTML = ""
	} else if template.Web != nil && template.Web.HTML != "" {
		// Use the pre-rendered HTML from the web template
		postContentHTML = template.Web.HTML
	} else {
		s.logger.WithFields(map[string]interface{}{
			"template_id":      post.Settings.Template.TemplateID,
			"template_version": post.Settings.Template.TemplateVersion,
		}).Warn("Template has no web content - post content will be empty")
		postContentHTML = ""
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

	// Add compiled HTML content to post data
	if postData, ok := templateData["post"].(domain.MapOfAny); ok {
		// Extract table of contents from HTML and ensure headings have IDs
		tocItems, modifiedHTML, err := ExtractTableOfContents(postContentHTML)
		if err != nil {
			s.logger.WithField("error", err.Error()).Warn("Failed to extract table of contents")
			// Continue without TOC if extraction fails, use original HTML
			tocItems = []domain.TOCItem{}
			modifiedHTML = postContentHTML
		}

		// Use modified HTML (with IDs added to headings) for content
		postData["content"] = modifiedHTML

		// Convert TOC items to a format suitable for Liquid templates
		tocData := make([]map[string]interface{}, len(tocItems))
		for i, item := range tocItems {
			tocData[i] = map[string]interface{}{
				"id":    item.ID,
				"level": item.Level,
				"text":  item.Text,
			}
		}
		postData["table_of_contents"] = tocData
	}

	// Prepare partials map for the template engine
	partials := map[string]string{
		"shared":  theme.Files.SharedLiquid,
		"header":  theme.Files.HeaderLiquid,
		"footer":  theme.Files.FooterLiquid,
		"styles":  theme.Files.StylesCSS,
		"scripts": theme.Files.ScriptsJS,
	}

	// Render the post template with partials
	html, err := liquid.RenderBlogTemplate(theme.Files.PostLiquid, templateData, partials)
	if err != nil {
		// Log detailed error for debugging
		s.logger.WithFields(map[string]interface{}{
			"error":                err.Error(),
			"workspace_id":         workspaceID,
			"theme_version":        theme.Version,
			"post_slug":            postSlug,
			"category_slug":        categorySlug,
			"post_template_length": len(theme.Files.PostLiquid),
			"partials":             []string{"shared", "header", "footer", "styles", "scripts"},
		}).Error("Failed to render post template - check DEBUG logs for template details")

		return "", &domain.BlogRenderError{
			Code:    domain.ErrCodeInvalidLiquidSyntax,
			Message: fmt.Sprintf("Failed to render post template: %v", err),
			Details: err,
		}
	}

	html = liquid.InjectFeedDiscoveryTags(html, blogTitleForDiscovery(workspace), categorySlug)
	return html, nil
}

// RenderPostContent returns the post body HTML prepared for syndication
// (RSS / JSON Feed) — no theme chrome, no header/footer, relative URLs
// rewritten to absolute against the workspace origin, sanitized against
// XSS, and stripped of XML-illegal control characters.
//
// Errors surface as *domain.BlogRenderError so callers (feed builder) can
// discriminate "post not found" from "render failed" and apply the fallback
// ladder documented in BuildFeed. Callers that already hold the workspace,
// post, and template entities should use renderPostContentFromEntities to
// avoid redundant lookups.
func (s *BlogService) RenderPostContent(ctx context.Context, workspaceID, categorySlug, postSlug string) (string, error) {
	workspace, err := s.workspaceRepo.GetByID(ctx, workspaceID)
	if err != nil {
		return "", &domain.BlogRenderError{
			Code:    domain.ErrCodeRenderFailed,
			Message: "Failed to get workspace",
			Details: err,
		}
	}

	post, err := s.postRepo.GetPostByCategoryAndSlug(ctx, categorySlug, postSlug)
	if err != nil {
		return "", &domain.BlogRenderError{
			Code:    domain.ErrCodePostNotFound,
			Message: "Post not found",
			Details: err,
		}
	}
	if !post.IsPublished() {
		return "", &domain.BlogRenderError{
			Code:    domain.ErrCodePostNotFound,
			Message: "Post is not published",
		}
	}

	tmpl, err := s.templateRepo.GetTemplateByID(ctx, workspaceID, post.Settings.Template.TemplateID, int64(post.Settings.Template.TemplateVersion))
	if err != nil {
		return "", &domain.BlogRenderError{
			Code:    domain.ErrCodeRenderFailed,
			Message: "Failed to get post template",
			Details: err,
		}
	}
	return renderPostContentFromEntities(workspace, tmpl)
}

// renderPostContentFromEntities is the shared rendering + sanitization path
// used by both RenderPostContent (which loads entities itself) and BuildFeed
// (which preloads them in batch). Does not hit the database.
func renderPostContentFromEntities(workspace *domain.Workspace, tmpl *domain.Template) (string, error) {
	if tmpl == nil || tmpl.Web == nil || tmpl.Web.HTML == "" {
		return "", &domain.BlogRenderError{
			Code:    domain.ErrCodeRenderFailed,
			Message: "Post template has no web content",
		}
	}
	sanitized, err := liquid.SanitizeFeedHTML(tmpl.Web.HTML, workspaceBlogOrigin(workspace))
	if err != nil {
		return "", &domain.BlogRenderError{
			Code:    domain.ErrCodeRenderFailed,
			Message: "Failed to sanitize post HTML",
			Details: err,
		}
	}
	return sanitized, nil
}

// BuildFeed loads the newest published posts (optionally filtered by
// category), renders each body, and returns a *domain.BlogFeed the feed
// renderer package can serialize. Callers that care about conditional GET
// should call GetFeedFingerprint first and skip BuildFeed on cache hits —
// body rendering is the expensive step.
func (s *BlogService) BuildFeed(ctx context.Context, workspaceID string, categorySlug *string) (*domain.BlogFeed, error) {
	workspace, err := s.workspaceRepo.GetByID(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace: %w", err)
	}

	origin := workspaceBlogOrigin(workspace)
	if origin == "" {
		return nil, fmt.Errorf("feed unavailable: workspace has no website URL configured")
	}

	limit := workspace.Settings.BlogSettings.GetFeedMaxItems()

	// The repo methods read workspaceID from ctx (like ListPosts).
	feedCtx := context.WithValue(ctx, domain.WorkspaceIDKey, workspaceID)

	posts, err := s.postRepo.ListFeedPosts(feedCtx, categorySlug, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list feed posts: %w", err)
	}

	maxUpdatedAt, idsHash, err := s.postRepo.GetFeedFingerprint(feedCtx, categorySlug, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to compute feed fingerprint: %w", err)
	}
	// Empty feed: repo returns zero-time. Substitute workspace.UpdatedAt so
	// the channel's <lastBuildDate> / Last-Modified are sane rather than
	// epoch.
	if maxUpdatedAt.IsZero() {
		maxUpdatedAt = workspace.UpdatedAt.UTC()
	}

	// Resolve categories for the items in one shot.
	categoryIDs := make([]string, 0, len(posts))
	seen := map[string]struct{}{}
	for _, p := range posts {
		if _, ok := seen[p.CategoryID]; ok {
			continue
		}
		seen[p.CategoryID] = struct{}{}
		categoryIDs = append(categoryIDs, p.CategoryID)
	}
	categoriesByID := map[string]*domain.BlogCategory{}
	if len(categoryIDs) > 0 {
		cats, err := s.categoryRepo.GetCategoriesByIDs(feedCtx, categoryIDs)
		if err != nil {
			s.logger.WithField("error", err.Error()).Warn("Failed to load categories for feed; continuing with empty names")
		} else {
			for _, c := range cats {
				categoriesByID[c.ID] = c
			}
		}
	}

	summaryOnly := workspace.Settings.BlogSettings != nil && workspace.Settings.BlogSettings.FeedSummaryOnly

	// Preload templates for all posts up front: many posts share the same
	// (templateID, version) pair, and the feed used to re-fetch the workspace
	// + post + template for every item through RenderPostContent. A single
	// pass here turns the worst case from O(N) round-trips into O(distinct
	// templates).
	templates := map[string]*domain.Template{}
	if !summaryOnly {
		for _, post := range posts {
			key := post.Settings.Template.TemplateID + "@" + strconv.Itoa(post.Settings.Template.TemplateVersion)
			if _, cached := templates[key]; cached {
				continue
			}
			tmpl, err := s.templateRepo.GetTemplateByID(feedCtx, workspaceID, post.Settings.Template.TemplateID, int64(post.Settings.Template.TemplateVersion))
			if err != nil {
				s.logger.WithFields(map[string]interface{}{
					"workspace_id":     workspaceID,
					"post_id":          post.ID,
					"template_id":      post.Settings.Template.TemplateID,
					"template_version": post.Settings.Template.TemplateVersion,
					"error":            err.Error(),
				}).Warn("Feed: template fetch failed; post will fall back to excerpt")
				templates[key] = nil
				continue
			}
			templates[key] = tmpl
		}
	}

	items := make([]domain.BlogFeedItem, 0, len(posts))
	for _, post := range posts {
		cat := categoriesByID[post.CategoryID]
		if cat == nil {
			s.logger.WithFields(map[string]interface{}{
				"workspace_id": workspaceID,
				"post_id":      post.ID,
				"category_id":  post.CategoryID,
			}).Error("Feed: dropping item — category not found (orphan post)")
			continue
		}

		item := domain.BlogFeedItem{
			GUID:             post.ID,
			Title:            post.Settings.Title,
			URL:              buildPostURL(origin, cat.Slug, post.Slug),
			CategorySlug:     cat.Slug,
			CategoryName:     cat.Settings.Name,
			Excerpt:          post.Settings.Excerpt,
			Authors:          post.Settings.Authors,
			FeaturedImageURL: post.Settings.FeaturedImageURL,
			UpdatedAt:        post.UpdatedAt,
		}
		if post.PublishedAt != nil {
			item.PublishedAt = *post.PublishedAt
		}

		// Content fallback ladder: full render → excerpt → drop.
		if summaryOnly {
			item.ContentHTML = post.Settings.Excerpt
			items = append(items, item)
			continue
		}

		tmpl := templates[post.Settings.Template.TemplateID+"@"+strconv.Itoa(post.Settings.Template.TemplateVersion)]
		body, err := renderPostContentFromEntities(workspace, tmpl)
		if err != nil {
			s.logger.WithFields(map[string]interface{}{
				"workspace_id":  workspaceID,
				"post_id":       post.ID,
				"category_slug": cat.Slug,
				"error":         err.Error(),
			}).Warn("Feed: body render failed; falling back to excerpt")
			if post.Settings.Excerpt == "" {
				s.logger.WithFields(map[string]interface{}{
					"workspace_id":  workspaceID,
					"post_id":       post.ID,
					"category_slug": cat.Slug,
				}).Error("Feed: dropping item — render failed and excerpt empty")
				continue
			}
			item.ContentHTML = post.Settings.Excerpt
		} else {
			item.ContentHTML = body
		}

		items = append(items, item)
	}

	language := workspace.Settings.DefaultLanguage
	if language == "" {
		language = "en"
	}
	blogTitle := ""
	blogDescription := ""
	var logoURL, iconURL string
	if bs := workspace.Settings.BlogSettings; bs != nil {
		blogTitle = bs.Title
		if bs.SEO != nil && bs.SEO.MetaDescription != "" {
			blogDescription = bs.SEO.MetaDescription
		}
		if bs.LogoURL != nil {
			logoURL = *bs.LogoURL
		}
		if bs.IconURL != nil {
			iconURL = *bs.IconURL
		}
	}
	if blogTitle == "" {
		blogTitle = workspace.Name
	}
	if blogDescription == "" {
		blogDescription = blogTitle
	}

	selfPath := "/feed.xml"
	if categorySlug != nil && *categorySlug != "" {
		selfPath = "/" + *categorySlug + "/feed.xml"
	}

	meta := domain.BlogFeedMeta{
		Title:       blogTitle,
		Description: blogDescription,
		SiteURL:     origin,
		FeedURL:     joinURL(origin, selfPath),
		SelfURL:     joinURL(origin, selfPath),
		Language:    language,
		IconURL:     iconURL,
		LogoURL:     logoURL,
		UpdatedAt:   maxUpdatedAt,
		ETag:        computeFeedETag(workspace, categorySlug, maxUpdatedAt, idsHash),
	}
	return &domain.BlogFeed{Meta: meta, Items: items}, nil
}

// GetFeedFingerprint returns (maxUpdatedAt, etag) without rendering items.
// HTTP handlers use this for conditional GET.
func (s *BlogService) GetFeedFingerprint(ctx context.Context, workspaceID string, categorySlug *string) (time.Time, string, error) {
	workspace, err := s.workspaceRepo.GetByID(ctx, workspaceID)
	if err != nil {
		return time.Time{}, "", fmt.Errorf("failed to get workspace: %w", err)
	}
	limit := workspace.Settings.BlogSettings.GetFeedMaxItems()
	feedCtx := context.WithValue(ctx, domain.WorkspaceIDKey, workspaceID)

	maxUpdatedAt, idsHash, err := s.postRepo.GetFeedFingerprint(feedCtx, categorySlug, limit)
	if err != nil {
		return time.Time{}, "", fmt.Errorf("failed to compute feed fingerprint: %w", err)
	}
	if maxUpdatedAt.IsZero() {
		maxUpdatedAt = workspace.UpdatedAt.UTC()
	}
	return maxUpdatedAt, computeFeedETag(workspace, categorySlug, maxUpdatedAt, idsHash), nil
}

// computeFeedETag hashes the fingerprint inputs to a short hex ETag.
// Inputs: maxUpdatedAt, idsHash, categorySlug, settings fingerprint (blog
// title/logos/feed toggles/default language). Any one changing invalidates.
func computeFeedETag(ws *domain.Workspace, categorySlug *string, maxUpdatedAt time.Time, idsHash string) string {
	var settingsPart struct {
		Title           string
		LogoURL         string
		IconURL         string
		FeedSummaryOnly bool
		FeedMaxItems    int
		DefaultLanguage string
	}
	if bs := ws.Settings.BlogSettings; bs != nil {
		settingsPart.Title = bs.Title
		if bs.LogoURL != nil {
			settingsPart.LogoURL = *bs.LogoURL
		}
		if bs.IconURL != nil {
			settingsPart.IconURL = *bs.IconURL
		}
		settingsPart.FeedSummaryOnly = bs.FeedSummaryOnly
		settingsPart.FeedMaxItems = bs.FeedMaxItems
	}
	settingsPart.DefaultLanguage = ws.Settings.DefaultLanguage
	settingsBlob, _ := json.Marshal(settingsPart)

	catSlug := ""
	if categorySlug != nil {
		catSlug = *categorySlug
	}

	h := sha256.New()
	h.Write([]byte(maxUpdatedAt.UTC().Format(time.RFC3339Nano)))
	h.Write([]byte{0})
	h.Write([]byte(idsHash))
	h.Write([]byte{0})
	h.Write([]byte(catSlug))
	h.Write([]byte{0})
	h.Write(settingsBlob)
	sum := h.Sum(nil)
	// Emit as a weak ETag: sanitizer output is deterministic but not
	// byte-identical across library version bumps (bluemonday, goquery), so
	// the strong-ETag guarantee would overstate equivalence.
	return `W/"` + hex.EncodeToString(sum[:8]) + `"`
}

func buildPostURL(origin, categorySlug, postSlug string) string {
	return joinURL(origin, "/"+categorySlug+"/"+postSlug)
}

func joinURL(origin, path string) string {
	if origin == "" {
		return path
	}
	origin = strings.TrimRight(origin, "/")
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return origin + path
}

func blogTitleForDiscovery(ws *domain.Workspace) string {
	if ws.Settings.BlogSettings != nil && ws.Settings.BlogSettings.Title != "" {
		return ws.Settings.BlogSettings.Title
	}
	return ws.Name
}

// workspaceBlogOrigin returns the public URL origin to use as the base for
// absolute URL rewriting in feed output. Mirrors the fallback chain used in
// domain.BuildBlogTemplateData (CustomEndpointURL > WebsiteURL).
func workspaceBlogOrigin(ws *domain.Workspace) string {
	if ws == nil {
		return ""
	}
	if ws.Settings.CustomEndpointURL != nil && *ws.Settings.CustomEndpointURL != "" {
		return *ws.Settings.CustomEndpointURL
	}
	return ws.Settings.WebsiteURL
}

// RenderCategoryPage renders a category page with posts in that category
func (s *BlogService) RenderCategoryPage(ctx context.Context, workspaceID, categorySlug string, page int, themeVersion *int) (string, error) {
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

	// Get theme (published or specific version)
	var theme *domain.BlogTheme
	if themeVersion != nil {
		theme, err = s.themeRepo.GetTheme(ctx, *themeVersion)
	} else {
		theme, err = s.themeRepo.GetPublishedTheme(ctx)
	}

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
		"shared":  theme.Files.SharedLiquid,
		"header":  theme.Files.HeaderLiquid,
		"footer":  theme.Files.FooterLiquid,
		"styles":  theme.Files.StylesCSS,
		"scripts": theme.Files.ScriptsJS,
	}

	// Render the category template with partials
	html, err := liquid.RenderBlogTemplate(theme.Files.CategoryLiquid, templateData, partials)
	if err != nil {
		// Log detailed error for debugging
		s.logger.WithFields(map[string]interface{}{
			"error":                    err.Error(),
			"workspace_id":             workspaceID,
			"theme_version":            theme.Version,
			"category_slug":            categorySlug,
			"category_template_length": len(theme.Files.CategoryLiquid),
			"partials":                 []string{"shared", "header", "footer", "styles", "scripts"},
		}).Error("Failed to render category template - check DEBUG logs for template details")

		return "", &domain.BlogRenderError{
			Code:    domain.ErrCodeInvalidLiquidSyntax,
			Message: fmt.Sprintf("Failed to render category template: %v", err),
			Details: err,
		}
	}

	html = liquid.InjectFeedDiscoveryTags(html, blogTitleForDiscovery(workspace), categorySlug)
	return html, nil
}
