# Console QA — TODO

Source: `plans/console-qa-pass-plan.md` (the Chrome pass of 2026-11-16). Done items are checked and struck through; skipped ones say why. Owner = the fixing agent's area (1 Contacts, 2 Templates/Broadcasts/Automations/Transactional, 3 Web Analytics/Blog/Files, 4 Settings/Logs/Dashboard/Shell). Items marked **(ask)** wait for Pierre's decision.

## 1. Contacts, lists, segments
- [x] ~~1.1 Contact sheet Next past the last loaded row freezes the tab (`limit` rewritten from the sheet state).~~ — done.
- [x] ~~1.2 Escape while editing a profile field inline closes the whole contact sheet.~~ — done.
- [x] ~~1.3 Message preview of an email whose stored data is encrypted renders without personalisation.~~ — done (timeline.list decrypts message data); demo automation messages without a first name go with 4.5.
- [x] ~~1.4 "Load 50 more" scrolls back to the top.~~ — done.
- [x] ~~1.5 Timeline "Emails" filter leaves out "Email delivered".~~ — done.
- [x] ~~1.6 "Advanced conditions: create a segment" drops the active filter.~~ — done.
- [x] ~~1.7 Bulk "Add to list" on a double opt-in list doesn't say opt-in is skipped~~ — done (warning in the dialog).
- [x] ~~1.8 Select-all is the 50 loaded rows only (no "select all 1,000")~~ — skipped (Pierre): a feature needing server-side bulk by filter.
- [x] ~~1.9 Delete confirmations differ (segment No/Yes popover without warning, list type-the-ID, bulk type-the-count, sheet popconfirm)~~ — done.
- [x] ~~1.10 Segment "+1" in Defined by has no tooltip listing the hidden conditions.~~ — done.
- [x] ~~1.11 Messages tab: rows not clickable (only the icon); Welcome Email "Transactional" vs "Email" in the timeline.~~ — done.
- [x] ~~1.12 Update Contact shows the overwrite banner when the email is locked.~~ — done.
- [x] ~~1.13 Create list drawer has no Cancel.~~ — done.
- [x] ~~1.14 Column selector ignores Escape; its tooltip stays over "New contact".~~ — done.
- [x] ~~1.15 List icon looks like an unchecked checkbox (rail and sheet Lists card).~~ — done.
- [x] ~~1.16 Activity condition: broadcast selector not narrowed by the chosen template.~~ — done (client-side on the 50 results returned).
- [x] ~~1.17 Created/Updated At in the workspace timezone without a label, while the timeline says another timezone.~~ — done.
- [x] ~~1.18 Bulk update (CSV): no default action, List select without placeholder.~~ — done.
- [x] ~~1.19 Header count stays at 999 during a search.~~ — left as is (Pierre): the header shows the view size.
- [x] ~~1.20 Add contact: focus doesn't move to an added optional field.~~ — done.
- [x] ~~1.21 Copy: operator placeholder "select a value"; segment name placeholder "i.e:".~~ — done.
- [x] ~~1.22 Cosmetic: timeline time wraps ("11:43 / AM"); custom field labels wrap; Timezone editor "Local time" misplaced; segment "0 contacts" overflows its ring; colour select pushed by a name error; Pending KPI without percentage; Cart Abandoners header raw `"add_to_cart"`; saved web filter without its "with filters" label; variant pill "Wee…".~~ — done.
- [x] ~~1.23 Console: `Space direction` (`TrackingConsent.tsx`), `Form already set initialValues … contact_list.operator`, missing `key` in `input_dimension_filters.tsx` and `TreeNodeInput`, static `message` without theme context on segment creation.~~ — done except the missing `key` warnings: not reproduced.

