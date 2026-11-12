# Localized date formats on web analytics and blog — plan

## Problem

Since dayjs follows the console language (#545), the date patterns written in English order print
translated words in English order: `'MMM D'` gives "mars 5", `'ha'` gives "3午後" in Japanese, and
`'MMM D, YYYY HH:mm'` gives "mars 5, 2026 15:00". Pierre decided to move them to localized formats
in one follow-up, once the screens' own redesigns had merged. Web analytics, annotations and the blog
have merged. The message history and `utils/formatters.tsx` wait for the Logs redesign and will get
their own PR.

## Change

1. **`console/src/i18n/dateFormats.ts`** (new). Short labels built with `Intl.DateTimeFormat` in the
   active Lingui locale, so each language orders and abbreviates its own way.
   - `formatMonthDay(d)` gives "Feb 3", "3 févr." or "2月3日".
   - `formatHour(d)` (`timeStyle: 'short'`) gives "9:00 AM", "09:00" or "9:00".
   - `formatHourOfDay(hour)` gives "12:00 AM" or "15:00", for the heat map's hour columns. The
     minutes stay so that a bare "15" (Spanish, Catalan, Italian, Brazilian Portuguese) never reads as a day.
   - `formatMonthYear(d)` gives "Aug 2026", "août 2026" or "2026年8月". The full year keeps it from
     reading as a day ("Aug 26").
   - `formatDayRange(start, end)` uses `formatRange`, which drops the parts both bounds share
     ("Feb 3 – 9", "3–9 févr."). It adds the full year only when the bounds' years differ, and
     gives one day when both are the same day. Intl writes Japanese ranges with numbers
     ("02/03～02/09"). Reversed bounds are put in order first, since some engines throw on them.
   - The functions take a `Dayjs` and format its wall clock: the fields the dayjs object shows,
     rebuilt as a UTC instant and formatted in UTC. A `.tz()` or `.utc()` instance therefore keeps
     the clock it was converted to, whatever the browser's zone.
   - Formatters are cached per locale and options. The locale is the active Lingui one, falling
     back to `en` when none is active (`Intl.DateTimeFormat('')` throws).
   - `plain()` turns Intl's narrow and thin no-break spaces into plain spaces ("9:00 AM",
     "Feb 3 – 9"): the same on screen, and the labels stay comparable in tests whatever the engine.
   - `tsconfig.app.json` adds `ES2021.Intl` to `lib` for `formatRange`. That is a type change only;
     every supported browser has the method.
2. **`console/src/components/web_analytics/lib/format.ts`**
   - `formatXAxisLabel` uses `formatHour`, `formatMonthDay` and `formatMonthYear`; the year stays
     `YYYY`.
   - `formatRangeSeriesName` uses `formatDayRange`.
3. **`console/src/components/web_analytics/lib/dates.ts`**: `formatDateRangeLabel` uses
   `formatDayRange`.
4. **`console/src/components/web_analytics/annotations/AnnotationTable.tsx`**: the date and time use
   dayjs `lll` (localized, still in the annotation's timezone).
5. **`console/src/components/blog/RecentThemesTable.tsx`**: the tooltip uses `lll` in the workspace
   timezone. It now imports `../../lib/dayjs`, which has `localizedFormat`, instead of extending bare
   dayjs itself. This is hygiene: plugins are global to the dayjs singleton, which `lib/dayjs` has
   always extended by the time the page renders.
6. **`console/src/components/web_analytics/toolbar.tsx`**: the custom RangePicker shows `ll` instead
   of `D MMM YYYY` (which gave "3 2月 2026" in Japanese).
7. **`console/src/components/web_analytics/lib/dictionaries.ts`**: `HOUR_LABELS` ("12a" … "11p" in
   every language) becomes `hourLabels()`, which `TrafficHeatmapWidget` memoises on the locale.
8. Comments citing the old labels ("3am", "Dec 21-27") and `transactionalTime.ts`'s "no dayjs locale
   is loaded" are updated.
9. **CHANGELOG**: folded into the unreleased 44.0 bullet of #545, with no separate line.

The English labels change too:
- "9am" becomes "9:00 AM";
- "Aug '26" becomes "Aug 2026";
- "3 - 9 Feb" becomes "Feb 3 – 9";
- the series names change, "Dec 21-27" becoming "Dec 21 – 27", with one day shown once and the year
  across years;
- "Aug 15, 2026 09:00" becomes "Aug 15, 2026 9:00 AM";
- the heat map hours become "12:00 AM" … "11:00 PM".

They follow the English convention, and every language now goes through the same path.

## Tests (TDD: written first, seen failing)

- **`console/src/i18n/dateFormats.test.ts`** (new). Each helper is checked in en, fr and ja (fr and ja
  activated on the Lingui singleton, then back to en).
  - The wall clock is kept: `dayjs.utc('2026-08-09T23:30:00Z')` gives "Aug 9", the same instant in
    Tokyo gives "Aug 10", and 02:00 UTC on the 10th in Los Angeles gives "Aug 9". The file sets
    `TZ=America/Los_Angeles` and restores it afterwards, so formatting on the browser's clock fails
    even on a UTC runner.
  - The range drops shared parts within a month, keeps both months across months, and adds the year
    across years.
- **`console/src/components/web_analytics/lib/format.test.ts`**: the English labels are updated, and
  French labels are added ("9 août", "août 2026").
- **`console/src/components/web_analytics/lib/dates.test.ts`**: the `formatDateRangeLabel` cases use
  the new English output, and a French case is added.
- **`console/src/components/web_analytics/tabs/AnnotationsTab.test.tsx`**: the expected English
  moment is `lll`, and a French case ("15 août 2026 09:00") fails with a hard-coded English pattern.
- **`console/src/components/blog/RecentThemesTable.test.tsx`** (new): hovering "Last Edited" in
  French shows `lll` on the Tokyo clock.
- **`console/src/components/web_analytics/toolbar.test.tsx`** (new): in Japanese, the custom range
  button reads "02/03～02/09" and the picker "2026年2月3日". It fails with `D MMM YYYY`.
- **`console/src/components/web_analytics/lib/dictionaries.test.ts`**: `hourLabels()` in English and
  French.
- `dateFormats.test.ts` also pins `formatHourOfDay`, the Japanese ranges and reversed bounds.
- `TrafficHeatmapWidget` has no test of its own; only `hourLabels()` is tested.

## Out of scope

- `MessageHistoryTable.tsx` and `utils/formatters.tsx`, which wait for the Logs redesign.
- `toLocaleDateString()` / `toLocaleString()` follow the browser's language, not the console's:
  `Integrations.tsx`, `LicenseSettings.tsx`, `LicenseBanner.tsx`, `AcceptInvitationPage.tsx`,
  `PostDrawer.tsx` and `ContactsCsvUploadDrawer.tsx`.
- `router.tsx` does not check that a custom range's start comes before its end. The label puts such
  a range in order, but the query keeps it as given.

## Verification

- `cd console && npx vitest run src/i18n src/components/web_analytics src/components/blog`
- Then once: `npm test`, `npx tsc -b`, and `npx eslint .` as CI runs it. `npm run lint` uses
  `--max-warnings 0` and already fails on warnings that predate this change.
