# Broadcast audience estimate (#440) + duplicate-send fix

Issue: https://github.com/Notifuse/notifuse/issues/440 — "`Segment` did not respect `List`". In the broadcast drawer, the segment picker shows each segment's whole size (`interested (3)`) even when none of those contacts are on the selected list. The send path already intersects list and segments; the picker does not.

Investigating it surfaced a real, released defect next to it, fixed first in Phase 1.

## Decisions (agreed 2026-10-15)

- **Picker label**: once a list is selected, each segment shows `in-list of segment-total`, e.g. `interested (0 of 3)`. Before a list is picked, it shows the segment total as today: `interested (3)`.
- **Estimated total** is shown in two places: under the audience fields in the drawer, and in the Send or Schedule dialog.
- **Console-only endpoint, for now**: `broadcasts.estimateAudience` is not added to `openapi/` or the API reference; its contract is documented in a code comment on the handler. It can be published later.
- **Permissions**: `broadcasts:read` and `lists:read` are always required, and `segments:read` too whenever the request counts segment membership (it names segments, or asks for per-segment counts). These are the gates that already guard the same numbers:
  - list member counts need `lists:read` (`ListService.GetListStats`, `list_service.go:192-200`);
  - segment sizes need `segments:read` (`segments.list?with_count=true`, through `SegmentService.authorize`).

  `contacts:read` is not needed. The segment service reserves it for evaluating arbitrary trees and listing a segment's contacts (`PreviewSegment`, `GetSegmentContacts`, `GetSegmentContactDetails`; rationale at `segment_service.go:79-86`), and the estimate only counts saved memberships. No console user loses anything: without `lists:read` there is no list to pick, and without `segments:read` no segment.
- **Two phases, two commits**. Phase 1 is a correctness fix that stands alone and can ship without Phase 2.

## Phase 1 — a contact in several selected segments is sent the broadcast once per segment

### Root cause (verified)

The list part is safe: `contact_lists` has primary key `(email, list_id)`, so filtering on one list yields at most one row per contact. The segment filter is a plain join added on top (`contact_postgres.go:1486-1504` in `GetContactsForBroadcast`, `:1758-1773` in `CountContactsForBroadcast`). The SQL the repository actually generates for `List: newsletter, Segments: [vip, fr]` (printed from the real code via sqlmock):

```sql
SELECT COUNT(*) FROM contacts c
JOIN contact_lists cl ON c.email = cl.email
JOIN lists l ON cl.list_id = l.id
JOIN contact_segments cs ON c.email = cs.email
WHERE cl.list_id = $1 AND l.deleted_at IS NULL AND cl.status <> $2 AND cl.status <> $3 AND cl.status <> $4
  AND (l.is_double_optin = $5 OR cl.status <> $6) AND cs.segment_id IN ($7,$8)
```

`contact_segments` holds one row per (contact, segment), so a contact in both `vip` and `fr` comes back twice. Reproduced in a scratch database: one list, alice in both segments, bob in one → rows `alice, alice, bob`, count 3.

Nothing downstream removes the duplicate. `queue_message_sender.go:145` gives every row a fresh UUID message id, `email_queue` has no unique constraint on (source, email), and the queue worker does not check whether that contact was already sent this broadcast. Consequences:

- The contact receives the email once per matching selected segment. Its rows are adjacent (`ORDER BY c.email`), so all of them are sent unless they straddle a fetch-page boundary, where the cursor skips the rest.
- `total_recipients` is inflated, and so is the A/B split computed from it (`orchestrator.go:829`).
- In an A/B test, each duplicate row draws a variant at random (`queue_message_sender.go:452`), so a contact can receive both variants and is counted twice in the test results.

It only happens when **two or more segments are selected and a contact belongs to more than one of them**. One segment, or none, is unaffected. It has been live since segments became a broadcast audience filter (commit `299f0d58`, first tagged v8.0).

