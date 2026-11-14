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
   the right. From 900 px to 1200 px the right column goes above the tabs, as in the drawer mockup;
   under 900 px everything stacks.
2. **Header without action buttons**: Refresh moves into the « … » menu, which also gets « Copy
   external ID » when the contact has one; Delete stays there, with contacts write.
3. **Activity feed** (`ContactActivityFeed`, replaces `components/timeline/ContactTimeline`): entries
   grouped by day (Today, Yesterday, date), a time column, a toned icon, a one-line sentence with the
   names in bold, an optional meta line and the source on the right; « Load more activity » at the
   bottom. Each entry kind is described by a pure function (`describeActivity`) so the wording is
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
| `components/contacts/sheet/ContactDetailsBody.tsx` | `ContactDetailsBody.test.tsx` | three columns hold the left sections, tabs and the right cards; no Refresh button in the header; Refresh, Copy external ID and Delete in « … »; Delete only with contacts write |
| `components/contacts/sheet/describeActivity.tsx` (new) | `describeActivity.test.tsx` (new) | sentence, tone and source for each entity type: email sent/delivered/opened/clicked/link click, list subscribed/pending/unsubscribed/removed and a status missing from `changes`, segment joined/left, contact created/updated with old → new, custom event with properties, inbound webhook bounce/complaint, web session, web page, automation start/end/failed |
| `components/contacts/sheet/activityParts.tsx` (new) | `describeActivity.test.tsx` | the name of an email opens its preview, by click (the link is a theme link, not a native button whose colour the antd reset overrides) |
| `components/contacts/sheet/ContactActivityFeed.tsx` (new) | `ContactActivityFeed.test.tsx` (new) | groups by day in the timezone (Today, Yesterday, date), time column, Load more calls the next page, empty state |
| `components/contacts/sheet/ContactActivityTab.tsx` | `ContactActivityTab.test.tsx` | renders the feed instead of the old timeline (existing filter cases kept) |
| `components/contacts/sheet/ContactMessagesTab.tsx` | `ContactMessagesTab.test.tsx` | name by origin, A/B variant badge, rendered subject, journey dots and badges, SMTP « Sent », « N of M », Load more |
| `components/contacts/sheet/MessageJourneySteps.tsx` (new) | `MessageJourneySteps.test.tsx` (new) | lit steps, failed/bounced/spam/unsubscribed badges, SMTP sent |
| `components/contacts/sheet/ContactListsCard.tsx` | `ContactListsCard.test.tsx` | count badge, two-line row with « Since », subscribe icon button, segments one per line with « since » |
| `components/contacts/sheet/ContactProfileFields.tsx` | `ContactProfileFields.test.tsx` | sections instead of panels (existing cases kept) |
| `components/segment/segmentColor.ts` (new) | `segmentColor.test.ts` (new) | preset names to colours, a CSS colour kept, unknown to the default |
| `components/contacts/ContactsTable.tsx`, `ContactsRail.tsx` | existing tests | dots use `segmentDotColor` |
| `components/timeline/ContactTimeline.tsx` (removed) | `ContactTimeline.test.tsx` (removed) | replaced by the feed |
| `i18n/locales/*.po` | — | `npm run lingui:extract`, French translations |

## Last step: checks

```
cd console
npm run lingui:extract
npx vitest run src/components/contacts src/components/segment src/components/timeline
npx tsc -b
npm run lint
npm run build
npm test
```

Then capture the sheet at 1440 and 1280 px with the demo data and compare it with mockups 51, 52 and 49.
