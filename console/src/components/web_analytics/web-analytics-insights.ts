import type { AnalyticsQuery, AnalyticsResponse } from '../../services/api/analytics'
import dayjs from '../../lib/dayjs'
import { filtersForSchema } from './lib/dimensions'
import { toNumber } from './lib/format'
import { buildWebQuery, mergeComparisonRows } from './lib/query'
import type { Granularity, ResolvedRange, WebDimensionFilter, WebSchema } from './lib/types'
import {
  bucketColumnFor,
  describeFilters,
  formatChangePercent,
  renderCell
} from './web-analytics-ai-tools'

export interface InsightSnapshot {
  timezone: string
  granularity: Granularity
  filters: WebDimensionFilter[]
  /**
   * MODEL-FACING English, e.g. "previous_7_days (2026-08-08..2026-08-14)", built
   * by the caller from the resolvers. Deliberately NOT the translated label: the
   * whole report body is parsed by the model, and a locale-dependent body makes
   * "## by channel_group" mean different things in different browsers. The
   * translated label exists too, and is used for exactly one thing - the tool
   * bubble the operator reads.
   */
  periodLabel: string
  range: ResolvedRange
  /** Null for all_time, or when the caller could not resolve a comparison. */
  compareRange: ResolvedRange | null
  /** Model-facing too, e.g. "previous_period (2026-08-01..2026-08-07)". */
  compareLabel: string
  /** Injected so the packer is testable with a fake and no network. */
  run: (query: AnalyticsQuery) => Promise<AnalyticsResponse>
}

/**
 * Headline measures. All nine live on web_sessions, so the whole KPI block is two
 * queries rather than a join across schemas: goal_conversions and goal_value are
 * denormalised onto the session row precisely so a goal headline does not need
 * web_goals.
 */
export const TOTAL_MEASURES = [
  'sessions',
  'pageviews',
  'pages_per_session',
  'bounce_rate',
  'median_duration',
  'median_scroll',
  'contacts',
  'goal_conversions',
  'goal_value'
]

/**
 * Breakdowns, in the order the packer drops them when the budget runs out. Where
 * traffic came from outranks who they were, which outranks what they used, because
 * that is the order an operator debugs a change in.
 */
export const BREAKDOWNS = [
  { dimension: 'channel_group' },
  { dimension: 'landing_path' },
  { dimension: 'country' },
  { dimension: 'device' },
  { dimension: 'referrer_domain', requireNonEmpty: true },
  { dimension: 'utm_campaign', requireNonEmpty: true }
]

/**
 * Measures every breakdown carries, and the columns actually printed.
 *
 * These are NOT the same list, and that is the point: mergeComparisonRows returns
 * `dimension_value`, plus a copy of every key of the source row, plus a `prev_` and a
 * `_change` for EVERY measure - eleven keys for these three measures. The packer
 * prints BREAKDOWN_COLUMNS and nothing else, so the row width is a decision here
 * rather than a consequence of what the merge helper happens to emit, and the
 * character budget below can be derived from it.
 *
 * Both lists are VALUE COLUMNS ONLY. The grouping column is not in either one: the
 * shared renderer prepends the dimension's own name to the header and
 * `row.dimension_value` to each row, so listing `goal_name` here as well would print
 * it twice in the goals header and duplicate the first cell of every goals row.
 */
export const BREAKDOWN_MEASURES = ['sessions', 'bounce_rate', 'median_duration']
export const BREAKDOWN_COLUMNS = [
  'sessions',
  'prev_sessions',
  'sessions_change',
  'bounce_rate',
  'median_duration'
]
export const GOAL_MEASURES = ['goals', 'sum_goal_value']
export const GOAL_COLUMNS = ['goals', 'prev_goals', 'goals_change', 'sum_goal_value']
/** mergeComparisonRows' own suffix (lib/query.ts:283); named once, matched twice. */
export const CHANGE_SUFFIX = '_change'

