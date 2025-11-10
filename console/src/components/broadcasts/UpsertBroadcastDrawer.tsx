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
  Alert,
  Tag,
  Tabs,
  Tooltip
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
import { DeleteOutlined, InfoCircleOutlined } from '@ant-design/icons'
import React from 'react'
import extractTLD from '../../lib/tld'
import type { List } from '../../services/api/list'
import { SEOSettingsForm } from '../seo/SEOSettingsForm'

// Custom component to handle A/B testing configuration
const ABTestingConfig = ({ form }: { form: any }) => {
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
  lists?: List[]
  segments?: { id: string; name: string; color: string; users_count?: number }[]
}

export function UpsertBroadcastDrawer({
  workspace,
  broadcast,
  buttonProps = {},
  buttonContent,
  onClose,
  lists = [],
  segments = []
}: UpsertBroadcastDrawerProps) {
  const [isOpen, setIsOpen] = useState(false)
  const [form] = Form.useForm()
  const queryClient = useQueryClient()
  const [loading, setLoading] = useState(false)
  const { message, modal } = App.useApp()
  const [formTouched, setFormTouched] = useState(false)
  const [tab, setTab] = useState<string>('audience')

  // Watch campaign name changes using Form.useWatch
  const campaignName = Form.useWatch('name', form)
  const abTestingEnabled = Form.useWatch(['test_settings', 'enabled'], form)

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
        audience: {
          ...broadcast.audience
        },
        channels: broadcast.channels || { email: true, web: false },
        web_publication_settings: broadcast.web_publication_settings || undefined,
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
          list: undefined,
          segments: [],
          exclude_unsubscribed: true
        },
        channels: {
          email: true,
          web: false
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
    setTab('audience')
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
          setTab('audience')
          if (onClose) {
            onClose()
          }
        }
      })
    } else {
      setIsOpen(false)
      form.resetFields()
      setFormTouched(false)
      setTab('audience')
      if (onClose) {
        onClose()
      }
    }
  }

  const validateCurrentTab = async (currentTab: string): Promise<boolean> => {
    // Validate fields based on current tab before proceeding
    const fieldsToValidate: string[][] = []

    if (currentTab === 'audience') {
      fieldsToValidate.push(['name'], ['audience', 'list'])
    } else if (currentTab === 'email') {
      // Add email tab validation if needed in the future
    } else if (currentTab === 'web') {
      // Check if web channel is enabled
      const webEnabled = form.getFieldValue(['channels', 'web'])
      if (webEnabled) {
        fieldsToValidate.push(['web_publication_settings', 'slug'])
      }
    }

    try {
      // Validate the fields for the current tab
      if (fieldsToValidate.length > 0) {
        await form.validateFields(fieldsToValidate)
      }
      return true
    } catch (errorInfo) {
      // Validation failed - error messages will be shown automatically by form
      console.log('Validation failed:', errorInfo)
      return false
    }
  }

  const goNext = async () => {
    const isValid = await validateCurrentTab(tab)
    if (!isValid) return

    // If validation passes, proceed to next tab
    const tabOrder = ['audience', 'email', 'web', 'content']
    const currentIndex = tabOrder.indexOf(tab)
    if (currentIndex < tabOrder.length - 1) {
      setTab(tabOrder[currentIndex + 1])
    }
  }

  const handleTabChange = async (newTab: string) => {
    // Only validate if moving forward (not backward)
    const tabOrder = ['audience', 'email', 'web', 'content']
    const currentIndex = tabOrder.indexOf(tab)
    const newIndex = tabOrder.indexOf(newTab)

    if (newIndex > currentIndex) {
      // Moving forward - validate current tab
      const isValid = await validateCurrentTab(tab)
      if (!isValid) return // Stay on current tab if validation fails
    }

    // Validation passed or moving backward - allow tab change
    setTab(newTab)
  }

  const renderDrawerFooter = () => {
    return (
      <div className="text-right">
        <Space>
          <Button type="link" loading={loading} onClick={handleClose}>
            Cancel
          </Button>

          {tab === 'audience' && (
            <Button type="primary" onClick={goNext}>
              Next
            </Button>
          )}

          {tab === 'email' && (
            <>
              <Button type="primary" ghost onClick={() => handleTabChange('audience')}>
                Previous
              </Button>
              <Button type="primary" onClick={goNext}>
                Next
              </Button>
            </>
          )}

          {tab === 'web' && (
            <>
              <Button type="primary" ghost onClick={() => handleTabChange('email')}>
                Previous
              </Button>
              <Button type="primary" onClick={goNext}>
                Next
              </Button>
            </>
          )}

          {tab === 'content' && (
            <>
              <Button type="primary" ghost onClick={() => handleTabChange('web')}>
                Previous
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
            </>
          )}
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
          width="700px"
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

              // Normalize list to always be a string (single select)
              if (payload.audience?.list && Array.isArray(payload.audience.list)) {
                payload.audience.list = payload.audience.list[0]
              }

              upsertBroadcastMutation.mutate(payload)
            }}
            onFinishFailed={(info) => {
              if (info.errorFields && info.errorFields.length > 0) {
                // Get the first error field name
                const firstErrorField = info.errorFields[0].name[0]

                // Map fields to tabs and switch directly (no validation needed for error display)
                if (
                  firstErrorField === 'name' ||
                  (Array.isArray(info.errorFields[0].name) &&
                    info.errorFields[0].name[0] === 'audience')
                ) {
                  setTab('audience')
                } else if (
                  (Array.isArray(info.errorFields[0].name) &&
                    info.errorFields[0].name[0] === 'channels' &&
                    info.errorFields[0].name[1] === 'email') ||
                  info.errorFields[0].name[0] === 'utm_parameters'
                ) {
                  setTab('email')
                } else if (
                  (Array.isArray(info.errorFields[0].name) &&
                    info.errorFields[0].name[0] === 'channels' &&
                    info.errorFields[0].name[1] === 'web') ||
                  info.errorFields[0].name[0] === 'web_publication_settings'
                ) {
                  setTab('web')
                } else if (
                  Array.isArray(info.errorFields[0].name) &&
                  info.errorFields[0].name[0] === 'test_settings'
                ) {
                  setTab('content')
                }

                message.error(`Please check the form for errors.`)
              }
              setLoading(false)
            }}
            onValuesChange={() => {
              setFormTouched(true)
            }}
          >
            <div className="flex">
              <Tabs
                activeKey={tab}
                onChange={handleTabChange}
                tabPosition="left"
                className="vertical-tabs"
                style={{ minHeight: 'calc(100vh - 65px)' }}
                items={[
                  {
                    key: 'audience',
                    label: '1. Audience'
                  },
                  {
                    key: 'email',
                    label: '2. Email'
                  },
                  {
                    key: 'web',
                    label: '3. Web'
                  },
                  {
                    key: 'content',
                    label: '4. Content'
                  }
                ]}
              />
              <div className="flex-1 relative">
                <div style={{ display: tab === 'audience' ? 'block' : 'none' }}>
                  <div className="pt-8 pr-8">
                    <Form.Item
                      name="name"
                      label="Broadcast name"
                      rules={[{ required: true, message: 'Please enter a broadcast name' }]}
                    >
                      <Input placeholder="E.g. Weekly Newsletter - May 2023" />
                    </Form.Item>

                    <Form.Item
                      name={['audience', 'list']}
                      label="List"
                      rules={[
                        {
                          required: true,
                          type: 'string',
                          message: 'Please select a list'
                        }
                      ]}
                    >
                      <Select
                        placeholder="Select a list"
                        options={lists.map((list) => ({
                          value: list.id,
                          label: list.name
                        }))}
                      />
                    </Form.Item>

                    <Form.Item
                      name={['audience', 'segments']}
                      label={
                        <span>
                          Belonging to at least one of the following segments{' '}
                          <Tooltip
                            title="Optionally filter contacts by segments within the selected lists"
                            className="ml-1"
                          >
                            <InfoCircleOutlined style={{ color: '#999' }} />
                          </Tooltip>
                        </span>
                      }
                    >
                      <Select
                        mode="multiple"
                        placeholder="Select segments (optional)"
                        options={segments.map((segment) => ({
                          value: segment.id,
                          label: segment.name
                        }))}
                        optionRender={(option) => {
                          const segment = segments.find((s) => s.id === option.value)
                          if (!segment) return option.label

                          return (
                            <Tag color={segment.color} bordered={false}>
                              {segment.name}
                              {segment.users_count !== undefined && (
                                <span className="ml-1">
                                  ({segment.users_count.toLocaleString()})
                                </span>
                              )}
                            </Tag>
                          )
                        }}
                        tagRender={(props) => {
                          const segment = segments.find((s) => s.id === props.value)
                          if (!segment) return <Tag {...props}>{props.label}</Tag>

                          return (
                            <Tag
                              color={segment.color}
                              bordered={false}
                              closable={props.closable}
                              onClose={props.onClose}
                              style={{ marginRight: 3 }}
                            >
                              {segment.name}
                              {segment.users_count !== undefined && (
                                <span className="ml-1">
                                  ({segment.users_count.toLocaleString()})
                                </span>
                              )}
                            </Tag>
                          )
                        }}
                      />
                    </Form.Item>
                  </div>
                </div>

                <div style={{ display: tab === 'email' ? 'block' : 'none' }}>
                  <div className="pt-8 pr-8">
                    <Row gutter={24}>
                      <Col span={12}>
                        <Form.Item
                          name={['channels', 'email']}
                          label="Send email"
                          valuePropName="checked"
                          initialValue={true}
                        >
                          <Switch />
                        </Form.Item>
                      </Col>
                      <Col span={12}>
                        <Form.Item noStyle dependencies={[['channels', 'email']]}>
                          {({ getFieldValue }) => {
                            const emailEnabled = getFieldValue(['channels', 'email'])
                            if (!emailEnabled) return null
                            return (
                              <Form.Item
                                name={['audience', 'exclude_unsubscribed']}
                                label="Exclude unsubscribed recipients"
                                valuePropName="checked"
                                initialValue={true}
                              >
                                <Switch />
                              </Form.Item>
                            )
                          }}
                        </Form.Item>
                      </Col>
                    </Row>

                    <div className="text-xs mt-4 mb-4 font-semibold border-b border-solid border-gray-300 pb-2 text-gray-500">
                      URL Tracking Parameters
                    </div>
                    <Alert
                      description="These parameters are automatically added to the URL of the broadcast. They are used by web analytics tools to analyze the performance of your campaign."
                      type="info"
                      className="!mb-4"
                    />
                    <Form.Item name={['utm_parameters', 'source']} label="utm_source">
                      <Input placeholder="Your website or company name" />
                    </Form.Item>
                    <Form.Item
                      name={['utm_parameters', 'medium']}
                      label="utm_medium"
                      initialValue="email"
                    >
                      <Input placeholder="email" />
                    </Form.Item>
                    <Form.Item name={['utm_parameters', 'campaign']} label="utm_campaign">
                      <Input />
                    </Form.Item>
                  </div>
                </div>

                <div style={{ display: tab === 'web' ? 'block' : 'none' }}>
                  <div className="pt-8 pr-8">
                    <Form.Item
                      name={['channels', 'web']}
                      label="Publish to web"
                      valuePropName="checked"
                      initialValue={false}
                    >
                      <Switch
                        disabled={broadcast?.status !== 'draft' && broadcast?.status !== undefined}
                      />
                    </Form.Item>

                    {/* Web Settings Section - shown when web channel is enabled */}
                    <Form.Item noStyle dependencies={[['channels', 'web']]}>
                      {({ getFieldValue }) => {
                        const webEnabled = getFieldValue(['channels', 'web'])
                        const customEndpoint = workspace.settings?.custom_endpoint_url

                        if (!webEnabled) return null

                        if (!customEndpoint) {
                          return (
                            <Alert
                              type="warning"
                              message="Web publications require a custom domain"
                              description="Go to workspace settings and configure a custom endpoint URL to enable web publications."
                              showIcon
                              className="mb-4"
                            />
                          )
                        }

                        const listId = getFieldValue(['audience', 'list'])

                        if (!listId) {
                          return (
                            <Alert
                              type="warning"
                              message="Please select a list"
                              description="Please select a list to enable web publications."
                              showIcon
                              className="mb-4"
                            />
                          )
                        }

                        const list = lists.find((l) => l.id === listId)

                        let listWebEnabled = false
                        if (list && list.web_publication_enabled === true) {
                          listWebEnabled = true
                        }

                        if (!listWebEnabled) {
                          return (
                            <Alert
                              type="warning"
                              message="Web publications are not enabled for this list"
                              description="Go to list settings and enable web publications to use this feature."
                              showIcon
                              className="mb-4"
                            />
                          )
                        }

                        return (
                          <div className="mb-4">
                            <Form.Item
                              name={['web_publication_settings', 'slug']}
                              label={
                                <span>
                                  URL Slug{' '}
                                  <Tooltip title="Final URL published to web" className="ml-1">
                                    <InfoCircleOutlined style={{ color: '#999' }} />
                                  </Tooltip>
                                </span>
                              }
                              rules={[
                                { required: true, message: 'Please enter a URL slug' },
                                {
                                  pattern: /^[a-z0-9-]+$/,
                                  message: 'Only lowercase letters, numbers, and hyphens allowed'
                                }
                              ]}
                            >
                              <Input placeholder="my-blog-post" addonBefore={`${list?.slug}/`} />
                            </Form.Item>

                            <SEOSettingsForm
                              namePrefix={['web_publication_settings']}
                              titlePlaceholder="Page title for search engines"
                              descriptionPlaceholder="Brief description for search results"
                            />
                          </div>
                        )
                      }}
                    </Form.Item>
                  </div>
                </div>

                <div style={{ display: tab === 'content' ? 'block' : 'none' }}>
                  <div className="p-8">
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
                                    tooltip={
                                      <Tooltip
                                        title="Tracking (opens & clicks) should be enabled in your workspace settings to use this feature"
                                        className="ml-1"
                                      >
                                        <InfoCircleOutlined style={{ color: '#999' }} />
                                      </Tooltip>
                                    }
                                  >
                                    <Switch
                                      disabled={!workspace.settings?.email_tracking_enabled}
                                    />
                                  </Form.Item>
                                </Col>
                              </Row>

                              <ABTestingConfig form={form} />

                              {/* Variations management will be added here */}
                              <div className="text-xs mt-4 mb-4 font-bold border-b border-solid pb-2 border-gray-400 text-gray-900">
                                Variations
                              </div>

                              <Form.List name={['test_settings', 'variations']}>
                                {(fields, { add, remove }) => (
                                  <>
                                    {fields.map((field) => (
                                      <div key={field.key} className="">
                                        <Row gutter={24}>
                                          <Col span={22}>
                                            <Form.Item
                                              key={`template-${field.key}`}
                                              name={[field.name, 'template_id']}
                                              label={`Template ${field.key + 1}`}
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
                                            <Col
                                              span={2}
                                              className="flex items-end justify-end pb-2"
                                            >
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
                  </div>
                </div>
              </div>
            </div>
          </Form>
        </Drawer>
      )}
    </>
  )
}
