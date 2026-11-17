import { describe, expect, it } from 'vitest'
import { buildBreakdownQuery, buildBreakdownSeriesQuery } from './breakdownQuery'

const RANGE = {
  startDay: '2026-11-10',
  endDay: '2026-11-16',
  startUtc: '2026-11-10T00:00:00Z',
  endUtc: '2026-11-16T23:59:59Z'
}
const FILTERS = [{ dimension: 'channel', operator: 'equals' as const, values: ['google-ads'] }]

describe('buildBreakdownQuery', () => {
  // The hover prefetch builds the same object: a change here would make the drawer miss its cache.
  it('queries one dimension of web_sessions over the period, at least minSessions per row, 100 rows', () => {
    expect(
      buildBreakdownQuery({
        dimension: 'device',
        filters: FILTERS,
        metricFilters: [{ metric: 'bounce_rate', operator: 'lt', values: [50] }],
        minSessions: 5,
        range: RANGE,
        timezone: 'Europe/Paris'
      })
    ).toEqual({
      schema: 'web_sessions',
      measures: ['sessions', 'median_duration', 'bounce_rate', 'median_scroll'],
      dimensions: ['device'],
      timezone: 'Europe/Paris',
      filters: [
        { member: 'created_at', operator: 'inDateRange', values: [RANGE.startUtc, RANGE.endUtc] },
        { member: 'channel', operator: 'equals', values: ['google-ads'] }
      ],
      having: [
        { member: 'bounce_rate', operator: 'lt', values: ['50'] },
        { member: 'sessions', operator: 'gte', values: ['5'] }
      ],
      order: { sessions: 'desc' },
      limit: 100
    })
  })
})

describe('buildBreakdownSeriesQuery', () => {
  it('buckets the row by the granularity, keeps its filters and timezone, with no having nor limit', () => {
    expect(
      buildBreakdownSeriesQuery({ filters: FILTERS, range: RANGE, timezone: 'Europe/Paris', granularity: 'day' })
    ).toEqual({
      schema: 'web_sessions',
      measures: ['sessions', 'median_duration', 'bounce_rate', 'median_scroll'],
      dimensions: [],
      timezone: 'Europe/Paris',
      timeDimensions: [{ dimension: 'created_at', granularity: 'day', dateRange: ['2026-11-10', '2026-11-16'] }],
      filters: [{ member: 'channel', operator: 'equals', values: ['google-ads'] }]
    })
  })
})