export const BREAKDOWN_ROWS = 8
export const GOAL_ROWS = 8
/**
 * Headroom under the hook's MAX_RESULT_CHARS_PER_TOOL (6000), and derived from the
 * column lists above rather than guessed: header block ~320, totals ~330, series
 * ~1590 at the bucket ceiling, ~510 per six-column breakdown (plus at most one char a
 * row, where a baseline of zero prints `0` rather than blank), goals ~385. A full
 * six-breakdown report therefore lands near 5700 - deliberately OVER this budget, so
 * the drop order is exercised on a wide workspace instead of being decoration, and the
 * ~800 chars left under 6000 absorb the `[omitted for size: …]` line and values longer
 * than the estimate.
 *
 * The packer must stop on a section boundary, because clip() truncating mid-table
 * would leave the model reading half a table as if it were the whole one.
 */
export const SUMMARY_CHAR_BUDGET = 5200
/** A year at day granularity is 365 rows of noise; downsample instead. */
export const MAX_SERIES_BUCKETS = 92
/** Beyond this the fan-out risks the hook's 20s tool timeout on a cold workspace. */
export const LONG_RANGE_DAYS = 120
export const LONG_RANGE_BREAKDOWNS = 3

/** A section is appended whole or not at all, so it is built before it is measured. */
interface Section {
  title: string
  body: string
}

const round1 = (n: number) => Math.round(n * 10) / 10
// The one change formatter the whole assistant prints through (web-analytics-ai-tools.ts,
// over lib/format.ts:92's changePercent): one decimal, empty cell for a zero baseline.
// EVERY change cell in this file goes through it, the breakdowns included - and they are
// RECOMPUTED from `<measure>` and `prev_<measure>` rather than read out of the merged
// row, because mergeComparisonRows stores the raw quotient (lib/query.ts:283):
// `sessions_change` arrives as -41.66666666666667, and printing that verbatim adds ~15
// characters to every change cell of every breakdown row - neither what the worked
// sample shows nor the width SUMMARY_CHAR_BUDGET is derived from.
const changePct = formatChangePercent
/** Whole local days covered by a resolved range, both ends inclusive. */
const daysBetween = (range: ResolvedRange): number =>
  dayjs(range.endDay).diff(dayjs(range.startDay), 'day') + 1

/** allSettled value, or the reason rendered as a body the model can read. */
function bodyOf(outcome: PromiseSettledResult<string>): string {
  return outcome.status === 'fulfilled'
    ? outcome.value
    : `(unavailable: ${outcome.reason instanceof Error ? outcome.reason.message : String(outcome.reason)})`
}

