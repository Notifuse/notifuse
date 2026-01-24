import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Typography,
  Space,
  Tooltip,
  Button,
  message,
  Popconfirm,
  Card,
  Statistic,
  Row,
  Col,
  Spin,
  Segmented,
  Descriptions,
  Divider
} from 'antd'
import { useParams } from '@tanstack/react-router'
import {
  transactionalNotificationsApi,
  TransactionalNotification
} from '../services/api/transactional_notifications'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import {
  faPenToSquare,
  faTrashCan,
  faPaperPlane,
  faEye,
  faCircleCheck,
  faCircleXmark
} from '@fortawesome/free-regular-svg-icons'
import { faTerminal, faTriangleExclamation } from '@fortawesome/free-solid-svg-icons'
import UpsertTransactionalNotificationDrawer from '../components/transactional/UpsertTransactionalNotificationDrawer'
import { useState } from 'react'
import dayjs from '../lib/dayjs'
import { useAuth, useWorkspacePermissions } from '../contexts/AuthContext'
import SendTemplateModal from '../components/templates/SendTemplateModal'
import TemplatePreviewDrawer from '../components/templates/TemplatePreviewDrawer'
import { templatesApi } from '../services/api/template'
import { Workspace, UserPermissions } from '../services/api/types'
import { ApiCommandModal } from '../components/transactional/ApiCommandModal'
import { analyticsService } from '../services/api/analytics'

const { Title, Paragraph } = Typography

// Helper function to get integration icon
const getIntegrationIcon = (integrationType: string) => {
  switch (integrationType) {
    case 'supabase':
      return <img src="/console/supabase.png" alt="Supabase" className="h-3" />
    default:
      return <FontAwesomeIcon icon={faTerminal} className="text-gray-600" />
  }
}