Why it shipped: the only test of these two queries with more than one segment (`contact_postgres_test.go:1815`, in "should count contacts for broadcast with segments filtering") is sqlmock and pins the SQL text, so it asserts the join rather than the result. No integration test sends a broadcast with segments at all. The user docs promise the opposite: "No duplicate handling needed since each contact appears once per list" (`../docs/features/broadcast-campaigns.mdx`, "Select a List"). That holds for the list alone; the segment filter broke it.

### Fix

`internal/repository/contact_postgres.go`:

1. Extract `withBroadcastListFilter(q sq.SelectBuilder, audience domain.AudienceSettings) sq.SelectBuilder`. It adds the `contact_lists cl` / `lists l` joins and every list predicate currently duplicated in both methods: list id, `l.deleted_at IS NULL`, the unsubscribed/bounced/complained exclusions when `ExcludeUnsubscribed`, and the double opt-in guard. It adds joins and conditions only; each caller keeps its own select list. Phase 2 reuses it, so the estimate and the send cannot drift apart.
2. Extract `broadcastSegmentsFilter(segments []string) sq.Sqlizer` returning `EXISTS (SELECT 1 FROM contact_segments cs WHERE cs.email = c.email AND cs.segment_id IN (?,?,…))`. Use the placeholder technique of the proven pattern at `contact_postgres.go:313-332`: an `sq.Expr` with literal `?` placeholders, renumbered by the outer `Dollar` builder. Do not nest a `psql` (Dollar) sub-builder, which would number its own placeholders from `$1`. Copy only the technique, not that subquery's `JOIN segments s`: the send has never filtered on segment rows, and this fix should change the duplicates only.
3. Rewrite both methods as: `FROM contacts c` → `withBroadcastListFilter` when `audience.List != ""` → `Where(broadcastSegmentsFilter(...))` when segments are given. The two segments-only `else` branches that rebuilt the whole query (`:1494-1504`, `:1766-1772`) collapse into this shape. The no-list path stays, now also free of duplicates. Only a broadcast stored before lists became required can reach it (`Broadcast.Validate`, `broadcast.go:400`). `ORDER BY c.email` and the keyset cursor `c.email > afterEmail` are unchanged, and `includeListID` still follows `audience.List != ""`. The cursor condition used to sit between the list conditions; after the extraction it follows them. That changes the argument order when a cursor is set, and no existing unit test passes a cursor, so a new case pins it (see Tests).

### Rollout notes

- No migration, no `VERSION` bump (41.1 is WIP).
- A broadcast mid-send at deploy finishes normally through the empty-fetch exit (`orchestrator.go:1103`). Its stored `total_recipients` stays inflated, so its progress stops short of 100% and its A/B split was computed from the larger total. This is a one-off with nothing to backfill.

### Tests — Phase 1

