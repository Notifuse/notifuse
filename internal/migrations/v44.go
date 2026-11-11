package migrations

import (
	"context"
	"fmt"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/database/schema"
	"github.com/Notifuse/notifuse/internal/domain"
)

// V44Migration makes a click on a link write its own timeline entry, on workspaces that
// already exist.
//
// track_message_history_changes() is reinstalled from the shared schema package. It now
// writes an email.link_clicked entry, with the URL, each time a click records a link of
// clicked_links through a verified token for the first time, so an automation can start on
// a click of a given link and a segment filtering on a clicked link is recomputed at once.
// See schema.TrackMessageHistoryChangesFunction.
//
// Replacing a function body takes no table lock, and message_history_changes_trigger stays
// attached. No column is renamed or dropped, so no automation trigger function needs
// regenerating. Clicks recorded before the upgrade are not replayed: inserting their entries
// here would start automations for old clicks, inside one transaction.
type V44Migration struct{}

func (m *V44Migration) GetMajorVersion() float64 { return 44.0 }

func (m *V44Migration) HasSystemUpdate() bool { return false }

func (m *V44Migration) HasWorkspaceUpdate() bool { return true }

func (m *V44Migration) ShouldRestartServer() bool { return false }

func (m *V44Migration) UpdateSystem(ctx context.Context, cfg *config.Config, db DBExecutor) error {
	return nil
}

func (m *V44Migration) UpdateWorkspace(ctx context.Context, cfg *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	if _, err := db.ExecContext(ctx, schema.TrackMessageHistoryChangesFunction()); err != nil {
		return fmt.Errorf("v44: failed to install the link click timeline step for workspace %s: %w", workspace.ID, err)
	}
	return nil
}

func init() {
	Register(&V44Migration{})
}
