# Web analytics amounts in the console language, and numbro removed

## Why

**No screen passes a currency today.** `MetricSummary`, `MetricChart` and `DimensionTableWidget`
accept one, but `DashboardTab` and `GoalDashboardDrawer` never pass it, `GoalCard`,
`BroadcastKpis` and `BroadcastFunnel` call `formatValue(v, 'currency')` without it, and no
workspace setting holds one. So every amount users
see goes through the no-currency branch ("999,999", "12 500" in French), in 43.0 as now. The
currency branch below is dormant: it is fixed so it is right the day a currency is wired, and the
CHANGELOG says nothing about it.

`formatCurrency` (`console/src/components/web_analytics/lib/format.ts`) is the last web analytics
formatter that abbreviates in English. It divides by a thousand or a million itself, formats the
quotient, and pastes "K" or "M" after the digits:

- 999,999 USD reads "$1,000K": the quotient 999.999 rounds to 1,000 after the unit was chosen. The
  same happens at 999,999,999 ("$1,000M"). Both are in the 43.0 code, unreachable from the UI.
- Nothing above "M": 2,345,678,000 USD reads "$2,346M".
- The suffix is English in every language: "12,5K €" in French where Intl writes "12,5 k €".

numbro is no longer imported anywhere in the console (its last importer, `utils/formatters.tsx`,
moved to Intl in #558; in 43.0 AnalyticsDashboard, SegmentsFilter and ContactsPage used it too);
only its `package.json` entry and lockfile entries (numbro, and `bignumber.js`, which
only numbro depends on) remain.

## Changes

1. **`formatCurrency`** with a currency: Intl `notation: 'compact'` with `style: 'currency'`,
   `currencyDisplay: 'narrowSymbol'`, `minimumFractionDigits: 0`, `maximumFractionDigits: 2`,
   `useGrouping: true`, from ten thousand up, compared on the rounded absolute value (9,999.5 reads
   "$10K" like 10,000). Grouping forced as in `formatNumber`: Spanish and Catalan write thousands of
   millions ("2.345,68 M €"), Japanese 万 ("￥1,234.57万"). Below ten thousand: the full amount, no
   decimals from a thousand, the cutoff on the absolute value (-1,234.567 USD reads "-$1,235").
   Intl picks the unit after rounding, so 999,999 USD reads "$1M", and each language writes its own
   unit ("12,35 k €", "2,35 Mio. €", "1,5 Md €", "R$ 2,35 mi", "￥1.23万"). German has no thousands
   unit, so a whole 12,345 € stays "12.345 €".
   The suffix regex and its comment go.
2. **`formatCurrency`** without a currency: the decimals cutoff reads the absolute value, so
   -1,234.567 reads "-1,235" like 1,234.567 reads "1,235" (today "-1,234.57").
3. **numbro**: removed from `package.json`, and its two blocks (numbro, `bignumber.js`) and root
   entry removed from `package-lock.json` by hand: `npm uninstall` with this machine's npm also
   stripped unrelated `libc` fields. Checked first that nothing else in the lockfile depends on `bignumber.js`.
4. **CHANGELOG** 44.0: only what users see. A Fix line for the negative cutoff (change 2, released
   in 43.0; goal values can be negative for refunds, `internal/domain/custom_event.go`). The 44.0
   Fix line from #558 about "2M 346 €" is removed: it described the dormant branch, which no user
   reached.

## Tests (TDD: written first, seen failing)

- **`web_analytics/lib/format.test.ts`**, `formatCurrency` describe:
  - English: 12,500 USD → "$12.5K", 999,999 → "$1M", 999,999,999 → "$1B", 2,345,678,000 → "$2.35B",
    -2,345,678 → "-$2.35M", 9,999.5 → "$10K", 9,999 → "$9,999", -1,234.567 → "-$1,235", 980 → "$980",
    12.25 → "$12.25"; without currency -1,234.567 → "-1,235".
  - French: 12,500 EUR → "12,5 k €", 2,345,678,000 → "2,35 Md €" (replacing today's "12,5K €" and
    "2 346M €" expectations), 980 → "980 €".
  - German: 12,345 EUR → "12.345 €", 2,345,678 → "2,35 Mio. €".
  - Spanish: 2,345,678,000 EUR → "2.345,68 M €" (fails without forced grouping).
  Non-breaking spaces compared as plain spaces, as today.
- The components that show amounts (`MetricSummary`, `GoalCard`, `GoalDashboardDrawer`,
  `DimensionTableWidget`, `DashboardTab`, `MetricChart`, `BroadcastKpis`, `BroadcastFunnel`) only
  call `formatValue`/`formatCurrency`. The tests of `MetricChart`, `BroadcastKpis` and
  `BroadcastFunnel` must still pass; the other five have no test of their own.

## Out of scope

- Wiring a currency to any screen (it needs a workspace setting, so a schema change).
- German has no compact unit below a million, so from 9,999.5 up it keeps cents ("9.999,67 €",
  "12.345,68 €") where 9,999.4 reads "9.999 €". Dormant like the rest of the branch.
- The chart axis for a currency metric goes through `formatNumber` (no symbol); unchanged.
- Japanese yen in compact keeps up to two decimals ("￥234.57万"): Intl's choice for the unit.

## Verification

`cd console && npx vitest run src/components/web_analytics`, then the full `npm test`,
`npm run lint`, `npm run build` (runs `tsc -b`, which type-checks the app; `tsc --noEmit -p .`
checks nothing, the root tsconfig being a solution file; the build also confirms nothing still
imports numbro).
