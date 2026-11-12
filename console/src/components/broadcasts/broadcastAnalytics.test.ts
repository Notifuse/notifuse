import { describe, it, expect } from 'vitest'
import dayjs from '../../lib/dayjs'
import {
  pageStatsQuery,
  statsByBroadcast,
  campaignVisitsQuery,
  campaignDayRows,
  periodTotalsQuery,
  readPeriodTotals,
  broadcastVisitsQuery,
  readBroadcastVisits,
  engagementQuery,
  pageVisitsStart
} from './broadcastAnalytics'
import type { Broadcast } from '../../services/api/broadcast'

const ISO_Z = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z$/

describe('pageStatsQuery (L2)', () => {
  it('aggregates the rows of a page by broadcast, with no date range', () => {
    expect(pageStatsQuery(['b2', 'b1'], 'Europe/Paris')).toEqual({
      schema: 'message_history',
      measures: [
        'count_sent',
        'count_failed',
        'count_opened',
        'count_opens_tracked',
        'count_clicked',
        'count_clicks_tracked',
        'count_unsubscribed',
        'count_bounced'
      ],
      dimensions: ['broadcast_id'],
      timezone: 'Europe/Paris',
      filters: [{ member: 'broadcast_id', operator: 'equals', values: ['b1', 'b2'] }]
    })
  })

  it('reads rows into numbers by broadcast', () => {
    const map = statsByBroadcast([
      { broadcast_id: 'b1', count_sent: '100', count_failed: 2, count_opened: 40, count_opens_tracked: 80 },
      { broadcast_id: null, count_sent: 5 }
    ])
    expect(map.get('b1')).toEqual({
      sent: 100,
      failed: 2,
      opened: 40,
      opensTracked: 80,
      clicked: 0,
      clicksTracked: 0,
      unsubscribed: 0,
      bounced: 0
    })
    expect(map.size).toBe(1)
  })
})

describe('campaignVisitsQuery (L3, L7)', () => {
  it('counts sessions by campaign and local day, between two instants', () => {
    const query = campaignVisitsQuery({
      campaigns: ['b', 'a', 'a'],
      start: dayjs.tz('2026-09-01', 'Europe/Paris'),
      end: dayjs('2026-11-12T12:00:00Z'),
      timezone: 'Europe/Paris'
    })
    expect(query).toEqual({
      schema: 'web_sessions',
      measures: ['sessions'],
      dimensions: ['utm_campaign', 'year', 'month', 'day'],
      timezone: 'Europe/Paris',
      filters: [
        { member: 'utm_campaign', operator: 'equals', values: ['a', 'b'] },
        { member: 'created_at', operator: 'inDateRange', values: ['2026-08-31T22:00:00.000Z', '2026-11-12T12:00:00.000Z'] }
      ]
    })
    query.filters![1].values.forEach((bound) => expect(bound).toMatch(ISO_Z))
  })

  it('adds conversions and value for the KPI', () => {
    const query = campaignVisitsQuery({
      campaigns: ['a'],
      start: dayjs('2026-10-01T00:00:00Z'),
      end: dayjs('2026-10-30T23:59:59.999Z'),
      timezone: 'UTC',
      withGoals: true
    })
    expect(query.measures).toEqual(['sessions', 'goal_conversions', 'goal_value'])
  })

  it('reads rows into campaign days', () => {
    expect(
      campaignDayRows([
        { utm_campaign: 'a', year: 2026, month: 3, day: 7, sessions: '4', goal_conversions: 1, goal_value: '9.5' },
        { utm_campaign: '', year: 2026, month: 3, day: 7, sessions: 1 }
      ])
    ).toEqual([{ campaign: 'a', day: '2026-03-07', sessions: 4, conversions: 1, value: 9.5 }])
  })
})

