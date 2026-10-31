# Workspace database repair and migration lock (part B of 2: second commit of one PR)

**Depends on part A** (`plans/fix-workspace-db-create-recovery-plan.md`), the first commit of the same PR: this plan reuses its name lock (`WithWorkspaceDatabaseLock`, with its lock-transaction rules), `WorkspaceDatabaseHoldsTables`, the transactional `InitializeWorkspaceDatabase`, the migration loop that skips empty workspaces (looking again inside the name lock), the connector with a context and `connect_timeout`, and `Delete` under the name lock (D8, part A's W5). The facts and test-server checks listed in part A apply here and are not repeated; line numbers are at `f635aa09` unless marked "working tree".

**Target**: 42.3, together with part A (Pierre, 2026-10-31: 42.2 ships first; one branch, one PR, two commits; minor; no schema change, no `vN.go`). The release gate (exact-tree CI suites twice, mutation checks of both parts, Gemini review of the whole diff) runs once, after this commit.

Split from part A on 2026-10-31: the background repair is only safe across instances with the migration lock, and neither is needed to stop the startup failure part A fixes. `Delete` under the name lock (D8) was first planned here and moved to part A, which needs it to be safe on its own.

## Problem

After part A:

1. A registered workspace whose database has no tables (left by a failed creation on an earlier version, problem 4 of part A) no longer stops startup, but stays broken: every request touching a table fails, and nothing ever gives it a schema.
2. `RunMigrations` takes no lock (`internal/migrations/manager.go:108-217`): instances starting together each run the same pending migrations in parallel.

Facts specific to this plan:

- The app has a shutdown context (`internal/app/app.go:228`, `:266`, created only in `NewApp`) cancelled first in `Shutdown` (working tree `:1959`, `:1950` at `f635aa09`); `Shutdown` waits only for `requestWg` (working tree `:2059`) and closes `a.db` in `cleanupResources` (working tree `:2141-2150`). The integration harness never calls `app.Start`, so its `Shutdown` takes the `server == nil` early return straight into `cleanupResources`; its `Restart` reuses the already-cancelled `shutdownCtx`. `NewManager` is built inline in `InitDB` (`:366`), whose live half is `codecov:ignore`: no seam for the migration manager in `internal/app`.
- `RunMigrations` gets `context.Background()` (`app.go:367`); the integration harness also calls it (`tests/testutil/database.go:382`) and boots the app through `InitDB` (`tests/testutil/server.go:173`).

## Locks

Both locks are built by part A's lock-transaction helper (detached transaction, caller's context only for the wait, `ctx.Err()` checked once the lock is granted, all four timeouts reset and the TCP keepalives set with `SET LOCAL`, commit error after success logged).

