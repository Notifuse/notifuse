package service

import (
	"context"
	"fmt"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/logger"
	"github.com/google/uuid"
)

// BlogService handles all blog-related operations
type BlogService struct {
	logger        logger.Logger
	categoryRepo  domain.BlogCategoryRepository
	postRepo      domain.BlogPostRepository
	workspaceRepo domain.WorkspaceRepository
	authService   domain.AuthService
}

// NewBlogService creates a new blog service
func NewBlogService(
	logger logger.Logger,
	categoryRepository domain.BlogCategoryRepository,
	postRepository domain.BlogPostRepository,
	workspaceRepository domain.WorkspaceRepository,
	authService domain.AuthService,
) *BlogService {
	return &BlogService{
		logger:        logger,
		categoryRepo:  categoryRepository,
		postRepo:      postRepository,
		workspaceRepo: workspaceRepository,
		authService:   authService,
	}
}

// ====================
// Category Operations
// ====================

// CreateCategory creates a new blog category
func (s *BlogService) CreateCategory(ctx context.Context, request *domain.CreateBlogCategoryRequest) (*domain.BlogCategory, error) {
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
	return s.categoryRepo.GetCategory(ctx, id)
}

// GetCategoryBySlug retrieves a blog category by slug
func (s *BlogService) GetCategoryBySlug(ctx context.Context, slug string) (*domain.BlogCategory, error) {
	return s.categoryRepo.GetCategoryBySlug(ctx, slug)
}

// UpdateCategory updates an existing blog category
func (s *BlogService) UpdateCategory(ctx context.Context, request *domain.UpdateBlogCategoryRequest) (*domain.BlogCategory, error) {
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

// DeleteCategory deletes a blog category
func (s *BlogService) DeleteCategory(ctx context.Context, request *domain.DeleteBlogCategoryRequest) error {
	// Validate the request
	if err := request.Validate(); err != nil {
		s.logger.Error("Failed to validate category deletion request")
		return err
	}

	// Delete the category
	if err := s.categoryRepo.DeleteCategory(ctx, request.ID); err != nil {
		s.logger.Error("Failed to delete category")
		return fmt.Errorf("failed to delete category: %w", err)
	}

	return nil
}

// ListCategories retrieves all blog categories for a workspace
func (s *BlogService) ListCategories(ctx context.Context) (*domain.BlogCategoryListResponse, error) {
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

	// Verify category exists if provided
	if request.CategoryID != nil && *request.CategoryID != "" {
		_, err := s.categoryRepo.GetCategory(ctx, *request.CategoryID)
		if err != nil {
			return nil, fmt.Errorf("category not found: %w", err)
		}
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
				TemplateData:    request.TemplateData,
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
	return s.postRepo.GetPost(ctx, id)
}

// GetPostBySlug retrieves a blog post by slug
func (s *BlogService) GetPostBySlug(ctx context.Context, slug string) (*domain.BlogPost, error) {
	return s.postRepo.GetPostBySlug(ctx, slug)
}

// GetPostByCategoryAndSlug retrieves a blog post by category slug and post slug
func (s *BlogService) GetPostByCategoryAndSlug(ctx context.Context, categorySlug, postSlug string) (*domain.BlogPost, error) {
	return s.postRepo.GetPostByCategoryAndSlug(ctx, categorySlug, postSlug)
}

// UpdatePost updates an existing blog post
func (s *BlogService) UpdatePost(ctx context.Context, request *domain.UpdateBlogPostRequest) (*domain.BlogPost, error) {
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

	// Verify category exists if provided
	if request.CategoryID != nil && *request.CategoryID != "" {
		_, err := s.categoryRepo.GetCategory(ctx, *request.CategoryID)
		if err != nil {
			return nil, fmt.Errorf("category not found: %w", err)
		}
	}

	// Update the post fields
	post.CategoryID = request.CategoryID
	post.Slug = request.Slug
	post.Settings.Title = request.Title
	post.Settings.Template.TemplateID = request.TemplateID
	post.Settings.Template.TemplateVersion = request.TemplateVersion
	post.Settings.Template.TemplateData = request.TemplateData
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
	// Validate the request
	if err := params.Validate(); err != nil {
		s.logger.Error("Failed to validate post list request")
		return nil, err
	}

	return s.postRepo.ListPosts(ctx, *params)
}

// PublishPost publishes a draft blog post
func (s *BlogService) PublishPost(ctx context.Context, request *domain.PublishBlogPostRequest) error {
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
