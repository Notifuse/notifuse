package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
)

// blogCategoryRepository implements domain.BlogCategoryRepository for PostgreSQL
type blogCategoryRepository struct {
	workspaceRepo domain.WorkspaceRepository
}

// NewBlogCategoryRepository creates a new PostgreSQL blog category repository
func NewBlogCategoryRepository(workspaceRepo domain.WorkspaceRepository) domain.BlogCategoryRepository {
	return &blogCategoryRepository{
		workspaceRepo: workspaceRepo,
	}
}

// WithTransaction executes a function within a transaction
func (r *blogCategoryRepository) WithTransaction(ctx context.Context, workspaceID string, fn func(*sql.Tx) error) error {
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}

	tx, err := workspaceDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer tx.Rollback()

	if err := fn(tx); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// CreateCategory persists a new blog category
func (r *blogCategoryRepository) CreateCategory(ctx context.Context, category *domain.BlogCategory) error {
	// Get workspace ID from context
	workspaceID, ok := ctx.Value("workspace_id").(string)
	if !ok {
		return fmt.Errorf("workspace_id not found in context")
	}

	return r.WithTransaction(ctx, workspaceID, func(tx *sql.Tx) error {
		return r.CreateCategoryTx(ctx, tx, category)
	})
}

// CreateCategoryTx persists a new blog category within a transaction
func (r *blogCategoryRepository) CreateCategoryTx(ctx context.Context, tx *sql.Tx, category *domain.BlogCategory) error {
	now := time.Now().UTC()
	category.CreatedAt = now
	category.UpdatedAt = now

	query := `
		INSERT INTO blog_categories (
			id, slug, settings, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5)
	`

	_, err := tx.ExecContext(ctx, query,
		category.ID,
		category.Slug,
		category.Settings,
		category.CreatedAt,
		category.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create blog category: %w", err)
	}

	return nil
}

// GetCategory retrieves a blog category by ID
func (r *blogCategoryRepository) GetCategory(ctx context.Context, id string) (*domain.BlogCategory, error) {
	workspaceID, ok := ctx.Value("workspace_id").(string)
	if !ok {
		return nil, fmt.Errorf("workspace_id not found in context")
	}

	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	return r.getCategoryByField(ctx, workspaceDB, "id", id)
}

// GetCategoryTx retrieves a blog category by ID within a transaction
func (r *blogCategoryRepository) GetCategoryTx(ctx context.Context, tx *sql.Tx, id string) (*domain.BlogCategory, error) {
	return r.getCategoryByFieldTx(ctx, tx, "id", id)
}

// GetCategoryBySlug retrieves a blog category by slug
func (r *blogCategoryRepository) GetCategoryBySlug(ctx context.Context, slug string) (*domain.BlogCategory, error) {
	workspaceID, ok := ctx.Value("workspace_id").(string)
	if !ok {
		return nil, fmt.Errorf("workspace_id not found in context")
	}

	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	return r.getCategoryByField(ctx, workspaceDB, "slug", slug)
}

// GetCategoryBySlugTx retrieves a blog category by slug within a transaction
func (r *blogCategoryRepository) GetCategoryBySlugTx(ctx context.Context, tx *sql.Tx, slug string) (*domain.BlogCategory, error) {
	return r.getCategoryByFieldTx(ctx, tx, "slug", slug)
}

// getCategoryByField retrieves a category by a specific field
func (r *blogCategoryRepository) getCategoryByField(ctx context.Context, db *sql.DB, field, value string) (*domain.BlogCategory, error) {
	query := fmt.Sprintf(`
		SELECT id, slug, settings, created_at, updated_at, deleted_at
		FROM blog_categories
		WHERE %s = $1 AND deleted_at IS NULL
	`, field)

	var category domain.BlogCategory
	err := db.QueryRowContext(ctx, query, value).Scan(
		&category.ID,
		&category.Slug,
		&category.Settings,
		&category.CreatedAt,
		&category.UpdatedAt,
		&category.DeletedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("blog category not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get blog category: %w", err)
	}

	return &category, nil
}

