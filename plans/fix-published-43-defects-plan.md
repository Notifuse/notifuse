# Fix defects published in 43.0 (console redesign, wave 1, thread B)

Corrections of behaviour already shipped in 43.0, found while writing the console redesign
specs 3 to 10. Each is a small PR from `dev` to `dev`, with no new screen, no table, column,
index, migration or environment variable. `internal/migrations/v44.go` is not touched (thread C
owns it, including the `stats.exited` recount of spec 6 §5 bis, and owns `timeline.list`
permissions of spec 4).

Every PR adds its own line(s) at the end of `## [44.0] - unreleased` in `CHANGELOG.md`.

Out of scope here (owned elsewhere): spec 6 "Inflight" in Flow Stats (replaced by the new
`automations.nodeStats`, thread C + screen thread), spec 4 `timeline.list` permission filtering
(thread C), spec 8 "View more" `period: 'all'` (the period filter does not exist yet).

---

## PR 1 — `fix/broadcast-get-template-version`

**Defect.** `BroadcastHandler.HandleGet` with `with_templates=true` loads version **1** of each
variation's template (`internal/http/broadcast_handler.go:152`), while `broadcasts.list`
(`broadcast_service.go:290`) and sending (`broadcast/orchestrator.go:155`) use version 0, the
latest. A broadcast opened in the console shows the template's first subject and content.

**Change.** Pass `0` instead of `1`.

| File | Test file | Cases |
|---|---|---|
| `internal/http/broadcast_handler.go` | `internal/http/broadcast_handler_test.go` | the existing `with_templates=true` cases expect `GetTemplateByID(…, "template123", int64(0))` instead of `gomock.Any()` (pins the version) |
| — | `tests/integration/broadcast_get_templates_test.go` (new) | template updated to version 2 (new subject): `broadcasts.get?with_templates=true` returns version 2's subject and `version: 2`, the same as `broadcasts.list?with_templates=true` |

CHANGELOG: `- **Fix**: A broadcast opened in the console now shows the latest version of its templates, the one that is sent, instead of their first version.`

---

## PR 2 — `fix/automation-get-not-found-and-reply-exits`

### 2a. `automations.get` answers 404

**Defect.** `AutomationRepository.GetByIDTx` returns an untyped `fmt.Errorf("automation not found: %s")`
on `sql.ErrNoRows` (`automation_postgres.go:172-174`); `handleGet` answers 500 "Failed to get
automation" (`automation_handler.go:100-107`).

**Change.**
- `GetByIDTx` returns `&domain.ErrNotFound{Entity: "automation", ID: id}` (message
  "automation not found with ID: …", still containing "automation not found").
- `AutomationService.Get` already wraps with `%w`, so `errors.As` sees it.
- `handleGet`: after `writePermissionError`, `errors.As(err, &notFound)` → 404 "Automation not found".
- Other callers of `GetByID` (`automation_service.go:153, 432, 511`, `automation_executor.go:81`)
  only wrap or log the error; none matches on its text (checked by grep). Their HTTP answers stay
  as they are (500 on update/delete/activate/pause of an unknown id, listed as known, not treated).

| File | Test file | Cases |
|---|---|---|
| `internal/repository/automation_postgres.go` | `internal/repository/automation_postgres_test.go` | not-found case: `errors.As(err, *domain.ErrNotFound)` true, `Entity == "automation"`, `ID` is the id |
| `internal/http/automation_handler.go` | `internal/http/automation_handler_test.go` | `handleGet`: service returns wrapped `ErrNotFound` → 404; other error → 500 (existing); permission error → 403 (existing) |
| — | `tests/integration/automation_get_not_found_test.go` (new) | `GET /api/automations.get` on an unknown id → 404; on a soft-deleted automation → 404 |

CHANGELOG: `- **Fix**: \`automations.get\` answers 404 instead of 500 for an automation that does not exist.`

### 2b. Exits on reply counted in `stats.exited`

**Defect.** `ExitContactJourneysOnReply` (`automation_postgres.go:568-595`) moves journeys to
`exited` with a bulk `UPDATE` and never increments `automations.stats.exited`, while every other
exit goes through `IncrementAutomationStat`. Published since 39.x.

**Change.** One statement run with `QueryContext` (spec 6 §5):