// Helper for rate calculation
const getRate = (numerator: number, denominator: number) => {
  if (denominator === 0) return '-'
  const percentage = (numerator / denominator) * 100
  if (percentage === 0 || percentage >= 10) return `${Math.round(percentage)}%`
  return `${percentage.toFixed(1)}%`
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


// Stats type
interface NotificationStats {
  sent: number
  delivered: number
  failed: number
  bounced: number
}

// Card component for each transactional notification
const TransactionalNotificationCard: React.FC<{
  notification: TransactionalNotification
  workspace: Workspace
  permissions: UserPermissions | undefined
  stats: NotificationStats
  isLoadingStats: boolean
  onDelete: (n: TransactionalNotification) => void
  onTest: (n: TransactionalNotification) => void
  onShowApi: (n: TransactionalNotification) => void
}> = ({
  notification,
  workspace,
  permissions,
  stats,
  isLoadingStats,
  onDelete,
  onTest,
  onShowApi
}) => {
  const integration = workspace.integrations?.find((i) => i.id === notification.integration_id)
  const canDelete = permissions?.transactional?.write && !notification.integration_id
  const canWrite = permissions?.transactional?.write

  return (
    <Card
      className="!mb-6"
      title={
        <Space size="large">
          {integration && (
            <Tooltip title={`Managed by ${integration.name} (${integration.type} integration)`}>
              {getIntegrationIcon(integration.type)}
            </Tooltip>
          )}
          <div>{notification.name}</div>
        </Space>
      }
      extra={
        <Space>
          {canDelete && (
            <Popconfirm
              title="Delete the notification?"
              description="This cannot be undone."
              onConfirm={() => onDelete(notification)}
              okText="Yes, Delete"
              cancelText="Cancel"
            >
              <Button type="text" size="small">
                <FontAwesomeIcon icon={faTrashCan} style={{ opacity: 0.7 }} />
              </Button>
            </Popconfirm>
          )}
          {canWrite && (
            <Tooltip title="Edit">
              <span>
                <UpsertTransactionalNotificationDrawer
                  workspace={workspace}
                  notification={notification}
                  buttonContent={<FontAwesomeIcon icon={faPenToSquare} style={{ opacity: 0.7 }} />}
                  buttonProps={{ type: 'text', size: 'small' }}
                />
              </span>
            </Tooltip>
          )}
          {notification.channels?.email?.template_id && (
            <TemplatePreview
              templateId={notification.channels.email.template_id}
              workspace={workspace}
            />
          )}
          {canWrite && (
            <Tooltip title="Test">
              <Button type="text" size="small" onClick={() => onTest(notification)}>
                <FontAwesomeIcon icon={faPaperPlane} style={{ opacity: 0.7 }} />
              </Button>
            </Tooltip>
          )}
          <Tooltip title="API Command">
            <Button type="text" size="small" onClick={() => onShowApi(notification)}>
              <FontAwesomeIcon icon={faTerminal} style={{ opacity: 0.7 }} />
            </Button>
          </Tooltip>
        </Space>
      }
    >
      {/* Stats Row - Delivery-focused metrics */}
      <Row gutter={[16, 16]}>
        <Col span={6}>
          <Statistic
            title={
              <Space className="font-medium">
                <FontAwesomeIcon
                  icon={faPaperPlane}
                  style={{ opacity: 0.7 }}
                  className="text-blue-500"
                />{' '}
                Sent
              </Space>
            }
            value={isLoadingStats ? '-' : stats.sent}
            valueStyle={{ fontSize: '16px' }}
            prefix={isLoadingStats ? <Spin size="small" /> : undefined}
          />
        </Col>
        <Col span={6}>
          <Statistic
            title={
              <Space className="font-medium">
                <FontAwesomeIcon
                  icon={faCircleCheck}
                  style={{ opacity: 0.7 }}
                  className="text-green-500"
                />{' '}
                Delivered
              </Space>
            }
            value={isLoadingStats ? '-' : getRate(stats.delivered, stats.sent)}
            valueStyle={{ fontSize: '16px' }}
            prefix={isLoadingStats ? <Spin size="small" /> : undefined}
          />
        </Col>
        <Col span={6}>
          <Statistic
            title={
              <Space className="font-medium">
                <FontAwesomeIcon
                  icon={faCircleXmark}
                  style={{ opacity: 0.7 }}
                  className="text-red-500"
                />{' '}
                Failed
              </Space>
            }
            value={isLoadingStats ? '-' : stats.failed}
            valueStyle={{ fontSize: '16px' }}
            prefix={isLoadingStats ? <Spin size="small" /> : undefined}
          />
        </Col>
        <Col span={6}>
          <Statistic
            title={
              <Space className="font-medium">
                <FontAwesomeIcon
                  icon={faTriangleExclamation}
                  style={{ opacity: 0.7 }}
                  className="text-orange-500"
                />{' '}
                Bounced
              </Space>
            }
            value={isLoadingStats ? '-' : stats.bounced}
            valueStyle={{ fontSize: '16px' }}
            prefix={isLoadingStats ? <Spin size="small" /> : undefined}
          />
        </Col>
      </Row>

      <Divider />

      <Descriptions size="small" column={2}>
        <Descriptions.Item label="ID">{notification.id}</Descriptions.Item>
        {notification.description && (
          <Descriptions.Item label="Description">{notification.description}</Descriptions.Item>
        )}
      </Descriptions>
    </Card>
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
  const [statsPeriod, setStatsPeriod] = useState<'7D' | '30D' | '60D'>('30D')

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

  // Get days from period
  const periodDays = statsPeriod === '7D' ? 7 : statsPeriod === '30D' ? 30 : 60

  // Fetch delivery-focused stats grouped by external_id
  const { data: statsData, isLoading: isLoadingStats } = useQuery({
    queryKey: ['transactional-stats', workspaceId, statsPeriod],
    queryFn: () =>
      analyticsService.query(
        {
          schema: 'message_history',
          measures: ['count_sent', 'count_delivered', 'count_failed', 'count_bounced'],
          dimensions: ['external_id'],
          filters: [
            { member: 'broadcast_id', operator: 'notSet', values: [] },
            {
              member: 'sent_at',
              operator: 'gte',
              values: [dayjs().subtract(periodDays, 'days').toISOString()]
            }
          ]
        },
        workspaceId as string
      ),
    enabled: !!workspaceId
  })

  // Helper to get stats for a notification
  const getStatsForNotification = (notificationId: string): NotificationStats => {
    const row = statsData?.data?.find((d) => d.external_id === notificationId)
    return {
      sent: Number(row?.count_sent || 0),
      delivered: Number(row?.count_delivered || 0),
      failed: Number(row?.count_failed || 0),
      bounced: Number(row?.count_bounced || 0)
    }
  }

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
        <Paragraph type="danger">{(notificationsError as Error)?.message}</Paragraph>
      </div>
    )
  }

  const notifications = notificationsData?.notifications || []
  const hasNotifications = notifications.length > 0

  if (!currentWorkspace) {
    return <div>Loading...</div>
  }

  return (
    <div className="p-6">
      <div className="flex justify-between items-center mb-6">
        <div className="text-2xl font-medium">Transactional Notifications</div>
        {currentWorkspace && hasNotifications && (
          <Space size="middle">
            <Segmented
              options={['7D', '30D', '60D']}
              value={statsPeriod}
              onChange={(value) => setStatsPeriod(value as '7D' | '30D' | '60D')}
            />
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
          </Space>
        )}
      </div>

      {isLoadingNotifications ? (
        <div>
          {[1, 2, 3].map((key) => (
            <Card key={key} loading className="!mb-6" />
          ))}
        </div>
      ) : hasNotifications ? (
        <div>
          {notifications.map((notification) => (
            <TransactionalNotificationCard
              key={notification.id}
              notification={notification}
              workspace={currentWorkspace}
              permissions={permissions}
              stats={getStatsForNotification(notification.id)}
              isLoadingStats={isLoadingStats}
              onDelete={handleDeleteNotification}
              onTest={handleTestNotification}
              onShowApi={handleShowApiModal}
            />
          ))}
        </div>
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
          template={{
            id: notificationToTest.channels.email.template_id,
            name: notificationToTest.name,
            version: 0,
            category: 'transactional',
            channel: 'email',
            created_at: '',
            updated_at: ''
          }}
          workspace={currentWorkspace || null}
          withCCAndBCC={true}
        />
      )}
    </div>
  )
}
