import { useState, useEffect } from 'react'
import { Button, Drawer, Form, Input, Space, App, Switch, Row, Col } from 'antd'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import {
  transactionalNotificationsApi,
  TransactionalNotification,
  CreateTransactionalNotificationRequest,
  UpdateTransactionalNotificationRequest
} from '../../services/api/transactional_notifications'
import type { Workspace } from '../../services/api/types'
import TemplateSelectorInput from '../templates/TemplateSelectorInput'
import React from 'react'
import extractTLD from '../utils/tld'

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
  buttonProps?: any
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
  const [isOpen, setIsOpen] = useState(false)
  const [form] = Form.useForm()
  const queryClient = useQueryClient()
  const [loading, setLoading] = useState(false)
  const { message, modal } = App.useApp()
  const [formTouched, setFormTouched] = useState(false)

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
      form.setFieldValue(['channels', 'email', 'utm_params', 'content'], apiId)
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
      message.success(`Notification ${notification ? 'updated' : 'created'} successfully`)
      handleClose()
      queryClient.invalidateQueries({ queryKey: ['transactional-notifications', workspace.id] })
      setLoading(false)
    },
    onError: (error) => {
      message.error(
        `Failed to ${notification ? 'update' : 'create'} notification: ${error.message}`
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
            template_id: '',
            tracking_enabled: true,
            utm_params: {
              source: domain || '',
              medium: 'email',
              campaign: 'transactional',
              content: ''
            },
            cc_enabled: false,
            bcc_enabled: false
          }
        }
      })
    }
    setFormTouched(false)
    setIsOpen(true)
  }

  const handleClose = () => {
    if (formTouched && !loading && !upsertNotificationMutation.isPending) {
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
            loading={loading || upsertNotificationMutation.isPending}
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
      <Button onClick={showDrawer} {...buttonProps}>
        {buttonContent || (notification ? 'Edit Notification' : 'Create Notification')}
      </Button>
      {isOpen && (
        <Drawer
          title={<>{notification ? 'Edit notification' : 'Create a notification'}</>}
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
                    metadata: values.metadata
                  }
                }
                upsertNotificationMutation.mutate(payload)
              }
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
              <Form.Item
                name="name"
                label="Notification name"
                rules={[{ required: true, message: 'Please enter a notification name' }]}
              >
                <Input placeholder="E.g. Password Reset Email" />
              </Form.Item>

              <Form.Item
                name="id"
                label="API Identifier"
                tooltip="This ID will be used when triggering the notification via API"
                rules={[
                  { required: true, message: 'Please enter an API identifier' },
                  {
                    pattern: /^[a-z0-9_]+$/,
                    message: 'ID can only contain lowercase letters, numbers, and underscores'
                  }
                ]}
              >
                <Input placeholder="E.g. password_reset" disabled={!!notification} />
              </Form.Item>

              <Form.Item name="description" label="Description">
                <Input.TextArea
                  rows={3}
                  placeholder="A brief description of this notification's purpose"
                />
              </Form.Item>

              <Form.Item
                name={['channels', 'email', 'template_id']}
                label="Email Template"
                rules={[{ required: true, message: 'Please select an email template' }]}
              >
                <TemplateSelectorInput
                  workspaceId={workspace.id}
                  placeholder="Select email template"
                  category="transactional"
                  utmDisabled={false}
                />
              </Form.Item>

              <p className="text-sm text-gray-500 pt-8">
                Define UTM parameters for links in your email for better campaign tracking.
              </p>

              <Row gutter={16}>
                <Col span={12}>
                  <Form.Item
                    name={['channels', 'email', 'utm_params', 'source']}
                    label="utm_source"
                    tooltip="Identifies which site sent the traffic (e.g. google, newsletter)"
                  >
                    <Input placeholder="e.g. notifuse" />
                  </Form.Item>

                  <Form.Item
                    name={['channels', 'email', 'utm_params', 'medium']}
                    label="utm_medium"
                    tooltip="Identifies what type of link was used (e.g. email, cpc, banner)"
                  >
                    <Input placeholder="e.g. email" />
                  </Form.Item>
                </Col>

                <Col span={12}>
                  <Form.Item
                    name={['channels', 'email', 'utm_params', 'campaign']}
                    label="utm_campaign"
                    tooltip="Identifies a specific product promotion or strategic campaign"
                  >
                    <Input placeholder="e.g. welcome_series" />
                  </Form.Item>

                  <Form.Item
                    name={['channels', 'email', 'utm_params', 'content']}
                    label="utm_content"
                    tooltip="Identifies what specifically was clicked (e.g. header_link, body_link)"
                  >
                    <Input placeholder="e.g. cta_button" />
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