export async function buildPeriodSummary(snapshot: InsightSnapshot): Promise<string> {
  const compare = snapshot.compareRange
  // Long ranges keep only the leading breakdowns: ~17 queries at maxConcurrency 2 is
  // nine waves, and TOOL_TIMEOUT_MS is 20s on a cold workspace.
  const breakdowns =
    daysBetween(snapshot.range) > LONG_RANGE_DAYS
      ? BREAKDOWNS.slice(0, LONG_RANGE_BREAKDOWNS)
      : BREAKDOWNS

  // ---- fan-out. One allSettled over the whole battery: a dimension this workspace
  // never populated must degrade to one "(unavailable: …)" section, not take the
  // report down. Each entry resolves to its RENDERED body, so a rejection carries the
  // reason and nothing else has to know which query failed.
  const totalsJob = renderTotals(snapshot, compare)
  const seriesJob = renderSeries(snapshot)
  const breakdownJobs = breakdowns.map((b) => renderBreakdown(snapshot, compare, b, BREAKDOWN_SPEC))
  const goalsJob = renderGoals(snapshot, compare)

  const settled = await Promise.allSettled([totalsJob, seriesJob, ...breakdownJobs, goalsJob])

  // ---- assembly order. Fixed, and it IS the drop order: totals, then the trend, then
  // the breakdowns in BREAKDOWNS order (where traffic came from before who they were
  // before what they used), then goals. Anything that does not fit is dropped from the
  // tail, so what survives is always the highest-priority prefix.
  const sections: Section[] = [
    { title: '## totals', body: bodyOf(settled[0]) },
    { title: `## sessions by ${snapshot.granularity}`, body: bodyOf(settled[1]) }
  ]
  breakdowns.forEach((breakdown, index) => {
    const body = bodyOf(settled[2 + index])
    // A requireNonEmpty breakdown whose rows are all the empty value carries no
    // information (referrer_domain on direct-only traffic, utm_campaign on a site that
    // never tags). renderBreakdown returns '' for that, and an empty section is
    // omitted silently - it was not dropped for size and must not appear in the
    // omitted-for-size line, or the model will offer to fetch nothing.
    if (!body) return
    sections.push({
      title: `## by ${breakdown.dimension} (top ${BREAKDOWN_ROWS})`,
      body
    })
  })
  sections.push({ title: `## goals (top ${GOAL_ROWS})`, body: bodyOf(settled[settled.length - 1]) })

  // ---- header. Always shipped, never counted against a section: it is what makes
  // every number below readable, and a summary without it is worse than no summary.
  const head = [
    'PERIOD SUMMARY',
    `period: ${snapshot.periodLabel}, ${snapshot.timezone}`,
    compare
      ? `comparison: ${snapshot.compareLabel}`
      : 'comparison: none (nothing precedes this range)',
    // Same renderer as every other tool result header (describeFilters, Step 5), so
    // "active filters:" means one thing across the whole assistant.
    `active filters: ${describeFilters(snapshot.filters)}`,
    'note: pct columns are the change versus the comparison period. Data lags live traffic by about a minute.'
  ].join('\n')

  // ---- running char accounting, checked BEFORE the append and on a section boundary.
  // Once one section does not fit, everything after it is dropped too: appending a
  // later small section over a dropped larger one would silently reorder the priority
  // list the drop order encodes.
  let out = head
  const omitted: string[] = []
  for (let index = 0; index < sections.length; index++) {
    const block = `\n\n${sections[index].title}\n${sections[index].body}`
    if (out.length + block.length > SUMMARY_CHAR_BUDGET) {
      omitted.push(...sections.slice(index).map((section) => section.title.replace('## ', '')))
      break
    }
    out += block
  }
  if (omitted.length > 0) {
    // Named, not silently absent: the model must be able to offer the operator the rest
    // instead of concluding the data does not exist.
    out += `\n\n[omitted for size: ${omitted.join(', ')} - ask for any of these directly]`
  }
  return out
}

/** Transposed: one row per measure, so the change column reads down the page. */
async function renderTotals(
  snapshot: InsightSnapshot,
  compare: ResolvedRange | null
): Promise<string> {
  const current = await runTotals(snapshot, snapshot.range)
  const previous = compare ? await runTotals(snapshot, compare) : null
  const lines = [compare ? 'measure,current,previous,change_pct' : 'measure,current']
  for (const measure of TOTAL_MEASURES) {
    const now = toNumber(current[measure])
    if (!compare || !previous) {
      lines.push(`${measure},${now}`)
      continue
    }
    const before = toNumber(previous[measure])
    lines.push(`${measure},${now},${before},${changePct(now, before)}`)
  }
  // Derived, and LABELLED derived: the engine has no conversion_rate measure and the
  // model must not present this as one it could have queried.
  const rate = (rows: Record<string, unknown>) =>
    toNumber(rows.sessions) === 0
      ? ''
      : String(round1((toNumber(rows.goal_conversions) / toNumber(rows.sessions)) * 100))
  const label = 'conversion_rate_pct(derived: goal_conversions/sessions)'
  lines.push(
    compare && previous
      ? `${label},${rate(current)},${rate(previous)},`
      : `${label},${rate(current)}`
  )
  return lines.join('\n')
}

