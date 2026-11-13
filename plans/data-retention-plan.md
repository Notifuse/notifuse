# Data retention plan

**Status**: draft, reviewed three times. Open decisions all settled on 2026-11-13. Implementation is planned for the week of 2026-11-16. Nothing is implemented.
**Date**: 2026-11-13
**Target**: needs a system + workspace migration.
- On `origin/dev` (b626344e), `v44.go` already exists for other work (click timeline entries, recent-failure indexes) and `VERSION` is 44.0, with 44.0 unreleased. So:
  - this plan's migration is **`v45.go`**, with `VERSION` 45.0 in the commit that adds it;
  - or its objects are folded into `v44.go` if they ship in 44.0, which is allowed while 44.0 is unreleased.
- Every mention of "v44" below means "this plan's migration".
- **Line numbers** were checked against `f4c3e472` (branch `chore/gomjml-notifuse-module`). `origin/dev` has moved since, for example `list_service.go` by about +12 lines after line 105, so re-check them before implementing.
**Licence**: new Enterprise feature `data_retention`, the 7th licensed capability. It is already advertised as "coming soon" on the Enterprise tier (`homepage/src/data/licensing.ts`, `comingSoon`).

## Goal

A workspace can set how long each category of personal data is kept, and Notifuse enforces it: every day it deletes or anonymises what is past its period, shows beforehand what a policy will remove, waits 7 days before a new or shortened policy takes effect, and keeps a report of every run. This is the storage-limitation tool a DPO asks for (GDPR art. 5(1)(e), 5(2), 25(2), 30(1)(f)); the inactive-contact rule follows the CNIL's 3-year prospect guidance.

## Decisions (Pierre, 2026-11-13)

- **D1 — Scope of v1 is every category**: inactive contacts, message history, contact timeline, custom events, web analytics, technical logs. Each comes with a preview and a run report.
- **D2 — A deleted contact's opt-outs survive as hashes, for 3 years** (Pierre, 2026-11-13; this replaces the minimal record of the first draft and reverses the October decision that `contacts.delete` is a pure delete).
  - When a contact is deleted, by the inactivity purge or by `contacts.delete`, the contact row and everything else go. Only a keyed hash of the address is kept, with its `unsubscribed`, `bounced` and `complained` statuses, list by list.
  - An unsubscribe never deletes anything: a contact unsubscribed from one list keeps receiving its other lists, its automations and its transactional emails.
  - Manual deletion offers an explicit "also forget their unsubscribes" option (API field and console checkbox, off by default) for test data or an explicit request that nothing remain. The purge has no such option.
  - **When the address comes back** (import, API upsert, form), it becomes an ordinary contact again and its old opt-outs are re-applied on those lists (D11).
  - **The hashes are deleted 3 years after they were recorded** (fixed). After that, a re-import has no memory of the opt-outs.
  - Why: the CNIL asks for an opposition list kept at least 3 years with only the data needed and suggests keeping hashes, which remain personal data. Without the hashes, a delete-then-reimport re-subscribes people who objected and re-mails hard bounces and complainers, which defeats the rule that broadcasts never mail them. The same pattern exists elsewhere: Iterable `forget`, Customer.io delete-and-suppress, Segment `SUPPRESS_WITH_DELETE`.
  - The mechanism is part of deletion, so it is free (Lot 0); the Enterprise purge reuses it.
- **D3 — Licence boundary: everything is Enterprise**, web-analytics retention included. The gate is on *configuring* a policy. The purge worker holds no licence provider. A policy saved while licensed keeps running after the key lapses. Disabling a category is always allowed. The fix to the leftovers of `contacts.delete` (Lot 0) is free: it is an art. 17 gap, and licensing never blocks a deletion.
- **D4 — The cloud plan quota (`PLAN_DATA_RETENTION_MONTHS`) is not enforced by this engine.** Instead, the cloud pricing FAQ, which says "data is permanently deleted", is corrected (Lot 0b).
- **D5 — The engagement clock is a fixed list, CNIL-aligned, and only *interactive* events are on it** (Pierre: "sending an email is not an interactive event"). An interactive event is one the contact performs. Something the workspace or Notifuse does to the contact never counts. The clock starts at collection, `LEAST(contacts.created_at, contacts.db_created_at)`, and only the events in the table below move it forward. `created_at` alone is not a collection date: the bulk upsert overwrites it with the import's value, which defaults to now (`contact_postgres.go:1471`, `:75-79`). A nightly CRM sync would make every synced contact immortal, the KASPR pattern. `db_created_at` is set by the database on insert and never rewritten.
  - CNIL référentiel; Spartoo SAN-2020-003; CALOGA SAN-2025-002: an email open is not a contact from the prospect.
  - KASPR, 5 December 2024: renewing retention on every profile update is disproportionate.
- **D10 — Custom events count only when the workspace designates them** (Pierre, O7). The retention policy holds `engagement_event_names`, empty by default: the event names that are actions of the contact (`purchase`, `login`…). Events from a CRM sync or a billing system stay off the list unless the workspace puts them there.
  - The custom-event part of the clock is **read at purge and preview time**: `MAX(occurred_at)` over `custom_events` whose name is designated (`idx_custom_events_email`). It is not folded into `contact_engagement`. A single `GREATEST` column could not honour a name added after the backfill (past events would never count) or a name removed (past touches could not be unwound).
  - The custom-events category could still delete a designated event while the contact's own period runs. So, before deleting designated events, that category folds their `MAX(occurred_at)` per contact into `contact_engagement`. A fold cannot be unwound: removing a name later does not shorten the clock of contacts whose events were already folded. This is accepted and documented: it errs on the side of keeping, and only concerns events older than the custom-events period.
- **D11 — Returned opt-outs are ordinary opt-outs.** The restored row is an ordinary `unsubscribed`, `bounced` or `complained` row, exactly like the opt-out of a contact that was never deleted, and the hash is deleted once restored. Review 2 showed that a separate rule for returned addresses creates two classes of opt-out. D12 gives the one rule for all of them.
- **D12 — Nobody re-subscribes an unsubscribed contact implicitly; anyone authenticated can do it explicitly** (Pierre, O10, 2026-11-13).
  - **Default.** `lists.subscribe` (API key or console), `contactLists.updateStatus`, imports and the automation add-to-list node leave an `unsubscribed` row unsubscribed. The API and console answers report `unsubscribed` for that list. No email is sent: this replaces today's forced double opt-in for an unsubscribed contact, which silently leaves the contact `pending` forever when the list has no double opt-in template (`list_service.go:429, 444, 539`).
  - **Explicit.** With `resubscribe: true`, any authenticated caller, an API key relaying a re-subscription from the customer's own system or a console user, sets the row `active` directly, **skipping the double opt-in** (Pierre).
  - **Audit.** `lists.subscribe` and `contactLists.updateStatus` are excluded data-plane routes today (`domain/audit_actions.go:234-242`). They move to `AuditedActions`, and the service calls `AuditRecord.Skip()` (`domain/audit_log.go:344`) unless a row actually moved out of `unsubscribed`. So the entry records the effect, not the flag, and a caller that always sends the flag does not flood the log.
    - The `RequireExport` mechanism cannot serve here: it applies to GET query strings only, and the body peek reads no booleans (`middleware/audit.go:111-127, 261-273`).
    - Audit recording is Enterprise; Community keeps the timeline's `list.resubscribed` row, which does not say who did it (see O12).
  - **The contact** re-subscribes through the form or the notification centre, with the double opt-in as today.
  - **Imports and automations never** re-subscribe: they carry no flag.
  - **`bounced` and `complained` stay terminal for the API** (`IsTerminalContactListStatus`, `domain/contact_list.go:33`): the API flag applies to `unsubscribed` only. The console's explicit "Re-subscribe" action may also clear `bounced`, never `complained` (O13).
  - **The contact's own public submission** on a list with no double opt-in template sets `active` directly (O11).
  - **Enforcement.** The same guard in the two repository writers:
    - `AddContactToList` gets the bulk path's `WHERE contact_lists.status NOT IN ('unsubscribed','bounced','complained')` (`contact_list_postgres.go:42` vs `:121`);
    - `UpdateContactListStatus` refuses any change out of those three statuses except to another opt-out status (`:270-273`).

    The legitimate exits go through a separate repository method: the contact's own path, and the flag.
  - Other callers of `AddContactToList` are audited for the guard: `supabase_service.go:368` (an upsert path, now refused silently for an opt-out, which is the intended outcome; it gets a log line) and the automation node.
  - **`resubscribe` is ignored on the public route**, which shares `SubscribeToListsRequest` (`public_handler.go:170`). The service reads back the stored row to report the real status, so no caller is told `pending` or `active` for a write the guard refused.
  - This closes the add-to-list node hole (`automation_node_executor.go:822-830`), whose execution output now records the refusal.
  - **Limit.** An integrator can set the flag on every call; that is then the controller's own, visible decision.
  - **Changelog.** The default is a behaviour change for integrations (no double opt-in email any more without the flag) and needs its own line.