| Implementation | Test file | Cases |
|---|---|---|
| `internal/repository/contact_postgres.go` | `internal/repository/contact_postgres_test.go` | Update `TestGetContactsForBroadcast` › "should get contacts for broadcast with segments filtering" (`:1540`) and `TestCountContactsForBroadcast` › "should count contacts for broadcast with segments filtering" (`:1799`) to expect `WHERE EXISTS (SELECT 1 FROM contact_segments cs WHERE cs.email = c.email AND cs.segment_id IN (…))` and no `JOIN contact_segments`. New case for each method: list + two segments, asserting the full SQL and the argument order (list, three statuses, `false`, `pending`, then the segment ids). New `GetContactsForBroadcast` case with a cursor + list + segments, pinning where `c.email > $n` lands in the arguments. Existing list-only and double-opt-in cases must pass unchanged, which proves the helper extraction preserved the list predicate (none of them passes a cursor). |
| (same) | `tests/integration/broadcast_segment_audience_test.go` (new, `TestBroadcastSegmentAudience_Integration`, modeled on `broadcast_double_optin_test.go`) | Fixture: one list; segments A and B seeded with direct `INSERT INTO segments` / `contact_segments` at version 1 (pattern at `contact_deletion_erasure_test.go:106-114`). Seeded memberships are stable because the integration server runs with the task scheduler disabled (`tests/testutil/server.go:116-118`), so the segment queue processor never re-evaluates them. alice on the list and in A and B; bob on the list and in A only; carol on the list with no segment; dave in A and B but not on the list; erin on the list, `unsubscribed`, in A. Cases: (1) `CountContactsForBroadcast` with `[A, B]` and `ExcludeUnsubscribed` = 2 (fails today with 3). (2) `GetContactsForBroadcast` with limit 100 returns exactly `[alice, bob]`, each once; this is the regression case (fails today with `alice, alice, bob`). (3) Keyset paging with limit 1: alice, then bob, then empty. This guards paging only: today's code passes it too, because the cursor `c.email > 'alice'` skips the second alice row at a page boundary. (4) Control `[A]` alone = alice, bob. (5) `ExcludeUnsubscribed: false` with `[A]` adds erin. (6) End to end, in `TestBroadcastSegmentAudienceSend_Integration`: send a broadcast to the list with `[A, B]` through steps 2 and 8-13 of `runBroadcastPaginationTest` (`broadcast_pagination_test.go`: SMTP provider, template, `factory.CreateBroadcast` with `WithBroadcastAudience`, `StartBackgroundWorkers`, `ScheduleBroadcast`, `WaitForBroadcastStatusWithExecution`). Then assert the broadcast's `enqueued_count` is 2 (3 today), which is final once it completes (`orchestrator.go:1451`). Don't assert through Mailpit: `GetAllMailpitRecipients` returns a set of addresses and cannot see a duplicate. The seeded memberships survive `ExecutePendingTasks`, because factory workspaces are created through the repository and only `WorkspaceService.CreateWorkspace` (`workspace_service.go:374`) creates the `process_contact_segment_queue` task. |

### Changelog — Phase 1

Add under `## [41.1] - WIP`, kept to 2-3 sentences:

> **Fix**: A broadcast filtered by several segments no longer sends twice to a contact who belongs to more than one of them. Each extra segment the contact matched produced another copy of the email, and inflated the recipient count and A/B test results.

### Implemented (2026-10-18)

Phase 1 is implemented as planned, on top of the keyset bounds that "Broadcasts to large lists are faster to send" added (`cl.email > ?` next to `c.email > ?`). Deviations and notes:

- **The cursor bounds `contact_segments` too.** `broadcastSegmentsFilter(segments, afterEmail)` adds `AND cs.email > ?` inside the EXISTS when a page has a cursor, and `CountContactsForBroadcast` passes none. Without it, on PostgreSQL 17 with 1M contacts and two overlapping segments of 500k and 333k members, a page of 50 past the middle of the list took 60-130 ms against under 1 ms with it, the same as the old join, and so did PostgreSQL 18. The count is about as fast as the old join's on both.
- **The cursor follows the list conditions**, as step 3 says, so its arguments come after the statuses. A unit case pins the full argument order for a later page of a list and two segments; others pin a later page with segments and no list, and with neither.
- **The changelog bullet** is under `## [42.0] - WIP`, which replaced 41.1, and is reworded: a contact could get a copy per segment, not only two.
- **Tests.** `TestBroadcastSegmentAudienceSend_Integration` creates the broadcast through `broadcasts.create`, as `TestBroadcastAudienceStatusesAsDelivered` does, rather than the factory and an update. Besides `enqueued_count`, it asserts the send task's `total_recipients` and that `message_history` holds one message per recipient, which sees a duplicate that Mailpit's recipient set cannot. `TestBroadcastSegmentAudience_Integration` also covers segments without a list, in one page and in pages of one. `TestBroadcastAudiencePagination_Integration` pages through a list narrowed to two segments that share contacts, in pages of 2 and 50, and checks that each paged audience matches its count.

