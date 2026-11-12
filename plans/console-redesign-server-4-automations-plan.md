# Plan — console redesign, server part 4: automations, and the `stats.exited` recount (spec 6)

## Need

The redesigned Automations screens (table with journeys in progress, completed, exited and
failed and a 30-day open rate; an editor with a "With stats" switch showing results on every
step) need, before their console PR:

1. `automations.list` gains `with_journey_counts`;
2. a new route `automations.nodeStats`;
3. an `automation_id` dimension in the `message_history` analytics schema;
4. an `automation_id` filter on `messages.list`;
5. realistic outputs in the demo automation history, so the demo shows the stats;
6. the recount of `stats.exited` in the unreleased v44 migration (decided by Pierre on
   12 Nov 2026, spec 6 §5 bis).

Spec (reviewed): "Spec 6 · Automatisations", "Côté serveur" §1 to §4, §5 bis,
"Données de démonstration", and the server table of "Fichiers, tests et livraison".

Not here (separate "published server defects" thread, agreed with it on 12 Nov 2026): §5
(`ExitContactJourneysOnReply` incrementing `stats.exited`), §6 (`automations.get` 404, typed
`ErrNotFound` from `GetByID`), and their Fix lines. `automations.nodeStats` needs that 404:
see "Dependency".

No table, column or index; one data statement added to v44 (approved); no environment
variable; `VERSION` stays 44.0.

## Changes

### 1. `automations.list`: `with_journey_counts` (spec 6 §1)

- `ListAutomationsRequest` gains `WithJourneyCounts bool`, read by `FromURLParams`
  (`true`/`false`, other values 400).
- `AutomationRepository.CountJourneysByStatus(ctx, workspaceID, automationIDs []string) (map[string]JourneyCounts, error)`:

  ```sql
  SELECT automation_id, status, COUNT(*) FROM contact_automations
  WHERE automation_id = ANY($1) GROUP BY automation_id, status
  ```

- `JourneyCounts{Active, Completed, Exited, Failed, Total int}` (JSON snake_case); an unknown
  status counts in `Total` only.
- `AutomationService.CountJourneys(ctx, workspaceID, automationIDs) (map[string]JourneyCounts, error)`:
  authentication, `automations.read`. Empty ID list → empty map, no query.
- Handler (`automation_handler.go:127-140`): calls `List`, then `CountJourneys` with the
  returned IDs only when asked, and adds `journey_counts` (ID → counts) to the response, with
  zeros for an automation without journey. `List`'s signature and `Automation` do not change;
  `stats` is still returned as is.

### 2. `automations.nodeStats` (spec 6 §2)

`GET /api/automations.nodeStats?workspace_id=…&automation_id=…&since=…&until=…`.
`since`/`until` are RFC 3339 instants, both or neither (neither: all time); only one, an
unreadable value or `since` after `until` → 400.

- `domain.NodeStatsRequest` (+ `FromURLParams`, `Validate`), `NodeStats`,
  `NodeEmailStats`, `NodeStatsResponse` exactly as in the spec's JSON (`visits`, `completed`,
  `failed_attempts`, `skipped`, `skip_reasons`, `errors`, `failed_journeys`, `in_progress`,
  `waiting`, `retrying`, `branches`, and `email` with `executions`, `queued`, `not_sent`,
  `sent`, `delivered`, `opened`, `opens_tracked`, `clicked`, `clicks_tracked`, `failed`,
  `bounced`, `complained`, `unsubscribed`; `email` omitted when not readable).
- `AutomationService.GetNodeStats(ctx, workspaceID, automationID, since, until *time.Time)`:
  authentication, `automations.read`, load the automation (not found or deleted → not found
  error mapped to 404 by the handler), then the repository. Every node of the graph is in
  `nodes`, zero if never visited; a `node_id` from executions that is no longer in the graph
  is dropped. The email part is requested only with `message_history.read`
  (`userWorkspace.HasPermission`).
