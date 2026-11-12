# Plan — console redesign, server part 1: dashboard and transactional (specs 3 and 7)

## Need

The redesigned dashboard (spec 3, screen 131) and transactional screens (spec 7, screens 91
and 92) need four small server additions, delivered before their console PRs so the screen
threads can build on them:

1. an `origin` dimension in the `message_history` analytics schema (spec 3, Backend);
2. three partial indexes on `message_history` for the "Recent failures" card (spec 3,
   "Index partiels des échecs", approved by Pierre on 12 Nov 2026 as an addition to the
   unreleased v44 migration);
3. a `transactional_notification_id` filter on `messages.list` (spec 7, server §1);
4. a `last_sent_at` measure in the `message_history` analytics schema (spec 7, server §2).

Specs (reviewed): "Spec 3 · Tableau de bord" and "Spec 7 · Transactionnel". This PR holds no console change: the
console parts ship with the screen PRs.

This is part 1 of six server PRs for the console redesign (see "Sequence" at the end).

## Database

No table or column. Three indexes, added to the unreleased v44 (main is 43.0, `VERSION` is
44.0 on dev), approved by Pierre:

```sql
CREATE INDEX IF NOT EXISTS idx_message_history_failed_recent ON message_history (created_at DESC, id DESC) WHERE failed_at IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_message_history_bounced_recent ON message_history (created_at DESC, id DESC) WHERE bounced_at IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_message_history_complained_recent ON message_history (created_at DESC, id DESC) WHERE complained_at IS NOT NULL;
```

They follow the order of `messages.list` (`created_at DESC, id DESC`): `messages.list?is_complained=true&limit=5`
reads the first entries of the partial index and stops, instead of walking
`idx_message_history_created_at_id` over the whole table when no row matches (measured in
the spec: 108 ms on 3 M rows, ~1 s at 30 M).

- Plain `CREATE INDEX` (not `CONCURRENTLY`): migrations run inside a transaction.
- No column is renamed or dropped and no trigger function changes: no automation trigger
  regeneration.
- `v44.go` keeps its existing step first (timeline function), then runs the three
  statements, each wrapped with the workspace ID in its error.
- New workspaces get them from `internal/database/init.go`.
- No `VERSION` change; v44 is completed rather than adding v45 (the project rule only forbids
  amending a *shipped* migration).

## Changes

### 1. `origin` dimension (spec 3)

`internal/domain/analytics.go`, `message_history` dimensions:

```go
"origin": {
    Type:        "string",
    Title:       "Origin",
    SQL:         "(CASE WHEN broadcast_id IS NOT NULL THEN 'broadcast' WHEN automation_id IS NOT NULL THEN 'automation' ELSE 'transactional' END)",
    Description: "What sent the message: broadcast, automation or transactional",
},
```

The parentheses are required by the query builder for any dimension that is not a bare
column (it copies the SQL into SELECT, GROUP BY and filters). Test sends of a broadcast carry
its `broadcast_id` and count as `broadcast`; template tests and double opt-in confirmations
carry neither ID and count as `transactional` (spec 3, table "Ce que chaque valeur contient").
No change in `pkg/analytics`.

### 2. `transactional_notification_id` on `messages.list` (spec 7 §1)

- `MessageListParams` gains `TransactionalNotificationID string`
  (`json:"transactional_notification_id,omitempty"`), read by `FromQuery` next to
  `broadcast_id`, not validated (like `broadcast_id` and `template_id`).
- `MessageHistoryRepository.ListMessages` adds `sq.Eq{"transactional_notification_id": …}`
  when non-empty, next to `broadcast_id`. Order stays `created_at DESC, id DESC`.
- Permission unchanged: `message_history.read`.
- `messages.list` is not in `openapi/`: nothing to add there.

### 3. `last_sent_at` measure (spec 7 §2)

```go
"last_sent_at": {
    Type:        "max",
    Title:       "Last Sent At",
    SQL:         "sent_at",
    Description: "When the most recent message accepted by the email provider was sent",
    Filters: []analytics.MeasureFilter{
        // sent_at is stamped on every attempt, failed or not (see count_sent).
        {SQL: "failed_at IS NULL"},
    },
},
```

`buildMeasureSQL` renders `MAX(sent_at) FILTER (WHERE failed_at IS NULL)`. `ScanRows` turns a
`time.Time` into RFC 3339 UTC; a group with no matching message gives `null`.

