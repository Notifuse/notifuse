# Changelog

All notable changes to this project will be documented in this file.

## [39.0] - 2026-08-18

- **Feature**: Zapier. Settings → Integrations lists Zapier beside your other connectors, and connecting it creates an API key scoped to just what a Zap needs. From there a Zap can react to what happens in Notifuse — a contact created, a list subscribed to, a segment a contact entered — or reach the other way and create contacts, update them and subscribe them to your lists. Each Zap you switch on registers its own webhook subscription, which appears in Settings → Webhooks marked as Zapier's; switching the Zap off in Zapier removes it for you. Deleting the Zapier integration revokes the key it created, so the Zaps using it stop.

- **Fix** (direct API usage only — the console sends every field its forms own): Endpoints no longer clear the fields your request left out. A call naming only a name could blank a broadcast's schedule and audience targeting, an integration's credentials, a notification's channels, an automation's workflow, a blog theme's files or a workspace's file-storage settings. They patch now: omit and the stored value stands, send and it replaces, send empty and it clears. `lists.update` is the exception — an omitted or null `is_double_optin` or `is_public` answers 400 instead of guessing, because turning off confirmed opt-in is a consent change.

- **Fix**: Four fields the console dropped on save. Editing a webhook subscription switched it off and deliveries stopped silently; saving system settings blanked the OIDC redirect URI, breaking proxied SSO callbacks; saving a template detached the integration that owned it, which no API call could undo; and unchecking a broadcast's data feed left it running.

- **Feature**: A webhook subscription can be scoped to particular lists or segments. List and segment events previously reached every subscription watching that event type, whichever list or segment they concerned, leaving the filtering to whatever received them. Naming the lists or segments you care about now keeps the rest from being sent at all. Leave it unset and the subscription keeps receiving everything, exactly as before.

- **Fix**: A webhook subscription's signing secret is no longer handed out by editing it. The secret is owner-only, but renaming a subscription — or simply switching it off and back on — answered with the whole subscription, secret included, to any member holding webhook write permission. It is now returned only when a subscription is created or its secret deliberately rotated, which remain the two moments you are meant to copy it.

- **Improvement**: Outbound webhooks are now safe on more than one instance. Notifuse is designed to run as a single instance, and a second one would pick up the same queued deliveries independently, so a receiver saw the same event two or three times with nothing to tell the copies apart. A delivery is now claimed by one instance before it is sent, so running several no longer duplicates them. Receivers that built their own duplicate filtering can keep it; it will simply stop firing.

- **Improvement**: `tracking_enabled` has been removed from the broadcast create and update endpoints. Nothing ever read it — email tracking is a workspace-level setting — so clients sending it were being answered with success and no effect. Drop it from your requests; tracking itself is unchanged.

- **Improvement**: Outbound webhooks stop hammering an endpoint that has stopped answering. A subscription whose deliveries keep failing is now switched off automatically and records why, so a webhook you find disabled can be told apart from one you disabled yourself, and an endpoint replying "410 Gone" is retired at once. Retiring one takes sustained failure rather than a burst: a receiver that is briefly unreachable during an import is retried, not switched off, however many deliveries it refuses in that moment. Switching a subscription back on clears its failure history. Deleting a subscription now also clears the deliveries still queued for it, which previously lingered for the whole retention window and crowded out live traffic.

- **Improvement**: `contacts.upsert` and `lists.subscribe` now report what they did. Upsert returns the stored contact as it ended up after the merge, and subscribe returns one entry per requested list with the membership status it landed in — which double opt-in, an earlier unsubscribe or a bounced address can each decide for you. Both are additions; the fields existing clients already read are unchanged.

- **Fix**: `customEvents.upsert` no longer relabels an event's origin. A later write always stamped its own source over the stored one, so an event bridged in from Web Analytics became API-sourced the first time any call touched it — and because the outbound-webhook trigger excludes analytics events by reading that same field, those rows then started fanning out to third-party webhook subscribers. A write now leaves an origin it was not asked to change.

- **Improvement**: `customEvents.upsert` answers with the event as stored rather than with the request you sent. The write keeps the fields your body left out and discards a write whose `occurred_at` is not newer than the one already recorded, so the request was never a reliable description of what the row ended up holding.

- **Improvement**: `segments.contacts` can return whole contact records instead of email addresses alone, most recently joined first, so building on a segment no longer costs a lookup per address. Pass `expand=contact` to ask for it; without it the endpoint answers exactly as before.

- **Improvement**: `contacts.upsert` and `contacts.import` accept any JSON value in a `custom_json_*` field. They refused anything that was not an object or an array, while `lists.subscribe` stored the same value happily — so the same contact body succeeded through one endpoint and failed with a 400 through another.

- **Fix**: Endpoints answer with the right status when a request cannot be served. A revoked API key and a resource that no longer exists both came back as a server error, which reads to any client as "this instance is broken" rather than "this credential is dead" or "that list is gone" — so integrations retried instead of reconnecting or moving on. Revoked keys now answer 401, missing resources 404 naming what was not found, and deleting a webhook subscription that is already gone succeeds.

- **Feature**: API keys can be scoped. When you create a key you can now choose which resources it may read and write, instead of every key holding the run of the workspace. Leave the permissions out and the key gets full access as before, and you can change a key's scope at any time without re-issuing it. Two to know before narrowing one: sending transactional email needs Transactional write, and Contacts write is also what permits deleting contacts.

- **Change**: Several endpoints now enforce the permission they always implied — contact timelines, list membership, analytics queries, provider tests and workspace details. Members invited before permissions existed hold read-only grants, so a few write actions they could perform will now be refused; an owner can restore them from the members screen.

- **Change**: Sending transactional email requires Transactional write, over the API and the SMTP bridge alike. Registering webhooks with an email provider is now restricted to owners, like the integration it belongs to. The task endpoints require the permission belonging to the task's own feature, and tasks can no longer be created directly.

- **Change**: New Segments and Webhooks permissions. The v39 migration grants them to existing members, API keys and pending invitations, so nothing loses access on upgrade — re-narrow a key if that is not what you want.

- **Improvement**: Notifuse refuses to start when its database has been migrated by a newer build than the one starting. Such a database used to be reported as up to date and the build served traffic against a schema it was never compiled against; startup now stops instead, naming both versions and the two ways out — deploy a build at least as new as the database, or restore a database stamped no higher. Plan upgrades around it: a rolling deploy has to replace every replica before the first one migrates, because an older replica restarting afterwards will not come back up, and rolling back to a previous image needs a database from before the upgrade, since there is no down-migration and nothing lowers the recorded version for you.

- **Fix**: Closed several ways to gain access you were not granted. Any member could invite someone with any permissions they chose, an API key's address could be used to sign in as a person, and removing a key reported success even when the key had not actually been revoked.

- **Improvement**: A refused request now answers "forbidden" and names the missing permission, instead of a server error. This reaches the contact, list, segment and webhook endpoints too, which previously answered a permission refusal with a server error.

- **Improvement**: Web Analytics no longer credits a visit to your own domain. A session that starts mid-visit — when the 30-minute inactivity window lapses on a tab left open, or on the visitor's next internal click — took whichever of your pages they arrived from as its referrer, so your own hostname appeared in the referrers report and the visit's real acquisition source was lost. A referrer on the same host as the landing page is now dropped and the session reads as direct; another host, such as a docs subdomain, is still a real referral. Rows already recorded keep their values.

- **Fix**: The console no longer opens in the wrong language. Every time you opened it, the language remembered in your browser and the one saved on your account were fetched at the same time, and whichever arrived second decided the interface — so it could come up in a language you had only ever tried out, intermittently, with the language button still naming the right one. Your saved language now always wins, and a language is only remembered when you actually pick one.

- **Improvement**: Open tracking no longer leaves an empty row at the bottom of your emails. The tracking image sat on a line of text, so every tracked email ended about eighteen pixels taller than its own layout — in Outlook, a sliver of blank space you could still scroll into below the footer. The row now takes no height at all. Nothing changes about how opens are counted.

## [38.0] - 2026-08-17

- **Feature**: Web Analytics — the Staminads feature set, rebuilt on PostgreSQL. A ~21 KB gzipped cookieless browser SDK (`/na.js`) streams sessions, pageviews and goals into monthly-partitioned tables per workspace, with engaged-time metrics (TimeScore, bounce rate), editable channel-attribution rules (40 defaults, with historical backfill), goals carrying a value and a declared type — purchase, subscription, lead, signup, booking, trial or other, because only your site knows which it is — 10 custom dimensions, geo resolution from a bundled MaxMind GeoLite2 City database, and a console section with Dashboard, Explore, Goals, Filters, Annotations and Settings tabs plus a live view. Visitors can be tied to a contact: `NotifuseAnalytics.identify(email, hmac)` attaches a verified address to a session — the signature is checked against the workspace secret, so only your own server can mint one — and an address it vouches for becomes a contact if it is not one already, while visitors arriving from a tracked email link are identified automatically with no code. An identified visitor's sessions and pageviews then appear on the contact timeline, any page openable in a side panel, and both navigation and goals are usable in segment conditions, with goals also able to trigger automations (navigation never does). Calling identify() is itself the opt-in: a workspace that wants anonymous reporting simply never calls it, and obtaining consent to store browsing history against a contact remains yours to do. Charts can be annotated — a launch, a price change, an outage — each pinned to the timezone it was entered in, and broadcasts annotate themselves the moment a send starts. An AI assistant answers questions about your own traffic from the same reports the dashboard draws, so its answers and the charts beside them can never disagree; it can also drive the screen — period, comparison, filters, Explore reports — needs an LLM integration configured under Settings → Integrations, and never groups or filters by visitor email address. The v38 migration adds the tables to existing workspaces and grants the new web analytics permission to existing members and pending invitations; the feature is off until enabled in a workspace's Web Analytics settings. For high-traffic installs, `compose.alloydb.yaml` runs the same schema on AlloyDB Omni's columnar engine.

- **Improvement**: A sent broadcast links straight to its web analytics reports. Two icons in the broadcast's header open the Explore and Goals tabs filtered to that broadcast's UTM campaign over its send window plus the following week, and every variation row carries the same pair narrowed to its own UTM content — so an A/B test's two creatives can be compared by what they did on the site, not only by opens and clicks. The links need a UTM campaign on the broadcast: without one they stay disabled, because filtering on an empty campaign matches every untagged visit rather than none. A broadcast that sets its own UTM content disables the per-variation links for the same reason, since every variation then ships that one value instead of its template's.

- **Improvement**: The General and Blog workspace settings now use the same floating save bar as Web Analytics. It appears only once something has changed, offering Discard and Save Changes plus a Cmd/Ctrl+S shortcut, and leaving the section with unsaved edits asks for confirmation instead of dropping them silently — so the save control is no longer stranded below the fold of a long form. Pressing Enter in a field no longer submits these forms; save from the bar or with Cmd/Ctrl+S.
- **Improvement**: Saving blog settings no longer erases the SEO canonical URL. The console never loaded the stored value into the form, so every save sent it back empty and the blog's `<link rel="canonical">` tag disappeared. The field is now populated from the saved settings.

- **Improvement**: Tracked email links are no longer altered or missed. Appending UTM parameters rewrote the entire query string — dropping pairs Go's URL parser rejects (`?sid=1;2`, `?discount=50%off`) and re-escaping the rest — while uppercase `<a HREF="…">` was skipped altogether, leaving those links with no click tracking, UTM parameters or identity token. Links are now matched case-insensitively and left byte-for-byte as authored apart from what Notifuse appends; per-link click reporting groups by the rewritten URL, so an affected link starts a new row from this release.

- **Improvement**: `analytics.query` no longer answers an empty breakdown with one invented row. A grouped query that matched nothing returned a single row with an empty dimension and zero measures, which reads as real data and hides a table's own empty state. Ungrouped totals still answer zero, because a KPI has to render a number.
- **Improvement**: A sign-in link that carries an email (`/console/signin?email=…`) no longer loses it on the first click. A stale token in the browser makes the console's opening `user.me` call fail, and the 401 handler redirected to a bare `/console/signin` — discarding the query string that page needs — so the visitor got an empty form and only the second click worked, once the token had been cleared. The redirect is now skipped when the browser is already on the sign-in page.
- **Improvement**: The console renders Ant Design's own strings in Japanese, Italian and Brazilian Portuguese. All three were offered in the language picker and their translations shipped, but the map that hands a locale to Ant Design covered only five languages, so date pickers, table filter and sort menus, pagination and empty states silently stayed English for them. Notifuse's own strings were translated throughout, which is why this read as a half-translated screen rather than a missing language.

- **Improvement**: Listing, editing and deleting transactional notifications now require the Transactional permission. The three endpoints checked workspace membership only, so a member whose transactional access had been revoked could still read the notifications, repoint one at a different template and delete it — the console offered the permission and the API ignored it.
- **Improvement**: `analytics.query` now covers the whole of a date range's last day. A `timeDimensions` range ending on a bare date stopped at that day's midnight, so a report ending today showed nothing for today and a single-day range came back empty — the console compensated by asking for tomorrow instead. Bounds given as an explicit timestamp are still used exactly as sent.

- **Improvement**: Integration credentials are no longer sent back to the browser. Editing an integration and leaving a credential field blank now keeps the stored one, so only a value you actually type replaces it.
- **Improvement**: A send that fails no longer stores its template data in clear. Recording the failure rewrote the whole message history row from the copy held in memory, which still carried the plaintext variables, overwriting the blob that was encrypted when the row was created — magic links, confirmation tokens and API keys included. Transactional and double opt-in sends were affected; broadcasts and automations record failures differently and were not. Rows already written that way are left as they are.
- **Improvement**: Deleting a contact now erases every copy of their address. Message history and inbound webhook events had their identifying column replaced with `DELETED_EMAIL` while the address remained inside the stored payload on the same row — both of which are returned by their list endpoints — and custom events, segment memberships and segment-queue entries were not cleaned at all.
- **Improvement**: Deleting a contact now cancels email already queued to them. Queued messages carry the address independently of the contact record, so a broadcast or automation send that had not yet drained would still arrive after the contact was erased.

