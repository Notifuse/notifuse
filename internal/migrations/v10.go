package migrations

import (
	"context"
	"fmt"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
)

// V10Migration implements the migration from version 9.x to 10.0
// Adds list_ids column to message_history and creates trigger to auto-update contact_lists on complaint/bounce
type V10Migration struct{}

// GetMajorVersion returns the major version this migration handles
func (m *V10Migration) GetMajorVersion() float64 {
	return 10.0
}

// HasSystemUpdate indicates if this migration has system-level changes
func (m *V10Migration) HasSystemUpdate() bool {
	return false // No system-level changes needed
}

// HasWorkspaceUpdate indicates if this migration has workspace-level changes
func (m *V10Migration) HasWorkspaceUpdate() bool {
	return true // Adds column and triggers to workspace databases
}

// ShouldRestartServer indicates if the server should restart after this migration
func (m *V10Migration) ShouldRestartServer() bool {
	return false
}

// UpdateSystem executes system-level migration changes (none for v10)
func (m *V10Migration) UpdateSystem(ctx context.Context, config *config.Config, db DBExecutor) error {
	return nil
}

// UpdateWorkspace executes workspace-level migration changes
func (m *V10Migration) UpdateWorkspace(ctx context.Context, config *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	// 1. Add list_ids column to message_history
	_, err := db.ExecContext(ctx, `
		ALTER TABLE message_history
		ADD COLUMN IF NOT EXISTS list_ids TEXT[]
	`)
	if err != nil {
		return fmt.Errorf("failed to add list_ids column to message_history for workspace %s: %w", workspace.ID, err)
	}

	// 2. Backfill list_ids from broadcasts
	// Only include lists where the contact is actually a member
	_, err = db.ExecContext(ctx, `
		UPDATE message_history mh
		SET list_ids = ARRAY(
			SELECT cl.list_id
			FROM contact_lists cl
			WHERE cl.email = mh.contact_email
			AND cl.list_id IN (
				SELECT jsonb_array_elements_text(b.audience->'lists')
				FROM broadcasts b
				WHERE b.id = mh.broadcast_id
			)
			AND cl.deleted_at IS NULL
		)
		WHERE mh.broadcast_id IS NOT NULL
		AND mh.list_ids IS NULL
	`)
	if err != nil {
		return fmt.Errorf("failed to backfill list_ids for workspace %s: %w", workspace.ID, err)
	}

	// 3. Update contact_lists status for historical complaint events
	_, err = db.ExecContext(ctx, `
		UPDATE contact_lists cl
		SET status = 'complained',
			updated_at = mh.complained_at
		FROM message_history mh
		WHERE cl.email = mh.contact_email
		AND cl.list_id = ANY(mh.list_ids)
		AND mh.complained_at IS NOT NULL
		AND mh.list_ids IS NOT NULL
		AND array_length(mh.list_ids, 1) > 0
		AND cl.status != 'complained'
	`)
	if err != nil {
		return fmt.Errorf("failed to update contact_lists for historical complaints in workspace %s: %w", workspace.ID, err)
	}

	// 4. Update contact_lists status for historical hard bounce events
	_, err = db.ExecContext(ctx, `
		UPDATE contact_lists cl
		SET status = 'bounced',
			updated_at = mh.bounced_at
		FROM message_history mh
		WHERE cl.email = mh.contact_email
		AND cl.list_id = ANY(mh.list_ids)
		AND mh.bounced_at IS NOT NULL
		AND mh.list_ids IS NOT NULL
		AND array_length(mh.list_ids, 1) > 0
		AND cl.status NOT IN ('complained', 'bounced')
	`)
	if err != nil {
		return fmt.Errorf("failed to update contact_lists for historical bounces in workspace %s: %w", workspace.ID, err)
	}

	// 5. Create trigger function to update contact_lists on complaint/bounce (for future events)
	_, err = db.ExecContext(ctx, `
		CREATE OR REPLACE FUNCTION update_contact_lists_on_status_change()
		RETURNS TRIGGER AS $$
		BEGIN
			-- Handle complaint events (worst status - can upgrade from any status)
			IF NEW.complained_at IS NOT NULL AND OLD.complained_at IS NULL THEN
				IF NEW.list_ids IS NOT NULL AND array_length(NEW.list_ids, 1) > 0 THEN
					UPDATE contact_lists
					SET status = 'complained',
						updated_at = NEW.complained_at
					WHERE email = NEW.contact_email
					AND list_id = ANY(NEW.list_ids)
					AND status != 'complained';
				END IF;
			END IF;

			-- Handle bounce events (ONLY HARD BOUNCES - can only update if not already complained or bounced)
			-- Note: Application layer should only set bounced_at for hard/permanent bounces
			IF NEW.bounced_at IS NOT NULL AND OLD.bounced_at IS NULL THEN
				IF NEW.list_ids IS NOT NULL AND array_length(NEW.list_ids, 1) > 0 THEN
					UPDATE contact_lists
					SET status = 'bounced',
						updated_at = NEW.bounced_at
					WHERE email = NEW.contact_email
					AND list_id = ANY(NEW.list_ids)
					AND status NOT IN ('complained', 'bounced');
				END IF;
			END IF;

			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql
	`)
	if err != nil {
		return fmt.Errorf("failed to create trigger function for workspace %s: %w", workspace.ID, err)
	}

	// 6. Create trigger
	_, err = db.ExecContext(ctx, `
		DROP TRIGGER IF EXISTS message_history_status_trigger ON message_history;

		CREATE TRIGGER message_history_status_trigger
		AFTER UPDATE ON message_history
		FOR EACH ROW
		EXECUTE FUNCTION update_contact_lists_on_status_change()
	`)
	if err != nil {
		return fmt.Errorf("failed to create trigger for workspace %s: %w", workspace.ID, err)
	}

	return nil
}

// init registers this migration with the default registry
func init() {
	Register(&V10Migration{})
}
