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
import { REDACTED_FILTER_VALUE, WEB_TOOL_NAMES } from './web-analytics-ai-tools'
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
  summary: () => 'period summary',
  periodSet: (summary) => `period set: ${summary}`,
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

  return { deps, query, applyUiState, insert, posted, updates, controller, handlers, run }
}

/** The cube query the handler compiled and handed to the injected client. */
const sentQuery = (
  query: ReturnType<typeof createHarness>['query'],
  index = 0
): AnalyticsQuery => query.mock.calls[index][0]

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

    expect(posted).toEqual([{ content: 'running sessions by Device', toolName: undefined }])
    expect(updates).toEqual([{ content: 'sessions by Device - 1 rows', failed: undefined }])
  })

  it('marks the progress bubble failed when the query fails', async () => {
    const { run, query, updates } = createHarness()
    query.mockRejectedValue(new Error('boom'))

    await run(WEB_TOOL_NAMES.QUERY, { schema: 'web_sessions', measures: ['sessions'] })

    expect(updates).toEqual([{ content: 'failed sessions', failed: true }])
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
    expect(updates).toEqual([{ content: 'cancelled sessions', failed: undefined }])
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
      comparison: 'off'
    })

    // Two identical windows silently reported as "no change" is the failure this
    // avoids: an error the model can narrate is strictly better.
    expect(result!.isError).toBe(true)
    expect(result!.content).toBe('period "all_time" has no window before it to compare against')
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

    expect(posted).toEqual([{ content: 'running sessions by Device', toolName: undefined }])
    expect(updates).toEqual([{ content: 'sessions by Device - 1 rows', failed: undefined }])
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
      expected: 'explore report opened: drill-down device'
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
    expect(insert).toHaveBeenCalledWith('report opened: device / browser', WEB_TOOL_NAMES.SET_REPORT)
    expect(result!.content).toBe('explore report opened: drill-down device > browser, 1 filter(s)')
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

describe('the handler registry', () => {
  it('registers exactly the tools the model is offered', async () => {
    const { handlers } = createHarness()

    // A tool with no handler is refused by the hook mid-turn; a handler under a name no
    // tool declares is dead code the model can never reach.
    expect([...handlers.keys()].sort()).toEqual([...Object.values(WEB_TOOL_NAMES)].sort())
  })
})
