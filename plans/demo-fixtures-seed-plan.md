# Demo fixtures on any environment (`demo.seed`) — plan

## Problem

preview.notifuse.com should carry the same fixtures as demo.notifuse.com (the Apple workspace: 1000 contacts, templates, broadcasts with history, segments, automations, 400 days of web analytics) while staying a working preview: emails sent to Mailpit, workers running, Pierre signing in as root.

Today the only way to get the fixtures is `GET /api/demo.reset`, and it does not fit preview:

- The route is registered only when `ENVIRONMENT != production` (`internal/app/app.go:1749`). Preview runs `ENVIRONMENT=production` and must keep it: off production, `SignInWithEmail` returns the magic code in the response instead of mailing it (`internal/service/user_service.go:162`). Preview is public and its `ROOT_EMAIL` holds Pierre's addresses, so any other value lets anyone sign in as root. `ENVIRONMENT=demo` would also stop the email queue, the automation scheduler and the webhook delivery worker (`app.go:1935-1987`), make `EmailService.SendEmail` a no-op and close the mutating routes.
- `ResetDemo` deletes every workspace before seeding (`internal/service/demo_service.go:177`). Preview has `test` (release testing through Mailpit) and `tata`, which must survive.

The seed itself (`createDemoWorkspace`) is already almost environment-independent. Its one environment gate, `activateDemoAutomations` (`demo_automations.go:435`), keeps the seeded automations in draft outside `ENVIRONMENT=demo`, which is exactly what preview needs since its scheduler runs.

## Decisions

- **D1 (Pierre, 2026-11-15)**: no environment flag. A signed hit on `/api/demo.seed` is enough, on every environment, production included.
- **D2 (Pierre, 2026-11-15)**: preview's fixtures are refreshed every night by a Cloud Scheduler job, like `reset_demo` for the demo.
- **D3 (Pierre, 2026-11-15)**: implementation straight after the plan, on `feat/demo-fixtures-seed` from `origin/dev`.

## Why D1 holds, and the guard it needs

The HMAC is `ComputeEmailHMAC(PrimaryRootEmail, SECRET_KEY)` (`demo_service.go:164`). The system `SECRET_KEY` signs nothing else of this shape: every other `VerifyEmailHMAC` call uses a workspace secret key (`list_service.go:327`, `:613`, `notification_center_service.go:48`, `:131`), and `user.rootSignin` signs `email:timestamp` within a 60 s window (`user_service.go:303-315`). Forging a seed call therefore needs `SECRET_KEY`, which only the operator holds.

What the flag would have protected against, and what replaces it:

1. **A real workspace whose ID is `demo`.** On a self-hosted production instance, `demo` is a plausible name for a customer workspace. Replacing it would destroy real data. Guard: the seed replaces a `demo` workspace only if the system `settings` key `demo_fixtures_workspace_created_at` holds that workspace's `created_at` (RFC 3339, microseconds), written by the seed itself. The creation time rather than the ID: a fixtures workspace deleted by hand and a real `demo` created afterwards share the ID and would inherit an ID-only marker. A `demo` workspace whose creation time does not match → `409 Conflict`, nothing deleted. The marker is written twice (after the first code review): a pending one with the time `CreateWorkspace` is called, because `CreateWorkspace` can fail after inserting the row (owner membership, owner contact, default list) without removing it, then the exact `created_at`; a workspace matches when its `created_at` is within one minute after the marker. Both writes are detached from the request. On `ErrWorkspaceAlreadyExists` the previous marker is put back. The previous fixtures are deleted through `WorkspaceService.DeleteWorkspace` as root (integrations, then tasks, then the workspace), not the repository directly. The handler's debounce counts attempts: a failed attempt's answer is replayed to retries inside the window without rerunning, so a deterministic failure no longer reruns a reset (which deletes every workspace) on every scheduler retry.
2. **A leaked token.** The HMAC is static (no timestamp, because Cloud Scheduler sends a fixed request) and travels in the query string, so it can end up in proxy logs. A leak lets someone refresh the fixtures workspace at most every 5 minutes (2-3 min of CPU each time). Nothing else is reachable: other workspaces are never touched (guard 1), and no session or token is returned. Accepted as an operator-held credential (see [[feedback_operator_responsibility_severity]]); rotating `SECRET_KEY` revokes it.
3. **Quota.** The fixtures workspace counts toward the workspace quota (`workspace_service.go:257-275`, community limit 3). Preview has 2 workspaces today, so 3 with `demo`. Replacing an existing `demo` deletes before creating, so the count does not grow. A quota refusal surfaces as a 500 with the reason.