- `AutomationRepository.GetNodeStats(ctx, workspaceID, automationID, since, until, withEmail bool) (*NodeStatsRaw, error)`
  runs the spec's queries, each filtered on `automation_id`, the period bounding
  `entered_at` of executions:
  a. visits / completed / errors / failed_attempts / skipped by node, plus `skip_reasons` by
     `output->>'skip_reason'`;
  b. branch split (`filter` yes/no, `list_status_branch` `branch_taken`, `ab_test`
     `variant_id`, `branch` `path_taken`), NULL keys not returned;
  c. journeys in place (no period): `waiting` / `retrying` / `in_progress` from each active
     journey's last execution through `CROSS JOIN LATERAL … ORDER BY e.entered_at DESC LIMIT 1`;
     failed journeys by `current_node_id`, the period bounding the journey's `entered_at`;
  d. (only `withEmail`) email counters by node from `automation_node_executions` joined to
     `message_history` on `m.id = e.output->>'message_id'`, filters copied from the
     analytics measures, and `queued` from `email_queue` (served by `idx_email_queue_source`);
     `not_sent = executions − with_message − queued` computed in Go, never negative (floored
     at 0).
  The jsonb `?` operator is written `??` for Squirrel, or the queries are raw SQL with `$n`
  placeholders (as the repository already does for raw SQL).
- `AuditExcludedRoutes` gains `automations.nodeStats` (next to `automations.get`).
- Dead `domain.AutomationNodeStats` (`domain/automation.go:259-267`) is removed if nothing
  references it (checked by `go build ./...`); the console types go with the console PR.
- `openapi/paths/automations.yaml` (new) documents `automations.list` (with
  `with_journey_counts`) and `automations.nodeStats`, referenced from `openapi/openapi.yaml`;
  `openapi.json` regenerated.

### 3. `automation_id` analytics dimension (spec 6 §3)

`"automation_id": {Type: "string", Title: "Automation ID", SQL: "automation_id", Description: "Automation that sent the message"}`.

### 4. `messages.list` `automation_id` filter (spec 6 §4)

`MessageListParams.AutomationID` (`json:"automation_id,omitempty"`), read by `FromQuery`; the
repository adds `automation_id = $n` like `broadcast_id`. Not in `openapi/`.

### 5. Demo data

`internal/service/demo_automation_history.go` writes realistic `output` per node type
(`filter_passed`, `branch_taken`, `variant_id`, `path_taken`, `message_id`, `delay_until`) and
its automation messages carry `automation_id` and the `message_id` written in the output.

### 6. v44: recount of `stats.exited` (spec 6 §5 bis)

Appended to `V44Migration.UpdateWorkspace`, after the existing steps (timeline function and,
once part 1 is merged, the three indexes):

```sql
UPDATE automations a
SET stats = jsonb_set(COALESCE(a.stats, '{}'::jsonb), '{exited}',
      to_jsonb(GREATEST(COALESCE((a.stats->>'exited')::int, 0), c.n))),
    updated_at = NOW()
FROM (SELECT automation_id, COUNT(*) AS n FROM contact_automations WHERE status = 'exited' GROUP BY automation_id) c
WHERE c.automation_id = a.id
  AND a.deleted_at IS NULL
  AND (a.stats IS NULL OR jsonb_typeof(a.stats) = 'object')
  AND c.n > COALESCE((a.stats->>'exited')::int, 0);
```

Only `exited`, never downwards; idempotent. No column or trigger function touched: no
automation trigger regeneration. `stats->>'exited'` holding a non-integer string would make
the cast fail the migration: the statement guards it with
`CASE WHEN a.stats->>'exited' ~ '^[0-9]+$' THEN (a.stats->>'exited')::int ELSE 0 END` in both
places (checked: the executor only writes integers, so the guard only protects hand-edited
rows; such a row is then set to the journey count).

## Dependency

`automations.nodeStats` must answer 404 for an unknown automation. The "published defects"
thread makes `GetByID` return `domain.ErrNotFound`. This PR's service maps `*domain.ErrNotFound`
to the not-found result; its unit tests mock the repository returning that error. If that
PR is not merged when this one is ready, this branch rebases on it (or waits), so the
integration case "unknown automation → 404" passes against the real repository.

## Files and tests (TDD: tests first)

