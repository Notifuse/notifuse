package repository

import (
	"context"
	"fmt"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/analytics"
	"github.com/Notifuse/notifuse/pkg/logger"
)

type analyticsRepository struct {
	workspaceRepo domain.WorkspaceRepository
	sqlBuilder    *analytics.SQLBuilder
	logger        logger.Logger
}

// NewAnalyticsRepository creates a new PostgreSQL analytics repository
func NewAnalyticsRepository(workspaceRepo domain.WorkspaceRepository, logger logger.Logger) domain.AnalyticsRepository {
	return &analyticsRepository{
		workspaceRepo: workspaceRepo,
		sqlBuilder:    analytics.NewSQLBuilder(),
		logger:        logger,
	}
}

// Query executes an analytics query and returns the results
func (r *analyticsRepository) Query(ctx context.Context, workspaceID string, query analytics.Query) (*analytics.Response, error) {
	// Validate the query using predefined schemas
	schema, exists := domain.PredefinedSchemas[query.Schema]
	if !exists {
		r.logger.WithField("schema", query.Schema).WithField("workspace_id", workspaceID).Error("Unknown schema in analytics query")
		return nil, fmt.Errorf("unknown schema: %s", query.Schema)
	}

	// Create schema map for validation
	schemas := map[string]analytics.SchemaDefinition{query.Schema: schema}

	// Validate the query against the schema
	if err := analytics.DefaultValidate(query, schemas); err != nil {
		r.logger.WithField("workspace_id", workspaceID).WithField("error", err.Error()).Error("Analytics query validation failed")
		return nil, fmt.Errorf("query validation failed: %w", err)
	}

	// Generate SQL using the SQL builder
	sql, args, err := r.sqlBuilder.BuildSQL(query, schema)
	if err != nil {
		r.logger.WithField("workspace_id", workspaceID).WithField("error", err.Error()).Error("Failed to build SQL for analytics query")
		return nil, fmt.Errorf("failed to build SQL: %w", err)
	}

	// Get workspace database connection
	db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		r.logger.WithField("workspace_id", workspaceID).WithField("error", err.Error()).Error("Failed to get workspace database connection")
		return nil, fmt.Errorf("failed to get database connection: %w", err)
	}

	// Execute the query
	rows, err := db.QueryContext(ctx, sql, args...)
	if err != nil {
		r.logger.WithField("workspace_id", workspaceID).WithField("sql", sql).WithField("error", err.Error()).Error("Failed to execute analytics query")
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}
	defer rows.Close()

	// Get column information
	columns, err := rows.Columns()
	if err != nil {
		r.logger.WithField("workspace_id", workspaceID).WithField("error", err.Error()).Error("Failed to get query columns")
		return nil, fmt.Errorf("failed to get columns: %w", err)
	}

	// Parse results
	var data []map[string]interface{}
	for rows.Next() {
		// Create a slice of interface{} to hold the values
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		// Scan the row
		if err := rows.Scan(valuePtrs...); err != nil {
			r.logger.WithField("workspace_id", workspaceID).WithField("error", err.Error()).Error("Failed to scan query result row")
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		// Convert to map
		row := make(map[string]interface{})
		for i, col := range columns {
			val := values[i]
			// Convert []byte to string for better JSON serialization
			if b, ok := val.([]byte); ok {
				val = string(b)
			}
			row[col] = val
		}
		data = append(data, row)
	}

	// Check for iteration errors
	if err := rows.Err(); err != nil {
		r.logger.WithField("workspace_id", workspaceID).WithField("error", err.Error()).Error("Error during query result iteration")
		return nil, fmt.Errorf("error during result iteration: %w", err)
	}

	// Create response with debug information
	response := &analytics.Response{
		Data: data,
		Meta: analytics.Meta{
			Query:  sql,
			Params: args,
		},
	}

	r.logger.WithField("workspace_id", workspaceID).WithField("schema", query.Schema).WithField("rows", len(data)).Info("Analytics query executed successfully")

	return response, nil
}

// GetSchemas returns the available predefined schemas
func (r *analyticsRepository) GetSchemas(ctx context.Context, workspaceID string) (map[string]analytics.SchemaDefinition, error) {
	// For now, return all predefined schemas
	// In the future, this could be filtered based on workspace permissions or available tables
	schemas := make(map[string]analytics.SchemaDefinition)
	for name, schema := range domain.PredefinedSchemas {
		schemas[name] = schema
	}

	r.logger.WithField("workspace_id", workspaceID).WithField("schema_count", len(schemas)).Info("Retrieved analytics schemas")

	return schemas, nil
}
