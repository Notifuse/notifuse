# Fix: contacts removed from a list still count as its members (B24)

Created 2026-10-15. Checklist item B24 in `plans/made-up-tests-audit-checklist-plan.md`. Status: **implemented** (2026-10-16, branch `claude/made-up-tests-audit-high-ciyb05-b24`); see "Implementation".
- Pierre chose the scope on 2026-10-15: the full fix, with a v42 migration.
- An independent review then checked every claim and the design against the code. Its corrections are folded in; see "Independent review".
- Pierre settled the two remaining decisions on 2026-10-16:
  - P1: a re-add records what a subscribe does;
  - P2: the current unreleased work (41.1 so far) is released as v42, with B24 in it.

Line numbers are from HEAD `c974080a`. B1 (with D5 and D6) and the SSRF fix have uncommitted edits in `contact_postgres.go`, `list_service.go`, `automation_node_executor.go`, `automation_service.go` and `notification_center/src/App.tsx`, so working-tree lines differ there.

## What a user sees

Removing a contact from a list is a soft delete. It happens through the console (one contact, or the bulk action), `contactLists.removeContact`, or an automation's Remove from List node. The console promises "This is a true removal, not an unsubscribe" (`console/src/components/contacts/BulkActionsBar.tsx:103`). Yet the removed contact:
- keeps receiving every broadcast to the list, whatever `exclude_unsubscribed` says;
- stays in segments built on "is in list X", and never enters "not in list X" segments;
- still passes automation trigger conditions and filter or branch conditions on that list;
- still sees the list as subscribed in the notification center, private lists included;
- still has the list in the API's `contact_lists` and in Liquid's `contact.contact_lists`, marked `active`.

A List Status Branch, `contacts.list`, the list stats and the contact drawer already treat them as removed. Two nodes of the same automation can therefore disagree about the same contact.

## Verified facts

Levels: **Reproduced** means run on a real database through the API (probes listed under "Probes"); **Code-confirmed** means every step was read. The independent review re-checked every fact.

- **F1 — Removal is a soft delete that keeps the status.** `RemoveContactFromList` runs `UPDATE contact_lists SET deleted_at = $1 WHERE email = $2 AND list_id = $3` (`internal/repository/contact_list_postgres.go:280`). It has two callers:
  - `contactLists.removeContact` (`internal/service/contact_list_service.go:234`), which the console calls for one contact and, contact by contact, for the bulk action (`console/src/pages/ContactsPage.tsx:318`);
  - the Remove from List node (`internal/service/automation_node_executor.go:874`).

  Reproduced: after `removeContact`, the row is `status=active` with `deleted_at` set.
- **F2 — These readers already treat a removed row as no membership:**
  - `GetContactListByIDs` (`contact_list_postgres.go:143`), used by the List Status Branch (`automation_node_executor.go:940-954`), the automation email gate (`:249`) and `SubscribeToLists` (`internal/service/list_service.go:353`);
  - `GetListsByEmail` (`contact_list_postgres.go:207`) and `GetContactsByListID` (`:170`);
  - `contacts.list` with its list filters and `with_contact_lists` (`internal/repository/contact_postgres.go:297-305`, `:428`), which is what the console's contact drawer loads;
  - list stats (`internal/repository/list_postgres.go:229`);
  - `MarkEmailsAsBounced` (`contact_postgres.go:1872`).
- **F3 — The broadcast audience doesn't.** `GetContactsForBroadcast` and `CountContactsForBroadcast` join `contact_lists` and filter `l.deleted_at`, but never `cl.deleted_at` (`contact_postgres.go:1437-1471`, `:1729-1756`).
  - Every broadcast has a list (`internal/domain/broadcast.go:399-400`), and its audience comes only from these two queries (`internal/service/broadcast/orchestrator.go:256`, `:324`).
  - Reproduced by the audit: after `removeContact`, a broadcast with `exclude_unsubscribed: true` counted 2 recipients, and both contacts received the email.
- **F4 — Neither does the list condition.** `parseContactListConditionsWithEmailRef` (`internal/service/query_builder.go:1210-1253`) builds `EXISTS (SELECT 1 FROM contact_lists cl JOIN lists l … AND l.deleted_at IS NULL)` with no `cl.deleted_at`. Segments (`BuildSQL`, `:423`) and automation trigger conditions (`BuildTriggerCondition`, `:1066`, which dispatches at `:1150`) share it.
  - Reproduced against a removed contact: a segment "in list X, status active" kept them; "not in list X" left them out; a trigger condition "in list X, active" enrolled them; a filter node with that condition took the continue path.
  - A List Status Branch sends the same contact to "not in list" (Code-confirmed).
- **F5 — Segments and triggers run SQL compiled earlier.** No startup routine repairs either.
  - **Segments.** Builds and per-contact updates run the stored `segments.generated_sql` (`internal/service/segment_build_processor.go:108-113`, `internal/service/contact_segment_queue_processor.go:321`).
    - Only four places write it: segment creation, an update that sends a tree (`internal/service/segment_service.go:128`, `:297`), and the v36 and v37 migrations (`internal/migrations/v36.go:287-311`, `v37.go:273-291`).
    - `segments.rebuild` reuses the stored SQL (`segment_service.go:424-471`).
  - **Automation triggers.** Their conditions are compiled into the installed function with their values inlined (`internal/service/automation_trigger_generator.go:234-246`, `embedArgs` at `:417`).
    - The function is regenerated on activation (`internal/service/automation_service.go:362`), on an update whose trigger inputs changed (`:231-235`), and by v36 and v38.
  - **Run-time compilation.** Branch and filter nodes compile when they run (`automation_node_executor.go:618`, `:740`), and so does segment preview (`segment_service.go:525`).
  - v36, v37 and v38 call the live builder, so on a database below 37 they compile the *fixed* SQL before v42 runs.
