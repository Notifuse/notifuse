import { useEffect, useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Typography,
  Button,
  Table,
  Tooltip,
  Space,
  Modal,
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
import { faTerminal } from '@fortawesome/free-solid-svg-icons'
import { CreateTemplateDrawer } from '../components/templates/CreateTemplateDrawer'
import { renderCategoryTag } from '../components/templates'
import { useAuth, useWorkspacePermissions } from '../contexts/AuthContext'
import dayjs from '../lib/dayjs'
import TemplatePreviewDrawer from '../components/templates/TemplatePreviewDrawer'
import SendTemplateModal from '../components/templates/SendTemplateModal'
import { useLingui } from '@lingui/react/macro'

const { Title, Paragraph, Text } = Typography

// Helper function to get integration icon
const getIntegrationIcon = (integrationType: string) => {
  switch (integrationType) {
    case 'supabase':
      return <img src="/console/supabase.png" alt="Supabase" className="h-3" />
    default:
      return <FontAwesomeIcon icon={faTerminal} className="text-gray-600" />
  }
}

// Define search params interface
interface TemplatesSearch {
  category?: string
}

export function TemplatesPage() {
  const { t } = useLingui()
  const { workspaceId } = useParams({ from: '/console/workspace/$workspaceId/templates' })
  // Use useSearch to get query params
  const search = useSearch({ from: '/console/workspace/$workspaceId/templates' }) as TemplatesSearch
  const navigate = useNavigate({ from: '/console/workspace/$workspaceId/templates' })
  const queryClient = useQueryClient()
  const { workspaces } = useAuth()
  const { permissions } = useWorkspacePermissions(workspaceId)
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
    { label: t`All`, value: 'all' },
    { label: t`Marketing`, value: 'marketing' },
    { label: t`Transactional`, value: 'transactional' },
    { label: t`Welcome`, value: 'welcome' },
    { label: t`Opt-in`, value: 'opt_in' },
    { label: t`Unsubscribe`, value: 'unsubscribe' },
    { label: t`Bounce`, value: 'bounce' },
    { label: t`Blocklist`, value: 'blocklist' },
    { label: t`Other`, value: 'other' }
  ]

  // current workspace from workspaceId
  useEffect(() => {
    if (workspaces.length > 0) {
      const currentWorkspace = workspaces.find((w) => w.id === workspaceId)
      if (currentWorkspace) {
        // eslint-disable-next-line react-hooks/set-state-in-effect
        setWorkspace(currentWorkspace)
      }
    }
  }, [workspaces, workspaceId])

  const { data, isLoading } = useQuery({
    // Use selectedCategory from search params in queryKey
    queryKey: ['templates', workspaceId, selectedCategory],
    queryFn: () => {
      const params: { workspace_id: string; category?: string; channel?: string } = {
        workspace_id: workspaceId,
        channel: 'email'
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
      message.success(t`Template deleted successfully`)
      // Use selectedCategory from search params in invalidation
      queryClient.invalidateQueries({ queryKey: ['templates', workspaceId, selectedCategory] })
    },
    onError: (error: Error & { response?: { data?: { error?: string } } }) => {
      const errorMsg = error?.response?.data?.error || error.message
      message.error(t`Failed to delete template: ${errorMsg}`)
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
    return <div>{t`Loading...`}</div>
  }

  const columns: TableColumnType<Template>[] = [
    {
      title: t`Template`,
      dataIndex: 'name',
      key: 'name',
      render: (text: string, record: Template) => {
        const integration = workspace?.integrations?.find((i) => i.id === record.integration_id)
        return (
          <Space size="large">
            {record.integration_id && integration && (
              <Tooltip title={`Managed by ${integration.name} (${integration.type} integration)`}>
                {getIntegrationIcon(integration.type)}
              </Tooltip>
            )}
            <Tooltip title={t`ID for API:` + ' ' + record.id}>
              <Text strong>{text}</Text>
            </Tooltip>
          </Space>
        )
      }
    },
    {
      title: t`Category`,
      dataIndex: 'category',
      key: 'category',
      render: (category: string) => renderCategoryTag(category)
    },
    {
      title: t`Sender`,
      key: 'sender',
      render: (_: unknown, record: Template) => {
        if (workspace && record.email?.sender_id) {
          const isMarketing = record.category === 'marketing'
          const emailProvider = isMarketing ? marketingEmailProvider : transactionalEmailProvider
          if (emailProvider?.email_provider) {
            const sender = emailProvider.email_provider.senders.find(
              (sender) => sender.id === record.email?.sender_id
            )
            return `${sender?.name} <${sender?.email}>`
          }
        }
        return (
          <Tag bordered={false} color="blue">
            {t`default`}
          </Tag>
        )
      }
    },
    {
      title: t`Subject`,
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
      title: t`Created`,
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
      render: (_: unknown, record: Template) => (
        <Space>
          {workspace && (
            <Tooltip
              title={
                !permissions?.templates?.write
                  ? t`You don't have write permission for templates`
                  : t`Edit Template`
              }
            >
              <div>
                <CreateTemplateDrawer
                  template={record}
                  workspace={workspace}
                  buttonContent={<FontAwesomeIcon icon={faPenToSquare} style={{ opacity: 0.7 }} />}
                  buttonProps={{
                    type: 'text',
                    size: 'small',
                    disabled: !permissions?.templates?.write
                  }}
                />
              </div>
            </Tooltip>
          )}
          {workspace && (
            <Tooltip
              title={
                !permissions?.templates?.write
                  ? t`You don't have write permission for templates`
                  : t`Clone Template`
              }
            >
              <div>
                <CreateTemplateDrawer
                  fromTemplate={record}
                  workspace={workspace}
                  buttonContent={<FontAwesomeIcon icon={faCopy} style={{ opacity: 0.7 }} />}
                  buttonProps={{
                    type: 'text',
                    size: 'small',
                    disabled: !permissions?.templates?.write
                  }}
                />
              </div>
            </Tooltip>
          )}
          <Tooltip
            title={
              record.integration_id
                ? t`This template is managed by an integration and cannot be deleted`
                : !permissions?.templates?.write
                  ? t`You don't have write permission for templates`
                  : t`Delete Template`
            }
          >
            <Button
              type="text"
              icon={<FontAwesomeIcon icon={faTrashCan} style={{ opacity: 0.7 }} />}
              loading={deleteMutation.isPending}
              disabled={!permissions?.templates?.write || !!record.integration_id}
              onClick={() => {
                Modal.confirm({
                  title: t`Delete template?`,
                  content: (
                    <div>
                      <p>{t`Are you sure you want to delete this template?`}</p>
                      <p className="mt-2 text-gray-600">
                        {t`Note: The template will be hidden from your workspace but preserved to maintain the ability to preview previously sent broadcasts and messages that used this template.`}
                      </p>
                    </div>
                  ),
                  okText: t`Yes, Delete`,
                  okType: 'danger',
                  cancelText: t`Cancel`,
                  onOk: () => handleDelete(record.id)
                })
              }}
            />
          </Tooltip>
          <Tooltip
            title={
              !(permissions?.templates?.read && permissions?.contacts?.write)
                ? t`You need read template and write contact permissions to send test emails`
                : t`Send Test Email`
            }
          >
            <Button
              type="text"
              icon={<FontAwesomeIcon icon={faPaperPlane} style={{ opacity: 0.7 }} />}
              onClick={() => handleTestTemplate(record)}
              disabled={!(permissions?.templates?.read && permissions?.contacts?.write)}
            />
          </Tooltip>
          <Tooltip title={t`Preview Template`}>
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
        <div className="text-2xl font-medium">{t`Templates`}</div>
        {workspace && data?.templates && data.templates.length > 0 && (
          <Tooltip
            title={
              !permissions?.templates?.write
                ? t`You don't have write permission for templates`
                : undefined
            }
          >
            <div>
              <CreateTemplateDrawer
                workspace={workspace}
                buttonProps={{
                  disabled: !permissions?.templates?.write
                }}
              />
            </div>
          </Tooltip>
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
                {t`No templates found`}
              </Title>
              <Paragraph type="secondary">{t`Create your first template to get started`}</Paragraph>
              <div className="mt-4">
                {workspace && (
                  <Tooltip
                    title={
                      !permissions?.templates?.write
                        ? "You don't have write permission for templates"
                        : undefined
                    }
                  >
                    <div>
                      <CreateTemplateDrawer
                        workspace={workspace}
                        buttonProps={{
                          size: 'large',
                          disabled: !permissions?.templates?.write
                        }}
                      />
                    </div>
                  </Tooltip>
                )}
              </div>
            </>
          ) : (
            <>
              <Title level={4} type="secondary">
                {t`No templates found for category "${selectedCategory}"`}
              </Title>
              <Paragraph type="secondary">
                {t`Try selecting a different category or`}{' '}
                <Button type="link" onClick={() => setSelectedCategory('all')} className="p-0">
                  {t`reset the filter`}
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