| File | Change | Test file | Cases |
| --- | --- | --- | --- |
| `internal/domain/automation.go` | `WithJourneyCounts`, `JourneyCounts`, `NodeStatsRequest` + `Validate`, `NodeStats*` types, repository and service methods, removal of `AutomationNodeStats` | `internal/domain/automation_test.go` | `with_journey_counts` true, false, absent, invalid; `since`/`until` both, neither, only one (error), unreadable (error), reversed (error); `automation_id` missing (error) |
| `internal/domain/message_history.go` | `AutomationID` | `internal/domain/message_history_test.go` | read from the query; absent |
| `internal/domain/analytics.go` | `automation_id` dimension | `internal/domain/analytics_test.go` | exists, type `string` |
| `internal/domain/audit_actions.go` | `automations.nodeStats` excluded | `internal/http/audit_routes_test.go` (existing) | passes |
| `internal/domain/mocks/*` | regenerated | — | — |
| `internal/repository/automation_postgres.go` | `CountJourneysByStatus`, `GetNodeStats` | `internal/repository/automation_postgres_test.go` | sqlmock: each query with and without period, args in place; email query and queue query absent when not asked; errors returned; unknown status in `Total` only |
| `internal/repository/message_history_postgre.go` | `automation_id` condition | `internal/repository/message_history_postgre_test.go` | present with the filter, absent without |
| `internal/service/automation_service.go` | `CountJourneys`, `GetNodeStats` | `internal/service/automation_service_test.go` | refused without `automations.read`; not found → not found error; without `message_history.read` the repository is called with `withEmail=false` and `email` is absent; each graph node present at zero; unknown `node_id` dropped; `not_sent` computed and floored at 0; empty ID list → no query |
| `internal/service/demo_automation_history.go` | realistic outputs | `internal/service/demo_automation_history_test.go` | each node type has the output keys `nodeStats` reads; each `message_id` exists among the generated messages with `automation_id` |
| `internal/http/automation_handler.go` | `automations.nodeStats`; `journey_counts` | `internal/http/automation_handler_test.go` | non-GET → 405; bad period → 400; permission → 403; not found → 404; success; list with and without `journey_counts` (`CountJourneys` called only when asked, with the returned IDs) |
| `internal/migrations/v44.go` | recount statement | `internal/migrations/v44_test.go` | the statement is run after the previous steps; its error returned with the workspace ID |
| `openapi/paths/automations.yaml` (new), `openapi/openapi.yaml`, schemas | routes | `make openapi-lint` | — |
| `tests/integration/automation_node_stats_test.go` (new) | — | same | `TestAutomationNodeStats`: real executor on trigger → email → delay → filter → two emails: visits per node; filter split; contacts waiting on the delay; delay as last step counted waiting; a journey that fails once then succeeds: `retrying` while waiting, not after; `failed_attempts` one per attempt; email counters joined to `message_history` equal `analytics.query` on those messages; an email in the queue counted `queued`; period excludes an execution back-dated 40 days; a member without `message_history.read` gets no `email`; unknown automation → 404 |
| `tests/integration/automation_journey_counts_test.go` (new) | — | same | `TestAutomationJourneyCounts`: active, completed, exited and failed journeys: exact counts; automation without journey has zeros |
| `tests/integration/v44_exited_recount_test.go` (new) | — | same | `TestV44ExitedRecount`: `stats.exited = 1` with three exited journeys → 3; `stats.exited = 5` with two → stays 5; `stats` JSON `null` left as is; deleted automation untouched; migration run twice gives the same result |
| `CHANGELOG.md` | Feature line; recount phrase | — | — |

CHANGELOG (short; the console PR extends the Feature with the screens):

- **Feature**: The API adds `automations.nodeStats`, journey counts to `automations.list`, an
  `automation_id` filter to `messages.list` and an `automation_id` dimension to the message
  history analytics.
- The recount is customer-visible through the other thread's Fix line; spec 6 §5 bis words it
  "Contacts who leave an automation by replying now count in its exited total, past exits
  included." This PR does not add a line of its own: it sends that wording to the other thread
  so its Fix line says "past exits included" once both are in (whichever merges second
  adjusts the line).

## Verification