- **F6 — Segment membership is recomputed only when a timeline row is written.**
  - Every `contact_timeline` insert except segment events queues the contact (`internal/database/init.go:808-829`; trigger at `:950`).
  - The queue's primary key is `email` (`init.go:307-310`).
  - The processor claims rows older than 15 seconds, oldest first (`contact_segment_queue_processor.go:197-210`). It evaluates every active segment (`:107-121`) and removes the contact from each one it no longer matches (`:384-400`).
  - The queue task is permanent and recurring: about 50 seconds of batches, 100 ms apart (`internal/service/contact_segment_queue_task_processor.go:55-120`, `:176`).
- **F7 — Re-adding a contact with the same status writes no timeline row and sends no webhook.**
  - Only three statements ever clear `deleted_at` (`contact_list_postgres.go:41`, `:120`, `:245`): the `AddContactToList` and `BulkAddContactsToLists` upserts, and `UpdateContactListStatus`.
  - `track_contact_list_changes` handles an insert, a removal and a status change, and otherwise returns `NEW` (`init.go:635-697`; v23 installed the same body on existing databases, `internal/migrations/v23.go:45-110`).
  - `webhook_contact_lists_trigger` does the same (`internal/database/schema/webhook_triggers.go:143-165`). Its status branch also returns early for a change to `pending` (`:161-162`), so a re-add under double opt-in sends no webhook either.
  - Restore paths:
    - `lists.subscribe` and the public `/subscribe` (`list_service.go:353`, `:411`): the removed row reads as no row, then the upsert restores it;
    - `contacts.import` with list IDs (`BatchImportContacts`, `internal/service/contact_service.go:311`, `:450`);
    - the Add to List node (`automation_node_executor.go:806`);
    - the Supabase sync (`internal/service/supabase_service.go:368`);
    - an unsubscribe on a removed row, which `UnsubscribeFromLists` already turns into `unsubscribed` through `UpdateContactListStatus` (`list_service.go:580`).
  - `contactLists.updateStatus` doesn't restore: it answers `{"found":false}` for a removed row (`contact_list_service.go:178-188`). Reproduced.
  - Reproduced with the query-builder fix applied to a scratch copy:
    - after `removeContact`, the contact left the segment once the queue ran;
    - after `lists.subscribe`, the row was back (`active`, `deleted_at` null), but nothing was written to the timeline or queued, so the contact stayed out of "in list X".
  - **Fixing the query builder alone would keep every re-added contact out of their segments.**
- **F8 — Removed rows reach the API, the notification center and templates.** `fetchContact`, behind `GetContactByEmail` and `GetContactByExternalID` (`contact_postgres.go:130-171`), loads `contact_lists` filtering only `l.deleted_at`. That data reaches:
  - `contacts.getByEmail` and `contacts.getByExternalID` (`internal/http/contact_handler.go:130`, `:167`);
  - the `contacts.upsert` response (`contact_service.go:536`);
  - both copies in `/preferences` (`internal/service/notification_center_service.go:53`, `:80-82`);
  - Liquid's `contact.contact_lists`, because template data holds the whole contact (`internal/domain/template.go:1009`, `internal/domain/contact.go:1084`).

  Details:
  - The API contract says the field lists "the contact is subscribed to" (`openapi/components/schemas/contact.yaml:351-354`).
  - The widget treats a row as subscribed from `status` alone (`notification_center/src/App.tsx:260-263` at HEAD).
  - Nothing in the code reads `ContactList.DeletedAt` from these results.
  - Reproduced at HEAD: `/preferences` for a contact removed from a public list and a private list returned both rows as `active`, with `deleted_at` set.
  - B1's revised D6 ("leave only", 2026-10-16, uncommitted) already drops removed rows from both `/preferences` copies (`visibleContactLists` in `internal/service/notification_center_service.go`). On top of B1, the API, the upsert response and Liquid remain.
- **F9 — Fresh installs and upgrades share trigger bodies through `internal/database/schema`.**
  - `init.go` installs each `schema.Webhook*TriggerFunction()` (`init.go:957-973`), and `v39.go:177-181` loops over `schema.WebhookTriggerFunctions()`.
  - `track_contact_list_changes` is still inline in `init.go`.
  - `contact_lists.deleted_at` is in the base table (`init.go:149`), and that trigger already reads it on every workspace.
- **F10 — Release mechanics.**
  - v41 shipped (tag `v41.0`, 2026-09-17). The current unreleased work (41.1 so far) is released as v42 (P2), so B24's migration is v42, with `VERSION = "42.0"`. A shipped `vN.go` is never amended (`.claude/skills/create-migration/SKILL.md`).
  - **Every 42.0 build must contain `v42.go`.**
    - On a first run, the manager stamps a fresh install with the code's version (`internal/migrations/manager.go:130-132`), and it only runs migrations above the stamp (`:183`).
    - A build at 42.0 without `v42.go` would therefore stamp fresh installs 42, and they would never run it. That would happen with a dev build bumped early.
    - An upgraded database is stamped only after a migration runs (`:189`, `:211`), so it stays at 41 until the build that carries `v42.go`.
  - VERSION is compiled into the web analytics SDK bundle. A bump needs `make sdk-build` on Node 24, with `web_analytics_sdk/dist` and `package.json` committed (`Makefile:69-75`, `.github/workflows/web-analytics-sdk.yml`).
  - GHSA-88mf and GHSA-mvfg ship in v42 alongside B24 (P2). `plans/fix-automation-webhook-node-ssrf-plan.md` was updated to match on 2026-10-16; it had said 41.1.
