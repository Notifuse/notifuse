package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
)

// blogThemeRepository implements domain.BlogThemeRepository for PostgreSQL
type blogThemeRepository struct {
	workspaceRepo domain.WorkspaceRepository
}

// NewBlogThemeRepository creates a new PostgreSQL blog theme repository
func NewBlogThemeRepository(workspaceRepo domain.WorkspaceRepository) domain.BlogThemeRepository {
	return &blogThemeRepository{
		workspaceRepo: workspaceRepo,
	}
}

// WithTransaction executes a function within a transaction
func (r *blogThemeRepository) WithTransaction(ctx context.Context, workspaceID string, fn func(*sql.Tx) error) error {
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

// CreateTheme persists a new blog theme
func (r *blogThemeRepository) CreateTheme(ctx context.Context, theme *domain.BlogTheme) error {
	workspaceID, ok := ctx.Value("workspace_id").(string)
	if !ok {
		return fmt.Errorf("workspace_id not found in context")
	}

	return r.WithTransaction(ctx, workspaceID, func(tx *sql.Tx) error {
		return r.CreateThemeTx(ctx, tx, theme)
	})
}

// CreateThemeTx persists a new blog theme within a transaction
func (r *blogThemeRepository) CreateThemeTx(ctx context.Context, tx *sql.Tx, theme *domain.BlogTheme) error {
	// Generate next version number
	var maxVersion sql.NullInt64
	err := tx.QueryRowContext(ctx, "SELECT MAX(version) FROM blog_themes").Scan(&maxVersion)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("failed to get max version: %w", err)
	}

	nextVersion := 1
	if maxVersion.Valid {
		nextVersion = int(maxVersion.Int64) + 1
	}
	theme.Version = nextVersion

	now := time.Now().UTC()
	theme.CreatedAt = now
	theme.UpdatedAt = now

	query := `
		INSERT INTO blog_themes (
			version, published_at, published_by_user_id, files, notes, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
	`

	_, err = tx.ExecContext(ctx, query,
		theme.Version,
		theme.PublishedAt,
		theme.PublishedByUserID,
		theme.Files,
		theme.Notes,
		theme.CreatedAt,
		theme.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create blog theme: %w", err)
	}

	return nil
}

// GetTheme retrieves a blog theme by version
func (r *blogThemeRepository) GetTheme(ctx context.Context, version int) (*domain.BlogTheme, error) {
	workspaceID, ok := ctx.Value("workspace_id").(string)
	if !ok {
		return nil, fmt.Errorf("workspace_id not found in context")
	}

	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	query := `
		SELECT version, published_at, published_by_user_id, files, notes, created_at, updated_at
		FROM blog_themes
		WHERE version = $1
	`

	var theme domain.BlogTheme
	var notes sql.NullString
	var publishedByUserID sql.NullString
	err = workspaceDB.QueryRowContext(ctx, query, version).Scan(
		&theme.Version,
		&theme.PublishedAt,
		&publishedByUserID,
		&theme.Files,
		&notes,
		&theme.CreatedAt,
		&theme.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("blog theme not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get blog theme: %w", err)
	}

	// Convert sql.NullString to *string
	if notes.Valid {
		theme.Notes = &notes.String
	}
	if publishedByUserID.Valid {
		theme.PublishedByUserID = &publishedByUserID.String
	}

	return &theme, nil
}

// GetThemeTx retrieves a blog theme by version within a transaction
func (r *blogThemeRepository) GetThemeTx(ctx context.Context, tx *sql.Tx, version int) (*domain.BlogTheme, error) {
	query := `
		SELECT version, published_at, published_by_user_id, files, notes, created_at, updated_at
		FROM blog_themes
		WHERE version = $1
	`

	var theme domain.BlogTheme
	var notes sql.NullString
	var publishedByUserID sql.NullString
	err := tx.QueryRowContext(ctx, query, version).Scan(
		&theme.Version,
		&theme.PublishedAt,
		&publishedByUserID,
		&theme.Files,
		&notes,
		&theme.CreatedAt,
		&theme.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("blog theme not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get blog theme: %w", err)
	}

	// Convert sql.NullString to *string
	if notes.Valid {
		theme.Notes = &notes.String
	}
	if publishedByUserID.Valid {
		theme.PublishedByUserID = &publishedByUserID.String
	}

	return &theme, nil
}

// GetPublishedTheme retrieves the currently published blog theme
func (r *blogThemeRepository) GetPublishedTheme(ctx context.Context) (*domain.BlogTheme, error) {
	workspaceID, ok := ctx.Value("workspace_id").(string)
	if !ok {
		return nil, fmt.Errorf("workspace_id not found in context")
	}

	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	query := `
		SELECT version, published_at, published_by_user_id, files, notes, created_at, updated_at
		FROM blog_themes
		WHERE published_at IS NOT NULL
	`

	var theme domain.BlogTheme
	var notes sql.NullString
	var publishedByUserID sql.NullString
	err = workspaceDB.QueryRowContext(ctx, query).Scan(
		&theme.Version,
		&theme.PublishedAt,
		&publishedByUserID,
		&theme.Files,
		&notes,
		&theme.CreatedAt,
		&theme.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("no published blog theme found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get published blog theme: %w", err)
	}

	// Convert sql.NullString to *string
	if notes.Valid {
		theme.Notes = &notes.String
	}
	if publishedByUserID.Valid {
		theme.PublishedByUserID = &publishedByUserID.String
	}

	return &theme, nil
}

