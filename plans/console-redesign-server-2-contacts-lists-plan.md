# Plan — console redesign, server part 2: contacts and lists (spec 4)

## Need

The redesigned Contacts page (lists merged into it, contact sheet with activity filters and a
Web visits tab) needs four read-side server changes, delivered before its console PR:

1. `contacts.list` gains a `search` parameter;
2. `lists.list` gains `with_counts`;
3. `timeline.list` gains `entity_types`, and filters entries by the member's permissions;
4. two routes return a contact's web visits.

Spec (reviewed): "Spec 4 · Contacts et listes", section "Côté serveur" and the Backend
table of "Fichiers, tests et vérification". All open questions were decided by Pierre on
12 Nov 2026 as the spec recommends (list counts shown; `timeline.list` filtered server-side,
with a Fix line).

No table, column, index, migration or environment variable. Each route keeps the permission
it already requires; only `timeline.list` additionally filters by permissions.

## Changes

### 1. `contacts.list`: `search`

- `GetContactsRequest` gains `Search string` (`json:"search,omitempty"`), read by
  `FromQueryParams`, `TrimSpace`d, at most 100 characters (400 beyond).
- `contactRepository.GetContacts`: when non-empty, adds
  `(c.email ILIKE $p OR c.first_name ILIKE $p OR c.last_name ILIKE $p OR c.full_name ILIKE $p OR c.external_id ILIKE $p)`
  with `$p = '%' || escaped || '%'`, ANDed with the other filters. `%`, `_` and `\` are escaped
  with the existing `escapeLikePattern` (`broadcast_postgres.go:41`), so "john_doe" does not
  read `_` as a wildcard. PostgreSQL's default `LIKE` escape character is `\`, which is what
  that helper writes. Same cost as today's `email` filter (an `ILIKE` without trigram index).
- `openapi/paths/contacts.yaml`: `search` parameter of `contacts.list`; `make openapi-lint`
  and `make openapi-bundle` (the bundled `openapi.json` is regenerated in this PR; the docs
  repo copy is updated by the docs PR).

### 2. `lists.list`: `with_counts`

- `GetListsRequest` reads `with_counts` (`true`/`false`, anything else is 400).
- `domain.List` gains `ActiveCount *int` (`json:"active_count,omitempty"`): absent without the
  parameter, so no current response changes; a list without active subscriber carries `0`.
- New repository method `ListRepository.CountActiveContactsByList(ctx, workspaceID) (map[string]int, error)`:

  ```sql
  SELECT list_id, COUNT(*) AS active_count
  FROM contact_lists
  WHERE status = 'active' AND deleted_at IS NULL
  GROUP BY list_id
  ```

- New service method `ListService.GetActiveCounts(ctx, workspaceID) (map[string]int, error)`
  with the same authentication and `lists.read` check as `GetLists`. `GetLists` keeps its
  signature (it has internal callers, `list_service.go:341, 606`).
- `handleList` calls `GetLists`, then, only when `with_counts` is true, `GetActiveCounts`, and
  sets `ActiveCount` on each list (0 when missing from the map).
- Interfaces gain the methods; mocks regenerated with `go generate`.

### 3. `timeline.list`: `entity_types` and permission filtering (Fix)

- `TimelineListRequest` gains `EntityTypes []string`, read from `entity_types` (comma
  separated, trimmed, empty items dropped, deduplicated keeping the first order). `Validate`
  refuses any value outside `contact`, `contact_list`, `contact_segment`, `message_history`,
  `custom_event`, `inbound_webhook_event`, `webhook_event`, `automation`, `web_session`,
  `web_page` with a 400.
- `ContactTimelineRepository.List` gains `entityTypes []string`; non-empty adds
  `AND entity_type = ANY($n)` (pq array). `idx_contact_timeline_email_created_at` still serves;
  the cursor is unchanged.
- `ContactTimelineService.List` gains `entityTypes []string`. After the `contacts.read`
  check it removes from the requested types — or from all known types when none is requested —
  `message_history` without `message_history.read`, and `web_session` and `web_page` without
  `web_analytics.read`. When something was removed, the repository receives the remaining
  list explicitly; when nothing was removed and none was requested, it receives an empty list
  (no condition, as today). When every requested type is removed, the service returns an empty
  list and no cursor without calling the repository.
- `contact_timeline_handler.go` passes `req.EntityTypes`.
- Domain interfaces (`domain/contact_timeline.go`) change; mocks regenerated. Other callers
  of the repository `List` (if any) pass `nil`.
- CHANGELOG Fix line (released behaviour: members without these permissions read those
  entries today).

### 4. Contact web visits: two routes

Both in `WebAnalyticsHandler.RegisterRoutes`, inside the existing authenticated block, `GET`
only. Both require `web_analytics.read` **and** `contacts.read`, and answer 400
"web analytics is not enabled" when the workspace's `web_analytics` settings are absent or
`enabled` is false. The email goes through `domain.NormalizeEmail` before any query.

`GET /api/webAnalytics.contactVisits`: `workspace_id`, `email` (required), `filter`
(`all` default, `goal`, `email`), `limit` (1 to 50, default 20), `cursor`.

- `summary` (only without `cursor`, always over all of the contact's sessions whatever
  `filter`): `sessions`, `goal_sessions` (`goal_count > 0`), `email_sessions`
  (`channel_group = 'email'`), `median_duration_ms` (`percentile_cont(0.5)` on `duration_ms`,
  rounded), `first_session` (same fields as a session row, earliest `created_at`, then `id`),
  `goals` (at most 5: `goal_name`, `count`, `value` = sum of `goal_value`, by `count` desc then
  name), `top_pages` (3: `path`, `views`, by `views` desc then path).
- `sessions`: `id`, `session_date`, `created_at`, `duration_ms`, `pageview_count`,
  `landing_path`, `exit_path`, `channel`, `channel_group`, `referrer_domain`, `utm_source`,
  `utm_campaign`, `device`, `os`, `browser`, `country`, `goal_count`, `goal_value`,
  `first_goal_name` (sub-query on the session's `web_goals`, earliest `goal_at`, filtered by
  the same `session_date`, `session_id` and `contact_email`). Ordered by `created_at DESC, id DESC`;
  `filter=goal` adds `goal_count > 0`, `filter=email` adds `channel_group = 'email'`.
- `next_cursor`: opaque base64 of `created_at` (RFC 3339 nano) and `id` of the last row, set
  when one more row exists (fetch `limit + 1`).

`GET /api/webAnalytics.contactSessionPages`: `workspace_id`, `email`, `session_id`,
`session_date` (`YYYY-MM-DD`), all required. Returns `pages` (`entered_at`, `path`,
`duration_ms`, `page_number`, at most 200, by `entered_at`) and `goals` (`goal_at`,
`goal_name`, `goal_value`, `path`, by `goal_at`), both filtered by `session_date`,
`session_id` **and** `contact_email = $email`: a member cannot read another visitor's session
by changing the ID.

Every filter uses the bare `contact_email = $n` column so the partial indexes
`idx_web_sessions_contact_email`, `idx_web_pages_contact_email`, `idx_web_goals_contact_email`
(`schema/web_analytics_tables.go:117, 138, 158`) serve; `session_date = $n` prunes to one
partition for the pages route.

Layers:

- `internal/domain/web_analytics.go`: `ContactVisitsRequest` (+ `FromQuery`, `Validate`),
  `ContactSessionPagesRequest` (+ `FromQuery`, `Validate`), response types,
  `WebAnalyticsContactVisitsRepository` and `WebAnalyticsContactVisitsService` interfaces with
  `//go:generate mockgen`.
