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
	logger        logger.Logger
}

// NewAnalyticsRepository creates a new PostgreSQL analytics repository
func NewAnalyticsRepository(workspaceRepo domain.WorkspaceRepository, logger logger.Logger) domain.AnalyticsRepository {
	return &analyticsRepository{
		workspaceRepo: workspaceRepo,
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

	// Get workspace database connection
	db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		r.logger.WithField("workspace_id", workspaceID).WithField("error", err.Error()).Error("Failed to get workspace database connection")
		return nil, fmt.Errorf("failed to get database connection: %w", err)
	}

	// Execute the query using the analytics Query method
	response, err := query.Query(ctx, db, schema)
	if err != nil {
		r.logger.WithField("workspace_id", workspaceID).WithField("error", err.Error()).Error("Failed to execute analytics query")
		return nil, fmt.Errorf("failed to execute analytics query: %w", err)
	}

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

	return schemas, nil
}
