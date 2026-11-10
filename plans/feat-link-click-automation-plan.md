# Start an automation when a contact clicks a specific link — plan

Issue #498. Base: `dev` after the 43.0 release. Branches: `feat/link-click-automation` (notifuse),
`docs/link-click-automation` (docs), both into `dev`.

## Goal

1. Each time a contact clicks **a link they had not clicked yet in that email**, their timeline gets an
   `email.link_clicked` entry carrying the URL. That event can start an automation, filtered by link (contains),
   broadcast and template. It also queues the contact's segments for recomputation, which makes the existing
   "Click email" + link filter segment condition current as soon as the click happens.
2. From a broadcast's per-link statistics, open the list of emails (and so of contacts) on which that link was clicked.

## Schema impact

- No table, column, index or environment variable is created or changed.
- Workspace migration **v44** replaces the body of the PL/pgSQL function `track_message_history_changes()`. Its trigger
  (`message_history_changes_trigger`) is already attached and is left as is. `CREATE OR REPLACE FUNCTION` takes no table
  lock.
- New `contact_timeline.kind` value: `email.link_clicked` (a free `VARCHAR(150)`).
- The `clicked_links` JSONB gains a `verified` key per link. Not a schema change.
- No backfill (see Choices).

## Decisions

- **Forgeable click URLs.** `handleEncryptedClick` records the token's URL even when its signature does not verify, so a
  recipient can mint a click on any URL for their own message. Decision: record `verified: true` on a `clicked_links`
  entry clicked through a verified token, and write `email.link_clicked` only for a verified link. Per-link stats are
  unchanged; clicks in emails sent before 42.2 (unsigned links) start no per-link automation.
- **Event metering.** The monthly usage counter counts every non-web timeline entry. Decision: `email.link_clicked` is
  derived from a click that is already counted, so `monthlyUsageTimelineCount` excludes it
  (`AND kind <> 'email.link_clicked'`), without touching the partial index `idx_contact_timeline_billable`.

## Current state (checked in the code)

- `track_message_history_changes()` writes `email.clicked` only when `clicked_at` goes from NULL to a date: one event
  per email.
- `SetClicked` updates `clicked_at` (COALESCE) and `clicked_links` (key = clicked URL, `{count, first_at, last_at}`,
  capped at 50 keys) in the same UPDATE. A click with no URL (legacy `/visit`) only touches `clicked_at`.
- Segment recomputation: `contact_timeline_queue_trigger`, AFTER INSERT on `contact_timeline`. Automations: one trigger
  per automation on `contact_timeline`, with `WHEN NEW.kind = …` plus filters (`buildWHENClause`) and conditions
  evaluated in the function body.
- **The console rebuilds `trigger` from a fixed list of fields** (`buildTriggerConfig`, `hydrateTriggerNodeConfig` in
  `flowConverter.ts`): a field missing from that list is erased on every console save.
- The segment condition on `email.clicked` with `link_url` reads `message_history.clicked_links`, so it is right as soon
  as the contact is recomputed.
- v42 installed `schema.TrackMessageHistoryChangesFunction()` directly.
- Contact erasure anonymises `message_history` (`contact_email = 'DELETED_EMAIL'`, `clicked_links = NULL`), but
  `SetClicked` did not skip anonymised messages: a later click wrote the URL back.
- `message_history.automation_id` exists, which allows the loop guard.

## Choices

- **Name**: `email.link_clicked` (`NEW.channel || '.link_clicked'`). `email.clicked` keeps its meaning (the email's first
  click): counters, webhooks and existing automations do not change.
- **One entry per link newly verified for the email**, not per click: at most 50 per email.
- **`changes`**: `{"clicked_url": {"new": url}, "broadcast_id": {"new": …}, "template_id": {"new": …},
  "automation_id": {"new": …}}`, copied so `WHEN` filters without a subquery. `entity_id` = message id, `created_at` =
  `NEW.updated_at`. The two entries of a first click share a timestamp; their display order is not guaranteed.
- **Loop guard**: the `WHEN` of an `email.link_clicked` automation excludes clicks on emails that same automation sent
  (`COALESCE(NEW.changes->'automation_id'->>'new', '') <> <id>`). Otherwise, with `every_time`, each link of its own
  emails would start it again, up to 50 times per email, link scanners included.
