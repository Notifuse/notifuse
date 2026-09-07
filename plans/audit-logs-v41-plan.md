# Audit logs — Notifuse v41.0 implementation plan

First implementation step: copy this file to `notifuse/plans/audit-logs-v41-plan.md` (house rule: plans live in `plans/`).

## Context

Notifuse v40 moved to BSL 1.1 with five licence-gated capabilities. A sixth, **audit logs**, is already reserved end to end: `domain.FeatureAuditLogs = "audit_logs"` exists with an Enterprise-tier 402 message (`internal/domain/license.go:56-59, 262`), Enterprise keys are minted with the flag from day one (`cloud/billing-api/src/licences/licence-tiers.ts`), the console `LicenceGateNotice` already carries audit-log copy, `LICENSE_REQUIRED_TIER.audit_logs = 'Enterprise'`, and both the docs tier table (`docs/self-hosting/licence.mdx:34`) and the pricing page (`homepage/src/data/licensing.ts` `comingSoon`) say "coming soon". What is missing is the capability itself and its one gate call.

Goal for v41: a credible, compliance-oriented audit trail that a SOC 2 / ISO 27001 (A.8.15) questionnaire can be answered with, built pragmatically for a solo maintainer with **no Enterprise customers yet**. 2026 practice (WorkOS, Vercel, Linear, Notion, Customer.io, GitHub) converges on: one uniform actor / action / target / context schema with dot-notation actions, IP + user agent + request id, before/after diffs where they matter, append-only storage, configurable retention, filters + export, and logging failures as well as successes. SIEM streaming and hash chaining are common at the high end and were deferred after a second-opinion review (Gemini) on cost/benefit for a first version.

## Decisions (final)

| # | Topic | Decision |
|---|---|---|
| 1 | Scope | Control plane only: security, admin, configuration changes and bulk data operations. Never the data plane (`contacts.upsert`, `customEvents.upsert`, `transactional.send`, `lists.subscribe`, tracking). |
| 2 | Storage | **One `audit_logs` table in the system DB** with a nullable `workspace_id`. The trail survives workspace deletion, one migration, root cross-workspace queries are plain SQL. Precedent: `tasks`, `workspace_invitations`, `monthly_usage` already hold per-workspace rows in the system DB. |
| 3 | Licence gate | Gate **recording** only. Events are written while `Entitlements().Has(FeatureAuditLogs)` (grace counts). Listing, export and settings are never refused for licence reasons. Transitions write marker rows `licence.recordingStopped` / `licence.recordingResumed` so a gap is never silent. Startup log gains `audit_logs_recording`. |
| 4 | Integrity | Append-only DB trigger: UPDATE and TRUNCATE always refused, DELETE refused unless inside the purge function (transaction-local GUC). Hash chain **deferred**. Docs say plainly that the table owner / a superuser can bypass. |
| 5 | Access | Owners always. New **opt-in** permission resource `audit_logs` (read enforced, write unenforced like `message_history`). It lives in a new `OptInPermissionResources` list, **not** in `AllPermissionResources` (14 entries, pinned by `TestAllPermissionResources`): `grantsFullPermissions`, `NewFullPermissions`, telemetry `rbac_custom` and the console "Full Access" tag stay untouched, so no backfill and no regression for existing full-access members. Owners grant audit-log read explicitly per member / API key. Root users read the deployment scope. |
| 6 | Retention | Default 365 days. `workspace.settings.audit_logs.retention_days` (`*int`: nil = default, 0 = forever, else 30–3650) editable by owners; deployment default `audit_logs_retention_days` on `SystemConfig` (root). Daily purge worker through the SQL purge function. Purge never consults the licence (`# Never` list in `license.go`). |
| 7 | Export | `POST /api/auditLogs.export` streams CSV or NDJSON with the same filters, capped at 100 000 rows (`X-Export-Truncated: true` + trailing marker); the export is itself an audit event. Webhook / SIEM streaming **deferred**. |
| 8 | Failures | Recorded by the same middleware: 403 → `denied`, 402 → `denied` + `metadata.licence_refused=true`, other 4xx/5xx → `failure` with `status_code`. 401 is skipped (expired-session noise; also what keeps unauthenticated scanners from filling the table — 403/402 only ever come from an authenticated actor). Failed logins are `user.verify` / `user.rootSignin` / `user.oidc.exchange` with `outcome=failure` and `metadata.reason`. |
| 9 | Instrumentation | **Middleware-first.** One `AuditMiddleware` **inside the mux** records every classified `POST /api/<route>`; the action name **is the RPC route name** (`workspaces.inviteMember`, `templates.update`). A request-scoped `AuditRecord` in the context is filled by the auth layer (actor) and enriched by ~10 security-boundary services (target name, redacted `{old,new}` diffs). A source-walking test forces every `/api/` route to be classified as audited or excluded, so new routes cannot drift silently. |

## Event model

Row: `id`, `occurred_at`, `workspace_id` (NULL = deployment scope), `action`, `category`, `outcome` (`success` / `failure` / `denied`), `status_code`, actor (`actor_type` user / api_key / system / anonymous, `actor_id`, `actor_email`, `actor_name`, `actor_role` owner / member / root, `auth_method` session / api_key / magic_code / sso / root_password), target (`target_type`, `target_id`, `target_name`), context (`ip_address`, `user_agent`, `request_id`), `changes` JSONB `{field: {old, new}}` (same shape as `contact_timeline.changes`, redacted), `metadata` JSONB.

### Route classification (single source of truth: `internal/domain/audit_actions.go`)

`AuditedActions map[string]AuditActionSpec{Category, TargetType, TargetKey}` — `TargetKey` names the top-level JSON body key holding the target id when no service enriches (`id`, `email`, `template_id`, …).

