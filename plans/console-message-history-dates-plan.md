# Message history and event property dates on the zone they name — plan

## Problem

Two hover dates print a timezone name next to a time that is not in it, and the word "in" stays in
English whatever the console language:

- `MessageHistoryTable.tsx` `formatDate` gives `` `${dayjs(d).format('lll')} in ${workspace.settings.timezone}` ``:
  the time is on the browser's clock, the zone is the workspace's.
- `utils/formatters.tsx` `formatValue` (dates inside a contact's event properties in the timeline,
  and a contact's datetime fields through `InlineEditableField.tsx`, both with the workspace or
  contact timezone) gives `` `${date.format('LLLL')} in ${timezone}` ``, with the same mismatch.

`ContactTimeline.tsx` already fixed its own tooltip this way in #545 (`.tz(zone)` and
`` t`${when} in ${zone}` ``). This is the second half of the follow-up of #550, held until the first
Logs PR (#552) merged; it did not touch these two files.

## Change

1. **`console/src/components/messages/MessageHistoryTable.tsx`**: `formatDate` converts to the
   workspace timezone (`'UTC'` when unset, where the label read "in undefined" and `.tz('')` would
   throw) and wraps the suffix: `` t`${when} in ${zone}` ``.
2. **`console/src/utils/formatters.tsx`**: with a timezone, the date is `date.tz(timezone).format('LLLL')`
   and the title `i18n._(msg`${when} in ${zone}`)` (the function is not a component, so no hook).
   Without a timezone, the plain `LLLL` on the browser clock, with no zone name, as before.
3. Both use the placeholder names `when` and `zone`, so the message id is `{when} in {zone}`, already
   translated in every catalog. `npm run lingui:extract` adds no message (only reference churn, not
   kept).
4. **CHANGELOG**: the existing 44.0 Fix bullet on hover dates is extended to a contact's fields and
   the message history.
   The behaviour was released, so it stays a Fix.

## Tests (TDD: written first; the Tokyo cases seen failing)

- **`console/src/utils/formatters.test.tsx`** (new): a Tokyo timezone gives
  "Wednesday, August 12, 2026 7:00 PM in Asia/Tokyo" for 10:00 UTC; no timezone gives plain `LLLL`
  (unchanged behaviour, a regression guard).
- **`console/src/components/messages/MessageHistoryTable.test.tsx`**: `renderTable` takes a workspace;
  hovering Created At in a Tokyo workspace shows "Aug 1, 2026 7:00 PM in Asia/Tokyo"; a workspace
  with no timezone shows "Aug 1, 2026 10:00 AM in UTC".
- The Tokyo cases fail with the old code on any runner not itself in Tokyo (CI runs in UTC).

## Out of scope

- The second Logs PR replaces `MessageHistoryTab`'s table; the Logs thread is told to keep this
  pattern. `MessageHistoryTable` stays in use by `contacts/sheet/ContactMessagesTab.tsx`.
- A timezone name the browser's ICU does not know makes `.tz()` throw. Workspace timezones are
  checked by `domain.IsValidTimezone`, and `ContactTimeline` already converts the same zone.

## Verification

- `cd console && npx vitest run src/utils src/components/messages src/components/timeline`
- Then once: `npm test`, `npx tsc -b`, `npx eslint .`