// getCategoryByFieldTx retrieves a category by a specific field within a transaction
func (r *blogCategoryRepository) getCategoryByFieldTx(ctx context.Context, tx *sql.Tx, field, value string) (*domain.BlogCategory, error) {
	query := fmt.Sprintf(`
		SELECT id, slug, settings, created_at, updated_at, deleted_at
		FROM blog_categories
		WHERE %s = $1 AND deleted_at IS NULL
	`, field)

	var category domain.BlogCategory
	err := tx.QueryRowContext(ctx, query, value).Scan(
		&category.ID,
		&category.Slug,
		&category.Settings,
		&category.CreatedAt,
		&category.UpdatedAt,
		&category.DeletedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("blog category not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get blog category: %w", err)
	}

	return &category, nil
}

// UpdateCategory updates an existing blog category
func (r *blogCategoryRepository) UpdateCategory(ctx context.Context, category *domain.BlogCategory) error {
	workspaceID, ok := ctx.Value("workspace_id").(string)
	if !ok {
		return fmt.Errorf("workspace_id not found in context")
	}

	return r.WithTransaction(ctx, workspaceID, func(tx *sql.Tx) error {
		return r.UpdateCategoryTx(ctx, tx, category)
	})
}

// UpdateCategoryTx updates an existing blog category within a transaction
func (r *blogCategoryRepository) UpdateCategoryTx(ctx context.Context, tx *sql.Tx, category *domain.BlogCategory) error {
	category.UpdatedAt = time.Now().UTC()

	query := `
		UPDATE blog_categories
		SET slug = $1, settings = $2, updated_at = $3
		WHERE id = $4 AND deleted_at IS NULL
	`

	result, err := tx.ExecContext(ctx, query,
		category.Slug,
		category.Settings,
		category.UpdatedAt,
		category.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update blog category: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("blog category not found")
	}

	return nil
}

// DeleteCategory soft deletes a blog category
func (r *blogCategoryRepository) DeleteCategory(ctx context.Context, id string) error {
	workspaceID, ok := ctx.Value("workspace_id").(string)
	if !ok {
		return fmt.Errorf("workspace_id not found in context")
	}

	return r.WithTransaction(ctx, workspaceID, func(tx *sql.Tx) error {
		return r.DeleteCategoryTx(ctx, tx, id)
	})
}

// DeleteCategoryTx soft deletes a blog category within a transaction
func (r *blogCategoryRepository) DeleteCategoryTx(ctx context.Context, tx *sql.Tx, id string) error {
	query := `
		UPDATE blog_categories
		SET deleted_at = $1
		WHERE id = $2 AND deleted_at IS NULL
	`

	result, err := tx.ExecContext(ctx, query, time.Now().UTC(), id)
	if err != nil {
		return fmt.Errorf("failed to delete blog category: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("blog category not found")
	}

	return nil
}

// ListCategories retrieves all blog categories for a workspace
func (r *blogCategoryRepository) ListCategories(ctx context.Context) ([]*domain.BlogCategory, error) {
	workspaceID, ok := ctx.Value("workspace_id").(string)
	if !ok {
		return nil, fmt.Errorf("workspace_id not found in context")
	}

	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	query := `
		SELECT id, slug, settings, created_at, updated_at, deleted_at
		FROM blog_categories
		WHERE deleted_at IS NULL
		ORDER BY created_at DESC
	`

	rows, err := workspaceDB.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list blog categories: %w", err)
	}
	defer rows.Close()

	var categories []*domain.BlogCategory
	for rows.Next() {
		var category domain.BlogCategory
		err := rows.Scan(
			&category.ID,
			&category.Slug,
			&category.Settings,
			&category.CreatedAt,
			&category.UpdatedAt,
			&category.DeletedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan blog category: %w", err)
		}
		categories = append(categories, &category)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating blog categories: %w", err)
	}

	return categories, nil
}

// blogPostRepository implements domain.BlogPostRepository for PostgreSQL
type blogPostRepository struct {
	workspaceRepo domain.WorkspaceRepository
}

// NewBlogPostRepository creates a new PostgreSQL blog post repository
func NewBlogPostRepository(workspaceRepo domain.WorkspaceRepository) domain.BlogPostRepository {
	return &blogPostRepository{
		workspaceRepo: workspaceRepo,
	}
}

// WithTransaction executes a function within a transaction
func (r *blogPostRepository) WithTransaction(ctx context.Context, workspaceID string, fn func(*sql.Tx) error) error {
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}

	tx, err := workspaceDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer tx.Rollback()

	if err := fn(tx); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// CreatePost persists a new blog post
