import React, { useState, useEffect } from 'react'
import { useLingui } from '@lingui/react/macro'
import { Input, Drawer, List, Empty, Spin, Button, Space } from 'antd'
import { EyeOutlined, SearchOutlined, PlusOutlined } from '@ant-design/icons'
import { useQuery } from '@tanstack/react-query'
import { templatesApi } from '../../services/api/template'
import type { Template, Workspace } from '../../services/api/types'
import TemplatePreviewPopover from './TemplatePreviewDrawer'
import { CreateTemplateDrawer } from './CreateTemplateDrawer'
import { useAuth } from '../../contexts/AuthContext'

interface TemplateSelectorInputProps {
  value?: string | null
  onChange?: (value: string | null) => void
  workspaceId: string
  category?:
    | 'marketing'
    | 'transactional'
    | 'welcome'
    | 'opt_in'
    | 'unsubscribe'
    | 'bounce'
    | 'blocklist'
    | 'other'
  placeholder?: string
  clearable?: boolean
  disabled?: boolean
}

const TemplateSelectorInput: React.FC<TemplateSelectorInputProps> = ({
  value,
  onChange,
  workspaceId,
  category,
  placeholder,
  clearable = true,
  disabled = false
}) => {
  const { t } = useLingui()
  const defaultPlaceholder = placeholder || t`Select a template`
  const [open, setOpen] = useState<boolean>(false)
  const [selectedTemplate, setSelectedTemplate] = useState<Template | null>(null)
  const [searchQuery, setSearchQuery] = useState<string>('')
  const { workspaces } = useAuth()

  // Find the current workspace from the workspaces array
  const currentWorkspace = workspaces.find((workspace) => workspace.id === workspaceId)

  // Fetch templates with optional category filter
  const {
    data: templatesResponse,
    isLoading,
    refetch
  } = useQuery({
    queryKey: ['templates', workspaceId, category],
    queryFn: async () => {
      // Assume the API accepts a category parameter for filtering
      const response = await templatesApi.list({
        workspace_id: workspaceId,
        category: category,
        channel: 'email'
      })
      return response
    },
    enabled: !!workspaceId
  })

  // Fetch selected template details if we only have the ID
  useEffect(() => {
    if (value && workspaceId && !selectedTemplate) {
      // Fetch template details using the value (template ID)
      templatesApi
        .get({ workspace_id: workspaceId, id: value })
        .then((response) => {
          if (response.template) {
            setSelectedTemplate(response.template)
          }
        })
        .catch((error) => {
          console.error('Failed to fetch template details:', error)
        })
    }
  }, [value, workspaceId, selectedTemplate])

  // Get templates array from response
  const templates = templatesResponse?.templates || []

  // Filter templates based on search query
  const filteredTemplates = templates.filter((template) =>
    template.name.toLowerCase().includes(searchQuery.toLowerCase())
  )

  const handleSelect = (template: Template) => {
    setSelectedTemplate(template)
    onChange?.(template.id)
    setOpen(false)
  }

  const showDrawer = () => {
    if (!disabled) {
      setOpen(true)
    }
  }

  const onClose = () => {
    setOpen(false)
    setSearchQuery('')
  }

  // Handle template creation complete - refetch templates and select the new one
  const handleTemplateCreated = async () => {
    await refetch()
    // Templates will be refetched, wait for the drawer to close before refetching
    setTimeout(() => {
      setOpen(true) // Reopen the template selection drawer
    }, 500)
  }

  // Handle clone template complete - same as handleTemplateCreated
  const handleTemplateCloned = async () => {
    await refetch()
    // Templates will be refetched, wait for the drawer to close before refetching
    setTimeout(() => {
      setOpen(true) // Reopen the template selection drawer
    }, 500)
  }

  if (!currentWorkspace) {
    return <div>{t`Loading...`}</div>
  }

  return (
    <>
      <Space.Compact style={{ width: '100%' }}>
        <Input
          value={selectedTemplate?.name || ''}
          placeholder={defaultPlaceholder}
          readOnly={!clearable}
          disabled={disabled}
          onClick={showDrawer}
          onClear={() => {
            setSelectedTemplate(null)
            onChange?.(null)
          }}
          allowClear={clearable}
          style={{ flex: 1 }}
        />
        {selectedTemplate && currentWorkspace && (
          <TemplatePreviewPopover record={selectedTemplate} workspace={currentWorkspace}>
            <Button icon={<EyeOutlined />} />
          </TemplatePreviewPopover>
        )}
      </Space.Compact>

      <Drawer
        title={t`Select Template`}
        width={600}
        onClose={onClose}
        open={open}
        styles={{
          body: { paddingBottom: 80 }
        }}
      >
        <div style={{ marginBottom: 16, display: 'flex', gap: 8 }}>
          <Input
            placeholder={t`Search templates...`}
            prefix={<SearchOutlined />}
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            style={{ flex: 1 }}
          />
          {currentWorkspace && (
            <CreateTemplateDrawer
              workspace={currentWorkspace}
              forceCategory={category}
              buttonProps={{
                type: 'primary',
                icon: <PlusOutlined />,
                children: null
              }}
              onClose={handleTemplateCreated}
            />
          )}
        </div>

        {isLoading ? (
          <div style={{ textAlign: 'center', padding: '40px 0' }}>
            <Spin size="large" />
          </div>
        ) : filteredTemplates.length > 0 ? (
          <List
            itemLayout="horizontal"
            bordered
            dataSource={filteredTemplates}
            size="small"
            renderItem={(template) => (
              <List.Item
                actions={[
                  <TemplatePreviewPopover
                    key="preview"
                    record={template}
                    workspace={currentWorkspace as Workspace}
                  >
                    <Button type="text" icon={<EyeOutlined />} />
                  </TemplatePreviewPopover>,
                  currentWorkspace && (
                    <CreateTemplateDrawer
                      key="clone"
                      workspace={currentWorkspace}
                      fromTemplate={template}
                      forceCategory={category}
                      buttonProps={{
                        type: 'link',
                        title: t`Clone`
                      }}
                      buttonContent={t`Clone`}
                      onClose={handleTemplateCloned}
                    />
                  ),
                  <Button key="select" type="link" onClick={() => handleSelect(template)}>
                    {t`Select`}
                  </Button>
                ]}
              >
                <List.Item.Meta
                  title={
                    <a onClick={() => handleSelect(template)} style={{ cursor: 'pointer' }}>
                      {template.name}
                    </a>
                  }
                  description={template.category || t`No category`}
                />
              </List.Item>
            )}
          />
        ) : (
          <Empty
            description={
              category
                ? t`No templates found for ${category.replace('_', ' ')} category`
                : t`No templates found`
            }
            image={Empty.PRESENTED_IMAGE_SIMPLE}
          >
            {currentWorkspace && (
              <CreateTemplateDrawer
                workspace={currentWorkspace}
                forceCategory={category}
                buttonProps={{
                  type: 'primary',
                  icon: <PlusOutlined />,
                  children: category
                    ? t`Create New ${category.charAt(0).toUpperCase() + category.slice(1).replace('_', ' ')} Template`
                    : t`Create New Template`
                }}
                onClose={handleTemplateCreated}
              />
            )}
          </Empty>
        )}
      </Drawer>
    </>
  )
}

export default TemplateSelectorInput
