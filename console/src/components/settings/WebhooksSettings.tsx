import { useState, useEffect } from 'react'
import {
  Alert,
  Card,
  Button,
  Space,
  Form,
  Input,
  Select,
  Checkbox,
  message,
  Drawer,
  Row,
  Col
} from 'antd'
import { faPlus } from '@fortawesome/free-solid-svg-icons'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { useLingui } from '@lingui/react/macro'
import { SettingsSectionHeader } from './SettingsSectionHeader'
import { WebhookCard } from './WebhookCard'
import Subtitle from '../common/subtitle'
import {
  webhookSubscriptionApi,
  WebhookSubscription,
  CustomEventFilters,
  UpdateWebhookSubscriptionRequest
} from '../../services/api/webhook_subscription'

interface WebhooksSettingsProps {
  workspaceId: string
}

export function WebhooksSettings({ workspaceId }: WebhooksSettingsProps) {
  const { t } = useLingui()
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
  // eslint-disable-next-line react-hooks/exhaustive-deps -- fetchSubscriptions is stable
  }, [workspaceId])

  const fetchSubscriptions = async () => {
    try {
      setLoading(true)
      const response = await webhookSubscriptionApi.list(workspaceId)
      setSubscriptions(response.subscriptions || [])
    } catch (error) {
      console.error('Failed to fetch webhook subscriptions:', error)
      message.error(t`Failed to load webhook subscriptions`)
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

  // Neither of these seeds `enabled`: the drawer registers no such Form.Item, so a value put in
  // the store here is never returned by validateFields and only makes the switch look like it
  // round-trips through the form. The drawer does not send the flag at all, and the switch
  // itself lives on the subscription card.
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
        // subscription somebody disabled from the card meanwhile — wiping its failure
        // counters on the way through. The switch on the card is the only place enabled
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
      fetchSubscriptions()
    } catch (error) {
      console.error('Failed to save webhook subscription:', error)
      message.error(t`Failed to save webhook subscription`)
    } finally {
      setSaving(false)
    }
  }

  const handleDelete = async (id: string) => {
    try {
      await webhookSubscriptionApi.delete(workspaceId, id)
      message.success(t`Webhook subscription deleted`)
      fetchSubscriptions()
    } catch (error) {
      console.error('Failed to delete webhook subscription:', error)
      message.error(t`Failed to delete webhook subscription`)
    }
  }

  const handleToggle = async (id: string, enabled: boolean) => {
    try {
      await webhookSubscriptionApi.toggle({
        workspace_id: workspaceId,
        id,
        enabled
      })
      message.success(enabled ? t`Webhook enabled` : t`Webhook disabled`)
      fetchSubscriptions()
    } catch (error) {
      console.error('Failed to toggle webhook subscription:', error)
      message.error(t`Failed to toggle webhook subscription`)
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

  return (
    <>
      <SettingsSectionHeader
        title={t`Webhooks`}
        description={t`Configure outgoing webhooks to receive real-time notifications when events occur in your workspace.`}
      />

      {subscriptions.length === 0 && !loading ? (
        <Card className="text-center py-8">
          <p className="text-gray-500 mb-4">{t`No webhook subscriptions configured`}</p>
          <Button type="primary" onClick={handleCreate}>
            <FontAwesomeIcon icon={faPlus} className="mr-2" />
            {t`Create Webhook`}
          </Button>
        </Card>
      ) : (
        <>
          <div className="mb-4 text-right">
            <Button type="primary" onClick={handleCreate}>
              <FontAwesomeIcon icon={faPlus} className="mr-2" />
              {t`Add Webhook`}
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
        title={editingSubscription ? t`Edit Webhook` : t`Create Webhook`}
        size={500}
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
            rules={[{ required: true, message: t`Please select at least one event type` }]}
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
