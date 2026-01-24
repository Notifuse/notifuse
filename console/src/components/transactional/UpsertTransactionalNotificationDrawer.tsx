import { useState, useEffect } from 'react'
import { Button, Drawer, Form, Input, Space, App, Row, Col } from 'antd'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useLingui } from '@lingui/react/macro'
import {
  transactionalNotificationsApi,
  TransactionalNotification,
  CreateTransactionalNotificationRequest,
  UpdateTransactionalNotificationRequest
} from '../../services/api/transactional_notifications'
import type { Workspace } from '../../services/api/types'
import TemplateSelectorInput from '../templates/TemplateSelectorInput'
import React from 'react'
import extractTLD from '../../lib/tld'

// Helper function to generate a valid API ID from a name
const generateApiId = (name: string): string => {
  if (!name) return ''

  // Remove special characters, replace spaces with underscores, and convert to lowercase
  return name
    .toLowerCase()
    .replace(/[^\w\s]/g, '')
    .replace(/\s+/g, '_')
    .replace(/_+/g, '_')
}

interface UpsertTransactionalNotificationDrawerProps {
  workspace: Workspace
  notification?: TransactionalNotification
  buttonProps?: Record<string, unknown>
  buttonContent?: React.ReactNode
  onClose?: () => void
}

export function UpsertTransactionalNotificationDrawer({
  workspace,
  notification,
  buttonProps = {},
  buttonContent,
  onClose
}: UpsertTransactionalNotificationDrawerProps) {
  const { t } = useLingui()
  const [isOpen, setIsOpen] = useState(false)
  const [form] = Form.useForm()
  const queryClient = useQueryClient()
  const [loading, setLoading] = useState(false)
  const { message, modal } = App.useApp()
  const [formTouched, setFormTouched] = useState(false)

  // Check if notification is managed by an integration
  const isIntegrationManaged =
    notification?.integration_id != null && notification.integration_id !== ''

  // Watch notification name changes using Form.useWatch
  const notificationName = Form.useWatch('name', form)

  // Update API ID when name changes
  useEffect(() => {
    if (notificationName && isOpen && !notification) {
      // Only auto-generate ID for new notifications
      const apiId = generateApiId(notificationName)
      form.setFieldValue('id', apiId)

      // Also update utm_content with the same pattern
      // UTM parameters are independent of tracking_enabled
      form.setFieldValue(['tracking_settings', 'utm_content'], apiId)
    }
  }, [notificationName, form, isOpen, notification])

  const upsertNotificationMutation = useMutation({
    mutationFn: (
      values: CreateTransactionalNotificationRequest | UpdateTransactionalNotificationRequest
    ) => {
      if (notification) {
        return transactionalNotificationsApi.update(
          values as UpdateTransactionalNotificationRequest
        )
      } else {
        return transactionalNotificationsApi.create(
          values as CreateTransactionalNotificationRequest
        )
      }
    },
    onSuccess: () => {
      message.success(
        notification
          ? t`Notification updated successfully`
          : t`Notification created successfully`
      )
      handleClose()
      queryClient.invalidateQueries({ queryKey: ['transactional-notifications', workspace.id] })
      setLoading(false)
    },
    onError: (error) => {
      message.error(
        notification
          ? t`Failed to update notification: ${error.message}`
          : t`Failed to create notification: ${error.message}`
      )
      setLoading(false)
    }
  })

  const showDrawer = () => {
    if (notification) {
      // For existing notifications, populate form with current values
      form.setFieldsValue({
        id: notification.id,
        name: notification.name,
        description: notification.description,
        channels: notification.channels,
        tracking_settings: notification.tracking_settings,
        metadata: notification.metadata || undefined
      })
    } else {
      // Extract domain from website_url
      const domain = extractTLD(workspace.settings.website_url || '')

      // Set default values for a new notification
      form.setFieldsValue({
        id: '',
        name: '',
        description: '',
        channels: {
          email: {
            template_id: ''
          }
        },
        tracking_settings: {
          utm_source: domain || '',
          utm_medium: 'email',
          utm_campaign: 'transactional',
          utm_content: ''
        }
      })
    }
    setFormTouched(false)
    setIsOpen(true)
  }

  const handleClose = () => {
    if (formTouched && !loading && !upsertNotificationMutation.isPending) {
      modal.confirm({
        title: t`Unsaved changes`,
        content: t`You have unsaved changes. Are you sure you want to close this drawer?`,
        okText: t`Yes`,
        cancelText: t`No`,
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
            {t`Cancel`}
          </Button>
          <Button
            loading={loading || upsertNotificationMutation.isPending}
            onClick={() => {
              form.submit()
            }}
            type="primary"
          >
            {t`Save`}
          </Button>
        </Space>
      </div>
    )
  }

  return (
    <>
      <Button onClick={showDrawer} {...buttonProps}>
        {buttonContent || (notification ? t`Edit Notification` : t`Create Notification`)}
      </Button>
      {isOpen && (
        <Drawer
          title={<>{notification ? t`Edit notification` : t`Create a notification`}</>}
          closable={true}
          width={600}
          keyboard={false}
          maskClosable={false}
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

              if (notification) {
                // Update notification
                const payload: UpdateTransactionalNotificationRequest = {
                  workspace_id: workspace.id,
                  id: notification.id,
                  updates: {
                    name: values.name,
                    description: values.description,
                    channels: values.channels,
                    tracking_settings: values.tracking_settings,
                    metadata: values.metadata
                  }
                }
                upsertNotificationMutation.mutate(payload)
              } else {
                // Create notification
                const payload: CreateTransactionalNotificationRequest = {
                  workspace_id: workspace.id,
                  notification: {
                    id: values.id,
                    name: values.name,
                    description: values.description,
                    channels: values.channels,
                    tracking_settings: values.tracking_settings,
                    metadata: values.metadata
                  }
                }
                upsertNotificationMutation.mutate(payload)
              }
            }}
            onFinishFailed={(info) => {
              if (info.errorFields) {
                message.error(t`Please check the form for errors.`)
              }
              setLoading(false)
            }}
            onValuesChange={() => {
              setFormTouched(true)
            }}
          >
            <div className="p-8">
              <Form.Item
                name="name"
                label={t`Notification name`}
                rules={[{ required: true, message: t`Please enter a notification name` }]}
                tooltip={
                  isIntegrationManaged
                    ? t`This notification is managed by an integration and cannot be renamed`
                    : undefined
                }
              >
                <Input placeholder={t`E.g. Password Reset Email`} disabled={isIntegrationManaged} />
              </Form.Item>

              <Form.Item
                name="id"
                label={t`API Identifier`}
                tooltip={t`This ID will be used when triggering the notification via API`}
                rules={[
                  { required: true, message: t`Please enter an API identifier` },
                  {
                    pattern: /^[a-z0-9_]+$/,
                    message: t`ID can only contain lowercase letters, numbers, and underscores`
                  }
                ]}
              >
                <Input placeholder={t`E.g. password_reset`} disabled={!!notification} />
              </Form.Item>

              <Form.Item
                name={['channels', 'email', 'template_id']}
                label={t`Email Template`}
                rules={[{ required: true, message: t`Please select an email template` }]}
                tooltip={
                  isIntegrationManaged
                    ? t`This notification is managed by an integration and the template cannot be changed`
                    : undefined
                }
              >
                <TemplateSelectorInput
                  workspaceId={workspace.id}
                  placeholder={t`Select email template`}
                  category="transactional"
                  disabled={isIntegrationManaged}
                />
              </Form.Item>

              <Form.Item name="description" label={t`Description`}>
                <Input.TextArea
                  rows={3}
                  placeholder={t`A brief description of this notification's purpose`}
                />
              </Form.Item>

              <p className="text-sm text-gray-500 pt-8">
                {t`Define UTM parameters for links in your email for better campaign tracking.`}
              </p>

              <Row gutter={16}>
                <Col span={12}>
                  <Form.Item
                    name={['tracking_settings', 'utm_source']}
                    label="utm_source"
                    tooltip={t`Identifies which site sent the traffic (e.g. google, newsletter)`}
                  >
                    <Input placeholder={t`e.g. notifuse`} />
                  </Form.Item>

                  <Form.Item
                    name={['tracking_settings', 'utm_medium']}
                    label="utm_medium"
                    tooltip={t`Identifies what type of link was used (e.g. email, cpc, banner)`}
                  >
                    <Input placeholder={t`e.g. email`} />
                  </Form.Item>
                </Col>

                <Col span={12}>
                  <Form.Item
                    name={['tracking_settings', 'utm_campaign']}
                    label="utm_campaign"
                    tooltip={t`Identifies a specific product promotion or strategic campaign`}
                  >
                    <Input placeholder={t`e.g. welcome_series`} />
                  </Form.Item>

                  <Form.Item
                    name={['tracking_settings', 'utm_content']}
                    label="utm_content"
                    tooltip={t`Identifies what specifically was clicked (e.g. header_link, body_link)`}
                  >
                    <Input placeholder={t`e.g. cta_button`} />
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

export default UpsertTransactionalNotificationDrawer
