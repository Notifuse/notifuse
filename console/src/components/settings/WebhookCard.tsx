import { useState, useEffect } from 'react'
import {
  Alert,
  Card,
  Button,
  Tag,
  Space,
  Input,
  Switch,
  message,
  Tooltip,
  Popconfirm,
  Row,
  Col,
  Statistic,
  Divider,
  Segmented,
  Modal,
  Select
} from 'antd'
import { faRotate, faBarsStaggered } from '@fortawesome/free-solid-svg-icons'
import { faPenToSquare, faTrashCan, faCopy, faEye } from '@fortawesome/free-regular-svg-icons'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { useNavigate } from '@tanstack/react-router'
import { useLingui } from '@lingui/react/macro'
import { webhookSubscriptionApi, WebhookSubscription } from '../../services/api/webhook_subscription'
import { analyticsService } from '../../services/api/analytics'

interface WebhookCardProps {
  webhook: WebhookSubscription
  workspaceId: string
  onEdit: (webhook: WebhookSubscription) => void
  onDelete: (id: string) => void
  onToggle: (id: string, enabled: boolean) => void
  onRefresh: () => void
}

interface TestResult {
  success: boolean
  statusCode: number
  responseBody: string
  error?: string
}

type TimeRange = '1D' | '7D'

export function WebhookCard({
  webhook,
  workspaceId,
  onEdit,
  onDelete,
  onToggle,
  onRefresh
}: WebhookCardProps) {
  const { t } = useLingui()
  const navigate = useNavigate()
  const [visibleSecret, setVisibleSecret] = useState(false)
  const [timeRange, setTimeRange] = useState<TimeRange>('1D')
  const [stats, setStats] = useState<{ delivered: number; failed: number }>({
    delivered: 0,
    failed: 0
  })
  const [statsLoading, setStatsLoading] = useState(false)
  const [testModalVisible, setTestModalVisible] = useState(false)
  const [selectedEventType, setSelectedEventType] = useState<string>('')
  const [testLoading, setTestLoading] = useState(false)
  const [testResult, setTestResult] = useState<TestResult | null>(null)
  const [zapierDeleteVisible, setZapierDeleteVisible] = useState(false)

  const isZapier = webhook.source === 'zapier'
  // A subscription Notifuse switched off after sustained delivery failure carries
  // a reason; one the user switched off never does.
  const autoDisabled = !webhook.enabled && Boolean(webhook.disabled_reason)
  const consecutiveFailures = webhook.consecutive_failures ?? 0

  useEffect(() => {
    let cancelled = false

    const fetchStats = async () => {
      setStatsLoading(true)
      try {
        const now = new Date()
        const fromDate = new Date()
        if (timeRange === '1D') {
          fromDate.setDate(now.getDate() - 1)
        } else {
          fromDate.setDate(now.getDate() - 7)
        }

        const result = await analyticsService.query(
          {
            schema: 'webhook_deliveries',
            measures: ['count_delivered', 'count_failed'],
            dimensions: ['subscription_id'],
            filters: [
              {
                member: 'subscription_id',
                operator: 'equals',
                values: [webhook.id]
              },
              {
                member: 'created_at',
                operator: 'gte',
                values: [fromDate.toISOString()]
              }
            ]
          },
          workspaceId
        )

        if (!cancelled) {
          if (result.data.length > 0) {
            const row = result.data[0]
            setStats({
              delivered: (row.count_delivered as number) || 0,
              failed: (row.count_failed as number) || 0
            })
          } else {
            setStats({ delivered: 0, failed: 0 })
          }
        }
      } catch (error) {
        console.error('Failed to fetch webhook stats:', error)
      } finally {
        if (!cancelled) {
          setStatsLoading(false)
        }
      }
    }

    fetchStats()

    return () => {
      cancelled = true
    }
  }, [webhook.id, workspaceId, timeRange])

  const handleRegenerateSecret = async () => {
    try {
      await webhookSubscriptionApi.regenerateSecret(workspaceId, webhook.id)
      message.success(t`Webhook secret regenerated`)
      onRefresh()
    } catch (error) {
      console.error('Failed to regenerate secret:', error)
      message.error(t`Failed to regenerate webhook secret`)
    }
  }

  const copyToClipboard = (text: string, label: string) => {
    navigator.clipboard.writeText(text)
    message.success(t`${label} copied to clipboard`)
  }

  const formatEventType = (eventType: string) => {
    return eventType
  }

  const handleTest = async () => {
    if (!selectedEventType) return
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
    setSelectedEventType(webhook.settings.event_types?.[0] || '')
    setTestResult(null)
    setTestModalVisible(true)
  }

  const confirmZapierDelete = () => {
    setZapierDeleteVisible(false)
    onDelete(webhook.id)
  }

  return (
    <Card
      styles={{ body: { padding: 0 } }}
      title={
        <Space size="large">
          <span className="font-medium">{webhook.name}</span>
          {isZapier && (
            <Tooltip title={t`Zapier created this webhook for one of your Zaps.`}>
              <Tag variant="filled" color="orange">
                {t`Zapier`}
              </Tag>
            </Tooltip>
          )}
          {autoDisabled && (
            <Tag variant="filled" color="red">
              {t`Auto-disabled`}
            </Tag>
          )}
          {webhook.enabled ? (
            <Popconfirm
              title={t`Disable this webhook?`}
              description={t`The webhook will stop receiving events.`}
              onConfirm={() => onToggle(webhook.id, false)}
              okText={t`Yes`}
              cancelText={t`No`}
            >
              <Tooltip title={t`Disable webhook`}>
                <Switch checked={true} size="small" />
              </Tooltip>
            </Popconfirm>
          ) : (
            <Tooltip title={t`Enable webhook`}>
              <Switch
                checked={false}
                onChange={() => onToggle(webhook.id, true)}
                size="small"
              />
            </Tooltip>
          )}
        </Space>
      }
      extra={
        <Space>
          {/* Zapier's hook URLs answer 2xx unconditionally, so a test against a
              deleted Zap still reports success — a green tick that means nothing
              is worse than no button at all. */}
          {!isZapier && (
            <Tooltip title={t`Test Webhook`}>
              <Button type="text" size="small" onClick={openTestModal}>
                {t`Send Test`}
              </Button>
            </Tooltip>
          )}
          <Tooltip title={t`View Logs`}>
            <Button
              type="text"
              size="small"
              onClick={() =>
                navigate({
                  to: '/console/workspace/$workspaceId/logs',
                  params: { workspaceId },
                  search: { tab: 'outgoing-webhooks' }
                })
              }
            >
              <FontAwesomeIcon icon={faBarsStaggered} />
            </Button>
          </Tooltip>
          {isZapier ? (
            <Tooltip title={t`Delete`}>
              <Button type="text" size="small" onClick={() => setZapierDeleteVisible(true)}>
                <FontAwesomeIcon icon={faTrashCan} />
              </Button>
            </Tooltip>
          ) : (
            <Popconfirm
              title={t`Delete this webhook?`}
              description={t`This action cannot be undone.`}
              onConfirm={() => onDelete(webhook.id)}
              okText={t`Yes`}
              cancelText={t`No`}
            >
              <Tooltip title={t`Delete`}>
                <Button type="text" size="small">
                  <FontAwesomeIcon icon={faTrashCan} />
                </Button>
              </Tooltip>
            </Popconfirm>
          )}
          <Tooltip title={t`Edit`}>
            <Button type="text" size="small" onClick={() => onEdit(webhook)}>
              <FontAwesomeIcon icon={faPenToSquare} />
            </Button>
          </Tooltip>
        </Space>
      }
      className="mb-4"
    >
      <div className="p-4">
        {autoDisabled && (
          <Alert
            type="error"
            showIcon
            className="mb-4"
            title={t`Notifuse disabled this webhook automatically`}
            description={
              <>
                <div>{webhook.disabled_reason}</div>
                {consecutiveFailures > 0 && (
                  <div className="mt-1">
                    {t`Consecutive delivery failures: ${consecutiveFailures}`}
                  </div>
                )}
                <div className="mt-1">
                  {isZapier
                    ? t`Fix the Zap in Zapier, then switch this webhook back on to resume deliveries.`
                    : t`Fix the endpoint, then switch this webhook back on to resume deliveries.`}
                </div>
              </>
            }
          />
        )}

        {isZapier && (
          <Alert
            type="info"
            showIcon
            className="mb-4"
            title={t`Created by Zapier`}
            description={
              <>
                <div>{t`This webhook belongs to a Zap. Turn that Zap off in Zapier and Zapier removes the webhook for you.`}</div>
                <div className="mt-1">{t`Test deliveries are unavailable here: Zapier's hook URLs always answer with success, so a test would report a green tick even for a Zap that no longer exists.`}</div>
              </>
            }
          />
        )}

        <div className="flex items-center gap-2 mb-3">
          <span className="text-gray-500 text-sm">{t`URL:`}</span>
          <code className="text-sm bg-gray-100 px-2 py-1 rounded truncate flex-1">
            {webhook.url}
          </code>
        </div>
        <div className="flex items-center gap-2 mb-3">
          <span className="text-gray-500 text-sm">{t`Secret:`}</span>
          {!webhook.secret ? (
            <span className="text-gray-400 text-sm italic flex-1">
              {t`Only a workspace owner can view or rotate the signing secret`}
            </span>
          ) : (
          <Space.Compact className="flex-1">
            <Input
              size="small"
              value={visibleSecret ? webhook.secret : '••••••••••••••••••••••••'}
              readOnly
            />
            <Tooltip title={visibleSecret ? t`Hide` : t`Show`}>
              <Button size="small" onClick={() => setVisibleSecret(!visibleSecret)}>
                <FontAwesomeIcon icon={faEye} className="opacity-70" />
              </Button>
            </Tooltip>
            <Popconfirm
              title={t`Regenerate secret?`}
              description={t`This will invalidate the current secret. You'll need to update your webhook receiver.`}
              onConfirm={handleRegenerateSecret}
              okText={t`Yes`}
              cancelText={t`No`}
            >
              <Tooltip title={t`Regenerate`}>
                <Button size="small">
                  <FontAwesomeIcon icon={faRotate} className="opacity-70" />
                </Button>
              </Tooltip>
            </Popconfirm>
            <Tooltip title={t`Copy`}>
              <Button size="small" onClick={() => copyToClipboard(webhook.secret, t`Secret`)}>
                <FontAwesomeIcon icon={faCopy} className="opacity-70" />
              </Button>
            </Tooltip>
          </Space.Compact>
          )}
        </div>
        <Divider className="my-6!" />
        <div className="flex justify-between items-center mb-4">
          <span className="text-gray-500 text-sm">
            {timeRange === '1D' ? t`Last 24 hours` : t`Last 7 days`}
          </span>
          <Segmented
            size="small"
            options={['1D', '7D']}
            value={timeRange}
            onChange={(value) => setTimeRange(value as TimeRange)}
          />
        </div>
        <Row>
          <Col span={8} className="text-center">
            <Statistic title={t`Delivered`} value={stats.delivered} loading={statsLoading} />
          </Col>
          <Col span={8} className="text-center">
            <Statistic title={t`Failed`} value={stats.failed} loading={statsLoading} />
          </Col>
          <Col span={8} className="text-center">
            <Statistic
              title={t`Success Rate`}
              value={
                stats.delivered + stats.failed > 0
                  ? Math.round((stats.delivered / (stats.delivered + stats.failed)) * 100)
                  : 0
              }
              suffix="%"
              loading={statsLoading}
            />
          </Col>
        </Row>
        <Divider className="my-6!" />
        <div style={{ columnCount: 3, columnGap: '1rem' }}>
          {(webhook.settings.event_types || []).map((type) => (
            <div key={type} style={{ breakInside: 'avoid' }}>
              <Tag variant="filled" color="green" className="text-xs !mb-1">
                {formatEventType(type)}
              </Tag>
            </div>
          ))}
        </div>
      </div>

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

      {/* Test Webhook Modal */}
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
          <div className="bg-gray-900 text-gray-100 p-4 rounded-lg font-mono text-sm overflow-auto max-h-80">
            <div>
              <span className="text-purple-400">HTTP/1.1</span>{' '}
              <span className={testResult.success ? 'text-green-400' : 'text-red-400'}>
                {testResult.statusCode || 0}
              </span>
            </div>
            {testResult.error && (
              <>
                <div className="mt-3 text-gray-500">{t`Error:`}</div>
                <div className="text-red-400">{testResult.error}</div>
              </>
            )}
            {testResult.responseBody && (
              <>
                <div className="mt-3 text-gray-500">{t`Body:`}</div>
                <div className="text-gray-300 whitespace-pre-wrap break-all">
                  {testResult.responseBody}
                </div>
              </>
            )}
          </div>
        ) : testLoading ? (
          <div className="py-8 text-center text-gray-500">{t`Sending test webhook...`}</div>
        ) : (
          <div className="py-4">
            <label className="block text-sm text-gray-600 mb-2">
              {t`Select event type to test:`}
            </label>
            <Select
              value={selectedEventType}
              onChange={setSelectedEventType}
              style={{ width: '100%' }}
              placeholder={t`Select an event type`}
              options={(webhook.settings.event_types || []).map((type) => ({
                value: type,
                label: type
              }))}
            />
          </div>
        )}
      </Modal>
    </Card>
  )
}