- **Improvement**: A List Status node now renders in an automation's Flow Stats. It appeared there with no icon and no title, and all three of its branches were dropped, so everything downstream of it floated on the canvas disconnected from the flow it belongs to.

- **Improvement**: Text that had gone missing from three screens is back, in every language. Broadcast status badges and their tooltips rendered blank, the automation Add node menu listed raw keys like `list_status_branch` instead of node names, and the toast shown when single sign-on refuses a login carried no message at all. Each passed its translation function into a helper, which silently disabled it and left those strings resolving to nothing — translations most of them already had.

- **Feature**: Automation triggers can filter who enters. A trigger now takes **entry conditions** — the same builder segments use, over contact properties, list memberships, past activity and goals — so an event enrolls only the contacts that match. Set them in the trigger panel under Entry conditions. A contact who does not match is never enrolled, which differs from a Filter node after the trigger: that enrolls first and then exits, and with **Once per contact** it uses up the contact's single entry. A count of past activity includes the event that fired the trigger, so "opened at least 3 emails" enrolls on the third open.

- **Improvement**: Every automation node takes an optional description, not just the Filter node. It appears under the node's title on the canvas and in Flow Stats, so a flow of Email and Delay cards explains itself without opening each node. Removing a condition tree now asks for confirmation first.

- **Improvement**: Undo in the automation editor now works in whole edits rather than single characters. Typing in a text field — a webhook URL, a custom event name, a node description — recorded a step per keystroke, so Cmd+Z crawled back one letter at a time and a long edit could push earlier node and connection changes out of the 200-step history. A run of typing in one field is now a single step.

- **Improvement**: An automation's status and the database trigger that enrolls contacts for it are now kept in agreement. They were saved separately, so an automation could keep enrolling after being paused or deleted, or show Live and enroll nobody; enrollment now checks that the automation is live, and two people acting on the same automation at once no longer silently undo each other.

- **Fix**: In the Blog and Email AI assistants, a reply that only used a tool and wrote no text left the conversation in a state the AI provider rejected, so the next message failed. The conversation is now repaired before it is sent.
- **Fix**: `user.me` answers 500 instead of 401 when it cannot verify a session at all. A transient database error was reported as an expired session, and the console deletes the stored token on any 401 — signing people out over a blip.
- **Fix**: Duplicate rows are recognised on a PostgreSQL server running a non-English locale. They were detected by matching the driver's English error text, which PostgreSQL translates according to `lc_messages` — so on a French or Japanese server single sign-on reported an account already linked to another identity as a generic “sign-in failed, please try again”, re-running the setup wizard failed instead of accepting the root user it had already created, and creating a template whose id was already taken answered a bare 500 rather than saying which id clashed. Detection now keys off the SQLSTATE, which no locale changes.
- **Fix**: The transactional notification endpoints now answer 403 when a member lacks permission, instead of a 500 — or, when the denial came from the template check, a 400 blaming the template (`invalid template for channel email: Insufficient permissions…`). `transactional.testTemplate` went further and answered 200 with the denial buried in the response body; it also answers 404 again for a template that genuinely does not exist.
- **Fix**: The blog feed no longer compresses its response for a client that refused compression. `Accept-Encoding: gzip;q=0` is an explicit refusal, and the feed read it as consent.

## [37.3] - 2026-08-06

- **Fix**: A broadcast that was interrupted once no longer displays that error for the rest of its send. Recording why a run was cut short — which now happens on every restart, since an interruption stops consuming the retry budget — wrote a reason that nothing cleared until the whole broadcast finished, so a send progressing perfectly well kept showing a red error from a restart that happened slices ago. The reason is now cleared when the task is claimed again, and still shown while the task is waiting to resume.

## [37.2] - 2026-08-05

- **Improvement**: A broadcast no longer dies when the connection that triggered it goes away. Task execution ran on the triggering HTTP request's context, so the dispatcher's own 53s client timeout — only 3s above a 50s broadcast slice — cancelled the run mid-batch, aborting the enqueue transaction and surfacing as `sql: transaction has already been committed or rolled back` followed by a misleading `[BROADCAST_NOT_FOUND] ... context canceled`. Execution is now bounded by the task's own deadline instead of the caller's connection, and each dispatch waits for as long as its task may legitimately run rather than a fixed 53s (which also silently truncated the 300s segment-recompute tasks).
- **Improvement**: `max_retries` is now a consecutive-failure budget instead of a lifetime one. It was never reset, and *pausing* a broadcast counted as a failed attempt, so three pause/resume cycles — or three transient failures hours apart during a long send — permanently killed a broadcast. A run interrupted by a restart or a hung-up caller no longer consumes the budget at all, and resuming a broadcast clears it.
- **Improvement**: A failed batch enqueue no longer skips recipients. The sender reported the whole batch as processed even though its single transaction had written nothing, so the orchestrator advanced past those contacts and they were never mailed — with every enqueue failing, a broadcast would march through its entire audience and still report itself as sent. The batch is now retried from the same cursor.
- **Improvement**: A broadcast interrupted past its retry limit is now paused with a reason, and resumable from where it stopped, instead of being stranded. The write that finalised its status ran on the very context whose cancellation caused the failure, so it silently did nothing and left the broadcast stuck in "sending" for good with its task dead.
- **Improvement**: An A/B test whose first sending run was cut short — a transient database error, a restart — could then send the **entire audience** as a single blast with variations mixed, instead of only the test sample, and leave the broadcast stuck in "testing" with no way forward. The run that opens a test phase records the phase in the task's state, but a failed run saves no state, and the next run had no way to recover the phase from the broadcast's own status.
- **Change**: `GET /api/cron` returns `202 Accepted` immediately instead of holding the caller open for the whole run, restoring the behaviour external cron setups relied on before in-process execution landed. A caller that gives up no longer cancels the tasks it started, and overlapping triggers are ignored while a run is in flight.

## [37.1] - 2026-08-04

- **Feature**: Amazon SES integrations can isolate their sending reputation. Notifuse provisions a tenant per integration with its own suppression list and associates the configuration set and sender identities with it, so one workspace's bounces can no longer pause sending or suppress recipients for another. Sends moved to the SES v2 API, which is the only place the tenant parameter exists. Isolation can be switched off again at any time — sends revert to the shared reputation on the next message, while the tenant keeps its suppression list for when it is switched back on. An existing tenant and configuration set can be used instead under Advanced, and everything is off by default — AWS bills per tenant per month (#400)
- **Fix**: Emails sent with both CC and BCC through Amazon SES dropped their CC recipients. The attachment path wrote a `Cc:` header but built the envelope from the To and BCC addresses only, so the CC recipients were never actually delivered to — the message looked correct in the inbox of everyone who did receive it.
- **Fix**: Amazon SES sends no longer look up the configuration set on every message. AWS allows that call once per second per account and region, so any send rate above one per second was throttled, and a throttled lookup silently sent the message with no configuration set at all — losing its delivery, bounce and complaint tracking. The name is now recorded when webhooks are registered, and the lookup that remains for older integrations is memoised and de-duplicated.
- **Fix**: Updating an email integration through the API no longer erases server-managed SES state. `workspaces.updateIntegration` replaced the whole provider object, so any client whose payload omitted them wiped the inbound topic ARN — breaking stop-on-reply — along with the configuration set and tenant names. These fields are now preserved server-side.
- **Fix**: Unregistering webhooks on an Amazon SES integration no longer deletes the configuration set when a tenant sends through it, which would have made every subsequent send fail. Deleting the integration now removes the whole set of provider resources — the configuration set and, for a Notifuse-managed tenant, the tenant itself — in the order AWS requires, so nothing is left billing after the integration is gone. Reading webhook status also no longer crashes when the configuration set exists with no event destination, which is the normal state of an operator-supplied one.

## [37.0] - 2026-08-02