## 2. Templates, broadcasts, automations, transactional
- [x] ~~2.1 Automation A/B test with a 3rd variant loops renders (React Flow edge for handle "C").~~ — done.
- [x] ~~2.2 Demo seed templates carry every MJML attribute twice (fix the seed only~~ — done (seed only).
- [x] ~~2.3 Transactional sheet never shows `utm_term`.~~ — done.
- [x] ~~2.4 Automations list wider than 1440 px (Activate and "…" hidden).~~ — done.
- [x] ~~2.5 Cancelled broadcast's "More actions" opens an empty menu.~~ — done.
- [x] ~~2.6 Template selector (segment Activity condition) previews Win-back while "Any template" is Current.~~ — done (preview follows only a real pointer move; not reproducible headless).
- [x] ~~2.7 Template selector search leaves a hidden template selected and usable.~~ — done.
- [x] ~~2.8 Template editor: line height 1.5 shown as "2" with "%"; new Image block drawn left while Align is center.~~ — done.
- [x] ~~2.9 Usage counts ignore sent broadcasts; "Used by" labels broadcasts "Newsletter".~~ — done: "Not in use" with a tooltip saying sent broadcasts are not counted (server counting unchanged).
- [x] ~~2.10 "Duplicate and edit" leaves preview text and translation subject previews empty; Create only shows a red dot.~~ — done for demo templates (seed carries preview text) and a refused save now opens the language at fault.
- [x] ~~2.11 "Code (MJML)" disabled on existing templates with no reason given.~~ — done.
- [x] ~~2.12 Saving a template closes the editor~~ — done (a new template still closes on create).
- [x] ~~2.13 Save buttons enabled with no change (template "Save v2", automation Save).~~ — done.
- [x] ~~2.14 Gallery Blank card says "Category to choose" under an imposed category; Opt-in selector Full preview opens in English after picking Spanish.~~ — done.
- [x] ~~2.15 Transactional editor: invalid identifier reported as "Required … Add"; error and help repeat; template help stays once chosen; tracking help never follows the mode; "UTM added" pill colour differs between modes; unsaved dialog says "close this drawer" with No/Yes.~~ — done.
- [x] ~~2.16 Cancelled broadcasts sit under Failed; sending card says Completed 0/109.~~ — done (Cancelled tab).
- [x] ~~2.17 Broadcast subject pencil opens the Templates list in a new tab instead of the editor.~~ — done.
- [x] ~~2.18 Data feeds wording: lowercase summary, "Feed URL ⓘ:", error as soon as toggled, only one of two empty feeds listed.~~ — done.
- [x] ~~2.19 Automation step: two different "failed" numbers side by side; "All variants must be connected" only a short toast, no node highlighted.~~ — done.
- [x] ~~2.20 Cosmetic: Title Case strings (Send Test Email, Delete Broadcast, Take a Tour, Custom Events Goal, Column Attributes, Email Opened); tooltips that stay after a click (Previous (K), Add block in column, category lock over New template); clipped "Once per ...", "39 s median time ..."; overlapping "Test sent"/"Winner sent"; category tag colours differ by screen; Translations phone mock vs Settings mock; test data contact vs URLs built for john.doe; icon buttons without accessible names (New broadcast, template Send test/Save, variant buttons).~~ — done: sentence case, tooltips, clipped labels, tags, phone mock; preview links rebuilt from the test contact in the compile API; "Previous (K)" tooltip and "Test sent"/"Winner sent" overlap not reproduced.
- [x] ~~2.21 Console: Modal `maskClosable`, Tree `rootStyle`, Alert `message` deprecations.~~ — done.

## 3. Web Analytics, blog, file manager
- [x] ~~3.1 Goals card median (999) differs from the goal dashboard's (853.63).~~ — done.
- [x] ~~3.2 Blog "No share image → Add" does nothing (category edit, new category, new post).~~ — done.
- [x] ~~3.3 New blog post starts with the Tiptap demo content.~~ — done.
- [x] ~~3.4 Expanded widget: filtering from page 2 leaves "No data available" on page 2.~~ — done.
- [x] ~~3.5 Filter rule editor: validation errors stay after fixing the fields.~~ — done.
- [x] ~~3.6 Theme preview mobile: body clipped; date "mars 14, 2024· 8 min read".~~ — done.
- [x] ~~3.7 Goal filter note names only the session figures while other widgets don't change either.~~ — done.
- [x] ~~3.8 Breakdown tables truncate values (~330 px columns).~~ — done.
- [x] ~~3.9 Chart axes: annotation label over the y-axis; last x label "Nov 1"; hourly without dates.~~ — done.
- [x] ~~3.10 Countries map: no tooltip, no click filter; List tab shows codes instead of names.~~ — country names fixed; map tooltip and click did not reproduce (they work in headless Chrome).
- [x] ~~3.11 Filter chips not editable; empty "Apply filter" silent.~~ — done.
- [x] ~~3.12 Filters page: "cancelled by user" backfill shown as an error; "Is Direct" free text; rule delete Popconfirm purple.~~ — done.
- [x] ~~3.13 "Test all rules" counts disabled rules and shows no resolved values — decided: count and list enabled rules only; disabled ones shown apart, greyed, "not applied"; no resolved values.~~ — done.
- [x] ~~3.14 Blog: header count follows the tab while the rail says 12; tab counts ignore search; All mixes dates; Scheduled posts offer Unpublish; share description single line; remove-author No/Yes; Save enabled with no change in the post edit drawer.~~ — done; All sorts by the date shown and Drafts by last edit (blogPosts.list order, documented in OpenAPI).
- [x] ~~3.15 Blog "Add author" doesn't offer existing authors.~~ — done.
- [x] ~~3.16 Theme preview uses made-up workspace and categories.~~ — done.
- [x] ~~3.17 File Manager: search results don't show the folder; rail order differs from grid/table; empty folder delete says "and its 0 files"; picker selection scrolls out of view; "Label :" inline labels in the folder and storage dialogs.~~ — done.
- [x] ~~3.18 File Manager folder rename~~ — skipped (Pierre): renaming an S3 prefix would break image URLs already in emails.
- [x] ~~3.19 Search/name fields not focused when their popup opens (Explore Add dimension, new folder, Add author).~~ — done.
- [x] ~~3.20 Cosmetic: Annotations title twice; "New Theme" casing.~~ — done.
- [x] ~~3.21 Console: List deprecated, Input `addonBefore`, validator `callback`, Table `rowKey` index; ECharts "instance disposed" leaving Web Analytics.~~ — done except ECharts "instance disposed": left as is (Pierre: library internals, invisible).

## 4. Settings, logs, dashboard, app shell
- [x] ~~4.1 Audit log drawer of an integration update grows to ~5,000 px.~~ — done.
- [x] ~~4.2 Logs › Messages drawer shows the subject as raw Liquid until Sent content is opened.~~ — done.
- [x] ~~4.3 Webhook form: Save on empty fields also toasts "Failed to save"; `[object Object]` in the console.~~ — done.
- [x] ~~4.4 Dashboard "Delivered" rate shown for SMTP providers that don't report delivery.~~ — done (rate counts only providers that report deliveries; all-SMTP shows —).
- [x] ~~4.5 Demo seed: delivered webhooks without delivered messages (seed consistency)~~ — done: seeded delivery/bounce/complaint events are SES notifications with matching message timestamps; automation messages carry first/last name; the demo integration stays SMTP (the preview seed sends through it) and Logs/dashboard show the rate when deliveries came back (Pierre kept that rule).
- [x] ~~4.6 Email provider drawer closes without asking and loses unsaved edits.~~ — done.
- [x] ~~4.7 SMTP password field doesn't show that one is stored (SES does).~~ — not reproduced: the demo SMTP integration stores no password; a stored one already shows its hint.
- [x] ~~4.8 Deleting the default provider has no specific warning.~~ — done.
- [x] ~~4.9 Custom field Rename can move the label to another slot — decided: Rename only renames (slot locked); Text/date vs String/Datetime.~~ — done.
- [x] ~~4.10 Logs › Messages counts vanish on 30 days / All time without explanation.~~ — done.
- [x] ~~4.11 Logs tabs inconsistent: periods, Clear all, copy-link label, drawer URL.~~ — done: Messages, Incoming webhooks and Audit log share one period list incl. 90 days and custom range; Outgoing has none (API); no drawer URLs (Pierre).
- [x] ~~4.12 Dashboard chart: title doesn't follow Opens/Clicks; orphan tooltip line; log scale dips below 1; 7 d axis stops a day early; New contacts row opens a filtered list.~~ — done; the 7 d axis ending yesterday is by design (complete days), now labelled.
- [x] ~~4.13 Logs 7 d message count (1,536) vs dashboard 7 d (1,914).~~ — both counts are right (complete days vs rolling 7×24 h); the dashboard now shows its date range.
- [x] ~~4.14 Outgoing webhooks empty state says "Add a webhook" with one existing; create form has no enabled toggle; "Select all" never toggles.~~ — done; new webhooks stay enabled on create (Pierre).
- [x] ~~4.15 SES "register webhooks" looks like a link; card Send test dialog says "saved or not".~~ — done.
- [x] ~~4.16 Cosmetic: Title Case strings (Invite Member, Email Address, Edit/Create Webhook, Event Types, Custom Event Filters, Goal Types, Add/Edit Custom Field Label, Delete Workspace, Sender Name, "Please enter Server token"); delete confirmations purple vs red; webhooks badge tooltip lowercase and sticky; blog settings counters on the row divider; Logs "Clear all" wraps at three chips; Root User's empty Joined cell; API key suffix with the port; workspace page "+" without tooltip and card not keyboard-reachable; audit backfill filed under Contacts.~~ — done: sentence case, red delete confirmations, badge tooltip, counters, Joined cell, API key port, workspace page; audit resource "Web analytics"; Period chip on the right of the Logs toolbars (search 240 px) — "Clear all" still wraps with a long third chip on Messages and on the root Audit log row; sticky badge tooltip not reproduced.
- [x] ~~4.17 Console: unhandled `[object Object]` rejection in integrations (not reproduced).~~ — done (sender dialog Save on an invalid address).