```sql
WITH exited AS (
  UPDATE contact_automations SET status = 'exited', scheduled_at = NULL, exit_reason = $1
  WHERE contact_email = $2 AND status = 'active' AND entered_at < $3
    AND automation_id IN (SELECT id FROM automations WHERE exit_on_reply = TRUE AND deleted_at IS NULL)
    [AND automation_id = $5]
  RETURNING automation_id
), counts AS (
  SELECT automation_id, COUNT(*) AS n FROM exited GROUP BY automation_id
)
UPDATE automations a
SET stats = COALESCE(a.stats, '{}'::jsonb) ||
            jsonb_build_object('exited', COALESCE((a.stats->>'exited')::int, 0) + c.n),
    updated_at = $4
FROM counts c WHERE a.id = c.automation_id
RETURNING c.n
```

The function sums the returned `n` (journeys exited), as it returns today. Same JSONB write as
`IncrementAutomationStat`. Row lock on `automations` serialises concurrent exits. No migration:
thread C's v44 recount (`GREATEST`, never lower) stays compatible.

Note: `stats` may hold JSON `null` (see `automation_stats_scalar_bug_test.go`); `COALESCE` covers SQL
NULL only. Check how `IncrementAutomationStat` and the v35 fix treat a JSON `null` / scalar and
copy that guard exactly.

| File | Test file | Cases |
|---|---|---|
| `internal/repository/automation_postgres.go` | `internal/repository/automation_postgres_reply_test.go` (sqlmock) | with an automation id: query contains `WITH exited AS`, `RETURNING c.n`, args in order, returns the summed `n`; without id: no automation filter; two rows (two automations) → sum; no row → 0; query error wrapped |
| — | `tests/integration/automation_reply_exit_stats_test.go` (new) | real DB: two exit_on_reply automations with an active journey each + one without exit_on_reply; reply exit with `automationID=nil` → returns 2, each exit_on_reply automation `stats.exited` +1, the other untouched, `enrolled`/`completed` untouched; `stats` previously `{"exited":4}` → 5; journey entered after `before` not exited; two concurrent calls on two contacts of the same automation → `exited` +2 |

CHANGELOG: `- **Fix**: Contacts who leave an automation by replying now count in its exited total.`
(thread C may extend it with "past exits included" when its v44 recount lands.)

---

## PR 3 — `fix/logs-filters`

### 3a. Status filter of outgoing webhook deliveries

**Defect.** The Outgoing webhooks tab offers a Status filter (`OutgoingWebhooksTab.tsx:80-90`) but
never sends it (`:153-158` passes only `subscription_id`), and the server has no such parameter.

**Change.**
- `handleGetDeliveries` reads `status`; unknown value (not one of `pending`, `delivering`,
  `delivered`, `failed`, the `WebhookDeliveryStatus*` constants) → 400.
- `WebhookSubscriptionService.GetDeliveries(ctx, workspaceID, subscriptionID, status *string, limit, offset)`.
- `webhookDeliveryRepository.ListAll(ctx, workspaceID, subscriptionID, status *string, limit, offset)`:
  one Squirrel-built `WHERE` shared by the count and the page, with optional `subscription_id`
  and `status`. Interfaces (`domain.WebhookDeliveryRepository`, service interface in
  `internal/http`) and mocks regenerated.
- Console: `webhookSubscriptionApi.getDeliveries(workspaceId, subscriptionId, limit, offset, status?)`
  (status appended last so existing calls keep compiling), passed from `apiFilters.status`.
- `openapi/paths/webhook-subscriptions.yaml`: `status` query parameter with its enum; `make openapi-bundle`.

| File | Test file | Cases |
|---|---|---|
| `internal/repository/webhook_delivery_postgres.go` | `internal/repository/webhook_delivery_postgres_test.go` | the four combinations (none, subscription, status, both): count and page queries carry the same conditions and args |
| `internal/service/webhook_subscription_service.go` | `internal/service/webhook_subscription_service_test.go` | status passed to the repo; existing auth cases unchanged |
| `internal/http/webhook_subscription_handler.go` | `internal/http/webhook_subscription_handler_test.go` | `status=failed` passed to the service; `status=bogus` → 400; absent → nil |
| `console/src/services/api/webhook_subscription.ts`, `console/src/components/webhooks/OutgoingWebhooksTab.tsx` | `console/src/components/webhooks/OutgoingWebhooksTab.test.tsx` | with `?status=failed` in the URL, `getDeliveries` is called with `'failed'` |
| — | `tests/integration/webhook_deliveries_status_filter_test.go` (new) | deliveries in each status on two subscriptions: `status=failed` returns only failed ones and `total` counts them; with `subscription_id` too; invalid status → 400 |

