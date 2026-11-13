import { useMemo, useState } from 'react'
import { useInfiniteQuery, useQuery, useQueryClient } from '@tanstack/react-query'
import { Alert, Button, Empty, Result, Skeleton, Table, Tooltip, type MenuProps } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { useLingui } from '@lingui/react/macro'
import { plural } from '@lingui/core/macro'
import { useNavigate } from '@tanstack/react-router'
import { ChevronRight, RefreshCw } from 'lucide-react'
import { useAuth } from '../../contexts/AuthContext'
import {
  webhookSubscriptionApi,
  type WebhookDelivery
} from '../../services/api/webhook_subscription'
import {
  KeyValueList,
  Mono,
  Panel,
  Pill,
  SidePanel,
  TableToolbar,
  useNumberFormat,
  type PillTone
} from '../layout'
import { CodeBlock } from './CodeBlock'
import { FilterChip } from './FilterChip'
import { DELIVERY_STATUS_LABELS } from './labels'
import { DELIVERY_STATUSES, type DeliveryStatus } from './logsSearch'
import { formatLogDateTime, formatLogTime } from './logTime'
import { useLogsUrl } from './useLogsUrl'
import { deliveryState, type DeliveryState } from './deliveryState'

const PAGE_SIZE = 50

const STATE_TONES: Record<DeliveryState, PillTone> = {
  delivered: 'success',
  pending: 'neutral',
  delivering: 'info',
  retrying: 'warning',
  failed: 'danger'
}

export interface OutgoingWebhooksTabProps {
  workspaceId: string
}