| Category | Audited routes (POST) |
|---|---|
| auth | `user.signin`, `user.verify`, `user.rootSignin`, `user.oidc.exchange`, `user.logout`, `setup.initialize` (+ GET `user.oidc.callback` via `AuditLoggedReads`, always) |
| licence | `licence.set` |
| system | `settings.update` |
| workspaces | `workspaces.create`, `workspaces.update`, `workspaces.delete`, `workspaces.setBlogSettings`, `workspaces.setCustomFieldLabels`, `workspaces.setWebAnalyticsSettings`, `workspaces.setAuditLogSettings` (new) |
| members | `workspaces.inviteMember`, `workspaces.acceptInvitation`, `workspaces.deleteInvitation`, `workspaces.removeMember`, `workspaces.setUserPermissions` |
| api_keys | `workspaces.createAPIKey`, `workspaces.connectZapier` |
| integrations | `workspaces.createIntegration`, `workspaces.updateIntegration`, `workspaces.deleteIntegration`, `ses.enableTenantIsolation`, `webhooks.register` |
| webhooks | `webhookSubscriptions.create/update/delete/toggle/regenerateSecret` |
| templates | `templates.create/update/delete`, `templateBlocks.create/update/delete` |
| broadcasts | `broadcasts.create/update/schedule/pause/resume/cancel/delete/retryFailed/selectWinner/sendToIndividual` |
| automations | `automations.create/update/delete/activate/pause` |
| lists | `lists.create/update/delete` |
| segments | `segments.create/update/delete/rebuild` |
| transactional | `transactional.create/update/delete` |
| blog | `blogPosts.create/update/delete/publish/unpublish`, `blogCategories.create/update/delete`, `blogThemes.create/update/publish` |
| contacts | `contacts.delete`, `contacts.import`, `customEvents.import`, `annotations.create/update/delete`, `webAnalytics.backfillStart/backfillCancel` |
| audit | `auditLogs.export` (new) |

`AuditLoggedReads`: GET `contacts.list` when `export=true` and no cursor (`ExportContactsModal.tsx:131-158` loops `contactsApi.list` with a cursor; add the flag on the first call) → action `contacts.export`; GET `user.oidc.callback` (`oidc_handler.go:54`, the IdP redirect) always. There is no message-history export in the console today, so no `messages.export` in v41. `Export bool` is added to `GetContactsRequest.FromQueryParams` (`contact.go:397`).

`AuditExcludedRoutes` (explicit, tested): data plane and reads — `contacts.upsert`, `customEvents.upsert`, `transactional.send`, `transactional.testTemplate`, `lists.subscribe`, `contactLists.removeContact`, `contactLists.updateStatus`, `llm.chat`, `email.testProvider`, `settings.testSmtp`, `setup.testSmtp`, `setup.status`, `tasks.*`, `cron`, `cron.status`, `detect`, `demo.reset`, `templates.compile`, `segments.preview`, `broadcasts.refreshGlobalFeed`, `broadcasts.testRecipientFeed`, `webhookSubscriptions.test`, `user.updateLanguage`, `user.oidc.start`, `user.me`, `workspaces.verifyInvitationToken`, and every read (`*.get`, `*.list`, `*.status`, `*.stats`, `*.count`, `*.query`, `*.schemas`, `*.deliveries`, `*.eventTypes`, `*.nodeExecutions`, `*.contacts`, `*.members`, `*.getByIDs`, `*.getContactsByList`, `*.getListsByContact`, `*.getByEmail`, `*.getByExternalID`, `*.getTestResults`, `*.getPublished`, `*.listTenants`, `*.listConfigurationSets`, `*.verifyTenant`, `usage.get`, `analytics.*`, `timeline.list`, `inboundWebhookEvents.list`, `messages.*` stats, `licence.get`, `settings.get`, `auditLogs.list/get/actions`).

Non-HTTP events (written directly through the recorder, `actor_type=system`): `audit.purged` (metadata `deleted`, `retention_days`, `scope`), `licence.recordingStopped`, `licence.recordingResumed`. **Not recorded in v41**: `PauseForCircuitBreaker` — it runs on the send path, which must never consult the entitlement provider; documented as a known gap.

### Enrichment sites (`domain.AuditFromContext(ctx)` is nil-safe; no new constructor parameters)

| # | File / function | Enrichment |
|---|---|---|
| 1 | `internal/http/middleware/auth.go` `RequireAuth` | `SetActorClaims(userID, userType)`; `auth_method` session or api_key |
| 2 | `internal/service/auth_service.go` `AuthenticateUserForWorkspace` | `SetActor(user, userWorkspace, isRoot)` + `SetWorkspace(workspaceID)` (actor_role root when `isRootEmail`) |
| 3 | `user_service.go` `SignIn` (rate limiter `signin` namespace at line 82, `ErrUserNotFound` at 92-111), `VerifyCode`, `RootSignin`, `Logout`; `oidc_service.go` callback / exchange | success: `SetActor(user)`, `SetAuthMethod`; failure: `Fail(reason)` (`rate_limited`, `unknown_email`, `invalid_code`, `expired_code`, `bad_password`, `sso_error`) + target email. HTTP responses unchanged (no enumeration leak; the reason is visible only in the log) |
| 4 | `workspace_service.go` `SetUserPermissions` | target user (id, email), `SetChanges({"permissions": {old, new}})` |
| 5 | `InviteMember`, `AcceptInvitation`, `DeleteInvitation`, `RemoveMember` / `RemoveUserFromWorkspace` | target email, `metadata.member_type` user / api_key, granted scope |
| 6 | `CreateAPIKey`, `ConnectZapier` | target api key (id, email), `metadata.scope`; never the token |
| 7 | `CreateIntegration`, `UpdateIntegration`, `DeleteIntegration`; `ses_discovery_service.go` `EnableTenantIsolation` | target integration id + provider kind; `changes` from `AuditDiff` with credentials redacted |
| 8 | `CreateWorkspace`, `UpdateWorkspace`, `DeleteWorkspace`, `SetBlogSettings`, `SetWebAnalyticsSettings`, `SetCustomFieldLabels`, `SetAuditLogSettings` | target workspace (id, name); `changes` via `AuditDiff` (secret key redacted); create sets `SetWorkspace(newID)` |
| 9 | `settings_handler.go` `handleUpdate` (holds old + new config); `license_service.go` `SetKey` | changed keys with `SMTPPassword`, `OIDCClientSecret`, TLS key redacted; licence: `metadata` tier, org, expires_at, features (never the key) |
| 10 | `contact_service.go` `BatchImportContacts`; `custom_event_service.go` `ImportEvents` | `AddMetadata("count", n)`, `("list_ids", ids)` |
| 11 | `audit_log_handler.go` export | `AddMetadata("format", f)`, `("rows", n)`, `("truncated", bool)`, filters |