### 3b. Auth email and User created types of incoming webhooks

**Defect.** `InboundWebhookEventListParams.Validate` (`internal/domain/inbound_webhook_event.go:224-237`)
accepts six of the eight `EmailEventType` values; the console offers `auth_email` and
`before_user_created`, which answer 400.

**Change.** Accept the eight types (add `EmailEventAuthEmail`, `EmailEventBeforeUserCreated`).

| File | Test file | Cases |
|---|---|---|
| `internal/domain/inbound_webhook_event.go` | `internal/domain/inbound_webhook_event_test.go` | `auth_email` and `before_user_created` valid; `bogus` still rejected |
| — | `tests/integration/` (in the 3a test file, renamed `logs_filters_test.go`) | `inboundWebhookEvents.list?event_type=auth_email` → 200 with the auth_email event only |

### 3c. Tab kept in the URL of the logs tabs

**Defect.** `MessageHistoryTab`, `InboundWebhookEventsTab` and `OutgoingWebhooksTab` rewrite the URL
with `window.history.pushState(pathname + filters)` on mount and on every filter change
(`MessageHistoryTab.tsx:353-365, 459`, `InboundWebhookEventsTab.tsx:~150-160, 248`,
`OutgoingWebhooksTab.tsx:128-141, 231`), dropping `tab`. A copied link of a webhooks tab,
filtered or not, reopens the Messages tab.

**Change.** New helper `console/src/components/logs/logsUrl.ts`:
`buildLogsTabUrl(pathname: string, currentSearch: string, filters: {field, value}[]): string`,
keeping `tab` from the current search and writing only the given filters. The six `pushState`
call sites use it. Behaviour otherwise unchanged (still `pushState`).

| File | Test file | Cases |
|---|---|---|
| `console/src/components/logs/logsUrl.ts` (new) | `logsUrl.test.ts` (new) | keeps `tab=inbound`; drops other old params; writes filters; no `tab` and no filter → bare pathname; values URL-encoded |
| the three tabs | their existing `*.test.tsx` | after mount with `?tab=inbound&event_type=bounce`, `window.location.search` still has `tab=inbound` (Inbound and Outgoing at least) |

### 3d. "View more" of the failed messages card

**Defect.** `FailedMessagesTable.handleViewMore` navigates with `search: { is_failed: 'true' }`;
the router serialises the string as `is_failed=%22true%22`, `MessageHistoryTab` reads `"true"`
(quoted) ≠ `'true'` → `is_failed=false` → the messages that did **not** fail.

**Change.** `search: { tab: 'messages', is_failed: true }` (boolean, serialised `is_failed=true`).

| File | Test file | Cases |
|---|---|---|
| `console/src/components/analytics/FailedMessagesTable.tsx` | `FailedMessagesTable.test.tsx` (existing or new) | View more calls `navigate` with `search: { tab: 'messages', is_failed: true }` |
| — | `console/src/router` round-trip check in the same test (or `MessageHistoryTab.test.tsx`) | `is_failed=true` in the URL yields `is_failed: true` in the `messages.list` call |

CHANGELOG (PR 3):
- `- **Fix**: The status filter of outgoing webhook deliveries now filters them. \`webhookSubscriptions.deliveries\` takes \`status\`.`
- `- **Fix**: Filtering incoming webhooks by Auth email or User created no longer fails.`
- `- **Fix**: A link to a webhooks tab, filtered or not, opens that tab again, not the messages.`
- `- **Fix**: View more on the failed messages card opens the failed messages.`

Docs (separate PR on Notifuse/docs, to `dev`): copy the bundled `openapi.json`.

---

## PR 4 — `fix/blog-scheduled-posts-and-list`

### 4a. Scheduled post offline until its date

**Defect.** A post whose `published_at` is in the future is public (post page, home page,
category page, sitemap, `GetPublicPostByCategoryAndSlug`); only RSS checks the date
(`blog_postgres.go:857-858`, `:933`).

