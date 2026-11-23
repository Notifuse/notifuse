package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/Notifuse/notifuse/internal/domain"
)

type contactErasureRepository struct {
	workspaceRepo domain.WorkspaceRepository
}

// NewContactErasureRepository creates the PostgreSQL contact erasure repository.
func NewContactErasureRepository(workspaceRepo domain.WorkspaceRepository) domain.ContactErasureRepository {
	return &contactErasureRepository{workspaceRepo: workspaceRepo}
}

// The hash is computed by contact_opt_out_hash() in SQL only, never in Go: Go's
// strings.ToLower and PostgreSQL's lower() can differ, and the restore trigger hashes with
// the SQL function. The rows are deleted and hashed in one statement: the DELETE locks them,
// so an opt-out written meanwhile waits and is then either hashed or never deleted.
const deleteListRowsKeepingOptOutsSQL = `WITH gone AS (
		DELETE FROM contact_lists WHERE email = $1
		RETURNING list_id, status, deleted_at, updated_at
	)
	INSERT INTO contact_opt_out_hashes (email_hash, list_id, status, removed, opted_out_at, recorded_at, source)
	SELECT contact_opt_out_hash($1), gone.list_id, gone.status, gone.deleted_at IS NOT NULL, gone.updated_at, NOW(), $2
	FROM gone
	WHERE gone.status IN ('unsubscribed', 'bounced', 'complained')
	ON CONFLICT (email_hash, list_id) DO UPDATE
	SET status = EXCLUDED.status, removed = EXCLUDED.removed, opted_out_at = EXCLUDED.opted_out_at,
		recorded_at = EXCLUDED.recorded_at, source = EXCLUDED.source
	WHERE EXCLUDED.opted_out_at >= contact_opt_out_hashes.opted_out_at`

func (r *contactErasureRepository) DeleteListRowsKeepingOptOuts(ctx context.Context, workspaceID, email string, source domain.OptOutHashSource) (int64, error) {
	db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return 0, fmt.Errorf("failed to get workspace connection: %w", err)
	}
	result, err := db.ExecContext(ctx, deleteListRowsKeepingOptOutsSQL, email, string(source))
	if err != nil {
		return 0, fmt.Errorf("failed to record opt-out hashes: %w", err)
	}
	return result.RowsAffected()
}

func (r *contactErasureRepository) ForgetOptOutHashes(ctx context.Context, workspaceID, email string) error {
	db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM contact_opt_out_hashes WHERE email_hash = contact_opt_out_hash($1)`, email); err != nil {
		return fmt.Errorf("failed to forget opt-out hashes: %w", err)
	}
	return nil
}

func (r *contactErasureRepository) DeleteJourneysForEmail(ctx context.Context, workspaceID, email string) error {
	db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace connection: %w", err)
	}
	// automation_node_executions cascades from contact_automations. The trigger log goes
	// with the contact: it is what keeps a "once" automation from enrolling them again,
	// and a contact who comes back is a new contact.
	if _, err := db.ExecContext(ctx, `DELETE FROM contact_automations WHERE contact_email = $1`, email); err != nil {
		return fmt.Errorf("failed to delete automation journeys: %w", err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM automation_trigger_log WHERE contact_email = $1`, email); err != nil {
		return fmt.Errorf("failed to delete automation trigger log: %w", err)
	}
	return nil
}

// The address sits at the top level of list, segment and email payloads, under contact for
// contact events, and under custom_event for custom events. The two kinds of event the
// deletion itself sends are kept: contact.deleted, and segment.left for each segment the
// contact leaves, which is how subscribers learn of it.
const deleteWebhookDeliveriesForEmailSQL = `DELETE FROM webhook_deliveries
	WHERE event_type NOT IN ('contact.deleted', 'segment.left')
	AND (payload->>'email' = $1
		OR payload->'contact'->>'email' = $1
		OR payload->'custom_event'->>'email' = $1)`

func (r *contactErasureRepository) DeleteWebhookDeliveriesForEmail(ctx context.Context, workspaceID, email string) (int64, error) {
	db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return 0, fmt.Errorf("failed to get workspace connection: %w", err)
	}
	result, err := db.ExecContext(ctx, deleteWebhookDeliveriesForEmailSQL, email)
	if err != nil {
		return 0, fmt.Errorf("failed to delete webhook deliveries: %w", err)
	}
	return result.RowsAffected()
}

const purgeOptOutHashesSQL = `DELETE FROM contact_opt_out_hashes o
	WHERE o.recorded_at < $1
	OR NOT EXISTS (SELECT 1 FROM lists l WHERE l.id = o.list_id AND l.deleted_at IS NULL)`

func (r *contactErasureRepository) PurgeOptOutHashes(ctx context.Context, workspaceID string, recordedBefore time.Time) (int64, error) {
	db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return 0, fmt.Errorf("failed to get workspace connection: %w", err)
	}
	result, err := db.ExecContext(ctx, purgeOptOutHashesSQL, recordedBefore)
	if err != nil {
		return 0, fmt.Errorf("failed to purge opt-out hashes: %w", err)
	}
	return result.RowsAffected()
}