## Backend design

### A. Version, schema, migration

- `config/config.go:18`: `VERSION = "41.0"`; then `cd web_analytics_sdk && npm run build`, commit `dist/` + `package.json` (skill rule).
- `internal/database/schema/audit_logs_tables.go` → `AuditLogsTableDefinitions() []string` + `const AuditPurgeGUC = "notifuse.audit_purge"`. Included by **both** the fresh-install path — `schema.TableDefinitions` (iterated by `InitializeDatabase`, `internal/database/init.go:16`; append the audit statements there) plus `"audit_logs"` in `schema.TableNames` — and the upgrade path `internal/migrations/v41.go`, so both run byte-identical SQL. No REFERENCES / CHECK inside CREATE TABLE.

```sql
CREATE TABLE IF NOT EXISTS audit_logs (
  id VARCHAR(32) PRIMARY KEY,
  occurred_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  workspace_id VARCHAR(32),
  action VARCHAR(100) NOT NULL, category VARCHAR(50) NOT NULL, outcome VARCHAR(20) NOT NULL, status_code INTEGER,
  actor_type VARCHAR(20) NOT NULL, actor_id VARCHAR(64), actor_email VARCHAR(255), actor_name VARCHAR(255), actor_role VARCHAR(20), auth_method VARCHAR(20),
  target_type VARCHAR(50), target_id VARCHAR(255), target_name VARCHAR(255),
  ip_address VARCHAR(45), user_agent TEXT, request_id VARCHAR(64),
  changes JSONB, metadata JSONB
);
CREATE INDEX IF NOT EXISTS idx_audit_logs_workspace_time ON audit_logs (workspace_id, occurred_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_audit_logs_time   ON audit_logs (occurred_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_audit_logs_actor  ON audit_logs (actor_id, occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_logs_action ON audit_logs (action, occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_logs_target ON audit_logs (target_type, target_id, occurred_at DESC);
CREATE OR REPLACE FUNCTION audit_logs_guard() RETURNS TRIGGER …
  -- UPDATE / TRUNCATE: RAISE EXCEPTION 'audit_logs is append-only' USING ERRCODE = 'insufficient_privilege'
  -- DELETE: RETURN OLD only when current_setting('notifuse.audit_purge', true) = 'on', else raise
DO $$ … IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'audit_logs_guard') THEN CREATE TRIGGER audit_logs_guard BEFORE UPDATE OR DELETE ON audit_logs FOR EACH ROW EXECUTE FUNCTION audit_logs_guard(); END IF … (same for a BEFORE TRUNCATE … FOR EACH STATEMENT trigger)
CREATE OR REPLACE FUNCTION audit_logs_purge(p_workspace_id VARCHAR, p_before TIMESTAMPTZ) RETURNS BIGINT
  -- PERFORM set_config('notifuse.audit_purge','on',true); DELETE … WHERE occurred_at < p_before AND (workspace_id = p_workspace_id OR (p_workspace_id IS NULL AND workspace_id IS NULL)); GET DIAGNOSTICS n = ROW_COUNT; RETURN n;
CREATE OR REPLACE FUNCTION audit_logs_purge_orphans(p_keep VARCHAR[], p_before TIMESTAMPTZ) RETURNS BIGINT  -- rows of deleted workspaces
```

- `internal/migrations/v41.go` (`V41Migration`: 41.0, `HasSystemUpdate=true`, `HasWorkspaceUpdate=false`, `ShouldRestartServer=false`): `UpdateSystem` iterates `AuditLogsTableDefinitions()` (error prefix `v41:`); **no permission backfill** (decision 5). Register in `init()`.
- `internal/domain/workspace.go`: `PermissionResourceAuditLogs = "audit_logs"`; `OptInPermissionResources = []PermissionResource{PermissionResourceAuditLogs}` right after `AllPermissionResources` with the rationale; `knownPermissionResources` (line 74) = All ∪ OptIn so `UserPermissions.Validate` accepts it and `HasPermission` works (absence = denied). `AllPermissionResources`, `NewFullPermissions`, `grantsFullPermissions` (`permissions_predicate.go:33`), telemetry `rbac_custom` (`telemetry_service.go:390`) untouched.
- `CHANGELOG.md` `## [41.0]` entry (section F).

### B. Domain — `internal/domain/audit_log.go` (+ `audit_actions.go`)

