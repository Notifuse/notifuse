import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ToolResult, ToolRunContext } from '../ai-assistant'
import type { AnalyticsQuery, AnalyticsResponse } from '../../services/api/analytics'
import type { LLMChatEvent } from '../../services/api/llm'
import {
  buildWebAnalyticsToolHandlers,
  type WebAnalyticsAiDeps,
  type WebAnalyticsAiLabels
} from './web-analytics-ai-handlers'
import { buildPeriodSummary, type InsightSnapshot } from './web-analytics-insights'
import {
  MAX_SERIES_ROWS,
  REDACTED_FILTER_VALUE,
  WEB_TOOL_NAMES,
  type PendingUiState
} from './web-analytics-ai-tools'
import type { ResolvedRange, WebDimensionFilter } from './lib/types'

// The insight battery is a ~17-query fan-out of its own, already covered where it
// lives. Mocking it is what lets this file assert the SNAPSHOT the handler derives -
// the period, the forced comparison and the filters it hands over - which is the part
// summarize_period is responsible for.
vi.mock('./web-analytics-insights', () => ({ buildPeriodSummary: vi.fn() }))

const packer = vi.mocked(buildPeriodSummary)

/** The dashboard's resolved window in every test: a full, closed week. */
const RANGE: ResolvedRange = {
  startDay: '2026-08-08',
  endDay: '2026-08-14',
  startUtc: '2026-08-08T00:00:00.000Z',
  endUtc: '2026-08-14T23:59:59.999Z'
}

const EMPTY: AnalyticsResponse = { data: [], meta: { total: 0, query: 'SELECT 1', params: [] } }

/**
 * meta.query is rendered SQL and meta.params are bind values; every response a stub
 * hands back carries both, so any formatter that leaked them would show up in an
 * assertion on the result content.
 */
const respond = (data: Record<string, unknown>[]): AnalyticsResponse => ({
  data,
  meta: {
    total: data.length,
    query: 'SELECT sessions FROM web_sessions WHERE tenant = $1',
    params: ['bind-value-42']
  }
})

// Identity-ish labels: the handlers own the wording of the model-facing content, and
// the operator-facing bubbles are `t`-built in the component. Distinct prefixes here
// make it visible WHICH label a bubble was rewritten with.
const labels: WebAnalyticsAiLabels = {
  running: (what) => `running ${what}`,
  rows: (what, count) => `${what} - ${count} rows`,
  cancelled: (what) => `cancelled ${what}`,
  failed: (what) => `failed ${what}`,
  series: (what, granularity) => `${what} per ${granularity}`,
  summary: () => 'period summary',
  // Deterministic and value-bearing, so an assertion can see WHICH search params
  // the handler handed the operator's line - the model's own summary is a
  // different string built beside it.
  periodSet: (change) =>
    `period set: ${[change.period, change.customStart, change.customEnd, change.comparison, change.timezone]
      .filter(Boolean)
      .join('|')}`,
  filtersApplied: (count) => `filters applied: ${count}`,
  filtersCleared: () => 'filters cleared',
  reportOpened: (dimensions) => `report opened: ${dimensions}`,
  navigated: (tab) => `navigated: ${tab}`,
  catalogRead: () => 'catalog read'
}

const filter = (
  dimension: string,
  values: string[],
  operator: WebDimensionFilter['operator'] = 'equals'
): WebDimensionFilter => ({ dimension, operator, values })

function createHarness(overrides: Partial<WebAnalyticsAiDeps> = {}) {
  const query = vi.fn(async (_query: AnalyticsQuery): Promise<AnalyticsResponse> => EMPTY)
  // Typed with the real parameter rather than `vi.fn(async () => {})`: an untyped
  // stub gives mock.calls an empty-tuple element type, so reading calls[0][0] - which
  // is how every navigation assertion here works - is a typecheck error even though
  // vitest runs it happily.
  const applyUiState = vi.fn(async (_change: Parameters<WebAnalyticsAiDeps['applyUiState']>[0]) => {})
  const insert = vi.fn()
  // What a sibling tool of the same round has already asked the page for. Mutable
  // and read through the getter, so a test can stage it AFTER the handlers were
  // built - which is exactly the ordering production has, and the reason a
  // snapshotted overlay would fix nothing.
  const pending: { current: PendingUiState } = { current: {} }
  const posted: { content: string; toolName?: string }[] = []
  const updates: { content: string; failed?: boolean }[] = []
  const controller = new AbortController()

  const ctx: ToolRunContext = {
    progress: (content: string, toolName?: string) => {
      posted.push({ content, toolName })
      return {
        update: (text: string, opts?: { failed?: boolean }) =>
          updates.push({ content: text, failed: opts?.failed })
      }
    },
    signal: controller.signal,
    round: 1
  }

  const deps: WebAnalyticsAiDeps = {
    workspaceId: 'ws-1',
    timezone: 'UTC',
    currentPeriod: 'previous_7_days',
    currentResolved: RANGE,
    currentComparison: 'previous_period',
    currentFilters: [],
    currentGranularity: 'day',
    query,
    applyUiState,
    pendingUiState: () => pending.current,
    labels,
    ...overrides
  }

  const handlers = buildWebAnalyticsToolHandlers(deps)

  const run = async (name: string, input: Record<string, unknown> = {}) => {
    const handler = handlers.get(name)
    if (!handler) throw new Error(`no handler registered for ${name}`)
    const event = { type: 'tool_use', tool_name: name, tool_input: input } as LLMChatEvent
    return (await handler(event, insert, ctx)) as ToolResult | undefined
  }

  return { deps, query, applyUiState, insert, pending, posted, updates, controller, handlers, run }
}

/** The cube query the handler compiled and handed to the injected client. */
const sentQuery = (
  query: ReturnType<typeof createHarness>['query'],
  index = 0
): AnalyticsQuery => query.mock.calls[index][0]

/** The single navigation a UI handler issued. */
const navigation = (applyUiState: ReturnType<typeof createHarness>['applyUiState'], index = 0) =>
  applyUiState.mock.calls[index][0]

const lines = (content: string) => content.split('\n')

beforeEach(() => {
  packer.mockReset()
  packer.mockResolvedValue('PERIOD SUMMARY\n...')
})

