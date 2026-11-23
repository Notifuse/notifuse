package migrations

import (
	"context"
	"fmt"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/database/schema"
	"github.com/Notifuse/notifuse/internal/domain"
)

// V45Migration is the one migration of 45.0. Version 44.1 was never released: 45.0 ships its
// work together with several projects that each add their part here (error phases and
// CapacityExhausted, DKIM signing, MCP access, data retention, two-factor authentication).
// This file starts empty; each project adds its system or workspace part in its own pull
// request, and turns on HasSystemUpdate or HasWorkspaceUpdate when it does.
//
// The dispatcher runs only migrations above the database's version and then stamps it
// (manager.go, RunMigrations), so a database that already ran a 45.0 build from dev never
// runs a part added to this file afterwards. Until 45.0 is released:
//
//   - every part must be safe to run again on a database that already has it
//     (IF NOT EXISTS, CREATE OR REPLACE, guarded DO blocks, updates that rewrite nothing the
//     second time);
//   - after a part is merged, each database already stamped 45 (the preview, local
//     development databases) is put back to 44 so that it runs the whole of v45 again:
//     UPDATE settings SET value = '44', updated_at = CURRENT_TIMESTAMP WHERE key = 'db_version' AND value = '45'
//
// Once 45.0 is released this file is frozen like every shipped migration: a fix to a
// database at 45 is a new major.
//
// Parts so far:
//
//   - two-factor authentication (system): the user_totp and user_recovery_codes tables, the
//     columns of user_sessions that record how a session was opened and its pending
//     challenge. The DDL is schema.TwoFactorTableDefinitions, shared verbatim with
//     internal/database/init.go.
//   - data retention, Lot 0 (workspace): the opt-out hashes of deleted contacts, their
//     restore trigger and the bodies that honour them (v45_data_retention.go).
//   - data retention, Lot 1 (system, after the two-factor tables): one engagement backfill
//     task per existing workspace (v45_data_retention.go).
type V45Migration struct{}

func (m *V45Migration) GetMajorVersion() float64 { return 45.0 }

func (m *V45Migration) HasSystemUpdate() bool { return true }

func (m *V45Migration) HasWorkspaceUpdate() bool { return true }

func (m *V45Migration) ShouldRestartServer() bool { return false }

func (m *V45Migration) UpdateSystem(ctx context.Context, cfg *config.Config, db DBExecutor) error {
	for _, query := range schema.TwoFactorTableDefinitions() {
		if _, err := db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("v45: failed to create the two-factor authentication tables: %w", err)
		}
	}
	// Data retention, Lot 1: one engagement backfill task per existing workspace
	// (v45_data_retention.go).
	if err := applyDataRetentionSystemV45(ctx, db); err != nil {
		return err
	}
	// MCP access: the tables of the OAuth authorization server (v45_oauth.go).
	if err := applyOAuthSystemV45(ctx, db); err != nil {
		return err
	}
	return nil
}

func (m *V45Migration) UpdateWorkspace(ctx context.Context, cfg *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	// Data retention, Lot 0: the opt-out hashes of deleted contacts and their restore
	// trigger, and the bodies that honour them (v45_data_retention.go).
	if err := applyDataRetentionWorkspaceV45(ctx, db); err != nil {
		return err
	}
	// MCP access: external_id made unique, so that transactional.send deduplicates
	// concurrent calls too (v45_external_id.go).
	if err := applyExternalIDWorkspaceV45(ctx, db); err != nil {
		return err
	}
	return nil
}

func init() {
	Register(&V45Migration{})
}