`/api/demo.reset` keeps its current registration (non-production only): it deletes every workspace and has no business on a production host.

- **D4 (Pierre, 2026-11-15) — licence on preview.** Seeded templates carry `fr`/`es` translations, and `TemplateService.CreateTemplate` refuses translations without `FeatureTemplateI18n` (`template_service.go:230`). Preview gets demo's `NOTIFUSE_LICENSE_KEY`: licence claims (`pkg/license/license.go:111`) carry no host, so the key is not bound to the demo host. The seed keeps a single code path; an unlicensed instance gets a 500 naming the refusal (D5).
- **D5 (Pierre, 2026-11-15) — a half seed fails loudly.** `createDemoWorkspace` returns the `addSampleData` error instead of logging it (`demo_service.go:327-338`), for both seed and reset: 500, Scheduler job marked failed (see [[feedback_no_log_only_warnings]]).
- **D6 (Pierre, 2026-11-15) — HMAC in `?hmac=` only**, the reset's convention. No header variant.

## Design

### Service — `internal/service/demo_service.go`

- `const demoFixturesWorkspaceID = "demo"` and `const demoFixturesSettingKey = "demo_fixtures_workspace_created_at"`; `createDemoWorkspace` uses the constant instead of the literal and returns the sample-data error (D5).
- `var ErrDemoWorkspaceNotFixtures = errors.New(...)` (exported, the handler maps it to 409).
- `func (s *DemoService) SeedFixtures(ctx context.Context) error`:
  1. `workspaceRepo.GetByID(ctx, "demo")`. Not found → step 3.
  2. Found → read the marker through the settings repository. Missing, unparsable or not equal to the workspace's `created_at` → `ErrDemoWorkspaceNotFixtures`. Present → `deleteWorkspace(ctx, "demo")` (the per-workspace body of `deleteAllWorkspaces`, extracted: `workspaceRepo.Delete` + `taskRepo.DeleteAll`); a delete failure aborts the seed (unlike the reset loop, which carries on).
  3. `createDemoWorkspace(ctx)`, which writes the marker right after `CreateWorkspace` succeeds and before the sample data, so a seed that fails halfway leaves a workspace the next seed may replace.
- `ResetDemo` is unchanged except that it goes through the same `createDemoWorkspace`, so the demo host gets the marker too (harmless).
- `DemoService` gains a settings repository dependency (`domain.SettingRepository`, already implemented by `internal/repository/setting_repository.go`), wired in `app.go:1215`.
- Delete then re-create of the same database name: on the demo the workers are off; on preview they run. The race that used to recreate a dropped database (`EnsureWorkspaceDatabaseExists`, see [[local-dev-env]]) is closed on `dev` by `3dea2b28`/`060b9ae3` (databases created only for registered workspaces, under the name lock). The integration test below exercises it with the workers on.

### Handler — `internal/http/demo_handler.go`

- New route `/api/demo.seed` (GET, same HMAC check before the lock, same `TryLock` and 200-on-skip semantics as the reset, see `project_demo_reset_429`).
- One mutex shared by reset and seed (they must never run together); separate `lastSeed` debounce (5 min).
- `ErrDemoWorkspaceNotFixtures` → 409 with a message that names the workspace and says nothing was deleted.
- Registration: `RegisterRoutes(mux, resetEnabled bool)` — seed always, reset only when `resetEnabled`. `app.go:1749` becomes an unconditional `demoHandler.RegisterRoutes(a.mux, !a.config.IsProduction())`.

### Test harness — `tests/testutil/server.go`

The harness server has a 30 s `WriteTimeout`, the application's none (`app.go:1838`). A seed runs 50-100 s: the harness cut the connection and Go's transport silently retried the GET, which then met the debounce (`skipped`). `ServerManager.SetWriteTimeout` lifts it for the next `Start`/`Restart`; the default is unchanged for every other test. Audit classification: `demo.seed` joins `demo.reset` in `AuditExcludedRoutes` (`TestEveryAPIRouteIsClassifiedForAudit`). `cmd/hmac` also prints the seed URL.

