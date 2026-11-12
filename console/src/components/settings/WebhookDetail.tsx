import { useEffect, useRef, useState, type ReactNode } from 'react'
import {
  Alert,
  App,
  Button,
  Modal,
  Popconfirm,
  Result,
  Segmented,
  Select,
  Skeleton,
  Space,
  Switch,
  Tooltip
} from 'antd'
import { Copy, Eye, EyeOff, RotateCw } from 'lucide-react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useLingui } from '@lingui/react/macro'
import {
  webhookSubscriptionApi,
  type WebhookSubscription
} from '../../services/api/webhook_subscription'
import { analyticsService } from '../../services/api/analytics'
import { ApiError, shouldRetryQuery } from '../../services/api/errors'
import { copyToClipboard } from '../../lib/copyToClipboard'
import {
  KeyValueList,
  KpiStrip,
  Mono,
  Pill,
  SidePanel,
  useNumberFormat,
  useRelativeDate
} from '../layout'
import type { DrawerNav } from '../layout'
import { invalidateWebhookQueries, webhookQueryKeys } from './webhookQueries'

interface WebhookDetailProps {
  workspaceId: string
  /** The subscription shown; the panel is closed without one. */
  webhookId?: string
  /** Its row of the list, shown while the subscription loads. */
  placeholder?: WebhookSubscription
  /** Owner or `webhook_subscriptions.write`: every write is disabled without it. */
  canManage: boolean
  nav?: DrawerNav
  onClose: () => void
  onEdit: (subscription: WebhookSubscription) => void
  /** The subscription is gone (a stale link): the caller drops it from the URL. */
  onNotFound: () => void
}

interface TestResult {
  success: boolean
  statusCode: number
  responseBody: string
  error?: string
}

type TimeRange = '1D' | '7D'

const DAY = 24 * 60 * 60 * 1000
const isNotFound = (error: unknown) => error instanceof ApiError && error.status === 404

