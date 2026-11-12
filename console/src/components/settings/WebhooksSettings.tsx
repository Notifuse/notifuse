import { useEffect, useMemo, useState } from 'react'
import {
  Alert,
  Button,
  Checkbox,
  Col,
  Drawer,
  Empty,
  Form,
  Input,
  Result,
  Row,
  Select,
  Skeleton,
  Space,
  Table,
  Tooltip,
  message
} from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { ChevronRight, Globe, Plus, RefreshCw } from 'lucide-react'
import { keepPreviousData, useQuery, useQueryClient } from '@tanstack/react-query'
import { useLingui } from '@lingui/react/macro'
import { SettingsSectionHeader } from './SettingsSectionHeader'
import { WebhookDetail } from './WebhookDetail'
import { invalidateWebhookQueries, webhookQueryKeys } from './webhookQueries'
import Subtitle from '../common/subtitle'
import { Mono, Panel, Pill, useNumberFormat, useRelativeDate, type PillTone } from '../layout'
import {
  webhookSubscriptionApi,
  WebhookSubscription,
  CustomEventFilters,
  UpdateWebhookSubscriptionRequest
} from '../../services/api/webhook_subscription'
import { analyticsService } from '../../services/api/analytics'
import { ApiError } from '../../services/api/errors'

interface WebhooksSettingsProps {
  workspaceId: string
  /** Owner or `webhook_subscriptions.write`: every write is disabled without it. */
  canManage: boolean
  /** The subscription whose panel is open (`?webhook=`). */
  openWebhookId?: string
  /** Opens a subscription's panel, or closes it without an id. */
  onOpenWebhook: (id?: string) => void
}

interface DeliveryCounts {
  delivered: number
  failed: number
}

const DAY = 24 * 60 * 60 * 1000
const MINUTE = 60 * 1000

/** Seven days back, rounded down to the minute so the query key holds still between renders. */
const sevenDaysAgo = () =>
  new Date(Math.floor((Date.now() - 7 * DAY) / MINUTE) * MINUTE).toISOString()

/** The success rate rounded as it is shown, so the tone never contradicts the figure. */
function rateTone(ratio: number): PillTone {
  const shown = Math.round(ratio * 1000) / 1000
  if (shown >= 0.99) return 'success'
  if (shown >= 0.9) return 'warning'
  return 'danger'
}

/** A long URL cut in its middle, which keeps the host and the end of the path. */
function middleTruncate(text: string, max = 44): string {
  if (text.length <= max) return text
  const tail = Math.floor((max - 1) / 3)
  return `${text.slice(0, max - 1 - tail)}…${text.slice(-tail)}`
}

// One above antd's drawer default (1000), which the detail panel uses.
const EDIT_DRAWER_Z_INDEX = 1001

