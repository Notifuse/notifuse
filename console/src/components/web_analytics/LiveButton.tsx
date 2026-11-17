import { Link } from '@tanstack/react-router'
import { useLingui } from '@lingui/react/macro'
import { useWebAnalytics } from './useWebAnalytics'
import { useMinuteTick } from './lib/useMinuteTick'
import { readTotals, useWebQuery } from './lib/query'
import { useNumberFormat } from '../layout'
import { AnalyticsQuery } from '../../services/api/analytics'

const REFRESH_MS = 15_000

export function LiveButton() {
  const { t } = useLingui()
  const { int } = useNumberFormat()
  const { workspaceId } = useWebAnalytics()

  // Sessions still active, i.e. last seen in the past 30 minutes. The window
  // only moves once a minute: derived from the clock on every render it would
  // produce a new query key each time, and the query would refetch forever.
  const anchor = useMinuteTick()
  const query: AnalyticsQuery = {
    schema: 'web_sessions',
    measures: ['sessions'],
    dimensions: [],
    filters: [
      {
        member: 'updated_at',
        operator: 'inDateRange',
        values: [
          anchor.subtract(30, 'minute').toISOString(),
          anchor.add(5, 'minute').toISOString()
        ]
      }
    ]
  }
  const { data } = useWebQuery(workspaceId, query, { refetchInterval: REFRESH_MS })
  const sessions = readTotals(data, ['sessions']).sessions

  return (
    <Link
      to="/console/workspace/$workspaceId/web-analytics/live"
      params={{ workspaceId }}
      className="inline-flex items-center gap-2 rounded-full border border-line bg-surface px-3 py-1 text-xs text-ink-2 hover:border-primary-500"
    >
      <span className="relative flex h-2 w-2">
        <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-success-fg opacity-50" />
        <span className="relative inline-flex h-2 w-2 rounded-full bg-success-fg" />
      </span>
      <span>{t`Live`}</span>
      <span className="font-semibold text-ink">{int(sessions)}</span>
    </Link>
  )
}
