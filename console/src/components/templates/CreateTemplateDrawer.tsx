import { useState, useMemo, useRef } from 'react'
import {
  Button,
  Drawer,
  Form,
  Input,
  Select,
  Space,
  App,
  Tabs,
  Row,
  Col,
  Tag,
  Dropdown,
  MenuProps
} from 'antd'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { templatesApi } from '../../services/api/template'
import { workspaceService } from '../../services/api/workspace'
import type { Template, Workspace, TemplateBlock } from '../../services/api/types'
import EmailBuilder from '../email_builder/EmailBuilder'
import type { EmailBlock } from '../email_builder/types'
import { kebabCase } from 'lodash'
import IphoneEmailPreview from './PhonePreview'
import defaultTemplateData from './email-template.json'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faQuestion } from '@fortawesome/free-solid-svg-icons'
import { Tour } from 'antd/lib'
import { ImportExportButton } from './ImportExportButton'
import { useAuth } from '../../contexts/AuthContext'

interface CreateTemplateDrawerProps {
  workspace: Workspace
  template?: Template
  fromTemplate?: Template
  buttonProps?: any
  buttonContent?: React.ReactNode
  onClose?: () => void
  forceCategory?: string
}

/**
 * Creates default email blocks from the template JSON
 */
const createDefaultBlocks = (): EmailBlock => {
  return defaultTemplateData.emailTree as EmailBlock
}

// Help & Support dropdown component
const HelpSupportDropdown: React.FC<{ onStartTour: () => void }> = ({ onStartTour }) => {
  const menuItems: MenuProps['items'] = [
    {
      key: 'tour',
      label: 'Take a Tour',
      icon: <FontAwesomeIcon icon={faQuestion} />,
      onClick: onStartTour
    }
  ]

  return (
    <Dropdown menu={{ items: menuItems }} placement="bottomRight" trigger={['click']}>
      <Button
        size="small"
        title="Help & Support"
        type="primary"
        ghost
        icon={<FontAwesomeIcon icon={faQuestion} size="sm" />}
      >
        Help
      </Button>
    </Dropdown>
  )
}
/**
 * Renders a Tag component with the appropriate color for an email template category
 */
export const renderCategoryTag = (category: string) => {
  let color = 'default'

  if (['marketing', 'transactional'].includes(category)) {
    color = 'green'
  } else if (category === 'welcome') {
    color = 'blue'
  } else if (['opt_in', 'unsubscribe', 'bounce', 'blocklist'].includes(category)) {
    color = 'purple'
  }

  return (
    <Tag bordered={false} color={color}>
      {category.charAt(0).toUpperCase() + category.slice(1).replace('_', '-')}
    </Tag>
  )
}