- `AuditLog` entity (fields above), `AuditChange{Old, New any}`, outcome / actor-type / auth-method constants, `DefaultAuditLogRetentionDays = 365`, `AuditLogExportMaxRows = 100_000`, `AuditLogDefaultListLimit = 50`, `AuditLogMaxListLimit = 100`.
- `AuditRecord` (request-scoped, mutex-guarded) with nil-safe methods: `SetActorClaims`, `SetActor(user *User, uw *UserWorkspace, isRoot bool)`, `SetWorkspace`, `SetTarget(typ, id, name)`, `SetChanges`, `AddMetadata`, `SetAuthMethod`, `Fail(reason)`, `Skip()`; `WithAuditRecord(ctx, *AuditRecord)`, `AuditFromContext(ctx) *AuditRecord` (`AuditRecordKey contextKey`).
- `AuditDiff(before, after map[string]any) map[string]AuditChange` (shallow; nested objects compared with `reflect.DeepEqual` and stored whole; keys matching `(?i)secret|password|passwd|token|api_key|apikey|credential|private|signature|access_key` → both sides `"[redacted]"`, change still visible; values above 2 KB become `"[changed, N bytes]"`; `created_at` / `updated_at` / `version` ignored) and `RedactAuditMap`.
- `AuditLogFilter{Scope (workspace|deployment), WorkspaceID *string, Actions, Categories []string, ActorID, ActorEmail, ActorType, Outcome, TargetType, TargetID, IPAddress string, From, To *time.Time, Cursor string, Limit int}`; `ListAuditLogsRequest.FromURLParams/Validate` (workspace scope requires `workspace_id`; malformed `from`/`to` is a 400; cursor = base64 `occurred_at~id` as in `message_history_postgre.go:685-730`); `ExportAuditLogsRequest{Filter, Format csv|ndjson}`; `ListAuditLogsResponse{Logs, NextCursor, HasMore}`; `AuditActionDescriptor{Action, Category, TargetType}`.
- `AuditLogSettings{RetentionDays *int}` + `ValidateForSave()` (nil ok, 0 ok, else 30..3650) + `EffectiveRetentionDays(default int)`; `WorkspaceSettings.AuditLogs *AuditLogSettings json:"audit_logs,omitempty"`, preserved across a partial `workspaces.update` by adding `"audit_logs"` to `preservableWorkspaceSettingKeys` (`workspace.go:1439`) and a copy case in `PreserveOmitted` (`workspace.go:~1495`); `SetAuditLogSettingsRequest{WorkspaceID, Settings}.Validate()`.
- Interfaces (mockgen directives at top): `AuditLogRepository{Insert, List, GetByID, Stream(ctx, filter, fn), Purge(ctx, workspaceID *string, before) (int64, error), PurgeOrphans(ctx, keep []string, before) (int64, error)}`; `AuditLogRecorder{Record(ctx, *AuditLog)}` + `NoopAuditLogRecorder`; `AuditLogService{List, Get, Export(ctx, req, w io.Writer, flush func()) (rows int64, truncated bool, err error), Actions() []AuditActionDescriptor}`.
- `audit_actions.go`: `AuditActionSpec{Category, TargetType, TargetKey string}`, `AuditedActions`, `AuditLoggedReads`, `AuditExcludedRoutes`, `AuditActionCatalogue() []AuditActionDescriptor` (sorted; includes the non-HTTP actions).

### C. Repository — `internal/repository/audit_log_postgres.go`

System-DB repo taking `*sql.DB` (like `NewUserRepository(a.db)`), Squirrel with `sq.Dollar`. `Insert` (single row), `List` (keyset pagination on `(occurred_at, id)`, filters map 1:1 to columns, `actor_email` `ILIKE`, `actions` → `sq.Eq{"action": []string}`, scope deployment = no workspace predicate unless a `workspace_id` filter is given), `GetByID`, `Stream` (pages of 1000 by cursor, callback per row, stops when the callback returns an error), `Purge` → `SELECT audit_logs_purge($1, $2)`, `PurgeOrphans` → `SELECT audit_logs_purge_orphans($1, $2)`. Column list + `scanAuditLog`; empty result is a non-nil slice; `changes::text` / `metadata::text` scanned into `json.RawMessage`.

### D. Recorder + service — `internal/service/audit_log_service.go`

- `AuditLogService{repo, entitlements domain.EntitlementProvider, authService, userRepo, workspaceRepo, settingRepo, isRootEmail func(string) bool, logger, now func() time.Time; mu; lastLicensed *bool}`; `isLicensedFor` copied verbatim from `template_service.go:169` (nil provider ⇒ true).
- `Record(ctx, e)`: `defer recover()`; compute `licensed`; if `lastLicensed` is set and differs, insert the marker row first (the one row an unlicensed install writes); update `lastLicensed`; if not licensed return; fill `ID` (32-char uuid), `OccurredAt` (UTC now if zero); **actor completion**: deployment-level routes (`settings.update`, `licence.set`, `user.logout`, OIDC) never call `AuthenticateUserForWorkspace`, so when `actor_id` is set and `actor_email` is empty look the user up with `userRepo.GetUserByID` and set `actor_role = root` when `isRootEmail(email)`; write with `context.WithoutCancel(ctx)` + 5 s timeout so a client that disconnects after the action still produces its row; `repo.Insert`; on error log at error level with `action`, `request_id` — **never** return an error (fail-open). Synchronous.
- `Init(ctx)` seeds `lastLicensed`, called from `app.InitServices` right after `licenseService`; add `"audit_logs_recording": ent.Has(domain.FeatureAuditLogs)` to the startup log at `license_service.go:597` beside `sso_gated`.
- `List` / `Get` / `Export` authorisation: workspace scope → `AuthenticateUserForWorkspace` + (`Role == "owner"` or `HasPermission(PermissionResourceAuditLogs, Read)`), else `NewPermissionError(audit_logs, read, …)`; deployment scope → `AuthenticateUserFromContext` + `isRootEmail`. **No licence check anywhere on the read path.** Export writes rows with `encoding/csv` (fixed header = column list, JSON columns serialised) or one JSON object per line, flushes every 500 rows, stops at the cap and returns `truncated=true`, and annotates the request record (enrichment 11).
- `PurgeExpired(ctx) (int64, error)`: for each workspace: days = `ws.Settings.AuditLogs.EffectiveRetentionDays(systemDefault)`; 0 ⇒ skip; `repo.Purge(&ws.ID, now-days)`; then `repo.Purge(nil, now-default)` and `repo.PurgeOrphans(existingIDs, now-default)`; when anything was deleted, `Record` one `audit.purged` (actor system) per scope. System default read from `settingRepo.Get("audit_logs_retention_days")` (absent ⇒ 365). No licence check anywhere in this path.
- `internal/service/audit_log_purge_worker.go`: template `web_analytics_maintenance_worker.go` (`initialDelay` 2 min, 24 h ticker, exported `RunOnce`, injectable `nowFn`); deps `service`, logger — **no `EntitlementProvider` field** (a reflect test pins it). Started from `app.Start` next to the maintenance worker. Multi-replica runs are idempotent.
- `SettingService.SystemConfig` gains `AuditLogsRetentionDays int` (key `audit_logs_retention_days`, missing ⇒ 365, 0 ⇒ forever), read/written by `settings.get` / `settings.update` (root-only path), validated with `AuditLogSettings` bounds.
- `WorkspaceService.SetAuditLogSettings(ctx, workspaceID, settings)` — copy of `SetWebAnalyticsSettings` (`workspace_service.go:1100`): authenticate, `HasPermission(PermissionResourceWorkspace, Write)`, `ValidateForSave`, `GetByID`, mutate, `Update`; enrichment 8.

