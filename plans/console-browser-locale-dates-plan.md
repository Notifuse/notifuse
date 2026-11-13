# Last browser-language dates follow the console language — plan

## Problem

Six dates are written with `toLocaleDateString()` / `toLocaleString()`, which use the browser's
language, not the console's: a French console in an English browser shows "8/1/2026". They were
left out of #550 and #553 and listed there as known. Pierre chose to align them in one small PR.

- `components/settings/Integrations.tsx` `formatConnectedOn`: the day a Zapier card was connected.
- `components/settings/LicenseSettings.tsx`: the licence's Expires row.
- `components/license/LicenseBanner.tsx`: the expiry date in the grace-period banner.
- `pages/AcceptInvitationPage.tsx`: the invitation's expiry date.
- `components/blog/PostDrawer.tsx`: the date and time in "Found unsaved changes from …".
- `components/contacts/ContactsCsvUploadDrawer.tsx`: the date and time in "A previous upload for … was
  found (…)".

## Change

1. The four dates use dayjs `ll` and the two date-times `lll`, through `lib/dayjs`, whose global locale
   follows the console language since #545 (`activateDayjsLocale`). The clock stays the browser's,
   as with `toLocale*`: these are instants with no timezone of their own to show.
2. `formatConnectedOn` keeps its guard: `dayjs(value).isValid()`, else an em dash. Its comment is
   updated.
3. The two prompts keep a call expression inside `t`, so their message ids keep the positional
   placeholder (`{0}`) and stay translated. `npm run lingui:extract` adds no message (reference churn
   only, not kept).
4. English changes too: in a US-English browser, "8/1/2026" becomes "Aug 1, 2026", and "8/1/2026, 10:00:00 AM" becomes
   "Aug 1, 2026 10:00 AM", as `WorkspaceMembers` already writes dates (`dateStyle: 'medium'`).
5. **CHANGELOG**: folded into the unreleased 44.0 Improvement bullet on dates following the console
   language. The old output was released, but it is a language improvement, not a fix.

## Tests (TDD: written first, seen failing)

Each activates the French dayjs locale and puts English back afterwards. Instants are at midday UTC,
so the day is the same on any runner clock from UTC−11 to UTC+11.

- `components/license/LicenseBanner.test.tsx`: the grace banner reads "expired on 1 mars 2026".
- `components/settings/LicenseSettings.test.tsx`: the Expires row reads "15 mars 2027".
- `components/settings/Integrations.test.tsx`: a Zapier card reads "1 août 2026".
- `__tests__/AcceptInvitationPage.test.tsx`: the invitation reads "20 nov. 2026"; the fixture gains an
  `expires_at`.
- `PostDrawer` and `ContactsCsvUploadDrawer` have no test of their prompts: they come from
  localStorage and saved upload progress inside large drawers. Their change is one `format('lll')`
  each, with the same dayjs locale the four tested screens use.

## Out of scope

- Numbers written with `toLocaleString()` (counts in the CSV import, the export modal, sending
  progress, rate limits) still follow the browser's language.

## Verification

- `cd console && npx vitest run src/components/license src/components/settings src/__tests__/AcceptInvitationPage.test.tsx src/components/blog src/components/contacts`
- Then once: `npm test`, `npx tsc -b`, `npx eslint .`
