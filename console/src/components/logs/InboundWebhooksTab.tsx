import { useMemo, useState } from 'react'
import { useInfiniteQuery, useQueryClient } from '@tanstack/react-query'
import { Button, Empty, Result, Skeleton, Table, Tooltip, type MenuProps } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { useLingui } from '@lingui/react/macro'
import { plural } from '@lingui/core/macro'
import { Link, useNavigate } from '@tanstack/react-router'
import { ChevronRight, RefreshCw } from 'lucide-react'
import { useAuth } from '../../contexts/AuthContext'
import type { UserPermissions } from '../../services/api/types'
import {
  listInboundWebhookEvents,
  type InboundWebhookEvent,
  type InboundWebhookEventListParams
} from '../../services/api/inbound_webhook_event'
import {
  KeyValueList,
  Mono,
  Panel,
  Pill,
  SidePanel,
  TableToolbar,
  useNumberFormat
} from '../layout'
import type { PillTone } from '../layout'
import { CodeBlock } from './CodeBlock'
import { FilterChip } from './FilterChip'
import { LogsSearchBox } from './LogsSearchBox'
import { INBOUND_EVENT_LABELS, WEBHOOK_SOURCE_LABELS } from './labels'
import {
  INBOUND_EVENT_TYPES,
  WEBHOOK_SOURCES,
  type InboundEventType,
  type LivePeriod,
  type LogsWebhookSource
} from './logsSearch'
import { formatLogDateTime, formatLogTime } from './logTime'
import { ERASED_EMAIL } from './messageProvider'
import { defaultInboundPeriod, periodBounds } from './period'
import { useLogsUrl } from './useLogsUrl'

const PAGE_SIZE = 50

const EVENT_TONES: Record<InboundEventType, PillTone> = {
  delivered: 'success',
  bounce: 'warning',
  complaint: 'danger',
  unsubscribe: 'neutral',
  reply: 'info',
  auto_reply: 'info',
  auth_email: 'neutral',
  before_user_created: 'neutral'
}

// Only these name a message of the workspace; a reply's message_id is the reply's own Message-ID.
const DELIVERY_EVENTS: readonly InboundEventType[] = [
  'delivered',
  'bounce',
  'complaint',
  'unsubscribe'
]

const SEARCH_KEYS = ['recipient_email', 'message_id'] as const

export interface InboundWebhooksTabProps {
  workspaceId: string
  /** The member's rights, as the Logs page loaded them. */
  permissions: UserPermissions | null
}

