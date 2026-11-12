import type { Dayjs } from 'dayjs'
import dayjs from '../../../lib/dayjs'
import { toNumber } from '../../web_analytics/lib/format'
import type { EngagementGranularity } from '../broadcastAnalytics'

const HOUR_MS = 3_600_000

export interface Bucket {
  /** The local hour or day of the workspace timezone, written as the server writes it: `…Z`. */
  key: string
  value: number
}

/**
 * The key of the bucket an instant falls in. The server truncates `opened_at AT TIME ZONE tz`,
 * a timestamp without zone, and serialises it with a "Z": the local time, labelled UTC. Keys are
 * built the same way, and the server value is never converted, which would shift the chart by
 * the timezone's offset.
 */
export function bucketKey(instant: Dayjs | string, granularity: EngagementGranularity, timezone: string): string {
  const local = dayjs(instant).tz(timezone)
  return granularity === 'hour' ? local.format('YYYY-MM-DDTHH:00:00[Z]') : local.format('YYYY-MM-DDT00:00:00[Z]')
}

/** The key of a bucket the server returned, read as text. */
export function serverBucketKey(value: unknown, granularity: EngagementGranularity): string | null {
  if (typeof value !== 'string') return null
  const match = /^(\d{4}-\d{2}-\d{2})(?:[T ](\d{2}))?/.exec(value)
  if (!match) return null
  return granularity === 'hour' ? `${match[1]}T${match[2] ?? '00'}:00:00Z` : `${match[1]}T00:00:00Z`
}

/**
 * Every bucket from the one of `start` to the one of `end` (bounded by now), with the server's
 * value or 0. On a daylight-saving change an hour that does not exist locally has no bucket, and
 * an hour that occurs twice is one bucket, as the server groups it.
 */
export function fillBuckets({
  start,
  end,
  granularity,
  timezone,
  rows,
  column,
  measure,
  now = dayjs()
}: {
  start: Dayjs
  end: Dayjs
  granularity: EngagementGranularity
  timezone: string
  rows: Record<string, unknown>[]
  /** The bucket column (`opened_at_hour`). */
  column: string
  measure: string
  now?: Dayjs
}): Bucket[] {
  const values = new Map<string, number>()
  for (const row of rows) {
    const key = serverBucketKey(row[column], granularity)
    if (key) values.set(key, (values.get(key) ?? 0) + toNumber(row[measure]))
  }

  const last = (end.isAfter(now) ? now : end).valueOf()
  const keys: string[] = []
  const seen = new Set<string>()
  const push = (instant: Dayjs) => {
    const key = bucketKey(instant, granularity, timezone)
    if (!seen.has(key)) {
      seen.add(key)
      keys.push(key)
    }
  }
  if (granularity === 'hour') {
    // Absolute steps of one hour: dayjs steps in a timezone skip or repeat around a change.
    for (let t = dayjs(start).tz(timezone).startOf('hour').valueOf(); t <= last; t += HOUR_MS) push(dayjs(t))
  } else {
    let day = dayjs(start).tz(timezone).format('YYYY-MM-DD')
    for (let t = dayjs.tz(day, timezone).valueOf(); t <= last; t = dayjs.tz(day, timezone).valueOf()) {
      push(dayjs(t))
      day = dayjs.utc(day).add(1, 'day').format('YYYY-MM-DD')
    }
  }
  return keys.map((key) => ({ key, value: values.get(key) ?? 0 }))
}
