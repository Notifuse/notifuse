# Console QA pass — 2026-11-16

Four agents drove Pierre's Chrome on https://notifusedev.com:5175 (branch feat/template-gallery, workspace demo), clicking every control of their area. Rules: saves with QA-prefixed values allowed, no delete, no send, no credentials. Screenshots are in `/var/folders/98/x2bqfngd1j9cq24cs51ggq300000gn/T/claude-chrome-screenshots-IGFT1C/`. No `/api/` request failed anywhere; console output was antd deprecation warnings apart from the items below.

Nothing here is verified in code yet unless it says so: each fix starts by reproducing.

## A. Bugs

### A1. Freezes, loops, broken layout
- **Contacts › contact sheet, Next past the last loaded row freezes the tab.** `contacts?limit=50&contact=deborah.bell552@aol.com` (50 of 1,000) → Next (or J): URL becomes `limit=100&contact=carol.garcia@aol.com`, `contacts.list?limit=100` answers 200, then the tab hangs for good (reproduced twice; the same URL loads fine in a fresh tab). Pointer: `console/src/pages/ContactsPage.tsx` ~350-365 and 761 (limit rewritten from the sheet state).
- **Automation editor, an A/B test step with a 3rd variant loops renders**: 60-80 `[React Flow]: Couldn't create edge for source handle id: "C", edge id: placeholder-edge-<id>-C` per second until variant C is removed.
- **Logs › Audit log, an integration update's drawer grows to ~5,000 px**: the old `email_provider` JSON is one unbroken line in the Changes table; After column, Metadata and Raw entry go off-screen.
- **Automations list is wider than a 1440 px screen**: Activate and "…" columns hidden behind a horizontal scroll (scrollWidth 1250 vs 1170).

### A2. Wrong data shown
- **Demo templates carry every MJML attribute twice** (`font-size="24px" … font-size="14px"`, two background-colors, two border-radius), so each preview screen draws the same template differently. Lead: `internal/service/demo_automation_templates.go` sets kebab-case keys on blocks made by `NewBaseBlock`.
- **Logs › Messages drawer shows the subject as raw Liquid** (`{{contact.first_name}}, Your Weekly Update is Here!`) until the Sent content tab has been opened once; also when moving with the arrows/J.
- **Contact sheet message preview of an email whose data is encrypted renders without personalisation** ("Welcome to our community, !"); Template Data shows the `_encrypted` blob. Mary Cooper's Password Reset renders fine.
- **Goals: the card's median (999, also 999 on purchase) differs from the goal dashboard's (853.63)** for the same goal and period.
- **Dashboard "Delivered" shows 12.5% / 3.9% / 31.8%** while both providers are SMTP, which Logs says don't report delivery ("—").
- **Transactional sheet never shows `utm_term`** (the new editor saves it): `TransactionalDetail.tsx` ~386 loops over source/medium/campaign then content only.
- **Timeline "Emails" filter leaves out "Email delivered"** events (they show under Events).
- **Demo seed contradicts itself on delivery**: incoming "SMTP · Delivered" webhooks for Weekly Newsletter #1 while the message stays "Sent"; delivered step off on opened/clicked rows. Probably the seed.

### A3. Controls that do nothing or do the wrong thing
- **Template selector (segment Activity condition) previews Win-back Offer while "Any template" is Current**; "Use this template" would pick Win-back. Reopening: same.
- **Template selector search leaves a hidden template selected**: after a search, the pane still shows Win-back Offer (not in the results) with "Use this template" enabled.
- **Escape while editing a profile field inline closes the whole contact sheet** (date picker, dropdowns and modals are fine).
- **Blog "No share image → Add" does nothing** (category edit, new category, new post): `['seo','og_image']` in `SeoCards.tsx:66` has no focusable element for `showField` (`CategoryDrawer.tsx:162`) — unconfirmed.
- **A new blog post starts with the Tiptap demo content** (205 words and a mountain photo).
- **Settings › Webhooks, Save on an empty form shows "Failed to save webhook subscription"** on top of the field errors; console `[object Object]` and the antd static-message context warning.
- **Broadcasts, a cancelled broadcast's "More actions" opens an empty menu.**
- **Web Analytics filters: validation errors stay after fixing the fields** (red borders and "All conditions must have a value" until the next submit; valid selects outlined too).
- **Dashboard expanded widget: filtering from page 2 leaves the drawer on page 2 with "No data available"** and no pagination back.
- **Contacts "Load 50 more" scrolls back to the top.**
- **Template editor: line height 1.5 shows as "2" with "%"**; a new Image block is drawn left-aligned while Align is center.
- **Theme editor preview (mobile): body text clipped on the right; date "mars 14, 2024· 8 min read"** (French month, English order, no space).
- **Email provider drawer closes without asking**: an added sender is lost on X/Escape.

## B. UX

