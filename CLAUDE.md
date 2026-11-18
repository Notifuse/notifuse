# Notifuse — Project Instructions

Self-hosted email marketing platform. Go backend (Clean Architecture, stdlib `http.ServeMux`, PostgreSQL 18, 17 the compatibility floor) + React frontends (`console/` admin app, `notification_center/` embeddable widget).

**User-facing documentation lives in a separate git repository**, `../docs` (Mintlify). Doc edits never
show up in this repo's `git status` and ship as their own PR — easy to write and then forget to commit. Dependency and version details live in `go.mod`, `console/package.json`, and `notification_center/package.json` — read those rather than trusting any list here.

## Architecture

Clean Architecture with constructor-based dependency injection, layered as:

- `internal/domain` — entities and interfaces
- `internal/service` — business logic
- `internal/repository` — data access (Squirrel query builder)
- `internal/http` — handlers and middleware
- `internal/migrations` — database migration system

Interface conventions: define interfaces in the consuming package (not the implementing one), keep them small and focused, and generate mocks with `//go:generate mockgen`.

## Architecture reference

The private repository `Notifuse/architecture` (checked out at `../architecture`) documents the broadcast,
email queue and provider webhook flows, task scheduling and shutdown, and the properties each one holds,
verified against a notifuse commit. When a change alters one of these flows (its steps, the conditions its
writes check, its transactions, or a property's statement), update the matching `flows/*.md` in a PR to
that repository, and mark each finding the change settles `fixed` with the notifuse commit. Without access
to it, list the flows the change touches in the PR description.

## Database migrations

One system database plus one database per workspace; a schema change can touch either or both. All schema changes go through the custom migration system in `internal/migrations/`:

- Version format is `vMAJOR.minor`: bump MAJOR for database schema changes, minor for everything else. The code version lives in `config/config.go` (`VERSION`).
- Each migration implements `MajorMigrationInterface`, registers itself via `init()`, and declares whether it updates the system DB, workspace DBs, or both. Migrations run automatically on startup.
- Migrations must be idempotent (`IF NOT EXISTS`), run in transactions, preserve existing data via defaults, and be documented in `CHANGELOG.md`.
- **No PostgreSQL 18-only DDL.** `compose.yaml` ships 18, but 17 is the floor (AlloyDB Omni and Supabase have no public 18) and CI runs it; Notifuse refuses to start below 17 (`database.MinServerVersionNum`, `internal/database/version.go`).
- **Never amend a shipped `vN.go` to fix databases already at N.** The dispatcher selects
  `migrationVersion > currentDBVersion`, so an amended `vN.go` reaches fresh installs and databases
  below N and never those at N — silently. Always a new major.
- **Altering `contacts`, `contact_lists`, `lists`, `custom_events` or `contact_timeline` means
  regenerating automation trigger functions.** An automation's trigger conditions are compiled into its
  PL/pgSQL function body, and a function body registers no `pg_depend` entries — so renaming or dropping a
  column those conditions reference is not blocked and raises nothing at migration time. It arms a failure
  that fires on the next write to `contact_timeline`, which every contact, list, message-history,
  custom-event and inbound-webhook write feeds. `V38Migration.healAutomationTriggerConditions` is the
  pattern: regenerate through the real generator, probe each condition before installing, savepoint per
  automation.
- **Changelog**: a defect in code that never shipped gets **no `Fix` bullet** — fold it into the
  unreleased version's feature bullet. Only fixes to already-released behaviour get their own `Fix`
  line; otherwise the entry describes a version nobody ran.
- Full step-by-step walkthrough and example: use the `create-migration` skill.

## API conventions

RPC-style endpoints with dot notation, not REST:

```
POST /api/workspace.create
POST /api/workspace.update
POST /api/contact.create
GET  /api/contact.list
```

## Design decisions (don't relitigate)

- Standard library HTTP mux — deliberately no web framework.
- JWT for stateless auth. No user passwords: people sign in with a magic code or OIDC SSO, and a root email can also sign in programmatically with an HMAC signature (`user.rootSignin`).
- Console UI: Ant Design for complex components, Tailwind for utility styling; TanStack Query for all server state; TanStack Router for routing.
- Emails: MJML (Notifuse/safe-mjml) rendering + Liquid templating (Notifuse/liquidgo).

## Frontend

- Console i18n uses LinguiJS with mandatory wrapping of all user-facing strings — rules and commands are in `console/CLAUDE.md`.
- Strict TypeScript; prefer interfaces over types for object shapes; never `any`.

## Testing

- Backend test commands are defined in the `Makefile` — per-layer targets (`make test-domain`, `test-service`, `test-repo`, `test-http`, `test-migrations`, …), `make test-unit`, `make test-integration`, `make coverage`. Check the Makefile for the current list.
- Frontend: `cd console && npm test`.
- Every touched backend file needs corresponding unit tests in its layer; new features also need integration tests exercising the full stack.
- **Never state a count in a test name or comment unless an assertion pins it.** A prose "39 rules"
  beside a `len(...)`-derived assertion rots without ever failing CI. Either `require.Len(t, x, 39)` or
  count-agnostic wording — and a test that asserts against the very constant it protects
  (`assert.Len(t, got, someConst)`) pins nothing at all.

## Plans directory

Plans and specs are not kept in this repository: never commit them here. They live in the private repository `Notifuse/architecture` (checked out at `../architecture`), under `plans/notifuse/`, as kebab-case markdown: `feature-name-plan.md`, `fix-description-plan.md`, `refactor-component-plan.md`, `migration-v7-plan.md`. `plans/` is ignored here so that a draft never gets staged by accident. Every plan must specify, for each implementation file, the corresponding test file and test cases, and must end with a step that runs the relevant test commands for the layers touched.

## 🤖 Claude Agent Rules

- **Never self-advertise**: Do not add "Generated with Claude", "Co-Authored-By: Claude", or any AI attribution
- **Clean commits**: No AI signatures, footers, or mentions in git commit messages
- **Clean releases**: No AI credits in changelogs, release notes, or PR descriptions
- **No AI markers**: No comments like "// Generated by Claude" in code
