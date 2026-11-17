/**
 * Where the workspace stands with sending, for the dashboard's empty states:
 * - `loading`: not known yet;
 * - `never`: no message at all;
 * - `today`: messages, but only after the period's end (today) and none in its days: the
 *   period counts complete days up to yesterday, so today's sends show tomorrow;
 * - `counted`: the period's figures say what there is to say (an error reading the latest
 *   message or the series counts as this, so the page keeps its usual figures).
 */
export type SendState = 'loading' | 'never' | 'today' | 'counted'

export interface SendStateInput {
  /** The latest message's creation; null when there is none, undefined while it loads. */
  latestAt: string | null | undefined
  latestError?: boolean
  /** The period's end (instant). */
  periodEndUtc: string
  /** Emails sent in the period's series; undefined while it loads. */
  periodSent: number | undefined
  seriesError?: boolean
}

export function sendStateOf({ latestAt, latestError, periodEndUtc, periodSent, seriesError }: SendStateInput): SendState {
  if (latestError) return 'counted'
  if (latestAt === undefined) return 'loading'
  if (latestAt === null) return 'never'
  // Parsed on both sides: the server's timestamps may carry an offset or nanoseconds.
  const latest = Date.parse(latestAt)
  const end = Date.parse(periodEndUtc)
  if (!(latest > end)) return 'counted'
  if (seriesError) return 'counted'
  if (periodSent === undefined) return 'loading'
  return periodSent === 0 ? 'today' : 'counted'
}
