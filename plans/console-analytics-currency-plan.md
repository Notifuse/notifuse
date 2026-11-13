# Web analytics amounts in the console language, and numbro removed

## Why

`formatCurrency` (`console/src/components/web_analytics/lib/format.ts`) is the last web analytics
formatter that abbreviates in English. It divides by a thousand or a million itself, formats the
quotient, and pastes "K" or "M" after the digits:

- 999,999 USD reads "$1,000K": the quotient 999.999 rounds to 1,000 after the unit was chosen. The
  same happens at 999,999,999 ("$1,000M"). Both shipped in 43.0.
- Nothing above "M": 2,345,678,000 USD reads "$2,346M".
- The suffix is English in every language: "12,5K €" in French where Intl writes "12,5 k €".

numbro is no longer imported anywhere in the console (`utils/formatters.tsx` moved to Intl in #558);
only its `package.json` entry and lockfile entries (numbro and its sole dependent-only
`bignumber.js`) remain.

## Changes

1. **`formatCurrency`** with a currency: Intl `notation: 'compact'` with `style: 'currency'`,
   `currencyDisplay: 'narrowSymbol'`, `minimumFractionDigits: 0`, `maximumFractionDigits: 2`, from
   ten thousand (absolute value) up. Below ten thousand, unchanged: the full amount, no decimals from
   a thousand. Intl picks the unit after rounding, so 999,999 USD reads "$1M", and each language
   writes its own unit ("12,35 k €", "2,35 Mio. €", "1,5 Md €", "R$ 2,35 mi", "￥1.23万"). German has
   no thousands unit, so 12,345 € stays "12.345 €"; that is how German writes it.
   The suffix regex and its comment go.
2. **`formatCurrency`** without a currency: the decimals cutoff reads the absolute value, so
   -1,234.567 reads "-1,235" like 1,234.567 reads "1,235" (today "-1,234.57").
3. **numbro**: `npm uninstall numbro` in `console/`, which drops numbro and `bignumber.js` from
   `package-lock.json`. Checked first that nothing else in the lockfile depends on `bignumber.js`.
4. **CHANGELOG** 44.0: the 43.0 Fix line on web analytics rounding gains "$1,000K"; the Improvement
   sentence on analytics counts adds amounts ("12,5 k €").

## Tests (TDD: written first, seen failing)

- **`web_analytics/lib/format.test.ts`**, `formatCurrency` describe:
  - English: 12,500 USD → "$12.5K", 999,999 → "$1M", 999,999,999 → "$1B", 2,345,678,000 → "$2.35B",
    -2,345,678 → "-$2.35M", 9,999 → "$9,999", 980 → "$980", 12.25 → "$12.25"; without currency
    -1,234.567 → "-1,235".
  - French: 12,500 EUR → "12,5 k €", 2,345,678,000 → "2,35 Md €" (replacing today's "12,5K €" and
    "2 346M €" expectations), 980 → "980 €".
  - German: 12,345 EUR → "12.345 €", 2,345,678 → "2,35 Mio. €".
  Non-breaking spaces compared as plain spaces, as today.
- The components that show amounts (`MetricSummary`, `GoalCard`, `GoalDashboardDrawer`,
  `DimensionTableWidget`, `DashboardTab`, `MetricChart`) only call `formatValue`/`formatCurrency`:
  their existing tests must still pass.

## Out of scope

- The chart axis for a currency metric goes through `formatNumber` (no symbol); unchanged.
- Japanese yen in compact keeps up to two decimals ("￥234.57万"): Intl's choice for the unit.

## Verification

`cd console && npx vitest run src/components/web_analytics`, then the full `npm test`,
`npx tsc --noEmit -p .`, `npm run lint`, `npm run build` (confirms nothing still imports numbro).
