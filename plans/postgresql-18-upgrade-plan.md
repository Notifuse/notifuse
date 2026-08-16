# PostgreSQL 18 upgrade plan

**Status**: proposed — not started
**Date**: 2026-08-16
**Decisions taken** (confirmed with Pierre before writing):

1. **`compose.yaml` defaults to PostgreSQL 18 for new installs.** 17 stays supported.
2. **Existing installs migrate through a one-shot `pgautoupgrade` container**, driven by a single command.
3. **17 remains the DDL compatibility floor.** No PG18-only DDL, ever, while `compose.alloydb.yaml` exists.
4. **Scope is the shipped product**: `notifuse`, `deploy` and `docs` repos. `cloud/` is out of scope (it already runs PG18 — see §3.5).

---

## 1. Why this is not a one-line tag bump

Three independent things change between `postgres:17-alpine` and `postgres:18-alpine`, and only one of them is PostgreSQL itself.

### 1.1 The Docker image moved `PGDATA` and `VOLUME`

Verified against the image Dockerfiles and reproduced locally:

| | `postgres:17-alpine` (17.11) | `postgres:18-alpine` (18.6) |
|---|---|---|
| `PGDATA` | `/var/lib/postgresql/data` | `/var/lib/postgresql/18/docker` |
| `VOLUME` | `/var/lib/postgresql/data` | `/var/lib/postgresql` |