### E. Middleware — `internal/http/middleware/audit.go` (inside the mux)

- `NewAuditMiddleware(recorder domain.AuditLogRecorder, logger) func(http.Handler) http.Handler`. **Mounting**: `tests/testutil/server.go:180,246` serves `app.GetMux()` directly and bypasses the `Start()` chain (`app.go:1490-1504`), so the middleware must live inside the mux. Three lines at the end of `InitHandlers`, after the 34 unchanged `RegisterRoutes(a.mux)` calls: `inner := a.mux; a.mux = http.NewServeMux(); a.mux.Handle("/", middleware.NewAuditMiddleware(a.auditLogService, a.logger)(inner))`. The SPA root handler (`root_handler.go:842` `HandleFunc("/")`) stays on the inner mux; `GetMux()`, `SetHandler()` and `Start()` are unchanged; the record pointer placed in the outer context is reachable from the per-route `requireAuth`. OPTIONS and non-`/api/` paths return after the request-id header.
- Per request: `X-Request-ID` (reuse incoming, else generate; echo on the response), client IP (move `getClientIP` from `internal/http/public_handler.go:652` to `middleware.ClientIP`, keep a one-line delegate), `r.UserAgent()`, new `AuditRecord` in ctx, `auditResponseWriter` capturing status **and forwarding `http.Flusher`** (llm chat, setup, settings SMTP test, export all stream).
- Classification: `route := strings.TrimPrefix(path, "/api/")`; POST + `AuditedActions[route]` → audited; GET + `AuditLoggedReads[route]` (+ `export=true` / empty cursor where required) → audited; else return without recording (data-plane requests pay a map lookup, nothing more).
- Body peek (audited POST with JSON content type only): read up to 1 MB into a buffer, replay with `io.MultiReader(buf, r.Body)`; shallow-decode `map[string]json.RawMessage`; default `workspace_id` and the route's `TargetKey` into the record (service enrichment overrides). Larger or non-JSON bodies skip extraction; the handler still receives every byte.
- After the handler: `outcome` from status (2xx success; 403 denied; 402 denied + `metadata.licence_refused`; 401 skip; else failure with `status_code`); on 402/403 the writer keeps the first 4 KB of the JSON body so `metadata.feature` / `required_tier` (402, `writeLicenseRequired`) or `resource` / `permission` (403, `utils.go:96-103`) can be recorded; build the `AuditLog` from the record (+ route category / target type); if `record.Skipped()` return; mark the record finalised (later `AuditFromContext` calls from handler goroutines become no-ops); `recorder.Record(ctx, event)`.
- `internal/http/audit_routes_test.go`: `TestEveryAPIRouteIsClassifiedForAudit` walks `internal/http/*.go` (non-test) for `"/api/…"` literals in `mux.Handle` calls and asserts each is in exactly one of `AuditedActions`, `AuditLoggedReads`, `AuditExcludedRoutes` — same style as `TestEntitlementProviderCallSitesAreListed`.

### F. Handler — `internal/http/audit_log_handler.go`

`AuditLogHandler{service, logger, getJWTSecret, isDemo}`; routes: `GET /api/auditLogs.list`, `GET /api/auditLogs.get`, `GET /api/auditLogs.actions`, `POST /api/auditLogs.export` (all `requireAuth`; export: authorisation before headers so a 403 is still JSON; then `Content-Type text/csv; charset=utf-8` or `application/x-ndjson`, `Content-Disposition attachment; filename="audit-logs-<scope|workspace>-<YYYYMMDD>.<ext>"`, `Cache-Control: no-store`, `X-Content-Type-Options: nosniff`, `X-Export-Truncated` when capped; first streaming download in the codebase — say so in the file). Verb checks satisfy `verb_guard_test.go`. Errors through `writeServiceError`. `POST /api/workspaces.setAuditLogSettings` in `workspace_handler.go` (`restrictedInDemo(requireAuth(...))`, mirror `handleSetWebAnalyticsSettings`). OpenAPI: `openapi/paths/audit-logs.yaml` + `openapi/components/schemas/audit-log.yaml` + refs in `openapi/openapi.yaml`; `make openapi-bundle`; a `audit_log_openapi_test.go` pins list limit and retention bounds (mirror `annotation_openapi_test.go`).