**Change.**
- `BlogPost.IsLive(now time.Time) bool`: `PublishedAt != nil && !PublishedAt.After(now)`.
- `blog_service.go:803` (`GetPublicPostByCategoryAndSlug`), `:1359` (post page render), `:1514`
  (`RenderPostContent`) use `IsLive(time.Now().UTC())` instead of `IsPublished()`; a scheduled
  post answers "post not found" / `ErrCodePostNotFound` like a draft.
- `ListBlogPostsRequest.PublishedBefore *time.Time` (`json:"-"`, never read from the API; `FromURLParams`
  untouched). Repository `ListPosts` adds `published_at <= $n` to both count and page when set.
- Public lists set it to `time.Now().UTC()`: `ListPublicPosts` (`:813`, used by the sitemap,
  `root_handler.go:567`), home page (`:1181`), category page (`:1914`).
- RSS (`ListFeedPosts`, `blog_postgres.go:858, 933`): `NOW()` → a `time.Now().UTC()` parameter,
  since `published_at` is `TIMESTAMP` without time zone and `NOW()` depends on the session zone.
- `IsPublished()` keeps its meaning for the console and the API (`status=published`).

| File | Test file | Cases |
|---|---|---|
| `internal/domain/blog.go` | `internal/domain/blog_test.go` | `IsLive`: nil → false; past → true; equal to now → true; future → false |
| `internal/repository/blog_postgres.go` | `internal/repository/blog_postgres_test.go` | `PublishedBefore` set → `published_at <= $n` in count and page with the arg; unset → absent; feed query takes a time arg instead of `NOW()` |
| `internal/service/blog_service.go` | `internal/service/blog_service_test.go` | scheduled post: `GetPublicPostByCategoryAndSlug` → error; post page render → `ErrCodePostNotFound`; `RenderPostContent` → not found; `ListPublicPosts`, home and category pages pass a `PublishedBefore` close to now |
| `internal/http/root_handler.go` | (no change; sitemap goes through `ListPublicPosts`) | — |
| — | `tests/integration/blog_scheduled_post_test.go` (new) | one live post and one scheduled for tomorrow: home page, category page, post page (404 for the scheduled one), sitemap and RSS show only the live one; console `blogPosts.list?status=published` still lists both |

### 4b. Blog post list beyond 100 posts

**Defect.** `PostsTable.tsx:70-78` fetches `limit: 100` once and paginates on the client: posts
101+ never show.

**Change.** Server-side pagination with the existing API: a `page` state (reset to 1 when status
or category changes), query key `['blog-posts', workspaceId, status, categoryId, page]`,
`blogPostsApi.list(..., { limit: 50, page })`, and the antd `Table` `pagination={{ current: page,
pageSize: 50, total: data.total_count, onChange: setPage, showTotal }}`. `keepPreviousData`
(`placeholderData`) while a page loads. Invalidations on `['blog-posts', workspaceId]` keep working (prefix).

| File | Test file | Cases |
|---|---|---|
| `console/src/components/blog/PostsTable.tsx` | `console/src/components/blog/PostsTable.test.tsx` (new) | first call `{ limit: 50, page: 1 }`; with `total_count: 120`, clicking page 3 calls `{ limit: 50, page: 3 }`; changing status resets to page 1 |

CHANGELOG (PR 4):
- `- **Fix**: A scheduled blog post now appears on the blog, in its sitemap and at its address only from its publication date.`
- `- **Fix**: The blog's post list in the console now shows every post, not only the first 100.`

---

## PR 5 — `fix/file-manager-listing`

The file manager talks to the bucket from the browser (`console/src/components/file_manager/fileManager.tsx`).

### 5a. More than 1,000 files, and folders created outside Notifuse

**Defect.** `fetchObjects` sends one `ListObjectsV2` (`fileManager.tsx:149-238`): at most 1,000
objects. A folder exists only if it has a `…/` marker object, so files under a prefix created by
another S3 tool (or left by 5b) are invisible.

**Change.** New module `console/src/components/file_manager/storageObjects.ts`:
- `listAllObjects(client, bucket, { prefix?, maxObjects? })`: loops on `ContinuationToken` while
  `IsTruncated`; stops at `maxObjects` (default `MAX_OBJECTS = 10_000`) and returns
  `{ objects, capped }`.
- `toStorageItems(objects, settings)`: the current mapping (moved as is), plus one folder item
  for each intermediate prefix of a file key that has no marker (implicit folder), deduplicated.
- `fetchObjects` uses both; when `capped`, an antd `Alert` (warning) above the table:
  "Showing the first 10,000 files of this bucket." (Lingui, extracted, 7 translations).