/** The trend, downsampled rather than truncated: a clipped tail hides the recent end. */
async function renderSeries(snapshot: InsightSnapshot): Promise<string> {
  const rows = await runSeries(snapshot)
  // THE BUCKET COLUMN IS NOT CALLED `bucket`. A bucketed query comes back with the
  // time dimension suffixed by the granularity - timeBucketColumn (lib/dates.ts:199-201,
  // pinned by lib/dates.test.ts) returns `created_at_day` - so `row.bucket` is
  // undefined for every row and the whole section would render as empty cells. The
  // column is READ under the engine's name and PRINTED as `bucket`, which is the same
  // split the breakdowns make when they print `dimension_value` under the dimension's
  // own name: the declared column list stays the packer's, the lookup stays the
  // engine's. bucketColumnFor (web-analytics-ai-tools.ts) owns the schema -> time
  // dimension map, so this file does not repeat it; the series is always web_sessions.
  const bucketColumn = bucketColumnFor('web_sessions', snapshot.granularity)
  const stride = Math.ceil(rows.length / MAX_SERIES_BUCKETS)
  // ANCHORED ON THE NEWEST BUCKET, counting backwards. A stride counted forwards from
  // index 0 keeps the oldest bucket and drops the newest whenever the row count is not
  // an exact multiple of the stride - which is the clipped recent end this function
  // exists to avoid, reintroduced by the sampling itself. Counting back from the last
  // row keeps the end of the series the operator actually asked about and drops from
  // the far end instead; the kept count is the same either way.
  const kept =
    stride > 1 ? rows.filter((_row, index) => (rows.length - 1 - index) % stride === 0) : rows
  const lines = ['bucket,sessions']
  for (const row of kept) {
    lines.push(`${renderCell(row[bucketColumn])},${toNumber(row.sessions)}`)
  }
  if (stride > 1) {
    // Said explicitly, or the model reads a sampled series as a complete one and
    // narrates a "gap" that is an artefact of the sampling.
    lines.push(
      `note: downsampled - one bucket in every ${stride} is shown, ${kept.length} of ${rows.length}`
    )
  }
  return lines.join('\n')
}

/** '' when a requireNonEmpty dimension has nothing but the empty value. */
// Parameterised over the column contract rather than hard-wired to BREAKDOWN_*, so the
// goals section is a call rather than a second copy of this function.
export interface BreakdownSpec {
  schema: WebSchema
  measures: string[]
  columns: string[]
  rows: number
}

async function renderBreakdown(
  snapshot: InsightSnapshot,
  compare: ResolvedRange | null,
  breakdown: { dimension: string; requireNonEmpty?: boolean },
  spec: BreakdownSpec
): Promise<string> {
  const current = await runBreakdown(snapshot, snapshot.range, breakdown.dimension, spec)
  // The comparison window is fetched AFTER the current one and SCOPED TO ITS VALUES,
  // rather than as a second independent top-N. Two independent top-Ns leave every row
  // that was outside the previous window's top-N with no `prev_` value at all - and
  // the biggest riser is precisely the row most likely to have been outside it, so the
  // one row the operator most wants marked as a riser is the one that renders blank.
  // Asking for these values by name means the comparison query can always answer for a
  // row that is on screen, which is what lets a blank cell below mean "no traffic then"
  // instead of "not in the other window's top-N".
  const values = current.map((row) => String(row[breakdown.dimension] ?? ''))
  const previous =
    compare && values.length > 0
      ? await runBreakdown(snapshot, compare, breakdown.dimension, spec, values)
      : []
  const merged = mergeComparisonRows(current, previous, breakdown.dimension, spec.measures)
  const rows = breakdown.requireNonEmpty
    ? merged.filter((row) => String(row.dimension_value).length > 0)
    : merged
  if (rows.length === 0) return ''
  // The declared column list, not the row's own keys: mergeComparisonRows hands back
  // eleven of those. The first column prints under the dimension's own name because
  // "by channel_group" with a column called dimension_value is one more indirection.
  const columns = compare
    ? spec.columns
    : spec.columns.filter(
        (column) => !column.startsWith('prev_') && !column.endsWith(CHANGE_SUFFIX)
      )
  const lines = [[breakdown.dimension, ...columns].join(',')]
  for (const row of rows.slice(0, spec.rows)) {
    lines.push(
      [
        renderCell(row.dimension_value),
        ...columns.map((column) => {
          // A change column is RECOMPUTED through changePct, never printed from the
          // row: the merge stores the unrounded quotient. Everything else prints the
          // row's own value.
          if (!column.endsWith(CHANGE_SUFFIX)) {
            // mergeComparisonRows omits `prev_<measure>` entirely when the comparison
            // window returned no row for this value. That window was asked for these
            // exact values, so nothing came back because nothing happened: print the
            // zero, and leave the blank to the change cell, where blank already means
            // "no baseline" everywhere else in the report.
            if (column.startsWith('prev_') && row[column] === undefined) return '0'
            return renderCell(row[column])
          }
          const measure = column.slice(0, -CHANGE_SUFFIX.length)
          return changePct(toNumber(row[measure]), toNumber(row[`prev_${measure}`]))
        })
      ].join(',')
    )
  }
  return lines.join('\n')
}