- **Migration lock** (D7): `pg_advisory_xact_lock(<migration key>)` (exclusive) in a transaction on the system database for the whole of `RunMigrations`, on a connection of its own, alongside the per-migration transactions of `executeMigration`, which never take it. `pg_advisory_xact_lock_shared(<migration key>)` is held by the background repair for each workspace **until that workspace's initialisation has committed**. A new key, different from `secretKeyRotationLockID` and part A's name-lock class.
- `RunMigrations` first tries `pg_try_advisory_xact_lock`; when another holder has it, it logs the holders' pids (`pg_locks` joined with `pg_stat_activity`, single-bigint form: `objsubid = 1`, every granted row aggregated), then waits. The lookup failing or finding nobody (the holder may have just released) is only logged. **The wait is unbounded by design** (Pierre, 2026-10-31): an instance must not serve before the migrations are applied, and a legitimate long migration is never interrupted. A holder lost with its node is released once the server notices the dead connection: within about a minute with the helper's keepalives (part A, "The name lock"; checked by the probe described there), instead of about 2 hours with Linux defaults. The log line names the holder if it takes longer (a pooler in between, a Unix socket).
- **Order**: migration lock before name lock, never the reverse. `RunMigrations` takes the name lock (part A's W4) inside the migration lock, which respects that order. **No nesting**, as in part A.
- **42.2 and older instances take neither lock**: during a rolling deploy, the old pods still migrate as before until they are gone. Said in the PR.

## Changes

### W1. Migration lock (`internal/migrations/manager.go`)

- `RunMigrations` begins the migration-lock transaction and takes the exclusive lock **before** reading the version, so a second instance waits for the first and then finds the version already current. Released when `RunMigrations` returns, `ErrRestartRequired` included. Everything else in `RunMigrations` is as part A leaves it.

### W2. Background repair of empty workspaces (`internal/migrations/manager.go`, `internal/app/app.go`)

- New `Manager.InitializeEmptyWorkspaces(ctx, cfg, db)` (D1, D6), started in the background by `app.go` once `RunMigrations` has returned nil, with `a.shutdownCtx`, through a small `startBackground(name, fn)` helper that registers it on a new `a.backgroundWg`. `cleanupResources(ctx)` (reached by both `Shutdown` paths, including the harness's `server == nil` one) waits on it before closing `a.db`, bounded by its `ctx` and, when that has no deadline, by `a.shutdownTimeout` (`a.shutdownCtx` is cancelled at the start of `Shutdown`, so it tells the repair to stop but cannot bound the wait). On `ErrRestartRequired` the process exits as today and the next start runs it. After the harness's `Restart`, `shutdownCtx` is already cancelled, so the repair does not run again: harmless, and no test relies on it.
- For each registered workspace, sequentially:
  - connect and check `WorkspaceDatabaseHoldsTables` within a 10-second deadline (field on the manager so tests can shorten it), without any lock: a database holding tables, almost all of them, costs one connection and nothing more (moved here from the next step after the part B review: the check under the name lock makes the order safe);
  - for a database found empty, begin a transaction holding the **shared** migration lock, and read `db_version`: if it differs from the code version, stop the whole loop (another build has taken over, or a migration is due). The shared lock stays held until this workspace's initialisation has committed, so no migration on any instance can start between the version read and the initialisation: an older pod never writes its schema after a newer one migrated and skipped the empty workspace, and a repaired workspace never receives pending migrations (D3);
  - take the name lock, check again inside it that the workspace is still registered and still empty, and initialise under its own longer bound (5 minutes), not the 10-second one;
  - failures never stop anything: a workspace whose database is unreachable or fails to initialise is skipped with an error log; a missing one is skipped with a warning saying what to do (the same text as the startup error of part A), unless the workspace was deleted since the listing (a probe failure a deletion can cause, a missing database or a terminated connection, is looked at again under the name lock, which waits for a deletion in progress), as today (the connection manager still recreates a missing database of a registered workspace on the first request). Not a surfaced warning: that workspace was already failing on every request, and this step does not change how visible that is. Each initialisation is logged at info with the workspace ID.
- Cost (estimate from a review, not measured): about 10 to 40 seconds at 2,000 workspaces over TLS, off the startup path. The integration harness boots the app per suite, so it runs there too: measure the integration suite's duration before and after.

Known gap, not fixed here: the background repair could initialise an empty database into which an operator is restoring a dump with Notifuse running. Restores are normally done with Notifuse stopped; noted in the PR.

## Decisions (Pierre, 2026-10-31)

- **D1.** Empty registered workspaces are repaired at every startup.
- **D6.** The repair runs in the background, after `RunMigrations`.
- **D7.** A migration lock across instances: exclusive for `RunMigrations`, shared for the repair (held through each initialisation), which also stops when the version is not the code's. Also fixes problem 2. A starting instance waits, without a time limit, for another instance's migrations, and logs the holder's pid while it waits; the lock connection carries server-side TCP keepalives, so a dead holder is released within about a minute (chosen 2026-10-31).
- **D9** (confirmed 2026-10-31): repair failures are never fatal: logged, the instance keeps running, and the workspace stays broken as before; the PR describes it as an unhandled case. With no workspace migration pending, a missing database does not stop startup either.

## Tests (written first)

Unit, `internal/migrations/manager_test.go` and `manager_runwindow_test.go` (every existing `RunMigrations` sqlmock test gains the migration-lock expectations):
- `RunMigrations` tries the exclusive lock, and when it is taken logs each holder's pid and whether it migrates (exclusive) or repairs (shared), then waits, all before reading the version under the lock; the lock transaction is released on every return path, `ErrRestartRequired` included.
- Restart for another instance's migration (`manager_repair_test.go`, added after the part B reviews): the version is read before the lock is tried; a failed read stops startup with that error, and a database already ahead of the code is refused there, without waiting; `RunMigrations` returns `ErrRestartRequired` when the version read under the lock is higher than the first read across a migration that requires a restart (V14) or may (V36, whose answer depends on its own run: `MayRestartServer`), whether the lock was awaited or granted at once; not when the version did not move, moved across neither, or moved down, nor on a first run.
- `InitializeEmptyWorkspaces` (injected initialiser and locks, short deadline): one empty workspace → initialised, registration and emptiness checked again inside the name lock, and the shared migration lock's transaction commits only after the initialisation (sqlmock order); the initialisation runs under its own bound, not the 10-second one; a connection failure on one workspace → the next one still handled, nothing returned as fatal; a database holding tables → no initialisation; `db_version` different from the code version → stops before the first workspace; a connector blocking until its context ends → moves on after the deadline; a cancelled parent context → stops; a missing database → a warning with what to do, without the migration lock, and nothing at all for a workspace deleted or a database created since; any other probe failure → an error, without the name lock; a listing ended by a cancelled context → nothing logged.

Unit, `internal/database/migration_lock_test.go`: the exclusive lock granted at once runs `fn` with no deadline; held elsewhere, the holders (pid, shared or not, application, address) are passed to `waiting` before the blocking wait; a failed holder lookup is passed on and the wait still made; `fn`'s error comes back with a rollback; the wait ends with ctx; the shared lock's work context is bounded; `LockTransactionSettings` returns a copy. `internal/database/workspace_lock_test.go`: a hold of zero sets no hold deadline. `internal/database/init_test.go`: `InitializeWorkspaceDatabaseContext` commits the whole schema in one transaction and runs nothing under a cancelled ctx.

Unit, `internal/app/app_background_test.go`: `startBackground` runs `fn` with `a.shutdownCtx` and registers it on `a.backgroundWg`; `Shutdown` cancels the context and `cleanupResources` waits for a `fn` that blocks until cancellation before closing the database, on both the started-server path and the `server == nil` path; a `fn` that ignores cancellation does not hold `cleanupResources` past the shutdown context. That `InitDB` starts the repair only after `RunMigrations` returned nil and its last fallible step (the connection manager) succeeded is in the `codecov:ignore` half: checked by reading, and the PR says so.

Integration (real PostgreSQL, `-tags integration,licdev`, `INTEGRATION_TESTS=true`), with part A's barrier rules (timeouts on every wait, assertions filtered on `classid`/`objid`):
- `tests/integration/migration_empty_workspace_test.go` (extends part A's file), on a throwaway system database:
  - at the current version: one registered workspace with an empty database, one with no database → `RunMigrations` nil, then `InitializeEmptyWorkspaces` (called directly, awaited) gives the empty one its tables and creates nothing for the other;
  - `db_version` one below: `RunMigrations` leaves the empty one empty (part A), then `InitializeEmptyWorkspaces` gives it its tables;
  - migration lock, with a barrier: the test holds the exclusive migration key; `RunMigrations` and `InitializeEmptyWorkspaces` both wait (seen in `pg_locks`); released → both complete;
  - the repair holds the shared lock through the initialisation: at the current version, with an empty workspace and the repair held by a barrier on that workspace's name lock (so it already holds the shared migration lock), a concurrent `RunMigrations` waits on the exclusive lock; after release, `RunMigrations` returns only once the workspace has its tables;
  - `db_version` set above the code version → `InitializeEmptyWorkspaces` initialises nothing;
  - restart after a wait: with `db_version` below a migration requiring a restart, `RunMigrations` waits on the held lock; the test stamps the code version, releases, and `RunMigrations` returns `ErrRestartRequired`.

Mutation checks (each must fail at least one test): remove the migration lock (migration barrier test); release the shared lock before the initialisation (repair-holds-lock test); remove the version re-read in the repair (integration, version above code); skip `backgroundWg.Wait` in `cleanupResources` (app unit); make a repair failure fatal (unit).

## CHANGELOG

This commit rewrites part A's bullet in `[42.3]` into one:

`- **Fix**: A workspace whose creation stops partway no longer leaves a half-built database behind, and a leftover database that already holds tables is refused with a message naming it; a request arriving while a workspace is deleted no longer recreates its database. At startup, updates no longer stop on a workspace whose database has no tables, which is now set up shortly after; an update that cannot start because a workspace's database is missing explains how to fix it, and instances starting together apply updates one at a time.`

## Review history

Carried over from the reviews of the combined plan (2026-10-31), all checked against the code: D3 held only inside one process (D7, version re-read in the repair); the repair must hold the shared lock through the initialisation; the migration-lock wait is unbounded and says so; `Shutdown` did not wait for the goroutine, and the harness's shutdown path skips the server; the missing app seam; the initialisation's own bound. The delete-race items (deterministic test, repository seam, registration re-check inside the lock) went to part A with D8. Pierre's answers (2026-10-31): 42.3 with part A, one PR with two commits, repair failures never fatal (D9), unbounded wait with keepalives.

- Gemini (on both parts, 2026-10-31; checks in the project files, `reviews/fix-workspace-db-create-recovery-gemini-verif-plan.md`): the holder lookup handles the single-bigint form, several holders and none; the background wait is bounded by `cleanupResources`'s context, since `a.shutdownCtx` is already cancelled when it runs.

## Test commands

1. `make test-migrations`, `make test-database`, `go test ./internal/app/...`
2. `docker compose -f tests/compose.test.yaml up -d`, then the touched integration tests only: `INTEGRATION_TESTS=true go test -race -tags integration,licdev ./tests/integration/ -run 'TestMigrationEmptyWorkspace' -v`
3. `make test-unit`, then the full integration suite in CI, comparing its duration with the last `dev` run.
4. Release gate on the finished PR (both commits): exact-tree CI suites twice, the mutation checks of parts A and B, Gemini review of the whole diff.
