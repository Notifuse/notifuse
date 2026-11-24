package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/lib/pq"

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
// custom dimension titles). A workspace that cannot be read fails the call:
// answering without the web schemas would refuse a web analytics query as an
// unknown schema, or list a catalogue without them.
func (r *analyticsRepository) resolveSchemas(ctx context.Context, workspaceID string, timezone string) (map[string]analytics.SchemaDefinition, error) {
	workspace, err := r.workspaceRepo.GetByID(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to load workspace for schema resolution: %w", err)
	}
	var webAnalytics *domain.WebAnalyticsSettings
	if workspace != nil {
		webAnalytics = workspace.Settings.WebAnalytics
	}
	return domain.ResolveAnalyticsSchemas(webAnalytics, timezone), nil
}

// Query executes an analytics query and returns the results. It reads the workspace once to
// resolve the schemas, and the query is validated once, by analytics.Query.Query, against the
// schema it names.
func (r *analyticsRepository) Query(ctx context.Context, workspaceID string, query analytics.Query) (*analytics.Response, error) {
	schemas, err := r.resolveSchemas(ctx, workspaceID, query.GetDefaultTimezone())
	if err != nil {
		r.logger.WithField("workspace_id", workspaceID).WithField("error", err.Error()).Error("Failed to resolve analytics schemas")
		return nil, err
	}
	schema, exists := schemas[query.Schema]
	if !exists {
		return nil, analytics.UnknownSchemaError(query.Schema)
	}

	// Get workspace database connection
	db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		r.logger.WithField("workspace_id", workspaceID).WithField("error", err.Error()).Error("Failed to get workspace database connection")
		return nil, fmt.Errorf("failed to get database connection: %w", err)
	}

	response, err := r.executeQuery(ctx, db, query, schema)
	if err != nil {
		// A refusal is the caller's to fix; the service logs it as a warning.
		var queryErr *analytics.QueryError
		if !errors.As(err, &queryErr) {
			r.logger.WithField("workspace_id", workspaceID).WithField("error", err.Error()).Error("Failed to execute analytics query")
		}
		return nil, fmt.Errorf("failed to execute analytics query: %w", err)
	}

	// The generated SQL and its values never leave the server: they are logged here, at debug
	// level, for whoever investigates a query.
	r.logger.WithField("workspace_id", workspaceID).
		WithField("schema", query.Schema).
		WithField("sql", response.Meta.Query).
		WithField("params", response.Meta.Params).
		Debug("Analytics query executed")

	return response, nil
}

// executeQuery runs the query inside a read-only transaction, which bounds its statement at
// analytics.MaxQueryDuration and applies the work_mem override when configured (SET LOCAL
// requires a transaction). Every caller of the engine, the HTTP API and MCP alike, goes
// through here, so this is the one place the bound is set; MCP adds a shorter one of its own.
func (r *analyticsRepository) executeQuery(ctx context.Context, db *sql.DB, query analytics.Query, schema analytics.SchemaDefinition) (*analytics.Response, error) {
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("failed to begin analytics transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// SET LOCAL takes no bind parameters; the value is a constant.
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("SET LOCAL statement_timeout = %d", analytics.MaxQueryDuration.Milliseconds())); err != nil {
		return nil, fmt.Errorf("failed to set statement_timeout: %w", err)
	}
	if r.workMem != "" {
		// r.workMem is whitelisted at construction.
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("SET LOCAL work_mem = '%s'", r.workMem)); err != nil {
			return nil, fmt.Errorf("failed to set work_mem: %w", err)
		}
	}

	response, err := query.Query(ctx, tx, schema)
	if err != nil {
		return nil, classifyQueryError(err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit analytics transaction: %w", err)
	}
	return response, nil
}

// callerValueErrors are the class 22 errors a value of the caller's query causes: text the
// column's type cannot read (22P02, such as "abc" compared with a number) and a date or time
// that does not parse or does not exist (22007, 22008). Other class 22 errors, such as a
// division by zero or a numeric overflow (22003), come from a schema's own SQL as often as
// from a value, and stay server errors.
var callerValueErrors = []pq.ErrorCode{"22P02", "22007", "22008"}

// classifyQueryError turns the database errors the caller's query causes into an
// analytics.QueryError, which answers as a validation error: a statement past the time bound,
// and a value the database cannot read as its member's type. The message is fixed: the
// database's own text can name the generated SQL's internals. Any other error is ours and
// stays as it is, logged by the caller and answered as a server error.
func classifyQueryError(err error) error {
	var pqErr *pq.Error
	if !errors.As(err, &pqErr) {
		return err
	}
	switch {
	case pqErr.Code == "57014" && strings.Contains(pqErr.Message, "statement timeout"):
		return &analytics.QueryError{Message: fmt.Sprintf("the query ran past %s: narrow its date range or filters", analytics.MaxQueryDuration)}
	case slices.Contains(callerValueErrors, pqErr.Code):
		return &analytics.QueryError{Message: "a value of the query's filters or having does not fit its member's type: check its numbers, dates and times"}
	}
	return err
}

// GetSchemas returns the schemas available to the workspace.
func (r *analyticsRepository) GetSchemas(ctx context.Context, workspaceID string) (map[string]analytics.SchemaDefinition, error) {
	// The catalog describes what can be queried, not one query's output, so
	// its dimension expressions are the UTC ones.
	schemas, err := r.resolveSchemas(ctx, workspaceID, "UTC")
	if err != nil {
		r.logger.WithField("workspace_id", workspaceID).WithField("error", err.Error()).Error("Failed to resolve analytics schemas")
		return nil, err
	}
	return schemas, nil
}
