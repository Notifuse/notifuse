import type { Dayjs } from 'dayjs'
import dayjs from '../../lib/dayjs'
import type { AnalyticsQuery } from '../../services/api/analytics'
import type { Broadcast } from '../../services/api/broadcast'
import type { ResolvedRange } from '../web_analytics/lib/types'
import { toNumber } from '../web_analytics/lib/format'
import type { AttributionWindow } from '../web_analytics/lib/broadcastLinks'

type Row = Record<string, unknown>

/** Days a broadcast's visits are read back: older sends show "—". */
export const VISITS_LOOKBACK_DAYS = 90

const iso = (d: Dayjs) => d.toISOString()

// ---------------------------------------------------------------------------------------------
// L2: the stats of the rows of a page, by broadcast. No date bound: the broadcast_id filter uses
// the partial index on broadcast_id.

const PAGE_STATS_MEASURES = [
  'count_sent',
  'count_failed',
  'count_opened',
  'count_opens_tracked',
  'count_clicked',
  'count_clicks_tracked',
  'count_unsubscribed',
  'count_bounced'
]

export function pageStatsQuery(broadcastIds: string[], timezone: string): AnalyticsQuery {
  return {
    schema: 'message_history',
    measures: PAGE_STATS_MEASURES,
    dimensions: ['broadcast_id'],
    timezone,
    filters: [{ member: 'broadcast_id', operator: 'equals', values: [...broadcastIds].sort() }]
  }
}

export interface EmailCounts {
  sent: number
  opened: number
  opensTracked: number
  clicked: number
  clicksTracked: number
  unsubscribed: number
  bounced: number
}

export interface BroadcastRowStats extends EmailCounts {
  failed: number
}

const readCounts = (row: Row | undefined): EmailCounts => ({
  sent: toNumber(row?.count_sent),
  opened: toNumber(row?.count_opened),
  opensTracked: toNumber(row?.count_opens_tracked),
  clicked: toNumber(row?.count_clicked),
  clicksTracked: toNumber(row?.count_clicks_tracked),
  unsubscribed: toNumber(row?.count_unsubscribed),
  bounced: toNumber(row?.count_bounced)
})

export function statsByBroadcast(rows: Row[]): Map<string, BroadcastRowStats> {
  const map = new Map<string, BroadcastRowStats>()
  for (const row of rows) {
    if (typeof row.broadcast_id !== 'string' || !row.broadcast_id) continue
    const counts = readCounts(row)
    map.set(row.broadcast_id, {
      sent: counts.sent,
      failed: toNumber(row.count_failed),
      opened: counts.opened,
      opensTracked: counts.opensTracked,
      clicked: counts.clicked,
      clicksTracked: counts.clicksTracked,
      unsubscribed: counts.unsubscribed,
      bounced: counts.bounced
    })
  }
  return map
}

// ---------------------------------------------------------------------------------------------
// L3 and L7: sessions by campaign and local day. No time dimension, hence no gap filler.

export interface CampaignVisitsInput {
  campaigns: string[]
  start: Dayjs
  end: Dayjs
  timezone: string
  /** Conversions and their value too (L7). */
  withGoals?: boolean
}

export function campaignVisitsQuery({ campaigns, start, end, timezone, withGoals }: CampaignVisitsInput): AnalyticsQuery {
  return {
    schema: 'web_sessions',
    measures: withGoals ? ['sessions', 'goal_conversions', 'goal_value'] : ['sessions'],
    dimensions: ['utm_campaign', 'year', 'month', 'day'],
    timezone,
    filters: [
      { member: 'utm_campaign', operator: 'equals', values: [...new Set(campaigns)].sort() },
      { member: 'created_at', operator: 'inDateRange', values: [iso(start), iso(end)] }
    ]
  }
}

export interface CampaignDayRow {
  campaign: string
  /** The local day, YYYY-MM-DD. */
  day: string
  sessions: number
  conversions: number
  value: number
}

const pad = (n: number) => String(n).padStart(2, '0')

export function campaignDayRows(rows: Row[]): CampaignDayRow[] {
  const result: CampaignDayRow[] = []
  for (const row of rows) {
    if (typeof row.utm_campaign !== 'string' || !row.utm_campaign) continue
    result.push({
      campaign: row.utm_campaign,
      day: `${toNumber(row.year)}-${pad(toNumber(row.month))}-${pad(toNumber(row.day))}`,
      sessions: toNumber(row.sessions),
      conversions: toNumber(row.goal_conversions),
      value: toNumber(row.goal_value)
    })
  }
  return result
}