### G. Wiring — `internal/app/app.go`

Struct fields `auditLogRepo`, `auditLogService`, `auditLogPurgeWorker`; `InitRepositories`: `repository.NewAuditLogRepository(a.db)`; `InitServices`: construct right after `a.licenseService`, call `Init(ctx)`; inject into `settingsHandler` (system diff) and the workspace service (`SetAuditLogSettings`); `InitHandlers`: inner/outer mux (E) + `NewAuditLogHandler(...).RegisterRoutes(inner)`; `Start`: `go a.auditLogPurgeWorker.Start(ctx)`. `internal/app/app_license_wiring_test.go`: add the audit service to `licenceConsumers` (G6, a recorder, never a refusal).

### H. Licence bookkeeping

- `internal/domain/license.go`: `FeatureAuditLogs` comment (56-59) now says what it gates (recording only). Ledger under `# Call sites` → Gates: **G6** — `internal/service/audit_log_service.go, Record — FeatureAuditLogs`: records-or-not; an unlicensed deployment writes no row and is refused nothing; writes one marker row per transition; listing, export and purge never ask; the purge worker must never be given the provider. `# Never` paragraph: add that the recorder is consulted only by the HTTP middleware after the response and by the purge worker's own marker path, and can only skip a write — it cannot refuse, pause, send or delete; `PauseForCircuitBreaker` stays unrecorded for that reason. `TestEntitlementProviderCallSitesAreListed` must pass.
- `internal/service/entitlements_wiring_test.go` `fullyLicensedProvider` already grants `FeatureAuditLogs`.

## Console design (`console/`)

- `src/services/api/permissions.ts`: `"audit_logs"` in the `PermissionResource` union; new `OPT_IN_PERMISSION_RESOURCES = ['audit_logs']` (**not** in `ALL_PERMISSION_RESOURCES`, which mirrors Go order); `UNENFORCED_PERMISSIONS` += `["audit_logs", "write"]`; descriptor (endpoints `auditLogs.list`, `auditLogs.get`, `auditLogs.export`; write: "nothing to write, the log is append-only"); `createFullPermissions` / `createEmptyPermissions` keep iterating `ALL`; the matrix sends `{...matrix, audit_logs: {read, write: read}}`.
- `src/components/settings/PermissionsMatrix.tsx`: render opt-in resources in a "Governance" group after Workspace, write cell locked.
- `src/services/api/client.ts`: `api.download(endpoint, body?): Promise<Blob>` (same Bearer / endpoint logic, no JSON parse, error mapping on non-2xx); lift `downloadFile` from `ExportContactsModal.tsx:99` into `src/lib/download.ts`.
- `src/services/api/audit_log.ts` (template `inbound_webhook_event.ts`): `AuditLog`, `AuditChange`, `AuditLogListParams`, `AuditLogListResult`, `AuditActionDescriptor`; `listAuditLogs`, `getAuditLog`, `listAuditActions`, `exportAuditLogs(params, format)` (download → blob → `downloadFile`); `setAuditLogSettings` + `AuditLogSettings` in `workspace.ts` / `types/settings.ts`.
- `src/components/settings/SettingsSidebar.tsx`: `'audit-logs'` in `SETTINGS_SECTIONS`; menu item (icon `AuditOutlined`) pushed when `canReadAuditLogs` (new prop; `WorkspaceSettingsPage.tsx` computes `isOwner || member.permissions?.audit_logs?.read` from the same members query as `isOwner`).
- `src/pages/WorkspaceSettingsPage.tsx`: `case 'audit-logs': <AuditLogsSettings workspace isOwner isRoot />`; content `maxWidth` 100% for this section (escapes the 700 px cap).
- New `src/components/settings/audit_logs/`:
  - `AuditLogsSettings.tsx` — `SettingsSectionHeader`; `<LicenceGateNotice feature="audit_logs" workspaceId />` above a still-working table when `!has('audit_logs')`; Tabs **Workspace** / **Deployment** (root only, `scope=deployment`, extra Workspace column + filter); toolbar filters, `Export` dropdown (CSV / NDJSON, current filters, cap note); `AuditLogRetentionCard` for owners.
  - `AuditLogTable.tsx` — pattern `InboundWebhookEventsTab.tsx`: Popover filters (date `RangePicker`, action Select grouped by category from `auditLogs.actions`, outcome, actor email, actor type, target id, IP); `useQuery(['audit-logs', scope, workspaceId, filters, cursor])`, accumulate pages, Load more; columns time (`dayjs().fromNow()` + tooltip), actor (email + role tag), action (`<code>`), target (`type: name`), outcome Tag (green / red / orange), IP; row click opens the drawer; `?id=` opens it directly; `Table styles={{ wrapper: { maxWidth: '100%' } }}`.
  - `AuditLogDetailsDrawer.tsx` — `Descriptions` actor / context / target; `AuditChangesTable` rendering `key: old → new` (port of `ContactTimeline.tsx:462-490`, JSON values in a tooltip, `[redacted]` as a Tag); raw JSON `<pre>` (`OutgoingWebhooksTab.tsx:502` idiom); copy-link button.
  - `AuditLogRetentionCard.tsx` — `InputNumber` (empty = default 365, 0 = forever, 30–3650) + `SettingsSaveBar` (pattern `WebAnalyticsSettings.tsx:350`).
  - `actionLabels.ts` — category → colour, action → label (falls back to the raw action).