func (r *blogPostRepository) CreatePost(ctx context.Context, post *domain.BlogPost) error {
	workspaceID, ok := ctx.Value("workspace_id").(string)
	if !ok {
		return fmt.Errorf("workspace_id not found in context")
	}

	return r.WithTransaction(ctx, workspaceID, func(tx *sql.Tx) error {
		return r.CreatePostTx(ctx, tx, post)
	})
}

// CreatePostTx persists a new blog post within a transaction
func (r *blogPostRepository) CreatePostTx(ctx context.Context, tx *sql.Tx, post *domain.BlogPost) error {
	now := time.Now().UTC()
	post.CreatedAt = now
	post.UpdatedAt = now

	query := `
		INSERT INTO blog_posts (
			id, category_id, slug, settings, published_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
	`

	_, err := tx.ExecContext(ctx, query,
		post.ID,
		post.CategoryID,
		post.Slug,
		post.Settings,
		post.PublishedAt,
		post.CreatedAt,
		post.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create blog post: %w", err)
	}

	return nil
}

// GetPost retrieves a blog post by ID
func (r *blogPostRepository) GetPost(ctx context.Context, id string) (*domain.BlogPost, error) {
	workspaceID, ok := ctx.Value("workspace_id").(string)
	if !ok {
		return nil, fmt.Errorf("workspace_id not found in context")
	}

	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	query := `
		SELECT id, category_id, slug, settings, published_at, created_at, updated_at, deleted_at
		FROM blog_posts
		WHERE id = $1 AND deleted_at IS NULL
	`

	var post domain.BlogPost
	err = workspaceDB.QueryRowContext(ctx, query, id).Scan(
		&post.ID,
		&post.CategoryID,
		&post.Slug,
		&post.Settings,
		&post.PublishedAt,
		&post.CreatedAt,
		&post.UpdatedAt,
		&post.DeletedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("blog post not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get blog post: %w", err)
	}

	return &post, nil
}

// GetPostTx retrieves a blog post by ID within a transaction
func (r *blogPostRepository) GetPostTx(ctx context.Context, tx *sql.Tx, id string) (*domain.BlogPost, error) {
	query := `
		SELECT id, category_id, slug, settings, published_at, created_at, updated_at, deleted_at
		FROM blog_posts
		WHERE id = $1 AND deleted_at IS NULL
	`

	var post domain.BlogPost
	err := tx.QueryRowContext(ctx, query, id).Scan(
		&post.ID,
		&post.CategoryID,
		&post.Slug,
		&post.Settings,
		&post.PublishedAt,
		&post.CreatedAt,
		&post.UpdatedAt,
		&post.DeletedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("blog post not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get blog post: %w", err)
	}

	return &post, nil
}

// GetPostBySlug retrieves a blog post by slug
func (r *blogPostRepository) GetPostBySlug(ctx context.Context, slug string) (*domain.BlogPost, error) {
	workspaceID, ok := ctx.Value("workspace_id").(string)
	if !ok {
		return nil, fmt.Errorf("workspace_id not found in context")
	}

	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	query := `
		SELECT id, category_id, slug, settings, published_at, created_at, updated_at, deleted_at
		FROM blog_posts
		WHERE slug = $1 AND deleted_at IS NULL
	`

	var post domain.BlogPost
	err = workspaceDB.QueryRowContext(ctx, query, slug).Scan(
		&post.ID,
		&post.CategoryID,
		&post.Slug,
		&post.Settings,
		&post.PublishedAt,
		&post.CreatedAt,
		&post.UpdatedAt,
		&post.DeletedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("blog post not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get blog post: %w", err)
	}

	return &post, nil
}

// GetPostBySlugTx retrieves a blog post by slug within a transaction
func (r *blogPostRepository) GetPostBySlugTx(ctx context.Context, tx *sql.Tx, slug string) (*domain.BlogPost, error) {
	query := `
		SELECT id, category_id, slug, settings, published_at, created_at, updated_at, deleted_at
		FROM blog_posts
		WHERE slug = $1 AND deleted_at IS NULL
	`

	var post domain.BlogPost
	err := tx.QueryRowContext(ctx, query, slug).Scan(
		&post.ID,
		&post.CategoryID,
		&post.Slug,
		&post.Settings,
		&post.PublishedAt,
		&post.CreatedAt,
		&post.UpdatedAt,
		&post.DeletedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("blog post not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get blog post: %w", err)
	}

	return &post, nil
}