/** A subscription in the settings side panel: what the webhook card showed, reorganised. */
export function WebhookDetail({
  workspaceId,
  webhookId,
  placeholder,
  canManage,
  nav,
  onClose,
  onEdit,
  onNotFound
}: WebhookDetailProps) {
  const { t } = useLingui()
  const { message } = App.useApp()
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const relativeDate = useRelativeDate()
  const { int, percent } = useNumberFormat()

  // The id stays rendered while the panel slides out after the URL dropped it.
  const [shownId, setShownId] = useState(webhookId)
  useEffect(() => {
    if (webhookId) setShownId(webhookId)
  }, [webhookId])
  const id = webhookId ?? shownId

  const [visibleSecret, setVisibleSecret] = useState(false)
  const [timeRange, setTimeRange] = useState<TimeRange>('1D')
  const [testModalVisible, setTestModalVisible] = useState(false)
  const [selectedEventType, setSelectedEventType] = useState<string>('')
  const [testLoading, setTestLoading] = useState(false)
  const [testResult, setTestResult] = useState<TestResult | null>(null)
  const [zapierDeleteVisible, setZapierDeleteVisible] = useState(false)

  useEffect(() => {
    setVisibleSecret(false)
  }, [id])

  // W4: a link opens the right subscription even before the list has loaded.
  const subscriptionQuery = useQuery({
    queryKey: webhookQueryKeys.subscription(workspaceId, id ?? ''),
    queryFn: () => webhookSubscriptionApi.get(workspaceId, id as string),
    enabled: Boolean(webhookId),
    placeholderData: placeholder ? { subscription: placeholder } : undefined,
    // A 404 is an answer, not a failure: the panel closes at once.
    retry: (count, error) => !isNotFound(error) && shouldRetryQuery(count, error)
  })
  const webhook = subscriptionQuery.data?.subscription

  const reportedMissing = useRef<string | undefined>(undefined)
  useEffect(() => {
    // Closed: the next opening of the same id reports it again.
    if (!webhookId) {
      reportedMissing.current = undefined
      return
    }
    if (!isNotFound(subscriptionQuery.error) || reportedMissing.current === webhookId) return
    reportedMissing.current = webhookId
    message.info(t`This webhook no longer exists`)
    onNotFound()
  }, [webhookId, subscriptionQuery.error, message, onNotFound, t])

  // W3: the counts of the period the panel shows.
  const statsQuery = useQuery({
    queryKey: webhookQueryKeys.stats(workspaceId, id ?? '', timeRange),
    enabled: Boolean(webhookId),
    queryFn: async () => {
      const from = new Date(Date.now() - (timeRange === '1D' ? 1 : 7) * DAY)
      const result = await analyticsService.query(
        {
          schema: 'webhook_deliveries',
          measures: ['count_delivered', 'count_failed'],
          dimensions: ['subscription_id'],
          filters: [
            {
              member: 'subscription_id',
              operator: 'equals',
              values: [id as string]
            },
            {
              member: 'created_at',
              operator: 'gte',
              values: [from.toISOString()]
            }
          ]
        },
        workspaceId
      )
      const row = result.data[0]
      return {
        delivered: Number(row?.count_delivered) || 0,
        failed: Number(row?.count_failed) || 0
      }
    }
  })

  const isZapier = webhook?.source === 'zapier'
  // A subscription Notifuse switched off after sustained delivery failure carries
  // a reason; one the user switched off never does.
  const autoDisabled = Boolean(webhook && !webhook.enabled && webhook.disabled_reason)
  const consecutiveFailures = webhook?.consecutive_failures ?? 0
  const noPermission = canManage ? undefined : t`You don't have permission to manage webhooks`

  const refresh = () => invalidateWebhookQueries(queryClient, workspaceId)

  const handleToggle = async (enabled: boolean) => {
    if (!webhook) return
    try {
      await webhookSubscriptionApi.toggle({
        workspace_id: workspaceId,
        id: webhook.id,
        enabled
      })
      message.success(enabled ? t`Webhook enabled` : t`Webhook disabled`)
      await refresh()
    } catch (error) {
      console.error('Failed to toggle webhook subscription:', error)
      message.error(t`Failed to toggle webhook subscription`)
    }
  }

  const handleDelete = async () => {
    if (!webhook) return
    try {
      await webhookSubscriptionApi.delete(workspaceId, webhook.id)
      message.success(t`Webhook subscription deleted`)
      onClose()
      await refresh()
    } catch (error) {
      console.error('Failed to delete webhook subscription:', error)
      message.error(t`Failed to delete webhook subscription`)
    }
  }

  const handleRegenerateSecret = async () => {
    if (!webhook) return
    try {
      await webhookSubscriptionApi.regenerateSecret(workspaceId, webhook.id)
      message.success(t`Webhook secret regenerated`)
      await refresh()
    } catch (error) {
      console.error('Failed to regenerate secret:', error)
      message.error(t`Failed to regenerate webhook secret`)
    }
  }

  const handleCopySecret = async () => {
    if (!webhook) return
    if (await copyToClipboard(webhook.secret)) message.success(t`Secret copied to clipboard`)
    else message.error(t`Failed to copy to clipboard`)
  }

  const handleTest = async () => {
    if (!webhook || !selectedEventType) return
    try {
      setTestLoading(true)
      setTestResult(null)
      const result = await webhookSubscriptionApi.test(workspaceId, webhook.id, selectedEventType)
      setTestResult({
        success: result.success,
        statusCode: result.status_code,
        responseBody: result.response_body,
        error: result.error
      })
    } catch (error) {
      console.error('Failed to test webhook:', error)
      setTestResult({
        success: false,
        statusCode: 0,
        responseBody: '',
        error: t`Failed to send test webhook`
      })
    } finally {
      setTestLoading(false)
    }
  }

  const openTestModal = () => {
    setSelectedEventType(webhook?.settings.event_types?.[0] || '')
    setTestResult(null)
    setTestModalVisible(true)
  }

  const confirmZapierDelete = () => {
    setZapierDeleteVisible(false)
    void handleDelete()
  }

  const viewDeliveries = () =>
    navigate({
      to: '/console/workspace/$workspaceId/logs',
      params: { workspaceId },
      search: { tab: 'outgoing-webhooks', subscription_id: id }
    })

  const status = webhook ? (
    <>
      {autoDisabled ? (
        <Pill tone="warning">{t`Auto-disabled`}</Pill>
      ) : webhook.enabled ? (
        <Pill tone="success">{t`Active`}</Pill>
      ) : (
        <Pill tone="neutral">{t`Disabled`}</Pill>
      )}
      {isZapier ? (
        <Tooltip title={t`Zapier created this webhook for one of your Zaps.`}>
          <span>
            <Pill tone="neutral" dot={false}>
              {t`Zapier`}
            </Pill>
          </span>
        </Tooltip>
      ) : null}
    </>
  ) : null

  // The switch of the card, with its confirmation before switching off.
  const toggle = webhook ? (
    <Tooltip title={noPermission ?? (webhook.enabled ? t`Disable webhook` : t`Enable webhook`)}>
      {webhook.enabled ? (
        <Popconfirm
          title={t`Disable this webhook?`}
          description={t`The webhook will stop receiving events.`}
          onConfirm={() => handleToggle(false)}
          okText={t`Yes`}
          cancelText={t`No`}
          disabled={!canManage}
        >
          <Switch checked size="small" disabled={!canManage} aria-label={t`Disable webhook`} />
        </Popconfirm>
      ) : (
        <Switch
          checked={false}
          size="small"
          disabled={!canManage}
          aria-label={t`Enable webhook`}
          onChange={() => handleToggle(true)}
        />
      )}
    </Tooltip>
  ) : null

  const withPermission = (node: ReactNode) =>
    noPermission ? (
      <Tooltip title={noPermission}>
        <span>{node}</span>
      </Tooltip>
    ) : (
      node
    )

  const footer = webhook ? (
    <>
      {/* Zapier's hook URLs answer 2xx unconditionally, so a test against a
          deleted Zap still reports success — a green tick that means nothing
          is worse than no button at all. */}
      {!isZapier
        ? withPermission(
            <Button onClick={openTestModal} disabled={!canManage}>
              {t`Send test`}
            </Button>
          )
        : null}
      <Button onClick={viewDeliveries}>{t`View deliveries`}</Button>
      {withPermission(
        <Button onClick={() => onEdit(webhook)} disabled={!canManage}>
          {t`Edit`}
        </Button>
      )}
      {isZapier ? (
        withPermission(
          <Button danger onClick={() => setZapierDeleteVisible(true)} disabled={!canManage}>
            {t`Delete`}
          </Button>
        )
      ) : (
        <Popconfirm
          title={t`Delete this webhook?`}
          description={t`This action cannot be undone.`}
          onConfirm={handleDelete}
          okText={t`Yes`}
          cancelText={t`No`}
          disabled={!canManage}
        >
          {withPermission(
            <Button danger disabled={!canManage}>
              {t`Delete`}
            </Button>
          )}
        </Popconfirm>
      )}
    </>
  ) : null

  const secretValue = webhook ? (
    !webhook.secret ? (
      <span className="text-ink-3 italic">{t`Only a workspace owner can view or rotate the signing secret`}</span>
    ) : (
      <div className="flex min-w-0 flex-wrap items-center gap-1">
        <Mono>{visibleSecret ? webhook.secret : '••••••••••••••••••••••••'}</Mono>
        <Button
          type="text"
          size="small"
          icon={
            visibleSecret ? (
              <EyeOff size={14} aria-hidden="true" />
            ) : (
              <Eye size={14} aria-hidden="true" />
            )
          }
          onClick={() => setVisibleSecret(!visibleSecret)}
        >
          {visibleSecret ? t`Hide` : t`Show`}
        </Button>
        <Tooltip title={t`Copy`}>
          <Button
            type="text"
            size="small"
            aria-label={t`Copy secret`}
            icon={<Copy size={14} aria-hidden="true" />}
            onClick={handleCopySecret}
          />
        </Tooltip>
        <Popconfirm
          title={t`Regenerate secret?`}
          description={t`This will invalidate the current secret. You'll need to update your webhook receiver.`}
          onConfirm={handleRegenerateSecret}
          okText={t`Yes`}
          cancelText={t`No`}
          disabled={!canManage}
        >
          {withPermission(
            <Button
              type="text"
              size="small"
              disabled={!canManage}
              icon={<RotateCw size={14} aria-hidden="true" />}
            >
              {t`Regenerate`}
            </Button>
          )}
        </Popconfirm>
      </div>
    )
  ) : null

  const filters = webhook?.custom_event_filters
  const filterParts = [
    filters?.goal_types?.length ? t`Goal types: ${filters.goal_types.join(', ')}` : null,
    filters?.event_names?.length ? t`Event names: ${filters.event_names.join(', ')}` : null
  ].filter((part): part is string => part !== null)

  const stats = statsQuery.data
  const attempts = stats ? stats.delivered + stats.failed : 0

  const renderBody = () => {
    if (!webhook) {
      if (subscriptionQuery.isError && !isNotFound(subscriptionQuery.error)) {
        return (
          <Result
            status="error"
            title={t`Couldn't load this webhook`}
            subTitle={
              subscriptionQuery.error instanceof Error ? subscriptionQuery.error.message : undefined
            }
            extra={<Button onClick={() => subscriptionQuery.refetch()}>{t`Retry`}</Button>}
          />
        )
      }
      return <Skeleton active paragraph={{ rows: 6 }} />
    }
    return (
      <div className="flex flex-col gap-4">
        {autoDisabled ? (
          <Alert
            type="error"
            showIcon
            title={t`Notifuse disabled this webhook automatically`}
            description={
              <>
                <div>{webhook.disabled_reason}</div>
                {consecutiveFailures > 0 && (
                  <div className="mt-1">{t`Consecutive delivery failures: ${consecutiveFailures}`}</div>
                )}
                <div className="mt-1">
                  {isZapier
                    ? t`Fix the Zap in Zapier, then switch this webhook back on to resume deliveries.`
                    : t`Fix the endpoint, then switch this webhook back on to resume deliveries.`}
                </div>
              </>
            }
          />
        ) : null}

        {isZapier ? (
          <Alert
            type="info"
            showIcon
            title={t`Created by Zapier`}
            description={
              <>
                <div>{t`This webhook belongs to a Zap. Turn that Zap off in Zapier and Zapier removes the webhook for you.`}</div>
                <div className="mt-1">{t`Test deliveries are unavailable here: Zapier's hook URLs always answer with success, so a test would report a green tick even for a Zap that no longer exists.`}</div>
              </>
            }
          />
        ) : null}

        <KeyValueList
          labelWidth={120}
          items={[
            { label: t`Endpoint`, value: <Mono copy>{webhook.url}</Mono> },
            {
              label: t`Events`,
              value: (
                <div className="flex flex-wrap gap-1">
                  {(webhook.settings.event_types || []).map((type) => (
                    <Mono key={type}>{type}</Mono>
                  ))}
                </div>
              )
            },
            ...(filterParts.length > 0
              ? [
                  {
                    label: t`Custom event filters`,
                    value: (
                      <div className="flex flex-col">
                        {filterParts.map((part) => (
                          <span key={part}>{part}</span>
                        ))}
                      </div>
                    )
                  }
                ]
              : []),
            { label: t`Secret`, value: secretValue },
            { label: t`Created`, value: relativeDate(webhook.created_at) },
            {
              label: t`Last success`,
              value: webhook.last_delivery_at ? (
                relativeDate(webhook.last_delivery_at)
              ) : (
                <span className="text-ink-3">{t`No successful delivery`}</span>
              )
            }
          ]}
        />

        <section className="flex flex-col gap-2">
          <div className="flex items-center justify-between gap-3">
            <h3 data-unreset className="m-0 text-base font-semibold text-ink">
              {t`Deliveries`}
            </h3>
            <Segmented
              size="small"
              value={timeRange}
              onChange={(value) => setTimeRange(value as TimeRange)}
              options={[
                { label: t`24 hours`, value: '1D' },
                { label: t`7 days`, value: '7D' }
              ]}
            />
          </div>
          <KpiStrip
            loading={statsQuery.isPending}
            items={[
              {
                key: 'delivered',
                label: t`Delivered`,
                value: stats ? int(stats.delivered) : '—'
              },
              {
                key: 'failed',
                label: t`Failed`,
                value: stats ? int(stats.failed) : '—'
              },
              {
                key: 'rate',
                label: t`Success rate`,
                value: stats && attempts > 0 ? percent(stats.delivered / attempts) : '—'
              }
            ]}
          />
        </section>
      </div>
    )
  }

  return (
    <SidePanel
      open={Boolean(webhookId)}
      onClose={onClose}
      title={webhook?.name ?? ''}
      status={status}
      actions={toggle}
      nav={nav}
      footer={footer}
    >
      {renderBody()}

      {/* Deleting a Zapier subscription breaks its Zap silently — Zapier keeps
          the Zap on and simply never fires it — so it gets its own dialog. */}
      <Modal
        title={t`Delete this Zapier webhook?`}
        open={zapierDeleteVisible}
        onCancel={() => setZapierDeleteVisible(false)}
        onOk={confirmZapierDelete}
        okText={t`Delete anyway`}
        okButtonProps={{ danger: true }}
        cancelText={t`Cancel`}
      >
        <p className="mb-3">
          {t`Deleting this webhook breaks the Zap that created it. The Zap stays switched on in Zapier and reports no error — it simply stops receiving events.`}
        </p>
        <p>
          {t`To stop these events, turn the Zap off in Zapier instead. Zapier then removes this webhook for you.`}
        </p>
      </Modal>

      <Modal
        title={t`Test Webhook`}
        open={testModalVisible}
        onCancel={() => setTestModalVisible(false)}
        footer={
          testResult ? (
            <Button onClick={() => setTestModalVisible(false)}>{t`Close`}</Button>
          ) : (
            <Space>
              <Button onClick={() => setTestModalVisible(false)}>{t`Cancel`}</Button>
              <Button
                type="primary"
                onClick={handleTest}
                loading={testLoading}
                disabled={!selectedEventType}
              >
                {t`Send Test`}
              </Button>
            </Space>
          )
        }
      >
        {testResult ? (
          <div className="max-h-80 overflow-auto rounded-lg border border-line bg-subtle p-4 font-mono text-sm text-ink">
            <div>
              <span className="text-ink-3">HTTP/1.1</span>{' '}
              <span className={testResult.success ? 'text-success-fg' : 'text-danger-fg'}>
                {testResult.statusCode || 0}
              </span>
            </div>
            {testResult.error && (
              <>
                <div className="mt-3 text-ink-3">{t`Error:`}</div>
                <div className="text-danger-fg">{testResult.error}</div>
              </>
            )}
            {testResult.responseBody && (
              <>
                <div className="mt-3 text-ink-3">{t`Body:`}</div>
                <div className="break-all whitespace-pre-wrap text-ink-2">
                  {testResult.responseBody}
                </div>
              </>
            )}
          </div>
        ) : testLoading ? (
          <div className="py-8 text-center text-ink-3">{t`Sending test webhook...`}</div>
        ) : (
          <div className="py-4">
            <label className="mb-2 block text-sm text-ink-2">{t`Select event type to test:`}</label>
            <Select
              value={selectedEventType}
              onChange={setSelectedEventType}
              style={{ width: '100%' }}
              placeholder={t`Select an event type`}
              options={(webhook?.settings.event_types || []).map((type) => ({
                value: type,
                label: type
              }))}
            />
          </div>
        )}
      </Modal>
    </SidePanel>
  )
}
