package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/Notifuse/notifuse/internal/domain"
)

type templateRepository struct {
	workspaceRepo domain.WorkspaceRepository
}

// NewTemplateRepository creates a new PostgreSQL template repository
func NewTemplateRepository(workspaceRepo domain.WorkspaceRepository) domain.TemplateRepository {
	return &templateRepository{
		workspaceRepo: workspaceRepo,
	}
}

func (r *templateRepository) CreateTemplate(ctx context.Context, workspaceID string, template *domain.Template) error {
	// Get the workspace database connection
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}

	now := time.Now().UTC()
	template.CreatedAt = now
	template.UpdatedAt = now

	// Ensure version is at least 1 for creation
	if template.Version == 0 {
		template.Version = 1
	}

	query := `
		INSERT INTO templates (
			id, 
			name, 
			version, 
			channel, 
			email,
			web, 
			category, 
			template_macro_id, 
			integration_id,
			test_data, 
			settings, 
			created_at, 
			updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`
	_, err = workspaceDB.ExecContext(ctx, query,
		template.ID,
		template.Name,
		template.Version,
		template.Channel,
		template.Email,
		template.Web,
		template.Category,
		template.TemplateMacroID,
		template.IntegrationID,
		template.TestData,
		template.Settings,
		template.CreatedAt,
		template.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to create template: %w", err)
	}
	return nil
}

func (r *templateRepository) GetTemplateByID(ctx context.Context, workspaceID string, id string, version int64) (*domain.Template, error) {
	// Get the workspace database connection
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	var query string
	var args []interface{}

	if version > 0 {
		// Get specific version
		query = `
			SELECT 
				id, 
				name, 
				version, 
				channel, 
				email,
				web, 
				category, 
				template_macro_id, 
				integration_id,
				test_data, 
				settings, 
				created_at, 
				updated_at
			FROM templates
			WHERE id = $1 AND version = $2
		`
		args = []interface{}{id, version}
	} else {
		// Get latest version
		query = `
			SELECT 
				id, 
				name, 
				version, 
				channel, 
				email,
				web, 
				category, 
				template_macro_id, 
				integration_id,
				test_data, 
				settings, 
				created_at, 
				updated_at
			FROM templates
			WHERE id = $1
			ORDER BY version DESC
			LIMIT 1
		`
		args = []interface{}{id}
	}

	row := workspaceDB.QueryRowContext(ctx, query, args...)

	template, err := scanTemplate(row)
	if err == sql.ErrNoRows {
		return nil, &domain.ErrTemplateNotFound{Message: "template not found"}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get template: %w", err)
	}

	return template, nil
}

func (r *templateRepository) GetTemplateLatestVersion(ctx context.Context, workspaceID string, id string) (int64, error) {
	// Get the workspace database connection
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return 0, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	query := `
		SELECT MAX(version) 
		FROM templates
		WHERE id = $1
	`

	var version int64
	err = workspaceDB.QueryRowContext(ctx, query, id).Scan(&version)
	if err == sql.ErrNoRows {
		return 0, &domain.ErrTemplateNotFound{Message: "template not found"}
	}
	if err != nil {
		return 0, fmt.Errorf("failed to get template latest version: %w", err)
	}

	return version, nil
}

func (r *templateRepository) GetTemplates(ctx context.Context, workspaceID string, category string, channel string) ([]*domain.Template, error) {
	// Get the workspace database connection
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace connection: %w", err)
	}

	// Get only the latest version of each template
	latestVersionsCTE := `
		WITH latest_versions AS (
			SELECT id, MAX(version) as max_version
			FROM templates
			GROUP BY id
		)
	`

	psql := sq.StatementBuilder.PlaceholderFormat(sq.Dollar)

	selectBuilder := psql.Select(
		"t.id",
		"t.name",
		"t.version",
		"t.channel",
		"t.email",
		"t.web",
		"t.category",
		"t.template_macro_id",
		"t.integration_id",
		"t.test_data",
		"t.settings",
		"t.created_at",
		"t.updated_at",
	).Prefix(latestVersionsCTE).
		From("templates t").
		Join("latest_versions lv ON t.id = lv.id AND t.version = lv.max_version").
		Where(sq.Eq{"t.deleted_at": nil}).
		OrderBy("t.updated_at DESC")

	if category != "" {
		selectBuilder = selectBuilder.Where(sq.Eq{"t.category": category})
	}

	if channel != "" {
		selectBuilder = selectBuilder.Where(sq.Eq{"t.channel": channel})
	}

	query, args, err := selectBuilder.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build query: %w", err)
	}

	rows, err := workspaceDB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get templates: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var templates []*domain.Template
	for rows.Next() {
		template, err := scanTemplate(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan template: %w", err)
		}
		templates = append(templates, template)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating template rows: %w", err)
	}

	return templates, nil
}

func (r *templateRepository) UpdateTemplate(ctx context.Context, workspaceID string, template *domain.Template) error {
	// Get the workspace database connection
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}

	// Get the latest version
	latestVersion, err := r.GetTemplateLatestVersion(ctx, workspaceID, template.ID)
	if err != nil {
		return fmt.Errorf("failed to get template latest version: %w", err)
	}

	// Increment version
	template.Version = latestVersion + 1
	template.UpdatedAt = time.Now().UTC()

	// Create a new version instead of updating the existing one
	query := `
		INSERT INTO templates (
			id, 
			name, 
			version, 
			channel, 
			email,
			web, 
			category, 
			template_macro_id, 
			integration_id,
			test_data, 
			settings, 
			created_at, 
			updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`
	_, err = workspaceDB.ExecContext(ctx, query,
		template.ID,
		template.Name,
		template.Version,
		template.Channel,
		template.Email,
		template.Web,
		template.Category,
		template.TemplateMacroID,
		template.IntegrationID,
		template.TestData,
		template.Settings,
		template.CreatedAt,
		template.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to update template: %w", err)
	}

	return nil
}

func (r *templateRepository) DeleteTemplate(ctx context.Context, workspaceID string, id string) error {
	// Get the workspace database connection
	workspaceDB, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}

	// Soft delete by setting deleted_at
	query := `UPDATE templates SET deleted_at = NOW() WHERE id = $1 AND deleted_at IS NULL`

	result, err := workspaceDB.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete template: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get affected rows: %w", err)
	}

	if rows == 0 {
		return &domain.ErrTemplateNotFound{Message: "template not found"}
	}

	return nil
}

// scanTemplate scans a template from a database row
func scanTemplate(scanner interface {
	Scan(dest ...interface{}) error
}) (*domain.Template, error) {
	var (
		template        domain.Template
		templateMacroID sql.NullString
		integrationID   sql.NullString
	)

	err := scanner.Scan(
		&template.ID,
		&template.Name,
		&template.Version,
		&template.Channel,
		&template.Email,
		&template.Web,
		&template.Category,
		&templateMacroID,
		&integrationID,
		&template.TestData,
		&template.Settings,
		&template.CreatedAt,
		&template.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	// Handle nullable fields
	if templateMacroID.Valid {
		template.TemplateMacroID = &templateMacroID.String
	}
	if integrationID.Valid {
		template.IntegrationID = &integrationID.String
	}

	return &template, nil
}