// GetPostByCategoryAndSlug retrieves a blog post by category slug and post slug
func (r *blogPostRepository) GetPostByCategoryAndSlug(ctx context.Context, categorySlug, postSlug string) (*domain.BlogPost, error) {
	workspaceID, ok := ctx.Value("workspace_id").(string)
	if !ok {
		return nil, fmt.Errorf("workspace_id not found in context")
	}

	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	query := `
		SELECT p.id, p.category_id, p.slug, p.settings, p.published_at, p.created_at, p.updated_at, p.deleted_at
		FROM blog_posts p
		INNER JOIN blog_categories c ON p.category_id = c.id
		WHERE c.slug = $1 AND p.slug = $2 AND p.deleted_at IS NULL AND c.deleted_at IS NULL
	`

	var post domain.BlogPost
	err = workspaceDB.QueryRowContext(ctx, query, categorySlug, postSlug).Scan(
		&post.ID,
		&post.CategoryID,
		&post.Slug,
		&post.Settings,
		&post.PublishedAt,
		&post.CreatedAt,
		&post.UpdatedAt,
		&post.DeletedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("blog post not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get blog post: %w", err)
	}

	return &post, nil
}

// UpdatePost updates an existing blog post
func (r *blogPostRepository) UpdatePost(ctx context.Context, post *domain.BlogPost) error {
	workspaceID, ok := ctx.Value("workspace_id").(string)
	if !ok {
		return fmt.Errorf("workspace_id not found in context")
	}

	return r.WithTransaction(ctx, workspaceID, func(tx *sql.Tx) error {
		return r.UpdatePostTx(ctx, tx, post)
	})
}

// UpdatePostTx updates an existing blog post within a transaction
func (r *blogPostRepository) UpdatePostTx(ctx context.Context, tx *sql.Tx, post *domain.BlogPost) error {
	post.UpdatedAt = time.Now().UTC()

	query := `
		UPDATE blog_posts
		SET category_id = $1, slug = $2, settings = $3, published_at = $4, updated_at = $5
		WHERE id = $6 AND deleted_at IS NULL
	`

	result, err := tx.ExecContext(ctx, query,
		post.CategoryID,
		post.Slug,
		post.Settings,
		post.PublishedAt,
		post.UpdatedAt,
		post.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update blog post: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("blog post not found")
	}

	return nil
}

// DeletePost soft deletes a blog post
func (r *blogPostRepository) DeletePost(ctx context.Context, id string) error {
	workspaceID, ok := ctx.Value("workspace_id").(string)
	if !ok {
		return fmt.Errorf("workspace_id not found in context")
	}

	return r.WithTransaction(ctx, workspaceID, func(tx *sql.Tx) error {
		return r.DeletePostTx(ctx, tx, id)
	})
}

