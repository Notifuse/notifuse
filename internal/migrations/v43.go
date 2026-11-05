package migrations

import (
	"context"
	"fmt"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/database/schema"
	"github.com/Notifuse/notifuse/internal/domain"
)

// V43Migration adds per-contact consent to open and click tracking, on
// workspaces that already exist.
//
//  1. track_contact_changes() and webhook_contacts_trigger() are reinstalled
//     from the shared schema package, so a change of tracking_opens_consent,
//     tracking_clicks_consent or tracking_consent_source writes a
//     contact.updated timeline row and sends a contact.updated webhook. The
//     triggers stay attached: replacing a function body takes no table lock.
//  2. message_history gains opens_tracked and clicks_tracked: what each email
//     actually carried, so a rate can be computed over the tracked sends. NULL
//     on every existing row, which reads as tracked, so the rates of a
//     broadcast sent before 43 do not move.
//  3. contacts gains the two consents, their date and their source. NULL is an
//     unknown consent, which the workspace decides on.
//  4. broadcasts gains tracking_mode ("" or no_tracking).
//
// All columns are nullable with no default, so ADD COLUMN is a catalog change
// and rewrites no table. The ALTERs take ACCESS EXCLUSIVE on their table, held
// until commit; lock_timeout makes a busy table fail the migration (retried on
// the next start) rather than queue every write behind it. message_history is
// altered before contacts, the order in which its status trigger locks them.
//
// No automation trigger function needs regenerating: no column is renamed or
// dropped, and none of them can reference a column that did not exist.
type V43Migration struct{}

func (m *V43Migration) GetMajorVersion() float64 { return 43.0 }

func (m *V43Migration) HasSystemUpdate() bool { return false }

func (m *V43Migration) HasWorkspaceUpdate() bool { return true }

func (m *V43Migration) ShouldRestartServer() bool { return false }

func (m *V43Migration) UpdateSystem(ctx context.Context, cfg *config.Config, db DBExecutor) error {
	return nil
}

// v43WorkspaceStatements is every statement the workspace update runs, in order.
func v43WorkspaceStatements() []string {
	return []string{
		`SET LOCAL lock_timeout = '5s'`,
		schema.TrackContactChangesFunction(),
		schema.WebhookContactsTriggerFunction(),
		`ALTER TABLE message_history
			ADD COLUMN IF NOT EXISTS opens_tracked BOOLEAN,
			ADD COLUMN IF NOT EXISTS clicks_tracked BOOLEAN`,
		`ALTER TABLE contacts
			ADD COLUMN IF NOT EXISTS tracking_opens_consent VARCHAR(10),
			ADD COLUMN IF NOT EXISTS tracking_clicks_consent VARCHAR(10),
			ADD COLUMN IF NOT EXISTS tracking_consent_updated_at TIMESTAMPTZ,
			ADD COLUMN IF NOT EXISTS tracking_consent_source VARCHAR(50)`,
		`ALTER TABLE broadcasts
			ADD COLUMN IF NOT EXISTS tracking_mode VARCHAR(20)`,
	}
}

func (m *V43Migration) UpdateWorkspace(ctx context.Context, cfg *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	for _, statement := range v43WorkspaceStatements() {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("v43: failed to add tracking consent for workspace %s: %w", workspace.ID, err)
		}
	}
	return nil
}

func init() {
	Register(&V43Migration{})
}