- **Template selector / templates**: usage counts ignore sent broadcasts ("Weekly Newsletter v2" Not used though variant B of a sent broadcast; "Used by" labels broadcasts "Newsletter"); "Duplicate and edit" leaves preview text and translation subject previews empty and Create only shows a red dot on Translations; "Code (MJML)" disabled on existing templates with no reason; Save v2 closes the editor; Save buttons active with no change (template, automation, blog post edit); gallery Blank card says "Category to choose" under an imposed Marketing; Opt-in selector Full preview opens in English after picking Spanish.
- **Transactional editor**: an invalid identifier is reported as "Required … Add" in the checks, and error + help repeat the same sentence; template help text stays once chosen; tracking help text never changes with the mode; "UTM added" pill grey vs green between modes; unsaved-changes dialog says "close this drawer" with No/Yes.
- **Broadcasts**: cancelled broadcasts sit under Failed and the sending card says Completed 0/109; subject pencil opens the Templates list in a new tab instead of the editor; data feeds summary/label/validation wording ("the global data feed has no URL", "Feed URL ⓘ:", error as soon as toggled, only one of two empty feeds listed).
- **Automations**: two different "failed" numbers side by side on a step; save error ("All variants must be connected…") is a short toast with no node highlighted.
- **Contacts**: "Advanced conditions: create a segment" drops the active filter; bulk Add to list on a double opt-in list doesn't say opt-in is skipped (check intent); select-all is the 50 loaded rows only; delete confirmations differ everywhere (segment No/Yes popover without warning, list type-the-ID, bulk type-the-count, sheet popconfirm); segment "+1" in Defined by has no tooltip; Messages tab rows not clickable (only the icon), Welcome Email "Transactional" vs "Email"; Update Contact shows the overwrite banner on a locked email; Create list drawer has no Cancel; column selector ignores Escape and its tooltip stays over "New contact"; list icon looks like an unchecked checkbox; Activity broadcast selector not narrowed by the template; Created/Updated At in workspace timezone without a label; Bulk update CSV has no default action; header count stays 999 during a search; focus doesn't move to an added optional field.
- **Web Analytics**: goal filter note only names the session figures while Top Pages, Campaigns, Devices and the heatmap don't change either; breakdown tables ~330 px truncate values; chart axes (annotation label over y-axis, last x label "Nov 1", hourly without dates); Countries map has no tooltip/click filter and the List tab shows codes; filter chips not editable; empty "Apply filter" silent; backfill "cancelled by user" shown as an error; Test all rules counts disabled rules and shows no resolved values; "Is Direct" free text; rule delete Popconfirm purple.
- **Blog**: header count follows the tab while the rail says 12; tab counts ignore search; All mixes dates; Scheduled posts offer Unpublish; Add author doesn't offer existing authors; share description single line; remove-author No/Yes; theme preview uses made-up workspace and categories.
- **File Manager**: search results don't show the folder; rail order differs from grid/table; empty folder delete says "and its 0 files"; no rename; picker selection scrolls out of view; Create folder / storage dialogs use "Label :" inline labels.
- **Settings / Logs / Dashboard**: SMTP password field doesn't show a stored one (SES does); deleting the default provider has no specific warning; custom field Rename can move the label to another slot; Text/date vs String/Datetime; Messages counts vanish on 30 days/All time; Logs tabs inconsistent (periods, Clear all, copy-link, URL); "Emails sent per day" title doesn't follow Opens/Clicks, orphan tooltip line, log scale dips below 1; New contacts row opens a filtered list; 7 d axis stops a day early; Logs 7 d count 1,536 vs dashboard 1,914; outgoing webhooks empty state says "Add a webhook" with one existing; webhook create has no enabled toggle; "Select all" never toggles; SES "register webhooks" looks like a link; card Send test dialog says "saved or not".
- **Search/name fields not focused when their popup opens**: Explore Add dimension, File Manager new folder, blog Add author.

## C. Cosmetic

Title Case strings in a sentence-case UI (Invite Member, Email Address, Edit/Create Webhook, Event Types, Custom Event Filters, Goal Types, Add/Edit Custom Field Label, New Theme, Delete Workspace, Sender Name, Send Test Email, Delete Broadcast, Take a Tour, Custom Events Goal, Column Attributes, Email Opened, "Please enter Server token"); delete confirmations purple vs red; tooltips that stay after a click (Previous (K), Add block in column, category lock over New template, webhooks-not-registered badge, lowercase start); clipped text ("Once per ...", "39 s median time ...", overlapping "Test sent"/"Winner sent", variant pill "Wee…", timeline "11:43 / AM", "Custom Number / 1"); blog settings counters on the row divider; Logs "Clear all" wraps at three chips; Root User's empty Joined cell; API key suffix with the port; workspace page "+" without tooltip and card not keyboard-reachable; audit backfill filed under Contacts; Annotations title twice; segment "0 contacts" overflowing its ring; Timezone editor "Local time" misplaced; colour select pushed by a name error; Pending KPI without percentage; Cart Abandoners header raw `"add_to_cart"`; saved web filter without its label; category tag colours differ by screen; Translations phone mock vs Settings mock; test data contact vs URLs built for john.doe; icon buttons without accessible names (New broadcast, template Send test/Save, variant buttons).

Console warnings: antd deprecations (Space direction in `TrackingConsent.tsx`, Tree rootStyle, Alert message, Modal maskClosable, List, Input addonBefore, validator callback, Table rowKey index); `Form already set 'initialValues' … contact_list.operator`; missing `key` in `input_dimension_filters.tsx` and `TreeNodeInput` (`segment/input.tsx`); static `message` without theme context; ECharts "instance disposed" leaving Web Analytics.

## D. Created on demo during the pass

QA broadcast (draft, A/B), QA template (`qa-template`, v2), QA automation (draft), QA notification (`qa_notification`), QA list (`qalist`, double opt-in) with QA subscription confirmation template, QA segment (`qa_segment`), contact `qa.contact@example.com`, Richard Harris renamed "QA test" with job "QA job", filter rule "QA rule" (disabled), goal none, blog category "QA category", draft post "QA post" (plus its hidden email template), folder `qa-folder`, sender "QA sender <qa@example.com>" on Demo SMTP Integration, webhook "QA webhook" (disabled), custom field label "QA field" on custom_string_5. Not from the agents: the "aa" notification. The nightly demo reset clears all of it.
