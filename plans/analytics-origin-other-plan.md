# Analytics origin `other`, aligned with the Logs filter

## Why

The analytics `origin` dimension of `message_history` (server part 1, unreleased 44.0) sorts a message by
`broadcast_id`, then `automation_id`, and calls everything else `transactional`. The `origin` filter of
`messages.list` and `messages.count` (server part 5, same release) only calls `transactional` a message with a
`transactional_notification_id`, and gives the messages sent by none of the three (template tests, double
opt-in confirmations) their own value, `other`. The dashboard and the Logs page would then show different
transactional figures for the same period. Pierre chose to align the analytics on `other`.

The dashboard of the console reads the dimension one origin at a time (`useOriginSeries`) and sums the
origins it asks for into its KPIs (`computeDashboardKpis`). If it kept asking for three origins, the messages
that leave `transactional` would leave the dashboard's totals too: it asks for a fourth series, `other`.

Also in this PR: the `RequireBound` comment (part 5) names the accepted fields instead of "a filter on an
indexed column", which was wider than the code (`created_before` alone, `is_failed`, `is_bounced`,
`is_complained` can use an index and are refused).

## Changes (as built, after the plan and code reviews)

### Server

- `internal/domain/analytics.go`, dimension `origin`:
  - SQL: `(CASE WHEN broadcast_id IS NOT NULL THEN 'broadcast' WHEN automation_id IS NOT NULL THEN 'automation' WHEN transactional_notification_id IS NOT NULL THEN 'transactional' ELSE 'other' END)`;
  - Description: "What sent the message: broadcast, automation, transactional, or other (none of the three, such as template tests and double opt-in confirmations)";
  - the comment above it: the values and their order are those of the `messages.list` origin filter.
- `internal/repository/message_history_postgre.go`, `applyMessageFilters`: the origin filter follows the dimension's order. Some automation emails also carry a notification ID (the 28.0 upgrade backfilled one by template, excluding broadcasts only): `automation` excludes a broadcast ID, `transactional` excludes a broadcast or automation ID, so the four values partition the table as the dimension does.
- `internal/service/demo_service.go`: the demo workspace's transactional emails carried no notification ID, so they would become `other` on the demo dashboard. The demo now creates a `welcome` notification beside `password_reset`, and each seeded transactional email carries the ID of the notification that sends it (`demoNotificationIDs`).
- `internal/domain/message_filter.go`, `RequireBound` comment: names the six accepted fields (besides `created_after`), and says that without one of them the other filters are refused, even one an index could use, such as `created_before`. No code change.
- `CHANGELOG.md`, 44.0: the origin dimension's Feature line lists `other`, "the values of the `messages.list` filter"; the dashboard's Feature line lists the `other` series. No Fix line: neither shipped.

### Console (dashboard)

- `dashboardKpis.ts`: `MessageOrigin` adds `'other'`; `MESSAGE_ORIGINS` becomes `['broadcast', 'transactional', 'automation', 'other']`.
- `DashboardKpis.tsx`: the empty `series` fallback gets `other: emptyTotals()`.
- `OriginVolumeChart.tsx`: a fourth line `other`, named t`Other` (already translated in every catalogue), colour `colors.chart.series4`; broadcasts as bars, the other origins as lines.
- `theme/tokens.ts`: `chart.series4`, amber (`#f59e0b`); a gray would read as a legend item switched off. `chart` is not mirrored in the Tailwind `@theme` block.
- `useOriginSeries.ts`, `originChart.ts`: doc comments without a count of series.
- No `.po` change: `Other` exists in every catalogue, and `lingui extract` only moves references.

## Tests

| Implementation file | Test file | Cases |
|---|---|---|
| `internal/domain/analytics.go` | `internal/domain/analytics_test.go` `TestMessageHistoryOriginDimension` | the exact new SQL |
| `internal/domain/analytics.go` (query builder) | `internal/repository/analytics_postgres_test.go` (`originCaseSQL`) | the SQL in SELECT, GROUP BY and filters |
| `internal/repository/message_history_postgre.go` | `internal/repository/message_filter_postgres_test.go` | the exclusive `automation` and `transactional` conditions, alone and with other filters; `CountMessages` with the same conditions |
| `internal/service/demo_service.go` | `demo_service_test.go` `TestDemoService_TransactionalMessagesCarryTheirNotification`; `demo_sample_data_test.go` `TestDemoService_CreateSampleTransactionalNotifications`; `demo_sample_gap_test.go` | each seeded transactional email carries its notification's ID; both notifications are created with their templates; a creation failure never aborts the seed |
| `internal/domain/message_filter.go` | existing `TestMessageFilter_RequireBound` | unchanged (comment only) |
| full stack | `tests/integration/analytics_message_origin_test.go` `TestAnalyticsMessageOrigin` | the dimension splits into `{broadcast: 2, automation: 3, transactional: 3, other: 2}`, one automation email carrying a notification ID too; each origin filter, `other` included, counts its own messages; the daily Paris series of `transactional` counts the notification messages only |
| full stack | `tests/integration/logs_filters_test.go` `TestLogsFilters` | the automation email with a notification ID is listed under `automation`, not `transactional`; `messages.count` by `automation_id` counts both automation emails |
| `dashboardKpis.ts` | `dashboardKpis.test.ts` | `other` is summed into the totals (1000, not 980), the broadcast share and the rates |
| `useOriginSeries.ts` / `OriginVolumeChart.tsx` | `OriginVolumeChart.test.tsx` | one query per origin, `other` included; the series by name, type and values |
| `DashboardKpis.tsx` | `DashboardKpis.test.tsx` | the `totals` helper gives `other` (type only; the sums are pinned in `dashboardKpis.test.ts`) |
| `originChart.ts` | `originChart.test.ts` | the fixtures give `other`; its metric values; not empty when only `other` sent |

## Review corrections (applied, folded into the sections above)

- `internal/repository/analytics_postgres_test.go` pins the dimension's SQL (`originCaseSQL`): updated, and `make test-repo` added to the run.
- The daily Paris series filters `origin=transactional`: its expected day count drops to the three notification messages.
- Type check with `npx tsc -b`, as CI does (`tsc --noEmit -p .` reads a config with no files).
- `RequireBound` comment: names the six accepted fields, and says that without one of them (or `created_after`) the other filters are refused, even one an index could use, such as `created_before`.
- Precedence: some automation emails carry a notification ID too (the 28.0 backfill). The dimension calls them `automation`; the list filter now does the same, `automation` excluding a broadcast ID and `transactional` excluding a broadcast or automation ID (`applyMessageFilters`). Pinned by a message with both IDs in `TestAnalyticsMessageOrigin` and `TestLogsFilters`, and by the SQL in `TestApplyMessageFilters`.
- The 44.0 dashboard CHANGELOG line lists the `other` series; the Description says "such as".
- Colour: amber (`#f59e0b`) rather than `gray[400]`, which reads as a legend item switched off.
- No `.po` change: `Other` already exists in every catalogue, and the extract only moves references.
- Code review: the demo workspace seeded its transactional emails without a notification ID; it now creates a `welcome` notification and gives each email its notification's ID.

## Run

```
make test-domain test-repo test-service
INTEGRATION_TESTS=true go test -race -tags integration,licdev -count=1 -run 'TestAnalyticsMessageOrigin|TestLogsFilters' ./tests/integration/
cd console && npx vitest run src/components/analytics src/theme src/i18n && npx tsc -b && npx eslint .
```