- **Security**: Segment and automation conditions that filter contact-timeline events by a field value built their SQL by splicing the field name straight into the query text. A crafted `field_name` could close the quote and append arbitrary SQL, and the segment preview count turned that into a boolean oracle — any workspace member could read any table in their workspace database. JSONB keys are now bound as query parameters, and workspace migration v37 recompiles stored segment queries so already-saved segments are repaired without being re-saved.
- **Feature**: Goal conditions can be negated. A segment or automation filter can now say "has **not** purchased in the last 30 days", which also matches contacts with no matching events at all. This was previously inexpressible: the condition compiles to an aggregate grouped by contact, so a contact with zero events produced no group and could never satisfy it — meaning a "count is 0" condition silently matched nobody, and comparisons like `sum ≤ 1000` silently excluded everyone who had never converted (#399).
- **Feature**: Goal conditions can filter on the event name, the goal name and the event's own `properties` payload, instead of only the seven goal types. The goal-name filter was already supported by the API but had no input in the console (#399).
- **Feature**: Contact datetime properties gain a "not in the last X days" operator. It deliberately includes contacts whose date was never set — someone who never converted has not converted in the last 30 days either — where a plain negation would silently drop them (#399).
- **Feature**: The segment editor counts matching contacts while a condition is still open in its form, instead of only once it has been confirmed. The count refreshes on its own as the condition's inputs change, and keeps the last valid number on screen — dimmed — while a condition is too incomplete to be counted.
- **Fix**: Changing the timeframe operator of a segment or automation condition no longer carries the previous operator's values over. Switching "in date range" to "in the last X days" kept both dates, so saving failed with `in_the_last_days requires 1 value` — and coming from a single date it parsed the year, quietly meaning "in the last 2026 days". Switching the other way silently reused the range's start date. The values are now cleared when the operator changes.
- **Fix**: Custom events whose `event_name` exceeded 37 characters could not be created at all. The timeline trigger writes `custom_event.<event_name>` into `contact_timeline.kind`, which was `VARCHAR(50)`, and because the trigger runs after insert the overflow aborted the whole event ingestion. Workspace migration v37 widens the column.
- **Fix**: Workspace migration v37 could abort startup with `cannot alter type of a column used in a trigger definition`. PostgreSQL refuses to widen `contact_timeline.kind` while any trigger depends on the column, and every live automation installs one whose WHEN clause reads it — a single active automation in any workspace was enough to stop the server from booting, on every restart. The migration now drops the dependent triggers, widens the column and recreates them unchanged in the same transaction.

## [36.1] - 2026-08-01

- **Fix**: Deleting a workspace no longer revokes the application user's privileges on the *system* database. The revoke statements applied to the connected database rather than to the workspace being deleted, so afterwards authentication failed with `permission denied for table users` (superuser deployments, the default, were unaffected). Workspace databases are now removed with a single `DROP DATABASE ... WITH (FORCE)` and the system-database cleanup runs in one transaction. To repair an already-affected install, connect to the system database and run `GRANT ALL ON ALL TABLES IN SCHEMA public TO <db_user>;` plus the same for `ALL SEQUENCES` (#396)
- **Fix**: `templates.create` and `templates.update` no longer crash the request when the `email` object omits `visual_editor_tree`. Validation called a method on the missing tree and panicked, so the connection was dropped and API clients behind a reverse proxy saw a bodyless `502 Bad Gateway`; such payloads now return `400` naming the missing field and pointing to the code-mode alternative (`editor_mode: "code"` with `mjml_source`). Also applies to per-language `translations` entries (#401)
- **Fix**: Applying preview text to a template whose MJML already contains a self-closing `<mj-preview />`, or an `mj-preview` carrying attributes, no longer injects a *second* preview element. Only the plain `<mj-preview>` form was recognised, so the other two shapes were treated as "no preview present" — the attribute form shipped two preheader blocks with the stale text still in the first, and a bare `<mj-preview />` (which an `mj-liquid` block can emit) was duplicated instead of filled. Both shapes are now matched, attributes are preserved, and a self-closing tag is expanded in place (#394)
- **Fix**: Preview text containing Liquid no longer breaks sends for code-mode templates, and preview or title text that renders to a value containing `<` or `>` no longer fails compilation. Code-mode templates escaped the preview text *before* rendering it, so a feed value containing `&` (e.g. a "News & Updates" subject) aborted the whole send with an XML parse error, and Liquid comparisons such as `{% if a > b %}` silently evaluated the wrong branch because the operator had been entity-encoded. Separately, `<` and `>` were escaped with named entities that the MJML parser decodes back into markup before parsing, so an `mj-preview` or `mj-title` block whose Liquid resolved to a value containing angle brackets failed to compile (#394)
- **Fix**: Liquid in a template's *Preview Text* is rendered again at send time for visual-editor templates whose body has no `mj-preview` block — the common case for console-built templates. The preview text was spliced into the MJML after Liquid processing had already run, so expressions like `{{ global_feed.subject }}` reached recipients literally in the inbox preview text; the injected value is now rendered through Liquid first, and malformed Liquid in preview text surfaces as a compile error instead of being sent raw. Rendered preview values containing `<`, `>` or `&` no longer break the send either: the injected text is escaped with numeric character references, which survive MJML parsing where the named `&lt;`/`&gt;` entities are decoded back into markup (#394)

## [36.0] - 2026-07-10

- **Feature**: Transactional email attachments support a per-attachment `content_id` for inline images (e.g. per-recipient QR codes) — set `disposition: "inline"` plus `content_id` and reference the image from the template as `<img src="cid:...">`. Works across all email providers; inline images are assembled in a `multipart/related` MIME subtree on Amazon SES (previously they could show up as dangling attachments in Gmail/Outlook) and the SMTP provider now emits the RFC 2045 angle-bracketed `Content-ID` header that strict clients like Outlook require (#393)
- **Fix**: SMTP connections on port 465 now use implicit TLS (SMTPS). Workspace SMTP integrations, the setup wizard's "Test SMTP Connection", and system emails (magic codes, invitations, alerts) always spoke plaintext-first SMTP — waiting for a greeting that an SMTPS server never sends before negotiating STARTTLS — so a port-465 + TLS configuration hung or timed out. With TLS enabled and port 465, the client now performs the TLS handshake immediately after connecting and skips STARTTLS. TLS on any other port still negotiates STARTTLS, and `use_tls=false` remains plaintext, including on port 465. The TLS handshakes in the raw SMTP sender (both implicit and STARTTLS) are also now bounded by the dial timeout instead of hanging indefinitely on an unresponsive server (#385)
- **Fix**: OIDC sign-in no longer permanently fails with IdPs that omit the `email_verified` claim (Microsoft Entra ID, Cloudflare Access): an absent claim decoded as `false` and rejected every first login. Setting the new env-only `OIDC_ALLOW_UNVERIFIED_EMAIL=true` (default `false`) accepts first logins whose id_token omits the claim; an explicit `email_verified=false` is still always rejected, and the rejection log now names the flag to set (#388)
- **Fix**: Enabling OIDC through the setup wizard persisted `oidc_scopes="openid"` (the wizard has no scopes field), and that stored value overrode the `openid email profile` default at boot — authorize requests then lacked the email/profile scopes, so IdPs returned tokens without an email. The wizard and the settings screen now persist the full default when scopes are left empty, and the v36 system migration heals existing rows still holding the bare legacy value (when a row is healed the server restarts once so the change takes effect immediately) (#389)
- **Feature**: Segments and automation conditions can now target link clicks by broadcast and by URL. A "clicked email" timeline condition can be scoped to a specific broadcast and/or to a clicked link, where the link is matched as a case-insensitive substring of the destination URL (e.g. `/pricing` matches `https://acme.com/pricing?utm_campaign=summer`). This makes segments and automation filters like "clicked the pricing link in the Summer Sale broadcast" possible; it builds on the per-link click data already recorded in `message_history.clicked_links`, so no migration is needed (#339, #311).
- **Fix**: Viewing the contacts of a segment no longer returns a server error. The `segments.contacts` endpoint ordered by a `created_at` column that `contact_segments` does not have, so every request failed; it now orders by `matched_at` (when the contact joined the segment).
- **Fix**: Removed the "Photo URL" option from the automation *contact updated* trigger's field list — contacts have no photo-URL field, so an automation filtered on it could never fire.
- **Fix**: Segment/automation conditions that filter contact-timeline events by a field value now work. The query builder referenced a non-existent `metadata` column on `contact_timeline` (event fields live in the `changes` JSONB as `{field: {old, new}}`), so any such condition failed at SQL execution; it now reads the field's new value from `changes`.
- **Fix / Change**: The contact timeline now records email engagement events in the same dot notation as the rest of the timeline and the webhooks — `email.opened`, `email.clicked`, `email.bounced`, `email.complained`, `email.unsubscribed`, plus `email.sent` and `email.updated` — replacing the old `open_email` / `click_email` / `insert_message_history` / `update_message_history` names. This fixes automations triggered by an email engagement event, which previously never fired because the trigger listened for `email.clicked` while the timeline stored `click_email`. Workspace migration v36 renames existing timeline rows, rewrites stored segment definitions (both the tree and the compiled query) and regenerates the triggers of already-live automations, so segments and automations keep working without needing to be re-saved. The `email.sent` and `email.delivered` options were removed from the automation trigger list — they could never fire, and an `email.sent` trigger would loop an "every time" automation that sends email. (#339)
- **Security**: `golang.org/x/crypto` is pinned to 0.52.0 in both Go modules, closing the SSH-related advisories (12 critical/high) flagged by Dependabot.
- **Fix**: Automation email nodes now display the selected template's name on the canvas for non-marketing templates (e.g. `welcome`). The canvas resolved names from a reference list restricted to `category: 'marketing'`, while the node picker lets you select an email template of any category — so a validly selected non-marketing template fell back to the generic "Template set" label; the reference list is no longer category-restricted (#391).
- **Change**: Mailjet can now only be used as a transactional email provider, not a marketing one, because the Mailjet API overwrites unsubscribe links and breaks Notifuse's unsubscribe links. Newly assigning Mailjet as the marketing provider is rejected at settings-save, and marketing sends are refused at provider resolution — so workspaces that already had Mailjet assigned keep saving unrelated settings normally, but their broadcasts fail with a clear error until they switch the marketing provider.

## [35.1] - 2026-07-08

- **Security**: Bumped `golang.org/x/crypto` to 0.52.0 in both Go modules to resolve the SSH-related advisories (12 critical/high) flagged by Dependabot.

## [35.0] - 2026-07-03

- **Fix**: The contacts multi-select no longer gets out of sync when deleting via the row's ⋯ menu — clicking a dropdown item invisibly toggled the row's selection (the click bubbles through the React portal to the row's select-on-click handler), and a deleted contact was never removed from the selection, leaving a phantom "N contacts selected" bar whose stale emails a later bulk *Add to list* would even re-create via upsert (#374).
- **Feature**: Broadcast reports now show a per-link click breakdown — total clicks and unique clickers per destination URL (UTM parameters kept, since they are commonly used to tell links apart). Clicks are recorded per URL in a new `message_history.clicked_links` JSONB column added by workspace migration v35; recording starts immediately after the upgrade, including for emails already sent (the destination URL is embedded in their tracking tokens), so the breakdown only covers post-upgrade clicks and legacy `/visit` links remain aggregate-only (#339, #311).
- **Fix**: One-time authentication emails (the built-in Supabase notifications: magic link, signup confirmation, invite, recovery, email change) are no longer rewritten through the click-tracking redirect — their links previously depended on the API being up and could be consumed by mailbox link scanners before the user clicked. Transactional notifications gain a tri-state `tracking_mode` (inherit the workspace setting, or disabled — which also suppresses UTM rewriting); an absent field on update keeps the stored value so partial edits cannot wipe an opt-out, and migration v35 sets `disabled` on existing Supabase notifications.
- **Fix**: A/B-test broadcasts no longer attribute most recipients' link clicks and `{{ utm_content }}` values to the wrong variant. When no explicit `utm_content` was set, the first processed recipient's template ID was written back onto the shared broadcast object and frozen for every later recipient — and in batch sends the template data even lagged one recipient behind the rewritten links. `utm_content` is now computed per recipient from the variant actually chosen for them, and `utm_term` is applied consistently across both send paths.
- **Fix**: The automation *Add to List* node no longer fails silently. Its "Subscribed" option stored an invalid `subscribed` status that the backend rejected (only `active`/`pending` are valid), so contact journeys stalled at that node with no visible error. The editor now stores `active`, node configurations are validated when the automation is saved so an invalid status is caught immediately, and a data migration repairs existing automations, contact lists, and timeline entries that still carry `subscribed` (#376).
- **Improvement**: Concurrent template edits no longer silently overwrite each other. Template saves were last-writer-wins, so a save based on a stale revision clobbered newer changes. The editor now sends the revision it was based on and the server rejects a stale-base save with `409 Conflict`, prompting to reload the latest or overwrite. Covers the email template, blog post, and transactional-notification editors (#378).
- **Security**: Bumped the console's `echarts` dependency to 6.1.0 to resolve a cross-site scripting (XSS) advisory (GHSA-fgmj-fm8m-jvvx) flagged by Dependabot.
- **Fix**: Resolved intermittent `pq: password authentication failed` / "failed to get workspace connection" errors under load. The workspace connection manager health-check-pinged the cached pool on every query and evicted, closed, and rebuilt it (re-hitting the `postgres` admin database) on any slow ping or transient blip; it now reuses cached pools and creates them without a global lock (#380).
- **Fix**: Adding a contact (or adding a contact to a list from the details drawer) now refreshes the contacts list immediately instead of requiring a hard page reload. The "Add" contact drawer never invalidated the React Query cache on success, and the "add to list" action only refreshed the contact details — both now invalidate the contacts list (and total count) so the new contact appears right away (#364).
- **Improvement**: Faster email rendering — Liquid engines are now reused across blocks via a pool instead of being rebuilt for every block, and constant regexes are compiled once, cutting per-block Liquid processing time by ~3× (~8× under concurrent sends) during broadcasts (#381).
- **Improvement**: AI Email Designer reliability with reasoning models (e.g. DeepSeek) — thinking now streams into a collapsible panel, the preview refreshes immediately after AI edits, a generated email that doesn't compile surfaces the MJML error instead of silently looking successful, hitting the token limit shows a non-destructive warning instead of wiping the answer, and OpenAI-compatible integrations gain a reasoning-effort selector (#363).
- **Feature**: Search templates by name on the templates list — a debounced name search beside the category filter, persisted in the URL, mirroring the broadcasts search (the already-loaded list is filtered client-side).

## [34.1] - 2026-06-25

- **Fix**: Workspace SMTP integrations now connect to servers that advertise only `AUTH LOGIN` (such as Azure Communication Services) — the raw SMTP sender hardcoded `AUTH PLAIN` and was rejected with a 504 before credentials were ever checked. It now reads the AUTH mechanisms advertised in EHLO and uses LOGIN when PLAIN isn't offered, preferring PLAIN when both are available (#368).
- **Fix**: Unsubscribing from the notification center works again. The widget's "Unsubscribe" action and per-list toggle (and the console) now post to a dedicated `/unsubscribe` endpoint, while `/unsubscribe-oneclick` is reserved for the RFC 8058 mail-client one-click carried in the `List-Unsubscribe` header (it still accepts the legacy JSON body as a backward-compatible shim). When v34.0 made `/unsubscribe-oneclick` strictly RFC 8058 for the Gmail/Yahoo one-click fix, the notification center's JSON request was rejected with `400 "Invalid request"` and contacts stayed subscribed (#371).
- **Fix**: A freshly installed root account no longer crashes the console on first login. Before any workspace existed, `user.me` returned `"workspaces": null` instead of `[]` for the `ROOT_EMAIL` user — the root path returns the workspace list straight from the database, which is a nil slice when empty — and the console crashed with `Cannot read properties of null (reading 'length')` instead of redirecting to workspace creation. The repository now returns an empty (non-nil) slice so the API always serializes `[]`, and the console normalizes a null `workspaces` to an empty array as a safeguard (#367).
- **Fix**: An `mj-button` (or `mj-social`) whose inner padding was edited in the visual editor no longer vanishes in Gmail — the inner-padding object was compiled into the CSS as a Go map literal (`padding:map[bottom:0px top:0px]`) that strict clients reject; it now compiles to a valid CSS shorthand, and the editor no longer stores padding as an object (#369).

## [34.0] - 2026-06-20

- **Feature**: Single Sign-On via OpenID Connect (OIDC) alongside magic-code login — off by default and enabled per deployment with `OIDC_*` env vars, the setup wizard, or Settings → SSO (client secret encrypted at rest), so the sign-in page shows an SSO button only when it is turned on. Invited-users-only by default with opt-in just-in-time provisioning gated by a verified-email domain allowlist; identities are keyed on the durable issuer+subject pair (never email alone) and login still requires a workspace invite or `ROOT_EMAIL` for access. Uses Authorization Code + PKCE and works with any compliant provider (Google Workspace, Keycloak, Okta, …); adds the `federated_identities` system table (migration v34).
- **Fix**: One-click unsubscribe (RFC 8058) now works end-to-end. The `/unsubscribe-oneclick` endpoint takes its parameters from the `List-Unsubscribe` URL query string — it previously tried to JSON-decode the POST body and rejected every mail-client request with `400 "Invalid request body"`. The emitted URL now also carries the `email_hmac` the endpoint verifies. And the endpoint no longer applies User-Agent bot detection, which silently dropped the automated POSTs that Gmail/Yahoo/Apple (and tools like `curl`) actually send (returning `200` while leaving the contact subscribed); it instead requires the RFC 8058 `List-Unsubscribe=One-Click` body token to deflect bare prefetch/scanner POSTs (#362).
- **Feature**: Google Gemini is now a selectable LLM provider for the AI agent (blog & email generation), alongside Anthropic and OpenAI — configure it under Settings → Integrations with a Gemini API key and model (default Gemini 3.1 Pro); when multiple LLM integrations are configured, a provider dropdown in the AI chat selects which to use.
- **Feature**: Search broadcasts by name and filter by status on the broadcasts list — a grouped status filter (All/Draft/Scheduled/Sending/Sent/Failed) plus a debounced name search beside it, both persisted in the URL; the `broadcasts.list` API now accepts multiple statuses and a name search (#335).

## [33.0] - 2026-06-16

### Database Schema Changes

- Migration v33.0 (workspace): adds `message_history.smtp_message_id` (with a partial index) for reply matching, `automations.exit_on_reply`, a partial unique index on `inbound_webhook_events` to dedup replayed replies, and redefines the `track_inbound_webhook_event_changes()` trigger so inbound events of type `reply`/`auto_reply` surface on the contact timeline as `email.replied`/`email.auto_reply`. Column adds are nullable/constant-default (instant, no table rewrite).

### Features

- **Feature**: Stop-on-reply for automations (#346). When a contact replies to a sequence email, the journey stops. Replies are ingested via a new public endpoint `POST /webhooks/email/inbound?workspace_id={id}&integration_id={id}` (Mailgun first; provider-agnostic parser registry for the rest). For Mailgun, the inbound Route that forwards replies to this endpoint is now created automatically by the same **Register Webhooks** action used for delivery/bounce webhooks (only the DNS MX records remain a manual step); the route is non-preemptive (no `stop()`, lower priority) so it never silently overrides other inbound consumers on a shared Mailgun domain. The public endpoint is rate-limited per source IP and per workspace, and permanent client errors return 4xx (not 5xx) so providers don't retry-loop. Inbound mail is classified so bounces and out-of-office auto-replies never count as a reply. A genuine reply is matched to the send strictly by `In-Reply-To`/`References` → the `Message-ID` stored at send time (persisted before the email is dispatched so even an instant reply matches), which both identifies the contact and scopes the exit to the exact automation that sent the replied-to email; replies that don't match a stored send are ignored, and a replayed/duplicate reply is deduplicated so it can't re-exit a re-enrolled journey. The matched reply is recorded on the contact timeline as `email.replied` and — when the automation has **Exit on reply** enabled — exits the contact's journey (bounded to journeys entered before the reply). The stop is enforced by a layered guarantee (event-driven interrupt + an active-guarded optimistic lock on the executor's happy *and* error paths + a just-in-time guard in the email queue worker) so it holds whether the contact is mid-delay or the next email is already queued, and a concurrent exit is never resurrected by a retry. The feature is free for workspaces that don't enable it (no extra per-send queries or index maintenance).
- **Feature**: Stop-on-reply also supports **Amazon SES** — inbound replies arrive as RSA-signature-verified, topic-bound Amazon SNS notifications; **Register Webhooks** auto-provisions the SNS topic + a coexistence-safe SES receipt rule (scoped to verified identities, region-validated), and the Message-ID SES returns at send is captured and matched host-independently (#346).
- **Feature**: `ROOT_EMAIL` now accepts multiple comma/semicolon-separated emails, so a shared self-hosted instance can have more than one root administrator without displacing the first. Root-gated actions (workspace creation, system settings, root HMAC sign-in) now check list membership instead of a single equality, and a user row is created on startup for every listed root so each can sign in immediately. Matching is case-sensitive and a single email behaves exactly as before. The console System Settings drawer edits the list as tags; the setup wizard still establishes the primary (first) root. Example: `ROOT_EMAIL=alice@example.com,bob@example.com` (#361).

### Fixes

- **Fix**: Translated email templates now send with their own inbox preview text (preheader) instead of the default language's. The inbox preview is rendered from the `mj-preview` block embedded in the email tree, but the metadata-sync that stamps it from the `subject_preview` field only ran for the default template — never for translations — so a translation kept the preview value it was cloned with, even after its preview was edited and saved. The sync now stamps every language variant, and all send paths (broadcast, automation, transactional) additionally inject each resolved variant's `subject_preview` at compile time, which also corrects already-saved translations without a re-save (#359).

## [32.3] - 2026-06-01

- **Security**: Broadcast data-feed endpoints (`broadcasts.refreshGlobalFeed`, `broadcasts.testRecipientFeed`) are no longer a server-side request forgery (SSRF) vector and now require `broadcasts:write`. The data-feed fetcher used a plain HTTP client with no address validation, so any authenticated workspace member — including a read-only member — could make the server fetch an arbitrary URL (internal services, the private network, or the cloud instance metadata endpoint) and read back the JSON response. The fetcher now uses the SSRF-safe client already used for favicon detection (dial-time rejection of private/loopback/link-local/reserved ranges, redirect re-validation, DNS-rebinding protection), and both service methods enforce the same write permission as broadcast creation. Trusted self-hosted deployments that intentionally fetch feeds from their internal network can opt out with `BROADCAST_DATA_FEED_ALLOW_PRIVATE_HOSTS=true`.
- **Security**: All broadcast operations now enforce workspace permissions. Previously only create/get/refresh/test were permission-checked, so any workspace member — including a read-only member — could update, delete, schedule, pause, resume, cancel, send, and select A/B winners for broadcasts. Mutating operations now require `broadcasts:write` and listing/test-results require `broadcasts:read`; unauthorized requests receive `403 Forbidden`.
- **Fix**: The task scheduler now executes due tasks in-process when the internal scheduler is enabled (`TASK_SCHEDULER_ENABLED`), instead of dispatching them over HTTP to its own `/api/tasks.execute` endpoint. In single-instance deployments where the app cannot reach its own public URL (e.g. a pod that is itself the load balancer's backend), the self-call failed with `connection refused` and left `send_broadcast` and other tasks stuck `pending`; HTTP fan-out is still used when the scheduler is disabled (external cron).
- **Fix**: Selecting a different email node in the automation editor now refreshes the config panel — the shared template selector (`TemplateSelectorInput`) cached the first template it resolved and ignored later changes to its controlled `value`, so switching between email nodes kept showing (and appearing to edit) the first node's template (#353).
- **Fix**: Test emails sent from the template editor now honor the template's Reply-To — the `transactional.testTemplate` path built the message from the modal's options only and never fell back to the template's `reply_to`, so test emails arrived without a `Reply-To` header (real automation/broadcast/transactional sends were already unaffected); an explicit Reply-To from the modal's Advanced options still takes precedence (#355).
- **Fix**: Workspace members with the `workspace` write permission ("full access") can now manage contact custom field labels. Previously both the Settings → Custom Fields controls and the underlying save were gated to workspace **owners** only, so full-access members had no way to add or edit field labels (#354). Custom field labels are now managed via a dedicated, permission-checked endpoint `POST /api/workspaces.setCustomFieldLabels` (granular `workspace:write` instead of owner role), mirroring the template-blocks pattern. As a side effect, `workspaces.update` no longer writes custom field labels — so an owner saving general settings can no longer clobber labels set by a member.
- **Fix**: Workspace members with the `blog` write permission can now manage blog settings — enabling the blog and editing its title, SEO, pagination, and feed configuration. Previously both the Settings → Blog editor and the underlying save were gated to workspace **owners** only, so a delegated "blog manager" granted `blog:write` could publish posts and themes but could not enable the blog or change its settings. Blog settings are now managed via a dedicated, permission-checked endpoint `POST /api/workspaces.setBlogSettings` (granular `blog:write` instead of owner role), mirroring the custom-field-labels pattern. As a side effect, `workspaces.update` no longer writes blog settings — so an owner saving general settings can no longer clobber blog config set by a member.
- **Fix**: Broadcasts to a double opt-in list no longer reach contacts who never confirmed — recipients whose `contact_list` status is `pending` are now excluded from both the recipient count and the send (#344).
- **Fix**: Typing into a button's text editor in the email builder no longer puts each character on its own line — StarterKit's `TrailingNode` was enabled in the button's paragraph-less inline schema, where it falls back to `hardBreak` and appended a `<br>` after every keystroke; it is now disabled for the inline editor (#352).
- **Improvement**: `{{ workspace.base_url }}` / `{{ workspace.website_url }}` now render in the template preview — the `/api/templates.compile` endpoint injects the workspace object server-side (filling only missing keys, so historical message snapshots are preserved), so any API consumer gets it, not just the console, and the Preview tab no longer renders `website_url` as empty (#342).
- **Refactor**: Extracted shared `WorkspaceSettings.ResolveEndpoint` and `BuildWorkspaceTemplateVars` helpers, replacing ~8 duplicated copies of the tracking-endpoint resolution and `workspace` template-object construction across the send and preview paths.

## [32.2] - 2026-05-31

- **Feature**: Exposed `{{ workspace.website_url }}` in email templates — the workspace's public Website URL (trailing slash trimmed), distinct from `{{ workspace.base_url }}` (the tracking endpoint) — so templates can compose application links like `{{ workspace.website_url }}/users/verify/xxx` instead of pointing at the tracking domain (#342).

## [32.1] - 2026-05-29

- **Feature**: Exposed `{{ workspace.base_url }}` in email templates — the resolved Custom Endpoint URL (or the default API endpoint), trailing slash trimmed — so templates can compose links from relative paths like `{{ workspace.base_url }}/users/verify/xxx` (#342).
- **Security**: Bumped `liquidjs` to 10.27.0 in console to clear 6 Dependabot alerts (critical RCE, ReDoS in `strip_html`, `date` filter padding DoS, `{% render %}` `ownPropertyOnly` bypass, empty `{% for %}` renderLimit bypass, and `strip_html` newline XSS); `npm audit fix` also cleared transitive `brace-expansion` and `ws` advisories.
- **Fix**: Mailgun webhook registration no longer fails with `400` on domains shared with other services — Notifuse now merges its callback URL into each event's existing URL set via `PUT` (up to Mailgun's limit of 3 per event) instead of always `POST`ing, and unregistering removes only its own URL while preserving other consumers' (#340).

## [32.0] - 2026-05-22

### Database Schema Changes

- Migration v32.0 adds a `language` column (`VARCHAR(10) NOT NULL DEFAULT 'en'`) to the system `users` table. Existing users default to English.

### Features

- **Feature**: System emails and the console UI are now localized per user. Each user has a `language` preference — one of `en`, `fr`, `es`, `de`, `ca`, `pt-BR`, `ja`, `it` — that drives both their console UI locale and the language of the system emails (authentication code, workspace invitation, broadcast circuit-breaker alert) sent to them. The language is changed from the console language switcher and persisted via the new `POST /api/user.updateLanguage` endpoint. Magic-code emails use the recipient's language, circuit-breaker alerts use each owner's language, and workspace invitations use the inviter's language.

## [31.0] - 2026-05-19

### Database Schema Changes

- Migration v31.0 updates the `queue_contact_for_segment_recomputation` trigger function on every workspace database to short-circuit when the inserted `contact_timeline` row is itself a segment membership event (`kind IN ('segment.joined', 'segment.left')`).

### Fixes

- **Fix**: `queue_contact_for_segment_recomputation` trigger no longer re-enqueues contacts when the inserted `contact_timeline` event is itself a segment membership change (`segment.joined`/`segment.left`). Removes a self-loop where every membership write re-queued the same contact.
- **Fix**: Recurring tasks dispatched via HTTP now write `timeout_after` in UTC. The column is `TIMESTAMP WITHOUT TIME ZONE` and the scheduler compares it against `time.Now().UTC()`; on non-UTC hosts the local-time value caused the task to appear "still running" for the host's UTC offset. Same fix applied to the broadcast-pause `next_run_after`.
- **Fix**: `GetWorkspaceConnection`'s pool health check now uses an isolated context for `pool.PingContext` instead of the caller's. A caller-context cancellation no longer triggers pool eviction.

## [30.3] - 2026-05-14

- **Fix**: UTM parameters (`utm_source`, `utm_medium`, `utm_campaign`, `utm_content`, `utm_term`) were dropped from tracked links when click tracking was enabled — the encrypted `/r/` redirect token embedded the raw destination URL instead of the UTM-augmented one. The UTM parameters are now preserved in the redirect target.

## [30.2] - 2026-05-13

- **Fix**: SES `4.4.7 Message expired` (retry-exhaustion) now suppresses on the first event, and any recipient that accumulates 5 consecutive soft bounces with no successful delivery in between is also suppressed; `MessageTooLarge`/`ContentRejected`/`AttachmentRejected` never count (#323).
- **Fix**: Email AI Assistant `setEmailTree` tool now declares `items` on its `children` array schema, so OpenAI-compatible providers no longer reject the request with `array schema missing items` (#324). Anthropic was already lenient about this; only OpenAI-compatible endpoints surfaced the error.
- **Improvement**: `/api/templates.compile` now accepts and returns `subject` and `subject_preview`, rendered through the same Liquid engine used at send time. Previously the API only returned `mjml`/`html`, so the console preview drawer rendered the subject in-browser with `liquidjs`, which could diverge from the Go-side `liquidgo` output used by the send pipeline. Any API consumer can now retrieve the rendered subject directly (#329).
- **Deps**: Bumped `liquidjs` to 10.25.7, `postcss` to 8.5.14, `fast-xml-parser` override to ≥5.8.0 (+ new `fast-xml-builder` ≥1.1.7 override), and `github.com/prometheus/prometheus` to v0.311.3.

## [30.1] - 2026-04-27

- **Security**: Bumped `go.opentelemetry.io/otel` to v1.41.0 in `telemetry/go.mod` (CVE-2026-29181).
- **Deps**: Bumped `gomjml` to v0.12.0.

### Breaking Changes

- **SMTP auth with `SMTP_USE_TLS=false`**: When TLS is explicitly disabled, the SMTP client now uses `PLAIN-NOENC` (go-mail's `SMTPAuthPlainNoEnc`) explicitly instead of `SMTPAuthAutoDiscover`. Previously, go-mail's auto-discover refused `PLAIN`/`LOGIN` over an unencrypted connection (only `SCRAM-SHA-*` and `CRAM-MD5` were tried), and `SMTPAuthPlain` itself also refused unencrypted connections at the AUTH step. `PLAIN-NOENC` bypasses both gates while sending the standard `AUTH PLAIN` command on the wire, so any server that advertises `AUTH PLAIN` (e.g. local maddy/Mailpit relays) accepts it. Operators who have set `SMTP_USE_TLS=false` have already accepted plaintext credential transit, so forcing `PLAIN` aligns with their stated intent. **Action**: none if your relay accepts `PLAIN`. If your relay only accepts `SCRAM`/`CRAM-MD5`, you must enable TLS (`SMTP_USE_TLS=true`) — auto-discover continues to apply when TLS is on.

## [30.0] - 2026-04-23

### Breaking Changes

- **Webhooks**: Signatures now conform to the [Standard Webhooks](https://github.com/standard-webhooks/standard-webhooks/blob/main/spec/standard-webhooks.md) specification and match the published verification code in the docs (#318)
  - Stored secrets are now prefixed with `whsec_` and the 32 random bytes after the prefix are base64-decoded before use as the HMAC key (previously the 44-char base64 string was used directly as raw bytes, making the published Python/JS/Go/PHP verification snippets always fail)
  - V30 migration rotates every existing webhook secret to the new format. Subscriptions, URLs, event filters, enabled state, and delivery history are preserved; only the secret value changes
  - **Consumer action required**: copy the new `whsec_…` secret from the console into your environment and update your verification code to the spec-compliant form shown in the docs. Deliveries that fire during the gap will retry automatically once the consumer's secret is updated

### Data migration

- **Timezone**: `Europe/Kiev` is rewritten to the IANA-canonical `Europe/Kyiv` across `workspaces.settings`, `contacts.timezone`, `segments.timezone`, and `broadcasts.schedule.timezone`. Stored `Europe/Kiev` continued to resolve at runtime via Go's tzdata alias, but the console dropdown (which no longer lists the obsolete name) showed an empty selection for affected rows. The `contacts` triggers are briefly disabled around the rename so it does not emit `contact.updated` webhook events or fill `contact_timeline` with rename entries.

### Other changes

- **Task dispatch no longer swallowed by auth proxies (#320, #317)**: The scheduler's internal `POST /api/tasks.execute` client now refuses to follow redirects (`CheckRedirect = http.ErrUseLastResponse`). Previously, an auth-walling reverse proxy (Cloudflare Access, Authelia, oauth2-proxy, Traefik Forward Auth, etc.) sitting in front of the API could respond with a 302 to its login page; Go's default `http.Client` followed the redirect as a GET, the login page returned 200 OK HTML, and the dispatcher logged "dispatched successfully" while the task never ran. The 302 is now surfaced in the non-200 branch (with the `Location` header logged) so the misconfiguration is loud instead of silent. **Operator note**: if your ingress performs an HTTP→HTTPS redirect on the API path, set `$API_ENDPOINT` to the final HTTPS URL — the dispatch client will no longer silently upgrade it.
- **Task scheduler**: Scheduler tick no longer waits on in-flight HTTP dispatches, so one slow recurring task can't delay dispatch of others on the same tick. Stale tasks left in `running` with an expired `timeout_after` are now reclaimed by `MarkAsRunningTx` on a subsequent tick instead of looping on 409 indefinitely (#317).
- **Task dispatch observability**: The scheduler-side "Task execution request dispatched successfully" log now includes the HTTP `status_code`, and `tasks.execute` logs an entry line on the handler side. Diffing the two streams makes any remaining silent-interception failure mode visible.
- **Feature**: Added AWS region `eu-central-2` (Europe, Zurich) to the S3 provider and integrations region selectors (#316).

## [29.5] - 2026-04-20

- **Feature**: Pause, resume, and cancel broadcasts mid-delivery — even after the orchestrator has finished enqueueing — and cancel is now allowed from the Processing state (#303)
- **Contacts**: Added in-table bulk actions (multi-select delete, add to list, remove from list, unsubscribe) with progress modal and "Skipped" tagging for no-op cases (#299)

## [29.4] - 2026-04-15

- **Feature**: Added `SMTP_BRIDGE_TLS` setting (`off` / `starttls` / `implicit`) to let operators run the SMTP bridge behind a TLS-terminating reverse proxy or in implicit-TLS (SMTPS) mode (#314)
- **Feature**: Blog RSS 2.0 and JSON Feed 1.1 syndication — automatic `/feed.xml` and `/feed.json` endpoints per workspace, per-category feeds, conditional GET with ETag, gzip, XSS-sanitized content, autodiscovery `<link>` tags, and admin-configurable feed settings
- **i18n**: Notification center confirmation banner (subscribe/unsubscribe result) is now translated in all supported languages instead of always showing English (#315)
- **Security**: Bumped transitive `github.com/prometheus/prometheus` from v0.35.0 to v0.311.2 to clear Dependabot alert for CVE-2026-40179 (stored XSS in Prometheus web UI; Notifuse only imports `model/value`, so it was not exploitable)

## [29.3] - 2026-04-12

- **Fix**: Double opt-in confirmation link now correctly transitions contacts from Pending to Active instead of resending the confirmation email in a loop (#313)

## [29.2] - 2026-04-08

- **Feature**: Added OpenAI as LLM provider alongside Anthropic — supports any OpenAI-compatible endpoint (OpenRouter, Ollama, vLLM, LiteLLM, Azure, etc.) via custom base URL, with full streaming and tool use support
- **Security**: Updated liquidjs to 10.25.5 in console and vitest to 3.2.4 in notification center to fix 5 Dependabot vulnerabilities
- **Deps**: Updated @vitejs/plugin-react to 5.2.0 in console and notification center
- **Feature**: Added System Settings drawer for root admin to view and edit system configuration from the dashboard
- **Workspace**: Enforce workspace creation limits via `MAX_WORKSPACES` env var (0 = unlimited), with "upgrade your plan" messaging in the console
- **Improvement**: Email open tracking now works independently of click tracking — added Cache-Control headers to prevent proxy caching, encrypted tracking URLs (`/t/`, `/r/`) to avoid pixel blocker detection, and padded tracking pixel (#307)
- **i18n**: Added Polish language support to the notification center

## [29.1] - 2026-04-07

- **Security**: Upgraded Vite to 7.3.2 in console and notification center to fix arbitrary file read via WebSocket (CVE-2026-39363)
- **Fix**: Removed invalid `visibility` attribute from MJML section output that caused template compilation errors (#305)
- **Fix**: Automation now exits when contact is unsubscribed/bounced/complained for marketing emails, while still allowing transactional emails to be sent (#304)
- **Fix**: Social media buttons now link directly to pages by default instead of wrapping URLs in share prompts; added "Share link" toggle to social element settings (#306)

## [29.0] - 2026-04-04

### Breaking Changes

- **Rename**: "SMTP Relay" renamed to "SMTP Bridge" throughout the application
  - Environment variables: `SMTP_RELAY_*` renamed to `SMTP_BRIDGE_*` (old names still accepted for backward compatibility)
  - Database settings keys migrated automatically via V29 migration
  - JSON API: `smtp_relay_*` fields renamed to `smtp_bridge_*` in setup endpoints
  - Frontend routes: `/settings/smtp-relay` changed to `/settings/smtp-bridge`
  - UI labels: "SMTP Relay" changed to "SMTP Bridge"

- **Workspace**: Enforce team member limits via `MAX_USERS` env var (0 = unlimited), with checks on invite, accept invitation, and direct add — API key users are excluded from the count

- **Security**: Fixed SSRF vulnerability in `/api/detect-favicon` endpoint by adding a safe HTTP client with private IP blocking, DNS rebinding protection, scheme validation, and response size limits
- **Security**: Upgraded happy-dom to 20.8.9 in notification center and picomatch to 4.0.4 in console
- **Improvement**: SMTP EHLO hostname now defaults to the from-email domain instead of the SMTP host, improving compatibility with strict providers (#301)
- **Security**: Updated lodash/lodash-es to 4.18.x, brace-expansion to 5.0.5, and yaml to 2.8.3 to fix prototype pollution, code injection, ReDoS, and stack overflow vulnerabilities

## [28.4] - 2026-03-27

- **Security**: Upgraded picomatch to 4.0.4 in notification center
- **Contacts**: Fixed dropdown menu becoming unresponsive after deleting contacts, and pagination state now persists in URL across page refreshes (#294)
- **Templates**: Test emails now load the full contact record, so Liquid variables like `{{ contact.first_name }}` render correctly

## [28.3] - 2026-03-20

- **Security**: Upgraded google.golang.org/grpc to v1.79.3
- **Security**: Upgraded fast-xml-parser to v5.5.8

## [28.2] - 2026-03-17

- **Postmark**: Added configurable Message Stream support, allowing Postmark to be used for both transactional (`outbound`) and broadcast/marketing emails (#289)
- **Broadcasts**: Fixed MJML code mode templates failing with "template missing content" error when sending broadcasts
- **Contacts**: Fixed `/api/contacts.list` rejecting partial email searches with "invalid email format" error. The `email` filter now accepts partial strings for substring matching as intended (#292)

## [28.1] - 2026-03-09

- **Transactional Emails**: Added `subject_preview` override to `email_options`, allowing dynamic email preheader text per API call with Liquid templating support
- **Templates**: Added language selection to "Send Test Email" modal and "Preview Template" drawer, allowing users to preview and test translated email variants
- **Demo**: Demo workspace now includes French and Spanish translations for all 4 email templates, showcasing the multi-language feature
- **Templates**: Downloaded template files now use the template's name as filename instead of a generic name (#286)
- **Email Builder**: Added `mj-liquid` block type for embedding raw MJML+Liquid code in the visual editor, enabling dynamic structural content like for-loops generating columns or conditional sections
- **Security**: Upgraded liquidjs to 10.25.0

## [28.0] - 2026-03-05

- **Templates**: Added option to choose between visual email builder or MJML code editor when creating templates
- **Templates**: Added ability to translate email templates to languages configured in workspace settings
- **Contacts**: Fixed invalid "Blacklisted" status option in change status dropdown, replaced with valid "Bounced" and "Complained" statuses (#285)
- **SMTP**: Added configurable EHLO hostname for SMTP connections. Some SMTP servers reject `EHLO localhost`; users can now set a custom hostname (e.g., their domain) via the `SMTP_EHLO_HOSTNAME` env var, setup wizard, or workspace integration settings. Defaults to the SMTP host value when empty.
- **Transactional Notifications**: Fixed delivery stats (sent, delivered, failed, bounced) always showing 0 by linking messages to their originating notification via a new `transactional_notification_id` column
- **Email Builder**: Fixed `<mj-attributes>` global styles not applying in preview and sent emails (#282)

## [27.4] - 2026-03-01

- **Notification Center**: Fixed browser language auto-save overwriting contact's manually chosen language. Now only auto-detects when contact has no language set.
- **Email Builder**: Fixed missing MJML component defaults (border-radius, borders, background, direction, textAlign) causing styles not to render in the WYSIWYG editor
- **Transactional Emails**: Added `subject` override to `email_options`, allowing dynamic email subject lines per API call with Liquid templating support (#281)

## [27.3] - 2026-02-28

- **Segments**: Added template filter to email activity conditions, allowing segments like "opened template X at least 3 times"
- **SMTP**: Fixed TLS override option not working for system emails (magic codes, invitations, alerts). The "Use TLS" toggle was ignored, causing certificate errors on local SMTP relays (#275)
- **Automations**: Fixed automation emails rendering `{{ notification_center_url }}` and `{{ unsubscribe_url }}` as empty strings by using the shared template data builder (#279)
- **Segments**: Fixed race condition where background task execution could pick up unrelated tasks, causing flaky segment recompute behavior
- **Security**: Upgraded rollup to 4.59.0 and minimatch to 10.2.4

## [27.2] - 2026-02-21

- **Contacts**: Fixed panic (502) when calling `/api/contacts.list` without the `limit` parameter (#264)
- **Security**: Upgraded fast-xml-parser to 5.3.6
- **Email Builder**: Fixed block toolbar not appearing on divider blocks and improved toolbar positioning
- **Setup**: Strip trailing slash from API endpoint to prevent double-slash URLs breaking sign-in (#266)
- **Email Builder**: Fixed Liquid template variables (e.g. `{{ contact.email }}`) not rendering in preview due to `&nbsp;` entities inserted by Tiptap v3 (#267)
- Update Anthropic Sonnet model from `claude-sonnet-4-5-20250929` to `claude-sonnet-4-6`

## [27.1] - 2026-02-14

- **Automations**: Per-email integration override — choose which email provider sends each email node, for IP warming and load distribution (#257)
- Update Anthropic Opus model from `claude-opus-4-5-20251101` to `claude-opus-4-6`
- **Segments**: Allow `count_value=0` in activity conditions to support "never did X" segments (#249)
- **Security**: Upgraded markdown-it to 14.1.1

## [27.0] - 2026-02-07

### New Features

- **Broadcast Data Feeds**: Added external data feed integration for broadcasts, allowing dynamic content injection from external APIs
  - **Global Feed**: Fetch data once before broadcast starts, available to all recipients via `{{ global_feed.* }}` template variable
  - **Per-Recipient Feed**: Fetch personalized data for each recipient via `{{ recipient_feed.* }}` template variable
  - Custom HTTP headers support for API authentication
  - Automatic retry with circuit breaker protection
  - SSRF protection with URL validation (blocks localhost and private IPs)
  - Real-time feed testing from the broadcast editor

### Database Migration

- Added `data_feed` JSONB column to `broadcasts` table (workspace migration)

### Fixes

- **SMTP M365 OAuth2**: Fixed XOAUTH2 authentication to use sender email instead of fixed auth email, resolving SendAs permission errors and incorrect Sent folder placement (#250)
- **File Manager**: Sanitize uploaded filenames by replacing spaces with dashes and lowercasing extensions (#252)
- **Broadcasts**: Fixed custom endpoint URL not propagating to click tracking and open tracking URLs in sent emails (#254)
- **Email Builder**: Fixed text color/background changes not applying when re-selecting already-styled text in the rich text editor
- **Contacts**: Bulk import now processes contacts in batches of 500, fixing crashes on large imports and improving performance

## [26.15] - 2026-01-31

- **Contacts**: Emails are now normalized to lowercase on import to prevent case-sensitivity issues (#231)
- **Security**: Upgraded fast-xml-parser to 5.3.4

## [26.14] - 2026-01-30

- **Email Builder**: Fixed buttons with HTML content like `<strong>` rendering as default "Button" text instead of custom content (#242, [gomjml PR#33](https://github.com/preslavrachev/gomjml/pull/33))

## [26.13] - 2026-01-25

- **Segments**: Added real-time validation when creating segments to detect duplicate IDs before submission (#243)
- **Email Builder**: Fixed clicking image blocks with links navigating away instead of allowing block selection (#239)
- **Email Builder**: Fixed popover buttons overflowing in certain languages by using auto-width with minimum constraint (#240)
- **Amazon SES**: Fixed Notifuse message ID extraction from webhook

## [26.12] - 2026-01-24

- **Transactional Notifications**: Added card-based UI with delivery stats (sent, delivered, failed, bounced) and period selector (7D/30D/60D)
- **Email Builder**: Fixed clicking links in mj-text blocks navigating away instead of allowing text editing
- **Console i18n**: Added internationalization support with translations for English, French, Spanish, German, Catalan, Brazilian Portuguese, Italian, and Japanese
- **File Manager**: Increased actions column width and simplified new folder modal

## [26.11] - 2026-01-24

- **Broadcasts**: Fixed crash on Broadcasts page when `variations` is null instead of empty array (#233)
- **SMTP Email**: Fixed URL corruption in emails where dots were stripped after quoted-printable line breaks by adding proper SMTP dot-stuffing

## [26.10] - 2026-01-23

- **Automations**: Email node template selector now shows all template categories instead of only marketing templates
- **File Manager**: Added warning modal when selecting images larger than 200KB from email editor to prevent slow email loading times
- **SES Email**: Fixed incorrect quoted-printable encoding in raw emails causing Gmail to break rendering for broadcasts with List-Unsubscribe headers or attachments (#230)
- **Email Builder**: Fixed Raw HTML block content not appearing in preview or sent emails (#229)

## [26.9] - 2026-01-21

- **Contacts**: Fixed CSV import not trimming non-breaking spaces (NBSP) from email addresses and string fields (#223)
- **Email Builder**: Fixed MJML export stripping Liquid template syntax like `{{contact.external_id}}` from links (#225)
- **Email Builder**: Preview now shows `[undefined: varName]` debug message when Liquid variables are missing from template data (#226)

## [26.8] - 2026-01-19

- **File Manager**: Added explicit "Select" button in actions column when opened from email editor, replacing row-click selection behavior
- **File Manager**: Fixed files with spaces in filename breaking image delivery by URL-encoding path segments (#209)
- **Email Builder**: Fixed MJML template import failing when content contains HTML tags like `<br>` or entities like `&nbsp;` (#218)
- **Email Builder**: Fixed link editing in text blocks requiring exact text selection to update URL

## [26.7] - 2026-01-18

- **MJML Engine**: Switched from `mjml-go` to `gomjml` v0.10.0 for MJML-to-HTML compilation
- **File Manager**: Fixed drop zone overlay staying active when dragging files out without dropping
- **Contacts**: Added CSV export with support for current page filters and all contact fields

## [26.6] - 2026-01-18

### Improvements

- **Broadcast Progress UX**: Added progress bar with ETA, clearer status badges, and tooltips to show sending progress (#185)
- **File Manager**: Added single file download, bulk delete, and multi-file ZIP download for selected files (#201)
- **File Manager**: Folder navigation now syncs with browser URL, enabling back/forward navigation and deep linking to folders

## [26.5] - 2026-01-18

### New Features

- Added **SMTP OAuth2 authentication** method with support for Microsoft 365 and Gmail SMTP servers (#184)
- Added **Twilio SendGrid email provider** integration (#178)

### Bug Fixes

- Fix resubcribe to private lists in notification center
- Fix "custom CSS" edit when imported MJML templates have missing default attributes
- Fix email editor switch settings (fullWidth, fluidOnMobile) not persisting after page refresh (#206)

## [26.4] - 2026-01-15

### Bug Fixes

- **Supabase Integration**: Fixed "Before User Created" webhook URL mismatch causing HTTP 405 errors (#198)
- **Email Builder Image Width**: Fixed invalid MJML generated when adding images via the editor. The default `width="100%"` is not valid for mj-image (only px values allowed). Images now default to filling container width when no explicit width is set (#196)

## [26.3] - 2026-01-13

### Bug Fixes

- **Email Builder Column Width**: Fixed columns displaying with zero width in WYSIWYG editor, causing text to wrap character-by-character

## [26.2] - 2026-01-13

### Bug Fixes

- **Email Reply-To Header**: Fixed Reply-To address from email templates not being included in automation and broadcast queue emails (#193)

## [26.1] - 2026-01-13

### Bug Fixes

- **Supabase Integration**: Fixed incorrect JSON field mapping for new email address in email_change webhooks. Changed `email_new` to `new_email` to match Supabase Auth API specification.

## [26.0] - 2026-01-12

### Bug Fixes

- **Automation Email Templating**: Fixed `{{ contact.first_name }}` rendering as `&{John false}` instead of `John` in automation emails. Contact data is now properly converted before passing to the Liquid template engine.
- **List Status Values**: Fixed `add_to_list` automation nodes using invalid `subscribed` status instead of `active`
- **Automation Stats**: Fixed automation stats being reset to 0 when updating automation

### Migration

- **v26**: Automatically fixes `subscribed` → `active` in contact_lists, automation nodes, and timeline entries. Recomputes automation stats from actual data.

## [25.1] - 2026-01-11

### Bug Fixes

- **Automation Terminal Delay Nodes**: Fixed bug where contacts at terminal delay nodes (delay node with no next node) were incorrectly marked as "completed" instead of staying "active" while waiting for the delay to expire
  - Contacts now correctly wait at terminal delay nodes until scheduled time, then complete

### Testing Improvements

- Fixed invalid delay node configurations in automation e2e tests (invalid unit "seconds" and duration 0)
- Added `waitForStatsCompleted()` helper for polling-based stats verification to fix test flakiness
- Improved test stability for automation scheduler timing edge cases

## [24.0] - 2026-01-10

### Bug Fixes

- **Automation Enrollment Failure**: Fixed "cannot set path in scalar" error when enrolling contacts in automations where the `stats` field contained a JSONB scalar value instead of an object
  - Migration v24 fixes existing automations by setting `stats = '{}'` where `jsonb_typeof(stats) != 'object'`

## [23.0] - 2026-01-09

### Bug Fixes

- **Automation Triggers Not Firing**: Fixed critical bug where automations with "List / Subscribed" trigger never fired for workspaces created after v18 (#190)
  - Root cause: `init.go` had outdated trigger functions producing generic event kinds (`insert_contact_list`) instead of semantic event kinds (`list.subscribed`)
  - New workspaces only got `InitializeWorkspaceDatabase()` without running migrations, so they never received the v18 trigger updates
  - V23 migration reinstalls correct trigger functions on ALL existing workspaces
  - Updated `init.go` to use semantic event kinds for new workspaces
  - Fixed e2e test that was masking the bug by manually inserting timeline events

### Database Schema Changes

- Migration v23.0 reinstalls three PostgreSQL trigger functions with semantic event naming:
  - `track_contact_list_changes()`: `insert_contact_list` → `list.subscribed`, `list.confirmed`, etc.
  - `track_contact_segment_changes()`: `join_segment` → `segment.joined`, `leave_segment` → `segment.left`
  - `track_contact_changes()`: `insert_contact` → `contact.created`, `update_contact` → `contact.updated`

## [22.6] - 2026-01-06

### Bug Fixes

- **SMTP Multi-line Banner**: Fixed SMTP connections failing with "EHLO rejected with code: 220" error when server sends multi-line 220 greeting banner (RFC 5321 compliant fix for #183)

## [22.5] - 2026-01-06

### Bug Fixes

- **Segment Date Filters**: Fixed date picker sending dates in wrong format (`YYYY-MM-DD HH:mm:ss` instead of ISO8601). Frontend now sends proper RFC3339 format for all segment date filters including contact fields, timeline timeframes, and custom events goal timeframes (fixes #182)

## [22.4] - 2026-01-06

### Bug Fixes

- **Broadcast Emails**: Fixed regression from v21.0 email queue system where system template variables (`{{ unsubscribe_url }}`, `{{ notification_center_url }}`, `{{ broadcast.name }}`, etc.) were not rendering in broadcast emails (fixes #180)
- **Message History**: Fixed regression where template data was empty in message history for queue-based sends

## [22.3] - 2026-01-06

### Bug Fixes

- **Automation Flow Editor**: Fixed stale closure bug causing nodes to disappear when adding children to ListStatusBranch handles (fixes #179)

## [22.2] - 2025-12-31

### Features

- **Programmatic Root Authentication**: New `/api/user.rootSignin` endpoint for CI/CD and automation
  - HMAC-SHA256 signature authentication using existing `SECRET_KEY`
  - 60-second timestamp window to prevent replay attacks
  - Rate limited (5 attempts per 5 minutes)

## [22.1] - 2025-12-29

### Features

- **Email AI Assistant**: AI-powered design assistant for email templates
  - Streaming chat with Anthropic Claude models
  - Tool use for modifying email structure, blocks, and content
  - Server-side web scraping and search for content inspiration
  - Auto-expand tree to selected block for better navigation

### Improvements

- **AI Assistant UX**: Both Email and Blog AI assistants now show a helpful setup prompt when the Anthropic integration is not configured, guiding users to the integration settings
- **Unified AI Assistant codebase**: Refactored Email and Blog AI assistants to share common code, reducing duplication by ~60% and ensuring consistent behavior
- **Consistent AI Assistant styling**: Both assistants now use the same color scheme for a unified experience

## [22.0] - 2025-12-28

### Features

- **Blog AI Assistant**: AI-powered writing assistant for blog posts

  - Streaming chat with Anthropic Claude models (Opus, Sonnet, Haiku)
  - Tool use for updating blog content and metadata directly in editor
  - Server-side web scraping and search via Firecrawl integration
  - Session cost tracking with input/output token breakdown

- **LLM Integration**: Anthropic API support with encrypted API key storage
- **Firecrawl Integration**: Web scraping (`scrape_url`) and search (`search_web`) tools

## [21.0] - 2025-12-23

### Database Schema Changes

- Migration v21.0 introduces the email queue system:
  - `email_queue` table for unified broadcast and automation email delivery
  - Added `enqueued_count` column to `broadcasts` table
  - Migrated broadcast statuses: `sending` → `processing`, `sent` → `processed`

### Features

- **Email Queue System**: Centralized queue for all outbound marketing emails

  - Unified delivery for broadcasts and automations
  - Priority-based processing with retry logic
  - Per-integration rate limiting
  - Background worker with graceful shutdown

- **Automation Performance**: Single-tick execution optimization

  - Process multiple nodes per scheduler tick until delay or completion
  - 10-node safety limit per tick prevents runaway loops
  - State persisted after each node for crash recovery

- **Automation Timeline Events**: Track contact journey lifecycle
  - `automation.start` event on enrollment
  - `automation.end` event on completion/exit/failure

### Breaking Changes

- Broadcast statuses renamed: `sending` → `processing`, `sent` → `processed`

## [20.0] - 2025-12-21

### Database Schema Changes

- Migration v20.0 introduces the automations system with 4 new workspace tables:
  - `automations` - Workflow definitions with trigger config, nodes, and statistics
  - `contact_automations` - Tracks each contact's journey through automations
  - `automation_node_executions` - Audit log of node executions for debugging
  - `automation_trigger_log` - Trigger event logging

### Features

- **Marketing Automations**: Visual workflow builder for automated contact journeys

  - Event-driven triggers from contact timeline (contact, list, segment, email, custom events)
  - Trigger frequency control: `once` (first occurrence) or `every_time`
  - Conditional triggers using segment filter conditions
  - Field-specific triggers for contact updates (e.g., trigger only when `custom_string_1` changes)

- **Automation Node Types**:

  - **Trigger**: Entry point based on timeline events with configurable conditions
  - **Delay**: Pause workflow for minutes, hours, or days
  - **Email**: Send templated emails using workspace email provider with tracking
  - **Branch**: Conditional branching with multiple paths based on segment conditions
  - **Filter**: Pass/fail routing based on contact attributes
  - **Add to List**: Subscribe contacts to additional lists
  - **Remove from List**: Unsubscribe contacts from lists
  - **A/B Test**: Deterministic variant selection using FNV-32a hashing for consistent splits
  - **Webhook**: POST contact data to external URLs with authorization headers

- **Visual Flow Editor**: Drag-and-drop canvas for designing automation workflows

  - Node positioning with visual connections
  - Type-specific configuration panels
  - Real-time validation

- **Automation Lifecycle Management**:

  - Draft mode for building and testing
  - Activate to go live (creates PostgreSQL triggers)
  - Pause to stop new enrollments while preserving in-progress journeys
  - Soft-delete with recovery capability

- **Contact Journey Tracking**:

  - Full audit trail of node executions with timestamps and duration
  - Contact status tracking (active, completed, exited, failed)
  - Exit reasons for debugging
  - Node execution output logging

- **Execution Engine**:

  - Background scheduler polling every 10 seconds
  - Batch processing (50 contacts per batch)
  - Round-robin workload distribution across workspaces
  - Retry logic with exponential backoff (max 5 retries)
  - Graceful shutdown handling

- **Statistics Dashboard**:

  - Enrolled contacts count
  - Completed journeys
  - Exited contacts (with reasons)
  - Failed executions

- **API Endpoints** (`/api/automations.*`):
  - `create`, `get`, `list`, `update`, `delete`
  - `activate`, `pause` for lifecycle management
  - `nodeExecutions` for contact journey audit trail

## [19.6] - 2025-12-19

- Fix: SMTP integration now works with strict SMTP servers (#172)
  - Replaced go-mail SMTP client with raw SMTP command implementation
  - MAIL FROM command no longer includes BODY=8BITMIME or SMTPUTF8 extensions that caused "501 5.5.4 Syntax error in parameters" errors
  - Message composition (MIME, headers, attachments) still handled by go-mail

## [19.5] - 2025-12-16

- Fix: Task completion now saves final state to prevent stale progress display in UI (#157)
- Fix: Prevent concurrent task execution race condition that could cause duplicate broadcast emails
- Fix: Unsubscribes via notification center link are now tracked in broadcast statistics (#165)
- Fix: Email builder now respects column width attributes in section blocks
- Fix: Contact bulk import now handles duplicate emails in a single batch (#167)
- Fix: Image alt text now supports Liquid template variables (#168)

## [19.4] - 2025-12-12

- Fix: Broadcast recipient count mismatch - `CountContactsForBroadcast` now filters soft-deleted lists consistently with `GetContactsForBroadcast`
- Fix: Contact list pagination now uses nanosecond precision timestamps to prevent skipping contacts created within the same second (#159)
- Fix: Broadcast delivery now uses deterministic ordering (`created_at ASC, email ASC`) to prevent skipping contacts with identical timestamps during bulk imports (#157)
- Enhancement: SES configuration set is now optional for transactional emails
- Fix: `mailto:` links are no longer tracked (prevents broken email client links)

## [19.3] - 2025-12-09

- Fix SES non-ASCII characters in email local part (e.g., `Añejandramendo@gmail.com`) now encoded with RFC 2047

## [19.2] - 2025-12-04

- fix update contact form
- fix non-ASCII characters in SES
- fix team table overflow when email is too long (#149)
- add TLS switch to setup wizard SMTP settings

## [19.1] - 2025-12-02

- Enhancement: Added `full_name` field to the Add Contact drawer

## [19.0] - 2025-12-01

### Features

- **Outgoing Webhooks**: Subscribe to workspace events and receive HTTP notifications
  - CRUD API for webhook subscriptions (`/api/webhookSubscriptions.*`)
  - Event types: `contact.*`, `email.*`, `list.*`, `segment.*`, `custom_event.*`
  - HMAC-SHA256 signature verification (Standard Webhooks spec)
  - Automatic retries with exponential backoff (up to 10 attempts over 24h)
  - Custom event filters for fine-grained subscription control
  - Test webhook endpoint for integration debugging
  - Delivery logs with 7-day retention
- **Contact `full_name` field**: Native field for systems without separate first/last names

### Bug Fixes

- Fixed timeline timestamps showing incorrect times (now uses `CURRENT_TIMESTAMP` in triggers)
- Fixed JSON field filters with number/time values failing validation (#140)
- Fixed contact scan error on migrated databases due to column ordering mismatch (replaced `SELECT *` with explicit column list)

### Breaking Changes

- Renamed `webhook_events` table to `inbound_webhook_events` to distinguish from outgoing webhooks

## [18.3] - 2025-11-30

- Fix: SEO settings not being persisted when creating a new blog post
- Enhancement: Featured image thumbnail now displays in the Title column with a popover for full-size preview in the blog posts list

## [18.2] - 2025-11-29

### Changes

- **API Endpoints**: Renamed custom events endpoints from singular to plural for consistency
  - `POST /api/customEvent.upsert` → `POST /api/customEvents.upsert`
  - `POST /api/customEvent.import` → `POST /api/customEvents.import`
  - `GET /api/customEvent.get` → `GET /api/customEvents.get`
  - `GET /api/customEvent.list` → `GET /api/customEvents.list`

## [18.1] - 2025-11-29

### Enhancements

- **File Manager**: Added support for modern image formats (WebP, AVIF, JPEG XL)
  - `.webp` files now display with proper image previews
  - `.avif` files now display with proper image previews
  - `.jxl` (JPEG XL) files now display with proper image previews

## [18.0] - 2025-11-29

### Database Schema Changes

- Migration v18.0 introduces custom events tracking system
- Removed deprecated contact fields: `lifetime_value`, `orders_count`, `last_order_at`
- Renamed contact timeline event kinds to semantic dotted format:
  - Contact: `insert_contact` → `contact.created`, `update_contact` → `contact.updated`
  - Lists: `insert_contact_list` → `list.subscribed`/`list.pending`, status changes → `list.confirmed`/`list.resubscribed`/`list.unsubscribed`/`list.bounced`/`list.complained`
  - Segments: `join_segment` → `segment.joined`, `leave_segment` → `segment.left`
- Added `custom_events` table for tracking user behavior and goals
- Added computed fields in segmentation engine for custom events goal aggregations

### Features

- **Custom Events API**: Track user behavior and conversion goals

  - `POST /api/customEvents.upsert` - Create or update a single event
  - `POST /api/customEvents.import` - Batch import up to 50 events
  - `GET /api/customEvents.get` - Retrieve event by workspace, event name, and external ID
  - `GET /api/customEvents.list` - List events by email or event name
  - Goal tracking with types: `purchase`, `subscription`, `lead`, `signup`, `booking`, `trial`, `other`
  - Soft-delete support via `deleted_at` field

- **Segmentation with Custom Events Goals**: Build segments based on goal aggregations
  ```json
  {
    "source": "custom_events_goals",
    "custom_events_goal": {
      "goal_type": "purchase",
      "aggregate_operator": "sum",
      "operator": "gte",
      "value": 500,
      "timeframe_operator": "in_the_last_days",
      "timeframe_values": ["30"]
    }
  }
  ```
  - Aggregate operators: `sum`, `count`, `avg`, `min`, `max`
  - Timeframe operators: `anytime`, `in_the_last_days`, `in_date_range`, `before_date`, `after_date`

### Breaking Changes

- Removed `lifetime_value`, `orders_count`, `last_order_at` from Contact model
- Segments using deprecated contact fields will be deleted during migration
- Contact timeline event kinds renamed (existing data migrated automatically)
- Segment tree `TreeNodeLeaf.table` field renamed to `source` (existing segments migrated automatically)

## [17.4] - 2025-11-28

- Enhancement: The File Manager now supports S3-compatible storage providers with path-style bucket endpoint.

## [17.3] - 2025-11-27

- New feature: Blog posts can now be scheduled for publication at past or future dates, allowing you to plan posts in advance or import posts with their original publication dates.

## [17.2] - 2025-11-27

- Fix: retrieve broadcasts list when `pause_reason` or `winning_template` is null to avoid SQL errors
- Fix: blog cache is cleared when updating mailing lists to ensure subscription forms work correctly
- Enahncement: blog theme shows a helpful error message when no public lists are configured for newsletter subscription forms

## [17.1] - 2025-11-27

- Fix: Invitation links now correctly point to `/console/accept-invitation` instead of `/accept-invitation` to match the new console path

## [17.0] - 2025-11-26

### Database Schema Changes

- Migration v17.0 introduces blog feature support

### Features

- **Blog Feature**: Full-featured blogging system with advanced templating capabilities

  - Notion-like blog post editor for intuitive content creation
  - Full control over templating using Liquid syntax for dynamic content
  - The blog is served at the root path `/` of the custom domain configured in the workspace settings when it's enabled

- **Segmentation Engine JSON Support**: Enhanced contact segmentation with JSON attribute matching
  - Support for matching contact `custom_json_x` attributes in segmentation rules
  - Advanced filtering capabilities for complex contact data structures
  - Improved targeting precision for email campaigns and contact management

### Enhancements

- **Template Permission Updates**: Custom email blocks now use template write permissions instead of workspace write permissions for better security granularity
- **Auto-unsubscribe Enhancement**: Notification center now automatically processes unsubscribe actions when the unsubscribe link loads, improving user experience

### Breaking Changes

- The console UI is now serverd at `/console` instead of `/` to avoid conflicts with the new blog feature. When the blog is disabled the `/` path will redirect to `/console`.
- Permission system updated: saving custom email blocks requires `templates:write` permission instead of `workspace:write`

## [16.3] - 2025-11-15

### Fixes

- Fix: Replace hardcoded SMTP PLAIN auth with auto-discover to support Azure Communication Email and other providers requiring LOGIN authentication

## [16.2] - 2025-11-14

### Fixes

- Fix: Setup wizard redirection loop when API_ENDPOINT differs from actual console host name
- Add `/healthz` endpoint for container health checks that pings the database

## [16.1] - 2025-11-06

### Features

- **SMTP Relay for Transactional Emails**: Built-in SMTP relay server for sending transactional emails via standard SMTP clients
  - Send transactional emails using SMTP protocol instead of HTTP API calls
  - Useful for integrating legacy systems and standard email libraries
  - TLS encryption support on port 587 with STARTTLS
  - Authentication using workspace API credentials
  - Email body contains JSON payload matching Transactional API format
  - Configuration via environment variables: `SMTP_RELAY_ENABLED`, `SMTP_RELAY_PORT`, `SMTP_RELAY_DOMAIN`, `SMTP_RELAY_TLS_CERT_BASE64`, `SMTP_RELAY_TLS_KEY_BASE64`

## [16.0] - 2025-11-03

### Database Schema Changes

- Added `integration_id` column to `transactional_notifications` and `templates` tables for integration-managed resources

### Security

- **Message Data Encryption**: Template data variables are now encrypted at rest in the database
  - Message history `message_data.data` field is encrypted using AES-256-GCM with workspace secret key
  - Only new messages are encrypted (no migration of existing data)
  - Automatic encryption on message creation and decryption on retrieval
  - Backward compatible: unencrypted messages from before this version can still be read
  - Metadata remains unencrypted for query performance
  - Protects sensitive data like tokens, API keys, and personal information in template variables

### Features

- **Supabase Integration**: Connect Supabase Auth with Notifuse
  - Auth Email Hook: Send branded authentication emails (signup, magic link, password recovery, email change, reauthentication)
  - Before User Created Hook: Automatically sync new Supabase users to Notifuse contacts
  - Auto-generated email templates for all Supabase auth events with proper Liquid variables
  - HMAC-SHA256 webhook signature verification for security
  - Optional automatic list subscription and disposable email rejection
  - Field mapping: Supabase `user_metadata` to configurable Notifuse custom JSON field
- Integration-managed templates and transactional notifications cannot be deleted (but can be edited)

## [15.0] - 2025-11-01

### 🔒 SECURITY UPGRADE: PASETO → JWT + Enhanced Authentication Security

This is a **major security release** that migrates the authentication system from PASETO to JWT (HS256) and implements comprehensive security improvements.

### ⚠️ BREAKING CHANGES

**Migration Requirements:**

- **REQUIRED**: Set `SECRET_KEY` environment variable before upgrading
  - **CRITICAL FOR EXISTING DEPLOYMENTS**:
    - If you already have `SECRET_KEY` set: **Keep it unchanged** (do not generate a new one)
    - If migrating from PASETO: Use your existing PASETO key: `export SECRET_KEY="$PASETO_PRIVATE_KEY"`
  - **For new installations only**: Generate new key: `export SECRET_KEY=$(openssl rand -base64 32)`
- Server will automatically restart after migration to reload JWT configuration

**🚨 CRITICAL WARNING**:

- **DO NOT change your existing SECRET_KEY** - it encrypts all workspace integration secrets (email provider API keys, SMTP passwords, etc.)
- Changing SECRET_KEY will:
  - ❌ Make all encrypted integration secrets unreadable (permanent data loss)
  - ❌ Break all email sending (SparkPost, Mailjet, Mailgun, SMTP credentials lost)
  - ❌ Invalidate all JWT tokens and user sessions
- **Only generate a new SECRET_KEY for fresh installations with no existing data**

**What Gets Invalidated (During PASETO → JWT Migration):**

- ✗ All user sessions (users must log in again - PASETO tokens → JWT tokens)
- ✗ All API keys (must be regenerated - PASETO format → JWT format)
- ✗ All pending workspace invitations (invitation tokens were PASETO-signed)
- ✗ All active magic codes (migrating from plain-text → HMAC-SHA256 hashes)

**Why:** PASETO tokens are incompatible with JWT verification. Clean migration ensures no security gaps.

**Important Notes:**

- If you're already using JWT (not migrating from PASETO), and you keep your existing `SECRET_KEY`, your existing sessions remain valid.
- The `SECRET_KEY` is also used to encrypt workspace integration secrets (API keys, SMTP credentials). **Never change it on existing deployments** or you'll lose access to all encrypted credentials permanently.

### Security Improvements

#### 1. **JWT Authentication (HS256)**

- Migrated from PASETO to industry-standard JWT with HMAC-SHA256 signing
- **Simplified setup**: Uses symmetric key (`SECRET_KEY`) instead of PASETO's asymmetric key pair
  - No need to generate and manage separate public/private keys
  - Single `SECRET_KEY` environment variable for all cryptographic operations
  - Easier deployment and configuration management
- Algorithm confusion attack prevention (strict HMAC validation)
- Comprehensive token validation (signature, expiration, claims)
- Compatible with standard JWT libraries and tools

#### 2. **HMAC-Protected Magic Codes**

- Magic codes now stored as HMAC-SHA256 hashes (no plain text in database)
- Database compromise cannot reveal authentication codes
- Constant-time comparison prevents timing attacks
- Migration clears all existing plain-text codes

#### 3. **Server-Side Logout**

- New `/api/user.logout` endpoint (POST, requires authentication)
- Deletes ALL sessions for the authenticated user from database
- Tokens become immediately invalid after logout
- Protected endpoints now verify session exists in database
- Returns 401 Unauthorized if session has been deleted
- Frontend integration with graceful error handling

#### 4. **Rate Limiting for Authentication Endpoints**

- Protection against brute force attacks and email bombing
- In-memory rate limiter with sliding window algorithm
- Sign-in endpoint: 5 attempts per 5 minutes per email address
- Verify code endpoint: 5 attempts per 5 minutes per email address
- Rate limiter automatically resets on successful authentication
- Thread-safe concurrent access with automatic cleanup
- Independent rate limits per user and per endpoint
- Prevents magic code brute force attacks (blocks 99%+ of attempts)

### Features

- Enhanced session verification in `GetCurrentUser` endpoint
- Added `DeleteAllSessionsByUserID` method to user repository
- Added `Logout` method to user service interface
- Frontend `AuthContext` now calls backend logout before clearing local storage
- Migration automatically cleans up incompatible authentication artifacts

### Testing

- New integration tests for logout functionality (`tests/integration/user_logout_test.go`)
- New integration tests for rate limiter (`tests/integration/rate_limiter_test.go`)
- Unit tests for rate limiter with race detection
- Fixed race condition in concurrent rate limiter test (atomic operations)
- All tests pass with `-race` flag enabled

### Documentation

- Updated security audit document (`SECURITY_AUDIT_JWT_SESSIONS.md`)
- Changed "No Server-Side Logout" from 🔴 CRITICAL to ✅ IMPLEMENTED
- Changed "Brute Force Risk" from MEDIUM to LOW
- Added detailed implementation notes and testing coverage
- Comprehensive migration guide in v15 migration file

### Post-Migration Actions Required

1. **Users**: Log in again with email/password (or magic code)
2. **API Key Holders**: Regenerate API keys in Settings → API Keys
3. **Integrations**: Update all API integrations with new keys
4. **Workspace Admins**: Resend pending invitations via Settings → Members → Invitations

### Migration Notes

- Migration v15 is idempotent and safe to run multiple times
- Estimated migration time: < 1 second
- Server automatically restarts after migration
- Migration validates `SECRET_KEY` environment variable before proceeding
- Comprehensive migration summary displayed in console

## [14.1] - 2025-11-01

### Features

- **Bulk Contact Import**: New `/api/contacts.import` endpoint for efficiently importing large numbers of contacts
  - Creates or updates multiple contacts in a single batch operation using PostgreSQL bulk upsert
  - Returns individual operation results (created/updated/error) for each contact
  - Optional bulk subscription to lists via `subscribe_to_lists` parameter
  - Significantly faster than individual upsert operations for large imports
  - Batch size of 100 contacts processed at a time in the UI
  - Supports partial success - some contacts can succeed while others fail validation

## [14.0] - 2025-10-31

### Database Schema Changes

- Added `channel_options` JSONB column to `message_history` table to store email/SMS/push delivery options (CC, BCC, FromName, ReplyTo...)

### Features

- **Internal Task Scheduler**: Tasks now execute automatically every 30 seconds
  - No external cron job required
  - Configurable via `TASK_SCHEDULER_ENABLED`, `TASK_SCHEDULER_INTERVAL`, `TASK_SCHEDULER_MAX_TASKS`
  - Starts automatically with the app, stops gracefully on shutdown
  - Faster task processing (30s vs 60s minimum with external cron)
- **Privacy Settings**: New optional configuration for telemetry and update checks
  - `TELEMETRY` environment variable (optional) - Send anonymous usage statistics
  - `CHECK_FOR_UPDATES` environment variable (optional) - Check for new versions
  - Both can be configured via setup wizard if not set as environment variables
  - For existing installations: migration v14 sets both to `true` by default (respects env vars if set)
  - Environment variables always take precedence over database settings
- Message history now stores email delivery options:
  - CC (carbon copy recipients)
  - BCC (blind carbon copy recipients)
  - FromName (sender display name override)
  - ReplyTo (reply-to address override)
- Message preview drawer displays email delivery options when present
- Only stores email options in this version (SMS/push to be added later)
- Modernized Docker Compose to use current standards: renamed `docker-compose.yml` to `compose.yaml`, removed deprecated `version` field, updated commands to use `docker compose` plugin syntax, and improved `.env` file integration

### UI Changes

- Removed cron setup instructions from setup wizard
- Removed cron status warning banner from workspace layout
- Simpler onboarding experience - no manual cron configuration needed
- Added preview mode to notification center
- **Setup Wizard Improvements**:
  - Added newsletter subscription option
  - PASETO keys configuration moved to collapsible "Advanced Settings" section
  - Added "Privacy Settings" section for telemetry and update check configuration
  - Improved restart handling: displays setup completion screen immediately while server restarts
  - User can review generated keys before manually redirecting to signin

### Deprecated (kept for backward compatibility)

- `/api/cron` HTTP endpoint (internal scheduler is now primary)
- `/api/cron.status` HTTP endpoint (still functional but not advertised)

### Fixes

- Fix: SMTP now supports unauthenticated/anonymous connections (e.g., local mail relays on port 25)
- Fix: Docker images now built with CGO disabled to prevent SIGILL crashes on older CPUs
- Fix: Decode HTML entities in URL attributes to ensure links with query parameters work correctly in MJML-compiled emails
- Fix: Normalize browser timezone names to canonical IANA format to prevent timezone mismatch errors
- Fix: Broadcast pause also pauses the associated task

### Migration Notes

- Added `ShouldRestartServer()` method to migration interface
- Migrations can now trigger automatic server restart when config reload is needed
- Existing messages will have `channel_options = NULL` (no backfill)
- Migration v14 adds default telemetry and update check settings for existing installations (both default to `true`)
- Migration is idempotent and safe to run multiple times
- Estimated migration time: < 1 second per workspace
- Server will automatically restart after migration to reload all configuration settings

## [13.7] - 2025-10-25

- New feature: transactional email API now supports `from_name` parameter to override the default sender name

## [13.6] - 2025-10-24

- Upgrade github.com/wneessen/go-mail from v0.7.1 to v0.7.2

## [13.5] - 2025-10-23

- Fix: SMTP transport now supports multiple CC and BCC recipients

## [13.4] - 2025-10-22

- Fix: segment filters now support multiple values for contains/not_contains operators
- Multiple values are combined with OR logic as indicated in the UI

## [13.3] - 2025-10-11

- Fix: custom field labels now display consistently in contacts table column headers and JSON viewer popups
- Contacts table columns now use custom field labels from workspace settings instead of generic defaults
- JSON custom fields now show custom labels in their popover titles

## [13.2] - 2025-10-10

- Add new filters to message history: filter by message ID, external ID, and list ID
- List ID filter supports searching messages sent to a specific list
- New feature: customize display names for contact custom fields in workspace settings

## [13.1] - 2025-10-09

- Fix SMTP form default `use_tls` not being included in form submissions

## [13.0] - 2025-10-09

- New feature: segmentation engine now supports relative dates (e.g., "in the last 30 days")
- Segments containing relative dates are automatically refreshed every day at 5am in the segment timezone
- Fix critical regression introduced in v11 that blocked broadcast sending

## [12.0] - 2025-10-08

- Move rate limit configuration from broadcast audience settings to email integration settings
- Rate limit is now a required field on email integrations (default: 25 emails/minute)
- Simplifies broadcast configuration and centralizes rate limiting at the integration level
- Migration v12 automatically sets default rate limit on all existing email integrations

## [11.0] - 2025-10-08

- New feature: setup wizard for initial configuration
- Many environment variables are now optional and can be configured through the setup wizard: `ROOT_EMAIL`, `API_ENDPOINT`, `PASETO_PRIVATE_KEY`, `PASETO_PUBLIC_KEY`, `SMTP_HOST`, `SMTP_PORT`, `SMTP_USERNAME`, `SMTP_PASSWORD`, `SMTP_FROM_EMAIL`, `SMTP_FROM_NAME`
- Database environment variables remain required: `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`
- `SECRET_KEY` remains required (or `PASETO_PRIVATE_KEY` as backward compatibility fallback) to encrypt sensitive data in the database
- Configuration can be provided through setup wizard on first install and stored securely in database
- PASETO keys can be generated automatically and shown at the end of the setup wizard
- Environment variables always override database settings when present

## [10.0] - 2025-10-07

- New feature: automatic contact list status updates on bounce and complaint events
- Add `list_ids` TEXT[] column to `message_history` table to track which lists a message was sent to
- Add database trigger to automatically update `contact_lists` status to 'bounced' or 'complained' when hard bounces or complaints occur
- Distinguish between hard bounces (permanent failures) and soft bounces (temporary failures) - only hard bounces affect contact status
- Status hierarchy: complained > bounced > other statuses
- Backfill historical broadcast messages with list associations
- Render broadcast name in message logs
- escape special characters in MJML import/export

## [9.0] - 2025-10-06

- New feature: email attachments support for transactional emails
- Add `message_attachments` table for deduplication and storage of attachment content
- Add `attachments` JSONB column to `message_history` table
- Support for attachments across all ESP integrations (SMTP, SES, SparkPost, Postmark, Mailgun, Mailjet)
- Maximum 20 attachments per email, 3MB per file, 10MB total

## [8.0] - 2025-10-04

- New feature: real-time contact segmentation engine
- Add `db_created_at` and `db_updated_at` fields to contacts table for accurate database tracking
- Add `kind` field to contact timeline for granular event types (e.g., open_email, click_email)
- Make `created_at` and `updated_at` optional with database defaults to support historical data imports
- Ensure all timestamps stored in UTC timezone

## [7.1] - 2025-10-04

- Fix panic when broadcast rate limit is set to less than 60 emails per minute
- Improve rate limiting calculation to properly handle low rate limits

## [7.0] - 2025-10-02

- New feature: contact events timeline (messages, webhook events, profile mutations etc...). It's the backbone of the upcoming automations feature.

## [6.11] - 2025-09-30

- Implement per-broadcast rate limiting functionality
- Add support for broadcast-specific rate limits that override system defaults
- Make rate limit field required in broadcast form with default value of 25 emails/minute
- Add comprehensive test coverage for per-broadcast rate limiting

## [6.10] - 2025-09-29

- Upgrade github.com/wneessen/go-mail from v0.6.2 to v0.7.1

## [6.9] - 2025-09-28

- Add cron status monitoring endpoint `/api/cron.status`
- Add SettingRepository for managing application settings
- Add automatic cron health checking in frontend console
- Add visual banner with setup instructions when cron is not running
- Update TaskService to track last cron execution timestamp

## [6.8] - 2025-09-24

- Fix scheduled broadcast time handling to use string format instead of time.Time
- Remove broadcast service dependency from task service tests
- Update ParseScheduledDateTime tests to match implementation behavior

## [6.7] - 2025-09-19

- Add new workspace dashboard

## [6.6] - 2025-09-12

- Bulk update contacts functionality to console

## [6.5] - 2025-09-10

- Add delete contact functionality to console
- Redact email addresses in message history and webhook events when deleting a contact

## [6.4] - 2025-09-10

- Add test email functionality to broadcast variations
- Fix permissions for test emails to require read template and write contact permissions

## [6.3] - 2025-09-08

- Fix set permissions on root user
- Force all permissions to owners

## [6.2] - 2025-09-08

- Fix circuit breaker error message in broadcast pause reason
- Simplify broadcast circuit breaker notification email

## [6.1] - 2025-09-07

- hide menu items in console when user doesn't have access to the resource
- disabled create/update buttons in console when user doesn't have write permissions

## [6.0] - 2025-09-07

- Add permissions with roles per workspace

## [5.0] - 2025-09-06

- Add pause_reason column to the broadcasts table to store the reason for broadcast pause
- Pause broadcasts when circuit breaker is triggered
- Add system notification service to email circuit breaker events

## [4.0] - 2025-09-06

- Add migrations to the system and workspace databases
- Add permissions column to the user_workspaces table for future permission management
- Add UI previsions about broadcast rate limit per hour/day

## [3.14] - 2025-09-05

- Fix VARCHAR(255) constraint for status_info in message_history table

## [3.13] - 2025-09-03

- Fix z-index for file manager in template editor
- Improve broadcast UI with remaining test time, refresh button, and variations stats
- Improve transactional email API command modal with more examples and better documentation

## [3.12] - 2025-09-02

### Security

- Only root user can create new workspaces
- Added server-side validation to restrict workspace creation to the user specified in `ROOT_EMAIL` environment variable
- Create workspace UI elements are now hidden for non-root users in the console interface

## [v3.11] - 2025-09-01

### Fixed

- Hide deleted list in notification center when user has subscribed

## [v3.10] - 2025-09-01

### Added

- View a resend member invitations
- Access template test data in "Send test template" transactional email

## [v3.9] - 2025-08-31

### Added

- Mailgun integration now supports broadcast campaigns and newsletters, in addition to transactional emails

## [v3.8] - 2025-08-30

### Fixed

- Fixed issue: accept invitation

## [v3.7] - 2025-08-28

### Added

- New feature: custom endpoint URL in workspace settings to customize the tracking links and notification center URLs

## [v3.6] - 2025-08-28

### Fixed

- MJML raw-block is now editable

## [v3.5] - 2025-08-28

### Changed

- Dates format is only English

## [v3.4] - 2025-08-27

### Changed

- Anonymous users can't signin anymore, they need to be invited to a workspace

## [v3.3] - 2025-08-27

### Deprecated

- The SECRET_KEY env var is now deprecated, and uses the PASETO_PRIVATE_KEY value to simplify deployments

## [v3.2] - 2025-08-27

### Added

- Install Notifuse quickly for non-production workload using a Docker compose that embeds Postgres

## [v3.1] - 2025-08-25

### Added

- Launch of the new Notifuse V3
