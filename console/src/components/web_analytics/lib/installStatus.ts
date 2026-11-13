import { useMemo } from 'react'
import dayjs from '../../../lib/dayjs'
import { useWebAnalytics } from '../useWebAnalytics'
import { buildWebQuery, readTotals, useWebQuery } from './query'
import { ResolvedRange } from './types'

/**
 * Last activity rather than session start. The question these probes ask is
 * "is the tracker still sending", and a visitor who landed thirty hours ago
 * and is still beating answers yes, while their session start does not.
 */
const ACTIVITY_TIME_DIMENSION = 'updated_at'

/** Floor for the lifetime probe when the workspace has no creation date. */
const EPOCH = '2000-01-01T00:00:00.000Z'

/**
 * The demo workspace, whose id `ResetDemo` hardcodes. Its history is generated
 * in one pass per reset and never beats again, so a day later both probes read
 * as a dead install — on a site none of its visitors could install anything on.
 */
const DEMO_WORKSPACE_ID = 'demo'

export type InstallState =
  /** A probe is still in flight; show the page and let its widgets load. */
  | 'loading'
  /** Traffic is arriving, or a probe failed and must not hide the page. */
  | 'ok'
  /** The workspace has no web analytics settings at all. */
  | 'not_configured'
  /** Settings exist but the feature is switched off. */
  | 'disabled'
  /** Enabled, but not a single session has ever been recorded. */
  | 'never_received'
  /** Enabled and used before, but nothing came in over the last 24 hours. */
  | 'stalled'

export interface InstallProbe {
  hasSettings: boolean
  enabled: boolean
  /** False on config screens, which only care whether the feature exists. */
  checkTraffic: boolean
  /** Undefined while the 24-hour probe is in flight. */
  sessionsLast24h?: number
  /** Undefined while the lifetime probe is in flight, or when it was skipped. */
  sessionsEver?: number
  /** A probe that errored resolves to `ok`, never to an install screen. */
  failed?: boolean
}

/**
 * Turns the two probes and the workspace settings into the one thing the
 * overlay renders from. Split out of the hook so every branch is testable
 * without a query client.
 */
export function deriveInstallState(probe: InstallProbe): InstallState {
  if (!probe.hasSettings) return 'not_configured'
  // Ahead of the `enabled` check on purpose: attribution rules are written on
  // a config screen precisely while collection is still off, so switching the
  // feature on must not be a prerequisite for editing them.
  if (!probe.checkTraffic) return 'ok'
  if (!probe.enabled) return 'disabled'
  // An API hiccup is not an install problem: hiding a working dashboard behind
  // a snippet is worse than showing whatever the widgets manage to load.
  if (probe.failed) return 'ok'
  if (probe.sessionsLast24h === undefined) return 'loading'
  if (probe.sessionsLast24h > 0) return 'ok'
  if (probe.sessionsEver === undefined) return 'loading'
  return probe.sessionsEver > 0 ? 'stalled' : 'never_received'
}

/**
 * Answers "is anything reaching this workspace" independently of the report on
 * screen: neither the selected period nor the active filters take part, since
 * an empty week of a working install is not an install problem.
 *
 * The lifetime probe only runs once the recent one comes back empty, so a
 * healthy workspace pays for a single extra aggregate.
 */
export function useInstallStatus(options?: { checkTraffic?: boolean }): InstallState {
  const { workspaceId, workspace, settings, timezone } = useWebAnalytics()

  const checkTraffic = options?.checkTraffic !== false && workspaceId !== DEMO_WORKSPACE_ID
  const hasSettings = settings != null
  const enabled = settings?.enabled === true
  const probing = checkTraffic && hasSettings && enabled

  // Anchored to the top of the hour: a window recomputed on every render would
  // give the query a new key each time and refetch forever.
  const hourBucket = dayjs().startOf('hour').valueOf()

  const recentRange = useMemo<ResolvedRange>(() => {
    const anchor = dayjs(hourBucket)
    // The end runs slightly ahead of now so a row written with a clock a few
    // seconds fast still counts.
    const start = anchor.subtract(24, 'hour')
    const end = anchor.add(1, 'hour')
    return {
      startDay: start.format('YYYY-MM-DD'),
      endDay: end.format('YYYY-MM-DD'),
      startUtc: start.toISOString(),
      endUtc: end.toISOString()
    }
  }, [hourBucket])

  // Bounded by the workspace's own creation rather than an open-ended scan:
  // no session can predate the workspace that recorded it.
  const lifetimeRange = useMemo<ResolvedRange>(() => {
    const start = dayjs(workspace?.created_at || EPOCH)
    const end = dayjs(hourBucket).add(1, 'hour')
    return {
      startDay: start.format('YYYY-MM-DD'),
      endDay: end.format('YYYY-MM-DD'),
      startUtc: start.toISOString(),
      endUtc: end.toISOString()
    }
  }, [workspace?.created_at, hourBucket])

  const recentQuery = useMemo(
    () =>
      probing
        ? buildWebQuery({
            schema: 'web_sessions',
            measures: ['sessions'],
            range: recentRange,
            timeDimension: ACTIVITY_TIME_DIMENSION,
            timezone
          })
        : null,
    [probing, recentRange, timezone]
  )

  const recent = useWebQuery(workspaceId, recentQuery)
  const sessionsLast24h = recent.data ? readTotals(recent.data, ['sessions']).sessions : undefined

  const lifetimeQuery = useMemo(
    () =>
      probing && sessionsLast24h === 0
        ? buildWebQuery({
            schema: 'web_sessions',
            measures: ['sessions'],
            range: lifetimeRange,
            timeDimension: ACTIVITY_TIME_DIMENSION,
            timezone
          })
        : null,
    [probing, sessionsLast24h, lifetimeRange, timezone]
  )

  const lifetime = useWebQuery(workspaceId, lifetimeQuery)
  const sessionsEver =
    lifetimeQuery && lifetime.data ? readTotals(lifetime.data, ['sessions']).sessions : undefined

  return deriveInstallState({
    hasSettings,
    enabled,
    checkTraffic,
    sessionsLast24h,
    sessionsEver,
    failed: recent.isError || lifetime.isError
  })
}
