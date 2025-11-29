import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Typography,
  Space,
  Tooltip,
  Button,
  message,
  Table,
  Tag,
  Popconfirm
} from 'antd'
import { useParams } from '@tanstack/react-router'
import {
  transactionalNotificationsApi,
  TransactionalNotification,
  ChannelTemplates
} from '../services/api/transactional_notifications'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import {
  faPenToSquare,
  faTrashCan,
  faEnvelope,
  faPaperPlane,
  faEye
} from '@fortawesome/free-regular-svg-icons'
import { faTerminal } from '@fortawesome/free-solid-svg-icons'
import UpsertTransactionalNotificationDrawer from '../components/transactional/UpsertTransactionalNotificationDrawer'
import { useState } from 'react'
import dayjs from '../lib/dayjs'
import { useAuth, useWorkspacePermissions } from '../contexts/AuthContext'
import SendTemplateModal from '../components/templates/SendTemplateModal'
import TemplatePreviewDrawer from '../components/templates/TemplatePreviewDrawer'
import { templatesApi } from '../services/api/template'
import { Workspace } from '../services/api/types'
import { ApiCommandModal } from '../components/transactional/ApiCommandModal'

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

// Template preview component
const TemplatePreview: React.FC<{ templateId: string; workspace: Workspace }> = ({
  templateId,
  workspace
}) => {
  const { data: templateData } = useQuery({
    queryKey: ['template', workspace.id, templateId],
    queryFn: () => templatesApi.get({ workspace_id: workspace.id, id: templateId }),
    enabled: !!workspace.id && !!templateId
  })

  if (!templateData?.template) {
    return null
  }

  return (
    <TemplatePreviewDrawer record={templateData.template} workspace={workspace}>
      <Tooltip title="Preview template">
        <Button type="text" size="small" className="ml-2">
          <FontAwesomeIcon icon={faEye} style={{ opacity: 0.7 }} />
        </Button>
      </Tooltip>
    </TemplatePreviewDrawer>
  )
}

// Component for rendering channels
const ChannelsList: React.FC<{ channels: ChannelTemplates; workspace: Workspace }> = ({
  channels,
  workspace
}) => {
  return (
    <Space direction="vertical" size="small">
      {channels.email && (
        <div className="flex items-center justify-between w-full">
          <Tag bordered={false} color="blue">
            <FontAwesomeIcon icon={faEnvelope} style={{ opacity: 0.7 }} /> Email
          </Tag>
          {channels.email.template_id && workspace.id && (
            <TemplatePreview templateId={channels.email.template_id} workspace={workspace} />
          )}
        </div>
      )}
      {/* Add more channel types here as they become available */}
    </Space>
  )
}