- `internal/repository/web_analytics_contact_visits_postgres.go` (new): the queries, on the
  workspace connection.
- `internal/service/web_analytics_contact_visits_service.go` (new): authentication, both
  permissions, enabled check (workspace settings), `NormalizeEmail`, summary only on the first
  page.
- `internal/http/web_analytics_handler.go`: two handlers; `NewWebAnalyticsHandler` takes the
  new service (nil keeps the routes unregistered, for existing tests).
- `internal/app/app.go`: builds repository and service, passes the service to
  `NewWebAnalyticsHandler`.
- Both routes are classified for the audit catalogue (`AuditExcludedRoutes` in
  `internal/domain/audit_actions.go`, read routes), as `internal/http/audit_routes_test.go`
  requires for every `/api/` route.

These two routes, `lists.list` and `timeline.list` are not in `openapi/` and stay internal to
the console.

## Files and tests (TDD: tests first)

| File | Change | Test file | Cases |
| --- | --- | --- | --- |
| `internal/domain/contact.go` | `Search` in `GetContactsRequest` and `FromQueryParams` | `internal/domain/contact_test.go` | read from the query; spaces trimmed; 101 characters refused, 100 accepted |
| `internal/repository/contact_postgres.go` | `OR` condition of `search`, wildcards escaped | `internal/repository/contact_postgres_test.go` | expected SQL (sqlmock); `search` combined with `list_id`; "a_b" sent as `%a\_b%`; `%` and `\` escaped |
| `internal/domain/list.go` | `ActiveCount`, `WithCounts` read by `GetListsRequest`, repository and service methods | `internal/domain/list_test.go` | `with_counts` true, false, absent, invalid (error); JSON without `active_count` when nil, with `0` when set to 0 |
| `internal/repository/list_postgres.go` | `CountActiveContactsByList` | `internal/repository/list_postgres_test.go` | expected SQL; map built from rows; query error returned |
| `internal/service/list_service.go` | `GetActiveCounts` | `internal/service/list_service_test.go` | refused without `lists.read`; auth error returned; repository map returned |
| `internal/http/list_handler.go` | `with_counts` | `internal/http/list_handler_test.go` | without the parameter the response has no `active_count` key and `GetActiveCounts` is not called; with it, each list carries its count and a list missing from the map carries `0`; service error → 500; permission error → 403 |
| `internal/domain/contact_timeline.go` | `EntityTypes`, validation, dedup; interface signatures | `internal/domain/contact_timeline_test.go` | comma-separated values; duplicates removed; blanks dropped; unknown value → error |
| `internal/repository/contact_timeline_postgre.go` | `= ANY` condition | `internal/repository/contact_timeline_postgre_test.go` | condition and array argument present with types, absent without; cursor condition kept together with the filter |
| `internal/service/contact_timeline_service.go` | new `List` signature, permission filtering | `internal/service/contact_timeline_service_test.go` | full permissions and no type: repository gets no type; without `message_history.read`, `message_history` removed with no type requested and with `message_history,contact` requested; without `web_analytics.read`, `web_session` and `web_page` removed; every requested type removed → empty list, no cursor, repository not called; without `contacts.read` still refused |
| `internal/http/contact_timeline_handler.go` | reads `entity_types`, passes it | `internal/http/contact_timeline_handler_test.go` | parameter passed to the service; 400 on an unknown value |
| `internal/domain/web_analytics.go` | visit request/response types, interfaces, mockgen | `internal/domain/web_analytics_test.go` | `contactVisits`: email required, unknown `filter` refused, `limit` 0 and 51 refused, defaults (`all`, 20); `contactSessionPages`: each of the four parameters required, bad `session_date` refused |
| `internal/repository/web_analytics_contact_visits_postgres.go` (new) | queries | `internal/repository/web_analytics_contact_visits_postgres_test.go` | sqlmock: bare `contact_email = $1`; order `created_at DESC, id DESC`; cursor condition; `goal` and `email` filters; summary queries take no `filter` condition; goals and pages tie order in the SQL; pages and goals filtered by `session_date`, `session_id` and `contact_email` |
| `internal/service/web_analytics_contact_visits_service.go` (new) | permissions, enabled check, `NormalizeEmail` | `internal/service/web_analytics_contact_visits_service_test.go` | without `web_analytics.read` → permission error; without `contacts.read` → permission error; settings absent or disabled → the dedicated error; email in upper case normalised before the repository call; summary requested without cursor, not with one |
| `internal/http/web_analytics_handler.go` | two routes | `internal/http/web_analytics_handler_test.go` | non-GET → 405; missing parameter → 400; disabled → 400 with the message; permission error → 403; success JSON shape |
| `internal/domain/audit_actions.go` | two routes in `AuditExcludedRoutes` | `internal/http/audit_routes_test.go` (existing) | passes with the new routes |
| `internal/app/app.go` | wiring | `internal/app/app_test.go` | the app builds with the new service (existing construction tests) |
| `openapi/paths/contacts.yaml` | `search` on `contacts.list` | `make openapi-lint` | — |
| `tests/integration/contacts_lists_redesign_test.go` (new) | full stack | same | `TestContactsListsRedesign`: `search` finds by first name, by full name and by external ID, and "a_b" does not match "axb"; `with_counts` on two lists (one active, one unsubscribed, one empty list → 0); `entity_types=contact` returns only contact entries; a member without `message_history.read` gets no `message_history` entry with no filter |
| `tests/integration/web_analytics_contact_visits_test.go` (new) | full stack | same | `TestWebAnalyticsContactVisits`: two contacts with sessions on two monthly partitions: each sees only theirs, summary counts and median; `filter=goal`; pagination with `next_cursor`; `contactSessionPages` with another contact's email returns nothing; goals and pages in order; disabled analytics → 400 |
| `CHANGELOG.md` | under `[44.0] - unreleased` | — | — |

CHANGELOG lines (short, customer-facing; the console PR extends the Feature with the screen):

- **Feature**: `contacts.list` takes `search`, which matches email, names and external ID.
- **Fix**: A contact's activity no longer shows messages and website visits to members who
  are not allowed to read them.

## Out of scope

- Console (screen PR), docs PR (copy of `openapi.json`, pages listed in the spec).
- `EXPLAIN ANALYZE` of `with_counts` and `contactVisits` on the largest demo workspace: done
  in the console PR, which issues them (spec 4, Vérification).

## Verification

```bash
make test-domain && make test-repo && make test-service && make test-http
INTEGRATION_TESTS=true go test -race -tags integration,licdev -run 'TestContactsListsRedesign|TestWebAnalyticsContactVisits' ./tests/integration/ -v
make openapi-lint && make openapi-bundle
```

Then `make test-unit` once before review.

## Plan review and corrections

Review: `reviews/console-redesign-server/plan-2-review.md` in the project files. Every finding
was checked in the code; the corrections below take precedence over the sections above.

- **S1**: a contact without sessions: `median_duration_ms` is `COALESCE(ROUND(percentile_cont(0.5) …)::bigint, 0)`,
  `first_session` is `null`, `goals` and `top_pages` are `[]`. Repository and integration cases.
- **S2**: `session_date` is a `YYYY-MM-DD` string in session rows, the format
  `contactSessionPages` takes back; round-trip test.
- **S3**: `session_id` and the cursor are validated in `Validate` (`uuid.Parse`, cursor
  decoding): 400, with tests.
- **S4**: pages ordered `entered_at, tab_id, page_number`; goals `goal_at, tab_id,
  client_ts_ms, goal_name`; same order in the `first_goal_name` sub-query; `tab_id` returned
  with pages and goals; a multi-tab test.
- **S5**: the handler gets the service through a setter `WithContactVisits(svc)` (as
  `WithWebAnalyticsCacheInvalidator`), so the five `NewWebAnalyticsHandler` call sites are
  untouched; a test checks the routes answer 401 (not 404) once the setter is used.
- **S6 (decided by Pierre on 12 Nov 2026: hidden with message history)**:
  `inbound_webhook_event` entries carry message data (template, message ID, bounce
  diagnostics), so without `message_history.read` they are dropped from `timeline.list` like
  message entries (added to the deny-list, one service test and one integration case). Website
  goals reach the timeline as `custom_event` entries and stay.
- **S7**: the server filters by permission only (spec §3), not by whether web analytics is
  enabled; the console hides the Web filter when it is disabled.
- **S8**: sentinel `domain.ErrWebAnalyticsNotEnabled` → 400 via `errors.Is`; permission →
  403; validation → 400; anything else → 500 with a fixed message (not `err.Error()`). The
  service takes `WorkspaceRepository` to read the settings. Signatures:
  `ContactVisits(ctx, req ContactVisitsRequest) (*ContactVisitsResponse, error)` and
  `ContactSessionPages(ctx, req ContactSessionPagesRequest) (*ContactSessionPagesResponse, error)`.
- **N14 (adopted)**: `contactSessionPages` first checks that the session belongs to the
  contact (`web_sessions` by `session_date`, `id`, `contact_email`), then reads its pages and
  goals by primary key (`session_date`, `session_id`): same guarantee, and pages of a tab
  stamped before identification are not lost. Unknown or foreign session → empty `pages` and
  `goals`.
- N1: `webhook_event` kept with a "legacy (pre-v19)" comment. N2: `ct.entity_type = ANY($n)`.
  N3: search pattern built in Go, five `ILike` arguments. N4: 100 runes
  (`utf8.RuneCountInString`), NUL and invalid UTF-8 refused. N5: every type removed → `[]`
  (tested). N6: existing timeline service tests updated for the reduced type list. N7: with
  no type requested and something removed, the repository gets a deny-list
  (`ct.entity_type <> ALL($n)`), so a later type is not hidden by accident. N8: `LIMIT` values
  and `limit + 1` pinned in sqlmock tests. N9: previous month's partition created in the
  integration test; list counts phrased as two lists with pinned values. N10: Fix line says
  "members and API keys". N11: `internal/app` tests run. N12: `GetLists` gets a `withCounts`
  option instead of a second service method (its only caller is the handler), one
  authentication per request. N13: no other repository `List` caller.
