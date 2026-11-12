import type { KpiTone } from '../layout'

/** The message_history measures of every statistic of the page (M in spec 7). */
export interface NotificationStats {
  sent: number
  delivered: number
  failed: number
  bounced: number
  opened: number
  opensTracked: number
  clicked: number
  clicksTracked: number
}

export const STATS_MEASURES: Record<keyof NotificationStats, string> = {
  sent: 'count_sent',
  delivered: 'count_delivered',
  failed: 'count_failed',
  bounced: 'count_bounced',
  opened: 'count_opened',
  opensTracked: 'count_opens_tracked',
  clicked: 'count_clicked',
  clicksTracked: 'count_clicks_tracked'
}

const FIELDS = Object.keys(STATS_MEASURES) as (keyof NotificationStats)[]

/** Delivery below this share of the sends is a warning (the mockup's threshold). */
export const DELIVERED_WARNING = 0.97
/** Bounces from this share of the sends on are a warning. */
export const BOUNCE_WARNING = 0.02
/** Points of difference within which a rate counts as unchanged. */
const POINTS_BAND = 0.05

export function emptyStats(): NotificationStats {
  return { sent: 0, delivered: 0, failed: 0, bounced: 0, opened: 0, opensTracked: 0, clicked: 0, clicksTracked: 0 }
}

export function toNumber(value: unknown): number {
  const parsed = typeof value === 'string' ? Number(value) : value
  return typeof parsed === 'number' && Number.isFinite(parsed) ? parsed : 0
}

export function readStats(row: Record<string, unknown> | undefined): NotificationStats {
  const stats = emptyStats()
  if (!row) return stats
  for (const field of FIELDS) stats[field] = toNumber(row[STATS_MEASURES[field]])
  return stats
}

/**
 * Rows of a query grouped by `transactional_notification_id`. A notification without a message in
 * the window has no row: the engine makes no zero row when there is a dimension.
 */
export function statsByNotification(rows: Record<string, unknown>[] | undefined): Map<string, NotificationStats> {
  const map = new Map<string, NotificationStats>()
  for (const row of rows ?? []) {
    const id = row.transactional_notification_id
    if (typeof id === 'string' && id) map.set(id, readStats(row))
  }
  return map
}

export function statsOf(map: Map<string, NotificationStats> | undefined, id: string): NotificationStats {
  return map?.get(id) ?? emptyStats()
}

export function sumStats(map: Map<string, NotificationStats> | undefined, ids: string[]): NotificationStats {
  const sum = emptyStats()
  for (const id of ids) {
    const stats = statsOf(map, id)
    for (const field of FIELDS) sum[field] += stats[field]
  }
  return sum
}

/** `last_sent_at` by notification: null when every message of the window failed. */
export function lastSentByNotification(rows: Record<string, unknown>[] | undefined): Map<string, string | null> {
  const map = new Map<string, string | null>()
  for (const row of rows ?? []) {
    const id = row.transactional_notification_id
    if (typeof id !== 'string' || !id) continue
    const at = row.last_sent_at
    map.set(id, typeof at === 'string' && at ? at : null)
  }
  return map
}

const ratio = (numerator: number, denominator: number): number | null =>
  denominator > 0 ? numerator / denominator : null

export interface TrackedRate {
  rate: number | null
  /** Some emails were sent, none tracked. */
  notTracked: boolean
  /** Fewer emails tracked than sent. */
  partial: boolean
  tracked: number
}

function trackedRate(count: number, tracked: number, sent: number): TrackedRate {
  return {
    rate: ratio(count, tracked),
    notTracked: sent > 0 && tracked === 0,
    partial: tracked > 0 && tracked < sent,
    tracked
  }
}

export interface StatsRates {
  delivered: number | null
  deliveredTone: KpiTone
  bounced: number | null
  bouncedTone: KpiTone
  /** Failures over every attempt: `count_sent` leaves failures out. */
  failedShare: number | null
  opens: TrackedRate
  clicks: TrackedRate
}

export function statsRates(stats: NotificationStats): StatsRates {
  const delivered = ratio(stats.delivered, stats.sent)
  const bounced = ratio(stats.bounced, stats.sent)
  return {
    delivered,
    deliveredTone: delivered !== null && delivered < DELIVERED_WARNING ? 'warning' : 'neutral',
    bounced,
    bouncedTone: bounced !== null && bounced >= BOUNCE_WARNING ? 'warning' : 'neutral',
    failedShare: ratio(stats.failed, stats.sent + stats.failed),
    opens: trackedRate(stats.opened, stats.opensTracked, stats.sent),
    clicks: trackedRate(stats.clicked, stats.clicksTracked, stats.sent)
  }
}

export interface PointsChange {
  points: number
  tone: KpiTone
}

/** Delivery rate change in percentage points; null without a rate on either side. */
export function deliveredChange(
  current: Pick<NotificationStats, 'sent' | 'delivered'>,
  previous: Pick<NotificationStats, 'sent' | 'delivered'> | undefined
): PointsChange | null {
  const now = ratio(current.delivered, current.sent)
  const before = previous ? ratio(previous.delivered, previous.sent) : null
  if (now === null || before === null) return null
  const points = (now - before) * 100
  return { points, tone: points > POINTS_BAND ? 'positive' : points < -POINTS_BAND ? 'negative' : 'neutral' }
}

export interface DailyPoint {
  /** YYYY-MM-DD, the workspace's day as the server wrote it. */
  day: string
  sent: number
  delivered: number
  bounced: number
  failed: number
}

/**
 * Rows of the daily series. The server already fills one row per day, keyed
 * "YYYY-MM-DDT00:00:00Z" in local time: the day is the first ten characters, no timezone shift.
 */
export function parseDaily(rows: Record<string, unknown>[] | undefined): DailyPoint[] {
  return (rows ?? []).flatMap((row) => {
    const day = String(row.created_at_day ?? '').slice(0, 10)
    // A row without a valid day has no bar to go in.
    if (!/^\d{4}-\d{2}-\d{2}$/.test(day) || Number.isNaN(Date.parse(`${day}T00:00:00Z`))) return []
    return [
      {
        day,
        sent: toNumber(row.count_sent),
        delivered: toNumber(row.count_delivered),
        bounced: toNumber(row.count_bounced),
        failed: toNumber(row.count_failed)
      }
    ]
  })
}

/** The day with the most failures, or null without any. The first of equal days wins. */
export function peakFailureDay(points: DailyPoint[]): string | null {
  let peak: DailyPoint | null = null
  for (const point of points) if (point.failed > 0 && (!peak || point.failed > peak.failed)) peak = point
  return peak?.day ?? null
}
