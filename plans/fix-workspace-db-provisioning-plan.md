# Workspace database provisioning plan

**Status**: plan reviewed by Gemini, implemented
**Date**: 2026-10-23
**Target**: 42.0 (unreleased). No migration, no table or environment variable change, no version bump.
**Base**: `origin/dev` at `df2d701`.

## Goal

A workspace database is created only for a workspace that exists, and its name is built, checked and
quoted in one place.

## Changes

1. **One name helper**, `database.WorkspaceDatabaseName(prefix, workspaceID)` (`internal/database/utils.go`):
   hyphens become underscores as before; the workspace ID part may only hold ASCII letters, digits and
   underscores, at most 63 (a bound on arbitrary input only), else `invalid workspace ID for a database
   name: "<id>"`; then a name with an ASCII capital is refused as before (`workspace database name must be
   lowercase: <name>`). Every workspace ID a create accepts already passes (lowercase alphanumeric, at most
   20 characters), and an ID with other characters never had a working database.
2. **One DSN builder**: `GetWorkspaceDSN` returns `(string, error)` through the helper, and the connection
   manager and the migration connector use it instead of building their own.
3. **Quoted create**: `EnsureWorkspaceDatabaseExists` creates `CREATE DATABASE <pq.QuoteIdentifier(name)>`.
   The name is lowercase, so the database gets the same name as before.
4. **Delete**: `DeleteDatabase` drops the helper's name as written (it no longer lowercases it with Unicode
   rules, which missed a prefix with a non-ASCII capital) and refuses a name the helper refuses.
5. **Connection manager**: `GetWorkspaceConnection` checks the ID with the helper before taking capacity or
   opening a connection; when the database is missing, it is created again only if the `workspaces` table
   has the workspace (`domain.ErrWorkspaceNotFound` otherwise). A cached pool is returned without the check, as it was checked when created.

## Tests (written first, red before the change)

- `internal/database/utils_test.go`: `TestWorkspaceDatabaseName` (accepted names, ASCII capitals, refused
  IDs including a 64-character one); `TestGetWorkspaceDSN`, `TestConnectToWorkspace` and
  `TestEnsureWorkspaceDatabaseExists` refuse an ID the helper refuses before connecting.
- `pkg/database/connection_manager_test.go`: a refused ID takes no capacity and evicts no idle pool;
  `checkWorkspaceRegistered` (found, not found, query error).
- `internal/repository/workspace_database_test.go`: the drop uses the name as written; a refused name runs
  no statement.
- `internal/migrations/manager_test.go`: the migration connector refuses an ID before connecting.
- `tests/integration/workspace_database_provisioning_test.go`: the app's connection manager creates no database
  for an unregistered ID, creates and initialises the missing database of a registered one, and refuses an ID
  the helper refuses.
- `tests/integration/email_handler_test.go`: the legacy tracking URLs answer as before for a workspace
  that does not exist, and no workspace database appears.

Test harness: `tests/testutil/server.go` resets the connection manager singleton before each test app
initializes, so every app's manager uses that app's system database (it kept the first app's, closed, which
the registration check would read). Tested by `TestWorkspaceDatabaseProvisioning`, which starts and shuts
down a first app, then checks the second app's own manager.

## Not in scope

Creating and initialising a workspace database atomically; checking `DB_NAME` and `DB_PREFIX` at
startup.

## Test commands

`make test-database`, `go test ./pkg/database/...`, `make test-repo`, `make test-migrations`,
`make test-unit`, `make test-integration`.
