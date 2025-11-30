import { useState, useEffect } from 'react'
import {
  Card,
  Button,
  Table,
  Tag,
  Space,
  Modal,
  Form,
  Input,
  Switch,
  Select,
  message,
  Tooltip,
  Popconfirm,
  Drawer,
  Descriptions,
  Alert
} from 'antd'
import {
  faPlus,
  faCheck,
  faTimes,
  faExclamationTriangle,
  faRefresh
} from '@fortawesome/free-solid-svg-icons'
import { faPenToSquare, faTrashCan, faCopy, faEye } from '@fortawesome/free-regular-svg-icons'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { SettingsSectionHeader } from './SettingsSectionHeader'
import {
  webhookSubscriptionApi,
  WebhookSubscription,
  WebhookDelivery,
  CustomEventFilters
} from '../../services/api/webhook_subscription'

interface WebhooksSettingsProps {
  workspaceId: string
}

export function WebhooksSettings({ workspaceId }: WebhooksSettingsProps) {
  const [subscriptions, setSubscriptions] = useState<WebhookSubscription[]>([])
  const [eventTypes, setEventTypes] = useState<string[]>([])
  const [loading, setLoading] = useState(true)
  const [drawerVisible, setDrawerVisible] = useState(false)
  const [editingSubscription, setEditingSubscription] = useState<WebhookSubscription | null>(null)
  const [form] = Form.useForm()
  const [saving, setSaving] = useState(false)
  const [testingId, setTestingId] = useState<string | null>(null)
  const [testResult, setTestResult] = useState<{
    success: boolean
    statusCode: number
    responseBody: string
    error?: string
  } | null>(null)
  const [testModalVisible, setTestModalVisible] = useState(false)

  // Deliveries drawer state
  const [deliveriesDrawerVisible, setDeliveriesDrawerVisible] = useState(false)
  const [selectedSubscription, setSelectedSubscription] = useState<WebhookSubscription | null>(null)
  const [deliveries, setDeliveries] = useState<WebhookDelivery[]>([])
  const [deliveriesLoading, setDeliveriesLoading] = useState(false)
  const [deliveriesTotal, setDeliveriesTotal] = useState(0)
  const [deliveriesPage, setDeliveriesPage] = useState(1)
  const [deliveriesPageSize] = useState(10)

  // Secret visibility state
  const [visibleSecrets, setVisibleSecrets] = useState<Set<string>>(new Set())

  useEffect(() => {
    fetchSubscriptions()
    fetchEventTypes()
  }, [workspaceId])

  const fetchSubscriptions = async () => {
    try {
      setLoading(true)
      const response = await webhookSubscriptionApi.list(workspaceId)
      setSubscriptions(response.subscriptions || [])
    } catch (error) {
      console.error('Failed to fetch webhook subscriptions:', error)
      message.error('Failed to load webhook subscriptions')
    } finally {
      setLoading(false)
    }
  }

  const fetchEventTypes = async () => {
    try {
      const response = await webhookSubscriptionApi.getEventTypes()
      setEventTypes(response.event_types || [])
    } catch (error) {
      console.error('Failed to fetch event types:', error)
    }
  }

  const handleCreate = () => {
    setEditingSubscription(null)
    form.resetFields()
    form.setFieldsValue({
      enabled: true,
      event_types: []
    })
    setDrawerVisible(true)
  }

  const handleEdit = (subscription: WebhookSubscription) => {
    setEditingSubscription(subscription)
    form.setFieldsValue({
      name: subscription.name,
      url: subscription.url,
      description: subscription.description,
      event_types: subscription.event_types,
      enabled: subscription.enabled,
      custom_event_filters: subscription.custom_event_filters
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
        if (values.custom_event_goal_type || values.custom_event_names?.length) {
          customEventFilters = {
            goal_type: values.custom_event_goal_type || undefined,
            event_names: values.custom_event_names?.length ? values.custom_event_names : undefined
          }
        }
      }

      if (editingSubscription) {
        await webhookSubscriptionApi.update({
          workspace_id: workspaceId,
          id: editingSubscription.id,
          name: values.name,
          url: values.url,
          description: values.description || '',
          event_types: values.event_types,
          custom_event_filters: customEventFilters,
          enabled: values.enabled
        })
        message.success('Webhook subscription updated')
      } else {
        await webhookSubscriptionApi.create({
          workspace_id: workspaceId,
          name: values.name,
          url: values.url,
          description: values.description || '',
          event_types: values.event_types,
          custom_event_filters: customEventFilters
        })
        message.success('Webhook subscription created')
      }

      setDrawerVisible(false)
      fetchSubscriptions()
    } catch (error) {
      console.error('Failed to save webhook subscription:', error)
      message.error('Failed to save webhook subscription')
    } finally {
      setSaving(false)
    }
  }

  const handleDelete = async (id: string) => {
    try {
      await webhookSubscriptionApi.delete(workspaceId, id)
      message.success('Webhook subscription deleted')
      fetchSubscriptions()
    } catch (error) {
      console.error('Failed to delete webhook subscription:', error)
      message.error('Failed to delete webhook subscription')
    }
  }

  const handleToggle = async (id: string, enabled: boolean) => {
    try {
      await webhookSubscriptionApi.toggle({
        workspace_id: workspaceId,
        id,
        enabled
      })
      message.success(`Webhook ${enabled ? 'enabled' : 'disabled'}`)
      fetchSubscriptions()
    } catch (error) {
      console.error('Failed to toggle webhook subscription:', error)
      message.error('Failed to toggle webhook subscription')
    }
  }

  const handleTest = async (id: string) => {
    try {
      setTestingId(id)
      const result = await webhookSubscriptionApi.test(workspaceId, id)
      setTestResult({
        success: result.success,
        statusCode: result.status_code,
        responseBody: result.response_body,
        error: result.error
      })
      setTestModalVisible(true)
    } catch (error) {
      console.error('Failed to test webhook:', error)
      message.error('Failed to send test webhook')
    } finally {
      setTestingId(null)
    }
  }

  const handleRegenerateSecret = async (id: string) => {
    try {
      await webhookSubscriptionApi.regenerateSecret(workspaceId, id)
      message.success('Webhook secret regenerated')
      fetchSubscriptions()
    } catch (error) {
      console.error('Failed to regenerate secret:', error)
      message.error('Failed to regenerate webhook secret')
    }
  }

  const handleViewDeliveries = async (subscription: WebhookSubscription) => {
    setSelectedSubscription(subscription)
    setDeliveriesPage(1)
    setDeliveriesDrawerVisible(true)
    await fetchDeliveries(subscription.id, 1)
  }

  const fetchDeliveries = async (subscriptionId: string, page: number) => {
    try {
      setDeliveriesLoading(true)
      const offset = (page - 1) * deliveriesPageSize
      const response = await webhookSubscriptionApi.getDeliveries(
        workspaceId,
        subscriptionId,
        deliveriesPageSize,
        offset
      )
      setDeliveries(response.deliveries || [])
      setDeliveriesTotal(response.total)
    } catch (error) {
      console.error('Failed to fetch deliveries:', error)
      message.error('Failed to load delivery history')
    } finally {
      setDeliveriesLoading(false)
    }
  }

  const toggleSecretVisibility = (id: string) => {
    const newVisibleSecrets = new Set(visibleSecrets)
    if (newVisibleSecrets.has(id)) {
      newVisibleSecrets.delete(id)
    } else {
      newVisibleSecrets.add(id)
    }
    setVisibleSecrets(newVisibleSecrets)
  }

  const copyToClipboard = (text: string, label: string) => {
    navigator.clipboard.writeText(text)
    message.success(`${label} copied to clipboard`)
  }

  const formatEventType = (eventType: string) => {
    return eventType
      .split('_')
      .map((word) => word.charAt(0).toUpperCase() + word.slice(1))
      .join(' ')
  }

  const getStatusTag = (status: string) => {
    switch (status) {
      case 'delivered':
        return (
          <Tag color="green">
            <FontAwesomeIcon icon={faCheck} className="mr-1" /> Delivered
          </Tag>
        )
      case 'pending':
        return (
          <Tag color="blue">
            <FontAwesomeIcon icon={faRefresh} className="mr-1" /> Pending
          </Tag>
        )
      case 'failed':
        return (
          <Tag color="red">
            <FontAwesomeIcon icon={faTimes} className="mr-1" /> Failed
          </Tag>
        )
      default:
        return <Tag>{status}</Tag>
    }
  }

  const columns = [
    {
      title: 'Name',
      dataIndex: 'name',
      key: 'name',
      render: (text: string, record: WebhookSubscription) => (
        <div>
          <div className="font-medium">{text}</div>
          {record.description && (
            <div className="text-xs text-gray-500">{record.description}</div>
          )}
        </div>
      )
    },
    {
      title: 'URL',
      dataIndex: 'url',
      key: 'url',
      render: (text: string) => (
        <Tooltip title={text}>
          <span className="text-xs font-mono truncate block max-w-[200px]">{text}</span>
        </Tooltip>
      )
    },
    {
      title: 'Events',
      dataIndex: 'event_types',
      key: 'event_types',
      render: (types: string[]) => (
        <div>
          {types.slice(0, 2).map((type) => (
            <Tag key={type} className="mb-1">
              {formatEventType(type)}
            </Tag>
          ))}
          {types.length > 2 && <Tag>+{types.length - 2} more</Tag>}
        </div>
      )
    },
    {
      title: 'Status',
      key: 'status',
      render: (_: unknown, record: WebhookSubscription) => (
        <Space direction="vertical" size="small">
          <Switch
            checked={record.enabled}
            onChange={(checked) => handleToggle(record.id, checked)}
            checkedChildren="On"
            unCheckedChildren="Off"
          />
          {record.failure_count > 0 && (
            <Tooltip title={`${record.failure_count} failed deliveries`}>
              <Tag color="orange">
                <FontAwesomeIcon icon={faExclamationTriangle} className="mr-1" />
                {record.failure_count} failed
              </Tag>
            </Tooltip>
          )}
        </Space>
      )
    },
    {
      title: 'Stats',
      key: 'stats',
      render: (_: unknown, record: WebhookSubscription) => (
        <div className="text-xs">
          <div className="text-green-600">{record.success_count} delivered</div>
          <div className="text-red-500">{record.failure_count} failed</div>
        </div>
      )
    },
    {
      title: 'Actions',
      key: 'actions',
      render: (_: unknown, record: WebhookSubscription) => (
        <Space>
          <Tooltip title="Edit">
            <Button type="text" size="small" onClick={() => handleEdit(record)}>
              <FontAwesomeIcon icon={faPenToSquare} />
            </Button>
          </Tooltip>
          <Tooltip title="View Deliveries">
            <Button type="text" size="small" onClick={() => handleViewDeliveries(record)}>
              <FontAwesomeIcon icon={faEye} />
            </Button>
          </Tooltip>
          <Tooltip title="Test Webhook">
            <Button
              type="text"
              size="small"
              onClick={() => handleTest(record.id)}
              loading={testingId === record.id}
            >
              Send Test
            </Button>
          </Tooltip>
          <Popconfirm
            title="Delete this webhook?"
            description="This action cannot be undone."
            onConfirm={() => handleDelete(record.id)}
            okText="Yes"
            cancelText="No"
          >
            <Tooltip title="Delete">
              <Button type="text" size="small" danger>
                <FontAwesomeIcon icon={faTrashCan} />
              </Button>
            </Tooltip>
          </Popconfirm>
        </Space>
      )
    }
  ]

  const deliveryColumns = [
    {
      title: 'Event',
      dataIndex: 'event_type',
      key: 'event_type',
      render: (type: string) => <Tag>{formatEventType(type)}</Tag>
    },
    {
      title: 'Status',
      dataIndex: 'status',
      key: 'status',
      render: (status: string) => getStatusTag(status)
    },
    {
      title: 'Attempts',
      key: 'attempts',
      render: (_: unknown, record: WebhookDelivery) => (
        <span>
          {record.attempts}/{record.max_attempts}
        </span>
      )
    },
    {
      title: 'Response',
      key: 'response',
      render: (_: unknown, record: WebhookDelivery) => (
        <div className="text-xs">
          {record.last_response_status && (
            <Tag color={record.last_response_status >= 200 && record.last_response_status < 300 ? 'green' : 'red'}>
              HTTP {record.last_response_status}
            </Tag>
          )}
          {record.last_error && (
            <Tooltip title={record.last_error}>
              <span className="text-red-500 truncate block max-w-[150px]">{record.last_error}</span>
            </Tooltip>
          )}
        </div>
      )
    },
    {
      title: 'Created',
      dataIndex: 'created_at',
      key: 'created_at',
      render: (date: string) => new Date(date).toLocaleString()
    }
  ]

  const selectedEventTypes = Form.useWatch('event_types', form)
  const showCustomEventFilters = selectedEventTypes?.some((t: string) => t.startsWith('custom_event.'))

  return (
    <>
      <SettingsSectionHeader
        title="Webhooks"
        description="Configure outgoing webhooks to receive real-time notifications when events occur in your workspace."
      />

      <div className="mb-4 text-right">
        <Button type="primary" onClick={handleCreate}>
          <FontAwesomeIcon icon={faPlus} className="mr-2" />
          Add Webhook
        </Button>
      </div>

      <Card>
        <Table
          dataSource={subscriptions}
          columns={columns}
          rowKey="id"
          loading={loading}
          pagination={false}
          locale={{ emptyText: 'No webhook subscriptions configured' }}
        />
      </Card>

      {/* Create/Edit Drawer */}
      <Drawer
        title={editingSubscription ? 'Edit Webhook' : 'Create Webhook'}
        width={500}
        open={drawerVisible}
        onClose={() => setDrawerVisible(false)}
        footer={
          <div className="text-right">
            <Space>
              <Button onClick={() => setDrawerVisible(false)}>Cancel</Button>
              <Button type="primary" onClick={handleSave} loading={saving}>
                Save
              </Button>
            </Space>
          </div>
        }
      >
        <Form form={form} layout="vertical">
          <Form.Item
            name="name"
            label="Name"
            rules={[{ required: true, message: 'Please enter a name' }]}
          >
            <Input placeholder="My Webhook" />
          </Form.Item>

          <Form.Item
            name="url"
            label="Endpoint URL"
            rules={[
              { required: true, message: 'Please enter a URL' },
              { type: 'url', message: 'Please enter a valid URL' }
            ]}
          >
            <Input placeholder="https://example.com/webhook" />
          </Form.Item>

          <Form.Item name="description" label="Description">
            <Input.TextArea rows={2} placeholder="Optional description" />
          </Form.Item>

          <Form.Item
            name="event_types"
            label="Event Types"
            rules={[{ required: true, message: 'Please select at least one event type' }]}
          >
            <Select
              mode="multiple"
              placeholder="Select events to receive"
              options={eventTypes.map((type) => ({
                label: formatEventType(type),
                value: type
              }))}
            />
          </Form.Item>

          {showCustomEventFilters && (
            <>
              <Alert
                message="Custom Event Filters"
                description="Optionally filter which custom events trigger this webhook."
                type="info"
                showIcon
                className="mb-4"
              />
              <Form.Item name="custom_event_goal_type" label="Goal Type (optional)">
                <Input placeholder="e.g., conversion, engagement" />
              </Form.Item>
              <Form.Item name="custom_event_names" label="Event Names (optional)">
                <Select
                  mode="tags"
                  placeholder="Enter event names to filter"
                  tokenSeparators={[',']}
                />
              </Form.Item>
            </>
          )}

          {editingSubscription && (
            <>
              <Form.Item name="enabled" label="Enabled" valuePropName="checked">
                <Switch />
              </Form.Item>

              <Form.Item label="Signing Secret">
                <Input.Group compact>
                  <Input
                    style={{ width: 'calc(100% - 120px)' }}
                    value={
                      visibleSecrets.has(editingSubscription.id)
                        ? editingSubscription.secret
                        : '••••••••••••••••••••••••'
                    }
                    readOnly
                  />
                  <Tooltip title={visibleSecrets.has(editingSubscription.id) ? 'Hide' : 'Show'}>
                    <Button
                      onClick={() => toggleSecretVisibility(editingSubscription.id)}
                    >
                      <FontAwesomeIcon icon={faEye} />
                    </Button>
                  </Tooltip>
                  <Tooltip title="Copy">
                    <Button
                      onClick={() => copyToClipboard(editingSubscription.secret, 'Secret')}
                    >
                      <FontAwesomeIcon icon={faCopy} />
                    </Button>
                  </Tooltip>
                </Input.Group>
                <div className="mt-2">
                  <Popconfirm
                    title="Regenerate secret?"
                    description="This will invalidate the current secret. You'll need to update your webhook receiver."
                    onConfirm={() => handleRegenerateSecret(editingSubscription.id)}
                    okText="Yes"
                    cancelText="No"
                  >
                    <Button size="small" type="link">
                      Regenerate Secret
                    </Button>
                  </Popconfirm>
                </div>
              </Form.Item>
            </>
          )}
        </Form>

        {editingSubscription && (
          <Alert
            message="Webhook Signature"
            description={
              <div className="text-xs">
                <p>Webhooks are signed using HMAC-SHA256 per the Standard Webhooks specification.</p>
                <p className="mt-2">Headers sent:</p>
                <ul className="list-disc ml-4 mt-1">
                  <li><code>webhook-id</code>: Unique delivery ID</li>
                  <li><code>webhook-timestamp</code>: Unix timestamp</li>
                  <li><code>webhook-signature</code>: v1,{'{base64-signature}'}</li>
                </ul>
              </div>
            }
            type="info"
            showIcon
            className="mt-4"
          />
        )}
      </Drawer>

      {/* Test Result Modal */}
      <Modal
        title="Test Webhook Result"
        open={testModalVisible}
        onCancel={() => setTestModalVisible(false)}
        footer={
          <Button onClick={() => setTestModalVisible(false)}>Close</Button>
        }
      >
        {testResult && (
          <Descriptions bordered column={1} size="small">
            <Descriptions.Item label="Status">
              {testResult.success ? (
                <Tag color="green">
                  <FontAwesomeIcon icon={faCheck} className="mr-1" /> Success
                </Tag>
              ) : (
                <Tag color="red">
                  <FontAwesomeIcon icon={faTimes} className="mr-1" /> Failed
                </Tag>
              )}
            </Descriptions.Item>
            <Descriptions.Item label="HTTP Status">
              <Tag color={testResult.statusCode >= 200 && testResult.statusCode < 300 ? 'green' : 'red'}>
                {testResult.statusCode || 'N/A'}
              </Tag>
            </Descriptions.Item>
            {testResult.error && (
              <Descriptions.Item label="Error">
                <span className="text-red-500">{testResult.error}</span>
              </Descriptions.Item>
            )}
            {testResult.responseBody && (
              <Descriptions.Item label="Response Body">
                <pre className="text-xs bg-gray-100 p-2 rounded overflow-auto max-h-40">
                  {testResult.responseBody}
                </pre>
              </Descriptions.Item>
            )}
          </Descriptions>
        )}
      </Modal>

      {/* Deliveries Drawer */}
      <Drawer
        title={`Delivery History - ${selectedSubscription?.name || ''}`}
        width={700}
        open={deliveriesDrawerVisible}
        onClose={() => setDeliveriesDrawerVisible(false)}
      >
        <Table
          dataSource={deliveries}
          columns={deliveryColumns}
          rowKey="id"
          loading={deliveriesLoading}
          pagination={{
            current: deliveriesPage,
            pageSize: deliveriesPageSize,
            total: deliveriesTotal,
            onChange: (page) => {
              setDeliveriesPage(page)
              if (selectedSubscription) {
                fetchDeliveries(selectedSubscription.id, page)
              }
            }
          }}
          locale={{ emptyText: 'No deliveries yet' }}
        />
      </Drawer>
    </>
  )
}