- **F11 — Test harness.**
  - The task scheduler is off (`tests/testutil/server.go:116-118`), and `StartBackgroundWorkers` doesn't start the webhook delivery worker (`:373-388`). Tests therefore age `queued_at` and call `ProcessQueue` themselves (`segment_queue_processor_test.go`), and start the delivery worker as `webhook_worker_e2e_test.go:1148` does.
  - Workspace migrations run in one transaction per workspace (`internal/migrations/manager.go:269-289`). v42 needs one for its savepoints, so its test follows `v36_dot_notation_migration_test.go:84-90`; v37's test runs on the pool instead.
  - `factory.GetContactAutomation` errors when there is no enrollment; `FindContactAutomation` returns nil instead (`tests/testutil/factory.go:2533-2546`).
  - PostgreSQL folds the unquoted trigger function name to lowercase: id `NzVhKyBT5dcx3BC8EsQi9A` was installed as `automation_trigger_nzvhkybt5dcx3bc8esqi9a`.
- **F12 — Installed trigger functions can be patched in place.** Reproduced:
  - Take `pg_get_functiondef` of a pre-42 function and replace ` AND l.deleted_at IS NULL)` with ` AND cl.deleted_at IS NULL AND l.deleted_at IS NULL)`.
  - Running it as `CREATE OR REPLACE` gave a body byte-identical to what the fixed generator installs. The `pg_trigger` OID was unchanged, and the selection query matched the function before and not after.
  - The removed contact was no longer enrolled, and the member still was.

## Decisions

- **D1 — Full fix, with a v42 migration.** Pierre, 2026-10-15.
- **D2 — A removed contact is not a member, anywhere.** This is what F2's readers already do. Consequences:
  - "not in list X" matches them;
  - `exclude_unsubscribed: false` doesn't bring them back, since a removal isn't a status;
  - the API's `contact_lists`, Liquid's `contact.contact_lists` and the notification center stop listing the list. For a private list, D6 then refuses a rejoin by signature, as for anyone without a row.

  Automation emails keep reaching them as non-members by design (B1, D1); a List Status Branch or a "not in list" condition leaves them out.
- **D3 — The upgrade corrects memberships at once.** The migration re-queues the contacts whose result the fix changes. Their `segment.left` and `segment.joined` fire once, with the matching webhooks and segment-triggered automations. Pierre accepted this effect when choosing D1.
- **D4 — v42 patches the installed trigger functions in place (F12) rather than regenerating them.**
  - **Why not regenerate as v38 does.**
    - `DROP/CREATE TRIGGER` holds `ACCESS EXCLUSIVE` on `contact_timeline` until commit. The review reproduced `deadlock detected` between that and a concurrent write to a re-queued contact. When `DROP TRIGGER` lost, the migration still committed with that automation unrepaired, and it is never retried.
    - Regeneration also trusts `trigger_config`. That is why v38 must skip automations whose stored config lost `updated_fields`.
  - **What the in-place patch avoids.** It takes no table lock and reads no stored config, so it needs no skip.
  - **CLAUDE.md alignment.** CLAUDE.md names v38's pattern for column changes, where a condition may reference a column a workspace lacks. Here, the patched text is exactly the real generator's output. It only adds a column every workspace has (F9).
- **P1 — Decided (Pierre, 2026-10-16): re-adding a removed contact emits what an insert of that row would.**
  - **Timeline, same status:** the insert kind (`list.subscribed` for `active`, `list.pending`, `list.unsubscribed`, …), written with `operation = 'insert'` and the insert's `changes`.
    - The console then shows "subscribed … with status active" rather than "status changed → active" (`console/src/components/timeline/ContactTimeline.tsx:301-321`).
    - Nothing else reads `operation`.
  - **Webhook:** the insert kind, with `previous_status: null`, both for a same-status re-add and for a re-add moved to `pending` (F7).
  - **A re-add that also changes status otherwise keeps today's transition kind** (`list.resubscribed`, `list.confirmed`, `list.unsubscribed`). Status changes already win over removals (`webhook_triggers_test.go:313-345`), and `webhook_payload_samples_test.go:281-283` pins `list.resubscribed` for a returning contact.
  - **Consequences:**
    - an automation on `list.subscribed` with `frequency: every_time` enrolls the re-added contact again, bulk re-adds through `contacts.import` included; a `once` automation doesn't (reproduced by the review);
    - webhook consumers learn about re-adds.
  - **Rejected alternative:** a timeline-only kind such as `list.restored` that only re-queues the contact. It needs a new kind in `domain/automation.go:58-59` and `domain/webhook_subscription.go:162-169`, plus a timeline label.
- **P2 — Decided (Pierre, 2026-10-16): the current unreleased work (41.1 so far) is released as v42, and B24 is part of it, with the v42 migration.** 41.1 is never tagged.
  - **Advisories.** GHSA-88mf and GHSA-mvfg become "patched in 42.0". `plans/fix-automation-webhook-node-ssrf-plan.md` says so too, as of 2026-10-16.
  - **Order.** v42 is tagged once B24 has landed, since v42 contains this fix and every 42.0 build must contain `v42.go` (F10).
  - **Renaming.**
    - The CHANGELOG heading can become `## [42.0] - WIP` at any time.
    - `VERSION` moves to `42.0` in the commit that adds `v42.go`.

## The fix

### Part 1 — broadcast audience

`internal/repository/contact_postgres.go`, in `GetContactsForBroadcast` and `CountContactsForBroadcast`:
- Add `Where(sq.Eq{"cl.deleted_at": nil})` right after the `l.deleted_at` filter. It renders `cl.deleted_at IS NULL` without a placeholder, so the argument numbering doesn't move.
- It applies whatever `exclude_unsubscribed` says.
- If #440 Phase 1 lands first, the predicate goes into its `withBroadcastListFilter` instead.

### Part 2 — the list condition

`internal/service/query_builder.go`, `parseContactListConditionsWithEmailRef`: add `cl.deleted_at IS NULL` before `l.deleted_at IS NULL` in the `EXISTS` conditions.
- `not_in` negates the whole `EXISTS`, so "not in list X" then matches removed contacts.
- This reaches branch and filter nodes and segment preview at once, along with new or edited segments and newly installed triggers.
- Part 5 repairs the stored ones.

