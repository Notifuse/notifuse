package migrations

import (
	"context"
	"fmt"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/database/schema"
	"github.com/Notifuse/notifuse/internal/domain"
)

// V44Migration, on workspaces that already exist, makes a click on a link write its own
// timeline entry, adds the partial indexes of the dashboard's recent failures and dates the
// broadcasts that started without a start date.
//
// track_message_history_changes() is reinstalled from the shared schema package. It now
// writes an email.link_clicked entry, with the URL, each time a click records a link of
// clicked_links through a verified token for the first time, so an automation can start on
// a click of a given link and a segment filtering on a clicked link is recomputed at once.
// See schema.TrackMessageHistoryChangesFunction. Replacing a function body takes no table
// lock, and message_history_changes_trigger stays attached. Clicks recorded before the
// upgrade are not replayed: inserting their entries here would start automations for old
// clicks, inside one transaction.
//
// Three partial indexes then hold the failed, bounced and complained messages in
// messages.list order (created_at DESC, id DESC), so the latest few are read without walking
// the whole table when none matches. The builds are plain CREATE INDEX (the migration runs in
// a transaction) and hold a SHARE lock on message_history until commit: writes to it wait,
// about a second for 2 million rows in memory, longer once the table no longer fits. A rerun
// takes the lock too, even when the indexes exist. statement_timeout is lifted for the
// builds, since a build cut short fails the migration again at every start (see v42).
// lock_timeout bounds the wait for the lock behind a transaction already writing to the
// table, every later write queuing behind that wait: past 5 seconds the migration fails and
// is retried at the next start.
//
// Before the builds, so outside their lock, the broadcasts that started sending without a
// start date get the creation time of their first message as one (v44StartedAtBackfillSQL).
// Until v44 only "send now" and a resume set started_at, so a scheduled broadcast never had
// it; a sending run now records it. Drafts and scheduled broadcasts are left alone even when
// they have messages: those are test sends, or a scheduled send still in progress, which its
// next sending run dates. Only a NULL is filled, so a rerun writes nothing, and a broadcast
// without any message keeps no date. The first messages are found in one grouped pass over
// message_history, not one walk per broadcast.
//
// No column is renamed or dropped, so no automation trigger function needs regenerating.
type V44Migration struct{}

// v44StartedAtBackfillSQL dates each broadcast that sent without a start date from its first
// message.
const v44StartedAtBackfillSQL = `UPDATE broadcasts b SET started_at = m.first_at
FROM (SELECT mh.broadcast_id, MIN(mh.created_at) AS first_at
      FROM message_history mh
      WHERE mh.broadcast_id IN (SELECT id FROM broadcasts
                                WHERE started_at IS NULL AND status NOT IN ('draft', 'scheduled'))
      GROUP BY mh.broadcast_id) m
WHERE b.id = m.broadcast_id AND b.started_at IS NULL AND b.status NOT IN ('draft', 'scheduled')`

func (m *V44Migration) GetMajorVersion() float64 { return 44.0 }

func (m *V44Migration) HasSystemUpdate() bool { return false }

func (m *V44Migration) HasWorkspaceUpdate() bool { return true }

func (m *V44Migration) ShouldRestartServer() bool { return false }

func (m *V44Migration) UpdateSystem(ctx context.Context, cfg *config.Config, db DBExecutor) error {
	return nil
}

// v44WorkspaceStatement is one step of the workspace update, with what its error says.
type v44WorkspaceStatement struct {
	sql  string
	step string
}

// v44WorkspaceStatements are the workspace update's statements, run in this order.
func v44WorkspaceStatements() []v44WorkspaceStatement {
	return []v44WorkspaceStatement{
		{schema.TrackMessageHistoryChangesFunction(), "install the link click timeline step"},
		{`SET LOCAL statement_timeout = 0`, "lift the statement timeout"},
		{`SET LOCAL lock_timeout = '5s'`, "set the lock timeout"},
		{v44StartedAtBackfillSQL, "date the broadcasts that started without a start date"},
		{`CREATE INDEX IF NOT EXISTS idx_message_history_failed_recent ON message_history(created_at DESC, id DESC) WHERE failed_at IS NOT NULL`, "create idx_message_history_failed_recent"},
		{`CREATE INDEX IF NOT EXISTS idx_message_history_bounced_recent ON message_history(created_at DESC, id DESC) WHERE bounced_at IS NOT NULL`, "create idx_message_history_bounced_recent"},
		{`CREATE INDEX IF NOT EXISTS idx_message_history_complained_recent ON message_history(created_at DESC, id DESC) WHERE complained_at IS NOT NULL`, "create idx_message_history_complained_recent"},
	}
}

func (m *V44Migration) UpdateWorkspace(ctx context.Context, cfg *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	for _, stmt := range v44WorkspaceStatements() {
		if _, err := db.ExecContext(ctx, stmt.sql); err != nil {
			return fmt.Errorf("v44: failed to %s for workspace %s: %w", stmt.step, workspace.ID, err)
		}
	}
	return nil
}

func init() {
	Register(&V44Migration{})
}
