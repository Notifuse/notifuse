package migrations

import (
	"context"

	"github.com/Notifuse/notifuse/config"
	"github.com/Notifuse/notifuse/internal/domain"
)

// V40Migration changes no schema. It exists so the database version stamp can
// follow the code version across the licence change, and that is not a
// formality — without it the stamp never moves.
//
// Manager.RunMigrations writes the new version with SetCurrentDBVersion only
// AFTER the migration loop, and the loop is preceded by an early return:
//
//	if len(migrationsToRun) == 0 { log("No migrations to run"); return nil }
//
// The selection above it is `migrationVersion > currentDBVersion &&
// migrationVersion <= currentCodeVersion`. So a v40 build against a database
// stamped 39 with no migration registered at 40 selects nothing, returns
// through that branch, and never reaches the write. Every boot repeats it, and
// the database stays stamped 39 for the life of the release — reporting a
// version this build is not.
//
// That is survivable until it is not. The stamp is a single scalar, and the
// same `>` selection means a later v41 running against a still-39 database
// would be the thing that finally moves it — so the gap closes silently and
// nobody ever learns the stamp was wrong. It also disarms the guard directly
// above: `currentDBVersion > currentCodeVersion` is what refuses to boot a
// rollback against a newer schema, and a stamp frozen a major behind is a stamp
// that cannot trip it.
//
// Both halves are declared false and both methods are no-ops. The dispatcher
// asks a migration which databases it wants and connects to nothing else, so
// this costs one empty transaction against the system database and no
// workspace connections at all.
//
// v40 is the release that moves the Licensed Work from AGPL-3.0-or-later to
// BSL 1.1 (see LICENSE). Nothing about that licence is enforced here or
// anywhere in the migration system: the gates read an Ed25519-signed key at
// runtime, and no schema, column or row is involved.
type V40Migration struct{}

func (m *V40Migration) GetMajorVersion() float64 { return 40.0 }

func (m *V40Migration) HasSystemUpdate() bool { return false }

func (m *V40Migration) HasWorkspaceUpdate() bool { return false }

func (m *V40Migration) ShouldRestartServer() bool { return false }

func (m *V40Migration) UpdateSystem(ctx context.Context, cfg *config.Config, db DBExecutor) error {
	return nil
}

func (m *V40Migration) UpdateWorkspace(ctx context.Context, cfg *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	return nil
}

func init() {
	Register(&V40Migration{})
}