### 5b. Deleting a folder deletes its content

**Defect.** `deleteObject(key, true)` deletes only the folder marker; the confirmation promises
"with all its content". Files stay in the bucket, invisible.

**Change.** `deleteFolder(client, bucket, prefix)` in `storageObjects.ts`: `listAllObjects` with
`Prefix` (no cap), then `DeleteObject` per key (marker included), at most 5 at a time (same as the
current multi-delete, `fileManager.tsx:586-600`); returns `{ deleted, failed }`. `DeleteObjects` is
not used (checksum header refused by several S3-compatible providers). The folder branch of
`deleteObject` calls it; a partial failure shows an error with the failed count. An implicit
folder (no marker) is deleted the same way.

| File | Test file | Cases |
|---|---|---|
| `storageObjects.ts` (new) | `storageObjects.test.ts` (new, mocked `send`) | three pages read (`send` called 3 times, tokens chained); cap of 2 objects with pages of 2 → stops, `capped: true`; implicit folders `a/` and `a/b/` created for `a/b/c.png`, none duplicated when a marker exists; item paths/names as today for files and folders; `deleteFolder`: lists with `Prefix`, one `DeleteObject` per key incl. marker, concurrency ≤ 5, one rejection → `failed: 1` |
| `fileManager.tsx` | covered through `storageObjects.test.ts`; `npx tsc -b` | — |

CHANGELOG (PR 5):
- `- **Fix**: The file manager now lists every file of the bucket, beyond the first 1,000, and shows folders created outside Notifuse.`
- `- **Fix**: Deleting a folder in the file manager now deletes its files too.`

---

## Process (each PR)

1. `git fetch origin dev`, branch from `origin/dev`.
2. Tests first (unit, then integration), then code, until green.
3. Commands run for the layers touched:
   - Go: `make test-domain`, `make test-repo`, `make test-service`, `make test-http` as touched;
     the new integration tests alone with
     `INTEGRATION_TESTS=true go test -race -tags integration,licdev -run '<Name>' ./tests/integration/`
     (Postgres on :5433); `go vet ./...`; `make openapi-bundle` for PR 3.
   - Console: `npx vitest run <touched dirs>`, `npm run lingui:extract` (PR 5), `npm run lint`,
     `npx tsc -b`, `npm run build`.
4. Code review by an independent Fable reviewer (high effort) on the PR's last commit; confirmed
   findings fixed; CI green on that commit before announcing it ready.

---

## Corrections after spec review (supersede the sections above where they differ)

Spec review: Opus, xhigh effort, on this plan at `origin/dev` de243ef. Every finding was checked in
the code; all are integrated below.

**PR 1.** The console never calls `broadcasts.get` with `with_templates` (only
`BroadcastSelectorInput.tsx:49`, without it); the broadcasts page uses `broadcasts.list`, already at
version 0. The defect reaches API clients only. CHANGELOG:
`- **Fix**: \`broadcasts.get\` with \`with_templates=true\` now returns the latest version of each variation's template, the one that is sent, instead of its first version.`
Integration test in the existing `tests/integration/broadcast_handler_test.go`, calling
`client.Get("/api/broadcasts.get", …)` directly (`GetBroadcast` takes no extra params).

**PR 2a.** Existing handler case "not found" (`automation_handler_test.go:170-180`, untyped error →
500) renamed "untyped error → 500"; typed case added. `automation_e2e_test.go:1836-1840`
(soft-deleted automation not 200) tightened to 404. `handleGet` logs a 404 at Warn, not Error.
Other routes on an unknown id keep their 500 (known, not treated).

**PR 2b.**
- Stop-on-reply shipped in 33.0. `DeleteTx`'s bulk `automation_deleted` exit also skips the
  counter: known, not treated (the automation is deleted).
- JSON `null` or scalar `stats`: `'null'::jsonb || {...}` gives an array, which then breaks
  `GetByIDTx`'s unmarshal. Guard with `jsonb_typeof`:
  `stats = (CASE WHEN jsonb_typeof(a.stats) = 'object' THEN a.stats ELSE '{}'::jsonb END) || jsonb_build_object('exited', COALESCE((CASE WHEN jsonb_typeof(a.stats) = 'object' THEN a.stats->>'exited' END)::int, 0) + c.n)`.