describe('periodTotalsQuery (L4, L5)', () => {
  it('totals the broadcast emails of a period between UTC instants', () => {
    const query = periodTotalsQuery(
      { startDay: '2026-10-13', endDay: '2026-11-11', startUtc: '2026-10-12T22:00:00.000Z', endUtc: '2026-11-11T22:59:59.999Z' },
      'Europe/Paris'
    )
    expect(query).toEqual({
      schema: 'message_history',
      measures: [
        'count_sent',
        'count_opened',
        'count_opens_tracked',
        'count_clicked',
        'count_clicks_tracked',
        'count_unsubscribed',
        'count_bounced'
      ],
      dimensions: [],
      timezone: 'Europe/Paris',
      filters: [
        { member: 'origin', operator: 'equals', values: ['broadcast'] },
        { member: 'channel', operator: 'equals', values: ['email'] },
        { member: 'created_at', operator: 'inDateRange', values: ['2026-10-12T22:00:00.000Z', '2026-11-11T22:59:59.999Z'] }
      ]
    })
  })

  it('reads the totals, zero without a row', () => {
    expect(readPeriodTotals([{ count_sent: '10', count_bounced: 1 }])).toMatchObject({ sent: 10, bounced: 1, opened: 0 })
    expect(readPeriodTotals([]).sent).toBe(0)
  })
})

describe('broadcastVisitsQuery (F6)', () => {
  it('reads the visits of one campaign over its attribution window, whole local days', () => {
    const query = broadcastVisitsQuery({
      campaign: 'news_1',
      window: { start: dayjs.tz('2026-11-01', 'Europe/Paris'), end: dayjs('2026-11-08T10:00:00Z') },
      timezone: 'Europe/Paris'
    })
    expect(query).toEqual({
      schema: 'web_sessions',
      measures: ['sessions', 'median_duration', 'goal_conversions', 'goal_value'],
      dimensions: [],
      timezone: 'Europe/Paris',
      filters: [
        { member: 'utm_campaign', operator: 'equals', values: ['news_1'] },
        { member: 'created_at', operator: 'inDateRange', values: ['2026-10-31T23:00:00.000Z', '2026-11-08T22:59:59.999Z'] }
      ]
    })
  })

  it('reads its single row', () => {
    expect(readBroadcastVisits([{ sessions: 12, median_duration: '40', goal_conversions: 2, goal_value: 30 }])).toEqual({
      sessions: 12,
      medianDuration: 40,
      conversions: 2,
      value: 30
    })
  })
})

describe('engagementQuery (F7, F8)', () => {
  it('buckets opens by hour without a date range, bounded by a filter', () => {
    const query = engagementQuery({
      broadcastId: 'b1',
      kind: 'opens',
      granularity: 'hour',
      start: dayjs('2026-11-10T08:00:00Z'),
      end: dayjs('2026-11-12T08:00:00Z'),
      timezone: 'America/New_York'
    })
    expect(query).toEqual({
      schema: 'message_history',
      measures: ['count_opened'],
      dimensions: [],
      timezone: 'America/New_York',
      timeDimensions: [{ dimension: 'opened_at', granularity: 'hour' }],
      filters: [
        { member: 'broadcast_id', operator: 'equals', values: ['b1'] },
        { member: 'opened_at', operator: 'inDateRange', values: ['2026-11-10T08:00:00.000Z', '2026-11-12T08:00:00.000Z'] }
      ]
    })
    expect(query.timeDimensions![0]).not.toHaveProperty('dateRange')
  })

  it('reads clicks on clicked_at', () => {
    const query = engagementQuery({
      broadcastId: 'b1',
      kind: 'clicks',
      granularity: 'day',
      start: dayjs('2026-11-01T00:00:00Z'),
      end: dayjs('2026-11-12T00:00:00Z'),
      timezone: 'UTC'
    })
    expect(query.measures).toEqual(['count_clicked'])
    expect(query.timeDimensions).toEqual([{ dimension: 'clicked_at', granularity: 'day' }])
    expect(query.filters![1].member).toBe('clicked_at')
  })
})

describe('pageVisitsStart', () => {
  const now = dayjs('2026-11-12T12:00:00Z')
  const b = (started_at?: string) => ({ id: 'x', started_at }) as unknown as Broadcast

  it('is the oldest start day of the page', () => {
    expect(pageVisitsStart([b('2026-11-01T10:00:00Z'), b('2026-10-20T10:00:00Z'), b()], 'UTC', now)!.toISOString()).toBe(
      '2026-10-20T00:00:00.000Z'
    )
  })

  it('is bounded at 90 days before today', () => {
    expect(pageVisitsStart([b('2026-01-01T10:00:00Z')], 'UTC', now)!.toISOString()).toBe('2026-08-14T00:00:00.000Z')
  })

  it('is null without a started row', () => {
    expect(pageVisitsStart([b()], 'UTC', now)).toBeNull()
  })
})
