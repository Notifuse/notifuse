# Numbers follow the console language — plan

## Problem

Numbers written with `toLocaleString()` or `Intl.NumberFormat(undefined)` use the browser's
language: a French console in an English browser shows "12,345". The values of a contact's activity
and fields go through numbro, which writes English separators whatever the language. #555 listed
these as known. Pierre chose to align them in one small PR.

## Change

1. Counters use `int` from `useNumberFormat()` (`components/layout/format.ts`), the console's
   integer formatter in the active Lingui locale:
   - `broadcasts/SendingProgress.tsx`: "1 234 of 5 678".
   - `web_analytics/filters/BackfillStatus.tsx`: "12 345 rows rewritten".
   - `contacts/ExportContactsModal.tsx`: "Fetched … contacts...".
   - `contacts/ContactsCsvUploadDrawer.tsx`: the import, progress, error and resume counts, and the
     row count of the file.
   - `settings/Integrations.tsx`: the emails per hour and per day under the rate limit.
2. `timeline/ContactTimeline.tsx`: the old and new values of a contact change (the `{old, new}` pairs
   the contact trigger writes, shown "1 234,5 → 12 345", with every decimal they carry so that
   0.0001 never reads "0"), a flat changed number and a goal value pass
   `i18n.locale` to `toLocaleString`, keeping their options (the goal value keeps two decimals and
   its "$"). Non-numbers are still shown with `String()`.
3. `web_analytics/lib/format.ts` `formatCurrency`: `i18n.locale || 'en'` instead of the browser's
   locale, in both branches. The K and M suffix now goes after the whole number, group separators
   included, so a French amount past a billion reads "2 346M €" and not "2M 346 €". That fixes a
   released behaviour (in a French browser), so it gets its own Fix line.
4. `utils/formatters.tsx` `formatValue`: `Intl.NumberFormat(i18n.locale || 'en')` replaces numbro,
   with the same rule: a positive decimal keeps up to two digits, anything else is rounded half up
   with `Math.round` first, as numbro did, and a rounded -0 shows as "0", not "-0". It serves the timeline's event values and a contact's number fields (`InlineEditableField`).
5. Every changed message keeps a call expression inside `t`, so the message ids keep their
   positional placeholders. `npm run lingui:extract` adds and changes no message (not kept).
6. **CHANGELOG**: folded into the unreleased 44.0 Improvement bullet on the console language, plus a
   Fix line for the billion suffix.

## Tests (TDD: written first, seen failing)

French is activated on the Lingui singleton and English put back afterwards. French groups
thousands with a narrow no-break space, matched with `\s` or normalised.

- `web_analytics/lib/format.test.ts`: `formatCurrency` in French ("1 235", "12,25", "980 €",
  "12,5K €", "2 346M €") and English ("1,235", "$12.5K", "$2,346M").
- `broadcasts/SendingProgress.test.tsx` (new): "1 234 of 5 678".
- `web_analytics/filters/BackfillStatus.test.tsx` (new): "12 345 rows rewritten", with the service
  mocked.
- `utils/formatters.test.tsx`: "12 345" and "1 234,57" in French, "12,345" and "1,234.5" in English,
  and -0.4 gives "0", -1234.5 "-1,234".
- `timeline/ContactTimeline.test.tsx`: a purchase goal reads "$1 234,50", and a contact change
  `{ old: 1234.5, new: 12345 }` reads "1 234,5" and "12 345" in French, and 0.0001 and 48.8566141
  keep every decimal.
- `settings/Integrations.test.tsx`: the SES form's rate limit (25 per minute) reads "≈ 1 500" and
  "≈ 36 000" in French.
- Not tested: `ExportContactsModal` (the count shows only while the export fetches) and
  `ContactsCsvUploadDrawer` (inside the import flow). Each is the same `int(...)` call the tested
  screens use. The flat changed number in the timeline is not tested either: the trigger always
  writes old/new pairs.

## Out of scope

- The web analytics abbreviations, percentages and durations (`formatNumber`, `formatValue`,
  `formatAxisValue`: "1.2K", "12.5%", "5m 12s") are written in English whatever the language. They
  are not the browser's language, and a change there touches every chart.
- The `numbro` dependency is no longer imported but stays in `package.json`.

## Verification

- `cd console && npx vitest run src/components/web_analytics src/components/broadcasts src/utils src/components/timeline src/components/contacts src/components/settings`
- Then once: `npm test`, `npx tsc -b`, `npx eslint .`