## Phase 2 — `broadcasts.estimateAudience` and the picker counts

### API

`POST /api/broadcasts.estimateAudience`. This is a POST because the body carries the audience object with its segment array, with the same precedent as `broadcasts.testRecipientFeed`. It is read-only, so it is not wrapped in `restrictedInDemo`.

```json
// request
{ "workspace_id": "ws1",
  "audience": { "list": "newsletter", "segments": ["vip", "fr"], "exclude_unsubscribed": true },
  "include_segment_counts": true }
// response
{ "total": 2,
  "segment_counts": { "vip": { "in_list": 2, "total": 4 }, "fr": { "in_list": 1, "total": 2 } } }
```

- `total`: the recipient count a send would start from now (its `total_recipients`). It is `CountContactsForBroadcast`, the same predicate the send fetches with. Individual sends can still fail later.
- `segment_counts` is returned only when `include_segment_counts` is set. It carries **both numbers of the "x of y" label**: `in_list`, the segment's members who are on `audience.list` under the same list rules, and `total`, the segment's whole size. It covers every live segment that has at least one member; a segment with none is absent, meaning `0 of 0`. It ignores `audience.segments`. The drawer asks for it; the Send dialog does not, which spares it a GROUP BY it would throw away.
- `omitempty` also drops an *empty* map, so when no segment has members the field is missing even though counts were requested. The console must read `segment_counts ?? {}` once the response has arrived, never treat a missing field as "not loaded".
- Both numbers come from **one SQL statement**, so they share one snapshot, and they use the picker's membership rule `s.version = cs.version` (the rule `users_count` uses at `segment_postgres.go:149-160`). That guarantees x ≤ y. Taking y from the cached `segments.list` response instead would not: it can be minutes older than x, so a growing segment could show `4 of 3`, and a rebuild in progress could show `3 of 1`. `total` keeps the send's rule, because its job is to predict the send.

### Backend files

1. `internal/domain/broadcast.go`
   - `EstimateAudienceRequest { WorkspaceID; Audience AudienceSettings; IncludeSegmentCounts bool }` with `Validate()`: `workspace_id` required; `audience.list` required, with the same message as `Broadcast.Validate` ("list is required"); no empty segment id.
   - `SegmentAudienceCount { InList int \`json:"in_list"\`; Total int \`json:"total"\` }` and `EstimateAudienceResponse { Total int; SegmentCounts map[string]SegmentAudienceCount \`json:"segment_counts,omitempty"\` }`.
   - Add `EstimateAudience(ctx, *EstimateAudienceRequest) (*EstimateAudienceResponse, error)` to `BroadcastService` (`:1177`).
2. `internal/domain/contact.go`: add `CountBroadcastAudienceBySegment(ctx, workspaceID string, audience AudienceSettings) (map[string]SegmentAudienceCount, error)` to `ContactRepository`, next to `CountContactsForBroadcast` (`:670`).
3. `internal/repository/contact_postgres.go`: implement it as one statement. Build the in-list subquery with `sq.Select("c.email").From("contacts c")` (default `?` format, not `psql`) passed through `withBroadcastListFilter`. Attach it with `JoinClause(sq.Expr("LEFT JOIN (?) il ON il.email = cs.email", sub))` on a `Dollar` outer query; `sq.Expr` expands a nested builder by calling its `ToSql()` (squirrel v1.5.4 `expr.go:62-64`), which is why the sub must stay in `?` format. Do **not** write `LeftJoin("(?) il ON …", sub)`: a string join keeps its arguments as plain bind values (`part.go:23-25`), so the builder itself would be sent to Postgres as a parameter and fail at runtime.
   ```sql
   SELECT cs.segment_id, COUNT(*) AS total, COUNT(il.email) AS in_list
   FROM contact_segments cs
   JOIN segments s ON s.id = cs.segment_id AND s.version = cs.version
   LEFT JOIN (<in-list subquery>) il ON il.email = cs.email
   WHERE s.status <> 'deleted'
   GROUP BY cs.segment_id
   ```
   The list rules still come from the one helper. `il` has at most one row per email (`contact_lists` PK is `(email, list_id)` and the list is fixed), and `contact_segments`' PK is `(email, segment_id)`, so neither join can double-count. Return a non-nil map.
