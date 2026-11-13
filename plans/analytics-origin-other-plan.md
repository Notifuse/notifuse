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

## Changes

### Server

- `internal/domain/analytics.go`, dimension `origin`:
  - SQL: `(CASE WHEN broadcast_id IS NOT NULL THEN 'broadcast' WHEN automation_id IS NOT NULL THEN 'automation' WHEN transactional_notification_id IS NOT NULL THEN 'transactional' ELSE 'other' END)`;
  - Description: "What sent the message: broadcast, automation, transactional, or other (none of the three: template tests, double opt-in confirmations)";
  - the comment above it: template tests and double opt-in confirmations carry no ID and count as `other`, the value the `messages.list` origin filter gives them.
- `internal/domain/message_filter.go`, `RequireBound` comment: "it needs created_after, or one of id, contact_email, broadcast_id, automation_id, transactional_notification_id and template_id, which an index narrows. created_before alone, list_id, external_id and the is_* filters are refused. A wide period or a common value can still read a large part of the table." No code change.
- `CHANGELOG.md`, 44.0 Feature line of the origin dimension: values become `broadcast`, `automation`, `transactional` or `other`, "as the `messages.list` filter". No Fix line: the dimension never shipped.

### Console (dashboard)

- `console/src/components/analytics/dashboardKpis.ts`: `MessageOrigin` adds `'other'`; `MESSAGE_ORIGINS` becomes `['broadcast', 'transactional', 'automation', 'other']`.
- `console/src/components/analytics/DashboardKpis.tsx`: the empty `series` fallback gets `other: emptyTotals()`.
- `console/src/components/analytics/OriginVolumeChart.tsx`: a fourth line `other`, named t`Other` (already translated in every catalogue), colour `colors.chart.series4`; the doc comment says broadcasts as bars, the three others as lines.
- `console/src/theme/tokens.ts`: `chart.series4`, a gray (`gray[400]`), so the minor series stays quiet next to the three main ones. `chart` is not mirrored in the Tailwind `@theme` block, so `tailwindTheme.contract.test.ts` is not concerned.
- `console/src/components/analytics/useOriginSeries.ts`: the doc comment says the totals are the sums of the series (no count).
- `originChart.ts`: its doc comment says "The daily series of one metric" (no count).
- `npm run lingui:extract`: no new message expected (`Other` exists); the catalogues only move references.

## Tests

| Implementation file | Test file | Cases |
|---|---|---|
| `internal/domain/analytics.go` | `internal/domain/analytics_test.go` `TestMessageHistoryOriginDimension` | the exact new SQL, parenthesised |
| `internal/domain/message_filter.go` | existing `TestMessageFilter_RequireBound` | unchanged (comment only) |
| `analytics.go` (full stack) | `tests/integration/analytics_message_origin_test.go` `TestAnalyticsMessageOrigin` | the dimension splits into `{broadcast: 2, automation: 2, transactional: 3, other: 2}`; each origin filter, `other` included, counts its own messages; the daily Paris series stays as is |
| `dashboardKpis.ts` | `dashboardKpis.test.ts` | the totals include `other` (sent share and rates over the four origins) |
| `useOriginSeries.ts` / `OriginVolumeChart.tsx` | `OriginVolumeChart.test.tsx` | one series per origin, `other` included (`['automation', 'broadcast', 'other', 'transactional']`); the legend shows Other |
| `DashboardKpis.tsx` | `DashboardKpis.test.tsx` | the `totals` helper gives `other`; a sent figure that includes `other` |
| `originChart.ts` | `originChart.test.ts` | the fixtures give the four origins; the log-axis and empty rules hold with `other` |

Integration test: the existing test's comment ("Transactional: three notification messages … plus a template test and a double opt-in confirmation") is split, so the two ID-less messages are labelled as `other`.

## Plan review corrections (applied)

- `internal/repository/analytics_postgres_test.go` pins the dimension's SQL (`originCaseSQL`): updated, and `make test-repo` added to the run.
- The daily Paris series filters `origin=transactional`: its expected day count drops to the three notification messages.
- Type check with `npx tsc -b`, as CI does (`tsc --noEmit -p .` reads a config with no files).
- `RequireBound` comment: names the six accepted fields, and says that without one of them (or `created_after`) the other filters are refused, even one an index could use, such as `created_before`.
- Precedence: some automation emails carry a notification ID too (the 28.0 backfill). The dimension calls them `automation`; the list filter now does the same, `automation` excluding a broadcast ID and `transactional` excluding a broadcast or automation ID (`applyMessageFilters`). Pinned by a message with both IDs in `TestAnalyticsMessageOrigin` and `TestLogsFilters`, and by the SQL in `TestApplyMessageFilters`.
- The 44.0 dashboard CHANGELOG line lists the `other` series; the Description says "such as".
- Colour: amber (`#f59e0b`) rather than `gray[400]`, which reads as a legend item switched off.
- No `.po` change: `Other` already exists in every catalogue, and the extract only moves references.

## Run

```
make test-domain test-repo
INTEGRATION_TESTS=true go test -race -tags integration,licdev -count=1 -run 'TestAnalyticsMessageOrigin|TestLogsFilters' ./tests/integration/
cd console && npx vitest run src/components/analytics src/theme src/i18n && npx tsc -b && npx eslint .
```
