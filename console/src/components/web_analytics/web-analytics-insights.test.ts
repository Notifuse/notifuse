import { describe, expect, it } from 'vitest'
import type { AnalyticsQuery, AnalyticsResponse } from '../../services/api/analytics'
import { MAX_RESULT_CHARS_PER_TOOL } from '../ai-assistant/wire'
import type { ResolvedRange, WebDimensionFilter } from './lib/types'
import {
  BREAKDOWNS,
  BREAKDOWN_ROWS,
  MAX_SERIES_BUCKETS,
  SUMMARY_CHAR_BUDGET,
  buildPeriodSummary,
  type InsightSnapshot
} from './web-analytics-insights'

const RANGE: ResolvedRange = {
  startDay: '2026-08-08',
  endDay: '2026-08-14',
  startUtc: '2026-08-07T22:00:00.000Z',
  endUtc: '2026-08-14T21:59:59.999Z'
}

const COMPARE: ResolvedRange = {
  startDay: '2026-08-01',
  endDay: '2026-08-07',
  startUtc: '2026-07-31T22:00:00.000Z',
  endUtc: '2026-08-07T21:59:59.999Z'
}

type Window = 'current' | 'previous'

/**
 * Every stubbed response carries a meta block, and its contents are recognisable
 * strings: meta.query is the rendered SQL and meta.params the bind values, so a
 * packer that ever reads them leaks internal schema into a model payload. Keeping
 * them here means every assertion in this file runs against a response that HAS
 * something to leak.
 */
const META = {
  total: 0,
  executionTime: 4,
  query: 'SELECT sensitive_sql_marker FROM web_sessions_local',
  params: ['bind-value-marker']
}

const respond = (rows: Record<string, unknown>[]): AnalyticsResponse => ({
  data: rows,
  meta: { ...META, total: rows.length }
})

type Kind = 'totals' | 'series' | 'breakdown' | 'goals'

interface Classified {
  kind: Kind
  dimension: string
  window: Window
}

function classify(query: AnalyticsQuery, range: ResolvedRange): Classified {
  const timeDimension = query.timeDimensions?.[0]
  const rangeFilter = (query.filters ?? []).find((filter) => filter.operator === 'inDateRange')
  const window: Window = timeDimension
    ? timeDimension.dateRange?.[0] === range.startDay
      ? 'current'
      : 'previous'
    : rangeFilter?.values[0] === range.startUtc
      ? 'current'
      : 'previous'
  if (timeDimension) return { kind: 'series', dimension: '', window }
  if (query.schema === 'web_goals') return { kind: 'goals', dimension: 'goal_name', window }
  if ((query.dimensions ?? []).length === 0) return { kind: 'totals', dimension: '', window }
  return { kind: 'breakdown', dimension: query.dimensions[0], window }
}

const TOTALS: Record<Window, Record<string, unknown>> = {
  current: {
    sessions: 8412,
    pageviews: 21030,
    pages_per_session: 2.5,
    bounce_rate: 44.1,
    median_duration: 92,
    median_scroll: 61,
    contacts: 120,
    goal_conversions: 244,
    goal_value: 12000
  },
  previous: {
    sessions: 10233,
    pageviews: 27100,
    pages_per_session: 2.65,
    bounce_rate: 41.2,
    median_duration: 88,
    median_scroll: 59,
    contacts: 133,
    goal_conversions: 300,
    goal_value: 15400
  }
}

const SERIES: Record<string, unknown>[] = [
  { created_at_day: '2026-08-08', sessions: 1402 },
  { created_at_day: '2026-08-09', sessions: 1188 },
  { created_at_day: '2026-08-10', sessions: 1310 }
]

/**
 * Raw engine rows, not merged ones: the packer runs them through
 * mergeComparisonRows itself, which is exactly how a row ends up carrying far more
 * keys than the packer prints.
 *
 * The first row's two windows are 3500 against 6000, so the merge stores a raw
 * -41.66666666666667; the second row's previous window is an explicit 0, which is
 * the "no baseline" case that must print blank rather than "0".
 */
