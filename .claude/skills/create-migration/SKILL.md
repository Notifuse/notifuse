---
name: create-migration
description: Create a Notifuse database migration — bump VERSION in config/config.go, add a vN.go migration in internal/migrations/ implementing MajorMigrationInterface, update CHANGELOG.md, and test it. Use for any system or workspace database schema change.
---

# Creating a database migration

Notifuse manages one system database plus one database per workspace. The migration system compares the code version (`VERSION` in `config/config.go`) with the database version on startup, runs pending system migrations in a transaction, then connects to each workspace database and runs pending workspace migrations, and finally records the new version.

## Process

1. **Update version**: increment the major version in `config/config.go` (`VERSION = "N.0"`). Major = schema changes; minor = everything else.
2. **Create migration file**: new file in `internal/migrations/` (e.g. `vN.go`).
3. **Implement the interface**:

```go
type MajorMigrationInterface interface {
    GetMajorVersion() float64                    // e.g. 7.0
    HasSystemUpdate() bool                       // touches system database
    HasWorkspaceUpdate() bool                    // touches workspace databases
    UpdateSystem(ctx context.Context, config *config.Config, db DBExecutor) error
    UpdateWorkspace(ctx context.Context, config *config.Config, workspace *domain.Workspace, db DBExecutor) error
}
```

4. **Register it** via `init()` in the same file.
5. **Update `CHANGELOG.md`** — document the schema change; call out breaking changes for upgrade planning.
6. **Test**: `make test-migrations`, plus integration tests when the change affects runtime behavior.

## Example

```go
// internal/migrations/v7.go
package migrations

import (
    "context"

    "github.com/Notifuse/notifuse/config"
    "github.com/Notifuse/notifuse/internal/domain"
)

type V7Migration struct{}

func (m *V7Migration) GetMajorVersion() float64 { return 7.0 }
func (m *V7Migration) HasSystemUpdate() bool    { return true }
func (m *V7Migration) HasWorkspaceUpdate() bool { return false }

func (m *V7Migration) UpdateSystem(ctx context.Context, config *config.Config, db DBExecutor) error {
    _, err := db.ExecContext(ctx, `
        CREATE TABLE IF NOT EXISTS new_feature (
            id VARCHAR(32) PRIMARY KEY,
            name VARCHAR(255) NOT NULL,
            created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
        )
    `)
    return err
}

func (m *V7Migration) UpdateWorkspace(ctx context.Context, config *config.Config, workspace *domain.Workspace, db DBExecutor) error {
    return nil
}

func init() {
    Register(&V7Migration{})
}
```

## Safety rules

- **Idempotent**: use `IF NOT EXISTS` / `ADD COLUMN IF NOT EXISTS` so migrations can run more than once safely.
- **Never amend an already-shipped `vN.go` to fix a database at N.** "Runs more than once safely" is
  true of the SQL and false of the dispatcher: the manager selects
  `migrationVersion > currentDBVersion` (`internal/migrations/manager.go:151`) and records only the
  major integer. So editing `vN.go` reaches fresh installs and every database *below* N, and **never**
  the databases already at N — which are exactly the ones a fix is usually for. It fails silently:
  no error, no skipped-migration log, just a step that never runs where it was needed.
  The fix is always a new major: bump `config/config.go` VERSION and add `vN+1.go`.
- **A new table must be added in TWO places.** The migration `vN.go` is what existing databases run;
  `internal/database/schema/*_tables.go` is what a *fresh install* runs. Write only the migration and
  every new install lacks the table; write only the schema file and every upgrade lacks it. Neither
  mistake fails at boot — it surfaces later as a missing-relation error on whichever population you
  did not cover. Keep the two SQL texts byte-identical so they cannot drift.
- **Statements that resolve a table via `::regclass` must run after that table exists.** A guard like
  `conrelid = 'my_table'::regclass` is evaluated when the statement is planned, so placing it before
  the `CREATE TABLE` in the same list throws `relation does not exist` rather than skipping politely.
- **Transactional**: each migration runs in a transaction; failures roll back automatically.
- **Backward compatible**: new columns get defaults so existing data keeps working.
- Keep each migration focused on a single schema change; test against a copy of production data when feasible.
