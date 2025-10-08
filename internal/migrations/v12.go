package migrations

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
)

// V12Migration implements the migration from version 11.x to 12.0
// Sets default rate limit on all email provider integrations
type V12Migration struct{}

// GetMajorVersion returns the major version this migration handles
func (m *V12Migration) GetMajorVersion() float64 {
	return 12.0
}

// HasSystemUpdate indicates if this migration has system-level changes
func (m *V12Migration) HasSystemUpdate() bool {
	return true // Updates workspace integrations in system database
}

// HasWorkspaceUpdate indicates if this migration has workspace-level changes
func (m *V12Migration) HasWorkspaceUpdate() bool {
	return false // No workspace database changes needed
}

// workspaceIntegrationData holds workspace data for migration
type workspaceIntegrationData struct {
	ID                  string
	Integrations        []domain.Integration
	UpdatedIntegrations []byte
	NeedsUpdate         bool
}

// UpdateSystem executes system-level migration changes
// Adds default rate_limit_per_minute to all email provider integrations
func (m *V12Migration) UpdateSystem(ctx context.Context, cfg *config.Config, db DBExecutor) error {
	// Query all workspaces from the system database
	rows, err := db.QueryContext(ctx, "SELECT id, integrations FROM workspaces")
	if err != nil {
		return fmt.Errorf("failed to query workspaces: %w", err)
	}
	defer rows.Close()

	// Read all workspaces into memory first to avoid "unexpected Parse response" error
	// (PostgreSQL doesn't allow executing queries while reading from another query on the same connection)
	var workspacesToUpdate []workspaceIntegrationData

	for rows.Next() {
		var workspaceID string
		var integrationsData []byte

		if err := rows.Scan(&workspaceID, &integrationsData); err != nil {
			return fmt.Errorf("failed to scan workspace row: %w", err)
		}

		// Parse integrations
		var integrations []domain.Integration
		if len(integrationsData) > 0 {
			if err := json.Unmarshal(integrationsData, &integrations); err != nil {
				return fmt.Errorf("failed to unmarshal integrations for workspace %s: %w", workspaceID, err)
			}
		}

		// Check if workspace has any integrations
		if len(integrations) == 0 {
			// No integrations to migrate
			continue
		}

		// Track if we made any changes
		madeChanges := false

		// Iterate through all integrations
		for i := range integrations {
			integration := &integrations[i]

			// Only process email integrations
			if integration.Type != domain.IntegrationTypeEmail {
				continue
			}

			// Check if rate limit is already set
			if integration.EmailProvider.RateLimitPerMinute > 0 {
				// Already has a rate limit, skip
				continue
			}

			// Set default rate limit
			integration.EmailProvider.RateLimitPerMinute = 25
			madeChanges = true
		}

		// If we made changes, prepare the update
		if madeChanges {
			// Serialize integrations to JSON
			integrationsJSON, err := json.Marshal(integrations)
			if err != nil {
				return fmt.Errorf("failed to marshal integrations for workspace %s: %w", workspaceID, err)
			}

			workspacesToUpdate = append(workspacesToUpdate, workspaceIntegrationData{
				ID:                  workspaceID,
				Integrations:        integrations,
				UpdatedIntegrations: integrationsJSON,
				NeedsUpdate:         true,
			})
		}
	}

	// Check for any errors during iteration
	if err := rows.Err(); err != nil {
		return fmt.Errorf("error iterating workspace rows: %w", err)
	}

	// Close rows before executing updates
	rows.Close()

	// Now execute all updates
	for _, workspace := range workspacesToUpdate {
		_, err = db.ExecContext(ctx, `
			UPDATE workspaces
			SET integrations = $1, updated_at = NOW()
			WHERE id = $2
		`, workspace.UpdatedIntegrations, workspace.ID)
		if err != nil {
			return fmt.Errorf("failed to update workspace integrations for workspace %s: %w", workspace.ID, err)
		}
	}

	return nil
}

// UpdateWorkspace executes workspace-level migration changes (none for v12)
func (m *V12Migration) UpdateWorkspace(ctx context.Context, cfg *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	return nil
}

// init registers this migration with the default registry
func init() {
	Register(&V12Migration{})
}