// GetPublishedThemeTx retrieves the currently published blog theme within a transaction
func (r *blogThemeRepository) GetPublishedThemeTx(ctx context.Context, tx *sql.Tx) (*domain.BlogTheme, error) {
	query := `
		SELECT version, published_at, published_by_user_id, files, notes, created_at, updated_at
		FROM blog_themes
		WHERE published_at IS NOT NULL
	`

	var theme domain.BlogTheme
	var notes sql.NullString
	var publishedByUserID sql.NullString
	err := tx.QueryRowContext(ctx, query).Scan(
		&theme.Version,
		&theme.PublishedAt,
		&publishedByUserID,
		&theme.Files,
		&notes,
		&theme.CreatedAt,
		&theme.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("no published blog theme found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get published blog theme: %w", err)
	}

	// Convert sql.NullString to *string
	if notes.Valid {
		theme.Notes = &notes.String
	}
	if publishedByUserID.Valid {
		theme.PublishedByUserID = &publishedByUserID.String
	}

	return &theme, nil
}

// UpdateTheme updates an existing blog theme
func (r *blogThemeRepository) UpdateTheme(ctx context.Context, theme *domain.BlogTheme) error {
	workspaceID, ok := ctx.Value("workspace_id").(string)
	if !ok {
		return fmt.Errorf("workspace_id not found in context")
	}

	return r.WithTransaction(ctx, workspaceID, func(tx *sql.Tx) error {
		return r.UpdateThemeTx(ctx, tx, theme)
	})
}

// UpdateThemeTx updates an existing blog theme within a transaction
func (r *blogThemeRepository) UpdateThemeTx(ctx context.Context, tx *sql.Tx, theme *domain.BlogTheme) error {
	theme.UpdatedAt = time.Now().UTC()

	query := `
		UPDATE blog_themes
		SET files = $1, notes = $2, updated_at = $3
		WHERE version = $4 AND published_at IS NULL
	`

	result, err := tx.ExecContext(ctx, query,
		theme.Files,
		theme.Notes,
		theme.UpdatedAt,
		theme.Version,
	)
	if err != nil {
		return fmt.Errorf("failed to update blog theme: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("blog theme not found or already published")
	}

	return nil
}

// PublishTheme publishes a blog theme by version
func (r *blogThemeRepository) PublishTheme(ctx context.Context, version int, publishedByUserID string) error {
	workspaceID, ok := ctx.Value("workspace_id").(string)
	if !ok {
		return fmt.Errorf("workspace_id not found in context")
	}

	return r.WithTransaction(ctx, workspaceID, func(tx *sql.Tx) error {
		return r.PublishThemeTx(ctx, tx, version, publishedByUserID)
	})
}

// PublishThemeTx publishes a blog theme by version within a transaction
func (r *blogThemeRepository) PublishThemeTx(ctx context.Context, tx *sql.Tx, version int, publishedByUserID string) error {
	// First, verify the theme exists
	_, err := r.GetThemeTx(ctx, tx, version)
	if err != nil {
		return err
	}

	// Unpublish all themes
	_, err = tx.ExecContext(ctx, `
		UPDATE blog_themes
		SET published_at = NULL, published_by_user_id = NULL, updated_at = $1
		WHERE published_at IS NOT NULL
	`, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("failed to unpublish existing themes: %w", err)
	}

	// Publish the target theme
	now := time.Now().UTC()
	result, err := tx.ExecContext(ctx, `
		UPDATE blog_themes
		SET published_at = $1, published_by_user_id = $2, updated_at = $3
		WHERE version = $4
	`, now, publishedByUserID, now, version)
	if err != nil {
		return fmt.Errorf("failed to publish blog theme: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("blog theme not found")
	}

	return nil
}

// ListThemes retrieves blog themes with pagination
func (r *blogThemeRepository) ListThemes(ctx context.Context, params domain.ListBlogThemesRequest) (*domain.BlogThemeListResponse, error) {
	workspaceID, ok := ctx.Value("workspace_id").(string)
	if !ok {
		return nil, fmt.Errorf("workspace_id not found in context")
	}

	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	// Count total
	var totalCount int
	err = workspaceDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM blog_themes").Scan(&totalCount)
	if err != nil {
		return nil, fmt.Errorf("failed to count blog themes: %w", err)
	}

	// Get themes with pagination
	query := `
		SELECT version, published_at, published_by_user_id, files, notes, created_at, updated_at
		FROM blog_themes
		ORDER BY version DESC
		LIMIT $1 OFFSET $2
	`

	rows, err := workspaceDB.QueryContext(ctx, query, params.Limit, params.Offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list blog themes: %w", err)
	}
	defer rows.Close()

	var themes []*domain.BlogTheme
	for rows.Next() {
		var theme domain.BlogTheme
		var notes sql.NullString
		var publishedByUserID sql.NullString
		err := rows.Scan(
			&theme.Version,
			&theme.PublishedAt,
			&publishedByUserID,
			&theme.Files,
			&notes,
			&theme.CreatedAt,
			&theme.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan blog theme: %w", err)
		}
		// Convert sql.NullString to *string
		if notes.Valid {
			theme.Notes = &notes.String
		}
		if publishedByUserID.Valid {
			theme.PublishedByUserID = &publishedByUserID.String
		}
		themes = append(themes, &theme)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating blog themes: %w", err)
	}

	return &domain.BlogThemeListResponse{
		Themes:     themes,
		TotalCount: totalCount,
	}, nil
}

