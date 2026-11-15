# Plan — contact sheet aligned with the Nuit claire mockups (51, 52, 49)

## Need

The review of the console against the mockups (screens 51 « Fiche contact plein écran, activité »,
52 « Visites web » and the Messages state 49) found the contact sheet off-topic: two columns instead
of three, a Refresh button in the header, the old activity timeline and the old technical messages
table. Pierre asked for every unwanted deviation to be fixed, screen by screen; this PR covers the
contact sheet. Console only: no Go file, no SQL, no environment variable.

Kept as the spec decided (not deviations): no MRR KPI, no « vs workspace » comparison, the Gravatar
avatar, no Newsletters / Transactional / Automations sub-filters in the Messages tab, no GDPR export,
no subscription source (« via ») under a list.

## Changes

1. **Three columns** (`ContactDetailsBody`): attributes on the left (Profile, Custom fields,
   Tracking consent, System as sections of one card), the tabs in the centre, Lists and Segments on
   the right. From 900 px to 1400 px the right column goes above the tabs, as in the drawer mockup;
   under 900 px everything stacks.
2. **Header without action buttons**: Refresh moves into the « … » menu, which also gets « Copy
   external ID » when the contact has one; Delete stays there, with contacts write.
3. **Activity feed** (`ContactActivityFeed`, replaces `components/timeline/ContactTimeline`): entries
   grouped by day (Today, Yesterday, date), a time column, a toned icon, a one-line sentence with the
   names in bold, an optional meta line and the source on the right; « Load more activity » at the
   bottom. Each entry kind is described by a hook (`useActivityDescriber`) so the wording is
   unit-tested. The email preview (template name as a link) and the web page preview are kept. Fixes
   the « → undefined » of a list entry without its status in `changes`.
4. **Messages tab** (`ContactMessagesTab`): Message (name of the newsletter, automation,
   notification or template, « Variant B » for an A/B test, the subject rendered with the message
   data underneath), Type, Journey (Delivered, Opened, Clicked as lit dots; Failed, Bounced, Marked
   as spam and Unsubscribed as badges; « Sent » with the SMTP tooltip when the provider reports no
   delivery), Sent, and a « View the sent email » button; « N of M messages » with Load more.
5. **Lists and Segments cards**: count badge in the header, « + » icon to subscribe, each list on two
   lines (name and status, then « Since … ») so a long name no longer truncates to nothing; segments
   one per line with their dot and « since … ».
6. **Segment dots**: `segmentDotColor` maps the preset names a segment stores (`volcano`,
   `geekblue`…) to their colour, so every segment has a dot (sheet, contacts table, rail).
7. « Times in {timezone} » no longer wraps.

## Files and tests

| Implementation | Test file | Cases |
|---|---|---|
| `components/contacts/sheet/ContactDetailsBody.tsx` | `ContactDetailsBody.test.tsx` | Refresh and Delete in « … », Delete only with contacts write; three columns with lists or segments to read, two without (`data-columns`); tabs and their counts by permission |
| `components/contacts/sheet/sheetDates.ts` | `sheetDates.test.ts`, `ContactActivityFeed.test.tsx` | `sheetTimezone` keeps a known contact timezone and falls back to the workspace's for a missing or unknown one; Today, Yesterday and the date in the feed |
| `components/contacts/sheet/describeActivity.tsx` (`useActivityDescriber`, a hook) | `describeActivity.test.tsx` (new) | fixtures are the rows the triggers write: email sent, delivered, opened, clicked, link clicked (with and without envelope), failed, bounced, complained, unsubscribed; list subscribed, pending, unsubscribed, bounced, resubscribed, confirmed, removed (never read from the current status), a row without status; segment joined and left; profile update with old → new, +N, consent fields; custom event from its kind, goal and value; numbers in French; inbound webhook bounce from its own changes, provider unsubscribe; web session (never 0 s, exit without entry, no envelope); web page link; automation start, completed, failed; unknown entity |
| `components/contacts/sheet/activityParts.tsx` (new) | `describeActivity.test.tsx` | a linked name is a button by role and opens its email or page |
| `components/contacts/sheet/contactFieldLabels.ts` | `describeActivity.test.tsx` | tracking consent fields have a label |
| `components/contacts/sheet/ContactActivityFeed.tsx` (new) | `ContactActivityFeed.test.tsx` (new) | groups by day in a non-UTC timezone with each time, Load more, empty state, the email preview gets the message's template data, not its envelope |
| `components/contacts/sheet/ContactActivityTab.tsx` | `ContactActivityTab.test.tsx` | renders the feed (existing filter cases kept) |
| `components/contacts/sheet/ContactMessagesTab.tsx` | `ContactMessagesTab.test.tsx` | name by origin, A/B variant, subject in the language sent and rendered, subject forced at send time, the preview opens the email in the language sent, a template that fails to load says so, « N of M message(s) » in the singular and plural, Load more, no names nor preview without the rights |
| `components/contacts/sheet/MessageJourneySteps.tsx` (new) | `MessageJourneySteps.test.tsx` (new) | lit steps, a click implies open and delivery, SMTP « Sent », failed and bounced with their reason for screen readers, complaint after a bounce, each step's state in words and focusable |
| `components/contacts/sheet/ContactListsCard.tsx`, `SheetSection.tsx` | `ContactListsCard.test.tsx` | count badges, « Since » on the list row, « since » on a segment, remove after confirmation, rights |
| `components/contacts/sheet/ContactProfileFields.tsx` | `ContactProfileFields.test.tsx` | existing cases kept |
| `components/contacts/visits/ContactWebVisits.tsx` | `ContactWebVisits.test.tsx` | the session day above its time; fixed widths so the table fits the centre column (checked by capture) |
| `components/segment/segmentColor.ts` (new) | `segmentColor.test.ts` (new) | preset names to colours, a CSS colour kept, the form's default preset without a colour |
| `components/contacts/ContactsTable.tsx`, `ContactsRail.tsx` | `segmentColor.test.ts` | dots use `segmentDotColor` |
| `components/timeline/webPageUrl.ts` (moved out of `ContactTimeline.tsx`) | `webPageUrl.test.ts` | the page URL cases of the old timeline tests |
| `components/timeline/ContactTimeline.tsx`, `index.ts` | removed | replaced by the feed; its description cases moved to `describeActivity.test.tsx` |
| `services/api/contact_timeline.ts` | type only | `message_data` typed as the stored envelope |
| `utils/formatters.tsx` | `formatters.test.tsx` | dead `formatEventName` and `getSourceBadge` removed |
| `CHANGELOG.md` | — | feature bullet, and a Fix line for « → undefined », which shipped in 43.0 |
| `i18n/locales/*.po` | `catalogues.test.ts` | extracted, translated in every locale |

## Known deviations from the mockups

- The source chip on an activity entry is the entry's type (Email, List, Segment, Web analytics, the provider of a webhook), not the mockup's origin (API, Webhook Stripe, Automatisation): the timeline does not record where a change came from.
- Custom event properties are not shown: the timeline query joins no custom event data.
- At 1440 px the Messages and Web visits tables are narrower than in the mockup; three columns start at 1400 px, below which the Lists and Segments cards go above the tabs.

## Last step: checks

```
cd console
npm run lingui:extract && git diff --exit-code src/i18n   # catalogues already up to date
npx vitest run src/components/contacts src/components/segment src/components/timeline
npx tsc -b
npm run lint
npm run build
npm test
```

Then capture the sheet at 1440 and 1280 px with the demo data and compare it with mockups 51, 52 and 49.
