import { useState, useEffect } from 'react'
import { Card, Button, Space, Form, Input, Select, Checkbox, message, Drawer, Row, Col } from 'antd'
import { faPlus } from '@fortawesome/free-solid-svg-icons'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { SettingsSectionHeader } from './SettingsSectionHeader'
import { WebhookCard } from './WebhookCard'
import Subtitle from '../common/subtitle'
import {
  webhookSubscriptionApi,
  WebhookSubscription,
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
      event_types: subscription.settings.event_types,
      enabled: subscription.enabled,
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
        await webhookSubscriptionApi.update({
          workspace_id: workspaceId,
          id: editingSubscription.id,
          name: values.name,
          url: values.url,
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

  const formatEventType = (eventType: string) => {
    return eventType
  }

  const selectedEventTypes = Form.useWatch('event_types', form)
  const showCustomEventFilters = selectedEventTypes?.some((t: string) =>
    t.startsWith('custom_event.')
  )

  return (
    <>
      <SettingsSectionHeader
        title="Webhooks"
        description="Configure outgoing webhooks to receive real-time notifications when events occur in your workspace."
      />

      {subscriptions.length === 0 && !loading ? (
        <Card className="text-center py-8">
          <p className="text-gray-500 mb-4">No webhook subscriptions configured</p>
          <Button type="primary" onClick={handleCreate}>
            <FontAwesomeIcon icon={faPlus} className="mr-2" />
            Create Webhook
          </Button>
        </Card>
      ) : (
        <>
          <div className="mb-4 text-right">
            <Button type="primary" onClick={handleCreate}>
              <FontAwesomeIcon icon={faPlus} className="mr-2" />
              Add Webhook
            </Button>
          </div>

          {subscriptions.map((webhook) => (
            <WebhookCard
              key={webhook.id}
              webhook={webhook}
              workspaceId={workspaceId}
              onEdit={handleEdit}
              onDelete={handleDelete}
              onToggle={handleToggle}
              onRefresh={fetchSubscriptions}
            />
          ))}
        </>
      )}

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

          <Form.Item
            name="event_types"
            label={
              <div className="flex justify-between w-full">
                <span>Event Types</span>
                <a
                  onClick={(e) => {
                    e.preventDefault()
                    form.setFieldsValue({ event_types: eventTypes })
                  }}
                >
                  Select all
                </a>
              </div>
            }
            rules={[{ required: true, message: 'Please select at least one event type' }]}
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
                Custom Event Filters (optional)
              </Subtitle>
              <Form.Item name="custom_event_goal_types" label="Goal Types">
                <Select
                  mode="multiple"
                  placeholder="Select goal types to filter"
                  options={[
                    { value: 'purchase', label: 'Purchase' },
                    { value: 'subscription', label: 'Subscription' },
                    { value: 'lead', label: 'Lead' },
                    { value: 'signup', label: 'Signup' },
                    { value: 'booking', label: 'Booking' },
                    { value: 'trial', label: 'Trial' },
                    { value: 'other', label: 'Other' }
                  ]}
                />
              </Form.Item>
              <Form.Item name="custom_event_names" label="Event Names">
                <Select
                  mode="tags"
                  placeholder="Enter event names to filter"
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
