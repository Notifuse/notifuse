import { useQueries } from '@tanstack/react-query'
import { Link, useNavigate } from '@tanstack/react-router'
import { Skeleton, Table } from 'antd'
import { CircleCheck, Inbox } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import { Panel, Pill, type PillTone } from '../layout'
import { listMessages, type MessageListParams } from '../../services/api/messages_history'
import { LOGS_TAB } from '../logs/logsUrl'
import { BlockError } from './BlockError'
import { DASHBOARD_REFRESH_MS } from './dashboardClient'
import type { SendState } from './dashboardSendState'
import { formatEventTime } from './dashboardFormat'
import { openableRow } from './dashboardTable'
import {
  mergeFailures,
  RECENT_FAILURES_LIMIT,
  type FailureKind,
  type FailureLists,
  type FailureRow
} from './recentFailuresModel'

interface RecentFailuresProps {
  workspaceId: string
  timezone: string
  /**
   * Before the first send there is nothing to list, and a green « No failure » would read as a
   * result: the card says what will show instead, and asks nothing. `counted` by default.
   */
  sendState?: SendState
}

const LISTS: { list: keyof FailureLists; params: MessageListParams }[] = [
  { list: 'complained', params: { is_complained: true } },
  { list: 'bounced', params: { is_bounced: true } },
  { list: 'failed', params: { is_failed: true } }
]

const TONE: Record<FailureKind, PillTone> = {
  complaint: 'danger',
  bounce: 'warning',
  failed: 'danger'
}

/** The five latest complaints, bounces and failures, whatever sent them. */
export function RecentFailures({ workspaceId, timezone, sendState = 'counted' }: RecentFailuresProps) {
  const { t, i18n } = useLingui()
  const navigate = useNavigate()

  const results = useQueries({
    queries: LISTS.map(({ list, params }) => ({
      queryKey: ['dashboard', 'failures', workspaceId, list],
      queryFn: () => listMessages(workspaceId, { ...params, limit: RECENT_FAILURES_LIMIT }),
      refetchInterval: DASHBOARD_REFRESH_MS,
      enabled: sendState === 'counted' || sendState === 'today'
    }))
  })

  const label: Record<FailureKind, string> = {
    complaint: t`Complaint`,
    bounce: t`Bounce`,
    failed: t`Failed`
  }
  const fallbackReason: Record<FailureKind, string> = {
    complaint: t`Marked as spam`,
    bounce: t`Bounced`,
    failed: t`Failed`
  }

  // A failed refresh keeps the rows already shown.
  const failed = results.find((result) => result.isError && !result.data)
  const loading = results.some((result) => result.isLoading)
  const lists: FailureLists = {}
  LISTS.forEach(({ list }, index) => {
    lists[list] = results[index].data?.messages
  })
  const rows = mergeFailures(lists)

  const openMessage = (row: FailureRow) =>
    void navigate({
      to: '/console/workspace/$workspaceId/logs',
      params: { workspaceId },
      search: {
        tab: LOGS_TAB.messages,
        id: row.message.id,
        message: row.message.id
      }
    })

  const extra = (
    <Link
      to="/console/workspace/$workspaceId/logs"
      params={{ workspaceId }}
      // A boolean, which the router writes as is_failed=true; the string 'true' would be written
      // JSON-quoted and read back as not true. All time: the card lists the latest failures
      // whatever their date, past the tab's default 24 hours.
      search={{ tab: LOGS_TAB.messages, is_failed: true, period: 'all' }}
    >
      {t`View in logs`}
    </Link>
  )

  let body
  // Before isLoading: a query that is not enabled is not loading.
  if (sendState === 'never') {
    body = (
      <div className="flex items-start gap-2.5 px-3.5 py-4 text-sm text-ink-2">
        <span aria-hidden="true" className="flex size-7 shrink-0 items-center justify-center rounded-lg bg-fill text-ink-3">
          <Inbox size={14} />
        </span>
        <span>{t`Nothing has been sent yet. Bounces, complaints and failures will show here.`}</span>
      </div>
    )
  } else if (sendState === 'loading') {
    body = (
      <div className="px-3.5 py-3">
        <Skeleton active paragraph={{ rows: 3 }} title={false} />
      </div>
    )
  } else if (failed) {
    body = (
      <BlockError
        error={failed.error}
        onRetry={() => results.forEach((result) => result.isError && void result.refetch())}
      />
    )
  } else if (loading) {
    body = (
      <div className="px-3.5 py-3">
        <Skeleton active paragraph={{ rows: 3 }} title={false} />
      </div>
    )
  } else if (rows.length === 0) {
    body = (
      <div className="flex items-center gap-2 px-3.5 py-6 text-sm text-ink-3">
        <CircleCheck size={16} className="text-success-fg" aria-hidden="true" />
        {t`No failure`}
      </div>
    )
  } else {
    body = (
      <Table<FailureRow>
        size="small"
        showHeader={false}
        pagination={false}
        tableLayout="fixed"
        rowKey={(row) => row.message.id}
        dataSource={rows}
        onRow={(row) => openableRow(() => openMessage(row))}
        columns={[
          {
            key: 'at',
            width: 64,
            render: (_, row) => (
              <span className="text-xs text-ink-3 tabular-nums">{formatEventTime(row.at, timezone, i18n.locale)}</span>
            )
          },
          {
            key: 'who',
            render: (_, row) => (
              <div className="min-w-0">
                <div className="truncate text-ink">{row.message.contact_email}</div>
                <div className="truncate text-xs text-ink-3" title={row.reason ?? undefined}>
                  {row.reason ?? fallbackReason[row.kind]}
                </div>
              </div>
            )
          },
          {
            key: 'kind',
            align: 'right',
            width: 110,
            render: (_, row) => <Pill tone={TONE[row.kind]}>{label[row.kind]}</Pill>
          }
        ]}
      />
    )
  }

  return (
    <Panel title={t`Recent failures`} extra={extra} flush>
      {body}
    </Panel>
  )
}
