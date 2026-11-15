# Integration tests speedup plan

Status: all decisions taken (D1-D5, 2026-11-15); `-race` stays. A + B implemented on `chore/integration-tests-speedup` (full local suite green, 2026-11-15); C implemented on `ci/integration-test-shards`. Not committed yet. A SQL dump of the demo fixtures was considered and rejected (dates relative to today, frozen compiled triggers/segments, per-instance encrypted SMTP secret).

## Diagnosis (measured, CI run 43727040717 on dev, 2026-11-15)

The `Integration Tests` job took 45 min: ~2m45 of checkout, dependencies and `-race` compilation, then 39 min of tests run one after the other. Before #591 (demo fixtures seed) the job took ~33 min.

- The whole suite is one package (`./tests/integration`, 410 top-level tests). Only `connection_pool_limits_test.go` calls `t.Parallel()`, so Go runs everything sequentially.
- `TestDemoSeedOnAProductionInstance` takes 600 s. It seeds twice, and each seed generates 100 000 web-analytics sessions (`demoWebAnalyticsSessions`, `internal/service/demo_web_analytics.go:26`) in ~4m40. The test asserts nothing about that volume.
- ~54 subtests take 10-12 s each. `ScheduleBroadcastTask` makes a send-now task due `sendNowTaskDelay` (5 s, `internal/service/task_service.go:1229`) in the future, and `WaitForBroadcastStatusWithExecution` (`tests/testutil/helpers.go:577`) only executes pending tasks every 2 s + 500 ms. A broadcast therefore waits ~7 s before its first run and ~2.5 s more for its second pass.
- ~310 short tests cost 1-3 s each, mostly creating a fresh system DB and workspace DB.

Duration buckets (top-level tests): ≥30 s: 14 tests / 1197 s; 10-30 s: 39 / 624 s; 5-10 s: 23 / 146 s; 1-5 s: 289 / 370 s.

## Why not `t.Parallel()` inside the package

- One shared Mailpit, and 23 files call `ClearMailpitMessages` (DELETE of every message): parallel tests would wipe each other's mail.
- Some workspace IDs are fixed (`demo` in the demo seed tests).
- Postgres `max_connections` is 100 and every test opens its own pools.

CI sharding (one Postgres + one Mailpit per job) sidesteps all three without touching test isolation. The repository is public, so the extra runner minutes are free.

## Target

| Step | Expected job duration |
|---|---|
| Today | ~45 min |
| A. Demo seed volume configurable | ~35 min |
| B. Send-now delay configurable + faster helper | ~29 min |
| C. 4 shards balanced on measured durations | ~10 min |

These are estimates from the log above; step D measures the real numbers.

## Decisions to confirm

- **D1. Where the knobs live.** Decided 2026-11-15: plain fields on existing config structs, never read from the environment, zero value = production default. `config.DemoConfig.WebAnalyticsSessions int` and `config.TaskSchedulerConfig.SendNowDelay time.Duration`, where 0 or less keeps the 5 s default (a plain value suffices since D4 set the harness to 1 s, not 0). Alternative: setters on the services called by the harness through `GetApp()`. Config fields follow the existing precedent (`TaskScheduler.Interval` is shortened by the harness).
- **D2. Shard count.** Decided 2026-11-15: 4. Wall time ≈ compile overhead (~3 min) + 29 min / N; 4 gives ~10 min, 6 gives ~8 min for 50 % more jobs.
- **D4. Send-now delay in the harness.** Decided 2026-11-15: 1 s, not 0. The 5 s delay covers the window where the task commits before its broadcast; 1 s keeps a non-zero margin so the tests still run the same ordering as production, while saving 4 s per broadcast. Checked: no integration test pins the 5 s (the `NextRunAfter` assertions are on scheduled broadcasts).
- **D5. Sequencing.** Decided 2026-11-15: two PRs to `dev`, one after the other. PR 1 = A + B (code, measured in CI on its own); PR 2 = C (workflow + shard tool), opened once PR 1 is merged, so each gain is measured and each change can be reverted alone. Work happens in a git worktree from `origin/dev`, leaving `feat/contacts-ui-polish` untouched.
- **D3. Balancing source.** Decided 2026-11-15: a committed `tests/integration/testdata/durations.json` (test name → seconds), refreshed by hand with a make target. Stale or missing entries only hurt balance, never correctness.

## A. Demo seed volume

Files:
- `config/config.go`: add `WebAnalyticsSessions int` to `DemoConfig` (comment: test-only, 0 = `demoWebAnalyticsSessions`).
- `internal/service/demo_web_analytics.go`: `seedWebAnalytics` uses `s.config.Demo.WebAnalyticsSessions` when > 0. Keep `Days` at 400 so the span-dependent behaviour stays covered.
- `tests/testutil/server.go`: `NewServerManager` sets `Demo.WebAnalyticsSessions = 2000`.