describe('query_web_analytics', () => {
  it('runs the compiled cube query on the injected client', async () => {
    const { run, query } = createHarness()
    query.mockResolvedValue(respond([{ channel_group: 'search-organic', sessions: 120 }]))

    await run(WEB_TOOL_NAMES.QUERY, {
      schema: 'web_sessions',
      measures: ['sessions'],
      dimensions: ['channel_group'],
      limit: 5
    })

    expect(query).toHaveBeenCalledTimes(1)
    expect(sentQuery(query)).toMatchObject({
      schema: 'web_sessions',
      measures: ['sessions'],
      dimensions: ['channel_group'],
      timezone: 'UTC',
      limit: 5,
      // A grouped query is ordered by its first measure, so the rows the limit keeps
      // are the ones worth spending the model's context on.
      order: { sessions: 'desc' }
    })
    expect(sentQuery(query).filters).toEqual([
      { member: 'created_at', operator: 'inDateRange', values: [RANGE.startUtc, RANGE.endUtc] }
    ])
  })

  it('returns the rows as CSV with the row count appended', async () => {
    const { run, query } = createHarness()
    query.mockResolvedValue(
      respond([
        { channel_group: 'search-organic', sessions: 120 },
        { channel_group: 'direct', sessions: 80 }
      ])
    )

    const result = await run(WEB_TOOL_NAMES.QUERY, {
      schema: 'web_sessions',
      measures: ['sessions'],
      dimensions: ['channel_group']
    })

    expect(lines(result!.content)).toEqual([
      `web_sessions | ${RANGE.startDay}..${RANGE.endDay} | tz UTC | filters: none`,
      'channel_group,sessions',
      'search-organic,120',
      'direct,80',
      '(2 rows)'
    ])
  })

  it('never puts the rendered SQL or its bind values in the model-facing result', async () => {
    const { run, query } = createHarness()
    query.mockResolvedValue(respond([{ sessions: 10 }]))

    const result = await run(WEB_TOOL_NAMES.QUERY, {
      schema: 'web_sessions',
      measures: ['sessions']
    })

    expect(result!.content).not.toContain('SELECT')
    expect(result!.content).not.toContain('bind-value-42')
  })

  it('caps the rows it returns and says the list was cut', async () => {
    const { run, query } = createHarness()
    const rows = Array.from({ length: 25 }, (_row, index) => ({
      channel_group: `channel-${index}`,
      sessions: 100 - index
    }))
    query.mockResolvedValue(respond(rows))

    const result = await run(WEB_TOOL_NAMES.QUERY, {
      schema: 'web_sessions',
      measures: ['sessions'],
      dimensions: ['channel_group']
    })

    const body = lines(result!.content)
    // A truncated list read as a complete one is the failure: the model would tell the
    // operator these are all the channels there are.
    expect(body[body.length - 1]).toBe(
      '(showing first 20 of 25 rows; ask for a narrower query to see the rest)'
    )
    // result header + column header + 20 capped rows + the truncation note
    expect(body).toHaveLength(23)
    expect(body).toContain('channel-19,81')
    expect(body).not.toContain('channel-20,80')
  })

  it('names the offending field when the model input fails validation', async () => {
    const { run, query, posted } = createHarness()

    const result = await run(WEB_TOOL_NAMES.QUERY, {
      schema: 'web_sessions',
      measures: ['conversion_rate']
    })

    expect(result).toEqual({
      content: expect.stringContaining('unknown measure "conversion_rate"'),
      isError: true
    })
    expect(query).not.toHaveBeenCalled()
    // Nothing ran, so nothing should have been narrated to the operator either.
    expect(posted).toEqual([])
  })

  it('reports a rejected query as an error instead of an empty table', async () => {
    const { run, query } = createHarness()
    query.mockRejectedValue(new Error('analytics engine unavailable'))

    const result = await run(WEB_TOOL_NAMES.QUERY, {
      schema: 'web_sessions',
      measures: ['sessions']
    })

    expect(result).toEqual({ content: 'analytics engine unavailable', isError: true })
  })

  it('posts a progress bubble and rewrites it with the row count when the query lands', async () => {
    const { run, query, posted, updates } = createHarness()
    query.mockResolvedValue(respond([{ device: 'mobile', sessions: 3 }]))

    await run(WEB_TOOL_NAMES.QUERY, {
      schema: 'web_sessions',
      measures: ['sessions'],
      dimensions: ['device']
    })

    expect(posted).toEqual([{ content: 'running Device', toolName: undefined }])
    expect(updates).toEqual([{ content: 'Device - 1 rows', failed: undefined }])
  })

  it('marks the progress bubble failed when the query fails', async () => {
    const { run, query, updates } = createHarness()
    query.mockRejectedValue(new Error('boom'))

    await run(WEB_TOOL_NAMES.QUERY, { schema: 'web_sessions', measures: ['sessions'] })

    expect(updates).toEqual([{ content: 'failed Sessions', failed: true }])
  })

  it('abandons the result once the run is aborted', async () => {
    const { run, query, controller, updates } = createHarness()
    // Aborted while the query was in flight: the user cancelled or started a new turn.
    query.mockImplementation(async () => {
      controller.abort()
      return respond([{ sessions: 10 }])
    })

    const result = await run(WEB_TOOL_NAMES.QUERY, {
      schema: 'web_sessions',
      measures: ['sessions']
    })

    expect(result).toBeUndefined()
    expect(updates).toEqual([{ content: 'cancelled Sessions', failed: undefined }])
  })

  it('leads a time series with its bucket column', async () => {
    const { run, query } = createHarness()
    query.mockResolvedValue(respond([{ created_at_day: '2026-08-08', sessions: 12 }]))

    const result = await run(WEB_TOOL_NAMES.QUERY, {
      schema: 'web_sessions',
      measures: ['sessions'],
      granularity: 'day'
    })

    expect(sentQuery(query).timeDimensions).toEqual([
      { dimension: 'created_at', granularity: 'day', dateRange: [RANGE.startDay, RANGE.endDay] }
    ])
    // `bucket` is not the column name the engine returns; reading the wrong one renders
    // a whole series of empty cells.
    expect(lines(result!.content)[1]).toBe('created_at_day,sessions')
    expect(lines(result!.content)[2]).toBe('2026-08-08,12')
  })

  it('asks for a whole ungrouped series rather than an arbitrary slice of it', async () => {
    const { run, query } = createHarness()
    query.mockResolvedValue(respond([{ created_at_day: '2026-08-08', sessions: 12 }]))

    await run(WEB_TOOL_NAMES.QUERY, {
      schema: 'web_sessions',
      measures: ['sessions'],
      granularity: 'day'
    })

    // A series with nothing to group by gets no ORDER BY, so a LIMIT keeps whichever
    // buckets the plan produced first - and the engine gap-fills the rest back in as
    // zeros, so the answer is not even shorter for it. It is bounded by the period.
    expect(sentQuery(query).limit).toBeUndefined()
  })

  it('still caps a series that is grouped, which the engine does order', async () => {
    const { run, query } = createHarness()
    query.mockResolvedValue(respond([{ created_at_day: '2026-08-08', device: 'mobile', sessions: 12 }]))

    await run(WEB_TOOL_NAMES.QUERY, {
      schema: 'web_sessions',
      measures: ['sessions'],
      dimensions: ['device'],
      granularity: 'day'
    })

    expect(sentQuery(query).limit).toBe(MAX_SERIES_ROWS * 2)
  })

  it('downsamples a long series across the whole span instead of dropping its recent end', async () => {
    const { run, query } = createHarness()
    const buckets = 720
    const day = (index: number) => new Date(Date.UTC(2025, 0, 1 + index)).toISOString().slice(0, 10)
    const rows = Array.from({ length: buckets }, (_row, index) => ({
      created_at_day: day(index),
      sessions: index
    }))
    // Handed back newest-first: the engine emits no ORDER BY for this shape, so the
    // row order is whatever the plan produced and the handler cannot assume one.
    query.mockResolvedValue(respond([...rows].reverse()))

    const result = await run(WEB_TOOL_NAMES.QUERY, {
      schema: 'web_sessions',
      measures: ['sessions'],
      granularity: 'day'
    })

    const stride = Math.ceil(buckets / MAX_SERIES_ROWS)
    const kept = 180
    const body = lines(result!.content)
    // header, column names, the kept buckets, the row count, the note.
    expect(body).toHaveLength(kept + 4)
    expect(body[1]).toBe('created_at_day,sessions')
    // Oldest first, and the newest bucket is the one the anchor guarantees: keeping
    // the FIRST 200 of 720 would end the series seven months before the period does,
    // which the model reports as traffic having stopped.
    expect(body[2]).toBe(`${day(3)},3`)
    expect(body[kept + 1]).toBe(`${day(buckets - 1)},${buckets - 1}`)
    expect(body[body.length - 1]).toBe(
      `note: downsampled, not truncated - one bucket in every ${stride} is shown, ` +
        `${kept} of ${buckets}, ending on the most recent`
    )
    expect(result!.content).not.toContain('showing first')
  })

  it('prints every bucket of a series that fits, with no sampling note', async () => {
    const { run, query } = createHarness()
    const rows = [
      { created_at_day: '2026-08-09', sessions: 20 },
      { created_at_day: '2026-08-08', sessions: 10 }
    ]
    query.mockResolvedValue(respond(rows))

    const result = await run(WEB_TOOL_NAMES.QUERY, {
      schema: 'web_sessions',
      measures: ['sessions'],
      granularity: 'day'
    })

    expect(lines(result!.content).slice(1)).toEqual([
      'created_at_day,sessions',
      '2026-08-08,10',
      '2026-08-09,20',
      '(2 rows)'
    ])
  })

  it('refuses a granularity the engine has no bucket for', async () => {
    const { run, query } = createHarness()

    const result = await run(WEB_TOOL_NAMES.QUERY, {
      schema: 'web_sessions',
      measures: ['sessions'],
      granularity: 'fortnight'
    })

    // Dropped instead of refused, it answers a different question from the one asked:
    // a single total presented as a trend.
    expect(result!.isError).toBe(true)
    expect(result!.content).toContain('unknown granularity "fortnight"')
    expect(query).not.toHaveBeenCalled()
  })

  it('applies the dashboard filters by default, so the answer matches the chart on screen', async () => {
    const { run, query } = createHarness({ currentFilters: [filter('device', ['mobile'])] })
    query.mockResolvedValue(respond([{ sessions: 10 }]))

    const result = await run(WEB_TOOL_NAMES.QUERY, {
      schema: 'web_sessions',
      measures: ['sessions']
    })

    expect(sentQuery(query).filters).toContainEqual({
      member: 'device',
      operator: 'equals',
      values: ['mobile']
    })
    expect(result!.content).toContain('filters: device equals mobile')
  })

  it('lets a model filter replace the dashboard filter on the same dimension', async () => {
    const { run, query } = createHarness({ currentFilters: [filter('device', ['mobile'])] })
    query.mockResolvedValue(respond([{ sessions: 10 }]))

    const result = await run(WEB_TOOL_NAMES.QUERY, {
      schema: 'web_sessions',
      measures: ['sessions'],
      filters: [{ dimension: 'device', operator: 'equals', values: ['desktop'] }]
    })

    // Both kept would AND into a condition nothing matches - an empty table the model
    // would report as "no traffic".
    const applied = sentQuery(query).filters!.filter((entry) => entry.member === 'device')
    expect(applied).toEqual([{ member: 'device', operator: 'equals', values: ['desktop'] }])
    expect(result!.content).toContain('filters: device equals desktop')
  })

  it('drops the dashboard filters when the model opts out explicitly', async () => {
    const { run, query } = createHarness({ currentFilters: [filter('device', ['mobile'])] })
    query.mockResolvedValue(respond([{ sessions: 10 }]))

    const result = await run(WEB_TOOL_NAMES.QUERY, {
      schema: 'web_sessions',
      measures: ['sessions'],
      ignore_dashboard_filters: true
    })

    expect(sentQuery(query).filters).toEqual([
      { member: 'created_at', operator: 'inDateRange', values: [RANGE.startUtc, RANGE.endUtc] }
    ])
    expect(result!.content).toContain('filters: none')
  })

  it('never inherits a contact_email filter from the dashboard, opted out or not', async () => {
    // The operator's own filter bar is an egress path no tool argument names: it is
    // parsed out of the URL and the console's FilterBuilder offers contact_email.
    const pageFilters = [filter('contact_email', ['someone@example.com']), filter('device', ['mobile'])]

    const inherited = createHarness({ currentFilters: pageFilters })
    inherited.query.mockResolvedValue(respond([{ sessions: 10 }]))
    const merged = await inherited.run(WEB_TOOL_NAMES.QUERY, {
      schema: 'web_sessions',
      measures: ['sessions']
    })

    expect(JSON.stringify(sentQuery(inherited.query))).not.toContain('someone@example.com')
    expect(sentQuery(inherited.query).filters).not.toContainEqual(
      expect.objectContaining({ member: 'contact_email' })
    )
    expect(merged!.content).toContain('filters: device equals mobile')
    expect(merged!.content).not.toContain('someone@example.com')

    const ignoring = createHarness({ currentFilters: pageFilters })
    ignoring.query.mockResolvedValue(respond([{ sessions: 10 }]))
    const isolated = await ignoring.run(WEB_TOOL_NAMES.QUERY, {
      schema: 'web_sessions',
      measures: ['sessions'],
      ignore_dashboard_filters: true
    })

    expect(JSON.stringify(sentQuery(ignoring.query))).not.toContain('someone@example.com')
    expect(isolated!.content).not.toContain('someone@example.com')
  })

  it('refuses an order key that identifies individual visitors', async () => {
    const { run, query } = createHarness()

    const result = await run(WEB_TOOL_NAMES.QUERY, {
      schema: 'web_sessions',
      measures: ['sessions'],
      order_by: 'contact_email'
    })

    expect(result!.isError).toBe(true)
    expect(result!.content).toContain('cannot order by "contact_email"')
    expect(query).not.toHaveBeenCalled()
  })

  it('refuses an order key the query does not select', async () => {
    const { run, query } = createHarness()

    const result = await run(WEB_TOOL_NAMES.QUERY, {
      schema: 'web_sessions',
      measures: ['sessions'],
      dimensions: ['device'],
      order_by: 'pageviews'
    })

    expect(result!.isError).toBe(true)
    expect(result!.content).toContain('order_by must name one of the measures or dimensions')
    expect(query).not.toHaveBeenCalled()
  })
})

