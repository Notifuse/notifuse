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
import { useLingui } from '@lingui/react/macro'
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

// Custom component to handle A/B testing configuration
const ABTestingConfig = ({ form }: { form: ReturnType<typeof Form.useForm>[0] }) => {
  const { t } = useLingui()
  const autoSendWinner = Form.useWatch(['test_settings', 'auto_send_winner'], form)

  if (!autoSendWinner) return null

  return (
    <Row gutter={24}>
      <Col span={12}>
        <Form.Item
          name={['test_settings', 'auto_send_winner_metric']}
          label={t`Winning metric`}
          rules={[{ required: true }]}
        >
          <Select
            options={[
              { value: 'open_rate', label: t`Open Rate` },
              { value: 'click_rate', label: t`Click Rate` }
            ]}
          />
        </Form.Item>
      </Col>
      <Col span={12}>
        <Form.Item
          name={['test_settings', 'test_duration_hours']}
          label={t`Test duration (hours)`}
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
  buttonProps?: Record<string, unknown>
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
  const { t } = useLingui()
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
      message.success(broadcast ? t`Broadcast updated successfully` : t`Broadcast created successfully`)
      handleClose()
      queryClient.invalidateQueries({ queryKey: ['broadcasts', workspace.id] })
      setLoading(false)
    },
    onError: (error) => {
      message.error(broadcast ? t`Failed to update broadcast: ${error.message}` : t`Failed to create broadcast: ${error.message}`)
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
        title: t`Unsaved changes`,
        content: t`You have unsaved changes. Are you sure you want to close this drawer?`,
        okText: t`Yes`,
        cancelText: t`No`,
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
    const tabOrder = ['audience', 'email', 'content']
    const currentIndex = tabOrder.indexOf(tab)
    if (currentIndex < tabOrder.length - 1) {
      setTab(tabOrder[currentIndex + 1])
    }
  }

  const handleTabChange = async (newTab: string) => {
    // Only validate if moving forward (not backward)
    const tabOrder = ['audience', 'email', 'content']
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
            {t`Cancel`}
          </Button>

          {tab === 'audience' && (
            <Button type="primary" onClick={goNext}>
              {t`Next`}
            </Button>
          )}

          {tab === 'email' && (
            <>
              <Button type="primary" ghost onClick={() => handleTabChange('audience')}>
                {t`Previous`}
              </Button>
              <Button type="primary" onClick={goNext}>
                {t`Next`}
              </Button>
            </>
          )}

          {tab === 'content' && (
            <>
              <Button type="primary" ghost onClick={() => handleTabChange('email')}>
                {t`Previous`}
              </Button>
              <Button
                loading={loading || upsertBroadcastMutation.isPending}
                onClick={() => {
                  form.submit()
                }}
                type="primary"
              >
                {t`Save`}
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
        {buttonContent || (broadcast ? t`Edit Broadcast` : t`Create Broadcast`)}
      </Button>
      {isOpen && (
        <Drawer
          title={<>{broadcast ? t`Edit broadcast` : t`Create a broadcast`}</>}
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
                  Array.isArray(info.errorFields[0].name) &&
                  info.errorFields[0].name[0] === 'test_settings'
                ) {
                  setTab('content')
                }

                message.error(t`Please check the form for errors.`)
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
                    label: t`1. Audience`
                  },
                  {
                    key: 'email',
                    label: t`2. Web Analytics`
                  },
                  {
                    key: 'content',
                    label: t`3. Content`
                  }
                ]}
              />
              <div className="flex-1 relative">
                <div style={{ display: tab === 'audience' ? 'block' : 'none' }}>
                  <div className="pt-8 pr-8">
                    <Form.Item
                      name="name"
                      label={t`Broadcast name`}
                      rules={[{ required: true, message: t`Please enter a broadcast name` }]}
                    >
                      <Input placeholder={t`E.g. Weekly Newsletter - May 2023`} />
                    </Form.Item>

                    <Form.Item
                      name={['audience', 'list']}
                      label={t`List`}
                      rules={[
                        {
                          required: true,
                          type: 'string',
                          message: t`Please select a list`
                        }
                      ]}
                    >
                      <Select
                        placeholder={t`Select a list`}
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
                          {t`Belonging to at least one of the following segments`}{' '}
                          <Tooltip
                            title={t`Optionally filter contacts by segments within the selected lists`}
                            className="ml-1"
                          >
                            <InfoCircleOutlined style={{ color: '#999' }} />
                          </Tooltip>
                        </span>
                      }
                    >
                      <Select
                        mode="multiple"
                        placeholder={t`Select segments (optional)`}
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

                    <Form.Item
                      name={['audience', 'exclude_unsubscribed']}
                      label={t`Exclude unsubscribed recipients`}
                      valuePropName="checked"
                      initialValue={true}
                    >
                      <Switch />
                    </Form.Item>
                  </div>
                </div>

                <div style={{ display: tab === 'email' ? 'block' : 'none' }}>
                  <div className="pt-8 pr-8">
                    <Alert
                      description={t`These parameters are automatically added to the URL of the broadcast. They are used by web analytics tools to analyze the performance of your campaign.`}
                      type="info"
                      className="!mb-4"
                    />
                    <Form.Item name={['utm_parameters', 'source']} label="utm_source">
                      <Input placeholder={t`Your website or company name`} />
                    </Form.Item>
                    <Form.Item
                      name={['utm_parameters', 'medium']}
                      label="utm_medium"
                      initialValue="email"
                    >
                      <Input placeholder={t`email`} />
                    </Form.Item>
                    <Form.Item name={['utm_parameters', 'campaign']} label="utm_campaign">
                      <Input />
                    </Form.Item>
                  </div>
                </div>

                <div style={{ display: tab === 'content' ? 'block' : 'none' }}>
                  <div className="pt-8 pr-8">
                    {!workspace.settings?.email_tracking_enabled && (
                      <Alert
                        description={t`Tracking (opens & clicks) must be enabled in workspace settings to use A/B testing features.`}
                        type="info"
                        showIcon
                        className="!mb-4"
                      />
                    )}

                    <Form.Item
                      name={['test_settings', 'enabled']}
                      label={t`Enable A/B Testing`}
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
                                    label={t`Test sample size (%)`}
                                    rules={[{ required: true }]}
                                  >
                                    <InputNumber min={1} max={100} />
                                  </Form.Item>
                                </Col>
                                <Col span={12}>
                                  <Form.Item
                                    name={['test_settings', 'auto_send_winner']}
                                    label={t`Automatically send winner`}
                                    valuePropName="checked"
                                    tooltip={
                                      <Tooltip
                                        title={t`Tracking (opens & clicks) should be enabled in your workspace settings to use this feature`}
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
                                {t`Variations`}
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
                                              label={t`Template ${field.key + 1}`}
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
                                                      (v: { template_id?: string }, i: number) =>
                                                        v?.template_id === value && i !== field.name
                                                    )

                                                    if (duplicates.length > 0) {
                                                      return Promise.reject(
                                                        new Error(
                                                          t`This template is already used in another variation`
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
                                                placeholder={t`Select template`}
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
                                                  title={t`Remove variation`}
                                                  description={t`Are you sure you want to remove this variation?`}
                                                  onConfirm={() => remove(field.name)}
                                                  okText={t`Yes`}
                                                  cancelText={t`No`}
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
                                        {t`+ Add variation`}
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
                              label={t`Template`}
                              rules={[{ required: true }]}
                            >
                              <TemplateSelectorInput
                                workspaceId={workspace.id}
                                placeholder={t`Select template`}
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
