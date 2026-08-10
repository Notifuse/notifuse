import { describe, expect, it } from 'vitest'
import { buildWebQuery, mergeComparisonRows } from './query'
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