export function CreateTemplateDrawer({
  workspace,
  template,
  fromTemplate,
  buttonProps = {},
  buttonContent,
  onClose,
  forceCategory
}: CreateTemplateDrawerProps) {
  const [isOpen, setIsOpen] = useState(false)
  const [form] = Form.useForm()
  const queryClient = useQueryClient()
  const [tab, setTab] = useState<string>('settings')
  const [loading, setLoading] = useState(false)
  const { message } = App.useApp()
  const { refreshWorkspaces } = useAuth()
  const [tourOpen, setTourOpen] = useState(false)
  const [forcedViewMode, setForcedViewMode] = useState<'edit' | 'preview' | null>(null)
  const [selectedBlockId, setSelectedBlockId] = useState<string | null>(null)

  // Refs for tour targets
  const treePanelRef = useRef<HTMLDivElement>(null)
  const editPanelRef = useRef<HTMLDivElement>(null)
  const settingsPanelRef = useRef<HTMLDivElement>(null)
  const previewSwitcherRef = useRef<HTMLDivElement>(null)
  const mobileDesktopSwitcherRef = useRef<HTMLDivElement>(null)
  const importExportButtonRef = useRef<HTMLDivElement>(null)

  // set the tree apart to avoid rerendering the Email Editor when the tree changes
  const [visualEditorTree, setVisualEditorTree] = useState<EmailBlock>(() => {
    if (template && template.email?.visual_editor_tree) {
      // Check if visual_editor_tree is already an object
      if (typeof template.email.visual_editor_tree === 'object') {
        return template.email.visual_editor_tree as unknown as EmailBlock
      }

      // Otherwise parse it from string
      try {
        return JSON.parse(template.email.visual_editor_tree) as EmailBlock
      } catch (error) {
        console.error('Error parsing visual editor tree:', error)
        message.error('Error loading template: Invalid template data')
        return createDefaultBlocks()
      }
    }
    return createDefaultBlocks()
  })

  // Add Form.useWatch for the email fields
  const senderID = Form.useWatch(['email', 'sender_id'], form)
  const emailSubject = Form.useWatch(['email', 'subject'], form)
  const emailPreview = Form.useWatch(['email', 'subject_preview'], form)
  const categoryValue = forceCategory || Form.useWatch(['category'], form)

  const emailProvider = useMemo(() => {
    const providerId =
      categoryValue === 'marketing'
        ? workspace.settings.marketing_email_provider_id
        : workspace.settings.transactional_email_provider_id
    return workspace.integrations?.find((integration) => integration.id === providerId)
  }, [workspace.integrations, categoryValue])

  const emailSender = useMemo(() => {
    if (emailProvider) {
      return emailProvider.email_provider.senders.find((sender) => sender.id === senderID)
    }
    return null
  }, [emailProvider, senderID])

  const updateWorkspaceMutation = useMutation({
    mutationFn: (updatedSettings: any) => {
      return workspaceService.update({
        id: workspace.id,
        name: workspace.name,
        settings: updatedSettings
      })
    },
    onSuccess: async () => {
      // Invalidate workspace query to refetch latest data
      queryClient.invalidateQueries({ queryKey: ['workspace', workspace.id] })
      message.success('Template block saved successfully')

      // Refresh workspaces in AuthContext to immediately update the workspace state
      // This ensures the EmailBuilder shows the saved blocks without requiring a page refresh
      await refreshWorkspaces()
    },
    onError: (error: any) => {
      console.error('Failed to update workspace:', error)
      message.error('Failed to save template block')
    }
  })

  const createTemplateMutation = useMutation({
    mutationFn: (values: any) => {
      if (template) {
        return templatesApi.update({
          ...values,
          channel: 'email',
          workspace_id: workspace.id,
          id: template.id
        })
      } else {
        return templatesApi.create({
          ...values,
          channel: 'email',
          workspace_id: workspace.id
        })
      }
    },
    onSuccess: () => {
      message.success(`Template ${template ? 'updated' : 'created'} successfully`)
      handleClose()
      queryClient.invalidateQueries({ queryKey: ['templates', workspace.id] })
      setLoading(false)
    },
    onError: (error) => {
      message.error(`Failed to ${template ? 'update' : 'create'} template: ${error.message}`)
      setLoading(false)
    }
  })

  const defaultTestData = {
    contact: {
      first_name: 'John',
      last_name: 'Doe',
      email: 'john.doe@example.com'
    },
    unsubscribe_url: `${window.API_ENDPOINT}/unsubscribe?email={{ contact.email }}&lid={{ list.id }}&email_hmac={{ contact.hmac }}`
  }

  const showDrawer = () => {
    if (template) {
      form.setFieldsValue({
        name: template.name,
        id: template.id || kebabCase(template.name),
        category: template.category || undefined,
        email: {
          sender_id: template.email?.sender_id || undefined,
          reply_to: template.email?.reply_to || undefined,
          subject: template.email?.subject || '',
          subject_preview: template.email?.subject_preview || '',
          content: template.email?.visual_editor_tree || '',
          visual_editor_tree: template.email?.visual_editor_tree || createDefaultBlocks()
        },
        test_data: template.test_data || defaultTestData
      })
    } else if (fromTemplate) {
      // Clone template functionality - append "copy" as suffix instead of "Copy of" prefix
      form.setFieldsValue({
        name: `${fromTemplate.name} copy`,
        id: kebabCase(`${fromTemplate.name}-copy`),
        category: fromTemplate.category || forceCategory || undefined,
        email: {
          sender_id: fromTemplate.email?.sender_id || undefined,
          reply_to: fromTemplate.email?.reply_to || undefined,
          subject: fromTemplate.email?.subject || '',
          subject_preview: fromTemplate.email?.subject_preview || '',
          content: fromTemplate.email?.visual_editor_tree || '',
          visual_editor_tree: fromTemplate.email?.visual_editor_tree || createDefaultBlocks()
        },
        test_data: fromTemplate.test_data || defaultTestData
      })

      // Update the visual editor tree
      if (fromTemplate.email?.visual_editor_tree) {
        if (typeof fromTemplate.email.visual_editor_tree === 'object') {
          setVisualEditorTree(fromTemplate.email.visual_editor_tree as unknown as EmailBlock)
        } else {
          try {
            setVisualEditorTree(JSON.parse(fromTemplate.email.visual_editor_tree) as EmailBlock)
          } catch (error) {
            console.error('Error parsing visual editor tree:', error)
            message.error('Error loading template: Invalid template data')
          }
        }
      }
    }
    setIsOpen(true)
  }

  const handleClose = () => {
    setIsOpen(false)
    form.resetFields()
    setTab('settings')
    if (onClose) {
      onClose()
    }
  }

  const handleImport = (tree: EmailBlock) => {
    setVisualEditorTree(tree)
  }

  const handleSaveBlock = async (
    block: EmailBlock,
    operation: 'create' | 'update' | 'delete',
    nameOrId: string
  ) => {
    try {
      const currentTemplateBlocks = workspace.settings.template_blocks || []
      let updatedTemplateBlocks: TemplateBlock[]

      if (operation === 'create') {
        // Create new template block
        const newTemplateBlock: TemplateBlock = {
          id: '', // Will be generated by the backend
          name: nameOrId,
          block: block,
          created: new Date().toISOString(),
          updated: new Date().toISOString()
        }
        updatedTemplateBlocks = [...currentTemplateBlocks, newTemplateBlock]
      } else if (operation === 'update') {
        // Update existing template block
        updatedTemplateBlocks = currentTemplateBlocks.map((templateBlock) =>
          templateBlock.id === nameOrId
            ? { ...templateBlock, block: block, updated: new Date().toISOString() }
            : templateBlock
        )
      } else if (operation === 'delete') {
        // Delete template block
        updatedTemplateBlocks = currentTemplateBlocks.filter(
          (templateBlock) => templateBlock.id !== nameOrId
        )
      } else {
        return // Invalid operation
      }

      // Update workspace settings with new template blocks
      const updatedSettings = {
        ...workspace.settings,
        template_blocks: updatedTemplateBlocks
      }

      await updateWorkspaceMutation.mutateAsync(updatedSettings)
    } catch (error) {
      console.error('Failed to save template block:', error)
    }
  }

  const goNext = () => {
    setTab('template')
  }

  return (
    <>
      <Button type="primary" onClick={showDrawer} {...buttonProps}>
        {buttonContent ||
          (template ? 'Edit Template' : fromTemplate ? 'Clone Template' : 'Create Template')}
      </Button>
      {isOpen && (
        <Drawer
          title={
            <>
              {template
                ? 'Edit email template'
                : fromTemplate
                  ? 'Clone email template'
                  : 'Create an email template'}
            </>
          }
          closable={true}
          keyboard={false}
          maskClosable={false}
          width={'100%'}
          open={isOpen}
          onClose={handleClose}
          className="drawer-no-transition drawer-body-no-padding"
          extra={
            <div className="text-right">
              <Space>
                <Button type="link" loading={loading} onClick={handleClose}>
                  Cancel
                </Button>

                {tab === 'settings' && (
                  <Button type="primary" onClick={goNext}>
                    Next
                  </Button>
                )}
                {tab === 'template' && (
                  <Button type="primary" ghost onClick={() => setTab('settings')}>
                    Previous
                  </Button>
                )}

                {tab === 'template' && (
                  <Button
                    loading={loading || createTemplateMutation.isPending}
                    onClick={() => {
                      form.submit()
                    }}
                    type="primary"
                  >
                    Save
                  </Button>
                )}
              </Space>
            </div>
          }
        >
          <Form
            form={form}
            layout="vertical"
            onFinish={(values) => {
              setLoading(true)
              values.email.visual_editor_tree = visualEditorTree
              createTemplateMutation.mutate(values)
            }}
            onFinishFailed={(info) => {
              if (info.errorFields) {
                info.errorFields.forEach((field: any) => {
                  // field.name can be an array, so we need to concatenate the array into a string
                  const fieldName = field.name.join('.')
                  if (
                    [
                      'name',
                      'id',
                      'category',
                      'email.sender_id',
                      'email.subject',
                      'email.subject_preview',
                      'email.reply_to'
                    ].indexOf(fieldName) !== -1
                  ) {
                    setTab('settings')
                  }
                })
              }
              setLoading(false)
            }}
            initialValues={{
              'email.visual_editor_tree': visualEditorTree,
              category: forceCategory || undefined,
              test_data: defaultTestData
            }}
          >
            <Form.Item name="test_data" hidden>
              <Input type="hidden" />
            </Form.Item>

            <div className="flex justify-center">
              <Tabs
                activeKey={tab}
                centered
                onChange={(k) => setTab(k)}
                style={{ display: 'inline-block' }}
                className="tabs-in-header"
                destroyInactiveTabPane={false}
                items={[
                  {
                    key: 'settings',
                    label: '1. Settings'
                  },
                  {
                    key: 'template',
                    label: '2. Template'
                  }
                ]}
              />
            </div>
            <div className="relative">
              <div style={{ display: tab === 'settings' ? 'block' : 'none' }}>
                <div className="p-8">
                  <Row gutter={24}>
                    <Col span={8}>
                      <Form.Item name="name" label="Template name" rules={[{ required: true }]}>
                        <Input
                          placeholder="i.e: Welcome Email"
                          onChange={(e: any) => {
                            if (!template) {
                              const id = kebabCase(e.target.value)
                              form.setFieldsValue({ id: id })
                              form.validateFields(['id'])
                            }
                          }}
                        />
                      </Form.Item>
                    </Col>
                    <Col span={8}>
                      <Form.Item
                        name="id"
                        label="Template ID (utm_content)"
                        tooltip="This is the ID that will be used as the utm_content parameter in the links URL to track the template"
                        rules={[
                          {
                            required: true,
                            type: 'string',
                            pattern: /^[a-z0-9]+(-[a-z0-9]+)*$/,
                            message: 'ID must contain only lowercase letters, numbers, and hyphens'
                          },
                          {
                            validator: async (_rule, value) => {
                              if (value && !template) {
                                try {
                                  await templatesApi.get({ workspace_id: workspace.id, id: value })
                                  return Promise.reject('Template ID already exists')
                                } catch (error) {
                                  return Promise.resolve()
                                }
                              }
                              return Promise.resolve()
                            }
                          }
                        ]}
                      >
                        <Input
                          disabled={template ? true : false}
                          placeholder="i.e: welcome-email"
                        />
                      </Form.Item>
                    </Col>
                    <Col span={8}>
                      <Form.Item
                        name="category"
                        label="Category"
                        rules={[{ required: true, type: 'string' }]}
                      >
                        <Select
                          placeholder="Select category"
                          disabled={forceCategory ? true : false}
                          options={[
                            {
                              value: 'marketing',
                              label: renderCategoryTag('marketing')
                            },
                            {
                              value: 'transactional',
                              label: renderCategoryTag('transactional')
                            },
                            {
                              value: 'welcome',
                              label: renderCategoryTag('welcome')
                            },
                            {
                              value: 'opt_in',
                              label: renderCategoryTag('opt_in')
                            },
                            {
                              value: 'unsubscribe',
                              label: renderCategoryTag('unsubscribe')
                            },
                            {
                              value: 'bounce',
                              label: renderCategoryTag('bounce')
                            },
                            {
                              value: 'blocklist',
                              label: renderCategoryTag('blocklist')
                            }
                          ]}
                        />
                      </Form.Item>
                    </Col>
                  </Row>

                  <div className="text-lg my-8 font-bold">Sender</div>
                  <Row gutter={24}>
                    <Col span={12}>
                      <Form.Item
                        name={['email', 'subject']}
                        label="Email subject"
                        rules={[{ required: true, type: 'string' }]}
                      >
                        <Input placeholder="Templating markup allowed" />
                      </Form.Item>
                      <Form.Item
                        name={['email', 'subject_preview']}
                        label="Subject preview"
                        rules={[{ required: true, type: 'string' }]}
                      >
                        <Input placeholder="Templating markup allowed" />
                      </Form.Item>

                      <Form.Item
                        name={['email', 'reply_to']}
                        label="Reply to"
                        rules={[{ required: false, type: 'email' }]}
                      >
                        <Input />
                      </Form.Item>

                      <Form.Item
                        name={['email', 'sender_id']}
                        label={`Custom sender (${
                          categoryValue === 'marketing' ? 'marketing' : 'transactional'
                        } email provider)`}
                        rules={[{ required: false, type: 'string' }]}
                      >
                        <Select
                          options={emailProvider?.email_provider.senders.map((sender) => ({
                            value: sender.id,
                            label: `${sender.name} <${sender.email}>`
                          }))}
                          allowClear={true}
                        />
                      </Form.Item>
                    </Col>
                    <Col span={12}>
                      <div className="flex justify-center">
                        <IphoneEmailPreview
                          sender={emailSender?.name || 'Sender Name'}
                          subject={emailSubject || 'Email Subject'}
                          previewText={emailPreview || 'Preview text will appear here...'}
                          timestamp="Now"
                          currentTime="12:12"
                        />
                      </div>
                    </Col>
                  </Row>
                </div>
              </div>

              <div style={{ display: tab === 'template' ? 'block' : 'none' }}>
                <Form.Item dependencies={['id']} style={{ margin: 0 }}>
                  {(form) => {
                    const testData = form.getFieldValue('test_data')

                    return (
                      <EmailBuilder
                        tree={visualEditorTree}
                        onTreeChange={setVisualEditorTree}
                        onCompile={async (tree: EmailBlock, testData?: any) => {
                          try {
                            const response = await templatesApi.compile({
                              workspace_id: workspace.id,
                              message_id: 'preview',
                              visual_editor_tree: tree as any,
                              test_data: testData || {}
                            })

                            if (response.error) {
                              return {
                                html: '',
                                mjml: response.mjml || '',
                                errors: [response.error]
                              }
                            }

                            return {
                              html: response.html || '',
                              mjml: response.mjml || '',
                              errors: []
                            }
                          } catch (error: any) {
                            console.error('Compilation error:', error)
                            return {
                              html: '',
                              mjml: '',
                              errors: [{ message: error.message || 'Compilation failed' }]
                            }
                          }
                        }}
                        testData={testData}
                        onTestDataChange={(newTestData) => {
                          form.setFieldsValue({
                            test_data: newTestData
                          })
                        }}
                        treePanelRef={treePanelRef as React.RefObject<HTMLDivElement>}
                        editPanelRef={editPanelRef as React.RefObject<HTMLDivElement>}
                        settingsPanelRef={settingsPanelRef as React.RefObject<HTMLDivElement>}
                        previewSwitcherRef={previewSwitcherRef as React.RefObject<HTMLDivElement>}
                        mobileDesktopSwitcherRef={
                          mobileDesktopSwitcherRef as React.RefObject<HTMLDivElement>
                        }
                        forcedViewMode={forcedViewMode}
                        savedBlocks={workspace.settings.template_blocks || []}
                        onSaveBlock={handleSaveBlock}
                        onSelectBlock={setSelectedBlockId}
                        selectedBlockId={selectedBlockId}
                        hiddenBlocks={['mj-title', 'mj-preview']}
                        toolbarActions={
                          <div className="flex gap-2 items-start">
                            <HelpSupportDropdown
                              onStartTour={() => {
                                setTourOpen(true)
                              }}
                            />
                            <div ref={importExportButtonRef}>
                              <ImportExportButton
                                onImport={handleImport}
                                // onTestDataImport={handleTestDataImport}
                                tree={visualEditorTree}
                                testData={testData}
                                workspaceId={workspace.id}
                              />
                            </div>
                          </div>
                        }
                      />
                    )
                  }}
                </Form.Item>
              </div>
            </div>
          </Form>
          <Tour
            open={tourOpen}
            onClose={() => {
              setTourOpen(false)
              // Reset forced view mode when tour closes
              setForcedViewMode(null)
              // Mark tour as seen
              localStorage.setItem('email-builder-tour-seen', 'true')
            }}
            onChange={(current) => {
              // Change email builder state based on tour step
              switch (current) {
                case 2: // Edit panel step (0-indexed)
                  // Select the body block to demonstrate block selection
                  const bodyBlock = visualEditorTree.children?.find(
                    (child) => child.type === 'mj-body'
                  )
                  if (bodyBlock) {
                    setSelectedBlockId(bodyBlock.id)
                  }
                  setForcedViewMode('edit')
                  break
                case 4: // Preview step (0-indexed)
                case 5: // Mobile/Desktop preview step
                  // Automatically switch to preview mode when reaching the preview steps
                  setForcedViewMode('preview')
                  break
                case 6: // Import/Export step
                  // Switch back to edit mode for import/export step
                  setForcedViewMode('edit')
                  break
                default:
                  // For other steps, ensure we're in edit mode
                  setForcedViewMode('edit')
                  break
              }
            }}
            steps={[
              {
                title: 'Welcome to Email Builder! 🎉',
                description:
                  "Let's take a quick tour to help you get started with building beautiful emails using MJML.",
                target: null // Center of screen
              },
              {
                title: 'Email Structure Tree',
                description:
                  'This is your email structure tree. You can drag and drop blocks to reorganize your email layout. Click the + buttons to add new blocks, or drag blocks from one section to another.',
                target: () => treePanelRef.current!,
                placement: 'right' as const
              },
              {
                title: 'Visual Email Editor',
                description:
                  'This is your visual email editor. Click on any element in your email to select it. Selected elements will be highlighted with a blue border and show editing options.',
                target: () => editPanelRef.current!,
                placement: 'top' as const
              },
              {
                title: 'Block Settings Panel',
                description:
                  'When you select a block, its settings appear here. Modify colors, text, spacing, alignment, and other properties to customize your email design.',
                target: () => settingsPanelRef.current!,
                placement: 'left' as const
              },
              {
                title: 'Preview Your Email',
                description:
                  'Switch to Preview mode to see how your email will look to recipients. This shows the final rendered version with all styling applied.',
                target: () => previewSwitcherRef.current!,
                placement: 'bottom' as const
              },
              {
                title: 'Mobile & Desktop Preview',
                description:
                  'Toggle between mobile and desktop views to see how your email appears on different devices. Mobile view shows a 400px width while desktop shows the full width.',
                target: () => mobileDesktopSwitcherRef.current!,
                placement: 'left' as const
              },
              {
                title: 'Import & Export Templates',
                description:
                  'Use this button to import saved email templates or export your finished emails. You can import JSON/MJML templates or export as HTML, MJML, or JSON for future use.',
                target: () => importExportButtonRef.current!,
                placement: 'bottom' as const
              }
            ]}
            indicatorsRender={(current, total) => (
              <span
                style={{
                  color: '#1890ff',
                  fontSize: '12px',
                  fontWeight: 'bold'
                }}
              >
                {current + 1} / {total}
              </span>
            )}
          />
        </Drawer>
      )}
    </>
  )
}
