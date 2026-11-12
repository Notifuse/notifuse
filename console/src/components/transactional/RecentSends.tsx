import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { Empty, Skeleton, Table, Tooltip } from 'antd'
import { ChevronRight } from 'lucide-react'
import { useLingui } from '@lingui/react/macro'
import { Mono, Panel, Pill } from '../layout'
import { listMessages, type MessageHistory } from '../../services/api/messages_history'
import { messageStatus, type MessageStatusKey } from '../../lib/messageStatus'
import { LOGS_TAB } from '../logs/logsUrl'
import { LoadError } from './LoadError'
import { formatFullDateTime, formatSendTime } from './transactionalTime'

interface RecentSendsProps {
  workspaceId: string
  notificationId: string
  timezone: string
  canReadMessages: boolean
  now: Date
}

export const RECENT_SENDS_LIMIT = 10
export const RECENT_SENDS_REFRESH_MS = 30_000

/** The ten latest messages of the notification, newest first. */
export function RecentSends({ workspaceId, notificationId, timezone, canReadMessages, now }: RecentSendsProps) {
  const { t, i18n } = useLingui()

  const recent = useQuery({
    queryKey: ['transactional-recent', workspaceId, notificationId],
    queryFn: () =>
      listMessages(workspaceId, { transactional_notification_id: notificationId, limit: RECENT_SENDS_LIMIT }),
    enabled: canReadMessages,
    // Only while the tab is visible: TanStack Query pauses intervals in the background by default.
    refetchInterval: RECENT_SENDS_REFRESH_MS
  })

  const statusLabel: Record<MessageStatusKey, string> = {
    failed: t`Failed`,
    complained: t`Complaint`,
    bounced: t`Bounced`,
    unsubscribed: t`Unsubscribed`,
    clicked: t`Clicked`,
    opened: t`Opened`,
    delivered: t`Delivered`,
    sent: t`Sent`
  }

  const extra = canReadMessages ? (
    <Link
      to="/console/workspace/$workspaceId/logs"
      params={{ workspaceId }}
      search={{ tab: LOGS_TAB.messages, transactional_notification_id: notificationId }}
    >
      {t`View in logs`}
    </Link>
  ) : undefined

  if (!canReadMessages) {
    return (
      <Panel title={t`Recent sends`}>
        <p data-unreset className="m-0 text-sm text-ink-3">{t`You need access to message history to see sends`}</p>
      </Panel>
    )
  }

  const rows = recent.data?.messages ?? []
  let body
  // A failed refresh keeps the rows already loaded.
  if (recent.isError && !recent.data) body = <LoadError error={recent.error} onRetry={() => void recent.refetch()} />
  else if (recent.isLoading)
    body = (
      <div className="px-3.5 py-3">
        <Skeleton active title={false} paragraph={{ rows: 4 }} />
      </div>
    )
  else if (rows.length === 0)
    body = <Empty className="py-4" image={Empty.PRESENTED_IMAGE_SIMPLE} description={t`No sends yet`} />
  else
    body = (
      <Table<MessageHistory>
        size="small"
        pagination={false}
        tableLayout="fixed"
        rowKey="id"
        dataSource={rows}
        columns={[
          {
            key: 'time',
            title: t`Time`,
            width: 130,
            render: (_, message) => (
              <Tooltip title={formatFullDateTime(message.sent_at || message.created_at, timezone, i18n.locale)}>
                <span className="text-ink-2 tabular-nums">
                  {formatSendTime(message.sent_at || message.created_at, now, timezone, i18n.locale)}
                </span>
              </Tooltip>
            )
          },
          {
            key: 'recipient',
            title: t`Recipient`,
            ellipsis: true,
            render: (_, message) => <span className="text-ink">{message.contact_email}</span>
          },
          {
            key: 'status',
            title: t`Status`,
            width: 120,
            render: (_, message) => {
              const status = messageStatus(message)
              const pill = <Pill tone={status.tone}>{statusLabel[status.key]}</Pill>
              const explained = (status.key === 'failed' || status.key === 'bounced') && message.status_info
              return explained ? <Tooltip title={message.status_info}>{pill}</Tooltip> : pill
            }
          },
          {
            key: 'external',
            title: t`External ID`,
            width: 190,
            ellipsis: true,
            render: (_, message) =>
              message.external_id ? <Mono copy>{message.external_id}</Mono> : <span className="text-ink-4">—</span>
          },
          {
            key: 'open',
            width: 40,
            align: 'right',
            render: (_, message) => (
              <Link
                to="/console/workspace/$workspaceId/logs"
                params={{ workspaceId }}
                search={{ tab: LOGS_TAB.messages, id: message.id, message: message.id }}
                aria-label={t`Open in logs`}
                className="inline-flex text-ink-3"
              >
                <ChevronRight size={15} aria-hidden="true" />
              </Link>
            )
          }
        ]}
      />
    )

  return (
    <Panel title={t`Recent sends`} extra={extra} flush>
      {body}
    </Panel>
  )
}
