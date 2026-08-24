package service

import (
	"context"
	"fmt"

	"github.com/Notifuse/notifuse/internal/domain"
	"github.com/Notifuse/notifuse/pkg/analytics"
	"github.com/Notifuse/notifuse/pkg/logger"
)

// AnalyticsService handles analytics operations
type AnalyticsService struct {
	repo        domain.AnalyticsRepository
	authService domain.AuthService
	logger      logger.Logger
}

// NewAnalyticsService creates a new analytics service
func NewAnalyticsService(
	repo domain.AnalyticsRepository,
	authService domain.AuthService,
	logger logger.Logger,
) *AnalyticsService {
	return &AnalyticsService{
		repo:        repo,
		authService: authService,
		logger:      logger,
	}
}

// Ensure AnalyticsService implements the interface
var _ domain.AnalyticsService = (*AnalyticsService)(nil)

// Query executes an analytics query for a workspace
func (s *AnalyticsService) Query(ctx context.Context, workspaceID string, query analytics.Query) (*analytics.Response, error) {
	// Authenticate user and verify they have access to the workspace
	ctx, user, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		s.logger.WithField("workspace_id", workspaceID).WithField("error", err.Error()).Error("Failed to authenticate user for analytics query")
		return nil, fmt.Errorf("failed to authenticate user: %w", err)
	}

	// A query reads a table, so it costs the read grant on the resource that owns
	// that table — web analytics carries visitor-level data, message_history
	// carries recipient identities, and so on. Workspace membership alone must not
	// be enough to read any of them through this endpoint.
	//
	// An unmapped schema is refused rather than served: falling through would make
	// a schema added later readable by every member, which is the exact gap this
	// mapping closes.
	resource, mapped := domain.AnalyticsSchemaResource(query.Schema)
	if !mapped {
		return nil, domain.NewPermissionError(
			"",
			domain.PermissionTypeRead,
			fmt.Sprintf("Insufficient permissions: schema %s cannot be queried", query.Schema),
		)
	}
	if !userWorkspace.HasPermission(resource, domain.PermissionTypeRead) {
		return nil, domain.NewPermissionError(
			resource,
			domain.PermissionTypeRead,
			fmt.Sprintf("Insufficient permissions: read access to %s required", resource),
		)
	}

	// Get schemas for validation
	schemas, err := s.repo.GetSchemas(ctx, workspaceID)
	if err != nil {
		s.logger.WithField("workspace_id", workspaceID).
			WithField("user_id", user.ID).
			WithField("error", err.Error()).
			Error("Failed to get schemas for validation")
		return nil, fmt.Errorf("failed to get schemas: %w", err)
	}

	// Validate the query using default validation
	if err := analytics.DefaultValidate(query, schemas); err != nil {
		s.logger.WithField("workspace_id", workspaceID).
			WithField("user_id", user.ID).
			WithField("error", err.Error()).
			Error("Analytics query validation failed")
		return nil, fmt.Errorf("query validation failed: %w", err)
	}

	// Execute the query through the repository
	response, err := s.repo.Query(ctx, workspaceID, query)
	if err != nil {
		s.logger.WithField("workspace_id", workspaceID).
			WithField("user_id", user.ID).
			WithField("error", err.Error()).
			Error("Failed to execute analytics query")
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}

	// Time series gap filling is now handled in the analytics package during row scanning

	return response, nil
}

// GetSchemas returns the available analytics schemas for a workspace
func (s *AnalyticsService) GetSchemas(ctx context.Context, workspaceID string) (map[string]analytics.SchemaDefinition, error) {
	// Authenticate user and verify they have access to the workspace
	ctx, user, userWorkspace, err := s.authService.AuthenticateUserForWorkspace(ctx, workspaceID)
	if err != nil {
		s.logger.WithField("workspace_id", workspaceID).WithField("error", err.Error()).Error("Failed to authenticate user for schemas request")
		return nil, fmt.Errorf("failed to authenticate user: %w", err)
	}

	// Get schemas from repository
	schemas, err := s.repo.GetSchemas(ctx, workspaceID)
	if err != nil {
		s.logger.WithField("workspace_id", workspaceID).
			WithField("user_id", user.ID).
			WithField("error", err.Error()).
			Error("Failed to get analytics schemas")
		return nil, fmt.Errorf("failed to get schemas: %w", err)
	}

	// The catalogue answers with what the caller could actually query, using the
	// same mapping Query gates on — an unmapped schema is listed to nobody. This
	// degrades rather than refusing: the metadata is a measure and dimension
	// listing, so a caller holding one resource still gets a usable catalogue
	// instead of a 403 it cannot act on.
	readable := make(map[string]analytics.SchemaDefinition, len(schemas))
	for name, schema := range schemas {
		resource, mapped := domain.AnalyticsSchemaResource(name)
		if mapped && userWorkspace.HasPermission(resource, domain.PermissionTypeRead) {
			readable[name] = schema
		}
	}

	return readable, nil
}
