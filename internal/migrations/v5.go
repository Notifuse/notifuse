package migrations

import (
	"context"
	"fmt"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
)

// V5Migration implements the migration from version 4.x to 5.0
type V5Migration struct{}

// GetMajorVersion returns the major version this migration handles
func (m *V5Migration) GetMajorVersion() float64 {
	return 5.0
}

// HasSystemUpdate indicates if this migration has system-level changes
func (m *V5Migration) HasSystemUpdate() bool {
	return false
}

// HasWorkspaceUpdate indicates if this migration has workspace-level changes
func (m *V5Migration) HasWorkspaceUpdate() bool {
	return true
}

// UpdateSystem executes system-level migration changes
func (m *V5Migration) UpdateSystem(ctx context.Context, config *config.Config, db DBExecutor) error {
	// No system-level changes for v5
	return nil
}

// UpdateWorkspace executes workspace-level migration changes
func (m *V5Migration) UpdateWorkspace(ctx context.Context, config *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	// Add pause_reason column to broadcasts table
	// Using IF NOT EXISTS to make the migration idempotent
	_, err := db.ExecContext(ctx, `
		ALTER TABLE broadcasts
		ADD COLUMN IF NOT EXISTS pause_reason TEXT
	`)
	if err != nil {
		return fmt.Errorf("failed to add pause_reason column to broadcasts table for workspace %s: %w", workspace.ID, err)
	}

	return nil
}

// init registers this migration with the default registry
func init() {
	Register(&V5Migration{})
}