/** Logs → Outgoing webhooks: what Notifuse sent to the workspace's webhook subscriptions. */
export function OutgoingWebhooksTab({ workspaceId }: OutgoingWebhooksTabProps) {
  const { t, i18n } = useLingui()
  const { int } = useNumberFormat()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { workspaces } = useAuth()
  const timezone = workspaces.find((workspace) => workspace.id === workspaceId)?.settings.timezone
  const { search, update, clear } = useLogsUrl(workspaceId, 'outgoing-webhooks')
  const subscriptionId = search.subscription_id
  const status = search.status
  const filtered = subscriptionId !== undefined || status !== undefined
  const [openId, setOpenId] = useState<string>()

  // O1: the subscriptions, for the filter and the names.
  const subscriptionsQuery = useQuery({
    queryKey: ['webhook-subscriptions', workspaceId],
    queryFn: () => webhookSubscriptionApi.list(workspaceId),
    staleTime: 5 * 60 * 1000
  })
  const subscriptions = subscriptionsQuery.data?.subscriptions ?? []
  const subscriptionName = (id: string) =>
    subscriptions.find((subscription) => subscription.id === id)?.name

  // O2: deliveries by offset, 50 at a time.
  const deliveriesQuery = useInfiniteQuery({
    queryKey: ['logs', 'deliveries', workspaceId, { subscriptionId, status }],
    queryFn: ({ pageParam }) =>
      webhookSubscriptionApi.getDeliveries(
        workspaceId,
        subscriptionId,
        PAGE_SIZE,
        pageParam,
        status
      ),
    initialPageParam: 0,
    getNextPageParam: (last) => {
      const next = last.offset + last.deliveries.length
      return last.deliveries.length > 0 && next < last.total ? next : undefined
    },
    staleTime: 30_000
  })

  // A delivery created between two calls shifts the offset, and the next page repeats a row.
  const deliveries = useMemo(() => {
    const seen = new Set<string>()
    const out: WebhookDelivery[] = []
    for (const pageData of deliveriesQuery.data?.pages ?? []) {
      for (const delivery of pageData.deliveries) {
        if (seen.has(delivery.id)) continue
        seen.add(delivery.id)
        out.push(delivery)
      }
    }
    return out
  }, [deliveriesQuery.data])
  const total = deliveriesQuery.data?.pages[0]?.total

  const refresh = () =>
    void queryClient.resetQueries({ queryKey: ['logs', 'deliveries', workspaceId] })

  const subscriptionMenu: MenuProps = {
    items: [
      { key: '', label: t`All` },
      ...subscriptions.map((subscription) => ({ key: subscription.id, label: subscription.name }))
    ],
    selectable: true,
    selectedKeys: [subscriptionId ?? ''],
    onClick: ({ key }) => update({ subscription_id: key || undefined })
  }
  const statusMenu: MenuProps = {
    items: [
      { key: '', label: t`All` },
      ...DELIVERY_STATUSES.map((value) => ({
        key: value,
        label: i18n._(DELIVERY_STATUS_LABELS[value])
      }))
    ],
    selectable: true,
    selectedKeys: [status ?? ''],
    onClick: ({ key }) => update({ status: (key || undefined) as DeliveryStatus | undefined })
  }

  const toolbar = (
    <TableToolbar
      start={
        <>
          <FilterChip
            label={t`Subscription`}
            value={subscriptionId ? (subscriptionName(subscriptionId) ?? subscriptionId) : t`All`}
            menu={subscriptionMenu}
          />
          <FilterChip
            label={t`Status`}
            value={status ? i18n._(DELIVERY_STATUS_LABELS[status]) : t`All`}
            menu={statusMenu}
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

  const columns: ColumnsType<WebhookDelivery> = [
    {
      title: t`Time`,
      key: 'time',
      width: 150,
      render: (_, delivery) => (
        <Tooltip title={formatLogDateTime(delivery.created_at, timezone)}>
          <span className="text-ink-3 tabular-nums">
            {formatLogTime(i18n, delivery.created_at, timezone)}
          </span>
        </Tooltip>
      )
    },
    { title: t`Event`, key: 'event', render: (_, delivery) => <Mono>{delivery.event_type}</Mono> },
    {
      title: t`Subscription`,
      key: 'subscription',
      render: (_, delivery) => {
        const name = subscriptionName(delivery.subscription_id)
        return name ?? <Mono>{delivery.subscription_id}</Mono>
      }
    },
    {
      title: t`Status`,
      key: 'status',
      render: (_, delivery) => {
        const state = deliveryState(delivery)
        return <Pill tone={STATE_TONES[state]}>{i18n._(DELIVERY_STATUS_LABELS[state])}</Pill>
      }
    },
    {
      title: t`Attempts`,
      key: 'attempts',
      render: (_, delivery) => (
        <span className="tabular-nums">
          {delivery.attempts}/{delivery.max_attempts}
        </span>
      )
    },
    {
      title: t`Response`,
      key: 'response',
      render: (_, delivery) => <ResponseCell delivery={delivery} />
    },
    {
      key: 'open',
      width: 32,
      render: () => <ChevronRight size={14} className="text-ink-4" aria-hidden="true" />
    }
  ]

  let body
  if (deliveriesQuery.isPending) {
    body = (
      <div className="p-3.5">
        <Skeleton active title={false} paragraph={{ rows: 8 }} />
      </div>
    )
  } else if (deliveriesQuery.isError) {
    body = (
      <Result
        status="error"
        title={t`Couldn't load the deliveries`}
        subTitle={
          deliveriesQuery.error instanceof Error ? deliveriesQuery.error.message : undefined
        }
        extra={<Button onClick={() => void deliveriesQuery.refetch()}>{t`Retry`}</Button>}
      />
    )
  } else if (deliveries.length === 0) {
    body = (
      <div className="py-10">
        {filtered ? (
          <Empty description={t`No results match`}>
            <Button onClick={() => clear()}>{t`Clear filters`}</Button>
          </Empty>
        ) : (
          <Empty description={t`No webhook deliveries yet`}>
            <Button
              onClick={() =>
                void navigate({
                  to: '/console/workspace/$workspaceId/settings/$section',
                  params: { workspaceId, section: 'webhooks' }
                })
              }
            >
              {t`Add a webhook`}
            </Button>
          </Empty>
        )}
      </div>
    )
  } else {
    body = (
      <Table<WebhookDelivery>
        size="small"
        columns={columns}
        dataSource={deliveries}
        rowKey="id"
        pagination={false}
        rowClassName={(delivery) =>
          delivery.id === openId ? 'cursor-pointer is-hover' : 'cursor-pointer'
        }
        onRow={(delivery) => ({
          onClick: () => setOpenId(delivery.id),
          onKeyDown: (event) => {
            if (event.key === 'Enter') setOpenId(delivery.id)
          },
          tabIndex: 0
        })}
      />
    )
  }

  const shown = int(deliveries.length)
  const footer =
    deliveries.length > 0 && total !== undefined ? (
      <div className="flex w-full items-center justify-between">
        <span className="text-sm text-ink-3">
          {plural(total, { one: `${shown} of # delivery`, other: `${shown} of # deliveries` })}
        </span>
        {deliveriesQuery.hasNextPage ? (
          <Button
            size="small"
            loading={deliveriesQuery.isFetchingNextPage}
            onClick={() => void deliveriesQuery.fetchNextPage()}
          >
            {t`Load more`}
          </Button>
        ) : null}
      </div>
    ) : undefined

  const openIndex = deliveries.findIndex((delivery) => delivery.id === openId)
  const open = openIndex >= 0 ? deliveries[openIndex] : undefined

  return (
    <>
      <Panel flush footer={footer}>
        {toolbar}
        {body}
      </Panel>
      <DeliveryDetail
        delivery={open}
        workspaceId={workspaceId}
        timezone={timezone}
        subscriptionName={open ? subscriptionName(open.subscription_id) : undefined}
        onClose={() => setOpenId(undefined)}
        onPrev={openIndex > 0 ? () => setOpenId(deliveries[openIndex - 1].id) : undefined}
        onNext={
          openIndex >= 0 && openIndex < deliveries.length - 1
            ? () => setOpenId(deliveries[openIndex + 1].id)
            : undefined
        }
        position={openIndex + 1}
        total={deliveries.length}
      />
    </>
  )
}

function ResponseCell({ delivery }: { delivery: WebhookDelivery }) {
  const code = delivery.last_response_status
  if (code === undefined && !delivery.last_error) return <span className="text-ink-4">—</span>
  return (
    <span className="flex min-w-0 items-center gap-2">
      {code !== undefined ? (
        <Pill tone={code >= 200 && code < 300 ? 'success' : 'danger'} dot={false}>
          {code}
        </Pill>
      ) : null}
      {delivery.last_error ? (
        <Tooltip title={delivery.last_error}>
          <span className="max-w-[260px] truncate text-ink-3">{delivery.last_error}</span>
        </Tooltip>
      ) : null}
    </span>
  )
}

interface DeliveryDetailProps {
  delivery?: WebhookDelivery
  workspaceId: string
  timezone?: string
  subscriptionName?: string
  onClose: () => void
  onPrev?: () => void
  onNext?: () => void
  position: number
  total: number
}

function DeliveryDetail({
  delivery,
  workspaceId,
  timezone,
  subscriptionName,
  onClose,
  onPrev,
  onNext,
  position,
  total
}: DeliveryDetailProps) {
  const { t, i18n } = useLingui()
  const navigate = useNavigate()
  if (!delivery) return <SidePanel open={false} onClose={onClose} title="" copyLink={false} />

  const state = deliveryState(delivery)
  const date = (value?: string) => (value ? formatLogDateTime(value, timezone) : undefined)
  const subscription = subscriptionName ?? delivery.subscription_id

  return (
    <SidePanel
      open
      onClose={onClose}
      title={`${delivery.event_type} · ${subscription}`}
      status={<Pill tone={STATE_TONES[state]}>{i18n._(DELIVERY_STATUS_LABELS[state])}</Pill>}
      meta={[date(delivery.created_at)]}
      nav={{ onPrev, onNext, position, total }}
      // The detail has no URL of its own: a copied link would not reopen it.
      copyLink={false}
      footer={
        <Button
          disabled={subscriptionName === undefined}
          onClick={() =>
            void navigate({
              to: '/console/workspace/$workspaceId/settings/$section',
              params: { workspaceId, section: 'webhooks' },
              search: { webhook: delivery.subscription_id }
            })
          }
        >
          {t`Open subscription`}
        </Button>
      }
    >
      <div key={delivery.id} className="flex flex-col gap-4">
        <KeyValueList
          items={[
            { label: t`Delivery ID`, value: <Mono copy>{delivery.id}</Mono> },
            { label: t`Subscription ID`, value: <Mono copy>{delivery.subscription_id}</Mono> },
            { label: t`Created`, value: date(delivery.created_at) },
            { label: t`Last attempt`, value: date(delivery.last_attempt_at) },
            { label: t`Delivered`, value: date(delivery.delivered_at) },
            ...(state === 'retrying' || state === 'pending'
              ? [{ label: t`Next attempt`, value: date(delivery.next_attempt_at) }]
              : []),
            {
              label: t`Attempts`,
              value: (
                <span className="tabular-nums">
                  {delivery.attempts}/{delivery.max_attempts}
                </span>
              )
            }
          ]}
        />
        <section className="flex flex-col gap-2">
          <h4 data-unreset className="m-0 text-sm font-semibold text-ink">
            {t`Response`}
          </h4>
          {delivery.last_error ? (
            <Alert type="warning" showIcon title={delivery.last_error} />
          ) : null}
          {delivery.last_response_status !== undefined ? (
            <>
              <div>
                <Pill
                  tone={
                    delivery.last_response_status >= 200 && delivery.last_response_status < 300
                      ? 'success'
                      : 'danger'
                  }
                  dot={false}
                >
                  {delivery.last_response_status}
                </Pill>
              </div>
              {delivery.last_response_body ? (
                <CodeBlock value={delivery.last_response_body} label={t`response`} />
              ) : null}
            </>
          ) : (
            <span className="text-sm text-ink-3">{t`No response`}</span>
          )}
        </section>
        <section className="flex flex-col gap-2">
          <h4 data-unreset className="m-0 text-sm font-semibold text-ink">
            {t`Payload`}
          </h4>
          <CodeBlock value={delivery.payload} label={t`payload`} />
        </section>
      </div>
    </SidePanel>
  )
}
