import { useEffect, useId, useState, type KeyboardEvent } from 'react'
import { Button, Input, Segmented, Table, Tooltip, type TableProps } from 'antd'
import { useInfiniteQuery, useQueryClient, type InfiniteData, type QueryKey } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useLingui } from '@lingui/react/macro'
import { plural } from '@lingui/core/macro'
import { Eye } from 'lucide-react'
import { Mono, Panel, useNumberFormat } from '../../layout'
import { isFullEmail } from '../../../lib/email'
import {
  listMessages,
  type MessageFilterParams,
  type MessageHistory,
  type MessageHistoryStatusSum,
  type MessageListResult,
  type MessageOrderBy
} from '../../../services/api/messages_history'
import type { Broadcast } from '../../../services/api/broadcast'
import type { List } from '../../../services/api/list'
import type { UserPermissions } from '../../../services/api/workspace'
import { MessageJourneySteps } from '../../contacts/sheet/MessageJourneySteps'
import { MessageDetail } from '../../logs/MessageDetail'
import { ERASED_EMAIL } from '../../logs/messageProvider'
import { useBroadcastDates } from '../useBroadcastDates'
import type { RecipientStep } from './BroadcastFunnel'

/** Rows of the first page, so that the list stays short under the funnel. */
export const RECIPIENTS_SHOWN = 5
/** Rows each "Show more" adds. */
export const RECIPIENTS_MORE = 20

interface StepQuery {
  orderBy: MessageOrderBy
  /** The status filter of the step, as messages.list and the logs URL name it. */
  flag: keyof MessageFilterParams
  /** Sent lists what the Sent cell counts: the messages sent without failing. */
  value: boolean
  total: (stats: MessageHistoryStatusSum) => number
}

const STEP_QUERY: Record<RecipientStep, StepQuery> = {
  sent: { orderBy: 'sent_at', flag: 'is_failed', value: false, total: (s) => s.total_sent },
  delivered: { orderBy: 'delivered_at', flag: 'is_delivered', value: true, total: (s) => s.total_delivered },
  opened: { orderBy: 'opened_at', flag: 'is_opened', value: true, total: (s) => s.total_opened },
  clicked: { orderBy: 'clicked_at', flag: 'is_clicked', value: true, total: (s) => s.total_clicked },
  bounced: { orderBy: 'bounced_at', flag: 'is_bounced', value: true, total: (s) => s.total_bounced },
  complained: { orderBy: 'complained_at', flag: 'is_complained', value: true, total: (s) => s.total_complained },
  unsubscribed: {
    orderBy: 'unsubscribed_at',
    flag: 'is_unsubscribed',
    value: true,
    total: (s) => s.total_unsubscribed
  },
  failed: { orderBy: 'failed_at', flag: 'is_failed', value: true, total: (s) => s.total_failed }
}

const stepDate = (message: MessageHistory, orderBy: MessageOrderBy): string | undefined =>
  message[orderBy as keyof MessageHistory] as string | undefined

export interface RecipientVariant {
  /** "A", "B"… */
  letter: string
  templateId: string
}

export interface BroadcastRecipientsProps {
  workspaceId: string
  permissions?: UserPermissions | null
  broadcast: Broadcast
  timezone: string
  step: RecipientStep
  /** The totals of the whole broadcast, and of each variant by template id. */
  stats: MessageHistoryStatusSum
  variationStats: Record<string, MessageHistoryStatusSum>
  /** The variants of an A/B test; empty without one. */
  variants: RecipientVariant[]
  smtp: boolean
  lists: Pick<List, 'id' | 'name'>[]
  /** Reloads the list while the broadcast sends, as long as only its first page is loaded. */
  refetchInterval: number | false
}

