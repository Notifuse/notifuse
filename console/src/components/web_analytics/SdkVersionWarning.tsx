import { ReactNode, useMemo } from 'react'
import { Alert, Button } from 'antd'
import { Link } from '@tanstack/react-router'
import { useLingui } from '@lingui/react/macro'
import dayjs from '../../lib/dayjs'
import { useWebAnalytics } from './useWebAnalytics'
import { buildWebQuery, useWebQuery } from './lib/query'
import { ResolvedRange } from './lib/types'

/** Newest first, comparing numeric segments so 1.10.0 outranks 1.9.0. */
function compareVersions(a: string, b: string): number {
  const left = a.split('.').map((part) => parseInt(part, 10) || 0)
  const right = b.split('.').map((part) => parseInt(part, 10) || 0)
  for (let i = 0; i < Math.max(left.length, right.length); i++) {
    const difference = (right[i] ?? 0) - (left[i] ?? 0)
    if (difference !== 0) return difference
  }
  return 0
}

/**
 * Flags a site serving more than one tracker build.
 *
 * A single version is normal, whatever it is. Several at once over a day means
 * some visitors are still being handed a cached copy of an older snippet, and
 * that is worth acting on because the two builds may not record the same
 * fields.
 */
export function SdkVersionWarning(): ReactNode {
  const { t } = useLingui()
  const { workspaceId, timezone } = useWebAnalytics()

  // Recomputing the window on every render would give the query a new key each
  // time and refetch forever, so it is anchored to the top of the current hour
  // and only moves once an hour. Both ends round out to whole hours.
  const hourBucket = dayjs().startOf('hour').valueOf()
  const range = useMemo<ResolvedRange>(() => {
    const anchor = dayjs(hourBucket).tz(timezone)
    const start = anchor.subtract(1, 'day')
    const end = anchor.add(1, 'hour')
    return {
      startDay: start.format('YYYY-MM-DD'),
      endDay: end.format('YYYY-MM-DD'),
      startUtc: start.toISOString(),
      endUtc: end.toISOString()
    }
  }, [timezone, hourBucket])

  const query = useMemo(
    () =>
      buildWebQuery({
        schema: 'web_sessions',
        measures: ['sessions'],
        dimensions: ['sdk_version'],
        range,
        timezone,
        limit: 10
      }),
    [range, timezone]
  )

  const { data, isLoading } = useWebQuery(workspaceId, query)

  const versions = useMemo(() => {
    const unique = new Set<string>()
    for (const row of data?.data ?? []) {
      const version = String(row.sdk_version ?? '').trim()
      if (version) unique.add(version)
    }
    return [...unique].sort(compareVersions)
  }, [data])

  if (isLoading || versions.length < 2) return null

  const [newest, ...older] = versions

  return (
    <Alert
      type="warning"
      showIcon
      className="mb-4"
      title={
        <div className="flex items-center justify-between gap-4">
          <span>
            {t`Visitors are being tracked by more than one SDK version (${older.join(
              ', '
            )} alongside ${newest}). A cached copy of the tracking snippet is probably still being served.`}
          </span>
          <Link
            to="/console/workspace/$workspaceId/settings/$section"
            params={{ workspaceId, section: 'web-analytics' }}
          >
            <Button type="link" size="small" className="p-0">
              {t`View snippet`}
            </Button>
          </Link>
        </div>
      }
    />
  )
}
