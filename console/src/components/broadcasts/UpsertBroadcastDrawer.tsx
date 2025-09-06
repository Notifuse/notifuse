import { useState, useEffect } from 'react'
import {
  Button,
  Drawer,
  Form,
  Input,
  Select,
  Space,
  App,
  Row,
  Col,
  Switch,
  InputNumber,
  Popconfirm,
  Alert
} from 'antd'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import {
  broadcastApi,
  Broadcast,
  CreateBroadcastRequest,
  UpdateBroadcastRequest
} from '../../services/api/broadcast'
import type { Workspace } from '../../services/api/types'
import TemplateSelectorInput from '../templates/TemplateSelectorInput'
import { DeleteOutlined } from '@ant-design/icons'
import React from 'react'
import extractTLD from '../utils/tld'

// Custom component to handle A/B testing configuration
const ABTestingConfig = ({ form, trackingEnabled }: { form: any; trackingEnabled: boolean }) => {
  const autoSendWinner = Form.useWatch(['test_settings', 'auto_send_winner'], form)

  if (!autoSendWinner) return null

  return (
    <Row gutter={24}>
      <Col span={12}>
        <Form.Item
          name={['test_settings', 'auto_send_winner_metric']}
          label="Winning metric"
          rules={[{ required: true }]}
        >
          <Select
            options={[
              { value: 'open_rate', label: 'Open Rate' },
              { value: 'click_rate', label: 'Click Rate' }
            ]}
          />
        </Form.Item>
      </Col>
      <Col span={12}>
        <Form.Item
          name={['test_settings', 'test_duration_hours']}
          label="Test duration (hours)"
          rules={[{ required: true }]}
        >
          <InputNumber min={1} />
        </Form.Item>
      </Col>
    </Row>
  )
}

interface UpsertBroadcastDrawerProps {
  workspace: Workspace
  broadcast?: Broadcast
  buttonProps?: any
  buttonContent?: React.ReactNode
  onClose?: () => void
  lists?: { id: string; name: string }[]
}