/** The recipients at one step of the funnel, latest first by that step's date. */
export function BroadcastRecipients({
  workspaceId,
  permissions,
  broadcast,
  timezone,
  step,
  stats,
  variationStats,
  variants,
  smtp,
  lists,
  refetchInterval
}: BroadcastRecipientsProps) {
  const { t } = useLingui()
  const { int } = useNumberFormat()
  const dates = useBroadcastDates()
  const navigate = useNavigate()
  const [order, setOrder] = useState<'desc' | 'asc'>('desc')
  const [variant, setVariant] = useState<string>('all')
  const [draft, setDraft] = useState('')
  const [email, setEmail] = useState<string | undefined>()
  const [invalid, setInvalid] = useState(false)
  const [open, setOpen] = useState<MessageHistory | null>(null)
  const hintId = useId()
  const queryClient = useQueryClient()

  const query = STEP_QUERY[step]
  const templateId = variant === 'all' ? undefined : variant
  const filters: MessageFilterParams = {
    broadcast_id: broadcast.id,
    [query.flag]: query.value,
    ...(templateId ? { template_id: templateId } : {}),
    ...(email ? { contact_email: email } : {})
  }

  const queryKey: QueryKey = [
    'broadcast-recipients',
    workspaceId,
    broadcast.id,
    step,
    order,
    templateId ?? '',
    email ?? ''
  ]

  // Keeps the first page only. The pages behind it stay out of the cache, where TanStack Query
  // would reload them all on each refresh and the reload while sending would stay off.
  const firstPageOnly = (key: QueryKey) => {
    queryClient.setQueryData<InfiniteData<MessageListResult, string | undefined>>(key, (old) =>
      old && old.pages.length > 1 ? { pages: old.pages.slice(0, 1), pageParams: old.pageParams.slice(0, 1) } : old
    )
  }
  // Another step, order, variant or email, or a closed drawer, leaves the list it showed on its
  // first rows.
  const keyString = JSON.stringify(queryKey)
  useEffect(() => () => firstPageOnly(JSON.parse(keyString) as QueryKey), [keyString]) // eslint-disable-line react-hooks/exhaustive-deps

  const listQuery = useInfiniteQuery({
    queryKey,
    queryFn: ({ pageParam }) =>
      listMessages(workspaceId, {
        ...filters,
        order_by: query.orderBy,
        order,
        cursor: pageParam,
        limit: pageParam ? RECIPIENTS_MORE : RECIPIENTS_SHOWN
      }),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => (last.has_more ? last.next_cursor : undefined),
    // TanStack Query refetches every loaded page at each interval: only a single page is.
    refetchInterval: (q) => (refetchInterval && (q.state.data?.pages.length ?? 1) === 1 ? refetchInterval : false),
    staleTime: 30_000,
    retry: false
  })

  const rows = listQuery.data?.pages.flatMap((page) => page.messages) ?? []
  // A failed first load has nothing to show; a failed "Show more" or reload keeps the rows loaded.
  const failedEmpty = listQuery.isError && rows.length === 0
  const failedMore = listQuery.isError && rows.length > 0
  const settled = !listQuery.isLoading && !failedEmpty
  const canMore = settled && listQuery.hasNextPage
  const canLess = settled && (listQuery.data?.pages.length ?? 0) > 1

  const totals = templateId ? variationStats[templateId] : stats
  const total = totals ? query.total(totals) : undefined

  const stepLabels: Record<RecipientStep, string> = {
    sent: t`Sent`,
    delivered: t`Delivered`,
    opened: t`Opened`,
    clicked: t`Clicked`,
    bounced: t`Bounced`,
    complained: t`Marked as spam`,
    unsubscribed: t`Unsubscribed`,
    failed: t`Failed`
  }
  // opened_at and clicked_at keep the first open and the first click.
  const dateHeaders: Record<RecipientStep, string> = {
    ...stepLabels,
    opened: t`First opened`,
    clicked: t`First clicked`
  }

  const delay = (message: MessageHistory, at: string): string | null => {
    if (step === 'sent' || !message.sent_at) return null
    const minutes = Math.floor((new Date(at).getTime() - new Date(message.sent_at).getTime()) / 60_000)
    if (minutes < 0) return null
    if (minutes < 60) return t`+${minutes} min`
    if (minutes < 24 * 60) {
      const hours = Math.floor(minutes / 60)
      return t`+${hours} h`
    }
    const days = Math.floor(minutes / (24 * 60))
    return t`+${days} d`
  }

  const search = (value: string) => {
    const normalized = value.trim().toLowerCase()
    if (normalized === '') {
      setInvalid(false)
      setEmail(undefined)
      return
    }
    if (!isFullEmail(normalized)) {
      setInvalid(true)
      return
    }
    setInvalid(false)
    setEmail(normalized)
  }

  const openLogs = () => {
    const params = new URLSearchParams()
    params.set('broadcast_id', broadcast.id)
    params.set(query.flag, String(query.value))
    if (templateId) params.set('template_id', templateId)
    if (email) params.set('contact_email', email)
    navigate({ to: `/console/workspace/${workspaceId}/logs?${params.toString()}` as string & {} })
  }

  const onTableChange: TableProps<MessageHistory>['onChange'] = (_pagination, _filters, sorter) => {
    const next = Array.isArray(sorter) ? sorter[0] : sorter
    setOrder(next?.order === 'ascend' ? 'asc' : 'desc')
  }

  const countText = total === undefined ? null : plural(total, { one: '# recipient', other: '# recipients' })
  const shownTotal = rows.length
  const shownCount = int(shownTotal)
  const totalCount = total === undefined ? '' : int(total)
  const footerCount = failedMore
    ? t`Couldn't load the recipients`
    : !settled
      ? null
      : email
        ? plural(shownTotal, { one: 'Showing # recipient', other: 'Showing # recipients' })
        : total === undefined
          ? null
          : t`Showing ${shownCount} of ${totalCount}`

  return (
    <>
      <Panel
        flush
        title={
          <span>
            {stepLabels[step]}
            {countText ? <span className="font-normal text-ink-3"> · {countText}</span> : null}
          </span>
        }
        extra={
          <>
            {variants.length > 0 ? (
              <Segmented<string>
                size="small"
                value={variant}
                onChange={setVariant}
                options={[
                  { value: 'all', label: t`All` },
                  ...variants.map((v) => ({ value: v.templateId, label: v.letter }))
                ]}
              />
            ) : null}
            <span className="w-52">
              <Input
                size="small"
                allowClear
                aria-label={t`Exact email`}
                aria-invalid={invalid || undefined}
                aria-describedby={invalid ? hintId : undefined}
                status={invalid ? 'error' : undefined}
                placeholder={t`Exact email`}
                value={draft}
                onChange={(event) => {
                  setDraft(event.target.value)
                  if (invalid) setInvalid(false)
                  // Cleared with the cross or by hand: the whole step again, without Enter.
                  if (event.target.value === '') search('')
                }}
                onPressEnter={(event: KeyboardEvent<HTMLInputElement>) => search(event.currentTarget.value)}
                onKeyDown={(event) => {
                  if (event.key === 'Escape') setInvalid(false)
                }}
              />
            </span>
          </>
        }
        footer={
          <>
            <span>{footerCount}</span>
            <span className="flex items-center gap-1">
              {canLess ? (
                <Button type="link" size="small" onClick={() => firstPageOnly(queryKey)}>
                  {t`Show less`}
                </Button>
              ) : null}
              {canMore ? (
                <Button
                  type="link"
                  size="small"
                  loading={listQuery.isFetchingNextPage}
                  onClick={() => void listQuery.fetchNextPage()}
                >
                  {t`Show more`}
                </Button>
              ) : null}
              <Button type="link" size="small" onClick={openLogs}>
                {t`Open in logs`}
              </Button>
            </span>
          </>
        }
      >
        {/* Always there, so a screen reader reads the hint when it appears */}
        <div aria-live="polite">
          {invalid ? (
            <p data-unreset id={hintId} className="m-0 px-3.5 pt-2 text-xs text-ink-3">
              {t`Enter a full email address`}
            </p>
          ) : null}
        </div>
        {failedEmpty ? (
          <div className="flex items-center gap-2 px-3.5 py-3">
            <p data-unreset className="m-0 text-sm text-ink-3">{t`Couldn't load the recipients`}</p>
            <Button size="small" onClick={() => void listQuery.refetch()}>
              {t`Retry`}
            </Button>
          </div>
        ) : (
          <Table<MessageHistory>
            rowKey="id"
            size="small"
            loading={listQuery.isLoading}
            dataSource={rows}
            pagination={false}
            tableLayout="fixed"
            scroll={{ x: 640 }}
            onChange={onTableChange}
            showSorterTooltip={false}
            locale={{ emptyText: email ? t`No results match` : t`No recipients at this step yet` }}
            columns={[
              {
                key: 'contact',
                title: t`Contact`,
                render: (_, message) =>
                  message.contact_email === ERASED_EMAIL ? (
                    <span className="text-ink-3">{t`Contact erased`}</span>
                  ) : (
                    <Mono>{message.contact_email}</Mono>
                  )
              },
              {
                key: 'journey',
                title: t`Journey`,
                width: 250,
                render: (_, message) => <MessageJourneySteps message={message} smtp={smtp} timezone={timezone} />
              },
              {
                key: 'date',
                title: dateHeaders[step],
                width: 230,
                sorter: true,
                sortOrder: order === 'asc' ? 'ascend' : 'descend',
                sortDirections: ['descend', 'ascend', 'descend'],
                render: (_, message) => {
                  const at = stepDate(message, query.orderBy)
                  if (!at) return null
                  const after = delay(message, at)
                  return (
                    <span className="whitespace-nowrap tabular-nums">
                      {dates.dateTime(at, timezone)}
                      {after ? <span className="text-ink-3"> {after}</span> : null}
                    </span>
                  )
                }
              },
              {
                key: 'view',
                width: 48,
                align: 'right',
                render: (_, message) => (
                  <Tooltip title={t`View message`}>
                    <Button
                      type="text"
                      aria-label={t`View message`}
                      icon={<Eye size={14} aria-hidden="true" />}
                      onClick={() => setOpen(message)}
                    />
                  </Tooltip>
                )
              }
            ]}
          />
        )}
      </Panel>
      {open ? (
        <MessageDetail
          workspaceId={workspaceId}
          permissions={permissions ?? null}
          id={open.id}
          row={open}
          names={{
            broadcast: (id) => (id === broadcast.id ? broadcast.name : undefined),
            automation: () => undefined,
            list: (id) => lists.find((l) => l.id === id)?.name
          }}
          copyLink={false}
          onClose={() => setOpen(null)}
        />
      ) : null}
    </>
  )
}