### Part 3 — re-adding a removed contact is an event (P1)

- **Timeline trigger.** New `internal/database/schema/contact_list_triggers.go` with `TrackContactListChangesFunction()`, which `init.go` uses instead of its inline copy, so fresh installs and upgrades run the same body.
  - The body is the v23 one plus a branch after the status-transition branch: `ELSIF OLD.deleted_at IS NOT NULL AND NEW.deleted_at IS NULL THEN`.
  - That branch sets `op := 'insert'`, the insert branch's kind for `NEW.status`, and the insert's `changes`, `{list_id: {new}, status: {new}}`.
  - Timeline conditions on `changes` therefore match a re-add exactly as they match a first subscription.
  - Transitions and removals are unchanged.
- **Webhook trigger.** In `schema.WebhookContactListsTriggerFunction()`:
  - `restored := OLD.deleted_at IS NOT NULL AND NEW.deleted_at IS NULL`, and a `prev_status` variable replaces the inline `CASE` in the payload.
  - The status branch gains `ELSIF restored THEN` (insert kind, `prev_status := NULL`) just before its `ELSE RETURN NEW`. This covers a re-add moved to `pending`.
  - After the `list.removed` branch, a same-status re-add gets the same treatment.
  - Every other update keeps `OLD.status`, and an insert keeps NULL.
- The new timeline row re-queues the contact (F6), which closes F7.

### Part 4 — the contact's memberships, as returned

`internal/repository/contact_postgres.go`, `fetchContact` (`:165-171`): add `Where(sq.Eq{"cl.deleted_at": nil})` to the list query. This fixes the API, the upsert response and Liquid. In `/preferences`, it makes B1's `visibleContactLists` filter on `DeletedAt` redundant. That filter stays, because it also hides private lists the contact left. Nothing changes in the widget.

### Part 5 — `internal/migrations/v42.go`

Workspace only (`HasSystemUpdate` false), no restart. It runs in the per-workspace migration transaction, and each step is idempotent. The order keeps every lock short and leaves nothing waiting on `contact_timeline`:

1. **`SET LOCAL statement_timeout = 0`**, as in v37. The loops can outlast a configured timeout, and a failed migration would fail again on every restart.
2. **Install both trigger functions.** Run `schema.TrackContactListChangesFunction()` and `schema.WebhookContactListsTriggerFunction()`, both `CREATE OR REPLACE`. The triggers stay attached (see `v39.go:165-176`).
3. **Patch the installed automation trigger functions in place (D4, F12).**
   - Select `p.oid, pg_get_functiondef(p.oid) FROM pg_proc p WHERE p.proname LIKE 'automation\_trigger\_%' AND p.prosrc LIKE '%FROM contact_lists cl JOIN lists l%' AND p.prosrc NOT LIKE '%cl.deleted_at IS NULL%'`.
   - Read every row and close the cursor before writing, since the migration shares one connection.
   - For each one, replace every ` AND l.deleted_at IS NULL)` with ` AND cl.deleted_at IS NULL AND l.deleted_at IS NULL)` and run the result, inside its own savepoint. On failure, roll back to the savepoint and go on.
   - This covers live automations, and the function an interrupted Pause can leave behind (HEAD `automation_service.go:414-451`); that one is harmless, because the enroll function refuses non-live automations.
   - Paused and draft automations get the new SQL from the generator when activated.
4. **Recompile segments**, using v37's pattern (`v37.go:226-297`).
   - Select `id, tree, generated_sql FROM segments WHERE status <> 'deleted'`.
   - Recompile with `BuildSQL` each segment whose `generated_sql` contains `FROM contact_lists cl JOIN lists l` and not `cl.deleted_at IS NULL`, then update `generated_sql` and `generated_args`. Skip a tree that doesn't parse or compile, and keep its SQL.
   - From **every** selected segment, recompiled or not, collect the `list_id` of each `contact_lists` leaf, with a private walker in `v42.go`. A segment that v36 or v37 already recompiled on an older database still needs its removed contacts re-queued (F5).
5. **Re-queue the affected contacts**, last, so the queue row locks are held only until commit. This step runs only when step 4 collected list IDs: `INSERT INTO contact_segment_queue (email, queued_at) SELECT DISTINCT cl.email, NOW() FROM contact_lists cl JOIN lists l ON l.id = cl.list_id WHERE cl.deleted_at IS NOT NULL AND l.deleted_at IS NULL AND cl.list_id = ANY($1) ON CONFLICT (email) DO UPDATE SET queued_at = EXCLUDED.queued_at`. Only these contacts' results change. A re-run queues them again, which is harmless.

### Part 6 — version, changelog, docs

- **Version.** `config/config.go`: `VERSION = "42.0"`, in the same commit as `v42.go` (F10). Then run `make sdk-build` on Node 24, and commit `web_analytics_sdk/dist` and `package.json` with it.
- **CHANGELOG.md.** Under `## [42.0] - WIP`, add these bullets after the entries already there. If the heading still reads `## [41.1] - WIP`, rename it first (P2). They fix behaviour that has shipped, so each one is a **Fix** (the re-add one ended up an **Improvement**; see Implementation). Drafts:
  > - **Fix**: A contact removed from a list is now out of it everywhere. They no longer receive its broadcasts, and they no longer count as a member in segments, automation trigger conditions, filter and branch nodes, the notification center, the API's `contact_lists` or a template's `contact.contact_lists`. "Not in list" conditions now match them. Upgrading corrects existing segments and live automations, so removed contacts leave those segments, and their `segment.left` and `segment.joined` events fire once.
  >
  > - **Fix**: Adding a removed contact back to a list now records `list.subscribed` (`list.pending` with double opt-in) on their timeline and as a webhook event, as a first subscription does. Before, the re-add recorded nothing. Automations triggered by `list.subscribed` with "every time" frequency now enroll a re-added contact again, including contacts re-added in bulk by an import.