Tests:
- `internal/service/demo_web_analytics_test.go` (or the existing generator test file): seeding with `WebAnalyticsSessions = 0` asks the generator for 100 000 sessions; with 500 it asks for 500.
- `tests/integration/demo_seed_test.go`: unchanged assertions; check it now runs in seconds. Also run `TestDemoWebActivityOnTheContactTimeline` and `TestDemoAutomationsSeedRealHistory`, which read the same seed.

Trap: check nothing in the integration suite counts the seeded sessions or relies on the volume (grep `sessions`, `pageviews` in `tests/integration/demo_*`).

## B. Send-now delay

Files:
- `config/config.go`: add `SendNowDelay time.Duration` to `TaskSchedulerConfig` (0 or less = default).
- `internal/service/task_service.go`: `sendNowTaskDelay` becomes the default; `SetSendNowDelay` (same pattern as `SetAutoExecuteImmediate`, called from `internal/app/app.go`) stores it, `ScheduleBroadcastTask` uses it. Update the constant's comment.
- `internal/app/app.go:955`: pass the configured delay.
- `tests/testutil/server.go`: both `NewServerManager` and `NewServerManagerWithLiveScheduler` set it to 1 s (D4).
- `tests/testutil/helpers.go`: `WaitForBroadcastStatusWithExecution` executes tasks every 500 ms instead of 2 s, polls the status every 250 ms instead of 1 s, sleeps 100 ms instead of 500 ms after an execution, and logs only failures and status changes.

Tests:
- `internal/service/task_service_schedule_test.go`: the existing assertion (NextRunAfter ≈ now + 5 s) keeps passing with the default; add one case where the configured delay is 0 and NextRunAfter ≈ now.
- `internal/app` wiring: covered by the integration suite.
- Integration: the delay protects the case "the task runs before the broadcast commits; it finds a draft and hands the task back". Grep done 2026-11-15: no test pins the 5 s. Run every broadcast test (`-run 'Broadcast|DataFeed|RecipientFeed|Liquid|Mailjet|ABTest|Pause|Circuit'`) twice to flush out new races; per memory, automation asserts already race the 500 ms scheduler.

## C. CI sharding

Files:
- `tests/shard/main.go` (new, package `main`): reads test names (output of `-test.list`) on stdin, reads `durations.json`, assigns tests to N shards by longest-first greedy (unknown tests weigh the median), prints the anchored regex `^(TestA|TestB|…)$` for shard i. Deterministic for a given input.
- `tests/shard/main_test.go`: every listed test lands in exactly one shard (union = input, no duplicates) for N = 1..8; a test missing from durations.json is still assigned; same input → same output; the heaviest test is alone when it outweighs the rest divided by N; a regex-special character in a name is escaped.
- `tests/integration/testdata/durations.json` (new): generated from the 2026-11-15 run.
- `Makefile`: `integration-durations` target that turns a `go test -json` output into durations.json.
- `.github/workflows/go.yml`: the `integration-tests` job becomes a matrix `shard: [0, 1, 2, 3]` (name `Integration Tests (shard N/4)`), `fail-fast: false`, each with its own postgres + mailpit services. Each shard: `go test -c -race -tags integration,licdev -o integ.test ./tests/integration`, then **from `tests/integration/`** (testdata paths are relative) `../../integ.test -test.list '.*' | go run ./tests/shard …` and `../../integ.test -test.run "$REGEX" -test.v -test.timeout 30m`. A final `Integration Tests` job (`needs`, `if: always()`) fails unless every shard succeeded, so there is still one check to look at. Drop `-coverprofile` (its upload is already commented out) or keep it per shard as an artifact.

Traps:
- Run the binary with `-test.v` (or through `go tool test2json`) so the logs keep the `--- PASS` lines used for triage.
- A panic in one shard now truncates only that shard, but the memory rule still applies: diff tests that ran against `-test.list` before trusting a run. Add a step that does that diff automatically per shard.
- `-test.list '.*'` lists top-level tests only, which is what `-run` needs.

## D. Verify

1. Unit: `make test-service` (task_service, demo service), `go test ./tests/shard/...`, `make test-domain` if config types are touched.
2. Integration (local, `docker compose -f tests/compose.test.yaml up -d`, `INTEGRATION_TESTS=true`, `-tags integration,licdev`): the demo tests and the broadcast tests listed in B, twice.
3. CI: push the branch, read the four shard durations and the summary job; compare the per-shard test lists against `-test.list` (no test lost, none run twice); compare the total against today's 45 min.
4. Code review on the final diff before proposing a commit.