/**
 * Where the visits of a page start: the oldest start day of its rows, bounded at
 * VISITS_LOOKBACK_DAYS before today. Null when no row has started.
 */
export function pageVisitsStart(broadcasts: Broadcast[], timezone: string, now: Dayjs = dayjs()): Dayjs | null {
  const starts = broadcasts.filter((b) => b.started_at).map((b) => dayjs(b.started_at).tz(timezone).startOf('day'))
  if (starts.length === 0) return null
  const oldest = starts.reduce((a, b) => (b.isBefore(a) ? b : a))
  const floor = now.tz(timezone).startOf('day').subtract(VISITS_LOOKBACK_DAYS, 'day')
  return oldest.isBefore(floor) ? floor : oldest
}

// ---------------------------------------------------------------------------------------------
// L4 and L5: the broadcast emails of a period, as the dashboard totals filtered on one origin.

export function periodTotalsQuery(range: ResolvedRange, timezone: string): AnalyticsQuery {
  return {
    schema: 'message_history',
    measures: PAGE_STATS_MEASURES.filter((m) => m !== 'count_failed'),
    dimensions: [],
    timezone,
    filters: [
      { member: 'origin', operator: 'equals', values: ['broadcast'] },
      { member: 'channel', operator: 'equals', values: ['email'] },
      { member: 'created_at', operator: 'inDateRange', values: [range.startUtc, range.endUtc] }
    ]
  }
}

export function readPeriodTotals(rows: Row[]): EmailCounts {
  return readCounts(rows[0])
}

// ---------------------------------------------------------------------------------------------
// F6: the visits of one broadcast over its attribution window, in whole local days.

export function broadcastVisitsQuery({
  campaign,
  window,
  timezone
}: {
  campaign: string
  window: AttributionWindow
  timezone: string
}): AnalyticsQuery {
  return {
    schema: 'web_sessions',
    measures: ['sessions', 'median_duration', 'goal_conversions', 'goal_value'],
    dimensions: [],
    timezone,
    filters: [
      { member: 'utm_campaign', operator: 'equals', values: [campaign] },
      {
        member: 'created_at',
        operator: 'inDateRange',
        values: [iso(window.start.tz(timezone).startOf('day')), iso(window.end.tz(timezone).endOf('day'))]
      }
    ]
  }
}

export interface BroadcastVisits {
  sessions: number
  medianDuration: number
  conversions: number
  value: number
}

export function readBroadcastVisits(rows: Row[]): BroadcastVisits {
  const row = rows[0]
  return {
    sessions: toNumber(row?.sessions),
    medianDuration: toNumber(row?.median_duration),
    conversions: toNumber(row?.goal_conversions),
    value: toNumber(row?.goal_value)
  }
}

// ---------------------------------------------------------------------------------------------
// F7 and F8: opens or clicks over time. No dateRange, so no gap filler: fillBuckets completes
// the empty buckets.

export type EngagementKind = 'opens' | 'clicks'
export type EngagementGranularity = 'hour' | 'day'

export const ENGAGEMENT_COLUMNS: Record<EngagementKind, { measure: string; dimension: string }> = {
  opens: { measure: 'count_opened', dimension: 'opened_at' },
  clicks: { measure: 'count_clicked', dimension: 'clicked_at' }
}

export function engagementQuery({
  broadcastId,
  kind,
  granularity,
  start,
  end,
  timezone
}: {
  broadcastId: string
  kind: EngagementKind
  granularity: EngagementGranularity
  start: Dayjs
  end: Dayjs
  timezone: string
}): AnalyticsQuery {
  const { measure, dimension } = ENGAGEMENT_COLUMNS[kind]
  return {
    schema: 'message_history',
    measures: [measure],
    dimensions: [],
    timezone,
    timeDimensions: [{ dimension, granularity }],
    filters: [
      { member: 'broadcast_id', operator: 'equals', values: [broadcastId] },
      { member: dimension, operator: 'inDateRange', values: [iso(start), iso(end)] }
    ]
  }
}