export function UpsertBroadcastDrawer({
  workspace,
  broadcast,
  buttonProps = {},
  buttonContent,
  onClose,
  lists = []
}: UpsertBroadcastDrawerProps) {
  const [isOpen, setIsOpen] = useState(false)
  const [form] = Form.useForm()
  const queryClient = useQueryClient()
  const [loading, setLoading] = useState(false)
  const { message, modal } = App.useApp()
  const [formTouched, setFormTouched] = useState(false)

  // Watch campaign name changes using Form.useWatch
  const campaignName = Form.useWatch('name', form)
  const abTestingEnabled = Form.useWatch(['test_settings', 'enabled'], form)
  const rateLimitPerMinute = Form.useWatch(['audience', 'rate_limit_per_minute'], form)

  // Enable tracking when A/B testing is enabled
  useEffect(() => {
    if (abTestingEnabled) {
      form.setFieldValue('tracking_enabled', true)
    }
  }, [abTestingEnabled, form])

  // Update utm_campaign when campaign name changes
  useEffect(() => {
    if (campaignName && isOpen) {
      // Convert to snake_case: lowercase, replace spaces and special chars with underscore
      const snakeCaseName = campaignName
        .toLowerCase()
        .replace(/[^\w\s]/g, '_') // Replace special characters with underscore
        .replace(/\s+/g, '_') // Replace spaces with underscore
        .replace(/_+/g, '_') // Replace multiple underscores with a single one

      // Set the utm_campaign value
      form.setFieldValue(['utm_parameters', 'campaign'], snakeCaseName)
    }
  }, [campaignName, form, isOpen])

  const upsertBroadcastMutation = useMutation({
    mutationFn: (values: CreateBroadcastRequest | UpdateBroadcastRequest) => {
      // Clone the values to avoid modifying the original
      const payload = { ...values }

      // Make sure schedule is set to not scheduled by default
      payload.schedule = {
        is_scheduled: false,
        use_recipient_timezone: false
      }

      // For logging or debugging
      // console.log('Submitting broadcast:', payload);

      if (broadcast) {
        return broadcastApi.update(payload as UpdateBroadcastRequest)
      } else {
        return broadcastApi.create(payload as CreateBroadcastRequest)
      }
    },
    onSuccess: () => {
      message.success(`Broadcast ${broadcast ? 'updated' : 'created'} successfully`)
      handleClose()
      queryClient.invalidateQueries({ queryKey: ['broadcasts', workspace.id] })
      setLoading(false)
    },
    onError: (error) => {
      message.error(`Failed to ${broadcast ? 'update' : 'create'} broadcast: ${error.message}`)
      setLoading(false)
    }
  })

  const showDrawer = () => {
    if (broadcast) {
      // For existing broadcasts, we need to ensure the schedule settings
      // match our form structure with the new fields
      form.setFieldsValue({
        id: broadcast.id,
        name: broadcast.name,
        audience: broadcast.audience,
        test_settings: broadcast.test_settings,
        utm_parameters: broadcast.utm_parameters || undefined,
        metadata: broadcast.metadata || undefined
      })
    } else {
      // Extract TLD from website URL
      const websiteTLD = extractTLD(workspace.settings.website_url || '')

      // Set default values for a new broadcast
      form.setFieldsValue({
        name: '',
        audience: {
          lists: [],
          segments: [],
          exclude_unsubscribed: true,
          skip_duplicate_emails: true
        },
        test_settings: {
          enabled: false,
          sample_percentage: 50,
          auto_send_winner: false,
          variations: [
            {
              id: 'default',
              name: 'Default',
              template_id: ''
            }
          ]
        },
        utm_parameters: {
          source: websiteTLD || undefined,
          medium: 'email'
        }
      })
    }
    setFormTouched(false)
    setIsOpen(true)
  }

  const handleClose = () => {
    if (formTouched && !loading && !upsertBroadcastMutation.isPending) {
      modal.confirm({
        title: 'Unsaved changes',
        content: 'You have unsaved changes. Are you sure you want to close this drawer?',
        okText: 'Yes',
        cancelText: 'No',
        onOk: () => {
          setIsOpen(false)
          form.resetFields()
          setFormTouched(false)
          if (onClose) {
            onClose()
          }
        }
      })
    } else {
      setIsOpen(false)
      form.resetFields()
      setFormTouched(false)
      if (onClose) {
        onClose()
      }
    }
  }

  const renderDrawerFooter = () => {
    return (
      <div className="text-right">
        <Space>
          <Button type="link" loading={loading} onClick={handleClose}>
            Cancel
          </Button>
          <Button
            loading={loading || upsertBroadcastMutation.isPending}
            onClick={() => {
              form.submit()
            }}
            type="primary"
          >
            Save
          </Button>
        </Space>
      </div>
    )
  }

  return (
    <>
      <Button type="primary" onClick={showDrawer} {...buttonProps}>
        {buttonContent || (broadcast ? 'Edit Broadcast' : 'Create Broadcast')}
      </Button>
      {isOpen && (
        <Drawer
          title={<>{broadcast ? 'Edit broadcast' : 'Create a broadcast'}</>}
          closable={true}
          keyboard={false}
          maskClosable={false}
          width={'95%'}
          open={isOpen}
          onClose={handleClose}
          className="drawer-no-transition drawer-body-no-padding"
          extra={renderDrawerFooter()}
        >
          <Form
            form={form}
            layout="vertical"
            onFinish={(values) => {
              setLoading(true)

              // Ensure workspace_id is included
              const payload = {
                ...values,
                workspace_id: workspace.id,
                // Set default schedule
                schedule: {
                  is_scheduled: false,
                  use_recipient_timezone: false
                }
              }

              // Add ID for updates
              if (broadcast) {
                payload.id = broadcast.id
              }

              upsertBroadcastMutation.mutate(payload)
            }}
            onFinishFailed={(info) => {
              if (info.errorFields) {
                message.error(`Please check the form for errors.`)
              }
              setLoading(false)
            }}
            onValuesChange={() => {
              setFormTouched(true)
            }}
          >
            <div className="p-8">
              <Row gutter={48}>
                {/* Left Column */}
                <Col span={12}>
                  <div className="text-xs mb-6 font-bold border-b border-solid pb-2 border-gray-400 text-gray-900">
                    Broadcast Settings
                  </div>

                  <Form.Item
                    name="name"
                    label="Broadcast name"
                    rules={[{ required: true, message: 'Please enter a broadcast name' }]}
                  >
                    <Input placeholder="E.g. Weekly Newsletter - May 2023" />
                  </Form.Item>

                  <div className="text-xs mt-8 mb-6 font-bold border-b border-solid pb-2 border-gray-400 text-gray-900">
                    Audience Selection
                  </div>

                  <Form.Item
                    name={['audience', 'lists']}
                    label="Lists"
                    extra="Select the contact lists to include in this broadcast"
                    rules={[
                      {
                        required: true,
                        type: 'array',
                        min: 1,
                        message: 'Please select at least one list'
                      }
                    ]}
                  >
                    <Select
                      mode="multiple"
                      placeholder="Select lists"
                      options={lists.map((list) => ({
                        value: list.id,
                        label: list.name
                      }))}
                    />
                  </Form.Item>

                  <div className="text-xs mt-12 mb-4 font-bold border-b border-solid pb-2 border-gray-400 text-gray-900">
                    Advanced Options
                  </div>

                  <Row gutter={24}>
                    <Col span={12}>
                      <Form.Item
                        name={['audience', 'exclude_unsubscribed']}
                        label="Exclude unsubscribed recipients"
                        valuePropName="checked"
                        initialValue={true}
                      >
                        <Switch />
                      </Form.Item>
                    </Col>
                    <Col span={12}>
                      <Form.Item
                        name={['audience', 'skip_duplicate_emails']}
                        label="Skip duplicate emails"
                        valuePropName="checked"
                        initialValue={true}
                      >
                        <Switch />
                      </Form.Item>
                    </Col>
                  </Row>

                  <Form.Item
                    name={['audience', 'rate_limit_per_minute']}
                    label="Rate limit (emails per minute)"
                  >
                    <InputNumber min={1} />
                  </Form.Item>

                  {rateLimitPerMinute && rateLimitPerMinute > 0 && (
                    <div className="text-xs text-gray-600 -mt-4 mb-4">
                      <div>≈ {(rateLimitPerMinute * 60).toLocaleString()} emails per hour</div>
                      <div>≈ {(rateLimitPerMinute * 60 * 24).toLocaleString()} emails per day</div>
                    </div>
                  )}

                  <div className="text-xs mt-12 mb-4 font-bold border-b border-solid border-gray-400 pb-2 text-gray-900">
                    URL Tracking Parameters
                  </div>
                  <Row gutter={24}>
                    <Col span={8}>
                      <Form.Item name={['utm_parameters', 'source']} label="utm_source">
                        <Input placeholder="Your website or company name" />
                      </Form.Item>
                    </Col>
                    <Col span={8}>
                      <Form.Item
                        name={['utm_parameters', 'medium']}
                        label="utm_medium"
                        initialValue="email"
                      >
                        <Input placeholder="email" />
                      </Form.Item>
                    </Col>
                    <Col span={8}>
                      <Form.Item name={['utm_parameters', 'campaign']} label="utm_campaign">
                        <Input />
                      </Form.Item>
                    </Col>
                  </Row>
                </Col>

                {/* Right Column */}
                <Col span={12}>
                  <div className="text-xs mb-6 font-bold border-b border-solid pb-2 border-gray-400 text-gray-900">
                    Template
                  </div>

                  {!workspace.settings?.email_tracking_enabled && (
                    <Alert
                      description="Tracking (opens & clicks) must be enabled in workspace settings to use A/B testing features."
                      type="info"
                      showIcon
                      className="!mb-4"
                    />
                  )}

                  <Form.Item
                    name={['test_settings', 'enabled']}
                    label="Enable A/B Testing"
                    valuePropName="checked"
                  >
                    <Switch disabled={!workspace.settings?.email_tracking_enabled} />
                  </Form.Item>

                  <Form.Item
                    noStyle
                    shouldUpdate={(prevValues, currentValues) => {
                      return (
                        prevValues.test_settings?.enabled !== currentValues.test_settings?.enabled
                      )
                    }}
                  >
                    {({ getFieldValue }) => {
                      const testEnabled = getFieldValue(['test_settings', 'enabled'])

                      if (testEnabled) {
                        return (
                          <>
                            <Row gutter={24}>
                              <Col span={12}>
                                <Form.Item
                                  name={['test_settings', 'sample_percentage']}
                                  label="Test sample size (%)"
                                  rules={[{ required: true }]}
                                >
                                  <InputNumber min={1} max={100} />
                                </Form.Item>
                              </Col>
                              <Col span={12}>
                                <Form.Item
                                  name={['test_settings', 'auto_send_winner']}
                                  label="Automatically send winner"
                                  valuePropName="checked"
                                  tooltip="Tracking (opens & clicks) should be enabled in your workspace settings to use this feature"
                                >
                                  <Switch disabled={!workspace.settings?.email_tracking_enabled} />
                                </Form.Item>
                              </Col>
                            </Row>

                            <ABTestingConfig
                              form={form}
                              trackingEnabled={workspace.settings?.email_tracking_enabled}
                            />

                            {/* Variations management will be added here */}
                            <div className="text-xs mt-4 mb-4 font-bold border-b border-solid pb-2 border-gray-400 text-gray-900">
                              Variations
                            </div>

                            <Form.List name={['test_settings', 'variations']}>
                              {(fields, { add, remove }) => (
                                <>
                                  {fields.map((field) => (
                                    <div key={field.key} className="border p-4 mb-4 rounded">
                                      <Row gutter={24}>
                                        <Col span={22}>
                                          <Form.Item
                                            key={`template-${field.key}`}
                                            name={[field.name, 'template_id']}
                                            label="Template"
                                            rules={[
                                              { required: true },
                                              ({ getFieldsValue }) => ({
                                                validator(_, value) {
                                                  if (!value) return Promise.resolve()

                                                  // Get all variations
                                                  const allVariations =
                                                    getFieldsValue()?.test_settings?.variations ||
                                                    []

                                                  // Check if this template is used in any other variation
                                                  const duplicates = allVariations.filter(
                                                    (v: any, i: number) =>
                                                      v?.template_id === value && i !== field.name
                                                  )

                                                  if (duplicates.length > 0) {
                                                    return Promise.reject(
                                                      new Error(
                                                        'This template is already used in another variation'
                                                      )
                                                    )
                                                  }

                                                  return Promise.resolve()
                                                }
                                              })
                                            ]}
                                          >
                                            <TemplateSelectorInput
                                              workspaceId={workspace.id}
                                              placeholder="Select template"
                                              category="marketing"
                                            />
                                          </Form.Item>
                                        </Col>
                                        {fields.length > 1 && (
                                          <Col span={2} className="flex items-end justify-end pb-2">
                                            <Form.Item label=" ">
                                              <Popconfirm
                                                title="Remove variation"
                                                description="Are you sure you want to remove this variation?"
                                                onConfirm={() => remove(field.name)}
                                                okText="Yes"
                                                cancelText="No"
                                              >
                                                <Button
                                                  type="text"
                                                  danger
                                                  icon={<DeleteOutlined />}
                                                />
                                              </Popconfirm>
                                            </Form.Item>
                                          </Col>
                                        )}
                                      </Row>
                                    </div>
                                  ))}

                                  {fields.length < 5 && (
                                    <Button
                                      type="primary"
                                      ghost
                                      onClick={() =>
                                        add({
                                          id: `variation-${fields.length + 1}`,
                                          template_id: ''
                                        })
                                      }
                                      block
                                    >
                                      + Add variation
                                    </Button>
                                  )}
                                </>
                              )}
                            </Form.List>
                          </>
                        )
                      }

                      // If A/B testing is disabled, show single template config
                      return (
                        <div>
                          <Form.Item
                            name={['test_settings', 'variations', 0, 'template_id']}
                            label="Template"
                            rules={[{ required: true }]}
                          >
                            <TemplateSelectorInput
                              workspaceId={workspace.id}
                              placeholder="Select template"
                              category="marketing"
                            />
                          </Form.Item>
                        </div>
                      )
                    }}
                  </Form.Item>
                </Col>
              </Row>
            </div>
          </Form>
        </Drawer>
      )}
    </>
  )
}
