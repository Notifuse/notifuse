import type { AuditPeriod, LivePeriod, LogsPeriod, LogsSearch } from './logsSearch'

const MINUTE = 60_000
const HOUR = 60 * MINUTE
const DAY = 24 * HOUR

const DURATIONS: Partial<Record<LogsPeriod, number>> = {
  '1h': HOUR,
  '24h': DAY,
  '7d': 7 * DAY,
  '30d': 30 * DAY,
  '90d': 90 * DAY
}

export interface PeriodBounds {
  from?: string
  to?: string
}

/**
 * The bounds a period sends. `now` is the tab's own clock, set when its filters change or on
 * Refresh, not the render's: the lower bound is part of the query key, rounded down to the minute
 * so two queries of the same minute share their cache.
 */
export function periodBounds(
  period: LogsPeriod,
  now: number,
  custom: PeriodBounds = {}
): PeriodBounds {
  if (period === 'custom') {
    return {
      ...(custom.from ? { from: custom.from } : {}),
      ...(custom.to ? { to: custom.to } : {})
    }
  }
  const duration = DURATIONS[period]
  if (duration === undefined) return {}
  const from = Math.floor((now - duration) / MINUTE) * MINUTE
  return { from: new Date(from).toISOString() }
}

// Each one is served by an index of message_history; list_id and external_id are not.
export const INDEXED_MESSAGE_FILTERS = [
  'id',
  'contact_email',
  'broadcast_id',
  'automation_id',
  'transactional_notification_id',
  'template_id'
] as const

export function hasIndexedMessageFilter(search: LogsSearch): boolean {
  return INDEXED_MESSAGE_FILTERS.some((key) => search[key] !== undefined)
}

/** A link naming what sent the messages opens on their whole history. */
export function defaultMessagesPeriod(search: LogsSearch): LivePeriod {
  return hasIndexedMessageFilter(search) ? 'all' : '24h'
}

export function defaultInboundPeriod(search: LogsSearch): LivePeriod {
  return search.message_id !== undefined || search.recipient_email !== undefined ? 'all' : '24h'
}

export function defaultAuditPeriod(search: LogsSearch): AuditPeriod {
  return search.target_id !== undefined ? 'all' : '7d'
}

/**
 * The longest rolling period whose messages are counted without an indexed filter. On 6 million
 * messages, 1.5 million of them in the last 30 days, messages.count read the whole table over 30
 * days: 1 to 2 seconds warm, 2 to 3.4 seconds cold. Over 7 days it took 0.2 to 0.65 seconds.
 */
export const COUNT_MAX_PERIOD: CountedPeriod = '7d'

/** A rolling period: the ones whose messages can be counted without an indexed filter. */
export type CountedPeriod = Exclude<LivePeriod, 'all'>

const COUNTED_PERIODS: readonly LivePeriod[] = ['1h', '24h', '7d', '30d']

/** Whether the counters of the Messages tab ask for numbers: the server needs a bound. */
export function canCountMessages(search: LogsSearch, period: LivePeriod): boolean {
  if (hasIndexedMessageFilter(search)) return true
  return COUNTED_PERIODS.slice(0, COUNTED_PERIODS.indexOf(COUNT_MAX_PERIOD) + 1).includes(period)
}