4. `internal/service/broadcast_service.go`: `EstimateAudience` authenticates, requires `broadcasts:read` and `lists:read`, and requires `segments:read` when `len(audience.Segments) > 0 || IncludeSegmentCounts` (`domain.NewPermissionError`). It calls `contactRepo.CountContactsForBroadcast`, plus `CountBroadcastAudienceBySegment` only when `IncludeSegmentCounts`.
5. `internal/http/broadcast_handler.go`: register the route next to the others (`:49-66`). `HandleEstimateAudience` follows `HandleTestRecipientFeed`: 405 on non-POST, 400 on bad JSON or `Validate`, 403 via `writePermissionError`, 500 "Failed to estimate audience". Its doc comment carries the contract and says it is deliberately left out of the public API reference.
6. Regenerate only the two affected mocks by running the `mockgen` lines at `broadcast.go:15` (`BroadcastService`) and `contact.go:25` (`ContactRepository`). Don't run `go generate ./internal/domain`, which rewrites every mock in the package. Both interfaces are implemented only by their mockgen mocks and the real types (no hand-written fakes), and the orchestrator holds a `domain.ContactRepository`, so nothing else needs touching.

### Console files

1. `console/src/services/api/broadcast.ts`: `EstimateAudienceRequest` / `EstimateAudienceResponse` interfaces and `broadcastApi.estimateAudience` (`api.post`).
2. `console/src/components/broadcasts/useAudienceEstimate.ts` (new): a TanStack `useQuery` keyed on `['broadcast-audience-estimate', workspaceId, list, sorted segments, exclude_unsubscribed, include_segment_counts]`. It takes an `enabled` flag from the caller (drawer open, modal visible) and also requires a list. It uses `placeholderData: keepPreviousData` so counts don't flicker between selections, and a short `staleTime`. While `isPlaceholderData` is true the numbers belong to the previous selection, possibly the previous list, so callers render them dimmed.
3. `console/src/components/broadcasts/UpsertBroadcastDrawer.tsx` (`:532-590`):
   - `Form.useWatch` the three audience fields. `exclude_unsubscribed` is `undefined` before mount, so treat it as `true`, its `initialValue`. Pass `enabled: isOpen`: the drawer is mounted three times on the Broadcasts page (`BroadcastsPage.tsx:611`, `:1615`, `:1754`) and keeps its form after closing.
   - One `segmentCountLabel` helper replaces the markup duplicated in `optionRender` and `tagRender`. With a list selected and an estimate response present, it shows `t\`${inList} of ${total}\``, with **both numbers taken from `segment_counts ?? {}`** and formatted with `toLocaleString`; a segment absent from the map shows `0 of 0`. Otherwise it shows `(users_count)` as today. Never mix `users_count` into the "of" label; see the snapshot note under API.
   - Ask for `include_segment_counts` only when the `segments` prop is non-empty. A member without `segments:read` has no segments to pick, because `segments.list` already refuses them, and asking would turn the whole estimate into a 403.
   - Under the `exclude_unsubscribed` switch (it changes the total): `t\`Estimated recipients: ${n}\``, dimmed while refetching and hidden on error.
4. `console/src/components/broadcasts/SendOrScheduleModal.tsx`: `useAudienceEstimate(workspaceId, broadcast?.audience, { enabled: visible })` without segment counts. `broadcast` is `null` until a broadcast is picked, and the modal stays mounted (`BroadcastsPage.tsx:1770`). It shows `Estimated recipients: N` above the form (`:163`), hidden on error. That includes the 403 a member without `lists:read`, or without `segments:read` on a segment-filtered broadcast, now receives.
5. i18n: `npm run lingui:extract`, then translate the new msgids in all seven non-English catalogues (`ca de es fr it ja pt-BR`), and `npm run lingui:compile`. `src/i18n/locales/catalogues.test.ts` fails on any empty `msgstr` outside the licence exemption, so this has to happen before the console tests run.