- Deadlock: lock the automations rows in id order before updating:
  `locked AS (SELECT a.id FROM automations a JOIN counts c ON c.automation_id = a.id ORDER BY a.id FOR UPDATE)`,
  outer `UPDATE … FROM counts c JOIN locked l ON l.id = c.automation_id`.
- Args: `$1 reason, $2 email, $3 before, $4 updated_at, $5 automation_id` (optional filter inside
  the CTE, query built with `fmt.Sprintf`). `QueryContext`, rows summed, `rows.Err` checked.
- Caller: `InboundWebhookEventService.ProcessInboundReply` (`inbound_webhook_event_service.go:200`), logs `n` only.
- Tests: existing `automation_postgres_reply_test.go` cases move from `ExpectExec`/`$4` to
  `ExpectQuery` with the new numbering; added: `jsonb_typeof` and `FOR UPDATE` in the query,
  `rows.Err` error. Integration adds: `stats` = `'null'::jsonb` → `{"exited":1}`.

**PR 3a.** No service interface in `internal/http` (the handler holds `*service.WebhookSubscriptionService`).
Update: `mocks/mock_webhook_delivery_repository.go` (go:generate at `domain/webhook_subscription.go:4`),
the hand-written `fakeDeliveryStore.ListAll` (`internal/service/webhook_delivery_worker_test.go:1571`),
`ListAll` expectations in `internal/http/webhook_subscription_handler_coverage_test.go` and
`webhook_subscription_service_test.go`, regexes in `webhook_delivery_postgres_test.go:350-400`.
Handler cases go in `_coverage_test.go`. Docs PR also adds `&status=failed` to the deliveries
example in `features/webhooks.mdx`.

**PR 3c.** Each tab passes its own key to the helper (`messages`, `incoming-webhooks`,
`outgoing-webhooks`, the keys of `LogsPage.tsx:53-73`) instead of reading `tab` from the URL in the
effect (the router writes the URL in a microtask): `buildLogsTabUrl(pathname, tabKey, filters)`.
Tests use the real keys, the Outgoing test its `status` filter. Known, not treated: changing tab
keeps the previous tab's params (an audit `id` becomes the Messages `id` filter).

**PR 3d.** Round trip tested with `defaultStringifySearch` from `@tanstack/react-router`:
`{is_failed:'true'}` → `?is_failed=%22true%22`, `{is_failed:true}` → `?is_failed=true`.

**PR 4a.**
- The API reads list params through `parseListBlogPostsParams` (`blog_handler.go:532-557`), which
  never sets `PublishedBefore`.
- Feed: `ListFeedPosts` (`blog_postgres.go:858`) and `GetFeedFingerprint` (`:933`) compute
  `time.Now().UTC()` inside the repository and pass it as a parameter (no signature change).
  `PublishedBefore` is passed as `.UTC()`.
- Home and category pages are cached 5 minutes (`BlogCacheTTL`): a scheduled post shows there up to
  5 minutes after its date; the post page checks before the cache. Tests do not rely on the cache.
  Docs already promise it (`blog/posts.mdx:47-54`).
- Known, not treated: the sitemap asks for 1,000 posts but `Validate` caps at 100
  (`root_handler.go:566-570`, `domain/blog.go:621-624`), so it lists the 100 newest.

**PR 4b.** `page` always in the query key, which also separates it from `BlogPage.tsx:47-51`'s
`['blog-posts', ws, 'all', undefined]` (`limit: 1`) key it collides with today. Empty state based on
`total_count`; when a page comes back empty with `total_count > 0`, go to the last page. Table
`loading` from `isFetching`. Tests: keys differ; empty page N → last page.

**PR 5.**
- Folder delete stays sequential, like the current multi-delete (`fileManager.tsx:587-600`, a
  `for … await` loop).
- `deleteFolder` rejects an empty prefix or one not ending in `/`, without calling `send` (test).
- Implicit folders are derived from every key, folder markers included (a marker `a/b/` without
  `a/` makes `a/`). Their `last_modified` is the newest descendant's (`dayjs(undefined)` would
  show today). Tests for both.
- Listing loop also stops when `IsTruncated` is set without `NextContinuationToken`.
- Partial failure message is a new Lingui string: extracted and translated in the 7 other locales.
- CHANGELOG: `- **Fix**: The file manager now lists up to 10,000 files of the bucket instead of the first 1,000, says when there are more, and shows folders created outside Notifuse.`