### 4. Partial indexes (spec 3)

See Database.

## Files and tests (TDD: tests first)

| File | Change | Test file | Cases |
| --- | --- | --- | --- |
| `internal/domain/analytics.go` | `origin` dimension; `last_sent_at` measure | `internal/domain/analytics_test.go` | `origin` exists in `message_history`, type `string`, SQL is the parenthesised `CASE`; `last_sent_at` exists, type `max`, SQL `sent_at`, exactly one filter `failed_at IS NULL` |
| `internal/repository/analytics_postgres.go` (unchanged) | — | `internal/repository/analytics_postgres_test.go` (sqlmock) | filter `origin equals automation` puts the `CASE` in the `WHERE`; dimension `origin` puts it in `SELECT` and `GROUP BY`; `last_sent_at` with a `transactional_notification_id` dimension and an `in` filter renders `MAX(sent_at) FILTER (WHERE failed_at IS NULL)` |
| `internal/domain/message_history.go` | `TransactionalNotificationID` in `MessageListParams`, read by `FromQuery` | `internal/domain/message_history_test.go` | read from the URL; absent leaves it empty |
| `internal/repository/message_history_postgre.go` | `transactional_notification_id = $n` condition | `internal/repository/message_history_postgre_test.go` (sqlmock) | condition present with the parameter, absent without; combined with `is_failed` (both conditions and args in order) |
| `internal/http/message_history_handler.go` | nothing (passes `MessageListParams` through) | `internal/http/message_history_handler_test.go` | `transactional_notification_id` reaches the service (`gomock.Eq` on the full params) |
| `internal/database/init.go` | three partial indexes | `internal/database/init_test.go` (or the existing schema test that lists init statements) | the three statements are in the workspace DDL |
| `internal/migrations/v44.go` | three `CREATE INDEX IF NOT EXISTS` after the timeline step; doc comment updated | `internal/migrations/v44_test.go` | existing test renamed and extended: the timeline function, then the three statements, in that order and nothing else (`ExpectationsWereMet`); an error on each statement is returned with the workspace ID and stops the migration |
| `tests/integration/analytics_message_origin_test.go` (new) | full stack | same | `TestAnalyticsMessageOrigin`: a broadcast message, its test send (broadcast ID), two automation messages, three transactional messages, a template test and a double opt-in confirmation (no IDs), two origins on the same day: each `origin` filter returns the right count; daily series in `Europe/Paris` with a message at 00:30 local time (the evening before in UTC) lands on its local day; the last day of `dateRange` is included; `inDateRange` on ISO instants; a member without `message_history` read is refused |
| `tests/integration/message_history_handler_test.go` | new cases | same | `messages.list?transactional_notification_id=` returns only that notification's messages; `analytics.query` with `last_sent_at` by `transactional_notification_id` returns the last successful send per notification; a notification whose last message failed returns the success before it; one whose messages all failed returns `null`; one without messages in the window is absent |
| `tests/integration/v44_message_history_indexes_migration_test.go` (new) | migration | same | `TestV44MessageHistoryIndexesMigration`: drop the three indexes, run v44 twice (no error, indexes present); with `enable_seqscan = off` and after `ANALYZE`, `EXPLAIN` of the complaints query (`WHERE complained_at IS NOT NULL ORDER BY created_at DESC, id DESC LIMIT 5`) uses `idx_message_history_complained_recent` |
| `CHANGELOG.md` | under `[44.0] - unreleased`, one Feature line (below) | — | — |

CHANGELOG (customer-facing, short; the screen PRs extend the dashboard and transactional
lines with the screens):

- **Feature**: The analytics API gains an `origin` dimension (broadcast, automation or
  transactional) and a `last_sent_at` measure on `message_history`, and `messages.list`
  filters by `transactional_notification_id`.

No line for the indexes (internal, spec 3).

## Out of scope

- Console: everything (screen PRs).
- Fixes of released defects: handled in the separate "published server defects" thread.
- `EXPLAIN ANALYZE` on the largest demo workspace (spec 3 "Vérification", spec 7 "Coût"):
  done in the console PRs, which issue the queries.

## Sequence of the server PRs

1. This PR: specs 3 and 7.
2. Spec 4: contacts, lists, timeline, contact web visits.
3. Spec 5: broadcasts.
4. Spec 6: automations, and the `stats.exited` recount in v44.
5. Spec 8: logs (`messages.list` origin and period, `messages.count`, webhook source).
6. Specs 9 and 10: members' name, `templates.usage`, `blogPosts.counts`, `blogPosts.list`.

