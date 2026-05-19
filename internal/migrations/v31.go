package migrations

import (
	"context"
	"fmt"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
)

// V31Migration breaks the segment-recomputation self-loop.
//
// The previous queue_contact_for_segment_recomputation() unconditionally
// upserted into contact_segment_queue whenever any row was inserted into
// contact_timeline. That included rows produced by track_contact_segment_changes
// (kind='segment.joined' / 'segment.left'), which fire as the queue worker
// itself writes contact_segments. The effect was twofold:
//
//  1. Every membership write re-queued the same contact for re-processing
//     after the 15s debounce — pure wasted work.
//  2. The worker's INSERT contact_segments transaction ended up contending
//     with concurrent /opens and delivery-webhook writers on the same
//     (email) row lock in contact_segment_queue. Under broadcast load this
//     stalled the worker for >10s per batch and caused the queue to
//     accumulate without draining.
//
// The function now short-circuits when the timeline event is itself a
// segment membership change, leaving every other event path unchanged.
type V31Migration struct{}

func (m *V31Migration) GetMajorVersion() float64 {
	return 31.0
}

func (m *V31Migration) HasSystemUpdate() bool {
	return false
}

func (m *V31Migration) HasWorkspaceUpdate() bool {
	return true
}

func (m *V31Migration) ShouldRestartServer() bool {
	return false
}

func (m *V31Migration) UpdateSystem(ctx context.Context, cfg *config.Config, db DBExecutor) error {
	return nil
}

func (m *V31Migration) UpdateWorkspace(ctx context.Context, cfg *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	_, err := db.ExecContext(ctx, `
		CREATE OR REPLACE FUNCTION queue_contact_for_segment_recomputation()
		RETURNS TRIGGER AS $$
		BEGIN
			-- Skip re-queue when the timeline event is itself a segment
			-- membership change. The queue worker writes contact_segments,
			-- which fires track_contact_segment_changes (inserts a
			-- contact_timeline row with kind='segment.joined'/'segment.left'),
			-- which would re-enter this function and re-queue the same
			-- contact for re-processing — a self-loop that also contends
			-- with concurrent open-tracking writes on the contact_segment_queue
			-- (email) row lock and prevents batches from completing.
			IF NEW.kind IN ('segment.joined', 'segment.left') THEN
				RETURN NEW;
			END IF;

			INSERT INTO contact_segment_queue (email, queued_at)
			VALUES (NEW.email, CURRENT_TIMESTAMP)
			ON CONFLICT (email) DO UPDATE SET queued_at = EXCLUDED.queued_at;
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;
	`)
	if err != nil {
		return fmt.Errorf("failed to update queue_contact_for_segment_recomputation function for workspace %s: %w", workspace.ID, err)
	}

	return nil
}

func init() {
	Register(&V31Migration{})
}