### Ops (after merge and deploy of preview)

- Preview env (Coolify app `oc8ksgokss4cooko0sg00844`): `NOTIFUSE_LICENSE_KEY` (D4). Nothing else changes; `ENVIRONMENT` stays `production`.
- HMAC: computed on the Coolify host from the preview container's env (`ROOT_EMAIL` first entry, `SECRET_KEY`), HMAC-SHA256 hex like `crypto.ComputeHMAC256`, so the secret never leaves the server.
- First seed by hand with curl, then the Cloud Scheduler job `seed_preview_fixtures` (project `notifusev3`, `europe-west1`): every night at 04:00 Europe/Paris, `GET https://preview.notifuse.com/api/demo.seed?hmac=…`, `attemptDeadline` 600 s (the demo's reset takes ~160 s against 180 s, see `project_demo_reset_429`), bounded retries (`retryCount` 2) instead of the reset job's unlimited ones.
- No `vN.go`, no `VERSION` bump. No CHANGELOG entry: operator tooling with no user surface (see [[feedback_changelog_only_user_visible]]). No architecture flow touched.

## Files and tests

| Implementation | Test file | Cases |
|---|---|---|
| `internal/service/demo_service.go` | `internal/service/demo_service_test.go` (+ `demo_service_reset_full_test.go` for the reset path) | `SeedFixtures` with no `demo` workspace creates it and writes the marker; with a marked `demo` deletes it (workspace + tasks) then re-creates it; with an unmarked `demo` returns `ErrDemoWorkspaceNotFixtures` and calls neither `Delete` nor `CreateWorkspace`; a `Delete` failure aborts before `CreateWorkspace`; other workspaces are never listed nor deleted; marker written before `addSampleData` runs (a sample-data failure still leaves the marker); `ResetDemo` writes the marker; a sample-data failure is returned by both `SeedFixtures` and `ResetDemo` (D5) |
| `internal/http/demo_handler.go` | `internal/http/demo_handler_test.go` | seed: method not allowed, missing HMAC 400, invalid HMAC 401 checked before the lock, success 200 `status: seeded`, recent seed 200 `skipped`, seed while a reset holds the lock 200 `in_progress` (and the reverse), `ErrDemoWorkspaceNotFixtures` → 409, other error → 500; `RegisterRoutes(mux, false)` registers seed but not reset |
| `internal/app/app.go` | `internal/app/app_test.go` (new test; no existing one covers the demo routes) | production config: `/api/demo.seed` answers 400 without HMAC (route present), `/api/demo.reset` answers 404 |
| — | `tests/integration/demo_seed_test.go` (new) | production environment, workers started, an existing workspace `test` with a contact: signed seed → 200, workspace `demo` exists with contacts/templates/broadcasts, `test` and its contact untouched, seeded automations are draft; second seed (after the debounce, via a fresh handler) replaces `demo` without a duplicate-database error; an unmarked `demo` workspace → 409 and its data intact |

## Steps

1. Branch `feat/demo-fixtures-seed` from fetched `origin/dev`; move this plan onto it.
2. Service: constants, `deleteWorkspace` extraction, settings dependency, `SeedFixtures`, marker in `createDemoWorkspace`, D5; mocks regenerated (`go generate`) if the settings repository mock is missing.
3. Handler + `app.go` registration.
4. Unit tests above, then the integration test.
5. Run: `make test-service`, `make test-http`, `go test ./internal/app/...`, and the integration test alone: `INTEGRATION_TESTS=true go test -tags integration,licdev ./tests/integration -run 'TestDemoSeed' -v` (Postgres + Mailpit from `docker compose -f tests/compose.test.yaml up -d`), plus the existing `TestDemoAutomationsSeedRealHistory` and `TestDemoWebActivityOnTheContactTimeline` to check the seed refactor did not regress.
6. `/code-review` on the final diff, then commit + PR to `dev` on Pierre's go.
7. After merge and deploy to preview: D4 env change, first seed by curl, check in the console (workspace Apple, Mailpit receives a test send), then the Scheduler job.