```bash
make test-domain test-repo test-service test-http test-migrations
make openapi-lint && make openapi-bundle
INTEGRATION_TESTS=true go test -race -tags integration,licdev ./tests/integration -run 'TestAutomationNodeStats|TestAutomationJourneyCounts|TestV44ExitedRecount' -v
```

Then `make test-unit` once before review. `EXPLAIN (ANALYZE, BUFFERS)` on a generated
1-million-journey automation is done before the console PR (spec 6 "Coût").

## Plan review and corrections

Review: `reviews/console-redesign-server/plan-4-review.md` in the project files. Every finding
was checked in the code; the corrections below take precedence over the sections above.

- **B1**: `queued` is `next_retry_at IS NOT NULL AND attempts < max_attempts`, not
  `status = 'failed' AND attempts < max_attempts`: `MarkAsPermanentlyFailed` keeps
  `status 'failed'`, sets `next_retry_at = NULL` and leaves `attempts` low
  (`email_queue_postgres.go:429-446`), so a permanent 5xx would count as queued.
- **S1**: `queued` and `not_sent` come from one statement: query d LEFT JOINs `email_queue`
  on the execution, and counts per execution `m.id IS NULL AND q.id IS NOT NULL` (queued) and
  `m.id IS NULL AND q.id IS NULL` (not sent). Same period, same snapshot, no floor. The PR
  description says this departs from the spec's two-query formula and why.
- **S2**: the recount is exactly this statement, and `v44_test.go` expects this string:

  ```sql
  UPDATE automations a
  SET stats = jsonb_set(COALESCE(a.stats, '{}'::jsonb), '{exited}', to_jsonb(c.n)),
      updated_at = NOW()
  FROM (SELECT automation_id, COUNT(*) AS n FROM contact_automations
        WHERE status = 'exited' GROUP BY automation_id) c
  WHERE c.automation_id = a.id
    AND a.deleted_at IS NULL
    AND (a.stats IS NULL OR jsonb_typeof(a.stats) = 'object')
    AND c.n > CASE WHEN a.stats->>'exited' ~ '^[0-9]{1,18}$'
                   THEN (a.stats->>'exited')::bigint ELSE 0 END;
  ```

  `TestV44ExitedRecount` adds: a non-integer `exited` (`"abc"`, `7.0`, `true`) is set to the journey count
  without an error; `99999999999` (beyond `int`) is read as `bigint` and stays; other keys such
  as `enrolled` are untouched.
- **S3**: the demo gains automation messages: written after the journey transaction commits,
  in small committed batches (as `demo_automation_history.go:285-289`), with the NOT NULL
  columns filled. Each output is derived from the node config and the next node of the path
  (`wb-filter` passed vs `cr-filter` rejected, `branch_taken` from the edge, `variant_id` whose
  `NextNodeID` is the next node; exit-path email carries `skipped`/`skip_reason`). The demo test
  checks each split matches the edge taken.
- **S4**: integration recipes: fail-then-succeed is a webhook node to an `httptest` server
  answering 500 then 200 (`AllowPrivateDeliveryHosts = true`), the 2-minute retry pulled
  forward with `UpdateContactAutomationScheduledAt`; the queued case uses a hand-inserted
  `email_queue` row; the spec's "a cancelled send counts in `not_sent`" case is added.
- **S5**: `TestAutomationNodeStats` asserts `messages.list?automation_id=` returns only that
  automation's messages.
- **S6**: this PR merges after thread B's Fix (dependency), so it edits that Fix line itself
  to add "past exits included".
- **S7**: when the member may read message history, every email node has an `email` object
  (zeros if no message); `node_type` comes from the automation's graph, never from the
  execution row; `nodes`, `journey_counts` and nested maps are `{}`, never `null`. One test each.
- N1: v44's workspace steps are the ordered `v44WorkspaceStatements()` list introduced by
  server part 1; the recount is appended last, the doc comment extended. N2: the demo failure
  rows get the shape the query counts. N3 (uncertain): clock skew between app and database is
  checked when query c is written; noted if not handled. N4: `automations.list` is documented with a full
  `Automation` schema. N5: `pq.Array`, `regexp.QuoteMeta`, `errors.As`, return types stated in
  code. N6: databases already stamped 44 (dev, demo) do not get the recount: noted in the PR.