function breakdownRows(dimension: string, window: Window): Record<string, unknown>[] {
  const sessions = window === 'current' ? [3500, 500] : [6000, 0]
  return [
    { [dimension]: `${dimension}-a`, sessions: sessions[0], bounce_rate: 44.1, median_duration: 92 },
    { [dimension]: `${dimension}-b`, sessions: sessions[1], bounce_rate: 68.3, median_duration: 31 }
  ]
}

/**
 * The riser case, on channel_group only: one row that leads the current window and sat
 * behind a wall of bulk rows in the previous one.
 *
 * This fixture behaves like the engine rather than like a canned answer — it honours
 * the `in` filter and the limit — because that is the only way the assertion can tell a
 * scoped comparison query from an independent top-N. Every other dimension falls back
 * to the shared rows.
 */
// Extends the row shape the runners actually return: an interface without an index
// signature is not assignable to Record<string, unknown>, and these fixtures are
// handed straight back to the battery as engine rows.
interface ChannelRow extends Record<string, unknown> {
  channel_group: string
  sessions: number
  bounce_rate: number
  median_duration: number
}

const PREVIOUS_CHANNELS: ChannelRow[] = [
  ...Array.from({ length: 10 }, (_row, index) => ({
    channel_group: `bulk-${index}`,
    sessions: 900 - index,
    bounce_rate: 40,
    median_duration: 60
  })),
  { channel_group: 'riser', sessions: 120, bounce_rate: 51.5, median_duration: 70 }
]

function risingChannelGroup(
  dimension: string,
  window: Window,
  query: AnalyticsQuery
): Record<string, unknown>[] {
  if (dimension !== 'channel_group') return breakdownRows(dimension, window)
  if (window === 'current') {
    return [{ channel_group: 'riser', sessions: 4200, bounce_rate: 44.1, median_duration: 92 }]
  }
  const scoping = (query.filters ?? []).find(
    (filter) => filter.member === dimension && filter.operator === 'in'
  )
  const matched = scoping
    ? PREVIOUS_CHANNELS.filter((row) => scoping.values.includes(row.channel_group))
    : PREVIOUS_CHANNELS
  return [...matched]
    .sort((left, right) => right.sessions - left.sessions)
    .slice(0, query.limit ?? matched.length)
}

function goalRows(window: Window): Record<string, unknown>[] {
  const goals = window === 'current' ? 244 : 300
  return [{ goal_name: 'signup', goals, sum_goal_value: goals * 10 }]
}

interface Fixtures {
  range?: ResolvedRange
  totals?: Record<Window, Record<string, unknown>>
  series?: Record<string, unknown>[]
  /**
   * The QUERY is handed over too, because the comparison window is not an independent
   * top-N any more: a fixture that wants to prove the scoping filter is honoured has
   * to be able to read it. Fixtures that do not care keep the two-argument shape.
   */
  breakdown?: (
    dimension: string,
    window: Window,
    query: AnalyticsQuery
  ) => Record<string, unknown>[]
  goals?: (window: Window) => Record<string, unknown>[]
  /** Returns an error message to reject the matching query with. */
  fail?: (call: Classified) => string | undefined
}

function makeRun(fixtures: Fixtures = {}) {
  const queries: AnalyticsQuery[] = []
  const calls: Classified[] = []
  const range = fixtures.range ?? RANGE
  const run = async (query: AnalyticsQuery): Promise<AnalyticsResponse> => {
    queries.push(query)
    const call = classify(query, range)
    calls.push(call)
    const failure = fixtures.fail?.(call)
    if (failure) throw new Error(failure)
    switch (call.kind) {
      case 'totals':
        return respond([(fixtures.totals ?? TOTALS)[call.window]])
      case 'series':
        return respond(fixtures.series ?? SERIES)
      case 'goals':
        return respond((fixtures.goals ?? goalRows)(call.window))
      default:
        return respond((fixtures.breakdown ?? breakdownRows)(call.dimension, call.window, query))
    }
  }
  return { run, queries, calls }
}

