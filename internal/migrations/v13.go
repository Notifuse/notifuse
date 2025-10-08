package migrations

import (
	"context"
	"fmt"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
)

// V13Migration implements the migration from version 12.x to 13.0
// Adds recompute_after column to segments table for daily segment recomputation
// Creates permanent task for each workspace to check for segments due for recomputation
type V13Migration struct{}

// GetMajorVersion returns the major version this migration handles
func (m *V13Migration) GetMajorVersion() float64 {
	return 13.0
}

// HasSystemUpdate indicates if this migration has system-level changes
func (m *V13Migration) HasSystemUpdate() bool {
	return true
}

// HasWorkspaceUpdate indicates if this migration has workspace-level changes
func (m *V13Migration) HasWorkspaceUpdate() bool {
	return true
}

// UpdateSystem executes system-level migration changes
func (m *V13Migration) UpdateSystem(ctx context.Context, config *config.Config, db DBExecutor) error {
	// Get all workspaces to create segment recompute tasks
	rows, err := db.QueryContext(ctx, `SELECT id FROM workspaces`)
	if err != nil {
		return fmt.Errorf("failed to query workspaces: %w", err)
	}
	defer rows.Close()

	// Collect all workspace IDs first to avoid query conflicts
	var workspaceIDs []string
	for rows.Next() {
		var workspaceID string
		if err := rows.Scan(&workspaceID); err != nil {
			return fmt.Errorf("failed to scan workspace ID: %w", err)
		}
		workspaceIDs = append(workspaceIDs, workspaceID)
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("error iterating workspaces: %w", err)
	}

	// Now process each workspace
	for _, workspaceID := range workspaceIDs {
		// Create permanent task for checking segments that need recomputation
		// This task will run every ~10 minutes to check for segments with recompute_after <= now
		// Check if task already exists first
		var exists bool
		err = db.QueryRowContext(ctx, `
			SELECT EXISTS(SELECT 1 FROM tasks WHERE workspace_id = $1 AND type = 'check_segment_recompute')
		`, workspaceID).Scan(&exists)
		if err != nil {
			return fmt.Errorf("failed to check existing task for workspace %s: %w", workspaceID, err)
		}

		// Only create if it doesn't exist
		if !exists {
			_, err = db.ExecContext(ctx, `
				INSERT INTO tasks (id, workspace_id, type, status, next_run_after, max_runtime, max_retries, retry_interval, progress, state, created_at, updated_at)
				VALUES (
					gen_random_uuid(),
					$1,
					'check_segment_recompute',
					'pending',
					CURRENT_TIMESTAMP,
					50,
					3,
					60,
					0,
					'{"message": "Check segments for daily recompute"}'::jsonb,
					CURRENT_TIMESTAMP,
					CURRENT_TIMESTAMP
				)
			`, workspaceID)
			if err != nil {
				return fmt.Errorf("failed to create segment recompute task for workspace %s: %w", workspaceID, err)
			}
		}
	}

	return nil
}

// UpdateWorkspace executes workspace-level migration changes
func (m *V13Migration) UpdateWorkspace(ctx context.Context, config *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	// Add recompute_after column to segments table
	_, err := db.ExecContext(ctx, `
		ALTER TABLE segments
		ADD COLUMN IF NOT EXISTS recompute_after TIMESTAMP NULL
	`)
	if err != nil {
		return fmt.Errorf("failed to add recompute_after column to segments table for workspace %s: %w", workspace.ID, err)
	}

	return nil
}

// init registers this migration with the default registry
func init() {
	Register(&V13Migration{})
}