- **No new segment condition**: once the event is written, the existing `email.clicked` + `link_url` condition is
  recomputed on each new link and also covers history.
- **No backfill**: inserting timeline entries in the migration would start automations and recomputation for old
  clicks, inside a single transaction.
- **No `email.link_clicked` webhook** (out of scope).
- **Freeze the v42 body** in `v42.go` as a constant, as v7, v8 and v36 keep theirs.
- **Clicks on an anonymised message**: `SetClicked` (and the open path) skip messages whose `contact_email` is
  `DELETED_EMAIL`; the redirect still works.
- **Log filter**: `clicked_url`, exact key match with the jsonb `?` operator (`??` to Squirrel) behind a
  `jsonb_typeof(clicked_links) = 'object'` guard. Not `clicked_links -> key IS NOT NULL`: PostgreSQL has no estimate for
  that expression and assumes nearly every row matches, so with the page LIMIT it walks the `created_at` index over the
  whole table for a little-clicked link; `?` has an estimate, and the broadcast index is used. `clicked_url` requires
  `broadcast_id`. The console passes the URL returned by the per-link stats, so the same string.

## Steps (TDD: tests first, then code)

### 1. PL/pgSQL function and migration

**`internal/database/schema/message_history_status_trigger.go`**: in the UPDATE branch, right after the `clicked_at`
block, a step introduced by the anchor comment `LinkClickedStepAnchor`. It inserts one `email.link_clicked` entry per
`clicked_links` key whose value is an object with `verified = 'true'` and whose OLD value was not verified, with
`jsonb_typeof` guards on NEW, OLD and the value, ordered by key.
- Test `message_history_status_trigger_test.go`: the step is present once, after the `clicked_at` block, with its guards.

**`internal/migrations/v42.go`**: constant `v42TrackMessageHistoryChangesFn` = the shipped body, used in place of the
shared function.
- Test `v42_test.go`: existing tests point at the constant; new test: the shared function minus the anchored step equals
  the constant.

**`internal/migrations/v44.go`** (new): `V44Migration` (44.0, workspace only) executes
`schema.TrackMessageHistoryChangesFunction()`. No trigger created or dropped, no column touched, so no automation
trigger function to regenerate.
- Test `v44_test.go` (sqlmock): version, flags, the single statement is exactly the shared function, execution error
  returned, `UpdateSystem` no-op. `manager_runwindow_test.go` follows the new latest version.

**`config/config.go`**: `VERSION = "44.0"`; `cd web_analytics_sdk && npm run build`, commit `dist/` and `package.json`.
Comments in `init.go`, `init_test.go`, `v36.go` that name the latest installer updated.

### 2. Domain

**`internal/domain/automation.go`**: `email.link_clicked` in `ValidEventKinds`; `TimelineTriggerConfig` gains
`LinkURL`, `BroadcastID`, `TemplateID` (`*string`, `omitempty`); `Validate` refuses them for any other event kind, and
refuses a NUL, surrounding whitespace, or a `link_url` longer than `MaxClickedURLLength`.
- Test `automation_test.go`: valid kind; each field accepted with `email.link_clicked`; refused with other kinds; too
  long, NUL, surrounding whitespace refused.

**`internal/domain/message_history.go`**: `MessageListParams.ClickedURL` (`clicked_url`), read by `FromQuery`;
`Validate`: max length, `broadcast_id` required. `SetClicked` gains `verified bool` in the repository interface.
- Test `message_history_test.go`: `FromQuery` reads `clicked_url`; `Validate` refuses too long and missing
  `broadcast_id`.

**`internal/domain/email_provider.go`**: `VisitLink(…, verified bool)`. Mocks updated.

### 3. Services, repository, HTTP

**`internal/service/automation_trigger_generator.go`**: `buildWHENClause` for `email.link_clicked` adds
`strpos(lower(NEW.changes->'clicked_url'->>'new'), lower(<lit>)) > 0`, `NEW.changes->'broadcast_id'->>'new' = <lit>`,
`NEW.changes->'template_id'->>'new' = <lit>`, and always the loop guard.
- Test `automation_trigger_generator_test.go`: WHEN for each filter and their combination; guard always present; quote,
  `%`, `_` in the URL.