function snapshotOf(
  run: InsightSnapshot['run'],
  overrides: Partial<InsightSnapshot> = {}
): InsightSnapshot {
  return {
    timezone: 'Europe/Paris',
    granularity: 'day',
    filters: [],
    periodLabel: 'previous_7_days (2026-08-08..2026-08-14)',
    range: RANGE,
    compareRange: COMPARE,
    compareLabel: 'previous_period (2026-08-01..2026-08-07)',
    run,
    ...overrides
  }
}

/** The body of one `## …` section, without its heading. */
function section(summary: string, title: string): string {
  const block = summary.split('\n\n').find((candidate) => candidate.startsWith(`${title}\n`))
  return block ? block.slice(title.length + 1) : ''
}

describe('buildPeriodSummary', () => {
  it('reports every headline measure against the comparison window', async () => {
    const { run } = makeRun()
    const summary = await buildPeriodSummary(snapshotOf(run))
    const totals = section(summary, '## totals').split('\n')

    expect(totals[0]).toBe('measure,current,previous,change_pct')
    // -17.8, not -17.79732…: this is the column the operator reads down the page.
    expect(totals[1]).toBe('sessions,8412,10233,-17.8')
    expect(summary).toContain('period: previous_7_days (2026-08-08..2026-08-14), Europe/Paris')
    expect(summary).toContain('comparison: previous_period (2026-08-01..2026-08-07)')
    expect(summary).toContain('active filters: none')
  })

  it('drops the comparison columns when there is nothing to compare against', async () => {
    const { run } = makeRun()
    const summary = await buildPeriodSummary(snapshotOf(run, { compareRange: null }))

    expect(summary).toContain('comparison: none (nothing precedes this range)')
    expect(section(summary, '## totals').split('\n')[0]).toBe('measure,current')
    // Kept columns would be a column of empty cells inviting the model to narrate a
    // trend it has no data for.
    expect(section(summary, '## by channel_group (top 8)').split('\n')[0]).toBe(
      'channel_group,sessions,bounce_rate,median_duration'
    )
    expect(summary).not.toContain('prev_sessions')
    expect(summary).not.toContain('sessions_change')
  })

  it('labels the conversion rate as derived so the model never claims it as a measure', async () => {
    const { run } = makeRun()
    const summary = await buildPeriodSummary(snapshotOf(run))

    // 244/8412 and 300/10233, each to one decimal, with an empty change cell.
    expect(section(summary, '## totals')).toContain(
      'conversion_rate_pct(derived: goal_conversions/sessions),2.9,2.9,'
    )
  })

  it('downsamples a long series and says how much of it is shown', async () => {
    const rows = Array.from({ length: 200 }, (_row, index) => ({
      created_at_day: `bucket-${index}`,
      sessions: index
    }))
    const { run } = makeRun({ series: rows, breakdown: () => [], goals: () => [] })
    const summary = await buildPeriodSummary(snapshotOf(run))
    const series = section(summary, '## sessions by day').split('\n')

    expect(Math.ceil(rows.length / MAX_SERIES_BUCKETS)).toBe(3)
    // Header + kept rows + the note, and the note is the point: a model reading a
    // sampled series as a complete one narrates gaps that are sampling artefacts.
    expect(series).toHaveLength(1 + 67 + 1)
    expect(series[series.length - 1]).toBe(
      'note: downsampled - one bucket in every 3 is shown, 67 of 200'
    )
    // 200 rows at a stride of 3 leaves a remainder, so the two anchorings differ: kept
    // last, the sample ends on bucket-199; anchored on index 0 it would end on
    // bucket-198 and throw away the newest bucket - the recent end of the series the
    // operator asked about, and the very truncation downsampling replaces.
    expect(series[series.length - 2]).toBe('bucket-199,199')
    expect(series[1]).toBe('bucket-1,1')
    expect(series[2]).toBe('bucket-4,4')
  })

  it('omits a require-non-empty breakdown that holds nothing but the empty value', async () => {
    const { run } = makeRun({
      breakdown: (dimension, window) =>
        dimension === 'utm_campaign'
          ? [{ utm_campaign: '', sessions: window === 'current' ? 90 : 80 }]
          : breakdownRows(dimension, window)
    })
    const summary = await buildPeriodSummary(snapshotOf(run))

    expect(summary).not.toContain('## by utm_campaign')
    // Silently, though: it was not dropped for size, and naming it there would have
    // the model offer to fetch a table that holds nothing.
    expect(summary).not.toContain('[omitted for size:')
    expect(summary).toContain('## by referrer_domain (top 8)')
  })

  it('drops whole sections on the character budget and names the ones it dropped', async () => {
    const summary = await buildPeriodSummary(snapshotOf(makeRun(WIDE).run, { range: WIDE_RANGE }))
    const omission = summary.split('\n').find((line) => line.startsWith('[omitted for size:'))

    expect(omission).toBeDefined()
    const dropped = (omission as string)
      .replace('[omitted for size: ', '')
      .replace(' - ask for any of these directly]', '')
      .split(', ')
    // Dropping happens from the tail, so goals — the lowest-priority section — is
    // always among the casualties once anything is dropped.
    expect(dropped).toContain(`goals (top ${BREAKDOWN_ROWS})`)
    for (const title of dropped) {
      // Named AND absent: half a table left in the body would be read as a whole one.
      expect(summary).not.toContain(`## ${title}\n`)
    }
    // What survives is the highest-priority PREFIX: appending a later small section
    // over a dropped larger one would silently reorder the priority list.
    const kept = summary
      .split('\n')
      .filter((line) => line.startsWith('## '))
      .map((line) => line.slice(3))
    expect([...kept, ...dropped]).toEqual([
      'totals',
      'sessions by day',
      ...BREAKDOWNS.map((breakdown) => `by ${breakdown.dimension} (top ${BREAKDOWN_ROWS})`),
      `goals (top ${BREAKDOWN_ROWS})`
    ])
    expect(kept.slice(0, 3)).toEqual(['totals', 'sessions by day', 'by channel_group (top 8)'])
  })

  it('stays under the per-tool payload ceiling even on a wide workspace', async () => {
    const summary = await buildPeriodSummary(snapshotOf(makeRun(WIDE).run, { range: WIDE_RANGE }))

    // The budget bounds the sections; the omission line is appended after it, and the
    // headroom under the wire limit is what absorbs that line.
    expect(summary.length).toBeLessThanOrEqual(MAX_RESULT_CHARS_PER_TOOL)
    expect(summary.split('[omitted for size:')[0].length).toBeLessThanOrEqual(SUMMARY_CHAR_BUDGET)
  })

  it('prints a breakdown under the declared column list and nothing wider', async () => {
    const { run } = makeRun()
    const summary = await buildPeriodSummary(snapshotOf(run))
    const rows = section(summary, '## by channel_group (top 8)').split('\n')

    // Asserted as a literal: mergeComparisonRows hands back a row carrying the copied
    // dimension key plus a prev_/_change pair for EVERY measure, so a packer printing
    // row keys — or a measure quietly added to the battery — would widen every row and
    // invalidate the character budget the drop order rests on.
    expect(rows[0]).toBe(
      'channel_group,sessions,prev_sessions,sessions_change,bounce_rate,median_duration'
    )
    expect(rows[1]).toBe('channel_group-a,3500,6000,-41.7,44.1,92')
  })

  it('prints goals under their own column list without repeating the grouping column', async () => {
    const { run } = makeRun()
    const summary = await buildPeriodSummary(snapshotOf(run))
    const rows = section(summary, '## goals (top 8)').split('\n')

    // GOAL_COLUMNS carries value columns only, because the shared renderer prepends
    // the grouping column itself. Listing goal_name there too prints it twice in the
    // header and duplicates the first cell of every row — this is the case that fails.
    expect(rows[0]).toBe('goal_name,goals,prev_goals,goals_change,sum_goal_value')
    expect(rows[1]).toBe('signup,244,300,-18.7,2440')
  })

  it('reads the bucket column the engine actually returns', async () => {
    const { run } = makeRun()
    const summary = await buildPeriodSummary(snapshotOf(run))
    const rows = section(summary, '## sessions by day').split('\n')

    expect(rows[0]).toBe('bucket,sessions')
    // A packer reading `row.bucket` renders an empty first cell for every row and
    // still passes every header assertion in this file, so this one is on the values:
    // a bucketed query comes back keyed `created_at_day`.
    expect(rows.slice(1)).toEqual(['2026-08-08,1402', '2026-08-09,1188', '2026-08-10,1310'])
  })

  it('prints change cells to one decimal, and blank when there is no baseline', async () => {
    const { run } = makeRun()
    const summary = await buildPeriodSummary(snapshotOf(run))
    const rows = section(summary, '## by channel_group (top 8)').split('\n')

    // The merge stores the raw quotient; printing it verbatim adds ~15 characters to
    // every change cell, which is neither the documented format nor the width the
    // character budget is derived from.
    expect(rows[1]).toContain(',-41.7,')
    expect(summary).not.toContain('-41.66')
    // A zero baseline prints empty: "0" there reads as "no change" when the truth is
    // "no previous data".
    expect(rows[2]).toBe('channel_group-b,500,0,,68.3,31')
  })

  it('asks the comparison window for the rows on screen, so the biggest riser has a baseline', async () => {
    const { run, queries, calls } = makeRun({ breakdown: risingChannelGroup })
    const summary = await buildPeriodSummary(snapshotOf(run))
    const rows = section(summary, '## by channel_group (top 8)').split('\n')

    // 120 sessions in the previous window put `riser` far outside its top 8, so two
    // independent top-Ns never intersect on it and the change cell renders blank — for
    // the one row of the report the operator most wants marked as a riser.
    expect(rows[1]).toBe('riser,4200,120,3400,44.1,92')
    const comparison = calls.findIndex(
      (call) =>
        call.kind === 'breakdown' &&
        call.dimension === 'channel_group' &&
        call.window === 'previous'
    )
    const scoping = queries[comparison].filters?.find((filter) => filter.operator === 'in')
    expect(scoping).toEqual({ member: 'channel_group', operator: 'in', values: ['riser'] })
  })

  it('prints a zero baseline for a row the comparison window genuinely has no data for', async () => {
    const { run } = makeRun({
      breakdown: (dimension, window) =>
        window === 'current'
          ? breakdownRows(dimension, window)
          : breakdownRows(dimension, window).slice(0, 1)
    })
    const summary = await buildPeriodSummary(snapshotOf(run))
    const rows = section(summary, '## by channel_group (top 8)').split('\n')

    // The comparison window was asked for BOTH values by name, so a row it did not
    // answer for had no traffic then — printed as the 0 it is rather than as the empty
    // cell that used to mean "not in the other window's top-N". The blank stays on the
    // change column, where blank means "no baseline" everywhere in this report.
    expect(rows[2]).toBe('channel_group-b,500,0,,68.3,31')
  })

  it('renders a failed section as unavailable and still ships the rest', async () => {
    const { run } = makeRun({
      fail: (call) => (call.dimension === 'country' ? 'engine timeout' : undefined)
    })
    const summary = await buildPeriodSummary(snapshotOf(run))

    expect(section(summary, '## by country (top 8)')).toBe('(unavailable: engine timeout)')
    expect(summary).toContain('measure,current,previous,change_pct')
    expect(summary).toContain('## by device (top 8)')
    expect(summary).toContain('## goals (top 8)')
  })

  it('never sends a filter to a schema that cannot group by it', async () => {
    const filters: WebDimensionFilter[] = [
      { dimension: 'goal_name', operator: 'equals', values: ['signup'] },
      { dimension: 'exit_path', operator: 'equals', values: ['/pricing'] }
    ]
    const { run, queries, calls } = makeRun()
    const summary = await buildPeriodSummary(snapshotOf(run, { filters }))

    for (const [index, query] of queries.entries()) {
      const call = calls[index]
      const members = (query.filters ?? [])
        .filter((filter) => filter.operator !== 'inDateRange')
        .map((filter) => filter.member)
      // goal_name only exists on web_goals and exit_path only on web_sessions; sending
      // either to the other schema fails the WHOLE query rather than being ignored,
      // blanking a section the dashboard beside it still shows.
      const expected = call.kind === 'goals' ? ['goal_name'] : ['exit_path']
      // A comparison window carries one filter more — the values the current window
      // returned — and it is the grouping dimension of that same query, so the scoping
      // never smuggles a member the schema cannot take.
      if ((call.kind === 'breakdown' || call.kind === 'goals') && call.window === 'previous') {
        expected.push(call.dimension)
      }
      expect(members).toEqual(expected)
    }
    // The header still names both, because the summary must describe the dashboard the
    // operator is actually looking at.
    expect(summary).toContain(
      'active filters: goal_name equals signup AND exit_path equals /pricing'
    )
  })

  it('queries only the leading breakdowns when the range is long enough to risk a timeout', async () => {
    const longRange: ResolvedRange = {
      startDay: '2026-01-01',
      endDay: '2026-08-14',
      startUtc: '2025-12-31T23:00:00.000Z',
      endUtc: '2026-08-14T21:59:59.999Z'
    }
    const { run, calls } = makeRun({ range: longRange })
    await buildPeriodSummary(snapshotOf(run, { range: longRange, compareRange: null }))

    const dimensions = [
      ...new Set(calls.filter((call) => call.kind === 'breakdown').map((call) => call.dimension))
    ]
    expect(dimensions).toEqual(['channel_group', 'landing_path', 'country'])
    // The tail is genuinely skipped rather than merely absent from the battery.
    expect(BREAKDOWNS.map((breakdown) => breakdown.dimension)).toContain('device')
  })

  it('encodes a bucketed range as calendar days and an aggregate range as instants', async () => {
    const { run, queries, calls } = makeRun()
    await buildPeriodSummary(snapshotOf(run))

    const series = queries[calls.findIndex((call) => call.kind === 'series')]
    // The engine's gap filler parses these bounds as YYYY-MM-DD; an instant here fails
    // the whole time-series query rather than degrading.
    expect(series.timeDimensions).toEqual([
      { dimension: 'created_at', granularity: 'day', dateRange: ['2026-08-08', '2026-08-14'] }
    ])
    expect(series.filters).toBeUndefined()

    const totals = queries[calls.findIndex((call) => call.kind === 'totals')]
    expect(totals.timeDimensions).toBeUndefined()
    // A granularity on an aggregate query would split every row per day.
    expect(totals.filters).toEqual([
      { member: 'created_at', operator: 'inDateRange', values: [RANGE.startUtc, RANGE.endUtc] }
    ])
  })

  it('never puts the response metadata into the model payload', async () => {
    const { run } = makeRun()
    const summary = await buildPeriodSummary(snapshotOf(run))

    // meta.query is the fully rendered SQL and meta.params the bind values.
    expect(summary).not.toContain('sensitive_sql_marker')
    expect(summary).not.toContain('bind-value-marker')
    expect(summary).not.toContain('executionTime')
  })
})