// DeletePostTx soft deletes a blog post within a transaction
func (r *blogPostRepository) DeletePostTx(ctx context.Context, tx *sql.Tx, id string) error {
	query := `
		UPDATE blog_posts
		SET deleted_at = $1
		WHERE id = $2 AND deleted_at IS NULL
	`

	result, err := tx.ExecContext(ctx, query, time.Now().UTC(), id)
	if err != nil {
		return fmt.Errorf("failed to delete blog post: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("blog post not found")
	}

	return nil
}

// ListPosts retrieves blog posts with filtering and pagination
func (r *blogPostRepository) ListPosts(ctx context.Context, params domain.ListBlogPostsRequest) (*domain.BlogPostListResponse, error) {
	workspaceID, ok := ctx.Value("workspace_id").(string)
	if !ok {
		return nil, fmt.Errorf("workspace_id not found in context")
	}

	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	// Build the WHERE clause based on filters
	whereConditions := []string{"deleted_at IS NULL"}
	args := []interface{}{}
	argIndex := 1

	if params.CategoryID != "" {
		whereConditions = append(whereConditions, fmt.Sprintf("category_id = $%d", argIndex))
		args = append(args, params.CategoryID)
		argIndex++
	}

	switch params.Status {
	case domain.BlogPostStatusDraft:
		whereConditions = append(whereConditions, "published_at IS NULL")
	case domain.BlogPostStatusPublished:
		whereConditions = append(whereConditions, "published_at IS NOT NULL")
		// BlogPostStatusAll means no filter on published_at
	}

	whereClause := "WHERE " + whereConditions[0]
	for i := 1; i < len(whereConditions); i++ {
		whereClause += " AND " + whereConditions[i]
	}

	// Count total
	countQuery := fmt.Sprintf(`
		SELECT COUNT(*)
		FROM blog_posts
		%s
	`, whereClause)

	var totalCount int
	err = workspaceDB.QueryRowContext(ctx, countQuery, args...).Scan(&totalCount)
	if err != nil {
		return nil, fmt.Errorf("failed to count blog posts: %w", err)
	}

	// Get posts with pagination
	query := fmt.Sprintf(`
		SELECT id, category_id, slug, settings, published_at, created_at, updated_at, deleted_at
		FROM blog_posts
		%s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argIndex, argIndex+1)

	args = append(args, params.Limit, params.Offset)

	rows, err := workspaceDB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list blog posts: %w", err)
	}
	defer rows.Close()

	var posts []*domain.BlogPost
	for rows.Next() {
		var post domain.BlogPost
		err := rows.Scan(
			&post.ID,
			&post.CategoryID,
			&post.Slug,
			&post.Settings,
			&post.PublishedAt,
			&post.CreatedAt,
			&post.UpdatedAt,
			&post.DeletedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan blog post: %w", err)
		}
		posts = append(posts, &post)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating blog posts: %w", err)
	}

	return &domain.BlogPostListResponse{
		Posts:      posts,
		TotalCount: totalCount,
	}, nil
}

// PublishPost sets the published_at timestamp to now
func (r *blogPostRepository) PublishPost(ctx context.Context, id string) error {
	workspaceID, ok := ctx.Value("workspace_id").(string)
	if !ok {
		return fmt.Errorf("workspace_id not found in context")
	}

	return r.WithTransaction(ctx, workspaceID, func(tx *sql.Tx) error {
		return r.PublishPostTx(ctx, tx, id)
	})
}

// PublishPostTx sets the published_at timestamp to now within a transaction
func (r *blogPostRepository) PublishPostTx(ctx context.Context, tx *sql.Tx, id string) error {
	query := `
		UPDATE blog_posts
		SET published_at = $1, updated_at = $2
		WHERE id = $3 AND deleted_at IS NULL AND published_at IS NULL
	`

	now := time.Now().UTC()
	result, err := tx.ExecContext(ctx, query, now, now, id)
	if err != nil {
		return fmt.Errorf("failed to publish blog post: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("blog post not found or already published")
	}

	return nil
}

// UnpublishPost sets the published_at timestamp to null
func (r *blogPostRepository) UnpublishPost(ctx context.Context, id string) error {
	workspaceID, ok := ctx.Value("workspace_id").(string)
	if !ok {
		return fmt.Errorf("workspace_id not found in context")
	}

	return r.WithTransaction(ctx, workspaceID, func(tx *sql.Tx) error {
		return r.UnpublishPostTx(ctx, tx, id)
	})
}

// UnpublishPostTx sets the published_at timestamp to null within a transaction
func (r *blogPostRepository) UnpublishPostTx(ctx context.Context, tx *sql.Tx, id string) error {
	query := `
		UPDATE blog_posts
		SET published_at = NULL, updated_at = $1
		WHERE id = $2 AND deleted_at IS NULL AND published_at IS NOT NULL
	`

	now := time.Now().UTC()
	result, err := tx.ExecContext(ctx, query, now, id)
	if err != nil {
		return fmt.Errorf("failed to unpublish blog post: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("blog post not found or not published")
	}

	return nil
}

// DeletePostsByCategoryIDTx soft deletes all posts belonging to a category within a transaction
func (r *blogPostRepository) DeletePostsByCategoryIDTx(ctx context.Context, tx *sql.Tx, categoryID string) (int64, error) {
	query := `
		UPDATE blog_posts
		SET deleted_at = $1
		WHERE category_id = $2 AND deleted_at IS NULL
	`

	result, err := tx.ExecContext(ctx, query, time.Now().UTC(), categoryID)
	if err != nil {
		return 0, fmt.Errorf("failed to delete blog posts by category: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}

	return rowsAffected, nil
}