- **`../docs`**, in a separate PR. `features/automations.mdx` and `features/webhooks.mdx` already have uncommitted edits from other sessions.
  - `features/automations.mdx:145`: the Remove from List node "removes the contact from a list". It reads "Unsubscribe contact from a list" today, which the node never did.
  - `features/lists.mdx`: add a short "Removing a contact" section. A removal isn't an unsubscribe, and it takes the contact out of the list for broadcasts, segments and conditions. Adding them back makes them a member again.
  - `features/automations.mdx:59` and `features/webhooks.mdx:53`: `list.subscribed` also fires when a removed contact is added back.

## Pre-42 fixtures (HEAD `c974080a`'s real output, printed by the probe)

- Segment "in list X, status active": `SELECT email FROM contacts WHERE EXISTS (SELECT 1 FROM contact_lists cl JOIN lists l ON cl.list_id = l.id WHERE cl.email = contacts.email AND cl.list_id = $1 AND cl.status = $2 AND l.deleted_at IS NULL)`
- Segment "not in list X": `SELECT email FROM contacts WHERE NOT EXISTS (SELECT 1 FROM contact_lists cl JOIN lists l ON cl.list_id = l.id WHERE cl.email = contacts.email AND cl.list_id = $1 AND l.deleted_at IS NULL)`
- Trigger guard: `IF (EXISTS (SELECT 1 FROM contact_lists cl JOIN lists l ON cl.list_id = l.id WHERE cl.email = NEW.email AND cl.list_id = '<list id>' AND cl.status = 'active' AND l.deleted_at IS NULL)) THEN`

After Part 2, removing ` AND cl.deleted_at IS NULL` from the new output gives these strings exactly (the review verified it). The migration test builds its pre-42 state that way and asserts equality with these strings, so the fixture stays tied to the real producer.

## Tests — write first; they must fail before the change

| Implementation | Test file | Cases |
|---|---|---|
| `internal/service/query_builder.go` | `internal/service/query_builder_test.go` | New `TestQueryBuilder_ContactListConditionIgnoresRemovedRows`. It runs `BuildSQL` and `BuildTriggerCondition` over `in` with a status, `in` without one, and `not_in`. Each `EXISTS` holds `cl.deleted_at IS NULL`, `not_in` wraps it in `NOT EXISTS`, and the args are unchanged. |
| `internal/service/automation_trigger_generator.go` (unchanged; consumes Part 2) | `internal/service/automation_trigger_generator_test.go` | Extend the `contact_lists` guard test (`:291-309`): `ConditionGuard` and `ValidationQuery` hold `cl.deleted_at IS NULL`. |
| `internal/repository/contact_postgres.go` (Parts 1 and 4) | `internal/repository/contact_postgres_test.go` | Every list-filtered expectation in `TestGetContactsForBroadcast` and `TestCountContactsForBroadcast` gains `AND cl\.deleted_at IS NULL` after `l\.deleted_at IS NULL`, with the same args. Every `fetchContact` list-query expectation (`:87`, `:170`, `:235`, `:314`, `:373`) gains the same filter. They all fail first. |
| `internal/database/schema/contact_list_triggers.go` (new) | `internal/database/schema/contact_list_triggers_test.go` (new) | The insert, removal and transition branches keep v23's kinds. The restore branch comes after the transition branch, maps every status as the insert branch does, sets `op := 'insert'`, and writes `changes` as `{key: {new}}`. |
| `internal/database/init.go` | `internal/database/init_test.go` | With `recordWorkspaceSchema` (`:353`), the workspace initializer issues exactly `schema.TrackContactListChangesFunction()`. |
| `internal/database/schema/webhook_triggers.go` | `internal/database/schema/webhook_triggers_test.go` | Replace the exact `previous_status` assertion at `:344`. New cases: a same-status re-add and a re-add to `pending` map to the insert kind with `prev_status := NULL`; every other update keeps `OLD.status`; existing kinds unchanged. |
| `internal/migrations/v42.go` (new) | `internal/migrations/v42_test.go` (new, sqlmock) | See the list below. |
| `config/config.go` | `make test-migrations` | Existing suite. |

`v42_test.go` cases:
- metadata (42.0, workspace only, no restart) and registration;
- `UpdateSystem` is a no-op;
- the statement order of Part 5;
- a selected function is run with the patched definition inside a savepoint;
- a failed patch rolls back to its savepoint, and the next function is still patched;
- no selected function means no DDL;
- a segment with old SQL is updated with SQL holding `cl.deleted_at IS NULL`, checked with a matcher like v37's `parameterizedSQL`;
- trees that don't parse or compile keep their SQL;
- list IDs come from every non-deleted segment, including one whose SQL is already new;
- the re-queue runs with exactly those IDs, and not at all when there are none.

### `tests/integration/removed_contact_list_membership_test.go` (new): `TestRemovedContactListMembership`

A fresh workspace, so the `init.go` path, driven through the public API. Links and headers come from delivered emails, and custom events go through `/api/customEvents.upsert`.

Setup:
- An SMTP provider pointing at Mailpit, `StartBackgroundWorkers`, and `SetWorkspaceCustomEndpointURL`.
- List X, a public list Y, and a private list Z.
- Contacts:
  - `member`, active on X;
  - `apiRemoved`, active on X, Y and Z, then removed from X and Z through `contactLists.removeContact`;
  - `nodeRemoved`, removed from X by a live automation's Remove from List node. Its event is fired, then the test waits until the row has `deleted_at`;
  - `never`, with no row.

Assertions on segments and enrollments name the contacts involved rather than comparing whole sets. Subtests:

1. **Broadcast.** Two broadcasts to X, one with `exclude_unsubscribed: true` and one with `false`, created through `broadcasts.create` and sent with `send_now`. Only `member` receives either one (Mailpit, by recipient), and each broadcast's recipient count is 1.
2. **Segments.** "in list X, active" and "not in list X", created through `segments.create` and awaited with `WaitForSegmentBuilt`. "in" holds `member` and neither removed contact. "not in" holds `apiRemoved`, `nodeRemoved` and `never`.
3. **A removal, then a re-add, moves the contact.** With both segments built:
   - Remove a fresh contact `toggled` through the API, age `queued_at`, and call `ProcessQueue`. They leave "in", join "not in", and their timeline has `segment.left` and `segment.joined`.
   - Re-add them through `lists.subscribe`. The timeline gains `list.subscribed` with operation `insert`, and after `ProcessQueue` they're back in "in".
   - Before the fix they never leave; with Part 2 alone, they never come back.
4. **Trigger condition.** A live automation on a custom event, with the condition "in list X, active". The event enrolls `member`. `FindContactAutomation` returns nil for `apiRemoved` and for `never`.
5. **Filter node.** `member` continues and `apiRemoved` exits; two Add to List nodes record the path, as in `testAutomationFilterNode`.
6. **Webhook.** The test reads the POSTs its receiver gets.
   - Setup: a subscription to `list.removed`, `list.subscribed` and `list.pending` on X and on a public double opt-in list D, through `createZapierSubscription` (`webhook_payload_samples_test.go:304`). It delivers to an `httptest` receiver; the suite sets `cfg.Webhook.AllowPrivateDeliveryHosts`, as `webhook_subscription_e2e_test.go:617` does, and the delivery worker is started as `webhook_worker_e2e_test.go:1148` does.
   - Removing a contact from X, then re-adding them, delivers `list.removed`, then `list.subscribed` with `previous_status: null`.
   - Re-adding a removed contact to D through the public `/subscribe` delivers `list.pending` with `previous_status: null`.
7. **Notification center.** Send a broadcast to Y and take the signed parameters from the `List-Unsubscribe` header delivered to `apiRemoved`, as `notification_center_private_lists_test.go` does. Then `GET /preferences` as the widget does (`notification_center/src/api/notification_center.ts:75-86`). Its `contact_lists`, top-level and inside `contact`, holds Y and neither X nor Z. X can still appear in `public_lists`.
8. **API and Liquid.**
   - `contacts.getByEmail` for `apiRemoved` lists only Y in `contact_lists`.
   - `transactional.send` of a template rendering `{% for l in contact.contact_lists %}[{{ l.list_id }}]{% endfor %}` to `apiRemoved`: the email read from Mailpit holds `[Y]`, and neither `[X]` nor `[Z]`.

### `tests/integration/v42_removed_list_members_migration_test.go` (new): `TestV42RemovedListMembersMigration`

Build the pre-42 state, then run `V42Migration.UpdateWorkspace` in a transaction and commit, as v36's test does.

Pre-42 state:
- `V23Migration.UpdateWorkspace` reinstalls the pre-42 `track_contact_list_changes`, which is the real old code. Its other two functions match `init.go`'s, so nothing else is downgraded.
- `webhook_contact_lists_trigger` is replaced by a stub (`RETURN NEW`), so the assertion that v42 installs it means something. A subscription to `list.subscribed` exists.
- Two segments ("in" and "not in") and a live automation whose trigger condition is "in list X, active" are created through the API. Their stored SQL and function body are then turned into the pre-42 fixtures (the function through `pg_get_functiondef`, as the probe did) and asserted equal to them. The `pg_trigger` OID is recorded.
- A third segment with a list condition on list W keeps its new SQL. That stands for one v36 or v37 already recompiled, and `wRemoved` is removed from W.
- A paused automation carries the same condition, and a segment has old SQL and a tree that parses but no longer compiles (a list condition without a list). Not an unparseable tree: an active segment whose tree doesn't parse makes `GetSegments` fail, which stops the segment queue for the whole workspace; the unit test covers that v42 skips one.
- `member` and `removed` are active on X. `removed` is removed through the API, and the queue is drained: they're still in "in", which is the old behaviour.

Assertions:
- the recompiled SQL holds the predicate and still executes;
- the queue holds `removed` and `wRemoved`, not `member`;
- after aging and `ProcessQueue`, `removed` has left "in" and joined "not in";
- the live function's body is byte-identical to a freshly generated one, its `pg_trigger` OID is unchanged, and the event enrolls `member` but not `removed`;
- the paused automation is still not armed;
- the segment whose tree no longer compiles keeps its SQL;
- a re-add through `lists.subscribe` writes `list.subscribed`, and queues a `list.subscribed` delivery with `previous_status` null;
- re-running the migration changes no SQL or function body, and memberships stay the same after `ProcessQueue`.

### Existing tests to run

- Integration, unchanged:
  - `TestAutomationTriggerConditions`, `TestAutomation` (list operations, List Status Branch, filter node);
  - `TestSegmentE2E`, `TestSegmentQueueProcessorIntegration`;
  - `TestWebhookWorkerEndToEnd`, `TestWebhookPayloadSamples`, `TestWebhookSubscriptionE2E`;
  - `TestContactDeletionErasure`, `TestListUnsubscribeHeaders`;
  - the broadcast suite (`TestBroadcast…`, which includes `TestBroadcastExcludesPendingDoubleOptIn_Integration` and `TestBroadcastAudienceStatusesAsDelivered`);
  - `TestAutomationUnsubscribeAsDelivered`, `TestNotificationCenterPrivateListsAsDelivered`, `TestUpdateContactPreferences_Integration`;
  - `TestV36DotNotationDataMigration` and the v37 migration tests.
- `TestBroadcastPartialFailure` is the known B26 flake.
- Nothing changes in the console or the widget.

## Rollout and upgrade effects

