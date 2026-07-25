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
- **Transactional**: each migration runs in a transaction; failures roll back automatically.
- **Backward compatible**: new columns get defaults so existing data keeps working.
- Keep each migration focused on a single schema change; test against a copy of production data when feasible.
