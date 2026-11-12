import type { Dayjs } from 'dayjs'
import dayjs from '../../lib/dayjs'
import type { Broadcast } from '../../services/api/broadcast'
import { broadcastAttributionWindow, type AttributionWindow } from '../web_analytics/lib/broadcastLinks'
import { VISITS_LOOKBACK_DAYS, type CampaignDayRow } from './broadcastAnalytics'

const DAY = 'YYYY-MM-DD'

/** The campaign a broadcast's visits are matched on, or null. */
export const broadcastCampaign = (broadcast: Broadcast): string | null =>
  broadcast.utm_parameters?.campaign?.trim() || null

const inWindow = (day: string, window: AttributionWindow) =>
  day >= window.start.format(DAY) && day <= window.end.format(DAY)

/**
 * The sessions of a broadcast's campaign inside its attribution window (list column, L3). Null
 * without a campaign, before it started, or for a send older than VISITS_LOOKBACK_DAYS.
 */
export function broadcastVisits(
  broadcast: Broadcast,
  rows: CampaignDayRow[],
  timezone: string,
  now: Dayjs = dayjs()
): number | null {
  const campaign = broadcastCampaign(broadcast)
  const window = broadcastAttributionWindow(broadcast, timezone, now)
  if (!campaign || !window) return null
  const floor = now.tz(timezone).startOf('day').subtract(VISITS_LOOKBACK_DAYS, 'day')
  if (window.start.isBefore(floor)) return null
  return rows
    .filter((r) => r.campaign === campaign && inWindow(r.day, window))
    .reduce((sum, r) => sum + r.sessions, 0)
}

export interface TrafficTotals {
  sessions: number
  conversions: number
  value: number
}

/**
 * The Website traffic KPI (L6 and L7): for each campaign, a day of the period counts once when
 * it falls in the window of at least one broadcast of that campaign.
 */
export function kpiTraffic(
  broadcasts: Broadcast[],
  rows: CampaignDayRow[],
  period: { startDay: string; endDay: string },
  timezone: string,
  now: Dayjs = dayjs()
): TrafficTotals {
  const windows = new Map<string, AttributionWindow[]>()
  for (const broadcast of broadcasts) {
    const campaign = broadcastCampaign(broadcast)
    const window = broadcastAttributionWindow(broadcast, timezone, now)
    if (!campaign || !window) continue
    windows.set(campaign, [...(windows.get(campaign) ?? []), window])
  }
  const totals: TrafficTotals = { sessions: 0, conversions: 0, value: 0 }
  for (const r of rows) {
    if (r.day < period.startDay || r.day > period.endDay) continue
    if (!(windows.get(r.campaign) ?? []).some((w) => inWindow(r.day, w))) continue
    totals.sessions += r.sessions
    totals.conversions += r.conversions
    totals.value += r.value
  }
  return totals
}

/** The campaigns of a set of broadcasts, without empty ones. */
export const campaignsOf = (broadcasts: Broadcast[]): string[] => [
  ...new Set(broadcasts.map(broadcastCampaign).filter((c): c is string => c !== null))
]
