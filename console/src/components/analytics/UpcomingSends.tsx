import { useQuery } from '@tanstack/react-query'
import { Link, useNavigate } from '@tanstack/react-router'
import { Button, Skeleton } from 'antd'
import { CalendarClock } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import { plural } from '@lingui/core/macro'
import { Panel } from '../layout'
import { broadcastApi } from '../../services/api/broadcast'
import { listsApi } from '../../services/api/list'
import { BlockError } from './BlockError'
import { DASHBOARD_REFRESH_MS } from './dashboardClient'
import { formatDayAndTime } from './dashboardFormat'
import { openableRow } from './dashboardTable'
import { sortUpcoming, timeZoneAbbreviation, type UpcomingSend } from './upcomingSendsModel'

interface UpcomingSendsProps {
  workspaceId: string
  timezone: string
  /** Names the target list; without it, « 1 list ». */
  canReadLists: boolean
  /** Shows the « New broadcast » button of the empty card. */
  canCreate: boolean
  /** Marketing emails have a provider that can send; without one the empty card says so. */
  marketingReady?: boolean
  /** Shows the « Configure » link to the integrations settings. */
  canConfigure?: boolean
}

const UPCOMING_LIMIT = 5
/** broadcasts.list returns at most this many; it sorts by creation, so the page sorts them itself. */
const SCHEDULED_FETCH_LIMIT = 100

/** The next five scheduled broadcasts, soonest first. */
export function UpcomingSends({
  workspaceId,
  timezone,
  canReadLists,
  canCreate,
  marketingReady = true,
  canConfigure = false
}: UpcomingSendsProps) {
  const { t, i18n } = useLingui()
  const navigate = useNavigate()

  const scheduled = useQuery({
    queryKey: ['dashboard', 'upcoming', workspaceId],
    queryFn: () =>
      broadcastApi.list({
        workspace_id: workspaceId,
        statuses: ['scheduled'],
        limit: SCHEDULED_FETCH_LIMIT
      }),
    refetchInterval: DASHBOARD_REFRESH_MS
  })
  const hasRows = (scheduled.data?.broadcasts.length ?? 0) > 0
  const lists = useQuery({
    queryKey: ['lists', workspaceId],
    queryFn: () => listsApi.list({ workspace_id: workspaceId }),
    enabled: canReadLists && hasRows
  })

  // Rows go to the scheduled broadcasts until the Broadcasts page opens one from its URL.
  const scheduledBroadcasts = {
    to: '/console/workspace/$workspaceId/broadcasts',
    params: { workspaceId },
    search: { status: 'scheduled' }
  } as const

  const upcoming = sortUpcoming(scheduled.data?.broadcasts ?? [], timezone)
  const shown = upcoming.slice(0, UPCOMING_LIMIT)
  // A scheduled broadcast without a date (never seen in practice) is neither shown nor counted.
  const undated = (scheduled.data?.broadcasts.length ?? 0) - upcoming.length
  const more = Math.max(0, (scheduled.data?.total_count ?? 0) - undated - shown.length)

  const listName = (send: UpcomingSend) => {
    const id = send.broadcast.audience?.list
    const name = id ? lists.data?.lists.find((list) => list.id === id)?.name : undefined
    return name ?? t`1 list`
  }
  const when = (send: UpcomingSend) => {
    const time = formatDayAndTime(send.at, send.timezone, i18n.locale)
    if (send.recipientTime) return t`${time} local time`
    if (send.timezone === (timezone || 'UTC')) return time
    return `${time} ${timeZoneAbbreviation(send.timezone, send.at, i18n.locale)}`
  }

  const extra = (
    <Link to="/console/workspace/$workspaceId/broadcasts" params={{ workspaceId }}>
      {t`Broadcasts`}
    </Link>
  )
  const footer =
    more > 0 ? (
      <Link {...scheduledBroadcasts}>{t`${plural(more, { one: '+# more', other: '+# more' })}`}</Link>
    ) : undefined

  let body
  if (scheduled.isError && !scheduled.data) {
    body = <BlockError error={scheduled.error} onRetry={() => void scheduled.refetch()} />
  } else if (scheduled.isLoading) {
    body = (
      <div className="px-3.5 py-3">
        <Skeleton active paragraph={{ rows: 2 }} title={false} />
      </div>
    )
  } else if (shown.length === 0) {
    body = (
      <div className="flex items-start gap-2.5 px-3.5 py-4 text-sm text-ink-2">
        <span aria-hidden="true" className="flex size-7 shrink-0 items-center justify-center rounded-lg bg-fill text-ink-3">
          <CalendarClock size={14} />
        </span>
        <div className="flex min-w-0 flex-col gap-2">
          <span>{t`Scheduled broadcasts will show here.`}</span>
          {canCreate || (!marketingReady && canConfigure) ? (
            <div className="flex flex-wrap items-center gap-2">
              {canCreate ? (
                // The Broadcasts page opens its composer on compose=new.
                <Link to="/console/workspace/$workspaceId/broadcasts" params={{ workspaceId }} search={{ compose: 'new' }}>
                  <Button size="small">{t`New broadcast`}</Button>
                </Link>
              ) : null}
              {!marketingReady ? (
                <span className="text-xs text-ink-3">
                  {t`Sending it needs an email provider`}
                  {canConfigure ? (
                    <>
                      {' · '}
                      <Link
                        to="/console/workspace/$workspaceId/settings/$section"
                        params={{ workspaceId, section: 'integrations' }}
                      >
                        {t`Configure`}
                      </Link>
                    </>
                  ) : null}
                </span>
              ) : null}
            </div>
          ) : null}
        </div>
      </div>
    )
  } else {
    body = (
      <div className="flex flex-col">
        {shown.map((send) => (
          <div
            key={send.broadcast.id}
            {...openableRow(() => void navigate(scheduledBroadcasts), true)}
            className="flex cursor-pointer gap-3 border-t border-line px-3.5 py-2.5 text-sm first:border-t-0 hover:bg-fill"
          >
            <span className="w-[120px] shrink-0 text-xs text-ink-3 tabular-nums">{when(send)}</span>
            <div className="min-w-0">
              <div className="truncate font-medium text-ink">{send.broadcast.name}</div>
              <div className="truncate text-xs text-ink-3">
                {listName(send)}
                {send.broadcast.test_settings?.enabled ? ` · ${t`A/B`}` : ''}
              </div>
            </div>
          </div>
        ))}
      </div>
    )
  }

  return (
    <Panel title={t`Upcoming sends`} extra={extra} footer={footer} flush>
      {body}
    </Panel>
  )
}