- **D8 — `contact.deleted` during a retention purge carries the email only** (Pierre, O1). Subscribers get the signal to propagate the deletion. They do not get a copy of the data being erased. Manual deletion keeps today's full payload.
- **D9 — Commitment wording** (Pierre, O2). On the self-hosted commitments page, "The application never … deletes a row" becomes "never deletes a row on its own — only a retention policy you configure does". This is a clarification published with the release, with no 90-day notice: nothing is deleted without the customer's decision.
- **D6 — Who configures**: root sets a deployment default per category; the workspace owner overrides it per workspace. Same shape as the audit-log retention.
- **D7 — Safety**: a preview with counts per category before saving. Any category that is created or shortened waits 7 days before its first purge, during which it can be cancelled; a banner shows in the console and the owners receive an email. After that the purge runs daily and writes a report for every run.

### Engagement events (D5)

| Source | Event | Resets the clock? | Why |
|---|---|---|---|
| Tracked link | `email.clicked` (`EmailService.VisitLink`) | **Yes**, on every click (not only the first, which is all `SetClicked` records: `WHERE clicked_at IS NULL`). Two exceptions: a click on the unsubscribe or preferences link, and a contact whose click tracking is off or refused. | A click is the CNIL's own example. An unsubscribe is an opposition, not interest. Those links do go through `VisitLink`: `TrackLinks` only leaves `action=tracking` and `action=tracking_optout` unwrapped (`pkg/notifuse_mjml/template_compilation.go:225-254`), so `VisitLink` must recognise the notification-centre destination. Deriving engagement from a click the contact refused to have tracked would be click tracking by another name. |
| Reply | `email.replied` | **Yes** | The contact wrote to the sender |
| Reply | `email.auto_reply` | No | Sent by a mail server, not the person |
| Double opt-in | `list.confirmed` | **Yes** | The contact clicked the confirmation |
| Subscription | `list.subscribed` / `list.resubscribed` through the public endpoint (`SubscribeToLists` with `!hasBearerToken`, form or notification centre with `email_hmac`) | **Yes, only when the resulting row is `active`.** A `pending` row (double opt-in) is not engagement; its confirmation is. | The contact asked to subscribe. Without an HMAC, anyone can post an address to a single-opt-in public list: accepted, like the form it is. |
| Subscription | `list.subscribed` / `list.pending` / `list.resubscribed` through the API, an import or the console | No | Done by the workspace |
| Notification centre | `UpdateContactPreferences` (language, timezone, tracking consent) | No | It changes no subscription (`notification_center_service.go:124-170`); the centre's subscribe and unsubscribe go through `/subscribe` and `/unsubscribe`, i.e. the rows above. |
| Web analytics | A session identified to the contact | **Yes** | The person visited, logged in on the customer's site. Hooked where identified sessions are projected to the timeline (`WebAnalyticsBuffer.projectNavigation` → `ProjectContactNavigation`, `web_analytics_timeline_projection.go:260`). The contact bridge only carries goals. |
| Web analytics | An anonymous session | No | Not linked to a contact |
| Email lifecycle | `email.sent`, `email.delivered`, `email.opened`, `email.bounced`, `email.complained`, `email.unsubscribed`, `email.updated` | No | Sent by the workspace, an open is not a contact (CNIL), or an opposition or failure |
| Transactional | Any transactional send | No | Sent by the workspace |
| Contact | `contact.created` | Starts the clock | Collection |
| Contact | `contact.updated`, upsert, import, console edit | No | KASPR |
| Segments and automations | `segment.joined`, `segment.left`, `automation.start`, node executions | No | Computed by Notifuse |
| Custom events | `custom_event.created` | **Yes, only for the names in `engagement_event_names` (D10)** | A custom event can be a contact action (`purchase`, `login`) or a system fact (`invoice_generated`, `subscription_renewed`, a CRM sync). Read live at purge and preview time (D10): `MAX(LEAST(occurred_at, now()))` over designated names, excluding soft-deleted rows (`deleted_at`), so a future date or a soft delete through `Upsert` cannot pin the clock. No `Touch` call: every ingestion path (`Upsert`, `BatchImport`, the bridge's `EmitGoals`) is covered by construction. |
| Custom events | `custom_event.updated`, `custom_event.deleted` | No | Edits, not actions |

## Decisions taken by this plan (open to review)

- **P1 — Engagement lives in a new table, not on `contacts`.** `contact_engagement (email PK, last_engaged_at)`. A row exists only once something happened. The clock is `GREATEST(COALESCE(e.last_engaged_at, '-infinity'), LEAST(c.created_at, c.db_created_at), <designated custom events>)`, so contacts created by any path, including bulk paths that bypass `ContactService`, need no row at creation. The row is deleted with the contact (Lot 0), otherwise a contact recreated later would inherit the stale clock. Updating `contacts` on every click would fire the `contact.updated` webhook and, for tracked columns, write timeline rows; a separate table touches neither, and leaves the automation trigger functions alone (CLAUDE.md: altering `contacts` columns is the risky path).
- **P2 — Engagement is recorded in services, not triggers.** A trigger on `contact_lists` cannot tell a contact subscribing through a form from an import; the service can (`SubscribeToLists(..., hasBearerToken)`). One small port, `EngagementRecorder`, is called from each entry point. A test lists the entry points.
- **P3a — Dev's v44 changes the click picture.** `track_message_history_changes` now writes an `email.link_clicked` timeline entry, with the URL, for each link first recorded through a verified token (`origin/dev:internal/migrations/v44.go`). Two consequences:
  - The backfill reads those entries for clicks after the 44.0 upgrade, and **excludes unsubscribe and notification-centre URLs by URL**. `message_history.clicked_at` only serves for older clicks, where it cannot tell those links apart.
  - The P5 guard (`DELETED_EMAIL`) and any other change to `track_message_history_changes` must start from dev's v44 body, through `schema.TrackMessageHistoryChangesFunction`, never from the 43.x body.
- **P3 — The backfill counts only what history proves was interactive.** That means clicks (`message_history.clicked_at`, minus unsubscribe and preference URLs when `clicked_links` tells them apart), `email.replied`, `list.confirmed`, identified web sessions, and custom events whose name is in `engagement_event_names`. A past `list.subscribed` or `list.resubscribed` is not counted, because history does not record whether the contact or the workspace made it.
  - The resulting under-count is small: the clock starts at collection (D5), and a form subscription usually coincides with the contact's creation.
  - The preview and the 7-day delay catch the rest.
- **P4 — Policies are opt-in.** No personal-data category is on by default (the two technical logs keep today's 7-day sweep), on new or existing workspaces. Art. 25(2) argues for on-by-default, but turning on a deletion by default on an upgrade is the one failure this feature cannot have. The console suggests CNIL values when a category is switched on.
- **P5 — `message_history` is redacted, never deleted.** Broadcast stats are computed from these rows at read time (`GetBroadcastStats`, `message_history_postgre.go:1043`). The row keeps its ids, its timestamps and its broadcast/automation links; the address, `message_data`, `clicked_links`, `channel_options`, `status_info` (bounce and send errors quote the recipient) and the `attachments` references are wiped. This is redaction, not anonymisation in the WP216 sense: `external_id`, `smtp_message_id` and the timeline's `entity_id` (the message id) still link a row to a person while the timeline keeps it. The console says so, and warns when the timeline period is longer than the message-history period.
  - **`track_message_history_changes` writes `OLD.status_info` into a new `email.updated` timeline row** whenever that column changes (`message_history_status_trigger.go:183-190`). Without a guard, the redaction itself, manual or purge, would copy the bounce text into the timeline under `DELETED_EMAIL`. v44 replaces the function so an UPDATE whose `NEW.contact_email = 'DELETED_EMAIL'` returns early.
- **P6 — Web analytics expires by whole month**, by dropping partitions. That is instant, returns the disk, and cuts the relation count that slows `pg_upgrade` (plans/postgresql-18-upgrade-plan.md:372). Closed months in `monthly_usage` are already protected by `GREATEST`.

## Current behaviour

- **No general retention.** Three things are purged by age: audit logs (`AuditLogPurgeWorker`, configurable), webhook deliveries (7 days, hardcoded, `webhook_delivery_worker.go:263`), terminally failed email-queue rows (7 days, hardcoded, `queue/worker.go:81`).
- **Web analytics** never drops a partition (`web_analytics_maintenance_worker.go:21-22`); the docs say "There is no retention setting yet".
- **`PLAN_DATA_RETENTION_MONTHS`** is read only by the startup log (`internal/app/app.go:2274`).
- **No last-engagement column** anywhere.
- **No suppression table.** Opt-outs exist only as `contact_lists` rows, and `contacts.delete` removes them: a later import re-subscribes the address (the docs carry a warning to that effect).
- **`AddContactToList` protects only `bounced` and `complained`** (`contact_list_postgres.go:42`): an API `lists.subscribe` turns `unsubscribed` into `pending` and sends a double opt-in email. The bulk path also protects `unsubscribed` (`:121`).
- **`contacts.delete` leaves personal data behind** (`ContactService.DeleteContact`, `contact_service.go:163-309`):
  - `contact_automations` (and its `automation_node_executions` through the cascade) and `automation_trigger_log`;
  - in `message_history`, `channel_options` (cc, bcc, reply_to, subject) and the `attachments` references;
  - `message_attachments` rows, which nothing ever deletes.
- **Deletion side effects to keep in mind**:
  - deleting `contact_segments` rows fires a `segment.left` timeline row and a webhook;
  - deleting `contacts` fires a `contact.deleted` webhook carrying the full row;
  - an UPDATE of `contacts` writes a `contact.updated` timeline row with the old values of tracked columns, and a webhook with the full row.
- **Functional lookups on old rows**: `WasSentForList`, `GetRecipientAndList`, `GetBySMTPMessageID`, `GetForWebVersion`, transactional `external_id` idempotency (all on `message_history`), consecutive soft-bounce counting (`inbound_webhook_events`), "once" automation dedup (`automation_trigger_log`). These set the minimum periods below.

## Categories, clocks, actions and bounds

| Category | Clock | Action | Min | Suggested in UI |
|---|---|---|---|---|
| Inactive contacts | the D5 clock: collection `LEAST(created_at, db_created_at)`, `contact_engagement.last_engaged_at`, designated custom events read live (D10) | Full delete through the batch form of Lot 0's eraser, opt-out hashes included (D2). Contacts on an exempt list or segment are skipped. | 6 months | 36 months (CNIL prospects) |
| Message history | `created_at` (the indexed `idx_message_history_created_at_id`; there is no index on `sent_at`) | Redact (P5). Walk a keyset cursor on `(created_at, id)` stored in `data_retention_state`, and skip rows already redacted (`contact_email <> 'DELETED_EMAIL'`). Without the cursor, each batch would re-walk every redacted row from the oldest end of the index. | 90 days (bounce/complaint attribution, web version, reply matching) | 36 months |
| Contact timeline | `created_at` (the partial `idx_contact_timeline_billable` excludes web rows). A plain index is built `CONCURRENTLY` by the worker the first time the category is enabled on a workspace, not in the migration: a non-concurrent build on the hottest table would block writes, and every workspace would pay it for an opt-in feature. | Delete | 90 days | 36 months |
| Custom events | `occurred_at` (and soft-deleted rows by `deleted_at`) | Delete | 30 days | 36 months |
| Web analytics | partition month (`session_date`) | `DROP` the `web_sessions/pages/goals` partitions of that month, with a short `lock_timeout` and a retry the next day: the drop takes ACCESS EXCLUSIVE on the parent and would queue behind a dashboard read, stalling ingestion behind it | 1 month | 25 months (CNIL audience measurement) |
| Inbound webhook events | `timestamp` | Delete | 30 days (soft-bounce counting) | 12 months (CNIL logs) |
| Webhook deliveries | `created_at` | Delete (replaces the hardcoded 7 days, which stays the default: the one category that is on by default, because it already is today, P4) | 7 days | 7 days |
| Email queue, failed rows | `processed_at` | Delete (replaces the hardcoded 7 days, which stays the default, like webhook deliveries) | 7 days | 7 days |
| Automation history | `contact_automations` finished (`completed`/`exited`/`failed`), by `entered_at` | Delete (cascades node executions). Never `automation_trigger_log` (deleting it re-enrols contacts in "once" automations); it goes with the contact. | 90 days | 36 months |

A value of 0 means the category is off. The maximum is 10 years. Audit logs keep their own existing setting; the retention page links to it so a DPO sees the whole picture in one place.

**Exemptions** (inactive contacts only): a set of list ids and segment ids whose members are never purged as inactive, e.g. customers under contract or a legal hold. If an exempt segment's computation fails, the run skips the contact category rather than purging without the exemption.

**Opt-out hashes (D2, D11)**:
- **Table** `contact_opt_out_hashes (email_hash BYTEA, list_id VARCHAR(32), status VARCHAR(20) CHECK (status IN ('unsubscribed','bounced','complained')), opted_out_at TIMESTAMPTZ, recorded_at TIMESTAMPTZ NOT NULL, source VARCHAR(20) CHECK (source IN ('delete','retention')), PRIMARY KEY (email_hash, list_id))`, indexed on `recorded_at` for the 3-year expiry. It holds no address and no other field.
- **Key.** A per-workspace random key, 32 bytes, in a one-row table `contact_opt_out_hash_key`.
  - It is generated **in SQL, by the shared schema**: `uuid_send(gen_random_uuid()) || uuid_send(gen_random_uuid())`, with `ON CONFLICT DO NOTHING`. `gen_random_uuid()` is core PostgreSQL since 13, so there is no pgcrypto, the v30 constraint (`v30.go:78`).
  - One generator therefore covers new workspaces, migrated ones and `repairWorkspace`, which builds a schema through the initializer only (`migrations/manager.go:755`).
  - The hash lives in one SQL function, `contact_opt_out_hash(email)`, which raises when the key is missing rather than returning NULL.
  - **Hash.** `sha256(key || convert_to(lower(btrim(email) COLLATE "C"), 'UTF8'))`, with PostgreSQL's built-in `sha256(bytea)`. It is computed **only in SQL, never in Go**, because Go's `ToLower` and PostgreSQL's `lower()` can differ. The `"C"` collation keeps the hash independent of the database's collation, so a `pg_upgrade` or a restore elsewhere cannot silently change the hash of a non-ASCII address. (With "C", only ASCII letters are lowered; that is enough when it is applied identically on write and on lookup.)
    - The key prevents matching across workspaces. It does not protect against someone holding the database, since it sits in the same database and its backups. The CNIL says a hash remains personal data anyway.
  - **Normalisation.** Not every write path normalises addresses today: the custom-event path creates contacts with the raw address (`custom_event_service.go:56-64`). Lot 0 normalises that path and audits the others; the hash's own `lower(btrim())` makes it robust either way.
- **Write.** The eraser (manual and batch) inserts the contact's opt-out rows before deleting `contact_lists`, upserting on conflict (latest status and date win). It writes nothing when the caller set `forget_opt_outs`.
- **Restore.** An `AFTER INSERT` trigger on `contacts` inserts the hashed opt-outs back into `contact_lists`, with their status, for lists that still exist and are not soft-deleted (`lists.deleted_at IS NULL`; list deletion is soft, `list_postgres.go:191, 212`), with `created_at = updated_at = now()` and `deleted_at = NULL` (`ON CONFLICT DO NOTHING`: a row already there is a later write and wins), then deletes the hashes. `RecordForEmail` applies the same list filter.
  - It sets a transaction-local GUC, `notifuse.opt_out_restore`, around that insert. Both `contact_lists` functions read it:
    - `track_contact_list_changes` writes a timeline row of a new kind, `list.opt_out_restored`;
    - `webhook_contact_lists_trigger` emits nothing, where it would emit `list.unsubscribed` (`webhook_triggers.go:160-166`).
  - Otherwise every automation and webhook subscriber listening for `list.unsubscribed` would fire on a re-import.
  - Consumers of the new kind are listed and updated: `domain/automation.go` and console `TriggerConfigForm.tsx`. It emits no webhook, so the webhook event list is not touched.
- **Deleted lists.** Deleting a list deletes its hashes.
- **After erasure: not covered.** An unsubscribe link, a bounce or a complaint arriving after the contact was deleted is dropped, as today (`list_service.go:724`). Recording a hash there would need the list binding, which reads `message_history.contact_email` (`message_history_postgre.go:392`), already `DELETED_EMAIL`. It would also recreate personal data after a `forget_opt_outs` delete. Dropped for simplicity; the person was not reachable anyway.
- **Expiry.** The daily purge worker deletes hashes whose `recorded_at` is older than 3 years. It runs in every licence state: it is a deletion. Pierre chose a fixed 3 years. Review 2 notes that the CNIL gives 3 years as a minimum and art. 21(3) sets no end, so after expiry a re-import re-subscribes the person.
- **Preview.** For the contact category, the preview says how many contacts will be deleted and how many opt-outs will be kept as hashes.

**Triggers during a purge**: the purge runs with a transaction-local GUC `notifuse.retention_purge = 'on'`, the pattern `audit_logs_purge` already uses (`schema/audit_logs_tables.go:23-25`). When it is on, the `contact_segments` triggers write no `segment.left` timeline row or webhook for a contact being erased. `track_contact_changes` needs no change: contacts are deleted, never updated, and `contact_changes_trigger` is `AFTER INSERT OR UPDATE` (`init.go:788`). With the GUC on, the webhook trigger on `contacts` sends `contact.deleted` with `{"email"}` only (D8). These function bodies are compiled into every workspace, so v44 replaces them.

## Licence

- **Feature**: `domain.FeatureDataRetention = "data_retention"`, the exact wire string the homepage already uses.
- **Gate (G7)** in `DataRetentionService.SetWorkspacePolicy` and `SetDeploymentDefaults`. A save passes without a licence when it only disables categories or removes exemptions. Anything else (a new category, a changed period, new exemptions) returns `*ErrFeatureNotLicensed` (402, `RequiredTier: Enterprise`).
- **What is never gated**: the preview, the run list and the export are never gated. The purge worker holds no `EntitlementProvider`, and policies saved under a key keep running after it lapses.
- **Documentation and licence texts**:
  - the call-site list in `license.go` gets the G7 entry; `TestEntitlementProviderCallSitesAreListed` enforces it;
  - LICENSE Additional Use Grant gets item (7): "configuring an automatic data retention policy";
  - LICENSING.md, `homepage/src/data/licensing.ts` (`licensedCapabilities` en/fr, removal from `comingSoon`, "Six capabilities" → "Seven" in `selfHostBlurb` and the cloud FAQ), and `docs/self-hosting/licence.mdx` are updated to match.

## Files

### Lot 0 — `contacts.delete` leftovers and opt-out hashes (free, own PR)

Lot 0 needs a workspace migration for the hash tables and the three triggers. If it ships in the same release as Lot 1, both go in one `v44.go`, amended freely while 44.0 is unreleased. If it ships first, it takes `v44` and Lot 1 takes `v45`.

| File | Change | Test file / cases |
|---|---|---|
| `internal/service/contact_service.go` | `DeleteContact` also deletes the contact's `contact_automations` rows (node executions cascade) and its `automation_trigger_log` rows, and redacts `channel_options` and `attachments` in `message_history`. The steps are factored into a `contactEraser` reused by Lot 1's batch purge. | `contact_service_test.go`: each new step is called with the email; a failure in a best-effort step does not abort the delete (same contract as the web-analytics step); the order matches the existing comment. |
| `internal/repository/automation_postgres.go` (+ trigger log repo) | `DeleteContactAutomationsByEmail`, `DeleteTriggerLogByEmail` | `automation_postgres_test.go`: sqlmock SQL and args; zero rows is no error. |
| `internal/repository/message_history_postgre.go:1311` + `schema/message_history_status_trigger.go` | The redaction also sets `channel_options = NULL`, `attachments = NULL`, `status_info = NULL`. **The `DELETED_EMAIL` guard in `track_message_history_changes` (P5) ships in Lot 0's migration**, because this row is what triggers the copy. | `message_history_postgre_test.go`: the UPDATE sets the 6 columns. |
| `internal/repository/inbound_webhook_event_postgres.go:456` | The redaction also sets `bounce_diagnostic = NULL` (the MTA text quotes the recipient) | `inbound_webhook_event_postgres_test.go`: the UPDATE sets the 3 columns. |
| `internal/repository/contact_engagement_postgres.go` | `DeleteForEmail` (the table is created in Lot 1; Lot 0 ships first, so this row lands with Lot 1 and is listed here so the eraser stays one routine) | sqlmock |
| `internal/repository/contact_opt_out_hash_postgres.go` (new) | `RecordForEmail(email, source)` (insert … select from `contact_lists`, upsert), `DeleteForList(listID)` (list deletion), `PurgeOlderThan(before)` | sqlmock: the hash expression; only the 3 statuses; upsert keeps the latest; integration below |
| `internal/database/schema/contact_opt_out_triggers.go` (new) + `init.go` + migration | The two tables, the key generator, `contact_opt_out_hash(email)`, the restore trigger on `contacts`, the `notifuse.opt_out_restore` branch in both `contact_lists` functions (`list.opt_out_restored`, no webhook) | `contact_opt_out_triggers_test.go`: the generated SQL. Integration:<br>• restore after re-import through `contacts.upsert`, `contacts.import` and the custom-event path, with mixed-case and padded addresses;<br>• the hash is deleted once restored;<br>• a deleted list is skipped;<br>• no `list.unsubscribed` timeline row, no webhook and no automation run on restore;<br>• a workspace built by `repairWorkspace` has a key;<br>• then D12 for API, import, console and the automation node. |
| `internal/service/contact_service.go`, `internal/http/contact_handler.go`, console delete dialog | `forget_opt_outs` (bool, default false) on `contacts.delete`; the checkbox "Also forget their unsubscribes" | handler test with a raw JSON body with and without the field; service test: no hash written when set |
| `internal/repository/contact_list_postgres.go` | D12 guard in `AddContactToList`; new `ResubscribeContactToList` (no guard on `unsubscribed`, still none on `bounced`/`complained`) | sqlmock: the guard clause; the resubscribe method's clause |
| `internal/service/list_service.go`, `internal/service/contact_list_service.go`, `internal/http/list_handler.go` / `contact_list_handler.go` | The `resubscribe` field (bool, default false) on `lists.subscribe` and `contactLists.updateStatus`. With it, the row is set `active` with no double opt-in. Without it, the stored status is reported, and the forced double opt-in branch for `unsubscribed` is removed. The public path (no bearer token) keeps its double opt-in. | service tests: API key without the flag → `unsubscribed` reported, no email; with the flag → `active`, no email; bounced with the flag → still bounced; public path unchanged; handler tests with raw JSON bodies, with and without the key |
| `internal/domain/audit_actions.go`, the two services | Both routes move to `AuditedActions`; `AuditRecord.Skip()` unless a row left `unsubscribed` | audit test: an entry when a row moved, none for a flagged call that changed nothing, none without the flag; the route-classification test stays green |
| `internal/service/automation_node_executor.go` | Nothing to change in logic (the guard refuses), but the node reads back the stored row with `GetContactListIncludingRemoved` and writes the refusal into its execution output | node test: add-to-list on an unsubscribed contact → row unchanged, output says so |
| Console contact drawer | A "Re-subscribe" action on an unsubscribed list row, which sends the flag, with a confirmation that says the double opt-in is skipped | component test |
| `internal/service/custom_event_service.go` | The contact it creates is normalised like the other paths | service test |
| `internal/service/data_retention_worker.go` (new) | Daily; in Lot 0 it only purges hashes older than 3 years. Lot 1 extends it. It holds no licence provider. | worker test: interval, `RunOnce`, the 3-year bound |
| `internal/repository/webhook_delivery_postgres.go` | `DeleteForEmail`: deliveries not yet delivered whose payload carries the address (`payload @> {"email": …}` at the top level and under `contact`), so queued `contact.updated` / `list.*` payloads do not ship the data after the erasure. The `contact.deleted` row the delete itself queues is kept (manual delete keeps today's payload, D8). | sqlmock; integration below |
| `internal/repository/attachment_postgres.go` | `DeleteOrphans(ctx, olderThan)`: attachments no `message_history.attachments` references any more | `attachment_postgres_test.go`: SQL; integration case below. |
| `tests/integration/contact_delete_erasure_test.go` (new) | A contact with an automation run, a "once" trigger log, a message with cc/bcc and an attachment, a bounced message whose `status_info` quotes the address, unsubscribed from one list and active on another, deleted through `/api/contacts.delete` as a raw JSON body | No row of any workspace table still holds the address, and none holds the cc/bcc values or the attachment bytes, **except the one `contact.deleted` webhook delivery the delete queues** (manual delete keeps its full payload, D8; it goes with the 7-day sweep). Checked by scanning every text/jsonb column for the address, rather than listing tables. Exactly one hash row exists, for the unsubscribed list. Re-importing the address restores `unsubscribed` on that list only. With `forget_opt_outs: true`, no hash row. |

### Lot 0b — cloud pricing FAQ (homepage repo, separate PR)

`homepage/src/pages/pricing/cloud.astro:153-156`: the answer no longer says data past the window "is permanently deleted". It describes the window as the history the plan guarantees to keep queryable. The tooltip at line 79 is aligned. `cloud/plans/PRICING_STRATEGY.md:179` is aligned the same way. No test; check the built page.

### Lot 1 — engine (Enterprise)

**Domain**

| File | Change | Test file / cases |
|---|---|---|
| `internal/domain/data_retention.go` (new) | `RetentionCategory` (closed set), `DataRetentionPolicy {Categories map[RetentionCategory]int /*days*/, ExemptListIDs, ExemptSegmentIDs, EngagementEventNames}`, `PendingChange {Category, Days, EffectiveAt}`, `Validate` (bounds per category, known categories only, exemption ids non-empty), `Effective(defaults)`, `Widens(stored)` for the licence gate, `RetentionRun` and its counts. | `data_retention_test.go`: bounds per category (min-1 refused, min accepted, 0 accepted, >3650 refused); unknown category refused; `Effective` (nil workspace policy → defaults, per-category override, 0 override disables a default); `Widens` table (disable only → false, remove exemption → false, shorten → true, lengthen → true, new category → true, add exemption → true, any change to `EngagementEventNames` → true); removing an event name is a pending change, like shortening, because it can cause deletions; JSON round-trip. |
| `internal/domain/license.go` | `FeatureDataRetention`, `IsValid`, `NewFeatureNotLicensedError` case, G7 call-site entry | `license_test.go`: `TestFeatureIsValid`, `TestFeatureWireStrings` (`"data_retention"`), `TestErrFeatureNotLicensed` (Enterprise), call-site test stays green. |
| `internal/domain/workspace.go` | `WorkspaceSettings.DataRetention *DataRetentionPolicy` (nil = deployment defaults) and `PendingRetention []PendingChange`, preserved across partial updates like `AuditLogs` (`workspace.go:1671`) | `workspace_test.go`: a general settings update keeps both fields; marshal/unmarshal. |

**Migration v44** (`internal/migrations/v44.go`, system and workspace DBs; `config/config.go` VERSION 44.0 in the same commit)

The migration:
- creates `contact_engagement (email VARCHAR(255) PRIMARY KEY, last_engaged_at TIMESTAMPTZ NOT NULL)` with an index on `last_engaged_at`;
- creates `data_retention_runs (id UUID PK, started_at, finished_at, status, policy JSONB, counts JSONB, error TEXT)`;
- `CREATE OR REPLACE`s the `contact_segments` timeline and webhook functions and the `contacts` webhook function, so each honours `notifuse.retention_purge`;
- creates `data_retention_erasure_queue (email, run_id, enqueued_at)` and `data_retention_state`;
- carries Lot 0's objects if both ship together: `track_message_history_changes` (the `DELETED_EMAIL` guard, P5), the opt-out hash tables, key and function, the restore trigger, and the two `contact_lists` functions (`notifuse.opt_out_restore`);
- system part: inserts the backfill tasks.

The new bodies come from the `schema` package, which `init.go` also uses, so new and migrated workspaces cannot differ. It is idempotent (`IF NOT EXISTS`) and runs in a transaction. The backfill is **not** in the migration: it runs as a task (below).

Test file: `v44_test.go`:
- version 44;
- system part (backfill tasks, `WHERE NOT EXISTS`) and workspace part;
- the SQL contains each `IF NOT EXISTS`;
- the function bodies equal the `schema` builders;
- integration: running v44 twice is a no-op, and a workspace created fresh at 44 has the same schema as one migrated from 43 (the schema-parity test pattern of web analytics).

**Repositories**

| File | Change | Test file / cases |
|---|---|---|
| `internal/repository/contact_engagement_postgres.go` (new) | `Touch(email, at)` = `INSERT … ON CONFLICT (email) DO UPDATE SET last_engaged_at = GREATEST(…)`; `Backfill(batch)`; `ListExpired(before, exemptions, limit)` (joins `contacts`, excludes exempt list members and exempt segment members, clock = the D5 clock, plus a per-contact floor: never before `db_created_at` + 7 days, so an import carrying historical `created_at` values (a migration from another ESP) is not purged on arrival); `CountExpired` for the preview. | sqlmock: GREATEST upsert; exemptions turn into `NOT EXISTS` subqueries; limit; a contact with no engagement row uses `LEAST(created_at, db_created_at)`; a contact imported yesterday with a 2019 `created_at` is not listed. Integration: an older `Touch` never moves the date back. |
| `internal/repository/data_retention_postgres.go` (new) | Batched purge statements, each with `set_config('notifuse.retention_purge','on',true)` and a `LIMIT`ed CTE (5,000 rows per transaction):<br>• `RedactMessageHistory(before, cursor)`: the keyset cursor `(created_at, id)` is stored in `data_retention_state`, so it survives to the next day's run<br>• `DeleteTimeline(before)`<br>• `DeleteCustomEvents(before)`<br>• `DeleteInboundEvents(before)`<br>• `DeleteWebhookDeliveries(before)`<br>• `DeleteFinishedAutomationRuns(before)`<br>• `ListAnalyticsPartitionsBefore(month)` + `DropAnalyticsMonth(month)`<br>• `DeleteContacts`: the batch form of Lot 0's eraser, hashes included. It honours the constraint `contact_service.go:216-227` documents: no single transaction, contact row first.<br>&nbsp;&nbsp;1. The batch's emails go into a work table, `data_retention_erasure_queue (email, run_id)`.<br>&nbsp;&nbsp;2. In one short transaction with the GUC, the opt-out hashes are written, the contact's queued mail is deleted, and the contact rows are deleted. Writing the hashes there means a re-insert right after finds them.<br>&nbsp;&nbsp;3. The dependents are drained from the work table, idempotently, statement by statement. **The drain skips and dequeues any email that exists again in `contacts`** (a CRM sync re-created it), so a live contact's new rows are never erased.<br>A crash mid-batch leaves the work table to finish from, rather than orphaned history no join on `contacts` can find. Batches are a few hundred contacts, since each carries unbounded history and a `contact.deleted` delivery per subscription.<br>• `Count*` twins for the preview<br>• `InsertRun`, `FinishRun`, `ListRuns` | sqlmock per statement: the GUC is set in the same transaction; LIMIT; the anonymise SET list (P5); `DropAnalyticsMonth` drops the 3 partitions of exactly that month and refuses the current and next month. |
| `internal/repository/webhook_delivery_postgres.go`, `email_queue` | `CleanupOldDeliveries` takes the effective days instead of the constant | existing tests updated for the parameter. |

**Services**

| File | Change | Test file / cases |
|---|---|---|
| `internal/service/engagement_recorder.go` (new) | `EngagementRecorder.Touch(ctx, workspaceID, email, at)`, best effort (logged, never fails the caller) | `engagement_recorder_test.go`: error swallowed; empty email ignored. |
| Entry points: `email_service.go` `VisitLink` (not `OpenEmail`); `list_service.go` `SubscribeToLists` only when `!hasBearerToken` and on DOI confirmation; `inbound_reply_*` on a reply (not an auto-reply); `web_analytics_buffer.go` `projectNavigation` for identified sessions. Custom events need no call (read live, D10). | One `Touch` call each | Each service test asserts `Touch` is called on the right branch (for example `SubscribeToLists`: only when the row written is `active`); The following assert it is **not** called (D5): `OpenEmail`, a click on the unsubscribe link, an API `SubscribeToLists` with a bearer token, an unsubscribe-only preference change, an auto-reply, any send, a contact upsert and an import. `engagement_entry_points_test.go` lists the expected callers by grepping the package, so a new entry point is a conscious choice. |
| `internal/service/data_retention_service.go` (new) | The service exposes:<br>• `GetPolicy`: stored, effective, pending, last runs<br>• `Preview(policy)`: counts per category; contacts to delete and opt-outs to keep as hashes; the segments whose conditions read timeline or custom events older than the new period, which will change<br>• `SetWorkspacePolicy` (owner only, G7, pending logic of D7, owner email, audit entry)<br>• `CancelPending`<br>• `SetDeploymentDefaults` (root, G7)<br>• `ListRuns`<br>• `ExportRuns` (CSV)<br>• `RunWorkspace(ctx, ws, now)`: categories in a fixed order; contacts first, so their history goes in the same run; each category isolated, so one failing does not stop the others; writes the run row, and an audit entry `dataRetention.purged` with counts only, never addresses.<br><br>It takes a per-workspace advisory lock, so two API containers never purge the same workspace at once. The lock is session-level, on a dedicated connection held for the run; behind a transaction-mode pooler it would not hold, so the docs say so.<br><br>Each run has a budget (contacts and rows per category), stops cleanly on shutdown (ctx) and resumes the next day. A first run on a large base therefore spreads over several days rather than flooding subscribers with `contact.deleted` deliveries.<br><br>A change of the root deployment defaults goes through the same preview, the 7-day pending period and the owner email, for every workspace that inherits it.<br><br>Removing an exemption, or deleting an exempt list or segment, is a pending change too: it causes deletions. | `data_retention_service_test.go`:<br>• the gate table: unlicensed disable passes; unlicensed shorten → 402; lapsed key keeps the stored policy effective; the purge path takes no provider (compile-time: the constructor has no provider parameter)<br>• pending: a shortened category is not purged before `EffectiveAt`, is purged after it; a lengthened category is effective at once; cancel clears it<br>• non-owner refused<br>• exemption failure skips contacts only<br>• one category error leaves the others run and the run row says `partial`<br>• audit metadata holds counts, never emails<br>• the lock not acquired → no-op |
| `internal/service/data_retention_worker.go` (from Lot 0) | Extended: daily, 5 minutes after boot, `RunOnce` exported (shape of `AuditLogPurgeWorker`) | `data_retention_worker_test.go`: interval, initial delay, ctx cancellation, `RunOnce` iterates workspaces and continues past a failing one. |
| `internal/service/data_retention_backfill_task.go` (new) + `task_service.go` `getTaskTypes()` | Task `data_retention_backfill` per workspace. It sets `last_engaged_at` = GREATEST of the last `clicked_at` (excluding unsubscribe and preference links), the last `email.replied` and `list.confirmed` timeline rows (P3, no `list.subscribed`/`list.resubscribed`), and the last identified web session (read from `contact_timeline` rows with `entity_type = 'web_session'`, indexed by email, rather than scanning the partitioned `web_sessions`). Batched, resumable from its cursor. The task type is added to `internal/domain/task.go` with its state struct. The contact category cannot be switched on until it has finished; the preview says so. Readiness is a **positive marker in the workspace DB**: a one-row `data_retention_state (backfill_completed_at)`.
  - The task writes it when it finishes; the shared schema in `init.go` writes it for every new or repaired schema, which has no history to read. A missing marker reads as **not ready**, never as ready.
  - v44's system part enqueues one task per existing workspace with `WHERE NOT EXISTS` (a task for that workspace and type). The migration transaction commits before the version is stamped (`migrations/manager.go:375, 467`), so a crash in between re-runs v44; `tasks` has no unique key on (workspace, type) (`system_tables.go:53-74`).
  - A `failed` or `paused` task leaves the marker unset, and the console says so. | `data_retention_backfill_task_test.go`: resumes from cursor; GREATEST across sources; the type is in `getTaskTypes()` (`TestTaskService_RegisterProcessor`). |
| `internal/service/workspace_service.go` | nothing: the key comes from the shared schema, readiness from the task table | — |

**HTTP**

| File | Change | Test file / cases |
|---|---|---|
| `internal/http/data_retention_handler.go` (new) | `GET /api/dataRetention.get`, `POST /api/dataRetention.preview`, `POST /api/dataRetention.set`, `POST /api/dataRetention.cancelPending`, `GET /api/dataRetention.runs`, `GET /api/dataRetention.exportRuns` | `data_retention_handler_test.go`: method checks; workspace_id required; 402 body on the gate; 403 for a non-owner; raw-JSON bodies, including a missing `categories` key and an unknown category. |
| `internal/http/settings_handler.go` | `data_retention_defaults` in the root system config, validated like `audit_logs_retention_days` | `settings_handler_test.go`: round-trip; invalid bounds → 400; unlicensed widening → 402. |
| `internal/app/app.go` | Wire the service, the handler, the worker; webhook-delivery and email-queue cleanups read the effective periods | `app_test.go` wiring test if one covers workers. |

**Console**

| File | Change | Test file / cases |
|---|---|---|
| `console/src/components/settings/data_retention/DataRetentionSettings.tsx` (new) + `SettingsSidebar.tsx` | The page has:<br>• one row per category: a switch, a period, the minimum shown, the CNIL suggestion, and a link to its source;<br>• exemptions (list and segment pickers);<br>• the custom event names that count as engagement (D10), picked from the names the workspace has received;<br>• "Preview" before "Save", in a modal with counts and the affected segments;<br>• a pending banner with the date and "Cancel";<br>• a run history table with export;<br>• `LicenceGateNotice` when unlicensed (disabling stays available);<br>• a link to the audit-log retention. | `DataRetentionSettings.test.tsx`:<br>• save is disabled until a preview was shown for the current values;<br>• the pending banner renders and cancel calls the API;<br>• unlicensed: switches can be turned off, not on;<br>• a period below the minimum shows the error;<br>• the run table renders a `partial` run. |
| `console/src/components/settings/SystemSettingsDrawer.tsx` | Deployment defaults (root) | existing drawer test extended. |
| `console/src/services/api/dataRetention.ts`, `types` | API client and interfaces (no `any`) | typecheck. |
| `console/src/i18n/locales/*.po` | Lingui extraction; all 8 locales | `npm run i18n` / CI check. |

**Integration** (`tests/integration/data_retention_test.go`, tags `integration,licdev`)

1. **Licence gate.** With no key, `dataRetention.set` enabling a category → 402. Disabling a stored one → 200.
2. **End-to-end.** With a key:
   - seed contacts with old engagement (direct SQL on `contact_engagement`, old `created_at` rows, an old analytics month);
   - one contact is unsubscribed from a list, one is exempt by list, one clicked recently, one only opened recently;
   - set a policy; the preview counts match the seeds exactly;
   - run before `EffectiveAt` → nothing removed;
   - advance the service clock, run →
     - the opener is deleted (an open is not engagement);
     - the clicker is kept;
     - the exempt contact is kept;
     - the unsubscribed inactive contact is deleted and one hash row is kept for its list; re-created through `contacts.upsert`, it is `unsubscribed` again on that list; `lists.subscribe` via the API leaves it `unsubscribed` and sends no email, and with `resubscribe: true` makes it `active` with no email (D12);
     - a contact whose only recent activity is a custom event not in `engagement_event_names` is deleted; one with a designated event is kept;
     - a seeded click on the email's unsubscribe link (followed through `/r/…` as delivered, read from Mailpit) did not move the clock;
     - no `segment.left` timeline row or webhook was written for the erased contact;
     - broadcast stats for the anonymised messages are unchanged;
     - the analytics month's partitions are gone;
     - the run row and the audit entry exist with counts.
3. **Lapse.** Expire the key, advance the clock again → the purge still runs.
4. **Two workers.** Hold the workspace's advisory lock from the test, run the worker, and check that it writes no run row. Two workers racing on a small dataset would pass or fail by timing.
5. **Each engagement entry point, positively, through its real HTTP path.** For each, start from an old contact:
   - a click through `/r/…` as delivered;
   - a public subscribe;
   - a DOI confirmation;
   - a reply webhook;
   - a designated custom event;
   - an identified web session.

   The contact survives the run. Seeding `contact_engagement` directly would let an unwired or nil recorder, which is best effort, pass the whole suite.
6. **Re-import.** An old contact re-imported through `contacts.import` without `created_at` still expires (D5 clock).
7. **Redaction leaves no copy.** A bounced message whose `status_info` quotes the address, redacted by the purge and by `contacts.delete`, leaves no `email.updated` timeline row.

Only these tests and Lot 0's erasure test run in the integration step, with `INTEGRATION_TESTS=true`.

### Docs (`../docs`, own PR)

- New `features/data-retention.mdx`: categories, clocks, what counts as engagement (and why opens do not), designated custom events, opt-out hashes, the 7-day delay, the reports, the licence, backups (deleted data stays in your backups until they rotate; re-apply after a restore — EDPB CEF report, February 2026).
- `self-hosting/licence.mdx`: row 7.
- `web-analytics/setup.mdx:107`: the retention setting.
- Contacts deletion page: the Lot 0 tables, the opt-out hashes and the "forget" option. The warning that a re-import re-subscribes a deleted address is replaced.

### Changelog (44.0 WIP)

- One Feature bullet for Lot 1, 2-4 sentences.
- One Fix bullet for Lot 0's leftovers, which fixes released behaviour.
- One Feature bullet for the opt-out hashes: a deleted contact's unsubscribes, bounces and complaints are remembered for 3 years, and re-applied if the address returns.
- One Fix bullet for the automation add-to-list node and the console status update re-subscribing unsubscribed contacts, and the console reviving bounces (released behaviour). This ships in its own PR first, with D12 complete: the guard, `ResubscribeContactToList`, the flag, and the public path of O11. The guard alone would break the public re-subscription.
- One line for D12: without `resubscribe: true`, `lists.subscribe` on an unsubscribed contact no longer sends a double opt-in email and reports `unsubscribed`; with it, the contact is re-subscribed directly.

## Open questions

- **O1, O2** — decided: see D8 and D9.
- **O7, O8** — decided: see D10 and D11 (O8 is reopened as O10). **O9** is gone with the minimal record.
- **O10** — decided: see D12.
- **O11** — decided (Pierre): on a list with no double opt-in template, the contact's own submission through the public path is the re-subscription and sets the row `active` directly. With a template, the double opt-in stays. This also ends today's stuck-`pending` case (`list_service.go:429, 444, 539`).
- **O12** — decided (Pierre): no actor on the timeline row. Who re-subscribed is known only through the audit log, so only with an Enterprise key covering audit logs.
- **O13** — decided (Pierre): the console's "Re-subscribe" action can also clear a `bounced` row, with a confirmation, for example after a mailbox was fixed. A `complained` row stays final. The API's `resubscribe` flag clears `unsubscribed` only.
- **O3 — The "expiring soon" re-permission segment.** The CNIL allows a re-contact before deletion: a segment condition "inactive-contact retention expires within N days" would let an automation run that campaign, and a click would reset the clock. It needs a new condition type in the segment query builder. In v1, or next?
- **O4 — Root ceiling.** D6 gives root a default. Should root also be able to set a maximum the workspaces cannot exceed (the DPO argument)? Deferred unless asked.
- **O5 — End-of-contract deletion certificate** for Cloud tenants (SCC Clause 10(d), GDPR art. 28(3)(g)). This is Cloud manager work, not this engine.
- **O6 — Opens under the CNIL pixel recommendation (14 April 2026).** Only the last-open date is exempt from consent. A mode that keeps nothing but that date is a separate, tracking-consent feature, not retention.

## Pre-existing defects found along the way (separate fixes)

- **The automation add-to-list node can re-subscribe an unsubscribed contact** (`automation_node_executor.go:822-830` → `AddContactToList`, guarded for `bounced`/`complained` only). Closed by D12 for every opt-out, but it is a consent defect in released code and deserves its own Fix line.
- **`contactLists.updateStatus` writes any status unconditionally** (`contact_list_postgres.go:270-273`), so the console can set a bounced or complained row back to `active`; `IsTerminalContactListStatus` has no caller outside its test. Closed by D12's guard on that method too.
- **The bulk upsert overwrites `created_at`** with the import's value, now by default (`contact_postgres.go:1471`). Segments on the creation date drift with every re-import.

## Review 3 — Fable (high), targeted, 2026-11-13

Verdict: `sound_with_changes`. Of the review-2 items:
- resolved: M2, M3, M5, M6;
- partly resolved: B1, B2, B3, B4, M1, M4, M7. Their remaining gaps are fixed above.

New findings, checked against the code and folded in:
1. `UpdateContactListStatus` bypasses the guard (`contact_list_postgres.go:270-273`; it is also a pre-existing defect).
2. The contact's re-subscription dead-ends without a template (O11).
3. The batch eraser's order and the re-created contact.
4. Historical `created_at` values on import (the 7-day floor).
5. The readiness marker and the re-run of v44 (`manager.go:375, 467`).
6. The audit mechanism (`middleware/audit.go:111-127, 261-273`).
7. Unsubscribe after erasure: dropped.
8. The collation, soft-deleted lists, the pooler, the Supabase caller, the public route flag, the node read-back.

The self-contradictions listed were cleaned up.

## Review 2 — Fable (high), 2026-11-13

Verdict: `needs_rework`, 4 blockers and 7 majors, each quoting the plan verbatim.

**Checked against the code and folded in above:**
- B1: the redaction copies `status_info` into the timeline (`message_history_status_trigger.go:183-190`).
- B2: `created_at` is overwritten on import (`contact_postgres.go:1471`); the clock is now based on `db_created_at`.
- B3: the batch eraser must not be one transaction (`contact_service.go:216-227`); now a work table.
- B4: the D11 trigger created two classes of opt-out (`list_service.go:396-447`, `automation_node_executor.go:822-830`); now ordinary rows, plus O10.
- M1: designated events are read live.
- M2: the key is generated in SQL by the shared schema.
- M3: the restore is silent for webhooks.
- M4: concurrent index from the worker; tasks inserted by the system migration.
- M5: normalisation.
- M6: positive tests per entry point, and the lock test.
- M7: keyset cursor; "redaction", not anonymisation.
- Minor findings: partition `lock_timeout`, pending exemptions, the email-queue row, P4 wording, run budget, deployment defaults, deleted lists, unsubscribe after erasure, consumers of the new kind.

**Not changed:**
- The fixed 3-year hash expiry stays Pierre's decision, with the review's objection recorded.
- Scanner clicks rely on the existing bot check (`email_handler.go:223-237`).

**On review 1:**
- Fable judged two of its findings wrongly rejected: the timeline `created_at` index, which is partial, and the backfill tasks, which only a system migration can insert.
- Both points are now handled.

## Review 1 — Gemini 4 Argon (high), via agy, 2026-11-13

Verdict: `needs_rework`, 5 blockers and 5 majors.

**Accepted and folded in above.** Each was checked against the code before being taken:
- the erasure gaps: `status_info`, `bounce_diagnostic`, queued webhook payloads, the `contact_engagement` row;
- the minimal-record lifecycle (API subscribe, re-import, visibility, `Touch`). After the review, Pierre replaced the minimal record with opt-out hashes (D2), which removes the visibility and re-import problems and keeps the API one (D12);
- the re-selecting anonymisation UPDATE, and the missing `sent_at` index;
- unsubscribe links going through `VisitLink`, and click-tracking consent;
- `pending` subscriptions;
- `UpdateContactPreferences` changing no subscription;
- identified sessions projected by the buffer, not the bridge;
- the custom-event edge cases (soft delete, future dates, `BatchImport`, `EmitGoals`);
- backfill readiness for new workspaces and fresh installs;
- the task type in `domain/task.go`.

**Rejected, because they review text that is not in this plan.** The reviewer cited a `RunNow` endpoint, backfill tasks inserted by `V44 UpdateSystem`, an age-based purge of `automation_trigger_log` (this plan forbids it), a `timestamp` column on `contact_timeline`, wiring in `cmd/notifuse/main.go`, and an `init.go` trigger populating `contact_engagement`.

**Rejected as false against the code:**
- "`DeleteContact` never deletes `custom_events`": it does (`contact_service.go:258-259`).
- "deleting `contact_lists` rows fires `list.removed`": both triggers are `AFTER INSERT OR UPDATE` (`init.go:790, 809`).
- "`contact_timeline` has no `created_at` index": `idx_contact_timeline_billable` is one, partial.

**Kept as an alternative, not adopted:** engagement columns on `contacts`. This spares a join, but every click becomes an UPDATE of `contacts`, whose webhook and change triggers would each need a new guard.

## Test commands

```bash
make test-domain
make test-service
make test-repo
make test-http
make test-migrations
docker compose -f tests/compose.test.yaml up -d
INTEGRATION_TESTS=true go test -tags integration,licdev ./tests/integration/ -run 'TestDataRetention|TestContactDeleteErasure' -v
cd console && npm test -- data_retention SettingsSidebar SystemSettingsDrawer
```

Before the commit: `/code-review` on the final diff, then the release gate for a feature that deletes data: mutation check on the purge statements and the engagement entry points, and an independent review.