- `src/components/settings/SystemSettingsDrawer.tsx`: root-only "Audit log retention (deployment default)" `InputNumber` bound to `audit_logs_retention_days`.
- `ExportContactsModal.tsx`: `export: true` on the first `contactsApi.list` call (cursor undefined) so the server records `contacts.export` once.
- i18n: every string in `t\`\`` / `<Trans>`; `npm run lingui:extract`; **French** translations for every new string (`catalogues.test.ts` fails otherwise; the licence-folder exemption does not cover `components/settings/audit_logs/`).

## Licence, docs, marketing, changelog

- `notifuse/LICENSE` Additional Use Grant: `(6) recording an audit log of administrative actions.` `LICENSING.md`: "five" → "six", item 6.
- `docs/self-hosting/licence.mdx` (separate repo, own PR): "Five" → "Six"; "What needs a key" row **Audit logs** — "Recording. Without a key nothing is written; the page, the export and the settings stay available and simply show an empty log. A marker row says when recording stopped and resumed."; tier grid `coming soon` → `✓` (Enterprise).
- `docs/features/audit-logs.mdx` (new; `docs.json` nav after `features/logs`): what is recorded (catalogue table generated from `AuditActionCatalogue()`), what never is (data plane, circuit-breaker pause), row fields, who can read (owners, opt-in `audit_logs` grant, root deployment view), filters, export formats + cap, retention, append-only guarantee and its limit (table owner / superuser can bypass; no hash chain yet), licence behaviour + markers, API section (`api-reference` group "Audit logs", regenerate `docs/openapi.json` via `make openapi-bundle`).
- `homepage/src/data/licensing.ts`: remove `audit_logs` from `comingSoon`; add the sixth `licensedCapabilities` entry (EN + FR; `refusedAt`: "Nothing is refused — an unlicensed deployment simply records nothing; the page and the export stay available"); fix the comment at lines 81-82; "Five capabilities" copy in `pages/pricing/self-hosted.astro` and `fr/tarifs/auto-heberge.astro`.
- `CHANGELOG.md` `## [41.0] - <date>`: **Feature** (audit logs: what, who, filters, export, retention, append-only, request ids, opt-in read grant), **Change** (sixth Licensed Feature in the v41 LICENSE — recording only; v40 and earlier untouched), **Improvement** (`X-Request-ID` echoed on every API response). House voice as in the v40 entry; no `Fix` bullets for unshipped code.

## Tests (per file)

| Implementation file | Test file | Cases |
|---|---|---|
| `internal/database/schema/audit_logs_tables.go` | `schema/audit_logs_tables_test.go` | every statement idempotent; no REFERENCES / CHECK; guard raises on UPDATE and TRUNCATE, DELETE branch references `AuditPurgeGUC`; purge functions present; `TableNames` includes `audit_logs`; pinned statement count |
| `internal/migrations/v41.go` | `migrations/v41_test.go` | metadata (41.0, system=true, workspace=false), registered, selection `>40 && <=41` = `[41.0]`, `MatchesTheCodeVersion`, sqlmock: SQL executed equals `AuditLogsTableDefinitions()` byte for byte, `DoesNotTouchPermissions` (no `UPDATE user_workspaces`) |
| `internal/domain/audit_log.go`, `audit_actions.go` | `domain/audit_log_test.go` | `FromURLParams` parsing, limit clamp, bad `from`/cursor → error, scope rules; `AuditDiff` redaction table (`password`, `smtp_password`, `client_secret`, `access_key_id`, `Token`), size cap, ignored keys, nested whole; `AuditRecord` nil-safety + concurrent `AddMetadata`; `AuditLogSettings.ValidateForSave` (nil, 0, 29, 30, 3650, 3651); catalogue sorted / unique / known categories |
| `internal/domain/workspace.go` | `domain/workspace_test.go` (+ `service/permissions_predicate_test.go`) | `TestAllPermissionResources` unchanged (14); opt-in: `Validate` accepts `audit_logs`, `NewFullPermissions` lacks it, `HasPermission` false unless granted / owner; `grantsFullPermissions` ignores opt-in keys; `audit_logs` settings preserved on partial update |
| `internal/repository/audit_log_postgres.go` | `repository/audit_log_postgres_test.go` (sqlmock) | insert SQL; each filter → WHERE clause; keyset cursor (limit+1, has_more, invalid cursor); stream batches; purge / purge-orphans call the SQL functions |
| `internal/service/audit_log_service.go` | `service/audit_log_service_test.go` (gomock) | licensed / grace / nil provider → insert; unlicensed → no insert; transition → marker then state, both directions; repo error swallowed; cancelled ctx still writes; actor email/name completed from `userRepo` when only claims are known, role root for `isRootEmail`; List/Get/Export authorisation (member without grant 403, member with grant ok, owner ok, root deployment ok, non-root deployment 403); read path never calls the provider (mock with zero `EXPECT`s); export CSV header/rows, NDJSON, cap + truncated, export event recorded; `PurgeExpired` per-workspace days, nil → default, 0 skips, deployment default, orphans, `audit.purged` recorded only when rows deleted; provider never called in purge |
| `internal/service/audit_log_purge_worker.go` | `service/audit_log_purge_worker_test.go` | interval / initial delay, stop on cancel, `RunOnce` delegates; reflect: no `EntitlementProvider` field |
| `internal/service/workspace_service.go` (`SetAuditLogSettings`, enrichments 4-8) | `workspace_service_test.go` | permission check, validation, settings persisted; record target / changes populated; API key token never in metadata; integration credentials `[redacted]` |
| `auth_service.go`, `user_service.go`, `oidc_service.go`, `setting_service.go`, `license_service.go`, `contact_service.go`, `custom_event_service.go` | existing `_test.go` files | actor set on auth (role root for `isRootEmail`); failure reasons without changing responses; SMTP / OIDC secrets redacted; licence metadata without the key; import counts; `SystemConfig` retention default / round-trip |
| `internal/http/middleware/audit.go`, `auth.go` | `middleware/audit_test.go`, `auth_test.go` | request id generated / echoed; `ClientIP` cases; excluded route and OPTIONS not recorded; audited POST recorded with actor / outcome / status; 403 → denied with `resource` / `permission`; 402 → denied + `feature`; 401 skipped; body peek replays every byte (200 KB body) and extracts `workspace_id` / target; > 1 MB still reaches the handler; Flusher forwarded; logged read with `export=true` only on first page; `record.Skip()` honoured; finalised record ignores late enrichment; `RequireAuth` populates actor claims; recorder panic does not break the response |
| `internal/http/audit_routes_test.go` | — | every `/api/` route classified exactly once |
| `internal/http/audit_log_handler.go` | `http/audit_log_handler_test.go` (httptest) | verbs; list / get / actions JSON; bad `from` 400; permission 403; export headers, disposition, streaming + flush count, 403 before any byte, truncated header |
| `internal/http/workspace_handler.go`, `settings_handler.go` | existing tests | `setAuditLogSettings` decode / validate / 403; system settings retention round-trip + range; `system.settings` diff redaction |
| `internal/app/app.go` | `app_license_wiring_test.go`, `app_test.go` | audit service in `licenceConsumers`; a request through `GetMux()` carries `X-Request-ID` |
| `internal/domain/license.go` | `license_test.go` | call-site list includes the recorder (G6); console tier mirror unchanged |
| `tests/integration/audit_log_test.go` (`integration,licdev`) | — | licensed: root signs in → `user.rootSignin` row; invite member → row with actor / target / changes; `contacts.upsert` → no row; scoped key 403 → denied row; member without grant 403 on list, with grant 200, root deployment scope 200, non-root 403; export CSV rows + `auditLogs.export` row; `UPDATE audit_logs` raises; purge function deletes only older rows; unlicensed harness: no rows, list 200 empty; transition marker present after swapping providers |
| Console: `audit_log.ts`, `client.ts`, `permissions.ts`, `PermissionsMatrix.tsx`, `AuditLogsSettings.tsx`, `AuditLogTable.tsx`, `AuditLogDetailsDrawer.tsx`, `AuditLogRetentionCard.tsx`, `actionLabels.ts`, `SettingsSidebar.tsx`, `WorkspaceSettingsPage.tsx`, `SystemSettingsDrawer.tsx` | sibling `*.test.ts(x)` | query-string building; `api.download` sends Bearer / returns Blob / maps 403; opt-in absent from `createFullPermissions`, write unenforced, descriptors cover ALL ∪ OPT_IN (update `PermissionsMatrix.test.tsx:69-73`, `workspace.test.ts:85,156`, `WorkspaceMembers.test.tsx` "does not count audit_logs toward Full Access"); gate notice when unlicensed and list still requested; Deployment tab root-only; rows render + Load more + filter resets cursor; drawer old→new and `[redacted]`; export with active filters; retention validation + save; sidebar walk test + hidden without grant; page routes `/settings/audit-logs`; system retention field round-trips; smoke test mocks `audit_log` service |