Source: [docker-library/postgres#1259](https://github.com/docker-library/postgres/pull/1259) (merged 2025-06-06), `18/alpine3.24/Dockerfile:201-206`. The official docs now say: *"The defined `VOLUME` was changed in 18 and above to `/var/lib/postgresql`. Mounts and volumes should be targeted at the updated location."*

**What a naive tag bump does to a Notifuse install today** (`compose.yaml:74` + `:82`, named volume at `/var/lib/postgresql/data`): the 18 entrypoint's `docker_error_old_databases` guard fires *before* `initdb`, prints a long explanatory error, and exits 1. With `restart: unless-stopped` this is a crash loop. **Data is untouched.** This is the good failure — but it is still a hard outage for anyone who runs `git pull && docker compose up -d`.

⚠️ **The widely-blogged "fix" — `PGDATA=/var/lib/postgresql/data/pgdata` — is the silent-data-loss variant.** Overriding `PGDATA` makes the entrypoint's `elif [ "$PGDATA" = "/var/lib/postgresql/$PG_MAJOR/docker" ]` branch false, which disables the *entire* old-database detection block. Reproduced: container starts, exit 0, no warning, `notifuse_system` and every `notifuse_ws_*` gone, old cluster still sitting on disk unreferenced. **The docs must forbid this explicitly.** (Note: `cloud/manager/internal/k8s/postgres.go:204-205` uses exactly this pattern — safe there only because every tenant volume is provisioned fresh. Do not copy it to self-hosted.)

### 1.2 `initdb` 18 enables data checksums by default

PG18 "Migration to Version 18": *"Change initdb default to enable data checksums… pg_upgrade requires matching cluster checksum settings."* Measured: `postgres:17-alpine` → `data_checksums = off`, `postgres:18-alpine` → `on`. A hand-rolled `pg_upgrade` between the two images aborts with `old cluster does not use data checksums but the new one does`.

`pgautoupgrade` detects this and passes `--no-data-checksums` automatically (`postgres-docker-entrypoint.sh:570`). `tianon/postgres-upgrade` does not — that is still-open [issue #121](https://github.com/tianon/docker-postgres-upgrade/issues/121). This is the deciding reason to use `pgautoupgrade`.

Consequence: **upgraded clusters end up with checksums off, fresh 18 installs get them on.** Acceptable divergence; document `pg_checksums --enable` as an optional later step.

### 1.3 The PostgreSQL server itself — no impact on our code

All 11 "Migration to Version 18" incompatibilities were cross-checked against the codebase, and the schema was installed and exercised end-to-end on a live PostgreSQL 18.6 server (real `InitializeDatabase`, `InitializeWorkspaceDatabase`, `AutomationTriggerGenerator.Generate`, timeline/webhook triggers, automation enrolment, web-analytics upserts and rollups, partition routing, the v37 trigger drop/widen/recreate dance, v37/v38 `pg_depend`/`pg_trigger` catalog queries).

| PG18 incompatibility | Notifuse | Evidence |
|---|---|---|
| `initdb` data checksums on | **YES** | operational — see §1.2 |
| Timezone abbreviation lookup order | no | `IsValidTimezone`/`internal/domain/timezones.go` accepts only real zone names; app never sets a session `TimeZone` |
| MD5 password auth deprecated | no | app never issues `CREATE/ALTER ROLE`; `md5()` is used as a hash only (`web_analytics_timeline_projection.go:89,157`) |
| `VACUUM`/`ANALYZE` recurse into children | no | no `INHERITS` anywhere; declarative partitions already cascaded on 17 |
| `COPY FROM` `\.` in CSV | no | no `COPY`/`pq.CopyIn` |
| Unlogged partitioned tables disallowed | no | no `UNLOGGED` |
| AFTER triggers run as queuing role | no | no `SET ROLE`, no `SECURITY DEFINER`, no deferred triggers |
| `GRANT … RULE` removed | no | never used |
| `pg_backend_memory_contexts` changes | no | not referenced |
| FTS/`pg_trgm` collation provider | no | no tsvector/tsquery/pg_trgm/text_pattern_ops |
| PK/FK collation determinism | no | no `COLLATE`, no `citext` |

Also verified clean on 18.6: partial-index `ON CONFLICT` inference (`annotation_postgres.go:182`), `RETURNING (xmax = 0)` (`contact_postgres.go:1384`), `FOR UPDATE SKIP LOCKED`, BRIN + GIN index cascade to partitions, `DROP DATABASE … WITH (FORCE)`, jsonb subscripting, `PERCENTILE_CONT`, every `pg_stat_*`/`information_schema` column the repo reads, and `lib/pq v1.10.9`'s SCRAM-SHA-256 against an 18 server.

**One latent (pre-existing, not PG18) bug found**: `internal/repository/web_analytics_postgres.go:543` matches on `pqErr.Message` text (`"no partition of relation"`) rather than SQLSTATE — the only such site in the repo, and one the codebase elsewhere explicitly calls out as an anti-pattern. The message is unchanged on 18.6, but it is translated by `lc_messages`, so on a non-English server the auto-create-partition-and-retry path silently drops the beat batch. Folded into W5.

---

## 2. Blast radius by deployment shape

| Deployment | What happens on `git pull && docker compose up -d` after this change | Action needed |
|---|---|---|
| Stock `compose.yaml`, named volume | postgres crash-loops with the explanatory error; api healthcheck fails. **No data loss.** | Run the one-shot upgrade |
| Stock compose, **bind mount** at `/var/lib/postgresql/data` | Same hard error (the entrypoint has a `/proc/self/mountinfo` fallback since [#1409](https://github.com/docker-library/postgres/pull/1409)) | Run the one-shot upgrade |
| Own compose, user-set `PGDATA` | ⚠️ **Silent re-init.** Old data orphaned on disk. | Docs must warn; W2 guard makes the app refuse |
| External PostgreSQL (`DB_HOST` → RDS/Cloud SQL/…) | Nothing. Notifuse only connects. | Nothing — 18 already works |
| Coolify one-click (`deploy/coolify/notifuse.yaml`) | Same crash loop. Coolify's `getMountPath` runs on **create only**, so an existing service keeps `/var/lib/postgresql/data` | Update the template + a doc note |
| Dokploy catalog | Maintained upstream (`Dokploy/templates`) — this repo cannot reach it. Dokploy also mounts the PGDATA leaf `/var/lib/postgresql/18/docker`, which forfeits `pg_upgrade --link` forever | Separate upstream PR |
| AlloyDB Omni (`compose.alloydb.yaml`) | Unaffected — stays on 17 | Bump 17.5.0 → 17.9.0 only |

**Sizing note for the runbook**: `pg_upgrade` cost here is driven by *relation count*, not bytes. Notifuse creates 3 monthly partitions × 3 indexes per workspace per month and **never drops them** (`web_analytics_maintenance_worker.go:88-105` only ever creates current/next month). A measured 25-workspace / 24-month cluster held 19,695 relation files in 341 MB; `--link` and `--copy` took the same ~14 s because the work is catalog work. A 200-workspace / 5-year install is ~150k relations. Tell operators to measure `find $PGDATA/base -type f | wc -l` before booking the window. A partition-retention policy is worth opening as separate work — it would cut this cost permanently.

---

## 3. Constraints that are not ours to change

### 3.1 AlloyDB Omni pins the DDL floor at 17

AlloyDB Omni **does** have a PostgreSQL 18 base — 18.3.0, GA 2026-07-15 — but the container images are gated. Docker Hub `google/alloydbomni` has 87 tags and **zero** matching `18`; `latest` → `17.9.0`. The repository overview says: *"To get access to the AlloyDB Omni PostgreSQL 18 images … please fill out the AlloyDB Omni Registration form."* PG18 Omni ships via the Kubernetes operator (digest-pinned) and RPM only.

So a Notifuse user cannot `docker compose -f compose.alloydb.yaml up -d` on PG18 without registering with Google. **`compose.alloydb.yaml` can move 17.5.0 → 17.9.0 and no further.**

### 3.2 Supabase has no PG18 either

`supabase/postgres` tops out at 17.x; maintainer statement 2026-05-04: *"it's not very soon, but eventually in 2026."* Supabase users are exactly the audience that points a self-hosted app at an external Postgres.

**Together these make "17 stays the compatibility floor" a hard rule, not a preference.** Every other managed provider is ready (RDS since 2025-11, Aurora 2026-06, Cloud SQL GA and default, Azure GA, Neon GA, Render/Tiger default, Railway, DigitalOcean).

### 3.3 Never change base-image *flavour* and the major version in one step

`pg_upgrade` does not rebuild indexes, and musl and glibc disagree on text ordering under the same `en_US.utf8`:

```
alpine/musl : Ann@x.com, Zoe@x.com, _bob@x.com, alice@x.com, ann@x.com
debian/glibc: alice@x.com, ann@x.com, Ann@x.com, _bob@x.com, Zoe@x.com
```

`contacts.email` is the PRIMARY KEY of the largest table in every workspace database (`internal/database/init.go:80`). Alpine also records **no** `datcollversion` at all (musl defines none of `__GLIBC__`/`LC_VERSION_MASK`/`WIN32` in `pg_locale_libc.c`), so PostgreSQL emits no warning — the corruption is silent. Since the shipped compose is alpine and `pgautoupgrade:18-alpine` is alpine, the documented path is safe and `PGAUTO_REINDEX=no` is correct; anyone on `postgres:17` (Debian) must use `pgautoupgrade:18-trixie` and leave reindexing on.

Both `postgres:17` and `postgres:18` are currently Debian 13 trixie / glibc 2.41, so the plain tags cross no glibc boundary *today*.

### 3.4 `pgautoupgrade` is a transitional tool, not a runtime image

Good: MIT, 1.2k stars, pushed daily, CI genuinely covers 17→18 on both flavours, handles both PG18 landmines automatically, and its maintainers explicitly designed it to be discarded (*"it should always be an option to ditch pgautoupgrade for the plain Postgres image"*).

Bad: **no tagged releases** (you can pin a Postgres version, not a tool version); target lags upstream (18.4 vs community 18.6); it uses `pg_upgrade --link` and then **deletes the old cluster** (`postgres-docker-entrypoint.sh:686`), so there is no rollback other than a backup; open [#211](https://github.com/pgautoupgrade/docker-pgautoupgrade/issues/211) — *"Upgrading fails with ssl enabled"*, filed 2026-03-04, zero maintainer response; open #210 (tablespaces on separate volumes).

It also runs `reindexdb --all --concurrently` and a per-database `VACUUM (ANALYZE)` afterwards, serially, with no `-j`. Over one database per workspace that entirely negates `--link`'s speed. **`PGAUTO_REINDEX=no` is mandatory for this to be usable at Notifuse's shape** — and it is safe, per §3.3.

→ Therefore: **one-shot, opt-in profile; never the long-running `postgres` service.**

### 3.5 `cloud/` already runs PG18 in production

`cloud/manager/internal/config/config.go:61` — `v.SetDefault("k8s.pg_image", "postgres:18-alpine")`, matched in the Ansible role, the tenant manifest template, cloud CI, and the README. Every managed tenant has been on PostgreSQL 18 in production. That is strong independent evidence that the schema and the app are fine on 18, and it means self-hosted and cloud currently disagree — this plan closes that gap.

---

## 4. Work packages

### W1 — Compose files and the runtime image

| File | Change |
|---|---|
| `compose.yaml:74` | `postgres:17-alpine` → `postgres:18-alpine` |
| `compose.yaml:82` | `postgres-data:/var/lib/postgresql/data` → `postgres-data:/var/lib/postgresql` |
| `compose.yaml:66-73` | Rewrite the pin comment. It is now false in two ways: it says the volume "would no longer receive the data at all" (the current image hard-errors instead — that wording predates [#3b6b5fca](https://github.com/docker-library/postgres/commit/3b6b5fca)), and the pin itself is gone. New comment states: the mount is the parent by design; **never set `PGDATA`**; existing installs must run the upgrade profile first |
| `compose.yaml:50-51` | `depends_on: - postgres` → `depends_on: postgres: condition: service_healthy` (matches `deploy/coolify/notifuse.yaml:34-36`, and stops the api racing a freshly-initdb'd cluster) |
| `compose.yaml` api service | Add `stop_grace_period: 75s`. `cmd/api/main.go:61-67` asks for 65 s of graceful shutdown inside a 70 s context; Compose's default is 10 s then SIGKILL, which drops up to `SessionFlushInterval = 60s` of un-persisted web-analytics session state (`web_analytics_buffer.go:54`) |
| `compose.yaml` **new** | `pg-upgrade` service under `profiles: ["pg-upgrade"]` — see below |
| `compose.alloydb.yaml:64` | `google/alloydbomni:17.5.0` → `17.9.0` |
| `compose.alloydb.yaml:11-12` | Reword: PG18 Omni exists (18.3.0 GA) but its images require a Google access-registration form, so the public compose path stays on 17 — and **that is why no PG18-only DDL is allowed anywhere in the repo** |
| `Dockerfile:95` | `alpine:3.19` → `alpine:3.24`. 3.19 is EOL and its `postgresql-client` is 16.11 — already a major behind the server we ship. 3.24 matches `postgres:18-alpine`'s base, so the bundled `psql`/`pg_dump` become 18.6 and actually work against the shipped server. (Alternative considered: drop `postgresql-client` entirely — nothing in non-test Go imports `os/exec`, `CMD` is the binary directly, and it has been unused since the first Docker commit. Bumping the base is the better trade: it also clears an EOL base image, and operators do reach for `docker exec … psql`.) |

The upgrade profile, added to `compose.yaml` rather than a second file so the volume resolves to the same Compose project without any `external:`/naming ambiguity:

```yaml
  # One-shot PostgreSQL 17 -> 18 upgrade for an existing install. Not started by
  # `docker compose up`; run it explicitly, once, with the stack stopped:
  #
  #   docker compose stop -t 75 api && docker compose stop postgres
  #   docker compose --profile pg-upgrade run --rm pg-upgrade
  #   docker compose up -d
  #
  # It upgrades the data directory in place with pg_upgrade --link and then
  # DELETES the old cluster. Back up first — see docs/self-hosting/upgrading-postgresql.
  pg-upgrade:
    profiles: ['pg-upgrade']
    image: pgautoupgrade/pgautoupgrade:${PGAUTO_TAG:-18-alpine}
    environment:
      - POSTGRES_USER=${DB_USER:-postgres}
      - POSTGRES_PASSWORD=${DB_PASSWORD:-postgres}
      - POSTGRES_DB=${POSTGRES_DB:-postgres}
      - PGAUTO_ONESHOT=yes
      # Skips reindexdb --all, which would otherwise rebuild every index in every
      # workspace database serially. Safe ONLY because both the old and the new
      # image are Alpine/musl, whose collation is byte order and cannot drift.
      # If you changed the postgres image to Debian, set PGAUTO_TAG=18-trixie
      # and PGAUTO_REINDEX=yes.
      - PGAUTO_REINDEX=${PGAUTO_REINDEX:-no}
    volumes:
      - postgres-data:/var/lib/postgresql
    networks:
      - notifuse-network
```

Why this works for Notifuse's exact shape without any manual `mv`: the existing volume holds the PG17 cluster at its **root** (it was mounted at `/var/lib/postgresql/data`, which *was* `PGDATA`). Mounted at `/var/lib/postgresql`, pgautoupgrade's `postgres-docker-entrypoint.sh:349` sees `/var/lib/postgresql/PG_VERSION`, target major 18, `PGDATA=/var/lib/postgresql/18/docker` → sets `MOVING_TO_NEW_STRUCTURE=1` and restructures the directory itself. `tianon/postgres-upgrade` assumes the old cluster is already at `17/docker` and would require a hand-rolled, destructive-if-interrupted `mv` — which is why it is not the choice here.

**Tests**: no Go tests. Verified by W6 (upgrade rehearsal) and by a manual fresh-install smoke test (`docker compose up -d` on an empty volume → setup wizard reachable, `SHOW data_checksums` = `on`, `SHOW data_directory` = `/var/lib/postgresql/18/docker`).

### W2 — Safety rails: stop the app turning an empty cluster into a plausible install

This is the highest-value code change in the plan. Today, pointed at an empty cluster, Notifuse manufactures a healthy-looking install in about a second, with one `info` line among ~100 startup lines as the only signal — reproduced against the real binary:

```
{"level":"info","message":"System database check completed"}
{"level":"info","code_version":"38","message":"First run detected, initializing database version"}
{"level":"info","version":"38","message":"Database version updated"}
{"level":"info","message":"Server started successfully"}
```

Path: `app.go:304` `EnsureSystemDatabaseExists` runs a bare `CREATE DATABASE` when absent (`internal/database/utils.go:179-189`, no "should already exist" check) → `app.go:335` `InitializeDatabase` creates every table → `app.go:343` `RunMigrations` takes the `!versionExists` branch (`internal/migrations/manager.go:122-129`), stamps `db_version = 38` and returns nil. Worse: `db_version` is now 38, so a later restore of the real data into that same system database looks up to date.

A related hole: if the system DB is restored but a workspace DB is missing, and PostgreSQL was upgraded *without* upgrading Notifuse, the app boots clean and then `pkg/database/connection_manager.go:291` catches SQLSTATE `3D000` on first access and **silently** `CREATE DATABASE`s an empty workspace — minutes after the upgrade, decoupled from it. (If Notifuse *was* also upgraded, migration fails closed and the app refuses to start — correct behaviour.)

Changes:

| File | Change |
|---|---|
| `config/config.go` (`DatabaseConfig` struct ~line 100; defaults ~line 420) | Add `ExpectExisting bool` / `DB_EXPECT_EXISTING`, default `false` |
| `internal/database/utils.go` | Add `AssertMinimumServerVersion(db *sql.DB) error` using `SELECT current_setting('server_version_num')::int` against `const MinServerVersionNum = 170000`; call it in `EnsureSystemDatabaseExists` right after `db.Ping()` (line ~166), before the `pg_database` lookup. First connection the process makes, on the maintenance DB, upstream of all DDL |
| `internal/database/utils.go` | `EnsureSystemDatabaseExists` takes the new flag: when set and the database does not exist, return a fatal error naming the database instead of creating it |
| `internal/migrations/manager.go:122` | Raise the first-run log to `Warn`, reword to *"First run detected — creating a new empty database. If this is NOT a new installation, STOP: the server is pointing at an empty database."* Under `DB_EXPECT_EXISTING`, return an error instead |
| `internal/app/app.go` after `RunMigrations` (~line 343) | Startup reconciliation: for every row in `workspaces`, assert `<prefix>_ws_<id>` exists; log `Error` per miss, and refuse to start under `DB_EXPECT_EXISTING` |
| `pkg/database/connection_manager.go:291` | Log `Warn` in the auto-create branch — it currently creates a database with no log line at all |

**Tests**

| Implementation file | Test file | Cases |
|---|---|---|
| `config/config.go` | `config/config_test.go` | `DB_EXPECT_EXISTING` defaults false; parses `true`; survives a full `Load` |
| `internal/database/utils.go` | `internal/database/utils_test.go` | `AssertMinimumServerVersion` accepts 170000/180000, rejects 160000 with a message naming both versions, propagates a query error; `EnsureSystemDatabaseExists` creates when flag off, returns an error and issues **no** `CREATE DATABASE` when flag on and the DB is absent, succeeds when flag on and the DB exists (sqlmock) |
| `internal/migrations/manager.go` | `internal/migrations/manager_test.go` | `RunMigrations` on `!versionExists` with flag off → stamps version, logs at Warn; with flag on → returns an error and does **not** call `SetCurrentDBVersion` |
| `internal/app/app.go` | `internal/app/app_test.go` | reconciliation logs Error for a workspace row with no database; returns an error under the flag; passes when all databases exist |
| `pkg/database/connection_manager.go` | `pkg/database/connection_manager_test.go` | the `3D000` branch emits a Warn before calling `EnsureWorkspaceDatabaseExists` |
| end-to-end | `tests/integration/pg_expect_existing_test.go` (new, `//go:build integration`) | boot against an empty cluster with the flag set → refuses to start; boot with a system-DB-only restore and a `workspaces` row → refuses to start; both boot normally with the flag unset |

### W3 — CI matrix

| File | Change |
|---|---|
| `.github/workflows/go.yml:63` | Turn the postgres service image into a matrix over `[postgres:17-alpine, postgres:18-alpine]`. This covers both majors **and** switches CI to the flavour production actually runs — today CI runs Debian/glibc while `compose.yaml` ships Alpine/musl, so CI has never exercised production's text ordering |
| `.github/workflows/go.yml:61-62` | Rewrite the comment: 17 is the floor (AlloyDB Omni + Supabase), 18 is the shipped default, both are tested |
| `tests/compose.test.yaml:7` | `postgres:17` → `postgres:17-alpine`, with `image: ${POSTGRES_TEST_IMAGE:-postgres:17-alpine}` so 18 can be run locally without editing the file |
| `tests/compose.test.yaml:3-6` | Reword: still the floor, now also the shipped flavour |

⚠️ **Flavour flip is the risky half of W3, not the version bump.** Alpine reports `no usable system locales were found` and its collation is byte order. Run the full integration suite on `postgres:17-alpine` locally and diff the results against `postgres:17` before merging; if anything sorts differently, that is a finding about production, not about the test image. Existing local `postgres_test_data` volumes need one `docker compose -f tests/compose.test.yaml down -v`.

Two unit tests pin French `lc_messages` error text captured from PG17 (`internal/repository/template_postgres_test.go:283`, `internal/repository/federated_identity_postgres_test.go:96`). They construct `pq.Error` values directly, so they never touch a server and are not at risk — noted so nobody re-audits them.

**Tests**: the existing suites, run twice by the matrix. No new Go tests.

### W4 — Documentation

| Repo / file | Change |
|---|---|
| `docs/self-hosting/upgrading-postgresql.mdx` **(new)** | The runbook in §5, verbatim. Add to `docs.json` under the `self-hosting` group |
| `docs/self-hosting/backups.mdx` **(new, or a section on the page above)** | The docs currently contain **zero** backup instructions — the only occurrence of "backups" site-wide is the Cloud upsell at `installation.mdx:7`. Publishing an upgrade procedure without one is not acceptable |
| `docs/self-hosting/installation.mdx:59` | *"Recommended version: PostgreSQL 17 or higher"* → 18 recommended, 17 minimum, and note the compose file now mounts the volume at `/var/lib/postgresql` |
| `README.md:65` | Restate: ships PostgreSQL 18, requires 17+, existing installs follow the upgrade page |
| `CLAUDE.md:3` | "PostgreSQL 17" → "PostgreSQL 18 (17 is the compatibility floor — see `compose.alloydb.yaml`)" |
| `CLAUDE.md` migrations section | Promote the no-PG18-only-DDL rule from a comment in `compose.alloydb.yaml` to an explicit repo rule, so `create-migration` inherits it |
| `tests/MAKEFILE_TEST_COMMANDS.md:220` | Fix the pre-existing drift (documents `postgres:17-alpine`, the file actually used `postgres:17`) and align with W3 |
| `tests/testutil/database.go:356-357` | Comment says "the test container is pinned to 17" — update |
| `internal/service/query_builder.go:822` | Comment says "PostgreSQL 17 subscript notation" — jsonb subscripting is PG14+. Fix while nearby |
| `deploy/coolify/notifuse.yaml:43,49` + `:39-42` | Image, volume path, comment — same three edits as `compose.yaml` |
| `deploy/README.md:30-34`, `deploy/coolify/README.md:37` | Rewrite the "pinned to 17" guidance |
| `CHANGELOG.md` | One **Improvement** bullet folded into the existing unreleased `[38.0]` section (per repo convention: no VERSION bump, no `vN.go` — this is not a schema change) |

Draft changelog bullet:

> - **Improvement**: The bundled PostgreSQL moves to 18 for new installations, and the compose file now mounts its volume at `/var/lib/postgresql` as PostgreSQL 18's image requires. Notifuse still runs on PostgreSQL 17 and above, so nothing forces you to move. Existing installations must upgrade the data directory before starting the new container — one command, documented under Self-hosting → Upgrading PostgreSQL. Back up first: the upgrade is done in place and is not reversible.

**Out of this repo**: the Dokploy catalog entry (`Dokploy/templates`, `blueprints/notifuse/`) is maintained upstream and needs its own PR. Note that Dokploy mounts the PGDATA leaf `/var/lib/postgresql/18/docker` rather than the parent, which forfeits `pg_upgrade --link` for every future major — worth raising with them.

### W5 — Fold in the partition-error locale bug

`internal/repository/web_analytics_postgres.go:543` — replace `strings.Contains(pqErr.Message, "no partition of relation")` with a locale-proof test. `pq.Error.Routine` is populated and never translated; on both 17.11 and 18.6 the error carries `LOCATION: ExecFindPartition, execPartition.c`. Use `pqErr.Code == "23514" && pqErr.Routine == "ExecFindPartition"`.

**Tests**: `internal/repository/web_analytics_postgres_test.go` — auto-create-and-retry fires for a `23514`/`ExecFindPartition` error whose message is the French translation; does **not** fire for an unrelated `23514` check violation; does not fire for a different SQLSTATE.

### W6 — Upgrade rehearsal (recommended; nightly, not on every PR)

`scripts/pg-upgrade-rehearsal.sh` + a nightly job in `.github/workflows/`. This is the only test that exercises the volume relayout, the checksum flip and the relation-count cost together:

1. Create a scratch volume; start `postgres:17-alpine` mounted at `/var/lib/postgresql/data`.
2. Seed it through the real code path: boot the binary, create N workspaces, insert contacts and web-analytics rows across two months so partitions exist.
3. Snapshot: `SELECT datname FROM pg_database ORDER BY 1`, per-workspace row counts for `contacts`/`contact_lists`/`message_history`/`contact_timeline`/`web_sessions`, `settings.db_version`, and `string_agg(email, ',' ORDER BY email)` from `contacts` as a collation canary.
4. Stop; run `pgautoupgrade/pgautoupgrade:18-alpine` with `PGAUTO_ONESHOT=yes PGAUTO_REINDEX=no` and the volume at `/var/lib/postgresql`.
5. Start stock `postgres:18-alpine` at `/var/lib/postgresql`; assert `version()` is 18.x, `data_directory` is `/var/lib/postgresql/18/docker`, and every snapshot value matches.
6. Run the existing integration suite against the upgraded volume.
7. Fire one write into `contact_timeline` per workspace that has a live automation — trigger function bodies are compiled at runtime and register no `pg_depend`, so nothing else proves they survived.

---

## 5. The user-facing runbook (content for the new docs page)

Ordered so that every step is either reversible or preceded by a backup.

```bash
cd /path/to/notifuse

# 1. Record the pre-upgrade state (app still running)
docker compose exec postgres psql -U postgres -Atc \
  "SELECT datname FROM pg_database ORDER BY 1" > pre-upgrade-databases.txt
docker compose exec postgres sh -c 'find "$PGDATA/base" -type f | wc -l'   # sizing

# 2. Stop the app cleanly. -t 75 matters: Notifuse asks for 65s to drain its
#    web-analytics buffer, and Compose's default is 10s then SIGKILL.
docker compose stop -t 75 api

# 3. Logical backup, taken with an 18 client (pg_dump refuses to dump a server
#    newer than itself; it may be newer than the server). The api is stopped, so
#    this is consistent — pg_dumpall uses a separate snapshot per database, and
#    Notifuse's system DB references per-workspace DBs across that boundary.
docker run --rm --network notifuse_notifuse-network -v "$PWD/backup:/backup" postgres:18-alpine \
  sh -c 'PGPASSWORD=$PGPASSWORD pg_dumpall -h postgres -U postgres \
           --globals-only --no-role-passwords > /backup/globals.sql'
# per-database, parallel and restartable (pg_dumpall has no -F/-j, even in 18)
for db in $(docker compose exec -T postgres psql -U postgres -Atc \
     "SELECT datname FROM pg_database WHERE datname='notifuse_system' OR datname LIKE 'notifuse\_ws\_%'"); do
  docker run --rm --network notifuse_notifuse-network -v "$PWD/backup:/backup" postgres:18-alpine \
    pg_dump -h postgres -U postgres -Fd -j 4 -f "/backup/$db" "$db"
done

# 4. Stop the database and take a cold copy of the volume. This is the only
#    real rollback artifact: Docker named volumes have no snapshot primitive,
#    and a hot copy of a live PGDATA is torn.
docker compose stop postgres
docker run --rm -v notifuse_postgres-data:/v -v "$PWD/backup:/backup" alpine \
  tar czf /backup/pg17-volume.tgz -C /v .

# 5. Update Notifuse to the new release, but do NOT start it yet.
git pull            # or: docker compose pull, if you use the published image

# 6. Upgrade the data directory. In place, with pg_upgrade --link; the old
#    cluster is deleted on success.
docker compose --profile pg-upgrade run --rm pg-upgrade

# 7. Start the stack
docker compose up -d

# 8. Verify BEFORE trusting it
docker compose exec postgres psql -U postgres -Atc "SELECT version()"
docker compose exec postgres psql -U postgres -Atc \
  "SELECT datname FROM pg_database ORDER BY 1" | diff - pre-upgrade-databases.txt
docker compose exec postgres psql -U postgres -Atc \
  "SELECT datname, datcollversion FROM pg_database"    # no mismatch warning

# 9. Refresh what pg_upgrade did not carry over. PG18 transfers most optimizer
#    statistics, but not extended statistics and none of the cumulative
#    statistics that drive autovacuum.
docker compose exec postgres vacuumdb -U postgres --all --analyze-in-stages --missing-stats-only -j 4
docker compose exec postgres vacuumdb -U postgres --all --analyze-only -j 4
```

Points the page must state in its own words:

- ⚠️ **Never set `PGDATA` to work around a startup error.** It disables the image's own data-detection guard and lets it initialise an empty cluster on top of your data with no error at all. If the container refuses to start, that refusal is protecting you.
- ⚠️ **`docker compose down -v` and `docker volume prune` destroy the database.** On Coolify/Dokploy the tar in step 4 has to be run from their terminal, since they own the volumes.
- If a restore from `globals.sql` is ever needed: `pg_dumpall` emits `ALTER ROLE postgres … PASSWORD '<old hash>'`, which **overwrites** the password the new container was started with. `pq: password authentication failed` after a restore is this, not a network or pooler problem. Either use `--no-role-passwords` as above, or re-issue `ALTER ROLE postgres PASSWORD '<DB_PASSWORD>'` as the last step. `ERROR: role "postgres" already exists` during a globals restore is expected and harmless.
- **Rollback**: once step 6 completes there is no rollback — `pgautoupgrade` uses `--link` and deletes the old cluster. Restore is `docker compose down`, remove the volume, recreate it from `pg17-volume.tgz`, and put the old compose file back.
- **Staying on 17 is fine.** Keep `image: postgres:17-alpine` and `- postgres-data:/var/lib/postgresql/data`. Both must stay together — the image tag and the mount path are a matched pair.
- Upgraded clusters keep `data_checksums = off` (fresh 18 installs get `on`). To enable them later: stop the server and run `pg_checksums --enable` from an image whose major matches the cluster.
- Do the upgrade on the same CPU architecture the cluster was created on. PG18 clusters record their own `char` signedness, and `pg_upgrade` adopts the build platform's when coming from 17 (`--set-char-signedness`). Don't combine an x86↔ARM move with the major upgrade.
- If you use SSL on the PostgreSQL server, or tablespaces on separate volumes, `pgautoupgrade` has open bugs (#211, #210) — use the dump/restore path instead.

---

## 6. Order of work

1. **W2** first, alone. The whole safety story rests on failures being loud; ship the guard before making a change that can point people at an empty cluster.
2. **W3** next — get 18 green in CI before anything advertises it. Validate the Alpine flip separately from the version bump.
3. **W5** — small, independent, and touches the same subsystem.
4. **W1** — the actual bump, once CI proves 18.
5. **W6** — rehearsal, ideally before W4 publishes the runbook.
6. **W4** last, so the docs describe what actually shipped. `docs` is a separate git repo and a separate PR — easy to write and forget to commit.

---

## 7. Open questions

1. **`DB_EXPECT_EXISTING` name and default.** Proposed: default `false` (nothing changes for existing users), documented as mandatory for the duration of an upgrade window. Alternative: default `true` and require an explicit `DB_ALLOW_BOOTSTRAP=true` on first install — safer, but breaks every existing automated deployment. Recommendation: `false`.
2. **Runtime base image** — `alpine:3.19` → `alpine:3.24` (recommended, gives a working 18.6 client and clears an EOL base) vs dropping `postgresql-client` (nothing calls it). Proceeding with the bump unless told otherwise.
3. **W6 rehearsal** — worth the CI cost, or a one-off manual run against a clone of a real install?

---

## 8. Findings outside this plan's scope

Surfaced during the audit, not PG18 issues, worth their own tickets:

- **Backups miss every workspace database.** `cloud/manager/internal/backup/pgdump.go:27` dumps only `notifuse_system`; `grep notifuse_ws` over `cloud/manager/internal/backup/` returns nothing. `backup-scripts/backup_postgres_to_gcs.sh:31` defaults to a single database too. Contacts, message history and all web-analytics partitions live in the per-workspace databases. This is the most serious thing the audit found and it has nothing to do with PostgreSQL 18.
- **`backup-scripts` has no client-version guard**, and its README examples (`:369`, `:402`) use `postgres:15` — a 15 client aborts against an 18 server, and `set -euo pipefail` kills the whole run.
- **Web-analytics partitions are never dropped.** No retention policy anywhere; relation count grows without bound, which is what makes every future `pg_upgrade` slower.
- **`pkg/analytics/validation.go:96`** uses `time.LoadLocation`, which accepts `""` and `"Local"`; PostgreSQL rejects `Local`. `internal/domain/annotation.go:83` already documents why `IsValidTimezone` exists instead.
- **No telemetry field for the PostgreSQL version** (`telemetry/main.go:18-53`). Adding one would let us size how many self-hosters are on 17 vs 18 before ever raising the floor.

---

## 9. Test commands to run at the end

```bash
# Unit layers touched by W2 and W5
make test-database
make test-migrations
make test-repo
make test-pkg
make test-unit

# Integration, on both majors (W3 matrix, run locally one at a time)
docker compose -f tests/compose.test.yaml up -d
make test-integration
POSTGRES_TEST_IMAGE=postgres:18-alpine docker compose -f tests/compose.test.yaml up -d --force-recreate
make test-integration

# Targeted integration tests added by W2
go test -tags integration ./tests/integration/ -run 'TestPGExpectExisting' -v

# Upgrade rehearsal (W6)
./scripts/pg-upgrade-rehearsal.sh
```

Per repo convention, run only the `Test*` functions covering the modified code, and always pass `-tags integration` — six files build out silently without it. Start PostgreSQL and Mailpit first with `docker compose -f tests/compose.test.yaml up -d`.