**`internal/service/email_service.go`**: `VisitLink(…, verified)` passed on to `SetClicked`.
**`internal/service/demo_service.go`**: demo clicks are verified.
**`internal/http/email_handler.go`**: passes `verified` from `/r/`, and `false` from the legacy `/visit`.
- Tests `email_service_test.go`, `demo_service_test.go`, `email_handler_tracking_test.go`: the flag is passed as is; a
  forged, unsigned or legacy token passes `false`.
- Test `message_history_handler_test.go`: `clicked_url` passed through; 400 without `broadcast_id`.

**`internal/repository/message_history_postgre.go`**: `SetClicked` sets `verified: true` on the entry (new or existing)
when verified and never turns it back to false; both click and open UPDATEs skip `DELETED_EMAIL`. `ListMessages`:
`clicked_url` filter.
- Tests `message_history_postgre_test.go`, `message_history_tracking_consent_test.go` (sqlmock): queries and arguments.

**`internal/repository/web_analytics_postgres.go`**: `AND kind <> 'email.link_clicked'` in `monthlyUsageTimelineCount`.
- Test `web_analytics_postgres_test.go`: query text.

### 4. Console

- `services/api/automation.ts`: kind and `TimelineTriggerConfig` fields.
- `components/automations/utils/flowConverter.ts`: carries `link_url`, `broadcast_id`, `template_id` both ways.
  Test `flowConverter.test.ts`: lossless round trip.
- `components/automations/config/TriggerConfigForm.tsx`: "Clicked a link" option, "Clicked link contains", broadcast and
  template fields, cleared on kind change. Test `TriggerConfigForm.test.tsx`.
- `components/automations/nodes/TriggerNode.tsx`, `AutomationCard.tsx`: readable label and link filter. Test
  `TriggerNode.test.tsx`.
- `components/timeline/ContactTimeline.tsx`: "clicked a link" entry with the URL. Test `ContactTimeline.test.tsx`.
- `services/api/messages_history.ts`, `components/messages/MessageHistoryTab.tsx`: `clicked_url` read from the URL,
  offered only with a broadcast filter, and cleared with it. Test `MessageHistoryTab.test.tsx`.
- `components/broadcasts/BroadcastLinkStats.tsx`: "See who clicked" per row, opening the log with `broadcast_id`,
  `clicked_url` and, for a variation, `template_id`, encoded with `URLSearchParams`. Test `BroadcastLinkStats.test.tsx`.
- `npm run lingui:extract`, translations in every locale.

### 5. Integration tests (`tests/integration/`, real database)

- `timeline_link_clicked_test.go`: first verified click; repeat click and a second link, with the segment queue and a
  segment on the link clicked second; unsigned then signed click; click with no URL; the 50-link cap; malformed
  `clicked_links`; an update that does not touch `clicked_links`; a click on an erased contact's email; the
  `messages.list` round trip with the per-link stats; the usage counter; v44 upgrading a body without the step.
- `automation_link_click_trigger_test.go`: an `email.link_clicked` automation with `link_url` and `broadcast_id` enrols
  the contact when they click that link second and `add_to_list` adds them; another link or broadcast does nothing; a
  click on an email the automation sent does not restart it; a URL with `'`, `%`, `_`; an update keeps the filters.

### 6. Docs (`docs` repository, separate PR into `dev`)

`features/automations.mdx`, `features/email-tracking.mdx`, `features/contacts.mdx`, `features/logs.mdx`,
`features/broadcast-campaigns.mdx`.

### 7. CHANGELOG

`## [44.0] - unreleased`: the trigger, segments current on each new link, who clicked each link, clicks and opens on an
erased contact's email no longer recorded.

### 8. Checks

```bash
make test-domain test-service test-repo test-http test-migrations test-database
make test-integration
cd console && npm test && npm run lint && npm run build
cd web_analytics_sdk && npm run build
```

## Known, not handled

- No `email.link_clicked` webhook.
- Clicks before 44.0 are not replayed into the timeline.
- Stored URLs keep their UTM parameters; the trigger's "contains" filter copes with that, the log filter takes the exact
  key from the per-link stats.
- A click on a new link now waits for the contact's `contact_segment_queue` row lock before the redirect, as the first
  click already does.
- A link clicked before 44.0 has no `verified` key, so its first signed click after the upgrade writes its entry
  (documented).
- The monthly meter reads `kind` from the table for each row of the month: the partial index no longer answers alone.