## Verification

1. `go generate ./internal/domain/...`; `make test-domain test-service test-repo test-http test-migrations test-database test-pkg`; `go test -race ./internal/app/...`.
2. `make test-licence-dev` and `make test-integration` (`integration,licdev`).
3. `cd console && npm run lingui:extract && npm test && npm run build`.
4. `make openapi-bundle && make openapi-lint`; copy `openapi.json` to the docs repo; open the docs PR separately.
5. Manual with a dev Enterprise key: invite a member, change permissions, create an API key, delete a contact, hit a 403 with a scoped key; open Settings → Audit logs, filter, open a row, export CSV and NDJSON, check `X-Request-ID` on responses; remove the key → `licence.recordingStopped` appears and further actions are not recorded; reinstall → `licence.recordingResumed`; `psql`: `UPDATE audit_logs …` fails with insufficient_privilege; unlicensed install shows the gate notice above an empty, working table.
6. `cd web_analytics_sdk && npm run build` and commit `dist/` + `package.json` with the VERSION bump.

## Deferred (later release, when an Enterprise buyer asks)

- SHA-256 hash chain + verify endpoint (append-only trigger is the v41 guarantee).
- Streaming: `audit_log.recorded` webhook event type dispatched from Go, or SIEM drains (Datadog / Splunk / S3).
- Old/new diffs for content resources (templates, broadcasts, automations …); v41 records action + target id only.
- Recording `PauseForCircuitBreaker` (send path must not consult the licence).
- Logging who *viewed* the audit log (export is logged; page views are not).

## Risks and gotchas

- `TestV40Migration_MatchesTheCodeVersion` fails the moment `VERSION` is bumped without `v41.go`; do both in one commit.
- `TestEntitlementProviderCallSitesAreListed` requires the G6 entry and that only `audit_log_service.go` mentions the provider (not the middleware, worker or repository).
- Opt-in resource: `TestAllPermissionResources` (pins 14) and the console `ALL_PERMISSION_RESOURCES` mirror stay unchanged; console tests asserting "keys sent == ALL" move to ALL ∪ OPT_IN.
- The audit response writer must forward `http.Flusher`.
- Body peek is bounded (1 MB) and only on audited POST routes; imports above the cap lose target extraction, never bytes.
- Middleware must be inside the mux or the integration harness (`GetMux()`) never sees it.
- Multi-replica: purge worker runs on every replica (idempotent); marker rows are per-process state, so a transition may produce one marker per replica — acceptable, documented.
- Docs live in a separate repo: write `docs/features/audit-logs.mdx` and the licence page edits, and open that PR explicitly.