- **Segment backlog.** The re-queued rows carry the upgrade time, and the queue is processed oldest first in roughly 50-second runs (F6).
  - Until that backlog clears, segment updates for every other contact wait behind it, and so do segment-triggered automations.
  - On a workspace with many removals on lists that segments use, that can take minutes.
  - Segment counts then drop, and `segment.left` and `segment.joined` fire once for the corrected memberships (D3).
- **Broadcasts.** A broadcast that is sending during the upgrade skips removed contacts on its remaining pages. Its `total_recipients` was counted at the start and stays higher, so its progress stops short of 100% (`orchestrator.go:1104`). This happens once and needs no backfill.
- **Enrolled contacts.** Contacts already enrolled by a stale trigger condition continue their journey. Their later filter nodes use the new SQL.
- **Rolling restart.** A segment saved, or an automation activated, on the old instance after the migration commits keeps the old SQL until it is edited. v37 and v38 have the same exposure.
- **API change.** The API's `contact_lists` no longer includes removed memberships, which is what its schema describes. The timeline still records `list.removed`.

## Dependencies and order

1. **B1 is committed first.** It changes the same lines of both audience queries and their tests. D6 defines what the notification center shows, and the integration tests reuse its helpers (`deliveredLinkHref` lives in B1's uncommitted `automation_unsubscribe_delivered_test.go:389`).
2. **B24 lands before the v42 tag** (P2). v42 is the current unreleased work plus B24.
3. **#440 Phase 1 and B27 touch the same queries.** Whichever lands second keeps the other's predicates.

## Out of scope (found while planning)

- **`segments.rebuild` reuses the stored SQL.** Every query-builder fix therefore needs a migration to reach existing segments. Compiling from the tree on rebuild would let a rebuild repair a segment.
- **A removed opt-out can be bypassed.** `SubscribeToLists` reads a removed `unsubscribed` row as no row (`list_service.go:353-392`). On a list without double opt-in, the public form then re-subscribes a contact who had unsubscribed and was later removed, without the forced double opt-in.
- **B27 is not fixed here.** A `pending` row on a list without double opt-in is still mailed. Both audience queries keep `l.is_double_optin = false OR cl.status <> 'pending'`, and the automation send gate never skips `pending`. This plan only touches the same two queries; see the checklist for B27's own fix direction.
- *Resolved since:* a removed contact could come back to a private list by unsubscribing through an old link, then rejoining. B1's revised D6 ("leave only", 2026-10-16) closes this, because a signed request can no longer rejoin a private list the contact left.
- **`list.removed` renders as "status changed → undefined" in the console.** Its `changes` has no `status` (`init.go:663-668`, `ContactTimeline.tsx:280-284`).
- **v38's `changes ?` guard leaves stale triggers.** Automations it skipped keep a trigger narrower than their stored config. Pausing and re-activating one would widen it to every contact update; the repair is to restore `updated_fields` and save. v42 doesn't need this, because it patches functions in place (D4).

## Independent review (2026-10-15)

A separate reviewer re-checked every fact, searched the whole repo for other readers that ignore `cl.deleted_at`, and probed the design on the scratch stack. Changes made from it:
- **Step 5 deadlock.** The regenerate-and-re-queue design could deadlock with a concurrent write, losing a trigger repair silently. It is replaced by in-place patching (D4) and a re-queue that runs last.
- **Wrong rollout advice.** "Pause and re-activate" would have widened guard-skipped automations to every contact update. It is removed and moved to Out of scope with the correct repair.
- **Re-queue scope.** List IDs now come from every non-deleted segment, so upgrades from below 37 are covered. Deleted lists are excluded, and the backlog is documented.
- **Wider exposure.** F8 grew to the API, the upsert response, both `/preferences` copies and Liquid, so Part 4 moved to `fetchContact`.
- **Pending re-adds.** A re-add under double opt-in sent no webhook; it is now covered.
- **Tests.**
  - The exact `previous_status` assertion is replaced, and `init.go` gets a test.
  - The webhook subtest starts the delivery worker and uses `createZapierSubscription`.
  - The not-enrolled checks use `FindContactAutomation`, events go through the API, and assertions name contacts rather than whole sets.
  - The migration test's webhook check is no longer vacuous, and its re-run check now tolerates a re-queue.
- **Wording.** F7's restore paths, F9 and F11 are corrected, the docs line numbers are fixed, a stale out-of-scope docs item is dropped, `-timeout` is added, and `--exit-code` is dropped.

## Implementation (2026-10-16)

Built on `origin/dev` at `44880ee`, where B1 had landed, then merged with `origin/dev` at `65b8f54`. Every part above is done; the line numbers above are from `c974080a` and were re-located.

- **Part 1 and Part 4.** `cl.deleted_at IS NULL` follows `l.deleted_at IS NULL` in `GetContactsForBroadcast`, `CountContactsForBroadcast` and `fetchContact`.
- **Part 2.** `parseContactListConditionsWithEmailRef` emits `cl.deleted_at IS NULL AND l.deleted_at IS NULL`. v42 patches by inserting the first before the second, so the builder keeps them adjacent.
- **Part 3.** The timeline function lives in `internal/database/schema/contact_list_triggers.go`, and `init.go` installs it from there. The webhook function gained `restored` and `prev_status`. Found in review: removing a contact who was already removed rewrote `deleted_at` and recorded `list.removed` again, re-enrolling them in "every time" automations triggered by it. The timeline's removal branch now requires a row that had no `deleted_at`, as the webhook trigger already did, and v42 reinstalls it; `RemoveContactFromList` still moves `deleted_at`.
- **Part 5.** `internal/migrations/v42.go` follows the five steps. The automation functions are patched in place (D4), not regenerated as v38 does. A failed patch rolls back to its own savepoint. The re-queue inserts into `contact_segment_queue` in email order, as the web analytics re-queue does, so the two take the queue's row locks in the same order; a writer that queues contacts out of email order, such as an import, can still deadlock with it.
- **Part 6.** `VERSION = "42.0"`. The SDK bundle was rebuilt on Node 24, and only its version changed. `package-lock.json`'s root version moved with `package.json`, as it did for 41.1. The CHANGELOG heading is now `## [42.0] - WIP`, and the two drafted **Fix** bullets were added, then reworded in review, which also added a **Fix** for re-removal and a **Change** line for the v42 schema. The re-add bullet was later listed as an **Improvement** and took in the re-removal line.
- **Deviation: webhook assertions.** Both integration tests read the queued `webhook_deliveries` rows rather than starting the delivery worker and an `httptest` receiver. The fix is in what the trigger queues; delivery is unchanged.
- **Deviation: segments are patched in place, not recompiled (final review).** Recompiling from the tree, as Part 5 step 4 had it, did more than the fix:
  - It also applied every builder change made since the segment was saved, while only removed list members are re-queued, so the segment's other members keep results from the old SQL. For example, 39.0's `jsonb_typeof` guards on custom_json key checks have reached no segment saved before 39.0, because only v36 and v37 recompile.
  - It left the bug in a tree that no longer compiles.
  - Its read-then-write could overwrite a segment saved concurrently with SQL from the tree it had read.

  Step 4 is now a single `UPDATE segments SET generated_sql = replace(generated_sql, …)`, selected the same way as the function patch. `generated_args`, `version` and `status` are untouched, and the list IDs still come from every non-deleted segment's tree. `TestV42Migration_PatchTurnsPre42OutputIntoTheCurrentOutput` pins that the old ending occurs only at the end of a list condition, in both segments and trigger functions; the same battery was checked against `origin/dev`'s real builder. The integration test now asserts that the patched SQL is byte-identical to the fixed builder's, that a pre-39 custom_json segment gains only the list predicate, and that the uncompilable segment is patched too.
- **Moved in the changelog:** hiding removed lists in the notification center is credited to the private-lists bullet, not B24's. `visibleContactLists`, from the private-lists fix in the same release, already drops removed rows, so the integration subtest guards that behaviour end to end rather than testing this fix.
- **Build tag.** Both new integration files are built only under the `integration` tag, since they use `createZapierSubscription` from an integration-tagged file.
- **Broadcast paging (decided by Pierre on 2026-10-17, after the SQL review).** `GetContactsForBroadcast` repeats the cursor on `cl.email`. With `c.email > $cursor` alone, the planner merge-joined `contacts` with `contact_lists` through the latter's primary key from its first email, so every page read all the rows before the cursor. On a list of 1M contacts at 50 per page, a page near the end took about 490 ms on PostgreSQL 17, and 0.3 ms with both bounds. This predates the PR. Tests: `TestGetContactsForBroadcast` (sqlmock, the bound on both tables) and `tests/integration/broadcast_audience_pagination_test.go` (paging returns every recipient once, in email order, and nobody else).
- **Not done here:** the `../docs` edits, which belong in a separate PR in that repository.
- **Integration tests:** `TestRemovedContactListMembership`, `TestV42RemovedListMembersMigration`, `TestSignedLinkMembershipStateMatrix`, `TestBroadcastAudienceStatusesAsDelivered` and every `TestWebhook*` passed locally against Postgres 16 and Mailpit. CI runs the whole integration package with `-tags integration,licdev` on every push to this PR, and it passed at `abbca9f`. The whole integration package also passed locally on PostgreSQL 17.11 and 18.6, at #456's head `b7a5f1fb`, which carries this branch at `5203ffac`. Three of the new subtests could not pass as first written, and were fixed: a segment whose tree did not parse made `ProcessQueue` fail, a filter-node exit path added a contact to a second list, and the notification center subtest's broadcast had no template, so sending it failed with "variation not found".

## Probes

All of these ran on a scratch copy of HEAD with its own Postgres. Some have the Part 2 fix applied, as noted.
- `/private/tmp/claude-501/-Users-pierre-Sites-notifuse3-code-notifuse/84ed8eef-397b-4cc7-8f0f-3dbe5170ef12/scratchpad/b24/src/tests/integration/zz_probe_b24_query_builder_test.go`: F4 and F5, run at HEAD before the fix was applied.
- `…/zz_probe_b24_restore_test.go`: F7, with Part 2 applied.
- `…/zz_probe_b24_preferences_test.go`: F8.
- `…/zz_probe_b24_inplace_test.go`: F12.
- `…/zz_probe_b24_review_test.go`: the reviewer's probes (fixtures, LIKE selection, trigger behaviour, deadlock).

## Running the tests

Run this last, after every part above.

1. Unit: `make test-domain`, `make test-service`, `make test-repo`, `make test-http`, `make test-migrations`, plus `go test -race ./internal/database/...`.
2. SDK bundle after the bump: run `make sdk-build`, then check `git diff --stat -- web_analytics_sdk`: only the version changes.
3. Integration: start the stack with `docker compose -f tests/compose.test.yaml up -d`, then run:

   `INTEGRATION_TESTS=true go test -race -timeout 20m -tags integration,licdev -run 'TestRemovedContactListMembership|TestV42RemovedListMembersMigration|TestAutomationTriggerConditions|TestAutomation$|TestSegmentE2E|TestSegmentQueueProcessorIntegration|TestWebhookWorkerEndToEnd|TestWebhookPayloadSamples|TestWebhookSubscriptionE2E|TestContactDeletionErasure|TestListUnsubscribeHeaders|TestBroadcast|TestAutomationUnsubscribeAsDelivered|TestNotificationCenterPrivateListsAsDelivered|TestUpdateContactPreferences_Integration|TestV36DotNotationDataMigration|TestV37KindWidening' ./tests/integration/ -v`

   Without `INTEGRATION_TESTS=true`, every test skips and still prints `ok`.
