package migrations

import (
	"context"
	"fmt"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
)

// V35Migration repairs automation "add to list" nodes that stored the invalid
// status value 'subscribed' instead of the canonical 'active'.
//
// The v26 migration performed the same repair, but the console kept emitting
// 'subscribed' for new/edited add_to_list nodes afterwards, so automations built
// since then reintroduced the invalid value. This migration re-runs the value
// repair (paired with the console fix that now stores 'active') and recomputes
// automation stats so dashboards reflect the corrected data.
//
// It also adds per-link click tracking storage and opts the built-in Supabase
// auth notifications out of click tracking.
//
// This migration:
//  1. Updates contact_lists records with status='subscribed' -> 'active'
//  2. Updates automation nodes with status='subscribed' in their config -> 'active'
//  3. Updates contact_timeline changes JSON containing 'subscribed' -> 'active'
//  4. Recomputes automation stats from actual contact_automations data
//  5. Adds the message_history.clicked_links JSONB column, a per-message map of
//     clicked destination URL -> {count, first_at, last_at} that powers the
//     per-link click breakdown in broadcast reports
//  6. Sets tracking_mode "disabled" on the Supabase transactional notifications
//     so their one-time auth links (magic link, recovery, invite, ...) are no
//     longer rewritten through the click-tracking redirect
type V35Migration struct{}

func (m *V35Migration) GetMajorVersion() float64 {
	return 35.0
}

func (m *V35Migration) HasSystemUpdate() bool {
	return false
}

func (m *V35Migration) HasWorkspaceUpdate() bool {
	return true
}

func (m *V35Migration) ShouldRestartServer() bool {
	return false
}

func (m *V35Migration) UpdateSystem(ctx context.Context, cfg *config.Config, db DBExecutor) error {
	return nil
}

func (m *V35Migration) UpdateWorkspace(ctx context.Context, cfg *config.Config, workspace *domain.Workspace, db DBExecutor) error {
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
		-- Guard on jsonb_typeof = 'array': a nil Nodes slice is marshalled to the
		-- JSON scalar 'null' (not SQL NULL and not '[]'), and jsonb_array_elements
		-- raises "cannot extract elements from a scalar" on it. This predicate also
		-- excludes SQL NULL, so the previous "nodes IS NOT NULL" check is unneeded.
		WHERE jsonb_typeof(nodes) = 'array'
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

	// Step 5: Add the clicked_links column (nullable, no default: instant, no
	// table rewrite; existing rows read as SQL NULL until their first click)
	_, err = db.ExecContext(ctx, `
		ALTER TABLE message_history ADD COLUMN IF NOT EXISTS clicked_links JSONB
	`)
	if err != nil {
		return fmt.Errorf("failed to add clicked_links column: %w", err)
	}

	// Step 6: Opt the Supabase auth notifications out of tracking (tracking_mode
	// "disabled" is the tri-state's full veto: no redirect, no pixel, no UTM).
	// The '_' in the LIKE pattern is escaped so only real 'supabase_*' IDs match
	// (a bare '_' is a single-character wildcard), and integration_id IS NOT NULL
	// restricts the fix to integration-created notifications — a user-created
	// notification that merely has a supabase_-style ID must not be opted out.
	// The jsonb_typeof guard protects against jsonb-null (or other
	// non-object) tracking_settings values: '||' on a non-object degrades to
	// array concatenation, which would corrupt the value and break the
	// notification's Scan. Such rows are left untouched.
	_, err = db.ExecContext(ctx, `
		UPDATE transactional_notifications
		SET tracking_settings = (COALESCE(tracking_settings, '{}'::jsonb) - 'disable_tracking') || '{"tracking_mode": "disabled"}'::jsonb
		WHERE id LIKE 'supabase\_%'
		  AND integration_id IS NOT NULL
		  AND (tracking_settings IS NULL OR jsonb_typeof(tracking_settings) = 'object')
	`)
	if err != nil {
		return fmt.Errorf("failed to update supabase notification tracking settings: %w", err)
	}

	return nil
}

func init() {
	Register(&V35Migration{})
}
