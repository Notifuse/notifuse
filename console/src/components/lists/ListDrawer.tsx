import React from 'react'
import { Button, Drawer, Form, Input, Switch, App, Tooltip } from 'antd'
import { InfoCircleOutlined } from '@ant-design/icons'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { listsApi } from '../../services/api/list'
import type {
  CreateListRequest,
  List,
  UpdateListRequest,
  TemplateReference
} from '../../services/api/types'
import { TemplateSelectorInput } from '../../components/templates'

interface CreateListDrawerProps {
  workspaceId: string
  list?: List
  buttonProps?: {
    type?: 'primary' | 'default' | 'link' | 'text'
    buttonContent?: React.ReactNode
    size?: 'large' | 'middle' | 'small'
    disabled?: boolean
  }
}

export function CreateListDrawer({
  workspaceId,
  list,
  buttonProps = {
    type: 'primary',
    buttonContent: list ? 'Edit List' : 'Create List',
    size: 'middle'
  }
}: CreateListDrawerProps) {
  const [open, setOpen] = React.useState(false)
  const [form] = Form.useForm()
  const queryClient = useQueryClient()
  const isEditMode = !!list
  const { message } = App.useApp()

  // Generate list ID from name (alphanumeric only)
  const generateListId = (name: string) => {
    if (!name) return ''
    // Remove spaces and non-alphanumeric characters
    return name
      .toLowerCase()
      .replace(/[^a-z0-9]/g, '')
      .substring(0, 32)
  }

  // Update generated ID when name changes
  const handleNameChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    if (isEditMode) return // Don't update ID in edit mode

    const name = e.target.value
    const id = generateListId(name)
    form.setFieldsValue({ id })
  }

  const createListMutation = useMutation({
    mutationFn: (data: CreateListRequest) => {
      return listsApi.create(data)
    },
    onSuccess: () => {
      message.success('List created successfully')
      queryClient.invalidateQueries({ queryKey: ['lists', workspaceId] })
      setOpen(false)
      form.resetFields()
    },
    onError: (error) => {
      message.error(`Failed to create list: ${error}`)
    }
  })

  const updateListMutation = useMutation({
    mutationFn: (data: UpdateListRequest) => {
      return listsApi.update(data)
    },
    onSuccess: () => {
      message.success('List updated successfully')
      queryClient.invalidateQueries({ queryKey: ['lists', workspaceId] })
      setOpen(false)
      form.resetFields()
    },
    onError: (error) => {
      message.error(`Failed to update list: ${error}`)
    }
  })

  const showDrawer = () => {
    if (isEditMode) {
      // Populate form with existing list data
      form.setFieldsValue({
        id: list.id,
        name: list.name,
        description: list.description,
        is_double_optin: list.is_double_optin,
        is_public: list.is_public,
        double_optin_template_id: list.double_optin_template?.id,
        welcome_template_id: list.welcome_template?.id,
        unsubscribe_template_id: list.unsubscribe_template?.id
      })
    }
    setOpen(true)
  }

  const onClose = () => {
    setOpen(false)
    form.resetFields()
  }

  const onFinish = (values: Record<string, unknown>) => {
    // Convert template ID to proper template reference if needed
    let doubleOptInTemplate: TemplateReference | undefined = undefined
    if (values.is_double_optin && values.double_optin_template_id) {
      doubleOptInTemplate = {
        id: String(values.double_optin_template_id),
        version: 1 // Using default version
      }
    }

    let welcomeTemplate: TemplateReference | undefined = undefined
    if (values.welcome_template_id) {
      welcomeTemplate = {
        id: String(values.welcome_template_id),
        version: 1 // Using default version
      }
    }

    let unsubscribeTemplate: TemplateReference | undefined = undefined
    if (values.unsubscribe_template_id) {
      unsubscribeTemplate = {
        id: String(values.unsubscribe_template_id),
        version: 1 // Using default version
      }
    }

    if (isEditMode) {
      const request: UpdateListRequest = {
        workspace_id: workspaceId,
        id: list.id,
        name: String(values.name),
        is_double_optin: Boolean(values.is_double_optin),
        is_public: Boolean(values.is_public),
        description: values.description ? String(values.description) : undefined,
        double_optin_template: doubleOptInTemplate,
        welcome_template: welcomeTemplate,
        unsubscribe_template: unsubscribeTemplate
      }
      updateListMutation.mutate(request)
    } else {
      const request: CreateListRequest = {
        workspace_id: workspaceId,
        id: String(values.id),
        name: String(values.name),
        is_double_optin: Boolean(values.is_double_optin),
        is_public: Boolean(values.is_public),
        description: values.description ? String(values.description) : undefined,
        double_optin_template: doubleOptInTemplate,
        welcome_template: welcomeTemplate,
        unsubscribe_template: unsubscribeTemplate
      }
      createListMutation.mutate(request)
    }
  }

  return (
    <>
      <Button
        type={buttonProps.type || 'primary'}
        onClick={showDrawer}
        size={buttonProps.size}
        disabled={buttonProps.disabled}
      >
        {buttonProps.buttonContent || (isEditMode ? 'Edit List' : 'Create List')}
      </Button>
      <Drawer
        title={isEditMode ? 'Edit List' : 'Create New List'}
        width={400}
        onClose={onClose}
        open={open}
        styles={{
          body: { paddingBottom: 80 }
        }}
        extra={
          <Button
            type="primary"
            onClick={() => form.submit()}
            loading={isEditMode ? updateListMutation.isPending : createListMutation.isPending}
          >
            {isEditMode ? 'Save' : 'Create'}
          </Button>
        }
      >
        <Form
          form={form}
          layout="vertical"
          onFinish={onFinish}
          initialValues={{
            is_double_optin: false,
            is_public: false
          }}
        >
          <Form.Item
            name="name"
            label="Name"
            rules={[
              { required: true, message: 'Please enter a list name' },
              { max: 255, message: 'Name must be less than 255 characters' }
            ]}
          >
            <Input placeholder="Enter list name" onChange={handleNameChange} />
          </Form.Item>

          <Form.Item
            name="id"
            label="List ID"
            rules={[
              { required: true, message: 'Please enter a list ID' },
              { pattern: /^[a-zA-Z0-9]+$/, message: 'ID must be alphanumeric' },
              { max: 32, message: 'ID must be less than 32 characters' }
            ]}
          >
            <Input placeholder="Enter a unique alphanumeric ID" disabled={isEditMode} />
          </Form.Item>

          <Form.Item name="description" label="Description">
            <Input.TextArea rows={4} placeholder="Enter list description" />
          </Form.Item>

          <Form.Item
            name="is_public"
            label={
              <span>
                Public &nbsp;
                <Tooltip title="Public lists are visible in the Notification Center for users to subscribe to">
                  <InfoCircleOutlined />
                </Tooltip>
              </span>
            }
            valuePropName="checked"
          >
            <Switch />
          </Form.Item>

          <Form.Item
            name="is_double_optin"
            label={
              <span>
                Double Opt-in &nbsp;
                <Tooltip title="When enabled, subscribers must confirm their subscription via email before being added to the list">
                  <InfoCircleOutlined />
                </Tooltip>
              </span>
            }
            valuePropName="checked"
          >
            <Switch />
          </Form.Item>

          <Form.Item
            noStyle
            shouldUpdate={(prevValues, currentValues) =>
              prevValues.is_double_optin !== currentValues.is_double_optin
            }
          >
            {({ getFieldValue }) =>
              getFieldValue('is_double_optin') ? (
                <Form.Item
                  name="double_optin_template_id"
                  label="Double Opt-in Template"
                  rules={[
                    { required: true, message: 'Please select a template for double opt-in' }
                  ]}
                >
                  <TemplateSelectorInput
                    workspaceId={workspaceId}
                    category="opt_in"
                    placeholder="Select confirmation email template"
                    clearable={false}
                  />
                </Form.Item>
              ) : null
            }
          </Form.Item>

          <Form.Item
            name="welcome_template_id"
            label={
              <span>
                Welcome Template &nbsp;
                <Tooltip title="Email template sent to subscribers when they join this list">
                  <InfoCircleOutlined />
                </Tooltip>
              </span>
            }
          >
            <TemplateSelectorInput
              workspaceId={workspaceId}
              category="welcome"
              placeholder="Select welcome email template"
              clearable={true}
            />
          </Form.Item>

          <Form.Item
            name="unsubscribe_template_id"
            label={
              <span>
                Unsubscribe Template &nbsp;
                <Tooltip title="Email template sent to subscribers when they unsubscribe from this list">
                  <InfoCircleOutlined />
                </Tooltip>
              </span>
            }
          >
            <TemplateSelectorInput
              workspaceId={workspaceId}
              category="unsubscribe"
              placeholder="Select unsubscribe email template"
              clearable={true}
            />
          </Form.Item>
        </Form>
      </Drawer>
    </>
  )
}