export const BREAKDOWN_SPEC: BreakdownSpec = {
  schema: 'web_sessions',
  measures: BREAKDOWN_MEASURES,
  columns: BREAKDOWN_COLUMNS,
  rows: BREAKDOWN_ROWS
}
export const GOAL_SPEC: BreakdownSpec = {
  schema: 'web_goals',
  measures: GOAL_MEASURES,
  columns: GOAL_COLUMNS,
  rows: GOAL_ROWS
}

const renderGoals = (snapshot: InsightSnapshot, compare: ResolvedRange | null) =>
  renderBreakdown(snapshot, compare, { dimension: 'goal_name' }, GOAL_SPEC)

/**
 * The ungrouped headline row.
 *
 * Returns `response.data[0] ?? {}` - the ROW OBJECT, not the array - because
 * renderTotals indexes its result by measure name. An ungrouped aggregate is always
 * one row, which the engine zero-fills (readTotals reads it the same way,
 * lib/query.ts:246-252), so the `?? {}` guards a rejected or empty response rather
 * than an expected shape. Returning the array here would make every total render as
 * `undefined` - silently, since toNumber maps it to 0.
 */
async function runTotals(
  snapshot: InsightSnapshot,
  range: ResolvedRange
): Promise<Record<string, unknown>> {
  const response = await snapshot.run(
    buildWebQuery({
      schema: 'web_sessions',
      measures: TOTAL_MEASURES,
      range,
      // Narrowed per schema (lib/dimensions.ts:183-188): a goal_name filter sent to
      // web_sessions fails the WHOLE query, and the summary must reflect what is on
      // screen - filters included - or it contradicts the dashboard beside it.
      filters: filtersForSchema(snapshot.filters, 'web_sessions'),
      timezone: snapshot.timezone
    })
  )
  // `data` only. meta.query is the rendered SQL and meta.params the bind values;
  // neither may ever reach a model.
  return response.data?.[0] ?? {}
}

/**
 * The trend's raw buckets. Built through buildWebQuery's granularity path, which is
 * the only place that encodes a bucketed range as bare calendar days on
 * `timeDimensions` (lib/query.ts:113-127) - an aggregate range would go on an
 * `inDateRange` filter as full instants instead.
 */
async function runSeries(snapshot: InsightSnapshot): Promise<Record<string, unknown>[]> {
  const response = await snapshot.run(
    buildWebQuery({
      schema: 'web_sessions',
      measures: ['sessions'],
      range: snapshot.range,
      granularity: snapshot.granularity,
      filters: filtersForSchema(snapshot.filters, 'web_sessions'),
      timezone: snapshot.timezone
    })
  )
  return response.data ?? []
}

/**
 * One breakdown window. Returns the ROW ARRAY, because mergeComparisonRows joins two
 * of them on the dimension value.
 *
 * `only` restricts the grouping to a given set of dimension values, and is how the
 * comparison window is fetched: its top-N is not the current window's top-N, so it is
 * asked for the values already on screen instead of for its own leaders.
 */
async function runBreakdown(
  snapshot: InsightSnapshot,
  range: ResolvedRange,
  dimension: string,
  spec: BreakdownSpec,
  only?: string[]
): Promise<Record<string, unknown>[]> {
  const response = await snapshot.run(
    buildWebQuery({
      schema: spec.schema,
      measures: spec.measures,
      dimensions: [dimension],
      range,
      // Per schema again, and against the spec's own schema rather than web_sessions:
      // the goals section runs on web_goals, where a landing_path filter would fail
      // the query outright.
      filters: [
        ...filtersForSchema(snapshot.filters, spec.schema),
        ...(only ? [{ dimension, operator: 'in' as const, values: only }] : [])
      ],
      // buildWebQuery orders a grouped query by its first measure descending, so the
      // rows the limit keeps are the ones worth spending the row budget on. On an
      // `only` query it can drop nothing: that call passes at most spec.rows values.
      limit: spec.rows,
      timezone: snapshot.timezone
    })
  )
  return response.data ?? []
}