Each one starts from `dev` and goes to `dev`; a later one rebases when an earlier one merges
(`messages.list` filters are touched by 1, 4 and 5).

## Verification

```bash
make test-domain
make test-repo
make test-http
make test-migrations
INTEGRATION_TESTS=true go test -race -tags integration,licdev -run 'TestAnalyticsMessageOrigin|TestMessageHistory|TestV44MessageHistoryIndexesMigration' ./tests/integration/ -v
```

Then `make test-unit` once before review.

## Plan review and corrections

Review: `reviews/console-redesign-server/plan-1-review.md` in the project files. Every finding
was checked in the code; the corrections below take precedence over the sections above.

- **S1**: after the timeline step, v44 runs `SET LOCAL statement_timeout = 0` (an index build
  can outlast a configured timeout, and a failed migration fails again at every start, as
  `v42.go:89-93` explains) and `SET LOCAL lock_timeout = '5s'` (as v39 and v43: a busy table
  fails the migration, retried on the next start, instead of queuing writes). The workspace
  steps become one ordered list `v44WorkspaceStatements()`, asserted exactly by `v44_test.go`.
  The doc comment says the builds hold a `SHARE` lock on `message_history` (writes wait) until
  commit, a few seconds on a large table; the old "takes no table lock" sentence is rewritten.
- **S2**: the EXPLAIN test runs in one transaction with `SET LOCAL enable_seqscan = off`, on
  the exact query `ListMessages` builds (its column list, `ORDER BY created_at DESC, id DESC`,
  `LIMIT 6`), for each of the three flags.
- **S3**: verification adds `make test-database` and `TestTimelineLinkClicked` (already runs
  v44 on a real database).
- **S4**: the `last_sent_at` comment says a retry upsert does not move `sent_at`
  (`message_history_postgre.go:275-281`), and that for broadcast and automation messages
  `sent_at` is the time the email was queued (`worker.go`, `SentAt: entry.CreatedAt`), so
  `last_sent_at` there is the last queuing time (corrected after the code review).
- N1: `init_test.go` uses `recordWorkspaceSchema(...).statementContaining`. N2: inline
  `MessageHistoryOption` closures for `created_at`, `transactional_notification_id`,
  `complained_at`; `last_sent_at` compared at second precision. N3: not taken (literal SQL
  in both places, asserted by tests). N4: two CHANGELOG bullets, one per spec. N5: test named
  `v44_message_history_indexes_migration_test.go` / `TestV44MessageHistoryIndexesMigration`.
  N6: the docs PR of spec 7 ships with the console PR. N7: "exactly one filter" pinned with
  `require.Len(…, 1)`. N8: databases already stamped 44 (dev, demo) do not get the indexes:
  noted for the console PRs' EXPLAIN. N9: released behaviours noted, unchanged.

## Code review and corrections

Review: `reviews/console-redesign-server/pr-1-code-review.md` in the project files.

- B1 (false comment on `sent_at` for broadcast and automation messages): reworded.
- Taken: the v44 doc comment gives the measured lock time (about 1 s for 2 M rows in memory),
  says a rerun takes the lock too, and states the `lock_timeout` rule without contradiction;
  the CHANGELOG has an Improvement line on the indexes and the write wait during the upgrade;
  the origin filter test looks at the query's own `WHERE`; the unpinned "two IDs" comment is
  reworded.
- Known, not handled (for the console PRs): `origin` as a dimension with a daily series loses
  rows in the gap filler (one query per origin instead); ordering by `last_sent_at` fails in
  the query builder (existing limit, as `count_sent`); `last_sent_at` is `0` in a gap-filled
  bucket; the `origin` filter cannot use an index (bounded by the date range);
  `count_transactional_messages` counts automation messages (existing measure);
  `transaction_timeout` is not lifted (no other migration does); the EXPLAIN test copies the
  query by hand; the origin integration test builds fixture rows.

## Final verification

```bash
make test-domain test-repo test-http test-migrations test-database
INTEGRATION_TESTS=true go test -race -tags integration,licdev -run 'TestAnalyticsMessageOrigin|TestMessageHistory|TestV44MessageHistoryIndexesMigration|TestTimelineLinkClicked' ./tests/integration/ -v
make test-unit
```
