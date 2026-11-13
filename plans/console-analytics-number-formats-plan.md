# Web analytics abbreviations, percentages and durations in the console language — plan

## Problem

`components/web_analytics/lib/format.ts` writes counts, percentages and durations by hand in
English, whatever the console language: "1.5K", "12.5%", "5m 12s", and on chart axes "13%" and
"2.5m". Several components add their own `toFixed(1)}%`. #558 listed this as known; Pierre chose to
change it in one PR.

## Change

1. **`web_analytics/lib/format.ts`**, all in the active Lingui locale (`i18n.locale || 'en'`):
   - `formatNumber`: rounded first; under 1,000 an integer (`maximumFractionDigits: 0`), so 999.6
     reads "1K" like 1000; from 1,000 Intl's compact notation with at most one decimal and grouping
     forced (compact leaves German and Japanese four-digit counts as "1234"; forced, "1.234"). English stays "1.5K" and "2.4M" (but "1K", not "1.0K"); French
     gives "1,5 k" and "2,4 M"; each language abbreviates its own way (German not below a million,
     Japanese "1.5万" from ten thousand). 999,999 reads "1M", no longer "1000.0K".
   - `formatPercent(value, decimals = 1, minimumDecimals = decimals)` (new): a value in percent,
     through Intl's percent style. "12.5%" in English, "12,5 %" in French.
   - `formatDuration(seconds)`: the messages a contact's visits already use
     (`contacts/visits/duration.ts`: "{seconds} s", "{minutes} min {seconds} s", "{hours} h {mm} min"),
     plus two new round forms, "{minutes} min" and "{hours} h". The value is rounded to the second
     first, and from an hour to the minute from the value itself, so 299.6 s reads "5 min", not
     "4 min 60 s", 3629.5 s "1 h", and the old "1h 60m" cannot happen.
   - `formatValue` and `formatAxisValue` use these: percentages through `formatPercent` (one decimal,
     none on axes), axis durations "45 s" or "2,5 min" (rounded first, so 59.6 s reads "1 min").
   - `formatCurrency` reads the same locale helper; unchanged otherwise.
2. Components that wrote their own percent sign use `formatPercent`: `Delta.tsx` (period change),
   `MetricChart.tsx` (tooltip change; the sign is now written separately, "+" or "-", before the
   absolute value), `explore/ExploreTable.tsx` (share, bounce rate, scroll depth),
   `explore/BreakdownDrawer.tsx` (bounce), `FilterBuilder.tsx` (a percentage filter's value, with
   every decimal it carries).
   `lib/csv.ts` keeps calling `formatDuration`: the export's readable duration column is now in the
   console's language ("5 min 12 s", "5 Min. 12 Sek."); the raw seconds column beside it is unchanged.
3. **Catalogs**: the two new messages are added by hand to the 8 `.po` files, translated as the
   existing duration messages are (de "{minutes} Min." / "{hours} Std.", ja "{minutes} 分" /
   "{hours} 時間", the others "{minutes} min" / "{hours} h"). `npm run lingui:extract` finds exactly
   these two new ids and no missing translation; its reference churn is not kept.
4. English changes: durations as a contact's visits already write them ("5m 12s" becomes
   "5 min 12 s", "2h 30m" "2 h 30 min", "1h 5m" "1 h 05 min", "45s" "45 s"; on axes "2.5m" becomes
   "2.5 min" and "1.0m" "1 min"); counts "1.0K" become "1K", "1500.0M" "1.5B", and 999.6 "1K".
5. **CHANGELOG**: folded into the unreleased 44.0 Improvement bullet on the console language. The
   rounding defects shipped in 43.0 ("4m 60s", "60s" for 59.5 s, "1h 60m", "1000.0K"), so they get
   their own Fix line.

## Tests (TDD: written first, seen failing)

- **`web_analytics/lib/format.test.ts`**:
  - English: `formatNumber` (999, "1.5K", "2.4M", 999,999 → "1M"), `formatDuration` ("45 s",
    "5 min 12 s", "5 min", "2 h 30 min", "1 h 05 min", "2 h", "0 s", and the carries 299.6 → "5 min",
    7199.9 → "2 h", and from an hour 7170 → "2 h", 3630 → "1 h 01 min", 3629.5 → "1 h"), 999.6 →
    "1K", `formatPercent`, `formatValue` and `formatAxisValue` for percentages, durations and counts
    (59.6 s on an axis → "1 min").
  - French (Lingui `fr` activated, then `en`): "1,5 k", "2,4 M", "12,5 %", axis "2,5 min" and "13 %".
  - German: 1234 → "1.234", 2,400,000 → "2,4 Mio.".
- **`web_analytics/MetricChart.test.tsx`**: the tooltip change against the previous period reads
  "+20.0%" and "-20.0%", never "--".
- The other components only call these helpers: their existing tests (explore, broadcasts) still
  pass. `Delta`, `ExploreTable`, `BreakdownDrawer`, `FilterBuilder` and `lib/csv.ts` have no test of
  their own.

## Out of scope

- The translated duration words come from the catalogs; the tests load empty catalogs, so they
  check the English source and Intl's separators, not the German or Japanese words.
- `DimensionTableWidget`'s `%` is a CSS bar width, not text.
- `formatCurrency` keeps its own "K"/"M" suffix after an amount in the console's language ("15K €",
  "$12.5K"): switching it to compact currency notation changes every amount's rounding.
- antd's `Progress` in `filters/BackfillStatus.tsx` writes its own "45%".
- A few counts are shown without separators (`LiveButton`, `LiveSessionMap`,
  `explore/MinSessionsInput`); they are small numbers in practice.

## Verification

- `cd console && npx vitest run src/components/web_analytics src/components/broadcasts src/components/analytics`
- Then once: `npm test`, `npx tsc -b`, `npx eslint .`, and `npm run lingui:extract` (no new id
  missing a translation).
