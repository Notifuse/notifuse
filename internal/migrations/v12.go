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
	return false // No system-level changes needed
}

// HasWorkspaceUpdate indicates if this migration has workspace-level changes
func (m *V12Migration) HasWorkspaceUpdate() bool {
	return true // Updates workspace integrations
}

// UpdateSystem executes system-level migration changes (none for v12)
func (m *V12Migration) UpdateSystem(ctx context.Context, cfg *config.Config, db DBExecutor) error {
	return nil
}

// UpdateWorkspace executes workspace-level migration changes
// Adds default rate_limit_per_minute to all email provider integrations
func (m *V12Migration) UpdateWorkspace(ctx context.Context, cfg *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	// Check if workspace has any integrations
	if len(workspace.Integrations) == 0 {
		// No integrations to migrate
		return nil
	}

	// Track if we made any changes
	madeChanges := false

	// Iterate through all integrations
	for i := range workspace.Integrations {
		integration := &workspace.Integrations[i]

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

	// If we made changes, update the workspace in the database
	if madeChanges {
		// Serialize integrations to JSON
		integrationsJSON, err := json.Marshal(workspace.Integrations)
		if err != nil {
			return fmt.Errorf("failed to marshal integrations: %w", err)
		}

		// Update the workspace integrations in the database
		_, err = db.ExecContext(ctx, `
			UPDATE workspaces
			SET integrations = $1, updated_at = NOW()
			WHERE id = $2
		`, integrationsJSON, workspace.ID)
		if err != nil {
			return fmt.Errorf("failed to update workspace integrations: %w", err)
		}
	}

	return nil
}

// init registers this migration with the default registry
func init() {
	Register(&V12Migration{})
}
