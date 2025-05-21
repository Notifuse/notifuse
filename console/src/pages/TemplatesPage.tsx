import { useEffect, useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Typography,
  Button,
  Table,
  Tooltip,
  Space,
  Popconfirm,
  message,
  Segmented,
  Tag,
  TableColumnType
} from 'antd'
import { useParams, useSearch, useNavigate } from '@tanstack/react-router'
import { templatesApi } from '../services/api/template'
import type { Template, Workspace } from '../services/api/types'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import {
  faPenToSquare,
  faEye,
  faTrashCan,
  faPaperPlane,
  faCopy
} from '@fortawesome/free-regular-svg-icons'
import { CreateTemplateDrawer } from '../components/templates/CreateTemplateDrawer'
import { renderCategoryTag } from '../components/templates'
import { useAuth } from '../contexts/AuthContext'
import dayjs from '../lib/dayjs'
import TemplatePreviewDrawer from '../components/templates/TemplatePreviewDrawer'
import SendTemplateModal from '../components/templates/SendTemplateModal'

const { Title, Paragraph, Text } = Typography

// Define search params interface
interface TemplatesSearch {
  category?: string
}

export function TemplatesPage() {
  const { workspaceId } = useParams({ from: '/workspace/$workspaceId/templates' })
  // Use useSearch to get query params
  const search = useSearch({ from: '/workspace/$workspaceId/templates' }) as TemplatesSearch
  const navigate = useNavigate({ from: '/workspace/$workspaceId/templates' })
  const queryClient = useQueryClient()
  const { workspaces } = useAuth()
  const [workspace, setWorkspace] = useState<Workspace | null>(null)
  // Derive selectedCategory from search params, default to 'all'
  const selectedCategory = search.category || 'all'
  // Add state for the test template modal
  const [testModalOpen, setTestModalOpen] = useState(false)
  const [templateToTest, setTemplateToTest] = useState<Template | null>(null)

  // Function to update search params
  const setSelectedCategory = (category: string) => {
    navigate({
      search: (prev) => ({ ...prev, category: category === 'all' ? undefined : category })
    })
  }

  // Backend categories + All
  const categories = [
    { label: 'All', value: 'all' },
    { label: 'Marketing', value: 'marketing' },
    { label: 'Transactional', value: 'transactional' },
    { label: 'Welcome', value: 'welcome' },
    { label: 'Opt-in', value: 'opt_in' },
    { label: 'Unsubscribe', value: 'unsubscribe' },
    { label: 'Bounce', value: 'bounce' },
    { label: 'Blocklist', value: 'blocklist' },
    { label: 'Other', value: 'other' }
  ]

  // current workspace from workspaceId
  useEffect(() => {
    if (workspaces.length > 0) {
      const currentWorkspace = workspaces.find((w) => w.id === workspaceId)
      if (currentWorkspace) {
        setWorkspace(currentWorkspace)
      }
    }
  }, [workspaces, workspaceId])

  const { data, isLoading } = useQuery({
    // Use selectedCategory from search params in queryKey
    queryKey: ['templates', workspaceId, selectedCategory],
    queryFn: () => {
      const params: { workspace_id: string; category?: string } = {
        workspace_id: workspaceId
      }
      if (selectedCategory !== 'all') {
        params.category = selectedCategory
      }
      return templatesApi.list(params)
    }
  })

  const deleteMutation = useMutation({
    mutationFn: templatesApi.delete,
    onSuccess: () => {
      message.success('Template deleted successfully')
      // Use selectedCategory from search params in invalidation
      queryClient.invalidateQueries({ queryKey: ['templates', workspaceId, selectedCategory] })
    },
    onError: (error: any) => {
      const errorMsg = error?.response?.data?.error || error.message
      message.error(`Failed to delete template: ${errorMsg}`)
    }
  })

  const handleDelete = (templateId: string) => {
    deleteMutation.mutate({ workspace_id: workspaceId!, id: templateId })
  }

  const hasTemplates = !isLoading && data?.templates && data.templates.length > 0

  // Add function to handle testing a template
  const handleTestTemplate = (template: Template) => {
    setTemplateToTest(template)
    setTestModalOpen(true)
  }

  const marketingEmailProvider = workspace?.integrations?.find(
    (integration) => integration.id === workspace.settings.marketing_email_provider_id
  )
  const transactionalEmailProvider = workspace?.integrations?.find(
    (integration) => integration.id === workspace.settings.transactional_email_provider_id
  )

  if (!workspace) {
    return <div>Loading...</div>
  }

  const columns: TableColumnType<Template>[] = [
    {
      title: 'Template',
      dataIndex: 'name',
      key: 'name',
      render: (text: string, record: Template) => (
        <Tooltip title={'ID for API: ' + record.id}>
          <Text strong>{text}</Text>
        </Tooltip>
      )
    },
    {
      title: 'Category',
      dataIndex: 'category',
      key: 'category',
      render: (category: string) => renderCategoryTag(category)
    },
    {
      title: 'Sender',
      key: 'sender',
      render: (_: any, record: Template) => {
        if (workspace && record.email?.sender_id) {
          const isMarketing = record.category === 'marketing'
          const emailProvider = isMarketing ? marketingEmailProvider : transactionalEmailProvider
          if (emailProvider) {
            const sender = emailProvider.email_provider.senders.find(
              (sender) => sender.id === record.email?.sender_id
            )
            return `${sender?.name} <${sender?.email}>`
          }
        }
        return (
          <Tag bordered={false} color="blue">
            default
          </Tag>
        )
      }
    },
    {
      title: 'Subject',
      dataIndex: ['email', 'subject'],
      key: 'subject',
      render: (subject: string, record: Template) => (
        <div>
          <Text>{subject}</Text>
          {record.email?.subject_preview && (
            <div>
              <Text type="secondary" className="text-xs">
                {record.email.subject_preview}
              </Text>
            </div>
          )}
        </div>
      )
    },
    {
      title: 'Created',
      dataIndex: 'created_at',
      key: 'created_at',
      render: (date: string) => (
        <Tooltip
          title={
            dayjs(date).tz(workspace?.settings.timezone).format('llll') +
            ' in ' +
            workspace?.settings.timezone
          }
        >
          <span>{dayjs(date).format('ll')}</span>
        </Tooltip>
      )
    },
    {
      title: '',
      key: 'actions',
      render: (_: any, record: Template) => (
        <Space>
          {workspace && (
            <Tooltip title="Edit Template">
              <>
                <CreateTemplateDrawer
                  template={record}
                  workspace={workspace}
                  buttonContent={<FontAwesomeIcon icon={faPenToSquare} style={{ opacity: 0.7 }} />}
                  buttonProps={{ type: 'text', size: 'small' }}
                />
              </>
            </Tooltip>
          )}
          {workspace && (
            <Tooltip title="Clone Template">
              <>
                <CreateTemplateDrawer
                  fromTemplate={record}
                  workspace={workspace}
                  buttonContent={<FontAwesomeIcon icon={faCopy} style={{ opacity: 0.7 }} />}
                  buttonProps={{ type: 'text', size: 'small' }}
                />
              </>
            </Tooltip>
          )}
          <Tooltip title="Delete Template">
            <Popconfirm
              title="Delete the template?"
              description="Are you sure you want to delete this template? All versions will be deleted."
              onConfirm={() => handleDelete(record.id)}
              okText="Yes, Delete"
              cancelText="Cancel"
              placement="topRight"
            >
              <Button
                type="text"
                icon={<FontAwesomeIcon icon={faTrashCan} style={{ opacity: 0.7 }} />}
                loading={deleteMutation.isPending}
              />
            </Popconfirm>
          </Tooltip>
          <Tooltip title="Send Test Email">
            <Button
              type="text"
              icon={<FontAwesomeIcon icon={faPaperPlane} style={{ opacity: 0.7 }} />}
              onClick={() => handleTestTemplate(record)}
            />
          </Tooltip>
          <Tooltip title="Preview Template">
            <>
              <TemplatePreviewDrawer record={record} workspace={workspace}>
                <Button
                  type="text"
                  icon={<FontAwesomeIcon icon={faEye} style={{ opacity: 0.7 }} />}
                />
              </TemplatePreviewDrawer>
            </>
          </Tooltip>
        </Space>
      )
    }
  ]

  return (
    <div className="p-6">
      <div className="flex justify-between items-center mb-6">
        <div className="text-2xl font-medium">Templates</div>
        {workspace && data?.templates && data.templates.length > 0 && (
          <CreateTemplateDrawer workspace={workspace} />
        )}
      </div>

      <div className="mb-4">
        <Segmented
          options={categories}
          // Use selectedCategory from search params as value
          value={selectedCategory}
          // Update search params on change
          onChange={(value) => setSelectedCategory(value as string)}
        />
      </div>

      {isLoading ? (
        <Table columns={columns} dataSource={[]} loading={true} rowKey="id" />
      ) : hasTemplates ? (
        <Table
          columns={columns}
          dataSource={data.templates}
          rowKey="id"
          pagination={{ hideOnSinglePage: true }}
          className="border border-gray-200 rounded-md"
        />
      ) : (
        <div className="text-center py-12">
          {selectedCategory === 'all' ? (
            <>
              <Title level={4} type="secondary">
                No templates found
              </Title>
              <Paragraph type="secondary">Create your first template to get started</Paragraph>
              <div className="mt-4">
                {workspace && (
                  <CreateTemplateDrawer workspace={workspace} buttonProps={{ size: 'large' }} />
                )}
              </div>
            </>
          ) : (
            <>
              <Title level={4} type="secondary">
                No templates found for category "{selectedCategory}"
              </Title>
              <Paragraph type="secondary">
                Try selecting a different category or{' '}
                <Button type="link" onClick={() => setSelectedCategory('all')} className="p-0">
                  reset the filter
                </Button>
                .
              </Paragraph>
            </>
          )}
        </div>
      )}

      {/* Use the new SendTemplateModal component */}
      <SendTemplateModal
        isOpen={testModalOpen}
        onClose={() => setTestModalOpen(false)}
        template={templateToTest}
        workspace={workspace}
      />
    </div>
  )
}
