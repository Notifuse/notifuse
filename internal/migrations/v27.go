package migrations

import (
	"context"
	"fmt"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
)

// V27Migration adds the data_feed column to the broadcasts table.
//
// This migration adds support for external data feed integration in broadcasts:
// - data_feed: JSONB column containing DataFeedSettings (global_feed, global_feed_data, global_feed_fetched_at, recipient_feed)
type V27Migration struct{}

func (m *V27Migration) GetMajorVersion() float64 {
	return 27.0
}

func (m *V27Migration) HasSystemUpdate() bool {
	return false
}

func (m *V27Migration) HasWorkspaceUpdate() bool {
	return true
}

func (m *V27Migration) ShouldRestartServer() bool {
	return false
}

func (m *V27Migration) UpdateSystem(ctx context.Context, cfg *config.Config, db DBExecutor) error {
	return nil
}

func (m *V27Migration) UpdateWorkspace(ctx context.Context, cfg *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	// Add data_feed column (JSONB for DataFeedSettings - contains global_feed, global_feed_data, global_feed_fetched_at, recipient_feed)
	_, err := db.ExecContext(ctx, `
		ALTER TABLE broadcasts
		ADD COLUMN IF NOT EXISTS data_feed JSONB DEFAULT NULL
	`)
	if err != nil {
		return fmt.Errorf("failed to add data_feed column: %w", err)
	}

	return nil
}

func init() {
	Register(&V27Migration{})
}