/**
 * A workspace wide enough that the report cannot fit: 91 days (short of the
 * long-range cut, so all six breakdowns run), a full series, and every breakdown
 * filled to its row limit with realistic-width values.
 */
const WIDE_RANGE: ResolvedRange = {
  startDay: '2026-05-16',
  endDay: '2026-08-14',
  startUtc: '2026-05-15T22:00:00.000Z',
  endUtc: '2026-08-14T21:59:59.999Z'
}

const WIDE: Fixtures = {
  range: WIDE_RANGE,
  series: Array.from({ length: 91 }, (_row, index) => ({
    created_at_day: `2026-05-${String(index + 1).padStart(2, '0')}`,
    sessions: 1000 + index
  })),
  breakdown: (dimension, window) =>
    Array.from({ length: BREAKDOWN_ROWS }, (_row, index) => ({
      [dimension]: `/blog/${dimension}-value-${index}?utm_campaign=summer-sale`,
      sessions: (window === 'current' ? 3000 : 4000) - index * 111,
      bounce_rate: 44.1 + index,
      median_duration: 92 + index
    })),
  goals: (window) =>
    Array.from({ length: BREAKDOWN_ROWS }, (_row, index) => ({
      goal_name: `checkout-step-${index}`,
      goals: (window === 'current' ? 240 : 300) - index * 11,
      sum_goal_value: 12000 - index * 111
    }))
}
