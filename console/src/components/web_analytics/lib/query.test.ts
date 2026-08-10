import { describe, expect, it } from 'vitest'
import { buildBestCombinationQuery, buildWebQuery, mergeComparisonRows } from './query'
import { ResolvedRange } from './types'

const RANGE: ResolvedRange = {
  startDay: '2026-03-08',
  endDay: '2026-03-14',
  startUtc: '2026-03-07T23:00:00.000Z',
  endUtc: '2026-03-14T22:59:59.999Z'
}

describe('buildWebQuery', () => {
  it('sends bucketed ranges as calendar days', () => {
    const query = buildWebQuery({
      schema: 'web_sessions',
      measures: ['sessions'],
      range: RANGE,
      granularity: 'day',
      timezone: 'Europe/Paris'
    })

    // The engine's gap filler parses these bounds as YYYY-MM-DD; an instant
    // here makes the whole time-series query fail rather than degrade.
    expect(query.timeDimensions).toEqual([
      { dimension: 'created_at', granularity: 'day', dateRange: ['2026-03-08', '2026-03-14'] }
    ])
    expect(query.filters).toBeUndefined()
    expect(query.timezone).toBe('Europe/Paris')
  })

  it('sends aggregate ranges as absolute instants', () => {
    const query = buildWebQuery({
      schema: 'web_sessions',
      measures: ['sessions'],
      dimensions: ['channel'],
      range: RANGE,
      timezone: 'Europe/Paris'
    })

    // A plain range filter is compared verbatim, so the local day bounds are
    // converted here rather than left for the server to guess.
    expect(query.timeDimensions).toBeUndefined()
    expect(query.filters?.[0]).toEqual({
      member: 'created_at',
      operator: 'inDateRange',
      values: ['2026-03-07T23:00:00.000Z', '2026-03-14T22:59:59.999Z']
    })
  })

  it('lets the live view range over last activity instead of session start', () => {
    // "Who is here now" is a question about the last beat, not about when the
    // visitor first landed; ranging over created_at would drop the reader who
    // has been on the page for an hour.
    const query = buildWebQuery({
      schema: 'web_sessions',
      measures: ['sessions'],
      range: RANGE,
      timeDimension: 'updated_at',
      timezone: 'UTC'
    })
    expect(query.filters?.[0].member).toBe('updated_at')
  })

  it('uses each schema time dimension', () => {
    expect(
      buildWebQuery({ schema: 'web_goals', measures: ['goals'], range: RANGE, timezone: 'UTC' })
        .filters?.[0].member
    ).toBe('goal_at')
    expect(
      buildWebQuery({ schema: 'web_pages', measures: ['page_count'], range: RANGE, timezone: 'UTC' })
        .filters?.[0].member
    ).toBe('entered_at')
  })

  it('translates empty-value filters to the empty string the columns actually store', () => {
    const query = buildWebQuery({
      schema: 'web_sessions',
      measures: ['sessions'],
      range: RANGE,
      timezone: 'UTC',
      filters: [
        { dimension: 'utm_campaign', operator: 'isNotEmpty', values: [] },
        { dimension: 'referrer_domain', operator: 'isEmpty', values: [] }
      ]
    })

    expect(query.filters).toContainEqual({
      member: 'utm_campaign',
      operator: 'notEquals',
      values: ['']
    })
    expect(query.filters).toContainEqual({
      member: 'referrer_domain',
      operator: 'equals',
      values: ['']
    })
  })

  it('passes dimension filters through with their values stringified', () => {
    const query = buildWebQuery({
      schema: 'web_sessions',
      measures: ['sessions'],
      range: RANGE,
      timezone: 'UTC',
      filters: [{ dimension: 'day_of_week', operator: 'equals', values: [3] }]
    })

    expect(query.filters).toContainEqual({
      member: 'day_of_week',
      operator: 'equals',
      values: ['3']
    })
  })

  it('renders metric filters as HAVING conditions', () => {
    const query = buildWebQuery({
      schema: 'web_sessions',
      measures: ['sessions'],
      dimensions: ['channel'],
      range: RANGE,
      timezone: 'UTC',
      metricFilters: [{ metric: 'bounce_rate', operator: 'lt', values: [50] }]
    })

    expect(query.having).toContainEqual({ member: 'bounce_rate', operator: 'lt', values: ['50'] })
  })

  it('applies the minimum-sessions threshold only to grouped session queries', () => {
    const grouped = buildWebQuery({
      schema: 'web_sessions',
      measures: ['sessions'],
      dimensions: ['channel'],
      range: RANGE,
      timezone: 'UTC',
      minSessions: 10
    })
    expect(grouped.having).toContainEqual({ member: 'sessions', operator: 'gte', values: ['10'] })

    // Without a GROUP BY the threshold would filter away the very row it is
    // meant to summarize.
    const totals = buildWebQuery({
      schema: 'web_sessions',
      measures: ['sessions'],
      range: RANGE,
      timezone: 'UTC',
      minSessions: 10
    })
    expect(totals.having).toBeUndefined()

    // Other schemas have no `sessions` measure, so the engine would reject it.
    const goals = buildWebQuery({
      schema: 'web_goals',
      measures: ['goals'],
      dimensions: ['goal_name'],
      range: RANGE,
      timezone: 'UTC',
      minSessions: 10
    })
    expect(goals.having).toBeUndefined()
  })

  it('ignores a threshold of one, which excludes nothing', () => {
    const query = buildWebQuery({
      schema: 'web_sessions',
      measures: ['sessions'],
      dimensions: ['channel'],
      range: RANGE,
      timezone: 'UTC',
      minSessions: 1
    })
    expect(query.having).toBeUndefined()
  })

  it('orders grouped queries by the first measure and leaves totals unordered', () => {
    expect(
      buildWebQuery({
        schema: 'web_sessions',
        measures: ['sessions', 'median_duration'],
        dimensions: ['channel'],
        range: RANGE,
        timezone: 'UTC'
      }).order
    ).toEqual({ sessions: 'desc' })

    expect(
      buildWebQuery({
        schema: 'web_sessions',
        measures: ['sessions'],
        range: RANGE,
        timezone: 'UTC'
      }).order
    ).toBeUndefined()
  })
})