describe('compare_periods', () => {
  const week = { period: 'custom', start_date: '2026-08-08', end_date: '2026-08-14' }

  it('issues one query per window and joins them on the dimension value', async () => {
    const { run, query } = createHarness()
    query
      .mockResolvedValueOnce(
        respond([
          { device: 'mobile', sessions: 100 },
          { device: 'desktop', sessions: 50 }
        ])
      )
      .mockResolvedValueOnce(respond([{ device: 'mobile', sessions: 60 }]))

    const result = await run(WEB_TOOL_NAMES.COMPARE, {
      ...week,
      schema: 'web_sessions',
      measures: ['sessions'],
      dimensions: ['device']
    })

    expect(query).toHaveBeenCalledTimes(2)
    expect(lines(result!.content).slice(1)).toEqual([
      'device,sessions,prev_sessions,sessions_change',
      'mobile,100,60,66.7',
      // No previous row: an empty change cell, never a fabricated zero.
      'desktop,50,,'
    ])
  })

  it('applies the dashboard filters to both windows and states them once', async () => {
    const { run, query } = createHarness({ currentFilters: [filter('country', ['FR'])] })

    const result = await run(WEB_TOOL_NAMES.COMPARE, {
      ...week,
      schema: 'web_sessions',
      measures: ['sessions']
    })

    for (const call of query.mock.calls) {
      expect(call[0].filters).toContainEqual({
        member: 'country',
        operator: 'equals',
        values: ['FR']
      })
    }
    expect(result!.content.match(/filters:/g)).toHaveLength(1)
    expect(result!.content).toContain('filters: country equals FR')
  })

  it('reads vs_same_dates_last_year as the same calendar dates a year earlier', async () => {
    const { run } = createHarness()

    const result = await run(WEB_TOOL_NAMES.COMPARE, {
      ...week,
      schema: 'web_sessions',
      measures: ['sessions'],
      comparison: 'vs_same_dates_last_year'
    })

    expect(lines(result!.content)[0]).toContain(
      'previous 2025-08-08..2025-08-14 (previous_year)'
    )
  })

  it('refuses "previous_year" as a comparison, because it is a period', async () => {
    const { run, query } = createHarness()

    const result = await run(WEB_TOOL_NAMES.COMPARE, {
      ...week,
      schema: 'web_sessions',
      measures: ['sessions'],
      comparison: 'previous_year'
    })

    // Read as a comparison it means "same dates last year"; read as a period it means
    // "last year". A model that swaps them produces a plausible, wrong report.
    expect(result!.isError).toBe(true)
    expect(result!.content).toContain('"previous_year" is a PERIOD, not a comparison')
    expect(query).not.toHaveBeenCalled()
  })

  it('puts the preceding window immediately before the period, without overlapping it', async () => {
    const { run } = createHarness()

    const result = await run(WEB_TOOL_NAMES.COMPARE, {
      ...week,
      schema: 'web_sessions',
      measures: ['sessions'],
      comparison: 'vs_preceding_window'
    })

    expect(lines(result!.content)[0]).toBe(
      'web_sessions | current 2026-08-08..2026-08-14 | ' +
        'previous 2026-08-01..2026-08-07 (previous_period) | tz UTC | filters: none'
    )
  })

  it('refuses more than one dimension rather than silently collapsing rows', async () => {
    const { run, query } = createHarness()

    const result = await run(WEB_TOOL_NAMES.COMPARE, {
      ...week,
      schema: 'web_sessions',
      measures: ['sessions'],
      dimensions: ['device', 'country']
    })

    expect(result!.isError).toBe(true)
    expect(result!.content).toContain('at most one dimension')
    expect(query).not.toHaveBeenCalled()
  })

  it('renders one row per measure when no dimension was given', async () => {
    const { run, query } = createHarness()
    query
      .mockResolvedValueOnce(respond([{ sessions: 100, goal_value: 10 }]))
      .mockResolvedValueOnce(respond([{ sessions: 60, goal_value: 0 }]))

    const result = await run(WEB_TOOL_NAMES.COMPARE, {
      ...week,
      schema: 'web_sessions',
      measures: ['sessions', 'goal_value']
    })

    expect(lines(result!.content).slice(1)).toEqual([
      'measure,current,previous,change_pct',
      // One decimal: the raw quotient is 66.66666666666667, ~15 characters of noise in
      // every change cell of every row.
      'sessions,100,60,66.7',
      // Empty, not "0": a zero baseline means "no previous data", and "0" reads as
      // "no change".
      'goal_value,10,0,'
    ])
  })

  it('prints the declared columns per measure, not every key the merge emits', async () => {
    const { run, query } = createHarness()
    query
      .mockResolvedValueOnce(respond([{ device: 'mobile', sessions: 100, bounce_rate: 40 }]))
      .mockResolvedValueOnce(respond([{ device: 'mobile', sessions: 60, bounce_rate: 50 }]))

    const result = await run(WEB_TOOL_NAMES.COMPARE, {
      ...week,
      schema: 'web_sessions',
      measures: ['sessions', 'bounce_rate'],
      dimensions: ['device']
    })

    expect(lines(result!.content)[1]).toBe(
      'device,sessions,prev_sessions,sessions_change,bounce_rate,prev_bounce_rate,bounce_rate_change'
    )
    // dimension_value is the merge's own key and must not reach the model as a column.
    expect(result!.content).not.toContain('dimension_value')
    expect(lines(result!.content)[2]).toBe('mobile,100,60,66.7,40,50,-20')
  })

  it('refuses a comparison window that does not exist', async () => {
    const { run, query } = createHarness({ currentPeriod: 'all_time' })

    const result = await run(WEB_TOOL_NAMES.COMPARE, {
      period: 'current',
      schema: 'web_sessions',
      measures: ['sessions'],
      comparison: 'vs_preceding_window'
    })

    // all_time already starts at the first session, so the window before it holds
    // nothing. Two windows silently reported as "no change" is the failure this
    // avoids: an error the model can narrate is strictly better.
    expect(result!.isError).toBe(true)
    expect(result!.content).toBe('period "all_time" has no window before it to compare against')
    expect(query).not.toHaveBeenCalled()
  })

  it('says what "off" means here rather than blaming the period for it', async () => {
    const { run, query } = createHarness()

    const result = await run(WEB_TOOL_NAMES.COMPARE, {
      ...week,
      schema: 'web_sessions',
      measures: ['sessions'],
      comparison: 'off'
    })

    // "off" belongs to set_dashboard_period and is not in this tool's enum. The
    // period in this call HAS a preceding window, so reporting it as one that does
    // not exist would send the model looking for a different period.
    expect(result!.isError).toBe(true)
    expect(result!.content).toContain('compare_periods always reports two windows')
    expect(result!.content).toContain('vs_preceding_window')
    expect(result!.content).not.toContain('no window before it')
    expect(query).not.toHaveBeenCalled()
  })

  it('reports the row count on the progress bubble when both windows land', async () => {
    const { run, query, posted, updates } = createHarness()
    query
      .mockResolvedValueOnce(respond([{ device: 'mobile', sessions: 100 }]))
      .mockResolvedValueOnce(respond([{ device: 'mobile', sessions: 60 }]))

    await run(WEB_TOOL_NAMES.COMPARE, {
      ...week,
      schema: 'web_sessions',
      measures: ['sessions'],
      dimensions: ['device']
    })

    expect(posted).toEqual([{ content: 'running Device', toolName: undefined }])
    expect(updates).toEqual([{ content: 'Device - 1 rows', failed: undefined }])
  })
})

