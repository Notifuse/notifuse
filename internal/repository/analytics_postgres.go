package repository

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/analytics"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// workMemRe whitelists the values accepted for SET LOCAL work_mem.
var workMemRe = regexp.MustCompile(`^[0-9]{1,6}(kB|MB|GB)$`)

type analyticsRepository struct {
	workspaceRepo domain.WorkspaceRepository
	logger        logger.Logger
	// workMem, when valid, is applied via SET LOCAL for each analytics query
	// so percentile sorts over large ranges don't spill to temp files.
	workMem string
}

// NewAnalyticsRepository creates a new PostgreSQL analytics repository.
// workMem is the per-query working memory (e.g. "64MB"); empty or invalid
// values disable the override.
func NewAnalyticsRepository(workspaceRepo domain.WorkspaceRepository, logger logger.Logger, workMem string) domain.AnalyticsRepository {
	if workMem != "" && !workMemRe.MatchString(workMem) {
		logger.WithField("work_mem", workMem).Error("Invalid ANALYTICS_WORK_MEM value; ignoring")
		workMem = ""
	}
	return &analyticsRepository{
		workspaceRepo: workspaceRepo,
		logger:        logger,
		workMem:       workMem,
	}
}

// resolveSchemas merges the static predefined schemas with the workspace's
// web analytics schemas (which carry per-workspace bounce threshold and
// custom dimension titles).
func (r *analyticsRepository) resolveSchemas(ctx context.Context, workspaceID string, timezone string) map[string]analytics.SchemaDefinition {
	var webAnalytics *domain.WebAnalyticsSettings
	workspace, err := r.workspaceRepo.GetByID(ctx, workspaceID)
	if err != nil {
		r.logger.WithField("workspace_id", workspaceID).WithField("error", err.Error()).Error("Failed to load workspace for schema resolution")
	} else if workspace != nil {
		webAnalytics = workspace.Settings.WebAnalytics
	}
	return domain.ResolveAnalyticsSchemas(webAnalytics, timezone)
}

// Query executes an analytics query and returns the results
func (r *analyticsRepository) Query(ctx context.Context, workspaceID string, query analytics.Query) (*analytics.Response, error) {
	schemas := r.resolveSchemas(ctx, workspaceID, query.GetDefaultTimezone())
	schema, exists := schemas[query.Schema]
	if !exists {
		r.logger.WithField("schema", query.Schema).WithField("workspace_id", workspaceID).Error("Unknown schema in analytics query")
		return nil, fmt.Errorf("unknown schema: %s", query.Schema)
	}

	// Get workspace database connection
	db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		r.logger.WithField("workspace_id", workspaceID).WithField("error", err.Error()).Error("Failed to get workspace database connection")
		return nil, fmt.Errorf("failed to get database connection: %w", err)
	}

	response, err := r.executeQuery(ctx, db, query, schema)
	if err != nil {
		r.logger.WithField("workspace_id", workspaceID).WithField("error", err.Error()).Error("Failed to execute analytics query")
		return nil, fmt.Errorf("failed to execute analytics query: %w", err)
	}

	return response, nil
}

// executeQuery runs the query, inside a read-only transaction with a work_mem
// override when configured (SET LOCAL requires a transaction).
func (r *analyticsRepository) executeQuery(ctx context.Context, db *sql.DB, query analytics.Query, schema analytics.SchemaDefinition) (*analytics.Response, error) {
	if r.workMem == "" {
		return query.Query(ctx, db, schema)
	}

	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("failed to begin analytics transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// r.workMem is whitelisted at construction; SET LOCAL takes no bind params.
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("SET LOCAL work_mem = '%s'", r.workMem)); err != nil {
		return nil, fmt.Errorf("failed to set work_mem: %w", err)
	}

	response, err := query.Query(ctx, tx, schema)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit analytics transaction: %w", err)
	}
	return response, nil
}

// GetSchemas returns the schemas available to the workspace.
func (r *analyticsRepository) GetSchemas(ctx context.Context, workspaceID string) (map[string]analytics.SchemaDefinition, error) {
	// The catalog describes what can be queried, not one query's output, so
	// its dimension expressions are the UTC ones.
	return r.resolveSchemas(ctx, workspaceID, "UTC"), nil
}
