package migrations

import (
	"context"
	"fmt"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
)

// V26Migration fixes invalid 'subscribed' status values and recomputes automation stats.
//
// Bugs fixed:
// 1. Automation add_to_list nodes used 'subscribed' instead of 'active' status
// 2. Automation stats could be reset to 0 when updating automation
//
// This migration:
// 1. Updates contact_lists records with status='subscribed' -> 'active'
// 2. Updates automation nodes with status='subscribed' in their config -> 'active'
// 3. Updates contact_timeline changes JSON containing 'subscribed' -> 'active'
// 4. Recomputes automation stats from actual contact_automations data
type V26Migration struct{}

func (m *V26Migration) GetMajorVersion() float64 {
	return 26.0
}

func (m *V26Migration) HasSystemUpdate() bool {
	return false
}

func (m *V26Migration) HasWorkspaceUpdate() bool {
	return true
}

func (m *V26Migration) ShouldRestartServer() bool {
	return false
}

func (m *V26Migration) UpdateSystem(ctx context.Context, cfg *config.Config, db DBExecutor) error {
	return nil
}

func (m *V26Migration) UpdateWorkspace(ctx context.Context, cfg *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	// Step 1: Fix contact_lists with invalid 'subscribed' status
	_, err := db.ExecContext(ctx, `
		UPDATE contact_lists
		SET status = 'active', updated_at = NOW()
		WHERE status = 'subscribed'
	`)
	if err != nil {
		return fmt.Errorf("failed to update contact_lists status: %w", err)
	}

	// Step 2: Fix automation nodes with 'subscribed' status in config
	// Only updates automations that have add_to_list nodes with subscribed status
	_, err = db.ExecContext(ctx, `
		UPDATE automations
		SET nodes = (
			SELECT COALESCE(jsonb_agg(
				CASE
					WHEN node->>'type' = 'add_to_list' AND node->'config'->>'status' = 'subscribed'
					THEN jsonb_set(node, '{config,status}', '"active"')
					ELSE node
				END
			), '[]'::jsonb)
			FROM jsonb_array_elements(nodes) AS node
		),
		updated_at = NOW()
		WHERE nodes IS NOT NULL
		AND EXISTS (
			SELECT 1 FROM jsonb_array_elements(nodes) AS node
			WHERE node->>'type' = 'add_to_list'
			AND node->'config'->>'status' = 'subscribed'
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to update automation nodes: %w", err)
	}

	// Step 3: Fix contact_timeline entries with 'subscribed' in changes JSON
	_, err = db.ExecContext(ctx, `
		UPDATE contact_timeline
		SET changes = (
			CASE
				WHEN changes->'status'->>'old' = 'subscribed' AND changes->'status'->>'new' = 'subscribed'
				THEN jsonb_set(jsonb_set(changes, '{status,old}', '"active"'), '{status,new}', '"active"')
				WHEN changes->'status'->>'old' = 'subscribed'
				THEN jsonb_set(changes, '{status,old}', '"active"')
				WHEN changes->'status'->>'new' = 'subscribed'
				THEN jsonb_set(changes, '{status,new}', '"active"')
				ELSE changes
			END
		)
		WHERE entity_type = 'contact_list'
		AND (changes->'status'->>'old' = 'subscribed' OR changes->'status'->>'new' = 'subscribed')
	`)
	if err != nil {
		return fmt.Errorf("failed to update contact_timeline changes: %w", err)
	}

	// Step 4: Recompute automation stats from actual contact_automations data
	_, err = db.ExecContext(ctx, `
		UPDATE automations a
		SET stats = COALESCE((
			SELECT jsonb_build_object(
				'enrolled', COUNT(*),
				'completed', COUNT(*) FILTER (WHERE ca.status = 'completed'),
				'exited', COUNT(*) FILTER (WHERE ca.status = 'exited'),
				'failed', COUNT(*) FILTER (WHERE ca.status = 'failed')
			)
			FROM contact_automations ca
			WHERE ca.automation_id = a.id
		), '{"enrolled":0,"completed":0,"exited":0,"failed":0}'::jsonb),
		updated_at = NOW()
		WHERE deleted_at IS NULL
	`)
	if err != nil {
		return fmt.Errorf("failed to recompute automation stats: %w", err)
	}

	return nil
}

func init() {
	Register(&V26Migration{})
}