describe('summarize_period', () => {
  const snapshotOf = (): InsightSnapshot => packer.mock.calls[0][0]

  it('summarises the period the dashboard is showing, never one the model names', async () => {
    const { run, deps } = createHarness({
      currentPeriod: 'custom',
      currentCustomStart: '2026-08-08',
      currentCustomEnd: '2026-08-14'
    })

    await run(WEB_TOOL_NAMES.SUMMARIZE, { period: 'previous_30_days' })

    const snapshot = snapshotOf()
    expect(snapshot.range).toEqual(RANGE)
    expect(snapshot.periodLabel).toBe('custom (2026-08-08..2026-08-14)')
    expect(snapshot.timezone).toBe('UTC')
    expect(snapshot.granularity).toBe('day')
    expect(snapshot.run).toBe(deps.query)
  })

  it('forces a comparison window when the dashboard is not comparing anything', async () => {
    const { run } = createHarness({
      currentComparison: 'none',
      currentPeriod: 'custom',
      currentCustomStart: '2026-08-08',
      currentCustomEnd: '2026-08-14'
    })

    await run(WEB_TOOL_NAMES.SUMMARIZE, {})

    // "What changed?" has no answer without a baseline, and the forced window must be
    // named or the model attributes the change to a period nobody chose.
    expect(snapshotOf().compareRange).toMatchObject({
      startDay: '2026-08-01',
      endDay: '2026-08-07'
    })
    expect(snapshotOf().compareLabel).toBe('previous_period (2026-08-01..2026-08-07)')
  })

  it('honours the comparison the model asked for over the dashboard setting', async () => {
    const { run } = createHarness({
      currentComparison: 'previous_period',
      currentPeriod: 'custom',
      currentCustomStart: '2026-08-08',
      currentCustomEnd: '2026-08-14'
    })

    await run(WEB_TOOL_NAMES.SUMMARIZE, { comparison: 'vs_same_dates_last_year' })

    expect(snapshotOf().compareLabel).toBe('previous_year (2025-08-08..2025-08-14)')
  })

  it('passes no comparison window for all_time, since nothing precedes it', async () => {
    const { run } = createHarness({ currentPeriod: 'all_time', currentComparison: 'previous_period' })

    await run(WEB_TOOL_NAMES.SUMMARIZE, {})

    expect(snapshotOf().compareRange).toBeNull()
    expect(snapshotOf().compareLabel).toBe('none (nothing precedes this range)')
  })

  it('drops a contact_email filter before the battery queries or prints it', async () => {
    const { run } = createHarness({
      currentFilters: [filter('contact_email', ['someone@example.com']), filter('device', ['mobile'])]
    })

    await run(WEB_TOOL_NAMES.SUMMARIZE, {})

    expect(snapshotOf().filters).toEqual([filter('device', ['mobile'])])
    expect(JSON.stringify(snapshotOf().filters)).not.toContain('someone@example.com')
  })

  it('posts a progress bubble and rewrites it in place when the report lands', async () => {
    const { run, posted, updates } = createHarness()

    const result = await run(WEB_TOOL_NAMES.SUMMARIZE, {})

    expect(posted).toEqual([{ content: 'period summary', toolName: WEB_TOOL_NAMES.SUMMARIZE }])
    expect(updates).toEqual([{ content: 'period summary', failed: undefined }])
    expect(result).toEqual({ content: 'PERIOD SUMMARY\n...' })
  })

  it('marks the bubble failed and reports the error when the report cannot be built', async () => {
    const { run, updates } = createHarness()
    packer.mockRejectedValue(new Error('workspace database is starting up'))

    const result = await run(WEB_TOOL_NAMES.SUMMARIZE, {})

    expect(result).toEqual({ content: 'workspace database is starting up', isError: true })
    expect(updates).toEqual([{ content: 'failed period summary', failed: true }])
  })
})