export function WebhooksSettings({
  workspaceId,
  canManage,
  openWebhookId,
  onOpenWebhook
}: WebhooksSettingsProps) {
  const { t } = useLingui()
  const queryClient = useQueryClient()
  const relativeDate = useRelativeDate()
  const { percent } = useNumberFormat()
  const [drawerVisible, setDrawerVisible] = useState(false)
  const [editingSubscription, setEditingSubscription] = useState<WebhookSubscription | null>(null)
  const [form] = Form.useForm()
  const [saving, setSaving] = useState(false)
  // W2 counts from this time; it moves only with Refresh.
  const [from, setFrom] = useState(sevenDaysAgo)
  useEffect(() => {
    setFrom(sevenDaysAgo())
  }, [workspaceId])

  // W1. The cache holds the raw response, as the Outgoing webhooks tab of Logs reads it.
  const listQuery = useQuery({
    queryKey: webhookQueryKeys.list(workspaceId),
    queryFn: () => webhookSubscriptionApi.list(workspaceId)
  })
  const subscriptions = useMemo(() => listQuery.data?.subscriptions ?? [], [listQuery.data])

  const eventTypesQuery = useQuery({
    queryKey: ['webhook-event-types'],
    queryFn: () => webhookSubscriptionApi.getEventTypes(),
    staleTime: Infinity
  })
  const eventTypes = eventTypesQuery.data?.event_types ?? []

  // W2: one grouped query for every subscription.
  const ratesQuery = useQuery({
    queryKey: webhookQueryKeys.successRates(workspaceId, from),
    queryFn: async () => {
      const result = await analyticsService.query(
        {
          schema: 'webhook_deliveries',
          measures: ['count_delivered', 'count_failed'],
          dimensions: ['subscription_id'],
          filters: [{ member: 'created_at', operator: 'gte', values: [from] }]
        },
        workspaceId
      )
      const counts = new Map<string, DeliveryCounts>()
      for (const row of result.data) {
        counts.set(String(row.subscription_id), {
          delivered: Number(row.count_delivered) || 0,
          failed: Number(row.count_failed) || 0
        })
      }
      return counts
    },
    placeholderData: keepPreviousData
  })

  const refresh = () => {
    setFrom(sevenDaysAgo())
    void queryClient.invalidateQueries({
      queryKey: webhookQueryKeys.list(workspaceId)
    })
  }

  // Neither of these seeds `enabled`: the drawer registers no such Form.Item, so a value put in
  // the store here is never returned by validateFields and only makes the switch look like it
  // round-trips through the form. The drawer does not send the flag at all, and the switch
  // itself lives in the subscription's panel.
  const handleCreate = () => {
    setEditingSubscription(null)
    form.resetFields()
    form.setFieldsValue({
      event_types: []
    })
    setDrawerVisible(true)
  }

  const handleEdit = (subscription: WebhookSubscription) => {
    setEditingSubscription(subscription)
    form.setFieldsValue({
      name: subscription.name,
      url: subscription.url,
      event_types: subscription.settings.event_types,
      custom_event_goal_types: subscription.custom_event_filters?.goal_types,
      custom_event_names: subscription.custom_event_filters?.event_names
    })
    setDrawerVisible(true)
  }

  const handleSave = async () => {
    try {
      const values = await form.validateFields()
      setSaving(true)

      // Build custom_event_filters if any custom_event type is selected
      let customEventFilters: CustomEventFilters | undefined
      const hasCustomEvent = values.event_types?.some((t: string) => t.startsWith('custom_event.'))
      if (hasCustomEvent) {
        if (values.custom_event_goal_types?.length || values.custom_event_names?.length) {
          customEventFilters = {
            goal_types: values.custom_event_goal_types?.length
              ? values.custom_event_goal_types
              : undefined,
            event_names: values.custom_event_names?.length ? values.custom_event_names : undefined
          }
        }
      }

      if (editingSubscription) {
        // enabled is deliberately absent: it is one of the keys this endpoint patches
        // instead of replacing, so a body that never mentions it leaves the subscription
        // set to whatever it is set to now. Sending editingSubscription.enabled would send
        // the value read when the drawer opened, and a stale true switches back on a
        // subscription somebody disabled from its panel meanwhile — wiping its failure
        // counters on the way through. The switch in the panel is the only place enabled
        // moves. Nothing here casts the key away any more; the request type says it is
        // optional, which is what keeps the next author from innocently filling it in.
        const update: UpdateWebhookSubscriptionRequest = {
          workspace_id: workspaceId,
          id: editingSubscription.id,
          name: values.name,
          url: values.url,
          event_types: values.event_types,
          // Named on every save, empty included. The endpoint keeps a filter the body does
          // not name, and this drawer is the only place a user can remove these — it renders
          // their controls and rebuilds the filter from them each time — so "the controls
          // are empty" is a removal and has to be said rather than implied.
          custom_event_filters: customEventFilters ?? {},
          // The opposite case: no control here owns the list and segment filters — Zapier
          // writes them when it registers a Zap — so the drawer has no value of its own to
          // send and passes back what it read. The endpoint would keep them either way now;
          // echoing them is what makes that independent of the server's contract.
          list_ids: editingSubscription.settings.list_ids,
          segment_ids: editingSubscription.settings.segment_ids
        }
        await webhookSubscriptionApi.update(update)
        message.success(t`Webhook subscription updated`)
      } else {
        await webhookSubscriptionApi.create({
          workspace_id: workspaceId,
          name: values.name,
          url: values.url,
          event_types: values.event_types,
          custom_event_filters: customEventFilters
        })
        message.success(t`Webhook subscription created`)
      }

      setDrawerVisible(false)
      await invalidateWebhookQueries(queryClient, workspaceId)
    } catch (error) {
      console.error('Failed to save webhook subscription:', error)
      message.error(t`Failed to save webhook subscription`)
    } finally {
      setSaving(false)
    }
  }

  const formatEventType = (eventType: string) => {
    return eventType
  }

  // Zapier owns the endpoint URL of the subscriptions it creates: repointing it
  // here leaves the Zap switched on in Zapier while it silently stops firing.
  const editingZapierSubscription = editingSubscription?.source === 'zapier'

  const selectedEventTypes = Form.useWatch('event_types', form)
  const showCustomEventFilters = selectedEventTypes?.some((t: string) =>
    t.startsWith('custom_event.')
  )

  const noPermission = canManage ? undefined : t`You don't have permission to manage webhooks`
  const addButton = (
    <Tooltip title={noPermission}>
      <span className="inline-block">
        <Button
          type="primary"
          size="small"
          icon={<Plus size={14} aria-hidden="true" />}
          onClick={handleCreate}
          disabled={!canManage}
        >
          {t`Add endpoint`}
        </Button>
      </span>
    </Tooltip>
  )

  const rateCell = (subscription: WebhookSubscription) => {
    if (ratesQuery.isPending) return <Skeleton.Button active size="small" />
    if (ratesQuery.isError) {
      return (
        <Tooltip title={t`Couldn't load the success rate`}>
          <span className="text-ink-4">—</span>
        </Tooltip>
      )
    }
    const counts = ratesQuery.data?.get(subscription.id)
    const attempts = counts ? counts.delivered + counts.failed : 0
    if (!counts || attempts === 0) return <span className="text-ink-4">—</span>
    const ratio = counts.delivered / attempts
    return <Pill tone={rateTone(ratio)}>{percent(ratio)}</Pill>
  }

  const columns: ColumnsType<WebhookSubscription> = [
    {
      title: t`Endpoint`,
      key: 'endpoint',
      render: (_, subscription) => (
        <div className="flex min-w-0 items-center gap-2.5">
          <Globe size={15} aria-hidden="true" className="shrink-0 text-ink-3" />
          <div className="flex min-w-0 flex-col gap-0.5">
            <div className="flex min-w-0 items-center gap-1.5">
              <span className="truncate font-medium text-ink">{subscription.name}</span>
              {subscription.source === 'zapier' ? (
                <Pill tone="neutral" dot={false}>
                  {t`Zapier`}
                </Pill>
              ) : null}
              {!subscription.enabled ? (
                subscription.disabled_reason ? (
                  <Pill tone="warning">{t`Auto-disabled`}</Pill>
                ) : (
                  <Pill tone="neutral">{t`Disabled`}</Pill>
                )
              ) : null}
            </div>
            <Tooltip title={subscription.url}>
              <span className="min-w-0">
                <Mono>{middleTruncate(subscription.url)}</Mono>
              </span>
            </Tooltip>
          </div>
        </div>
      )
    },
    {
      title: t`Events`,
      key: 'events',
      width: 200,
      render: (_, subscription) => {
        const types = subscription.settings.event_types ?? []
        if (types.length === 0) return <span className="text-ink-4">—</span>
        const more = types.length - 1
        return (
          <Tooltip
            title={
              <div className="flex flex-col">
                {types.map((type) => (
                  <span key={type}>{type}</span>
                ))}
              </div>
            }
          >
            <span className="inline-flex min-w-0 items-center gap-1.5">
              <Mono>{types[0]}</Mono>
              {more > 0 ? <span className="shrink-0 text-xs text-ink-3">+{more}</span> : null}
            </span>
          </Tooltip>
        )
      }
    },
    {
      title: t`Success 7d`,
      key: 'success',
      width: 110,
      render: (_, subscription) => rateCell(subscription)
    },
    {
      title: t`Last success`,
      key: 'last-success',
      width: 150,
      render: (_, subscription) =>
        subscription.last_delivery_at ? (
          <span className="text-ink-2">{relativeDate(subscription.last_delivery_at)}</span>
        ) : (
          <span className="text-ink-3">{t`No successful delivery`}</span>
        )
    },
    {
      title: '',
      key: 'open',
      width: 40,
      align: 'right',
      render: () => <ChevronRight size={15} aria-hidden="true" className="text-ink-4" />
    }
  ]

  const listError = listQuery.error
  const forbidden = listError instanceof ApiError && listError.status === 403

  const renderList = () => {
    if (listQuery.isPending) {
      return (
        <div data-testid="webhooks-skeleton" className="px-3.5 py-3">
          <Skeleton active title={false} paragraph={{ rows: 4 }} />
        </div>
      )
    }
    if (listQuery.isError) {
      return forbidden ? (
        <Result status="warning" title={t`You don't have permission to view webhooks`} />
      ) : (
        <Result
          status="error"
          title={t`Couldn't load the webhooks`}
          subTitle={listError instanceof Error ? listError.message : undefined}
          extra={<Button onClick={() => listQuery.refetch()}>{t`Retry`}</Button>}
        />
      )
    }
    if (subscriptions.length === 0) {
      return (
        <div className="py-8">
          <Empty
            image={Empty.PRESENTED_IMAGE_SIMPLE}
            description={
              <div className="flex flex-col gap-1">
                <span className="font-medium text-ink">{t`No webhooks yet`}</span>
                <span className="text-ink-3">{t`Add an endpoint to receive real-time notifications when events occur in your workspace.`}</span>
              </div>
            }
          >
            {addButton}
          </Empty>
        </div>
      )
    }
    return (
      <Table<WebhookSubscription>
        rowKey="id"
        size="middle"
        columns={columns}
        dataSource={subscriptions}
        pagination={false}
        rowClassName={() => 'cursor-pointer'}
        onRow={(subscription) => ({
          onClick: () => onOpenWebhook(subscription.id),
          onKeyDown: (event) => {
            if (event.key === 'Enter') onOpenWebhook(subscription.id)
          },
          tabIndex: 0
        })}
      />
    )
  }

  // J and K walk the rows of the list while a panel is open.
  const openIndex = subscriptions.findIndex((subscription) => subscription.id === openWebhookId)
  const nav =
    openIndex >= 0
      ? {
          position: openIndex + 1,
          total: subscriptions.length,
          onPrev: openIndex > 0 ? () => onOpenWebhook(subscriptions[openIndex - 1].id) : undefined,
          onNext:
            openIndex < subscriptions.length - 1
              ? () => onOpenWebhook(subscriptions[openIndex + 1].id)
              : undefined
        }
      : undefined

  return (
    <>
      <SettingsSectionHeader
        title={t`Webhooks`}
        description={t`Tell your systems when something changes`}
      />

      <Panel
        flush
        title={t`Outgoing webhooks`}
        extra={
          <>
            <Tooltip title={t`Refresh`}>
              <Button
                type="text"
                size="small"
                aria-label={t`Refresh`}
                icon={<RefreshCw size={14} aria-hidden="true" />}
                onClick={refresh}
              />
            </Tooltip>
            {addButton}
          </>
        }
      >
        {renderList()}
      </Panel>

      <WebhookDetail
        workspaceId={workspaceId}
        webhookId={openWebhookId}
        placeholder={openIndex >= 0 ? subscriptions[openIndex] : undefined}
        canManage={canManage}
        // The edit drawer covers the panel at a higher z-index, which the panel's own check
        // (document order) cannot see, so its J and K stop while the drawer is open.
        nav={drawerVisible ? undefined : nav}
        onClose={() => onOpenWebhook(undefined)}
        onEdit={handleEdit}
        onNotFound={() => {
          onOpenWebhook(undefined)
          // Deleted elsewhere: the row leaves the list rather than open an empty panel again.
          void invalidateWebhookQueries(queryClient, workspaceId)
        }}
      />

      {/* Create/Edit Drawer */}
      {/* Above the detail panel it opens from: both are drawers at the same level, and the one
          whose portal comes last in the page would otherwise paint on top. */}
      <Drawer
        title={editingSubscription ? t`Edit Webhook` : t`Create Webhook`}
        size={500}
        zIndex={EDIT_DRAWER_Z_INDEX}
        open={drawerVisible}
        onClose={() => setDrawerVisible(false)}
        footer={
          <div className="text-right">
            <Space>
              <Button onClick={() => setDrawerVisible(false)}>{t`Cancel`}</Button>
              <Button type="primary" onClick={handleSave} loading={saving}>
                {t`Save`}
              </Button>
            </Space>
          </div>
        }
      >
        <Form form={form} layout="vertical">
          {editingZapierSubscription && (
            <Alert
              type="warning"
              showIcon
              className="mb-4"
              title={t`Managed by Zapier`}
              description={t`Zapier owns this webhook's endpoint URL, so it cannot be changed here. Changing the event types breaks the Zap that created this webhook without reporting any error in Zapier.`}
            />
          )}

          <Form.Item
            name="name"
            label={t`Name`}
            rules={[{ required: true, message: t`Please enter a name` }]}
          >
            <Input placeholder={t`My Webhook`} />
          </Form.Item>

          <Form.Item
            name="url"
            label={t`Endpoint URL`}
            rules={[
              { required: true, message: t`Please enter a URL` },
              { type: 'url', message: t`Please enter a valid URL` }
            ]}
          >
            <Input placeholder="https://example.com/webhook" disabled={editingZapierSubscription} />
          </Form.Item>

          <Form.Item
            name="event_types"
            label={
              <div className="flex justify-between w-full">
                <span>{t`Event Types`}</span>
                <a
                  onClick={(e) => {
                    e.preventDefault()
                    form.setFieldsValue({ event_types: eventTypes })
                  }}
                >
                  {t`Select all`}
                </a>
              </div>
            }
            rules={[
              {
                required: true,
                message: t`Please select at least one event type`
              }
            ]}
            className="[&_.ant-form-item-label]:w-full [&_.ant-form-item-label>label]:w-full [&_.ant-form-item-label>label]:inline-flex"
          >
            <Checkbox.Group className="w-full">
              <Row>
                <Col span={12}>
                  {eventTypes
                    .filter((type) => {
                      const entity = type.split('.')[0]
                      return ['contact', 'list', 'segment'].includes(entity)
                    })
                    .map((type) => (
                      <div key={type} className="mb-2">
                        <Checkbox value={type}>{formatEventType(type)}</Checkbox>
                      </div>
                    ))}
                </Col>
                <Col span={12}>
                  {eventTypes
                    .filter((type) => {
                      const entity = type.split('.')[0]
                      return !['contact', 'list', 'segment'].includes(entity)
                    })
                    .map((type) => (
                      <div key={type} className="mb-2">
                        <Checkbox value={type}>{formatEventType(type)}</Checkbox>
                      </div>
                    ))}
                </Col>
              </Row>
            </Checkbox.Group>
          </Form.Item>

          {showCustomEventFilters && (
            <>
              <Subtitle className="mb-6" borderBottom primary>
                {t`Custom Event Filters (optional)`}
              </Subtitle>
              <Form.Item name="custom_event_goal_types" label={t`Goal Types`}>
                <Select
                  mode="multiple"
                  placeholder={t`Select goal types to filter`}
                  options={[
                    { value: 'purchase', label: t`Purchase` },
                    { value: 'subscription', label: t`Subscription` },
                    { value: 'lead', label: t`Lead` },
                    { value: 'signup', label: t`Signup` },
                    { value: 'booking', label: t`Booking` },
                    { value: 'trial', label: t`Trial` },
                    { value: 'other', label: t`Other` }
                  ]}
                />
              </Form.Item>
              <Form.Item name="custom_event_names" label={t`Event Names`}>
                <Select
                  mode="tags"
                  placeholder={t`Enter event names to filter`}
                  tokenSeparators={[',']}
                />
              </Form.Item>
            </>
          )}
        </Form>
      </Drawer>
    </>
  )
}
