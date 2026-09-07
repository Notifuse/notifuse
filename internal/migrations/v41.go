package migrations

import (
	"context"
	"fmt"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/database/schema"
	"github.com/Notifuse/notifuse/internal/domain"
)

// V41Migration creates the audit log: the audit_logs table, its indexes, the
// append-only guard trigger and the purge functions, in the SYSTEM database
// only. Workspace databases are untouched — audit rows are keyed by
// workspace_id in the system database precisely so that deleting a workspace
// (which drops its database) does not delete the record of who deleted it.
//
// The DDL is schema.AuditLogsTableDefinitions, shared verbatim with
// internal/database/init.go: a fresh install and an upgraded one run the same
// statements, so neither can end up with a table the other lacks.
//
// No permission backfill. audit_logs is an opt-in permission resource
// (domain.OptInPermissionResources): an owner grants it per member or per API
// key, and a member whose stored map predates it is simply denied, which is
// the right default for a log. Appending it to every existing map — the v38/v39
// pattern — would have handed every member read access to the audit trail the
// moment they upgraded.
//
// Nothing here consults the licence. Recording is what the licence gates, at
// runtime, in the audit service; the table exists on every deployment so that
// listing, exporting and the retention settings work in every licence state.
type V41Migration struct{}

func (m *V41Migration) GetMajorVersion() float64 { return 41.0 }

func (m *V41Migration) HasSystemUpdate() bool { return true }

func (m *V41Migration) HasWorkspaceUpdate() bool { return false }

func (m *V41Migration) ShouldRestartServer() bool { return false }

func (m *V41Migration) UpdateSystem(ctx context.Context, cfg *config.Config, db DBExecutor) error {
	for _, query := range schema.AuditLogsTableDefinitions() {
		if _, err := db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("v41: failed to create audit_logs in system database: %w", err)
		}
	}
	return nil
}

func (m *V41Migration) UpdateWorkspace(ctx context.Context, cfg *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	return nil
}

func init() {
	Register(&V41Migration{})
}