export function TransactionalNotificationsPage() {
  const { workspaceId } = useParams({ strict: false })
  const { workspaces } = useAuth()
  const { permissions } = useWorkspacePermissions(workspaceId as string)
  const queryClient = useQueryClient()

  // Find the current workspace from the workspaces array
  const currentWorkspace = workspaces.find((workspace) => workspace.id === workspaceId)

  const [testModalOpen, setTestModalOpen] = useState(false)
  const [apiModalOpen, setApiModalOpen] = useState(false)
  const [currentApiNotification, setCurrentApiNotification] =
    useState<TransactionalNotification | null>(null)
  const [notificationToTest, setNotificationToTest] = useState<TransactionalNotification | null>(
    null
  )

  // Removed usePrismjs hook call

  // Fetch notifications
  const {
    data: notificationsData,
    isLoading: isLoadingNotifications,
    error: notificationsError
  } = useQuery({
    queryKey: ['transactional-notifications', workspaceId],
    queryFn: () =>
      transactionalNotificationsApi.list({
        workspace_id: workspaceId as string
      }),
    enabled: !!workspaceId
  })

  const handleDeleteNotification = async (notification: TransactionalNotification) => {
    try {
      await transactionalNotificationsApi.delete({
        workspace_id: workspaceId as string,
        id: notification.id
      })

      message.success('Transactional notification deleted successfully')

      // Refresh the list
      queryClient.invalidateQueries({ queryKey: ['transactional-notifications', workspaceId] })
    } catch (error) {
      console.error('Failed to delete notification:', error)
      message.error('Failed to delete notification')
    }
  }

  const handleTestNotification = (notification: TransactionalNotification) => {
    setNotificationToTest(notification)
    setTestModalOpen(true)
  }

  const handleShowApiModal = (notification: TransactionalNotification) => {
    setCurrentApiNotification(notification)
    setApiModalOpen(true)
  }

  if (notificationsError) {
    return (
      <div>
        <Title level={4}>Error loading data</Title>
        <Text type="danger">{(notificationsError as Error)?.message}</Text>
      </div>
    )
  }

  const notifications = notificationsData?.notifications || []
  const hasNotifications = notifications.length > 0

  if (!currentWorkspace) {
    return <div>Loading...</div>
  }

  const canDelete = (notification: TransactionalNotification) => {
    return permissions?.transactional?.write && !notification.integration_id
  }

  const canWrite = permissions?.transactional?.write

  const columns = [
    {
      title: 'Name / ID',
      dataIndex: 'name',
      key: 'name',
      render: (text: string, record: TransactionalNotification) => {
        const integration = currentWorkspace?.integrations?.find(
          (i) => i.id === record.integration_id
        )
        return (
          <Space size="large">
            {record.integration_id && integration && (
              <Tooltip title={`Managed by ${integration.name} (${integration.type} integration)`}>
                {getIntegrationIcon(integration.type)}
              </Tooltip>
            )}
            <div>
              <div className="font-bold">{text}</div>
              <div className=" text-gray-500">{record.id}</div>
            </div>
          </Space>
        )
      }
    },
    {
      title: 'Description',
      dataIndex: 'description',
      key: 'description',
      render: (text: string) => <Text ellipsis>{text}</Text>
    },
    {
      title: 'Channels',
      dataIndex: 'channels',
      key: 'channels',
      render: (channels: ChannelTemplates) => (
        <ChannelsList channels={channels} workspace={currentWorkspace} />
      )
    },
    {
      title: 'Created',
      dataIndex: 'created_at',
      key: 'created_at',
      render: (date: string) => (
        <Tooltip
          title={
            dayjs(date).tz(currentWorkspace?.settings.timezone).format('llll') +
            ' in ' +
            currentWorkspace?.settings.timezone
          }
        >
          <span>{dayjs(date).format('ll')}</span>
        </Tooltip>
      )
    },
    {
      title: '',
      key: 'actions',
      width: 100,
      align: 'right' as const,
      render: (_: unknown, record: TransactionalNotification) => (
        <Space>
          {canDelete(record) && (
            <>
              <Popconfirm
                title="Delete the notification?"
                description="Are you sure you want to delete this notification? This cannot be undone."
                onConfirm={() => handleDeleteNotification(record)}
                okText="Yes, Delete"
                cancelText="Cancel"
                placement="topRight"
                disabled={!!record.integration_id}
              >
                <Button
                  type="text"
                  size="small"
                  disabled={!permissions?.transactional?.write || !!record.integration_id}
                >
                  <FontAwesomeIcon icon={faTrashCan} style={{ opacity: 0.7 }} />
                </Button>
              </Popconfirm>
            </>
          )}
          {canWrite && currentWorkspace?.id && (
            <Tooltip title={'Edit'}>
              <>
                <UpsertTransactionalNotificationDrawer
                  workspace={currentWorkspace}
                  notification={record}
                  buttonContent={<FontAwesomeIcon icon={faPenToSquare} style={{ opacity: 0.7 }} />}
                  buttonProps={{ type: 'text', size: 'small', disabled: !canWrite }}
                />
              </>
            </Tooltip>
          )}
          {canWrite && (
            <Tooltip title={'Test'}>
              <Button
                type="text"
                size="small"
                onClick={() => handleTestNotification(record)}
                disabled={!canWrite}
              >
                <FontAwesomeIcon icon={faPaperPlane} style={{ opacity: 0.7 }} />
              </Button>
            </Tooltip>
          )}
          <Tooltip title={'API Command'}>
            <Button
              type="text"
              size="small"
              onClick={() => handleShowApiModal(record)}
              disabled={!canWrite}
            >
              <FontAwesomeIcon icon={faTerminal} style={{ opacity: 0.7 }} />
            </Button>
          </Tooltip>
        </Space>
      )
    }
  ]

  return (
    <div className="p-6">
      <div className="flex justify-between items-center mb-6">
        <div className="text-2xl font-medium">Transactional Notifications</div>
        {currentWorkspace && hasNotifications && (
          <Tooltip
            title={
              !permissions?.transactional?.write
                ? "You don't have write permission for transactional notifications"
                : undefined
            }
          >
            <div>
              <UpsertTransactionalNotificationDrawer
                workspace={currentWorkspace}
                buttonContent={'Create Notification'}
                buttonProps={{
                  type: 'primary',
                  disabled: !permissions?.transactional?.write
                }}
              />
            </div>
          </Tooltip>
        )}
      </div>

      {isLoadingNotifications ? (
        <Table columns={columns} dataSource={[]} loading={true} rowKey="id" />
      ) : hasNotifications ? (
        <Table
          columns={columns}
          dataSource={notifications}
          rowKey="id"
          pagination={{ hideOnSinglePage: true }}
          className="border border-gray-200 rounded-md"
        />
      ) : (
        <div className="text-center py-12">
          <Title level={4} type="secondary">
            No transactional notifications found
          </Title>
          <Paragraph type="secondary">Create your first notification to get started</Paragraph>
          <div className="mt-4">
            {currentWorkspace && (
              <Tooltip
                title={
                  !permissions?.transactional?.write
                    ? "You don't have write permission for transactional notifications"
                    : undefined
                }
              >
                <div>
                  <UpsertTransactionalNotificationDrawer
                    workspace={currentWorkspace}
                    buttonContent="Create Notification"
                    buttonProps={{
                      type: 'primary',
                      disabled: !permissions?.transactional?.write
                    }}
                  />
                </div>
              </Tooltip>
            )}
          </div>
        </div>
      )}

      {/* API Command Modal */}
      <ApiCommandModal
        open={apiModalOpen}
        onClose={() => setApiModalOpen(false)}
        notification={currentApiNotification}
        workspaceId={workspaceId as string}
      />

      {/* Use SendTemplateModal for testing */}
      {notificationToTest?.channels?.email?.template_id && (
        <SendTemplateModal
          isOpen={testModalOpen}
          onClose={() => setTestModalOpen(false)}
          template={
            {
              id: notificationToTest.channels.email.template_id,
              name: notificationToTest.name,
              version: 0,
              category: 'transactional',
              channel: 'email',
              created_at: '',
              updated_at: ''
            }
          }
          workspace={currentWorkspace || null}
          withCCAndBCC={true}
        />
      )}
    </div>
  )
}