### Performance check (before merge)

On the test Postgres (`:5433`), seed a scratch database with `generate_series`: 200k contacts, a 150k-contact list, and 5 overlapping segments of 20k-150k members. Run `EXPLAIN (ANALYZE, BUFFERS)` on: the Phase 1 EXISTS fetch page and count against the old JOIN; and `CountBroadcastAudienceBySegment` against the existing `with_count` query (`segment_postgres.go:150`), which already runs on every Broadcasts page load. Acceptance: EXISTS no slower than the JOIN, and the per-segment query within ~2× of `with_count`. Drop the scratch database afterwards.

### Tests — Phase 2

| Implementation | Test file | Cases |
|---|---|---|
| `internal/domain/broadcast.go` | `internal/domain/broadcast_test.go` | `EstimateAudienceRequest.Validate`: missing `workspace_id`; missing `audience.list`; an empty segment id; valid without segments; valid with segments and `include_segment_counts`. |
| `internal/repository/contact_postgres.go` | `internal/repository/contact_postgres_test.go` | `TestCountBroadcastAudienceBySegment`: full SQL including the version join, the `LEFT JOIN` on the in-list subquery, the `deleted` filter and `GROUP BY`; args with and without `ExcludeUnsubscribed`, numbered continuously from the subquery's `?`s; `audience.Segments` does not reach the SQL; rows map to `{in_list, total}`; empty result gives an empty non-nil map; connection, query and scan errors are returned wrapped. |
| `internal/service/broadcast_service.go` | `internal/service/broadcast_service_test.go` | `TestBroadcastService_EstimateAudience`: auth failure; no `broadcasts:read` → `PermissionError`; no `lists:read` → `PermissionError`, even list-only; segments named without `segments:read` → `PermissionError`; `include_segment_counts` without `segments:read` → `PermissionError`; `broadcasts:read` + `lists:read` only, list-only request → total returned and `CountBroadcastAudienceBySegment` never called (strict gomock); full permissions → both; an error from either repo call is returned. |
| `internal/http/broadcast_handler.go` | `internal/http/broadcast_handler_test.go` | GET → 405; malformed JSON → 400; a **raw JSON body** with no `audience` → 400 (a typed struct literal cannot express a missing field); `PermissionError` → 403; service error → 500; success → 200 with `total`, and `segment_counts` absent when nil. |
| full stack | `tests/integration/broadcast_segment_audience_test.go` (extends Phase 1's fixture) | Through `suite.APIClient.Post`, logged in as the owner: `[A, B]` with `ExcludeUnsubscribed` → `total` 2; with `include_segment_counts` → A `{in_list: 2, total: 4}` (alice, bob on the list; erin unsubscribed and dave off-list only count in `total`), B `{in_list: 1, total: 2}`. A member from `factory.CreateUser()` + `AddUserToWorkspaceWithPermissions` with `domain.NewFullPermissions()` minus segments read, logged in with `client.Login(member.Email, "password")` (the pattern in `automation_e2e_test.go`): list-only request → 200, request naming segments → 403, `include_segment_counts` → 403. A second member without `lists:read`: list-only request → 403. |
| `UpsertBroadcastDrawer.tsx`, `useAudienceEstimate.ts` | `console/src/components/broadcasts/UpsertBroadcastDrawer.test.tsx` | Add `estimateAudience` to the existing `broadcastApi` mock. (1) No list: the option shows `(3)` from `users_count` and the estimate is not requested. (2) List selected, estimate `{in_list: 0, total: 5}` while `users_count` is 3: the option shows `(0 of 5)`, proving both numbers come from the estimate, plus `Estimated recipients: 0`. (3) Picking a segment re-requests with that segment in `audience.segments`. (4) Estimate fails: options fall back to `(3)` and no estimate line. (5) Empty `segments` prop → request carries `include_segment_counts: false`. (6) A segment missing from `segment_counts` shows `(0 of 0)`, and so does every segment when the response has no `segment_counts` field at all (the `omitempty` case); neither falls back to `users_count`. (7) Drawer never opened → no estimate request. |
| `SendOrScheduleModal.tsx` | `console/src/components/broadcasts/SendOrScheduleModal.test.tsx` (new) | Shows `Estimated recipients: N` for the broadcast's audience; requests without segment counts; no request while `visible` is false; hides the line when the estimate fails (including 403). |

### Changelog — Phase 2

Under `## [41.1] - WIP`, as an **Improvement** (the picker showed segment totals by design):

> **Improvement**: When creating a broadcast, each segment now shows how many of its contacts are on the selected list, such as "interested (0 of 3)", along with an estimated recipient count that also appears when you send or schedule (#440). The picker used to show each segment's whole size, so a segment with nobody on the list still looked populated.

### Docs (`../docs`, separate repo and PR)

`features/broadcast-campaigns.mdx`, "Segment Filters" section. The page never says how several selected segments combine; the picker means **in at least one of** them. Add that, and that such a contact still receives the broadcast once. Leave line 28's "AND/OR logic" alone: it describes conditions inside one segment's definition, which is accurate. Also mention the estimated recipient count shown while building and before sending. The "Select a List" promise of no duplicates becomes true again with Phase 1 and needs no edit.

## Out of scope

- **Sending during a segment rebuild.** The send reads every `contact_segments` row regardless of version. While a segment is being rebuilt, after its definition changed, a broadcast therefore reaches a mix of contacts matched under the old and the new definition. Filtering on version instead would reach only the contacts already re-evaluated. The honest fix is probably to warn, or block, when a selected segment is `building`. That is a separate issue.
- Publishing `broadcasts.estimateAudience` in the API reference (decided: later).
- A database backstop against duplicate sends. A unique index on `email_queue (source_id, contact_email)` would not be one: a row is deleted once sent (`MarkAsSent`, `email_queue_postgres.go:224-230`), so it would only guard rows still waiting. A real guard would sit on `message_history` and be its own migration and decision. Phase 1 removes the only known source of duplicates.

## Final step — run the tests for the touched layers

```bash
make test-domain
make test-repo
make test-service      # ./internal/service and ./internal/service/broadcast (orchestrator uses the regenerated mock)
make test-http
# integration: start Postgres :5433 + Mailpit first
docker compose -f tests/compose.test.yaml up -d
INTEGRATION_TESTS=true go test -tags integration,licdev -count=1 -v -run 'TestBroadcastSegmentAudience|TestBroadcastExcludesPendingDoubleOptIn_Integration|TestBroadcastPagination' ./tests/integration/
# 1st pattern: both new tests (repository-level + end-to-end send)
# 2nd pattern: the list-predicate regression test the helper extraction must keep green
# 3rd pattern: the existing paging tests, which exercise the rewritten fetch against a real database
# console: catalogues must be extracted and translated BEFORE the tests (catalogues.test.ts)
cd console && npm run lingui:extract && npm run lingui:compile \
  && npx vitest run src/components/broadcasts/ src/i18n/ src/__tests__/pages.smoke.test.tsx src/pages/BroadcastsPage.statusBadge.test.tsx \
  && npm run lint && npm run build
# the two page tests render BroadcastsPage, which mounts the drawers and the send modal
```

`npm test` is bare `vitest`, which starts watch mode, so use `vitest run`. `npm run build` is the only typecheck (`tsc -b`). Check that every integration test ran: the `-v` output must list each `Test*` name with `--- PASS`. An `ok` without them means the tags or `INTEGRATION_TESTS` were missing.