describe('list_dimensions_and_measures', () => {
  it('answers from the catalog without touching the network', async () => {
    const { run, query, insert } = createHarness()

    const result = await run(WEB_TOOL_NAMES.CATALOG, {})

    expect(query).not.toHaveBeenCalled()
    expect(result!.content).toContain('## web_sessions')
    expect(result!.content).toContain('## web_pages')
    expect(result!.content).toContain('## web_goals')
    expect(insert).toHaveBeenCalledWith('catalog read', WEB_TOOL_NAMES.CATALOG)
  })

  it('never names a dimension that identifies individual visitors', async () => {
    const { run } = createHarness()

    const result = await run(WEB_TOOL_NAMES.CATALOG, {})

    // Withheld dimensions are not merely refused when used: the model must not learn
    // they exist, or it will keep asking for them.
    expect(result!.content).not.toContain('contact_email')
    expect(result!.content).not.toContain('latitude')
    expect(result!.content).not.toContain('longitude')
  })
})

describe('UI tools', () => {
  const uiCalls: { name: string; input: Record<string, unknown>; expected: string }[] = [
    {
      name: WEB_TOOL_NAMES.SET_PERIOD,
      input: { period: 'previous_28_days' },
      expected: 'dashboard updated: period previous_28_days'
    },
    {
      name: WEB_TOOL_NAMES.SET_FILTERS,
      input: {
        mode: 'replace',
        filters: [{ dimension: 'device', operator: 'equals', values: ['mobile'] }]
      },
      expected: 'dashboard filters (replace): device equals mobile'
    },
    {
      name: WEB_TOOL_NAMES.SET_REPORT,
      input: { dimensions: ['device'] },
      expected: 'explore report opened: drill-down device, filters: none, minimum sessions per row: none'
    },
    { name: WEB_TOOL_NAMES.NAVIGATE, input: { tab: 'goals' }, expected: 'now showing the goals section' }
  ]

  it.each(uiCalls)('$name changes the page with a single navigation', async ({ name, input }) => {
    const { run, applyUiState } = createHarness()

    await run(name, input)

    // Two navigations in one tick lose the first: the second search updater reads the
    // params from before the first landed.
    expect(applyUiState).toHaveBeenCalledTimes(1)
  })

  it.each(uiCalls)('$name states the resulting state without buying a round', async ({
    name,
    input,
    expected
  }) => {
    const { run } = createHarness()

    const result = await run(name, input)

    expect(result).toEqual({ content: expected, silent: true })
  })

  it('opens an explore report with its tab and every search param in one call', async () => {
    const { run, applyUiState, insert } = createHarness()

    const result = await run(WEB_TOOL_NAMES.SET_REPORT, {
      dimensions: ['device', 'browser'],
      filters: [{ dimension: 'country', operator: 'equals', values: ['FR'] }],
      min_sessions: 25
    })

    expect(applyUiState).toHaveBeenCalledTimes(1)
    expect(applyUiState).toHaveBeenCalledWith({
      tab: 'explore',
      search: {
        dimensions: 'device,browser',
        filters: JSON.stringify([{ dimension: 'country', operator: 'equals', values: ['FR'] }]),
        minSessions: 25
      }
    })
    expect(insert).toHaveBeenCalledWith('report opened: Device / Browser', WEB_TOOL_NAMES.SET_REPORT)
    // The RESULTING state, so the next round describes the report that is on screen.
    expect(result!.content).toBe(
      'explore report opened: drill-down device > browser, filters: country equals FR, ' +
        'minimum sessions per row: 25'
    )
  })

  it('leaves the filter bar and the threshold alone when the report names neither', async () => {
    const { run, applyUiState } = createHarness({
      currentFilters: [filter('device', ['mobile'])],
      currentMinSessions: 25
    })

    const result = await run(WEB_TOOL_NAMES.SET_REPORT, { dimensions: ['landing_path'] })

    // A search param written as undefined is DROPPED by the route's validateSearch, so
    // writing the two optional params unconditionally silently reset the operator's own
    // segment and threshold to the page defaults every time a report was opened.
    expect(Object.keys(navigation(applyUiState).search ?? {})).toEqual(['dimensions'])
    // ...and the acknowledgement states what survived, or the model goes on describing
    // a segment nobody is looking at.
    expect(result!.content).toBe(
      'explore report opened: drill-down landing_path, filters: device equals mobile, ' +
        'minimum sessions per row: 25'
    )
  })

  it('clears the filter bar and the threshold when the report says so explicitly', async () => {
    const { run, applyUiState } = createHarness({
      currentFilters: [filter('device', ['mobile'])],
      currentMinSessions: 25
    })

    const result = await run(WEB_TOOL_NAMES.SET_REPORT, {
      dimensions: ['landing_path'],
      filters: [],
      min_sessions: 1
    })

    // An empty list and a threshold that hides nothing are how the two are cleared:
    // both keys are written, as the absent params the dashboard reads as "no filter"
    // and "no threshold".
    expect(navigation(applyUiState).search).toEqual({
      dimensions: 'landing_path',
      filters: undefined,
      minSessions: undefined
    })
    expect(Object.keys(navigation(applyUiState).search ?? {}).sort()).toEqual([
      'dimensions',
      'filters',
      'minSessions'
    ])
    expect(result!.content).toBe(
      'explore report opened: drill-down landing_path, filters: none, minimum sessions per row: none'
    )
  })

  it('withholds a contact_email value from the report acknowledgement it carried forward', async () => {
    const { run } = createHarness({
      currentFilters: [filter('contact_email', ['someone@example.com'])]
    })

    const result = await run(WEB_TOOL_NAMES.SET_REPORT, { dimensions: ['device'] })

    // The bar the report inherits is the operator's, and it can hold an address the
    // model must never read - the same door set_dashboard_filters guards.
    expect(result!.content).toContain(`contact_email equals ${REDACTED_FILTER_VALUE}`)
    expect(result!.content).not.toContain('someone@example.com')
  })

  it('replaces the whole filter bar in replace mode, so repeating an instruction is idempotent', async () => {
    const { run, applyUiState } = createHarness({
      currentFilters: [filter('device', ['mobile']), filter('country', ['FR'])]
    })

    await run(WEB_TOOL_NAMES.SET_FILTERS, {
      mode: 'replace',
      filters: [{ dimension: 'device', operator: 'equals', values: ['mobile'] }]
    })

    expect(applyUiState).toHaveBeenCalledWith({
      search: { filters: JSON.stringify([filter('device', ['mobile'])]) }
    })
  })

  it('replaces only the same-dimension filters in add mode', async () => {
    const { run, applyUiState, insert } = createHarness({
      currentFilters: [filter('device', ['mobile']), filter('country', ['FR'])]
    })

    const result = await run(WEB_TOOL_NAMES.SET_FILTERS, {
      mode: 'add',
      filters: [{ dimension: 'device', operator: 'equals', values: ['desktop'] }]
    })

    // Keeping both device filters would AND into a condition nothing matches, which
    // renders as an empty dashboard.
    expect(applyUiState).toHaveBeenCalledWith({
      search: {
        filters: JSON.stringify([filter('country', ['FR']), filter('device', ['desktop'])])
      }
    })
    expect(insert).toHaveBeenCalledWith('filters applied: 2', WEB_TOOL_NAMES.SET_FILTERS)
    expect(result!.content).toBe(
      'dashboard filters (add): country equals FR AND device equals desktop'
    )
  })

  it('clears the filter search param in clear mode', async () => {
    const { run, applyUiState, insert } = createHarness({
      currentFilters: [filter('device', ['mobile'])]
    })

    const result = await run(WEB_TOOL_NAMES.SET_FILTERS, { mode: 'clear' })

    expect(applyUiState).toHaveBeenCalledWith({ search: { filters: undefined } })
    expect(insert).toHaveBeenCalledWith('filters cleared', WEB_TOOL_NAMES.SET_FILTERS)
    expect(result!.content).toBe('dashboard filters cleared')
  })

  it('refuses a filter dimension no widget on the page can group by', async () => {
    const { run, applyUiState } = createHarness()

    const result = await run(WEB_TOOL_NAMES.SET_FILTERS, {
      mode: 'replace',
      filters: [{ dimension: 'page_path', operator: 'equals', values: ['/pricing'] }]
    })

    // page_path lives only on web_pages, so every widget on the page drops it: the
    // screen would not change while the acknowledgement reported the filter applied.
    expect(result!.isError).toBe(true)
    expect(result!.content).toContain('cannot be applied to the dashboard filter bar')
    expect(result!.content).toContain('web_sessions or web_goals')
    expect(applyUiState).not.toHaveBeenCalled()
  })

  it('refuses a filter mode it does not know', async () => {
    const { run, applyUiState } = createHarness()

    const result = await run(WEB_TOOL_NAMES.SET_FILTERS, { mode: 'toggle' })

    expect(result!.isError).toBe(true)
    expect(result!.content).toContain('unknown mode "toggle"')
    expect(applyUiState).not.toHaveBeenCalled()
  })

  it('clears a stale custom range when moving to a preset period', async () => {
    const { run, applyUiState } = createHarness()

    await run(WEB_TOOL_NAMES.SET_PERIOD, { period: 'previous_28_days' })

    // Left behind, the old bounds make the date picker display a range the period no
    // longer means.
    expect(applyUiState).toHaveBeenCalledWith({
      search: { period: 'previous_28_days', customStart: undefined, customEnd: undefined }
    })
  })

  it('sets a custom range from two dates', async () => {
    const { run, applyUiState } = createHarness()

    const result = await run(WEB_TOOL_NAMES.SET_PERIOD, {
      period: 'custom',
      start_date: '2026-07-01',
      end_date: '2026-07-31'
    })

    expect(applyUiState).toHaveBeenCalledWith({
      search: { period: 'custom', customStart: '2026-07-01', customEnd: '2026-07-31' }
    })
    expect(result!.content).toBe('dashboard updated: period custom (2026-07-01..2026-07-31)')
  })

  it('refuses a custom range that ends before it starts', async () => {
    const { run, applyUiState } = createHarness()

    const result = await run(WEB_TOOL_NAMES.SET_PERIOD, {
      period: 'custom',
      start_date: '2026-07-31',
      end_date: '2026-07-01'
    })

    expect(result!.isError).toBe(true)
    expect(result!.content).toBe('end_date 2026-07-01 is before start_date 2026-07-31')
    expect(applyUiState).not.toHaveBeenCalled()
  })

  it('switches the comparison off by writing the dashboard\'s own "none" mode', async () => {
    const { run, applyUiState } = createHarness()

    const result = await run(WEB_TOOL_NAMES.SET_PERIOD, { comparison: 'off' })

    expect(applyUiState).toHaveBeenCalledWith({ search: { comparison: 'none' } })
    expect(result!.content).toBe('dashboard updated: comparison none')
  })

  it('refuses a period call that names no field at all', async () => {
    const { run, applyUiState } = createHarness()

    const result = await run(WEB_TOOL_NAMES.SET_PERIOD, {})

    expect(result!.isError).toBe(true)
    expect(result!.content).toContain('needs at least one of period, comparison or timezone')
    expect(applyUiState).not.toHaveBeenCalled()
  })

  it('names the valid presets when the period is unknown', async () => {
    const { run, applyUiState } = createHarness()

    const result = await run(WEB_TOOL_NAMES.SET_PERIOD, { period: 'last_week' })

    expect(result!.isError).toBe(true)
    expect(result!.content).toContain('unknown period "last_week"')
    expect(result!.content).toContain('previous_7_days')
    expect(result!.content).toContain('"custom" with dates')
    expect(applyUiState).not.toHaveBeenCalled()
  })

  it('withholds a contact_email value from the filter acknowledgement while keeping it in the URL', async () => {
    const { run, applyUiState } = createHarness({
      currentFilters: [filter('contact_email', ['someone@example.com'])]
    })

    const result = await run(WEB_TOOL_NAMES.SET_FILTERS, {
      mode: 'add',
      filters: [{ dimension: 'device', operator: 'equals', values: ['mobile'] }]
    })

    // Dropping the operator's own filter out of their own dashboard would be the worse
    // bug, so the URL keeps it; only the sentence handed to the model is redacted.
    const written = applyUiState.mock.calls[0][0] as { search: { filters?: string } }
    expect(written.search.filters).toContain('someone@example.com')

    expect(result!.content).toBe(
      `dashboard filters (add): contact_email equals ${REDACTED_FILTER_VALUE} AND device equals mobile`
    )
    expect(result!.content).not.toContain('someone@example.com')
  })

  it('refuses to navigate to the tab where the assistant hides itself', async () => {
    const { run, applyUiState, insert } = createHarness()

    const result = await run(WEB_TOOL_NAMES.NAVIGATE, { tab: 'filters' })

    // Honouring it would make the panel disappear mid-turn, and the continuation round
    // would write its answer into an element nobody can see.
    expect(result!.isError).toBe(true)
    expect(result!.content).toContain('cannot open "filters"')
    expect(applyUiState).not.toHaveBeenCalled()
    expect(insert).not.toHaveBeenCalled()
  })

  it('navigates to a data tab', async () => {
    const { run, applyUiState, insert } = createHarness()

    await run(WEB_TOOL_NAMES.NAVIGATE, { tab: 'explore' })

    expect(applyUiState).toHaveBeenCalledWith({ tab: 'explore' })
    expect(insert).toHaveBeenCalledWith('navigated: explore', WEB_TOOL_NAMES.NAVIGATE)
  })
})

