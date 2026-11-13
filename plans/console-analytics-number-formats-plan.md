# Web analytics abbreviations, percentages and durations in the console language — plan

## Problem

`components/web_analytics/lib/format.ts` writes counts, percentages and durations by hand in
English, whatever the console language: "1.5K", "12.5%", "5m 12s", and on chart axes "13%" and
"2.5m". Several components add their own `toFixed(1)}%`. #558 listed this as known; Pierre chose to
change it in one PR.

## Change

1. **`web_analytics/lib/format.ts`**, all in the active Lingui locale (`i18n.locale || 'en'`):
   - `formatNumber`: under 1,000 an integer (`maximumFractionDigits: 0`); from 1,000 Intl's compact
     notation with at most one decimal. English stays "1.5K" and "2.4M" (but "1K", not "1.0K"); French
     gives "1,5 k" and "2,4 M"; each language abbreviates its own way (German not below a million,
     Japanese "1.5万" from ten thousand). 999,999 reads "1M", no longer "1000.0K".
   - `formatPercent(value, decimals = 1, minimumDecimals = decimals)` (new): a value in percent,
     through Intl's percent style. "12.5%" in English, "12,5 %" in French.
   - `formatDuration(seconds)`: the messages a contact's visits already use
     (`contacts/visits/duration.ts`: "{seconds} s", "{minutes} min {seconds} s", "{hours} h {mm} min"),
     plus two new round forms, "{minutes} min" and "{hours} h". The value is rounded to the second
     first, and to the minute from an hour, so 299.6 s reads "5 min", not "4 min 60 s", and the old
     "1h 60m" cannot happen.
   - `formatValue` and `formatAxisValue` use these: percentages through `formatPercent` (one decimal,
     none on axes), axis durations "45 s" or "2,5 min".
   - `formatCurrency` reads the same locale helper; unchanged otherwise.
2. Components that wrote their own percent sign use `formatPercent`: `Delta.tsx` (period change),
   `MetricChart.tsx` (tooltip change; the sign is now written separately, "+" or "-", before the
   absolute value), `explore/ExploreTable.tsx` (share, bounce rate, scroll depth),
   `explore/BreakdownDrawer.tsx` (bounce), `FilterBuilder.tsx` (a percentage filter's value, up to two
   decimals).
3. **Catalogs**: the two new messages are added by hand to the 8 `.po` files, translated as the
   existing duration messages are (de "{minutes} Min." / "{hours} Std.", ja "{minutes} 分" /
   "{hours} 時間", the others "{minutes} min" / "{hours} h"). `npm run lingui:extract` finds exactly
   these two new ids and no missing translation; its reference churn is not kept.
4. English changes: "5m 12s" becomes "5 min 12 s", "2h 30m" "2 h 30 min", "1h 5m" "1 h 05 min",
   "45s" "45 s", and "1.0K" "1K", as a contact's visits already write them.
5. **CHANGELOG**: folded into the unreleased 44.0 Improvement bullet on the console language.

## Tests (TDD: written first, seen failing)

- **`web_analytics/lib/format.test.ts`**:
  - English: `formatNumber` (999, "1.5K", "2.4M", 999,999 → "1M"), `formatDuration` ("45 s",
    "5 min 12 s", "5 min", "2 h 30 min", "1 h 05 min", "2 h", "0 s", and the carries 299.6 → "5 min",
    7199.9 → "2 h"), `formatPercent`, `formatValue` and `formatAxisValue` for percentages, durations
    and counts.
  - French (Lingui `fr` activated, then `en`): "1,5 k", "2,4 M", "12,5 %", axis "2,5 min" and "13 %".
- The components only call these helpers: their existing tests (MetricChart, explore, broadcasts)
  still pass. `Delta`, `ExploreTable`, `BreakdownDrawer` and `FilterBuilder` have no test of their own.

## Out of scope

- The translated duration words come from the catalogs; the tests load empty catalogs, so they
  check the English source and Intl's separators, not the German or Japanese words.
- `DimensionTableWidget`'s `%` is a CSS bar width, not text.

## Verification

- `cd console && npx vitest run src/components/web_analytics src/components/broadcasts src/components/analytics`
- Then once: `npm test`, `npx tsc -b`, `npx eslint .`, and `npm run lingui:extract` (no new id
  missing a translation).