describe('mergeComparisonRows', () => {
  it('joins on the dimension value and derives the change', () => {
    const merged = mergeComparisonRows(
      [
        { channel: 'organic-search', sessions: 150 },
        { channel: 'google-ads', sessions: 50 }
      ],
      [
        { channel: 'organic-search', sessions: 100 },
        { channel: 'newsletter', sessions: 80 }
      ],
      'channel',
      ['sessions']
    )

    expect(merged).toHaveLength(2)
    expect(merged[0]).toMatchObject({
      dimension_value: 'organic-search',
      sessions: 150,
      prev_sessions: 100,
      sessions_change: 50
    })
    // A value only present in the comparison period is not a row of "what
    // happened"; it would have no current figure to annotate.
    expect(merged.map((row) => row.dimension_value)).not.toContain('newsletter')
  })

  it('leaves the change off rows with no counterpart', () => {
    const merged = mergeComparisonRows([{ channel: 'direct', sessions: 10 }], [], 'channel', [
      'sessions'
    ])
    expect(merged[0].prev_sessions).toBeUndefined()
    expect(merged[0].sessions_change).toBeUndefined()
  })

  it('parses measures that arrive as strings from numeric columns', () => {
    const merged = mergeComparisonRows(
      [{ channel: 'direct', median_duration: '42.5' }],
      [{ channel: 'direct', median_duration: '85.0' }],
      'channel',
      ['median_duration']
    )
    expect(merged[0].median_duration).toBe(42.5)
    expect(merged[0].median_duration_change).toBe(-50)
  })

  it('keys rows by the empty string when the dimension has no value', () => {
    const merged = mergeComparisonRows(
      [{ utm_campaign: '', sessions: 5 }],
      [{ utm_campaign: '', sessions: 4 }],
      'utm_campaign',
      ['sessions']
    )
    expect(merged[0].dimension_value).toBe('')
    expect(merged[0].prev_sessions).toBe(4)
  })
})

// The explore report's "Best TimeScore" tile. Unlike every other query on the
// page this one groups by all the report's dimensions at once, because the
// best-performing combination is a property of the whole report rather than of
// a level. Keep this body in step with the request literal in
// tests/integration/web_analytics_console_queries_test.go — that test proves
// Postgres executes the shape, this one proves the console still builds it.
describe('buildWebQuery: the best-combination query', () => {
  const LEVEL_BASE = {
    schema: 'web_sessions' as const,
    measures: ['sessions', 'median_duration'],
    filters: [],
    metricFilters: [],
    minSessions: 10,
    order: { sessions: 'desc' as const },
    limit: 100,
    timezone: 'UTC'
  }

  const build = (dimensions: string[]) =>
    buildBestCombinationQuery(LEVEL_BASE, dimensions, RANGE)

  it('overrides the drill-down level defaults it is spread from', () => {
    // The level base carries the table's own order and page size. Spreading it
    // is what keeps the filters and the timezone in step, so the two overrides
    // have to come after it — sort by sessions here would name the busiest
    // combination rather than the best-performing one, which is a plausible
    // number in the wrong tile.
    const query = build(['utm_source', 'device'])

    expect(query.order).toEqual({ median_duration: 'desc' })
    expect(query.limit).toBe(1)
  })

  it('groups by every dimension of the report, in drill-down order', () => {
    // Order matters beyond the grouping: the tooltip lists the winning values
    // in this order, so it has to be the order the operator sees in the chips.
    expect(build(['utm_source', 'utm_medium', 'device']).dimensions).toEqual([
      'utm_source',
      'utm_medium',
      'device'
    ])
  })

  it('applies the session threshold, unlike the totals beside it', () => {
    // The four totals tiles deliberately ignore minSessions; this one cannot,
    // or a single-session combination wins on a median of one number.
    expect(build(['utm_source']).having).toEqual([
      { member: 'sessions', operator: 'gte', values: ['10'] }
    ])
  })

  it('ranges over session start, not last activity', () => {
    // The live view overrides the time dimension; the report must not, or the
    // winner would be drawn from a different period than the table under it.
    const query = build(['utm_source'])

    expect(query.timeDimensions).toBeUndefined()
    expect(query.filters?.[0]).toEqual({
      member: 'created_at',
      operator: 'inDateRange',
      values: [RANGE.startUtc, RANGE.endUtc]
    })
  })
})