/**
 * A round's tools are dispatched synchronously against ONE frozen deps snapshot,
 * and the system prompt actively encourages batching a UI write with the query
 * that reads it. Everything below stages the overlay AFTER the handlers were
 * built - the ordering production has - so a snapshot taken at build time would
 * fail every one of them.
 */
describe('a sibling tool of the same round', () => {
  const staged: PendingUiState = {
    period: 'custom',
    customStart: '2026-07-01',
    customEnd: '2026-07-31'
  }

  it('changes the window query_web_analytics reads, not just the one it announces', async () => {
    const { run, query, pending } = createHarness()
    pending.current = staged
    query.mockResolvedValue(respond([{ sessions: 10 }]))

    const result = await run(WEB_TOOL_NAMES.QUERY, {
      schema: 'web_sessions',
      measures: ['sessions']
    })

    // The pre-baked currentResolved is the window the page showed BEFORE the sibling
    // ran; answering under it contradicts the acknowledgement the model already read.
    expect(sentQuery(query).filters).toContainEqual({
      member: 'created_at',
      operator: 'inDateRange',
      values: ['2026-07-01T00:00:00.000Z', '2026-07-31T23:59:59.999Z']
    })
    expect(lines(result!.content)[0]).toContain('2026-07-01..2026-07-31')
  })

  it('changes the window compare_periods reads', async () => {
    const { run, pending } = createHarness()
    pending.current = staged

    const result = await run(WEB_TOOL_NAMES.COMPARE, {
      period: 'current',
      schema: 'web_sessions',
      measures: ['sessions'],
      comparison: 'vs_preceding_window'
    })

    expect(lines(result!.content)[0]).toContain('current 2026-07-01..2026-07-31')
    expect(lines(result!.content)[0]).toContain('previous 2026-05-31..2026-06-30')
  })

  it('changes the period and the comparison summarize_period reports on', async () => {
    const { run, pending } = createHarness({ currentComparison: 'none' })
    pending.current = { ...staged, comparison: 'previous_year' }

    await run(WEB_TOOL_NAMES.SUMMARIZE, {})

    const snapshot = packer.mock.calls[0][0]
    expect(snapshot.periodLabel).toBe('custom (2026-07-01..2026-07-31)')
    expect(snapshot.compareLabel).toBe('previous_year (2025-07-01..2025-07-31)')
  })

  it('changes the filters a query is computed under', async () => {
    const { run, query, pending } = createHarness({ currentFilters: [filter('country', ['FR'])] })
    pending.current = { filters: [filter('device', ['mobile'])] }
    query.mockResolvedValue(respond([{ sessions: 10 }]))

    const result = await run(WEB_TOOL_NAMES.QUERY, {
      schema: 'web_sessions',
      measures: ['sessions']
    })

    expect(sentQuery(query).filters).not.toContainEqual(
      expect.objectContaining({ member: 'country' })
    )
    expect(result!.content).toContain('filters: device equals mobile')
  })

  it('lets two set_dashboard_filters calls of one round both take effect', async () => {
    const { run, applyUiState, pending } = createHarness()
    // What the first call of the round staged; without reading it the second call
    // computes "add" against an empty bar and drops it, while both acknowledgements
    // tell the model the bar holds two filters.
    pending.current = { filters: [filter('country', ['FR'])] }

    const result = await run(WEB_TOOL_NAMES.SET_FILTERS, {
      mode: 'add',
      filters: [{ dimension: 'device', operator: 'equals', values: ['mobile'] }]
    })

    expect(navigation(applyUiState).search).toEqual({
      filters: JSON.stringify([filter('country', ['FR']), filter('device', ['mobile'])])
    })
    expect(result!.content).toBe(
      'dashboard filters (add): country equals FR AND device equals mobile'
    )
  })

  it('does not switch a comparison the round just set back off', async () => {
    const { run, applyUiState, pending } = createHarness({ currentComparison: 'none' })
    // Staged by an earlier set_dashboard_period call of this same round.
    pending.current = { comparison: 'previous_year' }

    // A null is how a model expresses "leave this one alone", and it lands inside the
    // branch that writes the param - so the value written is the fallback. Taken from
    // the pre-round page it would write 'none' over the comparison the round turned
    // on, two acknowledgements apart.
    const result = await run(WEB_TOOL_NAMES.SET_PERIOD, {
      period: 'previous_28_days',
      comparison: null
    })

    expect(navigation(applyUiState).search).toMatchObject({ comparison: 'previous_year' })
    expect(result!.content).toBe(
      'dashboard updated: period previous_28_days, comparison previous_year'
    )
  })

  it('leaves the explore report carrying the filters the round already applied', async () => {
    const { run, applyUiState, pending } = createHarness({ currentFilters: [] })
    pending.current = { filters: [filter('device', ['mobile'])], minSessions: 30 }

    const result = await run(WEB_TOOL_NAMES.SET_REPORT, { dimensions: ['landing_path'] })

    expect(Object.keys(navigation(applyUiState).search ?? {})).toEqual(['dimensions'])
    expect(result!.content).toBe(
      'explore report opened: drill-down landing_path, filters: device equals mobile, ' +
        'minimum sessions per row: 30'
    )
  })

  it('does not report a section as newly opened when the round already opened it', async () => {
    const { run, applyUiState, pending } = createHarness({ currentTab: 'dashboard' })
    pending.current = { tab: 'explore', dimensions: ['device', 'browser'] }

    const result = await run(WEB_TOOL_NAMES.NAVIGATE, { tab: 'explore' })

    expect(applyUiState).toHaveBeenCalledWith({ tab: 'explore' })
    expect(result!.content).toBe('already showing the explore section, grouped by device > browser')
  })

  it('tells the model an empty explore section is empty, so it can fill it', async () => {
    const { run } = createHarness({ currentTab: 'dashboard', currentDimensions: [] })

    const result = await run(WEB_TOOL_NAMES.NAVIGATE, { tab: 'explore' })

    // Explore renders whatever drill-down the URL carries, which is frequently none;
    // the model cannot see that the section it just opened is blank.
    expect(result!.content).toBe(
      `now showing the explore section, with no drill-down configured yet - ` +
        `use ${WEB_TOOL_NAMES.SET_REPORT} to build one`
    )
  })

})

describe('the handler registry', () => {
  it('registers exactly the tools the model is offered', async () => {
    const { handlers } = createHarness()

    // A tool with no handler is refused by the hook mid-turn; a handler under a name no
    // tool declares is dead code the model can never reach.
    expect([...handlers.keys()].sort()).toEqual([...Object.values(WEB_TOOL_NAMES)].sort())
  })
})
