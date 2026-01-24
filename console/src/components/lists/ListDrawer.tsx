import React from 'react'
import { Button, Drawer, Form, Input, Switch, App, Tooltip } from 'antd'
import { InfoCircleOutlined } from '@ant-design/icons'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useLingui } from '@lingui/react/macro'
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
  const { t } = useLingui()
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
      message.success(t`List created successfully`)
      queryClient.invalidateQueries({ queryKey: ['lists', workspaceId] })
      setOpen(false)
      form.resetFields()
    },
    onError: (error) => {
      message.error(t`Failed to create list: ${error}`)
    }
  })

  const updateListMutation = useMutation({
    mutationFn: (data: UpdateListRequest) => {
      return listsApi.update(data)
    },
    onSuccess: () => {
      message.success(t`List updated successfully`)
      queryClient.invalidateQueries({ queryKey: ['lists', workspaceId] })
      setOpen(false)
      form.resetFields()
    },
    onError: (error) => {
      message.error(t`Failed to update list: ${error}`)
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
        double_optin_template_id: list.double_optin_template?.id
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

    if (isEditMode) {
      const request: UpdateListRequest = {
        workspace_id: workspaceId,
        id: list.id,
        name: String(values.name),
        is_double_optin: Boolean(values.is_double_optin),
        is_public: Boolean(values.is_public),
        description: values.description ? String(values.description) : undefined,
        double_optin_template: doubleOptInTemplate
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
        double_optin_template: doubleOptInTemplate
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
        {buttonProps.buttonContent || (isEditMode ? t`Edit List` : t`Create List`)}
      </Button>
      <Drawer
        title={isEditMode ? t`Edit List` : t`Create New List`}
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
            {isEditMode ? t`Save` : t`Create`}
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
            label={t`Name`}
            rules={[
              { required: true, message: t`Please enter a list name` },
              { max: 255, message: t`Name must be less than 255 characters` }
            ]}
          >
            <Input placeholder={t`Enter list name`} onChange={handleNameChange} />
          </Form.Item>

          <Form.Item
            name="id"
            label={t`List ID`}
            rules={[
              { required: true, message: t`Please enter a list ID` },
              { pattern: /^[a-zA-Z0-9]+$/, message: t`ID must be alphanumeric` },
              { max: 32, message: t`ID must be less than 32 characters` }
            ]}
          >
            <Input placeholder={t`Enter a unique alphanumeric ID`} disabled={isEditMode} />
          </Form.Item>

          <Form.Item name="description" label={t`Description`}>
            <Input.TextArea rows={4} placeholder={t`Enter list description`} />
          </Form.Item>

          <Form.Item
            name="is_public"
            label={
              <span>
                {t`Public`} &nbsp;
                <Tooltip title={t`Public lists are visible in the Notification Center for users to subscribe to`}>
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
                {t`Double Opt-in`} &nbsp;
                <Tooltip title={t`When enabled, subscribers must confirm their subscription via email before being added to the list`}>
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
                  label={t`Double Opt-in Template`}
                  rules={[
                    { required: true, message: t`Please select a template for double opt-in` }
                  ]}
                >
                  <TemplateSelectorInput
                    workspaceId={workspaceId}
                    category="opt_in"
                    placeholder={t`Select confirmation email template`}
                    clearable={false}
                  />
                </Form.Item>
              ) : null
            }
          </Form.Item>

        </Form>
      </Drawer>
    </>
  )
}