/** Logs → Incoming webhooks: what providers and Supabase sent to Notifuse. */
export function InboundWebhooksTab({ workspaceId, permissions }: InboundWebhooksTabProps) {
  const { t, i18n } = useLingui()
  const { int } = useNumberFormat()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { workspaces } = useAuth()
  const timezone = workspaces.find((workspace) => workspace.id === workspaceId)?.settings.timezone
  const canReadMessages = Boolean(permissions?.message_history?.read)
  const { search, update, clear } = useLogsUrl(workspaceId, 'incoming-webhooks')
  const period: LivePeriod =
    (search.period as LivePeriod | undefined) ?? defaultInboundPeriod(search)
  const [openId, setOpenId] = useState<string>()

  // The clock of the period: set when the filters change and on Refresh, never by a re-render.
  const filtersKey = JSON.stringify(search)
  const [clock, setClock] = useState(() => ({ key: filtersKey, now: Date.now() }))
  if (clock.key !== filtersKey) setClock({ key: filtersKey, now: Date.now() })
  const now = clock.key === filtersKey ? clock.now : Date.now()

  const params = useMemo(() => {
    const out: InboundWebhookEventListParams = {}
    if (search.recipient_email) out.recipient_email = search.recipient_email
    if (search.message_id) out.message_id = search.message_id
    if (search.source) out.source = search.source
    if (search.event_type) out.event_type = search.event_type
    const after = periodBounds(period, now).from
    if (after) out.timestamp_after = after
    return out
    // `search` is new on every render; its content is the key.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [filtersKey, period, now])

  // W1: 50 events at a time, by cursor.
  const listQuery = useInfiniteQuery({
    queryKey: ['logs', 'inbound', workspaceId, params],
    queryFn: ({ pageParam }) =>
      listInboundWebhookEvents(workspaceId, { ...params, cursor: pageParam, limit: PAGE_SIZE }),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => (last.has_more ? last.next_cursor : undefined),
    staleTime: 30_000
  })
  const rows = useMemo(
    () => listQuery.data?.pages.flatMap((page) => page.events) ?? [],
    [listQuery.data]
  )

  // Refresh moves the clock to now and asks the list once: again when the window stays the same
  // (all time, or the same minute); otherwise the moved window is a new key, fetched on its own.
  const refresh = () => {
    const next = Date.now()
    const sameWindow = periodBounds(period, next).from === periodBounds(period, now).from
    setClock({ key: filtersKey, now: next })
    if (sameWindow) void queryClient.resetQueries({ queryKey: ['logs', 'inbound', workspaceId] })
  }

  const periodLabels: Record<LivePeriod, string> = {
    '1h': t`Last hour`,
    '24h': t`Last 24 hours`,
    '7d': t`Last 7 days`,
    '30d': t`Last 30 days`,
    all: t`All time`
  }
  const sourceLabel = (source: string) =>
    source in WEBHOOK_SOURCE_LABELS
      ? i18n._(WEBHOOK_SOURCE_LABELS[source as LogsWebhookSource])
      : source
  const eventLabel = (type: string) =>
    type in INBOUND_EVENT_LABELS ? i18n._(INBOUND_EVENT_LABELS[type as InboundEventType]) : type

  const periodMenu: MenuProps = {
    items: (['1h', '24h', '7d', '30d', 'all'] as const).map((key) => ({
      key,
      label: periodLabels[key]
    })),
    selectable: true,
    selectedKeys: [period],
    onClick: ({ key }) => update({ period: key as LivePeriod })
  }
  const sourceMenu: MenuProps = {
    items: [
      { key: '', label: t`All` },
      ...WEBHOOK_SOURCES.map((source) => ({ key: source, label: sourceLabel(source) }))
    ],
    selectable: true,
    selectedKeys: [search.source ?? ''],
    onClick: ({ key }) => update({ source: (key || undefined) as LogsWebhookSource | undefined })
  }
  const typeMenu: MenuProps = {
    items: [
      { key: '', label: t`All` },
      ...INBOUND_EVENT_TYPES.map((type) => ({ key: type, label: eventLabel(type) }))
    ],
    selectable: true,
    selectedKeys: [search.event_type ?? ''],
    onClick: ({ key }) => update({ event_type: (key || undefined) as InboundEventType | undefined })
  }

  const filtered =
    SEARCH_KEYS.some((key) => search[key] !== undefined) ||
    search.source !== undefined ||
    search.event_type !== undefined
  const clearFilters = () => clear(['period'])

  const toolbar = (
    <TableToolbar
      start={
        <>
          <LogsSearchBox
            keys={[
              { key: 'recipient_email', label: t`Email`, email: true },
              { key: 'message_id', label: t`Message ID` }
            ]}
            values={search}
            onWrite={(patch) => update(patch, { replace: true })}
            ariaLabel={t`Search the webhook events`}
            placeholder={t`Exact value`}
          />
          <FilterChip label={t`Period`} value={periodLabels[period]} menu={periodMenu} />
          <FilterChip
            label={t`Source`}
            value={search.source ? sourceLabel(search.source) : t`All`}
            menu={sourceMenu}
          />
          <FilterChip
            label={t`Type`}
            value={search.event_type ? eventLabel(search.event_type) : t`All`}
            menu={typeMenu}
          />
        </>
      }
      end={
        <Tooltip title={t`Refresh`}>
          <Button
            type="text"
            size="small"
            aria-label={t`Refresh`}
            icon={<RefreshCw size={14} />}
            onClick={refresh}
          />
        </Tooltip>
      }
    />
  )

  const eventPill = (event: InboundWebhookEvent) => {
    const label =
      event.type === 'bounce' && event.bounce_type
        ? `${eventLabel(event.type)} · ${event.bounce_type}`
        : eventLabel(event.type)
    return <Pill tone={EVENT_TONES[event.type] ?? 'neutral'}>{label}</Pill>
  }

  const linkedMessage = (event: InboundWebhookEvent) => {
    if (!event.message_id) return <span className="text-ink-4">—</span>
    if (!DELIVERY_EVENTS.includes(event.type) || !canReadMessages)
      return <Mono>{event.message_id}</Mono>
    return (
      <Link
        to="/console/workspace/$workspaceId/logs"
        params={{ workspaceId }}
        search={{ tab: 'messages', id: event.message_id, message: event.message_id }}
        onClick={(clickEvent) => clickEvent.stopPropagation()}
      >
        <Mono>{event.message_id}</Mono>
      </Link>
    )
  }

  const recipient = (event: InboundWebhookEvent) =>
    event.recipient_email === ERASED_EMAIL ? (
      <span className="text-ink-3">{t`Contact erased`}</span>
    ) : (
      <span className="truncate">{event.recipient_email}</span>
    )

  const columns: ColumnsType<InboundWebhookEvent> = [
    {
      title: t`Time`,
      key: 'time',
      width: 140,
      render: (_, event) => (
        <Tooltip title={formatLogDateTime(event.timestamp, timezone)}>
          <span className="text-ink-3 tabular-nums">
            {formatLogTime(i18n, event.timestamp, timezone)}
          </span>
        </Tooltip>
      )
    },
    { title: t`Source`, key: 'source', render: (_, event) => sourceLabel(event.source) },
    { title: t`Event`, key: 'event', render: (_, event) => eventPill(event) },
    { title: t`Recipient`, key: 'recipient', render: (_, event) => recipient(event) },
    {
      title: t`Linked message`,
      key: 'message',
      render: (_, event) => <span className="block max-w-[200px]">{linkedMessage(event)}</span>
    },
    {
      key: 'open',
      width: 32,
      render: () => <ChevronRight size={14} className="text-ink-4" aria-hidden="true" />
    }
  ]

  let body
  if (listQuery.isPending) {
    body = (
      <div className="p-3.5">
        <Skeleton active title={false} paragraph={{ rows: 8 }} />
      </div>
    )
  } else if (listQuery.isError) {
    body = (
      <Result
        status="error"
        title={t`Couldn't load the webhook events`}
        subTitle={listQuery.error instanceof Error ? listQuery.error.message : undefined}
        extra={<Button onClick={() => void listQuery.refetch()}>{t`Retry`}</Button>}
      />
    )
  } else if (rows.length === 0) {
    const showAll = <Button onClick={() => update({ period: 'all' })}>{t`Show all time`}</Button>
    body = (
      <div className="py-10">
        {filtered ? (
          <Empty description={t`No results match`}>
            <Button onClick={clearFilters}>{t`Clear filters`}</Button>
          </Empty>
        ) : period === 'all' ? (
          <Empty description={t`No webhook events received yet`}>
            <Button
              onClick={() =>
                void navigate({
                  to: '/console/workspace/$workspaceId/settings/$section',
                  params: { workspaceId, section: 'integrations' }
                })
              }
            >
              {t`Set up an email provider`}
            </Button>
          </Empty>
        ) : search.period === undefined ? (
          <Empty description={t`Nothing in the last 24 hours`}>{showAll}</Empty>
        ) : (
          <Empty description={t`Nothing in this period`}>{showAll}</Empty>
        )}
      </div>
    )
  } else {
    body = (
      <Table<InboundWebhookEvent>
        size="small"
        columns={columns}
        dataSource={rows}
        rowKey="id"
        pagination={false}
        rowClassName={(event) =>
          event.id === openId ? 'cursor-pointer is-hover' : 'cursor-pointer'
        }
        onRow={(event) => ({
          onClick: () => setOpenId(event.id),
          onKeyDown: (keyEvent) => {
            if (keyEvent.key === 'Enter') setOpenId(event.id)
          },
          tabIndex: 0
        })}
      />
    )
  }

  const shown = int(rows.length)
  const footer =
    rows.length > 0 ? (
      <div className="flex w-full items-center justify-between">
        <span className="text-sm text-ink-3">
          {plural(rows.length, { one: `${shown} event`, other: `${shown} events` })}
        </span>
        {listQuery.hasNextPage ? (
          <Button
            size="small"
            loading={listQuery.isFetchingNextPage}
            onClick={() => void listQuery.fetchNextPage()}
          >
            {t`Load more`}
          </Button>
        ) : null}
      </div>
    ) : undefined

  const openIndex = rows.findIndex((event) => event.id === openId)
  const open = openIndex >= 0 ? rows[openIndex] : undefined

  return (
    <>
      <Panel flush footer={footer}>
        {toolbar}
        {body}
      </Panel>
      {open ? (
        <SidePanel
          open
          onClose={() => setOpenId(undefined)}
          title={eventPill(open)}
          meta={[
            open.recipient_email === ERASED_EMAIL ? t`Contact erased` : open.recipient_email,
            formatLogDateTime(open.timestamp, timezone)
          ]}
          nav={{
            position: openIndex + 1,
            total: rows.length,
            onPrev: openIndex > 0 ? () => setOpenId(rows[openIndex - 1].id) : undefined,
            onNext:
              openIndex < rows.length - 1 ? () => setOpenId(rows[openIndex + 1].id) : undefined
          }}
          // The detail has no URL of its own: a copied link would not reopen it.
          copyLink={false}
        >
          <div key={open.id} className="flex flex-col gap-4">
            <KeyValueList
              items={[
                { label: t`Event ID`, value: <Mono copy>{open.id}</Mono> },
                { label: t`Source`, value: sourceLabel(open.source) },
                { label: t`Integration ID`, value: <Mono>{open.integration_id}</Mono> },
                { label: t`Message ID`, value: linkedMessage(open) },
                { label: t`Received`, value: formatLogDateTime(open.created_at, timezone) },
                ...(open.bounce_type ? [{ label: t`Bounce type`, value: open.bounce_type }] : []),
                ...(open.bounce_category
                  ? [{ label: t`Bounce category`, value: open.bounce_category }]
                  : []),
                ...(open.bounce_diagnostic
                  ? [{ label: t`Bounce diagnostic`, value: open.bounce_diagnostic }]
                  : []),
                ...(open.complaint_feedback_type
                  ? [{ label: t`Feedback type`, value: open.complaint_feedback_type }]
                  : [])
              ]}
            />
            <section className="flex flex-col gap-2">
              <h4 data-unreset className="m-0 text-sm font-semibold text-ink">
                {t`Raw payload`}
              </h4>
              <CodeBlock value={open.raw_payload} label={t`payload`} />
            </section>
          </div>
        </SidePanel>
      ) : (
        <SidePanel open={false} onClose={() => setOpenId(undefined)} title="" copyLink={false} />
      )}
    </>
  )
}
