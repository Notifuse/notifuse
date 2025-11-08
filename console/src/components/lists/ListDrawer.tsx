import React from 'react'
import {
  Button,
  Drawer,
  Form,
  Input,
  Switch,
  App,
  Tooltip,
  Row,
  Col,
  Divider,
  Alert,
  Popconfirm
} from 'antd'
import { InfoCircleOutlined } from '@ant-design/icons'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { listsApi } from '../../services/api/list'
import type {
  CreateListRequest,
  List,
  UpdateListRequest,
  TemplateReference,
  Workspace
} from '../../services/api/types'
import { TemplateSelectorInput } from '../../components/templates'
import { SEOSettingsForm } from '../seo/SEOSettingsForm'

interface CreateListDrawerProps {
  workspaceId: string
  workspace?: Workspace
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
  workspace,
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
  const [isSlugLocked, setIsSlugLocked] = React.useState(false)

  // Check if workspace web publication is configured
  const isWebPublicationConfigured =
    workspace?.settings?.custom_endpoint_url && workspace?.settings?.web_publications_enabled

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
      const existingSlug = list.web_publication_settings?.slug || list.slug || ''
      form.setFieldsValue({
        id: list.id,
        name: list.name,
        description: list.description,
        is_double_optin: list.is_double_optin,
        is_public: list.is_public,
        double_optin_template_id: list.double_optin_template?.id,
        welcome_template_id: list.welcome_template?.id,
        unsubscribe_template_id: list.unsubscribe_template?.id,
        web_publication_enabled: list.web_publication_enabled,
        web_publication_settings: {
          slug: existingSlug,
          meta_title: list.web_publication_settings?.meta_title || '',
          meta_description: list.web_publication_settings?.meta_description || '',
          keywords: list.web_publication_settings?.keywords || [],
          og_title: list.web_publication_settings?.og_title || '',
          og_description: list.web_publication_settings?.og_description || '',
          og_image: list.web_publication_settings?.og_image || ''
        }
      })
      // Lock slug if it was previously set
      setIsSlugLocked(!!existingSlug)
    }
    setOpen(true)
  }

  const onClose = () => {
    setOpen(false)
    form.resetFields()
    setIsSlugLocked(false)
  }

  const onFinish = (values: any) => {
    // Convert template ID to proper template reference if needed
    let doubleOptInTemplate: TemplateReference | undefined = undefined
    if (values.is_double_optin && values.double_optin_template_id) {
      doubleOptInTemplate = {
        id: values.double_optin_template_id,
        version: 1 // Using default version
      }
    }

    let welcomeTemplate: TemplateReference | undefined = undefined
    if (values.welcome_template_id) {
      welcomeTemplate = {
        id: values.welcome_template_id,
        version: 1 // Using default version
      }
    }

    let unsubscribeTemplate: TemplateReference | undefined = undefined
    if (values.unsubscribe_template_id) {
      unsubscribeTemplate = {
        id: values.unsubscribe_template_id,
        version: 1 // Using default version
      }
    }

    // Bundle web publication settings
    let webPublicationSettings = undefined
    if (values.web_publication_enabled && values.web_publication_settings) {
      webPublicationSettings = {
        slug: values.web_publication_settings.slug,
        meta_title: values.web_publication_settings.meta_title,
        meta_description: values.web_publication_settings.meta_description,
        keywords: values.web_publication_settings.keywords,
        og_title: values.web_publication_settings.og_title,
        og_description: values.web_publication_settings.og_description,
        og_image: values.web_publication_settings.og_image
      }
    }

    if (isEditMode) {
      const request: UpdateListRequest = {
        workspace_id: workspaceId,
        id: list.id,
        name: values.name,
        is_double_optin: values.is_double_optin || false,
        is_public: values.is_public || false,
        description: values.description,
        double_optin_template: doubleOptInTemplate,
        welcome_template: welcomeTemplate,
        unsubscribe_template: unsubscribeTemplate,
        web_publication_enabled: values.web_publication_enabled || false,
        web_publication_settings: webPublicationSettings
      }
      updateListMutation.mutate(request)
    } else {
      const request: CreateListRequest = {
        workspace_id: workspaceId,
        id: values.id,
        name: values.name,
        is_double_optin: values.is_double_optin || false,
        is_public: values.is_public || false,
        description: values.description,
        double_optin_template: doubleOptInTemplate,
        welcome_template: welcomeTemplate,
        unsubscribe_template: unsubscribeTemplate,
        web_publication_enabled: values.web_publication_enabled || false,
        web_publication_settings: webPublicationSettings
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
        width={960}
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
            is_public: false,
            web_publication_enabled: false
          }}
        >
          <Row gutter={32}>
            <Col span={12}>
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
                <Input.TextArea rows={1} placeholder="Enter list description" />
              </Form.Item>

              <Form.Item
                name="is_public"
                label={
                  <span>
                    Public (required for web publication) &nbsp;
                    <Tooltip title="Public lists are visible in the Notification Center for users to subscribe to">
                      <InfoCircleOutlined />
                    </Tooltip>
                  </span>
                }
                valuePropName="checked"
              >
                <Switch />
              </Form.Item>
            </Col>

            <Col span={12}>
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
            </Col>
          </Row>

          <Divider orientation="left" plain>
            Web Publication
          </Divider>

          {!isWebPublicationConfigured && (
            <Alert
              type="warning"
              message="Web Publication Not Configured"
              description="To enable web publication for this list, you must first configure a custom endpoint URL and enable web publications in your workspace settings."
              style={{ marginBottom: 24 }}
            />
          )}

          <Form.Item
            noStyle
            shouldUpdate={(prevValues, currentValues) =>
              prevValues.is_public !== currentValues.is_public
            }
          >
            {({ getFieldValue, setFieldsValue }) => {
              const isPublic = getFieldValue('is_public')

              // Automatically disable web publication when list is not public
              if (!isPublic && getFieldValue('web_publication_enabled')) {
                setFieldsValue({ web_publication_enabled: false })
              }

              return (
                <>
                  {!isPublic && (
                    <p style={{ marginBottom: 24, color: '#666' }}>
                      Web publication requires the list to be public. Enable the 'Public' setting
                      above to access web publication features.
                    </p>
                  )}
                  <Row gutter={32} className="mb-4">
                    <Col span={12}>
                      <Form.Item
                        name="web_publication_enabled"
                        label={
                          <span>
                            Enable &nbsp;
                            <Tooltip title="Allow this list to be published on the web with a dedicated page">
                              <InfoCircleOutlined />
                            </Tooltip>
                          </span>
                        }
                        valuePropName="checked"
                      >
                        <Switch disabled={!isPublic} />
                      </Form.Item>
                    </Col>
                    <Col span={12}>
                      <Form.Item
                        noStyle
                        shouldUpdate={(prevValues, currentValues) =>
                          prevValues.web_publication_enabled !==
                          currentValues.web_publication_enabled
                        }
                      >
                        {({ getFieldValue }) =>
                          getFieldValue('web_publication_enabled') ? (
                            <Form.Item
                              name={['web_publication_settings', 'slug']}
                              label="List Slug"
                              rules={[
                                { required: true, message: 'Please enter a slug' },
                                {
                                  pattern: /^[a-z0-9-]+$/,
                                  message:
                                    'Slug must be lowercase letters, numbers, and hyphens only'
                                },
                                { max: 100, message: 'Slug must be less than 100 characters' }
                              ]}
                            >
                              <Input
                                placeholder="my-newsletter"
                                disabled={isSlugLocked}
                                suffix={
                                  isSlugLocked ? (
                                    <Popconfirm
                                      title="Edit Slug"
                                      description="Changing the slug will impact SEO for existing publications. The old URL will no longer work. Are you sure you want to edit it?"
                                      onConfirm={() => setIsSlugLocked(false)}
                                      okText="Yes, Edit"
                                      cancelText="Cancel"
                                    >
                                      <Button type="link" size="small">
                                        Edit
                                      </Button>
                                    </Popconfirm>
                                  ) : null
                                }
                              />
                            </Form.Item>
                          ) : null
                        }
                      </Form.Item>
                    </Col>
                  </Row>

                  <Form.Item
                    noStyle
                    shouldUpdate={(prevValues, currentValues) =>
                      prevValues.web_publication_enabled !== currentValues.web_publication_enabled
                    }
                  >
                    {({ getFieldValue }) =>
                      getFieldValue('web_publication_enabled') ? (
                        <SEOSettingsForm
                          twoColumns={true}
                          namePrefix={['web_publication_settings']}
                          titlePlaceholder="SEO title for the list page"
                          descriptionPlaceholder="Description of this mailing list"
                        />
                      ) : null
                    }
                  </Form.Item>
                </>
              )
            }}
          </Form.Item>
        </Form>
      </Drawer>
    </>
  )
}
